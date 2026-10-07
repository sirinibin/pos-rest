package erp

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// testX builds a mapCtx without touching the database.
func testX(storeHex string, store M, warehouses ...M) *mapCtx {
	x := &mapCtx{storeHex: storeHex, store: store, whLoaded: true, wh: warehouses, cache: map[string]M{}}
	x.c = &Ctx{UserID: primitive.NewObjectID(), UserName: "Tester", storeIdx: map[string]M{storeHex: store}, Stores: []M{store},
		RoleID: "r_admin", Perms: permsFromM(systemRoleByID("r_admin")["perms"])}
	return x
}

func hexID() string { return primitive.NewObjectID().Hex() }

// ---------------- stores ----------------

func TestStoreToContract_ModernAndLegacyShapes(t *testing.T) {
	sid := hexID()
	modern := M{"_id": sid, "name": "Al Noor", "name_in_arabic": "النور", "code": "ANT", "branch_name": "Olaya",
		"vat_no": "310122393500003", "registration_number": "1010101010", "vat_percent": 15.0, "phone": "0112345678",
		"national_address":    M{"building_no": "1234", "street_name": "Olaya", "city_name": "Riyadh", "zipcode": "12345", "additional_no": "6789"},
		"settings":            M{"enable_warehouse_module": true, "evolution_api_key": "secret", "invoice": M{"quotation_title": "Q", "receivabale_title": ""}},
		"zatca":               M{"phase": "2", "connected": true, "production_request_id": int64(12345), "private_key": "PRIVATE", "secret": "S"},
		"sales_serial_number": M{"prefix": "S-INV", "start_from_count": int64(1), "padding_count": int64(3)},
		"bank_account":        M{"bank_name": "SNB", "iban": "SA03"}}
	rec := storeToContract(testX(sid, modern), modern)
	checks := map[string]interface{}{
		"nameEn": "Al Noor", "nameAr": "النور", "short": "ANT", "vatNo": "310122393500003", "crNo": "1010101010",
		"vatPercent": 15.0, "address.buildingNo": "1234", "address.countryEn": "Saudi Arabia", "zatca.phase": 2,
		"zatca.connected": true, "zatca.pcsid": "12345", "flags.enable_warehouse_module": true, "serials.sales.prefix": "S-INV-",
		"serials.sales.start": 1, "titles.debitNoteEn": "Debit Note", "titles.creditNoteEn": "Credit Note", "titles.quotationEn": "Q",
		"bank.iban": "SA03", "whatsapp.evolution.apiKey": masked, "currency.code": "SAR",
	}
	for path, want := range checks {
		if got := get(rec, path); got != want {
			t.Errorf("%s=%#v want %#v", path, got, want)
		}
	}
	// secrets never leak
	for _, k := range []string{"private_key", "secret", "otp"} {
		if get(rec, "zatca."+k) != nil {
			t.Errorf("zatca.%s leaked", k)
		}
	}
	// OLD store: no settings sub-doc, top-level legacy flag, int32 VAT, no national address, code with digits
	old := M{"_id": sid, "name": "Old Branch", "code": "OLD1", "vat_percent": int32(15), "enable_warehouse_module": true}
	r2 := storeToContract(testX(sid, old), old)
	if r2["short"] != "OLD" || r2["vatPercent"] != 15.0 || get(r2, "flags.enable_warehouse_module") != true || get(r2, "zatca.phase") != 1 {
		t.Fatalf("legacy fallbacks: short=%v vat=%v flags=%v zatca=%v", r2["short"], r2["vatPercent"], r2["flags"], r2["zatca"])
	}
	if get(r2, "serials.proforma.prefix") != "PI-" || get(r2, "serials.sales.prefix") != "SALES-" {
		t.Fatalf("defaults: %v", r2["serials"])
	}
}

func TestStoreShort(t *testing.T) {
	cases := []struct {
		st   M
		want string
	}{
		{M{"erp": M{"x": M{"short": "OLY"}}, "code": "zz"}, "OLY"},
		{M{"code": "ANT"}, "ANT"},
		{M{"code": "6ac3fe23"}, "ACFE"}, // guest-register hex code: letters only
		{M{"code": "12345678", "name": "Gulf Union Ozone"}, "GUO"},
		{M{"code": "1", "name": "x"}, "MAIN"},
		{nil, "MAIN"},
	}
	for _, c := range cases {
		if got := storeShort(c.st); got != c.want {
			t.Errorf("storeShort(%v)=%s want %s", c.st, got, c.want)
		}
	}
}

func TestStoreToLegacy_OnlyChangedAndServerOwnedZatca(t *testing.T) {
	sid := hexID()
	prev := M{"_id": sid, "zatca": M{"phase": "1", "env": "NonProduction"}, "settings": M{"invoice": M{"quotation_title": "Old"}},
		"sales_serial_number": M{"prefix": "S-INV", "start_from_count": int64(1), "padding_count": int64(3)}}
	rec := M{"nameEn": "New Name", "phone": "0112345678", "zatca": M{"phase": 2, "connected": true, "pcsid": "fake"},
		"whatsapp": M{"evolution": M{"url": "u", "apiKey": masked}}, "serials": M{"sales": M{"prefix": "INV-", "start": 5}},
		"titles": M{"debitNoteEn": "DN title"}}
	p, err := storeToLegacy(testX(sid, prev), rec, prev, knownSet("nameEn", "zatca", "whatsapp", "serials", "titles"), false)
	if err != nil {
		t.Fatal(err)
	}
	if p["name"] != "New Name" {
		t.Fatal("name")
	}
	if _, ok := p["phone"]; ok {
		t.Fatal("unchanged keys must not be written")
	}
	if get(p, "zatca.phase") != "2" || get(p, "zatca.connected") != nil {
		t.Fatalf("only phase is client-writable: %v", p["zatca"])
	}
	if _, ok := sub(p, "settings")["evolution_api_key"]; ok {
		t.Fatal("masked secret must not overwrite the stored key")
	}
	if get(p, "sales_serial_number.prefix") != "INV" || get(p, "sales_serial_number.start_from_count") != int64(5) || get(p, "sales_serial_number.padding_count") != int64(3) {
		t.Fatalf("serial: %v", p["sales_serial_number"])
	}
	if get(p, "settings.invoice.receivable_title") != "DN title" || get(p, "settings.invoice.quotation_title") != "Old" {
		t.Fatalf("titles must keep other invoice settings: %v", get(p, "settings.invoice"))
	}
	errs := storeValidate(nil, M{"nameEn": "", "vatNo": "1", "crNo": "x", "short": "toolong", "serials": M{"sales": M{"prefix": "bad prefix!", "start": 0}}}, nil)
	for _, f := range []string{"nameEn", "vatNo", "crNo", "short", "serials.sales.prefix", "serials.sales.start"} {
		if _, ok := errs[f]; !ok {
			t.Errorf("expected store validation error %s: %v", f, errs)
		}
	}
}

