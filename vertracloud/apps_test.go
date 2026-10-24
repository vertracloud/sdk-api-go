package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const (
	testAppID     = "app_123"
	testEnvID     = "env_456"
	testReleaseID = "rel_789"
	testDomainID  = "dom_321"
)

const rollbackReleaseJSON = `{"id":"rel_9","app_id":"app_123","source":"rollback","status":"active","message":null,"author":"octo","github":null,"files_changed":2,"rollback":{"available":false,"expires_at":null,"from":"rel_789"},"has_log":false,"build_duration_ms":null,"first_started_at":"2026-10-11T12:00:00.000Z","created_at":"2026-10-11T12:00:00.000Z"}`

// parseMultipartRequest decodes a captured multipart/form-data request into
// plain text fields and file fields, for asserting on field names/content
// without hand-parsing MIME boundaries in every test.
func parseMultipartRequest(t *testing.T, req vertratest.CapturedCall) (fields map[string]string, files map[string]string, fileNames map[string]string) {
	t.Helper()
	ct := req.ContentType
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Fatalf("Content-Type = %q, want multipart/form-data prefix", ct)
	}
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("parse Content-Type: %v", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		t.Fatalf("Content-Type %q has no boundary", ct)
	}

	fields = map[string]string{}
	files = map[string]string{}
	fileNames = map[string]string{}

	mr := multipart.NewReader(bytes.NewReader(req.Body), boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart part body: %v", err)
		}
		name := part.FormName()
		if fn := part.FileName(); fn != "" {
			files[name] = string(data)
			fileNames[name] = fn
		} else {
			fields[name] = string(data)
		}
	}
	return fields, files, fileNames
}

