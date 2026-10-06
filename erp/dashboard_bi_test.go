package erp

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---- pure ----

func TestNewBIPeriod_StoreTimezone(t *testing.T) {
	riy, _ := time.LoadLocation("Asia/Riyadh")
	cases := []struct {
		name      string
		now       time.Time
		loc       *time.Location
		today     string
		first     string
		thisMonth string
	}{
		// 30 Sep 21:30 UTC is already 1 Oct in Saudi Arabia: October is the current month
		{"saudi after midnight", time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC), riy, "2026-10-01", "2025-11", "2026-10"},
		{"utc still september", time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC), time.UTC, "2026-09-30", "2025-10", "2026-09"},
		{"nil location = riyadh", time.Date(2026, 9, 30, 21, 30, 0, 0, time.UTC), nil, "2026-10-01", "2025-11", "2026-10"},
		{"year boundary", time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), riy, "2026-01-15", "2025-02", "2026-01"},
	}
	for _, c := range cases {
		p := NewBIPeriod(c.now, c.loc, 12)
		if p.Today != c.today || p.ThisMonth != c.thisMonth || p.Months[0] != c.first || len(p.Months) != 12 {
			t.Errorf("%s: %+v", c.name, p)
		}
		if p.Months[11] != p.ThisMonth || p.From != p.Months[0]+"-01" {
			t.Errorf("%s: bounds %+v", c.name, p)
		}
	}
	if p := NewBIPeriod(time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), time.UTC, 1); p.To != "2026-02-28" || p.From != "2026-02-01" {
		t.Errorf("february: %+v", p)
	}
	if p := NewBIPeriod(time.Now(), time.UTC, 0); len(p.Months) != 1 {
		t.Errorf("n<1 must cover one month: %+v", p)
	}
}

func TestBIDays(t *testing.T) {
	for _, c := range []struct {
		a, b string
		n    int
	}{{"2026-10-01", "2026-10-06", 5}, {"2026-03-28", "2026-03-30", 2}, {"2026-10-06", "2026-10-01", -5}, {"bad", "2026-10-01", 0}} {
		if got := biDays(c.a, c.b); got != c.n {
			t.Errorf("%s→%s = %d, want %d", c.a, c.b, got, c.n)
		}
	}
}

func TestBITotals_MatchesClientComputeTotals(t *testing.T) {
	items := []interface{}{M{"qty": 2.0, "unitPrice": 100.0, "unitDiscount": 10.0, "purchasePrice": 60.0}}
	cases := []struct {
		name                  string
		rec                   M
		taxable, net, balance float64
	}{
		// 2×(100−10)=180, VAT 27 → 207
		{"default vat 15", M{"items": items}, 180, 207, 207},
		{"zero vat", M{"items": items, "vatPercent": 0.0}, 180, 180, 180},
		{"discount and shipping", M{"items": items, "discount": 20.0, "shipping": 10.0}, 170, 195.5, 195.5},
		{"part paid and cash discount", M{"items": items, "payments": []interface{}{M{"amount": 100.0}}, "cashDiscount": 7.0}, 180, 207, 100},
		{"overpaid clamps to zero", M{"items": items, "payments": []interface{}{M{"amount": 500.0}}}, 180, 207, 0},
		{"manual rounding", M{"items": items, "rounding": -0.5}, 180, 206.5, 206.5},
		// 1×10.01 → 11.51 (VAT 1.50) → auto rounded to 11.50
		{"auto rounding", M{"items": []interface{}{M{"qty": 1.0, "unitPrice": 10.01}}, "roundingAuto": true}, 10.01, 11.5, 11.5},
		{"no items", M{}, 0, 0, 0},
	}
	for _, c := range cases {
		tx, net, bal := biTotals(c.rec)
		if tx != c.taxable || net != c.net || bal != c.balance {
			t.Errorf("%s: got %v/%v/%v want %v/%v/%v", c.name, tx, net, bal, c.taxable, c.net, c.balance)
		}
	}
}

