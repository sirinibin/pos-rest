package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// ── Payment Validation Logic (pure, no DB) ────────────────────────────────────
//
// testQSRValidateParams holds all inputs to the four-layer payment validation
// mirrored from quotation_sales_return.go lines 1881-1908.
type testQSRValidateParams struct {
	qsrNetTotal                   float64
	qsrCashDiscount               float64
	totalPayment                  float64
	quotationType                 string
	quotationNetTotal             float64
	quotationTotalPaymentReceived float64
	quotationReturnAmount         float64
	scenario                      string // "create" or "update"
	oldQSRTotalPaymentPaid        float64
}

// testQSRValidateResult holds the resulting error strings (empty = no error).
type testQSRValidateResult struct {
	totalPaymentErr string
	netTotalErr     string
}

// testValidateQSRPayment is a pure re-implementation of the four validation
// layers at lines 1881-1908 of quotation_sales_return.go.  No DB, no HTTP.
//
// Layer order (matches source exactly):
//  1. totalPayment > qsr.NetTotal – qsr.CashDiscount     → "total_payment" (Net Total - Cash Discount)
//  2. totalPayment > quotation.NetTotal                   → "total_payment" (Original NetTotal)
//  3. qsr.NetTotal > quotation.NetTotal                   → "net_total"
//  4. TotalPaymentReceived check — skipped for invoice    → "total_payment" (total payment received)
func testValidateQSRPayment(p testQSRValidateParams) testQSRValidateResult {
	res := testQSRValidateResult{}

	// Layer 1
	if p.totalPayment > (p.qsrNetTotal - p.qsrCashDiscount) {
		res.totalPaymentErr = "Total payment should not exceed: " +
			fmt.Sprintf("%.02f", p.qsrNetTotal-p.qsrCashDiscount) +
			" (Net Total - Cash Discount)"
		return res
	}

	// Layer 2
	if p.totalPayment > p.quotationNetTotal {
		res.totalPaymentErr = "Total payment amount should not exceed Original QuotationSales Net Total: " +
			fmt.Sprintf("%.02f", p.quotationNetTotal)
		return res
	}

	// Layer 3
	if p.qsrNetTotal > p.quotationNetTotal {
		res.netTotalErr = "Net Total  should not exceed Original QuotationSales Net Total: " +
			fmt.Sprintf("%.02f", p.quotationNetTotal)
		return res
	}

	// Layer 4 — skipped entirely when quotation.Type == "invoice"
	if p.quotationType != "invoice" {
		if p.scenario == "update" {
			available := p.quotationTotalPaymentReceived - (p.quotationReturnAmount - p.oldQSRTotalPaymentPaid)
			if p.totalPayment > available {
				res.totalPaymentErr = "Total payment should not be greater than " +
					fmt.Sprintf("%.2f", available) + " (total payment received)"
				return res
			}
		} else {
			available := p.quotationTotalPaymentReceived - p.quotationReturnAmount
			if p.totalPayment > available {
				res.totalPaymentErr = "Total payment should not be greater than " +
					fmt.Sprintf("%.2f", available) + " (total payment received)"
				return res
			}
		}
	}

	return res
}

// ── Test 1: Exact bug scenario ────────────────────────────────────────────────
//
// type=invoice, net_total=149.5, TotalPaymentReceived=130 (partial) — creating a
// QSR with totalPayment=149.5 was producing a 400 error before the fix.

func TestQSR_InvoicePaymentValidation_ExactBugScenario(t *testing.T) {
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   149.5,
		qsrCashDiscount:               0,
		totalPayment:                  149.5,
		quotationType:                 "invoice",
		quotationNetTotal:             149.5,
		quotationTotalPaymentReceived: 130,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if res.totalPaymentErr != "" {
		t.Errorf("bug scenario: expected no total_payment error, got: %q", res.totalPaymentErr)
	}
	if res.netTotalErr != "" {
		t.Errorf("bug scenario: expected no net_total error, got: %q", res.netTotalErr)
	}
}

// ── Tests 2-3: Invoice type — TotalPaymentReceived check skipped (table) ──────

