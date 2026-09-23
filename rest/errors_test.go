package rest

import (
	"testing"
	"time"
)

func TestAPIError_UntilFromTypedDetails(t *testing.T) {
	err := apiErrorFromHTTP(423, []byte(`{"code":"APP_SHIELD_COOLDOWN","details":{"until":"2026-09-23T12:00:00Z"}}`), "")
	got := err.Until()
	want := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Fatalf("Until() = %v, want %v", got, want)
	}
}

func TestAPIError_ValidationPath(t *testing.T) {
	err := apiErrorFromHTTP(422, []byte(`{"code":"VALIDATION_ERROR","message":"invalid","details":{"path":"body.name"}}`), "")
	if got := err.ValidationPath(); got != "body.name" {
		t.Fatalf("ValidationPath() = %q, want body.name", got)
	}
}
