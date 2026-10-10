package erp

// stats_oracle_test.go — every statistic the web app shows must equal an
// independent oracle computed straight from the raw documents in MongoDB.
//
// The oracle never calls the stats code (ComputeTotals, addUp, the dashboard
// formulas' loaders).  It reads the legacy documents the old system's own
// models wrote (net_total, vat_price, cash_discount … computed by models.Order
// & co. on save), the payment collections, and the raw `date` instants, and
// works out the store day of each document in the store's IANA timezone with
// time.In.  Then it compares, period by period:
//
//   - GET /<list>/stats count / net / vat / paid / balance (the list tiles)
//   - the list's own `total` (the pager) and /stats paging (every id once)
//   - groupBy=party (customer balances) and lines=payments (payments lists)
//   - /dashboard/revenue, /vat, /total-expense, /net-profit (old business
//     dashboard formulas, on the oracle's sums) and the /dashboard/feed rows
//
// in a Saudi (UTC+3), a UAE (UTC+4) and an India (UTC+5:30, rupee rounding)
// store, with documents a minute either side of month and day boundaries,
// discounts, shipping, cash discounts, split and late payments, partial
// returns, an invoice the old system deleted, a deleted expense and a sale
// older than the web app's one-year window.

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---- the oracle ----

type oracleDoc struct {
	ID, Party, Day         string
	Deleted                bool
	Net, Vat, CashDiscount float64
	Commission             float64
	Paid, Balance          float64
	Amount                 float64 // expenses: VAT included (legacy amount)
	Payments               []oraclePay
}

type oraclePay struct {
	Day, Method string
	Amount      float64
}

// oracleDocs reads a store collection as the old system saved it.
func oracleDocs(t *testing.T, sid, coll, payColl, payLink string, loc *time.Location) []oracleDoc {
	t.Helper()
	ctx, cancel := dbctx()
	defer cancel()
	day := func(v interface{}) string {
		switch d := v.(type) {
		case primitive.DateTime:
			return d.Time().In(loc).Format(layoutDay)
		case time.Time:
			return d.In(loc).Format(layoutDay)
		}
		return ""
	}
	pays := map[primitive.ObjectID][]oraclePay{}
	if payColl != "" {
		cur, err := storeDB(sid).Collection(payColl).Find(ctx, bson.M{"deleted": bson.M{"$ne": true}})
		if err != nil {
			t.Fatal(err)
		}
		for cur.Next(ctx) {
			var p bson.M
			_ = cur.Decode(&p)
			id, _ := p[payLink].(primitive.ObjectID)
			pays[id] = append(pays[id], oraclePay{Day: day(p["date"]), Method: fmt.Sprint(p["method"]), Amount: num(p["amount"])})
		}
		cur.Close(ctx)
	}
	cur, err := storeDB(sid).Collection(coll).Find(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	defer cur.Close(ctx)
	out := []oracleDoc{}
	for cur.Next(ctx) {
		var d bson.M
		_ = cur.Decode(&d)
		id := d["_id"].(primitive.ObjectID)
		o := oracleDoc{ID: id.Hex(), Day: day(d["date"]), Deleted: d["deleted"] == true, Net: num(d["net_total"]),
			Vat: num(d["vat_price"]), CashDiscount: num(d["cash_discount"]), Commission: num(d["commission"]), Amount: num(d["amount"])}
		if env, ok := d["erp"].(bson.M); ok && (env["del"] == true || env["hd"] == true) {
			o.Deleted = true
		}
		for _, k := range []string{"customer_id", "vendor_id"} {
			if p, ok := d[k].(primitive.ObjectID); ok {
				o.Party = p.Hex()
			}
		}
		o.Payments = pays[id]
		for _, p := range o.Payments {
			o.Paid += p.Amount
		}
		o.Paid = round2(o.Paid)
		o.Balance = math.Max(0, round2(o.Net-o.Paid-o.CashDiscount))
		out = append(out, o)
	}
	return out
}

type oracleSums struct {
	Count                            int
	Net, Vat, Paid, Balance, CashD   float64
	Commission, Amount, ExpenseNoVat float64
}

// in: the live documents of [from, to] (store days, "" = open).
func oracleIn(docs []oracleDoc, from, to string, keep func(oracleDoc) bool) oracleSums {
	var s oracleSums
	for _, d := range docs {
		if d.Deleted || (from != "" && d.Day < from) || (to != "" && d.Day > to) || (keep != nil && !keep(d)) {
			continue
		}
		s.Count++
		s.Net += d.Net
		s.Vat += d.Vat
		s.Paid += d.Paid
		s.Balance += d.Balance
		s.CashD += d.CashDiscount
		s.Commission += d.Commission
		s.Amount += d.Amount
		s.ExpenseNoVat += d.Amount - d.Vat
	}
	return s
}

func near(a, b float64) bool { return math.Abs(round2(a)-round2(b)) < 0.0051 }

// ---- the seeded stores ----

type oracleStore struct {
	cc, tok, sid, ms string
	loc              *time.Location
	vat              float64
	c1, c2, v1, p1   string
	saleIDs          []string
}

func oracleSignup(t *testing.T, cc string) *oracleStore {
	t.Helper()
	var body M
	switch cc {
	case "IN":
		body = indiaSignup()
	default:
		body = validSignup()
		if cc != "SA" {
			co := body["company"].(M)
			co["countryCode"], co["vatNo"], co["mobile"] = cc, "", "+971501234567"
			co["address"] = M{"streetEn": "Sheikh Zayed Road", "cityEn": "Dubai"}
		}
	}
	body["owner"].(M)["email"] = "oracle-" + strings.ToLower(cc) + "+" + time.Now().Format("150405.000000") + "@signup.example"
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("signup %s: %d %s", cc, r.Code, r.Raw)
	}
	s := &oracleStore{cc: cc, tok: str(r.Body["accessToken"]), sid: str(get(r.Body, "store.id")),
		vat: num(get(r.Body, "store.vatPercent"))}
	s.ms = "ms_" + s.sid
	s.loc = storeLocation(M{"country_code": cc})
	return s
}

