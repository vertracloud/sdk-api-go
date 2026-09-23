package vertracloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/vertracloud/sdk-api-go/internal/vertratest"
	"github.com/vertracloud/sdk-api-go/rest"
)

func TestBilling_OrderCreateDecodesDiscountAndRequiredFields(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(201, `{"id":"ord-1","code":null,"status":"unpaid","plan":"pro","duration":30,"discount":{"percent":10,"coupon":"SAVE","price":100},"price":90,"expires_at":null}`)
	got, err := newBillingService(fake).Orders().Create(context.Background(), OrderCreateBody{Plan: "pro", Months: 1})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil || !strings.Contains(string(encoded), `"discount"`) || !strings.Contains(string(encoded), `"duration":30`) || !strings.Contains(string(encoded), `"plan":"pro"`) {
		t.Fatalf("order response = %s, err = %v", encoded, err)
	}
	for field, wantType := range map[string]string{"Status": "OrderStatus", "Provider": "OrderProvider", "Type": "OrderType"} {
		if got, ok := reflect.TypeOf(OrderListItem{}).FieldByName(field); !ok || got.Type.Name() != wantType {
			t.Fatalf("OrderListItem.%s type = %v, want %s", field, got.Type, wantType)
		}
	}
}

func TestBilling_TransactionAmountAcceptsNumberAndString(t *testing.T) {
	var got PixPaymentResponse
	if err := json.Unmarshal([]byte(`{"transaction_amount":12.5}`), &got); err != nil || got.TransactionAmount != 12.5 {
		t.Fatalf("number transaction_amount = %#v, err = %v", got, err)
	}
	if reflect.TypeOf(got.TransactionAmount) != reflect.TypeOf(float64(0)) {
		t.Fatalf("TransactionAmount type = %T, want float64", got.TransactionAmount)
	}
	if err := json.Unmarshal([]byte(`{"transaction_amount":"12.5"}`), &got); err == nil {
		t.Fatal("string transaction_amount accepted, want number-only contract")
	}
}

func TestBilling_OrderCreateRequiresPlanAndMonthsFields(t *testing.T) {
	encoded, err := json.Marshal(OrderCreateBody{})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["plan"]; !ok {
		t.Fatalf("order create body missing required plan: %s", encoded)
	}
	if _, ok := body["months"]; !ok {
		t.Fatalf("order create body missing required months: %s", encoded)
	}
}

const testOrderID = "order_123"

