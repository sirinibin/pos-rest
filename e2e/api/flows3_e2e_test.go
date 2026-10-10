//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Functional tests, part 3: quotation invoices and their returns, payments
// against purchases and returns, non-VAT sales returns, purchase requests and
// previous/next/last navigation.

// ── Quotation invoices and quotation sales returns ───────────────────────────

func TestFlow_QuotationInvoiceSellsAndCanBeReturned(t *testing.T) {
	// Quotation invoices post to the ledger only when the store turns it on.
	sid := newStore(t, map[string]interface{}{"settings": map[string]interface{}{"quotation_invoice_accounting": true}})
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Invoice Customer", map[string]interface{}{"credit_limit": 5000})

	code, res := in(t, sid, "POST", "/v1/quotation", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": agoStr(time.Hour), "vat_percent": 15, "type": "invoice",
		"validity_days": 7, "delivery_days": 3, "payment_status": "paid",
		"products":       []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 4, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
		"payments_input": []map[string]interface{}{payment(460, agoStr(time.Hour))},
	})
	q := mustOK(t, "quotation invoice", code, res)
	qid := str(q, "id")
	wantNum(t, "net_total", num(q, "net_total"), 460)
	wantBalancedLedger(t, sid, qid, 0)

	ret := func(qty float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/quotation-sales-return", map[string]interface{}{
			"store_id": sid, "quotation_id": qid, "customer_id": customer, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
			"products": []map[string]interface{}{{
				"product_id": widget, "name": "Widget", "quantity": qty, "unit_price": 100,
				"purchase_unit_price": 60, "unit": "PC", "selected": true,
			}},
		})
	}
	code, res = ret(5)
	mustReject(t, "return more than invoiced", code, res, "quantity_0")

	code, res = ret(1)
	r := mustOK(t, "return 1", code, res)
	wantNum(t, "return net_total", num(r, "net_total"), 115)
	wantBalancedLedger(t, sid, str(r, "id"), 0)

	code, res = ret(4)
	mustReject(t, "return more than what is left (3)", code, res, "quantity_0")

	// Refunds against the return are capped by it.
	refund := func(amount float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/quotation-sales-return-payment", map[string]interface{}{
			"store_id": sid, "quotation_sales_return_id": str(r, "id"), "quotation_id": qid,
			"amount": amount, "method": "cash", "date_str": nowStr(),
		})
	}
	code, res = refund(500)
	mustReject(t, "refund above the return", code, res, "amount")
	code, res = refund(0)
	mustReject(t, "zero refund", code, res, "amount")
	code, res = refund(115)
	mustOK(t, "refund the return", code, res)

	code, res = in(t, sid, "POST", "/v1/quotation-sales-return", map[string]interface{}{
		"store_id": sid, "quotation_id": "5f1d7f3e9b1e8a3f4c2b1a00", "date_str": nowStr(), "vat_percent": 15,
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 1, "unit_price": 100, "selected": true}},
	})
	mustReject(t, "return against an unknown quotation", code, res)
}

// ── Payments against purchases and returns ───────────────────────────────────

// paymentFlow drives one payment endpoint against a document whose total is
// due: over-payments, zero and negative amounts are refused, payments move
// the status to partly paid and paid, and deleting one moves it back.
type paymentFlow struct {
	path    string                 // e.g. /v1/purchase-payment
	link    map[string]interface{} // the fields naming the document
	docPath string                 // GET path of the document
	due     float64
}

