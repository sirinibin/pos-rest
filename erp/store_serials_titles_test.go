package erp

import (
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/controller"
	"go.mongodb.org/mongo-driver/bson"
)

// Store settings: the Serials tab rows for credit notes (customer withdrawals),
// debit notes (customer deposits), proforma invoices, non-VAT sales and non-VAT
// sales returns, and the Titles tab rows that depend on the store form
// settings non_vat_sales and enable_sales_in_quotation.

// requestedSerials are the five serial keys the Serials tab gained, with the
// prefix a brand-new store gets for each.
var requestedSerials = map[string]string{
	"withdrawal":   "CUST-PAYBLE-", // credit note
	"deposit":      "CUST-RCVBLE-", // debit note
	"proforma":     "PI-",
	"nonvat":       "NVS-",
	"nonvatReturn": "NVS-R-",
}

// newStoreDoc is a new store as sign-up / admin "Add store" saves it: the
// legacy store built by controller.NewRegistrationStore plus erp.x.serials.
func newStoreDoc(t *testing.T) M {
	t.Helper()
	st := controller.NewRegistrationStore(controller.GuestRegisterRequest{
		Name: "Owner", Email: "owner@example.com", StoreName: "Al Noor Trading", StoreNameInArabic: "النور",
		BusinessCategory: "General Trading", CountryCode: "SA", CountryName: "Saudi Arabia", ZatcaPhase: "2",
	}, time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC))
	raw, err := bson.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var d bson.M
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	doc := normDoc(d)
	doc["_id"] = hexID()
	doc["erp"] = M{"x": M{"short": "ANT", "serials": newStoreSerials()}}
	return doc
}

func TestNewStore_FillsTheFiveRequestedSerials(t *testing.T) {
	doc := newStoreDoc(t)
	rec := storeToContract(testX(str(doc["_id"]), doc), doc)
	for k, want := range requestedSerials {
		if got := get(rec, "serials."+k+".prefix"); got != want {
			t.Errorf("serials.%s.prefix=%#v want %q", k, got, want)
		}
		if got := get(rec, "serials."+k+".start"); got != 1 {
			t.Errorf("serials.%s.start=%#v want 1", k, got)
		}
	}
	// the legacy counters behind four of them are stored on the store itself
	for _, lk := range []string{"customer_withdrawal_serial_number", "customer_deposit_serial_number",
		"non_vat_sales_serial_number", "non_vat_sales_return_serial_number"} {
		sn := sub(doc, lk)
		if str(sn["prefix"]) == "" || intv(sn["padding_count"]) < 1 || intv(sn["start_from_count"]) != 1 {
			t.Errorf("%s not filled on a new store: %v", lk, sn)
		}
	}
	// proforma has no legacy counter: kept in erp.x.serials
	if get(doc, "erp.x.serials.proforma.prefix") != "PI-" {
		t.Errorf("proforma serial not stored on the new store: %v", get(doc, "erp.x.serials"))
	}
}

func TestNewStoreSerials_EveryKeyWithoutALegacyCounter(t *testing.T) {
	s := newStoreSerials()
	if len(s) != len(defaultSerialPrefix) {
		t.Fatalf("got %d keys, want %d", len(s), len(defaultSerialPrefix))
	}
	for k, p := range defaultSerialPrefix {
		v, _ := s[k].(M)
		if v == nil || v["prefix"] != p || v["start"] != 1 {
			t.Errorf("%s=%v want prefix %q start 1", k, s[k], p)
		}
		if _, legacy := serialMap[k]; legacy {
			t.Errorf("%s has a legacy counter and must not be duplicated in erp.x", k)
		}
		if !validSerialPrefix(p) {
			t.Errorf("default prefix %q fails the prefix rule", p)
		}
	}
	// a fresh map every call (callers may change it)
	s["proforma"].(M)["prefix"] = "X-"
	if newStoreSerials()["proforma"].(M)["prefix"] != "PI-" {
		t.Fatal("newStoreSerials must not share state")
	}
}

