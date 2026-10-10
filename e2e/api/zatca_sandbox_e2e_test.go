//go:build e2e && zatca

package apie2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// ZATCA Phase 2 against ZATCA's sandbox (the developer portal). Runs only
// with -tags "e2e zatca" and needs the ZatcaPython venv and internet access;
// tests.yml runs it in the zatca-sandbox job. The store is always a new,
// non-production one with ZATCA's test VAT number and CRN, so nothing here
// touches a real taxpayer.
const (
	zatcaTestVAT = "399999999900003"
	zatcaTestCRN = "4030360927"
	zatcaTestOTP = "12345" // the developer portal's test OTP
)

func newZatcaStore(t *testing.T) string {
	t.Helper()
	sid := newStore(t, map[string]interface{}{
		"vat_no": zatcaTestVAT, "registration_number": zatcaTestCRN, "business_category": "Supply activities",
		"zatca": map[string]interface{}{"phase": "2", "env": "NonProduction"},
	})
	code, res := call(t, "POST", "/v1/store/zatca/connect", authToken(t), map[string]interface{}{"id": sid, "otp": zatcaTestOTP})
	if code != http.StatusOK || !res.Status {
		t.Fatalf("connect to the ZATCA sandbox: HTTP %d %v", code, res.Errors)
	}
	code, res = call(t, "GET", "/v1/store/"+sid, authToken(t), nil)
	z, _ := resultMap(t, res)["zatca"].(map[string]interface{})
	if code != http.StatusOK || z["connected"] != true {
		t.Fatalf("store not connected after onboarding: HTTP %d zatca=%v", code, z)
	}
	if z["env"] != "NonProduction" {
		t.Fatalf("ZATCA env = %v; tests must never use Production", z["env"])
	}
	return sid
}

func zatcaOf(t *testing.T, sid, path string) map[string]interface{} {
	t.Helper()
	code, res := in(t, sid, "GET", path, nil)
	z, _ := mustOK(t, "view "+path, code, res)["zatca"].(map[string]interface{})
	return z
}

func wantReported(t *testing.T, what string, z map[string]interface{}) {
	t.Helper()
	if z["reporting_passed"] != true {
		t.Fatalf("%s: reporting_passed=%v errors=%v compliance_errors=%v", what, z["reporting_passed"], z["reporting_errors"], z["compliance_check_errors"])
	}
	if s, _ := z["qr_code"].(string); s == "" {
		t.Fatalf("%s: no QR code", what)
	}
	if s, _ := z["reporting_invoice_hash"].(string); s == "" {
		t.Fatalf("%s: no invoice hash", what)
	}
}

