package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Account : Account structure
type Account struct {
	ID                   primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID              *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	ReferenceID          *primitive.ObjectID `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	ReferenceModel       *string             `bson:"reference_model,omitempty" json:"reference_model,omitempty"`
	Type                 string              `bson:"type,omitempty" json:"type,omitempty"` //drawing,expense,asset,liability,equity,revenue
	Number               string              `bson:"number,omitempty" json:"number,omitempty"`
	Name                 string              `bson:"name,omitempty" json:"name,omitempty"`
	Phone                *string             `bson:"phone,omitempty" json:"phone,omitempty"`
	Balance              float64             `bson:"balance" json:"balance"`
	DebitOrCreditBalance string              `bson:"debit_or_credit_balance" json:"debit_or_credit_balance"`
	DebitTotal           float64             `bson:"debit_total" json:"debit_total"`
	CreditTotal          float64             `bson:"credit_total" json:"credit_total"`
	Open                 bool                `bson:"open" json:"open"`
	SearchLabel          string              `json:"search_label"`
	CreatedAt            *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt            *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type AccountStats struct {
	ID          *primitive.ObjectID `json:"id" bson:"_id"`
	DebitTotal  float64             `json:"debit_total" bson:"debit_total"`
	CreditTotal float64             `json:"credit_total" bson:"credit_total"`
}

type AccountListStats struct {
	ID                 *primitive.ObjectID `json:"id" bson:"_id"`
	DebitBalanceTotal  float64             `json:"debit_balance_total" bson:"debit_balance_total"`
	CreditBalanceTotal float64             `json:"credit_balance_total" bson:"credit_balance_total"`
}

func GetAccountListStats(filter map[string]interface{}) (stats AccountListStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				"debit_balance_total": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$debit_or_credit_balance", "debit_balance"}},
					"$balance",
					0,
				}}},
				"credit_balance_total": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$debit_or_credit_balance", "credit_balance"}},
					"$balance",
					0,
				}}},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return stats, err
		}
		stats.DebitBalanceTotal = RoundFloat(stats.DebitBalanceTotal, 2)
		stats.CreditBalanceTotal = RoundFloat(stats.CreditBalanceTotal, 2)
	}
	return stats, nil
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
		stats.DebitTotal = RoundFloat(stats.DebitTotal, 2)
		stats.CreditTotal = RoundFloat(stats.CreditTotal, 2)
	}

	account.DebitTotal = stats.DebitTotal
	account.CreditTotal = stats.CreditTotal

	if stats.CreditTotal > stats.DebitTotal {
		account.Balance = RoundFloat((stats.CreditTotal - stats.DebitTotal), 2)
	} else {
		account.Balance = RoundFloat((stats.DebitTotal - stats.CreditTotal), 2)
	}

	if account.Type == "asset" || account.Type == "liability" {
		if stats.CreditTotal > stats.DebitTotal {
			account.Type = "liability" //creditor
		} else {
			account.Type = "asset" //debtor
		}
	}

	if account.Type == "drawing" || account.Type == "expense" || account.Type == "asset" {
		account.DebitOrCreditBalance = "debit_balance"
	} else if account.Type == "liability" || account.Type == "capital" || account.Type == "revenue" {
		account.DebitOrCreditBalance = "credit_balance"
	}

	if account.Balance == 0 {
		account.Open = false
		//account.DebitOrCreditBalance = "na"
	} else {
		account.Open = true
	}

	now := time.Now()
	account.UpdatedAt = &now
	err = account.Update()
	if err != nil {
		return err
	}

	return nil
}

func SearchAccount(w http.ResponseWriter, r *http.Request) (models []Account, criterias SearchCriterias, err error) {
	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := 0.0
	keys, ok := r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	var storeID primitive.ObjectID
	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
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
		}*/

	keys, ok = r.URL.Query()["search[reference_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["reference_code"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[search]"]
	if ok && len(keys[0]) >= 1 {
		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		/*isInteger := true
		intValue, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			isInteger = false
		}
		*/

		criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
			{"number": bson.M{"$regex": keys[0], "$options": "i"}},
		}

		/*
			if isInteger {
				criterias.SearchBy["$or"] = []bson.M{
					{"name": bson.M{"$regex": keys[0], "$options": "i"}},
					{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
					{"number": bson.M{"$regex": intValue, "$options": "i"}},
				}

			}
		*/
	}

	keys, ok = r.URL.Query()["search[phone]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["phone"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[type]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] != "" {
			criterias.SearchBy["type"] = keys[0]
		}
	}

	keys, ok = r.URL.Query()["search[number]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["number"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[balance]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["balance"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["balance"] = value
		}
	}

	keys, ok = r.URL.Query()["search[debit_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["debit_total"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["debit_total"] = value
		}
	}

	keys, ok = r.URL.Query()["search[credit_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["credit_total"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["credit_total"] = value
		}
	}

	keys, ok = r.URL.Query()["search[open]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return models, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["open"] = bson.M{"$eq": true}
		} else if value == 0 {
			criterias.SearchBy["open"] = bson.M{"$eq": false}
		}
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.Replace(keys[0], "stores.", "stores."+storeID.Hex()+".", -1)
		criterias.SortBy = GetSortByFields(keys[0])
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

	var updatedAtStartDate time.Time
	var updatedAtEndDate time.Time

	keys, ok = r.URL.Query()["search[updated_at]"]
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
		criterias.SearchBy["updated_at"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	keys, ok = r.URL.Query()["search[updated_at_from]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		updatedAtStartDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			updatedAtStartDate = ConvertTimeZoneToUTC(timeZoneOffset, updatedAtStartDate)
		}
	}

	keys, ok = r.URL.Query()["search[updated_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		updatedAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return models, criterias, err
		}

		if timeZoneOffset != 0 {
			updatedAtEndDate = ConvertTimeZoneToUTC(timeZoneOffset, updatedAtEndDate)
		}

		updatedAtEndDate = updatedAtEndDate.Add(time.Hour * time.Duration(24))
		updatedAtEndDate = updatedAtEndDate.Add(-time.Second * time.Duration(1))
	}

	if !updatedAtStartDate.IsZero() && !updatedAtEndDate.IsZero() {
		criterias.SearchBy["updated_at"] = bson.M{"$gte": updatedAtStartDate, "$lte": updatedAtEndDate}
	} else if !updatedAtStartDate.IsZero() {
		criterias.SearchBy["updated_at"] = bson.M{"$gte": updatedAtStartDate}
	} else if !updatedAtEndDate.IsZero() {
		criterias.SearchBy["updated_at"] = bson.M{"$lte": updatedAtEndDate}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("account")
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
		return models, criterias, errors.New("Error fetching Customers:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := Account{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		model.SearchLabel = model.Name + " A/c #" + model.Number

		if model.Phone != nil {
			model.SearchLabel += " ph: " + *model.Phone
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil

}

func CreateAccountIfNotExists(
	storeID *primitive.ObjectID,
	referenceID *primitive.ObjectID,
	referenceModel *string,
	name string,
	phone *string,
) (account *Account, err error) {
	if referenceID != nil {
		account, err = FindAccountByReferenceID(*referenceID, *storeID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}

	} else if phone != nil {
		account, err = FindAccountByPhone(*phone, storeID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	} else if referenceID == nil {
		//Only for accounts like Cash,Bank,Sales
		account, err = FindAccountByName(name, storeID, nil, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	if account != nil {
		return account, nil
	}

	startFrom := 1000
	count, err := GetTotalCount(bson.M{"store_id": storeID}, "account")
	if err != nil {
		return nil, err
	}
	accountNumber := strconv.Itoa(startFrom + int(count))

	now := time.Now()
	account = &Account{
		StoreID:        storeID,
		ReferenceID:    referenceID,
		ReferenceModel: referenceModel,
		Name:           name,
		Number:         accountNumber,
		Phone:          phone,
		Balance:        0,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if referenceModel == nil && (name == "Cash" || name == "Bank") {
		account.Type = "asset"
	} else if referenceModel == nil && (name == "Sales") {
		account.Type = "revenue"
	} else if referenceModel == nil && (name == "Sales Return") {
		account.Type = "expense"
	} else if referenceModel == nil && (name == "Purchase") {
		account.Type = "expense"
	} else if referenceModel == nil && (name == "Purchase Return") {
		account.Type = "revenue"
	} else if referenceModel == nil && (name == "Cash discount allowed") {
		account.Type = "expense"
	} else if referenceModel == nil && (name == "Cash discount received") {
		account.Type = "revenue"
	} else if *referenceModel == "investor" {
		account.Type = "capital"
	}

	//account = &accountModel
	err = account.Insert()
	if err != nil {
		return nil, errors.New("error creating new account: " + err.Error())
	}

	return account, nil
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

func SetAccountBalances(accounts map[string]Account) error {
	for _, account := range accounts {
		err := account.CalculateBalance()
		if err != nil {
			return err
		}
	}
	return nil
}