func TestStoreSerials_RequestedKeysRoundTripToLegacy(t *testing.T) {
	sid := hexID()
	prev := newStoreDoc(t)
	rec := M{"serials": M{
		"withdrawal": M{"prefix": "CN-", "start": 10}, "deposit": M{"prefix": "DBN-", "start": 20},
		"nonvat": M{"prefix": "NV-2026-", "start": 1}, "nonvatReturn": M{"prefix": "NVR-", "start": 7},
		"proforma": M{"prefix": "PRO-", "start": 3},
	}}
	for k, v := range storeValidate(nil, rec, prev) {
		if strings.HasPrefix(k, "serials.") {
			t.Fatalf("valid serial rejected: %s %s", k, v)
		}
	}
	p, err := storeToLegacy(testX(sid, prev), rec, prev, knownSet("serials"), false)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]interface{}{
		"customer_withdrawal_serial_number":  {"CN", int64(10)},
		"customer_deposit_serial_number":     {"DBN", int64(20)},
		"non_vat_sales_serial_number":        {"NV-2026", int64(1)},
		"non_vat_sales_return_serial_number": {"NVR", int64(7)},
	}
	for lk, w := range want {
		if get(p, lk+".prefix") != w[0] || get(p, lk+".start_from_count") != w[1] {
			t.Errorf("%s=%v want prefix %v start %v", lk, p[lk], w[0], w[1])
		}
		if intv(get(p, lk+".padding_count")) < 1 {
			t.Errorf("%s keeps a padding count: %v", lk, p[lk])
		}
	}
	// proforma lives in erp.x (hybrid "serials"), not on a legacy field
	sb := newStoresResource().Backend.(*storesBackend)
	if get(sb.extras(rec), "serials.proforma.prefix") != "PRO-" {
		t.Fatalf("proforma serial not kept in erp.x: %v", sb.extras(rec))
	}
	// and is read back from there
	doc := newStoreDoc(t)
	doc["erp"] = M{"x": M{"serials": M{"proforma": M{"prefix": "PRO-", "start": 3}}}}
	if pr, st := serialCfg(doc, "proforma"); pr != "PRO-" || st != 3 {
		t.Fatalf("proforma serialCfg=%s,%d", pr, st)
	}
}

func TestStoreSerials_RequestedKeysValidation(t *testing.T) {
	for k := range requestedSerials {
		errs := storeValidate(nil, M{"nameEn": "x", "serials": M{k: M{"prefix": "bad prefix", "start": 0}}}, nil)
		if errs["serials."+k+".prefix"] == "" || errs["serials."+k+".start"] == "" {
			t.Errorf("%s: expected prefix and start errors, got %v", k, errs)
		}
		ok := storeValidate(nil, M{"nameEn": "x", "serials": M{k: M{"prefix": "CN-2026-", "start": 5}}}, nil)
		if ok["serials."+k+".prefix"] != "" || ok["serials."+k+".start"] != "" {
			t.Errorf("%s: valid serial rejected: %v", k, ok)
		}
	}
}

func TestStoreFlags_NonVatSalesAndSalesInQuotation(t *testing.T) {
	sid := hexID()
	for _, on := range []bool{true, false} {
		d := M{"_id": sid, "name": "S", "settings": M{"non_vat_sales": on, "enable_sales_in_quotation": !on}}
		rec := storeToContract(testX(sid, d), d)
		if get(rec, "flags.non_vat_sales") != on || get(rec, "flags.enable_sales_in_quotation") != !on {
			t.Fatalf("read flags (on=%v): %v", on, rec["flags"])
		}
	}
	// a legacy store with the old top-level setting
	d := M{"_id": sid, "name": "S", "non_vat_sales": true}
	if get(storeToContract(testX(sid, d), d), "flags.non_vat_sales") != true {
		t.Fatal("legacy top-level non_vat_sales not read")
	}
	// written to the legacy settings the dashboard revenue reads
	prev := M{"_id": sid, "settings": M{"enable_warehouse_module": true}}
	p, err := storeToLegacy(testX(sid, prev), M{"flags": M{"non_vat_sales": true, "enable_sales_in_quotation": true}}, prev, knownSet("flags"), false)
	if err != nil {
		t.Fatal(err)
	}
	if get(p, "settings.non_vat_sales") != true || get(p, "settings.enable_sales_in_quotation") != true {
		t.Fatalf("flags not written: %v", p["settings"])
	}
	p, _ = storeToLegacy(testX(sid, prev), M{"flags": M{"non_vat_sales": false}}, prev, knownSet("flags"), false)
	if get(p, "settings.non_vat_sales") != false {
		t.Fatalf("turning non_vat_sales off not written: %v", p["settings"])
	}
	if _, set := sub(p, "settings")["enable_sales_in_quotation"]; set {
		t.Fatal("a flag the client did not send must not be written")
	}
}

