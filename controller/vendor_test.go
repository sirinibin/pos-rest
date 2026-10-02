package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// ---------------------------------------------------------------------------
// Unauthenticated handler tests
// ---------------------------------------------------------------------------

func TestVendorHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListVendor",
			method:  http.MethodGet,
			path:    "/v1/vendor",
			handler: ListVendor,
		},
		{
			name:    "CreateVendor",
			method:  http.MethodPost,
			path:    "/v1/vendor",
			handler: CreateVendor,
		},
		{
			name:    "UpdateVendor",
			method:  http.MethodPut,
			path:    "/v1/vendor/" + fakeID,
			handler: UpdateVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewVendor",
			method:  http.MethodGet,
			path:    "/v1/vendor/" + fakeID,
			handler: ViewVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewVendorByVatNoByName",
			method:  http.MethodGet,
			path:    "/v1/vendor/by-vat-name",
			handler: ViewVendorByVatNoByName,
		},
		{
			name:    "DeleteVendor",
			method:  http.MethodDelete,
			path:    "/v1/vendor/" + fakeID,
			handler: DeleteVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreVendor",
			method:  http.MethodPost,
			path:    "/v1/vendor/" + fakeID + "/restore",
			handler: RestoreVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "VendorSummary",
			method:  http.MethodGet,
			path:    "/v1/vendor/summary",
			handler: VendorSummary,
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

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
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
				t.Errorf("expected errors[access_token], got %v", resp.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration stubs
// ---------------------------------------------------------------------------

// TestCreateVendor_Integration documents the end-to-end vendor creation path.
func TestCreateVendor_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually in a connected environment")
}

// TestUpdateVendor_Integration documents the opening-balance-changed path.
func TestUpdateVendor_Integration_OpeningBalanceChanged(t *testing.T) {
	t.Skip("requires live MongoDB with a seeded vendor and a valid auth token")
}
