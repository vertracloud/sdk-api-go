package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

type blobReq struct {
	Method, Path, EscPath string
	Query                 map[string][]string
	Header                http.Header
	Body                  []byte
}

// blobFake is an httptest server that records every request and lets a test
// answer each one.
type blobFake struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []blobReq
	h    func(w http.ResponseWriter, r *http.Request, body []byte)
}

func newBlobFake(t *testing.T, h func(w http.ResponseWriter, r *http.Request, body []byte)) *blobFake {
	f := &blobFake{h: h}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, blobReq{r.Method, r.URL.Path, r.URL.EscapedPath(), r.URL.Query(), r.Header.Clone(), body})
		f.mu.Unlock()
		f.h(w, r, body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *blobFake) client(t *testing.T) *BlobClient {
	rc, err := rest.NewClient("key_123", rest.WithBaseURL("http://unused.invalid"))
	if err != nil {
		t.Fatal(err)
	}
	return New(rc, WithBlobBaseURL(f.srv.URL+"/v1")).Blob
}

func (f *blobFake) find(method, path string) []blobReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []blobReq
	for _, r := range f.reqs {
		if r.Method == method && r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

func writeResp(w http.ResponseWriter, status int, v any) {
	raw, _ := json.Marshal(map[string]any{"response": v})
	w.WriteHeader(status)
	w.Write(raw)
}

const objJSON = `{"id":"1","path":"a/b.txt","size":3}`

func objResp(w http.ResponseWriter) {
	w.Write([]byte(`{"response":` + objJSON + `}`))
}

func TestBlobDefaultsAndAuth(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) { objResp(w) })
	c := f.client(t)
	if _, err := c.Get(context.Background(), "1", rest.WithWorkspaceID("ws1")); err != nil {
		t.Fatal(err)
	}
	r := f.reqs[0]
	if r.Path != "/v1/objects/1" || r.Header.Get("Authorization") != "Bearer key_123" || r.Query["workspace_id"][0] != "ws1" {
		t.Fatalf("bad request %+v", r)
	}
	// Default base URL derives from a real rest client.
	rc, _ := rest.NewClient("k")
	if _, ok := rc.(rest.BaseURLCloner); !ok {
		t.Fatal("rest client must be a BaseURLCloner")
	}
	if New(rc).Blob == nil {
		t.Fatal("Blob not registered")
	}
}

func TestBlobPutRenameOnConflict(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) { objResp(w) })
	c := f.client(t)
	yes := true
	if _, err := c.Put(context.Background(), "a/b.txt", strings.NewReader("abc"), 3, &BlobPutOptions{RenameOnConflict: &yes}); err != nil {
		t.Fatal(err)
	}
	if q := f.find("POST", "/v1/objects")[0].Query; len(q["rename_on_conflict"]) != 1 || q["rename_on_conflict"][0] != "true" {
		t.Fatalf("query %v", q)
	}
}

func TestBlobPutSimple(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) { objResp(w) })
	c := f.client(t)
	var last atomic.Int64
	vis, over := BlobVisibilityPublic, true
	_, err := c.Put(context.Background(), "a/b.txt", strings.NewReader("abc"), 3, &BlobPutOptions{
		Visibility: vis, Overwrite: &over, Metadata: map[string]string{"owner": "joao"},
		OnProgress: func(l, _ int64) { last.Store(l) },
	}, rest.WithWorkspaceID("ws1"))
	if err != nil {
		t.Fatal(err)
	}
	r := f.find("POST", "/v1/objects")[0]
	q := r.Query
	if q["path"][0] != "a/b.txt" || q["visibility"][0] != "public" || q["overwrite"][0] != "true" ||
		q["metadata"][0] != `{"owner":"joao"}` || q["workspace_id"][0] != "ws1" {
		t.Fatalf("query %v", q)
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") ||
		!strings.Contains(string(r.Body), `name=file`) || !strings.Contains(string(r.Body), "abc") {
		t.Fatalf("not a multipart file upload: %q", r.Body)
	}
	if last.Load() != 3 {
		t.Fatalf("progress %d", last.Load())
	}
}

// upload fake: direct uploads with configurable mode, storing parts by number.
type uploadFake struct {
	*blobFake
	mu       sync.Mutex
	parts    map[int][]byte
	single   []byte
	singleH  http.Header
	inFlight int32
	maxIn    int32
	failOnce map[int]bool
	partReqs [][]int
	size     int64
	partSize int64
	mode     string
	uploaded []int
	onPut    func(n int, r *http.Request)
}

