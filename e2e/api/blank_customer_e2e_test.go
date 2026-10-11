//go:build e2e

package apie2e

import (
	"strings"
	"testing"
	"time"
)

// A customer name typed into a sale, quotation or delivery note creates that
// customer. A name of only spaces (or invisible characters) used to create a
// customer with a blank name; it must fall back to UNKNOWN instead, and the
// customer form must refuse it.

var blankNames = map[string]string{
	"space":            " ",
	"spaces":           "    ",
	"tab and newline":  "\t\n",
	"no-break space":   "\u00a0",
	"zero-width space": "\u200b",
	"mixed invisibles": " \u200b\ufeff\u2060 ",
}

// wantNoBlankCustomers fails if any customer in the store has a blank name.
func wantNoBlankCustomers(t *testing.T, sid string) {
	t.Helper()
	code, res := in(t, sid, "GET", "/v1/customer?limit=500", nil)
	if code != 200 || !res.Status {
		t.Fatalf("list customers: HTTP %d %v", code, res.Errors)
	}
	for _, c := range resultList(t, res) {
		if strings.TrimSpace(strings.Trim(str(c, "name"), "\u200b\ufeff\u2060")) == "" {
			t.Fatalf("customer %s (%s) has a blank name %q", str(c, "code"), str(c, "id"), str(c, "name"))
		}
	}
}

func TestBlankCustomer_SaleWithBlankTypedNameUsesUnknown(t *testing.T) {
	sid := newStore(t)
	item := createProduct(t, sid, "Blank Name Item", 100, 60)
	when := agoStr(time.Hour)
	unknownID := ""
	for label, name := range blankNames {
		t.Run(label, func(t *testing.T) {
			body := saleBody(sid, "", when, []line{{id: item, name: "Blank Name Item", qty: 1, price: 100, cost: 60}}, payment(115, when))
			body["customer_name"] = name
			code, res := in(t, sid, "POST", "/v1/order", body)
			o := mustOK(t, "sale with customer name "+label, code, res)
			if got := str(o, "customer_name"); got != "UNKNOWN" {
				t.Fatalf("customer_name = %q, want UNKNOWN", got)
			}
			if unknownID == "" {
				unknownID = str(o, "customer_id")
			} else if got := str(o, "customer_id"); got != unknownID {
				t.Fatalf("customer_id = %s, want the one UNKNOWN customer %s", got, unknownID)
			}
		})
	}
	wantNoBlankCustomers(t, sid)
}

func TestBlankCustomer_SaleTypedNameIsTrimmed(t *testing.T) {
	sid := newStore(t)
	item := createProduct(t, sid, "Trim Item", 100, 60)
	when := agoStr(time.Hour)
	body := saleBody(sid, "", when, []line{{id: item, name: "Trim Item", qty: 1, price: 100, cost: 60}}, payment(115, when))
	body["customer_name"] = "  \u200bWalk In Ali \t"
	code, res := in(t, sid, "POST", "/v1/order", body)
	o := mustOK(t, "sale with padded customer name", code, res)
	if got := str(o, "customer_name"); got != "Walk In Ali" {
		t.Fatalf("customer_name = %q, want %q", got, "Walk In Ali")
	}
	code, res = in(t, sid, "GET", "/v1/customer/"+str(o, "customer_id"), nil)
	if got := str(mustOK(t, "get new customer", code, res), "name"); got != "Walk In Ali" {
		t.Fatalf("new customer name = %q, want %q", got, "Walk In Ali")
	}
}

func TestBlankCustomer_SaleUpdateWithBlankTypedNameUsesUnknown(t *testing.T) {
	sid := newStore(t)
	item := createProduct(t, sid, "Edit Item", 100, 60)
	customer := createCustomer(t, sid, "Named Customer")
	when := agoStr(time.Hour)
	body := saleBody(sid, customer, when, []line{{id: item, name: "Edit Item", qty: 1, price: 100, cost: 60}}, payment(115, when))
	code, res := in(t, sid, "POST", "/v1/order", body)
	o := mustOK(t, "create sale", code, res)

	// The user clears the customer box and types a space before saving.
	body = saleBody(sid, "", when, []line{{id: item, name: "Edit Item", qty: 1, price: 100, cost: 60}}, payment(115, when))
	body["id"] = str(o, "id")
	body["customer_id"] = ""
	body["customer_name"] = " "
	code, res = in(t, sid, "PUT", "/v1/order/"+str(o, "id"), body)
	u := mustOK(t, "update sale", code, res)
	if got := str(u, "customer_name"); got != "UNKNOWN" {
		t.Fatalf("customer_name = %q, want UNKNOWN", got)
	}
	wantNoBlankCustomers(t, sid)
}

func TestBlankCustomer_QuotationAndDeliveryNoteWithBlankTypedName(t *testing.T) {
	sid := newStore(t)
	item := createProduct(t, sid, "Doc Item", 100, 60)
	when := agoStr(time.Hour)
	products := []map[string]interface{}{{"product_id": item, "name": "Doc Item", "quantity": 1, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}}

	code, res := in(t, sid, "POST", "/v1/quotation", map[string]interface{}{
		"store_id": sid, "customer_name": "  ", "date_str": when, "vat_percent": 15, "type": "quotation",
		"validity_days": 7, "delivery_days": 3, "products": products,
	})
	if got := str(mustOK(t, "quotation with blank customer name", code, res), "customer_name"); got != "UNKNOWN" {
		t.Fatalf("quotation customer_name = %q, want UNKNOWN", got)
	}

	code, res = in(t, sid, "POST", "/v1/delivery-note", map[string]interface{}{
		"store_id": sid, "customer_name": "\u200b", "date_str": when, "vat_percent": 15, "products": products,
	})
	if got := str(mustOK(t, "delivery note with blank customer name", code, res), "customer_name"); got != "UNKNOWN" {
		t.Fatalf("delivery note customer_name = %q, want UNKNOWN", got)
	}
	wantNoBlankCustomers(t, sid)
}

func TestBlankCustomer_CustomerFormRefusesBlankName(t *testing.T) {
	sid := newStore(t)
	for label, name := range blankNames {
		t.Run("create "+label, func(t *testing.T) {
			code, res := in(t, sid, "POST", "/v1/customer", map[string]interface{}{"store_id": sid, "name": name})
			mustReject(t, "create customer named "+label, code, res, "name")
		})
	}
	id := createCustomer(t, sid, "Real Name")
	for label, name := range blankNames {
		t.Run("update "+label, func(t *testing.T) {
			code, res := in(t, sid, "PUT", "/v1/customer/"+id, map[string]interface{}{"store_id": sid, "name": name})
			mustReject(t, "rename customer to "+label, code, res, "name")
		})
	}
	wantNoBlankCustomers(t, sid)
}
