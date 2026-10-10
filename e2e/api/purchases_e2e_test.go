//go:build e2e

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Buying and inventory: purchases, purchase returns, purchase orders and
// requests, purchase bills, stock transfers, warehouses, products and their
// lookups, vendors and customers, RFQs, threads and notifications.

// ---------- helpers (prefix pur) ----------

// purLine is one document line for the oracle.
type purLine struct{ qty, price, disc float64 }

// purOracle computes a document's taxable amount, VAT and net in integer
// cents from the inputs; VAT rounds halves up (ZATCA rounding).
func purOracle(lines []purLine, discount, shipping, vatPct float64) (taxable, vat, net int64) {
	for _, l := range lines {
		taxable += Cents(l.qty * (l.price - l.disc))
	}
	taxable += Cents(shipping) - Cents(discount)
	vat = (taxable*int64(vatPct*100) + 5000) / 10000
	return taxable, vat, taxable + vat
}

func purC(c int64) float64 { return float64(c) / 100 }

// purItem is a contract line in warehouse wh.
func purItem(pid, wh string, qty, price, disc float64) M {
	return M{"productId": pid, "qty": qty, "unitPrice": price, "unitDiscount": disc, "warehouseId": wh, "vatPercent": 15}
}

// purStockIn is the product's quantity in one warehouse.
func purStockIn(t *testing.T, s *Store, pid, wh string) float64 {
	t.Helper()
	return F(Read(t, s.Token, "products", pid), "stock."+wh+".qty")
}

// purWaitStock waits for the product's quantity in a warehouse.
func purWaitStock(t *testing.T, s *Store, pid, wh string, want float64) {
	t.Helper()
	var got float64
	Eventually(t, "stock", func() bool { got = purStockIn(t, s, pid, wh); return got == want })
}

// purWaitBalance waits for a vendor's credit balance (negative = payable).
func purWaitBalance(t *testing.T, s *Store, vid string, want float64) {
	t.Helper()
	Eventually(t, "vendor balance", func() bool {
		return Cents(F(Read(t, s.Token, "vendors", vid), "creditBalance")) == Cents(want)
	})
}

// purIDs is the ids of a list answer.
func purIDs(rows []M) map[string]bool {
	out := map[string]bool{}
	for _, r := range rows {
		out[S(r["id"])] = true
	}
	return out
}

// purNeedErr fails unless the answer is the status with error.fields[field].
func purNeedErr(t *testing.T, r Resp, code int, field, what string) {
	t.Helper()
	if r.Code != code {
		t.Fatalf("%s: want HTTP %d, got %s", what, code, r)
	}
	if field != "" && r.ErrField(field) == "" {
		t.Errorf("%s: want error.fields[%s], got %s", what, field, r)
	}
}

// purAgo is a contract datetime d before now in the store's zone.
func purAgo(s *Store, d time.Duration) string {
	return time.Now().In(s.Loc).Add(-d).Format("2006-01-02T15:04")
}

// purRaw calls a legacy /v1 route and tolerates a dropped connection (a
// panicking legacy handler); err is non-nil when no answer came back.
func purRaw(t *testing.T, method, path, token string, body interface{}) (int, string, error) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, baseURL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	rb, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(rb), nil
}

// ---------- purchases ----------

