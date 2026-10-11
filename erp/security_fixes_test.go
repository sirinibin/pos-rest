package erp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NEW-AUTH-WEAKPW: one password rule for sign-up, own change and admin set.
func TestPasswordRuleError(t *testing.T) {
	for p, want := range map[string]string{
		"":            "at least 8 characters",
		"Ab1!":        "at least 8 characters",
		"abcdefgh":    "too weak",
		"abcdefghij":  "too weak",
		"Abcdefgh":    "",
		"abcdefg1":    "",
		"Staff@12345": "",
	} {
		if got := passwordRuleError(p); got != want {
			t.Errorf("passwordRuleError(%q) = %q, want %q", p, got, want)
		}
	}
	if e := ownPasswordErrors("Old@12345", "abcdefgh"); e["newPassword"] != "too weak" {
		t.Errorf("own change accepts a weak password: %v", e)
	}
	if e := ValidateSignup(M{"owner": M{"password": "abcdefgh"}}); e["owner.password"] != "too weak" {
		t.Errorf("sign-up rule changed: %v", e["owner.password"])
	}
}

// NEW-SEC-PWHIST: history records that a password changed, never the value,
// and stored entries from before are masked when read.
func TestHistory_PasswordsMasked(t *testing.T) {
	ch := diff(M{"name": "A"}, M{"name": "B", "password": "Secret@123"})
	found := false
	for _, c := range ch {
		cm := c.(M)
		if strings.Contains(str(cm["from"])+str(cm["to"]), "Secret@123") {
			t.Fatalf("history keeps the password: %v", cm)
		}
		if cm["field"] == "password" {
			found = true
			if cm["to"] != secretMask || cm["from"] != "—" {
				t.Errorf("password change: %v", cm)
			}
		}
	}
	if !found {
		t.Fatalf("the password change is not recorded: %v", ch)
	}
	stored := []interface{}{
		M{"at": "x", "by": "A", "action": "updated", "changes": []interface{}{M{"field": "password", "from": "—", "to": "Plain@123"}, M{"field": "name", "from": "a", "to": "b"}}},
		M{"at": "y", "by": "A", "action": "updated", "changes": []interface{}{}},
	}
	h := historyOfIn(riyadh, M{"erp": M{"h": stored}})
	if s := toJSON(h); strings.Contains(s, "Plain@123") || !strings.Contains(s, `"to":"b"`) {
		t.Fatalf("stored history read back: %s", s)
	}
	if strings.Contains(toJSON(stored), "Plain@123") == false {
		t.Fatal("masking must not modify the stored slice in place")
	}
}

func toJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// NEW-RBAC-MAXDISC: a role's maxDiscount limits line and document discounts
// on selling documents (400 on the field).
func TestDiscountLimitErrors(t *testing.T) {
	sales := &Resource{Path: "sales"}
	cashier := &Ctx{RoleID: "r_cashier"} // maxDiscount 5
	line := func(price, disc float64) M { return M{"qty": 2.0, "unitPrice": price, "unitDiscount": disc} }
	if e := discountLimitErrors(cashier, sales, M{"items": []interface{}{line(100, 50)}}, nil); e["items.0.unitDiscount"] == "" {
		t.Errorf("50%% line discount allowed for a cashier: %v", e)
	}
	if e := discountLimitErrors(cashier, sales, M{"items": []interface{}{line(100, 5)}}, nil); e != nil {
		t.Errorf("5%% line discount refused: %v", e)
	}
	if e := discountLimitErrors(cashier, sales, M{"items": []interface{}{line(100, 0)}, "discount": 20.0}, nil); e["discount"] == "" {
		t.Errorf("10%% document discount allowed for a cashier: %v", e)
	}
	if e := discountLimitErrors(cashier, sales, M{"items": []interface{}{line(100, 0)}, "discount": 10.0}, nil); e != nil {
		t.Errorf("5%% document discount refused: %v", e)
	}
	// an edit that doesn't touch discounts is not checked
	if e := discountLimitErrors(cashier, sales, M{"items": []interface{}{line(100, 50)}}, []string{"notes"}); e != nil {
		t.Errorf("unrelated edit refused: %v", e)
	}
	for _, c := range []*Ctx{{RoleID: "r_admin"}, {Admin: true, RoleID: "r_cashier"}} {
		if e := discountLimitErrors(c, sales, M{"items": []interface{}{line(100, 90)}}, nil); e != nil {
			t.Errorf("no limit for %+v: %v", c.RoleID, e)
		}
	}
	if e := discountLimitErrors(cashier, &Resource{Path: "purchases"}, M{"items": []interface{}{line(100, 90)}}, nil); e != nil {
		t.Errorf("purchases are not limited: %v", e)
	}
}

// Sign-up: server-side failures are 503, never a 400 blaming the form.
func TestSignupErr(t *testing.T) {
	if e := signupErr(&v1Result{HTTP: 500, Errors: map[string]string{"store": "boom"}}, nil); e.Status != http.StatusServiceUnavailable {
		t.Errorf("db failure: %d", e.Status)
	}
	if e := signupErr(&v1Result{HTTP: 503, Errors: map[string]string{"db": "boom"}}, nil); e.Status != http.StatusServiceUnavailable {
		t.Errorf("db read failure: %d", e.Status)
	}
	if e := signupErr(&v1Result{HTTP: 400, Errors: map[string]string{"email": "Email is already in use"}}, map[string]string{"email": "owner.email"}); e.Status != 400 || e.Fields["owner.email"] == "" {
		t.Errorf("validation: %+v", e)
	}
}

