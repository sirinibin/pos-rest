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

//Vendor : Vendor structure
type Vendor struct {
	ID                         primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name                       string              `bson:"name" json:"name"`
	NameInArabic               string              `bson:"name_in_arabic" json:"name_in_arabic"`
	Title                      string              `bson:"title" json:"title"`
	TitleInArabic              string              `bson:"title_in_arabic" json:"title_in_arabic"`
	Email                      string              `bson:"email,omitempty" json:"email"`
	Phone                      string              `bson:"phone,omitempty" json:"phone"`
	PhoneInArabic              string              `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic"`
	Address                    string              `bson:"address,omitempty" json:"address"`
	AddressInArabic            string              `bson:"address_in_arabic,omitempty" json:"address_in_arabic"`
	VATNo                      string              `bson:"vat_no,omitempty" json:"vat_no"`
	VATNoInArabic              string              `bson:"vat_no_in_arabic,omitempty" json:"vat_no_in_arabic"`
	VatPercent                 *float32            `bson:"vat_percent" json:"vat_percent"`
	RegistrationNumber         string              `bson:"registration_number,omitempty" json:"registration_number"`
	RegistrationNumberInArabic string              `bson:"registration_number_arabic,omitempty" json:"registration_number_in_arabic"`
	NationalAddresss           NationalAddresss    `bson:"national_address,omitempty" json:"national_address"`
	Logo                       string              `bson:"logo,omitempty" json:"logo"`
	LogoContent                string              `json:"logo_content,omitempty"`
	Deleted                    bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                  *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser              *User               `json:"deleted_by_user,omitempty"`
	DeletedAt                  *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                  *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                  *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                  *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                  *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser              *User               `json:"created_by_user,omitempty"`
	UpdatedByUser              *User               `json:"updated_by_user,omitempty"`
	CreatedByName              string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName              string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName              string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog                  []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
	SearchLabel                string              `json:"search_label"`
}

type NationalAddresss struct {
	ApplicationNo           string `bson:"application_no,omitempty" json:"application_no"`
	ApplicationNoArabic     string `bson:"application_no_arabic,omitempty" json:"application_no_arabic"`
	ServiceNo               string `bson:"service_no,omitempty" json:"service_no"`
	ServiceNoArabic         string `bson:"service_no_arabic,omitempty" json:"service_no_arabic"`
	CustomerAccountNo       string `bson:"customer_account_no,omitempty" json:"customer_account_no"`
	CustomerAccountNoArabic string `bson:"customer_account_no_arabic,omitempty" json:"customer_account_no_arabic"`
	BuildingNo              string `bson:"building_no,omitempty" json:"building_no"`
	BuildingNoArabic        string `bson:"building_no_arabic,omitempty" json:"building_no_arabic"`
	StreetName              string `bson:"street_name,omitempty" json:"street_name"`
	StreetNameArabic        string `bson:"street_name_arabic,omitempty" json:"street_name_arabic"`
	DistrictName            string `bson:"district_name,omitempty" json:"district_name"`
	DistrictNameArabic      string `bson:"district_name_arabic,omitempty" json:"district_name_arabic"`
	CityName                string `bson:"city_name,omitempty" json:"city_name"`
	CityNameArabic          string `bson:"city_name_arabic,omitempty" json:"city_name_arabic"`
	ZipCode                 string `bson:"zipcode,omitempty" json:"zipcode"`
	ZipCodeArabic           string `bson:"zipcode_arabic,omitempty" json:"zipcode_arabic"`
	AdditionalNo            string `bson:"additional_no,omitempty" json:"additional_no"`
	AdditionalNoArabic      string `bson:"additional_no_arabic,omitempty" json:"additional_no_arabic"`
	UnitNo                  string `bson:"unit_no,omitempty" json:"unit_no"`
	UnitNoArabic            string `bson:"unit_no_arabic,omitempty" json:"unit_no_arabic"`
}

