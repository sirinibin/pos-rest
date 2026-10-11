package models

import (
	"context"
	"reflect"
	"strconv"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A product's stock is recomputed from its history and saved. Two documents
// saved at once could each recompute before the other saved, and the later
// save wrote the older figure; and a save of the WHOLE product put back the
// name, prices or stock adjustments another request had changed since this
// one read the product. So:
//   - productStockLocks serialises recompute-and-save per product (in-process:
//     the API runs as one process), so the last save counts every entry;
//   - the save writes only the fields the recompute (or stats update) changed
//     ($set of those paths), never the whole product document.
var productStockLocks KeyedLocks

func lockProductStock(productID *primitive.ObjectID) func() {
	return keyedLock(&productStockLocks, productID.Hex())
}

// RefreshProductStock recomputes and saves a product's stock in this store,
// then the stock of the products in its set.
func (store *Store) RefreshProductStock(productID *primitive.ObjectID) error {
	return store.UpdateProductLocked(productID, func(product *Product) error {
		return product.SetStock()
	})
}

// UpdateProductLocked loads a product, applies fn and saves the fields fn
// changed while holding the product's lock, then does the same for the
// products in its set.
func (store *Store) UpdateProductLocked(productID *primitive.ObjectID, fn func(*Product) error) error {
	setIDs, err := store.updateOneProductLocked(productID, fn)
	if err != nil {
		return err
	}
	for _, id := range setIDs {
		if _, err := store.updateOneProductLocked(id, fn); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) updateOneProductLocked(productID *primitive.ObjectID, fn func(*Product) error) (setIDs []*primitive.ObjectID, err error) {
	if productID == nil || productID.IsZero() {
		return nil, nil
	}
	unlock := lockProductStock(productID)
	defer unlock()
	product, err := store.FindProductByID(productID, bson.M{})
	if err != nil {
		return nil, err
	}
	before, err := productDoc(product)
	if err != nil {
		return nil, err
	}
	if err = fn(product); err != nil {
		return nil, err
	}
	if err = store.saveProductChanges(product, before); err != nil {
		return nil, err
	}
	for _, sp := range product.Set.Products {
		setIDs = append(setIDs, sp.ProductID)
	}
	return setIDs, nil
}

// productDoc is the product as stored (bson field names).
func productDoc(product *Product) (bson.M, error) {
	raw, err := bson.Marshal(product)
	if err != nil {
		return nil, err
	}
	var d bson.M
	err = bson.Unmarshal(raw, &d)
	return d, err
}

// productChanges is the $set of the fields that differ between before and
// after: top-level fields, and product_stores.<store>.<field> one by one so
// a store entry is never written back whole.
func productChanges(before, after bson.M) bson.M {
	set := bson.M{}
	for k, v := range after {
		if k == "_id" {
			continue
		}
		if k == "product_stores" {
			bs, _ := before[k].(bson.M)
			as, _ := v.(bson.M)
			for sid, entry := range as {
				be, _ := bs[sid].(bson.M)
				ae, ok := entry.(bson.M)
				if !ok || be == nil {
					if !reflect.DeepEqual(bs[sid], entry) {
						set["product_stores."+sid] = entry
					}
					continue
				}
				for f, fv := range ae {
					if !reflect.DeepEqual(be[f], fv) {
						set["product_stores."+sid+"."+f] = fv
					}
				}
			}
			continue
		}
		if !reflect.DeepEqual(before[k], v) {
			set[k] = v
		}
	}
	return set
}

func (store *Store) saveProductChanges(product *Product, before bson.M) error {
	after, err := productDoc(product)
	if err != nil {
		return err
	}
	set := productChanges(before, after)
	if len(set) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = db.GetDB("store_"+store.ID.Hex()).Collection("product").UpdateOne(ctx, bson.M{"_id": product.ID}, bson.M{"$set": set})
	return err
}

// ---- ledger accounts ----

// Two documents posting for a new customer at once both found no account
// and both created one (with the same account number, taken from a count).
// accountCreateLocks serialises find-or-create per store.
var accountCreateLocks KeyedLocks

// An account's balance is recomputed from its postings and saved; two at once
// could save the older figure last. accountBalanceLocks serialises it per
// account.
var accountBalanceLocks KeyedLocks

// freeAccountNumber is the first account number from n up that no account of
// the store uses. Call it under accountCreateLocks.
func (store *Store) freeAccountNumber(storeID *primitive.ObjectID, n int) (string, error) {
	for i := 0; i < 10000; i++ {
		num := strconv.Itoa(n + i)
		taken, err := store.GetTotalCount(bson.M{"store_id": storeID, "number": num}, "account")
		if err != nil {
			return "", err
		}
		if taken == 0 {
			return num, nil
		}
	}
	return strconv.Itoa(n), nil
}

func lockAccountBalance(accountID primitive.ObjectID) func() {
	return keyedLock(&accountBalanceLocks, accountID.Hex())
}

// setPartyBalance writes only a customer's or vendor's credit balance (and
// its account, when given).
func setPartyBalance(store *Store, collection string, id primitive.ObjectID, balance float64, account *Account) error {
	set := bson.M{"credit_balance": balance}
	if account != nil {
		set["account"] = account
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.GetDB("store_"+store.ID.Hex()).Collection(collection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// setPartyAccount writes only a customer's or vendor's account.
func setPartyAccount(store *Store, collection string, id primitive.ObjectID, account *Account) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.GetDB("store_"+store.ID.Hex()).Collection(collection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"account": account}})
	return err
}

// withoutPartyBalance is a customer or vendor as a $set without its credit
// balance and account. Those come from the ledger and only SetCreditBalance
// writes them (setPartyBalance): a customer loaded before a payment was
// posted and saved after it would otherwise put the older balance back.
func withoutPartyBalance(party interface{}) (bson.M, error) {
	raw, err := bson.Marshal(party)
	if err != nil {
		return nil, err
	}
	var set bson.M
	if err := bson.Unmarshal(raw, &set); err != nil {
		return nil, err
	}
	delete(set, "credit_balance")
	delete(set, "account")
	return set, nil
}

// RefreshCustomerCreditBalance recomputes a customer's credit balance from
// the ledger after a document posted for them.
func RefreshCustomerCreditBalance(storeID, customerID *primitive.ObjectID) error {
	if storeID == nil || customerID == nil || customerID.IsZero() {
		return nil
	}
	store, err := FindStoreByID(storeID, bson.M{})
	if err != nil {
		return err
	}
	customer, err := store.FindCustomerByID(customerID, bson.M{})
	if err != nil {
		return err
	}
	return customer.SetCreditBalance()
}

// AddNonVATSalesReturn counts a return on its non-VAT sale ($inc, so two
// returns saved at once both count).
func (store *Store) AddNonVATSalesReturn(saleID *primitive.ObjectID, amount float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.GetDB("store_"+store.ID.Hex()).Collection("non_vat_sales").UpdateOne(ctx, bson.M{"_id": saleID},
		bson.M{"$inc": bson.M{"return_count": int64(1), "return_amount": amount}})
	return err
}
