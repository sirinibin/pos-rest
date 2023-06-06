package models

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// PurchaseCashDiscount : PurchaseCashDiscount structure
type PurchaseCashDiscount struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	PurchaseID    *primitive.ObjectID `json:"purchase_id" bson:"purchase_id"`
	PurchaseCode  string              `json:"purchase_code" bson:"purchase_code"`
	Amount        float64             `json:"amount" bson:"amount"`
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
func (purchaseCashDiscount *PurchaseCashDiscount) AttributesValueChangeEvent(purchaseCashDiscountOld *PurchaseCashDiscount) error {

	if purchaseCashDiscount.Name != purchaseCashDiscountOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": purchaseCashDiscount.ID},
			bson.M{"$pull": bson.M{
				"customer_name": purchaseCashDiscountOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": purchaseCashDiscount.ID},
			bson.M{"$push": bson.M{
				"customer_name": purchaseCashDiscount.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (purchaseCashDiscount *PurchaseCashDiscount) UpdateForeignLabelFields() error {

	if purchaseCashDiscount.StoreID != nil && !purchaseCashDiscount.StoreID.IsZero() {
		store, err := FindStoreByID(purchaseCashDiscount.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchaseCashDiscount.StoreName = store.Name
	} else {
		purchaseCashDiscount.StoreName = ""
	}

	if purchaseCashDiscount.PurchaseID != nil && !purchaseCashDiscount.PurchaseID.IsZero() {
		purchase, err := FindPurchaseByID(purchaseCashDiscount.PurchaseID, bson.M{"id": 1, "code": 1})
		if err != nil {
			return err
		}
		purchaseCashDiscount.PurchaseCode = purchase.Code
	} else {
		purchaseCashDiscount.PurchaseCode = ""
	}

	if purchaseCashDiscount.CreatedBy != nil {
		createdByUser, err := FindUserByID(purchaseCashDiscount.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchaseCashDiscount.CreatedByName = createdByUser.Name
	}

	if purchaseCashDiscount.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(purchaseCashDiscount.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchaseCashDiscount.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func SearchPurchaseCashDiscount(w http.ResponseWriter, r *http.Request) (models []PurchaseCashDiscount, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[purchase_id]"]
	if ok && len(keys[0]) >= 1 {
		purchaseID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["purchase_id"] = purchaseID
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

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_cash_discount")
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
		return models, criterias, errors.New("Error fetching purchase cash discount:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		purchaseCashDiscount := PurchaseCashDiscount{}
		err = cur.Decode(&purchaseCashDiscount)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, purchaseCashDiscount)
	} //end for loop

	return models, criterias, nil
}

func (purchaseCashDiscount *PurchaseCashDiscount) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	var oldPurchaseCashDiscount *PurchaseCashDiscount

	if scenario == "update" {
		if purchaseCashDiscount.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsPurchaseCashDiscountExists(&purchaseCashDiscount.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid purchase cash discount:" + purchaseCashDiscount.ID.Hex()
		}

		oldPurchaseCashDiscount, err = FindPurchaseCashDiscountByID(&purchaseCashDiscount.ID, bson.M{})
		if err != nil {
			errs["sales_cash_discount"] = err.Error()
			return errs
		}

	}

	if purchaseCashDiscount.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if purchaseCashDiscount.Amount < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	purchaseCashDiscountStats, err := GetPurchaseCashDiscountStats(bson.M{"purchase_id": purchaseCashDiscount.PurchaseID})
	if err != nil {
		return errs
	}

	purchase, err := FindPurchaseByID(purchaseCashDiscount.PurchaseID, bson.M{})
	if err != nil {
		return errs
	}
	if scenario == "update" {
		if ((purchaseCashDiscountStats.TotalCashDiscount - oldPurchaseCashDiscount.Amount) + purchaseCashDiscount.Amount) >= purchase.Total {
			errs["amount"] = "Amount should be less than " + fmt.Sprintf("%.02f", (purchase.Total-(purchaseCashDiscountStats.TotalCashDiscount-oldPurchaseCashDiscount.Amount)))
		}
	} else {
		if (purchaseCashDiscountStats.TotalCashDiscount + purchaseCashDiscount.Amount) >= purchase.Total {
			errs["amount"] = "Amount should be less than " + fmt.Sprintf("%.02f", (purchase.Total-purchaseCashDiscountStats.TotalCashDiscount))
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (purchaseCashDiscount *PurchaseCashDiscount) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := purchaseCashDiscount.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	purchaseCashDiscount.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &purchaseCashDiscount)
	if err != nil {
		return err
	}
	return nil
}

func (purchaseCashDiscount *PurchaseCashDiscount) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := purchaseCashDiscount.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchaseCashDiscount.ID},
		bson.M{"$set": purchaseCashDiscount},
		updateOptions,
	)
	return err
}

func FindPurchaseCashDiscountByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchaseCashDiscount *PurchaseCashDiscount, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&purchaseCashDiscount)
	if err != nil {
		return nil, err
	}

	return purchaseCashDiscount, err
}

func IsPurchaseCashDiscountExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

type PurchaseCashDiscountStats struct {
	ID                *primitive.ObjectID `json:"id" bson:"_id"`
	TotalCashDiscount float64             `json:"total_cash_discount" bson:"total_cash_discount"`
}

func GetPurchaseCashDiscountStats(filter map[string]interface{}) (stats PurchaseCashDiscountStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_cash_discount")
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

		stats.TotalCashDiscount = math.Round(stats.TotalCashDiscount*100) / 100
	}

	return stats, nil
}
