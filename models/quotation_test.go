package models

import (
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeQuotationProduct(qty, price, discount float64) QuotationProduct {
	return QuotationProduct{
		Quantity:    qty,
		UnitPrice:   price,
		UnitDiscount: discount,
	}
}

func makeQuotation(products []QuotationProduct, vatPct float64, discount, shipping float64) Quotation {
	v := vatPct
	return Quotation{
		Products:               products,
		VatPercent:             &v,
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

// ── FindTotal ─────────────────────────────────────────────────────────────────

func TestQuotation_FindTotal_SingleProduct_NoDiscount(t *testing.T) {
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(2, 50.00, 0)}, 15, 0, 0)
	q.FindTotal()
	if q.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", q.Total)
	}
}

func TestQuotation_FindTotal_SingleProduct_WithDiscount(t *testing.T) {
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(1, 100.00, 10.00)}, 15, 0, 0)
	q.FindTotal()
	if q.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", q.Total)
	}
}

func TestQuotation_FindTotal_MultipleProducts(t *testing.T) {
	products := []QuotationProduct{
		makeQuotationProduct(1, 50.00, 0),    // 50.00
		makeQuotationProduct(2, 25.00, 5.00), // 2*(25-5)=40.00
		makeQuotationProduct(3, 10.00, 0),    // 30.00
	}
	q := makeQuotation(products, 15, 0, 0)
	q.FindTotal()
	if q.Total != 120.00 {
		t.Errorf("Total = %v, want 120.00", q.Total)
	}
}

func TestQuotation_FindTotal_EmptyProducts(t *testing.T) {
	q := makeQuotation([]QuotationProduct{}, 15, 0, 0)
	q.FindTotal()
	if q.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", q.Total)
	}
}

func TestQuotation_FindTotal_ZeroQuantity(t *testing.T) {
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(0, 100.00, 0)}, 15, 0, 0)
	q.FindTotal()
	if q.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", q.Total)
	}
}

func TestQuotation_FindTotal_FractionalPrices(t *testing.T) {
	// 3 * 33.333 = 99.999 → rounds to 100.00
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(3, 33.333, 0)}, 15, 0, 0)
	q.FindTotal()
	if q.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", q.Total)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestQuotation_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	q := Quotation{Discount: 0, NetTotal: 115.00}
	q.CalculateDiscountPercentage()
	if q.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", q.DiscountPercent)
	}
}

func TestQuotation_CalculateDiscountPercentage_NegativeDiscount(t *testing.T) {
	q := Quotation{Discount: -5.00, NetTotal: 115.00}
	q.CalculateDiscountPercentage()
	if q.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (negative treated as zero)", q.DiscountPercent)
	}
}

func TestQuotation_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	// NetTotal=103.50, Discount=10 → base=113.50 → percent=8.81
	q := Quotation{Discount: 10.00, NetTotal: 103.50}
	q.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if q.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", q.DiscountPercent, want)
	}
}

func TestQuotation_CalculateDiscountPercentage_ZeroBase(t *testing.T) {
	q := Quotation{Discount: 10.00, NetTotal: -10.00}
	q.CalculateDiscountPercentage()
	if q.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (zero-base guard)", q.DiscountPercent)
	}
}

// ── FindTotalQuantity ─────────────────────────────────────────────────────────

func TestQuotation_FindTotalQuantity_SumAllProducts(t *testing.T) {
	products := []QuotationProduct{
		makeQuotationProduct(3, 10, 0),
		makeQuotationProduct(5, 20, 0),
		makeQuotationProduct(2, 30, 0),
	}
	q := Quotation{Products: products}
	q.FindTotalQuantity()
	if q.TotalQuantity != 10 {
		t.Errorf("TotalQuantity = %v, want 10", q.TotalQuantity)
	}
}

func TestQuotation_FindTotalQuantity_EmptyProducts(t *testing.T) {
	q := Quotation{Products: []QuotationProduct{}}
	q.FindTotalQuantity()
	if q.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", q.TotalQuantity)
	}
}

func TestQuotation_FindTotalQuantity_FractionalQuantity(t *testing.T) {
	products := []QuotationProduct{
		makeQuotationProduct(1.5, 10, 0),
		makeQuotationProduct(2.5, 10, 0),
	}
	q := Quotation{Products: products}
	q.FindTotalQuantity()
	if q.TotalQuantity != 4.0 {
		t.Errorf("TotalQuantity = %v, want 4.0", q.TotalQuantity)
	}
}
