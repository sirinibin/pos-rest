package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestWarehouseAndSignature_Unauthenticated verifies that every warehouse and
// signature endpoint returns HTTP 401 with errors.access_token when no
// authentication token is provided.
func TestWarehouseAndSignature_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── warehouse.go ──────────────────────────────────────────────────────
		{
			name:    "ListWarehouse",
			method:  http.MethodGet,
			path:    "/v1/warehouse",
			handler: ListWarehouse,
		},
		{
			name:    "CreateWarehouse",
			method:  http.MethodPost,
			path:    "/v1/warehouse",
			handler: CreateWarehouse,
		},
		{
			name:    "UpdateWarehouse",
			method:  http.MethodPut,
			path:    "/v1/warehouse/64abc123456789001234abcd",
			handler: UpdateWarehouse,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewWarehouse",
			method:  http.MethodGet,
			path:    "/v1/warehouse/64abc123456789001234abcd",
			handler: ViewWarehouse,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteWarehouse",
			method:  http.MethodDelete,
			path:    "/v1/warehouse/64abc123456789001234abcd",
			handler: DeleteWarehouse,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		// ── signature.go ──────────────────────────────────────────────────────
		{
			name:    "ListSignature",
			method:  http.MethodGet,
			path:    "/v1/signature",
			handler: ListSignature,
		},
		{
			name:    "CreateSignature",
			method:  http.MethodPost,
			path:    "/v1/signature",
			handler: CreateSignature,
		},
		{
			name:    "UpdateSignature",
			method:  http.MethodPut,
			path:    "/v1/signature/64abc123456789001234abcd",
			handler: UpdateSignature,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewSignature",
			method:  http.MethodGet,
			path:    "/v1/signature/64abc123456789001234abcd",
			handler: ViewSignature,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteSignature",
			method:  http.MethodDelete,
			path:    "/v1/signature/64abc123456789001234abcd",
			handler: DeleteSignature,
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

// TestWarehouse_Integration is a stub for DB-backed integration tests.
func TestWarehouse_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}

// TestSignature_Integration is a stub for DB-backed integration tests.
func TestSignature_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}
