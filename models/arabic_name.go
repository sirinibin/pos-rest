package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ArabicName : predefined Arabic name entry with English label
type ArabicName struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	NameInEnglish string              `bson:"name_in_english,omitempty" json:"name_in_english,omitempty"`
	NameInArabic  string              `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	Deleted       bool                `bson:"deleted" json:"deleted"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by" bson:"deleted_by"`
	DeletedAt     *time.Time          `bson:"deleted_at" json:"deleted_at"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
}

func (arabicName *ArabicName) UpdateForeignLabelFields() error {
	if arabicName.CreatedBy != nil {
		createdByUser, err := FindUserByID(arabicName.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		arabicName.CreatedByName = createdByUser.Name
	}

	if arabicName.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(arabicName.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		arabicName.UpdatedByName = updatedByUser.Name
	}

	return nil
}

func (store *Store) SearchArabicName(w http.ResponseWriter, r *http.Request) (arabicNames []ArabicName, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	var keys []string
	var ok bool

	var storeID primitive.ObjectID
	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return arabicNames, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return arabicNames, criterias, err
		}
		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	// Search in both English and Arabic name with a single search[name] param
	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = bson.A{
			bson.M{"name_in_english": bson.M{"$regex": keys[0], "$options": "i"}},
			bson.M{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	ParseTextSearch(r, &criterias, "search[name_in_english]", "name_in_english")
	ParseTextSearch(r, &criterias, "search[name_in_arabic]", "name_in_arabic")

	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return arabicNames, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return arabicNames, criterias, err
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("arabic_name")
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
		return arabicNames, criterias, errors.New("Error fetching arabic names:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		if err := cur.Err(); err != nil {
			return arabicNames, criterias, errors.New("Cursor error:" + err.Error())
		}
		arabicName := ArabicName{}
		if err = cur.Decode(&arabicName); err != nil {
			return arabicNames, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		arabicNames = append(arabicNames, arabicName)
	}

	return arabicNames, criterias, nil
}

func (arabicName *ArabicName) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(arabicName.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if arabicName.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsArabicNameExists(&arabicName.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}
		if !exists {
			errs["id"] = "Invalid Arabic Name:" + arabicName.ID.Hex()
		}
	}

	if govalidator.IsNull(arabicName.NameInEnglish) {
		errs["name_in_english"] = "Name in English is required"
	}

	if govalidator.IsNull(arabicName.NameInArabic) {
		errs["name_in_arabic"] = "Name in Arabic is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (store *Store) IsArabicNameExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, bson.M{"_id": ID})
	return (count > 0), err
}

func (arabicName *ArabicName) Insert() error {
	collection := db.GetDB("store_" + arabicName.StoreID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := arabicName.UpdateForeignLabelFields(); err != nil {
		return err
	}

	arabicName.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &arabicName)
	return err
}

func (arabicName *ArabicName) Update() error {
	collection := db.GetDB("store_" + arabicName.StoreID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := arabicName.UpdateForeignLabelFields(); err != nil {
		return err
	}

	now := time.Now()
	arabicName.UpdatedAt = &now

	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err := collection.UpdateOne(ctx, bson.M{"_id": arabicName.ID}, bson.M{"$set": arabicName}, updateOptions)
	return err
}

func (store *Store) FindArabicNameByID(ID *primitive.ObjectID, selectFields map[string]interface{}) (*ArabicName, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var arabicName ArabicName
	err := collection.FindOne(ctx, bson.M{"_id": ID, "store_id": store.ID}, findOneOptions).Decode(&arabicName)
	if err != nil {
		return nil, err
	}
	return &arabicName, nil
}

func (arabicName *ArabicName) DeleteArabicName(tokenClaims TokenClaims) error {
	collection := db.GetDB("store_" + arabicName.StoreID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	arabicName.Deleted = true
	arabicName.DeletedBy = &userID
	now := time.Now()
	arabicName.DeletedAt = &now

	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err = collection.UpdateOne(ctx, bson.M{"_id": arabicName.ID}, bson.M{"$set": arabicName}, updateOptions)
	return err
}

func (arabicName *ArabicName) HardDeleteArabicName() error {
	collection := db.GetDB("store_" + arabicName.StoreID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.DeleteOne(ctx, bson.M{"_id": arabicName.ID})
	return err
}

func (arabicName *ArabicName) RestoreArabicName(tokenClaims TokenClaims) error {
	collection := db.GetDB("store_" + arabicName.StoreID.Hex()).Collection("arabic_name")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	arabicName.Deleted = false
	arabicName.DeletedBy = nil
	arabicName.DeletedAt = nil

	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err := collection.UpdateOne(ctx, bson.M{"_id": arabicName.ID}, bson.M{"$set": arabicName}, updateOptions)
	return err
}
