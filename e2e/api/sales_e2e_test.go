//go:build e2e

package api

// Sales invoices and sales returns end to end: totals and VAT against an
// independent oracle, payments and customer balances, stock per warehouse,
// updates, lists and /stats, concurrency, idempotency and the return limits.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- oracle (exact decimals, integer cents) ----------

// salesRat is the decimal the client typed (the shortest repr of the float).
func salesRat(x float64) *big.Rat {
	r, _ := new(big.Rat).SetString(strconv.FormatFloat(x, 'f', -1, 64))
	return r
}

// salesRound rounds r to an integer, halves away from zero.
func salesRound(r *big.Rat) int64 {
	neg := r.Sign() < 0
	a := new(big.Rat).Abs(r)
	n := new(big.Int).Mul(a.Num(), big.NewInt(2))
	n.Add(n, a.Denom())
	n.Quo(n, new(big.Int).Mul(a.Denom(), big.NewInt(2)))
	if neg {
		return -n.Int64()
	}
	return n.Int64()
}

// salesToCents is a decimal amount in cents (half away from zero).
func salesToCents(r *big.Rat) int64 { return salesRound(new(big.Rat).Mul(r, big.NewRat(100, 1))) }

type salesLn struct{ qty, price, disc float64 }

type salesRDoc struct {
	id        string
	net, paid int64
}

type salesTot struct{ lines, taxable, vat, net int64 }

// salesOracle: each line qty × (price − discount) to the cent, then
// taxable = lines − discount + shipping, VAT on the taxable total.
func salesOracle(lines []salesLn, discount, shipping, vatPct float64) salesTot {
	var o salesTot
	for _, l := range lines {
		amt := new(big.Rat).Mul(salesRat(l.qty), new(big.Rat).Sub(salesRat(l.price), salesRat(l.disc)))
		o.lines += salesToCents(amt)
	}
	o.taxable = o.lines - salesToCents(salesRat(discount)) + salesToCents(salesRat(shipping))
	v := new(big.Rat).Mul(big.NewRat(o.taxable, 1), salesRat(vatPct))
	o.vat = salesRound(v.Quo(v, big.NewRat(100, 1)))
	o.net = o.taxable + o.vat
	return o
}

func salesC(x float64) string { return fmt.Sprintf("%.2f", x) }

func salesEqC(t testing.TB, what string, got float64, wantCents int64) {
	t.Helper()
	if Cents(got) != wantCents {
		t.Errorf("%s: got %.2f, want %.2f", what, got, float64(wantCents)/100)
	}
}

// ---------- helpers ----------

func salesBody(s *Store, customerID interface{}, items []M, pays []M) M {
	b := M{"storeId": s.ID, "date": s.Now(), "items": items}
	if customerID != nil {
		b["customerId"] = customerID
	}
	if pays != nil {
		b["payments"] = pays
	}
	return b
}

func salesLine(pid interface{}, wh string, qty, price float64) M {
	return M{"productId": pid, "qty": qty, "unitPrice": price, "unitDiscount": 0, "warehouseId": wh, "vatPercent": 15}
}

func salesPay(amount float64, method string) M { return M{"amount": amount, "method": method} }

func salesStockAt(t testing.TB, s *Store, pid interface{}, wh string) float64 {
	t.Helper()
	return F(Read(t, s.Token, "products", S(pid)), "stock."+wh+".qty")
}

// salesStockSettles waits for the stock to reach want and reports the last value.
func salesStockSettles(t testing.TB, s *Store, pid interface{}, wh string, want float64) (float64, bool) {
	t.Helper()
	var got float64
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got = salesStockAt(t, s, pid, wh); Cents(got) == Cents(want) {
			return got, true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return got, false
}

func salesWaitStock(t testing.TB, s *Store, pid interface{}, wh string, want float64) {
	t.Helper()
	var got float64
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got = salesStockAt(t, s, pid, wh); Cents(got) == Cents(want) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Errorf("stock of %v in %s: got %v, want %v", pid, wh, got, want)
}

func salesBalance(t testing.TB, s *Store, cid interface{}) float64 {
	t.Helper()
	return F(Read(t, s.Token, "customers", S(cid)), "creditBalance")
}

func salesWaitBalance(t testing.TB, s *Store, cid interface{}, wantCents int64) {
	t.Helper()
	var got float64
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got = salesBalance(t, s, cid); Cents(got) == wantCents {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Errorf("customer %v balance: got %.2f, want %.2f", cid, got, float64(wantCents)/100)
}

// salesWantErr checks a 400 validation answer naming the field.
func salesWantErr(t testing.TB, r Resp, status int, field string) {
	t.Helper()
	if r.Code != status {
		t.Errorf("want HTTP %d (%s), got %s", status, field, r)
		return
	}
	if field != "" && r.ErrField(field) == "" {
		t.Errorf("want error on %q, got %s", field, r)
	}
}

// salesTry is Call with a short timeout that reports a dropped connection or
// a hang instead of failing the test (used where the server is known to panic).
func salesTry(t testing.TB, method, path, token string, body interface{}, timeout time.Duration) (Resp, error) {
	t.Helper()
	j, _ := json.Marshal(body)
	req, err := http.NewRequest(method, baseURL+"/v1/erp"+path, bytes.NewReader(j))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return Resp{}, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	recordStatus(method, req.URL.Path, res.StatusCode)
	out := Resp{Code: res.StatusCode, Raw: string(b), Header: res.Header}
	_ = json.Unmarshal(b, &out.Body)
	return out, nil
}

func salesCodeNo(t testing.TB, code, prefix string) int {
	t.Helper()
	if !strings.HasPrefix(code, prefix) {
		t.Errorf("code %q does not start with %q", code, prefix)
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(code, prefix))
	if err != nil {
		t.Errorf("code %q: %v", code, err)
	}
	return n
}

func salesCount(t testing.TB, s *Store, path string) int {
	t.Helper()
	r := Must(t, Call(t, "GET", "/"+path+"?storeId="+s.ID+"&select=code", s.Token, nil), 200, "list "+path)
	return int(Num(r.Body["total"]))
}

// ---------- tests ----------

// TestSalesCreate_TotalsVATOracle: line discounts, fractional quantities,
// document discount, shipping, cash discount and commission, half-cent VAT.
func TestSalesCreate_TotalsVATOracle(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 10, 20, 1000)
	pid := S(p["id"])

	t.Run("one line", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 2, 20)}, nil))
		o := salesOracle([]salesLn{{2, 20, 0}}, 0, 0, 15)
		salesEqC(t, "total", F(r, "legacyTotals.total"), o.lines)
		salesEqC(t, "vat", F(r, "legacyTotals.vat"), o.vat)
		salesEqC(t, "net", F(r, "legacyTotals.net"), o.net)
		salesEqC(t, "profit", F(r, "legacyTotals.profit"), 2*(2000-1000))
		if F(r, "items.0.vatPercent") != 15 || F(r, "vatPercent") != 15 {
			t.Errorf("vatPercent per line / document: %v %v", Get(r, "items.0.vatPercent"), r["vatPercent"])
		}
		if S(Get(r, "items.0.nameAr")) != "منتج" || S(Get(r, "items.0.warehouseId")) != s.MS {
			t.Errorf("line copied from the product: %v", Get(r, "items.0"))
		}
	})

	t.Run("many lines with discounts shipping cash discount commission", func(t *testing.T) {
		b := salesBody(s, nil, []M{
			{"productId": pid, "qty": 1.5, "unitPrice": 33.33, "unitDiscount": 1.11, "warehouseId": s.MS},
			s.Line(pid, 3, 0.1),
			{"productId": pid, "qty": 0.333, "unitPrice": 7.77, "unitDiscount": 0, "warehouseId": s.MS},
		}, []M{salesPay(10, "cash"), salesPay(5, "bank_transfer")})
		b["discount"], b["shipping"], b["cashDiscount"], b["commission"] = 2, 5, 1, 3
		b["remarks"] = "فاتورة اختبار ✓"
		r := Create(t, s.Token, "sales", b)
		o := salesOracle([]salesLn{{1.5, 33.33, 1.11}, {3, 0.1, 0}, {0.333, 7.77, 0}}, 2, 5, 15)
		salesEqC(t, "total", F(r, "legacyTotals.total"), o.lines)
		salesEqC(t, "vat", F(r, "legacyTotals.vat"), o.vat)
		salesEqC(t, "net", F(r, "legacyTotals.net"), o.net)
		salesEqC(t, "paid", F(r, "legacyTotals.paid"), 1500)
		salesEqC(t, "balance", F(r, "legacyTotals.balance"), o.net-1500-100)
		if S(Get(r, "legacyTotals.paymentStatus")) != "paid_partially" {
			t.Errorf("payment status %v", Get(r, "legacyTotals.paymentStatus"))
		}
		if r["remarks"] != "فاتورة اختبار ✓" || F(r, "commission") != 3 || r["commissionMethod"] != "cash" ||
			F(r, "discount") != 2 || F(r, "shipping") != 5 || F(r, "cashDiscount") != 1 {
			t.Errorf("echoed fields: remarks=%v commission=%v/%v discount=%v shipping=%v cash=%v", r["remarks"],
				r["commission"], r["commissionMethod"], r["discount"], r["shipping"], r["cashDiscount"])
		}
		if n := len(Objs(r["items"])); n != 3 {
			t.Errorf("%d lines", n)
		}
	})

	t.Run("half cent VAT rounding", func(t *testing.T) {
		// VAT of each taxable amount ends in exactly half a cent
		for _, up := range []float64{0.1, 1.1, 10.1, 0.3, 0.7, 2.3, 0.05, 4.35, 0.03, 0.01, 1.03} {
			r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 1, up)}, nil))
			o := salesOracle([]salesLn{{1, up, 0}}, 0, 0, 15)
			salesEqC(t, "vat of "+salesC(up), F(r, "legacyTotals.vat"), o.vat)
			salesEqC(t, "net of "+salesC(up), F(r, "legacyTotals.net"), o.net)
		}
		// 3.30 × 15% = 0.495 exactly → 0.50, but 3.3*0.15 in float is 0.49499…
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 1, 3.3)}, nil))
		o := salesOracle([]salesLn{{1, 3.3, 0}}, 0, 0, 15)
		KnownBug(t, "NEW-vat-half-cent", "VAT of 3.30 at 15% is 0.495 → 0.50; the server rounds the float product 0.49499… to 0.49 (net 3.79 not 3.80)",
			Cents(F(r, "legacyTotals.vat")) != o.vat)
	})

	t.Run("huge amounts", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 100000, 99999.99)}, nil))
		o := salesOracle([]salesLn{{100000, 99999.99, 0}}, 0, 0, 15)
		salesEqC(t, "net", F(r, "legacyTotals.net"), o.net)
		salesEqC(t, "vat", F(r, "legacyTotals.vat"), o.vat)
	})

	t.Run("free text line becomes a service product", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{{"nameEn": "Installation service " + Uniq(), "nameAr": "خدمة تركيب", "qty": 1, "unitPrice": 50}}, nil))
		if S(Get(r, "items.0.productId")) == "" || S(Get(r, "items.0.nameAr")) != "خدمة تركيب" {
			t.Errorf("free-text line: %v", Get(r, "items.0"))
		}
		salesEqC(t, "net", F(r, "legacyTotals.net"), salesOracle([]salesLn{{1, 50, 0}}, 0, 0, 15).net)
	})

	t.Run("B2B customer with VAT number is a standard invoice", func(t *testing.T) {
		b2b := s.Customer(t, "300000000000003")
		b := salesBody(s, b2b["id"], []M{s.Line(pid, 1, 10)}, nil)
		b["vatNo"] = "300000000000003"
		r := Create(t, s.Token, "sales", b)
		if S(Get(r, "zatca.invoiceType")) != "standard" || r["vatNo"] != "300000000000003" || r["customerId"] != b2b["id"] {
			t.Errorf("B2B sale: zatca=%v vatNo=%v customer=%v", r["zatca"], r["vatNo"], r["customerId"])
		}
		// without the document VAT number the invoice is simplified (the
		// client copies the customer's VAT number into the document)
		r = Create(t, s.Token, "sales", salesBody(s, b2b["id"], []M{s.Line(pid, 1, 10)}, nil))
		if S(Get(r, "zatca.invoiceType")) != "simplified" {
			t.Errorf("no document vatNo: %v", r["zatca"])
		}
	})

	t.Run("walk-in customer", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 1, 10)}, nil))
		if r["customerName"] != "UNKNOWN" || S(r["customerId"]) == "" || S(Get(r, "zatca.invoiceType")) != "simplified" {
			t.Errorf("walk-in: %v %v %v", r["customerId"], r["customerName"], r["zatca"])
		}
		r2 := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 1, 10)}, nil))
		if r2["customerId"] != r["customerId"] {
			t.Errorf("walk-in sales use one UNKNOWN customer: %v vs %v", r["customerId"], r2["customerId"])
		}
	})
}