// TestApps_RouteMatrix checks, for every non-multipart/non-SSE/non-binary
// route (38 of the 42), that the Go method issues the expected HTTP method
// and path, and that WithWorkspaceID is forwarded correctly. Apps.Create,
// Apps.Files().Upload, Apps.Download and Apps.Realtime are covered by their
// own tests below.
func TestApps_RouteMatrix(t *testing.T) {
	opts := []rest.RequestOpt{rest.WithWorkspaceID("ws_1")}

	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(fake *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, svc AppsService) error
	}

	cases := []routeCase{
		{
			name: "Runtimes", wantMethod: http.MethodGet, wantPath: "/v1/apps/runtimes",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Runtimes(ctx, opts...)
				return err
			},
		},
		{
			name: "StatusAll", wantMethod: http.MethodGet, wantPath: "/v1/apps/status",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `[{"id":"1","cpu":"1%","ram":"1%","running":true,"uptime":10}]`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.StatusAll(ctx, opts...)
				return err
			},
		},
		{
			name: "Get", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"id":"1","name":"x"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Get(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Status", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/status",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"id":"1","cpu":"1%","ram":"1%","status":"up","running":true,"storage":"1%","network":{"total":"0","now":"0"},"uptime":1}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Status(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Metrics", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/metrics",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Metrics(ctx, testAppID, nil, opts...)
				return err
			},
		},
		{
			name: "Logs", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/logs",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `"log line"`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Logs(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Deploys.List", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/deploys",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Deploys().List(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Deploys.Webhook.Get", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/deploys/webhook",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app"}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Deploys().Webhook().Get(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Start", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/start",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"ok"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Start(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Stop", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/stop",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"ok"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Stop(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Restart", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/restart",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"status":"ok"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				reinstall := true
				_, err := svc.Restart(ctx, testAppID, &ApplicationRestartBody{ReinstallDependencies: &reinstall}, opts...)
				return err
			},
		},
		{
			name: "UpdateConfig", wantMethod: http.MethodPatch, wantPath: "/v1/apps/" + testAppID + "/config",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `"success"`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				name := "new-name"
				_, err := svc.UpdateConfig(ctx, testAppID, ApplicationUpdateConfigBody{Name: &name}, opts...)
				return err
			},
		},
		{
			name: "Deploys.Webhook.Create", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/deploys/webhook",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"url":"https://example.com"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Deploys().Webhook().Create(ctx, testAppID, ApplicationWebhookCreateBody{
					Owner: "o", RepoName: "r", RepoID: "1", AccountID: "a",
				}, opts...)
				return err
			},
		},
		{
			name: "Deploys.Webhook.Delete", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/deploys/webhook",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Deploys().Webhook().Delete(ctx, testAppID, opts...)
			},
		},
		{
			name: "Domains.List", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/domains",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Domains().List(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Domains.Add", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/domains",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Domains().Add(ctx, testAppID, ApplicationDomainAddBody{Hostname: "x.com"}, opts...)
				return err
			},
		},
		{
			name: "Domains.Update", wantMethod: http.MethodPatch, wantPath: "/v1/apps/" + testAppID + "/domains/" + testDomainID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Domains().Update(ctx, testAppID, testDomainID, ApplicationDomainUpdateBody{Primary: true}, opts...)
				return err
			},
		},
		{
			name: "Domains.Remove", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/domains/" + testDomainID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Domains().Remove(ctx, testAppID, testDomainID, opts...)
			},
		},
		{
			name: "Domains.DNS", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/domains/" + testDomainID + "/dns",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Domains().DNS(ctx, testAppID, testDomainID, opts...)
				return err
			},
		},
		{
			name: "Domains.Connect", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/domains/" + testDomainID + "/connect",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"url":"https://dns.example/apply"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Domains().Connect(ctx, testAppID, testDomainID, opts...)
				return err
			},
		},
		{
			name: "Network.PurgeCache", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/network/purge-cache",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Network().PurgeCache(ctx, testAppID, nil, opts...)
			},
		},
		{
			name: "Network.SetSubdomain", wantMethod: http.MethodPatch, wantPath: "/v1/apps/" + testAppID + "/network/subdomain",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"subdomain":"x"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().SetSubdomain(ctx, testAppID, "x", opts...)
				return err
			},
		},
		{
			name: "Network.Publish", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/network/publish",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"subdomain":"x","domains":[],"type":2}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().Publish(ctx, testAppID, "", opts...)
				return err
			},
		},
		{
			name: "Network.Unpublish", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/network/publish",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"subdomain":null,"domains":[],"type":2}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().Unpublish(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Delete", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Delete(ctx, testAppID, opts...)
			},
		},
		{
			name: "Envs.List", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/envs",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Envs().List(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Envs.Set", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/envs",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Envs().Set(ctx, testAppID, []ApplicationEnvironmentInput{{Key: "K", Value: "V"}}, opts...)
				return err
			},
		},
		{
			name: "Envs.Delete", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/envs/" + testEnvID,
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Envs().Delete(ctx, testAppID, testEnvID, opts...)
			},
		},
		{
			name: "Files.List", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/files",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Files().List(ctx, testAppID, ApplicationFileListParams{}, opts...)
				return err
			},
		},
		{
			name: "Files.Tree", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/files/tree",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `[{"type":"directory","name":"root","path":"/","last_modified":"2026-01-01T00:00:00Z","children":[{"type":"file","name":"main.go","path":"/main.go","last_modified":"2026-01-01T00:00:00Z"}]}]`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Files().Tree(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Files.Read", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/files/content",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"type":"base64","data":"aGk=","size":2,"last_modified":"2026-01-01T00:00:00Z"}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Files().Read(ctx, testAppID, ApplicationFileReadParams{Path: "main.py"}, opts...)
				return err
			},
		},
		{
			name: "Files.Write", wantMethod: http.MethodPut, wantPath: "/v1/apps/" + testAppID + "/files",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Files().Write(ctx, testAppID, ApplicationFileWriteBody{Path: "main.py"}, opts...)
			},
		},
		{
			name: "Files.Move", wantMethod: http.MethodPatch, wantPath: "/v1/apps/" + testAppID + "/files",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Files().Move(ctx, testAppID, ApplicationFileMoveBody{Path: "a.py", To: "b.py"}, opts...)
			},
		},
		{
			name: "Files.Delete", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/files",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Files().Delete(ctx, testAppID, ApplicationFileDeleteBody{Path: "main.py"}, opts...)
			},
		},
		{
			name: "Releases.List", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/releases",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"data":[],"total_count":0,"page":1,"limit":20,"total_pages":0,"has_next_page":false,"has_previous_page":false}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Releases().List(ctx, testAppID, ApplicationReleasesListParams{}, opts...)
				return err
			},
		},
		{
			name: "Releases.Log", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/releases/" + testReleaseID + "/log",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"content":null}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Releases().Log(ctx, testAppID, testReleaseID, opts...)
				return err
			},
		},
		{
			name: "Releases.Preview", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/releases/" + testReleaseID + "/rollback",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"environment_changes":[]}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Releases().Preview(ctx, testAppID, testReleaseID, opts...)
				return err
			},
		},
		{
			name: "Releases.Rollback", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/releases/" + testReleaseID + "/rollback",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, rollbackReleaseJSON) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Releases().Rollback(ctx, testAppID, testReleaseID, ApplicationRollbackBody{}, opts...)
				return err
			},
		},
	}

	if len(cases) != 38 {
		t.Fatalf("expected 38 table cases (42 routes minus Create/Files.Upload/Download/Realtime), got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			svc := newAppsService(fake)

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

func TestApps_FilesTree_DecodesArrayWithoutPathQuery(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"type":"directory","name":"root","path":"/","last_modified":"2026-01-01T00:00:00Z","children":[{"type":"file","name":"main.go","path":"/main.go","last_modified":"2026-01-01T00:00:00Z"}]}]`)

	got, err := newAppsService(fake).Files().Tree(context.Background(), testAppID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "root" || len(got[0].Children) != 1 || got[0].Children[0].Name != "main.go" {
		t.Fatalf("tree = %#v", got)
	}
	if fake.Requests[0].Query.Get("path") != "" {
		t.Fatalf("path query = %q, want omitted", fake.Requests[0].Query.Get("path"))
	}
}

func TestApps_DeploysWebhookGet_DecodesAPIShape(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app","branch":"main"}`)

	got, err := newAppsService(fake).Deploys().Webhook().Get(context.Background(), testAppID)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app","branch":"main"}`
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != want {
		t.Fatalf("webhook = %s, want %s", encoded, want)
	}
}

func TestApps_DeploysWebhookGet_DecodesBranch(t *testing.T) {
	release := "release"
	for _, tc := range []struct {
		name string
		resp string
		want *string
	}{
		{"null means main or master", `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app","branch":null}`, nil},
		{"set branch", `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app","branch":"release"}`, &release},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, tc.resp)
			got, err := newAppsService(fake).Deploys().Webhook().Get(context.Background(), testAppID)
			if err != nil {
				t.Fatal(err)
			}
			if (got.Branch == nil) != (tc.want == nil) || (tc.want != nil && *got.Branch != *tc.want) {
				t.Fatalf("branch = %v, want %v", got.Branch, tc.want)
			}
		})
	}
}

