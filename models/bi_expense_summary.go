package models

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIExpenseSummary holds monthly expense totals broken down by category.
// Collection: bi_expense_summary
type BIExpenseSummary struct {
	ID               primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID          primitive.ObjectID `json:"store_id" bson:"store_id"`
	Year             int                `json:"year" bson:"year"`
	Month            int                `json:"month" bson:"month"`                 // 1-12
	Period           string             `json:"period" bson:"period"`               // "2026-05"
	CategoryName     string             `json:"category_name" bson:"category_name"` // "General" if none
	TotalAmount      float64            `json:"total_amount" bson:"total_amount"`
	VatAmount        float64            `json:"vat_amount" bson:"vat_amount"`
	TransactionCount int64              `json:"transaction_count" bson:"transaction_count"`
	UpdatedAt        time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBIExpenseSummary returns monthly expense summary for the given number of months.
func (store *Store) GetBIExpenseSummary(months int) ([]BIExpenseSummary, error) {
	if months <= 0 {
		months = 6
	}
	if months > 36 {
		months = 36
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_expense_summary")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cutoff = start of the month that is (months-1) months ago.
	// e.g. months=6 on May-2026 → cutoff = Dec-2025 start → returns Dec,Jan,Feb,Mar,Apr,May (6 months).
	now := time.Now()
	cutoffMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)
	filter := bson.M{
		"$or": bson.A{
			bson.M{"year": bson.M{"$gt": cutoffMonth.Year()}},
			bson.M{
				"year":  cutoffMonth.Year(),
				"month": bson.M{"$gte": int(cutoffMonth.Month())},
			},
		},
	}

	opts := options.Find().SetSort(bson.D{
		{Key: "year", Value: 1},
		{Key: "month", Value: 1},
		{Key: "category_name", Value: 1},
	})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []BIExpenseSummary
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// UpsertBIExpenseSummary computes and upserts expense summary for the given year/month.
func UpsertBIExpenseSummary(storeID primitive.ObjectID, year, month int) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_expense_summary")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	period := fmt.Sprintf("%04d-%02d", year, month)
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	expenseColl := db.GetDB("store_" + storeID.Hex()).Collection("expense")

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
			"date":     bson.M{"$gte": startDate, "$lt": endDate},
		}}},
		{{Key: "$unwind", Value: bson.M{
			"path":                       "$category_name",
			"preserveNullAndEmptyArrays": true,
		}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"category": bson.M{
					"$ifNull": bson.A{"$category_name", "General"},
				},
			},
			"total_amount":      bson.M{"$sum": "$amount"},
			"vat_amount":        bson.M{"$sum": "$vat_price"},
			"transaction_count": bson.M{"$sum": 1},
		}}},
	}

	cur, err := expenseColl.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	var rows []bson.M
	cur.All(ctx, &rows)

	// Delete existing for this year/month (no store_id filter — collection is store-scoped)
	collection.DeleteMany(ctx, bson.M{"year": year, "month": month})

	now := time.Now()
	for _, row := range rows {
		idMap, _ := row["_id"].(bson.M)
		catName := "General"
		if idMap != nil {
			if cn, ok := idMap["category"].(string); ok && cn != "" {
				catName = cn
			}
		}

		doc := BIExpenseSummary{
			StoreID:          storeID,
			Year:             year,
			Month:            month,
			Period:           period,
			CategoryName:     catName,
			TotalAmount:      toFloat64(row["total_amount"]),
			VatAmount:        toFloat64(row["vat_amount"]),
			TransactionCount: toInt64(row["transaction_count"]),
			UpdatedAt:        now,
		}
		collection.InsertOne(ctx, doc)
	}

	log.Printf("[BI] expense_summary upsert done — period=%s store=%s rows=%d", period, storeID.Hex(), len(rows))
	return nil
}

// RunBIExpenseSummaryUpdate refreshes expense summary for current and previous N months.
func RunBIExpenseSummaryUpdate(storeID primitive.ObjectID, monthsBack int) {
	now := time.Now()
	for i := 0; i <= monthsBack; i++ {
		t := now.AddDate(0, -i, 0)
		if err := UpsertBIExpenseSummary(storeID, t.Year(), int(t.Month())); err != nil {
			log.Printf("[BI] expense_summary error %04d-%02d store=%s: %v",
				t.Year(), int(t.Month()), storeID.Hex(), err)
		}
	}
}
