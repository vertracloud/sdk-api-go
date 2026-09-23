// Package vertracloud is the official Go SDK for the Vertra Cloud public
// API (https://api.vertracloud.app). It is a thin, typed wrapper over the
// sibling rest package: one method per route, and no automatic retries — a
// 429 or a transient network error is always returned to the caller, who
// decides on backoff.
//
//	restClient, err := rest.NewClient(os.Getenv("VERTRA_API_KEY"))
//	if err != nil { ... }
//	client := vertracloud.New(restClient)
//	app, err := client.Apps.Get(ctx, appID)
package vertracloud

import "github.com/vertracloud/sdk-api-go/rest"

// Client is the Vertra Cloud API client, grouped by domain.
//
// Each domain field is an interface (AppsService, DatabasesService, ...) so
// it can be replaced by a mock in your tests. New methods may be added to
// these interfaces in minor releases: embed the interface in your mock
// rather than implementing it from scratch.
type Client struct {
	Apps       AppsService
	Databases  DatabasesService
	Snapshots  SnapshotsService
	Account    AccountService
	Workspaces WorkspacesService
	Billing    BillingService
	Status     StatusService
}

// New builds a Client from restClient (see rest.NewClient and
// rest.NewPublicClient).
func New(restClient rest.Client) *Client {
	return &Client{
		Apps:       newAppsService(restClient),
		Databases:  newDatabasesService(restClient),
		Snapshots:  newSnapshotsService(restClient),
		Account:    newAccountService(restClient),
		Workspaces: newWorkspacesService(restClient),
		Billing:    newBillingService(restClient),
		Status:     newStatusService(restClient),
	}
}
