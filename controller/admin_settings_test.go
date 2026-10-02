package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAdminSettingsHandlers_Unauthenticated verifies both admin settings handlers
// return 401 with {"error": "unauthorized"} when no auth token is provided.
// Note: admin_settings.go uses a simple {"error": "..."} envelope, not the
// standard {"status": false, "errors": {...}} format.
func TestAdminSettingsHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{"GetAdminSettings", http.MethodGet, "/v1/admin-settings", GetAdminSettingsHandler},
		{"UpdateAdminSettings", http.MethodPut, "/v1/admin-settings", UpdateAdminSettingsHandler},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", ct)
			}

			// admin_settings handlers use {"error": "unauthorized"} format
			var resp map[string]string
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp["error"] == "" {
				t.Errorf("expected non-empty error field, got %v", resp)
			}
		})
	}
}
