// Package vertratest holds test doubles shared by the SDK's own test
// suites. It lives under internal/ so the Go toolchain keeps it out of
// reach of any consumer of the SDK — it is test infrastructure, never part
// of the public API — while still being importable from both
// github.com/vertracloud/sdk-api-go/rest and github.com/vertracloud/sdk-api-go/vertracloud,
// which both need a fake to drive.
//
// Two fakes live here, for two different layers:
//
//   - FakeRoundTripper (this file) implements http.RoundTripper. It is used
//     by rest package tests, which need to exercise real HTTP request
//     construction (headers, query encoding, status handling) end to end.
//   - FakeRestClient (fakerest.go) implements rest.Client directly, with no
//     HTTP involved at all. It is used by vertracloud package tests, which
//     only care that a domain service calls Do/DoSSE with the right
//     method/path/query/body — not how those bytes travel over the wire.
//
// Neither file has a _test.go suffix on purpose: a _test.go file cannot be
// imported by another package's tests, and both of these need to be.
package vertratest

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"
)

// CapturedRequest records one HTTP call observed by FakeRoundTripper.
type CapturedRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// fakeResponse is one queued response for FakeRoundTripper to return.
type fakeResponse struct {
	Status int
	Body   []byte
	// BodyReader, when set, is used verbatim as the response body instead
	// of Body — e.g. an io.Pipe end that never closes, for tests that need
	// a response body which streams (or stalls) rather than arriving
	// fully-buffered.
	BodyReader io.ReadCloser
	Header     http.Header
	Delay      time.Duration
	Err        error
}

// FakeRoundTripper is an http.RoundTripper test double. It records every
// request it sees and serves responses from a FIFO queue.
type FakeRoundTripper struct {
	t         *testing.T
	Requests  []CapturedRequest
	responses []fakeResponse
}

// NewFakeRoundTripper returns an empty FakeRoundTripper bound to t (used to
// fail the test loudly if a request arrives with no queued response left).
func NewFakeRoundTripper(t *testing.T) *FakeRoundTripper {
	return &FakeRoundTripper{t: t}
}

// Enqueue adds a raw response to the FIFO queue.
func (f *FakeRoundTripper) Enqueue(status int, body []byte, header http.Header) {
	f.responses = append(f.responses, fakeResponse{Status: status, Body: body, Header: header})
}

// EnqueueJSON wraps body (a raw JSON fragment, e.g. `{"id":"1"}` or `[1,2]`)
// in the {"response": ...} envelope and enqueues it with status 200.
func (f *FakeRoundTripper) EnqueueJSON(status int, body string) {
	f.Enqueue(status, []byte(`{"response":`+body+`}`), nil)
}

// EnqueueError enqueues a raw error body, e.g. `{"code":"X","message":"Y"}`.
func (f *FakeRoundTripper) EnqueueError(status int, body string) {
	f.Enqueue(status, []byte(body), nil)
}

// EnqueueDelayed enqueues a response that only resolves after delay (or
// when the request's context is canceled first, whichever comes first).
// Used to exercise timeout behavior.
func (f *FakeRoundTripper) EnqueueDelayed(status int, body []byte, delay time.Duration) {
	f.responses = append(f.responses, fakeResponse{Status: status, Body: body, Delay: delay})
}

// EnqueueBody enqueues a response whose body is read directly from body
// (e.g. an *io.PipeReader the test controls), instead of a fully-buffered
// []byte — for tests that need to observe behavior while the body is still
// arriving (idle timeouts, cancellation mid-stream).
func (f *FakeRoundTripper) EnqueueBody(status int, body io.ReadCloser, header http.Header) {
	f.responses = append(f.responses, fakeResponse{Status: status, BodyReader: body, Header: header})
}

func (f *FakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			f.t.Fatalf("FakeRoundTripper: read request body: %v", err)
		}
		body = b
	}
	f.Requests = append(f.Requests, CapturedRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: req.Header.Clone(),
		Body:    body,
	})

	if len(f.responses) == 0 {
		f.t.Fatalf("FakeRoundTripper: no queued response for %s %s", req.Method, req.URL.String())
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]

	if resp.Delay > 0 {
		select {
		case <-time.After(resp.Delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	if resp.Err != nil {
		return nil, resp.Err
	}

	header := resp.Header
	if header == nil {
		header = http.Header{}
		header.Set("Content-Type", "application/json")
	}
	respBody := resp.BodyReader
	if respBody == nil {
		respBody = io.NopCloser(bytes.NewReader(resp.Body))
	}
	return &http.Response{
		StatusCode: resp.Status,
		Body:       respBody,
		Header:     header,
		Request:    req,
	}, nil
}
