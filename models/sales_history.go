package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

type ProductSalesHistory struct {
	ID           primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID      *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName    string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID    primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID   *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	OrderID      *primitive.ObjectID `json:"order_id,omitempty" bson:"order_id,omitempty"`
	OrderCode    string              `json:"order_code,omitempty" bson:"order_code,omitempty"`
	Quantity     float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice    float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit         string              `bson:"unit,omitempty" json:"unit,omitempty"`
	CreatedAt    *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt    *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (order *Order) AddProductsSalesHistory() error {
	exists, err := IsSalesHistoryExistsByOrderID(&order.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, orderProduct := range order.Products {

		history := ProductSalesHistory{
			StoreID:      order.StoreID,
			StoreName:    order.StoreName,
			ProductID:    orderProduct.ProductID,
			CustomerID:   order.CustomerID,
			CustomerName: order.CustomerName,
			OrderID:      &order.ID,
			OrderCode:    order.Code,
			Quantity:     orderProduct.Quantity,
			UnitPrice:    orderProduct.UnitPrice,
			Unit:         orderProduct.Unit,
			CreatedAt:    order.CreatedAt,
			UpdatedAt:    order.UpdatedAt,
		}
		history.ID = primitive.NewObjectID()

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}
	}

	return nil
}

func IsSalesHistoryExistsByOrderID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"order_id": ID,
	})

	return (count > 0), err
}
