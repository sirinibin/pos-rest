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
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
	"gopkg.in/mgo.v2/bson"
)

// CustomerWithdrawal : CustomerWithdrawal structure
type CustomerWithdrawal struct {
	ID              primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code            string              `bson:"code" json:"code"`
	Amount          float64             `bson:"amount" json:"amount"`
	Description     string              `bson:"description" json:"description"`
	Remarks         string              `bson:"remarks" json:"remarks"`
	BankReferenceNo string              `bson:"bank_reference_no" json:"bank_reference_no"`
	Date            *time.Time          `bson:"date" json:"date"`
	DateStr         string              `json:"date_str,omitempty" bson:"-"`
	CustomerID      *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Customer        *Customer           `json:"customer" bson:"-"`
	CustomerName    string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	PaymentMethod   string              `json:"payment_method" bson:"payment_method"`
	StoreID         *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName       string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode       string              `json:"store_code,omitempty" bson:"store_code,omitempty"`
	Images          []string            `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent   []string            `json:"images_content,omitempty"`
	CreatedAt       *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy       *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser   *User               `json:"created_by_user,omitempty"`
	UpdatedByUser   *User               `json:"updated_by_user,omitempty"`
	CategoryName    []string            `json:"category_name" bson:"category_name"`
	CreatedByName   string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName   string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName   string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted         bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy       *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser   *User               `json:"deleted_by_user,omitempty"`
	DeletedAt       *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	Payments        []PayablePayment    `bson:"payments" json:"payments"`
	NetTotal        float64             `bson:"net_total" json:"net_total"`
	PaymentMethods  []string            `json:"payment_methods" bson:"payment_methods"`
}

type PayablePayment struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
	Amount        *float64            `json:"amount" bson:"amount"`
	Method        string              `json:"method" bson:"method"`
	BankReference *string             `json:"bank_reference" bson:"bank_reference"`
	Description   *string             `json:"description" bson:"description"`
	InvoiceID     *primitive.ObjectID `json:"invoice_id" bson:"invoice_id"`
	InvoiceCode   *string             `json:"invoice_code" bson:"invoice_code"`
	InvoiceType   *string             `json:"invoice_type" bson:"invoice_type"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName     string              `json:"store_name" bson:"store_name"`
}

func (customerwithdrawal *CustomerWithdrawal) HandleDeletedPayments(customerwithdrawalOld *CustomerWithdrawal) error {
	store, _ := FindStoreByID(customerwithdrawal.StoreID, bson.M{})

	for _, oldPayment := range customerwithdrawalOld.Payments {
		found := false
		deleteSalesPayment := false
		for _, payment := range customerwithdrawal.Payments {
			if payment.ID.Hex() == oldPayment.ID.Hex() {
				found = true
				if (payment.InvoiceID == nil || payment.InvoiceID.IsZero()) && (oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero()) {
					deleteSalesPayment = true
				}
				break
			}
		} //end for2

		if !found || deleteSalesPayment {
			if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() {
				salesreturn, err := store.FindSalesReturnByID(oldPayment.InvoiceID, bson.M{})
				if err != nil {
					return err
				}
				err = salesreturn.DeletePaymentsByPayablePaymentID(oldPayment.ID)
				if err != nil {
					return err
				}

				_, err = salesreturn.SetPaymentStatus()
				if err != nil {
					return err
				}

				err = salesreturn.Update()
				if err != nil {
					return err
				}

				err = salesreturn.UndoAccounting()
				if err != nil {
					return err
				}

				err = salesreturn.DoAccounting()
				if err != nil {
					return err
				}

				err = salesreturn.SetCustomerSalesReturnStats()
				if err != nil {
					return err
				}

			}
		}
	} //end for1
	return nil
}