func TestQSR_InvoicePaymentValidation_TotalPaymentReceivedSkipped(t *testing.T) {
	cases := []struct {
		name                          string
		scenario                      string
		totalPayment                  float64
		quotationTotalPaymentReceived float64
		quotationReturnAmount         float64
		oldQSRTotalPaymentPaid        float64
	}{
		// Test 2: create, totalPayment well above TotalPaymentReceived → no error
		{
			name:                          "create_payment_far_exceeds_received",
			scenario:                      "create",
			totalPayment:                  200,
			quotationTotalPaymentReceived: 100,
			quotationReturnAmount:         0,
		},
		// Test 3: update, same exceeding scenario → still no error
		{
			name:                          "update_payment_exceeds_received_with_return",
			scenario:                      "update",
			totalPayment:                  150,
			quotationTotalPaymentReceived: 100,
			quotationReturnAmount:         20,
			oldQSRTotalPaymentPaid:        20,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := testValidateQSRPayment(testQSRValidateParams{
				qsrNetTotal:                   200,
				qsrCashDiscount:               0,
				totalPayment:                  tc.totalPayment,
				quotationType:                 "invoice",
				quotationNetTotal:             200,
				quotationTotalPaymentReceived: tc.quotationTotalPaymentReceived,
				quotationReturnAmount:         tc.quotationReturnAmount,
				scenario:                      tc.scenario,
				oldQSRTotalPaymentPaid:        tc.oldQSRTotalPaymentPaid,
			})
			if res.totalPaymentErr != "" {
				t.Errorf("invoice type: expected no total_payment error, got: %q", res.totalPaymentErr)
			}
		})
	}
}

// ── Tests 4-5: Non-invoice — TotalPaymentReceived check enforced (table) ──────

func TestQSR_NonInvoicePaymentValidation_TotalPaymentReceivedEnforced(t *testing.T) {
	cases := []struct {
		name                          string
		scenario                      string
		totalPayment                  float64
		quotationTotalPaymentReceived float64
		quotationReturnAmount         float64
		oldQSRTotalPaymentPaid        float64
	}{
		// Test 4: create — totalPayment exceeds TotalPaymentReceived
		{
			name:                          "create_payment_exceeds_received",
			scenario:                      "create",
			totalPayment:                  110,
			quotationTotalPaymentReceived: 100,
			quotationReturnAmount:         0,
		},
		// Test 5: update — totalPayment exceeds adjusted available
		// available = TotalPaymentReceived(130) - (ReturnAmount(50) - oldPaid(20)) = 100
		{
			name:                          "update_payment_exceeds_adjusted_available",
			scenario:                      "update",
			totalPayment:                  110,
			quotationTotalPaymentReceived: 130,
			quotationReturnAmount:         50,
			oldQSRTotalPaymentPaid:        20,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := testValidateQSRPayment(testQSRValidateParams{
				qsrNetTotal:                   200,
				qsrCashDiscount:               0,
				totalPayment:                  tc.totalPayment,
				quotationType:                 "quotation",
				quotationNetTotal:             200,
				quotationTotalPaymentReceived: tc.quotationTotalPaymentReceived,
				quotationReturnAmount:         tc.quotationReturnAmount,
				scenario:                      tc.scenario,
				oldQSRTotalPaymentPaid:        tc.oldQSRTotalPaymentPaid,
			})
			if !strings.Contains(res.totalPaymentErr, "total payment received") {
				t.Errorf("expected error containing 'total payment received', got: %q", res.totalPaymentErr)
			}
		})
	}
}

// ── Test 6: Boundary — exactly at the limit (non-invoice) → no error ──────────

func TestQSR_NonInvoicePaymentValidation_ExactlyAtLimit(t *testing.T) {
	// totalPayment == TotalPaymentReceived: condition is strictly >, so no error
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   100,
		qsrCashDiscount:               0,
		totalPayment:                  100,
		quotationType:                 "quotation",
		quotationNetTotal:             100,
		quotationTotalPaymentReceived: 100,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if res.totalPaymentErr != "" {
		t.Errorf("at exact limit: expected no error, got: %q", res.totalPaymentErr)
	}
}

