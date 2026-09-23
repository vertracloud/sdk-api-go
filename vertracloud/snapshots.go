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
// id), not a user id, despite the ":id" path segment. This domain does not
// read rest.WithWorkspaceID — the API does not read workspace_id on any
// snapshots:* route; opts is still accepted
// so callers can override the request timeout.
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
	// with ctx instead.
	Download(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (io.ReadCloser, error)
	// Create: POST /v1/users/:id/snapshots — scope snapshots:write. No
	// request body.
	Create(ctx context.Context, resourceID string, scope SnapshotScope, opts ...rest.RequestOpt) (ResourceSnapshot, error)
	// Restore: POST /v1/users/:id/snapshots/:snapshot_id/restore — scope
	// snapshots:write. No request body; restores into the source resource.
	Restore(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error)
	// RestoreTo is the same route with the optional resource_id body field used
	// to restore into another resource.
	RestoreTo(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, targetResourceID string, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error)
}

func newSnapshotsService(rc rest.Client) SnapshotsService { return &snapshotsServiceImpl{rest: rc} }

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
	ID         string  `json:"id"`
	ResourceID string  `json:"resource_id"`
	AuthorID   *string `json:"author_id"`
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
}

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

type SnapshotRestoreResponse struct {
	Message string `json:"message"`
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

func (s *snapshotsServiceImpl) Create(ctx context.Context, resourceID string, scope SnapshotScope, opts ...rest.RequestOpt) (ResourceSnapshot, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, snapshotsPath(resourceID, ""), scopeQuery(scope), nil, "", opts...)
	if err != nil {
		return ResourceSnapshot{}, err
	}
	return rest.DecodeJSON[ResourceSnapshot](data)
}

func (s *snapshotsServiceImpl) Restore(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error) {
	return s.RestoreTo(ctx, resourceID, snapshotID, scope, "", opts...)
}

func (s *snapshotsServiceImpl) RestoreTo(ctx context.Context, resourceID, snapshotID string, scope SnapshotScope, targetResourceID string, opts ...rest.RequestOpt) (SnapshotRestoreResponse, error) {
	path := snapshotsPath(resourceID, "/"+url.PathEscape(snapshotID)+"/restore")
	var body io.Reader
	contentType := ""
	var err error
	if targetResourceID != "" {
		body, err = snapshotJSONBody(struct {
			ResourceID string `json:"resource_id"`
		}{ResourceID: targetResourceID})
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

func snapshotJSONBody(value any) (*bytes.Reader, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode snapshot request body: %w", err)
	}
	return bytes.NewReader(b), nil
}
