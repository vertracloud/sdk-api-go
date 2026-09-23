package vertracloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// AppsService groups every /v1/apps/* route (36 routes: runtimes, status,
// realtime, metrics, logs, download, deploys, network, envs, files).
//
// Nested resources (deploys, network, envs, files, and their own children)
// are exposed as methods returning their own service interface — e.g.
// AppsService.Deploys(), AppsNetworkService.CustomDomain() — built on
// demand by the implementation. Construction is cheap (one struct literal),
// so nothing is cached.
type AppsService interface {
	// Runtimes: GET /v1/apps/runtimes — scope apps:read.
	Runtimes(ctx context.Context, opts ...rest.RequestOpt) (ApplicationRuntimes, error)
	// StatusAll: GET /v1/apps/status — scope apps:read.
	StatusAll(ctx context.Context, opts ...rest.RequestOpt) ([]ApplicationStatusShort, error)
	// Get: GET /v1/apps/:id — scope apps:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (Application, error)
	// Status: GET /v1/apps/:id/status — scope apps:read.
	Status(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationStatusInfo, error)
	// Realtime: GET /v1/apps/:id/realtime — scope apps:read. The only SSE
	// route in the catalog; params may be nil. There is no automatic
	// reconnection — the stream ends when the connection closes or
	// EventStream.Close is called. Pass rest.WithIdleTimeout in opts to fail
	// EventStream.Next when no event arrives within a window.
	Realtime(ctx context.Context, id string, params *ApplicationRealtimeParams, opts ...rest.RequestOpt) (*ApplicationRealtimeEventStream, error)
	// Metrics: GET /v1/apps/:id/metrics — scope apps:read. params may be nil.
	Metrics(ctx context.Context, id string, params *ApplicationMetricsParams, opts ...rest.RequestOpt) ([]ApplicationMetric, error)
	// Logs: GET /v1/apps/:id/logs — scope apps:read. Returns the raw log
	// text.
	Logs(ctx context.Context, id string, opts ...rest.RequestOpt) (string, error)
	// Download: GET /v1/apps/:id/download — scope apps:read. Streams the
	// app's zip; the caller must Close it. No timeout applies unless
	// rest.WithRequestTimeout is passed — bound it with ctx instead.
	Download(ctx context.Context, id string, opts ...rest.RequestOpt) (io.ReadCloser, error)
	// Create: POST /v1/apps — scope apps:write. multipart/form-data; see
	// ApplicationCreateParams for the File/SnapshotID exclusivity rule.
	Create(ctx context.Context, params ApplicationCreateParams, opts ...rest.RequestOpt) (Application, error)
	// Start: POST /v1/apps/:id/start — scope apps:write.
	Start(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationOperationResponse, error)
	// Stop: POST /v1/apps/:id/stop — scope apps:write.
	Stop(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationOperationResponse, error)
	// Restart: POST /v1/apps/:id/restart — scope apps:write. body may be nil
	// for a normal restart (reuses the install/build cache).
	Restart(ctx context.Context, id string, body *ApplicationRestartBody, opts ...rest.RequestOpt) (ApplicationOperationResponse, error)
	// UpdateConfig: PATCH /v1/apps/:id/config — scope apps:write. Returns the
	// literal string the API sends back (e.g. "success"), not the app.
	UpdateConfig(ctx context.Context, id string, body ApplicationUpdateConfigBody, opts ...rest.RequestOpt) (string, error)
	// Delete: DELETE /v1/apps/:id — scope apps:delete.
	Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error

	Deploys() AppsDeploysService
	Network() AppsNetworkService
	Envs() AppsEnvsService
	Files() AppsFilesService
}

// ApplicationRealtimeParams are the optional parameters of AppsService.Realtime.
type ApplicationRealtimeParams struct {
	// Since resumes the stream after this Unix ms cursor; 0 omits it.
	Since int64
}

// ApplicationMetricsParams are the optional parameters of AppsService.Metrics.
type ApplicationMetricsParams struct {
	// Range is one of "10m", "30m" or "24h"; empty uses the API default.
	Range string
	// Since is a Unix ms cursor; 0 omits it.
	Since int64
}

// ApplicationRealtimeEventType is the SSE event name emitted by apps/realtime.
type ApplicationRealtimeEventType string

const (
	ApplicationRealtimeEventTypeLogs      ApplicationRealtimeEventType = "logs"
	ApplicationRealtimeEventTypeSystem    ApplicationRealtimeEventType = "system"
	ApplicationRealtimeEventTypeHeartbeat ApplicationRealtimeEventType = "heartbeat"
)

type ApplicationRealtimeEvent struct {
	Event ApplicationRealtimeEventType
	Data  string
	ID    string
}

type ApplicationRealtimeEventStream struct{ *rest.EventStream }

func (s *ApplicationRealtimeEventStream) Event() ApplicationRealtimeEvent {
	event := s.EventStream.Event()
	return ApplicationRealtimeEvent{Event: ApplicationRealtimeEventType(event.Event), Data: event.Data, ID: event.ID}
}

func newAppsService(rc rest.Client) AppsService { return &appsServiceImpl{rest: rc} }

type appsServiceImpl struct{ rest rest.Client }

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// ApplicationLanguage is a wire enum.
type ApplicationLanguage string