func TestApps_DeploysWebhookCreate_BranchIsOptionalInBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		branch string
		want   string
	}{
		{"without branch", "", `{"owner":"o","repo_name":"r","repo_id":"1","account_id":"a"}`},
		{"with branch", "release", `{"owner":"o","repo_name":"r","repo_id":"1","account_id":"a","branch":"release"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, `{"url":"https://example.com/hook"}`)
			_, err := newAppsService(fake).Deploys().Webhook().Create(context.Background(), testAppID, ApplicationWebhookCreateBody{
				Owner: "o", RepoName: "r", RepoID: "1", AccountID: "a", Branch: tc.branch,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := string(fake.Requests[0].Body); got != tc.want {
				t.Fatalf("body = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestApps_OwnerPlanIsNullablePlanName(t *testing.T) {
	typ := reflect.TypeOf(Application{}.OwnerPlan)
	if typ.Kind() != reflect.Ptr || typ.Elem().Name() != "UserPlanName" {
		t.Fatalf("application owner_plan type = %s, want *UserPlanName", typ)
	}
	var app Application
	if err := json.Unmarshal([]byte(`{"owner_plan":"enterprise-12"}`), &app); err != nil {
		t.Fatal(err)
	}
	if app.OwnerPlan == nil || *app.OwnerPlan != UserPlanNameEnterprise12 {
		t.Fatalf("owner_plan = %v", app.OwnerPlan)
	}
	if err := json.Unmarshal([]byte(`{"owner_plan":null}`), &app); err != nil || app.OwnerPlan != nil {
		t.Fatalf("null owner_plan = %v, err %v", app.OwnerPlan, err)
	}
}

func TestApps_DecodesDomainsList(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"id":"d1","hostname":"x.com","apex":true,"companion_of":null,"redirect_to":null,"redirect_status":308,"path_prefix":"/api","port":3000,"status":"active","ssl_status":"pending","created_at":"2026-10-20T12:00:00Z"},{"id":"d2","hostname":"www.x.com","apex":false,"companion_of":"d1","redirect_to":"d1","redirect_status":301,"path_prefix":null,"port":80,"status":"moved","ssl_status":"failed","created_at":"2026-10-20T12:00:00Z"}]`)
	got, err := newAppsService(fake).Domains().List(context.Background(), testAppID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("domains = %#v", got)
	}
	d := got[0]
	if d.Hostname != "x.com" || !d.Apex || d.CompanionOf != nil || d.RedirectStatus != ApplicationDomainRedirectStatusPermanentRedirect ||
		d.PathPrefix == nil || *d.PathPrefix != "/api" || d.Port != 3000 || d.Status != ApplicationDomainStatusActive || d.SSLStatus != ApplicationDomainSSLStatusPending {
		t.Fatalf("domain = %#v", d)
	}
	w := got[1]
	if w.CompanionOf == nil || *w.CompanionOf != "d1" || w.RedirectTo == nil || *w.RedirectTo != "d1" ||
		w.RedirectStatus != ApplicationDomainRedirectStatusMovedPermanently || w.Status != ApplicationDomainStatusMoved || w.SSLStatus != ApplicationDomainSSLStatusFailed {
		t.Fatalf("companion = %#v", w)
	}
}

func TestApps_ApplicationDecodesDomains(t *testing.T) {
	var app Application
	if err := json.Unmarshal([]byte(`{"id":"a","domains":[{"id":"d1","hostname":"x.com","port":80}]}`), &app); err != nil {
		t.Fatal(err)
	}
	if len(app.Domains) != 1 || app.Domains[0].Hostname != "x.com" {
		t.Fatalf("domains = %#v", app.Domains)
	}
	var pub ApplicationWebPublish
	if err := json.Unmarshal([]byte(`{"subdomain":null,"domains":["x.com"],"type":1}`), &pub); err != nil || len(pub.Domains) != 1 {
		t.Fatalf("publish = %#v, err %v", pub, err)
	}
}

func TestApps_DomainsAdd_Body(t *testing.T) {
	no := false
	cases := []struct {
		name string
		body ApplicationDomainAddBody
		want string
	}{
		{"hostname only", ApplicationDomainAddBody{Hostname: "x.com"}, `{"hostname":"x.com"}`},
		{"all fields", ApplicationDomainAddBody{Hostname: "x.com", WWW: &no, Port: 3000, PathPrefix: "/api", RedirectTo: "d1", RedirectStatus: ApplicationDomainRedirectStatusFound},
			`{"hostname":"x.com","www":false,"port":3000,"path_prefix":"/api","redirect_to":"d1","redirect_status":302}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, `[]`)
			if _, err := newAppsService(fake).Domains().Add(context.Background(), testAppID, tc.body); err != nil {
				t.Fatal(err)
			}
			req := fake.Requests[0]
			if string(req.Body) != tc.want || req.ContentType != "application/json" {
				t.Errorf("body=%s ct=%q, want %s", req.Body, req.ContentType, tc.want)
			}
		})
	}
}

func TestApps_DomainsUpdate_Body(t *testing.T) {
	port := 8080
	cases := []struct {
		name string
		body ApplicationDomainUpdateBody
		want string
	}{
		{"empty", ApplicationDomainUpdateBody{}, `{}`},
		{"port and status", ApplicationDomainUpdateBody{Port: &port, RedirectStatus: ApplicationDomainRedirectStatusTemporaryRedirect}, `{"port":8080,"redirect_status":307}`},
		{"clear nullable", ApplicationDomainUpdateBody{PathPrefix: NullableNull[string](), RedirectTo: NullableNull[string]()}, `{"path_prefix":null,"redirect_to":null}`},
		{"set nullable", ApplicationDomainUpdateBody{PathPrefix: NullableValue("/v1"), RedirectTo: NullableValue("d2")}, `{"path_prefix":"/v1","redirect_to":"d2"}`},
		{"primary", ApplicationDomainUpdateBody{Primary: true}, `{"primary":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, `[]`)
			if _, err := newAppsService(fake).Domains().Update(context.Background(), testAppID, testDomainID, tc.body); err != nil {
				t.Fatal(err)
			}
			if got := string(fake.Requests[0].Body); got != tc.want {
				t.Errorf("body=%s, want %s", got, tc.want)
			}
		})
	}
}

