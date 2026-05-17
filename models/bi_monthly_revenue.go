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

// ── Shared BI conversion helpers ─────────────────────────────────────────────

func toFloat64(v interface{}) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	return 0
}

func toInt64(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	case int:
		return int64(x)
	}
	return 0
}

// BIMonthlyRevenue holds pre-aggregated monthly revenue stats for a store.
// Collection: bi_monthly_revenue
// Updated every hour by the incremental cron, and on demand via backfill.
type BIMonthlyRevenue struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID         primitive.ObjectID `json:"store_id" bson:"store_id"`
	Year            int                `json:"year" bson:"year"`
	Month           int                `json:"month" bson:"month"`   // 1-12
	Period          string             `json:"period" bson:"period"` // "2026-05"
	OrderCount      int64              `json:"order_count" bson:"order_count"`
	GrossRevenue    float64            `json:"gross_revenue" bson:"gross_revenue"`
	NetRevenue      float64            `json:"net_revenue" bson:"net_revenue"` // after returns
	Profit          float64            `json:"profit" bson:"profit"`
	Loss            float64            `json:"loss" bson:"loss"`
	VatCollected    float64            `json:"vat_collected" bson:"vat_collected"`
	DiscountGiven   float64            `json:"discount_given" bson:"discount_given"`
	ReturnCount     int64              `json:"return_count" bson:"return_count"`
	ReturnAmount    float64            `json:"return_amount" bson:"return_amount"`
	CashCollected   float64            `json:"cash_collected" bson:"cash_collected"`
	BankCollected   float64            `json:"bank_collected" bson:"bank_collected"`
	NewCustomers    int64              `json:"new_customers" bson:"new_customers"`
	UniqueCustomers int64              `json:"unique_customers" bson:"unique_customers"`
	UpdatedAt       time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBIMonthlyRevenue returns monthly revenue records for a store.
// months: how many months back to return (default 12, max 60).
func (store *Store) GetBIMonthlyRevenue(months int) ([]BIMonthlyRevenue, error) {
	if months <= 0 {
		months = 12
	}
	if months > 60 {
		months = 60
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_monthly_revenue")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cutoff := time.Now().AddDate(0, -months, 0)
	// The collection is already scoped to this store's DB, so no store_id filter needed.
	// (Historical docs may have store_id as string vs ObjectId — year/month range is sufficient.)
	filter := bson.M{
		"$or": bson.A{
			bson.M{"year": bson.M{"$gt": cutoff.Year()}},
			bson.M{
				"year":  cutoff.Year(),
				"month": bson.M{"$gte": int(cutoff.Month())},
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

	var results []BIMonthlyRevenue
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// UpsertBIMonthlyRevenue computes and upserts monthly revenue for the given year/month.
func UpsertBIMonthlyRevenue(storeID primitive.ObjectID, year, month int) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_monthly_revenue")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	period := fmt.Sprintf("%04d-%02d", year, month)

	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	ordersColl := db.GetDB("store_" + storeID.Hex()).Collection("order")

	// Aggregate orders for this month
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
			"date":     bson.M{"$gte": startDate, "$lt": endDate},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":              nil,
			"order_count":      bson.M{"$sum": 1},
			"gross_revenue":    bson.M{"$sum": "$net_total"},
			"profit":           bson.M{"$sum": "$profit"},
			"loss":             bson.M{"$sum": "$loss"},
			"vat_collected":    bson.M{"$sum": "$vat_price"},
			"discount_given":   bson.M{"$sum": "$discount"},
			"return_count":     bson.M{"$sum": "$return_count"},
			"return_amount":    bson.M{"$sum": "$return_amount"},
			"cash_collected":   bson.M{"$sum": "$cash_sales"},
			"bank_collected":   bson.M{"$sum": "$bank_account_sales"},
			"unique_customers": bson.M{"$addToSet": "$customer_id"},
		}}},
	}

	cur, err := ordersColl.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	doc := BIMonthlyRevenue{
		StoreID:   storeID,
		Year:      year,
		Month:     month,
		Period:    period,
		UpdatedAt: time.Now(),
	}

	if cur.Next(ctx) {
		var row bson.M
		if err := cur.Decode(&row); err == nil {
			doc.OrderCount = toInt64(row["order_count"])
			doc.GrossRevenue = toFloat64(row["gross_revenue"])
			doc.Profit = toFloat64(row["profit"])
			doc.Loss = toFloat64(row["loss"])
			doc.VatCollected = toFloat64(row["vat_collected"])
			doc.DiscountGiven = toFloat64(row["discount_given"])
			doc.ReturnCount = toInt64(row["return_count"])
			doc.ReturnAmount = toFloat64(row["return_amount"])
			doc.CashCollected = toFloat64(row["cash_collected"])
			doc.BankCollected = toFloat64(row["bank_collected"])
			// count unique customers
			if arr, ok := row["unique_customers"].(primitive.A); ok {
				doc.UniqueCustomers = int64(len(arr))
			}
			doc.NetRevenue = doc.GrossRevenue - doc.ReturnAmount
		}
	}

	// Count new customers (first order ever in this month)
	doc.NewCustomers = countNewCustomers(ctx, storeID, startDate, endDate)

	// Use year+month as upsert key (collection is store-scoped; avoids ObjectId vs string mismatch).
	upsertOpts := options.Replace().SetUpsert(true)
	_, err = collection.ReplaceOne(ctx,
		bson.M{"year": year, "month": month},
		doc, upsertOpts)
	return err
}

// countNewCustomers counts customers whose very first order falls in [start, end).
func countNewCustomers(ctx context.Context, storeID primitive.ObjectID, start, end time.Time) int64 {
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
			"first_date": bson.M{"$gte": start, "$lt": end},
		}}},
		{{Key: "$count", Value: "n"}},
	}
	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	defer cur.Close(ctx)
	if cur.Next(ctx) {
		var r bson.M
		cur.Decode(&r)
		return toInt64(r["n"])
	}
	return 0
}

// RunBIMonthlyRevenueUpdate refreshes bi_monthly_revenue for the current and previous N months.
func RunBIMonthlyRevenueUpdate(storeID primitive.ObjectID, monthsBack int) {
	now := time.Now()
	for i := 0; i <= monthsBack; i++ {
		t := now.AddDate(0, -i, 0)
		if err := UpsertBIMonthlyRevenue(storeID, t.Year(), int(t.Month())); err != nil {
			log.Printf("[BI] monthly_revenue upsert %04d-%02d store=%s err=%v",
				t.Year(), int(t.Month()), storeID.Hex(), err)
		}
	}
}