const (
	ApplicationLanguageJavaScript ApplicationLanguage = "javascript"
	ApplicationLanguageTypeScript ApplicationLanguage = "typescript"
	ApplicationLanguageBun        ApplicationLanguage = "bun"
	ApplicationLanguagePython     ApplicationLanguage = "python"
	ApplicationLanguageStatic     ApplicationLanguage = "static"
	ApplicationLanguagePHP        ApplicationLanguage = "php"
	ApplicationLanguageGo         ApplicationLanguage = "go"
	ApplicationLanguageRuby       ApplicationLanguage = "ruby"
	ApplicationLanguageJava       ApplicationLanguage = "java"
	ApplicationLanguageRust       ApplicationLanguage = "rust"
)

// ApplicationStatus is one of "up" | "down".
type ApplicationStatus string

const (
	ApplicationStatusUp   ApplicationStatus = "up"
	ApplicationStatusDown ApplicationStatus = "down"
)

// ApplicationCluster is a wire enum.
type ApplicationCluster int

const (
	ApplicationClusterUSA1 ApplicationCluster = 1
	ApplicationClusterUSA2 ApplicationCluster = 2
	ApplicationClusterUSA3 ApplicationCluster = 3
)

// ApplicationType is a wire enum.
type ApplicationType int

const (
	ApplicationTypeBot     ApplicationType = 1
	ApplicationTypeWebsite ApplicationType = 2
)

// ApplicationFileType is a wire enum.
type ApplicationFileType string

const (
	ApplicationFileTypeFile      ApplicationFileType = "file"
	ApplicationFileTypeDirectory ApplicationFileType = "directory"
)

// ApplicationVersion is a named string type. The server may add values, so
// callers should preserve unknown values when reading an application.
type ApplicationVersion string

const (
	ApplicationVersionRecommended ApplicationVersion = "recommended"
	ApplicationVersionLatest      ApplicationVersion = "latest"
	ApplicationVersionAuto        ApplicationVersion = "auto"
)

// ApplicationFileContentType is a wire enum.
type ApplicationFileContentType string

const ApplicationFileContentTypeBase64 ApplicationFileContentType = "base64"

// ApplicationShieldIncidentReason is a wire enum.
type ApplicationShieldIncidentReason string

const (
	ApplicationShieldIncidentReasonBurst     ApplicationShieldIncidentReason = "burst"
	ApplicationShieldIncidentReasonRateLimit ApplicationShieldIncidentReason = "rate_limit"
)

// ApplicationShieldIncidentDirection is a wire enum.
type ApplicationShieldIncidentDirection string

const (
	ApplicationShieldIncidentDirectionIn  ApplicationShieldIncidentDirection = "in"
	ApplicationShieldIncidentDirectionOut ApplicationShieldIncidentDirection = "out"
)

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

type ApplicationGithub struct {
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
}

type ApplicationShieldCooldown struct {
	Until     time.Time                          `json:"until"`
	Reason    ApplicationShieldIncidentReason    `json:"reason"`
	Direction ApplicationShieldIncidentDirection `json:"direction"`
	Strikes   int                                `json:"strikes"`
}

