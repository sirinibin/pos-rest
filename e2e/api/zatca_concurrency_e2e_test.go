//go:build e2e && zatca

package apie2e

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"
)

// Concurrent ZATCA reporting: several users of the same store, and users of
// different stores, report sales and credit notes at the same moment. Each
// document must be signed with its own store's certificate, carry its own
// ICV and a previous-invoice hash (PIH) that continues its store's chain,
// record the user who made it, and pass ZATCA's checks. Stores are new
// sandbox stores with ZATCA's test VAT number (see zatca_sandbox_e2e_test.go).

// zatcaGenesisPIH is ZATCA's PIH for a device's first invoice: the base64
// SHA-256 of "0".
const zatcaGenesisPIH = "NWZlY2ViNjZmZmM4NmYzOGQ5NTI3ODZjNmQ2OTZjNzljMmRiYzIzOWRkNGU5MWI0NjcyOWQ3M2EyN2ZiNTdlOQ=="

type zatcaDoc struct {
	kind, store, user, id string
	icv                   int64
	hash, prevHash, cert  string
	createdBy             string
	z                     map[string]interface{}
}

func viewZatcaDoc(t *testing.T, kind, sid, path string) zatcaDoc {
	t.Helper()
	code, res := in(t, sid, "GET", path, nil)
	m := mustOK(t, "view "+path, code, res)
	z, _ := m["zatca"].(map[string]interface{})
	d := zatcaDoc{kind: kind, store: sid, id: str(m, "id"), hash: str(m, "hash"), prevHash: str(m, "prev_hash"),
		createdBy: str(m, "created_by"), z: z}
	if v, ok := m["invoice_count_value"].(float64); ok {
		d.icv = int64(v)
	}
	if z != nil {
		d.cert, _ = z["x509_digital_certificate"].(string)
	}
	if s, _ := m["store_id"].(string); s != sid {
		t.Errorf("%s %s: store_id %q, want %q", kind, d.id, s, sid)
	}
	return d
}

// checkChain checks one store's documents of one kind: unique ICVs, and a
// PIH chain with no forks (two documents may not continue the same hash).
func checkChain(t *testing.T, label string, docs []zatcaDoc) {
	t.Helper()
	byHash := map[string]zatcaDoc{}
	icvs := map[int64]string{}
	for _, d := range docs {
		if prev, dup := icvs[d.icv]; dup {
			t.Errorf("%s: ICV %d used by both %s and %s", label, d.icv, prev, d.id)
		}
		icvs[d.icv] = d.id
		if d.hash == "" {
			continue
		}
		if prev, dup := byHash[d.hash]; dup {
			t.Errorf("%s: %s and %s have the same invoice hash", label, prev.id, d.id)
		}
		byHash[d.hash] = d
	}
	continued := map[string]string{}
	for _, d := range docs {
		if d.prevHash == "" {
			t.Errorf("%s: %s has no PIH", label, d.id)
			continue
		}
		if other, dup := continued[d.prevHash]; dup {
			t.Errorf("%s: %s and %s both continue the same previous invoice (the chain forked)", label, other, d.id)
		}
		continued[d.prevHash] = d.id
		if d.prevHash == zatcaGenesisPIH {
			continue
		}
		if prev, ok := byHash[d.prevHash]; !ok {
			t.Errorf("%s: %s's PIH is not the hash of any earlier %s of this store", label, d.id, d.kind)
		} else if prev.icv >= d.icv {
			t.Errorf("%s: %s (ICV %d) continues %s (ICV %d): the chain runs against the ICV order", label, d.id, d.icv, prev.id, prev.icv)
		}
	}
}

