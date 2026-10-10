//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Functional tests: whole business flows through the real server and a real
// MongoDB, checking what a shop owner relies on (totals with VAT, stock,
// payment status, customer/vendor balances, double-entry ledger, serials).

// ── Purchases ────────────────────────────────────────────────────────────────

func TestFlow_PurchaseAddsStockComputesVATAndPaymentStatus(t *testing.T) {
	sid := newStore(t)
	vendor := createVendor(t, sid, "Flow Vendor")
	widget := createProduct(t, sid, "Widget", 100, 60)
	gadget := createProduct(t, sid, "Gadget", 50, 20)

	// Fully paid: 10 x 60 + 5 x (20 - 2 discount) = 690; VAT 15% = 103.5.
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, nowStr(),
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}, {id: gadget, name: "Gadget", qty: 5, price: 20, discount: 2}},
		payment(793.5, nowStr())))
	p := mustOK(t, "create purchase", code, res)
	wantNum(t, "total", num(p, "total"), 690)
	wantNum(t, "vat_price", num(p, "vat_price"), 103.5)
	wantNum(t, "net_total", num(p, "net_total"), 793.5)
	if got := str(p, "payment_status"); got != "paid" {
		t.Fatalf("payment_status = %q, want paid", got)
	}
	wantNum(t, "balance_amount", num(p, "balance_amount"), 0)
	wantStock(t, sid, widget, 10)
	wantStock(t, sid, gadget, 5)
	wantBalancedLedger(t, sid, str(p, "id"), 793.5)

	// Partly paid, then unpaid.
	code, res = in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, nowStr(),
		[]line{{id: widget, name: "Widget", qty: 1, price: 60}}, payment(20, nowStr())))
	p = mustOK(t, "create partly paid purchase", code, res)
	if got := str(p, "payment_status"); got != "paid_partially" {
		t.Fatalf("payment_status = %q, want paid_partially", got)
	}
	wantNum(t, "balance_amount", num(p, "balance_amount"), 69-20)

	code, res = in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, nowStr(),
		[]line{{id: widget, name: "Widget", qty: 1, price: 60}}))
	p = mustOK(t, "create unpaid purchase", code, res)
	if got := str(p, "payment_status"); got != "not_paid" {
		t.Fatalf("payment_status = %q, want not_paid", got)
	}
	wantNum(t, "balance_amount", num(p, "balance_amount"), 69)
	wantStock(t, sid, widget, 12)
	wantBalancedLedger(t, sid, str(p, "id"), 0)
}

func TestFlow_PurchaseValidation(t *testing.T) {
	sid := newStore(t)
	vendor := createVendor(t, sid, "Validation Vendor")
	widget := createProduct(t, sid, "Widget", 100, 60)
	ok := []line{{id: widget, name: "Widget", qty: 1, price: 60}}

	cases := []struct {
		name string
		edit func(b map[string]interface{})
		keys []string
	}{
		{"no products", func(b map[string]interface{}) { b["products"] = []interface{}{} }, []string{"product_id"}},
		{"missing date", func(b map[string]interface{}) { delete(b, "date_str") }, []string{"date_str"}},
		{"bad date", func(b map[string]interface{}) { b["date_str"] = "10/10/2026" }, []string{"date_str"}},
		{"missing vat percent", func(b map[string]interface{}) { delete(b, "vat_percent") }, []string{"vat_percent"}},
		{"zero quantity", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 0, "purchase_unit_price": 60}}
		}, []string{"quantity_0"}},
		{"unknown product", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": "5f1d7f3e9b1e8a3f4c2b1a00", "name": "Ghost", "quantity": 1, "purchase_unit_price": 60}}
		}, []string{"product_id_0"}},
		{"payment above net total", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{payment(1000, nowStr())}
		}, []string{"total_payment"}},
		{"payment without method", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{{"date_str": nowStr(), "amount": 10}}
		}, []string{"payment_method_0"}},
		{"payment without amount", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{{"date_str": nowStr(), "method": "cash"}}
		}, []string{"payment_amount_0"}},
		{"payment with bad date", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{payment(10, "tomorrow")}
		}, []string{"payment_date_0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := purchaseBody(sid, vendor, nowStr(), ok)
			c.edit(b)
			code, res := in(t, sid, "POST", "/v1/purchase", b)
			mustReject(t, c.name, code, res, c.keys...)
		})
	}
	// None of the rejected purchases may have touched stock.
	wantStock(t, sid, widget, 0)
}

