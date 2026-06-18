package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// CustomerOverCredit holds a customer that has exceeded their credit limit.
type CustomerOverCredit struct {
	CustomerID     string  `json:"customer_id"`
	CustomerName   string  `json:"customer_name"`
	CustomerCode   string  `json:"customer_code"`
	Phone          string  `json:"phone"`
	CreditLimit    float64 `json:"credit_limit"`
	CreditBalance  float64 `json:"credit_balance"`  // outstanding owed by customer
	ExcessAmount   float64 `json:"excess_amount"`   // balance - limit
	ExcessPercent  float64 `json:"excess_pct"`      // how far over limit
}

// GetCustomersOverCreditLimit returns customers whose credit balance exceeds their credit limit.
func (store *Store) GetCustomersOverCreditLimit(limit int) ([]CustomerOverCredit, error) {
	if limit <= 0 {
		limit = 100
	}
	col := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter := bson.M{
		"deleted":  bson.M{"$ne": true},
		"store_id": store.ID,
		// Find customers where credit_balance > credit_limit AND credit_limit > 0
		"$expr": bson.M{
			"$and": bson.A{
				bson.M{"$gt": bson.A{"$credit_balance", "$credit_limit"}},
				bson.M{"$gt": bson.A{"$credit_limit", 0}},
			},
		},
	}

	opts := options.Find().
		SetSort(bson.M{"credit_balance": -1}).
		SetLimit(int64(limit)).
		SetProjection(bson.M{
			"_id": 1, "name": 1, "code": 1, "phone": 1,
			"credit_limit": 1, "credit_balance": 1,
		})

	cursor, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	type row struct {
		ID            primitive.ObjectID `bson:"_id"`
		Name          string             `bson:"name"`
		Code          string             `bson:"code"`
		Phone         string             `bson:"phone"`
		CreditLimit   float64            `bson:"credit_limit"`
		CreditBalance float64            `bson:"credit_balance"`
	}

	var results []CustomerOverCredit
	for cursor.Next(ctx) {
		var r row
		if err := cursor.Decode(&r); err != nil {
			continue
		}
		excess := r.CreditBalance - r.CreditLimit
		pct := 0.0
		if r.CreditLimit > 0 {
			pct = (excess / r.CreditLimit) * 100
		}
		results = append(results, CustomerOverCredit{
			CustomerID:    r.ID.Hex(),
			CustomerName:  r.Name,
			CustomerCode:  r.Code,
			Phone:         r.Phone,
			CreditLimit:   RoundTo2Decimals(r.CreditLimit),
			CreditBalance: RoundTo2Decimals(r.CreditBalance),
			ExcessAmount:  RoundTo2Decimals(excess),
			ExcessPercent: RoundTo2Decimals(pct),
		})
	}
	return results, nil
}
