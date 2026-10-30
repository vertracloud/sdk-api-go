package vertracloud

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const deployJSON = `{"id":"dep_1","kind":"deploy","template_id":"tpl_1","version_id":"v1","version":"1.0.0","status":"queued","current_step":null,"steps":[{"step":"validate","status":"pending","started_at":null,"finished_at":null}],"error":null,"resources":[],"stack_id":null,"application_id":null,"outputs":[{"label":{"pt-br":"Senha"},"value":null,"secret":true,"once":true}],"created_at":"2026-10-27T10:00:00Z","updated_at":"2026-10-27T10:00:00Z","finished_at":null}`

// TestTemplates_RouteMatrix checks the method, path and workspace of every
// template route.
func TestTemplates_RouteMatrix(t *testing.T) {
	opts := []rest.RequestOpt{rest.WithWorkspaceID("ws_1")}
	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(f *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, s TemplatesService) error
	}
	obj := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{}`) }
	list := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `[]`) }
	null := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `null`) }
	job := func(f *vertratest.FakeRestClient) { f.EnqueueJSON(202, deployJSON) }
	manifest := TemplateManifest{ID: "x"}
	cases := []routeCase{
		{"List", http.MethodGet, "/v1/templates", func(f *vertratest.FakeRestClient) {
			f.EnqueueJSON(200, `{"data":[],"total_count":0,"page":1,"limit":20,"total_pages":0,"has_next_page":false,"has_previous_page":false}`)
		}, func(ctx context.Context, s TemplatesService) error { _, err := s.List(ctx, nil, opts...); return err }},
		{"Get", http.MethodGet, "/v1/templates/n8n", obj, func(ctx context.Context, s TemplatesService) error { _, err := s.Get(ctx, "n8n", opts...); return err }},
		{"Categories", http.MethodGet, "/v1/templates/categories", list, func(ctx context.Context, s TemplatesService) error { _, err := s.Categories(ctx, opts...); return err }},
		{"Versions", http.MethodGet, "/v1/templates/tpl_1/versions", list, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Versions(ctx, "tpl_1", opts...)
			return err
		}},
		{"Validate", http.MethodPost, "/v1/templates/validate", func(f *vertratest.FakeRestClient) {
			f.EnqueueJSON(200, `{"valid":true,"issues":[],"required_plan":"pro","ram_min_total":512,"services":[]}`)
		}, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Validate(ctx, manifest, opts...)
			return err
		}},
		{"Deploy", http.MethodPost, "/v1/templates/tpl_1/deploy", job, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Deploy(ctx, "tpl_1", TemplateDeployBody{Name: "a"}, opts...)
			return err
		}},
		{"Deploys.Get", http.MethodGet, "/v1/template-deploys/dep_1", func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, deployJSON) }, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Deploys().Get(ctx, "dep_1", opts...)
			return err
		}},
		{"Deploys.Events", http.MethodGet, "/v1/template-deploys/dep_1/events", func(f *vertratest.FakeRestClient) { f.EnqueueSSE(200, "event: end\ndata: {}\n\n") }, func(ctx context.Context, s TemplatesService) error {
			st, err := s.Deploys().Events(ctx, "dep_1", opts...)
			if err != nil {
				return err
			}
			return st.Close()
		}},
		{"Create", http.MethodPost, "/v1/templates", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Create(ctx, TemplateCreateParams{Manifest: manifest}, opts...)
			return err
		}},
		{"FromApp", http.MethodPost, "/v1/templates/from-app/app_1", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.FromApp(ctx, "app_1", TemplateFromAppBody{ID: "x"}, opts...)
			return err
		}},
		{"FromStack", http.MethodPost, "/v1/templates/from-stack/stk_1", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.FromStack(ctx, "stk_1", TemplateFromStackBody{ID: "x"}, opts...)
			return err
		}},
		{"AppDraft", http.MethodGet, "/v1/templates/from-app/app_1/draft", func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"members":[]}`) }, func(ctx context.Context, s TemplatesService) error {
			_, err := s.AppDraft(ctx, "app_1", opts...)
			return err
		}},
		{"StackDraft", http.MethodGet, "/v1/templates/from-stack/stk_1/draft", func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"members":[]}`) }, func(ctx context.Context, s TemplatesService) error {
			_, err := s.StackDraft(ctx, "stk_1", opts...)
			return err
		}},
		{"Update", http.MethodPatch, "/v1/templates/tpl_1", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Update(ctx, "tpl_1", TemplateUpdateDetailsBody{Tags: []string{"a"}}, opts...)
			return err
		}},
		{"Delete", http.MethodDelete, "/v1/templates/tpl_1", null, func(ctx context.Context, s TemplatesService) error { return s.Delete(ctx, "tpl_1", opts...) }},
		{"NewVersion", http.MethodPost, "/v1/templates/tpl_1/versions", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.NewVersion(ctx, "tpl_1", TemplateVersionCreateParams{Manifest: manifest}, opts...)
			return err
		}},
		{"LinkGitHub", http.MethodPut, "/v1/templates/tpl_1/github", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.LinkGitHub(ctx, "tpl_1", TemplateGitHubLinkBody{Repository: "a/b"}, opts...)
			return err
		}},
		{"UnlinkGitHub", http.MethodDelete, "/v1/templates/tpl_1/github", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.UnlinkGitHub(ctx, "tpl_1", opts...)
			return err
		}},
		{"ShareLinks.Create", http.MethodPost, "/v1/templates/tpl_1/share-links", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.ShareLinks().Create(ctx, "tpl_1", TemplateShareLinkCreateBody{}, opts...)
			return err
		}},
		{"ShareLinks.List", http.MethodGet, "/v1/templates/tpl_1/share-links", list, func(ctx context.Context, s TemplatesService) error {
			_, err := s.ShareLinks().List(ctx, "tpl_1", opts...)
			return err
		}},
		{"ShareLinks.Revoke", http.MethodDelete, "/v1/templates/tpl_1/share-links/lnk_1", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.ShareLinks().Revoke(ctx, "tpl_1", "lnk_1", opts...)
			return err
		}},
		{"Share.Preview", http.MethodGet, "/v1/share/templates/tok", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Share().Preview(ctx, "tok", opts...)
			return err
		}},
		{"Share.Deploy", http.MethodPost, "/v1/share/templates/tok/deploy", job, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Share().Deploy(ctx, "tok", TemplateShareDeployBody{Name: "a"}, opts...)
			return err
		}},
		{"Share.Copy", http.MethodPost, "/v1/share/templates/tok/copy", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Share().Copy(ctx, "tok", TemplateShareCopyBody{}, opts...)
			return err
		}},
		{"Share.Report", http.MethodPost, "/v1/share/templates/tok/report", null, func(ctx context.Context, s TemplatesService) error {
			return s.Share().Report(ctx, "tok", TemplateReportBody{Reason: TemplateReportReasonScam}, opts...)
		}},
		{"ReportApp", http.MethodPost, "/v1/apps/app_1/template/report", null, func(ctx context.Context, s TemplatesService) error {
			return s.ReportApp(ctx, "app_1", TemplateReportBody{Reason: TemplateReportReasonOther}, opts...)
		}},
		{"Updates.AppInfo", http.MethodGet, "/v1/apps/app_1/template/update", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Updates().AppInfo(ctx, "app_1", opts...)
			return err
		}},
		{"Updates.AppUpdate", http.MethodPost, "/v1/apps/app_1/template/update", job, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Updates().AppUpdate(ctx, "app_1", TemplateUpdateBody{}, opts...)
			return err
		}},
		{"Updates.AppDetach", http.MethodPost, "/v1/apps/app_1/template/detach", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Updates().AppDetach(ctx, "app_1", opts...)
			return err
		}},
		{"Updates.StackInfo", http.MethodGet, "/v1/stacks/stk_1/template/update", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Updates().StackInfo(ctx, "stk_1", opts...)
			return err
		}},
		{"Updates.StackUpdate", http.MethodPost, "/v1/stacks/stk_1/template/update", job, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Updates().StackUpdate(ctx, "stk_1", TemplateUpdateBody{}, opts...)
			return err
		}},
		{"Updates.StackDetach", http.MethodPost, "/v1/stacks/stk_1/template/detach", obj, func(ctx context.Context, s TemplatesService) error {
			_, err := s.Updates().StackDetach(ctx, "stk_1", opts...)
			return err
		}},
	}
	if len(cases) != 32 {
		t.Fatalf("expected 32 template routes, got %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			if err := tc.invoke(context.Background(), newTemplatesService(fake)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fake.Requests) != 1 {
				t.Fatalf("expected 1 request, got %d", len(fake.Requests))
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

func TestTemplates_ListQuery(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"data":[{"id":"tpl_1","slug":"n8n","origin":"vertra","visibility":"public","name":{"pt-br":"n8n"},"category":"automation","required_plan":"pro","ram_min_total":512,"current_version":{"id":"v1","version":"1.2.0","security":false,"created_at":"2026-10-27T10:00:00Z"},"services":[{"id":"app","kind":"app","source_type":"image","database":null,"blob_visibility":null,"ram":{"min":256,"recommended":512},"web":"required","conditional":false}],"uses_count":null,"owner":null,"github":null,"account":{"fits":false,"reasons":[{"reason":"ram","required":512,"available":128}],"required_ram":512,"free_ram":128,"needs_image_slot":true,"image_slots_free":0,"in_use":false,"in_use_count":0}}],"total_count":1,"page":2,"limit":10,"total_pages":2,"has_next_page":false,"has_previous_page":true}`)
	fits := true
	page, err := newTemplatesService(fake).List(context.Background(), &TemplateListParams{
		Search: "bot", Category: TemplateCategoryBots, Origin: TemplateOriginPrivate, Tag: "x", Fits: &fits,
		Sort: TemplateSortNewest, Page: 2, Limit: 10, WorkspaceID: "ws_9",
	})
	if err != nil {
		t.Fatal(err)
	}
	q := fake.Requests[0].Query
	for k, want := range map[string]string{"search": "bot", "category": "bots", "origin": "private", "tag": "x", "fits": "true", "sort": "newest", "page": "2", "limit": "10"} {
		if q.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if fake.Requests[0].WorkspaceID != "ws_9" {
		t.Errorf("workspace = %q", fake.Requests[0].WorkspaceID)
	}
	if page.Page != 2 || !page.HasPreviousPage || len(page.Data) != 1 {
		t.Fatalf("page = %+v", page)
	}
	tpl := page.Data[0]
	if tpl.Slug != "n8n" || tpl.RequiredPlan != "pro" || tpl.UsesCount != nil || tpl.Owner != nil {
		t.Errorf("template = %+v", tpl)
	}
	if tpl.Account == nil || tpl.Account.Fits || tpl.Account.Reasons[0].Reason != TemplateFitReasonRAM || string(tpl.Account.Reasons[0].Required) != "512" {
		t.Errorf("account = %+v", tpl.Account)
	}
	if tpl.Services[0].Web == nil || *tpl.Services[0].Web != TemplateWebModeRequired || *tpl.Services[0].SourceType != TemplateSourceTypeImage {
		t.Errorf("service = %+v", tpl.Services[0])
	}
}

