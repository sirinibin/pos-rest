package erp

import (
	"testing"
)

// POS terminals (Industrial supplies, Auto spare parts) save a cash discount and a
// salesman commission on the sale exactly like the Sales form: cashDiscount,
// commission and commissionMethod map to the legacy cash_discount, commission and
// commission_payment_method, the cash discount never changes the invoice totals, and
// payments + cash discount may not exceed the net total.

func posSaleRec(pid, wh string, over M) M {
	rec := M{"date": "2026-10-09T10:00", "customerId": nil, "customerName": "", "vatPercent": 15.0, "discount": 0.0,
		"shipping": 0.0, "rounding": 0.0, "formType": "pos", "posType": "parts", "posMeta": M{},
		"items":      []interface{}{M{"productId": pid, "qty": 1.0, "unitPrice": 100.0, "unitDiscount": 0.0, "warehouseId": wh}},
		"payments":   []interface{}{M{"date": "2026-10-09T10:00", "amount": 110.0, "method": "cash"}},
		"commission": 0.0, "commissionMethod": "cash", "cashDiscount": 0.0}
	for k, v := range over {
		rec[k] = v
	}
	return rec
}

func posSaleCtx() (*mapCtx, string, string) {
	sid, pid, wh := hexID(), hexID(), hexID()
	x := testX(sid, M{"_id": sid, "vat_percent": 15.0}, M{"_id": wh, "code": "WH1"})
	x.cache["product|"+pid] = M{"_id": pid, "name": "Brake pads front", "item_code": "BP", "part_number": "BP-1", "unit": "pcs",
		"product_stores": M{sid: M{"purchase_unit_price": 60.0}}}
	return x, pid, wh
}

func TestPosSaleToLegacy_CashDiscountAndCommission(t *testing.T) {
	cases := []struct {
		name             string
		over             M
		wantCD, wantComm float64
		wantMethod       string
	}{
		{"none", M{}, 0, 0, "cash"},
		{"cash discount only", M{"cashDiscount": 5.0}, 5, 0, "cash"},
		{"commission by bank transfer", M{"commission": 15.0, "commissionMethod": "bank_transfer"}, 0, 15, "bank_transfer"},
		{"both", M{"cashDiscount": 2.3, "commission": 7.5, "commissionMethod": "cash"}, 2.3, 7.5, "cash"},
		{"commission with no method defaults to cash", M{"commission": 4.0, "commissionMethod": ""}, 0, 4, "cash"},
	}
	b := newSalesResource().Backend.(*legacyBackend)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, pid, wh := posSaleCtx()
			rec := posSaleRec(pid, wh, c.over)
			p, err := b.toL(x, rec, nil, allKeys(rec), true)
			if err != nil {
				t.Fatal(err)
			}
			if p["cash_discount"] != c.wantCD || p["commission"] != c.wantComm || p["commission_payment_method"] != c.wantMethod {
				t.Fatalf("cash_discount %v commission %v method %v", p["cash_discount"], p["commission"], p["commission_payment_method"])
			}
			// the invoice itself is untouched: same line price and no discount
			if p["discount"] != 0.0 {
				t.Fatalf("discount %v", p["discount"])
			}
			line := arr(p["products"])[0].(M)
			if line["unit_price"] != 100.0 {
				t.Fatalf("line %v", line)
			}
		})
	}
}

func TestPosSaleToLegacy_OnlyChangedFieldsOnUpdate(t *testing.T) {
	x, pid, wh := posSaleCtx()
	b := newSalesResource().Backend.(*legacyBackend)
	rec := posSaleRec(pid, wh, M{"cashDiscount": 5.0, "commission": 9.0})
	p, err := b.toL(x, rec, rec, map[string]bool{"cashDiscount": true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if p["cash_discount"] != 5.0 {
		t.Fatalf("cash_discount %v", p["cash_discount"])
	}
	if _, ok := p["commission"]; ok {
		t.Fatal("unchanged commission is not resent")
	}
}

func TestPosSaleToContract_CashDiscountAndCommission(t *testing.T) {
	sid, pid := hexID(), hexID()
	b := newSalesResource().Backend.(*legacyBackend)
	x := testX(sid, M{"_id": sid})
	d := saleDoc(sid, pid)
	d["cash_discount"] = 4.5
	d["commission"] = 12.0
	d["commission_payment_method"] = "bank_transfer"
	rec := b.toC(x, d)
	if rec["cashDiscount"] != 4.5 || rec["commission"] != 12.0 || rec["commissionMethod"] != "bank_transfer" {
		t.Fatalf("%v %v %v", rec["cashDiscount"], rec["commission"], rec["commissionMethod"])
	}
}

func TestPosSaleValidate_PaymentsPlusCashDiscount(t *testing.T) {
	cfg := &docCfg{party: "customer", payments: true, rounding: true, discount: true, cashDiscount: true, commission: true}
	cases := []struct {
		name    string
		paid    float64
		cd      float64
		wantErr bool
	}{
		{"paid in full, no cash discount", 115, 0, false},
		{"paid less the cash discount", 110, 5, false},
		{"credit sale with a cash discount", 0, 5, false},
		{"paid in full and a cash discount", 115, 5, true},
		{"partly paid, cash discount tips it over", 112, 5, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, pid, wh := posSaleCtx()
			pays := []interface{}{}
			if c.paid > 0 {
				pays = append(pays, M{"amount": c.paid, "method": "cash"})
			}
			rec := posSaleRec(pid, wh, M{"payments": pays, "cashDiscount": c.cd})
			errs := cfg.validate(x, rec, nil)
			if (errs["payments"] != "") != c.wantErr {
				t.Fatalf("errs %v", errs)
			}
		})
	}
}
