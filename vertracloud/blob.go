package vertracloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/vertracloud/sdk-api-go/rest"
)

// DefaultBlobBaseURL is the Blob Storage API base URL (it already includes
// the /v1 prefix).
const DefaultBlobBaseURL = "https://blob.vertracloud.app/v1"

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

type BlobVisibility string

const (
	BlobVisibilityPublic  BlobVisibility = "public"
	BlobVisibilityPrivate BlobVisibility = "private"
)

type BlobContentDisposition string

const (
	BlobContentDispositionInline     BlobContentDisposition = "inline"
	BlobContentDispositionAttachment BlobContentDisposition = "attachment"
)

type BlobUploadMode string

const (
	BlobUploadModeSingle    BlobUploadMode = "single"
	BlobUploadModeMultipart BlobUploadMode = "multipart"
)

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

// BlobObject is one stored file.
type BlobObject struct {
	ID                 string                 `json:"id"`
	Path               string                 `json:"path"`
	Size               int64                  `json:"size"`
	ContentType        string                 `json:"content_type"`
	ETag               string                 `json:"etag"`
	SHA256             *string                `json:"sha256"`
	Visibility         BlobVisibility         `json:"visibility"`
	URL                *string                `json:"url"`
	CacheControl       *string                `json:"cache_control"`
	ContentDisposition BlobContentDisposition `json:"content_disposition"`
	Filename           *string                `json:"filename"`
	Metadata           map[string]string      `json:"metadata"`
	ExpiresAt          *time.Time             `json:"expires_at"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// BlobUpload is a direct upload in progress.
type BlobUpload struct {
	ID            string            `json:"id"`
	Path          string            `json:"path"`
	Mode          BlobUploadMode    `json:"mode"`
	Status        string            `json:"status"`
	Size          int64             `json:"size"`
	ExpiresAt     time.Time         `json:"expires_at"`
	URL           string            `json:"url,omitempty"`
	Headers       map[string]string `json:"headers"`
	PartSize      int64             `json:"part_size,omitempty"`
	PartCount     int               `json:"part_count,omitempty"`
	UploadedParts []int             `json:"uploaded_parts,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
}

// BlobUploadPart is the pre-signed URL of one part of a multipart upload.
type BlobUploadPart struct {
	PartNumber int               `json:"part_number"`
	URL        string            `json:"url"`
	Size       int64             `json:"size"`
	Headers    map[string]string `json:"headers"`
}

// BlobLimits are the plan limits reported by Stats.
type BlobLimits struct {
	MaxSimpleUploadBytes         int64  `json:"max_simple_upload_bytes"`
	MaxSinglePutBytes            int64  `json:"max_single_put_bytes"`
	MaxObjectBytes               int64  `json:"max_object_bytes"`
	MinPartBytes                 int64  `json:"min_part_bytes"`
	MaxPartBytes                 int64  `json:"max_part_bytes"`
	MaxParts                     int    `json:"max_parts"`
	MaxOpenUploads               int    `json:"max_open_uploads"`
	UploadsPerSecond             *int64 `json:"uploads_per_second"`
	MaxRules                     int    `json:"max_rules"`
	MaxCustomDomains             int    `json:"max_custom_domains"`
	MetadataEnabled              bool   `json:"metadata_enabled"`
	SharePasswordEnabled         bool   `json:"share_password_enabled"`
	NoCacheEnabled               bool   `json:"no_cache_enabled"`
	MinExpirationSeconds         int64  `json:"min_expiration_seconds"`
	ImageTransformationsPerMonth int    `json:"image_transformations_per_month"`
}

// BlobStats is the storage usage of the account.
type BlobStats struct {
	Objects                  int64      `json:"objects"`
	UsedBytes                int64      `json:"used_bytes"`
	QuotaBytes               int64      `json:"quota_bytes"`
	OverQuotaBytes           int64      `json:"over_quota_bytes"`
	EstimatedOverageCents    int64      `json:"estimated_overage_cents"`
	MonthAverageBytes        int64      `json:"month_average_bytes"`
	ImageTransformationsUsed int64      `json:"image_transformations_used"`
	PublicBaseURL            string     `json:"public_base_url"`
	Limits                   BlobLimits `json:"limits"`
}

