package vertracloud

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

func TestApplication_DecodesStackAndTemplate(t *testing.T) {
	var app Application
	raw := `{"id":"app_1","name":"n8n","stack_id":"stk_1","stack_service_key":"app","template":{"template_id":"tpl_1","slug":"n8n","name":{"pt-br":"n8n","en":"n8n"},"origin":"community","version_id":"v1","version":"1.0.0","closed":true,"detached":false,"update_available":true,"latest_version":"1.1.0","removed":false,"via_share_link":true,"service_id":null,"permissions":{"code_view":false,"code_edit":false,"ram_change":true,"ram_below_min":false,"ram_above_max":false,"env_add":false,"commands_edit":false,"terminal":false,"save_as_template":false,"detach":false}}}`
	if err := json.Unmarshal([]byte(raw), &app); err != nil {
		t.Fatal(err)
	}
	if *app.StackID != "stk_1" || *app.StackServiceKey != "app" {
		t.Errorf("stack fields = %v %v", app.StackID, app.StackServiceKey)
	}
	tpl := app.Template
	if tpl == nil || tpl.Slug != "n8n" || tpl.Origin != TemplateOriginCommunity || !tpl.Closed || !tpl.ViaShareLink || *tpl.LatestVersion != "1.1.0" || tpl.ServiceID != nil || !tpl.Permissions.RAMChange || tpl.Name.EN != "n8n" {
		t.Errorf("template = %+v", tpl)
	}

	var plain Application
	if err := json.Unmarshal([]byte(`{"id":"app_2"}`), &plain); err != nil {
		t.Fatal(err)
	}
	if plain.StackID != nil || plain.Template != nil || plain.StackServiceKey != nil {
		t.Errorf("a plain app has no stack or template: %+v", plain)
	}
}

func TestApplicationEnvironment_DecodesManagedBy(t *testing.T) {
	var envs []ApplicationEnvironment
	raw := `[{"id":"1","key":"DATABASE_URL","value":"x","note":null,"managed_by":{"type":"stack_binding","binding_id":"bnd_1","stack_id":"stk_1"},"created_at":"2026-10-27T10:00:00Z"},{"id":"2","key":"PUBLIC_URL","value":"y","note":null,"managed_by":{"type":"stack_binding","binding_id":"bnd_2","stack_id":null},"created_at":"2026-10-27T10:00:00Z"},{"id":"3","key":"A","value":"z","note":null,"created_at":"2026-10-27T10:00:00Z"}]`
	if err := json.Unmarshal([]byte(raw), &envs); err != nil {
		t.Fatal(err)
	}
	if envs[0].ManagedBy == nil || envs[0].ManagedBy.BindingID != "bnd_1" || *envs[0].ManagedBy.StackID != "stk_1" {
		t.Errorf("env 0 = %+v", envs[0].ManagedBy)
	}
	if envs[1].ManagedBy == nil || envs[1].ManagedBy.StackID != nil {
		t.Errorf("env 1 = %+v", envs[1].ManagedBy)
	}
	if envs[2].ManagedBy != nil {
		t.Errorf("env 2 = %+v", envs[2].ManagedBy)
	}
}

func TestDatabase_DecodesStackFields(t *testing.T) {
	var db Database
	if err := json.Unmarshal([]byte(`{"id":"db_1","stack_id":"stk_1","stack_service_key":"db"}`), &db); err != nil {
		t.Fatal(err)
	}
	if *db.StackID != "stk_1" || *db.StackServiceKey != "db" {
		t.Errorf("db = %+v", db)
	}
}

