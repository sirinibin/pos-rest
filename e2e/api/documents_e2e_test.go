//go:build e2e

package api

import (
	"fmt"
	"math"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Selling documents besides sales (quotations, quotation returns, proformas,
// delivery notes, non-VAT sales and returns), drafts, POS records and numbers,
// and the workshop/customer resources (packages, vehicles, repair jobs,
// signatures).

// ---------- helpers (docs prefix: shared package, other files run alongside) ----------

func docsR2(x float64) float64 { return math.Round(x*100) / 100 }

// docsLn is one document line for the totals oracle.
type docsLn struct{ q, p, d, cost float64 }

// docsOracle computes a sales document's totals the way the contract defines
// them: total = Σ qty × (price − discount); VAT on (total − discount +
// shipping) at the document rate; profit = Σ of the lines' positive margins.
func docsOracle(lines []docsLn, discount, shipping, vatPct float64) (total, vat, net, profit float64) {
	for _, l := range lines {
		total += l.q * (l.p - l.d)
		if m := l.q*(l.p-l.d) - l.q*l.cost; m > 0 {
			profit += m
		}
	}
	total = docsR2(total)
	taxable := total - discount + shipping
	vat = docsR2(taxable * vatPct / 100)
	net = docsR2(taxable + vat)
	return total, vat, net, docsR2(profit)
}

func docsLineD(s *Store, pid string, qty, price, disc float64) M {
	l := s.Line(pid, qty, price)
	l["unitDiscount"] = disc
	return l
}

// docsWantErr fails unless r has the status and names every field in error.fields.
func docsWantErr(t *testing.T, r Resp, status int, what string, fields ...string) {
	t.Helper()
	if r.Code != status {
		t.Errorf("%s: want HTTP %d, got %s", what, status, r)
		return
	}
	for _, f := range fields {
		if r.ErrField(f) == "" {
			t.Errorf("%s: want error.fields[%s], got %s", what, f, r)
		}
	}
}

func docsTotals(t *testing.T, what string, rec M, total, vat, net float64) {
	t.Helper()
	EqMoney(t, what+" total", F(rec, "legacyTotals.total"), total)
	EqMoney(t, what+" vat", F(rec, "legacyTotals.vat"), vat)
	EqMoney(t, what+" net", F(rec, "legacyTotals.net"), net)
}

func docsCodes(rows []M) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, S(r["code"]))
	}
	return out
}

func docsHas(ids []M, id string) bool {
	for _, r := range ids {
		if S(r["id"]) == id {
			return true
		}
	}
	return false
}

// docsStats GETs /<path>/stats with the query and returns the body.
func docsStats(t *testing.T, s *Store, path, query string) M {
	t.Helper()
	return Must(t, Call(t, "GET", "/"+path+"/stats?storeId="+s.ID+"&"+query, s.Token, nil), 200, "stats "+path).Body
}

// docsPut replaces a record with its current contract body plus changes.
func docsPut(t *testing.T, tok, path, id string, change M) Resp {
	t.Helper()
	cur := Read(t, tok, path, id)
	for k, v := range change {
		cur[k] = v
	}
	return Call(t, "PUT", "/"+path+"/"+id, tok, cur)
}

func docsStockEventually(t *testing.T, s *Store, pid string, want float64) {
	t.Helper()
	var got float64
	Eventually(t, fmt.Sprintf("stock of %s = %v", pid, want), func() bool {
		got = s.Stock(t, pid)
		return got == want
	})
}

// docsStockStays checks the stock has not moved after the background work had time to run.
func docsStockStays(t *testing.T, s *Store, pid string, want float64) {
	t.Helper()
	time.Sleep(1500 * time.Millisecond)
	if got := s.Stock(t, pid); got != want {
		t.Errorf("stock of %s moved: got %v, want %v", pid, got, want)
	}
}

// docsRoutes calls every generic route of a resource once for success and once
// for an error, with a live record id and an id that does not exist.
func docsRoutes(t *testing.T, s *Store, path, live string, stats bool, missing string) {
	t.Helper()
	Must(t, Call(t, "GET", "/"+path+"?storeId="+s.ID+"&limit=1", s.Token, nil), 200, "list "+path)
	Must(t, Call(t, "GET", "/"+path, s.Token, nil), 400, "list without store "+path)
	Must(t, Call(t, "GET", "/"+path+"/"+live, s.Token, nil), 200, "get "+path)
	Must(t, Call(t, "GET", "/"+path+"/"+missing, s.Token, nil), 404, "get missing "+path)
	Must(t, Call(t, "PATCH", "/"+path+"/"+missing, s.Token, M{"remarks": "x"}), 404, "patch missing "+path)
	Must(t, Call(t, "PUT", "/"+path+"/"+missing, s.Token, M{"remarks": "x"}), 404, "put missing "+path)
	Must(t, Call(t, "DELETE", "/"+path+"/"+missing, s.Token, nil), 404, "delete missing "+path)
	Must(t, Call(t, "POST", "/"+path+"/"+missing+"/restore", s.Token, nil), 404, "restore missing "+path)
	// restoring a record that is not deleted answers it unchanged
	if r := Must(t, Call(t, "POST", "/"+path+"/"+live+"/restore", s.Token, nil), 200, "restore live "+path).Body; r["deleted"] != false {
		t.Errorf("%s restore of a live record: %v", path, r["deleted"])
	}
	if stats {
		Must(t, Call(t, "GET", "/"+path+"/stats?storeId="+s.ID+"&sum=one", s.Token, nil), 200, "stats "+path)
		Must(t, Call(t, "GET", "/"+path+"/stats?storeId="+s.ID+"&from=2026-01-32", s.Token, nil), 400, "stats bad from "+path)
	}
}

const docsPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// ---------- quotations ----------

