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
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type VendorStore struct {
	StoreID                          primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName                        string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic                string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	PurchaseCount                    int64              `bson:"purchase_count" json:"purchase_count"`
	PurchaseAmount                   float64            `bson:"purchase_amount" json:"purchase_amount"`
	PurchasePaidAmount               float64            `bson:"purchase_paid_amount" json:"purchase_paid_amount"`
	PurchaseBalanceAmount            float64            `bson:"purchase_balance_amount" json:"purchase_balance_amount"`
	PurchaseRetailProfit             float64            `bson:"purchase_retail_profit" json:"purchase_retail_profit"`
	PurchaseRetailLoss               float64            `bson:"purchase_retail_loss" json:"purchase_retail_loss"`
	PurchaseWholesaleProfit          float64            `bson:"purchase_wholesale_profit" json:"purchase_wholesale_profit"`
	PurchaseWholesaleLoss            float64            `bson:"purchase_wholesale_loss" json:"purchase_wholesale_loss"`
	PurchasePaidCount                int64              `bson:"purchase_paid_count" json:"purchase_paid_count"`
	PurchaseNotPaidCount             int64              `bson:"purchase_not_paid_count" json:"purchase_not_paid_count"`
	PurchasePaidPartiallyCount       int64              `bson:"purchase_paid_partially_count" json:"purchase_paid_partially_count"`
	PurchaseReturnCount              int64              `bson:"purchase_return_count" json:"purchase_return_count"`
	PurchaseReturnAmount             float64            `bson:"purchase_return_amount" json:"purchase_return_amount"`
	PurchaseReturnPaidAmount         float64            `bson:"purchase_return_paid_amount" json:"purchase_return_paid_amount"`
	PurchaseReturnBalanceAmount      float64            `bson:"purchase_return_balance_amount" json:"purchase_return_balance_amount"`
	PurchaseReturnPaidCount          int64              `bson:"purchase_return_paid_count" json:"purchase_return_paid_count"`
	PurchaseReturnNotPaidCount       int64              `bson:"purchase_return_not_paid_count" json:"purchase_return_not_paid_count"`
	PurchaseReturnPaidPartiallyCount int64              `bson:"purchase_return_paid_partially_count" json:"purchase_return_paid_partially_count"`
}

// Vendor : Vendor structure
type Vendor struct {
	ID                         primitive.ObjectID     `json:"id,omitempty" bson:"_id,omitempty"`
	Code                       string                 `bson:"code,omitempty" json:"code,omitempty"`
	Name                       string                 `bson:"name" json:"name"`
	NameInArabic               string                 `bson:"name_in_arabic" json:"name_in_arabic"`
	SearchWords                []string               `bson:"search_words,omitempty" json:"search_words,omitempty"`
	SearchWordsInArabic        []string               `bson:"search_words_in_arabic,omitempty" json:"search_words_in_arabic,omitempty"`
	AdditionalKeywords         []string               `bson:"additional_keywords" json:"additional_keywords"`
	Title                      string                 `bson:"title" json:"title"`
	TitleInArabic              string                 `bson:"title_in_arabic" json:"title_in_arabic"`
	Email                      string                 `bson:"email,omitempty" json:"email"`
	Phone                      string                 `bson:"phone,omitempty" json:"phone"`
	PhoneInArabic              string                 `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic"`
	Address                    string                 `bson:"address,omitempty" json:"address"`
	AddressInArabic            string                 `bson:"address_in_arabic,omitempty" json:"address_in_arabic"`
	VATNo                      string                 `bson:"vat_no,omitempty" json:"vat_no"`
	VATNoInArabic              string                 `bson:"vat_no_in_arabic,omitempty" json:"vat_no_in_arabic"`
	VatPercent                 *float32               `bson:"vat_percent" json:"vat_percent"`
	RegistrationNumber         string                 `bson:"registration_number,omitempty" json:"registration_number"`
	RegistrationNumberInArabic string                 `bson:"registration_number_arabic,omitempty" json:"registration_number_in_arabic"`
	NationalAddresss           NationalAddress        `bson:"national_address,omitempty" json:"national_address"`
	CountryName                string                 `bson:"country_name" json:"country_name"`
	CountryCode                string                 `bson:"country_code" json:"country_code"`
	ContactPerson              string                 `bson:"contact_person,omitempty" json:"contact_person,omitempty"`
	CreditLimit                float64                `bson:"credit_limit" json:"credit_limit"`
	CreditBalance              float64                `json:"credit_balance" bson:"credit_balance"`
	Account                    *Account               `json:"account" bson:"account"`
	Logo                       string                 `bson:"logo,omitempty" json:"logo"`
	LogoContent                string                 `json:"logo_content,omitempty"`
	Deleted                    bool                   `bson:"deleted" json:"deleted"`
	DeletedBy                  *primitive.ObjectID    `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser              *User                  `json:"deleted_by_user,omitempty" bson:"-"`
	DeletedAt                  *time.Time             `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                  *time.Time             `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                  *time.Time             `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                  *primitive.ObjectID    `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                  *primitive.ObjectID    `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser              *User                  `json:"created_by_user,omitempty"`
	UpdatedByUser              *User                  `json:"updated_by_user,omitempty"`
	CreatedByName              string                 `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName              string                 `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName              string                 `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Stores                     map[string]VendorStore `bson:"stores" json:"stores"`
	SearchLabel                string                 `json:"search_label" bson:"search_label"`
	StoreID                    *primitive.ObjectID    `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Remarks                    string                 `bson:"remarks,omitempty" json:"remarks,omitempty"`
	UseRemarksInPurchases      bool                   `bson:"use_remarks_in_purchases" json:"use_remarks_in_purchases"`
	Images                     []string               `bson:"images,omitempty" json:"images,omitempty"`
}

