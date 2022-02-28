package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

type ProductPurchaseHistory struct {
	ID           primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	ProductID    primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	VendorID     *primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorName   string              `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	PurchaseID   *primitive.ObjectID `json:"purchase_id,omitempty" bson:"purchase_id,omitempty"`
	PurchaseCode string              `json:"purchase_code,omitempty" bson:"purchase_code,omitempty"`
	Quantity     float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice    float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit         string              `bson:"unit,omitempty" json:"unit,omitempty"`
	CreatedAt    *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt    *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (purchase *Purchase) AddProductsPurchaseHistory() error {

	exists, err := IsPurchaseHistoryExistsByPurchaseID(&purchase.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, purchaseProduct := range purchase.Products {

		history := ProductPurchaseHistory{
			ProductID:    purchaseProduct.ProductID,
			VendorID:     purchase.VendorID,
			VendorName:   purchase.VendorName,
			PurchaseID:   &purchase.ID,
			PurchaseCode: purchase.Code,
			Quantity:     purchaseProduct.Quantity,
			UnitPrice:    purchaseProduct.PurchaseUnitPrice,
			Unit:         purchaseProduct.Unit,
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

func IsPurchaseHistoryExistsByPurchaseID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"purchase_id": ID,
	})

	return (count > 0), err
}

/*
func FindPurchaseHistoryByPurchaseID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchaseHistory *ProductPurchaseHistory, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"order_id": ID}, findOneOptions).
		Decode(&purchaseHistory)
	if err != nil {
		return nil, err
	}

	return purchaseHistory, err
}
*/
