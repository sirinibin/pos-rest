package erp

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// a mapCtx with two warehouses and no database
func stockTestCtx() (*mapCtx, string, string) {
	st, w1, w2 := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	x := &mapCtx{storeHex: st.Hex(), cache: map[string]M{}, whLoaded: true,
		wh: []M{{"_id": w1, "code": "WH1"}, {"_id": w2, "code": "WH2"}}}
	return x, w1.Hex(), w2.Hex()
}

func TestHistoryWarehouseStocks(t *testing.T) {
	x, w1, w2 := stockTestCtx()
	ms := mainStoreWarehouseID(x.storeHex)
	got := historyWarehouseStocks(x, M{"main_store": 4.0, "WH1": 10.0, "WH2": -1.0, "gone": 7.0})
	if len(got) != 3 || num(got[ms]) != 4 || num(got[w1]) != 10 || num(got[w2]) != -1 {
		t.Fatalf("warehouse stocks %v", got)
	}
	if got := historyWarehouseStocks(x, nil); len(got) != 0 {
		t.Fatalf("no map → %v", got)
	}
	s := historyStockOf(x, M{"stock": 12.123456, "warehouse_stocks": M{"main_store": 12.123456}})
	if s.stock != 12.1235 || num(s.wh[ms]) != 12.1235 {
		t.Fatalf("stock of %v", s)
	}
}

func TestMatchHistoryStock(t *testing.T) {
	rec := func(n float64) historyStockRec { return historyStockRec{stock: n, wh: M{}} }
	rows := []M{{"docId": "a"}, {"docId": "a"}, {"docId": "a"}, {"docId": "b"}, {"docId": "c"}}
	left := matchHistoryStock(rows, map[string][]historyStockRec{"a": {rec(9), rec(7)}, "b": {rec(3)}})
	// the k-th line of a document gets its k-th record, extra lines the last one
	if rows[0]["stock"] != 9.0 || rows[1]["stock"] != 7.0 || rows[2]["stock"] != 7.0 || rows[3]["stock"] != 3.0 {
		t.Fatalf("rows %v", rows)
	}
	if len(left) != 1 || left[0]["docId"] != "c" || rows[4]["stock"] != nil {
		t.Fatalf("left %v", left)
	}
	if left := matchHistoryStock(nil, nil); len(left) != 0 {
		t.Fatalf("empty → %v", left)
	}
}

func TestHistorySign(t *testing.T) {
	for _, c := range []struct {
		ref    string
		qs     bool
		expect int
	}{
		{"purchase", false, 1}, {"sales_return", false, 1}, {"stock_adjustment_by_adding", false, 1},
		{"sales", false, -1}, {"purchase_return", false, -1}, {"stock_adjustment_by_removing", false, -1},
		{"quotation_invoice", false, 0}, {"quotation_invoice", true, -1},
		{"quotation_sales_return", false, 0}, {"quotation_sales_return", true, 1},
		{"stock_transfer", true, 0}, {"delivery_note", true, 0}, {"quotation", true, 0}, {"", false, 0},
	} {
		if got := historySign(c.ref, c.qs); got != c.expect {
			t.Errorf("%s (quotation stock %v) → %d, want %d", c.ref, c.qs, got, c.expect)
		}
	}
	// every legacy reference type maps to a kind the client knows
	for ref, kind := range historyRefKinds {
		if kind != "adjustment" && historyResource(kind) == nil {
			t.Errorf("%s → unknown kind %s", ref, kind)
		}
	}
}

