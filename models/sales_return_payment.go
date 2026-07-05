package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SalesReturnPayment : SalesReturnPayment structure
type SalesReturnPayment struct {
	ID               primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date             *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr          string              `json:"date_str,omitempty" bson:"-"`
	SalesReturnID    *primitive.ObjectID `json:"sales_return_id" bson:"sales_return_id"`
	SalesReturnCode  string              `json:"sales_return_code" bson:"sales_return_code"`
	OrderID          *primitive.ObjectID `json:"order_id" bson:"order_id"`
	OrderCode        string              `json:"order_code" bson:"order_code"`
	Amount           float64             `json:"amount" bson:"amount"`
	Method           string              `json:"method" bson:"method"`
	Description      *string             `json:"description,omitempty" bson:"description,omitempty"`
	ReferenceType    string              `json:"reference_type" bson:"reference_type"`
	ReferenceCode    string              `json:"reference_code" bson:"reference_code"`
	ReferenceID      *primitive.ObjectID `json:"reference_id" bson:"reference_id"`
	CreatedAt        *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt        *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy        *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy        *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName    string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName    string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID          *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName        string              `json:"store_name" bson:"store_name"`
	Deleted          bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy        *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser    *User               `json:"deleted_by_user,omitempty"`
	DeletedAt        *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	PayableID        *primitive.ObjectID `json:"payable_id" bson:"payable_id"`
	PayablePaymentID *primitive.ObjectID `json:"payable_payment_id" bson:"payable_payment_id"`
}

/*
func (salesreturnPayment *SalesReturnPayment) AttributesValueChangeEvent(salesreturnPaymentOld *SalesReturnPayment) error {

	if salesreturnPayment.Name != salesreturnPaymentOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesreturnPayment.ID},
			bson.M{"$pull": bson.M{
				"customer_name": salesreturnPaymentOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": salesreturnPayment.ID},
			bson.M{"$push": bson.M{
				"customer_name": salesreturnPayment.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (salesreturnPayment *SalesReturnPayment) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(salesreturnPayment.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if salesreturnPayment.StoreID != nil && !salesreturnPayment.StoreID.IsZero() {
		store, err := FindStoreByID(salesreturnPayment.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding store: " + err.Error())
		}
		salesreturnPayment.StoreName = store.Name
	} else {
		salesreturnPayment.StoreName = ""
	}

	if salesreturnPayment.SalesReturnID != nil && !salesreturnPayment.SalesReturnID.IsZero() {
		salesReturn, err := store.FindSalesReturnByID(salesreturnPayment.SalesReturnID, bson.M{"id": 1, "code": 1})
		if err != nil {
			//return err
			return errors.New("Error finding sales return id: " + err.Error())
		}
		salesreturnPayment.SalesReturnCode = salesReturn.Code
	} else {
		salesreturnPayment.SalesReturnCode = ""
	}

	if salesreturnPayment.OrderID != nil && !salesreturnPayment.OrderID.IsZero() {
		order, err := store.FindOrderByID(salesreturnPayment.OrderID, bson.M{"id": 1, "code": 1})
		if err != nil {
			//return err
			return errors.New("Error finding order id: " + err.Error())
		}
		salesreturnPayment.OrderCode = order.Code
	} else {
		salesreturnPayment.OrderCode = ""
	}

	if salesreturnPayment.CreatedBy != nil {
		createdByUser, err := FindUserByID(salesreturnPayment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			//return err
			return errors.New("Error finding user: " + err.Error())
		}
		salesreturnPayment.CreatedByName = createdByUser.Name
	}

	if salesreturnPayment.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(salesreturnPayment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			//return err
			return errors.New("Error finding user: " + err.Error())
		}
		salesreturnPayment.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func (store *Store) SearchSalesReturnPayment(w http.ResponseWriter, r *http.Request) (models []SalesReturnPayment, criterias SearchCriterias, err error) {

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

	if err = ParseFloatWithOperatorFilter(r, &criterias, "search[amount]", "amount"); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[updated_by_name]", "updated_by_name")

	ParseTextSearch(r, &criterias, "search[store_name]", "store_name")

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return models, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[sales_return_id]", "sales_return_id"); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[sales_return_code]", "sales_return_code")

	if err = ParseObjectIDFilter(r, &criterias, "search[order_id]", "order_id"); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[order_code]", "order_code")

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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_return_payment")

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
		return models, criterias, errors.New("Error fetching sales return payment :" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		salesreturnPayment := SalesReturnPayment{}
		err = cur.Decode(&salesreturnPayment)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, salesreturnPayment)
	} //end for loop

	return models, criterias, nil
}

func (salesReturnPayment *SalesReturnPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(salesReturnPayment.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	//var oldSalesReturnPayment *SalesReturnPayment

	if govalidator.IsNull(salesReturnPayment.Method) {
		errs["method"] = "Payment method is required"
	}

	if govalidator.IsNull(salesReturnPayment.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, salesReturnPayment.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		salesReturnPayment.Date = &date
	}

	var oldSalesReturnPayment *SalesReturnPayment

	if scenario == "update" {
		if salesReturnPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsSalesReturnPaymentExists(&salesReturnPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid sales return payment :" + salesReturnPayment.ID.Hex()
		}

		oldSalesReturnPayment, err = store.FindSalesReturnPaymentByID(&salesReturnPayment.ID, bson.M{})
		if err != nil {
			errs["sales_return_payment"] = err.Error()
			return errs
		}

	}

	if salesReturnPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if ToFixed(salesReturnPayment.Amount, 2) < 0 {
		errs["amount"] = "Amount should be > 0"
	}

	salesReturn, err := store.FindSalesReturnByID(salesReturnPayment.SalesReturnID, bson.M{})
	if err != nil {
		errs["sales_return"] = "error finding sales return" + err.Error()
	}

	if salesReturnPayment.Amount > RoundTo2Decimals(salesReturn.NetTotal-salesReturn.CashDiscount) {
		errs["amount"] = "Amount should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(salesReturn.NetTotal-salesReturn.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if scenario == "update" {
		if (salesReturnPayment.Amount + (salesReturn.TotalPaymentPaid - oldSalesReturnPayment.Amount)) > RoundTo2Decimals(salesReturn.NetTotal-salesReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(salesReturn.NetTotal-salesReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	} else {
		if (salesReturnPayment.Amount + (salesReturn.TotalPaymentPaid)) > RoundTo2Decimals(salesReturn.NetTotal-salesReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(salesReturn.NetTotal-salesReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	}

	//validating with order received payment
	order, err := store.FindOrderByID(salesReturnPayment.OrderID, bson.M{})
	if err != nil {
		errs["sales"] = "error finding sale" + err.Error()
	}

	if scenario == "update" {
		if (salesReturnPayment.Amount + (salesReturn.TotalPaymentPaid - oldSalesReturnPayment.Amount)) > order.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (order.TotalPaymentReceived)) + " (Total Received payment)"
			return
		}
	} else {
		if (salesReturnPayment.Amount + (salesReturn.TotalPaymentPaid)) > order.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (order.TotalPaymentReceived)) + " (Total Received payment)"
			return
		}
	}

	/*
		salesReturnPaymentStats, err := GetSalesReturnPaymentStats(bson.M{"sales_return_id": salesReturnPayment.SalesReturnID})
		if err != nil {
			return errs
		}

		salesReturn, err := FindSalesReturnByID(salesReturnPayment.SalesReturnID, bson.M{})
		if err != nil {
			return errs
		}
	*/

	/*
		if scenario == "update" {
			if ((salesReturnPaymentStats.TotalPayment - *oldSalesReturnPayment.Amount) + *salesReturnPayment.Amount) > salesReturn.NetTotal {
				if (salesReturn.NetTotal - (salesReturnPaymentStats.TotalPayment - *oldSalesReturnPayment.Amount)) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (salesReturn.NetTotal-(salesReturnPaymentStats.TotalPayment-*oldSalesReturnPayment.Amount)))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		} else {

			if ToFixed((salesReturnPaymentStats.TotalPayment+*salesReturnPayment.Amount), 2) > salesReturn.NetTotal {
				if ToFixed((salesReturn.NetTotal-salesReturnPaymentStats.TotalPayment), 2) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (salesReturn.NetTotal-salesReturnPaymentStats.TotalPayment))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		}
	*/

	/*
		if scenario == "update" {
			if ((salesReturnPaymentStats.TotalPayment - oldSalesReturnPayment.Amount) + salesreturnPayment.Amount) > salesReturn.NetTotal {
				if (salesReturn.NetTotal - (salesReturnPaymentStats.TotalPayment - oldSalesReturnPayment.Amount)) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (salesReturn.NetTotal-(salesReturnPaymentStats.TotalPayment-oldSalesReturnPayment.Amount)))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		} else {
			if (salesReturnPaymentStats.TotalPayment + salesreturnPayment.Amount) > salesReturn.NetTotal {
				if (salesReturn.NetTotal - salesReturnPaymentStats.TotalPayment) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (salesReturn.NetTotal-salesReturnPaymentStats.TotalPayment))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", salesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		}
	*/

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (salesreturnPayment *SalesReturnPayment) Insert() error {
	collection := db.GetDB("store_" + salesreturnPayment.StoreID.Hex()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := salesreturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	salesreturnPayment.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &salesreturnPayment)
	if err != nil {
		return err
	}
	return nil
}

