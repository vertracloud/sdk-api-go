package vertracloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const testImageID = "img_123"

// decodeJSONBody parses a captured JSON request body into a generic map.
func decodeJSONBody(t *testing.T, req vertratest.CapturedCall) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(req.Body, &m); err != nil {
		t.Fatalf("decode request body %q: %v", req.Body, err)
	}
	return m
}

// TestImages_RouteMatrix checks that each of the 13 image routes (the 12
// /v1/images routes plus POST /v1/apps/image) issues the expected HTTP method
// and path, and that WithWorkspaceID is forwarded.
func TestImages_RouteMatrix(t *testing.T) {
	opts := []rest.RequestOpt{rest.WithWorkspaceID("ws_1")}
	imagePath := "/v1/images/" + testImageID

	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(fake *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, fake *vertratest.FakeRestClient) error
	}

	cases := []routeCase{
		{
			name: "List", wantMethod: http.MethodGet, wantPath: "/v1/images",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).List(ctx, opts...)
				return err
			},
		},
		{
			name: "Get", wantMethod: http.MethodGet, wantPath: imagePath,
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"id":"img_123"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Get(ctx, testImageID, opts...)
				return err
			},
		},
		{
			name: "Create", wantMethod: http.MethodPost, wantPath: "/v1/images",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(202, `{"id":"img_123"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Create(ctx, ImageCreateParams{Name: "n", File: strings.NewReader("zip")}, opts...)
				return err
			},
		},
		{
			name: "Update", wantMethod: http.MethodPatch, wantPath: imagePath,
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"id":"img_123"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				name := "n2"
				_, err := newImagesService(f).Update(ctx, testImageID, ImageUpdateBody{Name: &name}, opts...)
				return err
			},
		},
		{
			name: "Rebuild", wantMethod: http.MethodPut, wantPath: imagePath,
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(202, `{"id":"img_123"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Rebuild(ctx, testImageID, ImageRebuildParams{File: strings.NewReader("zip")}, opts...)
				return err
			},
		},
		{
			name: "Delete", wantMethod: http.MethodDelete, wantPath: imagePath,
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"deleted_apps":[]}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Delete(ctx, testImageID, ImageDeleteBody{ConfirmName: "n"}, opts...)
				return err
			},
		},
		{
			name: "Versions", wantMethod: http.MethodGet, wantPath: imagePath + "/versions",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Versions(ctx, testImageID, opts...)
				return err
			},
		},
		{
			name: "Logs", wantMethod: http.MethodGet, wantPath: imagePath + "/logs",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueSSE(200, "event: log\ndata: line\n\n") },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				stream, err := newImagesService(f).Logs(ctx, testImageID, nil, opts...)
				if err != nil {
					return err
				}
				return stream.Close()
			},
		},
		{
			name: "Context", wantMethod: http.MethodGet, wantPath: imagePath + "/context",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"url":"https://example.com/ctx.zip"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Context(ctx, testImageID, opts...)
				return err
			},
		},
		{
			name: "LinkGitHub", wantMethod: http.MethodPut, wantPath: imagePath + "/github",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"id":"img_123"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).LinkGitHub(ctx, testImageID, ImageGitHubLinkBody{Repository: "a/b", Branch: "main"}, opts...)
				return err
			},
		},
		{
			name: "UnlinkGitHub", wantMethod: http.MethodDelete, wantPath: imagePath + "/github",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(200, `{"id":"img_123"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).UnlinkGitHub(ctx, testImageID, opts...)
				return err
			},
		},
		{
			name: "Check", wantMethod: http.MethodPost, wantPath: "/v1/images/check",
			enqueue: func(f *vertratest.FakeRestClient) {
				f.EnqueueJSON(200, `{"refusals":[],"warnings":[],"suggested_port":80}`)
			},
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newImagesService(f).Check(ctx, ImageCheckBody{Dockerfile: "FROM scratch", Files: []string{"Dockerfile"}}, opts...)
				return err
			},
		},
		{
			name: "Apps.CreateFromImage", wantMethod: http.MethodPost, wantPath: "/v1/apps/image",
			enqueue: func(f *vertratest.FakeRestClient) { f.EnqueueJSON(202, `{"id":"app_1"}`) },
			invoke: func(ctx context.Context, f *vertratest.FakeRestClient) error {
				_, err := newAppsService(f).CreateFromImage(ctx, ApplicationCreateFromImageParams{ImageID: testImageID, Name: "w", Memory: 256}, opts...)
				return err
			},
		},
	}

	if len(cases) != 13 {
		t.Fatalf("expected 13 table cases (12 image routes + POST /v1/apps/image), got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)

			if err := tc.invoke(context.Background(), fake); err != nil {
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

// TestImages_Create_FileSourceMultipart covers the upload source: a "file"
// part (default name image.zip), and name and workspace_id as text fields.
// No GitHub field is sent.
func TestImages_Create_FileSourceMultipart(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"img_123","name":"node-web","owner":"account","status":"building","apps":[]}`)

	img, err := newImagesService(fake).Create(context.Background(), ImageCreateParams{
		Name:        "node-web",
		File:        strings.NewReader("zip-bytes"),
		WorkspaceID: "ws_1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if img.Status != CustomImageStatusBuilding {
		t.Errorf("status = %q, want building", img.Status)
	}

	req := fake.Requests[0]
	if req.Method != http.MethodPost || req.Path != "/v1/images" {
		t.Errorf("request = %s %s, want POST /v1/images", req.Method, req.Path)
	}

	fields, files, fileNames := parseMultipartRequest(t, req)
	if files["file"] != "zip-bytes" {
		t.Errorf("file part = %q, want zip-bytes", files["file"])
	}
	if fileNames["file"] != "image.zip" {
		t.Errorf("file part filename = %q, want image.zip", fileNames["file"])
	}
	if fields["name"] != "node-web" {
		t.Errorf("name field = %q, want node-web", fields["name"])
	}
	if fields["workspace_id"] != "ws_1" {
		t.Errorf("workspace_id field = %q, want ws_1", fields["workspace_id"])
	}
	for _, k := range []string{"github_repository", "github_branch"} {
		if _, ok := fields[k]; ok {
			t.Errorf("%s field present, want absent for the file source", k)
		}
	}
}

// TestImages_Create_GitHubSourceMultipart covers the GitHub source: no file
// part, github_repository and github_branch as text fields.
func TestImages_Create_GitHubSourceMultipart(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"img_123","name":"api","status":"building"}`)

	_, err := newImagesService(fake).Create(context.Background(), ImageCreateParams{
		Name:             "api",
		GitHubRepository: "acme/api",
		GitHubBranch:     "main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	fields, files, _ := parseMultipartRequest(t, fake.Requests[0])
	if _, ok := files["file"]; ok {
		t.Errorf("file part present, want absent for the GitHub source")
	}
	if fields["github_repository"] != "acme/api" {
		t.Errorf("github_repository = %q, want acme/api", fields["github_repository"])
	}
	if fields["github_branch"] != "main" {
		t.Errorf("github_branch = %q, want main", fields["github_branch"])
	}
	if _, ok := fields["workspace_id"]; ok {
		t.Errorf("workspace_id present, want absent when not set")
	}
}

// TestImages_Create_GitHubWithoutBranch checks that, without a branch, the
// github_branch field is not sent and the default branch applies server-side.
func TestImages_Create_GitHubWithoutBranch(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"img_123","name":"api","status":"building","github":null}`)

	img, err := newImagesService(fake).Create(context.Background(), ImageCreateParams{
		Name:             "api",
		GitHubRepository: "acme/api",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if img.GitHub != nil {
		t.Errorf("github = %+v, want nil for a build with no link", img.GitHub)
	}

	fields, files, _ := parseMultipartRequest(t, fake.Requests[0])
	if _, ok := files["file"]; ok {
		t.Errorf("file part present, want absent for the GitHub source")
	}
	if fields["github_repository"] != "acme/api" {
		t.Errorf("github_repository = %q, want acme/api", fields["github_repository"])
	}
	if _, ok := fields["github_branch"]; ok {
		t.Errorf("github_branch present, want absent when no branch is given")
	}
}

// TestImages_Rebuild_Multipart checks that Rebuild sends the whole context in
// a "file" part of a PUT.
func TestImages_Rebuild_Multipart(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"img_123","status":"building"}`)

	img, err := newImagesService(fake).Rebuild(context.Background(), testImageID, ImageRebuildParams{
		File: strings.NewReader("new-zip"),
	})
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if img.Status != CustomImageStatusBuilding {
		t.Errorf("status = %q, want building", img.Status)
	}

	req := fake.Requests[0]
	if req.Method != http.MethodPut || req.Path != "/v1/images/"+testImageID {
		t.Errorf("request = %s %s, want PUT /v1/images/%s", req.Method, req.Path, testImageID)
	}
	_, files, fileNames := parseMultipartRequest(t, req)
	if files["file"] != "new-zip" {
		t.Errorf("file part = %q, want new-zip", files["file"])
	}
	if fileNames["file"] != "image.zip" {
		t.Errorf("file part filename = %q, want image.zip", fileNames["file"])
	}
}

// TestImages_Rebuild_RequiresFile checks that Rebuild refuses to send a
// request without a build context.
func TestImages_Rebuild_RequiresFile(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	if _, err := newImagesService(fake).Rebuild(context.Background(), testImageID, ImageRebuildParams{}); err == nil {
		t.Fatal("Rebuild without File returned no error")
	}
	if len(fake.Requests) != 0 {
		t.Errorf("sent %d requests, want none", len(fake.Requests))
	}
}

// TestImages_Delete_SendsConfirmName checks the body of DELETE and decodes the
// apps removed with the image.
func TestImages_Delete_SendsConfirmName(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"deleted_apps":["app_1","app_2"]}`)

	res, err := newImagesService(fake).Delete(context.Background(), testImageID, ImageDeleteBody{ConfirmName: "node-web"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !reflect.DeepEqual(res.DeletedApps, []string{"app_1", "app_2"}) {
		t.Errorf("deleted apps = %v", res.DeletedApps)
	}
	body := decodeJSONBody(t, fake.Requests[0])
	if body["confirm_name"] != "node-web" {
		t.Errorf("confirm_name = %v, want node-web", body["confirm_name"])
	}
}

// TestImages_Update_Body checks that only the given fields are sent and that
// NullableNull sends an explicit null to clear the description.
func TestImages_Update_Body(t *testing.T) {
	name := "node-web-2"
	cases := []struct {
		name string
		body ImageUpdateBody
		want string
	}{
		{"name only", ImageUpdateBody{Name: &name}, `{"name":"node-web-2"}`},
		{"description only", ImageUpdateBody{Description: NullableValue("Base Node 22")}, `{"description":"Base Node 22"}`},
		{"clear description", ImageUpdateBody{Description: NullableNull[string]()}, `{"description":null}`},
		{"both", ImageUpdateBody{Name: &name, Description: NullableValue("x")}, `{"name":"node-web-2","description":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			fake.EnqueueJSON(200, `{"id":"img_123","name":"node-web-2","description":"Base Node 22"}`)
			img, err := newImagesService(fake).Update(context.Background(), testImageID, tc.body)
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			if got := string(fake.Requests[0].Body); got != tc.want {
				t.Errorf("body = %s, want %s", got, tc.want)
			}
			if fake.Requests[0].ContentType != "application/json" {
				t.Errorf("content type = %q", fake.Requests[0].ContentType)
			}
			if img.Description == nil || *img.Description != "Base Node 22" {
				t.Errorf("description = %v", img.Description)
			}
		})
	}
}

// TestImages_Update_NameInUseIsAPIError checks that a duplicate name comes back as a 400 VALIDATION_ERROR.
func TestImages_Update_NameInUseIsAPIError(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(400, `{"code":"VALIDATION_ERROR","details":{"path":"name"}}`)
	name := "taken"
	_, err := newImagesService(fake).Update(context.Background(), testImageID, ImageUpdateBody{Name: &name})
	apiErr, ok := rest.AsAPIError(err)
	if !ok || apiErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("err = %v", err)
	}
}

// TestImages_Get_DecodesDescription checks the nullable description.
func TestImages_Get_DecodesDescription(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"img_123","description":null}`)
	fake.EnqueueJSON(200, `{"id":"img_123","description":"Base"}`)
	svc := newImagesService(fake)
	none, err := svc.Get(context.Background(), testImageID)
	if err != nil || none.Description != nil {
		t.Fatalf("null description = %v, err %v", none.Description, err)
	}
	some, err := svc.Get(context.Background(), testImageID)
	if err != nil || some.Description == nil || *some.Description != "Base" {
		t.Fatalf("description = %v, err %v", some.Description, err)
	}
}

// TestImages_Create_SendsDescriptionWhenSet checks the optional description text field.
func TestImages_Create_SendsDescriptionWhenSet(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"img_123"}`)
	fake.EnqueueJSON(202, `{"id":"img_123"}`)
	svc := newImagesService(fake)
	if _, err := svc.Create(context.Background(), ImageCreateParams{Name: "n", Description: "Base Node 22", GitHubRepository: "a/b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), ImageCreateParams{Name: "n", GitHubRepository: "a/b"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fake.Requests[0].Body), "name=\"description\"\r\n\r\nBase Node 22") {
		t.Errorf("description part missing: %q", fake.Requests[0].Body)
	}
	if strings.Contains(string(fake.Requests[1].Body), "description") {
		t.Errorf("description sent when empty: %q", fake.Requests[1].Body)
	}
}

// TestImages_Get_DecodesNullableFields checks a failed image with no built
// version, a linked repository and an outdated app.
func TestImages_Get_DecodesNullableFields(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"img_123","name":"node-web","owner":"account","status":"failed","error":"DOCKERFILE_REFUSED","digest":null,"uncompressed_bytes":null,"layer_count":null,"suggested_port":null,"apps":[{"id":"app_1","name":"web","version_id":null,"outdated":true}],"github":{"repository":"acme/api","branch":"main"},"current_version_id":null,"created_at":"2026-10-11T12:00:00.000Z","updated_at":"2026-10-11T12:05:00.000Z"}`)

	img, err := newImagesService(fake).Get(context.Background(), testImageID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if img.Status != CustomImageStatusFailed || img.Owner != CustomImageOwnerAccount {
		t.Errorf("status/owner = %q/%q", img.Status, img.Owner)
	}
	if img.Error == nil || *img.Error != "DOCKERFILE_REFUSED" {
		t.Errorf("error = %v, want DOCKERFILE_REFUSED", img.Error)
	}
	if img.Digest != nil || img.UncompressedBytes != nil || img.LayerCount != nil || img.CurrentVersionID != nil {
		t.Errorf("nullable build fields should be nil before a successful build: %+v", img)
	}
	if len(img.Apps) != 1 || img.Apps[0].VersionID != nil || !img.Apps[0].Outdated {
		t.Errorf("apps = %+v", img.Apps)
	}
	if img.GitHub == nil || img.GitHub.Repository != "acme/api" || img.GitHub.Branch != "main" {
		t.Errorf("github = %+v", img.GitHub)
	}
	if img.CreatedAt.Year() != 2026 || img.UpdatedAt.Minute() != 5 {
		t.Errorf("timestamps = %v / %v", img.CreatedAt, img.UpdatedAt)
	}
}

// TestImages_Get_DecodesSuggestedPort checks that suggested_port decodes to a
// number when the image has one, and stays nil for images that predate it.
func TestImages_Get_DecodesSuggestedPort(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"img_123","name":"node-web","owner":"account","status":"success","error":null,"digest":"sha256:abc","uncompressed_bytes":1024,"layer_count":3,"suggested_port":3000,"apps":[],"github":null,"current_version_id":"ver_1","created_at":"2026-10-11T12:00:00.000Z","updated_at":"2026-10-11T12:05:00.000Z"}`)
	img, err := newImagesService(fake).Get(context.Background(), testImageID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if img.SuggestedPort == nil || *img.SuggestedPort != 3000 {
		t.Errorf("suggested_port = %v, want 3000", img.SuggestedPort)
	}

	fake.EnqueueJSON(200, `{"id":"img_123","name":"node-web","owner":"account","status":"success","error":null,"digest":"sha256:abc","uncompressed_bytes":1024,"layer_count":3,"suggested_port":null,"apps":[],"github":null,"current_version_id":"ver_1","created_at":"2026-10-11T12:00:00.000Z","updated_at":"2026-10-11T12:05:00.000Z"}`)
	img, err = newImagesService(fake).Get(context.Background(), testImageID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if img.SuggestedPort != nil {
		t.Errorf("suggested_port = %v, want nil for an old image", *img.SuggestedPort)
	}
}

// TestImages_Versions_DecodesSourceAndMessage checks the enum and the nullable
// message of each version.
func TestImages_Versions_DecodesSourceAndMessage(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `[{"id":"ver_2","digest":"sha256:abc","source":"commit","message":null,"created_at":"2026-10-11T12:00:00.000Z","in_use":true},{"id":"ver_1","digest":"sha256:def","source":"github","message":"fix port","created_at":"2026-10-10T12:00:00.000Z","in_use":false}]`)

	versions, err := newImagesService(fake).Versions(context.Background(), testImageID)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
	if versions[0].Source != CustomImageBuildSourceCommit || versions[0].Message != nil || !versions[0].InUse {
		t.Errorf("versions[0] = %+v", versions[0])
	}
	if versions[1].Source != CustomImageBuildSourceGitHub || versions[1].Message == nil || *versions[1].Message != "fix port" {
		t.Errorf("versions[1] = %+v", versions[1])
	}
}

// TestImages_Logs_StreamsLinesAndSendsVersionID checks that each log event is
// one line, and that version_id is sent when set.
func TestImages_Logs_StreamsLinesAndSendsVersionID(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueSSE(200, "event: log\ndata: Step 1/3 : FROM node:22\n\nevent: log\ndata: Successfully built\n\n")

	stream, err := newImagesService(fake).Logs(context.Background(), testImageID, &ImageLogsParams{VersionID: "ver_1"})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer stream.Close()

	req := fake.Requests[0]
	if req.Method != http.MethodGet || req.Path != "/v1/images/"+testImageID+"/logs" {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}
	if got := req.Query.Get("version_id"); got != "ver_1" {
		t.Errorf("version_id query = %q, want ver_1", got)
	}

	var lines []string
	for stream.Next(context.Background()) {
		lines = append(lines, stream.Line())
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream ended with error: %v", err)
	}
	want := []string{"Step 1/3 : FROM node:22", "Successfully built"}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("lines = %q, want %q", lines, want)
	}
}

// TestImages_Logs_OmitsVersionIDWhenNil checks that no query is sent for the
// latest build.
func TestImages_Logs_OmitsVersionIDWhenNil(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueSSE(200, "event: log\ndata: x\n\n")

	stream, err := newImagesService(fake).Logs(context.Background(), testImageID, nil)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer stream.Close()

	if len(fake.Requests[0].Query) != 0 {
		t.Errorf("query = %v, want none", fake.Requests[0].Query)
	}
}

// TestImages_Context_DecodesURL checks the temporary download URL.
func TestImages_Context_DecodesURL(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"url":"https://example.com/ctx.zip?sig=x"}`)

	ctxRes, err := newImagesService(fake).Context(context.Background(), testImageID)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if ctxRes.URL != "https://example.com/ctx.zip?sig=x" {
		t.Errorf("url = %q", ctxRes.URL)
	}
}