// TestPurchases_DocumentFlow: lines, discounts, VAT oracle, payments, payment
// status, stock per warehouse, cost price, vendor payable, edits, delete /
// restore, stats and list filters.
func TestPurchases_DocumentFlow(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	p := s.Product(t, 10, 20, 5)
	pid := S(p["id"])
	v := s.Vendor(t)
	vid := S(v["id"])
	wh := Create(t, s.Token, "warehouses", M{"storeId": s.ID, "nameEn": "Second WH", "code": "SEC"})
	whID := S(wh["id"])
	// autoRetail: the last purchase's retail/wholesale prices become the product's
	Patch(t, s.Token, "products", pid, M{"pricing": M{"purchase": 10, "retail": 20, "autoRetail": true, "autoWholesale": true}})

	type doc struct {
		id                   string
		net, vat, paid, cash int64
	}
	var docs []doc

	// line 1 into the second warehouse with a unit discount, line 2 into the main store
	l1 := purItem(pid, whID, 4, 25, 1.5)
	l1["retailPrice"], l1["wholesalePrice"] = 40, 35
	lines := []purLine{{4, 25, 1.5}, {3, 9.99, 0}}
	_, wantVAT, wantNet := purOracle(lines, 3.33, 7.5, 15)
	var purID string

	t.Run("create with lines, discount, shipping and a partial payment", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid,
			"vendorInvoiceNo": "INV-" + Uniq(), "discount": 3.33, "shipping": 7.5, "remarks": "شحنة أولى",
			"items":    []M{l1, purItem(pid, s.MS, 3, 9.99, 0)},
			"payments": []M{{"date": s.Now(), "amount": 50, "method": "cash"}}}), 201, "create purchase").Body
		purID = S(r["id"])
		if !strings.HasPrefix(S(r["code"]), "P-INV-") {
			t.Errorf("purchase number: %v", r["code"])
		}
		EqMoney(t, "net", F(r, "legacyTotals.net"), purC(wantNet))
		EqMoney(t, "vat", F(r, "legacyTotals.vat"), purC(wantVAT))
		EqMoney(t, "paid", F(r, "legacyTotals.paid"), 50)
		EqMoney(t, "balance", F(r, "legacyTotals.balance"), purC(wantNet)-50)
		if S(Get(r, "legacyTotals.paymentStatus")) != "paid_partially" {
			t.Errorf("payment status: %v", Get(r, "legacyTotals.paymentStatus"))
		}
		if S(r["remarks"]) != "شحنة أولى" || S(r["vendorName"]) != S(v["nameEn"]) {
			t.Errorf("remarks / vendor name: %v %v", r["remarks"], r["vendorName"])
		}
		docs = append(docs, doc{purID, wantNet, wantVAT, 5000, 0})
		// stock in per warehouse
		purWaitStock(t, s, pid, whID, 4)
		purWaitStock(t, s, pid, s.MS, 5+3)
		// cost price = the last purchase line's price; autoRetail copies retail/wholesale
		pr := Read(t, s.Token, "products", pid)
		if F(pr, "pricing.purchase") != 9.99 || F(pr, "pricing.retail") != 40 || F(pr, "pricing.wholesale") != 35 {
			t.Errorf("product pricing after purchase: %v", pr["pricing"])
		}
		purWaitBalance(t, s, vid, -(purC(wantNet) - 50))
	})

	t.Run("pay the rest: status paid, payable cleared", func(t *testing.T) {
		cur := Read(t, s.Token, "purchases", purID)
		pays := append(Objs(cur["payments"]), M{"date": s.Now(), "amount": purC(wantNet) - 50, "method": "bank_transfer"})
		r := Patch(t, s.Token, "purchases", purID, M{"payments": pays})
		if S(Get(r, "legacyTotals.paymentStatus")) != "paid" || Cents(F(r, "legacyTotals.balance")) != 0 {
			t.Errorf("after full payment: %v", r["legacyTotals"])
		}
		if len(Objs(r["payments"])) != 2 {
			t.Errorf("payments: %v", r["payments"])
		}
		docs[0].paid = wantNet
		purWaitBalance(t, s, vid, 0)
	})

	t.Run("validation errors", func(t *testing.T) {
		base := func(mod func(b M)) M {
			b := M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "items": []M{purItem(pid, s.MS, 1, 5, 0)}}
			mod(b)
			return b
		}
		cases := []struct {
			name, field string
			body        M
		}{
			{"unsupported payment method", "payments.0.method", base(func(b M) { b["payments"] = []M{{"amount": 1, "method": "bitcoin"}} })},
			{"negative payment", "payments.0.amount", base(func(b M) { b["payments"] = []M{{"amount": -1, "method": "cash"}} })},
			{"overpayment", "payments", base(func(b M) { b["payments"] = []M{{"amount": 100, "method": "cash"}} })},
			{"zero unit price", "items.0.unitPrice", base(func(b M) { b["items"] = []M{purItem(pid, s.MS, 1, 0, 0)} })},
			{"zero qty", "items.0.qty", base(func(b M) { b["items"] = []M{purItem(pid, s.MS, 0, 5, 0)} })},
			{"unknown product", "items.0.productId", base(func(b M) { b["items"] = []M{purItem("not-a-product", s.MS, 1, 5, 0)} })},
			{"product of another store", "items.0.productId", base(func(b M) { b["items"] = []M{purItem(S(other.Product(t, 1, 2, 0)["id"]), s.MS, 1, 5, 0)} })},
			{"unknown warehouse", "items.0.warehouseId", base(func(b M) { b["items"] = []M{purItem(pid, "nope", 1, 5, 0)} })},
			{"no lines", "items", base(func(b M) { b["items"] = []M{} })},
			{"no date", "date", base(func(b M) { delete(b, "date") })},
			{"bad date", "date", base(func(b M) { b["date"] = "15/01/2026" })},
			{"negative discount", "discount", base(func(b M) { b["discount"] = -1 })},
			{"unknown vendor (client id)", "vendorId", base(func(b M) { b["vendorId"] = "ven_unknown" })},
		}
		for _, c := range cases {
			purNeedErr(t, Call(t, "POST", "/purchases", s.Token, c.body), 400, c.field, c.name)
		}
		// a line that makes the net negative is reported as a payments error,
		// because the paid-vs-net check runs before the line checks
		for _, c := range []struct {
			name, field string
			body        M
		}{
			{"negative unit price", "items.0.unitPrice", base(func(b M) { b["items"] = []M{purItem(pid, s.MS, 1, -2, 0)} })},
			{"negative qty", "items.0.qty", base(func(b M) { b["items"] = []M{purItem(pid, s.MS, -1, 5, 0)} })},
			{"discount above price", "items.0.unitDiscount", base(func(b M) { b["items"] = []M{purItem(pid, s.MS, 1, 5, 6)} })},
		} {
			r := Call(t, "POST", "/purchases", s.Token, c.body)
			if r.Code != 400 {
				t.Errorf("%s: %s", c.name, r)
			}
			KnownBug(t, "NEW-PUR-PAY-MASK", c.name+": reported as error.fields[payments] instead of "+c.field, r.ErrField(c.field) == "")
		}
		purNeedErr(t, Call(t, "POST", "/purchases", s.Token, base(func(b M) { delete(b, "storeId") })), 400, "storeId", "no store")
		purNeedErr(t, Call(t, "POST", "/purchases", s.Token, base(func(b M) { b["storeId"] = other.ID })), 403, "", "another store")
		purNeedErr(t, Call(t, "POST", "/purchases", "", base(func(b M) {})), 401, "", "no token")
		purNeedErr(t, Call(t, "POST", "/purchases", s.Token, `{"storeId":`), 400, "", "malformed JSON")
	})

	t.Run("vendor invoice number is unique per vendor", func(t *testing.T) {
		inv := "VI-" + Uniq()
		body := func(vendor string) M {
			return M{"storeId": s.ID, "date": s.Now(), "vendorId": vendor, "vendorInvoiceNo": inv, "items": []M{purItem(pid, s.MS, 1, 2, 0)}}
		}
		r := Must(t, Call(t, "POST", "/purchases", s.Token, body(vid)), 201, "first invoice").Body
		_, vat, net := purOracle([]purLine{{1, 2, 0}}, 0, 0, 15)
		docs = append(docs, doc{S(r["id"]), net, vat, 0, 0})
		dup := Call(t, "POST", "/purchases", s.Token, body(vid))
		if dup.Code != 400 {
			t.Fatalf("duplicate vendor invoice no.: %s", dup)
		}
		KnownBug(t, "NEW-PUR-VINV-FIELD", "duplicate vendor invoice error is reported under the legacy key vendor_invoice_no, not vendorInvoiceNo",
			dup.ErrField("vendorInvoiceNo") == "" && dup.ErrField("vendor_invoice_no") != "")
		v2 := s.Vendor(t)
		r2 := Must(t, Call(t, "POST", "/purchases", s.Token, body(S(v2["id"]))), 201, "same number, other vendor").Body
		docs = append(docs, doc{S(r2["id"]), net, vat, 0, 0})
	})

	t.Run("unknown vendor ids are rejected", func(t *testing.T) {
		ov := other.Vendor(t)
		for _, id := range []string{S(ov["id"]), "000000000000000000000001"} {
			r := Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": id, "items": []M{purItem(pid, s.MS, 1, 2, 0)}})
			KnownBug(t, "NEW-PUR-UNKNOWN-VENDOR", "a vendorId that does not exist in the store (or belongs to another company) is booked to the UNKNOWN vendor with 201 instead of 400 vendorId",
				r.Code == 201 && S(r.Body["vendorName"]) == "UNKNOWN")
			if r.Code == 201 {
				_, vat, net := purOracle([]purLine{{1, 2, 0}}, 0, 0, 15)
				docs = append(docs, doc{S(r.Body["id"]), net, vat, 0, 0})
			}
		}
		// the other company's vendor is untouched
		if b := F(Read(t, other.Token, "vendors", S(ov["id"])), "creditBalance"); b != 0 {
			t.Errorf("other company's vendor balance changed: %v", b)
		}
	})

	t.Run("no vendor books the UNKNOWN vendor", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": s.Now(), "items": []M{purItem(pid, s.MS, 1, 5, 0)}}), 201, "no vendor").Body
		if S(r["vendorName"]) != "UNKNOWN" || S(r["vendorNameAr"]) != "مجهول" {
			t.Errorf("unknown vendor: %v / %v", r["vendorName"], r["vendorNameAr"])
		}
		_, vat, net := purOracle([]purLine{{1, 5, 0}}, 0, 0, 15)
		docs = append(docs, doc{S(r["id"]), net, vat, 0, 0})
	})

	t.Run("VAT rounding of half cents", func(t *testing.T) {
		for _, price := range []float64{0.1, 0.3, 0.7, 1.1, 2.5, 0.03, 33.3} {
			r := Must(t, Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid,
				"items": []M{purItem(pid, s.MS, 1, price, 0)}}), 201, "purchase").Body
			_, vat, net := purOracle([]purLine{{1, price, 0}}, 0, 0, 15)
			docs = append(docs, doc{S(r["id"]), net, vat, 0, 0})
			got := Cents(F(r, "legacyTotals.vat"))
			if price == 33.3 {
				// 33.30 × 15% = 4.995 → 5.00; the float product is 4.99499…
				KnownBug(t, "NEW-VAT-HALF", "VAT 4.995 on a 33.30 purchase is rounded down to 4.99 (float half-cent rounding)", got == vat-1)
				continue
			}
			if got != vat || Cents(F(r, "legacyTotals.net")) != net {
				t.Errorf("price %.2f: vat %d net %v, want %d / %d", price, got, F(r, "legacyTotals.net"), vat, net)
			}
		}
	})

	t.Run("huge quantities stay exact", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 1000000, 999.99, 0)}}), 201, "huge").Body
		_, vat, net := purOracle([]purLine{{1000000, 999.99, 0}}, 0, 0, 15)
		EqMoney(t, "huge net", F(r, "legacyTotals.net"), purC(net))
		docs = append(docs, doc{S(r["id"]), net, vat, 0, 0})
	})

	var editID string
	t.Run("PATCH quantity re-adjusts stock; If-Match conflicts", func(t *testing.T) {
		before := purStockIn(t, s, pid, s.MS)
		r := Must(t, Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 2, 8, 0)}}), 201, "purchase").Body
		editID = S(r["id"])
		purWaitStock(t, s, pid, s.MS, before+2)
		r = Patch(t, s.Token, "purchases", editID, M{"items": []M{purItem(pid, s.MS, 6, 8, 0)}})
		_, vat, net := purOracle([]purLine{{6, 8, 0}}, 0, 0, 15)
		EqMoney(t, "net after qty edit", F(r, "legacyTotals.net"), purC(net))
		if Num(r["version"]) != 2 {
			t.Errorf("version after one edit: %v", r["version"])
		}
		purWaitStock(t, s, pid, s.MS, before+6)
		docs = append(docs, doc{editID, net, vat, 0, 0})
		stale := Call(t, "PATCH", "/purchases/"+editID, s.Token, M{"remarks": "x"}, "If-Match", "1")
		if stale.Code != 409 || stale.ErrCode() != "version_conflict" {
			t.Errorf("stale If-Match: %s", stale)
		}
		bad := Call(t, "PATCH", "/purchases/"+editID, s.Token, M{"items": []M{purItem(pid, s.MS, 0, 8, 0)}})
		purNeedErr(t, bad, 400, "items.0.qty", "zero qty on edit")
		purNeedErr(t, Call(t, "PATCH", "/purchases/000000000000000000000001", s.Token, M{"remarks": "x"}), 404, "", "patch unknown id")
		purNeedErr(t, Call(t, "PATCH", "/purchases/"+editID, other.Token, M{"remarks": "x"}), 404, "", "patch from another company")
		purNeedErr(t, Call(t, "PATCH", "/purchases/"+editID+"?storeId="+other.ID, s.Token, M{"remarks": "x"}), 403, "", "patch naming a foreign store")
	})

	t.Run("PUT replaces the document", func(t *testing.T) {
		before := purStockIn(t, s, pid, s.MS)
		r := Must(t, Call(t, "PUT", "/purchases/"+editID, s.Token, M{"date": s.Now(), "vendorId": vid, "remarks": "replaced",
			"items": []M{purItem(pid, s.MS, 1, 8, 0)}}), 200, "put").Body
		_, vat, net := purOracle([]purLine{{1, 8, 0}}, 0, 0, 15)
		EqMoney(t, "net after put", F(r, "legacyTotals.net"), purC(net))
		if S(r["remarks"]) != "replaced" || len(Objs(r["items"])) != 1 {
			t.Errorf("put result: %v %v", r["remarks"], r["items"])
		}
		purWaitStock(t, s, pid, s.MS, before-5)
		docs[len(docs)-1] = doc{editID, net, vat, 0, 0}
		purNeedErr(t, Call(t, "PUT", "/purchases/"+editID, s.Token, M{"date": s.Now(), "vendorId": vid, "items": []M{}}), 400, "items", "put without lines")
	})

	t.Run("DELETE is unsupported; restore", func(t *testing.T) {
		r := Call(t, "DELETE", "/purchases/"+purID, s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("delete purchase: %s", r)
		}
		if Read(t, s.Token, "purchases", purID)["deleted"] != false {
			t.Error("purchase must not be deleted")
		}
		// restoring a live record answers it unchanged
		rr := Must(t, Call(t, "POST", "/purchases/"+purID+"/restore", s.Token, nil), 200, "restore live").Body
		if S(rr["id"]) != purID {
			t.Errorf("restore: %v", rr["id"])
		}
		purNeedErr(t, Call(t, "POST", "/purchases/000000000000000000000001/restore", s.Token, nil), 404, "", "restore unknown")
		purNeedErr(t, Call(t, "GET", "/purchases/000000000000000000000001", s.Token, nil), 404, "", "get unknown")
		purNeedErr(t, Call(t, "GET", "/purchases/"+purID, other.Token, nil), 404, "", "get from another company")
		purNeedErr(t, Call(t, "DELETE", "/purchases/000000000000000000000001", s.Token, nil), 404, "", "delete unknown")
	})

	t.Run("stats match the oracle", func(t *testing.T) {
		var net, vat, paid, bal int64
		for _, d := range docs {
			net += d.net
			vat += d.vat
			paid += d.paid
			if b := d.net - d.paid - d.cash; b > 0 {
				bal += b
			}
		}
		r := Must(t, Call(t, "GET", "/purchases/stats?storeId="+s.ID+"&sum=net,vat,paid,balance,one", s.Token, nil), 200, "stats").Body
		if int(F(r, "count")) != len(docs) || int(F(r, "sums.one")) != len(docs) {
			t.Errorf("stats count %v, want %d", r["count"], len(docs))
		}
		// the half-cent VAT bug moves VAT and net by one cent
		halfBug := int64(0)
		if KB := Cents(F(r, "sums.vat")); KB == vat-1 {
			halfBug = 1
		}
		if Cents(F(r, "sums.vat")) != vat-halfBug || Cents(F(r, "sums.net")) != net-halfBug {
			t.Errorf("stats vat/net %v %v, want %v %v", F(r, "sums.vat"), F(r, "sums.net"), purC(vat), purC(net))
		}
		EqMoney(t, "stats paid", F(r, "sums.paid"), purC(paid))
		if d := Cents(F(r, "sums.balance")) - (bal - halfBug); d != 0 {
			t.Errorf("stats balance %v, want %v", F(r, "sums.balance"), purC(bal))
		}
		// a period with nothing in it
		e := Must(t, Call(t, "GET", "/purchases/stats?storeId="+s.ID+"&from=2020-01-01&to=2020-01-31&sum=net,one", s.Token, nil), 200, "empty stats").Body
		if F(e, "count") != 0 || F(e, "sums.net") != 0 {
			t.Errorf("empty period stats: %v", e)
		}
		purNeedErr(t, Call(t, "GET", "/purchases/stats?storeId="+s.ID+"&from=2026-13-01", s.Token, nil), 400, "from", "bad from")
		purNeedErr(t, Call(t, "GET", "/purchases/stats?storeId="+s.ID+"&from=2026-02-01&to=2026-01-01", s.Token, nil), 400, "from", "from after to")
		purNeedErr(t, Call(t, "GET", "/purchases/stats?storeId="+other.ID, s.Token, nil), 403, "", "stats of another store")
	})

	t.Run("list filters, search, sort, select, pagination", func(t *testing.T) {
		all := List(t, s.Token, "purchases", "storeId="+s.ID+"&limit=500")
		if len(all) != len(docs) {
			t.Errorf("list: %d rows, want %d", len(all), len(docs))
		}
		paid := List(t, s.Token, "purchases", "storeId="+s.ID+"&where.paymentStatus=paid&select=id")
		if len(paid) != 1 || S(paid[0]["id"]) != purID {
			t.Errorf("where.paymentStatus=paid: %v", paid)
		}
		mine := List(t, s.Token, "purchases", "storeId="+s.ID+"&where.vendorId="+vid+"&select=id,vendorId")
		for _, r := range mine {
			if S(r["vendorId"]) != vid {
				t.Errorf("where.vendorId leaked %v", r)
			}
		}
		page := Must(t, Call(t, "GET", "/purchases?storeId="+s.ID+"&sort=-code&limit=2&page=1&select=id,code", s.Token, nil), 200, "page")
		if len(page.Data()) != 2 || int(F(page.Body, "total")) != len(docs) || S(page.Data()[0]["code"]) <= S(page.Data()[1]["code"]) {
			t.Errorf("sorted page: %v", page.Body)
		}
		if _, ok := page.Data()[0]["items"]; ok {
			t.Error("select=id,code returned items")
		}
		q := List(t, s.Token, "purchases", "storeId="+s.ID+"&q="+url.QueryEscape(S(Read(t, s.Token, "purchases", purID)["code"]))+"&select=id")
		if !purIDs(q)[purID] {
			t.Errorf("q by code: %v", q)
		}
		purNeedErr(t, Call(t, "GET", "/purchases?storeId="+s.ID+"&where.bogus=1", s.Token, nil), 400, "where.bogus", "unknown filter")
		purNeedErr(t, Call(t, "GET", "/purchases?storeId="+s.ID+"&limit=0", s.Token, nil), 400, "limit", "limit 0")
		purNeedErr(t, Call(t, "GET", "/purchases?storeId="+s.ID+"&page=0", s.Token, nil), 400, "page", "page 0")
		purNeedErr(t, Call(t, "GET", "/purchases?storeId="+s.ID+"&select=a$b", s.Token, nil), 400, "select", "bad select")
		purNeedErr(t, Call(t, "GET", "/purchases", s.Token, nil), 400, "storeId", "list without store")
		purNeedErr(t, Call(t, "GET", "/purchases?storeId="+other.ID, s.Token, nil), 403, "", "list of another store")
	})

	t.Run("backdated document and date filters", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchases", s.Token, M{"storeId": s.ID, "date": "2026-01-15T10:00", "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 1, 3, 0)}}), 201, "backdated").Body
		if S(r["date"]) != "2026-01-15T10:00" {
			t.Errorf("date round trip: %v", r["date"])
		}
		jan := List(t, s.Token, "purchases", "storeId="+s.ID+"&from=2026-01-01&to=2026-01-31&select=id")
		if len(jan) != 1 || S(jan[0]["id"]) != S(r["id"]) {
			t.Errorf("January list: %v", jan)
		}
		st := Must(t, Call(t, "GET", "/purchases/stats?storeId="+s.ID+"&from=2026-01-01&to=2026-01-31&sum=net,one", s.Token, nil), 200, "Jan stats").Body
		_, _, net := purOracle([]purLine{{1, 3, 0}}, 0, 0, 15)
		if F(st, "count") != 1 || Cents(F(st, "sums.net")) != net {
			t.Errorf("January stats: %v", st)
		}
	})

	t.Run("permissions", func(t *testing.T) {
		_, viewer := s.User(t, "r_viewer")
		purNeedErr(t, Call(t, "POST", "/purchases", viewer, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "items": []M{purItem(pid, s.MS, 1, 3, 0)}}), 403, "", "viewer creates")
		if len(List(t, viewer, "purchases", "storeId="+s.ID+"&select=id")) == 0 {
			t.Error("viewer cannot list purchases")
		}
	})
}