// BlobBatchResult is the outcome of one id in UpdateMany.
type BlobBatchResult struct {
	ID     string      `json:"id"`
	OK     bool        `json:"ok"`
	Code   string      `json:"code,omitempty"`
	Object *BlobObject `json:"object,omitempty"`
}

// BlobDeleteResult is the outcome of DeleteMany.
type BlobDeleteResult struct {
	Deleted  []string `json:"deleted"`
	NotFound []string `json:"not_found"`
	// Remaining is true when more objects matched than one call can delete.
	// DeleteMany already loops on it, so it is false in its return value.
	Remaining bool `json:"remaining"`
}

// BlobDownload is a temporary download link.
type BlobDownload struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// BlobUploadToken is a short-lived credential restricted to the upload routes.
type BlobUploadToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
}

// BlobShare is a share link.
type BlobShare struct {
	ID                 string    `json:"id"`
	ObjectID           string    `json:"object_id"`
	URL                string    `json:"url"`
	ExpiresAt          time.Time `json:"expires_at"`
	MaxDownloads       *int      `json:"max_downloads"`
	RemainingDownloads *int      `json:"remaining_downloads"`
	HasPassword        bool      `json:"has_password"`
	ObjectIsPublic     bool      `json:"object_is_public"`
	CreatedAt          time.Time `json:"created_at"`
}

