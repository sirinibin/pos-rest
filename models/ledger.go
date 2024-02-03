package models

import (
	"context"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Ledger : Ledger structure
type Ledger struct {
	ID             primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID        *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	ReferenceID    primitive.ObjectID  `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	ReferenceModel string              `bson:"reference_model,omitempty" json:"reference_model,omitempty"`
	ReferenceCode  string              `bson:"reference_code,omitempty" json:"reference_code,omitempty"`
	Journals       []Journal           `json:"journals,omitempty" bson:"journals,omitempty"`
	CreatedAt      *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt      *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// Journal : Journal structure
type Journal struct {
	Date *time.Time `bson:"date,omitempty" json:"date,omitempty"`
	//Account       Account            `json:"account,omitempty" bson:"account,omitempty"`
	AccountID     primitive.ObjectID `json:"account_id,omitempty" bson:"account_id,omitempty"`
	AccountName   string             `json:"account_name,omitempty" bson:"account_name,omitempty"`
	AccountNumber int64              `bson:"account_number,omitempty" json:"account_number,omitempty"`
	DebitOrCredit string             `json:"debit_or_credit,omitempty" bson:"debit_or_credit,omitempty"`
	Debit         float64            `bson:"debit" json:"debit"`
	Credit        float64            `bson:"credit" json:"credit"`
	GroupAccounts []int64            `bson:"group_accounts" json:"group_accounts"`
	CreatedAt     *time.Time         `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time         `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func RemoveLedgerByReferenceID(referenceID primitive.ObjectID) error {
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("ledger")
	_, err := collection.DeleteOne(ctx, bson.M{
		"reference_id": referenceID,
	})
	if err != nil {
		return err
	}

	return nil
}

func (ledger *Ledger) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("ledger")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ledger.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &ledger)
	if err != nil {
		return err
	}

	return nil
}

func (ledger *Ledger) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("ledger")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": ledger.ID},
		bson.M{"$set": ledger},
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

func FindLedgerByReferenceID(
	referenceID primitive.ObjectID,
	storeID primitive.ObjectID,
	selectFields map[string]interface{},
) (ledger *Ledger, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("ledger")
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
		Decode(&ledger)
	if err != nil {
		return nil, err
	}

	return ledger, err
}