func TestDocsQuotations(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	p := s.Product(t, 10, 20, 50)
	pid := S(p["id"])
	cid := S(s.Customer(t, "")["id"])
	vatPct := F(s.Rec, "vatPercent")
	base := func(items ...M) M {
		return M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "validityDays": 7, "deliveryDays": 3, "items": items}
	}
	var q1, qi M

	t.Run("plain quotation: totals oracle, default type, payments dropped, no stock move", func(t *testing.T) {
		b := base(docsLineD(s, pid, 3, 33.33, 0), docsLineD(s, pid, 1, 0.1, 0), docsLineD(s, pid, 7, 10.07, 0.03))
		b["status"] = "created"
		b["payments"] = []M{{"date": s.Now(), "amount": 50, "method": "cash"}}
		q1 = Create(t, s.Token, "quotations", b)
		total, vat, net, profit := docsOracle([]docsLn{{3, 33.33, 0, 10}, {1, 0.1, 0, 10}, {7, 10.07, 0.03, 10}}, 0, 0, vatPct)
		docsTotals(t, "quotation", q1, total, vat, net)
		EqMoney(t, "quotation profit", F(q1, "legacyTotals.profit"), profit)
		if q1["type"] != "quotation" || q1["status"] != "created" || S(q1["code"]) != "QTN-001" || F(q1, "version") != 1 {
			t.Errorf("type/status/code/version: %v %v %v %v", q1["type"], q1["status"], q1["code"], q1["version"])
		}
		if len(Objs(q1["payments"])) != 0 || F(q1, "legacyTotals.paid") != 0 {
			t.Errorf("a plain quotation carries no payments: %v", q1["payments"])
		}
		docsStockStays(t, s, pid, 50)
	})

	t.Run("discount and shipping move the VAT base", func(t *testing.T) {
		b := base(s.Line(pid, 2, 50))
		b["discount"], b["shipping"] = 5, 10
		r := Create(t, s.Token, "quotations", b)
		total, vat, net, _ := docsOracle([]docsLn{{2, 50, 0, 10}}, 5, 10, vatPct)
		docsTotals(t, "discounted quotation", r, total, vat, net)
		if S(r["code"]) != "QTN-002" {
			t.Errorf("numbering: got %v, want QTN-002", r["code"])
		}
	})

	t.Run("validation", func(t *testing.T) {
		cases := []struct {
			name   string
			mut    func(b M)
			fields []string
		}{
			{"bad type", func(b M) { b["type"] = "estimate" }, []string{"type"}},
			{"validity 0", func(b M) { b["validityDays"] = 0 }, []string{"validityDays"}},
			{"delivery negative", func(b M) { b["deliveryDays"] = -2 }, []string{"deliveryDays"}},
			{"no date", func(b M) { delete(b, "date") }, []string{"date"}},
			{"negative discount", func(b M) { b["discount"] = -1 }, []string{"discount"}},
			{"zero qty", func(b M) { b["items"] = []M{s.Line(pid, 0, 10)} }, []string{"items.0.qty"}},
			{"overpaid invoice", func(b M) {
				b["type"] = "invoice"
				b["payments"] = []M{{"date": s.Now(), "amount": 1000, "method": "cash"}}
			}, []string{"payments"}},
		}
		for _, tc := range cases {
			b := base(s.Line(pid, 1, 10))
			tc.mut(b)
			docsWantErr(t, Call(t, "POST", "/quotations", s.Token, b), 400, tc.name, tc.fields...)
		}
		docsWantErr(t, Call(t, "POST", "/quotations", s.Token, "{bad json"), 400, "malformed json")
		docsWantErr(t, Call(t, "POST", "/quotations", s.Token, M{"date": s.Now()}), 400, "no store", "storeId")
		docsWantErr(t, Call(t, "POST", "/quotations", s.Token, M{"storeId": other.ID, "date": s.Now()}), 403, "foreign store")
	})

	t.Run("quotation invoice: payments, balance, still no stock move", func(t *testing.T) {
		b := base(s.Line(pid, 3, 100))
		b["type"] = "invoice"
		b["payments"] = []M{{"date": s.Now(), "amount": 100.5, "method": "cash"}}
		qi = Create(t, s.Token, "quotations", b)
		total, vat, net, _ := docsOracle([]docsLn{{3, 100, 0, 10}}, 0, 0, vatPct)
		docsTotals(t, "quotation invoice", qi, total, vat, net)
		EqMoney(t, "paid", F(qi, "legacyTotals.paid"), 100.5)
		EqMoney(t, "balance", F(qi, "legacyTotals.balance"), net-100.5)
		if S(Get(qi, "legacyTotals.paymentStatus")) != "paid_partially" {
			t.Errorf("payment status: %v", Get(qi, "legacyTotals.paymentStatus"))
		}
		// the store setting update_product_stock_on_quotation_sales is off for new stores
		docsStockStays(t, s, pid, 50)
	})

	t.Run("status changes with If-Match, PATCH and PUT", func(t *testing.T) {
		id := S(q1["id"])
		r := Call(t, "PATCH", "/quotations/"+id, s.Token, M{"status": "accepted"}, "If-Match", "99")
		if r.Code != 409 || r.ErrCode() != "version_conflict" {
			t.Errorf("stale If-Match: %s", r)
		}
		r = Call(t, "PATCH", "/quotations/"+id, s.Token, M{"status": "accepted"}, "If-Match", "abc")
		if r.Code != 409 {
			t.Errorf("unreadable If-Match: %s", r)
		}
		up := Patch(t, s.Token, "quotations", id, M{"status": "accepted"})
		if up["status"] != "accepted" || F(up, "version") != 2 {
			t.Errorf("accepted: status %v version %v", up["status"], up["version"])
		}
		up = Patch(t, s.Token, "quotations", id, M{"status": "rejected", "remarks": "عرض مرفوض – too expensive"})
		if up["status"] != "rejected" || up["remarks"] != "عرض مرفوض – too expensive" || F(up, "version") != 3 {
			t.Errorf("rejected: %v %v %v", up["status"], up["remarks"], up["version"])
		}
		h := Objs(up["history"])
		if len(h) < 3 || h[len(h)-1]["action"] != "e2e" {
			t.Errorf("history keeps every change with the change reason: %v", up["history"])
		}
		docsWantErr(t, PatchResp(t, s.Token, "quotations", id, M{"validityDays": -1}), 400, "patch validity", "validityDays")
		docsWantErr(t, PatchResp(t, s.Token, "quotations", id, M{"storeId": other.ID}), 403, "move to a foreign store")
		r = docsPut(t, s.Token, "quotations", id, M{"status": "created", "items": []M{s.Line(pid, 4, 20)}})
		Must(t, r, 200, "PUT quotation")
		total, vat, net, _ := docsOracle([]docsLn{{4, 20, 0, 10}}, 0, 0, vatPct)
		docsTotals(t, "after PUT", r.Body, total, vat, net)
		if r.Body["status"] != "created" || F(r.Body, "version") != 4 {
			t.Errorf("PUT: status %v version %v", r.Body["status"], r.Body["version"])
		}
		docsWantErr(t, Call(t, "PUT", "/quotations/"+id, s.Token, M{"type": "bogus", "date": s.Now()}), 400, "PUT bad type", "type")
		Must(t, Call(t, "PATCH", "/quotations/000000000000000000000000", s.Token, M{"status": "x"}), 404, "patch unknown")
		docsStockStays(t, s, pid, 50)
	})

	t.Run("convert to a sale: link both ways, only the sale moves stock", func(t *testing.T) {
		qid := S(qi["id"])
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "quotationId": qid,
			"items": []M{s.Line(pid, 3, 100)}, "payments": []M{{"date": s.Now(), "amount": 345, "method": "cash"}}})
		if sale["quotationId"] != qid || sale["quotationCode"] != qi["code"] {
			t.Errorf("sale links the quotation: %v %v", sale["quotationId"], sale["quotationCode"])
		}
		if !strings.HasPrefix(S(sale["code"]), "S-INV-") {
			t.Errorf("sale numbering is its own: %v", sale["code"])
		}
		docsStockEventually(t, s, pid, 47)
		Eventually(t, "quotation lists the sale", func() bool {
			q := Read(t, s.Token, "quotations", qid)
			ids, _ := q["orderIds"].([]interface{})
			codes, _ := q["orderCodes"].([]interface{})
			return len(ids) == 1 && S(ids[0]) == S(sale["id"]) && len(codes) == 1 && S(codes[0]) == S(sale["code"])
		})
		docsWantErr(t, Call(t, "POST", "/sales", s.Token, M{"storeId": s.ID, "date": s.Now(), "customerId": cid,
			"quotationId": "000000000000000000000000", "items": []M{s.Line(pid, 1, 100)}}), 400, "unknown quotation", "quotationId")
	})

	t.Run("lists: search, ids, select, sort, paging, filters", func(t *testing.T) {
		all := List(t, s.Token, "quotations", "storeId="+s.ID+"&sort=code")
		if got := docsCodes(all); len(got) != 3 || got[0] != "QTN-001" || got[2] != "QTN-003" {
			t.Fatalf("sorted codes: %v", got)
		}
		desc := List(t, s.Token, "quotations", "storeId="+s.ID+"&sort=-code&limit=1&page=2")
		if got := docsCodes(desc); len(got) != 1 || got[0] != "QTN-002" {
			t.Errorf("page 2 of -code: %v", got)
		}
		r := Must(t, Call(t, "GET", "/quotations?storeId="+s.ID+"&limit=2", s.Token, nil), 200, "limit")
		if len(r.Data()) != 2 || F(r.Body, "total") != 3 {
			t.Errorf("limit 2 of 3: %d rows, total %v", len(r.Data()), r.Body["total"])
		}
		if got := List(t, s.Token, "quotations", "storeId="+s.ID+"&q=QTN-003"); len(got) != 1 || got[0]["id"] != qi["id"] {
			t.Errorf("search by code: %v", docsCodes(got))
		}
		if got := List(t, s.Token, "quotations", "storeId="+s.ID+"&ids="+S(q1["id"])); len(got) != 1 {
			t.Errorf("ids filter: %d", len(got))
		}
		sel := List(t, s.Token, "quotations", "storeId="+s.ID+"&select=code,type")
		if len(sel) != 3 || sel[0]["items"] != nil || sel[0]["code"] == nil {
			t.Errorf("select trims fields: %v", sel[0])
		}
		if got := List(t, s.Token, "quotations", "storeId="+s.ID+"&where.paymentStatus=paid_partially"); len(got) != 1 || got[0]["id"] != qi["id"] {
			t.Errorf("paymentStatus filter: %v", docsCodes(got))
		}
		if got := List(t, s.Token, "quotations", "storeId="+s.ID+"&where.customerId="+cid); len(got) != 3 {
			t.Errorf("customer filter: %d", len(got))
		}
		docsWantErr(t, Call(t, "GET", "/quotations?storeId="+s.ID+"&sort=nope", s.Token, nil), 400, "unknown sort", "sort")
		docsWantErr(t, Call(t, "GET", "/quotations?storeId="+s.ID+"&limit=0", s.Token, nil), 400, "limit 0", "limit")
		docsWantErr(t, Call(t, "GET", "/quotations?storeId="+s.ID+"&page=-1", s.Token, nil), 400, "page -1", "page")
		docsWantErr(t, Call(t, "GET", "/quotations", s.Token, nil), 400, "no storeId", "storeId")
		Must(t, Call(t, "GET", "/quotations?storeId="+s.ID, other.Token, nil), 403, "foreign store list")
		Must(t, Call(t, "GET", "/quotations/"+S(q1["id"]), other.Token, nil), 404, "foreign record")
		Must(t, Call(t, "GET", "/quotations/not-an-id", s.Token, nil), 404, "bad id")
		Must(t, Call(t, "GET", "/quotations?storeId="+s.ID, "", nil), 401, "no token")
	})

	t.Run("stats: count, money sums and the expired status", func(t *testing.T) {
		old := base(s.Line(pid, 1, 40))
		old["date"] = time.Now().In(s.Loc).AddDate(0, 0, -10).Format("2006-01-02T15:04")
		old["validityDays"] = 2
		Create(t, s.Token, "quotations", old)
		st := docsStats(t, s, "quotations", "sum=net,vat,one,paid")
		_, v1, n1, _ := docsOracle([]docsLn{{4, 20, 0, 10}}, 0, 0, vatPct)
		_, v2, n2, _ := docsOracle([]docsLn{{2, 50, 0, 10}}, 5, 10, vatPct)
		_, v3, n3, _ := docsOracle([]docsLn{{3, 100, 0, 10}}, 0, 0, vatPct)
		_, v4, n4, _ := docsOracle([]docsLn{{1, 40, 0, 10}}, 0, 0, vatPct)
		if F(st, "count") != 4 || F(st, "sums.one") != 4 {
			t.Errorf("count: %v", st)
		}
		EqMoney(t, "stats net", F(st, "sums.net"), n1+n2+n3+n4)
		EqMoney(t, "stats vat", F(st, "sums.vat"), v1+v2+v3+v4)
		EqMoney(t, "stats paid", F(st, "sums.paid"), 100.5)
		ex := docsStats(t, s, "quotations", "sum=one&f.qstatus=expired")
		if F(ex, "count") != 1 {
			t.Errorf("one quotation past its validity is expired: %v", ex)
		}
		g := docsStats(t, s, "quotations", "sum=one&groupBy=qstatus")
		if F(g, "groups.expired.count") != 1 || F(g, "groups.created.count") != 1 || F(g, "groups..count") != 2 {
			t.Errorf("groups by quotation status: %v", g["groups"])
		}
		docsWantErr(t, Call(t, "GET", "/quotations/stats?storeId="+s.ID+"&from=2026-13-01", s.Token, nil), 400, "bad from", "from")
		docsWantErr(t, Call(t, "GET", "/quotations/stats?storeId="+s.ID+"&from=2026-10-10&to=2026-10-01", s.Token, nil), 400, "from after to", "from")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "quotations", S(q1["id"]), true, "000000000000000000000000")
	})

	t.Run("delete, list without it, restore is unsupported", func(t *testing.T) {
		id := S(q1["id"])
		r := Call(t, "DELETE", "/quotations/"+id, s.Token, nil, "If-Match", "1")
		if r.Code != 409 || r.ErrCode() != "version_conflict" {
			t.Errorf("delete with stale version: %s", r)
		}
		d := Must(t, Call(t, "DELETE", "/quotations/"+id, s.Token, nil), 200, "delete quotation").Body
		if d["deleted"] != true {
			t.Errorf("deleted flag: %v", d["deleted"])
		}
		if docsHas(List(t, s.Token, "quotations", "storeId="+s.ID), id) {
			t.Error("deleted quotation still listed")
		}
		if !docsHas(List(t, s.Token, "quotations", "storeId="+s.ID+"&includeDeleted=1"), id) {
			t.Error("includeDeleted lists it")
		}
		Must(t, Call(t, "DELETE", "/quotations/"+id, s.Token, nil), 200, "delete twice")
		r = Call(t, "POST", "/quotations/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore: %s", r)
		}
		Must(t, Call(t, "POST", "/quotations/"+S(qi["id"])+"/restore", s.Token, nil), 200, "restore of a live record is a no-op")
		Must(t, Call(t, "DELETE", "/quotations/000000000000000000000000", s.Token, nil), 404, "delete unknown")
		Must(t, Call(t, "POST", "/quotations/000000000000000000000000/restore", s.Token, nil), 404, "restore unknown")
		// the next quotation keeps counting
		n := Create(t, s.Token, "quotations", base(s.Line(pid, 1, 1)))
		if S(n["code"]) != "QTN-005" {
			t.Errorf("numbering after a delete: %v", n["code"])
		}
	})

	t.Run("roles: viewer reads only, salesman cannot delete", func(t *testing.T) {
		_, viewer := s.User(t, "r_viewer")
		Must(t, Call(t, "GET", "/quotations?storeId="+s.ID, viewer, nil), 200, "viewer list")
		Must(t, Call(t, "POST", "/quotations", viewer, base(s.Line(pid, 1, 1))), 403, "viewer create")
		_, sm := s.User(t, "r_salesman")
		Must(t, Call(t, "DELETE", "/quotations/"+S(qi["id"]), sm, nil), 403, "salesman delete")
		Must(t, Call(t, "POST", "/quotations", sm, base(s.Line(pid, 1, 1))), 201, "salesman create")
	})

	t.Run("customer of another store", func(t *testing.T) {
		b := base(s.Line(pid, 1, 10))
		b["customerId"] = S(other.Customer(t, "")["id"])
		r := Call(t, "POST", "/quotations", s.Token, b)
		KnownBug(t, "NEW-foreign-customer", "a quotation accepts another store's customer id (saved as customer UNKNOWN) instead of 400 customerId", r.Code == 201)
		if r.Code != 201 {
			docsWantErr(t, r, 400, "foreign customer", "customerId")
		}
	})
}

// ---------- quotation returns ----------

