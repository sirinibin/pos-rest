//go:build integration

package models

import (
	"context"
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
