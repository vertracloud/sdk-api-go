package vertracloud

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// StacksService groups the /v1/stacks* routes. A stack groups applications,
// databases and Blob folders, with the connections between them: the platform
// injects the connection variables (and the certificates of a database) into
// the applications and keeps them up to date.
//
// An application reads its variables and certificates only when it starts, so
// a change that alters them does not restart it: the answer carries
// RestartRequired and AppsToRestart, and the restart is yours to ask for.
//
// To act on a stack of a workspace, pass rest.WithWorkspaceID (sent as the
// workspace_id query parameter) on the calls that read or change a stack; the
// owner of the stack may omit it. A key created for the stack's Blob folder
// is managed by the platform (see APIKeyManager).
type StacksService interface {
	// List: GET /v1/stacks — scope stacks:read. With rest.WithWorkspaceID,
	// lists the stacks of that workspace.
	List(ctx context.Context, opts ...rest.RequestOpt) ([]Stack, error)
	// Get: GET /v1/stacks/:stack_id — scope stacks:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (Stack, error)
	// Create: POST /v1/stacks — scope stacks:write. Creates a stack from
	// resources that already exist, all in the same account or workspace and
	// none in another stack.
	Create(ctx context.Context, body StackCreateBody, opts ...rest.RequestOpt) (StackChange, error)
	// Transform: POST /v1/stacks/transform — scope stacks:write. Turns the
	// applications and databases of a folder into a stack; the folder stays.
	Transform(ctx context.Context, body StackTransformBody, opts ...rest.RequestOpt) (StackChange, error)
	// Update: PATCH /v1/stacks/:stack_id — scope stacks:write. At least one
	// field is required.
	Update(ctx context.Context, id string, body StackUpdateBody, opts ...rest.RequestOpt) (Stack, error)
	// Delete: DELETE /v1/stacks/:stack_id — scope stacks:delete. With mode
	// "delete_all" an API key also needs apps:delete and databases:delete (and
	// blob:write when the Blob files are deleted).
	Delete(ctx context.Context, id string, body StackDeleteBody, opts ...rest.RequestOpt) (StackDeleteResult, error)

	// Members gives access to the members of a stack.
	Members() StackMembersService
	// Bindings gives access to the connections of a stack.
	Bindings() StackBindingsService
	// Snapshots gives access to the snapshots of a whole stack.
	Snapshots() StackSnapshotsService

	// Connection: GET /v1/stacks/:stack_id/connection — scope stacks:read.
	// Code to connect an application of the stack to what it is linked to.
	Connection(ctx context.Context, id string, params StackConnectionParams, opts ...rest.RequestOpt) (StackConnectionInfo, error)
	// SetNetworkMode: POST /v1/stacks/:stack_id/network-mode — scope
	// stacks:write. Only "public" is accepted for now; "private" fails with
	// STACK_NETWORK_MODE_UNAVAILABLE.
	SetNetworkMode(ctx context.Context, id string, mode StackNetworkMode, opts ...rest.RequestOpt) (Stack, error)
	// AddDatabase: POST /v1/stacks/:stack_id/databases — scope stacks:write.
	// Creates a database already inside the stack and connects it to the
	// applications you choose.
	AddDatabase(ctx context.Context, id string, body StackDatabaseBody, opts ...rest.RequestOpt) (StackDatabaseAdded, error)
	// AddDatabaseToApp: POST /v1/apps/:id/stack/databases — scope
	// stacks:write. The same as AddDatabase for the stack of an application;
	// on an application that is not in a stack, it creates the stack too,
	// named by body.StackName.
	AddDatabaseToApp(ctx context.Context, appID string, body StackDatabaseBody, opts ...rest.RequestOpt) (StackDatabaseAdded, error)
	// AddBlob: POST /v1/stacks/:stack_id/blob — scope stacks:write. Adds a
	// folder of your Blob Storage to the stack, with an API key restricted to
	// it. The plan must include Blob Storage.
	AddBlob(ctx context.Context, id string, body StackBlobBody, opts ...rest.RequestOpt) (StackBlobAdded, error)
}

