package vertracloud

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const testDBID = "db_123"

// TestDatabases_RouteMatrix checks, for every Databases route except Create
// (covered separately below, since it sends workspace_id in the body
// instead of the query), that the Go method issues the expected HTTP
// method and path, and that rest.WithWorkspaceID is forwarded correctly.
func TestDatabases_RouteMatrix(t *testing.T) {
	opts := []rest.RequestOpt{rest.WithWorkspaceID("ws_1")}

	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(fake *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, svc DatabasesService) error
	}

	cases := []routeCase{
		{
			name: "StatusAll", wantMethod: http.MethodGet, wantPath: "/v1/databases/status",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `[{"id":"1","cpu":"1%","ram":"1%","storage":"1%","running":true}]`)
			},
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.StatusAll(ctx, opts...)
				return err
			},
		},
		{
			name: "Get", wantMethod: http.MethodGet, wantPath: "/v1/databases/" + testDBID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"id":"1","name":"x"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Get(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Status", wantMethod: http.MethodGet, wantPath: "/v1/databases/" + testDBID + "/status",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"id":"1","cpu":"1%","ram":"1%","status":"up","running":true,"storage":"1%","network":{"total":"0","now":"0"},"uptime":1}`)
			},
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Status(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Metrics", wantMethod: http.MethodGet, wantPath: "/v1/databases/" + testDBID + "/metrics",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Metrics(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Update", wantMethod: http.MethodPut, wantPath: "/v1/databases/" + testDBID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"success"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				name := "new"
				_, err := svc.Update(ctx, testDBID, DatabaseUpdateBody{Name: &name}, opts...)
				return err
			},
		},
		{
			name: "Start", wantMethod: http.MethodPost, wantPath: "/v1/databases/" + testDBID + "/start",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"ok"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Start(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Stop", wantMethod: http.MethodPost, wantPath: "/v1/databases/" + testDBID + "/stop",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"ok"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Stop(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Reset", wantMethod: http.MethodPost, wantPath: "/v1/databases/" + testDBID + "/reset",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"success"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Reset(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Delete", wantMethod: http.MethodDelete, wantPath: "/v1/databases/" + testDBID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `"`+testDBID+`"`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Delete(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Credentials.Certificate.Get", wantMethod: http.MethodGet, wantPath: "/v1/databases/" + testDBID + "/credentials/certificate",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"crt":"c","key":"k","pem":"p"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Credentials().Certificate().Get(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Credentials.Certificate.Reset", wantMethod: http.MethodPost, wantPath: "/v1/databases/" + testDBID + "/credentials/certificate/reset",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"crt":"new-crt","key":"new-key","pem":"new-pem"}`)
			},
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Credentials().Certificate().Reset(ctx, testDBID, opts...)
				return err
			},
		},
		{
			name: "Credentials.Password.Reset", wantMethod: http.MethodPost, wantPath: "/v1/databases/" + testDBID + "/credentials/reset",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"password":"newpass"}`) },
			invoke: func(ctx context.Context, svc DatabasesService) error {
				_, err := svc.Credentials().Password().Reset(ctx, testDBID, opts...)
				return err
			},
		},
	}

	if len(cases) != 12 {
		t.Fatalf("expected 12 table cases (13 routes minus Create), got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			svc := newDatabasesService(fake)

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
			if req.WorkspaceID != "ws_1" {
				t.Errorf("workspace id = %q, want ws_1", req.WorkspaceID)
			}
		})
	}
}

func TestDatabases_Update_DecodesStatusResponse(t *testing.T) {
	if got := reflect.TypeOf(DatabaseOperationResponse{}.Status).Name(); got != "ResourceOperationStatus" {
		t.Fatalf("database operation status = %s, want ResourceOperationStatus", got)
	}
	if got := reflect.TypeOf(Database{}.OwnerPlanID).Name(); got != "UserPlan" {
		t.Fatalf("database owner_plan_id type = %s, want UserPlan", got)
	}
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"status":"success"}`)
	name := "new"
	got, err := newDatabasesService(fake).Update(context.Background(), testDBID, DatabaseUpdateBody{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"status":"success"}` {
		t.Fatalf("response = %s, want status response", encoded)
	}
}

func TestDatabases_ResetDeleteAndCertificateReset_ReturnResponses(t *testing.T) {
	t.Run("reset status", func(t *testing.T) {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueJSON(200, `{"status":"success"}`)
		got, err := newDatabasesService(fake).Reset(context.Background(), testDBID)
		if err != nil || got.Status != "success" {
			t.Fatalf("response = %#v, err = %v", got, err)
		}
	})
	t.Run("delete id", func(t *testing.T) {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueJSON(200, `"`+testDBID+`"`)
		got, err := newDatabasesService(fake).Delete(context.Background(), testDBID)
		if err != nil || got != testDBID {
			t.Fatalf("response = %q, err = %v", got, err)
		}
	})
	t.Run("certificate", func(t *testing.T) {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueJSON(200, `{"crt":"new-crt","key":"new-key","pem":"new-pem"}`)
		got, err := newDatabasesService(fake).Credentials().Certificate().Reset(context.Background(), testDBID)
		if err != nil || got.Crt != "new-crt" || got.Key != "new-key" || got.Pem != "new-pem" {
			t.Fatalf("response = %#v, err = %v", got, err)
		}
	})
}

func TestDatabases_StatusAllDecodesMultipleAndEmptyItems(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want int
	}{
		{name: "multiple", json: `[{"id":"1"},{"id":"2"}]`, want: 2},
		{name: "empty", json: `[]`, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, tc.json)
			got, err := newDatabasesService(fake).StatusAll(context.Background())
			if err != nil {
				t.Fatalf("StatusAll() error = %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("StatusAll() returned %d items, want %d", len(got), tc.want)
			}
		})
	}
}

// TestDatabases_Create_WorkspaceInBody checks that Create sends
// workspace_id (and every other field) as a JSON body field — unlike every other
// Databases route, where workspace_id travels through rest.WithWorkspaceID.
func TestDatabases_Create_WorkspaceInBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"mydb"}`)
	svc := newDatabasesService(fake)

	dbType := DatabaseTypePostgreSQL
	desc := NullableValue("prod db")
	_, err := svc.Create(context.Background(), DatabaseCreateBody{
		Name:        "mydb",
		Description: desc,
		Type:        &dbType,
		RAM:         512,
		WorkspaceID: "ws_42",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(fake.Requests))
	}

	req := fake.Requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.Path != "/v1/databases" {
		t.Errorf("path = %s, want /v1/databases", req.Path)
	}
	if req.WorkspaceID != "" {
		t.Errorf("resolved opts workspace id = %q, want empty (it belongs in the body, no opt was passed)", req.WorkspaceID)
	}

	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body["workspace_id"] != "ws_42" {
		t.Errorf("body workspace_id = %v, want ws_42", body["workspace_id"])
	}
	if body["name"] != "mydb" {
		t.Errorf("body name = %v, want mydb", body["name"])
	}
	if body["ram"] != float64(512) {
		t.Errorf("body ram = %v, want 512", body["ram"])
	}
	if body["type"] != float64(1) {
		t.Errorf("body type = %v, want 1", body["type"])
	}
}

// TestDatabases_Create_SnapshotID_NoWorkspace checks that omitted optional
// fields (WorkspaceID, Type, Description) are absent from the body rather
// than sent as zero values.
func TestDatabases_Create_SnapshotID_NoWorkspace(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"mydb"}`)
	svc := newDatabasesService(fake)

	_, err := svc.Create(context.Background(), DatabaseCreateBody{
		Name:       "mydb",
		RAM:        256,
		SnapshotID: "snap_1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if body["snapshot_id"] != "snap_1" {
		t.Errorf("body snapshot_id = %v, want snap_1", body["snapshot_id"])
	}
	for _, absent := range []string{"workspace_id", "type", "description"} {
		if _, ok := body[absent]; ok {
			t.Errorf("body has %q = %v, want absent", absent, body[absent])
		}
	}
}
