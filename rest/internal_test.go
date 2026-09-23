package rest

// This file is the only rest test file in package rest (internal test
// package) rather than rest_test (external). It exists solely for the two
// checks below that need the unexported *client struct fields; every other
// test in this package lives in the external rest_test package instead, so
// it can import internal without an import cycle — vertratest's
// FakeRestClient implements rest.Client, so vertratest necessarily imports
// this package, and an internal test file importing vertratest back would
// be a cycle.

import "testing"

func TestNewClient_Defaults(t *testing.T) {
	c, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	impl, ok := c.(*client)
	if !ok {
		t.Fatalf("NewClient returned %T, want *client", c)
	}
	if impl.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", impl.baseURL, defaultBaseURL)
	}
	if impl.timeout != defaultTimeout {
		t.Errorf("timeout = %v, want %v", impl.timeout, defaultTimeout)
	}
	if impl.userAgent != "vertracloud-sdk-go/"+sdkVersion {
		t.Errorf("userAgent = %q", impl.userAgent)
	}
}

func TestDecodeJSONAcceptsNullEnvelope(t *testing.T) {
	got, err := DecodeJSON[string]([]byte(`{"response":null}`))
	if err != nil || got != "" {
		t.Fatalf("DecodeJSON(null) = %q, err = %v", got, err)
	}
}

func TestWithBaseURL_TrimsTrailingSlash(t *testing.T) {
	c, err := NewClient("test-key", WithBaseURL("https://example.test/"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if impl := c.(*client); impl.baseURL != "https://example.test" {
		t.Errorf("baseURL = %q", impl.baseURL)
	}
}
