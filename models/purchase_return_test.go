package models

import (
	"testing"
)

func makePRProduct(qty, purchaseReturnPrice, discount float64, selected bool) PurchaseReturnProduct {
	return PurchaseReturnProduct{
		Quantity:                qty,
		PurchaseReturnUnitPrice: purchaseReturnPrice,
		UnitDiscount:            discount,
		Selected:                selected,
	}
}

func makePurchaseReturn(products []PurchaseReturnProduct, vatPct float64, discount, shipping float64) PurchaseReturn {
	v := vatPct
	return PurchaseReturn{
		Products:               products,
		VatPercent:             &v,
		Discount:               discount,
		ShippingOrHandlingFees: shipping,
		AutoRoundingAmount:     false,
	}
}

// ── FindTotal — uses PurchaseReturnUnitPrice and Selected filter ───────────────

func TestPurchaseReturn_FindTotal_OnlySelectedProducts(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(1, 80.00, 0, true),  // 80.00 included
		makePRProduct(2, 50.00, 0, false), // skipped
		makePRProduct(1, 20.00, 0, true),  // 20.00 included
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotal()
	if pr.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00 (only selected)", pr.Total)
	}
}

func TestPurchaseReturn_FindTotal_NoneSelected(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(1, 80.00, 0, false),
		makePRProduct(2, 50.00, 0, false),
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotal()
	if pr.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00 (none selected)", pr.Total)
	}
}

func TestPurchaseReturn_FindTotal_WithDiscount_Selected(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(1, 100.00, 10.00, true), // 90.00
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotal()
	if pr.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", pr.Total)
	}
}

func TestPurchaseReturn_FindTotal_UsesReturnPrice_NotPurchasePrice(t *testing.T) {
	// PurchaseReturnUnitPrice is the credit price, may differ from original cost
	products := []PurchaseReturnProduct{
		makePRProduct(2, 75.00, 0, true), // 2 * 75 = 150
	}
	pr := makePurchaseReturn(products, 0, 0, 0)
	pr.FindTotal()
	if pr.Total != 150.00 {
		t.Errorf("Total = %v, want 150.00 (uses PurchaseReturnUnitPrice)", pr.Total)
	}
}

// ── FindNetTotal ──────────────────────────────────────────────────────────────

func TestPurchaseReturn_FindNetTotal_BasicVAT15(t *testing.T) {
	pr := makePurchaseReturn([]PurchaseReturnProduct{makePRProduct(1, 100.00, 0, true)}, 15, 0, 0)
	pr.FindNetTotal()
	if pr.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00", pr.VatPrice)
	}
	if pr.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", pr.NetTotal)
	}
}

func TestPurchaseReturn_FindNetTotal_WithDiscount(t *testing.T) {
	pr := makePurchaseReturn([]PurchaseReturnProduct{makePRProduct(1, 100.00, 0, true)}, 15, 10.00, 0)
	pr.FindNetTotal()
	if pr.VatPrice != 13.50 {
		t.Errorf("VatPrice = %v, want 13.50", pr.VatPrice)
	}
	if pr.NetTotal != 103.50 {
		t.Errorf("NetTotal = %v, want 103.50", pr.NetTotal)
	}
}

func TestPurchaseReturn_FindNetTotal_ZeroVAT(t *testing.T) {
	pr := makePurchaseReturn([]PurchaseReturnProduct{makePRProduct(1, 100.00, 0, true)}, 0, 0, 0)
	pr.FindNetTotal()
	if pr.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", pr.NetTotal)
	}
}

// ── CalculateDiscountPercentage ───────────────────────────────────────────────

func TestPurchaseReturn_CalculateDiscountPercentage_ZeroDiscount(t *testing.T) {
	pr := PurchaseReturn{Discount: 0, NetTotal: 115.00}
	pr.CalculateDiscountPercentage()
	if pr.DiscountPercent != 0.00 {
		t.Errorf("DiscountPercent = %v, want 0.00", pr.DiscountPercent)
	}
}

func TestPurchaseReturn_CalculateDiscountPercentage_WithDiscount(t *testing.T) {
	pr := PurchaseReturn{Discount: 10.00, NetTotal: 103.50}
	pr.CalculateDiscountPercentage()
	want := RoundTo2Decimals((10.00 / 113.50) * 100)
	if pr.DiscountPercent != want {
		t.Errorf("DiscountPercent = %v, want %v", pr.DiscountPercent, want)
	}
}

// ── FindTotalQuantity ─────────────────────────────────────────────────────────

func TestPurchaseReturn_FindTotalQuantity_OnlySelectedProducts(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(3, 10, 0, true),  // 3 included
		makePRProduct(2, 10, 0, false), // skipped
		makePRProduct(5, 10, 0, true),  // 5 included
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotalQuantity()
	if pr.TotalQuantity != 8 {
		t.Errorf("TotalQuantity = %v, want 8", pr.TotalQuantity)
	}
}

func TestPurchaseReturn_FindTotalQuantity_NoneSelected(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(3, 10, 0, false),
		makePRProduct(2, 10, 0, false),
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotalQuantity()
	if pr.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0 (none selected)", pr.TotalQuantity)
	}
}

func TestPurchaseReturn_FindTotalQuantity_EmptyProducts(t *testing.T) {
	pr := makePurchaseReturn(nil, 15, 0, 0)
	pr.FindTotalQuantity()
	if pr.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0 (empty)", pr.TotalQuantity)
	}
}

func TestPurchaseReturn_FindTotalQuantity_FractionalQuantities(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(1.5, 10, 0, true),
		makePRProduct(2.5, 10, 0, true),
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotalQuantity()
	if pr.TotalQuantity != 4.0 {
		t.Errorf("TotalQuantity = %v, want 4.0", pr.TotalQuantity)
	}
}

func TestPurchaseReturn_FindTotalQuantity_SingleSelected(t *testing.T) {
	products := []PurchaseReturnProduct{
		makePRProduct(7, 10, 0, true),
	}
	pr := makePurchaseReturn(products, 15, 0, 0)
	pr.FindTotalQuantity()
	if pr.TotalQuantity != 7 {
		t.Errorf("TotalQuantity = %v, want 7", pr.TotalQuantity)
	}
}
