package erp

import (
	"reflect"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// The header store switcher lists /auth/me stores filtered by user.storeIds,
// so storeIds must cover every store the caller can access.
func TestUserToContractFull_StoreIDs(t *testing.T) {
	a, b, c := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	all := []M{{"_id": a}, {"_id": b}, {"_id": c}}
	allHex := []string{a.Hex(), b.Hex(), c.Hex()}
	cases := []struct {
		name   string
		user   M
		stores []M
		want   []string
	}{
		{"admin flag, no store_ids", M{"admin": true}, all, allHex},
		{"admin flag, subset store_ids", M{"admin": true, "store_ids": bson.A{a}}, all, allHex},
		{"legacy Admin role, subset store_ids", M{"role": "Admin", "store_ids": bson.A{b}}, all, allHex},
		{"legacy admin role is case-insensitive", M{"role": "admin", "store_ids": bson.A{a, b}}, all, allHex},
		{"admin with hex-string store_ids", M{"admin": true, "store_ids": bson.A{a.Hex()}}, all, allHex},
		{"admin, no stores at all", M{"admin": true, "store_ids": bson.A{a}}, []M{}, []string{}},
		{"non-admin keeps its own store_ids", M{"role": "Manager", "store_ids": bson.A{a, c}}, []M{{"_id": a}, {"_id": c}}, []string{a.Hex(), c.Hex()}},
		{"non-admin with no store_ids", M{"role": "Salesman"}, []M{}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.user["_id"] = primitive.NewObjectID()
			got, _ := userToContractFull(tc.user, tc.stores)["storeIds"].([]string)
			g := append([]string{}, got...)
			w := append([]string{}, tc.want...)
			sort.Strings(g)
			sort.Strings(w)
			if !reflect.DeepEqual(g, w) {
				t.Fatalf("storeIds = %v, want %v", got, tc.want)
			}
		})
	}
}

// The users list replaces the current user's record in the UI, so it must
// agree with /auth/me.
func TestUserToContract_AdminStoreIDsFollowViewerStores(t *testing.T) {
	a, b := hexID(), hexID()
	x := testX(a, M{"_id": a})
	x.c.Stores = []M{{"_id": a}, {"_id": b}}
	cases := []struct {
		name string
		user M
		want int
	}{
		{"admin flag with subset", M{"admin": true, "store_ids": []interface{}{a}}, 2},
		{"legacy Admin role without store_ids", M{"role": "Admin"}, 2},
		{"manager keeps own store_ids", M{"role": "Manager", "store_ids": []interface{}{b}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.user["_id"] = hexID()
			if got := strs(userToContract(x, tc.user)["storeIds"]); len(got) != tc.want {
				t.Fatalf("storeIds = %v, want %d", got, tc.want)
			}
		})
	}
	// without a request context (e.g. history rendering) the stored ids are used
	if got := strs(userToContract(nil, M{"admin": true, "store_ids": []interface{}{a}})["storeIds"]); len(got) != 1 {
		t.Fatalf("nil ctx storeIds = %v", got)
	}
}

func TestUserToContractFull_NoPassword(t *testing.T) {
	u := M{"_id": primitive.NewObjectID(), "admin": true, "password": "hash"}
	if _, ok := userToContractFull(u, nil)["password"]; ok {
		t.Fatal("password must never be rendered")
	}
}

// Regression: a legacy admin whose store_ids lists only some stores must
// still see every store, both in /auth/me stores and user.storeIds.
func TestAPI_Me_AdminStoreIDsCoverAllStores(t *testing.T) {
	requireDB(t)
	ctx, cancel := dbctx()
	defer cancel()
	users := mainDB().Collection("user")
	if _, err := users.UpdateOne(ctx, bson.M{"_id": fx.Admin}, bson.M{"$set": bson.M{"store_ids": bson.A{fx.StoreA}}}); err != nil {
		t.Fatal(err)
	}
	defer users.UpdateOne(ctx, bson.M{"_id": fx.Admin}, bson.M{"$set": bson.M{"store_ids": bson.A{fx.StoreA, fx.StoreB}}})

	me := call(t, "GET", "/auth/me", login(t, fx.AdminEmail), nil)
	if me.Code != 200 {
		t.Fatalf("me: %d %s", me.Code, me.Raw)
	}
	ids := map[string]bool{}
	for _, id := range arr(get(me.Body, "user.storeIds")) {
		ids[str(id)] = true
	}
	stores := arr(me.Body["stores"])
	if len(stores) < 2 || len(ids) != len(stores) {
		t.Fatalf("storeIds=%v stores=%d", ids, len(stores))
	}
	for _, s := range stores {
		if id := str(s.(M)["id"]); !ids[id] {
			t.Errorf("store %s returned but missing from user.storeIds", id)
		}
	}
	if !ids[storeA()] || !ids[storeB()] {
		t.Fatalf("admin must see store A and B: %v", ids)
	}

	// GET /users renders the admin with the same storeIds (the UI swaps the
	// current user for this record once users load)
	list := call(t, "GET", "/users", login(t, fx.AdminEmail), nil)
	found := false
	for _, u := range list.data() {
		if str(u.(M)["id"]) == fx.Admin.Hex() {
			found = true
			if n := len(arr(u.(M)["storeIds"])); n != len(ids) {
				t.Fatalf("users list admin storeIds=%d, /auth/me=%d", n, len(ids))
			}
		}
	}
	if !found {
		t.Fatalf("admin missing from /users: %d %s", list.Code, list.Raw)
	}

	// a non-admin still only sees the stores it was granted
	mgr := call(t, "GET", "/auth/me", login(t, fx.ManagerEmail), nil)
	if got := arr(get(mgr.Body, "user.storeIds")); len(got) != 1 || got[0] != storeA() {
		t.Fatalf("manager storeIds = %v", got)
	}
}