// TestImages_LinkGitHub_Body checks the JSON body of PUT /github.
func TestImages_LinkGitHub_Body(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"img_123","github":{"repository":"acme/api","branch":"main"}}`)

	img, err := newImagesService(fake).LinkGitHub(context.Background(), testImageID, ImageGitHubLinkBody{
		Repository: "acme/api",
		Branch:     "main",
	})
	if err != nil {
		t.Fatalf("LinkGitHub: %v", err)
	}
	if img.GitHub == nil || img.GitHub.Repository != "acme/api" {
		t.Errorf("github = %+v", img.GitHub)
	}

	req := fake.Requests[0]
	if req.ContentType != "application/json" {
		t.Errorf("content type = %q, want application/json", req.ContentType)
	}
	body := decodeJSONBody(t, req)
	if body["repository"] != "acme/api" || body["branch"] != "main" || len(body) != 2 {
		t.Errorf("body = %v", body)
	}
}

// TestImages_LinkGitHub_WithoutBranch checks that an empty branch is left out
// of the JSON body.
func TestImages_LinkGitHub_WithoutBranch(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"img_123","github":{"repository":"acme/api","branch":"main"}}`)

	if _, err := newImagesService(fake).LinkGitHub(context.Background(), testImageID, ImageGitHubLinkBody{
		Repository: "acme/api",
	}); err != nil {
		t.Fatalf("LinkGitHub: %v", err)
	}
	body := decodeJSONBody(t, fake.Requests[0])
	if body["repository"] != "acme/api" || len(body) != 1 {
		t.Errorf("body = %v, want only repository", body)
	}
	if _, ok := body["branch"]; ok {
		t.Errorf("branch present, want absent when empty")
	}
}

