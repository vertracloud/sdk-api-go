package vertracloud

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// ApplicationAnalyticsDimension is what the traffic of a site can be broken
// down and filtered by. Every value is both a key of
// ApplicationAnalyticsTraffic.Breakdowns and the name of a filter of the
// analytics routes (see ApplicationAnalyticsFilters).
type ApplicationAnalyticsDimension string

const (
	// ApplicationAnalyticsDimensionCountry is the country of the visitor, ISO 3166-1 alpha-2.
	ApplicationAnalyticsDimensionCountry ApplicationAnalyticsDimension = "country"
	// ApplicationAnalyticsDimensionEdgeLocation is the IATA code of the edge location that answered.
	ApplicationAnalyticsDimensionEdgeLocation ApplicationAnalyticsDimension = "edge_location"
	// ApplicationAnalyticsDimensionHost is the hostname the visitor asked for; only hosts of the app are accepted.
	ApplicationAnalyticsDimensionHost ApplicationAnalyticsDimension = "host"
	// ApplicationAnalyticsDimensionPath is the path of the request, without the query string. As a filter it is a prefix.
	ApplicationAnalyticsDimensionPath ApplicationAnalyticsDimension = "path"
	// ApplicationAnalyticsDimensionMethod is the HTTP method.
	ApplicationAnalyticsDimensionMethod ApplicationAnalyticsDimension = "method"
	// ApplicationAnalyticsDimensionStatus is the HTTP status the visitor received. As a filter it also accepts a class: 2xx, 3xx, 4xx or 5xx.
	ApplicationAnalyticsDimensionStatus ApplicationAnalyticsDimension = "status"
	// ApplicationAnalyticsDimensionProtocol is the HTTP version.
	ApplicationAnalyticsDimensionProtocol ApplicationAnalyticsDimension = "protocol"
	// ApplicationAnalyticsDimensionDevice is the kind of device (desktop, mobile, tablet).
	ApplicationAnalyticsDimensionDevice ApplicationAnalyticsDimension = "device"
	// ApplicationAnalyticsDimensionOS is the operating system of the visitor.
	ApplicationAnalyticsDimensionOS ApplicationAnalyticsDimension = "os"
	// ApplicationAnalyticsDimensionBrowser is the browser of the visitor.
	ApplicationAnalyticsDimensionBrowser ApplicationAnalyticsDimension = "browser"
	// ApplicationAnalyticsDimensionReferer is the host of the page that linked to the request; "(none)" is a direct visit.
	ApplicationAnalyticsDimensionReferer ApplicationAnalyticsDimension = "referer"
	// ApplicationAnalyticsDimensionASN is the autonomous system number of the visitor's network.
	ApplicationAnalyticsDimensionASN ApplicationAnalyticsDimension = "asn"
	// ApplicationAnalyticsDimensionIP is the IP address of the visitor.
	ApplicationAnalyticsDimensionIP ApplicationAnalyticsDimension = "ip"
	// ApplicationAnalyticsDimensionContentType is the kind of content answered (html, json, css, js, png...).
	ApplicationAnalyticsDimensionContentType ApplicationAnalyticsDimension = "content_type"
	// ApplicationAnalyticsDimensionCacheStatus is the cache result of the request (hit, miss, dynamic...).
	ApplicationAnalyticsDimensionCacheStatus ApplicationAnalyticsDimension = "cache_status"
	// ApplicationAnalyticsDimensionBot is the category of a verified bot; "(none)" is not a verified bot.
	ApplicationAnalyticsDimensionBot ApplicationAnalyticsDimension = "bot"
)

// ApplicationAnalyticsNoValue is the value of a dimension for a request that
// has nothing for it (no referer, not a bot, unknown country...). Accepted as a
// filter value.
const ApplicationAnalyticsNoValue = "(none)"

// ApplicationAnalyticsInterval is the size of each point of a time series,
// chosen by the API from the length of the window.
type ApplicationAnalyticsInterval string

const (
	ApplicationAnalyticsInterval1m  ApplicationAnalyticsInterval = "1m"
	ApplicationAnalyticsInterval5m  ApplicationAnalyticsInterval = "5m"
	ApplicationAnalyticsInterval15m ApplicationAnalyticsInterval = "15m"
	ApplicationAnalyticsInterval1h  ApplicationAnalyticsInterval = "1h"
)