// TestPurchases_Returns: partial, full, over-return, stock out, vendor
// balance, refunds, edits, delete / restore and stats.
func TestPurchases_Returns(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 10, 20, 5)
	pid := S(p["id"])
	v := s.Vendor(t)
	vid := S(v["id"])
	// purchase 10 × 12.50, 20 paid
	pur := Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "items": []M{purItem(pid, s.MS, 10, 12.5, 0)},
		"payments": []M{{"date": s.Now(), "amount": 20, "method": "cash"}}})
	purID := S(pur["id"])
	_, _, purNet := purOracle([]purLine{{10, 12.5, 0}}, 0, 0, 15)
	payable := purNet - 2000
	purWaitStock(t, s, pid, s.MS, 15)
	purWaitBalance(t, s, vid, -purC(payable))
	ret := func(qty float64, pays []M) Resp {
		b := M{"storeId": s.ID, "date": s.Now(), "purchaseId": purID, "vendorId": vid, "items": []M{purItem(pid, s.MS, qty, 12.5, 0)}}
		if pays != nil {
			b["payments"] = pays
		}
		return Call(t, "POST", "/purchase-returns", s.Token, b)
	}
	var retNets []int64
	var retPaid int64

	t.Run("more than purchased in one return is refused", func(t *testing.T) {
		r := ret(11, nil)
		if r.Code != 400 || !strings.Contains(r.Raw, "should not exceed Original Purchase Net Total") {
			t.Errorf("over return: %s", r)
		}
	})

	var prID string
	t.Run("partial return with a refund from the vendor", func(t *testing.T) {
		r := Must(t, ret(4, []M{{"date": s.Now(), "amount": 10, "method": "cash"}}), 201, "partial return").Body
		prID = S(r["id"])
		_, _, net := purOracle([]purLine{{4, 12.5, 0}}, 0, 0, 15)
		retNets = append(retNets, net)
		retPaid += 1000
		EqMoney(t, "return net", F(r, "legacyTotals.net"), purC(net))
		if S(r["purchaseCode"]) != S(pur["code"]) || !strings.HasPrefix(S(r["code"]), "PR-INV-") {
			t.Errorf("return links: %v %v", r["purchaseCode"], r["code"])
		}
		if S(Get(r, "legacyTotals.paymentStatus")) != "paid_partially" {
			t.Errorf("return payment status: %v", Get(r, "legacyTotals.paymentStatus"))
		}
		purWaitStock(t, s, pid, s.MS, 11)
		// payable drops by the return and rises by the refund received
		payable = payable - net + 1000
		purWaitBalance(t, s, vid, -purC(payable))
		if q := F(Read(t, s.Token, "purchases", purID), "items.0.qtyReturned"); q != 4 {
			t.Errorf("purchase qtyReturned: %v", q)
		}
	})

	t.Run("refund larger than what was paid is refused", func(t *testing.T) {
		// 20 paid, 10 already refunded: 11 is too much
		r := ret(1, []M{{"date": s.Now(), "amount": 11, "method": "cash"}})
		purNeedErr(t, r, 400, "payments", "refund above paid")
		purNeedErr(t, ret(1, []M{{"date": s.Now(), "amount": 1, "method": "gold"}}), 400, "payments.0.method", "refund bad method")
	})

	t.Run("cumulative returns above the purchased quantity", func(t *testing.T) {
		// 4 returned, 6 left: returning 7 must be refused
		r := ret(7, nil)
		KnownBug(t, "NEW-PR-OVERRETURN", "a second purchase return can take the returned quantity above the purchased quantity (7 of 6 left accepted)", r.Code == 201)
		if r.Code == 201 {
			_, _, net := purOracle([]purLine{{7, 12.5, 0}}, 0, 0, 15)
			retNets = append(retNets, net)
			payable -= net
			purWaitStock(t, s, pid, s.MS, 4)
			purWaitBalance(t, s, vid, -purC(payable))
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		purNeedErr(t, Call(t, "POST", "/purchase-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 1, 12.5, 0)}}), 400, "purchaseId", "no purchase")
		purNeedErr(t, Call(t, "POST", "/purchase-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "purchaseId": purID, "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 0, 12.5, 0)}}), 400, "items.0.qty", "zero qty")
		purNeedErr(t, Call(t, "POST", "/purchase-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "purchaseId": "pur_nope", "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 1, 12.5, 0)}}), 400, "purchaseId", "unknown purchase")
		purNeedErr(t, Call(t, "POST", "/purchase-returns", s.Token, M{"storeId": s.ID, "purchaseId": purID, "vendorId": vid,
			"items": []M{purItem(pid, s.MS, 1, 12.5, 0)}}), 400, "date", "no date")
	})

	t.Run("PATCH and PUT a return", func(t *testing.T) {
		before := purStockIn(t, s, pid, s.MS)
		r := Patch(t, s.Token, "purchase-returns", prID, M{"items": []M{purItem(pid, s.MS, 3, 12.5, 0)}, "payments": Read(t, s.Token, "purchase-returns", prID)["payments"]})
		_, _, net := purOracle([]purLine{{3, 12.5, 0}}, 0, 0, 15)
		EqMoney(t, "edited return net", F(r, "legacyTotals.net"), purC(net))
		purWaitStock(t, s, pid, s.MS, before+1)
		payable += retNets[0] - net
		retNets[0] = net
		purWaitBalance(t, s, vid, -purC(payable))
		pu := Must(t, Call(t, "PUT", "/purchase-returns/"+prID, s.Token, M{"date": s.Now(), "purchaseId": purID, "vendorId": vid, "remarks": "مرتجع",
			"items": []M{purItem(pid, s.MS, 3, 12.5, 0)}, "payments": []M{{"date": s.Now(), "amount": 10, "method": "cash"}}}), 200, "put return").Body
		if S(pu["remarks"]) != "مرتجع" {
			t.Errorf("put return remarks: %v", pu["remarks"])
		}
		purNeedErr(t, Call(t, "PATCH", "/purchase-returns/"+prID, s.Token, M{"remarks": "x"}, "If-Match", "999"), 409, "", "stale return")
		purNeedErr(t, Call(t, "PUT", "/purchase-returns/"+prID, s.Token, M{"date": s.Now(), "purchaseId": purID, "vendorId": vid, "items": []M{}}), 400, "items", "put without lines")
	})

	t.Run("DELETE is unsupported (legacy delete is a no-op); restore", func(t *testing.T) {
		before := purStockIn(t, s, pid, s.MS)
		r := Call(t, "DELETE", "/purchase-returns/"+prID, s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("delete return: %s", r)
		}
		time.Sleep(300 * time.Millisecond)
		if got := purStockIn(t, s, pid, s.MS); got != before {
			t.Errorf("a refused delete moved stock: %v → %v", before, got)
		}
		Must(t, Call(t, "POST", "/purchase-returns/"+prID+"/restore", s.Token, nil), 200, "restore live return")
		purNeedErr(t, Call(t, "POST", "/purchase-returns/000000000000000000000001/restore", s.Token, nil), 404, "", "restore unknown")
	})

	t.Run("stats match the oracle", func(t *testing.T) {
		var net int64
		for _, n := range retNets {
			net += n
		}
		r := Must(t, Call(t, "GET", "/purchase-returns/stats?storeId="+s.ID+"&sum=net,paid,one", s.Token, nil), 200, "return stats").Body
		if int(F(r, "count")) != len(retNets) {
			t.Errorf("return count %v, want %d", r["count"], len(retNets))
		}
		EqMoney(t, "returns net", F(r, "sums.net"), purC(net))
		EqMoney(t, "returns refunds", F(r, "sums.paid"), purC(retPaid))
		purNeedErr(t, Call(t, "GET", "/purchase-returns/stats?storeId="+s.ID+"&to=bad", s.Token, nil), 400, "to", "bad to")
		rows := List(t, s.Token, "purchase-returns", "storeId="+s.ID+"&where.vendorId="+vid+"&select=id,purchaseId")
		if len(rows) != len(retNets) {
			t.Errorf("returns by vendor: %v", rows)
		}
	})
}

