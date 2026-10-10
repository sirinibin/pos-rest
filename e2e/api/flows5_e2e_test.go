//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Functional tests, part 5: line quantities, stock transfer edits, ZATCA
// credentials in store responses, onboarding errors and reporting in stores
// that aren't on ZATCA.

// A negative quantity used to be saved on every document but non-VAT sales
// and purchase requests; on a purchase it drove the product's stock negative.
func TestFlow_NegativeLineQuantitiesAreRefused(t *testing.T) {
	sid := newStore(t)
	a := createProduct(t, sid, "Widget A", 100, 60)
	b := createProduct(t, sid, "Widget B", 100, 60)
	vendor := createVendor(t, sid, "Quantity Vendor")
	customer := createCustomer(t, sid, "Quantity Customer", map[string]interface{}{"credit_limit": 5000})
	when := agoStr(time.Hour)

	// The coordinator's repro: line B at -1 on a paid purchase.
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, when,
		[]line{{id: a, name: "Widget A", qty: 5, price: 60}, {id: b, name: "Widget B", qty: -1, price: 60}}, payment(276, when)))
	mustReject(t, "purchase with a negative line", code, res, "quantity_1")

	code, res = in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, when,
		[]line{{id: a, name: "Widget A", qty: 10, price: 60}}, payment(690, when)))
	p := mustOK(t, "purchase of 10", code, res)
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: a, name: "Widget A", qty: 2, price: 100, cost: 60}}, payment(230, when)))
	o := mustOK(t, "sale of 2", code, res)

	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, nowStr(),
		[]line{{id: a, name: "Widget A", qty: 2, price: 100, cost: 60}, {id: b, name: "Widget B", qty: -1, price: 100, cost: 60}}, payment(115, nowStr())))
	mustReject(t, "sale with a negative line", code, res, "quantity_1")

	neg := []map[string]interface{}{{"product_id": a, "name": "Widget A", "quantity": -1, "unit_price": 100,
		"purchase_unit_price": 60, "purchasereturn_unit_price": 60, "unit": "PC", "selected": true}}
	for _, d := range []struct {
		path string
		body map[string]interface{}
	}{
		{"/v1/quotation", map[string]interface{}{"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 15,
			"validity_days": 7, "delivery_days": 3, "type": "quotation", "products": neg}},
		{"/v1/delivery-note", map[string]interface{}{"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 15, "products": neg}},
		{"/v1/sales-return", map[string]interface{}{"store_id": sid, "order_id": str(o, "id"), "customer_id": customer, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{}, "products": neg}},
		{"/v1/purchase-return", map[string]interface{}{"store_id": sid, "purchase_id": str(p, "id"), "vendor_id": vendor, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{}, "purchase_returned_by": meID(t), "products": neg}},
	} {
		code, res := in(t, sid, "POST", d.path, d.body)
		mustReject(t, d.path+" with a negative line", code, res, "quantity_0")
	}

	time.Sleep(time.Second)
	wantStock(t, sid, a, 8)
	wantStock(t, sid, b, 0)
}

// Editing a transfer may re-use the stock it already moved, and no more.
func TestFlow_StockTransferEditCountsWhatItAlreadyMoved(t *testing.T) {
	sid := newStore(t)
	from, fromCode := createWarehouse(t, sid, "East Warehouse")
	to, toCode := createWarehouse(t, sid, "West Warehouse")
	widget := createProduct(t, sid, "Widget", 100, 60)
	vendor := createVendor(t, sid, "Edit Vendor")
	pb := purchaseBody(sid, vendor, agoStr(time.Hour), []line{{id: widget, name: "Widget", qty: 5, price: 60}})
	pb["products"].([]map[string]interface{})[0]["warehouse_id"] = from
	code, res := in(t, sid, "POST", "/v1/purchase", pb)
	mustOK(t, "purchase into the east warehouse", code, res)
	eventually(t, "east stock", func() string {
		if got := warehouseStock(t, sid, widget, fromCode); !approx(got, 5) {
			return fmt.Sprintf("%s = %v, want 5", fromCode, got)
		}
		return ""
	})

	body := func(qty float64) map[string]interface{} {
		return map[string]interface{}{
			"store_id": sid, "from_warehouse_id": from, "to_warehouse_id": to, "date_str": nowStr(), "vat_percent": 15,
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": qty, "unit_price": 60, "unit": "PC"}},
		}
	}
	code, res = in(t, sid, "POST", "/v1/stock-transfer", body(3))
	tr := mustOK(t, "transfer 3", code, res)
	eventually(t, "east stock after transfer", func() string {
		if got := warehouseStock(t, sid, widget, fromCode); !approx(got, 2) {
			return fmt.Sprintf("%s = %v, want 2", fromCode, got)
		}
		return ""
	})

	code, res = in(t, sid, "PUT", "/v1/stock-transfer/"+str(tr, "id"), body(6))
	mustReject(t, "edit to 6 when 2 are left and 3 already moved", code, res, "quantity_0")
	code, res = in(t, sid, "PUT", "/v1/stock-transfer/"+str(tr, "id"), body(5))
	mustOK(t, "edit to 5", code, res)
	eventually(t, "stocks after the edit", func() string {
		e, w := warehouseStock(t, sid, widget, fromCode), warehouseStock(t, sid, widget, toCode)
		if !approx(e, 0) || !approx(w, 5) {
			return fmt.Sprintf("%s %v %s %v, want 0 and 5", fromCode, e, toCode, w)
		}
		return ""
	})
}

// The store's ZATCA signing key and API secrets never go out in a response.
func TestSecurity_StoreResponsesCarryNoZatcaSecrets(t *testing.T) {
	sid := newStore(t)
	secretKeys := []string{"private_key", "secret", "binary_security_token", "production_secret", "production_binary_security_token"}
	check := func(what string, z map[string]interface{}) {
		t.Helper()
		for _, k := range secretKeys {
			if v, _ := z[k].(string); v != "" {
				t.Fatalf("%s: zatca.%s is in the response", what, k)
			}
		}
	}

	code, res := call(t, "GET", "/v1/store/"+sid, authToken(t), nil)
	st := resultMap(t, res)
	z, _ := st["zatca"].(map[string]interface{})
	if code != http.StatusOK {
		t.Fatalf("view store: HTTP %d", code)
	}
	check("view", z)

	// A client can't plant credentials through the store form either.
	if z == nil {
		z = map[string]interface{}{}
	}
	z["private_key"] = "planted-key"
	z["secret"] = "planted-secret"
	st["zatca"] = z
	code, res = call(t, "PUT", "/v1/store/"+sid, authToken(t), st)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("update store: HTTP %d %v", code, res.Errors)
	}
	u, _ := resultMap(t, res)["zatca"].(map[string]interface{})
	check("update", u)

	code, res = call(t, "GET", "/v1/store?search[name]=&limit=50&sort=-created_at", authToken(t), nil)
	if code != http.StatusOK {
		t.Fatalf("list stores: HTTP %d", code)
	}
	if strings.Contains(string(res.Result), "planted-") {
		t.Fatalf("the store list carries ZATCA credentials")
	}
}

