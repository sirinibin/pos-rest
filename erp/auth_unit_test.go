package erp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func validSignup() M {
	return M{
		"owner": M{"name": "Sirin", "email": "owner@example.com", "mobile": "0512345678", "password": "Str0ng!Pass"},
		"company": M{
			"nameEn": "Al Noor Trading Co.", "nameAr": "شركة النور", "vatNo": "310122393500003", "crNo": "1010101010",
			"mobile": "0112345678", "type": "retail", "plan": "professional",
			"address": M{"buildingNo": "1234", "streetEn": "Olaya", "streetAr": "العليا", "districtEn": "Olaya", "districtAr": "العليا",
				"cityEn": "Riyadh", "cityAr": "الرياض", "postalCode": "12345", "additionalNo": "6789", "shortAddress": "RRRD1234"},
		},
	}
}

func TestValidateSignup_OwnerRules(t *testing.T) {
	if errs := ValidateSignup(validSignup()); len(errs) != 0 {
		t.Fatalf("valid signup rejected: %v", errs)
	}
	mut := func(path string, v interface{}) M {
		b := cloneM(validSignup())
		parts := splitPath(path)
		cur := b
		for _, p := range parts[:len(parts)-1] {
			cur = cur[p].(M)
		}
		cur[parts[len(parts)-1]] = v
		return b
	}
	tests := []struct {
		path  string
		value interface{}
		field string
	}{
		{"company.vatNo", "31012239350000", "company.vatNo"},
		{"company.vatNo", "210122393500003", "company.vatNo"},
		{"company.vatNo", "", "company.vatNo"},
		{"company.crNo", "123", "company.crNo"},
		{"company.crNo", "", "company.crNo"},
		{"company.nameEn", " ", "company.nameEn"},
		{"company.nameAr", "Latin only", "company.nameAr"},
		{"company.mobile", "12", "company.mobile"},
		{"company.address.buildingNo", "12", "company.address.buildingNo"},
		{"company.address.postalCode", "1234", "company.address.postalCode"},
		{"company.address.additionalNo", "", "company.address.additionalNo"},
		{"company.address.streetAr", "Olaya", "company.address.streetAr"},
		{"company.address.districtAr", "", "company.address.districtAr"},
		{"company.address.cityEn", "", "company.address.cityEn"},
		{"company.address.shortAddress", "bad", "company.address.shortAddress"},
		{"company.type", "bakery", "company.type"},
		{"company.plan", "gold", "company.plan"},
		{"owner.email", "not-an-email", "owner.email"},
		{"owner.mobile", "0112345678", "owner.mobile"},
		{"owner.password", "short", "owner.password"},
		{"owner.password", "alllowercaseletters", "owner.password"},
		{"owner.name", "", "owner.name"},
	}
	for _, tc := range tests {
		t.Run(tc.path+"="+str(tc.value), func(t *testing.T) {
			errs := ValidateSignup(mut(tc.path, tc.value))
			if _, ok := errs[tc.field]; !ok {
				t.Fatalf("expected error on %s, got %v", tc.field, errs)
			}
		})
	}
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, r := range p {
		if r == '.' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}

func TestDeriveShort(t *testing.T) {
	cases := map[string]string{"Al Noor Trading Co.": "ANTC", "x": "MAIN", "Gulf Union Ozone Trading Company LLC": "GUOTC", "ab": "AB"}
	for in, want := range cases {
		if got := deriveShort(in); got != want {
			t.Errorf("deriveShort(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMeta_Capabilities(t *testing.T) {
	rec := httptest.NewRecorder()
	handleMeta(rec, httptest.NewRequest("GET", "/v1/erp/meta", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	var body struct {
		Capabilities map[string]interface{} `json:"capabilities"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	want := map[string]interface{}{"clientIds": true, "serverNumbers": true, "serverStock": true, "serverTotals": true, "zatca": "server", "realtime": false}
	for k, v := range want {
		if body.Capabilities[k] != v {
			t.Errorf("capability %s=%v want %v", k, body.Capabilities[k], v)
		}
	}
}

func TestPasswordStrength(t *testing.T) {
	if passwordStrength("abc") != 0 || passwordStrength("abcdefgh") != 1 || passwordStrength("Abcdefg1!") != 4 {
		t.Fatal("strength")
	}
}

func TestErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErr(rec, errBadRequest("", map[string]string{"nameEn": "required"}))
	if rec.Code != 400 {
		t.Fatal(rec.Code)
	}
	var b struct {
		Error APIError `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &b)
	if b.Error.Code != "validation" || b.Error.Fields["nameEn"] != "required" || b.Error.Message == "" {
		t.Fatalf("%+v", b)
	}
	rec = httptest.NewRecorder()
	writeErr(rec, errUnsupported("nope"))
	if rec.Code != http.StatusConflict {
		t.Fatal("unsupported_legacy is 409 (non-retryable, client re-GETs)")
	}
}

func TestLegacyErrTranslation(t *testing.T) {
	r := &v1Result{HTTP: 400, Errors: map[string]string{
		"date_str": "Date is required", "quantity_1": "Quantity is required", "payment_amount_0": "Payment amount is required",
		"customer_receivable_payment_method_2": "Payment method is required", "unknown_key": "x",
	}}
	e := legacyErr(r, docFieldErr, docLineErr, "items")
	if e.Status != 400 || e.Fields["date"] == "" || e.Fields["items.1.qty"] == "" || e.Fields["payments.0.amount"] == "" ||
		e.Fields["payments.2.method"] == "" || e.Fields["unknown_key"] == "" {
		t.Fatalf("%+v", e)
	}
	if legacyErr(&v1Result{HTTP: 401, Errors: map[string]string{"access_token": "x"}}, nil, nil, "").Status != 401 {
		t.Fatal("401")
	}
	if legacyErr(&v1Result{HTTP: 403, Errors: map[string]string{"role": "x"}}, nil, nil, "").Status != 403 {
		t.Fatal("403")
	}
	if legacyErr(&v1Result{HTTP: 500, Errors: map[string]string{"insert": "db down"}}, nil, nil, "").Status != 500 {
		t.Fatal("infra errors stay 5xx (retryable)")
	}
	if legacyErr(&v1Result{HTTP: 500, Errors: map[string]string{"name": "Name is required"}}, nil, nil, "").Status != 400 {
		t.Fatal("legacy validation sent with 500 must become 400")
	}
}

func TestV1ResultOK(t *testing.T) {
	cases := []struct {
		r    v1Result
		want bool
	}{
		{v1Result{HTTP: 200, Status: true}, true},
		{v1Result{HTTP: 200, Result: json.RawMessage(`{"id":"x"}`)}, true}, // CreateProduct never sets status
		{v1Result{HTTP: 200, Result: json.RawMessage(`null`)}, false},
		{v1Result{HTTP: 200, Errors: map[string]string{"a": "b"}}, false},
		{v1Result{HTTP: 400, Status: true}, false},
	}
	for i, c := range cases {
		if c.r.ok() != c.want {
			t.Errorf("case %d: ok=%v want %v", i, c.r.ok(), c.want)
		}
	}
}