func TestTemplates_ListOmitsEmptyQuery(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"data":[]}`)
	if _, err := newTemplatesService(fake).List(context.Background(), &TemplateListParams{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Requests[0].Query) != 0 {
		t.Errorf("query = %v, want empty", fake.Requests[0].Query)
	}
}

func TestTemplates_DeployBodyAndIdempotency(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, deployJSON)
	job, err := newTemplatesService(fake).Deploy(context.Background(), "n8n", TemplateDeployBody{
		Name:           "meu",
		Version:        "1.0.0",
		Inputs:         map[string]TemplateInputValue{"email": "a@b.c", "workers": 2, "debug": true},
		Services:       map[string]TemplateDeployServiceOptions{"app": {RAM: 512, Subdomain: "random"}},
		NetworkMode:    StackNetworkModePublic,
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := fake.Requests[0]
	if got := req.Header.Get("Idempotency-Key"); got != "key-1" {
		t.Errorf("Idempotency-Key = %q", got)
	}
	body := decodeJSONBody(t, req)
	if _, ok := body["IdempotencyKey"]; ok {
		t.Errorf("IdempotencyKey leaked into the body: %v", body)
	}
	if _, ok := body["idempotency_key"]; ok {
		t.Errorf("idempotency_key leaked into the body: %v", body)
	}
	if body["name"] != "meu" || body["version"] != "1.0.0" || body["network_mode"] != "public" {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["workspace_id"]; ok {
		t.Errorf("empty workspace_id must be omitted: %v", body)
	}
	inputs := body["inputs"].(map[string]any)
	if inputs["workers"] != float64(2) || inputs["debug"] != true {
		t.Errorf("inputs = %v", inputs)
	}
	if job.Status != TemplateDeployStatusQueued || job.Status.Finished() || job.CurrentStep != nil || job.StackID != nil {
		t.Errorf("job = %+v", job)
	}
	if len(job.Outputs) != 1 || !job.Outputs[0].Once {
		t.Fatalf("outputs = %+v", job.Outputs)
	}
	if _, ok := job.Outputs[0].Text(); ok {
		t.Error("a null output has no text")
	}
}

func TestTemplates_DeployWithoutKeySendsNoHeader(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, deployJSON)
	if _, err := newTemplatesService(fake).Deploy(context.Background(), "n8n", TemplateDeployBody{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if v := fake.Requests[0].Header.Get("Idempotency-Key"); v != "" {
		t.Errorf("Idempotency-Key = %q, want none", v)
	}
}

func TestTemplates_ShareDeployIdempotencyAndNoVersion(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, deployJSON)
	if _, err := newTemplatesService(fake).Share().Deploy(context.Background(), "tok", TemplateShareDeployBody{Name: "a", IdempotencyKey: "k"}); err != nil {
		t.Fatal(err)
	}
	if fake.Requests[0].Header.Get("Idempotency-Key") != "k" {
		t.Error("missing Idempotency-Key")
	}
	if _, ok := decodeJSONBody(t, fake.Requests[0])["version"]; ok {
		t.Error("a share deploy has no version")
	}
}

func TestTemplates_DeployOutputText(t *testing.T) {
	out := TemplateDeployOutput{Value: json.RawMessage(`"s3cret"`)}
	if s, ok := out.Text(); !ok || s != "s3cret" {
		t.Errorf("Text = %q %v", s, ok)
	}
	if _, ok := (TemplateDeployOutput{Value: json.RawMessage(`{"pt-br":"x"}`)}).Text(); ok {
		t.Error("an i18n value is not plain text")
	}
}

func TestTemplates_ValidateSendsManifestAndReadsIssues(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"valid":false,"issues":[{"path":"services.app.ram","code":"invalid_ram","message":"too low"}],"required_plan":null,"ram_min_total":null,"services":[]}`)
	res, err := newTemplatesService(fake).Validate(context.Background(), json.RawMessage(`{"id":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(fake.Requests[0].Body) != `{"manifest":{"id":"x"}}` {
		t.Errorf("body = %s", fake.Requests[0].Body)
	}
	if res.Valid || res.RequiredPlan != nil || res.RAMMinTotal != nil || res.Issues[0].Code != TemplateValidationCodeInvalidRAM {
		t.Errorf("res = %+v", res)
	}
}

func readMultipart(t *testing.T, req vertratest.CapturedCall) map[string]string {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(req.ContentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type %q: %v", req.ContentType, err)
	}
	mr := multipart.NewReader(strings.NewReader(string(req.Body)), params["boundary"])
	parts := map[string]string{}
	var order []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(p)
		name := p.FormName()
		if p.FileName() != "" {
			name += "|" + p.FileName() + "|" + p.Header.Get("Content-Type")
		}
		parts[name] = string(b)
		order = append(order, name)
	}
	parts["__order"] = strings.Join(order, ",")
	return parts
}

func TestTemplates_CreateMultipart(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"tpl_1"}`)
	_, err := newTemplatesService(fake).Create(context.Background(), TemplateCreateParams{
		Manifest:    TemplateManifest{ID: "meu", Version: "1.0.0", Name: TemplateI18nText{PtBR: "Meu"}, Category: TemplateCategoryOther, Services: map[string]TemplateManifestService{"web": {Source: &TemplateSource{Type: TemplateSourceTypeCode, Path: "web"}}}},
		Changelog:   &TemplateI18nText{PtBR: "primeira"},
		WorkspaceID: "ws_1",
		Sources:     map[string]io.Reader{"web": strings.NewReader("ZIPWEB"), "api": strings.NewReader("ZIPAPI")},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := fake.Requests[0]
	if req.Method != http.MethodPost || req.Path != "/v1/templates" {
		t.Fatalf("%s %s", req.Method, req.Path)
	}
	parts := readMultipart(t, req)
	var m map[string]any
	if err := json.Unmarshal([]byte(parts["manifest"]), &m); err != nil || m["id"] != "meu" {
		t.Fatalf("manifest part = %q (%v)", parts["manifest"], err)
	}
	if parts["changelog"] != `{"pt-br":"primeira"}` || parts["workspace_id"] != "ws_1" {
		t.Errorf("parts = %v", parts)
	}
	if parts["sources[web]|web.zip|application/zip"] != "ZIPWEB" || parts["sources[api]|api.zip|application/zip"] != "ZIPAPI" {
		t.Errorf("source parts = %v", parts)
	}
	if !strings.HasSuffix(parts["__order"], "sources[api]|api.zip|application/zip,sources[web]|web.zip|application/zip") {
		t.Errorf("sources are not in a stable order: %s", parts["__order"])
	}
}

func TestTemplates_NewVersionMultipartHasNoWorkspaceField(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"v2","version":"1.1.0","created_at":"2026-10-27T10:00:00Z","security":false,"changelog":null,"manifest":null}`)
	v, err := newTemplatesService(fake).NewVersion(context.Background(), "tpl_1", TemplateVersionCreateParams{Manifest: map[string]any{"id": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	parts := readMultipart(t, fake.Requests[0])
	if _, ok := parts["workspace_id"]; ok {
		t.Error("NewVersion must not send workspace_id")
	}
	if _, ok := parts["changelog"]; ok {
		t.Error("changelog must be omitted when nil")
	}
	if v.Version != "1.1.0" || v.Manifest != nil {
		t.Errorf("version = %+v", v)
	}
}

func TestTemplates_CreateNeedsManifest(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	if _, err := newTemplatesService(fake).Create(context.Background(), TemplateCreateParams{}); err == nil {
		t.Fatal("expected an error without a manifest")
	}
	if len(fake.Requests) != 0 {
		t.Error("no request must be made")
	}
}

func TestTemplates_FromAppBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"tpl_1"}`)
	_, err := newTemplatesService(fake).FromApp(context.Background(), "app_1", TemplateFromAppBody{
		ID: "meu", Name: TemplateI18nText{PtBR: "Meu"}, Description: TemplateI18nText{PtBR: "d"}, Category: TemplateCategoryBots,
		Member: TemplateFromMember{ResourceID: "app_1", ServiceID: "app", Env: []TemplateEnvReview{{Key: "TOKEN", Mode: TemplateEnvReviewModeQuestion, Label: &TemplateI18nText{PtBR: "Token"}}}, Paths: []string{"src"}, RAM: &TemplateRAM{Min: 128, Recommended: 256}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := decodeJSONBody(t, fake.Requests[0])
	member := body["member"].(map[string]any)
	env := member["env"].([]any)[0].(map[string]any)
	if env["mode"] != "question" || env["key"] != "TOKEN" || member["service_id"] != "app" {
		t.Errorf("member = %v", member)
	}
}

func TestTemplates_DraftDecodes(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"members":[{"resource_id":"a1","resource_type":"application","name":"api","ram":{"min":128,"recommended":256},"env":[{"key":"API_KEY","looks_secret":true,"suggested_mode":"question"}],"paths":[{"path":".env","type":"file","selected":false,"reason":"env_file"},{"path":"src","type":"directory","selected":true,"reason":null}]}]}`)
	d, err := newTemplatesService(fake).AppDraft(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	m := d.Members[0]
	if m.ResourceType != WorkspaceResourceTypeApplication || !m.Env[0].LooksSecret || m.Env[0].SuggestedMode != TemplateEnvReviewModeQuestion {
		t.Errorf("member = %+v", m)
	}
	if m.Paths[0].Reason == nil || *m.Paths[0].Reason != "env_file" || m.Paths[1].Reason != nil {
		t.Errorf("paths = %+v", m.Paths)
	}
}

func TestTemplates_UpdateDetailsOmitsUnset(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{}`)
	if _, err := newTemplatesService(fake).Update(context.Background(), "tpl_1", TemplateUpdateDetailsBody{Category: TemplateCategoryCMS}); err != nil {
		t.Fatal(err)
	}
	if string(fake.Requests[0].Body) != `{"category":"cms"}` {
		t.Errorf("body = %s", fake.Requests[0].Body)
	}
}

func TestTemplates_ShareLinkCreateAndPreview(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"lnk_1","template_id":"tpl_1","mode":"full_copy","last4":"ab12","uses":0,"max_uses":5,"expires_at":null,"revoked_at":null,"created_at":"2026-10-27T10:00:00Z","token":"tok_secret"}`)
	link, err := newTemplatesService(fake).ShareLinks().Create(context.Background(), "tpl_1", TemplateShareLinkCreateBody{Mode: TemplateShareLinkModeFullCopy, MaxUses: 5, ExpiresInDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	if string(fake.Requests[0].Body) != `{"mode":"full_copy","max_uses":5,"expires_in_days":7}` {
		t.Errorf("body = %s", fake.Requests[0].Body)
	}
	if link.Token != "tok_secret" || link.Last4 != "ab12" || *link.MaxUses != 5 || link.ExpiresAt != nil {
		t.Errorf("link = %+v", link)
	}

	fake.EnqueueJSON(200, `{"mode":"use","name":{"pt-br":"X"},"description":{"pt-br":"d"},"category":"bots","icon_url":null,"author_display":"ana","version":"1.0.0","services":[],"creates_stack":false,"ram_min_total":256,"required_plan":"pro","image_slots_needed":2,"verified":false,"code_view":true,"byok":false,"inputs":[],"permissions":{"code_view":true,"code_edit":false,"ram_change":true,"ram_below_min":false,"ram_above_max":false,"env_add":true,"commands_edit":false,"terminal":false,"save_as_template":false,"detach":true}}`)
	pv, err := newTemplatesService(fake).Share().Preview(context.Background(), "a/b")
	if err != nil {
		t.Fatal(err)
	}
	if fake.Requests[1].Path != "/v1/share/templates/a%2Fb" {
		t.Errorf("token must be escaped: %s", fake.Requests[1].Path)
	}
	if pv.Verified || pv.ImageSlotsNeeded != 2 || !pv.Permissions.Detach || pv.Account != nil || *pv.AuthorDisplay != "ana" {
		t.Errorf("preview = %+v", pv)
	}
}

func TestTemplates_UpdateInfoDecodes(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"update_available":true,"current_version":"1.0.0","latest_version":"1.1.0","security":true,"changelog":{"pt-br":"fix"},"new_inputs":[],"added_services":[],"removed_services":["old"],"ram_changes":[{"service_id":"app","current":128,"required_min":256}],"free_ram":512,"opens_permissions":false,"rollback_with_data":true,"blocked":{"reason":"ram","details":{"need":256}}}`)
	info, err := newTemplatesService(fake).Updates().AppInfo(context.Background(), "app_1")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Security || info.RemovedServices[0] != "old" || info.RAMChanges[0].RequiredMin != 256 || info.Blocked.Reason != TemplateUpdateBlockReasonRAM {
		t.Errorf("info = %+v", info)
	}
}

