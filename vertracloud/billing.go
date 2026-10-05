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

// BillingService groups every /v1/orders* and /v1/redeem/:code route (7
// routes): order listing/status/creation/PIX under Orders, and Redeem as a
// direct method (it is not nested under Orders — a redeem code is not an
// order).
type BillingService interface {
	// Redeem: POST /v1/redeem/:code — scope redeem:write.
	Redeem(ctx context.Context, code string, opts ...rest.RequestOpt) (RedeemResponse, error)
	Orders() BillingOrdersService
	// Details: GET /v1/users/me/billing — scope billing:read. Returns nil
	// until billing details are saved.
	Details(ctx context.Context, opts ...rest.RequestOpt) (*BillingDetails, error)
	// UpdateDetails: PUT /v1/users/me/billing — scope billing:write.
	// Replaces the details; they must be complete before Orders().Create.
	UpdateDetails(ctx context.Context, body BillingDetailsBody, opts ...rest.RequestOpt) (BillingDetails, error)
}

func newBillingService(rc rest.Client) BillingService { return &billingServiceImpl{rest: rc} }

type billingServiceImpl struct{ rest rest.Client }

// billingJSONBody marshals v to an io.Reader suitable for rest.Client.Do,
// paired with the "application/json" content type.
func billingJSONBody(v any) (*bytes.Reader, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("vertracloud: encode request body: %w", err)
	}
	return bytes.NewReader(b), nil
}

func (s *billingServiceImpl) Redeem(ctx context.Context, code string, opts ...rest.RequestOpt) (RedeemResponse, error) {
	data, err := s.rest.Do(ctx, http.MethodPost, "/v1/redeem/"+url.PathEscape(code), nil, nil, "", opts...)
	if err != nil {
		return RedeemResponse{}, err
	}
	return rest.DecodeJSON[RedeemResponse](data)
}

