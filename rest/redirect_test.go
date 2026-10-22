package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/vertracloud/sdk-api-go/rest"
)

// authLog records the Authorization header of every request a server sees.
type authLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *authLog) add(v string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, v)
}

func (l *authLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

const redirectToken = "vc_secret_token"

func redirectClient(t *testing.T, base string, opts ...rest.ClientOpt) rest.Client {
	t.Helper()
	c, err := rest.NewClient(redirectToken, append([]rest.ClientOpt{rest.WithBaseURL(base)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRedirectSameOriginKeepsAuthorization(t *testing.T) {
	var log authLog
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.Header.Get("Authorization"))
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		log.add(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := redirectClient(t, srv.URL).Do(context.Background(), "GET", "/start", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	got := log.get()
	if len(got) != 2 || got[0] != "Bearer "+redirectToken || got[1] != "Bearer "+redirectToken {
		t.Fatalf("same-origin redirect must keep the token on both hops, got %v", got)
	}
}

func TestRedirectToOtherOriginDropsAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		target func(t *testing.T) (url string, log *authLog, closeFn func())
	}{
		{"other port", func(t *testing.T) (string, *authLog, func()) {
			var l authLog
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				l.add(r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(`{}`))
			}))
			return s.URL, &l, s.Close
		}},
		{"other host", func(t *testing.T) (string, *authLog, func()) {
			var l authLog
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				l.add(r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(`{}`))
			}))
			// same listener, but addressed by a different hostname
			return strings.Replace(s.URL, "127.0.0.1", "localhost", 1), &l, s.Close
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			targetURL, targetLog, closeTarget := tc.target(t)
			defer closeTarget()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, targetURL+"/final", http.StatusFound)
			}))
			defer origin.Close()

			if _, err := redirectClient(t, origin.URL).Do(context.Background(), "GET", "/start", nil, nil, ""); err != nil {
				t.Fatal(err)
			}
			got := targetLog.get()
			if len(got) != 1 || got[0] != "" {
				t.Fatalf("token leaked to %s: %v", tc.name, got)
			}
		})
	}
}

func TestRedirectChainBackToOriginDoesNotResendToken(t *testing.T) {
	var logA authLog
	var bURL string
	var aURL string
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logA.add(r.URL.Path + "|" + r.Header.Get("Authorization"))
		if r.URL.Path == "/start" {
			http.Redirect(w, r, bURL+"/hop", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer a.Close()
	aURL = a.URL
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, aURL+"/back", http.StatusFound)
	}))
	defer b.Close()
	bURL = b.URL

	if _, err := redirectClient(t, a.URL).Do(context.Background(), "GET", "/start", nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	got := logA.get()
	if len(got) != 2 || got[0] != "/start|Bearer "+redirectToken || got[1] != "/back|" {
		t.Fatalf("token must not return after leaving the origin, got %v", got)
	}
}

func TestRedirectStreamDropsAuthorizationOnOtherPort(t *testing.T) {
	var log authLog
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("data: x\n\n"))
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer origin.Close()

	body, err := redirectClient(t, origin.URL).DoStream(context.Background(), "GET", "/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if got := log.get(); len(got) != 1 || got[0] != "" {
		t.Fatalf("token leaked on stream redirect: %v", got)
	}
}

func TestRedirectLimitStillTenHops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := redirectClient(t, srv.URL).Do(context.Background(), "GET", "/loop", nil, nil, ""); err == nil {
		t.Fatal("expected an error after too many redirects")
	}
}