func (store *Store) SaveVendorImage(vendorID *primitive.ObjectID, filename string) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")

	filter := bson.M{
		"_id":      vendorID,
		"store_id": store.ID,
	}

	update := bson.M{
		"$push": bson.M{
			"images": filename,
		},
	}

	_, err := collection.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (vendor *Vendor) GenerateSearchWords() {
	cleanedWords := CleanString(vendor.Code + "  " + vendor.Name + "  " + vendor.VATNo + "  " + vendor.Phone)
	cleanedWordsArabic := CleanString(vendor.NameInArabic + "  " + vendor.VATNoInArabic + "  " + vendor.PhoneInArabic)

	vendor.SearchWords = generatePrefixesSuffixesSubstrings(cleanedWords)

	additionalSearchTerms := vendor.GetAdditionalSearchTerms()
	for _, term := range additionalSearchTerms {
		vendor.SearchWords = append(vendor.SearchWords, generatePrefixesSuffixesSubstrings(term)...)
	}

	if vendor.Code != "" {
		vendor.SearchWords = append(vendor.SearchWords, string(vendor.Code[0]))
	}

	if vendor.Name != "" {
		vendor.SearchWords = append(vendor.SearchWords, string(vendor.Name[0]))
	}

	if vendor.VATNo != "" {
		vendor.SearchWords = append(vendor.SearchWords, string(vendor.VATNo[0]))
	}

	if vendor.Phone != "" {
		vendor.SearchWords = append(vendor.SearchWords, string(vendor.Phone[0]))
	}

	if cleanedWordsArabic != "" {
		vendor.SearchWordsInArabic = generatePrefixesSuffixesSubstrings(cleanedWordsArabic)
	}
}

func (vendor *Vendor) GetAdditionalSearchTerms() []string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	searchTerm := []string{}

	if containsSpecialChars(vendor.Name) {
		searchTerm = append(searchTerm, re.ReplaceAllString(vendor.Name, ""))

	}
	if containsSpecialChars(vendor.Code) {
		searchTerm = append(searchTerm, re.ReplaceAllString(vendor.Code, ""))

	}

	if vendor.CountryName != "" {
		searchTerm = append(searchTerm, re.ReplaceAllString(vendor.CountryName, ""))
	}

	return searchTerm
}

