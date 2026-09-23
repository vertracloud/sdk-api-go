package rest

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrSSEIdleTimeout is returned by EventStream.Err (and makes Next return
// false) when no event arrives within the stream's idle timeout.
var ErrSSEIdleTimeout = errors.New("vertracloud: sse idle timeout")

// SSEEvent is one parsed Server-Sent Events message. Data is always the raw
// opaque string from the wire — callers parse it as JSON themselves when
// the route documents a JSON payload.
type SSEEvent struct {
	Event string
	Data  string
	ID    string
}

type lineResult struct {
	line string
	err  error
}

// EventStream is a synchronous, pull-based iterator over a Server-Sent
// Events response body. Typical usage:
//
//	stream, err := client.Apps.Realtime(ctx, appID, nil)
//	defer stream.Close()
//	for stream.Next(ctx) {
//		ev := stream.Event()
//		...
//	}
//	if err := stream.Err(); err != nil {
//		...
//	}
type EventStream struct {
	body        io.ReadCloser
	reader      *bufio.Reader
	idleTimeout time.Duration

	lineCh chan lineResult
	closed chan struct{}
	once   sync.Once

	current SSEEvent
	err     error
}

// OpenEventStream opens a Server-Sent Events stream on path through c. The
// client's default timeout does not apply (the stream is long-lived by
// design); cancel ctx or pass WithIdleTimeout instead. Close the stream
// when done.
func OpenEventStream(ctx context.Context, c Client, path string, query url.Values, opts ...RequestOpt) (*EventStream, error) {
	opts = append(opts[:len(opts):len(opts)], WithHeader("Accept", "text/event-stream"))
	body, err := c.DoStream(ctx, http.MethodGet, path, query, opts...)
	if err != nil {
		return nil, err
	}
	return newEventStream(body, resolve(opts).IdleTimeout), nil
}

func newEventStream(body io.ReadCloser, idleTimeout time.Duration) *EventStream {
	es := &EventStream{
		body:        body,
		reader:      bufio.NewReader(body),
		idleTimeout: idleTimeout,
		lineCh:      make(chan lineResult),
		closed:      make(chan struct{}),
	}
	go es.pump()
	return es
}

// pump reads lines from the response body in the background so that Next
// can select on ctx cancellation and the idle timeout at the same time.
func (s *EventStream) pump() {
	defer close(s.lineCh)
	for {
		line, err := s.reader.ReadString('\n')
		select {
		case s.lineCh <- lineResult{line: line, err: err}:
		case <-s.closed:
			return
		}
		if err != nil {
			return
		}
	}
}

// Next advances to the next event, blocking until one is available, ctx is
// canceled, the idle timeout elapses, or the stream ends. It returns false
// at end of stream or on error — check Err to tell the two apart.
func (s *EventStream) Next(ctx context.Context) bool {
	if s.err != nil {
		return false
	}

	var ev SSEEvent
	var dataLines []string
	haveFields := false

	dispatch := func() bool {
		ev.Data = strings.Join(dataLines, "\n")
		s.current = ev
		return true
	}

	var timer *time.Timer
	var idleC <-chan time.Time
	if s.idleTimeout > 0 {
		timer = time.NewTimer(s.idleTimeout)
		defer timer.Stop()
		idleC = timer.C
	}

	for {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(s.idleTimeout)
		}

		select {
		case <-ctx.Done():
			s.err = ctx.Err()
			return false

		case <-idleC:
			s.err = ErrSSEIdleTimeout
			return false

		case lr, ok := <-s.lineCh:
			if !ok {
				if haveFields {
					return dispatch()
				}
				return false
			}
			if lr.err != nil && lr.err != io.EOF {
				s.err = lr.err
				return false
			}

			line := strings.TrimRight(lr.line, "\r\n")
			if line == "" {
				if haveFields {
					return dispatch()
				}
				// Empty line with nothing accumulated: keep reading,
				// unless the stream also ended right here.
				if lr.err == io.EOF {
					return false
				}
				continue
			}

			field, value := splitSSEField(line)
			switch field {
			case "event":
				ev.Event = value
				haveFields = true
			case "data":
				dataLines = append(dataLines, value)
				haveFields = true
			case "id":
				ev.ID = value
				haveFields = true
			default:
				// Unknown field (including "retry" and comments starting
				// with ':') is ignored, never causes a parse failure.
			}

			if lr.err == io.EOF {
				if haveFields {
					return dispatch()
				}
				return false
			}
		}
	}
}

// splitSSEField parses one SSE line into (field, value), stripping a single
// leading space from value per the SSE spec. A line with no colon is a
// field with an empty value; a line starting with ':' is a comment and
// parses to field == "".
func splitSSEField(line string) (field, value string) {
	idx := strings.IndexByte(line, ':')
	if idx == -1 {
		return line, ""
	}
	field = line[:idx]
	value = line[idx+1:]
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	return field, value
}

// Event returns the event produced by the most recent successful Next call.
func (s *EventStream) Event() SSEEvent {
	return s.current
}

// Err returns the error that stopped iteration, or nil if the stream ended
// cleanly (server closed the connection with no error).
func (s *EventStream) Err() error {
	return s.err
}

// Close stops the background reader and closes the underlying response
// body. Safe to call more than once, and safe to call after Next has
// already returned false.
func (s *EventStream) Close() error {
	var err error
	s.once.Do(func() {
		close(s.closed)
		err = s.body.Close()
	})
	return err
}
