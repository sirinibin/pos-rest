//go:build integration

package models

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// One ICV sequence and one PIH chain per store across invoices, credit notes
// and debit notes, continuing above documents reported before the shared
// counter existed, and never reaching into another store.
func TestNextZatcaChainLink(t *testing.T) {
	storeID := primitive.NewObjectID().Hex()
	otherID := primitive.NewObjectID().Hex()
	storeDB := db.GetDB("store_" + storeID)
	t.Cleanup(func() {
		_ = storeDB.Drop(context.Background())
		_ = db.GetDB("store_" + otherID).Drop(context.Background())
	})
	genesis, _ := GenerateInvoiceHash("0")

	next := func(id string) ZatcaChainLink {
		t.Helper()
		link, err := NextZatcaChainLink(id)
		if err != nil {
			t.Fatalf("NextZatcaChainLink: %v", err)
		}
		return link
	}
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	insert := func(storeID, collection string, doc bson.M) {
		t.Helper()
		if _, err := db.GetDB("store_"+storeID).Collection(collection).InsertOne(context.Background(), doc); err != nil {
			t.Fatal(err)
		}
	}
	reported := func(hash string, icv int64, minutes int) bson.M {
		z := bson.M{"reporting_passed": true, "reporting_passed_at": at.Add(time.Duration(minutes) * time.Minute)}
		if icv > 0 {
			z["icv"] = icv
		}
		return bson.M{"hash": hash, "invoice_count_value": 900 + minutes, "zatca": z}
	}

	if l := next(storeID); l.ICV != 1 || l.PIH != genesis {
		t.Fatalf("empty store: got %+v, want ICV 1 and the hash of 0", l)
	}

	// Before the shared counter: separate per-type sequences, the highest
	// reported one being invoice 7. Unreported documents never count.
	insert(storeID, "order", bson.M{"hash": "inv-old", "invoice_count_value": 7,
		"zatca": bson.M{"reporting_passed": true, "reporting_passed_at": at}})
	insert(storeID, "salesreturn", bson.M{"hash": "cn-old", "invoice_count_value": 3,
		"zatca": bson.M{"reporting_passed": true, "reporting_passed_at": at.Add(time.Minute)}})
	insert(storeID, "order", bson.M{"hash": "failed", "invoice_count_value": 50,
		"zatca": bson.M{"reporting_passed": false}})
	if l := next(storeID); l.ICV != 8 || l.PIH != "cn-old" {
		t.Fatalf("after legacy documents: got %+v, want ICV 8 continuing the last reported (credit note) hash", l)
	}

	// Shared counter: an invoice, a debit note and a credit note in turn.
	insert(storeID, "order", reported("inv-8", 8, 2))
	insert(storeID, "customerdeposit", reported("dn-9", 9, 3))
	if l := next(storeID); l.ICV != 10 || l.PIH != "dn-9" {
		t.Fatalf("after a debit note: got %+v, want ICV 10 and the debit note's hash", l)
	}
	insert(storeID, "customerwithdrawal", reported("cn-10", 10, 4))
	if l := next(storeID); l.ICV != 11 || l.PIH != "cn-10" {
		t.Fatalf("after a credit note: got %+v, want ICV 11 and the credit note's hash", l)
	}

	// Another store's documents never move this store's chain.
	insert(otherID, "order", reported("other-store", 500, 9))
	if l := next(storeID); l.ICV != 11 || l.PIH != "cn-10" {
		t.Fatalf("another store's invoice changed this store's chain: %+v", l)
	}
	if l := next(otherID); l.ICV != 501 || l.PIH != "other-store" {
		t.Fatalf("other store: got %+v", l)
	}
}