func TestDocsQuotationReturns(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	pid := S(s.Product(t, 10, 20, 50)["id"])
	cid := S(s.Customer(t, "")["id"])
	vatPct := F(s.Rec, "vatPercent")
	_, qnet, _, _ := func() (float64, float64, float64, float64) {
		_, v, n, p := docsOracle([]docsLn{{3, 100, 0, 10}}, 0, 0, vatPct)
		return v, n, v, p
	}()
	q := Create(t, s.Token, "quotations", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "type": "invoice",
		"validityDays": 7, "deliveryDays": 3, "items": []M{s.Line(pid, 3, 100)},
		"payments": []M{{"date": s.Now(), "amount": qnet, "method": "cash"}}})
	qid := S(q["id"])
	ret := func(qty float64, pay float64) M {
		b := M{"storeId": s.ID, "date": s.Now(), "quotationId": qid, "customerId": cid, "items": []M{s.Line(pid, qty, 100)}}
		if pay > 0 {
			b["payments"] = []M{{"date": s.Now(), "amount": pay, "method": "cash"}}
		}
		return b
	}
	var r1 M

	t.Run("return one: totals oracle, refund, link to the quotation", func(t *testing.T) {
		_, v, n, _ := docsOracle([]docsLn{{1, 100, 0, 10}}, 0, 0, vatPct)
		r1 = Create(t, s.Token, "quotation-returns", ret(1, n))
		docsTotals(t, "quotation return", r1, 100, v, n)
		if r1["orderCode"] != q["code"] || r1["quotationId"] != qid {
			t.Errorf("links the quotation: %v %v", r1["orderCode"], r1["quotationId"])
		}
		if S(Get(r1, "legacyTotals.paymentStatus")) != "paid" {
			t.Errorf("refund paid: %v", r1["legacyTotals"])
		}
		docsStockStays(t, s, pid, 50)
	})

	t.Run("over-return and missing quotation are refused", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/quotation-returns", s.Token, ret(5, 0)), 400, "more than invoiced", "items.0.qty")
		// 1 already returned: 3 more would exceed the 3 invoiced
		docsWantErr(t, Call(t, "POST", "/quotation-returns", s.Token, ret(3, 0)), 400, "cumulative over-return", "items.0.qty")
		b := ret(1, 0)
		delete(b, "quotationId")
		docsWantErr(t, Call(t, "POST", "/quotation-returns", s.Token, b), 400, "no quotation", "quotationId")
		b = ret(1, 0)
		b["quotationId"] = "000000000000000000000000"
		docsWantErr(t, Call(t, "POST", "/quotation-returns", s.Token, b), 400, "unknown quotation", "quotationId")
		b = ret(1, 0)
		b["payments"] = []M{{"date": s.Now(), "amount": -5, "method": "cash"}}
		docsWantErr(t, Call(t, "POST", "/quotation-returns", s.Token, b), 400, "negative refund", "payments.0.amount")
	})

	t.Run("second return within the rest, numbering", func(t *testing.T) {
		r2 := Create(t, s.Token, "quotation-returns", ret(2, 0))
		if S(r2["code"]) == S(r1["code"]) || S(r2["code"]) == "" {
			t.Errorf("distinct return numbers: %v %v", r1["code"], r2["code"])
		}
		docsWantErr(t, Call(t, "POST", "/quotation-returns", s.Token, ret(1, 0)), 400, "all 3 returned", "items.0.qty")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "quotation-returns", S(r1["id"]), true, "000000000000000000000000")
	})

	t.Run("patch, put, delete unsupported, restore no-op, stats", func(t *testing.T) {
		id := S(r1["id"])
		up := Patch(t, s.Token, "quotation-returns", id, M{"remarks": "مرتجع"})
		if up["remarks"] != "مرتجع" || F(up, "version") != 2 {
			t.Errorf("patch: %v %v", up["remarks"], up["version"])
		}
		Must(t, docsPut(t, s.Token, "quotation-returns", id, M{"remarks": "put"}), 200, "PUT return")
		docsWantErr(t, PatchResp(t, s.Token, "quotation-returns", id, M{"items": []M{s.Line(pid, 9, 100)}}), 400, "patch over-return", "items.0.qty")
		r := Call(t, "DELETE", "/quotation-returns/"+id, s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("delete: %s", r)
		}
		Must(t, Call(t, "POST", "/quotation-returns/"+id+"/restore", s.Token, nil), 200, "restore live")
		Must(t, Call(t, "POST", "/quotation-returns/nope/restore", s.Token, nil), 404, "restore unknown")
		st := docsStats(t, s, "quotation-returns", "sum=net,vat,one")
		_, v, n, _ := docsOracle([]docsLn{{3, 100, 0, 10}}, 0, 0, vatPct)
		if F(st, "count") != 2 {
			t.Errorf("stats count: %v", st)
		}
		EqMoney(t, "returns net", F(st, "sums.net"), n)
		EqMoney(t, "returns vat", F(st, "sums.vat"), v)
		if got := List(t, s.Token, "quotation-returns", "storeId="+s.ID+"&where.customerId="+cid); len(got) != 2 {
			t.Errorf("list by customer: %d", len(got))
		}
		docsWantErr(t, Call(t, "GET", "/quotation-returns/stats?storeId="+s.ID+"&to=10-10-2026", s.Token, nil), 400, "bad to", "to")
	})
}

// ---------- delivery notes ----------

func TestDocsDeliveryNotes(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	pid := S(s.Product(t, 10, 20, 30)["id"])
	cid := S(s.Customer(t, "")["id"])
	body := func(est string, items ...M) M {
		return M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "estDelivery": est, "items": items}
	}
	var dn M

	t.Run("create at price 0, numbering, no stock effect", func(t *testing.T) {
		dn = Create(t, s.Token, "delivery-notes", body(s.Today(), s.Line(pid, 4, 0)))
		if !strings.HasSuffix(S(dn["code"]), "000001") || dn["estDelivery"] != s.Today() {
			t.Errorf("code/estDelivery: %v %v", dn["code"], dn["estDelivery"])
		}
		if F(dn, "legacyTotals.net") != 0 {
			t.Errorf("zero-price note totals: %v", dn["legacyTotals"])
		}
		dn2 := Create(t, s.Token, "delivery-notes", body(s.Today(), s.Line(pid, 1, 5)))
		if !strings.HasSuffix(S(dn2["code"]), "000002") {
			t.Errorf("second number: %v", dn2["code"])
		}
		// delivery notes do not move stock in the legacy system (the sale does)
		docsStockStays(t, s, pid, 30)
	})

	t.Run("validation", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/delivery-notes", s.Token, body("", s.Line(pid, 1, 0))), 400, "no estDelivery", "estDelivery")
		b := body(s.Today(), s.Line(pid, 1, 0))
		delete(b, "date")
		docsWantErr(t, Call(t, "POST", "/delivery-notes", s.Token, b), 400, "no date", "date")
		docsWantErr(t, Call(t, "POST", "/delivery-notes", s.Token, body(s.Today(), s.Line(pid, -1, 0))), 400, "negative qty", "items.0.qty")
	})

	t.Run("link to a sale, status, patch, put", func(t *testing.T) {
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "items": []M{s.Line(pid, 2, 20)},
			"payments": []M{{"date": s.Now(), "amount": 46, "method": "cash"}}})
		docsStockEventually(t, s, pid, 28)
		b := body(s.Today(), s.Line(pid, 2, 0))
		b["orderId"] = sale["id"]
		ln := Create(t, s.Token, "delivery-notes", b)
		if ln["orderCode"] != sale["code"] {
			t.Errorf("note links the sale: %v vs %v", ln["orderCode"], sale["code"])
		}
		docsStockStays(t, s, pid, 28)
		id := S(dn["id"])
		up := Patch(t, s.Token, "delivery-notes", id, M{"remarks": "deliver fast", "status": "delivered"})
		if up["remarks"] != "deliver fast" || F(up, "version") != 2 {
			t.Errorf("patch: %v %v", up["remarks"], up["version"])
		}
		if got := Read(t, s.Token, "delivery-notes", id); got["status"] != "delivered" {
			t.Errorf("status round-trips: %v", got["status"])
		}
		Must(t, docsPut(t, s.Token, "delivery-notes", id, M{"remarks": "put"}), 200, "PUT note")
		r := Call(t, "PATCH", "/delivery-notes/"+id, s.Token, M{"remarks": "x"}, "If-Match", "1")
		if r.Code != 409 {
			t.Errorf("stale version: %s", r)
		}
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "delivery-notes", S(dn["id"]), true, "000000000000000000000000")
	})

	t.Run("delete unsupported, restore no-op, late stats", func(t *testing.T) {
		id := S(dn["id"])
		r := Call(t, "DELETE", "/delivery-notes/"+id, s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("delete: %s", r)
		}
		Must(t, Call(t, "POST", "/delivery-notes/"+id+"/restore", s.Token, nil), 200, "restore live")
		Must(t, Call(t, "POST", "/delivery-notes/000000000000000000000000/restore", s.Token, nil), 404, "restore unknown")
		yesterday := time.Now().In(s.Loc).AddDate(0, 0, -3).Format("2006-01-02")
		late := body(yesterday, s.Line(pid, 1, 0))
		late["status"] = "pending"
		Create(t, s.Token, "delivery-notes", late)
		st := docsStats(t, s, "delivery-notes", "sum=one&groupBy=late")
		if F(st, "count") != 4 || F(st, "groups.y.count") != 1 {
			t.Errorf("one pending note is late: %v", st)
		}
		if got := docsStats(t, s, "delivery-notes", "sum=one&f.late=y"); F(got, "count") != 1 {
			t.Errorf("late filter: %v", got)
		}
		docsWantErr(t, Call(t, "GET", "/delivery-notes/stats?storeId="+s.ID+"&sum="+strings.Repeat("one,", 21)+"one", s.Token, nil), 400, "too many sums", "sum")
	})
}

// ---------- non-VAT sales and returns ----------