func TestTemplates_UpdateBodyOmitsEmpty(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, deployJSON)
	if _, err := newTemplatesService(fake).Updates().StackUpdate(context.Background(), "stk_1", TemplateUpdateBody{RAM: map[string]int{"app": 256}}); err != nil {
		t.Fatal(err)
	}
	if string(fake.Requests[0].Body) != `{"ram":{"app":256}}` {
		t.Errorf("body = %s", fake.Requests[0].Body)
	}
}

func TestTemplates_DeployEventStream(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueSSE(200, strings.Join([]string{
		`event: progress`, `data: {"deploy_id":"dep_1","status":"running","current_step":"apps","steps":[{"step":"apps","status":"running","started_at":"2026-10-27T10:00:00Z","finished_at":null}],"error":null}`, ``,
		`event: heartbeat`, `data: {}`, ``,
		`event: progress`, `data: {"deploy_id":"dep_1","status":"failed","current_step":null,"steps":[],"error":{"code":"TEMPLATE_DEPLOY_FAILED","message":"x","step":"apps"}}`, ``,
		`event: end`, `data: {}`, ``,
		`event: progress`, `data: {"deploy_id":"dep_1","status":"succeeded","current_step":null,"steps":[],"error":null}`, ``, ``,
	}, "\n"))
	stream, err := newTemplatesService(fake).Deploys().Events(context.Background(), "dep_1")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if fake.Requests[0].Header.Get("Accept") != "text/event-stream" {
		t.Error("missing Accept header")
	}
	var seen []TemplateDeployStatus
	for stream.Next(context.Background()) {
		seen = append(seen, stream.Event().Status)
	}
	if len(seen) != 2 || seen[0] != TemplateDeployStatusRunning || seen[1] != TemplateDeployStatusFailed {
		t.Fatalf("seen = %v", seen)
	}
	if !stream.Ended() || stream.Err() != nil {
		t.Errorf("ended=%v err=%v", stream.Ended(), stream.Err())
	}
	ev := stream.Event()
	if ev.Error == nil || ev.Error.Code != ErrCodeTemplateDeployFailed || *ev.Error.Step != TemplateDeployStepApps {
		t.Errorf("last event = %+v", ev)
	}
	if stream.Next(context.Background()) {
		t.Error("nothing is read after the end event")
	}
}

