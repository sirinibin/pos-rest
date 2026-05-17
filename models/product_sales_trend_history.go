package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ProductSalesTrendHistory records a monthly snapshot of a product's sales velocity trend.
// Written by the BI cron job (every 3 hours); upserted by {date, store_id, product_id}.
type ProductSalesTrendHistory struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID                  *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	ProductID                primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	ProductName              string              `bson:"product_name,omitempty" json:"product_name,omitempty"`
	ProductNameInArabic      string              `bson:"product_name_in_arabic" json:"product_name_in_arabic"`
	SalesVelocityTrend       string              `bson:"sales_velocity_trend,omitempty" json:"sales_velocity_trend,omitempty"`
	SalesVelocityTrendReason string              `bson:"sales_velocity_trend_reason,omitempty" json:"sales_velocity_trend_reason,omitempty"`
	SlopPercentPerMonth      float64             `bson:"slop_percent_per_month" json:"slop_percent_per_month"`
	MomentumPercentPer3Month float64             `bson:"momentum_percent_per_3month" json:"momentum_percent_per_3month"`
	AvgMonthlyQty            float64             `bson:"avg_monthly_qty" json:"avg_monthly_qty"`
	Recent3MonthQty          float64             `bson:"recent_3month_qty" json:"recent_3month_qty"`
	Revenue                  float64             `bson:"revenue" json:"revenue"`
	Profit                   float64             `bson:"profit" json:"profit"`
	CreatedAt                *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// GetProductSalesTrendHistory returns velocity trend history for a product, newest first.
func (store *Store) GetProductSalesTrendHistory(productID primitive.ObjectID) ([]ProductSalesTrendHistory, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_trend_history")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "date", Value: -1}}).SetLimit(100)
	cursor, err := collection.Find(ctx, bson.M{
		"product_id": productID,
		"store_id":   store.ID,
	}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []ProductSalesTrendHistory
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
