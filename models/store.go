package models

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

//Store : Store structure
type Store struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name            string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic    string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	Title           string             `bson:"title,omitempty" json:"title,omitempty"`
	TitleInArabic   string             `bson:"title_in_arabic,omitempty" json:"title_in_arabic"`
	Email           string             `bson:"email,omitempty" json:"email,omitempty"`
	Phone           string             `bson:"phone,omitempty" json:"phone,omitempty"`
	PhoneInArabic   string             `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic,omitempty"`
	Address         string             `bson:"address,omitempty" json:"address,omitempty"`
	AddressInArabic string             `bson:"address_in_arabic,omitempty" json:"address_in_arabic,omitempty"`
	VATNo           string             `bson:"vat_no,omitempty" json:"vat_no,omitempty"`
	VATNoInArabic   string             `bson:"vat_no_in_arabic,omitempty" json:"vat_no_in_arabic,omitempty"`
	VatPercent      *float32           `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	Logo            string             `bson:"logo,omitempty" json:"logo,omitempty"`
	LogoContent     string             `json:"logo_content,omitempty"`
	Deleted         bool               `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy       primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt       time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt       time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy       primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

type SearchCriterias struct {
	Page     int                    `bson:"page,omitempty" json:"page,omitempty"`
	Size     int                    `bson:"size,omitempty" json:"size,omitempty"`
	SearchBy map[string]interface{} `bson:"search_by,omitempty" json:"search_by,omitempty"`
	SortBy   map[string]interface{} `bson:"sort_by,omitempty" json:"sort_by,omitempty"`
}

func GetSortByFields(sortString string) (sortBy map[string]interface{}) {
	sortFieldWithOrder := strings.Fields(sortString)
	sortBy = map[string]interface{}{}

	if len(sortFieldWithOrder) == 2 {
		if sortFieldWithOrder[1] == "1" {
			sortBy[sortFieldWithOrder[0]] = 1 // Sort by Ascending order
		} else if sortFieldWithOrder[1] == "-1" {
			sortBy[sortFieldWithOrder[0]] = -1 // Sort by Descending order
		}
	} else if len(sortFieldWithOrder) == 1 {
		sortBy[sortFieldWithOrder[0]] = 1 // Sort by Ascending order
	}

	return sortBy
}

func SearchStore(w http.ResponseWriter, r *http.Request) (storees []Store, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})

	keys, ok := r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	//Fetch all device documents with (garbage:true AND (gc_processed:false if exist OR gc_processed not exist ))
	/* Note: Actual Record fetching will not happen here
	as it is using mongodb cursor and record fetching will
	start with we call cur.Next()
	*/
	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return storees, criterias, errors.New("Error fetching storees:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return storees, criterias, errors.New("Cursor error:" + err.Error())
		}
		store := Store{}
		err = cur.Decode(&store)
		if err != nil {
			return storees, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		storees = append(storees, store)
	} //end for loop

	return storees, criterias, nil

}

func (store *Store) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if store.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsStoreExists(store.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Store:" + store.ID.Hex()
		}

	}

	if govalidator.IsNull(store.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(store.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(store.Address) {
		errs["address"] = "Address is required"
	}

	if govalidator.IsNull(store.Phone) {
		errs["phone"] = "Phone is required"
	}

	if store.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if store.ID.IsZero() {
		if govalidator.IsNull(store.LogoContent) {
			errs["logo_content"] = "Logo is required"
		}
	}

	if !govalidator.IsNull(store.LogoContent) {
		valid, err := IsStringBase64(store.LogoContent)
		if err != nil {
			errs["logo_content"] = err.Error()
		}

		if !valid {
			errs["logo_content"] = "Invalid base64 string"
		}
	}

	emailExists, err := store.IsEmailExists()
	if err != nil {
		errs["email"] = err.Error()
	}

	if emailExists {
		errs["email"] = "E-mail is Already in use"
	}

	if emailExists {
		w.WriteHeader(http.StatusConflict)
	} else if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (store *Store) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store.ID = primitive.NewObjectID()

	if !govalidator.IsNull(store.LogoContent) {
		err := store.SaveLogoFile()
		if err != nil {
			return err
		}
	}

	_, err := collection.InsertOne(ctx, &store)
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) SaveLogoFile() error {
	content, err := base64.StdEncoding.DecodeString(store.LogoContent)
	if err != nil {
		return err
	}

	extension, err := GetFileExtensionFromBase64(content)
	if err != nil {
		return err
	}

	filename := "images/store/logo_" + store.ID.Hex() + extension
	err = SaveBase64File(filename, content)
	if err != nil {
		return err
	}
	store.Logo = filename
	store.LogoContent = ""
	return nil
}

func (store *Store) Update() (*Store, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	if !govalidator.IsNull(store.LogoContent) {
		err := store.SaveLogoFile()
		if err != nil {
			return nil, err
		}
	}
	store.LogoContent = ""

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": store.ID},
		bson.M{"$set": store},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return store, nil
	}
	return nil, nil
}

func (store *Store) DeleteStore(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	store.Deleted = true
	store.DeletedBy = userID
	store.DeletedAt = time.Now().Local()

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": store.ID},
		bson.M{"$set": store},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindStoreByID(ID primitive.ObjectID) (store *Store, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}).
		Decode(&store)
	if err != nil {
		return nil, err
	}

	return store, err
}

func (store *Store) IsEmailExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if store.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": store.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": store.Email,
			"_id":   bson.M{"$ne": store.ID},
		})
	}

	return (count == 1), err
}

func IsStoreExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
