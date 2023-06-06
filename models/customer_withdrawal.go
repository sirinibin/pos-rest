package models

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// CustomerWithdrawal : CustomerWithdrawal structure
type CustomerWithdrawal struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code          string              `bson:"code,omitempty" json:"code,omitempty"`
	Amount        float64             `bson:"amount" json:"amount"`
	Description   string              `bson:"description,omitempty" json:"description,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty"`
	CustomerID    *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName  string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	PaymentMethod string              `json:"payment_method" bson:"payment_method"`
	StoreID       *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName     string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode     string              `json:"store_code,omitempty" bson:"store_code,omitempty"`
	Images        []string            `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent []string            `json:"images_content,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser *User               `json:"created_by_user,omitempty"`
	UpdatedByUser *User               `json:"updated_by_user,omitempty"`
	CategoryName  []string            `json:"category_name" bson:"category_name"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted       bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser *User               `json:"deleted_by_user,omitempty"`
	DeletedAt     *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}

func (customerwithdrawal *CustomerWithdrawal) AttributesValueChangeEvent(customerwithdrawalOld *CustomerWithdrawal) error {

	return nil
}

func (customerwithdrawal *CustomerWithdrawal) UpdateForeignLabelFields() error {

	customerwithdrawal.CategoryName = []string{}

	if customerwithdrawal.CustomerID != nil {
		customer, err := FindCustomerByID(customerwithdrawal.CustomerID, bson.M{"id": 1, "name": 1})
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

func GetCustomerWithdrawalStats(filter map[string]interface{}) (stats CustomerWithdrawalStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
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
		stats.Total = math.Round(stats.Total*100) / 100
	}
	return stats, nil
}

func SearchCustomerWithdrawal(w http.ResponseWriter, r *http.Request) (customerwithdrawals []CustomerWithdrawal, criterias SearchCriterias, err error) {

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

	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
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

func (customerwithdrawal *CustomerWithdrawal) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if customerwithdrawal.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsCustomerWithdrawalExists(&customerwithdrawal.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid CustomerWithdrawal:" + customerwithdrawal.ID.Hex()
		}

	}

	if customerwithdrawal.StoreID == nil || customerwithdrawal.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(customerwithdrawal.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + customerwithdrawal.StoreID.Hex()
			return errs
		}
	}

	if customerwithdrawal.CustomerID == nil || customerwithdrawal.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(customerwithdrawal.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + customerwithdrawal.CustomerID.Hex()
		}
	}

	if govalidator.IsNull(customerwithdrawal.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if customerwithdrawal.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if govalidator.IsNull(customerwithdrawal.Description) {
		errs["description"] = "Description is required"
	}

	if govalidator.IsNull(customerwithdrawal.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, customerwithdrawal.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		customerwithdrawal.Date = &date
	}

	for k, imageContent := range customerwithdrawal.ImagesContent {
		splits := strings.Split(imageContent, ",")

		if len(splits) == 2 {
			customerwithdrawal.ImagesContent[k] = splits[1]
		} else if len(splits) == 1 {
			customerwithdrawal.ImagesContent[k] = splits[0]
		}

		valid, err := IsStringBase64(customerwithdrawal.ImagesContent[k])
		if err != nil {
			errs["images_content"] = err.Error()
		}

		if !valid {
			errs["images_"+strconv.Itoa(k)] = "Invalid base64 string"
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func FindLastCustomerWithdrawal(
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{}, findOneOptions).
		Decode(&customerwithdrawal)
	if err != nil {
		return nil, err
	}

	return customerwithdrawal, err
}

func FindLastCustomerWithdrawalByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
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

func (customerwithdrawal *CustomerWithdrawal) MakeCode() error {
	lastCustomerWithdrawal, err := FindLastCustomerWithdrawalByStoreID(customerwithdrawal.StoreID, bson.M{})
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
}

func (customerwithdrawal *CustomerWithdrawal) Insert() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	customerwithdrawal.ID = primitive.NewObjectID()

	if len(customerwithdrawal.Code) == 0 {
		err = customerwithdrawal.MakeCode()
		if err != nil {
			log.Print("Error making code")
			return err
		}
	}

	if len(customerwithdrawal.ImagesContent) > 0 {
		err := customerwithdrawal.SaveImages()
		if err != nil {
			return err
		}
	}

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

func (customerwithdrawal *CustomerWithdrawal) SaveImages() error {

	for _, imageContent := range customerwithdrawal.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		filename := "images/customer_withdrawals/" + GenerateFileName("customerwithdrawal_", extension)
		err = SaveBase64File(filename, content)
		if err != nil {
			return err
		}
		customerwithdrawal.Images = append(customerwithdrawal.Images, "/"+filename)
	}

	customerwithdrawal.ImagesContent = []string{}

	return nil
}

func (customerwithdrawal *CustomerWithdrawal) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	if len(customerwithdrawal.ImagesContent) > 0 {
		err := customerwithdrawal.SaveImages()
		if err != nil {
			return err
		}
	}

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
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
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

func FindCustomerWithdrawalByCode(
	code string,
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"code": code}, findOneOptions).
		Decode(&customerwithdrawal)
	if err != nil {
		return nil, err
	}

	return customerwithdrawal, err
}

func FindCustomerWithdrawalByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerwithdrawal *CustomerWithdrawal, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
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

func (customerwithdrawal *CustomerWithdrawal) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
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
}

func IsCustomerWithdrawalExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessCustomerWithdrawals() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerwithdrawal")
	ctx := context.Background()
	findOptions := options.Find()

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

		/*
			store, err := FindStoreByCode("GUO", bson.M{})
			if err != nil {
				return errors.New("Error finding store:" + err.Error())
			}
			model.StoreID = &store.ID
		*/

		err = model.Update()
		if err != nil {
			return err
		}

	}

	return nil
}
