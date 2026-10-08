package erp

import (
	"math"
	"testing"
)

// The legacy save rounds the running line sum to 2 decimals after every line;
// the web app sums exactly. With VAT-inclusive prices (4-decimal unit prices)
// the two nets can be a cent apart, and the legacy save then rejected a
// payment of the full total ("Total payment should not exceed: 329.99"), so a
// POS sale of 180.00 + 150.00 was lost. rounding_amount carries the cent.
func TestLegacyBeforeRoundingAndCentFix(t *testing.T) {
	line := func(qty, price, disc float64) M { return M{"qty": qty, "unitPrice": price, "unitDiscount": disc} }
	items := func(ls ...M) []interface{} {
		out := []interface{}{}
		for _, l := range ls {
			out = append(out, l)
		}
		return out
	}
	for _, c := range []struct {
		name          string
		rec           M
		vat           float64
		client, legal float64
	}{
		// 180.00 and 150.00 incl. 15% VAT (the workshop POS case)
		{"workshop services", M{"items": items(line(1, 156.5217, 0), line(1, 130.4348, 0))}, 15, 330.00, 329.99},
		{"2-decimal prices agree", M{"items": items(line(2, 10, 0), line(1, 5.5, 0.5))}, 15, 28.75, 28.75},
		{"one line agrees", M{"items": items(line(3, 1.7391, 0))}, 15, 6.00, 6.00},
		{"discount and shipping", M{"items": items(line(1, 156.5217, 0), line(1, 130.4348, 0)), "discount": 10.0, "shipping": 5.0}, 15, 324.25, 324.24},
		{"empty", M{}, 15, 0, 0},
	} {
		if got := ksTotals(c.rec, c.vat); math.Abs(got-c.client) > 1e-9 {
			t.Errorf("%s: client net %.2f, want %.2f", c.name, got, c.client)
		}
		if got := legacyBeforeRounding(c.rec, c.vat); math.Abs(got-c.legal) > 1e-9 {
			t.Errorf("%s: legacy net %.2f, want %.2f", c.name, got, c.legal)
		}
		fix := roundingCentFix(c.rec, c.vat)
		if math.Abs(round2(c.legal+fix)-c.client) > 1e-9 {
			t.Errorf("%s: legacy net %.2f + fix %.2f ≠ client net %.2f", c.name, c.legal, fix, c.client)
		}
	}
}

// Against the database: the workshop sale is saved with its full payment, the
// legacy net is the client's net, and the rounding reads back as sent.
func TestAPI_SaleCentDriftIsAbsorbed(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	wh := "ms_" + storeA()
	items := []M{
		{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 156.5217, "warehouseId": wh},
		{"productId": fx.ProductA1.Hex(), "qty": 1, "unitPrice": 130.4348, "warehouseId": wh},
	}
	for _, c := range []struct {
		name     string
		rounding float64
		paid     float64
	}{
		{"no rounding", 0, 330.00},
		{"client rounding kept", -0.05, 329.95},
	} {
		r := call(t, "POST", "/sales", tok, M{"storeId": storeA(), "date": "2026-10-07T10:00", "items": items,
			"roundingAuto": false, "rounding": c.rounding, "vatPercent": 15,
			"payments": []M{{"date": "2026-10-07T10:00", "amount": c.paid, "method": "cash"}}})
		if r.Code != 201 {
			t.Fatalf("%s: sale paid in full rejected: %d %s", c.name, r.Code, r.Raw)
		}
		if got := num(r.Body["rounding"]); math.Abs(got-c.rounding) > 1e-9 {
			t.Errorf("%s: rounding read back %.2f, want %.2f", c.name, got, c.rounding)
		}
		doc := rawDoc(t, storeA(), "order", str(r.Body["id"]))
		if got := num(doc["net_total"]); math.Abs(got-c.paid) > 1e-9 {
			t.Errorf("%s: legacy net_total %.2f, want %.2f", c.name, got, c.paid)
		}
		if got := num(doc["total_payment_received"]); got != 0 && math.Abs(got-c.paid) > 1e-9 {
			t.Errorf("%s: legacy payment %.2f, want %.2f", c.name, got, c.paid)
		}
	}
}
