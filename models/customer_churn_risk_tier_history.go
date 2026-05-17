package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CustomerChurnRiskTierHistory records a monthly snapshot of a customer's churn risk.
// Written by the BI cron job (every 3 hours); upserted by {date, store_id, customer_id}.
type CustomerChurnRiskTierHistory struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date               *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID            *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID         primitive.ObjectID  `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName       string              `bson:"customer_name,omitempty" json:"customer_name,omitempty"`
	CustomerNameArabic string              `bson:"customer_name_arabic" json:"customer_name_arabic"`
	RiskTier           string              `bson:"risk_tier,omitempty" json:"risk_tier,omitempty"`
	RiskTierReason     string              `bson:"risk_tier_reason,omitempty" json:"risk_tier_reason,omitempty"`
	ChurnPercent       float64             `bson:"churn_percent" json:"churn_percent"`
	TotalSpend         float64             `bson:"total_spend" json:"total_spend"`
	DaysSinceLastBuy   int                 `bson:"days_since_last_buy" json:"days_since_last_buy"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// GetCustomerChurnHistory returns churn risk tier history for a customer, newest first.
func (store *Store) GetCustomerChurnHistory(customerID primitive.ObjectID) ([]CustomerChurnRiskTierHistory, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer_churn_risk_tier_history")
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

	var results []CustomerChurnRiskTierHistory
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