// TestImages_Check_DecodesFindings checks the request body (a nil file list
// is sent as an empty array) and the line that may be null.
func TestImages_Check_DecodesFindings(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"refusals":[{"code":"run_network_host","line":4},{"code":"volume_refused","line":6}],"warnings":[{"code":"workdir_app","line":2},{"code":"volume_outside_app","line":null}],"suggested_port":3000}`)

	res, err := newImagesService(fake).Check(context.Background(), ImageCheckBody{Dockerfile: "FROM node\n"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(res.Refusals) != 2 || res.Refusals[1].Code != DockerfileIssueCodeVolumeRefused || res.Refusals[0].Code != DockerfileIssueCodeRunNetworkHost || res.Refusals[0].Line == nil || *res.Refusals[0].Line != 4 {
		t.Errorf("refusals = %+v", res.Refusals)
	}
	if len(res.Warnings) != 2 || res.Warnings[1].Line != nil {
		t.Errorf("warnings = %+v", res.Warnings)
	}
	if res.SuggestedPort != 3000 {
		t.Errorf("suggested port = %d, want 3000", res.SuggestedPort)
	}

	body := decodeJSONBody(t, fake.Requests[0])
	if files, ok := body["files"].([]any); !ok || len(files) != 0 {
		t.Errorf("files = %#v, want an empty array", body["files"])
	}
	if body["dockerfile"] != "FROM node\n" {
		t.Errorf("dockerfile = %v", body["dockerfile"])
	}
}

// TestImages_RejectsDotSegmentIDs checks that an id which would change the
// route never reaches the transport.
func TestImages_RejectsDotSegmentIDs(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	_, err := newImagesService(fake).Get(context.Background(), "..")
	if !errors.Is(err, ErrInvalidPathSegment) {
		t.Fatalf("err = %v, want ErrInvalidPathSegment", err)
	}
	if len(fake.Requests) != 0 {
		t.Errorf("sent %d requests, want none", len(fake.Requests))
	}
}

// TestApps_CreateFromImage_Body checks the JSON body of POST /v1/apps/image
// and that the app's custom_image is decoded.
func TestApps_CreateFromImage_Body(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"app_1","name":"web","status":"building","custom_image":{"image_id":"img_123","version_id":null,"outdated":false,"port":3000}}`)

	port := 3000
	app, err := newAppsService(fake).CreateFromImage(context.Background(), ApplicationCreateFromImageParams{
		ImageID:     testImageID,
		Name:        "web",
		Memory:      512,
		Description: "landing",
		Web:         true,
		Subdomain:   "random",
		Port:        &port,
		Envs:        []ApplicationEnvironmentInput{{Key: "K", Value: "V"}},
		WorkspaceID: "ws_1",
	})
	if err != nil {
		t.Fatalf("CreateFromImage: %v", err)
	}
	if app.CustomImage == nil || app.CustomImage.ImageID != testImageID || app.CustomImage.Port == nil || *app.CustomImage.Port != 3000 {
		t.Errorf("custom_image = %+v", app.CustomImage)
	}

	req := fake.Requests[0]
	if req.Method != http.MethodPost || req.Path != "/v1/apps/image" {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}
	if req.ContentType != "application/json" {
		t.Errorf("content type = %q, want application/json", req.ContentType)
	}
	body := decodeJSONBody(t, req)
	want := map[string]any{
		"image_id":     testImageID,
		"name":         "web",
		"memory":       float64(512),
		"description":  "landing",
		"web":          true,
		"subdomain":    "random",
		"port":         float64(3000),
		"workspace_id": "ws_1",
	}
	for k, v := range want {
		if !reflect.DeepEqual(body[k], v) {
			t.Errorf("body[%s] = %#v, want %#v", k, body[k], v)
		}
	}
	envs, ok := body["envs"].([]any)
	if !ok || len(envs) != 1 {
		t.Errorf("envs = %#v, want one entry", body["envs"])
	}
}

