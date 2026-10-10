//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Concurrency: many users creating sales and sales returns at the same
// moment, in one store and across stores. The race-db job runs this against
// a -race build of the server.

// startTogether runs fns at the same moment and waits for all of them.
func startTogether(fns []func()) {
	var ready, done sync.WaitGroup
	gate := make(chan struct{})
	for _, fn := range fns {
		ready.Add(1)
		done.Add(1)
		go func(fn func()) {
			defer done.Done()
			ready.Done()
			<-gate
			fn()
		}(fn)
	}
	ready.Wait()
	close(gate)
	done.Wait()
}

// asUser is in() with another user's token.
func asUser(t *testing.T, token, sid, method, path string, body interface{}) (int, apiResponse) {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return call(t, method, path+sep+storeQuery(sid), token, body)
}

type concDoc struct{ store, user, userName, id string }

func TestConcurrency_SalesAndReturnsAcrossUsersAndStores(t *testing.T) {
	const (
		storesN       = 2
		usersPerStore = 3
		salesPerUser  = 4
		stockIn       = 100.0
	)
	type user struct{ id, name, token string }
	type setup struct {
		sid, product, customer string
		users                  []user
	}
	stores := make([]*setup, storesN)
	for i := range stores {
		s := &setup{sid: newStore(t)}
		s.product = createProduct(t, s.sid, "Busy Widget", 100, 60)
		s.customer = createCustomer(t, s.sid, "Busy Customer", map[string]interface{}{"credit_limit": 100000})
		vendor := createVendor(t, s.sid, "Busy Vendor")
		when := agoStr(time.Hour)
		code, res := in(t, s.sid, "POST", "/v1/purchase", purchaseBody(s.sid, vendor, when,
			[]line{{id: s.product, name: "Busy Widget", qty: stockIn, price: 60}}, payment(stockIn*60*1.15, when)))
		mustOK(t, "stock purchase", code, res)
		for u := 0; u < usersPerStore; u++ {
			label := fmt.Sprintf("conc-%d-s%d-u%d", time.Now().UnixNano()%1000000, i, u)
			id, token := staff(t, label, "Manager", []string{s.sid})
			s.users = append(s.users, user{id, "E2E " + label, token})
		}
		stores[i] = s
	}
	for _, s := range stores {
		wantStock(t, s.sid, s.product, stockIn)
	}

	// Every user of every store sells at once: 2 units for 230 with VAT,
	// 100 paid, 130 on the customer's account.
	var mu sync.Mutex
	var sales []concDoc
	var fns []func()
	for _, s := range stores {
		for _, u := range s.users {
			for n := 0; n < salesPerUser; n++ {
				s, u := s, u
				fns = append(fns, func() {
					when := agoStr(30 * time.Minute)
					b := saleBody(s.sid, s.customer, when, []line{{id: s.product, name: "Busy Widget", qty: 2, price: 100, cost: 60}}, payment(100, when))
					code, res := asUser(t, u.token, s.sid, "POST", "/v1/order", b)
					if code != http.StatusOK || !res.Status {
						t.Errorf("store %s %s: sale HTTP %d %v", s.sid, u.name, code, res.Errors)
						return
					}
					mu.Lock()
					sales = append(sales, concDoc{s.sid, u.id, u.name, str(resultMap(t, res), "id")})
					mu.Unlock()
				})
			}
		}
	}
	startTogether(fns)
	perStore := usersPerStore * salesPerUser
	if len(sales) != storesN*perStore {
		t.Fatalf("%d of %d concurrent sales were saved", len(sales), storesN*perStore)
	}
	checkDocs(t, "sale", "/v1/order/", sales, 230)

	for _, s := range stores {
		s := s
		wantStock(t, s.sid, s.product, stockIn-2*float64(perStore))
		eventually(t, "customer balance in store "+s.sid, func() string {
			code, res := in(t, s.sid, "GET", "/v1/customer/"+s.customer, nil)
			c := mustOK(t, "get customer", code, res)
			if got, want := num(c, "credit_balance"), 130*float64(perStore); !approx(got, want) {
				return fmt.Sprintf("credit_balance = %v, want %v (%d sales x 130)", got, want, perStore)
			}
			return ""
		})
	}

	// Then every user returns one unit of each of its sales at once, refunding
	// the 100 the customer paid.
	var returns []concDoc
	fns = nil
	for _, sale := range sales {
		sale := sale
		var s *setup
		var token string
		for _, st := range stores {
			for _, u := range st.users {
				if u.id == sale.user {
					s, token = st, u.token
				}
			}
		}
		fns = append(fns, func() {
			code, res := asUser(t, token, s.sid, "POST", "/v1/sales-return", map[string]interface{}{
				"store_id": s.sid, "order_id": sale.id, "customer_id": s.customer, "date_str": nowStr(), "vat_percent": 15,
				"payment_status": "partially_paid", "payments_input": []map[string]interface{}{payment(100, nowStr())},
				"products": []map[string]interface{}{{"product_id": s.product, "name": "Busy Widget", "quantity": 1,
					"unit_price": 100, "purchase_unit_price": 60, "unit": "PC", "selected": true}},
			})
			if code != http.StatusOK || !res.Status {
				t.Errorf("store %s %s: return of %s HTTP %d %v", s.sid, sale.userName, sale.id, code, res.Errors)
				return
			}
			mu.Lock()
			returns = append(returns, concDoc{s.sid, sale.user, sale.userName, str(resultMap(t, res), "id")})
			mu.Unlock()
		})
	}
	startTogether(fns)
	if len(returns) != len(sales) {
		t.Fatalf("%d of %d concurrent returns were saved", len(returns), len(sales))
	}
	checkDocs(t, "sales return", "/v1/sales-return/", returns, 115)
	for _, s := range stores {
		wantStock(t, s.sid, s.product, stockIn-float64(perStore))
	}

	// The same user and store totals as if the sales had come one by one.
	for _, s := range stores {
		code, res := in(t, s.sid, "GET", "/v1/order?limit=100", nil)
		if code != http.StatusOK || !res.Status {
			t.Fatalf("list sales: HTTP %d %v", code, res.Errors)
		}
		if got := len(resultList(t, res)); got != perStore {
			t.Errorf("store %s lists %d sales, want %d (no sales from the other store)", s.sid, got, perStore)
		}
	}
}

