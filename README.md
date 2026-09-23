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

Also available: `rest.WithBaseURL` and `rest.WithUserAgent`. The public status route needs no key:

```go
status, err := vertracloud.New(rest.NewPublicClient()).Status.Get(ctx)
```

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
```

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

## Coverage

| Domain | Service | Routes |
|---|---|---|
| Apps | `client.Apps` (+ `.Deploys()`, `.Network()`, `.Envs()`, `.Files()`) | 36 |
| Databases | `client.Databases` (+ `.Credentials()`) | 13 |
| Snapshots | `client.Snapshots` | 5 |
| Account | `client.Account` (+ `.Sessions()`, `.Folders()`, `.Favorites()`) | 10 |
| Workspaces | `client.Workspaces` (+ `.Members()`, `.Roles()`, `.Invites()`, `.ActionRequests()`, `.Apps()`, `.Databases()`, `.Folders()`, `.Favorites()`) | 30 |
| Billing | `client.Billing` (+ `.Orders()`) | 5 |
| Public status | `client.Status` | 1 |

Dashboard-only features (activity log, notifications, API key management, the database **Data** tab, plan downgrade, creating workspace invites, transferring workspace ownership and approving action requests) are not part of the public API. See [what an API key cannot do](https://docs.vertracloud.app/sdks).

## Versioning

The SDK follows semantic versioning. Until `v1.0.0`, minor releases may contain breaking changes; they are always called out in the release notes.

## Contributing

Issues and pull requests are welcome. Before sending a change, run:

```bash
gofmt -l . && go vet ./... && go test -race ./...
```

## License

[MIT](LICENSE)
