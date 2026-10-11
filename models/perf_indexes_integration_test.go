//go:build integration

package models

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func indexKeySet(t *testing.T, col *mongo.Collection) map[string]bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cur, err := col.Indexes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var specs []struct {
		Key bson.D `bson:"key"`
	}
	if err := cur.All(ctx, &specs); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	for _, s := range specs {
		b, _ := bson.MarshalExtJSON(s.Key, false, false)
		set[string(b)] = true
	}
	return set
}

func keyJSON(d bson.D) string {
	b, _ := bson.MarshalExtJSON(d, false, false)
	return string(b)
}

func TestCreatePerfIndexes_AllPresentAndIdempotent(t *testing.T) {
	id := primitive.NewObjectID()
	store := &Store{ID: id}
	sdb := db.GetDB("store_" + id.Hex())
	t.Cleanup(func() { _ = sdb.Drop(context.Background()) })

	// twice: the second run must be a no-op, not an error
	for i := 0; i < 2; i++ {
		if err := store.CreatePerfIndexes(); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	for _, ix := range perfStoreIndexes {
		if !indexKeySet(t, sdb.Collection(ix.Coll))[keyJSON(ix.Keys)] {
			t.Errorf("%s: index %s missing", ix.Coll, keyJSON(ix.Keys))
		}
	}
}

func TestCreatePerfIndexes_StatsSumUsesIndex(t *testing.T) {
	id := primitive.NewObjectID()
	store := &Store{ID: id}
	sdb := db.GetDB("store_" + id.Hex())
	t.Cleanup(func() { _ = sdb.Drop(context.Background()) })
	ctx := context.Background()

	col := sdb.Collection("customerdeposit")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var docs []interface{}
	for i := 0; i < 50; i++ {
		docs = append(docs, bson.M{"store_id": id, "date": base.AddDate(0, 0, i), "amount": 10, "deleted": i%10 == 0})
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePerfIndexes(); err != nil {
		t.Fatal(err)
	}

	match := bson.M{"deleted": bson.M{"$ne": true}, "store_id": id,
		"date": bson.M{"$gte": base, "$lt": base.AddDate(0, 0, 20)}}
	var explain bson.M
	err := sdb.RunCommand(ctx, bson.D{{Key: "explain", Value: bson.D{
		{Key: "aggregate", Value: "customerdeposit"},
		{Key: "pipeline", Value: bson.A{bson.M{"$match": match}, bson.M{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}}}},
		{Key: "cursor", Value: bson.M{}},
	}}}).Decode(&explain)
	if err != nil {
		t.Fatal(err)
	}
	plan, _ := bson.MarshalExtJSON(explain, false, false)
	if !bytes.Contains(plan, []byte(`"store_id_1_date_-1_deleted_1"`)) || bytes.Contains(plan, []byte(`"COLLSCAN"`)) {
		t.Fatalf("stats sum does not use the perf index: %s", plan)
	}

	// and the total over the indexed range is still right: days 0..19, minus deleted days 0 and 10
	cur, err := col.Aggregate(ctx, bson.A{bson.M{"$match": match}, bson.M{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var out []struct {
		Total float64 `bson:"total"`
	}
	if err := cur.All(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Total != 180 {
		t.Fatalf("total %v, want 180", out)
	}
}

func TestEnsureProcurementMessageIndexes_UnreadIndex(t *testing.T) {
	EnsureProcurementMessageIndexes()
	want := bson.D{{Key: "store_id", Value: 1}, {Key: "direction", Value: 1}, {Key: "type", Value: 1}, {Key: "read", Value: 1}, {Key: "message_date", Value: 1}}
	if !indexKeySet(t, procurementMessageCol())[keyJSON(want)] {
		t.Fatalf("procurement_messages unread index %s missing", keyJSON(want))
	}
}

// An existing store (made before today's indexes) gets every index a new
// store gets when the server starts: running CreateAllIndexes again over a
// store that already has some of them is a no-op for those, adds the rest,
// and a clashing index of the store's own only costs that one index.
func TestCreateAllIndexes_ExistingStoreGetsEveryIndex(t *testing.T) {
	id := primitive.NewObjectID()
	store := &Store{ID: id}
	sdb := db.GetDB("store_" + id.Hex())
	t.Cleanup(func() { _ = sdb.Drop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// an old store: data, one of the indexes already there, and an index of
	// its own on the same keys as one of ours but under another name
	if _, err := sdb.Collection("salesreturn").InsertOne(ctx, bson.M{"code": "R-1",
		"zatca": bson.M{"reporting_passed": true, "icv": 3}}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateCompoundIndex("order", zatcaChainIndexes[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := sdb.Collection("vendor").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "code", Value: 1}}, Options: options.Index().SetName("legacy_vendor_code")}); err != nil {
		t.Fatal(err)
	}

	_ = store.CreateAllIndexes() // the clashing vendor index is reported, the rest still built
	if err := store.CreatePerfIndexes(); err != nil {
		t.Fatalf("second run must be a no-op: %v", err)
	}
	for _, ix := range perfStoreIndexes {
		if !indexKeySet(t, sdb.Collection(ix.Coll))[keyJSON(ix.Keys)] {
			t.Errorf("%s: index %s missing", ix.Coll, keyJSON(ix.Keys))
		}
	}
	for _, want := range []struct {
		coll string
		keys bson.D
	}{
		{"order", bson.D{{Key: "customer_id", Value: 1}}},
		{"product_history", bson.D{{Key: "product_id", Value: 1}}},
		{"vendor", bson.D{{Key: "phone", Value: 1}}},
	} {
		if !indexKeySet(t, sdb.Collection(want.coll))[keyJSON(want.keys)] {
			t.Errorf("%s: index %s missing", want.coll, keyJSON(want.keys))
		}
	}
}