// checkDocs reads each document back: unique codes per store, the right
// store and user, and a balanced ledger.
func checkDocs(t *testing.T, kind, path string, docs []concDoc, total float64) {
	t.Helper()
	codes := map[string]string{}
	for _, d := range docs {
		code, res := in(t, d.store, "GET", path+d.id, nil)
		m := mustOK(t, "view "+kind, code, res)
		label := fmt.Sprintf("%s %s", kind, d.id)
		key := d.store + "/" + str(m, "code")
		if str(m, "code") == "" {
			t.Errorf("%s: no code", label)
		} else if other, dup := codes[key]; dup {
			t.Errorf("%s and %s in store %s share code %s", other, d.id, d.store, str(m, "code"))
		}
		codes[key] = d.id
		if got := str(m, "store_id"); got != d.store {
			t.Errorf("%s: store_id %s, want %s", label, got, d.store)
		}
		if got := str(m, "created_by"); got != d.user {
			t.Errorf("%s: created_by %s, want %s (%s)", label, got, d.user, d.userName)
		}
		if got := str(m, "created_by_name"); got != "" && got != d.userName {
			t.Errorf("%s: created_by_name %q, want %q", label, got, d.userName)
		}
		if !approx(num(m, "net_total"), total) {
			t.Errorf("%s: net_total %v, want %v", label, num(m, "net_total"), total)
		}
		wantBalancedLedger(t, d.store, d.id, total)
	}
}
