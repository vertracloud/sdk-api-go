package vertracloud

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

const (
	testWorkspaceID = "ws_9"
	testUserID      = "user_1"
	testRoleID      = "role_1"
	testWSFolderID  = "wsfolder_1"
)

func TestWorkspaces_CreateAndAcceptExposeRealResponseTypes(t *testing.T) {
	ws := newWorkspacesService(vertratest.NewFakeRestClient(t))
	if got := reflect.TypeOf(ws.Create).Out(0).Name(); got != "WorkspaceInfo" {
		t.Fatalf("Create response = %s, want WorkspaceInfo", got)
	}
	if got := reflect.TypeOf(ws.Invites().Accept).Out(0).Name(); got != "WorkspaceInviteAccepted" {
		t.Fatalf("Accept response = %s, want WorkspaceInviteAccepted", got)
	}
	if got := reflect.TypeOf(ws.Update).In(2).Name(); got != "WorkspaceUpdateBody" {
		t.Fatalf("Update body = %s, want partial WorkspaceUpdateBody", got)
	}
}

// TestWorkspaces_RouteMatrix checks, for every one of the 30 Workspaces
// routes, that the Go method issues the expected HTTP method and path
// (same contract as TestApps_RouteMatrix / TestAccount_RouteMatrix).
// workspace_id is intentionally NOT asserted here: unlike Account,
// Workspaces routes are already scoped by :id in the path.
func TestWorkspaces_RouteMatrix(t *testing.T) {
	orgJSON := `{"scope":"workspace","user_id":"u1","workspace_id":"` + testWorkspaceID + `","folders":[],"favorites":[]}`
	folderJSON := `{"id":"` + testWSFolderID + `","name":"x","color":"blue","position":0,"resources":[],"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z"}`
	workspaceJSON := `{"id":"` + testWorkspaceID + `","name":"x","description":null,"owner_id":"u1","owner":{"display_name":"Ana"},"members_count":1,"deleted_at":null,"frozen":false,"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z"}`
	roleJSON := `{"id":"` + testRoleID + `","workspace_id":"` + testWorkspaceID + `","name":"Dev","permissions":["apps:read"],"preset":null,"position":0,"members_count":0,"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z"}`
	actionRequestJSON := testActionRequestJSON
	memberJSON := `{"user_id":"` + testUserID + `","role_id":"` + testRoleID + `","role_name":"Dev","joined_at":"2026-09-22T12:00:00.000Z","expires_at":null,"user":{"display_name":"Ana","email":"a@x.com"}}`

	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(ft *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, svc WorkspacesService) error
	}

	cases := []routeCase{
		{
			name: "List", wantMethod: http.MethodGet, wantPath: "/v1/workspaces",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.List(ctx)
				return err
			},
		},
		{
			name: "Get", wantMethod: http.MethodGet, wantPath: "/v1/workspaces/" + testWorkspaceID,
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"id":"`+testWorkspaceID+`","name":"x","description":null,"owner_id":"u1","owner":{"display_name":"Ana"},"members_count":1,"deleted_at":null,"frozen":false,"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z","members":[],"roles":[],"applications":[],"databases":[],"permissions":[],"is_owner":true,"owner_plan_id":2,"resource_organization":`+orgJSON+`}`)
			},
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Get(ctx, testWorkspaceID)
				return err
			},
		},
		{
			name: "Create", wantMethod: http.MethodPost, wantPath: "/v1/workspaces",
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"id":"`+testWorkspaceID+`","name":"x","description":null,"owner_id":"u1","owner":{"display_name":"Ana"},"members_count":1,"deleted_at":null,"frozen":false,"created_at":"2026-09-22T12:00:00Z","updated_at":"2026-09-22T12:00:00Z","members":[],"roles":[],"applications":[],"databases":[],"permissions":[],"is_owner":true,"owner_plan_id":2,"resource_organization":`+orgJSON+`}`)
			},
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Create(ctx, WorkspaceCreateBody{Name: "x"})
				return err
			},
		},
		{
			name: "Update", wantMethod: http.MethodPut, wantPath: "/v1/workspaces/" + testWorkspaceID,
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, workspaceJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				name := "y"
				_, err := svc.Update(ctx, testWorkspaceID, WorkspaceUpdateBody{Name: &name})
				return err
			},
		},
		{
			name: "Delete", wantMethod: http.MethodDelete, wantPath: "/v1/workspaces/" + testWorkspaceID,
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueRaw(204, nil) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Delete(ctx, testWorkspaceID)
			},
		},
		{
			name: "Members.List", wantMethod: http.MethodGet, wantPath: "/v1/workspaces/" + testWorkspaceID + "/members",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Members().List(ctx, testWorkspaceID)
				return err
			},
		},
		{
			name:       "Members.Update",
			wantMethod: http.MethodPut,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/members/" + testUserID,
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, memberJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Members().Update(ctx, testWorkspaceID, testUserID, WorkspaceMemberUpdateBody{RoleID: testRoleID})
				return err
			},
		},
		{
			name:       "Members.Remove",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/members/" + testUserID,
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Members().Remove(ctx, testWorkspaceID, testUserID)
			},
		},
		{
			name: "Roles.List", wantMethod: http.MethodGet, wantPath: "/v1/workspaces/" + testWorkspaceID + "/roles",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Roles().List(ctx, testWorkspaceID)
				return err
			},
		},
		{
			name: "Roles.Create", wantMethod: http.MethodPost, wantPath: "/v1/workspaces/" + testWorkspaceID + "/roles",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, roleJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Roles().Create(ctx, testWorkspaceID, WorkspaceRoleBody{Name: "Dev", Permissions: []WorkspacePermission{WorkspacePermissionAppsRead}})
				return err
			},
		},
		{
			name:       "Roles.Update",
			wantMethod: http.MethodPut,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/roles/" + testRoleID,
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, roleJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Roles().Update(ctx, testWorkspaceID, testRoleID, WorkspaceRoleBody{Name: "Dev", Permissions: []WorkspacePermission{WorkspacePermissionAppsRead}})
				return err
			},
		},
		{
			name:       "Roles.Delete",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/roles/" + testRoleID,
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Roles().Delete(ctx, testWorkspaceID, testRoleID)
			},
		},
		{
			name: "Invites.List", wantMethod: http.MethodGet, wantPath: "/v1/workspaces/" + testWorkspaceID + "/invites",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Invites().List(ctx, testWorkspaceID)
				return err
			},
		},
		{
			name: "Invites.Delete", wantMethod: http.MethodDelete, wantPath: "/v1/workspaces/" + testWorkspaceID + "/invites/invite_1",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueRaw(204, nil) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Invites().Delete(ctx, testWorkspaceID, "invite_1")
			},
		},
		{
			name: "Invites.Preview", wantMethod: http.MethodGet, wantPath: "/v1/workspaces/invites/tok_1",
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"workspace":{"id":"`+testWorkspaceID+`","name":"x"},"inviter":{"display_name":"Ana"},"role_name":"Dev","kind":"link","expires_at":"2026-09-22T12:00:00.000Z"}`)
			},
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Invites().Preview(ctx, "tok_1")
				return err
			},
		},
		{
			name: "Invites.Accept", wantMethod: http.MethodPost, wantPath: "/v1/workspaces/invites/tok_1/accept",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{"id":"`+testWorkspaceID+`","name":"x"}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Invites().Accept(ctx, "tok_1")
				return err
			},
		},
		{
			name: "Invites.Decline", wantMethod: http.MethodPost, wantPath: "/v1/workspaces/invites/tok_1/decline",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueRaw(204, nil) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Invites().Decline(ctx, "tok_1")
			},
		},
		{
			name: "ActionRequests.List", wantMethod: http.MethodGet, wantPath: "/v1/workspaces/" + testWorkspaceID + "/action-requests",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.ActionRequests().List(ctx, testWorkspaceID, nil)
				return err
			},
		},
		{
			name: "ActionRequests.Create", wantMethod: http.MethodPost, wantPath: "/v1/workspaces/" + testWorkspaceID + "/action-requests",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, actionRequestJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.ActionRequests().Create(ctx, testWorkspaceID, WorkspaceActionRequestCreateBody{Action: WorkspaceActionRequestActionAppDelete, ResourceID: "app_1"})
				return err
			},
		},
		{
			name:       "Apps.Add",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/apps/app_1",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Apps().Add(ctx, testWorkspaceID, "app_1")
			},
		},
		{
			name:       "Apps.Remove",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/apps/app_1",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Apps().Remove(ctx, testWorkspaceID, "app_1")
			},
		},
		{
			name:       "Databases.Add",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/databases/db_1",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Databases().Add(ctx, testWorkspaceID, "db_1")
			},
		},
		{
			name:       "Databases.Remove",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/databases/db_1",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Databases().Remove(ctx, testWorkspaceID, "db_1")
			},
		},
		{
			name:       "Folders.Create",
			wantMethod: http.MethodPost,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/folders",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, folderJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Folders().Create(ctx, testWorkspaceID, ResourceFolderCreateBody{Name: "x"})
				return err
			},
		},
		{
			name:       "Folders.Update",
			wantMethod: http.MethodPatch,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/folders/" + testWSFolderID,
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, folderJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				name := "y"
				_, err := svc.Folders().Update(ctx, testWorkspaceID, testWSFolderID, ResourceFolderUpdateBody{Name: &name})
				return err
			},
		},
		{
			name:       "Folders.Delete",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/folders/" + testWSFolderID,
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				return svc.Folders().Delete(ctx, testWorkspaceID, testWSFolderID)
			},
		},
		{
			name:       "Folders.AddResource",
			wantMethod: http.MethodPut,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/folders/" + testWSFolderID + "/resources/application/app_2",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Folders().AddResource(ctx, testWorkspaceID, testWSFolderID, WorkspaceResourceTypeApplication, "app_2", nil)
				return err
			},
		},
		{
			name:       "Folders.RemoveResource",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/folders/" + testWSFolderID + "/resources/database/db_2",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Folders().RemoveResource(ctx, testWorkspaceID, testWSFolderID, WorkspaceResourceTypeDatabase, "db_2")
				return err
			},
		},
		{
			name:       "Favorites.Add",
			wantMethod: http.MethodPut,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/favorites/application/app_3",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Favorites().Add(ctx, testWorkspaceID, WorkspaceResourceTypeApplication, "app_3", nil)
				return err
			},
		},
		{
			name:       "Favorites.Remove",
			wantMethod: http.MethodDelete,
			wantPath:   "/v1/workspaces/" + testWorkspaceID + "/resource-organization/favorites/database/db_3",
			enqueue:    func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, orgJSON) },
			invoke: func(ctx context.Context, svc WorkspacesService) error {
				_, err := svc.Favorites().Remove(ctx, testWorkspaceID, WorkspaceResourceTypeDatabase, "db_3")
				return err
			},
		},
	}

	if len(cases) != 30 {
		t.Fatalf("route matrix has %d cases, want 30 (every workspaces route in the API key catalog)", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			svc := newWorkspacesService(fake)

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
		})
	}
}

func TestWorkspaces_CreateAndAcceptDecodeActualResponses(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"`+testWorkspaceID+`","name":"Eng","description":null,"owner_id":"u1","owner":{"display_name":"Ana"},"members_count":1,"deleted_at":null,"frozen":false,"created_at":"2026-09-22T12:00:00Z","updated_at":"2026-09-22T12:00:00Z","members":[],"roles":[],"applications":[],"databases":[],"permissions":["apps:read"],"is_owner":true,"owner_plan_id":2,"resource_organization":{"scope":"workspace","user_id":"u1","workspace_id":"`+testWorkspaceID+`","folders":[],"favorites":[]}}`)
	fake.EnqueueJSON(200, `{"id":"`+testWorkspaceID+`","name":"Eng"}`)
	svc := newWorkspacesService(fake)
	created, err := svc.Create(context.Background(), WorkspaceCreateBody{Name: "Eng"})
	if err != nil || created.Name != "Eng" || !created.IsOwner || len(created.Permissions) != 1 || created.ResourceOrganization.WorkspaceID == nil || *created.ResourceOrganization.WorkspaceID != testWorkspaceID {
		t.Fatalf("create response = %#v, err = %v", created, err)
	}
	accepted, err := svc.Invites().Accept(context.Background(), "invite-token")
	if err != nil || accepted.ID != testWorkspaceID || accepted.Name != "Eng" {
		t.Fatalf("accept response = %#v, err = %v", accepted, err)
	}
}

