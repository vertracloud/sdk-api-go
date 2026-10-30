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
	"sort"
	"strconv"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// TemplatesService groups the template routes: the catalog, using a template
// (deploy), your private templates, share links and updating what was created
// from a template.
//
// Using a template creates, as one operation, everything it describes (an
// application, or a stack with applications, databases and a Blob folder).
// Deploy returns a job (TemplateDeploy) right away: follow it with
// Deploys().Get or Deploys().Events.
//
// To act on a workspace, pass rest.WithWorkspaceID (sent as the workspace_id
// query parameter); the create calls also take WorkspaceID in their body.
// Using a template in a workspace and creating or changing templates of a
// workspace are for the workspace owner (WORKSPACE_OWNER_ONLY otherwise).
type TemplatesService interface {
	// List: GET /v1/templates — scope templates:read. The public catalog also
	// works without a key (see rest.NewPublicClient). params may be nil.
	List(ctx context.Context, params *TemplateListParams, opts ...rest.RequestOpt) (TemplatePage, error)
	// Get: GET /v1/templates/:template_id — scope templates:read. idOrSlug is
	// the id or the slug.
	Get(ctx context.Context, idOrSlug string, opts ...rest.RequestOpt) (TemplateDetail, error)
	// Categories: GET /v1/templates/categories — scope templates:read. The
	// public catalog, with the number of templates per category.
	Categories(ctx context.Context, opts ...rest.RequestOpt) ([]TemplateCategoryCount, error)
	// Versions: GET /v1/templates/:template_id/versions — scope
	// templates:read. Newest first.
	Versions(ctx context.Context, idOrSlug string, opts ...rest.RequestOpt) ([]TemplateVersion, error)
	// Validate: POST /v1/templates/validate — scope templates:read. Checks a
	// manifest and creates nothing. manifest is a TemplateManifest, a map,
	// json.RawMessage or any value that marshals to JSON. The answer is 200
	// with Valid false and the findings, not an error.
	Validate(ctx context.Context, manifest any, opts ...rest.RequestOpt) (TemplateValidation, error)

	// Deploy: POST /v1/templates/:template_id/deploy — scope templates:deploy.
	// The API replies 202 with the job (status "queued"), or 200 with the same
	// job when body.IdempotencyKey was already used with the same request.
	Deploy(ctx context.Context, idOrSlug string, body TemplateDeployBody, opts ...rest.RequestOpt) (TemplateDeploy, error)
	// Deploys gives access to the deploy jobs.
	Deploys() TemplateDeploysService

	// Create: POST /v1/templates — scope templates:write. multipart/form-data:
	// creates a private template with its first version.
	Create(ctx context.Context, params TemplateCreateParams, opts ...rest.RequestOpt) (TemplateDetail, error)
	// FromApp: POST /v1/templates/from-app/:app_id — scope templates:write.
	// Saves an application as a private template.
	FromApp(ctx context.Context, appID string, body TemplateFromAppBody, opts ...rest.RequestOpt) (TemplateDetail, error)
	// FromStack: POST /v1/templates/from-stack/:stack_id — scope
	// templates:write. Saves a stack as a private template.
	FromStack(ctx context.Context, stackID string, body TemplateFromStackBody, opts ...rest.RequestOpt) (TemplateDetail, error)
	// AppDraft: GET /v1/templates/from-app/:app_id/draft — scope
	// templates:write. What the platform proposes when the application becomes
	// a template; values are never included.
	AppDraft(ctx context.Context, appID string, opts ...rest.RequestOpt) (TemplateDraft, error)
	// StackDraft: GET /v1/templates/from-stack/:stack_id/draft — scope
	// templates:write.
	StackDraft(ctx context.Context, stackID string, opts ...rest.RequestOpt) (TemplateDraft, error)
	// Update: PATCH /v1/templates/:template_id — scope templates:write.
	// Changes the details of a private template; at least one field.
	Update(ctx context.Context, id string, body TemplateUpdateDetailsBody, opts ...rest.RequestOpt) (TemplateDetail, error)
	// Delete: DELETE /v1/templates/:template_id — scope templates:write.
	// Removes a private template and revokes its links; what was already
	// created from it keeps running.
	Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error
	// NewVersion: POST /v1/templates/:template_id/versions — scope
	// templates:write. multipart/form-data; the version number of the
	// manifest must be higher than the current one.
	NewVersion(ctx context.Context, id string, params TemplateVersionCreateParams, opts ...rest.RequestOpt) (TemplateVersion, error)
	// LinkGitHub: PUT /v1/templates/:template_id/github — scope
	// templates:write. Links the template to a repository that has the
	// manifest at its root: each push to the branch creates a version.
	LinkGitHub(ctx context.Context, id string, body TemplateGitHubLinkBody, opts ...rest.RequestOpt) (TemplateDetail, error)
	// UnlinkGitHub: DELETE /v1/templates/:template_id/github — scope
	// templates:write.
	UnlinkGitHub(ctx context.Context, id string, opts ...rest.RequestOpt) (TemplateDetail, error)

	// ShareLinks gives access to the share links of a private template.
	ShareLinks() TemplateShareLinksService
	// Share gives access to what a share link offers to whoever holds it.
	Share() TemplateShareService

	// Updates gives access to updating, and leaving, the template an
	// application or a stack was created from.
	Updates() TemplateUpdatesService
	// ReportApp: POST /v1/apps/:id/template/report — scope templates:write.
	// Reports the template an application came from to the platform.
	ReportApp(ctx context.Context, appID string, body TemplateReportBody, opts ...rest.RequestOpt) error
}

// TemplateDeploysService groups the deploy job routes.
type TemplateDeploysService interface {
	// Get: GET /v1/template-deploys/:deploy_id — scope templates:deploy.
	// Outputs marked Once come back on the first read of a finished job and
	// are nil afterwards.
	Get(ctx context.Context, deployID string, opts ...rest.RequestOpt) (TemplateDeploy, error)
	// Events: GET /v1/template-deploys/:deploy_id/events — scope
	// templates:deploy. Server-Sent Events: the current state first, then one
	// event every time the job changes step. Outputs are never sent here: read
	// the job at the end. There is no automatic reconnection.
	Events(ctx context.Context, deployID string, opts ...rest.RequestOpt) (*TemplateDeployEventStream, error)
}

// TemplateShareLinksService groups the share link routes of a template.
type TemplateShareLinksService interface {
	// Create: POST /v1/templates/:template_id/share-links — scope
	// templates:write. The token is in the answer and is shown only once.
	Create(ctx context.Context, templateID string, body TemplateShareLinkCreateBody, opts ...rest.RequestOpt) (TemplateShareLinkCreated, error)
	// List: GET /v1/templates/:template_id/share-links — scope templates:read.
	List(ctx context.Context, templateID string, opts ...rest.RequestOpt) ([]TemplateShareLink, error)
	// Revoke: DELETE /v1/templates/:template_id/share-links/:link_id — scope
	// templates:write. No new use, and no more updates offered; what was
	// created from it is not affected.
	Revoke(ctx context.Context, templateID, linkID string, opts ...rest.RequestOpt) (TemplateShareLink, error)
}

// TemplateShareService groups the /v1/share/templates/:token routes.
type TemplateShareService interface {
	// Preview: GET /v1/share/templates/:token — scope templates:read. What the
	// link gives, before using it. Works without authentication.
	Preview(ctx context.Context, token string, opts ...rest.RequestOpt) (TemplateSharePreview, error)
	// Deploy: POST /v1/share/templates/:token/deploy — scope templates:deploy.
	// For a link in "use" mode; a "full_copy" link is refused here.
	Deploy(ctx context.Context, token string, body TemplateShareDeployBody, opts ...rest.RequestOpt) (TemplateDeploy, error)
	// Copy: POST /v1/share/templates/:token/copy — scope templates:write. For
	// a link in "full_copy" mode: gives you a private template of your own,
	// which counts against your limit of private templates.
	Copy(ctx context.Context, token string, body TemplateShareCopyBody, opts ...rest.RequestOpt) (TemplateDetail, error)
	// Report: POST /v1/share/templates/:token/report — scope templates:write.
	Report(ctx context.Context, token string, body TemplateReportBody, opts ...rest.RequestOpt) error
}

