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

// AccountService groups every /v1/users/me* route (10 routes: profile,
// sessions, folders, favorites).
//
// There is no Account.Activity: the activity log is WEBSITE_ONLY by
// product decision (21/09/2026) and is not in the api-key scope catalog —
// do not add one without a matching ROUTE_MATRIX.md entry and scope.
type AccountService interface {
	// Get: GET /v1/users/me — scope account:read. There is no GET /v1/apps
	// or /v1/databases: the caller's own apps/databases come from here
	// (.Applications/.Databases) or from Apps.StatusAll/Databases.StatusAll.
	Get(ctx context.Context, opts ...rest.RequestOpt) (AccountInfo, error)
	// Update: PATCH /v1/users/me — scope account:write.
	Update(ctx context.Context, body AccountUpdateBody, opts ...rest.RequestOpt) (APIUser, error)
	Sessions() AccountSessionsService
	Folders() AccountFoldersService
	Favorites() AccountFavoritesService
}

func newAccountService(rc rest.Client) AccountService { return &accountServiceImpl{rest: rc} }

type accountServiceImpl struct{ rest rest.Client }

// accountJSONBody marshals v to an io.Reader suitable for rest.Client.Do,
// paired with the "application/json" content type.
func accountJSONBody(v any) (*bytes.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	return bytes.NewReader(b), nil
}

func (s *accountServiceImpl) Get(ctx context.Context, opts ...rest.RequestOpt) (AccountInfo, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/users/me", nil, nil, "", opts...)
	if err != nil {
		return AccountInfo{}, err
	}
	return rest.DecodeJSON[AccountInfo](data)
}

func (s *accountServiceImpl) Update(ctx context.Context, body AccountUpdateBody, opts ...rest.RequestOpt) (APIUser, error) {
	r, err := accountJSONBody(body)
	if err != nil {
		return APIUser{}, err
	}
	data, err := s.rest.Do(ctx, http.MethodPatch, "/v1/users/me", nil, r, "application/json", opts...)
	if err != nil {
		return APIUser{}, err
	}
	return rest.DecodeJSON[APIUser](data)
}

func (s *accountServiceImpl) Sessions() AccountSessionsService {
	return &accountSessionsServiceImpl{rest: s.rest}
}

func (s *accountServiceImpl) Folders() AccountFoldersService {
	return &accountFoldersServiceImpl{rest: s.rest}
}

func (s *accountServiceImpl) Favorites() AccountFavoritesService {
	return &accountFavoritesServiceImpl{rest: s.rest}
}

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

// APIUser is the authenticated user's own profile response.
type APIUser struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Email    string       `json:"email"`
	PlanID   UserPlan     `json:"plan_id"`
	Language UserLanguage `json:"language"`
	// WorkspaceInvitesEnabled: accepts a workspace invite by e-mail
	// (WORKSPACE_INVITES_DISABLED when off).
	WorkspaceInvitesEnabled bool      `json:"workspace_invites_enabled"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// UserLanguage is an account UI and email language.
type UserLanguage string

const (
	UserLanguagePtBR UserLanguage = "pt-br"
	UserLanguageEnUS UserLanguage = "en-us"
	UserLanguageEsES UserLanguage = "es-es"
)

type AccountConnection struct {
	Provider  string    `json:"provider"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type AccountPlanMemory struct {
	Limit int `json:"limit"`
	Used  int `json:"used"`
}

// UserPlan is a numeric plan identifier used on the API wire.
type UserPlan int

const (
	UserPlanFree         UserPlan = 1
	UserPlanEconomy      UserPlan = 2
	UserPlanPro          UserPlan = 3
	UserPlanScale        UserPlan = 4
	UserPlanEnterprise4  UserPlan = 5
	UserPlanEnterprise8  UserPlan = 6
	UserPlanEnterprise16 UserPlan = 7
	UserPlanEnterprise32 UserPlan = 8
	UserPlanIntermediary UserPlan = 9
	UserPlanEnterprise12 UserPlan = 10
	UserPlanEnterprise14 UserPlan = 11
	UserPlanEnterprise18 UserPlan = 12
	UserPlanEnterprise20 UserPlan = 13
	UserPlanEnterprise22 UserPlan = 14
	UserPlanEnterprise24 UserPlan = 15
	UserPlanEnterprise26 UserPlan = 16
	UserPlanEnterprise28 UserPlan = 17
	UserPlanEnterprise30 UserPlan = 18
	UserPlanEnterprise6  UserPlan = 19
)

type AccountPlan struct {
	ID        UserPlan          `json:"id"`
	Name      string            `json:"name"`
	ExpiresAt *time.Time        `json:"expires_at"`
	Duration  int               `json:"duration"`
	Memory    AccountPlanMemory `json:"memory"`
}

// AccountInfo is the full shape returned by
// Account.Get — the profile plus its plan, every
// personally-owned app/database, connections, and the caller's persisted
// resource organization (folders/favorites), independent of the local
// Flow groups.
type AccountInfo struct {
	APIUser
	Plan                 AccountPlan                   `json:"plan"`
	Applications         []Application                 `json:"applications"`
	Databases            []Database                    `json:"databases"`
	Connections          []AccountConnection           `json:"connections"`
	ResourceOrganization WorkspaceResourceOrganization `json:"resource_organization"`
}

