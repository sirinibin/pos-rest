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
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Warehouse : Warehouse structure
type Warehouse struct {
	ID              primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name            string              `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic    string              `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	Code            string              `bson:"code" json:"code"`
	Email           string              `bson:"email,omitempty" json:"email,omitempty"`
	Phone           string              `bson:"phone,omitempty" json:"phone,omitempty"`
	PhoneInArabic   string              `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic,omitempty"`
	Address         string              `bson:"address,omitempty" json:"address,omitempty"`
	AddressInArabic string              `bson:"address_in_arabic,omitempty" json:"address_in_arabic,omitempty"`
	ZipCode         string              `bson:"zipcode,omitempty" json:"zipcode,omitempty"`
	ZipCodeInArabic string              `bson:"zipcode_in_arabic,omitempty" json:"zipcode_in_arabic,omitempty"`
	CountryName     string              `bson:"country_name" json:"country_name"`
	CountryCode     string              `bson:"country_code" json:"country_code"`
	NationalAddress NationalAddress     `bson:"national_address,omitempty" json:"national_address,omitempty"`
	Deleted         bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy       *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser   *User               `json:"deleted_by_user,omitempty"`
	DeletedAt       *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt       *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy       *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser   *User               `json:"created_by_user,omitempty"`
	UpdatedByUser   *User               `json:"updated_by_user,omitempty"`
	CreatedByName   string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName   string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName   string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	StoreID         *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Store           *Store              `json:"store,omitempty"`
}

/*
func (warehouse *Warehouse) SetChangeLog(
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

	warehouse.ChangeLog = append(
		warehouse.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}
*/

func (warehouse *Warehouse) AttributesValueChangeEvent(warehouseOld *Warehouse) error {

	if warehouse.Name != warehouseOld.Name {
		/*
			usedInCollections := []string{
				"order",
				"purchase",
				"quotation",
			}

			for _, collectionName := range usedInCollections {
				err := warehouse.UpdateManyByCollectionName(
					collectionName,
					bson.M{"warehouse_id": warehouse.ID},
					bson.M{"warehouse_name": warehouse.Name},
				)
				if err != nil {
					return nil
				}
			}*/
	}

	return nil
}