func (vendor *Vendor) SetAdditionalkeywords() {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	if containsSpecialChars(vendor.Code) {
		vendor.AdditionalKeywords = append(vendor.AdditionalKeywords, re.ReplaceAllString(vendor.Code, ""))
	}

	if containsSpecialChars(vendor.Name) {
		vendor.AdditionalKeywords = append(vendor.AdditionalKeywords, re.ReplaceAllString(vendor.Name, ""))
	}
}

func (vendor *Vendor) SetCreditBalance() error {
	if vendor == nil {
		return nil
	}

	store, err := FindStoreByID(vendor.StoreID, bson.M{})
	if err != nil {
		return err
	}
	var account *Account

	account, err = store.FindAccountByReferenceID(vendor.ID, store.ID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("error finding vendor account:" + err.Error())
	}

	if account == nil && vendor.VATNo != "" {
		account, err = store.FindAccountByVatNo(vendor.VATNo, &store.ID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("error finding vendor account:" + err.Error())
		}
	}

	if account == nil && vendor.Phone != "" {
		account, err = store.FindAccountByPhone(vendor.Phone, &store.ID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("error finding vendor account:" + err.Error())
		}
	}

	if account != nil {
		vendor.Account = account
		vendor.CreditBalance = account.Balance
		if account.Type == "liability" {
			vendor.CreditBalance = account.Balance * -1
		}
		err = vendor.Update()
		if err != nil {
			return errors.New("error updating vendor credit balance:" + err.Error())
		}
	}

	return nil
}

//Stores2                    []VendorStore          `bson:"stores,omitempty" json:"stores,omitempty"`

