package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// QuotationPayment : QuotationPayment structure
type QuotationPayment struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
	QuotationID   *primitive.ObjectID `json:"quotation_id" bson:"quotation_id"`
	QuotationCode string              `json:"quotation_code" bson:"quotation_code"`
	Amount        *float64            `json:"amount" bson:"amount"`
	Method        string              `json:"method" bson:"method"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName     string              `json:"store_name" bson:"store_name"`
	Deleted       bool                `bson:"deleted" json:"deleted"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser *User               `json:"deleted_by_user" bson:"-"`
	DeletedAt     *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}

/*
func (quotationPayment *QuotationPayment) AttributesValueChangeEvent(quotationPaymentOld *QuotationPayment) error {

	if quotationPayment.Name != quotationPaymentOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": quotationPayment.ID},
			bson.M{"$pull": bson.M{
				"customer_name": quotationPaymentOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": quotationPayment.ID},
			bson.M{"$push": bson.M{
				"customer_name": quotationPayment.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (quotationPayment *QuotationPayment) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(quotationPayment.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if quotationPayment.StoreID != nil && !quotationPayment.StoreID.IsZero() {
		store, err := FindStoreByID(quotationPayment.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotationPayment.StoreName = store.Name
	} else {
		quotationPayment.StoreName = ""
	}

	if quotationPayment.QuotationID != nil && !quotationPayment.QuotationID.IsZero() {
		quotation, err := store.FindQuotationByID(quotationPayment.QuotationID, bson.M{"id": 1, "code": 1})
		if err != nil && err != mongo.ErrNoDocuments {
			return err
		}

		if quotation != nil {
			quotationPayment.QuotationCode = quotation.Code
		}
	} else {
		quotationPayment.QuotationCode = ""
	}

	if quotationPayment.CreatedBy != nil {
		createdByUser, err := FindUserByID(quotationPayment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotationPayment.CreatedByName = createdByUser.Name
	}

	if quotationPayment.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(quotationPayment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotationPayment.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func (store *Store) SearchQuotationPayment(w http.ResponseWriter, r *http.Request) (models []QuotationPayment, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return models, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	timeZoneOffset := 0.0
	keys, ok = r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	keys, ok = r.URL.Query()["search[created_by_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["created_by_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[method]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["method"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[pay_from_account]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["pay_from_account"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["amount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["amount"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[updated_by_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["updated_by_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[store_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["store_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[quotation_id]"]
	if ok && len(keys[0]) >= 1 {
		quotationID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["quotation_id"] = quotationID
	}

	keys, ok = r.URL.Query()["search[quotation_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["quotation_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
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
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
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
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	} else if !startDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate}
	} else if !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$lte": endDate}
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
	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_payment")
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
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching quotation payment :" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotationPayment := QuotationPayment{}
		err = cur.Decode(&quotationPayment)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, quotationPayment)
	} //end for loop

	return models, criterias, nil
}

func (quotationPayment *QuotationPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(quotationPayment.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	var oldQuotationPayment *QuotationPayment

	if govalidator.IsNull(quotationPayment.Method) {
		errs["method"] = "Payment method is required"
	}

	if govalidator.IsNull(quotationPayment.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, quotationPayment.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		quotationPayment.Date = &date
	}

	if scenario == "update" {
		if quotationPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsQuotationPaymentExists(&quotationPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid quotation payment :" + quotationPayment.ID.Hex()
		}

		oldQuotationPayment, err = store.FindQuotationPaymentByID(&quotationPayment.ID, bson.M{})
		if err != nil {
			errs["quotation_payment"] = err.Error()
			return errs
		}
	}

	if quotationPayment.Amount == nil {
		errs["amount"] = "Amount is required"
	}

	if quotationPayment.Amount != nil && ToFixed(*quotationPayment.Amount, 2) <= 0 {
		errs["amount"] = "Amount should be > 0"
	}

	/*
		quotationpaymentStats, err := GetQuotationPaymentStats(bson.M{"quotation_id": quotationPayment.QuotationID})
		if err != nil {
			return errs
		}
	*/

	quotation, err := store.FindQuotationByID(quotationPayment.QuotationID, bson.M{})
	if err != nil {
		return errs
	}

	/*
		maxAllowedAmount := 0.00

		if scenario == "update" {
			maxAllowedAmount = (quotation.NetTotal - quotation.CashDiscount) - (quotationpaymentStats.TotalPayment - *oldQuotationPayment.Amount)
		} else {
			maxAllowedAmount = (quotation.NetTotal - quotation.CashDiscount) - quotationpaymentStats.TotalPayment
		}

		if *quotationPayment.Amount > RoundFloat(maxAllowedAmount, 2) {
			errs["amount"] = "The amount should not be greater than " + fmt.Sprintf("%.02f", maxAllowedAmount)
		}
	*/

	customer, err := store.FindCustomerByID(quotation.CustomerID, bson.M{})
	if err != nil {
		errs["customer_id"] = "Invalid Customer:" + quotation.CustomerID.Hex()
	}

	customerAccount, err := store.FindAccountByReferenceID(customer.ID, *quotation.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_account"] = "Error finding customer account: " + err.Error()
	}

	if quotationPayment.Method == "customer_account" {
		log.Print("Checking customer account Balance")
		if customerAccount != nil {
			if scenario == "update" {
				extraAmount := 0.00
				if oldQuotationPayment != nil && *oldQuotationPayment.Amount > *quotationPayment.Amount {
					extraAmount = *oldQuotationPayment.Amount - *quotationPayment.Amount
				}

				if extraAmount > 0 {
					if customerAccount.Balance == 0 {
						errs["payment_method"] = "customer account balance is zero, Please add " + fmt.Sprintf("%.02f", (extraAmount)) + " to customer account to continue"
					} else if customerAccount.Type == "asset" {
						errs["payment_method"] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", (customerAccount.Balance+extraAmount)) + " to customer account to continue"
					} else if customerAccount.Type == "liability" && customerAccount.Balance < extraAmount {
						errs["payment_method"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", extraAmount) + " to customer account to continue"
					}
				}

			} else {

				if customerAccount.Balance == 0 {
					errs["payment_method"] = "customer account balance is zero"
				} else if customerAccount.Type == "asset" {
					errs["payment_method"] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance)
				} else if customerAccount.Type == "liability" && customerAccount.Balance < *quotationPayment.Amount {
					errs["payment_method"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance)
				}

			}

		} else {
			errs["payment_method"] = "customer account balance is zero"
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (quotationPayment *QuotationPayment) Insert() error {
	collection := db.GetDB("store_" + quotationPayment.StoreID.Hex()).Collection("quotation_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := quotationPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	quotationPayment.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &quotationPayment)
	if err != nil {
		return err
	}

	return nil
}

func (quotationPayment *QuotationPayment) Update() error {
	collection := db.GetDB("store_" + quotationPayment.StoreID.Hex()).Collection("quotation_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := quotationPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationPayment.ID},
		bson.M{"$set": quotationPayment},
		updateOptions,
	)

	return err
}

func (store *Store) FindQuotationPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotationPayment *QuotationPayment, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_payment")
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
		}, findOneOptions).
		Decode(&quotationPayment)
	if err != nil {
		return nil, err
	}

	return quotationPayment, err
}

