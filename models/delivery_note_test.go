package models

import (
	"testing"
)

func makeDNProduct(qty, price, discount float64) DeliveryNoteProduct {
	return DeliveryNoteProduct{
		Quantity:    qty,
		UnitPrice:   price,
		UnitDiscount: discount,
	}
}

func makeDeliveryNote(products []DeliveryNoteProduct, vatPct *float64, discount, shipping float64) DeliveryNote {
	return DeliveryNote{
		Products:               products,
		VatPercent:             vatPct,
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

func dnVAT(v float64) *float64 { return &v }

// ── FindTotal ─────────────────────────────────────────────────────────────────

func TestDeliveryNote_FindTotal_SingleProduct(t *testing.T) {
	dn := makeDeliveryNote([]DeliveryNoteProduct{makeDNProduct(2, 50.00, 0)}, dnVAT(15), 0, 0)
	dn.FindTotal()
	if dn.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", dn.Total)
	}
}

func TestDeliveryNote_FindTotal_WithDiscount(t *testing.T) {
	dn := makeDeliveryNote([]DeliveryNoteProduct{makeDNProduct(1, 100.00, 10.00)}, dnVAT(15), 0, 0)
	dn.FindTotal()
	if dn.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", dn.Total)
	}
}

func TestDeliveryNote_FindTotal_EmptyProducts(t *testing.T) {
	dn := makeDeliveryNote([]DeliveryNoteProduct{}, dnVAT(15), 0, 0)
	dn.FindTotal()
	if dn.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", dn.Total)
	}
}

// ── FindNetTotal — key difference: nil VatPercent auto-initialises to 0 ───────

func TestDeliveryNote_FindNetTotal_BasicVAT15(t *testing.T) {
	dn := makeDeliveryNote([]DeliveryNoteProduct{makeDNProduct(1, 100.00, 0)}, dnVAT(15), 0, 0)
	dn.FindNetTotal()
	if dn.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00", dn.VatPrice)
	}
	if dn.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", dn.NetTotal)
	}
}

func TestDeliveryNote_FindNetTotal_NilVatPercent_DefaultsToZero(t *testing.T) {
	// DeliveryNote.FindNetTotal() auto-initialises nil VatPercent to 0 — unlike other
	// models which would panic on nil pointer dereference.
	dn := makeDeliveryNote([]DeliveryNoteProduct{makeDNProduct(1, 100.00, 0)}, nil, 0, 0)
	dn.FindNetTotal() // must not panic
	if dn.VatPrice != 0.00 {
		t.Errorf("VatPrice = %v, want 0.00 (nil VatPercent → 0%%)", dn.VatPrice)
	}
	if dn.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", dn.NetTotal)
	}
}

func TestDeliveryNote_FindNetTotal_WithDiscount(t *testing.T) {
	dn := makeDeliveryNote([]DeliveryNoteProduct{makeDNProduct(1, 100.00, 0)}, dnVAT(15), 10.00, 0)
	dn.FindNetTotal()
	if dn.VatPrice != 13.50 {
		t.Errorf("VatPrice = %v, want 13.50", dn.VatPrice)
	}
	if dn.NetTotal != 103.50 {
		t.Errorf("NetTotal = %v, want 103.50", dn.NetTotal)
	}
}

func TestDeliveryNote_FindNetTotal_WithShipping(t *testing.T) {
	dn := makeDeliveryNote([]DeliveryNoteProduct{makeDNProduct(1, 100.00, 0)}, dnVAT(15), 0, 20.00)
	dn.FindNetTotal()
	if dn.NetTotal != 138.00 {
		t.Errorf("NetTotal = %v, want 138.00", dn.NetTotal)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestDeliveryNote_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	dn := DeliveryNote{Discount: 0, NetTotal: 115.00}
	dn.CalculateDiscountPercentage()
	if dn.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", dn.DiscountPercent)
	}
}

func TestDeliveryNote_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	dn := DeliveryNote{Discount: 10.00, NetTotal: 103.50}
	dn.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if dn.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", dn.DiscountPercent, want)
	}
}
