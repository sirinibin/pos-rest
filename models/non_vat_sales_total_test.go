package models

import "testing"

// A non-VAT sale sent with vat_percent 0 and no unit_price_with_vat used to
// total 0, so any payment was refused as "should not exceed 0.00".
func TestNonVATSalesFindTotal(t *testing.T) {
	pct := func(v float64) *float64 { return &v }
	cases := []struct {
		name               string
		vat                *float64
		p                  QuotationProduct
		wantTotal, wantVAT float64
	}{
		{"zero VAT, no price with VAT", pct(0), QuotationProduct{Quantity: 2, UnitPrice: 100}, 200, 200},
		{"zero VAT with a discount", pct(0), QuotationProduct{Quantity: 2, UnitPrice: 100, UnitDiscount: 10}, 180, 180},
		{"15% VAT, no price with VAT", pct(15), QuotationProduct{Quantity: 2, UnitPrice: 100}, 200, 230},
		{"price with VAT given", pct(0), QuotationProduct{Quantity: 1, UnitPrice: 100, UnitPriceWithVAT: 100}, 100, 100},
		{"no VAT percent", nil, QuotationProduct{Quantity: 1, UnitPrice: 100}, 100, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &NonVATSales{VatPercent: c.vat, Products: []QuotationProduct{c.p}}
			s.FindTotal()
			s.FindVatPrice()
			if s.Total != c.wantTotal || s.TotalWithVAT != c.wantVAT {
				t.Fatalf("total %v with VAT %v, want %v and %v", s.Total, s.TotalWithVAT, c.wantTotal, c.wantVAT)
			}
			if c.vat != nil && *c.vat == 0 && s.VatPrice != 0 {
				t.Fatalf("vat_price = %v, want 0", s.VatPrice)
			}
		})
	}
}

func TestNonVATSalesReturnFindTotal_ZeroVAT(t *testing.T) {
	zero := 0.0
	r := &NonVATSalesReturn{VatPercent: &zero, Products: []QuotationSalesReturnProduct{{Quantity: 2, UnitPrice: 100, UnitDiscount: 5}}}
	r.FindTotal()
	if r.Total != 190 || r.TotalWithVAT != 190 {
		t.Fatalf("total %v with VAT %v, want 190 and 190", r.Total, r.TotalWithVAT)
	}
}
