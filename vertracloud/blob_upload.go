package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

const (
	// blobSimpleMaxBytes is the largest file sent in one multipart request
	// through the API; bigger files use a direct (pre-signed) upload.
	blobSimpleMaxBytes = 5 << 20
	blobPartBatch      = 100
	blobDefaultConc    = 4
	blobDefaultRetries = 3
)

// blobRetryBackoff is the base delay between part retries (var so tests can shrink it).
var blobRetryBackoff = 300 * time.Millisecond

// blobProgress serializes OnProgress calls.
type blobProgress struct {
	mu     sync.Mutex
	loaded int64
	total  int64
	fn     func(loaded, total int64)
}

func (p *blobProgress) add(n int64) {
	if n == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loaded += n
	if p.fn != nil {
		p.fn(p.loaded, p.total)
	}
}

// countReader reports bytes read to a blobProgress and keeps its own count so
// a failed attempt can be rolled back.
type countReader struct {
	r io.Reader
	n int64
	p *blobProgress
}

func (c *countReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	c.p.add(int64(n))
	return n, err
}

// Put uploads r (size bytes) to path. Files up to 5 MB go in one multipart
// request through the API; larger ones use a direct upload (a single PUT, or
// parts in parallel when the API asks for multipart). Cancelling ctx aborts a
// direct upload on the server, best-effort. Use Resume to continue one.
func (b *BlobClient) Put(ctx context.Context, path string, r io.Reader, size int64, o *BlobPutOptions, opts ...rest.RequestOpt) (BlobObject, error) {
	if o == nil {
		o = &BlobPutOptions{}
	}
	if size < 0 {
		return BlobObject{}, errors.New("vertracloud: Put needs the exact size of the reader")
	}
	prog := &blobProgress{total: size, fn: o.OnProgress}
	if size <= blobSimpleMaxBytes {
		return b.putSimple(ctx, path, r, size, o, prog, opts)
	}
	up, err := blobCall[BlobUpload](ctx, b, http.MethodPost, "/uploads", nil, o.uploadBody(path, size), opts)
	if err != nil {
		return BlobObject{}, err
	}
	obj, err := b.runUpload(ctx, up, r, o, prog, opts)
	if err != nil && ctx.Err() != nil {
		b.abortUpload(ctx, up.ID, opts)
	}
	return obj, err
}

// Resume continues an interrupted direct upload (see GET /uploads/{id}).
// r must give the same file from its first byte; parts the server already has
// are skipped. Cancelling ctx here does not abort the upload.
func (b *BlobClient) Resume(ctx context.Context, uploadID string, r io.Reader, o *BlobPutOptions, opts ...rest.RequestOpt) (BlobObject, error) {
	if o == nil {
		o = &BlobPutOptions{}
	}
	up, err := blobCall[BlobUpload](ctx, b, http.MethodGet, "/uploads/"+url.PathEscape(uploadID), nil, nil, opts)
	if err != nil {
		return BlobObject{}, err
	}
	if up.Status != "pending" {
		return BlobObject{}, fmt.Errorf("vertracloud: upload %s is %s, not pending", uploadID, up.Status)
	}
	return b.runUpload(ctx, up, r, o, &blobProgress{total: up.Size, fn: o.OnProgress}, opts)
}

func (o *BlobPutOptions) uploadBody(path string, size int64) any {
	return struct {
		Path               string                 `json:"path"`
		Size               int64                  `json:"size"`
		Visibility         BlobVisibility         `json:"visibility,omitempty"`
		Overwrite          *bool                  `json:"overwrite,omitempty"`
		RandomSuffix       *bool                  `json:"random_suffix,omitempty"`
		RenameOnConflict   *bool                  `json:"rename_on_conflict,omitempty"`
		CacheControl       string                 `json:"cache_control,omitempty"`
		ContentDisposition BlobContentDisposition `json:"content_disposition,omitempty"`
		Filename           string                 `json:"filename,omitempty"`
		ExpiresAt          *time.Time             `json:"expires_at,omitempty"`
		ChecksumSHA256     string                 `json:"checksum_sha256,omitempty"`
		Metadata           map[string]string      `json:"metadata,omitempty"`
	}{path, size, o.Visibility, o.Overwrite, o.RandomSuffix, o.RenameOnConflict, o.CacheControl, o.ContentDisposition, o.Filename, o.ExpiresAt, o.ChecksumSHA256, o.Metadata}
}

