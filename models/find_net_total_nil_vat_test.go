package models

import "testing"

// Calculate-net-total and create handlers call FindNetTotal before Validate,
// so a body without vat_percent used to dereference a nil pointer and kill the
// request. FindNetTotal must leave VAT alone and let Validate report
// "VAT Percentage is required".
func TestFindNetTotal_NilVatPercentDoesNotPanic(t *testing.T) {
	cases := map[string]func(){
		"order":                  func() { (&Order{}).FindNetTotal() },
		"purchase":               func() { (&Purchase{}).FindNetTotal() },
		"purchase return":        func() { (&PurchaseReturn{}).FindNetTotal() },
		"sales return":           func() { (&SalesReturn{}).FindNetTotal() },
		"quotation":              func() { (&Quotation{}).FindNetTotal() },
		"quotation sales return": func() { (&QuotationSalesReturn{}).FindNetTotal() },
		"stock transfer":         func() { (&StockTransfer{}).FindNetTotal() },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("FindNetTotal panicked without vat_percent: %v", r)
				}
			}()
			fn()
		})
	}
}

func TestFindNetTotal_WithVatPercent(t *testing.T) {
	vat := 15.0
	o := &Order{VatPercent: &vat, ShippingOrHandlingFees: 7.77, Discount: 1.11,
		Products: []OrderProduct{{Quantity: 3, UnitPrice: 33.33}}}
	o.FindNetTotal()
	// base 99.99 + 7.77 - 1.11 = 106.65, VAT 16.00
	if o.VatPrice != 16 || o.NetTotal != 122.65 {
		t.Fatalf("order: vat %v net %v, want 16 and 122.65", o.VatPrice, o.NetTotal)
	}

	p := &Purchase{VatPercent: &vat, Products: []PurchaseProduct{{Quantity: 10, PurchaseUnitPrice: 60}}}
	p.FindNetTotal()
	if p.VatPrice != 90 || p.NetTotal != 690 {
		t.Fatalf("purchase: vat %v net %v, want 90 and 690", p.VatPrice, p.NetTotal)
	}

	zero := 0.0
	z := &Order{VatPercent: &zero, Products: []OrderProduct{{Quantity: 2, UnitPrice: 50}}}
	z.FindNetTotal()
	if z.VatPrice != 0 || z.NetTotal != 100 {
		t.Fatalf("0%% VAT: vat %v net %v, want 0 and 100", z.VatPrice, z.NetTotal)
	}
}
