// Package rest_test is the external test package for rest: it imports both
// rest and internal, which is only possible because it is a
// distinct package from rest itself (an internal `package rest` test file
// cannot import vertratest, since vertratest imports rest to implement
// rest.Client — see internal_test.go for the couple of checks that need
// package-internal access instead).
package rest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const (
	testBaseURL   = "https://api.test"
	testUserAgent = "vertracloud-sdk-go/test"
	testTimeout   = 30 * time.Second
	testAPIKey    = "test-key"
)

// newTestClient returns a rest.Client wired to a fresh
// vertratest.FakeRoundTripper, ready for any test in this file.
func newTestClient(t *testing.T, opts ...rest.ClientOpt) (rest.Client, *vertratest.FakeRoundTripper) {
	t.Helper()
	ft := vertratest.NewFakeRoundTripper(t)
	allOpts := append([]rest.ClientOpt{
		rest.WithBaseURL(testBaseURL),
		rest.WithUserAgent(testUserAgent),
		rest.WithTimeout(testTimeout),
		rest.WithHTTPClient(&http.Client{Transport: ft}),
	}, opts...)
	c, err := rest.NewClient(testAPIKey, allOpts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, ft
}

type fakePayload struct {
	ID string `json:"id"`
}

// --- construction -----------------------------------------------------------

func TestNewClient_RequiresAPIKey(t *testing.T) {
	if _, err := rest.NewClient(""); err == nil {
		t.Fatal("expected error for empty API key")
	}
}

func TestPublicClientOmitsAuthorization(t *testing.T) {
	fake := vertratest.NewFakeRoundTripper(t)
	fake.EnqueueJSON(http.StatusOK, `{"response":{"status":"healthy"}}`)
	client := rest.NewPublicClient(rest.WithBaseURL(testBaseURL), rest.WithHTTPClient(&http.Client{Transport: fake}))
	if _, err := client.Do(context.Background(), http.MethodGet, "/v1/status", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := fake.Requests[0].Headers.Get("Authorization"); got != "" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

// --- auth + user agent headers ----------------------------------------------

func TestDo_SetsAuthAndUserAgentHeaders(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueJSON(http.StatusOK, `{"id":"abc"}`)

	data, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	got, err := rest.DecodeJSON[fakePayload](data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.ID != "abc" {
		t.Errorf("ID = %q, want abc", got.ID)
	}

	req := ft.Requests[0]
	if req.Headers.Get("Authorization") != "Bearer "+testAPIKey {
		t.Errorf("Authorization = %q", req.Headers.Get("Authorization"))
	}
	if req.Headers.Get("User-Agent") != testUserAgent {
		t.Errorf("User-Agent = %q", req.Headers.Get("User-Agent"))
	}
}

// --- envelope unwrap ---------------------------------------------------------

func TestDecodeJSON_UnwrapsEnvelope(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueJSON(http.StatusOK, `{"id":"app-1"}`)

	data, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/app-1", nil, nil, "")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	got, err := rest.DecodeJSON[fakePayload](data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.ID != "app-1" {
		t.Errorf("ID = %q, want app-1", got.ID)
	}
}

func TestDecodeJSON_NoContentReturnsZeroValue(t *testing.T) {
	c, ft := newTestClient(t)
	ft.Enqueue(http.StatusNoContent, nil, nil)

	data, err := c.Do(context.Background(), http.MethodDelete, "/v1/apps/app-1", nil, nil, "")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	got, err := rest.DecodeJSON[fakePayload](data)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got != (fakePayload{}) {
		t.Errorf("got %+v, want zero value", got)
	}
}

func TestDo_SendsJSONBody(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueJSON(http.StatusOK, `{}`)

	_, err := c.Do(context.Background(), http.MethodPost, "/v1/apps", nil, strings.NewReader(`{"name":"demo"}`), "application/json")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !strings.Contains(string(ft.Requests[0].Body), `"name":"demo"`) {
		t.Errorf("body = %s, want name field", ft.Requests[0].Body)
	}
	if ft.Requests[0].Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", ft.Requests[0].Headers.Get("Content-Type"))
	}
}

// --- workspace_id query merging ----------------------------------------------

func TestDo_MergesWorkspaceID(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueJSON(http.StatusOK, `{}`)

	q := url.Values{"path": []string{"/src"}}
	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x/files", q, nil, "", rest.WithWorkspaceID("ws-1"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	u, _ := url.Parse(ft.Requests[0].URL)
	if u.Query().Get("workspace_id") != "ws-1" {
		t.Errorf("workspace_id missing from query: %s", ft.Requests[0].URL)
	}
	if u.Query().Get("path") != "/src" {
		t.Errorf("existing query param dropped: %s", ft.Requests[0].URL)
	}
}

func TestDo_NoWorkspaceIDOmitsParam(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueJSON(http.StatusOK, `{}`)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	u, _ := url.Parse(ft.Requests[0].URL)
	if u.Query().Has("workspace_id") {
		t.Errorf("workspace_id should be omitted, got: %s", ft.Requests[0].URL)
	}
}

// --- error predicates ---------------------------------------------------------

func TestAPIError_PredicatesByStatus(t *testing.T) {
	check := func(t *testing.T, status int, predicate func(*rest.APIError) bool) {
		t.Helper()
		c, ft := newTestClient(t)
		ft.EnqueueError(status, `{"code":"SOME_CODE","message":"boom"}`)

		_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
		if err == nil {
			t.Fatalf("status %d: expected error", status)
		}
		var apiErr *rest.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("status %d: errors.As to *APIError failed", status)
		}
		if apiErr.Status != status || apiErr.Code != "SOME_CODE" || apiErr.Message != "boom" {
			t.Errorf("status %d: unexpected APIError %+v", status, apiErr)
		}
		if !predicate(apiErr) {
			t.Errorf("status %d: predicate did not match", status)
		}
	}

	t.Run("401 IsAuthenticationError", func(t *testing.T) {
		check(t, http.StatusUnauthorized, (*rest.APIError).IsAuthenticationError)
	})
	t.Run("403 IsPermissionError", func(t *testing.T) {
		check(t, http.StatusForbidden, (*rest.APIError).IsPermissionError)
	})
	t.Run("404 IsNotFoundError", func(t *testing.T) {
		check(t, http.StatusNotFound, (*rest.APIError).IsNotFoundError)
	})
	t.Run("400 IsValidationError", func(t *testing.T) {
		check(t, http.StatusBadRequest, (*rest.APIError).IsValidationError)
	})
	t.Run("422 IsValidationError", func(t *testing.T) {
		check(t, http.StatusUnprocessableEntity, (*rest.APIError).IsValidationError)
	})
	t.Run("429 IsRateLimitError", func(t *testing.T) {
		check(t, http.StatusTooManyRequests, (*rest.APIError).IsRateLimitError)
	})
}

func TestAPIError_PredicatesAreExclusive(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusNotFound, `{"code":"X","message":"y"}`)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	var apiErr *rest.APIError
	errors.As(err, &apiErr)
	if apiErr.IsAuthenticationError() || apiErr.IsPermissionError() || apiErr.IsValidationError() || apiErr.IsRateLimitError() {
		t.Errorf("only IsNotFoundError should be true for 404: %+v", apiErr)
	}
	if !apiErr.IsNotFoundError() {
		t.Error("IsNotFoundError should be true for 404")
	}
}

