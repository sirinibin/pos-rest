package erp

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// A sales invoice reported to ZATCA (Phase 2) only takes changes to its cash
// discount and its payments (user, 2026-10-11, POS terminal request).

func TestZatcaLockOn(t *testing.T) {
	reported := M{"zatca": M{"reporting_passed": true}}
	phase2 := M{"zatca": M{"phase": "2"}}
	off := false
	on := true
	for _, c := range []struct {
		name  string
		store M
		doc   M
		want  bool
	}{
		{"reported, phase 2, setting unset", phase2, reported, true},
		{"reported, phase 2, setting on", M{"zatca": M{"phase": "2"}, "settings": M{"disable_sales_edit_once_reported_to_zatca": on}}, reported, true},
		{"reported, phase 2, setting off", M{"zatca": M{"phase": "2"}, "settings": M{"disable_sales_edit_once_reported_to_zatca": off}}, reported, false},
		{"reported, phase 1", M{"zatca": M{"phase": "1"}}, reported, false},
		{"reported, no store", nil, reported, false},
		{"not reported", phase2, M{"zatca": M{"reporting_passed": false}}, false},
		{"failed report", phase2, M{"zatca": M{"reporting_failed_count": 2}}, false},
		{"no zatca block", phase2, M{}, false},
		{"no doc", phase2, nil, false},
	} {
		if got := zatcaLockOn(c.store, c.doc); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestZatcaLockedChanges(t *testing.T) {
	known := newSalesResource().Backend.(*legacyBackend).known
	prev := M{"cashDiscount": 0.0, "payments": []interface{}{}, "remarks": "a", "discount": 0.0,
		"items": []interface{}{M{"qty": 1.0}}, "posMeta": M{"note": "x"}, "date": "2026-10-11T10:00"}
	for _, c := range []struct {
		name    string
		next    M
		changed []string
		want    []string
	}{
		{"cash discount only", M{"cashDiscount": 5.0}, []string{"cashDiscount"}, []string{}},
		{"payments only", M{"payments": []interface{}{M{"amount": 10.0}}}, []string{"payments"}, []string{}},
		{"both allowed", M{"cashDiscount": 1.0, "payments": []interface{}{M{"amount": 1.0}}}, []string{"cashDiscount", "payments"}, []string{}},
		{"items", M{"items": []interface{}{M{"qty": 2.0}}}, []string{"items"}, []string{"items"}},
		{"remarks and discount", M{"remarks": "b", "discount": 3.0}, []string{"discount", "remarks"}, []string{"discount"}},
		{"remarks only (not in the XML)", M{"remarks": "Job JC-1"}, []string{"remarks"}, []string{}},
		{"job card and vehicle links", M{"repairJobId": "j1", "vehicleId": "v1"}, []string{"repairJobId", "vehicleId"}, []string{}},
		{"date", M{"date": "2026-10-12T10:00"}, []string{"date"}, []string{"date"}},
		{"customer", M{"customerId": "abc"}, []string{"customerId"}, []string{"customerId"}},
		{"commission", M{"commission": 4.0}, []string{"commission"}, []string{"commission"}},
		{"value resent unchanged", M{"remarks": "a", "items": []interface{}{M{"qty": 1.0}}}, []string{"items", "remarks"}, []string{}},
		{"client-only extras", M{"posMeta": M{"note": "y"}}, []string{"posMeta"}, []string{}},
		{"mixed", M{"cashDiscount": 2.0, "shipping": 9.0}, []string{"cashDiscount", "shipping"}, []string{"shipping"}},
	} {
		next := cloneM(prev)
		for k, v := range c.next {
			next[k] = v
		}
		got := zatcaLockedChanges(known, prev, next, c.changed)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestZatcaLockGuard_Message(t *testing.T) {
	b := newSalesResource().Backend.(*legacyBackend)
	x := &mapCtx{store: M{"zatca": M{"phase": "2"}}}
	prevDoc := M{"zatca": M{"reporting_passed": true}}
	err := b.updateGuard(x, prevDoc, M{"discount": 0.0}, M{"discount": 2.0}, []string{"discount"})
	ae, ok := err.(*APIError)
	if !ok || ae.Status != 400 || ae.Fields["discount"] == "" || !strings.Contains(ae.Message, "reported to ZATCA") {
		t.Fatalf("got %#v", err)
	}
	if err := b.updateGuard(x, prevDoc, M{"cashDiscount": 0.0}, M{"cashDiscount": 3.0}, []string{"cashDiscount"}); err != nil {
		t.Fatalf("cash discount refused: %v", err)
	}
	if err := b.updateGuard(x, M{"zatca": M{}}, M{"discount": 0.0}, M{"discount": 2.0}, []string{"discount"}); err != nil {
		t.Fatalf("unreported invoice refused: %v", err)
	}
	// the other document types have no lock
	for _, r := range []*Resource{newQuotationsResource(), newSalesReturnsResource()} {
		if r.Backend.(*legacyBackend).updateGuard != nil {
			t.Errorf("%s has a ZATCA lock", r.Name)
		}
	}
}

func TestZatcaLocked_RenderedOnSales(t *testing.T) {
	b := newSalesResource().Backend.(*legacyBackend)
	sid := hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0, "zatca": M{"phase": "2"}})
	d := M{"_id": hexID(), "code": "S-1", "zatca": M{"reporting_passed": true, "is_simplified": true}}
	if rec := b.toC(x, d); rec["zatcaLocked"] != true {
		t.Fatalf("reported invoice: zatcaLocked %v", rec["zatcaLocked"])
	}
	d["zatca"] = M{}
	if rec := b.toC(x, d); rec["zatcaLocked"] != false {
		t.Fatalf("unreported invoice: zatcaLocked %v", rec["zatcaLocked"])
	}
}

// End to end on MongoDB: a reported sale refuses edits except the cash
// discount and payments; the store setting switches the lock off.
func TestAPI_ZatcaReportedSaleLock(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sid, _ := oidOf(storeA())
	ctx, cancel := dbctx()
	var before bson.M
	_ = mainDB().Collection("store").FindOne(ctx, bson.M{"_id": sid}).Decode(&before)
	cancel()
	restore := bson.M{}
	unset := bson.M{}
	for _, k := range []string{"phase"} {
		if z, ok := before["zatca"].(bson.M); ok && z[k] != nil {
			restore["zatca."+k] = z[k]
		} else {
			unset["zatca."+k] = ""
		}
	}
	if s, ok := before["settings"].(bson.M); ok && s["disable_sales_edit_once_reported_to_zatca"] != nil {
		restore["settings.disable_sales_edit_once_reported_to_zatca"] = s["disable_sales_edit_once_reported_to_zatca"]
	} else {
		unset["settings.disable_sales_edit_once_reported_to_zatca"] = ""
	}
	setStore := func(set, uns bson.M) {
		ctx, cancel := dbctx()
		defer cancel()
		u := bson.M{}
		if len(set) > 0 {
			u["$set"] = set
		}
		if len(uns) > 0 {
			u["$unset"] = uns
		}
		if len(u) > 0 {
			if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": sid}, u); err != nil {
				t.Fatal(err)
			}
		}
	}
	defer setStore(restore, unset)
	setStore(bson.M{"zatca.phase": "2"}, bson.M{"settings.disable_sales_edit_once_reported_to_zatca": ""})

	sale := call(t, "POST", "/sales", tok, M{"storeId": storeA(), "date": "2026-10-11T09:00", "remarks": "lock test",
		"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 2, "unitPrice": 50, "warehouseId": "ms_" + storeA()}}})
	if sale.Code != 201 {
		t.Fatalf("sale: %d %s", sale.Code, sale.Raw)
	}
	id := str(sale.Body["id"])
	if sale.Body["zatcaLocked"] != false {
		t.Fatalf("new sale locked: %v", sale.Body["zatcaLocked"])
	}
	// not reported yet: anything may change
	if r := call(t, "PATCH", "/sales/"+id, tok, M{"remarks": "edited before reporting"}); r.Code != 200 {
		t.Fatalf("edit before reporting: %d %s", r.Code, r.Raw)
	}
	oid, _ := oidOf(id)
	ctx, cancel = dbctx()
	_, _ = storeDB(storeA()).Collection("order").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"zatca.reporting_passed": true, "zatca.is_simplified": true, "zatca.reporting_passed_at": time.Now()}})
	cancel()
	g := call(t, "GET", "/sales/"+id, tok, nil)
	if g.Code != 200 || g.Body["zatcaLocked"] != true || get(g.Body, "zatca.status") != "reported" {
		t.Fatalf("reported sale: %d zatcaLocked=%v status=%v", g.Code, g.Body["zatcaLocked"], get(g.Body, "zatca.status"))
	}
	items := g.Body["items"]
	for _, c := range []struct {
		name  string
		patch M
		field string
	}{
		{"line quantity", M{"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 3, "unitPrice": 50, "warehouseId": "ms_" + storeA()}}}, "items"},
		{"bill discount", M{"discount": 5}, "discount"},
		{"date", M{"date": "2026-10-11T11:00"}, "date"},
		{"phone", M{"phone": "0500000000"}, "phone"},
		{"shipping", M{"shipping": 7}, "shipping"},
		{"commission", M{"commission": 3, "commissionMethod": "cash"}, "commission"},
		{"cash discount with a line change", M{"cashDiscount": 1, "items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 50, "warehouseId": "ms_" + storeA()}}}, "items"},
	} {
		r := call(t, "PATCH", "/sales/"+id, tok, c.patch)
		if r.Code != 400 || r.errField(c.field) == "" || !strings.Contains(str(get(r.Body, "error.message")), "ZATCA") {
			t.Errorf("%s: want 400 on %s, got %d %s", c.name, c.field, r.Code, r.Raw)
		}
	}
	// nothing changed on the invoice
	g2 := call(t, "GET", "/sales/"+id, tok, nil)
	if !reflect.DeepEqual(g2.Body["items"], items) || g2.Body["remarks"] != "edited before reporting" || num(g2.Body["discount"]) != 0 {
		t.Fatalf("a refused edit changed the invoice: %s", g2.Raw)
	}
	// the remarks are not in the reported XML: a terminal's note after the sale is taken
	if r := call(t, "PATCH", "/sales/"+id, tok, M{"remarks": "Job JC-1 · ABC 123"}); r.Code != 200 || r.Body["remarks"] != "Job JC-1 · ABC 123" {
		t.Fatalf("remarks after reporting: %d %s", r.Code, r.Raw)
	}
	// the same items resent unchanged are not a change
	if r := call(t, "PATCH", "/sales/"+id, tok, M{"items": items, "cashDiscount": 4}); r.Code != 200 || num(r.Body["cashDiscount"]) != 4 {
		t.Fatalf("cash discount with unchanged items: %d %s", r.Code, r.Raw)
	}
	// payments received
	pay := []M{{"date": "2026-10-11T12:00", "amount": 50, "method": "cash"}}
	r := call(t, "PATCH", "/sales/"+id, tok, M{"payments": pay})
	if r.Code != 200 || len(arr(r.Body["payments"])) != 1 || num(get(arr(r.Body["payments"])[0].(M), "amount")) != 50 {
		t.Fatalf("payments: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "PATCH", "/sales/"+id, tok, M{"cashDiscount": 2.5, "payments": []M{{"date": "2026-10-11T12:00", "amount": 60, "method": "cash"}}}); r.Code != 200 || num(r.Body["cashDiscount"]) != 2.5 {
		t.Fatalf("cash discount + payments: %d %s", r.Code, r.Raw)
	}
	// the store setting switched off: the invoice may be edited again
	setStore(bson.M{"settings.disable_sales_edit_once_reported_to_zatca": false}, nil)
	if r := call(t, "PATCH", "/sales/"+id, tok, M{"discount": 1}); r.Code != 200 || r.Body["zatcaLocked"] != false {
		t.Fatalf("lock off: %d %s", r.Code, r.Raw)
	}
	// Phase 1: no lock either
	setStore(bson.M{"zatca.phase": "1"}, bson.M{"settings.disable_sales_edit_once_reported_to_zatca": ""})
	if r := call(t, "PATCH", "/sales/"+id, tok, M{"discount": 2}); r.Code != 200 {
		t.Fatalf("phase 1: %d %s", r.Code, r.Raw)
	}
}
