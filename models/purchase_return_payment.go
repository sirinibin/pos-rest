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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PurchaseReturnPayment : PurchaseReturnPayment structure
type PurchaseReturnPayment struct {
	ID                  primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr             string              `json:"date_str,omitempty" bson:"-"`
	PurchaseReturnID    *primitive.ObjectID `json:"purchase_return_id" bson:"purchase_return_id"`
	PurchaseReturnCode  string              `json:"purchase_return_code" bson:"purchase_return_code"`
	PurchaseID          *primitive.ObjectID `json:"purchase_id" bson:"purchase_id"`
	PurchaseCode        string              `json:"purchase_code" bson:"purchase_code"`
	Amount              *float64            `json:"amount" bson:"amount"`
	Method              string              `json:"method" bson:"method"`
	CreatedAt           *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt           *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy           *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy           *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName       string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName       string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID             *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName           string              `json:"store_name" bson:"store_name"`
	Deleted             bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy           *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser       *User               `json:"deleted_by_user,omitempty"`
	DeletedAt           *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	ReceivableID        *primitive.ObjectID `json:"receivable_id" bson:"receivable_id"`
	ReceivablePaymentID *primitive.ObjectID `json:"receivable_payment_id" bson:"receivable_payment_id"`
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
	store, err := FindStoreByID(purchasereturnPayment.StoreID, bson.M{})
	if err != nil {
		return err
	}

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
		purchaseReturn, err := store.FindPurchaseReturnByID(purchasereturnPayment.PurchaseReturnID, bson.M{"id": 1, "code": 1})
		if err != nil {
			return err
		}
		purchasereturnPayment.PurchaseReturnCode = purchaseReturn.Code
	} else {
		purchasereturnPayment.PurchaseReturnCode = ""
	}

	if purchasereturnPayment.PurchaseID != nil && !purchasereturnPayment.PurchaseID.IsZero() {
		purchase, err := store.FindPurchaseByID(purchasereturnPayment.PurchaseID, bson.M{"id": 1, "code": 1})
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

func (store *Store) SearchPurchaseReturnPayment(w http.ResponseWriter, r *http.Request) (models []PurchaseReturnPayment, criterias SearchCriterias, err error) {

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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_return_payment")
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

	store, err := FindStoreByID(purchasereturnPayment.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	var oldPurchaseReturnPayment *PurchaseReturnPayment

	if govalidator.IsNull(purchasereturnPayment.Method) {
		errs["method"] = "Payment method is required"
	}

	if govalidator.IsNull(purchasereturnPayment.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, purchasereturnPayment.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		purchasereturnPayment.Date = &date
	}

	if scenario == "update" {
		if purchasereturnPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsPurchaseReturnPaymentExists(&purchasereturnPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid purchase return payment :" + purchasereturnPayment.ID.Hex()
		}

		oldPurchaseReturnPayment, err = store.FindPurchaseReturnPaymentByID(&purchasereturnPayment.ID, bson.M{})
		if err != nil {
			errs["purchase_return_payment"] = err.Error()
			return errs
		}

	}

	if purchasereturnPayment.Amount == nil {
		errs["amount"] = "Amount is required"
	} else if ToFixed(*purchasereturnPayment.Amount, 2) <= 0 {
		errs["amount"] = "Amount should be > 0"
	}

	purchaseReturn, err := store.FindPurchaseReturnByID(purchasereturnPayment.PurchaseReturnID, bson.M{})
	if err != nil {
		errs["sales_return"] = "error finding sales return" + err.Error()
	}

	if *purchasereturnPayment.Amount > RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount) {
		errs["amount"] = "Amount should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if scenario == "update" {
		if (*purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid - *oldPurchaseReturnPayment.Amount)) > RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	} else {
		if (*purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid)) > RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	}

	//validating with payment paid payment in purchase
	purchase, err := store.FindOrderByID(purchasereturnPayment.PurchaseID, bson.M{})
	if err != nil {
		errs["sales"] = "error finding sale" + err.Error()
	}

	if scenario == "update" {
		if (*purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid - *oldPurchaseReturnPayment.Amount)) > purchase.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (purchase.TotalPaymentReceived)) + " (Total Paid payment)"
			return
		}
	} else {
		if (*purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid)) > purchase.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (purchase.TotalPaymentReceived)) + " (Total Paid payment)"
			return
		}
	}

	/*
		purchaseReturnPaymentStats, err := GetPurchaseReturnPaymentStats(bson.M{"purchase_return_id": purchasereturnPayment.PurchaseReturnID})
		if err != nil {
			return errs
		}

		purchaseReturn, err := FindPurchaseReturnByID(purchasereturnPayment.PurchaseReturnID, bson.M{})
		if err != nil {
			return errs
		}



		if scenario == "update" {
			if purchasereturnPayment.Amount != nil && ToFixed(((purchaseReturnPaymentStats.TotalPayment-*oldPurchaseReturnPayment.Amount)+*purchasereturnPayment.Amount), 2) > purchaseReturn.NetTotal {
				if ToFixed((purchaseReturn.NetTotal-(purchaseReturnPaymentStats.TotalPayment-*oldPurchaseReturnPayment.Amount)), 2) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (purchaseReturn.NetTotal-(purchaseReturnPaymentStats.TotalPayment-*oldPurchaseReturnPayment.Amount)))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR"
				}
			}
		} else {

			if purchasereturnPayment.Amount != nil && ToFixed((purchaseReturnPaymentStats.TotalPayment+*purchasereturnPayment.Amount), 2) > purchaseReturn.NetTotal {
				if ToFixed((purchaseReturn.NetTotal-purchaseReturnPaymentStats.TotalPayment), 2) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (purchaseReturn.NetTotal-purchaseReturnPaymentStats.TotalPayment))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", purchaseReturnPaymentStats.TotalPayment) + " SAR"
				}
			}
		}
	*/

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (purchasereturnPayment *PurchaseReturnPayment) Insert() error {
	collection := db.GetDB("store_" + purchasereturnPayment.StoreID.Hex()).Collection("purchase_return_payment")
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
	collection := db.GetDB("store_" + purchasereturnPayment.StoreID.Hex()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
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

func (store *Store) FindPurchaseReturnPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (purchasereturnPayment *PurchaseReturnPayment, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_return_payment")
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
		Decode(&purchasereturnPayment)
	if err != nil {
		return nil, err
	}

	return purchasereturnPayment, err
}

func (store *Store) IsPurchaseReturnPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

type PurchaseReturnPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func (store *Store) GetPurchaseReturnPaymentStats(filter map[string]interface{}) (stats PurchaseReturnPaymentStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_return_payment")
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

func (store *Store) ProcessPurchaseReturnPayments() error {
	log.Print("Processing purchase return payments")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	searchBy := make(map[string]interface{})
	searchBy["deleted"] = bson.M{"$ne": true}
	cur, err := collection.Find(ctx, searchBy, findOptions)
	if err != nil {
		return errors.New("Error fetching purchase return payments:" + err.Error())
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
		model := PurchaseReturnPayment{}
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

			//return err
		}
	}

	log.Print("DONE!")
	return nil
}