// ApplicationAnalyticsStatusClass is a class of HTTP status accepted by the status filter.
type ApplicationAnalyticsStatusClass string

const (
	ApplicationAnalyticsStatusClass2xx ApplicationAnalyticsStatusClass = "2xx"
	ApplicationAnalyticsStatusClass3xx ApplicationAnalyticsStatusClass = "3xx"
	ApplicationAnalyticsStatusClass4xx ApplicationAnalyticsStatusClass = "4xx"
	ApplicationAnalyticsStatusClass5xx ApplicationAnalyticsStatusClass = "5xx"
)

// ApplicationAnalyticsOrder is the order of the request log by time.
type ApplicationAnalyticsOrder string

const (
	ApplicationAnalyticsOrderAsc  ApplicationAnalyticsOrder = "asc"
	ApplicationAnalyticsOrderDesc ApplicationAnalyticsOrder = "desc"
)

// ApplicationAnalyticsFilters are the filters of the analytics routes, one per
// ApplicationAnalyticsDimension; all of them together narrow every part of the
// answer. Each takes one value, as found in a breakdown entry or a request of
// the log (ApplicationAnalyticsNoValue for requests with no value); empty
// omits the filter. Path is a prefix and Status also takes a class ("5xx").
type ApplicationAnalyticsFilters struct {
	Country      string
	EdgeLocation string
	Host         string
	Path         string
	Method       string
	Status       string
	Protocol     string
	Device       string
	OS           string
	Browser      string
	Referer      string
	ASN          string
	IP           string
	ContentType  string
	CacheStatus  string
	Bot          string
}

func (f ApplicationAnalyticsFilters) addTo(q url.Values) {
	addQueryParam(q, "country", f.Country)
	addQueryParam(q, "edge_location", f.EdgeLocation)
	addQueryParam(q, "host", f.Host)
	addQueryParam(q, "path", f.Path)
	addQueryParam(q, "method", f.Method)
	addQueryParam(q, "status", f.Status)
	addQueryParam(q, "protocol", f.Protocol)
	addQueryParam(q, "device", f.Device)
	addQueryParam(q, "os", f.OS)
	addQueryParam(q, "browser", f.Browser)
	addQueryParam(q, "referer", f.Referer)
	addQueryParam(q, "asn", f.ASN)
	addQueryParam(q, "ip", f.IP)
	addQueryParam(q, "content_type", f.ContentType)
	addQueryParam(q, "cache_status", f.CacheStatus)
	addQueryParam(q, "bot", f.Bot)
}

// ApplicationAnalyticsRange is the window and filters shared by the analytics
// routes. The window is at most 30 days long and cannot start earlier than the
// plan's Limits.WebAnalyticsHistoryDays (else 400 ANALYTICS_RANGE_EXCEEDS_PLAN,
// see AnalyticsRangeExceedsPlan).
type ApplicationAnalyticsRange struct {
	// Start of the window; the zero time omits it (default: 24 hours before End).
	Start time.Time
	// End of the window; the zero time omits it (default: now).
	End     time.Time
	Filters ApplicationAnalyticsFilters
	// WorkspaceID is sent the same way as rest.WithWorkspaceID; an explicit
	// rest.WithWorkspaceID option takes precedence.
	WorkspaceID string
}

func (r ApplicationAnalyticsRange) query() url.Values {
	q := url.Values{}
	if !r.Start.IsZero() {
		q.Set("start", r.Start.UTC().Format(time.RFC3339Nano))
	}
	if !r.End.IsZero() {
		q.Set("end", r.End.UTC().Format(time.RFC3339Nano))
	}
	r.Filters.addTo(q)
	return q
}

// ApplicationAnalyticsTrafficParams are the optional parameters of AppsAnalyticsService.Traffic.
type ApplicationAnalyticsTrafficParams struct {
	ApplicationAnalyticsRange
	// Compare also sends the previous period of the same length (Previous).
	Compare bool
	// CompareStart starts the compared period, of the same length as the window
	// (End - Start), instead of the one right before it; the zero time omits it.
	// It implies Compare, and Previous.Start/Previous.End show the period used.
	// A period that ends after the window starts is 400 INVALID_RANGE; one that
	// starts before the plan's history is 400 ANALYTICS_RANGE_EXCEEDS_PLAN.
	CompareStart time.Time
	// Breakdowns limits the dimensions broken down; empty means all of them.
	Breakdowns []ApplicationAnalyticsDimension
	// Limit is the entries per breakdown, 1-25; 0 omits it (API default 10).
	Limit int
}

