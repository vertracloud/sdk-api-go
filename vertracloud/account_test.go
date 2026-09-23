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

const testFolderID = "folder_123"

// TestAccount_RouteMatrix checks, for every one of the 10 Account routes,
// that the Go method issues the expected HTTP method and path, and that
// rest.WithWorkspaceID is forwarded as the workspace_id query parameter
// (same contract as every other domain — see TestApps_RouteMatrix).
func TestAccount_RouteMatrix(t *testing.T) {
	opt := rest.WithWorkspaceID("ws_1")
	orgJSON := `{"scope":"personal","user_id":"u1","workspace_id":null,"folders":[],"favorites":[]}`
	folderJSON := `{"id":"` + testFolderID + `","name":"x","color":"blue","position":0,"resources":[],"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z"}`

	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(ft *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, c *Client) error
	}

	cases := []routeCase{
		{
			name: "Get", wantMethod: http.MethodGet, wantPath: "/v1/users/me",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{"id":"u1","name":"x"}`) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Get(ctx, opt)
				return err
			},
		},
		{
			name: "Update", wantMethod: http.MethodPatch, wantPath: "/v1/users/me",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{"id":"u1","name":"y"}`) },
			invoke: func(ctx context.Context, c *Client) error {
				name := "y"
				_, err := c.Account.Update(ctx, AccountUpdateBody{Name: &name}, opt)
				return err
			},
		},
		{
			name: "Sessions.List", wantMethod: http.MethodGet, wantPath: "/v1/users/me/sessions",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Sessions().List(ctx, opt)
				return err
			},
		},
		{
			name: "Folders.Create", wantMethod: http.MethodPost, wantPath: "/v1/users/me/resource-organization/folders",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, folderJSON) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Folders().Create(ctx, ResourceFolderCreateBody{Name: "x"}, opt)
				return err
			},
		},
		{
			name: "Folders.Update", wantMethod: http.MethodPatch, wantPath: "/v1/users/me/resource-organization/folders/" + testFolderID,
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, folderJSON) },
			invoke: func(ctx context.Context, c *Client) error {
				name := "y"
				_, err := c.Account.Folders().Update(ctx, testFolderID, ResourceFolderUpdateBody{Name: &name}, opt)
				return err
			},
		},
		{
			name: "Folders.Delete", wantMethod: http.MethodDelete, wantPath: "/v1/users/me/resource-organization/folders/" + testFolderID,
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, c *Client) error {
				return c.Account.Folders().Delete(ctx, testFolderID, opt)
			},
		},
		{
			name:       "Folders.AddResource",
			wantMethod: http.MethodPut,
			wantPath:   "/v1/users/me/resource-organization/folders/" + testFolderID + "/resources/application/app_1",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Folders().AddResource(ctx, testFolderID, WorkspaceResourceTypeApplication, "app_1", nil, opt)
				return err
			},
		},
		{
			name:       "Folders.RemoveResource",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/users/me/resource-organization/folders/" + testFolderID + "/resources/database/db_1",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Folders().RemoveResource(ctx, testFolderID, WorkspaceResourceTypeDatabase, "db_1", opt)
				return err
			},
		},
		{
			name:       "Favorites.Add",
			wantMethod: http.MethodPut,
			wantPath:   "/v1/users/me/resource-organization/favorites/application/app_2",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Favorites().Add(ctx, WorkspaceResourceTypeApplication, "app_2", nil, opt)
				return err
			},
		},
		{
			name:       "Favorites.Remove",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/users/me/resource-organization/favorites/database/db_2",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Account.Favorites().Remove(ctx, WorkspaceResourceTypeDatabase, "db_2", opt)
				return err
			},
		},
	}

	if len(cases) != 10 {
		t.Fatalf("route matrix has %d cases, want 10 (ROUTE_MATRIX.md Account section)", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			c := &Client{Account: newAccountService(fake)}

			if err := tc.invoke(context.Background(), c); err != nil {
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
				t.Errorf("workspace_id = %q, want ws_1", req.WorkspaceID)
			}
		})
	}
}

