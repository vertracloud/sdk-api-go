package vertracloud

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const stackJSON = `{"id":"stk_1","name":"loja","description":null,"network_mode":"public","owner_id":"u1","workspace_id":null,"members":[{"resource_type":"application","resource_id":"app_1","name":"api","status":"up","ram":256,"service_key":"app","database_type":null,"closed":false,"blob":null},{"resource_type":"database","resource_id":"db_1","name":"pg","status":"up","ram":256,"service_key":null,"database_type":1,"closed":false,"blob":null},{"resource_type":"blob","resource_id":"blb_1","name":"files","status":null,"ram":0,"service_key":null,"database_type":null,"closed":false,"blob":{"prefix":"loja/","visibility":"private","used_bytes":null,"api_key_id":"key_1"}}],"status":{"up":2,"down":0,"building":0,"total":2},"ram_total":512,"bindings":[{"id":"bnd_1","stack_id":"stk_1","app_id":"app_1","database_id":"db_1","blob_id":null,"target_app_id":null,"env_map":{"DATABASE_URL":"url","PGSSLROOTCERT":"ssl.ca_path"},"certs_path":"/app/.vertra/certs","created_at":"2026-10-27T10:00:00Z"}],"template":{"template_id":"tpl_1","slug":"loja","name":{"pt-br":"Loja"},"origin":"vertra","version_id":"v1","version":"1.0.0","closed":false,"detached":false,"update_available":true,"latest_version":"1.1.0","removed":false,"via_share_link":false},"update_available":true,"created_at":"2026-10-27T10:00:00Z","updated_at":"2026-10-27T10:00:00Z"}`

func stackWithRestart() string {
	return strings.TrimSuffix(stackJSON, "}") + `,"restart_required":true,"apps_to_restart":["app_1"]}`
}

