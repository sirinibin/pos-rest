package models

import (
	"fmt"
	"strconv"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// salesReturnLineCheck checks one selected line of a sales return against
// the sale it returns:
//   - the product must be on the sale;
//   - the quantity returned of the product, over every line of this return
//     up to this one, must not exceed what was sold minus what other returns
//     already took back (an edit gives back what this return held before);
//   - the refund price per unit (unit price - unit discount) must not exceed
//     the price the product was sold at.
//
// Quantities are summed per product (the sale may carry a product on several
// lines; returned quantities are booked on one of them).
func salesReturnLineCheck(errs map[string]string, index int, line SalesReturnProduct, lines []SalesReturnProduct,
	order *Order, scenario string, oldSalesReturn *SalesReturn) {
	idx := strconv.Itoa(index)
	pid := line.ProductID

	onSale := false
	sold, returned := 0.0, 0.0
	unit := ""
	soldNet, priced := 0.0, false
	for _, op := range order.Products {
		if op.ProductID != pid {
			continue
		}
		onSale = true
		sold += op.Quantity
		returned += op.QuantityReturned
		unit = op.Unit
		// old sales may carry only the VAT-inclusive price: no price check then
		if op.UnitPrice > 0 || op.UnitPriceWithVAT == 0 {
			if n := op.UnitPrice - op.UnitDiscount; !priced || n > soldNet {
				soldNet = n
			}
			priced = true
		}
	}
	if !onSale {
		errs["product_id_"+idx] = "Product is not on the sales invoice " + order.Code
		return
	}

	if scenario == "update" && oldSalesReturn != nil {
		returned -= salesReturnQty(oldSalesReturn.Products, pid, len(oldSalesReturn.Products))
	}
	maxAllowedQuantity := sold - returned - salesReturnQty(lines, pid, index)
	if line.Quantity > maxAllowedQuantity+1e-9 {
		errs["quantity_"+idx] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", maxAllowedQuantity) + " " + unit
	}

	if priced && line.UnitPrice-line.UnitDiscount > soldNet+0.005 {
		errs["unit_price_"+idx] = "Unit price should not be greater than the sold unit price: " + fmt.Sprintf("%.02f", soldNet)
	}
}

// salesReturnQty is the quantity of a product on the selected lines before index.
func salesReturnQty(lines []SalesReturnProduct, pid primitive.ObjectID, before int) float64 {
	q := 0.0
	for i, l := range lines {
		if i >= before {
			break
		}
		if l.Selected && l.ProductID == pid {
			q += l.Quantity
		}
	}
	return q
}
