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
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
	"gopkg.in/mgo.v2/bson"
)

// Capital : Capital structure
type Capital struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code               string              `bson:"code,omitempty" json:"code,omitempty"`
	Amount             float64             `bson:"amount" json:"amount"`
	Description        string              `bson:"description,omitempty" json:"description,omitempty"`
	Date               *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr            string              `json:"date_str,omitempty" bson:"-"`
	InvestedByUserID   *primitive.ObjectID `json:"invested_by_user_id,omitempty" bson:"invested_by_user_id,omitempty"`
	InvestedByUserName string              `json:"invested_by_user_name,omitempty" bson:"invested_by_user_name,omitempty"`
	PaymentMethod      string              `json:"payment_method" bson:"payment_method"`
	StoreID            *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName          string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode          string              `json:"store_code,omitempty" bson:"store_code,omitempty"`
	Images             []string            `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent      []string            `json:"images_content,omitempty"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser      *User               `json:"created_by_user,omitempty"`
	UpdatedByUser      *User               `json:"updated_by_user,omitempty"`
	CategoryName       []string            `json:"category_name" bson:"category_name"`
	CreatedByName      string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName      string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted            bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy          *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser      *User               `json:"deleted_by_user,omitempty"`
	DeletedAt          *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}

func (capital *Capital) AttributesValueChangeEvent(capitalOld *Capital) error {

	return nil
}

func (capital *Capital) UpdateForeignLabelFields() error {

	capital.CategoryName = []string{}

	if capital.InvestedByUserID != nil {
		user, err := FindUserByID(capital.InvestedByUserID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		capital.InvestedByUserName = user.Name
	}

	if capital.CreatedBy != nil {
		createdByUser, err := FindUserByID(capital.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		capital.CreatedByName = createdByUser.Name
	}

	if capital.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(capital.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		capital.UpdatedByName = updatedByUser.Name
	}

	if capital.DeletedBy != nil && !capital.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(capital.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		capital.DeletedByName = deletedByUser.Name
	}

	if capital.StoreID != nil {
		store, err := FindStoreByID(capital.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		capital.StoreName = store.Name
		capital.StoreCode = store.Code
	}

	return nil
}

type CapitalStats struct {
	ID    *primitive.ObjectID `json:"id" bson:"_id"`
	Total float64             `json:"total" bson:"total"`
}

func GetCapitalStats(filter map[string]interface{}) (stats CapitalStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
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

func SearchCapital(w http.ResponseWriter, r *http.Request) (capitals []Capital, criterias SearchCriterias, err error) {

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
			return capitals, criterias, err
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

	keys, ok = r.URL.Query()["search[invested_by_user_id]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			categoryID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return capitals, criterias, err
			}
			objecIds = append(objecIds, categoryID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["invested_by_user_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return capitals, criterias, err
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
			return capitals, criterias, err
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
			return capitals, criterias, err
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
			return capitals, criterias, err
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
			return capitals, criterias, err
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
			return capitals, criterias, err
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
			return capitals, criterias, err
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
			return capitals, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
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
		return capitals, criterias, errors.New("Error fetching capitals:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return capitals, criterias, errors.New("Cursor error:" + err.Error())
		}
		capital := Capital{}
		err = cur.Decode(&capital)
		if err != nil {
			return capitals, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			capital.CreatedByUser, _ = FindUserByID(capital.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			capital.UpdatedByUser, _ = FindUserByID(capital.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			capital.DeletedByUser, _ = FindUserByID(capital.DeletedBy, deletedByUserSelectFields)
		}

		capitals = append(capitals, capital)
	} //end for loop

	return capitals, criterias, nil

}

func (capital *Capital) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if capital.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsCapitalExists(&capital.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Capital:" + capital.ID.Hex()
		}

	}

	if capital.StoreID == nil || capital.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(capital.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + capital.StoreID.Hex()
			return errs
		}
	}

	if capital.InvestedByUserID == nil || capital.InvestedByUserID.IsZero() {
		errs["invested_by_user_id"] = "Invester is required"
	} else {
		exists, err := IsUserExists(capital.InvestedByUserID)
		if err != nil {
			errs["invested_by_user_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["invested_by_user_id"] = "Invalid Customer:" + capital.InvestedByUserID.Hex()
		}
	}

	if govalidator.IsNull(capital.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if capital.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if govalidator.IsNull(capital.Description) {
		errs["description"] = "Description is required"
	}

	if govalidator.IsNull(capital.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, capital.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		capital.Date = &date
	}

	for k, imageContent := range capital.ImagesContent {
		splits := strings.Split(imageContent, ",")

		if len(splits) == 2 {
			capital.ImagesContent[k] = splits[1]
		} else if len(splits) == 1 {
			capital.ImagesContent[k] = splits[0]
		}

		valid, err := IsStringBase64(capital.ImagesContent[k])
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

func FindLastCapital(
	selectFields map[string]interface{},
) (capital *Capital, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
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
		Decode(&capital)
	if err != nil {
		return nil, err
	}

	return capital, err
}

func FindLastCapitalByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (capital *Capital, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&capital)
	if err != nil {
		return nil, err
	}

	return capital, err
}

func (capital *Capital) MakeCode() error {
	lastCapital, err := FindLastCapitalByStoreID(capital.StoreID, bson.M{})
	if err != nil && mongo.ErrNoDocuments != err {
		return err
	}
	if lastCapital == nil {
		store, err := FindStoreByID(capital.StoreID, bson.M{})
		if err != nil {
			return err
		}
		capital.Code = store.Code + "-100000"
	} else {
		splits := strings.Split(lastCapital.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			capital.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := capital.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastCapital.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		capital.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func (capital *Capital) Insert() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	capital.ID = primitive.NewObjectID()

	if len(capital.Code) == 0 {
		err = capital.MakeCode()
		if err != nil {
			log.Print("Error making code")
			return err
		}
	}

	if len(capital.ImagesContent) > 0 {
		err := capital.SaveImages()
		if err != nil {
			return err
		}
	}

	err = capital.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &capital)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func (capital *Capital) SaveImages() error {

	for _, imageContent := range capital.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		filename := "images/capitals/" + GenerateFileName("capital_", extension)
		err = SaveBase64File(filename, content)
		if err != nil {
			return err
		}
		capital.Images = append(capital.Images, "/"+filename)
	}

	capital.ImagesContent = []string{}

	return nil
}

func (capital *Capital) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	if len(capital.ImagesContent) > 0 {
		err := capital.SaveImages()
		if err != nil {
			return err
		}
	}

	err := capital.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": capital.ID},
		bson.M{"$set": capital},
		updateOptions,
	)
	return err
}

func (capital *Capital) DeleteCapital(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = capital.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	capital.Deleted = true
	capital.DeletedBy = &userID
	now := time.Now()
	capital.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": capital.ID},
		bson.M{"$set": capital},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindCapitalByCode(
	code string,
	selectFields map[string]interface{},
) (capital *Capital, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"code": code}, findOneOptions).
		Decode(&capital)
	if err != nil {
		return nil, err
	}

	return capital, err
}

func FindCapitalByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (capital *Capital, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&capital)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		capital.CreatedByUser, _ = FindUserByID(capital.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		capital.UpdatedByUser, _ = FindUserByID(capital.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		capital.DeletedByUser, _ = FindUserByID(capital.DeletedBy, fields)
	}

	return capital, err
}

func (capital *Capital) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if capital.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": capital.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": capital.Code,
			"_id":  bson.M{"$ne": capital.ID},
		})
	}

	return (count > 0), err
}

func IsCapitalExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessCapitals() error {
	totalCount, err := GetTotalCount(bson.M{}, "capital")
	if err != nil {
		return err
	}

	collection := db.Client().Database(db.GetPosDB()).Collection("capital")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching capitals" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	bar := progressbar.Default(totalCount)
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := Capital{}
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

		err = model.UndoAccounting()
		if err != nil {
			return errors.New("error undo accounting: " + err.Error())
		}

		err = model.DoAccounting()
		if err != nil {
			return errors.New("error doing accounting: " + err.Error())
		}

		err = model.Update()
		if err != nil {
			return err
		}
		bar.Add(1)
	}

	return nil
}

