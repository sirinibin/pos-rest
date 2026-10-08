package erp

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestQuotationExpiry(t *testing.T) {
	cases := []struct {
		date string
		days float64
		want string
	}{
		{"2026-10-01T10:30", 7, "2026-10-08T10:30"},
		{"2026-10-01T10:30:59", 0, "2026-10-01T10:30"},
		{"2026-10-01", 30, "2026-10-31T00:00"},
		{"2026-12-25T23:00", 10, "2027-01-04T23:00"},
		{"2026-10-01T10:30", 0.5, "2026-10-01T22:30"},
		{"", 7, ""},
		{"not a date", 7, ""},
	}
	for _, c := range cases {
		if got := quotationExpiry(c.date, c.days); got != c.want {
			t.Errorf("quotationExpiry(%q, %v) = %q, want %q", c.date, c.days, got, c.want)
		}
	}
}

// quotationStatusAt is conversions.js quotationStatus: an open quotation past its
// expiry reads "expired"; a decided one keeps its status.
func TestQuotationStatusAt(t *testing.T) {
	now := "2026-10-08T12:00"
	cases := []struct{ status, expiry, want string }{
		{"pending", "2026-10-08T11:59", "expired"},
		{"pending", "2026-10-08T12:00", "pending"},
		{"pending", "2026-10-09T00:00", "pending"},
		{"created", "2026-01-01T00:00", "expired"},
		{"delivered", "2026-01-01T00:00", "expired"},
		{"accepted", "2026-01-01T00:00", "accepted"},
		{"rejected", "2026-01-01T00:00", "rejected"},
		{"cancelled", "2026-01-01T00:00", "cancelled"},
		{"expired", "2027-01-01T00:00", "expired"},
		{"pending", "", "pending"},
	}
	for _, c := range cases {
		if got := quotationStatusAt(c.status, c.expiry, now); got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.status, c.expiry, got, c.want)
		}
	}
}

func TestDeliveryLateAt(t *testing.T) {
	now := "2026-10-08T12:00"
	cases := []struct{ status, est, want string }{
		{"pending", "2026-10-08T11:00", "y"},
		{"pending", "2026-10-08T11:00:30", "y"},
		{"pending", "2026-10-08T13:00", "n"},
		{"pending", "2026-10-07", "y"},
		{"pending", "", "n"},
		{"delivered", "2026-01-01T00:00", "n"},
		{"cancelled", "2026-01-01T00:00", "n"},
	}
	for _, c := range cases {
		if got := deliveryLateAt(c.status, c.est, now); got != c.want {
			t.Errorf("%s/%s: %q, want %q", c.status, c.est, got, c.want)
		}
	}
}

func TestDerivedOf(t *testing.T) {
	q := StatsQuery{Filters: map[string]string{"qstatus": "expired", "party": "c1"}, Sort: "-qstatus",
		Sums: []string{"net", "one|late=y", "one|status=pending"}, GroupBy: "late"}
	got := derivedOf(q)
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"late", "qstatus"}) {
		t.Fatalf("derivedOf: %v", got)
	}
	if d := derivedOf(StatsQuery{Sort: "-date", Sums: []string{"net"}}); len(d) != 0 {
		t.Fatalf("no derived: %v", d)
	}
	// the fields they read are fetched; the derived names themselves are not fields
	f := statFields(q)
	for _, k := range []string{"status", "expiry", "estDelivery"} {
		if !f[k] {
			t.Errorf("%s not read: %v", k, f)
		}
	}
	for _, k := range []string{"qstatus", "late"} {
		if f[k] {
			t.Errorf("%s read as a field: %v", k, f)
		}
	}
}

func TestStatRowOf_Expiry(t *testing.T) {
	r := statRowOf(M{"id": "q1", "date": "2026-10-01T09:00", "validityDays": 14.0, "status": "pending"},
		map[string]bool{"expiry": true, "status": true})
	if r.Vals["expiry"] != "2026-10-15T09:00" || r.Vals["status"] != "pending" {
		t.Fatalf("vals: %v", r.Vals)
	}
}