// ── Test 7: Just over the limit (non-invoice) → error ────────────────────────

func TestQSR_NonInvoicePaymentValidation_JustOverLimit(t *testing.T) {
	// totalPayment=100.01, TotalPaymentReceived=100 → triggers layer-4
	// qsrNetTotal and quotationNetTotal set to 200 so layers 1-3 don't fire
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   200,
		qsrCashDiscount:               0,
		totalPayment:                  100.01,
		quotationType:                 "quotation",
		quotationNetTotal:             200,
		quotationTotalPaymentReceived: 100,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if !strings.Contains(res.totalPaymentErr, "total payment received") {
		t.Errorf("just over limit: expected 'total payment received' error, got: %q", res.totalPaymentErr)
	}
}

// ── Test 8: ReturnAmount > 0 reduces available (non-invoice) ─────────────────

func TestQSR_NonInvoicePaymentValidation_ReturnAmountReducesAvailable(t *testing.T) {
	// TotalPaymentReceived=130, ReturnAmount=30 → available=100; totalPayment=101 → error
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   130,
		qsrCashDiscount:               0,
		totalPayment:                  101,
		quotationType:                 "quotation",
		quotationNetTotal:             130,
		quotationTotalPaymentReceived: 130,
		quotationReturnAmount:         30,
		scenario:                      "create",
	})
	if !strings.Contains(res.totalPaymentErr, "total payment received") {
		t.Errorf("return reduces available: expected 'total payment received' error, got: %q", res.totalPaymentErr)
	}
	if !strings.Contains(res.totalPaymentErr, "100.00") {
		t.Errorf("return reduces available: expected available=100.00 in error, got: %q", res.totalPaymentErr)
	}
}

// ── Test 9: TotalPaymentReceived=0, type=quotation → any payment errors ───────

func TestQSR_NonInvoicePaymentValidation_ZeroReceived(t *testing.T) {
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   100,
		qsrCashDiscount:               0,
		totalPayment:                  1,
		quotationType:                 "quotation",
		quotationNetTotal:             100,
		quotationTotalPaymentReceived: 0,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if !strings.Contains(res.totalPaymentErr, "total payment received") {
		t.Errorf("zero received (quotation): expected 'total payment received' error, got: %q", res.totalPaymentErr)
	}
}

// ── Test 10: TotalPaymentReceived=0, type=invoice → no error (check skipped) ──

func TestQSR_InvoicePaymentValidation_ZeroReceived(t *testing.T) {
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   100,
		qsrCashDiscount:               0,
		totalPayment:                  1,
		quotationType:                 "invoice",
		quotationNetTotal:             100,
		quotationTotalPaymentReceived: 0,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if res.totalPaymentErr != "" {
		t.Errorf("zero received (invoice): expected no error (check skipped), got: %q", res.totalPaymentErr)
	}
}

// ── Test 11: Layer 1 fires regardless of invoice type ────────────────────────

func TestQSR_PaymentValidation_Layer1FiresForInvoice(t *testing.T) {
	// qsr.NetTotal=100, CashDiscount=10 → ceiling=90; totalPayment=91 → layer-1 error
	// Even though type=invoice (which would skip layer-4), layer-1 always fires
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   100,
		qsrCashDiscount:               10,
		totalPayment:                  91,
		quotationType:                 "invoice",
		quotationNetTotal:             200,
		quotationTotalPaymentReceived: 50,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if !strings.Contains(res.totalPaymentErr, "Net Total - Cash Discount") {
		t.Errorf("layer-1 invoice: expected 'Net Total - Cash Discount' error, got: %q", res.totalPaymentErr)
	}
}

// ── Test 12: Layer 2 fires before TotalPaymentReceived check (non-invoice) ────

