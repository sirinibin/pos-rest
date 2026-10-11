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

// A store is one ZATCA device: invoices, credit notes and debit notes share
// one ICV sequence and one PIH chain, continued in the order documents were
// reported (not the order they were saved).
func TestNextZatcaChainLink_OneChainPerStore(t *testing.T) {
	storeID := primitive.NewObjectID()
	storeDB := db.GetDB("store_" + storeID.Hex())
	t.Cleanup(func() {
		for _, c := range zatcaChainCollections {
			_ = storeDB.Collection(c).Drop(context.Background())
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	zero, err := GenerateInvoiceHash("0")
	if err != nil {
		t.Fatal(err)
	}
	link, err := NextZatcaChainLink(storeID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 1 || link.PIH != zero {
		t.Fatalf("empty store: got ICV %d PIH %q, want 1 and the hash of 0", link.ICV, link.PIH)
	}

	base := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	insert := func(coll string, doc bson.M) {
		t.Helper()
		if _, err := storeDB.Collection(coll).InsertOne(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	// a sale reported before the shared counter existed: its own number was its ICV
	insert("order", bson.M{"hash": "H-sale-1", "invoice_count_value": 4,
		"zatca": bson.M{"reporting_passed": true, "reporting_passed_at": base}})
	// a return reported later with the shared counter
	insert("salesreturn", bson.M{"hash": "H-return", "invoice_count_value": 1,
		"zatca": bson.M{"reporting_passed": true, "icv": 5, "reporting_passed_at": base.Add(time.Minute)}})
	// a deposit (debit note) reported last
	insert("customerdeposit", bson.M{"hash": "H-deposit", "invoice_count_value": 1,
		"zatca": bson.M{"reporting_passed": true, "icv": 6, "reporting_passed_at": base.Add(2 * time.Minute)}})
	// saved but never reported: takes no part in the chain
	insert("customerwithdrawal", bson.M{"hash": "H-unreported", "invoice_count_value": 9,
		"zatca": bson.M{"reporting_passed": false, "icv": 9}})
	insert("order", bson.M{"hash": "H-sale-saved-early", "invoice_count_value": 2,
		"zatca": bson.M{"reporting_passed": false}})

	link, err = NextZatcaChainLink(storeID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 7 {
		t.Errorf("ICV = %d, want 7 (one past the store's highest, any document type)", link.ICV)
	}
	if link.PIH != "H-deposit" {
		t.Errorf("PIH = %q, want the hash of the deposit reported last", link.PIH)
	}
}

func TestZatcaXMLPath_CarriesTheStore(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	if zatcaXMLPath("invoice", &a, "S-1") == zatcaXMLPath("invoice", &b, "S-1") {
		t.Error("two stores' first invoices share an XML file")
	}
	if got := zatcaXMLPath("invoice", nil, "S-1"); got != "ZatcaPython/templates/invoice_S-1.xml" {
		t.Errorf("no store: %q", got)
	}
}
