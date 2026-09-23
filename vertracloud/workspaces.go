package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// WorkspacesService groups every /v1/workspaces* route exposed to the public
// client (30 routes: workspaces, members, roles, invites, action requests,
// apps, databases, folders, favorites).
//
// Unlike AppsService/DatabasesService, Workspaces routes have no use for
// rest.WithWorkspaceID: the workspace is already identified by the :id path
// segment. Setting it here is redundant, though the transport still
// forwards it if set.
type WorkspacesService interface {
	// List: GET /v1/workspaces — scope workspaces:read.
	List(ctx context.Context, opts ...rest.RequestOpt) ([]Workspace, error)
	// Get: GET /v1/workspaces/:id — scope workspaces:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (WorkspaceInfo, error)
	// Create: POST /v1/workspaces — scope workspaces:write. Only the
	// caller's own plan gates this (Enterprise); the server enforces it.
	Create(ctx context.Context, body WorkspaceCreateBody, opts ...rest.RequestOpt) (WorkspaceInfo, error)
	// Update: PUT /v1/workspaces/:id — scope workspaces:write, owner only.
	Update(ctx context.Context, id string, body WorkspaceUpdateBody, opts ...rest.RequestOpt) (Workspace, error)
	// Delete: DELETE /v1/workspaces/:id — scope workspaces:delete, owner only.
	// This is a soft delete; the API keeps the workspace row for billing history.
	Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error
	Members() WorkspacesMembersService
	Roles() WorkspacesRolesService
	Invites() WorkspacesInvitesService
	ActionRequests() WorkspacesActionRequestsService
	Apps() WorkspacesAppsService
	Databases() WorkspacesDatabasesService
	Folders() WorkspacesFoldersService
	Favorites() WorkspacesFavoritesService
}

func newWorkspacesService(rc rest.Client) WorkspacesService {
	return &workspacesServiceImpl{rest: rc}
}

type workspacesServiceImpl struct{ rest rest.Client }

// workspaceJSONBody marshals v to an io.Reader suitable for rest.Client.Do,
// paired with the "application/json" content type.
func workspaceJSONBody(v any) (*bytes.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	return bytes.NewReader(b), nil
}

func (s *workspacesServiceImpl) List(ctx context.Context, opts ...rest.RequestOpt) ([]Workspace, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/workspaces", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]Workspace](data)
}

func (s *workspacesServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (WorkspaceInfo, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, workspacePath(id, ""), nil, nil, "", opts...)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return rest.DecodeJSON[WorkspaceInfo](data)
}

func (s *workspacesServiceImpl) Create(ctx context.Context, body WorkspaceCreateBody, opts ...rest.RequestOpt) (WorkspaceInfo, error) {
	r, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/workspaces", nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return rest.DecodeJSON[WorkspaceInfo](data)
}

func (s *workspacesServiceImpl) Update(ctx context.Context, id string, body WorkspaceUpdateBody, opts ...rest.RequestOpt) (Workspace, error) {
	r, err := workspaceJSONBody(body)
	if err != nil {
		return Workspace{}, err
	}
	data, err := s.rest.Do(ctx, http.MethodPut, workspacePath(id, ""), nil, r, "application/json", opts...)
	if err != nil {
		return Workspace{}, err
	}
	return rest.DecodeJSON[Workspace](data)
}

func (s *workspacesServiceImpl) Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error {
	_, err := s.rest.Do(ctx, http.MethodDelete, workspacePath(id, ""), nil, nil, "", opts...)
	return err
}

func (s *workspacesServiceImpl) Members() WorkspacesMembersService {
	return &workspacesMembersServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) Roles() WorkspacesRolesService {
	return &workspacesRolesServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) Invites() WorkspacesInvitesService {
	return &workspacesInvitesServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) ActionRequests() WorkspacesActionRequestsService {
	return &workspacesActionRequestsServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) Apps() WorkspacesAppsService {
	return &workspacesAppsServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) Databases() WorkspacesDatabasesService {
	return &workspacesDatabasesServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) Folders() WorkspacesFoldersService {
	return &workspacesFoldersServiceImpl{rest: s.rest}
}

