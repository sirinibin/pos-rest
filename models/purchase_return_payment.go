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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

//PurchaseReturnPayment : PurchaseReturnPayment structure
type PurchaseReturnPayment struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	PurchaseReturnID   *primitive.ObjectID `json:"purchase_return_id" bson:"purchase_return_id"`
	PurchaseReturnCode string              `json:"purchase_return_code" bson:"purchase_return_code"`
	PurchaseID         *primitive.ObjectID `json:"purchase_id" bson:"purchase_id"`
	PurchaseCode       string              `json:"purchase_code" bson:"purchase_code"`
	Amount             float64             `json:"amount" bson:"amount"`
	Method             string              `json:"method" bson:"method"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName      string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID            *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName          string              `json:"store_name" bson:"store_name"`
}

/*
func (purchasereturnPayment *PurchaseReturnPayment) AttributesValueChangeEvent(purchasereturnPaymentOld *PurchaseReturnPayment) error {

	if purchasereturnPayment.Name != purchasereturnPaymentOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": purchasereturnPayment.ID},
			bson.M{"$pull": bson.M{
				"customer_name": purchasereturnPaymentOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": purchasereturnPayment.ID},
			bson.M{"$push": bson.M{
				"customer_name": purchasereturnPayment.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (purchasereturnPayment *PurchaseReturnPayment) UpdateForeignLabelFields() error {

	if purchasereturnPayment.StoreID != nil && !purchasereturnPayment.StoreID.IsZero() {
		store, err := FindStoreByID(purchasereturnPayment.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturnPayment.StoreName = store.Name
	} else {
		purchasereturnPayment.StoreName = ""
	}

	if purchasereturnPayment.PurchaseReturnID != nil && !purchasereturnPayment.PurchaseReturnID.IsZero() {
		purchaseReturn, err := FindPurchaseReturnByID(purchasereturnPayment.PurchaseReturnID, bson.M{"id": 1, "code": 1})
		if err != nil {
			return err
		}
		purchasereturnPayment.PurchaseReturnCode = purchaseReturn.Code
	} else {
		purchasereturnPayment.PurchaseReturnCode = ""
	}

	if purchasereturnPayment.PurchaseID != nil && !purchasereturnPayment.PurchaseID.IsZero() {
		purchase, err := FindPurchaseByID(purchasereturnPayment.PurchaseID, bson.M{"id": 1, "code": 1})
		if err != nil && err != mongo.ErrNoDocuments {
			return err
		}
		if err != mongo.ErrNoDocuments {
			purchasereturnPayment.PurchaseCode = purchase.Code
		}

	} else {
		purchasereturnPayment.PurchaseCode = ""
	}

	if purchasereturnPayment.CreatedBy != nil {
		createdByUser, err := FindUserByID(purchasereturnPayment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturnPayment.CreatedByName = createdByUser.Name
	}

	if purchasereturnPayment.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(purchasereturnPayment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		purchasereturnPayment.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func SearchPurchaseReturnPayment(w http.ResponseWriter, r *http.Request) (models []PurchaseReturnPayment, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[method]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["method"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
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

	keys, ok = r.URL.Query()["search[purchase_return_id]"]
	if ok && len(keys[0]) >= 1 {
		purchaseReturnID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["purchase_return_id"] = purchaseReturnID
	}

	keys, ok = r.URL.Query()["search[purchase_return_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["purchase_return_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[purchase_id]"]
	if ok && len(keys[0]) >= 1 {
		purchaseID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["purchase_id"] = purchaseID
	}

	keys, ok = r.URL.Query()["search[purchase_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["purchase_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching purchase return payment :" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		purchasereturnPayment := PurchaseReturnPayment{}
		err = cur.Decode(&purchasereturnPayment)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, purchasereturnPayment)
	} //end for loop

	return models, criterias, nil
}

func (purchasereturnPayment *PurchaseReturnPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	var oldPurchaseReturnPayment *PurchaseReturnPayment

	if scenario == "update" {
		if purchasereturnPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsPurchaseReturnPaymentExists(&purchasereturnPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid purchase return payment :" + purchasereturnPayment.ID.Hex()
		}

		oldPurchaseReturnPayment, err = FindPurchaseReturnPaymentByID(&purchasereturnPayment.ID, bson.M{})
		if err != nil {
			errs["purchase_return_payment"] = err.Error()
			return errs
		}
	}

	if purchasereturnPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if purchasereturnPayment.Amount < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	purchaseReturnPaymentStats, err := GetPurchaseReturnPaymentStats(bson.M{"purchase_return_id": purchasereturnPayment.PurchaseReturnID})
	if err != nil {
		return errs
	}

	purchaseReturn, err := FindPurchaseReturnByID(purchasereturnPayment.PurchaseReturnID, bson.M{})
	if err != nil {
		return errs
	}

	if scenario == "update" {
		if ((purchaseReturnPaymentStats.TotalPayment - oldPurchaseReturnPayment.Amount) + purchasereturnPayment.Amount) > purchaseReturn.NetTotal {
			if (purchaseReturn.NetTotal - (purchaseReturnPaymentStats.TotalPayment - oldPurchaseReturnPayment.Amount)) > 0 {
				errs["amount"] = "Vendor has already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (purchaseReturn.NetTotal-(purchaseReturnPaymentStats.TotalPayment-oldPurchaseReturnPayment.Amount)))
			} else {
				errs["amount"] = "Vendor has already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR"
			}

		}
	} else {
		if (purchaseReturnPaymentStats.TotalPayment + purchasereturnPayment.Amount) > purchaseReturn.NetTotal {
			if (purchaseReturn.NetTotal - purchaseReturnPaymentStats.TotalPayment) > 0 {
				errs["amount"] = "Vendor has already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (purchaseReturn.NetTotal-purchaseReturnPaymentStats.TotalPayment))
			} else {
				errs["amount"] = "Vendor has already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR"
			}

		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (purchasereturnPayment *PurchaseReturnPayment) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := purchasereturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	purchasereturnPayment.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &purchasereturnPayment)
	if err != nil {
		return err
	}
	return nil
}

func (purchasereturnPayment *PurchaseReturnPayment) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := purchasereturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": purchasereturnPayment.ID},
		bson.M{"$set": purchasereturnPayment},
		updateOptions,
	)
	return err
}

func FindPurchaseReturnPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchasereturnPayment *PurchaseReturnPayment, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&purchasereturnPayment)
	if err != nil {
		return nil, err
	}

	return purchasereturnPayment, err
}

func IsPurchaseReturnPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

type PurchaseReturnPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func GetPurchaseReturnPaymentStats(filter map[string]interface{}) (stats PurchaseReturnPaymentStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":           nil,
				"total_payment": bson.M{"$sum": "$amount"},
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

		stats.TotalPayment = math.Round(stats.TotalPayment*100) / 100
	}

	return stats, nil
}