func TestHistoryAllFilter(t *testing.T) {
	x, _, _ := stockTestCtx()
	x.store = M{"country_code": "SA"}
	pid := primitive.NewObjectID()
	f, err := historyAllFilter(x, pid, []string{"sales"}, " S-1 ", "2026-10-01", "2026-10-02")
	if err != nil {
		t.Fatal(err)
	}
	if f["product_id"] != pid || len(f["$or"].(bson.A)) != 6 {
		t.Fatalf("filter %v", f)
	}
	rng := f["date"].(bson.M)
	from, to := rng["$gte"].(time.Time), rng["$lt"].(time.Time)
	// whole days in the store's timezone (Riyadh, UTC+3)
	if from.UTC().Format(time.RFC3339) != "2026-09-30T21:00:00Z" || to.UTC().Format(time.RFC3339) != "2026-10-02T21:00:00Z" {
		t.Fatalf("range %v → %v", from.UTC(), to.UTC())
	}
	if f, _ := historyAllFilter(x, pid, []string{"sales"}, "", "", ""); f["date"] != nil || f["$or"] != nil {
		t.Fatalf("no q/from/to %v", f)
	}
	for _, bad := range [][2]string{{"nope", ""}, {"", "10/01/2026"}} {
		if _, err := historyAllFilter(x, pid, nil, "", bad[0], bad[1]); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	// a regex-looking search is literal
	f, _ = historyAllFilter(x, pid, nil, "a.b(", "", "")
	if p := f["$or"].(bson.A)[0].(bson.M)["reference_code"].(primitive.Regex).Pattern; p != `a\.b\(` {
		t.Fatalf("pattern %s", p)
	}
}

func TestHistoryAllRow(t *testing.T) {
	x, w1, _ := stockTestCtx()
	x.store = M{"country_code": "SA"}
	id, ref, cust := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	d := M{"_id": id, "reference_type": "sales", "reference_id": ref, "reference_code": "S-9", "date": time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC),
		"customer_id": cust, "customer_name": "Acme", "customer_name_arabic": "أكمي", "quantity": 3.0, "unit": "pcs",
		"unit_price": 100.0, "unit_discount": 10.0, "net_price": 270.0, "profit": 50.0, "loss": 5.0, "purchase_unit_price": 60.0,
		"warehouse_code": "WH1", "stock": 7.0, "warehouse_stocks": M{"main_store": 2.0, "WH1": 5.0}}
	r := historyAllRow(x, d, false)
	if r["id"] != id.Hex() || r["kind"] != "sales" || r["docId"] != ref.Hex() || r["code"] != "S-9" || r["date"] != "2026-10-01T10:00" {
		t.Fatalf("row %v", r)
	}
	if r["partyName"] != "Acme" || r["partyNameAr"] != "أكمي" || r["partyId"] != cust.Hex() || r["change"] != -3.0 || r["qty"] != 3.0 {
		t.Fatalf("party/qty %v", r)
	}
	if r["price"] != 90.0 || r["total"] != 270.0 || r["profit"] != 45.0 || r["warehouseId"] != w1 || r["stock"] != 7.0 {
		t.Fatalf("money/stock %v", r)
	}
	if ws := r["warehouseStocks"].(M); num(ws[w1]) != 5 || num(ws[mainStoreWarehouseID(x.storeHex)]) != 2 {
		t.Fatalf("warehouse stocks %v", ws)
	}
	// a purchase names the vendor and its cost; an adjustment has no document
	v := primitive.NewObjectID()
	p := historyAllRow(x, M{"reference_type": "purchase", "vendor_id": v, "vendor_name": "Gulf", "quantity": 10.0,
		"purchase_unit_price": 80.0, "stock": 17.0}, false)
	if p["partyName"] != "Gulf" || p["partyId"] != v.Hex() || p["price"] != 80.0 || p["change"] != 10.0 || p["kind"] != "purchases" {
		t.Fatalf("purchase %v", p)
	}
	a := historyAllRow(x, M{"reference_type": "stock_adjustment_by_removing", "quantity": 2.0, "reason": "damaged", "stock": 4.0}, false)
	if a["docId"] != nil || a["kind"] != "adjustment" || a["change"] != -2.0 || a["reason"] != "damaged" ||
		a["warehouseId"] != mainStoreWarehouseID(x.storeHex) {
		t.Fatalf("adjustment %v", a)
	}
	tr := historyAllRow(x, M{"reference_type": "stock_transfer", "quantity": 1.0, "to_warehouse_code": "WH1"}, false)
	if tr["fromWarehouseId"] != mainStoreWarehouseID(x.storeHex) || tr["toWarehouseId"] != w1 || tr["change"] != 0.0 {
		t.Fatalf("transfer %v", tr)
	}
	if q := historyAllRow(x, M{"reference_type": "quotation_invoice", "quantity": 1.0}, true); q["change"] != -1.0 || q["kind"] != "quotations" {
		t.Fatalf("quotation invoice %v", q)
	}
}