// The Quotations tabs, tiles and pages go by qstatus: counts per status, a tab's
// page and sorting by status all agree.
func TestAddUp_QuotationStatus(t *testing.T) {
	q := func(id, date, status, expiry string, net float64) statRow {
		r := statRow{ID: id, Vals: map[string]interface{}{"date": date, "status": status, "expiry": expiry}}
		r.T.Net = net
		return r
	}
	rows := []statRow{
		q("a", "2025-01-01", "pending", "2025-01-31T00:00", 10),  // expired long ago
		q("b", "2026-10-01", "pending", "2026-10-31T00:00", 20),  // open
		q("c", "2026-09-01", "accepted", "2026-09-10T00:00", 30), // accepted stays accepted
		q("d", "2026-10-05", "created", "2026-10-06T00:00", 40),  // draft, expired
	}
	base := StatsQuery{DateKey: "date", Now: "2026-10-08T12:00", Sums: []string{"net", "one|qstatus=expired"}}
	all := addUp(base, rows, nil)
	if all.Count != 4 || all.Sums["one|qstatus=expired"] != 2 {
		t.Fatalf("all: %+v", all)
	}
	tab := base
	tab.Filters = map[string]string{"qstatus": "expired"}
	tab.Page, tab.Limit, tab.Sort = 1, 25, "-date"
	got := addUp(tab, rows, nil)
	if got.Count != 2 || got.Sums["net"] != 50 || !reflect.DeepEqual(got.IDs, []string{"d", "a"}) {
		t.Fatalf("expired tab: %+v", got)
	}
	bySt := base
	bySt.Page, bySt.Limit, bySt.Sort = 1, 25, "qstatus"
	if got := addUp(bySt, rows, nil); !reflect.DeepEqual(got.IDs, []string{"c", "a", "d", "b"}) {
		t.Fatalf("by status: %v", got.IDs)
	}
	byExp := base
	byExp.Page, byExp.Limit, byExp.Sort = 1, 25, "-expiry"
	if got := addUp(byExp, rows, nil); !reflect.DeepEqual(got.IDs, []string{"b", "d", "c", "a"}) {
		t.Fatalf("by expiry: %v", got.IDs)
	}
	// the cached rows are not changed by a request
	if _, ok := rows[0].Vals["qstatus"]; ok {
		t.Fatal("cached row changed")
	}
}

func TestAddUp_DeliveryLate(t *testing.T) {
	d := func(id, status, est string) statRow {
		return statRow{ID: id, Vals: map[string]interface{}{"date": "2026-10-01", "status": status, "estDelivery": est}}
	}
	rows := []statRow{d("a", "pending", "2026-10-02T10:00"), d("b", "pending", "2026-10-20T10:00"),
		d("c", "delivered", "2026-10-02T10:00")}
	q := StatsQuery{DateKey: "date", Now: "2026-10-08T12:00",
		Sums: []string{"one|status=pending", "one|status=delivered", "one|late=y"}}
	out := addUp(q, rows, nil)
	want := map[string]float64{"one|status=pending": 2, "one|status=delivered": 1, "one|late=y": 1}
	if out.Count != 3 || !reflect.DeepEqual(out.Sums, want) {
		t.Fatalf("delivery tiles: %+v", out)
	}
}

// lines=payments pages return the lines themselves.
func TestAddUp_LinePages(t *testing.T) {
	rec := M{"id": "s1", "code": "S-1", "date": "2026-10-01T09:00", "customerId": "c1", "customerName": "Acme",
		"payments": []interface{}{
			M{"id": "p1", "amount": 10.0, "method": "cash", "date": "2026-10-02T09:00"},
			M{"id": "p2", "amount": 30.0, "method": "card", "description": "ref 9"},
		}}
	rec2 := M{"id": "s2", "code": "S-2", "date": "2026-10-03T09:00", "customerId": "c2",
		"payments": []interface{}{M{"id": "p3", "amount": 20.0, "method": "cash"}}}
	fields := map[string]bool{linesField: true, "date": true, "code": true, "customerName": true}
	rows := []statRow{statRowOf(rec, fields), statRowOf(rec2, fields)}
	q := StatsQuery{DateKey: "date", Lines: "payments", Sums: []string{"amount"}, Page: 1, Limit: 2, Sort: "-amount"}
	out := addUp(q, rows, nil)
	if out.Count != 3 || out.Sums["amount"] != 60 || out.IDs != nil || len(out.Rows) != 2 {
		t.Fatalf("page 1: %+v", out)
	}
	if out.Rows[0]["id"] != "s1~p2" || out.Rows[0]["docId"] != "s1" || out.Rows[0]["code"] != "S-1" ||
		out.Rows[0]["description"] != "ref 9" || out.Rows[0]["date"] != "2026-10-01T09:00" ||
		out.Rows[0]["partyId"] != "c1" || out.Rows[1]["id"] != "s2~p3" {
		t.Fatalf("rows: %v", out.Rows)
	}
	q.Page = 2
	if out := addUp(q, rows, nil); len(out.Rows) != 1 || out.Rows[0]["id"] != "s1~p1" || out.Rows[0]["method"] != "cash" {
		t.Fatalf("page 2: %v", out.Rows)
	}
	q.Filters = map[string]string{"method": "cash"}
	q.Page = 1
	if out := addUp(q, rows, nil); out.Count != 2 || out.Sums["amount"] != 30 || len(out.Rows) != 2 {
		t.Fatalf("cash lines: %+v", out)
	}
}