func (salesreturnPayment *SalesReturnPayment) Update() error {
	collection := db.GetDB("store_" + salesreturnPayment.StoreID.Hex()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := salesreturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return errors.New("Error updating foreign label fields: " + err.Error())
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesreturnPayment.ID},
		bson.M{"$set": salesreturnPayment},
		updateOptions,
	)
	if err != nil {
		return errors.New("Error updating sales return payment: " + err.Error())
	}

	return nil

}

func (store *Store) FindSalesReturnPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesreturnPayment *SalesReturnPayment, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_return_payment")
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
		Decode(&salesreturnPayment)
	if err != nil {
		return nil, err
	}

	return salesreturnPayment, err
}

func (store *Store) IsSalesReturnPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

type SalesReturnPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func (store *Store) GetSalesReturnPaymentStats(filter map[string]interface{}) (stats SalesReturnPaymentStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_return_payment")
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

func (store *Store) ProcessSalesReturnPayments() error {
	log.Print("Processing sales return payments")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	searchBy := make(map[string]interface{})
	searchBy["deleted"] = bson.M{"$ne": true}
	cur, err := collection.Find(ctx, searchBy, findOptions)
	if err != nil {
		return errors.New("Error fetching sales return payments:" + err.Error())
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
		model := SalesReturnPayment{}
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
			log.Print("Sales ID: " + model.OrderCode)
			log.Print("SalesReturn ID: " + model.SalesReturnCode)
			log.Print(err)

			//return err
		}
	}

	log.Print("DONE!")
	return nil
}

func (salesReturnPayment *SalesReturnPayment) DeleteSalesReturnPayment() (err error) {
	collection := db.GetDB("store_" + salesReturnPayment.StoreID.Hex()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesReturnPayment.ID},
		bson.M{"$set": salesReturnPayment},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