func TestBIDocOf(t *testing.T) {
	d := BIDocOf(M{"id": "s1", "code": "INV-1", "date": "2026-10-05T14:30", "customerId": "c1", "customerName": "A",
		"items": []interface{}{M{"productId": "p1", "qty": 3.0, "unitPrice": 10.0, "unitDiscount": 1.0, "purchasePrice": 4.0}},
		"orderIds": []interface{}{"o1"}, "status": "accepted"})
	if d.Day != "2026-10-05" || d.month() != "2026-10" || d.CustomerID != "c1" || len(d.Lines) != 1 {
		t.Fatalf("%+v", d)
	}
	if l := d.Lines[0]; l.Rev != 27 || l.Cost != 12 || l.Qty != 3 {
		t.Errorf("line %+v", l)
	}
	if d.Status != "accepted" || len(d.OrderIDs) != 1 || d.Taxable != 27 {
		t.Errorf("%+v", d)
	}
	if (BIDoc{}).month() != "" {
		t.Error("empty day must have no month")
	}
}

func biSale(id, day, cust string, taxable, balance float64, lines ...BILine) BIDoc {
	return BIDoc{ID: id, Code: "INV-" + id, Day: day, CustomerID: cust, NameEn: "Doc " + cust, Taxable: taxable,
		Net: round2(taxable * 1.15), Balance: balance, Lines: lines}
}

func biTestPeriod() BIPeriod {
	return NewBIPeriod(time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC), time.UTC, 12) // Nov 2025 – Oct 2026
}

func TestComputeBI_Customers(t *testing.T) {
	p := biTestPeriod()
	in := BIInput{
		Sales: []BIDoc{
			biSale("old", "2024-03-10", "c1", 100, 0), // bought long before the 12 months: returning, not new
			biSale("s1", "2026-09-10", "c1", 100, 0),
			biSale("s2", "2026-09-12", "c2", 1000, 0), // first purchase in Sep 2026: new
			biSale("s3", "2026-10-02", "c2", 1000, 0),
			biSale("w1", "2026-10-03", "", 99999, 0), // walk-in: no customer
			biSale("s4", "2026-05-01", "c3", 50000, 0),
		},
		Customers: []BICustomer{{ID: "c1", NameEn: "Master name"}},
	}
	r := ComputeBI(in, p)
	c := r.Customers
	if len(c.Rows) != 3 {
		t.Fatalf("walk-in sales must not become a customer: %+v", c.Rows)
	}
	byID := map[string]BICustomerRow{}
	for _, row := range c.Rows {
		byID[row.ID] = row
	}
	if byID["c1"].First != "2024-03-10" || byID["c1"].Orders != 2 || byID["c1"].NameEn != "Master name" {
		t.Errorf("c1 lifetime: %+v", byID["c1"])
	}
	if byID["c3"].Tier != "high" || byID["c2"].Tier != "low" || byID["c1"].Tier != "low" {
		t.Errorf("tiers: %+v", c.Rows)
	}
	if c.Risk["high"] != 1 || c.Risk["low"] != 2 || c.Active != 2 {
		t.Errorf("risk %v active %d", c.Risk, c.Active)
	}
	sep := c.NewVsReturning[10]
	if sep.Key != "2026-09" || sep.NewC != 1 || sep.Ret != 1 {
		t.Errorf("Sep new/returning %+v", sep)
	}
	if oct := c.NewVsReturning[11]; oct.NewC != 0 || oct.Ret != 1 {
		t.Errorf("Oct %+v", oct)
	}
	if c.Repeat < 66.6 || c.Repeat > 66.7 {
		t.Errorf("repeat %v", c.Repeat)
	}
	// c1 230, c2 2300, c3 57500 (incl VAT)
	if c.Clv[0].Customers != 2 || c.Clv[5].Customers != 0 || c.Clv[4].Customers != 1 {
		t.Errorf("clv %+v", c.Clv)
	}
	// cohorts in range: May 2026 (c3) and Sep 2026 (c2); c1's cohort is before the range
	if len(c.Cohorts) != 2 || c.Cohorts[0].Key != "2026-05" || c.Cohorts[1].Key != "2026-09" {
		t.Fatalf("cohorts %+v", c.Cohorts)
	}
	sepC := c.Cohorts[1]
	if *sepC.Heat[0] != 100 || *sepC.Heat[1] != 100 || sepC.Heat[2] != nil {
		t.Errorf("Sep cohort heat %v", sepC.Heat)
	}
	if may := c.Cohorts[0]; *may.Heat[0] != 100 || *may.Heat[1] != 0 || may.Heat[6] != nil {
		t.Errorf("May cohort heat %v", may.Heat)
	}
}

