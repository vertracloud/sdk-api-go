package vertracloud

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
)

const analyticsMetaJSON = `"start":"2026-11-02T00:00:00.000Z","end":"2026-11-03T00:00:00.000Z","hosts":["app.example.com"],"web_published":true,"sampled":false,"stale":false,"generated_at":"2026-11-03T00:01:00.000Z","history_days":7`

func analyticsRequestJSON(id string) string {
	return `{"time":"2026-11-02T12:00:00.000Z","request_id":"` + id + `","host":"app.example.com","path":"/","query":"","method":"GET","protocol":"HTTP/2","status":200,` +
		`"container_status":null,"edge_time_ms":12,"container_time_ms":null,"ip":"203.0.113.7","asn":64500,"asn_name":"Example Net","country":"BR","edge_location":"GRU",` +
		`"referer":"(none)","referer_url":null,"device":"desktop","os":"Linux","browser":"Firefox","bot":"(none)","content_type":"html","cache_status":"hit"}`
}

func TestAnalytics_Traffic_SendsQueryAndDecodes(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`,"interval":"1h","summary":{"requests":10,"visits":4,"bytes":2048,"errors_4xx":1,"errors_5xx":2,"error_rate_4xx":0.1,"error_rate_5xx":0.2,"edge_time_p95_ms":120.5},`+
		`"series":[{"time":"2026-11-02T00:00:00.000Z","requests":10,"visits":4,"bytes":2048,"errors_4xx":1,"errors_5xx":2}],"previous":null,`+
		`"breakdowns":{"country":[{"value":"BR","label":null,"requests":10,"visits":4,"bytes":2048}],"asn":[{"value":"28573","label":"Example Net","requests":3,"visits":1,"bytes":9}]}}`)
	svc := New(fake).Apps.Analytics()
	got, err := svc.Traffic(context.Background(), "app 1", &ApplicationAnalyticsTrafficParams{
		ApplicationAnalyticsRange: ApplicationAnalyticsRange{
			Start:       time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
			Filters:     ApplicationAnalyticsFilters{Status: "5xx", Referer: ApplicationAnalyticsNoValue, Path: "/api"},
			WorkspaceID: "ws_1",
		},
		Compare:    true,
		Breakdowns: []ApplicationAnalyticsDimension{ApplicationAnalyticsDimensionCountry, ApplicationAnalyticsDimensionPath},
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("Traffic: %v", err)
	}
	req := fake.Requests[0]
	if req.Method != http.MethodGet || req.Path != "/v1/apps/app%201/analytics" || req.WorkspaceID != "ws_1" {
		t.Fatalf("request: %#v", req)
	}
	want := map[string]string{"start": "2026-11-02T00:00:00Z", "compare": "true", "breakdowns": "country,path", "status": "5xx", "referer": "(none)", "path": "/api", "limit": "5"}
	for k, v := range want {
		if req.Query.Get(k) != v {
			t.Fatalf("query %s = %q, want %q (%v)", k, req.Query.Get(k), v, req.Query)
		}
	}
	if req.Query.Has("end") || req.Query.Has("country") {
		t.Fatalf("unset params must not be sent: %v", req.Query)
	}
	if !got.WebPublished || got.HistoryDays != 7 || got.Interval != ApplicationAnalyticsInterval1h || got.Hosts[0] != "app.example.com" {
		t.Fatalf("meta: %#v", got)
	}
	if got.Summary == nil || got.Summary.Errors5xx != 2 || got.Summary.EdgeTimeP95Ms != 120.5 || got.Previous != nil {
		t.Fatalf("summary: %#v", got.Summary)
	}
	asn := got.Breakdowns[ApplicationAnalyticsDimensionASN][0]
	if asn.Label == nil || *asn.Label != "Example Net" || got.Breakdowns[ApplicationAnalyticsDimensionCountry][0].Label != nil {
		t.Fatalf("breakdowns: %#v", got.Breakdowns)
	}
}