func (vendor *Vendor) AttributesValueChangeEvent(vendorOld *Vendor) error {

	if vendor.Name != vendorOld.Name {
		/*
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
		*/
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

func (store *Store) SearchVendor(w http.ResponseWriter, r *http.Request) (vendors []Vendor, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var storeID primitive.ObjectID
	keys, ok := r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return vendors, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.Replace(keys[0], "stores.", "stores."+storeID.Hex()+".", -1)
		criterias.SortBy = GetSortByFields(keys[0])
	}

	timeZoneOffset := 0.0
	keys, ok = r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	keys, ok = r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	keys, ok = r.URL.Query()["search[credit_balance]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["credit_balance"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["credit_balance"] = value
		}
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {

		vendorIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			vendorID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return vendors, criterias, err
			}
			objecIds = append(objecIds, vendorID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[purchase_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		//criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[phone]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		//criterias.SearchBy["phone"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[vat_no]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		//criterias.SearchBy["vat_no"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[purchase_paid_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_paid_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_paid_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_paid_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_balance_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_balance_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_retail_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_retail_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_retail_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_retail_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_retail_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_retail_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_retail_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_retail_loss", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_wholesale_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_wholesale_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_wholesale_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_wholesale_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_wholesale_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_wholesale_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_wholesale_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_wholesale_loss", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_not_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_not_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_not_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_not_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_paid_partially_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_paid_partially_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_paid_partially_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_paid_partially_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_return_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_return_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_paid_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_paid_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_paid_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_return_paid_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_balance_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_return_balance_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_return_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_not_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_not_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_not_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_return_not_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_paid_partially_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return vendors, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_paid_partially_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".purchase_return_paid_partially_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_return_paid_partially_count", operator, &storeID, value)
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
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		/*criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
			//{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
			//	{"phone_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}*/
	}

	keys, ok = r.URL.Query()["search[query]"]
	if ok && len(keys[0]) >= 1 {
		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		/*criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
			{"vat_no": bson.M{"$regex": keys[0], "$options": "i"}},
			{"code": bson.M{"$regex": keys[0], "$options": "i"}},
		}*/
	}

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		//criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	//findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	//findOptions.SetOplogReplay()

	/*
		createdByUserSelectFields := map[string]interface{}{}
		updatedByUserSelectFields := map[string]interface{}{}
		deletedByUserSelectFields := map[string]interface{}{}
	*/

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields

		/*
			if _, ok := criterias.Select["created_by_user.id"]; ok {
				createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
			}

			if _, ok := criterias.Select["created_by_user.id"]; ok {
				updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
			}

			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
			}*/

	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	findOptions.SetSort(criterias.SortBy)

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

		/*

			if _, ok := criterias.Select["created_by_user.id"]; ok {
				vendor.CreatedByUser, _ = FindUserByID(vendor.CreatedBy, createdByUserSelectFields)
			}
			if _, ok := criterias.Select["updated_by_user.id"]; ok {
				vendor.UpdatedByUser, _ = FindUserByID(vendor.UpdatedBy, updatedByUserSelectFields)
			}
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				vendor.DeletedByUser, _ = FindUserByID(vendor.DeletedBy, deletedByUserSelectFields)
			}*/

		vendors = append(vendors, vendor)
	} //end for loop

	return vendors, criterias, nil

}

func (vendor *Vendor) SetSearchLabel() {
	if vendor == nil {
		return
	}

	vendor.SearchLabel = "#" + vendor.Code + " " + vendor.Name

	if vendor.NameInArabic != "" {
		vendor.SearchLabel += " / " + vendor.NameInArabic
	}

	if vendor.Phone != "" {
		vendor.SearchLabel += " Phone: " + vendor.Phone
	}

	if vendor.PhoneInArabic != "" {
		vendor.SearchLabel += " / " + vendor.PhoneInArabic
	}

	if vendor.VATNo != "" {
		vendor.SearchLabel += " VAT #" + vendor.VATNo
	}
}

func (vendor *Vendor) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(vendor.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	if scenario == "update" {
		if vendor.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsVendorExists(&vendor.ID)
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

	if !govalidator.IsNull(strings.TrimSpace(vendor.VATNo)) && !IsValidDigitNumber(strings.TrimSpace(vendor.VATNo), "15") {
		errs["vat_no"] = "VAT No. should be 15 digits"
	} else if !govalidator.IsNull(strings.TrimSpace(vendor.VATNo)) && !IsNumberStartAndEndWith(strings.TrimSpace(vendor.VATNo), "3") {
		errs["vat_no"] = "VAT No. should start and end with 3"
	}

	if !govalidator.IsNull(vendor.RegistrationNumber) && !IsAlphanumeric(vendor.RegistrationNumber) {
		errs["registration_number"] = "Registration Number should be alpha numeric(a-zA-Z|0-9)"
	}

	if !govalidator.IsNull(strings.TrimSpace(vendor.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(vendor.Phone)) {
		errs["phone"] = "Invalid phone no."
	} else if !govalidator.IsNull(strings.TrimSpace(vendor.Phone)) {

		if strings.HasPrefix(vendor.Phone, "+966") {
			vendor.Phone = strings.TrimPrefix(vendor.Phone, "+966")
			vendor.Phone = "0" + vendor.Phone
		}

		phoneExists, err := vendor.IsPhoneExists()
		if err != nil {
			errs["phone"] = err.Error()
		}

		if phoneExists {
			errs["phone"] = "Phone No. already exists."
		}

		if phoneExists {
			w.WriteHeader(http.StatusConflict)
			return errs
		}
	}

	if !govalidator.IsNull(strings.TrimSpace(vendor.VATNo)) && !govalidator.IsNull(strings.TrimSpace(vendor.Name)) {
		vendorExists, err := vendor.IsVendorExistsByVatNoByName()
		if err != nil {
			errs["vat_no"] = err.Error()
		}

		if vendorExists {
			errs["vat_no"] = "VAT No. already exists with vendor name: " + vendor.Name
		}

		if vendorExists {
			w.WriteHeader(http.StatusConflict)
			return errs
		}
	}

	if !govalidator.IsNull(strings.TrimSpace(vendor.Code)) {
		codeExists, err := vendor.IsCodeExists()
		if err != nil {
			errs["code"] = err.Error()
		}

		if codeExists {
			errs["code"] = "ID already exists."
		}

		if codeExists {
			w.WriteHeader(http.StatusConflict)
			return errs
		}
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

func (vendor *Vendor) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if vendor.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": vendor.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": vendor.Code,
			"_id":  bson.M{"$ne": vendor.ID},
		})
	}

	return (count > 0), err
}

func (vendor *Vendor) IsVatNoExists() (exists bool, err error) {
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if vendor.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no": vendor.VATNo,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no": vendor.VATNo,
			"_id":    bson.M{"$ne": vendor.ID},
		})
	}

	return (count > 0), err
}