func TestComputeBI_ProductsAgingQuotationsAsk(t *testing.T) {
	p := biTestPeriod()
	l := func(id string, qty, rev, cost float64) BILine {
		return BILine{ProductID: id, Qty: qty, Rev: rev, Cost: cost, NameEn: "line " + id}
	}
	in := BIInput{
		Sales: []BIDoc{
			biSale("a", "2025-01-01", "c1", 500, 500, l("p1", 1, 500, 100)),    // outside the months, open 640 days
			biSale("b", "2026-09-15", "c1", 1000, 1150, l("p1", 10, 1000, 400)), // last month
			biSale("c", "2026-10-01", "c2", 200, 230, l("p2", 2, 200, 50)),
			biSale("d", "2026-10-02", "c2", 300, 345, l("p2", 3, 300, 90)),
		},
		NonVAT:   []BIDoc{biSale("n", "2026-08-20", "c2", 400, 0, l("p3", 4, 400, 100))},
		Returns:  []BIDoc{{ID: "r", Day: "2026-10-03", OrderID: "c", Balance: 30, Taxable: 100, Lines: []BILine{l("p2", 1, 100, 25)}}},
		Deposits: []BIDoc{{ID: "dep", OrderID: "d", Amount: 345}},
		Quotations: []BIDoc{
			{Day: "2026-09-01", Status: "accepted", Taxable: 100, OrderIDs: []string{"b"}},
			{Day: "2026-09-02", Status: "rejected", Taxable: 50},
			{Day: "2026-09-03", Status: "created", Taxable: 10},
			{Day: "2024-01-01", Status: "accepted", Taxable: 999}, // before the months
		},
		Customers: []BICustomer{{ID: "c1", NameEn: "Cust 1", Opening: 100}, {ID: "c2", NameEn: "Cust 2"}},
		Products: map[string]BIProduct{
			"p1":   {ID: "p1", NameEn: "Prod 1", Code: "P1", Stock: 5, Purchase: 40},
			"p2":   {ID: "p2", NameEn: "Prod 2", Code: "P2", Stock: 1, Purchase: 25},
			"idle": {ID: "idle", NameEn: "Idle", Stock: 10, Purchase: 30},
			"svc":  {ID: "svc", NameEn: "Service", Stock: 10, Purchase: 30, IsService: true},
		},
	}
	r := ComputeBI(in, p)

	if len(r.Products) != 3 || r.Products[0].ID != "p1" || r.Products[0].Rev != 1000 {
		t.Fatalf("products by revenue, months only: %+v", r.Products)
	}
	p2 := r.Products[1]
	if p2.ID != "p2" || p2.Qty != 5 || p2.RetQty != 1 || p2.RetRev != 100 || p2.Code != "P2" || p2.NameEn != "Prod 2" {
		t.Errorf("p2 %+v", p2)
	}
	if p2.ByMonth[11] != 500 || p2.QtyByMonth[11] != 5 || p2.Margin != 72 {
		t.Errorf("p2 months %+v", p2)
	}
	if r.Products[2].NameEn != "line p3" {
		t.Errorf("unknown product keeps its line name: %+v", r.Products[2])
	}

	// aging: a 500 (90+), b 1150 (21 days), c 230−30 return = 200, d 345−345 deposit = paid
	if r.Aging[3].Count != 1 || r.Aging[3].Amount != 500 || r.Aging[0].Count != 2 || r.Aging[0].Amount != 1350 {
		t.Errorf("aging %+v", r.Aging)
	}
	if r.Aging[0].Rows[0].ID != "b" {
		t.Errorf("rows oldest first: %+v", r.Aging[0].Rows)
	}
	if r.Receivables != 1850 {
		t.Errorf("receivables %v", r.Receivables)
	}

	q := r.Quotations
	if q.Created != 3 || q.Accepted != 1 || q.Decided != 2 || q.Rate != 50 || q.Invoiced != 1 || q.Sent != 2 {
		t.Errorf("quotations %+v", q)
	}
	if q.ByStatus[3].S != "accepted" || q.ByStatus[3].Value != 100 {
		t.Errorf("by status %+v", q.ByStatus)
	}

	a := r.Ask
	if a.LastMonth != "2026-09" || len(a.TopLastMonth) != 1 || a.TopLastMonth[0].ID != "p1" {
		t.Errorf("top last month %+v", a.TopLastMonth)
	}
	// c1: opening 100 + 500 + 1150, oldest 2025-01-01
	if len(a.Overdue) != 1 || a.Overdue[0].ID != "c1" || a.Overdue[0].Balance != 1750 {
		t.Errorf("overdue %+v", a.Overdue)
	}
	if a.Monthly[11].Revenue != 400 || a.Monthly[11].Orders != 2 || a.Monthly[10].Revenue != 1000 || a.Monthly[9].Revenue != 400 {
		t.Errorf("monthly %+v", a.Monthly)
	}
	if a.SlowCount != 1 || a.Slow[0].ID != "idle" || a.SlowValue != 300 {
		t.Errorf("slow %+v", a)
	}
}

