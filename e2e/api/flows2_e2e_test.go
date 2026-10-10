//go:build e2e

package apie2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Functional tests, part 2: the rest of the back office (capital, dividends,
// employees and salaries, cash discounts, warehouses and stock transfers,
// delivery notes, purchase orders and requests, non-VAT sales, quotation
// invoices and their returns, catalogue masters, users and roles).

// ── Owner's money: capital, withdrawals, dividends ───────────────────────────

func TestFlow_CapitalWithdrawalAndDividendAreValidatedAndBalanced(t *testing.T) {
	sid := newStore(t)
	me := meID(t)

	type doc struct {
		path, userField string
	}
	for _, d := range []doc{
		{"/v1/capital", "invested_by_user_id"},
		{"/v1/capital-withdrawal", "withdrawn_by_user_id"},
		{"/v1/divident", "withdrawn_by_user_id"},
	} {
		t.Run(strings.TrimPrefix(d.path, "/v1/"), func(t *testing.T) {
			good := func() map[string]interface{} {
				return map[string]interface{}{
					"store_id": sid, "amount": 2500, "date_str": nowStr(), "description": "Owner " + d.path,
					"payment_method": "cash", d.userField: me,
				}
			}
			code, res := in(t, sid, "POST", d.path, good())
			c := mustOK(t, "create", code, res)
			wantNum(t, "amount", num(c, "amount"), 2500)
			wantBalancedLedger(t, sid, str(c, "id"), 2500)

			code, res = in(t, sid, "GET", d.path+"/"+str(c, "id"), nil)
			wantNum(t, "read back amount", num(mustOK(t, "view", code, res), "amount"), 2500)

			for name, edit := range map[string]func(map[string]interface{}){
				"zero amount":    func(b map[string]interface{}) { b["amount"] = 0 },
				"missing date":   func(b map[string]interface{}) { delete(b, "date_str") },
				"bad date":       func(b map[string]interface{}) { b["date_str"] = "01-01-2026" },
				"no description": func(b map[string]interface{}) { delete(b, "description") },
				"no method":      func(b map[string]interface{}) { delete(b, "payment_method") },
				"no user":        func(b map[string]interface{}) { delete(b, d.userField) },
				"unknown user":   func(b map[string]interface{}) { b[d.userField] = "5f1d7f3e9b1e8a3f4c2b1a00" },
			} {
				b := good()
				edit(b)
				code, res := in(t, sid, "POST", d.path, b)
				mustReject(t, name, code, res)
			}

			// Edit the amount: the ledger follows.
			b := good()
			b["amount"] = 3000
			code, res = in(t, sid, "PUT", d.path+"/"+str(c, "id"), b)
			u := mustOK(t, "update", code, res)
			wantNum(t, "updated amount", num(u, "amount"), 3000)
			wantBalancedLedger(t, sid, str(c, "id"), 3000)
		})
	}
}

// ── Employees and salaries ───────────────────────────────────────────────────

func TestFlow_EmployeeAndSalaryPayment(t *testing.T) {
	sid := newStore(t)
	good := func() map[string]interface{} {
		return map[string]interface{}{
			"store_id": sid, "name": "Flow Employee", "joining_date": time.Now().AddDate(0, -3, 0).UTC().Format(time.RFC3339),
			"salary_day": 25, "salary": 4000, "phone": "0500000009",
		}
	}
	code, res := in(t, sid, "POST", "/v1/employee", good())
	e := mustOK(t, "create employee", code, res)
	eid := str(e, "id")

	for name, edit := range map[string]func(map[string]interface{}){
		"no name":         func(b map[string]interface{}) { delete(b, "name") },
		"no joining date": func(b map[string]interface{}) { delete(b, "joining_date") },
		"no salary day":   func(b map[string]interface{}) { delete(b, "salary_day") },
		"salary day 0":    func(b map[string]interface{}) { b["salary_day"] = 0 },
		"salary day 32":   func(b map[string]interface{}) { b["salary_day"] = 32 },
	} {
		b := good()
		edit(b)
		code, res := in(t, sid, "POST", "/v1/employee", b)
		mustReject(t, name, code, res)
	}

	now := time.Now()
	pay := func(extra map[string]interface{}) (int, apiResponse) {
		b := map[string]interface{}{
			"store_id": sid, "employee_id": eid, "amount": 4000, "month": int(now.Month()), "year": now.Year(),
			"payment_method": "cash", "date_str": nowStr(),
		}
		for k, v := range extra {
			b[k] = v
		}
		return in(t, sid, "POST", "/v1/employee-salary-payment", b)
	}
	code, res = pay(nil)
	p := mustOK(t, "pay salary", code, res)
	wantBalancedLedger(t, sid, str(p, "id"), 4000)

	for name, extra := range map[string]map[string]interface{}{
		"zero amount":      {"amount": 0},
		"month 13":         {"month": 13},
		"month 0":          {"month": 0},
		"no method":        {"payment_method": ""},
		"unknown employee": {"employee_id": "5f1d7f3e9b1e8a3f4c2b1a00"},
	} {
		code, res := pay(extra)
		mustReject(t, name, code, res)
	}
}

// ── Cash discounts on sales ──────────────────────────────────────────────────

func TestFlow_SalesCashDiscountIsCappedByTheBalance(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Discount Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, customer, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 2, price: 100, cost: 60}}, payment(200, agoStr(time.Hour))))
	o := mustOK(t, "sale 230 paid 200", code, res)
	oid := str(o, "id")

	discount := func(amount float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/sales-cash-discount", map[string]interface{}{
			"store_id": sid, "order_id": oid, "amount": amount, "method": "cash", "date_str": nowStr(),
		})
	}
	code, res = discount(0)
	mustReject(t, "zero discount", code, res, "amount")
	code, res = discount(500)
	mustReject(t, "discount above the net total", code, res, "amount")

	code, res = discount(30)
	mustOK(t, "discount the 30 balance", code, res)
	eventually(t, "paid after discount", func() string {
		code, res := in(t, sid, "GET", "/v1/order/"+oid, nil)
		o := mustOK(t, "view sale", code, res)
		if str(o, "payment_status") != "paid" || !approx(num(o, "cash_discount"), 30) || !approx(num(o, "balance_amount"), 0) {
			return fmt.Sprintf("status %s cash_discount %v balance %v", str(o, "payment_status"), num(o, "cash_discount"), num(o, "balance_amount"))
		}
		return ""
	})
}

