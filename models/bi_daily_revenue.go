package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// DailyRevenueStat holds aggregated sales for a single calendar day.
type DailyRevenueStat struct {
	Date        string  `json:"date"`         // "2026-01-15"
	DayOfWeek   string  `json:"day_of_week"`  // "Monday"
	OrderCount  int64   `json:"order_count"`
	GrossSales  float64 `json:"gross_sales"`  // sum of order net_total
	ReturnCount int64   `json:"return_count"` // returns processed on that day
	ReturnAmount float64 `json:"return_amount"`
	NetSales    float64 `json:"net_sales"` // gross_sales − return_amount
}

var dayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// GetDailyRevenue returns day-by-day sales totals for the given UTC period.
// tzOffset is CountryTimezoneOffset(store.CountryCode) — negative for east-of-UTC.
func (store *Store) GetDailyRevenue(start, end time.Time, tzOffset float64) ([]DailyRevenueStat, error) {
	col := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shift UTC timestamps to local time for day-boundary grouping.
	// For UTC+3 (tzOffset=-3): localMs = UTC + 3h = UTC - tzOffset*h
	offsetMillis := int64(-tzOffset * 3600 * 1000)

	pipeline := mongo.Pipeline{
		{{"$match", bson.M{
			"deleted": bson.M{"$ne": true},
			"date":    bson.M{"$gte": start, "$lt": end},
			"store_id": store.ID,
		}}},
		{{"$group", bson.M{
			"_id": bson.M{
				"$dateToString": bson.M{
					"format": "%Y-%m-%d",
					"date": bson.M{
						"$add": bson.A{"$date", offsetMillis},
					},
				},
			},
			"order_count":   bson.M{"$sum": 1},
			"gross_sales":   bson.M{"$sum": "$net_total"},
			"return_count":  bson.M{"$sum": "$return_count"},
			"return_amount": bson.M{"$sum": "$return_amount"},
		}}},
		{{"$sort", bson.M{"_id": 1}}},
	}

	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rows []struct {
		Date         string  `bson:"_id"`
		OrderCount   int64   `bson:"order_count"`
		GrossSales   float64 `bson:"gross_sales"`
		ReturnCount  int64   `bson:"return_count"`
		ReturnAmount float64 `bson:"return_amount"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	result := make([]DailyRevenueStat, 0, len(rows))
	for _, row := range rows {
		// Parse date to get day-of-week
		t, err := time.Parse("2006-01-02", row.Date)
		dow := ""
		if err == nil {
			dow = dayNames[t.Weekday()]
		}
		netSales := row.GrossSales - row.ReturnAmount
		result = append(result, DailyRevenueStat{
			Date:         row.Date,
			DayOfWeek:    dow,
			OrderCount:   row.OrderCount,
			GrossSales:   RoundTo2Decimals(row.GrossSales),
			ReturnCount:  row.ReturnCount,
			ReturnAmount: RoundTo2Decimals(row.ReturnAmount),
			NetSales:     RoundTo2Decimals(netSales),
		})
	}
	return result, nil
}
