package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// DatabasesService groups every /v1/databases/* route (14 routes: status,
// metrics, lifecycle, credentials).
//
// Credentials is a nested resource, exposed as a method returning its own
// service interface — same pattern as AppsService.Deploys()/Network() in
// apps.go — built on demand by the implementation.
type DatabasesService interface {
	// StatusAll: GET /v1/databases/status — scope databases:read.
	StatusAll(ctx context.Context, opts ...rest.RequestOpt) ([]DatabaseStatusShort, error)
	// Versions: GET /v1/databases/versions — scope databases:read. The catalog
	// is keyed by engine type; each entry has the recommended line and the
	// lines available for new databases. A database's version cannot be
	// changed after creation; security updates are applied on stop/start.
	Versions(ctx context.Context, opts ...rest.RequestOpt) (DatabaseVersions, error)
	// Get: GET /v1/databases/:id — scope databases:read.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (Database, error)
	// Status: GET /v1/databases/:id/status — scope databases:read.
	Status(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseStatusInfo, error)
	// Metrics: GET /v1/databases/:id/metrics — scope databases:read.
	Metrics(ctx context.Context, id string, opts ...rest.RequestOpt) ([]DatabaseMetric, error)
	// Create: POST /v1/databases — scope databases:write. See
	// DatabaseCreateBody for the WorkspaceID/SnapshotID notes.
	Create(ctx context.Context, params DatabaseCreateBody, opts ...rest.RequestOpt) (Database, error)
	// Update: PUT /v1/databases/:id — scope databases:write.
	Update(ctx context.Context, id string, body DatabaseUpdateBody, opts ...rest.RequestOpt) (DatabaseOperationResponse, error)
	// Start: POST /v1/databases/:id/start — scope databases:write.
	Start(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseOperationResponse, error)
	// Stop: POST /v1/databases/:id/stop — scope databases:write.
	Stop(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseOperationResponse, error)
	// Reset: POST /v1/databases/:id/reset — scope databases:write. Resets
	// the database's data.
	Reset(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseOperationResponse, error)
	// Delete: DELETE /v1/databases/:id — scope databases:delete.
	Delete(ctx context.Context, id string, opts ...rest.RequestOpt) (string, error)

	Credentials() DatabasesCredentialsService
}

func newDatabasesService(rc rest.Client) DatabasesService {
	return &databasesServiceImpl{rest: guardPaths(rc)}
}

type databasesServiceImpl struct{ rest rest.Client }

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// DatabaseType is a wire enum.
type DatabaseType int

const (
	DatabaseTypePostgreSQL DatabaseType = 1
	DatabaseTypeMongoDB    DatabaseType = 2
	DatabaseTypeRedis      DatabaseType = 3
	DatabaseTypeMySQL      DatabaseType = 4
)

// DatabaseStatus is one of "up" | "down".
type DatabaseStatus string

const (
	DatabaseStatusUp   DatabaseStatus = "up"
	DatabaseStatusDown DatabaseStatus = "down"
)

// DatabaseCluster is a wire enum.
type DatabaseCluster int

const (
	DatabaseClusterUSA1 DatabaseCluster = 1
	DatabaseClusterUSA2 DatabaseCluster = 2
	DatabaseClusterUSA3 DatabaseCluster = 3
)

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

