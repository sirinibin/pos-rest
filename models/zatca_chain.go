package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// zatcaChainCollections hold the documents a store's ZATCA device (EGS unit)
// signs: invoices, credit notes (sales returns, customer withdrawals) and
// debit notes (customer deposits). ZATCA wants one invoice counter (ICV) and
// one previous-invoice-hash (PIH) chain per device across all of them.
var zatcaChainCollections = []string{"order", "salesreturn", "customerdeposit", "customerwithdrawal"}

type zatcaChainDoc struct {
	Hash              string `bson:"hash"`
	InvoiceCountValue int64  `bson:"invoice_count_value"`
	Zatca             struct {
		ICV        int64      `bson:"icv"`
		ReportedAt *time.Time `bson:"reporting_passed_at"`
	} `bson:"zatca"`
}

// ZatcaChainLink is where the store's next ZATCA document continues the chain.
type ZatcaChainLink struct {
	ICV int64  // invoice counter value of the next document
	PIH string // hash of the store's last reported document
}

// NextZatcaChainLink returns the ICV and PIH for the store's next ZATCA
// document. The ICV is one past the highest one the store reported, of any
// document type; the PIH is the hash of the last document it reported, of
// any type (ZATCA's hash of "0" for the first one). Documents reported before
// the shared counter existed have no zatca.icv: their invoice_count_value is
// what went to ZATCA, so the counter continues above those too.
// Callers hold the store's "zatca" queue so no other report runs meanwhile.
func NextZatcaChainLink(storeID string) (link ZatcaChainLink, err error) {
	storeDB := db.GetDB("store_" + storeID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	projection := bson.M{"hash": 1, "invoice_count_value": 1, "zatca.icv": 1, "zatca.reporting_passed_at": 1}
	findOne := func(collection string, filter bson.M, sort bson.D) (*zatcaChainDoc, error) {
		var doc zatcaChainDoc
		err := storeDB.Collection(collection).FindOne(ctx, filter,
			options.FindOne().SetSort(sort).SetProjection(projection)).Decode(&doc)
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &doc, nil
	}

	reported := bson.M{"zatca.reporting_passed": true}
	var last *zatcaChainDoc
	var maxICV int64
	for _, collection := range zatcaChainCollections {
		doc, err := findOne(collection, reported, bson.D{{Key: "zatca.reporting_passed_at", Value: -1}})
		if err != nil {
			return link, err
		}
		if doc != nil && doc.Zatca.ReportedAt != nil &&
			(last == nil || doc.Zatca.ReportedAt.After(*last.Zatca.ReportedAt)) {
			last = doc
		}

		doc, err = findOne(collection, reported, bson.D{{Key: "zatca.icv", Value: -1}})
		if err != nil {
			return link, err
		}
		if doc != nil && doc.Zatca.ICV > maxICV {
			maxICV = doc.Zatca.ICV
		}

		doc, err = findOne(collection,
			bson.M{"zatca.reporting_passed": true, "zatca.icv": bson.M{"$exists": false}},
			bson.D{{Key: "invoice_count_value", Value: -1}})
		if err != nil {
			return link, err
		}
		if doc != nil && doc.InvoiceCountValue > maxICV {
			maxICV = doc.InvoiceCountValue
		}
	}

	link.ICV = maxICV + 1
	if last != nil && last.Hash != "" {
		link.PIH = last.Hash
	} else if link.PIH, err = GenerateInvoiceHash("0"); err != nil {
		return link, err
	}
	return link, nil
}

// zatcaXMLPath is where a document's unsigned XML is written for the signing
// script. Stores number their documents independently, so two stores' "first
// invoice" share a code: the store id keeps them from overwriting each other.
func zatcaXMLPath(kind string, storeID *primitive.ObjectID, code string) string {
	store := ""
	if storeID != nil {
		store = storeID.Hex() + "_"
	}
	return "ZatcaPython/templates/" + kind + "_" + store + code + ".xml"
}

// IsZatcaReported reports whether ZATCA already accepted the store's document
// with this id in the collection (one of zatcaChainCollections).
func (store *Store) IsZatcaReported(collection string, id *primitive.ObjectID) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := db.GetDB("store_"+store.ID.Hex()).Collection(collection).CountDocuments(ctx,
		bson.M{"_id": id, "zatca.reporting_passed": true})
	return n > 0, err
}