func TestFlow_PurchaseReturnRemovesStockAndRejectsOverReturn(t *testing.T) {
	sid := newStore(t)
	vendor := createVendor(t, sid, "Return Vendor")
	widget := createProduct(t, sid, "Widget", 100, 60)
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}))
	purchase := mustOK(t, "create purchase", code, res)
	wantStock(t, sid, widget, 10)

	returnBody := func(qty float64) map[string]interface{} {
		return map[string]interface{}{
			"store_id": sid, "purchase_id": str(purchase, "id"), "vendor_id": vendor, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
			"purchase_returned_by": meID(t),
			"products": []map[string]interface{}{{
				"product_id": widget, "name": "Widget", "quantity": qty, "purchase_unit_price": 60,
				"purchasereturn_unit_price": 60, "unit": "PC", "selected": true,
			}},
		}
	}

	code, res = in(t, sid, "POST", "/v1/purchase-return", returnBody(11))
	mustReject(t, "return more than purchased", code, res, "quantity_0", "net_total")

	code, res = in(t, sid, "POST", "/v1/purchase-return", returnBody(4))
	pr := mustOK(t, "return 4", code, res)
	wantNum(t, "return net_total", num(pr, "net_total"), 4*60*1.15)
	wantStock(t, sid, widget, 6)
	wantBalancedLedger(t, sid, str(pr, "id"), 0)

	code, res = in(t, sid, "POST", "/v1/purchase-return", returnBody(7))
	mustReject(t, "return more than what is left (6)", code, res, "quantity_0")
	wantStock(t, sid, widget, 6)

	code, res = in(t, sid, "POST", "/v1/purchase-return", returnBody(6))
	mustOK(t, "return the remaining 6", code, res)
	wantStock(t, sid, widget, 0)

	code, res = in(t, sid, "POST", "/v1/purchase-return", returnBody(1))
	mustReject(t, "everything already returned", code, res, "quantity_0")
	wantStock(t, sid, widget, 0)
}

// ── Sales ────────────────────────────────────────────────────────────────────

func TestFlow_SaleReducesStockAndUpdatesCustomerAndLedger(t *testing.T) {
	sid := newStore(t)
	vendor := createVendor(t, sid, "Stock Vendor")
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Credit Customer", map[string]interface{}{"credit_limit": 5000})
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}))
	mustOK(t, "stock up", code, res)
	wantStock(t, sid, widget, 10)

	// 3 x (100 - 5 discount) = 285, VAT 42.75, net 327.75; paid 200.
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, nowStr(),
		[]line{{id: widget, name: "Widget", qty: 3, price: 100, cost: 60, discount: 5}}, payment(200, nowStr())))
	o := mustOK(t, "create sale", code, res)
	wantNum(t, "total", num(o, "total"), 285)
	wantNum(t, "vat_price", num(o, "vat_price"), 42.75)
	wantNum(t, "net_total", num(o, "net_total"), 327.75)
	wantNum(t, "balance_amount", num(o, "balance_amount"), 127.75)
	wantNum(t, "profit", num(o, "profit"), 3*(95-60))
	if got := str(o, "payment_status"); got != "paid_partially" {
		t.Fatalf("payment_status = %q, want paid_partially", got)
	}
	if !strings.HasPrefix(str(o, "code"), "SAL-") {
		t.Fatalf("code %q should use the store's sales prefix", str(o, "code"))
	}
	wantStock(t, sid, widget, 7)
	wantBalancedLedger(t, sid, str(o, "id"), 327.75)

	eventually(t, "customer credit balance", func() string {
		code, res := in(t, sid, "GET", "/v1/customer/"+customer, nil)
		c := mustOK(t, "get customer", code, res)
		if got := num(c, "credit_balance"); !approx(got, 127.75) {
			return fmt.Sprintf("credit_balance = %v, want 127.75", got)
		}
		return ""
	})

	// Reading it back gives the same figures.
	code, res = in(t, sid, "GET", "/v1/order/"+str(o, "id"), nil)
	back := mustOK(t, "view sale", code, res)
	wantNum(t, "stored net_total", num(back, "net_total"), 327.75)
}