// ---------------- products ----------------

func productFixture(sid, wh string) M {
	return M{"_id": hexID(), "name": "Oil", "item_code": "EO-1", "part_number": "P1", "unit": "pcs",
		"category_id": []interface{}{primitive.NewObjectID()}, "brand_id": nil,
		"product_stores": M{sid: M{"purchase_unit_price": 80.0, "retail_unit_price": "120", "wholesale_unit_price": int32(100),
			"stock": 100.0, "warehouse_stocks": M{"main_store": 90.0, "WH1": 10.0}, "warehouse_racks": M{"WH1": "R1"},
			"sales_count": int64(4), "stock_adjustments": []interface{}{M{"date": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "type": "adding", "quantity": 100.0}}}},
		"set": M{"products": []interface{}{M{"produc_id": primitive.NewObjectID(), "quantity": int32(2)}}}, "is_set": true}
}

func TestProductToContract(t *testing.T) {
	sid, wh := hexID(), hexID()
	x := testX(sid, M{"_id": sid}, M{"_id": wh, "code": "WH1"})
	p := productFixture(sid, wh)
	rec := productToContract(x, p)
	if rec["code"] != "EO-1" || get(rec, "pricing.retail") != 120.0 || get(rec, "pricing.wholesale") != 100.0 {
		t.Fatalf("basic: %v", rec)
	}
	if get(rec, "stock.ms_"+sid+".qty") != 90.0 || get(rec, "stock."+wh+".qty") != 10.0 || get(rec, "stock."+wh+".rack") != "R1" {
		t.Fatalf("stock: %v", rec["stock"])
	}
	comps := arr(rec["components"])
	if len(comps) != 1 || get(comps[0].(M), "qty") != 2.0 || get(comps[0].(M), "productId") == nil {
		t.Fatalf("components from misspelled produc_id: %v", comps)
	}
	// OLD product: no product_stores at all → zero stock, top-level rack on main store
	old := M{"_id": hexID(), "name": "Legacy", "rack": "R-9"}
	r2 := productToContract(x, old)
	if get(r2, "stock.ms_"+sid+".qty") != 0.0 || get(r2, "stock.ms_"+sid+".rack") != "R-9" || get(r2, "pricing.retail") != 0.0 {
		t.Fatalf("old product: %v", r2)
	}
	// no warehouse_stocks: main = stock - Σ warehouses
	p3 := M{"_id": hexID(), "product_stores": M{sid: M{"stock": 12.0, "warehouse_stocks": nil}}}
	if get(productToContract(x, p3), "stock.ms_"+sid+".qty") != 12.0 {
		t.Fatal("fallback main stock")
	}
	// services carry no stock
	if len(sub(productToContract(x, M{"_id": hexID(), "is_service": true}), "stock")) != 0 {
		t.Fatal("service stock")
	}
}

func TestProductToLegacy_CreateWithOpeningStock(t *testing.T) {
	sid, wh := hexID(), hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0, "code": "ANT"}, M{"_id": wh, "code": "WH1"})
	rec := M{"code": "SKU-1", "nameEn": "Filter", "pricing": M{"purchase": 10.0, "retail": 20.0}, "categoryIds": []interface{}{},
		"stock": M{"ms_" + sid: M{"qty": 5.0, "rack": "A"}, wh: M{"qty": 2.0}}}
	p, err := productToLegacy(x, rec, nil, allKeys(rec), true)
	if err != nil {
		t.Fatal(err)
	}
	ps := sub(sub(p, "product_stores"), sid)
	if ps["retail_unit_price"] != 20.0 || ps["retail_unit_price_with_vat"] != 23.0 || ps["store_id"] != sid {
		t.Fatalf("pricing: %v", ps)
	}
	adjs := arr(ps["stock_adjustments"])
	if len(adjs) != 2 {
		t.Fatalf("opening stock must become 2 adjustments: %v", adjs)
	}
	total := 0.0
	for _, a := range adjs {
		am := a.(M)
		if am["type"] != "adding" || am["reason"] != "opening" || str(am["date_str"]) == "" {
			t.Fatalf("adj: %v", am)
		}
		if am["warehouse_id"] == wh && am["warehouse_code"] != "WH1" {
			t.Fatal("warehouse code must accompany warehouse id (legacy dereferences it)")
		}
		total += num(am["quantity"])
	}
	if total != 7 || get(ps, "warehouse_racks.main_store") != "A" {
		t.Fatalf("total=%v racks=%v", total, ps["warehouse_racks"])
	}
}