// TemplateUpdatesService groups the update and detach routes of what was
// created from a template. An update is never automatic: it runs only when
// you ask. It takes a safety snapshot, updates the services one after the
// other and, if one fails, goes back to the snapshot.
type TemplateUpdatesService interface {
	// AppInfo: GET /v1/apps/:id/template/update — scope templates:read. What
	// updating to the latest version would do.
	AppInfo(ctx context.Context, appID string, opts ...rest.RequestOpt) (TemplateUpdateInfo, error)
	// AppUpdate: POST /v1/apps/:id/template/update — scope templates:deploy.
	// The API replies 202 with a job (Kind "update"). A blocked update fails
	// with TEMPLATE_UPDATE_BLOCKED (see TemplateUpdateBlockedReason).
	AppUpdate(ctx context.Context, appID string, body TemplateUpdateBody, opts ...rest.RequestOpt) (TemplateDeploy, error)
	// AppDetach: POST /v1/apps/:id/template/detach — scope apps:write. The
	// application stops receiving versions and becomes an ordinary one.
	// Refused for an application of a closed template (TEMPLATE_CLOSED).
	AppDetach(ctx context.Context, appID string, opts ...rest.RequestOpt) (Application, error)
	// StackInfo: GET /v1/stacks/:stack_id/template/update — scope templates:read.
	StackInfo(ctx context.Context, stackID string, opts ...rest.RequestOpt) (TemplateUpdateInfo, error)
	// StackUpdate: POST /v1/stacks/:stack_id/template/update — scope templates:deploy.
	StackUpdate(ctx context.Context, stackID string, body TemplateUpdateBody, opts ...rest.RequestOpt) (TemplateDeploy, error)
	// StackDetach: POST /v1/stacks/:stack_id/template/detach — scope
	// stacks:write. The stack becomes one assembled by hand.
	StackDetach(ctx context.Context, stackID string, opts ...rest.RequestOpt) (Stack, error)
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// TemplateOrigin says who publishes a template: "vertra" and "community" are
// public catalog entries; "private" belongs to a user or a workspace and is
// shared by link.
type TemplateOrigin string

const (
	TemplateOriginVertra    TemplateOrigin = "vertra"
	TemplateOriginCommunity TemplateOrigin = "community"
	TemplateOriginPrivate   TemplateOrigin = "private"
)

// TemplateVisibility is the visibility of a template in the catalog.
type TemplateVisibility string

const (
	TemplateVisibilityPublic     TemplateVisibility = "public"
	TemplateVisibilityBeta       TemplateVisibility = "beta"
	TemplateVisibilityHidden     TemplateVisibility = "hidden"
	TemplateVisibilityDeprecated TemplateVisibility = "deprecated"
)

// TemplateCategory is the category of a template.
type TemplateCategory string

const (
	TemplateCategoryAutomation TemplateCategory = "automation"
	TemplateCategoryAIAgents   TemplateCategory = "ai-agents"
	TemplateCategoryBots       TemplateCategory = "bots"
	TemplateCategoryAPIs       TemplateCategory = "apis"
	TemplateCategoryCMS        TemplateCategory = "cms"
	TemplateCategoryMonitoring TemplateCategory = "monitoring"
	TemplateCategoryStarters   TemplateCategory = "starters"
	TemplateCategoryDatabases  TemplateCategory = "databases"
	TemplateCategoryOther      TemplateCategory = "other"
)

// TemplateSort is the order of TemplatesService.List. Default: recommended.
type TemplateSort string

const (
	TemplateSortRecommended TemplateSort = "recommended"
	TemplateSortPopular     TemplateSort = "popular"
	TemplateSortNewest      TemplateSort = "newest"
	TemplateSortName        TemplateSort = "name"
)

// TemplateSourceType is where the code of a service comes from.
type TemplateSourceType string

const (
	TemplateSourceTypeCode  TemplateSourceType = "code"
	TemplateSourceTypeImage TemplateSourceType = "image"
)

// TemplateWebMode says whether a service of a template is published on the web.
type TemplateWebMode string

const (
	TemplateWebModeRequired TemplateWebMode = "required"
	TemplateWebModeOptional TemplateWebMode = "optional"
	TemplateWebModeNone     TemplateWebMode = "none"
)

// TemplateInputType is the type of a question of a template.
type TemplateInputType string

const (
	TemplateInputTypeText    TemplateInputType = "text"
	TemplateInputTypeSecret  TemplateInputType = "secret"
	TemplateInputTypeSelect  TemplateInputType = "select"
	TemplateInputTypeBoolean TemplateInputType = "boolean"
	TemplateInputTypeNumber  TemplateInputType = "number"
	TemplateInputTypeURL     TemplateInputType = "url"
	TemplateInputTypeEmail   TemplateInputType = "email"
)

// TemplateDatabaseEngine is the engine of a database service of a template.
type TemplateDatabaseEngine string

const (
	TemplateDatabaseEnginePostgres TemplateDatabaseEngine = "postgres"
	TemplateDatabaseEngineMySQL    TemplateDatabaseEngine = "mysql"
	TemplateDatabaseEngineMongoDB  TemplateDatabaseEngine = "mongodb"
	TemplateDatabaseEngineRedis    TemplateDatabaseEngine = "redis"
)

// TemplateBlobVisibility is the visibility of a Blob folder created by a
// template or added to a stack.
type TemplateBlobVisibility string

const (
	TemplateBlobVisibilityPrivate TemplateBlobVisibility = "private"
	TemplateBlobVisibilityPublic  TemplateBlobVisibility = "public"
)

// TemplateShareLinkMode is what a share link gives: "use" creates resources
// from the template; "full_copy" gives a private template of your own.
type TemplateShareLinkMode string

const (
	TemplateShareLinkModeUse      TemplateShareLinkMode = "use"
	TemplateShareLinkModeFullCopy TemplateShareLinkMode = "full_copy"
)

// TemplateDeployStatus is the state of a deploy job.
type TemplateDeployStatus string

const (
	TemplateDeployStatusQueued    TemplateDeployStatus = "queued"
	TemplateDeployStatusRunning   TemplateDeployStatus = "running"
	TemplateDeployStatusUndoing   TemplateDeployStatus = "undoing"
	TemplateDeployStatusSucceeded TemplateDeployStatus = "succeeded"
	TemplateDeployStatusFailed    TemplateDeployStatus = "failed"
)

// Finished reports whether the job reached a final state.
func (s TemplateDeployStatus) Finished() bool {
	return s == TemplateDeployStatusSucceeded || s == TemplateDeployStatusFailed
}

// TemplateDeployKind is what a job does: create (deploy) or update.
type TemplateDeployKind string

const (
	TemplateDeployKindDeploy TemplateDeployKind = "deploy"
	TemplateDeployKindUpdate TemplateDeployKind = "update"
)

// TemplateDeployStep is a step of a deploy job.
type TemplateDeployStep string

const (
	TemplateDeployStepValidate     TemplateDeployStep = "validate"
	TemplateDeployStepSnapshot     TemplateDeployStep = "snapshot"
	TemplateDeployStepNetwork      TemplateDeployStep = "network"
	TemplateDeployStepDatabases    TemplateDeployStep = "databases"
	TemplateDeployStepBlob         TemplateDeployStep = "blob"
	TemplateDeployStepImages       TemplateDeployStep = "images"
	TemplateDeployStepApps         TemplateDeployStep = "apps"
	TemplateDeployStepVariables    TemplateDeployStep = "variables"
	TemplateDeployStepCertificates TemplateDeployStep = "certificates"
	TemplateDeployStepStart        TemplateDeployStep = "start"
	TemplateDeployStepHealthcheck  TemplateDeployStep = "healthcheck"
	TemplateDeployStepOutputs      TemplateDeployStep = "outputs"
	TemplateDeployStepRollback     TemplateDeployStep = "rollback"
)

// TemplateDeployStepStatus is the state of one step of a job.
type TemplateDeployStepStatus string

const (
	TemplateDeployStepStatusPending TemplateDeployStepStatus = "pending"
	TemplateDeployStepStatusRunning TemplateDeployStepStatus = "running"
	TemplateDeployStepStatusDone    TemplateDeployStepStatus = "done"
	TemplateDeployStepStatusSkipped TemplateDeployStepStatus = "skipped"
	TemplateDeployStepStatusFailed  TemplateDeployStepStatus = "failed"
)

// TemplateReportReason is why a template is reported.
type TemplateReportReason string

const (
	TemplateReportReasonMalware      TemplateReportReason = "malware"
	TemplateReportReasonScam         TemplateReportReason = "scam"
	TemplateReportReasonCopyright    TemplateReportReason = "copyright"
	TemplateReportReasonLeakedSecret TemplateReportReason = "leaked_secret"
	TemplateReportReasonOther        TemplateReportReason = "other"
)

// TemplateFitReason is why a template does not fit an account right now.
type TemplateFitReason string

const (
	TemplateFitReasonNoPlan         TemplateFitReason = "no_plan"
	TemplateFitReasonRAM            TemplateFitReason = "ram"
	TemplateFitReasonWebPublish     TemplateFitReason = "web_publish"
	TemplateFitReasonImagePlan      TemplateFitReason = "image_plan"
	TemplateFitReasonImageSlots     TemplateFitReason = "image_slots"
	TemplateFitReasonPrivateNetwork TemplateFitReason = "private_network"
	TemplateFitReasonBlobPlan       TemplateFitReason = "blob_plan"
	TemplateFitReasonMinPlan        TemplateFitReason = "min_plan"
	TemplateFitReasonMaxApps        TemplateFitReason = "max_apps"
	TemplateFitReasonMaxDatabases   TemplateFitReason = "max_databases"
	TemplateFitReasonDeploysPerHour TemplateFitReason = "deploys_per_hour"
)

// TemplateUpdateBlockReason is why an update is not possible.
type TemplateUpdateBlockReason string

const (
	TemplateUpdateBlockReasonNoNewVersion     TemplateUpdateBlockReason = "no_new_version"
	TemplateUpdateBlockReasonDetached         TemplateUpdateBlockReason = "detached"
	TemplateUpdateBlockReasonTemplateRemoved  TemplateUpdateBlockReason = "template_removed"
	TemplateUpdateBlockReasonLinkRevoked      TemplateUpdateBlockReason = "link_revoked"
	TemplateUpdateBlockReasonRAM              TemplateUpdateBlockReason = "ram"
	TemplateUpdateBlockReasonPlan             TemplateUpdateBlockReason = "plan"
	TemplateUpdateBlockReasonImageSlots       TemplateUpdateBlockReason = "image_slots"
	TemplateUpdateBlockReasonAppBuilding      TemplateUpdateBlockReason = "app_building"
	TemplateUpdateBlockReasonDeployInProgress TemplateUpdateBlockReason = "deploy_in_progress"
)

// TemplateValidationCode identifies one finding of a manifest check.
type TemplateValidationCode string

const (
	TemplateValidationCodeInvalidType        TemplateValidationCode = "invalid_type"
	TemplateValidationCodeRequired           TemplateValidationCode = "required"
	TemplateValidationCodeUnknownField       TemplateValidationCode = "unknown_field"
	TemplateValidationCodeInvalidSlug        TemplateValidationCode = "invalid_slug"
	TemplateValidationCodeInvalidVersion     TemplateValidationCode = "invalid_version"
	TemplateValidationCodeInvalidCategory    TemplateValidationCode = "invalid_category"
	TemplateValidationCodeInvalidText        TemplateValidationCode = "invalid_text"
	TemplateValidationCodeInvalidRAM         TemplateValidationCode = "invalid_ram"
	TemplateValidationCodeInvalidPlan        TemplateValidationCode = "invalid_plan"
	TemplateValidationCodeUnknownPlaceholder TemplateValidationCode = "unknown_placeholder"
	TemplateValidationCodeUnknownService     TemplateValidationCode = "unknown_service"
	TemplateValidationCodeUnknownInput       TemplateValidationCode = "unknown_input"
	TemplateValidationCodeInvalidWhen        TemplateValidationCode = "invalid_when"
	TemplateValidationCodeInvalidOption      TemplateValidationCode = "invalid_option"
	TemplateValidationCodeInvalidSource      TemplateValidationCode = "invalid_source"
	TemplateValidationCodeInvalidPermissions TemplateValidationCode = "invalid_permissions"
	TemplateValidationCodeForbiddenEnv       TemplateValidationCode = "forbidden_env"
	TemplateValidationCodeForbiddenInPublic  TemplateValidationCode = "forbidden_in_public"
	TemplateValidationCodeSecretInManifest   TemplateValidationCode = "secret_in_manifest"
	TemplateValidationCodeInvalidDeploy      TemplateValidationCode = "invalid_deploy"
	TemplateValidationCodeInvalidConnection  TemplateValidationCode = "invalid_connection"
	TemplateValidationCodeDuplicateID        TemplateValidationCode = "duplicate_id"
	TemplateValidationCodeTooManyServices    TemplateValidationCode = "too_many_services"
)

// TemplateEnvReviewMode is what happens to a variable of an app when it
// becomes a template.
type TemplateEnvReviewMode string

const (
	// TemplateEnvReviewModeQuestion becomes a question the user answers; the value is not copied.
	TemplateEnvReviewModeQuestion TemplateEnvReviewMode = "question"
	// TemplateEnvReviewModeKeep copies the value into the template: only for variables that are not secrets.
	TemplateEnvReviewModeKeep TemplateEnvReviewMode = "keep"
	// TemplateEnvReviewModeGenerate becomes a secret generated for each user.
	TemplateEnvReviewModeGenerate TemplateEnvReviewMode = "generate"
	// TemplateEnvReviewModeHidden copies the value but hides it from whoever uses the template (a closed template).
	TemplateEnvReviewModeHidden TemplateEnvReviewMode = "hidden"
	// TemplateEnvReviewModeOmit leaves the variable out of the template.
	TemplateEnvReviewModeOmit TemplateEnvReviewMode = "omit"
)

// ---------------------------------------------------------------------------
// Shared payloads
// ---------------------------------------------------------------------------

// TemplateI18nText is a text in the three supported languages. PtBR is
// required; a missing EN or ES falls back to PtBR.
type TemplateI18nText struct {
	PtBR string `json:"pt-br"`
	EN   string `json:"en,omitempty"`
	ES   string `json:"es,omitempty"`
}

// TemplateInputValue is the answer to a question of a template: a string, a
// number (float64 once decoded) or a bool.
type TemplateInputValue = any

// TemplateEffectivePermissions are the permissions of a template after
// defaults and derived restrictions, as they apply to an app created from it.
type TemplateEffectivePermissions struct {
	CodeView       bool `json:"code_view"`
	CodeEdit       bool `json:"code_edit"`
	RAMChange      bool `json:"ram_change"`
	RAMBelowMin    bool `json:"ram_below_min"`
	RAMAboveMax    bool `json:"ram_above_max"`
	EnvAdd         bool `json:"env_add"`
	CommandsEdit   bool `json:"commands_edit"`
	Terminal       bool `json:"terminal"`
	SaveAsTemplate bool `json:"save_as_template"`
	// Detach is true when editing the code detaches the app from the template (open templates only).
	Detach bool `json:"detach"`
}

// TemplateOriginInfo is where an app or a stack came from, as shown on the
// resource. See ApplicationTemplate and Stack.Template.
type TemplateOriginInfo struct {
	TemplateID string           `json:"template_id"`
	Slug       string           `json:"slug"`
	Name       TemplateI18nText `json:"name"`
	// IconURL is the stable icon URL of the template; nil for a private template without icon or a removed one.
	IconURL   *string        `json:"icon_url"`
	Origin    TemplateOrigin `json:"origin"`
	VersionID string         `json:"version_id"`
	// Version is the version the resource was created from or last updated to.
	Version string `json:"version"`
	// Closed is true for an app of a closed template: its code is not accessible.
	Closed bool `json:"closed"`
	// Detached is true when the resource left the template and no longer receives versions.
	Detached bool `json:"detached"`
	// UpdateAvailable is true when the template has a newer version this resource can update to.
	UpdateAvailable bool `json:"update_available"`
	// LatestVersion is the latest version of the template; nil when there is no newer one.
	LatestVersion *string `json:"latest_version"`
	// Removed is true when the author removed the template: the resource keeps running but gets no more updates.
	Removed bool `json:"removed"`
	// ViaShareLink is true when the resource was created from a share link.
	ViaShareLink bool `json:"via_share_link"`
}

// ApplicationTemplate is the template an application was created from.
type ApplicationTemplate struct {
	TemplateOriginInfo
	// ServiceID is the id of the service of the template this application is; nil when the template has a single service.
	ServiceID *string `json:"service_id"`
	// Permissions is what the template lets you do with this application.
	Permissions TemplateEffectivePermissions `json:"permissions"`
	// LockedDeploySettings are the deploy settings the template locks for this application ("zero_downtime",
	// "auto_rollback"): UpdateConfig refuses another value with TEMPLATE_SETTING_LOCKED. Empty once detached.
	LockedDeploySettings []string `json:"locked_deploy_settings,omitempty"`
}

// ---------------------------------------------------------------------------
// Manifest
// ---------------------------------------------------------------------------

// Limits of the name and description of a template.
const (
	TemplateNameMaxLength        = 50
	TemplateDescriptionMaxLength = 128
)

// TemplateLinks are the links of a template.
type TemplateLinks struct {
	Website string `json:"website,omitempty"`
	Docs    string `json:"docs,omitempty"`
	Source  string `json:"source,omitempty"`
	License string `json:"license,omitempty"`
}

// TemplateRAM is the memory of a service, in MB.
type TemplateRAM struct {
	Min         int `json:"min"`
	Recommended int `json:"recommended"`
	Max         int `json:"max,omitempty"`
}

// TemplateWhen makes a service or option depend on the answer to a question.
type TemplateWhen struct {
	Input string `json:"input"`
	// Equals is a string, a number or a bool.
	Equals any `json:"equals"`
}

// TemplateInputValidation is the validation of a question.
type TemplateInputValidation struct {
	Regex     string   `json:"regex,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
}

// TemplateOptionSets is what choosing an option of a select question changes.
type TemplateOptionSets struct {
	// Services turns services "on" or "off", by service id.
	Services map[string]string `json:"services,omitempty"`
	// Env sets variables, by service id and variable name.
	Env map[string]map[string]string `json:"env,omitempty"`
	// RAMExtra adds memory in MB, by service id.
	RAMExtra map[string]int `json:"ram_extra,omitempty"`
}

// TemplateInputOption is one option of a select question.
type TemplateInputOption struct {
	Value string              `json:"value"`
	Label *TemplateI18nText   `json:"label,omitempty"`
	Sets  *TemplateOptionSets `json:"sets,omitempty"`
}

// TemplateInput is a question the user answers when using a template.
type TemplateInput struct {
	ID         string                   `json:"id"`
	Type       TemplateInputType        `json:"type"`
	Label      TemplateI18nText         `json:"label"`
	Help       *TemplateI18nText        `json:"help,omitempty"`
	Required   bool                     `json:"required,omitempty"`
	Default    TemplateInputValue       `json:"default,omitempty"`
	Validation *TemplateInputValidation `json:"validation,omitempty"`
	// Generate says how a secret is generated, "random:<n>" or "uuid".
	Generate string                `json:"generate,omitempty"`
	Editable *bool                 `json:"editable,omitempty"`
	When     *TemplateWhen         `json:"when,omitempty"`
	Options  []TemplateInputOption `json:"options,omitempty"`
}

// TemplatePermissions are the permissions the author gives whoever uses the template.
type TemplatePermissions struct {
	Code     *TemplatePermissionCode     `json:"code,omitempty"`
	RAM      *TemplatePermissionRAM      `json:"ram,omitempty"`
	Env      *TemplatePermissionEnv      `json:"env,omitempty"`
	Commands *TemplatePermissionCommands `json:"commands,omitempty"`
	Version  *TemplatePermissionVersion  `json:"version,omitempty"`
	Share    *TemplatePermissionShare    `json:"share,omitempty"`
}

type TemplatePermissionCode struct {
	View *bool `json:"view,omitempty"`
	Edit *bool `json:"edit,omitempty"`
}

type TemplatePermissionRAM struct {
	Change   *bool `json:"change,omitempty"`
	BelowMin *bool `json:"below_min,omitempty"`
	AboveMax *bool `json:"above_max,omitempty"`
}

type TemplatePermissionEnv struct {
	Add *bool `json:"add,omitempty"`
}

type TemplatePermissionCommands struct {
	Edit *bool `json:"edit,omitempty"`
}

type TemplatePermissionVersion struct {
	Selectable *bool `json:"selectable,omitempty"`
}

type TemplatePermissionShare struct {
	FullCopy *bool `json:"full_copy,omitempty"`
}

// TemplateDeploySettings is the deploy behavior of an application service.
type TemplateDeploySettings struct {
	ZeroDowntime *TemplateZeroDowntimeSetting `json:"zero_downtime,omitempty"`
	AutoRollback *TemplateAutoRollbackSetting `json:"auto_rollback,omitempty"`
	Rollback     *TemplateRollbackSetting     `json:"rollback,omitempty"`
}

type TemplateZeroDowntimeSetting struct {
	Default ApplicationZeroDowntime `json:"default"`
	Locked  bool                    `json:"locked,omitempty"`
}

type TemplateAutoRollbackSetting struct {
	Default bool `json:"default"`
	Locked  bool `json:"locked,omitempty"`
}

type TemplateRollbackSetting struct {
	// WithData is "required" or "optional".
	WithData string `json:"with_data"`
}

// TemplateSource is where the code of an application service comes from.
type TemplateSource struct {
	Type TemplateSourceType `json:"type"`
	// Image is an image of your account, by id (only with Type "image").
	Image   string `json:"image,omitempty"`
	Path    string `json:"path,omitempty"`
	Context string `json:"context,omitempty"`
}

// TemplateHealthcheck is the health check of an application service.
type TemplateHealthcheck struct {
	Path    string `json:"path,omitempty"`
	Timeout int    `json:"timeout"`
}

// TemplateConnection is a requirement on a connection of a service.
type TemplateConnection struct {
	Private string `json:"private,omitempty"`
}

// TemplateBlobSettings are the settings of a Blob service.
type TemplateBlobSettings struct {
	Visibility TemplateBlobVisibility `json:"visibility,omitempty"`
}

// TemplateManifestService is one service of a manifest. It is an application
// (Source set), a database (Database set) or a Blob folder (Blob set); the
// API checks that exactly one kind is used.
type TemplateManifestService struct {
	// Application.
	Source         *TemplateSource               `json:"source,omitempty"`
	Language       ApplicationLanguage           `json:"language,omitempty"`
	RuntimeVersion string                        `json:"runtime_version,omitempty"`
	Main           string                        `json:"main,omitempty"`
	Build          string                        `json:"build,omitempty"`
	Start          string                        `json:"start,omitempty"`
	Port           int                           `json:"port,omitempty"`
	Web            TemplateWebMode               `json:"web,omitempty"`
	Env            map[string]string             `json:"env,omitempty"`
	EnvHidden      []string                      `json:"env_hidden,omitempty"`
	Healthcheck    *TemplateHealthcheck          `json:"healthcheck,omitempty"`
	Deploy         *TemplateDeploySettings       `json:"deploy,omitempty"`
	Connections    map[string]TemplateConnection `json:"connections,omitempty"`
	// Database.
	Database TemplateDatabaseEngine `json:"database,omitempty"`
	Version  string                 `json:"version,omitempty"`
	// Blob.
	Blob *TemplateBlobSettings `json:"blob,omitempty"`
	// Shared by every kind.
	RAM  *TemplateRAM  `json:"ram,omitempty"`
	When *TemplateWhen `json:"when,omitempty"`
}

// TemplateOutput is something shown to the user after the deploy.
type TemplateOutput struct {
	Label TemplateI18nText `json:"label"`
	// Value is a string or a TemplateI18nText, as raw JSON.
	Value  json.RawMessage `json:"value"`
	Secret bool            `json:"secret,omitempty"`
	Once   bool            `json:"once,omitempty"`
}

// TemplateManifest is the description of a template: what it creates, what
// it asks and what it allows. Its fields are checked by
// TemplatesService.Validate.
type TemplateManifest struct {
	ID          string                             `json:"id"`
	Version     string                             `json:"version"`
	Name        TemplateI18nText                   `json:"name"`
	Description TemplateI18nText                   `json:"description"`
	Category    TemplateCategory                   `json:"category"`
	Tags        []string                           `json:"tags,omitempty"`
	Icon        string                             `json:"icon,omitempty"`
	Links       *TemplateLinks                     `json:"links,omitempty"`
	Landing     string                             `json:"landing,omitempty"`
	MinPlan     string                             `json:"min_plan,omitempty"`
	Inputs      []TemplateInput                    `json:"inputs,omitempty"`
	Services    map[string]TemplateManifestService `json:"services"`
	Permissions *TemplatePermissions               `json:"permissions,omitempty"`
	Outputs     []TemplateOutput                   `json:"outputs,omitempty"`
	Secrets     []string                           `json:"secrets,omitempty"`
	BYOK        bool                               `json:"byok,omitempty"`
}

// ---------------------------------------------------------------------------
// Catalog payloads
// ---------------------------------------------------------------------------

// TemplateServiceSummary is a service of a template, as listed in the catalog.
type TemplateServiceSummary struct {
	ID string `json:"id"`
	// Kind is "app", "database" or "blob".
	Kind           string                  `json:"kind"`
	SourceType     *TemplateSourceType     `json:"source_type"`
	Database       *TemplateDatabaseEngine `json:"database"`
	BlobVisibility *TemplateBlobVisibility `json:"blob_visibility"`
	RAM            TemplateRAM             `json:"ram"`
	Web            *TemplateWebMode        `json:"web"`
	// Conditional is true when the service depends on an answer.
	Conditional bool `json:"conditional"`
}

// TemplateOwner is who owns a private template.
type TemplateOwner struct {
	UserID string `json:"user_id"`
	// WorkspaceID is nil when the template belongs to the user.
	WorkspaceID *string `json:"workspace_id"`
}

// TemplateVersionSummary is a version of a template without its content.
type TemplateVersionSummary struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	// Security is true when the version fixes a security problem.
	Security  bool      `json:"security"`
	CreatedAt time.Time `json:"created_at"`
}

// TemplateFitIssue is one reason a template does not fit the account. Required
// and Available are a number or a string, as raw JSON.
type TemplateFitIssue struct {
	Reason    TemplateFitReason `json:"reason"`
	Required  json.RawMessage   `json:"required,omitempty"`
	Available json.RawMessage   `json:"available,omitempty"`
}

// TemplateAccountContext is how a template fits the caller's account. Present
// only when the call is authenticated.
type TemplateAccountContext struct {
	Fits    bool               `json:"fits"`
	Reasons []TemplateFitIssue `json:"reasons"`
	// RequiredRAM is the memory the template needs, in MB.
	RequiredRAM    int  `json:"required_ram"`
	FreeRAM        int  `json:"free_ram"`
	NeedsImageSlot bool `json:"needs_image_slot"`
	ImageSlotsFree int  `json:"image_slots_free"`
	InUse          bool `json:"in_use"`
	InUseCount     int  `json:"in_use_count"`
}

// TemplateGitHub is the repository and branch a template is linked to.
type TemplateGitHub struct {
	Repository string `json:"repository"`
	Branch     string `json:"branch"`
}

// Template is a template as listed in the catalog.
type Template struct {
	ID          string             `json:"id"`
	Slug        string             `json:"slug"`
	Origin      TemplateOrigin     `json:"origin"`
	Visibility  TemplateVisibility `json:"visibility"`
	Name        TemplateI18nText   `json:"name"`
	Description TemplateI18nText   `json:"description"`
	Category    TemplateCategory   `json:"category"`
	Tags        []string           `json:"tags"`
	IconURL     *string            `json:"icon_url"`
	// AuthorDisplay is the author as shown to you; nil for a template of the platform.
	AuthorDisplay  *string                  `json:"author_display"`
	Links          TemplateLinks            `json:"links"`
	Landing        *string                  `json:"landing"`
	Featured       bool                     `json:"featured"`
	CurrentVersion TemplateVersionSummary   `json:"current_version"`
	Services       []TemplateServiceSummary `json:"services"`
	// CreatesStack is true when using the template creates a stack (more than one service).
	CreatesStack bool `json:"creates_stack"`
	// RAMMinTotal is the minimum memory of all services together, in MB.
	RAMMinTotal int  `json:"ram_min_total"`
	BYOK        bool `json:"byok"`
	// RequiredPlan is the lowest plan that can use the template.
	RequiredPlan UserPlanName `json:"required_plan"`
	CodeView     bool         `json:"code_view"`
	// UsesCount is the number of uses; nil when it is not shown.
	UsesCount *int            `json:"uses_count"`
	Owner     *TemplateOwner  `json:"owner"`
	GitHub    *TemplateGitHub `json:"github"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	// Account is how the template fits your account; nil without authentication.
	Account *TemplateAccountContext `json:"account,omitempty"`
}

// TemplateDetail is a template with its questions, permissions and texts.
type TemplateDetail struct {
	Template
	Inputs      []TemplateInput              `json:"inputs"`
	Permissions TemplateEffectivePermissions `json:"permissions"`
	Outputs     []TemplateOutput             `json:"outputs"`
	Readme      *TemplateI18nText            `json:"readme"`
	Changelog   *TemplateI18nText            `json:"changelog"`
}

// TemplateVersion is a version of a template.
type TemplateVersion struct {
	TemplateVersionSummary
	Changelog *TemplateI18nText `json:"changelog"`
	// Manifest is nil when the manifest is not shown (a closed template).
	Manifest *TemplateManifest `json:"manifest"`
}

// TemplateCategoryCount is the number of public templates of a category.
type TemplateCategoryCount struct {
	Category TemplateCategory `json:"category"`
	Count    int              `json:"count"`
}

// TemplatePage is one page of TemplatesService.List.
type TemplatePage struct {
	Data            []Template `json:"data"`
	TotalCount      int        `json:"total_count"`
	Page            int        `json:"page"`
	Limit           int        `json:"limit"`
	TotalPages      int        `json:"total_pages"`
	HasNextPage     bool       `json:"has_next_page"`
	HasPreviousPage bool       `json:"has_previous_page"`
}

// TemplateListParams are the optional filters of TemplatesService.List.
type TemplateListParams struct {
	// Search is text to look for in the name, description and tags.
	Search   string
	Category TemplateCategory
	// Origin "private" lists your private templates (and the ones of
	// WorkspaceID); it needs authentication.
	Origin TemplateOrigin
	Tag    string
	// Fits true keeps only the templates that fit your account right now. Needs authentication.
	Fits *bool
	// Sort defaults to "recommended".
	Sort TemplateSort
	// Page is 1-based; 0 omits it. Limit 0 omits it.
	Page  int
	Limit int
	// WorkspaceID lists the templates of that workspace. Same as
	// rest.WithWorkspaceID; an explicit option passed to List takes precedence.
	WorkspaceID string
}

// TemplateValidationIssue is one finding of a manifest check.
type TemplateValidationIssue struct {
	// Path is the place in the manifest.
	Path    string                 `json:"path"`
	Code    TemplateValidationCode `json:"code"`
	Message string                 `json:"message"`
}

// TemplateValidation is the result of TemplatesService.Validate.
type TemplateValidation struct {
	Valid  bool                      `json:"valid"`
	Issues []TemplateValidationIssue `json:"issues"`
	// RequiredPlan is nil when the manifest is not valid.
	RequiredPlan *UserPlanName `json:"required_plan"`
	// RAMMinTotal is nil when the manifest is not valid.
	RAMMinTotal *int                     `json:"ram_min_total"`
	Services    []TemplateServiceSummary `json:"services"`
}

// ---------------------------------------------------------------------------
// Deploy payloads
// ---------------------------------------------------------------------------

// TemplateDeployServiceOptions is what the user chooses for one service.
type TemplateDeployServiceOptions struct {
	// Name of the application or database; default: the name of the deploy followed by the service id.
	Name string `json:"name,omitempty"`
	// RAM in MB, within what the template allows.
	RAM int `json:"ram,omitempty"`
	// Subdomain, for an application with web: "random" or the one you want.
	Subdomain string `json:"subdomain,omitempty"`
}

// TemplateDeployBody is the body of TemplatesService.Deploy.
type TemplateDeployBody struct {
	// Name of the application, or of the stack when the template has more than one service. Required.
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Version is a version number. Only valid when the template allows choosing versions; default: the latest.
	Version string `json:"version,omitempty"`
	// Inputs are the answers to the questions of the template, by question id.
	Inputs map[string]TemplateInputValue `json:"inputs,omitempty"`
	// Services are the choices per service, by service id.
	Services map[string]TemplateDeployServiceOptions `json:"services,omitempty"`
	// NetworkMode: only "public" is accepted.
	NetworkMode StackNetworkMode `json:"network_mode,omitempty"`
	// WorkspaceID is the workspace that will own what is created.
	WorkspaceID string `json:"workspace_id,omitempty"`
	// FolderID is the folder to put the application or stack in.
	FolderID string `json:"folder_id,omitempty"`
	// IdempotencyKey is sent as the Idempotency-Key header (not in the body).
	// It makes the call safe to repeat: the same key with the same request
	// returns the same job instead of creating a second set of resources; the
	// same key with a different request fails with IDEMPOTENCY_KEY_CONFLICT.
	// Recommended for every call.
	IdempotencyKey string `json:"-"`
}

// TemplateShareDeployBody is the body of TemplateShareService.Deploy: the
// same as TemplateDeployBody, without a choice of version.
type TemplateShareDeployBody struct {
	Name        string                                  `json:"name"`
	Description string                                  `json:"description,omitempty"`
	Inputs      map[string]TemplateInputValue           `json:"inputs,omitempty"`
	Services    map[string]TemplateDeployServiceOptions `json:"services,omitempty"`
	NetworkMode StackNetworkMode                        `json:"network_mode,omitempty"`
	WorkspaceID string                                  `json:"workspace_id,omitempty"`
	FolderID    string                                  `json:"folder_id,omitempty"`
	// IdempotencyKey is sent as the Idempotency-Key header; see TemplateDeployBody.
	IdempotencyKey string `json:"-"`
}

// TemplateDeployStepState is the state of one step of a job.
type TemplateDeployStepState struct {
	Step       TemplateDeployStep       `json:"step"`
	Status     TemplateDeployStepStatus `json:"status"`
	StartedAt  *time.Time               `json:"started_at"`
	FinishedAt *time.Time               `json:"finished_at"`
}

// TemplateDeployError is why a job failed.
type TemplateDeployError struct {
	// Code is an error sentinel such as TEMPLATE_RAM_INSUFFICIENT.
	Code    string `json:"code"`
	Message string `json:"message"`
	// Step is where it failed; nil when it is not tied to a step.
	Step    *TemplateDeployStep `json:"step"`
	Details map[string]any      `json:"details,omitempty"`
}

// TemplateDeployResource is something a job created or updated.
type TemplateDeployResource struct {
	ResourceType WorkspaceResourceType `json:"resource_type"`
	ResourceID   string                `json:"resource_id"`
	// ServiceID is the id of the service in the template; nil for the stack.
	ServiceID *string `json:"service_id"`
	Name      string  `json:"name"`
}

// TemplateDeployOutput is something shown to the user after the deploy.
type TemplateDeployOutput struct {
	Label TemplateI18nText `json:"label"`
	// Value is a string or a TemplateI18nText, as raw JSON; "null" (or empty)
	// for an output marked Once that was already read. See Text.
	Value  json.RawMessage `json:"value"`
	Secret bool            `json:"secret"`
	Once   bool            `json:"once"`
}

// Text returns Value when it is a plain string.
func (o TemplateDeployOutput) Text() (string, bool) {
	if len(o.Value) == 0 || o.Value[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(o.Value, &s) != nil {
		return "", false
	}
	return s, true
}

// TemplateDeploy is a deploy or update job.
type TemplateDeploy struct {
	ID         string               `json:"id"`
	Kind       TemplateDeployKind   `json:"kind"`
	TemplateID string               `json:"template_id"`
	VersionID  string               `json:"version_id"`
	Version    string               `json:"version"`
	Status     TemplateDeployStatus `json:"status"`
	// CurrentStep is nil when the job is not running.
	CurrentStep *TemplateDeployStep       `json:"current_step"`
	Steps       []TemplateDeployStepState `json:"steps"`
	Error       *TemplateDeployError      `json:"error"`
	// Resources are what the job created or updated.
	Resources []TemplateDeployResource `json:"resources"`
	// StackID is the stack of a template with more than one service; nil otherwise.
	StackID *string `json:"stack_id"`
	// ApplicationID is the application of a template with a single application service; nil otherwise.
	ApplicationID *string                `json:"application_id"`
	Outputs       []TemplateDeployOutput `json:"outputs"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	FinishedAt    *time.Time             `json:"finished_at"`
}

// TemplateDeployEvent is one event of the progress stream of a job. It never
// carries outputs, resources or values.
type TemplateDeployEvent struct {
	DeployID    string                    `json:"deploy_id"`
	Status      TemplateDeployStatus      `json:"status"`
	CurrentStep *TemplateDeployStep       `json:"current_step"`
	Steps       []TemplateDeployStepState `json:"steps"`
	Error       *TemplateDeployError      `json:"error"`
}

// TemplateDeployEventStream is the progress of a deploy job, streamed as
// Server-Sent Events:
//
//	stream, err := client.Templates.Deploys().Events(ctx, job.ID)
//	if err != nil { ... }
//	defer stream.Close()
//	for stream.Next(ctx) {
//		ev := stream.Event()
//		...
//	}
//	if err := stream.Err(); err != nil { ... }
//	job, err := client.Templates.Deploys().Get(ctx, job.ID) // outputs
//
// Heartbeats are skipped. Next returns false when the API closes the stream
// with its "end" event (Ended reports true), when the connection ends, or on
// error.
type TemplateDeployEventStream struct {
	es    *rest.EventStream
	cur   TemplateDeployEvent
	err   error
	ended bool
}

// Next advances to the next progress event.
func (s *TemplateDeployEventStream) Next(ctx context.Context) bool {
	if s.err != nil || s.ended {
		return false
	}
	for s.es.Next(ctx) {
		ev := s.es.Event()
		switch ev.Event {
		case "end":
			s.ended = true
			return false
		case "progress", "message", "":
			if ev.Data == "" {
				continue
			}
			var decoded TemplateDeployEvent
			if err := json.Unmarshal([]byte(ev.Data), &decoded); err != nil {
				s.err = fmt.Errorf("vertracloud: decode deploy event: %w", err)
				return false
			}
			s.cur = decoded
			return true
		default:
			// heartbeat and unknown events
		}
	}
	return false
}

// Event returns the event produced by the most recent successful Next.
func (s *TemplateDeployEventStream) Event() TemplateDeployEvent { return s.cur }

// Ended reports whether the stream closed with the API's "end" event.
func (s *TemplateDeployEventStream) Ended() bool { return s.ended }

// Err returns the error that stopped iteration, or nil.
func (s *TemplateDeployEventStream) Err() error {
	if s.err != nil {
		return s.err
	}
	return s.es.Err()
}

// Close stops the stream. Safe to call more than once.
func (s *TemplateDeployEventStream) Close() error { return s.es.Close() }

// ---------------------------------------------------------------------------
// Private template payloads
// ---------------------------------------------------------------------------

// TemplateCreateParams is the input of TemplatesService.Create.
type TemplateCreateParams struct {
	// Manifest is the manifest: a TemplateManifest, a map, json.RawMessage or
	// any value that marshals to JSON. Required.
	Manifest any
	// Changelog is the message of the first version. Optional.
	Changelog *TemplateI18nText
	// WorkspaceID makes the workspace the owner of the template when set.
	WorkspaceID string
	// Sources holds the ZIP of the code of each service of type "code", by
	// service id. Services of type "image" need none: they refer to an image of
	// your account by id.
	Sources map[string]io.Reader
}

// TemplateVersionCreateParams is the input of TemplatesService.NewVersion.
type TemplateVersionCreateParams struct {
	// Manifest is the new manifest; its version must be higher than the current one. Required.
	Manifest any
	// Changelog is the message of the version. Optional.
	Changelog *TemplateI18nText
	// Sources holds the ZIP of the code of each service of type "code", by service id.
	Sources map[string]io.Reader
}

// TemplateEnvReview is the review of one variable when saving an app as a template.
type TemplateEnvReview struct {
	Key  string                `json:"key"`
	Mode TemplateEnvReviewMode `json:"mode"`
	// Label is the label of the question, with "question" and "generate".
	Label *TemplateI18nText `json:"label,omitempty"`
	Help  *TemplateI18nText `json:"help,omitempty"`
	// Generate is how it is generated, "random:<n>" or "uuid", for "generate".
	Generate string `json:"generate,omitempty"`
}

// TemplateFromMember is what to save of one member of the application or stack.
type TemplateFromMember struct {
	// ResourceID is the id of the application or database.
	ResourceID string `json:"resource_id"`
	// ServiceID is the id of the service in the manifest.
	ServiceID string `json:"service_id"`
	// Env is a review for every variable of the application; a variable
	// without review is left out.
	Env []TemplateEnvReview `json:"env,omitempty"`
	// Paths are the paths of the /app that go into the template (code applications only).
	Paths []string     `json:"paths,omitempty"`
	RAM   *TemplateRAM `json:"ram,omitempty"`
}

// TemplateFromAppBody is the body of TemplatesService.FromApp.
type TemplateFromAppBody struct {
	// ID is the slug of the template.
	ID          string           `json:"id"`
	Name        TemplateI18nText `json:"name"`
	Description TemplateI18nText `json:"description"`
	Category    TemplateCategory `json:"category"`
	Tags        []string         `json:"tags,omitempty"`
	// Version defaults to 1.0.0.
	Version     string               `json:"version,omitempty"`
	Permissions *TemplatePermissions `json:"permissions,omitempty"`
	// Member is what to save of the application.
	Member      TemplateFromMember `json:"member"`
	WorkspaceID string             `json:"workspace_id,omitempty"`
}

// TemplateFromStackBody is the body of TemplatesService.FromStack.
type TemplateFromStackBody struct {
	ID          string               `json:"id"`
	Name        TemplateI18nText     `json:"name"`
	Description TemplateI18nText     `json:"description"`
	Category    TemplateCategory     `json:"category"`
	Tags        []string             `json:"tags,omitempty"`
	Version     string               `json:"version,omitempty"`
	Permissions *TemplatePermissions `json:"permissions,omitempty"`
	// Members is what to save of each member of the stack.
	Members     []TemplateFromMember `json:"members"`
	WorkspaceID string               `json:"workspace_id,omitempty"`
}

// TemplateDraftEnv is a variable of an app as the review screen offers it.
type TemplateDraftEnv struct {
	Key string `json:"key"`
	// LooksSecret is true when the name looks like a secret: it should not be copied.
	LooksSecret   bool                  `json:"looks_secret"`
	SuggestedMode TemplateEnvReviewMode `json:"suggested_mode"`
}

// TemplateDraftPath is a file or folder of the /app of an application.
type TemplateDraftPath struct {
	Path string `json:"path"`
	// Type is "file" or "directory".
	Type string `json:"type"`
	// Selected is whether it goes into the template by default.
	Selected bool `json:"selected"`
	// Reason is why it is not selected by default (env_file, platform_folder,
	// dependencies, database_file, uploads); nil when it is.
	Reason *string `json:"reason"`
}

// TemplateDraftMember is what the platform proposes for one member.
type TemplateDraftMember struct {
	ResourceID string `json:"resource_id"`
	// ResourceType is "application" or "database".
	ResourceType WorkspaceResourceType `json:"resource_type"`
	Name         string                `json:"name"`
	// RAM is a good starting point for the memory of the service.
	RAM   TemplateRAM         `json:"ram"`
	Env   []TemplateDraftEnv  `json:"env"`
	Paths []TemplateDraftPath `json:"paths"`
}

// TemplateDraft is what the platform proposes when an application (or the
// members of a stack) becomes a template. Values are never included.
type TemplateDraft struct {
	Members []TemplateDraftMember `json:"members"`
}

// TemplateUpdateDetailsBody is the body of TemplatesService.Update. At least
// one field is required; a nil or empty field is left untouched.
type TemplateUpdateDetailsBody struct {
	Name        *TemplateI18nText `json:"name,omitempty"`
	Description *TemplateI18nText `json:"description,omitempty"`
	Category    TemplateCategory  `json:"category,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// TemplateGitHubLinkBody is the body of TemplatesService.LinkGitHub.
type TemplateGitHubLinkBody struct {
	// Repository is the repository, as "owner/name".
	Repository string `json:"repository"`
	// Branch defaults to the default branch of the repository when empty.
	Branch string `json:"branch,omitempty"`
}

// ---------------------------------------------------------------------------
// Share payloads
// ---------------------------------------------------------------------------

// TemplateShareLinkCreateBody is the body of TemplateShareLinksService.Create.
type TemplateShareLinkCreateBody struct {
	// Mode defaults to "use". "full_copy" needs permissions.share.full_copy
	// in the template and is not compatible with a closed template.
	Mode TemplateShareLinkMode `json:"mode,omitempty"`
	// MaxUses is the maximum number of uses; 0 means no limit.
	MaxUses int `json:"max_uses,omitempty"`
	// ExpiresInDays is the days until the link expires; 0 means it does not expire.
	ExpiresInDays int `json:"expires_in_days,omitempty"`
}

// TemplateShareLink is a share link of a private template. The token is
// never listed.
type TemplateShareLink struct {
	ID         string                `json:"id"`
	TemplateID string                `json:"template_id"`
	Mode       TemplateShareLinkMode `json:"mode"`
	// Last4 are the last characters of the token, to tell links apart.
	Last4     string     `json:"last4"`
	Uses      int        `json:"uses"`
	MaxUses   *int       `json:"max_uses"`
	ExpiresAt *time.Time `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// TemplateShareLinkCreated is a new share link with its token, shown only now.
type TemplateShareLinkCreated struct {
	TemplateShareLink
	Token string `json:"token"`
}

// TemplateSharePreview is what a share link gives, before using it.
type TemplateSharePreview struct {
	Mode          TemplateShareLinkMode    `json:"mode"`
	Name          TemplateI18nText         `json:"name"`
	Description   TemplateI18nText         `json:"description"`
	Category      TemplateCategory         `json:"category"`
	IconURL       *string                  `json:"icon_url"`
	AuthorDisplay *string                  `json:"author_display"`
	Version       string                   `json:"version"`
	Services      []TemplateServiceSummary `json:"services"`
	CreatesStack  bool                     `json:"creates_stack"`
	RAMMinTotal   int                      `json:"ram_min_total"`
	RequiredPlan  UserPlanName             `json:"required_plan"`
	// ImageSlotsNeeded is the number of image slots using the link takes.
	ImageSlotsNeeded int `json:"image_slots_needed"`
	// Verified is always false: a template shared by link is not reviewed by the platform.
	Verified    bool                         `json:"verified"`
	CodeView    bool                         `json:"code_view"`
	BYOK        bool                         `json:"byok"`
	Inputs      []TemplateInput              `json:"inputs"`
	Permissions TemplateEffectivePermissions `json:"permissions"`
	// Account is how the template fits your account; nil without authentication.
	Account *TemplateAccountContext `json:"account,omitempty"`
}

// TemplateShareCopyBody is the body of TemplateShareService.Copy.
type TemplateShareCopyBody struct {
	// WorkspaceID is the workspace that will own the copy.
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// TemplateReportBody is the body of TemplateShareService.Report and
// TemplatesService.ReportApp.
type TemplateReportBody struct {
	Reason TemplateReportReason `json:"reason"`
	// Details is free text, up to 1,000 characters.
	Details string `json:"details,omitempty"`
}

// ---------------------------------------------------------------------------
// Update payloads
// ---------------------------------------------------------------------------

// TemplateUpdateBlocked is why an update is not possible.
type TemplateUpdateBlocked struct {
	Reason  TemplateUpdateBlockReason `json:"reason"`
	Details map[string]any            `json:"details,omitempty"`
}

// TemplateRAMChange is a service whose minimum memory rose in the new version.
type TemplateRAMChange struct {
	ServiceID   string `json:"service_id"`
	Current     int    `json:"current"`
	RequiredMin int    `json:"required_min"`
}

// TemplateUpdateInfo is what updating to the latest version would do.
type TemplateUpdateInfo struct {
	UpdateAvailable bool    `json:"update_available"`
	CurrentVersion  string  `json:"current_version"`
	LatestVersion   *string `json:"latest_version"`
	// Security is true when the new version fixes a security problem.
	Security  bool              `json:"security"`
	Changelog *TemplateI18nText `json:"changelog"`
	// NewInputs are the required questions the new version adds.
	NewInputs       []TemplateInput          `json:"new_inputs"`
	AddedServices   []TemplateServiceSummary `json:"added_services"`
	RemovedServices []string                 `json:"removed_services"`
	RAMChanges      []TemplateRAMChange      `json:"ram_changes"`
	FreeRAM         int                      `json:"free_ram"`
	// OpensPermissions is true when the new version gives more permissions.
	OpensPermissions bool `json:"opens_permissions"`
	// RollbackWithData is true when going back to the safety snapshot restores the data as well.
	RollbackWithData bool `json:"rollback_with_data"`
	// Blocked is why the update cannot run now; nil when it can.
	Blocked *TemplateUpdateBlocked `json:"blocked"`
}

// TemplateUpdateBody is the body of TemplateUpdatesService.AppUpdate and StackUpdate.
type TemplateUpdateBody struct {
	// Inputs are the answers to the required questions the new version adds.
	Inputs map[string]TemplateInputValue `json:"inputs,omitempty"`
	// RAM is the new memory in MB per service, for the services whose minimum memory rose.
	RAM map[string]int `json:"ram,omitempty"`
}

// ---------------------------------------------------------------------------
// Implementation
// ---------------------------------------------------------------------------

func newTemplatesService(rc rest.Client) TemplatesService {
	return &templatesServiceImpl{rest: guardPaths(rc)}
}

type templatesServiceImpl struct{ rest rest.Client }

func (s *templatesServiceImpl) Deploys() TemplateDeploysService {
	return &templateDeploysServiceImpl{rest: s.rest}
}
func (s *templatesServiceImpl) ShareLinks() TemplateShareLinksService {
	return &templateShareLinksServiceImpl{rest: s.rest}
}
func (s *templatesServiceImpl) Share() TemplateShareService {
	return &templateShareServiceImpl{rest: s.rest}
}
func (s *templatesServiceImpl) Updates() TemplateUpdatesService {
	return &templateUpdatesServiceImpl{rest: s.rest}
}

// templatePath builds /v1/templates/:id<suffix>, URL-encoding id.
func templatePath(id, suffix string) string {
	return "/v1/templates/" + url.PathEscape(id) + suffix
}

// templateJSON runs a request with a JSON body (nil body = no body) and
// returns the raw response.
func templateJSON(ctx context.Context, rc rest.Client, method, path string, query url.Values, body any, opts []rest.RequestOpt) ([]byte, error) {
	if body == nil {
		return rc.Do(ctx, method, path, query, nil, "", opts...)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	return rc.Do(ctx, method, path, query, bytes.NewReader(payload), "application/json", opts...)
}

// templateCall is templateJSON plus decoding of the {"response": ...} envelope.
func templateCall[T any](ctx context.Context, rc rest.Client, method, path string, query url.Values, body any, opts []rest.RequestOpt) (T, error) {
	var zero T
	data, err := templateJSON(ctx, rc, method, path, query, body, opts)
	if err != nil {
		return zero, err
	}
	return rest.DecodeJSON[T](data)
}

// withIdempotencyKey adds the Idempotency-Key header when key is not empty.
func withIdempotencyKey(key string, opts []rest.RequestOpt) []rest.RequestOpt {
	if key == "" {
		return opts
	}
	return append([]rest.RequestOpt{rest.WithHeader("Idempotency-Key", key)}, opts...)
}

func (s *templatesServiceImpl) List(ctx context.Context, params *TemplateListParams, opts ...rest.RequestOpt) (TemplatePage, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "search", params.Search)
		addQueryParam(q, "category", string(params.Category))
		addQueryParam(q, "origin", string(params.Origin))
		addQueryParam(q, "tag", params.Tag)
		addQueryBool(q, "fits", params.Fits)
		addQueryParam(q, "sort", string(params.Sort))
		if params.Page > 0 {
			q.Set("page", strconv.Itoa(params.Page))
		}
		if params.Limit > 0 {
			q.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.WorkspaceID != "" {
			opts = append([]rest.RequestOpt{rest.WithWorkspaceID(params.WorkspaceID)}, opts...)
		}
	}
	return templateCall[TemplatePage](ctx, s.rest, http.MethodGet, "/v1/templates", q, nil, opts)
}

func (s *templatesServiceImpl) Get(ctx context.Context, idOrSlug string, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodGet, templatePath(idOrSlug, ""), nil, nil, opts)
}

func (s *templatesServiceImpl) Categories(ctx context.Context, opts ...rest.RequestOpt) ([]TemplateCategoryCount, error) {
	return templateCall[[]TemplateCategoryCount](ctx, s.rest, http.MethodGet, "/v1/templates/categories", nil, nil, opts)
}

func (s *templatesServiceImpl) Versions(ctx context.Context, idOrSlug string, opts ...rest.RequestOpt) ([]TemplateVersion, error) {
	return templateCall[[]TemplateVersion](ctx, s.rest, http.MethodGet, templatePath(idOrSlug, "/versions"), nil, nil, opts)
}

func (s *templatesServiceImpl) Validate(ctx context.Context, manifest any, opts ...rest.RequestOpt) (TemplateValidation, error) {
	return templateCall[TemplateValidation](ctx, s.rest, http.MethodPost, "/v1/templates/validate", nil, struct {
		Manifest any `json:"manifest"`
	}{manifest}, opts)
}

func (s *templatesServiceImpl) Deploy(ctx context.Context, idOrSlug string, body TemplateDeployBody, opts ...rest.RequestOpt) (TemplateDeploy, error) {
	return templateCall[TemplateDeploy](ctx, s.rest, http.MethodPost, templatePath(idOrSlug, "/deploy"), nil, body, withIdempotencyKey(body.IdempotencyKey, opts))
}

// multipartTemplate builds the multipart body shared by Create and NewVersion:
// the manifest and changelog as JSON text fields, then one ZIP part per
// service in a stable order.
func multipartTemplate(manifest any, changelog *TemplateI18nText, workspaceID string, sources map[string]io.Reader) (io.Reader, string, error) {
	if manifest == nil {
		return nil, "", errors.New("vertracloud: Manifest is required")
	}
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", fmt.Errorf("vertracloud: encode manifest: %w", err)
	}
	fields := []multipartField{{Name: "manifest", Value: string(rawManifest)}}
	if changelog != nil {
		rawChangelog, err := json.Marshal(changelog)
		if err != nil {
			return nil, "", fmt.Errorf("vertracloud: encode changelog: %w", err)
		}
		fields = append(fields, multipartField{Name: "changelog", Value: string(rawChangelog)})
	}
	if workspaceID != "" {
		fields = append(fields, multipartField{Name: "workspace_id", Value: workspaceID})
	}
	ids := make([]string, 0, len(sources))
	for id := range sources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if sources[id] == nil {
			return nil, "", fmt.Errorf("vertracloud: source of service %q is nil", id)
		}
		fields = append(fields, multipartField{
			Name:        "sources[" + id + "]",
			FileName:    id + ".zip",
			ContentType: "application/zip",
			Reader:      sources[id],
		})
	}
	body, contentType := newMultipartBody(fields)
	return body, contentType, nil
}

func (s *templatesServiceImpl) Create(ctx context.Context, params TemplateCreateParams, opts ...rest.RequestOpt) (TemplateDetail, error) {
	body, contentType, err := multipartTemplate(params.Manifest, params.Changelog, params.WorkspaceID, params.Sources)
	if err != nil {
		return TemplateDetail{}, err
	}
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/templates", nil, body, contentType, opts...)
	if err != nil {
		return TemplateDetail{}, err
	}
	return rest.DecodeJSON[TemplateDetail](data)
}

func (s *templatesServiceImpl) NewVersion(ctx context.Context, id string, params TemplateVersionCreateParams, opts ...rest.RequestOpt) (TemplateVersion, error) {
	body, contentType, err := multipartTemplate(params.Manifest, params.Changelog, "", params.Sources)
	if err != nil {
		return TemplateVersion{}, err
	}
	data, err := s.rest.Do(ctx, http.MethodPost, templatePath(id, "/versions"), nil, body, contentType, opts...)
	if err != nil {
		return TemplateVersion{}, err
	}
	return rest.DecodeJSON[TemplateVersion](data)
}

func (s *templatesServiceImpl) FromApp(ctx context.Context, appID string, body TemplateFromAppBody, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodPost, "/v1/templates/from-app/"+url.PathEscape(appID), nil, body, opts)
}

func (s *templatesServiceImpl) FromStack(ctx context.Context, stackID string, body TemplateFromStackBody, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodPost, "/v1/templates/from-stack/"+url.PathEscape(stackID), nil, body, opts)
}

func (s *templatesServiceImpl) AppDraft(ctx context.Context, appID string, opts ...rest.RequestOpt) (TemplateDraft, error) {
	return templateCall[TemplateDraft](ctx, s.rest, http.MethodGet, "/v1/templates/from-app/"+url.PathEscape(appID)+"/draft", nil, nil, opts)
}

func (s *templatesServiceImpl) StackDraft(ctx context.Context, stackID string, opts ...rest.RequestOpt) (TemplateDraft, error) {
	return templateCall[TemplateDraft](ctx, s.rest, http.MethodGet, "/v1/templates/from-stack/"+url.PathEscape(stackID)+"/draft", nil, nil, opts)
}

func (s *templatesServiceImpl) Update(ctx context.Context, id string, body TemplateUpdateDetailsBody, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodPatch, templatePath(id, ""), nil, body, opts)
}

func (s *templatesServiceImpl) Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error {
	_, err := templateJSON(ctx, s.rest, http.MethodDelete, templatePath(id, ""), nil, nil, opts)
	return err
}

func (s *templatesServiceImpl) LinkGitHub(ctx context.Context, id string, body TemplateGitHubLinkBody, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodPut, templatePath(id, "/github"), nil, body, opts)
}

func (s *templatesServiceImpl) UnlinkGitHub(ctx context.Context, id string, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodDelete, templatePath(id, "/github"), nil, nil, opts)
}

func (s *templatesServiceImpl) ReportApp(ctx context.Context, appID string, body TemplateReportBody, opts ...rest.RequestOpt) error {
	_, err := templateJSON(ctx, s.rest, http.MethodPost, appPath(appID, "/template/report"), nil, body, opts)
	return err
}

// --- deploys ---

type templateDeploysServiceImpl struct{ rest rest.Client }

func (s *templateDeploysServiceImpl) Get(ctx context.Context, deployID string, opts ...rest.RequestOpt) (TemplateDeploy, error) {
	return templateCall[TemplateDeploy](ctx, s.rest, http.MethodGet, "/v1/template-deploys/"+url.PathEscape(deployID), nil, nil, opts)
}

func (s *templateDeploysServiceImpl) Events(ctx context.Context, deployID string, opts ...rest.RequestOpt) (*TemplateDeployEventStream, error) {
	es, err := rest.OpenEventStream(ctx, s.rest, "/v1/template-deploys/"+url.PathEscape(deployID)+"/events", nil, opts...)
	if err != nil {
		return nil, err
	}
	return &TemplateDeployEventStream{es: es}, nil
}

// --- share links ---

type templateShareLinksServiceImpl struct{ rest rest.Client }

func (s *templateShareLinksServiceImpl) Create(ctx context.Context, templateID string, body TemplateShareLinkCreateBody, opts ...rest.RequestOpt) (TemplateShareLinkCreated, error) {
	return templateCall[TemplateShareLinkCreated](ctx, s.rest, http.MethodPost, templatePath(templateID, "/share-links"), nil, body, opts)
}

func (s *templateShareLinksServiceImpl) List(ctx context.Context, templateID string, opts ...rest.RequestOpt) ([]TemplateShareLink, error) {
	return templateCall[[]TemplateShareLink](ctx, s.rest, http.MethodGet, templatePath(templateID, "/share-links"), nil, nil, opts)
}

func (s *templateShareLinksServiceImpl) Revoke(ctx context.Context, templateID, linkID string, opts ...rest.RequestOpt) (TemplateShareLink, error) {
	return templateCall[TemplateShareLink](ctx, s.rest, http.MethodDelete, templatePath(templateID, "/share-links/"+url.PathEscape(linkID)), nil, nil, opts)
}

// --- share ---

type templateShareServiceImpl struct{ rest rest.Client }

func sharePath(token, suffix string) string {
	return "/v1/share/templates/" + url.PathEscape(token) + suffix
}

func (s *templateShareServiceImpl) Preview(ctx context.Context, token string, opts ...rest.RequestOpt) (TemplateSharePreview, error) {
	return templateCall[TemplateSharePreview](ctx, s.rest, http.MethodGet, sharePath(token, ""), nil, nil, opts)
}

func (s *templateShareServiceImpl) Deploy(ctx context.Context, token string, body TemplateShareDeployBody, opts ...rest.RequestOpt) (TemplateDeploy, error) {
	return templateCall[TemplateDeploy](ctx, s.rest, http.MethodPost, sharePath(token, "/deploy"), nil, body, withIdempotencyKey(body.IdempotencyKey, opts))
}

func (s *templateShareServiceImpl) Copy(ctx context.Context, token string, body TemplateShareCopyBody, opts ...rest.RequestOpt) (TemplateDetail, error) {
	return templateCall[TemplateDetail](ctx, s.rest, http.MethodPost, sharePath(token, "/copy"), nil, body, opts)
}

func (s *templateShareServiceImpl) Report(ctx context.Context, token string, body TemplateReportBody, opts ...rest.RequestOpt) error {
	_, err := templateJSON(ctx, s.rest, http.MethodPost, sharePath(token, "/report"), nil, body, opts)
	return err
}

// --- updates ---

type templateUpdatesServiceImpl struct{ rest rest.Client }

func (s *templateUpdatesServiceImpl) AppInfo(ctx context.Context, appID string, opts ...rest.RequestOpt) (TemplateUpdateInfo, error) {
	return templateCall[TemplateUpdateInfo](ctx, s.rest, http.MethodGet, appPath(appID, "/template/update"), nil, nil, opts)
}

func (s *templateUpdatesServiceImpl) AppUpdate(ctx context.Context, appID string, body TemplateUpdateBody, opts ...rest.RequestOpt) (TemplateDeploy, error) {
	return templateCall[TemplateDeploy](ctx, s.rest, http.MethodPost, appPath(appID, "/template/update"), nil, body, opts)
}

func (s *templateUpdatesServiceImpl) AppDetach(ctx context.Context, appID string, opts ...rest.RequestOpt) (Application, error) {
	return templateCall[Application](ctx, s.rest, http.MethodPost, appPath(appID, "/template/detach"), nil, nil, opts)
}

func (s *templateUpdatesServiceImpl) StackInfo(ctx context.Context, stackID string, opts ...rest.RequestOpt) (TemplateUpdateInfo, error) {
	return templateCall[TemplateUpdateInfo](ctx, s.rest, http.MethodGet, stackPath(stackID, "/template/update"), nil, nil, opts)
}

func (s *templateUpdatesServiceImpl) StackUpdate(ctx context.Context, stackID string, body TemplateUpdateBody, opts ...rest.RequestOpt) (TemplateDeploy, error) {
	return templateCall[TemplateDeploy](ctx, s.rest, http.MethodPost, stackPath(stackID, "/template/update"), nil, body, opts)
}

func (s *templateUpdatesServiceImpl) StackDetach(ctx context.Context, stackID string, opts ...rest.RequestOpt) (Stack, error) {
	return templateCall[Stack](ctx, s.rest, http.MethodPost, stackPath(stackID, "/template/detach"), nil, nil, opts)
}
