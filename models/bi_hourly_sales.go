package models

import (
	"context"
	"fmt"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// HourlySalesStat holds aggregated sales for a single hour of the day.
type HourlySalesStat struct {
	Hour        int     `json:"hour"`         // 0-23
	HourLabel   string  `json:"hour_label"`   // "9 AM", "2 PM"
	OrderCount  int64   `json:"order_count"`
	GrossSales  float64 `json:"gross_sales"`
	AvgOrderVal float64 `json:"avg_order_value"`
}

// GetHourlySales aggregates sales by hour of day for the given UTC range.
// tzOffset = CountryTimezoneOffset(store.CountryCode) — negative for east-of-UTC.
func (store *Store) GetHourlySales(start, end time.Time, tzOffset float64) ([]HourlySalesStat, error) {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	offsetMillis := int64(-tzOffset * 3600 * 1000)

	pipeline := mongo.Pipeline{
		{{"$match", bson.M{
			"deleted":  bson.M{"$ne": true},
			"store_id": store.ID,
			"date":     bson.M{"$gte": start, "$lt": end},
		}}},
		{{"$group", bson.M{
			"_id": bson.M{
				"$hour": bson.M{
					"$add": bson.A{"$date", offsetMillis},
				},
			},
			"order_count": bson.M{"$sum": 1},
			"gross_sales": bson.M{"$sum": "$net_total"},
		}}},
		{{"$sort", bson.M{"_id": 1}}},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	type row struct {
		Hour       int     `bson:"_id"`
		OrderCount int64   `bson:"order_count"`
		GrossSales float64 `bson:"gross_sales"`
	}

	rowMap := map[int]row{}
	for cursor.Next(ctx) {
		var r row
		if err := cursor.Decode(&r); err == nil {
			rowMap[r.Hour] = r
		}
	}

	// Build all 24 hours (zero-fill missing hours)
	results := make([]HourlySalesStat, 24)
	for h := 0; h < 24; h++ {
		r := rowMap[h]
		avg := 0.0
		if r.OrderCount > 0 {
			avg = r.GrossSales / float64(r.OrderCount)
		}
		results[h] = HourlySalesStat{
			Hour:        h,
			HourLabel:   fmt.Sprintf("%s", hourLabel(h)),
			OrderCount:  r.OrderCount,
			GrossSales:  RoundTo2Decimals(r.GrossSales),
			AvgOrderVal: RoundTo2Decimals(avg),
		}
	}
	return results, nil
}

func hourLabel(h int) string {
	switch {
	case h == 0:
		return "12 AM"
	case h < 12:
		return fmt.Sprintf("%d AM", h)
	case h == 12:
		return "12 PM"
	default:
		return fmt.Sprintf("%d PM", h-12)
	}
}