func TestDocsNonVAT(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	pid := S(s.Product(t, 10, 20, 40)["id"])
	cid := S(s.Customer(t, "")["id"])
	var ns M

	t.Run("sale: no VAT whatever the lines say, own numbering, stock out", func(t *testing.T) {
		l := s.Line(pid, 4, 12.5) // vatPercent 15 on the line
		ns = Create(t, s.Token, "nonvat-sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "items": []M{l},
			"payments": []M{{"date": s.Now(), "amount": 20, "method": "cash"}}})
		total, vat, net, profit := docsOracle([]docsLn{{4, 12.5, 0, 10}}, 0, 0, 0)
		docsTotals(t, "non-VAT sale", ns, total, vat, net)
		if F(ns, "vatPercent") != 0 || F(ns, "items.0.vatPercent") != 0 || vat != 0 {
			t.Errorf("VAT forced to 0: %v %v", ns["vatPercent"], Get(ns, "items.0.vatPercent"))
		}
		EqMoney(t, "balance", F(ns, "legacyTotals.balance"), net-20)
		KnownBug(t, "NEW-nonvat-profit", "non-VAT sale profit is never computed on save (always 0)",
			Cents(F(ns, "legacyTotals.profit")) != Cents(profit))
		if S(ns["code"]) != "NVS-001" {
			t.Errorf("non-VAT numbering: %v", ns["code"])
		}
		docsStockEventually(t, s, pid, 36)
		// a VAT sale does not take a non-VAT number
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "items": []M{s.Line(pid, 1, 20)},
			"payments": []M{{"date": s.Now(), "amount": 23, "method": "cash"}}})
		if strings.HasPrefix(S(sale["code"]), "NVS") {
			t.Errorf("sale code %v", sale["code"])
		}
		ns2 := Create(t, s.Token, "nonvat-sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "items": []M{s.Line(pid, 1, 9.99)}})
		if S(ns2["code"]) != "NVS-002" || F(ns2, "legacyTotals.vat") != 0 {
			t.Errorf("second non-VAT sale: %v %v", ns2["code"], ns2["legacyTotals"])
		}
		docsStockEventually(t, s, pid, 34)
	})

	t.Run("sale validation", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/nonvat-sales", s.Token, M{"storeId": s.ID, "customerId": cid, "items": []M{s.Line(pid, 1, 1)}}), 400, "no date", "date")
		docsWantErr(t, Call(t, "POST", "/nonvat-sales", s.Token, M{"storeId": s.ID, "date": s.Now(), "customerId": cid,
			"items": []M{s.Line(pid, 1, 10)}, "payments": []M{{"date": s.Now(), "amount": 11, "method": "cash"}}}), 400, "overpaid", "payments")
		docsWantErr(t, Call(t, "POST", "/nonvat-sales", s.Token, M{"storeId": s.ID, "date": s.Now(), "customerId": cid,
			"discount": -3, "items": []M{s.Line(pid, 1, 10)}}), 400, "negative discount", "discount")
	})

	var nr M
	t.Run("return: totals, stock back, numbering", func(t *testing.T) {
		nr = Create(t, s.Token, "nonvat-returns", M{"storeId": s.ID, "date": s.Now(), "orderId": ns["id"], "customerId": cid,
			"items": []M{s.Line(pid, 1, 12.5)}})
		docsTotals(t, "non-VAT return", nr, 12.5, 0, 12.5)
		if nr["orderCode"] != ns["code"] || !strings.HasPrefix(S(nr["code"]), "NVS-R-") {
			t.Errorf("return links the sale: %v %v", nr["orderCode"], nr["code"])
		}
		docsStockEventually(t, s, pid, 35)
		b := M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "items": []M{s.Line(pid, 1, 12.5)}}
		docsWantErr(t, Call(t, "POST", "/nonvat-returns", s.Token, b), 400, "no sale", "orderId")
	})

	t.Run("over-return is refused", func(t *testing.T) {
		// 4 sold, 1 returned: 5 more is over the sale
		r := Call(t, "POST", "/nonvat-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": ns["id"], "customerId": cid,
			"items": []M{s.Line(pid, 5, 12.5)}})
		KnownBug(t, "NEW-nonvat-overreturn", "non-VAT returns accept more than was sold (stock goes up by the excess)", r.Code == 201)
		if r.Code == 201 {
			// undo it so the rest of the test runs on the intended state
			Must(t, Call(t, "DELETE", "/nonvat-returns/"+r.ID(), s.Token, nil), 200, "delete over-return")
		} else {
			docsWantErr(t, r, 400, "over-return", "items.0.qty")
		}
		docsStockEventually(t, s, pid, 35)
	})

	t.Run("patch and put the sale rewrite the full document", func(t *testing.T) {
		id := S(ns["id"])
		up := Patch(t, s.Token, "nonvat-sales", id, M{"items": []M{s.Line(pid, 6, 13)}, "remarks": "بدون ضريبة"})
		total, _, net, _ := docsOracle([]docsLn{{6, 13, 0, 10}}, 0, 0, 0)
		docsTotals(t, "patched non-VAT sale", up, total, 0, net)
		if up["remarks"] != "بدون ضريبة" || len(Objs(up["payments"])) != 1 {
			t.Errorf("patch keeps the rest of the document: %v %v", up["remarks"], up["payments"])
		}
		docsStockEventually(t, s, pid, 33)
		r := docsPut(t, s.Token, "nonvat-sales", id, M{"remarks": "put"})
		Must(t, r, 200, "PUT non-VAT sale")
		docsTotals(t, "after PUT", r.Body, total, 0, net)
		docsWantErr(t, PatchResp(t, s.Token, "nonvat-sales", id, M{"discount": -1}), 400, "patch negative discount", "discount")
		up = Patch(t, s.Token, "nonvat-returns", S(nr["id"]), M{"remarks": "r"})
		if up["remarks"] != "r" {
			t.Errorf("patch return: %v", up["remarks"])
		}
		Must(t, docsPut(t, s.Token, "nonvat-returns", S(nr["id"]), M{"remarks": "r2"}), 200, "PUT return")
	})

	t.Run("stats, lists", func(t *testing.T) {
		st := docsStats(t, s, "nonvat-sales", "sum=net,vat,paid,balance,one")
		_, _, n1, _ := docsOracle([]docsLn{{6, 13, 0, 10}}, 0, 0, 0)
		if F(st, "count") != 2 || F(st, "sums.vat") != 0 {
			t.Errorf("non-VAT stats: %v", st)
		}
		EqMoney(t, "stats net", F(st, "sums.net"), n1+9.99)
		EqMoney(t, "stats paid", F(st, "sums.paid"), 20)
		EqMoney(t, "stats balance", F(st, "sums.balance"), n1+9.99-20)
		rs := docsStats(t, s, "nonvat-returns", "sum=net,one")
		EqMoney(t, "returns net", F(rs, "sums.net"), 12.5)
		if got := List(t, s.Token, "nonvat-sales", "storeId="+s.ID+"&sort=-netTotal"); len(got) != 2 || got[0]["id"] != ns["id"] {
			t.Errorf("sort by net desc: %v", docsCodes(got))
		}
		docsWantErr(t, Call(t, "GET", "/nonvat-sales?storeId="+s.ID+"&where.nope=1", s.Token, nil), 400, "unknown filter")
		Must(t, Call(t, "GET", "/nonvat-returns/stats?storeId="+s.ID+"&from=x", s.Token, nil), 400, "bad from")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "nonvat-sales", S(ns["id"]), true, "000000000000000000000000")
		docsRoutes(t, s, "nonvat-returns", S(nr["id"]), true, "000000000000000000000000")
	})

	t.Run("delete puts stock back, restore unsupported", func(t *testing.T) {
		d := Must(t, Call(t, "DELETE", "/nonvat-returns/"+S(nr["id"]), s.Token, nil), 200, "delete return").Body
		if d["deleted"] != true {
			t.Errorf("deleted: %v", d["deleted"])
		}
		docsStockEventually(t, s, pid, 32)
		r := Call(t, "POST", "/nonvat-returns/"+S(nr["id"])+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore return: %s", r)
		}
		Must(t, Call(t, "DELETE", "/nonvat-sales/"+S(ns["id"]), s.Token, nil), 200, "delete sale")
		docsStockEventually(t, s, pid, 38)
		r = Call(t, "POST", "/nonvat-sales/"+S(ns["id"])+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore sale: %s", r)
		}
		Must(t, Call(t, "DELETE", "/nonvat-sales/000000000000000000000000", s.Token, nil), 404, "delete unknown")
		if st := docsStats(t, s, "nonvat-sales", "sum=one"); F(st, "count") != 1 {
			t.Errorf("deleted sale leaves the tiles: %v", st)
		}
	})
}

// ---------- proformas (new collection) ----------

func TestDocsProformas(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	pid := S(s.Product(t, 10, 20, 15)["id"])
	cid := S(s.Customer(t, "")["id"])
	body := func(items ...M) M {
		return M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "validityDays": 7, "deliveryDays": 3, "items": items}
	}
	var p1 M

	t.Run("create: server number, client number kept once, no stock", func(t *testing.T) {
		b := body(s.Line(pid, 2, 50))
		b["posType"] = "restaurant"
		p1 = Create(t, s.Token, "proformas", b)
		if !strings.HasPrefix(S(p1["id"]), "pro_") || !strings.HasSuffix(S(p1["code"]), "PI-0001") || F(p1, "version") != 1 {
			t.Errorf("id/code/version: %v %v %v", p1["id"], p1["code"], p1["version"])
		}
		code := "PF-" + Digits(5)
		b = body(s.Line(pid, 1, 1))
		b["code"] = code
		if got := Create(t, s.Token, "proformas", b); got["code"] != code {
			t.Errorf("client code kept: %v", got["code"])
		}
		if got := Create(t, s.Token, "proformas", b); got["code"] == code || !strings.HasSuffix(S(got["code"]), "PI-0002") {
			t.Errorf("a used code gets the next number: %v", got["code"])
		}
		b = body(s.Line(pid, 1, 1))
		b["id"] = p1["id"]
		r := Call(t, "POST", "/proformas", s.Token, b)
		if r.Code != 409 || r.ErrCode() != "id_conflict" {
			t.Errorf("duplicate id: %s", r)
		}
		docsStockStays(t, s, pid, 15)
	})

	t.Run("validation", func(t *testing.T) {
		r := Call(t, "POST", "/proformas", s.Token, M{"storeId": s.ID, "date": "", "validityDays": 0, "deliveryDays": -1,
			"items": []M{{"qty": 0, "unitPrice": -1}}})
		docsWantErr(t, r, 400, "every field", "date", "validityDays", "deliveryDays", "items.0.qty", "items.0.unitPrice")
		docsWantErr(t, Call(t, "POST", "/proformas", s.Token, body()), 400, "no lines", "items")
		Must(t, Call(t, "POST", "/proformas", other.Token, body(s.Line(pid, 1, 1))), 403, "foreign store")
	})

	t.Run("patch, put, versions, history", func(t *testing.T) {
		id := S(p1["id"])
		up := Patch(t, s.Token, "proformas", id, M{"remarks": "فاتورة مبدئية", "validityDays": 14})
		if up["remarks"] != "فاتورة مبدئية" || F(up, "validityDays") != 14 || F(up, "version") != 2 || up["code"] != p1["code"] {
			t.Errorf("patch: %v", up)
		}
		if h := Objs(up["history"]); len(h) != 2 || len(Objs(h[1]["changes"])) == 0 {
			t.Errorf("history with the field diff: %v", up["history"])
		}
		r := Call(t, "PATCH", "/proformas/"+id, s.Token, M{"remarks": "x"}, "If-Match", "1")
		if r.Code != 409 || r.ErrCode() != "version_conflict" {
			t.Errorf("stale version: %s", r)
		}
		docsWantErr(t, PatchResp(t, s.Token, "proformas", id, M{"items": []M{}}), 400, "patch no lines", "items")
		r = docsPut(t, s.Token, "proformas", id, M{"items": []M{s.Line(pid, 3, 40)}, "remarks": nil})
		Must(t, r, 200, "PUT proforma")
		if F(r.Body, "items.0.qty") != 3 || r.Body["remarks"] != nil || F(r.Body, "version") != 3 || r.Body["createdAt"] != p1["createdAt"] {
			t.Errorf("PUT replaces the record: %v", r.Body)
		}
		Must(t, Call(t, "PATCH", "/proformas/pro_nope", s.Token, M{"remarks": "x"}), 404, "patch unknown")
		Must(t, Call(t, "PUT", "/proformas/pro_nope", s.Token, body(s.Line(pid, 1, 1))), 404, "put unknown")
	})

	t.Run("lists: filters, sort, paging, stats", func(t *testing.T) {
		if got := List(t, s.Token, "proformas", "storeId="+s.ID+"&where.posType=restaurant"); len(got) != 1 || got[0]["id"] != p1["id"] {
			t.Errorf("posType filter: %v", docsCodes(got))
		}
		if got := List(t, s.Token, "proformas", "storeId="+s.ID+"&where.customerId="+cid); len(got) != 3 {
			t.Errorf("customer filter: %d", len(got))
		}
		docsWantErr(t, Call(t, "GET", "/proformas?storeId="+s.ID+"&where.status=x", s.Token, nil), 400, "unknown filter", "where.status")
		docsWantErr(t, Call(t, "GET", "/proformas?storeId="+s.ID+"&sort=code", s.Token, nil), 400, "unknown sort", "sort")
		docsWantErr(t, Call(t, "GET", "/proformas?storeId="+s.ID+"&min.qty=1", s.Token, nil), 400, "ranges", "where")
		r := Must(t, Call(t, "GET", "/proformas?storeId="+s.ID+"&sort=-date&limit=2&page=2", s.Token, nil), 200, "page 2")
		if len(r.Data()) != 1 || F(r.Body, "total") != 3 {
			t.Errorf("page 2 of 3 by 2: %d rows, total %v", len(r.Data()), r.Body["total"])
		}
		st := docsStats(t, s, "proformas", "sum=one")
		if F(st, "count") != 3 {
			t.Errorf("stats: %v", st)
		}
		Must(t, Call(t, "GET", "/proformas/stats?storeId="+other.ID, s.Token, nil), 403, "foreign stats")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "proformas", S(p1["id"]), true, "pro_nope")
	})

	t.Run("delete, restore, hard delete", func(t *testing.T) {
		id := S(p1["id"])
		d := Must(t, Call(t, "DELETE", "/proformas/"+id, s.Token, nil), 200, "delete").Body
		if d["deleted"] != true || F(d, "version") != 4 {
			t.Errorf("soft delete: %v %v", d["deleted"], d["version"])
		}
		if docsHas(List(t, s.Token, "proformas", "storeId="+s.ID), id) {
			t.Error("deleted proforma listed")
		}
		rs := Must(t, Call(t, "POST", "/proformas/"+id+"/restore", s.Token, nil), 200, "restore").Body
		if rs["deleted"] != false || F(rs, "version") != 5 {
			t.Errorf("restore: %v %v", rs["deleted"], rs["version"])
		}
		Must(t, Call(t, "POST", "/proformas/pro_nope/restore", s.Token, nil), 404, "restore unknown")
		Must(t, Call(t, "DELETE", "/proformas/"+id+"?hard=1", s.Token, nil), 204, "hard delete")
		Must(t, Call(t, "GET", "/proformas/"+id, s.Token, nil), 404, "gone")
		Must(t, Call(t, "DELETE", "/proformas/"+id, s.Token, nil), 404, "delete gone")
		_, viewer := s.User(t, "r_viewer")
		Must(t, Call(t, "POST", "/proformas", viewer, body(s.Line(pid, 1, 1))), 403, "viewer create")
	})
}