func (capital *Capital) DoAccounting() error {
	ledger, err := capital.CreateLedger()
	if err != nil {
		return err
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return err
	}
	return nil
}

func (capital *Capital) UndoAccounting() error {
	ledger, err := FindLedgerByReferenceID(capital.ID, *capital.StoreID, bson.M{})
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

	err = RemoveLedgerByReferenceID(capital.ID)
	if err != nil {
		return err
	}

	err = RemovePostingsByReferenceID(capital.ID)
	if err != nil {
		return err
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return err
	}

	return nil
}

func (capital *Capital) CreateLedger() (ledger *Ledger, err error) {
	now := time.Now()

	investor, err := FindUserByID(capital.InvestedByUserID, bson.M{})
	if err != nil {
		return nil, err
	}

	referenceModel := "investor"
	investorAccount, err := CreateAccountIfNotExists(
		capital.StoreID,
		&investor.ID,
		&referenceModel,
		investor.Name+" Capital",
		&investor.Mob,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := CreateAccountIfNotExists(capital.StoreID, nil, nil, "Cash", nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := CreateAccountIfNotExists(capital.StoreID, nil, nil, "Bank", nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	receivingAccount := Account{}
	if capital.PaymentMethod == "cash" {
		receivingAccount = *cashAccount
	} else if slices.Contains(BANK_PAYMENT_METHODS, capital.PaymentMethod) {
		receivingAccount = *bankAccount
	}

	groupID := primitive.NewObjectID()

	journals = append(journals, Journal{
		Date:          capital.Date,
		AccountID:     receivingAccount.ID,
		AccountNumber: receivingAccount.Number,
		AccountName:   receivingAccount.Name,
		DebitOrCredit: "debit",
		Debit:         capital.Amount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	journals = append(journals, Journal{
		Date:          capital.Date,
		AccountID:     investorAccount.ID,
		AccountNumber: investorAccount.Number,
		AccountName:   investorAccount.Name,
		DebitOrCredit: "credit",
		Credit:        capital.Amount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	ledger = &Ledger{
		StoreID:        capital.StoreID,
		ReferenceID:    capital.ID,
		ReferenceModel: "capital",
		ReferenceCode:  capital.Code,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	err = ledger.Insert()
	if err != nil {
		return nil, err
	}

	/*
		err = investorAccount.CalculateBalance()
		if err != nil {
			return nil, errors.New("error calculating account balance: " + err.Error())
		}
	*/

	return ledger, nil
}
