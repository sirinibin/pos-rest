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

// TestUser_Integration runs create -> view -> list -> update -> delete for a
// user against a real MongoDB + Redis, including the admin-role guard, the
// non-admin list scoping and that a deleted user's token stops working.
func TestUser_Integration(t *testing.T) {
	fx := requireDB(t)
	adminTok := tokenFor(t, fx.AdminEmail)
	managerTok := tokenFor(t, fx.ManagerEmail)

	email := fmt.Sprintf("it-user-%d@t1.example", time.Now().UnixNano())
	const password = "It-Pass@123"
	body := map[string]interface{}{
		"name": "IT User", "email": email, "mob": "0599990001", "password": password,
		"role": "Manager", "store_ids": []string{fx.StoreA.Hex()},
	}

	// auth, validation and permission failures
	gbExpectErr(t, "create without token", callHandler(t, CreateUser, "POST", "/v1/user", "", body), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "create without name/mob/password",
		callHandler(t, CreateUser, "POST", "/v1/user", adminTok, map[string]interface{}{"email": fmt.Sprintf("x-%d@t1.example", time.Now().UnixNano())}),
		http.StatusBadRequest, "name", "mob", "password")
	gbExpectErr(t, "create with negative opening balance",
		callHandler(t, CreateUser, "POST", "/v1/user", adminTok, map[string]interface{}{"name": "Neg", "email": fmt.Sprintf("neg-%d@t1.example", time.Now().UnixNano()),
			"mob": "0599990009", "password": password, "opening_balance": -5, "opening_balance_type": "weird"}),
		http.StatusBadRequest, "opening_balance", "opening_balance_type")
	asAdmin := map[string]interface{}{"name": "Wannabe Admin", "email": fmt.Sprintf("wannabe-%d@t1.example", time.Now().UnixNano()),
		"mob": "0599990002", "password": password, "role": "Admin"}
	gbExpectErr(t, "manager creating an Admin", callHandler(t, CreateUser, "POST", "/v1/user", managerTok, asAdmin), http.StatusOK, "role")
	if _, err := models.FindUserByEmail(asAdmin["email"].(string)); err == nil {
		t.Fatalf("manager was able to create an Admin user")
	}

	// create
	r := callHandler(t, CreateUser, "POST", "/v1/user", adminTok, body)
	gbExpect(t, "create", r, http.StatusOK, true)
	created := r.resultMap(t)
	id := gbStr(created, "id")
	if id == "" || gbStr(created, "created_by_name") != "Admin T1" {
		t.Fatalf("create: %s", r.Raw)
	}
	if names, _ := created["store_names"].([]interface{}); len(names) != 1 || names[0] != "Al Noor Trading - Olaya (ANT)" {
		t.Errorf("create: store_names = %v", created["store_names"])
	}
	oid, _ := primitive.ObjectIDFromHex(id)
	dbUser, err := models.FindUserByID(&oid, bson.M{})
	if err != nil {
		t.Fatalf("db read: %v", err)
	}
	if dbUser.Password == password || !dbUser.VerifyPassword(password) || dbUser.Email != email || dbUser.Role != "Manager" ||
		len(dbUser.StoreIDs) != 1 || *dbUser.StoreIDs[0] != fx.StoreA {
		t.Errorf("db: password must be stored hashed and verifiable; user=%+v", dbUser)
	}

	// duplicate e-mail
	gbExpectErr(t, "duplicate email", callHandler(t, CreateUser, "POST", "/v1/user", adminTok, body), http.StatusConflict, "email")

	// view (password never returned); the new user's own token works
	userTok := tokenFor(t, email)
	r = callHandler(t, ViewUser, "GET", "/v1/user/"+id, userTok, nil, "id", id)
	gbExpect(t, "view as self", r, http.StatusOK, true)
	viewed := r.resultMap(t)
	if gbStr(viewed, "email") != email || gbStr(viewed, "name") != "IT User" {
		t.Errorf("view: %s", r.Raw)
	}
	if _, ok := viewed["password"]; ok {
		t.Errorf("view leaked the password hash: %s", r.Raw)
	}
	missing := primitive.NewObjectID().Hex()
	gbExpectErr(t, "view unknown", callHandler(t, ViewUser, "GET", "/v1/user/"+missing, adminTok, nil, "id", missing), http.StatusOK, "view")

	// list: admin finds it by e-mail; a manager of the same store also sees it but never Admin users
	r = callHandler(t, ListUser, "GET", gbURL("/v1/user", primitive.NilObjectID, "search[email]", email), adminTok, nil)
	gbExpect(t, "admin list", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); len(rows) != 1 || rows[id] == nil || gbTotalCount(t, r) != 1 {
		t.Errorf("admin list by email: %s", r.Raw)
	}
	r = callHandler(t, ListUser, "GET", gbURL("/v1/user", primitive.NilObjectID, "limit", "1000"), managerTok, nil)
	gbExpect(t, "manager list", r, http.StatusOK, true)
	mrows := gbIDs(gbList(t, r))
	if mrows[id] == nil {
		t.Errorf("manager of the same store cannot see the new user")
	}
	for rid, row := range mrows {
		if gbStr(row, "role") == "Admin" || row["admin"] == true || rid == fx.Admin.Hex() {
			t.Errorf("manager list exposes an admin user: %v", row)
		}
	}
	r = callHandler(t, ListUser, "GET", gbURL("/v1/user", primitive.NilObjectID, "search[role]", "Admin", "limit", "1000"), managerTok, nil)
	gbExpect(t, "manager list role=Admin", r, http.StatusOK, true)
	for _, row := range gbList(t, r) {
		if gbStr(row, "role") == "Admin" {
			t.Errorf("manager searching role=Admin got an admin: %v", row)
		}
	}

	// update: rename, new mobile and password; manager may not promote to Admin
	upd := map[string]interface{}{"name": "IT User Renamed", "email": email, "mob": "0599990003", "password": "New-Pass@456",
		"role": "SalesMan", "store_ids": []string{fx.StoreA.Hex()}}
	r = callHandler(t, UpdateUser, "PUT", "/v1/user/"+id, adminTok, upd, "id", id)
	gbExpect(t, "update", r, http.StatusOK, true)
	if m := r.resultMap(t); gbStr(m, "name") != "IT User Renamed" || gbStr(m, "role") != "SalesMan" || m["password"] != nil {
		t.Errorf("update result: %s", r.Raw)
	}
	dbUser, _ = models.FindUserByID(&oid, bson.M{})
	if dbUser == nil || dbUser.Mob != "0599990003" || !dbUser.VerifyPassword("New-Pass@456") || dbUser.VerifyPassword(password) || dbUser.UpdatedByName != "Admin T1" {
		t.Errorf("update not persisted: %+v", dbUser)
	}
	promote := map[string]interface{}{"name": "IT User Renamed", "email": email, "mob": "0599990003", "role": "Admin"}
	gbExpectErr(t, "manager promoting to Admin", callHandler(t, UpdateUser, "PUT", "/v1/user/"+id, managerTok, promote, "id", id), http.StatusOK, "role")
	gbExpectErr(t, "update with taken email",
		callHandler(t, UpdateUser, "PUT", "/v1/user/"+id, adminTok, map[string]interface{}{"name": "X", "email": fx.SalesEmail, "mob": "0599990003"}, "id", id),
		http.StatusConflict, "email")
	if u, _ := models.FindUserByID(&oid, bson.M{}); u == nil || u.Role != "SalesMan" || u.Email != email {
		t.Errorf("rejected updates changed the user: %+v", u)
	}

	// delete: gone from lists, and the user's token is rejected
	r = callHandler(t, DeleteUser, "DELETE", "/v1/user/"+id, adminTok, nil, "id", id)
	gbExpect(t, "delete", r, http.StatusOK, true)
	r = callHandler(t, ListUser, "GET", gbURL("/v1/user", primitive.NilObjectID, "search[email]", email), adminTok, nil)
	gbExpect(t, "list after delete", r, http.StatusOK, true)
	if rows := gbList(t, r); len(rows) != 0 {
		t.Errorf("deleted user still listed: %s", r.Raw)
	}
	gbExpectErr(t, "deleted user's token", callHandler(t, ViewUser, "GET", "/v1/user/"+id, userTok, nil, "id", id), http.StatusUnauthorized, "access_token")
}

// TestCanAccessUserRoles_NoUserObject verifies that canAccessUserRoles returns
// false when models.UserObject is nil (no authenticated session).
func TestCanAccessUserRoles_NoUserObject(t *testing.T) {
	// Ensure no user is set in the global slot.
	original := models.UserObject
	models.UserObject = nil
	defer func() { models.UserObject = original }()

	actions := []string{"view", "create", "update", "delete"}
	for _, action := range actions {
		if canAccessUserRoles(action) {
			t.Errorf("canAccessUserRoles(%q) = true with nil UserObject, want false", action)
		}
	}
}
