package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAutomobileAndChartHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{"GetAutoMobileDashboard", http.MethodGet, "/v1/automobile/dashboard", GetAutoMobileDashboard},
		{"ShareChartImage", http.MethodPost, "/v1/chart-image-share", ShareChartImage},
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
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
			}
		})
	}
}
