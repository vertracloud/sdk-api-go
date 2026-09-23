// Package reqopts holds the resolved form of rest.RequestOpt. It lives
// under internal/ so the SDK's own test fakes can read what a caller asked
// for without the rest package exporting its plumbing.
package reqopts

import (
	"net/http"
	"net/url"
	"time"
)

// Config is what a slice of rest.RequestOpt resolves to.
type Config struct {
	WorkspaceID string
	Timeout     time.Duration
	IdleTimeout time.Duration
	Query       url.Values
	Header      http.Header
}
