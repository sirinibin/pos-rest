package models

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	store, err := FindStoreByID(purchaseCashDiscount.StoreID, bson.M{})
	if err != nil {
		return err
	}

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
		purchase, err := store.FindPurchaseByID(purchaseCashDiscount.PurchaseID, bson.M{"id": 1, "code": 1})
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

func (store *Store) SearchPurchaseCashDiscount(w http.ResponseWriter, r *http.Request) (models []PurchaseCashDiscount, criterias SearchCriterias, err error) {

	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	var keys []string
	var ok bool

	ParseTextSearch(r, &criterias, "search[created_by_name]", "created_by_name")

	if err = ParseFloatWithOperatorFilter(r, &criterias, "search[amount]", "amount"); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[updated_by_name]", "updated_by_name")

	ParseTextSearch(r, &criterias, "search[store_name]", "store_name")

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return models, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[purchase_id]", "purchase_id"); err != nil {
		return models, criterias, err
	}

	if err = ParseObjectIDListFilter(r, &criterias, "search[created_by]", "created_by"); err != nil {
		return models, criterias, err
	}

	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_cash_discount")

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

	store, err := FindStoreByID(purchaseCashDiscount.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	var oldPurchaseCashDiscount *PurchaseCashDiscount

	if scenario == "update" {
		if purchaseCashDiscount.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsPurchaseCashDiscountExists(&purchaseCashDiscount.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid purchase cash discount:" + purchaseCashDiscount.ID.Hex()
		}

		oldPurchaseCashDiscount, err = store.FindPurchaseCashDiscountByID(&purchaseCashDiscount.ID, bson.M{})
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

	purchaseCashDiscountStats, err := store.GetPurchaseCashDiscountStats(bson.M{"purchase_id": purchaseCashDiscount.PurchaseID})
	if err != nil {
		return errs
	}

	purchase, err := store.FindPurchaseByID(purchaseCashDiscount.PurchaseID, bson.M{})
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
	collection := db.GetDB("store_" + purchaseCashDiscount.StoreID.Hex()).Collection("purchase_cash_discount")
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
	collection := db.GetDB("store_" + purchaseCashDiscount.StoreID.Hex()).Collection("purchase_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
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

func (store *Store) FindPurchaseCashDiscountByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchaseCashDiscount *PurchaseCashDiscount, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_cash_discount")
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
		Decode(&purchaseCashDiscount)
	if err != nil {
		return nil, err
	}

	return purchaseCashDiscount, err
}

func (store *Store) IsPurchaseCashDiscountExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_cash_discount")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

type PurchaseCashDiscountStats struct {
	ID                *primitive.ObjectID `json:"id" bson:"_id"`
	TotalCashDiscount float64             `json:"total_cash_discount" bson:"total_cash_discount"`
}

func (store *Store) GetPurchaseCashDiscountStats(filter map[string]interface{}) (stats PurchaseCashDiscountStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_cash_discount")
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
