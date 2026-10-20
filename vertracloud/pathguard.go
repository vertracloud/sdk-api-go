package vertracloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/vertracloud/sdk-api-go/rest"
)

// ErrInvalidPathSegment is returned, before any request is made, when an
// identifier that becomes a URL path segment is empty, "." or "..", also when
// percent-encoded. HTTP clients and servers collapse dot-segments, so such a
// call would reach a different route than the one requested. Test with
// errors.Is.
var ErrInvalidPathSegment = errors.New("vertracloud: invalid path segment")

func isDotSegment(segment string) bool {
	current := segment
	for i := 0; i < 4; i++ {
		if current == "" || current == "." || current == ".." {
			return true
		}
		decoded, err := url.PathUnescape(current)
		if err != nil || decoded == current {
			return false
		}
		current = decoded
	}
	return false
}

// checkPath validates every segment of an already escaped request path.
// Segments are escaped one by one (url.PathEscape), so a "/" inside an id
// never splits a segment.
func checkPath(path string) error {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if isDotSegment(segment) {
			return fmt.Errorf("%w: %q", ErrInvalidPathSegment, segment)
		}
	}
	return nil
}

// pathGuard refuses invalid path segments before delegating to the wrapped
// client; every service builds its requests through one.
type pathGuard struct{ rest.Client }

func guardPaths(rc rest.Client) rest.Client {
	if rc == nil {
		return nil
	}
	if _, ok := rc.(pathGuard); ok {
		return rc
	}
	return pathGuard{rc}
}

func (g pathGuard) Do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, opts ...rest.RequestOpt) ([]byte, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	return g.Client.Do(ctx, method, path, query, body, contentType, opts...)
}

func (g pathGuard) DoStream(ctx context.Context, method, path string, query url.Values, opts ...rest.RequestOpt) (io.ReadCloser, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	return g.Client.DoStream(ctx, method, path, query, opts...)
}