func TestFlow_SaleNetTotalMatchesCalculateEndpoint(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	// Awkward numbers: thirds and a shipping fee, to catch rounding drift.
	body := saleBody(sid, "", nowStr(), []line{{id: widget, name: "Widget", qty: 3, price: 33.33, cost: 10}})
	body["shipping_handling_fees"] = 7.77
	body["discount"] = 1.11

	code, res := in(t, sid, "POST", "/v1/order/calculate-net-total", body)
	calc := mustOK(t, "calculate", code, res)

	code, res = in(t, sid, "POST", "/v1/order", body)
	o := mustOK(t, "create", code, res)
	for _, k := range []string{"total", "vat_price", "net_total"} {
		wantNum(t, k+" (calculate vs create)", num(calc, k), num(o, k))
	}
	// base = 99.99 + 7.77 - 1.11 = 106.65; VAT = 16.00; net = 122.65
	wantNum(t, "net_total", num(o, "net_total"), 122.65)
	wantNum(t, "vat_price", num(o, "vat_price"), 16)
}

func TestFlow_SaleValidation(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	poor := createCustomer(t, sid, "Low Limit Customer", map[string]interface{}{"credit_limit": 10})
	saudi := createCustomer(t, sid, "Saudi Customer", map[string]interface{}{"country_code": "SA", "credit_limit": 1000})
	foreign := createCustomer(t, newStore(t), "Other Store Customer")
	ok := []line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}

	cases := []struct {
		name string
		edit func(b map[string]interface{})
		keys []string
	}{
		{"no products", func(b map[string]interface{}) { b["products"] = []interface{}{} }, []string{"product_id"}},
		{"missing date", func(b map[string]interface{}) { delete(b, "date_str") }, []string{"date_str"}},
		{"bad date", func(b map[string]interface{}) { b["date_str"] = "yesterday" }, []string{"date_str"}},
		{"missing vat percent", func(b map[string]interface{}) { delete(b, "vat_percent") }, []string{"vat_percent"}},
		{"zero quantity", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 0, "unit_price": 100}}
		}, []string{"quantity_0"}},
		{"missing unit price", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 1}}
		}, []string{"unit_price_0"}},
		{"discount above price", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 1, "unit_price": 100, "unit_discount": 150}}
		}, []string{"unit_discount_0", "cash_discount", "total_payment"}},
		{"short product name", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": widget, "name": "W", "quantity": 1, "unit_price": 100}}
		}, []string{"name_0"}},
		{"unknown product", func(b map[string]interface{}) {
			b["products"] = []map[string]interface{}{{"product_id": "5f1d7f3e9b1e8a3f4c2b1a00", "name": "Ghost", "quantity": 1, "unit_price": 100}}
		}, []string{"product_id_0"}},
		{"payment above net total", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{payment(116, nowStr())}
		}, []string{"total_payment"}},
		{"payment with bad date", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{payment(10, "tomorrow")}
		}, []string{"payment_date_0"}},
		{"payment with zero amount", func(b map[string]interface{}) {
			b["payments_input"] = []map[string]interface{}{{"date_str": nowStr(), "amount": 0, "method": "cash"}}
		}, []string{"payment_amount_0"}},
		{"cash discount above net total", func(b map[string]interface{}) { b["cash_discount"] = 500 }, []string{"cash_discount"}},
		{"negative cash discount", func(b map[string]interface{}) { b["cash_discount"] = -1 }, []string{"cash_discount", "discount"}},
		{"bad phone", func(b map[string]interface{}) { b["phone"] = "12" }, []string{"phone"}},
		{"VAT no. wrong length", func(b map[string]interface{}) { b["customer_id"] = saudi; b["vat_no"] = "3001" }, []string{"vat_no"}},
		{"VAT no. not starting/ending with 3", func(b map[string]interface{}) { b["customer_id"] = saudi; b["vat_no"] = "123456789012345" }, []string{"vat_no"}},
		{"credit limit exceeded", func(b map[string]interface{}) { b["customer_id"] = poor }, []string{"customer_credit_limit", "customer_id"}},
		{"unknown customer id", func(b map[string]interface{}) { b["customer_id"] = "5f1d7f3e9b1e8a3f4c2b1a00" }, []string{"customer_id"}},
		{"customer from another store", func(b map[string]interface{}) { b["customer_id"] = foreign }, []string{"customer_id"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := saleBody(sid, "", nowStr(), ok)
			c.edit(b)
			code, res := in(t, sid, "POST", "/v1/order", b)
			mustReject(t, c.name, code, res, c.keys...)
		})
	}

	code, res := in(t, sid, "GET", "/v1/order?limit=100", nil)
	if code != http.StatusOK || len(resultList(t, res)) != 0 {
		t.Fatalf("rejected sales must not be saved, list has %d", len(resultList(t, res)))
	}
}

