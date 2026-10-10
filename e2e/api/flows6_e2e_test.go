//go:build e2e

package apie2e

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fmtAmount(v float64) string { return "paid " + strconv.FormatFloat(v, 'f', -1, 64) }

// Functional tests, part 6: the frontend thread's reports (rounding on full
// refunds, non-VAT returns from the app's form, validation status codes,
// blank names, user rules, role permissions on writes).

// 75.50 + 15% VAT is 86.825: the sale charges 86.83 and a full return must be
// able to refund all of it.
func TestFlow_FullRefundOfAHalfCentSale(t *testing.T) {
	for _, paid := range []float64{86.83, 86.825} {
		t.Run(fmtAmount(paid), func(t *testing.T) { fullRefundOfAHalfCentSale(t, paid) })
	}
}

func fullRefundOfAHalfCentSale(t *testing.T, paid float64) {
	sid := newStore(t)
	item := createProduct(t, sid, "Half Cent Item", 75.50, 50)
	customer := createCustomer(t, sid, "Rounding Customer")
	when := agoStr(time.Hour)
	code, res := in(t, sid, "POST", "/v1/order", saleBody(sid, customer, when,
		[]line{{id: item, name: "Half Cent Item", qty: 1, price: 75.50, cost: 50}}, payment(paid, when)))
	o := mustOK(t, "sale of 75.50 + VAT", code, res)
	wantNum(t, "sale net_total", num(o, "net_total"), 86.83)

	code, res = in(t, sid, "POST", "/v1/sales-return", map[string]interface{}{
		"store_id": sid, "order_id": str(o, "id"), "customer_id": customer, "date_str": nowStr(), "vat_percent": 15,
		"payment_status": "paid", "payments_input": []map[string]interface{}{payment(86.83, nowStr())},
		"products": []map[string]interface{}{{"product_id": item, "name": "Half Cent Item", "quantity": 1, "unit_price": 75.50,
			"purchase_unit_price": 50, "unit": "PC", "selected": true}},
	})
	sr := mustOK(t, "full refund of 86.83", code, res)
	wantNum(t, "return net_total", num(sr, "net_total"), 86.83)
	wantBalancedLedger(t, sid, str(sr, "id"), 0)
}

// staff creates a user as the admin and signs them in.
func staff(t *testing.T, label, role string, stores []string, extra ...map[string]interface{}) (id, token string) {
	t.Helper()
	mail := "e2e-" + label + "-" + runID + "@startpos.test"
	body := map[string]interface{}{"name": "E2E " + label, "email": mail, "mob": "0501234571",
		"password": "Staff-Passw0rd!", "role": role, "store_ids": stores}
	for _, e := range extra {
		for k, v := range e {
			body[k] = v
		}
	}
	code, res := call(t, "POST", "/v1/user", authToken(t), body)
	u := mustOK(t, "create "+label, code, res)
	_, _, token = login(t, mail, "Staff-Passw0rd!")
	if token == "" {
		t.Fatalf("%s cannot log in", label)
	}
	return str(u, "id"), token
}

func wantStatus(t *testing.T, what string, code int, res apiResponse, want int) {
	t.Helper()
	if code != want || res.Status {
		t.Fatalf("%s: HTTP %d status=%v %v, want %d", what, code, res.Status, res.Errors, want)
	}
}