// ApplicationAnalyticsErrorsParams are the optional parameters of AppsAnalyticsService.Errors.
type ApplicationAnalyticsErrorsParams struct {
	ApplicationAnalyticsRange
	// Compare also sends the previous period of the same length (Previous).
	Compare bool
	// CompareStart starts the compared period, of the same length as the window
	// (End - Start), instead of the one right before it; the zero time omits it.
	// It implies Compare, and Previous.Start/Previous.End show the period used.
	// A period that ends after the window starts is 400 INVALID_RANGE; one that
	// starts before the plan's history is 400 ANALYTICS_RANGE_EXCEEDS_PLAN.
	CompareStart time.Time
	// Include4xx counts 4xx as errors too. Default false: 4xx is noisy, mostly bots and scanners.
	Include4xx bool
	// Limit is the entries in Paths and Statuses, 1-25; 0 omits it (API default 10).
	Limit int
}

// ApplicationAnalyticsPerformanceParams are the optional parameters of AppsAnalyticsService.Performance.
type ApplicationAnalyticsPerformanceParams struct {
	ApplicationAnalyticsRange
	// Compare also sends the previous period of the same length (Previous).
	Compare bool
	// CompareStart starts the compared period, of the same length as the window
	// (End - Start), instead of the one right before it; the zero time omits it.
	// It implies Compare, and Previous.Start/Previous.End show the period used.
	// A period that ends after the window starts is 400 INVALID_RANGE; one that
	// starts before the plan's history is 400 ANALYTICS_RANGE_EXCEEDS_PLAN.
	CompareStart time.Time
	// Limit is the entries in Countries, EdgeLocations, SlowPaths and PathIssues, 1-25; 0 omits it (API default 10).
	Limit int
}

// ApplicationAnalyticsRequestsParams are the optional parameters of
// AppsAnalyticsService.Requests and RequestsIter.
type ApplicationAnalyticsRequestsParams struct {
	ApplicationAnalyticsRange
	// Cursor is the Cursor of the previous page, sent with the same window,
	// filters and order; any other cursor is refused with VALIDATION_ERROR.
	Cursor string
	// Order is by time; empty omits it (API default desc, newest first).
	Order ApplicationAnalyticsOrder
	// Limit is 1-100; 0 omits it (API default 50).
	Limit int
}

// ApplicationAnalyticsMeta carries the fields every analytics answer has. An
// app with no site published answers WebPublished false, no Hosts and empty
// data, never an error.
type ApplicationAnalyticsMeta struct {
	// Start and End are the window actually read; they may be rounded from the one asked for.
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Hosts are the hostnames of the app that were read: its subdomain and its active domains.
	Hosts        []string `json:"hosts"`
	WebPublished bool     `json:"web_published"`
	// Sampled is true when traffic is high enough that the numbers are estimated from a sample.
	Sampled bool `json:"sampled"`
	// Stale is true when the latest data could not be read and this is an earlier answer; see GeneratedAt.
	Stale       bool      `json:"stale"`
	GeneratedAt time.Time `json:"generated_at"`
	// HistoryDays is how many days back the plan of the app's owner can read.
	HistoryDays int `json:"history_days"`
}

// ApplicationAnalyticsSeriesMeta is the meta of the answers that carry time series.
type ApplicationAnalyticsSeriesMeta struct {
	ApplicationAnalyticsMeta
	Interval ApplicationAnalyticsInterval `json:"interval"`
}

// ApplicationAnalyticsBreakdownEntry is one entry of a breakdown, ordered by Requests (highest first).
type ApplicationAnalyticsBreakdownEntry struct {
	// Value is the value of the dimension; send it as the filter of the same name.
	Value string `json:"value"`
	// Label is a readable name when Value is a code (the network name of an asn); nil otherwise.
	Label    *string `json:"label"`
	Requests int64   `json:"requests"`
	Visits   int64   `json:"visits"`
	// Bytes sent to visitors.
	Bytes int64 `json:"bytes"`
}