func TestStoreTitles_NewKeysKeptAndRendered(t *testing.T) {
	titles := M{"purchaseEn": "Purchase Invoice", "purchaseAr": "فاتورة مشتريات", "nonvatEn": "Invoice", "nonvatAr": "فاتورة",
		"nonvatReturnEn": "Return Note", "nonvatReturnAr": "سند مرتجع", "quotationSalesEn": "Sales Invoice",
		"quotationSalesAr": "فاتورة مبيعات", "quotationReturnEn": "Credit Note", "quotationReturnAr": "إشعار دائن"}
	sb := newStoresResource().Backend.(*storesBackend)
	x := sb.extras(M{"titles": titles})
	for k, v := range titles {
		if get(x, "titles."+k) != v {
			t.Errorf("titles.%s not kept in erp.x: %v", k, x["titles"])
		}
	}
	doc := newStoreDoc(t)
	doc["erp"] = M{"x": M{"titles": titles}}
	rec := sb.render(newMapCtx(nil, ""), doc, false)
	for k, v := range titles {
		if get(rec, "titles."+k) != v {
			t.Errorf("rendered titles.%s=%v want %v", k, get(rec, "titles."+k), v)
		}
	}
	// legacy-mapped titles still win over erp.x copies
	if get(rec, "titles.debitNoteEn") != "Debit Note" || get(rec, "titles.creditNoteEn") != "Credit Note" {
		t.Errorf("debit/credit note defaults: %v", rec["titles"])
	}
}

func TestStoreValidate_Titles(t *testing.T) {
	long := strings.Repeat("a", maxTitleLen+1)
	arabic100 := strings.Repeat("ف", maxTitleLen) // 100 runes, 200 bytes
	cases := []struct {
		name  string
		title M
		prev  M
		field string // "" = no error
	}{
		{"short english", M{"purchaseEn": "Purchase Invoice"}, nil, ""},
		{"empty title", M{"nonvatEn": ""}, nil, ""},
		{"null title", M{"nonvatAr": nil}, nil, ""},
		{"100 arabic letters", M{"quotationSalesAr": arabic100}, nil, ""},
		{"too long", M{"quotationReturnEn": long}, nil, "titles.quotationReturnEn"},
		{"too long arabic", M{"nonvatReturnAr": arabic100 + "ف"}, nil, "titles.nonvatReturnAr"},
		{"not text", M{"purchaseAr": 12}, nil, "titles.purchaseAr"},
		{"unchanged long stored title", M{"invoiceEn": long}, M{"erp": M{"x": M{"titles": M{"invoiceEn": long}}}}, ""},
		{"changed long title", M{"invoiceEn": long + "b"}, M{"erp": M{"x": M{"titles": M{"invoiceEn": long}}}}, "titles.invoiceEn"},
		{"unknown key ignored", M{"somethingEn": long}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := storeValidate(nil, M{"nameEn": "x", "titles": c.title}, c.prev)
			var got []string
			for k := range errs {
				if strings.HasPrefix(k, "titles.") {
					got = append(got, k)
				}
			}
			if c.field == "" && len(got) > 0 {
				t.Fatalf("unexpected title errors %v", errs)
			}
			if c.field != "" && (len(got) != 1 || got[0] != c.field) {
				t.Fatalf("want error on %s, got %v", c.field, errs)
			}
		})
	}
}

