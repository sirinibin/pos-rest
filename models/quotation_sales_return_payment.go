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
	"go.mongodb.org/mongo-driver/mongo/options"
)

// QuotationSalesReturnPayment : QuotationSalesReturnPayment structure
type QuotationSalesReturnPayment struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                  string              `json:"date_str,omitempty" bson:"-"`
	QuotationSalesReturnID   *primitive.ObjectID `json:"quotation_sales_return_id" bson:"quotation_sales_return_id"`
	QuotationSalesReturnCode string              `json:"quotation_sales_return_code" bson:"quotation_sales_return_code"`
	QuotationID              *primitive.ObjectID `json:"quotation_id" bson:"quotation_id"`
	QuotationCode            string              `json:"quotation_code" bson:"quotation_code"`
	Amount                   float64             `json:"amount" bson:"amount"`
	Method                   string              `json:"method" bson:"method"`
	CreatedAt                *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName            string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName            string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID                  *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName                string              `json:"store_name" bson:"store_name"`
	Deleted                  bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser            *User               `json:"deleted_by_user,omitempty"`
	DeletedAt                *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	PayableID                *primitive.ObjectID `json:"payable_id" bson:"payable_id"`
	PayablePaymentID         *primitive.ObjectID `json:"payable_payment_id" bson:"payable_payment_id"`
}

/*
func (quotationsalesreturnPayment *QuotationSalesReturnPayment) AttributesValueChangeEvent(quotationsalesreturnPaymentOld *QuotationSalesReturnPayment) error {

	if quotationsalesreturnPayment.Name != quotationsalesreturnPaymentOld.Name {
		err := UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": quotationsalesreturnPayment.ID},
			bson.M{"$pull": bson.M{
				"customer_name": quotationsalesreturnPaymentOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": quotationsalesreturnPayment.ID},
			bson.M{"$push": bson.M{
				"customer_name": quotationsalesreturnPayment.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}
*/

