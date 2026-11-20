package vertracloud

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

func TestSnapshots_Restore_ReadsJobIDAndWarning(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_9","warning":"IMAGE_VERSION_NOT_FOUND"}}`))

	res, err := newSnapshotsService(fake).Restore(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.JobID != "job_9" || res.Warning == nil || *res.Warning != SnapshotImageWarningImageVersionNotFound {
		t.Errorf("response = %#v", res)
	}
}

func TestSnapshotJobs_ListAndGet(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	job := `{"id":"job_1","kind":"restore","resource_id":"app_1","status":"failed","reason":"SNAPSHOT_CORRUPTED","created_at":"2026-11-16T10:00:00.000Z","finished_at":"2026-11-16T10:01:00.000Z"}`
	fake.EnqueueJSON(200, `[`+job+`]`)
	fake.EnqueueJSON(200, job)
	svc := newSnapshotJobsService(fake)

	jobs, err := svc.List(context.Background(), testResourceID, SnapshotScopeApplications, &SnapshotJobsParams{Kind: SnapshotJobKindRestore, Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	req := fake.Requests[0]
	if req.Path != "/v1/users/"+testResourceID+"/snapshots/jobs" || req.Query.Get("scope") != "applications" || req.Query.Get("kind") != "restore" || req.Query.Get("limit") != "5" {
		t.Errorf("request = %s %v", req.Path, req.Query)
	}
	if len(jobs) != 1 || jobs[0].Status != SnapshotJobStatusFailed || jobs[0].Reason != ErrCodeSnapshotCorrupted || jobs[0].FinishedAt == nil {
		t.Errorf("jobs = %#v", jobs)
	}

	got, err := svc.Get(context.Background(), testResourceID, "job_1", SnapshotScopeApplications)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fake.Requests[1].Path != "/v1/users/"+testResourceID+"/snapshots/jobs/job_1" || got.Kind != SnapshotJobKindRestore {
		t.Errorf("request = %s, job = %#v", fake.Requests[1].Path, got)
	}
}

func TestSnapshotJobs_Wait_PollsUntilDone(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"queued"}`)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"running"}`)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"ready"}`)

	job, err := newSnapshotJobsService(fake).Wait(context.Background(), testResourceID, "job_1", SnapshotScopeDatabases, &WaitOptions{Interval: time.Millisecond})
	if err != nil || job.Status != SnapshotJobStatusReady || len(fake.Requests) != 3 {
		t.Fatalf("job = %#v, err = %v, calls = %d", job, err, len(fake.Requests))
	}
}

func TestSnapshotJobs_Wait_FailedIsASnapshotJobError(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"failed","reason":"SNAPSHOT_EXCEEDS_STORAGE","details":{"required_bytes":900,"available_bytes":100,"required_ram_mb":512}}`)

	job, err := newSnapshotJobsService(fake).Wait(context.Background(), testResourceID, "job_1", SnapshotScopeApplications, nil)
	var jobErr *SnapshotJobError
	if !errors.As(err, &jobErr) {
		t.Fatalf("err = %v, want *SnapshotJobError", err)
	}
	if jobErr.Status != SnapshotJobStatusFailed || jobErr.Reason != ErrCodeSnapshotExceedsStorage || jobErr.Details == nil || jobErr.Details.RequiredBytes != 900 || jobErr.Job.ID != "job_1" || job.ID != "job_1" {
		t.Errorf("err = %#v", jobErr)
	}
	if d, ok := SnapshotExceedsStorageDetails(err); !ok || d.RequiredBytes != 900 || d.AvailableBytes != 100 || d.RequiredRAMMB != 512 {
		t.Errorf("details = %#v, %v", d, ok)
	}
	if _, ok := SnapshotTooLargeMaxBytes(err); ok {
		t.Error("another reason must not match")
	}
}

