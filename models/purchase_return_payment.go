package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
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
	Amount              float64             `json:"amount" bson:"amount"`
	Method              string              `json:"method" bson:"method"`
	Description         *string             `json:"description,omitempty" bson:"description,omitempty"`
	ReferenceType       string              `json:"reference_type" bson:"reference_type"`
	ReferenceCode       string              `json:"reference_code" bson:"reference_code"`
	ReferenceID         *primitive.ObjectID `json:"reference_id" bson:"reference_id"`
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

	criterias = InitSearchCriterias()
	ParseDeletedFilter(r, &criterias)

	var keys []string
	var ok bool

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)

	if err = ParseExactDateFilter(r, &criterias, "search[date_str]", "date", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[from_date]", "search[to_date]", "date", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[created_by_name]", "created_by_name")

	ParseTextSearch(r, &criterias, "search[method]", "method")

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

	ParseTextSearch(r, &criterias, "search[updated_by_name]", "updated_by_name")

	ParseTextSearch(r, &criterias, "search[store_name]", "store_name")

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return models, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[purchase_return_id]", "purchase_return_id"); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[purchase_return_code]", "purchase_return_code")

	if err = ParseObjectIDFilter(r, &criterias, "search[purchase_id]", "purchase_id"); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[purchase_code]", "purchase_code")

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

	if purchasereturnPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	} else if ToFixed(purchasereturnPayment.Amount, 2) < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	purchaseReturn, err := store.FindPurchaseReturnByID(purchasereturnPayment.PurchaseReturnID, bson.M{})
	if err != nil {
		errs["sales_return"] = "error finding sales return" + err.Error()
	}

	if purchasereturnPayment.Amount > RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount) {
		errs["amount"] = "Amount should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if scenario == "update" {
		if (purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid - oldPurchaseReturnPayment.Amount)) > RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	} else {
		if (purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid)) > RoundTo2Decimals(purchaseReturn.NetTotal-purchaseReturn.CashDiscount) {
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
		if (purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid - oldPurchaseReturnPayment.Amount)) > purchase.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (purchase.TotalPaymentReceived)) + " (Total Paid payment)"
			return
		}
	} else {
		if (purchasereturnPayment.Amount + (purchaseReturn.TotalPaymentPaid)) > purchase.TotalPaymentReceived {
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