func TestProductHistoryAll_Unauthenticated(t *testing.T) {
	r := call(t, "GET", "/products/abc/history?kind=all&storeId=x", "", nil)
	if r.Code != http.StatusUnauthorized {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
}

// seedStockHistory writes a legacy product_history for product pid:
// opening 5 (day 70), purchase P-INV-001 +10 → 15 (day 60), sale S-INV-001 −2 → 13 (day 30).
func seedStockHistory(t *testing.T, pid primitive.ObjectID) {
	t.Helper()
	n := time.Now()
	day := func(d int) time.Time { return n.AddDate(0, 0, -d) }
	recs := []interface{}{
		bson.M{"_id": primitive.NewObjectID(), "store_id": fx.StoreA, "product_id": pid, "date": day(70),
			"reference_type": "stock_adjustment_by_adding", "quantity": 5.0, "reason": "opening", "stock": 5.0,
			"warehouse_stocks": bson.M{"main_store": 5.0}},
		bson.M{"_id": primitive.NewObjectID(), "store_id": fx.StoreA, "product_id": pid, "date": day(60),
			"reference_type": "purchase", "reference_id": fx.PurchaseA1, "reference_code": "P-INV-001", "vendor_id": fx.VendorA1,
			"vendor_name": "Gulf Lubricants", "quantity": 10.0, "purchase_unit_price": 80.0, "net_price": 800.0, "stock": 15.0,
			"warehouse_code": "WH1", "warehouse_stocks": bson.M{"main_store": 5.0, "WH1": 10.0}},
		bson.M{"_id": primitive.NewObjectID(), "store_id": fx.StoreA, "product_id": pid, "date": day(30),
			"reference_type": "sales", "reference_id": fx.OrderA1, "reference_code": "S-INV-001", "customer_id": fx.CustomerA1,
			"customer_name": "Riyadh Motors", "quantity": 2.0, "unit_price": 120.0, "net_price": 240.0, "stock": 13.0,
			"warehouse_stocks": bson.M{"main_store": 3.0, "WH1": 10.0}},
	}
	col := storeDB(storeA()).Collection(historyColl)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := col.InsertMany(ctx, recs); err != nil {
		t.Fatal(err)
	}
	ids := bson.A{}
	for _, r := range recs {
		ids = append(ids, r.(bson.M)["_id"])
	}
	t.Cleanup(func() {
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = col.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": ids}})
	})
}

func TestAPI_ProductHistoryStock(t *testing.T) {
	requireDB(t)
	seedStockHistory(t, fx.ProductA1)
	fresh := primitive.NewObjectID()
	seedStockHistory(t, fresh)
	tok := login(t, fx.ManagerEmail)
	base := "/products/" + fx.ProductA1.Hex() + "/history?storeId=" + storeA()
	ms, wh := mainStoreWarehouseID(storeA()), fx.WarehouseA.Hex()
	// each document row: the stock after it, from its own product_history record
	s := call(t, "GET", base+"&kind=sales&q=S-INV-001", tok, nil)
	if s.Code != 200 || len(s.data()) != 1 {
		t.Fatalf("sales: %d %s", s.Code, s.Raw)
	}
	row := s.data()[0].(M)
	if num(row["stock"]) != 13 || num(sub(row, "warehouseStocks")[ms]) != 3 || num(sub(row, "warehouseStocks")[wh]) != 10 {
		t.Fatalf("sales row stock %v", row)
	}
	p := call(t, "GET", base+"&kind=purchases&q=P-INV-001", tok, nil)
	if p.Code != 200 || len(p.data()) != 1 || num(p.data()[0].(M)["stock"]) != 15 {
		t.Fatalf("purchase stock: %s", p.Raw)
	}
	// no record of its own (a plain quotation, day 40): the stock at its date
	q := call(t, "GET", base+"&kind=quotations&q=QTN-001", tok, nil)
	if q.Code != 200 || len(q.data()) != 1 || num(q.data()[0].(M)["stock"]) != 15 {
		t.Fatalf("quotation stock: %s", q.Raw)
	}
	// a product without any history: stock 0 on every row, never missing
	o := call(t, "GET", "/products/"+fx.ProductA2.Hex()+"/history?storeId="+storeA()+"&kind=stockTransfers", tok, nil)
	if o.Code != 200 || len(o.data()) < 1 || o.data()[0].(M)["stock"] == nil {
		t.Fatalf("no history: %s", o.Raw)
	}
	// kind=all: every record, newest first, with the stock after it
	base = "/products/" + fresh.Hex() + "/history?storeId=" + storeA()
	a := call(t, "GET", base+"&kind=all", tok, nil)
	if a.Code != 200 || a.Body["kind"] != "all" || num(a.Body["total"]) != 3 || len(a.data()) != 3 {
		t.Fatalf("all: %d %s", a.Code, a.Raw)
	}
	rows := a.data()
	first, last := rows[0].(M), rows[2].(M)
	if first["kind"] != "sales" || num(first["stock"]) != 13 || num(first["change"]) != -2 || first["docId"] != fx.OrderA1.Hex() ||
		last["kind"] != "adjustment" || num(last["stock"]) != 5 || last["docId"] != nil || last["reason"] != "opening" {
		t.Fatalf("all rows %v", rows)
	}
	if sums := sub(a.Body, "sums"); num(sums["in"]) != 15 || num(sums["out"]) != 2 || num(sums["qty"]) != 13 || num(sums["lines"]) != 3 {
		t.Fatalf("all sums %v", sums)
	}
	if pg := call(t, "GET", base+"&kind=all&limit=1&page=2", tok, nil); len(pg.data()) != 1 || pg.data()[0].(M)["kind"] != "purchases" {
		t.Fatalf("all page 2: %s", pg.Raw)
	}
	if f := call(t, "GET", base+"&kind=all&q=riyadh", tok, nil); num(f.Body["total"]) != 1 {
		t.Fatalf("all q: %s", f.Raw)
	}
	if f := call(t, "GET", base+"&kind=all&to="+time.Now().AddDate(0, 0, -50).Format("2006-01-02"), tok, nil); num(f.Body["total"]) != 2 {
		t.Fatalf("all to: %s", f.Raw)
	}
	// validation and access
	if bad := call(t, "GET", base+"&kind=all&from=nope", tok, nil); bad.Code != 400 || bad.errField("from") == "" {
		t.Fatalf("bad from: %d %s", bad.Code, bad.Raw)
	}
	if bad := call(t, "GET", base+"&kind=all&page=0", tok, nil); bad.Code != 400 {
		t.Fatalf("bad page: %d", bad.Code)
	}
	if bad := call(t, "GET", "/products/"+fx.ProductA1.Hex()+"/history?kind=all", tok, nil); bad.Code != 400 {
		t.Fatalf("no store: %d", bad.Code)
	}
	if c := call(t, "GET", base+"&kind=all&limit=1000", tok, nil); num(c.Body["limit"]) != historyMaxLimit {
		t.Fatalf("limit cap: %s", c.Raw)
	}
	if o := call(t, "GET", base+"&kind=all", login(t, fx.UserBEmail), nil); o.Code != 403 {
		t.Fatalf("other store's user: %d", o.Code)
	}
}