func TestAnalytics_Errors_And_Performance_Decode(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`,"interval":"5m","include_4xx":true,"summary":null,"series":[],"previous":null,`+
		`"paths":[{"value":"/api","requests":10,"errors":3,"error_rate":0.3}],"statuses":[{"value":"502","label":null,"requests":3,"visits":0,"bytes":0}]}`)
	lat := `{"avg_ms":10,"p50_ms":8,"p75_ms":12,"p90_ms":20,"p95_ms":30,"p99_ms":50,"p999_ms":90}`
	brief := `{"avg_ms":10,"p50_ms":8,"p95_ms":30,"p99_ms":50}`
	fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`,"interval":"1h","summary":{"requests":5,"edge":`+lat+`,"container":null},"series":[{"time":"2026-11-02T00:00:00.000Z","requests":5,"edge":`+brief+`,"container":null}],"previous":null,`+
		`"countries":[{"value":"BR","label":null,"requests":5,"edge":`+brief+`,"container":`+brief+`}],"edge_locations":[],"slow_paths":[],`+
		`"path_issues":[{"path":"/api","requests":5,"errors":1,"error_rate":0.2,"avg_ms":10,"p99_ms":50}]}`)
	svc := New(fake).Apps.Analytics()

	errs, err := svc.Errors(context.Background(), testAppID, &ApplicationAnalyticsErrorsParams{Include4xx: true, Compare: true})
	if err != nil {
		t.Fatalf("Errors: %v", err)
	}
	if fake.Requests[0].Path != "/v1/apps/"+testAppID+"/analytics/errors" || fake.Requests[0].Query.Get("include_4xx") != "true" || fake.Requests[0].Query.Get("compare") != "true" {
		t.Fatalf("request: %#v", fake.Requests[0])
	}
	if !errs.Include4xx || errs.Summary != nil || errs.Paths[0].Errors != 3 || errs.Statuses[0].Value != "502" || errs.Interval != ApplicationAnalyticsInterval5m {
		t.Fatalf("errors: %#v", errs)
	}

	perf, err := svc.Performance(context.Background(), testAppID, &ApplicationAnalyticsPerformanceParams{ApplicationAnalyticsRange: ApplicationAnalyticsRange{Filters: ApplicationAnalyticsFilters{ASN: "28573"}}})
	if err != nil {
		t.Fatalf("Performance: %v", err)
	}
	if fake.Requests[1].Path != "/v1/apps/"+testAppID+"/analytics/performance" || fake.Requests[1].Query.Get("asn") != "28573" {
		t.Fatalf("request: %#v", fake.Requests[1])
	}
	if perf.Summary == nil || perf.Summary.Edge.P999Ms != 90 || perf.Summary.Container != nil || perf.Series[0].Container != nil || perf.Countries[0].Container == nil || perf.PathIssues[0].P99Ms != 50 {
		t.Fatalf("performance: %#v", perf)
	}
}

func TestAnalytics_Requests_SendsCursorOrderAndDecodes(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`,"order":"asc","requests":[`+analyticsRequestJSON("r1")+`],"cursor":"next"}`)
	got, err := New(fake).Apps.Analytics().Requests(context.Background(), testAppID, &ApplicationAnalyticsRequestsParams{Cursor: "abc", Order: ApplicationAnalyticsOrderAsc, Limit: 100})
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}
	q := fake.Requests[0].Query
	if q.Get("cursor") != "abc" || q.Get("order") != "asc" || q.Get("limit") != "100" {
		t.Fatalf("query: %v", q)
	}
	r := got.Requests[0]
	if r.RequestID != "r1" || r.ContainerStatus != nil || r.ContainerTimeMs != nil || r.RefererURL != nil || r.ASN != 64500 || r.Time.Year() != 2026 {
		t.Fatalf("request: %#v", r)
	}
	if got.Cursor == nil || *got.Cursor != "next" || got.Order != ApplicationAnalyticsOrderAsc {
		t.Fatalf("page: %#v", got)
	}
}

func TestAnalytics_RequestsIter_FollowsCursorWithPinnedWindow(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`,"order":"desc","requests":[`+analyticsRequestJSON("r1")+`,`+analyticsRequestJSON("r2")+`],"cursor":"c1"}`)
	fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`,"order":"desc","requests":[`+analyticsRequestJSON("r3")+`],"cursor":null}`)
	it := New(fake).Apps.Analytics().RequestsIter(testAppID, &ApplicationAnalyticsRequestsParams{Limit: 2})
	var ids []string
	for it.Next(context.Background()) {
		ids = append(ids, it.Request().RequestID)
	}
	if it.Err() != nil {
		t.Fatalf("Err: %v", it.Err())
	}
	if strings.Join(ids, ",") != "r1,r2,r3" || len(fake.Requests) != 2 {
		t.Fatalf("ids = %v, requests = %d", ids, len(fake.Requests))
	}
	first, second := fake.Requests[0].Query, fake.Requests[1].Query
	if first.Has("cursor") || second.Get("cursor") != "c1" {
		t.Fatalf("cursors: %v %v", first, second)
	}
	if first.Get("start") == "" || first.Get("start") != second.Get("start") || first.Get("end") != second.Get("end") {
		t.Fatalf("window not pinned: %v %v", first, second)
	}
}