func TestFlow_SalesInvoiceCodesAreUniqueAndSequentialUnderConcurrency(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 10, 5)
	const n = 8
	var wg sync.WaitGroup
	codes := make([]string, n)
	errs := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, "", nowStr(),
				[]line{{id: widget, name: "Widget", qty: 1, price: 10, cost: 5}}, payment(11.5, nowStr())))
			if code != http.StatusOK || !res.Status {
				errs[i] = fmt.Sprintf("HTTP %d %v", code, res.Errors)
				return
			}
			codes[i] = str(resultMap(t, res), "code")
		}(i)
	}
	wg.Wait()
	for i, e := range errs {
		if e != "" {
			t.Fatalf("concurrent sale %d failed: %s", i, e)
		}
	}
	sort.Strings(codes)
	for i, c := range codes {
		want := fmt.Sprintf("SAL--%04d", i+1)
		if c != want {
			t.Fatalf("codes = %v, want SAL--0001..SAL--%04d with no gaps or duplicates", codes, n)
		}
	}
	wantStock(t, sid, widget, -n)
}

func TestFlow_SalePaymentsMovePaymentStatus(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Paying Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, customer, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}))
	o := mustOK(t, "unpaid sale", code, res)
	oid := str(o, "id")
	if str(o, "payment_status") != "not_paid" {
		t.Fatalf("payment_status = %q, want not_paid", str(o, "payment_status"))
	}

	pay := func(amount float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/sales-payment", map[string]interface{}{
			"store_id": sid, "order_id": oid, "amount": amount, "method": "cash", "date_str": nowStr(),
		})
	}
	status := func() map[string]interface{} {
		code, res := in(t, sid, "GET", "/v1/order/"+oid, nil)
		return mustOK(t, "view sale", code, res)
	}

	code, res = pay(200)
	mustReject(t, "payment above net total", code, res, "amount")
	code, res = pay(-5)
	mustReject(t, "negative payment", code, res, "amount")
	code, res = pay(0)
	mustReject(t, "zero payment", code, res, "amount")

	code, res = pay(15)
	first := mustOK(t, "pay 15", code, res)
	eventually(t, "partly paid", func() string {
		o := status()
		if str(o, "payment_status") != "paid_partially" || !approx(num(o, "balance_amount"), 100) {
			return fmt.Sprintf("status %s balance %v", str(o, "payment_status"), num(o, "balance_amount"))
		}
		return ""
	})

	code, res = pay(101)
	mustReject(t, "second payment above the balance", code, res, "amount")

	code, res = pay(100)
	mustOK(t, "pay the rest", code, res)
	eventually(t, "paid", func() string {
		o := status()
		if str(o, "payment_status") != "paid" || !approx(num(o, "balance_amount"), 0) {
			return fmt.Sprintf("status %s balance %v", str(o, "payment_status"), num(o, "balance_amount"))
		}
		return ""
	})

	code, res = in(t, sid, "DELETE", "/v1/sales-payment/"+str(first, "id"), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("delete first payment: HTTP %d %v", code, res.Errors)
	}
	eventually(t, "back to partly paid", func() string {
		o := status()
		if str(o, "payment_status") != "paid_partially" || !approx(num(o, "balance_amount"), 15) {
			return fmt.Sprintf("status %s balance %v", str(o, "payment_status"), num(o, "balance_amount"))
		}
		return ""
	})
	wantBalancedLedger(t, sid, oid, 0)
}