func TestSnapshotJobs_Wait_SkippedIsASnapshotJobErrorAndNoReasonFallsBack(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"skipped","reason":"DATABASE_NOT_READY"}`)
	fake.EnqueueJSON(200, `{"id":"job_2","kind":"snapshot","status":"failed"}`)
	svc := newSnapshotJobsService(fake)

	_, err := svc.Wait(context.Background(), testResourceID, "job_1", SnapshotScopeDatabases, nil)
	var jobErr *SnapshotJobError
	if !errors.As(err, &jobErr) || jobErr.Status != SnapshotJobStatusSkipped || jobErr.Reason != ErrCodeDatabaseNotReady {
		t.Fatalf("err = %v", err)
	}
	_, err = svc.Wait(context.Background(), testResourceID, "job_2", SnapshotScopeDatabases, nil)
	if !errors.As(err, &jobErr) || jobErr.Reason != ErrCodeSnapshotFailed {
		t.Fatalf("err = %v", err)
	}
}

func TestSnapshotJobs_Wait_CallsOnProgressWhileNotEnded(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"queued"}`)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"running","progress":{"phase":"copying","percent":40,"done_bytes":4,"total_bytes":10,"eta_seconds":3}}`)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"ready"}`)

	var seen []SnapshotJob
	_, err := newSnapshotJobsService(fake).Wait(context.Background(), testResourceID, "job_1", SnapshotScopeApplications, &WaitOptions{Interval: time.Millisecond, OnProgress: func(job SnapshotJob) { seen = append(seen, job) }})
	if err != nil || len(seen) != 2 || seen[0].Status != SnapshotJobStatusQueued || seen[0].Progress != nil {
		t.Fatalf("seen = %#v, err = %v", seen, err)
	}
	p := seen[1].Progress
	if p == nil || p.Phase != SnapshotJobPhaseCopying || p.Percent == nil || *p.Percent != 40 || p.DoneBytes != 4 || p.TotalBytes != 10 || p.ETASeconds == nil || *p.ETASeconds != 3 {
		t.Errorf("progress = %#v", p)
	}
}

func TestSnapshotJobs_Wait_TimesOutWithTheLastJob(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	for range 10 {
		fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"running"}`)
	}

	_, err := newSnapshotJobsService(fake).Wait(context.Background(), testResourceID, "job_1", SnapshotScopeApplications, &WaitOptions{Interval: 5 * time.Millisecond, Timeout: 20 * time.Millisecond})
	var timedOut *WaitTimeoutError
	if !errors.As(err, &timedOut) || timedOut.Job == nil || timedOut.Job.Status != SnapshotJobStatusRunning || timedOut.Timeout != 20*time.Millisecond {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("a WaitTimeoutError must match context.DeadlineExceeded")
	}
}

func TestSnapshots_CreateAndWait(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(201, []byte(`{"response":{"id":"snap_1","resource_id":"app_1"}}`))
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_3"}}`))
	fake.EnqueueJSON(200, `{"id":"job_3","kind":"snapshot","status":"running"}`)
	fake.EnqueueJSON(200, `{"id":"job_3","kind":"snapshot","status":"ready","snapshot_id":"snap_2"}`)
	fake.EnqueueJSON(200, `[{"id":"snap_9"},{"id":"snap_2","resource_id":"app_1"}]`)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_4"}}`))
	fake.EnqueueJSON(200, `{"id":"job_4","kind":"snapshot","status":"failed","reason":"SNAPSHOT_TOO_LARGE","details":{"max_bytes":10}}`)
	svc := newSnapshotsService(fake)
	fast := &WaitOptions{Interval: time.Millisecond}

	got, err := svc.CreateAndWait(context.Background(), testResourceID, SnapshotScopeApplications, fast)
	if err != nil || got.ID != "snap_1" || len(fake.Requests) != 1 {
		t.Fatalf("201: got = %#v, err = %v, calls = %d", got, err, len(fake.Requests))
	}
	got, err = svc.CreateAndWait(context.Background(), testResourceID, SnapshotScopeApplications, fast)
	if err != nil || got.ID != "snap_2" {
		t.Fatalf("202: got = %#v, err = %v", got, err)
	}
	if last := fake.Requests[len(fake.Requests)-1]; last.Path != "/v1/users/"+testResourceID+"/snapshots" || last.Method != http.MethodGet {
		t.Errorf("last request = %s %s", last.Method, last.Path)
	}
	_, err = svc.CreateAndWait(context.Background(), testResourceID, SnapshotScopeApplications, fast)
	var jobErr *SnapshotJobError
	if !errors.As(err, &jobErr) || jobErr.Reason != ErrCodeSnapshotTooLarge {
		t.Fatalf("failed: err = %v", err)
	}
	if limit, ok := SnapshotTooLargeMaxBytes(err); !ok || limit != 10 {
		t.Errorf("limit = %d, %v", limit, ok)
	}
}

