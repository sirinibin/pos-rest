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

// BITopCustomer holds aggregated customer spending analytics for a rolling window.
// Period: "30d" | "90d" | "all"
// Collection: bi_top_customers
type BITopCustomer struct {
	ID                 primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID            primitive.ObjectID `json:"store_id" bson:"store_id"`
	CustomerID         primitive.ObjectID `json:"customer_id" bson:"customer_id"`
	CustomerName       string             `json:"customer_name" bson:"customer_name"`
	CustomerPhone      string             `json:"customer_phone" bson:"customer_phone"`
	Period             string             `json:"period" bson:"period"` // "30d" | "90d" | "all"
	OrderCount         int64              `json:"order_count" bson:"order_count"`
	TotalSpend         float64            `json:"total_spend" bson:"total_spend"`
	AvgOrderValue      float64            `json:"avg_order_value" bson:"avg_order_value"`
	TotalProfit        float64            `json:"total_profit" bson:"total_profit"`
	LastOrderDate      *time.Time         `json:"last_order_date" bson:"last_order_date"`
	DaysSinceLastOrder int                `json:"days_since_last_order" bson:"days_since_last_order"`
	IsNew              bool               `json:"is_new" bson:"is_new"` // first order within period
	OutstandingBalance float64            `json:"outstanding_balance" bson:"outstanding_balance"`
	RankBySpend        int                `json:"rank_by_spend" bson:"rank_by_spend"`
	UpdatedAt          time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBITopCustomers returns top customers ranked by total spend.
func (store *Store) GetBITopCustomers(period string, limit int) ([]BITopCustomer, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	if period == "" {
		period = "30d"
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_top_customers")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := bson.M{"store_id": store.ID, "period": period}
	opts := options.Find().
		SetSort(bson.D{{Key: "rank_by_spend", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []BITopCustomer
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// UpsertBITopCustomers computes and upserts top customer records for the given period.
func UpsertBITopCustomers(storeID primitive.ObjectID, period string) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_top_customers")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	ordersColl := db.GetDB("store_" + storeID.Hex()).Collection("order")

	match := bson.M{
		"store_id":    storeID,
		"deleted":     bson.M{"$ne": true},
		"customer_id": bson.M{"$ne": nil},
	}
	if period == "30d" {
		match["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -30)}
	} else if period == "90d" {
		match["date"] = bson.M{"$gte": time.Now().AddDate(0, 0, -90)}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.M{
			"_id":             "$customer_id",
			"customer_name":   bson.M{"$first": "$customer_name"},
			"order_count":     bson.M{"$sum": 1},
			"total_spend":     bson.M{"$sum": "$net_total"},
			"total_profit":    bson.M{"$sum": "$profit"},
			"last_order_date": bson.M{"$max": "$date"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total_spend", Value: -1}}}},
		{{Key: "$limit", Value: 500}},
	}

	cur, err := ordersColl.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	var rows []bson.M
	cur.All(ctx, &rows)

	// Build outstanding balance map from unpaid orders
	outstandingMap := buildCustomerOutstandingMap(ctx, storeID)
	// Build new-customer set (first ever order in period)
	newCustomers := buildNewCustomerSet(ctx, storeID, period)

	// Delete old records for this period
	collection.DeleteMany(ctx, bson.M{"store_id": storeID, "period": period})

	now := time.Now()
	for rank, row := range rows {
		customerID, _ := row["_id"].(primitive.ObjectID)
		totalSpend := toFloat64(row["total_spend"])
		orderCount := toInt64(row["order_count"])
		avgOrderValue := float64(0)
		if orderCount > 0 {
			avgOrderValue = totalSpend / float64(orderCount)
		}

		var lastOrderDate *time.Time
		daysSince := 0
		if t, ok := row["last_order_date"].(primitive.DateTime); ok {
			dt := t.Time()
			lastOrderDate = &dt
			daysSince = int(now.Sub(dt).Hours() / 24)
		}

		doc := BITopCustomer{
			StoreID:            storeID,
			CustomerID:         customerID,
			CustomerName:       toString(row["customer_name"]),
			Period:             period,
			OrderCount:         orderCount,
			TotalSpend:         totalSpend,
			AvgOrderValue:      avgOrderValue,
			TotalProfit:        toFloat64(row["total_profit"]),
			LastOrderDate:      lastOrderDate,
			DaysSinceLastOrder: daysSince,
			IsNew:              newCustomers[customerID.Hex()],
			OutstandingBalance: outstandingMap[customerID.Hex()],
			RankBySpend:        rank + 1,
			UpdatedAt:          now,
		}
		collection.InsertOne(ctx, doc)
	}

	log.Printf("[BI] top_customers upsert done — period=%s store=%s rows=%d", period, storeID.Hex(), len(rows))
	return nil
}

// buildCustomerOutstandingMap returns {customer_id_hex: balance_amount} for unpaid orders.
func buildCustomerOutstandingMap(ctx context.Context, storeID primitive.ObjectID) map[string]float64 {
	m := make(map[string]float64)
	coll := db.GetDB("store_" + storeID.Hex()).Collection("order")

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id":       storeID,
			"deleted":        bson.M{"$ne": true},
			"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
			"customer_id":    bson.M{"$ne": nil},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":     "$customer_id",
			"balance": bson.M{"$sum": "$balance_amount"},
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
		m[id.Hex()] = toFloat64(r["balance"])
	}
	return m
}

// buildNewCustomerSet returns set of customer IDs whose first-ever order is within the period.
func buildNewCustomerSet(ctx context.Context, storeID primitive.ObjectID, period string) map[string]bool {
	m := make(map[string]bool)

	var since time.Time
	if period == "30d" {
		since = time.Now().AddDate(0, 0, -30)
	} else if period == "90d" {
		since = time.Now().AddDate(0, 0, -90)
	} else {
		return m
	}

	coll := db.GetDB("store_" + storeID.Hex()).Collection("order")
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id":    storeID,
			"deleted":     bson.M{"$ne": true},
			"customer_id": bson.M{"$ne": nil},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":        "$customer_id",
			"first_date": bson.M{"$min": "$date"},
		}}},
		{{Key: "$match", Value: bson.M{
			"first_date": bson.M{"$gte": since},
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
		m[id.Hex()] = true
	}
	return m
}

// RunBITopCustomersUpdate refreshes all periods for a store.
func RunBITopCustomersUpdate(storeID primitive.ObjectID) {
	for _, period := range []string{"30d", "90d", "all"} {
		if err := UpsertBITopCustomers(storeID, period); err != nil {
			log.Printf("[BI] top_customers error period=%s store=%s: %v", period, storeID.Hex(), err)
		}
	}
}
