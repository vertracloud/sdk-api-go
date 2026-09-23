package rest_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// --- OpenEventStream wiring -------------------------------------------------------

func TestOpenEventStream_ParsesEventsOverHTTP(t *testing.T) {
	c, ft := newTestClient(t)
	raw := "event: log\ndata: hello\n\n"
	header := http.Header{}
	header.Set("Content-Type", "text/event-stream")
	ft.Enqueue(http.StatusOK, []byte(raw), header)

	stream, err := rest.OpenEventStream(context.Background(), c, "/v1/apps/x/realtime", nil)
	if err != nil {
		t.Fatalf("OpenEventStream: %v", err)
	}
	defer stream.Close()

	if !stream.Next(context.Background()) {
		t.Fatalf("expected an event, err=%v", stream.Err())
	}
	if stream.Event().Event != "log" || stream.Event().Data != "hello" {
		t.Errorf("event = %+v", stream.Event())
	}

	req := ft.Requests[0]
	if req.Headers.Get("Accept") != "text/event-stream" {
		t.Errorf("Accept = %q", req.Headers.Get("Accept"))
	}
	if req.Headers.Get("Authorization") != "Bearer "+testAPIKey {
		t.Errorf("Authorization = %q", req.Headers.Get("Authorization"))
	}
}

func TestOpenEventStream_ErrorStatusIsTyped(t *testing.T) {
	c, ft := newTestClient(t)
	ft.EnqueueError(http.StatusForbidden, `{"code":"API_KEY_SCOPE_DENIED","message":"nope"}`)

	_, err := rest.OpenEventStream(context.Background(), c, "/v1/apps/x/realtime", nil)
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As to *APIError failed: %v", err)
	}
	if !apiErr.IsPermissionError() {
		t.Error("expected IsPermissionError")
	}
}

// TestOpenEventStream_IdleTimeoutTriggersOverHTTP proves WithIdleTimeout
// actually reaches the EventStream returned by OpenEventStream: the fake response
// body is a pipe end that never receives a write, so Next can only return
// false via the idle timeout, never via EOF.
func TestOpenEventStream_IdleTimeoutTriggersOverHTTP(t *testing.T) {
	c, ft := newTestClient(t)
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	ft.EnqueueBody(http.StatusOK, pr, nil)

	stream, err := rest.OpenEventStream(context.Background(), c, "/v1/apps/x/realtime", nil, rest.WithIdleTimeout(20*time.Millisecond))
	if err != nil {
		t.Fatalf("OpenEventStream: %v", err)
	}
	defer stream.Close()

	if stream.Next(context.Background()) {
		t.Fatal("expected Next to return false on idle timeout")
	}
	if !errors.Is(stream.Err(), rest.ErrSSEIdleTimeout) {
		t.Errorf("Err() = %v, want ErrSSEIdleTimeout", stream.Err())
	}
}
