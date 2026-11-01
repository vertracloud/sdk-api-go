package vertracloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

const plansCatalogJSON = `{"grid":"v3","cycles":[{"months":1,"discount_percent":0},{"months":6,"discount_percent":15}],"plans":[` +
	`{"name":"starter-1","family":"starter","legacy":false,"memory_mb":1024,"max_vcpus_per_app":1,"price":19.9,"payment_methods":["pix","card"],` +
	`"limits":{"deploys_per_hour":20,"requests_per_minute":120,"requests_per_day":43200,"max_domains":0,"max_domain_ports":0,"manual_backups_per_day":8,"workspace_members":1,"workspace_roles":0,"release_versions":3,"release_retention_days":7,"max_custom_images":0,` +
	`"blob":{"quota_bytes":10737418240,"max_rules":5,"max_custom_domains":0,"image_transformations_per_month":1000,"s3_requests_per_10s":100}},` +
	`"features":{"web_published":true,"daily_backups":true,"create_backup":true,"advanced_metrics":false,"create_workspace":false,"custom_domain":false,"custom_subdomain":true,"file_manager":true,"database_manager":false,"auto_deploy":true,"rollback":true,"auto_rollback":false,"zero_downtime":false,"delete_snapshot":true,"powered_by_badge":true,"blob_metadata":false,"blob_share_password":false,"blob_unlimited_upload_rate":false,"blob_no_cache":false,"blob_short_expiration":false}},` +
	`{"name":"economy","family":null,"legacy":true,"memory_mb":512,"max_vcpus_per_app":1,"price":9.9,"payment_methods":["pix"],"limits":{"blob":{}},"features":{}}]}`

func TestPlans_List_DecodesCatalog(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, plansCatalogJSON)
	got, err := New(fake).Plans.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(fake.Requests) != 1 || fake.Requests[0].Method != http.MethodGet || fake.Requests[0].Path != "/v1/plans" {
		t.Fatalf("requests: %#v", fake.Requests)
	}
	if got.Grid != PlanGridV3 || len(got.Cycles) != 2 || got.Cycles[1].Months != 6 || got.Cycles[1].DiscountPercent != 15 {
		t.Fatalf("catalog: %#v", got)
	}
	p := got.Plans[0]
	if p.Name != UserPlanNameStarter1 || p.Family == nil || *p.Family != PlanFamilyStarter || p.Legacy || p.MemoryMB != 1024 || p.Price != 19.9 {
		t.Fatalf("plan: %#v", p)
	}
	if p.Limits.RequestsPerMinute != 120 || p.Limits.Blob.QuotaBytes != 10737418240 || !p.Features.AutoDeploy || p.Features.AutoRollback {
		t.Fatalf("limits/features: %#v %#v", p.Limits, p.Features)
	}
	if len(p.PaymentMethods) != 2 || p.PaymentMethods[1] != OrderProviderCard {
		t.Fatalf("payment methods: %#v", p.PaymentMethods)
	}
	if got.Plans[1].Family != nil || !got.Plans[1].Legacy || got.Plans[1].Name != UserPlanNameEconomy {
		t.Fatalf("legacy plan: %#v", got.Plans[1])
	}
}

func TestPlans_List_WorksWithPublicClient(t *testing.T) {
	var auth string
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":` + plansCatalogJSON + `}`))
	}))
	defer srv.Close()

	client := New(rest.NewPublicClient(rest.WithBaseURL(srv.URL)))
	got, err := client.Plans.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if auth != "" {
		t.Errorf("Authorization header sent by the public client")
	}
	if path != "/v1/plans" || len(got.Plans) != 2 {
		t.Errorf("path=%s plans=%d", path, len(got.Plans))
	}
}

func TestUserPlanName_V3Constants(t *testing.T) {
	names := map[UserPlanName]string{
		UserPlanNameStarter1: "starter-1", UserPlanNameStarter2: "starter-2",
		UserPlanNamePro4: "pro-4", UserPlanNamePro6: "pro-6", UserPlanNamePro8: "pro-8",
		UserPlanNameBusiness12: "business-12", UserPlanNameBusiness16: "business-16", UserPlanNameBusiness24: "business-24",
		UserPlanNameEnterprise32: "enterprise-32", UserPlanNameEnterprise512: "enterprise-512",
		UserPlanNameEconomy: "economy", UserPlanNameEnterprise30: "enterprise-30",
	}
	for got, want := range names {
		if string(got) != want {
			t.Errorf("constant = %q, want %q", got, want)
		}
	}
}
