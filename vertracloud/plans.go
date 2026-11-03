package vertracloud

import (
	"context"
	"net/http"

	"github.com/vertracloud/sdk-api-go/rest"
)

// PlanFamily is the family of a plan.
type PlanFamily string

const (
	PlanFamilyStarter    PlanFamily = "starter"
	PlanFamilyPro        PlanFamily = "pro"
	PlanFamilyBusiness   PlanFamily = "business"
	PlanFamilyEnterprise PlanFamily = "enterprise"
)

// PlanCycle is a billing period and its discount over the monthly price.
// Months is 1, 3, 6 or 12.
type PlanCycle struct {
	Months          int     `json:"months"`
	DiscountPercent float64 `json:"discount_percent"`
}

type PlanBlobLimits struct {
	QuotaBytes                   int64 `json:"quota_bytes"`
	MaxRules                     int   `json:"max_rules"`
	MaxCustomDomains             int   `json:"max_custom_domains"`
	ImageTransformationsPerMonth int   `json:"image_transformations_per_month"`
	S3RequestsPer10s             int   `json:"s3_requests_per_10s"`
}

type PlanLimits struct {
	DeploysPerHour       int `json:"deploys_per_hour"`
	RequestsPerMinute    int `json:"requests_per_minute"`
	RequestsPerDay       int `json:"requests_per_day"`
	MaxDomains           int `json:"max_domains"`
	MaxDomainPorts       int `json:"max_domain_ports"`
	ManualBackupsPerDay  int `json:"manual_backups_per_day"`
	WorkspaceMembers     int `json:"workspace_members"`
	WorkspaceRoles       int `json:"workspace_roles"`
	ReleaseVersions      int `json:"release_versions"`
	ReleaseRetentionDays int `json:"release_retention_days"`
	MaxCustomImages      int `json:"max_custom_images"`
	// WebAnalyticsHistoryDays is how many days back Web Analytics can be read.
	WebAnalyticsHistoryDays int            `json:"web_analytics_history_days"`
	Blob                    PlanBlobLimits `json:"blob"`
}

type PlanFeatures struct {
	WebPublished            bool `json:"web_published"`
	DailyBackups            bool `json:"daily_backups"`
	CreateBackup            bool `json:"create_backup"`
	AdvancedMetrics         bool `json:"advanced_metrics"`
	CreateWorkspace         bool `json:"create_workspace"`
	CustomDomain            bool `json:"custom_domain"`
	CustomSubdomain         bool `json:"custom_subdomain"`
	FileManager             bool `json:"file_manager"`
	DatabaseManager         bool `json:"database_manager"`
	AutoDeploy              bool `json:"auto_deploy"`
	Rollback                bool `json:"rollback"`
	AutoRollback            bool `json:"auto_rollback"`
	ZeroDowntime            bool `json:"zero_downtime"`
	DeleteSnapshot          bool `json:"delete_snapshot"`
	PoweredByBadge          bool `json:"powered_by_badge"`
	BlobMetadata            bool `json:"blob_metadata"`
	BlobSharePassword       bool `json:"blob_share_password"`
	BlobUnlimitedUploadRate bool `json:"blob_unlimited_upload_rate"`
	BlobNoCache             bool `json:"blob_no_cache"`
	BlobShortExpiration     bool `json:"blob_short_expiration"`
	// WebAnalyticsPerformance allows GET /v1/apps/:id/analytics/performance.
	WebAnalyticsPerformance bool `json:"web_analytics_performance"`
	// WebAnalyticsRequests allows GET /v1/apps/:id/analytics/requests.
	WebAnalyticsRequests bool `json:"web_analytics_requests"`
}

// CatalogPlan is one plan of the catalog returned by Plans.List.
type CatalogPlan struct {
	Name PlanName `json:"name"`
	// Family is nil for a plan outside the four families.
	Family *PlanFamily `json:"family"`
	// Legacy is true for a plan that is no longer sold.
	Legacy   bool `json:"legacy"`
	MemoryMB int  `json:"memory_mb"`
	// MaxVCPUsPerApp is the highest vCPU count one application can use.
	MaxVCPUsPerApp float64 `json:"max_vcpus_per_app"`
	// Price is the monthly price, in BRL.
	Price          float64         `json:"price"`
	PaymentMethods []OrderProvider `json:"payment_methods"`
	Limits         PlanLimits      `json:"limits"`
	Features       PlanFeatures    `json:"features"`
}

// PlansCatalog is the plans currently for sale and the billing periods with
// their discounts.
type PlansCatalog struct {
	Cycles []PlanCycle   `json:"cycles"`
	Plans  []CatalogPlan `json:"plans"`
}

// PlansService is the public plan catalog.
type PlansService interface {
	// List: GET /v1/plans — no authentication, so it also works with a
	// client built by rest.NewPublicClient.
	List(ctx context.Context, opts ...rest.RequestOpt) (PlansCatalog, error)
}

type plansServiceImpl struct{ rest rest.Client }

func newPlansService(rc rest.Client) PlansService { return &plansServiceImpl{rest: rc} }

func (s *plansServiceImpl) List(ctx context.Context, opts ...rest.RequestOpt) (PlansCatalog, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/plans", nil, nil, "", opts...)
	if err != nil {
		return PlansCatalog{}, err
	}
	return rest.DecodeJSON[PlansCatalog](data)
}
