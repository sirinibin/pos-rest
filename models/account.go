package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Account : Account structure
type Account struct {
	ID             primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID        *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	ReferenceID    *primitive.ObjectID `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	ReferenceModel *string             `bson:"reference_model,omitempty" json:"reference_model,omitempty"`
	Type           string              `bson:"type,omitempty" json:"type,omitempty"` //drawing,expense,asset,liability,equity,revenue
	Number         int64               `bson:"number,omitempty" json:"number,omitempty"`
	Name           string              `bson:"name,omitempty" json:"name,omitempty"`
	Phone          *string             `bson:"phone,omitempty" json:"phone,omitempty"`
	Balance        float64             `bson:"balance" json:"balance"`
	DebitTotal     float64             `bson:"debit_total" json:"debit_total"`
	CreditTotal    float64             `bson:"credit_total" json:"credit_total"`
	Open           bool                `bson:"open" json:"open"`
	CreatedAt      *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt      *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (account *Account) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &account)
	if err != nil {
		return err
	}

	return nil
}

func (account *Account) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": account.ID},
		bson.M{"$set": account},
		updateOptions,
	)
	if err != nil {
		return err
	}

	if updateResult.MatchedCount > 0 {
		return nil
	}

	return nil
}

func FindAccountByID(
	ID primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil {
		return nil, err
	}

	return account, err
}

func FindAccountByReferenceID(
	referenceID primitive.ObjectID,
	storeID primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"reference_id": referenceID,
			"store_id":     storeID,
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil {
		return nil, err
	}

	return account, err
}

func FindAccountByPhone(
	phone string,
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"phone":    phone,
			"store_id": storeID,
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil {
		return nil, err
	}

	return account, err
}

func FindAccountByName(
	name string,
	storeID *primitive.ObjectID,
	referenceID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	filter := bson.M{
		"name":     name,
		"store_id": storeID,
	}

	if referenceID != nil {
		filter["reference_id"] = nil
	}

	err = collection.FindOne(ctx, filter, findOneOptions). //"deleted": bson.M{"$ne": true}
								Decode(&account)
	if err != nil {
		return nil, err
	}

	return account, err
}

func (account *Account) IsPhoneExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if account.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone":    account.Phone,
			"store_id": account.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone":    account.Phone,
			"store_id": account.StoreID,
			"_id":      bson.M{"$ne": account.ID},
		})
	}

	return (count == 1), err
}

func IsAccountExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
