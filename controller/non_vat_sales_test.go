package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestNonVATSales_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListNonVATSales",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales",
			handler: ListNonVATSales,
		},
		{
			name:    "CreateNonVATSales",
			method:  http.MethodPost,
			path:    "/v1/non-vat-sales",
			handler: CreateNonVATSales,
		},
		{
			name:    "ViewNonVATSales",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales/64abc123456789001234abcd",
			handler: ViewNonVATSales,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateNonVATSales",
			method:  http.MethodPut,
			path:    "/v1/non-vat-sales/64abc123456789001234abcd",
			handler: UpdateNonVATSales,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteNonVATSales",
			method:  http.MethodDelete,
			path:    "/v1/non-vat-sales/64abc123456789001234abcd",
			handler: DeleteNonVATSales,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewPreviousNonVATSale",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales/64abc123456789001234abcd/previous",
			handler: ViewPreviousNonVATSale,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewNextNonVATSale",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales/64abc123456789001234abcd/next",
			handler: ViewNextNonVATSale,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewLastNonVATSale",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales/last",
			handler: ViewLastNonVATSale,
		},
		{
			name:    "CalculateNonVATSalesNetTotal",
			method:  http.MethodPost,
			path:    "/v1/non-vat-sales/calculate-net-total",
			handler: CalculateNonVATSalesNetTotal,
		},
		// Non-VAT Sales Return
		{
			name:    "ListNonVATSalesReturn",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales-return",
			handler: ListNonVATSalesReturn,
		},
		{
			name:    "CreateNonVATSalesReturn",
			method:  http.MethodPost,
			path:    "/v1/non-vat-sales-return",
			handler: CreateNonVATSalesReturn,
		},
		{
			name:    "ViewNonVATSalesReturn",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales-return/64abc123456789001234abcd",
			handler: ViewNonVATSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateNonVATSalesReturn",
			method:  http.MethodPut,
			path:    "/v1/non-vat-sales-return/64abc123456789001234abcd",
			handler: UpdateNonVATSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteNonVATSalesReturn",
			method:  http.MethodDelete,
			path:    "/v1/non-vat-sales-return/64abc123456789001234abcd",
			handler: DeleteNonVATSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateNonVATSalesReturnNetTotal",
			method:  http.MethodPost,
			path:    "/v1/non-vat-sales-return/calculate-net-total",
			handler: CalculateNonVATSalesReturnNetTotal,
		},
		// History
		{
			name:    "ListNonVATSalesHistory",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales-history",
			handler: ListNonVATSalesHistory,
		},
		{
			name:    "ListNonVATSalesReturnHistory",
			method:  http.MethodGet,
			path:    "/v1/non-vat-sales-return-history",
			handler: ListNonVATSalesReturnHistory,
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