func TestApps_DomainsDNSAndConnect_Decode(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"type":"CNAME","name":"x.com","value":"edge.example","status":"pending"},{"type":"TXT","name":"_v.x.com","value":"abc","status":"active"}]`)
	fake.EnqueueJSON(200, `{"url":"https://dns.example/apply"}`)
	svc := newAppsService(fake).Domains()
	recs, err := svc.DNS(context.Background(), testAppID, testDomainID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Type != ApplicationDomainDNSRecordTypeCNAME || recs[0].Status != ApplicationDomainDNSRecordStatusPending ||
		recs[1].Type != ApplicationDomainDNSRecordTypeTXT || recs[1].Status != ApplicationDomainDNSRecordStatusActive {
		t.Fatalf("records = %#v", recs)
	}
	conn, err := svc.Connect(context.Background(), testAppID, testDomainID)
	if err != nil || conn.URL != "https://dns.example/apply" {
		t.Fatalf("connect = %#v, err %v", conn, err)
	}
}

func TestApps_DomainsPathEscapesIDs(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `null`)
	if err := newAppsService(fake).Domains().Remove(context.Background(), "a/b", "d c"); err != nil {
		t.Fatal(err)
	}
	if got := fake.Requests[0].Path; got != "/v1/apps/a%2Fb/domains/d%20c" {
		t.Fatalf("path = %s", got)
	}
}

func TestApps_RealtimeEventUsesContractEnum(t *testing.T) {
	if got := reflect.TypeOf(ApplicationRealtimeEvent{}.Event).Name(); got != "ApplicationRealtimeEventType" {
		t.Fatalf("realtime event type = %s, want ApplicationRealtimeEventType", got)
	}
}

func TestApps_WaitUntilReady(t *testing.T) {
	fast := &WaitOptions{Interval: time.Millisecond}
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"app-1","status":"building"}`)
	fake.EnqueueJSON(200, `{"id":"app-1","status":"up"}`)
	app, err := newAppsService(fake).WaitUntilReady(context.Background(), "app-1", fast)
	if err != nil || app.Status != ApplicationStatusUp {
		t.Fatalf("app = %#v, err = %v", app, err)
	}

	fake = vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"app-1","status":"down"}`)
	_, err = newAppsService(fake).WaitUntilReady(context.Background(), "app-1", fast)
	var down *AppNotReadyError
	if !errors.As(err, &down) || down.Code != "APP_DOWN" {
		t.Fatalf("err = %v, want APP_DOWN", err)
	}

	fake = vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"app-1","status":"down","last_build_error":"INSTALL_OUT_OF_MEMORY"}`)
	_, err = newAppsService(fake).WaitUntilReady(context.Background(), "app-1", fast)
	if !errors.As(err, &down) || down.Code != string(ApplicationBuildErrorInstallOutOfMemory) {
		t.Fatalf("err = %v, want INSTALL_OUT_OF_MEMORY", err)
	}

	fake = vertratest.NewFakeRestClient(t)
	for i := 0; i < 50; i++ {
		fake.EnqueueJSON(200, `{"id":"app-1","status":"building"}`)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err = newAppsService(fake).WaitUntilReady(ctx, "app-1", fast); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestApps_RestartBodyCarriesReinstallFlag(t *testing.T) {
	if got := reflect.TypeOf(ApplicationOperationResponse{}.Status).Name(); got != "ResourceOperationStatus" {
		t.Fatalf("application operation status = %s, want ResourceOperationStatus", got)
	}
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"status":"success"}`)
	reinstall := true
	if _, err := newAppsService(fake).Restart(context.Background(), testAppID, &ApplicationRestartBody{ReinstallDependencies: &reinstall}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fake.Requests[0].Body), `"reinstall_dependencies":true`) {
		t.Fatalf("restart body = %s", fake.Requests[0].Body)
	}
}

func TestApps_StatusShortRetainsStorageAndNullableNetwork(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"id":"app-1","cpu":"0%","ram":"0 MB","storage":"0 MB","network":null,"running":false,"uptime":null}]`)
	got, err := newAppsService(fake).StatusAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Storage == nil || *got[0].Storage != "0 MB" || got[0].Network != nil {
		t.Fatalf("status = %#v", got)
	}
}