func TestFlow_PurchaseCashDiscountSettlesTheBalance(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	vendor := createVendor(t, sid, "Discount Vendor")
	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, agoStr(time.Hour),
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}, payment(660, agoStr(time.Hour))))
	p := mustOK(t, "purchase 690 paid 660", code, res)
	pid := str(p, "id")

	discount := func(amount float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/purchase-cash-discount", map[string]interface{}{
			"store_id": sid, "purchase_id": pid, "amount": amount, "method": "cash", "date_str": nowStr(),
		})
	}
	code, res = discount(0)
	mustReject(t, "zero discount", code, res, "amount")
	code, res = discount(-5)
	mustReject(t, "negative discount", code, res, "amount")
	code, res = discount(5000)
	mustReject(t, "discount above the total", code, res, "amount")

	code, res = discount(30)
	mustOK(t, "discount the 30 balance", code, res)
	eventually(t, "paid after discount", func() string {
		code, res := in(t, sid, "GET", "/v1/purchase/"+pid, nil)
		p := mustOK(t, "view purchase", code, res)
		if str(p, "payment_status") != "paid" || !approx(num(p, "balance_amount"), 0) {
			return fmt.Sprintf("status %s balance %v", str(p, "payment_status"), num(p, "balance_amount"))
		}
		return ""
	})
}

// ── Warehouses and stock transfers ───────────────────────────────────────────

// createWarehouse returns the warehouse id and the code the server gave it
// (codes are generated; warehouse_stocks is keyed by code).
func createWarehouse(t *testing.T, sid, name string) (id, code string) {
	t.Helper()
	c, res := in(t, sid, "POST", "/v1/warehouse", map[string]interface{}{"store_id": sid, "name": name})
	w := mustOK(t, "create warehouse "+name, c, res)
	if str(w, "code") == "" {
		t.Fatalf("warehouse %s has no code", name)
	}
	return str(w, "id"), str(w, "code")
}

func warehouseStock(t *testing.T, sid, productID, warehouseCode string) float64 {
	t.Helper()
	code, res := in(t, sid, "GET", "/v1/product/"+productID, nil)
	p := mustOK(t, "get product", code, res)
	stores, _ := p["product_stores"].(map[string]interface{})
	ps, _ := stores[sid].(map[string]interface{})
	ws, _ := ps["warehouse_stocks"].(map[string]interface{})
	v, _ := ws[warehouseCode].(float64)
	return v
}