func newUploadFake(t *testing.T, mode string, size, partSize int64) *uploadFake {
	u := &uploadFake{parts: map[int][]byte{}, failOnce: map[int]bool{}, size: size, partSize: partSize, mode: mode}
	count := 0
	if partSize > 0 {
		count = int((size + partSize - 1) / partSize)
	}
	var base string
	upload := func() map[string]any {
		m := map[string]any{"id": "up1", "path": "big.bin", "mode": mode, "status": "pending", "size": size,
			"headers": map[string]string{"x-amz-test": "yes"}}
		if mode == "single" {
			m["url"] = base + "/storage/single"
		} else {
			m["part_size"], m["part_count"], m["uploaded_parts"] = partSize, count, u.uploaded
		}
		return m
	}
	u.blobFake = newBlobFake(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/uploads", r.Method == "GET" && r.URL.Path == "/v1/uploads/up1":
			writeResp(w, 200, upload())
		case r.URL.Path == "/v1/uploads/up1/parts":
			var in struct {
				PartNumbers []int `json:"part_numbers"`
			}
			json.Unmarshal(body, &in)
			u.mu.Lock()
			u.partReqs = append(u.partReqs, in.PartNumbers)
			u.mu.Unlock()
			var out []map[string]any
			for _, n := range in.PartNumbers {
				out = append(out, map[string]any{"part_number": n, "url": fmt.Sprintf("%s/storage/part/%d", base, n),
					"headers": map[string]string{"x-amz-test": "yes"}})
			}
			writeResp(w, 200, out)
		case r.URL.Path == "/v1/uploads/up1/complete":
			objResp(w)
		case r.URL.Path == "/v1/uploads/up1" && r.Method == "DELETE":
			writeResp(w, 200, nil)
		case r.Method == "PUT" && r.URL.Path == "/storage/single":
			u.mu.Lock()
			u.single, u.singleH = body, r.Header.Clone()
			u.mu.Unlock()
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/storage/part/"):
			n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/storage/part/"))
			cur := atomic.AddInt32(&u.inFlight, 1)
			for {
				m := atomic.LoadInt32(&u.maxIn)
				if cur <= m || atomic.CompareAndSwapInt32(&u.maxIn, m, cur) {
					break
				}
			}
			defer atomic.AddInt32(&u.inFlight, -1)
			if u.onPut != nil {
				u.onPut(n, r)
			}
			time.Sleep(10 * time.Millisecond)
			u.mu.Lock()
			fail := u.failOnce[n]
			delete(u.failOnce, n)
			if !fail {
				u.parts[n] = body
			}
			u.mu.Unlock()
			if fail {
				w.WriteHeader(503)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	base = u.srv.URL
	return u
}

func (u *uploadFake) assembled(count int) []byte {
	var b []byte
	for n := 1; n <= count; n++ {
		b = append(b, u.parts[n]...)
	}
	return b
}

func testData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

// onlyReader hides ReaderAt so the sequential path is used.
type onlyReader struct{ r io.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

func TestBlobPutDirectSingle(t *testing.T) {
	size := 6 << 20
	u := newUploadFake(t, "single", int64(size), 0)
	data := testData(size)
	_, err := u.client(t).Put(context.Background(), "big.bin", bytes.NewReader(data), int64(size), &BlobPutOptions{Visibility: BlobVisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(u.single, data) {
		t.Fatal("body mismatch")
	}
	if u.singleH.Get("x-amz-test") != "yes" || u.singleH.Get("Authorization") != "" {
		t.Fatalf("headers %v", u.singleH)
	}
	if u.find("PUT", "/storage/single")[0].Header.Get("Content-Length") != strconv.Itoa(size) {
		t.Fatal("content-length missing")
	}
	var body map[string]any
	json.Unmarshal(u.find("POST", "/v1/uploads")[0].Body, &body)
	if body["path"] != "big.bin" || body["size"].(float64) != float64(size) || body["visibility"] != "private" {
		t.Fatalf("init body %v", body)
	}
	if len(u.find("POST", "/v1/uploads/up1/complete")) != 1 {
		t.Fatal("complete not called")
	}
}

func TestBlobPutMultipartConcurrent(t *testing.T) {
	size, ps := 5<<20+10, int64(1<<20)
	u := newUploadFake(t, "multipart", int64(size), ps)
	data := testData(size)
	var last atomic.Int64
	_, err := u.client(t).Put(context.Background(), "big.bin", bytes.NewReader(data), int64(size), &BlobPutOptions{
		Concurrency: 3, OnProgress: func(l, total int64) {
			if l > total {
				t.Errorf("progress %d > %d", l, total)
			}
			last.Store(l)
		},
	}, rest.WithWorkspaceID("ws9"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(u.assembled(6), data) {
		t.Fatal("assembled mismatch")
	}
	if u.maxIn < 2 || u.maxIn > 3 {
		t.Fatalf("max in flight %d, want 2..3", u.maxIn)
	}
	if last.Load() != int64(size) {
		t.Fatalf("final progress %d", last.Load())
	}
	if u.find("POST", "/v1/uploads/up1/parts")[0].Query["workspace_id"][0] != "ws9" {
		t.Fatal("workspace_id missing on parts call")
	}
}

func TestBlobPutMultipartBatchesOf100(t *testing.T) {
	size, ps := 5<<20+1, int64(10000) // 525 parts
	u := newUploadFake(t, "multipart", int64(size), ps)
	data := testData(size)
	if _, err := u.client(t).Put(context.Background(), "big.bin", bytes.NewReader(data), int64(size), nil); err != nil {
		t.Fatal(err)
	}
	if len(u.partReqs) != 6 {
		t.Fatalf("%d part batches, want 6", len(u.partReqs))
	}
	for _, b := range u.partReqs {
		if len(b) > 100 {
			t.Fatalf("batch of %d", len(b))
		}
	}
	if !bytes.Equal(u.assembled(525), data) {
		t.Fatal("assembled mismatch")
	}
}

func TestBlobPutMultipartSequentialReader(t *testing.T) {
	size, ps := 5<<20+10, int64(1<<20)
	u := newUploadFake(t, "multipart", int64(size), ps)
	data := testData(size)
	if _, err := u.client(t).Put(context.Background(), "big.bin", onlyReader{bytes.NewReader(data)}, int64(size), nil); err != nil {
		t.Fatal(err)
	}
	if u.maxIn != 1 {
		t.Fatalf("sequential reader uploaded %d parts at once", u.maxIn)
	}
	if !bytes.Equal(u.assembled(6), data) {
		t.Fatal("assembled mismatch")
	}
}

func TestBlobPartRetry(t *testing.T) {
	blobRetryBackoff = time.Millisecond
	for name, wrap := range map[string]func(io.Reader) io.Reader{
		"readerat": func(r io.Reader) io.Reader { return r }, "sequential": func(r io.Reader) io.Reader { return onlyReader{r} },
	} {
		t.Run(name, func(t *testing.T) {
			size, ps := 5<<20+10, int64(1<<20)
			u := newUploadFake(t, "multipart", int64(size), ps)
			u.failOnce[2] = true
			data := testData(size)
			var last atomic.Int64
			_, err := u.client(t).Put(context.Background(), "big.bin", wrap(bytes.NewReader(data)), int64(size), &BlobPutOptions{
				OnProgress: func(l, total int64) {
					if l > total || l < 0 {
						t.Errorf("progress %d of %d", l, total)
					}
					last.Store(l)
				}})
			if err != nil {
				t.Fatal(err)
			}
			if n := len(u.find("PUT", "/storage/part/2")); n != 2 {
				t.Fatalf("part 2 sent %d times, want 2", n)
			}
			if !bytes.Equal(u.assembled(6), data) || last.Load() != int64(size) {
				t.Fatal("bad result after retry")
			}
		})
	}
}

func TestBlobPartNonRetryableFails(t *testing.T) {
	size, ps := 5<<20+10, int64(1<<20)
	u := newUploadFake(t, "multipart", int64(size), ps)
	u.onPut = func(n int, r *http.Request) {}
	// 403 from storage (expired URL) is not retried.
	orig := u.blobFake.h
	u.blobFake.h = func(w http.ResponseWriter, r *http.Request, b []byte) {
		if r.URL.Path == "/storage/part/1" {
			w.WriteHeader(403)
			return
		}
		orig(w, r, b)
	}
	_, err := u.client(t).Put(context.Background(), "big.bin", bytes.NewReader(testData(size)), int64(size), nil)
	apiErr, ok := rest.AsAPIError(err)
	if !ok || apiErr.Status != 403 {
		t.Fatalf("err = %v", err)
	}
	if n := len(u.find("PUT", "/storage/part/1")); n != 1 {
		t.Fatalf("403 retried %d times", n)
	}
}

func TestBlobPutCancelAborts(t *testing.T) {
	size, ps := 5<<20+10, int64(1<<20)
	u := newUploadFake(t, "multipart", int64(size), ps)
	ctx, cancel := context.WithCancel(context.Background())
	u.onPut = func(n int, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}
	_, err := u.client(t).Put(ctx, "big.bin", bytes.NewReader(testData(size)), int64(size), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(u.find("DELETE", "/v1/uploads/up1")) != 1 {
		t.Fatal("upload not aborted on cancel")
	}
	if len(u.find("POST", "/v1/uploads/up1/complete")) != 0 {
		t.Fatal("complete must not run")
	}
}

func TestBlobResumeSkipsUploadedParts(t *testing.T) {
	for name, wrap := range map[string]func(io.Reader) io.Reader{
		"readerat": func(r io.Reader) io.Reader { return r }, "sequential": func(r io.Reader) io.Reader { return onlyReader{r} },
	} {
		t.Run(name, func(t *testing.T) {
			size, ps := 5<<20+10, int64(1<<20)
			u := newUploadFake(t, "multipart", int64(size), ps)
			u.uploaded = []int{1, 2, 5}
			data := testData(size)
			for _, n := range u.uploaded {
				u.parts[n] = data[int64(n-1)*ps : min(int64(n)*ps, int64(size))]
			}
			var first atomic.Int64
			first.Store(-1)
			_, err := u.client(t).Resume(context.Background(), "up1", wrap(bytes.NewReader(data)), &BlobPutOptions{
				OnProgress: func(l, _ int64) { first.CompareAndSwap(-1, l) }})
			if err != nil {
				t.Fatal(err)
			}
			if got := u.partReqs[0]; fmt.Sprint(got) != "[3 4 6]" {
				t.Fatalf("requested parts %v", got)
			}
			if len(u.find("PUT", "/storage/part/1")) != 0 || !bytes.Equal(u.assembled(6), data) {
				t.Fatal("resume re-sent or corrupted parts")
			}
			if first.Load() < 3<<20 {
				t.Fatalf("progress should start counting uploaded parts, first=%d", first.Load())
			}
			if len(u.find("DELETE", "/v1/uploads/up1")) != 0 {
				t.Fatal("resume must not abort")
			}
		})
	}
}

func TestBlobList(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.URL.Query().Get("cursor") == "" {
			w.Write([]byte(`{"response":{"objects":[{"id":"1"},{"id":"2"}],"folders":["a/"],"cursor":"c2"}}`))
			return
		}
		w.Write([]byte(`{"response":{"objects":[{"id":"3"}],"folders":[],"cursor":null}}`))
	})
	it := f.client(t).List(context.Background(), &BlobListOptions{Prefix: "images/", Delimiter: "/", Limit: 2}, rest.WithWorkspaceID("w"))
	var ids []string
	for it.Next(context.Background()) {
		ids = append(ids, it.Object().ID)
	}
	if it.Err() != nil || strings.Join(ids, ",") != "1,2,3" || len(it.Folders()) != 1 {
		t.Fatalf("ids=%v err=%v", ids, it.Err())
	}
	q := f.reqs[0].Query
	if q["prefix"][0] != "images/" || q["delimiter"][0] != "/" || q["limit"][0] != "2" || q["workspace_id"][0] != "w" {
		t.Fatalf("query %v", q)
	}
	if f.reqs[1].Query["cursor"][0] != "c2" || len(f.reqs) != 2 {
		t.Fatal("cursor not followed")
	}
}

func TestBlobListError(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.WriteHeader(403)
		w.Write([]byte(`{"code":"API_KEY_SCOPE_DENIED"}`))
	})
	it := f.client(t).List(context.Background(), nil)
	if it.Next(context.Background()) {
		t.Fatal("Next should be false")
	}
	if e, ok := rest.AsAPIError(it.Err()); !ok || e.Code != "API_KEY_SCOPE_DENIED" {
		t.Fatalf("err %v", it.Err())
	}
}

func TestBlobDeleteManyRepeatsWhileRemaining(t *testing.T) {
	calls := 0
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		calls++
		if calls < 3 {
			w.Write([]byte(`{"response":{"deleted":["a","b"],"not_found":[],"remaining":true}}`))
			return
		}
		w.Write([]byte(`{"response":{"deleted":["c"],"not_found":["x"],"remaining":false}}`))
	})
	c := f.client(t)
	res, err := c.DeleteMany(context.Background(), BlobDeleteManyOptions{Prefix: "tmp/"})
	if err != nil || calls != 3 || len(res.Deleted) == 0 || len(res.Deleted) != 5 || len(res.NotFound) != 1 || res.Remaining {
		t.Fatalf("calls=%d res=%+v err=%v", calls, res, err)
	}
	if string(f.reqs[0].Body) != `{"prefix":"tmp/"}` || f.reqs[0].Method != "DELETE" || f.reqs[0].Path != "/v1/objects" {
		t.Fatalf("bad request %+v", f.reqs[0])
	}
	if _, err := c.DeleteMany(context.Background(), BlobDeleteManyOptions{}); err == nil {
		t.Fatal("empty options must fail")
	}
}

func TestBlobDeleteManyChunksIDs(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Write([]byte(`{"response":{"deleted":[],"not_found":["z"],"remaining":false}}`))
	})
	ids := make([]string, 2500)
	for i := range ids {
		ids[i] = strconv.Itoa(i)
	}
	if _, err := f.client(t).DeleteMany(context.Background(), BlobDeleteManyOptions{IDs: ids}); err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 3 {
		t.Fatalf("%d calls, want 3 chunks", len(f.reqs))
	}
}

