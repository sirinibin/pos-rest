package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ProductAbcXyzClassificationHistory records a monthly snapshot of a product's
// ABC-XYZ inventory classification.
// Written by the BI cron job (every 3 hours); upserted by {date, store_id, product_id}.
type ProductAbcXyzClassificationHistory struct {
	ID                  primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID             *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	ProductID           primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	ProductName         string              `bson:"product_name,omitempty" json:"product_name,omitempty"`
	ProductNameInArabic string              `bson:"product_name_in_arabic" json:"product_name_in_arabic"`
	Class               string              `bson:"class,omitempty" json:"class,omitempty"`
	ClassReason         string              `bson:"class_reason,omitempty" json:"class_reason,omitempty"`
	AbcTier             string              `bson:"abc_tier,omitempty" json:"abc_tier,omitempty"`
	XyzTier             string              `bson:"xyz_tier,omitempty" json:"xyz_tier,omitempty"`
	CV                  float64             `bson:"cv" json:"cv"`
	ActiveMonths        int                 `bson:"active_months" json:"active_months"`
	StockingStrategy    string              `bson:"stocking_strategy,omitempty" json:"stocking_strategy,omitempty"`
	CreatedAt           *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt           *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// GetProductAbcXyzHistory returns ABC-XYZ classification history for a product, newest first.
func (store *Store) GetProductAbcXyzHistory(productID primitive.ObjectID) ([]ProductAbcXyzClassificationHistory, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_abc_xyz_classification_history")
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

	var results []ProductAbcXyzClassificationHistory
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
