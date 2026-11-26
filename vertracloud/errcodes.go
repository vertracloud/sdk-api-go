package vertracloud

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// Error codes of the template, stack and web analytics routes, as found in APIError.Code.
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
	ErrCodeTemplateSettingLocked          = "TEMPLATE_SETTING_LOCKED" // details.setting: "zero_downtime" or "auto_rollback"
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

	// Regions and nodes.
	ErrCodeRegionNotAvailable = "REGION_NOT_AVAILABLE" // 400
	ErrCodeNodeNotAllowed     = "NODE_NOT_ALLOWED"     // 403
	ErrCodeNodeNotInRegion    = "NODE_NOT_IN_REGION"   // 400

	// Web Analytics.
	ErrCodePlanDoesNotSupportWebAnalyticsPerformance = "PLAN_DOES_NOT_SUPPORT_WEB_ANALYTICS_PERFORMANCE" // 403
	ErrCodePlanDoesNotSupportWebAnalyticsRequests    = "PLAN_DOES_NOT_SUPPORT_WEB_ANALYTICS_REQUESTS"    // 403
	ErrCodeAnalyticsRangeExceedsPlan                 = "ANALYTICS_RANGE_EXCEEDS_PLAN"                    // 400; details: see AnalyticsRangeExceedsPlan
	ErrCodeAnalyticsBusy                             = "ANALYTICS_BUSY"                                  // 503; details: see AnalyticsBusyRetryAfter

	// Snapshots.
	ErrCodeDatabaseNotReady         = "DATABASE_NOT_READY"          // 503
	ErrCodeSnapshotFailed           = "SNAPSHOT_FAILED"             // job reason
	ErrCodeSnapshotTimeout          = "SNAPSHOT_TIMEOUT"            // 504
	ErrCodeSnapshotTooLarge         = "SNAPSHOT_TOO_LARGE"          // 413; details: see SnapshotTooLargeMaxBytes
	ErrCodeSnapshotRateLimited      = "SNAPSHOT_RATE_LIMITED"       // 429; details: see SnapshotRateLimitDetails
	ErrCodeSnapshotNotFound         = "SNAPSHOT_NOT_FOUND"          // 404
	ErrCodeSnapshotExceedsStorage   = "SNAPSHOT_EXCEEDS_STORAGE"    // 400; details: see SnapshotExceedsStorageDetails
	ErrCodeSnapshotCorrupted        = "SNAPSHOT_CORRUPTED"          // job reason
	ErrCodeSnapshotImportFailed     = "SNAPSHOT_IMPORT_FAILED"      // job reason
	ErrCodeSnapshotEngineMismatch   = "SNAPSHOT_ENGINE_MISMATCH"    // job reason
	ErrCodeSnapshotVersionNewer     = "SNAPSHOT_VERSION_NEWER"      // the snapshot's engine line is newer than the database's
	ErrCodeStackRestoreMemberFailed = "STACK_RESTORE_MEMBER_FAILED" // job reason: an earlier member of the stack restore failed
	ErrCodeRestoreCooldown          = "RESTORE_COOLDOWN"            // 429; the wait between restores, see RestoreCooldownRetryAfter
	ErrCodeRestoreInProgress        = "RESTORE_IN_PROGRESS"         // 409; the resource is being restored
	ErrCodeAppBuilding              = "APP_BUILDING"                // 409; the app is being deployed
	ErrCodeOperationInProgress      = "OPERATION_IN_PROGRESS"       // 409; e.g. a snapshot of the app is running

	// File manager.
	ErrCodeFileTooLarge = "FILE_TOO_LARGE" // 413; details: see FileTooLargeMaxBytes

	// API keys restricted to a Blob folder, or managed by the platform.
	ErrCodeBlobPrefixDenied = "BLOB_PREFIX_DENIED"
	ErrCodeAPIKeyManaged    = "API_KEY_MANAGED"
)

