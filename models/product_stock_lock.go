package models

import (
	"context"
	"sync"
	"time"

	"github.com/sirinibin/startpos/backend/db"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A product's stock is recomputed from its history and saved with the whole
// product. Two documents saved at once could each recompute before the other
// saved, and the later save wrote the older figure (a sale or return lost
// from the stock). productStockLocks serializes recompute-and-save per
// product, so the last save always counts every history entry.
var productStockLocks sync.Map // product id hex -> *sync.Mutex

func lockProductStock(productID *primitive.ObjectID) func() {
	return keyedLock(&productStockLocks, productID.Hex())
}

// keyedLock locks the mutex for key in locks and returns its unlock.
func keyedLock(locks *sync.Map, key string) func() {
	m, _ := locks.LoadOrStore(key, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Two documents posting for a new customer at once both found no account
// and both created one (with the same account number, taken from a count).
// accountCreateLocks serializes find-or-create per store.
var accountCreateLocks sync.Map // store id hex -> *sync.Mutex

// RefreshProductStock recomputes and saves a product's stock in this store,
// then the stock of the products in its set.
func (store *Store) RefreshProductStock(productID *primitive.ObjectID) error {
	setIDs, err := store.refreshOneProductStock(productID)
	if err != nil {
		return err
	}
	for _, id := range setIDs {
		if _, err := store.refreshOneProductStock(id); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) refreshOneProductStock(productID *primitive.ObjectID) (setIDs []*primitive.ObjectID, err error) {
	unlock := lockProductStock(productID)
	defer unlock()
	product, err := store.FindProductByID(productID, bson.M{})
	if err != nil {
		return nil, err
	}
	if err = product.SetStock(); err != nil {
		return nil, err
	}
	if err = product.Update(&store.ID); err != nil {
		return nil, err
	}
	for _, sp := range product.Set.Products {
		setIDs = append(setIDs, sp.ProductID)
	}
	return setIDs, nil
}

// UpdateProductLocked loads a product, applies fn and saves it while holding
// the product's stock lock, then does the same for the products in its set.
// Product stats updates save the whole product: one that read the product
// before a concurrent stock save wrote the old stock back.
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
	unlock := lockProductStock(productID)
	defer unlock()
	product, err := store.FindProductByID(productID, bson.M{})
	if err != nil {
		return nil, err
	}
	if err = fn(product); err != nil {
		return nil, err
	}
	if err = product.Update(&store.ID); err != nil {
		return nil, err
	}
	for _, sp := range product.Set.Products {
		setIDs = append(setIDs, sp.ProductID)
	}
	return setIDs, nil
}

// Checks that read the database and then insert ("is this email taken?",
// "was the same receipt just saved?") pass for every one of several requests
// that arrive together. namedLocks serializes such check-and-insert per key.
var namedLocks sync.Map // key -> *sync.Mutex

// LockKey locks key and returns its unlock.
func LockKey(key string) func() {
	return keyedLock(&namedLocks, key)
}

// An account's balance is recomputed from its postings and saved; two at once
// could save the older figure last. accountBalanceLocks serializes it per account.
var accountBalanceLocks sync.Map // account id hex -> *sync.Mutex

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

// AddNonVATSalesReturn counts a return on its non-VAT sale.
func (store *Store) AddNonVATSalesReturn(saleID *primitive.ObjectID, amount float64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.GetDB("store_"+store.ID.Hex()).Collection("non_vat_sales").UpdateOne(ctx, bson.M{"_id": saleID},
		bson.M{"$inc": bson.M{"return_count": 1, "return_amount": amount}})
	return err
}