func (s *workspacesServiceImpl) Favorites() WorkspacesFavoritesService {
	return &workspacesFavoritesServiceImpl{rest: s.rest}
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// WorkspacePermission is one of the 18
// granular permissions a workspace role can grant. The owner (owner_id)
// never needs one of these — owner-only actions (rename/delete workspace,
// transfer ownership, link/unlink a project, deploy webhook, web publish)
// check owner_id directly.
type WorkspacePermission string

const (
	WorkspacePermissionAppsRead             WorkspacePermission = "apps:read"
	WorkspacePermissionAppsManage           WorkspacePermission = "apps:manage"
	WorkspacePermissionAppsLifecycle        WorkspacePermission = "apps:lifecycle"
	WorkspacePermissionAppsEnvs             WorkspacePermission = "apps:envs"
	WorkspacePermissionAppsFiles            WorkspacePermission = "apps:files"
	WorkspacePermissionAppsDelete           WorkspacePermission = "apps:delete"
	WorkspacePermissionDatabasesRead        WorkspacePermission = "databases:read"
	WorkspacePermissionDatabasesManage      WorkspacePermission = "databases:manage"
	WorkspacePermissionDatabasesLifecycle   WorkspacePermission = "databases:lifecycle"
	WorkspacePermissionDatabasesCredentials WorkspacePermission = "databases:credentials"
	WorkspacePermissionDatabasesData        WorkspacePermission = "databases:data"
	WorkspacePermissionDatabasesDelete      WorkspacePermission = "databases:delete"
	WorkspacePermissionSnapshotsRead        WorkspacePermission = "snapshots:read"
	WorkspacePermissionSnapshotsManage      WorkspacePermission = "snapshots:manage"
	WorkspacePermissionMembersRead          WorkspacePermission = "members:read"
	WorkspacePermissionMembersManage        WorkspacePermission = "members:manage"
	WorkspacePermissionRolesManage          WorkspacePermission = "roles:manage"
	WorkspacePermissionActivitiesRead       WorkspacePermission = "activities:read"
)

// WorkspaceRolePreset is the seed role a
// workspace is created with. Renamable/deletable like any role — this is
// purely informational once set.
type WorkspaceRolePreset string

const (
	WorkspaceRolePresetAdmin     WorkspaceRolePreset = "admin"
	WorkspaceRolePresetDeveloper WorkspaceRolePreset = "developer"
	WorkspaceRolePresetOperator  WorkspaceRolePreset = "operator"
	WorkspaceRolePresetViewer    WorkspaceRolePreset = "viewer"
)

// WorkspaceResourceType is the kind of
// resource a folder/favorite entry points at, or that Workspaces.Apps /
// Workspaces.Databases links to a workspace.
type WorkspaceResourceType string

const (
	WorkspaceResourceTypeApplication WorkspaceResourceType = "application"
	WorkspaceResourceTypeDatabase    WorkspaceResourceType = "database"
)

// WorkspaceFolderColor is a wire enum.
type WorkspaceFolderColor string

const (
	WorkspaceFolderColorNeutral WorkspaceFolderColor = "neutral"
	WorkspaceFolderColorRed     WorkspaceFolderColor = "red"
	WorkspaceFolderColorOrange  WorkspaceFolderColor = "orange"
	WorkspaceFolderColorYellow  WorkspaceFolderColor = "yellow"
	WorkspaceFolderColorGreen   WorkspaceFolderColor = "green"
	WorkspaceFolderColorBlue    WorkspaceFolderColor = "blue"
	WorkspaceFolderColorPurple  WorkspaceFolderColor = "purple"
)

// WorkspaceResourceOrganizationScope is whether a WorkspaceResourceOrganization belongs to the
// caller's personal space (Account.Get/Update) or to one workspace
// (Workspaces.Get).
type WorkspaceResourceOrganizationScope string

const (
	WorkspaceResourceOrganizationScopePersonal  WorkspaceResourceOrganizationScope = "personal"
	WorkspaceResourceOrganizationScopeWorkspace WorkspaceResourceOrganizationScope = "workspace"
)

// ---------------------------------------------------------------------------
// Payload structs — resource organization
//
// These are shared by both this file's Workspaces.Folders/Favorites routes
// and account.go's Account.Folders/Favorites routes (identical wire
// shape).
// ---------------------------------------------------------------------------

type WorkspaceResourceRef struct {
	ResourceType WorkspaceResourceType `json:"resource_type"`
	ResourceID   string                `json:"resource_id"`
}

// WorkspaceFolderItem is one resource placed inside
// a WorkspaceResourceFolder.
type WorkspaceFolderItem struct {
	WorkspaceResourceRef
	Position int `json:"position"`
}

type WorkspaceResourceFolder struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Color     WorkspaceFolderColor  `json:"color"`
	Position  int                   `json:"position"`
	Resources []WorkspaceFolderItem `json:"resources"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
}

type WorkspaceFavorite struct {
	WorkspaceResourceRef
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
}

// WorkspaceResourceOrganization is the
// full folders+favorites tree for one scope. Returned by Account.Get,
// Account.Update, Workspaces.Get (as the .ResourceOrganization field), and
// by every Folders.AddResource/RemoveResource and Favorites.Add/Remove
// call (both Account's and Workspaces') — this Go SDK follows the
// published rest/v1 shape for those rather than inventing a narrower
// return type, matching the divergence note in ROUTE_MATRIX.md.
type WorkspaceResourceOrganization struct {
	Scope       WorkspaceResourceOrganizationScope `json:"scope"`
	UserID      string                             `json:"user_id"`
	WorkspaceID *string                            `json:"workspace_id"`
	Folders     []WorkspaceResourceFolder          `json:"folders"`
	Favorites   []WorkspaceFavorite                `json:"favorites"`
}

// WorkspaceInviteKind is the kind of invitation sent by a workspace owner or
// member with members:manage.
type WorkspaceInviteKind string

const (
	WorkspaceInviteKindEmail WorkspaceInviteKind = "email"
	WorkspaceInviteKindLink  WorkspaceInviteKind = "link"
)

// WorkspaceInvite is the API representation returned by invite listing and
// embedded in an invite creation response.
type WorkspaceInvite struct {
	ID            string              `json:"id"`
	WorkspaceID   string              `json:"workspace_id"`
	Kind          WorkspaceInviteKind `json:"kind"`
	Email         *string             `json:"email"`
	RoleID        string              `json:"role_id"`
	RoleName      string              `json:"role_name"`
	InvitedBy     WorkspaceInviteUser `json:"invited_by"`
	ExpiresAt     time.Time           `json:"expires_at"`
	AcceptedAt    *time.Time          `json:"accepted_at"`
	RevokedAt     *time.Time          `json:"revoked_at"`
	Uses          int                 `json:"uses"`
	MaxUses       *int                `json:"max_uses"`
	ExpiresInDays *int                `json:"expires_in_days"`
	CreatedAt     time.Time           `json:"created_at"`
}

type WorkspaceInviteUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// WorkspaceInvitePreview is what the recipient of an invite sees before
// accepting it.
type WorkspaceInvitePreview struct {
	Workspace WorkspaceInvitePreviewWorkspace `json:"workspace"`
	Inviter   WorkspaceInvitePreviewInviter   `json:"inviter"`
	RoleName  string                          `json:"role_name"`
	Kind      WorkspaceInviteKind             `json:"kind"`
	ExpiresAt time.Time                       `json:"expires_at"`
}

type WorkspaceInvitePreviewWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WorkspaceInvitePreviewInviter struct {
	DisplayName string `json:"display_name"`
}

// ---------------------------------------------------------------------------
// Payload structs — action requests
// ---------------------------------------------------------------------------

// WorkspaceActionRequestAction is the sensitive action a member asks someone
// with the matching permission to perform.
type WorkspaceActionRequestAction string

const (
	WorkspaceActionRequestActionAppDelete       WorkspaceActionRequestAction = "app_delete"
	WorkspaceActionRequestActionDatabaseDelete  WorkspaceActionRequestAction = "database_delete"
	WorkspaceActionRequestActionSnapshotCreate  WorkspaceActionRequestAction = "snapshot_create"
	WorkspaceActionRequestActionSnapshotRestore WorkspaceActionRequestAction = "snapshot_restore"
)

// WorkspaceActionRequestStatus is the lifecycle state of an action request.
// A pending request expires after 24 hours without a decision.
type WorkspaceActionRequestStatus string

const (
	WorkspaceActionRequestStatusPending  WorkspaceActionRequestStatus = "pending"
	WorkspaceActionRequestStatusApproved WorkspaceActionRequestStatus = "approved"
	WorkspaceActionRequestStatusRejected WorkspaceActionRequestStatus = "rejected"
	WorkspaceActionRequestStatusExpired  WorkspaceActionRequestStatus = "expired"
)

// WorkspaceActionRequest is a request, made by a member without the needed
// permission, for someone who has it to perform a sensitive action.
type WorkspaceActionRequest struct {
	ID           string                       `json:"id"`
	WorkspaceID  string                       `json:"workspace_id"`
	Action       WorkspaceActionRequestAction `json:"action"`
	ResourceType WorkspaceResourceType        `json:"resource_type"`
	ResourceID   string                       `json:"resource_id"`
	ResourceName *string                      `json:"resource_name"`
	Params       WorkspaceActionRequestParams `json:"params"`
	Status       WorkspaceActionRequestStatus `json:"status"`
	RequestedBy  WorkspaceInviteUser          `json:"requested_by"`
	DecidedBy    *WorkspaceInviteUser         `json:"decided_by"`
	DecidedAt    *time.Time                   `json:"decided_at"`
	ExpiresAt    time.Time                    `json:"expires_at"`
	CreatedAt    time.Time                    `json:"created_at"`
}

// WorkspaceActionRequestParams is the action-specific params union.
type WorkspaceActionRequestParams interface {
	workspaceActionRequestParams()
}

type WorkspaceActionRequestEmptyParams struct{}

func (WorkspaceActionRequestEmptyParams) workspaceActionRequestParams() {}

type WorkspaceActionRequestSnapshotRestoreParams struct {
	SnapshotID string `json:"snapshot_id"`
}

func (WorkspaceActionRequestSnapshotRestoreParams) workspaceActionRequestParams() {}

func (r *WorkspaceActionRequest) UnmarshalJSON(data []byte) error {
	var wire struct {
		ID           string                       `json:"id"`
		WorkspaceID  string                       `json:"workspace_id"`
		Action       WorkspaceActionRequestAction `json:"action"`
		ResourceType WorkspaceResourceType        `json:"resource_type"`
		ResourceID   string                       `json:"resource_id"`
		ResourceName *string                      `json:"resource_name"`
		Params       json.RawMessage              `json:"params"`
		Status       WorkspaceActionRequestStatus `json:"status"`
		RequestedBy  WorkspaceInviteUser          `json:"requested_by"`
		DecidedBy    *WorkspaceInviteUser         `json:"decided_by"`
		DecidedAt    *time.Time                   `json:"decided_at"`
		ExpiresAt    time.Time                    `json:"expires_at"`
		CreatedAt    time.Time                    `json:"created_at"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*r = WorkspaceActionRequest{ID: wire.ID, WorkspaceID: wire.WorkspaceID, Action: wire.Action, ResourceType: wire.ResourceType, ResourceID: wire.ResourceID, ResourceName: wire.ResourceName, Status: wire.Status, RequestedBy: wire.RequestedBy, DecidedBy: wire.DecidedBy, DecidedAt: wire.DecidedAt, ExpiresAt: wire.ExpiresAt, CreatedAt: wire.CreatedAt}
	if wire.Action == WorkspaceActionRequestActionSnapshotRestore {
		var params WorkspaceActionRequestSnapshotRestoreParams
		if err := json.Unmarshal(wire.Params, &params); err != nil {
			return err
		}
		r.Params = params
	} else {
		r.Params = WorkspaceActionRequestEmptyParams{}
	}
	return nil
}

