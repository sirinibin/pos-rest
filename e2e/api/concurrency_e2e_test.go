//go:build e2e

package api

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// Sales under concurrency: several users of the same store and users of other
// stores create sales at the same moment, then pay credit sales off at the
// same moment. Afterwards every store must have unique, gapless invoice
// numbers, stock deducted exactly once per line, the right payment status per
// sale, the right customer balances, and list stats and dashboards equal to
// an independent calculation. Helpers use the conc prefix.

type concSale struct {
	user     string
	customer string // "" = walk-in
	lines    []concLine
	pay      string // full, partial, none
	net, vat int64  // oracle, cents
	paid     int64
	id, code string
}

type concLine struct {
	product int
	qty     float64
}

type concStore struct {
	s        *Store
	users    map[string]string // role -> token
	products []string
	prices   []float64
	stock    []float64
	cust     []string
	sales    []*concSale
}

const concPartial = 500 // a partial payment, cents

// concSetup signs up a store with an owner, a manager, a salesman and a
// cashier, three products (the last one scarce) and two customers.
func concSetup(t *testing.T, country string) *concStore {
	t.Helper()
	cs := &concStore{s: Signup(t, country), users: map[string]string{}}
	cs.users["owner"] = cs.s.Token
	for _, role := range []string{"r_manager", "r_salesman", "r_cashier"} {
		_, cs.users[role] = cs.s.User(t, role)
	}
	cs.prices = []float64{20, 8, 3}
	cs.stock = []float64{1000, 1000, 10}
	for i, price := range cs.prices {
		cs.products = append(cs.products, S(cs.s.Product(t, price/2, price, cs.stock[i])["id"]))
	}
	for i := 0; i < 2; i++ {
		cs.cust = append(cs.cust, S(cs.s.Customer(t, "")["id"]))
	}
	return cs
}

// concPlan gives each user perSale sales: every sale sells one of the scarce
// product (so it is oversold by the whole burst), plus one or two other
// lines; walk-in sales pay in full, customer sales pay in full, partly or
// not at all.
func (cs *concStore) concPlan(perUser int) {
	vat := F(cs.s.Rec, "vatPercent")
	roles := []string{"owner", "r_manager", "r_salesman", "r_cashier"}
	for ui, role := range roles {
		for k := 0; k < perUser; k++ {
			n := ui*perUser + k
			sl := &concSale{user: role, lines: []concLine{{2, 1}, {n % 2, float64(1 + n%4)}}}
			if n%3 == 0 {
				sl.lines = append(sl.lines, concLine{1 - n%2, 2})
			}
			sl.pay = "full"
			if n%2 == 1 {
				sl.customer = cs.cust[(n/2)%2]
				sl.pay = []string{"full", "partial", "none"}[(n/2)%3]
			}
			var ls []salesLn
			for _, l := range sl.lines {
				ls = append(ls, salesLn{l.qty, cs.prices[l.product], 0})
			}
			o := salesOracle(ls, 0, 0, vat)
			sl.net, sl.vat = o.net, o.vat
			switch sl.pay {
			case "full":
				sl.paid = sl.net
			case "partial":
				sl.paid = concPartial
			}
			cs.sales = append(cs.sales, sl)
		}
	}
}

func (cs *concStore) concBody(sl *concSale) M {
	var items []M
	for _, l := range sl.lines {
		items = append(items, cs.s.Line(cs.products[l.product], l.qty, cs.prices[l.product]))
	}
	var cust interface{}
	if sl.customer != "" {
		cust = sl.customer
	}
	pays := []M{}
	if sl.paid > 0 {
		pays = append(pays, salesPay(float64(sl.paid)/100, []string{"cash", "bank_card", "bank_transfer"}[sl.net%3]))
	}
	return salesBody(cs.s, cust, items, pays)
}

func concStatus(paid, net int64) string {
	switch {
	case paid == 0:
		return "not_paid"
	case paid >= net:
		return "paid"
	}
	return "paid_partially"
}

// concRun starts every call at the same moment and waits for all of them.
func concRun(calls []func()) {
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, c := range calls {
		wg.Add(1)
		go func(c func()) {
			defer wg.Done()
			<-start
			c()
		}(c)
	}
	close(start)
	wg.Wait()
}

