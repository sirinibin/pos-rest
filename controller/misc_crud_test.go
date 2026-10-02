package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestMiscCRUD_Unauthenticated verifies that every listed endpoint returns
// HTTP 401 with {"status":false,"errors":{"access_token":…}} when no
// authentication token is provided.
func TestMiscCRUD_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── CapitalWithdrawal ─────────────────────────────────────────────────
		{
			name:    "ListCapitalWithdrawal",
			method:  http.MethodGet,
			path:    "/v1/capitalwithdrawal",
			handler: ListCapitalWithdrawal,
		},
		{
			name:    "CreateCapitalWithdrawal",
			method:  http.MethodPost,
			path:    "/v1/capitalwithdrawal",
			handler: CreateCapitalWithdrawal,
		},
		{
			name:    "UpdateCapitalWithdrawal",
			method:  http.MethodPut,
			path:    "/v1/capitalwithdrawal/" + fakeID,
			handler: UpdateCapitalWithdrawal,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCapitalWithdrawal",
			method:  http.MethodGet,
			path:    "/v1/capitalwithdrawal/" + fakeID,
			handler: ViewCapitalWithdrawal,
			muxVars: map[string]string{"id": fakeID},
		},
		// ViewCapitalWithdrawalByCode uses {code} not {id}.
		{
			name:    "ViewCapitalWithdrawalByCode",
			method:  http.MethodGet,
			path:    "/v1/capitalwithdrawal/code/CW-001",
			handler: ViewCapitalWithdrawalByCode,
			muxVars: map[string]string{"code": "CW-001"},
		},
		{
			name:    "DeleteCapitalWithdrawal",
			method:  http.MethodDelete,
			path:    "/v1/capitalwithdrawal/" + fakeID,
			handler: DeleteCapitalWithdrawal,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── CustomerPackage ───────────────────────────────────────────────────
		{
			name:    "ListCustomerPackage",
			method:  http.MethodGet,
			path:    "/v1/customer-package",
			handler: ListCustomerPackage,
		},
		{
			name:    "CreateCustomerPackage",
			method:  http.MethodPost,
			path:    "/v1/customer-package",
			handler: CreateCustomerPackage,
		},
		{
			name:    "UpdateCustomerPackage",
			method:  http.MethodPut,
			path:    "/v1/customer-package/" + fakeID,
			handler: UpdateCustomerPackage,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomerPackage",
			method:  http.MethodGet,
			path:    "/v1/customer-package/" + fakeID,
			handler: ViewCustomerPackage,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteCustomerPackage",
			method:  http.MethodDelete,
			path:    "/v1/customer-package/" + fakeID,
			handler: DeleteCustomerPackage,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── History endpoints ─────────────────────────────────────────────────
		{
			name:    "ListStockTransferHistory",
			method:  http.MethodGet,
			path:    "/v1/stocktransfer/history",
			handler: ListStockTransferHistory,
		},
		{
			name:    "ListDeliveryNoteHistory",
			method:  http.MethodGet,
			path:    "/v1/deliverynote/history",
			handler: ListDeliveryNoteHistory,
		},
		{
			name:    "ListQuotationHistory",
			method:  http.MethodGet,
			path:    "/v1/quotation/history",
			handler: ListQuotationHistory,
		},
		{
			name:    "QuotationHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/quotation/history/summary",
			handler: QuotationHistorySummary,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ───────────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ──────────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body ──────────────────────────────────────────────────────────
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

// ── Image upload / delete stubs ───────────────────────────────────────────────
//
// All three Upload*Image handlers call r.ParseMultipartForm before any auth
// check, so they cannot be covered by a plain no-token unit test.
// All three Delete*Image handlers carry no authentication guard — they validate
// required query parameters and return 400, not 401.

// TestUploadCustomerImage_SkipAuth_Integration stubs the customer image upload
// handler which reads multipart form data before authenticating.
func TestUploadCustomerImage_SkipAuth_Integration(t *testing.T) {
	t.Skip("UploadCustomerImage reads multipart form data before auth check; requires integration environment with real DB and CDN")
}

// TestUploadVendorImage_SkipAuth_Integration stubs the vendor image upload
// handler which reads multipart form data before authenticating.
func TestUploadVendorImage_SkipAuth_Integration(t *testing.T) {
	t.Skip("UploadVendorImage reads multipart form data before auth check; requires integration environment with real DB and CDN")
}

// TestDeleteCustomerImage_NoAuthGuard documents that DeleteCustomerImage has
// no authentication guard and returns 400 when required query params are absent.
func TestDeleteCustomerImage_NoAuthGuard(t *testing.T) {
	r := httptest.NewRequest(http.MethodDelete, "/v1/customer/image", nil)
	w := httptest.NewRecorder()
	DeleteCustomerImage(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required params, got %d", res.StatusCode)
	}
}

// TestDeleteVendorImage_NoAuthGuard documents that DeleteVendorImage has no
// authentication guard and returns 400 when required query params are absent.
func TestDeleteVendorImage_NoAuthGuard(t *testing.T) {
	r := httptest.NewRequest(http.MethodDelete, "/v1/vendor/image", nil)
	w := httptest.NewRecorder()
	DeleteVendorImage(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required params, got %d", res.StatusCode)
	}
}