func TestStacks_RouteMatrix(t *testing.T) {
	opts := []rest.RequestOpt{rest.WithWorkspaceID("ws_1")}
	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(f *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, s StacksService) error
	}
	stack := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, stackJSON) }
	change := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, stackWithRestart()) }
	obj := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{}`) }
	list := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `[]`) }
	cases := []routeCase{
		{"List", http.MethodGet, "/v1/stacks", list, func(ctx context.Context, s StacksService) error { _, err := s.List(ctx, opts...); return err }},
		{"Get", http.MethodGet, "/v1/stacks/stk_1", stack, func(ctx context.Context, s StacksService) error { _, err := s.Get(ctx, "stk_1", opts...); return err }},
		{"Create", http.MethodPost, "/v1/stacks", change, func(ctx context.Context, s StacksService) error {
			_, err := s.Create(ctx, StackCreateBody{Name: "a"}, opts...)
			return err
		}},
		{"Transform", http.MethodPost, "/v1/stacks/transform", change, func(ctx context.Context, s StacksService) error {
			_, err := s.Transform(ctx, StackTransformBody{FolderID: "f"}, opts...)
			return err
		}},
		{"Update", http.MethodPatch, "/v1/stacks/stk_1", stack, func(ctx context.Context, s StacksService) error {
			n := "b"
			_, err := s.Update(ctx, "stk_1", StackUpdateBody{Name: &n}, opts...)
			return err
		}},
		{"Delete", http.MethodDelete, "/v1/stacks/stk_1", func(f *vertratest.FakeRestClient) {
			f.EnqueueJSON(200, `{"deleted_apps":[],"deleted_databases":[],"deleted_blob":false}`)
		}, func(ctx context.Context, s StacksService) error {
			_, err := s.Delete(ctx, "stk_1", StackDeleteBody{Mode: StackDeleteModeDissolve, ConfirmName: "loja"}, opts...)
			return err
		}},
		{"Members.Add", http.MethodPost, "/v1/stacks/stk_1/members", change, func(ctx context.Context, s StacksService) error {
			_, err := s.Members().Add(ctx, "stk_1", StackMemberAddBody{ResourceType: StackMemberTypeDatabase, ResourceID: "db_1"}, opts...)
			return err
		}},
		{"Members.Remove", http.MethodDelete, "/v1/stacks/stk_1/members/database/db_1", change, func(ctx context.Context, s StacksService) error {
			_, err := s.Members().Remove(ctx, "stk_1", StackMemberTypeDatabase, "db_1", nil, opts...)
			return err
		}},
		{"Bindings.List", http.MethodGet, "/v1/stacks/stk_1/bindings", list, func(ctx context.Context, s StacksService) error {
			_, err := s.Bindings().List(ctx, "stk_1", opts...)
			return err
		}},
		{"Bindings.Create", http.MethodPost, "/v1/stacks/stk_1/bindings", obj, func(ctx context.Context, s StacksService) error {
			_, err := s.Bindings().Create(ctx, "stk_1", StackBindingCreateBody{DatabaseID: "db_1"}, opts...)
			return err
		}},
		{"Bindings.Update", http.MethodPatch, "/v1/stacks/stk_1/bindings/bnd_1", obj, func(ctx context.Context, s StacksService) error {
			_, err := s.Bindings().Update(ctx, "stk_1", "bnd_1", map[string]StackBindingValue{"X": StackBindingValueURL}, opts...)
			return err
		}},
		{"Bindings.Delete", http.MethodDelete, "/v1/stacks/stk_1/bindings/bnd_1", func(f *vertratest.FakeRestClient) {
			f.EnqueueJSON(200, `{"restart_required":false,"apps_to_restart":[]}`)
		}, func(ctx context.Context, s StacksService) error {
			_, err := s.Bindings().Delete(ctx, "stk_1", "bnd_1", nil, opts...)
			return err
		}},
		{"Connection", http.MethodGet, "/v1/stacks/stk_1/connection", func(f *vertratest.FakeRestClient) {
			f.EnqueueJSON(200, `{"app_id":"app_1","language":"javascript","snippets":[]}`)
		}, func(ctx context.Context, s StacksService) error {
			_, err := s.Connection(ctx, "stk_1", StackConnectionParams{AppID: "app_1"}, opts...)
			return err
		}},
		{"SetNetworkMode", http.MethodPost, "/v1/stacks/stk_1/network-mode", stack, func(ctx context.Context, s StacksService) error {
			_, err := s.SetNetworkMode(ctx, "stk_1", StackNetworkModePublic, opts...)
			return err
		}},
		{"AddDatabase", http.MethodPost, "/v1/stacks/stk_1/databases", obj, func(ctx context.Context, s StacksService) error {
			_, err := s.AddDatabase(ctx, "stk_1", StackDatabaseBody{Name: "pg", RAM: 256}, opts...)
			return err
		}},
		{"AddDatabaseToApp", http.MethodPost, "/v1/apps/app_1/stack/databases", obj, func(ctx context.Context, s StacksService) error {
			_, err := s.AddDatabaseToApp(ctx, "app_1", StackDatabaseBody{Name: "pg", RAM: 256}, opts...)
			return err
		}},
		{"AddBlob", http.MethodPost, "/v1/stacks/stk_1/blob", obj, func(ctx context.Context, s StacksService) error {
			_, err := s.AddBlob(ctx, "stk_1", StackBlobBody{}, opts...)
			return err
		}},
		{"Snapshots.List", http.MethodGet, "/v1/stacks/stk_1/snapshots", list, func(ctx context.Context, s StacksService) error {
			_, err := s.Snapshots().List(ctx, "stk_1", opts...)
			return err
		}},
		{"Snapshots.Create", http.MethodPost, "/v1/stacks/stk_1/snapshots", obj, func(ctx context.Context, s StacksService) error {
			_, err := s.Snapshots().Create(ctx, "stk_1", opts...)
			return err
		}},
		{"Snapshots.Restore", http.MethodPost, "/v1/stacks/stk_1/snapshots/snp_1/restore", func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"message":"ok"}`) }, func(ctx context.Context, s StacksService) error {
			_, err := s.Snapshots().Restore(ctx, "stk_1", "snp_1", nil, opts...)
			return err
		}},
	}
	if len(cases) != 20 {
		t.Fatalf("expected 20 stack routes, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			if err := tc.invoke(context.Background(), newStacksService(fake)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := fake.Requests[0]
			if got.Method != tc.wantMethod || got.Path != tc.wantPath {
				t.Errorf("got %s %s, want %s %s", got.Method, got.Path, tc.wantMethod, tc.wantPath)
			}
			if got.WorkspaceID != "ws_1" {
				t.Errorf("workspace id = %q, want ws_1", got.WorkspaceID)
			}
		})
	}
}

