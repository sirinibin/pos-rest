package erp

import "testing"

// POS Documents (quotations, proforma invoices, purchases and non-VAT sales
// made from a POS terminal) lists ask for one terminal's documents with
// ?where.posType=<terminal>.
func TestPosDocumentsWherePosType(t *testing.T) {
	for _, name := range []string{"sales", "quotations", "nonvatSales", "purchases"} {
		b := resourceByName(name).Backend.(*legacyBackend)
		f, _, err := b.listExtras(nil, "", ListQuery{Where: map[string]string{"posType": "grocery"}})
		if err != nil || f["erp.x.posType"] != "grocery" {
			t.Errorf("%s: filter %v err %v", name, f, err)
		}
		// a comma list matches any of the terminals
		f, _, err = b.listExtras(nil, "", ListQuery{Where: map[string]string{"posType": "grocery,parts"}})
		if err != nil || f["erp.x.posType"] == nil {
			t.Errorf("%s: comma list %v err %v", name, f, err)
		}
	}
	// returns are not made from a POS terminal
	for _, name := range []string{"salesReturns", "purchaseReturns", "quotationReturns", "nonvatReturns", "deliveryNotes"} {
		b := resourceByName(name).Backend.(*legacyBackend)
		if _, _, err := b.listExtras(nil, "", ListQuery{Where: map[string]string{"posType": "grocery"}}); err == nil || err.(*APIError).Status != 400 {
			t.Errorf("%s accepted where.posType: %v", name, err)
		}
	}
}

func TestProformasWhereKeys(t *testing.T) {
	b := resourceByName("proformas").Backend.(*nativeBackend)
	cases := []struct {
		key  string
		want bool
	}{
		{"posType", true},
		{"customerId", true},
		{"status", false},
		{"amount", false},
		{"", false},
	}
	for _, c := range cases {
		if got := b.whereKeys[c.key]; got != c.want {
			t.Errorf("whereKeys[%q]=%v want %v", c.key, got, c.want)
		}
	}
	if b.serialKey != "proforma" || b.coll != "erp_proforma" || b.dateField != "date" {
		t.Errorf("proforma backend changed: %+v", b)
	}
}

// The store's non-VAT sales switch (legacy settings.non_vat_sales) is the
// contract flag enable_nonvat_sales, read and written both ways.
func TestStoreFlagNonVatSales(t *testing.T) {
	if settingsFlag["enable_nonvat_sales"] != "non_vat_sales" {
		t.Fatalf("settingsFlag: %v", settingsFlag)
	}
	sid := hexID()
	for _, c := range []struct {
		doc  M
		want bool
	}{
		{M{"_id": sid, "name": "A", "settings": M{"non_vat_sales": true}}, true},
		{M{"_id": sid, "name": "A", "settings": M{"non_vat_sales": false}}, false},
		{M{"_id": sid, "name": "A", "settings": M{}}, false},
		{M{"_id": sid, "name": "A"}, false},
	} {
		rec := storeToContract(testX(sid, c.doc), c.doc)
		if got := get(rec, "flags.enable_nonvat_sales"); got != c.want {
			t.Errorf("%v: flags.enable_nonvat_sales=%v want %v", c.doc["settings"], got, c.want)
		}
	}
	prev := M{"_id": sid, "settings": M{"non_vat_sales": false}}
	for _, on := range []bool{true, false} {
		p, err := storeToLegacy(testX(sid, prev), M{"flags": M{"enable_nonvat_sales": on}}, prev, knownSet("flags"), false)
		if err != nil {
			t.Fatal(err)
		}
		if got := get(p, "settings.non_vat_sales"); got != on {
			t.Errorf("write %v: settings.non_vat_sales=%v", on, got)
		}
	}
}

// Documents made in a POS terminal keep its posType and its Documents list
// finds them with ?where.posType (needs the test database).
func TestAPI_PosDocumentsByTerminal(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sA, ms, dt := storeA(), "ms_"+storeA(), "2026-10-09T10:00"
	line := func(price, vat float64) []M {
		return []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": price, "unitDiscount": 0, "warehouseId": ms, "vatPercent": vat}}
	}
	docs := []struct {
		path string
		body M
	}{
		{"/quotations", M{"type": "quotation", "status": "created", "validityDays": 2, "deliveryDays": 7, "customerId": fx.CustomerA1.Hex(), "items": line(25, 15)}},
		{"/proformas", M{"type": "invoice", "status": "created", "validityDays": 15, "deliveryDays": 7, "items": line(40, 15)}},
		{"/purchases", M{"vendorId": fx.VendorA1.Hex(), "items": line(18, 15)}},
		{"/nonvat-sales", M{"customerId": fx.CustomerA1.Hex(), "vatPercent": 0, "items": line(30, 0)}},
	}
	for _, d := range docs {
		body := M{"storeId": sA, "date": dt, "posType": "grocery"}
		for k, v := range d.body {
			body[k] = v
		}
		cr := call(t, "POST", d.path, tok, body)
		if cr.Code != 201 && cr.Code != 200 {
			t.Fatalf("%s create: %d %s", d.path, cr.Code, cr.Raw)
		}
		id := str(cr.Body["id"])
		if got := str(cr.Body["posType"]); got != "grocery" {
			t.Errorf("%s posType = %q", d.path, got)
		}
		has := func(term string) bool {
			res := call(t, "GET", d.path+"?storeId="+sA+"&where.posType="+term+"&sort=-date&limit=50", tok, nil)
			if res.Code != 200 {
				t.Fatalf("%s list %s: %d %s", d.path, term, res.Code, res.Raw)
			}
			for _, r := range res.data() {
				if str(r.(M)["id"]) == id {
					return true
				}
			}
			return false
		}
		if !has("grocery") {
			t.Errorf("%s: not listed for its terminal", d.path)
		}
		if has("parts") {
			t.Errorf("%s: listed for another terminal", d.path)
		}
	}
}