// ApplicationAnalyticsTrafficSummary holds the totals of the traffic of a period.
type ApplicationAnalyticsTrafficSummary struct {
	Requests int64 `json:"requests"`
	// Visits are the requests that start a visit (a page opened directly or from another site).
	Visits    int64 `json:"visits"`
	Bytes     int64 `json:"bytes"`
	Errors4xx int64 `json:"errors_4xx"`
	Errors5xx int64 `json:"errors_5xx"`
	// ErrorRate4xx and ErrorRate5xx are errors / requests, from 0 to 1.
	ErrorRate4xx float64 `json:"error_rate_4xx"`
	ErrorRate5xx float64 `json:"error_rate_5xx"`
	// EdgeTimeP95Ms is the 95th percentile of the edge time, in milliseconds.
	EdgeTimeP95Ms float64 `json:"edge_time_p95_ms"`
}

// ApplicationAnalyticsTrafficPoint is one point of the traffic series; Time is its start.
type ApplicationAnalyticsTrafficPoint struct {
	Time      time.Time `json:"time"`
	Requests  int64     `json:"requests"`
	Visits    int64     `json:"visits"`
	Bytes     int64     `json:"bytes"`
	Errors4xx int64     `json:"errors_4xx"`
	Errors5xx int64     `json:"errors_5xx"`
}

// ApplicationAnalyticsTrafficPeriod is the previous period (same length, right
// before the window), sent when Compare is true. Series has the same Interval
// and number of points as the current one, in the same order.
type ApplicationAnalyticsTrafficPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	// Summary is nil when there was no request in the period.
	Summary *ApplicationAnalyticsTrafficSummary `json:"summary"`
	Series  []ApplicationAnalyticsTrafficPoint  `json:"series"`
}

// ApplicationAnalyticsTraffic is the answer of GET /v1/apps/:id/analytics.
type ApplicationAnalyticsTraffic struct {
	ApplicationAnalyticsSeriesMeta
	// Summary is nil when there was no request in the window.
	Summary *ApplicationAnalyticsTrafficSummary `json:"summary"`
	Series  []ApplicationAnalyticsTrafficPoint  `json:"series"`
	// Previous is nil unless Compare was set.
	Previous *ApplicationAnalyticsTrafficPeriod `json:"previous"`
	// Breakdowns holds the top entries per dimension: only the dimensions asked for, or all of them.
	Breakdowns map[ApplicationAnalyticsDimension][]ApplicationAnalyticsBreakdownEntry `json:"breakdowns"`
}

// ApplicationAnalyticsErrorsSummary holds the totals of the errors of a period.
// Errors and ErrorRate count 4xx only with Include4xx.
type ApplicationAnalyticsErrorsSummary struct {
	Requests int64 `json:"requests"`
	// Errors is Errors5xx, plus Errors4xx with Include4xx.
	Errors int64 `json:"errors"`
	// ErrorRate is Errors / Requests, from 0 to 1.
	ErrorRate float64 `json:"error_rate"`
	Errors4xx int64   `json:"errors_4xx"`
	Errors5xx int64   `json:"errors_5xx"`
}

// ApplicationAnalyticsErrorsPoint is one point of the errors series; Time is its start.
type ApplicationAnalyticsErrorsPoint struct {
	Time      time.Time `json:"time"`
	Requests  int64     `json:"requests"`
	Errors4xx int64     `json:"errors_4xx"`
	Errors5xx int64     `json:"errors_5xx"`
}

// ApplicationAnalyticsErrorsPeriod is the previous period of an errors answer.
type ApplicationAnalyticsErrorsPeriod struct {
	Start   time.Time                          `json:"start"`
	End     time.Time                          `json:"end"`
	Summary *ApplicationAnalyticsErrorsSummary `json:"summary"`
	Series  []ApplicationAnalyticsErrorsPoint  `json:"series"`
}

// ApplicationAnalyticsErrorEntry is a value of a dimension with its errors.
type ApplicationAnalyticsErrorEntry struct {
	Value    string `json:"value"`
	Requests int64  `json:"requests"`
	// Errors counts 4xx only with Include4xx.
	Errors    int64   `json:"errors"`
	ErrorRate float64 `json:"error_rate"`
}

