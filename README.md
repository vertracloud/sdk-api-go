# Vertra Cloud Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/vertracloud/sdk-api-go.svg)](https://pkg.go.dev/github.com/vertracloud/sdk-api-go)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Official Go SDK for the [Vertra Cloud](https://vertracloud.app) public API: apps, databases, snapshots, account, workspaces and billing — one typed method per route.

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
err = client.Databases.Reset(ctx, dbID, rest.WithRequestTimeout(2*time.Minute))

// Send a parameter or header the SDK does not model yet
files, err := client.Apps.Files().List(ctx, appID, &vertracloud.ApplicationFileListParams{Path: "/"}, rest.WithQuery("new_param", "1"))
```

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

`UserPlanName` holds a plan name (`UserPlanNamePro`, `UserPlanNameEnterprise12`, ...). `Account.Get` returns `Plan == nil` for an account without a plan, and apps, databases and workspaces expose the owner's plan as `OwnerPlan *UserPlanName` (nil = no plan).

Public payloads, enums, and input types use the `Application` prefix. Input
types end in `Params` for query/options/multipart values and `Body` for JSON
bodies. Resource deletion methods use `Delete`; `Remove` is reserved for
unlinking a relationship such as a workspace member, linked app, favorite,
or custom domain.

### Uploading and downloading

Uploads are streamed from any `io.Reader`; downloads are streamed to you as an `io.ReadCloser`, so large files never sit in memory. Streamed calls have no default timeout — bound them with `ctx` or `rest.WithRequestTimeout`.

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

zip, err := client.Apps.Download(ctx, appID)
if err != nil {
	log.Fatal(err)
}
defer zip.Close()

out, _ := os.Create("backup.zip")
defer out.Close()
_, err = io.Copy(out, zip)
```

`client.Snapshots.Download` works the same way.

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
| Apps | `client.Apps` (+ `.Deploys()`, `.Network()`, `.Envs()`, `.Files()`, `.Releases()`) | 41 |
| Images | `client.Images` | 12 |
| Databases | `client.Databases` (+ `.Credentials()`) | 14 |
| Snapshots | `client.Snapshots` | 5 |
| Account | `client.Account` (+ `.Sessions()`, `.Folders()`, `.Favorites()`) | 10 |
| Workspaces | `client.Workspaces` (+ `.Members()`, `.Roles()`, `.Invites()`, `.ActionRequests()`, `.Apps()`, `.Databases()`, `.Folders()`, `.Favorites()`) | 30 |
| Billing | `client.Billing` (+ `.Orders()`) | 7 |
| Public status | `client.Status` | 1 |

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
