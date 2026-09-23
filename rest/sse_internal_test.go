package rest

// EventStream parsing tests; they need the unexported constructor.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func newTestEventStream(raw string, idleTimeout time.Duration) *EventStream {
	return newEventStream(io.NopCloser(strings.NewReader(raw)), idleTimeout)
}

func TestEventStream_ParsesMultipleEvents(t *testing.T) {
	raw := "event: log\ndata: line one\nid: 1\n\n" +
		"event: log\ndata: line two\n\n"
	s := newTestEventStream(raw, 0)
	defer s.Close()

	var got []SSEEvent
	for s.Next(context.Background()) {
		got = append(got, s.Event())
	}
	if err := s.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Event != "log" || got[0].Data != "line one" || got[0].ID != "1" {
		t.Errorf("event[0] = %+v", got[0])
	}
	if got[1].Event != "log" || got[1].Data != "line two" {
		t.Errorf("event[1] = %+v", got[1])
	}
}

func TestEventStream_MultilineDataJoinedWithNewline(t *testing.T) {
	raw := "data: line one\ndata: line two\n\n"
	s := newTestEventStream(raw, 0)
	defer s.Close()

	if !s.Next(context.Background()) {
		t.Fatalf("expected an event, err=%v", s.Err())
	}
	if s.Event().Data != "line one\nline two" {
		t.Errorf("Data = %q", s.Event().Data)
	}
}

func TestEventStream_IgnoresCommentsAndRetry(t *testing.T) {
	raw := ": keep-alive\nretry: 5000\ndata: hello\n\n"
	s := newTestEventStream(raw, 0)
	defer s.Close()

	if !s.Next(context.Background()) {
		t.Fatalf("expected an event, err=%v", s.Err())
	}
	if s.Event().Data != "hello" {
		t.Errorf("Data = %q", s.Event().Data)
	}
}

func TestEventStream_MalformedLinesDoNotPanic(t *testing.T) {
	raw := "not-a-valid-sse-line-at-all\n\ndata: still works\n\n"
	s := newTestEventStream(raw, 0)
	defer s.Close()

	count := 0
	for s.Next(context.Background()) {
		count++
	}
	if err := s.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d events, want 1", count)
	}
}

func TestEventStream_EOFWithoutTrailingBlankLineFlushesLastEvent(t *testing.T) {
	raw := "event: last\ndata: no trailing blank line"
	s := newTestEventStream(raw, 0)
	defer s.Close()

	if !s.Next(context.Background()) {
		t.Fatalf("expected the final event to flush on EOF, err=%v", s.Err())
	}
	if s.Event().Event != "last" || s.Event().Data != "no trailing blank line" {
		t.Errorf("event = %+v", s.Event())
	}
	if s.Next(context.Background()) {
		t.Fatal("expected no further events")
	}
	if err := s.Err(); err != nil {
		t.Fatalf("expected clean EOF, got: %v", err)
	}
}

func TestEventStream_ContextCancellation(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s := newEventStream(pr, 0)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if s.Next(ctx) {
		t.Fatal("expected Next to return false for a canceled context")
	}
	if !errors.Is(s.Err(), context.Canceled) {
		t.Errorf("Err() = %v, want context.Canceled", s.Err())
	}
}

func TestEventStream_IdleTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	s := newEventStream(pr, 20*time.Millisecond)
	defer s.Close()

	if s.Next(context.Background()) {
		t.Fatal("expected Next to return false on idle timeout")
	}
	if !errors.Is(s.Err(), ErrSSEIdleTimeout) {
		t.Errorf("Err() = %v, want ErrSSEIdleTimeout", s.Err())
	}
}

func TestEventStream_CloseIsIdempotentAndUnblocksNext(t *testing.T) {
	pr, pw := io.Pipe()
	s := newEventStream(pr, 0)

	done := make(chan struct{})
	go func() {
		s.Next(context.Background())
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Next did not unblock after Close")
	}
	pw.Close()
}
