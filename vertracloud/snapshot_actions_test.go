package vertracloud

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

func TestApps_Delete_SendsNoQueryAndNoBody(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `null`)

	if err := newAppsService(fake).Delete(context.Background(), testAppID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(fake.Requests[0].Query) != 0 || len(fake.Requests[0].Body) != 0 {
		t.Errorf("query = %v, body = %q, want both empty", fake.Requests[0].Query, fake.Requests[0].Body)
	}
}

func TestDatabases_Delete_ReturnsTheID(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `"`+testDBID+`"`)

	id, err := newDatabasesService(fake).Delete(context.Background(), testDBID)
	if err != nil || id != testDBID {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if len(fake.Requests[0].Query) != 0 || len(fake.Requests[0].Body) != 0 {
		t.Errorf("query = %v, body = %q, want both empty", fake.Requests[0].Query, fake.Requests[0].Body)
	}
}

func TestDatabases_Reset_SendsNoBodyAndReturnsTheStatus(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"status":"success"}`)

	res, err := newDatabasesService(fake).Reset(context.Background(), testDBID)
	if err != nil || res.Status != "success" {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	if len(fake.Requests[0].Body) != 0 {
		t.Errorf("body = %q, want empty", fake.Requests[0].Body)
	}
}

func TestSnapshots_RestoreWith_SendsOnlyItsFields(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_1","warning":"IMAGE_VERSION_NOT_FOUND"}}`))

	res, err := newSnapshotsService(fake).RestoreWith(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications, SnapshotRestoreParams{RestoreImageVersion: true})
	if err != nil {
		t.Fatalf("RestoreWith: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(fake.Requests[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if len(body) != 1 || body["restore_image_version"] != true {
		t.Errorf("body = %v", body)
	}
	if res.JobID != "job_1" || res.Warning == nil || *res.Warning != SnapshotImageWarningImageVersionNotFound {
		t.Errorf("response = %#v", res)
	}
}

func TestSnapshots_Restore_SendsNoBodyAndNoLongTimeout(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(202, []byte(`{"response":{"job_id":"job_1"}}`))

	res, err := newSnapshotsService(fake).Restore(context.Background(), testResourceID, testSnapshotID, SnapshotScopeApplications)
	if err != nil || res.JobID != "job_1" {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
	if len(fake.Requests[0].Body) != 0 || fake.Requests[0].Timeout != 0 {
		t.Errorf("body = %q, timeout = %v, want empty and the client default", fake.Requests[0].Body, fake.Requests[0].Timeout)
	}
}

func TestSnapshots_Create_DefaultsToATimeoutAbove90Seconds(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueRaw(201, []byte(`{"response":{"id":"snp_1"}}`))
	if _, err := newSnapshotsService(fake).Create(context.Background(), testResourceID, SnapshotScopeApplications); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := fake.Requests[0].Timeout; got <= 95*time.Second {
		t.Errorf("default timeout = %v, want above 95s", got)
	}

	fake.EnqueueRaw(201, []byte(`{"response":{"id":"snp_2"}}`))
	if _, err := newSnapshotsService(fake).Create(context.Background(), testResourceID, SnapshotScopeApplications, rest.WithRequestTimeout(5*time.Second)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := fake.Requests[1].Timeout; got != 5*time.Second {
		t.Errorf("caller timeout = %v, want 5s", got)
	}
}

func TestSnapshotRateLimitDetails_ReadsScopeAndResetsAt(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(429, `{"code":"SNAPSHOT_RATE_LIMITED","details":{"scope":"account","retry_after":120,"resets_at":"2026-11-17T10:00:00Z"}}`)
	_, err := newSnapshotsService(fake).Create(context.Background(), testResourceID, SnapshotScopeApplications)

	d, ok := SnapshotRateLimitDetails(err)
	if !ok || d.Scope != SnapshotRateLimitScopeAccount || d.RetryAfter != 120 || d.ResetsAt == nil {
		t.Fatalf("details = %#v, ok = %v", d, ok)
	}
}

func TestLimitDetails_ReadByteLimits(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(413, `{"code":"FILE_TOO_LARGE","details":{"max_bytes":10485760}}`)
	fake.EnqueueError(413, `{"code":"SNAPSHOT_TOO_LARGE","details":{"max_bytes":1024}}`)
	_, fileErr := newAppsService(fake).Files().Read(context.Background(), testAppID, ApplicationFileReadParams{Path: "a"})
	_, snapErr := newSnapshotsService(fake).Create(context.Background(), testResourceID, SnapshotScopeApplications)

	if n, ok := FileTooLargeMaxBytes(fileErr); !ok || n != 10<<20 {
		t.Errorf("max_bytes = %d, ok = %v", n, ok)
	}
	if n, ok := SnapshotTooLargeMaxBytes(snapErr); !ok || n != 1024 {
		t.Errorf("max_bytes = %d, ok = %v", n, ok)
	}
}

func TestSnapshotAndPlanFeatures_DecodeNewFields(t *testing.T) {
	var snap ResourceSnapshot
	if err := json.Unmarshal([]byte(`{"id":"1","origin":"automatic"}`), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Origin == nil || *snap.Origin != SnapshotOriginAutomatic {
		t.Errorf("origin = %v", snap.Origin)
	}
	var f PlanFeatures
	if err := json.Unmarshal([]byte(`{"daily_app_backups":true,"daily_database_backups":false}`), &f); err != nil {
		t.Fatal(err)
	}
	if !f.DailyAppBackups || f.DailyDatabaseBackups {
		t.Errorf("features = %#v", f)
	}
}
