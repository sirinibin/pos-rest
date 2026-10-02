package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestUserRole_Unauthenticated verifies that every user-role endpoint returns
// HTTP 401 with errors.access_token when no authentication token is provided.
func TestUserRole_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListUserRole",
			method:  http.MethodGet,
			path:    "/v1/userrole",
			handler: ListUserRole,
		},
		{
			name:    "CreateUserRole",
			method:  http.MethodPost,
			path:    "/v1/userrole",
			handler: CreateUserRole,
		},
		{
			name:    "ViewUserRole",
			method:  http.MethodGet,
			path:    "/v1/userrole/64abc123456789001234abcd",
			handler: ViewUserRole,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateUserRole",
			method:  http.MethodPut,
			path:    "/v1/userrole/64abc123456789001234abcd",
			handler: UpdateUserRole,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteUserRole",
			method:  http.MethodDelete,
			path:    "/v1/userrole/64abc123456789001234abcd",
			handler: DeleteUserRole,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "GetEffectivePermissions",
			method:  http.MethodGet,
			path:    "/v1/userrole/effective-permissions",
			handler: GetEffectivePermissions,
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

// TestUserRole_Integration is a stub for DB-backed integration tests.
func TestUserRole_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}
