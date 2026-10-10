//go:build e2e

package apie2e

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Helpers shared by the functional (business-flow) suites. Each flow test
// gets its own store, so stock levels, serial numbers and balances start
// from zero and tests never see one another's documents.

var storeSeq uint32

// newStore creates a fresh store from fixtures/store.json and returns its id.
func newStore(t *testing.T, overrides ...map[string]interface{}) string {
	t.Helper()
	tok := authToken(t)
	raw, err := os.ReadFile("../fixtures/store.json")
	if err != nil {
		t.Fatalf("read store fixture: %v", err)
	}
	var store map[string]interface{}
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatalf("parse store fixture: %v", err)
	}
	n := atomic.AddUint32(&storeSeq, 1)
	store["code"] = fmt.Sprintf("F%s%03d", runID[len(runID)-5:], n)
	store["name"] = fmt.Sprintf("Flow Store %s-%d", runID, n)
	for _, o := range overrides {
		for k, v := range o {
			store[k] = v
		}
	}
	code, res := call(t, "POST", "/v1/store", tok, store)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("create store: HTTP %d %v", code, res.Errors)
	}
	id, _ := resultMap(t, res)["id"].(string)
	if id == "" {
		t.Fatalf("create store returned no id")
	}
	return id
}

// in calls an endpoint scoped to a store (search[store_id]=...).
func in(t *testing.T, sid, method, path string, body interface{}) (int, apiResponse) {
	t.Helper()
	sep := "?"
	for _, c := range path {
		if c == '?' {
			sep = "&"
		}
	}
	return call(t, method, path+sep+storeQuery(sid), authToken(t), body)
}

// mustOK fails the test unless the call succeeded, and returns its result.
func mustOK(t *testing.T, what string, code int, res apiResponse) map[string]interface{} {
	t.Helper()
	if code != http.StatusOK || !res.Status {
		t.Fatalf("%s: HTTP %d status=%v errors=%v", what, code, res.Status, res.Errors)
	}
	return resultMap(t, res)
}

// mustReject fails the test unless the call was refused with an error under
// one of the given keys.
func mustReject(t *testing.T, what string, code int, res apiResponse, keys ...string) {
	t.Helper()
	if code == http.StatusOK && res.Status {
		t.Fatalf("%s: want rejection, got HTTP 200 status=true", what)
	}
	if code >= 500 {
		t.Fatalf("%s: want a 4xx validation error, got HTTP %d %v", what, code, res.Errors)
	}
	if len(keys) == 0 {
		return
	}
	for _, k := range keys {
		if _, ok := res.Errors[k]; ok {
			return
		}
	}
	t.Fatalf("%s: want an error under one of %v, got %v", what, keys, res.Errors)
}