// Application is the shape returned by Get and by every
// mutating route that echoes the app back.
type Application struct {
	ID                  string                     `json:"id"`
	Cluster             ApplicationCluster         `json:"cluster"`
	Type                ApplicationType            `json:"type"`
	Name                string                     `json:"name"`
	Description         string                     `json:"description,omitempty"`
	OwnerID             string                     `json:"owner_id"`
	OwnerPlanID         UserPlan                   `json:"owner_plan_id"`
	Language            ApplicationLanguage        `json:"language"`
	RAM                 int                        `json:"ram"`
	Status              ApplicationStatus          `json:"status"`
	Subdomain           *string                    `json:"subdomain"`
	PublicURL           *string                    `json:"public_url"`
	Github              *ApplicationGithub         `json:"github,omitempty"`
	CustomDomain        *string                    `json:"custom_domain"`
	LastSnapshot        *time.Time                 `json:"last_snapshot"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
	MainFile            string                     `json:"main_file"`
	Version             ApplicationVersion         `json:"version"`
	AutoRestart         bool                       `json:"auto_restart"`
	StartCommand        *string                    `json:"start_command"`
	BuildCommand        *string                    `json:"build_command"`
	OfflineSince        *time.Time                 `json:"offline_since"`
	MissingDependencies []string                   `json:"missing_dependencies,omitempty"`
	RemovedDirectories  []string                   `json:"removed_directories,omitempty"`
	ShieldCooldown      *ApplicationShieldCooldown `json:"shield_cooldown"`
}

type ApplicationNetwork struct {
	Total string `json:"total"`
	Now   string `json:"now"`
}

// ApplicationStatusInfo is the response of GET /v1/apps/:id/status.
type ApplicationStatusInfo struct {
	ID         string              `json:"id"`
	CPU        string              `json:"cpu"`
	RAM        string              `json:"ram"`
	Status     ApplicationStatus   `json:"status"`
	Running    bool                `json:"running"`
	Installing *bool               `json:"installing,omitempty"`
	Storage    string              `json:"storage"`
	Network    *ApplicationNetwork `json:"network"`
	Uptime     int                 `json:"uptime"`
}

// ApplicationStatusShort is the response of GET /v1/apps/status.
//
// The endpoint returns one status item per application.
type ApplicationStatusShort struct {
	ID         string              `json:"id"`
	CPU        string              `json:"cpu"`
	RAM        string              `json:"ram"`
	Storage    *string             `json:"storage,omitempty"`
	Network    *ApplicationNetwork `json:"network,omitempty"`
	Running    bool                `json:"running"`
	Installing *bool               `json:"installing,omitempty"`
	// Uptime is nil when unknown (stopped container, or a host that didn't
	// respond) — never 0 as a sentinel; 0 means "just started".
	Uptime *int `json:"uptime"`
}

type ApplicationMetric struct {
	CPU     float64   `json:"cpu"`
	RAM     float64   `json:"ram"`
	Storage float64   `json:"storage"`
	Date    time.Time `json:"date"`
	Network []float64 `json:"network"`
}

type ApplicationFile struct {
	Type         ApplicationFileType `json:"type"`
	Name         string              `json:"name"`
	Path         string              `json:"path"`
	Size         string              `json:"size,omitempty"`
	LastModified time.Time           `json:"last_modified"`
}

// ApplicationFileTree is a file node plus optional children. GET
// /v1/apps/:id/files/tree returns a slice of root nodes of this shape.
type ApplicationFileTree struct {
	ApplicationFile
	Children []ApplicationFileTree `json:"children,omitempty"`
}

// ApplicationFileContent is a file read from an app. Data holds the file
// bytes in standard base64; Size is the decoded length in bytes.
type ApplicationFileContent struct {
	Type         ApplicationFileContentType `json:"type"`
	Data         string                     `json:"data"`
	Size         int                        `json:"size"`
	LastModified time.Time                  `json:"last_modified"`
}

// Bytes decodes Data.
func (c ApplicationFileContent) Bytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(c.Data)
}

type ApplicationEnvironment struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Note      *string   `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// ApplicationEnvironmentInput is one entry of the Apps.Envs().Set body.
//
// A nil Note leaves the existing annotation untouched; this SDK cannot send
// `note: null` to clear it.
type ApplicationEnvironmentInput struct {
	Key   string            `json:"key"`
	Value string            `json:"value"`
	Note  *Nullable[string] `json:"note,omitempty"`
}

type ResourceOperationStatus string

const ResourceOperationStatusSuccess ResourceOperationStatus = "success"

type ApplicationOperationResponse struct {
	Status ResourceOperationStatus `json:"status"`
}

type ApplicationDeployment struct {
	AppID     string    `json:"app_id"`
	CommitID  string    `json:"commit_id"`
	Message   string    `json:"message"`
	Pusher    string    `json:"pusher"`
	Branch    string    `json:"branch"`
	CreatedAt time.Time `json:"created_at"`
}

type ApplicationDNSRecord struct {
	Type   ApplicationDNSRecordType   `json:"type"`
	Name   string                     `json:"name"`
	Value  string                     `json:"value"`
	Status ApplicationDNSRecordStatus `json:"status"`
}

type ApplicationDNSRecordType string

const (
	ApplicationDNSRecordTypeCNAME ApplicationDNSRecordType = "CNAME"
	ApplicationDNSRecordTypeTXT   ApplicationDNSRecordType = "TXT"
)

type ApplicationDNSRecordStatus string

const (
	ApplicationDNSRecordStatusActive            ApplicationDNSRecordStatus = "active"
	ApplicationDNSRecordStatusPendingValidation ApplicationDNSRecordStatus = "pending_validation"
)

type ApplicationWebhook struct {
	WebhookURL string `json:"webhook_url"`
	RepoOwner  string `json:"repo_owner"`
	RepoName   string `json:"repo_name"`
}

type ApplicationSubdomainResponse struct {
	Subdomain string `json:"subdomain"`
}

type ApplicationCustomDomainResponse struct {
	Domain string `json:"domain"`
}

type ApplicationWebPublish struct {
	Subdomain    *string         `json:"subdomain"`
	CustomDomain *string         `json:"custom_domain"`
	Type         ApplicationType `json:"type"`
}

// ApplicationRuntimeEntry is one entry of GET /v1/apps/runtimes, keyed by runtime
// name (e.g. "javascript", "python").
type ApplicationRuntimeEntry struct {
	Recommended string   `json:"recommended"`
	Latest      string   `json:"latest"`
	Specific    []string `json:"specific"`
}

// ApplicationRuntimes is GET /v1/apps/runtimes, a map keyed by
// runtime name.
type ApplicationRuntimes map[ApplicationLanguage]ApplicationRuntimeEntry

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

// ApplicationCreateParams is the body of POST /v1/apps.
// It is multipart/form-data on the wire: File is streamed as the `file`
// part, every other field goes as a text part.
//
// Exactly one of File or SnapshotID must be set: neither -> SOURCE_REQUIRED,
// both -> SOURCE_CONFLICT (server-side validation; this SDK does not
// duplicate that check).
type ApplicationCreateParams struct {
	// File is the application zip. Mutually exclusive with SnapshotID.
	File io.Reader
	// FileName is the multipart filename for File. Defaults to "app.zip".
	FileName string
	// SnapshotID creates the app from an existing application snapshot
	// instead of a zip. Mutually exclusive with File.
	SnapshotID string

	Name        string
	Description *Nullable[string]
	// Memory is RAM in MB. Required unless SnapshotID is set; 0 omits the
	// field so the server can fall back to the snapshot's own memory.
	Memory  int
	Main    string
	Version ApplicationVersion
	Start   string
	Build   string
	// AutoRestart controls whether the app restarts after process exit.
	// It is sent as the multipart field "autorestart" when set.
	AutoRestart *bool
	// WorkspaceID associates the new app with a workspace when set.
	WorkspaceID string
	// Subdomain: "random" (or empty) lets the platform pick one.
	Subdomain string
	Envs      []ApplicationEnvironmentInput
}

// ApplicationRestartBody is the optional body of POST /v1/apps/:id/restart.
// No body (or a zero-value
// ApplicationRestartBody) means a normal restart, reusing the install/build cache.
type ApplicationRestartBody struct {
	CleanupOldRuntimeLanguage *ApplicationLanguage `json:"cleanup_old_runtime_language,omitempty"`
	// ReinstallDependencies reinstalls dependencies from scratch, ignoring
	// the install cache.
	ReinstallDependencies *bool `json:"reinstall_dependencies,omitempty"`
	// ForceBuild re-runs the build command even without a code change.
	// Counts as a deploy against the plan's hourly limit.
	ForceBuild *bool `json:"force_build,omitempty"`
}

// ApplicationUpdateConfigBody is the body of PATCH /v1/apps/:id/config.
// Every field is optional, but at least one must be set. Use NullableNull to
// clear Description/StartCommand/BuildCommand; nil leaves the value untouched.
type ApplicationUpdateConfigBody struct {
	Name         *string              `json:"name,omitempty"`
	Description  *Nullable[string]    `json:"description,omitempty"`
	MainFile     *string              `json:"main_file,omitempty"`
	Version      *ApplicationVersion  `json:"version,omitempty"`
	AutoRestart  *bool                `json:"auto_restart,omitempty"`
	StartCommand *Nullable[string]    `json:"start_command,omitempty"`
	BuildCommand *Nullable[string]    `json:"build_command,omitempty"`
	RAM          *int                 `json:"ram,omitempty"`
	Language     *ApplicationLanguage `json:"language,omitempty"`
}

// ApplicationPurgeCacheBody is the body of POST /v1/apps/:id/network/purge-cache.
// Both fields empty/nil purges every
// host of the app.
type ApplicationPurgeCacheBody struct {
	Hostnames []string `json:"hostnames,omitempty"`
	Paths     []string `json:"paths,omitempty"`
}

// ApplicationWebhookCreateBody is the body of POST /v1/apps/:id/deploys/webhook.
// All four fields are required.
type ApplicationWebhookCreateBody struct {
	Owner     string `json:"owner"`
	RepoName  string `json:"repo_name"`
	RepoID    string `json:"repo_id"`
	AccountID string `json:"account_id"`
}

// ApplicationFileWriteBody is the body of PUT /v1/apps/:id/files.
// Content omitted creates/truncates an
// empty file.
type ApplicationFileWriteBody struct {
	Path         string     `json:"path"`
	Content      *string    `json:"content,omitempty"`
	LastModified *time.Time `json:"last_modified,omitempty"`
}

// ApplicationFileListParams contains the optional file-list filter.
type ApplicationFileListParams struct{ Path string }

// ApplicationFileReadParams contains the required file-content path.
type ApplicationFileReadParams struct{ Path string }

// ApplicationFileDeleteBody is the JSON body of DELETE /v1/apps/:id/files.
type ApplicationFileDeleteBody struct {
	Path string `json:"path"`
}

// ApplicationFileMoveBody is the body of PATCH /v1/apps/:id/files.
type ApplicationFileMoveBody struct {
	Path string `json:"path"`
	To   string `json:"to"`
}

// ---------------------------------------------------------------------------
// appPath helper
// ---------------------------------------------------------------------------

// appPath builds /v1/apps/:id<suffix>, URL-encoding id.
func appPath(id, suffix string) string {
	return "/v1/apps/" + url.PathEscape(id) + suffix
}

// ---------------------------------------------------------------------------
// AppsService — top-level routes
// ---------------------------------------------------------------------------

func (s *appsServiceImpl) Runtimes(ctx context.Context, opts ...rest.RequestOpt) (ApplicationRuntimes, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/apps/runtimes", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[ApplicationRuntimes](data)
}

func (s *appsServiceImpl) StatusAll(ctx context.Context, opts ...rest.RequestOpt) ([]ApplicationStatusShort, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/apps/status", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationStatusShort](data)
}

func (s *appsServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (Application, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, appPath(id, ""), nil, nil, "", opts...)
	if err != nil {
		return Application{}, err
	}
	return rest.DecodeJSON[Application](data)
}

func (s *appsServiceImpl) Status(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationStatusInfo, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, appPath(id, "/status"), nil, nil, "", opts...)
	if err != nil {
		return ApplicationStatusInfo{}, err
	}
	return rest.DecodeJSON[ApplicationStatusInfo](data)
}