func (vendor *Vendor) SetChangeLog(
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

	vendor.ChangeLog = append(
		vendor.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (vendor *Vendor) AttributesValueChangeEvent(vendorOld *Vendor) error {

	if vendor.Name != vendorOld.Name {
		usedInCollections := []string{
			"purchase",
		}

		for _, collectionName := range usedInCollections {
			err := UpdateManyByCollectionName(
				collectionName,
				bson.M{"vendor_id": vendor.ID},
				bson.M{"vendor_name": vendor.Name},
			)
			if err != nil {
				return nil
			}
		}
	}

	return nil
}

func (vendor *Vendor) UpdateForeignLabelFields() error {

	if vendor.CreatedBy != nil {
		createdByUser, err := FindUserByID(vendor.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		vendor.CreatedByName = createdByUser.Name
	}

	if vendor.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(vendor.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		vendor.UpdatedByName = updatedByUser.Name
	}

	if vendor.DeletedBy != nil && !vendor.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(vendor.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		vendor.DeletedByName = deletedByUser.Name
	}

	return nil
}

func SearchVendor(w http.ResponseWriter, r *http.Request) (vendors []Vendor, criterias SearchCriterias, err error) {

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
			return vendors, criterias, err
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
			return vendors, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return vendors, criterias, err
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
		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

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
		return vendors, criterias, errors.New("Error fetching vendores:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return vendors, criterias, errors.New("Cursor error:" + err.Error())
		}
		vendor := Vendor{}
		err = cur.Decode(&vendor)
		if err != nil {
			return vendors, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		vendor.SearchLabel = vendor.Name

		if vendor.NameInArabic != "" {
			vendor.SearchLabel += " / " + vendor.NameInArabic
		}

		if vendor.Phone != "" {
			vendor.SearchLabel += " " + vendor.Phone
		}

		if vendor.PhoneInArabic != "" {
			vendor.SearchLabel += " / " + vendor.PhoneInArabic
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			vendor.CreatedByUser, _ = FindUserByID(vendor.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			vendor.UpdatedByUser, _ = FindUserByID(vendor.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			vendor.DeletedByUser, _ = FindUserByID(vendor.DeletedBy, deletedByUserSelectFields)
		}

		vendors = append(vendors, vendor)
	} //end for loop

	return vendors, criterias, nil

}

func (vendor *Vendor) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if vendor.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsVendorExists(&vendor.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Vendor:" + vendor.ID.Hex()
		}
	}

	if govalidator.IsNull(vendor.Name) {
		errs["name"] = "Name is required"
	}

	/*
		if govalidator.IsNull(vendor.NameInArabic) {
			errs["name_in_arabic"] = "Name in Arabic is required"
		}
	*/

	/*
		if govalidator.IsNull(vendor.Email) {
			errs["email"] = "E-mail is required"
		}
	*/

	/*
		if govalidator.IsNull(vendor.Address) {
			errs["address"] = "Address is required"
		}
	*/

	/*
		if govalidator.IsNull(vendor.AddressInArabic) {
			errs["address_in_arabic"] = "Address in Arabic is required"
		}
	*/

	if govalidator.IsNull(vendor.Phone) {
		errs["phone"] = "Phone is required"
	}

	/*
		if govalidator.IsNull(vendor.PhoneInArabic) {
			errs["phone_in_arabic"] = "Phone in Arabic is required"
		}
	*/

	/*
		if vendor.VatPercent == nil {
			errs["vat_percent"] = "VAT Percentage is required"
		}
	*/

	/*
		if govalidator.IsNull(vendor.VATNo) {
			errs["vat_no"] = "VAT NO. is required"
		}
	*/

	/*
		if govalidator.IsNull(vendor.VATNoInArabic) {
			errs["vat_no_in_arabic"] = "VAT NO. is required"
		}
	*/

	/*
		if vendor.ID.IsZero() {
			if govalidator.IsNull(vendor.LogoContent) {
				//errs["logo_content"] = "Logo is required"
			}
		}
	*/

	if !govalidator.IsNull(vendor.LogoContent) {
		splits := strings.Split(vendor.LogoContent, ",")

		if len(splits) == 2 {
			vendor.LogoContent = splits[1]
		} else if len(splits) == 1 {
			vendor.LogoContent = splits[0]
		}

		valid, err := IsStringBase64(vendor.LogoContent)
		if err != nil {
			errs["logo_content"] = err.Error()
		}

		if !valid {
			errs["logo_content"] = "Invalid base64 string"
		}
	}

	if !govalidator.IsNull(vendor.Email) {
		emailExists, err := vendor.IsEmailExists()
		if err != nil {
			errs["email"] = err.Error()
		}

		if emailExists {
			errs["email"] = "E-mail is Already in use"
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (vendor *Vendor) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	vendor.ID = primitive.NewObjectID()

	err := vendor.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	if !govalidator.IsNull(vendor.LogoContent) {
		err := vendor.SaveLogoFile()
		if err != nil {
			return err
		}
	}

	vendor.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &vendor)
	if err != nil {
		return err
	}
	return nil
}

func (vendor *Vendor) SaveLogoFile() error {
	content, err := base64.StdEncoding.DecodeString(vendor.LogoContent)
	if err != nil {
		return err
	}

	extension, err := GetFileExtensionFromBase64(content)
	if err != nil {
		return err
	}

	filename := "images/vendor/logo_" + vendor.ID.Hex() + extension
	err = SaveBase64File(filename, content)
	if err != nil {
		return err
	}
	vendor.Logo = "/" + filename
	vendor.LogoContent = ""
	return nil
}

func (vendor *Vendor) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := vendor.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	if !govalidator.IsNull(vendor.LogoContent) {
		err := vendor.SaveLogoFile()
		if err != nil {
			return err
		}
	}
	vendor.LogoContent = ""

	vendor.SetChangeLog("update", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": vendor.ID},
		bson.M{"$set": vendor},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (vendor *Vendor) DeleteVendor(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = vendor.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	vendor.Deleted = true
	vendor.DeletedBy = &userID
	now := time.Now()
	vendor.DeletedAt = &now

	vendor.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": vendor.ID},
		bson.M{"$set": vendor},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindVendorByID(
	ID *primitive.ObjectID,
	selectFields bson.M,
) (vendor *Vendor, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if selectFields != nil {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&vendor)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		vendor.CreatedByUser, _ = FindUserByID(vendor.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		vendor.UpdatedByUser, _ = FindUserByID(vendor.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		vendor.DeletedByUser, _ = FindUserByID(vendor.DeletedBy, fields)
	}

	return vendor, err
}

func (vendor *Vendor) IsEmailExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if vendor.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": vendor.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": vendor.Email,
			"_id":   bson.M{"$ne": vendor.ID},
		})
	}

	return (count == 1), err
}

func IsVendorExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
