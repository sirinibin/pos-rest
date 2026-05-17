package models

import (
	"context"
	"log"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIVendorPerformance holds aggregated purchase analytics per vendor.
// Period: "30d" | "90d" | "all"
// Collection: bi_vendor_performance
type BIVendorPerformance struct {
	ID                    primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID               primitive.ObjectID `json:"store_id" bson:"store_id"`
	VendorID              primitive.ObjectID `json:"vendor_id" bson:"vendor_id"`
	VendorName            string             `json:"vendor_name" bson:"vendor_name"`
	Period                string             `json:"period" bson:"period"` // "30d" | "90d" | "all"
	PurchaseCount         int64              `json:"purchase_count" bson:"purchase_count"`
	TotalPurchased        float64            `json:"total_purchased" bson:"total_purchased"`
	TotalPaid             float64            `json:"total_paid" bson:"total_paid"`
	Outstanding           float64            `json:"outstanding" bson:"outstanding"`
	VatPaid               float64            `json:"vat_paid" bson:"vat_paid"`
	LastPurchaseDate      *time.Time         `json:"last_purchase_date" bson:"last_purchase_date"`
	DaysSinceLastPurchase int                `json:"days_since_last_purchase" bson:"days_since_last_purchase"`
	RankBySpend           int                `json:"rank_by_spend" bson:"rank_by_spend"`
	UpdatedAt             time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBIVendorPerformance returns vendor performance ranked by total purchased.
func (store *Store) GetBIVendorPerformance(period string, limit int) ([]BIVendorPerformance, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	if period == "" {
		period = "30d"
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_vendor_performance")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collection is store-scoped; no store_id filter to avoid ObjectId/string type mismatch.
	filter := bson.M{"period": period}
	opts := options.Find().
		SetSort(bson.D{{Key: "rank_by_spend", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []BIVendorPerformance
	cursor.All(ctx, &results)
	return results, nil
}

// UpsertBIVendorPerformance computes and upserts vendor performance for the given period.
func UpsertBIVendorPerformance(storeID primitive.ObjectID, period string) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_vendor_performance")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	purchaseColl := db.GetDB("store_" + storeID.Hex()).Collection("purchase")

	match := bson.M{
		"store_id":  storeID,
		"deleted":   bson.M{"$ne": true},
		"vendor_id": bson.M{"$ne": nil},
	}
	if period == "30d" {
		match["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -30)}
	} else if period == "90d" {
		match["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -90)}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":                "$vendor_id",
			"vendor_name":        bson.M{"$first": "$vendor_name"},
			"purchase_count":     bson.M{"$sum": 1},
			"total_purchased":    bson.M{"$sum": "$net_total"},
			"total_paid":         bson.M{"$sum": "$total_payment_received"},
			"vat_paid":           bson.M{"$sum": "$vat_price"},
			"outstanding":        bson.M{"$sum": "$balance_amount"},
			"last_purchase_date": bson.M{"$max": "$date"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total_purchased", Value: -1}}}},
		{{Key: "$limit", Value: 500}},
	}

	cur, err := purchaseColl.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	var rows []bson.M
	cur.All(ctx, &rows)

	// Delete old data for this period (no store_id filter — collection is store-scoped)
	collection.DeleteMany(ctx, bson.M{"period": period})

	now := time.Now()
	for rank, row := range rows {
		vendorID, _ := row["_id"].(primitive.ObjectID)

		var lastPurchaseDate *time.Time
		daysSince := 0
		if dt, ok := row["last_purchase_date"].(primitive.DateTime); ok {
			t := dt.Time()
			lastPurchaseDate = &t
			daysSince = int(now.Sub(t).Hours() / 24)
		}

		doc := BIVendorPerformance{
			StoreID:               storeID,
			VendorID:              vendorID,
			VendorName:            toString(row["vendor_name"]),
			Period:                period,
			PurchaseCount:         toInt64(row["purchase_count"]),
			TotalPurchased:        toFloat64(row["total_purchased"]),
			TotalPaid:             toFloat64(row["total_paid"]),
			Outstanding:           toFloat64(row["outstanding"]),
			VatPaid:               toFloat64(row["vat_paid"]),
			LastPurchaseDate:      lastPurchaseDate,
			DaysSinceLastPurchase: daysSince,
			RankBySpend:           rank + 1,
			UpdatedAt:             now,
		}
		collection.InsertOne(ctx, doc)
	}

	log.Printf("[BI] vendor_performance upsert done — period=%s store=%s rows=%d",
		period, storeID.Hex(), len(rows))
	return nil
}

// RunBIVendorPerformanceUpdate refreshes all periods for a store.
func RunBIVendorPerformanceUpdate(storeID primitive.ObjectID) {
	for _, period := range []string{"30d", "90d", "all"} {
		if err := UpsertBIVendorPerformance(storeID, period); err != nil {
			log.Printf("[BI] vendor_performance error period=%s store=%s: %v",
				period, storeID.Hex(), err)
		}
	}
}