func (s *appsServiceImpl) Realtime(ctx context.Context, id string, params *ApplicationRealtimeParams, opts ...rest.RequestOpt) (*ApplicationRealtimeEventStream, error) {
	q := url.Values{}
	if params != nil && params.Since != 0 {
		q.Set("since", strconv.FormatInt(params.Since, 10))
	}
	stream, err := rest.OpenEventStream(ctx, s.rest, appPath(id, "/realtime"), q, opts...)
	if err != nil {
		return nil, err
	}
	return &ApplicationRealtimeEventStream{EventStream: stream}, nil
}

func (s *appsServiceImpl) Metrics(ctx context.Context, id string, params *ApplicationMetricsParams, opts ...rest.RequestOpt) ([]ApplicationMetric, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "range", params.Range)
		if params.Since != 0 {
			q.Set("since", strconv.FormatInt(params.Since, 10))
		}
	}
	data, err := s.rest.Do(ctx, http.MethodGet, appPath(id, "/metrics"), q, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationMetric](data)
}

func (s *appsServiceImpl) Logs(ctx context.Context, id string, opts ...rest.RequestOpt) (string, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, appPath(id, "/logs"), nil, nil, "", opts...)
	if err != nil {
		return "", err
	}
	return rest.DecodeJSON[string](data)
}

