package rest

import (
	"net/http"
	"testing"
)

func redirectReq(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Proxy-Authorization", "Basic secret")
	req.Header.Set("Cookie", "s=secret")
	req.Header.Set("Accept", "application/json")
	return req
}

func TestCheckRedirectOrigins(t *testing.T) {
	tests := []struct {
		name     string
		original string
		hops     []string // URLs after the original, the last one is the pending request
		wantKeep bool
	}{
		{"same origin", "https://api.example.com/a", []string{"https://api.example.com/b"}, true},
		{"default port explicit", "https://api.example.com/a", []string{"https://api.example.com:443/b"}, true},
		{"host case", "https://API.example.com/a", []string{"https://api.example.com/b"}, true},
		{"https to http", "https://api.example.com/a", []string{"http://api.example.com/b"}, false},
		{"http to https", "http://api.example.com/a", []string{"https://api.example.com/b"}, false},
		{"other port", "https://api.example.com/a", []string{"https://api.example.com:8443/b"}, false},
		{"other host", "https://api.example.com/a", []string{"https://other.example.com/b"}, false},
		{"leave then return", "https://api.example.com/a", []string{"https://other.example.com/b", "https://api.example.com/c"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			via := []*http.Request{redirectReq(t, tc.original)}
			for _, u := range tc.hops[:len(tc.hops)-1] {
				via = append(via, redirectReq(t, u))
			}
			req := redirectReq(t, tc.hops[len(tc.hops)-1])
			if err := stripCredentialsOnRedirect(req, via); err != nil {
				t.Fatal(err)
			}
			for _, h := range []string{"Authorization", "Proxy-Authorization", "Cookie"} {
				if has := req.Header.Get(h) != ""; has != tc.wantKeep {
					t.Errorf("%s kept=%v, want %v", h, has, tc.wantKeep)
				}
			}
			if req.Header.Get("Accept") == "" {
				t.Error("non-credential headers must survive")
			}
		})
	}
}

func TestCheckRedirectLimit(t *testing.T) {
	via := make([]*http.Request, maxRedirects)
	for i := range via {
		via[i] = redirectReq(t, "https://api.example.com/a")
	}
	if err := stripCredentialsOnRedirect(redirectReq(t, "https://api.example.com/b"), via); err == nil {
		t.Fatal("expected error at the redirect limit")
	}
}
