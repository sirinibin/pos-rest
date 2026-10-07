package erp

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Old business dashboard revenue card (business_dashboard/charts/KPICards.js).
func TestDashboardRevenue_Formula(t *testing.T) {
	in := RevenueInputs{Sales: 11500, SalesReturn: 1150, QtnSales: 2300, QtnSalesReturn: 230, NonVatSales: 900, NonVatSalesReturn: 100}
	cases := []struct {
		name  string
		f     RevenueFlags
		total float64
		rev   float64
	}{
		{"VAT sales only", RevenueFlags{}, 10350, 10350},
		{"with quotation sales", RevenueFlags{SalesInQuotation: true}, 12420, 12420},
		{"with non-VAT sales", RevenueFlags{NonVatSales: true}, 11150, 10350},
		{"both", RevenueFlags{true, true}, 13220, 12420},
	}
	for _, c := range cases {
		got := DashboardRevenue(in, c.f, 15)
		if got.Total != c.total || got.Revenue != c.rev {
			t.Errorf("%s: %+v", c.name, got)
		}
		// VAT is split from the VAT-bearing revenue only
		if got.Vat+got.RevenueWithoutVat != got.Revenue {
			t.Errorf("%s: vat split %+v", c.name, got)
		}
	}
	// a store's rate comes from dashboardVatPercent; 0 (no VAT) means no VAT
	if g := DashboardRevenue(RevenueInputs{Sales: 115}, RevenueFlags{}, 0); g.Vat != 0 || g.RevenueWithoutVat != 115 {
		t.Errorf("default VAT: %+v", g)
	}
	if g := DashboardRevenue(RevenueInputs{NonVatSales: 50}, RevenueFlags{}, 15); g.Total != 0 || g.NonVatNet != 0 {
		t.Errorf("non-VAT off: %+v", g)
	}
}

func TestDashboardRevenue_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		if r := call(t, "GET", "/dashboard/revenue?storeId=x", tok, nil); r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

func TestAPI_DashboardRevenue(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "rev")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	setStore := func(m bson.M) {
		m["country_code"] = "SA"
		_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": m})
	}
	setStore(bson.M{"settings.non_vat_sales": false, "settings.enable_sales_in_quotation": false})
	inOct := time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC)
	inSep := time.Date(2026, 9, 30, 20, 30, 0, 0, time.UTC)
	doc := func(d time.Time, net float64, extra ...bson.M) interface{} {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": d, "net_total": net}
		for _, e := range extra {
			for k, v := range e {
				m[k] = v
			}
		}
		return m
	}
	ins := func(coll string, docs ...interface{}) {
		if _, err := storeDB(sid).Collection(coll).InsertMany(ctx, docs); err != nil {
			t.Fatalf("%s: %v", coll, err)
		}
	}
	ins("order", doc(inOct, 11500), doc(inSep, 99999), doc(inOct, 5555, bson.M{"deleted": true}))
	ins("salesreturn", doc(inOct, 1150))
	ins("quotation", doc(inOct, 2300, bson.M{"type": "invoice"}), doc(inOct, 7777, bson.M{"type": "quotation"}))
	ins("quotation_sales_return", doc(inOct, 230))
	ins("non_vat_sales", doc(inOct, 900))
	ins("non_vat_sales_return", doc(inOct, 100))

	url := "/dashboard/revenue?storeId=" + sid + "&from=2026-10-01&to=2026-10-31"
	r := call(t, "GET", url, owner, nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
	if res := sub(r.Body, "result"); res["total"] != 10350.0 || res["revenueWithoutVat"] != 9000.0 {
		t.Fatalf("settings off: %v", res)
	}
	if in := sub(r.Body, "inputs"); in["nonVatSales"] != 0.0 || in["qtnSales"] != 0.0 {
		t.Errorf("non-VAT and quotation totals are only read when their settings are on: %v", in)
	}
	setStore(bson.M{"settings.non_vat_sales": true, "settings.enable_sales_in_quotation": true})
	r = call(t, "GET", url, owner, nil)
	if res := sub(r.Body, "result"); res["total"] != 13220.0 || res["revenue"] != 12420.0 || res["nonVatNet"] != 800.0 {
		t.Fatalf("settings on: %v", res)
	}
	if f := sub(r.Body, "flags"); f["nonVatSales"] != true || f["salesInQuotation"] != true {
		t.Errorf("flags %v", f)
	}
	if r := call(t, "GET", "/dashboard/revenue", owner, nil); r.Code != 400 {
		t.Errorf("missing storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/revenue?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("other store: %d", r.Code)
	}
}