// The Payments lists page their lines on the server: every line once, each with its
// document, adding up to the count and total.
func TestAPI_ListStats_LinePages(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	tot := call(t, "GET", "/sales/stats?storeId="+storeA()+"&lines=payments&sum=amount", tok, nil)
	n := int(num(tot.Body["count"]))
	if tot.Code != 200 || n == 0 {
		t.Fatalf("lines: %d %s", tot.Code, tot.Raw)
	}
	seen := map[string]bool{}
	var sum float64
	for page := 1; page <= n; page++ {
		r := call(t, "GET", "/sales/stats?storeId="+storeA()+"&lines=payments&sum=amount&keys=code&page="+itoa(page)+
			"&limit=1&sort=-date", tok, nil)
		rows := arr(r.Body["rows"])
		if r.Code != 200 || len(rows) != 1 || r.Body["ids"] != nil {
			t.Fatalf("page %d: %d %s", page, r.Code, r.Raw)
		}
		row := rows[0].(M)
		id := str(row["id"])
		if seen[id] || str(row["docId"]) == "" || str(row["code"]) == "" {
			t.Fatalf("page %d row %v", page, row)
		}
		seen[id] = true
		sum += num(row["amount"])
	}
	if round2(sum) != num(get(tot.Body, "sums.amount")) {
		t.Fatalf("lines add up to %v, total %v", round2(sum), get(tot.Body, "sums.amount"))
	}
}

// Quotations: qstatus tabs and counts over every quotation.
func TestAPI_ListStats_QuotationStatus(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	all := call(t, "GET", "/quotations/stats?storeId="+storeA()+"&sum=one", tok, nil)
	if all.Code != 200 {
		t.Fatalf("stats: %d %s", all.Code, all.Raw)
	}
	n := int(num(all.Body["count"]))
	total := 0
	for _, st := range []string{"created", "delivered", "pending", "accepted", "rejected", "expired", "cancelled"} {
		r := call(t, "GET", "/quotations/stats?storeId="+storeA()+"&f.qstatus="+st+"&page=1&limit=500&sort=qstatus", tok, nil)
		if r.Code != 200 || len(arr(r.Body["ids"])) != int(num(r.Body["count"])) {
			t.Fatalf("%s: %d %s", st, r.Code, r.Raw)
		}
		total += int(num(r.Body["count"]))
	}
	if total != n {
		t.Fatalf("tabs add up to %d of %d", total, n)
	}
	if r := call(t, "GET", "/quotations/stats?storeId="+storeA()+"&sum=one|qstatus=expired&page=1&sort=-expiry", tok, nil); r.Code != 200 {
		t.Fatalf("expiry sort: %d %s", r.Code, r.Raw)
	}
}

