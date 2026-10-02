package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestPaymentHandlers_Unauthenticated verifies that every payment-related
// handler returns HTTP 401 with {"status":false,"errors":{"access_token":…}}
// when no authentication token is provided.
func TestPaymentHandlers_Unauthenticated(t *testing.T) {
	const idVar = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── sales_payment.go ──────────────────────────────────────────────
		{
			name:    "ListSalesPayment",
			method:  http.MethodGet,
			path:    "/v1/salespayment",
			handler: ListSalesPayment,
		},
		{
			name:    "CreateSalesPayment",
			method:  http.MethodPost,
			path:    "/v1/salespayment",
			handler: CreateSalesPayment,
		},
		{
			name:    "UpdateSalesPayment",
			method:  http.MethodPut,
			path:    "/v1/salespayment/" + idVar,
			handler: UpdateSalesPayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "ViewSalesPayment",
			method:  http.MethodGet,
			path:    "/v1/salespayment/" + idVar,
			handler: ViewSalesPayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "DeleteSalesPayment",
			method:  http.MethodDelete,
			path:    "/v1/salespayment/" + idVar,
			handler: DeleteSalesPayment,
			muxVars: map[string]string{"id": idVar},
		},

		// ── sales_return_payment.go ───────────────────────────────────────
		{
			name:    "ListSalesReturnPayment",
			method:  http.MethodGet,
			path:    "/v1/salesreturnpayment",
			handler: ListSalesReturnPayment,
		},
		{
			name:    "CreateSalesReturnPayment",
			method:  http.MethodPost,
			path:    "/v1/salesreturnpayment",
			handler: CreateSalesReturnPayment,
		},
		{
			name:    "UpdateSalesReturnPayment",
			method:  http.MethodPut,
			path:    "/v1/salesreturnpayment/" + idVar,
			handler: UpdateSalesReturnPayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "ViewSalesReturnPayment",
			method:  http.MethodGet,
			path:    "/v1/salesreturnpayment/" + idVar,
			handler: ViewSalesReturnPayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "DeleteSalesReturnPayment",
			method:  http.MethodDelete,
			path:    "/v1/salesreturnpayment/" + idVar,
			handler: DeleteSalesReturnPayment,
			muxVars: map[string]string{"id": idVar},
		},

		// ── purchase_payment.go ───────────────────────────────────────────
		{
			name:    "ListPurchasePayment",
			method:  http.MethodGet,
			path:    "/v1/purchasepayment",
			handler: ListPurchasePayment,
		},
		{
			name:    "CreatePurchasePayment",
			method:  http.MethodPost,
			path:    "/v1/purchasepayment",
			handler: CreatePurchasePayment,
		},
		{
			name:    "UpdatePurchasePayment",
			method:  http.MethodPut,
			path:    "/v1/purchasepayment/" + idVar,
			handler: UpdatePurchasePayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "ViewPurchasePayment",
			method:  http.MethodGet,
			path:    "/v1/purchasepayment/" + idVar,
			handler: ViewPurchasePayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "DeletePurchasePayment",
			method:  http.MethodDelete,
			path:    "/v1/purchasepayment/" + idVar,
			handler: DeletePurchasePayment,
			muxVars: map[string]string{"id": idVar},
		},

		// ── purchase_return_payment.go ────────────────────────────────────
		{
			name:    "ListPurchaseReturnPayment",
			method:  http.MethodGet,
			path:    "/v1/purchasereturnpayment",
			handler: ListPurchaseReturnPayment,
		},
		{
			name:    "CreatePurchaseReturnPayment",
			method:  http.MethodPost,
			path:    "/v1/purchasereturnpayment",
			handler: CreatePurchaseReturnPayment,
		},
		{
			name:    "UpdatePurchaseReturnPayment",
			method:  http.MethodPut,
			path:    "/v1/purchasereturnpayment/" + idVar,
			handler: UpdatePurchaseReturnPayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "ViewPurchaseReturnPayment",
			method:  http.MethodGet,
			path:    "/v1/purchasereturnpayment/" + idVar,
			handler: ViewPurchaseReturnPayment,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "DeletePurchaseReturnPayment",
			method:  http.MethodDelete,
			path:    "/v1/purchasereturnpayment/" + idVar,
			handler: DeletePurchaseReturnPayment,
			muxVars: map[string]string{"id": idVar},
		},

		// ── purchase_cash_discount.go ─────────────────────────────────────
		{
			name:    "ListPurchaseCashDiscount",
			method:  http.MethodGet,
			path:    "/v1/purchasecashdiscount",
			handler: ListPurchaseCashDiscount,
		},
		{
			name:    "CreatePurchaseCashDiscount",
			method:  http.MethodPost,
			path:    "/v1/purchasecashdiscount",
			handler: CreatePurchaseCashDiscount,
		},
		{
			name:    "UpdatePurchaseCashDiscount",
			method:  http.MethodPut,
			path:    "/v1/purchasecashdiscount/" + idVar,
			handler: UpdatePurchaseCashDiscount,
			muxVars: map[string]string{"id": idVar},
		},
		{
			name:    "ViewPurchaseCashDiscount",
			method:  http.MethodGet,
			path:    "/v1/purchasecashdiscount/" + idVar,
			handler: ViewPurchaseCashDiscount,
			muxVars: map[string]string{"id": idVar},
		},

		// ── purchase_history.go ───────────────────────────────────────────
		{
			name:    "ListPurchaseHistory",
			method:  http.MethodGet,
			path:    "/v1/purchase/history",
			handler: ListPurchaseHistory,
		},
		{
			name:    "PurchaseHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/purchase/history/summary",
			handler: PurchaseHistorySummary,
		},

		// ── purchase_return_history.go ────────────────────────────────────
		{
			name:    "ListPurchaseReturnHistory",
			method:  http.MethodGet,
			path:    "/v1/purchase/return/history",
			handler: ListPurchaseReturnHistory,
		},
		{
			name:    "PurchaseReturnHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/purchase/return/history/summary",
			handler: PurchaseReturnHistorySummary,
		},

		// ── sales_return_history.go ───────────────────────────────────────
		{
			name:    "ListSalesReturnHistory",
			method:  http.MethodGet,
			path:    "/v1/sales/return/history",
			handler: ListSalesReturnHistory,
		},
		{
			name:    "SalesReturnHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/sales/return/history/summary",
			handler: SalesReturnHistorySummary,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))

			// Inject gorilla/mux path variables when the route requires them.
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}

			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ──────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ─────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body ─────────────────────────────────────────────────────
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
