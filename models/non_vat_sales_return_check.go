package models

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// checkReturnedQuantities: a non-VAT return takes back at most what its sale
// sold, less what the sale's other returns already took back (an edit does
// not count the return's own previous lines).
func (ret *NonVATSalesReturn) checkReturnedQuantities(store *Store, errs map[string]string) error {
	if ret.NonVATSalesID == nil || ret.NonVATSalesID.IsZero() {
		return nil
	}
	sale, err := store.FindNonVATSalesByID(ret.NonVATSalesID, bson.M{"products": 1})
	if err != nil || sale == nil {
		errs["non_vat_sales_id"] = "Invalid non-VAT sale"
		return nil
	}
	sold := map[primitive.ObjectID]float64{}
	for _, p := range sale.Products {
		sold[p.ProductID] += p.Quantity
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("non_vat_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	filter := bson.M{"non_vat_sales_id": ret.NonVATSalesID, "deleted": bson.M{"$ne": true}}
	if !ret.ID.IsZero() {
		filter["_id"] = bson.M{"$ne": ret.ID}
	}
	var others []NonVATSalesReturn
	cur, err := collection.Find(ctx, filter)
	if err != nil {
		return err
	}
	if err := cur.All(ctx, &others); err != nil {
		return err
	}
	returned := map[primitive.ObjectID]float64{}
	for _, o := range others {
		for _, p := range o.Products {
			if p.Selected {
				returned[p.ProductID] += p.Quantity
			}
		}
	}
	nonVATReturnLineErrors(errs, ret.Products, sold, returned)
	return nil
}

// nonVATReturnLineErrors checks the selected lines against what was sold and
// already returned, per product, in line order.
func nonVATReturnLineErrors(errs map[string]string, lines []QuotationSalesReturnProduct, sold, returned map[primitive.ObjectID]float64) {
	taken := map[primitive.ObjectID]float64{}
	for i, l := range lines {
		if !l.Selected || l.ProductID.IsZero() {
			continue
		}
		idx := strconv.Itoa(i)
		s, onSale := sold[l.ProductID]
		if !onSale {
			errs["product_id_"+idx] = "Product is not on the non-VAT sale"
			continue
		}
		max := s - returned[l.ProductID] - taken[l.ProductID]
		if l.Quantity > max+1e-9 {
			if max < 0 {
				max = 0
			}
			errs["quantity_"+idx] = "Quantity should not be greater than sold quantity: " + fmt.Sprintf("%.02f", max)
		}
		taken[l.ProductID] += l.Quantity
	}
}