func (s *oracleStore) post(t *testing.T, path string, body M) M {
	t.Helper()
	body["storeId"] = s.sid
	r := call(t, "POST", path, s.tok, body)
	if r.Code != 201 {
		t.Fatalf("%s POST %s: %d %s", s.cc, path, r.Code, r.Raw)
	}
	return r.Body
}

func (s *oracleStore) get(t *testing.T, path string) M {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	r := call(t, "GET", path+sep+"storeId="+s.sid, s.tok, nil)
	if r.Code != 200 {
		t.Fatalf("%s GET %s: %d %s", s.cc, path, r.Code, r.Raw)
	}
	return r.Body
}

// seed writes the corner cases through the API (as the web app does) and returns
// the store days used: the last day of last month (lm), the first of this month
// (fm) and an old day beyond the one-year window.
func (s *oracleStore) seed(t *testing.T) (lm, fm, old string) {
	t.Helper()
	now := time.Now().In(s.loc)
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.loc)
	last := first.AddDate(0, 0, -1)
	lm, fm = last.Format(layoutDay), first.Format(layoutDay)
	oldT := now.AddDate(0, 0, -400)
	old = oldT.Format(layoutDay)
	wall := func(d string, hm string) string { return d + "T" + hm }

	vatNo := ""
	if s.cc == "SA" {
		vatNo = "300000000000003"
	}
	phone := map[string][3]string{"SA": {"0551000001", "0551000002", "0551000003"}, "AE": {"0501000001", "0501000002", "0501000003"},
		"IN": {"9876500001", "9876500002", "9876500003"}}[s.cc]
	s.c1 = str(s.post(t, "/customers", M{"nameEn": "Oracle Customer One", "phone": phone[0], "vatNo": vatNo})["id"])
	s.c2 = str(s.post(t, "/customers", M{"nameEn": "Oracle Customer Two", "phone": phone[1]})["id"])
	s.v1 = str(s.post(t, "/vendors", M{"nameEn": "Oracle Vendor", "phone": phone[2]})["id"])
	s.p1 = str(s.post(t, "/products", M{"nameEn": "Oracle Widget", "pricing": M{"retail": 100.0, "purchase": 61.37}})["id"])
	line := func(qty, price, disc float64) []M {
		return []M{{"productId": s.p1, "qty": qty, "unitPrice": price, "unitDiscount": disc, "warehouseId": s.ms,
			"vatPercent": s.vat, "purchasePrice": 61.37}}
	}
	sale := func(c, when string, items []M, extra M) string {
		b := M{"date": when, "customerId": c, "items": items}
		for k, v := range extra {
			b[k] = v
		}
		id := str(s.post(t, "/sales", b)["id"])
		s.saleIDs = append(s.saleIDs, id)
		return id
	}
	// a minute before and after midnight at the month boundary (store time)
	sLate := sale(s.c1, wall(lm, "23:59"), line(3, 33.33, 1.11), M{"discount": 5.0, "shipping": 12.5,
		"payments": []M{{"date": wall(lm, "23:59"), "amount": 50.0, "method": "cash"}, {"date": wall(fm, "09:00"), "amount": 20.0, "method": "bank_transfer"}}})
	sEarly := sale(s.c2, wall(fm, "00:00"), line(2, 100.0, 0), M{ // paid in full, by card
		"payments": []M{{"date": wall(fm, "00:00"), "amount": round2(200 * (1 + s.vat/100)), "method": "debit_card"}}})
	sale(s.c1, wall(fm, "00:01"), line(7, 9.87, 0.5), M{"cashDiscount": 2.0, "commission": 3.0,
		"payments": []M{{"date": wall(fm, "00:01"), "amount": 10.0, "method": "cash"}}})
	sale(s.c2, wall(fm, "12:00"), line(1, 250.0, 0), nil) // not paid
	sale(s.c1, wall(old, "10:00"), line(4, 75.5, 0), M{"payments": []M{{"date": wall(old, "10:00"), "amount": 100.0, "method": "cash"}}})
	// an invoice the old system deleted (legacy deleted:true; the app cannot delete sales)
	gone := sale(s.c2, wall(fm, "08:00"), line(5, 40.0, 0), nil)
	oid, _ := primitive.ObjectIDFromHex(gone)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := storeDB(s.sid).Collection("order").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"deleted": true, "updated_at": time.Now()}}); err != nil {
		t.Fatal(err)
	}
	// a partial return of the late sale, on the first day of this month, refunded in part
	s.post(t, "/sales-returns", M{"date": wall(fm, "10:00"), "orderId": sLate, "customerId": s.c1,
		"items": line(1, 33.33, 1.11), "payments": []M{{"date": wall(fm, "10:00"), "amount": 10.0, "method": "cash"}}})
	_ = sEarly
	// purchases and a return
	pur := str(s.post(t, "/purchases", M{"date": wall(lm, "23:30"), "vendorId": s.v1, "vendorInvoiceNo": uniq("OV"),
		"items":    []M{{"productId": s.p1, "qty": 10, "unitPrice": 61.37, "warehouseId": s.ms, "vatPercent": s.vat}},
		"payments": []M{{"date": wall(lm, "23:30"), "amount": 300.0, "method": "bank_transfer"}}})["id"])
	s.post(t, "/purchase-returns", M{"date": wall(fm, "00:30"), "purchaseId": pur, "vendorId": s.v1,
		"items": []M{{"productId": s.p1, "qty": 2, "unitPrice": 61.37, "warehouseId": s.ms, "vatPercent": s.vat}}})
	// expenses: one with VAT at the boundary, one deleted
	cat := str(s.post(t, "/expense-categories", M{"nameEn": "Oracle " + s.cc})["id"])
	s.post(t, "/expenses", M{"date": wall(lm, "23:59"), "amount": 200.0, "vatAmount": 10.0, "description": "Rent", "method": "cash", "categoryId": cat})
	s.post(t, "/expenses", M{"date": wall(fm, "00:00"), "amount": 80.0, "vatAmount": 0.0, "description": "Water", "method": "bank_transfer", "categoryId": cat})
	del := s.post(t, "/expenses", M{"date": wall(fm, "11:00"), "amount": 999.0, "vatAmount": 0.0, "description": "Mistake", "method": "cash", "categoryId": cat})
	if r := call(t, "DELETE", "/expenses/"+str(del["id"]), s.tok, nil); r.Code != 200 {
		t.Fatalf("delete expense: %d %s", r.Code, r.Raw)
	}
	// a salary paid on the first day of the month (its date field is paymentDate)
	emp := str(s.post(t, "/employees", M{"nameEn": "Oracle Employee", "joinDate": "2026-01-01", "basicSalary": 3000, "status": "active",
		"phone": phone[0]})["id"])
	s.post(t, "/salaries", M{"employeeId": emp, "period": fm[:7], "netSalary": 3000.0, "paymentDate": fm, "method": "cash"})
	return
}

