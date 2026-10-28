package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// ImagesService groups the /v1/images/* routes (custom Docker images) and the
// Dockerfile check. Apps that run from an image are created with
// AppsService.CreateFromImage (POST /v1/apps/image).
//
// A build starts in the background: Create and Rebuild return the image with
// Status CustomImageStatusBuilding. Follow it with Logs, or read Get until the
// status leaves "building".
//
// An image that came from a template (CustomImage.Kind "template_copy", or a
// platform image) cannot be renamed, rebuilt, linked to GitHub or committed to:
// those calls fail with IMAGE_TEMPLATE_ONLY. Deleting an image that is the source
// of one of your private templates fails with IMAGE_IN_USE.
//
// To act on an image of a workspace, pass rest.WithWorkspaceID (sent as the
// workspace_id query parameter) on List, Get, Update, Rebuild, Delete, Versions, Logs,
// Context, LinkGitHub and UnlinkGitHub. Rebuild and Logs also take it in their
// params. Without it, the caller's own images are used.
type ImagesService interface {
	// List: GET /v1/images — scope images:read.
	List(ctx context.Context, opts ...rest.RequestOpt) ([]CustomImage, error)
	// Get: GET /v1/images/:id — scope images:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (CustomImage, error)
	// Create: POST /v1/images — scope images:write. multipart/form-data. Set
	// exactly one source: File, or GitHubRepository (GitHubBranch is optional).
	// A repository in your GitHub App installations is linked and each push to
	// the branch rebuilds the image; a public repository outside them is built
	// once, with no link (GitHub is nil). A private or missing repository
	// outside them fails with INSTALLATION_NOT_FOUND. The API replies 202 with
	// Status "building".
	Create(ctx context.Context, params ImageCreateParams, opts ...rest.RequestOpt) (CustomImage, error)
	// Update: PATCH /v1/images/:id — scope images:write. Renames the image
	// and/or changes its description, without building anything: apps keep
	// running and nothing restarts. Send only what changes; at least one field
	// is required (VALIDATION_ERROR otherwise). A name already used by another
	// of your images also fails with VALIDATION_ERROR (details.path "name").
	// System images cannot be changed (ACCESS_DENIED).
	Update(ctx context.Context, id string, body ImageUpdateBody, opts ...rest.RequestOpt) (CustomImage, error)
	// Rebuild: PUT /v1/images/:id — scope images:write. multipart/form-data
	// that replaces the whole build context. Refused with
	// IMAGE_BUILD_IN_PROGRESS while a build runs. Apps are not restarted.
	Rebuild(ctx context.Context, id string, params ImageRebuildParams, opts ...rest.RequestOpt) (CustomImage, error)
	// Delete: DELETE /v1/images/:id — scope images:delete. The image, its
	// versions and its context are deleted, and so is every app created from
	// it: an image with apps also needs apps:delete. body.ConfirmName must equal
	// the image name (IMAGE_NAME_MISMATCH otherwise).
	Delete(ctx context.Context, id string, body ImageDeleteBody, opts ...rest.RequestOpt) (ImageDeleteResult, error)
	// Versions: GET /v1/images/:id/versions — scope images:read. Newest first.
	Versions(ctx context.Context, id string, opts ...rest.RequestOpt) ([]CustomImageVersion, error)
	// Logs: GET /v1/images/:id/logs — scope images:read. Server-Sent Events:
	// the history is repeated, then the build is followed live. params may be
	// nil (latest build). There is no automatic reconnection.
	Logs(ctx context.Context, id string, params *ImageLogsParams, opts ...rest.RequestOpt) (*ImageLogStream, error)
	// Context: GET /v1/images/:id/context — scope images:read. Returns a
	// temporary download URL of the current build context.
	Context(ctx context.Context, id string, opts ...rest.RequestOpt) (ImageContext, error)
	// LinkGitHub: PUT /v1/images/:id/github — scope images:write. Links a
	// repository in your GitHub App installations: a push to the branch rebuilds
	// the image. The branch is optional (default: the repository's default branch).
	LinkGitHub(ctx context.Context, id string, body ImageGitHubLinkBody, opts ...rest.RequestOpt) (CustomImage, error)
	// UnlinkGitHub: DELETE /v1/images/:id/github — scope images:write.
	UnlinkGitHub(ctx context.Context, id string, opts ...rest.RequestOpt) (CustomImage, error)
	// Check: POST /v1/images/check — scope images:read. Checks a Dockerfile
	// and its file list before building; creates nothing.
	Check(ctx context.Context, body ImageCheckBody, opts ...rest.RequestOpt) (DockerfileCheck, error)
}