func TestProductToLegacy_UpdateAbsoluteStockAndPreserveStats(t *testing.T) {
	sid, wh := hexID(), hexID()
	x := testX(sid, M{"_id": sid}, M{"_id": wh, "code": "WH1"})
	prev := productFixture(sid, wh)
	// stock absolute: main 90→85 (remove 5), WH1 10→10 (no-op)
	rec := productToContract(x, prev)
	rec["stock"] = M{"ms_" + sid: M{"qty": 85.0}, wh: M{"qty": 10.0}}
	p, err := productToLegacy(x, rec, prev, knownSet("stock"), false)
	if err != nil {
		t.Fatal(err)
	}
	ps := sub(sub(p, "product_stores"), sid)
	if ps["sales_count"] == nil || num(ps["purchase_unit_price"]) != 80 {
		t.Fatalf("full product_stores entry must be sent (legacy decoder replaces map values): %v", ps)
	}
	adjs := arr(ps["stock_adjustments"])
	if len(adjs) != 2 {
		t.Fatalf("existing + 1 new adjustment expected: %v", adjs)
	}
	if str(adjs[0].(M)["date_str"]) == "" {
		t.Fatal("existing adjustments need date_str for legacy validation")
	}
	last := adjs[1].(M)
	if last["type"] != "removing" || last["quantity"] != 5.0 || last["warehouse_id"] != nil {
		t.Fatalf("new adj: %v", last)
	}
	if _, ok := p["name"]; ok {
		t.Fatal("unchanged fields must not be sent")
	}
	if errs := productValidate(testX(sid, nil), M{"nameEn": "", "barcode": "12", "pricing": M{"retail": -1.0, "min": 5.0, "max": 2.0}}, nil); len(errs) != 4 {
		t.Fatalf("product validation: %v", errs)
	}
}

// ---------------- customers / vendors ----------------

func TestCustomerMapping(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid})
	toC := simpleToC(customerFields, partyAddressToC)
	old := M{"_id": hexID(), "name": "Walk In", "phone": "05 1234 5679", "credit_limit": int32(500), "address": "Old street"}
	rec := toC(x, old)
	if rec["creditLimit"] != 500.0 || rec["phone"] != "05 1234 5679" || rec["addressText"] != "Old street" || rec["nameAr"] != "" {
		t.Fatalf("old customer: %v", rec)
	}
	if get(rec, "address.countryEn") != "Saudi Arabia" {
		t.Fatal("default country")
	}
	in := M{"nameEn": "Garage", "vatNo": "300000000000003", "phone": "0551112222", "openingBalanceType": "debit",
		"address": M{"buildingNo": "1234", "cityEn": "Riyadh", "countryEn": "Saudi Arabia"}, "code": "CLIENT-SENT"}
	p, err := simpleToL(customerFields, partyAddressToL)(x, in, nil, allKeys(in), true)
	if err != nil {
		t.Fatal(err)
	}
	if p["name"] != "Garage" || get(p, "national_address.building_no") != "1234" || p["country_code"] != "SA" {
		t.Fatalf("customer payload: %v", p)
	}
	if _, ok := p["code"]; ok {
		t.Fatal("code is server-assigned (legacy counter)")
	}
	if p["vat_no_in_arabic"] != "٣٠٠٠٠٠٠٠٠٠٠٠٠٠٣" || p["opening_balance_type"] != "receivable" {
		t.Fatalf("derived fields: %v", p)
	}
	errs := partyValidate(x, M{"nameEn": "A", "nameAr": "Latin", "vatNo": "123", "creditLimit": -1.0, "email": "x", "address": M{"postalCode": "1"}}, nil)
	for _, f := range []string{"nameEn", "nameAr", "vatNo", "creditLimit", "email", "address.postalCode"} {
		if _, ok := errs[f]; !ok {
			t.Errorf("missing validation %s: %v", f, errs)
		}
	}
}

func TestVendorMapping(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid})
	d := M{"_id": hexID(), "name": "Gulf Lub", "category_name": []interface{}{"Lubricants"}, "vat_percent": int32(15), "use_remarks_in_purchases": true}
	rec := simpleToC(vendorFields, vendorExtraToC)(x, d)
	if rec["vatPercent"] != 15.0 || len(strs(rec["category"])) != 1 || rec["useRemarksInPurchases"] != true {
		t.Fatalf("vendor: %v", rec)
	}
}

// ---------------- documents ----------------

func saleDoc(sid, pid string) M {
	pay := primitive.NewObjectID()
	return M{"_id": hexID(), "code": "S-INV-001", "date": time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC), "vat_percent": int32(15),
		"customer_id": primitive.NewObjectID(), "customer_name": "Riyadh Motors", "vat_no": "300000000000003",
		"products": []interface{}{M{"product_id": pid, "name": "Oil", "quantity": int32(2), "unit_price": "120", "purchase_unit_price": 80.0,
			"unit_discount": 5.0, "warehouse_id": nil, "warehouse_code": nil, "quantity_returned": 1.0, "item_code": "EO"}},
		"discount": 0.0, "shipping_handling_fees": 0.0, "net_total": 264.5, "total_payment_received": 264.5, "payment_status": "paid",
		"payments": []interface{}{M{"_id": pay, "date": time.Date(2026, 10, 1, 7, 0, 0, 0, time.UTC), "amount": 264.5, "method": "cash"}},
		"zatca":    M{"reporting_passed": true, "is_simplified": false, "qr_code": "QR", "reporting_passed_at": time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)},
		"uuid":     "u-1", "hash": "h", "prev_hash": "p", "invoice_count_value": int64(7), "rounding_amount": 0.0}
}

func TestSalesToContract(t *testing.T) {
	sid, pid := hexID(), hexID()
	cfg := newSalesResource().Backend.(*legacyBackend)
	x := testX(sid, M{"_id": sid})
	rec := cfg.toC(x, saleDoc(sid, pid))
	it := arr(rec["items"])[0].(M)
	if it["qty"] != 2.0 || it["unitPrice"] != 120.0 || it["unitDiscount"] != 5.0 || it["warehouseId"] != "ms_"+sid || it["qtyReturned"] != 1.0 {
		t.Fatalf("line: %v", it)
	}
	if rec["date"] != "2026-10-01T10:00" || rec["vatPercent"] != 15.0 || rec["code"] != "S-INV-001" {
		t.Fatalf("header: %v", rec)
	}
	pays := arr(rec["payments"])
	if len(pays) != 1 || pays[0].(M)["amount"] != 264.5 || pays[0].(M)["id"] == "" {
		t.Fatalf("payments: %v", pays)
	}
	z := sub(rec, "zatca")
	if z["status"] != "cleared" || z["invoiceType"] != "standard" || z["icv"] != int64(7) || z["qr"] != "QR" || z["pih"] != "p" {
		t.Fatalf("zatca: %v", z)
	}
	if get(rec, "legacyTotals.net") != 264.5 {
		t.Fatal("legacy totals")
	}
}