// TestPurchases_OrdersRequestsBills: purchase orders, purchase requests
// (status decisions) and the purchase bills inbox, with their links.
func TestPurchases_OrdersRequestsBills(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	p := s.Product(t, 10, 20, 0)
	pid := S(p["id"])
	v := s.Vendor(t)
	vid := S(v["id"])

	var poID string
	t.Run("purchase order lifecycle", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchase-orders", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "expectedDate": s.Today(),
			"status": "draft", "discount": 2, "items": []M{purItem(pid, s.MS, 3, 9, 0)}}), 201, "po").Body
		poID = S(r["id"])
		_, _, net := purOracle([]purLine{{3, 9, 0}}, 2, 0, 15)
		EqMoney(t, "po net", F(r, "legacyTotals.net"), purC(net))
		if !strings.HasPrefix(S(r["code"]), "PO-") || S(r["status"]) != "draft" || S(r["expectedDate"]) != s.Today() {
			t.Errorf("po: %v %v %v", r["code"], r["status"], r["expectedDate"])
		}
		// an order does not move stock
		time.Sleep(300 * time.Millisecond)
		if s.Stock(t, pid) != 0 {
			t.Error("a purchase order moved stock")
		}
		purNeedErr(t, Call(t, "POST", "/purchase-orders", s.Token, M{"storeId": s.ID, "date": s.Now(), "status": "sent", "items": []M{purItem(pid, s.MS, 1, 9, 0)}}), 400, "vendorId", "sent without vendor")
		Must(t, Call(t, "POST", "/purchase-orders", s.Token, M{"storeId": s.ID, "date": s.Now(), "status": "draft", "items": []M{purItem(pid, s.MS, 1, 9, 0)}}), 201, "draft without vendor")
		purNeedErr(t, Call(t, "POST", "/purchase-orders", s.Token, M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "status": "sent", "items": []M{}}), 400, "items", "no lines")
		pu := Must(t, Call(t, "PUT", "/purchase-orders/"+poID, s.Token, M{"date": s.Now(), "vendorId": vid, "status": "sent", "expectedDate": s.Today(),
			"items": []M{purItem(pid, s.MS, 4, 9, 0)}}), 200, "put po").Body
		_, _, net = purOracle([]purLine{{4, 9, 0}}, 0, 0, 15)
		if S(pu["status"]) != "sent" || S(pu["code"]) != S(r["code"]) || Cents(F(pu, "legacyTotals.net")) != net {
			t.Errorf("put po: %v %v %v", pu["status"], pu["code"], pu["legacyTotals"])
		}
		purNeedErr(t, Call(t, "PUT", "/purchase-orders/"+poID, s.Token, M{"date": s.Now(), "status": "sent", "items": []M{purItem(pid, s.MS, 4, 9, 0)}}), 400, "vendorId", "put without vendor")
		st := Must(t, Call(t, "GET", "/purchase-orders/stats?storeId="+s.ID+"&sum=net,one", s.Token, nil), 200, "po stats").Body
		_, _, draftNet := purOracle([]purLine{{1, 9, 0}}, 0, 0, 15)
		if F(st, "count") != 2 || Cents(F(st, "sums.net")) != net+draftNet {
			t.Errorf("po stats: %v", st)
		}
		purNeedErr(t, Call(t, "GET", "/purchase-orders/stats?storeId="+other.ID, s.Token, nil), 403, "", "po stats other store")
	})

	t.Run("purchase order converted to a purchase", func(t *testing.T) {
		po := Read(t, s.Token, "purchase-orders", poID)
		pur := Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "poId": poID, "poCode": po["code"], "items": po["items"]})
		if S(pur["poId"]) != poID {
			t.Errorf("purchase keeps poId: %v", pur["poId"])
		}
		r := Patch(t, s.Token, "purchase-orders", poID, M{"purchaseId": pur["id"], "status": "received"})
		if S(r["purchaseId"]) != S(pur["id"]) || S(r["purchaseCode"]) != S(pur["code"]) || S(r["status"]) != "received" {
			t.Errorf("po link: %v %v %v", r["purchaseId"], r["purchaseCode"], r["status"])
		}
		purWaitStock(t, s, pid, s.MS, 4)
		purNeedErr(t, PatchResp(t, s.Token, "purchase-orders", poID, M{"purchaseId": "pur_missing"}), 400, "purchaseId", "link unknown purchase")
	})

	t.Run("purchase order soft delete, restore and hard delete", func(t *testing.T) {
		d := Must(t, Call(t, "DELETE", "/purchase-orders/"+poID, s.Token, nil), 200, "delete po").Body
		if d["deleted"] != true {
			t.Errorf("deleted flag: %v", d["deleted"])
		}
		if purIDs(List(t, s.Token, "purchase-orders", "storeId="+s.ID+"&select=id"))[poID] {
			t.Error("deleted po still listed")
		}
		if !purIDs(List(t, s.Token, "purchase-orders", "storeId="+s.ID+"&select=id&includeDeleted=1"))[poID] {
			t.Error("deleted po missing with includeDeleted")
		}
		r := Must(t, Call(t, "POST", "/purchase-orders/"+poID+"/restore", s.Token, nil), 200, "restore po").Body
		if r["deleted"] != false {
			t.Errorf("restored flag: %v", r["deleted"])
		}
		Must(t, Call(t, "DELETE", "/purchase-orders/"+poID+"?hard=1", s.Token, nil), 204, "hard delete po")
		purNeedErr(t, Call(t, "GET", "/purchase-orders/"+poID, s.Token, nil), 404, "", "po after hard delete")
		purNeedErr(t, Call(t, "POST", "/purchase-orders/"+poID+"/restore", s.Token, nil), 404, "", "restore hard-deleted po")
		purNeedErr(t, Call(t, "DELETE", "/purchase-orders/"+poID, s.Token, nil), 404, "", "delete hard-deleted po")
	})

	t.Run("purchase request status decisions", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchase-requests", s.Token, M{"storeId": s.ID, "date": s.Now(), "status": "pending", "notes": "نحتاج قطع",
			"items": []M{purItem(pid, s.MS, 3, 0, 0)}}), 201, "pr").Body
		id := S(r["id"])
		if !strings.HasPrefix(S(r["code"]), "PR-") || S(r["notes"]) != "نحتاج قطع" || S(r["assignedTo"]) == "" {
			t.Errorf("pr: %v %v %v", r["code"], r["notes"], r["assignedTo"])
		}
		for _, st := range []string{"accepted", "partially_accepted", "rejected", "pending"} {
			if g := Patch(t, s.Token, "purchase-requests", id, M{"status": st}); S(g["status"]) != st {
				t.Errorf("status %s: got %v", st, g["status"])
			}
		}
		h := Read(t, s.Token, "purchase-requests", id)["history"]
		if len(Objs(h)) != 5 {
			t.Errorf("history entries: %d", len(Objs(h)))
		}
		purNeedErr(t, Call(t, "POST", "/purchase-requests", s.Token, M{"storeId": s.ID, "date": s.Now(), "status": "pending", "items": []M{}}), 400, "items", "pr no lines")
		purNeedErr(t, Call(t, "POST", "/purchase-requests", s.Token, M{"storeId": s.ID, "date": s.Now(), "assignedTo": "usr_nope",
			"items": []M{purItem(pid, s.MS, 1, 0, 0)}}), 400, "assignedTo", "pr unknown assignee")
		po := Create(t, s.Token, "purchase-orders", M{"storeId": s.ID, "date": s.Now(), "vendorId": vid, "status": "draft", "prId": id,
			"items": []M{purItem(pid, s.MS, 3, 9, 0)}})
		if S(po["prId"]) != id || S(po["prCode"]) != S(r["code"]) {
			t.Errorf("po from pr: %v %v", po["prId"], po["prCode"])
		}
		l := Patch(t, s.Token, "purchase-requests", id, M{"poId": po["id"], "status": "accepted"})
		if S(l["poId"]) != S(po["id"]) || S(l["poCode"]) != S(po["code"]) {
			t.Errorf("pr → po link: %v %v", l["poId"], l["poCode"])
		}
		// PUT replaces the record, so assignedTo is required again (create defaults it)
		np := Call(t, "PUT", "/purchase-requests/"+id, s.Token, M{"date": s.Now(), "status": "rejected", "items": []M{purItem(pid, s.MS, 2, 0, 0)}})
		if np.Code != 400 {
			t.Errorf("put pr without assignee: %s", np)
		}
		KnownBug(t, "NEW-PR-ASSIGNED-FIELD", "a missing assignedTo is reported under the legacy field assigned_to", np.ErrField("assignedTo") == "")
		Must(t, Call(t, "PUT", "/purchase-requests/"+id, s.Token, M{"date": s.Now(), "status": "rejected", "assignedTo": r["assignedTo"],
			"items": []M{purItem(pid, s.MS, 2, 0, 0)}}), 200, "put pr")
		Must(t, Call(t, "DELETE", "/purchase-requests/"+id, s.Token, nil), 200, "delete pr")
		Must(t, Call(t, "POST", "/purchase-requests/"+id+"/restore", s.Token, nil), 200, "restore pr")
		Must(t, Call(t, "DELETE", "/purchase-requests/"+id+"?hard=1", s.Token, nil), 204, "hard delete pr")
		purNeedErr(t, Call(t, "GET", "/purchase-requests/"+id, s.Token, nil), 404, "", "pr after hard delete")
		st := Must(t, Call(t, "GET", "/purchase-requests/stats?storeId="+s.ID+"&sum=one", s.Token, nil), 200, "pr stats").Body
		if F(st, "count") != 0 {
			t.Errorf("pr stats after hard delete: %v", st)
		}
		purNeedErr(t, Call(t, "GET", "/purchase-requests/stats?storeId="+s.ID+"&from=x", s.Token, nil), 400, "from", "pr stats bad from")
	})

	t.Run("purchase bills inbox", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/purchase-bills", s.Token, M{"storeId": s.ID, "vendorId": vid, "receivedAt": s.Now(), "fileName": "فاتورة.pdf",
			"amount": 115, "vat": 15, "status": "new", "vendorInvoiceNo": "B-1"}), 201, "bill").Body
		id := S(r["id"])
		if !strings.HasPrefix(id, "pbl_") || !strings.Contains(S(r["code"]), "BILL-") || S(r["fileName"]) != "فاتورة.pdf" {
			t.Errorf("bill: %v %v %v", id, r["code"], r["fileName"])
		}
		Create(t, s.Token, "purchase-bills", M{"storeId": s.ID, "vendorId": vid, "receivedAt": s.Now(), "fileName": "b2.pdf", "amount": 57.5, "status": "converted"})
		bad := Call(t, "POST", "/purchase-bills", s.Token, M{"storeId": s.ID, "amount": 0})
		for _, f := range []string{"vendorId", "amount", "receivedAt", "fileName"} {
			purNeedErr(t, bad, 400, f, "bill validation")
		}
		purNeedErr(t, Call(t, "POST", "/purchase-bills", s.Token, M{"storeId": s.ID, "vendorId": "ven_x", "receivedAt": s.Now(), "fileName": "b", "amount": 1}), 400, "vendorId", "bill unknown vendor")
		ov := other.Vendor(t)
		x := Call(t, "POST", "/purchase-bills", s.Token, M{"storeId": s.ID, "vendorId": ov["id"], "receivedAt": s.Now(), "fileName": "b", "amount": 1})
		KnownBug(t, "NEW-BILL-VENDOR", "a purchase bill accepts the vendor id of another company (any 24-hex id passes)", x.Code == 201)
		if x.Code == 201 {
			Must(t, Call(t, "DELETE", "/purchase-bills/"+S(x.Body["id"])+"?hard=1", s.Token, nil), 204, "remove stray bill")
		}
		nw := List(t, s.Token, "purchase-bills", "storeId="+s.ID+"&where.status=new&select=id,status")
		if len(nw) != 1 || S(nw[0]["id"]) != id {
			t.Errorf("new bills: %v", nw)
		}
		st := Must(t, Call(t, "GET", "/purchase-bills/stats?storeId="+s.ID+"&sum=amount,one", s.Token, nil), 200, "bill stats").Body
		if F(st, "count") != 2 || Cents(F(st, "sums.amount")) != 17250 {
			t.Errorf("bill stats: %v", st)
		}
		u := Patch(t, s.Token, "purchase-bills", id, M{"status": "converted"})
		if S(u["status"]) != "converted" || S(u["code"]) != S(r["code"]) {
			t.Errorf("patch bill: %v %v", u["status"], u["code"])
		}
		purNeedErr(t, PatchResp(t, s.Token, "purchase-bills", id, M{"amount": -1}), 400, "amount", "bill negative amount")
		pu := Must(t, Call(t, "PUT", "/purchase-bills/"+id, s.Token, M{"vendorId": vid, "receivedAt": s.Now(), "fileName": "x.pdf", "amount": 115, "status": "new"}), 200, "put bill").Body
		KnownBug(t, "NEW-NATIVE-PUT-RENUMBER", "PUT without code gives a numbered record (bill, pending transfer) a new number", S(pu["code"]) != S(r["code"]))
		purNeedErr(t, Call(t, "PUT", "/purchase-bills/"+id, s.Token, M{"vendorId": vid, "fileName": "x.pdf", "amount": 115}), 400, "receivedAt", "put bill without receivedAt")
		d := Must(t, Call(t, "DELETE", "/purchase-bills/"+id, s.Token, nil), 200, "delete bill").Body
		if d["deleted"] != true {
			t.Errorf("bill deleted: %v", d["deleted"])
		}
		if r := Must(t, Call(t, "POST", "/purchase-bills/"+id+"/restore", s.Token, nil), 200, "restore bill").Body; r["deleted"] != false {
			t.Errorf("bill restored: %v", r["deleted"])
		}
		purNeedErr(t, Call(t, "POST", "/purchase-bills/pbl_000/restore", s.Token, nil), 404, "", "restore unknown bill")
		purNeedErr(t, Call(t, "GET", "/purchase-bills/"+id, other.Token, nil), 404, "", "bill from another company")
		purNeedErr(t, Call(t, "GET", "/purchase-bills/stats?storeId="+s.ID+"&from=2026-02-01&to=2026-01-01", s.Token, nil), 400, "from", "bill stats range")
	})
}

// ---------- RFQs, suppliers, threads, notifications ----------

// TestPurchases_RFQsThreadsNotifications: RFQ intake and its suppliers, the
// messaging threads and the notification feed.
func TestPurchases_RFQsThreadsNotifications(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")

	t.Run("rfq suppliers", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/rfq-suppliers", s.Token, M{"storeId": s.ID, "name": "مورد قطع " + Uniq(), "phone": "0512345678",
			"categories": []string{"filters", "oil"}, "rating": 4, "lat": 24.7, "lng": 46.6, "notes": "fast"}), 201, "supplier").Body
		id := S(r["id"])
		if len(Objs(r["categories"])) != 0 || len(r["categories"].([]interface{})) != 2 || F(r, "lat") != 24.7 || S(r["notes"]) != "fast" {
			t.Errorf("supplier: %v", r)
		}
		purNeedErr(t, Call(t, "POST", "/rfq-suppliers", s.Token, M{"storeId": s.ID, "name": "No cat"}), 400, "categories", "supplier without categories")
		if g := Patch(t, s.Token, "rfq-suppliers", id, M{"rating": 5}); F(g, "rating") != 5 {
			t.Errorf("rating: %v", g["rating"])
		}
		Must(t, Call(t, "PUT", "/rfq-suppliers/"+id, s.Token, M{"name": "Renamed", "categories": []string{"tyres"}}), 200, "put supplier")
		purNeedErr(t, Call(t, "PUT", "/rfq-suppliers/"+id, s.Token, M{"name": "Renamed", "categories": []string{}}), 400, "categories", "put supplier without categories")
		if Must(t, Call(t, "DELETE", "/rfq-suppliers/"+id, s.Token, nil), 200, "delete supplier").Body["deleted"] != true {
			t.Error("supplier not deleted")
		}
		if purIDs(List(t, s.Token, "rfq-suppliers", "storeId="+s.ID+"&select=id"))[id] {
			t.Error("deleted supplier listed")
		}
		Must(t, Call(t, "POST", "/rfq-suppliers/"+id+"/restore", s.Token, nil), 200, "restore supplier")
		if !purIDs(List(t, s.Token, "rfq-suppliers", "storeId="+s.ID+"&q=Renamed&select=id"))[id] {
			t.Error("restored supplier not found by q")
		}
		purNeedErr(t, Call(t, "GET", "/rfq-suppliers/"+id, other.Token, nil), 404, "", "supplier of another company")
		Must(t, Call(t, "DELETE", "/rfq-suppliers/"+id+"?hard=1", s.Token, nil), 204, "hard delete supplier")
		purNeedErr(t, Call(t, "POST", "/rfq-suppliers/"+id+"/restore", s.Token, nil), 404, "", "restore hard-deleted supplier")
	})

	t.Run("rfqs", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/rfqs", s.Token, M{"storeId": s.ID, "customerName": "عميل RFQ", "source": "manual", "receivedAt": s.Now(),
			"message": "need filters", "items": []M{{"name": "Oil filter", "qty": 3, "unit": "pcs"}, {"name": "فلتر هواء", "qty": 1, "unit": "pcs"}}}), 201, "rfq").Body
		id := S(r["id"])
		items := Objs(r["items"])
		if len(items) != 2 || F(items[0], "qty") != 3 || S(items[1]["name"]) != "فلتر هواء" || S(r["status"]) == "" {
			t.Errorf("rfq: %v", r)
		}
		purNeedErr(t, Call(t, "POST", "/rfqs", s.Token, M{"storeId": s.ID}), 400, "", "empty rfq")
		if g := Patch(t, s.Token, "rfqs", id, M{"status": "processing", "markup": 12.5}); S(g["status"]) != "processing" || F(g, "markup") != 12.5 {
			t.Errorf("rfq patch: %v %v", g["status"], g["markup"])
		}
		Must(t, Call(t, "PUT", "/rfqs/"+id, s.Token, M{"customerName": "RFQ put", "message": "x", "items": []M{{"name": "Brake pad", "qty": 2}}}), 200, "put rfq")
		purNeedErr(t, Call(t, "PATCH", "/rfqs/"+id, s.Token, M{"markup": 1}, "If-Match", "1"), 409, "", "stale rfq")
		Create(t, s.Token, "rfqs", M{"storeId": s.ID, "customerName": "Second", "receivedAt": s.Now(), "message": "m", "items": []M{{"name": "Belt", "qty": 1}}})
		st := Must(t, Call(t, "GET", "/rfqs/stats?storeId="+s.ID+"&sum=one", s.Token, nil), 200, "rfq stats").Body
		if F(st, "count") != 2 {
			t.Errorf("rfq stats: %v", st)
		}
		purNeedErr(t, Call(t, "GET", "/rfqs/stats?storeId="+s.ID+"&from=nope", s.Token, nil), 400, "from", "rfq stats bad from")
		Must(t, Call(t, "DELETE", "/rfqs/"+id, s.Token, nil), 200, "delete rfq")
		if F(Must(t, Call(t, "GET", "/rfqs/stats?storeId="+s.ID+"&sum=one", s.Token, nil), 200, "rfq stats").Body, "count") != 1 {
			t.Error("deleted rfq still counted")
		}
		Must(t, Call(t, "POST", "/rfqs/"+id+"/restore", s.Token, nil), 200, "restore rfq")
		Must(t, Call(t, "DELETE", "/rfqs/"+id+"?hard=1", s.Token, nil), 204, "hard delete rfq")
		purNeedErr(t, Call(t, "GET", "/rfqs/"+id, s.Token, nil), 404, "", "rfq after hard delete")
		purNeedErr(t, Call(t, "GET", "/rfqs?storeId="+other.ID, s.Token, nil), 403, "", "rfqs of another store")
	})

	t.Run("threads", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/threads", s.Token, M{"storeId": s.ID, "name": "مورد", "channel": "whatsapp",
			"messages": []M{{"id": "m1", "dir": "out", "text": "السلام عليكم", "at": s.Now()}}}), 201, "thread").Body
		id := S(r["id"])
		if !strings.HasPrefix(id, "thr_") || S(Get(r, "messages.0.text")) != "السلام عليكم" {
			t.Errorf("thread: %v", r)
		}
		g := Patch(t, s.Token, "threads", id, M{"pinned": true, "unread": 2})
		if g["pinned"] != true || F(g, "unread") != 2 {
			t.Errorf("thread patch: %v", g)
		}
		purNeedErr(t, Call(t, "PATCH", "/threads/"+id, s.Token, M{"pinned": false}, "If-Match", "99"), 409, "", "stale thread")
		pu := Must(t, Call(t, "PUT", "/threads/"+id, s.Token, M{"name": "Renamed", "channel": "email", "messages": []M{}}), 200, "put thread").Body
		if S(pu["channel"]) != "email" || pu["pinned"] != nil {
			t.Errorf("put thread replaces: %v", pu)
		}
		purNeedErr(t, Call(t, "POST", "/threads", s.Token, M{"name": "x"}), 400, "storeId", "thread without store")
		Must(t, Call(t, "DELETE", "/threads/"+id, s.Token, nil), 200, "delete thread")
		if purIDs(List(t, s.Token, "threads", "storeId="+s.ID+"&select=id"))[id] {
			t.Error("deleted thread listed")
		}
		Must(t, Call(t, "POST", "/threads/"+id+"/restore", s.Token, nil), 200, "restore thread")
		purNeedErr(t, Call(t, "GET", "/threads/"+id, other.Token, nil), 404, "", "thread of another company")
		purNeedErr(t, Call(t, "POST", "/threads/thr_missing/restore", s.Token, nil), 404, "", "restore unknown thread")
		// no date field → no /stats route
		purNeedErr(t, Call(t, "GET", "/threads/stats?storeId="+s.ID, s.Token, nil), 404, "", "threads have no stats")
	})

	t.Run("notifications", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/notifications", s.Token, M{"storeId": s.ID, "type": "stock", "tone": "warning", "titleEn": "Low stock",
			"titleAr": "مخزون منخفض", "at": s.Now(), "read": false, "link": "/products"}), 201, "notification").Body
		id := S(r["id"])
		if !strings.HasPrefix(id, "not_") || S(r["titleAr"]) != "مخزون منخفض" {
			t.Errorf("notification: %v", r)
		}
		if g := Patch(t, s.Token, "notifications", id, M{"read": true}); g["read"] != true {
			t.Errorf("mark read: %v", g["read"])
		}
		Must(t, Call(t, "PUT", "/notifications/"+id, s.Token, M{"type": "stock", "titleEn": "Again", "read": false}), 200, "put notification")
		purNeedErr(t, Call(t, "POST", "/notifications", s.Token, M{"type": "stock"}), 400, "storeId", "notification without store")
		purNeedErr(t, Call(t, "POST", "/notifications", s.Token, M{"storeId": other.ID, "type": "stock"}), 403, "", "notification for another store")
		Must(t, Call(t, "DELETE", "/notifications/"+id, s.Token, nil), 200, "delete notification")
		Must(t, Call(t, "POST", "/notifications/"+id+"/restore", s.Token, nil), 200, "restore notification")
		Must(t, Call(t, "DELETE", "/notifications/"+id+"?hard=1", s.Token, nil), 204, "hard delete notification")
		purNeedErr(t, Call(t, "DELETE", "/notifications/"+id, s.Token, nil), 404, "", "delete hard-deleted notification")
		purNeedErr(t, Call(t, "PATCH", "/notifications/"+id, s.Token, M{"read": true}), 404, "", "patch hard-deleted notification")
		purNeedErr(t, Call(t, "PUT", "/notifications/"+id, s.Token, M{"read": true}), 404, "", "put hard-deleted notification")
	})
}