// ImageCreateParams is the input of ImagesService.Create.
//
// Exactly one source must be set: File, or GitHubRepository (with an optional
// GitHubBranch). The API rejects any other combination; this SDK does not
// duplicate that check.
type ImageCreateParams struct {
	// File is the build context: a ZIP file, up to 100 MB. Its root
	// must hold a file named exactly Dockerfile (one wrapping folder is
	// accepted). Mutually exclusive with GitHubRepository.
	File io.Reader
	// FileName is the multipart filename for File. Defaults to "image.zip".
	FileName string
	// Name is the image name. Required.
	Name string
	// Description is an optional free-text description, up to 128 characters.
	// When empty it is not sent (the image has no description).
	Description string
	// GitHubRepository is the repository to build from, as "owner/name".
	GitHubRepository string
	// GitHubBranch is the branch of GitHubRepository. Optional: when empty it is
	// not sent and the repository's default branch is used.
	GitHubBranch string
	// WorkspaceID makes the workspace the owner of the image when set.
	WorkspaceID string
}

// ImageUpdateBody is the body of ImagesService.Update. Every field is
// optional, but at least one must be set; a nil field leaves the value untouched.
type ImageUpdateBody struct {
	// Name is the new name: 1 to 50 characters, unique among the owner's
	// images. Sending the current name is not an error. A name cannot be null.
	Name *string `json:"name,omitempty"`
	// Description is the new description, up to 128 characters. A plain
	// *string with omitempty could not send null, so this uses Nullable:
	// NullableValue("text") sets it, NullableNull[string]() (JSON null) or
	// NullableValue("") removes it, and nil leaves it untouched.
	Description *Nullable[string] `json:"description,omitempty"`
}

// ImageRebuildParams is the input of ImagesService.Rebuild.
type ImageRebuildParams struct {
	// File is the new build context, replacing the previous one. Required.
	File io.Reader
	// FileName is the multipart filename for File. Defaults to "image.zip".
	FileName string
	// WorkspaceID acts on an image of that workspace (?workspace_id=). Same as
	// rest.WithWorkspaceID; an explicit option passed to Rebuild takes precedence.
	WorkspaceID string
}

// ImageDeleteBody is the body of ImagesService.Delete.
type ImageDeleteBody struct {
	// ConfirmName must be equal to the image name.
	ConfirmName string `json:"confirm_name"`
}

// ImageDeleteResult is the response of ImagesService.Delete.
type ImageDeleteResult struct {
	// DeletedApps lists the ids of the apps deleted together with the image.
	DeletedApps []string `json:"deleted_apps"`
}

// ImageLogsParams are the optional parameters of ImagesService.Logs.
type ImageLogsParams struct {
	// VersionID selects the build whose log to read. Empty means the latest build.
	VersionID string
	// WorkspaceID reads the log of an image of that workspace (?workspace_id=).
	// Same as rest.WithWorkspaceID; an explicit option passed to Logs takes precedence.
	WorkspaceID string
}

// ImageLogStream is the build log of an image, streamed as Server-Sent
// Events: each event carries one line of text. Use Next and Line, then Err.
type ImageLogStream struct{ *rest.EventStream }

// Line returns the text of the current log event.
func (s *ImageLogStream) Line() string {
	return s.EventStream.Event().Data
}

// ImageContext is the response of ImagesService.Context.
type ImageContext struct {
	// URL is a temporary download URL of the build context.
	URL string `json:"url"`
}

// ImageGitHubLinkBody is the body of ImagesService.LinkGitHub.
type ImageGitHubLinkBody struct {
	// Repository is the repository, as "owner/name".
	Repository string `json:"repository"`
	// Branch is the branch whose pushes rebuild the image. Optional: when empty
	// it is not sent and the repository's default branch is used.
	Branch string `json:"branch,omitempty"`
}

