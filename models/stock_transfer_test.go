package models

import (
	"testing"
)

func makeSTProduct(qty, price, discount float64) StockTransferProduct {
	return StockTransferProduct{
		Quantity:    qty,
		UnitPrice:   price,
		UnitDiscount: discount,
	}
}

func makeStockTransfer(products []StockTransferProduct, vatPct float64) StockTransfer {
	v := vatPct
	return StockTransfer{
		Products:           products,
		VatPercent:         &v,
		AutoRoundingAmount: false,
	}
}

// ── FindTotal — no Selected filter, uses UnitPrice ────────────────────────────

func TestStockTransfer_FindTotal_SingleProduct(t *testing.T) {
	st := makeStockTransfer([]StockTransferProduct{makeSTProduct(2, 50.00, 0)}, 15)
	st.FindTotal()
	if st.Total != 100.00 {
		t.Errorf("Total = %v, want 100.00", st.Total)
	}
}

func TestStockTransfer_FindTotal_MultipleProducts(t *testing.T) {
	products := []StockTransferProduct{
		makeSTProduct(1, 50.00, 0),    // 50.00
		makeSTProduct(2, 25.00, 5.00), // 40.00
	}
	st := makeStockTransfer(products, 15)
	st.FindTotal()
	if st.Total != 90.00 {
		t.Errorf("Total = %v, want 90.00", st.Total)
	}
}

func TestStockTransfer_FindTotal_EmptyProducts(t *testing.T) {
	st := makeStockTransfer([]StockTransferProduct{}, 15)
	st.FindTotal()
	if st.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", st.Total)
	}
}

// ── FindNetTotal — key difference: no shipping/discount; baseTotal = Total ────

func TestStockTransfer_FindNetTotal_BasicVAT15(t *testing.T) {
	st := makeStockTransfer([]StockTransferProduct{makeSTProduct(1, 100.00, 0)}, 15)
	st.FindNetTotal()
	if st.VatPrice != 15.00 {
		t.Errorf("VatPrice = %v, want 15.00", st.VatPrice)
	}
	if st.NetTotal != 115.00 {
		t.Errorf("NetTotal = %v, want 115.00", st.NetTotal)
	}
}

func TestStockTransfer_FindNetTotal_ZeroVAT(t *testing.T) {
	st := makeStockTransfer([]StockTransferProduct{makeSTProduct(1, 100.00, 0)}, 0)
	st.FindNetTotal()
	if st.VatPrice != 0.00 {
		t.Errorf("VatPrice = %v, want 0.00", st.VatPrice)
	}
	if st.NetTotal != 100.00 {
		t.Errorf("NetTotal = %v, want 100.00", st.NetTotal)
	}
}

func TestStockTransfer_FindNetTotal_NoDiscountOrShipping(t *testing.T) {
	// Even with product-level discount, order-level discount/shipping don't apply
	st := makeStockTransfer([]StockTransferProduct{makeSTProduct(1, 100.00, 10.00)}, 15)
	st.FindNetTotal()
	// Total = 90 (product discount applied in FindTotal)
	// baseTotal = Total = 90 (no order discount)
	// VatPrice = 90 * 0.15 = 13.50; NetTotal = 103.50
	if st.VatPrice != 13.50 {
		t.Errorf("VatPrice = %v, want 13.50", st.VatPrice)
	}
	if st.NetTotal != 103.50 {
		t.Errorf("NetTotal = %v, want 103.50", st.NetTotal)
	}
}

// ── FindTotalQuantity ─────────────────────────────────────────────────────────

func TestStockTransfer_FindTotalQuantity_Sum(t *testing.T) {
	products := []StockTransferProduct{
		makeSTProduct(3, 10, 0),
		makeSTProduct(5, 20, 0),
	}
	st := StockTransfer{Products: products}
	st.FindTotalQuantity()
	if st.TotalQuantity != 8 {
		t.Errorf("TotalQuantity = %v, want 8", st.TotalQuantity)
	}
}

func TestStockTransfer_FindTotalQuantity_Empty(t *testing.T) {
	st := StockTransfer{Products: []StockTransferProduct{}}
	st.FindTotalQuantity()
	if st.TotalQuantity != 0 {
		t.Errorf("TotalQuantity = %v, want 0", st.TotalQuantity)
	}
}