// ---- the comparisons ----

type oracleList struct {
	path, coll, payColl, payLink string
}

var oracleLists = []oracleList{
	{"sales", "order", "sales_payment", "order_id"},
	{"sales-returns", "salesreturn", "sales_return_payment", "sales_return_id"},
	{"purchases", "purchase", "purchase_payment", "purchase_id"},
	{"purchase-returns", "purchasereturn", "purchase_return_payment", "purchase_return_id"},
}

func TestAPI_StatsOracle(t *testing.T) {
	requireDB(t)
	for _, cc := range []string{"SA", "AE", "IN"} {
		cc := cc
		t.Run(cc, func(t *testing.T) {
			s := oracleSignup(t, cc)
			defer cleanupStore(t, s.sid)
			lm, fm, old := s.seed(t)
			lmFirst := lm[:8] + "01"
			fmLast := time.Date(atoiDay(fm, 0), time.Month(atoiDay(fm, 1)), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, -1).Format(layoutDay)
			periods := [][2]string{
				{"", ""},          // all time
				{lmFirst, lm},     // last month
				{fm, fmLast},      // this month
				{lm, lm},          // the day before the boundary
				{fm, fm},          // the day after
				{old, old},        // beyond the one-year window
				{lmFirst, fmLast}, // both months
				{time.Now().In(s.loc).AddDate(0, 0, -365).Format(layoutDay), ""}, // the browser's window
			}
			raw := map[string][]oracleDoc{}
			for _, l := range oracleLists {
				raw[l.path] = oracleDocs(t, s.sid, l.coll, l.payColl, l.payLink, s.loc)
			}
			raw["expenses"] = oracleDocs(t, s.sid, "expense", "", "", s.loc)

			// the oracle itself must see the corner cases it is meant to test
			if o := oracleIn(raw["sales"], lm, lm, nil); o.Count != 1 {
				t.Fatalf("oracle: %d sales on %s (the 23:59 sale)", o.Count, lm)
			}
			if o := oracleIn(raw["sales"], fm, fm, nil); o.Count != 3 {
				t.Fatalf("oracle: %d live sales on %s", o.Count, fm)
			}

			for _, l := range oracleLists {
				for _, p := range periods {
					checkListStats(t, s, l.path, raw[l.path], p)
				}
			}
			for _, p := range periods {
				checkExpenseStats(t, s, raw["expenses"], p)
			}
			checkPartyBalances(t, s, raw["sales"])
			checkPaymentLines(t, s, raw["sales"], periods)
			checkStatsPaging(t, s)
			for _, p := range periods[1:] {
				checkDashboards(t, s, raw, p)
			}
			checkFeed(t, s, raw["sales"], lmFirst, fmLast)
			checkSalaryPeriods(t, s, lm, fm)
			if cc == "SA" {
				checkStoreWithoutCountry(t, s, raw, [][2]string{{lm, lm}, {fm, fm}, {lmFirst, lm}})
			}
			checkFiguresFollowWrites(t, s, fm)
		})
	}
}