// Database is the shape returned by Get and by every mutating
// route that echoes the database back.
type Database struct {
	ID          string          `json:"id"`
	Cluster     DatabaseCluster `json:"cluster"`
	Type        DatabaseType    `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	OwnerID     string          `json:"owner_id"`
	OwnerPlan   *UserPlanName   `json:"owner_plan"`
	Status      DatabaseStatus  `json:"status"`
	RAM         int             `json:"ram"`
	// Host is e.g. "vertra-cloud-<type>-<dbId>.vertraweb.app".
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastSnapshot *time.Time `json:"last_snapshot"`
	OfflineSince *time.Time `json:"offline_since"`
	// Version is the engine major line (e.g. "18", "8.0"); the patch is never
	// exposed. It is not limited to the catalog: older databases may report a
	// line that is no longer offered for new ones.
	Version string `json:"version"`
	// StackID is the stack the database belongs to; nil when it is not in one.
	StackID *string `json:"stack_id,omitempty"`
	// StackServiceKey is the id of the service of the stack's template this database is; nil when the user added it.
	StackServiceKey *string `json:"stack_service_key,omitempty"`
}

// DatabaseVersionCatalog lists the versions of one engine.
type DatabaseVersionCatalog struct {
	// Recommended is used when DatabaseCreateBody.Version is omitted.
	Recommended string `json:"recommended"`
	// Available holds every line that can be chosen for a new database.
	Available []string `json:"available"`
}

// DatabaseVersions is the response of GET /v1/databases/versions, keyed by
// the engine type as a string ("1" PostgreSQL, "2" MongoDB, "3" Redis, "4"
// MySQL).
type DatabaseVersions map[string]DatabaseVersionCatalog

type DatabaseNetwork struct {
	Total string `json:"total"`
	Now   string `json:"now"`
}

// DatabaseStatusInfo is the response of GET /v1/databases/:id/status.
type DatabaseStatusInfo struct {
	ID      string           `json:"id"`
	CPU     string           `json:"cpu"`
	RAM     string           `json:"ram"`
	Status  DatabaseStatus   `json:"status"`
	Running bool             `json:"running"`
	Storage string           `json:"storage"`
	Network *DatabaseNetwork `json:"network"`
	Uptime  int              `json:"uptime"`
}

// DatabaseStatusShort is the response of GET /v1/databases/status.
//
// The endpoint returns one status item per database.
type DatabaseStatusShort struct {
	ID      string `json:"id"`
	CPU     string `json:"cpu"`
	RAM     string `json:"ram"`
	Storage string `json:"storage"`
	Running bool   `json:"running"`
}

type DatabaseOperationResponse struct {
	Status ResourceOperationStatus `json:"status"`
}

// DatabaseCertificate is a client certificate for TLS connections to the
// database.
type DatabaseCertificate struct {
	Crt string `json:"crt"`
	Key string `json:"key"`
	Pem string `json:"pem"`
}

type DatabaseMetric struct {
	CPU     float64   `json:"cpu"`
	RAM     float64   `json:"ram"`
	Storage float64   `json:"storage"`
	Date    time.Time `json:"date"`
	Network []float64 `json:"network"`
}

// DatabaseCertificateReset is the new certificate after a reset, with the
// applications of stacks connected to the database that must restart to read it.
type DatabaseCertificateReset struct {
	DatabaseCertificate
	RestartNotice
}

// DatabasePasswordReset is the new password after a reset, with the
// applications of stacks connected to the database that must restart to read it.
type DatabasePasswordReset struct {
	Password string `json:"password"`
	RestartNotice
}

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

// DatabaseCreateBody is the body of POST /v1/databases.
// Unlike every other Databases route, WorkspaceID travels as a body field
// here, not through rest.WithWorkspaceID/the workspace_id query param.
//
// SnapshotID creates the database with the snapshot's engine and restores
// its volume; when set, Type is optional but must match the snapshot's
// engine if provided (server-side validation: SNAPSHOT_TYPE_MISMATCH). This
// SDK does not duplicate that check.
//
// Version is the engine major line (see Versions). Omitted, the recommended
// one is used; with SnapshotID, leave it empty to keep the snapshot's line
// (an older one fails with SNAPSHOT_VERSION_NEWER). A value outside the
// catalog fails with INVALID_DATABASE_VERSION.
type DatabaseCreateBody struct {
	Name        string            `json:"name"`
	Description *Nullable[string] `json:"description,omitempty"`
	Type        *DatabaseType     `json:"type,omitempty"`
	// RAM is in MB.
	RAM         int    `json:"ram"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	SnapshotID  string `json:"snapshot_id,omitempty"`
	Version     string `json:"version,omitempty"`
}

// DatabaseUpdateBody is the body of PUT /v1/databases/:id. Every field is
// optional.
type DatabaseUpdateBody struct {
	Name        *string           `json:"name,omitempty"`
	Description *Nullable[string] `json:"description,omitempty"`
	RAM         *int              `json:"ram,omitempty"`
}

// ---------------------------------------------------------------------------
// dbPath helper
// ---------------------------------------------------------------------------

// dbPath builds /v1/databases/:id<suffix>, URL-encoding id.
func dbPath(id, suffix string) string {
	return "/v1/databases/" + url.PathEscape(id) + suffix
}

// ---------------------------------------------------------------------------
// DatabasesService — top-level routes
// ---------------------------------------------------------------------------

func (s *databasesServiceImpl) StatusAll(ctx context.Context, opts ...rest.RequestOpt) ([]DatabaseStatusShort, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/databases/status", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]DatabaseStatusShort](data)
}

func (s *databasesServiceImpl) Versions(ctx context.Context, opts ...rest.RequestOpt) (DatabaseVersions, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/databases/versions", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[DatabaseVersions](data)
}

func (s *databasesServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (Database, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, dbPath(id, ""), nil, nil, "", opts...)
	if err != nil {
		return Database{}, err
	}
	return rest.DecodeJSON[Database](data)
}

func (s *databasesServiceImpl) Status(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseStatusInfo, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, dbPath(id, "/status"), nil, nil, "", opts...)
	if err != nil {
		return DatabaseStatusInfo{}, err
	}
	return rest.DecodeJSON[DatabaseStatusInfo](data)
}

func (s *databasesServiceImpl) Metrics(ctx context.Context, id string, opts ...rest.RequestOpt) ([]DatabaseMetric, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, dbPath(id, "/metrics"), nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]DatabaseMetric](data)
}

func (s *databasesServiceImpl) Create(ctx context.Context, params DatabaseCreateBody, opts ...rest.RequestOpt) (Database, error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return Database{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/databases", nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return Database{}, err
	}
	return rest.DecodeJSON[Database](data)
}