func (f paymentFlow) run(t *testing.T, sid string) {
	t.Helper()
	pay := func(amount float64) (int, apiResponse) {
		b := map[string]interface{}{"store_id": sid, "amount": amount, "method": "cash", "date_str": nowStr()}
		for k, v := range f.link {
			b[k] = v
		}
		return in(t, sid, "POST", f.path, b)
	}
	status := func(wantStatus string, wantBalance float64) {
		t.Helper()
		eventually(t, "status "+wantStatus, func() string {
			code, res := in(t, sid, "GET", f.docPath, nil)
			d := mustOK(t, "view document", code, res)
			if str(d, "payment_status") != wantStatus || !approx(num(d, "balance_amount"), wantBalance) {
				return fmt.Sprintf("status %s balance %v, want %s %v", str(d, "payment_status"), num(d, "balance_amount"), wantStatus, wantBalance)
			}
			return ""
		})
	}

	code, res := pay(f.due + 1)
	mustReject(t, "payment above what is due", code, res, "amount")
	code, res = pay(0)
	mustReject(t, "zero payment", code, res, "amount")
	code, res = pay(-1)
	mustReject(t, "negative payment", code, res, "amount")

	code, res = pay(10)
	first := mustOK(t, "pay 10", code, res)
	status("paid_partially", f.due-10)

	code, res = pay(f.due - 10 + 1)
	mustReject(t, "second payment above the balance", code, res, "amount")
	code, res = pay(f.due - 10)
	mustOK(t, "pay the rest", code, res)
	status("paid", 0)

	code, res = in(t, sid, "DELETE", f.path+"/"+str(first, "id"), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("delete first payment: HTTP %d %v", code, res.Errors)
	}
	status("paid_partially", 10)

	unknown := map[string]interface{}{}
	for k := range f.link {
		unknown[k] = "5f1d7f3e9b1e8a3f4c2b1a00"
	}
	b := map[string]interface{}{"store_id": sid, "amount": 1, "method": "cash", "date_str": nowStr()}
	for k, v := range unknown {
		b[k] = v
	}
	code, res = in(t, sid, "POST", f.path, b)
	mustReject(t, "payment for an unknown document", code, res)
}

func TestFlow_PurchasePayments(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	vendor := createVendor(t, sid, "Paid Vendor")
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}))
	p := mustOK(t, "unpaid purchase", code, res)
	if str(p, "payment_status") != "not_paid" {
		t.Fatalf("payment_status = %q, want not_paid", str(p, "payment_status"))
	}
	paymentFlow{
		path: "/v1/purchase-payment", link: map[string]interface{}{"purchase_id": str(p, "id")},
		docPath: "/v1/purchase/" + str(p, "id"), due: 690,
	}.run(t, sid)
}

func TestFlow_SalesReturnPayments(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Refund Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, customer, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 2, price: 100, cost: 60}}, payment(230, agoStr(time.Hour))))
	o := mustOK(t, "paid sale", code, res)
	code, res = in(t, sid, "POST", "/v1/sales-return", map[string]interface{}{
		"store_id": sid, "order_id": str(o, "id"), "customer_id": customer, "date_str": nowStr(),
		"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 2, "unit_price": 100,
			"purchase_unit_price": 60, "unit": "PC", "selected": true}},
	})
	sr := mustOK(t, "unrefunded return", code, res)
	paymentFlow{
		path: "/v1/sales-return-payment", link: map[string]interface{}{"sales_return_id": str(sr, "id"), "order_id": str(o, "id")},
		docPath: "/v1/sales-return/" + str(sr, "id"), due: 230,
	}.run(t, sid)
}

func TestFlow_PurchaseReturnPayments(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	vendor := createVendor(t, sid, "Refunding Vendor")
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}, payment(690, agoStr(time.Hour))))
	p := mustOK(t, "paid purchase", code, res)
	code, res = in(t, sid, "POST", "/v1/purchase-return", map[string]interface{}{
		"store_id": sid, "purchase_id": str(p, "id"), "vendor_id": vendor, "date_str": nowStr(),
		"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
		"purchase_returned_by": meID(t),
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 2, "purchase_unit_price": 60,
			"purchasereturn_unit_price": 60, "unit": "PC", "selected": true}},
	})
	pr := mustOK(t, "unrefunded purchase return", code, res)
	paymentFlow{
		path: "/v1/purchase-return-payment", link: map[string]interface{}{"purchase_return_id": str(pr, "id"), "purchase_id": str(p, "id")},
		docPath: "/v1/purchase-return/" + str(pr, "id"), due: 138,
	}.run(t, sid)
}

// ── Non-VAT sales returns ────────────────────────────────────────────────────