// TestApps_CreateFromImage_OmitsOptionalFields checks that web: false still
// sends "web" and that unset optional fields are left out of the body.
func TestApps_CreateFromImage_OmitsOptionalFields(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"app_2","name":"bot","status":"building","custom_image":null}`)

	app, err := newAppsService(fake).CreateFromImage(context.Background(), ApplicationCreateFromImageParams{
		ImageID: testImageID,
		Name:    "bot",
		Memory:  256,
		Web:     false,
	})
	if err != nil {
		t.Fatalf("CreateFromImage: %v", err)
	}
	if app.CustomImage != nil {
		t.Errorf("custom_image = %+v, want nil when the server sends null", app.CustomImage)
	}

	body := decodeJSONBody(t, fake.Requests[0])
	if body["web"] != false {
		t.Errorf("web = %v, want false", body["web"])
	}
	for _, k := range []string{"description", "subdomain", "port", "envs", "workspace_id"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s present, want omitted", k)
		}
	}
}

// workspaceRequest is what the recording server saw for one request.
type workspaceRequest struct {
	method      string
	path        string
	hasKey      bool
	workspaceID string
}

// workspaceRecorder is an httptest server that records the method, path and
// workspace_id query parameter of every request, so the tests check the
// query that really goes on the wire.
type workspaceRecorder struct {
	mu   sync.Mutex
	reqs []workspaceRequest
}

// newWorkspaceRecorder starts the server and returns a real rest client
// pointed at it.
func newWorkspaceRecorder(t *testing.T) (rest.Client, *workspaceRecorder) {
	t.Helper()
	rec := &workspaceRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hasKey := r.URL.Query()["workspace_id"]
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, workspaceRequest{
			method:      r.Method,
			path:        r.URL.Path,
			hasKey:      hasKey,
			workspaceID: r.URL.Query().Get("workspace_id"),
		})
		rec.mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/logs") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: log\ndata: line\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"response":null}`)
	}))
	t.Cleanup(srv.Close)

	rc, err := rest.NewClient("test-key", rest.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return rc, rec
}

