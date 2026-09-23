// Package rest is the low-level HTTP transport for the Vertra Cloud Go SDK:
// authentication, timeouts, the {"response": ...} envelope (DecodeJSON),
// typed errors (APIError) and Server-Sent Events (EventStream). It knows
// nothing about Vertra Cloud's domains — those live in the sibling
// vertracloud package, which is what most programs should use.
package rest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/reqopts"
)

const (
	defaultBaseURL = "https://api.vertracloud.app"
	defaultTimeout = 30 * time.Second

	// sdkVersion must match the module's release tag.
	sdkVersion = "0.1.0"
)

// Client is the low-level HTTP client for the Vertra Cloud public API. It
// knows nothing about Vertra domain types (Application, Database, ...) —
// only how to execute one authenticated request. No method retries
// automatically.
//
// Client is an interface so tests can substitute a fake; new methods may be
// added to it in minor releases, so implement it only in tests.
type Client interface {
	// Do executes one request and returns the full response body.
	// body/contentType are both optional (nil/""). A status >= 400
	// response becomes a non-nil *APIError.
	Do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, opts ...RequestOpt) ([]byte, error)
	// DoStream executes one request and returns the response body without
	// reading it — for downloads and event streams. The client's default
	// timeout is not applied (only WithRequestTimeout, when given); the
	// caller must Close the body. A status >= 400 response becomes a
	// non-nil *APIError.
	DoStream(ctx context.Context, method, path string, query url.Values, opts ...RequestOpt) (io.ReadCloser, error)
}

// clientConfig holds every setting ClientOpt can change, resolved to
// defaults by NewClient before building the client.
type clientConfig struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
	timeout    time.Duration
}

// ClientOpt configures a Client during NewClient.
type ClientOpt func(*clientConfig)

// WithBaseURL overrides the API base URL. Default: https://api.vertracloud.app.
func WithBaseURL(baseURL string) ClientOpt {
	return func(c *clientConfig) {
		if baseURL != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithHTTPClient injects a custom *http.Client, e.g. one with a fake
// Transport for tests. Default: &http.Client{}.
func WithHTTPClient(hc *http.Client) ClientOpt {
	return func(c *clientConfig) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithTimeout overrides the default per-request timeout. Default: 30s.
func WithTimeout(d time.Duration) ClientOpt {
	return func(c *clientConfig) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithUserAgent overrides the default User-Agent header. Default:
// "vertracloud-sdk-go/<version>".
func WithUserAgent(ua string) ClientOpt {
	return func(c *clientConfig) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// client is the only implementation of Client in this package.
type client struct {
	apiKey     string
	baseURL    string
	userAgent  string
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient builds a Client. apiKey is required; every other setting is an
// option with a working default.
func NewClient(apiKey string, opts ...ClientOpt) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("vertracloud: API key is required")
	}
	c := newClient(opts)
	c.apiKey = apiKey
	return c, nil
}

// NewPublicClient builds a Client without an API key, for public routes
// such as the platform status.
func NewPublicClient(opts ...ClientOpt) Client {
	return newClient(opts)
}

func newClient(opts []ClientOpt) *client {
	cfg := &clientConfig{
		baseURL:    defaultBaseURL,
		userAgent:  "vertracloud-sdk-go/" + sdkVersion,
		httpClient: &http.Client{},
		timeout:    defaultTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return &client{baseURL: cfg.baseURL, userAgent: cfg.userAgent, httpClient: cfg.httpClient, timeout: cfg.timeout}
}

// newRequest builds the outgoing request: path + query (with workspace_id
// and WithQuery overrides), auth, User-Agent, Content-Type and WithHeader
// overrides, in that order.
func (c *client) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, cfg reqopts.Config) (*http.Request, error) {
	q := url.Values{}
	for k, v := range query {
		q[k] = append([]string(nil), v...)
	}
	if cfg.WorkspaceID != "" {
		q.Set("workspace_id", cfg.WorkspaceID)
	}
	for k, v := range cfg.Query {
		q[k] = append([]string(nil), v...)
	}
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		if rc, ok := body.(io.Closer); ok {
			rc.Close() // unblocks a streaming multipart writer
		}
		return nil, fmt.Errorf("vertracloud: build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range cfg.Header {
		req.Header[k] = append([]string(nil), v...)
	}
	return req, nil
}

func (c *client) Do(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, opts ...RequestOpt) ([]byte, error) {
	cfg := resolve(opts)

	timeout := c.timeout
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	req, err := c.newRequest(ctx, method, path, query, body, contentType, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: read response body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, apiErrorFromHTTP(resp.StatusCode, data, resp.Header.Get("Retry-After"))
	}
	return data, nil
}

func (c *client) DoStream(ctx context.Context, method, path string, query url.Values, opts ...RequestOpt) (io.ReadCloser, error) {
	cfg := resolve(opts)

	cancel := context.CancelFunc(func() {})
	if cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	}

	req, err := c.newRequest(ctx, method, path, query, nil, "", cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("vertracloud: request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer cancel()
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, apiErrorFromHTTP(resp.StatusCode, data, resp.Header.Get("Retry-After"))
	}
	return &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}, nil
}

// cancelOnClose releases the per-call timeout context when the caller is
// done with a streamed body.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
