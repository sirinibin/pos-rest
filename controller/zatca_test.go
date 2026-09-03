package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestZatcaReportHandlers_Unauthenticated verifies that all four ZATCA report
// handlers (ReportOrderToZatca, ReportSalesReturnToZatca,
// ReportCustomerDepositToZatca, ReportCustomerWithdrawalToZatca) return HTTP
// 401 with errors["access_token"] when no authentication token is provided.
//
// These tests do not require a database connection because the auth check is
// the very first guard in each handler — the DB is never reached.
func TestZatcaReportHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
		muxVars map[string]string
	}{
		{
			name:    "ReportOrderToZatca",
			handler: ReportOrderToZatca,
			path:    "/v1/order/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ReportSalesReturnToZatca",
			handler: ReportSalesReturnToZatca,
			path:    "/v1/sales-return/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ReportCustomerDepositToZatca",
			handler: ReportCustomerDepositToZatca,
			path:    "/v1/customer-deposit/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ReportCustomerWithdrawalToZatca",
			handler: ReportCustomerWithdrawalToZatca,
			path:    "/v1/customer-withdrawal/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(""))
			// Inject gorilla/mux path variables without starting a real router.
			r = mux.SetURLVars(r, tc.muxVars)

			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ──────────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ─────────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body ─────────────────────────────────────────────────────────
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if resp.Status {
				t.Error("expected status=false in body")
			}

			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"] key, got errors=%v", resp.Errors)
			}
		})
	}
}

// TestZatcaReportHandlers_ReconnectRequired documents the expected behavior
// when store.Zatca.ZatcaReconnectRequired is true.
//
// INTEGRATION TEST — SKIPPED in unit-test runs.
//
// The reconnect guard fires after ParseStore, which performs a real MongoDB
// lookup: it reads search[store_id] from the query string and calls
// models.FindStoreByID. There is no in-process stub for the MongoDB layer, so
// this test cannot exercise the 403 path without a live database seeded with a
// store that has Zatca.ZatcaReconnectRequired = true and a valid auth token.
//
// Expected behavior once integration conditions are met:
//   - HTTP 403 Forbidden
//   - response body: status=false
//   - response body: errors["zatca_reconnect"] contains "ZATCA re-connection is required"
func TestZatcaReportHandlers_ReconnectRequired(t *testing.T) {
	handlers := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"ReportOrderToZatca", ReportOrderToZatca},
		{"ReportSalesReturnToZatca", ReportSalesReturnToZatca},
		{"ReportCustomerDepositToZatca", ReportCustomerDepositToZatca},
		{"ReportCustomerWithdrawalToZatca", ReportCustomerWithdrawalToZatca},
	}

	for _, h := range handlers {
		h := h // capture range variable
		t.Run(h.name, func(t *testing.T) {
			t.Skip(
				"integration test: requires a live MongoDB with a store seeded with " +
					"Zatca.ZatcaReconnectRequired=true and a valid JWT auth token",
			)
		})
	}
}

// TestZatcaReportHandlers_InvalidID documents the expected behavior when a
// syntactically invalid (non-hex) resource ID is supplied in the URL path.
//
// INTEGRATION TEST — SKIPPED in unit-test runs.
//
// The ObjectID-parsing guard fires after the auth check, which calls
// models.AuthenticateByAccessToken and requires a valid JWT backed by a live
// MongoDB user record. Without a real token, every handler returns 401 before
// it ever reaches the ID-parsing step — so the 400 path is unreachable in
// pure unit tests.
//
// Expected behavior once integration conditions are met (valid token, bad ID):
//   - HTTP 400 Bad Request
//   - response body: status=false
//   - response body: errors[errorKey] contains "Invalid ... ID"
func TestZatcaReportHandlers_InvalidID(t *testing.T) {
	cases := []struct {
		name     string
		handler  http.HandlerFunc
		errorKey string
	}{
		{"ReportOrderToZatca", ReportOrderToZatca, "order_id"},
		{"ReportSalesReturnToZatca", ReportSalesReturnToZatca, "sales_return_id"},
		{"ReportCustomerDepositToZatca", ReportCustomerDepositToZatca, "deposit_id"},
		{"ReportCustomerWithdrawalToZatca", ReportCustomerWithdrawalToZatca, "withdrawal_id"},
	}

	for _, c := range cases {
		c := c // capture range variable
		t.Run(c.name, func(t *testing.T) {
			t.Skip(
				"integration test: the invalid-ID guard (400) is unreachable without a " +
					"valid auth token because the auth check fires first (401); " +
					"expected error key: " + c.errorKey,
			)
		})
	}
}

// TestClearZatcaReconnect_Unauthenticated verifies that PUT
// /v1/store/{id}/zatca/clear-reconnect returns HTTP 401 when no token is given.
func TestClearZatcaReconnect_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodPut,
		"/v1/store/64abc123456789001234abcd/zatca/clear-reconnect",
		strings.NewReader(""),
	)
	r = mux.SetURLVars(r, map[string]string{"id": "64abc123456789001234abcd"})

	w := httptest.NewRecorder()
	ClearZatcaReconnect(w, r)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}

	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["access_token"]; !ok {
		t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
	}
}

// TestClearZatcaReconnect_AdminOnly documents the expected 403 behavior when a
// non-Admin token is provided.
//
// INTEGRATION TEST — SKIPPED in unit-test runs.
//
// The role check fires after the auth guard, which requires a valid JWT backed
// by a live MongoDB user record. Without a real token the handler returns 401
// before reaching the role check.
//
// Expected behavior once integration conditions are met (valid non-admin token):
//   - HTTP 403 Forbidden
//   - response body: status=false
//   - response body: errors["role"] present
func TestClearZatcaReconnect_AdminOnly(t *testing.T) {
	t.Skip("integration test: role check unreachable without a valid auth token and live MongoDB")
}

// TestClearZatcaReconnect_InvalidStoreID documents the expected 400 behavior
// when the store ID is not a valid hex ObjectID.
//
// INTEGRATION TEST — SKIPPED in unit-test runs.
//
// The store-ID guard fires after the role check which requires a valid admin
// JWT and live MongoDB. Without those, the handler returns 401/403 first.
//
// Expected behavior once integration conditions are met (valid admin token, bad ID):
//   - HTTP 400 Bad Request
//   - response body: errors["id"] present
func TestClearZatcaReconnect_InvalidStoreID(t *testing.T) {
	t.Skip("integration test: ID guard unreachable without a valid admin token and live MongoDB")
}