// ApplicationAnalyticsErrors is the answer of GET /v1/apps/:id/analytics/errors.
type ApplicationAnalyticsErrors struct {
	ApplicationAnalyticsSeriesMeta
	// Include4xx is whether 4xx counts as an error in Errors, ErrorRate, Paths and Statuses.
	Include4xx bool `json:"include_4xx"`
	// Summary is nil when there was no request in the window.
	Summary *ApplicationAnalyticsErrorsSummary `json:"summary"`
	Series  []ApplicationAnalyticsErrorsPoint  `json:"series"`
	// Previous is nil unless Compare was set.
	Previous *ApplicationAnalyticsErrorsPeriod `json:"previous"`
	// Paths are the paths with the most errors (highest Errors first); Value is the path.
	Paths []ApplicationAnalyticsErrorEntry `json:"paths"`
	// Statuses is how the errors spread across status codes (highest Requests first); Value is the status, such as "502".
	Statuses []ApplicationAnalyticsBreakdownEntry `json:"statuses"`
}

// ApplicationAnalyticsLatency is the average and percentiles of a time, in milliseconds.
type ApplicationAnalyticsLatency struct {
	AvgMs  float64 `json:"avg_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P75Ms  float64 `json:"p75_ms"`
	P90Ms  float64 `json:"p90_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	P999Ms float64 `json:"p999_ms"`
}

// ApplicationAnalyticsLatencyBrief is the average and the main percentiles of a time, in milliseconds.
type ApplicationAnalyticsLatencyBrief struct {
	AvgMs float64 `json:"avg_ms"`
	P50Ms float64 `json:"p50_ms"`
	P95Ms float64 `json:"p95_ms"`
	P99Ms float64 `json:"p99_ms"`
}

// ApplicationAnalyticsPerformanceSummary holds the times of a period. Edge is
// the whole time the visitor waited for the first byte; Container is the part
// your app took, nil when the request did not reach the app.
type ApplicationAnalyticsPerformanceSummary struct {
	Requests  int64                        `json:"requests"`
	Edge      ApplicationAnalyticsLatency  `json:"edge"`
	Container *ApplicationAnalyticsLatency `json:"container"`
}

// ApplicationAnalyticsPerformancePoint is one point of the performance series; Time is its start.
type ApplicationAnalyticsPerformancePoint struct {
	Time      time.Time                         `json:"time"`
	Requests  int64                             `json:"requests"`
	Edge      ApplicationAnalyticsLatencyBrief  `json:"edge"`
	Container *ApplicationAnalyticsLatencyBrief `json:"container"`
}

// ApplicationAnalyticsPerformancePeriod is the previous period of a performance answer.
type ApplicationAnalyticsPerformancePeriod struct {
	Start   time.Time                               `json:"start"`
	End     time.Time                               `json:"end"`
	Summary *ApplicationAnalyticsPerformanceSummary `json:"summary"`
	Series  []ApplicationAnalyticsPerformancePoint  `json:"series"`
}

// ApplicationAnalyticsLatencyEntry is a value of a dimension with its times.
type ApplicationAnalyticsLatencyEntry struct {
	// Value is the value of the dimension; send it as the filter of the same name.
	Value string `json:"value"`
	// Label is a readable name when Value is a code; nil otherwise.
	Label     *string                           `json:"label"`
	Requests  int64                             `json:"requests"`
	Edge      ApplicationAnalyticsLatencyBrief  `json:"edge"`
	Container *ApplicationAnalyticsLatencyBrief `json:"container"`
}

// ApplicationAnalyticsPathIssue is the health of a path: traffic, errors and time.
type ApplicationAnalyticsPathIssue struct {
	// Path is the path; send it as the Path filter.
	Path     string `json:"path"`
	Requests int64  `json:"requests"`
	// Errors counts the requests answered with a 5xx status.
	Errors    int64   `json:"errors"`
	ErrorRate float64 `json:"error_rate"`
	// AvgMs is the average edge time, in milliseconds.
	AvgMs float64 `json:"avg_ms"`
	// P99Ms is the 99th percentile of the edge time, in milliseconds.
	P99Ms float64 `json:"p99_ms"`
}