func TestWorkspaces_UpdateAllowsDescriptionOnlyBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"`+testWorkspaceID+`","name":"Eng","description":"updated"}`)
	description := "updated"
	if _, err := newWorkspacesService(fake).Update(context.Background(), testWorkspaceID, WorkspaceUpdateBody{Description: &description}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["description"] != description || body["name"] != nil {
		t.Fatalf("description-only update body = %s", fake.Requests[0].Body)
	}
}

// TestWorkspaces_MembersUpdate_BodyFields checks that role_id/expires_at
// land in the body, not the query.
func TestWorkspaces_MembersUpdate_BodyFields(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"user_id":"`+testUserID+`","role_id":"`+testRoleID+`","role_name":"Dev","joined_at":"2026-09-22T12:00:00.000Z","expires_at":"2027-01-01T00:00:00Z","user":{"display_name":"Ana","email":"a@x.com"}}`)
	svc := newWorkspacesService(fake)

	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.Members().Update(context.Background(), testWorkspaceID, testUserID, WorkspaceMemberUpdateBody{RoleID: testRoleID, ExpiresAt: NullableValue(expires)})
	if err != nil {
		t.Fatalf("Members.Update: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["role_id"] != testRoleID {
		t.Errorf("body[role_id] = %v, want %s", body["role_id"], testRoleID)
	}
	if body["expires_at"] != "2027-01-01T00:00:00Z" {
		t.Errorf("body[expires_at] = %v, want 2027-01-01T00:00:00Z", body["expires_at"])
	}
}