func TestFlow_UpdateSaleAdjustsStock(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	gadget := createProduct(t, sid, "Gadget", 50, 20)
	body := saleBody(sid, "", agoStr(time.Hour), []line{{id: widget, name: "Widget", qty: 3, price: 100, cost: 60}}, payment(345, agoStr(time.Hour)))
	code, res := in(t, sid, "POST", "/v1/order", body)
	o := mustOK(t, "create sale", code, res)
	wantStock(t, sid, widget, -3)

	// Change quantity 3 -> 1 and add a gadget line.
	body = saleBody(sid, "", agoStr(time.Hour), []line{
		{id: widget, name: "Widget", qty: 1, price: 100, cost: 60},
		{id: gadget, name: "Gadget", qty: 2, price: 50, cost: 20},
	}, payment(230, agoStr(time.Hour)))
	body["id"] = str(o, "id")
	code, res = in(t, sid, "PUT", "/v1/order/"+str(o, "id"), body)
	u := mustOK(t, "update sale", code, res)
	wantNum(t, "updated net_total", num(u, "net_total"), 230)
	wantStock(t, sid, widget, -1)
	wantStock(t, sid, gadget, -2)
	wantBalancedLedger(t, sid, str(o, "id"), 230)
}

// ── Sales returns ────────────────────────────────────────────────────────────

func TestFlow_SalesReturnRestoresStockAndRejectsOverReturn(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Returning Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, customer, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 5, price: 100, cost: 60}}, payment(575, agoStr(time.Hour))))
	o := mustOK(t, "sale of 5", code, res)
	wantStock(t, sid, widget, -5)

	ret := func(qty float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/sales-return", map[string]interface{}{
			"store_id": sid, "order_id": str(o, "id"), "customer_id": customer, "date_str": nowStr(),
			"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
			"products": []map[string]interface{}{{
				"product_id": widget, "name": "Widget", "quantity": qty, "unit_price": 100,
				"purchase_unit_price": 60, "unit": "PC", "selected": true,
			}},
		})
	}

	code, res = ret(6)
	mustReject(t, "return more than sold", code, res, "quantity_0")

	code, res = ret(2)
	sr := mustOK(t, "return 2", code, res)
	wantNum(t, "return net_total", num(sr, "net_total"), 230)
	wantStock(t, sid, widget, -3)
	wantBalancedLedger(t, sid, str(sr, "id"), 0)

	code, res = ret(4)
	mustReject(t, "return more than what is left (3)", code, res, "quantity_0")

	code, res = ret(3)
	mustOK(t, "return the remaining 3", code, res)
	wantStock(t, sid, widget, 0)

	code, res = ret(1)
	mustReject(t, "everything already returned", code, res, "quantity_0")

	eventually(t, "sale return totals", func() string {
		code, res := in(t, sid, "GET", "/v1/order/"+str(o, "id"), nil)
		back := mustOK(t, "view sale", code, res)
		if int(num(back, "return_count")) != 2 {
			return fmt.Sprintf("return_count %v, want 2", num(back, "return_count"))
		}
		return ""
	})
}

func TestFlow_SalesReturnNeedsAValidSale(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	base := func() map[string]interface{} {
		return map[string]interface{}{
			"store_id": sid, "date_str": nowStr(), "vat_percent": 15, "payment_status": "not_paid",
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 1, "unit_price": 100, "selected": true}},
		}
	}
	b := base()
	code, res := in(t, sid, "POST", "/v1/sales-return", b)
	mustReject(t, "no order id", code, res, "order_id")

	b = base()
	b["order_id"] = "5f1d7f3e9b1e8a3f4c2b1a00"
	code, res = in(t, sid, "POST", "/v1/sales-return", b)
	mustReject(t, "unknown order", code, res, "order_id")
}

// ── Quotations, expenses, deposits ───────────────────────────────────────────

func TestFlow_QuotationDoesNotTouchStock(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Quote Customer")
	code, res := in(t, sid, "POST", "/v1/quotation", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 15, "type": "quotation",
		"validity_days": 7, "delivery_days": 3,
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 4, "unit_price": 100, "unit": "PC"}},
	})
	q := mustOK(t, "create quotation", code, res)
	wantNum(t, "quotation net_total", num(q, "net_total"), 460)
	if !strings.HasPrefix(str(q, "code"), "QUO-") {
		t.Fatalf("quotation code %q should use the QUO- prefix", str(q, "code"))
	}
	time.Sleep(time.Second)
	wantStock(t, sid, widget, 0)

	code, res = in(t, sid, "POST", "/v1/quotation", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 15, "products": []interface{}{},
		"validity_days": 7, "delivery_days": 3,
	})
	mustReject(t, "quotation without products", code, res)
}