// Onboarding failures say what went wrong with a 4xx or 502, not a 500.
func TestFlow_ZatcaConnectErrorsAreNot500(t *testing.T) {
	sid := newStore(t)
	code, res := call(t, "POST", "/v1/store/zatca/connect", authToken(t), map[string]interface{}{"id": "not-an-id", "otp": "123345"})
	if code != http.StatusBadRequest || res.Status {
		t.Fatalf("bad store id: HTTP %d status=%v, want 400", code, res.Status)
	}
	code, res = call(t, "POST", "/v1/store/zatca/connect", authToken(t), map[string]interface{}{"id": "5f1d7f3e9b1e8a3f4c2b1a00", "otp": "123345"})
	if code != http.StatusNotFound && code != http.StatusForbidden {
		t.Fatalf("unknown store: HTTP %d, want 404 (or 403 for a store the user can't use)", code)
	}
	// The store isn't set up for ZATCA (no VAT number, no Phase 2), and in this
	// job there is no ZatcaPython venv: either way the answer is not a 500.
	code, res = call(t, "POST", "/v1/store/zatca/connect", authToken(t), map[string]interface{}{"id": sid, "otp": "123345"})
	if res.Status {
		t.Fatalf("a store without ZATCA details connected: HTTP %d", code)
	}
	if code == http.StatusInternalServerError || code == http.StatusOK {
		t.Fatalf("failed onboarding answered HTTP %d %v", code, res.Errors)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("failed onboarding gave no reason")
	}
}

// A sale marked for ZATCA reporting in a store that isn't connected to ZATCA
// is saved; it used to need internet access first.
func TestFlow_SaleMarkedForZatcaInAStoreOffZatcaIsSaved(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	b := saleBody(sid, "", nowStr(), []line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}, payment(115, nowStr()))
	b["enable_report_to_zatca"] = true
	code, res := in(t, sid, "POST", "/v1/order", b)
	o := mustOK(t, "sale marked for reporting", code, res)
	z, _ := o["zatca"].(map[string]interface{})
	if z != nil && z["reporting_passed"] == true {
		t.Fatalf("a store off ZATCA reported a sale")
	}
}