func TestConcurrency_SalesManyUsersManyStores(t *testing.T) {
	t.Parallel()
	const perUser = 6
	var stores []*concStore
	for _, c := range []string{"SA", "SA", "AE"} {
		cs := concSetup(t, c)
		cs.concPlan(perUser)
		stores = append(stores, cs)
	}

	// 1. every user of every store creates its sales at the same moment
	var calls []func()
	var mu sync.Mutex
	for _, cs := range stores {
		for _, sl := range cs.sales {
			cs, sl := cs, sl
			body := cs.concBody(sl)
			calls = append(calls, func() {
				r := Call(t, "POST", "/sales", cs.users[sl.user], body)
				mu.Lock()
				defer mu.Unlock()
				if r.Code != 201 {
					t.Errorf("store %s, %s: create sale: %s", cs.s.ID, sl.user, r)
					return
				}
				sl.id, sl.code = r.ID(), S(r.Body["code"])
				what := fmt.Sprintf("%s sale %s by %s", cs.s.Country, sl.code, sl.user)
				salesEqC(t, what+" net", F(r.Body, "legacyTotals.net"), sl.net)
				salesEqC(t, what+" vat", F(r.Body, "legacyTotals.vat"), sl.vat)
				salesEqC(t, what+" paid", F(r.Body, "legacyTotals.paid"), sl.paid)
				salesEqC(t, what+" balance", F(r.Body, "legacyTotals.balance"), sl.net-sl.paid)
				if got := S(Get(r.Body, "legacyTotals.paymentStatus")); got != concStatus(sl.paid, sl.net) {
					t.Errorf("%s: payment status %q, want %q", what, got, concStatus(sl.paid, sl.net))
				}
			})
		}
	}
	concRun(calls)
	if t.Failed() {
		t.FailNow()
	}

	for _, cs := range stores {
		cs := cs
		t.Run(cs.s.Country+" store "+cs.s.ID[len(cs.s.ID)-6:], func(t *testing.T) {
			concCheckStore(t, cs, "after the burst of sales")
		})
	}

	// 2. every user pays off the credit sales it made (a cashier may not edit
	// a sale, so the manager pays off the cashier's), all at the same moment,
	// so payments of the same customer's sales land concurrently
	calls = nil
	for _, cs := range stores {
		for _, sl := range cs.sales {
			if sl.paid == sl.net {
				continue
			}
			cs, sl := cs, sl
			rest := sl.net - sl.paid
			payer := sl.user
			if payer == "r_cashier" {
				payer = "r_manager"
			}
			calls = append(calls, func() {
				cur := Call(t, "GET", "/sales/"+sl.id, cs.users[payer], nil)
				if cur.Code != 200 {
					t.Errorf("%s reads %s: %s", payer, sl.code, cur)
					return
				}
				pays := append(Objs(cur.Body["payments"]), M{"amount": float64(rest) / 100, "method": "cash", "date": cs.s.Now()})
				r := Call(t, "PATCH", "/sales/"+sl.id, cs.users[payer], M{"payments": pays},
					"If-Match", Num(cur.Body["version"]).String(), "X-Change-Reason", "e2e")
				if r.Code != 200 {
					t.Errorf("%s pays off %s: %s", payer, sl.code, r)
					return
				}
				mu.Lock()
				sl.paid = sl.net
				mu.Unlock()
				salesEqC(t, sl.code+" balance after paying off", F(r.Body, "legacyTotals.balance"), 0)
				if S(Get(r.Body, "legacyTotals.paymentStatus")) != "paid" {
					t.Errorf("%s after paying off: %v", sl.code, Get(r.Body, "legacyTotals.paymentStatus"))
				}
			})
		}
	}
	if len(calls) < 6 {
		t.Fatalf("only %d credit sales to pay off; the plan should make more", len(calls))
	}
	concRun(calls)
	for _, cs := range stores {
		cs := cs
		t.Run(cs.s.Country+" store "+cs.s.ID[len(cs.s.ID)-6:]+" paid off", func(t *testing.T) {
			concCheckStore(t, cs, "after paying off concurrently")
		})
	}
}