func TestComputeBI_Empty(t *testing.T) {
	r := ComputeBI(BIInput{}, biTestPeriod())
	if r.Customers.Rows == nil || r.Products == nil || len(r.Aging) != 4 || r.Aging[0].Rows == nil ||
		len(r.Customers.NewVsReturning) != 12 || len(r.Customers.Clv) != 6 || r.Customers.Cohorts == nil ||
		r.Ask.Overdue == nil || r.Ask.Slow == nil || len(r.Quotations.ByStatus) != 7 {
		t.Errorf("an empty store must still give every chart its shape: %+v", r)
	}
}

func TestDashboardBI_Unauthenticated(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt"} {
		r := call(t, "GET", "/dashboard/bi?storeId=x", tok, nil)
		if r.Code != http.StatusUnauthorized {
			t.Errorf("token %q: got %d %s", tok, r.Code, r.Raw)
		}
	}
}

// ---- DB-backed API + integration ----

func TestAPI_DashboardBI(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "bidash")
	defer cleanupStore(t, sid)
	oid, _ := primitive.ObjectIDFromHex(sid)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"country_code": "SA", "vat_percent": 15.0}}); err != nil {
		t.Fatal(err)
	}
	sdb := storeDB(sid)
	riy, _ := time.LoadLocation("Asia/Riyadh")
	now := time.Now().In(riy)
	thisMonth := now.Format("2006-01")
	cust := primitive.NewObjectID()
	prod := primitive.NewObjectID()
	idle := primitive.NewObjectID()
	order := func(d time.Time, customer interface{}, qty float64, extra bson.M) bson.M {
		m := bson.M{"_id": primitive.NewObjectID(), "store_id": oid, "date": d, "customer_id": customer,
			"customer_name": "Old Customer", "vat_percent": 15.0, "code": "INV",
			"products": bson.A{bson.M{"product_id": prod, "quantity": qty, "unit_price": 100.0, "purchase_unit_price": 40.0}}}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	twoYearsAgo := now.AddDate(-2, 0, 0)
	if _, err := sdb.Collection("customer").InsertOne(ctx, bson.M{"_id": cust, "name": "Real Name", "store_id": oid}); err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Collection("product").InsertMany(ctx, []interface{}{
		bson.M{"_id": prod, "name": "Widget", "item_code": "W1", "product_stores": bson.M{sid: bson.M{"stock": 3.0, "purchase_unit_price": 40.0}}},
		bson.M{"_id": idle, "name": "Idle", "product_stores": bson.M{sid: bson.M{"stock": 10.0, "purchase_unit_price": 5.0}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Collection("order").InsertMany(ctx, []interface{}{
		order(twoYearsAgo, cust, 1, nil), // first purchase two years ago, still open
		order(now.Add(-time.Minute), cust, 2, bson.M{"payments": bson.A{bson.M{"amount": 230.0, "method": "cash"}}}),
		order(now.Add(-time.Minute), nil, 5, bson.M{"payments": bson.A{bson.M{"amount": 575.0, "method": "cash"}}}), // walk-in
		order(now.Add(-time.Minute), cust, 50, bson.M{"erp": bson.M{"del": true}}),                                  // deleted
	}); err != nil {
		t.Fatal(err)
	}

	r := call(t, "GET", "/dashboard/bi?storeId="+sid, owner, nil)
	if r.Code != 200 {
		t.Fatalf("%d %s", r.Code, r.Raw)
	}
	per := sub(r.Body, "period")
	if per["thisMonth"] != thisMonth || per["today"] != now.Format("2006-01-02") || r.Body["timezone"] != "Asia/Riyadh" {
		t.Errorf("period in store time: %v %v", per, r.Body["timezone"])
	}
	cs := sub(r.Body, "customers")
	rows := arr(cs["rows"])
	if len(rows) != 1 {
		t.Fatalf("one customer (walk-in and deleted left out): %v", rows)
	}
	c0 := rows[0].(M)
	if c0["nameEn"] != "Real Name" || c0["orders"] != 2.0 || c0["first"] != twoYearsAgo.Format("2006-01-02") {
		t.Errorf("customer lifetime from the whole history: %v", c0)
	}
	nr := arr(cs["newVsReturning"])
	if last := nr[len(nr)-1].(M); last["ret"] != 1.0 || last["newC"] != 0.0 {
		t.Errorf("a customer first seen two years ago is returning: %v", last)
	}
	prods := arr(r.Body["products"])
	if len(prods) != 1 || prods[0].(M)["qty"] != 7.0 || prods[0].(M)["nameEn"] != "Widget" || prods[0].(M)["code"] != "W1" {
		t.Errorf("products this year (deleted left out): %v", prods)
	}
	aging := arr(r.Body["aging"])
	if b := aging[3].(M); b["count"] != 1.0 || b["amount"] != 115.0 {
		t.Errorf("an invoice open for two years is in 90+: %v", aging)
	}
	if r.Body["receivables"] != 115.0 {
		t.Errorf("receivables %v", r.Body["receivables"])
	}
	ask := sub(r.Body, "ask")
	if ask["slowCount"] != 1.0 || ask["slowValue"] != 50.0 {
		t.Errorf("slow stock: %v", ask)
	}
	if r.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("must never be cached: %q", r.Header.Get("Cache-Control"))
	}

	// validation and access
	if r := call(t, "GET", "/dashboard/bi", owner, nil); r.Code != 400 || r.errField("storeId") == "" {
		t.Errorf("missing storeId: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/dashboard/bi?storeId="+fx.StoreA.Hex(), owner, nil); r.Code != 404 {
		t.Errorf("someone else's store: %d", r.Code)
	}
	if r := call(t, "GET", "/dashboard/bi?storeId="+fx.StoreA.Hex(), login(t, fx.AdminEmail), nil); r.Code != 200 {
		t.Errorf("admin: %d %s", r.Code, r.Raw)
	}
}
