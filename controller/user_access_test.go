package controller

import (
	"testing"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestManageUserError(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	u := func(role string, admin bool, stores ...primitive.ObjectID) *models.User {
		x := &models.User{ID: primitive.NewObjectID(), Role: role, Admin: admin}
		for i := range stores {
			x.StoreIDs = append(x.StoreIDs, &stores[i])
		}
		return x
	}
	mgrA := u("Manager", false, a)
	cases := []struct {
		name     string
		req, tgt *models.User
		ok       bool
	}{
		{"admin role manages anyone", u("Admin", false), u("Admin", false, b), true},
		{"admin flag manages anyone", u("SalesMan", true), u("Manager", false, b), true},
		{"self", mgrA, mgrA, true},
		{"manager, same store salesman", mgrA, u("SalesMan", false, a, b), true},
		{"manager, other store", mgrA, u("SalesMan", false, b), false},
		{"manager, admin target", mgrA, u("Admin", false, a), false},
		{"salesman manages nobody else", u("SalesMan", false, a), u("SalesMan", false, a), false},
	}
	for _, c := range cases {
		if got := manageUserError(c.req, c.tgt) == ""; got != c.ok {
			t.Errorf("%s: allowed=%v, want %v", c.name, got, c.ok)
		}
	}
}

func TestGrantErrors(t *testing.T) {
	a, b, c := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	scoped := &models.User{Role: "Manager", StoreIDs: []*primitive.ObjectID{&a}}
	unscoped := &models.User{Role: "Manager"}
	admin := &models.User{Role: "Admin"}
	ids := func(x ...primitive.ObjectID) []*primitive.ObjectID {
		out := []*primitive.ObjectID{}
		for i := range x {
			out = append(out, &x[i])
		}
		return out
	}
	cases := []struct {
		name        string
		req         *models.User
		admin       bool
		stores, had []*primitive.ObjectID
		wantKey     string
	}{
		{"admin grants anything", admin, true, ids(b, c), nil, ""},
		{"own store", scoped, false, ids(a), nil, ""},
		{"other store", scoped, false, ids(b), nil, "store_ids"},
		{"other store the user already had", scoped, false, ids(a, b), ids(b), ""},
		{"empty list means every store", scoped, false, ids(), nil, "store_ids"},
		{"admin flag", scoped, true, ids(a), nil, "admin"},
		{"unscoped manager may give any store", unscoped, false, ids(c), nil, ""},
		{"unscoped manager still can't grant admin", unscoped, true, ids(c), nil, "admin"},
	}
	for _, x := range cases {
		errs := grantErrors(x.req, x.admin, x.stores, x.had)
		if x.wantKey == "" && len(errs) > 0 {
			t.Errorf("%s: unexpected %v", x.name, errs)
		}
		if x.wantKey != "" && errs[x.wantKey] == "" {
			t.Errorf("%s: want a %s error, got %v", x.name, x.wantKey, errs)
		}
	}
}

func TestViewUserAllowed(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	me := &models.User{ID: primitive.NewObjectID(), Role: "Manager", StoreIDs: []*primitive.ObjectID{&a}}
	created := &models.User{ID: primitive.NewObjectID(), CreatedBy: &me.ID, StoreIDs: []*primitive.ObjectID{&b}}
	other := &models.User{ID: primitive.NewObjectID(), StoreIDs: []*primitive.ObjectID{&b}}
	if !viewUserAllowed(me, me) || !viewUserAllowed(me, created) || viewUserAllowed(me, other) {
		t.Fatalf("view rule: self and created users visible, others' users hidden")
	}
}
