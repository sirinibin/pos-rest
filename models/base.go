package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PaymentBase holds the 18 fields that are identical across all six payment types
// (SalesPayment, PurchasePayment, QuotationPayment, SalesReturnPayment,
// PurchaseReturnPayment, QuotationSalesReturnPayment).
// Embed this struct (unnamed) in each concrete payment type; bson and JSON tags
// are promoted to the outer struct so serialisation is unchanged.
type PaymentBase struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
	Amount        float64             `json:"amount" bson:"amount"`
	Method        string              `json:"method" bson:"method"`
	ReferenceType string              `json:"reference_type" bson:"reference_type"`
	ReferenceCode string              `json:"reference_code" bson:"reference_code"`
	ReferenceID   *primitive.ObjectID `json:"reference_id" bson:"reference_id"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName     string              `json:"store_name" bson:"store_name"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt     *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}
