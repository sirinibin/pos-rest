package models

import (
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeSRProduct(qty, price, discount float64, selected bool) SalesReturnProduct {
	return SalesReturnProduct{
		Quantity:    qty,
		UnitPrice:   price,
		UnitDiscount: discount,
		Selected:    selected,
	}
}

func makeSalesReturn(products []SalesReturnProduct, vatPct float64, discount, shipping float64) SalesReturn {
	v := vatPct
	return SalesReturn{
		Products:               products,
		VatPercent:             &v,
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

// ── FindTotal — only Selected products are counted ────────────────────────────

func TestSalesReturn_FindTotal_OnlySelectedProducts(t *testing.T) {
	products := []SalesReturnProduct{
		makeSRProduct(1, 50.00, 0, true),  // 50.00 — included
		makeSRProduct(2, 30.00, 0, false), // 60.00 — skipped
		makeSRProduct(1, 20.00, 0, true),  // 20.00 — included
	}
	sr := makeSalesReturn(products, 15, 0, 0)
	sr.FindTotal()
	if sr.Total != 70.00 {
		t.Errorf("Total = %v, want 70.00 (only selected products)", sr.Total)
	}
}

func TestSalesReturn_FindTotal_NoneSelected_TotalIsZero(t *testing.T) {
	products := []SalesReturnProduct{
		makeSRProduct(1, 50.00, 0, false),
		makeSRProduct(2, 30.00, 0, false),
	}
	sr := makeSalesReturn(products, 15, 0, 0)
	sr.FindTotal()
	if sr.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00 (no selected products)", sr.Total)
	}
}

func TestSalesReturn_FindTotal_AllSelected(t *testing.T) {
	products := []SalesReturnProduct{
		makeSRProduct(2, 50.00, 0, true), // 100.00
		makeSRProduct(1, 30.00, 0, true), // 30.00
	}
	sr := makeSalesReturn(products, 15, 0, 0)
	sr.FindTotal()
	if sr.Total != 130.00 {
		t.Errorf("Total = %v, want 130.00", sr.Total)
	}
}

func TestSalesReturn_FindTotal_WithDiscount_OnlySelected(t *testing.T) {
	products := []SalesReturnProduct{
		makeSRProduct(1, 100.00, 10.00, true),  // 90.00
		makeSRProduct(1, 100.00, 10.00, false), // skipped
	}
	sr := makeSalesReturn(products, 15, 0, 0)
	sr.FindTotal()
	if sr.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", sr.Total)
	}
}

func TestSalesReturn_FindTotal_EmptyProducts(t *testing.T) {
	sr := makeSalesReturn([]SalesReturnProduct{}, 15, 0, 0)
	sr.FindTotal()
	if sr.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", sr.Total)
	}
}

// ── FindNetTotal ──────────────────────────────────────────────────────────────

func TestSalesReturn_FindNetTotal_BasicVAT15(t *testing.T) {
	sr := makeSalesReturn([]SalesReturnProduct{makeSRProduct(1, 100.00, 0, true)}, 15, 0, 0)
	sr.FindNetTotal()

	if sr.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", sr.Total)
	}
	if sr.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00", sr.VatPrice)
	}
	if sr.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", sr.NetTotal)
	}
}

func TestSalesReturn_FindNetTotal_WithOrderDiscount(t *testing.T) {
	// Total=100, Discount=10, VAT=15% → base=90, VatPrice=13.50, NetTotal=103.50
	sr := makeSalesReturn([]SalesReturnProduct{makeSRProduct(1, 100.00, 0, true)}, 15, 10.00, 0)
	sr.FindNetTotal()

	if sr.VatPrice != 13.50 {
		t.Errorf("VatPrice = %v, want 13.50", sr.VatPrice)
	}
	if sr.NetTotal != 103.50 {
		t.Errorf("NetTotal = %v, want 103.50", sr.NetTotal)
	}
}

func TestSalesReturn_FindNetTotal_WithShippingFees(t *testing.T) {
	// Total=100, Shipping=20, VAT=15% → base=120, VatPrice=18, NetTotal=138
	sr := makeSalesReturn([]SalesReturnProduct{makeSRProduct(1, 100.00, 0, true)}, 15, 0, 20.00)
	sr.FindNetTotal()

	if sr.VatPrice != 18.00 {
		t.Errorf("VatPrice = %v, want 18.00", sr.VatPrice)
	}
	if sr.NetTotal != 138.00 {
		t.Errorf("NetTotal = %v, want 138.00", sr.NetTotal)
	}
}

func TestSalesReturn_FindNetTotal_ZeroVAT(t *testing.T) {
	sr := makeSalesReturn([]SalesReturnProduct{makeSRProduct(1, 100.00, 0, true)}, 0, 0, 0)
	sr.FindNetTotal()

	if sr.VatPrice != 0.00 {
		t.Errorf("VatPrice = %v, want 0.00", sr.VatPrice)
	}
	if sr.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", sr.NetTotal)
	}
}

func TestSalesReturn_FindNetTotal_UnselectedProductsExcluded(t *testing.T) {
	// Only selected=true product (100) counts; unselected (200) is ignored
	products := []SalesReturnProduct{
		makeSRProduct(1, 100.00, 0, true),
		makeSRProduct(1, 200.00, 0, false),
	}
	sr := makeSalesReturn(products, 15, 0, 0)
	sr.FindNetTotal()

	if sr.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00 (unselected product excluded)", sr.NetTotal)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestSalesReturn_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	sr := SalesReturn{Discount: 0, NetTotal: 115.00}
	sr.CalculateDiscountPercentage()
	if sr.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", sr.DiscountPercent)
	}
}

func TestSalesReturn_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	sr := SalesReturn{Discount: 10.00, NetTotal: 103.50}
	sr.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if sr.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", sr.DiscountPercent, want)
	}
}

func TestSalesReturn_CalculateDiscountPercentage_ZeroBase(t *testing.T) {
	sr := SalesReturn{Discount: 10.00, NetTotal: -10.00}
	sr.CalculateDiscountPercentage()
	if sr.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (zero-base guard)", sr.DiscountPercent)
	}
}

// ── FindTotalQuantity — only Selected products counted ────────────────────────

func TestSalesReturn_FindTotalQuantity_OnlySelectedProducts(t *testing.T) {
	products := []SalesReturnProduct{
		makeSRProduct(3, 10, 0, true),
		makeSRProduct(5, 20, 0, false), // skipped
		makeSRProduct(2, 30, 0, true),
	}
	sr := SalesReturn{Products: products}
	sr.FindTotalQuantity()
	if sr.TotalQuantity != 5 {
		t.Errorf("TotalQuantity = %v, want 5 (only selected)", sr.TotalQuantity)
	}
}

func TestSalesReturn_FindTotalQuantity_NoneSelected(t *testing.T) {
	products := []SalesReturnProduct{
		makeSRProduct(3, 10, 0, false),
		makeSRProduct(5, 20, 0, false),
	}
	sr := SalesReturn{Products: products}
	sr.FindTotalQuantity()
	if sr.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", sr.TotalQuantity)
	}
}

func TestSalesReturn_FindTotalQuantity_EmptyProducts(t *testing.T) {
	sr := SalesReturn{Products: []SalesReturnProduct{}}
	sr.FindTotalQuantity()
	if sr.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", sr.TotalQuantity)
	}
}