func (warehouse *Warehouse) UpdateManyByCollectionName(
	collectionName string,
	filter bson.M,
	updateValues bson.M,
) error {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	defer cancel()

	_, err := collection.UpdateMany(
		ctx,
		filter,
		bson.M{"$set": updateValues},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (warehouse *Warehouse) UpdateForeignLabelFields() error {
	if warehouse.CreatedBy != nil {
		createdByUser, err := FindUserByID(warehouse.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		warehouse.CreatedByName = createdByUser.Name
	}

	if warehouse.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(warehouse.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		warehouse.UpdatedByName = updatedByUser.Name
	}

	if warehouse.DeletedBy != nil && !warehouse.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(warehouse.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		warehouse.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (store *Store) SearchWarehouse(w http.ResponseWriter, r *http.Request) (warehouses []Warehouse, criterias SearchCriterias, err error) {
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
			return warehouses, criterias, err
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
			return warehouses, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return warehouses, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
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
		return warehouses, criterias, errors.New("Error fetching warehouses:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return warehouses, criterias, errors.New("Cursor error:" + err.Error())
		}
		warehouse := Warehouse{}
		err = cur.Decode(&warehouse)
		if err != nil {
			return warehouses, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			warehouse.CreatedByUser, _ = FindUserByID(warehouse.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			warehouse.UpdatedByUser, _ = FindUserByID(warehouse.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			warehouse.DeletedByUser, _ = FindUserByID(warehouse.DeletedBy, deletedByUserSelectFields)
		}

		warehouses = append(warehouses, warehouse)
	} //end for loop

	return warehouses, criterias, nil

}

func (warehouse *Warehouse) TrimSpaceFromFields() {
	warehouse.Name = strings.TrimSpace(warehouse.Name)
	warehouse.NameInArabic = strings.TrimSpace(warehouse.NameInArabic)
	warehouse.Code = strings.TrimSpace(warehouse.Code)
	warehouse.ZipCode = strings.TrimSpace(warehouse.ZipCode)
	warehouse.Phone = strings.TrimSpace(warehouse.Phone)
	warehouse.Email = strings.TrimSpace(warehouse.Email)
	warehouse.Address = strings.TrimSpace(warehouse.Address)
	warehouse.AddressInArabic = strings.TrimSpace(warehouse.AddressInArabic)
	warehouse.NationalAddress.BuildingNo = strings.TrimSpace(warehouse.NationalAddress.BuildingNo)
	warehouse.NationalAddress.StreetName = strings.TrimSpace(warehouse.NationalAddress.StreetName)
	warehouse.NationalAddress.StreetNameArabic = strings.TrimSpace(warehouse.NationalAddress.StreetNameArabic)
	warehouse.NationalAddress.DistrictName = strings.TrimSpace(warehouse.NationalAddress.DistrictName)
	warehouse.NationalAddress.DistrictNameArabic = strings.TrimSpace(warehouse.NationalAddress.DistrictNameArabic)
	warehouse.NationalAddress.CityName = strings.TrimSpace(warehouse.NationalAddress.CityName)
	warehouse.NationalAddress.CityNameArabic = strings.TrimSpace(warehouse.NationalAddress.CityNameArabic)
	warehouse.NationalAddress.ZipCode = strings.TrimSpace(warehouse.NationalAddress.ZipCode)
	warehouse.NationalAddress.AdditionalNo = strings.TrimSpace(warehouse.NationalAddress.AdditionalNo)
	warehouse.NationalAddress.UnitNo = strings.TrimSpace(warehouse.NationalAddress.UnitNo)
}

func (warehouse *Warehouse) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	warehouse.TrimSpaceFromFields()
	errs = make(map[string]string)

	store, err := FindStoreByID(warehouse.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	/*oldWarehouse, err := FindWarehouseByID(&warehouse.ID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		w.WriteHeader(http.StatusBadRequest)
		errs["id"] = err.Error()
		return errs
	}*/

	if scenario == "update" {
		if warehouse.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsWarehouseExists(&warehouse.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Warehouse:" + warehouse.ID.Hex()
		}
	}

	if govalidator.IsNull(warehouse.Name) {
		errs["name"] = "Name is required"
	}

	/*
		if govalidator.IsNull(warehouse.CountryCode) {
			errs["country_code"] = "Country is required"
		}*/

	/*
			if govalidator.IsNull(warehouse.NameInArabic) {
				errs["name_in_arabic"] = "Name in Arabic is required"
			}

		if govalidator.IsNull(warehouse.NameInArabic) {
			errs["registration_number_in_arabic"] = "Registration Number/C.R NO. in Arabic is required"
		}*/

	/*
		if govalidator.IsNull(warehouse.ZipCode) {
			errs["zipcode"] = "Zipcode is required"
		} else if !IsValidDigitNumber(warehouse.NationalAddress.ZipCode, "5") {
			errs["zipcode"] = "Zipcode should be 5 digits"
		}*/

	if !govalidator.IsNull(warehouse.NationalAddress.ZipCode) && !IsValidDigitNumber(warehouse.NationalAddress.ZipCode, "5") {
		errs["zipcode"] = "Zipcode should be 5 digits"
	}

	/*
		if govalidator.IsNull(warehouse.NameInArabic) {
			errs["zipcode_in_arabic"] = "ZIP/PIN Code in Arabic is required"
		}

		if govalidator.IsNull(warehouse.Email) {
			errs["email"] = "E-mail is required"
		}

		if govalidator.IsNull(warehouse.Address) {
			errs["address"] = "Address is required"
		}

		if govalidator.IsNull(warehouse.AddressInArabic) {
			errs["address_in_arabic"] = "Address in Arabic is required"
		}*/

	/*
		if govalidator.IsNull(warehouse.Phone) {
			errs["phone"] = "Phone is required"
		} else if !ValidateSaudiPhone(warehouse.Phone) {
			errs["phone"] = "Invalid phone no."
		}*/

	if !govalidator.IsNull(warehouse.Phone) && !ValidateSaudiPhone(warehouse.Phone) {
		errs["phone"] = "Invalid phone no."
	}

	/*
		if govalidator.IsNull(warehouse.PhoneInArabic) {
			errs["phone_in_arabic"] = "Phone in Arabic is required"
		}*/

	//National address
	/*
		if govalidator.IsNull(warehouse.NationalAddress.BuildingNo) {
			errs["national_address_building_no"] = "Building number is required"
		} else {
			if !IsValidDigitNumber(warehouse.NationalAddress.BuildingNo, "4") {
				errs["national_address_building_no"] = "Building number should be 4 digits"
			}
		}*/

	if !govalidator.IsNull(warehouse.NationalAddress.BuildingNo) && !IsValidDigitNumber(warehouse.NationalAddress.BuildingNo, "4") {
		errs["national_address_building_no"] = "Building number should be 4 digits"
	}

	/*
		if govalidator.IsNull(warehouse.NationalAddress.StreetName) {
			errs["national_address_street_name"] = "Street name is required"
		}

		if govalidator.IsNull(warehouse.NationalAddress.StreetNameArabic) {
			errs["national_address_street_name_arabic"] = "Street name in arabic is required"
		}

		if govalidator.IsNull(warehouse.NationalAddress.DistrictName) {
			errs["national_address_district_name"] = "District name is required"
		}

		if govalidator.IsNull(warehouse.NationalAddress.DistrictNameArabic) {
			errs["national_address_district_name_arabic"] = "District name in arabic is required"
		}

		if govalidator.IsNull(warehouse.NationalAddress.CityName) {
			errs["national_address_city_name"] = "City name is required"
		}

		if govalidator.IsNull(warehouse.NationalAddress.CityNameArabic) {
			errs["national_address_city_name_arabic"] = "City name in arabic is required"
		}*/

	/*
		if govalidator.IsNull(warehouse.NationalAddress.ZipCode) {
			errs["national_address_zipcode"] = "Zip code is required"
		} else if !IsValidDigitNumber(warehouse.NationalAddress.ZipCode, "5") {
			errs["national_address_zipcode"] = "Zip code should be 5 digits"
		}*/

	if !govalidator.IsNull(warehouse.NationalAddress.ZipCode) && !IsValidDigitNumber(warehouse.NationalAddress.ZipCode, "5") {
		errs["national_address_zipcode"] = "Zip code should be 5 digits"
	}

	/*
		emailExists, err := warehouse.IsEmailExists()
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
	*/

	return errs
}

func (warehouse *Warehouse) Insert() error {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	warehouse.ID = primitive.NewObjectID()

	err := warehouse.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.InsertOne(ctx, &warehouse)
	if err != nil {
		return err
	}
	return nil
}

func (warehouse *Warehouse) Update() error {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := warehouse.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": warehouse.ID},
		bson.M{"$set": warehouse},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (warehouse *Warehouse) DeleteWarehouse(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = warehouse.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	warehouse.Deleted = true
	warehouse.DeletedBy = &userID
	now := time.Now()
	warehouse.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": warehouse.ID},
		bson.M{"$set": warehouse},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindWarehouseByCode(
	Code string,
	selectFields map[string]interface{},
) (warehouse *Warehouse, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"code": Code}, findOneOptions).
		Decode(&warehouse)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		warehouse.CreatedByUser, _ = FindUserByID(warehouse.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		warehouse.UpdatedByUser, _ = FindUserByID(warehouse.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		warehouse.DeletedByUser, _ = FindUserByID(warehouse.DeletedBy, fields)
	}

	return warehouse, err
}

func (store *Store) FindWarehouseByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (warehouse *Warehouse, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&warehouse)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		warehouse.CreatedByUser, _ = FindUserByID(warehouse.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		warehouse.UpdatedByUser, _ = FindUserByID(warehouse.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		warehouse.DeletedByUser, _ = FindUserByID(warehouse.DeletedBy, fields)
	}

	return warehouse, err
}

func (warehouse *Warehouse) IsEmailExists() (exists bool, err error) {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if warehouse.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": warehouse.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": warehouse.Email,
			"_id":   bson.M{"$ne": warehouse.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) IsWarehouseExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (warehouse *Warehouse) IsWarehouseExistsByCode() (exists bool, err error) {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if warehouse.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": warehouse.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": warehouse.Code,
			"_id":  bson.M{"$ne": warehouse.ID},
		})
	}

	return (count > 0), err
}

func ProcessWarehouses() error {
	log.Printf("Processing warehouses")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
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
			warehouse := Warehouse{}
			err = cur.Decode(&warehouse)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			/*if warehouse.Code == "LGK-SIMULATION" || warehouse.Code == "LGK" || warehouse.Code == "LGK-TEST" || warehouse.Code == "PH2" {
				//warehouse.ImportProductsFromExcel("xl/ALL_ITEAM_AND_PRICE.xlsx")
				warehouse.UpdateProductStockFromExcel("xl/STOCK.xlsx")
				//warehouse.ImportProductCategoriesFromExcel("xl/CategoryDateList.xlsx")
				//warehouse.ImportCustomersFromExcel("xl/CUSTOMER_LIST.xlsx")
				//warehouse.ImportVendorsFromExcel("xl/SuppLIERList03-06-2025.csv.xlsx")

			} else {
				continue
			}*/

			/*
				_, err = warehouse.CreateDB()
				if err != nil {
					return err
				}*/
			/*
				err = warehouse.Update()
				if err != nil {
					return err
				}
			*/
		}
	}

	log.Print("DONE!")
	return nil
}

func (store *Store) GetAllWarehouses() (warehouses []Warehouse, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSort(map[string]interface{}{
		"created_at": 1,
	})
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return warehouses, errors.New("Error fetching products" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return warehouses, errors.New("Cursor error:" + err.Error())
		}
		warehouse := Warehouse{}
		err = cur.Decode(&warehouse)
		if err != nil {
			return warehouses, errors.New("Cursor decode error:" + err.Error())
		}

		warehouses = append(warehouses, warehouse)

	}

	return warehouses, nil
}

func (warehouse *Warehouse) MakeCode() error {
	store, err := FindStoreByID(warehouse.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := warehouse.StoreID.Hex() + "_warehouse_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetWarehouseCount()
		if err != nil {
			return err
		}

		startFrom := int64(1)

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

	paddingCount := int64(1)
	warehouse.Code = fmt.Sprintf("%s%0*d", "WH", paddingCount, incr)
	return nil
}

func (warehouse *Warehouse) UnMakeCode() error {
	// Global counter key
	redisKey := warehouse.StoreID.Hex() + "_warehouse_counter"

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	return nil
}

func (store *Store) GetWarehouseCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("warehouse")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}
