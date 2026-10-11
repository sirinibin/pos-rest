package models

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NEW-nonvat-overreturn: per product, returned ≤ sold − returned by the sale's other returns.
func TestNonVATReturnLineErrors(t *testing.T) {
	a, b, x := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	sold := map[primitive.ObjectID]float64{a: 4, b: 1}
	returned := map[primitive.ObjectID]float64{a: 1}
	ln := func(p primitive.ObjectID, q float64, sel bool) QuotationSalesReturnProduct {
		return QuotationSalesReturnProduct{ProductID: p, Quantity: q, Selected: sel}
	}
	for _, c := range []struct {
		name  string
		lines []QuotationSalesReturnProduct
		want  []string
	}{
		{"the rest", []QuotationSalesReturnProduct{ln(a, 3, true), ln(b, 1, true)}, nil},
		{"over the rest", []QuotationSalesReturnProduct{ln(a, 5, true)}, []string{"quantity_0"}},
		{"over over two lines", []QuotationSalesReturnProduct{ln(a, 2, true), ln(a, 2, true)}, []string{"quantity_1"}},
		{"unselected lines do not count", []QuotationSalesReturnProduct{ln(a, 9, false), ln(a, 3, true)}, nil},
		{"not on the sale", []QuotationSalesReturnProduct{ln(x, 1, true)}, []string{"product_id_0"}},
	} {
		errs := map[string]string{}
		nonVATReturnLineErrors(errs, c.lines, sold, returned)
		if len(errs) != len(c.want) {
			t.Errorf("%s: errors %v, want %v", c.name, errs, c.want)
			continue
		}
		for _, k := range c.want {
			if errs[k] == "" {
				t.Errorf("%s: missing %s in %v", c.name, k, errs)
			}
		}
	}
}

// NEW-sr-foreign-line, NEW-sr-price-above-sale, NEW-sr-update-panic.
func TestSalesReturnLineCheck(t *testing.T) {
	a, b, x := primitive.NewObjectID(), primitive.NewObjectID(), primitive.NewObjectID()
	order := &Order{Code: "S-1", Products: []OrderProduct{
		{ProductID: a, Quantity: 3, QuantityReturned: 1, UnitPrice: 10, UnitDiscount: 1, Unit: "pcs"},
		{ProductID: b, Quantity: 2, UnitPrice: 7},
	}}
	ln := func(p primitive.ObjectID, q, price, disc float64) SalesReturnProduct {
		return SalesReturnProduct{ProductID: p, Quantity: q, UnitPrice: price, UnitDiscount: disc, Selected: true}
	}
	cases := []struct {
		name     string
		lines    []SalesReturnProduct
		scenario string
		old      []SalesReturnProduct
		want     map[string]bool // error keys expected (any line)
	}{
		{"fine", []SalesReturnProduct{ln(a, 2, 10, 1), ln(b, 2, 7, 0)}, "create", nil, nil},
		{"not on the sale", []SalesReturnProduct{ln(x, 1, 1, 0)}, "create", nil, map[string]bool{"product_id_0": true}},
		{"too many", []SalesReturnProduct{ln(a, 3, 10, 1)}, "create", nil, map[string]bool{"quantity_0": true}},
		{"too many over two lines", []SalesReturnProduct{ln(a, 1, 10, 1), ln(a, 2, 10, 1)}, "create", nil, map[string]bool{"quantity_1": true}},
		{"price above the sold net", []SalesReturnProduct{ln(a, 1, 10, 0)}, "create", nil, map[string]bool{"unit_price_0": true}},
		{"price below is fine", []SalesReturnProduct{ln(a, 1, 8, 0)}, "create", nil, nil},
		{"VAT-inclusive rounding is fine", []SalesReturnProduct{ln(b, 1, 7.0043, 0)}, "create", nil, nil},
		// an edit gives back what the return held: 1 returned (by this return) → up to 3 again
		{"edit adds a line", []SalesReturnProduct{ln(a, 3, 10, 1), ln(b, 1, 7, 0)}, "update", []SalesReturnProduct{ln(a, 1, 10, 1)}, nil},
		{"edit adds too much", []SalesReturnProduct{ln(a, 1, 10, 1), ln(b, 3, 7, 0)}, "update", []SalesReturnProduct{ln(a, 1, 10, 1)}, map[string]bool{"quantity_1": true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := map[string]string{}
			var old *SalesReturn
			if c.old != nil {
				old = &SalesReturn{Products: c.old}
			}
			for i, l := range c.lines {
				salesReturnLineCheck(errs, i, l, c.lines, order, c.scenario, old)
			}
			for k := range errs {
				if !c.want[k] {
					t.Errorf("unexpected error %s: %s", k, errs[k])
				}
			}
			for k := range c.want {
				if _, ok := errs[k]; !ok {
					t.Errorf("missing error %s (got %v)", k, errs)
				}
			}
		})
	}
}