// The app's non-VAT return form sends no "selected" flag; those lines count
// as selected, restore stock, and an explicit "selected": false stays out.
func TestFlow_NonVATReturnFromTheAppsForm(t *testing.T) {
	sid := newStore(t)
	a := createProduct(t, sid, "Widget A", 100, 60)
	b := createProduct(t, sid, "Widget B", 100, 60)
	customer := createCustomer(t, sid, "Form Customer", map[string]interface{}{"credit_limit": 1000})
	code, res := in(t, sid, "POST", "/v1/non-vat-sales", map[string]interface{}{
		"store_id": sid, "customer_id": customer, "date_str": agoStr(time.Hour), "vat_percent": 0,
		"products": []map[string]interface{}{
			{"product_id": a, "name": "Widget A", "quantity": 3, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"},
			{"product_id": b, "name": "Widget B", "quantity": 2, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"},
		},
		"payments_input": []map[string]interface{}{payment(500, agoStr(time.Hour))},
	})
	s := mustOK(t, "non-VAT sale", code, res)
	wantStock(t, sid, a, -3)

	code, res = in(t, sid, "POST", "/v1/non-vat-sales-return", map[string]interface{}{
		"store_id": sid, "non_vat_sales_id": str(s, "id"), "customer_id": customer, "date_str": nowStr(), "vat_percent": 0,
		"payment_status": "not_paid", "payments_input": []interface{}{},
		"products": []map[string]interface{}{
			{"product_id": a, "name": "Widget A", "quantity": 1, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC"},
			{"product_id": b, "name": "Widget B", "quantity": 2, "unit_price": 100, "purchase_unit_price": 60, "unit": "PC", "selected": false},
		},
	})
	ret := mustOK(t, "return without selected flags", code, res)
	wantNum(t, "return net_total (only line A)", num(ret, "net_total"), 100)
	wantStock(t, sid, a, -2)
	wantStock(t, sid, b, -2)
}

// Validation errors answer 400, not 200 with "status": false, and a name
// made only of spaces is no name.
func TestFlow_ValidationErrorsAre400AndBlankNamesAreRefused(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	vendor := createVendor(t, sid, "PO Vendor")
	for _, c := range []struct {
		what, path string
		body map[string]interface{}
	}{
		{"customer package without a name", "/v1/customer-package", map[string]interface{}{"store_id": sid, "price": 10}},
		{"customer package named with spaces", "/v1/customer-package", map[string]interface{}{"store_id": sid, "name": "   ", "price": 10}},
		{"service category named with spaces", "/v1/service-category", map[string]interface{}{"store_id": sid, "name": "   "}},
		{"product category named with spaces", "/v1/product-category", map[string]interface{}{"store_id": sid, "name": "   "}},
		{"vendor named with spaces", "/v1/vendor", map[string]interface{}{"store_id": sid, "name": "  "}},
		{"signature without a store", "/v1/signature", map[string]interface{}{"name": "Manager"}},
		{"user role without a name", "/v1/user-role", map[string]interface{}{"store_id": sid}},
		{"purchase order without products", "/v1/purchase-order", map[string]interface{}{"store_id": sid, "vendor_id": vendor,
			"date_str": nowStr(), "vat_percent": 15, "products": []interface{}{}}},
		{"purchase request of zero", "/v1/purchase-request", map[string]interface{}{"store_id": sid, "date_str": nowStr(),
			"vat_percent": 15, "assigned_to": meID(t),
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 0, "unit_price": 60, "unit": "PC"}}}},
	} {
		code, res := in(t, sid, "POST", c.path, c.body)
		wantStatus(t, c.what, code, res, http.StatusBadRequest)
	}
}

func TestSecurity_UserRules(t *testing.T) {
	tok := authToken(t)
	sid := newStore(t)
	mail := "e2e-short-" + runID + "@startpos.test"
	code, res := call(t, "POST", "/v1/user", tok, map[string]interface{}{"name": "Short Password", "email": mail,
		"mob": "0501234572", "password": "123", "role": "SalesMan", "store_ids": []string{sid}})
	wantStatus(t, "password 123", code, res, http.StatusBadRequest)

	code, res = call(t, "POST", "/v1/user", tok, map[string]interface{}{"name": "Good Password", "email": mail,
		"mob": "0501234572", "password": "123456", "role": "SalesMan", "store_ids": []string{sid}})
	u := mustOK(t, "password of 6", code, res)
	if p := str(u, "password"); p != "" {
		t.Fatalf("user create returned the password hash")
	}
	code, res = call(t, "PUT", "/v1/user/"+str(u, "id"), tok, map[string]interface{}{"name": "Good Password", "email": mail,
		"mob": "0501234572", "password": "12", "role": "SalesMan", "store_ids": []string{sid}})
	wantStatus(t, "changing to a 2-character password", code, res, http.StatusBadRequest)

	_, salesman := staff(t, "rules-sm", "SalesMan", []string{sid})
	code, res = call(t, "POST", "/v1/user", salesman, map[string]interface{}{"name": "By SalesMan",
		"email": "e2e-bysm-" + runID + "@startpos.test", "mob": "0501234573", "password": "Staff-Passw0rd!",
		"role": "SalesMan", "store_ids": []string{sid}})
	wantStatus(t, "a SalesMan creating a user", code, res, http.StatusForbidden)

	_, manager := staff(t, "rules-mgr", "Manager", []string{sid})
	code, res = call(t, "PUT", "/v1/user/"+str(u, "id"), manager, map[string]interface{}{"name": "Good Password", "email": mail,
		"mob": "0501234572", "role": "Admin", "store_ids": []string{sid}})
	wantStatus(t, "a Manager making their SalesMan an Admin", code, res, http.StatusForbidden)
}

func TestSecurity_PurchaseRequestDecisionsBelongToTheAssignee(t *testing.T) {
	sid := newStore(t)
	widget := createProduct(t, sid, "Widget", 100, 60)
	assignee, assigneeTok := staff(t, "pr-assignee", "Manager", []string{sid})
	_, otherTok := staff(t, "pr-other", "Manager", []string{sid})
	newPR := func() string {
		code, res := in(t, sid, "POST", "/v1/purchase-request", map[string]interface{}{
			"store_id": sid, "date_str": nowStr(), "vat_percent": 15, "assigned_to": assignee,
			"products": []map[string]interface{}{{"product_id": widget, "name": "Widget", "quantity": 5, "unit_price": 60, "unit": "PC"}},
		})
		return str(mustOK(t, "purchase request", code, res), "id")
	}
	pr := newPR()
	for _, action := range []string{"accept", "reject"} {
		code, res := call(t, "POST", "/v1/purchase-request/"+pr+"/"+action+"?search[store_id]="+sid, otherTok, map[string]interface{}{})
		wantStatus(t, "someone else trying to "+action, code, res, http.StatusForbidden)
	}
	code, res := call(t, "POST", "/v1/purchase-request/"+pr+"/accept?search[store_id]="+sid, assigneeTok, map[string]interface{}{})
	if got := str(mustOK(t, "the assignee accepts", code, res), "status"); got != "accepted" {
		t.Fatalf("status = %q, want accepted", got)
	}
	code, res = call(t, "POST", "/v1/purchase-request/"+newPR()+"/reject?search[store_id]="+sid, authToken(t), map[string]interface{}{})
	mustOK(t, "an admin rejects", code, res)
}

// In a store with the RBAC module on, a role that grants only "read" on
// products can't create one; resources no role mentions stay open, and with
// the module off roles are not enforced.
func TestSecurity_RolePermissionsApplyToWrites(t *testing.T) {
	tok := authToken(t)
	for _, rbacOn := range []bool{true, false} {
		sid := newStore(t, map[string]interface{}{"settings": map[string]interface{}{"enable_rbac_module": rbacOn}})
		code, res := call(t, "POST", "/v1/user-role", tok, map[string]interface{}{"store_id": sid, "name": "Read-only products",
			"permissions": []map[string]interface{}{
				{"resource": "products", "read": true},
				{"resource": "vendors", "read": true, "create": true},
			}})
		role := mustOK(t, "role", code, res)
		label := "rbac-off"
		if rbacOn {
			label = "rbac-on"
		}
		_, sm := staff(t, label, "SalesMan", []string{sid}, map[string]interface{}{"role_ids": []string{str(role, "id")}})
		product := map[string]interface{}{"store_id": sid, "name": "Role Widget", "unit": "PC",
			"product_stores": map[string]interface{}{sid: map[string]interface{}{"store_id": sid, "retail_unit_price": 10, "purchase_unit_price": 5}}}
		code, res = call(t, "POST", "/v1/product", sm, product)
		if rbacOn {
			wantStatus(t, "create a product with read-only products", code, res, http.StatusForbidden)
		} else if code == http.StatusForbidden {
			t.Fatalf("RBAC off: product create refused: %v", res.Errors)
		}
		code, res = call(t, "POST", "/v1/vendor", sm, map[string]interface{}{"store_id": sid, "name": "Role Vendor " + label})
		mustOK(t, "create a vendor the role allows", code, res)
		code, res = call(t, "POST", "/v1/customer", sm, map[string]interface{}{"store_id": sid, "name": "Role Customer " + label})
		mustOK(t, "create a customer no role mentions", code, res)
	}
}

func TestFlow_RFQSupplierListServesBothShapes(t *testing.T) {
	sid := newStore(t)
	code, res := call(t, "GET", "/v1/rfq-suppliers?store_id="+sid, authToken(t), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("list suppliers: HTTP %d", code)
	}
	_, raw := rawJSON(t, "GET", "/v1/rfq-suppliers?store_id="+sid, authToken(t), nil)
	for _, key := range []string{`"items"`, `"result"`, `"total_count"`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("supplier list has no %s: %s", key, raw)
		}
	}
}

// A service category name is trimmed before the duplicate check, a blank
// name is "required" (not 409) and a deleted category frees its name.
func TestFlow_ServiceCategoryNames(t *testing.T) {
	sid := newStore(t)
	create := func(name string) (int, apiResponse) {
		return in(t, sid, "POST", "/v1/service-category", map[string]interface{}{"store_id": sid, "name": name})
	}
	code, res := create("  Repairs  ")
	c := mustOK(t, "create Repairs", code, res)
	if got := str(c, "name"); got != "Repairs" {
		t.Fatalf("saved name %q, want Repairs", got)
	}
	code, res = create("Repairs ")
	wantStatus(t, "a duplicate with a trailing space", code, res, http.StatusConflict)

	code, res = in(t, sid, "DELETE", "/v1/service-category/"+str(c, "id"), nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("delete Repairs: HTTP %d %v", code, res.Errors)
	}
	code, res = create("Repairs")
	mustOK(t, "re-create a deleted name", code, res)

	code, res = create("   ")
	wantStatus(t, "a blank name", code, res, http.StatusBadRequest)
	if res.Errors["name"] != "Name is required" {
		t.Fatalf("blank name error %q, want Name is required", res.Errors["name"])
	}
}