func atoiDay(d string, part int) int {
	var y, m, dd int
	fmt.Sscanf(d, "%d-%d-%d", &y, &m, &dd)
	return []int{y, m, dd}[part]
}

func periodQS(p [2]string) string {
	v := url.Values{}
	if p[0] != "" {
		v.Set("from", p[0])
	}
	if p[1] != "" {
		v.Set("to", p[1])
	}
	if e := v.Encode(); e != "" {
		return "&" + e
	}
	return ""
}

// The list tiles (count, total, VAT, paid, balance) and the pager.
func checkListStats(t *testing.T, s *oracleStore, path string, docs []oracleDoc, p [2]string) {
	t.Helper()
	at := fmt.Sprintf("%s /%s %v", s.cc, path, p)
	want := oracleIn(docs, p[0], p[1], nil)
	got := s.get(t, "/"+path+"/stats?sum=net,vat,paid,balance,one"+periodQS(p))
	if int(num(got["count"])) != want.Count || int(num(get(got, "sums.one"))) != want.Count {
		t.Errorf("%s: count %v want %d", at, got["count"], want.Count)
	}
	for k, w := range map[string]float64{"net": want.Net, "vat": want.Vat, "paid": want.Paid, "balance": want.Balance} {
		if g := num(get(got, "sums."+k)); !near(g, w) {
			t.Errorf("%s: %s %v want %v (raw documents)", at, k, g, round2(w))
		}
	}
	// the pager: the list's own total for the same period
	lst := s.get(t, "/"+path+"?limit=1"+periodQS(p))
	if int(num(lst["total"])) != want.Count {
		t.Errorf("%s: list total %v, tiles %d", at, lst["total"], want.Count)
	}
	// with includeDeleted the deleted ones come back too
	all := oracleIn(docs, p[0], p[1], nil)
	for _, d := range docs {
		if d.Deleted && (p[0] == "" || d.Day >= p[0]) && (p[1] == "" || d.Day <= p[1]) {
			all.Count++
		}
	}
	if inc := s.get(t, "/"+path+"/stats?sum=one&includeDeleted=1"+periodQS(p)); int(num(inc["count"])) != all.Count {
		t.Errorf("%s: includeDeleted count %v want %d", at, inc["count"], all.Count)
	}
}