// take returns the requests recorded since the previous call and clears them.
func (r *workspaceRecorder) take() []workspaceRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.reqs
	r.reqs = nil
	return out
}

// TestImages_WorkspaceQueryOnEveryRoute checks that each route which acts on
// a workspace image sends ?workspace_id= when rest.WithWorkspaceID is given,
// and leaves the parameter out when it is not.
func TestImages_WorkspaceQueryOnEveryRoute(t *testing.T) {
	rc, rec := newWorkspaceRecorder(t)
	svc := newImagesService(rc)
	ctx := context.Background()
	imagePath := "/v1/images/" + testImageID

	routes := []struct {
		name       string
		wantMethod string
		wantPath   string
		call       func(opts ...rest.RequestOpt) error
	}{
		{"List", http.MethodGet, "/v1/images", func(opts ...rest.RequestOpt) error {
			_, err := svc.List(ctx, opts...)
			return err
		}},
		{"Get", http.MethodGet, imagePath, func(opts ...rest.RequestOpt) error {
			_, err := svc.Get(ctx, testImageID, opts...)
			return err
		}},
		{"Update", http.MethodPatch, imagePath, func(opts ...rest.RequestOpt) error {
			name := "node-web-2"
			_, err := svc.Update(ctx, testImageID, ImageUpdateBody{Name: &name}, opts...)
			return err
		}},
		{"Rebuild", http.MethodPut, imagePath, func(opts ...rest.RequestOpt) error {
			_, err := svc.Rebuild(ctx, testImageID, ImageRebuildParams{File: strings.NewReader("zip")}, opts...)
			return err
		}},
		{"Delete", http.MethodDelete, imagePath, func(opts ...rest.RequestOpt) error {
			_, err := svc.Delete(ctx, testImageID, ImageDeleteBody{ConfirmName: "node-web"}, opts...)
			return err
		}},
		{"Versions", http.MethodGet, imagePath + "/versions", func(opts ...rest.RequestOpt) error {
			_, err := svc.Versions(ctx, testImageID, opts...)
			return err
		}},
		{"Logs", http.MethodGet, imagePath + "/logs", func(opts ...rest.RequestOpt) error {
			stream, err := svc.Logs(ctx, testImageID, nil, opts...)
			if err != nil {
				return err
			}
			return stream.Close()
		}},
		{"Context", http.MethodGet, imagePath + "/context", func(opts ...rest.RequestOpt) error {
			_, err := svc.Context(ctx, testImageID, opts...)
			return err
		}},
		{"LinkGitHub", http.MethodPut, imagePath + "/github", func(opts ...rest.RequestOpt) error {
			_, err := svc.LinkGitHub(ctx, testImageID, ImageGitHubLinkBody{Repository: "acme/api", Branch: "main"}, opts...)
			return err
		}},
		{"UnlinkGitHub", http.MethodDelete, imagePath + "/github", func(opts ...rest.RequestOpt) error {
			_, err := svc.UnlinkGitHub(ctx, testImageID, opts...)
			return err
		}},
	}
	if len(routes) != 10 {
		t.Fatalf("expected 10 workspace routes, got %d", len(routes))
	}

	for _, rt := range routes {
		t.Run(rt.name, func(t *testing.T) {
			rec.take()
			if err := rt.call(rest.WithWorkspaceID("ws_1")); err != nil {
				t.Fatalf("call with workspace: %v", err)
			}
			reqs := rec.take()
			if len(reqs) != 1 {
				t.Fatalf("want 1 request, got %d", len(reqs))
			}
			got := reqs[0]
			if got.method != rt.wantMethod || got.path != rt.wantPath {
				t.Errorf("request = %s %s, want %s %s", got.method, got.path, rt.wantMethod, rt.wantPath)
			}
			if got.workspaceID != "ws_1" {
				t.Errorf("workspace_id = %q, want ws_1", got.workspaceID)
			}

			if err := rt.call(); err != nil {
				t.Fatalf("call without workspace: %v", err)
			}
			reqs = rec.take()
			if len(reqs) != 1 {
				t.Fatalf("want 1 request, got %d", len(reqs))
			}
			if reqs[0].hasKey {
				t.Errorf("workspace_id sent without a workspace, want it absent")
			}
		})
	}
}

