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

// ── FindNetTotal ──────────────────────────────────────────────────────────────

func makeQSR(products []QuotationSalesReturnProduct, vatPct float64, discount, shipping float64) QuotationSalesReturn {
	v := vatPct
	return QuotationSalesReturn{
		Products:               products,
		VatPercent:             &v,
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

func TestQSR_FindNetTotal_VATAlwaysCalculated(t *testing.T) {
	// Regression: returning a type=invoice quotation must include VAT — HideQuotationInvoiceVAT removed
	q := makeQSR([]QuotationSalesReturnProduct{makeQSRProduct(1, 100.00, 0, true)}, 15, 0, 0)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00 (VAT must not be zeroed)", q.VatPrice)
	}
	if q.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", q.NetTotal)
	}
}

func TestQSR_FindNetTotal_ZeroVAT(t *testing.T) {
	q := makeQSR([]QuotationSalesReturnProduct{makeQSRProduct(2, 50.00, 0, true)}, 0, 0, 0)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 0.00 {
		t.Errorf("VatPrice = %v, want 0.00", q.VatPrice)
	}
	if q.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", q.NetTotal)
	}
}

func TestQSR_FindNetTotal_WithShipping(t *testing.T) {
	// Total=100, shipping=20, discount=0, vat=10% → base=120, vat=12, net=132
	q := makeQSR([]QuotationSalesReturnProduct{makeQSRProduct(1, 100.00, 0, true)}, 10, 0, 20)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 12.00 {
		t.Errorf("VatPrice = %v, want 12.00", q.VatPrice)
	}
	if q.NetTotal != 132.00 {
		t.Errorf("NetTotal = %v, want 132.00", q.NetTotal)
	}
}

func TestQSR_FindNetTotal_WithDocumentDiscount(t *testing.T) {
	// Total=200, discount=50, shipping=0, vat=15% → base=150, vat=22.50, net=172.50
	q := makeQSR([]QuotationSalesReturnProduct{makeQSRProduct(2, 100.00, 0, true)}, 15, 50, 0)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 22.50 {
		t.Errorf("VatPrice = %v, want 22.50", q.VatPrice)
	}
	if q.NetTotal != 172.50 {
		t.Errorf("NetTotal = %v, want 172.50", q.NetTotal)
	}
}

func TestQSR_FindNetTotal_SkipsUnselectedProducts(t *testing.T) {
	// Only selected product included in total — VAT applies to that total only
	products := []QuotationSalesReturnProduct{
		makeQSRProduct(1, 100.00, 0, true),
		makeQSRProduct(1, 500.00, 0, false), // must be ignored
	}
	q := makeQSR(products, 10, 0, 0)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 10.00 {
		t.Errorf("VatPrice = %v, want 10.00 (unselected product excluded)", q.VatPrice)
	}
	if q.NetTotal != 110.00 {
		t.Errorf("NetTotal = %v, want 110.00", q.NetTotal)
	}
}

func TestQSR_FindNetTotal_ShippingAndDiscount(t *testing.T) {
	// Total=100, shipping=30, discount=20, vat=10% → base=110, vat=11, net=121
	q := makeQSR([]QuotationSalesReturnProduct{makeQSRProduct(1, 100.00, 0, true)}, 10, 20, 30)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 11.00 {
		t.Errorf("VatPrice = %v, want 11.00", q.VatPrice)
	}
	if q.NetTotal != 121.00 {
		t.Errorf("NetTotal = %v, want 121.00", q.NetTotal)
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
