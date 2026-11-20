package vertracloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

const mib = 1 << 20

func downloadClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	rc, err := rest.NewClient("test-key", rest.WithBaseURL(srv.URL), rest.WithTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return New(rc)
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// 400 MB reach the disk with a flat heap, the temporary file grows before the
// response ends, and the final file is byte-identical.
func TestSnapshots_DownloadToFile_ConstantMemoryAndIdenticalFile(t *testing.T) {
	const total = 400
	dir := t.TempDir()
	target := filepath.Join(dir, "snapshot.zip")
	expected := sha256.New()
	var peak atomic.Uint64
	var midSize atomic.Int64
	var finalNameBeforeEnd atomic.Bool

	var base runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&base)

	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		chunk := make([]byte, mib)
		for i := 0; i < total; i++ {
			for j := range chunk {
				chunk[j] = byte(i % 251)
			}
			expected.Write(chunk)
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if i%10 == 0 {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				if m.HeapInuse > peak.Load() {
					peak.Store(m.HeapInuse)
				}
			}
			if i == total/2 {
				time.Sleep(200 * time.Millisecond)
				for _, name := range dirNames(t, dir) {
					if strings.HasSuffix(name, ".part") {
						if st, err := os.Stat(filepath.Join(dir, name)); err == nil {
							midSize.Store(st.Size())
						}
					}
					if name == "snapshot.zip" {
						finalNameBeforeEnd.Store(true)
					}
				}
			}
		}
	})
	n, err := c.Snapshots.DownloadToFile(context.Background(), "res_1", "snap_1", SnapshotScopeApplications, target)
	if err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	if n != total*mib {
		t.Fatalf("bytes = %d, want %d", n, total*mib)
	}
	if midSize.Load() < 100*mib {
		t.Errorf("temporary file had %d bytes at half of the response, want >= 100 MiB", midSize.Load())
	}
	if finalNameBeforeEnd.Load() {
		t.Error("the final name existed before the end")
	}
	if growth := int64(peak.Load()) - int64(base.HeapInuse); growth > 96*mib {
		t.Errorf("heap grew %d MiB for a %d MiB file", growth/mib, total)
	}
	if got := sha256File(t, target); got != hex.EncodeToString(expected.Sum(nil)) {
		t.Error("file differs from what the server sent")
	}
	if names := dirNames(t, dir); len(names) != 1 || names[0] != "snapshot.zip" {
		t.Errorf("dir = %v, want only snapshot.zip", names)
	}
}

func TestSnapshots_DownloadToFile_StallFailsAndLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("part"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	started := time.Now()
	_, err := c.Snapshots.DownloadToFile(context.Background(), "res_1", "snap_1", SnapshotScopeApplications, filepath.Join(dir, "snapshot.zip"), rest.WithIdleTimeout(200*time.Millisecond))
	if !errors.Is(err, ErrDownloadStalled) {
		t.Fatalf("err = %v, want ErrDownloadStalled", err)
	}
	if time.Since(started) > 3*time.Second {
		t.Errorf("took %s", time.Since(started))
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("dir = %v, want empty", names)
	}
}

func TestSnapshots_DownloadToFile_SlowButSteadyOutlivesTheIdleTimeout(t *testing.T) {
	dir := t.TempDir()
	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		for range 6 {
			_, _ = w.Write([]byte("chunk"))
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	})
	target := filepath.Join(dir, "snapshot.zip")
	n, err := c.Snapshots.DownloadToFile(context.Background(), "res_1", "snap_1", SnapshotScopeApplications, target, rest.WithIdleTimeout(250*time.Millisecond))
	if err != nil || n != 30 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
}

func TestSnapshots_DownloadToFile_CancelledContextLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		for range 100 {
			if _, err := w.Write(make([]byte, 1024)); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)
	_, err := c.Snapshots.DownloadToFile(ctx, "res_1", "snap_1", SnapshotScopeApplications, filepath.Join(dir, "snapshot.zip"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("dir = %v, want empty", names)
	}
}