// ---------- inventory ----------

// TestInventory_StockTransfers: transfers between the main store and a
// second warehouse; pending → completed moves stock only when completed.
func TestInventory_StockTransfers(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	p := s.Product(t, 10, 20, 10)
	pid := S(p["id"])
	wh := Create(t, s.Token, "warehouses", M{"storeId": s.ID, "nameEn": "Branch WH", "nameAr": "مستودع الفرع", "code": "BR"})
	whID := S(wh["id"])
	tr := func(from, to string, qty float64, status string) M {
		b := M{"storeId": s.ID, "date": s.Now(), "fromWarehouseId": from, "toWarehouseId": to, "vatPercent": 15,
			"items": []M{{"productId": pid, "qty": qty, "unitPrice": 10}}}
		if status != "" {
			b["status"] = status
		}
		return b
	}

	var doneID string
	t.Run("completed transfer moves stock", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/stock-transfers", s.Token, tr(s.MS, whID, 3, "")), 201, "transfer").Body
		doneID = S(r["id"])
		if S(r["status"]) != "completed" || !strings.HasPrefix(S(r["code"]), "ST-TR-") || S(r["fromWarehouseId"]) != s.MS || S(r["toWarehouseId"]) != whID {
			t.Errorf("transfer: %v", r)
		}
		purWaitStock(t, s, pid, s.MS, 7)
		purWaitStock(t, s, pid, whID, 3)
	})

	t.Run("pending transfer moves nothing until completed", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/stock-transfers", s.Token, tr(whID, s.MS, 2, "pending")), 201, "pending").Body
		id := S(r["id"])
		if !strings.HasPrefix(id, "sto_") || S(r["status"]) != "pending" {
			t.Errorf("pending: %v", r)
		}
		time.Sleep(500 * time.Millisecond)
		if purStockIn(t, s, pid, whID) != 3 || purStockIn(t, s, pid, s.MS) != 7 {
			t.Error("a pending transfer moved stock")
		}
		if !purIDs(List(t, s.Token, "stock-transfers", "storeId="+s.ID+"&select=id"))[id] {
			t.Error("pending transfer not listed")
		}
		// edit while pending
		g := Patch(t, s.Token, "stock-transfers", id, M{"remarks": "تحويل", "items": []M{{"productId": pid, "qty": 1, "unitPrice": 10}}})
		if S(g["remarks"]) != "تحويل" || F(g, "items.0.qty") != 1 {
			t.Errorf("pending edit: %v", g)
		}
		purNeedErr(t, PatchResp(t, s.Token, "stock-transfers", id, M{"items": []M{{"productId": pid, "qty": 0}}}), 400, "items.0.qty", "pending zero qty")
		done := Patch(t, s.Token, "stock-transfers", id, M{"status": "completed"})
		if S(done["status"]) != "completed" || S(done["replacedId"]) != id || S(done["id"]) == id {
			t.Errorf("completion: %v", done)
		}
		purWaitStock(t, s, pid, whID, 2)
		purWaitStock(t, s, pid, s.MS, 8)
		purNeedErr(t, Call(t, "GET", "/stock-transfers/"+id, s.Token, nil), 404, "", "pending record after completion")
		re := Call(t, "PATCH", "/stock-transfers/"+S(done["id"]), s.Token, M{"status": "pending"})
		if re.Code != 409 || re.ErrCode() != "unsupported_legacy" {
			t.Errorf("reopen completed: %s", re)
		}
	})

	t.Run("pending delete, restore and PUT", func(t *testing.T) {
		r := Create(t, s.Token, "stock-transfers", tr(s.MS, whID, 1, "pending"))
		id := S(r["id"])
		if d := Must(t, Call(t, "DELETE", "/stock-transfers/"+id, s.Token, nil), 200, "delete pending").Body; d["deleted"] != true {
			t.Errorf("deleted: %v", d["deleted"])
		}
		if purIDs(List(t, s.Token, "stock-transfers", "storeId="+s.ID+"&select=id"))[id] {
			t.Error("deleted pending listed")
		}
		if r := Must(t, Call(t, "POST", "/stock-transfers/"+id+"/restore", s.Token, nil), 200, "restore pending").Body; r["deleted"] != false {
			t.Errorf("restored: %v", r["deleted"])
		}
		body := tr(s.MS, whID, 4, "pending")
		delete(body, "storeId")
		pu := Must(t, Call(t, "PUT", "/stock-transfers/"+id, s.Token, body), 200, "put pending").Body
		if F(pu, "items.0.qty") != 4 {
			t.Errorf("put pending qty: %v", pu["items"])
		}
		KnownBug(t, "NEW-NATIVE-PUT-RENUMBER", "PUT without code gives a pending transfer a new number", S(pu["code"]) != S(r["code"]))
		bad := tr(s.MS, s.MS, 4, "pending")
		delete(bad, "storeId")
		purNeedErr(t, Call(t, "PUT", "/stock-transfers/"+id, s.Token, bad), 400, "toWarehouseId", "put same warehouses")
		Must(t, Call(t, "DELETE", "/stock-transfers/"+id+"?hard=1", s.Token, nil), 204, "hard delete pending")
		purNeedErr(t, Call(t, "GET", "/stock-transfers/"+id, s.Token, nil), 404, "", "pending after hard delete")
	})

	t.Run("completed transfer: edit re-adjusts stock, delete unsupported", func(t *testing.T) {
		g := Patch(t, s.Token, "stock-transfers", doneID, M{"items": []M{{"productId": pid, "qty": 5, "unitPrice": 10}}, "remarks": "x"})
		if F(g, "items.0.qty") != 5 {
			t.Errorf("edit completed: %v", g["items"])
		}
		purWaitStock(t, s, pid, s.MS, 6)
		purWaitStock(t, s, pid, whID, 4)
		d := Call(t, "DELETE", "/stock-transfers/"+doneID, s.Token, nil)
		if d.Code != 409 || d.ErrCode() != "unsupported_legacy" {
			t.Errorf("delete completed: %s", d)
		}
		Must(t, Call(t, "POST", "/stock-transfers/"+doneID+"/restore", s.Token, nil), 200, "restore live completed")
		body := tr(s.MS, whID, 5, "completed")
		delete(body, "storeId")
		body["remarks"] = "put"
		Must(t, Call(t, "PUT", "/stock-transfers/"+doneID, s.Token, body), 200, "put completed")
		purNeedErr(t, Call(t, "PATCH", "/stock-transfers/"+doneID, s.Token, M{"remarks": "y"}, "If-Match", "1"), 409, "", "stale transfer")
	})

	t.Run("validation and quantity above stock", func(t *testing.T) {
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, tr(whID, whID, 1, "")), 400, "toWarehouseId", "same warehouse")
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, tr("nope", whID, 1, "")), 400, "fromWarehouseId", "unknown source")
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, tr(s.MS, "nope", 1, "")), 400, "toWarehouseId", "unknown target")
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, tr(s.MS, whID, 0, "")), 400, "items.0.qty", "zero qty")
		nb := tr(s.MS, whID, 1, "")
		nb["items"] = []M{}
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, nb), 400, "items", "no lines")
		up := tr(s.MS, whID, 1, "")
		up["items"] = []M{{"productId": "nope", "qty": 1}}
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, up), 400, "items.0.productId", "unknown product")
		nd := tr(s.MS, whID, 1, "")
		delete(nd, "date")
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, nd), 400, "date", "no date")
		purNeedErr(t, Call(t, "POST", "/stock-transfers", s.Token, M{"storeId": other.ID}), 403, "", "another store")
		// the legacy code does not check stock: the source goes negative
		before := purStockIn(t, s, pid, whID)
		Must(t, Call(t, "POST", "/stock-transfers", s.Token, tr(whID, s.MS, before+50, "")), 201, "transfer above stock")
		purWaitStock(t, s, pid, whID, -50)
		// without vatPercent the adapter sends 0 (the legacy panic of bug #5 is not reachable here)
		nv := tr(s.MS, whID, 1, "")
		delete(nv, "vatPercent")
		if r := Must(t, Call(t, "POST", "/stock-transfers", s.Token, nv), 201, "no vatPercent").Body; F(r, "vatPercent") != 0 {
			t.Errorf("vatPercent default: %v", r["vatPercent"])
		}
	})

	t.Run("legacy route: missing vat_percent", func(t *testing.T) {
		code, raw, err := purRaw(t, "POST", "/v1/stock-transfer?search%5Bstore_id%5D="+s.ID, s.Token, M{"store_id": s.ID,
			"date_str": time.Now().Format(time.RFC3339), "from_warehouse_code": "main_store", "to_warehouse_id": whID, "to_warehouse_code": wh["code"],
			"products": []M{{"product_id": pid, "name": "x", "quantity": 1, "unit_price": 10}}})
		KnownBug(t, "#5", "stock transfer without vat_percent panics instead of 400", err != nil || code >= 500)
		if err == nil && code < 500 && code != 400 {
			t.Errorf("legacy transfer without vat: %d %s", code, raw)
		}
	})

	t.Run("no stats route; product history shows transfers", func(t *testing.T) {
		purNeedErr(t, Call(t, "GET", "/stock-transfers/stats?storeId="+s.ID, s.Token, nil), 404, "", "stock-transfers stats")
		h := Must(t, Call(t, "GET", "/products/"+pid+"/history?kind=stockTransfers&storeId="+s.ID, s.Token, nil), 200, "history").Body
		if F(h, "total") < 3 {
			t.Errorf("transfer history rows: %v", h["total"])
		}
	})
}