// WorkspaceActionRequestCreateBody is the body of POST
// /v1/workspaces/:id/action-requests. Params carries the action-specific
// union, including an empty object for actions with no extra fields.
type WorkspaceActionRequestCreateBody struct {
	Action     WorkspaceActionRequestAction `json:"action"`
	ResourceID string                       `json:"resource_id"`
	Params     WorkspaceActionRequestParams `json:"-"`
}

func (b WorkspaceActionRequestCreateBody) MarshalJSON() ([]byte, error) {
	params := b.Params
	if params == nil {
		params = WorkspaceActionRequestEmptyParams{}
	}
	switch b.Action {
	case WorkspaceActionRequestActionSnapshotRestore:
		if _, ok := params.(WorkspaceActionRequestSnapshotRestoreParams); !ok {
			return nil, fmt.Errorf("snapshot_restore requires snapshot restore params")
		}
	default:
		if _, ok := params.(WorkspaceActionRequestEmptyParams); !ok {
			return nil, fmt.Errorf("action %q only accepts empty params", b.Action)
		}
	}
	return json.Marshal(struct {
		Action     WorkspaceActionRequestAction `json:"action"`
		ResourceID string                       `json:"resource_id"`
		Params     WorkspaceActionRequestParams `json:"params"`
	}{Action: b.Action, ResourceID: b.ResourceID, Params: params})
}