// BlobRule is a per-folder rule.
type BlobRule struct {
	Prefix          string         `json:"prefix"`
	Visibility      BlobVisibility `json:"visibility,omitempty"`
	ExpireSeconds   *int64         `json:"expire_seconds,omitempty"`
	MaxSize         *int64         `json:"max_size,omitempty"`
	Extensions      []string       `json:"extensions,omitempty"`
	CacheControl    string         `json:"cache_control,omitempty"`
	DeleteAfterDays *int           `json:"delete_after_days,omitempty"`
	ActiveFrom      *time.Time     `json:"active_from,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

// BlobRules is the rule list with the version tag to send back to Rules.Set.
type BlobRules struct {
	Rules []BlobRule `json:"rules"`
	ETag  string     `json:"etag"`
}

// BlobRuleInput is what a client sends for one rule (the prefix is the path
// of Rules.Put/Delete, or BlobRuleEntry.Prefix in Rules.Set).
type BlobRuleInput struct {
	Visibility      BlobVisibility `json:"visibility,omitempty"`
	ExpireSeconds   *int64         `json:"expire_seconds,omitempty"`
	MaxSize         *int64         `json:"max_size,omitempty"`
	Extensions      []string       `json:"extensions,omitempty"`
	CacheControl    string         `json:"cache_control,omitempty"`
	DeleteAfterDays *int           `json:"delete_after_days,omitempty"`
}

// BlobRuleEntry is one rule of the list sent to Rules.Set.
type BlobRuleEntry struct {
	Prefix string `json:"prefix"`
	BlobRuleInput
}

// BlobS3Credentials is the S3 key pair derived from the API key.
type BlobS3Credentials struct {
	AccessKeyID     string   `json:"access_key_id"`
	SecretAccessKey string   `json:"secret_access_key"`
	Endpoint        string   `json:"endpoint"`
	Region          string   `json:"region"`
	Buckets         []string `json:"buckets"`
	Access          struct {
		Read  bool `json:"read"`
		Write bool `json:"write"`
	} `json:"access"`
}

// BlobDomainVerification is the TXT record that proves ownership of a custom domain.
type BlobDomainVerification struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BlobCustomDomain is a custom domain serving the account's files.
type BlobCustomDomain struct {
	ID          string  `json:"id"`
	Hostname    string  `json:"hostname"`
	Status      string  `json:"status"`
	SSLStatus   string  `json:"ssl_status"`
	CNAMETarget string  `json:"cname_target"`
	Prefix      *string `json:"prefix"`
	// WebContent: true serves .html, .htm, .xhtml, .svg and .xml in the browser (sandboxed); false always downloads.
	WebContent bool `json:"web_content"`
	// Verification is set only while the domain is not active.
	Verification *BlobDomainVerification `json:"verification,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Request option structs
// ---------------------------------------------------------------------------

// BlobPutOptions are the options of Put and Resume. Every field is optional.
type BlobPutOptions struct {
	Visibility   BlobVisibility
	Overwrite    *bool
	RandomSuffix *bool
	// RenameOnConflict saves a taken path as name-1.ext, name-2.ext, … instead
	// of failing; the returned object has the final path. Not with Overwrite.
	RenameOnConflict   *bool
	CacheControl       string
	ContentDisposition BlobContentDisposition
	Filename           string
	ExpiresAt          *time.Time
	Metadata           map[string]string
	// ChecksumSHA256 is the hex SHA-256 of the file.
	ChecksumSHA256 string

	// OnProgress is called with the bytes uploaded so far and the total.
	// Calls are serialized, but may come from different goroutines.
	OnProgress func(loaded, total int64)
	// Concurrency is the number of parts uploaded in parallel (default 4).
	// It only applies when the reader is an io.ReaderAt; otherwise parts are
	// uploaded one at a time.
	Concurrency int
	// PartRetries is how many times a failed part is retried (default 3).
	PartRetries int
}

// BlobListOptions are the options of List.
type BlobListOptions struct {
	Prefix     string
	Delimiter  string
	Visibility BlobVisibility
	// Limit is the page size, 1-1000 (0 = server default).
	Limit int
}

// BlobUpdate holds the object fields that can change. Nil leaves a field
// alone; NullableNull restores its default. A nil Metadata value removes
// that key.
type BlobUpdate struct {
	Visibility         BlobVisibility                    `json:"visibility,omitempty"`
	CacheControl       *Nullable[string]                 `json:"cache_control,omitempty"`
	ContentDisposition *Nullable[BlobContentDisposition] `json:"content_disposition,omitempty"`
	Filename           *Nullable[string]                 `json:"filename,omitempty"`
	ExpiresAt          *Nullable[time.Time]              `json:"expires_at,omitempty"`
	Metadata           map[string]*string                `json:"metadata,omitempty"`
}

// BlobCopyOptions are the options of Copy and Move.
type BlobCopyOptions struct {
	Overwrite    *bool
	RandomSuffix *bool
	// RenameOnConflict (Copy only, ignored by Move) saves a taken path, even the
	// source's own, as name-1.ext, name-2.ext, … Not with Overwrite.
	RenameOnConflict *bool
	Visibility       BlobVisibility
}

// BlobDeleteManyOptions selects what DeleteMany removes: IDs or Prefix.
type BlobDeleteManyOptions struct {
	IDs    []string
	Prefix string
}

// BlobDownloadOptions are the options of DownloadURL.
type BlobDownloadOptions struct {
	// ExpiresIn is the link lifetime in seconds, 60-86400 (0 = server default).
	ExpiresIn   int
	Disposition BlobContentDisposition
	Filename    string
}

// BlobUploadTokenInput is the body of CreateUploadToken. Send Path or Prefix.
type BlobUploadTokenInput struct {
	Path       string         `json:"path,omitempty"`
	Prefix     string         `json:"prefix,omitempty"`
	Visibility BlobVisibility `json:"visibility,omitempty"`
	// Overwrite lets uploads made with the token replace an existing object (default false).
	Overwrite          bool                   `json:"overwrite,omitempty"`
	MaxSize            int64                  `json:"max_size,omitempty"`
	AllowedExtensions  []string               `json:"allowed_extensions,omitempty"`
	CacheControl       string                 `json:"cache_control,omitempty"`
	ContentDisposition BlobContentDisposition `json:"content_disposition,omitempty"`
	ExpiresAt          *time.Time             `json:"expires_at,omitempty"`
	Metadata           map[string]string      `json:"metadata,omitempty"`
	ExpiresIn          int                    `json:"expires_in,omitempty"`
	MaxUses            int                    `json:"max_uses,omitempty"`
}

// BlobShareInput is the body of Shares.Create.
type BlobShareInput struct {
	ObjectID     string `json:"object_id"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	MaxDownloads int    `json:"max_downloads,omitempty"`
	Password     string `json:"password,omitempty"`
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// BlobSharesService groups the /shares routes.
type BlobSharesService interface {
	Create(ctx context.Context, in BlobShareInput, opts ...rest.RequestOpt) (BlobShare, error)
	// List returns every share, or only those of objectID when it is not empty.
	List(ctx context.Context, objectID string, opts ...rest.RequestOpt) ([]BlobShare, error)
	Revoke(ctx context.Context, shareID string, opts ...rest.RequestOpt) error
}

// BlobRulesService groups the /rules routes.
type BlobRulesService interface {
	Get(ctx context.Context, opts ...rest.RequestOpt) (BlobRules, error)
	// Set replaces the whole list; etag is the one returned by Get (sent as
	// If-Match). A stale etag fails with code BLOB_RULES_CONFLICT.
	Set(ctx context.Context, rules []BlobRuleEntry, etag string, opts ...rest.RequestOpt) (BlobRules, error)
	// Put creates or changes the rule of one prefix and returns the whole list.
	Put(ctx context.Context, prefix string, rule BlobRuleInput, opts ...rest.RequestOpt) (BlobRules, error)
	Delete(ctx context.Context, prefix string, opts ...rest.RequestOpt) (BlobRules, error)
}

// BlobDomainsService groups the /domains routes.
type BlobDomainsService interface {
	List(ctx context.Context, opts ...rest.RequestOpt) ([]BlobCustomDomain, error)
	// Add registers hostname; prefix (optional) restricts it to one folder.
	Add(ctx context.Context, hostname, prefix string, webContent bool, opts ...rest.RequestOpt) (BlobCustomDomain, error)
	// Update changes whether the domain serves web content (HTML, SVG, XML) in the browser.
	Update(ctx context.Context, domainID string, webContent bool, opts ...rest.RequestOpt) (BlobCustomDomain, error)
	Verify(ctx context.Context, domainID string, opts ...rest.RequestOpt) (BlobCustomDomain, error)
	Delete(ctx context.Context, domainID string, opts ...rest.RequestOpt) error
}

// BlobClient is the Blob Storage client. Every method accepts
// rest.WithWorkspaceID, which acts on the blob of the workspace owner.
type BlobClient struct {
	Shares  BlobSharesService
	Rules   BlobRulesService
	Domains BlobDomainsService

	rest rest.Client
	http *http.Client
}

func newBlobClient(rc rest.Client, o options) *BlobClient {
	blobRest := o.blobClient
	if blobRest == nil {
		blobRest = rc
		if cl, ok := rc.(rest.BaseURLCloner); ok {
			base := o.blobBaseURL
			if base == "" {
				base = DefaultBlobBaseURL
			}
			blobRest = cl.CloneWithBaseURL(base)
		}
	}
	hc := o.blobHTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	b := &BlobClient{rest: guardPaths(blobRest), http: hc}
	b.Shares = &blobShares{b}
	b.Rules = &blobRules{b}
	b.Domains = &blobDomains{b}
	return b
}

// call runs one JSON request and decodes the envelope into T.
func blobCall[T any](ctx context.Context, b *BlobClient, method, path string, q url.Values, body any, opts []rest.RequestOpt) (T, error) {
	var zero T
	var r *bytes.Reader
	ct := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return zero, fmt.Errorf("vertracloud: encode blob request body: %w", err)
		}
		r = bytes.NewReader(raw)
		ct = "application/json"
	}
	var data []byte
	var err error
	if r != nil {
		data, err = b.rest.Do(ctx, method, path, q, r, ct, opts...)
	} else {
		data, err = b.rest.Do(ctx, method, path, q, nil, "", opts...)
	}
	if err != nil {
		return zero, err
	}
	return rest.DecodeJSON[T](data)
}

func blobObjectPath(id string) string { return "/objects/" + url.PathEscape(id) }

// Get: GET /objects/{id}.
func (b *BlobClient) Get(ctx context.Context, id string, opts ...rest.RequestOpt) (BlobObject, error) {
	return blobCall[BlobObject](ctx, b, http.MethodGet, blobObjectPath(id), nil, nil, opts)
}

// Update: PATCH /objects/{id}.
func (b *BlobClient) Update(ctx context.Context, id string, changes BlobUpdate, opts ...rest.RequestOpt) (BlobObject, error) {
	return blobCall[BlobObject](ctx, b, http.MethodPatch, blobObjectPath(id), nil, changes, opts)
}

// blobUpdateManyBatch is the most ids the API takes per PATCH /objects.
const blobUpdateManyBatch = 100

// UpdateMany: PATCH /objects — the same changes for every id. The API takes up to 100 ids per
// request, so longer lists are sent in batches of 100 and the results are joined in order.
func (b *BlobClient) UpdateMany(ctx context.Context, ids []string, changes BlobUpdate, opts ...rest.RequestOpt) ([]BlobBatchResult, error) {
	var all []BlobBatchResult
	for start := 0; start == 0 || start < len(ids); start += blobUpdateManyBatch {
		end := min(start+blobUpdateManyBatch, len(ids))
		body := struct {
			IDs     []string   `json:"ids"`
			Changes BlobUpdate `json:"changes"`
		}{ids[start:end], changes}
		res, err := blobCall[[]BlobBatchResult](ctx, b, http.MethodPatch, "/objects", nil, body, opts)
		if err != nil {
			return all, err
		}
		all = append(all, res...)
	}
	return all, nil
}

func (b *BlobClient) copyOrMove(ctx context.Context, id, path string, move bool, o *BlobCopyOptions, opts []rest.RequestOpt) (BlobObject, error) {
	body := struct {
		Source       string         `json:"source"`
		Path         string         `json:"path"`
		Move         bool           `json:"move,omitempty"`
		Overwrite    *bool          `json:"overwrite,omitempty"`
		RandomSuffix *bool          `json:"random_suffix,omitempty"`
		Rename       *bool          `json:"rename_on_conflict,omitempty"`
		Visibility   BlobVisibility `json:"visibility,omitempty"`
	}{Source: id, Path: path, Move: move}
	if o != nil {
		body.Overwrite, body.RandomSuffix, body.Rename, body.Visibility = o.Overwrite, o.RandomSuffix, o.RenameOnConflict, o.Visibility
	}
	return blobCall[BlobObject](ctx, b, http.MethodPost, "/objects/copy", nil, body, opts)
}

// Copy: POST /objects/copy — creates another object (counts toward storage).
func (b *BlobClient) Copy(ctx context.Context, id, path string, o *BlobCopyOptions, opts ...rest.RequestOpt) (BlobObject, error) {
	return b.copyOrMove(ctx, id, path, false, o, opts)
}

// Move: POST /objects/copy with move=true — keeps the id.
func (b *BlobClient) Move(ctx context.Context, id, path string, o *BlobCopyOptions, opts ...rest.RequestOpt) (BlobObject, error) {
	return b.copyOrMove(ctx, id, path, true, o, opts)
}

// Delete: DELETE /objects/{id}.
func (b *BlobClient) Delete(ctx context.Context, id string, opts ...rest.RequestOpt) error {
	_, err := blobCall[any](ctx, b, http.MethodDelete, blobObjectPath(id), nil, nil, opts)
	return err
}

// DeleteMany: DELETE /objects by ids (chunks of 1000) or by prefix, repeating
// each call while the API reports more matches. The result is the union of
// every call, with Remaining false.
func (b *BlobClient) DeleteMany(ctx context.Context, o BlobDeleteManyOptions, opts ...rest.RequestOpt) (BlobDeleteResult, error) {
	total := BlobDeleteResult{Deleted: []string{}, NotFound: []string{}}
	if (len(o.IDs) == 0) == (o.Prefix == "") {
		return total, errors.New("vertracloud: DeleteMany needs exactly one of IDs or Prefix")
	}
	var bodies []any
	if o.Prefix != "" {
		bodies = append(bodies, struct {
			Prefix string `json:"prefix"`
		}{o.Prefix})
	}
	for i := 0; i < len(o.IDs); i += 1000 {
		end := min(i+1000, len(o.IDs))
		bodies = append(bodies, struct {
			IDs []string `json:"ids"`
		}{o.IDs[i:end]})
	}
	for _, body := range bodies {
		for {
			res, err := blobCall[BlobDeleteResult](ctx, b, http.MethodDelete, "/objects", nil, body, opts)
			if err != nil {
				return total, err
			}
			total.Deleted = append(total.Deleted, res.Deleted...)
			total.NotFound = append(total.NotFound, res.NotFound...)
			// No progress while "remaining" would loop forever.
			if !res.Remaining || len(res.Deleted)+len(res.NotFound) == 0 {
				break
			}
		}
	}
	return total, nil
}

// DownloadURL: GET /objects/{id}/download with redirect=false. The link
// cannot be revoked.
func (b *BlobClient) DownloadURL(ctx context.Context, id string, o *BlobDownloadOptions, opts ...rest.RequestOpt) (BlobDownload, error) {
	q := url.Values{"redirect": {"false"}}
	if o != nil {
		if o.ExpiresIn > 0 {
			q.Set("expires_in", strconv.Itoa(o.ExpiresIn))
		}
		addQueryParam(q, "disposition", string(o.Disposition))
		addQueryParam(q, "filename", o.Filename)
	}
	return blobCall[BlobDownload](ctx, b, http.MethodGet, blobObjectPath(id)+"/download", q, nil, opts)
}

// Stats: GET /stats.
func (b *BlobClient) Stats(ctx context.Context, opts ...rest.RequestOpt) (BlobStats, error) {
	return blobCall[BlobStats](ctx, b, http.MethodGet, "/stats", nil, nil, opts)
}

// CreateUploadToken: POST /upload-tokens.
func (b *BlobClient) CreateUploadToken(ctx context.Context, in BlobUploadTokenInput, opts ...rest.RequestOpt) (BlobUploadToken, error) {
	return blobCall[BlobUploadToken](ctx, b, http.MethodPost, "/upload-tokens", nil, in, opts)
}

// S3Credentials: GET /s3/credentials (limited to 10 calls per hour).
func (b *BlobClient) S3Credentials(ctx context.Context, opts ...rest.RequestOpt) (BlobS3Credentials, error) {
	return blobCall[BlobS3Credentials](ctx, b, http.MethodGet, "/s3/credentials", nil, nil, opts)
}

// ---------------------------------------------------------------------------
// List iterator
// ---------------------------------------------------------------------------

// BlobIterator walks every object of a List, fetching pages on demand.
//
//	it := client.Blob.List(ctx, &vertracloud.BlobListOptions{Prefix: "images/"})
//	for it.Next(ctx) { obj := it.Object() }
//	if err := it.Err(); err != nil { ... }
type BlobIterator struct {
	b       *BlobClient
	o       BlobListOptions
	opts    []rest.RequestOpt
	page    []BlobObject
	i       int
	cursor  string
	done    bool
	cur     BlobObject
	folders []string
	err     error
}

// List: GET /objects, paginated by cursor.
func (b *BlobClient) List(_ context.Context, o *BlobListOptions, opts ...rest.RequestOpt) *BlobIterator {
	it := &BlobIterator{b: b, opts: opts}
	if o != nil {
		it.o = *o
	}
	return it
}

// Next advances to the next object, fetching the next page when needed. It
// returns false at the end or on error (see Err).
func (it *BlobIterator) Next(ctx context.Context) bool {
	for it.err == nil {
		if it.i < len(it.page) {
			it.cur = it.page[it.i]
			it.i++
			return true
		}
		if it.done {
			return false
		}
		q := url.Values{}
		addQueryParam(q, "prefix", it.o.Prefix)
		addQueryParam(q, "delimiter", it.o.Delimiter)
		addQueryParam(q, "visibility", string(it.o.Visibility))
		addQueryParam(q, "cursor", it.cursor)
		if it.o.Limit > 0 {
			q.Set("limit", strconv.Itoa(it.o.Limit))
		}
		res, err := blobCall[struct {
			Objects []BlobObject `json:"objects"`
			Folders []string     `json:"folders"`
			Cursor  *string      `json:"cursor"`
		}](ctx, it.b, http.MethodGet, "/objects", q, nil, it.opts)
		if err != nil {
			it.err = err
			return false
		}
		it.page, it.i = res.Objects, 0
		it.folders = append(it.folders, res.Folders...)
		if res.Cursor == nil || *res.Cursor == "" {
			it.done = true
		} else {
			it.cursor = *res.Cursor
		}
	}
	return false
}

// Object is the object Next stopped on.
func (it *BlobIterator) Object() BlobObject { return it.cur }

// Folders are the folder prefixes found so far (needs Delimiter).
func (it *BlobIterator) Folders() []string { return it.folders }

// Err is the error that ended the iteration, if any.
func (it *BlobIterator) Err() error { return it.err }

// ---------------------------------------------------------------------------
// Shares, rules, domains
// ---------------------------------------------------------------------------

type blobShares struct{ b *BlobClient }

func (s *blobShares) Create(ctx context.Context, in BlobShareInput, opts ...rest.RequestOpt) (BlobShare, error) {
	return blobCall[BlobShare](ctx, s.b, http.MethodPost, "/shares", nil, in, opts)
}

func (s *blobShares) List(ctx context.Context, objectID string, opts ...rest.RequestOpt) ([]BlobShare, error) {
	q := url.Values{}
	addQueryParam(q, "object_id", objectID)
	return blobCall[[]BlobShare](ctx, s.b, http.MethodGet, "/shares", q, nil, opts)
}

func (s *blobShares) Revoke(ctx context.Context, shareID string, opts ...rest.RequestOpt) error {
	_, err := blobCall[any](ctx, s.b, http.MethodDelete, "/shares/"+url.PathEscape(shareID), nil, nil, opts)
	return err
}

type blobRules struct{ b *BlobClient }

func (r *blobRules) Get(ctx context.Context, opts ...rest.RequestOpt) (BlobRules, error) {
	return blobCall[BlobRules](ctx, r.b, http.MethodGet, "/rules", nil, nil, opts)
}

func (r *blobRules) Set(ctx context.Context, rules []BlobRuleEntry, etag string, opts ...rest.RequestOpt) (BlobRules, error) {
	if rules == nil {
		rules = []BlobRuleEntry{}
	}
	opts = append(opts[:len(opts):len(opts)], rest.WithHeader("If-Match", etag))
	return blobCall[BlobRules](ctx, r.b, http.MethodPut, "/rules", nil, struct {
		Rules []BlobRuleEntry `json:"rules"`
	}{rules}, opts)
}

func (r *blobRules) Put(ctx context.Context, prefix string, rule BlobRuleInput, opts ...rest.RequestOpt) (BlobRules, error) {
	return blobCall[BlobRules](ctx, r.b, http.MethodPut, "/rules/"+url.PathEscape(prefix), nil, rule, opts)
}

func (r *blobRules) Delete(ctx context.Context, prefix string, opts ...rest.RequestOpt) (BlobRules, error) {
	return blobCall[BlobRules](ctx, r.b, http.MethodDelete, "/rules/"+url.PathEscape(prefix), nil, nil, opts)
}

type blobDomains struct{ b *BlobClient }

func (d *blobDomains) List(ctx context.Context, opts ...rest.RequestOpt) ([]BlobCustomDomain, error) {
	return blobCall[[]BlobCustomDomain](ctx, d.b, http.MethodGet, "/domains", nil, nil, opts)
}

func (d *blobDomains) Add(ctx context.Context, hostname, prefix string, webContent bool, opts ...rest.RequestOpt) (BlobCustomDomain, error) {
	return blobCall[BlobCustomDomain](ctx, d.b, http.MethodPost, "/domains", nil, struct {
		Hostname   string `json:"hostname"`
		Prefix     string `json:"prefix,omitempty"`
		WebContent bool   `json:"web_content,omitempty"`
	}{hostname, prefix, webContent}, opts)
}

func (d *blobDomains) Update(ctx context.Context, domainID string, webContent bool, opts ...rest.RequestOpt) (BlobCustomDomain, error) {
	return blobCall[BlobCustomDomain](ctx, d.b, http.MethodPatch, "/domains/"+url.PathEscape(domainID), nil, struct {
		WebContent bool `json:"web_content"`
	}{webContent}, opts)
}

func (d *blobDomains) Verify(ctx context.Context, domainID string, opts ...rest.RequestOpt) (BlobCustomDomain, error) {
	return blobCall[BlobCustomDomain](ctx, d.b, http.MethodPost, "/domains/"+url.PathEscape(domainID)+"/verify", nil, nil, opts)
}

func (d *blobDomains) Delete(ctx context.Context, domainID string, opts ...rest.RequestOpt) error {
	_, err := blobCall[any](ctx, d.b, http.MethodDelete, "/domains/"+url.PathEscape(domainID), nil, nil, opts)
	return err
}
