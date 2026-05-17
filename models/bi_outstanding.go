package models

import (
	"context"
	"log"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIOutstanding represents a single unpaid invoice (AR or AP).
// Type: "AR" = accounts receivable (customer owes), "AP" = accounts payable (we owe vendor).
// Collection: bi_outstanding
type BIOutstanding struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID      primitive.ObjectID `json:"store_id" bson:"store_id"`
	Type         string             `json:"type" bson:"type"` // "AR" | "AP"
	PartyID      primitive.ObjectID `json:"party_id" bson:"party_id"`
	PartyName    string             `json:"party_name" bson:"party_name"`
	PartyPhone   string             `json:"party_phone" bson:"party_phone"`
	InvoiceID    primitive.ObjectID `json:"invoice_id" bson:"invoice_id"`
	InvoiceCode  string             `json:"invoice_code" bson:"invoice_code"`
	InvoiceDate  *time.Time         `json:"invoice_date" bson:"invoice_date"`
	InvoiceTotal float64            `json:"invoice_total" bson:"invoice_total"`
	DueAmount    float64            `json:"due_amount" bson:"due_amount"`
	DaysOld      int                `json:"days_old" bson:"days_old"` // days since invoice date
	UpdatedAt    time.Time          `json:"updated_at" bson:"updated_at"`
}

// BIOutstandingSummary is the top-level response wrapping AR and AP lists with totals.
type BIOutstandingSummary struct {
	Type          string          `json:"type"`
	TotalDue      float64         `json:"total_due"`
	TotalInvoices int             `json:"total_invoices"`
	Items         []BIOutstanding `json:"items"`
}

// GetBIOutstanding returns AR or AP outstanding items, sorted by days_old desc.
func (store *Store) GetBIOutstanding(outstandingType string, limit int) (BIOutstandingSummary, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if outstandingType != "AR" && outstandingType != "AP" {
		outstandingType = "AR"
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_outstanding")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collection is store-scoped; no store_id filter to avoid ObjectId/string type mismatch.
	filter := bson.M{"type": outstandingType}
	opts := options.Find().
		SetSort(bson.D{{Key: "days_old", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return BIOutstandingSummary{}, err
	}
	defer cursor.Close(ctx)

	var items []BIOutstanding
	cursor.All(ctx, &items)

	totalDue := 0.0
	for _, item := range items {
		totalDue += item.DueAmount
	}

	return BIOutstandingSummary{
		Type:          outstandingType,
		TotalDue:      totalDue,
		TotalInvoices: len(items),
		Items:         items,
	}, nil
}

// UpsertBIOutstanding does a full refresh of AR and AP outstanding items.
func UpsertBIOutstanding(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_outstanding")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Clear everything for this store (full refresh — no store_id filter to clear all docs regardless of type)
	collection.DeleteMany(ctx, bson.M{})

	now := time.Now()

	// ── AR: unpaid / partially paid sales orders ───────────────────────────
	ordersColl := db.GetDB("store_" + storeID.Hex()).Collection("order")
	arCursor, err := ordersColl.Find(ctx, bson.M{
		"store_id":       storeID,
		"deleted":        bson.M{"$ne": true},
		"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
		"customer_id":    bson.M{"$ne": nil},
	}, options.Find().SetProjection(bson.M{
		"_id": 1, "code": 1, "date": 1, "customer_id": 1,
		"customer_name": 1, "net_total": 1, "balance_amount": 1,
	}))
	if err == nil {
		defer arCursor.Close(ctx)
		for arCursor.Next(ctx) {
			var order bson.M
			if err := arCursor.Decode(&order); err != nil {
				continue
			}
			invoiceID, _ := order["_id"].(primitive.ObjectID)
			customerID, _ := order["customer_id"].(primitive.ObjectID)
			dueAmount := toFloat64(order["balance_amount"])
			if dueAmount <= 0 {
				continue
			}

			daysOld := 0
			var invoiceDate *time.Time
			if dt, ok := order["date"].(primitive.DateTime); ok {
				t := dt.Time()
				invoiceDate = &t
				daysOld = int(now.Sub(t).Hours() / 24)
			}

			doc := BIOutstanding{
				StoreID:      storeID,
				Type:         "AR",
				PartyID:      customerID,
				PartyName:    toString(order["customer_name"]),
				InvoiceID:    invoiceID,
				InvoiceCode:  toString(order["code"]),
				InvoiceDate:  invoiceDate,
				InvoiceTotal: toFloat64(order["net_total"]),
				DueAmount:    dueAmount,
				DaysOld:      daysOld,
				UpdatedAt:    now,
			}
			collection.InsertOne(ctx, doc)
		}
	}

	// ── AP: unpaid / partially paid purchases ─────────────────────────────
	purchaseColl := db.GetDB("store_" + storeID.Hex()).Collection("purchase")
	apCursor, err := purchaseColl.Find(ctx, bson.M{
		"store_id":       storeID,
		"deleted":        bson.M{"$ne": true},
		"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
		"vendor_id":      bson.M{"$ne": nil},
	}, options.Find().SetProjection(bson.M{
		"_id": 1, "code": 1, "date": 1, "vendor_id": 1,
		"vendor_name": 1, "net_total": 1, "balance_amount": 1,
	}))
	if err == nil {
		defer apCursor.Close(ctx)
		for apCursor.Next(ctx) {
			var purchase bson.M
			if err := apCursor.Decode(&purchase); err != nil {
				continue
			}
			invoiceID, _ := purchase["_id"].(primitive.ObjectID)
			vendorID, _ := purchase["vendor_id"].(primitive.ObjectID)
			dueAmount := toFloat64(purchase["balance_amount"])
			if dueAmount <= 0 {
				continue
			}

			daysOld := 0
			var invoiceDate *time.Time
			if dt, ok := purchase["date"].(primitive.DateTime); ok {
				t := dt.Time()
				invoiceDate = &t
				daysOld = int(now.Sub(t).Hours() / 24)
			}

			doc := BIOutstanding{
				StoreID:      storeID,
				Type:         "AP",
				PartyID:      vendorID,
				PartyName:    toString(purchase["vendor_name"]),
				InvoiceID:    invoiceID,
				InvoiceCode:  toString(purchase["code"]),
				InvoiceDate:  invoiceDate,
				InvoiceTotal: toFloat64(purchase["net_total"]),
				DueAmount:    dueAmount,
				DaysOld:      daysOld,
				UpdatedAt:    now,
			}
			collection.InsertOne(ctx, doc)
		}
	}

	// Count AR and AP
	arCount, _ := collection.CountDocuments(ctx, bson.M{"type": "AR"})
	apCount, _ := collection.CountDocuments(ctx, bson.M{"type": "AP"})
	log.Printf("[BI] outstanding upsert done — store=%s AR=%d AP=%d", storeID.Hex(), arCount, apCount)
	return nil
}

// RunBIOutstandingUpdate does a full refresh of outstanding for a store.
func RunBIOutstandingUpdate(storeID primitive.ObjectID) {
	if err := UpsertBIOutstanding(storeID); err != nil {
		log.Printf("[BI] outstanding error store=%s: %v", storeID.Hex(), err)
	}
}