// WorkspaceActionRequestListParams are the optional filters of
// WorkspacesActionRequestsService.List.
type WorkspaceActionRequestListParams struct {
	// Status filters by lifecycle state; empty returns every status.
	Status WorkspaceActionRequestStatus
}

// ---------------------------------------------------------------------------
// Payload structs — workspace/role/member
// ---------------------------------------------------------------------------

type WorkspaceRole struct {
	ID           string                `json:"id"`
	WorkspaceID  string                `json:"workspace_id"`
	Name         string                `json:"name"`
	Permissions  []WorkspacePermission `json:"permissions"`
	Preset       *WorkspaceRolePreset  `json:"preset"`
	Position     int                   `json:"position"`
	MembersCount int                   `json:"members_count"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// WorkspaceMemberUser is the embedded `user` object of WorkspaceMember.
type WorkspaceMemberUser struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type WorkspaceMember struct {
	UserID    string              `json:"user_id"`
	RoleID    string              `json:"role_id"`
	RoleName  string              `json:"role_name"`
	JoinedAt  time.Time           `json:"joined_at"`
	ExpiresAt *time.Time          `json:"expires_at"`
	User      WorkspaceMemberUser `json:"user"`
}

// WorkspaceOwner is the embedded `owner` object of Workspace.
type WorkspaceOwner struct {
	DisplayName string `json:"display_name"`
}

type Workspace struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Description  *string        `json:"description"`
	OwnerID      string         `json:"owner_id"`
	Owner        WorkspaceOwner `json:"owner"`
	MembersCount int            `json:"members_count"`
	// DeletedAt is non-nil on a soft-deleted workspace (name kept for the
	// billing ledger); see Workspaces.Delete.
	DeletedAt *time.Time `json:"deleted_at"`
	// Frozen is true when the owner's plan no longer includes workspaces.
	// Reads keep working; every write returns PLAN_RESTRICTED_FEATURE.
	Frozen    bool      `json:"frozen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkspaceInfo is the full shape returned by
// Workspaces.Get — the workspace plus its members, roles, linked
// apps/databases, the caller's resolved permissions, and the caller's own
// resource organization within it.
type WorkspaceInfo struct {
	Workspace
	Members              []WorkspaceMember             `json:"members"`
	Roles                []WorkspaceRole               `json:"roles"`
	Applications         []Application                 `json:"applications"`
	Databases            []Database                    `json:"databases"`
	Permissions          []WorkspacePermission         `json:"permissions"`
	IsOwner              bool                          `json:"is_owner"`
	OwnerPlanID          UserPlan                      `json:"owner_plan_id"`
	ResourceOrganization WorkspaceResourceOrganization `json:"resource_organization"`
}

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

// WorkspaceCreateBody is the body of POST /v1/workspaces and of PUT
// /v1/workspaces/:id.
type WorkspaceCreateBody struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// WorkspaceUpdateBody is the partial body of PUT /v1/workspaces/:id.
type WorkspaceUpdateBody struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// WorkspaceInviteAccepted is the response from POST /v1/workspace-invites/:token/accept.
type WorkspaceInviteAccepted struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkspaceMemberUpdateBody is the body of PUT
// /v1/workspaces/:id/members/:user_id.
//
// A nil ExpiresAt leaves the member's access expiration untouched;
// NullableNull clears it.
type WorkspaceMemberUpdateBody struct {
	RoleID    string               `json:"role_id,omitempty"`
	ExpiresAt *Nullable[time.Time] `json:"expires_at,omitempty"`
}

// WorkspaceRoleBody is the body of POST /v1/workspaces/:id/roles and PUT
// /v1/workspaces/:id/roles/:role_id.
type WorkspaceRoleBody struct {
	Name        string                `json:"name"`
	Permissions []WorkspacePermission `json:"permissions"`
	Position    *int                  `json:"position,omitempty"`
}

// ResourceFolderCreateBody is the body of POST
// .../resource-organization/folders, shared by Account.Folders.Create and
// Workspaces.Folders.Create.
type ResourceFolderCreateBody struct {
	Name     string               `json:"name"`
	Color    WorkspaceFolderColor `json:"color,omitempty"`
	Position *int                 `json:"position,omitempty"`
}

// ResourceFolderUpdateBody is the body of PATCH .../folders/:folder_id,
// shared by Account.Folders.Update and Workspaces.Folders.Update.
type ResourceFolderUpdateBody struct {
	Name     *string              `json:"name,omitempty"`
	Color    WorkspaceFolderColor `json:"color,omitempty"`
	Position *int                 `json:"position,omitempty"`
}

// ResourcePositionBody is the optional body of every folder-resource /
// favorite PUT route (Folders.AddResource, Favorites.Add): only an
// optional display Position. A nil body is sent as an empty {} object.
type ResourcePositionBody struct {
	Position *int `json:"position,omitempty"`
}

// ---------------------------------------------------------------------------
// path helpers
// ---------------------------------------------------------------------------

// workspacePath builds /v1/workspaces/:id<suffix>, URL-encoding id.
func workspacePath(id, suffix string) string {
	return "/v1/workspaces/" + url.PathEscape(id) + suffix
}

// workspaceFolderPath builds
// /v1/workspaces/:id/resource-organization/folders/:folder_id<suffix>.
func workspaceFolderPath(workspaceID, folderID, suffix string) string {
	return workspacePath(workspaceID, "/resource-organization/folders/"+url.PathEscape(folderID)+suffix)
}

// resourceTypeIDSuffix builds the :resource_type/:resource_id suffix
// shared by both Account and Workspaces folders/favorites routes.
func resourceTypeIDSuffix(resourceType WorkspaceResourceType, resourceID string) string {
	return url.PathEscape(string(resourceType)) + "/" + url.PathEscape(resourceID)
}

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

// WorkspacesMembersService groups the /v1/workspaces/:id/members* routes.
type WorkspacesMembersService interface {
	// List: GET /v1/workspaces/:id/members — scope workspaces:read.
	List(ctx context.Context, workspaceID string, opts ...rest.RequestOpt) ([]WorkspaceMember, error)
	// Update: PUT /v1/workspaces/:id/members/:user_id — scope
	// workspaces:write.
	Update(ctx context.Context, workspaceID, userID string, body WorkspaceMemberUpdateBody, opts ...rest.RequestOpt) (WorkspaceMember, error)
	// Remove: DELETE /v1/workspaces/:id/members/:user_id — scope
	// workspaces:write, or the member themselves leaving.
	Remove(ctx context.Context, workspaceID, userID string, opts ...rest.RequestOpt) error
}

type workspacesMembersServiceImpl struct{ rest rest.Client }

func (m *workspacesMembersServiceImpl) List(ctx context.Context, workspaceID string, opts ...rest.RequestOpt) ([]WorkspaceMember, error) {
	data, err := m.rest.Do(ctx, http.MethodGet, workspacePath(workspaceID, "/members"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]WorkspaceMember](data)
}

func (m *workspacesMembersServiceImpl) Update(ctx context.Context, workspaceID, userID string, body WorkspaceMemberUpdateBody, opts ...rest.RequestOpt) (WorkspaceMember, error) {
	r, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceMember{}, err
	}
	path := workspacePath(workspaceID, "/members/"+url.PathEscape(userID))
	data, err := m.rest.Do(ctx, http.MethodPut, path, nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceMember{}, err
	}
	return rest.DecodeJSON[WorkspaceMember](data)
}

