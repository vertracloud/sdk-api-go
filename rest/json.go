package rest

import (
	"encoding/json"
	"fmt"
)

// envelope is the wire shape every JSON response is wrapped in:
// {"response": ...}.
type envelope[T any] struct {
	Response T `json:"response"`
}

// DecodeJSON unwraps the {"response": ...} envelope into T. It is a
// package-level generic function rather than a method on Client because Go
// does not support type parameters on methods (see
// https://go.dev/doc/faq#generics) — domain services call
// client.Do(...) for the raw bytes, then rest.DecodeJSON[T](data) to get a
// typed value. Empty data decodes to the zero value of T with no error
// (204 No Content).
func DecodeJSON[T any](data []byte) (T, error) {
	var zero T
	if len(data) == 0 {
		return zero, nil
	}
	var env envelope[T]
	if err := json.Unmarshal(data, &env); err != nil {
		return zero, fmt.Errorf("vertracloud: decode response body: %w", err)
	}
	return env.Response, nil
}
