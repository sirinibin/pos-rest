package models

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
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

// Store : Store structure
type Store struct {
	ID                         primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name                       string                `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic               string                `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	Code                       string                `bson:"code" json:"code"`
	BranchName                 string                `bson:"branch_name" json:"branch_name"`
	BusinessCategory           string                `bson:"business_category" json:"business_category"`
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
	NationalAddress            NationalAddress       `bson:"national_address,omitempty" json:"national_address,omitempty"`
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
	Zatca                      Zatca                 `bson:"zatca,omitempty" json:"zatca,omitempty"`
	SalesSerialNumber          SerialNumber          `bson:"sales_serial_number,omitempty" json:"sales_serial_number,omitempty"`
	SalesReturnSerialNumber    SerialNumber          `bson:"sales_return_serial_number,omitempty" json:"sales_return_serial_number,omitempty"`
	PurchaseSerialNumber       SerialNumber          `bson:"purchase_serial_number,omitempty" json:"purchase_serial_number,omitempty"`
	PurchaseReturnSerialNumber SerialNumber          `bson:"purchase_return_serial_number,omitempty" json:"purchase_return_serial_number,omitempty"`
	QuotationSerialNumber      SerialNumber          `bson:"quotation_serial_number,omitempty" json:"quotation_serial_number,omitempty"`
	BankAccount                BankAccount           `bson:"bank_account,omitempty" json:"bank_account,omitempty"`
	CustomerSerialNumber       SerialNumber          `bson:"customer_serial_number,omitempty" json:"customer_serial_number,omitempty"`
	VendorSerialNumber         SerialNumber          `bson:"vendor_serial_number,omitempty" json:"vendor_serial_number,omitempty"`
}

type SerialNumber struct {
	Prefix         string `bson:"prefix,omitempty" json:"prefix"` //1 or 2
	StartFromCount int64  `bson:"start_from_count,omitempty" json:"start_from_count"`
	PaddingCount   int64  `bson:"padding_count,omitempty" json:"padding_count"`
}

type Zatca struct {
	Phase                         string              `bson:"phase,omitempty" json:"phase"` //1 or 2
	Env                           string              `bson:"env,omitempty" json:"env"`     //NonProduction | Simulation | Production
	Otp                           string              `bson:"otp,omitempty" json:"otp"`     //Need to obtain from zatca when going to production level
	PrivateKey                    string              `bson:"private_key,omitempty" json:"private_key"`
	Csr                           string              `bson:"csr,omitempty" json:"csr"` //Need to generate from store details, update it whenever the store details updates
	ComplianceRequestID           int64               `bson:"compliance_request_id,omitempty" json:"compliance_request_id"`
	BinarySecurityToken           string              `bson:"binary_security_token,omitempty" json:"binary_security_token"`
	Secret                        string              `bson:"secret,omitempty" json:"secret"`
	ProductionRequestID           int64               `bson:"production_request_id,omitempty" json:"production_request_id"`
	ProductionBinarySecurityToken string              `bson:"production_binary_security_token,omitempty" json:"production_binary_security_token"`
	ProductionSecret              string              `bson:"production_secret,omitempty" json:"production_secret"`
	Connected                     bool                `bson:"connected,omitempty" json:"connected,omitempty"`
	LastConnectedAt               *time.Time          `bson:"last_connected_at,omitempty" json:"last_connected_at,omitempty"`
	ConnectedBy                   *primitive.ObjectID `json:"connected_by,omitempty" bson:"connected_by,omitempty"`
	DisconnectedBy                *primitive.ObjectID `json:"disconnected_by,omitempty" bson:"disconnected_by,omitempty"`
	LastDisconnectedAt            *time.Time          `bson:"last_disconnected_at,omitempty" json:"last_disconnected_at,omitempty"`
	ConnectionFailedCount         int64               `bson:"connection_failed_count,omitempty" json:"connection_failed_count,omitempty"`
	ConnectionErrors              []string            `bson:"connection_errors,omitempty" json:"connection_errors,omitempty"`
	ConnectionLastFailedAt        *time.Time          `bson:"connection_last_failed_at,omitempty" json:"connection_last_failed_at,omitempty"`
}

/*
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
*/