func (m *workspacesMembersServiceImpl) Remove(ctx context.Context, workspaceID, userID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/members/"+url.PathEscape(userID))
	_, err := m.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Roles
// ---------------------------------------------------------------------------

// WorkspacesRolesService groups the /v1/workspaces/:id/roles* routes.
type WorkspacesRolesService interface {
	// List: GET /v1/workspaces/:id/roles — scope workspaces:read.
	List(ctx context.Context, workspaceID string, opts ...rest.RequestOpt) ([]WorkspaceRole, error)
	// Create: POST /v1/workspaces/:id/roles — scope workspaces:write.
	Create(ctx context.Context, workspaceID string, body WorkspaceRoleBody, opts ...rest.RequestOpt) (WorkspaceRole, error)
	// Update: PUT /v1/workspaces/:id/roles/:role_id — scope
	// workspaces:write.
	Update(ctx context.Context, workspaceID, roleID string, body WorkspaceRoleBody, opts ...rest.RequestOpt) (WorkspaceRole, error)
	// Delete: DELETE /v1/workspaces/:id/roles/:role_id — scope
	// workspaces:write. A role in use by a member or pending invite
	// refuses with WORKSPACE_ROLE_IN_USE.
	Delete(ctx context.Context, workspaceID, roleID string, opts ...rest.RequestOpt) error
}

type workspacesRolesServiceImpl struct{ rest rest.Client }

func (r *workspacesRolesServiceImpl) List(ctx context.Context, workspaceID string, opts ...rest.RequestOpt) ([]WorkspaceRole, error) {
	data, err := r.rest.Do(ctx, http.MethodGet, workspacePath(workspaceID, "/roles"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]WorkspaceRole](data)
}

func (r *workspacesRolesServiceImpl) Create(ctx context.Context, workspaceID string, body WorkspaceRoleBody, opts ...rest.RequestOpt) (WorkspaceRole, error) {
	rd, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceRole{}, err
	}
	data, err := r.rest.Do(ctx, http.MethodPost, workspacePath(workspaceID, "/roles"), nil, rd, "application/json", opts...)
	if err != nil {
		return WorkspaceRole{}, err
	}
	return rest.DecodeJSON[WorkspaceRole](data)
}

