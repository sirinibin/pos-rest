package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

type ProductQuotationHistory struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	ProductID     primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID    *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName  string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	QuotationID   *primitive.ObjectID `json:"quotation_id,omitempty" bson:"quotation_id,omitempty"`
	QuotationCode string              `json:"quotation_code,omitempty" bson:"quotation_code,omitempty"`
	Quantity      float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice     float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit          string              `bson:"unit,omitempty" json:"unit,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (quotation *Quotation) AddProductsQuotationHistory() error {
	exists, err := IsQuotationHistoryExistsByQuotationID(&quotation.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, quotationProduct := range quotation.Products {

		history := ProductQuotationHistory{
			ProductID:     quotationProduct.ProductID,
			CustomerID:    quotation.CustomerID,
			CustomerName:  quotation.CustomerName,
			QuotationID:   &quotation.ID,
			QuotationCode: quotation.Code,
			Quantity:      quotationProduct.Quantity,
			UnitPrice:     quotationProduct.UnitPrice,
			Unit:          quotationProduct.Unit,
		}
		history.ID = primitive.NewObjectID()

		now := time.Now()
		history.CreatedAt = &now
		history.UpdatedAt = &now

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}
	}

	return nil
}

func IsQuotationHistoryExistsByQuotationID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"quotation_id": ID,
	})

	return (count > 0), err
}