// StackMembersService groups the member routes of a stack.
type StackMembersService interface {
	// Add: POST /v1/stacks/:stack_id/members — scope stacks:write. Puts an
	// application or database that you already have into the stack: same
	// account or workspace, and not in another stack.
	Add(ctx context.Context, stackID string, body StackMemberAddBody, opts ...rest.RequestOpt) (StackChange, error)
	// Remove: DELETE /v1/stacks/:stack_id/members/:resource_type/:resource_id
	// — scope stacks:write. Takes a member out of the stack without deleting
	// it. params may be nil. Taking out a member that came from a template
	// detaches the stack from it; an application of a closed template cannot
	// leave (STACK_SERVICE_LOCKED).
	Remove(ctx context.Context, stackID string, resourceType StackMemberType, resourceID string, params *StackMemberRemoveParams, opts ...rest.RequestOpt) (StackChange, error)
}

// StackBindingsService groups the connection routes of a stack.
type StackBindingsService interface {
	// List: GET /v1/stacks/:stack_id/bindings — scope stacks:read.
	List(ctx context.Context, stackID string, opts ...rest.RequestOpt) ([]StackBinding, error)
	// Create: POST /v1/stacks/:stack_id/bindings — scope stacks:write.
	// Connects an application of the stack to a database, a Blob folder or
	// another application of the stack: exactly one of DatabaseID, BlobID
	// and TargetAppID.
	Create(ctx context.Context, stackID string, body StackBindingCreateBody, opts ...rest.RequestOpt) (StackBindingChange, error)
	// Update: PATCH /v1/stacks/:stack_id/bindings/:binding_id — scope
	// stacks:write. Replaces the variable names of the connection.
	Update(ctx context.Context, stackID, bindingID string, envMap map[string]StackBindingValue, opts ...rest.RequestOpt) (StackBindingChange, error)
	// Delete: DELETE /v1/stacks/:stack_id/bindings/:binding_id — scope
	// stacks:write. params may be nil.
	Delete(ctx context.Context, stackID, bindingID string, params *StackBindingDeleteParams, opts ...rest.RequestOpt) (RestartNotice, error)
}