func (store *Store) IsQuotationPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

type QuotationPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func (store *Store) GetQuotationPaymentStats(filter map[string]interface{}) (stats QuotationPaymentStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				//"total_payment": bson.M{"$sum": "$amount"},
				"total_payment": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$ne": []interface{}{"$deleted", true}},
					"$amount",
					0,
				}}},
				/*"total_payment": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$ne": []interface{}{
								"$deleted",
								true,
							},
						},
						"then": "$amount",
						"else": 0,
					},
				}},*/
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

		stats.TotalPayment = RoundFloat(stats.TotalPayment, 2)
	}

	return stats, nil
}

func (store *Store) ProcessQuotationPayments() error {
	log.Print("Processing quotation payments")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_payment")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	searchBy := make(map[string]interface{})
	searchBy["deleted"] = bson.M{"$ne": true}
	cur, err := collection.Find(ctx, searchBy, findOptions)
	if err != nil {
		return errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	//productCount := 1
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := QuotationPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		if model.Method == "bank_account" {
			model.Method = "bank_card"
		}

		//model.Date = model.CreatedAt
		//log.Print("Date updated")
		err = model.Update()
		if err != nil {
			log.Print(err)
			return err
		}
	}

	log.Print("DONE!")
	return nil
}

func (quotationPayment *QuotationPayment) DeleteQuotationPayment() (err error) {
	collection := db.GetDB("store_" + quotationPayment.StoreID.Hex()).Collection("quotation_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationPayment.ID},
		bson.M{"$set": quotationPayment},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