func (quotationsalesreturnPayment *QuotationSalesReturnPayment) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(quotationsalesreturnPayment.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if quotationsalesreturnPayment.StoreID != nil && !quotationsalesreturnPayment.StoreID.IsZero() {
		store, err := FindStoreByID(quotationsalesreturnPayment.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error finding store: " + err.Error())
		}
		quotationsalesreturnPayment.StoreName = store.Name
	} else {
		quotationsalesreturnPayment.StoreName = ""
	}

	if quotationsalesreturnPayment.QuotationSalesReturnID != nil && !quotationsalesreturnPayment.QuotationSalesReturnID.IsZero() {
		quotationsalesReturn, err := store.FindQuotationSalesReturnByID(quotationsalesreturnPayment.QuotationSalesReturnID, bson.M{"id": 1, "code": 1})
		if err != nil {
			//return err
			return errors.New("Error finding quotationsales return id: " + err.Error())
		}
		quotationsalesreturnPayment.QuotationSalesReturnCode = quotationsalesReturn.Code
	} else {
		quotationsalesreturnPayment.QuotationSalesReturnCode = ""
	}

	if quotationsalesreturnPayment.QuotationID != nil && !quotationsalesreturnPayment.QuotationID.IsZero() {
		quotation, err := store.FindQuotationByID(quotationsalesreturnPayment.QuotationID, bson.M{"id": 1, "code": 1})
		if err != nil {
			//return err
			return errors.New("Error finding quotation id: " + err.Error())
		}
		quotationsalesreturnPayment.QuotationCode = quotation.Code
	} else {
		quotationsalesreturnPayment.QuotationCode = ""
	}

	if quotationsalesreturnPayment.CreatedBy != nil {
		createdByUser, err := FindUserByID(quotationsalesreturnPayment.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			//return err
			return errors.New("Error finding user: " + err.Error())
		}
		quotationsalesreturnPayment.CreatedByName = createdByUser.Name
	}

	if quotationsalesreturnPayment.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(quotationsalesreturnPayment.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			//return err
			return errors.New("Error finding user: " + err.Error())
		}
		quotationsalesreturnPayment.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func (store *Store) SearchQuotationSalesReturnPayment(w http.ResponseWriter, r *http.Request) (models []QuotationSalesReturnPayment, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[quotation_sales_return_id]"]
	if ok && len(keys[0]) >= 1 {
		quotationsalesReturnID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["quotation_sales_return_id"] = quotationsalesReturnID
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["quotation_sales_return_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return_payment")

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
		return models, criterias, errors.New("Error fetching quotationsales return payment :" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotationsalesreturnPayment := QuotationSalesReturnPayment{}
		err = cur.Decode(&quotationsalesreturnPayment)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		models = append(models, quotationsalesreturnPayment)
	} //end for loop

	return models, criterias, nil
}

func (quotationsalesReturnPayment *QuotationSalesReturnPayment) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(quotationsalesReturnPayment.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	//var oldQuotationSalesReturnPayment *QuotationSalesReturnPayment

	if govalidator.IsNull(quotationsalesReturnPayment.Method) {
		errs["method"] = "Payment method is required"
	}

	if govalidator.IsNull(quotationsalesReturnPayment.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, quotationsalesReturnPayment.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		quotationsalesReturnPayment.Date = &date
	}

	var oldQuotationSalesReturnPayment *QuotationSalesReturnPayment

	if scenario == "update" {
		if quotationsalesReturnPayment.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsQuotationSalesReturnPaymentExists(&quotationsalesReturnPayment.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid quotationsales return payment :" + quotationsalesReturnPayment.ID.Hex()
		}

		oldQuotationSalesReturnPayment, err = store.FindQuotationSalesReturnPaymentByID(&quotationsalesReturnPayment.ID, bson.M{})
		if err != nil {
			errs["quotation_sales_return_payment"] = err.Error()
			return errs
		}

	}

	if quotationsalesReturnPayment.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if ToFixed(quotationsalesReturnPayment.Amount, 2) <= 0 {
		errs["amount"] = "Amount should be > 0"
	}

	quotationsalesReturn, err := store.FindQuotationSalesReturnByID(quotationsalesReturnPayment.QuotationSalesReturnID, bson.M{})
	if err != nil {
		errs["quotation_sales_return"] = "error finding quotationsales return" + err.Error()
	}

	if quotationsalesReturnPayment.Amount > (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount) {
		errs["amount"] = "Amount should not exceed: " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-quotationsalesReturn.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if scenario == "update" {
		if (quotationsalesReturnPayment.Amount + (quotationsalesReturn.TotalPaymentPaid - oldQuotationSalesReturnPayment.Amount)) > (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-quotationsalesReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	} else {
		if (quotationsalesReturnPayment.Amount + (quotationsalesReturn.TotalPaymentPaid)) > (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount) {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-quotationsalesReturn.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}
	}

	//validating with quotation received payment
	quotation, err := store.FindQuotationByID(quotationsalesReturnPayment.QuotationID, bson.M{})
	if err != nil {
		errs["quotationsales"] = "error finding sale" + err.Error()
	}

	if scenario == "update" {
		if (quotationsalesReturnPayment.Amount + (quotationsalesReturn.TotalPaymentPaid - oldQuotationSalesReturnPayment.Amount)) > quotation.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (quotation.TotalPaymentReceived)) + " (Total Received payment)"
			return
		}
	} else {
		if (quotationsalesReturnPayment.Amount + (quotationsalesReturn.TotalPaymentPaid)) > quotation.TotalPaymentReceived {
			errs["amount"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (quotation.TotalPaymentReceived)) + " (Total Received payment)"
			return
		}
	}

	/*
		quotationsalesReturnPaymentStats, err := GetQuotationSalesReturnPaymentStats(bson.M{"quotation_sales_return_id": quotationsalesReturnPayment.QuotationSalesReturnID})
		if err != nil {
			return errs
		}

		quotationsalesReturn, err := FindQuotationSalesReturnByID(quotationsalesReturnPayment.QuotationSalesReturnID, bson.M{})
		if err != nil {
			return errs
		}
	*/

	/*
		if scenario == "update" {
			if ((quotationsalesReturnPaymentStats.TotalPayment - *oldQuotationSalesReturnPayment.Amount) + *quotationsalesReturnPayment.Amount) > quotationsalesReturn.NetTotal {
				if (quotationsalesReturn.NetTotal - (quotationsalesReturnPaymentStats.TotalPayment - *oldQuotationSalesReturnPayment.Amount)) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-(quotationsalesReturnPaymentStats.TotalPayment-*oldQuotationSalesReturnPayment.Amount)))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		} else {

			if ToFixed((quotationsalesReturnPaymentStats.TotalPayment+*quotationsalesReturnPayment.Amount), 2) > quotationsalesReturn.NetTotal {
				if ToFixed((quotationsalesReturn.NetTotal-quotationsalesReturnPaymentStats.TotalPayment), 2) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-quotationsalesReturnPaymentStats.TotalPayment))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		}
	*/

	/*
		if scenario == "update" {
			if ((quotationsalesReturnPaymentStats.TotalPayment - oldQuotationSalesReturnPayment.Amount) + quotationsalesreturnPayment.Amount) > quotationsalesReturn.NetTotal {
				if (quotationsalesReturn.NetTotal - (quotationsalesReturnPaymentStats.TotalPayment - oldQuotationSalesReturnPayment.Amount)) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-(quotationsalesReturnPaymentStats.TotalPayment-oldQuotationSalesReturnPayment.Amount)))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		} else {
			if (quotationsalesReturnPaymentStats.TotalPayment + quotationsalesreturnPayment.Amount) > quotationsalesReturn.NetTotal {
				if (quotationsalesReturn.NetTotal - quotationsalesReturnPaymentStats.TotalPayment) > 0 {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR, So the amount should be less than or equal to  " + fmt.Sprintf("%.02f", (quotationsalesReturn.NetTotal-quotationsalesReturnPaymentStats.TotalPayment))
				} else {
					errs["amount"] = "You've already paid " + fmt.Sprintf("%.02f", quotationsalesReturnPaymentStats.TotalPayment) + " SAR"
				}

			}
		}
	*/

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (quotationsalesreturnPayment *QuotationSalesReturnPayment) Insert() error {
	collection := db.GetDB("store_" + quotationsalesreturnPayment.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := quotationsalesreturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	quotationsalesreturnPayment.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &quotationsalesreturnPayment)
	if err != nil {
		return err
	}
	return nil
}

func (quotationsalesreturnPayment *QuotationSalesReturnPayment) Update() error {
	collection := db.GetDB("store_" + quotationsalesreturnPayment.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := quotationsalesreturnPayment.UpdateForeignLabelFields()
	if err != nil {
		return errors.New("Error updating foreign label fields: " + err.Error())
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationsalesreturnPayment.ID},
		bson.M{"$set": quotationsalesreturnPayment},
		updateOptions,
	)
	if err != nil {
		return errors.New("Error updating quotationsales return payment: " + err.Error())
	}

	return nil

}

func (store *Store) FindQuotationSalesReturnPaymentByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotationsalesreturnPayment *QuotationSalesReturnPayment, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return_payment")
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
		Decode(&quotationsalesreturnPayment)
	if err != nil {
		return nil, err
	}

	return quotationsalesreturnPayment, err
}

func (store *Store) IsQuotationSalesReturnPaymentExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

type QuotationSalesReturnPaymentStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	TotalPayment float64             `json:"total_payment" bson:"total_payment"`
}

func (store *Store) GetQuotationSalesReturnPaymentStats(filter map[string]interface{}) (stats QuotationSalesReturnPaymentStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return_payment")
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

func (store *Store) ProcessQuotationSalesReturnPayments() error {
	log.Print("Processing quotationsales return payments")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	searchBy := make(map[string]interface{})
	searchBy["deleted"] = bson.M{"$ne": true}
	cur, err := collection.Find(ctx, searchBy, findOptions)
	if err != nil {
		return errors.New("Error fetching quotationsales return payments:" + err.Error())
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
		model := QuotationSalesReturnPayment{}
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
			log.Print("QuotationSales ID: " + model.QuotationCode)
			log.Print("QuotationSalesReturn ID: " + model.QuotationSalesReturnCode)
			log.Print(err)

			//return err
		}
	}

	log.Print("DONE!")
	return nil
}

func (quotationsalesReturnPayment *QuotationSalesReturnPayment) DeleteQuotationSalesReturnPayment() (err error) {
	collection := db.GetDB("store_" + quotationsalesReturnPayment.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationsalesReturnPayment.ID},
		bson.M{"$set": quotationsalesReturnPayment},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