// StackSnapshotsService groups the snapshot routes of a stack.
type StackSnapshotsService interface {
	// List: GET /v1/stacks/:stack_id/snapshots — scope stacks:read. Newest first.
	List(ctx context.Context, stackID string, opts ...rest.RequestOpt) ([]StackSnapshot, error)
	// Create: POST /v1/stacks/:stack_id/snapshots — scope stacks:write. Takes
	// a snapshot of every member that has data, together.
	Create(ctx context.Context, stackID string, opts ...rest.RequestOpt) (StackSnapshot, error)
	// Restore: POST /v1/stacks/:stack_id/snapshots/:snapshot_id/restore —
	// scope stacks:write. Brings every member back, together. params may be nil.
	Restore(ctx context.Context, stackID, snapshotID string, params *StackSnapshotRestoreParams, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error)
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// StackNetworkMode is how the members of a stack reach each other. "public"
// uses the public addresses of the services (databases with client
// certificates); "private" is reserved and not available yet.
type StackNetworkMode string

const (
	StackNetworkModePublic  StackNetworkMode = "public"
	StackNetworkModePrivate StackNetworkMode = "private"
)

// StackMemberType is the kind of a member of a stack.
type StackMemberType string

const (
	StackMemberTypeApplication StackMemberType = "application"
	StackMemberTypeDatabase    StackMemberType = "database"
	// StackMemberTypeBlob is a folder of the account's Blob Storage, with a key restricted to it.
	StackMemberTypeBlob StackMemberType = "blob"
)

// StackBlobDisposition is what happens to a Blob folder when it leaves a
// stack or the stack is deleted. In both cases the key the platform created
// for the stack is revoked.
type StackBlobDisposition string

const (
	// StackBlobDispositionKeep leaves the files where they are. Default.
	StackBlobDispositionKeep StackBlobDisposition = "keep"
	// StackBlobDispositionDelete removes the files.
	StackBlobDispositionDelete StackBlobDisposition = "delete"
)

// StackBindingRemoval is what happens to the variables of a connection when
// a member leaves a stack or a connection is removed.
type StackBindingRemoval string

const (
	// StackBindingRemovalKeepConnection rewrites the variables to the public
	// address with certificates and turns them into ordinary variables. Default.
	StackBindingRemovalKeepConnection StackBindingRemoval = "keep_connection"
	// StackBindingRemovalRemoveVariables deletes them (and the certificates copied to the app).
	StackBindingRemovalRemoveVariables StackBindingRemoval = "remove_variables"
)

// StackDeleteMode is how a stack is deleted.
type StackDeleteMode string

const (
	// StackDeleteModeDissolve removes only the stack: the members stay as
	// loose resources and Blob files are kept.
	StackDeleteModeDissolve StackDeleteMode = "dissolve"
	// StackDeleteModeDeleteAll deletes every member as well, each by its own deletion rule.
	StackDeleteModeDeleteAll StackDeleteMode = "delete_all"
)

// StackBindingValue is what a managed variable of a connection receives. For
// a database: url, host, port, user, password, database, the paths of the
// client certificates (ssl.ca_path, ssl.cert_path, ssl.key_path, ssl.pem_path,
// ssl.p12_path) and prisma_url (never added by default). For another app:
// url. For a Blob folder: endpoint, bucket, region, prefix, access_key_id,
// secret_access_key and public_base_url.
type StackBindingValue string

const (
	StackBindingValueURL             StackBindingValue = "url"
	StackBindingValueHost            StackBindingValue = "host"
	StackBindingValuePort            StackBindingValue = "port"
	StackBindingValueUser            StackBindingValue = "user"
	StackBindingValuePassword        StackBindingValue = "password"
	StackBindingValueDatabase        StackBindingValue = "database"
	StackBindingValueSSLCAPath       StackBindingValue = "ssl.ca_path"
	StackBindingValueSSLCertPath     StackBindingValue = "ssl.cert_path"
	StackBindingValueSSLKeyPath      StackBindingValue = "ssl.key_path"
	StackBindingValueSSLP12Path      StackBindingValue = "ssl.p12_path"
	StackBindingValueSSLPEMPath      StackBindingValue = "ssl.pem_path"
	StackBindingValuePrismaURL       StackBindingValue = "prisma_url"
	StackBindingValueEndpoint        StackBindingValue = "endpoint"
	StackBindingValueBucket          StackBindingValue = "bucket"
	StackBindingValueRegion          StackBindingValue = "region"
	StackBindingValuePrefix          StackBindingValue = "prefix"
	StackBindingValueAccessKeyID     StackBindingValue = "access_key_id"
	StackBindingValueSecretAccessKey StackBindingValue = "secret_access_key"
	StackBindingValuePublicBaseURL   StackBindingValue = "public_base_url"
)

// StackSnapshotReason is why a stack snapshot was taken.
type StackSnapshotReason string

const (
	StackSnapshotReasonManual StackSnapshotReason = "manual"
	// StackSnapshotReasonUpdate is a safety snapshot taken before an update to a newer template version.
	StackSnapshotReasonUpdate StackSnapshotReason = "update"
	// StackSnapshotReasonDelete was taken when the stack and its members were deleted.
	StackSnapshotReasonDelete StackSnapshotReason = "delete"
)

// ---------------------------------------------------------------------------
// Payloads
// ---------------------------------------------------------------------------

// RestartNotice is added to the answer of every change that can alter the
// variables or the certificates an application reads. RestartRequired is true
// when at least one application needs a restart to see the change, and
// AppsToRestart lists them.
type RestartNotice struct {
	RestartRequired bool     `json:"restart_required"`
	AppsToRestart   []string `json:"apps_to_restart"`
}

// StackBlob is the Blob folder of a Blob member of a stack.
type StackBlob struct {
	// Prefix is the folder of the account's Blob, ending in "/".
	Prefix     string                 `json:"prefix"`
	Visibility TemplateBlobVisibility `json:"visibility"`
	// UsedBytes is nil when it is not available.
	UsedBytes *int64 `json:"used_bytes"`
	// APIKeyID is the id of the API key the platform created for the stack, restricted to the folder.
	APIKeyID *string `json:"api_key_id"`
}

// StackMember is an application, a database or a Blob folder of a stack.
type StackMember struct {
	ResourceType StackMemberType `json:"resource_type"`
	// ResourceID is the id of the application or database; for a Blob folder, the id of the member.
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	// Status is an ApplicationStatus or a DatabaseStatus value; nil for a Blob folder.
	Status *string `json:"status"`
	// RAM in MB; 0 for a Blob folder.
	RAM int `json:"ram"`
	// ServiceKey is the id of the service in the template the stack came from; nil for a member the user added.
	ServiceKey *string `json:"service_key"`
	// DatabaseType is the engine of a database member; nil otherwise.
	DatabaseType *DatabaseType `json:"database_type"`
	// Closed is true for an application of a closed template: its code is not accessible.
	Closed bool `json:"closed"`
	// Blob is set for a Blob member.
	Blob *StackBlob `json:"blob"`
}

// StackStatus is the members online, offline and building, added up.
type StackStatus struct {
	Up       int `json:"up"`
	Down     int `json:"down"`
	Building int `json:"building"`
	// Total counts the members that have a status (applications and databases).
	Total int `json:"total"`
}

// StackBinding is a connection between members. Exactly one of DatabaseID,
// BlobID and TargetAppID is set.
type StackBinding struct {
	ID      string `json:"id"`
	StackID string `json:"stack_id"`
	// AppID is the application that receives the variables.
	AppID      string  `json:"app_id"`
	DatabaseID *string `json:"database_id"`
	BlobID     *string `json:"blob_id"`
	// TargetAppID is another application the application calls; the application itself when the variable carries its own address.
	TargetAppID *string `json:"target_app_id"`
	// EnvMap maps a variable name to what it receives.
	EnvMap map[string]StackBindingValue `json:"env_map"`
	// CertsPath is the folder inside the application where the client certificates of the database are written; nil without a database.
	CertsPath *string   `json:"certs_path"`
	CreatedAt time.Time `json:"created_at"`
}

// Stack is a group of applications, databases and Blob folders with the
// connections between them.
type Stack struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description *string          `json:"description"`
	NetworkMode StackNetworkMode `json:"network_mode"`
	OwnerID     string           `json:"owner_id"`
	// WorkspaceID is nil when the stack belongs to the user.
	WorkspaceID *string       `json:"workspace_id"`
	Members     []StackMember `json:"members"`
	// Status is the status of the members, added up.
	Status StackStatus `json:"status"`
	// RAMTotal is the memory of every member, in MB.
	RAMTotal int            `json:"ram_total"`
	Bindings []StackBinding `json:"bindings"`
	// Template is the template and version the stack came from; nil for a stack assembled by hand.
	Template *TemplateOriginInfo `json:"template"`
	// UpdateAvailable is true when the template has a newer version the stack can update to.
	UpdateAvailable bool      `json:"update_available"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// StackChange is a stack together with the restart notice of the change that returned it.
type StackChange struct {
	Stack
	RestartNotice
}

// StackBindingChange is a connection together with the restart notice of the change that returned it.
type StackBindingChange struct {
	StackBinding
	RestartNotice
}

// StackDeleteResult is what was deleted together with the stack. Empty with "dissolve".
type StackDeleteResult struct {
	DeletedApps      []string `json:"deleted_apps"`
	DeletedDatabases []string `json:"deleted_databases"`
	// DeletedBlob is true when the files of the Blob folder were deleted.
	DeletedBlob bool `json:"deleted_blob"`
}

// StackSnapshotMember is the snapshot of one member of a stack snapshot.
type StackSnapshotMember struct {
	// ResourceType is "application" or "database".
	ResourceType WorkspaceResourceType `json:"resource_type"`
	ResourceID   string                `json:"resource_id"`
	SnapshotID   string                `json:"snapshot_id"`
}

// StackSnapshot is a snapshot of every member of a stack taken together.
type StackSnapshot struct {
	ID      string              `json:"id"`
	StackID string              `json:"stack_id"`
	Reason  StackSnapshotReason `json:"reason"`
	Date    time.Time           `json:"date"`
	// Size is the total size, human readable.
	Size string `json:"size"`
	// Members is the snapshot of each member that has data.
	Members []StackSnapshotMember `json:"members"`
	// Closed is true when the stack runs a closed template: the snapshot cannot be downloaded.
	Closed bool `json:"closed"`
}

// StackConnectionSnippet is one piece of code that connects an application to
// a member of a stack, with the real names of the variables the platform injected.
type StackConnectionSnippet struct {
	Language ApplicationLanguage `json:"language"`
	// Driver is the driver or client library the code uses.
	Driver string `json:"driver"`
	// DatabaseType is the engine of the database; nil for a Blob folder.
	DatabaseType *DatabaseType `json:"database_type"`
	// Blob is true for a Blob folder (S3-compatible client).
	Blob  bool   `json:"blob"`
	Title string `json:"title"`
	Code  string `json:"code"`
	// EnvVars are the names of the variables the code reads.
	EnvVars []string `json:"env_vars"`
}

// StackConnectionInfo is how to connect an application to the members it is linked to.
type StackConnectionInfo struct {
	AppID    string                   `json:"app_id"`
	Language ApplicationLanguage      `json:"language"`
	Snippets []StackConnectionSnippet `json:"snippets"`
}

// StackDatabaseAdded is the result of adding a database to a stack.
type StackDatabaseAdded struct {
	RestartNotice
	Stack      Stack  `json:"stack"`
	DatabaseID string `json:"database_id"`
}

// StackBlobAdded is the result of adding a Blob folder to a stack.
type StackBlobAdded struct {
	RestartNotice
	Stack Stack       `json:"stack"`
	Blob  StackMember `json:"blob"`
}

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

// StackBindingInput is a connection to create together with a member that
// joins a stack or with a new database or Blob folder.
type StackBindingInput struct {
	// AppID is the application of the stack that receives the variables.
	AppID string `json:"app_id"`
	// EnvMap maps variable names to what each receives. Default: the standard
	// names of the engine (DATABASE_URL, MONGODB_URI, REDIS_URL, MYSQL_URL),
	// with the service name as prefix from the second of an engine.
	EnvMap map[string]StackBindingValue `json:"env_map,omitempty"`
}

// StackMemberRef points at an application or database to put in a stack.
type StackMemberRef struct {
	// ResourceType is "application" or "database".
	ResourceType StackMemberType `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
}

