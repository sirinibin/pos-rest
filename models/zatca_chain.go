package models

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// zatcaChainCollections hold every document a store's ZATCA device (EGS
// unit) signs: invoices, credit notes (sales returns, customer withdrawals)
// and debit notes (customer deposits). ZATCA counts one invoice counter (ICV)
// and one previous-invoice-hash (PIH) chain per device across all of them.
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

// zatcaChainIndexes let NextZatcaChainLink's lookups walk an index to the
// one document they want instead of scanning and sorting every reported
// document in memory, which on a busy store with a swapping server took
// longer than the request could wait. Ensured for every store on startup and
// for new stores with CreatePerfIndexes.
var zatcaChainIndexes = []bson.D{
	// the document reported last (its hash is the next PIH)
	{{Key: "zatca.reporting_passed", Value: 1}, {Key: "zatca.reporting_passed_at", Value: -1}},
	// the highest ICV, and the highest legacy document number among
	// documents reported before the shared counter (zatca.icv missing)
	{{Key: "zatca.reporting_passed", Value: 1}, {Key: "zatca.icv", Value: -1}, {Key: "invoice_count_value", Value: -1}},
}

// zatcaRejectedCollection keeps every document ZATCA rejected, one row per
// rejected submission. ZATCA records the hash of a rejected document and
// counts its ICV, so the chain goes on from it: its ICV is never used again
// and the next document's PIH is its hash (DTG p.48, FAQ p.71 and p.77). A
// sale rejected when it is created is never saved, so the row is the only
// place its ICV and hash live.
const zatcaRejectedCollection = "zatca_rejected"

// zatcaRejectedIndexes serve NextZatcaChainLink's two lookups in
// zatcaRejectedCollection (highest ICV, latest rejection).
var zatcaRejectedIndexes = []bson.D{
	{{Key: "icv", Value: -1}},
	{{Key: "rejected_at", Value: -1}},
}

// ZatcaRejection is one document submission ZATCA rejected.
type ZatcaRejection struct {
	ID         primitive.ObjectID  `bson:"_id,omitempty" json:"id,omitempty"`
	Collection string              `bson:"collection" json:"collection"`
	DocID      *primitive.ObjectID `bson:"doc_id,omitempty" json:"doc_id,omitempty"`
	Code       string              `bson:"code" json:"code"`
	UUID       string              `bson:"uuid" json:"uuid"`
	ICV        int64               `bson:"icv" json:"icv"`
	Hash       string              `bson:"hash" json:"hash"`
	PIH        string              `bson:"pih" json:"pih"`
	Error      string              `bson:"error,omitempty" json:"error,omitempty"`
	RejectedAt time.Time           `bson:"rejected_at" json:"rejected_at"`
}

// KeepZatcaRejectionInChain records a submission ZATCA rejected so the
// store's chain continues from it, and gives the document a new UUID: the
// corrected document goes to ZATCA as a new submission with a new UUID, ICV
// and hash (DTG FAQ p.77-78). It returns the UUID the document carries from
// now on (the old one if recording failed, so nothing else changes).
// Callers hold the store's "zatca" queue.
func KeepZatcaRejectionInChain(storeID *primitive.ObjectID, rejection ZatcaRejection) string {
	if storeID == nil || rejection.Hash == "" || rejection.ICV <= 0 {
		return rejection.UUID
	}
	storeDB := db.GetDB("store_" + storeID.Hex())
	ctx, cancel := context.WithTimeout(context.Background(), zatcaChainQueryTimeout)
	defer cancel()
	if rejection.RejectedAt.IsZero() {
		rejection.RejectedAt = time.Now()
	}
	if _, err := storeDB.Collection(zatcaRejectedCollection).InsertOne(ctx, rejection); err != nil {
		log.Printf("[ZATCA] store %s: keeping rejected %s %s (ICV %d) in the chain failed: %v",
			storeID.Hex(), rejection.Collection, rejection.Code, rejection.ICV, err)
		return rejection.UUID
	}
	newUUID := uuid.New().String()
	if rejection.DocID != nil && !rejection.DocID.IsZero() {
		if _, err := storeDB.Collection(rejection.Collection).UpdateOne(ctx,
			bson.M{"_id": rejection.DocID}, bson.M{"$set": bson.M{"uuid": newUUID}}); err != nil {
			log.Printf("[ZATCA] store %s: new UUID for rejected %s %s not saved: %v",
				storeID.Hex(), rejection.Collection, rejection.Code, err)
		}
	}
	return newUUID
}