// ---------- drafts ----------

func TestDocsDrafts(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	pid := S(s.Product(t, 10, 20, 100)["id"])
	cid := S(s.Customer(t, "")["id"])
	vid := S(s.Vendor(t)["id"])
	wh := Create(t, s.Token, "warehouses", M{"storeId": s.ID, "nameEn": "WH " + Uniq(), "code": "W" + Digits(6)})
	dq := func(path, q string) string { return "/drafts/" + path + "?storeId=" + s.ID + q }

	// real source documents some finalized drafts refer to
	sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "items": []M{s.Line(pid, 5, 20)},
		"payments": []M{{"date": s.Now(), "amount": 115, "method": "cash"}}})
	pur := Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "vendorInvoiceNo": "V" + Digits(6),
		"items": []M{s.Line(pid, 5, 10)}})
	qi := Create(t, s.Token, "quotations", M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "type": "invoice",
		"validityDays": 7, "deliveryDays": 3, "items": []M{s.Line(pid, 2, 20)}})
	docsStockEventually(t, s, pid, 100)

	payloads := map[string]M{
		"sales": {"date": s.Now(), "customerId": cid, "customerName": "عميل", "items": []M{s.Line(pid, 2, 20)},
			"payments": []M{{"date": s.Now(), "amount": 46, "method": "cash"}}},
		"pos":        {"date": s.Now(), "customerId": cid, "items": []M{s.Line(pid, 1, 20)}},
		"quotations": {"date": s.Now(), "customerId": cid, "validityDays": 5, "deliveryDays": 2, "items": []M{s.Line(pid, 2, 20)}},
		"purchases": {"date": s.Now(), "vendorId": vid, "vendorName": "مورد", "vendorInvoiceNo": "V" + Digits(6),
			"items": []M{s.Line(pid, 3, 9)}},
		"sales-returns":    {"date": s.Now(), "orderId": sale["id"], "customerId": cid, "items": []M{s.Line(pid, 1, 20)}},
		"purchase-returns": {"date": s.Now(), "purchaseId": pur["id"], "vendorId": vid, "items": []M{s.Line(pid, 1, 10)}},
		"delivery-notes":   {"date": s.Now(), "customerId": cid, "estDelivery": s.Today(), "items": []M{s.Line(pid, 1, 0)}},
		"purchase-orders": {"date": s.Now(), "vendorId": vid, "expectedDate": s.Today(), "status": "draft",
			"items": []M{s.Line(pid, 3, 9)}},
		"quotation-returns": {"date": s.Now(), "quotationId": qi["id"], "customerId": cid, "items": []M{s.Line(pid, 1, 20)}},
		"deposits":          {"date": s.Now(), "customerId": cid, "amount": 75, "method": "cash", "notes": "adv"},
		"withdrawals":       {"date": s.Now(), "customerId": cid, "amount": 5, "method": "cash", "type": "refund", "notes": "r"},
		"stock-transfers": {"date": s.Now(), "fromWarehouseId": s.MS, "toWarehouseId": wh["id"], "status": "completed",
			"items": []M{{"productId": pid, "qty": 1, "unitPrice": 9}}},
	}
	// stock after every finalize: sales -2, pos -1, purchases +3, sales return +1,
	// purchase return -1 (stock transfers move between warehouses, the rest no stock)
	stockDelta := map[string]float64{"sales": -2, "pos": -1, "purchases": 3, "sales-returns": 1, "purchase-returns": -1}
	ids := map[string]string{}

	t.Run("create, get, list, put every type: nothing real is touched", func(t *testing.T) {
		for typ, pl := range payloads {
			d := Create(t, s.Token, "drafts/"+typ, M{"storeId": s.ID, "payload": pl, "deviceId": "till-1"})
			if d["docType"] != typ || d["legacyDraft"] != false || d["deviceId"] != "till-1" || S(d["title"]) == "" {
				t.Errorf("%s draft: %v", typ, d)
			}
			ids[typ] = S(d["id"])
			g := Must(t, Call(t, "GET", dq(typ+"/"+ids[typ], ""), s.Token, nil), 200, "get draft").Body
			if S(Get(g, "payload.date")) != S(pl["date"]) || (pl["items"] != nil && Get(g, "payload.items.0") == nil) || g["storeId"] != s.ID {
				t.Errorf("%s draft payload round-trip: %v", typ, g["payload"])
			}
			pl2 := M{}
			for k, v := range pl {
				pl2[k] = v
			}
			pl2["remarks"] = "edited " + typ
			up := Must(t, Call(t, "PUT", "/drafts/"+typ+"/"+ids[typ], s.Token, M{"storeId": s.ID, "payload": pl2, "title": "T " + typ}), 200, "put draft").Body
			if up["title"] != "T "+typ || Get(up, "payload.remarks") != "edited "+typ {
				t.Errorf("%s put: %v", typ, up)
			}
			r := Must(t, Call(t, "GET", dq(typ, ""), s.Token, nil), 200, "list drafts")
			if len(r.Data()) != 1 || F(r.Body, "total") != 1 || r.Data()[0]["id"] != ids[typ] {
				t.Errorf("%s list: %v", typ, r.Body)
			}
		}
		// no numbers, no stock, no documents
		docsStockStays(t, s, pid, 100)
		if got := List(t, s.Token, "quotations", "storeId="+s.ID); len(got) != 1 {
			t.Errorf("a draft is not a quotation: %d", len(got))
		}
		if got := List(t, s.Token, "sales", "storeId="+s.ID); len(got) != 1 {
			t.Errorf("a draft is not a sale: %d", len(got))
		}
	})

	t.Run("aliases, unknown type, missing store, bad payload, foreign store", func(t *testing.T) {
		if got := Must(t, Call(t, "GET", dq("quotation/"+ids["quotations"], ""), s.Token, nil), 200, "alias").Body; got["docType"] != "quotations" {
			t.Errorf("legacy alias: %v", got["docType"])
		}
		Must(t, Call(t, "GET", dq("pos_cart", ""), s.Token, nil), 200, "pos_cart alias")
		Must(t, Call(t, "GET", dq("invoices", ""), s.Token, nil), 404, "unknown type list")
		Must(t, Call(t, "POST", "/drafts/invoices", s.Token, M{"storeId": s.ID, "payload": M{}}), 404, "unknown type create")
		docsWantErr(t, Call(t, "GET", "/drafts/sales", s.Token, nil), 400, "list without store", "storeId")
		docsWantErr(t, Call(t, "POST", "/drafts/sales", s.Token, M{"payload": M{"x": 1}}), 400, "create without store", "storeId")
		docsWantErr(t, Call(t, "POST", "/drafts/sales", s.Token, M{"storeId": s.ID}), 400, "no payload", "payload")
		docsWantErr(t, Call(t, "POST", "/drafts/sales", s.Token, M{"storeId": s.ID, "payload": "text"}), 400, "payload not an object", "payload")
		docsWantErr(t, Call(t, "PUT", "/drafts/sales/"+ids["sales"], s.Token, M{"storeId": s.ID}), 400, "put without payload", "payload")
		Must(t, Call(t, "POST", "/drafts/sales", s.Token, "{nope"), 400, "malformed json")
		Must(t, Call(t, "GET", dq("sales/000000000000000000000000", ""), s.Token, nil), 404, "unknown draft")
		Must(t, Call(t, "GET", dq("sales/xyz", ""), s.Token, nil), 404, "bad draft id")
		Must(t, Call(t, "PUT", "/drafts/sales/000000000000000000000000", s.Token, M{"storeId": s.ID, "payload": M{}}), 404, "put unknown")
		// another type's draft id is not found under this type
		Must(t, Call(t, "GET", dq("purchases/"+ids["sales"], ""), s.Token, nil), 404, "wrong type")
		Must(t, Call(t, "GET", "/drafts/sales?storeId="+s.ID, other.Token, nil), 403, "foreign list")
		Must(t, Call(t, "GET", "/drafts/sales/"+ids["sales"]+"?storeId="+s.ID, other.Token, nil), 403, "foreign get")
		Must(t, Call(t, "DELETE", "/drafts/sales/"+ids["sales"]+"?storeId="+s.ID, other.Token, nil), 403, "foreign delete")
		Must(t, Call(t, "POST", "/drafts/sales/"+ids["sales"]+"/finalize?storeId="+s.ID, other.Token, nil), 403, "foreign finalize")
		Must(t, Call(t, "GET", "/drafts/sales?storeId="+s.ID, "", nil), 401, "no token")
		// the other store has its own (empty) drafts
		if r := Must(t, Call(t, "GET", "/drafts/sales?storeId="+other.ID, other.Token, nil), 200, "own list"); len(r.Data()) != 0 {
			t.Errorf("drafts leak across stores: %v", r.Body)
		}
	})

	t.Run("roles", func(t *testing.T) {
		_, viewer := s.User(t, "r_viewer")
		Must(t, Call(t, "GET", dq("sales", ""), viewer, nil), 200, "viewer list")
		Must(t, Call(t, "POST", "/drafts/sales", viewer, M{"storeId": s.ID, "payload": M{"items": []M{}}}), 403, "viewer create")
		Must(t, Call(t, "PUT", "/drafts/sales/"+ids["sales"], viewer, M{"storeId": s.ID, "payload": M{}}), 403, "viewer put")
		Must(t, Call(t, "POST", dq("sales/"+ids["sales"]+"/finalize", ""), viewer, nil), 403, "viewer finalize")
		tmp := Create(t, s.Token, "drafts/sales", M{"storeId": s.ID, "payload": M{"items": []M{}}})
		r := Call(t, "DELETE", dq("sales/"+S(tmp["id"]), ""), viewer, nil)
		KnownBug(t, "NEW-draft-delete-perm", "DELETE /drafts/{type}/{id} has no permission check: a read-only viewer deletes drafts", r.Code == 204)
		if r.Code != 204 {
			Must(t, r, 403, "viewer delete")
			Must(t, Call(t, "DELETE", dq("sales/"+S(tmp["id"]), ""), s.Token, nil), 204, "owner delete")
		}
	})

	t.Run("finalize an invalid draft: 400, draft kept, nothing created", func(t *testing.T) {
		bad := Create(t, s.Token, "drafts/quotations", M{"storeId": s.ID, "payload": M{"customerId": cid, "validityDays": 0,
			"items": []M{s.Line(pid, 1, 1)}}})
		r := Call(t, "POST", dq("quotations/"+S(bad["id"])+"/finalize", ""), s.Token, nil)
		docsWantErr(t, r, 400, "invalid draft", "date", "validityDays")
		Must(t, Call(t, "GET", dq("quotations/"+S(bad["id"]), ""), s.Token, nil), 200, "draft kept")
		if got := List(t, s.Token, "quotations", "storeId="+s.ID); len(got) != 1 {
			t.Errorf("nothing created: %d quotations", len(got))
		}
		Must(t, Call(t, "DELETE", dq("quotations/"+S(bad["id"]), ""), s.Token, nil), 204, "delete draft")
		Must(t, Call(t, "GET", dq("quotations/"+S(bad["id"]), ""), s.Token, nil), 404, "deleted draft")
		Must(t, Call(t, "DELETE", dq("quotations/"+S(bad["id"]), ""), s.Token, nil), 404, "delete twice")
		Must(t, Call(t, "POST", dq("quotations/"+S(bad["id"])+"/finalize", ""), s.Token, nil), 404, "finalize deleted")
		docsWantErr(t, Call(t, "POST", "/drafts/quotations/"+ids["quotations"]+"/finalize", s.Token, nil), 400, "finalize without store", "storeId")
	})

	t.Run("finalize every type: the real document with the next number, draft removed", func(t *testing.T) {
		want := 100.0
		for typ := range payloads {
			r := Call(t, "POST", dq(typ+"/"+ids[typ]+"/finalize", ""), s.Token, nil)
			if r.Code != 201 {
				t.Errorf("finalize %s: %s", typ, r)
				continue
			}
			if r.ID() == "" || r.ID() == ids[typ] {
				t.Errorf("%s: finalize returns the new record: %v", typ, r.Body["id"])
			}
			Must(t, Call(t, "GET", dq(typ+"/"+ids[typ], ""), s.Token, nil), 404, "draft removed after finalize "+typ)
			if rows := Must(t, Call(t, "GET", dq(typ, ""), s.Token, nil), 200, "list").Data(); len(rows) != 0 {
				t.Errorf("%s drafts left: %d", typ, len(rows))
			}
			want += stockDelta[typ]
			switch typ {
			case "quotations":
				if S(r.Body["code"]) != "QTN-002" {
					t.Errorf("finalized quotation takes the next number: %v", r.Body["code"])
				}
			case "sales":
				if r.Body["customerId"] != cid || F(r.Body, "legacyTotals.paid") != 46 {
					t.Errorf("finalized sale: %v %v", r.Body["customerId"], r.Body["legacyTotals"])
				}
			}
			Must(t, Call(t, "POST", dq(typ+"/"+ids[typ]+"/finalize", ""), s.Token, nil), 404, "finalize twice "+typ)
		}
		docsStockEventually(t, s, pid, want-1) // stock transfer moved 1 out of the main store
		if got := List(t, s.Token, "sales", "storeId="+s.ID); len(got) != 3 {
			t.Errorf("sales after finalize (1 real + sales + pos): %d", len(got))
		}
	})
}

