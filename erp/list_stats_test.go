package erp

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// ---- pure functions ----

func TestParseStatsQuery(t *testing.T) {
	cases := []struct {
		name, qs string
		wantErr  string // field
		check    func(q StatsQuery) bool
	}{
		{"defaults", "", "", func(q StatsQuery) bool {
			return q.DateKey == "date" && q.From == "" && q.To == "" && len(q.Sums) == 0 && len(q.SearchKeys) == len(listSearchKeys) && !q.IncludeDeleted
		}},
		{"range, sums, search, filters", "from=2020-01-01&to=2026-10-07&sum=net,+vat,,paid&q=+ACME+&keys=code,phone&f.party=c1&f.pstatus=paid&f.=x&f.empty=&includeDeleted=true",
			"", func(q StatsQuery) bool {
				return q.From == "2020-01-01" && q.To == "2026-10-07" && len(q.Sums) == 3 && q.Sums[1] == "vat" &&
					q.Search == "acme" && len(q.SearchKeys) == 2 && q.Filters["party"] == "c1" && q.Filters["pstatus"] == "paid" &&
					len(q.Filters) == 2 && q.IncludeDeleted
			}},
		{"dateKey", "dateKey=paidAt", "", func(q StatsQuery) bool { return q.DateKey == "paidAt" }},
		{"groupBy", "groupBy=+party+", "", func(q StatsQuery) bool { return q.GroupBy == "party" }},
		{"lines", "lines=payments", "", func(q StatsQuery) bool { return q.Lines == "payments" && statFields(q)[linesField] }},
		{"bad lines", "lines=items", "lines", nil},
		{"bad from", "from=yesterday", "from", nil},
		{"bad to", "to=2026-13-01", "to", nil},
		{"from after to", "from=2026-02-01&to=2026-01-01", "from", nil},
		{"too many sums", "sum=a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p,q,r,s,t,u", "sum", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, err := parseStatsQuery(httptest.NewRequest("GET", "/x?"+c.qs, nil))
			if c.wantErr != "" {
				ae, ok := err.(*APIError)
				if !ok || ae.Fields[c.wantErr] == "" {
					t.Fatalf("want error on %s, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil || !c.check(q) {
				t.Fatalf("got %+v err %v", q, err)
			}
		})
	}
}

func statsInv(id, date string, paid float64, extra M) M {
	rec := M{"id": id, "code": "S-" + id, "date": date, "vatPercent": 15.0, "customerId": "c1", "customerName": "Acme Trading",
		"phone": "0500", "createdBy": "Ali",
		"items":    []interface{}{M{"qty": 1.0, "unitPrice": 100.0, "purchasePrice": 60.0}},
		"payments": []interface{}{}}
	if paid > 0 {
		rec["payments"] = []interface{}{M{"amount": paid, "method": "cash"}}
	}
	for k, v := range extra {
		rec[k] = v
	}
	return rec
}

func TestStatsMatch(t *testing.T) {
	today := "2026-10-07"
	inv := statsInv("a", "2026-08-01T10:00", 50, M{"zatca": M{"status": "reported"}})
	unpaid := statsInv("b", "2026-10-06T23:59", 0, nil)
	vendorDoc := M{"id": "v", "date": "2019-01-01T00:00", "vendorId": "v9", "vendorName": "Supplier", "status": "received",
		"meta": M{"kind": "x"}, "amount": 12.5}
	deleted := statsInv("d", "2026-08-01T10:00", 0, M{"deleted": true})
	q := func(mod func(*StatsQuery)) StatsQuery {
		s := StatsQuery{DateKey: "date", SearchKeys: listSearchKeys, Filters: map[string]string{}, Today: today}
		if mod != nil {
			mod(&s)
		}
		return s
	}
	cases := []struct {
		name string
		q    StatsQuery
		rec  M
		cr   map[string]float64
		want bool
	}{
		{"no filters", q(nil), inv, nil, true},
		{"deleted hidden", q(nil), deleted, nil, false},
		{"deleted shown", q(func(s *StatsQuery) { s.IncludeDeleted = true }), deleted, nil, true},
		{"search name, any case", q(func(s *StatsQuery) { s.Search = "acme" }), inv, nil, true},
		{"search miss", q(func(s *StatsQuery) { s.Search = "zzz" }), inv, nil, false},
		{"search only given keys", q(func(s *StatsQuery) { s.Search = "acme"; s.SearchKeys = []string{"code"} }), inv, nil, false},
		{"search number field", q(func(s *StatsQuery) { s.Search = "12.5"; s.SearchKeys = []string{"amount"} }), vendorDoc, nil, true},
		{"from inclusive", q(func(s *StatsQuery) { s.From = "2026-08-01" }), inv, nil, true},
		{"before from", q(func(s *StatsQuery) { s.From = "2026-08-02" }), inv, nil, false},
		{"to inclusive (late in the day)", q(func(s *StatsQuery) { s.To = "2026-10-06" }), unpaid, nil, true},
		{"after to", q(func(s *StatsQuery) { s.To = "2026-10-05" }), unpaid, nil, false},
		{"all time keeps very old", q(nil), vendorDoc, nil, true},
		{"party customer", q(func(s *StatsQuery) { s.Filters["party"] = "c1" }), inv, nil, true},
		{"party vendor", q(func(s *StatsQuery) { s.Filters["party"] = "v9" }), vendorDoc, nil, true},
		{"party other", q(func(s *StatsQuery) { s.Filters["party"] = "c2" }), inv, nil, false},
		{"pstatus partly", q(func(s *StatsQuery) { s.Filters["pstatus"] = "paid_partially" }), inv, nil, true},
		{"pstatus not paid", q(func(s *StatsQuery) { s.Filters["pstatus"] = "not_paid" }), unpaid, nil, true},
		{"pstatus mismatch", q(func(s *StatsQuery) { s.Filters["pstatus"] = "paid" }), unpaid, nil, false},
		{"zatca reported", q(func(s *StatsQuery) { s.Filters["zatca"] = "reported" }), inv, nil, true},
		{"zatca default not_reported", q(func(s *StatsQuery) { s.Filters["zatca"] = "not_reported" }), unpaid, nil, true},
		{"method cash", q(func(s *StatsQuery) { s.Filters["paymentMethod"] = "cash" }), inv, nil, true},
		{"method none", q(func(s *StatsQuery) { s.Filters["paymentMethod"] = "cash" }), unpaid, nil, false},
		{"plain method field (expenses)", q(func(s *StatsQuery) { s.Filters["method"] = "bank" }), M{"id": "e", "method": "bank"}, nil, true},
		{"nonEmpty y", q(func(s *StatsQuery) { s.Filters["nonEmpty:vatAmount"] = "y" }), M{"id": "e", "vatAmount": 3.0}, nil, true},
		{"nonEmpty n", q(func(s *StatsQuery) { s.Filters["nonEmpty:vatAmount"] = "n" }), M{"id": "e", "vatAmount": 0.0}, nil, true},
		{"nonEmpty y on empty", q(func(s *StatsQuery) { s.Filters["nonEmpty:vatAmount"] = "y" }), M{"id": "e"}, nil, false},
		{"createdBy field", q(func(s *StatsQuery) { s.Filters["createdBy"] = "Ali" }), inv, nil, true},
		{"plain field", q(func(s *StatsQuery) { s.Filters["status"] = "received" }), vendorDoc, nil, true},
		{"dotted field", q(func(s *StatsQuery) { s.Filters["meta.kind"] = "x" }), vendorDoc, nil, true},
		{"overdue (67 days, open)", q(func(s *StatsQuery) { s.Filters["overdue"] = "30" }), inv, nil, true},
		{"overdue settled by credits", q(func(s *StatsQuery) { s.Filters["overdue"] = "30" }), inv, map[string]float64{"a": 65}, false},
		{"not overdue yet (1 day)", q(func(s *StatsQuery) { s.Filters["overdue"] = "30" }), unpaid, nil, false},
		{"overdue bad days", q(func(s *StatsQuery) { s.Filters["overdue"] = "x" }), inv, nil, false},
		{"all filters together", q(func(s *StatsQuery) {
			s.From, s.To, s.Search = "2026-01-01", "2026-12-31", "s-a"
			s.Filters["party"], s.Filters["pstatus"], s.Filters["paymentMethod"] = "c1", "paid_partially", "cash"
		}), inv, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statsMatch(c.q, statRowOf(c.rec, statFields(c.q)), c.cr); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestIsOverdueDay(t *testing.T) {
	cases := []struct {
		d     string
		bal   float64
		days  float64
		today string
		want  bool
	}{
		{"2026-09-06", 10, 30, "2026-10-07", true},
		{"2026-09-07", 10, 30, "2026-10-07", false}, // exactly 30 days is not "more than"
		{"2026-01-01", 0.004, 30, "2026-10-07", false},
		{"", 10, 30, "2026-10-07", false},
		{"2026-10-07", 10, -1, "2026-10-07", false}, // not before today
		{"bad", 10, 30, "2026-10-07", false},
	}
	for _, c := range cases {
		if got := isOverdueDay(c.d, c.bal, c.days, c.today); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestAddStats(t *testing.T) {
	res := &StatsResult{Sums: map[string]float64{}}
	sums := []string{"net", "vat", "paid", "balance", "taxable", "profit", "cost", "amount", "meta.x", "missing"}
	fields := statFields(StatsQuery{DateKey: "date", Sums: sums})
	addStats(res, sums, statRowOf(statsInv("a", "2026-01-01T00:00", 50, M{"amount": "7.25", "meta": M{"x": 2.0}}), fields))
	addStats(res, sums, statRowOf(statsInv("b", "2026-01-01T00:00", 0, M{"amount": 1.0}), fields))
	want := map[string]float64{"net": 230, "vat": 30, "paid": 50, "balance": 180, "taxable": 200, "profit": 80, "cost": 120,
		"amount": 8.25, "meta.x": 2, "missing": 0}
	if res.Count != 2 {
		t.Fatalf("count %d", res.Count)
	}
	for k, v := range want {
		if res.Sums[k] != v {
			t.Errorf("%s: got %v want %v", k, res.Sums[k], v)
		}
	}
}

func TestJSString(t *testing.T) {
	cases := map[string]interface{}{"": nil, "x": "x", "true": true, "false": false, "12.5": 12.5, "3": 3, "7": int64(7)}
	for want, in := range cases {
		if got := jsString(in); got != want {
			t.Errorf("%v: got %q want %q", in, got, want)
		}
	}
}

func TestHasListStats(t *testing.T) {
	names := map[string]bool{}
	for _, r := range Resources() {
		if hasListStats(r) {
			names[r.Name] = true
		}
	}
	for _, n := range []string{"sales", "nonvatSales", "salesReturns", "nonvatReturns", "purchases", "purchaseReturns",
		"quotations", "expenses", "deposits"} {
		if !names[n] {
			t.Errorf("%s has no /stats", n)
		}
	}
	for _, n := range []string{"customers", "products", "users", "roles", "stores"} {
		if names[n] {
			t.Errorf("%s must not have /stats", n)
		}
	}
}

// ---- API ----

func TestListStats_Unauthenticated(t *testing.T) {
	n := 0
	for _, r := range Resources() {
		if !hasListStats(r) {
			continue
		}
		n++
		for _, tok := range []string{"", "not-a-jwt"} {
			res := call(t, "GET", "/"+r.Path+"/stats?storeId=x", tok, nil)
			if res.Code != 401 || res.errCode() == "" {
				t.Errorf("%s/stats (token %q): %d %s", r.Path, tok, res.Code, res.Raw)
			}
		}
	}
	if n < 10 {
		t.Fatalf("expected /stats on the document lists, got %d", n)
	}
}

// All time covers records older than the web app's one-year window (the Umluj bug:
// "All time" showed one year's sales).
func TestAPI_ListStats_AllTimeBeyondWindow(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	// every sale, from the list (no window) — what the totals must add up to
	all := call(t, "GET", "/sales?storeId="+storeA()+"&limit=500", tok, nil)
	var wantNet, wantPaid float64
	for _, d := range all.data() {
		tt := ComputeTotals(d.(M))
		wantNet += tt.Net
		wantPaid += tt.Paid
	}
	r := call(t, "GET", "/sales/stats?storeId="+storeA()+"&sum=net,vat,paid,balance", tok, nil)
	if r.Code != 200 {
		t.Fatalf("stats: %d %s", r.Code, r.Raw)
	}
	if int(num(r.Body["count"])) != len(all.data()) || len(all.data()) < 2 {
		t.Fatalf("count %v, list %d", r.Body["count"], len(all.data()))
	}
	sums := r.Body["sums"].(M)
	if num(sums["net"]) != round2(wantNet) || num(sums["paid"]) != round2(wantPaid) {
		t.Fatalf("sums %v want net %v paid %v", sums, round2(wantNet), round2(wantPaid))
	}
	// the one-year window leaves the 700-day-old sale out
	loc := orRiyadh(time.FixedZone("x", 3*3600))
	from := time.Now().In(loc).AddDate(0, 0, -365).Format(layoutDay)
	w := call(t, "GET", "/sales/stats?storeId="+storeA()+"&from="+from+"&sum=net", tok, nil)
	if int(num(w.Body["count"])) != int(num(r.Body["count"]))-1 {
		t.Fatalf("window count %v vs all %v", w.Body["count"], r.Body["count"])
	}
	// a range holding only the old sale
	old := time.Now().In(loc).AddDate(0, 0, -700).Format(layoutDay)
	o := call(t, "GET", "/sales/stats?storeId="+storeA()+"&from="+old+"&to="+old+"&sum=net", tok, nil)
	if int(num(o.Body["count"])) != 1 {
		t.Fatalf("old day count %v %s", o.Body["count"], o.Raw)
	}
	// filters
	f := call(t, "GET", "/sales/stats?storeId="+storeA()+"&f.party="+fx.CustomerA2.Hex(), tok, nil)
	if int(num(f.Body["count"])) != 1 {
		t.Fatalf("party filter count %v", f.Body["count"])
	}
	// per customer: each group adds up that customer's sales, and the groups add up to all
	g := call(t, "GET", "/sales/stats?storeId="+storeA()+"&sum=balance&groupBy=party", tok, nil)
	groups, _ := g.Body["groups"].(M)
	wantBal := map[string]float64{}
	for _, d := range all.data() {
		wantBal[str(d.(M)["customerId"])] += ComputeTotals(d.(M)).Balance
	}
	if len(groups) != len(wantBal) || len(groups) < 2 {
		t.Fatalf("groups %v want %v", groups, wantBal)
	}
	gn := 0
	for id, want := range wantBal {
		gr, _ := groups[id].(M)
		if gr == nil || num(get(gr, "sums.balance")) != round2(want) {
			t.Fatalf("group %s = %v want %v", id, gr, round2(want))
		}
		gn += int(num(gr["count"]))
	}
	if gn != len(all.data()) {
		t.Fatalf("groups count %d of %d", gn, len(all.data()))
	}
	if _, ok := r.Body["groups"]; ok {
		t.Fatal("groups without groupBy")
	}
	// payment lines: every sale's payments, per method
	var wantPay float64
	wantN := 0
	for _, d := range all.data() {
		for _, p := range arr(d.(M)["payments"]) {
			wantPay += num(p.(M)["amount"])
			wantN++
		}
	}
	pl := call(t, "GET", "/sales/stats?storeId="+storeA()+"&lines=payments&sum=amount&groupBy=method", tok, nil)
	if int(num(pl.Body["count"])) != wantN || wantN == 0 || num(get(pl.Body, "sums.amount")) != round2(wantPay) {
		t.Fatalf("payment lines %v want %d / %v", pl.Raw, wantN, round2(wantPay))
	}
	if len(pl.Body["groups"].(M)) == 0 {
		t.Fatalf("payment lines per method: %s", pl.Raw)
	}
	s := call(t, "GET", "/sales/stats?storeId="+storeA()+"&q=S-INV-000&keys=code", tok, nil)
	if int(num(s.Body["count"])) != 1 {
		t.Fatalf("search count %v", s.Body["count"])
	}
	// cached: reading again does not re-read the list…
	before := atomic.LoadInt64(&statsBuilt)
	call(t, "GET", "/sales/stats?storeId="+storeA()+"&sum=net", tok, nil)
	if atomic.LoadInt64(&statsBuilt) != before {
		t.Fatal("unchanged list was read again")
	}
	// …until a sale is added through the app
	cr := call(t, "POST", "/sales", tok, M{"storeId": storeA(), "date": time.Now().In(loc).Format("2006-01-02T15:04"),
		"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 25, "warehouseId": "ms_" + storeA()}}})
	if cr.Code != 201 && cr.Code != 200 {
		t.Fatalf("create sale: %d %s", cr.Code, cr.Raw)
	}
	n := call(t, "GET", "/sales/stats?storeId="+storeA()+"&sum=net", tok, nil)
	if int(num(n.Body["count"])) != int(num(r.Body["count"]))+1 {
		t.Fatalf("new sale not counted: %v", n.Body["count"])
	}
	// a field not read before (a new search key) re-reads the list
	before = atomic.LoadInt64(&statsBuilt)
	call(t, "GET", "/sales/stats?storeId="+storeA()+"&q=x&keys=remarks", tok, nil)
	if atomic.LoadInt64(&statsBuilt) != before+1 {
		t.Fatal("new field not read")
	}
	// deleted records (an expense): out by default, in with includeDeleted
	cat := call(t, "POST", "/expense-categories", tok, M{"nameEn": "Stats tests", "storeId": storeA()})
	exp := call(t, "POST", "/expenses", tok, M{"storeId": storeA(), "date": time.Now().In(loc).Format("2006-01-02T15:04"),
		"amount": 100.0, "vatAmount": 15.0, "description": "Stats test", "method": "cash", "categoryId": cat.Body["id"]})
	if exp.Code != 201 {
		t.Fatalf("expense: %d %s", exp.Code, exp.Raw)
	}
	e0 := call(t, "GET", "/expenses/stats?storeId="+storeA()+"&sum=amount,vatAmount", tok, nil)
	if d := call(t, "DELETE", "/expenses/"+str(exp.Body["id"]), tok, nil); d.Code != 200 {
		t.Fatalf("delete: %d %s", d.Code, d.Raw)
	}
	e1 := call(t, "GET", "/expenses/stats?storeId="+storeA()+"&sum=amount,vatAmount", tok, nil)
	e2 := call(t, "GET", "/expenses/stats?storeId="+storeA()+"&sum=amount,vatAmount&includeDeleted=1", tok, nil)
	if num(e0.Body["count"])-1 != num(e1.Body["count"]) || num(e2.Body["count"]) != num(e0.Body["count"]) {
		t.Fatalf("deleted expense: %v %v %v", e0.Body["count"], e1.Body["count"], e2.Body["count"])
	}
	if round2(num(get(e0.Body, "sums.amount"))-num(get(e1.Body, "sums.amount"))) != num(exp.Body["amount"]) {
		t.Fatalf("expense amount sums: %v → %v", e0.Body["sums"], e1.Body["sums"])
	}
	// validation and access
	if bad := call(t, "GET", "/sales/stats?storeId="+storeA()+"&from=nope", tok, nil); bad.Code != 400 {
		t.Fatalf("bad from: %d", bad.Code)
	}
	if bad := call(t, "GET", "/sales/stats", tok, nil); bad.Code != 400 {
		t.Fatalf("no store: %d", bad.Code)
	}
	if other := call(t, "GET", "/sales/stats?storeId="+storeA(), login(t, fx.UserBEmail), nil); other.Code != 403 {
		t.Fatalf("other store's user: %d", other.Code)
	}
}

func TestStatFields(t *testing.T) {
	f := statFields(StatsQuery{DateKey: "paidAt", SearchKeys: []string{"code"}, Sums: []string{"net", "amount"},
		Filters: map[string]string{"party": "x", "createdBy": "y"}, GroupBy: "status"})
	for _, k := range []string{"paidAt", "date", "code", "amount", "createdBy", "status"} {
		if !f[k] {
			t.Errorf("missing %s", k)
		}
	}
	if statFields(StatsQuery{GroupBy: "party"})["party"] {
		t.Error("groupBy=party is derived, not a field")
	}
	for _, k := range []string{"net", "party"} {
		if f[k] {
			t.Errorf("%s is derived, not a field", k)
		}
	}
}

func TestStatsCacheEviction(t *testing.T) {
	statsMu.Lock()
	saved, savedMax := statsCache, StatsCacheMax
	statsCache, StatsCacheMax = map[string]*statsEntry{}, 2
	statsMu.Unlock()
	defer func() {
		statsMu.Lock()
		statsCache, StatsCacheMax = saved, savedMax
		statsMu.Unlock()
	}()
	a := statsEntryFor("a")
	statsEntryFor("b")
	a.used = time.Now().Add(-time.Minute) // a is the least recently read
	statsEntryFor("c")
	if _, ok := statsCache["a"]; ok || len(statsCache) != 2 {
		t.Fatalf("LRU eviction: %v", len(statsCache))
	}
	statsCache["b"].used = time.Now().Add(-StatsCacheIdle - time.Second)
	statsEntryFor("c")
	if _, ok := statsCache["b"]; ok {
		t.Fatal("idle entry kept")
	}
}

func TestRetailWholesaleProfit(t *testing.T) {
	rec := M{"items": []interface{}{
		M{"qty": 2.0, "unitPrice": 10.0, "unitDiscount": 1.0, "retailPrice": 15.0, "wholesalePrice": 12.0},
		M{"qty": 1.0, "unitPrice": 5.0}, // no prices: sold at cost
	}}
	r, w := retailWholesaleProfit(rec)
	if r != 12 || w != 6 {
		t.Fatalf("got %v %v", r, w)
	}
	res := &StatsResult{Sums: map[string]float64{}}
	addStats(res, []string{"retailProfit", "wholesaleProfit"}, statRowOf(rec, nil))
	if res.Sums["retailProfit"] != 12 || res.Sums["wholesaleProfit"] != 6 {
		t.Fatalf("sums %v", res.Sums)
	}
}

func TestConditionalAndNonEmptySums(t *testing.T) {
	sums := []string{"net|status=accepted", "nonEmpty:orderIds", "nonEmpty:orderId", "amount|kind=a", "net", "one|status=accepted", "one"}
	fields := statFields(StatsQuery{DateKey: "date", Sums: sums})
	res := &StatsResult{Sums: map[string]float64{}}
	addStats(res, sums, statRowOf(statsInv("a", "2026-01-01T00:00", 0, M{"status": "accepted", "orderIds": []interface{}{"o1"}, "kind": "a", "amount": 4.0}), fields))
	addStats(res, sums, statRowOf(statsInv("b", "2026-01-01T00:00", 0, M{"status": "pending", "orderIds": []interface{}{}, "orderId": "o9", "kind": "b", "amount": 9.0}), fields))
	addStats(res, sums, statRowOf(statsInv("c", "2026-01-01T00:00", 0, M{"status": "accepted"}), fields))
	want := map[string]float64{"net|status=accepted": 230, "nonEmpty:orderIds": 1, "nonEmpty:orderId": 1, "amount|kind=a": 4, "net": 345,
		"one|status=accepted": 2, "one": 3}
	for k, v := range want {
		if res.Sums[k] != v {
			t.Errorf("%s: got %v want %v", k, res.Sums[k], v)
		}
	}
	for _, c := range []struct {
		v    interface{}
		want bool
	}{{nil, false}, {"", false}, {"x", true}, {[]interface{}{}, false}, {[]interface{}{1}, true}, {true, true}, {false, false},
		{0.0, false}, {2.0, true}, {M{}, false}, {M{"a": 1}, true}} {
		if nonEmpty(c.v) != c.want {
			t.Errorf("nonEmpty(%v)", c.v)
		}
	}
	if b, f, v := splitMeasure("net|status"); b != "net" || f != "" || v != "" {
		t.Errorf("malformed condition: %q %q %q", b, f, v)
	}
}

// The shared fixture (= starterp-frontend-v1 tests/fixtures/list-stats-parity.json,
// written by the web app's oracle src/lib/listStatsServer.js): the server adds up the
// same records to the same figures as the mock API and the browser.
func TestListStats_ParityFixture(t *testing.T) {
	_, here, _, _ := runtime.Caller(0) // DB-backed runs change the working directory
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "testdata", "list_stats_parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Today   string             `json:"today"`
		Credits map[string]float64 `json:"credits"`
		Rows    []M                `json:"rows"`
		Cases   []struct {
			Query map[string]string `json:"query"`
			Want  StatsResult       `json:"want"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if len(fx.Cases) < 20 {
		t.Fatalf("fixture has %d cases", len(fx.Cases))
	}
	for i, c := range fx.Cases {
		v := url.Values{}
		for k, val := range c.Query {
			v.Set(k, val)
		}
		q, err := parseStatsQuery(httptest.NewRequest("GET", "/x?"+v.Encode(), nil))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		q.Today = fx.Today
		fields := statFields(q)
		rows := make([]statRow, len(fx.Rows))
		for j, rec := range fx.Rows {
			rows[j] = statRowOf(rec, fields)
		}
		res := addUp(q, rows, fx.Credits)
		sameStats(t, fmt.Sprintf("case %d %v", i, c.Query), res, &c.Want)
	}
}

func sameStats(t *testing.T, at string, res, want *StatsResult) {
	t.Helper()
	if res.Count != want.Count {
		t.Errorf("%s: count %d want %d", at, res.Count, want.Count)
	}
	for k, w := range want.Sums {
		if got := round2(res.Sums[k]); math.Abs(got-w) > 0.0001 {
			t.Errorf("%s: %s = %v want %v", at, k, got, w)
		}
	}
	if len(res.Groups) != len(want.Groups) {
		t.Errorf("%s: %d groups want %d", at, len(res.Groups), len(want.Groups))
	}
	for k, wg := range want.Groups {
		g := res.Groups[k]
		if g == nil {
			t.Errorf("%s: no group %q", at, k)
			continue
		}
		sameStats(t, at+" group "+k, g, wg)
	}
}