// Proformas (a new collection) add up and page on the server like the legacy lists,
// expired ones included.
func TestAPI_ListStats_Proformas(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	before := call(t, "GET", "/proformas/stats?storeId="+storeA()+"&sum=net", tok, nil)
	if before.Code != 200 {
		t.Fatalf("stats: %d %s", before.Code, before.Raw)
	}
	n0 := int(num(before.Body["count"]))
	loc := orRiyadh(time.FixedZone("x", 3*3600))
	old := time.Now().In(loc).AddDate(-2, 0, 0).Format("2006-01-02T15:04")
	for i, d := range []string{old, time.Now().In(loc).Format("2006-01-02T15:04")} {
		cr := call(t, "POST", "/proformas", tok, M{"storeId": storeA(), "date": d, "validityDays": 7, "status": "pending",
			"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 100 * (i + 1)}}})
		if cr.Code != 201 && cr.Code != 200 {
			t.Fatalf("create: %d %s", cr.Code, cr.Raw)
		}
	}
	all := call(t, "GET", "/proformas/stats?storeId="+storeA()+"&sum=net&page=1&limit=500", tok, nil)
	if int(num(all.Body["count"])) != n0+2 || len(arr(all.Body["ids"])) != n0+2 {
		t.Fatalf("all: %s", all.Raw)
	}
	exp := call(t, "GET", "/proformas/stats?storeId="+storeA()+"&f.qstatus=expired&sum=one", tok, nil)
	if int(num(exp.Body["count"])) < 1 {
		t.Fatalf("expired: %s", exp.Raw)
	}
	from := time.Now().In(loc).AddDate(0, 0, -365).Format(layoutDay)
	yr := call(t, "GET", "/proformas/stats?storeId="+storeA()+"&from="+from+"&sum=net", tok, nil)
	if int(num(yr.Body["count"])) != int(num(all.Body["count"]))-1-countOlder(t, tok, from) {
		t.Fatalf("last year %v of %v", yr.Body["count"], all.Body["count"])
	}
}

// countOlder: proformas before the new one-year-old cut-off, besides the one this test made.
func countOlder(t *testing.T, tok, from string) int {
	r := call(t, "GET", "/proformas/stats?storeId="+storeA()+"&to="+from, tok, nil)
	return int(num(r.Body["count"])) - 1
}

// running=amount: each page record's running balance over the whole list (ledger.js
// W4: date then code order, deleted left out), whatever the filters.
func TestAddUp_Running(t *testing.T) {
	r := func(id, date, code, who string, amt float64, del bool) statRow {
		return statRow{ID: id, Deleted: del, Vals: map[string]interface{}{"date": date, "code": code, "amount": amt, "investor": who}}
	}
	rows := []statRow{
		r("c", "2026-03-01T10:00", "CAP-3", "B", 300, false),
		r("a", "2024-01-01T09:00", "CAP-1", "A", 100, false),
		r("x", "2025-01-01T09:00", "CAP-9", "A", 999, true),
		r("b", "2026-03-01T10:00", "CAP-2", "A", 50, false),
	}
	q := StatsQuery{DateKey: "date", Sums: []string{"amount"}, Page: 1, Limit: 2, Sort: "-date", Running: "amount",
		Filters: map[string]string{"investor": "B"}}
	out := addUp(q, rows, nil)
	if out.Count != 1 || !reflect.DeepEqual(out.Running, map[string]float64{"c": 450}) {
		t.Fatalf("filtered: %+v", out)
	}
	q.Filters = nil
	out = addUp(q, rows, nil)
	if !reflect.DeepEqual(out.IDs, []string{"c", "b"}) || !reflect.DeepEqual(out.Running, map[string]float64{"c": 450, "b": 150}) {
		t.Fatalf("page 1: %+v", out)
	}
	q.Page = 2
	if out := addUp(q, rows, nil); !reflect.DeepEqual(out.Running, map[string]float64{"a": 100}) {
		t.Fatalf("page 2: %+v", out.Running)
	}
	q.Running = ""
	if out := addUp(q, rows, nil); out.Running != nil {
		t.Fatalf("not asked: %v", out.Running)
	}
	if f := statFields(StatsQuery{DateKey: "date", Page: 1, Running: "amount"}); !f["amount"] || !f["code"] {
		t.Fatalf("fields: %v", f)
	}
}

func TestAPI_ListStats_RunningBalance(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	loc := orRiyadh(time.FixedZone("x", 3*3600))
	cr := call(t, "POST", "/capitals", tok, M{"storeId": storeA(), "date": time.Now().In(loc).Format("2006-01-02T15:04"),
		"investorUserId": fx.ManagerA.Hex(), "amount": 125, "method": "cash"})
	if cr.Code != 201 && cr.Code != 200 {
		t.Fatalf("create: %d %s", cr.Code, cr.Raw)
	}
	r := call(t, "GET", "/capitals/stats?storeId="+storeA()+"&sum=amount&page=1&limit=500&running=amount", tok, nil)
	if r.Code != 200 {
		t.Fatalf("capitals: %d %s", r.Code, r.Raw)
	}
	ids := arr(r.Body["ids"])
	run, _ := r.Body["running"].(M)
	if len(ids) < 2 || len(run) != len(ids) {
		t.Fatalf("running for %d of %d: %s", len(run), len(ids), r.Raw)
	}
	// the newest entry's running balance is the list's total
	if num(run[str(ids[0])]) != num(get(r.Body, "sums.amount")) || num(run[str(ids[1])]) >= num(run[str(ids[0])]) {
		t.Fatalf("newest balance %v, total %v", run[str(ids[0])], get(r.Body, "sums.amount"))
	}
}
