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
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// SalesCashDiscount : SalesCashDiscount structure
type SalesCashDiscount struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
	OrderID       *primitive.ObjectID `json:"order_id" bson:"order_id"`
	OrderCode     string              `json:"order_code" bson:"order_code"`
	Amount        float64             `json:"amount" bson:"amount"`
	Method        string              `json:"method" bson:"method"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName     string              `json:"store_name" bson:"store_name"`
}

/*
func (salesCashDiscount *SalesCashDiscount) AttributesValueChangeEvent(salesCashDiscountOld *SalesCashDiscount) error {

	if salesCashDiscount.Name != salesCashDiscountOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesCashDiscount.ID},
			bson.M{"$pull": bson.M{
				"customer_name": salesCashDiscountOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesCashDiscount.ID},
			bson.M{"$push": bson.M{
				"customer_name": salesCashDiscount.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (salesCashDiscount *SalesCashDiscount) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(salesCashDiscount.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if salesCashDiscount.StoreID != nil && !salesCashDiscount.StoreID.IsZero() {
		store, err := FindStoreByID(salesCashDiscount.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesCashDiscount.StoreName = store.Name
	} else {
		salesCashDiscount.StoreName = ""
	}

	if salesCashDiscount.OrderID != nil && !salesCashDiscount.OrderID.IsZero() {
		order, err := store.FindOrderByID(salesCashDiscount.OrderID, bson.M{"id": 1, "code": 1})
		if err != nil {
			return err
		}
		salesCashDiscount.OrderCode = order.Code
	} else {
		salesCashDiscount.OrderCode = ""
	}

	if salesCashDiscount.CreatedBy != nil {
		createdByUser, err := FindUserByID(salesCashDiscount.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesCashDiscount.CreatedByName = createdByUser.Name
	}

	if salesCashDiscount.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(salesCashDiscount.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesCashDiscount.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func (store *Store) SearchSalesCashDiscount(w http.ResponseWriter, r *http.Request) (models []SalesCashDiscount, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[method]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["method"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["created_by_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	keys, ok = r.URL.Query()["search[order_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["order_id"] = orderID
	}

	keys, ok = r.URL.Query()["search[order_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["order_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_cash_discount")

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
		return models, criterias, errors.New("Error fetching sales cash discount:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		salesCashDiscount := SalesCashDiscount{}
		err = cur.Decode(&salesCashDiscount)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, salesCashDiscount)
	} //end for loop

	return models, criterias, nil
}

func (salesCashDiscount *SalesCashDiscount) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(salesCashDiscount.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	var oldSalesCashDiscount *SalesCashDiscount

	if govalidator.IsNull(salesCashDiscount.Method) {
		errs["method"] = "Payment method is required"
	}

	if govalidator.IsNull(salesCashDiscount.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, salesCashDiscount.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		salesCashDiscount.Date = &date
	}

	if scenario == "update" {
		if salesCashDiscount.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsSalesCashDiscountExists(&salesCashDiscount.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid sales cash discount:" + salesCashDiscount.ID.Hex()
		}

		oldSalesCashDiscount, err = store.FindSalesCashDiscountByID(&salesCashDiscount.ID, bson.M{})
		if err != nil {
			errs["sales_cash_discount"] = err.Error()
			return errs
		}
	}

	if salesCashDiscount.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if salesCashDiscount.Amount < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	salescashdiscountStats, err := store.GetSalesCashDiscountStats(bson.M{"order_id": salesCashDiscount.OrderID})
	if err != nil {
		return errs
	}

	order, err := store.FindOrderByID(salesCashDiscount.OrderID, bson.M{})
	if err != nil {
		return errs
	}

	if scenario == "update" {
		if ((salescashdiscountStats.TotalCashDiscount - oldSalesCashDiscount.Amount) + salesCashDiscount.Amount) >= order.TotalPaymentReceived {
			errs["amount"] = "Amount should be less than " + fmt.Sprintf("%.02f", (order.TotalPaymentReceived-(salescashdiscountStats.TotalCashDiscount-oldSalesCashDiscount.Amount)))
		}
	} else {
		if (salescashdiscountStats.TotalCashDiscount + salesCashDiscount.Amount) >= order.TotalPaymentReceived {
			errs["amount"] = "Amount should be less than " + fmt.Sprintf("%.02f", (order.TotalPaymentReceived-salescashdiscountStats.TotalCashDiscount))
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (salesCashDiscount *SalesCashDiscount) Insert() error {
	collection := db.GetDB("store_" + salesCashDiscount.StoreID.Hex()).Collection("sales_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := salesCashDiscount.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	salesCashDiscount.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &salesCashDiscount)
	if err != nil {
		return err
	}
	return nil
}

func (salesCashDiscount *SalesCashDiscount) Update() error {
	collection := db.GetDB("store_" + salesCashDiscount.StoreID.Hex()).Collection("sales_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := salesCashDiscount.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesCashDiscount.ID},
		bson.M{"$set": salesCashDiscount},
		updateOptions,
	)
	return err
}

func (store *Store) FindSalesCashDiscountByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesCashDiscount *SalesCashDiscount, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_cash_discount")
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
		Decode(&salesCashDiscount)
	if err != nil {
		return nil, err
	}

	return salesCashDiscount, err
}

func (store *Store) IsSalesCashDiscountExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

type SalesCashDiscountStats struct {
	ID                *primitive.ObjectID `json:"id" bson:"_id"`
	TotalCashDiscount float64             `json:"total_cash_discount" bson:"total_cash_discount"`
}

func (store *Store) GetSalesCashDiscountStats(filter map[string]interface{}) (stats SalesCashDiscountStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                 nil,
				"total_cash_discount": bson.M{"$sum": "$amount"},
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

		stats.TotalCashDiscount = RoundFloat(stats.TotalCashDiscount, 2)
	}

	return stats, nil
}

func (store *Store) ProcessSalesCashDiscounts() error {
	log.Print("Processing sales cash discount")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_cash_discount")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
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
		model := SalesCashDiscount{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		order, _ := store.FindOrderByID(model.OrderID, bson.M{})
		order.CashDiscount = model.Amount
		order.Update()
		//model.Date = model.CreatedAt
		//model.Method = "cash"

		err = model.Update()
		if err != nil {
			return err
		}
	}

	log.Print("DONE!")
	return nil
}