// API (DB-backed): a store added by a platform admin has all five requested
// serials; PATCH saves them, the two store form settings and the new titles,
// and rejects an over-long title with its contract key.
func TestAPI_Stores_SerialsTitlesAndFormSettings(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	tag := time.Now().Format("150405000000")
	r := call(t, "POST", "/stores", admin, validNewStore(tag+"s"))
	if r.Code != 201 && r.Code != 200 {
		t.Fatalf("create store: %d %s", r.Code, r.Raw)
	}
	sid := str(r.Body["id"])
	defer cleanupStore(t, sid)
	for k, want := range requestedSerials {
		if get(r.Body, "serials."+k+".prefix") != want || num(get(r.Body, "serials."+k+".start")) != 1 {
			t.Errorf("new store serials.%s=%v want %s", k, get(r.Body, "serials."+k), want)
		}
	}
	g := call(t, "GET", "/stores/"+sid, admin, nil)
	p := call(t, "PATCH", "/stores/"+sid, admin, M{
		"serials": M{"withdrawal": M{"prefix": "CN-", "start": 5}, "deposit": M{"prefix": "DBN-", "start": 6},
			"proforma": M{"prefix": "PRO-", "start": 7}, "nonvat": M{"prefix": "NV-", "start": 8}, "nonvatReturn": M{"prefix": "NVR-", "start": 9}},
		"flags":  M{"non_vat_sales": true, "enable_sales_in_quotation": true},
		"titles": M{"purchaseEn": "Purchase Invoice", "nonvatAr": "فاتورة", "quotationSalesEn": "Sales Invoice"},
	}, "If-Match", str(g.Body["version"]), "X-Change-Reason", "settings")
	if p.Code != 200 {
		t.Fatalf("patch: %d %s", p.Code, p.Raw)
	}
	for k, want := range map[string]string{"withdrawal": "CN-", "deposit": "DBN-", "proforma": "PRO-", "nonvat": "NV-", "nonvatReturn": "NVR-"} {
		if get(p.Body, "serials."+k+".prefix") != want {
			t.Errorf("patched serials.%s=%v", k, get(p.Body, "serials."+k))
		}
	}
	if get(p.Body, "flags.non_vat_sales") != true || get(p.Body, "flags.enable_sales_in_quotation") != true {
		t.Errorf("flags: %v", p.Body["flags"])
	}
	if get(p.Body, "titles.purchaseEn") != "Purchase Invoice" || get(p.Body, "titles.nonvatAr") != "فاتورة" || get(p.Body, "titles.quotationSalesEn") != "Sales Invoice" {
		t.Errorf("titles: %v", p.Body["titles"])
	}
	b := call(t, "PATCH", "/stores/"+sid, admin, M{"titles": M{"purchaseEn": strings.Repeat("x", maxTitleLen+1)}})
	if b.Code != 400 || b.errField("titles.purchaseEn") == "" {
		t.Fatalf("long title: %d %s", b.Code, b.Raw)
	}
}

// non_vat_sales (Store settings switch) and enable_nonvat_sales (POS Documents)
// are aliases of the legacy settings.non_vat_sales: the client sends both, and
// the one it changed wins whatever the map order.
func TestStoreFlags_NonVatAliasesChangedOneWins(t *testing.T) {
	sid := hexID()
	cases := []struct {
		prev bool
		f    M
		want bool
	}{
		{false, M{"non_vat_sales": true, "enable_nonvat_sales": false}, true},
		{false, M{"non_vat_sales": false, "enable_nonvat_sales": true}, true},
		{true, M{"non_vat_sales": false, "enable_nonvat_sales": true}, false},
		{true, M{"non_vat_sales": true, "enable_nonvat_sales": false}, false},
		{true, M{"non_vat_sales": true, "enable_nonvat_sales": true}, true},
		{false, M{"enable_nonvat_sales": true}, true},
	}
	for i, c := range cases {
		prev := M{"_id": sid, "settings": M{"non_vat_sales": c.prev}}
		for n := 0; n < 40; n++ { // map iteration order varies
			p, err := storeToLegacy(testX(sid, prev), M{"flags": c.f}, prev, knownSet("flags"), false)
			if err != nil {
				t.Fatal(err)
			}
			if got := get(p, "settings.non_vat_sales"); got != c.want {
				t.Fatalf("case %d: settings.non_vat_sales=%v want %v", i, got, c.want)
			}
		}
	}
	d := M{"_id": sid, "settings": M{"non_vat_sales": true}}
	rec := storeToContract(testX(sid, d), d)
	if get(rec, "flags.non_vat_sales") != true || get(rec, "flags.enable_nonvat_sales") != true {
		t.Fatalf("both aliases read the legacy setting: %v", rec["flags"])
	}
}