func (r *workspacesRolesServiceImpl) Update(ctx context.Context, workspaceID, roleID string, body WorkspaceRoleBody, opts ...rest.RequestOpt) (WorkspaceRole, error) {
	rd, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceRole{}, err
	}
	path := workspacePath(workspaceID, "/roles/"+url.PathEscape(roleID))
	data, err := r.rest.Do(ctx, http.MethodPut, path, nil, rd, "application/json", opts...)
	if err != nil {
		return WorkspaceRole{}, err
	}
	return rest.DecodeJSON[WorkspaceRole](data)
}

func (r *workspacesRolesServiceImpl) Delete(ctx context.Context, workspaceID, roleID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/roles/"+url.PathEscape(roleID))
	_, err := r.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Apps / Databases — link existing resources to a workspace
// ---------------------------------------------------------------------------

// WorkspacesAppsService groups the /v1/workspaces/:id/apps/:app_id routes:
// linking/unlinking an existing application to a workspace, not the Apps
// domain itself.
type WorkspacesAppsService interface {
	// Add: POST /v1/workspaces/:id/apps/:app_id — scope workspaces:write,
	// only the app's owner may link it.
	Add(ctx context.Context, workspaceID, appID string, opts ...rest.RequestOpt) error
	// Remove: DELETE /v1/workspaces/:id/apps/:app_id — scope
	// workspaces:write.
	Remove(ctx context.Context, workspaceID, appID string, opts ...rest.RequestOpt) error
}

type workspacesAppsServiceImpl struct{ rest rest.Client }

func (a *workspacesAppsServiceImpl) Add(ctx context.Context, workspaceID, appID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/apps/"+url.PathEscape(appID))
	_, err := a.rest.Do(ctx, http.MethodPost, path, nil, nil, "", opts...)
	return err
}

func (a *workspacesAppsServiceImpl) Remove(ctx context.Context, workspaceID, appID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/apps/"+url.PathEscape(appID))
	_, err := a.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	return err
}

// WorkspacesDatabasesService groups the /v1/workspaces/:id/databases/:db_id
// routes: linking/unlinking an existing database to a workspace, not the
// Databases domain itself.
type WorkspacesDatabasesService interface {
	// Add: POST /v1/workspaces/:id/databases/:db_id — scope
	// workspaces:write.
	Add(ctx context.Context, workspaceID, dbID string, opts ...rest.RequestOpt) error
	// Remove: DELETE /v1/workspaces/:id/databases/:db_id — scope
	// workspaces:write.
	Remove(ctx context.Context, workspaceID, dbID string, opts ...rest.RequestOpt) error
}

type workspacesDatabasesServiceImpl struct{ rest rest.Client }

func (d *workspacesDatabasesServiceImpl) Add(ctx context.Context, workspaceID, dbID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/databases/"+url.PathEscape(dbID))
	_, err := d.rest.Do(ctx, http.MethodPost, path, nil, nil, "", opts...)
	return err
}

func (d *workspacesDatabasesServiceImpl) Remove(ctx context.Context, workspaceID, dbID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/databases/"+url.PathEscape(dbID))
	_, err := d.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	return err
}

// WorkspacesInvitesService groups the invitation routes: listing and revoking
// a workspace's invites, and previewing, accepting or declining an invite
// received by token. Creating invites is only available in the dashboard.
type WorkspacesInvitesService interface {
	// List: GET /v1/workspaces/:id/invites — scope workspaces:invites.
	List(ctx context.Context, workspaceID string, opts ...rest.RequestOpt) ([]WorkspaceInvite, error)
	// Revoke: DELETE /v1/workspaces/:id/invites/:invite_id — scope workspaces:invites.
	Delete(ctx context.Context, workspaceID, inviteID string, opts ...rest.RequestOpt) error
	// Preview: GET /v1/workspaces/invites/:token — scope workspaces:invites.
	Preview(ctx context.Context, token string, opts ...rest.RequestOpt) (WorkspaceInvitePreview, error)
	// Accept: POST /v1/workspaces/invites/:token/accept — scope
	// workspaces:invites. Returns the workspace the caller joined.
	Accept(ctx context.Context, token string, opts ...rest.RequestOpt) (WorkspaceInviteAccepted, error)
	// Decline: POST /v1/workspaces/invites/:token/decline — scope workspaces:invites.
	Decline(ctx context.Context, token string, opts ...rest.RequestOpt) error
}

type workspacesInvitesServiceImpl struct{ rest rest.Client }

// invitePath builds /v1/workspaces/invites/:token<suffix>, URL-encoding token.
func invitePath(token, suffix string) string {
	return "/v1/workspaces/invites/" + url.PathEscape(token) + suffix
}

func (i *workspacesInvitesServiceImpl) List(ctx context.Context, workspaceID string, opts ...rest.RequestOpt) ([]WorkspaceInvite, error) {
	data, err := i.rest.Do(ctx, http.MethodGet, workspacePath(workspaceID, "/invites"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]WorkspaceInvite](data)
}

func (i *workspacesInvitesServiceImpl) Delete(ctx context.Context, workspaceID, inviteID string, opts ...rest.RequestOpt) error {
	path := workspacePath(workspaceID, "/invites/"+url.PathEscape(inviteID))
	_, err := i.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	return err
}

func (i *workspacesInvitesServiceImpl) Preview(ctx context.Context, token string, opts ...rest.RequestOpt) (WorkspaceInvitePreview, error) {
	data, err := i.rest.Do(ctx, http.MethodGet, invitePath(token, ""), nil, nil, "", opts...)
	if err != nil {
		return WorkspaceInvitePreview{}, err
	}
	return rest.DecodeJSON[WorkspaceInvitePreview](data)
}

func (i *workspacesInvitesServiceImpl) Accept(ctx context.Context, token string, opts ...rest.RequestOpt) (WorkspaceInviteAccepted, error) {
	data, err := i.rest.Do(ctx, http.MethodPost, invitePath(token, "/accept"), nil, nil, "", opts...)
	if err != nil {
		return WorkspaceInviteAccepted{}, err
	}
	return rest.DecodeJSON[WorkspaceInviteAccepted](data)
}

func (i *workspacesInvitesServiceImpl) Decline(ctx context.Context, token string, opts ...rest.RequestOpt) error {
	_, err := i.rest.Do(ctx, http.MethodPost, invitePath(token, "/decline"), nil, nil, "", opts...)
	return err
}

// ---------------------------------------------------------------------------
// Action requests
// ---------------------------------------------------------------------------

// WorkspacesActionRequestsService groups the /v1/workspaces/:id/action-requests
// routes. Approving or rejecting a request is only available in the dashboard.
type WorkspacesActionRequestsService interface {
	// List: GET /v1/workspaces/:id/action-requests — scope workspaces:read.
	// params may be nil.
	List(ctx context.Context, workspaceID string, params *WorkspaceActionRequestListParams, opts ...rest.RequestOpt) ([]WorkspaceActionRequest, error)
	// Create: POST /v1/workspaces/:id/action-requests — scope workspaces:write.
	Create(ctx context.Context, workspaceID string, body WorkspaceActionRequestCreateBody, opts ...rest.RequestOpt) (WorkspaceActionRequest, error)
}

type workspacesActionRequestsServiceImpl struct{ rest rest.Client }

func (a *workspacesActionRequestsServiceImpl) List(ctx context.Context, workspaceID string, params *WorkspaceActionRequestListParams, opts ...rest.RequestOpt) ([]WorkspaceActionRequest, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "status", string(params.Status))
	}
	data, err := a.rest.Do(ctx, http.MethodGet, workspacePath(workspaceID, "/action-requests"), q, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]WorkspaceActionRequest](data)
}