// TestSalesCreate_Validation: every refused body answers 400 with the field
// and creates nothing.
func TestSalesCreate_Validation(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	o := Signup(t, "")
	p := s.Product(t, 10, 20, 50)
	pid := S(p["id"])
	op := o.Product(t, 1, 2, 5)
	oc := o.Customer(t, "")
	ok := 0

	cases := []struct {
		name   string
		body   M
		status int
		field  string
	}{
		{"no items", salesBody(s, nil, []M{}, nil), 400, "items"},
		{"no date", M{"storeId": s.ID, "items": []M{s.Line(pid, 1, 5)}}, 400, "date"},
		{"bad date", M{"storeId": s.ID, "date": "2026-13-45T10:00", "items": []M{s.Line(pid, 1, 5)}}, 400, "date"},
		{"zero qty", salesBody(s, nil, []M{s.Line(pid, 0, 5)}, nil), 400, "items.0.qty"},
		{"second line zero qty", salesBody(s, nil, []M{s.Line(pid, 1, 5), s.Line(pid, 0, 5)}, nil), 400, "items.1.qty"},
		{"missing product", salesBody(s, nil, []M{s.Line("aaaaaaaaaaaaaaaaaaaaaaaa", 1, 5)}, nil), 400, "items.0.productId"},
		{"product of another store", salesBody(s, nil, []M{s.Line(S(op["id"]), 1, 5)}, nil), 400, "items.0.productId"},
		{"product reference that is no id", salesBody(s, nil, []M{s.Line("no-such-product", 1, 5)}, nil), 400, "items.0.productId"},
		{"unknown warehouse", salesBody(s, nil, []M{salesLine(pid, "zzz", 1, 5)}, nil), 400, "items.0.warehouseId"},
		{"free text line without a name", salesBody(s, nil, []M{{"qty": 1, "unitPrice": 5}}, nil), 400, "items.0.nameEn"},
		{"bad VAT number", func() M { b := salesBody(s, nil, []M{s.Line(pid, 1, 5)}, nil); b["vatNo"] = "123"; return b }(), 400, "vatNo"},
		{"unsupported payment method", salesBody(s, nil, []M{s.Line(pid, 1, 10)}, []M{salesPay(1, "bitcoin")}), 400, "payments.0.method"},
		{"negative payment", salesBody(s, nil, []M{s.Line(pid, 1, 10)}, []M{salesPay(-1, "cash")}), 400, "payments.0.amount"},
		{"bad payment date", salesBody(s, nil, []M{s.Line(pid, 1, 10)}, []M{{"amount": 1, "method": "cash", "date": "yesterday"}}), 400, "payments.0.date"},
		{"overpaid", salesBody(s, nil, []M{s.Line(pid, 1, 10)}, []M{salesPay(11.51, "cash")}), 400, "payments"},
		{"overpaid with cash discount", func() M {
			b := salesBody(s, nil, []M{s.Line(pid, 1, 10)}, []M{salesPay(11, "cash")})
			b["cashDiscount"] = 1
			return b
		}(), 400, "payments"},
		{"cash discount equal to net", func() M {
			b := salesBody(s, nil, []M{s.Line(pid, 1, 10)}, nil)
			b["cashDiscount"] = 11.5
			return b
		}(), 400, "cashDiscount"},
		{"negative discount", func() M { b := salesBody(s, nil, []M{s.Line(pid, 1, 10)}, nil); b["discount"] = -1; return b }(), 400, "discount"},
		{"another store", M{"storeId": o.ID, "date": s.Now(), "items": []M{s.Line(pid, 1, 5)}}, 403, ""},
		{"no store", M{"date": s.Now(), "items": []M{s.Line(pid, 1, 5)}}, 400, "storeId"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			salesWantErr(t, Call(t, "POST", "/sales", s.Token, c.body), c.status, c.field)
		})
	}

	t.Run("malformed JSON and no token", func(t *testing.T) {
		r := Call(t, "POST", "/sales", s.Token, `{"storeId": "`+s.ID+`", "items": [`)
		if r.Code != 400 || r.ErrCode() != "malformed_json" {
			t.Errorf("malformed JSON: %s", r)
		}
		if r := Call(t, "POST", "/sales", "", salesBody(s, nil, []M{s.Line(pid, 1, 5)}, nil)); r.Code != 401 {
			t.Errorf("no token: %s", r)
		}
		if r := Call(t, "POST", "/sales", "not-a-token", salesBody(s, nil, []M{s.Line(pid, 1, 5)}, nil)); r.Code != 401 {
			t.Errorf("bad token: %s", r)
		}
	})

	t.Run("unit price zero", func(t *testing.T) {
		r := Call(t, "POST", "/sales", s.Token, salesBody(s, nil, []M{s.Line(pid, 1, 0)}, nil))
		salesWantErr(t, r, 400, "items.0.unitPrice")
		KnownBug(t, "NEW-zero-net-cash-discount", "a zero-price sale also gets a bogus cashDiscount error \"Cash discount should not be >= 0.00\" with no cash discount",
			r.ErrField("cashDiscount") != "")
	})

	t.Run("negative quantity price and line discount name the line", func(t *testing.T) {
		for _, c := range []struct {
			line  M
			field string
		}{
			{s.Line(pid, -1, 5), "items.0.qty"},
			{s.Line(pid, 1, -5), "items.0.unitPrice"},
			{M{"productId": pid, "qty": 1, "unitPrice": 5, "unitDiscount": 6, "warehouseId": s.MS}, "items.0.unitDiscount"},
		} {
			r := Call(t, "POST", "/sales", s.Token, salesBody(s, nil, []M{c.line}, nil))
			if r.Code != 400 {
				t.Errorf("%s: want 400, got %s", c.field, r)
				continue
			}
			KnownBug(t, "NEW-line-error-masked", "a line with a negative amount is refused with payments: \"paid + cash discount exceeds the net total\" instead of "+c.field,
				r.ErrField(c.field) == "" && r.ErrField("payments") != "")
		}
	})

	t.Run("customer that does not exist", func(t *testing.T) {
		for _, cid := range []string{"bbbbbbbbbbbbbbbbbbbbbbbb", S(oc["id"])} {
			r := Call(t, "POST", "/sales", s.Token, salesBody(s, cid, []M{s.Line(pid, 1, 5)}, nil))
			if r.Code == 201 {
				ok++
			}
			KnownBug(t, "NEW-sales-unknown-customer", "a customerId that is not a customer of the store (unknown, or another store's) is silently replaced by the UNKNOWN walk-in customer (201) instead of 400 customerId",
				r.Code == 201 && r.Body["customerName"] == "UNKNOWN")
		}
	})

	t.Run("nothing else was created", func(t *testing.T) {
		if n := salesCount(t, s, "sales"); n != ok {
			t.Errorf("%d sales in the store, want %d", n, ok)
		}
		Eventually(t, "stock unchanged", func() bool { return s.Stock(t, pid) == 50-float64(ok) })
	})
}

