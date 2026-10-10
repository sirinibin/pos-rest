package models

import (
	"sync"

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
