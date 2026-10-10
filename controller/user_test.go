package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
)

// TestUser_Unauthenticated verifies that every user endpoint that requires
// authentication returns HTTP 401 with errors.access_token when no token is
// provided.
// Register is a public endpoint (no token required) and is excluded here.
// ChangePassword and ToggleUserStatus are already covered in
// user_changepw_test.go and user_role_guard_test.go respectively.
func TestUser_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListUser",
			method:  http.MethodGet,
			path:    "/v1/user",
			handler: ListUser,
		},
		{
			name:    "CreateUser",
			method:  http.MethodPost,
			path:    "/v1/user",
			handler: CreateUser,
		},
		{
			name:    "UpdateUser",
			method:  http.MethodPut,
			path:    "/v1/user/64abc123456789001234abcd",
			handler: UpdateUser,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewUser",
			method:  http.MethodGet,
			path:    "/v1/user/64abc123456789001234abcd",
			handler: ViewUser,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteUser",
			method:  http.MethodDelete,
			path:    "/v1/user/64abc123456789001234abcd",
			handler: DeleteUser,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "LogOut",
			method:  http.MethodPost,
			path:    "/v1/logout",
			handler: LogOut,
		},
		{
			name:    "Me",
			method:  http.MethodGet,
			path:    "/v1/me",
			handler: Me,
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

// TestUser_Integration is a stub for DB-backed integration tests.
func TestUser_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}

// canAccessUserRoles decides from the request's own user: none means no
// access, an Admin always has it, others need a role that grants it.
func TestCanAccessUserRoles_UsesTheRequestsUser(t *testing.T) {
	for _, action := range []string{"read", "create", "update", "delete"} {
		if canAccessUserRoles(nil, action) {
			t.Errorf("canAccessUserRoles(nil, %q) = true, want false", action)
		}
		if !canAccessUserRoles(&models.User{Role: "Admin"}, action) {
			t.Errorf("an Admin may not %s user roles", action)
		}
		if canAccessUserRoles(&models.User{Role: "SalesMan"}, action) {
			t.Errorf("a SalesMan with no roles may %s user roles", action)
		}
	}
}
