# Vertra Cloud Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/vertracloud/sdk-api-go.svg)](https://pkg.go.dev/github.com/vertracloud/sdk-api-go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Official Go SDK for the [Vertra Cloud](https://vertracloud.app) public API: apps, databases, templates, stacks, snapshots, account, workspaces and billing — one typed method per route.

- Every call takes a `context.Context`.
- Zero dependencies outside the standard library.
- No hidden retries, caching or state: errors come back as they happened.

## Installation

```bash
go get github.com/vertracloud/sdk-api-go
```

Requires Go 1.22+.

## Getting an API key

Sign in to the [dashboard](https://vertracloud.app), open **Settings → API keys** and create a key with only the scopes your code needs (e.g. `apps:read`, `apps:write`). Keep it out of your source code — the examples read it from `VERTRA_API_KEY`.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/vertracloud/sdk-api-go/rest"
	"github.com/vertracloud/sdk-api-go/vertracloud"
)

func main() {
	restClient, err := rest.NewClient(os.Getenv("VERTRA_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	client := vertracloud.New(restClient)
	ctx := context.Background()

	me, err := client.Account.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Hi %s! Plan: %s\n", me.Name, me.Plan.Name)

	for _, app := range me.Applications {
		fmt.Println(app.ID, app.Name, app.Status)
	}
}
```

The SDK has two layers: `rest` is the HTTP transport (auth, timeouts, errors, streaming) and `vertracloud` is the typed API built on top of it. Most programs only touch `rest.NewClient` and then `vertracloud`.

## Documentation

- Guide: [docs.vertracloud.app/sdks](https://docs.vertracloud.app/sdks)
- API reference and scopes: [docs.vertracloud.app/api-reference](https://docs.vertracloud.app/api-reference/introduction)
- Go reference: [pkg.go.dev](https://pkg.go.dev/github.com/vertracloud/sdk-api-go)

## Usage

### Client options

```go
restClient, err := rest.NewClient(apiKey,
	rest.WithTimeout(10*time.Second),  // default 30s per call
	rest.WithHTTPClient(myHTTPClient), // custom transport, proxy, ...
)
```

Also available: `rest.WithBaseURL` and `rest.WithUserAgent`.

The default HTTP client follows up to 10 redirects and drops `Authorization` (and `Cookie`/`Proxy-Authorization`) as soon as a redirect changes scheme, host or port relative to the original request. A client passed to `rest.WithHTTPClient` is used as is, with its own `CheckRedirect`; Go's default policy keeps `Authorization` across a port change or an https to http downgrade, so set `CheckRedirect` on it if it may follow redirects to another origin. The public status route needs no key:

```go
status, err := vertracloud.New(rest.NewPublicClient()).Status.Get(ctx)
```

### Database versions

```go
catalog, err := client.Databases.Versions(ctx) // keyed by engine type as a string
if err != nil { /* ... */ }
dbType := vertracloud.DatabaseTypePostgreSQL
db, err := client.Databases.Create(ctx, vertracloud.DatabaseCreateBody{
	Name: "meu-db", Type: &dbType, RAM: 512,
	Version: catalog["1"].Recommended,
})
fmt.Println(db.Version) // major line, e.g. "18"
```

Omit `Version` to get the recommended one; with `SnapshotID`, leave it empty to keep the snapshot's line (an older one fails with `SNAPSHOT_VERSION_NEWER`, an unknown one with `INVALID_DATABASE_VERSION`). A database's version cannot be changed after creation; security updates are applied when it is stopped and started.

### Per-call options

Every method takes a variadic `...rest.RequestOpt` last:

```go
// Act on a resource that belongs to a workspace
app, err := client.Apps.Get(ctx, appID, rest.WithWorkspaceID(workspaceID))

// Give one slow call more time
_, err = client.Databases.Reset(ctx, dbID, rest.WithRequestTimeout(3*time.Minute))

// Send a parameter or header the SDK does not model yet
files, err := client.Apps.Files().List(ctx, appID, &vertracloud.ApplicationFileListParams{Path: "/"}, rest.WithQuery("new_param", "1"))
```

### Snapshots

Deleting an app or a database, resetting a database and restoring a snapshot run right away and are permanent: they replace or remove the current data. If you may need it again, take a snapshot first with `Snapshots.Create`. Taking a snapshot can take up to 90 s, so `Create` defaults to a 100 s timeout; pass `rest.WithRequestTimeout` to change it.

```go
if _, err := client.Snapshots.CreateAndWait(ctx, dbID, vertracloud.SnapshotScopeDatabases, nil); err != nil {
	return err
}
if _, err := client.Databases.Reset(ctx, dbID); err != nil {
	return err
}
```

`Apps.Delete` returns only an error, `Databases.Delete` returns the database id and `Databases.Reset` returns the operation status.

Snapshots are limited by the plan's quota per account and by an interval per resource (the numbers are in the [knowledge base](https://docs.vertracloud.app/knowledge-base)); requests that come too fast get a plain `429 RATE_LIMIT_EXCEEDED`. Past a snapshot limit the API answers `429 SNAPSHOT_RATE_LIMITED`; read it with `vertracloud.SnapshotRateLimitDetails(err)` (`Scope`, `RetryAfter`, `ResetsAt`). Other snapshot errors are `ErrCodeDatabaseNotReady`, `ErrCodeSnapshotFailed`, `ErrCodeSnapshotTimeout`, `ErrCodeSnapshotTooLarge` (`SnapshotTooLargeMaxBytes`), `ErrCodeRestoreCooldown` (`RestoreCooldownRetryAfter`), `ErrCodeRestoreInProgress`, `ErrCodeSnapshotExceedsStorage`, `ErrCodeSnapshotCorrupted`, `ErrCodeSnapshotImportFailed`, `ErrCodeSnapshotEngineMismatch`, `ErrCodeSnapshotVersionNewer`, `ErrCodeStackRestoreMemberFailed`, `ErrCodeAppBuilding` and `ErrCodeOperationInProgress`. Each snapshot has an `Origin`: `manual` or `automatic`.

The file manager accepts files of up to 10 MiB; a larger one fails with `413 FILE_TOO_LARGE` (`vertracloud.FileTooLargeMaxBytes(err)` gives the limit).

Nested resources are accessor methods: `client.Apps.Deploys()`, `client.Apps.Envs()`, `client.Workspaces.Members()`, `client.Billing.Orders()`, …

### Apps

```go
app, err := client.Apps.Get(ctx, appID)

_, err = client.Apps.Restart(ctx, appID, nil) // nil = normal restart
yes := true
_, err = client.Apps.Restart(ctx, appID, &vertracloud.ApplicationRestartBody{ReinstallDependencies: &yes})

metrics, err := client.Apps.Metrics(ctx, appID, &vertracloud.ApplicationMetricsParams{Range: "24h"})
logs, err := client.Apps.Logs(ctx, appID)

// Releases and rollback
page, err := client.Apps.Releases().List(ctx, appID, vertracloud.ApplicationReleasesListParams{})
preview, err := client.Apps.Releases().Preview(ctx, appID, releaseID) // scope apps:envs
rel, err := client.Apps.Releases().Rollback(ctx, appID, releaseID, vertracloud.ApplicationRollbackBody{}) // scope apps:write

// Deploy behavior (health_check_path: NullableNull clears it)
off := vertracloud.ApplicationZeroDowntimeOff
_, err = client.Apps.UpdateConfig(ctx, appID, vertracloud.ApplicationUpdateConfigBody{
	Deploy: &vertracloud.ApplicationDeploySettingsUpdate{
		ZeroDowntime:    &off,
		HealthCheckPath: vertracloud.NullableValue("/health"),
	},
})
```

`Releases().Preview` returns only the names of the environment variables a rollback would change (`added`, `modified` or `removed`), never their values. To also restore the variables saved with the release, set `RestoreEnvironment` in `ApplicationRollbackBody`; that needs the `apps:envs` scope as well (without it the call fails with 403 `API_KEY_SCOPE_DENIED`). Error codes such as `RELEASE_EXPIRED` or `RESTORE_COOLDOWN` arrive in `APIError.Code` as plain strings.

### Web Analytics

Traffic, errors, performance and the request log of an app's site (scope `apps:read`). Every `params` argument may be `nil`. The window (`Start`, `End`; at most 30 days, within the plan's `Limits.WebAnalyticsHistoryDays`) and the filters (`Filters`: `Country`, `EdgeLocation`, `Host`, `Path` as a prefix, `Method`, `Status` or a class such as `5xx`, `Protocol`, `Device`, `OS`, `Browser`, `Referer`, `ASN`, `IP`, `ContentType`, `CacheStatus`, `Bot`) live in the embedded `ApplicationAnalyticsRange`. A `Value` of a breakdown entry can be sent back as the filter unchanged; `vertracloud.ApplicationAnalyticsNoValue` (`"(none)"`) filters the requests that had no value.

```go
analytics := client.Apps.Analytics()

traffic, err := analytics.Traffic(ctx, appID, &vertracloud.ApplicationAnalyticsTrafficParams{
	ApplicationAnalyticsRange: vertracloud.ApplicationAnalyticsRange{
		Start:   time.Now().Add(-48 * time.Hour),
		Filters: vertracloud.ApplicationAnalyticsFilters{Country: "BR"},
	},
	Compare:    true,
	Breakdowns: []vertracloud.ApplicationAnalyticsDimension{vertracloud.ApplicationAnalyticsDimensionPath},
})
errs, err := analytics.Errors(ctx, appID, &vertracloud.ApplicationAnalyticsErrorsParams{Include4xx: true})
// CompareStart compares with the period of the same length that starts there (implies Compare).
week, err := analytics.Errors(ctx, appID, &vertracloud.ApplicationAnalyticsErrorsParams{
	ApplicationAnalyticsRange: vertracloud.ApplicationAnalyticsRange{Start: time.Now().Add(-24 * time.Hour)},
	CompareStart:              time.Now().Add(-8 * 24 * time.Hour),
})
perf, err := analytics.Performance(ctx, appID, nil) // plans with Features.WebAnalyticsPerformance
page, err := analytics.Requests(ctx, appID, &vertracloud.ApplicationAnalyticsRequestsParams{Limit: 100}) // plans with Features.WebAnalyticsRequests

it := analytics.RequestsIter(appID, nil) // follows the cursor
for it.Next(ctx) {
	r := it.Request()
	fmt.Println(r.RequestID, r.Status, r.EdgeTimeMs)
}
if err := it.Err(); err != nil {
	return err
}
```

An app with no site published answers `WebPublished: false` and empty data. A plan without the route fails with 403 `ErrCodePlanDoesNotSupportWebAnalyticsPerformance` or `ErrCodePlanDoesNotSupportWebAnalyticsRequests`; `vertracloud.AnalyticsRangeExceedsPlan(err)` reads the details of `ErrCodeAnalyticsRangeExceedsPlan` (400) and `vertracloud.AnalyticsBusyRetryAfter(err)` those of `ErrCodeAnalyticsBusy` (503).

### Custom domains

```go
domains, err := client.Apps.Domains().List(ctx, appID)

// Adds the domain and, by default, its www/root companion (not counted toward the plan limit)
added, err := client.Apps.Domains().Add(ctx, appID, vertracloud.ApplicationDomainAddBody{
	Hostname:   "example.com",
	Port:       3000,   // optional, 1-65535, default 80
	PathPrefix: "/api", // optional literal prefix
})

// Change one domain; NullableNull clears path_prefix / redirect_to
_, err = client.Apps.Domains().Update(ctx, appID, domainID, vertracloud.ApplicationDomainUpdateBody{
	RedirectTo:     vertracloud.NullableValue(otherDomainID),
	RedirectStatus: vertracloud.ApplicationDomainRedirectStatusFound,
})
_, err = client.Apps.Domains().Update(ctx, appID, domainID, vertracloud.ApplicationDomainUpdateBody{Primary: true})

records, err := client.Apps.Domains().DNS(ctx, appID, domainID) // records to create at your DNS provider
link, err := client.Apps.Domains().Connect(ctx, appID, domainID) // 404 DOMAIN_CONNECT_UNAVAILABLE when unsupported
err = client.Apps.Domains().Remove(ctx, appID, domainID)        // also removes the companion
```

`Application.Domains` carries the same list. Reads need `apps:read`; add, update, remove and connect need `apps:write`.

### Plans

`PlanName` holds a plan name. There are 22 names (`PlanNameStarter1`, `PlanNameStarter2`, `PlanNamePro4` to `PlanNamePro8`, `PlanNameBusiness12` to `PlanNameBusiness24`, `PlanNameEnterprise32` to `PlanNameEnterprise512`). `Account.Get` returns `Plan == nil` for an account without a plan, and apps, databases and workspaces expose the owner's plan as `OwnerPlan *PlanName` (nil = no plan).

`Plans.List` is public (`GET /v1/plans`): it returns the plans for sale with price, memory, vCPU, limits and features, and the billing periods (1, 3, 6 or 12 months) with their discounts. It works without an API key:

```go
catalog, err := vertracloud.New(rest.NewPublicClient()).Plans.List(ctx)
for _, plan := range catalog.Plans {
	fmt.Println(plan.Name, plan.Price, plan.MemoryMB)
}
```

`Billing.Orders().Quote` (scope `billing:read`) prices a plan and period, with an optional coupon, without creating an order. `Total` is in BRL and is what `Orders().Create` charges:

```go
quote, err := client.Billing.Orders().Quote(ctx, vertracloud.OrderQuoteBody{
	Plan:   vertracloud.PlanNameStarter2,
	Months: 6,
	Coupon: "MYCODE", // optional
})
fmt.Println(quote.Total, quote.PeriodDiscountPercent, quote.LegacyPrice)
```

Public payloads, enums, and input types use the `Application` prefix. Input
types end in `Params` for query/options/multipart values and `Body` for JSON
bodies. Resource deletion methods use `Delete`; `Remove` is reserved for
unlinking a relationship such as a workspace member, linked app, favorite,
or custom domain.

### Uploading and downloading

Uploads are streamed from any `io.Reader`. To download to disk use `DownloadToFile`, the recommended form for large files; `Download` streams the zip to you as an `io.ReadCloser` (no file is held in memory either) when you want to send the bytes somewhere else. Streamed calls have no default timeout — bound them with `ctx` or `rest.WithRequestTimeout`.

```go
f, err := os.Open("app.zip")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

app, err := client.Apps.Create(ctx, vertracloud.ApplicationCreateParams{
	File:     f,
	FileName: "app.zip",
	Name:     "my-app",
})

n, err := client.Apps.DownloadToFile(ctx, appID, "backup.zip", nil) // bytes written
if err != nil {
	log.Fatal(err)
}
```

`Apps.DownloadToFile` (and `Apps.Download`, which returns the stream) takes a manual snapshot of the app (it shows in the snapshot list and counts in the snapshot quota), follows its job when the API answers `202`, and downloads the snapshot zip. The API key needs the `snapshots:write` and `snapshots:read` scopes. The second argument is a `*vertracloud.WaitOptions` for the wait on the snapshot (`nil` = every 2 s, up to 30 minutes, `vertracloud.DefaultWaitTimeout`; a negative `Timeout` removes the limit); it ends with a `*vertracloud.WaitTimeoutError` (with the last `Job`), a `*vertracloud.SnapshotJobError` when the snapshot fails, and `ctx.Err()` when `ctx` ends. The download itself has no total deadline: a big zip keeps coming as long as bytes arrive; only `ctx`, an explicit `rest.WithRequestTimeout` or, for `DownloadToFile`, the stall limit ends it. `DownloadToFile` writes to a temporary file next to the destination (`path` + `.<random>.part`) and renames it to `path` only when the download ends: a failure, a cancelled `ctx` or a stall removes the temporary file, so `path` never holds a partial file (an existing `path` is replaced at the end). A stall is no byte for 60 s (`vertracloud.DefaultDownloadIdleTimeout`, or `rest.WithIdleTimeout`; negative disables) and fails with `vertracloud.ErrDownloadStalled`. To download a snapshot that already exists, use `client.Snapshots.DownloadToFile(ctx, resourceID, snapshotID, scope, path)` (or `Download` for the stream). A body cut before its `Content-Length` fails with `io.ErrUnexpectedEOF` and no file is kept. For a resource of a workspace, pass `rest.WithWorkspaceID(id)`: it is sent on every request of the flow (take the snapshot, follow the job, download), in `Apps.Download*`, `Snapshots.*`, `SnapshotJobs.*` and the stack snapshot waits.

### Restoring a snapshot

A restore runs as a job. `Restore`, `RestoreTo` and `RestoreWith` answer `202` with a `JobID` (and a `Warning` when `RestoreImageVersion` was asked but the image version no longer exists); the resource is not restored until the job ends. Follow it with `SnapshotJobs.Wait` (or `SnapshotJobs.Get` and `SnapshotJobs.List`; `scope` is optional on `Get` and `Wait`: pass `vertracloud.SnapshotScope("")` to omit it, and `List` takes a `Limit` from 1 to 50). `Wait` returns the job and a nil error when it is `ready`; a `*vertracloud.SnapshotJobError` (`Status` `failed` or `skipped`, `Reason`, `Details`, `Job`) when it ends any other way; a `*vertracloud.WaitTimeoutError` (with the last `Job`) after `WaitOptions.Timeout`, 30 minutes by default, while the job keeps running; and `ctx.Err()` when `ctx` ends. `WaitOptions.OnProgress` is called on every poll while the job has not ended: `job.Progress` (`Phase`, `Percent`, `DoneBytes`, `TotalBytes`, `ETASeconds`) is set while it is running.

```go
res, err := client.Snapshots.Restore(ctx, appID, snapshotID, vertracloud.SnapshotScopeApplications)
if err != nil {
	log.Fatal(err)
}
_, err = client.SnapshotJobs.Wait(ctx, appID, res.JobID, vertracloud.SnapshotScopeApplications, &vertracloud.WaitOptions{
	OnProgress: func(job vertracloud.SnapshotJob) {
		if p := job.Progress; p != nil && p.Percent != nil {
			fmt.Printf("%s %.0f%%\n", p.Phase, *p.Percent)
		}
	},
})
var jobErr *vertracloud.SnapshotJobError
if errors.As(err, &jobErr) {
	fmt.Println("restore did not finish:", jobErr.Status, jobErr.Reason) // such as SNAPSHOT_IMPORT_FAILED
}
```

Before the job the API can refuse with `400 SNAPSHOT_EXCEEDS_STORAGE` (`vertracloud.SnapshotExceedsStorageDetails(err)`), `409 RESTORE_IN_PROGRESS`, `409 APP_BUILDING` or `429 RESTORE_COOLDOWN` (the wait between restores; `RestoreCooldownRetryAfter(err)` reads `details.retry_after`). While a restore runs, writes to the resource answer `409 RESTORE_IN_PROGRESS`. `Snapshots.Create` can also answer `202` when the snapshot is not ready in time. That is a result, not an error: `SnapshotCreateResult.Accepted()` is true and `JobID` follows with `SnapshotJobs.Wait` (the `Snapshot` is set when the answer was `201`). Or call `Snapshots.CreateAndWait(ctx, resourceID, scope, nil)`, which creates, follows the job and returns the snapshot (same errors as `Wait`).

Creating an app or a database from a snapshot (`SnapshotID`) answers `202` and returns `RestoreJobID`; the resource exists before the restore ends. If that job fails, an app made from a code snapshot is deleted, while a database and an app on a custom image are kept as they are (empty, or with what you wrote since), so check the job.

### Waiting for the first build

`Apps.Create` returns right away with `Status` `ApplicationStatusBuilding`. Follow the build with `WaitUntilReady`, bounded by `ctx`:

```go
ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
defer cancel()
ready, err := client.Apps.WaitUntilReady(ctx, app.ID, &vertracloud.WaitOptions{Interval: 2 * time.Second})
var down *vertracloud.AppNotReadyError
if errors.As(err, &down) {
	// The build ended "down". down.Code is App.LastBuildError (e.g. INSTALL_OUT_OF_MEMORY),
	// or "APP_DOWN" when the API sent none. Read Apps.Logs for the cause.
}
_ = ready
```

While the app is `building`, actions that build or change it fail with 409 `APP_BUILDING` (`APIError.Code`); deleting is allowed.

### Realtime logs (SSE)

```go
stream, err := client.Apps.Realtime(ctx, appID, nil)
if err != nil {
	log.Fatal(err)
}
defer stream.Close()

for stream.Next(ctx) {
	ev := stream.Event()
	fmt.Println(ev.Event, ev.Data)
}
if err := stream.Err(); err != nil {
	log.Fatal(err)
}
```

Stop the stream by cancelling `ctx`. Pass `rest.WithIdleTimeout(d)` to also stop when no event arrives within `d`, and `&vertracloud.ApplicationRealtimeParams{Since: cursor}` to resume. There is no automatic reconnection.

### Custom Docker images

An image is a build context with a `Dockerfile`, built by the platform. Create and rebuild return right away with `Status` `CustomImageStatusBuilding`; follow the build with `Logs`, or read `Get` until the status leaves `building`.

```go
f, err := os.Open("context.zip")
if err != nil {
	log.Fatal(err)
}
defer f.Close()

img, err := client.Images.Create(ctx, vertracloud.ImageCreateParams{
	Name:        "node-web",
	Description: "Node 22 with our base packages", // optional, up to 128 characters
	File:        f, // or GitHubRepository (GitHubBranch is optional), not both
})
// With GitHubRepository, GitHubBranch is optional (default: the repository's default branch). A repository in your
// GitHub App installations is linked and each push to the branch rebuilds the image; a public repository outside
// them is built once, with no link.

// Read the build log, then run an app from the image
logs, err := client.Images.Logs(ctx, img.ID, nil)
if err != nil {
	log.Fatal(err)
}
defer logs.Close()
for logs.Next(ctx) {
	fmt.Println(logs.Line())
}

app, err := client.Apps.CreateFromImage(ctx, vertracloud.ApplicationCreateFromImageParams{
	ImageID:   img.ID,
	Name:      "web",
	Memory:    512,
	Web:       true,
	Subdomain: "random",
})

// Check a Dockerfile before uploading anything (creates nothing)
res, err := client.Images.Check(ctx, vertracloud.ImageCheckBody{
	Dockerfile: dockerfile,
	Files:      []string{"Dockerfile", "index.js"},
})
// res.Refusals block the build; res.Warnings do not; res.SuggestedPort is the first EXPOSE (or 80)
```

`RebuildFromSource` builds the image again without uploading anything: an image linked to GitHub uses the tip of the linked branch, any other uses the build context of the latest build, including a failed one. It counts toward the hourly deploy limit and returns with `Status` `CustomImageStatusBuilding`. `Rebuild` replaces the context with a new ZIP.

`Update` renames the image and/or changes its description without building anything; apps keep running. Send only what changes. `Name` is a `*string`; `Description` is a `*Nullable[string]` (as on the other PATCH bodies), because a plain `*string` with `omitempty` cannot send `null`: `NullableValue("text")` sets it, `NullableNull[string]()` removes it, and `nil` leaves it untouched. A name already used by another of your images fails with `APIError.Code` `VALIDATION_ERROR`.

```go
name := "node-base"
img, err := client.Images.Update(ctx, img.ID, vertracloud.ImageUpdateBody{
	Name:        &name,
	Description: vertracloud.NullableNull[string](), // remove the description
})
```

`Delete` needs `ConfirmName` equal to the image name. It also deletes every app created from the image, so deleting an image that has apps needs the `apps:delete` scope as well. `Create` fails with `APIError.Code` `DOCKERFILE_REFUSED` when the `Dockerfile` has refusals (for example `RUN --network=host`); run `Check` first to see them, or read the findings from the error with `vertracloud.DockerfileRefusedIssues(err)`.

To act on an image of a workspace, pass `rest.WithWorkspaceID(id)` (sent as `?workspace_id=`) to the image routes, or set `WorkspaceID` in `ImageRebuildParams` and `ImageLogsParams`.

### Templates

A template describes what to create: one application, or a stack with applications, databases and a Blob folder. `Deploy` returns a job right away; follow it with `Deploys().Events` (Server-Sent Events) or read `Deploys().Get`. A job that fails is undone as a whole.

```go
page, err := client.Templates.List(ctx, &vertracloud.TemplateListParams{
	Category: vertracloud.TemplateCategoryBots,
	Sort:     vertracloud.TemplateSortPopular,
})

job, err := client.Templates.Deploy(ctx, "n8n", vertracloud.TemplateDeployBody{
	Name:           "my-n8n",
	Inputs:         map[string]vertracloud.TemplateInputValue{"admin_email": "me@example.com"},
	IdempotencyKey: "deploy-my-n8n-1", // repeating the call returns the same job
})
if err != nil {
	log.Fatal(err)
}

stream, err := client.Templates.Deploys().Events(ctx, job.ID)
if err != nil {
	log.Fatal(err)
}
defer stream.Close()
for stream.Next(ctx) {
	ev := stream.Event()
	fmt.Println(ev.Status, ev.CurrentStep)
}
job, err = client.Templates.Deploys().Get(ctx, job.ID) // outputs are read here, never on the stream
```

Outputs marked `Once` come back on the first read of a finished job and are empty afterwards. The public catalog (`List`, `Get`, `Categories`, `Versions`) also works with `rest.NewPublicClient`.

Private templates: `Create` and `NewVersion` take the manifest and one ZIP per service that runs code (`Sources`); `FromApp` and `FromStack` save what you already run (read `AppDraft` / `StackDraft` first to review the variables). `ShareLinks()` creates, lists and revokes links; `Share()` previews, uses, copies or reports what a link gives. `Updates()` reads and runs the update of an app or stack that came from a template, and detaches it. `Validate` checks a manifest without creating anything; the findings of a refused one are in `vertracloud.TemplateManifestIssues(err)`.

An application that came from an open template stops receiving versions when its code is changed. The calls that change code (`Files().Write`, `Move`, `Delete`, `Upload` and `Deploys().Webhook().Create`) fail with `TEMPLATE_DETACH_CONFIRMATION_REQUIRED` until you set `ConfirmDetach: true`.

### Stacks

A stack groups applications, databases and Blob folders, and keeps the connection variables of each application up to date. Changes that alter variables or certificates return `RestartRequired` and `AppsToRestart`: an application reads them only when it starts, so the restart is yours to ask for.

```go
change, err := client.Stacks.Create(ctx, vertracloud.StackCreateBody{
	Name:    "shop",
	Members: []vertracloud.StackMemberRef{
		{ResourceType: vertracloud.StackMemberTypeApplication, ResourceID: appID},
		{ResourceType: vertracloud.StackMemberTypeDatabase, ResourceID: dbID},
	},
	Bindings: []vertracloud.StackConnectionInput{{
		StackBindingInput: vertracloud.StackBindingInput{AppID: appID},
		DatabaseID:        dbID,
	}},
})
if err == nil && change.RestartRequired {
	fmt.Println("restart:", change.AppsToRestart)
}
```

`Delete` needs `ConfirmName` equal to the stack name; `StackDeleteModeDissolve` removes only the stack and `StackDeleteModeDeleteAll` deletes the members too. Both answer `200` right away with `DeletedApps`, `DeletedDatabases` and `DeletedBlob`, and are permanent: take a snapshot first (`Snapshots().Create`) if you may need the data again. `Members()`, `Bindings()` and `Snapshots()` group the routes of members, connections and whole-stack snapshots; `Connection` returns code to connect an app to what it is linked to. Move a stack or a template to a workspace with `Workspaces.Stacks()` and `Workspaces.Templates()`. Error codes of both areas are the `vertracloud.ErrCode*` constants.

`Snapshots().Create` of a stack answers `202` with the id of the stack snapshot (`ID`) and one snapshot job per member (`Jobs`, databases first), each a regular manual snapshot. The stack snapshot is listed as `pending` (`Status`) and becomes `ready` when the last job ends; a failed one is not listed. Follow the jobs like the restore ones, or let `Stacks.Snapshots().WaitForCreate(ctx, res, nil)` do it (same errors as `WaitForRestore`); `Stacks.Snapshots().CreateAndWait(ctx, id, nil)` creates and waits, returning the accepted answer and the final jobs.

`Snapshots().Restore` of a stack also answers `202`, with one restore job per member (`Jobs`: `ResourceID`, `ResourceType`, `JobID`, and a `Warning` on the jobs whose member lost the recorded image version when `RestoreImageVersion` was asked). The server restores the members one after the other (databases first): the stack is back when every job is `ready`, and after a failed member the jobs behind it end `failed` with reason `ErrCodeStackRestoreMemberFailed`. A refusal before the queue keeps the code and `details` of the member. Follow the jobs with `SnapshotJobs.Wait` (scope by `ResourceType`) or let `Stacks.Snapshots().WaitForRestore(ctx, res, nil)` do it (`RestoreAndWait` restores and waits); it returns the final job of each member when all are `ready`, stops at the first one that ends `failed` or `skipped` with a `*vertracloud.SnapshotJobError` (the jobs so far come back too), and is bounded by one `WaitOptions.Timeout` for all of them and by `ctx`.

### Errors

Every non-2xx response is a `*rest.APIError` carrying the HTTP `Status`, the API `Code` (e.g. `APP_NOT_FOUND`), `Message` and `Details`:

```go
app, err := client.Apps.Get(ctx, appID)
if apiErr, ok := rest.AsAPIError(err); ok {
	switch {
	case apiErr.IsNotFoundError():
		// 404
	case apiErr.IsRateLimitError():
		if wait := apiErr.RetryAfter(); wait != nil {
			time.Sleep(*wait)
		}
	default:
		log.Printf("%s: %s", apiErr.Code, apiErr.Message)
	}
}
```

Other predicates: `IsAuthenticationError` (401), `IsPermissionError` (403 — including a missing key scope) and `IsValidationError` (400/422). The error codes are listed in the [API reference](https://docs.vertracloud.app/api-reference/introduction).

### Testing your code

Each domain field of `vertracloud.Client` (`Apps`, `Databases`, …) is an interface, so you can replace it with a mock. Embed the interface in your mock and override only what you use — new methods may be added in minor releases.

### Blob Storage

`client.Blob` talks to the Blob Storage API (`https://blob.vertracloud.app/v1`; change it with
`vertracloud.WithBlobBaseURL`). It needs an API key with `blob:read` / `blob:write`.

```go
f, _ := os.Open("logo.png")
st, _ := f.Stat()
obj, err := client.Blob.Put(ctx, "images/logo.png", f, st.Size(), &vertracloud.BlobPutOptions{
	Visibility: vertracloud.BlobVisibilityPublic,
	OnProgress: func(loaded, total int64) { fmt.Printf("%d/%d\n", loaded, total) },
})

it := client.Blob.List(ctx, &vertracloud.BlobListOptions{Prefix: "images/"})
for it.Next(ctx) {
	fmt.Println(it.Object().Path)
}
if err := it.Err(); err != nil { /* ... */ }

dl, _ := client.Blob.DownloadURL(ctx, obj.ID, &vertracloud.BlobDownloadOptions{ExpiresIn: 600})
_, _ = client.Blob.DeleteMany(ctx, vertracloud.BlobDeleteManyOptions{Prefix: "tmp/"})
```

`Put` sends files up to 5 MB in one request and larger ones directly to storage, in parts in parallel
(`BlobPutOptions.Concurrency`, default 4, when the reader is an `io.ReaderAt` such as `*os.File`). Cancelling
the context aborts the upload; `Resume(ctx, uploadID, reader, nil)` continues an interrupted one.
Also available: `Get`, `Update`, `UpdateMany`, `Copy`, `Move`, `Delete`, `Stats`, `CreateUploadToken`,
`S3Credentials`, `Shares`, `Rules` and `Domains`. Pass `rest.WithWorkspaceID` to act on a workspace owner's blob.

## Coverage

| Domain | Service | Routes |
|---|---|---|
| Apps | `client.Apps` (+ `.Deploys()`, `.Network()`, `.Envs()`, `.Files()`, `.Releases()`, `.Analytics()`) | 44 |
| Images | `client.Images` | 13 |
| Templates | `client.Templates` (+ `.Deploys()`, `.ShareLinks()`, `.Share()`, `.Updates()`) | 32 |
| Stacks | `client.Stacks` (+ `.Members()`, `.Bindings()`, `.Snapshots()`) | 20 |
| Databases | `client.Databases` (+ `.Credentials()`) | 14 |
| Snapshots | `client.Snapshots` | 5 |
| Snapshot jobs | `client.SnapshotJobs` | 2 |
| Account | `client.Account` (+ `.Sessions()`, `.Folders()`, `.Favorites()`) | 10 |
| Workspaces | `client.Workspaces` (+ `.Members()`, `.Roles()`, `.Invites()`, `.ActionRequests()`, `.Apps()`, `.Databases()`, `.Templates()`, `.Stacks()`, `.Folders()`, `.Favorites()`) | 34 |
| Billing | `client.Billing` (+ `.Orders()`, with `Quote`) | 8 |
| Public status | `client.Status` | 1 |
| Public plan catalog | `client.Plans` | 1 |

Dashboard-only features (activity log, notifications, API key management, the database **Data** tab, plan downgrade, creating workspace invites, paying by card, saved cards, automatic renewal, order receipts and approving action requests) are not part of the public API. See [what an API key cannot do](https://docs.vertracloud.app/sdks).

## Versioning

The SDK follows semantic versioning. Until `v1.0.0`, minor releases may contain breaking changes; they are always called out in the release notes.

## Contributing

Issues and pull requests are welcome. Before sending a change, run:

```bash
gofmt -l . && go vet ./... && go test -race ./...
```

## License

[MIT](LICENSE)