func checkExpenseStats(t *testing.T, s *oracleStore, docs []oracleDoc, p [2]string) {
	t.Helper()
	at := fmt.Sprintf("%s /expenses %v", s.cc, p)
	want := oracleIn(docs, p[0], p[1], nil)
	got := s.get(t, "/expenses/stats?sum=amount,vatAmount"+periodQS(p))
	if int(num(got["count"])) != want.Count {
		t.Errorf("%s: count %v want %d", at, got["count"], want.Count)
	}
	if g := num(get(got, "sums.amount")); !near(g, want.ExpenseNoVat) {
		t.Errorf("%s: amount (before VAT) %v want %v", at, g, round2(want.ExpenseNoVat))
	}
	if g := num(get(got, "sums.vatAmount")); !near(g, want.Vat) {
		t.Errorf("%s: VAT %v want %v", at, g, round2(want.Vat))
	}
	if lst := s.get(t, "/expenses?limit=1"+periodQS(p)); int(num(lst["total"])) != want.Count {
		t.Errorf("%s: list total %v, tiles %d", at, lst["total"], want.Count)
	}
}

// Customer balances (customers list, statement): per customer, all time.
func checkPartyBalances(t *testing.T, s *oracleStore, sales []oracleDoc) {
	t.Helper()
	got := s.get(t, "/sales/stats?sum=net,paid,balance&groupBy=party")
	groups, _ := got["groups"].(M)
	for _, c := range []string{s.c1, s.c2} {
		want := oracleIn(sales, "", "", func(d oracleDoc) bool { return d.Party == c })
		g, _ := groups[c].(M)
		if g == nil || int(num(g["count"])) != want.Count || !near(num(get(g, "sums.balance")), want.Balance) ||
			!near(num(get(g, "sums.net")), want.Net) || !near(num(get(g, "sums.paid")), want.Paid) {
			t.Errorf("%s customer %s: %v want count %d net %v paid %v balance %v", s.cc, c, g, want.Count,
				round2(want.Net), round2(want.Paid), round2(want.Balance))
		}
	}
}

// The Payments list: each payment line in its own period (the payment's date) per method.
func checkPaymentLines(t *testing.T, s *oracleStore, sales []oracleDoc, periods [][2]string) {
	t.Helper()
	for _, p := range periods {
		want := map[string]float64{}
		var total float64
		n := 0
		for _, d := range sales {
			if d.Deleted {
				continue
			}
			for _, pay := range d.Payments {
				if (p[0] != "" && pay.Day < p[0]) || (p[1] != "" && pay.Day > p[1]) {
					continue
				}
				want[pay.Method] += pay.Amount
				total += pay.Amount
				n++
			}
		}
		got := s.get(t, "/sales/stats?lines=payments&sum=amount&groupBy=method"+periodQS(p))
		if int(num(got["count"])) != n || !near(num(get(got, "sums.amount")), total) {
			t.Errorf("%s payments %v: %v / %v want %d / %v", s.cc, p, got["count"], get(got, "sums.amount"), n, round2(total))
		}
		groups, _ := got["groups"].(M)
		for m, w := range want {
			if g, _ := groups[m].(M); g == nil || !near(num(get(g, "sums.amount")), w) {
				t.Errorf("%s payments %v method %s: %v want %v", s.cc, p, m, g, round2(w))
			}
		}
	}
}