// ---------- POS records and numbers ----------

func TestDocsPosRecords(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	var rec M

	t.Run("CRUD with history and versions", func(t *testing.T) {
		rec = Create(t, s.Token, "pos-records", M{"storeId": s.ID, "terminal": "restaurant", "kind": "tables", "key": "T1",
			"status": "open", "data": M{"covers": 2, "note": "طاولة"}})
		if !strings.HasPrefix(S(rec["id"]), "pos_") || F(rec, "version") != 1 || Get(rec, "data.note") != "طاولة" {
			t.Errorf("created: %v", rec)
		}
		id := S(rec["id"])
		up := Patch(t, s.Token, "pos-records", id, M{"data": M{"covers": 4}, "status": "billed"})
		if F(up, "data.covers") != 4 || up["status"] != "billed" || F(up, "version") != 2 {
			t.Errorf("patched: %v", up)
		}
		if h := Objs(up["history"]); len(h) != 2 || len(Objs(h[1]["changes"])) != 0 {
			t.Errorf("bare history (who and when only): %v", up["history"])
		}
		r := docsPut(t, s.Token, "pos-records", id, M{"data": []interface{}{"a", "b"}, "status": nil})
		Must(t, r, 200, "PUT pos record")
		if F(r.Body, "version") != 3 || r.Body["status"] != nil {
			t.Errorf("PUT: %v", r.Body)
		}
		Create(t, s.Token, "pos-records", M{"storeId": s.ID, "terminal": "restaurant", "kind": "orders", "status": "open", "data": M{}})
		Create(t, s.Token, "pos-records", M{"storeId": s.ID, "terminal": "salon", "kind": "queue", "data": M{}})
	})

	t.Run("validation", func(t *testing.T) {
		r := Call(t, "POST", "/pos-records", s.Token, M{"storeId": s.ID, "terminal": "nope", "kind": "a b", "key": 5, "status": "x y", "data": "str"})
		docsWantErr(t, r, 400, "every field", "terminal", "kind", "key", "status", "data")
		docsWantErr(t, Call(t, "POST", "/pos-records", s.Token, M{"storeId": s.ID, "terminal": "restaurant", "kind": strings.Repeat("k", 41)}), 400, "kind too long", "kind")
		docsWantErr(t, Call(t, "POST", "/pos-records", s.Token, M{"storeId": s.ID, "terminal": "restaurant", "kind": "t", "key": strings.Repeat("ك", 121)}), 400, "key too long", "key")
		Must(t, Call(t, "POST", "/pos-records", s.Token, M{"storeId": s.ID, "terminal": "restaurant", "kind": "t", "key": strings.Repeat("ك", 120)}), 201, "key at the limit")
		big := M{"blob": strings.Repeat("x", 513*1024)}
		docsWantErr(t, Call(t, "POST", "/pos-records", s.Token, M{"storeId": s.ID, "terminal": "restaurant", "kind": "t", "data": big}), 400, "data too large", "data")
		id := S(rec["id"])
		docsWantErr(t, PatchResp(t, s.Token, "pos-records", id, M{"terminal": "salon"}), 400, "terminal fixed", "terminal")
		docsWantErr(t, PatchResp(t, s.Token, "pos-records", id, M{"kind": "orders"}), 400, "kind fixed", "kind")
		r = Call(t, "PATCH", "/pos-records/"+id, s.Token, M{"status": "x"}, "If-Match", "1")
		if r.Code != 409 {
			t.Errorf("stale version: %s", r)
		}
		Must(t, Call(t, "POST", "/pos-records", other.Token, M{"storeId": s.ID, "terminal": "restaurant", "kind": "t"}), 403, "foreign store")
		Must(t, Call(t, "GET", "/pos-records/"+id, other.Token, nil), 404, "foreign record")
	})

	t.Run("list filters", func(t *testing.T) {
		if got := List(t, s.Token, "pos-records", "storeId="+s.ID+"&where.terminal=restaurant&where.kind=tables"); len(got) != 1 || got[0]["id"] != rec["id"] {
			t.Errorf("terminal+kind: %d", len(got))
		}
		if got := List(t, s.Token, "pos-records", "storeId="+s.ID+"&where.kind=tables,queue"); len(got) != 2 {
			t.Errorf("kind list: %d", len(got))
		}
		if got := List(t, s.Token, "pos-records", "storeId="+s.ID+"&where.key=T1"); len(got) != 1 {
			t.Errorf("key: %d", len(got))
		}
		if got := List(t, s.Token, "pos-records", "storeId="+s.ID+"&where.status=open"); len(got) != 1 {
			t.Errorf("status: %d", len(got))
		}
		docsWantErr(t, Call(t, "GET", "/pos-records?storeId="+s.ID+"&where.data=1", s.Token, nil), 400, "unknown filter", "where.data")
		// no date field: no /stats route ("stats" reads as a record id)
		Must(t, Call(t, "GET", "/pos-records/stats?storeId="+s.ID, s.Token, nil), 404, "no stats")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "pos-records", S(rec["id"]), false, "pos_nope")
	})

	t.Run("delete and restore", func(t *testing.T) {
		id := S(rec["id"])
		Must(t, Call(t, "DELETE", "/pos-records/"+id, s.Token, nil), 200, "delete")
		if got := List(t, s.Token, "pos-records", "storeId="+s.ID+"&where.kind=tables"); len(got) != 0 {
			t.Errorf("deleted record listed: %d", len(got))
		}
		if got := List(t, s.Token, "pos-records", "storeId="+s.ID+"&where.kind=tables&includeDeleted=true"); len(got) != 1 {
			t.Errorf("includeDeleted: %d", len(got))
		}
		r := Must(t, Call(t, "POST", "/pos-records/"+id+"/restore", s.Token, nil), 200, "restore").Body
		if r["deleted"] != false {
			t.Errorf("restored: %v", r["deleted"])
		}
		Must(t, Call(t, "POST", "/pos-records/pos_nope/restore", s.Token, nil), 404, "restore unknown")
		Must(t, Call(t, "DELETE", "/pos-records/pos_nope", s.Token, nil), 404, "delete unknown")
	})

	t.Run("next-number: sequential per terminal and key, start, errors", func(t *testing.T) {
		nn := func(tok string, b M) Resp { return Call(t, "POST", "/pos-records/next-number", tok, b) }
		num := func(b M) float64 { return F(Must(t, nn(s.Token, b), 200, "next-number").Body, "number") }
		ord := M{"storeId": s.ID, "terminal": "restaurant", "key": "order", "start": 100}
		if a, b := num(ord), num(ord); a != 100 || b != 101 {
			t.Errorf("start 100: %v %v", a, b)
		}
		ord["start"] = 5 // start only applies to a new counter
		if c := num(ord); c != 102 {
			t.Errorf("start ignored afterwards: %v", c)
		}
		if n := num(M{"storeId": s.ID, "terminal": "restaurant", "key": "ticket"}); n != 1 {
			t.Errorf("another key starts at 1: %v", n)
		}
		if n := num(M{"storeId": s.ID, "terminal": "salon", "key": "order"}); n != 1 {
			t.Errorf("another terminal starts at 1: %v", n)
		}
		if n := num(M{"storeId": s.ID, "terminal": "salon", "key": "zero", "start": 0}); n != 0 {
			t.Errorf("start 0: %v", n)
		}
		if n := F(Must(t, Call(t, "POST", "/pos-records/next-number?storeId="+other.ID, other.Token, M{"terminal": "restaurant", "key": "order"}), 200, "other store").Body, "number"); n != 1 {
			t.Errorf("counters are per store: %v", n)
		}
		docsWantErr(t, nn(s.Token, M{"storeId": s.ID, "terminal": "nope", "key": "a b"}), 400, "bad terminal/key", "terminal", "key")
		docsWantErr(t, nn(s.Token, M{"storeId": s.ID, "terminal": "restaurant", "key": "k", "start": -1}), 400, "negative start", "start")
		docsWantErr(t, nn(s.Token, M{"storeId": s.ID, "terminal": "restaurant", "key": "k", "start": 2e12}), 400, "huge start", "start")
		docsWantErr(t, nn(s.Token, M{"terminal": "restaurant", "key": "k"}), 400, "no store", "storeId")
		Must(t, nn(other.Token, M{"storeId": s.ID, "terminal": "restaurant", "key": "k"}), 403, "foreign store")
		Must(t, nn("", M{"storeId": s.ID, "terminal": "restaurant", "key": "k"}), 401, "no token")
		Must(t, Call(t, "POST", "/pos-records/next-number", s.Token, "{x"), 400, "malformed json")
		_, viewer := s.User(t, "r_viewer")
		Must(t, nn(viewer, M{"storeId": s.ID, "terminal": "restaurant", "key": "k"}), 403, "viewer")
		_, cashier := s.User(t, "r_cashier")
		Must(t, nn(cashier, M{"storeId": s.ID, "terminal": "restaurant", "key": "k"}), 200, "cashier")
	})

	t.Run("next-number: concurrent devices get distinct consecutive numbers", func(t *testing.T) {
		const n = 25
		var mu sync.Mutex
		got := map[float64]bool{}
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := Call(t, "POST", "/pos-records/next-number", s.Token, M{"storeId": s.ID, "terminal": "restaurant", "key": "conc", "start": 1})
				if r.Code != 200 {
					t.Errorf("concurrent next-number: %s", r)
					return
				}
				mu.Lock()
				got[F(r.Body, "number")] = true
				mu.Unlock()
			}()
		}
		wg.Wait()
		for i := 1; i <= n; i++ {
			if !got[float64(i)] {
				t.Errorf("numbers 1..%d expected once each, missing %d (got %v)", n, i, got)
				break
			}
		}
	})

	t.Run("store posSettings", func(t *testing.T) {
		up := Patch(t, s.Token, "stores", s.ID, M{"posSettings": M{"restaurant": M{"printFormat": "r80", "autoPrint": true}}})
		if Get(up, "posSettings.restaurant.printFormat") != "r80" || Get(up, "posSettings.restaurant.autoPrint") != true {
			t.Errorf("posSettings saved: %v", up["posSettings"])
		}
		if got := Read(t, s.Token, "stores", s.ID); Get(got, "posSettings.restaurant.printFormat") != "r80" {
			t.Errorf("posSettings read back: %v", got["posSettings"])
		}
		docsWantErr(t, PatchResp(t, s.Token, "stores", s.ID, M{"posSettings": M{"restaurant": M{"printFormat": "r99", "autoPrint": "yes"}}}),
			400, "bad print settings", "posSettings.restaurant.printFormat", "posSettings.restaurant.autoPrint")
		docsWantErr(t, PatchResp(t, s.Token, "stores", s.ID, M{"posSettings": M{"spaceship": M{}}}), 400, "unknown terminal", "posSettings.spaceship")
		docsWantErr(t, PatchResp(t, s.Token, "stores", s.ID, M{"posSettings": "r80"}), 400, "not an object", "posSettings")
		docsWantErr(t, PatchResp(t, s.Token, "stores", s.ID, M{"posSettings": M{"restaurant": M{"blob": strings.Repeat("x", 65*1024)}}}), 400, "too large", "posSettings")
	})
}