func TestApps_StatusAcceptsNullNetwork(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"app-1","cpu":"0%","ram":"0 MB","status":"down","running":false,"storage":"0 MB","network":null,"uptime":0}`)
	got, err := newAppsService(fake).Status(context.Background(), testAppID)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.TypeOf(ApplicationStatusInfo{}.Network).Kind() != reflect.Ptr || got.ID != "app-1" {
		t.Fatalf("status network must be nullable; got %#v", got)
	}
}

func TestApps_StatusAllDecodesMultipleAndEmptyItems(t *testing.T) {
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
			got, err := newAppsService(fake).StatusAll(context.Background())
			if err != nil {
				t.Fatalf("StatusAll() error = %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("StatusAll() returned %d items, want %d", len(got), tc.want)
			}
		})
	}
}

// TestApps_Create_Multipart covers POST /v1/apps: the zip-file source, the
// envs JSON-in-a-text-field encoding, and that plain fields land as text
// parts.
func TestApps_Create_Multipart(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"myapp"}`)
	svc := newAppsService(fake)

	note := NullableValue("prod key")
	_, err := svc.Create(context.Background(), ApplicationCreateParams{
		File:        strings.NewReader("zip-bytes"),
		FileName:    "app.zip",
		Name:        "myapp",
		Memory:      512,
		Main:        "index.js",
		Version:     "latest",
		Language:    ApplicationLanguageDeno,
		WorkspaceID: "ws_1",
		Envs:        []ApplicationEnvironmentInput{{Key: "API_KEY", Value: "secret", Note: note}},
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
	if req.Path != "/v1/apps" {
		t.Errorf("path = %s, want /v1/apps", req.Path)
	}

	fields, files, fileNames := parseMultipartRequest(t, req)
	if files["file"] != "zip-bytes" {
		t.Errorf("file part content = %q, want %q", files["file"], "zip-bytes")
	}
	if fileNames["file"] != "app.zip" {
		t.Errorf("file part filename = %q, want app.zip", fileNames["file"])
	}
	if fields["language"] != "deno" {
		t.Errorf("language field = %q, want deno", fields["language"])
	}
	if fields["name"] != "myapp" {
		t.Errorf("name field = %q, want myapp", fields["name"])
	}
	if fields["memory"] != "512" {
		t.Errorf("memory field = %q, want 512", fields["memory"])
	}
	if fields["main"] != "index.js" {
		t.Errorf("main field = %q, want index.js", fields["main"])
	}
	if fields["workspace_id"] != "ws_1" {
		t.Errorf("workspace_id field = %q, want ws_1", fields["workspace_id"])
	}
	wantEnvs := `[{"key":"API_KEY","value":"secret","note":"prod key"}]`
	if fields["envs"] != wantEnvs {
		t.Errorf("envs field = %s, want %s", fields["envs"], wantEnvs)
	}
	if _, hasSnapshot := fields["snapshot_id"]; hasSnapshot {
		t.Errorf("snapshot_id field present, want absent when File is set")
	}
}

// TestApps_Create_FromSnapshot covers the snapshot_id source path (no file
// part at all).
func TestApps_Create_FromSnapshot(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"1","name":"myapp"}`)
	svc := newAppsService(fake)

	_, err := svc.Create(context.Background(), ApplicationCreateParams{
		SnapshotID: "snap_1",
		Name:       "myapp",
		Version:    "latest",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	fields, files, _ := parseMultipartRequest(t, fake.Requests[0])
	if fields["snapshot_id"] != "snap_1" {
		t.Errorf("snapshot_id field = %q, want snap_1", fields["snapshot_id"])
	}
	if _, hasFile := files["file"]; hasFile {
		t.Errorf("file part present, want absent when SnapshotID is set")
	}
}

// TestApps_Files_Upload_Multipart covers POST /v1/apps/:id/files/upload:
// field name "file", boundary parses cleanly, restart/workspace_id travel
// as query params / resolved opts (not form fields).
func TestApps_Files_Upload_Multipart(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-09-22T12:00:00.000Z","removed_directories":["tmp"]}`)
	svc := newAppsService(fake)

	restart := true
	got, err := svc.Files().Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("file-bytes"), FileName: "main.py", Restart: &restart}, rest.WithWorkspaceID("ws_9"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.AppID != testAppID || !got.UpdatedAt.Equal(time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)) || len(got.RemovedDirectories) != 1 || got.RemovedDirectories[0] != "tmp" {
		t.Fatalf("Upload response = %#v", got)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(fake.Requests))
	}
	req := fake.Requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.Path != "/v1/apps/"+testAppID+"/files/upload" {
		t.Errorf("path = %s", req.Path)
	}
	if got := req.Query.Get("restart"); got != "true" {
		t.Errorf("restart query = %q, want true", got)
	}
	if req.WorkspaceID != "ws_9" {
		t.Errorf("workspace id = %q, want ws_9", req.WorkspaceID)
	}

	files, fileContent, fileNames := parseMultipartRequest(t, req)
	_ = files
	if fileContent["file"] != "file-bytes" {
		t.Errorf("file content = %q, want file-bytes", fileContent["file"])
	}
	if fileNames["file"] != "main.py" {
		t.Errorf("file filename = %q, want main.py", fileNames["file"])
	}
}

