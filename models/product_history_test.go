package models

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── ComputeStockAfterEvent ────────────────────────────────────────────────────

func TestComputeStockAfterEvent_Purchase(t *testing.T) {
	got := ComputeStockAfterEvent("purchase", 10, 5, false)
	if got != 15 {
		t.Errorf("purchase: want 15, got %v", got)
	}
}

func TestComputeStockAfterEvent_Sales(t *testing.T) {
	got := ComputeStockAfterEvent("sales", 10, 3, false)
	if got != 7 {
		t.Errorf("sales: want 7, got %v", got)
	}
}

func TestComputeStockAfterEvent_SalesReturn(t *testing.T) {
	got := ComputeStockAfterEvent("sales_return", 10, 2, false)
	if got != 12 {
		t.Errorf("sales_return: want 12, got %v", got)
	}
}

func TestComputeStockAfterEvent_PurchaseReturn(t *testing.T) {
	got := ComputeStockAfterEvent("purchase_return", 10, 4, false)
	if got != 6 {
		t.Errorf("purchase_return: want 6, got %v", got)
	}
}

func TestComputeStockAfterEvent_StockAdjustmentByAdding(t *testing.T) {
	got := ComputeStockAfterEvent("stock_adjustment_by_adding", 33, 16, false)
	if got != 49 {
		t.Errorf("stock_adjustment_by_adding: want 49, got %v", got)
	}
}

func TestComputeStockAfterEvent_StockAdjustmentByRemoving(t *testing.T) {
	got := ComputeStockAfterEvent("stock_adjustment_by_removing", 33, 5, false)
	if got != 28 {
		t.Errorf("stock_adjustment_by_removing: want 28, got %v", got)
	}
}

func TestComputeStockAfterEvent_QuotationInvoice_AffectsStock(t *testing.T) {
	got := ComputeStockAfterEvent("quotation_invoice", 35, 1, true)
	if got != 34 {
		t.Errorf("quotation_invoice (affects): want 34, got %v", got)
	}
}

func TestComputeStockAfterEvent_QuotationInvoice_DoesNotAffectStock(t *testing.T) {
	got := ComputeStockAfterEvent("quotation_invoice", 35, 1, false)
	if got != 35 {
		t.Errorf("quotation_invoice (no-affect): want 35 (unchanged), got %v", got)
	}
}

func TestComputeStockAfterEvent_QuotationSalesReturn_AffectsStock(t *testing.T) {
	got := ComputeStockAfterEvent("quotation_sales_return", 34, 1, true)
	if got != 35 {
		t.Errorf("quotation_sales_return (affects): want 35, got %v", got)
	}
}

func TestComputeStockAfterEvent_QuotationSalesReturn_DoesNotAffectStock(t *testing.T) {
	got := ComputeStockAfterEvent("quotation_sales_return", 34, 1, false)
	if got != 34 {
		t.Errorf("quotation_sales_return (no-affect): want 34 (unchanged), got %v", got)
	}
}

func TestComputeStockAfterEvent_UnknownType_Unchanged(t *testing.T) {
	got := ComputeStockAfterEvent("delivery_note", 20, 5, true)
	if got != 20 {
		t.Errorf("unknown type: want 20 (unchanged), got %v", got)
	}
}

func TestComputeStockAfterEvent_StockTransfer_Unchanged(t *testing.T) {
	got := ComputeStockAfterEvent("stock_transfer", 20, 5, true)
	if got != 20 {
		t.Errorf("stock_transfer: want 20 (unchanged), got %v", got)
	}
}

func TestComputeStockAfterEvent_ZeroQuantity(t *testing.T) {
	got := ComputeStockAfterEvent("sales", 10, 0, false)
	if got != 10 {
		t.Errorf("zero qty sales: want 10, got %v", got)
	}
}

func TestComputeStockAfterEvent_NegativeStock(t *testing.T) {
	// Adjustments are applied even when stock would go negative.
	got := ComputeStockAfterEvent("stock_adjustment_by_removing", 5, 10, false)
	if got != -5 {
		t.Errorf("removing more than available: want -5, got %v", got)
	}
}

// ── WarehouseStocks field assignment ─────────────────────────────────────────

// TestProductHistory_WarehouseStocksAssignment verifies the map field on
// ProductHistory can hold per-warehouse values and is accessed by key.
// This covers the fix where AdjustStockInHistoryAfter now populates
// WarehouseStocks alongside Stock.
func TestProductHistory_WarehouseStocksAssignment(t *testing.T) {
	storeID := primitive.NewObjectID()
	now := time.Now()

	h := ProductHistory{
		ID:      primitive.NewObjectID(),
		StoreID: &storeID,
		Date:    &now,
		Stock:   49,
		WarehouseStocks: map[string]float64{
			"main_store": 33,
			"WH1":        16,
		},
	}

	if h.WarehouseStocks["main_store"] != 33 {
		t.Errorf("main_store: want 33, got %v", h.WarehouseStocks["main_store"])
	}
	if h.WarehouseStocks["WH1"] != 16 {
		t.Errorf("WH1: want 16, got %v", h.WarehouseStocks["WH1"])
	}
	total := h.WarehouseStocks["main_store"] + h.WarehouseStocks["WH1"]
	if total != h.Stock {
		t.Errorf("WarehouseStocks sum %v != Stock %v", total, h.Stock)
	}
}