// StackConnectionInput is a connection of StackCreateBody and StackMemberAddBody:
// the application that receives the variables, and the database or the other
// application it connects to.
type StackConnectionInput struct {
	StackBindingInput
	DatabaseID  string `json:"database_id,omitempty"`
	TargetAppID string `json:"target_app_id,omitempty"`
}

// StackCreateBody is the body of StacksService.Create.
type StackCreateBody struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// NetworkMode: only "public" is available for now. Default "public".
	NetworkMode StackNetworkMode `json:"network_mode,omitempty"`
	Members     []StackMemberRef `json:"members,omitempty"`
	// Bindings are the connections between the members.
	Bindings    []StackConnectionInput `json:"bindings,omitempty"`
	WorkspaceID string                 `json:"workspace_id,omitempty"`
}

// StackTransformConnection is a connection of StackTransformBody; DatabaseID is required.
type StackTransformConnection struct {
	StackBindingInput
	DatabaseID string `json:"database_id"`
}

// StackTransformBody is the body of StacksService.Transform.
type StackTransformBody struct {
	// FolderID is the folder to take the resources from.
	FolderID string `json:"folder_id"`
	// WorkspaceID is the workspace of the folder; empty means your own folder.
	WorkspaceID string `json:"workspace_id,omitempty"`
	Name        string `json:"name,omitempty"`
	// InjectVariables makes the platform add the connection variables and
	// certificates to the applications; without it they are assumed to
	// connect on their own.
	InjectVariables bool `json:"inject_variables"`
	// Bindings are the connections to create when InjectVariables is true.
	Bindings []StackTransformConnection `json:"bindings,omitempty"`
}

