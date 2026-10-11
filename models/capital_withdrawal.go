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
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
)

// CapitalWithdrawal : CapitalWithdrawal structure
type CapitalWithdrawal struct {
	ID                  primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code                string              `bson:"code,omitempty" json:"code,omitempty"`
	Amount              float64             `bson:"amount" json:"amount"`
	Description         string              `bson:"description,omitempty" json:"description,omitempty"`
	Date                *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr             string              `json:"date_str,omitempty" bson:"-"`
	WithdrawnByUserID   *primitive.ObjectID `json:"withdrawn_by_user_id,omitempty" bson:"withdrawn_by_user_id,omitempty"`
	WithdrawnByUserName string              `json:"withdrawn_by_user_name,omitempty" bson:"withdrawn_by_user_name,omitempty"`
	PaymentMethod       string              `json:"payment_method" bson:"payment_method"`
	StoreID             *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName           string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode           string              `json:"store_code,omitempty" bson:"store_code,omitempty"`
	Images              []string            `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent       []string            `json:"images_content,omitempty"`
	CreatedAt           *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt           *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy           *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy           *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser       *User               `json:"created_by_user,omitempty"`
	UpdatedByUser       *User               `json:"updated_by_user,omitempty"`
	CategoryName        []string            `json:"category_name" bson:"category_name"`
	CreatedByName       string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName       string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName       string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted             bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy           *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser       *User               `json:"deleted_by_user,omitempty"`
	DeletedAt           *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
}

func (capitalwithdrawal *CapitalWithdrawal) AttributesValueChangeEvent(capitalwithdrawalOld *CapitalWithdrawal) error {

	return nil
}

func (capitalwithdrawal *CapitalWithdrawal) UpdateForeignLabelFields() error {

	capitalwithdrawal.CategoryName = []string{}

	if capitalwithdrawal.WithdrawnByUserID != nil {
		user, err := FindUserByID(capitalwithdrawal.WithdrawnByUserID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		capitalwithdrawal.WithdrawnByUserName = user.Name
	}

	if capitalwithdrawal.CreatedBy != nil {
		createdByUser, err := FindUserByID(capitalwithdrawal.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		capitalwithdrawal.CreatedByName = createdByUser.Name
	}

	if capitalwithdrawal.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(capitalwithdrawal.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		capitalwithdrawal.UpdatedByName = updatedByUser.Name
	}

	if capitalwithdrawal.DeletedBy != nil && !capitalwithdrawal.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(capitalwithdrawal.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		capitalwithdrawal.DeletedByName = deletedByUser.Name
	}

	if capitalwithdrawal.StoreID != nil {
		store, err := FindStoreByID(capitalwithdrawal.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		capitalwithdrawal.StoreName = store.Name
		capitalwithdrawal.StoreCode = store.Code
	}

	return nil
}

type CapitalWithdrawalStats struct {
	ID    *primitive.ObjectID `json:"id" bson:"_id"`
	Total float64             `json:"total" bson:"total"`
}

func (store *Store) GetCapitalWithdrawalStats(filter map[string]interface{}) (stats CapitalWithdrawalStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
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

func (store *Store) SearchCapitalWithdrawal(w http.ResponseWriter, r *http.Request) (capitalwithdrawals []CapitalWithdrawal, criterias SearchCriterias, err error) {

	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	var keys []string
	var ok bool

	if err = ParseFloatWithOperatorFilter(r, &criterias, "search[amount]", "amount"); err != nil {
		return capitalwithdrawals, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[payment_method]", "payment_method")

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

	keys, ok = r.URL.Query()["search[category_id]"]
	if ok && len(keys[0]) >= 1 {

		categoryIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range categoryIds {
			categoryID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return capitalwithdrawals, criterias, err
			}
			objecIds = append(objecIds, categoryID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["category_id"] = bson.M{"$in": objecIds}
		}
	}

	if err = ParseObjectIDListFilter(r, &criterias, "search[created_by]", "created_by"); err != nil {
		return capitalwithdrawals, criterias, err
	}

	if err = ParseExactDateFilter(r, &criterias, "search[date_str]", "date", timeZoneOffset); err != nil {
		return capitalwithdrawals, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[from_date]", "search[to_date]", "date", timeZoneOffset); err != nil {
		return capitalwithdrawals, criterias, err
	}

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return capitalwithdrawals, criterias, err
		}
		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["created_at"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return capitalwithdrawals, criterias, err
	}

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return capitalwithdrawals, criterias, err
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")

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
		return capitalwithdrawals, criterias, errors.New("Error fetching capitalwithdrawals:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return capitalwithdrawals, criterias, errors.New("Cursor error:" + err.Error())
		}
		capitalwithdrawal := CapitalWithdrawal{}
		err = cur.Decode(&capitalwithdrawal)
		if err != nil {
			return capitalwithdrawals, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			capitalwithdrawal.CreatedByUser, _ = FindUserByID(capitalwithdrawal.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			capitalwithdrawal.UpdatedByUser, _ = FindUserByID(capitalwithdrawal.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			capitalwithdrawal.DeletedByUser, _ = FindUserByID(capitalwithdrawal.DeletedBy, deletedByUserSelectFields)
		}

		capitalwithdrawals = append(capitalwithdrawals, capitalwithdrawal)
	} //end for loop

	return capitalwithdrawals, criterias, nil

}

func (capitalwithdrawal *CapitalWithdrawal) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(capitalwithdrawal.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if capitalwithdrawal.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsCapitalWithdrawalExists(&capitalwithdrawal.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid CapitalWithdrawal:" + capitalwithdrawal.ID.Hex()
		}

	}

	if capitalwithdrawal.StoreID == nil || capitalwithdrawal.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(capitalwithdrawal.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + capitalwithdrawal.StoreID.Hex()
			return errs
		}
	}

	if capitalwithdrawal.WithdrawnByUserID == nil || capitalwithdrawal.WithdrawnByUserID.IsZero() {
		errs["withdrawn_by_user_id"] = "Withdrawer is required"
	} else {
		exists, err := IsUserExists(capitalwithdrawal.WithdrawnByUserID)
		if err != nil {
			errs["withdrawn_by_user_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["withdrawn_by_user_id"] = "Invalid Withdrawer:" + capitalwithdrawal.WithdrawnByUserID.Hex()
		}
	}

	if govalidator.IsNull(capitalwithdrawal.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if capitalwithdrawal.Amount == 0 {
		errs["amount"] = "Amount is required"
	}

	if govalidator.IsNull(capitalwithdrawal.Description) {
		errs["description"] = "Description is required"
	}

	if govalidator.IsNull(capitalwithdrawal.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, capitalwithdrawal.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		capitalwithdrawal.Date = &date
	}

	for k, imageContent := range capitalwithdrawal.ImagesContent {
		splits := strings.Split(imageContent, ",")

		if len(splits) == 2 {
			capitalwithdrawal.ImagesContent[k] = splits[1]
		} else if len(splits) == 1 {
			capitalwithdrawal.ImagesContent[k] = splits[0]
		}

		valid, err := IsStringBase64(capitalwithdrawal.ImagesContent[k])
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

func (store *Store) FindLastCapitalWithdrawal(
	selectFields map[string]interface{},
) (capitalwithdrawal *CapitalWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": store.ID}, findOneOptions).
		Decode(&capitalwithdrawal)
	if err != nil {
		return nil, err
	}

	return capitalwithdrawal, err
}

func (store *Store) FindLastCapitalWithdrawalByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (capitalwithdrawal *CapitalWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&capitalwithdrawal)
	if err != nil {
		return nil, err
	}

	return capitalwithdrawal, err
}

func (store *Store) GetCapitalWithdrawalCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

// capitalWithdrawalSerial is the store's capital withdrawal number series:
// the serial StartERP keeps in the store (erp.x.serials.capitalWithdrawal,
// default "CWD-" from 1). Capital withdrawals used to be numbered with the
// capital deposit settings, so their numbers ran in the capital series.
func capitalWithdrawalSerial(storeID *primitive.ObjectID) (prefix string, startFrom int64) {
	prefix, startFrom = "CWD", 1
	if storeID == nil {
		return
	}
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var raw struct {
		Erp struct {
			X struct {
				Serials struct {
					CapitalWithdrawal struct {
						Prefix string      `bson:"prefix"`
						Start  interface{} `bson:"start"`
					} `bson:"capitalWithdrawal"`
				} `bson:"serials"`
			} `bson:"x"`
		} `bson:"erp"`
	}
	err := collection.FindOne(ctx, bson.M{"_id": storeID},
		options.FindOne().SetProjection(bson.M{"erp.x.serials.capitalWithdrawal": 1})).Decode(&raw)
	if err != nil {
		return
	}
	s := raw.Erp.X.Serials.CapitalWithdrawal
	if p := strings.TrimSuffix(strings.TrimSpace(s.Prefix), "-"); p != "" {
		prefix = p
	}
	switch v := s.Start.(type) {
	case int32:
		startFrom = int64(v)
	case int64:
		startFrom = v
	case float64:
		startFrom = int64(v)
	}
	if startFrom < 1 {
		startFrom = 1
	}
	return
}

// highestCapitalWithdrawalNumber is the highest number an existing capital
// withdrawal of the store carries (the trailing digits of its code, in the
// old capital series or the new one), so the new series continues after it
// and no number is given twice.
func (store *Store) highestCapitalWithdrawalNumber() (int64, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cur, err := collection.Find(ctx, bson.M{}, options.Find().SetProjection(bson.M{"code": 1}))
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	max := int64(0)
	for cur.Next(ctx) {
		var d struct {
			Code string `bson:"code"`
		}
		if cur.Decode(&d) != nil {
			continue
		}
		if n := trailingNumber(d.Code); n > max {
			max = n
		}
	}
	return max, cur.Err()
}

// trailingNumber is the number at the end of a document code (0 if none).
func trailingNumber(code string) int64 {
	i := len(code)
	for i > 0 && code[i-1] >= '0' && code[i-1] <= '9' {
		i--
	}
	if i == len(code) {
		return 0
	}
	n, err := strconv.ParseInt(code[i:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

const capitalWithdrawalCounterKey = "_capital_withdrawal_cwd_counter"

func (capitalWithdrawal *CapitalWithdrawal) MakeRedisCode() error {
	store, err := FindStoreByID(capitalWithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	prefix, startFrom := capitalWithdrawalSerial(capitalWithdrawal.StoreID)
	// a counter of its own (the old _capital_withdrawal_counter followed the
	// capital series)
	redisKey := capitalWithdrawal.StoreID.Hex() + capitalWithdrawalCounterKey

	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	baseTime := time.Now().In(location)
	if capitalWithdrawal.CreatedAt != nil {
		baseTime = capitalWithdrawal.CreatedAt.In(location)
	}

	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		highest, err := store.highestCapitalWithdrawalNumber()
		if err != nil {
			return err
		}
		initial := startFrom - 1
		if highest > initial {
			initial = highest
		}
		// SetNX: a request that raced this one to the empty key keeps its value
		if err := db.RedisClient.SetNX(redisKey, initial, 0).Err(); err != nil {
			return err
		}
	}

	globalIncr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	var serialNumber int64 = globalIncr
	if strings.Contains(prefix, "DATE") && store.Settings.EnableMonthlySerialNumber {
		monthKey := baseTime.Format("200601")
		monthlyRedisKey := redisKey + "_" + monthKey
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)
			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "capitalwithdrawal")
			if err != nil {
				return err
			}
			if err := db.RedisClient.SetNX(monthlyRedisKey, startFrom+monthlyCount-1, 0).Err(); err != nil {
				return err
			}
		}
		monthlyIncr, err := db.RedisClient.Incr(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		serialNumber = monthlyIncr
	}

	paddingCount := store.CapitalDepositSerialNumber.PaddingCount
	if paddingCount < 1 {
		paddingCount = 4
	}
	capitalWithdrawal.Code = fmt.Sprintf("%s-%0*d", prefix, paddingCount, serialNumber)
	if strings.Contains(capitalWithdrawal.Code, "DATE") {
		capitalWithdrawal.Code = strings.ReplaceAll(capitalWithdrawal.Code, "DATE", baseTime.Format("20060102"))
	}

	return nil
}

func (capitalWithdrawal *CapitalWithdrawal) UnMakeRedisCode() error {
	store, err := FindStoreByID(capitalWithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	prefix, _ := capitalWithdrawalSerial(capitalWithdrawal.StoreID)
	redisKey := capitalWithdrawal.StoreID.Hex() + capitalWithdrawalCounterKey

	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}
	baseTime := time.Now().In(location)
	if capitalWithdrawal.CreatedAt != nil {
		baseTime = capitalWithdrawal.CreatedAt.In(location)
	}

	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	if strings.Contains(prefix, "DATE") && store.Settings.EnableMonthlySerialNumber {
		monthlyRedisKey := redisKey + "_" + baseTime.Format("200601")
		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
func (model *CapitalWithdrawal) MakeCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_capital_withdrawal_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetCapitalWithdrawalCount()
		if err != nil {
			return err
		}

		startFrom := store.CapitalWithdrawalSerialNumber.StartFromCount

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

	paddingCount := store.CapitalWithdrawalSerialNumber.PaddingCount

	model.Code = fmt.Sprintf("%s-%0*d", store.CapitalWithdrawalSerialNumber.Prefix, paddingCount, incr)
	return nil
}
*/

func (capitalwithdrawal *CapitalWithdrawal) Insert() (err error) {
	collection := db.GetDB("store_" + capitalwithdrawal.StoreID.Hex()).Collection("capitalwithdrawal")
	capitalwithdrawal.ID = primitive.NewObjectID()

	err = capitalwithdrawal.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &capitalwithdrawal)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func (capitalwithdrawal *CapitalWithdrawal) SaveImages() error {

	for _, imageContent := range capitalwithdrawal.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		baseFilename := GenerateFileName("capitalwithdrawal_", extension)
		relKey := "images/" + capitalwithdrawal.StoreID.Hex() + "/capital_withdrawals/" + baseFilename
		mimeType := "image/" + strings.TrimPrefix(extension, ".")
		url := SaveFileToStorage(relKey, content, mimeType)
		capitalwithdrawal.Images = append(capitalwithdrawal.Images, url)
	}

	capitalwithdrawal.ImagesContent = []string{}

	return nil
}

func (capitalwithdrawal *CapitalWithdrawal) Update() error {
	collection := db.GetDB("store_" + capitalwithdrawal.StoreID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	if len(capitalwithdrawal.ImagesContent) > 0 {
		err := capitalwithdrawal.SaveImages()
		if err != nil {
			return err
		}
	}

	err := capitalwithdrawal.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": capitalwithdrawal.ID},
		bson.M{"$set": capitalwithdrawal},
		updateOptions,
	)
	return err
}

func (capitalwithdrawal *CapitalWithdrawal) DeleteCapitalWithdrawal(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + capitalwithdrawal.StoreID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = capitalwithdrawal.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	capitalwithdrawal.Deleted = true
	capitalwithdrawal.DeletedBy = &userID
	now := time.Now()
	capitalwithdrawal.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": capitalwithdrawal.ID},
		bson.M{"$set": capitalwithdrawal},
		updateOptions,
	)
	if err != nil {
		return err
	}

	// the postings go with the document (a delete used to leave them on the
	// ledger, so cash and the party's balance still counted it)
	err = capitalwithdrawal.UndoAccounting()
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindCapitalWithdrawalByCode(
	code string,
	selectFields map[string]interface{},
) (capitalwithdrawal *CapitalWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
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
		Decode(&capitalwithdrawal)
	if err != nil {
		return nil, err
	}

	return capitalwithdrawal, err
}

func (store *Store) FindCapitalWithdrawalByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (capitalwithdrawal *CapitalWithdrawal, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
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
		Decode(&capitalwithdrawal)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		capitalwithdrawal.CreatedByUser, _ = FindUserByID(capitalwithdrawal.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		capitalwithdrawal.UpdatedByUser, _ = FindUserByID(capitalwithdrawal.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		capitalwithdrawal.DeletedByUser, _ = FindUserByID(capitalwithdrawal.DeletedBy, fields)
	}

	return capitalwithdrawal, err
}

func (capitalwithdrawal *CapitalWithdrawal) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + capitalwithdrawal.StoreID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if capitalwithdrawal.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": capitalwithdrawal.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": capitalwithdrawal.Code,
			"_id":  bson.M{"$ne": capitalwithdrawal.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) IsCapitalWithdrawalExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (store *Store) ProcessCapitalWithdrawals() error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("capitalwithdrawal")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching capitalwithdrawals" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		model := CapitalWithdrawal{}
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

	}

	return nil
}

// DoAccounting posts a capital withdrawal: the investor's drawings account
// is debited and the cash / bank paid out is credited (as for a dividend).
func (capitalwithdrawal *CapitalWithdrawal) DoAccounting() error {
	ledger, err := capitalwithdrawal.CreateLedger()
	if err != nil {
		return err
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return err
	}
	return nil
}

// UndoAccounting removes the capital withdrawal's ledger and postings and
// recalculates the balances of the accounts they touched.
func (capitalwithdrawal *CapitalWithdrawal) UndoAccounting() error {
	store, err := FindStoreByID(capitalwithdrawal.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(capitalwithdrawal.ID, *capitalwithdrawal.StoreID, bson.M{})
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

	err = store.RemoveLedgerByReferenceID(capitalwithdrawal.ID)
	if err != nil {
		return err
	}

	err = store.RemovePostingsByReferenceID(capitalwithdrawal.ID)
	if err != nil {
		return err
	}

	return SetAccountBalances(ledgerAccounts)
}

func (capitalwithdrawal *CapitalWithdrawal) CreateLedger() (ledger *Ledger, err error) {
	now := time.Now()

	store, err := FindStoreByID(capitalwithdrawal.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	if capitalwithdrawal.WithdrawnByUserID == nil {
		return nil, errors.New("capital withdrawal has no investor")
	}
	investor, err := FindUserByID(capitalwithdrawal.WithdrawnByUserID, bson.M{})
	if err != nil {
		return nil, err
	}

	referenceModel := "withdrawer"
	drawingAccount, err := store.CreateAccountIfNotExists(
		capitalwithdrawal.StoreID,
		&investor.ID,
		&referenceModel,
		investor.Name+" Drawings",
		&investor.Mob,
		nil,
	)
	if err != nil {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(capitalwithdrawal.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(capitalwithdrawal.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	payingAccount := Account{}
	if capitalwithdrawal.PaymentMethod == "cash" {
		payingAccount = *cashAccount
	} else if slices.Contains(BANK_PAYMENT_METHODS, capitalwithdrawal.PaymentMethod) {
		payingAccount = *bankAccount
	}

	groupID := primitive.NewObjectID()

	journals := []Journal{
		{
			Date:          capitalwithdrawal.Date,
			AccountID:     drawingAccount.ID,
			AccountNumber: drawingAccount.Number,
			AccountName:   drawingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         capitalwithdrawal.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		},
		{
			Date:          capitalwithdrawal.Date,
			AccountID:     payingAccount.ID,
			AccountNumber: payingAccount.Number,
			AccountName:   payingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        capitalwithdrawal.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		},
	}

	ledger = &Ledger{
		StoreID:        capitalwithdrawal.StoreID,
		ReferenceID:    capitalwithdrawal.ID,
		ReferenceModel: "capital_withdrawal",
		ReferenceCode:  capitalwithdrawal.Code,
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
