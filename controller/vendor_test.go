package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---------------------------------------------------------------------------
// Unauthenticated handler tests
// ---------------------------------------------------------------------------

func TestVendorHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListVendor",
			method:  http.MethodGet,
			path:    "/v1/vendor",
			handler: ListVendor,
		},
		{
			name:    "CreateVendor",
			method:  http.MethodPost,
			path:    "/v1/vendor",
			handler: CreateVendor,
		},
		{
			name:    "UpdateVendor",
			method:  http.MethodPut,
			path:    "/v1/vendor/" + fakeID,
			handler: UpdateVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewVendor",
			method:  http.MethodGet,
			path:    "/v1/vendor/" + fakeID,
			handler: ViewVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewVendorByVatNoByName",
			method:  http.MethodGet,
			path:    "/v1/vendor/by-vat-name",
			handler: ViewVendorByVatNoByName,
		},
		{
			name:    "DeleteVendor",
			method:  http.MethodDelete,
			path:    "/v1/vendor/" + fakeID,
			handler: DeleteVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreVendor",
			method:  http.MethodPost,
			path:    "/v1/vendor/" + fakeID + "/restore",
			handler: RestoreVendor,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "VendorSummary",
			method:  http.MethodGet,
			path:    "/v1/vendor/summary",
			handler: VendorSummary,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected application/json Content-Type, got %q", ct)
			}

			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[access_token], got %v", resp.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DB-backed integration tests (ERP_TEST_DB=1)
// ---------------------------------------------------------------------------

// gcVendorOBLedgers returns the opening-balance ledgers of a vendor.
func gcVendorOBLedgers(t *testing.T, storeID, vendorID primitive.ObjectID) []bson.M {
	t.Helper()
	return gcFindAll(t, storeID, "ledger", bson.M{"reference_id": vendorID, "reference_model": "vendor_opening_balance"})
}

// gcJournalSides returns the debit/credit amounts booked on the vendor's
// account and on OPENING BALANCE EQUITY by a single opening-balance ledger.
func gcJournalSides(t *testing.T, ledger bson.M, vendorName string) (vendorDr, vendorCr, equityDr, equityCr float64) {
	t.Helper()
	js, _ := ledger["journals"].(bson.A)
	if len(js) != 2 {
		t.Fatalf("opening-balance ledger must have 2 journals: %v", ledger)
	}
	for _, j := range js {
		m := j.(bson.M)
		dr, _ := m["debit"].(float64)
		cr, _ := m["credit"].(float64)
		switch m["account_name"] {
		case "OPENING BALANCE EQUITY":
			equityDr, equityCr = equityDr+dr, equityCr+cr
		case vendorName:
			vendorDr, vendorCr = vendorDr+dr, vendorCr+cr
		default:
			t.Fatalf("unexpected journal account %v", m["account_name"])
		}
	}
	return
}