func num(m map[string]interface{}, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

func str(m map[string]interface{}, key string) string {
	s, _ := m[key].(string)
	return s
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.005 }

func wantNum(t *testing.T, what string, got, want float64) {
	t.Helper()
	if !approx(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// nowStr is the timestamp format the API's date_str fields accept.
func nowStr() string { return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339) }

func agoStr(d time.Duration) string {
	return time.Now().UTC().Add(-d).Truncate(time.Second).Format(time.RFC3339)
}

func createProduct(t *testing.T, sid, name string, retail, purchase float64) string {
	t.Helper()
	code, res := in(t, sid, "POST", "/v1/product", map[string]interface{}{
		"store_id":    sid,
		"name":        name,
		"part_number": fmt.Sprintf("P-%s-%d", runID[len(runID)-5:], atomic.AddUint32(&storeSeq, 1)),
		"unit":        "PC",
		"product_stores": map[string]interface{}{
			sid: map[string]interface{}{"store_id": sid, "retail_unit_price": retail, "purchase_unit_price": purchase},
		},
	})
	id := str(mustOK(t, "create product "+name, code, res), "id")
	if id == "" {
		t.Fatalf("product %s has no id", name)
	}
	return id
}

func createCustomer(t *testing.T, sid, name string, extra ...map[string]interface{}) string {
	t.Helper()
	body := map[string]interface{}{"store_id": sid, "name": name}
	for _, e := range extra {
		for k, v := range e {
			body[k] = v
		}
	}
	code, res := in(t, sid, "POST", "/v1/customer", body)
	return str(mustOK(t, "create customer "+name, code, res), "id")
}

func createVendor(t *testing.T, sid, name string) string {
	t.Helper()
	code, res := in(t, sid, "POST", "/v1/vendor", map[string]interface{}{"store_id": sid, "name": name})
	return str(mustOK(t, "create vendor "+name, code, res), "id")
}

// line is one product row on a sale, purchase, return or quotation.
type line struct {
	id       string
	name     string
	qty      float64
	price    float64 // unit price (sales) or purchase unit price (purchases)
	cost     float64 // purchase unit price carried on a sale line, for profit
	discount float64 // unit discount
}

func payment(amount float64, date string) map[string]interface{} {
	return map[string]interface{}{"date_str": date, "amount": amount, "method": "cash"}
}

func purchaseBody(sid, vendor, date string, lines []line, payments ...map[string]interface{}) map[string]interface{} {
	products := []map[string]interface{}{}
	for _, l := range lines {
		products = append(products, map[string]interface{}{
			"product_id": l.id, "name": l.name, "quantity": l.qty, "purchase_unit_price": l.price,
			"unit_discount": l.discount, "unit": "PC",
		})
	}
	if payments == nil {
		payments = []map[string]interface{}{}
	}
	return map[string]interface{}{
		"store_id": sid, "vendor_id": vendor, "date_str": date, "vat_percent": 15,
		"products": products, "payments_input": payments,
	}
}

func saleBody(sid, customer, date string, lines []line, payments ...map[string]interface{}) map[string]interface{} {
	products := []map[string]interface{}{}
	for _, l := range lines {
		products = append(products, map[string]interface{}{
			"product_id": l.id, "name": l.name, "quantity": l.qty, "unit_price": l.price,
			"purchase_unit_price": l.cost, "unit_discount": l.discount, "unit": "PC",
		})
	}
	if payments == nil {
		payments = []map[string]interface{}{}
	}
	body := map[string]interface{}{
		"store_id": sid, "date_str": date, "vat_percent": 15,
		"products": products, "payments_input": payments,
	}
	if customer != "" {
		body["customer_id"] = customer
	}
	return body
}

// stockOf reads a product's stock in a store.
func stockOf(t *testing.T, sid, productID string) float64 {
	t.Helper()
	code, res := in(t, sid, "GET", "/v1/product/"+productID, nil)
	p := mustOK(t, "get product", code, res)
	stores, _ := p["product_stores"].(map[string]interface{})
	ps, _ := stores[sid].(map[string]interface{})
	return num(ps, "stock")
}

// eventually retries check until it returns "" or the deadline passes; some
// side effects (stock, balances, ledger) are written after the response.
func eventually(t *testing.T, what string, check func() string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		msg := check()
		if msg == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: %s", what, msg)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func wantStock(t *testing.T, sid, productID string, want float64) {
	t.Helper()
	eventually(t, "stock", func() string {
		if got := stockOf(t, sid, productID); !approx(got, want) {
			return fmt.Sprintf("stock = %v, want %v", got, want)
		}
		return ""
	})
}

// postings returns the ledger postings written for one document.
func postings(t *testing.T, sid, referenceID string) []map[string]interface{} {
	t.Helper()
	code, res := in(t, sid, "GET", "/v1/posting?search[reference_id]="+referenceID+"&limit=100", nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("list postings: HTTP %d %v", code, res.Errors)
	}
	return resultList(t, res)
}

// wantBalancedLedger checks double-entry bookkeeping for a document: it has
// postings, and across them total debits equal total credits.
func wantBalancedLedger(t *testing.T, sid, referenceID string, wantTotal float64) {
	t.Helper()
	eventually(t, "ledger for "+referenceID, func() string {
		rows := postings(t, sid, referenceID)
		if len(rows) == 0 {
			return "no postings"
		}
		var debit, credit float64
		for _, p := range rows {
			debit += num(p, "debit_total")
			credit += num(p, "credit_total")
		}
		if !approx(debit, credit) {
			return fmt.Sprintf("debits %.2f != credits %.2f", debit, credit)
		}
		if wantTotal > 0 && debit+0.005 < wantTotal {
			return fmt.Sprintf("debits %.2f less than document total %.2f", debit, wantTotal)
		}
		return ""
	})
}

// meID is the signed-in e2e user's id.
func meID(t *testing.T) string {
	t.Helper()
	code, res := call(t, "GET", "/v1/me", authToken(t), nil)
	id := str(mustOK(t, "me", code, res), "id")
	if id == "" {
		t.Fatalf("/v1/me returned no id")
	}
	return id
}