func TestZatcaStatusMapping(t *testing.T) {
	cases := []struct {
		d       M
		kind    string
		status  string
		invType string
	}{
		{M{}, "invoice", "not_reported", "simplified"},
		{M{"vat_no": "300000000000003"}, "invoice", "not_reported", "standard"},
		{M{"zatca": M{"reporting_passed": true, "is_simplified": true}}, "invoice", "reported", "simplified"},
		{M{"zatca": M{"reporting_failed_count": int64(2), "reporting_errors": []interface{}{"BR-KSA-44 invalid"}}}, "invoice", "failed", "simplified"},
		{M{"zatca": M{"reporting_passed": true, "is_simplified": false}}, "credit", "cleared", "credit"},
		{M{}, "credit", "not_reported", "credit-simplified"},
	}
	for i, c := range cases {
		z := zatcaToContract(riyadh, c.d, c.kind)
		if z["status"] != c.status || z["invoiceType"] != c.invType {
			t.Errorf("case %d: %v", i, z)
		}
	}
	if zatcaToContract(riyadh, cases[3].d, "invoice")["error"] != "BR-KSA-44 invalid" {
		t.Fatal("error surfaced")
	}
}

func TestSalesToLegacy_Create(t *testing.T) {
	sid, pid, wh := hexID(), hexID(), hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0}, M{"_id": wh, "code": "WH1"})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Oil 5W30", "item_code": "EO", "part_number": "PN", "unit": "pcs",
		"product_stores": M{sid: M{"purchase_unit_price": 80.0}}}
	b := newSalesResource().Backend.(*legacyBackend)
	rec := M{"date": "2026-10-05T11:30", "customerId": hexID(), "vatPercent": 15.0, "roundingAuto": true, "discount": 0.0,
		"items":    []interface{}{M{"productId": pid, "qty": 3.0, "unitPrice": 33.33, "unitDiscount": 0.0, "warehouseId": wh}},
		"payments": []interface{}{M{"id": "pay_client", "date": "2026-10-05T11:30", "amount": 115.0, "method": "cash"}, M{"amount": 0.0}}}
	p, err := b.toL(x, rec, nil, allKeys(rec), true)
	if err != nil {
		t.Fatal(err)
	}
	if p["date_str"] != "2026-10-05T11:30:00+03:00" || p["enable_report_to_zatca"] != false {
		t.Fatalf("header: %v", p)
	}
	line := arr(p["products"])[0].(M)
	if line["name"] != "Oil 5W30" || line["unit_price_with_vat"] != 38.3295 || line["warehouse_id"] != wh || line["warehouse_code"] != "WH1" ||
		line["purchase_unit_price"] != 80.0 || line["item_code"] != "EO" {
		t.Fatalf("line: %v", line)
	}
	pays := arr(p["payments_input"])
	if len(pays) != 1 {
		t.Fatalf("zero payment rows are dropped: %v", pays)
	}
	if _, ok := pays[0].(M)["id"]; ok {
		t.Fatal("client payment ids are not legacy ids: new payment")
	}
	// roundingAuto → explicit rounding to the nearest 0.05 (ks): 3*33.33=99.99 → vat 15.00 → 114.99 → 115.00
	if p["auto_rounding_amount"] != false || p["rounding_amount"] != 0.01 {
		t.Fatalf("rounding: %v %v", p["auto_rounding_amount"], p["rounding_amount"])
	}
}

// With the store's Warehouse module off the web app sends lines with no warehouse
// (or the virtual main store); both must save to the legacy main store bucket.
func TestSalesToLegacy_LineWithoutWarehouseGoesToMainStore(t *testing.T) {
	sid, pid := hexID(), hexID()
	for _, wh := range []interface{}{nil, "", "ms_" + sid} {
		x := testX(sid, M{"_id": sid, "vat_percent": 15.0})
		x.cache["product|"+pid] = M{"_id": pid, "name": "Oil", "product_stores": M{sid: M{}}}
		b := newSalesResource().Backend.(*legacyBackend)
		item := M{"productId": pid, "qty": 1.0, "unitPrice": 10.0, "unitDiscount": 0.0}
		if wh != nil {
			item["warehouseId"] = wh
		}
		rec := M{"date": "2026-10-05T11:30", "customerId": hexID(), "vatPercent": 15.0, "items": []interface{}{item}}
		p, err := b.toL(x, rec, nil, allKeys(rec), true)
		if err != nil {
			t.Fatalf("warehouseId %v: %v", wh, err)
		}
		line := arr(p["products"])[0].(M)
		if line["warehouse_id"] != nil || line["warehouse_code"] != nil {
			t.Fatalf("warehouseId %v: want main store, got %v", wh, line)
		}
	}
}

