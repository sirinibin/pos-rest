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
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// Store : Store structure
type Store struct {
	ID                         primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name                       string                `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic               string                `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	Code                       string                `bson:"code" json:"code"`
	Title                      string                `bson:"title,omitempty" json:"title,omitempty"`
	TitleInArabic              string                `bson:"title_in_arabic,omitempty" json:"title_in_arabic,omitempty"`
	RegistrationNumber         string                `bson:"registration_number,omitempty" json:"registration_number,omitempty"`
	RegistrationNumberInArabic string                `bson:"registration_number_arabic,omitempty" json:"registration_number_in_arabic,omitempty"`
	Email                      string                `bson:"email,omitempty" json:"email,omitempty"`
	Phone                      string                `bson:"phone,omitempty" json:"phone,omitempty"`
	PhoneInArabic              string                `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic,omitempty"`
	Address                    string                `bson:"address,omitempty" json:"address,omitempty"`
	AddressInArabic            string                `bson:"address_in_arabic,omitempty" json:"address_in_arabic,omitempty"`
	ZipCode                    string                `bson:"zipcode,omitempty" json:"zipcode,omitempty"`
	ZipCodeInArabic            string                `bson:"zipcode_in_arabic,omitempty" json:"zipcode_in_arabic,omitempty"`
	VATNo                      string                `bson:"vat_no" json:"vat_no"`
	VATNoInArabic              string                `bson:"vat_no_in_arabic,omitempty" json:"vat_no_in_arabic,omitempty"`
	VatPercent                 float64               `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	Logo                       string                `bson:"logo,omitempty" json:"logo,omitempty"`
	LogoContent                string                `json:"logo_content,omitempty"`
	NationalAddresss           NationalAddresss      `bson:"national_address,omitempty" json:"national_address,omitempty"`
	Deleted                    bool                  `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                  *primitive.ObjectID   `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser              *User                 `json:"deleted_by_user,omitempty"`
	DeletedAt                  *time.Time            `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                  *time.Time            `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                  *time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                  *primitive.ObjectID   `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                  *primitive.ObjectID   `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser              *User                 `json:"created_by_user,omitempty"`
	UpdatedByUser              *User                 `json:"updated_by_user,omitempty"`
	CreatedByName              string                `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName              string                `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName              string                `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	UseProductsFromStoreID     []*primitive.ObjectID `json:"use_products_from_store_id" bson:"use_products_from_store_id"`
	UseProductsFromStoreNames  []string              `json:"use_products_from_store_names" bson:"use_products_from_store_names"`
	ChangeLog                  []ChangeLog           `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func (store *Store) SetChangeLog(
	event string,
	name, oldValue, newValue interface{},
) {
	now := time.Now()
	description := ""
	if event == "create" {
		description = "Created by " + UserObject.Name
	} else if event == "update" {
		description = "Updated by " + UserObject.Name
	} else if event == "delete" {
		description = "Deleted by " + UserObject.Name
	} else if event == "view" {
		description = "Viewed by " + UserObject.Name
	} else if event == "attribute_value_change" && name != nil {
		description = name.(string) + " changed from " + oldValue.(string) + " to " + newValue.(string) + " by " + UserObject.Name
	}

	store.ChangeLog = append(
		store.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (store *Store) AttributesValueChangeEvent(storeOld *Store) error {

	if store.Name != storeOld.Name {
		usedInCollections := []string{
			"order",
			"purchase",
			"quotation",
		}

		for _, collectionName := range usedInCollections {
			err := UpdateManyByCollectionName(
				collectionName,
				bson.M{"store_id": store.ID},
				bson.M{"store_name": store.Name},
			)
			if err != nil {
				return nil
			}
		}
	}

	return nil
}

func (store *Store) UpdateForeignLabelFields() error {

	store.UseProductsFromStoreNames = []string{}

	for _, storeID := range store.UseProductsFromStoreID {
		storeTemp, err := FindStoreByID(storeID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding store id:" + storeID.Hex() + ",error:" + err.Error())
		}
		store.UseProductsFromStoreNames = append(store.UseProductsFromStoreNames, storeTemp.Name)
	}

	if store.CreatedBy != nil {
		createdByUser, err := FindUserByID(store.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		store.CreatedByName = createdByUser.Name
	}

	if store.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(store.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		store.UpdatedByName = updatedByUser.Name
	}

	if store.DeletedBy != nil && !store.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(store.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		store.DeletedByName = deletedByUser.Name
	}

	return nil
}

func SearchStore(w http.ResponseWriter, r *http.Request) (storees []Store, criterias SearchCriterias, err error) {

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

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return storees, criterias, err
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
			return storees, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return storees, criterias, err
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

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[registration_number]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["registration_number"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			store.CreatedByUser, _ = FindUserByID(store.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			store.UpdatedByUser, _ = FindUserByID(store.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			store.DeletedByUser, _ = FindUserByID(store.DeletedBy, deletedByUserSelectFields)
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
		exists, err := IsStoreExists(&store.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Store:" + store.ID.Hex()
		}

	}

	if len(store.UseProductsFromStoreID) == 0 {
		for i, storeID := range store.UseProductsFromStoreID {
			exists, err := IsStoreExists(storeID)
			if err != nil {
				errs["use_products_from_store_id_"+strconv.Itoa(i)] = err.Error()
			}

			if !exists {
				errs["use_products_from_store_id_"+strconv.Itoa(i)] = "Invalid store:" + storeID.Hex()
			}
		}

	}

	if govalidator.IsNull(store.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(store.Code) {
		errs["code"] = "Code is required"
	}

	if govalidator.IsNull(store.NameInArabic) {
		errs["name_in_arabic"] = "Name in Arabic is required"
	}

	if govalidator.IsNull(store.Name) {
		errs["registration_number"] = "Registration Number/C.R NO. is required"
	}

	if govalidator.IsNull(store.NameInArabic) {
		errs["registration_number_in_arabic"] = "Registration Number/C.R NO. in Arabic is required"
	}

	if govalidator.IsNull(store.Name) {
		errs["zipcode"] = "ZIP/PIN Code is required"
	}

	if govalidator.IsNull(store.NameInArabic) {
		errs["zipcode_in_arabic"] = "ZIP/PIN Code in Arabic is required"
	}

	if govalidator.IsNull(store.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(store.Address) {
		errs["address"] = "Address is required"
	}

	if govalidator.IsNull(store.AddressInArabic) {
		errs["address_in_arabic"] = "Address in Arabic is required"
	}

	if govalidator.IsNull(store.Phone) {
		errs["phone"] = "Phone is required"
	}

	if govalidator.IsNull(store.PhoneInArabic) {
		errs["phone_in_arabic"] = "Phone in Arabic is required"
	}

	if govalidator.IsNull(store.VATNo) {
		errs["vat_no"] = "VAT NO. is required"
	}

	if govalidator.IsNull(store.VATNoInArabic) {
		errs["vat_no_in_arabic"] = "VAT NO. is required"
	}

	if store.VatPercent == 0 {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if store.ID.IsZero() {
		if govalidator.IsNull(store.LogoContent) {
			errs["logo_content"] = "Logo is required"
		}
	}

	if !govalidator.IsNull(store.LogoContent) {
		splits := strings.Split(store.LogoContent, ",")

		if len(splits) == 2 {
			store.LogoContent = splits[1]
		} else if len(splits) == 1 {
			store.LogoContent = splits[0]
		}

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

	err := store.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	if !govalidator.IsNull(store.LogoContent) {
		err := store.SaveLogoFile()
		if err != nil {
			return err
		}
	}

	_, err = collection.InsertOne(ctx, &store)
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
	store.Logo = "/" + filename
	store.LogoContent = ""
	return nil
}

func (store *Store) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := store.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	if !govalidator.IsNull(store.LogoContent) {
		err := store.SaveLogoFile()
		if err != nil {
			return err
		}
	}
	store.LogoContent = ""

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

func (store *Store) DeleteStore(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = store.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	store.Deleted = true
	store.DeletedBy = &userID
	now := time.Now()
	store.DeletedAt = &now

	store.SetChangeLog("delete", nil, nil, nil)

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

func FindStoreByCode(
	Code string,
	selectFields map[string]interface{},
) (store *Store, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"code": Code}, findOneOptions).
		Decode(&store)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		store.CreatedByUser, _ = FindUserByID(store.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		store.UpdatedByUser, _ = FindUserByID(store.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		store.DeletedByUser, _ = FindUserByID(store.DeletedBy, fields)
	}

	return store, err
}

func FindStoreByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (store *Store, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&store)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		store.CreatedByUser, _ = FindUserByID(store.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		store.UpdatedByUser, _ = FindUserByID(store.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		store.DeletedByUser, _ = FindUserByID(store.DeletedBy, fields)
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

func IsStoreExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessStores() error {
	log.Printf("Processing stores")
	collection := db.Client().Database(db.GetPosDB()).Collection("store")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching products" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		store := Store{}
		err = cur.Decode(&store)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = store.Update()
		if err != nil {
			return err
		}
	}

	log.Print("DONE!")
	return nil
}
