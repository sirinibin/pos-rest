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

// Each chain lookup walks an index straight to its one document: no
// collection scan and no in-memory sort of every reported document, which is
// what timed out on the live server ("error finding the store's last ZATCA
// document ... context deadline exceeded"). The link itself is unchanged.
func TestNextZatcaChainLink_LookupsUseTheChainIndexes(t *testing.T) {
	id := primitive.NewObjectID()
	store := &Store{ID: id}
	storeDB := db.GetDB("store_" + id.Hex())
	t.Cleanup(func() { _ = storeDB.Drop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for _, coll := range zatcaChainCollections {
		var docs []interface{}
		for i := 1; i <= 300; i++ {
			z := bson.M{"reporting_passed": i%3 != 0, "reporting_passed_at": base.Add(time.Duration(i) * time.Minute)}
			if i > 100 { // reported with the shared counter
				z["icv"] = i
			}
			docs = append(docs, bson.M{"hash": coll + "-" + string(rune('A'+i%26)), "invoice_count_value": i, "zatca": z})
		}
		if _, err := storeDB.Collection(coll).InsertMany(ctx, docs); err != nil {
			t.Fatal(err)
		}
	}
	// the store's last report: a withdrawal (credit note) with the top ICV
	if _, err := storeDB.Collection("customerwithdrawal").InsertOne(ctx, bson.M{"hash": "H-last", "invoice_count_value": 301,
		"zatca": bson.M{"reporting_passed": true, "icv": 500, "reporting_passed_at": base.Add(24 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePerfIndexes(); err != nil {
		t.Fatal(err)
	}

	lookups := []struct {
		filter bson.M
		sort   bson.D
	}{
		{bson.M{"zatca.reporting_passed": true}, bson.D{{Key: "zatca.reporting_passed_at", Value: -1}}},
		{bson.M{"zatca.reporting_passed": true}, bson.D{{Key: "zatca.icv", Value: -1}}},
		{bson.M{"zatca.reporting_passed": true, "zatca.icv": bson.M{"$exists": false}},
			bson.D{{Key: "zatca.icv", Value: -1}, {Key: "invoice_count_value", Value: -1}}},
	}
	for _, coll := range zatcaChainCollections {
		for _, l := range lookups {
			var explain bson.M
			err := storeDB.RunCommand(ctx, bson.D{{Key: "explain", Value: bson.D{
				{Key: "find", Value: coll}, {Key: "filter", Value: l.filter}, {Key: "sort", Value: l.sort}, {Key: "limit", Value: 1},
			}}, {Key: "verbosity", Value: "executionStats"}}).Decode(&explain)
			if err != nil {
				t.Fatal(err)
			}
			plan, _ := bson.MarshalExtJSON(explain["queryPlanner"].(bson.M)["winningPlan"], false, false)
			if !bytes.Contains(plan, []byte(`"IXSCAN"`)) || bytes.Contains(plan, []byte(`"COLLSCAN"`)) ||
				bytes.Contains(plan, []byte(`"stage":"SORT"`)) {
				t.Errorf("%s %v sort %v: not an indexed walk: %s", coll, l.filter, l.sort, plan)
			}
			stats := explain["executionStats"].(bson.M)
			if n, _ := stats["totalDocsExamined"].(int32); n > 2 {
				t.Errorf("%s %v sort %v: examined %d documents, want at most 2", coll, l.filter, l.sort, n)
			}
		}
	}

	link, err := NextZatcaChainLink(id.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 501 || link.PIH != "H-last" {
		t.Fatalf("link %+v, want ICV 501 and the withdrawal's hash", link)
	}
}

// A document ZATCA rejected keeps its ICV and its hash in the store's chain
// (DTG p.48, FAQ p.71: documents 1, 2 rejected, 3 → 3's PIH is hash(2); the
// corrected 2 goes again as 4 with PIH hash(3)). Its ICV is never used again
// and it goes again with a new UUID.
func TestZatcaRejectedDocumentsStayInTheChain(t *testing.T) {
	storeID := primitive.NewObjectID()
	storeDB := db.GetDB("store_" + storeID.Hex())
	t.Cleanup(func() { _ = storeDB.Drop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	base := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	// document 1: reported
	if _, err := storeDB.Collection("order").InsertOne(ctx, bson.M{"hash": "H-1", "invoice_count_value": 1,
		"zatca": bson.M{"reporting_passed": true, "icv": 1, "reporting_passed_at": base}}); err != nil {
		t.Fatal(err)
	}
	// document 2: a saved sale ZATCA rejects
	saleID := primitive.NewObjectID()
	if _, err := storeDB.Collection("order").InsertOne(ctx, bson.M{"_id": saleID, "code": "S-2", "uuid": "uuid-2",
		"invoice_count_value": 2, "zatca": bson.M{"reporting_passed": false}}); err != nil {
		t.Fatal(err)
	}
	link, err := NextZatcaChainLink(storeID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 2 || link.PIH != "H-1" {
		t.Fatalf("before the rejection: link %+v, want ICV 2 and H-1", link)
	}
	newUUID := KeepZatcaRejectionInChain(&storeID, ZatcaRejection{
		Collection: "order", DocID: &saleID, Code: "S-2", UUID: "uuid-2",
		ICV: link.ICV, Hash: "H-2-rejected", PIH: link.PIH, Error: "BR-KSA-44", RejectedAt: base.Add(time.Minute),
	})
	if newUUID == "" || newUUID == "uuid-2" {
		t.Fatalf("the rejected sale keeps its UUID %q; ZATCA wants a new one when it goes again", newUUID)
	}
	var sale bson.M
	if err := storeDB.Collection("order").FindOne(ctx, bson.M{"_id": saleID}).Decode(&sale); err != nil {
		t.Fatal(err)
	}
	if sale["uuid"] != newUUID {
		t.Errorf("stored UUID %v, want the new %s", sale["uuid"], newUUID)
	}
	var row ZatcaRejection
	if err := storeDB.Collection(zatcaRejectedCollection).FindOne(ctx, bson.M{"doc_id": saleID}).Decode(&row); err != nil {
		t.Fatal(err)
	}
	if row.ICV != 2 || row.Hash != "H-2-rejected" || row.PIH != "H-1" || row.UUID != "uuid-2" || row.Code != "S-2" || row.Error != "BR-KSA-44" {
		t.Errorf("rejection row %+v", row)
	}

	// document 3 continues from the rejected one
	link, err = NextZatcaChainLink(storeID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 3 || link.PIH != "H-2-rejected" {
		t.Fatalf("after the rejection: link %+v, want ICV 3 and the rejected document's hash", link)
	}
	// document 3 (a return) is reported, then the corrected sale goes as 4
	if _, err := storeDB.Collection("salesreturn").InsertOne(ctx, bson.M{"hash": "H-3", "invoice_count_value": 1,
		"zatca": bson.M{"reporting_passed": true, "icv": 3, "reporting_passed_at": base.Add(2 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	link, err = NextZatcaChainLink(storeID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 4 || link.PIH != "H-3" {
		t.Fatalf("after document 3: link %+v, want ICV 4 and H-3", link)
	}

	// a sale rejected while it was being created is never saved: the row
	// alone keeps its ICV and hash
	KeepZatcaRejectionInChain(&storeID, ZatcaRejection{
		Collection: "order", Code: "S-5", UUID: "uuid-5", ICV: 4, Hash: "H-4-rejected", PIH: "H-3",
		RejectedAt: base.Add(3 * time.Minute),
	})
	link, err = NextZatcaChainLink(storeID.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if link.ICV != 5 || link.PIH != "H-4-rejected" {
		t.Fatalf("after an unsaved rejected sale: link %+v, want ICV 5 and H-4-rejected", link)
	}

	// nothing to keep without a hash or an ICV (the script failed before signing)
	for _, r := range []ZatcaRejection{{Collection: "order", UUID: "u", ICV: 9}, {Collection: "order", UUID: "u", Hash: "H"}} {
		if got := KeepZatcaRejectionInChain(&storeID, r); got != "u" {
			t.Errorf("%+v: UUID changed to %q", r, got)
		}
	}
	if got := KeepZatcaRejectionInChain(nil, ZatcaRejection{UUID: "u", ICV: 1, Hash: "H"}); got != "u" {
		t.Errorf("no store: UUID changed to %q", got)
	}
	n, err := storeDB.Collection(zatcaRejectedCollection).CountDocuments(ctx, bson.M{})
	if err != nil || n != 2 {
		t.Fatalf("rejection rows = %d (%v), want 2", n, err)
	}
}

// The rejected-submission lookups walk an index too.
func TestZatcaRejectedLookupsUseTheirIndexes(t *testing.T) {
	id := primitive.NewObjectID()
	store := &Store{ID: id}
	storeDB := db.GetDB("store_" + id.Hex())
	t.Cleanup(func() { _ = storeDB.Drop(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var docs []interface{}
	for i := 1; i <= 200; i++ {
		docs = append(docs, ZatcaRejection{Collection: "order", Code: "S", UUID: "u", ICV: int64(i), Hash: "H",
			RejectedAt: base.Add(time.Duration(i) * time.Minute)})
	}
	if _, err := storeDB.Collection(zatcaRejectedCollection).InsertMany(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePerfIndexes(); err != nil {
		t.Fatal(err)
	}
	for _, sort := range []bson.D{{{Key: "icv", Value: -1}}, {{Key: "rejected_at", Value: -1}}} {
		var explain bson.M
		err := storeDB.RunCommand(ctx, bson.D{{Key: "explain", Value: bson.D{
			{Key: "find", Value: zatcaRejectedCollection}, {Key: "filter", Value: bson.M{}}, {Key: "sort", Value: sort}, {Key: "limit", Value: 1},
		}}, {Key: "verbosity", Value: "executionStats"}}).Decode(&explain)
		if err != nil {
			t.Fatal(err)
		}
		plan, _ := bson.MarshalExtJSON(explain["queryPlanner"].(bson.M)["winningPlan"], false, false)
		if !bytes.Contains(plan, []byte(`"IXSCAN"`)) || bytes.Contains(plan, []byte(`"stage":"SORT"`)) {
			t.Errorf("sort %v: not an indexed walk: %s", sort, plan)
		}
	}
}