func (s *billingServiceImpl) Details(ctx context.Context, opts ...rest.RequestOpt) (*BillingDetails, error) {
	data, err := s.rest.Do(ctx, http.MethodGet, "/v1/users/me/billing", nil, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[*BillingDetails](data)
}

func (s *billingServiceImpl) UpdateDetails(ctx context.Context, body BillingDetailsBody, opts ...rest.RequestOpt) (BillingDetails, error) {
	r, err := billingJSONBody(body)
	if err != nil {
		return BillingDetails{}, err
	}
	data, err := s.rest.Do(ctx, http.MethodPut, "/v1/users/me/billing", nil, r, "application/json", opts...)
	if err != nil {
		return BillingDetails{}, err
	}
	return rest.DecodeJSON[BillingDetails](data)
}

func (s *billingServiceImpl) Orders() BillingOrdersService {
	return &billingOrdersServiceImpl{rest: s.rest}
}

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

// OrderStatus is the lifecycle state of an order.
type OrderStatus string

const (
	OrderStatusUnpaid    OrderStatus = "unpaid"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusExpired   OrderStatus = "expired"
)

// OrderType is every type an existing or newly created order may carry.
type OrderType string

const (
	OrderTypePurchase OrderType = "purchase"
	OrderTypeRenew    OrderType = "renew"
	OrderTypeUpgrade  OrderType = "upgrade"
)

// OrderProvider is Orders.List's optional
// "provider" query filter.
type OrderProvider string

const (
	OrderProviderPix        OrderProvider = "pix"
	OrderProviderCard       OrderProvider = "card"
	OrderProviderRedeemCode OrderProvider = "redeem_code"
)

// ---------------------------------------------------------------------------
// Payload structs
// ---------------------------------------------------------------------------

// OrderRelatedPlan is the related_to.plan shape of OrderRelated. Duration
// is only present when this struct comes from OrderListItem (GET
// /v1/orders); OrderStatusInfo's plan (GET /v1/orders/:orderId/status)
// never sets it.
type OrderRelatedPlan struct {
	Name     string `json:"name"`
	Duration int    `json:"duration,omitempty"`
	Months   int    `json:"months"`
}

// OrderRelated is the related_to object shared by OrderStatusInfo and
// OrderListItem.
type OrderRelated struct {
	Plan OrderRelatedPlan `json:"plan"`
}

// OrderCreateResponse is the body of POST
// /v1/orders.
type OrderCreateResponse struct {
	ID     string      `json:"id"`
	Code   *string     `json:"code"`
	Status OrderStatus `json:"status"`
	Plan   string      `json:"plan"`
	// Duration is the plan's duration in days, mirroring RedeemPlan.Duration
	// — not to be confused with OrderCreateBody.Months.
	Duration  int           `json:"duration"`
	Discount  OrderDiscount `json:"discount"`
	Price     float64       `json:"price"`
	ExpiresAt *time.Time    `json:"expires_at"`
	// AllowedPaymentMethods lists how the order can be paid. Pay it with
	// Orders().InitiatePix; card payments are made in the dashboard.
	AllowedPaymentMethods []OrderProvider `json:"allowed_payment_methods"`
}

type OrderDiscount struct {
	Percent *float64 `json:"percent"`
	Coupon  *string  `json:"coupon"`
	Price   float64  `json:"price"`
}

// OrderStatusInfo is the response of GET
// /v1/orders/:orderId/status.
type OrderStatusInfo struct {
	ID        string       `json:"id"`
	Status    OrderStatus  `json:"status"`
	Price     float64      `json:"price"`
	RelatedTo OrderRelated `json:"related_to"`
	// AllowedPaymentMethods: same meaning as in OrderCreateResponse.
	AllowedPaymentMethods []OrderProvider `json:"allowed_payment_methods"`
}

// OrderListItem is one entry of GET /v1/orders.
type OrderListItem struct {
	ID        string        `json:"id"`
	Status    OrderStatus   `json:"status"`
	Price     float64       `json:"price"`
	Provider  OrderProvider `json:"provider"`
	Type      OrderType     `json:"type"`
	RelatedTo OrderRelated  `json:"related_to"`
	CreatedAt time.Time     `json:"created_at"`
	PaidAt    *time.Time    `json:"paid_at"`
	// HasReceipt is true when a payment receipt exists (viewable in the dashboard).
	HasReceipt bool `json:"has_receipt"`
}

// PixQRCode is the qrcode object of PixPaymentResponse.
type PixQRCode struct {
	Copy   *string `json:"copy"`
	Base64 *string `json:"base64"`
}

// PixPaymentResponse is the response of POST /v1/orders/:orderId/initiate/pix.
type PixPaymentResponse struct {
	TransactionAmount float64   `json:"transaction_amount"`
	ExternalReference *string   `json:"external_reference"`
	TxID              string    `json:"txid"`
	QRCode            PixQRCode `json:"qrcode"`
}

// RedeemPlan is the plan object of RedeemResponse.
type RedeemPlan struct {
	Name     string `json:"name"`
	Duration int    `json:"duration"`
}

// RedeemResponse is the response of POST
// /v1/redeem/:code.
type RedeemResponse struct {
	Plan RedeemPlan `json:"plan"`
}

// BillingAddress is the postal address of the billing details. Country is a
// two-letter ISO code. In Brazil Number, District, CityCode, State and a
// 8-digit PostalCode are required.
type BillingAddress struct {
	Line1      string  `json:"line1"`
	Number     *string `json:"number"`
	Line2      *string `json:"line2"`
	District   *string `json:"district"`
	City       string  `json:"city"`
	CityCode   *string `json:"city_code"`
	State      *string `json:"state"`
	PostalCode *string `json:"postal_code"`
	Country    string  `json:"country"`
}

// BillingTaxIDType is the kind of Brazilian tax document.
type BillingTaxIDType string

const (
	BillingTaxIDTypeCPF  BillingTaxIDType = "cpf"
	BillingTaxIDTypeCNPJ BillingTaxIDType = "cnpj"
)

// BillingTaxID is the tax document as returned by the API: masked, never the
// full number.
type BillingTaxID struct {
	Type   BillingTaxIDType `json:"type"`
	Masked string           `json:"masked"`
}

// BillingDetails is the response of GET/PUT /v1/users/me/billing. Complete is
// true when the details are enough to create an order.
type BillingDetails struct {
	Name     *string         `json:"name"`
	Address  *BillingAddress `json:"address"`
	Phone    *string         `json:"phone"`
	TaxID    *BillingTaxID   `json:"tax_id"`
	Complete bool            `json:"complete"`
}

// ---------------------------------------------------------------------------
// Request bodies
// ---------------------------------------------------------------------------

// OrderCreateBody is the body of POST
// /v1/orders. Plan and Months are required for every order type. Months is 1, 3 or 12
// (the Economy plan only accepts 1).
type OrderCreateBody struct {
	Plan   string    `json:"plan"`
	Months int       `json:"months"`
	Coupon string    `json:"coupon,omitempty"`
	Type   OrderType `json:"type,omitempty"`
	Source string    `json:"source,omitempty"`
}

// BillingTaxIDBody is the tax document sent to PUT /v1/users/me/billing.
type BillingTaxIDBody struct {
	Type  BillingTaxIDType `json:"type"`
	Value string           `json:"value"`
}

// BillingDetailsBody is the body of PUT /v1/users/me/billing. Phone and TaxID
// are optional: nil omits the field (the saved value is kept), NullableNull
// sends null (removes the saved value). Phone is E.164.
type BillingDetailsBody struct {
	Name    string                      `json:"name"`
	Address BillingAddress              `json:"address"`
	Phone   *Nullable[string]           `json:"phone,omitempty"`
	TaxID   *Nullable[BillingTaxIDBody] `json:"tax_id,omitempty"`
}

// ---------------------------------------------------------------------------
// billingOrderPath helper
// ---------------------------------------------------------------------------

// billingOrderPath builds /v1/orders/:orderId<suffix>, URL-encoding orderID.
func billingOrderPath(orderID, suffix string) string {
	return "/v1/orders/" + url.PathEscape(orderID) + suffix
}

// ---------------------------------------------------------------------------
// BillingOrdersService
// ---------------------------------------------------------------------------

// BillingOrdersService groups the /v1/orders* routes.
type BillingOrdersService interface {
	// List: GET /v1/orders — scope billing:read. params may be nil.
	List(ctx context.Context, params *OrderListParams, opts ...rest.RequestOpt) ([]OrderListItem, error)
	// Status: GET /v1/orders/:orderId/status — scope billing:read.
	Status(ctx context.Context, orderID string, opts ...rest.RequestOpt) (OrderStatusInfo, error)
	// Create: POST /v1/orders — scope billing:write.
	Create(ctx context.Context, body OrderCreateBody, opts ...rest.RequestOpt) (OrderCreateResponse, error)
	// InitiatePix: POST /v1/orders/:orderId/initiate/pix — scope
	// billing:write. Generates (or re-fetches) the order's PIX QR code.
	InitiatePix(ctx context.Context, orderID string, opts ...rest.RequestOpt) (PixPaymentResponse, error)
}

// OrderListParams are the optional filters of BillingOrdersService.List.
type OrderListParams struct {
	// Provider filters by payment method; empty returns every provider.
	Provider OrderProvider
}

type billingOrdersServiceImpl struct{ rest rest.Client }

func (o *billingOrdersServiceImpl) List(ctx context.Context, params *OrderListParams, opts ...rest.RequestOpt) ([]OrderListItem, error) {
	q := url.Values{}
	if params != nil {
		addQueryParam(q, "provider", string(params.Provider))
	}
	data, err := o.rest.Do(ctx, http.MethodGet, "/v1/orders", q, nil, "", opts...)
	if err != nil {
		return nil, err
	}
	return rest.DecodeJSON[[]OrderListItem](data)
}

func (o *billingOrdersServiceImpl) Status(ctx context.Context, orderID string, opts ...rest.RequestOpt) (OrderStatusInfo, error) {
	data, err := o.rest.Do(ctx, http.MethodGet, billingOrderPath(orderID, "/status"), nil, nil, "", opts...)
	if err != nil {
		return OrderStatusInfo{}, err
	}
	return rest.DecodeJSON[OrderStatusInfo](data)
}

func (o *billingOrdersServiceImpl) Create(ctx context.Context, body OrderCreateBody, opts ...rest.RequestOpt) (OrderCreateResponse, error) {
	r, err := billingJSONBody(body)
	if err != nil {
		return OrderCreateResponse{}, err
	}
	data, err := o.rest.Do(ctx, http.MethodPost, "/v1/orders", nil, r, "application/json", opts...)
	if err != nil {
		return OrderCreateResponse{}, err
	}
	return rest.DecodeJSON[OrderCreateResponse](data)
}

func (o *billingOrdersServiceImpl) InitiatePix(ctx context.Context, orderID string, opts ...rest.RequestOpt) (PixPaymentResponse, error) {
	data, err := o.rest.Do(ctx, http.MethodPost, billingOrderPath(orderID, "/initiate/pix"), nil, nil, "", opts...)
	if err != nil {
		return PixPaymentResponse{}, err
	}
	return rest.DecodeJSON[PixPaymentResponse](data)
}
