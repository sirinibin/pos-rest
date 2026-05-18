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

// BIQuotationConversion holds monthly quotation-to-order conversion analytics.
// Collection: bi_quotation_conversion
type BIQuotationConversion struct {
	ID                  primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID             primitive.ObjectID `json:"store_id" bson:"store_id"`
	Year                int                `json:"year" bson:"year"`
	Month               int                `json:"month" bson:"month"`   // 1-12
	Period              string             `json:"period" bson:"period"` // "2026-05"
	QuotationsCreated   int64              `json:"quotations_created" bson:"quotations_created"`
	QuotationsConverted int64              `json:"quotations_converted" bson:"quotations_converted"`
	ConversionRate      float64            `json:"conversion_rate" bson:"conversion_rate"` // percent 0-100
	TotalQuotedValue    float64            `json:"total_quoted_value" bson:"total_quoted_value"`
	ConvertedValue      float64            `json:"converted_value" bson:"converted_value"`
	LostValue           float64            `json:"lost_value" bson:"lost_value"` // quoted - converted
	UpdatedAt           time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBIQuotationConversion returns monthly quotation conversion data.
func (store *Store) GetBIQuotationConversion(months int) ([]BIQuotationConversion, error) {
	if months <= 0 {
		months = 6
	}
	if months > 36 {
		months = 36
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_quotation_conversion")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Cutoff = start of the month that is (months-1) months ago.
	// e.g. months=6 on May-2026 → cutoff = Dec-2025 start → returns exactly 6 calendar months.
	now := time.Now()
	cutoffMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -(months - 1), 0)
	// Collection is store-scoped; no store_id filter to avoid ObjectId/string type mismatch.
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
	})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []BIQuotationConversion
	cursor.All(ctx, &results)
	return results, nil
}

// UpsertBIQuotationConversion computes and upserts conversion data for year/month.
func UpsertBIQuotationConversion(storeID primitive.ObjectID, year, month int) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_quotation_conversion")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	period := fmt.Sprintf("%04d-%02d", year, month)
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	quotationColl := db.GetDB("store_" + storeID.Hex()).Collection("quotation")

	// All quotations created in this month
	totalPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
			"date":     bson.M{"$gte": startDate, "$lt": endDate},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":         nil,
			"count":       bson.M{"$sum": 1},
			"total_value": bson.M{"$sum": "$net_total"},
		}}},
	}

	cur, err := quotationColl.Aggregate(ctx, totalPipeline)
	if err != nil {
		return err
	}

	var totalCreated int64
	var totalValue float64
	if cur.Next(ctx) {
		var r bson.M
		cur.Decode(&r)
		totalCreated = toInt64(r["count"])
		totalValue = toFloat64(r["total_value"])
	}
	cur.Close(ctx)

	// Converted quotations (order_id is set = linked to an order)
	convertedPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
			"date":     bson.M{"$gte": startDate, "$lt": endDate},
			"order_id": bson.M{"$ne": nil, "$exists": true},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":             nil,
			"count":           bson.M{"$sum": 1},
			"converted_value": bson.M{"$sum": "$net_total"},
		}}},
	}

	cur2, err := quotationColl.Aggregate(ctx, convertedPipeline)
	if err != nil {
		return err
	}
	defer cur2.Close(ctx)

	var converted int64
	var convertedValue float64
	if cur2.Next(ctx) {
		var r bson.M
		cur2.Decode(&r)
		converted = toInt64(r["count"])
		convertedValue = toFloat64(r["converted_value"])
	}

	conversionRate := float64(0)
	if totalCreated > 0 {
		conversionRate = float64(converted) / float64(totalCreated) * 100
	}

	doc := BIQuotationConversion{
		StoreID:             storeID,
		Year:                year,
		Month:               month,
		Period:              period,
		QuotationsCreated:   totalCreated,
		QuotationsConverted: converted,
		ConversionRate:      conversionRate,
		TotalQuotedValue:    totalValue,
		ConvertedValue:      convertedValue,
		LostValue:           totalValue - convertedValue,
		UpdatedAt:           time.Now(),
	}

	// Use year+month as upsert key (no store_id — collection is store-scoped)
	upsertOpts := options.Replace().SetUpsert(true)
	_, err = collection.ReplaceOne(ctx,
		bson.M{"year": year, "month": month},
		doc, upsertOpts)

	log.Printf("[BI] quotation_conversion upsert done — period=%s store=%s created=%d converted=%d rate=%.1f%%",
		period, storeID.Hex(), totalCreated, converted, conversionRate)
	return err
}

// RunBIQuotationConversionUpdate refreshes quotation conversion for current and previous N months.
func RunBIQuotationConversionUpdate(storeID primitive.ObjectID, monthsBack int) {
	now := time.Now()
	for i := 0; i <= monthsBack; i++ {
		t := now.AddDate(0, -i, 0)
		if err := UpsertBIQuotationConversion(storeID, t.Year(), int(t.Month())); err != nil {
			log.Printf("[BI] quotation_conversion error %04d-%02d store=%s: %v",
				t.Year(), int(t.Month()), storeID.Hex(), err)
		}
	}
}