// StackUpdateBody is the body of StacksService.Update. At least one field is
// required. Name is a *string; Description is a *Nullable[string]:
// NullableValue("text") sets it, NullableNull[string]() removes it and nil
// leaves it untouched.
type StackUpdateBody struct {
	Name        *string           `json:"name,omitempty"`
	Description *Nullable[string] `json:"description,omitempty"`
}

// StackDeleteBody is the body of StacksService.Delete.
type StackDeleteBody struct {
	Mode StackDeleteMode `json:"mode"`
	// ConfirmName must be equal to the stack name (STACK_NAME_MISMATCH otherwise).
	ConfirmName string `json:"confirm_name"`
	// Blob is what to do with the files of the stack's Blob folder when Mode
	// is "delete_all". Default "keep"; "dissolve" always keeps them.
	Blob StackBlobDisposition `json:"blob,omitempty"`
}

// StackMemberAddBody is the body of StackMembersService.Add.
type StackMemberAddBody struct {
	// ResourceType is "application" or "database".
	ResourceType StackMemberType `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	// Bindings are the connections to create, to the applications of the stack. May be empty.
	Bindings []StackConnectionInput `json:"bindings,omitempty"`
}

// StackMemberRemoveParams are the optional parameters of StackMembersService.Remove.
type StackMemberRemoveParams struct {
	// Bindings is what happens to the variables the connections of the member
	// created. Default "keep_connection".
	Bindings StackBindingRemoval
	// Blob is what happens to the files of a Blob member. Default "keep".
	Blob StackBlobDisposition
}

// StackBindingCreateBody is the body of StackBindingsService.Create. Set
// exactly one of DatabaseID, BlobID and TargetAppID.
type StackBindingCreateBody struct {
	StackBindingInput
	DatabaseID string `json:"database_id,omitempty"`
	// BlobID is the id of the Blob member.
	BlobID      string `json:"blob_id,omitempty"`
	TargetAppID string `json:"target_app_id,omitempty"`
}

// StackBindingDeleteParams are the optional parameters of StackBindingsService.Delete.
type StackBindingDeleteParams struct {
	// Variables is what happens to the variables of the connection. Default "keep_connection".
	Variables StackBindingRemoval
}

// StackConnectionParams are the parameters of StacksService.Connection.
type StackConnectionParams struct {
	// AppID is the application to write the code for. Required.
	AppID string
	// Language of the code; default: the language of the application.
	Language ApplicationLanguage
}

// StackDatabaseBody is the body of StacksService.AddDatabase and AddDatabaseToApp.
type StackDatabaseBody struct {
	Type DatabaseType `json:"type"`
	Name string       `json:"name"`
	// RAM in MB.
	RAM         int    `json:"ram"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	// Bindings are the applications to connect it to.
	Bindings []StackBindingInput `json:"bindings,omitempty"`
	// StackName is only for AddDatabaseToApp on an application that is not in a stack.
	StackName string `json:"stack_name,omitempty"`
}

