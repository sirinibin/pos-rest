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

// ── FindNetTotal ──────────────────────────────────────────────────────────────

func TestQuotation_FindNetTotal_VATAlwaysCalculated(t *testing.T) {
	// Regression: type=invoice must NOT zero VAT — HideQuotationInvoiceVAT logic removed
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(1, 100.00, 0)}, 15, 0, 0)
	q.Type = "invoice"
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00 (VAT must not be zeroed for type=invoice)", q.VatPrice)
	}
	if q.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", q.NetTotal)
	}
}

func TestQuotation_FindNetTotal_ZeroVAT(t *testing.T) {
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(2, 50.00, 0)}, 0, 0, 0)
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

func TestQuotation_FindNetTotal_WithShipping(t *testing.T) {
	// Total=100, shipping=20, discount=0, vat=10% → base=120, vat=12, net=132
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(1, 100.00, 0)}, 10, 0, 20)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 12.00 {
		t.Errorf("VatPrice = %v, want 12.00 (VAT on total+shipping)", q.VatPrice)
	}
	if q.NetTotal != 132.00 {
		t.Errorf("NetTotal = %v, want 132.00", q.NetTotal)
	}
}

func TestQuotation_FindNetTotal_WithDocumentDiscount(t *testing.T) {
	// Total=200, discount=50, shipping=0, vat=15% → base=150, vat=22.50, net=172.50
	products := []QuotationProduct{
		makeQuotationProduct(2, 100.00, 0),
	}
	q := makeQuotation(products, 15, 50, 0)
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 22.50 {
		t.Errorf("VatPrice = %v, want 22.50 (VAT after discount)", q.VatPrice)
	}
	if q.NetTotal != 172.50 {
		t.Errorf("NetTotal = %v, want 172.50", q.NetTotal)
	}
}

func TestQuotation_FindNetTotal_ShippingAndDiscount(t *testing.T) {
	// Total=100, shipping=30, discount=20, vat=10% → base=110, vat=11, net=121
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(1, 100.00, 0)}, 10, 20, 30)
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

func TestQuotation_FindNetTotal_TypeQuotation_VATCalculated(t *testing.T) {
	// type=quotation (default) must also have VAT calculated
	q := makeQuotation([]QuotationProduct{makeQuotationProduct(1, 200.00, 0)}, 5, 0, 0)
	q.Type = "quotation"
	if err := q.FindNetTotal(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.VatPrice != 10.00 {
		t.Errorf("VatPrice = %v, want 10.00", q.VatPrice)
	}
	if q.NetTotal != 210.00 {
		t.Errorf("NetTotal = %v, want 210.00", q.NetTotal)
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
