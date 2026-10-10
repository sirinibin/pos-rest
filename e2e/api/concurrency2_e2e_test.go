//go:build e2e

package apie2e

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Concurrency across the rest of the write paths: purchases and their
// returns, quotation invoices and their returns, non-VAT sales and returns,
// delivery notes, stock transfers, expenses, capital, capital withdrawals,
// dividends, customer deposits and withdrawals, and new customers, vendors
// and products, all made by several users of two stores at the same moment.
//
// A third, control store gets the very same documents one at a time. Each
// busy store must end exactly where the control store does: same stock,
// same customer and vendor balances, same ledger account balances, the same
// number of documents with unique codes. On top, in every store each
// account's balance must match its own postings, and no account may exist
// twice. The race-db job runs this against a -race build of the server.

type concStore struct {
	sid, product, customer, vendor, category, warehouse, warehouseCode string
}

type concUser struct{ id, name, token string }

// concOp is one kind of document; body builds the n-th one a user makes.
type concOp struct {
	kind, path string
	body       func(s *concStore, n int) map[string]interface{}
}

func setupConcStore(t *testing.T, label string) *concStore {
	t.Helper()
	s := &concStore{sid: newStore(t)}
	s.product = createProduct(t, s.sid, "Busy Widget", 100, 60)
	s.customer = createCustomer(t, s.sid, "Busy Customer", map[string]interface{}{"credit_limit": 1000000})
	s.vendor = createVendor(t, s.sid, "Busy Vendor")
	code, res := in(t, s.sid, "POST", "/v1/expense-category", map[string]interface{}{"store_id": s.sid, "name": "Utilities"})
	s.category = str(mustOK(t, label+" expense category", code, res), "id")
	s.warehouse, s.warehouseCode = createWarehouse(t, s.sid, "Busy Warehouse")
	when := agoStr(2 * time.Hour)
	code, res = in(t, s.sid, "POST", "/v1/purchase", purchaseBody(s.sid, s.vendor, when,
		[]line{{id: s.product, name: "Busy Widget", qty: 1000, price: 60}}, payment(69000, when)))
	mustOK(t, label+" opening stock", code, res)
	return s
}

func concPhase1(admin string) []concOp {
	when := func() string { return agoStr(time.Hour) }
	return []concOp{
		{"purchase", "/v1/purchase", func(s *concStore, n int) map[string]interface{} {
			return purchaseBody(s.sid, s.vendor, when(), []line{{id: s.product, name: "Busy Widget", qty: 10, price: 60}}, payment(690, when()))
		}},
		{"quotation invoice", "/v1/quotation", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{
				"store_id": s.sid, "customer_id": s.customer, "date_str": when(), "vat_percent": 15, "type": "invoice",
				"validity_days": 7, "delivery_days": 3, "payment_status": "paid",
				"products":       []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 2, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
				"payments_input": []map[string]interface{}{payment(230, when())},
			}
		}},
		{"non-VAT sale", "/v1/non-vat-sales", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{
				"store_id": s.sid, "customer_id": s.customer, "date_str": when(), "vat_percent": 0,
				"products":       []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 2, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
				"payments_input": []map[string]interface{}{payment(200, when())},
			}
		}},
		{"delivery note", "/v1/delivery-note", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{
				"store_id": s.sid, "customer_id": s.customer, "date_str": when(), "vat_percent": 15,
				"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1, "unit_price": 100, "unit": "PC"}},
			}
		}},
		{"stock transfer", "/v1/stock-transfer", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{
				"store_id": s.sid, "from_warehouse_id": "", "to_warehouse_id": s.warehouse, "date_str": when(), "vat_percent": 15,
				"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1, "unit_price": 60, "unit": "PC"}},
			}
		}},
		{"expense", "/v1/expense", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "amount": 50, "description": "Electricity", "date_str": when(),
				"payment_method": "cash", "category_id": []string{s.category}}
		}},
		{"capital", "/v1/capital", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "amount": 1000, "date_str": when(), "description": "Top-up",
				"payment_method": "cash", "invested_by_user_id": admin}
		}},
		{"capital withdrawal", "/v1/capital-withdrawal", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "amount": 100, "date_str": when(), "description": "Drawing",
				"payment_method": "cash", "withdrawn_by_user_id": admin}
		}},
		{"dividend", "/v1/divident", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "amount": 100, "date_str": when(), "description": "Dividend",
				"payment_method": "cash", "withdrawn_by_user_id": admin}
		}},
		// Amounts differ per document: the same receipt twice within 30
		// seconds is refused as a double submit (see TestConcurrency_DoubleSubmit).
		{"customer deposit", "/v1/customer-deposit", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "type": "customer", "customer_id": s.customer, "date_str": when(),
				"payments": []map[string]interface{}{{"date_str": when(), "amount": 300 + n, "method": "cash"}}}
		}},
		{"customer withdrawal", "/v1/customer-withdrawal", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "type": "customer", "customer_id": s.customer, "date_str": when(),
				"payments": []map[string]interface{}{{"date_str": when(), "amount": 100 + n, "method": "cash"}}}
		}},
		{"customer", "/v1/customer", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "name": fmt.Sprintf("Walk-in %d", n)}
		}},
		{"vendor", "/v1/vendor", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "name": fmt.Sprintf("Supplier %d", n)}
		}},
		{"product", "/v1/product", func(s *concStore, n int) map[string]interface{} {
			return map[string]interface{}{"store_id": s.sid, "name": fmt.Sprintf("Gadget %d", n), "unit": "PC",
				"product_stores": map[string]interface{}{s.sid: map[string]interface{}{"store_id": s.sid, "retail_unit_price": 10, "purchase_unit_price": 5}}}
		}},
	}
}