// StackBlobBody is the body of StacksService.AddBlob.
type StackBlobBody struct {
	// Prefix is the folder of the Blob, ending in "/". Without it the
	// platform picks a folder named after the stack.
	Prefix string `json:"prefix,omitempty"`
	// Visibility defaults to "private".
	Visibility TemplateBlobVisibility `json:"visibility,omitempty"`
	// Bindings are the applications to connect it to.
	Bindings []StackBindingInput `json:"bindings,omitempty"`
}

// StackSnapshotRestoreParams are the optional body fields of StackSnapshotsService.Restore.
type StackSnapshotRestoreParams struct {
	// RestoreImageVersion also takes the applications on a custom image back
	// to the image version of the snapshot, if it still exists. Default false.
	RestoreImageVersion bool `json:"restore_image_version,omitempty"`
}

// ---------------------------------------------------------------------------
// Implementation
// ---------------------------------------------------------------------------

func newStacksService(rc rest.Client) StacksService {
	return &stacksServiceImpl{rest: guardPaths(rc)}
}

type stacksServiceImpl struct{ rest rest.Client }

// stackPath builds /v1/stacks/:id<suffix>, URL-encoding id.
func stackPath(id, suffix string) string {
	return "/v1/stacks/" + url.PathEscape(id) + suffix
}