func TestFlow_NonVATSalesReturnRestoresStockAndRejectsOverReturn(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Non-VAT Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/non-vat-sales", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": agoStr(time.Hour), "vat_percent": 0,
		"products":       []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 3, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
		"payments_input": []map[string]interface{}{payment(300, agoStr(time.Hour))},
	})
	s := mustOK(t, "non-VAT sale of 3", code, res)
	wantStock(t, sid, widget, -3)

	ret := func(qty float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/non-vat-sales-return", map[string]interface{}{
			"store_id": sid, "non_vat_sales_id": str(s, "id"), "customer_id": customer, "date_str": nowStr(), "vat_percent": 0,
			"payment_status": "not_paid", "payments_input": []interface{}{},
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": qty, "unit_price": 100,
				"purchase_unit_price": 60, "unit": "PC", "selected": true}},
		})
	}
	code, res = ret(4)
	mustReject(t, "return more than sold", code, res, "quantity_0")
	code, res = ret(0)
	mustReject(t, "return of zero", code, res, "quantity_0")

	code, res = ret(2)
	r := mustOK(t, "return 2", code, res)
	wantNum(t, "return net_total", num(r, "net_total"), 200)
	wantStock(t, sid, widget, -1)

	code, res = ret(2)
	mustReject(t, "return more than what is left (1)", code, res, "quantity_0")
	wantStock(t, sid, widget, -1)
}

// ── Purchase requests ────────────────────────────────────────────────────────

func TestFlow_PurchaseRequestLeavesStockAlone(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	me := meID(t)
	body := func() map[string]interface{} {
		return map[string]interface{}{
			"store_id": sid, "date_str": nowStr(), "vat_percent": 15, "assigned_to": me,
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 5, "purchase_unit_price": 60, "unit": "PC"}},
		}
	}
	code, res := in(t, sid, "POST", "/v1/purchase-request", body())
	pr := mustOK(t, "purchase request", code, res)
	if str(pr, "code") == "" {
		t.Fatalf("purchase request has no code")
	}
	code, res = in(t, sid, "GET", "/v1/purchase-request/"+str(pr, "id"), nil)
	mustOK(t, "view purchase request", code, res)

	for name, edit := range map[string]func(map[string]interface{}){
		"no products":   func(b map[string]interface{}) { b["products"] = []interface{}{} },
		"no date":       func(b map[string]interface{}) { delete(b, "date_str") },
		"bad date":      func(b map[string]interface{}) { b["date_str"] = "yesterday" },
		"zero quantity": func(b map[string]interface{}) { b["products"].([]map[string]interface{})[0]["quantity"] = 0 },
		"no assignee":   func(b map[string]interface{}) { delete(b, "assigned_to") },
	} {
		b := body()
		edit(b)
		code, res := in(t, sid, "POST", "/v1/purchase-request", b)
		mustReject(t, name, code, res)
	}
	time.Sleep(time.Second)
	wantStock(t, sid, widget, 0)
}

// ── Previous / next / last navigation ────────────────────────────────────────

func TestFlow_SalesNavigation(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Navigation Customer", map[string]interface{}{"credit_limit": 5000})
	var ids []string
	for i := 3; i >= 1; i-- {
		code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, customer, agoStr(time.Duration(i)*time.Hour),
			[]line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}))
		ids = append(ids, str(mustOK(t, "sale", code, res), "id"))
	}
	code, res := in(t, sid, "GET", "/v1/last-order", nil)
	if got := str(mustOK(t, "last order", code, res), "id"); got != ids[2] {
		t.Fatalf("last order = %s, want the newest %s", got, ids[2])
	}
	code, res = in(t, sid, "GET", "/v1/previous-order/"+ids[1], nil)
	if got := str(mustOK(t, "previous order", code, res), "id"); got != ids[0] {
		t.Fatalf("previous of the middle sale = %s, want %s", got, ids[0])
	}
	code, res = in(t, sid, "GET", "/v1/next-order/"+ids[1], nil)
	if got := str(mustOK(t, "next order", code, res), "id"); got != ids[2] {
		t.Fatalf("next of the middle sale = %s, want %s", got, ids[2])
	}
	// Navigation never leaves the store.
	other := newStore(t)
	code, res = in(t, other, "GET", "/v1/last-order", nil)
	for _, id := range ids {
		if code == http.StatusOK && strings.Contains(string(res.Result), id) {
			t.Fatalf("another store's last-order returned this store's sale %s", id)
		}
	}
}
