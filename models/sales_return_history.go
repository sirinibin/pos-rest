package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

type ProductSalesReturnHistory struct {
	ID              primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	ProductID       primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID      *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName    string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	SalesReturnID   *primitive.ObjectID `json:"sales_return_id,omitempty" bson:"sales_return_id,omitempty"`
	SalesReturnCode string              `json:"sales_return_code,omitempty" bson:"sales_return_code,omitempty"`
	Quantity        float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice       float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit            string              `bson:"unit,omitempty" json:"unit,omitempty"`
	CreatedAt       *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (salesReturn *SalesReturn) AddProductsSalesReturnHistory() error {
	exists, err := IsSalesReturnHistoryExistsBySalesReturnID(&salesReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, salesReturnProduct := range salesReturn.Products {

		history := ProductSalesReturnHistory{
			ProductID:       salesReturnProduct.ProductID,
			CustomerID:      salesReturn.CustomerID,
			CustomerName:    salesReturn.CustomerName,
			SalesReturnID:   &salesReturn.ID,
			SalesReturnCode: salesReturn.Code,
			Quantity:        salesReturnProduct.Quantity,
			UnitPrice:       salesReturnProduct.UnitPrice,
			Unit:            salesReturnProduct.Unit,
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

func IsSalesReturnHistoryExistsBySalesReturnID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"sales_return_id": ID,
	})

	return (count > 0), err
}
