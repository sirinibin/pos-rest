package models

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Post : Post structure
type Post struct {
	Date          *time.Time         `bson:"date,omitempty" json:"date,omitempty"`
	AccountID     primitive.ObjectID `json:"account_id,omitempty" bson:"account_id,omitempty"`
	AccountName   string             `json:"account_name,omitempty" bson:"account_name,omitempty"`
	AccountNumber int64              `bson:"account_number,omitempty" json:"account_number,omitempty"`
	DebitOrCredit string             `json:"debit_or_credit,omitempty" bson:"debit_or_credit,omitempty"`
	Debit         float64            `bson:"debit" json:"debit"`
	Credit        float64            `bson:"credit" json:"credit"`
	CreatedAt     *time.Time         `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time         `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// Account : Account structure
type Posting struct {
	ID             primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date           *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID        *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	AccountID      primitive.ObjectID  `json:"account_id,omitempty" bson:"account_id,omitempty"`
	AccountName    string              `json:"account_name,omitempty" bson:"account_name,omitempty"`
	AccountNumber  int64               `bson:"account_number,omitempty" json:"account_number,omitempty"`
	ReferenceID    primitive.ObjectID  `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	ReferenceModel string              `bson:"reference_model,omitempty" json:"reference_model,omitempty"`
	ReferenceCode  string              `bson:"reference_code,omitempty" json:"reference_code,omitempty"`
	Posts          []Post              `json:"posts,omitempty" bson:"posts,omitempty"`
	DebitTotal     float64             `bson:"debit_total" json:"debit_total"`
	CreditTotal    float64             `bson:"credit_total" json:"credit_total"`
	CreatedAt      *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt      *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type AccountStats struct {
	ID          *primitive.ObjectID `json:"id" bson:"_id"`
	DebitTotal  float64             `json:"debit_total" bson:"debit_total"`
	CreditTotal float64             `json:"credit_total" bson:"credit_total"`
}

func (account *Account) CalculateBalance() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := map[string]interface{}{
		"account_id": account.ID,
	}

	stats := AccountStats{}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":          nil,
				"debit_total":  bson.M{"$sum": "$debit_total"},
				"credit_total": bson.M{"$sum": "$credit_total"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
		stats.DebitTotal = math.Round(stats.DebitTotal*100) / 100
		stats.CreditTotal = math.Round(stats.CreditTotal*100) / 100
	}

	account.DebitTotal = stats.DebitTotal
	account.CreditTotal = stats.CreditTotal

	if stats.CreditTotal > stats.DebitTotal {
		account.Balance = math.Round((stats.CreditTotal-stats.DebitTotal)*100) / 100
	} else {
		account.Balance = math.Round((stats.DebitTotal-stats.CreditTotal)*100) / 100
	}

	if account.Type == "asset" && account.ReferenceModel != nil && *account.ReferenceModel == "customer" {
		if stats.CreditTotal > stats.DebitTotal {
			account.Type = "liability"
		} else {
			account.Type = "asset" //debtor
		}
	}

	if account.Balance == 0 {
		account.Open = false
	} else {
		account.Open = true
	}

	err = account.Update()
	if err != nil {
		return err
	}

	return nil
}

func (ledger *Ledger) RemovePostings() error {
	log.Print("Removing postings")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	_, err := collection.DeleteMany(ctx, bson.M{
		"reference_id": ledger.ReferenceID,
	})
	if err != nil {
		return err
	}

	return nil
}

func (ledger *Ledger) CreatePostings() (postings []*Posting, err error) {
	log.Print("Creating postings")
	now := time.Now()

	for _, journal := range ledger.Journals {

		account, err := FindAccountByID(journal.AccountID, bson.M{})
		if err != nil {
			return nil, err
		}

		//account := journal.Account

		posts := []Post{} // Reset posts
		debitTotal := float64(0.00)
		creditTotal := float64(0.00)
		for _, journal2 := range ledger.Journals {
			if journal2.AccountID.Hex() == account.ID.Hex() || !journal.Date.Equal(*journal2.Date) {
				continue
			}

			if journal.DebitOrCredit == "debit" && journal2.DebitOrCredit == "credit" {
				amount := float64(0.00)
				if journal.Debit < journal2.Credit {
					amount = journal.Debit
				} else {
					amount = journal2.Credit
				}

				posts = append(posts, Post{
					Date:          journal2.Date,
					AccountID:     journal2.AccountID,
					AccountName:   journal2.AccountName,
					AccountNumber: journal2.AccountNumber,
					DebitOrCredit: "debit",
					Debit:         amount,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				debitTotal += amount
			} else if journal.DebitOrCredit == "credit" && journal2.DebitOrCredit == "debit" {
				amount := float64(0.00)
				if journal.Credit < journal2.Debit {
					amount = journal.Credit
				} else {
					amount = journal2.Debit
				}

				posts = append(posts, Post{
					Date: journal2.Date,
					//Account:       journal2.Account,
					AccountID:     journal2.AccountID,
					AccountName:   journal2.AccountName,
					AccountNumber: journal2.AccountNumber,
					DebitOrCredit: "credit",
					Credit:        amount,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				creditTotal += amount
			}
		}

		posting := &Posting{
			Date:           journal.Date,
			StoreID:        ledger.StoreID,
			AccountID:      account.ID,
			AccountName:    account.Name,
			AccountNumber:  account.Number,
			ReferenceID:    ledger.ReferenceID,
			ReferenceModel: ledger.ReferenceModel,
			ReferenceCode:  ledger.ReferenceCode,
			Posts:          posts,
			DebitTotal:     debitTotal,
			CreditTotal:    creditTotal,
			CreatedAt:      &now,
			UpdatedAt:      &now,
		}

		err = posting.Insert()
		if err != nil {
			return nil, err
		}

		postings = append(postings, posting)

		err = account.CalculateBalance()
		if err != nil {
			return nil, err
		}

	} // end for

	//Calculate Balance of cash account
	cashAccount, err := CreateAccountIfNotExists(ledger.StoreID, nil, nil, "Cash", "asset", nil, true)
	if err != nil {
		return nil, err
	}

	err = cashAccount.CalculateBalance()
	if err != nil {
		return nil, err
	}

	//Calculate Balance of bank account
	bankAccount, err := CreateAccountIfNotExists(ledger.StoreID, nil, nil, "Bank", "asset", nil, true)
	if err != nil {
		return nil, err
	}

	err = bankAccount.CalculateBalance()
	if err != nil {
		return nil, err
	}

	if ledger.ReferenceModel == "sales" {

		order, err := FindOrderByID(&ledger.ReferenceID, bson.M{})
		if err != nil {
			return nil, err
		}

		referenceModel := "customer"
		customerAccount, err := CreateAccountIfNotExists(
			order.StoreID,
			order.CustomerID,
			&referenceModel,
			order.CustomerName,
			"asset",
			nil,
			false,
		)
		if err != nil {
			return nil, err
		}

		err = customerAccount.CalculateBalance()
		if err != nil {
			return nil, err
		}
	}

	return postings, nil
}

func (posting *Posting) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	posting.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &posting)
	if err != nil {
		return err
	}

	return nil
}

func (posting *Posting) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": posting.ID},
		bson.M{"$set": posting},
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

func FindPostingByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (posting *Posting, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&posting)
	if err != nil {
		return nil, err
	}

	return posting, err
}

func IsPostingExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
