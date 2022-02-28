package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

type ProductPurchaseReturnHistory struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	ProductID          primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	VendorID           *primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorName         string              `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	PurchaseReturnID   *primitive.ObjectID `json:"purchase_return_id,omitempty" bson:"purchase_return_id,omitempty"`
	PurchaseReturnCode string              `json:"purchase_return_code,omitempty" bson:"purchase_return_code,omitempty"`
	Quantity           float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice          float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit               string              `bson:"unit,omitempty" json:"unit,omitempty"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (purchaseReturn *PurchaseReturn) AddProductsPurchaseReturnHistory() error {

	exists, err := IsPurchaseReturnHistoryExistsByPurchaseReturnID(&purchaseReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, purchaseReturnProduct := range purchaseReturn.Products {

		history := ProductPurchaseReturnHistory{
			ProductID:          purchaseReturnProduct.ProductID,
			VendorID:           purchaseReturn.VendorID,
			VendorName:         purchaseReturn.VendorName,
			PurchaseReturnID:   &purchaseReturn.ID,
			PurchaseReturnCode: purchaseReturn.Code,
			Quantity:           purchaseReturnProduct.Quantity,
			UnitPrice:          purchaseReturnProduct.PurchaseReturnUnitPrice,
			Unit:               purchaseReturnProduct.Unit,
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

func IsPurchaseReturnHistoryExistsByPurchaseReturnID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_purchase_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"purchase_return_id": ID,
	})

	return (count > 0), err
}