// TestBilling_RouteMatrix checks, for all 5 billing routes, that the Go
// method issues the expected HTTP method and path, and that
// rest.WithWorkspaceID is forwarded as the workspace_id query parameter.
func TestBilling_RouteMatrix(t *testing.T) {
	opt := rest.WithWorkspaceID("ws_1")

	type routeCase struct {
		name       string
		wantMethod string
		wantPath   string
		enqueue    func(ft *vertratest.FakeRestClient)
		invoke     func(ctx context.Context, c *Client) error
	}

	cases := []routeCase{
		{
			name: "Orders.List", wantMethod: http.MethodGet, wantPath: "/v1/orders",
			enqueue: func(ft *vertratest.FakeRestClient) { ft.EnqueueJSON(200, `[]`) },
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Billing.Orders().List(ctx, nil, opt)
				return err
			},
		},
		{
			name: "Orders.Status", wantMethod: http.MethodGet, wantPath: "/v1/orders/" + testOrderID + "/status",
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"id":"order_123","status":"paid","price":19.9,"related_to":{"plan":{"name":"pro","months":1}}}`)
			},
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Billing.Orders().Status(ctx, testOrderID, opt)
				return err
			},
		},
		{
			name: "Orders.Create", wantMethod: http.MethodPost, wantPath: "/v1/orders",
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"id":"order_123","code":null,"status":"unpaid","price":19.9,"expires_at":null}`)
			},
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Billing.Orders().Create(ctx, OrderCreateBody{Plan: "pro", Months: 1, Type: OrderTypePurchase}, opt)
				return err
			},
		},
		{
			name: "Orders.InitiatePix", wantMethod: http.MethodPost, wantPath: "/v1/orders/" + testOrderID + "/initiate/pix",
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"transaction_amount":19.9,"external_reference":null,"txid":"tx1","qrcode":{"copy":"000201","base64":"aGVsbG8="}}`)
			},
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Billing.Orders().InitiatePix(ctx, testOrderID, opt)
				return err
			},
		},
		{
			name: "Redeem", wantMethod: http.MethodPost, wantPath: "/v1/redeem/PROMO2026",
			enqueue: func(ft *vertratest.FakeRestClient) {
				ft.EnqueueJSON(200, `{"plan":{"name":"pro","duration":30}}`)
			},
			invoke: func(ctx context.Context, c *Client) error {
				_, err := c.Billing.Redeem(ctx, "PROMO2026", opt)
				return err
			},
		},
	}

	if len(cases) != 5 {
		t.Fatalf("expected 5 table cases (5 billing routes), got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := vertratest.NewFakeRestClient(t)
			tc.enqueue(fake)
			c := &Client{Billing: newBillingService(fake)}

			if err := tc.invoke(context.Background(), c); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fake.Requests) != 1 {
				t.Fatalf("want 1 request, got %d", len(fake.Requests))
			}

			req := fake.Requests[0]
			if req.Method != tc.wantMethod {
				t.Errorf("method = %s, want %s", req.Method, tc.wantMethod)
			}
			if req.Path != tc.wantPath {
				t.Errorf("path = %s, want %s", req.Path, tc.wantPath)
			}
			if req.WorkspaceID != "ws_1" {
				t.Errorf("workspace_id = %q, want ws_1", req.WorkspaceID)
			}
		})
	}
}

// TestBilling_Orders_List_ProviderFilter covers the optional "provider"
// query filter, both set and omitted.
func TestBilling_Orders_List_ProviderFilter(t *testing.T) {
	t.Run("with provider", func(t *testing.T) {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueJSON(200, `[]`)
		svc := newBillingService(fake)

		_, err := svc.Orders().List(context.Background(), &OrderListParams{Provider: OrderProviderPix})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if got := fake.Requests[0].Query.Get("provider"); got != "pix" {
			t.Errorf("provider query = %q, want pix", got)
		}
	})

	t.Run("without provider", func(t *testing.T) {
		fake := vertratest.NewFakeRestClient(t)
		fake.EnqueueJSON(200, `[]`)
		svc := newBillingService(fake)

		_, err := svc.Orders().List(context.Background(), nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if _, ok := fake.Requests[0].Query["provider"]; ok {
			t.Errorf("provider query present, want omitted")
		}
	})
}

// TestBilling_Orders_Create_Body checks the JSON body sent by Orders.Create
// uses the expected snake_case field names and omits zero-value fields.
func TestBilling_Orders_Create_Body(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"order_123","code":null,"status":"unpaid","price":19.9,"expires_at":null}`)
	svc := newBillingService(fake)

	_, err := svc.Orders().Create(context.Background(), OrderCreateBody{
		Plan:   "pro",
		Months: 3,
		Coupon: "SAVE10",
		Type:   OrderTypeRenew,
		Source: "dashboard_card",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(fake.Requests))
	}

	wantBody := `{"plan":"pro","months":3,"coupon":"SAVE10","type":"renew","source":"dashboard_card"}`
	if got := string(fake.Requests[0].Body); got != wantBody {
		t.Errorf("body = %s, want %s", got, wantBody)
	}
}

// TestBilling_Orders_Status_Decode checks OrderStatusInfo decodes the plan
// in related_to.
func TestBilling_Orders_Status_Decode(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueJSON(200, `{"id":"order_123","status":"paid","price":19.9,"related_to":{"plan":{"name":"pro","months":1}}}`)
	svc := newBillingService(fake)

	got, err := svc.Orders().Status(context.Background(), testOrderID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.RelatedTo.Plan.Name != "pro" || got.RelatedTo.Plan.Months != 1 {
		t.Errorf("RelatedTo.Plan = %+v, want pro/1", got.RelatedTo.Plan)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(fake.Requests))
	}
}

// TestBilling_Orders_Status_NotFound covers the typed-error path for a 404:
// callers distinguish error categories via *rest.APIError's IsNotFoundError
// predicate rather than a distinct error type.
func TestBilling_Orders_Status_NotFound(t *testing.T) {
	fake := vertratest.NewFakeRestClient(t)
	fake.EnqueueError(404, `{"code":"ORDER_NOT_FOUND","message":"order not found"}`)
	svc := newBillingService(fake)

	_, err := svc.Orders().Status(context.Background(), "missing")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var apiErr *rest.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v (%T), want *rest.APIError", err, err)
	}
	if !apiErr.IsNotFoundError() {
		t.Errorf("IsNotFoundError() = false, want true (status %d)", apiErr.Status)
	}
}