// concPhase2 returns one unit of a phase-1 document; nil for kinds without a return.
func concPhase2(kind string) *concOp {
	switch kind {
	case "purchase":
		return &concOp{"purchase return", "/v1/purchase-return", nil}
	case "quotation invoice":
		return &concOp{"quotation sales return", "/v1/quotation-sales-return", nil}
	case "non-VAT sale":
		return &concOp{"non-VAT sales return", "/v1/non-vat-sales-return", nil}
	}
	return nil
}

func concReturnBody(kind string, s *concStore, docID, admin string) map[string]interface{} {
	switch kind {
	case "purchase return":
		return map[string]interface{}{"store_id": s.sid, "purchase_id": docID, "vendor_id": s.vendor, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{}, "purchase_returned_by": admin,
			"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1, "purchase_unit_price": 60,
				"purchasereturn_unit_price": 60, "unit": "PC", "selected": true}}}
	case "quotation sales return":
		return map[string]interface{}{"store_id": s.sid, "quotation_id": docID, "customer_id": s.customer, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
			"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1, "unit_price": 100,
				"purchase_unit_price": 60, "unit": "PC", "selected": true}}}
	default: // non-VAT sales return
		return map[string]interface{}{"store_id": s.sid, "non_vat_sales_id": docID, "customer_id": s.customer, "date_str": nowStr(),
			"vat_percent": 0, "payment_status": "not_paid", "payments_input": []interface{}{},
			"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1, "unit_price": 100,
				"purchase_unit_price": 60, "unit": "PC", "selected": true}}}
	}
}

type concResult struct {
	kind, path string
	doc        concDoc
}

// concSnapshot is where a store ended up.
type concSnapshot struct {
	stock, warehouseStock, customerBalance, vendorBalance float64
	accounts                                              map[string][3]float64 // name -> debit, credit, balance
	counts                                                map[string]int
	byModel                                               map[string][2]float64 // account name / reference model -> debit, credit
}

