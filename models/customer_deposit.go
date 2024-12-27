package models

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
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

// CustomerDeposit : CustomerDeposit structure
type CustomerDeposit struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code          string              `bson:"code,omitempty" json:"code,omitempty"`
	Amount        float64             `bson:"amount" json:"amount"`
	Description   string              `bson:"description,omitempty" json:"description,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
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

func (customerdeposit *CustomerDeposit) AttributesValueChangeEvent(customerdepositOld *CustomerDeposit) error {

	return nil
}

func (customerdeposit *CustomerDeposit) UpdateForeignLabelFields() error {

	customerdeposit.CategoryName = []string{}

	if customerdeposit.CustomerID != nil {
		customer, err := FindCustomerByID(customerdeposit.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customerdeposit.CustomerName = customer.Name
	}

	if customerdeposit.CreatedBy != nil {
		createdByUser, err := FindUserByID(customerdeposit.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		customerdeposit.CreatedByName = createdByUser.Name
	}

	if customerdeposit.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(customerdeposit.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		customerdeposit.UpdatedByName = updatedByUser.Name
	}

	if customerdeposit.DeletedBy != nil && !customerdeposit.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(customerdeposit.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		customerdeposit.DeletedByName = deletedByUser.Name
	}

	if customerdeposit.StoreID != nil {
		store, err := FindStoreByID(customerdeposit.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		customerdeposit.StoreName = store.Name
		customerdeposit.StoreCode = store.Code
	}

	return nil
}

type CustomerDepositStats struct {
	ID    *primitive.ObjectID `json:"id" bson:"_id"`
	Total float64             `json:"total" bson:"total"`
}

func GetCustomerDepositStats(filter map[string]interface{}) (stats CustomerDepositStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
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
		stats.Total = RoundFloat(stats.Total, 2)
	}
	return stats, nil
}

func SearchCustomerDeposit(w http.ResponseWriter, r *http.Request) (customerdeposits []CustomerDeposit, criterias SearchCriterias, err error) {

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
			return customerdeposits, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["amount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["amount"] = float64(value)
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

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customerdeposits, criterias, err
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
				return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
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
		return customerdeposits, criterias, errors.New("Error fetching customerdeposits:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return customerdeposits, criterias, errors.New("Cursor error:" + err.Error())
		}
		customerdeposit := CustomerDeposit{}
		err = cur.Decode(&customerdeposit)
		if err != nil {
			return customerdeposits, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			customerdeposit.CreatedByUser, _ = FindUserByID(customerdeposit.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			customerdeposit.UpdatedByUser, _ = FindUserByID(customerdeposit.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			customerdeposit.DeletedByUser, _ = FindUserByID(customerdeposit.DeletedBy, deletedByUserSelectFields)
		}

		customerdeposits = append(customerdeposits, customerdeposit)
	} //end for loop

	return customerdeposits, criterias, nil

}

func (customerdeposit *CustomerDeposit) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if customerdeposit.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsCustomerDepositExists(&customerdeposit.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid CustomerDeposit:" + customerdeposit.ID.Hex()
		}

	}

	if customerdeposit.StoreID == nil || customerdeposit.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(customerdeposit.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + customerdeposit.StoreID.Hex()
			return errs
		}
	}

	if customerdeposit.CustomerID == nil || customerdeposit.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(customerdeposit.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + customerdeposit.CustomerID.Hex()
		}
	}

	if govalidator.IsNull(customerdeposit.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if customerdeposit.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if govalidator.IsNull(customerdeposit.Description) {
		errs["description"] = "Description is required"
	}

	if govalidator.IsNull(customerdeposit.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, customerdeposit.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		customerdeposit.Date = &date
	}

	for k, imageContent := range customerdeposit.ImagesContent {
		splits := strings.Split(imageContent, ",")

		if len(splits) == 2 {
			customerdeposit.ImagesContent[k] = splits[1]
		} else if len(splits) == 1 {
			customerdeposit.ImagesContent[k] = splits[0]
		}

		valid, err := IsStringBase64(customerdeposit.ImagesContent[k])
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

func FindLastCustomerDeposit(
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
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
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

func FindLastCustomerDepositByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

func (customerdeposit *CustomerDeposit) MakeCode() error {
	lastCustomerDeposit, err := FindLastCustomerDepositByStoreID(customerdeposit.StoreID, bson.M{})
	if err != nil && mongo.ErrNoDocuments != err {
		return err
	}
	if lastCustomerDeposit == nil {
		store, err := FindStoreByID(customerdeposit.StoreID, bson.M{})
		if err != nil {
			return err
		}
		customerdeposit.Code = store.Code + "-100000"
	} else {
		splits := strings.Split(lastCustomerDeposit.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			customerdeposit.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := customerdeposit.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastCustomerDeposit.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		customerdeposit.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func (customerdeposit *CustomerDeposit) Insert() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	customerdeposit.ID = primitive.NewObjectID()

	if len(customerdeposit.Code) == 0 {
		err = customerdeposit.MakeCode()
		if err != nil {
			log.Print("Error making code")
			return err
		}
	}

	if len(customerdeposit.ImagesContent) > 0 {
		err := customerdeposit.SaveImages()
		if err != nil {
			return err
		}
	}

	err = customerdeposit.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &customerdeposit)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func (customerdeposit *CustomerDeposit) SaveImages() error {

	for _, imageContent := range customerdeposit.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		filename := "images/customer_deposits/" + GenerateFileName("customerdeposit_", extension)
		err = SaveBase64File(filename, content)
		if err != nil {
			return err
		}
		customerdeposit.Images = append(customerdeposit.Images, "/"+filename)
	}

	customerdeposit.ImagesContent = []string{}

	return nil
}

func (customerdeposit *CustomerDeposit) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	if len(customerdeposit.ImagesContent) > 0 {
		err := customerdeposit.SaveImages()
		if err != nil {
			return err
		}
	}

	err := customerdeposit.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customerdeposit.ID},
		bson.M{"$set": customerdeposit},
		updateOptions,
	)
	return err
}

func (customerdeposit *CustomerDeposit) DeleteCustomerDeposit(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = customerdeposit.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	customerdeposit.Deleted = true
	customerdeposit.DeletedBy = &userID
	now := time.Now()
	customerdeposit.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customerdeposit.ID},
		bson.M{"$set": customerdeposit},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindCustomerDepositByCode(
	code string,
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"code": code}, findOneOptions).
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

func FindCustomerDepositByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		customerdeposit.CreatedByUser, _ = FindUserByID(customerdeposit.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		customerdeposit.UpdatedByUser, _ = FindUserByID(customerdeposit.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		customerdeposit.DeletedByUser, _ = FindUserByID(customerdeposit.DeletedBy, fields)
	}

	return customerdeposit, err
}

func (customerdeposit *CustomerDeposit) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customerdeposit.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": customerdeposit.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": customerdeposit.Code,
			"_id":  bson.M{"$ne": customerdeposit.ID},
		})
	}

	return (count > 0), err
}

func IsCustomerDepositExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessCustomerDeposits() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("customerdeposit")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching customerdeposits" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := CustomerDeposit{}
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

func (customerDeposit *CustomerDeposit) DoAccounting() error {
	ledger, err := customerDeposit.CreateLedger()
	if err != nil {
		return err
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return err
	}
	return nil
}

func (customerDeposit *CustomerDeposit) UndoAccounting() error {
	ledger, err := FindLedgerByReferenceID(customerDeposit.ID, *customerDeposit.StoreID, bson.M{})
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

	err = RemoveLedgerByReferenceID(customerDeposit.ID)
	if err != nil {
		return err
	}

	err = RemovePostingsByReferenceID(customerDeposit.ID)
	if err != nil {
		return err
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return err
	}

	return nil
}

func (customerDeposit *CustomerDeposit) CreateLedger() (ledger *Ledger, err error) {
	now := time.Now()

	customer, err := FindCustomerByID(customerDeposit.CustomerID, bson.M{})
	if err != nil {
		return nil, err
	}

	referenceModel := "customer"
	customerAccount, err := CreateAccountIfNotExists(
		customerDeposit.StoreID,
		&customer.ID,
		&referenceModel,
		customer.Name,
		&customer.Phone,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := CreateAccountIfNotExists(customerDeposit.StoreID, nil, nil, "Cash", nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := CreateAccountIfNotExists(customerDeposit.StoreID, nil, nil, "Bank", nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	receivingAccount := Account{}
	if customerDeposit.PaymentMethod == "cash" {
		receivingAccount = *cashAccount
	} else if customerDeposit.PaymentMethod == "bank_account" {
		receivingAccount = *bankAccount
	}

	groupID := primitive.NewObjectID()

	journals = append(journals, Journal{
		Date:          customerDeposit.Date,
		AccountID:     receivingAccount.ID,
		AccountNumber: receivingAccount.Number,
		AccountName:   receivingAccount.Name,
		DebitOrCredit: "debit",
		Debit:         customerDeposit.Amount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	journals = append(journals, Journal{
		Date:          customerDeposit.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "credit",
		Credit:        customerDeposit.Amount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	ledger = &Ledger{
		StoreID:        customerDeposit.StoreID,
		ReferenceID:    customerDeposit.ID,
		ReferenceModel: "customer_deposit",
		ReferenceCode:  customerDeposit.Code,
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