// TestWorkspaces_FoldersAddResource_NilBodySendsEmptyObject mirrors
// TestAccount_FoldersAddResource_NilBodySendsEmptyObject for the
// workspace-scoped folder route.
func TestWorkspaces_FoldersAddResource_NilBodySendsEmptyObject(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"scope":"workspace","user_id":"u1","workspace_id":"`+testWorkspaceID+`","folders":[],"favorites":[]}`)
	svc := newWorkspacesService(fake)

	_, err := svc.Folders().AddResource(context.Background(), testWorkspaceID, testWSFolderID, WorkspaceResourceTypeApplication, "app_1", nil)
	if err != nil {
		t.Fatalf("Folders.AddResource: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("body = %v, want empty object", body)
	}
}

// TestWorkspaces_Get_DecodesFullShape sanity-checks that WorkspaceInfo
// decodes both the flat Workspace fields and the nested
// members/roles/applications/databases/permissions/resource_organization.
func TestWorkspaces_Get_DecodesFullShape(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{
		"id":"`+testWorkspaceID+`","name":"Eng","description":"time de engenharia","owner_id":"u1",
		"owner":{"display_name":"Ana"},"members_count":2,"deleted_at":null,"frozen":false,
		"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z",
		"members":[{"user_id":"`+testUserID+`","role_id":"`+testRoleID+`","role_name":"Dev","joined_at":"2026-09-22T12:00:00.000Z","expires_at":null,"user":{"display_name":"Bea","email":"b@x.com"}}],
		"roles":[{"id":"`+testRoleID+`","workspace_id":"`+testWorkspaceID+`","name":"Dev","permissions":["apps:read"],"preset":"developer","position":0,"members_count":1,"created_at":"2026-09-22T12:00:00.000Z","updated_at":"2026-09-22T12:00:00.000Z"}],
		"applications":[],"databases":[],
		"permissions":["apps:read","apps:manage"],"is_owner":true,"owner_plan_id":3,
		"resource_organization":{"scope":"workspace","user_id":"u1","workspace_id":"`+testWorkspaceID+`","folders":[],"favorites":[]}
	}`)
	svc := newWorkspacesService(fake)

	info, err := svc.Get(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if info.ID != testWorkspaceID || info.Name != "Eng" || info.Owner.DisplayName != "Ana" {
		t.Errorf("flat Workspace fields not decoded: %+v", info.Workspace)
	}
	if len(info.Members) != 1 || info.Members[0].User.DisplayName != "Bea" {
		t.Errorf("Members not decoded: %+v", info.Members)
	}
	if len(info.Roles) != 1 || info.Roles[0].Preset == nil || *info.Roles[0].Preset != WorkspaceRolePresetDeveloper {
		t.Errorf("Roles not decoded: %+v", info.Roles)
	}
	if len(info.Permissions) != 2 || !info.IsOwner {
		t.Errorf("Permissions/IsOwner not decoded: %+v", info)
	}
	if info.ResourceOrganization.Scope != WorkspaceResourceOrganizationScopeWorkspace {
		t.Errorf("ResourceOrganization not decoded: %+v", info.ResourceOrganization)
	}
}

