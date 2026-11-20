package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// SnapshotsService groups every /v1/users/:id/snapshots* route (5 routes).
// Resources here are identified by the resource id (an app or a database
// id), not a user id, despite the ":id" path segment. Pass
// rest.WithWorkspaceID for a resource that belongs to a workspace: it is sent
// as workspace_id on every request, including the ones CreateAndWait and
// Apps.Download make (take the snapshot, follow the job, download).
type SnapshotsService interface {
	// ListAll: GET /v1/users/snapshots — scope snapshots:read. Every
	// snapshot the caller can see, grouped by resource.
	ListAll(ctx context.Context, scope SnapshotScope, opts ...rest.RequestOpt) ([]GroupedResourceSnapshots, error)
	// List: GET /v1/users/:id/snapshots — scope snapshots:read. resourceID
	// is the app/database id, not a user id.
	List(ctx context.Context, resourceID string, scope SnapshotScope, opts ...rest.RequestOpt) ([]ResourceSnapshot, error)
	// Download: GET /v1/users/:id/snapshots/:snapshot_id/download — scope
	// snapshots:read. Streams the snapshot's zip; the caller must Close it.
	// No timeout applies unless rest.WithRequestTimeout is passed — bound it
	// with ctx instead. The body is streamed, so memory does not grow with
	// the file; for a file on disk prefer DownloadToFile.
	Download(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (io.ReadCloser, error)
	// DownloadToFile is Download written to path: the bytes go to a
	// temporary file next to path (path + ".<random>.part"), renamed to path
	// only when the download ends, so path never holds a partial file and an
	// existing path is replaced at the end. Memory stays constant whatever the
	// size. Any failure, a cancelled ctx, or a stall (no byte for
	// DefaultDownloadIdleTimeout, or rest.WithIdleTimeout; negative disables)
	// removes the temporary file; a stall returns ErrDownloadStalled. There is
	// no total deadline. Returns the bytes written.
	DownloadToFile(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, path string, opts ...rest.RequestOpt) (int64, error)
	// Create: POST /v1/users/:id/snapshots — scope snapshots:write. No
	// request body. Can take about 90 s, so it defaults to a 100 s timeout
	// (override with rest.WithRequestTimeout). The answer is a result, never an
	// error: 201 fills Snapshot, and 202 (the snapshot is still being taken)
	// fills JobID (see SnapshotCreateResult.Accepted); follow the job with
	// SnapshotJobs.Wait, or use CreateAndWait to do both.
	Create(ctx context.Context, resourceID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotCreateResult, error)
	// CreateAndWait is Create followed, when the API answers 202, by
	// SnapshotJobs.Wait and the read of the snapshot the job produced. It
	// returns the snapshot, or the errors of Wait (*SnapshotJobError,
	// *WaitTimeoutError). opts may be nil; opts.Timeout is the total wait and
	// reqOpts (rest.WithRequestTimeout, ...) go to every request.
	CreateAndWait(ctx context.Context, resourceID string, scope SnapshotScope, opts *WaitOptions, reqOpts ...rest.RequestOpt) (ResourceSnapshot, error)
	// Restore: POST /v1/users/:id/snapshots/:snapshot_id/restore — scope
	// snapshots:write. No request body; restores into the source resource. The
	// API answers 202: the restore runs as a job and the resource is not
	// restored yet. Follow SnapshotRestoreResponse.JobID with SnapshotJobs.Get or
	// SnapshotJobs.Wait. Refusals that come before the job: SNAPSHOT_EXCEEDS_STORAGE
	// (see SnapshotExceedsStorageDetails), RESTORE_IN_PROGRESS, APP_BUILDING,
	// and RESTORE_COOLDOWN (429: the wait between restores, see
	// RestoreCooldownRetryAfter). The restore replaces the current data of the
	// resource: take a snapshot first (Create) if you may need it again.
	Restore(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error)
	// RestoreTo is the same route with the optional resource_id body field used
	// to restore into another resource.
	RestoreTo(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, targetResourceID string, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error)
	// RestoreWith is the same route with the body fields of SnapshotRestoreParams:
	// the optional resource_id, restore_image_version for an app on a custom
	// image (the image version of the snapshot, if it still exists).
	RestoreWith(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, params SnapshotRestoreParams, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error)
}

func newSnapshotsService(rc rest.Client) SnapshotsService {
	return &snapshotsServiceImpl{rest: guardPaths(rc)}
}

type snapshotsServiceImpl struct{ rest rest.Client }

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// ResourceType is what kind of resource a snapshot
// belongs to.
type ResourceType int

const (
	ResourceTypeApplication ResourceType = 1
	ResourceTypeDatabase    ResourceType = 2
)

// SnapshotScope is the "applications"|"databases" query parameter required
// on every Snapshots.* route.
type SnapshotScope string

const (
	SnapshotScopeApplications SnapshotScope = "applications"
	SnapshotScopeDatabases    SnapshotScope = "databases"
)

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

// ResourceSnapshot is one snapshot of one resource.
type ResourceSnapshot struct {
	ID         string `json:"id"`
	ResourceID string `json:"resource_id"`
	// ResourceKind is the wire "resource_type" field (numeric, nullable) —
	// named to avoid colliding with the ResourceType enum type declared
	// above.
	ResourceKind *int      `json:"resource_type"`
	Size         string    `json:"size"`
	Date         time.Time `json:"date"`
	// ResourceName is the resource's name frozen at snapshot time — it does
	// not follow renames and is nil for snapshots taken before this field
	// existed.
	ResourceName *string `json:"resource_name"`
	// Closed is true for a snapshot of an application that came from a closed
	// template. It cannot be downloaded (not even after the application is
	// deleted), only restores into an application of the same template, and an
	// application created from it stays closed; anything else fails with
	// TEMPLATE_CLOSED.
	Closed bool `json:"closed,omitempty"`
	// Origin tells how the snapshot was made. Nil for snapshots the API did
	// not label.
	Origin *SnapshotOrigin `json:"origin,omitempty"`
	// ChangedFiles counts, for an application, files that kept changing while
	// the snapshot was read, so their content may be from different moments.
	// Zero when none.
	ChangedFiles int `json:"changed_files,omitempty"`
}

// SnapshotOrigin is how a snapshot came to exist.
type SnapshotOrigin string

const (
	SnapshotOriginManual    SnapshotOrigin = "manual"
	SnapshotOriginAutomatic SnapshotOrigin = "automatic"
)

// GroupedResourceSnapshots is every snapshot of
// one resource, grouped together (GET /v1/users/snapshots).
type GroupedResourceSnapshots struct {
	ResourceID string `json:"resource_id"`
	// ResourceName is the resource's CURRENT name; falls back to the most
	// recent snapshot's frozen name once the resource itself is deleted, and
	// is nil only when neither exists.
	ResourceName *string            `json:"resource_name"`
	Type         ResourceType       `json:"type"`
	ResourceKind *int               `json:"resource_type"`
	Snapshots    []ResourceSnapshot `json:"snapshots"`
}

// SnapshotRestoreParams are the body fields of SnapshotsService.RestoreWith.
// A zero value sends no body, like Restore.
type SnapshotRestoreParams struct {
	// TargetResourceID restores into another resource of the same kind. Empty restores into the source.
	TargetResourceID string
	// RestoreImageVersion also goes back to the image version the app ran when
	// the snapshot was taken, if it still exists. Only for an app on a custom
	// image; false restores the data only.
	RestoreImageVersion bool
}

// longOperationTimeout covers taking a snapshot, which can take about 90 s.
const longOperationTimeout = 100 * time.Second

// withLongTimeout puts the long default first so a rest.WithRequestTimeout
// given by the caller wins.
func withLongTimeout(opts []rest.RequestOpt) []rest.RequestOpt {
	return append([]rest.RequestOpt{rest.WithRequestTimeout(longOperationTimeout)}, opts...)
}

// SnapshotImageWarning is a warning about a snapshot of an app on a custom image.
type SnapshotImageWarning string

const (
	// SnapshotImageWarningImageVersionNotFound: the image version recorded in
	// the snapshot was removed by retention, so the app stays on its current version.
	SnapshotImageWarningImageVersionNotFound SnapshotImageWarning = "IMAGE_VERSION_NOT_FOUND"
)

// SnapshotRestoreResponse is the answer of a restore (HTTP 202): the restore
// was accepted and runs as a job.
type SnapshotRestoreResponse struct {
	// JobID is the restore job. Follow it with SnapshotJobsService.Get or Wait.
	JobID string `json:"job_id"`
	// Warning is set when RestoreImageVersion was requested but the recorded
	// image version no longer exists: only the data will be restored. Nil otherwise.
	Warning *SnapshotImageWarning `json:"warning,omitempty"`
}

// StackSnapshotJob is the job of one member of a stack snapshot (taking it or
// restoring it). Follow it with SnapshotJobs.Get or Wait using ResourceID and
// the scope that matches ResourceType. Warning is set on a restore job when
// RestoreImageVersion was requested but the recorded image version no longer
// exists for that member: only its data is restored.
type StackSnapshotJob struct {
	ResourceID   string          `json:"resource_id"`
	ResourceType StackMemberType `json:"resource_type"`
	JobID        string          `json:"job_id"`
	// Warning is set only on the jobs of a stack restore. Nil otherwise.
	Warning *SnapshotImageWarning `json:"warning,omitempty"`
}

// StackSnapshotCreateResponse is the answer of taking a stack snapshot (HTTP
// 202): the id of the stack snapshot and one snapshot job per member (databases
// first); nothing is taken yet. Each member is a regular manual snapshot. The
// stack snapshot is listed as "pending" and becomes "ready" when the last job
// ends; a failed one is not listed.
type StackSnapshotCreateResponse struct {
	// ID is the id of the stack snapshot, the one StackSnapshotsService.List returns.
	ID   string             `json:"id"`
	Jobs []StackSnapshotJob `json:"jobs"`
}

// StackSnapshotRestoreResponse is the answer of restoring a stack snapshot
// (HTTP 202): one job per member, databases first; nothing is restored yet. The
// stack is back when every job ends "ready"; after a failed member the jobs
// behind it end "failed" with reason STACK_RESTORE_MEMBER_FAILED.
type StackSnapshotRestoreResponse struct {
	// Jobs are the restore jobs; a job's Warning says its member did not get
	// the recorded image version back.
	Jobs []StackSnapshotJob `json:"jobs"`
}

// SnapshotJobKind is what a job does.
type SnapshotJobKind string

const (
	SnapshotJobKindSnapshot SnapshotJobKind = "snapshot"
	SnapshotJobKindRestore  SnapshotJobKind = "restore"
)

// SnapshotJobStatus is where a job is.
type SnapshotJobStatus string

const (
	SnapshotJobStatusQueued  SnapshotJobStatus = "queued"
	SnapshotJobStatusRunning SnapshotJobStatus = "running"
	SnapshotJobStatusReady   SnapshotJobStatus = "ready"
	// SnapshotJobStatusSkipped: nothing was done: the resource went away
	// meanwhile, or the database was stopped (Reason says which).
	SnapshotJobStatusSkipped SnapshotJobStatus = "skipped"
	SnapshotJobStatusFailed  SnapshotJobStatus = "failed"
)

// Done reports whether the job has ended.
func (s SnapshotJobStatus) Done() bool {
	switch s {
	case SnapshotJobStatusReady, SnapshotJobStatusSkipped, SnapshotJobStatusFailed:
		return true
	}
	return false
}

// SnapshotJobPhase is the phase of a running job.
type SnapshotJobPhase string

const (
	SnapshotJobPhasePreparing SnapshotJobPhase = "preparing"
	SnapshotJobPhaseCopying   SnapshotJobPhase = "copying"
	SnapshotJobPhaseFinishing SnapshotJobPhase = "finishing"
)

// SnapshotJobProgress is the progress of a running job. It is nil while the
// job is queued, once it ends, or when it is not known.
type SnapshotJobProgress struct {
	Phase SnapshotJobPhase `json:"phase"`
	// Percent is the overall progress, 0 to 100. Nil when the total size is not
	// known (database snapshots).
	Percent *float64 `json:"percent,omitempty"`
	// DoneBytes and TotalBytes are zero when not known.
	DoneBytes  int64 `json:"done_bytes,omitempty"`
	TotalBytes int64 `json:"total_bytes,omitempty"`
	// ETASeconds is the estimated seconds left. Nil until there is a stable rate.
	ETASeconds *float64 `json:"eta_seconds,omitempty"`
}

// SnapshotJob is a snapshot or restore job of a resource.
type SnapshotJob struct {
	ID   string          `json:"id"`
	Kind SnapshotJobKind `json:"kind"`
	// ResourceID is the resource the job acts on: the one being backed up, or the target of a restore.
	ResourceID string `json:"resource_id"`
	// Origin is what asked for a snapshot job (the origin the snapshot gets); nil on restore jobs.
	Origin *SnapshotOrigin   `json:"origin,omitempty"`
	Status SnapshotJobStatus `json:"status"`
	// Reason is the error code of a failed or skipped job; empty otherwise.
	Reason string `json:"reason,omitempty"`
	// Details is what the error Reason would carry on a direct response, when it has any. Only set together with Reason.
	Details *SnapshotJobDetails `json:"details,omitempty"`
	// SnapshotID is the snapshot a snapshot job produced (always set once it is ready), or, on a restore job, the
	// snapshot being restored.
	SnapshotID string    `json:"snapshot_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	// StartedAt is nil while the job is queued.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// FinishedAt is nil until the job ends.
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// Progress is set only while the job is running.
	Progress *SnapshotJobProgress `json:"progress,omitempty"`
}

// SnapshotJobError is returned by SnapshotJobs.Wait (and by what waits on it)
// when the job ends failed or skipped. errors.As finds it;
// SnapshotExceedsStorageDetails and SnapshotTooLargeMaxBytes read its Details.
type SnapshotJobError struct {
	// Status is SnapshotJobStatusFailed or SnapshotJobStatusSkipped.
	Status SnapshotJobStatus
	// Reason is the error code of the job; ErrCodeSnapshotFailed when the API
	// sent none.
	Reason string
	// Details is what Reason carries on a direct response, when it has any.
	Details *SnapshotJobDetails
	// Job is the job as last read.
	Job SnapshotJob
}

func (e *SnapshotJobError) Error() string {
	return fmt.Sprintf("%s: the %s job %s ended %s", e.Reason, e.Job.Kind, e.Job.ID, e.Status)
}

func newSnapshotJobError(job SnapshotJob) *SnapshotJobError {
	reason := job.Reason
	if reason == "" {
		reason = ErrCodeSnapshotFailed
	}
	return &SnapshotJobError{Status: job.Status, Reason: reason, Details: job.Details, Job: job}
}

// SnapshotJobDetails is the details of a failed or skipped SnapshotJob: MaxBytes for
// ErrCodeSnapshotTooLarge, the embedded fields for ErrCodeSnapshotExceedsStorage. The fields
// the Reason does not carry are zero.
type SnapshotJobDetails struct {
	// MaxBytes is the largest content the resource's storage holds (SNAPSHOT_TOO_LARGE).
	MaxBytes int64 `json:"max_bytes,omitempty"`
	SnapshotExceedsStorage
}

// ---------------------------------------------------------------------------
// snapshotsPath helper
// ---------------------------------------------------------------------------

// snapshotsPath builds /v1/users/:resourceId/snapshots<suffix>, URL-encoding
// resourceId.
func snapshotsPath(resourceID, suffix string) string {
	return "/v1/users/" + url.PathEscape(resourceID) + "/snapshots" + suffix
}

func scopeQuery(scope SnapshotScope) url.Values {
	q := url.Values{}
	q.Set("scope", string(scope))
	return q
}

// ---------------------------------------------------------------------------
// SnapshotsService — routes
// ---------------------------------------------------------------------------

func (s *snapshotsServiceImpl) ListAll(ctx context.Context, scope SnapshotScope, opts ...rest.RequestOpt) ([]GroupedResourceSnapshots, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/users/snapshots", scopeQuery(scope), nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]GroupedResourceSnapshots](data)
}

func (s *snapshotsServiceImpl) List(ctx context.Context, resourceID string, scope SnapshotScope, opts ...rest.RequestOpt) ([]ResourceSnapshot, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, snapshotsPath(resourceID, ""), scopeQuery(scope), nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]ResourceSnapshot](data)
}

func (s *snapshotsServiceImpl) Download(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (io.ReadCloser, error) {
	path := snapshotsPath(resourceID, "/"+url.PathEscape(snapshotID)+"/download")
	return s.rest.DoStream(ctx, http.MethodGet, path, scopeQuery(scope), opts...)
}

func (s *snapshotsServiceImpl) DownloadToFile(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, path string, opts ...rest.RequestOpt) (int64, error) {
	return downloadToFile(ctx, func(ctx context.Context) (io.ReadCloser, error) {
		return s.Download(ctx, resourceID, snapshotID, scope, opts...)
	}, path, opts)
}

// SnapshotCreateResult is the answer of SnapshotsService.Create: the snapshot
// when it was taken in time (HTTP 201), or the job that is still taking it
// (HTTP 202). Exactly one of Snapshot and JobID is set.
type SnapshotCreateResult struct {
	// Snapshot is the new snapshot. Zero when Accepted.
	Snapshot ResourceSnapshot
	// JobID is the snapshot job to follow with SnapshotJobs.Wait. Empty when
	// the snapshot was taken in time.
	JobID string
}

// Accepted reports that the API answered 202: the snapshot is not ready yet
// and JobID follows it.
func (r SnapshotCreateResult) Accepted() bool { return r.JobID != "" }

func (s *snapshotsServiceImpl) Create(ctx context.Context, resourceID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotCreateResult, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, snapshotsPath(resourceID, ""), scopeQuery(scope), nil, "", withLongTimeout(opts)...)
	if err != nil {
		return SnapshotCreateResult{}, err
	}
	if jobID, ok := acceptedJobID(data); ok {
		return SnapshotCreateResult{JobID: jobID}, nil
	}
	snapshot, err := rest.DecodeJSON[ResourceSnapshot](data)
	if err != nil {
		return SnapshotCreateResult{}, err
	}
	if snapshot.ID == "" {
		// The API answered 202 (a snapshot still being taken) without the job to follow, or 2xx without a snapshot.
		return SnapshotCreateResult{}, &rest.APIError{Status: http.StatusAccepted, Code: ErrCodeSnapshotFailed, Message: "The snapshot is still being taken but no job_id came back"}
	}
	return SnapshotCreateResult{Snapshot: snapshot}, nil
}

func (s *snapshotsServiceImpl) Restore(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error) {
	return s.RestoreTo(ctx, resourceID, snapshotID, scope, "", opts...)
}

func (s *snapshotsServiceImpl) RestoreTo(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, targetResourceID string, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error) {
	return s.RestoreWith(ctx, resourceID, snapshotID, scope, SnapshotRestoreParams{TargetResourceID: targetResourceID}, opts...)
}

func (s *snapshotsServiceImpl) RestoreWith(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, params SnapshotRestoreParams, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error) {
	path := snapshotsPath(resourceID, "/"+url.PathEscape(snapshotID)+"/restore")
	var body io.Reader
	contentType := ""
	var err error
	if params.TargetResourceID != "" || params.RestoreImageVersion {
		body, err = snapshotJSONBody(struct {
			ResourceID          string `json:"resource_id,omitempty"`
			RestoreImageVersion bool   `json:"restore_image_version,omitempty"`
		}{ResourceID: params.TargetResourceID, RestoreImageVersion: params.RestoreImageVersion})
		if err != nil {
			return SnapshotRestoreResponse{}, err
		}
		contentType = "application/json"
	}
	data, err := s.rest.Do(ctx, http.MethodPost, path, scopeQuery(scope), body, contentType, opts...)
	if err != nil {
		return SnapshotRestoreResponse{}, err
	}
	return rest.DecodeJSON[SnapshotRestoreResponse](data)
}

func (s *snapshotsServiceImpl) CreateAndWait(ctx context.Context, resourceID string, scope SnapshotScope, opts *WaitOptions, reqOpts ...rest.RequestOpt) (ResourceSnapshot, error) {
	created, err := s.Create(ctx, resourceID, scope, reqOpts...)
	if err != nil || !created.Accepted() {
		return created.Snapshot, err
	}
	jobID := created.JobID
	job, err := newSnapshotJobsService(s.rest).Wait(ctx, resourceID, jobID, scope, opts, reqOpts...)
	if err != nil {
		return ResourceSnapshot{}, err
	}
	snapshots, err := s.List(ctx, resourceID, scope, reqOpts...)
	if err != nil {
		return ResourceSnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.ID == job.SnapshotID {
			return snapshot, nil
		}
	}
	return ResourceSnapshot{}, &rest.APIError{Status: http.StatusNotFound, Code: ErrCodeSnapshotNotFound, Message: fmt.Sprintf("snapshot job %s is ready but its snapshot is not in the list", jobID)}
}

func snapshotJSONBody(value any) (*bytes.Reader, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode snapshot request body: %w", err)
	}
	return bytes.NewReader(b), nil
}
