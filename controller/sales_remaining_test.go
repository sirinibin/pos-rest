package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestSalesRemaining_Unauthenticated covers sales.go handlers not tested in
// handler_test.go (ListOrder, ViewOrder, CalculateSalesNetTotal, CreateOrder
// are already covered there), plus sales_history.go and sales_cash_discount.go.
func TestSalesRemaining_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// sales.go — remaining handlers
		{
			name:    "UpdateOrder",
			method:  http.MethodPut,
			path:    "/v1/order/64abc123456789001234abcd",
			handler: UpdateOrder,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteOrder",
			method:  http.MethodDelete,
			path:    "/v1/order/64abc123456789001234abcd",
			handler: DeleteOrder,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "SalesSummary",
			method:  http.MethodGet,
			path:    "/v1/order/summary",
			handler: SalesSummary,
		},
		{
			name:    "ViewPreviousOrder",
			method:  http.MethodGet,
			path:    "/v1/order/64abc123456789001234abcd/previous",
			handler: ViewPreviousOrder,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewNextOrder",
			method:  http.MethodGet,
			path:    "/v1/order/64abc123456789001234abcd/next",
			handler: ViewNextOrder,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewLastOrder",
			method:  http.MethodGet,
			path:    "/v1/order/last",
			handler: ViewLastOrder,
		},
		// sales_history.go
		{
			name:    "ListSalesHistory",
			method:  http.MethodGet,
			path:    "/v1/sales-history",
			handler: ListSalesHistory,
		},
		{
			name:    "SalesHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/sales-history/summary",
			handler: SalesHistorySummary,
		},
		// sales_cash_discount.go
		{
			name:    "ListSalesCashDiscount",
			method:  http.MethodGet,
			path:    "/v1/sales-cash-discount",
			handler: ListSalesCashDiscount,
		},
		{
			name:    "CreateSalesCashDiscount",
			method:  http.MethodPost,
			path:    "/v1/sales-cash-discount",
			handler: CreateSalesCashDiscount,
		},
		{
			name:    "UpdateSalesCashDiscount",
			method:  http.MethodPut,
			path:    "/v1/sales-cash-discount/64abc123456789001234abcd",
			handler: UpdateSalesCashDiscount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewSalesCashDiscount",
			method:  http.MethodGet,
			path:    "/v1/sales-cash-discount/64abc123456789001234abcd",
			handler: ViewSalesCashDiscount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if resp.Status {
				t.Error("expected status=false in body")
			}

			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors.access_token key, got errors=%v", resp.Errors)
			}
		})
	}
}