func TestStacks_GetDecodes(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, stackJSON)
	st, err := newStacksService(fake).Get(context.Background(), "stk_1")
	if err != nil {
		t.Fatal(err)
	}
	if st.Description != nil || st.NetworkMode != StackNetworkModePublic || st.WorkspaceID != nil || st.RAMTotal != 512 || st.Status.Total != 2 {
		t.Errorf("stack = %+v", st)
	}
	app, db, blob := st.Members[0], st.Members[1], st.Members[2]
	if *app.Status != "up" || *app.ServiceKey != "app" || app.DatabaseType != nil {
		t.Errorf("app member = %+v", app)
	}
	if *db.DatabaseType != DatabaseTypePostgreSQL || db.ServiceKey != nil {
		t.Errorf("db member = %+v", db)
	}
	if blob.ResourceType != StackMemberTypeBlob || blob.Status != nil || blob.Blob.Prefix != "loja/" || blob.Blob.UsedBytes != nil || *blob.Blob.APIKeyID != "key_1" {
		t.Errorf("blob member = %+v", blob)
	}
	b := st.Bindings[0]
	if *b.DatabaseID != "db_1" || b.BlobID != nil || b.EnvMap["PGSSLROOTCERT"] != StackBindingValueSSLCAPath || *b.CertsPath != "/app/.vertra/certs" {
		t.Errorf("binding = %+v", b)
	}
	if st.Template == nil || st.Template.Slug != "loja" || !st.UpdateAvailable || *st.Template.LatestVersion != "1.1.0" {
		t.Errorf("template = %+v", st.Template)
	}
}