func TestAPIError_FallsBackOnInvalidJSON(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusInternalServerError, `not json at all`)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if apiErr.Code != "HTTP_500" {
		t.Errorf("Code = %q, want HTTP_500", apiErr.Code)
	}
}

// DEPLOY_RATE_LIMITED is the real shape of the deploy limiter: retry_after
// lives inside details and no Retry-After header is sent.
func TestAPIError_RetryAfterFromDetails(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusTooManyRequests, `{"code":"DEPLOY_RATE_LIMITED","details":{"limit":10,"window_seconds":3600,"retry_after":30}}`)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	ra := apiErr.RetryAfter()
	if ra == nil || *ra != 30*time.Second {
		t.Errorf("RetryAfter() = %v, want 30s", ra)
	}
}

func TestAPIError_RetryAfterFromHeader(t *testing.T) {
	c, ft := newTestClient(t)
	header := http.Header{}
	header.Set("Retry-After", "12")
	ft.Enqueue(http.StatusTooManyRequests, []byte(`{"code":"RATE_LIMIT_EXCEEDED","details":{"scope":"minute","limit":60,"used":61}}`), header)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	ra := apiErr.RetryAfter()
	if ra == nil || *ra != 12*time.Second {
		t.Errorf("RetryAfter() = %v, want 12s", ra)
	}
}

func TestAPIError_ErrorStringFormat(t *testing.T) {
	err := &rest.APIError{Status: 404, Code: "APP_NOT_FOUND", Message: "no such app"}
	want := "APP_NOT_FOUND (404): no such app"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// --- security: API key never leaks -------------------------------------------

func TestAPIError_NeverLeaksAPIKey(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusUnauthorized, `{"code":"API_KEY_INVALID","message":"bad key"}`)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("error string leaks API key: %s", err.Error())
	}
	var apiErr *rest.APIError
	errors.As(err, &apiErr)
	if apiErr.Details != nil && strings.Contains(fmt.Sprint(apiErr.Details), testAPIKey) {
		t.Errorf("error details leak API key: %v", apiErr.Details)
	}
}

// --- timeout -------------------------------------------------------------------

