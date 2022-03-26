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
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

//SalesCashDiscount : SalesCashDiscount structure
type SalesCashDiscount struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	OrderID       *primitive.ObjectID `json:"order_id" bson:"order_id"`
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

	if salesCashDiscount.StoreID != nil && !salesCashDiscount.StoreID.IsZero() {
		store, err := FindStoreByID(salesCashDiscount.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesCashDiscount.StoreName = store.Name
	} else {
		salesCashDiscount.StoreName = ""
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

func SearchSalesCashDiscount(w http.ResponseWriter, r *http.Request) (productCategories []SalesCashDiscount, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[parent_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["parent_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return productCategories, criterias, err
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
			return productCategories, criterias, err
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
			return productCategories, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return productCategories, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("product_category")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

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
		return productCategories, criterias, errors.New("Error fetching product categories:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return productCategories, criterias, errors.New("Cursor error:" + err.Error())
		}
		salesCashDiscount := SalesCashDiscount{}
		err = cur.Decode(&salesCashDiscount)
		if err != nil {
			return productCategories, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		productCategories = append(productCategories, salesCashDiscount)
	} //end for loop

	return productCategories, criterias, nil
}

func (salesCashDiscount *SalesCashDiscount) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if salesCashDiscount.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsSalesCashDiscountExists(&salesCashDiscount.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Product Category:" + salesCashDiscount.ID.Hex()
		}

	}

	/*
		if govalidator.IsNull(salesCashDiscount.Name) {
			errs["name"] = "Name is required"
		}
	*/

	/*
		nameExists, err := salesCashDiscount.IsNameExists()
		if err != nil {
			errs["name"] = err.Error()
		}

		if nameExists {
			errs["name"] = "Name is Already in use"
		}
		*

		if nameExists {
			w.WriteHeader(http.StatusConflict)
		} else if len(errs) > 0 {
			w.WriteHeader(http.StatusBadRequest)
		}
	*/

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

/*
func (salesCashDiscount *SalesCashDiscount) IsNameExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if salesCashDiscount.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": salesCashDiscount.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": salesCashDiscount.Name,
			"_id":  bson.M{"$ne": salesCashDiscount.ID},
		})
	}

	return (count == 1), err
}
*/

func (salesCashDiscount *SalesCashDiscount) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_category")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("product_category")
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

func FindSalesCashDiscountByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesCashDiscount *SalesCashDiscount, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&salesCashDiscount)
	if err != nil {
		return nil, err
	}

	return salesCashDiscount, err
}

func IsSalesCashDiscountExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
