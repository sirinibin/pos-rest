package models

import (
	"testing"
)

func makePayablePayments(entries []struct{ amount, discount float64; method string }) []PayablePayment {
	payments := make([]PayablePayment, len(entries))
	for i, e := range entries {
		payments[i] = PayablePayment{Amount: e.amount, Discount: e.discount, Method: e.method}
	}
	return payments
}

// ── CustomerWithdrawal.FindNetTotal ───────────────────────────────────────────
// Identical logic to CustomerDeposit but using PayablePayment instead of ReceivablePayment.

func TestCustomerWithdrawal_FindNetTotal_SinglePayment(t *testing.T) {
	cw := CustomerWithdrawal{
		Payments: makePayablePayments([]struct{ amount, discount float64; method string }{
			{500.00, 0, "cash"},
		}),
	}
	cw.FindNetTotal()
	if cw.Total != 500.00 {
		t.Errorf("Total = %v, want 500.00", cw.Total)
	}
	if cw.NetTotal != 500.00 {
		t.Errorf("NetTotal = %v, want 500.00", cw.NetTotal)
	}
}

func TestCustomerWithdrawal_FindNetTotal_MultiplePayments(t *testing.T) {
	cw := CustomerWithdrawal{
		Payments: makePayablePayments([]struct{ amount, discount float64; method string }{
			{400.00, 0, "cash"},
			{600.00, 0, "debit_card"},
		}),
	}
	cw.FindNetTotal()
	if cw.Total != 1000.00 {
		t.Errorf("Total = %v, want 1000.00", cw.Total)
	}
}

func TestCustomerWithdrawal_FindNetTotal_PaymentDiscount(t *testing.T) {
	cw := CustomerWithdrawal{
		Payments: makePayablePayments([]struct{ amount, discount float64; method string }{
			{1000.00, 100.00, "cash"},
		}),
	}
	cw.FindNetTotal()
	if cw.TotalDiscount != 100.00 {
		t.Errorf("TotalDiscount = %v, want 100.00", cw.TotalDiscount)
	}
	if cw.NetTotal != 900.00 {
		t.Errorf("NetTotal = %v, want 900.00", cw.NetTotal)
	}
}

func TestCustomerWithdrawal_FindNetTotal_EmptyPayments(t *testing.T) {
	cw := CustomerWithdrawal{Payments: []PayablePayment{}}
	cw.FindNetTotal()
	if cw.NetTotal != 0.00 {
		t.Errorf("NetTotal = %v, want 0.00", cw.NetTotal)
	}
}

func TestCustomerWithdrawal_FindNetTotal_UniquePaymentMethods(t *testing.T) {
	cw := CustomerWithdrawal{
		Payments: makePayablePayments([]struct{ amount, discount float64; method string }{
			{200.00, 0, "cash"},
			{300.00, 0, "cash"},
			{400.00, 0, "bank_transfer"},
		}),
	}
	cw.FindNetTotal()
	if len(cw.PaymentMethods) != 2 {
		t.Errorf("PaymentMethods len = %v, want 2 (deduplicated)", len(cw.PaymentMethods))
	}
}
