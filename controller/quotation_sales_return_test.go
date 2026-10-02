package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestQuotationSalesReturn_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListQuotationSalesReturn",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return",
			handler: ListQuotationSalesReturn,
		},
		{
			name:    "CreateQuotationSalesReturn",
			method:  http.MethodPost,
			path:    "/v1/quotation-sales-return",
			handler: CreateQuotationSalesReturn,
		},
		{
			name:    "UpdateQuotationSalesReturn",
			method:  http.MethodPut,
			path:    "/v1/quotation-sales-return/64abc123456789001234abcd",
			handler: UpdateQuotationSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewQuotationSalesReturn",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return/64abc123456789001234abcd",
			handler: ViewQuotationSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteQuotationSalesReturn",
			method:  http.MethodDelete,
			path:    "/v1/quotation-sales-return/64abc123456789001234abcd",
			handler: DeleteQuotationSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateQuotationSalesReturnNetTotal",
			method:  http.MethodPost,
			path:    "/v1/quotation-sales-return/calculate-net-total",
			handler: CalculateQuotationSalesReturnNetTotal,
		},
		{
			name:    "QuotationSalesReturnSummary",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return/summary",
			handler: QuotationSalesReturnSummary,
		},
		// History
		{
			name:    "ListQuotationSalesReturnHistory",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return-history",
			handler: ListQuotationSalesReturnHistory,
		},
		{
			name:    "QuotationSalesReturnHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return-history/summary",
			handler: QuotationSalesReturnHistorySummary,
		},
		// Payment
		{
			name:    "ListQuotationSalesReturnPayment",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return-payment",
			handler: ListQuotationSalesReturnPayment,
		},
		{
			name:    "CreateQuotationSalesReturnPayment",
			method:  http.MethodPost,
			path:    "/v1/quotation-sales-return-payment",
			handler: CreateQuotationSalesReturnPayment,
		},
		{
			name:    "UpdateQuotationSalesReturnPayment",
			method:  http.MethodPut,
			path:    "/v1/quotation-sales-return-payment/64abc123456789001234abcd",
			handler: UpdateQuotationSalesReturnPayment,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewQuotationSalesReturnPayment",
			method:  http.MethodGet,
			path:    "/v1/quotation-sales-return-payment/64abc123456789001234abcd",
			handler: ViewQuotationSalesReturnPayment,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteQuotationSalesReturnPayment",
			method:  http.MethodDelete,
			path:    "/v1/quotation-sales-return-payment/64abc123456789001234abcd",
			handler: DeleteQuotationSalesReturnPayment,
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
