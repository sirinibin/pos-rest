package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CustomerPredicted12MonthCLVHistory records a monthly snapshot of a customer's
// predicted 12-month customer lifetime value.
// Written by the BI cron job (every 3 hours); upserted by {date, store_id, customer_id}.
type CustomerPredicted12MonthCLVHistory struct {
	ID                                    primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                                  *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID                               *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID                            primitive.ObjectID  `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName                          string              `bson:"customer_name,omitempty" json:"customer_name,omitempty"`
	CustomerNameArabic                    string              `bson:"customer_name_arabic" json:"customer_name_arabic"`
	LifetimeValueSegmentFor12Months       string              `bson:"lifetime_value_segment_for_12months,omitempty" json:"lifetime_value_segment_for_12months,omitempty"`
	LifetimeValueSegmentReasonFor12Months string              `bson:"lifetime_value_segment_reason_for_12months,omitempty" json:"lifetime_value_segment_reason_for_12months,omitempty"`
	PredictedCLVAmount12Months            float64             `bson:"predicted_clv_amount_12months" json:"predicted_clv_amount_12months"`
	PredictedPurchasesCountForecast       float64             `bson:"predicted_purchases_count_forecast" json:"predicted_purchases_count_forecast"`
	PredictedAvgOrderAmount               float64             `bson:"predicted_avg_order_amount" json:"predicted_avg_order_amount"`
	HistoryOrdersCount                    int                 `bson:"history_orders_count" json:"history_orders_count"`
	HistorySpendAmount                    float64             `bson:"history_spend_amount" json:"history_spend_amount"`
	TenureDays                            int                 `bson:"tenure_days" json:"tenure_days"`
	CreatedAt                             *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                             *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// GetBICustomerCLV returns the latest predicted CLV snapshot for all customers,
// optionally filtered by segment ("High Value","Mid Value","Low Value").
// Sorted by predicted_clv_amount_12months desc.
func (store *Store) GetBICustomerCLV(segment string, limit int) ([]CustomerPredicted12MonthCLVHistory, error) {
	if limit <= 0 || limit > 2000 {
		limit = 50
	}
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer_predicted_12month_clv_history")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	matchFilter := bson.M{"store_id": store.ID}
	if segment != "" {
		matchFilter["lifetime_value_segment_for_12months"] = segment
	}

	pipeline := mongo.Pipeline{
		bson.D{bson.E{Key: "$match", Value: matchFilter}},
		bson.D{bson.E{Key: "$sort", Value: bson.D{bson.E{Key: "date", Value: -1}}}},
		bson.D{bson.E{Key: "$group", Value: bson.M{
			"_id": "$customer_id",
			"doc": bson.M{"$first": "$$ROOT"},
		}}},
		bson.D{bson.E{Key: "$replaceRoot", Value: bson.M{"newRoot": "$doc"}}},
		bson.D{bson.E{Key: "$sort", Value: bson.D{bson.E{Key: "predicted_clv_amount_12months", Value: -1}}}},
		bson.D{bson.E{Key: "$limit", Value: int64(limit)}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []CustomerPredicted12MonthCLVHistory
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// GetCustomerCLVHistory returns CLV segment history for a customer, newest first.
func (store *Store) GetCustomerCLVHistory(customerID primitive.ObjectID) ([]CustomerPredicted12MonthCLVHistory, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer_predicted_12month_clv_history")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "date", Value: -1}}).SetLimit(100)
	cursor, err := collection.Find(ctx, bson.M{
		"customer_id": customerID,
		"store_id":    store.ID,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []CustomerPredicted12MonthCLVHistory
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
