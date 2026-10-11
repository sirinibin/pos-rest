package models

import (
	"fmt"
	"log"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

// perfStoreIndexes are the compound indexes the dashboard / stats aggregates
// and the open-order lookups need. They match the slow queries seen on the
// live server: {deleted:{$ne}, store_id, date:{$gte,$lt}} + $group sums, and
// {deleted:{$ne}, status:{$in}, store_id} finds.
var perfStoreIndexes = []struct {
	Coll string
	Keys bson.D
}{
	{"order", bson.D{{Key: "store_id", Value: 1}, {Key: "status", Value: 1}, {Key: "deleted", Value: 1}}},
	{"order", bson.D{{Key: "store_id", Value: 1}, {Key: "date", Value: -1}, {Key: "deleted", Value: 1}}},
	{"purchase", bson.D{{Key: "store_id", Value: 1}, {Key: "date", Value: -1}, {Key: "deleted", Value: 1}}},
	{"salesreturn", bson.D{{Key: "store_id", Value: 1}, {Key: "date", Value: -1}, {Key: "deleted", Value: 1}}},
	{"expense", bson.D{{Key: "store_id", Value: 1}, {Key: "date", Value: -1}, {Key: "deleted", Value: 1}}},
	{"customerdeposit", bson.D{{Key: "store_id", Value: 1}, {Key: "date", Value: -1}, {Key: "deleted", Value: 1}}},
}

func init() {
	// the ZATCA chain lookups (NextZatcaChainLink) on every document type
	// that shares the store's ICV/PIH chain
	for _, coll := range zatcaChainCollections {
		for _, keys := range zatcaChainIndexes {
			perfStoreIndexes = append(perfStoreIndexes, struct {
				Coll string
				Keys bson.D
			}{coll, keys})
		}
	}
}

// CreatePerfIndexes creates perfStoreIndexes in this store's database.
// Creating an index that already exists with the same keys is a no-op.
func (store *Store) CreatePerfIndexes() error {
	var errs []string
	for _, ix := range perfStoreIndexes {
		if err := store.CreateCompoundIndex(ix.Coll, ix.Keys); err != nil {
			errs = append(errs, fmt.Sprintf("%s compound%v: %v", ix.Coll, ix.Keys, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("store %s: %d perf index error(s): %s", store.ID.Hex(), len(errs), strings.Join(errs, " | "))
	}
	return nil
}

// EnsureAllStoreIndexesAllStores gives every existing store every index a new
// store gets (CreateAllIndexes, perfStoreIndexes included), so an index added
// to the code reaches old stores on the next start too. Indexes a store
// already has are left as they are (creating one again is a no-op), nothing
// is dropped, and a store whose index differs (e.g. its own text index) only
// logs that index and carries on with the rest.
func EnsureAllStoreIndexesAllStores() {
	stores, err := GetAllStores()
	if err != nil {
		log.Printf("store indexes: listing stores: %v", err)
		return
	}
	for i := range stores {
		if err := stores[i].CreateAllIndexes(); err != nil {
			log.Printf("store indexes: %v", err)
		}
	}
}
