package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestQuotation_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListQuotation",
			method:  http.MethodGet,
			path:    "/v1/quotation",
			handler: ListQuotation,
		},
		{
			name:    "CreateQuotation",
			method:  http.MethodPost,
			path:    "/v1/quotation",
			handler: CreateQuotation,
		},
		{
			name:    "ViewQuotation",
			method:  http.MethodGet,
			path:    "/v1/quotation/64abc123456789001234abcd",
			handler: ViewQuotation,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateQuotation",
			method:  http.MethodPut,
			path:    "/v1/quotation/64abc123456789001234abcd",
			handler: UpdateQuotation,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteQuotation",
			method:  http.MethodDelete,
			path:    "/v1/quotation/64abc123456789001234abcd",
			handler: DeleteQuotation,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UnlinkOrderFromQuotation",
			method:  http.MethodPost,
			path:    "/v1/quotation/64abc123456789001234abcd/unlink-order",
			handler: UnlinkOrderFromQuotation,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateQuotationNetTotal",
			method:  http.MethodPost,
			path:    "/v1/quotation/calculate-net-total",
			handler: CalculateQuotationNetTotal,
		},
		{
			name:    "QuotationSummary",
			method:  http.MethodGet,
			path:    "/v1/quotation/summary",
			handler: QuotationSummary,
		},
		{
			name:    "QuotationSalesSummary",
			method:  http.MethodGet,
			path:    "/v1/quotation/sales-summary",
			handler: QuotationSalesSummary,
		},
		{
			name:    "ViewPreviousQuotation",
			method:  http.MethodGet,
			path:    "/v1/quotation/64abc123456789001234abcd/previous",
			handler: ViewPreviousQuotation,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewNextQuotation",
			method:  http.MethodGet,
			path:    "/v1/quotation/64abc123456789001234abcd/next",
			handler: ViewNextQuotation,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewLastQuotation",
			method:  http.MethodGet,
			path:    "/v1/quotation/last",
			handler: ViewLastQuotation,
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
				t.Errorf("expected 401, got %d", res.StatusCode)
			}
			if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
				t.Errorf("expected application/json Content-Type, got %q", ct)
			}
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors.access_token, got %v", resp.Errors)
			}
		})
	}
}