// TestSalesPayments_CustomerBalance: unpaid, partial, full and several
// payments; payment status, balance and the customer's receivable.
func TestSalesPayments_CustomerBalance(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 10, 20, 100)
	pid := S(p["id"])
	c := s.Customer(t, "")
	cid := c["id"]
	net := salesOracle([]salesLn{{2, 50, 0}}, 0, 0, 15).net // 115.00
	var receivable int64

	check := func(t *testing.T, r M, paid int64, status string) {
		t.Helper()
		salesEqC(t, "net", F(r, "legacyTotals.net"), net)
		salesEqC(t, "paid", F(r, "legacyTotals.paid"), paid)
		salesEqC(t, "balance", F(r, "legacyTotals.balance"), net-paid)
		if got := S(Get(r, "legacyTotals.paymentStatus")); got != status {
			t.Errorf("payment status %q, want %q", got, status)
		}
	}

	var credit M
	t.Run("unpaid sale on credit", func(t *testing.T) {
		credit = Create(t, s.Token, "sales", salesBody(s, cid, []M{s.Line(pid, 2, 50)}, nil))
		check(t, credit, 0, "not_paid")
		if len(Objs(credit["payments"])) != 0 {
			t.Errorf("payments %v", credit["payments"])
		}
		receivable += net
		salesWaitBalance(t, s, cid, receivable)
	})
	t.Run("partial", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, cid, []M{s.Line(pid, 2, 50)}, []M{salesPay(40, "cash")}))
		check(t, r, 4000, "paid_partially")
		receivable += net - 4000
		salesWaitBalance(t, s, cid, receivable)
	})
	t.Run("full with several methods", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, cid, []M{s.Line(pid, 2, 50)},
			[]M{salesPay(15, "cash"), salesPay(50, "debit_card"), salesPay(25, "bank_transfer"), salesPay(25, "credit_card")}))
		check(t, r, net, "paid")
		pays := Objs(r["payments"])
		if len(pays) != 4 || pays[1]["method"] != "debit_card" || F(pays[2], "amount") != 25 || S(pays[0]["id"]) == "" {
			t.Errorf("payments %v", r["payments"])
		}
		salesWaitBalance(t, s, cid, receivable)
	})
	t.Run("zero payment rows are dropped", func(t *testing.T) {
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 2, 50)}, []M{salesPay(0, "cash"), salesPay(10, "cash")}))
		if len(Objs(r["payments"])) != 1 {
			t.Errorf("payments %v", r["payments"])
		}
		check(t, r, 1000, "paid_partially")
	})
	t.Run("cash discount settles the rest", func(t *testing.T) {
		b := salesBody(s, nil, []M{s.Line(pid, 2, 50)}, []M{salesPay(110, "cash")})
		b["cashDiscount"] = 5
		r := Create(t, s.Token, "sales", b)
		salesEqC(t, "balance", F(r, "legacyTotals.balance"), 0)
		if S(Get(r, "legacyTotals.paymentStatus")) != "paid" {
			t.Errorf("status %v", Get(r, "legacyTotals.paymentStatus"))
		}
	})
	t.Run("payment dated before the invoice", func(t *testing.T) {
		// the legacy check is commented out: an earlier payment date is kept
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 2, 50)}, []M{{"amount": 1, "method": "cash", "date": "2020-01-01T10:00"}}))
		if S(Get(r, "payments.0.date")) != "2020-01-01T10:00" {
			t.Errorf("payment date %v", Get(r, "payments.0.date"))
		}
	})
	t.Run("receivable flow: later payments bring the balance to zero", func(t *testing.T) {
		id := S(credit["id"])
		r := Patch(t, s.Token, "sales", id, M{"payments": []M{{"amount": 15, "method": "cash", "date": s.Now()}}})
		check(t, r, 1500, "paid_partially")
		salesWaitBalance(t, s, cid, receivable-1500)
		cur := Read(t, s.Token, "sales", id)
		pays := append(Objs(cur["payments"]), M{"amount": 100, "method": "bank_cheque", "date": s.Now()})
		r = Patch(t, s.Token, "sales", id, M{"payments": pays})
		check(t, r, net, "paid")
		kept := false
		for _, pm := range Objs(r["payments"]) {
			kept = kept || (pm["id"] == Get(cur, "payments.0.id") && F(pm, "amount") == 15)
		}
		if len(Objs(r["payments"])) != 2 || !kept {
			t.Errorf("the first payment keeps its id: %v", r["payments"])
		}
		receivable -= net
		salesWaitBalance(t, s, cid, receivable)
		salesWantErr(t, PatchResp(t, s.Token, "sales", id, M{"payments": append(Objs(r["payments"]), salesPay(0.01, "cash"))}), 400, "payments")
		// removing every payment makes it a credit sale again
		r = Patch(t, s.Token, "sales", id, M{"payments": []M{}})
		check(t, r, 0, "not_paid")
		receivable += net
		salesWaitBalance(t, s, cid, receivable)
	})
	t.Run("list by payment status and customer", func(t *testing.T) {
		rows := List(t, s.Token, "sales", "storeId="+s.ID+"&where.customerId="+S(cid)+"&select=code,legacyTotals")
		var bal int64
		for _, r := range rows {
			bal += Cents(F(r, "legacyTotals.balance"))
		}
		if len(rows) != 3 || bal != receivable {
			t.Errorf("customer's sales: %d rows, balance %d, want 3 rows %d", len(rows), bal, receivable)
		}
		if n := len(List(t, s.Token, "sales", "storeId="+s.ID+"&where.paymentStatus=paid")); n != 2 {
			t.Errorf("%d paid sales, want 2", n)
		}
		if n := len(List(t, s.Token, "sales", "storeId="+s.ID+"&where.paymentStatus=not_paid")); n != 1 {
			t.Errorf("%d unpaid sales, want 1", n)
		}
	})
}