func (s *stacksServiceImpl) Members() StackMembersService {
	return &stackMembersServiceImpl{rest: s.rest}
}
func (s *stacksServiceImpl) Bindings() StackBindingsService {
	return &stackBindingsServiceImpl{rest: s.rest}
}
func (s *stacksServiceImpl) Snapshots() StackSnapshotsService {
	return &stackSnapshotsServiceImpl{rest: s.rest}
}

func (s *stacksServiceImpl) List(ctx context.Context, opts ...rest.RequestOpt) ([]Stack, error) {
	return templateCall[[]Stack](ctx, s.rest, http.MethodGet, "/v1/stacks", nil, nil, opts)
}

func (s *stacksServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (Stack, error) {
	return templateCall[Stack](ctx, s.rest, http.MethodGet, stackPath(id, ""), nil, nil, opts)
}

func (s *stacksServiceImpl) Create(ctx context.Context, body StackCreateBody, opts ...rest.RequestOpt) (StackChange, error) {
	return templateCall[StackChange](ctx, s.rest, http.MethodPost, "/v1/stacks", nil, body, opts)
}

func (s *stacksServiceImpl) Transform(ctx context.Context, body StackTransformBody, opts ...rest.RequestOpt) (StackChange, error) {
	return templateCall[StackChange](ctx, s.rest, http.MethodPost, "/v1/stacks/transform", nil, body, opts)
}

func (s *stacksServiceImpl) Update(ctx context.Context, id string, body StackUpdateBody, opts ...rest.RequestOpt) (Stack, error) {
	return templateCall[Stack](ctx, s.rest, http.MethodPatch, stackPath(id, ""), nil, body, opts)
}

func (s *stacksServiceImpl) Delete(ctx context.Context, id string, body StackDeleteBody, opts ...rest.RequestOpt) (StackDeleteResult, error) {
	return templateCall[StackDeleteResult](ctx, s.rest, http.MethodDelete, stackPath(id, ""), nil, body, opts)
}

func (s *stacksServiceImpl) Connection(ctx context.Context, id string, params StackConnectionParams, opts ...rest.RequestOpt) (StackConnectionInfo, error) {
	q := url.Values{}
	addQueryParam(q, "app_id", params.AppID)
	addQueryParam(q, "language", string(params.Language))
	return templateCall[StackConnectionInfo](ctx, s.rest, http.MethodGet, stackPath(id, "/connection"), q, nil, opts)
}

func (s *stacksServiceImpl) SetNetworkMode(ctx context.Context, id string, mode StackNetworkMode, opts ...rest.RequestOpt) (Stack, error) {
	return templateCall[Stack](ctx, s.rest, http.MethodPost, stackPath(id, "/network-mode"), nil, struct {
		NetworkMode StackNetworkMode `json:"network_mode"`
	}{mode}, opts)
}

func (s *stacksServiceImpl) AddDatabase(ctx context.Context, id string, body StackDatabaseBody, opts ...rest.RequestOpt) (StackDatabaseAdded, error) {
	return templateCall[StackDatabaseAdded](ctx, s.rest, http.MethodPost, stackPath(id, "/databases"), nil, body, opts)
}

func (s *stacksServiceImpl) AddDatabaseToApp(ctx context.Context, appID string, body StackDatabaseBody, opts ...rest.RequestOpt) (StackDatabaseAdded, error) {
	return templateCall[StackDatabaseAdded](ctx, s.rest, http.MethodPost, appPath(appID, "/stack/databases"), nil, body, opts)
}

func (s *stacksServiceImpl) AddBlob(ctx context.Context, id string, body StackBlobBody, opts ...rest.RequestOpt) (StackBlobAdded, error) {
	return templateCall[StackBlobAdded](ctx, s.rest, http.MethodPost, stackPath(id, "/blob"), nil, body, opts)
}

// --- members ---

type stackMembersServiceImpl struct{ rest rest.Client }

func (s *stackMembersServiceImpl) Add(ctx context.Context, stackID string, body StackMemberAddBody, opts ...rest.RequestOpt) (StackChange, error) {
	return templateCall[StackChange](ctx, s.rest, http.MethodPost, stackPath(stackID, "/members"), nil, body, opts)
}

func (s *stackMembersServiceImpl) Remove(ctx context.Context, stackID string, resourceType StackMemberType, resourceID string, params *StackMemberRemoveParams, opts ...rest.RequestOpt) (StackChange, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "bindings", string(params.Bindings))
		addQueryParam(q, "blob", string(params.Blob))
	}
	path := stackPath(stackID, "/members/"+url.PathEscape(string(resourceType))+"/"+url.PathEscape(resourceID))
	return templateCall[StackChange](ctx, s.rest, http.MethodDelete, path, q, nil, opts)
}