func (a *workspacesActionRequestsServiceImpl) Create(ctx context.Context, workspaceID string, body WorkspaceActionRequestCreateBody, opts ...rest.RequestOpt) (WorkspaceActionRequest, error) {
	r, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceActionRequest{}, err
	}
	data, err := a.rest.Do(ctx, http.MethodPost, workspacePath(workspaceID, "/action-requests"), nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceActionRequest{}, err
	}
	return rest.DecodeJSON[WorkspaceActionRequest](data)
}

// ---------------------------------------------------------------------------
// Folders / Favorites
// ---------------------------------------------------------------------------

// WorkspacesFoldersService groups the
// /v1/workspaces/:id/resource-organization/folders* routes.
type WorkspacesFoldersService interface {
	// Create: POST /v1/workspaces/:id/resource-organization/folders —
	// scope workspaces:write.
	Create(ctx context.Context, workspaceID string, body ResourceFolderCreateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error)
	// Update: PATCH .../resource-organization/folders/:folder_id — scope
	// workspaces:write.
	Update(ctx context.Context, workspaceID, folderID string, body ResourceFolderUpdateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error)
	// Delete: DELETE .../resource-organization/folders/:folder_id — scope
	// workspaces:write.
	Delete(ctx context.Context, workspaceID, folderID string, opts ...rest.RequestOpt) error
	// AddResource: PUT
	// .../folders/:folder_id/resources/:resource_type/:resource_id —
	// scope workspaces:write. body may be nil (sent as {}).
	AddResource(ctx context.Context, workspaceID, folderID string, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
	// RemoveResource: DELETE
	// .../folders/:folder_id/resources/:resource_type/:resource_id —
	// scope workspaces:write.
	RemoveResource(ctx context.Context, workspaceID, folderID string, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
}

type workspacesFoldersServiceImpl struct{ rest rest.Client }

func (f *workspacesFoldersServiceImpl) Create(ctx context.Context, workspaceID string, body ResourceFolderCreateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error) {
	r, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	path := workspacePath(workspaceID, "/resource-organization/folders")
	data, err := f.rest.Do(ctx, http.MethodPost, path, nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceFolder](data)
}

func (f *workspacesFoldersServiceImpl) Update(ctx context.Context, workspaceID, folderID string, body ResourceFolderUpdateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error) {
	r, err := workspaceJSONBody(body)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	data, err := f.rest.Do(ctx, http.MethodPatch, workspaceFolderPath(workspaceID, folderID, ""), nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceFolder](data)
}

func (f *workspacesFoldersServiceImpl) Delete(ctx context.Context, workspaceID, folderID string, opts ...rest.RequestOpt) error {
	_, err := f.rest.Do(ctx, http.MethodDelete, workspaceFolderPath(workspaceID, folderID, ""), nil, nil, "", opts...)
	return err
}

func (f *workspacesFoldersServiceImpl) AddResource(ctx context.Context, workspaceID, folderID string, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	var b ResourcePositionBody
	if body != nil {
		b = *body
	}
	r, err := workspaceJSONBody(b)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	path := workspaceFolderPath(workspaceID, folderID, "/resources/"+resourceTypeIDSuffix(resourceType, resourceID))
	data, err := f.rest.Do(ctx, http.MethodPut, path, nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}

func (f *workspacesFoldersServiceImpl) RemoveResource(ctx context.Context, workspaceID, folderID string, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	path := workspaceFolderPath(workspaceID, folderID, "/resources/"+resourceTypeIDSuffix(resourceType, resourceID))
	data, err := f.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}

// WorkspacesFavoritesService groups the
// /v1/workspaces/:id/resource-organization/favorites* routes.
type WorkspacesFavoritesService interface {
	// Add: PUT
	// .../resource-organization/favorites/:resource_type/:resource_id —
	// scope workspaces:write. body may be nil (sent as {}).
	Add(ctx context.Context, workspaceID string, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
	// Remove: DELETE
	// .../resource-organization/favorites/:resource_type/:resource_id —
	// scope workspaces:write.
	Remove(ctx context.Context, workspaceID string, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
}

type workspacesFavoritesServiceImpl struct{ rest rest.Client }

func (fav *workspacesFavoritesServiceImpl) Add(ctx context.Context, workspaceID string, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	var b ResourcePositionBody
	if body != nil {
		b = *body
	}
	r, err := workspaceJSONBody(b)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	path := workspacePath(workspaceID, "/resource-organization/favorites/"+resourceTypeIDSuffix(resourceType, resourceID))
	data, err := fav.rest.Do(ctx, http.MethodPut, path, nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}

func (fav *workspacesFavoritesServiceImpl) Remove(ctx context.Context, workspaceID string, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	path := workspacePath(workspaceID, "/resource-organization/favorites/"+resourceTypeIDSuffix(resourceType, resourceID))
	data, err := fav.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}
