package models

import (
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makePurchaseProduct(qty, purchasePrice, discount float64) PurchaseProduct {
	return PurchaseProduct{
		Quantity:          qty,
		PurchaseUnitPrice: purchasePrice,
		UnitDiscount:      discount,
	}
}

func makePurchase(products []PurchaseProduct, vatPct float64, discount, shipping float64) Purchase {
	v := vatPct
	return Purchase{
		Products:               products,
		VatPercent:             &v,
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

// ── FindTotal — uses PurchaseUnitPrice, not UnitPrice ─────────────────────────

func TestPurchase_FindTotal_SingleProduct_NoDiscount(t *testing.T) {
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(2, 50.00, 0)}, 15, 0, 0)
	p.FindTotal()
	if p.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", p.Total)
	}
}

func TestPurchase_FindTotal_SingleProduct_WithDiscount(t *testing.T) {
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(1, 100.00, 10.00)}, 15, 0, 0)
	p.FindTotal()
	if p.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", p.Total)
	}
}

func TestPurchase_FindTotal_MultipleProducts(t *testing.T) {
	products := []PurchaseProduct{
		makePurchaseProduct(1, 50.00, 0),    // 50.00
		makePurchaseProduct(2, 25.00, 5.00), // 2*(25-5)=40.00
		makePurchaseProduct(3, 10.00, 0),    // 30.00
	}
	p := makePurchase(products, 15, 0, 0)
	p.FindTotal()
	if p.Total != 120.00 {
		t.Errorf("Total = %v, want 120.00", p.Total)
	}
}

func TestPurchase_FindTotal_EmptyProducts(t *testing.T) {
	p := makePurchase([]PurchaseProduct{}, 15, 0, 0)
	p.FindTotal()
	if p.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", p.Total)
	}
}

func TestPurchase_FindTotal_ZeroQuantity(t *testing.T) {
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(0, 100.00, 0)}, 15, 0, 0)
	p.FindTotal()
	if p.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", p.Total)
	}
}

// ── FindNetTotal ──────────────────────────────────────────────────────────────

func TestPurchase_FindNetTotal_BasicVAT15(t *testing.T) {
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(1, 100.00, 0)}, 15, 0, 0)
	p.FindNetTotal()

	if p.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", p.Total)
	}
	if p.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00", p.VatPrice)
	}
	if p.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", p.NetTotal)
	}
}

func TestPurchase_FindNetTotal_WithOrderDiscount(t *testing.T) {
	// Total=100, Discount=10, VAT=15% → base=90, VatPrice=13.50, NetTotal=103.50
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(1, 100.00, 0)}, 15, 10.00, 0)
	p.FindNetTotal()

	if p.VatPrice != 13.50 {
		t.Errorf("VatPrice = %v, want 13.50", p.VatPrice)
	}
	if p.NetTotal != 103.50 {
		t.Errorf("NetTotal = %v, want 103.50", p.NetTotal)
	}
}

func TestPurchase_FindNetTotal_WithShippingFees(t *testing.T) {
	// Total=100, Shipping=20, VAT=15% → base=120, VatPrice=18, NetTotal=138
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(1, 100.00, 0)}, 15, 0, 20.00)
	p.FindNetTotal()

	if p.VatPrice != 18.00 {
		t.Errorf("VatPrice = %v, want 18.00", p.VatPrice)
	}
	if p.NetTotal != 138.00 {
		t.Errorf("NetTotal = %v, want 138.00", p.NetTotal)
	}
}

func TestPurchase_FindNetTotal_ZeroVAT(t *testing.T) {
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(1, 100.00, 0)}, 0, 0, 0)
	p.FindNetTotal()

	if p.VatPrice != 0.00 {
		t.Errorf("VatPrice = %v, want 0.00", p.VatPrice)
	}
	if p.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", p.NetTotal)
	}
}

func TestPurchase_FindNetTotal_WithDiscountAndShipping(t *testing.T) {
	// Total=100, Discount=10, Shipping=20, VAT=15% → base=110, VatPrice=16.50, NetTotal=126.50
	p := makePurchase([]PurchaseProduct{makePurchaseProduct(1, 100.00, 0)}, 15, 10.00, 20.00)
	p.FindNetTotal()

	if p.VatPrice != 16.50 {
		t.Errorf("VatPrice = %v, want 16.50", p.VatPrice)
	}
	if p.NetTotal != 126.50 {
		t.Errorf("NetTotal = %v, want 126.50", p.NetTotal)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestPurchase_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	p := Purchase{Discount: 0, NetTotal: 115.00}
	p.CalculateDiscountPercentage()
	if p.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", p.DiscountPercent)
	}
}

func TestPurchase_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	p := Purchase{Discount: 10.00, NetTotal: 103.50}
	p.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if p.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", p.DiscountPercent, want)
	}
}

func TestPurchase_CalculateDiscountPercentage_ZeroBase(t *testing.T) {
	p := Purchase{Discount: 10.00, NetTotal: -10.00}
	p.CalculateDiscountPercentage()
	if p.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00 (zero-base guard)", p.DiscountPercent)
	}
}

// ── FindTotalQuantity ─────────────────────────────────────────────────────────

func TestPurchase_FindTotalQuantity_SumAllProducts(t *testing.T) {
	products := []PurchaseProduct{
		makePurchaseProduct(3, 10, 0),
		makePurchaseProduct(5, 20, 0),
		makePurchaseProduct(2, 30, 0),
	}
	p := Purchase{Products: products}
	p.FindTotalQuantity()
	if p.TotalQuantity != 10 {
		t.Errorf("TotalQuantity = %v, want 10", p.TotalQuantity)
	}
}

func TestPurchase_FindTotalQuantity_EmptyProducts(t *testing.T) {
	p := Purchase{Products: []PurchaseProduct{}}
	p.FindTotalQuantity()
	if p.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", p.TotalQuantity)
	}
}