// TestApps_Files_Upload_PathQuery checks that Path travels as the "path"
// query param and is omitted when empty.
func TestApps_Files_Upload_PathQuery(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-09-22T12:00:00.000Z"}`)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-09-22T12:00:00.000Z"}`)
	svc := newAppsService(fake)

	if _, err := svc.Files().Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("x"), FileName: "a.txt", Path: "src/lib"}); err != nil {
		t.Fatalf("Upload with path: %v", err)
	}
	if _, err := svc.Files().Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("x"), FileName: "a.txt"}); err != nil {
		t.Fatalf("Upload without path: %v", err)
	}
	if len(fake.Requests) != 2 {
		t.Fatalf("want 2 requests, got %d", len(fake.Requests))
	}
	if got := fake.Requests[0].Query.Get("path"); got != "src/lib" {
		t.Errorf("path query = %q, want src/lib", got)
	}
	if fake.Requests[1].Query.Has("path") {
		t.Errorf("path query present when Path is empty: %q", fake.Requests[1].Query.Get("path"))
	}
}

// TestApps_Files_Upload_Message checks that Message travels as a text part
// only when it is non-empty.
func TestApps_Files_Upload_Message(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-09-22T12:00:00.000Z"}`)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-09-22T12:00:00.000Z"}`)
	svc := newAppsService(fake)

	if _, err := svc.Files().Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("x"), FileName: "a.zip", Message: "fix: boot"}); err != nil {
		t.Fatalf("Upload with message: %v", err)
	}
	if _, err := svc.Files().Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("x"), FileName: "a.zip"}); err != nil {
		t.Fatalf("Upload without message: %v", err)
	}
	if len(fake.Requests) != 2 {
		t.Fatalf("want 2 requests, got %d", len(fake.Requests))
	}

	withFields, _, _ := parseMultipartRequest(t, fake.Requests[0])
	if withFields["message"] != "fix: boot" {
		t.Errorf("message field = %q, want fix: boot", withFields["message"])
	}
	withoutFields, _, _ := parseMultipartRequest(t, fake.Requests[1])
	if _, ok := withoutFields["message"]; ok {
		t.Errorf("message field present when Message is empty: %q", withoutFields["message"])
	}
}

// TestApps_Releases_List_DecodesPage checks the page envelope decodes, that
// page/limit are omitted when zero, and that WorkspaceID is forwarded.
func TestApps_Releases_List_DecodesPage(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"data":[{"id":"rel_2","app_id":"app_123","source":"github","status":"active","message":"fix: boot","author":"octo","github":{"app_id":"app_123","commit_id":"abc123","message":"fix: boot","pusher":"octo","branch":"main","created_at":"2026-10-10T12:00:00.000Z"},"files_changed":3,"rollback":{"available":false,"expires_at":"2026-10-12T12:00:00.000Z","from":null},"has_log":true,"build_duration_ms":4200,"first_started_at":null,"created_at":"2026-10-10T12:00:00.000Z"}],"total_count":6,"page":2,"limit":5,"total_pages":2,"has_next_page":false,"has_previous_page":true}`)
	fake.EnqueueJSON(200, `{"data":[],"total_count":0,"page":1,"limit":20,"total_pages":0,"has_next_page":false,"has_previous_page":false}`)
	svc := newAppsService(fake)

	got, err := svc.Releases().List(context.Background(), testAppID, ApplicationReleasesListParams{Page: 2, Limit: 5, WorkspaceID: "ws_9"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.TotalCount != 6 || got.Page != 2 || got.Limit != 5 || !got.HasPreviousPage || got.HasNextPage || len(got.Data) != 1 {
		t.Fatalf("page = %#v", got)
	}
	rel := got.Data[0]
	if rel.Source != ApplicationReleaseSourceGitHub || rel.Status != ApplicationReleaseStatusActive || rel.Message == nil || *rel.Message != "fix: boot" || rel.Github == nil || rel.Github.CommitID != "abc123" {
		t.Fatalf("release = %#v", rel)
	}
	if rel.Rollback.Available || rel.Rollback.ExpiresAt == nil || rel.Rollback.From != nil || rel.FilesChanged != 3 || !rel.HasLog || rel.BuildDurationMs == nil || *rel.BuildDurationMs != 4200 {
		t.Fatalf("release fields = %#v", rel)
	}

	if _, err := svc.Releases().List(context.Background(), testAppID, ApplicationReleasesListParams{}); err != nil {
		t.Fatalf("List without params: %v", err)
	}
	if len(fake.Requests) != 2 {
		t.Fatalf("want 2 requests, got %d", len(fake.Requests))
	}
	first := fake.Requests[0]
	if first.Method != http.MethodGet || first.Path != "/v1/apps/"+testAppID+"/releases" {
		t.Errorf("request = %s %s", first.Method, first.Path)
	}
	if first.Query.Get("page") != "2" || first.Query.Get("limit") != "5" {
		t.Errorf("query = %v, want page=2&limit=5", first.Query)
	}
	if first.WorkspaceID != "ws_9" {
		t.Errorf("workspace id = %q, want ws_9", first.WorkspaceID)
	}
	if fake.Requests[1].Query.Has("page") || fake.Requests[1].Query.Has("limit") {
		t.Errorf("page/limit sent when zero: %v", fake.Requests[1].Query)
	}
}

// TestApps_Releases_Log_DecodesNullableContent checks that a null content
// decodes to nil, a string decodes to a pointer, and the release id is
// path-escaped.
func TestApps_Releases_Log_DecodesNullableContent(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"content":null}`)
	fake.EnqueueJSON(200, `{"content":"boom\nstack"}`)
	svc := newAppsService(fake)

	none, err := svc.Releases().Log(context.Background(), testAppID, testReleaseID)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if none.Content != nil {
		t.Fatalf("Content = %q, want nil", *none.Content)
	}

	some, err := svc.Releases().Log(context.Background(), testAppID, "rel/x")
	if err != nil {
		t.Fatalf("Log with content: %v", err)
	}
	if some.Content == nil || *some.Content != "boom\nstack" {
		t.Fatalf("Content = %v, want boom\\nstack", some.Content)
	}
	if got := fake.Requests[1].Path; got != "/v1/apps/"+testAppID+"/releases/rel%2Fx/log" {
		t.Errorf("path = %s, want escaped release id", got)
	}
}