func (customerwithdrawal *CustomerWithdrawal) CloseSalesPayments() error {
	store, _ := FindStoreByID(customerwithdrawal.StoreID, bson.M{})

	for _, payablePayment := range customerwithdrawal.Payments {
		if payablePayment.InvoiceID != nil && !payablePayment.InvoiceID.IsZero() {
			salesreturn, _ := store.FindSalesReturnByID(payablePayment.InvoiceID, bson.M{})
			err := salesreturn.UpdatePaymentFromPayablePayment(payablePayment, customerwithdrawal)
			if err != nil {
				return errors.New("error updating sales payment from payable payment: " + err.Error())
			}

			_, err = salesreturn.SetPaymentStatus()
			if err != nil {
				return errors.New("error setting payment status: " + err.Error())
			}

			err = salesreturn.Update()
			if err != nil {
				return errors.New("error updating salesreturn inside payment status: " + err.Error())
			}

			err = salesreturn.SetCustomerSalesReturnStats()
			if err != nil {
				return err
			}

			err = salesreturn.UndoAccounting()
			if err != nil {
				return err
			}

			err = salesreturn.DoAccounting()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (customerwithdrawal *CustomerWithdrawal) FindNetTotal() {
	netTotal := float64(0.00)
	paymentMethods := []string{}

	for _, payment := range customerwithdrawal.Payments {
		netTotal += *payment.Amount

		if !slices.Contains(paymentMethods, payment.Method) {
			paymentMethods = append(paymentMethods, payment.Method)
		}
	}
	customerwithdrawal.NetTotal = netTotal
	customerwithdrawal.PaymentMethods = paymentMethods
}

func (customerwithdrawal *CustomerWithdrawal) AttributesValueChangeEvent(customerwithdrawalOld *CustomerWithdrawal) error {

	return nil
}

func (customerwithdrawal *CustomerWithdrawal) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(customerwithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customerwithdrawal.CategoryName = []string{}

	if customerwithdrawal.CustomerID != nil && !customerwithdrawal.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(customerwithdrawal.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customerwithdrawal.CustomerName = customer.Name
	}

	if customerwithdrawal.CreatedBy != nil {
		createdByUser, err := FindUserByID(customerwithdrawal.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		customerwithdrawal.CreatedByName = createdByUser.Name
	}

	if customerwithdrawal.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(customerwithdrawal.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		customerwithdrawal.UpdatedByName = updatedByUser.Name
	}

	if customerwithdrawal.DeletedBy != nil && !customerwithdrawal.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(customerwithdrawal.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		customerwithdrawal.DeletedByName = deletedByUser.Name
	}

	if customerwithdrawal.StoreID != nil {
		store, err := FindStoreByID(customerwithdrawal.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		customerwithdrawal.StoreName = store.Name
		customerwithdrawal.StoreCode = store.Code
	}

	return nil
}

type CustomerWithdrawalStats struct {
	ID    *primitive.ObjectID `json:"id" bson:"_id"`
	Total float64             `json:"total" bson:"total"`
}

func (store *Store) GetCustomerWithdrawalStats(filter map[string]interface{}) (stats CustomerWithdrawalStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":   nil,
				"total": bson.M{"$sum": "$net_total"},
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
		stats.Total = RoundFloat(stats.Total, 2)
	}
	return stats, nil
}

func (store *Store) SearchCustomerWithdrawal(w http.ResponseWriter, r *http.Request) (customerwithdrawals []CustomerWithdrawal, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customerwithdrawals, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["amount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["amount"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customerwithdrawals, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[payment_methods]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethods := strings.Split(keys[0], ",")
		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_methods"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[payment_method]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["payment_method"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[description]"]
	if ok && len(keys[0]) >= 1 {
		searchWord := strings.Replace(keys[0], "\\", `\\`, -1)
		searchWord = strings.Replace(searchWord, "(", `\(`, -1)
		searchWord = strings.Replace(searchWord, ")", `\)`, -1)
		searchWord = strings.Replace(searchWord, "{", `\{`, -1)
		searchWord = strings.Replace(searchWord, "}", `\}`, -1)
		searchWord = strings.Replace(searchWord, "[", `\[`, -1)
		searchWord = strings.Replace(searchWord, "]", `\]`, -1)
		searchWord = strings.Replace(searchWord, `*`, `\*`, -1)

		searchWord = strings.Replace(searchWord, "_", `\_`, -1)
		searchWord = strings.Replace(searchWord, "+", `\\+`, -1)
		searchWord = strings.Replace(searchWord, "'", `\'`, -1)
		searchWord = strings.Replace(searchWord, `"`, `\"`, -1)

		criterias.SearchBy["$or"] = []bson.M{
			{"description": bson.M{"$regex": searchWord, "$options": "i"}},
		}
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customerwithdrawals, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customerwithdrawals, criterias, err
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
			return customerwithdrawals, criterias, err
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
			return customerwithdrawals, criterias, err
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
			return customerwithdrawals, criterias, err
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
			return customerwithdrawals, criterias, err
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
			return customerwithdrawals, criterias, err
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
			return customerwithdrawals, criterias, err
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

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return customerwithdrawals, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
		}

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
		return customerwithdrawals, criterias, errors.New("Error fetching customerwithdrawals:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return customerwithdrawals, criterias, errors.New("Cursor error:" + err.Error())
		}
		customerwithdrawal := CustomerWithdrawal{}
		err = cur.Decode(&customerwithdrawal)
		if err != nil {
			return customerwithdrawals, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			customerwithdrawal.CreatedByUser, _ = FindUserByID(customerwithdrawal.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			customerwithdrawal.UpdatedByUser, _ = FindUserByID(customerwithdrawal.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			customerwithdrawal.DeletedByUser, _ = FindUserByID(customerwithdrawal.DeletedBy, deletedByUserSelectFields)
		}

		customerwithdrawals = append(customerwithdrawals, customerwithdrawal)
	} //end for loop

	return customerwithdrawals, criterias, nil

}

func (customerWithdrawal *CustomerWithdrawal) Validate(w http.ResponseWriter, r *http.Request, scenario string, customerWithdrawalOld *CustomerWithdrawal) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(customerWithdrawal.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if customerWithdrawal.CustomerID == nil || customerWithdrawal.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	}

	if govalidator.IsNull(customerWithdrawal.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, customerWithdrawal.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		customerWithdrawal.Date = &date
	}

	for index, payment := range customerWithdrawal.Payments {
		if payment.ID.IsZero() {
			customerWithdrawal.Payments[index].ID = primitive.NewObjectID()
		}

		if govalidator.IsNull(payment.DateStr) {
			errs["customer_payable_payment_date_"+strconv.Itoa(index)] = "Payment date is required"
		} else {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			date, err := time.Parse(shortForm, payment.DateStr)
			if err != nil {
				errs["customer_payable_payment_date_"+strconv.Itoa(index)] = "Invalid date format"
			}

			customerWithdrawal.Payments[index].Date = &date
			payment.Date = &date

			if customerWithdrawal.Date != nil && IsAfter(customerWithdrawal.Date, customerWithdrawal.Payments[index].Date) {
				errs["customer_payable_payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to Payable date time"
			}
		}

		if payment.Amount == nil {
			errs["customer_payable_payment_amount_"+strconv.Itoa(index)] = "Payment amount is required"
		} else if *payment.Amount <= 0 {
			errs["customer_payable_payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
		}

		if payment.Method == "" {
			errs["customer_payable_payment_method_"+strconv.Itoa(index)] = "Payment method is required"
		}

		if payment.InvoiceID != nil && !payment.InvoiceID.IsZero() {
			salesreturn, err := store.FindSalesReturnByID(payment.InvoiceID, bson.M{})
			if err != nil {
				errs["customer_payable_payment_invoice_"+strconv.Itoa(index)] = "invalid invoice: " + err.Error()
			}

			if salesreturn.CustomerID != nil &&
				!salesreturn.CustomerID.IsZero() &&
				customerWithdrawal.CustomerID != nil &&
				!customerWithdrawal.CustomerID.IsZero() &&
				salesreturn.CustomerID.Hex() != customerWithdrawal.CustomerID.Hex() {
				errs["customer_payable_payment_invoice_"+strconv.Itoa(index)] = "Invoice is not belongs to the selected customer"
			}

			salesreturnBalanceAmount := salesreturn.BalanceAmount

			if scenario == "update" {
				oldTotalInvoicePaidAmount := float64(0.00)
				for _, oldPayment := range customerWithdrawalOld.Payments {
					if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && oldPayment.InvoiceID.Hex() == salesreturn.ID.Hex() {
						oldTotalInvoicePaidAmount += *oldPayment.Amount
					}
				}
				salesreturnBalanceAmount += oldTotalInvoicePaidAmount
			}

			if *payment.Amount > salesreturnBalanceAmount {
				errs["customer_payable_payment_amount_"+strconv.Itoa(index)] = "Payment amount should not be greater than " + fmt.Sprintf("%.02f", salesreturn.BalanceAmount) + " (Invoice Balance)"
			}

			totalInvoicePaidAmount := float64(0.00)
			for index2, payment2 := range customerWithdrawal.Payments {
				if payment2.InvoiceID != nil && !payment2.InvoiceID.IsZero() && payment2.InvoiceID.Hex() == salesreturn.ID.Hex() {
					if (salesreturnBalanceAmount - totalInvoicePaidAmount) == 0 {
						errs["customer_payable_payment_amount_"+strconv.Itoa(index2)] = "Payment is already closed for this invoice"
						break
					} else if (totalInvoicePaidAmount + *payment2.Amount) > salesreturnBalanceAmount {
						errs["customer_payable_payment_amount_"+strconv.Itoa(index2)] = "Payment amount should not be greater than " + fmt.Sprintf("%.02f", (salesreturnBalanceAmount-totalInvoicePaidAmount)) + " (Invoice Balance)"
						break
					}
					totalInvoicePaidAmount += *payment2.Amount
				}
			}

		}

	} //end for

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (model *CustomerWithdrawal) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if model.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code":     model.Code,
			"store_id": model.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code":     model.Code,
			"store_id": model.StoreID,
			"_id":      bson.M{"$ne": model.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) FindLastCustomerWithdrawal(
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&customerwithdrawal)
	if err != nil {
		return nil, err
	}

	return customerwithdrawal, err
}

func (store *Store) FindLastCustomerWithdrawalByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&customerwithdrawal)
	if err != nil {
		return nil, err
	}

	return customerwithdrawal, err
}

func (store *Store) GetCustomerWithdrawalCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (customerWithdrawal *CustomerWithdrawal) MakeRedisCode() error {
	store, err := FindStoreByID(customerWithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := customerWithdrawal.StoreID.Hex() + "_customer_withdrawal_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from order.CreatedAt or fallback to order.Date or now ===
	baseTime := customerWithdrawal.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("customerwithdrawal")
		if err != nil {
			return err
		}
		startFrom := store.CustomerWithdrawalSerialNumber.StartFromCount
		err = db.RedisClient.Set(redisKey, startFrom+count-1, 0).Err()
		if err != nil {
			return err
		}
	}

	// === 4. Increment global counter ===
	globalIncr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	// === 5. Determine which counter to use for order.Code ===
	useMonthly := strings.Contains(store.CustomerWithdrawalSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := customerWithdrawal.StoreID.Hex() + "_customer_withdrawal_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.CustomerWithdrawalSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "customerwithdrawal")
			if err != nil {
				return err
			}

			if monthlyCount == 0 {
				err = db.RedisClient.Set(monthlyRedisKey, startFrom+monthlyCount-1, 0).Err()
				if err != nil {
					return err
				}
			} else {
				err = db.RedisClient.Set(monthlyRedisKey, (globalIncr - 1), 0).Err()
				if err != nil {
					return err
				}
			}
		}

		// Increment monthly counter and use it
		monthlyIncr, err := db.RedisClient.Incr(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if store.EnableMonthlySerialNumber {
			serialNumber = monthlyIncr
		}
	}

	// === 6. Build the code ===
	paddingCount := store.CustomerWithdrawalSerialNumber.PaddingCount
	if store.CustomerWithdrawalSerialNumber.Prefix != "" {
		customerWithdrawal.Code = fmt.Sprintf("%s-%0*d", store.CustomerWithdrawalSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		customerWithdrawal.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(customerWithdrawal.Code, "DATE") {
		orderDate := baseTime.Format("20060102") // YYYYMMDD
		customerWithdrawal.Code = strings.ReplaceAll(customerWithdrawal.Code, "DATE", orderDate)
	}

	return nil
}

func (customerWithdrawal *CustomerWithdrawal) UnMakeRedisCode() error {
	store, err := FindStoreByID(customerWithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := customerWithdrawal.StoreID.Hex() + "_customer_withdrawal_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := customerWithdrawal.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.CustomerWithdrawalSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := customerWithdrawal.StoreID.Hex() + "_customer_withdrawal_counter_" + monthKey

		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
func (model *CustomerWithdrawal) MakeCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_customer_withdrawal_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetCustomerWithdrawalCount()
		if err != nil {
			return err
		}

		startFrom := store.CustomerWithdrawalSerialNumber.StartFromCount

		startFrom += count
		// Set the initial counter value (startFrom - 1) so that the first increment gives startFrom
		err = db.RedisClient.Set(redisKey, startFrom-1, 0).Err()
		if err != nil {
			return err
		}
	}

	incr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	paddingCount := store.CustomerWithdrawalSerialNumber.PaddingCount

	if store.CustomerWithdrawalSerialNumber.Prefix != "" {
		model.Code = fmt.Sprintf("%s-%0*d", store.CustomerWithdrawalSerialNumber.Prefix, paddingCount, incr)
	} else {
		model.Code = fmt.Sprintf("%s%0*d", store.CustomerWithdrawalSerialNumber.Prefix, paddingCount, incr)
	}

	if store.CountryCode != "" {
		timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]
		if ok {
			location, err := time.LoadLocation(timeZone)
			if err != nil {
				return errors.New("error loading location")
			}
			if model.Date != nil {
				currentDate := model.Date.In(location).Format("20060102") // YYYYMMDD
				model.Code = strings.ReplaceAll(model.Code, "DATE", currentDate)
			}
		}
	}

	return nil
}
*/

/*
func (customerwithdrawal *CustomerWithdrawal) MakeCode() error {
	store, err := FindStoreByID(customerwithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	lastCustomerWithdrawal, err := store.FindLastCustomerWithdrawalByStoreID(customerwithdrawal.StoreID, bson.M{})
	if err != nil && mongo.ErrNoDocuments != err {
		return err
	}

	if lastCustomerWithdrawal == nil {
		store, err := FindStoreByID(customerwithdrawal.StoreID, bson.M{})
		if err != nil {
			return err
		}
		customerwithdrawal.Code = store.Code + "-100000"
	} else {
		splits := strings.Split(lastCustomerWithdrawal.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			customerwithdrawal.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := customerwithdrawal.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastCustomerWithdrawal.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		customerwithdrawal.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}*/

func (customerwithdrawal *CustomerWithdrawal) Insert() (err error) {
	collection := db.GetDB("store_" + customerwithdrawal.StoreID.Hex()).Collection("customerwithdrawal")
	customerwithdrawal.ID = primitive.NewObjectID()

	err = customerwithdrawal.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &customerwithdrawal)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func (customerwithdrawal *CustomerWithdrawal) Update() error {
	collection := db.GetDB("store_" + customerwithdrawal.StoreID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := customerwithdrawal.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customerwithdrawal.ID},
		bson.M{"$set": customerwithdrawal},
		updateOptions,
	)
	return err
}

func (customerwithdrawal *CustomerWithdrawal) DeleteCustomerWithdrawal(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + customerwithdrawal.StoreID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = customerwithdrawal.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	customerwithdrawal.Deleted = true
	customerwithdrawal.DeletedBy = &userID
	now := time.Now()
	customerwithdrawal.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customerwithdrawal.ID},
		bson.M{"$set": customerwithdrawal},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindCustomerWithdrawalByCode(
	code string,
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"code":     code,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&customerwithdrawal)
	if err != nil {
		return nil, err
	}

	return customerwithdrawal, err
}

func (store *Store) FindCustomerWithdrawalByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
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
		Decode(&customerwithdrawal)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		customerwithdrawal.CreatedByUser, _ = FindUserByID(customerwithdrawal.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		customerwithdrawal.UpdatedByUser, _ = FindUserByID(customerwithdrawal.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		customerwithdrawal.DeletedByUser, _ = FindUserByID(customerwithdrawal.DeletedBy, fields)
	}

	return customerwithdrawal, err
}

/*
func (customerwithdrawal *CustomerWithdrawal) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + customerwithdrawal.StoreID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customerwithdrawal.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": customerwithdrawal.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": customerwithdrawal.Code,
			"_id":  bson.M{"$ne": customerwithdrawal.ID},
		})
	}

	return (count > 0), err
}*/

func (store *Store) IsCustomerWithdrawalExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func ProcessCustomerWithdrawals() error {
	log.Printf("Processing customer withdrawals")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}
	for _, store := range stores {
		log.Print("Store: " + store.Name)
		totalCount, err := store.GetTotalCount(bson.M{"store_id": store.ID}, "customerwithdrawal")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerwithdrawal")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)

		bar := progressbar.Default(totalCount)
		cur, err := collection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			return errors.New("Error fetching customerwithdrawals" + err.Error())
		}
		if cur != nil {
			defer cur.Close(ctx)
		}

		for i := 0; cur != nil && cur.Next(ctx); i++ {
			err := cur.Err()
			if err != nil {
				return errors.New("Cursor error:" + err.Error())
			}
			model := CustomerWithdrawal{}
			err = cur.Decode(&model)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			model.UndoAccounting()
			model.DoAccounting()
			if model.CustomerID != nil && !model.CustomerID.IsZero() {
				customer, _ := store.FindCustomerByID(model.CustomerID, bson.M{})
				if customer != nil {
					customer.SetCreditBalance()
				}
			}

			/*

				if len(model.Payments) == 0 {
					model.Payments = []PayablePayment{
						PayablePayment{
							Amount:        &model.Amount,
							Date:          model.Date,
							Method:        model.PaymentMethod,
							BankReference: &model.BankReferenceNo,
							Description:   &model.Description,
						},
					}
				}

				model.FindNetTotal()

				err = model.Update()
				if err != nil {
					log.Print("Error updating: " + model.Code + ", err: " + err.Error())
					//return err
				}
			*/
			bar.Add(1)
		}
	}
	log.Print("DONE!")
	return nil
}

func (customerWithdrawal *CustomerWithdrawal) DoAccounting() error {
	ledgers, err := customerWithdrawal.CreateLedger()
	if err != nil {
		return err
	}

	for _, ledger := range ledgers {
		_, err = ledger.CreatePostings()
		if err != nil {
			return err
		}
	}

	return nil
}

func (customerWithdrawal *CustomerWithdrawal) UndoAccounting() error {
	store, err := FindStoreByID(customerWithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledgers, err := store.FindLedgersByReferenceID(customerWithdrawal.ID, *customerWithdrawal.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	var relatedLedgerAccounts map[string]Account
	for _, ledger := range ledgers {
		ledgerAccounts, err := ledger.GetRelatedAccounts()
		if err != nil {
			return err
		}
		relatedLedgerAccounts = MergeAccountMaps(relatedLedgerAccounts, ledgerAccounts)
	}

	err = store.RemoveLedgerByReferenceID(customerWithdrawal.ID)
	if err != nil {
		return err
	}

	err = store.RemovePostingsByReferenceID(customerWithdrawal.ID)
	if err != nil {
		return err
	}

	err = SetAccountBalances(relatedLedgerAccounts)
	if err != nil {
		return err
	}

	return nil
}

func (customerWithdrawal *CustomerWithdrawal) CreateLedger() (ledgers []Ledger, err error) {
	store, err := FindStoreByID(customerWithdrawal.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()

	customer, err := store.FindCustomerByID(customerWithdrawal.CustomerID, bson.M{})
	if err != nil {
		return nil, err
	}

	referenceModel := "customer"
	customerAccount, err := store.CreateAccountIfNotExists(
		customerWithdrawal.StoreID,
		&customer.ID,
		&referenceModel,
		customer.Name,
		&customer.Phone,
		&customer.VATNo,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(customerWithdrawal.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(customerWithdrawal.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	for _, payment := range customerWithdrawal.Payments {
		journals := []Journal{}

		spendingAccount := Account{}
		if payment.Method == "cash" {
			spendingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			spendingAccount = *bankAccount
		}

		groupID := primitive.NewObjectID()

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     customerAccount.ID,
			AccountNumber: customerAccount.Number,
			AccountName:   customerAccount.Name,
			DebitOrCredit: "debit",
			Debit:         *payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     spendingAccount.ID,
			AccountNumber: spendingAccount.Number,
			AccountName:   spendingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        *payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

		ledger := &Ledger{
			StoreID:        customerWithdrawal.StoreID,
			ReferenceID:    customerWithdrawal.ID,
			ReferenceModel: "customer_withdrawal",
			ReferenceCode:  customerWithdrawal.Code,
			Journals:       journals,
			CreatedAt:      &now,
			UpdatedAt:      &now,
		}

		err = ledger.Insert()
		if err != nil {
			return nil, err
		}

		ledgers = append(ledgers, *ledger)
	}

	return ledgers, nil
}

/*
func (customerWithdrawal *CustomerWithdrawal) CreateLedger() (ledgers []Ledger, err error) {
	store, err := FindStoreByID(customerWithdrawal.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()

	customer, err := store.FindCustomerByID(customerWithdrawal.CustomerID, bson.M{})
	if err != nil {
		return nil, err
	}

	referenceModel := "customer"
	customerAccount, err := store.CreateAccountIfNotExists(
		customerWithdrawal.StoreID,
		&customer.ID,
		&referenceModel,
		customer.Name,
		&customer.Phone,
		&customer.VATNo,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(customerWithdrawal.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(customerWithdrawal.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	for _, payment := range customerWithdrawal.Payments {
		journals := []Journal{}

		receivingAccount := Account{}
		if payment.Method == "cash" {
			receivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			receivingAccount = *bankAccount
		}

		groupID := primitive.NewObjectID()

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     receivingAccount.ID,
			AccountNumber: receivingAccount.Number,
			AccountName:   receivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         *payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     customerAccount.ID,
			AccountNumber: customerAccount.Number,
			AccountName:   customerAccount.Name,
			DebitOrCredit: "credit",
			Credit:        *payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

		ledger := &Ledger{
			StoreID:        customerWithdrawal.StoreID,
			ReferenceID:    customerWithdrawal.ID,
			ReferenceModel: "customer_withdrawal",
			ReferenceCode:  customerWithdrawal.Code,
			Journals:       journals,
			CreatedAt:      &now,
			UpdatedAt:      &now,
		}

		err = ledger.Insert()
		if err != nil {
			return nil, err
		}

		ledgers = append(ledgers, *ledger)
	}

	return ledgers, nil
}*/
