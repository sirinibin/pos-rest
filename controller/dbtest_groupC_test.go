package controller

import (
	"context"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// RequireDBExt exposes the DB harness to the external test package
// (controller_test), which is where the erp router can be imported without an
// import cycle. It is only compiled into the test binary.
func RequireDBExt(t *testing.T) *erpfixture.Fixture {
	t.Helper()
	return requireDB(t)
}

// gcCloneStore inserts a copy of fixture store src under a fresh id with the
// given fields $set on top, copies the listed documents (collection -> ids)
// from the source store DB into the new store DB (store_id rewritten), and
// drops everything again in t.Cleanup. Use it instead of mutating the shared
// fixture stores.
func gcCloneStore(t *testing.T, src primitive.ObjectID, set bson.M, docs map[string][]primitive.ObjectID) primitive.ObjectID {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	main := db.GetDB("")
	var storeDoc bson.M
	if err := main.Collection("store").FindOne(ctx, bson.M{"_id": src}).Decode(&storeDoc); err != nil {
		t.Fatalf("load store %s: %v", src.Hex(), err)
	}
	id := primitive.NewObjectID()
	storeDoc["_id"] = id
	storeDoc["name"] = uniqName("GC Clone")
	if _, err := main.Collection("store").InsertOne(ctx, storeDoc); err != nil {
		t.Fatalf("insert cloned store: %v", err)
	}
	t.Cleanup(func() {
		c, cc := context.WithTimeout(context.Background(), 30*time.Second)
		defer cc()
		_, _ = db.GetDB("").Collection("store").DeleteOne(c, bson.M{"_id": id})
		_ = db.GetDB("store_" + id.Hex()).Drop(c)
	})
	if len(set) > 0 {
		if _, err := main.Collection("store").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
			t.Fatalf("mark cloned store: %v", err)
		}
	}
	srcDB, dstDB := db.GetDB("store_"+src.Hex()), db.GetDB("store_"+id.Hex())
	for coll, ids := range docs {
		for _, docID := range ids {
			var d bson.M
			if err := srcDB.Collection(coll).FindOne(ctx, bson.M{"_id": docID}).Decode(&d); err != nil {
				t.Fatalf("load %s %s: %v", coll, docID.Hex(), err)
			}
			d["store_id"] = id
			if _, err := dstDB.Collection(coll).InsertOne(ctx, d); err != nil {
				t.Fatalf("copy %s %s: %v", coll, docID.Hex(), err)
			}
		}
	}
	return id
}

// gcStoreFlag reads zatca.zatca_reconnect_required of a store straight from MongoDB.
func gcStoreReconnectFlag(t *testing.T, storeID primitive.ObjectID) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var s struct {
		Zatca struct {
			Reconnect bool `bson:"zatca_reconnect_required"`
		} `bson:"zatca"`
	}
	if err := db.GetDB("").Collection("store").FindOne(ctx, bson.M{"_id": storeID}).Decode(&s); err != nil {
		t.Fatalf("read store %s: %v", storeID.Hex(), err)
	}
	return s.Zatca.Reconnect
}

// gcFindOne decodes one document of a store collection (nil when absent).
func gcFindOne(t *testing.T, storeID primitive.ObjectID, coll string, filter bson.M) bson.M {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var d bson.M
	if err := db.GetDB("store_"+storeID.Hex()).Collection(coll).FindOne(ctx, filter).Decode(&d); err != nil {
		return nil
	}
	return d
}

// gcFindAll returns every document of a store collection matching filter.
func gcFindAll(t *testing.T, storeID primitive.ObjectID, coll string, filter bson.M) []bson.M {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := db.GetDB("store_"+storeID.Hex()).Collection(coll).Find(ctx, filter)
	if err != nil {
		t.Fatalf("find %s: %v", coll, err)
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		t.Fatalf("decode %s: %v", coll, err)
	}
	return out
}