func TestSnapshots_DownloadToFile_APIErrorLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"SNAPSHOT_NOT_FOUND","message":"nope"}`))
	})
	_, err := c.Snapshots.DownloadToFile(context.Background(), "res_1", "snap_1", SnapshotScopeApplications, filepath.Join(dir, "snapshot.zip"))
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "SNAPSHOT_NOT_FOUND" {
		t.Fatalf("err = %v, want APIError SNAPSHOT_NOT_FOUND", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("dir = %v, want empty", names)
	}
}

func TestApps_DownloadToFile_TakesTheSnapshotThenWritesTheZip(t *testing.T) {
	dir := t.TempDir()
	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"response":{"id":"snap_1","resource_id":"app_1"}}`))
			return
		}
		_, _ = w.Write([]byte("PK\x03\x04"))
	})
	target := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := c.Apps.DownloadToFile(context.Background(), "app_1", target, nil)
	if err != nil || n != 4 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "PK\x03\x04" {
		t.Errorf("file = %q", got)
	}
	if names := dirNames(t, dir); len(names) != 1 {
		t.Errorf("dir = %v", names)
	}
}

// A body cut before its Content-Length must not become a file: net/http
// reports it as io.ErrUnexpectedEOF.
func cutHandler(declared, sent int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(declared))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, sent))
		w.(http.Flusher).Flush()
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}
}

func TestSnapshots_DownloadToFile_BodyCutBeforeContentLengthLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	c := downloadClient(t, cutHandler(4*mib, 2*mib))
	target := filepath.Join(dir, "snapshot.zip")
	_, err := c.Snapshots.DownloadToFile(context.Background(), "res_1", "snap_1", SnapshotScopeApplications, target)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("dir = %v, want empty", names)
	}
}

func TestSnapshots_DownloadToFile_BodyCutDoesNotReplaceAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "snapshot.zip")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := downloadClient(t, cutHandler(1000, 10))
	if _, err := c.Snapshots.DownloadToFile(context.Background(), "res_1", "snap_1", SnapshotScopeApplications, target); err == nil {
		t.Fatal("err = nil, want an error")
	}
	if got, _ := os.ReadFile(target); string(got) != "old" {
		t.Errorf("file = %q, want old", got)
	}
	if names := dirNames(t, dir); len(names) != 1 {
		t.Errorf("dir = %v", names)
	}
}

func TestSnapshots_Download_BodyCutFailsTheRead(t *testing.T) {
	c := downloadClient(t, cutHandler(1000, 10))
	body, err := c.Snapshots.Download(context.Background(), "res_1", "snap_1", SnapshotScopeApplications)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if _, err := io.ReadAll(body); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
}

// rest.WithWorkspaceID reaches every request of the flow: take the snapshot,
// follow the job, download.
func TestApps_DownloadToFile_SendsTheWorkspaceOnEveryCall(t *testing.T) {
	const ws = "33333333-3333-3333-3333-333333333333"
	var urls []string
	c := downloadClient(t, func(w http.ResponseWriter, r *http.Request) {
		urls = append(urls, r.Method+" "+r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"response":{"job_id":"job_1"}}`))
		case strings.Contains(r.URL.Path, "/jobs/"):
			_, _ = w.Write([]byte(`{"response":{"id":"job_1","kind":"snapshot","resource_id":"app_1","status":"ready","snapshot_id":"snap_1"}}`))
		case strings.HasSuffix(r.URL.Path, "/download"):
			_, _ = w.Write([]byte("PK\x03\x04"))
		default:
			_, _ = w.Write([]byte(`{"response":[{"id":"snap_1","resource_id":"app_1"}]}`))
		}
	})
	target := filepath.Join(t.TempDir(), "app.zip")
	if _, err := c.Apps.DownloadToFile(context.Background(), "app_1", target, &WaitOptions{Interval: time.Millisecond}, rest.WithWorkspaceID(ws)); err != nil {
		t.Fatal(err)
	}
	if len(urls) != 4 {
		t.Fatalf("calls = %v", urls)
	}
	for _, u := range urls {
		if !strings.Contains(u, "workspace_id="+ws) {
			t.Errorf("no workspace_id: %s", u)
		}
	}
}