// TestSalesStock_WarehousesAndUpdates: stock out per warehouse, edits that
// move stock, version / history, If-Match, PUT, delete and restore.
func TestSalesStock_WarehousesAndUpdates(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	o := Signup(t, "")
	wh := S(Create(t, s.Token, "warehouses", M{"storeId": s.ID, "nameEn": "Second WH", "nameAr": "مستودع", "code": "W" + Digits(5)})["id"])
	a := Create(t, s.Token, "products", M{"storeId": s.ID, "nameEn": "A " + Uniq(), "nameAr": "أ",
		"pricing": M{"purchase": 5, "retail": 9}, "stock": M{s.MS: M{"qty": 20}, wh: M{"qty": 7}}})
	b := s.Product(t, 3, 7, 10)
	aid, bid := a["id"], b["id"]
	salesWaitStock(t, s, aid, wh, 7)

	var sale M
	t.Run("stock out per warehouse", func(t *testing.T) {
		sale = Create(t, s.Token, "sales", salesBody(s, nil, []M{salesLine(aid, s.MS, 2, 9), salesLine(aid, wh, 3, 9), salesLine(bid, s.MS, 1.5, 7)}, nil))
		salesWaitStock(t, s, aid, s.MS, 18)
		salesWaitStock(t, s, aid, wh, 4)
		salesWaitStock(t, s, bid, s.MS, 8.5)
		if S(Get(sale, "items.1.warehouseId")) != wh {
			t.Errorf("line warehouse %v", Get(sale, "items.1.warehouseId"))
		}
	})
	id := S(sale["id"])

	t.Run("selling more than in stock is allowed", func(t *testing.T) {
		// the legacy stock check is commented out: stock goes negative
		r := Create(t, s.Token, "sales", salesBody(s, nil, []M{salesLine(aid, wh, 10, 9)}, nil))
		salesWaitStock(t, s, aid, wh, -6)
		salesEqC(t, "net", F(r, "legacyTotals.net"), salesOracle([]salesLn{{10, 9, 0}}, 0, 0, 15).net)
	})

	t.Run("PATCH quantity re-adjusts stock", func(t *testing.T) {
		r := Patch(t, s.Token, "sales", id, M{"items": []M{salesLine(aid, s.MS, 5, 9), salesLine(aid, wh, 3, 9), salesLine(bid, s.MS, 1.5, 7)}})
		salesWaitStock(t, s, aid, s.MS, 15)
		salesWaitStock(t, s, aid, wh, -6)
		o := salesOracle([]salesLn{{5, 9, 0}, {3, 9, 0}, {1.5, 7, 0}}, 0, 0, 15)
		salesEqC(t, "net after edit", F(r, "legacyTotals.net"), o.net)
		if Num(r["version"]) != 2 {
			t.Errorf("version %v, want 2", r["version"])
		}
	})
	t.Run("remove a line puts its stock back", func(t *testing.T) {
		Patch(t, s.Token, "sales", id, M{"items": []M{salesLine(aid, s.MS, 5, 9), salesLine(aid, wh, 3, 9)}})
		salesWaitStock(t, s, bid, s.MS, 10)
		// let the edit's background work finish (see NEW-sales-stock-edit-race)
		time.Sleep(3 * time.Second)
	})
	t.Run("add a line", func(t *testing.T) {
		r := Patch(t, s.Token, "sales", id, M{"items": []M{salesLine(aid, s.MS, 5, 9), salesLine(aid, wh, 3, 9), salesLine(bid, s.MS, 4, 7)}})
		salesWaitStock(t, s, bid, s.MS, 6)
		if len(Objs(r["items"])) != 3 {
			t.Errorf("items %v", r["items"])
		}
	})
	t.Run("move a line to the other warehouse", func(t *testing.T) {
		Patch(t, s.Token, "sales", id, M{"items": []M{salesLine(aid, s.MS, 5, 9), salesLine(aid, s.MS, 3, 9), salesLine(bid, s.MS, 4, 7)}})
		salesWaitStock(t, s, aid, wh, -3)
		salesWaitStock(t, s, aid, s.MS, 12)
	})
	t.Run("refused PATCH changes nothing", func(t *testing.T) {
		before := Read(t, s.Token, "sales", id)
		salesWantErr(t, PatchResp(t, s.Token, "sales", id, M{"items": []M{salesLine(aid, s.MS, 0, 9)}}), 400, "items.0.qty")
		salesWantErr(t, PatchResp(t, s.Token, "sales", id, M{"items": []M{}}), 400, "items")
		salesWantErr(t, PatchResp(t, s.Token, "sales", id, M{"payments": []M{salesPay(100000, "cash")}}), 400, "payments")
		after := Read(t, s.Token, "sales", id)
		if Num(after["version"]) != Num(before["version"]) || len(Objs(after["items"])) != 3 {
			t.Errorf("refused edits changed the sale: v%v→v%v", before["version"], after["version"])
		}
		salesWaitStock(t, s, aid, s.MS, 12)
	})
	t.Run("If-Match", func(t *testing.T) {
		cur := Read(t, s.Token, "sales", id)
		v := Num(cur["version"])
		for _, im := range []string{"1", "abc", (v + 1).String()} {
			r := Call(t, "PATCH", "/sales/"+id, s.Token, M{"remarks": "stale"}, "If-Match", im)
			if r.Code != 409 || r.ErrCode() != "version_conflict" {
				t.Errorf("If-Match %s: %s", im, r)
			}
		}
		r := Must(t, Call(t, "PATCH", "/sales/"+id, s.Token, M{"remarks": "ملاحظة"}, "If-Match", `"`+v.String()+`"`), 200, "quoted If-Match")
		if r.Body["remarks"] != "ملاحظة" || Num(r.Body["version"]) != v+1 {
			t.Errorf("patch: remarks %v version %v", r.Body["remarks"], r.Body["version"])
		}
		// no If-Match: last write wins
		Must(t, Call(t, "PATCH", "/sales/"+id, s.Token, M{"remarks": "no if-match"}), 200, "PATCH without If-Match")
		// PUT does not check If-Match (engine.go: the check is PATCH only)
		cur = Read(t, s.Token, "sales", id)
		cur["remarks"] = "put"
		r = Call(t, "PUT", "/sales/"+id, s.Token, cur, "If-Match", "1")
		if r.Code != 200 || r.Body["remarks"] != "put" {
			t.Errorf("PUT with an old If-Match: %s", r)
		}
	})
	t.Run("history", func(t *testing.T) {
		r := Read(t, s.Token, "sales", id)
		h := Objs(r["history"])
		if len(h) != int(Num(r["version"])) || h[0]["action"] != "created" || h[1]["action"] != "e2e" {
			t.Errorf("version %v, history %v", r["version"], r["history"])
		}
		found := false
		for _, e := range h {
			for _, ch := range Objs(e["changes"]) {
				if ch["field"] == "items" && ch["from"] == "3 items" && ch["to"] == "2 items" {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("no history entry for the removed line: %v", h)
		}
		sel := Must(t, Call(t, "GET", "/sales/"+id+"?select=code,version", s.Token, nil), 200, "select").Body
		if sel["code"] != r["code"] || sel["items"] != nil {
			t.Errorf("select: %v", sel)
		}
	})
	t.Run("PUT replaces the document", func(t *testing.T) {
		r := Must(t, Call(t, "PUT", "/sales/"+id, s.Token, M{"date": s.Now(), "items": []M{salesLine(aid, wh, 1, 9)}}), 200, "PUT").Body
		if len(Objs(r["items"])) != 1 || r["customerName"] != "UNKNOWN" || r["remarks"] != "" {
			t.Errorf("PUT result: %v", r)
		}
		salesWaitStock(t, s, aid, s.MS, 20)
		salesWaitStock(t, s, aid, wh, -4)
		salesWaitStock(t, s, bid, s.MS, 10)
		salesWantErr(t, Call(t, "PUT", "/sales/"+id, s.Token, M{"date": s.Now(), "items": []M{}}), 400, "items")
	})
	t.Run("delete is not supported and restore is a no-op", func(t *testing.T) {
		for _, q := range []string{"", "?hard=1"} {
			r := Call(t, "DELETE", "/sales/"+id+q, s.Token, nil)
			if r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
				t.Errorf("DELETE%s: %s", q, r)
			}
		}
		r := Must(t, Call(t, "POST", "/sales/"+id+"/restore", s.Token, nil), 200, "restore").Body
		if r["deleted"] != false {
			t.Errorf("restore: %v", r["deleted"])
		}
		salesWaitStock(t, s, aid, wh, -4)
	})
	t.Run("back-to-back edits", func(t *testing.T) {
		// stock is recomputed in a goroutine per edit (controller/sales.go UpdateOrder):
		// re-adding a line right after removing it sometimes leaves its stock untouched
		c := s.Product(t, 1, 2, 10)
		e := Create(t, s.Token, "sales", salesBody(s, nil, []M{salesLine(aid, s.MS, 1, 9), salesLine(c["id"], s.MS, 2, 2)}, nil))
		salesWaitStock(t, s, c["id"], s.MS, 8)
		eid := S(e["id"])
		Patch(t, s.Token, "sales", eid, M{"items": []M{salesLine(aid, s.MS, 1, 9)}})
		Patch(t, s.Token, "sales", eid, M{"items": []M{salesLine(aid, s.MS, 1, 9), salesLine(c["id"], s.MS, 4, 2)}})
		if got, ok := salesStockSettles(t, s, c["id"], s.MS, 6); !ok {
			KnownBug(t, "NEW-sales-stock-edit-race", fmt.Sprintf("re-adding a line right after removing it left stock at %v, want 6", got), true)
		}
	})
	t.Run("missing and foreign ids", func(t *testing.T) {
		const none = "aaaaaaaaaaaaaaaaaaaaaaaa"
		for _, r := range []Resp{
			Call(t, "GET", "/sales/"+none, s.Token, nil),
			Call(t, "PATCH", "/sales/"+none, s.Token, M{"remarks": "x"}),
			Call(t, "PUT", "/sales/"+none, s.Token, M{"remarks": "x"}),
			Call(t, "DELETE", "/sales/"+none, s.Token, nil),
			Call(t, "POST", "/sales/"+none+"/restore", s.Token, nil),
			Call(t, "GET", "/sales/"+id, o.Token, nil),
			Call(t, "PATCH", "/sales/"+id, o.Token, M{"remarks": "x"}),
		} {
			if r.Code != 404 {
				t.Errorf("want 404, got %s", r)
			}
		}
		if r := PatchResp(t, s.Token, "sales", id, M{"storeId": o.ID}); r.Code != 403 {
			t.Errorf("move to another store: %s", r)
		}
		if r := Call(t, "GET", "/sales?storeId="+s.ID, o.Token, nil); r.Code != 403 {
			t.Errorf("list of another company's store: %s", r)
		}
		if r := Call(t, "GET", "/sales/"+id, "", nil); r.Code != 401 {
			t.Errorf("no token: %s", r)
		}
	})
}

// TestSalesListAndStats: invoice numbers, list filters / sort / select /
// search / paging, and /sales/stats against the oracle.
func TestSalesListAndStats(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 1, 2, 1000)
	pid := S(p["id"])
	ar := Create(t, s.Token, "customers", M{"storeId": s.ID, "nameEn": "Zeta Buyer " + Uniq(), "nameAr": "مشتري " + Digits(6), "phone": "05" + Digits(8)})
	other := s.Customer(t, "")
	yesterday := time.Now().In(s.Loc).AddDate(0, 0, -1)
	type doc struct {
		code, id, cust, day string
		o                   salesTot
		paid                int64
	}
	var docs []doc
	mk := func(cust interface{}, date time.Time, qty, price, paid float64) {
		b := salesBody(s, cust, []M{s.Line(pid, qty, price)}, nil)
		b["date"] = date.Format("2006-01-02T15:04")
		if paid > 0 {
			b["payments"] = []M{{"amount": paid, "method": "cash", "date": b["date"]}}
		}
		r := Create(t, s.Token, "sales", b)
		docs = append(docs, doc{code: S(r["code"]), id: S(r["id"]), cust: S(r["customerId"]), day: date.Format("2006-01-02"),
			o: salesOracle([]salesLn{{qty, price, 0}}, 0, 0, 15), paid: Cents(paid)})
	}
	mk(ar["id"], yesterday, 3, 12.34, 0)
	mk(ar["id"], yesterday, 1, 99.99, 50)
	mk(other["id"], time.Now().In(s.Loc), 2, 0.7, 1.61)
	mk(nil, time.Now().In(s.Loc), 7, 5.55, 0)
	mk(other["id"], time.Now().In(s.Loc), 1, 250, 100)

	t.Run("invoice numbers are sequential and unique", func(t *testing.T) {
		for i, d := range docs {
			if n := salesCodeNo(t, d.code, "S-INV-"); n != i+1 {
				t.Errorf("sale %d has code %s", i+1, d.code)
			}
		}
	})

	sum := func(f func(d doc) bool) (n int, net, vat, paid, bal int64) {
		for _, d := range docs {
			if f(d) {
				n++
				net += d.o.net
				vat += d.o.vat
				paid += d.paid
				bal += d.o.net - d.paid
			}
		}
		return
	}
	statsCheck := func(t *testing.T, qs string, f func(d doc) bool) {
		t.Helper()
		r := Must(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&sum=net,vat,paid,balance,one"+qs, s.Token, nil), 200, "stats"+qs).Body
		n, net, vat, paid, bal := sum(f)
		if int(Num(r["count"])) != n || int(F(r, "sums.one")) != n {
			t.Errorf("stats%s: count %v, want %d", qs, r["count"], n)
		}
		salesEqC(t, "stats net"+qs, F(r, "sums.net"), net)
		salesEqC(t, "stats vat"+qs, F(r, "sums.vat"), vat)
		salesEqC(t, "stats paid"+qs, F(r, "sums.paid"), paid)
		salesEqC(t, "stats balance"+qs, F(r, "sums.balance"), bal)
	}

	t.Run("stats", func(t *testing.T) {
		all := func(doc) bool { return true }
		statsCheck(t, "", all)
		statsCheck(t, "&from="+yesterday.Format("2006-01-02")+"&to="+yesterday.Format("2006-01-02"), func(d doc) bool { return d.day == yesterday.Format("2006-01-02") })
		statsCheck(t, "&from="+s.Today(), func(d doc) bool { return d.day == s.Today() })
		statsCheck(t, "&f.pstatus=paid_partially", func(d doc) bool { return d.paid > 0 && d.paid < d.o.net })
		statsCheck(t, "&q="+S(docs[3].code), func(d doc) bool { return d.code == docs[3].code })
		r := Must(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&sum=net,balance&groupBy=party", s.Token, nil), 200, "groupBy").Body
		for _, cust := range []string{S(ar["id"]), S(other["id"])} {
			_, net, _, _, bal := sum(func(d doc) bool { return d.cust == cust })
			salesEqC(t, "group net "+cust, F(r, "groups."+cust+".sums.net"), net)
			salesEqC(t, "group balance "+cust, F(r, "groups."+cust+".sums.balance"), bal)
		}
		r = Must(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&lines=payments&sum=amount&groupBy=method", s.Token, nil), 200, "payment lines").Body
		_, _, _, paid, _ := sum(all)
		salesEqC(t, "payment lines", F(r, "sums.amount"), paid)
		salesEqC(t, "cash payments", F(r, "groups.cash.sums.amount"), paid)
		if int(Num(r["count"])) != 3 {
			t.Errorf("payment lines count %v", r["count"])
		}
		r = Must(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&sum=one&page=2&limit=2&sort=code", s.Token, nil), 200, "stats page").Body
		if ids := Get(r, "ids"); len(ids.([]interface{})) != 2 || S(Get(r, "ids.0")) != docs[2].id {
			t.Errorf("stats page 2: %v", ids)
		}
	})

	t.Run("stats errors", func(t *testing.T) {
		for qs, field := range map[string]string{
			"&from=10-10-2026": "from", "&to=2026-02-30": "to", "&from=2026-10-10&to=2026-10-01": "from",
			"&lines=items": "lines", "&page=0": "page", "&page=1&limit=100000": "limit",
		} {
			salesWantErr(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&sum=net"+qs, s.Token, nil), 400, field)
		}
		salesWantErr(t, Call(t, "GET", "/sales/stats?sum=net", s.Token, nil), 400, "storeId")
		if r := Call(t, "GET", "/sales/stats?storeId="+s.ID, "", nil); r.Code != 401 {
			t.Errorf("stats without token: %s", r)
		}
	})

	t.Run("list filters sort select search paging", func(t *testing.T) {
		rows := List(t, s.Token, "sales", "storeId="+s.ID+"&sort=-netTotal&select=code,legacyTotals")
		want := append([]doc(nil), docs...)
		sort.SliceStable(want, func(i, j int) bool { return want[i].o.net > want[j].o.net })
		if len(rows) != len(want) {
			t.Fatalf("%d rows", len(rows))
		}
		for i, r := range rows {
			if r["code"] != want[i].code {
				t.Errorf("sort -netTotal row %d: %v, want %s", i, r["code"], want[i].code)
			}
			if r["items"] != nil || r["customerName"] != nil {
				t.Errorf("select returned more fields: %v", r)
			}
		}
		rows = List(t, s.Token, "sales", "storeId="+s.ID+"&sort=code&limit=2&page=3")
		if len(rows) != 1 || rows[0]["code"] != docs[4].code {
			t.Errorf("page 3 of 2: %v", rows)
		}
		r := Must(t, Call(t, "GET", "/sales?storeId="+s.ID+"&limit=2&page=1&sort=code", s.Token, nil), 200, "page").Body
		if int(Num(r["total"])) != 5 || len(Objs(r["data"])) != 2 {
			t.Errorf("paging total %v rows %d", r["total"], len(Objs(r["data"])))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&where.customerId="+S(ar["id"])); len(rows) != 2 {
			t.Errorf("customer filter: %d rows", len(rows))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&q="+S(docs[1].code)); len(rows) != 1 || rows[0]["code"] != docs[1].code {
			t.Errorf("q=code: %d rows", len(rows))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&q="+strings.ReplaceAll(S(ar["nameAr"]), " ", "%20")); len(rows) != 2 {
			t.Errorf("q=Arabic name: %d rows", len(rows))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&q=zeta%20buyer"); len(rows) != 2 {
			t.Errorf("q=name (any case): %d rows", len(rows))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&from="+s.Today()); len(rows) != 3 {
			t.Errorf("from today: %d rows", len(rows))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&where.paymentStatus=paid_partially"); len(rows) != 2 {
			t.Errorf("paid_partially: %d rows", len(rows))
		}
		if rows := List(t, s.Token, "sales", "storeId="+s.ID+"&q=no-such-invoice"); len(rows) != 0 {
			t.Errorf("q without a match: %d rows", len(rows))
		}
	})

	t.Run("list errors", func(t *testing.T) {
		for qs, field := range map[string]string{"&limit=0": "limit", "&limit=x": "limit", "&page=0": "page", "&sort=bogus": "sort", "&from=yesterday": "from"} {
			salesWantErr(t, Call(t, "GET", "/sales?storeId="+s.ID+qs, s.Token, nil), 400, field)
		}
		salesWantErr(t, Call(t, "GET", "/sales", s.Token, nil), 400, "storeId")
	})
}

// TestSalesConcurrency_ParallelCreates: 10 sales of one product at once.
func TestSalesConcurrency_ParallelCreates(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 10, 20, 100)
	pid := S(p["id"])
	const n = 10
	var wg sync.WaitGroup
	res := make([]Resp, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res[i] = Call(t, "POST", "/sales", s.Token, salesBody(s, nil, []M{s.Line(pid, float64(i+1), 20)}, []M{salesPay(1, "cash")}))
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{}
	nums := []int{}
	for i, r := range res {
		if r.Code != 201 {
			t.Errorf("sale %d: %s", i, r)
			continue
		}
		code := S(r.Body["code"])
		if seen[code] {
			t.Errorf("duplicate invoice code %s", code)
		}
		seen[code] = true
		nums = append(nums, salesCodeNo(t, code, "S-INV-"))
		salesEqC(t, "net", F(r.Body, "legacyTotals.net"), salesOracle([]salesLn{{float64(i + 1), 20, 0}}, 0, 0, 15).net)
	}
	sort.Ints(nums)
	for i, v := range nums {
		if v != i+1 {
			t.Errorf("invoice numbers %v, want 1..%d", nums, n)
			break
		}
	}
	salesWaitStock(t, s, pid, s.MS, 100-55)
	if c := salesCount(t, s, "sales"); c != n {
		t.Errorf("%d sales stored", c)
	}
	r := Must(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&sum=paid,one", s.Token, nil), 200, "stats").Body
	if F(r, "sums.one") != n || F(r, "sums.paid") != n {
		t.Errorf("stats after parallel sales: %v", r["sums"])
	}
}

// TestSalesIdempotency: a replayed Idempotency-Key returns the first answer
// and does not create a second sale.
func TestSalesIdempotency(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 10, 20, 50)
	pid := S(p["id"])
	key := "sale-" + Uniq() + Digits(6)
	body := salesBody(s, nil, []M{s.Line(pid, 2, 20)}, []M{salesPay(10, "cash")})

	first := Must(t, Call(t, "POST", "/sales", s.Token, body, "Idempotency-Key", key), 201, "first")
	if first.Header.Get("Idempotent-Replayed") != "" {
		t.Errorf("first call marked replayed")
	}
	again := Must(t, Call(t, "POST", "/sales", s.Token, body, "Idempotency-Key", key), 201, "replay")
	if again.ID() != first.ID() || again.Body["code"] != first.Body["code"] || again.Header.Get("Idempotent-Replayed") != "true" {
		t.Errorf("replay: %s vs %s (replayed=%q)", again.ID(), first.ID(), again.Header.Get("Idempotent-Replayed"))
	}
	// a different body with the same key replays the first answer too
	diff := Must(t, Call(t, "POST", "/sales", s.Token, salesBody(s, nil, []M{s.Line(pid, 9, 20)}, nil), "Idempotency-Key", key), 201, "same key, other body")
	if diff.ID() != first.ID() || F(diff.Body, "items.0.qty") != 2 {
		t.Errorf("same key, other body: %s", diff)
	}
	// parallel retries of a new key run once
	key2 := key + "-p"
	var wg sync.WaitGroup
	ids := make([]string, 5)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i] = Call(t, "POST", "/sales", s.Token, salesBody(s, nil, []M{s.Line(pid, 1, 20)}, nil), "Idempotency-Key", key2).ID()
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Errorf("parallel retries made different sales: %v", ids)
			break
		}
	}
	// a refused request is pinned as well
	key3 := key + "-e"
	salesWantErr(t, Call(t, "POST", "/sales", s.Token, M{"storeId": s.ID, "items": []M{s.Line(pid, 1, 20)}}, "Idempotency-Key", key3), 400, "date")
	salesWantErr(t, Call(t, "POST", "/sales", s.Token, body, "Idempotency-Key", key3), 400, "date")
	// a new key is a new sale
	Must(t, Call(t, "POST", "/sales", s.Token, body, "Idempotency-Key", key+"-n"), 201, "new key")

	if n := salesCount(t, s, "sales"); n != 3 {
		t.Errorf("%d sales, want 3", n)
	}
	salesWaitStock(t, s, pid, s.MS, 50-2-1-2)

	// the key is scoped to the user only, not to the endpoint
	r := Call(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": first.ID(),
		"items": []M{s.Line(pid, 1, 20)}}, "Idempotency-Key", key)
	KnownBug(t, "NEW-idem-scope", "an Idempotency-Key first used on POST /sales replays that sale (201, S-INV code) on POST /sales-returns; no return is created",
		r.Header.Get("Idempotent-Replayed") == "true" && r.ID() == first.ID())
}

// TestSalesReturns_Flow: partial and full returns, the remaining quantity,
// refunds, stock back in, customer balance, edits, delete / restore, stats.
func TestSalesReturns_Flow(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	o := Signup(t, "")
	a := s.Product(t, 10, 20, 50)
	b := s.Product(t, 3, 7, 10)
	x := s.Product(t, 1, 1, 1)
	aid, bid := S(a["id"]), S(b["id"])
	c := s.Customer(t, "")
	cid := c["id"]
	sale := Create(t, s.Token, "sales", salesBody(s, cid, []M{s.Line(aid, 3, 10), s.Line(bid, 2, 7)}, []M{salesPay(20, "cash")}))
	sid := S(sale["id"])
	saleNet := salesOracle([]salesLn{{3, 10, 0}, {2, 7, 0}}, 0, 0, 15).net
	receivable := saleNet - 2000
	salesWaitBalance(t, s, cid, receivable)
	salesWaitStock(t, s, aid, s.MS, 47)

	ret := func(items []M, pays []M) M {
		m := M{"storeId": s.ID, "date": s.Now(), "orderId": sid, "customerId": cid, "items": items}
		if pays != nil {
			m["payments"] = pays
		}
		return m
	}
	var live []salesRDoc

	var r1 M
	t.Run("partial return with refund", func(t *testing.T) {
		r1 = Create(t, s.Token, "sales-returns", ret([]M{s.Line(aid, 1, 10)}, []M{salesPay(11.5, "cash")}))
		o := salesOracle([]salesLn{{1, 10, 0}}, 0, 0, 15)
		salesEqC(t, "return net", F(r1, "legacyTotals.net"), o.net)
		salesEqC(t, "return vat", F(r1, "legacyTotals.vat"), o.vat)
		salesEqC(t, "return balance", F(r1, "legacyTotals.balance"), 0)
		if salesCodeNo(t, S(r1["code"]), "SR-INV-") != 1 || r1["orderId"] != sid || r1["orderCode"] != sale["code"] ||
			S(Get(r1, "zatca.invoiceType")) != "credit-simplified" {
			t.Errorf("return: code %v order %v/%v zatca %v", r1["code"], r1["orderId"], r1["orderCode"], Get(r1, "zatca.invoiceType"))
		}
		live = append(live, salesRDoc{S(r1["id"]), o.net, o.net})
		salesWaitStock(t, s, aid, s.MS, 48)
		sv := Read(t, s.Token, "sales", sid)
		if F(sv, "items.0.qtyReturned") != 1 || Get(sv, "items.1.qtyReturned") != nil {
			t.Errorf("sale lines after return: %v", sv["items"])
		}
		// the sale's own totals do not change
		salesEqC(t, "sale net", F(sv, "legacyTotals.net"), saleNet)
		salesWaitBalance(t, s, cid, receivable)
	})
	t.Run("more than sold is refused", func(t *testing.T) {
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(aid, 3, 10)}, nil)), 400, "items.0.qty")
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(bid, 2.5, 7)}, nil)), 400, "items.0.qty")
	})
	var r2 M
	t.Run("second return limited to the remainder", func(t *testing.T) {
		r2 = Create(t, s.Token, "sales-returns", ret([]M{s.Line(aid, 2, 10)}, nil))
		o := salesOracle([]salesLn{{2, 10, 0}}, 0, 0, 15)
		live = append(live, salesRDoc{S(r2["id"]), o.net, 0})
		receivable -= o.net
		salesWaitStock(t, s, aid, s.MS, 50)
		salesWaitBalance(t, s, cid, receivable)
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(aid, 0.5, 10)}, nil)), 400, "items.0.qty")
		if n := salesCodeNo(t, S(r2["code"]), "SR-INV-"); n != 2 {
			t.Errorf("second return code %v", r2["code"])
		}
	})
	t.Run("return payment limits", func(t *testing.T) {
		// refund above the return's net
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(bid, 1, 7)}, []M{salesPay(8.06, "cash")})), 400, "payments")
		// refund above what the customer paid on the sale (20 − 11.50 refunded)
		r := Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(bid, 2, 7)}, []M{salesPay(8.51, "cash")}))
		salesWantErr(t, r, 400, "payments")
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(bid, 1, 7)}, []M{salesPay(1, "gold")})), 400, "payments.0.method")
	})
	var r3 M
	t.Run("full return of the other line with a partial refund", func(t *testing.T) {
		r3 = Create(t, s.Token, "sales-returns", ret([]M{s.Line(bid, 2, 7)}, []M{salesPay(8.5, "bank_transfer")}))
		o := salesOracle([]salesLn{{2, 7, 0}}, 0, 0, 15)
		salesEqC(t, "balance", F(r3, "legacyTotals.balance"), o.net-850)
		if S(Get(r3, "legacyTotals.paymentStatus")) != "paid_partially" {
			t.Errorf("status %v", Get(r3, "legacyTotals.paymentStatus"))
		}
		live = append(live, salesRDoc{S(r3["id"]), o.net, 850})
		receivable -= o.net - 850
		salesWaitStock(t, s, bid, s.MS, 10)
		salesWaitBalance(t, s, cid, receivable)
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(bid, 1, 7)}, nil)), 400, "items.0.qty")
	})
	t.Run("validation", func(t *testing.T) {
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "items": []M{s.Line(aid, 1, 10)}}), 400, "orderId")
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": "no-such-sale", "items": []M{s.Line(aid, 1, 10)}}), 400, "orderId")
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{}, nil)), 400, "items")
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, ret([]M{s.Line(aid, 0, 10)}, nil)), 400, "items.0.qty")
		salesWantErr(t, Call(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "orderId": sid, "items": []M{s.Line(aid, 1, 10)}}), 400, "date")
		if r := Call(t, "POST", "/sales-returns", s.Token, M{"storeId": o.ID, "date": s.Now(), "orderId": sid, "items": []M{s.Line(aid, 1, 10)}}); r.Code != 403 {
			t.Errorf("another store: %s", r)
		}
		if r := Call(t, "POST", "/sales-returns", "", ret([]M{s.Line(aid, 1, 10)}, nil)); r.Code != 401 {
			t.Errorf("no token: %s", r)
		}
	})

	// corner cases on a second sale
	sale2 := Create(t, s.Token, "sales", salesBody(s, cid, []M{s.Line(aid, 2, 10)}, nil))
	s2 := S(sale2["id"])
	receivable += salesOracle([]salesLn{{2, 10, 0}}, 0, 0, 15).net
	salesWaitBalance(t, s, cid, receivable)
	t.Run("a product that is not on the invoice", func(t *testing.T) {
		r := Call(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": s2, "customerId": cid, "items": []M{s.Line(S(x["id"]), 1, 1)}})
		KnownBug(t, "NEW-sr-foreign-line", "a return of a product that is not on the invoice is accepted (201) and puts stock in; want 400 items.0.productId",
			r.Code == 201)
		if r.Code == 201 {
			salesWaitStock(t, s, x["id"], s.MS, 2)
			n := salesOracle([]salesLn{{1, 1, 0}}, 0, 0, 15).net
			live = append(live, salesRDoc{r.ID(), n, 0})
			receivable -= n
		}
	})
	t.Run("a refund price above the invoice price", func(t *testing.T) {
		r := Call(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": s2, "customerId": cid, "items": []M{s.Line(aid, 1, 15)}})
		KnownBug(t, "NEW-sr-price-above-sale", "a return line priced 15.00 for a line sold at 10.00 is accepted (201): the customer is credited more than was charged",
			r.Code == 201)
		if r.Code == 201 {
			n := salesOracle([]salesLn{{1, 15, 0}}, 0, 0, 15).net
			live = append(live, salesRDoc{r.ID(), n, 0})
			receivable -= n
		}
	})
	t.Run("a return without customerId", func(t *testing.T) {
		r := Create(t, s.Token, "sales-returns", M{"storeId": s.ID, "date": s.Now(), "orderId": s2, "items": []M{s.Line(aid, 1, 10)}})
		n := salesOracle([]salesLn{{1, 10, 0}}, 0, 0, 15).net
		live = append(live, salesRDoc{S(r["id"]), n, 0})
		KnownBug(t, "NEW-sr-no-customer", "a return created without customerId is not linked to the sale's customer (customerId null), so the customer's receivable is not reduced",
			r["customerId"] == nil)
		if r["customerId"] != nil {
			receivable -= n
		}
		salesWaitBalance(t, s, cid, receivable)
	})

	t.Run("PATCH and PUT of a return", func(t *testing.T) {
		id := S(r2["id"])
		// 2 → 1 returned: one unit goes back out of stock, the customer owes it again
		r := Patch(t, s.Token, "sales-returns", id, M{"items": []M{s.Line(aid, 1, 10)}})
		on := salesOracle([]salesLn{{1, 10, 0}}, 0, 0, 15).net
		salesEqC(t, "edited return net", F(r, "legacyTotals.net"), on)
		receivable += live[1].net - on
		live[1].net = on
		salesWaitStock(t, s, aid, s.MS, 49)
		salesWaitBalance(t, s, cid, receivable)
		// back up to the remainder of the sale (3 sold, 1 returned elsewhere)
		salesWantErr(t, PatchResp(t, s.Token, "sales-returns", id, M{"items": []M{s.Line(aid, 3, 10)}}), 400, "items.0.qty")
		if r := Call(t, "PATCH", "/sales-returns/"+id, s.Token, M{"remarks": "x"}, "If-Match", "1"); r.Code != 409 {
			t.Errorf("stale If-Match: %s", r)
		}
		cur := Read(t, s.Token, "sales-returns", id)
		cur["remarks"] = "مرتجع"
		pr := Must(t, Call(t, "PUT", "/sales-returns/"+id, s.Token, cur), 200, "PUT return").Body
		if pr["remarks"] != "مرتجع" || Num(pr["version"]) != Num(cur["version"])+1 {
			t.Errorf("PUT: %v v%v", pr["remarks"], pr["version"])
		}
		salesWaitStock(t, s, aid, s.MS, 49)
	})
	t.Run("PATCH adding a line to a return", func(t *testing.T) {
		id := S(r1["id"])
		r, err := salesTry(t, "PATCH", "/sales-returns/"+id, s.Token, M{"items": []M{s.Line(aid, 1, 10), s.Line(bid, 1, 7)}}, 20*time.Second)
		KnownBug(t, "NEW-sr-update-panic", "PATCH of a return with more lines than before panics (index out of range in SalesReturn.Validate) and drops the connection",
			err != nil)
		if err == nil && r.Code >= 500 {
			t.Errorf("PATCH adding a line: %s", r)
		}
	})

	t.Run("delete and restore a return", func(t *testing.T) {
		id := S(r3["id"])
		if r := Call(t, "DELETE", "/sales-returns/"+id, s.Token, nil, "If-Match", "99"); r.Code != 409 {
			t.Errorf("DELETE with stale If-Match: %s", r)
		}
		d := Must(t, Call(t, "DELETE", "/sales-returns/"+id, s.Token, nil), 200, "delete return").Body
		if d["deleted"] != true {
			t.Errorf("deleted %v", d["deleted"])
		}
		salesWaitStock(t, s, bid, s.MS, 8)
		g := Read(t, s.Token, "sales-returns", id)
		if g["deleted"] != true {
			t.Errorf("GET of a deleted return: deleted=%v", g["deleted"])
		}
		for _, rw := range List(t, s.Token, "sales-returns", "storeId="+s.ID) {
			if rw["id"] == id {
				t.Errorf("the deleted return is listed")
			}
		}
		found := false
		for _, rw := range List(t, s.Token, "sales-returns", "storeId="+s.ID+"&includeDeleted=1") {
			found = found || rw["id"] == id
		}
		if !found {
			t.Errorf("includeDeleted=1 does not list the deleted return")
		}
		salesStatsReturns(t, s, live, id)
		// deleting again is a no-op
		Must(t, Call(t, "DELETE", "/sales-returns/"+id, s.Token, nil), 200, "delete again")
		rs := Must(t, Call(t, "POST", "/sales-returns/"+id+"/restore", s.Token, nil), 200, "restore").Body
		if rs["deleted"] != false {
			t.Errorf("restored deleted=%v", rs["deleted"])
		}
		salesWaitStock(t, s, bid, s.MS, 10)
		salesStatsReturns(t, s, live, "")
		h := Objs(rs["history"])
		if len(h) < 3 || h[len(h)-1]["action"] != "restored" {
			t.Errorf("history %v", h)
		}
	})

	t.Run("stats and list errors", func(t *testing.T) {
		r := Must(t, Call(t, "GET", "/sales-returns/stats?storeId="+s.ID+"&sum=net&groupBy=party", s.Token, nil), 200, "groupBy").Body
		var net int64
		for _, d := range live[:5] {
			net += d.net
		}
		salesEqC(t, "returns of the customer", F(r, "groups."+S(cid)+".sums.net"), net)
		salesWantErr(t, Call(t, "GET", "/sales-returns/stats?storeId="+s.ID+"&from=2026-10-10&to=2026-01-01", s.Token, nil), 400, "from")
		salesWantErr(t, Call(t, "GET", "/sales-returns/stats", s.Token, nil), 400, "storeId")
		salesWantErr(t, Call(t, "GET", "/sales-returns?storeId="+s.ID+"&page=0", s.Token, nil), 400, "page")
		rows := List(t, s.Token, "sales-returns", "storeId="+s.ID+"&where.customerId="+S(cid)+"&sort=code&select=code,orderId")
		if len(rows) != 5 || rows[0]["orderId"] != sid || rows[0]["items"] != nil {
			t.Errorf("returns of the customer: %v", rows)
		}
		const none = "aaaaaaaaaaaaaaaaaaaaaaaa"
		for _, r := range []Resp{
			Call(t, "GET", "/sales-returns/"+none, s.Token, nil),
			Call(t, "PATCH", "/sales-returns/"+none, s.Token, M{"remarks": "x"}),
			Call(t, "PUT", "/sales-returns/"+none, s.Token, M{}),
			Call(t, "DELETE", "/sales-returns/"+none, s.Token, nil),
			Call(t, "POST", "/sales-returns/"+none+"/restore", s.Token, nil),
			Call(t, "GET", "/sales-returns/"+S(r1["id"]), o.Token, nil),
		} {
			if r.Code != 404 {
				t.Errorf("want 404, got %s", r)
			}
		}
	})
}

// salesStatsReturns compares /sales-returns/stats with the live returns (minus skip).
func salesStatsReturns(t *testing.T, s *Store, live []salesRDoc, skip string) {
	t.Helper()
	r := Must(t, Call(t, "GET", "/sales-returns/stats?storeId="+s.ID+"&sum=net,paid,balance,one", s.Token, nil), 200, "returns stats").Body
	var n int
	var net, paid int64
	for _, d := range live {
		if d.id == skip {
			continue
		}
		n++
		net += d.net
		paid += d.paid
	}
	if int(F(r, "sums.one")) != n {
		t.Errorf("returns stats count %v, want %d", Get(r, "sums.one"), n)
	}
	salesEqC(t, "returns stats net", F(r, "sums.net"), net)
	salesEqC(t, "returns stats paid", F(r, "sums.paid"), paid)
	salesEqC(t, "returns stats balance", F(r, "sums.balance"), net-paid)
}

// TestSalesReturns_UnknownOrder: a return naming a sale that does not exist.
func TestSalesReturns_UnknownOrder(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 1, 2, 5)
	pid := S(p["id"])
	r, err := salesTry(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": "aaaaaaaaaaaaaaaaaaaaaaaa",
		"items": []M{s.Line(pid, 1, 2)}}, 20*time.Second)
	KnownBug(t, "NEW-sr-unknown-order-panic", "POST /sales-returns with an orderId that is no sale of the store panics (nil order in SalesReturn.Validate) and drops the connection; want 400 orderId",
		err != nil)
	if err == nil {
		salesWantErr(t, r, 400, "orderId")
	}
	// the panic leaves the store's sales-return queue locked: later returns hang
	sale := Create(t, s.Token, "sales", salesBody(s, nil, []M{s.Line(pid, 1, 2)}, nil))
	r, err = salesTry(t, "POST", "/sales-returns", s.Token, M{"storeId": s.ID, "date": s.Now(), "orderId": sale["id"],
		"items": []M{s.Line(pid, 1, 2)}}, 5*time.Second)
	KnownBug(t, "NEW-sr-queue-stuck", "after that panic every later POST /sales-returns of the store hangs (the legacy per-store queue is never popped)",
		err != nil)
	if err == nil && r.Code != 201 {
		t.Errorf("valid return after the bad one: %s", r)
	}
}

// TestSalesPermissions: roles without sales rights.
func TestSalesPermissions(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	p := s.Product(t, 1, 2, 5)
	_, viewer := s.User(t, "r_viewer")
	if r := Call(t, "GET", "/sales?storeId="+s.ID, viewer, nil); r.Code != 200 {
		t.Errorf("viewer list: %s", r)
	}
	r := Call(t, "POST", "/sales", viewer, salesBody(s, nil, []M{s.Line(S(p["id"]), 1, 2)}, nil))
	if r.Code != 403 {
		t.Errorf("viewer create: %s", r)
	}
	_, cashier := s.User(t, "r_cashier")
	Must(t, Call(t, "POST", "/sales", cashier, salesBody(s, nil, []M{s.Line(S(p["id"]), 1, 2)}, nil)), 201, "cashier create")
}