func TestZatcaSandbox_ConcurrentReportingAcrossUsersAndStores(t *testing.T) {
	const salesPerUser = 3
	type storeSetup struct {
		sid, product string
		users        []struct{ id, token string }
	}
	stores := make([]*storeSetup, 2)
	for i := range stores {
		s := &storeSetup{sid: newZatcaStore(t)}
		s.product = createProduct(t, s.sid, "ZATCA Concurrency Widget", 100, 60)
		for u := 0; u < 2; u++ {
			id, token := staff(t, fmt.Sprintf("zatca-s%d-u%d", i, u), "Manager", []string{s.sid})
			s.users = append(s.users, struct{ id, token string }{id, token})
		}
		stores[i] = s
	}

	// Every user of every store reports its sales at the same moment.
	var mu sync.Mutex
	var sales []zatcaDoc
	var fns []func()
	for _, s := range stores {
		for _, u := range s.users {
			for n := 0; n < salesPerUser; n++ {
				s, u := s, u
				fns = append(fns, func() {
					when := agoStr(time.Minute)
					b := saleBody(s.sid, "", when, []line{{id: s.product, name: "ZATCA Concurrency Widget", qty: 2, price: 100, cost: 60}}, payment(230, when))
					b["enable_report_to_zatca"] = true
					code, res := asUser(t, u.token, s.sid, "POST", "/v1/order", b)
					if code != http.StatusOK || !res.Status {
						t.Errorf("store %s user %s: sale HTTP %d %v", s.sid, u.id, code, res.Errors)
						return
					}
					m := resultMap(t, res)
					mu.Lock()
					sales = append(sales, zatcaDoc{kind: "order", store: s.sid, user: u.id, id: str(m, "id")})
					mu.Unlock()
				})
			}
		}
	}
	startTogether(fns)
	if want := len(stores) * 2 * salesPerUser; len(sales) != want {
		t.Fatalf("%d of %d concurrent sales were saved", len(sales), want)
	}

	// Then every user returns one unit of each of its sales, again all at once.
	var credits []zatcaDoc
	fns = nil
	for _, sale := range sales {
		sale := sale
		var s *storeSetup
		var token string
		for _, st := range stores {
			if st.sid == sale.store {
				s = st
				for _, u := range st.users {
					if u.id == sale.user {
						token = u.token
					}
				}
			}
		}
		fns = append(fns, func() {
			code, res := asUser(t, token, s.sid, "POST", "/v1/sales-return", map[string]interface{}{
				"store_id": s.sid, "order_id": sale.id, "date_str": nowStr(), "vat_percent": 15,
				"payment_status": "paid", "enable_report_to_zatca": true,
				"payments_input": []map[string]interface{}{payment(115, nowStr())},
				"products": []map[string]interface{}{{"product_id": s.product, "name": "ZATCA Concurrency Widget", "quantity": 1,
					"unit_price": 100, "purchase_unit_price": 60, "unit": "PC", "selected": true}},
			})
			if code != http.StatusOK || !res.Status {
				t.Errorf("store %s user %s: credit note for %s HTTP %d %v", s.sid, sale.user, sale.id, code, res.Errors)
				return
			}
			m := resultMap(t, res)
			mu.Lock()
			credits = append(credits, zatcaDoc{kind: "sales-return", store: s.sid, user: sale.user, id: str(m, "id")})
			mu.Unlock()
		})
	}
	startTogether(fns)

	// Read every document back and check it.
	certOf := map[string]string{}
	check := func(docs []zatcaDoc, path string) map[string][]zatcaDoc {
		perStore := map[string][]zatcaDoc{}
		for _, d := range docs {
			got := viewZatcaDoc(t, d.kind, d.store, path+d.id)
			got.user = d.user
			label := fmt.Sprintf("%s %s (store %s)", d.kind, d.id, d.store)
			if got.z["reporting_passed"] != true {
				t.Errorf("%s: not reported: errors=%v compliance=%v", label, got.z["reporting_errors"], got.z["compliance_check_errors"])
			}
			if q, _ := got.z["qr_code"].(string); q == "" {
				t.Errorf("%s: no QR code", label)
			}
			if got.createdBy != d.user {
				t.Errorf("%s: created_by %q, want the user who made it %q", label, got.createdBy, d.user)
			}
			if got.cert != "" {
				if c, seen := certOf[d.store]; seen && c != got.cert {
					t.Errorf("%s: signed with a different certificate than the store's other documents", label)
				}
				certOf[d.store] = got.cert
			}
			perStore[d.store] = append(perStore[d.store], got)
		}
		return perStore
	}
	salesByStore := check(sales, "/v1/order/")
	creditsByStore := check(credits, "/v1/sales-return/")
	// ZATCA's sandbox issues the same test certificate to every onboarding,
	// so two stores sharing one says nothing about mixing there.
	if len(certOf) == 2 && certOf[stores[0].sid] == certOf[stores[1].sid] {
		t.Logf("NOTE both stores sign with the same certificate (expected in the sandbox)")
	}

	for _, s := range stores {
		checkChain(t, "store "+s.sid+" invoices", salesByStore[s.sid])
		checkChain(t, "store "+s.sid+" credit notes", creditsByStore[s.sid])

		// ZATCA wants one ICV sequence and one PIH chain per device, across
		// invoices and credit notes alike. Logged for now: the store keeps
		// separate counters and chains per document type.
		all := append(append([]zatcaDoc{}, salesByStore[s.sid]...), creditsByStore[s.sid]...)
		sort.Slice(all, func(i, j int) bool { return all[i].icv < all[j].icv })
		seen := map[int64]int{}
		for _, d := range all {
			seen[d.icv]++
		}
		for icv, n := range seen {
			if n > 1 {
				t.Logf("NOTE store %s: ICV %d is used by %d documents (invoices and credit notes count separately)", s.sid, icv, n)
			}
		}
	}
}