func TestDo_DefaultTimeoutIsEnforced(t *testing.T) {
	ft := vertratest.NewFakeRoundTripper(t)
	c, err := rest.NewClient(testAPIKey, rest.WithBaseURL(testBaseURL), rest.WithHTTPClient(&http.Client{Transport: ft}), rest.WithTimeout(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ft.EnqueueDelayed(http.StatusOK, []byte(`{"response":{}}`), 200*time.Millisecond)

	start := time.Now()
	_, doErr := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "")
	elapsed := time.Since(start)

	if doErr == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(doErr, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded in chain", doErr)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("took %v, want well under the 200ms delay", elapsed)
	}
}

func TestDo_PerCallTimeoutOverridesClientDefault(t *testing.T) {
	ft := vertratest.NewFakeRoundTripper(t)
	c, err := rest.NewClient(testAPIKey, rest.WithBaseURL(testBaseURL), rest.WithHTTPClient(&http.Client{Transport: ft}), rest.WithTimeout(time.Hour))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ft.EnqueueDelayed(http.StatusOK, []byte(`{"response":{}}`), 200*time.Millisecond)

	_, doErr := c.Do(context.Background(), http.MethodGet, "/v1/apps/x", nil, nil, "", rest.WithRequestTimeout(20*time.Millisecond))
	if !errors.Is(doErr, context.DeadlineExceeded) {
		t.Errorf("expected per-call timeout to override client default, got: %v", doErr)
	}
}

// --- binary (no envelope) -------------------------------------------------------

func TestDo_BinaryResponseReturnsRawBytesNoEnvelope(t *testing.T) {
	c, ft := newTestClient(t)
	raw := []byte{0x50, 0x4b, 0x03, 0x04, 0x00, 0x01} // not JSON, looks like a zip header
	ft.Enqueue(http.StatusOK, raw, nil)

	got, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x/download", nil, nil, "")
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("got %v, want %v", got, raw)
	}
}

func TestDo_BinaryErrorStillTyped(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusNotFound, `{"code":"APP_NOT_FOUND","message":"gone"}`)

	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x/download", nil, nil, "")
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if !apiErr.IsNotFoundError() {
		t.Error("expected IsNotFoundError")
	}
}

// --- streamed responses (DoStream) ------------------------------------------

func TestDoStream_ReturnsBodyUnread(t *testing.T) {
	c, ft := newTestClient(t)
	raw := []byte{0x50, 0x4b, 0x03, 0x04}
	ft.Enqueue(http.StatusOK, raw, nil)

	body, err := c.DoStream(context.Background(), http.MethodGet, "/v1/apps/x/download", nil)
	if err != nil {
		t.Fatalf("DoStream: %v", err)
	}
	got, err := io.ReadAll(body)
	body.Close()
	if err != nil || string(got) != string(raw) {
		t.Errorf("got %v (%v), want %v", got, err, raw)
	}
	if ft.Requests[0].Headers.Get("Authorization") != "Bearer "+testAPIKey {
		t.Error("DoStream must authenticate")
	}
}

func TestDoStream_ErrorStatusIsTyped(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusNotFound, `{"code":"APP_NOT_FOUND","message":"gone"}`)

	_, err := c.DoStream(context.Background(), http.MethodGet, "/v1/apps/x/download", nil)
	apiErr, ok := rest.AsAPIError(err)
	if !ok || !apiErr.IsNotFoundError() {
		t.Fatalf("want typed 404, got %v", err)
	}
}

// A download slower than the client's default timeout must still finish:
// only an explicit WithRequestTimeout bounds a stream.
func TestDoStream_IgnoresClientDefaultTimeout(t *testing.T) {
	c, ft := newTestClient(t, rest.WithTimeout(20*time.Millisecond))
	ft.EnqueueDelayed(http.StatusOK, []byte("zip"), 60*time.Millisecond)

	body, err := c.DoStream(context.Background(), http.MethodGet, "/v1/apps/x/download", nil)
	if err != nil {
		t.Fatalf("DoStream: %v", err)
	}
	body.Close()

	ft.EnqueueDelayed(http.StatusOK, []byte("zip"), 60*time.Millisecond)
	if _, err := c.DoStream(context.Background(), http.MethodGet, "/v1/apps/x/download", nil, rest.WithRequestTimeout(20*time.Millisecond)); err == nil {
		t.Error("expected WithRequestTimeout to bound the stream")
	}
}

// --- per-call escape hatches ------------------------------------------------

func TestWithQueryAndWithHeader(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueJSON(http.StatusOK, `{}`)

	q := url.Values{"path": []string{"/src"}}
	_, err := c.Do(context.Background(), http.MethodGet, "/v1/apps/x/files", q, nil, "",
		rest.WithQuery("path", "/lib"), rest.WithQuery("new_param", "1"), rest.WithHeader("X-Trace", "abc"))
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	req := ft.Requests[0]
	u, _ := url.Parse(req.URL)
	if u.Query().Get("path") != "/lib" || u.Query().Get("new_param") != "1" {
		t.Errorf("query = %s", u.RawQuery)
	}
	if req.Headers.Get("X-Trace") != "abc" {
		t.Errorf("X-Trace = %q", req.Headers.Get("X-Trace"))
	}
	if q.Get("path") != "/src" {
		t.Error("caller's query must not be mutated")
	}
}

func TestAsAPIError(t *testing.T) {
	if _, ok := rest.AsAPIError(errors.New("plain")); ok {
		t.Error("plain error is not an APIError")
	}
	wrapped := fmt.Errorf("ctx: %w", &rest.APIError{Status: 404, Code: "APP_NOT_FOUND"})
	apiErr, ok := rest.AsAPIError(wrapped)
	if !ok || apiErr.Code != "APP_NOT_FOUND" {
		t.Errorf("AsAPIError(wrapped) = %v, %v", apiErr, ok)
	}
}