func (store *Store) AttributesValueChangeEvent(storeOld *Store) error {

	if store.Name != storeOld.Name {
		usedInCollections := []string{
			"order",
			"purchase",
			"quotation",
		}

		for _, collectionName := range usedInCollections {
			err := store.UpdateManyByCollectionName(
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

func SearchStore(w http.ResponseWriter, r *http.Request) (stores []Store, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	tokenClaims, err := AuthenticateByAccessToken(r)
	if err != nil {
		return stores, criterias, err
	}

	accessingUserID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
	}

	accessingUser, err := FindUserByID(&accessingUserID, bson.M{})
	if err != nil {
		return stores, criterias, err
	}

	if accessingUser.Role != "Admin" {
		/*
			criterias.SearchBy["$or"] = []bson.M{
				{"store_id": storeID},
				{"store_id": bson.M{"$in": store.UseProductsFromStoreID}},
			}
		*/
		if len(accessingUser.StoreIDs) > 0 {
			criterias.SearchBy["_id"] = bson.M{"$in": accessingUser.StoreIDs}
		}
	}

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
			return stores, criterias, err
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
			return stores, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return stores, criterias, err
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

	keys, ok = r.URL.Query()["search[branch_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["branch_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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
		return stores, criterias, errors.New("Error fetching stores:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return stores, criterias, errors.New("Cursor error:" + err.Error())
		}
		store := Store{}
		err = cur.Decode(&store)
		if err != nil {
			return stores, criterias, errors.New("Cursor decode error:" + err.Error())
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

		stores = append(stores, store)
	} //end for loop

	return stores, criterias, nil

}

func IsNumberStartAndEndWith(num string, startEnd string) bool {
	// Create a dynamic regex pattern using the provided digit
	pattern := fmt.Sprintf(`^%s\d*%s$`, startEnd, startEnd)
	re := regexp.MustCompile(pattern)
	return re.MatchString(num)
}

func IsAlphanumeric(s string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9]+$`) // Only allows letters (a-z, A-Z) and numbers (0-9)
	return re.MatchString(s)
}

// ValidateSaudiPhone checks if a phone number is a valid Saudi number
func ValidateSaudiPhone(phone string) bool {
	// Regular expression for Saudi phone numbers
	re := regexp.MustCompile(`^(?:\+966|0)5\d{8}$`)

	return re.MatchString(phone)
}

func (store *Store) TrimSpaceFromFields() {
	store.BusinessCategory = strings.TrimSpace(store.BusinessCategory)
	store.Name = strings.TrimSpace(store.Name)
	store.NameInArabic = strings.TrimSpace(store.NameInArabic)
	store.Code = strings.TrimSpace(store.Code)
	store.BranchName = strings.TrimSpace(store.BranchName)
	store.Title = strings.TrimSpace(store.Title)
	store.TitleInArabic = strings.TrimSpace(store.TitleInArabic)
	store.RegistrationNumber = strings.TrimSpace(store.RegistrationNumber)
	store.ZipCode = strings.TrimSpace(store.ZipCode)
	store.Phone = strings.TrimSpace(store.Phone)
	store.VATNo = strings.TrimSpace(store.VATNo)
	store.Email = strings.TrimSpace(store.Email)
	store.Address = strings.TrimSpace(store.Address)
	store.AddressInArabic = strings.TrimSpace(store.AddressInArabic)
	store.NationalAddress.BuildingNo = strings.TrimSpace(store.NationalAddress.BuildingNo)
	store.NationalAddress.StreetName = strings.TrimSpace(store.NationalAddress.StreetName)
	store.NationalAddress.StreetNameArabic = strings.TrimSpace(store.NationalAddress.StreetNameArabic)
	store.NationalAddress.DistrictName = strings.TrimSpace(store.NationalAddress.DistrictName)
	store.NationalAddress.DistrictNameArabic = strings.TrimSpace(store.NationalAddress.DistrictNameArabic)
	store.NationalAddress.CityName = strings.TrimSpace(store.NationalAddress.CityName)
	store.NationalAddress.CityNameArabic = strings.TrimSpace(store.NationalAddress.CityNameArabic)
	store.NationalAddress.ZipCode = strings.TrimSpace(store.NationalAddress.ZipCode)
	store.NationalAddress.AdditionalNo = strings.TrimSpace(store.NationalAddress.AdditionalNo)
	store.NationalAddress.UnitNo = strings.TrimSpace(store.NationalAddress.UnitNo)
	store.SalesSerialNumber.Prefix = strings.TrimSpace(store.SalesSerialNumber.Prefix)
	store.SalesReturnSerialNumber.Prefix = strings.TrimSpace(store.SalesReturnSerialNumber.Prefix)
	store.PurchaseSerialNumber.Prefix = strings.TrimSpace(store.PurchaseSerialNumber.Prefix)
	store.PurchaseReturnSerialNumber.Prefix = strings.TrimSpace(store.PurchaseReturnSerialNumber.Prefix)
	store.QuotationSerialNumber.Prefix = strings.TrimSpace(store.QuotationSerialNumber.Prefix)
	store.CustomerSerialNumber.Prefix = strings.TrimSpace(store.CustomerSerialNumber.Prefix)
	store.VendorSerialNumber.Prefix = strings.TrimSpace(store.VendorSerialNumber.Prefix)
}

func (store *Store) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	store.TrimSpaceFromFields()
	errs = make(map[string]string)

	oldStore, err := FindStoreByID(&store.ID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		w.WriteHeader(http.StatusBadRequest)
		errs["id"] = err.Error()
		return errs
	}

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
		errs["code"] = "Branch code is required"
	}

	if govalidator.IsNull(store.BranchName) {
		errs["branch_name"] = "Branch name is required"
	}

	if govalidator.IsNull(store.NameInArabic) {
		errs["name_in_arabic"] = "Name in Arabic is required"
	}

	if govalidator.IsNull(store.RegistrationNumber) {
		errs["registration_number"] = "Registration Number / CRN is required"
	} else if !IsAlphanumeric(store.RegistrationNumber) {
		errs["registration_number"] = "Registration Number should be alpha numeric(a-zA-Z|0-9)"
	}

	if govalidator.IsNull(store.NameInArabic) {
		errs["registration_number_in_arabic"] = "Registration Number/C.R NO. in Arabic is required"
	}

	if govalidator.IsNull(store.ZipCode) {
		errs["zipcode"] = "Zipcode is required"
	} else if !IsValidDigitNumber(store.NationalAddress.ZipCode, "5") {
		errs["zipcode"] = "Zipcode should be 5 digits"
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
	} else if !ValidateSaudiPhone(store.Phone) {
		errs["phone"] = "Invalid phone no."
	}

	if govalidator.IsNull(store.PhoneInArabic) {
		errs["phone_in_arabic"] = "Phone in Arabic is required"
	}

	if govalidator.IsNull(store.VATNo) {
		errs["vat_no"] = "VAT NO. is required"
	} else if !IsValidDigitNumber(store.VATNo, "15") {
		errs["vat_no"] = "VAT No. should be 15 digits"
	} else if !IsNumberStartAndEndWith(store.VATNo, "3") {
		errs["vat_no"] = "VAT No. should start and end with 3"
	}

	if govalidator.IsNull(store.VATNoInArabic) {
		errs["vat_no_in_arabic"] = "VAT NO. is required"
	}

	if store.VatPercent == 0 {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if govalidator.IsNull(store.BusinessCategory) {
		errs["business_category"] = "Business category is required"
	}

	//National address
	if govalidator.IsNull(store.NationalAddress.BuildingNo) {
		errs["national_address_building_no"] = "Building number is required"
	} else {
		if !IsValidDigitNumber(store.NationalAddress.BuildingNo, "4") {
			errs["national_address_building_no"] = "Building number should be 4 digits"
		}
	}

	if govalidator.IsNull(store.NationalAddress.StreetName) {
		errs["national_address_street_name"] = "Street name is required"
	}

	if govalidator.IsNull(store.NationalAddress.StreetNameArabic) {
		errs["national_address_street_name_arabic"] = "Street name in arabic is required"
	}

	if govalidator.IsNull(store.NationalAddress.DistrictName) {
		errs["national_address_district_name"] = "District name is required"
	}

	if govalidator.IsNull(store.NationalAddress.DistrictNameArabic) {
		errs["national_address_district_name_arabic"] = "District name in arabic is required"
	}

	if govalidator.IsNull(store.NationalAddress.CityName) {
		errs["national_address_city_name"] = "City name is required"
	}

	if govalidator.IsNull(store.NationalAddress.CityNameArabic) {
		errs["national_address_city_name_arabic"] = "City name in arabic is required"
	}

	if govalidator.IsNull(store.NationalAddress.ZipCode) {
		errs["national_address_zipcode"] = "Zip code is required"
	} else if !IsValidDigitNumber(store.NationalAddress.ZipCode, "5") {
		errs["national_address_zipcode"] = "Zip code should be 5 digits"
	}

	//sales serial number
	if govalidator.IsNull(store.SalesSerialNumber.Prefix) {
		errs["sales_serial_number_prefix"] = "Prefix is required"
	}

	if store.SalesSerialNumber.PaddingCount <= 0 {
		errs["sales_serial_number_padding_count"] = "Padding count is required"
	}

	if store.SalesSerialNumber.StartFromCount < 0 {
		errs["sales_serial_number_start_from_count"] = "Counting start from, is required"
	}

	//sales return serial number
	if govalidator.IsNull(store.SalesReturnSerialNumber.Prefix) {
		errs["sales_return_serial_number_prefix"] = "Prefix is required"
	}

	if store.SalesReturnSerialNumber.PaddingCount <= 0 {
		errs["sales_return_serial_number_padding_count"] = "Padding count is required"
	}

	if store.SalesReturnSerialNumber.StartFromCount < 0 {
		errs["sales_return_serial_number_start_from_count"] = "Counting start from, is required"
	}

	//purchase serial number
	if govalidator.IsNull(store.PurchaseSerialNumber.Prefix) {
		errs["purchase_serial_number_prefix"] = "Prefix is required"
	}

	if store.SalesReturnSerialNumber.PaddingCount <= 0 {
		errs["purchase_serial_number_padding_count"] = "Padding count is required"
	}

	if store.SalesReturnSerialNumber.StartFromCount < 0 {
		errs["purchase_serial_number_start_from_count"] = "Counting start from, is required"
	}

	//purchase return serial number
	if govalidator.IsNull(store.PurchaseReturnSerialNumber.Prefix) {
		errs["purchase_return_serial_number_prefix"] = "Prefix is required"
	}

	if store.PurchaseReturnSerialNumber.PaddingCount <= 0 {
		errs["purchase_return_serial_number_padding_count"] = "Padding count is required"
	}

	if store.PurchaseReturnSerialNumber.StartFromCount < 0 {
		errs["purchase_return_serial_number_start_from_count"] = "Counting start from, is required"
	}

	//quotation return serial number
	if govalidator.IsNull(store.QuotationSerialNumber.Prefix) {
		errs["quotation_serial_number_prefix"] = "Prefix is required"
	}

	if store.QuotationSerialNumber.PaddingCount <= 0 {
		errs["quotation_serial_number_padding_count"] = "Padding count is required"
	}

	if store.QuotationSerialNumber.StartFromCount < 0 {
		errs["quotation_serial_number_start_from_count"] = "Counting start from, is required"
	}

	//customer serial number
	/*
		if govalidator.IsNull(store.CustomerSerialNumber.Prefix) {
			errs["customer_serial_number_prefix"] = "Prefix is required"
		}

		if store.CustomerSerialNumber.PaddingCount <= 0 {
			errs["customer_serial_number_padding_count"] = "Padding count is required"
		}

		if store.CustomerSerialNumber.StartFromCount < 0 {
			errs["customer_serial_number_start_from_count"] = "Counting start from, is required"
		}

		//vendor serial number
		if govalidator.IsNull(store.VendorSerialNumber.Prefix) {
			errs["vendor_serial_number_prefix"] = "Prefix is required"
		}

		if store.VendorSerialNumber.PaddingCount <= 0 {
			errs["vendor_serial_number_padding_count"] = "Padding count is required"
		}

		if store.VendorSerialNumber.StartFromCount < 0 {
			errs["vendor_serial_number_start_from_count"] = "Counting start from, is required"
		}
	*/

	if store.Zatca.Phase == "2" {
		if govalidator.IsNull(store.Zatca.Env) {
			errs["zatca_env"] = "Environment is required"
		}
	}

	if !store.ID.IsZero() && oldStore != nil && !govalidator.IsNull(oldStore.Zatca.Env) {
		if store.Zatca.Env != oldStore.Zatca.Env {
			salesCount, err := oldStore.GetSalesCount()
			if err != nil {
				errs["sales_count"] = "Error finding sales count"
			}

			if salesCount > 0 {
				errs["zatca_env"] = "You cannot change this as you have already created " + strconv.FormatInt(salesCount, 10) + " sales"
			}
		}
	}

	if !store.ID.IsZero() && oldStore != nil {
		if store.SalesSerialNumber.StartFromCount != oldStore.SalesSerialNumber.StartFromCount {
			salesCount, err := oldStore.GetSalesCount()
			if err != nil {
				errs["sales_serial_number_start_from_count"] = "Error finding sales count"
			}

			if salesCount > 0 {
				errs["sales_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(salesCount, 10) + " sales"
			}
		}

		if store.SalesReturnSerialNumber.StartFromCount != oldStore.SalesReturnSerialNumber.StartFromCount {
			salesReturnCount, err := oldStore.GetSalesReturnCount()
			if err != nil {
				errs["sales_return_serial_number_start_from_count"] = "Error finding sales return count"
			}

			if salesReturnCount > 0 {
				errs["sales_return_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(salesReturnCount, 10) + " sales returns"
			}
		}

		if store.PurchaseSerialNumber.StartFromCount != oldStore.PurchaseSerialNumber.StartFromCount {
			purchaseCount, err := oldStore.GetPurchaseCount()
			if err != nil {
				errs["purchase_serial_number_start_from_count"] = "Error finding purchase count"
			}

			if purchaseCount > 0 {
				errs["purchase_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(purchaseCount, 10) + " purchase"
			}
		}

		if store.PurchaseReturnSerialNumber.StartFromCount != oldStore.PurchaseReturnSerialNumber.StartFromCount {
			purchaseReturnCount, err := oldStore.GetPurchaseReturnCount()
			if err != nil {
				errs["purchase_return_serial_number_start_from_count"] = "Error finding purchase return count"
			}

			if purchaseReturnCount > 0 {
				errs["purchase_return_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(purchaseReturnCount, 10) + " purchase return"
			}
		}

		if store.QuotationSerialNumber.StartFromCount != oldStore.QuotationSerialNumber.StartFromCount {
			quotationCount, err := oldStore.GetQuotationCount()
			if err != nil {
				errs["quotation_serial_number_start_from_count"] = "Error finding quotation count"
			}

			if quotationCount > 0 {
				errs["quotation_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(quotationCount, 10) + " quotations"
			}
		}

		/*
			if store.CustomerSerialNumber.StartFromCount != oldStore.CustomerSerialNumber.StartFromCount {
				customerCount, err := oldStore.GetCustomerCount()
				if err != nil {
					errs["customer_serial_number_start_from_count"] = "Error finding customer count"
				}

				if customerCount > 0 {
					errs["customer_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(customerCount, 10) + " customers"
				}
			}

			if store.VendorSerialNumber.StartFromCount != oldStore.VendorSerialNumber.StartFromCount {
				vendorCount, err := oldStore.GetVendorCount()
				if err != nil {
					errs["vendor_serial_number_start_from_count"] = "Error finding vendor count"
				}

				if vendorCount > 0 {
					errs["vendor_serial_number_start_from_count"] = "You cannot change this as you have already created " + strconv.FormatInt(vendorCount, 10) + " vendors"
				}
			}*/
	}

	/*
		if store.ID.IsZero() {
			if govalidator.IsNull(store.LogoContent) {
				errs["logo_content"] = "Logo is required"
			}
		}*/

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
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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

	return (count > 0), err
}

func IsStoreExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func ProcessStores() error {
	log.Printf("Processing stores")
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
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

		_, err = store.CreateDB()
		if err != nil {
			return err
		}
		/*
			err = store.Update()
			if err != nil {
				return err
			}
		*/
	}

	log.Print("DONE!")
	return nil
}

func GetAllStores() (stores []Store, err error) {
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return stores, errors.New("Error fetching products" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return stores, errors.New("Cursor error:" + err.Error())
		}
		store := Store{}
		err = cur.Decode(&store)
		if err != nil {
			return stores, errors.New("Cursor decode error:" + err.Error())
		}

		stores = append(stores, store)

	}

	return stores, nil
}

func (store *Store) GetSalesReturnCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (store *Store) GetPurchaseCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (store *Store) GetPurchaseReturnCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (store *Store) GetQuotationCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

// Function to create a new DB for a store
func (store *Store) CreateDB() (*mongo.Database, error) {
	// Naming the database dynamically based on storeID
	dbName := "store_" + store.ID.Hex()
	storeDB := db.GetDB("store_" + store.ID.Hex())
	fmt.Println("✅ Database created for store:", dbName)
	return storeDB, nil
}