func (s *appsServiceImpl) Download(ctx context.Context, id string, opts ...rest.RequestOpt) (io.ReadCloser, error) {
	return s.rest.DoStream(ctx, http.MethodGet, appPath(id, "/download"), nil, opts...)
}

func (s *appsServiceImpl) Create(ctx context.Context, params ApplicationCreateParams, opts ...rest.RequestOpt) (Application, error) {
	var fields []multipartField

	if params.File != nil {
		fileName := params.FileName
		if fileName == "" {
			fileName = "app.zip"
		}
		fields = append(fields, multipartField{Name: "file", FileName: fileName, Reader: params.File})
	}
	if params.SnapshotID != "" {
		fields = append(fields, multipartField{Name: "snapshot_id", Value: params.SnapshotID})
	}
	fields = append(fields, multipartField{Name: "name", Value: params.Name})
	if params.Description != nil {
		value := ""
		if params.Description.Value != nil {
			value = *params.Description.Value
		}
		fields = append(fields, multipartField{Name: "description", Value: value})
	}
	if params.Memory > 0 {
		fields = append(fields, multipartField{Name: "memory", Value: strconv.Itoa(params.Memory)})
	}
	if params.Main != "" {
		fields = append(fields, multipartField{Name: "main", Value: params.Main})
	}
	if params.Version != "" {
		fields = append(fields, multipartField{Name: "version", Value: string(params.Version)})
	}
	if params.Start != "" {
		fields = append(fields, multipartField{Name: "start", Value: params.Start})
	}
	if params.Build != "" {
		fields = append(fields, multipartField{Name: "build", Value: params.Build})
	}
	if params.AutoRestart != nil {
		fields = append(fields, multipartField{Name: "autorestart", Value: strconv.FormatBool(*params.AutoRestart)})
	}
	if params.WorkspaceID != "" {
		fields = append(fields, multipartField{Name: "workspace_id", Value: params.WorkspaceID})
	}
	if params.Subdomain != "" {
		fields = append(fields, multipartField{Name: "subdomain", Value: params.Subdomain})
	}
	if len(params.Envs) > 0 {
		b, err := json.Marshal(params.Envs)
		if err != nil {
			return Application{}, fmt.Errorf("vertracloud: encode envs: %w", err)
		}
		fields = append(fields, multipartField{Name: "envs", Value: string(b)})
	}

	body, contentType := newMultipartBody(fields)
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/apps", nil, body, contentType, opts...)
	if err != nil {
		return Application{}, err
	}
	return rest.DecodeJSON[Application](data)
}

func (s *appsServiceImpl) Start(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationOperationResponse, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, appPath(id, "/start"), nil, nil, "", opts...)
	if err != nil {
		return ApplicationOperationResponse{}, err
	}
	return rest.DecodeJSON[ApplicationOperationResponse](data)
}

func (s *appsServiceImpl) Stop(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationOperationResponse, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, appPath(id, "/stop"), nil, nil, "", opts...)
	if err != nil {
		return ApplicationOperationResponse{}, err
	}
	return rest.DecodeJSON[ApplicationOperationResponse](data)
}