func TestSnapshotJobs_Wait_StopsWithContext(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"running"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newSnapshotJobsService(fake).Wait(ctx, testResourceID, "job_1", SnapshotScopeApplications, &WaitOptions{Interval: time.Hour})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSnapshots_Create_AcceptedIsAResultNotAnError(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_3"}}`))
	fake.EnqueueRaw(201, []byte(`{"response":{"id":"snap_1","resource_id":"app_1"}}`))
	svc := newSnapshotsService(fake)

	res, err := svc.Create(context.Background(), testResourceID, SnapshotScopeApplications)
	if err != nil {
		t.Fatalf("202 must not be an error: %v", err)
	}
	if !res.Accepted() || res.JobID != "job_3" || res.Snapshot.ID != "" {
		t.Errorf("202 result = %#v", res)
	}
	res, err = svc.Create(context.Background(), testResourceID, SnapshotScopeApplications)
	if err != nil || res.Accepted() || res.Snapshot.ID != "snap_1" || res.JobID != "" {
		t.Errorf("201 result = %#v, err = %v", res, err)
	}
}

func TestSnapshots_Create_AcceptedWithoutAJobIDIsAnError(t *testing.T) {
	for _, body := range []string{`{"response":{}}`, `{"response":{"job_id":""}}`} {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueRaw(202, []byte(body))
		_, err := newSnapshotsService(fake).Create(context.Background(), testResourceID, SnapshotScopeApplications)
		var apiErr *rest.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != ErrCodeSnapshotFailed {
			t.Errorf("%s: err = %v, want SNAPSHOT_FAILED", body, err)
		}
	}
}

func TestSnapshotJobs_Get_ScopeIsOptional(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"ready"}`)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"snapshot","status":"ready"}`)
	svc := newSnapshotJobsService(fake)

	if _, err := svc.Get(context.Background(), testResourceID, "job_1", ""); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, ok := fake.Requests[0].Query["scope"]; ok {
		t.Errorf("empty scope must be omitted, query = %v", fake.Requests[0].Query)
	}
	if _, err := svc.Get(context.Background(), testResourceID, "job_1", SnapshotScopeDatabases); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fake.Requests[1].Query.Get("scope") != "databases" {
		t.Errorf("scope = %v", fake.Requests[1].Query)
	}
}