func TestSalesToLegacy_PaymentOnlyUpdateKeepsLines(t *testing.T) {
	sid, pid := hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	b := newSalesResource().Backend.(*legacyBackend)
	prevDoc := saleDoc(sid, pid)
	rec := b.toC(x, prevDoc)
	payID := str(arr(rec["payments"])[0].(M)["id"])
	rec["payments"] = append(arr(rec["payments"]), M{"date": "2026-10-02T09:00", "amount": 10.0, "method": "bank_card"})
	p, err := b.toL(x, rec, prevDoc, knownSet("payments"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p["products"]; ok {
		t.Fatal("lines must not be resent on a payment-only change (ZATCA edit lock)")
	}
	pays := arr(p["payments_input"])
	if len(pays) != 2 || pays[0].(M)["id"] != payID {
		t.Fatalf("existing payment must keep its legacy id, else legacy deletes it: %v", pays)
	}
	if p["date_str"] == nil {
		t.Fatal("legacy Validate requires date_str on every update")
	}
}

func TestDocLines_PreserveQuantityReturned(t *testing.T) {
	sid, pid := hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Oil"}
	cfg := &docCfg{priceKey: "unit_price", costKey: "purchase_unit_price"}
	prev := saleDoc(sid, pid)
	lines, err := cfg.linesToLegacy(x, M{"items": []interface{}{M{"productId": pid, "qty": 4.0, "unitPrice": 120.0, "warehouseId": "ms_" + sid}}}, prev, 15)
	if err != nil {
		t.Fatal(err)
	}
	if lines[0].(M)["quantity_returned"] != 1.0 || lines[0].(M)["item_code"] != "EO" {
		t.Fatalf("legacy-only line fields lost: %v", lines[0])
	}
}

func TestDocLines_Validation(t *testing.T) {
	sid, pid := hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Oil"}
	cfg := &docCfg{priceKey: "unit_price"}
	unknown := hexID()
	x.cache["product|"+unknown] = nil // known-missing: no DB access in unit tests
	_, err := cfg.linesToLegacy(x, M{"items": []interface{}{
		M{"productId": pid, "qty": 0.0, "unitPrice": 10.0},
		M{"productId": pid, "qty": 1.0, "unitPrice": -1.0},
		M{"productId": pid, "qty": 1.0, "unitPrice": 5.0, "unitDiscount": 6.0},
		M{"productId": unknown, "qty": 1.0, "unitPrice": 5.0},
	}}, nil, 15)
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected validation error, got %v", err)
	}
	for _, f := range []string{"items.0.qty", "items.1.unitPrice", "items.2.unitDiscount", "items.3.productId"} {
		if _, ok := ae.Fields[f]; !ok {
			t.Errorf("missing %s in %v", f, ae.Fields)
		}
	}
	if _, err := cfg.linesToLegacy(x, M{"items": []interface{}{}}, nil, 15); err == nil {
		t.Fatal("no items must fail")
	}
}

func TestDocValidate_PaymentsExceedNet(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0})
	cfg := &docCfg{party: "customer", payments: true, rounding: true, discount: true}
	rec := M{"date": "2026-10-05T10:00", "items": []interface{}{M{"qty": 1.0, "unitPrice": 100.0}},
		"payments": []interface{}{M{"amount": 116.0}}, "vatNo": "123"}
	errs := cfg.validate(x, rec, nil)
	if errs["payments"] == "" || errs["vatNo"] == "" {
		t.Fatalf("%v", errs)
	}
	rec["payments"] = []interface{}{M{"amount": 115.0}}
	rec["vatNo"] = ""
	if len(cfg.validate(x, rec, nil)) != 0 {
		t.Fatal("exactly net is fine")
	}
	if cfg.validate(x, M{}, nil)["date"] == "" {
		t.Fatal("date required on create")
	}
}

func TestKsTotals_SeedCheck(t *testing.T) {
	// contract §3 seed check: gross 5600, itemDisc 193.40, discount 30 → net 6183.09
	rec := M{"items": []interface{}{M{"qty": 1.0, "unitPrice": 5600.0, "unitDiscount": 193.40}}, "discount": 30.0}
	if got := ksTotals(rec, 15); got != 6183.09 {
		t.Fatalf("ks=%v", got)
	}
}

func TestReturnsMapping_SelectedAndLink(t *testing.T) {
	sid, pid, oid := hexID(), hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Oil"}
	x.cache["order|"+oid] = M{"_id": oid, "code": "S-INV-9"}
	b := newSalesReturnsResource().Backend.(*legacyBackend)
	d := M{"_id": hexID(), "order_id": oid, "order_code": "S-INV-9", "products": []interface{}{
		M{"product_id": pid, "quantity": 1.0, "unit_price": 10.0, "selected": true},
		M{"product_id": pid, "quantity": 3.0, "unit_price": 10.0, "selected": false}}}
	rec := b.toC(x, d)
	if len(arr(rec["items"])) != 1 || rec["orderId"] != oid || sub(rec, "zatca")["invoiceType"] != "credit-simplified" {
		t.Fatalf("return: %v", rec)
	}
	p, err := b.toL(x, M{"date": "2026-10-05T10:00", "orderId": oid, "items": []interface{}{M{"productId": pid, "qty": 1.0, "unitPrice": 10.0}}}, nil,
		knownSet("date", "orderId", "items"), true)
	if err != nil {
		t.Fatal(err)
	}
	if p["order_id"] != oid || p["order_code"] != "S-INV-9" || arr(p["products"])[0].(M)["selected"] != true {
		t.Fatalf("return payload: %v", p)
	}
	if _, err := b.toL(x, M{"date": "2026-10-05T10:00", "items": []interface{}{}}, nil, knownSet("date", "orderId"), true); err == nil {
		t.Fatal("orderId is required")
	}
}

func TestPurchaseLines(t *testing.T) {
	sid, pid := hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Filter"}
	b := newPurchasesResource().Backend.(*legacyBackend)
	p, err := b.toL(x, M{"date": "2026-10-05", "vendorId": hexID(), "vatPercent": 15.0,
		"items": []interface{}{M{"productId": pid, "qty": 2.0, "unitPrice": 9.0, "retailPrice": 25.0, "wholesalePrice": 22.0}}}, nil,
		knownSet("date", "vendorId", "vatPercent", "items"), true)
	if err != nil {
		t.Fatal(err)
	}
	line := arr(p["products"])[0].(M)
	if line["purchase_unit_price"] != 9.0 || line["retail_unit_price"] != 25.0 || line["wholesale_unit_price"] != 22.0 {
		t.Fatalf("purchase line: %v", line)
	}
	if errs := b.validate(x, M{"date": "2026-10-05"}, nil); errs["vendorId"] == "" {
		t.Fatal("vendor required")
	}
}

