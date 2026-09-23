package vertratest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/reqopts"
	"github.com/vertracloud/sdk-api-go/rest"
)

// CapturedCall records one call observed by FakeRestClient.
type CapturedCall struct {
	Method      string
	Path        string
	Query       url.Values
	Body        []byte
	ContentType string
	// WorkspaceID is extracted from the resolved RequestOpts, so a test can
	// assert a domain method threaded rest.WithWorkspaceID through without
	// inspecting opts itself.
	WorkspaceID string
	// Header holds the headers set through rest.WithHeader (streamed calls
	// only).
	Header http.Header
}

// fakeRestResponse is one queued response for FakeRestClient.Do.
type fakeRestResponse struct {
	status int
	body   []byte
}

// FakeRestClient implements rest.Client directly, with no real HTTP
// involved: it exists so vertracloud domain service tests can assert what a
// method sent (method/path/query/body/workspace) and control what comes
// back, without constructing an *http.Client and a RoundTripper.
type FakeRestClient struct {
	t         *testing.T
	Requests  []CapturedCall
	responses []fakeRestResponse
}

// NewFakeRestClient returns an empty FakeRestClient bound to t (used to
// fail the test loudly if a call arrives with no queued response left).
func NewFakeRestClient(t *testing.T) *FakeRestClient {
	return &FakeRestClient{t: t}
}

// EnqueueJSON wraps rawJSON (e.g. `{"id":"1"}` or `[1,2]`) in the
// {"response": ...} envelope and enqueues it with status.
func (f *FakeRestClient) EnqueueJSON(status int, rawJSON string) {
	f.responses = append(f.responses, fakeRestResponse{status: status, body: []byte(`{"response":` + rawJSON + `}`)})
}

// EnqueueRaw enqueues body verbatim, with no envelope — for binary
// responses such as zip/certificate downloads.
func (f *FakeRestClient) EnqueueRaw(status int, body []byte) {
	f.responses = append(f.responses, fakeRestResponse{status: status, body: body})
}

// EnqueueError enqueues a raw error body, e.g. `{"code":"X","message":"Y"}`.
func (f *FakeRestClient) EnqueueError(status int, rawJSON string) {
	f.responses = append(f.responses, fakeRestResponse{status: status, body: []byte(rawJSON)})
}

// EnqueueSSE enqueues raw Server-Sent Events text (or an error body, when
// status >= 400) for the next streamed call.
func (f *FakeRestClient) EnqueueSSE(status int, raw string) {
	f.responses = append(f.responses, fakeRestResponse{status: status, body: []byte(raw)})
}

// Do implements rest.Client.
func (f *FakeRestClient) Do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, opts ...rest.RequestOpt) ([]byte, error) {
	var b []byte
	if body != nil {
		read, err := io.ReadAll(body)
		if err != nil {
			f.t.Fatalf("FakeRestClient: read request body: %v", err)
		}
		b = read
	}
	f.Requests = append(f.Requests, CapturedCall{
		Method:      method,
		Path:        path,
		Query:       query,
		Body:        b,
		ContentType: contentType,
		WorkspaceID: resolve(opts).WorkspaceID,
	})
	return f.next(method, path)
}

// DoStream implements rest.Client, serving the next queued response (from
// EnqueueRaw or EnqueueSSE) as the stream body.
func (f *FakeRestClient) DoStream(ctx context.Context, method, path string, query url.Values, opts ...rest.RequestOpt) (io.ReadCloser, error) {
	cfg := resolve(opts)
	f.Requests = append(f.Requests, CapturedCall{
		Method:      method,
		Path:        path,
		Query:       query,
		WorkspaceID: cfg.WorkspaceID,
		Header:      cfg.Header,
	})
	data, err := f.next(method, path)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *FakeRestClient) next(method, path string) ([]byte, error) {
	if len(f.responses) == 0 {
		f.t.Fatalf("FakeRestClient: no queued response for %s %s", method, path)
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]

	if resp.status >= 400 {
		return nil, fakeAPIError(resp.status, resp.body)
	}
	return resp.body, nil
}

func resolve(opts []rest.RequestOpt) reqopts.Config {
	var c reqopts.Config
	for _, opt := range opts {
		opt(&c)
	}
	return c
}

// fakeAPIError parses body the same way the real client's error path does
// (best-effort {code, message, details}, falling back to HTTP_<status>)
// and builds a *rest.APIError. It cannot set the unexported retry-after
// field, since that's real-HTTP behavior the rest package tests already
// cover directly.
func fakeAPIError(status int, body []byte) *rest.APIError {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details"`
	}
	_ = json.Unmarshal(body, &parsed)
	code := parsed.Code
	if code == "" {
		code = fmt.Sprintf("HTTP_%d", status)
	}
	return &rest.APIError{Status: status, Code: code, Message: parsed.Message, Details: parsed.Details}
}