func TestSnapshotJobs_Wait_WithoutScopeOmitsItOnEveryPoll(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"running"}`)
	fake.EnqueueJSON(200, `{"id":"job_1","kind":"restore","status":"ready"}`)
	if _, err := newSnapshotJobsService(fake).Wait(context.Background(), testResourceID, "job_1", "", &WaitOptions{Interval: time.Millisecond}); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	for i, req := range fake.Requests {
		if _, ok := req.Query["scope"]; ok {
			t.Errorf("poll %d sent scope: %v", i, req.Query)
		}
	}
}

func TestSnapshotJobs_ClientWiring(t *testing.T) {
	rc, err := rest.NewClient("test-key")
	if err != nil {
		t.Fatal(err)
	}
	if New(rc).SnapshotJobs == nil {
		t.Fatal("Client.SnapshotJobs is nil")
	}
}

func TestSnapshotErrorDetails(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(400, `{"code":"SNAPSHOT_EXCEEDS_STORAGE","details":{"required_bytes":2000,"available_bytes":1000,"required_ram_mb":512}}`)
	_, err := newSnapshotsService(fake).Restore(context.Background(), testResourceID, testSnapshotID, SnapshotScopeDatabases)
	d, ok := SnapshotExceedsStorageDetails(err)
	if !ok || d.RequiredBytes != 2000 || d.AvailableBytes != 1000 || d.RequiredRAMMB != 512 {
		t.Errorf("details = %#v, %v", d, ok)
	}

	fake.EnqueueError(429, `{"code":"RESTORE_COOLDOWN","details":{"retry_after":90}}`)
	_, err = newSnapshotsService(fake).Restore(context.Background(), testResourceID, testSnapshotID, SnapshotScopeDatabases)
	if wait, ok := RestoreCooldownRetryAfter(err); !ok || wait != 90*time.Second {
		t.Errorf("cooldown = %v, %v", wait, ok)
	}
	if _, ok := RestoreCooldownRetryAfter(nil); ok {
		t.Error("nil must not match")
	}
}

func TestApps_Download_CreatesAManualSnapshotThenDownloadsIt(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(201, []byte(`{"response":{"id":"snap_1","resource_id":"`+testAppID+`"}}`))
	fake.EnqueueRaw(200, []byte("PK-zip"))

	body, err := newAppsService(fake).Download(context.Background(), testAppID, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if string(got) != "PK-zip" || len(fake.Requests) != 2 {
		t.Fatalf("got %q after %d calls", got, len(fake.Requests))
	}
	create, dl := fake.Requests[0], fake.Requests[1]
	if create.Method != http.MethodPost || create.Path != "/v1/users/"+testAppID+"/snapshots" || create.Query.Get("scope") != "applications" {
		t.Errorf("create = %s %s %v", create.Method, create.Path, create.Query)
	}
	if dl.Method != http.MethodGet || dl.Path != "/v1/users/"+testAppID+"/snapshots/snap_1/download" || dl.Query.Get("scope") != "applications" {
		t.Errorf("download = %s %s %v", dl.Method, dl.Path, dl.Query)
	}
}

func TestApps_Download_FollowsTheJobOn202(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_5"}}`))
	fake.EnqueueJSON(200, `{"id":"job_5","kind":"snapshot","status":"running"}`)
	fake.EnqueueJSON(200, `{"id":"job_5","kind":"snapshot","status":"ready","snapshot_id":"snap_5"}`)
	fake.EnqueueJSON(200, `[{"id":"snap_5","resource_id":"`+testAppID+`"}]`)
	fake.EnqueueRaw(200, []byte("PK-zip"))

	body, err := newAppsService(fake).Download(context.Background(), testAppID, &WaitOptions{Interval: time.Millisecond})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	last := fake.Requests[len(fake.Requests)-1]
	if string(got) != "PK-zip" || last.Path != "/v1/users/"+testAppID+"/snapshots/snap_5/download" {
		t.Fatalf("got %q from %s", got, last.Path)
	}
}

func TestApps_Download_ErrorsStopTheFlow(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(403, `{"code":"PLAN_NOT_ALLOWED"}`)
	if _, err := newAppsService(fake).Download(context.Background(), testAppID, nil); err == nil {
		t.Fatal("want the create error")
	}
	if len(fake.Requests) != 1 {
		t.Errorf("calls = %d, want 1", len(fake.Requests))
	}

	fake = vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_6"}}`))
	fake.EnqueueJSON(200, `{"id":"job_6","kind":"snapshot","status":"failed","reason":"SNAPSHOT_TOO_LARGE","details":{"max_bytes":1}}`)
	_, err := newAppsService(fake).Download(context.Background(), testAppID, nil)
	var jobErr *SnapshotJobError
	if !errors.As(err, &jobErr) || jobErr.Reason != ErrCodeSnapshotTooLarge {
		t.Fatalf("err = %v", err)
	}
}