func TestNonVATForcesZero(t *testing.T) {
	sid, pid := hexID(), hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Item"}
	b := newNonVATSalesResource().Backend.(*legacyBackend)
	p, err := b.toL(x, M{"date": "2026-10-05", "vatPercent": 15.0, "items": []interface{}{M{"productId": pid, "qty": 1.0, "unitPrice": 10.0}}}, nil,
		knownSet("date", "items", "vatPercent"), true)
	if err != nil {
		t.Fatal(err)
	}
	if p["vat_percent"] != 0.0 || arr(p["products"])[0].(M)["unit_price_with_vat"] != 10.0 {
		t.Fatalf("non-VAT: %v", p)
	}
	if b.toC(x, M{"_id": hexID(), "vat_percent": 15.0})["vatPercent"] != 0.0 {
		t.Fatal("read side forced to 0")
	}
}

// ---------------- finance ----------------

func TestExpenseVATInclusiveMapping(t *testing.T) {
	sid, cat := hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	b := newExpensesResource().Backend.(*legacyBackend)
	rec := b.toC(x, M{"_id": hexID(), "amount": 1150.0, "vat_price": 150.0, "category_id": []interface{}{cat}, "payment_method": "cash"})
	if rec["amount"] != 1000.0 || rec["vatAmount"] != 150.0 || rec["categoryId"] != cat || rec["method"] != "cash" {
		t.Fatalf("expense: %v", rec)
	}
	x.cache["expense_category|"+cat] = M{"_id": cat, "name": "Rent"}
	p, err := b.toL(x, M{"date": "2026-10-05", "amount": 100.0, "vatAmount": 15.0, "categoryId": cat, "method": "cash"}, nil,
		knownSet("date", "amount", "vatAmount", "categoryId", "method"), true)
	if err != nil {
		t.Fatal(err)
	}
	if p["amount"] != 115.0 || p["description"] != "Rent" || len(arr(p["category_id"])) != 1 {
		t.Fatalf("expense payload: %v", p)
	}
	errs := b.validate(x, M{"amount": 0.0, "vatAmount": -1.0}, nil)
	for _, f := range []string{"categoryId", "date", "amount", "method", "vatAmount"} {
		if errs[f] == "" {
			t.Errorf("missing %s", f)
		}
	}
}

func TestDepositWithdrawalMapping(t *testing.T) {
	sid, cid, oid := hexID(), hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	x.cache["order|"+oid] = M{"_id": oid, "code": "S-INV-1"}
	b := newDepositsResource().Backend.(*legacyBackend)
	payID := primitive.NewObjectID()
	d := M{"_id": hexID(), "customer_id": cid, "net_total": 500.0, "payment_method": "", "type": "customer",
		"payments": []interface{}{M{"_id": payID, "amount": 500.0, "method": "cash", "invoice_id": oid, "invoice_code": "S-INV-1", "invoice_type": "sales"}}}
	rec := b.toC(x, d)
	if rec["amount"] != 500.0 || rec["method"] != "cash" || rec["orderId"] != oid || sub(rec, "zatca")["status"] != "not_reported" {
		t.Fatalf("deposit: %v", rec)
	}
	p, err := b.toL(x, M{"date": "2026-10-05T10:00", "customerId": cid, "amount": 200.0, "method": "bank_transfer", "orderId": oid}, d,
		knownSet("amount"), false)
	if err != nil {
		t.Fatal(err)
	}
	pay := arr(p["payments"])[0].(M)
	if pay["amount"] != 200.0 || pay["id"] != payID.Hex() || pay["invoice_code"] != "S-INV-1" || p["type"] != "customer" {
		t.Fatalf("payload: %v", p)
	}
	// multi-invoice legacy notes cannot be collapsed
	d["payments"] = append(arr(d["payments"]), M{"_id": primitive.NewObjectID(), "amount": 1.0})
	if _, err := b.toL(x, M{"date": "2026-10-05", "customerId": cid, "amount": 5.0, "method": "cash"}, d, knownSet("amount"), false); err == nil {
		t.Fatal("expected unsupported for multi-invoice notes")
	}
	w := newWithdrawalsResource().Backend.(*legacyBackend)
	if w.toC(x, M{"_id": hexID()})["type"] != "refund" {
		t.Fatal("withdrawal default type")
	}
}

func TestCapitalLikeMapping(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid})
	b := newDividendsResource().Backend.(*legacyBackend)
	uid := primitive.NewObjectID()
	rec := b.toC(x, M{"_id": hexID(), "withdrawn_by_user_id": uid, "withdrawn_by_user_name": "Owner", "amount": 50.0, "payment_method": "cash"})
	if rec["recipient"] != "Owner" || rec["recipientUserId"] != uid.Hex() {
		t.Fatalf("dividend: %v", rec)
	}
	p, err := b.toL(x, M{"date": "2026-10-05", "recipient": "Owner", "recipientUserId": uid.Hex(), "amount": 50.0, "method": "cash"}, nil,
		knownSet("date", "recipient", "recipientUserId", "amount", "method", "notes"), true)
	if err != nil || p["withdrawn_by_user_id"] != uid.Hex() || p["description"] != "dividends" {
		t.Fatalf("payload %v %v", p, err)
	}
}

func TestSalaryMapping(t *testing.T) {
	sid, eid := hexID(), hexID()
	x := testX(sid, M{"_id": sid})
	x.cache["employee|"+eid] = M{"_id": eid, "salary": 4000.0, "name_in_arabic": "أحمد"}
	b := newSalariesResource().Backend.(*legacyBackend)
	rec := b.toC(x, M{"_id": hexID(), "employee_id": eid, "month": int32(3), "year": int32(2026), "amount": 3500.0, "date": time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)})
	if rec["period"] != "2026-03" || rec["netSalary"] != 3500.0 || rec["basicSalary"] != 4000.0 || rec["employeeNameAr"] != "أحمد" || rec["paymentDate"] != "2026-04-01" {
		t.Fatalf("salary: %v", rec)
	}
	p, err := b.toL(x, M{"employeeId": eid, "period": "2026-05", "netSalary": 100.0, "method": "cash"}, nil,
		knownSet("employeeId", "period", "netSalary", "method"), true)
	if err != nil || p["month"] != int64(5) || p["year"] != int64(2026) || p["date_str"] == nil {
		t.Fatalf("payload %v %v", p, err)
	}
	if _, err := b.toL(x, M{"employeeId": eid, "period": "May"}, nil, knownSet("period", "employeeId"), true); err == nil {
		t.Fatal("bad period")
	}
}