// TestInventory_Warehouses: warehouse CRUD and the built-in main store.
func TestInventory_Warehouses(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	var id string
	t.Run("create, list, read", func(t *testing.T) {
		r := Must(t, Call(t, "POST", "/warehouses", s.Token, M{"storeId": s.ID, "nameEn": "North WH", "nameAr": "المستودع الشمالي",
			"code": "NORTH", "phone": "0501234567", "manager": "Ali"}), 201, "warehouse").Body
		id = S(r["id"])
		if S(r["nameAr"]) != "المستودع الشمالي" || S(r["manager"]) != "Ali" {
			t.Errorf("warehouse: %v", r)
		}
		KnownBug(t, "NEW-WH-CODE", "the warehouse code sent on create is ignored (legacy assigns WH<n>)", S(r["code"]) != "NORTH")
		rows := List(t, s.Token, "warehouses", "storeId="+s.ID)
		if len(rows) != 2 || S(rows[0]["id"]) != s.MS || rows[0]["virtual"] != true {
			t.Errorf("warehouse list: %v", rows)
		}
		ms := Read(t, s.Token, "warehouses", s.MS)
		if S(ms["code"]) != "MAIN" {
			t.Errorf("main store: %v", ms)
		}
		// every product shows the new warehouse in its stock map
		p := s.Product(t, 1, 2, 0)
		if _, ok := Read(t, s.Token, "products", S(p["id"]))["stock"].(M)[id]; !ok {
			t.Error("product stock map lacks the warehouse")
		}
	})
	t.Run("validation", func(t *testing.T) {
		bad := Call(t, "POST", "/warehouses", s.Token, M{"storeId": s.ID})
		purNeedErr(t, bad, 400, "nameEn", "no name")
		purNeedErr(t, bad, 400, "code", "no code")
		purNeedErr(t, Call(t, "POST", "/warehouses", s.Token, M{"storeId": s.ID, "nameEn": "X", "code": "main"}), 400, "code", "reserved code")
		purNeedErr(t, Call(t, "POST", "/warehouses", s.Token, M{"storeId": s.ID, "nameEn": "Phone", "code": "PH", "phone": "12"}), 400, "phone", "bad phone")
		purNeedErr(t, Call(t, "POST", "/warehouses", s.Token, M{"storeId": other.ID, "nameEn": "X", "code": "Y"}), 403, "", "another store")
		code, _, err := purRaw(t, "POST", "/v1/warehouse?search%5Bstore_id%5D="+s.ID, s.Token, M{"store_id": s.ID})
		KnownBug(t, "#9", "legacy warehouse validation errors answer HTTP 500", err == nil && code == 500)
	})
	t.Run("edit, delete, restore", func(t *testing.T) {
		g := Patch(t, s.Token, "warehouses", id, M{"nameEn": "North Renamed"})
		if S(g["nameEn"]) != "North Renamed" {
			t.Errorf("rename: %v", g["nameEn"])
		}
		purNeedErr(t, PatchResp(t, s.Token, "warehouses", id, M{"nameEn": ""}), 400, "nameEn", "blank name")
		Must(t, Call(t, "PUT", "/warehouses/"+id, s.Token, M{"nameEn": "North Put", "code": "WHX"}), 200, "put warehouse")
		for _, m := range []string{"PATCH", "DELETE"} {
			r := Call(t, m, "/warehouses/"+s.MS, s.Token, M{"nameEn": "x"})
			if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
				t.Errorf("%s main store: %s", m, r)
			}
		}
		purNeedErr(t, Call(t, "PUT", "/warehouses/"+s.MS, s.Token, M{"nameEn": "x", "code": "Q"}), 409, "", "put main store")
		d := Must(t, Call(t, "DELETE", "/warehouses/"+id, s.Token, nil), 200, "delete").Body
		if d["deleted"] != true {
			t.Errorf("deleted: %v", d["deleted"])
		}
		if len(List(t, s.Token, "warehouses", "storeId="+s.ID)) != 1 {
			t.Error("deleted warehouse listed")
		}
		r := Call(t, "POST", "/warehouses/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore warehouse: %s", r)
		}
		purNeedErr(t, Call(t, "GET", "/warehouses/"+id, other.Token, nil), 404, "", "warehouse of another company")
		purNeedErr(t, Call(t, "GET", "/warehouses/ms_000000000000000000000001", s.Token, nil), 404, "", "unknown main store")
	})
}