// A zip that takes longer than the client's default timeout to arrive must
// still finish: there is no total deadline on a download unless the caller
// passes rest.WithRequestTimeout or bounds ctx.
func TestApps_Download_SlowBodyOutlivesTheClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"response":{"id":"snap_1","resource_id":"` + testAppID + `"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		for range 5 {
			_, _ = w.Write([]byte("chunk"))
			w.(http.Flusher).Flush()
			time.Sleep(80 * time.Millisecond)
		}
	}))
	defer srv.Close()

	rc, err := rest.NewClient("test-key", rest.WithBaseURL(srv.URL), rest.WithTimeout(100*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	body, err := New(rc).Apps.Download(context.Background(), testAppID, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil || string(got) != "chunkchunkchunkchunkchunk" {
		t.Fatalf("read %q: %v", got, err)
	}
}

func TestCreate_RestoreJobID(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(202, `{"id":"db_1","restore_job_id":"job_7"}`)
	db, err := newDatabasesService(fake).Create(context.Background(), DatabaseCreateBody{Name: "x", SnapshotID: "snap_1"})
	if err != nil || db.RestoreJobID != "job_7" {
		t.Fatalf("db = %#v, err = %v", db, err)
	}

	fake.EnqueueJSON(202, `{"id":"app_1","restore_job_id":"job_8"}`)
	app, err := newAppsService(fake).Create(context.Background(), ApplicationCreateParams{Name: "x", SnapshotID: "snap_1"})
	if err != nil || app.RestoreJobID != "job_8" {
		t.Fatalf("app = %#v, err = %v", app, err)
	}
}

func TestStatus_RestoreJobID(t *testing.T) {
	ctx := context.Background()
	fake := vertratest.NewFakeRestClient(t)

	fake.EnqueueJSON(200, `{"id":"app_1","running":false,"restore_job_id":"job_1"}`)
	app, err := newAppsService(fake).Status(ctx, "app_1")
	if err != nil || app.RestoreJobID == nil || *app.RestoreJobID != "job_1" {
		t.Fatalf("app status = %#v, err = %v", app, err)
	}
	fake.EnqueueJSON(200, `[{"id":"app_1","restore_job_id":"job_2"},{"id":"app_2","restore_job_id":null}]`)
	apps, err := newAppsService(fake).StatusAll(ctx)
	if err != nil || len(apps) != 2 || apps[0].RestoreJobID == nil || *apps[0].RestoreJobID != "job_2" || apps[1].RestoreJobID != nil {
		t.Fatalf("apps status = %#v, err = %v", apps, err)
	}

	fake.EnqueueJSON(200, `{"id":"db_1","running":true,"restore_job_id":null}`)
	db, err := newDatabasesService(fake).Status(ctx, "db_1")
	if err != nil || db.RestoreJobID != nil {
		t.Fatalf("db status = %#v, err = %v", db, err)
	}
	fake.EnqueueJSON(200, `[{"id":"db_1","restore_job_id":"job_3"},{"id":"db_2","restore_job_id":null}]`)
	dbs, err := newDatabasesService(fake).StatusAll(ctx)
	if err != nil || len(dbs) != 2 || dbs[0].RestoreJobID == nil || *dbs[0].RestoreJobID != "job_3" || dbs[1].RestoreJobID != nil {
		t.Fatalf("dbs status = %#v, err = %v", dbs, err)
	}
}

func TestSnapshotJobDetailsAndSnapshotID(t *testing.T) {
	var failed SnapshotJob
	body := `{"id":"j1","kind":"restore","resource_id":"r1","status":"failed","reason":"SNAPSHOT_EXCEEDS_STORAGE","details":{"required_bytes":500,"available_bytes":100,"required_ram_mb":2048},"snapshot_id":"s1","created_at":"2026-01-01T00:00:00Z"}`
	if err := json.Unmarshal([]byte(body), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.Details == nil || failed.Details.RequiredBytes != 500 || failed.Details.AvailableBytes != 100 || failed.Details.RequiredRAMMB != 2048 || failed.SnapshotID != "s1" {
		t.Errorf("job = %#v", failed)
	}
	var tooLarge SnapshotJob
	if err := json.Unmarshal([]byte(`{"id":"j2","kind":"snapshot","resource_id":"r1","status":"failed","reason":"SNAPSHOT_TOO_LARGE","details":{"max_bytes":4096},"created_at":"2026-01-01T00:00:00Z"}`), &tooLarge); err != nil {
		t.Fatal(err)
	}
	if tooLarge.Details == nil || tooLarge.Details.MaxBytes != 4096 {
		t.Errorf("job = %#v", tooLarge)
	}
}