func (s *databasesServiceImpl) Update(ctx context.Context, id string, body DatabaseUpdateBody, opts ...rest.RequestOpt) (DatabaseOperationResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return DatabaseOperationResponse{}, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	data, err := s.rest.Do(ctx, http.MethodPut, dbPath(id, ""), nil, bytes.NewReader(payload), "application/json", opts...)
	if err != nil {
		return DatabaseOperationResponse{}, err
	}
	return rest.DecodeJSON[DatabaseOperationResponse](data)
}

func (s *databasesServiceImpl) Start(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseOperationResponse, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, dbPath(id, "/start"), nil, nil, "", opts...)
	if err != nil {
		return DatabaseOperationResponse{}, err
	}
	return rest.DecodeJSON[DatabaseOperationResponse](data)
}

func (s *databasesServiceImpl) Stop(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseOperationResponse, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, dbPath(id, "/stop"), nil, nil, "", opts...)
	if err != nil {
		return DatabaseOperationResponse{}, err
	}
	return rest.DecodeJSON[DatabaseOperationResponse](data)
}

func (s *databasesServiceImpl) Reset(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseOperationResponse, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, dbPath(id, "/reset"), nil, nil, "", opts...)
	if err != nil {
		return DatabaseOperationResponse{}, err
	}
	return rest.DecodeJSON[DatabaseOperationResponse](data)
}

func (s *databasesServiceImpl) Delete(ctx context.Context, id string, opts ...rest.RequestOpt) (string, error) {
	data, err := s.rest.Do(ctx, http.MethodDelete, dbPath(id, ""), nil, nil, "", opts...)
	if err != nil {
		return "", err
	}
	return rest.DecodeJSON[string](data)
}

// ---------------------------------------------------------------------------
// Credentials / Credentials.Certificate / Credentials.Password
// ---------------------------------------------------------------------------

// DatabasesCredentialsService groups the /v1/databases/:id/credentials*
// routes.
type DatabasesCredentialsService interface {
	Certificate() DatabasesCredentialsCertificateService
	Password() DatabasesCredentialsPasswordService
}

type databasesCredentialsServiceImpl struct{ rest rest.Client }

func (s *databasesServiceImpl) Credentials() DatabasesCredentialsService {
	return &databasesCredentialsServiceImpl{rest: s.rest}
}

func (c *databasesCredentialsServiceImpl) Certificate() DatabasesCredentialsCertificateService {
	return &databasesCredentialsCertificateServiceImpl{rest: c.rest}
}

func (c *databasesCredentialsServiceImpl) Password() DatabasesCredentialsPasswordService {
	return &databasesCredentialsPasswordServiceImpl{rest: c.rest}
}

// DatabasesCredentialsCertificateService groups the
// /v1/databases/:id/credentials/certificate* routes.
type DatabasesCredentialsCertificateService interface {
	// Get: GET /v1/databases/:id/credentials/certificate — scope
	// databases:credentials. Returns JSON, not a binary download.
	Get(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseCertificate, error)
	// Reset: POST /v1/databases/:id/credentials/certificate/reset — scope
	// databases:credentials. The answer says which applications of stacks
	// connected to the database must restart to read the new certificate.
	Reset(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseCertificateReset, error)
}

type databasesCredentialsCertificateServiceImpl struct{ rest rest.Client }

func (cert *databasesCredentialsCertificateServiceImpl) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseCertificate, error) {
	data, err := cert.rest.Do(ctx, http.MethodGet, dbPath(id, "/credentials/certificate"), nil, nil, "", opts...)
	if err != nil {
		return DatabaseCertificate{}, err
	}
	return rest.DecodeJSON[DatabaseCertificate](data)
}

func (cert *databasesCredentialsCertificateServiceImpl) Reset(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabaseCertificateReset, error) {
	data, err := cert.rest.Do(ctx, http.MethodPost, dbPath(id, "/credentials/certificate/reset"), nil, nil, "", opts...)
	if err != nil {
		return DatabaseCertificateReset{}, err
	}
	return rest.DecodeJSON[DatabaseCertificateReset](data)
}

// DatabasesCredentialsPasswordService groups the
// /v1/databases/:id/credentials/reset route.
type DatabasesCredentialsPasswordService interface {
	// Reset: POST /v1/databases/:id/credentials/reset — scope
	// databases:credentials. Returns the new password.
	Reset(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabasePasswordReset, error)
}

type databasesCredentialsPasswordServiceImpl struct{ rest rest.Client }

func (pw *databasesCredentialsPasswordServiceImpl) Reset(ctx context.Context, id string, opts ...rest.RequestOpt) (DatabasePasswordReset, error) {
	data, err := pw.rest.Do(ctx, http.MethodPost, dbPath(id, "/credentials/reset"), nil, nil, "", opts...)
	if err != nil {
		return DatabasePasswordReset{}, err
	}
	return rest.DecodeJSON[DatabasePasswordReset](data)
}
