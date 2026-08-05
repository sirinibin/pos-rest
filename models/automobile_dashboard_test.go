package models

import "testing"

// ── computeNetVAT ─────────────────────────────────────────────────────────────
// Formula: NetVAT = SalesVAT − SalesReturnVAT − PurchaseVAT + PurchaseReturnVAT − ExpenseVAT
//
// Key invariant: ExpenseVAT (input VAT paid to vendors) REDUCES the net liability,
// so it is subtracted.  These tests guard against accidentally reverting to addition.

func TestComputeNetVAT_AllZero(t *testing.T) {
	bd := VATBoxBreakdown{}
	if got := computeNetVAT(bd); got != 0 {
		t.Errorf("all-zero: want 0, got %v", got)
	}
}

func TestComputeNetVAT_OnlySalesVAT(t *testing.T) {
	bd := VATBoxBreakdown{SalesVAT: 100}
	if got := computeNetVAT(bd); got != 100 {
		t.Errorf("only SalesVAT=100: want 100, got %v", got)
	}
}

func TestComputeNetVAT_ExpenseVATReducesLiability(t *testing.T) {
	// Critical: ExpenseVAT must subtract, not add.
	bd := VATBoxBreakdown{SalesVAT: 100, ExpenseVAT: 30}
	want := 70.0
	if got := computeNetVAT(bd); got != want {
		t.Errorf("ExpenseVAT subtracts: want %.2f, got %.2f", want, got)
	}
}

func TestComputeNetVAT_ExpenseVATDoesNotAdd(t *testing.T) {
	// Regression guard: if ExpenseVAT were mistakenly added, result would be 130.
	bd := VATBoxBreakdown{SalesVAT: 100, ExpenseVAT: 30}
	got := computeNetVAT(bd)
	if got == 130 {
		t.Error("ExpenseVAT is being ADDED (bug): result is 130 instead of 70")
	}
}

func TestComputeNetVAT_FullScenario(t *testing.T) {
	// S=1000, SR=50, P=200, PR=20, E=80 → 1000-50-200+20-80 = 690
	bd := VATBoxBreakdown{
		SalesVAT:          1000,
		SalesReturnVAT:    50,
		PurchaseVAT:       200,
		PurchaseReturnVAT: 20,
		ExpenseVAT:        80,
	}
	want := 690.0
	if got := computeNetVAT(bd); got != want {
		t.Errorf("full scenario: want %.2f, got %.2f", want, got)
	}
}

func TestComputeNetVAT_NegativeResult(t *testing.T) {
	// PurchaseVAT > SalesVAT → refund scenario: result is negative.
	bd := VATBoxBreakdown{SalesVAT: 50, PurchaseVAT: 200}
	want := -150.0
	if got := computeNetVAT(bd); got != want {
		t.Errorf("refund scenario: want %.2f, got %.2f", want, got)
	}
}

func TestComputeNetVAT_SalesReturnReducesLiability(t *testing.T) {
	bd := VATBoxBreakdown{SalesVAT: 100, SalesReturnVAT: 25}
	want := 75.0
	if got := computeNetVAT(bd); got != want {
		t.Errorf("SalesReturnVAT subtracts: want %.2f, got %.2f", want, got)
	}
}

func TestComputeNetVAT_PurchaseReturnAddsBack(t *testing.T) {
	// When a purchase is returned, the input VAT credit is lost, so PurchaseReturnVAT is added.
	bd := VATBoxBreakdown{SalesVAT: 100, PurchaseVAT: 40, PurchaseReturnVAT: 10}
	// 100 - 40 + 10 = 70
	want := 70.0
	if got := computeNetVAT(bd); got != want {
		t.Errorf("PurchaseReturnVAT adds back: want %.2f, got %.2f", want, got)
	}
}

// ── roundVATBox ───────────────────────────────────────────────────────────────

func TestRoundVATBox_RoundsAllFields(t *testing.T) {
	bd := VATBoxBreakdown{
		SalesVAT:          100.1234,
		SalesReturnVAT:    10.5678,
		PurchaseVAT:       20.9999,
		PurchaseReturnVAT: 5.0001,
		ExpenseVAT:        3.1415,
		NetVAT:            -99.9999,
	}
	out := roundVATBox(bd)
	cases := []struct {
		name      string
		got, want float64
	}{
		{"SalesVAT", out.SalesVAT, 100.12},
		{"SalesReturnVAT", out.SalesReturnVAT, 10.57},
		{"PurchaseVAT", out.PurchaseVAT, 21.00},
		{"PurchaseReturnVAT", out.PurchaseReturnVAT, 5.00},
		{"ExpenseVAT", out.ExpenseVAT, 3.14},
		{"NetVAT", out.NetVAT, -100.00},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("roundVATBox %s: want %.2f, got %.2f", c.name, c.want, c.got)
		}
	}
}

func TestRoundVATBox_PreservesZero(t *testing.T) {
	out := roundVATBox(VATBoxBreakdown{})
	if out.NetVAT != 0 || out.SalesVAT != 0 {
		t.Errorf("all-zero input should produce all-zero output, got %+v", out)
	}
}

func TestRoundVATBox_NegativeNetVATPreservesSign(t *testing.T) {
	bd := VATBoxBreakdown{NetVAT: -42.5}
	out := roundVATBox(bd)
	if out.NetVAT != -42.50 {
		t.Errorf("negative NetVAT: want -42.50, got %.2f", out.NetVAT)
	}
}