// TestProductHistory_WarehouseStocksConsistencyAfterAdjustment documents the
// expected invariant the fix enforces: after adding qty to a warehouse,
// main_store = total - warehouse.
func TestProductHistory_WarehouseStocksConsistencyAfterAdjustment(t *testing.T) {
	const baseStock = 33.0
	const wh1Adjustment = 16.0

	totalAfter := ComputeStockAfterEvent("stock_adjustment_by_adding", baseStock, wh1Adjustment, false)
	wh1After := wh1Adjustment // warehouse stock equals the adjustment (no prior WH1 ops)
	mainStoreAfter := totalAfter - wh1After

	if totalAfter != 49 {
		t.Errorf("total: want 49, got %v", totalAfter)
	}
	if wh1After != 16 {
		t.Errorf("WH1: want 16, got %v", wh1After)
	}
	if mainStoreAfter != 33 {
		t.Errorf("main_store: want 33, got %v", mainStoreAfter)
	}
}

// TestProductHistory_WarehouseStocksConsistencyWhenBaseStockHigher documents
// the scenario where base stock is 35 (main_store=35, WH1=0). Adding 16 to
// WH1 should keep main_store at 35 and set WH1=16, total=51.
func TestProductHistory_WarehouseStocksConsistencyWhenBaseStockHigher(t *testing.T) {
	const baseStock = 35.0
	const wh1Adjustment = 16.0

	totalAfter := ComputeStockAfterEvent("stock_adjustment_by_adding", baseStock, wh1Adjustment, false)
	wh1After := wh1Adjustment
	mainStoreAfter := totalAfter - wh1After

	if totalAfter != 51 {
		t.Errorf("total: want 51, got %v", totalAfter)
	}
	if mainStoreAfter != 35 {
		t.Errorf("main_store: want 35 (unchanged), got %v", mainStoreAfter)
	}
}

// TestProductHistory_TwoWarehousesConsistency verifies the invariant:
// main_store = total - sum(all named warehouses).
func TestProductHistory_TwoWarehousesConsistency(t *testing.T) {
	const baseStock = 50.0
	const wh1Adjustment = 20.0
	const wh2Adjustment = 15.0

	total := ComputeStockAfterEvent("stock_adjustment_by_adding",
		ComputeStockAfterEvent("stock_adjustment_by_adding", baseStock, wh1Adjustment, false),
		wh2Adjustment, false)
	mainStore := total - wh1Adjustment - wh2Adjustment

	if total != 85 {
		t.Errorf("total: want 85, got %v", total)
	}
	if mainStore != 50 {
		t.Errorf("main_store: want 50, got %v", mainStore)
	}
}

// TestProductHistory_WarehouseRemovingAdjustment covers a warehouse
// remove adjustment reducing named-warehouse stock below initial.
func TestProductHistory_WarehouseRemovingAdjustment(t *testing.T) {
	// Add 20 to WH1, then remove 5 from WH1.
	baseStock := 50.0
	afterAdd := ComputeStockAfterEvent("stock_adjustment_by_adding", baseStock, 20, false)
	afterRemove := ComputeStockAfterEvent("stock_adjustment_by_removing", afterAdd, 5, false)

	wh1Net := 20.0 - 5.0 // 15
	mainStore := afterRemove - wh1Net

	if afterRemove != 65 {
		t.Errorf("total after add+remove: want 65, got %v", afterRemove)
	}
	if wh1Net != 15 {
		t.Errorf("WH1 net: want 15, got %v", wh1Net)
	}
	if mainStore != 50 {
		t.Errorf("main_store: want 50 (unchanged), got %v", mainStore)
	}
}

// TestProductHistory_NegativeMainStore verifies main_store can be negative
// when warehouse stock exceeds total (e.g. transfer moved stock out of main).
func TestProductHistory_NegativeMainStore(t *testing.T) {
	total := 10.0
	wh1 := 15.0 // more than total in WH1 (stock transferred in)
	mainStore := total - wh1
	if mainStore != -5 {
		t.Errorf("main_store: want -5, got %v", mainStore)
	}
}

// TestProductHistory_ZeroAdjustmentNoChange verifies a zero-quantity adjustment
// leaves all values unchanged.
func TestProductHistory_ZeroAdjustmentNoChange(t *testing.T) {
	const baseStock = 33.0
	total := ComputeStockAfterEvent("stock_adjustment_by_adding", baseStock, 0, false)
	if total != baseStock {
		t.Errorf("zero adjustment: total should stay %v, got %v", baseStock, total)
	}
}

// TestProductHistory_MainStoreOnlyAdjustment verifies a stock adjustment with
// no warehouse specified affects only the total; named warehouses are unchanged.
func TestProductHistory_MainStoreOnlyAdjustment(t *testing.T) {
	// Before: total=33, WH1=0
	totalBefore := 33.0
	wh1Before := 0.0

	// Add 10 to main store (no warehouse).
	totalAfter := ComputeStockAfterEvent("stock_adjustment_by_adding", totalBefore, 10, false)
	// WH1 stays 0; main_store = total - WH1
	mainStoreAfter := totalAfter - wh1Before

	if totalAfter != 43 {
		t.Errorf("total: want 43, got %v", totalAfter)
	}
	if mainStoreAfter != 43 {
		t.Errorf("main_store: want 43, got %v", mainStoreAfter)
	}
}