// The chain lookups only read, so a lookup that fails on a slow or briefly
// unreachable database is safe to repeat: nothing is reported or saved until
// it succeeds, and the ICV/PIH come from what is stored, not from a counter.
var (
	zatcaChainQueryTimeout = 20 * time.Second
	zatcaChainAttempts     = 3
	zatcaChainRetryDelay   = time.Second
	nextZatcaChainLinkOnce = nextZatcaChainLink
)

// isTransientDBError reports whether err is a database timeout or a dropped
// connection: worth another try, unlike a bad query or a decode error.
func isTransientDBError(err error) bool {
	return err != nil && (mongo.IsTimeout(err) || mongo.IsNetworkError(err) ||
		errors.Is(err, context.DeadlineExceeded))
}

// NextZatcaChainLink returns the ICV and PIH for the store's next ZATCA
// document, assigned when it is reported (not when it is saved), so the chain
// follows the order documents reach ZATCA. The ICV is one past the highest the
// store sent, of any document type; the PIH is the hash of the document it
// sent last, of any type (ZATCA's hash of "0" for the first one). Documents
// ZATCA rejected count too (zatcaRejectedCollection).
// Documents reported before the shared counter existed have no zatca.icv:
// their invoice_count_value is what went to ZATCA, so the counter continues
// above those too. Callers hold the store's "zatca" queue.
// A lookup that times out or loses its connection is tried again (up to
// zatcaChainAttempts in all) before the report gives up.
func NextZatcaChainLink(storeID string) (link ZatcaChainLink, err error) {
	for attempt := 1; ; attempt++ {
		link, err = nextZatcaChainLinkOnce(storeID)
		if err == nil || attempt >= zatcaChainAttempts || !isTransientDBError(err) {
			return link, err
		}
		log.Printf("[ZATCA] store %s: finding the last reported document failed (attempt %d of %d), retrying: %v",
			storeID, attempt, zatcaChainAttempts, err)
		time.Sleep(zatcaChainRetryDelay * time.Duration(attempt))
	}
}

func nextZatcaChainLink(storeID string) (link ZatcaChainLink, err error) {
	storeDB := db.GetDB("store_" + storeID)

	projection := bson.M{"hash": 1, "invoice_count_value": 1, "zatca.icv": 1, "zatca.reporting_passed_at": 1}
	findOne := func(collection string, filter bson.M, sort bson.D) (*zatcaChainDoc, error) {
		// each lookup gets its own deadline, so one slow read does not
		// starve the ones after it
		ctx, cancel := context.WithTimeout(context.Background(), zatcaChainQueryTimeout)
		defer cancel()
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
			// zatca.icv is the same (missing) on every match: sorting on it
			// too lets the second zatcaChainIndexes index give the order
			bson.D{{Key: "zatca.icv", Value: -1}, {Key: "invoice_count_value", Value: -1}})
		if err != nil {
			return link, err
		}
		if doc != nil && doc.InvoiceCountValue > maxICV {
			maxICV = doc.InvoiceCountValue
		}
	}

	// submissions ZATCA rejected stay in the chain
	lastHash := ""
	var lastAt time.Time
	if last != nil {
		lastHash, lastAt = last.Hash, *last.Zatca.ReportedAt
	}
	findRejected := func(sort bson.D) (*ZatcaRejection, error) {
		ctx, cancel := context.WithTimeout(context.Background(), zatcaChainQueryTimeout)
		defer cancel()
		var r ZatcaRejection
		err := storeDB.Collection(zatcaRejectedCollection).FindOne(ctx, bson.M{},
			options.FindOne().SetSort(sort)).Decode(&r)
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &r, nil
	}
	if r, err := findRejected(bson.D{{Key: "icv", Value: -1}}); err != nil {
		return link, err
	} else if r != nil && r.ICV > maxICV {
		maxICV = r.ICV
	}
	if r, err := findRejected(bson.D{{Key: "rejected_at", Value: -1}}); err != nil {
		return link, err
	} else if r != nil && r.Hash != "" && (lastHash == "" || r.RejectedAt.After(lastAt)) {
		lastHash = r.Hash
	}

	link.ICV = maxICV + 1
	if lastHash != "" {
		link.PIH = lastHash
	} else if link.PIH, err = GenerateInvoiceHash("0"); err != nil {
		return link, err
	}
	return link, nil
}

// zatcaXMLPath is where a document's unsigned XML is written for the signing
// script. Stores number their documents independently, so two stores' first
// invoices share a code: the store id keeps one from overwriting the other.
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