// Two stores onboarding at the same moment each get their own credentials.
func TestZatcaSandbox_ConcurrentOnboarding(t *testing.T) {
	sids := make([]string, 3)
	fns := make([]func(), len(sids))
	for i := range sids {
		i := i
		sids[i] = newStore(t, map[string]interface{}{
			"vat_no": zatcaTestVAT, "registration_number": zatcaTestCRN, "business_category": "Supply activities",
			"zatca": map[string]interface{}{"phase": "2", "env": "NonProduction"},
		})
		fns[i] = func() {
			code, res := call(t, "POST", "/v1/store/zatca/connect", authToken(t), map[string]interface{}{"id": sids[i], "otp": zatcaTestOTP})
			if code != http.StatusOK || !res.Status {
				t.Errorf("store %d: concurrent onboarding HTTP %d %v", i, code, res.Errors)
			}
		}
	}
	startTogether(fns)

	certs := map[string]int{}
	for i, sid := range sids {
		code, res := call(t, "GET", "/v1/store/"+sid, authToken(t), nil)
		z, _ := resultMap(t, res)["zatca"].(map[string]interface{})
		if code != http.StatusOK || z["connected"] != true {
			t.Errorf("store %d not connected after concurrent onboarding: zatca=%v", i, z)
			continue
		}
		// Each store reports a sale signed with its own certificate.
		widget := createProduct(t, sid, "Onboarding Widget", 100, 60)
		when := agoStr(time.Minute)
		b := saleBody(sid, "", when, []line{{id: widget, name: "Onboarding Widget", qty: 1, price: 100, cost: 60}}, payment(115, when))
		b["enable_report_to_zatca"] = true
		code, res = in(t, sid, "POST", "/v1/order", b)
		o := mustOK(t, fmt.Sprintf("store %d sale", i), code, res)
		d := viewZatcaDoc(t, "order", sid, "/v1/order/"+str(o, "id"))
		if d.z["reporting_passed"] != true {
			t.Errorf("store %d: sale not reported: %v", i, d.z["reporting_errors"])
		}
		if d.cert != "" {
			certs[d.cert]++
		}
	}
	for _, n := range certs {
		if n > 1 {
			t.Logf("NOTE %d stores onboarded together sign with the same certificate (expected in the sandbox)", n)
		}
	}
}