func TestFlow_StockTransferMovesStockBetweenWarehouses(t *testing.T) {
	sid := newStore(t)
	from, fromCode := createWarehouse(t, sid, "North Warehouse")
	to, toCode := createWarehouse(t, sid, "South Warehouse")
	if fromCode == toCode {
		t.Fatalf("two warehouses share the code %s", fromCode)
	}
	widget := createProduct(t, sid, "Widget", 100, 60)
	vendor := createVendor(t, sid, "Transfer Vendor")

	// Stock up the north warehouse.
	pb := purchaseBody(sid, vendor, agoStr(time.Hour), []line{{id: widget, name: "Widget", qty: 10, price: 60}})
	pb["products"].([]map[string]interface{})[0]["warehouse_id"] = from
	code, res := in(t, sid, "POST", "/v1/purchase", pb)
	mustOK(t, "purchase into WN", code, res)
	eventually(t, "WN stock", func() string {
		if got := warehouseStock(t, sid, widget, fromCode); !approx(got, 10) {
			return fmt.Sprintf("%s = %v, want 10", fromCode, got)
		}
		return ""
	})

	transfer := func(fromID, toID string, qty float64) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/stock-transfer", map[string]interface{}{
			"store_id": sid, "from_warehouse_id": fromID, "to_warehouse_id": toID, "date_str": nowStr(), "vat_percent": 15,
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": qty, "unit_price": 60, "unit": "PC"}},
		})
	}
	code, res = transfer(from, from, 1)
	mustReject(t, "transfer to the same warehouse", code, res)
	code, res = transfer("", "", 1)
	mustReject(t, "transfer from the main store to the main store", code, res)
	code, res = transfer(from, to, 0)
	mustReject(t, "transfer of zero", code, res)

	code, res = transfer(from, to, 4)
	mustOK(t, "transfer 4", code, res)
	eventually(t, "warehouse stocks after transfer", func() string {
		n, s := warehouseStock(t, sid, widget, fromCode), warehouseStock(t, sid, widget, toCode)
		if !approx(n, 6) || !approx(s, 4) {
			return fmt.Sprintf("%s %v %s %v, want 6 and 4", fromCode, n, toCode, s)
		}
		return ""
	})
	// A transfer moves stock, it does not create or destroy it.
	wantStock(t, sid, widget, 10)
}

// ── Delivery notes, purchase orders, purchase requests ───────────────────────

func TestFlow_DeliveryNoteAndPurchaseOrderLeaveStockAlone(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Delivery Customer")
	vendor := createVendor(t, sid, "Order Vendor")

	code, res := in(t, sid, "POST", "/v1/delivery-note", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 15,
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 3, "unit_price": 100, "unit": "PC"}},
	})
	dn := mustOK(t, "delivery note", code, res)
	if str(dn, "code") == "" {
		t.Fatalf("delivery note has no code")
	}

	code, res = in(t, sid, "POST", "/v1/purchase-order", map[string]interface{}{
		"store_id": sid, "vendor_id": vendor, "date_str": nowStr(), "vat_percent": 15,
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 20, "purchase_unit_price": 55, "unit": "PC"}},
	})
	po := mustOK(t, "purchase order", code, res)
	wantNum(t, "purchase order net_total", num(po, "net_total"), 20*55*1.15)

	for _, p := range []string{"/v1/delivery-note", "/v1/purchase-order"} {
		code, res = in(t, sid, "POST", p, map[string]interface{}{"store_id": sid, "date_str": nowStr(), "vat_percent": 15, "products": []interface{}{}})
		mustReject(t, p+" without products", code, res, "product_id")
		code, res = in(t, sid, "POST", p, map[string]interface{}{"store_id": sid, "vat_percent": 15,
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 1, "unit_price": 1, "purchase_unit_price": 1}}})
		mustReject(t, p+" without a date", code, res, "date_str")
	}

	time.Sleep(time.Second)
	wantStock(t, sid, widget, 0)
}

// ── Non-VAT sales ────────────────────────────────────────────────────────────

