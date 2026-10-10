package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// TestUserRole_Integration runs create -> view -> list -> update -> delete for
// an RBAC role against a real MongoDB + Redis, and checks that a non-admin
// user is denied without the role and allowed once the role grants access.
func TestUserRole_Integration(t *testing.T) {
	fx := requireDB(t)
	adminTok := tokenFor(t, fx.AdminEmail)
	managerTok := tokenFor(t, fx.ManagerEmail) // Manager without role_ids -> no RBAC permissions
	listURL := func(kv ...string) string { return gbURL("/v1/user-role", fx.StoreA, kv...) }

	name := uniqName("IT Role")
	body := map[string]interface{}{
		"name": name, "store_id": fx.StoreA.Hex(),
		"permissions": []map[string]interface{}{
			{"resource": "user_roles", "read": true, "create": false, "update": false, "delete": false},
			{"resource": "products", "read": true, "create": true, "update": true, "delete": false},
		},
	}

	// auth, permission and validation failures
	gbExpectErr(t, "create without token", callHandler(t, CreateUserRole, "POST", "/v1/user-role", "", body), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "create as manager without RBAC", callHandler(t, CreateUserRole, "POST", "/v1/user-role", managerTok, body), http.StatusForbidden, "access")
	gbExpectErr(t, "list as manager without RBAC", callHandler(t, ListUserRole, "GET", listURL(), managerTok, nil), http.StatusForbidden, "access")
	gbExpectErr(t, "create without name/store", callHandler(t, CreateUserRole, "POST", "/v1/user-role", adminTok, map[string]interface{}{"permissions": []interface{}{}}),
		http.StatusOK, "name", "store_id")
	gbExpectErr(t, "list without store", callHandler(t, ListUserRole, "GET", "/v1/user-role", adminTok, nil), http.StatusOK, "find")

	// create
	r := callHandler(t, CreateUserRole, "POST", "/v1/user-role", adminTok, body)
	gbExpect(t, "create", r, http.StatusOK, true)
	created := r.resultMap(t)
	id := gbStr(created, "id")
	if id == "" || gbStr(created, "store_name") != "Al Noor Trading" || gbStr(created, "created_by_name") != "Admin T1" {
		t.Fatalf("create: %s", r.Raw)
	}
	roleID, _ := primitive.ObjectIDFromHex(id)
	viewURL := gbURL("/v1/user-role/"+id, fx.StoreA)

	// view
	r = callHandler(t, ViewUserRole, "GET", viewURL, adminTok, nil, "id", id)
	gbExpect(t, "view", r, http.StatusOK, true)
	var role models.UserRole
	if err := json.Unmarshal(r.Result, &role); err != nil {
		t.Fatalf("decode role: %v", err)
	}
	if role.Name != name || len(role.Permissions) != 2 || role.Permissions[1].Resource != "products" || !role.Permissions[1].Update || role.Permissions[1].Delete {
		t.Errorf("view: %+v", role)
	}
	gbExpectErr(t, "view in another store", callHandler(t, ViewUserRole, "GET", gbURL("/v1/user-role/"+id, fx.StoreB), adminTok, nil, "id", id), http.StatusOK, "find")

	// list by name; the legacy fixture role is listed too
	r = callHandler(t, ListUserRole, "GET", listURL("search[name]", name), adminTok, nil)
	gbExpect(t, "list by name", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); len(rows) != 1 || rows[id] == nil || gbTotalCount(t, r) != 1 {
		t.Errorf("list by name: %s", r.Raw)
	}
	r = callHandler(t, ListUserRole, "GET", listURL("search[name]", "Legacy Stock Clerk"), adminTok, nil)
	gbExpect(t, "list legacy", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); rows[fx.LegacyRoleA.Hex()] == nil {
		t.Errorf("legacy role not listed: %s", r.Raw)
	}

	// a non-admin holding the role gets read access to roles (but not create)
	email := fmt.Sprintf("it-role-user-%d@t1.example", time.Now().UnixNano())
	r = callHandler(t, CreateUser, "POST", "/v1/user", adminTok, map[string]interface{}{
		"name": "IT Role Holder", "email": email, "mob": "0599990101", "password": "It-Pass@123",
		"role": "Manager", "store_ids": []string{fx.StoreA.Hex()}, "role_ids": []string{id},
	})
	gbExpect(t, "create role holder", r, http.StatusOK, true)
	holderID := gbStr(r.resultMap(t), "id")
	if names, _ := r.resultMap(t)["role_names"].([]interface{}); len(names) != 1 || names[0] != name {
		t.Errorf("role holder role_names = %v", r.resultMap(t)["role_names"])
	}
	holderTok := tokenFor(t, email)
	gbExpect(t, "list as role holder", callHandler(t, ListUserRole, "GET", listURL("search[name]", name), holderTok, nil), http.StatusOK, true)
	gbExpectErr(t, "create as role holder (read-only)", callHandler(t, CreateUserRole, "POST", "/v1/user-role", holderTok, body), http.StatusForbidden, "access")

	// update: rename and grant create on user_roles
	upd := map[string]interface{}{
		"name": name + " v2", "store_id": fx.StoreA.Hex(),
		"permissions": []map[string]interface{}{{"resource": "user_roles", "read": true, "create": true}},
	}
	gbExpectErr(t, "update as manager without RBAC", callHandler(t, UpdateUserRole, "PUT", viewURL, managerTok, upd, "id", id), http.StatusForbidden, "access")
	gbExpectErr(t, "update with empty name", callHandler(t, UpdateUserRole, "PUT", viewURL, adminTok,
		map[string]interface{}{"name": "", "store_id": fx.StoreA.Hex()}, "id", id), http.StatusOK, "name")
	r = callHandler(t, UpdateUserRole, "PUT", viewURL, adminTok, upd, "id", id)
	gbExpect(t, "update", r, http.StatusOK, true)
	stored, err := models.FindUserRoleByID(&fx.StoreA, &roleID, bson.M{})
	if err != nil || stored.Name != name+" v2" || len(stored.Permissions) != 1 || !stored.Permissions[0].Create || stored.UpdatedByName != "Admin T1" {
		t.Fatalf("update not persisted: %+v err=%v", stored, err)
	}
	// the updated permission takes effect for the role holder
	other := map[string]interface{}{"name": uniqName("IT Role by holder"), "store_id": fx.StoreA.Hex()}
	r = callHandler(t, CreateUserRole, "POST", "/v1/user-role", holderTok, other)
	gbExpect(t, "create as role holder after grant", r, http.StatusOK, true)
	otherID := gbStr(r.resultMap(t), "id")

	// delete: refused while assigned, allowed once the holder is gone
	gbExpectErr(t, "delete assigned role", callHandler(t, DeleteUserRole, "DELETE", viewURL, adminTok, nil, "id", id), http.StatusConflict, "delete")
	gbExpect(t, "delete role holder", callHandler(t, DeleteUser, "DELETE", "/v1/user/"+holderID, adminTok, nil, "id", holderID), http.StatusOK, true)
	gbExpectErr(t, "delete without token", callHandler(t, DeleteUserRole, "DELETE", viewURL, "", nil, "id", id), http.StatusUnauthorized, "access_token")
	r = callHandler(t, DeleteUserRole, "DELETE", viewURL, adminTok, nil, "id", id)
	gbExpect(t, "delete", r, http.StatusOK, true)
	gbExpectErr(t, "view deleted", callHandler(t, ViewUserRole, "GET", viewURL, adminTok, nil, "id", id), http.StatusOK, "find")
	r = callHandler(t, ListUserRole, "GET", listURL("search[name]", name), adminTok, nil)
	gbExpect(t, "list after delete", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); rows[id] != nil {
		t.Errorf("deleted role still listed: %s", r.Raw)
	}
	gbExpect(t, "delete second role", callHandler(t, DeleteUserRole, "DELETE", gbURL("/v1/user-role/"+otherID, fx.StoreA), adminTok, nil, "id", otherID), http.StatusOK, true)
}
