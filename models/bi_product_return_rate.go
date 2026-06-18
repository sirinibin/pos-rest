package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ProductReturnRate holds return metrics for a single product.
type ProductReturnRate struct {
	ProductID    string  `json:"product_id"`
	ProductName  string  `json:"product_name"`
	ProductCode  string  `json:"product_code"`
	UnitsSold    float64 `json:"units_sold"`
	UnitsReturned float64 `json:"units_returned"`
	ReturnRate   float64 `json:"return_rate_pct"` // (returned/sold)*100
	SalesAmount  float64 `json:"sales_amount"`
	ReturnAmount float64 `json:"return_amount"`
}

// GetProductReturnRates returns products ranked by return rate for a store.
// Joins orders + sales_returns aggregated by product.
func (store *Store) GetProductReturnRates(limit int) ([]ProductReturnRate, error) {
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbName := "store_" + store.ID.Hex()

	// ── Aggregate sales quantities by product ─────────────────────────────────
	salesCol := db.GetDB(dbName).Collection("order_item")
	salesPipeline := mongo.Pipeline{
		{{"$match", bson.M{"store_id": store.ID, "deleted": bson.M{"$ne": true}}}},
		{{"$group", bson.M{
			"_id":          "$product_id",
			"product_name": bson.M{"$first": "$product_name"},
			"product_code": bson.M{"$first": "$product_code"},
			"units_sold":   bson.M{"$sum": "$quantity"},
			"sales_amount": bson.M{"$sum": "$net_price"},
		}}},
	}
	salesCursor, err := salesCol.Aggregate(ctx, salesPipeline)
	if err != nil {
		return nil, err
	}
	defer salesCursor.Close(ctx)

	type salesRow struct {
		ProductID   primitive.ObjectID `bson:"_id"`
		ProductName string             `bson:"product_name"`
		ProductCode string             `bson:"product_code"`
		UnitsSold   float64            `bson:"units_sold"`
		SalesAmount float64            `bson:"sales_amount"`
	}
	salesMap := map[string]salesRow{}
	var sr salesRow
	for salesCursor.Next(ctx) {
		if err := salesCursor.Decode(&sr); err == nil {
			salesMap[sr.ProductID.Hex()] = sr
		}
	}

	// ── Aggregate return quantities by product ────────────────────────────────
	retCol := db.GetDB(dbName).Collection("sales_return_item")
	retPipeline := mongo.Pipeline{
		{{"$match", bson.M{"store_id": store.ID, "deleted": bson.M{"$ne": true}}}},
		{{"$group", bson.M{
			"_id":           "$product_id",
			"units_returned": bson.M{"$sum": "$quantity"},
			"return_amount": bson.M{"$sum": "$net_price"},
		}}},
	}
	retCursor, err := retCol.Aggregate(ctx, retPipeline)
	if err != nil {
		return nil, err
	}
	defer retCursor.Close(ctx)

	type retRow struct {
		ProductID     primitive.ObjectID `bson:"_id"`
		UnitsReturned float64            `bson:"units_returned"`
		ReturnAmount  float64            `bson:"return_amount"`
	}
	retMap := map[string]retRow{}
	var rr retRow
	for retCursor.Next(ctx) {
		if err := retCursor.Decode(&rr); err == nil {
			retMap[rr.ProductID.Hex()] = rr
		}
	}

	// ── Join and compute return rate ──────────────────────────────────────────
	var results []ProductReturnRate
	for pid, s := range salesMap {
		if s.UnitsSold <= 0 {
			continue
		}
		r := retMap[pid]
		rate := 0.0
		if s.UnitsSold > 0 {
			rate = (r.UnitsReturned / s.UnitsSold) * 100
		}
		if r.UnitsReturned <= 0 {
			continue // only include products with at least 1 return
		}
		results = append(results, ProductReturnRate{
			ProductID:     pid,
			ProductName:   s.ProductName,
			ProductCode:   s.ProductCode,
			UnitsSold:     RoundTo2Decimals(s.UnitsSold),
			UnitsReturned: RoundTo2Decimals(r.UnitsReturned),
			ReturnRate:    RoundTo2Decimals(rate),
			SalesAmount:   RoundTo2Decimals(s.SalesAmount),
			ReturnAmount:  RoundTo2Decimals(r.ReturnAmount),
		})
	}

	// Sort by return rate descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].ReturnRate > results[i].ReturnRate {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
