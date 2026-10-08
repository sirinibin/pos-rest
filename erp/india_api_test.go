package erp

import (
	"math"
	"testing"
	"time"
)

// Integration (needs MongoDB + Redis): an India sign-up gets rupees, GST,
// its state and no Arabic name; GSTINs must match the party's state, HSN
// codes are kept on products, GST rates are the GST 2.0 slabs and a sale
// rounds to the nearest rupee.
func TestAPI_India_SignupPartiesProductsSales(t *testing.T) {
	requireDB(t)
	body := indiaSignup()
	body["owner"].(M)["email"] = "india+" + time.Now().Format("150405.000000") + "@signup.example"
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("India signup: %d %s", r.Code, r.Raw)
	}
	st := sub(r.Body, "store")
	sid := str(st["id"])
	defer cleanupStore(t, sid)
	if st["countryCode"] != "IN" || get(st, "currency.code") != "INR" || st["vatPercent"] != 18.0 ||
		get(st, "country.zatca") != false || get(st, "country.taxSplit") != "gst" || get(st, "country.roundingStep") != 1.0 ||
		get(st, "address.stateCode") != "27" || get(st, "address.stateEn") != "Maharashtra" || st["vatNo"] != testGSTIN {
		t.Fatalf("India store: %v", st)
	}
	tok := str(r.Body["accessToken"])

	// customers: a Kerala GSTIN with a Kerala address, and a mismatch
	c := call(t, "POST", "/customers", tok, M{"storeId": sid, "nameEn": "Kochi Traders", "vatNo": "32AAPFU0939F1Z4",
		"phone": "9876543210", "address": M{"cityEn": "Kochi", "stateCode": "32", "postalCode": "682016"}})
	if c.Code != 201 {
		t.Fatalf("customer with Kerala GSTIN: %d %s", c.Code, c.Raw)
	}
	if got := get(c.Body, "address.stateCode"); got != "32" {
		t.Errorf("customer state read back %v", got)
	}
	bad := call(t, "POST", "/customers", tok, M{"storeId": sid, "nameEn": "Wrong State", "vatNo": "32AAPFU0939F1Z4",
		"address": M{"stateCode": "29"}})
	if bad.Code != 400 || bad.errField("vatNo") == "" {
		t.Errorf("GSTIN/state mismatch: %d %s", bad.Code, bad.Raw)
	}

	// vendors take a GSTIN too (the legacy check used to demand 15 digits 3…3)
	if v := call(t, "POST", "/vendors", tok, M{"storeId": sid, "nameEn": "Bengaluru Supplies", "vatNo": "29AAPFU0939F1ZR",
		"phone": "08041234567", "address": M{"stateCode": "29"}}); v.Code != 201 {
		t.Errorf("vendor with GSTIN: %d %s", v.Code, v.Raw)
	}

	// products: HSN kept, bad HSN refused
	p := call(t, "POST", "/products", tok, M{"storeId": sid, "nameEn": "Basmati rice 5 kg", "hsn": "1006",
		"pricing": M{"retail": 780.0}})
	if p.Code != 201 {
		t.Fatalf("product: %d %s", p.Code, p.Raw)
	}
	pid := str(p.Body["id"])
	if g := call(t, "GET", "/products/"+pid+"?storeId="+sid+"&select=hsn,nameEn", tok, nil); g.Code != 200 || g.Body["hsn"] != "1006" {
		t.Errorf("HSN read back: %d %s", g.Code, g.Raw)
	}
	if b := call(t, "POST", "/products", tok, M{"storeId": sid, "nameEn": "Bad HSN", "hsn": "10061"}); b.Code != 400 || b.errField("hsn") == "" {
		t.Errorf("bad HSN: %d %s", b.Code, b.Raw)
	}

	// a sale at 18% rounds to the rupee; 12% (pre-GST 2.0) is refused
	items := []M{{"productId": pid, "qty": 1, "unitPrice": 400.72, "warehouseId": "ms_" + sid}}
	s := call(t, "POST", "/sales", tok, M{"storeId": sid, "date": "2026-10-07T10:00", "items": items,
		"roundingAuto": true, "vatPercent": 18, "customerId": str(c.Body["id"]), "phone": "9876543210"})
	if s.Code != 201 {
		t.Fatalf("India sale: %d %s", s.Code, s.Raw)
	}
	// 400.72 + 18% = 472.85 → ₹473
	if got := num(s.Body["rounding"]); math.Abs(got-0.15) > 1e-9 {
		t.Errorf("rounding %.2f, want 0.15 (nearest rupee)", got)
	}
	if b := call(t, "POST", "/sales", tok, M{"storeId": sid, "date": "2026-10-07T10:00", "items": items, "vatPercent": 12}); b.Code != 400 || b.errField("vatPercent") == "" {
		t.Errorf("GST 12%%: %d %s", b.Code, b.Raw)
	}
}
