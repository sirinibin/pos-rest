package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestVehicleAndRepairJob_Unauthenticated verifies that every vehicle and
// repair-job endpoint returns HTTP 401 with errors.access_token when no
// authentication token is provided.
func TestVehicleAndRepairJob_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── vehicle.go ────────────────────────────────────────────────────────
		{
			name:    "ListVehicle",
			method:  http.MethodGet,
			path:    "/v1/vehicle",
			handler: ListVehicle,
		},
		{
			name:    "CreateVehicle",
			method:  http.MethodPost,
			path:    "/v1/vehicle",
			handler: CreateVehicle,
		},
		{
			name:    "ViewVehicle",
			method:  http.MethodGet,
			path:    "/v1/vehicle/64abc123456789001234abcd",
			handler: ViewVehicle,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateVehicle",
			method:  http.MethodPut,
			path:    "/v1/vehicle/64abc123456789001234abcd",
			handler: UpdateVehicle,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteVehicle",
			method:  http.MethodDelete,
			path:    "/v1/vehicle/64abc123456789001234abcd",
			handler: DeleteVehicle,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ListVehicleBrands",
			method:  http.MethodGet,
			path:    "/v1/vehicle/brands",
			handler: ListVehicleBrands,
		},
		// ── repair_job.go ─────────────────────────────────────────────────────
		{
			name:    "ListRepairJob",
			method:  http.MethodGet,
			path:    "/v1/repair-job",
			handler: ListRepairJob,
		},
		{
			name:    "CreateRepairJob",
			method:  http.MethodPost,
			path:    "/v1/repair-job",
			handler: CreateRepairJob,
		},
		{
			name:    "ViewRepairJob",
			method:  http.MethodGet,
			path:    "/v1/repair-job/64abc123456789001234abcd",
			handler: ViewRepairJob,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateRepairJob",
			method:  http.MethodPut,
			path:    "/v1/repair-job/64abc123456789001234abcd",
			handler: UpdateRepairJob,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteRepairJob",
			method:  http.MethodDelete,
			path:    "/v1/repair-job/64abc123456789001234abcd",
			handler: DeleteRepairJob,
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

// TestVehicle_Integration is a stub for DB-backed integration tests.
func TestVehicle_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}

// TestRepairJob_Integration is a stub for DB-backed integration tests.
func TestRepairJob_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}
