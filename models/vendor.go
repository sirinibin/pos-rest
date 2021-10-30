package models

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

//Vendor : Vendor structure
type Vendor struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name            string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic    string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	Title           string             `bson:"title,omitempty" json:"title,omitempty"`
	TitleInArabic   string             `bson:"title_in_arabic,omitempty" json:"title_in_arabic,omitempty"`
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

func SearchVendor(w http.ResponseWriter, r *http.Request) (vendores []Vendor, criterias SearchCriterias, err error) {

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

	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
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
		return vendores, criterias, errors.New("Error fetching vendores:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return vendores, criterias, errors.New("Cursor error:" + err.Error())
		}
		vendor := Vendor{}
		err = cur.Decode(&vendor)
		if err != nil {
			return vendores, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		vendores = append(vendores, vendor)
	} //end for loop

	return vendores, criterias, nil

}

func (vendor *Vendor) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if vendor.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsVendorExists(vendor.ID)
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

	if govalidator.IsNull(vendor.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(vendor.Address) {
		errs["address"] = "Address is required"
	}

	if govalidator.IsNull(vendor.Phone) {
		errs["phone"] = "Phone is required"
	}

	if vendor.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if vendor.ID.IsZero() {
		if govalidator.IsNull(vendor.LogoContent) {
			errs["logo_content"] = "Logo is required"
		}
	}

	if !govalidator.IsNull(vendor.LogoContent) {
		valid, err := IsStringBase64(vendor.LogoContent)
		if err != nil {
			errs["logo_content"] = err.Error()
		}

		if !valid {
			errs["logo_content"] = "Invalid base64 string"
		}
	}

	emailExists, err := vendor.IsEmailExists()
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

func (vendor *Vendor) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	vendor.ID = primitive.NewObjectID()

	if !govalidator.IsNull(vendor.LogoContent) {
		err := vendor.SaveLogoFile()
		if err != nil {
			return err
		}
	}

	_, err := collection.InsertOne(ctx, &vendor)
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

func (vendor *Vendor) Update() (*Vendor, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	if !govalidator.IsNull(vendor.LogoContent) {
		err := vendor.SaveLogoFile()
		if err != nil {
			return nil, err
		}
	}
	vendor.LogoContent = ""

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": vendor.ID},
		bson.M{"$set": vendor},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return vendor, nil
	}
	return nil, nil
}

func (vendor *Vendor) DeleteVendor(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	vendor.Deleted = true
	vendor.DeletedBy = userID
	vendor.DeletedAt = time.Now().Local()

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

func FindVendorByID(ID primitive.ObjectID) (vendor *Vendor, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}).
		Decode(&vendor)
	if err != nil {
		return nil, err
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

func IsVendorExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