// Paging a list on the server: every record exactly once, in date order, and
// the pager total is the count tile.
func checkStatsPaging(t *testing.T, s *oracleStore) {
	t.Helper()
	all := s.get(t, "/sales/stats?sum=one")
	n := int(num(all["count"]))
	seen := map[string]bool{}
	for page := 1; page <= n; page++ {
		r := s.get(t, fmt.Sprintf("/sales/stats?sum=one&page=%d&limit=2", page))
		if int(num(r["count"])) != n {
			t.Errorf("%s page %d count %v want %d", s.cc, page, r["count"], n)
		}
		ids := arr(r["ids"])
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			if seen[str(id)] {
				t.Errorf("%s: %s on two pages", s.cc, id)
			}
			seen[str(id)] = true
		}
	}
	if len(seen) != n {
		t.Errorf("%s: pages hold %d of %d sales", s.cc, len(seen), n)
	}
}

// The dashboard figures (old business dashboard formulas) on the oracle's sums,
// and against the list tiles of the same period.
func checkDashboards(t *testing.T, s *oracleStore, raw map[string][]oracleDoc, p [2]string) {
	t.Helper()
	at := fmt.Sprintf("%s dashboard %v", s.cc, p)
	in := func(path string) oracleSums { return oracleIn(raw[path], p[0], p[1], nil) }
	sales, sret, pur, pret, exp := in("sales"), in("sales-returns"), in("purchases"), in("purchase-returns"), in("expenses")

	rev := s.get(t, "/dashboard/revenue?fresh=1"+periodQS(p))
	wantRev := sales.Net - sret.Net
	if g := num(get(rev, "result.revenue")); !near(g, wantRev) {
		t.Errorf("%s: revenue %v want %v (sales %v − returns %v)", at, g, round2(wantRev), round2(sales.Net), round2(sret.Net))
	}
	// …and the Sales and Sales returns tiles of the same period agree with it
	st := s.get(t, "/sales/stats?sum=net"+periodQS(p))
	rt := s.get(t, "/sales-returns/stats?sum=net"+periodQS(p))
	if g := num(get(rev, "result.revenue")); !near(g, num(get(st, "sums.net"))-num(get(rt, "sums.net"))) {
		t.Errorf("%s: revenue %v but Sales − Returns tiles %v − %v", at, g, get(st, "sums.net"), get(rt, "sums.net"))
	}

	vat := s.get(t, "/dashboard/vat?fresh=1"+periodQS(p))
	wantVat := sales.Vat - sret.Vat - (pur.Vat - pret.Vat)
	if g := num(get(vat, "result.vatPayable")); !near(g, wantVat) {
		t.Errorf("%s: VAT payable %v want %v", at, g, round2(wantVat))
	}

	te := s.get(t, "/dashboard/total-expense?fresh=1"+periodQS(p))
	wantExp := exp.Amount + (pur.Net - pret.Net) + (sales.CashD - sret.CashD + pret.CashD - pur.CashD) + (sales.Commission - sret.Commission)
	if g := num(get(te, "result.total")); !near(g, wantExp) {
		t.Errorf("%s: total expense %v want %v", at, g, round2(wantExp))
	}

	np := s.get(t, "/dashboard/net-profit?fresh=1"+periodQS(p))
	wantNP := wantRev - wantExp
	if g := firstNum(np, "result.netProfit", "result.profit", "netProfit"); !near(g, wantNP) {
		t.Errorf("%s: net profit %v want %v (revenue − expense) %s", at, g, round2(wantNP), mustJSON(np))
	}
}

func firstNum(m M, paths ...string) float64 {
	for _, p := range paths {
		if v := get(m, p); v != nil {
			return num(v)
		}
	}
	return math.NaN()
}

// The main dashboard's feed: each sale's numbers and store day, for the window.
func checkFeed(t *testing.T, s *oracleStore, sales []oracleDoc, from, to string) {
	t.Helper()
	f := s.get(t, "/dashboard/feed?fresh=1")
	byID := map[string]M{}
	for _, d := range arr(get(f, "feed.sales")) {
		byID[str(d.(M)["id"])] = d.(M)
	}
	want := oracleIn(sales, from, to, nil)
	var net, paid, bal float64
	n := 0
	for _, d := range byID {
		day := first10(str(d["date"]))
		if day < from || day > to {
			continue
		}
		n++
		net += num(d["net"])
		paid += num(d["paid"])
		bal += num(d["balance"])
	}
	if n != want.Count || !near(net, want.Net) || !near(paid, want.Paid) || !near(bal, want.Balance) {
		t.Errorf("%s feed %s..%s: %d sales net %v paid %v balance %v want %d / %v / %v / %v", s.cc, from, to, n,
			round2(net), round2(paid), round2(bal), want.Count, round2(want.Net), round2(want.Paid), round2(want.Balance))
	}
	for _, d := range sales {
		if d.Deleted && byID[d.ID] != nil {
			t.Errorf("%s feed: deleted sale %s is on the dashboard", s.cc, d.ID)
		}
	}
}