// concCheckStore compares the store with the oracle: invoice numbers, stock,
// customer balances, list stats and the revenue and VAT dashboards.
func concCheckStore(t *testing.T, cs *concStore, when string) {
	s := cs.s
	t.Logf("%s: %d sales", when, len(cs.sales))

	// invoice numbers: unique and gapless per store, whatever the other
	// stores did at the same time
	var nums []int
	for _, sl := range cs.sales {
		nums = append(nums, salesCodeNo(t, sl.code, "S-INV-"))
	}
	sort.Ints(nums)
	for i, n := range nums {
		if n != i+1 {
			t.Errorf("invoice numbers %v, want 1..%d once each", nums, len(cs.sales))
			break
		}
	}
	if n := salesCount(t, s, "sales"); n != len(cs.sales) {
		t.Errorf("%d sales stored, want %d", n, len(cs.sales))
	}

	// stock goes out exactly once per line (the scarce product is oversold:
	// legacy allows negative stock)
	want := append([]float64(nil), cs.stock...)
	for _, sl := range cs.sales {
		for _, l := range sl.lines {
			want[l.product] -= l.qty
		}
	}
	if want[2] >= 0 {
		t.Fatalf("the plan should oversell the scarce product, want %v", want[2])
	}
	for i, pid := range cs.products {
		// each sale recomputes the product's stock and saves the whole product
		// after its own read, so a stale writer can finish last
		got, ok := salesStockSettles(t, s, pid, s.MS, want[i])
		concRace(t, "NEW-CONC-STOCK-MOVES", fmt.Sprintf("concurrent sales leave product stock %v, want %v (models/product.go:3349 whole-document $set after a stale recompute)", got, want[i]), !ok)
	}

	// customer balances: what the customer's sales still owe
	for _, cid := range cs.cust {
		var owed int64
		for _, sl := range cs.sales {
			if sl.customer == cid {
				owed += sl.net - sl.paid
			}
		}
		concCheckBalance(t, cs, cid, owed)
	}

	// list stats and dashboards against the oracle
	var net, vat, paid int64
	for _, sl := range cs.sales {
		net, vat, paid = net+sl.net, vat+sl.vat, paid+sl.paid
	}
	r := Must(t, Call(t, "GET", "/sales/stats?storeId="+s.ID+"&sum=net,vat,paid,balance,one", s.Token, nil), 200, "stats").Body
	salesEqC(t, "stats net", F(r, "sums.net"), net)
	salesEqC(t, "stats vat", F(r, "sums.vat"), vat)
	salesEqC(t, "stats paid", F(r, "sums.paid"), paid)
	salesEqC(t, "stats balance", F(r, "sums.balance"), net-paid)
	if F(r, "sums.one") != float64(len(cs.sales)) {
		t.Errorf("stats count %v, want %d", Get(r, "sums.one"), len(cs.sales))
	}
	now := time.Now().In(s.Loc)
	period := finPeriod(now.AddDate(0, 0, -1).Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02"))
	rev := finDash(t, s, "revenue", period)
	salesEqC(t, "dashboard revenue", F(rev, "result.revenue"), net)
	salesEqC(t, "dashboard revenue VAT", F(rev, "result.vat"), vat)
	vt := finDash(t, s, "vat", period)
	salesEqC(t, "dashboard output VAT", F(vt, "result.outVat"), vat)
}

// concCheckBalance waits for the customer's balance to equal what its sales
// still owe. Run one after another, the same sales and payments always give
// the right balance; at the same moment the customer's ledger account can
// end up wrong (a sale's ledger misses one of its payments while the sale and
// its payment records are right). One more edit on its own tells a stale
// cached balance from a wrong ledger. Both are open bugs, recorded with
// KnownBug when they reproduce (they don't on every run).
func concCheckBalance(t *testing.T, cs *concStore, cid string, owed int64) {
	t.Helper()
	got, ok := concBalanceSettles(t, cs.s, cid, owed)
	if ok {
		return
	}
	stale := got
	for _, sl := range cs.sales {
		if sl.customer == cid {
			cur := Read(t, cs.s.Token, "sales", sl.id)
			Patch(t, cs.s.Token, "sales", sl.id, M{"payments": cur["payments"]})
			break
		}
	}
	if got, ok = concBalanceSettles(t, cs.s, cid, owed); ok {
		KnownBug(t, "NEW-sales-balance-stale", fmt.Sprintf("edits of one customer's sales at the same moment leave its creditBalance stale (%.2f, want %.2f) until the next edit: "+
			"each edit recomputes it from the ledger in its own goroutine (controller/sales.go, customer.SetCreditBalance) and the last writer can hold an older read", stale, float64(owed)/100), true)
		return
	}
	var ledger float64
	if a := finAcctBy(finAccounts(t, cs.s), "referenceId", cid); a != nil {
		ledger = finSigned(a)
	}
	KnownBug(t, "NEW-sales-ledger-race", fmt.Sprintf("sales of one customer created or paid at the same moment leave the customer's ledger account wrong: "+
		"balance %.2f (ledger %.2f), want %.2f, and a later edit does not repair it; every sale and payment record is right, the sale's ledger misses a payment. "+
		"Sale accounting runs in an unsynchronised goroutine per create/edit (controller/sales.go DoAccounting: undo then redo by reference)", got, ledger, float64(owed)/100), true)
}

func concBalanceSettles(t *testing.T, s *Store, cid string, want int64) (float64, bool) {
	t.Helper()
	var got float64
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got = salesBalance(t, s, cid); Cents(got) == want {
			return got, true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return got, false
}
