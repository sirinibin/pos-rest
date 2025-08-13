package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	AccountNumber string             `bson:"account_number,omitempty" json:"account_number,omitempty"`
	DebitOrCredit string             `json:"debit_or_credit,omitempty" bson:"debit_or_credit,omitempty"`
	Debit         float64            `bson:"debit,omitempty" json:"debit,omitempty"`
	Credit        float64            `bson:"credit,omitempty" json:"credit,omitempty"`
	GroupAccounts []string           `bson:"group_accounts" json:"group_accounts"`
	GroupID       primitive.ObjectID `json:"group_id,omitempty" bson:"group_id,omitempty"`
	CreatedAt     *time.Time         `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time         `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

func (store *Store) SearchLedger(w http.ResponseWriter, r *http.Request) (models []Ledger, criterias SearchCriterias, err error) {
	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var storeID primitive.ObjectID
	keys, ok := r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[account_id]"]
	if ok && len(keys[0]) >= 1 {

		accountIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range accountIds {
			accountID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, accountID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["journals.account_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[reference_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["reference_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[reference_model]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["reference_model"] = keys[0]
	}

	/*

		keys, ok = r.URL.Query()["search[reference_model]"]
		if ok && len(keys[0]) >= 1 {
			criterias.SearchBy["reference_model"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		}
	*/

	keys, ok = r.URL.Query()["search[reference_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["reference_code"] = keys[0]
	}

	/*
		keys, ok = r.URL.Query()["sort"]
		if ok && len(keys[0]) >= 1 {
			keys[0] = strings.Replace(keys[0], "stores.", "stores."+storeID.Hex()+".", -1)
			criterias.SortBy = GetSortByFields(keys[0])
		}
	*/

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	timeZoneOffset := 0.0
	keys, ok = r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["journals.date"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	var startDate time.Time
	var endDate time.Time

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			endDate = ConvertTimeZoneToUTC(timeZoneOffset, endDate)
		}

		endDate = endDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
	}

	if !startDate.IsZero() && !endDate.IsZero() {
		criterias.SearchBy["journals.date"] = bson.M{"$gte": startDate, "$lte": endDate}
	} else if !startDate.IsZero() {
		criterias.SearchBy["journals.date"] = bson.M{"$gte": startDate}
	} else if !endDate.IsZero() {
		criterias.SearchBy["journals.date"] = bson.M{"$lte": endDate}
	}

	keys, ok = r.URL.Query()["search[debit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["journals.debit"] = bson.M{operator: value}
			/*
				criterias.SearchBy["journals.debit"] = bson.M{"$and": []interface{}{
					bson.M{operator: value},
					bson.M{"$ne": 0},
				}}
			*/

			/*
				criterias.SearchBy["$and"] = []bson.M{
					{"journals.debit": bson.M{operator: value}},
					{"journals.debit": bson.M{"$gt": 0}},
				}
			*/

			/*
				criterias.SearchBy["$or"] = []bson.M{
					{"part_number": bson.M{"$regex": searchWord, "$options": "i"}},
					{"name": bson.M{"$regex": searchWord, "$options": "i"}},
					{"name_in_arabic": bson.M{"$regex": searchWord, "$options": "i"}},
				}
			*/

		} else {
			criterias.SearchBy["journals.debit"] = value
		}
	}

	keys, ok = r.URL.Query()["search[credit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["journals.credit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["journals.credit"] = value
		}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}
		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["created_at"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	keys, ok = r.URL.Query()["search[created_at_from]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtStartDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			createdAtStartDate = ConvertTimeZoneToUTC(timeZoneOffset, createdAtStartDate)
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			createdAtEndDate = ConvertTimeZoneToUTC(timeZoneOffset, createdAtEndDate)
		}

		createdAtEndDate = createdAtEndDate.Add(time.Hour * time.Duration(24))
		createdAtEndDate = createdAtEndDate.Add(-time.Second * time.Duration(1))
	}

	if !createdAtStartDate.IsZero() && !createdAtEndDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate, "$lte": createdAtEndDate}
	} else if !createdAtStartDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$gte": createdAtStartDate}
	} else if !createdAtEndDate.IsZero() {
		criterias.SearchBy["created_at"] = bson.M{"$lte": createdAtEndDate}
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	//Fetch all device documents with (garbage:true AND (gc_processed:false if exist OR gc_processed not exist ))
	/* Note: Actual Record fetching will not happen here
	as it is using mongodb cursor and record fetching will
	start with we call cur.Next()
	*/
	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching Ledger:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := Ledger{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil

}

func (ledger *Ledger) GetRelatedAccounts() (map[string]Account, error) {
	store, err := FindStoreByID(ledger.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	accounts := map[string]Account{}
	for _, journal := range ledger.Journals {
		if journal.AccountID.IsZero() {
			continue
		}

		account, err := store.FindAccountByID(journal.AccountID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}

		if account != nil && !account.ID.IsZero() {
			accounts[account.ID.Hex()] = *account
		}
	}
	return accounts, nil
}

func (store *Store) RemoveLedgerByReferenceID(referenceID primitive.ObjectID) error {
	ctx := context.Background()
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	_, err := collection.DeleteMany(ctx, bson.M{
		"reference_id": referenceID,
	})
	if err != nil {
		return err
	}

	return nil
}

func (ledger *Ledger) Insert() error {
	collection := db.GetDB("store_" + ledger.StoreID.Hex()).Collection("ledger")

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
	collection := db.GetDB("store_" + ledger.StoreID.Hex()).Collection("ledger")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
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

func (store *Store) FindLedgersByReferenceID(
	referenceID primitive.ObjectID,
	storeID primitive.ObjectID,
	selectFields map[string]interface{},
) (ledgers []Ledger, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{
		"reference_id": referenceID,
		"store_id":     store.ID,
	}, findOptions)
	if err != nil {
		return ledgers, errors.New("Error fetching ledgers:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return ledgers, errors.New("Cursor error:" + err.Error())
		}
		ledger := Ledger{}
		err = cur.Decode(&ledger)
		if err != nil {
			return ledgers, errors.New("Cursor decode error:" + err.Error())
		}

		ledgers = append(ledgers, ledger)
	}

	return ledgers, nil
}

func (store *Store) FindLedgerByReferenceID(
	referenceID primitive.ObjectID,
	storeID primitive.ObjectID,
	selectFields map[string]interface{},
) (ledger *Ledger, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("ledger")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"reference_id": referenceID,
			"store_id":     store.ID,
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&ledger)
	if err != nil {
		return nil, err
	}

	return ledger, err
}

func (ledger *Ledger) SetPostBalancesByLedger(afterDate *time.Time) (err error) {
	store, err := FindStoreByID(ledger.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledgerAccounts := map[string]Account{}

	if ledger != nil {
		ledgerAccounts, err = ledger.GetRelatedAccounts()
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("Error getting related accounts: " + err.Error())
		}
	}
	errCh := map[string]chan error{}
	for _, account := range ledgerAccounts {
		//log.Print(account.Name)

		//errCh := make( chan error)

		go func() {
			//log.Print("Inside Async")
			postings, err := store.FindPostsByAccountID(&account.ID, afterDate)
			if err != nil {
				errCh[account.Name] <- err // send error back
			}

			for _, post := range postings {
				//log.Print(post.ReferenceCode)
				for j, subPost := range post.Posts {

					err = account.CalculateBalance(subPost.Date, &subPost.ID)
					if err != nil {
						errCh[account.Name] <- err // send error back
					}
					accountBalance := account.Balance

					if account.Type == "liability" && accountBalance > 0 {
						accountBalance = account.Balance * (-1)
					}

					newBalance := float64(0.00)

					amount := float64(0.00)
					if subPost.Debit > subPost.Credit {
						amount = subPost.Debit
						newBalance = accountBalance + amount
					} else {
						amount = subPost.Credit
						newBalance = accountBalance - amount

						if account.Type == "revenue" || account.Type == "capital" {
							newBalance = (accountBalance + amount)
						}
					}

					//log.Print("Setting Balance:")
					//log.Print(post.Posts[j].Balance)
					post.Posts[j].Balance = newBalance
					err = post.Update()
					if err != nil {
						errCh[account.Name] <- err // send error back
					}
				}

				/*
					if post.ReferenceModel == "sales" {
						postOrder, err := store.FindOrderByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postOrder == nil {
							continue
						}

						err = postOrder.UndoAccounting()
						if err != nil {
							return err
						}
						err = postOrder.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "sales_return" {
						postSalesReturn, err := store.FindSalesReturnByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postSalesReturn == nil {
							continue
						}

						err = postSalesReturn.UndoAccounting()
						if err != nil {
							return err
						}
						err = postSalesReturn.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "purchase" {
						postPurchase, err := store.FindPurchaseByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postPurchase == nil {
							continue
						}

						err = postPurchase.UndoAccounting()
						if err != nil {
							return err
						}

						err = postPurchase.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "purchase_return" {
						postPurchaseReturn, err := store.FindPurchaseReturnByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postPurchaseReturn == nil {
							continue
						}

						err = postPurchaseReturn.UndoAccounting()
						if err != nil {
							return err
						}

						err = postPurchaseReturn.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "quotation_sales" {
						postQuotation, err := store.FindQuotationByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postQuotation == nil {
							continue
						}

						err = postQuotation.UndoAccounting()
						if err != nil {
							return err
						}
						err = postQuotation.DoAccounting()
						if err != nil {
							return err
						}

					} else if post.ReferenceModel == "customer_deposit" || post.ReferenceModel == "vendor_deposit" {
						postCustomerDeposit, err := store.FindCustomerDepositByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postCustomerDeposit == nil {
							continue
						}

						err = postCustomerDeposit.UndoAccounting()
						if err != nil {
							return err
						}
						err = postCustomerDeposit.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "customer_withdrawal" || post.ReferenceModel == "vendor_withdrawal" {
						postCustomerWithdrawal, err := store.FindCustomerWithdrawalByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postCustomerWithdrawal == nil {
							continue
						}

						err = postCustomerWithdrawal.UndoAccounting()
						if err != nil {
							return err
						}
						err = postCustomerWithdrawal.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "capital" {
						postCapital, err := store.FindCapitalByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postCapital == nil {
							continue
						}

						err = postCapital.UndoAccounting()
						if err != nil {
							return err
						}
						err = postCapital.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "expense" {
						postExpense, err := store.FindExpenseByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postExpense == nil {
							continue
						}

						err = postExpense.UndoAccounting()
						if err != nil {
							return err
						}
						err = postExpense.DoAccounting()
						if err != nil {
							return err
						}
					} else if post.ReferenceModel == "drawing" {
						postDivident, err := store.FindDividentByID(&post.ReferenceID, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if postDivident == nil {
							continue
						}

						err = postDivident.UndoAccounting()
						if err != nil {
							return err
						}
						err = postDivident.DoAccounting()
						if err != nil {
							return err
						}
					}*/
			}
		}()

		// Wait for goroutine result
		/*if err := <-errCh[account.Name]; err != nil {
			//return err
			fmt.Println("Error:", err)
		}*/
	}

	return nil
}