func TestZatcaSandbox_OnboardReportAndCredit(t *testing.T) {
	sid := newZatcaStore(t)
	widget := createProduct(t, sid, "ZATCA Widget", 100, 60)
	when := agoStr(time.Minute)

	// Simplified (B2C) tax invoice: reported.
	b := saleBody(sid, "", when, []line{{id: widget, name: "ZATCA Widget", qty: 2, price: 100, cost: 60}}, payment(230, when))
	b["enable_report_to_zatca"] = true
	code, res := in(t, sid, "POST", "/v1/order", b)
	simplified := mustOK(t, "simplified invoice", code, res)
	z := zatcaOf(t, sid, "/v1/order/"+str(simplified, "id"))
	wantReported(t, "simplified invoice", z)
	z0 := z
	if z["is_simplified"] != true {
		t.Fatalf("a sale without a VAT-registered customer must be simplified")
	}

	// Standard (B2B) tax invoice to a VAT-registered customer: cleared.
	customer := createCustomer(t, sid, "ZATCA Business Customer", map[string]interface{}{
		"vat_no": "399999999800003", "registration_number": "4030360928", "credit_limit": 10000,
		"national_address": map[string]interface{}{"building_no": "1111", "street_name": "Prince Sultan", "district_name": "Rawdah",
			"city_name": "Jeddah", "zipcode": "23434", "additional_no": "2222"},
	})
	b = saleBody(sid, customer, when, []line{{id: widget, name: "ZATCA Widget", qty: 1, price: 100, cost: 60}}, payment(115, when))
	b["enable_report_to_zatca"] = true
	code, res = in(t, sid, "POST", "/v1/order", b)
	standard := mustOK(t, "standard invoice", code, res)
	z = zatcaOf(t, sid, "/v1/order/"+str(standard, "id"))
	wantReported(t, "standard invoice", z)
	if z["is_simplified"] == true {
		t.Fatalf("a sale to a VAT-registered customer must be a standard invoice")
	}

	// Credit note for part of the simplified invoice.
	code, res = in(t, sid, "POST", "/v1/sales-return", map[string]interface{}{
		"store_id": sid, "order_id": str(simplified, "id"), "date_str": nowStr(), "vat_percent": 15,
		"payment_status": "paid", "enable_report_to_zatca": true,
		"payments_input": []map[string]interface{}{payment(115, nowStr())},
		"products": []map[string]interface{}{{"product_id": widget, "name": "ZATCA Widget", "quantity": 1, "unit_price": 100,
			"purchase_unit_price": 60, "unit": "PC", "selected": true}},
	})
	credit := mustOK(t, "credit note", code, res)
	wantReported(t, "credit note", zatcaOf(t, sid, "/v1/sales-return/"+str(credit, "id")))

	// A reported invoice or credit note can't be reported twice: 409, and
	// the document keeps the hash ZATCA accepted.
	for _, doc := range []struct{ path, id, hash string }{
		{"/v1/order", str(simplified, "id"), str(z0, "reporting_invoice_hash")},
		{"/v1/sales-return", str(credit, "id"), str(zatcaOf(t, sid, "/v1/sales-return/"+str(credit, "id")), "reporting_invoice_hash")},
	} {
		code, res = in(t, sid, "POST", doc.path+"/zatca/report/"+doc.id, nil)
		if code != http.StatusConflict || res.Status || res.Errors["already_reported"] == "" {
			t.Fatalf("re-reporting %s %s: HTTP %d status=%v errors=%v, want 409 already_reported", doc.path, doc.id, code, res.Status, res.Errors)
		}
		if h := str(zatcaOf(t, sid, doc.path+"/"+doc.id), "reporting_invoice_hash"); h != doc.hash {
			t.Fatalf("re-reporting %s %s changed its hash from %q to %q", doc.path, doc.id, doc.hash, h)
		}
	}

	// Disconnecting stops reporting.
	code, res = call(t, "POST", "/v1/store/zatca/disconnect", authToken(t), map[string]interface{}{"id": sid})
	if code != http.StatusOK || !res.Status {
		t.Fatalf("disconnect: HTTP %d %v", code, res.Errors)
	}
	b = saleBody(sid, "", nowStr(), []line{{id: widget, name: "ZATCA Widget", qty: 1, price: 100, cost: 60}}, payment(115, nowStr()))
	b["enable_report_to_zatca"] = true
	code, res = in(t, sid, "POST", "/v1/order", b)
	after := mustOK(t, "sale after disconnect", code, res)
	if z := zatcaOf(t, sid, "/v1/order/"+str(after, "id")); z["reporting_passed"] == true {
		t.Fatalf("a sale after disconnecting was reported: %v", fmt.Sprint(z))
	}
}

// Onboarding with an OTP the sandbox rejects must fail cleanly and leave
// the store disconnected.
func TestZatcaSandbox_BadOTPLeavesStoreDisconnected(t *testing.T) {
	sid := newStore(t, map[string]interface{}{
		"vat_no": zatcaTestVAT, "registration_number": zatcaTestCRN,
		"zatca": map[string]interface{}{"phase": "2", "env": "NonProduction"},
	})
	code, res := call(t, "POST", "/v1/store/zatca/connect", authToken(t), map[string]interface{}{"id": sid, "otp": "000000"})
	if res.Status {
		t.Skipf("the sandbox accepted OTP 000000 (HTTP %d); it accepts any OTP today", code)
	}
	code, res = call(t, "GET", "/v1/store/"+sid, authToken(t), nil)
	z, _ := resultMap(t, res)["zatca"].(map[string]interface{})
	if z["connected"] == true {
		t.Fatalf("store connected after a failed onboarding")
	}
}