// ---------- customer packages ----------

func TestDocsPackages(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	cid := S(s.Customer(t, "")["id"])
	body := func() M {
		return M{"storeId": s.ID, "customerId": cid, "nameEn": "Wash x5 " + Uniq(), "nameAr": "غسيل ٥ مرات", "price": 100, "visits": 5,
			"used": 0, "validFrom": s.Today(), "validDays": 30, "status": "active", "services": []string{"wash", "polish"}}
	}
	var pk M

	t.Run("create and read", func(t *testing.T) {
		pk = Create(t, s.Token, "packages", body())
		if F(pk, "visits") != 5 || F(pk, "used") != 0 || F(pk, "price") != 100 || pk["validFrom"] != s.Today() || F(pk, "validDays") != 30 {
			t.Errorf("created: %v", pk)
		}
		if sv, _ := pk["services"].([]interface{}); len(sv) != 2 {
			t.Errorf("services: %v", pk["services"])
		}
		if got := Read(t, s.Token, "packages", S(pk["id"])); got["nameAr"] != "غسيل ٥ مرات" {
			t.Errorf("Arabic name: %v", got["nameAr"])
		}
	})

	t.Run("validation", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/packages", s.Token, M{"storeId": s.ID, "customerId": "", "nameEn": " ", "price": 0, "visits": 2.5,
			"used": 3, "validDays": 0}), 400, "every field", "customerId", "nameEn", "price", "visits", "used", "validDays", "validFrom")
		b := body()
		b["used"] = -1
		docsWantErr(t, Call(t, "POST", "/packages", s.Token, b), 400, "negative used", "used")
		b = body()
		b["price"] = -10
		docsWantErr(t, Call(t, "POST", "/packages", s.Token, b), 400, "negative price", "price")
	})

	t.Run("visits: use up to the total, never past it", func(t *testing.T) {
		id := S(pk["id"])
		for i := 1; i <= 5; i++ {
			up := Patch(t, s.Token, "packages", id, M{"used": i})
			if F(up, "used") != float64(i) || F(up, "version") != float64(i+1) {
				t.Errorf("visit %d: used %v version %v", i, up["used"], up["version"])
			}
		}
		docsWantErr(t, PatchResp(t, s.Token, "packages", id, M{"used": 6}), 400, "used > visits", "used")
		docsWantErr(t, PatchResp(t, s.Token, "packages", id, M{"visits": 4}), 400, "visits below used", "used")
		up := Patch(t, s.Token, "packages", id, M{"visits": 10, "status": "active"})
		if F(up, "visits")-F(up, "used") != 5 {
			t.Errorf("5 visits left: %v/%v", up["used"], up["visits"])
		}
		r := docsPut(t, s.Token, "packages", id, M{"status": "expired", "notes": "انتهت"})
		Must(t, r, 200, "PUT package")
		if r.Body["status"] != "expired" || r.Body["notes"] != "انتهت" {
			t.Errorf("PUT: %v", r.Body)
		}
		r = Call(t, "PATCH", "/packages/"+id, s.Token, M{"used": 1}, "If-Match", "2")
		if r.Code != 409 {
			t.Errorf("stale version: %s", r)
		}
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "packages", S(pk["id"]), false, "000000000000000000000000")
	})

	t.Run("lists, store isolation, delete, restore unsupported", func(t *testing.T) {
		Create(t, s.Token, "packages", body())
		if got := List(t, s.Token, "packages", "storeId="+s.ID); len(got) != 2 {
			t.Errorf("list: %d", len(got))
		}
		if got := List(t, s.Token, "packages", "storeId="+s.ID+"&q="+url.QueryEscape("غسيل")); len(got) != 2 {
			t.Errorf("Arabic search: %d", len(got))
		}
		if got := List(t, other.Token, "packages", "storeId="+other.ID); len(got) != 0 {
			t.Errorf("packages leak to another store: %d", len(got))
		}
		Must(t, Call(t, "GET", "/packages/"+S(pk["id"]), other.Token, nil), 404, "foreign read")
		Must(t, Call(t, "POST", "/packages", other.Token, body()), 403, "foreign create")
		b := body()
		b["storeId"] = other.ID
		r0 := Call(t, "POST", "/packages", other.Token, b)
		KnownBug(t, "NEW-foreign-customer", "a package accepts another store's customer id", r0.Code == 201)
		if r0.Code != 201 {
			docsWantErr(t, r0, 400, "customer of another store", "customerId")
		}
		// no date field: no stats route
		Must(t, Call(t, "GET", "/packages/stats?storeId="+s.ID, s.Token, nil), 404, "no stats")
		id := S(pk["id"])
		d := Must(t, Call(t, "DELETE", "/packages/"+id, s.Token, nil), 200, "delete").Body
		if d["deleted"] != true {
			t.Errorf("deleted: %v", d["deleted"])
		}
		if got := List(t, s.Token, "packages", "storeId="+s.ID); len(got) != 1 {
			t.Errorf("after delete: %d", len(got))
		}
		r := Call(t, "POST", "/packages/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore: %s", r)
		}
		Must(t, Call(t, "DELETE", "/packages/000000000000000000000000", s.Token, nil), 404, "delete unknown")
	})
}

// ---------- vehicles ----------

func TestDocsVehicles(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	cid := S(s.Customer(t, "")["id"])
	c2 := S(s.Customer(t, "")["id"])
	plate := func() string { return "ABC " + Digits(4) }
	var v1, v2 M

	t.Run("create: plate normalised, customer name copied", func(t *testing.T) {
		v1 = Create(t, s.Token, "vehicles", M{"storeId": s.ID, "plate": "  abc   " + Digits(4) + " ", "make": "Toyota", "model": "Camry",
			"customerId": cid, "year": 2021, "currentKm": 12345.5, "notes": "سيارة"})
		if p := S(v1["plate"]); p != strings.ToUpper(strings.Join(strings.Fields(p), " ")) || strings.Contains(p, "  ") || !strings.HasPrefix(p, "ABC ") {
			t.Errorf("plate: %q", p)
		}
		if S(v1["customerName"]) == "" || F(v1, "currentKm") != 12345.5 || v1["notes"] != "سيارة" {
			t.Errorf("created: %v", v1)
		}
		v2 = Create(t, s.Token, "vehicles", M{"storeId": s.ID, "plate": plate(), "make": "Nissan", "model": "Patrol", "customerId": c2, "year": 1990})
	})

	t.Run("validation", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/vehicles", s.Token, M{"storeId": s.ID, "plate": " ", "make": "", "year": 1900}), 400, "every field",
			"plate", "make", "customerId", "year")
		docsWantErr(t, Call(t, "POST", "/vehicles", s.Token, M{"storeId": s.ID, "plate": plate(), "make": "X", "customerId": cid, "year": 2031}), 400, "year 2031", "year")
		r := Call(t, "POST", "/vehicles", s.Token, M{"storeId": s.ID, "plate": plate(), "make": "X", "model": "Y",
			"customerId": S(other.Customer(t, "")["id"])})
		if r.Code != 400 {
			t.Errorf("customer of another store: %s", r)
		}
		KnownBug(t, "NEW-vehicle-customer-field", "unknown customer on a vehicle is reported as error.fields.foreign_fields, not customerId",
			r.Code == 400 && r.ErrField("customerId") == "")
	})

	t.Run("lists: filters, sort, search", func(t *testing.T) {
		if got := List(t, s.Token, "vehicles", "storeId="+s.ID+"&where.customerId="+cid); len(got) != 1 || got[0]["id"] != v1["id"] {
			t.Errorf("customer filter: %d", len(got))
		}
		if got := List(t, s.Token, "vehicles", "storeId="+s.ID+"&where.make=Nissan"); len(got) != 1 || got[0]["id"] != v2["id"] {
			t.Errorf("make filter: %d", len(got))
		}
		if got := List(t, s.Token, "vehicles", "storeId="+s.ID+"&sort=-year"); len(got) != 2 || got[0]["id"] != v1["id"] {
			t.Errorf("sort by year desc")
		}
		if got := List(t, s.Token, "vehicles", "storeId="+s.ID+"&q=camry"); len(got) != 1 {
			t.Errorf("search model: %d", len(got))
		}
		docsWantErr(t, Call(t, "GET", "/vehicles?storeId="+s.ID+"&sort=vin", s.Token, nil), 400, "unknown sort", "sort")
		Must(t, Call(t, "GET", "/vehicles?storeId="+s.ID+"&where.openJob=maybe", s.Token, nil), 400, "bad openJob")
		// a vehicle with an open repair job
		Create(t, s.Token, "repair-jobs", M{"storeId": s.ID, "date": s.Now(), "status": "open", "vehicleId": v2["id"], "customerId": c2, "complaint": "Brakes"})
		if got := List(t, s.Token, "vehicles", "storeId="+s.ID+"&where.openJob=true"); len(got) != 1 || got[0]["id"] != v2["id"] {
			t.Errorf("open job filter: %d", len(got))
		}
		if got := List(t, s.Token, "vehicles", "storeId="+s.ID+"&where.openJob=false"); len(got) != 1 || got[0]["id"] != v1["id"] {
			t.Errorf("no open job filter: %d", len(got))
		}
		Must(t, Call(t, "GET", "/vehicles/"+S(v1["id"]), other.Token, nil), 404, "foreign read")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "vehicles", S(v1["id"]), false, "000000000000000000000000")
	})

	t.Run("patch, put, delete, restore unsupported", func(t *testing.T) {
		id := S(v1["id"])
		up := Patch(t, s.Token, "vehicles", id, M{"color": "أبيض", "customerId": c2})
		if up["color"] != "أبيض" || up["customerId"] != c2 || F(up, "version") != 2 {
			t.Errorf("patch: %v", up)
		}
		docsWantErr(t, PatchResp(t, s.Token, "vehicles", id, M{"make": ""}), 400, "clear make", "make")
		Must(t, docsPut(t, s.Token, "vehicles", id, M{"vin": "JT123456789"}), 200, "PUT vehicle")
		Must(t, Call(t, "DELETE", "/vehicles/"+id, s.Token, nil), 200, "delete")
		if docsHas(List(t, s.Token, "vehicles", "storeId="+s.ID), id) {
			t.Error("deleted vehicle listed")
		}
		r := Call(t, "POST", "/vehicles/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore: %s", r)
		}
		Must(t, Call(t, "GET", "/vehicles/stats?storeId="+s.ID, s.Token, nil), 404, "no stats")
		_, cashier := s.User(t, "r_cashier")
		Must(t, Call(t, "POST", "/vehicles", cashier, M{"storeId": s.ID, "plate": plate(), "make": "X", "customerId": cid}), 403, "cashier has no workshop")
	})
}

// ---------- repair jobs ----------