// ApplicationAnalyticsPerformance is the answer of GET /v1/apps/:id/analytics/performance.
// It needs a plan with Features.WebAnalyticsPerformance.
type ApplicationAnalyticsPerformance struct {
	ApplicationAnalyticsSeriesMeta
	// Summary is nil when there was no request in the window.
	Summary *ApplicationAnalyticsPerformanceSummary `json:"summary"`
	Series  []ApplicationAnalyticsPerformancePoint  `json:"series"`
	// Previous is nil unless Compare was set.
	Previous *ApplicationAnalyticsPerformancePeriod `json:"previous"`
	// Countries are the times per country of the visitor (highest Requests first).
	Countries []ApplicationAnalyticsLatencyEntry `json:"countries"`
	// EdgeLocations are the times per edge location (highest Requests first).
	EdgeLocations []ApplicationAnalyticsLatencyEntry `json:"edge_locations"`
	// SlowPaths are the slowest paths by the edge p95 (slowest first); paths with very few requests are left out.
	SlowPaths []ApplicationAnalyticsLatencyEntry `json:"slow_paths"`
	// PathIssues are the most requested paths with their errors and times (highest Requests first).
	PathIssues []ApplicationAnalyticsPathIssue `json:"path_issues"`
}

// ApplicationAnalyticsRequest is one request of the log. Fields named after a
// dimension hold the same value as that dimension, so any of them can be sent
// as a filter; "(none)" means the request had no value.
type ApplicationAnalyticsRequest struct {
	Time time.Time `json:"time"`
	// RequestID identifies the request.
	RequestID string `json:"request_id"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	// Query is the query string without the leading "?"; empty when the request had none.
	Query    string `json:"query"`
	Method   string `json:"method"`
	Protocol string `json:"protocol"`
	// Status is the HTTP status the visitor received.
	Status int `json:"status"`
	// ContainerStatus is the HTTP status your app answered; nil when the request did not reach the app.
	ContainerStatus *int `json:"container_status"`
	// EdgeTimeMs is the whole time the visitor waited for the first byte, in milliseconds.
	EdgeTimeMs float64 `json:"edge_time_ms"`
	// ContainerTimeMs is the time your app took to answer; nil when the request did not reach the app.
	ContainerTimeMs *float64 `json:"container_time_ms"`
	IP              string   `json:"ip"`
	ASN             int64    `json:"asn"`
	ASNName         string   `json:"asn_name"`
	Country         string   `json:"country"`
	EdgeLocation    string   `json:"edge_location"`
	Referer         string   `json:"referer"`
	// RefererURL is the full referer URL; nil when the request had none.
	RefererURL  *string `json:"referer_url"`
	Device      string  `json:"device"`
	OS          string  `json:"os"`
	Browser     string  `json:"browser"`
	Bot         string  `json:"bot"`
	ContentType string  `json:"content_type"`
	CacheStatus string  `json:"cache_status"`
}

// ApplicationAnalyticsRequestList is the answer of GET /v1/apps/:id/analytics/requests:
// a page of the request log. It needs a plan with Features.WebAnalyticsRequests.
type ApplicationAnalyticsRequestList struct {
	ApplicationAnalyticsMeta
	Order    ApplicationAnalyticsOrder     `json:"order"`
	Requests []ApplicationAnalyticsRequest `json:"requests"`
	// Cursor is passed back as Cursor for the next page, with the same filters and order; nil is the last page.
	Cursor *string `json:"cursor"`
}

// ApplicationAnalyticsRangeExceedsPlanDetails are the details of an ANALYTICS_RANGE_EXCEEDS_PLAN error.
type ApplicationAnalyticsRangeExceedsPlanDetails struct {
	// HistoryDays is how many days back the plan can read.
	HistoryDays int `json:"history_days"`
	// Earliest is the earliest Start accepted right now.
	Earliest time.Time `json:"earliest"`
}

// AppsAnalyticsService groups the Web Analytics routes of an app (scope
// apps:read). Every params argument may be nil. An app with no site published
// answers WebPublished false and empty data, never an error. 503
// ANALYTICS_BUSY means try again in a minute (see AnalyticsBusyRetryAfter).
type AppsAnalyticsService interface {
	// Traffic: GET /v1/apps/:id/analytics — traffic of the site, with every filter applied to every part.
	Traffic(ctx context.Context, id string, params *ApplicationAnalyticsTrafficParams, opts ...rest.RequestOpt) (ApplicationAnalyticsTraffic, error)
	// Errors: GET /v1/apps/:id/analytics/errors — 5xx errors, and 4xx with Include4xx.
	Errors(ctx context.Context, id string, params *ApplicationAnalyticsErrorsParams, opts ...rest.RequestOpt) (ApplicationAnalyticsErrors, error)
	// Performance: GET /v1/apps/:id/analytics/performance — edge and container times.
	// 403 PLAN_DOES_NOT_SUPPORT_WEB_ANALYTICS_PERFORMANCE when the plan does not include it.
	Performance(ctx context.Context, id string, params *ApplicationAnalyticsPerformanceParams, opts ...rest.RequestOpt) (ApplicationAnalyticsPerformance, error)
	// Requests: GET /v1/apps/:id/analytics/requests — one page of the request log; pass the
	// answer's Cursor back in params.Cursor for the next one (nil Cursor = last page).
	// 403 PLAN_DOES_NOT_SUPPORT_WEB_ANALYTICS_REQUESTS when the plan does not include it.
	Requests(ctx context.Context, id string, params *ApplicationAnalyticsRequestsParams, opts ...rest.RequestOpt) (ApplicationAnalyticsRequestList, error)
	// RequestsIter walks the request log page after page. It sends the same
	// route as Requests, with the window pinned (Start and End, by default 24
	// hours ending now, resolved once) so every page reads the same one.
	RequestsIter(id string, params *ApplicationAnalyticsRequestsParams, opts ...rest.RequestOpt) *ApplicationAnalyticsRequestIterator
}

func addQueryCompareStart(q url.Values, t time.Time) {
	if !t.IsZero() {
		q.Set("compare_start", t.UTC().Format(time.RFC3339Nano))
	}
}

type appsAnalyticsServiceImpl struct{ rest rest.Client }

func (s *appsServiceImpl) Analytics() AppsAnalyticsService {
	return &appsAnalyticsServiceImpl{rest: s.rest}
}

func (a *appsAnalyticsServiceImpl) get(ctx context.Context, id, suffix string, q url.Values, workspaceID string, opts []rest.RequestOpt) ([]byte, error) {
	if workspaceID != "" {
		opts = append([]rest.RequestOpt{rest.WithWorkspaceID(workspaceID)}, opts...)
	}
	return a.rest.Do(ctx, http.MethodGet, appPath(id, "/analytics"+suffix), q, nil, "", opts...)
}

func (a *appsAnalyticsServiceImpl) Traffic(ctx context.Context, id string, params *ApplicationAnalyticsTrafficParams, opts ...rest.RequestOpt) (ApplicationAnalyticsTraffic, error) {
	if params == nil {
		params = &ApplicationAnalyticsTrafficParams{}
	}
	q := params.query()
	addQueryBool(q, "compare", boolPtrIfTrue(params.Compare))
	addQueryCompareStart(q, params.CompareStart)
	if len(params.Breakdowns) > 0 {
		names := make([]string, len(params.Breakdowns))
		for i, d := range params.Breakdowns {
			names[i] = string(d)
		}
		q.Set("breakdowns", strings.Join(names, ","))
	}
	addQueryLimit(q, params.Limit)
	data, err := a.get(ctx, id, "", q, params.WorkspaceID, opts)
	if err != nil {
		return ApplicationAnalyticsTraffic{}, err
	}
	return rest.DecodeJSON[ApplicationAnalyticsTraffic](data)
}

func (a *appsAnalyticsServiceImpl) Errors(ctx context.Context, id string, params *ApplicationAnalyticsErrorsParams, opts ...rest.RequestOpt) (ApplicationAnalyticsErrors, error) {
	if params == nil {
		params = &ApplicationAnalyticsErrorsParams{}
	}
	q := params.query()
	addQueryBool(q, "compare", boolPtrIfTrue(params.Compare))
	addQueryCompareStart(q, params.CompareStart)
	addQueryBool(q, "include_4xx", boolPtrIfTrue(params.Include4xx))
	addQueryLimit(q, params.Limit)
	data, err := a.get(ctx, id, "/errors", q, params.WorkspaceID, opts)
	if err != nil {
		return ApplicationAnalyticsErrors{}, err
	}
	return rest.DecodeJSON[ApplicationAnalyticsErrors](data)
}

func (a *appsAnalyticsServiceImpl) Performance(ctx context.Context, id string, params *ApplicationAnalyticsPerformanceParams, opts ...rest.RequestOpt) (ApplicationAnalyticsPerformance, error) {
	if params == nil {
		params = &ApplicationAnalyticsPerformanceParams{}
	}
	q := params.query()
	addQueryBool(q, "compare", boolPtrIfTrue(params.Compare))
	addQueryCompareStart(q, params.CompareStart)
	addQueryLimit(q, params.Limit)
	data, err := a.get(ctx, id, "/performance", q, params.WorkspaceID, opts)
	if err != nil {
		return ApplicationAnalyticsPerformance{}, err
	}
	return rest.DecodeJSON[ApplicationAnalyticsPerformance](data)
}

func (a *appsAnalyticsServiceImpl) Requests(ctx context.Context, id string, params *ApplicationAnalyticsRequestsParams, opts ...rest.RequestOpt) (ApplicationAnalyticsRequestList, error) {
	if params == nil {
		params = &ApplicationAnalyticsRequestsParams{}
	}
	q := params.query()
	addQueryParam(q, "cursor", params.Cursor)
	addQueryParam(q, "order", string(params.Order))
	addQueryLimit(q, params.Limit)
	data, err := a.get(ctx, id, "/requests", q, params.WorkspaceID, opts)
	if err != nil {
		return ApplicationAnalyticsRequestList{}, err
	}
	return rest.DecodeJSON[ApplicationAnalyticsRequestList](data)
}

func boolPtrIfTrue(v bool) *bool {
	if !v {
		return nil
	}
	return &v
}

func addQueryLimit(q url.Values, limit int) {
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
}

// ApplicationAnalyticsRequestIterator walks the request log page after page.
// Use it like bufio.Scanner:
//
//	it := client.Apps.Analytics().RequestsIter(appID, &vertracloud.ApplicationAnalyticsRequestsParams{})
//	for it.Next(ctx) {
//		r := it.Request()
//		...
//	}
//	if err := it.Err(); err != nil { ... }
type ApplicationAnalyticsRequestIterator struct {
	svc    *appsAnalyticsServiceImpl
	id     string
	params ApplicationAnalyticsRequestsParams
	opts   []rest.RequestOpt

	page    []ApplicationAnalyticsRequest
	index   int
	done    bool
	err     error
	current ApplicationAnalyticsRequest
}

func (a *appsAnalyticsServiceImpl) RequestsIter(id string, params *ApplicationAnalyticsRequestsParams, opts ...rest.RequestOpt) *ApplicationAnalyticsRequestIterator {
	p := ApplicationAnalyticsRequestsParams{}
	if params != nil {
		p = *params
	}
	if p.End.IsZero() {
		p.End = time.Now().UTC()
	}
	if p.Start.IsZero() {
		p.Start = p.End.Add(-24 * time.Hour)
	}
	return &ApplicationAnalyticsRequestIterator{svc: a, id: id, params: p, opts: opts}
}

// Next advances to the next request, fetching the next page when needed. It
// returns false at the end of the log or on error (see Err).
func (it *ApplicationAnalyticsRequestIterator) Next(ctx context.Context) bool {
	for {
		if it.err != nil {
			return false
		}
		if it.index < len(it.page) {
			it.current = it.page[it.index]
			it.index++
			return true
		}
		if it.done {
			return false
		}
		list, err := it.svc.Requests(ctx, it.id, &it.params, it.opts...)
		if err != nil {
			it.err = err
			return false
		}
		it.page, it.index = list.Requests, 0
		if list.Cursor == nil || *list.Cursor == "" {
			it.done = true
		} else {
			it.params.Cursor = *list.Cursor
		}
	}
}

// Request returns the request Next advanced to.
func (it *ApplicationAnalyticsRequestIterator) Request() ApplicationAnalyticsRequest {
	return it.current
}

// Err returns the error that stopped the iteration, if any.
func (it *ApplicationAnalyticsRequestIterator) Err() error { return it.err }
