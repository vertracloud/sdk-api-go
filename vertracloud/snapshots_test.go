package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

const (
	testResourceID = "app_123"
	testSnapshotID = "snap_456"
)

// TestSnapshots_RouteMatrix checks, for every Snapshots route except
// Download (binary, covered separately below), that the Go method issues
// the expected HTTP method and path, and that scope always lands as the
// required "scope" query parameter.
func TestSnapshots_RouteMatrix(t *testing.T) {
	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(fake *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, svc SnapshotsService) error
	}

	cases := []routeCase{
		{
			name: "ListAll", wantMethod: http.MethodGet, wantPath: "/v1/users/snapshots",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc SnapshotsService) error {
				_, err := svc.ListAll(ctx, SnapshotScopeApplications)
				return err
			},
		},
		{
			name: "List", wantMethod: http.MethodGet, wantPath: "/v1/users/" + testResourceID + "/snapshots",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc SnapshotsService) error {
				_, err := svc.List(ctx, testResourceID, SnapshotScopeApplications)
				return err
			},
		},
		{
			name: "Create", wantMethod: http.MethodPost, wantPath: "/v1/users/" + testResourceID + "/snapshots",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"id":"1","resource_id":"`+testResourceID+`","author_id":null,"resource_type":1,"size":"1MB","date":"2026-01-01T00:00:00Z","resource_name":"myapp"}`)
			},
			invoke: func(ctx context.Context, svc SnapshotsService) error {
				_, err := svc.Create(ctx, testResourceID, SnapshotScopeApplications)
				return err
			},
		},
		{
			name: "Restore", wantMethod: http.MethodPost, wantPath: "/v1/users/" + testResourceID + "/snapshots/" + testSnapshotID + "/restore",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"message":"restoring"}`) },
			invoke: func(ctx context.Context, svc SnapshotsService) error {
				_, err := svc.Restore(ctx, testResourceID, testSnapshotID, SnapshotScopeDatabases)
				return err
			},
		},
	}

	if len(cases) != 4 {
		t.Fatalf("expected 4 table cases (5 routes minus Download), got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			svc := newSnapshotsService(fake)

			if err := tc.invoke(context.Background(), svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fake.Requests) != 1 {
				t.Fatalf("want 1 request, got %d", len(fake.Requests))
			}

			req := fake.Requests[0]
			if req.Method != tc.wantMethod {
				t.Errorf("method = %s, want %s", req.Method, tc.wantMethod)
			}
			if req.Path != tc.wantPath {
				t.Errorf("path = %s, want %s", req.Path, tc.wantPath)
			}
			if got := req.Query.Get("scope"); got == "" {
				t.Errorf("scope query missing, want it present on every snapshots route")
			}
		})
	}
}

// TestSnapshots_Scope_IsExplicit checks that the exact scope value passed by
// the caller reaches the query string unchanged, for both allowed values.
func TestSnapshots_Scope_IsExplicit(t *testing.T) {
	for _, scope := range []SnapshotScope{SnapshotScopeApplications, SnapshotScopeDatabases} {
		t.Run(string(scope), func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, `[]`)
			svc := newSnapshotsService(fake)

			if _, err := svc.List(context.Background(), testResourceID, scope); err != nil {
				t.Fatalf("List: %v", err)
			}
			if got := fake.Requests[0].Query.Get("scope"); got != string(scope) {
				t.Errorf("scope query = %q, want %q", got, string(scope))
			}
		})
	}
}

// TestSnapshots_Download_Binary checks that Download returns the exact raw
// bytes of the response body with no {"response": ...} envelope parsing,
// and that scope still lands in the query string.
func TestSnapshots_Download_Binary(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	raw := []byte("PK\x03\x04-this-is-not-json-{\"response\":true}")
	fake.EnqueueRaw(200, raw)
	svc := newSnapshotsService(fake)

	body, err := svc.Download(context.Background(), testResourceID, testSnapshotID, SnapshotScopeDatabases)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if !bytes.Equal(got, raw) {
		t.Errorf("Download bytes = %q, want %q", got, raw)
	}

	req := fake.Requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	wantPath := "/v1/users/" + testResourceID + "/snapshots/" + testSnapshotID + "/download"
	if req.Path != wantPath {
		t.Errorf("path = %s, want %s", req.Path, wantPath)
	}
	if got := req.Query.Get("scope"); got != string(SnapshotScopeDatabases) {
		t.Errorf("scope query = %q, want %q", got, SnapshotScopeDatabases)
	}
}

// TestSnapshots_NoWorkspaceQuery checks that this domain does not thread a
// workspace id anywhere — snapshots are identified by resource id alone. No
// opt is passed, so the resolved WorkspaceID must be empty.
func TestSnapshots_NoWorkspaceQuery(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[]`)
	svc := newSnapshotsService(fake)

	if _, err := svc.List(context.Background(), testResourceID, SnapshotScopeApplications); err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := fake.Requests[0].WorkspaceID; got != "" {
		t.Errorf("workspace id = %q, want empty", got)
	}
}

func TestSnapshots_RestoreTo_SendsTargetResourceBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"message":"restoring"}`)
	svc := newSnapshotsService(fake)

	if _, err := svc.RestoreTo(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications, "app_target"); err != nil {
		t.Fatalf("RestoreTo: %v", err)
	}
	req := fake.Requests[0]
	if req.ContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", req.ContentType)
	}
	var body map[string]string
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["resource_id"] != "app_target" {
		t.Fatalf("resource_id = %q, want app_target", body["resource_id"])
	}
}
