package models

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
)

// Expense : Expense structure
type Expense struct {
	ID                  primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Code                string                `bson:"code,omitempty" json:"code,omitempty"`
	Amount              float64               `bson:"amount" json:"amount"`
	Description         string                `bson:"description" json:"description"`
	Date                *time.Time            `bson:"date,omitempty" json:"date,omitempty"`
	DateStr             string                `json:"date_str,omitempty" bson:"-"`
	PaymentMethod       string                `json:"payment_method" bson:"payment_method"`
	StoreID             *primitive.ObjectID   `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName           string                `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode           string                `json:"store_code,omitempty" bson:"store_code,omitempty"`
	CategoryID          []*primitive.ObjectID `json:"category_id" bson:"category_id"`
	Category            []*ExpenseCategory    `json:"category,omitempty"`
	Images              []string              `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent       []string              `json:"images_content,omitempty"`
	CreatedAt           *time.Time            `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt           *time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy           *primitive.ObjectID   `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy           *primitive.ObjectID   `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser       *User                 `json:"created_by_user,omitempty"`
	UpdatedByUser       *User                 `json:"updated_by_user,omitempty"`
	CategoryName        []string              `json:"category_name" bson:"category_name"`
	CreatedByName       string                `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName       string                `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName       string                `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted             bool                  `bson:"deleted" json:"deleted"`
	DeletedBy           *primitive.ObjectID   `json:"deleted_by" bson:"deleted_by"`
	DeletedByUser       *User                 `json:"deleted_by_user" bson:"deleted_by_user"`
	DeletedAt           *time.Time            `bson:"deleted_at" json:"deleted_at"`
	VendorID            *primitive.ObjectID   `json:"vendor_id" bson:"vendor_id"`
	Vendor              *Vendor               `json:"vendor,omitempty" bson:"-"`
	VendorInvoiceNumber string                `bson:"vendor_invoice_no" json:"vendor_invoice_no"`
	Taxable             bool                  `bson:"taxable" json:"taxable"`
	VatPercent          *float64              `bson:"vat_percent" json:"vat_percent"`
	VatPrice            float64               `bson:"vat_price" json:"vat_price"`
	VendorName          string                `json:"vendor_name" bson:"vendor_name"`
	VendorNameArabic    string                `json:"vendor_name_arabic" bson:"vendor_name_arabic"`
}

func (expense *Expense) AttributesValueChangeEvent(expenseOld *Expense) error {

	return nil
}

func (model *Expense) SetPostBalances() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(model.ID, *model.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("Error finding ledger by reference id: " + err.Error())
	}

	if err == mongo.ErrNoDocuments {
		return nil
	}

	err = ledger.SetPostBalancesByLedger(model.Date)
	if err != nil {
		return err
	}

	return nil
}

func (expense *Expense) UpdateForeignLabelFields() error {
	expense.CategoryName = []string{}

	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, categoryID := range expense.CategoryID {
		expenseCategory, err := store.FindExpenseCategoryByID(categoryID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding expense category id:" + categoryID.Hex() + ",error:" + err.Error())
		}
		expense.CategoryName = append(expense.CategoryName, expenseCategory.Name)
	}

	for _, category := range expense.Category {
		expenseCategory, err := store.FindExpenseCategoryByID(&category.ID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding expense category id:" + category.ID.Hex() + ",error:" + err.Error())
		}
		expense.CategoryName = append(expense.CategoryName, expenseCategory.Name)
	}

	if expense.CreatedBy != nil {
		createdByUser, err := FindUserByID(expense.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		expense.CreatedByName = createdByUser.Name
	}

	if expense.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(expense.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		expense.UpdatedByName = updatedByUser.Name
	}

	if expense.DeletedBy != nil && !expense.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(expense.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		expense.DeletedByName = deletedByUser.Name
	}

	if expense.StoreID != nil {
		store, err := FindStoreByID(expense.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		expense.StoreName = store.Name
		expense.StoreCode = store.Code
	}

	if expense.VendorID != nil && !expense.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(expense.VendorID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1})
		if err != nil {
			return err
		}
		expense.VendorName = vendor.Name
		expense.VendorNameArabic = vendor.NameInArabic
	} else {
		expense.VendorName = ""
		expense.VendorNameArabic = ""
	}

	return nil
}

type ExpenseStats struct {
	ID           *primitive.ObjectID `json:"id" bson:"_id"`
	Total        float64             `json:"total" bson:"total"`
	Cash         float64             `json:"cash" bson:"cash"`
	Bank         float64             `json:"bank" bson:"bank"`
	PurchaseFund float64             `json:"purchase_fund" bson:"purchase_fund"`
	Vat          float64             `json:"vat" bson:"vat"`
}

func (store *Store) GetExpenseStats(filter map[string]interface{}) (stats ExpenseStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":   nil,
				"total": bson.M{"$sum": "$amount"},
				"cash": bson.M{"$sum": bson.M{
					"$cond": bson.A{
						bson.M{"$eq": bson.A{"$payment_method", "cash"}},
						"$amount",
						0,
					},
				}},
				"bank": bson.M{"$sum": bson.M{
					"$cond": []interface{}{
						bson.M{"$in": []interface{}{
							"$payment_method",
							[]interface{}{"debit_card", "credit_card", "bank_card", "bank_transfer", "bank_cheque"},
						}},
						"$amount",
						0,
					},
				}},
				"purchase_fund": bson.M{"$sum": bson.M{
					"$cond": bson.A{
						bson.M{"$eq": bson.A{"$payment_method", "purchase_fund"}},
						"$amount",
						0,
					},
				}},
				"vat": bson.M{"$sum": "$vat_price"},
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

func (store *Store) SearchExpense(w http.ResponseWriter, r *http.Request) (expenses []Expense, criterias SearchCriterias, err error) {

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
			return expenses, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["amount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["amount"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[payment_method]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethods := strings.Split(keys[0], ",")

		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_method"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {

		vendorIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			vendorID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return expenses, criterias, err
			}
			objecIds = append(objecIds, vendorID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["vendor_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[vat_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return expenses, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["vat_price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["vat_price"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[vendor_invoice_no]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["vendor_invoice_no"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	keys, ok = r.URL.Query()["search[category_id]"]
	if ok && len(keys[0]) >= 1 {

		categoryIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range categoryIds {
			categoryID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return expenses, criterias, err
			}
			objecIds = append(objecIds, categoryID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["category_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return expenses, criterias, err
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
			return expenses, criterias, err
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
			return expenses, criterias, err
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
			return expenses, criterias, err
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
			return expenses, criterias, err
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
			return expenses, criterias, err
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
			return expenses, criterias, err
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
			return expenses, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	categorySelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}
	vendorSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields

		if _, ok := criterias.Select["category.id"]; ok {
			categorySelectFields = ParseRelationalSelectString(keys[0], "category")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
		}

		if _, ok := criterias.Select["vendor.id"]; ok {
			vendorSelectFields = ParseRelationalSelectString(keys[0], "vendor")
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
		return expenses, criterias, errors.New("Error fetching expenses:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return expenses, criterias, errors.New("Cursor error:" + err.Error())
		}
		expense := Expense{}
		err = cur.Decode(&expense)
		if err != nil {
			return expenses, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["category.id"]; ok {
			for _, categoryID := range expense.CategoryID {
				category, _ := store.FindExpenseCategoryByID(categoryID, categorySelectFields)
				expense.Category = append(expense.Category, category)
			}
		}

		if _, ok := criterias.Select["vendor.id"]; ok {
			expense.Vendor, _ = store.FindVendorByID(expense.VendorID, vendorSelectFields)
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			expense.CreatedByUser, _ = FindUserByID(expense.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			expense.UpdatedByUser, _ = FindUserByID(expense.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			expense.DeletedByUser, _ = FindUserByID(expense.DeletedBy, deletedByUserSelectFields)
		}

		expenses = append(expenses, expense)
	} //end for loop

	return expenses, criterias, nil

}

func (expense *Expense) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if expense.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsExpenseExists(&expense.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Expense:" + expense.ID.Hex()
		}
	}

	if expense.StoreID == nil || expense.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(expense.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + expense.StoreID.Hex()
			return errs
		}
	}

	if govalidator.IsNull(expense.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if expense.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if govalidator.IsNull(expense.Description) {
		errs["description"] = "Description is required"
	}

	if govalidator.IsNull(expense.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, expense.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		expense.Date = &date
	}

	if len(expense.CategoryID) == 0 {
		errs["category_id"] = "Atleast 1 category is required"
	} else {
		for i, categoryID := range expense.CategoryID {
			exists, err := store.IsExpenseCategoryExists(categoryID)
			if err != nil {
				errs["category_id_"+strconv.Itoa(i)] = err.Error()
			}

			if !exists {
				errs["category_id_"+strconv.Itoa(i)] = "Invalid category:" + categoryID.Hex()
			}
		}

	}

	for k, imageContent := range expense.ImagesContent {
		splits := strings.Split(imageContent, ",")

		if len(splits) == 2 {
			expense.ImagesContent[k] = splits[1]
		} else if len(splits) == 1 {
			expense.ImagesContent[k] = splits[0]
		}

		valid, err := IsStringBase64(expense.ImagesContent[k])
		if err != nil {
			errs["images_content"] = err.Error()
		}

		if !valid {
			errs["images_"+strconv.Itoa(k)] = "Invalid base64 string"
		}
	}

	var vendor *Vendor
	if expense.VendorID != nil && !expense.VendorID.IsZero() {
		vendor, err = store.FindVendorByID(expense.VendorID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			errs["vendor_id"] = "error finding vendor"
		}
	}

	if vendor == nil && govalidator.IsNull(expense.VendorName) {
		expense.VendorID = nil
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (store *Store) FindLastExpense(
	selectFields map[string]interface{},
) (expense *Expense, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	//findOneOptions.SetSort(map[string]interface{}{"_id": -1})
	findOneOptions.SetSort(map[string]interface{}{"date": -1})

	err = collection.FindOne(ctx,
		bson.M{
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&expense)
	if err != nil {
		return nil, err
	}

	return expense, err
}

func (store *Store) FindLastExpenseByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (expense *Expense, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": store.ID}, findOneOptions).
		Decode(&expense)
	if err != nil {
		return nil, err
	}

	return expense, err
}

func (expense *Expense) CreateNewVendorFromName() error {
	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return err
	}

	vendor, err := store.FindVendorByID(expense.VendorID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if vendor != nil || govalidator.IsNull(expense.VendorName) {
		return nil
	}

	now := time.Now()
	newVendor := Vendor{
		Name:      expense.VendorName,
		CreatedBy: expense.CreatedBy,
		UpdatedBy: expense.CreatedBy,
		CreatedAt: &now,
		UpdatedAt: &now,
		StoreID:   expense.StoreID,
	}

	err = newVendor.MakeCode()
	if err != nil {
		return err
	}

	newVendor.GenerateSearchWords()
	newVendor.SetSearchLabel()
	newVendor.SetAdditionalkeywords()

	err = newVendor.Insert()
	if err != nil {
		return err
	}
	err = newVendor.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	expense.VendorID = &newVendor.ID
	return nil
}

func (expense *Expense) MakeRedisCode() error {
	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := expense.StoreID.Hex() + "_expense_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from order.CreatedAt or fallback to order.Date or now ===
	baseTime := expense.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("expense")
		if err != nil {
			return err
		}
		startFrom := store.ExpenseSerialNumber.StartFromCount
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
	useMonthly := strings.Contains(store.ExpenseSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := expense.StoreID.Hex() + "_expense_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.ExpenseSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "expense")
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
		if store.Settings.EnableMonthlySerialNumber {
			serialNumber = monthlyIncr
		}
	}

	// === 6. Build the code ===
	paddingCount := store.ExpenseSerialNumber.PaddingCount
	if store.ExpenseSerialNumber.Prefix != "" {
		expense.Code = fmt.Sprintf("%s-%0*d", store.ExpenseSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		expense.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(expense.Code, "DATE") {
		orderDate := baseTime.Format("20060102") // YYYYMMDD
		expense.Code = strings.ReplaceAll(expense.Code, "DATE", orderDate)
	}

	return nil
}

func (expense *Expense) UnMakeRedisCode() error {
	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := expense.StoreID.Hex() + "_expense_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := expense.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.ExpenseSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := expense.StoreID.Hex() + "_expense_counter_" + monthKey

		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
func (model *Expense) MakeCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_expense_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetExpenseCount()
		if err != nil {
			return err
		}

		startFrom := store.ExpenseSerialNumber.StartFromCount

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

	paddingCount := store.ExpenseSerialNumber.PaddingCount
	if store.ExpenseSerialNumber.Prefix != "" {
		model.Code = fmt.Sprintf("%s-%0*d", store.ExpenseSerialNumber.Prefix, paddingCount, incr)
	} else {
		model.Code = fmt.Sprintf("%s%0*d", store.ExpenseSerialNumber.Prefix, paddingCount, incr)
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
func (expense *Expense) MakeCode() error {
	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return err
	}

	lastExpense, err := store.FindLastExpenseByStoreID(expense.StoreID, bson.M{})
	if err != nil && mongo.ErrNoDocuments != err {
		return err
	}
	if lastExpense == nil {
		store, err := FindStoreByID(expense.StoreID, bson.M{})
		if err != nil {
			return err
		}
		expense.Code = store.Code + "-100000"
	} else {
		splits := strings.Split(lastExpense.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			expense.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := expense.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastExpense.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		expense.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}*/

func (expense *Expense) Insert() (err error) {
	collection := db.GetDB("store_" + expense.StoreID.Hex()).Collection("expense")
	expense.ID = primitive.NewObjectID()

	err = expense.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &expense)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func (expense *Expense) SaveImages() error {

	for _, imageContent := range expense.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		filename := "images/expenses/" + GenerateFileName("expense_", extension)
		err = SaveBase64File(filename, content)
		if err != nil {
			return err
		}
		expense.Images = append(expense.Images, "/"+filename)
	}

	expense.ImagesContent = []string{}

	return nil
}

func (expense *Expense) Update() error {
	collection := db.GetDB("store_" + expense.StoreID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	if len(expense.ImagesContent) > 0 {
		err := expense.SaveImages()
		if err != nil {
			return err
		}
	}

	err := expense.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": expense.ID},
		bson.M{"$set": expense},
		updateOptions,
	)
	return err
}

func (expense *Expense) DeleteExpense(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + expense.StoreID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = expense.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	expense.Deleted = true
	expense.DeletedBy = &userID
	now := time.Now()
	expense.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": expense.ID},
		bson.M{"$set": expense},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindExpenseByCode(
	code string,
	selectFields map[string]interface{},
) (expense *Expense, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"code": code, "store_id": store.ID}, findOneOptions).
		Decode(&expense)
	if err != nil {
		return nil, err
	}

	return expense, err
}

func (store *Store) FindExpenseByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (expense *Expense, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
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
		Decode(&expense)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["category.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "category")
		for _, categoryID := range expense.CategoryID {
			category, _ := store.FindExpenseCategoryByID(categoryID, fields)
			expense.Category = append(expense.Category, category)
		}

	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		expense.CreatedByUser, _ = FindUserByID(expense.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		expense.UpdatedByUser, _ = FindUserByID(expense.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		expense.DeletedByUser, _ = FindUserByID(expense.DeletedBy, fields)
	}

	return expense, err
}

func (expense *Expense) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + expense.StoreID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if expense.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": expense.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": expense.Code,
			"_id":  bson.M{"$ne": expense.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) IsExpenseExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func ProcessExpenses() error {
	log.Print("Processing expenses")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {

		/*if store.Code != "GUOJ" {
			continue
		}*/
		totalCount, err := store.GetTotalCount(bson.M{}, "expense")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)
		//	findOptions.SetSort(bson.M{"date": 1})
		//findOptions.SetSort(bson.M{"date": 1})
		//findOptions.SetSort(bson.M{"date": 1})
		findOptions.SetSort(bson.M{"created_at": 1})

		cur, err := collection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			return errors.New("Error fetching expenses" + err.Error())
		}
		if cur != nil {
			defer cur.Close(ctx)
		}

		//var lastDate *time.Time
		bar := progressbar.Default(totalCount)
		for i := 0; cur != nil && cur.Next(ctx); i++ {
			err := cur.Err()
			if err != nil {
				return errors.New("Cursor error:" + err.Error())
			}
			model := Expense{}
			err = cur.Decode(&model)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			/*model.Date = model.CreatedAt
			model.Update()
			model.UndoAccounting()
			model.DoAccounting()*/

			/*if model.Date == nil || model.Date.IsZero() {
				model.Date = model.CreatedAt
				model.Update()
			}*/

			/*if lastDate != nil {
				// 1=> model.Date>*lastDate
				// 0=>same
				var newDate time.Time
				if model.Date.Compare(*lastDate) == 0 || model.Date.Compare(*lastDate) == -1 {
					// -1=> model.Date<*lastDate
					newDate = lastDate.Add(time.Minute * time.Duration(1))
					// order.Payments[i].Date.Add(1 * time.Minute)
					model.Date = &newDate
					model.Update()
				}

			}

			lastDate = model.Date
			model.UndoAccounting()
			model.DoAccounting()*/

			/*
				err = model.Update()
				if err != nil {
					return err
				}*/

			bar.Add(1)
		}
	}
	log.Print("Expenses DONE!")
	return nil
}

func (expense *Expense) FindNextExpense(selectFields map[string]interface{}) (nextExpense *Expense, err error) {
	collection := db.GetDB("store_" + expense.StoreID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"date": 1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"date":     bson.M{"$gte": expense.Date},
			"_id":      bson.M{"$ne": expense.ID},
			"store_id": expense.StoreID,
		}, findOneOptions).
		Decode(&nextExpense)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return nextExpense, nil
}

func (expense *Expense) FindPreviousExpense(selectFields map[string]interface{}) (previousExpense *Expense, err error) {
	collection := db.GetDB("store_" + expense.StoreID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"date": -1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"date":     bson.M{"$lte": expense.Date},
			"_id":      bson.M{"$ne": expense.ID},
			"store_id": expense.StoreID,
		}, findOneOptions).
		Decode(&previousExpense)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	return previousExpense, nil
}

//Accounting

func (expense *Expense) DoAccounting() error {
	previousExpense, err := expense.FindPreviousExpense(bson.M{})
	if err != nil {
		return errors.New("Error finding previous expense:" + err.Error())
	}

	if previousExpense != nil {
		if IsDateTimesEqual(expense.Date, previousExpense.Date) {
			newTime := expense.Date.Add(1 * time.Minute)
			expense.Date = &newTime
			err = expense.Update()
			if err != nil {
				return err
			}
		}
	}

	nextExpense, err := expense.FindNextExpense(bson.M{})
	if err != nil {
		return errors.New("Error finding next expense:" + err.Error())
	}

	if nextExpense != nil {
		if IsDateTimesEqual(expense.Date, nextExpense.Date) {
			newTime := expense.Date.Add(1 * time.Minute)
			nextExpense.Date = &newTime
			err = nextExpense.Update()
			if err != nil {
				return err
			}

			err = nextExpense.UndoAccounting()
			if err != nil {
				return err
			}
			err = nextExpense.DoAccounting()
			if err != nil {
				return err
			}
		}
	}

	ledger, err := expense.CreateLedger()
	if err != nil {
		return err
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return err
	}
	return nil
}

func (expense *Expense) UndoAccounting() error {
	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(expense.ID, *expense.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	ledgerAccounts := map[string]Account{}

	if ledger != nil {
		ledgerAccounts, err = ledger.GetRelatedAccounts()
		if err != nil {
			return err
		}
	}

	err = store.RemoveLedgerByReferenceID(expense.ID)
	if err != nil {
		return err
	}

	err = store.RemovePostingsByReferenceID(expense.ID)
	if err != nil {
		return err
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return err
	}

	return nil
}

func (expense *Expense) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expenseCategory, err := store.FindExpenseCategoryByID(expense.CategoryID[0], bson.M{})
	if err != nil {
		return nil, err
	}

	referenceModel := "expense_category"
	expenseAccount, err := store.CreateAccountIfNotExists(
		expense.StoreID,
		&expenseCategory.ID,
		&referenceModel,
		expenseCategory.Name+" Expense",
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(expense.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(expense.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	purchaseFundAccount, err := store.CreateAccountIfNotExists(expense.StoreID, nil, nil, "Purchase Fund", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	payingAccount := Account{}
	if expense.PaymentMethod == "cash" {
		payingAccount = *cashAccount
	} else if expense.PaymentMethod == "purchase_fund" {
		payingAccount = *purchaseFundAccount
	} else if slices.Contains(BANK_PAYMENT_METHODS, expense.PaymentMethod) {
		payingAccount = *bankAccount
	}

	groupID := primitive.NewObjectID()

	journals = append(journals, Journal{
		Date:          expense.Date,
		AccountID:     expenseAccount.ID,
		AccountNumber: expenseAccount.Number,
		AccountName:   expenseAccount.Name,
		DebitOrCredit: "debit",
		Debit:         expense.Amount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	journals = append(journals, Journal{
		Date:          expense.Date,
		AccountID:     payingAccount.ID,
		AccountNumber: payingAccount.Number,
		AccountName:   payingAccount.Name,
		DebitOrCredit: "credit",
		Credit:        expense.Amount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	ledger = &Ledger{
		StoreID:        expense.StoreID,
		ReferenceID:    expense.ID,
		ReferenceModel: "expense",
		ReferenceCode:  expense.Code,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	err = ledger.Insert()
	if err != nil {
		return nil, err
	}

	return ledger, nil
}