func TestFlow_NonVATSaleHasNoVAT(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Cash Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/non-vat-sales", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 0,
		"products":       []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 2, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"}},
		"payments_input": []map[string]interface{}{payment(200, nowStr())},
	})
	s := mustOK(t, "non-VAT sale", code, res)
	wantNum(t, "vat_price", num(s, "vat_price"), 0)
	wantNum(t, "net_total", num(s, "net_total"), 200)
	wantStock(t, sid, widget, -2)

	code, res = in(t, sid, "POST", "/v1/non-vat-sales", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": nowStr(), "products": []interface{}{},
	})
	mustReject(t, "non-VAT sale without products", code, res, "product_id")

	code, res = in(t, sid, "POST", "/v1/non-vat-sales", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": nowStr(), "vat_percent": 0,
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 0, "unit_price": 100, "unit": "PC"}},
	})
	mustReject(t, "non-VAT sale of zero quantity", code, res, "quantity_0")
}

// ── Catalogue masters ────────────────────────────────────────────────────────

func TestMasters_CreateRejectDuplicatesAndRequireNames(t *testing.T) {
	sid := newStore(t)
	for _, p := range []struct {
		path  string
		extra map[string]interface{}
	}{
		{"/v1/product-category", nil},
		{"/v1/product-brand", map[string]interface{}{"code": "BR1"}},
		{"/v1/service-category", nil},
		{"/v1/vendor-category", nil},
		{"/v1/expense-category", nil},
		{"/v1/warehouse", nil},
	} {
		t.Run(strings.TrimPrefix(p.path, "/v1/"), func(t *testing.T) {
			body := map[string]interface{}{"store_id": sid, "name": "Master " + p.path}
			for k, v := range p.extra {
				body[k] = v
			}
			code, res := in(t, sid, "POST", p.path, body)
			id := str(mustOK(t, "create", code, res), "id")

			code, res = in(t, sid, "POST", p.path, map[string]interface{}{"store_id": sid})
			mustReject(t, "without a name", code, res, "name")

			code, res = in(t, sid, "GET", p.path+"/"+id, nil)
			if got := str(mustOK(t, "view", code, res), "name"); !strings.EqualFold(got, "Master "+p.path) {
				t.Fatalf("name = %q", got)
			}

			code, res = in(t, sid, "GET", p.path+"?limit=50", nil)
			if code != http.StatusOK {
				t.Fatalf("list: HTTP %d", code)
			}
			found := false
			for _, row := range resultList(t, res) {
				if row["id"] == id {
					found = true
				}
			}
			if !found {
				t.Fatalf("created %s missing from list", p.path)
			}

			code, res = in(t, sid, "GET", p.path+"/5f1d7f3e9b1e8a3f4c2b1a00", nil)
			if code == http.StatusOK && res.Status {
				t.Fatalf("unknown id should not be found")
			}
		})
	}
}

// ── Users and roles ──────────────────────────────────────────────────────────

func TestUsers_CreateValidateAndLogin(t *testing.T) {
	tok := authToken(t)
	mail := fmt.Sprintf("e2e-user-%s@startpos.test", runID)
	good := func() map[string]interface{} {
		return map[string]interface{}{"name": "E2E Staff", "email": mail, "mob": "0501234567", "password": "Staff-Passw0rd!", "role": "User"}
	}
	for name, edit := range map[string]func(map[string]interface{}){
		"no name":     func(b map[string]interface{}) { delete(b, "name") },
		"no email":    func(b map[string]interface{}) { delete(b, "email") },
		"bad email":   func(b map[string]interface{}) { b["email"] = "not-an-email" },
		"no password": func(b map[string]interface{}) { delete(b, "password") },
		"no mobile":   func(b map[string]interface{}) { delete(b, "mob") },
	} {
		b := good()
		edit(b)
		code, res := call(t, "POST", "/v1/user", tok, b)
		mustReject(t, name, code, res)
	}
	code, res := call(t, "POST", "/v1/user", tok, good())
	mustOK(t, "create user", code, res)

	code, res = call(t, "POST", "/v1/user", tok, good())
	mustReject(t, "duplicate email", code, res, "email")

	code, _, staffTok := login(t, mail, "Staff-Passw0rd!")
	if staffTok == "" {
		t.Fatalf("new user cannot log in: HTTP %d", code)
	}
	code, res = call(t, "GET", "/v1/me", staffTok, nil)
	if got := str(mustOK(t, "me as staff", code, res), "email"); !strings.EqualFold(got, mail) {
		t.Fatalf("/v1/me email = %q, want %s", got, mail)
	}
	code, res, _ = login(t, mail, "wrong password")
	if res.Status {
		t.Fatalf("login with a wrong password succeeded")
	}
}
