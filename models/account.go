package models

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	NameArabic           string              `bson:"name_arabic,omitempty" json:"name_arabic,omitempty"`
	Phone                *string             `bson:"phone,omitempty" json:"phone,omitempty"`
	VatNo                *string             `bson:"vat_no,omitempty" json:"vat_no,omitempty"`
	Balance              float64             `bson:"balance" json:"balance"`
	DebitOrCreditBalance string              `bson:"debit_or_credit_balance" json:"debit_or_credit_balance"`
	DebitTotal           float64             `bson:"debit_total" json:"debit_total"`
	CreditTotal          float64             `bson:"credit_total" json:"credit_total"`
	Open                 bool                `bson:"open" json:"open"`
	SearchLabel          string              `json:"search_label"`
	CreatedAt            *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt            *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	Deleted              bool                `bson:"deleted" json:"deleted"`
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

func (store *Store) GetAccountListStats(filter map[string]interface{}) (stats AccountListStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
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

func (account *Account) CalculateBalance(beforeDate *time.Time, beforeID *primitive.ObjectID) error {
	collection := db.GetDB("store_" + account.StoreID.Hex()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := map[string]interface{}{
		"account_id": account.ID,
		"store_id":   account.StoreID,
	}

	if beforeDate != nil {
		filter["date"] = bson.M{"$lt": beforeDate}
	}

	/*
		if beforeID != nil {
			filter["posts._id"] = bson.M{"$lt": beforeID}
		}*/

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

	if account.ReferenceModel != nil && (*account.ReferenceModel == "customer" || *account.ReferenceModel == "vendor") {
		if stats.CreditTotal > stats.DebitTotal {
			account.Type = "liability" //creditor
		} else if stats.CreditTotal < stats.DebitTotal {
			account.Type = "asset" //debtor
		}
	}

	if account.Type == "asset" || account.Type == "liability" {
		if stats.CreditTotal > stats.DebitTotal {
			account.Type = "liability" //creditor
		} else if stats.CreditTotal < stats.DebitTotal {
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

	keys, ok = r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return models, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
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
		criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"name_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}

		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	keys, ok = r.URL.Query()["search[vat_no]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["vat_no"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	var collection *mongo.Collection
	if !storeID.IsZero() {
		collection = db.GetDB("store_" + storeID.Hex()).Collection("account")
	} else {
		collection = db.GetDB("").Collection("account")
	}

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

/*
	 else if phone != nil && !govalidator.IsNull(strings.TrimSpace(*phone)) {
			account, err = store.FindAccountByPhoneByName(*phone, name, storeID, bson.M{})
			if err != nil && err != mongo.ErrNoDocuments {
				return nil, err
			}
		}
*/
func (store *Store) CreateAccountIfNotExists(
	storeID *primitive.ObjectID,
	referenceID *primitive.ObjectID,
	referenceModel *string,
	name string,
	phone *string,
	vatNo *string,
) (account *Account, err error) {
	name = strings.ToUpper(name)
	if vatNo != nil && !govalidator.IsNull(strings.TrimSpace(*vatNo)) {
		account, err = store.FindAccountByVatNoByName(*vatNo, name, storeID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	} else if referenceID != nil && !referenceID.IsZero() {
		account, err = store.FindAccountByReferenceIDByName(*referenceID, name, *storeID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}

	} else if referenceID == nil && !govalidator.IsNull(strings.TrimSpace(name)) {
		//Only for accounts like Cash,Bank,Sales
		account, err = store.FindAccountByName(name, storeID, nil, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	if account != nil {
		return account, nil
	}

	startFrom := 1000
	count, err := store.GetTotalCount(bson.M{"store_id": storeID}, "account")
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
		VatNo:          vatNo,
		Balance:        0,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	if referenceModel == nil && (name == "CASH" || name == "BANK") {
		account.Type = "asset"
	} else if referenceModel == nil && (name == "SALES") {
		account.Type = "revenue"
	} else if referenceModel == nil && (name == "SALES RETURN") {
		account.Type = "expense"
	} else if referenceModel == nil && (name == "PURCHASE") {
		account.Type = "expense"
	} else if referenceModel == nil && (name == "PURCHASE RETURN") {
		account.Type = "revenue"
	} else if referenceModel == nil && (name == "CASH DISCOUNT ALLOWED") {
		account.Type = "expense"
	} else if referenceModel == nil && (name == "CASH DISCOUNT RECEIVED") {
		account.Type = "revenue"
	} else if referenceModel != nil && *referenceModel == "investor" {
		account.Type = "capital"
	} else if referenceModel != nil && *referenceModel == "withdrawer" {
		account.Type = "drawing"
	} else if referenceModel != nil && *referenceModel == "expense_category" {
		account.Type = "expense"
	}

	//account = &accountModel
	err = account.Insert()
	if err != nil {
		return nil, errors.New("error creating new account: " + err.Error())
	}

	return account, nil
}

func (account *Account) Insert() error {
	collection := db.GetDB("store_" + account.StoreID.Hex()).Collection("account")

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
	collection := db.GetDB("store_" + account.StoreID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	now := time.Now()
	account.UpdatedAt = &now

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

func (store *Store) FindAccountByID(
	ID primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"_id":      ID,
			"store_id": store.ID,
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByReferenceID(
	referenceID primitive.ObjectID,
	storeID primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	totalCount, err := store.GetTotalCount(bson.M{
		"reference_id": referenceID,
		"store_id":     store.ID,
		"deleted":      bson.M{"$ne": true},
	}, "account")
	if err != nil {
		return nil, err
	}

	criteria := bson.M{
		"reference_id": referenceID,
		"store_id":     store.ID,
		"deleted":      bson.M{"$ne": true},
	}

	if totalCount > 1 {
		criteria = bson.M{
			"reference_id": referenceID,
			"store_id":     store.ID,
			"open":         true,
			"deleted":      bson.M{"$ne": true},
		}
	}

	err = collection.FindOne(ctx,
		criteria, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByReferenceIDByName(
	referenceID primitive.ObjectID,
	name string,
	storeID primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"reference_id": referenceID,
			"name":         name,
			"store_id":     store.ID,
			"deleted":      bson.M{"$ne": true},
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByPhoneByName(
	phone string,
	name string,
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"phone":    phone,
			"name":     name,
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByVatNoByName(
	vatNo string,
	name string,
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"vat_no":   vatNo,
			"name":     name,
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByVatNo(
	vatNo string,
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"vat_no": vatNo,
			//"name":     name,
			"store_id": storeID,
			"deleted":  bson.M{"$ne": true},
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByPhone(
	phone string,
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
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
			"deleted":  bson.M{"$ne": true},
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&account)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return account, err
}

func (store *Store) FindAccountByName(
	name string,
	storeID *primitive.ObjectID,
	referenceID *primitive.ObjectID,
	selectFields map[string]interface{},
) (account *Account, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	filter := bson.M{
		"name":     name,
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	}

	if referenceID != nil {
		filter["reference_id"] = nil
	}

	err = collection.FindOne(ctx, filter, findOneOptions). //"deleted": bson.M{"$ne": true}
								Decode(&account)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return account, err
}

func (account *Account) IsPhoneExists() (exists bool, err error) {
	collection := db.GetDB("store_" + account.StoreID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if account.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone":    account.Phone,
			"store_id": account.StoreID,
			"deleted":  bson.M{"$ne": true},
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone":    account.Phone,
			"store_id": account.StoreID,
			"deleted":  bson.M{"$ne": true},
			"_id":      bson.M{"$ne": account.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) IsAccountExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func SetAccountBalances(accounts map[string]Account) error {
	for _, account := range accounts {
		err := account.CalculateBalance(nil, nil)
		if err != nil {
			return err
		}

		err = account.Update()
		if err != nil {
			return err
		}
	}
	return nil
}

func ProcessAccounts() error {
	log.Print("Processing accounts")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		totalCount, err := store.GetTotalCount(bson.M{}, "account")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)

		cur, err := collection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			return errors.New("Error fetching accounts" + err.Error())
		}
		if cur != nil {
			defer cur.Close(ctx)
		}

		bar := progressbar.Default(totalCount)
		for i := 0; cur != nil && cur.Next(ctx); i++ {
			err := cur.Err()
			if err != nil {
				return errors.New("Cursor error:" + err.Error())
			}
			model := Account{}
			err = cur.Decode(&model)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if model.ReferenceModel != nil && *model.ReferenceModel == "customer" && model.ReferenceID != nil && model.Open {
				customer, err := store.FindCustomerByID(model.ReferenceID, bson.M{})
				if err != nil && err != mongo.ErrNoDocuments {
					return err
				}

				if customer == nil {
					continue
				}

				customer.Account = &model
				err = customer.Update()
				if err != nil {
					return err
				}

				model.Name = customer.Name
				model.NameArabic = customer.NameInArabic
				err = model.Update()
				if err != nil {
					return err
				}
			}

			if model.ReferenceModel != nil && *model.ReferenceModel == "vendor" && model.ReferenceID != nil && model.Open {
				vendor, err := store.FindVendorByID(model.ReferenceID, bson.M{})
				if err != nil && err != mongo.ErrNoDocuments {
					return err
				}

				if vendor == nil {
					continue
				}

				vendor.Account = &model
				err = vendor.Update()
				if err != nil {
					return err
				}

				model.Name = vendor.Name
				model.NameArabic = vendor.NameInArabic

				err = model.Update()
				if err != nil {
					return err
				}
			}
			/*
				if model.ReferenceModel != nil && *model.ReferenceModel == "expense_category" {
					model.Type = "expense"
				}

				err = model.Update()
				if err != nil {
					return err
				}*/

			bar.Add(1)
		}
	}
	log.Print("Accounts DONE!")
	return nil
}

func (account *Account) DeleteAccount(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + account.StoreID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	account.Deleted = true

	//productCategory.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": account.ID},
		bson.M{"$set": account},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (account *Account) RestoreAccount(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + account.StoreID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	account.Deleted = false

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": account.ID},
		bson.M{"$set": account},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