// TestCreateVendor_Integration: create validates, upper-cases the name,
// assigns a code, and posts the opening balance (CR vendor / DR equity for
// the default "payable" type) only when an opening balance is given.
// credit_balance sign follows SetCreditBalance: payable (store owes vendor,
// liability) is negative, receivable (vendor owes store, asset) is positive.
func TestCreateVendor_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	sid := fx.StoreA.Hex()
	obDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	// validation rows
	for _, c := range []struct {
		name string
		body map[string]interface{}
		key  string
	}{
		{"name required", map[string]interface{}{"store_id": sid}, "name"},
		{"negative opening balance", map[string]interface{}{"store_id": sid, "name": "x", "opening_balance": -1, "opening_balance_date": obDate}, "opening_balance"},
		{"bad opening balance type", map[string]interface{}{"store_id": sid, "name": "x", "opening_balance": 1, "opening_balance_date": obDate, "opening_balance_type": "weird"}, "opening_balance_type"},
		{"opening balance date required", map[string]interface{}{"store_id": sid, "name": "x", "opening_balance": 10}, "opening_balance_date"},
	} {
		r := callHandler(t, CreateVendor, "POST", "/v1/vendor", tok, c.body)
		if r.Status || r.Errors[c.key] == nil {
			t.Errorf("%s: expected errors.%s, got %d %s", c.name, c.key, r.Code, r.Raw)
		}
	}

	// no opening balance → nothing posted
	name := uniqName("gc vendor plain")
	r := callHandler(t, CreateVendor, "POST", "/v1/vendor", tok, map[string]interface{}{"store_id": sid, "name": name})
	if !r.Status {
		t.Fatalf("create plain: %d %s", r.Code, r.Raw)
	}
	m := r.resultMap(t)
	if m["name"] != strings.ToUpper(name) || m["code"] == "" || m["code"] == nil || m["opening_balance_posted"] != false {
		t.Fatalf("create plain result: %v", m)
	}
	plainID, _ := primitive.ObjectIDFromHex(m["id"].(string))
	if gcFindOne(t, fx.StoreA, "vendor", bson.M{"_id": plainID}) == nil {
		t.Fatal("vendor not persisted")
	}
	if l := gcVendorOBLedgers(t, fx.StoreA, plainID); len(l) != 0 {
		t.Fatalf("no opening balance → no ledger, got %d", len(l))
	}
	// view returns it
	if v := callHandler(t, ViewVendor, "GET", "/v1/vendor/"+plainID.Hex()+"?search[store_id]="+sid, tok, nil, "id", plainID.Hex()); !v.Status || v.resultMap(t)["name"] != strings.ToUpper(name) {
		t.Fatalf("view: %s", v.Raw)
	}

	// opening balance 500 payable → CR vendor 500 / DR equity 500, posted.
	// SetCreditBalance reports a creditor (liability) account negated, so a
	// payable opening balance shows as credit_balance -500.
	name = uniqName("gc vendor ob")
	r = callHandler(t, CreateVendor, "POST", "/v1/vendor", tok, map[string]interface{}{
		"store_id": sid, "name": name, "opening_balance": 500.0, "opening_balance_date": obDate,
	})
	if !r.Status {
		t.Fatalf("create with opening balance: %d %s", r.Code, r.Raw)
	}
	m = r.resultMap(t)
	if m["opening_balance_posted"] != true {
		t.Fatalf("opening balance must be posted: %v", m)
	}
	if cb, _ := m["credit_balance"].(float64); cb != -500 {
		t.Errorf("credit_balance in response = %v, want -500", m["credit_balance"])
	}
	vid, _ := primitive.ObjectIDFromHex(m["id"].(string))
	ls := gcVendorOBLedgers(t, fx.StoreA, vid)
	if len(ls) != 1 {
		t.Fatalf("want exactly 1 opening-balance ledger, got %d", len(ls))
	}
	if ls[0]["reference_code"] != "OPENING-BALANCE" {
		t.Errorf("reference_code = %v", ls[0]["reference_code"])
	}
	vDr, vCr, eDr, eCr := gcJournalSides(t, ls[0], strings.ToUpper(name))
	if vDr != 0 || vCr != 500 || eDr != 500 || eCr != 0 {
		t.Fatalf("payable posting: vendor dr=%v cr=%v equity dr=%v cr=%v", vDr, vCr, eDr, eCr)
	}
	d := gcFindOne(t, fx.StoreA, "vendor", bson.M{"_id": vid})
	if d["opening_balance_posted"] != true {
		t.Errorf("opening_balance_posted not persisted: %v", d["opening_balance_posted"])
	}
	if p := gcFindAll(t, fx.StoreA, "posting", bson.M{"reference_id": vid, "reference_model": "vendor_opening_balance"}); len(p) == 0 {
		t.Error("opening balance postings missing")
	}
}

