package rest

import (
	"net/http"
	"net/url"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/reqopts"
)

// RequestOpt configures a single call. Build one with the With* functions
// in this package.
type RequestOpt func(*reqopts.Config)

// WithWorkspaceID scopes the call to a workspace resource instead of a
// personal one. Sent as the workspace_id query parameter; omitted from the
// request entirely when never set.
func WithWorkspaceID(id string) RequestOpt {
	return func(c *reqopts.Config) { c.WorkspaceID = id }
}

// WithRequestTimeout overrides the client's default timeout for this call
// alone. On a streamed call (Download, Realtime) there is no timeout unless
// this is set.
func WithRequestTimeout(d time.Duration) RequestOpt {
	return func(c *reqopts.Config) { c.Timeout = d }
}

// WithIdleTimeout makes a Server-Sent Events stream fail with
// ErrSSEIdleTimeout when no line arrives within d. Ignored by other calls.
// Zero (the default) means no idle timeout.
func WithIdleTimeout(d time.Duration) RequestOpt {
	return func(c *reqopts.Config) { c.IdleTimeout = d }
}

// WithQuery sets an extra query parameter on the call, replacing any value
// the SDK itself set for the same key. Use it for API parameters the SDK
// does not model yet.
func WithQuery(key, value string) RequestOpt {
	return func(c *reqopts.Config) {
		if c.Query == nil {
			c.Query = url.Values{}
		}
		c.Query.Set(key, value)
	}
}

// WithHeader sets an extra HTTP header on the call, replacing any value the
// SDK itself set for the same key.
func WithHeader(key, value string) RequestOpt {
	return func(c *reqopts.Config) {
		if c.Header == nil {
			c.Header = http.Header{}
		}
		c.Header.Set(key, value)
	}
}

func resolve(opts []RequestOpt) reqopts.Config {
	var c reqopts.Config
	for _, opt := range opts {
		opt(&c)
	}
	return c
}
