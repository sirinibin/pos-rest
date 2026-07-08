package models

import (
	"testing"
)

func makeQSRProduct(qty, price, discount float64, selected bool) QuotationSalesReturnProduct {
	return QuotationSalesReturnProduct{
		Quantity:    qty,
		UnitPrice:   price,
		UnitDiscount: discount,
		Selected:    selected,
	}
}

// ── FindTotal — Selected filter + UnitPrice ───────────────────────────────────

func TestQSR_FindTotal_OnlySelectedProducts(t *testing.T) {
	products := []QuotationSalesReturnProduct{
		makeQSRProduct(1, 50.00, 0, true),  // 50.00 included
		makeQSRProduct(2, 30.00, 0, false), // skipped
		makeQSRProduct(1, 20.00, 0, true),  // 20.00 included
	}
	q := QuotationSalesReturn{Products: products}
	q.FindTotal()
	if q.Total != 70.00 {
		t.Errorf("Total = %v, want 70.00 (only selected)", q.Total)
	}
}

func TestQSR_FindTotal_NoneSelected(t *testing.T) {
	products := []QuotationSalesReturnProduct{
		makeQSRProduct(1, 50.00, 0, false),
		makeQSRProduct(2, 30.00, 0, false),
	}
	q := QuotationSalesReturn{Products: products}
	q.FindTotal()
	if q.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00 (none selected)", q.Total)
	}
}

func TestQSR_FindTotal_WithDiscount(t *testing.T) {
	products := []QuotationSalesReturnProduct{
		makeQSRProduct(1, 100.00, 10.00, true), // 90.00
	}
	q := QuotationSalesReturn{Products: products}
	q.FindTotal()
	if q.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", q.Total)
	}
}

func TestQSR_FindTotal_EmptyProducts(t *testing.T) {
	q := QuotationSalesReturn{Products: []QuotationSalesReturnProduct{}}
	q.FindTotal()
	if q.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", q.Total)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestQSR_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	q := QuotationSalesReturn{Discount: 0, NetTotal: 115.00}
	q.CalculateDiscountPercentage()
	if q.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", q.DiscountPercent)
	}
}

func TestQSR_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	q := QuotationSalesReturn{Discount: 10.00, NetTotal: 103.50}
	q.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if q.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", q.DiscountPercent, want)
	}
}

func TestQSR_CalculateDiscountPercentage_ZeroBase(t *testing.T) {
	q := QuotationSalesReturn{Discount: 10.00, NetTotal: -10.00}
	q.CalculateDiscountPercentage()
	if q.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (zero-base guard)", q.DiscountPercent)
	}
}

// ── FindTotalQuantity — Selected filter ───────────────────────────────────────

func TestQSR_FindTotalQuantity_OnlySelected(t *testing.T) {
	products := []QuotationSalesReturnProduct{
		makeQSRProduct(3, 10, 0, true),
		makeQSRProduct(5, 20, 0, false), // skipped
		makeQSRProduct(2, 30, 0, true),
	}
	q := QuotationSalesReturn{Products: products}
	q.FindTotalQuantity()
	if q.TotalQuantity != 5 {
		t.Errorf("TotalQuantity = %v, want 5 (only selected)", q.TotalQuantity)
	}
}

func TestQSR_FindTotalQuantity_NoneSelected(t *testing.T) {
	products := []QuotationSalesReturnProduct{
		makeQSRProduct(3, 10, 0, false),
		makeQSRProduct(5, 20, 0, false),
	}
	q := QuotationSalesReturn{Products: products}
	q.FindTotalQuantity()
	if q.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", q.TotalQuantity)
	}
}

func TestQSR_FindTotalQuantity_Empty(t *testing.T) {
	q := QuotationSalesReturn{Products: []QuotationSalesReturnProduct{}}
	q.FindTotalQuantity()
	if q.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", q.TotalQuantity)
	}
}