// A new sale shows in the tiles and dashboard figures without a forced rebuild.
func checkFiguresFollowWrites(t *testing.T, s *oracleStore, fm string) {
	t.Helper()
	qs := "&from=" + fm + "&to=" + fm
	rev0 := num(get(s.get(t, "/dashboard/revenue?x=1"+qs), "result.revenue"))
	st0 := s.get(t, "/sales/stats?sum=net"+qs)
	b := s.post(t, "/sales", M{"date": fm + "T15:00", "customerId": s.c1, "items": []M{{"productId": s.p1, "qty": 1, "unitPrice": 10.0,
		"warehouseId": s.ms, "vatPercent": s.vat}}})
	oid, _ := primitive.ObjectIDFromHex(str(b["id"]))
	ctx, cancel := dbctx()
	defer cancel()
	var d bson.M
	_ = storeDB(s.sid).Collection("order").FindOne(ctx, bson.M{"_id": oid}).Decode(&d)
	add := num(d["net_total"])
	st1 := s.get(t, "/sales/stats?sum=net"+qs)
	if int(num(st1["count"])) != int(num(st0["count"]))+1 || !near(num(get(st1, "sums.net")), num(get(st0, "sums.net"))+add) {
		t.Errorf("%s: new sale not in the tiles: %v → %v (+%v)", s.cc, get(st0, "sums.net"), get(st1, "sums.net"), add)
	}
	if rev1 := num(get(s.get(t, "/dashboard/revenue?x=1"+qs), "result.revenue")); !near(rev1, rev0+add) {
		t.Errorf("%s: new sale not in the dashboard revenue: %v → %v (+%v)", s.cc, rev0, rev1, add)
	}
}

func mustJSON(v interface{}) string {
	keys := []string{}
	if m, ok := v.(M); ok {
		for k := range m {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return fmt.Sprint(keys)
}

// Salaries are dated by paymentDate: a period without ?dateKey counts them by it
// (the list's own date field), as the list does.
func checkSalaryPeriods(t *testing.T, s *oracleStore, lm, fm string) {
	t.Helper()
	for _, c := range []struct {
		qs   string
		want int
	}{{"&from=" + fm + "&to=" + fm, 1}, {"&from=" + lm + "&to=" + lm, 0}, {"", 1}, {"&from=" + fm + "&dateKey=paymentDate", 1}} {
		got := s.get(t, "/salaries/stats?sum=netSalary"+c.qs)
		if int(num(got["count"])) != c.want || (c.want == 1 && num(get(got, "sums.netSalary")) != 3000) {
			t.Errorf("%s salaries%s: %v want %d", s.cc, c.qs, got, c.want)
		}
		if lst := s.get(t, "/salaries?limit=1"+strings.Replace(c.qs, "&dateKey=paymentDate", "", 1)); int(num(lst["total"])) != c.want {
			t.Errorf("%s salaries list%s: total %v want %d", s.cc, c.qs, lst["total"], c.want)
		}
	}
}

// A store saved without a country code (older stores) or with a lower-case one is
// a Saudi store everywhere: the dashboard figures use Riyadh days like the lists.
func checkStoreWithoutCountry(t *testing.T, s *oracleStore, raw map[string][]oracleDoc, periods [][2]string) {
	t.Helper()
	oid, _ := primitive.ObjectIDFromHex(s.sid)
	ctx, cancel := dbctx()
	defer cancel()
	for _, cc := range []string{"", "sa"} {
		if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"country_code": cc}}); err != nil {
			t.Fatal(err)
		}
		for _, p := range periods {
			checkDashboards(t, s, raw, p)
		}
	}
	if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"country_code": "SA"}}); err != nil {
		t.Fatal(err)
	}
}