// TestInventory_Products: SKU/barcode rules, prices, absolute stock, sets,
// facets, search, select, pagination, delete / restore and history.
func TestInventory_Products(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	mk := func(b M) Resp { b["storeId"] = s.ID; return Call(t, "POST", "/products", s.Token, b) }
	cat := Create(t, s.Token, "categories", M{"nameEn": "Brakes " + Uniq(), "nameAr": "فرامل"})
	brand := Create(t, s.Token, "brands", M{"name": "Bosch " + Uniq()})
	var pid string

	t.Run("create: SKU, barcode, part number", func(t *testing.T) {
		r := Must(t, mk(M{"nameEn": "Brake Pad", "nameAr": "فحمات فرامل", "code": "SKU-1", "partNo": "BP-100", "barcode": "12345678",
			"categoryIds": []string{S(cat["id"])}, "brandId": brand["id"], "unit": "pcs", "pricing": M{"purchase": 10, "retail": 20, "wholesale": 15}}), 201, "product").Body
		pid = S(r["id"])
		if S(r["code"]) != "SKU-1" || S(r["barcode"]) != "12345678" || S(r["brandId"]) != S(brand["id"]) || F(r, "pricing.wholesale") != 15 {
			t.Errorf("product: %v", r)
		}
		purNeedErr(t, mk(M{"nameEn": "Other", "code": "sku-1"}), 400, "code", "SKU differs only by case")
		purNeedErr(t, mk(M{"nameEn": "Other", "partNo": "BP-100"}), 400, "partNo", "duplicate part number")
		purNeedErr(t, mk(M{"nameEn": "Other", "barcode": "12ab"}), 400, "barcode", "bad barcode")
		purNeedErr(t, mk(M{"nameEn": "Other", "barcode": "1234567"}), 400, "barcode", "7-digit barcode")
		dup := mk(M{"nameEn": "Barcode twin", "barcode": "12345678"})
		KnownBug(t, "NEW-BARCODE-DUP", "two products of a store can share one barcode", dup.Code == 201)
		auto := Must(t, mk(M{"nameEn": "Auto code"}), 201, "auto code").Body
		if !strings.Contains(S(auto["code"]), "-P-0") || S(auto["barcode"]) == "" {
			t.Errorf("auto code / barcode: %v %v", auto["code"], auto["barcode"])
		}
		// a product may be saved without prices (drafts, services priced on the invoice)
		if z := Must(t, mk(M{"nameEn": "No price yet"}), 201, "zero price").Body; F(z, "pricing.retail") != 0 {
			t.Errorf("zero price: %v", z["pricing"])
		}
		purNeedErr(t, mk(M{"nameEn": "Neg", "pricing": M{"retail": -1}}), 400, "pricing.retail", "negative retail")
		purNeedErr(t, mk(M{"nameEn": "MinMax", "pricing": M{"retail": 5, "min": 10, "max": 5}}), 400, "pricing.max", "max below min")
		purNeedErr(t, mk(M{"nameEn": ""}), 400, "nameEn", "no name")
		purNeedErr(t, mk(M{"nameEn": "AB"}), 400, "nameEn", "two-letter name (legacy min 3)")
		purNeedErr(t, mk(M{"nameEn": "Bad WH", "stock": M{"wh_nope": M{"qty": 3}}}), 400, "stock", "stock in an unknown warehouse")
		// any ms_ prefix is taken as this store's main warehouse
		fm := mk(M{"nameEn": "Foreign MS", "stock": M{"ms_000000000000000000000001": M{"qty": 3}}})
		KnownBug(t, "NEW-PROD-MS-ANY", "stock under another store's ms_ id is booked to this store's main warehouse", fm.Code == 201)
		purNeedErr(t, mk(M{"nameEn": "Bad brand", "brandId": "brd_nope"}), 400, "brandId", "unknown brand")
		uc := mk(M{"nameEn": "Bad category", "categoryIds": []string{"000000000000000000000001"}})
		if uc.Code != 400 {
			t.Fatalf("unknown category: %s", uc)
		}
		KnownBug(t, "NEW-PROD-CAT-FIELD", "an unknown categoryIds entry is reported as error.fields[adjustments.0.id]", uc.ErrField("categoryIds") == "")
		purNeedErr(t, Call(t, "POST", "/products", s.Token, M{"storeId": other.ID, "nameEn": "Foreign"}), 403, "", "another store")
	})

	t.Run("absolute stock edits and min stock", func(t *testing.T) {
		for _, c := range []struct{ qty, want float64 }{{7, 7}, {4, 4}, {-2, -2}, {2.5, 2.5}} {
			r := Patch(t, s.Token, "products", pid, M{"stock": M{s.MS: M{"qty": c.qty, "min": 3}}})
			if F(r, "stock."+s.MS+".qty") != c.want || F(r, "stock."+s.MS+".min") != 3 {
				t.Errorf("stock %v: %v", c.qty, r["stock"])
			}
		}
		if F(Read(t, s.Token, "products", pid), "legacyStockTotal") != 2.5 {
			t.Error("legacy stock total")
		}
		low := purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&where.stockStatus=low&select=id"))
		if !low[pid] {
			t.Error("2.5 with min 3 is low stock")
		}
		Patch(t, s.Token, "products", pid, M{"stock": M{s.MS: M{"qty": 0, "min": 3}}})
		if !purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&where.stockStatus=out&select=id"))[pid] {
			t.Error("0 is out of stock")
		}
		Patch(t, s.Token, "products", pid, M{"stock": M{s.MS: M{"qty": 10, "min": 3}}})
		if !purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&where.stockStatus=ok&select=id"))[pid] {
			t.Error("10 with min 3 is ok")
		}
		purNeedErr(t, Call(t, "GET", "/products?storeId="+s.ID+"&where.stockStatus=weird", s.Token, nil), 400, "", "bad stock status")
		// a service keeps no stock
		svc := Must(t, mk(M{"nameEn": "Fitting service", "isService": true, "pricing": M{"retail": 50}, "stock": M{s.MS: M{"qty": 5}}}), 201, "service").Body
		sr := Read(t, s.Token, "products", S(svc["id"]))
		if F(sr, "legacyStockTotal") != 0 {
			t.Errorf("service booked stock: %v", sr["legacyStockTotal"])
		}
		KnownBug(t, "NEW-SERVICE-STOCK", "a service echoes the stock sent on create although none is booked", len(sr["stock"].(M)) != 0)
	})

	t.Run("sets", func(t *testing.T) {
		c1 := s.Product(t, 1, 2, 10)
		set := Must(t, mk(M{"nameEn": "Brake kit", "isSet": true, "components": []M{{"productId": c1["id"], "qty": 2}, {"productId": pid, "qty": 1}},
			"pricing": M{"retail": 30}}), 201, "set").Body
		if set["isSet"] != true || len(Objs(set["components"])) != 2 || F(set, "components.0.qty") != 2 {
			t.Errorf("set: %v", set)
		}
		sets := purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&where.isSet=true&select=id"))
		if !sets[S(set["id"])] || sets[pid] {
			t.Errorf("where.isSet=true: %v", sets)
		}
		if purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&where.isSet=false&select=id"))[S(set["id"])] {
			t.Error("set listed as plain product")
		}
		r := mk(M{"nameEn": "Ghost kit", "isSet": true, "components": []M{{"productId": "000000000000000000000001", "qty": 1}}})
		KnownBug(t, "NEW-SET-COMPONENT", "a set accepts a component product id that does not exist", r.Code == 201)
		purNeedErr(t, mk(M{"nameEn": "Bad kit", "isSet": true, "components": []M{{"productId": "prd_nope", "qty": 1}}}), 400, "components", "unknown component (client id)")
	})

	t.Run("facets", func(t *testing.T) {
		f := Must(t, Call(t, "GET", "/products/facets?storeId="+s.ID, s.Token, nil), 200, "facets").Body
		var catCount, brandCount float64
		for _, c := range Objs(f["categoryId"]) {
			if S(c["id"]) == S(cat["id"]) {
				catCount = F(c, "count")
			}
		}
		for _, c := range Objs(f["brandId"]) {
			if S(c["id"]) == S(brand["id"]) {
				brandCount = F(c, "count")
			}
		}
		if catCount != 1 || brandCount != 1 {
			t.Errorf("facets: %v", f)
		}
		svc := Must(t, Call(t, "GET", "/products/facets?storeId="+s.ID+"&where.isService=true", s.Token, nil), 200, "facets services").Body
		if F(svc, "total") != 1 {
			t.Errorf("service facets total: %v", svc["total"])
		}
		purNeedErr(t, Call(t, "GET", "/products/facets", s.Token, nil), 400, "storeId", "facets without store")
		purNeedErr(t, Call(t, "GET", "/products/facets?storeId="+other.ID, s.Token, nil), 403, "", "facets of another store")
		purNeedErr(t, Call(t, "GET", "/products/facets?storeId="+s.ID+"&where.bogus=1", s.Token, nil), 400, "", "facets bad filter")
	})

	t.Run("search, select, sort and pagination", func(t *testing.T) {
		for _, q := range []string{"فرامل", "bp-100", "BP100", "sku-1", "brake"} {
			if !purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&q="+url.QueryEscape(q)+"&select=id"))[pid] {
				t.Errorf("q=%s does not find the product", q)
			}
		}
		if n := len(List(t, s.Token, "products", "storeId="+s.ID+"&q=zzqqxx&select=id")); n != 0 {
			t.Errorf("q without match: %d rows", n)
		}
		for _, n := range []string{"Pagi C", "Pagi A", "Pagi B"} {
			Must(t, mk(M{"nameEn": n}), 201, n)
		}
		r := Must(t, Call(t, "GET", "/products?storeId="+s.ID+"&where.categoryId="+S(cat["id"])+"&select=id,nameEn", s.Token, nil), 200, "by category")
		if len(r.Data()) != 1 || S(r.Data()[0]["id"]) != pid {
			t.Errorf("where.categoryId: %v", r.Body)
		}
		all := Must(t, Call(t, "GET", "/products?storeId="+s.ID+"&sort=nameEn&select=id,nameEn&limit=500", s.Token, nil), 200, "sorted")
		var pagi []string
		for _, row := range all.Data() {
			if strings.HasPrefix(S(row["nameEn"]), "Pagi ") {
				pagi = append(pagi, S(row["nameEn"]))
			}
			if _, ok := row["pricing"]; ok {
				t.Fatal("select=id,nameEn returned pricing")
			}
		}
		if strings.Join(pagi, ",") != "Pagi A,Pagi B,Pagi C" {
			t.Errorf("sort=nameEn: %v", pagi)
		}
		total := int(F(all.Body, "total"))
		p2 := Must(t, Call(t, "GET", "/products?storeId="+s.ID+"&sort=nameEn&select=id,nameEn&limit=2&page=2", s.Token, nil), 200, "page 2")
		if int(F(p2.Body, "total")) != total || len(p2.Data()) != 2 || S(p2.Data()[0]["id"]) != S(all.Data()[2]["id"]) {
			t.Errorf("page 2: %v", p2.Body)
		}
		desc := Must(t, Call(t, "GET", "/products?storeId="+s.ID+"&sort=-nameEn&select=id&limit=1", s.Token, nil), 200, "desc").Data()
		if S(desc[0]["id"]) != S(all.Data()[total-1]["id"]) {
			t.Error("sort=-nameEn")
		}
		g := Must(t, Call(t, "GET", "/products/"+pid+"?select=id,code", s.Token, nil), 200, "get select").Body
		if _, ok := g["nameEn"]; ok || S(g["code"]) != "SKU-1" {
			t.Errorf("get with select: %v", g)
		}
		purNeedErr(t, Call(t, "GET", "/products/"+pid+"?select=a$b", s.Token, nil), 400, "select", "get bad select")
		purNeedErr(t, Call(t, "GET", "/products?storeId="+s.ID+"&limit=abc", s.Token, nil), 400, "limit", "bad limit")
		purNeedErr(t, Call(t, "GET", "/products?storeId="+other.ID, s.Token, nil), 403, "", "list another store")
	})

	t.Run("edit, PUT, delete, restore", func(t *testing.T) {
		stale := Call(t, "PATCH", "/products/"+pid, s.Token, M{"nameEn": "x"}, "If-Match", "1")
		if stale.Code != 409 || stale.ErrCode() != "version_conflict" {
			t.Errorf("stale: %s", stale)
		}
		purNeedErr(t, PatchResp(t, s.Token, "products", pid, M{"barcode": "abc"}), 400, "barcode", "patch bad barcode")
		pu := Must(t, Call(t, "PUT", "/products/"+pid, s.Token, M{"nameEn": "Brake Pad 2", "code": "SKU-1", "pricing": M{"purchase": 11, "retail": 22},
			"stock": M{s.MS: M{"qty": 10}}}), 200, "put").Body
		if S(pu["nameEn"]) != "Brake Pad 2" || F(pu, "pricing.retail") != 22 {
			t.Errorf("put: %v", pu)
		}
		purNeedErr(t, Call(t, "PUT", "/products/"+pid, s.Token, M{"nameEn": ""}), 400, "nameEn", "put without name")
		purNeedErr(t, Call(t, "PATCH", "/products/"+pid, other.Token, M{"nameEn": "hack"}), 404, "", "patch from another company")
		d := Must(t, Call(t, "DELETE", "/products/"+pid, s.Token, nil), 200, "delete").Body
		if d["deleted"] != true {
			t.Errorf("deleted: %v", d["deleted"])
		}
		if purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&select=id"))[pid] {
			t.Error("deleted product listed")
		}
		if !purIDs(List(t, s.Token, "products", "storeId="+s.ID+"&select=id&includeDeleted=1"))[pid] {
			t.Error("deleted product missing with includeDeleted")
		}
		r := Must(t, Call(t, "POST", "/products/"+pid+"/restore", s.Token, nil), 200, "restore").Body
		if r["deleted"] != false {
			t.Errorf("restored: %v", r["deleted"])
		}
		purNeedErr(t, Call(t, "POST", "/products/000000000000000000000001/restore", s.Token, nil), 404, "", "restore unknown")
		purNeedErr(t, Call(t, "DELETE", "/products/zzz", s.Token, nil), 404, "", "delete bad id")
		purNeedErr(t, Call(t, "GET", "/products/"+pid, other.Token, nil), 404, "", "read from another company")
	})

	t.Run("history: purchase → sale → return with running stock", func(t *testing.T) {
		hp := Must(t, mk(M{"nameEn": "History item", "pricing": M{"purchase": 10, "retail": 20}}), 201, "history product").Body
		hid := S(hp["id"])
		v := s.Vendor(t)
		c := s.Customer(t, "")
		Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": purAgo(s, 3*time.Hour), "vendorId": v["id"], "items": []M{purItem(hid, s.MS, 10, 12, 0)}})
		sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": purAgo(s, 2*time.Hour), "customerId": c["id"], "items": []M{purItem(hid, s.MS, 3, 20, 0)},
			"payments": []M{{"date": purAgo(s, 2*time.Hour), "amount": 69, "method": "cash"}}})
		Create(t, s.Token, "sales-returns", M{"storeId": s.ID, "date": purAgo(s, time.Hour), "orderId": sale["id"], "customerId": c["id"],
			"items": []M{purItem(hid, s.MS, 1, 20, 0)}})
		purWaitStock(t, s, hid, s.MS, 8)
		var h M
		Eventually(t, "history rows", func() bool {
			h = Must(t, Call(t, "GET", "/products/"+hid+"/history?kind=all&storeId="+s.ID, s.Token, nil), 200, "history").Body
			return len(Objs(h["data"])) == 3
		})
		want := []struct {
			kind          string
			change, stock float64
		}{{"salesReturns", 1, 8}, {"sales", -3, 7}, {"purchases", 10, 10}}
		for i, row := range Objs(h["data"]) {
			w := want[i]
			if S(row["kind"]) != w.kind || F(row, "change") != w.change || F(row, "stock") != w.stock || F(row, "warehouseStocks."+s.MS) != w.stock {
				t.Errorf("history row %d: %v %v %v %v, want %+v", i, row["kind"], row["change"], row["stock"], row["warehouseStocks"], w)
			}
		}
		if F(h, "sums.in") != 11 || F(h, "sums.out") != 3 || F(h, "sums.qty") != 8 {
			t.Errorf("history sums: %v", h["sums"])
		}
		pk := Must(t, Call(t, "GET", "/products/"+hid+"/history?kind=purchases&storeId="+s.ID, s.Token, nil), 200, "purchase history").Body
		if F(pk, "total") != 1 || F(pk, "sums.qty") != 10 || Cents(F(pk, "sums.value")) != 12000 {
			t.Errorf("purchase history: %v", pk["sums"])
		}
		purNeedErr(t, Call(t, "GET", "/products/"+hid+"/history?kind=bogus&storeId="+s.ID, s.Token, nil), 400, "kind", "bad kind")
		purNeedErr(t, Call(t, "GET", "/products/"+hid+"/history?kind=sales", s.Token, nil), 400, "storeId", "history without store")
		purNeedErr(t, Call(t, "GET", "/products/zzz/history?kind=sales&storeId="+s.ID, s.Token, nil), 404, "", "history bad id")
	})

	t.Run("history: same-minute documents and opening stock", func(t *testing.T) {
		v := s.Vendor(t)
		c := s.Customer(t, "")
		a := Must(t, mk(M{"nameEn": "Same minute item", "pricing": M{"purchase": 10, "retail": 20}}), 201, "product").Body
		at := s.Now()
		Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": at, "vendorId": v["id"], "items": []M{purItem(S(a["id"]), s.MS, 10, 12, 0)}})
		Create(t, s.Token, "sales", M{"storeId": s.ID, "date": at, "customerId": c["id"], "items": []M{purItem(S(a["id"]), s.MS, 3, 20, 0)},
			"payments": []M{{"date": at, "amount": 69, "method": "cash"}}})
		purWaitStock(t, s, S(a["id"]), s.MS, 7)
		var rows []M
		Eventually(t, "rows", func() bool {
			rows = Must(t, Call(t, "GET", "/products/"+S(a["id"])+"/history?kind=all&storeId="+s.ID, s.Token, nil), 200, "history").Data()
			return len(rows) == 2
		})
		for _, r := range rows {
			if S(r["kind"]) == "sales" {
				KnownBug(t, "NEW-HIST-SAME-MINUTE", "a sale in the same minute as a purchase shows stock -3 instead of 7 (stock before it counts dates < , not <=)", F(r, "stock") != 7)
			}
		}
		// opening stock is dated a minute ahead, so a purchase right after it reads as earlier
		o := Must(t, mk(M{"nameEn": "Opening item", "pricing": M{"purchase": 10, "retail": 20}, "stock": M{s.MS: M{"qty": 5}}}), 201, "opening").Body
		Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": s.Now(), "vendorId": v["id"], "items": []M{purItem(S(o["id"]), s.MS, 10, 12, 0)}})
		purWaitStock(t, s, S(o["id"]), s.MS, 15)
		Eventually(t, "rows", func() bool {
			rows = Must(t, Call(t, "GET", "/products/"+S(o["id"])+"/history?kind=all&storeId="+s.ID, s.Token, nil), 200, "history").Data()
			return len(rows) == 2
		})
		for _, r := range rows {
			if S(r["kind"]) == "purchases" {
				KnownBug(t, "NEW-HIST-OPENING", "opening stock is dated one minute in the future: the purchase after it shows stock 10, not 15", F(r, "stock") == 10)
			}
		}
	})
}

