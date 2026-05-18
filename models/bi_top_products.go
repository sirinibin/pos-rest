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

// BITopProduct holds aggregated product performance for a rolling window.
// Period: "30d" | "90d" | "all"
// Collection: bi_top_products
type BITopProduct struct {
	ID            primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID       primitive.ObjectID `json:"store_id" bson:"store_id"`
	ProductID     primitive.ObjectID `json:"product_id" bson:"product_id"`
	ProductName   string             `json:"product_name" bson:"product_name"`
	PartNumber    string             `json:"part_number" bson:"part_number"`
	ItemCode      string             `json:"item_code" bson:"item_code"`
	CategoryName  string             `json:"category_name" bson:"category_name"`
	Period        string             `json:"period" bson:"period"` // "30d" | "90d" | "all"
	UnitsSold     float64            `json:"units_sold" bson:"units_sold"`
	Revenue       float64            `json:"revenue" bson:"revenue"`
	Profit        float64            `json:"profit" bson:"profit"`
	MarginPercent float64            `json:"margin_percent" bson:"margin_percent"` // profit/revenue*100
	ReturnCount   int64              `json:"return_count" bson:"return_count"`
	ReturnAmount  float64            `json:"return_amount" bson:"return_amount"`
	OrderCount    int64              `json:"order_count" bson:"order_count"`
	RankByRevenue int                `json:"rank_by_revenue" bson:"rank_by_revenue"`
	RankByProfit  int                `json:"rank_by_profit" bson:"rank_by_profit"`
	UpdatedAt     time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBITopProducts returns top products for a store for the given period and limit.
// period: "30d" | "90d" | "all"
func (store *Store) GetBITopProducts(period string, limit int) ([]BITopProduct, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	if period == "" {
		period = "30d"
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_top_products")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collection is store-scoped; no store_id filter to avoid ObjectId/string type mismatch.
	filter := bson.M{"period": period}
	opts := options.Find().
		SetSort(bson.D{{Key: "rank_by_revenue", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []BITopProduct
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// UpsertBITopProducts computes and upserts top products for the given period.
func UpsertBITopProducts(storeID primitive.ObjectID, period string) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_top_products")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Build date filter for the period
	matchDate := bson.M{}
	if period == "30d" {
		matchDate["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -30)}
	} else if period == "90d" {
		matchDate["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -90)}
	}
	// "all" — no date filter

	salesColl := db.GetDB("store_" + storeID.Hex()).Collection("product_sales_history")

	match := bson.M{"store_id": storeID}
	for k, v := range matchDate {
		match[k] = v
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":         "$product_id",
			"units_sold":  bson.M{"$sum": "$quantity"},
			"revenue":     bson.M{"$sum": "$net_price"},
			"profit":      bson.M{"$sum": "$profit"},
			"order_count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "revenue", Value: -1}}}},
		{{Key: "$limit", Value: 200}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "product",
			"localField":   "_id",
			"foreignField": "_id",
			"as":           "product_doc",
		}}},
		{{Key: "$addFields", Value: bson.M{
			"product_name":  bson.M{"$ifNull": []interface{}{bson.M{"$arrayElemAt": []interface{}{"$product_doc.name", 0}}, ""}},
			"part_number":   bson.M{"$ifNull": []interface{}{bson.M{"$arrayElemAt": []interface{}{"$product_doc.part_number", 0}}, ""}},
			"item_code":     bson.M{"$ifNull": []interface{}{bson.M{"$arrayElemAt": []interface{}{"$product_doc.item_code", 0}}, ""}},
			"category_name": bson.M{"$ifNull": []interface{}{bson.M{"$arrayElemAt": []interface{}{"$product_doc.category_name", 0}}, ""}},
		}}},
	}

	cur, err := salesColl.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	var rows []bson.M
	cur.All(ctx, &rows)

	// Build return stats map from sales_return_history
	returnMap := buildReturnMap(ctx, storeID, period)

	// Delete old data for this period (no store_id filter — collection is store-scoped)
	collection.DeleteMany(ctx, bson.M{"period": period})

	now := time.Now()
	for rank, row := range rows {
		productID, _ := row["_id"].(primitive.ObjectID)
		revenue := toFloat64(row["revenue"])
		profit := toFloat64(row["profit"])
		margin := float64(0)
		if revenue > 0 {
			margin = profit / revenue * 100
		}

		rt := returnMap[productID.Hex()]
		doc := BITopProduct{
			StoreID:       storeID,
			ProductID:     productID,
			ProductName:   toString(row["product_name"]),
			PartNumber:    toString(row["part_number"]),
			ItemCode:      toString(row["item_code"]),
			Period:        period,
			UnitsSold:     toFloat64(row["units_sold"]),
			Revenue:       revenue,
			Profit:        profit,
			MarginPercent: margin,
			ReturnCount:   rt.count,
			ReturnAmount:  rt.amount,
			OrderCount:    toInt64(row["order_count"]),
			RankByRevenue: rank + 1,
			RankByProfit:  rank + 1, // simplified; could re-sort by profit
			UpdatedAt:     now,
		}
		collection.InsertOne(ctx, doc)
	}

	log.Printf("[BI] top_products upsert done — period=%s store=%s rows=%d", period, storeID.Hex(), len(rows))
	return nil
}

type returnStat struct {
	count  int64
	amount float64
}

func buildReturnMap(ctx context.Context, storeID primitive.ObjectID, period string) map[string]returnStat {
	m := make(map[string]returnStat)
	coll := db.GetDB("store_" + storeID.Hex()).Collection("product_sales_return_history")

	match := bson.M{"store_id": storeID}
	if period == "30d" {
		match["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -30)}
	} else if period == "90d" {
		match["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -90)}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":    "$product_id",
			"count":  bson.M{"$sum": 1},
			"amount": bson.M{"$sum": "$net_price"},
		}}},
	}

	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return m
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var r bson.M
		cur.Decode(&r)
		id, _ := r["_id"].(primitive.ObjectID)
		m[id.Hex()] = returnStat{
			count:  toInt64(r["count"]),
			amount: toFloat64(r["amount"]),
		}
	}
	return m
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// RunBITopProductsUpdate refreshes all periods for a store.
func RunBITopProductsUpdate(storeID primitive.ObjectID) {
	for _, period := range []string{"30d", "90d", "all"} {
		if err := UpsertBITopProducts(storeID, period); err != nil {
			log.Printf("[BI] top_products error period=%s store=%s: %v", period, storeID.Hex(), err)
		}
	}
}