// --- bindings ---

type stackBindingsServiceImpl struct{ rest rest.Client }

func (s *stackBindingsServiceImpl) List(ctx context.Context, stackID string, opts ...rest.RequestOpt) ([]StackBinding, error) {
	return templateCall[[]StackBinding](ctx, s.rest, http.MethodGet, stackPath(stackID, "/bindings"), nil, nil, opts)
}

func (s *stackBindingsServiceImpl) Create(ctx context.Context, stackID string, body StackBindingCreateBody, opts ...rest.RequestOpt) (StackBindingChange, error) {
	return templateCall[StackBindingChange](ctx, s.rest, http.MethodPost, stackPath(stackID, "/bindings"), nil, body, opts)
}

func (s *stackBindingsServiceImpl) Update(ctx context.Context, stackID, bindingID string, envMap map[string]StackBindingValue, opts ...rest.RequestOpt) (StackBindingChange, error) {
	return templateCall[StackBindingChange](ctx, s.rest, http.MethodPatch, stackPath(stackID, "/bindings/"+url.PathEscape(bindingID)), nil, struct {
		EnvMap map[string]StackBindingValue `json:"env_map"`
	}{envMap}, opts)
}

func (s *stackBindingsServiceImpl) Delete(ctx context.Context, stackID, bindingID string, params *StackBindingDeleteParams, opts ...rest.RequestOpt) (RestartNotice, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "variables", string(params.Variables))
	}
	return templateCall[RestartNotice](ctx, s.rest, http.MethodDelete, stackPath(stackID, "/bindings/"+url.PathEscape(bindingID)), q, nil, opts)
}

// --- snapshots ---

type stackSnapshotsServiceImpl struct{ rest rest.Client }

func (s *stackSnapshotsServiceImpl) List(ctx context.Context, stackID string, opts ...rest.RequestOpt) ([]StackSnapshot, error) {
	return templateCall[[]StackSnapshot](ctx, s.rest, http.MethodGet, stackPath(stackID, "/snapshots"), nil, nil, opts)
}

func (s *stackSnapshotsServiceImpl) Create(ctx context.Context, stackID string, opts ...rest.RequestOpt) (StackSnapshot, error) {
	return templateCall[StackSnapshot](ctx, s.rest, http.MethodPost, stackPath(stackID, "/snapshots"), nil, nil, opts)
}

func (s *stackSnapshotsServiceImpl) Restore(ctx context.Context, stackID, snapshotID string, params *StackSnapshotRestoreParams, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error) {
	var body any
	if params != nil {
		body = *params
	}
	return templateCall[SnapshotRestoreResponse](ctx, s.rest, http.MethodPost, stackPath(stackID, "/snapshots/"+url.PathEscape(snapshotID)+"/restore"), nil, body, opts)
}
