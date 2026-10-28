package vertracloud

import (
	"encoding/json"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// Error codes of the template and stack routes, as found in APIError.Code.
// Compare with the constants instead of retyping the strings:
//
//	if apiErr, ok := rest.AsAPIError(err); ok && apiErr.Code == vertracloud.ErrCodeTemplateRAMInsufficient { ... }
const (
	// Templates.
	ErrCodeTemplateNotFound               = "TEMPLATE_NOT_FOUND"
	ErrCodeTemplateVersionNotFound        = "TEMPLATE_VERSION_NOT_FOUND"
	ErrCodeTemplateSlugTaken              = "TEMPLATE_SLUG_TAKEN"
	ErrCodeTemplateManifestInvalid        = "TEMPLATE_MANIFEST_INVALID" // details: see TemplateManifestIssues
	ErrCodeTemplateInputInvalid           = "TEMPLATE_INPUT_INVALID"    // details.issues lists the questions
	ErrCodeTemplateSourceRequired         = "TEMPLATE_SOURCE_REQUIRED"
	ErrCodeTemplateSecretInSource         = "TEMPLATE_SECRET_IN_SOURCE" // details: see TemplateSecretFiles
	ErrCodeTemplateGitHubManifestNotFound = "TEMPLATE_GITHUB_MANIFEST_NOT_FOUND"
	ErrCodeTemplatePlanRequired           = "TEMPLATE_PLAN_REQUIRED"
	ErrCodeTemplateRAMInsufficient        = "TEMPLATE_RAM_INSUFFICIENT"
	ErrCodeTemplateImageSlotsExhausted    = "TEMPLATE_IMAGE_SLOTS_EXHAUSTED"
	ErrCodeTemplateClosed                 = "TEMPLATE_CLOSED"
	ErrCodeTemplatePrivateLimitReached    = "TEMPLATE_PRIVATE_LIMIT_REACHED"
	ErrCodeTemplateDeployInProgress       = "TEMPLATE_DEPLOY_IN_PROGRESS"
	ErrCodeTemplateDeployFailed           = "TEMPLATE_DEPLOY_FAILED" // the deploy was undone; details.step is the step that failed
	ErrCodeTemplateDeployNotFound         = "TEMPLATE_DEPLOY_NOT_FOUND"
	ErrCodeIdempotencyKeyConflict         = "IDEMPOTENCY_KEY_CONFLICT"
	ErrCodeImageTemplateOnly              = "IMAGE_TEMPLATE_ONLY"
	ErrCodeImageInUse                     = "IMAGE_IN_USE" // details.templates lists the ids of the templates that use the image
	ErrCodeTemplateSaveNotAllowed         = "TEMPLATE_SAVE_NOT_ALLOWED"
	ErrCodeTemplateUpdateBlocked          = "TEMPLATE_UPDATE_BLOCKED" // details: see TemplateUpdateBlockedDetails
	ErrCodeTemplateDeprecated             = "TEMPLATE_DEPRECATED"
	ErrCodeTemplateVersionNotSelectable   = "TEMPLATE_VERSION_NOT_SELECTABLE"
	ErrCodeTemplateDetachConfirmRequired  = "TEMPLATE_DETACH_CONFIRMATION_REQUIRED" // details: see TemplateDetachConfirmation
	ErrCodeTemplateRollbackRequiresData   = "TEMPLATE_ROLLBACK_REQUIRES_DATA"
	ErrCodeTemplateStorageUnavailable     = "TEMPLATE_STORAGE_UNAVAILABLE"
	ErrCodeManagedEnvReadonly             = "MANAGED_ENV_READONLY"

	// Share links.
	ErrCodeShareLinkNotFound  = "SHARE_LINK_NOT_FOUND"
	ErrCodeShareLinkRevoked   = "SHARE_LINK_REVOKED"
	ErrCodeShareLinkExpired   = "SHARE_LINK_EXPIRED"
	ErrCodeShareLinkExhausted = "SHARE_LINK_EXHAUSTED"

	// Stacks.
	ErrCodeStackNotFound               = "STACK_NOT_FOUND"
	ErrCodeStackMemberConflict         = "STACK_MEMBER_CONFLICT"
	ErrCodeStackMemberNotFound         = "STACK_MEMBER_NOT_FOUND"
	ErrCodeStackBindingNotFound        = "STACK_BINDING_NOT_FOUND"
	ErrCodeStackNetworkModeUnavailable = "STACK_NETWORK_MODE_UNAVAILABLE"
	ErrCodeStackServiceLocked          = "STACK_SERVICE_LOCKED"
	ErrCodeStackNameMismatch           = "STACK_NAME_MISMATCH"
	ErrCodeStackTransformEmpty         = "STACK_TRANSFORM_EMPTY"
	ErrCodeStackBlobPrefixTaken        = "STACK_BLOB_PREFIX_TAKEN"

	// API keys restricted to a Blob folder, or managed by the platform.
	ErrCodeBlobPrefixDenied = "BLOB_PREFIX_DENIED"
	ErrCodeAPIKeyManaged    = "API_KEY_MANAGED"
)

// errorDetails decodes the details of err into T when err is an API error
// with the given code. ok is false for any other error or nil; ok is true
// with the zero T when the details are missing or unreadable.
func errorDetails[T any](err error, code string) (details T, ok bool) {
	apiErr, isAPI := rest.AsAPIError(err)
	if !isAPI || apiErr.Code != code {
		return details, false
	}
	raw, marshalErr := json.Marshal(apiErr.Details)
	if marshalErr != nil {
		return details, true
	}
	var decoded T
	if json.Unmarshal(raw, &decoded) != nil {
		return details, true
	}
	return decoded, true
}

// TemplateManifestIssues returns the findings of a TEMPLATE_MANIFEST_INVALID
// error (from Create, NewVersion, FromApp, ...). ok is false for any other error.
func TemplateManifestIssues(err error) (issues []TemplateValidationIssue, ok bool) {
	d, ok := errorDetails[struct {
		Issues []TemplateValidationIssue `json:"issues"`
	}](err, ErrCodeTemplateManifestInvalid)
	return d.Issues, ok
}

// TemplateSecretFiles returns the paths of a TEMPLATE_SECRET_IN_SOURCE error:
// the files of the code sent to become a template that hold a secret. ok is
// false for any other error.
func TemplateSecretFiles(err error) (files []string, ok bool) {
	d, ok := errorDetails[struct {
		Files []string `json:"files"`
	}](err, ErrCodeTemplateSecretInSource)
	return d.Files, ok
}

// TemplateUpdateBlockedDetails returns the reason of a TEMPLATE_UPDATE_BLOCKED
// error. ok is false for any other error.
func TemplateUpdateBlockedDetails(err error) (blocked TemplateUpdateBlocked, ok bool) {
	return errorDetails[TemplateUpdateBlocked](err, ErrCodeTemplateUpdateBlocked)
}

// TemplateDetachConfirmationAction is the action that needs ConfirmDetach.
type TemplateDetachConfirmationAction string

const (
	TemplateDetachConfirmationActionFilesWrite   TemplateDetachConfirmationAction = "files.write"
	TemplateDetachConfirmationActionDeployCode   TemplateDetachConfirmationAction = "deploy.code"
	TemplateDetachConfirmationActionDeployGitHub TemplateDetachConfirmationAction = "deploy.github"
	TemplateDetachConfirmationActionCommit       TemplateDetachConfirmationAction = "commit"
)

// TemplateDetachConfirmation returns the action of a
// TEMPLATE_DETACH_CONFIRMATION_REQUIRED error: repeat the call with
// ConfirmDetach set to true to detach the application from its template. ok is
// false for any other error.
func TemplateDetachConfirmation(err error) (action TemplateDetachConfirmationAction, ok bool) {
	d, ok := errorDetails[struct {
		Action TemplateDetachConfirmationAction `json:"action"`
	}](err, ErrCodeTemplateDetachConfirmRequired)
	return d.Action, ok
}

// ---------------------------------------------------------------------------
// API keys
// ---------------------------------------------------------------------------

// APIKeyManager says which stack the platform created an API key for. Such a
// key cannot be edited, rotated or revoked by hand (API_KEY_MANAGED): it is
// revoked when the stack is dissolved or deleted.
type APIKeyManager struct {
	// Type is "stack".
	Type string `json:"type"`
	// ID is the id of the stack the key was created for.
	ID string `json:"id"`
}

// APIKey is an API key as listed. The SDK has no route for API keys: the type
// is here to read the keys of a stack's Blob folder and for code that decodes
// them itself.
type APIKey struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Last4      *string  `json:"last4"`
	Scopes     []string `json:"scopes"`
	AllowedIPs []string `json:"allowed_ips"`
	// BlobPrefix is the Blob folder the key is restricted to (ends in "/");
	// nil means the whole Blob. A restricted key only works with the blob:*
	// scopes, only reaches objects under the folder and is refused
	// account-level Blob operations with BLOB_PREFIX_DENIED.
	BlobPrefix *string `json:"blob_prefix,omitempty"`
	// ManagedBy is set when the platform created the key for a stack; nil for keys you created.
	ManagedBy  *APIKeyManager `json:"managed_by,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	LastUsedAt *time.Time     `json:"last_used_at"`
}

// API key scopes added with templates and stacks.
const (
	APIKeyScopeTemplatesRead   = "templates:read"
	APIKeyScopeTemplatesWrite  = "templates:write"
	APIKeyScopeTemplatesDeploy = "templates:deploy"
	APIKeyScopeStacksRead      = "stacks:read"
	APIKeyScopeStacksWrite     = "stacks:write"
	APIKeyScopeStacksDelete    = "stacks:delete"
)
