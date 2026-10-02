package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestCapitalAndDividentHandlers_Unauthenticated verifies that every capital
// and divident handler returns HTTP 401 with errors["access_token"] when no
// auth token is supplied.
func TestCapitalAndDividentHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── capital.go ──────────────────────────────────────────────────────
		{
			name:    "ListCapital",
			method:  http.MethodGet,
			path:    "/v1/capital",
			handler: ListCapital,
		},
		{
			name:    "CreateCapital",
			method:  http.MethodPost,
			path:    "/v1/capital",
			handler: CreateCapital,
		},
		{
			name:    "UpdateCapital",
			method:  http.MethodPut,
			path:    "/v1/capital/" + fakeID,
			handler: UpdateCapital,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCapital",
			method:  http.MethodGet,
			path:    "/v1/capital/" + fakeID,
			handler: ViewCapital,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCapitalByCode",
			method:  http.MethodGet,
			path:    "/v1/capital/code/CAP-001",
			handler: ViewCapitalByCode,
			muxVars: map[string]string{"code": "CAP-001"},
		},
		{
			name:    "DeleteCapital",
			method:  http.MethodDelete,
			path:    "/v1/capital/" + fakeID,
			handler: DeleteCapital,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── divident.go ─────────────────────────────────────────────────────
		{
			name:    "ListDivident",
			method:  http.MethodGet,
			path:    "/v1/divident",
			handler: ListDivident,
		},
		{
			name:    "CreateDivident",
			method:  http.MethodPost,
			path:    "/v1/divident",
			handler: CreateDivident,
		},
		{
			name:    "UpdateDivident",
			method:  http.MethodPut,
			path:    "/v1/divident/" + fakeID,
			handler: UpdateDivident,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewDivident",
			method:  http.MethodGet,
			path:    "/v1/divident/" + fakeID,
			handler: ViewDivident,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewDividentByCode",
			method:  http.MethodGet,
			path:    "/v1/divident/code/DIV-001",
			handler: ViewDividentByCode,
			muxVars: map[string]string{"code": "DIV-001"},
		},
		{
			name:    "DeleteDivident",
			method:  http.MethodDelete,
			path:    "/v1/divident/" + fakeID,
			handler: DeleteDivident,
			muxVars: map[string]string{"id": fakeID},
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
