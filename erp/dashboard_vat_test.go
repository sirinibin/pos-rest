package erp

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Old business dashboard Overall Summary "VAT":
// (vatPrice - salesReturnVatPrice) - (purchaseVatPrice - purchaseReturnVatPrice)
func TestDashboardVat_Formula(t *testing.T) {
	cases := []struct {
		name string
		in   DashboardVatInputs
		want DashboardVatResult
	}{
		{"payable", DashboardVatInputs{1500, 150, 600, 60}, DashboardVatResult{1350, 540, 810}},
		{"refundable", DashboardVatInputs{100, 0, 900, 0}, DashboardVatResult{100, 900, -800}},
		{"nothing", DashboardVatInputs{}, DashboardVatResult{}},
		{"rounding", DashboardVatInputs{SalesVat: 0.105, PurchaseVat: 0.001}, DashboardVatResult{0.11, 0, 0.1}},
	}
	for _, c := range cases {
		if got := DashboardVat(c.in); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestDashboardVat_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		if r := call(t, "GET", "/dashboard/vat?storeId=x", tok, nil); r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

func TestAPI_DashboardVat(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "vat")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"country_code": "SA"}})
	sdb := storeDB(sid)
	inOct := time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC) // 1 Oct 00:30 Saudi time
	inSep := time.Date(2026, 9, 30, 20, 30, 0, 0, time.UTC) // 30 Sep 23:30 Saudi time
	doc := func(d time.Time, vat float64, extra ...bson.M) interface{} {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": d, "vat_price": vat}
		for _, e := range extra {
			for k, v := range e {
				m[k] = v
			}
		}
		return m
	}
	ins := func(coll string, docs ...interface{}) {
		if _, err := sdb.Collection(coll).InsertMany(ctx, docs); err != nil {
			t.Fatalf("%s: %v", coll, err)
		}
	}
	ins("order", doc(inOct, 1500), doc(inSep, 9999), doc(inOct, 7777, bson.M{"deleted": true}))
	ins("salesreturn", doc(inOct, 150))
	ins("purchase", doc(inOct, 600))
	ins("purchasereturn", doc(inOct, 60))
	ins("expense", doc(inOct, 0, bson.M{"amount": 115.0, "vat_amount": 15.0})) // not part of the old figure

	r := call(t, "GET", "/dashboard/vat?storeId="+sid+"&from=2026-10-01&to=2026-10-31", owner, nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
	res := sub(r.Body, "result")
	if res["outVat"] != 1350.0 || res["inVat"] != 540.0 || res["vatPayable"] != 810.0 {
		t.Fatalf("result %v", res)
	}
	if r := call(t, "GET", "/dashboard/vat?storeId="+sid, owner, nil); sub(r.Body, "inputs")["salesVat"] != 11499.0 {
		t.Errorf("all-time sales VAT %v", sub(r.Body, "inputs")["salesVat"])
	}
	if r := call(t, "GET", "/dashboard/vat", owner, nil); r.Code != 400 {
		t.Errorf("missing storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/vat?storeId="+sid+"&from=2026-10-09&to=2026-10-01", owner, nil); r.Code != 400 {
		t.Errorf("reversed range: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/vat?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("other store: %d", r.Code)
	}
}
