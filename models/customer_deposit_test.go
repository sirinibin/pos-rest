package models

import (
	"testing"
)

func makePayments(entries []struct{ amount, discount float64; method string }) []ReceivablePayment {
	payments := make([]ReceivablePayment, len(entries))
	for i, e := range entries {
		payments[i] = ReceivablePayment{Amount: e.amount, Discount: e.discount, Method: e.method}
	}
	return payments
}

// ── CustomerDeposit.FindNetTotal ──────────────────────────────────────────────
// Sums payments; Total = sum of amounts, TotalDiscount = sum of payment discounts,
// NetTotal = Total - TotalDiscount. Completely different from invoice calculation.

func TestCustomerDeposit_FindNetTotal_SinglePayment(t *testing.T) {
	cd := CustomerDeposit{
		Payments: makePayments([]struct{ amount, discount float64; method string }{
			{1000.00, 0, "cash"},
		}),
	}
	cd.FindNetTotal()
	if cd.Total != 1000.00 {
		t.Errorf("Total = %v, want 1000.00", cd.Total)
	}
	if cd.NetTotal != 1000.00 {
		t.Errorf("NetTotal = %v, want 1000.00 (no discount)", cd.NetTotal)
	}
}

func TestCustomerDeposit_FindNetTotal_MultiplePayments(t *testing.T) {
	cd := CustomerDeposit{
		Payments: makePayments([]struct{ amount, discount float64; method string }{
			{500.00, 0, "cash"},
			{300.00, 0, "bank_transfer"},
		}),
	}
	cd.FindNetTotal()
	if cd.Total != 800.00 {
		t.Errorf("Total = %v, want 800.00", cd.Total)
	}
	if cd.NetTotal != 800.00 {
		t.Errorf("NetTotal = %v, want 800.00", cd.NetTotal)
	}
}

func TestCustomerDeposit_FindNetTotal_PaymentDiscount(t *testing.T) {
	cd := CustomerDeposit{
		Payments: makePayments([]struct{ amount, discount float64; method string }{
			{1000.00, 50.00, "cash"},
		}),
	}
	cd.FindNetTotal()
	if cd.TotalDiscount != 50.00 {
		t.Errorf("TotalDiscount = %v, want 50.00", cd.TotalDiscount)
	}
	if cd.NetTotal != 950.00 {
		t.Errorf("NetTotal = %v, want 950.00 (1000 - 50 discount)", cd.NetTotal)
	}
}

func TestCustomerDeposit_FindNetTotal_MultipleDiscounts(t *testing.T) {
	cd := CustomerDeposit{
		Payments: makePayments([]struct{ amount, discount float64; method string }{
			{500.00, 25.00, "cash"},
			{300.00, 15.00, "bank_transfer"},
		}),
	}
	cd.FindNetTotal()
	if cd.TotalDiscount != 40.00 {
		t.Errorf("TotalDiscount = %v, want 40.00", cd.TotalDiscount)
	}
	if cd.NetTotal != 760.00 {
		t.Errorf("NetTotal = %v, want 760.00 (800 - 40 discounts)", cd.NetTotal)
	}
}

func TestCustomerDeposit_FindNetTotal_ZeroDiscountIgnored(t *testing.T) {
	// Payment.Discount <= 0 is not added to TotalDiscount
	cd := CustomerDeposit{
		Payments: makePayments([]struct{ amount, discount float64; method string }{
			{1000.00, 0, "cash"},
			{200.00, -5.00, "bank_transfer"}, // negative discount — not added
		}),
	}
	cd.FindNetTotal()
	if cd.TotalDiscount != 0.00 {
		t.Errorf("TotalDiscount = %v, want 0.00 (zero/negative discounts ignored)", cd.TotalDiscount)
	}
}

func TestCustomerDeposit_FindNetTotal_EmptyPayments(t *testing.T) {
	cd := CustomerDeposit{Payments: []ReceivablePayment{}}
	cd.FindNetTotal()
	if cd.Total != 0.00 {
		t.Errorf("Total = %v, want 0.00", cd.Total)
	}
	if cd.NetTotal != 0.00 {
		t.Errorf("NetTotal = %v, want 0.00", cd.NetTotal)
	}
}

func TestCustomerDeposit_FindNetTotal_UniquePaymentMethods(t *testing.T) {
	// Same method used twice — should appear only once in PaymentMethods
	cd := CustomerDeposit{
		Payments: makePayments([]struct{ amount, discount float64; method string }{
			{300.00, 0, "cash"},
			{200.00, 0, "cash"},
			{500.00, 0, "bank_transfer"},
		}),
	}
	cd.FindNetTotal()
	if len(cd.PaymentMethods) != 2 {
		t.Errorf("PaymentMethods len = %v, want 2 (deduplicated)", len(cd.PaymentMethods))
	}
}