// TestApps_Download_Binary checks that Download streams the exact raw
// bytes of the response body with no {"response": ...} envelope parsing.
func TestApps_Download_Binary(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	raw := []byte("PK\x03\x04-this-is-not-json-{\"response\":true}")
	fake.EnqueueRaw(200, raw)
	svc := newAppsService(fake)

	body, err := svc.Download(context.Background(), testAppID)
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
	if req.Path != "/v1/apps/"+testAppID+"/download" {
		t.Errorf("path = %s", req.Path)
	}
}

// TestApps_Realtime_SSE drives Realtime over the fake rest.Client, checking
// the request shape (GET, path, since query param) and that EventStream
// parses a real SSE payload.
func TestApps_Realtime_SSE(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	body := "event: logs\ndata: hello world\nid: 1\n\n" +
		"event: system\ndata: line two\n\n"
	fake.EnqueueSSE(200, body)
	svc := newAppsService(fake)

	stream, err := svc.Realtime(context.Background(), testAppID, &ApplicationRealtimeParams{Since: 42})
	if err != nil {
		t.Fatalf("Realtime: %v", err)
	}
	defer stream.Close()

	req := fake.Requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", req.Method)
	}
	if req.Path != "/v1/apps/"+testAppID+"/realtime" {
		t.Errorf("path = %s", req.Path)
	}
	if got := req.Query.Get("since"); got != "42" {
		t.Errorf("since query = %q, want 42", got)
	}

	var events []ApplicationRealtimeEvent
	for stream.Next(context.Background()) {
		events = append(events, stream.Event())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream ended with error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Event != ApplicationRealtimeEventTypeLogs || events[0].Data != "hello world" || events[0].ID != "1" {
		t.Errorf("event[0] = %+v", events[0])
	}
	if events[1].Data != "line two" {
		t.Errorf("event[1] = %+v", events[1])
	}
	if events[1].Event != ApplicationRealtimeEventTypeSystem {
		t.Errorf("event[1].Event = %q, want system", events[1].Event)
	}
}

// TestApps_Realtime_SinceOmitted checks that since=0 omits the query param
// entirely, rather than sending since=0.
func TestApps_Realtime_SinceOmitted(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueSSE(200, "")
	svc := newAppsService(fake)

	stream, err := svc.Realtime(context.Background(), testAppID, nil)
	if err != nil {
		t.Fatalf("Realtime: %v", err)
	}
	defer stream.Close()

	if _, ok := fake.Requests[0].Query["since"]; ok {
		t.Errorf("since query present with value 0, want omitted")
	}
}

// pipeRoundTripper serves one response whose body is a caller-supplied
// io.ReadCloser, letting a test model a connection that goes quiet without
// closing — something FakeRestClient's strings.Reader-backed SSE body
// can't do, since a strings.Reader always EOFs immediately once exhausted
// rather than blocking. Used to drive Apps.Realtime's idle-timeout and
// context-cancellation behavior through a real rest.Client (the
// EventStream parsing mechanics themselves are already covered directly in
// the rest package's own sse tests).
type pipeRoundTripper struct{ body io.ReadCloser }

func (p *pipeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       p.body,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Request:    req,
	}, nil
}

func newRealtimeStreamService(t *testing.T, body io.ReadCloser) AppsService {
	t.Helper()
	rc, err := rest.NewClient("test-key", rest.WithHTTPClient(&http.Client{Transport: &pipeRoundTripper{body: body}}))
	if err != nil {
		t.Fatalf("rest.NewClient: %v", err)
	}
	return newAppsService(rc)
}

// TestApps_Realtime_IdleTimeout checks that rest.WithIdleTimeout passed to
// Apps.Realtime actually reaches the returned EventStream.
func TestApps_Realtime_IdleTimeout(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	svc := newRealtimeStreamService(t, pr)

	stream, err := svc.Realtime(context.Background(), testAppID, nil, rest.WithIdleTimeout(20*time.Millisecond))
	if err != nil {
		t.Fatalf("Realtime: %v", err)
	}
	defer stream.Close()

	if stream.Next(context.Background()) {
		t.Fatal("Next returned true, want false on idle timeout")
	}
	if stream.Err() != rest.ErrSSEIdleTimeout {
		t.Errorf("Err() = %v, want ErrSSEIdleTimeout", stream.Err())
	}
}

// TestApps_Realtime_ContextCancel checks that canceling the context passed
// to EventStream.Next stops an Apps.Realtime stream.
func TestApps_Realtime_ContextCancel(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	svc := newRealtimeStreamService(t, pr)

	stream, err := svc.Realtime(context.Background(), testAppID, nil)
	if err != nil {
		t.Fatalf("Realtime: %v", err)
	}
	defer stream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- stream.Next(ctx) }()
	cancel()

	select {
	case ok := <-done:
		if ok {
			t.Fatal("Next returned true, want false on context cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Next did not return after context cancel")
	}
	if stream.Err() != context.Canceled {
		t.Errorf("Err() = %v, want context.Canceled", stream.Err())
	}
}

func TestApplicationFileContentBytes(t *testing.T) {
	var c ApplicationFileContent
	if err := json.Unmarshal([]byte(`{"type":"base64","data":"aGk=","size":2,"last_modified":"2026-01-01T00:00:00Z"}`), &c); err != nil {
		t.Fatal(err)
	}
	got, err := c.Bytes()
	if err != nil || string(got) != "hi" || c.Size != 2 || c.LastModified.Year() != 2026 {
		t.Fatalf("content = %+v, bytes = %q, err = %v", c, got, err)
	}
}