// TestInventory_Lookups: categories, brands, customer / vendor / expense
// categories (org-scoped) and product specs.
func TestInventory_Lookups(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")

	t.Run("categories", func(t *testing.T) {
		parent := Create(t, s.Token, "categories", M{"nameEn": "Engine", "nameAr": "محرك", "description": "d"})
		child := Must(t, Call(t, "POST", "/categories", s.Token, M{"nameEn": "Filters", "parentId": parent["id"]}), 201, "child").Body
		if S(child["parentId"]) != S(parent["id"]) || S(parent["nameAr"]) != "محرك" {
			t.Errorf("category: %v %v", child, parent)
		}
		purNeedErr(t, Call(t, "POST", "/categories", s.Token, M{"nameEn": "Engine"}), 400, "nameEn", "duplicate name")
		purNeedErr(t, Call(t, "POST", "/categories", s.Token, M{"nameEn": ""}), 400, "nameEn", "no name")
		purNeedErr(t, Call(t, "POST", "/categories", s.Token, M{"nameEn": "X", "parentId": "cat_nope"}), 400, "parentId", "unknown parent")
		purNeedErr(t, PatchResp(t, s.Token, "categories", S(parent["id"]), M{"parentId": parent["id"]}), 400, "parentId", "own parent")
		Patch(t, s.Token, "categories", S(child["id"]), M{"nameEn": "Oil filters"})
		Must(t, Call(t, "PUT", "/categories/"+S(child["id"]), s.Token, M{"nameEn": "Air filters"}), 200, "put")
		// org-scoped: the other company never sees it
		if purIDs(List(t, other.Token, "categories", ""))[S(parent["id"])] {
			t.Error("category leaked to another company")
		}
		purNeedErr(t, Call(t, "GET", "/categories/"+S(parent["id"]), other.Token, nil), 404, "", "category of another company")
		Must(t, Call(t, "DELETE", "/categories/"+S(child["id"]), s.Token, nil), 200, "delete")
		if purIDs(List(t, s.Token, "categories", ""))[S(child["id"])] {
			t.Error("deleted category listed")
		}
		Must(t, Call(t, "POST", "/categories/"+S(child["id"])+"/restore", s.Token, nil), 200, "restore")
		if !purIDs(List(t, s.Token, "categories", "q=Air"))[S(child["id"])] {
			t.Error("restored category not found")
		}
	})

	t.Run("brands", func(t *testing.T) {
		a := Create(t, s.Token, "brands", M{"name": "Arabian Oud", "nameAr": "العربية للعود", "website": "x.example"})
		b := Create(t, s.Token, "brands", M{"name": "Arabian Pipes"})
		if S(a["code"]) == "" || S(a["code"]) == S(b["code"]) {
			t.Errorf("brand codes must be unique: %v %v", a["code"], b["code"])
		}
		purNeedErr(t, Call(t, "POST", "/brands", s.Token, M{"name": " "}), 400, "name", "blank brand")
		Patch(t, s.Token, "brands", S(a["id"]), M{"name": "Arabian Oud Co"})
		Must(t, Call(t, "PUT", "/brands/"+S(b["id"]), s.Token, M{"name": "Pipes"}), 200, "put brand")
		purNeedErr(t, Call(t, "PUT", "/brands/"+S(b["id"]), s.Token, M{"name": ""}), 400, "name", "put blank brand")
		Must(t, Call(t, "DELETE", "/brands/"+S(b["id"]), s.Token, nil), 200, "delete brand")
		Must(t, Call(t, "POST", "/brands/"+S(b["id"])+"/restore", s.Token, nil), 200, "restore brand")
		purNeedErr(t, Call(t, "GET", "/brands/"+S(a["id"]), other.Token, nil), 404, "", "brand of another company")
	})

	t.Run("customer categories", func(t *testing.T) {
		c := Must(t, Call(t, "POST", "/customer-categories", s.Token, M{"name": "كبار العملاء"}), 201, "customer category").Body
		id := S(c["id"])
		if !strings.HasPrefix(id, "cct_") {
			t.Errorf("id: %v", id)
		}
		if g := Patch(t, s.Token, "customer-categories", id, M{"name": "VIP"}); S(g["name"]) != "VIP" {
			t.Errorf("rename: %v", g["name"])
		}
		Must(t, Call(t, "PUT", "/customer-categories/"+id, s.Token, M{"name": "Gold"}), 200, "put")
		// org-scoped native records live in one main-DB collection with no company filter
		leak := purIDs(List(t, other.Token, "customer-categories", ""))[id]
		KnownBug(t, "NEW-CCT-LEAK", "another company lists customer categories of every company", leak)
		g := Call(t, "GET", "/customer-categories/"+id, other.Token, nil)
		KnownBug(t, "NEW-CCT-LEAK", "another company reads a customer category by id (want 404)", g.Code != 404)
		Must(t, Call(t, "DELETE", "/customer-categories/"+id, s.Token, nil), 200, "delete")
		Must(t, Call(t, "POST", "/customer-categories/"+id+"/restore", s.Token, nil), 200, "restore")
		purNeedErr(t, Call(t, "PATCH", "/customer-categories/"+id, s.Token, M{"name": "x"}, "If-Match", "1"), 409, "", "stale")
		purNeedErr(t, Call(t, "POST", "/customer-categories/cct_missing/restore", s.Token, nil), 404, "", "restore unknown")
	})

	t.Run("vendor categories", func(t *testing.T) {
		c := Must(t, Call(t, "POST", "/vendor-categories", s.Token, M{"name": "Parts"}), 201, "vendor category").Body
		id := S(c["id"])
		purNeedErr(t, Call(t, "POST", "/vendor-categories", s.Token, M{"name": ""}), 400, "name", "blank")
		Patch(t, s.Token, "vendor-categories", id, M{"name": "Spare parts"})
		Must(t, Call(t, "PUT", "/vendor-categories/"+id, s.Token, M{"name": "Spares"}), 200, "put")
		// a vendor tagged with the category by name
		v := Create(t, s.Token, "vendors", M{"storeId": s.ID, "nameEn": "Tagged vendor", "category": []string{"Spares"}})
		if len(Objs(v["category"])) != 0 || len(v["category"].([]interface{})) != 1 {
			t.Errorf("vendor category: %v", v["category"])
		}
		if !purIDs(List(t, s.Token, "vendors", "storeId="+s.ID+"&where.category=Spares&select=id"))[S(v["id"])] {
			t.Error("where.category")
		}
		Must(t, Call(t, "DELETE", "/vendor-categories/"+id, s.Token, nil), 200, "delete")
		r := Call(t, "POST", "/vendor-categories/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore vendor category: %s", r)
		}
	})

	t.Run("expense categories", func(t *testing.T) {
		c := Must(t, Call(t, "POST", "/expense-categories", s.Token, M{"nameEn": "Rent", "nameAr": "إيجار"}), 201, "expense category").Body
		id := S(c["id"])
		purNeedErr(t, Call(t, "POST", "/expense-categories", s.Token, M{"nameEn": ""}), 400, "nameEn", "blank")
		if purIDs(List(t, other.Token, "expense-categories", ""))[id] {
			t.Error("expense category leaked to another company")
		}
		if !purIDs(List(t, s.Token, "expense-categories", ""))[id] {
			t.Error("expense category not listed")
		}
		Patch(t, s.Token, "expense-categories", id, M{"nameEn": "Office rent"})
		Must(t, Call(t, "PUT", "/expense-categories/"+id, s.Token, M{"nameEn": "Shop rent"}), 200, "put")
		Must(t, Call(t, "DELETE", "/expense-categories/"+id, s.Token, nil), 200, "delete")
		r := Call(t, "POST", "/expense-categories/"+id+"/restore", s.Token, nil)
		if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("restore expense category: %s", r)
		}
	})

	t.Run("product specs", func(t *testing.T) {
		c := Must(t, Call(t, "POST", "/product-specs", s.Token, M{"storeId": s.ID, "kind": "size", "name": "17 inch", "nameAr": "١٧ إنش"}), 201, "spec").Body
		id := S(c["id"])
		purNeedErr(t, Call(t, "POST", "/product-specs", s.Token, M{"storeId": s.ID, "kind": "zzz", "name": "x"}), 400, "kind", "bad kind")
		purNeedErr(t, Call(t, "POST", "/product-specs", s.Token, M{"storeId": s.ID, "kind": "size", "name": ""}), 400, "name", "no name")
		Patch(t, s.Token, "product-specs", id, M{"name": "18 inch"})
		Must(t, Call(t, "PUT", "/product-specs/"+id, s.Token, M{"kind": "size", "name": "19 inch"}), 200, "put spec")
		Must(t, Call(t, "DELETE", "/product-specs/"+id, s.Token, nil), 200, "delete spec")
		Must(t, Call(t, "POST", "/product-specs/"+id+"/restore", s.Token, nil), 200, "restore spec")
		purNeedErr(t, Call(t, "GET", "/product-specs/"+id, other.Token, nil), 404, "", "spec of another company")
	})
}

// TestInventory_VendorsCustomers: party CRUD, validation, Arabic names,
// opening balances and duplicates.
func TestInventory_VendorsCustomers(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	for _, kind := range []string{"vendors", "customers"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			mk := func(b M) Resp { b["storeId"] = s.ID; return Call(t, "POST", "/"+kind, s.Token, b) }
			r := Must(t, mk(M{"nameEn": "Acme Trading", "nameAr": "أكمي للتجارة", "vatNo": "399999999900003", "crNo": "4030360927",
				"phone": "0512345678", "email": "acme@e2e.example", "address": M{"buildingNo": "1234", "streetEn": "Olaya", "cityEn": "Riyadh", "postalCode": "12345"}}), 201, "create").Body
			id := S(r["id"])
			if S(r["nameAr"]) != "أكمي للتجارة" || S(r["vatNo"]) != "399999999900003" || S(r["phoneAr"]) != "٠٥١٢٣٤٥٦٧٨" || S(r["code"]) == "" {
				t.Errorf("party: %v", r)
			}
			cases := []struct {
				field string
				body  M
			}{
				{"nameEn", M{"nameEn": "A"}},
				{"nameAr", M{"nameEn": "Latin only", "nameAr": "abc"}},
				{"vatNo", M{"nameEn": "Bad VAT", "vatNo": "12345"}},
				{"vatNo", M{"nameEn": "Bad VAT 2", "vatNo": "199999999900003"}},
				{"email", M{"nameEn": "Bad mail", "email": "x@"}},
				{"phone", M{"nameEn": "Bad phone", "phone": "12"}},
				{"creditLimit", M{"nameEn": "Neg limit", "creditLimit": -5}},
				{"openingBalance", M{"nameEn": "Neg opening", "openingBalance": -1}},
			}
			for _, c := range cases {
				purNeedErr(t, mk(c.body), 400, c.field, kind+" "+c.field)
			}
			// the legacy system allows a second party with the same VAT number or name (branches)
			Must(t, mk(M{"nameEn": "Acme Branch", "vatNo": "399999999900003"}), 201, "same VAT")
			Must(t, mk(M{"nameEn": "Acme Trading"}), 201, "same name")
			// opening balance: payable for a vendor (negative), receivable for a customer
			obType, want := "credit", -100.0
			if kind == "customers" {
				obType, want = "debit", 100.0
			}
			ob := Must(t, mk(M{"nameEn": "Opening party", "openingBalance": 100, "openingBalanceType": obType, "openingBalanceDate": s.Now()}), 201, "opening").Body
			if F(ob, "creditBalance") != want || F(ob, "openingBalance") != 100 {
				t.Errorf("opening balance: %v %v", ob["creditBalance"], ob["openingBalance"])
			}
			if g := Patch(t, s.Token, kind, S(ob["id"]), M{"remarks": "r"}); F(g, "creditBalance") != want {
				t.Errorf("balance after edit: %v", g["creditBalance"])
			}
			if F(Read(t, s.Token, kind, S(ob["id"])), "creditBalance") != want {
				t.Error("balance on read")
			}
			over := List(t, s.Token, kind, "storeId="+s.ID+"&min.creditBalance=50&select=id")
			if kind == "customers" && !purIDs(over)[S(ob["id"])] {
				t.Errorf("min.creditBalance: %v", over)
			}
			if !purIDs(List(t, s.Token, kind, "storeId="+s.ID+"&q="+url.QueryEscape("أكمي")+"&select=id"))[id] {
				t.Error("Arabic search")
			}
			if !purIDs(List(t, s.Token, kind, "storeId="+s.ID+"&q=399999999900003&select=id"))[id] {
				t.Error("VAT search")
			}
			g := Patch(t, s.Token, kind, id, M{"nameAr": "أكمي الجديدة", "creditLimit": 5000})
			if S(g["nameAr"]) != "أكمي الجديدة" || F(g, "creditLimit") != 5000 {
				t.Errorf("patch: %v", g)
			}
			purNeedErr(t, PatchResp(t, s.Token, kind, id, M{"vatNo": "1"}), 400, "vatNo", "patch bad VAT")
			purNeedErr(t, Call(t, "PATCH", "/"+kind+"/"+id, s.Token, M{"remarks": "x"}, "If-Match", "1"), 409, "", "stale")
			purNeedErr(t, Call(t, "PATCH", "/"+kind+"/"+id, s.Token, M{"storeId": other.ID}), 403, "", "move to another store")
			Must(t, Call(t, "PUT", "/"+kind+"/"+id, s.Token, M{"nameEn": "Acme Put", "vatNo": "399999999900003"}), 200, "put")
			purNeedErr(t, Call(t, "PUT", "/"+kind+"/"+id, s.Token, M{"nameEn": "Z"}), 400, "nameEn", "put short name")
			purNeedErr(t, Call(t, "GET", "/"+kind+"/"+id, other.Token, nil), 404, "", "read from another company")
			purNeedErr(t, Call(t, "POST", "/"+kind, s.Token, M{"storeId": other.ID, "nameEn": "Foreign"}), 403, "", "create in another store")
			d := Must(t, Call(t, "DELETE", "/"+kind+"/"+id, s.Token, nil), 200, "delete").Body
			if d["deleted"] != true {
				t.Errorf("deleted: %v", d["deleted"])
			}
			if purIDs(List(t, s.Token, kind, "storeId="+s.ID+"&select=id"))[id] {
				t.Error("deleted party listed")
			}
			if r := Must(t, Call(t, "POST", "/"+kind+"/"+id+"/restore", s.Token, nil), 200, "restore").Body; r["deleted"] != false {
				t.Errorf("restored: %v", r["deleted"])
			}
			purNeedErr(t, Call(t, "POST", "/"+kind+"/000000000000000000000001/restore", s.Token, nil), 404, "", "restore unknown")
		})
	}
}