func takeConcSnapshot(t *testing.T, s *concStore, kinds map[string]string) concSnapshot {
	t.Helper()
	snap := concSnapshot{accounts: map[string][3]float64{}, counts: map[string]int{}, byModel: map[string][2]float64{}}
	snap.stock = stockOf(t, s.sid, s.product)
	snap.warehouseStock = warehouseStock(t, s.sid, s.product, s.warehouseCode)
	code, res := in(t, s.sid, "GET", "/v1/customer/"+s.customer, nil)
	snap.customerBalance = num(mustOK(t, "customer", code, res), "credit_balance")
	code, res = in(t, s.sid, "GET", "/v1/vendor/"+s.vendor, nil)
	snap.vendorBalance = num(mustOK(t, "vendor", code, res), "credit_balance")

	// Every account once, with a balance that matches its postings.
	code, res = in(t, s.sid, "GET", "/v1/account?limit=1000", nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("store %s: list accounts HTTP %d %v", s.sid, code, res.Errors)
	}
	accounts := resultList(t, res)
	code, res = in(t, s.sid, "GET", "/v1/posting?limit=5000", nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("store %s: list postings HTTP %d %v", s.sid, code, res.Errors)
	}
	posted := map[string][2]float64{}
	rows := resultList(t, res)
	if len(rows) >= 5000 {
		t.Fatalf("store %s: 5000+ postings; raise the limit", s.sid)
	}
	for _, p := range rows {
		k := str(p, "account_name") + " / " + str(p, "reference_model")
		v2 := snap.byModel[k]
		v2[0] += num(p, "debit_total")
		v2[1] += num(p, "credit_total")
		snap.byModel[k] = v2
		v := posted[str(p, "account_id")]
		v[0] += num(p, "debit_total")
		v[1] += num(p, "credit_total")
		posted[str(p, "account_id")] = v
	}
	for _, a := range accounts {
		name := str(a, "name")
		if _, dup := snap.accounts[name]; dup {
			t.Errorf("store %s: two accounts named %q", s.sid, name)
		}
		d, c, b := num(a, "debit_total"), num(a, "credit_total"), num(a, "balance")
		p := posted[str(a, "id")]
		if !approx(d, p[0]) || !approx(c, p[1]) || !approx(b, math.Abs(p[0]-p[1])) {
			t.Errorf("store %s: account %q shows debit %.2f credit %.2f balance %.2f, its postings add up to debit %.2f credit %.2f",
				s.sid, name, d, c, b, p[0], p[1])
		}
		snap.accounts[name] = [3]float64{p[0], p[1], math.Abs(p[0] - p[1])}
	}
	for kind, path := range kinds {
		code, res := in(t, s.sid, "GET", path+"?limit=1000", nil)
		if code != http.StatusOK || !res.Status {
			t.Fatalf("store %s: list %s HTTP %d %v", s.sid, kind, code, res.Errors)
		}
		snap.counts[kind] = len(resultList(t, res))
	}
	return snap
}

func compareConcSnapshots(t *testing.T, sid string, got, want concSnapshot) {
	t.Helper()
	diff := func(what string, g, w float64) {
		if !approx(g, w) {
			t.Errorf("store %s: %s is %.2f, but %.2f when the same documents come one at a time", sid, what, g, w)
		}
	}
	diff("product stock", got.stock, want.stock)
	diff("warehouse stock", got.warehouseStock, want.warehouseStock)
	diff("customer credit balance", got.customerBalance, want.customerBalance)
	diff("vendor credit balance", got.vendorBalance, want.vendorBalance)
	names := []string{}
	for n := range want.accounts {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		g, ok := got.accounts[n]
		if !ok {
			t.Errorf("store %s: no account %q", sid, n)
			continue
		}
		if !approx(g[0], want.accounts[n][0]) || !approx(g[1], want.accounts[n][1]) {
			diff("account "+n+" debits", g[0], want.accounts[n][0])
			diff("account "+n+" credits", g[1], want.accounts[n][1])
			for k, w := range want.byModel {
				if strings.HasPrefix(k, n+" / ") {
					if gm := got.byModel[k]; !approx(gm[0], w[0]) || !approx(gm[1], w[1]) {
						t.Logf("  %s: debit %.2f credit %.2f, one at a time %.2f / %.2f", k, gm[0], gm[1], w[0], w[1])
					}
				}
			}
		}
	}
	for n := range got.accounts {
		if _, ok := want.accounts[n]; !ok {
			t.Errorf("store %s: extra account %q", sid, n)
		}
	}
	for kind, n := range want.counts {
		if got.counts[kind] != n {
			t.Errorf("store %s: %d %s documents, want %d", sid, got.counts[kind], kind, n)
		}
	}
}