func TestAccount_Update_ReturnsAPIUserShape(t *testing.T) {
	if got := reflect.TypeOf(APIUser{}.Language).Name(); got != "UserLanguage" {
		t.Fatalf("APIUser.Language type = %s, want UserLanguage", got)
	}
	if got := reflect.TypeOf(AccountPlan{}.ID).Name(); got != "UserPlan" {
		t.Fatalf("AccountPlan.ID type = %s, want UserPlan", got)
	}
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"u1","name":"Ana","email":"ana@example.com","plan_id":2,"language":"pt-br","workspace_invites_enabled":true,"created_at":"2026-09-22T12:00:00Z","updated_at":"2026-09-23T12:00:00Z"}`)
	name := "Ana"
	got, err := newAccountService(fake).Update(context.Background(), AccountUpdateBody{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.TypeOf(got).Name() != "APIUser" || got.ID != "u1" || got.PlanID != 2 || got.Email != "ana@example.com" || got.Name != "Ana" {
		t.Fatalf("updated user = %#v", got)
	}
}

// TestAccount_FoldersAddResource_NilBodySendsEmptyObject covers the
// optional-position contract: a nil body must be sent as {} (not omitted,
// not null).
func TestAccount_FoldersAddResource_NilBodySendsEmptyObject(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"scope":"personal","user_id":"u1","workspace_id":null,"folders":[],"favorites":[]}`)
	svc := newAccountService(fake)

	_, err := svc.Folders().AddResource(context.Background(), testFolderID, WorkspaceResourceTypeApplication, "app_1", nil)
	if err != nil {
		t.Fatalf("AddResource: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %v, want empty object", body)
	}
}

// TestAccount_FavoritesAdd_PositionForwarded covers the optional-position
// contract in the other direction: a non-nil Position must reach the wire.
func TestAccount_FavoritesAdd_PositionForwarded(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"scope":"personal","user_id":"u1","workspace_id":null,"folders":[],"favorites":[]}`)
	svc := newAccountService(fake)

	pos := 3
	_, err := svc.Favorites().Add(context.Background(), WorkspaceResourceTypeApplication, "app_1", &ResourcePositionBody{Position: &pos})
	if err != nil {
		t.Fatalf("Favorites.Add: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got, ok := body["position"].(float64); !ok || got != 3 {
		t.Errorf("body[position] = %v, want 3", body["position"])
	}
}

// TestAccount_Get_DecodesFullShape sanity-checks that AccountInfo decodes
// the nested plan/applications/databases/connections/resource_organization
// shape, not just the flat Account fields.
func TestAccount_Get_DecodesFullShape(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{
		"id":"u1","name":"Ana","email":"a@x.com","plan_id":2,"language":"pt-br",
		"workspace_invites_enabled":true,
		"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z",
		"plan":{"id":2,"name":"Pro","expires_at":null,"duration":30,"memory":{"limit":2048,"used":512}},
		"applications":[],"databases":[],"connections":[{"provider":"github","username":"ana","created_at":"2026-09-22T12:00:00.000Z"}],
		"resource_organization":{"scope":"personal","user_id":"u1","workspace_id":null,"folders":[],"favorites":[]}
	}`)
	svc := newAccountService(fake)

	info, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.ID != "u1" || info.Name != "Ana" {
		t.Errorf("flat APIUser fields not decoded: %+v", info.APIUser)
	}
	if info.Plan.Name != "Pro" || info.Plan.Memory.Limit != 2048 {
		t.Errorf("Plan not decoded: %+v", info.Plan)
	}
	if len(info.Connections) != 1 || info.Connections[0].Provider != "github" {
		t.Errorf("Connections not decoded: %+v", info.Connections)
	}
	if info.ResourceOrganization.Scope != WorkspaceResourceOrganizationScopePersonal {
		t.Errorf("ResourceOrganization not decoded: %+v", info.ResourceOrganization)
	}
}