// TestImages_WorkspaceParamsForwardToQuery checks the WorkspaceID fields of
// ImageLogsParams and ImageRebuildParams, and that an explicit
// rest.WithWorkspaceID option wins over them.
func TestImages_WorkspaceParamsForwardToQuery(t *testing.T) {
	rc, rec := newWorkspaceRecorder(t)
	svc := newImagesService(rc)
	ctx := context.Background()

	stream, err := svc.Logs(ctx, testImageID, &ImageLogsParams{WorkspaceID: "ws_2"})
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	stream.Close()

	stream, err = svc.Logs(ctx, testImageID, &ImageLogsParams{WorkspaceID: "ws_2"}, rest.WithWorkspaceID("ws_3"))
	if err != nil {
		t.Fatalf("Logs with option: %v", err)
	}
	stream.Close()

	if _, err := svc.Rebuild(ctx, testImageID, ImageRebuildParams{File: strings.NewReader("zip"), WorkspaceID: "ws_2"}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if _, err := svc.Rebuild(ctx, testImageID, ImageRebuildParams{File: strings.NewReader("zip"), WorkspaceID: "ws_2"}, rest.WithWorkspaceID("ws_3")); err != nil {
		t.Fatalf("Rebuild with option: %v", err)
	}

	reqs := rec.take()
	want := []string{"ws_2", "ws_3", "ws_2", "ws_3"}
	if len(reqs) != len(want) {
		t.Fatalf("want %d requests, got %d", len(want), len(reqs))
	}
	for i, w := range want {
		if reqs[i].workspaceID != w {
			t.Errorf("request %d (%s %s): workspace_id = %q, want %q", i, reqs[i].method, reqs[i].path, reqs[i].workspaceID, w)
		}
	}
}

// TestApps_FileUpload_DecodesImageBuild checks the image build started by a
// commit to an app on a custom image, and that a regular app has none.
func TestApps_FileUpload_DecodesImageBuild(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-10-23T10:00:00.000Z","removed_directories":[],"image_build":{"image_id":"img_123","build_id":"bld_9"},"message":"Image rebuilding; the app restarts when the build succeeds"}`)
	fake.EnqueueJSON(200, `{"app_id":"app_123","updated_at":"2026-10-23T10:00:00.000Z","removed_directories":[],"image_build":null}`)
	files := newAppsService(fake).Files()

	res, err := files.Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("zip"), FileName: "c.zip"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.ImageBuild == nil || res.ImageBuild.ImageID != testImageID || res.ImageBuild.BuildID != "bld_9" {
		t.Errorf("image_build = %+v", res.ImageBuild)
	}
	if res.Message == "" {
		t.Errorf("message is empty, want the readable summary")
	}

	res, err = files.Upload(context.Background(), testAppID, ApplicationFileUploadParams{File: strings.NewReader("zip"), FileName: "c.zip"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.ImageBuild != nil || res.Message != "" {
		t.Errorf("regular app: image_build = %+v, message = %q, want none", res.ImageBuild, res.Message)
	}
}

// TestApps_CreateFromSnapshot_DecodesWarning checks the warning sent when the
// image version recorded in the snapshot no longer exists.
func TestApps_CreateFromSnapshot_DecodesWarning(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"app_1","name":"web","warning":"IMAGE_VERSION_NOT_FOUND"}`)

	app, err := newAppsService(fake).Create(context.Background(), ApplicationCreateParams{SnapshotID: "snap_1", Name: "web"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if app.Warning == nil || *app.Warning != SnapshotImageWarningImageVersionNotFound {
		t.Errorf("warning = %v, want IMAGE_VERSION_NOT_FOUND", app.Warning)
	}
}

// TestSnapshots_RestoreWith_SendsBody checks the body of the restore route:
// restore_image_version and resource_id are sent only when set, and the
// response warning is decoded.
func TestSnapshots_RestoreWith_SendsBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"message":"restored","warning":"IMAGE_VERSION_NOT_FOUND"}`)
	svc := newSnapshotsService(fake)

	res, err := svc.RestoreWith(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications, SnapshotRestoreParams{RestoreImageVersion: true})
	if err != nil {
		t.Fatalf("RestoreWith: %v", err)
	}
	if res.Warning == nil || *res.Warning != SnapshotImageWarningImageVersionNotFound {
		t.Errorf("warning = %v, want IMAGE_VERSION_NOT_FOUND", res.Warning)
	}
	req := fake.Requests[0]
	if req.Method != http.MethodPost || req.Path != "/v1/users/"+testResourceID+"/snapshots/"+testSnapshotID+"/restore" {
		t.Errorf("request = %s %s", req.Method, req.Path)
	}
	body := decodeJSONBody(t, req)
	if body["restore_image_version"] != true {
		t.Errorf("restore_image_version = %v, want true", body["restore_image_version"])
	}
	if _, ok := body["resource_id"]; ok {
		t.Errorf("resource_id present, want absent")
	}

	fake.EnqueueJSON(200, `{"message":"restored"}`)
	if _, err := svc.RestoreWith(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications, SnapshotRestoreParams{TargetResourceID: "app_target"}); err != nil {
		t.Fatalf("RestoreWith target: %v", err)
	}
	body = decodeJSONBody(t, fake.Requests[1])
	if body["resource_id"] != "app_target" {
		t.Errorf("resource_id = %v, want app_target", body["resource_id"])
	}
	if _, ok := body["restore_image_version"]; ok {
		t.Errorf("restore_image_version present, want absent when false")
	}

	fake.EnqueueJSON(200, `{"message":"restored"}`)
	if _, err := svc.RestoreWith(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications, SnapshotRestoreParams{}); err != nil {
		t.Fatalf("RestoreWith empty: %v", err)
	}
	if len(fake.Requests[2].Body) != 0 {
		t.Errorf("body = %q, want none when no field is set", fake.Requests[2].Body)
	}
}

// TestImages_DockerfileRefusedIssues checks that the findings of a
// DOCKERFILE_REFUSED error are readable from the typed error, and that other
// errors are not reported as refusals.
func TestImages_DockerfileRefusedIssues(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(400, `{"code":"DOCKERFILE_REFUSED","message":"refused","details":{"issues":[{"code":"run_network_host","line":4},{"code":"mount_ssh","line":null}]}}`)
	svc := newImagesService(fake)

	_, err := svc.Create(context.Background(), ImageCreateParams{Name: "n", File: strings.NewReader("zip")})
	issues, ok := DockerfileRefusedIssues(err)
	if !ok {
		t.Fatalf("DockerfileRefusedIssues(%v) ok = false, want true", err)
	}
	if len(issues) != 2 || issues[0].Code != DockerfileIssueCodeRunNetworkHost || issues[0].Line == nil || *issues[0].Line != 4 || issues[1].Line != nil {
		t.Errorf("issues = %+v", issues)
	}

	fake.EnqueueError(404, `{"code":"IMAGE_NOT_FOUND","message":"no"}`)
	_, err = svc.Get(context.Background(), testImageID)
	if issues, ok := DockerfileRefusedIssues(err); ok || issues != nil {
		t.Errorf("IMAGE_NOT_FOUND: ok = %v, issues = %+v, want false and nil", ok, issues)
	}

	fake.EnqueueError(400, `{"code":"DOCKERFILE_REFUSED","message":"refused"}`)
	_, err = svc.Create(context.Background(), ImageCreateParams{Name: "n", File: strings.NewReader("zip")})
	if issues, ok := DockerfileRefusedIssues(err); !ok || len(issues) != 0 {
		t.Errorf("refusal without details: ok = %v, issues = %+v, want true and none", ok, issues)
	}

	if _, ok := DockerfileRefusedIssues(nil); ok {
		t.Errorf("nil error reported as a refusal")
	}
}
