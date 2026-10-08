package erp

import (
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestHistoryResourceKinds(t *testing.T) {
	for kind, name := range productHistoryKinds {
		r := historyResource(kind)
		if r == nil || r.Name != name {
			t.Fatalf("kind %s → %v", kind, r)
		}
		if _, ok := r.Backend.(*legacyBackend); !ok {
			t.Errorf("kind %s is not a legacy document list", kind)
		}
		if r.Module == "" {
			t.Errorf("kind %s has no permission module", kind)
		}
	}
	for _, bad := range []string{"", "products", "Sales", "stockTransfers", "sales "} {
		if historyResource(bad) != nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestHistoryKeys(t *testing.T) {
	for _, c := range []struct{ kind, price, cost string }{
		{"sales", "unit_price", "purchase_unit_price"},
		{"quotations", "unit_price", "purchase_unit_price"},
		{"purchases", "purchase_unit_price", ""},
		{"purchaseReturns", "purchasereturn_unit_price", "purchase_unit_price"},
	} {
		p, k := historyKeys(c.kind)
		if p != c.price || k != c.cost {
			t.Errorf("%s → %s / %s", c.kind, p, k)
		}
	}
}

func TestHistoryRows(t *testing.T) {
	rec := M{"id": "d1", "code": "S-1", "date": "2026-10-01T10:00", "customerId": "c1", "customerName": "Acme",
		"customerNameAr": "أكمي", "paymentStatus": "paid", "items": []interface{}{
			M{"productId": "p1", "qty": 2.0, "unitPrice": 100.0, "unitDiscount": 10.0, "purchasePrice": 60.0, "unit": "pcs", "vatPercent": 15.0},
			M{"productId": "p2", "qty": 5.0, "unitPrice": 1.0},
			M{"productId": "p1", "qty": 1.0, "unitPrice": 50.0, "purchasePrice": 60.0, "qtyReturned": 1.0},
		}}
	rows := historyRows(rec, "p1")
	if len(rows) != 2 {
		t.Fatalf("rows %v", rows)
	}
	r := rows[0]
	if r["id"] != "d1:0" || r["docId"] != "d1" || r["partyName"] != "Acme" || r["partyNameAr"] != "أكمي" || r["partyId"] != "c1" {
		t.Fatalf("row 0 %v", r)
	}
	if r["price"] != 90.0 || r["total"] != 180.0 || r["cost"] != 120.0 || r["profit"] != 60.0 || r["paymentStatus"] != "paid" {
		t.Fatalf("row 0 money %v", r)
	}
	if rows[1]["id"] != "d1:2" || rows[1]["profit"] != -10.0 || rows[1]["qtyReturned"] != 1.0 {
		t.Fatalf("row 1 %v", rows[1])
	}
	// vendor documents name the vendor
	v := historyRows(M{"id": "p", "vendorId": "v1", "vendorName": "Gulf", "vendorInvoiceNo": "GL-1",
		"items": []interface{}{M{"productId": "p1", "qty": 3.0, "unitPrice": 7.0}}}, "p1")
	if len(v) != 1 || v[0]["partyName"] != "Gulf" || v[0]["partyId"] != "v1" || v[0]["vendorInvoiceNo"] != "GL-1" || v[0]["total"] != 21.0 {
		t.Fatalf("vendor row %v", v)
	}
	if got := historyRows(M{"id": "x"}, "p1"); len(got) != 0 {
		t.Fatalf("no lines → %v", got)
	}
}

func TestHistorySumsPipeline(t *testing.T) {
	pid := primitive.NewObjectID()
	p := historySumsPipeline(bson.M{"store_id": 1}, pid, "purchases")
	if len(p) != 5 || p[0].(bson.M)["$match"].(bson.M)["store_id"] != 1 || p[2].(bson.M)["$unwind"] != "$products" {
		t.Fatalf("pipeline %v", p)
	}
	if p[3].(bson.M)["$match"].(bson.M)["products.product_id"] != pid {
		t.Fatalf("line match %v", p[3])
	}
	g := p[4].(bson.M)["$group"].(bson.M)
	for _, k := range []string{"qty", "value", "cost", "lines"} {
		if g[k] == nil {
			t.Errorf("group misses %s", k)
		}
	}
	s := historySums(bson.M{"qty": 3.0, "value": 300.004, "cost": 100.0, "lines": int32(2)})
	if s["qty"] != 3.0 || s["value"] != 300.0 || s["profit"] != 200.0 || s["lines"] != int64(2) {
		t.Fatalf("sums %v", s)
	}
	if z := historySums(bson.M{}); z["qty"] != 0.0 || z["lines"] != int64(0) {
		t.Fatalf("empty sums %v", z)
	}
}

func TestProductHistory_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		r := call(t, "GET", "/products/abc/history?kind=sales&storeId=x", tok, nil)
		if r.Code != http.StatusUnauthorized || r.errCode() == "" {
			t.Errorf("token %q: %d %s", tok, r.Code, r.Raw)
		}
	}
}

func TestAPI_ProductHistory(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	base := "/products/" + fx.ProductA1.Hex() + "/history?storeId=" + storeA()
	// sales: the fixture invoice S-INV-001 sells 2 × 120 (cost 80)
	r := call(t, "GET", base+"&kind=sales&q=S-INV-001", tok, nil)
	if r.Code != 200 {
		t.Fatalf("sales: %d %s", r.Code, r.Raw)
	}
	if num(r.Body["total"]) != 1 || len(r.data()) != 1 || r.Body["kind"] != "sales" || num(r.Body["limit"]) != historyDefaultLimit {
		t.Fatalf("sales body %s", r.Raw)
	}
	row := r.data()[0].(M)
	if row["code"] != "S-INV-001" || num(row["qty"]) != 2 || num(row["total"]) != 240 || num(row["profit"]) != 80 ||
		row["partyName"] != "Riyadh Motors" || row["docId"] != fx.OrderA1.Hex() {
		t.Fatalf("sales row %v", row)
	}
	if s := r.Body["sums"].(M); num(s["qty"]) != 2 || num(s["value"]) != 240 || num(s["cost"]) != 160 || num(s["lines"]) != 1 {
		t.Fatalf("sales sums %v", s)
	}
	// every sale of the product, newest first, a page at a time
	all := call(t, "GET", base+"&kind=sales&limit=1", tok, nil)
	if all.Code != 200 || len(all.data()) != 1 || num(all.Body["total"]) < 1 {
		t.Fatalf("page 1: %d %s", all.Code, all.Raw)
	}
	if n := int(num(all.Body["total"])); n > 1 {
		p2 := call(t, "GET", base+"&kind=sales&limit=1&page=2", tok, nil)
		if len(p2.data()) != 1 || str(p2.data()[0].(M)["date"]) > str(all.data()[0].(M)["date"]) {
			t.Fatalf("page 2 not older: %s / %s", p2.Raw, all.Raw)
		}
	}
	// the product sold on another document only: not in this product's history
	other := call(t, "GET", "/products/"+fx.ProductA2.Hex()+"/history?storeId="+storeA()+"&kind=sales&q=S-INV-001", tok, nil)
	if other.Code != 200 || num(other.Body["total"]) != 0 || len(other.data()) != 0 {
		t.Fatalf("other product: %s", other.Raw)
	}
	// quotations and purchases
	q := call(t, "GET", base+"&kind=quotations&q=QTN-001", tok, nil)
	if q.Code != 200 || len(q.data()) != 1 || num(q.data()[0].(M)["qty"]) != 5 || q.data()[0].(M)["status"] == nil {
		t.Fatalf("quotations: %d %s", q.Code, q.Raw)
	}
	p := call(t, "GET", base+"&kind=purchases&q=P-INV-001", tok, nil)
	if p.Code != 200 || len(p.data()) != 1 {
		t.Fatalf("purchases: %d %s", p.Code, p.Raw)
	}
	pr := p.data()[0].(M)
	if num(pr["qty"]) != 10 || num(pr["price"]) != 80 || pr["partyName"] != "Gulf Lubricants" || pr["vendorInvoiceNo"] != "GL-77" {
		t.Fatalf("purchase row %v", pr)
	}
	if s := p.Body["sums"].(M); num(s["value"]) != 800 || num(s["qty"]) != 10 {
		t.Fatalf("purchase sums %v", s)
	}
	// limit is capped
	if c := call(t, "GET", base+"&kind=sales&limit=1000", tok, nil); num(c.Body["limit"]) != historyMaxLimit {
		t.Fatalf("limit cap: %s", c.Raw)
	}
	// validation and access
	if bad := call(t, "GET", base+"&kind=nope", tok, nil); bad.Code != 400 || bad.errField("kind") == "" {
		t.Fatalf("bad kind: %d %s", bad.Code, bad.Raw)
	}
	if bad := call(t, "GET", "/products/not-an-id/history?storeId="+storeA()+"&kind=sales", tok, nil); bad.Code != 404 {
		t.Fatalf("bad id: %d", bad.Code)
	}
	if bad := call(t, "GET", "/products/"+fx.ProductA1.Hex()+"/history?kind=sales", tok, nil); bad.Code != 400 {
		t.Fatalf("no store: %d", bad.Code)
	}
	if bad := call(t, "GET", base+"&kind=sales&from=nope", tok, nil); bad.Code != 400 {
		t.Fatalf("bad from: %d", bad.Code)
	}
	if o := call(t, "GET", base+"&kind=sales", login(t, fx.UserBEmail), nil); o.Code != 403 {
		t.Fatalf("other store's user: %d", o.Code)
	}
}