// docsRepairGrand is the legacy job total: labour is VAT-inclusive, parts are not.
func docsRepairGrand(parts []docsLn, labour, vatPct float64) float64 {
	pt := 0.0
	for _, p := range parts {
		pt += p.q * (p.p - p.d)
	}
	if vatPct <= 0 {
		return docsR2(labour + pt)
	}
	sub := docsR2(pt + docsR2(labour/(1+vatPct/100)))
	return docsR2(sub + docsR2(sub*vatPct/100))
}

func TestDocsRepairJobs(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	pid := S(s.Product(t, 10, 20, 20)["id"])
	cid := S(s.Customer(t, "")["id"])
	veh := Create(t, s.Token, "vehicles", M{"storeId": s.ID, "plate": "RJ " + Digits(4), "make": "Ford", "model": "Focus", "customerId": cid})
	vid := S(veh["id"])
	vatPct := F(s.Rec, "vatPercent")
	job := func(complaint string, labour float64, parts ...M) M {
		return M{"storeId": s.ID, "date": s.Now(), "status": "open", "vehicleId": vid, "customerId": cid, "complaint": complaint,
			"labour": labour, "parts": parts}
	}
	part := func(q, p, d float64) M { return M{"productId": pid, "qty": q, "unitPrice": p, "unitDiscount": d} }
	var j1, j2, j3 M

	t.Run("create: numbers, vehicle copied, parts+labour total oracle", func(t *testing.T) {
		j1 = Create(t, s.Token, "repair-jobs", job("صوت في المحرك\nsecond line", 99.99, part(3, 19.99, 0.33)))
		j2 = Create(t, s.Token, "repair-jobs", job("Brakes", 57.5, part(2, 25.5, 0.5)))
		j3 = Create(t, s.Token, "repair-jobs", job("Oil", 0))
		if j1["code"] != "RJ-1" || j2["code"] != "RJ-2" || j3["code"] != "RJ-3" {
			t.Errorf("numbers: %v %v %v", j1["code"], j2["code"], j3["code"])
		}
		if j1["title"] != "صوت في المحرك" || j1["plate"] != veh["plate"] || j1["make"] != "Ford" || F(j1, "vatPercent") != vatPct {
			t.Errorf("title/vehicle/vat: %v %v %v %v", j1["title"], j1["plate"], j1["make"], j1["vatPercent"])
		}
		if S(Get(j1, "parts.0.nameEn")) == "" || F(j1, "parts.0.qty") != 3 {
			t.Errorf("part name from the product: %v", j1["parts"])
		}
		r := Must(t, Call(t, "GET", "/repair-jobs?storeId="+s.ID+"&sum=grand&ids="+S(j1["id"]), s.Token, nil), 200, "sum one").Body
		EqMoney(t, "job 1 total", F(r, "sums.grand"), docsRepairGrand([]docsLn{{3, 19.99, 0.33, 0}}, 99.99, vatPct))
		r = Must(t, Call(t, "GET", "/repair-jobs?storeId="+s.ID+"&sum=grand", s.Token, nil), 200, "sum all").Body
		EqMoney(t, "all jobs total", F(r, "sums.grand"), docsRepairGrand([]docsLn{{3, 19.99, 0.33, 0}}, 99.99, vatPct)+
			docsRepairGrand([]docsLn{{2, 25.5, 0.5, 0}}, 57.5, vatPct))
		// parts on a job do not move stock (they move with the sale made from it)
		docsStockStays(t, s, pid, 20)
	})

	t.Run("validation", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/repair-jobs", s.Token, M{"storeId": s.ID, "odometer": -1, "status": "fixed"}), 400, "every field",
			"date", "vehicleId", "customerId", "complaint", "odometer", "status")
		docsWantErr(t, Call(t, "POST", "/repair-jobs", s.Token, job("x", 0, part(0, 1, 0))), 400, "zero qty part", "parts.0.qty")
		bad := job("x", 0, M{"productId": "000000000000000000000000", "qty": 1, "unitPrice": 1})
		docsWantErr(t, Call(t, "POST", "/repair-jobs", s.Token, bad), 400, "unknown part", "parts.0.productId")
		b := job("x", 0)
		b["vehicleId"] = "000000000000000000000000"
		docsWantErr(t, Call(t, "POST", "/repair-jobs", s.Token, b), 400, "unknown vehicle", "vehicleId")
		b = job("x", 0)
		b["estDelivery"] = "tomorrow"
		docsWantErr(t, Call(t, "POST", "/repair-jobs", s.Token, b), 400, "bad estDelivery", "estDelivery")
		Must(t, Call(t, "POST", "/repair-jobs", other.Token, job("x", 0)), 403, "foreign store")
	})

	t.Run("status flow and filters", func(t *testing.T) {
		id := S(j1["id"])
		for i, st := range []string{"in_progress", "completed", "delivered", "closed"} {
			up := Patch(t, s.Token, "repair-jobs", id, M{"status": st})
			if up["status"] != st || F(up, "version") != float64(i+2) {
				t.Errorf("status %s: %v v%v", st, up["status"], up["version"])
			}
		}
		docsWantErr(t, PatchResp(t, s.Token, "repair-jobs", id, M{"status": "done"}), 400, "bad status", "status")
		Patch(t, s.Token, "repair-jobs", S(j3["id"]), M{"status": "cancelled"})
		if got := List(t, s.Token, "repair-jobs", "storeId="+s.ID+"&where.status=open"); len(got) != 1 || got[0]["id"] != j2["id"] {
			t.Errorf("open jobs: %d", len(got))
		}
		if got := List(t, s.Token, "repair-jobs", "storeId="+s.ID+"&where.status=done"); len(got) != 2 {
			t.Errorf("done jobs: %d", len(got))
		}
		if got := List(t, s.Token, "repair-jobs", "storeId="+s.ID+"&where.status=notCancelled"); len(got) != 2 {
			t.Errorf("not cancelled: %d", len(got))
		}
		if got := List(t, s.Token, "repair-jobs", "storeId="+s.ID+"&where.vehicleId="+vid); len(got) != 3 {
			t.Errorf("by vehicle: %d", len(got))
		}
		Must(t, Call(t, "GET", "/repair-jobs?storeId="+s.ID+"&where.status=weird", s.Token, nil), 400, "bad status filter")
		st := docsStats(t, s, "repair-jobs", "sum=one,labour&groupBy=status")
		if F(st, "count") != 3 || F(st, "groups.closed.count") != 1 || F(st, "groups.cancelled.count") != 1 {
			t.Errorf("stats by status: %v", st)
		}
		EqMoney(t, "labour sum", F(st, "sums.labour"), 99.99+57.5)
		Must(t, Call(t, "GET", "/repair-jobs/stats?storeId="+s.ID+"&to=2026-02-30", s.Token, nil), 400, "bad to")
	})

	t.Run("patch parts and labour, put", func(t *testing.T) {
		id := S(j2["id"])
		up := Patch(t, s.Token, "repair-jobs", id, M{"parts": []M{part(1, 100, 0), part(2, 0.5, 0)}, "labour": 115, "workDone": "تم"})
		if len(Objs(up["parts"])) != 2 || F(up, "labour") != 115 || up["workDone"] != "تم" {
			t.Errorf("patch parts: %v", up)
		}
		r := Must(t, Call(t, "GET", "/repair-jobs?storeId="+s.ID+"&sum=grand&ids="+id, s.Token, nil), 200, "sum").Body
		EqMoney(t, "job 2 total after patch", F(r, "sums.grand"), docsRepairGrand([]docsLn{{1, 100, 0, 0}, {2, 0.5, 0, 0}}, 115, vatPct))
		Must(t, docsPut(t, s.Token, "repair-jobs", id, M{"inspection": "ok"}), 200, "PUT job")
		docsWantErr(t, Call(t, "PUT", "/repair-jobs/"+id, s.Token, M{"date": s.Now()}), 400, "PUT without required fields", "vehicleId")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "repair-jobs", S(j2["id"]), true, "000000000000000000000000")
	})

	t.Run("delete: number reuse, restore unsupported", func(t *testing.T) {
		Must(t, Call(t, "DELETE", "/repair-jobs/"+S(j2["id"]), s.Token, nil), 200, "delete")
		r := Call(t, "POST", "/repair-jobs/"+S(j2["id"])+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore: %s", r)
		}
		n := Create(t, s.Token, "repair-jobs", job("After delete", 0))
		codes := map[string]int{}
		for _, row := range List(t, s.Token, "repair-jobs", "storeId="+s.ID+"&includeDeleted=1") {
			codes[S(row["code"])]++
		}
		KnownBug(t, "#12", "repair job numbers repeat after a delete (count+1)", codes[S(n["code"])] > 1)
		Must(t, Call(t, "DELETE", "/repair-jobs/000000000000000000000000", s.Token, nil), 404, "delete unknown")
		Must(t, Call(t, "GET", "/repair-jobs/"+S(j1["id"]), other.Token, nil), 404, "foreign read")
	})
}

// ---------- signatures ----------

func TestDocsSignatures(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	var sg M

	t.Run("create from a data URL, image served as a file", func(t *testing.T) {
		sg = Create(t, s.Token, "signatures", M{"storeId": s.ID, "name": "توقيع المدير", "image": docsPNG})
		img := S(sg["image"])
		if sg["name"] != "توقيع المدير" || !strings.HasPrefix(img, "/images/"+s.ID+"/signatures/") || !strings.HasSuffix(img, ".png") {
			t.Errorf("created: %v", sg)
		}
		res := CallURL(t, "GET", baseURL+img, "", nil)
		if res.Code != 200 || !strings.HasPrefix(res.Raw, "\x89PNG") {
			t.Errorf("image file: HTTP %d (%d bytes)", res.Code, len(res.Raw))
		}
	})

	t.Run("validation and access", func(t *testing.T) {
		docsWantErr(t, Call(t, "POST", "/signatures", s.Token, M{"storeId": s.ID, "name": " ", "image": ""}), 400, "empty", "name", "image")
		Must(t, Call(t, "POST", "/signatures", other.Token, M{"storeId": s.ID, "name": "x", "image": docsPNG}), 403, "foreign store")
		_, sm := s.User(t, "r_salesman")
		Must(t, Call(t, "POST", "/signatures", sm, M{"storeId": s.ID, "name": "x", "image": docsPNG}), 403, "salesman has no settings")
		Must(t, Call(t, "GET", "/signatures/"+S(sg["id"]), other.Token, nil), 404, "foreign read")
	})

	t.Run("route matrix", func(t *testing.T) {
		docsRoutes(t, s, "signatures", S(sg["id"]), false, "000000000000000000000000")
	})

	t.Run("list, patch, put, delete, restore unsupported", func(t *testing.T) {
		id := S(sg["id"])
		if got := List(t, s.Token, "signatures", "storeId="+s.ID); len(got) != 1 || got[0]["id"] != id {
			t.Errorf("list: %d", len(got))
		}
		up := Patch(t, s.Token, "signatures", id, M{"name": "Manager"})
		if up["name"] != "Manager" || up["image"] != sg["image"] || F(up, "version") != 2 {
			t.Errorf("rename keeps the image: %v", up)
		}
		docsWantErr(t, PatchResp(t, s.Token, "signatures", id, M{"name": ""}), 400, "clear name", "name")
		Must(t, docsPut(t, s.Token, "signatures", id, M{"name": "Manager 2"}), 200, "PUT signature")
		Must(t, Call(t, "DELETE", "/signatures/"+id, s.Token, nil), 200, "delete")
		if got := List(t, s.Token, "signatures", "storeId="+s.ID); len(got) != 0 {
			t.Errorf("deleted signature listed: %d", len(got))
		}
		r := Call(t, "POST", "/signatures/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore: %s", r)
		}
		Must(t, Call(t, "DELETE", "/signatures/000000000000000000000000", s.Token, nil), 404, "delete unknown")
		Must(t, Call(t, "GET", "/signatures/stats?storeId="+s.ID, s.Token, nil), 404, "no stats")
	})
}