func TestAnalytics_RequestsIter_StopsOnError(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(403, `{"code":"PLAN_DOES_NOT_SUPPORT_WEB_ANALYTICS_REQUESTS","message":"no"}`)
	it := New(fake).Apps.Analytics().RequestsIter(testAppID, nil)
	if it.Next(context.Background()) {
		t.Fatal("Next should be false on error")
	}
	if it.Err() == nil || !strings.Contains(it.Err().Error(), ErrCodePlanDoesNotSupportWebAnalyticsRequests) {
		t.Fatalf("Err: %v", it.Err())
	}
}

func TestAnalytics_ErrorHelpers(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(400, `{"code":"ANALYTICS_RANGE_EXCEEDS_PLAN","message":"x","details":{"history_days":7,"earliest":"2026-10-27T00:00:00.000Z"}}`)
	fake.EnqueueError(503, `{"code":"ANALYTICS_BUSY","message":"x","details":{"retry_after":60}}`)
	svc := New(fake).Apps.Analytics()

	_, rangeErr := svc.Traffic(context.Background(), testAppID, nil)
	d, ok := AnalyticsRangeExceedsPlan(rangeErr)
	if !ok || d.HistoryDays != 7 || d.Earliest.Day() != 27 {
		t.Fatalf("range: %v %#v", ok, d)
	}
	if _, ok := AnalyticsBusyRetryAfter(rangeErr); ok {
		t.Fatal("AnalyticsBusyRetryAfter must not match another code")
	}

	_, busyErr := svc.Traffic(context.Background(), testAppID, nil)
	wait, ok := AnalyticsBusyRetryAfter(busyErr)
	if !ok || wait != time.Minute {
		t.Fatalf("busy: %v %v", ok, wait)
	}
	if _, ok := AnalyticsRangeExceedsPlan(busyErr); ok {
		t.Fatal("AnalyticsRangeExceedsPlan must not match another code")
	}
	if _, ok := AnalyticsRangeExceedsPlan(nil); ok {
		t.Fatal("nil error must not match")
	}
}

func TestAnalytics_CompareStart_SentOnTrafficErrorsAndPerformanceOnly(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	for i := 0; i < 3; i++ {
		fake.EnqueueJSON(200, `{`+analyticsMetaJSON+`}`)
	}
	svc := New(fake).Apps.Analytics()
	cs := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	rng := ApplicationAnalyticsRange{Start: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)}
	_, _ = svc.Traffic(context.Background(), testAppID, &ApplicationAnalyticsTrafficParams{ApplicationAnalyticsRange: rng, CompareStart: cs})
	_, _ = svc.Errors(context.Background(), testAppID, &ApplicationAnalyticsErrorsParams{ApplicationAnalyticsRange: rng, CompareStart: cs})
	_, _ = svc.Performance(context.Background(), testAppID, &ApplicationAnalyticsPerformanceParams{ApplicationAnalyticsRange: rng, CompareStart: cs})
	for i, req := range fake.Requests {
		if got := req.Query.Get("compare_start"); got != "2026-10-26T00:00:00Z" {
			t.Fatalf("request %d compare_start = %q (%v)", i, got, req.Query)
		}
	}
	if len(fake.Requests) != 3 {
		t.Fatalf("requests: %d", len(fake.Requests))
	}
	fake2 := vertratest.NewFakeRestClient(t)
	fake2.EnqueueJSON(200, `{`+analyticsMetaJSON+`}`)
	_, _ = New(fake2).Apps.Analytics().Traffic(context.Background(), testAppID, nil)
	if fake2.Requests[0].Query.Has("compare_start") {
		t.Fatalf("unset CompareStart must not be sent: %v", fake2.Requests[0].Query)
	}
}