// errorDetails decodes the details of err into T when err is an API error
// with the given code. ok is false for any other error or nil; ok is true
// with the zero T when the details are missing or unreadable.
func errorDetails[T any](err error, code string) (details T, ok bool) {
	var source any
	var jobErr *SnapshotJobError
	if errors.As(err, &jobErr) {
		if jobErr.Reason != code {
			return details, false
		}
		source = jobErr.Details
	} else if apiErr, isAPI := rest.AsAPIError(err); isAPI && apiErr.Code == code {
		source = apiErr.Details
	} else {
		return details, false
	}
	raw, marshalErr := json.Marshal(source)
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

// AnalyticsRangeExceedsPlan returns the details of an ANALYTICS_RANGE_EXCEEDS_PLAN
// error (400): how many days back the plan reads and the earliest Start accepted
// right now. ok is false for any other error.
func AnalyticsRangeExceedsPlan(err error) (details ApplicationAnalyticsRangeExceedsPlanDetails, ok bool) {
	return errorDetails[ApplicationAnalyticsRangeExceedsPlanDetails](err, ErrCodeAnalyticsRangeExceedsPlan)
}

// AnalyticsBusyRetryAfter returns how long to wait after an ANALYTICS_BUSY error
// (503: analytics is under high demand and no earlier answer is available; try
// again after the returned duration). ok is false for any other error; the duration is zero when the API sent none.
func AnalyticsBusyRetryAfter(err error) (wait time.Duration, ok bool) {
	d, ok := errorDetails[struct {
		RetryAfter float64 `json:"retry_after"`
	}](err, ErrCodeAnalyticsBusy)
	return time.Duration(d.RetryAfter * float64(time.Second)), ok
}

// SnapshotRateLimitScope is what a SNAPSHOT_RATE_LIMITED error counted against.
type SnapshotRateLimitScope string

const (
	SnapshotRateLimitScopeResource SnapshotRateLimitScope = "resource"
	SnapshotRateLimitScopeAccount  SnapshotRateLimitScope = "account"
)

// SnapshotRateLimit is the details of a SNAPSHOT_RATE_LIMITED error.
type SnapshotRateLimit struct {
	Scope SnapshotRateLimitScope `json:"scope"`
	// RetryAfter is how long to wait, in seconds.
	RetryAfter float64 `json:"retry_after"`
	// ResetsAt is when the account quota frees a snapshot. Nil unless Scope is "account".
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// SnapshotRateLimitDetails returns the details of a 429 SNAPSHOT_RATE_LIMITED
// error. ok is false for any other error.
func SnapshotRateLimitDetails(err error) (details SnapshotRateLimit, ok bool) {
	return errorDetails[SnapshotRateLimit](err, ErrCodeSnapshotRateLimited)
}

// SnapshotExceedsStorage is the details of a SNAPSHOT_EXCEEDS_STORAGE error.
type SnapshotExceedsStorage struct {
	// RequiredBytes is the space the snapshot needs.
	RequiredBytes int64 `json:"required_bytes"`
	// AvailableBytes is the space the target has.
	AvailableBytes int64 `json:"available_bytes"`
	// RequiredRAMMB is, for databases, the smallest memory size in MB whose storage has room. Zero when absent.
	RequiredRAMMB int `json:"required_ram_mb,omitempty"`
}

// SnapshotExceedsStorageDetails returns the details of the 400 that refuses a
// restore (or a create from a snapshot) because the snapshot does not fit in
// the target. ok is false for any other error.
func SnapshotExceedsStorageDetails(err error) (details SnapshotExceedsStorage, ok bool) {
	return errorDetails[SnapshotExceedsStorage](err, ErrCodeSnapshotExceedsStorage)
}

// acceptedJobID reads the {"response": {"job_id": ...}} body of a 202. ok is
// false for any other body, such as a snapshot (which has an "id").
func acceptedJobID(data []byte) (jobID string, ok bool) {
	var body struct {
		Response struct {
			ID    string `json:"id"`
			JobID string `json:"job_id"`
		} `json:"response"`
	}
	if json.Unmarshal(data, &body) != nil || body.Response.JobID == "" || body.Response.ID != "" {
		return "", false
	}
	return body.Response.JobID, true
}

// SnapshotTooLargeMaxBytes returns details.max_bytes of a SNAPSHOT_TOO_LARGE
// error (a direct 413 or a failed job): the largest snapshot allowed. ok is
// false for any other error; the limit is zero when the API sent none.
func SnapshotTooLargeMaxBytes(err error) (limit int64, ok bool) {
	d, ok := errorDetails[struct {
		MaxBytes int64 `json:"max_bytes"`
	}](err, ErrCodeSnapshotTooLarge)
	return d.MaxBytes, ok
}

// RestoreCooldownRetryAfter returns how long to wait after a 429
// RESTORE_COOLDOWN (the wait between restores, also on a release rollback):
// details.retry_after, or the Retry-After header. ok is false for any other
// error; the duration is zero when the API sent none.
func RestoreCooldownRetryAfter(err error) (wait time.Duration, ok bool) {
	d, ok := errorDetails[struct {
		RetryAfter float64 `json:"retry_after"`
	}](err, ErrCodeRestoreCooldown)
	if !ok {
		return 0, false
	}
	if d.RetryAfter > 0 {
		return time.Duration(d.RetryAfter * float64(time.Second)), true
	}
	if apiErr, isAPI := rest.AsAPIError(err); isAPI {
		if header := apiErr.RetryAfter(); header != nil {
			return *header, true
		}
	}
	return 0, true
}

// FileTooLargeMaxBytes returns details.max_bytes of a FILE_TOO_LARGE error
// from the file manager (10 MiB). ok is false for any other error; the limit
// is zero when the API sent none.
func FileTooLargeMaxBytes(err error) (limit int64, ok bool) {
	d, ok := errorDetails[struct {
		MaxBytes int64 `json:"max_bytes"`
	}](err, ErrCodeFileTooLarge)
	return d.MaxBytes, ok
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