func (s *appsServiceImpl) Restart(ctx context.Context, id string, body *ApplicationRestartBody, opts ...rest.RequestOpt) (ApplicationOperationResponse, error) {
	var b ApplicationRestartBody
	if body != nil {
		b = *body
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return ApplicationOperationResponse{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPost, appPath(id, "/restart"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return ApplicationOperationResponse{}, err
	}
	return rest.DecodeJSON[ApplicationOperationResponse](data)
}

func (s *appsServiceImpl) UpdateConfig(ctx context.Context, id string, body ApplicationUpdateConfigBody, opts ...rest.RequestOpt) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPatch, appPath(id, "/config"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return "", err
	}
	return rest.DecodeJSON[string](data)
}

func (s *appsServiceImpl) Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error {
	_, err := s.rest.Do(ctx, http.MethodDelete, appPath(id, ""), nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Deploys / Deploys.Webhook
// ---------------------------------------------------------------------------

// AppsDeploysService groups the /v1/apps/:id/deploys* routes.
type AppsDeploysService interface {
	// List: GET /v1/apps/:id/deploys — scope apps:read.
	List(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationDeployment, error)
	Webhook() AppsDeploysWebhookService
}

type appsDeploysServiceImpl struct{ rest rest.Client }

func (s *appsServiceImpl) Deploys() AppsDeploysService { return &appsDeploysServiceImpl{rest: s.rest} }

func (d *appsDeploysServiceImpl) List(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationDeployment, error) {
	data, err := d.rest.Do(ctx, http.MethodGet, appPath(id, "/deploys"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationDeployment](data)
}

func (d *appsDeploysServiceImpl) Webhook() AppsDeploysWebhookService {
	return &appsDeploysWebhookServiceImpl{rest: d.rest}
}

// AppsDeploysWebhookService groups the /v1/apps/:id/deploys/webhook routes.
type AppsDeploysWebhookService interface {
	// Get: GET /v1/apps/:id/deploys/webhook — scope apps:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationWebhook, error)
	// Create: POST /v1/apps/:id/deploys/webhook — scope apps:write.
	Create(ctx context.Context, id string, body ApplicationWebhookCreateBody, opts ...rest.RequestOpt) (ApplicationWebhook, error)
	// Delete: DELETE /v1/apps/:id/deploys/webhook — scope apps:write.
	Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error
}

type appsDeploysWebhookServiceImpl struct{ rest rest.Client }

func (w *appsDeploysWebhookServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationWebhook, error) {
	data, err := w.rest.Do(ctx, http.MethodGet, appPath(id, "/deploys/webhook"), nil, nil, "", opts...)
	if err != nil {
		return ApplicationWebhook{}, err
	}
	return rest.DecodeJSON[ApplicationWebhook](data)
}

func (w *appsDeploysWebhookServiceImpl) Create(ctx context.Context, id string, body ApplicationWebhookCreateBody, opts ...rest.RequestOpt) (ApplicationWebhook, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return ApplicationWebhook{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := w.rest.Do(ctx, http.MethodPost, appPath(id, "/deploys/webhook"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return ApplicationWebhook{}, err
	}
	return rest.DecodeJSON[ApplicationWebhook](data)
}

func (w *appsDeploysWebhookServiceImpl) Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error {
	_, err := w.rest.Do(ctx, http.MethodDelete, appPath(id, "/deploys/webhook"), nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Network / Network.CustomDomain
// ---------------------------------------------------------------------------

// AppsNetworkService groups the /v1/apps/:id/network/* routes.
type AppsNetworkService interface {
	// DNS: GET /v1/apps/:id/network/dns — scope apps:read.
	DNS(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationDNSRecord, error)
	// PurgeCache: POST /v1/apps/:id/network/purge-cache — scope apps:write.
	// body may be nil, sent as an empty {} object.
	PurgeCache(ctx context.Context, id string, body *ApplicationPurgeCacheBody, opts ...rest.RequestOpt) error
	// SetSubdomain: PATCH /v1/apps/:id/network/subdomain — scope apps:write.
	SetSubdomain(ctx context.Context, id, subdomain string, opts ...rest.RequestOpt) (ApplicationSubdomainResponse, error)
	// Publish: POST /v1/apps/:id/network/publish — scope apps:write.
	// subdomain empty lets the platform pick one (Pro plan is always random;
	// Scale can choose).
	Publish(ctx context.Context, id, subdomain string, opts ...rest.RequestOpt) (ApplicationWebPublish, error)
	// Unpublish: DELETE /v1/apps/:id/network/publish — scope apps:write.
	// Frees the subdomain, removes any custom domain, and makes the app
	// private.
	Unpublish(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationWebPublish, error)
	CustomDomain() AppsNetworkCustomDomainService
}

type appsNetworkServiceImpl struct{ rest rest.Client }

func (s *appsServiceImpl) Network() AppsNetworkService { return &appsNetworkServiceImpl{rest: s.rest} }

func (n *appsNetworkServiceImpl) DNS(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationDNSRecord, error) {
	data, err := n.rest.Do(ctx, http.MethodGet, appPath(id, "/network/dns"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationDNSRecord](data)
}

func (n *appsNetworkServiceImpl) PurgeCache(ctx context.Context, id string, body *ApplicationPurgeCacheBody, opts ...rest.RequestOpt) error {
	var b ApplicationPurgeCacheBody
	if body != nil {
		b = *body
	}
	payload, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	_, err = n.rest.Do(ctx, http.MethodPost, appPath(id, "/network/purge-cache"), nil, bytes.NewReader(payload), "application/json", opts...)
	return err
}

func (n *appsNetworkServiceImpl) SetSubdomain(ctx context.Context, id, subdomain string, opts ...rest.RequestOpt) (ApplicationSubdomainResponse, error) {
	body := struct {
		Subdomain string `json:"subdomain"`
	}{Subdomain: subdomain}
	payload, err := json.Marshal(body)
	if err != nil {
		return ApplicationSubdomainResponse{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := n.rest.Do(ctx, http.MethodPatch, appPath(id, "/network/subdomain"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return ApplicationSubdomainResponse{}, err
	}
	return rest.DecodeJSON[ApplicationSubdomainResponse](data)
}

func (n *appsNetworkServiceImpl) Publish(ctx context.Context, id, subdomain string, opts ...rest.RequestOpt) (ApplicationWebPublish, error) {
	body := struct {
		Subdomain string `json:"subdomain,omitempty"`
	}{Subdomain: subdomain}
	payload, err := json.Marshal(body)
	if err != nil {
		return ApplicationWebPublish{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := n.rest.Do(ctx, http.MethodPost, appPath(id, "/network/publish"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return ApplicationWebPublish{}, err
	}
	return rest.DecodeJSON[ApplicationWebPublish](data)
}

func (n *appsNetworkServiceImpl) Unpublish(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationWebPublish, error) {
	data, err := n.rest.Do(ctx, http.MethodDelete, appPath(id, "/network/publish"), nil, nil, "", opts...)
	if err != nil {
		return ApplicationWebPublish{}, err
	}
	return rest.DecodeJSON[ApplicationWebPublish](data)
}

func (n *appsNetworkServiceImpl) CustomDomain() AppsNetworkCustomDomainService {
	return &appsNetworkCustomDomainServiceImpl{rest: n.rest}
}

// AppsNetworkCustomDomainService groups the /v1/apps/:id/network/custom
// routes.
type AppsNetworkCustomDomainService interface {
	// Get: GET /v1/apps/:id/network/custom — scope apps:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationCustomDomainResponse, error)
	// Set: POST /v1/apps/:id/network/custom — scope apps:write.
	Set(ctx context.Context, id, domain string, opts ...rest.RequestOpt) (ApplicationCustomDomainResponse, error)
	// Remove: DELETE /v1/apps/:id/network/custom — scope apps:write.
	Remove(ctx context.Context, id string, opts ...rest.RequestOpt) error
}

type appsNetworkCustomDomainServiceImpl struct{ rest rest.Client }

func (cd *appsNetworkCustomDomainServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (ApplicationCustomDomainResponse, error) {
	data, err := cd.rest.Do(ctx, http.MethodGet, appPath(id, "/network/custom"), nil, nil, "", opts...)
	if err != nil {
		return ApplicationCustomDomainResponse{}, err
	}
	return rest.DecodeJSON[ApplicationCustomDomainResponse](data)
}

func (cd *appsNetworkCustomDomainServiceImpl) Set(ctx context.Context, id, domain string, opts ...rest.RequestOpt) (ApplicationCustomDomainResponse, error) {
	body := struct {
		Domain string `json:"domain"`
	}{Domain: domain}
	payload, err := json.Marshal(body)
	if err != nil {
		return ApplicationCustomDomainResponse{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := cd.rest.Do(ctx, http.MethodPost, appPath(id, "/network/custom"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return ApplicationCustomDomainResponse{}, err
	}
	return rest.DecodeJSON[ApplicationCustomDomainResponse](data)
}

func (cd *appsNetworkCustomDomainServiceImpl) Remove(ctx context.Context, id string, opts ...rest.RequestOpt) error {
	_, err := cd.rest.Do(ctx, http.MethodDelete, appPath(id, "/network/custom"), nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Envs
// ---------------------------------------------------------------------------

// AppsEnvsService groups the /v1/apps/:id/envs* routes.
type AppsEnvsService interface {
	// List: GET /v1/apps/:id/envs — scope apps:envs.
	List(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationEnvironment, error)
	// Set: POST /v1/apps/:id/envs — scope apps:envs. The wire body accepts
	// one env object or an array of them with no repeated key; this SDK
	// always sends an array (pass a single-element slice to set just one).
	Set(ctx context.Context, id string, envs []ApplicationEnvironmentInput, opts ...rest.RequestOpt) ([]ApplicationEnvironment, error)
	// Delete: DELETE /v1/apps/:id/envs/:envId — scope apps:envs.
	Delete(ctx context.Context, id, envID string, opts ...rest.RequestOpt) error
}

type appsEnvsServiceImpl struct{ rest rest.Client }

func (s *appsServiceImpl) Envs() AppsEnvsService { return &appsEnvsServiceImpl{rest: s.rest} }

func (e *appsEnvsServiceImpl) List(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationEnvironment, error) {
	data, err := e.rest.Do(ctx, http.MethodGet, appPath(id, "/envs"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationEnvironment](data)
}

func (e *appsEnvsServiceImpl) Set(ctx context.Context, id string, envs []ApplicationEnvironmentInput, opts ...rest.RequestOpt) ([]ApplicationEnvironment, error) {
	payload, err := json.Marshal(envs)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := e.rest.Do(ctx, http.MethodPost, appPath(id, "/envs"), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationEnvironment](data)
}

func (e *appsEnvsServiceImpl) Delete(ctx context.Context, id, envID string, opts ...rest.RequestOpt) error {
	_, err := e.rest.Do(ctx, http.MethodDelete, appPath(id, "/envs/"+url.PathEscape(envID)), nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

// AppsFilesService groups the /v1/apps/:id/files* routes.
type AppsFilesService interface {
	// List: GET /v1/apps/:id/files — scope apps:files. path filters to a
	// directory; empty lists the root.
	List(ctx context.Context, id string, params ApplicationFileListParams, opts ...rest.RequestOpt) ([]ApplicationFile, error)
	// Tree: GET /v1/apps/:id/files/tree — scope apps:files.
	Tree(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationFileTree, error)
	// Read: GET /v1/apps/:id/files/content — scope apps:files. path is
	// required.
	Read(ctx context.Context, id string, params ApplicationFileReadParams, opts ...rest.RequestOpt) (ApplicationFileContent, error)
	// Write: PUT /v1/apps/:id/files — scope apps:files. Creates or
	// overwrites the file at body.Path.
	Write(ctx context.Context, id string, body ApplicationFileWriteBody, opts ...rest.RequestOpt) error
	// Move: PATCH /v1/apps/:id/files — scope apps:files. Renames/moves
	// body.Path to body.To.
	Move(ctx context.Context, id string, body ApplicationFileMoveBody, opts ...rest.RequestOpt) error
	// Delete: DELETE /v1/apps/:id/files — scope apps:files.
	Delete(ctx context.Context, id string, body ApplicationFileDeleteBody, opts ...rest.RequestOpt) error
	// Upload: POST /v1/apps/:id/files/upload — scope apps:files.
	// multipart/form-data, streamed from params.File.
	Upload(ctx context.Context, id string, params ApplicationFileUploadParams, opts ...rest.RequestOpt) (ApplicationFileUploadResponse, error)
}

// ApplicationFileUploadParams is the input of AppsFilesService.Upload.
type ApplicationFileUploadParams struct {
	File     io.Reader
	FileName string
	// Restart, when non-nil, restarts (true) or not (false) the app after
	// the upload lands; nil uses the API default.
	Restart *bool
}

// ApplicationFileUploadResponse is the successful response from POST /v1/apps/:id/files/upload.
type ApplicationFileUploadResponse struct {
	AppID               string    `json:"app_id"`
	UpdatedAt           time.Time `json:"updated_at"`
	MissingDependencies []string  `json:"missing_dependencies,omitempty"`
	RemovedDirectories  []string  `json:"removed_directories,omitempty"`
}

type appsFilesServiceImpl struct{ rest rest.Client }

func (s *appsServiceImpl) Files() AppsFilesService { return &appsFilesServiceImpl{rest: s.rest} }

func (f *appsFilesServiceImpl) List(ctx context.Context, id string, params ApplicationFileListParams, opts ...rest.RequestOpt) ([]ApplicationFile, error) {
	q := url.Values{}
	addQueryParam(q, "path", params.Path)
	data, err := f.rest.Do(ctx, http.MethodGet, appPath(id, "/files"), q, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationFile](data)
}

func (f *appsFilesServiceImpl) Tree(ctx context.Context, id string, opts ...rest.RequestOpt) ([]ApplicationFileTree, error) {
	data, err := f.rest.Do(ctx, http.MethodGet, appPath(id, "/files/tree"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ApplicationFileTree](data)
}

func (f *appsFilesServiceImpl) Read(ctx context.Context, id string, params ApplicationFileReadParams, opts ...rest.RequestOpt) (ApplicationFileContent, error) {
	q := url.Values{}
	addQueryParam(q, "path", params.Path)
	data, err := f.rest.Do(ctx, http.MethodGet, appPath(id, "/files/content"), q, nil, "", opts...)
	if err != nil {
		return ApplicationFileContent{}, err
	}
	return rest.DecodeJSON[ApplicationFileContent](data)
}

func (f *appsFilesServiceImpl) Write(ctx context.Context, id string, body ApplicationFileWriteBody, opts ...rest.RequestOpt) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	_, err = f.rest.Do(ctx, http.MethodPut, appPath(id, "/files"), nil, bytes.NewReader(payload), "application/json", opts...)
	return err
}

func (f *appsFilesServiceImpl) Move(ctx context.Context, id string, body ApplicationFileMoveBody, opts ...rest.RequestOpt) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	_, err = f.rest.Do(ctx, http.MethodPatch, appPath(id, "/files"), nil, bytes.NewReader(payload), "application/json", opts...)
	return err
}

func (f *appsFilesServiceImpl) Delete(ctx context.Context, id string, body ApplicationFileDeleteBody, opts ...rest.RequestOpt) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	_, err = f.rest.Do(ctx, http.MethodDelete, appPath(id, "/files"), nil, bytes.NewReader(payload), "application/json", opts...)
	return err
}

func (f *appsFilesServiceImpl) Upload(ctx context.Context, id string, params ApplicationFileUploadParams, opts ...rest.RequestOpt) (ApplicationFileUploadResponse, error) {
	q := url.Values{}
	addQueryBool(q, "restart", params.Restart)
	body, contentType := newMultipartBody([]multipartField{{Name: "file", FileName: params.FileName, Reader: params.File}})
	data, err := f.rest.Do(ctx, http.MethodPost, appPath(id, "/files/upload"), q, body, contentType, opts...)
	if err != nil {
		return ApplicationFileUploadResponse{}, err
	}
	return rest.DecodeJSON[ApplicationFileUploadResponse](data)
}
