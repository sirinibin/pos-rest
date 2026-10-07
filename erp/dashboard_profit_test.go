package erp

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Old business dashboard profit card (business_dashboard/charts/KPICards.js):
// profitLoss = totalRevenue − expenseTotal, VAT split at the store rate.
func TestDashboardNetProfit_Formula(t *testing.T) {
	cases := []struct {
		name                 string
		rev, exp, vatPercent float64
		want                 NetProfitResult
	}{
		{"profit", 13220, 6263, 15, NetProfitResult{Revenue: 13220, Expense: 6263, Profit: 6957, Vat: 907.43, ProfitWithoutVat: 6049.57, Profitable: true}},
		{"loss", 1000, 3300, 15, NetProfitResult{Revenue: 1000, Expense: 3300, Profit: -2300, Vat: -300, ProfitWithoutVat: -2000, Profitable: false}},
		{"break-even counts as profit", 500, 500, 15, NetProfitResult{Revenue: 500, Expense: 500, Profitable: true}},
		{"no VAT rate defaults to 15%", 115, 0, 0, NetProfitResult{Revenue: 115, Profit: 115, Vat: 15, ProfitWithoutVat: 100, Profitable: true}},
		{"5% VAT", 105, 0, 5, NetProfitResult{Revenue: 105, Profit: 105, Vat: 5, ProfitWithoutVat: 100, Profitable: true}},
	}
	for _, c := range cases {
		if got := DashboardNetProfit(c.rev, c.exp, c.vatPercent); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
	// same inputs as the revenue and total expense tests, every setting on
	rev := DashboardRevenue(RevenueInputs{Sales: 11500, SalesReturn: 1150, QtnSales: 2300, QtnSalesReturn: 230, NonVatSales: 900, NonVatSalesReturn: 100}, RevenueFlags{true, true}, 15)
	if got := DashboardNetProfit(rev.Total, 6263, 15).Profit; got != 6957 {
		t.Errorf("revenue − expense: %v", got)
	}
}

func TestDashboardNetProfit_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		if r := call(t, "GET", "/dashboard/net-profit?storeId=x", tok, nil); r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

func TestAPI_DashboardNetProfit(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "profit")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	setStore := func(m bson.M) {
		m["country_code"] = "SA"
		m["vat_percent"] = 15.0
		_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": m})
	}
	setStore(bson.M{"settings.non_vat_sales": false, "settings.enable_sales_in_quotation": false,
		"settings.disable_purchases_on_accounts": false, "settings.enable_employee_module": false})
	inOct := time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC) // 1 Oct 00:30 Saudi time
	inSep := time.Date(2026, 9, 30, 20, 30, 0, 0, time.UTC) // 30 Sep 23:30 Saudi time
	doc := func(d time.Time, extra bson.M) interface{} {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": d}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	ins := func(coll string, docs ...interface{}) {
		if _, err := storeDB(sid).Collection(coll).InsertMany(ctx, docs); err != nil {
			t.Fatalf("%s: %v", coll, err)
		}
	}
	ins("order", doc(inOct, bson.M{"net_total": 11500.0}), doc(inSep, bson.M{"net_total": 99999.0}))
	ins("salesreturn", doc(inOct, bson.M{"net_total": 1150.0}))
	ins("expense", doc(inOct, bson.M{"amount": 1000.0}))
	ins("purchase", doc(inOct, bson.M{"net_total": 2000.0}))
	ins("non_vat_sales", doc(inOct, bson.M{"net_total": 900.0}))
	ins("non_vat_sales_return", doc(inOct, bson.M{"net_total": 100.0}))

	url := "/dashboard/net-profit?storeId=" + sid + "&from=2026-10-01&to=2026-10-31"
	r := call(t, "GET", url, owner, nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
	// 10350 revenue − (1000 + 2000) expense
	if res := sub(r.Body, "result"); res["profit"] != 7350.0 || res["revenue"] != 10350.0 || res["expense"] != 3000.0 || res["profitable"] != true {
		t.Fatalf("non-VAT off: %v", res)
	}
	if rev := sub(sub(r.Body, "revenue"), "result"); rev["total"] != 10350.0 {
		t.Errorf("revenue part: %v", rev)
	}
	setStore(bson.M{"settings.non_vat_sales": true})
	r = call(t, "GET", url, owner, nil)
	// non-VAT net revenue 800 is added with the setting on
	if res := sub(r.Body, "result"); res["profit"] != 8150.0 || res["revenue"] != 11150.0 {
		t.Fatalf("non-VAT on: %v", res)
	}
	if r := call(t, "GET", "/dashboard/net-profit", owner, nil); r.Code != 400 {
		t.Errorf("missing storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/net-profit?storeId="+sid+"&from=2026-10-05&to=2026-10-01", owner, nil); r.Code != 400 {
		t.Errorf("to before from: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/net-profit?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("other store: %d", r.Code)
	}
}