func TestDatabaseResets_CarryRestartNotice(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"crt":"C","key":"K","pem":"P","restart_required":true,"apps_to_restart":["app_1","app_2"]}`)
	fake.EnqueueJSON(200, `{"password":"pw","restart_required":false,"apps_to_restart":[]}`)
	svc := newDatabasesService(fake)
	cert, err := svc.Credentials().Certificate().Reset(context.Background(), "db_1")
	if err != nil {
		t.Fatal(err)
	}
	if cert.Crt != "C" || cert.Pem != "P" || !cert.RestartRequired || len(cert.AppsToRestart) != 2 {
		t.Errorf("cert = %+v", cert)
	}
	pw, err := svc.Credentials().Password().Reset(context.Background(), "db_1")
	if err != nil {
		t.Fatal(err)
	}
	if pw.Password != "pw" || pw.RestartRequired || len(pw.AppsToRestart) != 0 {
		t.Errorf("pw = %+v", pw)
	}
}

func TestResourceSnapshot_DecodesClosed(t *testing.T) {
	var snaps []ResourceSnapshot
	if err := json.Unmarshal([]byte(`[{"id":"s1","resource_id":"a","size":"1 MB","date":"2026-10-27T10:00:00Z","closed":true},{"id":"s2","resource_id":"a","size":"1 MB","date":"2026-10-27T10:00:00Z"}]`), &snaps); err != nil {
		t.Fatal(err)
	}
	if !snaps[0].Closed || snaps[1].Closed {
		t.Errorf("snaps = %+v", snaps)
	}
}

func TestCustomImage_DecodesKindAndOrigin(t *testing.T) {
	var img CustomImage
	raw := `{"id":"img_1","name":"n8n","kind":"template_copy","origin":{"template_id":"tpl_1","template_name":{"pt-br":"n8n"},"author_display":null},"created_at":"2026-10-27T10:00:00Z","updated_at":"2026-10-27T10:00:00Z"}`
	if err := json.Unmarshal([]byte(raw), &img); err != nil {
		t.Fatal(err)
	}
	if img.Kind != CustomImageKindTemplateCopy || img.Origin == nil || img.Origin.TemplateName.PtBR != "n8n" || img.Origin.AuthorDisplay != nil {
		t.Errorf("img = %+v", img)
	}
	var own CustomImage
	if err := json.Unmarshal([]byte(`{"id":"img_2"}`), &own); err != nil {
		t.Fatal(err)
	}
	if own.Kind != "" || own.Origin != nil {
		t.Errorf("own = %+v", own)
	}
}

func TestWorkspaceFolder_DecodesNotesAndFlowLayout(t *testing.T) {
	var f WorkspaceResourceFolder
	raw := `{"id":"f1","name":"loja","color":"red","position":0,"notes":"anotações","flow_layout":{"x":10.5,"y":-3,"collapsed":true},"resources":[{"resource_type":"stack","resource_id":"stk_1","position":0}],"created_at":"2026-10-27T10:00:00Z","updated_at":"2026-10-27T10:00:00Z"}`
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatal(err)
	}
	if *f.Notes != "anotações" || f.FlowLayout == nil || f.FlowLayout.X != 10.5 || !f.FlowLayout.Collapsed {
		t.Errorf("folder = %+v", f)
	}
	if f.Resources[0].ResourceType != WorkspaceResourceTypeStack {
		t.Errorf("resources = %+v", f.Resources)
	}
	var bare WorkspaceResourceFolder
	if err := json.Unmarshal([]byte(`{"id":"f2"}`), &bare); err != nil {
		t.Fatal(err)
	}
	if bare.Notes != nil || bare.FlowLayout != nil {
		t.Errorf("bare = %+v", bare)
	}
}

func TestAPIKey_DecodesBlobPrefixAndManager(t *testing.T) {
	var k APIKey
	raw := `{"id":"k1","name":"stack","prefix":"vc_","last4":"abcd","scopes":["blob:read","blob:write"],"allowed_ips":[],"blob_prefix":"loja/","managed_by":{"type":"stack","id":"stk_1"},"created_at":"2026-10-27T10:00:00Z","last_used_at":null}`
	if err := json.Unmarshal([]byte(raw), &k); err != nil {
		t.Fatal(err)
	}
	if *k.BlobPrefix != "loja/" || k.ManagedBy == nil || k.ManagedBy.Type != "stack" || k.ManagedBy.ID != "stk_1" || k.LastUsedAt != nil {
		t.Errorf("key = %+v", k)
	}
}

func TestConfirmDetach_FilesAndWebhook(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	for i := 0; i < 3; i++ {
		fake.EnqueueJSON(200, `null`)
	}
	fake.EnqueueJSON(200, `{"app_id":"a","updated_at":"2026-10-27T10:00:00Z"}`)
	fake.EnqueueJSON(200, `{"webhook_url":"u"}`)
	apps := newAppsService(fake)
	ctx := context.Background()
	content := "x"
	if err := apps.Files().Write(ctx, "a", ApplicationFileWriteBody{Path: "/a", Content: &content, ConfirmDetach: true}); err != nil {
		t.Fatal(err)
	}
	if err := apps.Files().Move(ctx, "a", ApplicationFileMoveBody{Path: "/a", To: "/b", ConfirmDetach: true}); err != nil {
		t.Fatal(err)
	}
	if err := apps.Files().Delete(ctx, "a", ApplicationFileDeleteBody{Path: "/a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := apps.Files().Upload(ctx, "a", ApplicationFileUploadParams{File: strings.NewReader("z"), FileName: "a.zip", ConfirmDetach: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := apps.Deploys().Webhook().Create(ctx, "a", ApplicationWebhookCreateBody{Owner: "o", RepoName: "r", RepoID: "1", AccountID: "2", ConfirmDetach: true}); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 1, 4} {
		if decodeJSONBody(t, fake.Requests[i])["confirm_detach"] != true {
			t.Errorf("request %d: confirm_detach not sent: %s", i, fake.Requests[i].Body)
		}
	}
	if strings.Contains(string(fake.Requests[2].Body), "confirm_detach") {
		t.Errorf("confirm_detach must be omitted when false: %s", fake.Requests[2].Body)
	}
	if fake.Requests[3].Query.Get("confirm_detach") != "true" || fake.Requests[3].Method != http.MethodPost {
		t.Errorf("upload query = %v", fake.Requests[3].Query)
	}
}

func TestConfirmDetach_UploadOmitsQueryWhenFalse(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"app_id":"a","updated_at":"2026-10-27T10:00:00Z"}`)
	if _, err := newAppsService(fake).Files().Upload(context.Background(), "a", ApplicationFileUploadParams{File: strings.NewReader("z"), FileName: "a.zip"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Requests[0].Query["confirm_detach"]; ok {
		t.Errorf("query = %v", fake.Requests[0].Query)
	}
}

func TestConfirmDetach_Error409(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(409, `{"code":"TEMPLATE_DETACH_CONFIRMATION_REQUIRED","details":{"action":"commit"}}`)
	err := newAppsService(fake).Files().Delete(context.Background(), "a", ApplicationFileDeleteBody{Path: "/a"})
	action, ok := TemplateDetachConfirmation(err)
	if !ok || action != TemplateDetachConfirmationActionCommit {
		t.Fatalf("action = %q ok = %v err = %v", action, ok, err)
	}
}
