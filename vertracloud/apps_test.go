package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
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
	testAppID = "app_123"
	testEnvID = "env_456"
)

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
// route (32 of the 36), that the Go method issues the expected HTTP method
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
			name: "Network.CustomDomain.Get", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/network/custom",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"domain":"x.com"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().CustomDomain().Get(ctx, testAppID, opts...)
				return err
			},
		},
		{
			name: "Network.DNS", wantMethod: http.MethodGet, wantPath: "/v1/apps/" + testAppID + "/network/dns",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().DNS(ctx, testAppID, opts...)
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
			name: "Network.CustomDomain.Set", wantMethod: http.MethodPost, wantPath: "/v1/apps/" + testAppID + "/network/custom",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `{"domain":"x.com"}`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().CustomDomain().Set(ctx, testAppID, "x.com", opts...)
				return err
			},
		},
		{
			name: "Network.CustomDomain.Remove", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/network/custom",
			enqueue: func(fake *vertratest.FakeRestClient) { fake.EnqueueJSON(200, `null`) },
			invoke: func(ctx context.Context, svc AppsService) error {
				return svc.Network().CustomDomain().Remove(ctx, testAppID, opts...)
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
				fake.EnqueueJSON(200, `{"subdomain":"x","custom_domain":null,"type":2}`)
			},
			invoke: func(ctx context.Context, svc AppsService) error {
				_, err := svc.Network().Publish(ctx, testAppID, "", opts...)
				return err
			},
		},
		{
			name: "Network.Unpublish", wantMethod: http.MethodDelete, wantPath: "/v1/apps/" + testAppID + "/network/publish",
			enqueue: func(fake *vertratest.FakeRestClient) {
				fake.EnqueueJSON(200, `{"subdomain":null,"custom_domain":null,"type":2}`)
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
	}

	if len(cases) != 32 {
		t.Fatalf("expected 32 table cases (36 routes minus Create/Files.Upload/Download/Realtime), got %d", len(cases))
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
	fake.EnqueueJSON(200, `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app"}`)

	got, err := newAppsService(fake).Deploys().Webhook().Get(context.Background(), testAppID)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"webhook_url":"https://example.com/hook","repo_owner":"acme","repo_name":"app"}`
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != want {
		t.Fatalf("webhook = %s, want %s", encoded, want)
	}
}

func TestApps_DNSRecordUsesContractEnums(t *testing.T) {
	if got := reflect.TypeOf(ApplicationDNSRecord{}.Type).Name(); got != "ApplicationDNSRecordType" {
		t.Fatalf("DNS record type = %s, want ApplicationDNSRecordType", got)
	}
	if got := reflect.TypeOf(ApplicationDNSRecord{}.Status).Name(); got != "ApplicationDNSRecordStatus" {
		t.Fatalf("DNS record status = %s, want ApplicationDNSRecordStatus", got)
	}
	if got := reflect.TypeOf(Application{}.OwnerPlanID).Name(); got != "UserPlan" {
		t.Fatalf("application owner_plan_id type = %s, want UserPlan", got)
	}
}

func TestApps_RealtimeEventUsesContractEnum(t *testing.T) {
	if got := reflect.TypeOf(ApplicationRealtimeEvent{}.Event).Name(); got != "ApplicationRealtimeEventType" {
		t.Fatalf("realtime event type = %s, want ApplicationRealtimeEventType", got)
	}
}

func TestApps_CreatePreservesRemovedDirectories(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"app-1","name":"demo","removed_directories":[".cache"]}`)
	app, err := newAppsService(fake).Create(context.Background(), ApplicationCreateParams{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(app.RemovedDirectories) != 1 || app.RemovedDirectories[0] != ".cache" {
		t.Fatalf("removed_directories = %#v", app.RemovedDirectories)
	}
}

func TestApps_RestartBodyCarriesCleanupLanguage(t *testing.T) {
	if got := reflect.TypeOf(ApplicationOperationResponse{}.Status).Name(); got != "ResourceOperationStatus" {
		t.Fatalf("application operation status = %s, want ResourceOperationStatus", got)
	}
	field, ok := reflect.TypeOf(ApplicationRestartBody{}).FieldByName("CleanupOldRuntimeLanguage")
	if !ok || field.Tag.Get("json") != "cleanup_old_runtime_language,omitempty" {
		t.Fatalf("cleanup_old_runtime_language field missing or incorrectly tagged")
	}
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"status":"success"}`)
	language := ApplicationLanguagePython
	if _, err := newAppsService(fake).Restart(context.Background(), testAppID, &ApplicationRestartBody{CleanupOldRuntimeLanguage: &language}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fake.Requests[0].Body), `"cleanup_old_runtime_language":"python"`) {
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
	autoRestart := true
	_, err := svc.Create(context.Background(), ApplicationCreateParams{
		File:        strings.NewReader("zip-bytes"),
		FileName:    "app.zip",
		Name:        "myapp",
		Memory:      512,
		Main:        "index.js",
		Version:     "latest",
		AutoRestart: &autoRestart,
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
	if fields["name"] != "myapp" {
		t.Errorf("name field = %q, want myapp", fields["name"])
	}
	if fields["memory"] != "512" {
		t.Errorf("memory field = %q, want 512", fields["memory"])
	}
	if fields["main"] != "index.js" {
		t.Errorf("main field = %q, want index.js", fields["main"])
	}
	if fields["autorestart"] != "true" {
		t.Errorf("autorestart field = %q, want true", fields["autorestart"])
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
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-09-22T12:00:00.000Z","missing_dependencies":["axios"],"removed_directories":["tmp"]}`)
	svc := newAppsService(fake)

	restart := true
	got, err := svc.Files().Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("file-bytes"), FileName: "main.py", Restart: &restart}, rest.WithWorkspaceID("ws_9"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.AppID != testAppID || !got.UpdatedAt.Equal(time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)) || len(got.MissingDependencies) != 1 || got.MissingDependencies[0] != "axios" || len(got.RemovedDirectories) != 1 || got.RemovedDirectories[0] != "tmp" {
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