type AccountSession struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Provider  string    `json:"provider"`
	IPAddress *string   `json:"ip_address"`
	Location  *string   `json:"location"`
	Device    *string   `json:"device"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsCurrent bool      `json:"is_current"`
}

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

// AccountUpdateBody is the body of PATCH /v1/users/me.
// Every field is optional.
type AccountUpdateBody struct {
	Name                    *string       `json:"name,omitempty"`
	Language                *UserLanguage `json:"language,omitempty"`
	WorkspaceInvitesEnabled *bool         `json:"workspace_invites_enabled,omitempty"`
}

// ---------------------------------------------------------------------------
// path helpers
// ---------------------------------------------------------------------------

// accountFolderPath builds
// /v1/users/me/folders/:folder_id<suffix>.
func accountFolderPath(folderID, suffix string) string {
	return "/v1/users/me/folders/" + url.PathEscape(folderID) + suffix
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// AccountSessionsService groups the /v1/users/me/sessions route.
type AccountSessionsService interface {
	// List: GET /v1/users/me/sessions — scope account:read.
	List(ctx context.Context, opts ...rest.RequestOpt) ([]AccountSession, error)
}

type accountSessionsServiceImpl struct{ rest rest.Client }

func (s *accountSessionsServiceImpl) List(ctx context.Context, opts ...rest.RequestOpt) ([]AccountSession, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/users/me/sessions", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]AccountSession](data)
}

// ---------------------------------------------------------------------------
// Folders / Favorites
// ---------------------------------------------------------------------------

// AccountFoldersService groups the
// /v1/users/me/folders* routes. Types
// (WorkspaceResourceFolder, ResourceFolderCreateBody, ...) live in
// workspaces.go: the wire shape is identical between the personal
// (account:write) and workspace (workspaces:write) folder routes.
type AccountFoldersService interface {
	// Create: POST /v1/users/me/folders — scope
	// account:write.
	Create(ctx context.Context, body ResourceFolderCreateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error)
	// Update: PATCH .../folders/:folder_id — scope account:write.
	Update(ctx context.Context, folderID string, body ResourceFolderUpdateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error)
	// Delete: DELETE .../folders/:folder_id — scope account:write.
	Delete(ctx context.Context, folderID string, opts ...rest.RequestOpt) error
	// AddResource: PUT
	// .../folders/:folder_id/resources/:resource_type/:resource_id — scope
	// account:write. body may be nil (sent as {}).
	AddResource(ctx context.Context, folderID string, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
	// RemoveResource: DELETE
	// .../folders/:folder_id/resources/:resource_type/:resource_id — scope
	// account:write.
	RemoveResource(ctx context.Context, folderID string, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
}

type accountFoldersServiceImpl struct{ rest rest.Client }

func (f *accountFoldersServiceImpl) Create(ctx context.Context, body ResourceFolderCreateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error) {
	r, err := accountJSONBody(body)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	data, err := f.rest.Do(ctx, http.MethodPost, "/v1/users/me/folders", nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceFolder](data)
}

func (f *accountFoldersServiceImpl) Update(ctx context.Context, folderID string, body ResourceFolderUpdateBody, opts ...rest.RequestOpt) (WorkspaceResourceFolder, error) {
	r, err := accountJSONBody(body)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	data, err := f.rest.Do(ctx, http.MethodPatch, accountFolderPath(folderID, ""), nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceFolder{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceFolder](data)
}

func (f *accountFoldersServiceImpl) Delete(ctx context.Context, folderID string, opts ...rest.RequestOpt) error {
	_, err := f.rest.Do(ctx, http.MethodDelete, accountFolderPath(folderID, ""), nil, nil, "", opts...)
	return err
}

func (f *accountFoldersServiceImpl) AddResource(ctx context.Context, folderID string, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	var b ResourcePositionBody
	if body != nil {
		b = *body
	}
	r, err := accountJSONBody(b)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	path := accountFolderPath(folderID, "/resources/"+resourceTypeIDSuffix(resourceType, resourceID))
	data, err := f.rest.Do(ctx, http.MethodPut, path, nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}

func (f *accountFoldersServiceImpl) RemoveResource(ctx context.Context, folderID string, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	path := accountFolderPath(folderID, "/resources/"+resourceTypeIDSuffix(resourceType, resourceID))
	data, err := f.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}

// AccountFavoritesService groups the
// /v1/users/me/favorites* routes.
type AccountFavoritesService interface {
	// Add: PUT
	// /v1/users/me/favorites/:resource_type/:resource_id
	// — scope account:write. body may be nil (sent as {}).
	Add(ctx context.Context, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
	// Remove: DELETE
	// /v1/users/me/favorites/:resource_type/:resource_id
	// — scope account:write.
	Remove(ctx context.Context, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error)
}

type accountFavoritesServiceImpl struct{ rest rest.Client }

func (fav *accountFavoritesServiceImpl) Add(ctx context.Context, resourceType WorkspaceResourceType, resourceID string, body *ResourcePositionBody, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	var b ResourcePositionBody
	if body != nil {
		b = *body
	}
	r, err := accountJSONBody(b)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	path := "/v1/users/me/favorites/" + resourceTypeIDSuffix(resourceType, resourceID)
	data, err := fav.rest.Do(ctx, http.MethodPut, path, nil, r, "application/json", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}

func (fav *accountFavoritesServiceImpl) Remove(ctx context.Context, resourceType WorkspaceResourceType, resourceID string, opts ...rest.RequestOpt) (WorkspaceResourceOrganization, error) {
	path := "/v1/users/me/favorites/" + resourceTypeIDSuffix(resourceType, resourceID)
	data, err := fav.rest.Do(ctx, http.MethodDelete, path, nil, nil, "", opts...)
	if err != nil {
		return WorkspaceResourceOrganization{}, err
	}
	return rest.DecodeJSON[WorkspaceResourceOrganization](data)
}