func TestTemplates_DeployEventStreamBadData(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueSSE(200, "event: progress\ndata: not json\n\n")
	stream, err := newTemplatesService(fake).Deploys().Events(context.Background(), "dep_1")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if stream.Next(context.Background()) || stream.Err() == nil {
		t.Errorf("expected a decode error, got %v", stream.Err())
	}
}

func TestTemplates_DeployEventStreamAPIError(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueSSE(404, `{"code":"TEMPLATE_DEPLOY_NOT_FOUND"}`)
	_, err := newTemplatesService(fake).Deploys().Events(context.Background(), "nope")
	apiErr, ok := rest.AsAPIError(err)
	if !ok || apiErr.Code != ErrCodeTemplateDeployNotFound || !apiErr.IsNotFoundError() {
		t.Fatalf("err = %v", err)
	}
}

func TestTemplates_RejectsDotSegments(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	if _, err := newTemplatesService(fake).Get(context.Background(), ".."); err == nil {
		t.Fatal("expected ErrInvalidPathSegment")
	}
	if _, err := newStacksService(fake).Get(context.Background(), ".."); err == nil {
		t.Fatal("expected ErrInvalidPathSegment")
	}
	if len(fake.Requests) != 0 {
		t.Error("no request must be made")
	}
}

func TestTemplates_VersionDecodesManifest(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"id":"v1","version":"1.0.0","security":false,"created_at":"2026-10-27T10:00:00Z","changelog":null,"manifest":{"id":"x","version":"1.0.0","name":{"pt-br":"X"},"description":{"pt-br":"d"},"category":"apis","inputs":[{"id":"mode","type":"select","label":{"pt-br":"Modo"},"options":[{"value":"a","sets":{"services":{"db":"on"},"ram_extra":{"app":128}}}],"when":{"input":"x","equals":true}}],"services":{"app":{"source":{"type":"image","image":"img_1"},"ram":{"min":128,"recommended":256},"web":"required","port":3000,"deploy":{"zero_downtime":{"default":"auto","locked":true}},"healthcheck":{"timeout":30}},"db":{"database":"postgres","ram":{"min":256,"recommended":256},"when":{"input":"mode","equals":"a"}},"files":{"blob":{"visibility":"private"}}},"outputs":[{"label":{"pt-br":"URL"},"value":"https://x","once":true}]}}]`)
	versions, err := newTemplatesService(fake).Versions(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	m := versions[0].Manifest
	if m == nil || m.Services["app"].Source.Type != TemplateSourceTypeImage || m.Services["app"].Port != 3000 || m.Services["db"].Database != TemplateDatabaseEnginePostgres {
		t.Fatalf("manifest = %+v", m)
	}
	if !m.Services["app"].Deploy.ZeroDowntime.Locked || m.Services["files"].Blob.Visibility != TemplateBlobVisibilityPrivate {
		t.Errorf("services = %+v", m.Services)
	}
	if m.Inputs[0].Options[0].Sets.Services["db"] != "on" || m.Inputs[0].When.Equals != true {
		t.Errorf("inputs = %+v", m.Inputs)
	}
	if s, ok := (TemplateDeployOutput{Value: m.Outputs[0].Value}).Text(); !ok || s != "https://x" {
		t.Errorf("output = %q", s)
	}
}

func TestErrorHelpers(t *testing.T) {
	newErr := func(code, details string) error {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueError(400, `{"code":"`+code+`","details":`+details+`}`)
		_, err := newTemplatesService(fake).Get(context.Background(), "x")
		return err
	}
	issues, ok := TemplateManifestIssues(newErr("TEMPLATE_MANIFEST_INVALID", `{"issues":[{"path":"id","code":"invalid_slug","message":"m"}]}`))
	if !ok || len(issues) != 1 || issues[0].Code != TemplateValidationCodeInvalidSlug {
		t.Errorf("issues = %v %v", issues, ok)
	}
	if _, ok := TemplateManifestIssues(newErr("OTHER", `{}`)); ok {
		t.Error("another code is not a manifest error")
	}
	if _, ok := TemplateManifestIssues(nil); ok {
		t.Error("nil is not a manifest error")
	}
	files, ok := TemplateSecretFiles(newErr("TEMPLATE_SECRET_IN_SOURCE", `{"files":[".env","k.pem"]}`))
	if !ok || len(files) != 2 || files[0] != ".env" {
		t.Errorf("files = %v %v", files, ok)
	}
	blocked, ok := TemplateUpdateBlockedDetails(newErr("TEMPLATE_UPDATE_BLOCKED", `{"reason":"app_building"}`))
	if !ok || blocked.Reason != TemplateUpdateBlockReasonAppBuilding {
		t.Errorf("blocked = %+v %v", blocked, ok)
	}
	action, ok := TemplateDetachConfirmation(newErr("TEMPLATE_DETACH_CONFIRMATION_REQUIRED", `{"action":"files.write"}`))
	if !ok || action != TemplateDetachConfirmationActionFilesWrite {
		t.Errorf("action = %q %v", action, ok)
	}
	// A matching code without readable details still reports ok.
	if _, ok := TemplateSecretFiles(newErr("TEMPLATE_SECRET_IN_SOURCE", `"oops"`)); !ok {
		t.Error("expected ok with unreadable details")
	}
}

func TestTemplates_ApplicationTemplateLockedDeploySettings(t *testing.T) {
	var got ApplicationTemplate
	raw := `{"template_id":"t","service_id":null,"permissions":{},"locked_deploy_settings":["zero_downtime"]}`
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.LockedDeploySettings) != 1 || got.LockedDeploySettings[0] != "zero_downtime" {
		t.Fatalf("locked_deploy_settings = %v", got.LockedDeploySettings)
	}
}