func (vendor *Vendor) IsVendorExistsByVatNoByName() (exists bool, err error) {
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if vendor.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no": vendor.VATNo,
			"name":   vendor.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no": vendor.VATNo,
			"name":   vendor.Name,
			"_id":    bson.M{"$ne": vendor.ID},
		})
	}

	return (count > 0), err
}

func (vendor *Vendor) IsPhoneExists() (exists bool, err error) {
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if vendor.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone": vendor.Phone,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone": vendor.Phone,
			"_id":   bson.M{"$ne": vendor.ID},
		})
	}

	return (count > 0), err
}

func (vendor *Vendor) Insert() error {
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
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
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
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
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
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

func (store *Store) FindVendorByID(
	ID *primitive.ObjectID,
	selectFields bson.M,
) (vendor *Vendor, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")
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
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
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

	return (count > 0), err
}

func (store *Store) IsVendorExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (store *Store) GetVendorCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

// make vendor code
func (vendor *Vendor) MakeCode() error {
	store, err := FindStoreByID(vendor.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := vendor.StoreID.Hex() + "_vendor_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetVendorCount()
		if err != nil {
			return err
		}

		startFrom := store.VendorSerialNumber.StartFromCount

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

	paddingCount := store.VendorSerialNumber.PaddingCount

	if store.VendorSerialNumber.Prefix != "" {
		vendor.Code = fmt.Sprintf("%s-%0*d", store.VendorSerialNumber.Prefix, paddingCount, incr)
	} else {
		vendor.Code = fmt.Sprintf("%s%0*d", store.VendorSerialNumber.Prefix, paddingCount, incr)
	}

	if store.CountryCode != "" {
		timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]
		if ok {
			location, err := time.LoadLocation(timeZone)
			if err != nil {
				return errors.New("error loading location")
			}
			currentDate := time.Now().In(location).Format("20060102") // YYYYMMDD
			vendor.Code = strings.ReplaceAll(vendor.Code, "DATE", currentDate)
		}
	}

	return nil
}

func ProcessVendors() error {
	log.Printf("Processing vendors")

	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		totalCount, err := store.GetTotalCount(bson.M{"store_id": store.ID}, "vendor")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetSort(map[string]interface{}{"created_at": 1})
		findOptions.SetAllowDiskUse(true)

		cur, err := collection.Find(ctx, bson.M{"store_id": store.ID}, findOptions)
		if err != nil {
			return errors.New("Error fetching vendors" + err.Error())
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
			vendor := Vendor{}
			err = cur.Decode(&vendor)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if vendor.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			vendor.GenerateSearchWords()
			vendor.SetSearchLabel()
			vendor.SetAdditionalkeywords()
			err = vendor.Update()
			if err != nil {
				log.Print("Store ID:" + store.ID.Hex())
				log.Print("Vendor Code.:" + vendor.Code)
				log.Print("Vendor ID:" + vendor.ID.Hex())
				continue
				//return err
			}

			/*
				vendor.Name = strings.ToUpper(vendor.Name)
				vendor.Update()
				account, _ := store.FindAccountByID(vendor.ID, bson.M{})
				if account != nil {
					account.Name = vendor.Name
					account.Update()
				}
			*/

			/*
				vendor.GenerateSearchWords()
				err = vendor.Update()
				if err != nil {
					log.Print("Store ID:" + store.ID.Hex())
					log.Print("Code:" + vendor.Code)
					log.Print("Name:" + vendor.Name)
					continue
					//return err
				}*/

			//	vendor.SetCreditBalance()
			//vendor.MakeCode()

			/*vendor.Code = fmt.Sprintf("%s%0*d", store.VendorSerialNumber.Prefix, store.VendorSerialNumber.PaddingCount, i+1)

			err = vendor.Update()
			if err != nil {
				return err
			}*/

			bar.Add(1)
		}
	}

	log.Print("DONE!")
	return nil
}

func (vendor *Vendor) RestoreVendor(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + vendor.StoreID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = vendor.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	vendor.Deleted = false
	vendor.DeletedBy = nil
	vendor.DeletedAt = nil

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
