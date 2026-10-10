//go:build e2e

package apie2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The dashboard is kept up to date in the background after every document;
// its month summary must add up to the documents that were entered.
func TestDashboard_MonthAddsUpToTheDocuments(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Dashboard Customer", map[string]interface{}{"credit_limit": 5000})
	vendor := createVendor(t, sid, "Dashboard Vendor")
	when := agoStr(time.Hour)

	code, res := in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, when,
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}, payment(690, when)))
	mustOK(t, "purchase 690", code, res)
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: widget, name: "Widget", qty: 2, price: 100, cost: 60}}, payment(230, when)))
	paid := mustOK(t, "paid sale 230", code, res)
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}))
	mustOK(t, "unpaid sale 115", code, res)
	code, res = in(t, sid, "POST", "/v1/sales-return", map[string]interface{}{
		"store_id": sid, "order_id": str(paid, "id"), "customer_id": customer, "date_str": nowStr(),
		"vat_percent": 15, "payment_status": "not_paid", "payments_input": []interface{}{},
		"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 1, "unit_price": 100,
			"purchase_unit_price": 60, "unit": "PC", "selected": true}},
	})
	mustOK(t, "return 1 (115)", code, res)
	code, res = in(t, sid, "POST", "/v1/expense-category", map[string]interface{}{"store_id": sid, "name": "Utilities"})
	cat := str(mustOK(t, "expense category", code, res), "id")
	code, res = in(t, sid, "POST", "/v1/expense", map[string]interface{}{
		"store_id": sid, "amount": 100, "description": "Electricity", "date_str": when, "payment_method": "cash",
		"category_id": []string{cat},
	})
	mustOK(t, "expense 100", code, res)

	month := time.Now().UTC().Format("2006-01")
	want := map[string]float64{
		"sales_amount": 345, "sales_count": 2, "unpaid_amount": 115, "payment_cash": 230,
		"sales_vat": 45, "sales_return_amount": 115, "sales_return_count": 1,
		"purchase_amount": 690, "purchase_vat": 90, "expense_amount": 100, "expense_count": 1,
	}
	eventually(t, "dashboard month", func() string {
		code, res := call(t, "GET", fmt.Sprintf("/v1/dashboard/monthly?store_id=%s&from_month=%s&to_month=%s", sid, month, month), authToken(t), nil)
		if code != 200 || !res.Status {
			return fmt.Sprintf("HTTP %d %v", code, res.Errors)
		}
		var rows []map[string]interface{}
		_ = json.Unmarshal(res.Result, &rows)
		var row map[string]interface{}
		for _, r := range rows {
			if str(r, "month_str") == month {
				row = r
			}
		}
		if row == nil {
			return "no row for " + month
		}
		var bad []string
		for k, v := range want {
			if !approx(num(row, k), v) {
				bad = append(bad, fmt.Sprintf("%s=%v want %v", k, num(row, k), v))
			}
		}
		return strings.Join(bad, ", ")
	})

	// Outstanding is what the customer owes net of what the store owes them:
	// 115 unpaid, less the 115 return not yet refunded, is nothing.
	eventually(t, "dashboard outstanding", func() string {
		code, res := call(t, "GET", "/v1/dashboard/outstanding?store_id="+sid, authToken(t), nil)
		if code != 200 || !res.Status {
			return fmt.Sprintf("HTTP %d %v", code, res.Errors)
		}
		var rows []map[string]interface{}
		_ = json.Unmarshal(res.Result, &rows)
		for _, r := range rows {
			if !approx(num(r, "outstanding"), 0) {
				return fmt.Sprintf("rows %v, want nothing outstanding", rows)
			}
		}
		return ""
	})
	// A second unpaid sale is then owed in full.
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: widget, name: "Widget", qty: 2, price: 100, cost: 60}}))
	mustOK(t, "unpaid sale 230", code, res)
	eventually(t, "dashboard outstanding after another unpaid sale", func() string {
		code, res := call(t, "GET", "/v1/dashboard/outstanding?store_id="+sid, authToken(t), nil)
		var rows []map[string]interface{}
		_ = json.Unmarshal(res.Result, &rows)
		if code != 200 || len(rows) != 1 || !approx(num(rows[0], "outstanding"), 230) {
			return fmt.Sprintf("HTTP %d rows %v, want one customer owing 230", code, rows)
		}
		return ""
	})

	// Another store's dashboard shows none of it.
	other := newStore(t)
	code, res = call(t, "GET", fmt.Sprintf("/v1/dashboard/monthly?store_id=%s&from_month=%s&to_month=%s", other, month, month), authToken(t), nil)
	if strings.Contains(string(res.Result), `"sales_amount":345`) {
		t.Fatalf("another store's dashboard shows this store's sales")
	}
}

// Double entry: over a mix of documents, every account's debits and credits
// add up to the same total, and the cash account holds the cash that moved.
func TestLedger_TrialBalanceAndCash(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	customer := createCustomer(t, sid, "Ledger Customer", map[string]interface{}{"credit_limit": 5000})
	vendor := createVendor(t, sid, "Ledger Vendor")
	when := agoStr(time.Hour)

	code, res := in(t, sid, "POST", "/v1/capital", map[string]interface{}{
		"store_id": sid, "amount": 2000, "date_str": when, "description": "Opening capital",
		"payment_method": "cash", "invested_by_user_id": meID(t),
	})
	mustOK(t, "capital 2000", code, res)
	code, res = in(t, sid, "POST", "/v1/purchase", purchaseBody(sid, vendor, when,
		[]line{{id: widget, name: "Widget", qty: 10, price: 60}}, payment(690, when)))
	mustOK(t, "cash purchase 690", code, res)
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: widget, name: "Widget", qty: 2, price: 100, cost: 60}}, payment(230, when)))
	mustOK(t, "cash sale 230", code, res)
	code, res = in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: widget, name: "Widget", qty: 1, price: 100, cost: 60}}))
	mustOK(t, "credit sale 115", code, res)
	code, res = in(t, sid, "POST", "/v1/expense-category", map[string]interface{}{"store_id": sid, "name": "Rent"})
	cat := str(mustOK(t, "expense category", code, res), "id")
	code, res = in(t, sid, "POST", "/v1/expense", map[string]interface{}{
		"store_id": sid, "amount": 100, "description": "Rent", "date_str": when, "payment_method": "cash", "category_id": []string{cat},
	})
	mustOK(t, "cash expense 100", code, res)

	eventually(t, "trial balance", func() string {
		code, res := in(t, sid, "GET", "/v1/account?limit=500", nil)
		if code != 200 || !res.Status {
			return fmt.Sprintf("HTTP %d %v", code, res.Errors)
		}
		var accounts []map[string]interface{}
		_ = json.Unmarshal(res.Result, &accounts)
		var debit, credit float64
		var cash map[string]interface{}
		for _, a := range accounts {
			debit += num(a, "debit_total")
			credit += num(a, "credit_total")
			if strings.EqualFold(str(a, "name"), "Cash") {
				cash = a
			}
		}
		if !approx(debit, credit) {
			return fmt.Sprintf("debits %.2f != credits %.2f", debit, credit)
		}
		if cash == nil {
			return "no Cash account"
		}
		// In: 2000 + 230. Out: 690 + 100.
		if !approx(num(cash, "debit_total"), 2230) || !approx(num(cash, "credit_total"), 790) || !approx(num(cash, "balance"), 1440) {
			return fmt.Sprintf("cash debit %v credit %v balance %v, want 2230 / 790 / 1440",
				num(cash, "debit_total"), num(cash, "credit_total"), num(cash, "balance"))
		}
		return ""
	})
}
