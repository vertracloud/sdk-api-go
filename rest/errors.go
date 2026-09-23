package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// APIError is the single error type returned for every non-2xx response
// from the Vertra Cloud API. It never carries the API key: only the HTTP
// status, the server-provided error code, message and details are stored.
// Callers distinguish error categories with the Is*() predicate methods
// instead of a type switch/errors.As on distinct error types.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details any

	// retryAfter is unexported: callers read it through RetryAfter() so the
	// zero value of APIError (used freely in tests) never needs a
	// constructor.
	retryAfter *time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (%d): %s", e.Code, e.Status, e.Message)
}

// IsAuthenticationError reports an HTTP 401 response (invalid or missing
// API key, unknown user, etc).
func (e *APIError) IsAuthenticationError() bool { return e.Status == http.StatusUnauthorized }

// IsPermissionError reports an HTTP 403 response: scope denial, IP
// allowlist denial, WEBSITE_ONLY routes and account bans all land here.
func (e *APIError) IsPermissionError() bool { return e.Status == http.StatusForbidden }

// IsNotFoundError reports an HTTP 404 response.
func (e *APIError) IsNotFoundError() bool { return e.Status == http.StatusNotFound }

// IsValidationError reports an HTTP 400 or 422 response (invalid body,
// query or path parameters).
func (e *APIError) IsValidationError() bool {
	return e.Status == http.StatusBadRequest || e.Status == http.StatusUnprocessableEntity
}

// IsRateLimitError reports an HTTP 429 response (request, daily or deploy
// limit). See RetryAfter for how long to wait.
func (e *APIError) IsRateLimitError() bool { return e.Status == http.StatusTooManyRequests }

// RetryAfter returns the wait duration from the error's details.retry_after
// field (seconds) or, failing that, from the Retry-After header. Nil when
// neither was present, which is the normal case for every status except
// 429.
func (e *APIError) RetryAfter() *time.Duration { return e.retryAfter }

// Until returns details.until when the API supplied a valid RFC3339 timestamp.
func (e *APIError) Until() *time.Time {
	data, err := json.Marshal(e.Details)
	if err != nil {
		return nil
	}
	var details struct {
		Until *time.Time `json:"until"`
	}
	if json.Unmarshal(data, &details) != nil {
		return nil
	}
	return details.Until
}

// ValidationPath returns details.path for validation errors, or an empty
// string when the API did not include one.
func (e *APIError) ValidationPath() string {
	data, err := json.Marshal(e.Details)
	if err != nil {
		return ""
	}
	var details struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(data, &details) != nil {
		return ""
	}
	return details.Path
}

// AsAPIError reports whether err (or anything it wraps) is an *APIError,
// and returns it:
//
//	if apiErr, ok := rest.AsAPIError(err); ok && apiErr.IsNotFoundError() { ... }
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	ok := errors.As(err, &apiErr)
	return apiErr, ok
}

// errorBody mirrors the wire shape of an API error response:
// {code, message?, details?}. Rate-limit responses carry
// details.retry_after.
type errorBody struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}

// apiErrorFromHTTP builds an *APIError from a response status, body and the
// Retry-After header. If body is not valid JSON (or carries no code), it
// falls back to a generic Code: "HTTP_<status>" rather than assuming a
// well-formed error body. details.retry_after takes precedence over the
// Retry-After header; the header is only consulted when details omitted it
// (the deploy limiter sends no header at all).
func apiErrorFromHTTP(status int, body []byte, retryAfterHeader string) *APIError {
	var parsed errorBody
	code := fmt.Sprintf("HTTP_%d", status)
	message := http.StatusText(status)
	var details any
	var retry struct {
		RetryAfter *float64 `json:"retry_after"`
	}

	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Code != "" {
		code = parsed.Code
		if parsed.Message != "" {
			message = parsed.Message
		}
		if len(parsed.Details) > 0 {
			_ = json.Unmarshal(parsed.Details, &details)
			_ = json.Unmarshal(parsed.Details, &retry) // details may not be an object
		}
	}

	e := &APIError{Status: status, Code: code, Message: message, Details: details}

	if retry.RetryAfter != nil {
		d := time.Duration(*retry.RetryAfter * float64(time.Second))
		e.retryAfter = &d
	} else if retryAfterHeader != "" {
		if secs, perr := strconv.ParseFloat(retryAfterHeader, 64); perr == nil {
			d := time.Duration(secs * float64(time.Second))
			e.retryAfter = &d
		}
	}

	return e
}
