package models

import (
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
)

// A store settings save used to write the whole store document: two admins
// saving different settings at the same moment lost one change, and a save
// that read the store before a ZATCA connect put the old connection state
// back. UpdateChanged writes only the fields that differ from the store as it
// was read (before).

// StoreSnapshot is the store as stored (bson field names), taken before the
// request changes it.
func StoreSnapshot(store *Store) (bson.M, error) {
	return bsonDoc(store)
}

func bsonDoc(v interface{}) (bson.M, error) {
	raw, err := bson.Marshal(v)
	if err != nil {
		return nil, err
	}
	var d bson.M
	err = bson.Unmarshal(raw, &d)
	return d, err
}

// changedFields adds to set the paths whose values differ between before and
// after, going depth levels into sub-documents so that one changed setting
// does not write its whole group back. A field missing from after (omitempty)
// is left as stored.
func changedFields(prefix string, before, after bson.M, depth int, set bson.M) {
	for k, av := range after {
		if prefix == "" && k == "_id" {
			continue
		}
		bv, had := before[k]
		if had && reflect.DeepEqual(bv, av) {
			continue
		}
		bm, bok := bv.(bson.M)
		am, aok := av.(bson.M)
		if depth > 0 && bok && aok {
			changedFields(prefix+k+".", bm, am, depth-1, set)
			continue
		}
		set[prefix+k] = av
	}
}

// UpdateChanged saves the store like Update, but writes only the fields that
// changed since before (StoreSnapshot of the store as read).
func (store *Store) UpdateChanged(before bson.M) error {
	unset, err := store.prepareUpdate()
	if err != nil {
		return err
	}
	after, err := bsonDoc(store)
	if err != nil {
		return err
	}
	set := bson.M{}
	changedFields("", before, after, 2, set)
	for k := range unset {
		delete(set, k)
	}
	updateDoc := bson.M{}
	if len(set) > 0 {
		updateDoc["$set"] = set
	}
	return store.writeUpdate(updateDoc, unset)
}