func TestApps_Releases_Preview_DecodesChangesWithoutValues(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"environment_changes":[{"key":"A","change":"added"},{"key":"B","change":"modified"},{"key":"C","change":"removed"}]}`)
	got, err := newAppsService(fake).Releases().Preview(context.Background(), testAppID, "rel/x", rest.WithWorkspaceID("ws_2"))
	if err != nil {
		t.Fatal(err)
	}
	want := []ApplicationEnvironmentChangeItem{
		{"A", ApplicationEnvironmentChangeAdded}, {"B", ApplicationEnvironmentChangeModified}, {"C", ApplicationEnvironmentChangeRemoved},
	}
	if !reflect.DeepEqual(got.EnvironmentChanges, want) {
		t.Fatalf("changes = %#v", got.EnvironmentChanges)
	}
	req := fake.Requests[0]
	if req.Path != "/v1/apps/"+testAppID+"/releases/rel%2Fx/rollback" || req.WorkspaceID != "ws_2" {
		t.Errorf("request = %s ws=%q query=%v", req.Path, req.WorkspaceID, req.Query)
	}
}

func TestApps_Releases_Rollback_Body(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name string
		body ApplicationRollbackBody
		want string
	}{
		{"absent", ApplicationRollbackBody{}, `{}`},
		{"true", ApplicationRollbackBody{RestoreEnvironment: &yes}, `{"restore_environment":true}`},
		{"false", ApplicationRollbackBody{RestoreEnvironment: &no}, `{"restore_environment":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, rollbackReleaseJSON)
			rel, err := newAppsService(fake).Releases().Rollback(context.Background(), testAppID, testReleaseID, tc.body, rest.WithWorkspaceID("ws_3"))
			if err != nil {
				t.Fatal(err)
			}
			req := fake.Requests[0]
			if string(req.Body) != tc.want || req.ContentType != "application/json" || req.WorkspaceID != "ws_3" {
				t.Errorf("body=%s ct=%q ws=%q, want body %s", req.Body, req.ContentType, req.WorkspaceID, tc.want)
			}
			if rel.Source != ApplicationReleaseSourceRollback || rel.Rollback.From == nil || *rel.Rollback.From != testReleaseID {
				t.Errorf("release = %#v", rel)
			}
		})
	}
}

func TestApps_Application_DecodesDeploy(t *testing.T) {
	var app Application
	if err := json.Unmarshal([]byte(`{"deploy":{"zero_downtime":"on","zero_downtime_active":true,"auto_rollback":true,"health_check_path":"/up"}}`), &app); err != nil {
		t.Fatal(err)
	}
	d := app.Deploy
	if d.ZeroDowntime != ApplicationZeroDowntimeOn || !d.ZeroDowntimeActive || !d.AutoRollback || d.HealthCheckPath == nil || *d.HealthCheckPath != "/up" {
		t.Fatalf("deploy = %#v", d)
	}
	app = Application{}
	if err := json.Unmarshal([]byte(`{"deploy":{"zero_downtime":"auto","zero_downtime_active":false,"auto_rollback":false,"health_check_path":null}}`), &app); err != nil {
		t.Fatal(err)
	}
	if app.Deploy.ZeroDowntime != ApplicationZeroDowntimeAuto || app.Deploy.HealthCheckPath != nil {
		t.Fatalf("deploy = %#v", app.Deploy)
	}
}

func TestApps_UpdateConfig_DeployAbsentNullValue(t *testing.T) {
	off, no := ApplicationZeroDowntimeOff, false
	for _, tc := range []struct {
		name string
		body ApplicationUpdateConfigBody
		want string
	}{
		{"deploy absent", ApplicationUpdateConfigBody{}, `{}`},
		{"empty deploy", ApplicationUpdateConfigBody{Deploy: &ApplicationDeploySettingsUpdate{}}, `{"deploy":{}}`},
		{"path absent", ApplicationUpdateConfigBody{Deploy: &ApplicationDeploySettingsUpdate{ZeroDowntime: &off, AutoRollback: &no}}, `{"deploy":{"zero_downtime":"off","auto_rollback":false}}`},
		{"path null", ApplicationUpdateConfigBody{Deploy: &ApplicationDeploySettingsUpdate{HealthCheckPath: NullableNull[string]()}}, `{"deploy":{"health_check_path":null}}`},
		{"path value", ApplicationUpdateConfigBody{Deploy: &ApplicationDeploySettingsUpdate{HealthCheckPath: NullableValue("/health")}}, `{"deploy":{"health_check_path":"/health"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, `"success"`)
			if _, err := newAppsService(fake).UpdateConfig(context.Background(), testAppID, tc.body); err != nil {
				t.Fatal(err)
			}
			req := fake.Requests[0]
			if req.Method != http.MethodPatch || string(req.Body) != tc.want {
				t.Errorf("%s body=%s, want %s", req.Method, req.Body, tc.want)
			}
		})
	}
}

func TestApps_Network_PublishWithPort_SendsPortOnlyWhenSet(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"subdomain":"web-1","domains":[]}`)
	fake.EnqueueJSON(200, `{"subdomain":"web-1","domains":[]}`)

	svc := newAppsService(fake).Network()
	port := 3000
	if _, err := svc.PublishWithPort(context.Background(), testAppID, "", &port); err != nil {
		t.Fatalf("PublishWithPort: %v", err)
	}
	if _, err := svc.Publish(context.Background(), testAppID, ""); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	withPort := decodeJSONBody(t, fake.Requests[0])
	if withPort["port"] != float64(3000) {
		t.Errorf("port = %v, want 3000", withPort["port"])
	}
	plain := decodeJSONBody(t, fake.Requests[1])
	if _, ok := plain["port"]; ok {
		t.Errorf("Publish sent port = %v, want it omitted", plain["port"])
	}
}
