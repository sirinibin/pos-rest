package erp

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---- pure functions ----

func TestComputeTotals(t *testing.T) {
	inv := M{"vatPercent": 15.0, "discount": 5.0, "shipping": 10.0, "cashDiscount": 2.0,
		"items": []interface{}{
			M{"qty": 2.0, "unitPrice": 100.0, "unitDiscount": 5.0, "purchasePrice": 60.0},
			M{"qty": "1.5", "unitPrice": "40", "purchasePrice": 25.0},
		},
		"payments": []interface{}{M{"amount": 100.0}, M{"amount": "50.5"}}}
	with := func(k string, v interface{}) M {
		m := M{}
		for kk, vv := range inv {
			m[kk] = vv
		}
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name string
		rec  M
		want FeedTotals
	}{
		// subtotal 260, item discount 10, discount 5, shipping 10 → taxable 255, VAT 38.25
		{"invoice", inv, FeedTotals{Taxable: 255, Vat: 38.25, Net: 293.25, Paid: 150.5, Balance: 140.75, Status: "paid_partially", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"auto rounding to 0.05", with("roundingAuto", true), FeedTotals{Taxable: 255, Vat: 38.25, Net: 293.25, Paid: 150.5, Balance: 140.75, Status: "paid_partially", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"manual rounding", with("rounding", -0.25), FeedTotals{Taxable: 255, Vat: 38.25, Net: 293, Paid: 150.5, Balance: 140.5, Status: "paid_partially", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"VAT percent missing = 15", with("vatPercent", nil), FeedTotals{Taxable: 255, Vat: 38.25, Net: 293.25, Paid: 150.5, Balance: 140.75, Status: "paid_partially", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"zero VAT", with("vatPercent", 0.0), FeedTotals{Taxable: 255, Vat: 0, Net: 255, Paid: 150.5, Balance: 102.5, Status: "paid_partially", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"overpaid → paid, balance 0", with("payments", []interface{}{M{"amount": 400.0}}), FeedTotals{Taxable: 255, Vat: 38.25, Net: 293.25, Paid: 400, Balance: 0, Status: "paid", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"unpaid", with("payments", nil), FeedTotals{Taxable: 255, Vat: 38.25, Net: 293.25, Paid: 0, Balance: 291.25, Status: "not_paid", Profit: 87.5, Cost: 157.5, Qty: 3.5}},
		{"empty", M{}, FeedTotals{Status: "paid"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ComputeTotals(c.rec); got != c.want {
				t.Errorf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
	// auto rounding rounds half up like Math.round (net 10.725 → 10.75)
	r := ComputeTotals(M{"vatPercent": 0.0, "roundingAuto": true, "items": []interface{}{M{"qty": 1.0, "unitPrice": 10.725}}})
	if r.Net != 10.75 {
		t.Errorf("auto rounding: %v", r.Net)
	}
}

func TestWallClock(t *testing.T) {
	cases := []struct {
		in        string
		hour, dow int // -1 = null
	}{
		{"2026-10-06T14:35", 14, 2},
		{"2026-10-04T00:00", 0, 0},
		{"2026-10-10T23:59:59", 23, 6},
		{"2024-02-29T08:00", 8, 4},
		{"2026-02-30T08:00", -1, -1},
		{"2026-10-06T25:00", -1, -1},
		{"", -1, -1},
		{"undefined", -1, -1},
		{"2026-10-06", 0, 2},
	}
	for _, c := range cases {
		h, d := wallClock(c.in)
		gh, gd := -1, -1
		if h != nil {
			gh = *h
		}
		if d != nil {
			gd = *d
		}
		if gh != c.hour || gd != c.dow {
			t.Errorf("%q: got %d/%d want %d/%d", c.in, gh, gd, c.hour, c.dow)
		}
	}
}

func TestFeedWindowStart(t *testing.T) {
	riy, _ := time.LoadLocation("Asia/Riyadh")
	// 22:30 UTC on 6 Oct is already 7 Oct in Riyadh
	day, at := FeedWindowStart(time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC), riy)
	if day != "2025-10-07" || !at.Equal(time.Date(2025, 10, 6, 21, 0, 0, 0, time.UTC)) {
		t.Errorf("got %s %s", day, at.UTC())
	}
	day, _ = FeedWindowStart(time.Date(2026, 10, 6, 20, 59, 0, 0, time.UTC), nil) // nil = Riyadh
	if day != "2025-10-06" {
		t.Errorf("still 6 Oct in Riyadh: %s", day)
	}
}

func TestDashboardFeedAdd(t *testing.T) {
	f := NewDashboardFeed()
	f.Add("purchaseRequests", M{"status": "pending"})
	f.Add("purchaseRequests", M{"status": "approved"})
	f.Add("purchaseOrders", M{"status": "draft"})
	f.Add("purchaseOrders", M{"status": "draft"})
	f.Add("nothing", M{"status": "draft"})
	if f.Counts["pendingPurchaseRequests"] != int64(1) || f.Counts["draftPurchaseOrders"] != int64(2) {
		t.Errorf("counts %v", f.Counts)
	}
	f.Add("salesReturns", M{"id": "r", "orderId": "s1", "remarks": "damaged"})
	f.Add("quotations", M{"id": "q", "status": "accepted", "orderIds": []interface{}{"s1", ""}})
	if f.SalesReturns[0].OrderID != "s1" || f.SalesReturns[0].Remarks != "damaged" {
		t.Errorf("return %+v", f.SalesReturns[0])
	}
	if q := f.Quotations[0]; q.DocStatus != "accepted" || !reflect.DeepEqual(q.OrderIDs, []string{"s1"}) {
		t.Errorf("quotation %+v", q)
	}
	f.Add("customers", M{"id": "c", "nameEn": "A", "history": []interface{}{M{}}, "address": "x", "phone": nil})
	if !reflect.DeepEqual(f.Customers[0], M{"id": "c", "nameEn": "A"}) {
		t.Errorf("customer trimmed to the dashboard's fields: %v", f.Customers[0])
	}
	b, _ := json.Marshal(NewDashboardFeed())
	if strings.Contains(string(b), "items") {
		t.Errorf("documents carry no invoice lines: %s", b)
	}
	if strings.Contains(string(b), "null") {
		t.Errorf("an empty feed has every list, never null: %s", b)
	}
}

// Invoice lines become one row per day and product: sales and non-VAT sales add to the
// sold figures, returns to the returned ones; documents carry no lines.
func TestDashboardFeed_ProductDays(t *testing.T) {
	f := NewDashboardFeed()
	line := func(p string, q, price, disc, cost interface{}) M {
		return M{"productId": p, "qty": q, "unitPrice": price, "unitDiscount": disc, "purchasePrice": cost}
	}
	f.Add("sales", M{"id": "s1", "date": "2026-03-01T09:00", "items": []interface{}{line("p", 2, 10.5, 0.5, 4), line("q", 1, 7, nil, nil)}})
	f.Add("nonvatSales", M{"id": "n1", "date": "2026-03-01T18:00", "items": []interface{}{line("p", "3", "10", 0, 4)}})
	f.Add("salesReturns", M{"id": "r1", "date": "2026-03-01T20:00", "items": []interface{}{line("p", 1, 10, 0, 4)}})
	f.Add("nonvatReturns", M{"id": "r2", "date": "2026-03-02T08:00", "items": []interface{}{line("p", 1, 10, 0, 4)}})
	f.Add("purchases", M{"id": "b1", "date": "2026-03-01T08:00", "items": []interface{}{line("p", 50, 4, 0, 4)}})
	want := []FeedProductDay{
		{D: "2026-03-01", ProductID: "p", Qty: 5, Rev: 50, Cost: 20, Lines: 2, RetQty: 1, RetRev: 10},
		{D: "2026-03-01", ProductID: "q", Qty: 1, Rev: 7, Lines: 1},
		{D: "2026-03-02", ProductID: "p", RetQty: 1, RetRev: 10},
	}
	if !reflect.DeepEqual(f.ProductDays, want) {
		t.Errorf("productDays\n got %+v\nwant %+v", f.ProductDays, want)
	}
	// parts loaded in parallel merge into the same rows
	m := NewDashboardFeed()
	a, b := NewDashboardFeed(), NewDashboardFeed()
	a.Add("sales", M{"date": "2026-03-01T09:00", "items": []interface{}{line("p", 2, 10, 0, 4)}})
	b.Add("salesReturns", M{"date": "2026-03-01T10:00", "items": []interface{}{line("p", 1, 10, 0, 4)}})
	b.Add("salesReturns", M{"date": "2026-03-02T10:00", "items": []interface{}{line("p", 1, 10, 0, 4)}})
	m.mergeProductDays(a.ProductDays)
	m.mergeProductDays(b.ProductDays)
	if !reflect.DeepEqual(m.ProductDays, []FeedProductDay{
		{D: "2026-03-01", ProductID: "p", Qty: 2, Rev: 20, Cost: 8, Lines: 1, RetQty: 1, RetRev: 10},
		{D: "2026-03-02", ProductID: "p", RetQty: 1, RetRev: 10},
	}) {
		t.Errorf("merged %+v", m.ProductDays)
	}
	if b, _ := json.Marshal(f.Sales[0]); strings.Contains(string(b), "items") {
		t.Errorf("a document carries no lines: %s", b)
	}
}

// The same fixture is checked against the JavaScript builder in starterp-frontend-v1
// (tests/unit/dashboardFeed.test.js), so the server reduces documents exactly like the
// browser did.
func TestDashboardFeed_ParityWithJS(t *testing.T) {
	_, here, _, _ := runtime.Caller(0) // DB-backed runs change the working directory
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "testdata", "dashboard_feed_parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		From     string                   `json:"from"`
		Records  map[string][]interface{} `json:"records"`
		Expected interface{}              `json:"expected"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	dateField := map[string]string{"salaries": "paymentDate"}
	f := NewDashboardFeed()
	for _, name := range append(append([]string{}, feedWindowed...), feedMasters...) {
		for _, r := range fx.Records[name] {
			rec := normDoc(r)
			// what the database query does on the server: live records in the window
			df := dateField[name]
			if df == "" {
				df = "date"
			}
			if boolv(rec["deleted"]) || (!contains(feedMasters, name) && first10(str(rec[df])) < fx.From) {
				continue
			}
			f.Add(name, rec)
		}
	}
	b, _ := json.Marshal(f)
	var got interface{}
	_ = json.Unmarshal(b, &got)
	if !reflect.DeepEqual(got, fx.Expected) {
		gm, wm := got.(map[string]interface{}), fx.Expected.(map[string]interface{})
		for k := range wm {
			if !reflect.DeepEqual(gm[k], wm[k]) {
				gb, _ := json.Marshal(gm[k])
				wb, _ := json.Marshal(wm[k])
				t.Errorf("%s differs:\n go %s\n js %s", k, truncate(string(gb), 1500), truncate(string(wb), 1500))
			}
		}
	}
}

func TestFingerprintable(t *testing.T) {
	for name, want := range map[string]bool{
		"order": true, "salesreturn": true, "sales_payment": true, "product": true, "store": true, "posting": true, "account": true,
		"product_sales_history": false, "order_history": false, "zatca_logs": false, "whatsapp_messages": false,
		"order_draft": false, "dashboard_monthly": false, "erp_idempotency": false, "system.views": false, "rfq_received": false,
	} {
		if got := fingerprintable(name); got != want {
			t.Errorf("%s: got %v", name, got)
		}
	}
}

func TestSnapKeyAndParams(t *testing.T) {
	k := &dashKind{Name: "vat", Params: []string{"from", "to"}}
	q := map[string][]string{"to": {"2026-10-31"}, "from": {" 2026-10-01 "}, "storeId": {"s"}, "fresh": {"1"}, "x": {"y"}}
	key, params, kq := snapKey("s", k, q)
	if key != "s|vat|from=2026-10-01&to=2026-10-31" || params != "from=2026-10-01&to=2026-10-31" || kq.Get("x") != "" || kq.Get("fresh") != "" {
		t.Errorf("%s / %s / %v", key, params, kq)
	}
	if key, _, _ := snapKey("s", &dashKind{Name: "bi"}, q); key != "s|bi|" {
		t.Errorf("no params: %s", key)
	}
	// a format revision is part of the key, so older saved snapshots are never served
	fk := registerDashKind(&dashKind{Name: "test-rev", Rev: 2})
	defer delete(dashKinds, "test-rev")
	if key, _, _ := snapKey("s", fk, q); key != "s|test-rev@2|" || kindOfKey(key) != fk {
		t.Errorf("revision key: %s", key)
	}
	for _, old := range []string{"s|test-rev|", "s|test-rev@1|", "s|gone|", "bad"} {
		if kindOfKey(old) != nil {
			t.Errorf("%s: an older format or unknown kind has no kind", old)
		}
	}
	if kindOfKey("s|vat|from=2026-10-01") != dashKinds["vat"] && dashKinds["vat"] != nil {
		t.Errorf("vat key")
	}
	for _, c := range []struct {
		from, to string
		ok       bool
	}{{"", "", true}, {"2026-10-01", "2026-10-31", true}, {"2026-10-01", "", true}, {"x", "", false}, {"", "2026-13-01", false}, {"2026-10-09", "2026-10-01", false}} {
		err := validateDashParams(k, map[string][]string{"from": {c.from}, "to": {c.to}})
		if (err == nil) != c.ok {
			t.Errorf("%q..%q: %v", c.from, c.to, err)
		}
	}
}

type hdrRecorder struct {
	h      http.Header
	status int
	body   []byte
}

func (r *hdrRecorder) Header() http.Header         { return r.h }
func (r *hdrRecorder) Write(b []byte) (int, error) { r.body = append(r.body, b...); return len(b), nil }
func (r *hdrRecorder) WriteHeader(s int)           { r.status = s }

func TestWriteSnap(t *testing.T) {
	riy, _ := time.LoadLocation("Asia/Riyadh")
	s := &dashSnap{Body: []byte(`{"a":1}`), GeneratedAt: time.Date(2026, 10, 6, 20, 0, 0, 0, time.UTC), BuildMs: 12, loc: riy}
	req, _ := http.NewRequest("GET", "/", nil)
	w := &hdrRecorder{h: http.Header{}}
	_ = writeSnap(w, req, s, false)
	var got M
	if err := json.Unmarshal(w.body, &got); err != nil {
		t.Fatalf("%s: %v", w.body, err)
	}
	if got["a"] != 1.0 || sub(got, "snapshot")["generatedAt"] != "2026-10-06T23:00:00" || sub(got, "snapshot")["stale"] != false ||
		sub(got, "snapshot")["buildMs"] != 12.0 || sub(got, "snapshot")["version"] == "" {
		t.Errorf("meta: %s", w.body)
	}
	if w.h.Get("Cache-Control") != "no-store" || w.h.Get("ETag") != `"`+str(sub(got, "snapshot")["version"])+`"` {
		t.Errorf("headers %v", w.h)
	}
	// the client sends back the version it has: nothing changed → 304, no body
	req.Header.Set("If-None-Match", w.h.Get("ETag"))
	w2 := &hdrRecorder{h: http.Header{}}
	_ = writeSnap(w2, req, s, false)
	if w2.status != http.StatusNotModified || len(w2.body) != 0 {
		t.Errorf("304: %d %s", w2.status, w2.body)
	}
	// …but a stale answer always comes back in full (the client must know to ask again)
	w3 := &hdrRecorder{h: http.Header{}}
	_ = writeSnap(w3, req, s, true)
	if w3.status != http.StatusOK || !strings.Contains(string(w3.body), `"stale":true`) {
		t.Errorf("stale: %d %s", w3.status, w3.body)
	}
	// big bodies are gzipped when the client accepts it
	big := &dashSnap{Body: []byte(`{"x":"` + strings.Repeat("a", 5000) + `"}`), GeneratedAt: time.Now(), loc: riy}
	req2, _ := http.NewRequest("GET", "/", nil)
	req2.Header.Set("Accept-Encoding", "gzip, br")
	w4 := &hdrRecorder{h: http.Header{}}
	_ = writeSnap(w4, req2, big, false)
	plain, err := gunzip(w4.body)
	if w4.h.Get("Content-Encoding") != "gzip" || err != nil || !strings.Contains(string(plain), `"x":"aaa`) || len(w4.body) > 500 {
		t.Errorf("gzip: %v %v %d", w4.h, err, len(w4.body))
	}
	// an empty object stays valid JSON
	w5 := &hdrRecorder{h: http.Header{}}
	_ = writeSnap(w5, req2, &dashSnap{Body: []byte(`{}`), loc: riy}, false)
	if err := json.Unmarshal(w5.body, &got); err != nil {
		t.Errorf("empty body: %s", w5.body)
	}
}

func TestDashboardFeed_Unauthenticated(t *testing.T) {
	for _, path := range []string{"/dashboard/feed?storeId=x", "/dashboard/feed"} {
		for _, tok := range []string{"", "not-a-jwt"} {
			if r := call(t, "GET", path, tok, nil); r.Code != http.StatusUnauthorized {
				t.Errorf("%s token %q: got %d %s", path, tok, r.Code, r.Raw)
			}
		}
	}
}

// ---- DB-backed API + integration ----

func TestAPI_DashboardFeed(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "feed")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"country_code": "SA", "vat_percent": 15.0}}); err != nil {
		t.Fatal(err)
	}
	sdb := storeDB(sid)
	riy, _ := time.LoadLocation("Asia/Riyadh")
	now := time.Now().In(riy)
	prod := primitive.NewObjectID()
	cust := primitive.NewObjectID()
	order := func(d time.Time, extra bson.M) bson.M {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": d, "customer_id": cust, "customer_name": "Feed Customer",
			"vat_percent": 15.0, "code": "INV-F",
			"products": bson.A{bson.M{"product_id": prod, "quantity": 2.0, "unit_price": 100.0, "purchase_unit_price": 40.0}}}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	if _, err := sdb.Collection("customer").InsertOne(ctx, bson.M{"_id": cust, "name": "Feed Customer", "store_id": oid, "phone": "0500000001"}); err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Collection("product").InsertOne(ctx, bson.M{"_id": prod, "name": "Feed Widget", "item_code": "FW1",
		"product_stores": bson.M{sid: bson.M{"stock": 3.0, "purchase_unit_price": 40.0}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Collection("order").InsertMany(ctx, []interface{}{
		order(now.Add(-time.Hour), nil),
		order(now.AddDate(0, 0, -400), nil),                            // outside the one-year window
		order(now.Add(-time.Hour), bson.M{"erp": bson.M{"del": true}}), // deleted
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Collection("expense").InsertOne(ctx, bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": now.Add(-time.Hour),
		"amount": 115.0, "vat_price": 15.0, "description": "Rent"}); err != nil {
		t.Fatal(err)
	}

	r := call(t, "GET", "/dashboard/feed?storeId="+sid, owner, nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
	if r.Body["today"] != now.Format(layoutDay) || r.Body["timezone"] != "Asia/Riyadh" || r.Body["from"] != now.AddDate(0, 0, -FeedWindowDays).Format(layoutDay) {
		t.Errorf("window in store time: %v %v %v", r.Body["today"], r.Body["from"], r.Body["timezone"])
	}
	snap := sub(r.Body, "snapshot")
	if snap["generatedAt"] == "" || snap["stale"] != false {
		t.Errorf("snapshot meta: %v", snap)
	}
	feed := sub(r.Body, "feed")
	sales := arr(feed["sales"])
	if len(sales) != 1 {
		t.Fatalf("one live sale in the window: %d %v", len(sales), sales)
	}
	s0 := sales[0].(M)
	if s0["taxable"] != 200.0 || s0["vat"] != 30.0 || s0["net"] != 230.0 || s0["balance"] != 230.0 || s0["cost"] != 80.0 ||
		s0["profit"] != 120.0 || s0["status"] != "not_paid" || s0["nameEn"] != "Feed Customer" || s0["customerId"] != cust.Hex() {
		t.Errorf("sale reduced to its numbers: %v", s0)
	}
	if s0["hour"] == nil || s0["items"] != nil {
		t.Errorf("sale hour, and no invoice lines: %v", s0)
	}
	if pd := arr(feed["productDays"]); len(pd) != 1 || pd[0].(M)["productId"] != prod.Hex() || pd[0].(M)["qty"] != 2.0 ||
		pd[0].(M)["rev"] != 200.0 || pd[0].(M)["cost"] != 80.0 || pd[0].(M)["lines"] != 1.0 {
		t.Errorf("product figures per day: %v", pd)
	}
	if ex := arr(feed["expenses"]); len(ex) != 1 || ex[0].(M)["description"] != "Rent" {
		t.Errorf("expenses: %v", ex)
	}
	if cs := arr(feed["customers"]); len(cs) != 1 || cs[0].(M)["nameEn"] != "Feed Customer" || cs[0].(M)["history"] != nil {
		t.Errorf("customers (trimmed): %v", cs)
	}
	if ps := arr(feed["products"]); len(ps) != 1 || ps[0].(M)["code"] != "FW1" {
		t.Errorf("products: %v", ps)
	}
	for _, k := range []string{"nonvatSales", "salesReturns", "purchases", "deposits", "salaries", "repairJobs", "vendors", "employees"} {
		if feed[k] == nil {
			t.Errorf("%s must be a list, not null", k)
		}
	}

	// exactly the sales the list endpoint serves the web app (same window)
	list := call(t, "GET", "/sales?storeId="+sid+"&includeDeleted=1&limit=500&from="+str(r.Body["from"]), owner, nil)
	live := []string{}
	for _, row := range list.data() {
		if !boolv(row.(M)["deleted"]) {
			live = append(live, str(row.(M)["id"]))
		}
	}
	if len(live) != 1 || live[0] != s0["id"] {
		t.Errorf("same sales as the list: %v vs %v", live, s0["id"])
	}

	if r := call(t, "GET", "/dashboard/feed", owner, nil); r.Code != 400 {
		t.Errorf("missing storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/feed?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("other store: %d", r.Code)
	}
	// a cashier (no reports permission) is refused, and the page falls back to its lists
	email := "feed-cashier+" + time.Now().Format("150405.000") + "@t1.example"
	if u := call(t, "POST", "/users", owner, M{"name": "Cashier", "email": email, "phone": "0551112299", "role": "r_cashier", "storeIds": []string{sid}, "password": "Cash@1234"}); u.Code != 201 {
		t.Fatalf("cashier: %d %s", u.Code, u.Raw)
	}
	ctok := str(call(t, "POST", "/auth/login", "", M{"email": email, "password": "Cash@1234"}).Body["accessToken"])
	if r := call(t, "GET", "/dashboard/feed?storeId="+sid, ctok, nil); r.Code != 403 {
		t.Errorf("cashier: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/dashboard/bi?storeId="+sid, ctok, nil); r.Code != 403 {
		t.Errorf("cashier BI: %d", r.Code)
	}
}

// Snapshots: served ready-made, rebuilt on writes, on direct database changes, on
// request (?fresh=1), kept across restarts, and served stale while a slow rebuild runs.
func TestAPI_DashboardSnapshots(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "snap")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	fetch := func(path string, headers ...string) resp {
		t.Helper()
		r := call(t, "GET", path, owner, nil, headers...)
		if r.Code != 200 && r.Code != 304 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Raw)
		}
		return r
	}
	builds := func() int64 { return atomicLoad(&dashBuilds) }
	feedPath := "/dashboard/feed?storeId=" + sid

	b0 := builds()
	r1 := fetch(feedPath)
	if builds() != b0+1 {
		t.Fatalf("first read builds the snapshot")
	}
	r2 := fetch(feedPath)
	if builds() != b0+1 || r2.Header.Get("ETag") != r1.Header.Get("ETag") {
		t.Errorf("second read is served ready-made (builds %d, etag %s vs %s)", builds()-b0, r2.Header.Get("ETag"), r1.Header.Get("ETag"))
	}
	if r := fetch(feedPath, "If-None-Match", r1.Header.Get("ETag")); r.Code != 304 {
		t.Errorf("unchanged with the version we have: %d", r.Code)
	}

	// a write through the app (an expense) → the next read has it
	cat := call(t, "POST", "/expense-categories", owner, M{"nameEn": "Snapshot tests", "storeId": sid})
	if cat.Code != 201 {
		t.Fatalf("expense category: %d %s", cat.Code, cat.Raw)
	}
	exp := call(t, "POST", "/expenses", owner, M{"storeId": sid, "date": time.Now().In(riyadh).Format(layoutDT), "amount": 100.0, "vatAmount": 15.0,
		"description": "Snapshot test", "method": "cash", "categoryId": cat.Body["id"]})
	if exp.Code != 201 {
		t.Fatalf("expense: %d %s", exp.Code, exp.Raw)
	}
	r3 := fetch(feedPath)
	if n := len(arr(sub(r3.Body, "feed")["expenses"])); n != 1 {
		t.Errorf("the new expense shows: %d", n)
	}

	// a change made straight in the database (another app, a script) → noticed too
	if _, err := storeDB(sid).Collection("expense").InsertOne(ctx, bson.M{"_id": primitive.NewObjectID(), "store_id": oid,
		"date": time.Now(), "amount": 50.0, "description": "Direct", "updated_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	if n := len(arr(sub(fetch(feedPath).Body, "feed")["expenses"])); n != 2 {
		t.Errorf("a direct database change shows: %d", n)
	}

	// ?fresh=1 rebuilds even when nothing changed (Recompute)
	b1 := builds()
	fetch(feedPath + "&fresh=1")
	if builds() != b1+1 {
		t.Errorf("fresh=1 rebuilds")
	}

	// a restart: memory empty, the saved snapshot is used (no rebuild)
	time.Sleep(300 * time.Millisecond) // saved in the background
	resetDashboardSnapshots()
	b2 := builds()
	if r := fetch(feedPath); builds() != b2 || len(arr(sub(r.Body, "feed")["expenses"])) != 2 {
		t.Errorf("served from the saved snapshot after a restart (builds %d)", builds()-b2)
	}

	// figure endpoints are snapshotted per period
	v1 := fetch("/dashboard/vat?storeId=" + sid + "&from=2026-10-01&to=2026-10-31")
	v2 := fetch("/dashboard/vat?storeId=" + sid + "&from=2026-09-01&to=2026-09-30")
	if str(sub(v1.Body, "snapshot")["version"]) == "" || v1.Body["from"] != "2026-10-01" || v2.Body["from"] != "2026-09-01" {
		t.Errorf("per-period snapshots: %v / %v", v1.Body["from"], v2.Body["from"])
	}
}

func atomicLoad(p *int64) int64 { return atomic.LoadInt64(p) }

func notifyLegacyDirty(oid primitive.ObjectID) {
	now := time.Now()
	models.MarkDashboardDirty(oid, &now)
}

// Write events rebuild in the background (no read needed); a slow rebuild is not
// waited for: the previous figures come back marked stale.
func TestAPI_DashboardSnapshotEvents(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "snapev")
	defer cleanupStore(t, sid)
	var slow, calls int64 // slow: milliseconds
	k := registerDashKind(&dashKind{Name: "test-slow", Handler: func(c *Ctx, w http.ResponseWriter, r *http.Request) error {
		n := atomic.AddInt64(&calls, 1)
		time.Sleep(time.Duration(atomic.LoadInt64(&slow)) * time.Millisecond)
		writeJSON(w, http.StatusOK, M{"calls": n})
		return nil
	}})
	defer delete(dashKinds, "test-slow")
	h := authed(snapshotted(k))
	read := func() M {
		t.Helper()
		req, _ := http.NewRequest("GET", Prefix+"/dashboard/test-slow?storeId="+sid, nil)
		req.Header.Set("Authorization", "Bearer "+owner)
		w := &hdrRecorder{h: http.Header{}}
		h(w, req)
		var m M
		if err := json.Unmarshal(w.body, &m); err != nil {
			t.Fatalf("%d %s", w.status, w.body)
		}
		return m
	}
	if m := read(); m["calls"] != 1.0 {
		t.Fatalf("built: %v", m)
	}

	oldDeb, oldWait := SnapshotDebounce, SnapshotWait
	SnapshotDebounce, SnapshotWait = 20*time.Millisecond, 50*time.Millisecond
	defer func() { SnapshotDebounce, SnapshotWait = oldDeb, oldWait }()
	startDashboardEvents()

	// a write event: rebuilt in the background, the next read is ready-made
	DashboardChanged(sid)
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&calls) < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := atomic.LoadInt64(&calls); n != 2 {
		t.Fatalf("background rebuild after a write: %d", n)
	}
	time.Sleep(50 * time.Millisecond)
	if m := read(); m["calls"] != 2.0 || sub(m, "snapshot")["stale"] != false {
		t.Errorf("fresh after the rebuild: %v", m)
	}

	// a legacy write (models.MarkDashboardDirty) is an event too
	atomic.StoreInt64(&slow, 400)
	oid, _ := primitive.ObjectIDFromHex(sid)
	SnapshotDebounce = time.Hour // keep the background rebuild out of the way
	notifyLegacyDirty(oid)
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	m := read()
	if sub(m, "snapshot")["stale"] != true || m["calls"] != 2.0 || time.Since(start) > 300*time.Millisecond {
		t.Errorf("slow rebuild: the previous figures, marked stale, without waiting: %v (%s)", m, time.Since(start))
	}
	time.Sleep(500 * time.Millisecond)
	if m := read(); m["calls"] != 3.0 || sub(m, "snapshot")["stale"] != false {
		t.Errorf("then the new ones: %v", m)
	}
}