// TestUpdateVendor_Integration_OpeningBalanceChanged: changing the opening
// balance amount / type re-posts exactly one replacement ledger (the old one
// is removed) and refreshes credit_balance; setting it to 0 removes the
// ledger and clears opening_balance_posted. Other fields are not affected.
func TestUpdateVendor_Integration_OpeningBalanceChanged(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	sid := fx.StoreA.Hex()
	obDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	name := uniqName("gc vendor upd")
	upper := strings.ToUpper(name)

	r := callHandler(t, CreateVendor, "POST", "/v1/vendor", tok, map[string]interface{}{
		"store_id": sid, "name": name, "opening_balance": 1000.0, "opening_balance_date": obDate, "phone": "0500000001",
	})
	if !r.Status {
		t.Fatalf("create: %s", r.Raw)
	}
	id := r.resultMap(t)["id"].(string)
	vid, _ := primitive.ObjectIDFromHex(id)
	firstLedger := gcVendorOBLedgers(t, fx.StoreA, vid)
	if len(firstLedger) != 1 {
		t.Fatalf("setup: want 1 ledger, got %d", len(firstLedger))
	}

	update := func(body map[string]interface{}) map[string]interface{} {
		t.Helper()
		body["name"] = name
		body["opening_balance_date"] = obDate
		u := callHandler(t, UpdateVendor, "PUT", "/v1/vendor/"+id+"?search[store_id]="+sid, tok, body, "id", id)
		if !u.Status {
			t.Fatalf("update %v: %d %s", body, u.Code, u.Raw)
		}
		return u.resultMap(t)
	}

	// 1000 → 1500 payable: old ledger replaced by one for 1500
	m := update(map[string]interface{}{"opening_balance": 1500.0})
	ls := gcVendorOBLedgers(t, fx.StoreA, vid)
	if len(ls) != 1 || ls[0]["_id"] == firstLedger[0]["_id"] {
		t.Fatalf("amount change must replace the ledger: %d ledgers", len(ls))
	}
	if _, vCr, eDr, _ := gcJournalSides(t, ls[0], upper); vCr != 1500 || eDr != 1500 {
		t.Fatalf("1500 payable: vendor cr=%v equity dr=%v", vCr, eDr)
	}
	if m["opening_balance_posted"] != true || m["opening_balance"] != 1500.0 {
		t.Fatalf("update result: posted=%v ob=%v", m["opening_balance_posted"], m["opening_balance"])
	}
	if cb, _ := m["credit_balance"].(float64); cb != -1500 {
		t.Errorf("credit_balance after 1500 payable = %v, want -1500", m["credit_balance"])
	}
	if m["phone"] != "0500000001" {
		t.Errorf("unrelated field changed: phone=%v", m["phone"])
	}

	// type → receivable: vendor owes store → DR vendor / CR equity
	m = update(map[string]interface{}{"opening_balance": 1500.0, "opening_balance_type": "receivable"})
	ls = gcVendorOBLedgers(t, fx.StoreA, vid)
	if len(ls) != 1 {
		t.Fatalf("type change: want 1 ledger, got %d", len(ls))
	}
	if vDr, vCr, eDr, eCr := gcJournalSides(t, ls[0], upper); vDr != 1500 || vCr != 0 || eDr != 0 || eCr != 1500 {
		t.Fatalf("receivable: vendor dr=%v cr=%v equity dr=%v cr=%v", vDr, vCr, eDr, eCr)
	}
	if cb, _ := m["credit_balance"].(float64); cb != 1500 {
		t.Errorf("credit_balance after 1500 receivable = %v, want 1500", m["credit_balance"])
	}

	// → 0: ledger and postings removed, opening_balance_posted false
	m = update(map[string]interface{}{"opening_balance": 0.0, "opening_balance_type": "payable"})
	if ls := gcVendorOBLedgers(t, fx.StoreA, vid); len(ls) != 0 {
		t.Fatalf("zero opening balance must remove the ledger, got %d", len(ls))
	}
	if p := gcFindAll(t, fx.StoreA, "posting", bson.M{"reference_id": vid, "reference_model": "vendor_opening_balance"}); len(p) != 0 {
		t.Fatalf("zero opening balance must remove postings, got %d", len(p))
	}
	if m["opening_balance_posted"] != false {
		t.Errorf("opening_balance_posted after clearing = %v", m["opening_balance_posted"])
	}
	if cb, _ := m["credit_balance"].(float64); cb != 0 {
		t.Errorf("credit_balance after clearing = %v, want 0", m["credit_balance"])
	}
	if d := gcFindOne(t, fx.StoreA, "vendor", bson.M{"_id": vid}); d["opening_balance_posted"] != false {
		t.Errorf("opening_balance_posted not cleared in DB: %v", d["opening_balance_posted"])
	}
}