// ImageCheckBody is the body of ImagesService.Check.
type ImageCheckBody struct {
	// Dockerfile is the content of the Dockerfile.
	Dockerfile string `json:"dockerfile"`
	// Files lists the paths of the build context, relative to its root.
	Files []string `json:"files"`
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// CustomImageStatus is the state of the latest build of an image. A failed
// rebuild does not change the current version: the apps keep running the last
// good one.
type CustomImageStatus string

const (
	// CustomImageStatusBuilding: a build is running. Rebuilds are refused
	// until it ends.
	CustomImageStatusBuilding CustomImageStatus = "building"
	// CustomImageStatusSuccess: the latest build finished; the image can
	// create apps.
	CustomImageStatusSuccess CustomImageStatus = "success"
	// CustomImageStatusFailed: the latest build failed; see CustomImage.Error.
	CustomImageStatusFailed CustomImageStatus = "failed"
)

// CustomImageKind says where an image comes from. "own" is built from your own
// context; "template_copy" is the slot an image of a template takes in your
// account: it cannot be rebuilt or edited (IMAGE_TEMPLATE_ONLY), and it goes
// away when the last application that uses it does.
type CustomImageKind string

const (
	CustomImageKindOwn          CustomImageKind = "own"
	CustomImageKindTemplateCopy CustomImageKind = "template_copy"
)

// CustomImageOrigin is the readable origin of an image that came from a template.
type CustomImageOrigin struct {
	TemplateID string `json:"template_id"`
	// TemplateName is the name of the template, as shown in the list.
	TemplateName TemplateI18nText `json:"template_name"`
	// AuthorDisplay is the author as shown to you; nil for a template of the platform.
	AuthorDisplay *string `json:"author_display"`
}

// CustomImageBuildSource is what triggered the build that produced a version.
type CustomImageBuildSource string

const (
	// CustomImageBuildSourceUpload: a full build context uploaded by the user
	// (Create or Rebuild).
	CustomImageBuildSourceUpload CustomImageBuildSource = "upload"
	// CustomImageBuildSourceCommit: files committed on top of the current
	// build context (panel, CLI or GitHub Action).
	CustomImageBuildSourceCommit CustomImageBuildSource = "commit"
	// CustomImageBuildSourceGitHub: a push to the linked repository and branch.
	CustomImageBuildSourceGitHub CustomImageBuildSource = "github"
)

// CustomImageOwner says who owns an image. System images come from the
// platform and cannot be changed or deleted by users.
type CustomImageOwner string

const (
	CustomImageOwnerAccount CustomImageOwner = "account"
	CustomImageOwnerSystem  CustomImageOwner = "system"
)

// DockerfileIssueCode identifies one finding of ImagesService.Check. The
// refusal codes block the build; the warning codes do not.
type DockerfileIssueCode string

const (
	// DockerfileIssueCodeMissingDockerfile (refusal): no file named exactly
	// Dockerfile at the root of the context.
	DockerfileIssueCodeMissingDockerfile DockerfileIssueCode = "missing_dockerfile"
	// DockerfileIssueCodeSyntaxFrontend (refusal): a "# syntax=" directive
	// points to something other than the official Dockerfile frontend.
	DockerfileIssueCodeSyntaxFrontend DockerfileIssueCode = "syntax_frontend"
	// DockerfileIssueCodeRunSecurityInsecure (refusal): RUN --security=insecure.
	DockerfileIssueCodeRunSecurityInsecure DockerfileIssueCode = "run_security_insecure"
	// DockerfileIssueCodeRunNetworkHost (refusal): RUN --network=host.
	DockerfileIssueCodeRunNetworkHost DockerfileIssueCode = "run_network_host"
	// DockerfileIssueCodeMountSSH (refusal): --mount=type=ssh.
	DockerfileIssueCodeMountSSH DockerfileIssueCode = "mount_ssh"
	// DockerfileIssueCodeMountSecret (refusal): --mount=type=secret.
	DockerfileIssueCodeMountSecret DockerfileIssueCode = "mount_secret"
	// DockerfileIssueCodeWorkdirApp (warning): WORKDIR at /app or below. /app
	// is the persistent disk, which hides what the image puts there.
	DockerfileIssueCodeWorkdirApp DockerfileIssueCode = "workdir_app"
	// DockerfileIssueCodeEnvFileInContext (warning): .env files in the
	// context. They are removed before the build and their keys become app
	// variables.
	DockerfileIssueCodeEnvFileInContext DockerfileIssueCode = "env_file_in_context"
	// DockerfileIssueCodeCopyEnvFile (warning): COPY or ADD of a .env file,
	// which is not in the build context, so the build fails on that line.
	DockerfileIssueCodeCopyEnvFile DockerfileIssueCode = "copy_env_file"
	// DockerfileIssueCodeVolumeOutsideApp (warning): VOLUME outside /app. At
	// runtime that path is a small temporary folder (64 MB, erased on
	// restart); only /app keeps data.
	DockerfileIssueCodeVolumeOutsideApp DockerfileIssueCode = "volume_outside_app"
	// DockerfileIssueCodeVolumeRefused (refusal): VOLUME at /app or below it
	// (/app is already the app's persistent disk), or with a relative path.
	DockerfileIssueCodeVolumeRefused DockerfileIssueCode = "volume_refused"
)

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

// CustomImage is a Docker image: a build context with a Dockerfile, built in
// isolation by the platform.
type CustomImage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Description is the free-text description (up to 128 characters); nil when the image has none.
	Description *string          `json:"description"`
	Owner       CustomImageOwner `json:"owner"`
	// Kind is "own" or "template_copy"; empty in older payloads, where it means "own".
	Kind CustomImageKind `json:"kind,omitempty"`
	// Origin is the template the image came from when Kind is "template_copy"; nil otherwise.
	Origin *CustomImageOrigin `json:"origin,omitempty"`
	// Status is the state of the latest build.
	Status CustomImageStatus `json:"status"`
	// Error is the readable reason of the latest failed build; nil when it did not fail.
	Error *string `json:"error"`
	// Digest is the sha256 digest of the current version; nil while no build has succeeded.
	Digest *string `json:"digest"`
	// UncompressedBytes is the size of the current version; nil while no build has succeeded.
	UncompressedBytes *int64 `json:"uncompressed_bytes"`
	// LayerCount is the number of layers of the current version; nil while no build has succeeded.
	LayerCount *int `json:"layer_count"`
	// SuggestedPort is the port suggested for a new app on this image: the first EXPOSE of the
	// Dockerfile of the latest build context, or 80. Nil for an image created before this field existed.
	SuggestedPort *int `json:"suggested_port"`
	// Apps lists the applications created from this image.
	Apps []CustomImageApp `json:"apps"`
	// GitHub is the linked repository and branch; nil when the image is not linked.
	GitHub           *CustomImageGitHub `json:"github"`
	CurrentVersionID *string            `json:"current_version_id"`
	CreatedAt        time.Time          `json:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at"`
}