const testActionRequestJSON = `{"id":"ar_1","workspace_id":"` + testWorkspaceID + `","action":"app_delete","resource_type":"application","resource_id":"app_1","resource_name":null,"params":{},"status":"pending","requested_by":{"id":"u2","display_name":"Bea"},"decided_by":null,"decided_at":null,"expires_at":"2026-09-23T12:00:00.000Z","created_at":"2026-09-22T12:00:00.000Z"}`

// TestWorkspaces_ActionRequests_StatusQueryAndBody checks the optional status
// filter goes to the query and the create body omits nil params.
func TestWorkspaces_ActionRequests_StatusQueryAndBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[`+testActionRequestJSON+`]`)
	fake.EnqueueJSON(200, `[]`)
	fake.EnqueueJSON(200, testActionRequestJSON)
	svc := newWorkspacesService(fake)
	ctx := context.Background()

	list, err := svc.ActionRequests().List(ctx, testWorkspaceID, &WorkspaceActionRequestListParams{Status: WorkspaceActionRequestStatusPending})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := fake.Requests[0].Query.Get("status"); got != "pending" {
		t.Errorf("status query = %q, want pending", got)
	}
	if len(list) != 1 || list[0].Status != WorkspaceActionRequestStatusPending || list[0].DecidedBy != nil || list[0].ExpiresAt.IsZero() {
		t.Errorf("action request not decoded: %+v", list)
	}

	if _, err := svc.ActionRequests().List(ctx, testWorkspaceID, &WorkspaceActionRequestListParams{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if fake.Requests[1].Query.Has("status") {
		t.Errorf("empty status must be omitted, got %v", fake.Requests[1].Query)
	}

	if _, err := svc.ActionRequests().Create(ctx, testWorkspaceID, WorkspaceActionRequestCreateBody{Action: WorkspaceActionRequestActionAppDelete, ResourceID: "app_1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(fake.Requests[2].Body, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["action"] != "app_delete" || body["resource_id"] != "app_1" {
		t.Errorf("body = %v", body)
	}
	if got, ok := body["params"].(map[string]any); !ok || len(got) != 0 {
		t.Errorf("empty action params must be {}, body = %v", body)
	}
}

func TestWorkspaces_ActionRequestSnapshotRestoreParamsAreTypedByAction(t *testing.T) {
	const response = `{"id":"ar_restore","workspace_id":"` + testWorkspaceID + `","action":"snapshot_restore","resource_type":"application","resource_id":"app_1","resource_name":"demo","params":{"snapshot_id":"snap_1"},"status":"pending","requested_by":{"id":"user_1","display_name":"Ana"},"decided_by":null,"decided_at":null,"expires_at":"2026-09-24T00:00:00Z","created_at":"2026-09-23T00:00:00Z"}`
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[`+response+`]`)
	fake.EnqueueJSON(201, response)
	svc := newWorkspacesService(fake)
	items, err := svc.ActionRequests().List(context.Background(), testWorkspaceID, nil)
	if err != nil {
		t.Fatal(err)
	}
	params, ok := items[0].Params.(WorkspaceActionRequestSnapshotRestoreParams)
	if !ok || params.SnapshotID != "snap_1" {
		t.Fatalf("decoded params = %#v", items[0].Params)
	}
	_, err = svc.ActionRequests().Create(context.Background(), testWorkspaceID, WorkspaceActionRequestCreateBody{
		Action: WorkspaceActionRequestActionSnapshotRestore, ResourceID: "app_1",
		Params: WorkspaceActionRequestSnapshotRestoreParams{SnapshotID: "snap_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(fake.Requests[1].Body, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["params"].(map[string]any)["snapshot_id"]; got != "snap_1" {
		t.Fatalf("create params = %v", body["params"])
	}
}
