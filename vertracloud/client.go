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

import (
	"net/http"

	"github.com/vertracloud/sdk-api-go/rest"
)

// Client is the Vertra Cloud API client, grouped by domain.
//
// Each domain field is an interface (AppsService, DatabasesService, ...) so
// it can be replaced by a mock in your tests. New methods may be added to
// these interfaces in minor releases: embed the interface in your mock
// rather than implementing it from scratch.
type Client struct {
	Apps       AppsService
	Images     ImagesService
	Databases  DatabasesService
	Snapshots  SnapshotsService
	Account    AccountService
	Workspaces WorkspacesService
	Billing    BillingService
	Status     StatusService
	// Blob is the Blob Storage client. It talks to its own host
	// (DefaultBlobBaseURL); see WithBlobBaseURL.
	Blob *BlobClient
}

// Option configures New.
type Option func(*options)

type options struct {
	blobBaseURL string
	blobClient  rest.Client
	blobHTTP    *http.Client
}

// WithBlobBaseURL overrides the Blob Storage base URL (default
// DefaultBlobBaseURL, which already includes /v1). It needs the rest client
// given to New to come from rest.NewClient.
func WithBlobBaseURL(baseURL string) Option {
	return func(o *options) { o.blobBaseURL = baseURL }
}

// WithBlobClient replaces the rest client used for the Blob Storage API
// calls (for example one built with rest.NewClient and an upload token).
func WithBlobClient(c rest.Client) Option {
	return func(o *options) { o.blobClient = c }
}

// WithBlobHTTPClient sets the HTTP client used for the direct PUTs to
// pre-signed upload URLs (no credentials are ever sent there). Default:
// http.DefaultClient.
func WithBlobHTTPClient(hc *http.Client) Option {
	return func(o *options) { o.blobHTTP = hc }
}

// New builds a Client from restClient (see rest.NewClient and
// rest.NewPublicClient).
func New(restClient rest.Client, opts ...Option) *Client {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	return &Client{
		Apps:       newAppsService(restClient),
		Images:     newImagesService(restClient),
		Databases:  newDatabasesService(restClient),
		Snapshots:  newSnapshotsService(restClient),
		Account:    newAccountService(restClient),
		Workspaces: newWorkspacesService(restClient),
		Billing:    newBillingService(restClient),
		Status:     newStatusService(restClient),
		Blob:       newBlobClient(restClient, o),
	}
}