func TestFlow_ExpenseCreateValidateAndLedger(t *testing.T) {
	sid := newStore(t)
	code, res := in(t, sid, "POST", "/v1/expense-category", map[string]interface{}{"store_id": sid, "name": "Rent"})
	cat := mustOK(t, "expense category", code, res)

	good := func() map[string]interface{} {
		return map[string]interface{}{
			"store_id": sid, "amount": 1500, "description": "October rent", "date_str": nowStr(),
			"payment_method": "cash", "category_id": []string{str(cat, "id")},
		}
	}
	code, res = in(t, sid, "POST", "/v1/expense", good())
	e := mustOK(t, "create expense", code, res)
	wantNum(t, "amount", num(e, "amount"), 1500)
	wantBalancedLedger(t, sid, str(e, "id"), 1500)

	for name, edit := range map[string]func(map[string]interface{}){
		"zero amount":      func(b map[string]interface{}) { b["amount"] = 0 },
		"negative amount":  func(b map[string]interface{}) { b["amount"] = -10 },
		"missing date":     func(b map[string]interface{}) { delete(b, "date_str") },
		"bad date":         func(b map[string]interface{}) { b["date_str"] = "soon" },
		"missing method":   func(b map[string]interface{}) { delete(b, "payment_method") },
		"no description":   func(b map[string]interface{}) { delete(b, "description") },
		"missing category": func(b map[string]interface{}) { delete(b, "category_id") },
	} {
		t.Run(name, func(t *testing.T) {
			b := good()
			edit(b)
			code, res := in(t, sid, "POST", "/v1/expense", b)
			mustReject(t, name, code, res)
		})
	}
}

func TestFlow_CustomerDepositAndWithdrawalAreBalanced(t *testing.T) {
	sid := newStore(t)
	customer := createCustomer(t, sid, "Deposit Customer")
	code, res := in(t, sid, "POST", "/v1/customer-deposit", map[string]interface{}{
		"store_id": sid, "type": "customer", "customer_id": customer, "date_str": nowStr(),
		"payments": []map[string]interface{}{{"date_str": nowStr(), "amount": 300, "method": "cash"}},
	})
	d := mustOK(t, "deposit", code, res)
	wantBalancedLedger(t, sid, str(d, "id"), 300)

	code, res = in(t, sid, "POST", "/v1/customer-withdrawal", map[string]interface{}{
		"store_id": sid, "type": "customer", "customer_id": customer, "date_str": nowStr(),
		"payments": []map[string]interface{}{{"date_str": nowStr(), "amount": 100, "method": "cash"}},
	})
	w := mustOK(t, "withdrawal", code, res)
	wantBalancedLedger(t, sid, str(w, "id"), 100)

	code, res = in(t, sid, "POST", "/v1/customer-deposit", map[string]interface{}{
		"store_id": sid, "type": "customer", "customer_id": customer, "date_str": nowStr(),
		"payments": []map[string]interface{}{{"date_str": nowStr(), "amount": -5, "method": "cash"}},
	})
	mustReject(t, "deposit with a negative payment", code, res)

	code, res = in(t, sid, "POST", "/v1/customer-deposit", map[string]interface{}{
		"store_id": sid, "type": "customer", "date_str": nowStr(),
		"payments": []map[string]interface{}{{"date_str": nowStr(), "amount": 5, "method": "cash"}},
	})
	mustReject(t, "deposit without a customer", code, res, "customer_id")
}

// ── Store isolation ──────────────────────────────────────────────────────────

func TestFlow_StoresAreIsolated(t *testing.T) {
	a := newStore(t)
	b := newStore(t)
	widget := createProduct(t, a, "Store A Widget", 100, 60)
	createCustomer(t, a, "Store A Customer")

	code, res := in(t, b, "GET", "/v1/product?limit=100", nil)
	if code != http.StatusOK {
		t.Fatalf("list products in B: HTTP %d", code)
	}
	for _, p := range resultList(t, res) {
		if p["id"] == widget {
			t.Fatalf("store A's product is listed in store B")
		}
	}
	code, res = in(t, b, "GET", "/v1/customer?limit=100", nil)
	if n := len(resultList(t, res)); code != http.StatusOK || n != 0 {
		t.Fatalf("store B should have no customers, got %d (HTTP %d)", n, code)
	}
	// Selling store A's product from store B must not move A's stock.
	code, res = in(t, b, "POST", "/v1/order", saleBody(b, "", nowStr(),
		[]line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}))
	if code == http.StatusOK && res.Status {
		time.Sleep(time.Second)
	}
	wantStock(t, a, widget, 0)
}