// ---------------- master data ----------------

func TestEmployeeVehicleMapping(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid})
	e := newEmployeesResource().Backend.(*legacyBackend)
	if e.toC(x, M{"_id": hexID(), "is_active": false})["status"] != "inactive" || e.toC(x, M{"_id": hexID()})["status"] != "active" {
		t.Fatal("employee status (missing is_active = active)")
	}
	p, err := e.toL(x, M{"nameEn": "Sami", "joinDate": "2026-01-01", "status": "active"}, nil, knownSet("nameEn", "joinDate", "status"), true)
	if err != nil || p["salary_day"] != 1 || p["is_active"] != true || p["joining_date"] != "2026-01-01T00:00:00+03:00" {
		t.Fatalf("employee payload %v %v", p, err)
	}
	if errs := e.validate(x, M{"nameEn": "x", "nationalId": "3123"}, nil); errs["nationalId"] == "" {
		t.Fatal("national id rule")
	}
	v := newVehiclesResource().Backend.(*legacyBackend)
	vp, _ := v.toL(x, M{"plate": " abj   1234 ", "make": "Toyota", "customerId": hexID()}, nil, knownSet("plate", "make", "customerId"), true)
	if vp["vehicle_number"] != "ABJ 1234" {
		t.Fatalf("plate normalization: %v", vp["vehicle_number"])
	}
}

func TestWarehouseVirtual(t *testing.T) {
	sid := hexID()
	if !isMainStoreWarehouse(mainStoreWarehouseID(sid)) || isMainStoreWarehouse(hexID()) {
		t.Fatal("virtual id")
	}
	x := testX(sid, M{"_id": sid}, M{"_id": "w1hex", "code": "WH1"})
	if x.whContractID(nil, "WH1") != "w1hex" || x.whContractID(nil, "main_store") != "ms_"+sid || x.whContractID(nil, nil) != "ms_"+sid {
		t.Fatal("legacy → contract warehouse")
	}
	id, code, err := x.legacyWarehouse("ms_" + sid)
	if err != nil || id != nil || code != nil {
		t.Fatal("main store maps to nil warehouse")
	}
	if virtualWHGuard("ms_x") == nil {
		t.Fatal("virtual warehouse is read-only")
	}
}

func TestRepairJobAndTransferMapping(t *testing.T) {
	sid, pid := hexID(), hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Filter", "unit": "pcs"}
	p, err := repairToLegacy(x, M{"date": "2026-10-05T09:00", "complaint": "AC not cooling\nalso noise", "plate": "ABC",
		"parts": []interface{}{M{"productId": pid, "qty": 2.0, "unitPrice": 25.0}}, "status": "open"}, nil,
		knownSet("date", "complaint", "plate", "parts", "status", "vatPercent"), true)
	if err != nil {
		t.Fatal(err)
	}
	if p["title"] != "AC not cooling" || arr(p["parts"])[0].(M)["name"] != "Filter" || arr(p["parts"])[0].(M)["total_price"] != 50.0 {
		t.Fatalf("repair: %v", p)
	}
	tp, err := stockTransferToLegacy(x, M{"date": "2026-10-05", "fromWarehouseId": "ms_" + sid, "toWarehouseId": "ms_" + sid,
		"items": []interface{}{M{"productId": pid, "qty": 1.0, "unitPrice": 10.0}}}, nil, knownSet("items"), true)
	if err != nil || arr(tp["products"])[0].(M)["name"] != "Filter" {
		t.Fatalf("transfer %v %v", tp, err)
	}
	if errs := stockTransferValidate(x, M{"date": "d", "fromWarehouseId": "a", "toWarehouseId": "a", "items": []interface{}{M{"qty": 0.0}}}, nil); errs["toWarehouseId"] == "" || errs["items.0.qty"] == "" {
		t.Fatalf("transfer validation: %v", errs)
	}
}

func TestUsersMapping(t *testing.T) {
	sid := hexID()
	x := testX(sid, M{"_id": sid})
	u := M{"_id": hexID(), "name": "Sales", "email": "Sales@X.com", "role": "SalesMan", "store_ids": []interface{}{sid}, "erp": M{"x": M{"status": "inactive"}}}
	rec := userToContract(x, u)
	if rec["role"] != "r_salesman" || rec["email"] != "sales@x.com" || rec["status"] != "inactive" || len(strs(rec["storeIds"])) != 1 {
		t.Fatalf("user: %v", rec)
	}
	p, err := userToLegacy(x, M{"name": "New", "email": " NEW@X.COM ", "phone": "0551112222", "role": "r_admin", "storeIds": []interface{}{sid}}, nil,
		knownSet("name", "email", "phone", "role", "storeIds"), true)
	if err != nil || p["role"] != "Manager" || p["email"] != "new@x.com" || len(str(p["password"])) < 8 {
		t.Fatalf("user payload %v %v", p, err)
	}
	if _, err := userToLegacy(x, M{"name": "x", "email": "a@b.c", "role": "r_viewer", "storeIds": []interface{}{hexID()}}, nil, knownSet("storeIds"), true); err == nil {
		t.Fatal("cannot grant inaccessible stores")
	}
	self := M{"_id": x.c.UserID.Hex(), "role": "Manager", "erp": M{"role": "r_manager"}}
	if _, err := userToLegacy(x, M{"name": "x", "email": "a@b.c", "role": "r_viewer", "storeIds": []interface{}{sid}}, self, knownSet("role"), false); err == nil {
		t.Fatal("cannot change own role")
	}
	legacyAdmin := M{"_id": hexID(), "role": "Admin"}
	pa, _ := userToLegacy(x, M{"name": "x", "email": "a@b.c", "role": "r_viewer", "storeIds": []interface{}{sid}}, legacyAdmin, knownSet("role"), false)
	if pa["role"] != "Admin" {
		t.Fatal("never demote a legacy platform admin")
	}
}