func TestBlobObjectRoutes(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch {
		case r.Method == "DELETE" && r.URL.Path == "/v1/objects/1", r.Method == "DELETE" && r.URL.Path == "/v1/shares/s1", r.Method == "DELETE" && r.URL.Path == "/v1/domains/d1":
			w.Write([]byte(`{"response":null}`))
		case strings.HasSuffix(r.URL.Path, "/download"):
			w.Write([]byte(`{"response":{"url":"https://x/d/t","expires_at":"2026-01-01T00:00:00Z"}}`))
		case r.Method == "PATCH" && r.URL.Path == "/v1/objects":
			w.Write([]byte(`{"response":[{"id":"1","ok":true},{"id":"2","ok":false,"code":"BLOB_OBJECT_NOT_FOUND"}]}`))
		case r.URL.Path == "/v1/stats":
			w.Write([]byte(`{"response":{"objects":2,"used_bytes":10,"limits":{"max_rules":5,"uploads_per_second":null}}}`))
		case r.URL.Path == "/v1/shares" && r.Method == "POST":
			w.WriteHeader(201)
			w.Write([]byte(`{"response":{"id":"s1","object_id":"1","max_downloads":10}}`))
		case r.URL.Path == "/v1/shares":
			w.Write([]byte(`{"response":[{"id":"s1"}]}`))
		case r.URL.Path == "/v1/domains" && r.Method == "POST":
			w.WriteHeader(201)
			w.Write([]byte(`{"response":{"id":"d1","hostname":"cdn.x.com","verification":{"type":"txt","name":"_vertra.cdn.x.com","value":"abc"}}}`))
		case r.URL.Path == "/v1/domains":
			w.Write([]byte(`{"response":[{"id":"d1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/verify"):
			w.Write([]byte(`{"response":{"id":"d1","status":"active"}}`))
		case r.URL.Path == "/v1/upload-tokens":
			w.Write([]byte(`{"response":{"token":"vcup_x","max_uses":1}}`))
		case r.URL.Path == "/v1/s3/credentials":
			w.Write([]byte(`{"response":{"access_key_id":"VCS3X","region":"auto","buckets":["public","private"],"access":{"read":true,"write":false}}}`))
		default:
			objResp(w)
		}
	})
	c, ctx := f.client(t), context.Background()
	last := func() blobReq { return f.reqs[len(f.reqs)-1] }
	body := func() map[string]any { var m map[string]any; json.Unmarshal(last().Body, &m); return m }

	name := "new"
	if _, err := c.Update(ctx, "1", BlobUpdate{Visibility: BlobVisibilityPublic, CacheControl: NullableNull[string](), Filename: NullableValue(name), Metadata: map[string]*string{"owner": nil}}); err != nil {
		t.Fatal(err)
	}
	if last().Method != "PATCH" || last().Path != "/v1/objects/1" || string(last().Body) != `{"visibility":"public","cache_control":null,"filename":"new","metadata":{"owner":null}}` {
		t.Fatalf("update: %s %s", last().Method, last().Body)
	}
	res, err := c.UpdateMany(ctx, []string{"1", "2"}, BlobUpdate{Visibility: BlobVisibilityPrivate})
	if err != nil || len(res) != 2 || res[1].Code != "BLOB_OBJECT_NOT_FOUND" || string(last().Body) != `{"ids":["1","2"],"changes":{"visibility":"private"}}` {
		t.Fatalf("update many: %v %s", err, last().Body)
	}
	over := true
	c.Copy(ctx, "1", "b/c.txt", &BlobCopyOptions{Overwrite: &over})
	if last().Path != "/v1/objects/copy" || body()["move"] != nil || body()["source"] != "1" || body()["overwrite"] != true {
		t.Fatalf("copy %s", last().Body)
	}
	c.Copy(ctx, "1", "b/c.txt", &BlobCopyOptions{RenameOnConflict: &over})
	if body()["rename_on_conflict"] != true || body()["overwrite"] != nil {
		t.Fatalf("copy rename %s", last().Body)
	}
	c.Move(ctx, "1", "b/c.txt", nil)
	if body()["move"] != true {
		t.Fatalf("move %s", last().Body)
	}
	if err := c.Delete(ctx, "1"); err != nil || last().Method != "DELETE" {
		t.Fatal(err)
	}
	dl, err := c.DownloadURL(ctx, "1", &BlobDownloadOptions{ExpiresIn: 600, Disposition: BlobContentDispositionAttachment, Filename: "a b.txt"})
	if err != nil || dl.URL != "https://x/d/t" {
		t.Fatal(err)
	}
	if q := last().Query; q["redirect"][0] != "false" || q["expires_in"][0] != "600" || q["disposition"][0] != "attachment" || q["filename"][0] != "a b.txt" {
		t.Fatalf("download query %v", q)
	}
	st, err := c.Stats(ctx)
	if err != nil || st.Objects != 2 || st.Limits.UploadsPerSecond != nil || st.Limits.MaxRules != 5 {
		t.Fatalf("stats %+v %v", st, err)
	}
	sh, err := c.Shares.Create(ctx, BlobShareInput{ObjectID: "1", MaxDownloads: 10})
	if err != nil || sh.ID != "s1" || last().Path != "/v1/shares" || string(last().Body) != `{"object_id":"1","max_downloads":10}` {
		t.Fatalf("share %v %s", err, last().Body)
	}
	c.Shares.List(ctx, "1")
	if last().Query["object_id"][0] != "1" {
		t.Fatal("shares list filter")
	}
	if err := c.Shares.Revoke(ctx, "s1"); err != nil || last().Path != "/v1/shares/s1" {
		t.Fatal(err)
	}
	if d, err := c.Domains.Add(ctx, "cdn.x.com", "img/", true); err != nil || d.Hostname != "cdn.x.com" || d.Verification == nil || d.Verification.Name != "_vertra.cdn.x.com" || d.Verification.Value != "abc" || string(last().Body) != `{"hostname":"cdn.x.com","prefix":"img/","web_content":true}` {
		t.Fatalf("domain add %v %s", err, last().Body)
	}
	c.Domains.List(ctx)
	if _, err := c.Domains.Update(ctx, "d1", false); err != nil || last().Method != "PATCH" || last().Path != "/v1/domains/d1" || string(last().Body) != `{"web_content":false}` {
		t.Fatalf("domain update %v %s", err, last().Body)
	}
	if d, err := c.Domains.Verify(ctx, "d1"); err != nil || d.Status != "active" || last().Path != "/v1/domains/d1/verify" || last().Method != "POST" {
		t.Fatal(err)
	}
	if err := c.Domains.Delete(ctx, "d1"); err != nil {
		t.Fatal(err)
	}
	tok, err := c.CreateUploadToken(ctx, BlobUploadTokenInput{Prefix: "avatars/", MaxSize: 5000000})
	if err != nil || tok.Token != "vcup_x" || string(last().Body) != `{"prefix":"avatars/","max_size":5000000}` {
		t.Fatalf("token %v %s", err, last().Body)
	}
	s3, err := c.S3Credentials(ctx)
	if err != nil || s3.AccessKeyID != "VCS3X" || !s3.Access.Read || s3.Access.Write {
		t.Fatalf("s3 %+v %v", s3, err)
	}
}

func TestBlobRules(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		if r.Header.Get("If-Match") == "stale" {
			w.WriteHeader(409)
			w.Write([]byte(`{"code":"BLOB_RULES_CONFLICT"}`))
			return
		}
		w.Write([]byte(`{"response":{"rules":[{"prefix":"tmp/","delete_after_days":30}],"etag":"e2"}}`))
	})
	c, ctx := f.client(t), context.Background()
	rs, err := c.Rules.Get(ctx)
	if err != nil || rs.ETag != "e2" || *rs.Rules[0].DeleteAfterDays != 30 {
		t.Fatalf("%+v %v", rs, err)
	}
	days := 30
	if _, err := c.Rules.Set(ctx, []BlobRuleEntry{{Prefix: "tmp/", BlobRuleInput: BlobRuleInput{DeleteAfterDays: &days}}}, "e1"); err != nil {
		t.Fatal(err)
	}
	r := f.reqs[len(f.reqs)-1]
	if r.Method != "PUT" || r.Path != "/v1/rules" || r.Header.Get("If-Match") != "e1" || string(r.Body) != `{"rules":[{"prefix":"tmp/","delete_after_days":30}]}` {
		t.Fatalf("set %s %q", r.Header.Get("If-Match"), r.Body)
	}
	_, err = c.Rules.Set(ctx, nil, "stale")
	if e, ok := rest.AsAPIError(err); !ok || e.Code != "BLOB_RULES_CONFLICT" || e.Status != 409 {
		t.Fatalf("conflict err %v", err)
	}
	c.Rules.Put(ctx, "tmp/sub dir/", BlobRuleInput{Visibility: BlobVisibilityPublic})
	r = f.reqs[len(f.reqs)-1]
	if r.Method != "PUT" || r.EscPath != "/v1/rules/tmp%2Fsub%20dir%2F" || string(r.Body) != `{"visibility":"public"}` {
		t.Fatalf("put %s %s", r.EscPath, r.Body)
	}
	c.Rules.Delete(ctx, "tmp/")
	r = f.reqs[len(f.reqs)-1]
	if r.Method != "DELETE" || r.EscPath != "/v1/rules/tmp%2F" {
		t.Fatalf("delete %s %s", r.Method, r.EscPath)
	}
}

func TestBlobUpdateManyBatches(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		var in struct {
			IDs []string `json:"ids"`
		}
		json.Unmarshal(body, &in)
		items := make([]string, len(in.IDs))
		for i, id := range in.IDs {
			items[i] = fmt.Sprintf(`{"id":%q}`, id)
		}
		w.Write([]byte(`{"response":[` + strings.Join(items, ",") + `]}`))
	})
	ids := make([]string, 250)
	for i := range ids {
		ids[i] = strconv.Itoa(i)
	}
	res, err := f.client(t).UpdateMany(context.Background(), ids, BlobUpdate{Visibility: BlobVisibilityPrivate})
	if err != nil || len(res) != 250 || res[0].ID != "0" || res[249].ID != "249" {
		t.Fatalf("update many: %v len=%d", err, len(res))
	}
	reqs := f.find("PATCH", "/v1/objects")
	if len(reqs) != 3 {
		t.Fatalf("want 3 requests, got %d", len(reqs))
	}
	for i, want := range []int{100, 100, 50} {
		var in struct{ IDs []string }
		json.Unmarshal(reqs[i].Body, &in)
		if len(in.IDs) != want {
			t.Fatalf("batch %d has %d ids, want %d", i, len(in.IDs), want)
		}
	}
}

func TestBlobUploadTokenOverwrite(t *testing.T) {
	f := newBlobFake(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Write([]byte(`{"response":{"token":"vcup_x","max_uses":1}}`))
	})
	if _, err := f.client(t).CreateUploadToken(context.Background(), BlobUploadTokenInput{Prefix: "a/", Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	if got := string(f.reqs[0].Body); got != `{"prefix":"a/","overwrite":true}` {
		t.Fatalf("body %s", got)
	}
}