func TestQSR_PaymentValidation_Layer2FiresBeforeReceivedCheck(t *testing.T) {
	// qsrNetTotal=250 so layer-1 passes (91 <= 250); totalPayment=150 > quotation.NetTotal=100
	// → layer-2 error.  TotalPaymentReceived=50 would also fail but layer-2 runs first.
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   250,
		qsrCashDiscount:               0,
		totalPayment:                  150,
		quotationType:                 "quotation",
		quotationNetTotal:             100,
		quotationTotalPaymentReceived: 50,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if !strings.Contains(res.totalPaymentErr, "Original QuotationSales Net Total") {
		t.Errorf("layer-2 fires first: expected 'Original QuotationSales Net Total' error, got: %q", res.totalPaymentErr)
	}
	if strings.Contains(res.totalPaymentErr, "total payment received") {
		t.Errorf("layer-2 fires first: must NOT contain 'total payment received', got: %q", res.totalPaymentErr)
	}
}

// ── Test 13: Zero-VAT invoice scenario ───────────────────────────────────────

func TestQSR_InvoicePaymentValidation_ZeroVATNoError(t *testing.T) {
	// vat_percent=0: qsr.NetTotal == quotation.NetTotal == totalPayment; type=invoice
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   104.35,
		qsrCashDiscount:               0,
		totalPayment:                  104.35,
		quotationType:                 "invoice",
		quotationNetTotal:             104.35,
		quotationTotalPaymentReceived: 104.35,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if res.totalPaymentErr != "" {
		t.Errorf("zero-VAT invoice: expected no total_payment error, got: %q", res.totalPaymentErr)
	}
	if res.netTotalErr != "" {
		t.Errorf("zero-VAT invoice: expected no net_total error, got: %q", res.netTotalErr)
	}
}

// ── Test 14: Old QSR paid amount restores headroom on update (non-invoice) ────

func TestQSR_NonInvoicePaymentValidation_OldQSRRestoresHeadroom(t *testing.T) {
	// TotalPaymentReceived=130, ReturnAmount=50, oldQSR.TotalPaymentPaid=50
	// available = 130 - (50 - 50) = 130; totalPayment=130 is not strictly > 130 → no error
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   130,
		qsrCashDiscount:               0,
		totalPayment:                  130,
		quotationType:                 "quotation",
		quotationNetTotal:             130,
		quotationTotalPaymentReceived: 130,
		quotationReturnAmount:         50,
		scenario:                      "update",
		oldQSRTotalPaymentPaid:        50,
	})
	if res.totalPaymentErr != "" {
		t.Errorf("old QSR restores headroom: expected no error (available=130), got: %q", res.totalPaymentErr)
	}
}

// ── Test 15: Cash discount reduces the effective ceiling (layer-1) ─────────────

func TestQSR_PaymentValidation_CashDiscountReducesCeiling(t *testing.T) {
	// qsr.NetTotal=100, CashDiscount=10 → ceiling=90; totalPayment=91 → layer-1 error
	res := testValidateQSRPayment(testQSRValidateParams{
		qsrNetTotal:                   100,
		qsrCashDiscount:               10,
		totalPayment:                  91,
		quotationType:                 "quotation",
		quotationNetTotal:             200,
		quotationTotalPaymentReceived: 200,
		quotationReturnAmount:         0,
		scenario:                      "create",
	})
	if !strings.Contains(res.totalPaymentErr, "Net Total - Cash Discount") {
		t.Errorf("cash discount ceiling: expected 'Net Total - Cash Discount' error, got: %q", res.totalPaymentErr)
	}
	if !strings.Contains(res.totalPaymentErr, "90.00") {
		t.Errorf("cash discount ceiling: expected 90.00 in error, got: %q", res.totalPaymentErr)
	}
}

// ── Source guard test ─────────────────────────────────────────────────────────
//
// Verifies that the invoice-type guard is still present in the source file.
// If someone accidentally deletes the guard this test fails immediately.

func TestQSR_SourceGuard_InvoiceTypeCheckPresent(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("quotation_sales_return.go"))
	if err != nil {
		t.Fatalf("could not read quotation_sales_return.go: %v", err)
	}
	const guard = `quotation.Type != "invoice"`
	if !strings.Contains(string(src), guard) {
		t.Errorf("source guard missing: %q not found in quotation_sales_return.go — the invoice bypass was deleted", guard)
	}
}
