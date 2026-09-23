package vertracloud

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

func TestNewMultipartBody_SendsFileAndFieldsOverHTTP(t *testing.T) {
	ft := vertratest.NewFakeRoundTripper(t)
	ft.EnqueueJSON(http.StatusOK, `{}`)
	rc, err := rest.NewClient("test-key", rest.WithHTTPClient(&http.Client{Transport: ft}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	body, contentType := newMultipartBody([]multipartField{
		{Name: "name", Value: "demo"},
		{Name: "file", FileName: "app.zip", Reader: strings.NewReader("zip-bytes")},
	})
	if _, err := rc.Do(context.Background(), http.MethodPost, "/v1/apps", nil, body, contentType); err != nil {
		t.Fatalf("Do: %v", err)
	}

	req := ft.Requests[0]
	if ct := req.Headers.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Errorf("Content-Type = %q", ct)
	}
	got := string(req.Body)
	for _, want := range []string{`name="name"`, "demo", "filename=app.zip", "zip-bytes", "Content-Type: application/zip"} {
		if !strings.Contains(got, want) {
			t.Errorf("body missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "filename=") != 1 {
		t.Errorf("only the file field should carry a filename:\n%s", got)
	}
}

func TestAddQueryParam_OmitsEmpty(t *testing.T) {
	q := url.Values{}
	addQueryParam(q, "path", "")
	if q.Has("path") {
		t.Error("expected empty value to be omitted")
	}
	addQueryParam(q, "path", "/src")
	if q.Get("path") != "/src" {
		t.Errorf("path = %q", q.Get("path"))
	}
}

func TestAddQueryBool_OmitsNilRendersTrueFalse(t *testing.T) {
	q := url.Values{}
	addQueryBool(q, "restart", nil)
	if q.Has("restart") {
		t.Error("expected nil bool to be omitted")
	}
	yes, no := true, false
	addQueryBool(q, "restart", &yes)
	addQueryBool(q, "force_build", &no)
	if q.Get("restart") != "true" || q.Get("force_build") != "false" {
		t.Errorf("query = %s", q.Encode())
	}
}