// CustomImageApp is an application that runs from a custom image.
type CustomImageApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// VersionID is the image version the app runs; nil when it has not started on any version yet.
	VersionID *string `json:"version_id"`
	// Outdated is true when the image has a newer version than the one the app
	// runs. The app moves to it on the next restart or deploy the user asks for.
	Outdated bool `json:"outdated"`
}

// CustomImageGitHub is the repository and branch an image is linked to.
type CustomImageGitHub struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
}

// CustomImageVersion is a successful build of an image. Each success is a new
// version of the same image.
type CustomImageVersion struct {
	ID string `json:"id"`
	// Digest is the sha256 digest of this version.
	Digest string `json:"digest"`
	// Source says what triggered the build.
	Source CustomImageBuildSource `json:"source"`
	// Message is the commit or upload message; nil when there is none.
	Message   *string   `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	// InUse is true when at least one application runs this version.
	InUse bool `json:"in_use"`
}

// DockerfileIssue is one finding of ImagesService.Check.
type DockerfileIssue struct {
	Code DockerfileIssueCode `json:"code"`
	// Line is the 1-based line of the Dockerfile; nil when the finding is not tied to a line.
	Line *int `json:"line"`
}

// DockerfileCheck is the result of ImagesService.Check.
type DockerfileCheck struct {
	// Refusals block the build.
	Refusals []DockerfileIssue `json:"refusals"`
	// Warnings do not block the build.
	Warnings []DockerfileIssue `json:"warnings"`
	// SuggestedPort is the first EXPOSE of the Dockerfile, or 80 when there is none.
	SuggestedPort int `json:"suggested_port"`
}

// ApplicationCustomImage is set on an application created from a custom
// image. The id and name of the application are on the Application itself.
type ApplicationCustomImage struct {
	ImageID string `json:"image_id"`
	// VersionID is the image version the app runs; nil when it has not started on any version yet.
	VersionID *string `json:"version_id"`
	// Outdated is true when the image has a newer version than the one the app runs.
	Outdated bool `json:"outdated"`
	// Port is the port the app listens on and the platform routes to; nil for an app without web traffic.
	Port *int `json:"port"`
}

// ApplicationCreateFromImageParams is the input of AppsService.CreateFromImage.
type ApplicationCreateFromImageParams struct {
	// ImageID is the image the app runs from. Required.
	ImageID string `json:"image_id"`
	// Name is the application name. Required.
	Name string `json:"name"`
	// Memory is the RAM in MB. Required.
	Memory      int    `json:"memory"`
	Description string `json:"description,omitempty"`
	// Web publishes the app on the web (Subdomain and Port apply). False is an
	// app without traffic, such as a bot or a worker.
	Web bool `json:"web"`
	// Subdomain is "random" or a name. Only with Web.
	Subdomain string `json:"subdomain,omitempty"`
	// Port is the port the app listens on. Only with Web. Nil means the first
	// EXPOSE of the image's Dockerfile, or 80.
	Port *int `json:"port,omitempty"`
	// Envs are the environment variables of the app.
	Envs []ApplicationEnvironmentInput `json:"envs,omitempty"`
	// WorkspaceID associates the app with a workspace when set.
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Implementation
// ---------------------------------------------------------------------------

func newImagesService(rc rest.Client) ImagesService {
	return &imagesServiceImpl{rest: guardPaths(rc)}
}

type imagesServiceImpl struct{ rest rest.Client }

// imagePath builds /v1/images/:id<suffix>, URL-encoding id.
func imagePath(id, suffix string) string {
	return "/v1/images/" + url.PathEscape(id) + suffix
}

func (s *imagesServiceImpl) List(ctx context.Context, opts ...rest.RequestOpt) ([]CustomImage, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/images", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]CustomImage](data)
}

func (s *imagesServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (CustomImage, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, imagePath(id, ""), nil, nil, "", opts...)
	if err != nil {
		return CustomImage{}, err
	}
	return rest.DecodeJSON[CustomImage](data)
}

func (s *imagesServiceImpl) Create(ctx context.Context, params ImageCreateParams, opts ...rest.RequestOpt) (CustomImage, error) {
	var fields []multipartField
	if params.File != nil {
		fileName := params.FileName
		if fileName == "" {
			fileName = "image.zip"
		}
		fields = append(fields, multipartField{Name: "file", FileName: fileName, Reader: params.File})
	}
	fields = append(fields, multipartField{Name: "name", Value: params.Name})
	if params.Description != "" {
		fields = append(fields, multipartField{Name: "description", Value: params.Description})
	}
	if params.GitHubRepository != "" {
		fields = append(fields, multipartField{Name: "github_repository", Value: params.GitHubRepository})
	}
	if params.GitHubBranch != "" {
		fields = append(fields, multipartField{Name: "github_branch", Value: params.GitHubBranch})
	}
	if params.WorkspaceID != "" {
		fields = append(fields, multipartField{Name: "workspace_id", Value: params.WorkspaceID})
	}

	body, contentType := newMultipartBody(fields)
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/images", nil, body, contentType, opts...)
	if err != nil {
		return CustomImage{}, err
	}
	return rest.DecodeJSON[CustomImage](data)
}

func (s *imagesServiceImpl) Update(ctx context.Context, id string, body ImageUpdateBody, opts ...rest.RequestOpt) (CustomImage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return CustomImage{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPatch, imagePath(id, ""), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return CustomImage{}, err
	}
	return rest.DecodeJSON[CustomImage](data)
}

func (s *imagesServiceImpl) Rebuild(ctx context.Context, id string, params ImageRebuildParams, opts ...rest.RequestOpt) (CustomImage, error) {
	if params.File == nil {
		return CustomImage{}, errors.New("vertracloud: Rebuild needs File")
	}
	if params.WorkspaceID != "" {
		opts = append([]rest.RequestOpt{rest.WithWorkspaceID(params.WorkspaceID)}, opts...)
	}
	fileName := params.FileName
	if fileName == "" {
		fileName = "image.zip"
	}
	body, contentType := newMultipartBody([]multipartField{
		{Name: "file", FileName: fileName, Reader: params.File},
	})
	data, err := s.rest.Do(ctx, http.MethodPut, imagePath(id, ""), nil, body, contentType, opts...)
	if err != nil {
		return CustomImage{}, err
	}
	return rest.DecodeJSON[CustomImage](data)
}

func (s *imagesServiceImpl) Delete(ctx context.Context, id string, body ImageDeleteBody, opts ...rest.RequestOpt) (ImageDeleteResult, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return ImageDeleteResult{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodDelete, imagePath(id, ""), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return ImageDeleteResult{}, err
	}
	return rest.DecodeJSON[ImageDeleteResult](data)
}

func (s *imagesServiceImpl) Versions(ctx context.Context, id string, opts ...rest.RequestOpt) ([]CustomImageVersion, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, imagePath(id, "/versions"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]CustomImageVersion](data)
}

func (s *imagesServiceImpl) Logs(ctx context.Context, id string, params *ImageLogsParams, opts ...rest.RequestOpt) (*ImageLogStream, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "version_id", params.VersionID)
		if params.WorkspaceID != "" {
			opts = append([]rest.RequestOpt{rest.WithWorkspaceID(params.WorkspaceID)}, opts...)
		}
	}
	stream, err := rest.OpenEventStream(ctx, s.rest, imagePath(id, "/logs"), q, opts...)
	if err != nil {
		return nil, err
	}
	return &ImageLogStream{EventStream: stream}, nil
}

func (s *imagesServiceImpl) Context(ctx context.Context, id string, opts ...rest.RequestOpt) (ImageContext, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, imagePath(id, "/context"), nil, nil, "", opts...)
	if err != nil {
		return ImageContext{}, err
	}
	return rest.DecodeJSON[ImageContext](data)
}

func (s *imagesServiceImpl) LinkGitHub(ctx context.Context, id string, body ImageGitHubLinkBody, opts ...rest.RequestOpt) (CustomImage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return CustomImage{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPut, imagePath(id, "/github"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return CustomImage{}, err
	}
	return rest.DecodeJSON[CustomImage](data)
}

func (s *imagesServiceImpl) UnlinkGitHub(ctx context.Context, id string, opts ...rest.RequestOpt) (CustomImage, error) {
	data, err := s.rest.Do(ctx, http.MethodDelete, imagePath(id, "/github"), nil, nil, "", opts...)
	if err != nil {
		return CustomImage{}, err
	}
	return rest.DecodeJSON[CustomImage](data)
}

func (s *imagesServiceImpl) Check(ctx context.Context, body ImageCheckBody, opts ...rest.RequestOpt) (DockerfileCheck, error) {
	if body.Files == nil {
		// Send an empty array, not null: the API expects a list.
		body.Files = []string{}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return DockerfileCheck{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/images/check", nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return DockerfileCheck{}, err
	}
	return rest.DecodeJSON[DockerfileCheck](data)
}

// DockerfileRefusedDetails is the details of a DOCKERFILE_REFUSED error (HTTP 400).
type DockerfileRefusedDetails struct {
	// Issues are the findings that blocked the build, as in DockerfileCheck.Refusals.
	Issues []DockerfileIssue `json:"issues"`
}

// DockerfileRefusedIssues reports whether err is a DOCKERFILE_REFUSED error
// (for example from ImagesService.Create or Rebuild) and returns its findings.
// ok is false for any other error or nil. ok is true with no issues when the
// error carries no readable details.
func DockerfileRefusedIssues(err error) (issues []DockerfileIssue, ok bool) {
	apiErr, isAPI := rest.AsAPIError(err)
	if !isAPI || apiErr.Code != "DOCKERFILE_REFUSED" {
		return nil, false
	}
	raw, marshalErr := json.Marshal(apiErr.Details)
	if marshalErr != nil {
		return nil, true
	}
	var details DockerfileRefusedDetails
	if json.Unmarshal(raw, &details) != nil {
		return nil, true
	}
	return details.Issues, true
}