// NEW-RBAC-DELROLE: a user whose custom role was deleted has no rights (not
// the legacy Salesman's).
func TestEffectivePerms_DeletedCustomRole(t *testing.T) {
	requireDB(t)
	id := "rol_" + primitive.NewObjectID().Hex()
	ctx, cancel := dbctx()
	defer cancel()
	_, err := mainDB().Collection(collCustomRole).InsertOne(ctx, bson.M{"_id": id, "name": "Gone " + id, "deleted": true,
		"perms": permsToM(permsFor(func(string) Perm { return viewPerm() })), nativeOwnerKey: []string{fx.StoreA.Hex()}})
	if err != nil {
		t.Fatal(err)
	}
	roleID, perms := effectivePerms(M{"role": "SalesMan", "erp": M{"role": id}}, nil)
	if roleID != id {
		t.Errorf("role id = %s", roleID)
	}
	for mod, p := range perms {
		for verb, ok := range p {
			if ok {
				t.Errorf("deleted role still grants %s.%s", mod, verb)
			}
		}
	}
}

// NEW-ISO-ROLES / NEW-ISO-CUSTOMER-CATEGORIES: org records of one company are
// not listed, read or changed by another; old records without an owner are
// claimed from the users who hold them or created them.
func TestOrgNativeRecords_TenantIsolation(t *testing.T) {
	requireDB(t)
	a, b := login(t, fx.ManagerEmail), login(t, fx.UserBEmail)
	name := "Iso cat " + primitive.NewObjectID().Hex()
	r := call(t, "POST", "/customer-categories", a, M{"name": name})
	if r.Code != 201 {
		t.Fatalf("create: %s", r.Raw)
	}
	id := str(r.Body["id"])
	if strings.Contains(r.Raw, nativeOwnerKey) {
		t.Errorf("the owner field leaks: %s", r.Raw)
	}
	if l := call(t, "GET", "/customer-categories?limit=1000", b, nil); strings.Contains(l.Raw, id) {
		t.Errorf("company B lists A's category")
	}
	if g := call(t, "GET", "/customer-categories/"+id, b, nil); g.Code != 404 {
		t.Errorf("company B reads A's category: %d", g.Code)
	}
	if p := call(t, "PATCH", "/customer-categories/"+id, b, M{"name": "x"}); p.Code != 404 {
		t.Errorf("company B patches A's category: %d", p.Code)
	}
	if d := call(t, "DELETE", "/customer-categories/"+id, b, nil); d.Code != 404 {
		t.Errorf("company B deletes A's category: %d", d.Code)
	}
	// A still has it, and its owner survives an edit
	if p := call(t, "PATCH", "/customer-categories/"+id, a, M{"name": name + " 2"}); p.Code != 200 {
		t.Fatalf("A patches: %s", p.Raw)
	}
	if l := call(t, "GET", "/customer-categories?limit=1000", a, nil); !strings.Contains(l.Raw, id) {
		t.Errorf("A lost its category after an edit")
	}

	// an old role (no owner) held by a store-A user is claimed for store A
	ctx, cancel := dbctx()
	defer cancel()
	rid := "rol_" + primitive.NewObjectID().Hex()
	_, err := mainDB().Collection(collCustomRole).InsertOne(ctx, bson.M{"_id": rid, "name": "Old role " + rid, "deleted": false,
		"perms": permsToM(permsFor(func(string) Perm { return viewPerm() })), "createdBy": "nobody " + rid})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = mainDB().Collection("user").UpdateOne(ctx, bson.M{"_id": fx.SalesmanA}, bson.M{"$set": bson.M{"erp.role": rid}})
	defer mainDB().Collection("user").UpdateOne(ctx, bson.M{"_id": fx.SalesmanA}, bson.M{"$unset": bson.M{"erp.role": ""}})
	claimOnce.Delete(collCustomRole)
	(&nativeBackend{coll: collCustomRole, org: true}).claimUnowned()
	var d bson.M
	if err := mainDB().Collection(collCustomRole).FindOne(ctx, bson.M{"_id": rid}).Decode(&d); err != nil {
		t.Fatal(err)
	}
	if owner := strs(normDoc(d)[nativeOwnerKey]); len(owner) != 1 || owner[0] != fx.StoreA.Hex() {
		t.Fatalf("claimed owner = %v, want [%s]", owner, fx.StoreA.Hex())
	}
	if g := call(t, "GET", "/roles/"+rid, b, nil); g.Code != 404 {
		t.Errorf("company B reads A's role: %d", g.Code)
	}
	if g := call(t, "GET", "/roles/"+rid, a, nil); g.Code != 200 {
		t.Errorf("A reads its own role: %d %s", g.Code, g.Raw)
	}
	if l := call(t, "GET", "/roles?limit=1000", b, nil); strings.Contains(l.Raw, rid) {
		t.Errorf("company B lists A's role")
	}
	// a role name is unique within a company only
	c := &Ctx{User: M{}, Stores: []M{{"_id": fx.StoreB}}, storeIdx: map[string]M{fx.StoreB.Hex(): {"_id": fx.StoreB}}}
	if e := roleValidate(c, M{"name": "Old role " + rid}, ""); e["name"] != "" {
		t.Errorf("company B may not reuse A's role name: %v", e)
	}
	c = &Ctx{User: M{}, Stores: []M{{"_id": fx.StoreA}}, storeIdx: map[string]M{fx.StoreA.Hex(): {"_id": fx.StoreA}}}
	if e := roleValidate(c, M{"name": "old ROLE " + rid}, ""); e["name"] == "" {
		t.Errorf("A reuses its own role name")
	}
	if customRoleOfCompany(&Ctx{User: M{}, storeIdx: map[string]M{fx.StoreB.Hex(): {}}, Stores: []M{{"_id": fx.StoreB}}}, rid) {
		t.Errorf("company B may assign A's role")
	}
}
