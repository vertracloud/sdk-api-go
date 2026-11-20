package vertracloud

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// SnapshotJobsService groups the snapshot and restore job routes under
// /v1/users/:id/snapshots/jobs (scope snapshots:read). resourceID is the app
// or database the job acts on (the target of a restore).
type SnapshotJobsService interface {
	// List: GET /v1/users/:id/snapshots/jobs — the jobs of one resource, most
	// recent first. scope is required. params may be nil; params.Limit is 1 to 50
	// (the API answers 400 VALIDATION_ERROR outside it; zero omits it and the
	// API default is 10).
	List(ctx context.Context, resourceID string, scope SnapshotScope, params *SnapshotJobsParams, opts ...rest.RequestOpt) ([]SnapshotJob, error)
	// Get: GET /v1/users/:id/snapshots/jobs/:job_id — one job. scope is
	// optional here: pass SnapshotScope("") to omit it. A job of another
	// resource, another account or that does not exist is 404 SNAPSHOT_NOT_FOUND.
	Get(ctx context.Context, resourceID, jobID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotJob, error)
	// Wait polls Get until the job ends. It returns the job and a nil error
	// when it is ready; a *SnapshotJobError (Status, Reason, Details, Job)
	// when it ends failed or skipped; a *WaitTimeoutError after opts.Timeout
	// (the total wait, default DefaultWaitTimeout, 30 minutes; the job keeps
	// running); and ctx.Err() when ctx ends. opts.OnProgress is called on every
	// poll while the job has not ended. opts may be nil; the limit of each
	// request is rest.WithRequestTimeout in reqOpts. scope is optional, as in Get.
	Wait(ctx context.Context, resourceID, jobID string, scope SnapshotScope, opts *WaitOptions, reqOpts ...rest.RequestOpt) (SnapshotJob, error)
}

func newSnapshotJobsService(rc rest.Client) SnapshotJobsService {
	return &snapshotJobsServiceImpl{rest: guardPaths(rc)}
}

type snapshotJobsServiceImpl struct{ rest rest.Client }

// SnapshotJobsParams filters SnapshotJobsService.List.
type SnapshotJobsParams struct {
	// Kind keeps only jobs of this kind. Empty returns both.
	Kind SnapshotJobKind
	// Limit is the maximum number of jobs: 1 to 50 (API default 10). Zero omits it.
	Limit int
}

func jobsPath(resourceID, suffix string) string {
	return snapshotsPath(resourceID, "/jobs"+suffix)
}

// optionalScopeQuery is scopeQuery that sends nothing for an empty scope.
func optionalScopeQuery(scope SnapshotScope) url.Values {
	if scope == "" {
		return nil
	}
	return scopeQuery(scope)
}

func (s *snapshotJobsServiceImpl) List(ctx context.Context, resourceID string, scope SnapshotScope, params *SnapshotJobsParams, opts ...rest.RequestOpt) ([]SnapshotJob, error) {
	q := scopeQuery(scope)
	if params != nil {
		if params.Kind != "" {
			q.Set("kind", string(params.Kind))
		}
		if params.Limit > 0 {
			q.Set("limit", strconv.Itoa(params.Limit))
		}
	}
	data, err := s.rest.Do(ctx, http.MethodGet, jobsPath(resourceID, ""), q, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]SnapshotJob](data)
}

func (s *snapshotJobsServiceImpl) Get(ctx context.Context, resourceID, jobID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotJob, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, jobsPath(resourceID, "/"+url.PathEscape(jobID)), optionalScopeQuery(scope), nil, "", opts...)
	if err != nil {
		return SnapshotJob{}, err
	}
	return rest.DecodeJSON[SnapshotJob](data)
}

func (s *snapshotJobsServiceImpl) Wait(ctx context.Context, resourceID, jobID string, scope SnapshotScope, opts *WaitOptions, reqOpts ...rest.RequestOpt) (SnapshotJob, error) {
	poll := newPoller(opts, 2*time.Second, DefaultWaitTimeout)
	for {
		job, err := s.Get(ctx, resourceID, jobID, scope, reqOpts...)
		if err != nil {
			return SnapshotJob{}, err
		}
		switch job.Status {
		case SnapshotJobStatusReady:
			return job, nil
		case SnapshotJobStatusFailed, SnapshotJobStatusSkipped:
			return job, newSnapshotJobError(job)
		}
		if opts != nil && opts.OnProgress != nil {
			opts.OnProgress(job)
		}
		again, waitErr := poll.next(ctx)
		if waitErr != nil {
			return job, waitErr
		}
		if !again {
			return job, &WaitTimeoutError{Timeout: poll.timeout, Job: &job}
		}
	}
}
