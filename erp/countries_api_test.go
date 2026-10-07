package erp

import (
	"testing"
	"time"
)

// Integration (needs MongoDB + Redis): a UAE and a Kuwait sign-up get their
// country's currency, VAT and invoice wording, and ZATCA calls are refused.
func TestAPI_Signup_GCCCountry(t *testing.T) {
	requireDB(t)
	for _, tc := range []struct {
		code, cur, title string
		vat              float64
	}{
		{"AE", "AED", "Tax Invoice", 5},
		{"KW", "KWD", "Invoice", 0},
	} {
		body := gccSignup(tc.code)
		body["owner"].(M)["email"] = "gcc+" + tc.code + time.Now().Format("150405.000000") + "@signup.example"
		r := call(t, "POST", "/auth/signup", "", body)
		if r.Code != 201 {
			t.Fatalf("%s signup: %d %s", tc.code, r.Code, r.Raw)
		}
		st := sub(r.Body, "store")
		sid := str(st["id"])
		if st["countryCode"] != tc.code || get(st, "currency.code") != tc.cur || st["vatPercent"] != tc.vat ||
			get(st, "country.zatca") != false || get(st, "titles.invoiceEn") != tc.title || get(st, "zatca.phase") != 1.0 {
			cleanupStore(t, sid)
			t.Fatalf("%s store: %v", tc.code, st)
		}
		tok := str(r.Body["accessToken"])
		z := call(t, "POST", "/stores/"+sid+"/zatca/connect", tok, M{"otp": "123456"})
		if z.Code != 409 || z.errCode() != "zatca_not_applicable" {
			cleanupStore(t, sid)
			t.Fatalf("%s ZATCA connect: %d %s", tc.code, z.Code, z.Raw)
		}
		cleanupStore(t, sid)
	}
}