// The chain lookup must not scan and sort a store's reported documents: on a
// large store that outran its timeout ("error finding the store's last ZATCA
// document: ... context deadline exceeded"). With the chain indexes every
// lookup reads at most a couple of documents and sorts nothing in memory.
func TestNextZatcaChainLink_UsesIndexes(t *testing.T) {
	storeID := primitive.NewObjectID().Hex()
	storeDB := db.GetDB("store_" + storeID)
	t.Cleanup(func() { _ = storeDB.Drop(context.Background()) })
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	// Legacy documents (no zatca.icv), then documents on the shared counter,
	// plus unreported drafts, in every chain collection.
	for ci, collection := range zatcaChainCollections {
		var docs []interface{}
		for i := 1; i <= 300; i++ {
			z := bson.M{"reporting_passed": i%10 != 0,
				"reporting_passed_at": at.Add(time.Duration(i*4+ci) * time.Minute)}
			if i > 100 {
				z["icv"] = int64(i*4 + ci)
			}
			docs = append(docs, bson.M{"hash": collection + "-" + strconv.Itoa(i),
				"invoice_count_value": int64(i), "zatca": z})
		}
		if _, err := storeDB.Collection(collection).InsertMany(ctx, docs); err != nil {
			t.Fatal(err)
		}
	}
	want, err := NextZatcaChainLink(storeID)
	if err != nil {
		t.Fatal(err)
	}

	if err := EnsureZatcaChainIndexes(storeID); err != nil {
		t.Fatalf("EnsureZatcaChainIndexes: %v", err)
	}
	if err := EnsureZatcaChainIndexes(storeID); err != nil {
		t.Fatalf("EnsureZatcaChainIndexes is not idempotent: %v", err)
	}
	got, err := NextZatcaChainLink(storeID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("indexes changed the answer: %+v, want %+v", got, want)
	}
	// Highest ICV is 299*4+3 (customerwithdrawal; 300 is unreported), and the
	// last reported document is that same one.
	if got.ICV != 299*4+3+1 || got.PIH != "customerwithdrawal-299" {
		t.Fatalf("got %+v", got)
	}

	queries := []struct {
		filter bson.M
		sort   bson.D
	}{
		{bson.M{"zatca.reporting_passed": true}, bson.D{{Key: "zatca.reporting_passed_at", Value: -1}}},
		{bson.M{"zatca.reporting_passed": true}, bson.D{{Key: "zatca.icv", Value: -1}}},
		{bson.M{"zatca.reporting_passed": true, "zatca.icv": bson.M{"$exists": false}},
			bson.D{{Key: "invoice_count_value", Value: -1}}},
	}
	for _, collection := range zatcaChainCollections {
		for _, q := range queries {
			var res bson.M
			err := storeDB.RunCommand(ctx, bson.D{
				{Key: "explain", Value: bson.D{
					{Key: "find", Value: collection},
					{Key: "filter", Value: q.filter},
					{Key: "sort", Value: q.sort},
					{Key: "limit", Value: 1},
				}},
				{Key: "verbosity", Value: "executionStats"},
			}).Decode(&res)
			if err != nil {
				t.Fatal(err)
			}
			plan, _ := bson.MarshalExtJSON(res["queryPlanner"].(bson.M)["winningPlan"], false, false)
			stats := res["executionStats"].(bson.M)
			examined := toInt64(stats["totalDocsExamined"])
			if strings.Contains(string(plan), `"COLLSCAN"`) || strings.Contains(string(plan), `"stage":"SORT"`) {
				t.Errorf("%s %v sort %v: scans or sorts in memory: %s", collection, q.filter, q.sort, plan)
			}
			if examined > 2 {
				t.Errorf("%s %v sort %v: examined %d documents, want at most 2", collection, q.filter, q.sort, examined)
			}
		}
	}
}

// A new store gets the chain indexes from CreateAllIndexes, which every way
// of creating a store runs (create, guest sign-up, duplicate).
func TestCreateAllIndexes_IncludesZatcaChainIndexes(t *testing.T) {
	store := Store{ID: primitive.NewObjectID()}
	storeDB := db.GetDB("store_" + store.ID.Hex())
	t.Cleanup(func() { _ = storeDB.Drop(context.Background()) })
	if err := store.CreateAllIndexes(); err != nil {
		t.Fatalf("CreateAllIndexes: %v", err)
	}
	ctx := context.Background()
	for _, collection := range zatcaChainCollections {
		cur, err := storeDB.Collection(collection).Indexes().List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var specs []struct {
			Key bson.D `bson:"key"`
		}
		if err := cur.All(ctx, &specs); err != nil {
			t.Fatal(err)
		}
		for _, model := range zatcaChainIndexes {
			want, _ := bson.Marshal(model.Keys)
			found := false
			for _, spec := range specs {
				got, _ := bson.Marshal(spec.Key)
				if bson.Raw(got).String() == bson.Raw(want).String() {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: missing index %v", collection, model.Keys)
			}
		}
	}
}
