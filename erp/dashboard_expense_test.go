package erp

import (
	"math"
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---- pure: the old business dashboard formula (reactjs-pos stats/index.js) ----

var expenseInputs = TotalExpenseInputs{
	Expense: 1000, Purchase: 5000, PurchaseReturn: 500,
	AccountedPurchase: 3000, AccountedPurchaseReturn: 200, DepositPurchaseFund: 100,
	SalesCashDiscount: 40, SalesReturnCashDiscount: 10,
	PurchaseCashDiscount: 30, PurchaseReturnCashDiscount: 5,
	AccountedPurchaseCashDiscount: 20, AccountedPurchaseReturnCashDisc: 3,
	QtnSalesCashDiscount: 7, QtnSalesReturnCashDiscount: 2,
	SalesCommission: 60, SalesReturnCommission: 15, SalaryPaid: 2500,
}

func TestTotalExpense_Formula(t *testing.T) {
	cases := []struct {
		name  string
		flags TotalExpenseFlags
		want  float64
	}{
		// 1000 + (5000-500) + (40-10+5-30) + (60-15)
		{"default: all purchases", TotalExpenseFlags{}, 1000 + 4500 + 5 + 45},
		// 1000 - 100 + 3000 - 200 + (40-10+3-20) + 45
		{"purchases on accounts disabled", TotalExpenseFlags{DisablePurchasesOnAccounts: true}, 1000 - 100 + 2800 + 13 + 45},
		// default + (7-2)
		{"sales in quotation", TotalExpenseFlags{SalesInQuotation: true}, 5550 + 5},
		// default + salary
		{"employee module", TotalExpenseFlags{EmployeeModule: true}, 5550 + 2500},
		{"all settings on", TotalExpenseFlags{true, true, true}, 3758 + 5 + 2500},
	}
	for _, c := range cases {
		got := TotalExpense(expenseInputs, c.flags, 15)
		if math.Abs(got.Total-c.want) > 0.001 {
			t.Errorf("%s: total %v, want %v", c.name, got.Total, c.want)
		}
		sum := expenseInputs.Expense + got.Purchases + got.CashDiscount + got.Commission + got.Salary
		if math.Abs(sum-got.Total) > 0.011 {
			t.Errorf("%s: parts %v don't add up to total %v", c.name, sum, got.Total)
		}
		if math.Abs(got.Vat+got.TotalWithoutVat-got.Total) > 0.011 {
			t.Errorf("%s: vat %v + ex %v != total %v", c.name, got.Vat, got.TotalWithoutVat, got.Total)
		}
	}
}

func TestTotalExpense_SalaryIgnoredWithoutEmployeeModule(t *testing.T) {
	got := TotalExpense(TotalExpenseInputs{Expense: 100, SalaryPaid: 900}, TotalExpenseFlags{}, 15)
	if got.Total != 100 || got.Salary != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestTotalExpense_VatAndRounding(t *testing.T) {
	got := TotalExpense(TotalExpenseInputs{Expense: 115}, TotalExpenseFlags{}, 15)
	if got.Vat != 15 || got.TotalWithoutVat != 100 {
		t.Fatalf("15%%: %+v", got)
	}
	// a store with no VAT rate falls back to 15% like the old dashboard
	if g := TotalExpense(TotalExpenseInputs{Expense: 115}, TotalExpenseFlags{}, 0); g.Vat != 15 {
		t.Fatalf("default vat: %+v", g)
	}
	if g := TotalExpense(TotalExpenseInputs{Expense: 10.005, Purchase: 0.001}, TotalExpenseFlags{}, 15); g.Total != 10.01 {
		t.Fatalf("rounding: %+v", g)
	}
	// purchase returns can exceed purchases: the figure goes down, never clamps
	if g := TotalExpense(TotalExpenseInputs{Purchase: 10, PurchaseReturn: 50}, TotalExpenseFlags{}, 15); g.Total != -40 {
		t.Fatalf("negative: %+v", g)
	}
}

func TestDashboardDateRange(t *testing.T) {
	// Saudi Arabia (UTC+3, offset -3): 1 Oct 00:00 store time = 30 Sep 21:00 UTC
	r, err := DashboardDateRange("2026-10-01", "2026-10-31", -3)
	if err != nil {
		t.Fatal(err)
	}
	if got := r["$gte"].(time.Time); !got.Equal(time.Date(2026, 9, 30, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("start %v", got)
	}
	if got := r["$lte"].(time.Time); !got.Equal(time.Date(2026, 10, 31, 20, 59, 59, 0, time.UTC)) {
		t.Errorf("end %v", got)
	}
	// India (UTC+5:30)
	r, _ = DashboardDateRange("2026-10-06", "2026-10-06", -5.5)
	if got := r["$gte"].(time.Time); !got.Equal(time.Date(2026, 10, 5, 18, 30, 0, 0, time.UTC)) {
		t.Errorf("IN start %v", got)
	}
	if r, _ := DashboardDateRange("", "", -3); r != nil {
		t.Errorf("no dates: %v", r)
	}
	if r, _ := DashboardDateRange("2026-10-01", "", -3); r["$lte"] != nil || r["$gte"] == nil {
		t.Errorf("open end: %v", r)
	}
	if r, _ := DashboardDateRange("", "2026-10-01", -3); r["$gte"] != nil || r["$lte"] == nil {
		t.Errorf("open start: %v", r)
	}
	bad := []struct{ from, to, field string }{
		{"01-10-2026", "", "from"}, {"", "2026/10/01", "to"}, {"2026-10-05", "2026-10-01", "to"}, {"2026-02-30", "", "from"},
	}
	for _, b := range bad {
		_, err := DashboardDateRange(b.from, b.to, -3)
		ae, ok := err.(*APIError)
		if !ok || ae.Status != http.StatusBadRequest || ae.Fields[b.field] == "" {
			t.Errorf("%q..%q: want 400 on %s, got %v", b.from, b.to, b.field, err)
		}
	}
}

func TestDashboardTotalExpense_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		r := call(t, "GET", "/dashboard/total-expense?storeId=x", tok, nil)
		if r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

// ---- DB-backed API + integration ----

func TestAPI_DashboardTotalExpense(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "totexp")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"country_code": "SA", "vat_percent": 15.0,
		"settings.disable_purchases_on_accounts": false, "settings.enable_sales_in_quotation": false,
		"settings.enable_employee_module": false,
	}}); err != nil {
		t.Fatal(err)
	}
	sdb := storeDB(sid)
	// Saudi time: 1 Oct 2026 00:30 = 30 Sep 21:30 UTC → belongs to October
	inOct := time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC)
	// 30 Sep 23:30 Saudi time = 20:30 UTC → September, outside the range
	inSep := time.Date(2026, 9, 30, 20, 30, 0, 0, time.UTC)
	doc := func(d time.Time, extra bson.M) bson.M {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": d}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	ins := func(coll string, docs ...interface{}) {
		if _, err := sdb.Collection(coll).InsertMany(ctx, docs); err != nil {
			t.Fatalf("%s: %v", coll, err)
		}
	}
	ins("expense", doc(inOct, bson.M{"amount": 1000.0}), doc(inSep, bson.M{"amount": 99999.0}),
		doc(inOct, bson.M{"amount": 5555.0, "deleted": true}))
	ins("order", doc(inOct, bson.M{"net_total": 9000.0, "cash_discount": 40.0, "commission": 60.0}))
	ins("salesreturn", doc(inOct, bson.M{"net_total": 100.0, "cash_discount": 10.0, "commission": 15.0}))
	ins("purchase", doc(inOct, bson.M{"net_total": 3000.0, "cash_discount": 20.0, "enable_on_accounts": true}),
		doc(inOct, bson.M{"net_total": 2000.0, "cash_discount": 10.0}))
	ins("purchasereturn", doc(inOct, bson.M{"net_total": 200.0, "cash_discount": 3.0, "enable_on_accounts": true}),
		doc(inOct, bson.M{"net_total": 300.0, "cash_discount": 2.0}))
	ins("customerdeposit", doc(inOct, bson.M{"net_total": 100.0, "payments": bson.A{bson.M{"method": "purchase_fund", "amount": 100.0}}}))
	ins("quotation", doc(inOct, bson.M{"type": "invoice", "net_total": 700.0, "cash_discount": 7.0}),
		doc(inOct, bson.M{"type": "quotation", "net_total": 999.0, "cash_discount": 99.0}))
	ins("quotation_sales_return", doc(inOct, bson.M{"net_total": 50.0, "cash_discount": 2.0}))
	ins("employee_salary_payment", doc(inOct, bson.M{"amount": 2500.0}))

	url := "/dashboard/total-expense?storeId=" + sid + "&from=2026-10-01&to=2026-10-31"
	get := func(tok string) resp { return call(t, "GET", url, tok, nil) }

	// default settings: all purchases − returns
	r := get(owner)
	if r.Code != 200 {
		t.Fatalf("default: %d %s", r.Code, r.Raw)
	}
	// 1000 + (5000-500) + (40-10+5-30) + (60-15) = 5550
	if v := sub(r.Body, "result")["total"]; v != 5550.0 {
		t.Fatalf("default total %v (%s)", v, r.Raw)
	}
	if v := sub(r.Body, "inputs")["expense"]; v != 1000.0 {
		t.Errorf("expense must exclude September and deleted rows: %v", v)
	}

	// all three store settings on
	_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"settings.disable_purchases_on_accounts": true, "settings.enable_sales_in_quotation": true,
		"settings.enable_employee_module": true,
	}})
	r = get(owner)
	// 1000 - 100 + 3000 - 200 + (40-10+3-20) + (7-2) + 45 + 2500 = 6263
	if v := sub(r.Body, "result")["total"]; v != 6263.0 {
		t.Fatalf("settings on total %v (%s)", v, r.Raw)
	}
	if f := sub(r.Body, "flags"); f["disablePurchasesOnAccounts"] != true || f["salesInQuotation"] != true || f["employeeModule"] != true {
		t.Errorf("flags %v", f)
	}

	// no range: September's expense counts too
	r = call(t, "GET", "/dashboard/total-expense?storeId="+sid, owner, nil)
	if v := sub(r.Body, "inputs")["expense"]; v != 100999.0 {
		t.Errorf("all-time expense %v", v)
	}

	// validation and access
	if r := call(t, "GET", "/dashboard/total-expense", owner, nil); r.Code != 400 || r.errField("storeId") == "" {
		t.Errorf("missing storeId: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", url+"x", owner, nil); r.Code != 400 {
		t.Errorf("bad date: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/dashboard/total-expense?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("someone else's store: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/total-expense?storeId="+fx.StoreA.Hex()+"&from=2026-10-01&to=2026-10-31", login(t, fx.AdminEmail), nil); r.Code != 200 {
		t.Errorf("admin: %d %s", r.Code, r.Raw)
	}
}
