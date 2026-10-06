package erp

import (
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSalaryBalanceStatus(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want string
	}{{-500, "owed_to_employees"}, {250, "employees_owe"}, {0, "settled"}, {0.001, "settled"}, {-0.001, "settled"}} {
		if got := SalaryBalanceStatus(c.in); got != c.want {
			t.Errorf("%v: %s want %s", c.in, got, c.want)
		}
	}
}

func TestDashboardSalaryBalance_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		if r := call(t, "GET", "/dashboard/salary-balance?storeId=x", tok, nil); r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

func TestAPI_DashboardSalaryBalance(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "salbal")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"settings.enable_employee_module": true}})
	acc := func(name, side string, bal float64, extra bson.M) interface{} {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "name": name, "reference_model": "employee",
			"open": true, "balance": bal, "debit_or_credit_balance": side}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if _, err := storeDB(sid).Collection("account").InsertMany(ctx, []interface{}{
		acc("Ali", "credit_balance", 1200, nil), // store owes Ali
		acc("Sara", "debit_balance", 300, nil),  // Sara owes the store (advance)
		acc("Old", "credit_balance", 999, bson.M{"open": false}),
		acc("Gone", "credit_balance", 999, bson.M{"deleted": true}),
		acc("Cash", "debit_balance", 5000, bson.M{"reference_model": ""}),
	}); err != nil {
		t.Fatal(err)
	}
	r := call(t, "GET", "/dashboard/salary-balance?storeId="+sid, owner, nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
	// debit − credit = 300 − 1200
	if r.Body["balance"] != -900.0 || r.Body["status"] != "owed_to_employees" || r.Body["employeeModule"] != true {
		t.Fatalf("body %v", r.Body)
	}
	emps, _ := r.Body["employees"].([]interface{})
	if len(emps) != 2 {
		t.Fatalf("employees %v", emps)
	}
	first := emps[0].(map[string]interface{})
	if first["name"] != "Ali" || first["direction"] != "owed_to_employee" || first["balance"] != 1200.0 {
		t.Errorf("first %v", first)
	}
	if r := call(t, "GET", "/dashboard/salary-balance", owner, nil); r.Code != 400 {
		t.Errorf("missing storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/salary-balance?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("other store: %d", r.Code)
	}
}