func TestRolesValidation(t *testing.T) {
	if errs := roleValidate(nil, M{"name": "admin", "maxDiscount": 150.0}, "zz"); errs["maxDiscount"] == "" {
		t.Fatalf("%v", errs)
	}
}

func TestDraftHelpers(t *testing.T) {
	if draftTitle(M{"customerName": "Ali", "items": []interface{}{1, 2}}) != "Ali · 2 line(s)" {
		t.Fatal(draftTitle(M{"customerName": "Ali", "items": []interface{}{1, 2}}))
	}
	for alias, t2 := range draftAliases {
		if _, ok := draftTypes[t2]; !ok {
			t.Errorf("alias %s → unknown %s", alias, t2)
		}
	}
	if len(draftTypes) != 12 {
		t.Fatal("12 draft collections per §2.1b")
	}
}

func TestAllResourcesCoverContract(t *testing.T) {
	want := []string{"stores", "users", "roles", "categories", "brands", "customer-categories", "vendor-categories", "expense-categories",
		"accounts", "warehouses", "products", "customers", "vendors", "employees", "vehicles", "signatures", "packages", "rfq-suppliers",
		"sales", "quotations", "proformas", "sales-returns", "delivery-notes", "nonvat-sales", "nonvat-returns", "quotation-returns",
		"purchases", "purchase-orders", "purchase-requests", "purchase-returns", "purchase-bills", "stock-transfers", "expenses",
		"deposits", "withdrawals", "capitals", "capital-withdrawals", "dividends", "salaries", "repair-jobs", "rfqs", "threads", "notifications"}
	got := map[string]*Resource{}
	for _, r := range allResources() {
		got[r.Path] = r
	}
	if len(want) != 43 || len(got) != 43 {
		t.Fatalf("want 43 resources, registry has %d", len(got))
	}
	org := knownSet("stores", "users", "roles", "categories", "brands", "customer-categories", "vendor-categories", "expense-categories", "accounts")
	for _, p := range want {
		r, ok := got[p]
		if !ok {
			t.Errorf("missing resource %s", p)
			continue
		}
		if org[p] != (r.Scope == "org") {
			t.Errorf("%s scope %s", p, r.Scope)
		}
		if r.Legacy == "" {
			t.Errorf("%s has no backing description", p)
		}
	}
}

func TestPaymentMethodEnum(t *testing.T) {
	for _, m := range []string{"cash", "debit_card", "credit_card", "bank_card", "bank_transfer", "bank_cheque", "customer_account", "vendor_account"} {
		if !validPaymentMethod(m) {
			t.Errorf("%s must be accepted", m)
		}
	}
	for _, m := range []string{"bank", "Cash", "card", "mada"} {
		if validPaymentMethod(m) {
			t.Errorf("%s must be rejected (legacy would post a ledger line without account)", m)
		}
	}
}

// The WhatsApp settings tab reads whatsapp.mode / evolution.status / waba.*.
// Those have no legacy field and are preserved in erp.x; the mapped part must
// not clobber them, and an unset SMTP port must be null (the client rejects 0).
func TestStoreToContract_WhatsappAndSmtpPort(t *testing.T) {
	sid := hexID()
	cases := []struct {
		name     string
		doc      M
		wantMode string
		wantPort interface{}
	}{
		{"fresh store", M{"_id": sid, "name": "A"}, "waba", nil},
		{"port 0", M{"_id": sid, "name": "A", "settings": M{"outgoing_email_smtp_port": int32(0)}}, "waba", nil},
		{"port set", M{"_id": sid, "name": "A", "settings": M{"outgoing_email_smtp_port": int32(587)}}, "waba", int64(587)},
		{"saved waba mode", M{"_id": sid, "name": "A", envKey: M{"x": M{"whatsapp": M{"mode": "waba"}}}}, "waba", nil},
		{"saved empty mode", M{"_id": sid, "name": "A", envKey: M{"x": M{"whatsapp": M{"mode": ""}}}}, "waba", nil},
		// WhatsApp is WABA-only: a mode an older client saved is reported as waba
		{"saved legacy mode", M{"_id": sid, "name": "A", envKey: M{"x": M{"whatsapp": M{"mode": "evolution"}}}}, "waba", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := storeToContract(testX(sid, tc.doc), tc.doc)
			if got := get(rec, "whatsapp.mode"); got != tc.wantMode {
				t.Errorf("mode=%v want %v", got, tc.wantMode)
			}
			if got := get(rec, "emailSettings.smtp.port"); got != tc.wantPort {
				t.Errorf("port=%#v want %#v", got, tc.wantPort)
			}
			// mapped evolution never carries status: that is preserved state
			if _, ok := sub(rec, "whatsapp.evolution")["status"]; ok {
				t.Errorf("mapped evolution must not set status")
			}
		})
	}
}

func TestStoreEnvelope_PreservesWhatsappState(t *testing.T) {
	sid := hexID()
	doc := M{"_id": sid, "name": "A",
		"settings": M{"evolution_api_url": "https://wa.example", "evolution_instance_name": "shop1", "evolution_api_key": "real-key"},
		envKey: M{"v": int64(3), "x": M{"whatsapp": M{
			"mode":      "waba",
			"evolution": M{"url": "stale", "instance": "stale", "apiKey": "real-key", "status": "connected"},
			"waba":      M{"phoneNumberId": "1098765432101", "businessAccountId": "2233445566778", "verified": true},
		}}}}
	rec := applyEnvelope(storeToContract(testX(sid, doc), doc), doc, false)
	checks := map[string]interface{}{
		"whatsapp.mode": "waba", "whatsapp.evolution.status": "connected",
		"whatsapp.evolution.url": "https://wa.example", "whatsapp.evolution.instance": "shop1",
		"whatsapp.evolution.apiKey": masked, "whatsapp.waba.verified": true,
		"whatsapp.waba.phoneNumberId": "1098765432101",
	}
	for path, want := range checks {
		if got := get(rec, path); got != want {
			t.Errorf("%s=%#v want %#v", path, got, want)
		}
	}
}