func TestStacks_ChangeCarriesRestartNotice(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, stackWithRestart())
	change, err := newStacksService(fake).Create(context.Background(), StackCreateBody{
		Name: "loja", NetworkMode: StackNetworkModePublic,
		Members: []StackMemberRef{{ResourceType: StackMemberTypeApplication, ResourceID: "app_1"}, {ResourceType: StackMemberTypeDatabase, ResourceID: "db_1"}},
		Bindings: []StackConnectionInput{{
			StackBindingInput: StackBindingInput{AppID: "app_1", EnvMap: map[string]StackBindingValue{"DATABASE_URL": StackBindingValueURL}},
			DatabaseID:        "db_1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !change.RestartRequired || len(change.AppsToRestart) != 1 || change.ID != "stk_1" || len(change.Members) != 3 {
		t.Errorf("change = %+v", change)
	}
	if got := string(fake.Requests[0].Body); got != `{"name":"loja","network_mode":"public","members":[{"resource_type":"application","resource_id":"app_1"},{"resource_type":"database","resource_id":"db_1"}],"bindings":[{"app_id":"app_1","env_map":{"DATABASE_URL":"url"},"database_id":"db_1"}]}` {
		t.Errorf("body = %s", got)
	}
}

func TestStacks_UpdateDescriptionNullable(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, stackJSON)
	fake.EnqueueJSON(200, stackJSON)
	fake.EnqueueJSON(200, stackJSON)
	s := newStacksService(fake)
	if _, err := s.Update(context.Background(), "stk_1", StackUpdateBody{Description: NullableNull[string]()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(context.Background(), "stk_1", StackUpdateBody{Description: NullableValue("nova")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(context.Background(), "stk_1", StackUpdateBody{}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{`{"description":null}`, `{"description":"nova"}`, `{}`} {
		if got := string(fake.Requests[i].Body); got != want {
			t.Errorf("body %d = %s, want %s", i, got, want)
		}
	}
}

func TestStacks_DeleteBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"deleted_apps":["a"],"deleted_databases":["d"],"deleted_blob":true}`)
	res, err := newStacksService(fake).Delete(context.Background(), "stk_1", StackDeleteBody{Mode: StackDeleteModeDeleteAll, ConfirmName: "loja", Blob: StackBlobDispositionDelete})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fake.Requests[0].Body); got != `{"mode":"delete_all","confirm_name":"loja","blob":"delete"}` {
		t.Errorf("body = %s", got)
	}
	if res.DeletedApps[0] != "a" || res.DeletedDatabases[0] != "d" || !res.DeletedBlob {
		t.Errorf("res = %+v", res)
	}
}

func TestStacks_RemoveMemberAndBindingQueries(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, stackWithRestart())
	fake.EnqueueJSON(200, `{"restart_required":true,"apps_to_restart":["app_1"]}`)
	s := newStacksService(fake)
	if _, err := s.Members().Remove(context.Background(), "stk_1", StackMemberTypeBlob, "blb_1", &StackMemberRemoveParams{Bindings: StackBindingRemovalRemoveVariables, Blob: StackBlobDispositionDelete}); err != nil {
		t.Fatal(err)
	}
	q := fake.Requests[0].Query
	if q.Get("bindings") != "remove_variables" || q.Get("blob") != "delete" {
		t.Errorf("query = %v", q)
	}
	res, err := s.Bindings().Delete(context.Background(), "stk_1", "bnd_1", &StackBindingDeleteParams{Variables: StackBindingRemovalRemoveVariables})
	if err != nil {
		t.Fatal(err)
	}
	if fake.Requests[1].Query.Get("variables") != "remove_variables" || !res.RestartRequired || res.AppsToRestart[0] != "app_1" {
		t.Errorf("query = %v res = %+v", fake.Requests[1].Query, res)
	}
}

func TestStacks_ConnectionQuery(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"app_id":"app_1","language":"python","snippets":[{"language":"python","driver":"psycopg","database_type":1,"blob":false,"title":"Postgres","code":"x","env_vars":["DATABASE_URL"]},{"language":"python","driver":"boto3","database_type":null,"blob":true,"title":"Blob","code":"y","env_vars":[]}]}`)
	info, err := newStacksService(fake).Connection(context.Background(), "stk_1", StackConnectionParams{AppID: "app_1", Language: ApplicationLanguagePython})
	if err != nil {
		t.Fatal(err)
	}
	q := fake.Requests[0].Query
	if q.Get("app_id") != "app_1" || q.Get("language") != "python" {
		t.Errorf("query = %v", q)
	}
	if info.Snippets[0].Driver != "psycopg" || *info.Snippets[0].DatabaseType != DatabaseTypePostgreSQL || !info.Snippets[1].Blob || info.Snippets[1].DatabaseType != nil {
		t.Errorf("info = %+v", info)
	}
}

func TestStacks_AddDatabaseAndBlob(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"stack":`+stackJSON+`,"database_id":"db_9","restart_required":true,"apps_to_restart":["app_1"]}`)
	fake.EnqueueJSON(201, `{"stack":`+stackJSON+`,"blob":{"resource_type":"blob","resource_id":"blb_1","name":"files","status":null,"ram":0,"service_key":null,"database_type":null,"closed":false,"blob":null},"restart_required":false,"apps_to_restart":[]}`)
	s := newStacksService(fake)
	added, err := s.AddDatabase(context.Background(), "stk_1", StackDatabaseBody{Type: DatabaseTypePostgreSQL, Name: "pg", RAM: 256, Bindings: []StackBindingInput{{AppID: "app_1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fake.Requests[0].Body); got != `{"type":1,"name":"pg","ram":256,"bindings":[{"app_id":"app_1"}]}` {
		t.Errorf("body = %s", got)
	}
	if added.DatabaseID != "db_9" || !added.RestartRequired || added.Stack.ID != "stk_1" {
		t.Errorf("added = %+v", added)
	}
	blob, err := s.AddBlob(context.Background(), "stk_1", StackBlobBody{Prefix: "loja/", Visibility: TemplateBlobVisibilityPublic})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fake.Requests[1].Body); got != `{"prefix":"loja/","visibility":"public"}` {
		t.Errorf("body = %s", got)
	}
	if blob.Blob.ResourceType != StackMemberTypeBlob || blob.RestartRequired {
		t.Errorf("blob = %+v", blob)
	}
}

func TestStacks_BindingBodies(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"bnd_2","stack_id":"stk_1","app_id":"app_1","database_id":null,"blob_id":"blb_1","target_app_id":null,"env_map":{},"certs_path":null,"created_at":"2026-10-27T10:00:00Z","restart_required":true,"apps_to_restart":["app_1"]}`)
	fake.EnqueueJSON(200, `{"id":"bnd_2","stack_id":"stk_1","app_id":"app_1","database_id":null,"blob_id":"blb_1","target_app_id":null,"env_map":{"S3_BUCKET":"bucket"},"certs_path":null,"created_at":"2026-10-27T10:00:00Z","restart_required":true,"apps_to_restart":[]}`)
	s := newStacksService(fake).Bindings()
	created, err := s.Create(context.Background(), "stk_1", StackBindingCreateBody{StackBindingInput: StackBindingInput{AppID: "app_1"}, BlobID: "blb_1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fake.Requests[0].Body); got != `{"app_id":"app_1","blob_id":"blb_1"}` {
		t.Errorf("body = %s", got)
	}
	if *created.BlobID != "blb_1" || !created.RestartRequired {
		t.Errorf("created = %+v", created)
	}
	updated, err := s.Update(context.Background(), "stk_1", "bnd_2", map[string]StackBindingValue{"S3_BUCKET": StackBindingValueBucket})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fake.Requests[1].Body); got != `{"env_map":{"S3_BUCKET":"bucket"}}` {
		t.Errorf("body = %s", got)
	}
	if updated.EnvMap["S3_BUCKET"] != StackBindingValueBucket {
		t.Errorf("updated = %+v", updated)
	}
}

func TestStacks_SnapshotsDecodeAndRestoreBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"id":"snp_1","stack_id":"stk_1","reason":"update","date":"2026-10-27T10:00:00Z","size":"12 MB","members":[{"resource_type":"application","resource_id":"app_1","snapshot_id":"s1"}],"closed":true}]`)
	fake.EnqueueJSON(200, `{"message":"ok","warning":"IMAGE_VERSION_NOT_FOUND"}`)
	fake.EnqueueJSON(200, `{"message":"ok"}`)
	s := newStacksService(fake).Snapshots()
	list, err := s.List(context.Background(), "stk_1")
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Reason != StackSnapshotReasonUpdate || !list[0].Closed || list[0].Members[0].SnapshotID != "s1" || list[0].Members[0].ResourceType != WorkspaceResourceTypeApplication {
		t.Errorf("list = %+v", list)
	}
	res, err := s.Restore(context.Background(), "stk_1", "snp_1", &StackSnapshotRestoreParams{RestoreImageVersion: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(fake.Requests[1].Body) != `{"restore_image_version":true}` || res.Warning == nil || *res.Warning != SnapshotImageWarningImageVersionNotFound {
		t.Errorf("body = %s res = %+v", fake.Requests[1].Body, res)
	}
	if _, err := s.Restore(context.Background(), "stk_1", "snp_1", nil); err != nil {
		t.Fatal(err)
	}
	if len(fake.Requests[2].Body) != 0 || fake.Requests[2].ContentType != "" {
		t.Errorf("a nil params sends no body, got %q (%s)", fake.Requests[2].Body, fake.Requests[2].ContentType)
	}
}

func TestStacks_TransformBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, stackWithRestart())
	_, err := newStacksService(fake).Transform(context.Background(), StackTransformBody{
		FolderID: "fld_1", InjectVariables: true,
		Bindings: []StackTransformConnection{{StackBindingInput: StackBindingInput{AppID: "app_1"}, DatabaseID: "db_1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(fake.Requests[0].Body); got != `{"folder_id":"fld_1","inject_variables":true,"bindings":[{"app_id":"app_1","database_id":"db_1"}]}` {
		t.Errorf("body = %s", got)
	}
}

func TestStacks_ErrorCode(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(404, `{"code":"STACK_NOT_FOUND"}`)
	_, err := newStacksService(fake).Get(context.Background(), "nope")
	apiErr, ok := rest.AsAPIError(err)
	if !ok || apiErr.Code != ErrCodeStackNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestStacks_ClientWiring(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	c := New(fake)
	if c.Templates == nil || c.Stacks == nil {
		t.Fatal("Templates and Stacks must be wired")
	}
	if c.Workspaces.Templates() == nil || c.Workspaces.Stacks() == nil {
		t.Fatal("workspace move services must be wired")
	}
}

func TestWorkspaces_MoveTemplatesAndStacks(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	for i := 0; i < 4; i++ {
		fake.EnqueueJSON(200, `null`)
	}
	ws := newWorkspacesService(fake)
	ctx := context.Background()
	for _, err := range []error{
		ws.Templates().Add(ctx, "ws_1", "tpl_1"),
		ws.Templates().Remove(ctx, "ws_1", "tpl_1"),
		ws.Stacks().Add(ctx, "ws_1", "stk_1"),
		ws.Stacks().Remove(ctx, "ws_1", "stk_1"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []struct{ method, path string }{
		{http.MethodPost, "/v1/workspaces/ws_1/templates/tpl_1"},
		{http.MethodDelete, "/v1/workspaces/ws_1/templates/tpl_1"},
		{http.MethodPost, "/v1/workspaces/ws_1/stacks/stk_1"},
		{http.MethodDelete, "/v1/workspaces/ws_1/stacks/stk_1"},
	}
	for i, w := range want {
		if got := fake.Requests[i]; got.Method != w.method || got.Path != w.path {
			t.Errorf("request %d = %s %s, want %s %s", i, got.Method, got.Path, w.method, w.path)
		}
	}
}