func (b *BlobClient) putSimple(ctx context.Context, p string, r io.Reader, size int64, o *BlobPutOptions, prog *blobProgress, opts []rest.RequestOpt) (BlobObject, error) {
	q := url.Values{"path": {p}}
	addQueryParam(q, "visibility", string(o.Visibility))
	addQueryBool(q, "overwrite", o.Overwrite)
	addQueryBool(q, "random_suffix", o.RandomSuffix)
	addQueryBool(q, "rename_on_conflict", o.RenameOnConflict)
	addQueryParam(q, "cache_control", o.CacheControl)
	addQueryParam(q, "content_disposition", string(o.ContentDisposition))
	addQueryParam(q, "filename", o.Filename)
	addQueryParam(q, "checksum_sha256", o.ChecksumSHA256)
	if o.ExpiresAt != nil {
		q.Set("expires_at", o.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if len(o.Metadata) > 0 {
		raw, err := json.Marshal(o.Metadata)
		if err != nil {
			return BlobObject{}, fmt.Errorf("vertracloud: encode metadata: %w", err)
		}
		q.Set("metadata", string(raw))
	}
	body, ct := newMultipartBody([]multipartField{{
		Name: "file", FileName: path.Base(p), Reader: &countReader{r: io.LimitReader(r, size), p: prog},
	}})
	// The default 30s request timeout would cut a slow link, so this call gets a 30-minute cap.
	opts = append(opts[:len(opts):len(opts)], rest.WithRequestTimeout(30*time.Minute))
	data, err := b.rest.Do(ctx, http.MethodPost, "/objects", q, body, ct, opts...)
	if err != nil {
		return BlobObject{}, err
	}
	return rest.DecodeJSON[BlobObject](data)
}

func (b *BlobClient) abortUpload(ctx context.Context, id string, opts []rest.RequestOpt) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, _ = blobCall[any](ctx, b, http.MethodDelete, "/uploads/"+url.PathEscape(id), nil, nil, opts)
}

func (b *BlobClient) runUpload(ctx context.Context, up BlobUpload, r io.Reader, o *BlobPutOptions, prog *blobProgress, opts []rest.RequestOpt) (BlobObject, error) {
	var err error
	switch up.Mode {
	case BlobUploadModeSingle:
		// No retry on the single PUT (a stream cannot rewind); Resume re-sends it.
		cr := &countReader{r: io.LimitReader(r, up.Size), p: prog}
		err = b.putTo(ctx, up.URL, up.Headers, cr, up.Size)
	case BlobUploadModeMultipart:
		err = b.putParts(ctx, up, r, o, prog, opts)
	default:
		err = fmt.Errorf("vertracloud: unknown upload mode %q", up.Mode)
	}
	if err != nil {
		return BlobObject{}, err
	}
	return blobCall[BlobObject](ctx, b, http.MethodPost, "/uploads/"+url.PathEscape(up.ID)+"/complete", nil, nil, opts)
}

// putTo PUTs body to a pre-signed URL with exactly the headers the API
// returned and no credentials.
func (b *BlobClient) putTo(ctx context.Context, rawURL string, headers map[string]string, body io.Reader, length int64) error {
	if length == 0 {
		body = http.NoBody
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, body)
	if err != nil {
		return fmt.Errorf("vertracloud: build upload request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.ContentLength = length
	resp, err := b.http.Do(req)
	if err != nil {
		return fmt.Errorf("vertracloud: upload failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &rest.APIError{Status: resp.StatusCode, Code: fmt.Sprintf("HTTP_%d", resp.StatusCode), Message: "upload to storage failed"}
	}
	return nil
}

func blobRetryable(err error) bool {
	var apiErr *rest.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500 || apiErr.Status == 429 || apiErr.Status == 408
	}
	return true
}

// putPart uploads one part, retrying transient failures and rolling the
// progress of a failed attempt back.
func (b *BlobClient) putPart(ctx context.Context, part BlobUploadPart, length int64, retries int, prog *blobProgress, body func() io.Reader) error {
	for attempt := 0; ; attempt++ {
		cr := &countReader{r: body(), p: prog}
		err := b.putTo(ctx, part.URL, part.Headers, cr, length)
		if err == nil {
			return nil
		}
		prog.add(-cr.n)
		if attempt >= retries || ctx.Err() != nil || !blobRetryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(blobRetryBackoff * time.Duration(attempt+1)):
		}
	}
}

func (b *BlobClient) putParts(ctx context.Context, up BlobUpload, r io.Reader, o *BlobPutOptions, prog *blobProgress, opts []rest.RequestOpt) error {
	if up.PartSize <= 0 || up.PartCount <= 0 {
		return errors.New("vertracloud: multipart upload without part_size/part_count")
	}
	partLen := func(n int) int64 { return min(up.PartSize, up.Size-int64(n-1)*up.PartSize) }
	have := map[int]bool{}
	var already int64
	for _, n := range up.UploadedParts {
		have[n] = true
		already += partLen(n)
	}
	prog.add(already)
	var pending []int
	for n := 1; n <= up.PartCount; n++ {
		if !have[n] {
			pending = append(pending, n)
		}
	}
	conc, retries := o.Concurrency, o.PartRetries
	if conc <= 0 {
		conc = blobDefaultConc
	}
	if retries <= 0 {
		retries = blobDefaultRetries
	}
	ra, seekable := r.(io.ReaderAt)
	var buf []byte
	next := 1 // sequential mode: next part number the reader is positioned at

	for i := 0; i < len(pending); i += blobPartBatch {
		batch := pending[i:min(i+blobPartBatch, len(pending))]
		urls, err := blobCall[[]BlobUploadPart](ctx, b, http.MethodPost, "/uploads/"+url.PathEscape(up.ID)+"/parts", nil,
			struct {
				PartNumbers []int `json:"part_numbers"`
			}{batch}, opts)
		if err != nil {
			return err
		}
		byNum := make(map[int]BlobUploadPart, len(urls))
		for _, p := range urls {
			byNum[p.PartNumber] = p
		}
		for _, n := range batch {
			if _, ok := byNum[n]; !ok {
				return fmt.Errorf("vertracloud: API returned no URL for part %d", n)
			}
		}

		if !seekable {
			if buf == nil {
				buf = make([]byte, up.PartSize)
			}
			for _, n := range batch {
				for ; next < n; next++ { // skip parts the server already has
					if _, err := io.CopyN(io.Discard, r, partLen(next)); err != nil {
						return fmt.Errorf("vertracloud: reader ended before part %d: %w", next, err)
					}
				}
				l := partLen(n)
				if _, err := io.ReadFull(r, buf[:l]); err != nil {
					return fmt.Errorf("vertracloud: reader ended before part %d: %w", n, err)
				}
				next = n + 1
				chunk := buf[:l]
				if err := b.putPart(ctx, byNum[n], l, retries, prog, func() io.Reader { return bytes.NewReader(chunk) }); err != nil {
					return err
				}
			}
			continue
		}

		bctx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		var once sync.Once
		var firstErr error
		sem := make(chan struct{}, conc)
		for _, n := range batch {
			sem <- struct{}{}
			if bctx.Err() != nil {
				<-sem
				break
			}
			wg.Add(1)
			go func(n int) {
				defer func() { <-sem; wg.Done() }()
				off, l := int64(n-1)*up.PartSize, partLen(n)
				err := b.putPart(bctx, byNum[n], l, retries, prog, func() io.Reader { return io.NewSectionReader(ra, off, l) })
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
				}
			}(n)
		}
		wg.Wait()
		cancel()
		if firstErr != nil {
			return firstErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}