func TestConcurrency_AllWritePathsMatchOneAtATime(t *testing.T) {
	const (
		storesN       = 2
		usersPerStore = 3
		docsPerUser   = 2
	)
	admin := meID(t)
	ops := concPhase1(admin)
	kinds := map[string]string{}
	for _, op := range ops {
		kinds[op.kind] = op.path
	}
	for _, op := range ops {
		if r := concPhase2(op.kind); r != nil {
			kinds[r.kind] = r.path
		}
	}

	type busy struct {
		*concStore
		users []concUser
	}
	stores := make([]*busy, storesN)
	for i := range stores {
		b := &busy{concStore: setupConcStore(t, fmt.Sprintf("store %d", i))}
		for u := 0; u < usersPerStore; u++ {
			label := fmt.Sprintf("conc2-%d-s%d-u%d", time.Now().UnixNano()%1000000, i, u)
			id, token := staff(t, label, "Manager", []string{b.sid})
			b.users = append(b.users, concUser{id, "E2E " + label, token})
		}
		stores[i] = b
	}
	control := setupConcStore(t, "control")

	// Phase 1: every user of every store makes every kind of document at once.
	var mu sync.Mutex
	var made []concResult
	var fns []func()
	for _, s := range stores {
		for ui, u := range s.users {
			for _, op := range ops {
				for n := 0; n < docsPerUser; n++ {
					s, u, op, n := s, u, op, ui*docsPerUser+n
					fns = append(fns, func() {
						code, res := asUser(t, u.token, s.sid, "POST", op.path, op.body(s.concStore, n))
						if code != http.StatusOK || !res.Status {
							t.Errorf("store %s %s: %s HTTP %d %v", s.sid, u.name, op.kind, code, res.Errors)
							return
						}
						mu.Lock()
						made = append(made, concResult{op.kind, op.path, concDoc{s.sid, u.id, u.name, str(resultMap(t, res), "id")}})
						mu.Unlock()
					})
				}
			}
		}
	}
	startTogether(fns)

	// The control store gets the same documents one at a time.
	var controlMade []concResult
	for n := 0; n < usersPerStore*docsPerUser; n++ {
		for _, op := range ops {
			code, res := in(t, control.sid, "POST", op.path, op.body(control, n))
			m := mustOK(t, "control "+op.kind, code, res)
			controlMade = append(controlMade, concResult{op.kind, op.path, concDoc{control.sid, admin, "", str(m, "id")}})
		}
	}

	// Phase 2: every returnable document gets a one-unit return, all at once.
	tokenOf := map[string]string{}
	storeOf := map[string]*concStore{}
	for _, s := range stores {
		storeOf[s.sid] = s.concStore
		for _, u := range s.users {
			tokenOf[u.id] = u.token
		}
	}
	fns = nil
	phase1 := append([]concResult{}, made...)
	for _, d := range phase1 {
		r := concPhase2(d.kind)
		if r == nil {
			continue
		}
		d, r := d, r
		fns = append(fns, func() {
			code, res := asUser(t, tokenOf[d.doc.user], d.doc.store, "POST", r.path, concReturnBody(r.kind, storeOf[d.doc.store], d.doc.id, admin))
			if code != http.StatusOK || !res.Status {
				t.Errorf("store %s %s: %s of %s HTTP %d %v", d.doc.store, d.doc.userName, r.kind, d.doc.id, code, res.Errors)
				return
			}
			mu.Lock()
			made = append(made, concResult{r.kind, r.path, concDoc{d.doc.store, d.doc.user, d.doc.userName, str(resultMap(t, res), "id")}})
			mu.Unlock()
		})
	}
	startTogether(fns)
	for _, d := range append([]concResult{}, controlMade...) {
		if r := concPhase2(d.kind); r != nil {
			code, res := in(t, control.sid, "POST", r.path, concReturnBody(r.kind, control, d.doc.id, admin))
			mustOK(t, "control "+r.kind, code, res)
		}
	}

	// Each document: its store, its user, a code no other document of its
	// kind in the store has, and a balanced ledger where it posts one.
	codes := map[string]string{}
	for _, d := range made {
		code, res := in(t, d.doc.store, "GET", d.path+"/"+d.doc.id, nil)
		m := mustOK(t, "view "+d.kind, code, res)
		label := fmt.Sprintf("%s %s (store %s)", d.kind, d.doc.id, d.doc.store)
		if got := str(m, "store_id"); got != d.doc.store {
			t.Errorf("%s: store_id %s", label, got)
		}
		if got := str(m, "created_by"); got != "" && got != d.doc.user {
			t.Errorf("%s: created_by %s, want %s (%s)", label, got, d.doc.user, d.doc.userName)
		}
		if c := str(m, "code"); c != "" {
			key := d.doc.store + "/" + d.kind + "/" + c
			if other, dup := codes[key]; dup {
				t.Errorf("%s and %s share code %s", label, other, c)
			}
			codes[key] = d.doc.id
		} else if d.kind != "product" {
			t.Errorf("%s: no code", label)
		}
	}

	want := takeConcSnapshot(t, control, kinds)
	for _, s := range stores {
		s := s
		// Balances are recomputed after the response; give them a moment.
		for i := 0; i < 20; i++ {
			got := takeConcSnapshot(t, s.concStore, kinds)
			if approx(got.stock, want.stock) && approx(got.customerBalance, want.customerBalance) && approx(got.vendorBalance, want.vendorBalance) {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		compareConcSnapshots(t, s.sid, takeConcSnapshot(t, s.concStore, kinds), want)
	}
}

// Two requests creating a user with the same email at the same moment: one
// wins, the other is refused, and only one such user exists.
func TestConcurrency_SameEmailUserCreatedOnce(t *testing.T) {
	sid := newStore(t)
	mail := "e2e-twin-" + runID + "@startpos.test"
	var mu sync.Mutex
	ok := 0
	fns := make([]func(), 6)
	for i := range fns {
		fns[i] = func() {
			code, res := call(t, "POST", "/v1/user", authToken(t), map[string]interface{}{"name": "E2E Twin", "email": mail,
				"mob": "0501234572", "password": "Staff-Passw0rd!", "role": "Manager", "store_ids": []string{sid}})
			if code == http.StatusOK && res.Status {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if code >= 500 {
				t.Errorf("duplicate user create answered HTTP %d %v", code, res.Errors)
			}
		}
	}
	startTogether(fns)
	if ok != 1 {
		t.Errorf("%d of %d concurrent creates of %s succeeded, want 1", ok, len(fns), mail)
	}
	code, res := call(t, "GET", "/v1/user?search[email]="+mail+"&limit=50", authToken(t), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("list users: HTTP %d %v", code, res.Errors)
	}
	n := 0
	for _, u := range resultList(t, res) {
		if strings.EqualFold(str(u, "email"), mail) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d users with email %s, want 1", n, mail)
	}
}

// The same receipt or payment submitted several times at once (a double
// click, a retried request) is saved once; the rest are refused as
// duplicates, as they are when they come one after another.
func TestConcurrency_DoubleSubmittedReceiptSavedOnce(t *testing.T) {
	sid := newStore(t)
	customer := createCustomer(t, sid, "Double Click Customer", map[string]interface{}{"credit_limit": 100000})
	for _, path := range []string{"/v1/customer-deposit", "/v1/customer-withdrawal"} {
		var mu sync.Mutex
		ok, refused := 0, 0
		fns := make([]func(), 5)
		for i := range fns {
			fns[i] = func() {
				code, res := in(t, sid, "POST", path, map[string]interface{}{"store_id": sid, "type": "customer", "customer_id": customer,
					"date_str": nowStr(), "payments": []map[string]interface{}{{"date_str": nowStr(), "amount": 75, "method": "cash"}}})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case code == http.StatusOK && res.Status:
					ok++
				case code == http.StatusConflict && res.Errors["duplicate"] != "":
					refused++
				default:
					t.Errorf("%s: HTTP %d %v", path, code, res.Errors)
				}
			}
		}
		startTogether(fns)
		if ok != 1 || refused != len(fns)-1 {
			t.Errorf("%s: %d saved and %d refused of %d identical submits, want 1 saved", path, ok, refused, len(fns))
		}
	}
}

// Several users returning units of the same document at the same moment:
// six one-unit returns of a four-unit document. Exactly four may be saved
// (never more than was sold or bought), and the document counts all four.
func TestConcurrency_ReturnsOfOneDocumentNeverExceedIt(t *testing.T) {
	s := setupConcStore(t, "returns")
	admin := meID(t)
	when := agoStr(time.Hour)
	mk := func(kind, path string, body map[string]interface{}) string {
		code, res := in(t, s.sid, "POST", path, body)
		return str(mustOK(t, kind, code, res), "id")
	}
	sale := mk("sale", "/v1/order", saleBody(s.sid, s.customer, when, []line{{id: s.product, name: "Busy Widget", qty: 4, price: 100, cost: 60}}, payment(460, when)))
	purchase := mk("purchase", "/v1/purchase", purchaseBody(s.sid, s.vendor, when, []line{{id: s.product, name: "Busy Widget", qty: 4, price: 60}}, payment(276, when)))
	quotation := mk("quotation invoice", "/v1/quotation", map[string]interface{}{
		"store_id": s.sid, "customer_id": s.customer, "date_str": when, "vat_percent": 15, "type": "invoice",
		"validity_days": 7, "delivery_days": 3, "payment_status": "paid",
		"products":       []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 4, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
		"payments_input": []map[string]interface{}{payment(460, when)},
	})
	nonVAT := mk("non-VAT sale", "/v1/non-vat-sales", map[string]interface{}{
		"store_id": s.sid, "customer_id": s.customer, "date_str": when, "vat_percent": 0,
		"products":       []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 4, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
		"payments_input": []map[string]interface{}{payment(400, when)},
	})
	cases := []struct{ kind, path, parentPath, parentID string }{
		{"sales return", "/v1/sales-return", "/v1/order/", sale},
		{"purchase return", "/v1/purchase-return", "/v1/purchase/", purchase},
		{"quotation sales return", "/v1/quotation-sales-return", "/v1/quotation/", quotation},
		{"non-VAT sales return", "/v1/non-vat-sales-return", "/v1/non-vat-sales/", nonVAT},
	}
	for _, c := range cases {
		c := c
		t.Run(c.kind, func(t *testing.T) {
			var mu sync.Mutex
			saved := 0
			fns := make([]func(), 6)
			for i := range fns {
				fns[i] = func() {
					var body map[string]interface{}
					if c.kind == "sales return" {
						body = map[string]interface{}{"store_id": s.sid, "order_id": c.parentID, "customer_id": s.customer, "date_str": nowStr(),
							"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
							"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1, "unit_price": 100,
								"purchase_unit_price": 60, "unit": "PC", "selected": true}}}
					} else {
						body = concReturnBody(c.kind, s, c.parentID, admin)
					}
					code, res := in(t, s.sid, "POST", c.path, body)
					switch {
					case code == http.StatusOK && res.Status:
						mu.Lock()
						saved++
						mu.Unlock()
					case code >= 500:
						t.Errorf("%s: HTTP %d %v", c.kind, code, res.Errors)
					}
				}
			}
			startTogether(fns)
			if saved != 4 {
				t.Errorf("%d of 6 one-unit returns of a 4-unit document were saved, want 4", saved)
			}
			for i := 0; i < 20; i++ {
				code, res := in(t, s.sid, "GET", c.parentPath+c.parentID, nil)
				p := mustOK(t, "view "+c.kind+" parent", code, res)
				if _, has := p["return_count"]; !has || int(num(p, "return_count")) == saved {
					break
				}
				if i == 19 {
					t.Errorf("parent shows return_count %v after %d returns", p["return_count"], saved)
				}
				time.Sleep(250 * time.Millisecond)
			}
		})
	}
}
