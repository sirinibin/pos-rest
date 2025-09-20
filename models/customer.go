package models

import (
	"context"
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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CustomerStore struct {
	StoreID                                primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName                              string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic                      string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	SalesCount                             int64              `bson:"sales_count" json:"sales_count"`
	SalesAmount                            float64            `bson:"sales_amount" json:"sales_amount"`
	SalesPaidAmount                        float64            `bson:"sales_paid_amount" json:"sales_paid_amount"`
	SalesBalanceAmount                     float64            `bson:"sales_balance_amount" json:"sales_balance_amount"`
	SalesProfit                            float64            `bson:"sales_profit" json:"sales_profit"`
	SalesLoss                              float64            `bson:"sales_loss" json:"sales_loss"`
	SalesPaidCount                         int64              `bson:"sales_paid_count" json:"sales_paid_count"`
	SalesNotPaidCount                      int64              `bson:"sales_not_paid_count" json:"sales_not_paid_count"`
	SalesPaidPartiallyCount                int64              `bson:"sales_paid_partially_count" json:"sales_paid_partially_count"`
	SalesReturnCount                       int64              `bson:"sales_return_count" json:"sales_return_count"`
	SalesReturnAmount                      float64            `bson:"sales_return_amount" json:"sales_return_amount"`
	SalesReturnPaidAmount                  float64            `bson:"sales_return_paid_amount" json:"sales_return_paid_amount"`
	SalesReturnBalanceAmount               float64            `bson:"sales_return_balance_amount" json:"sales_return_balance_amount"`
	SalesReturnProfit                      float64            `bson:"sales_return_profit" json:"sales_return_profit"`
	SalesReturnLoss                        float64            `bson:"sales_return_loss" json:"sales_return_loss"`
	SalesReturnPaidCount                   int64              `bson:"sales_return_paid_count" json:"sales_return_paid_count"`
	SalesReturnNotPaidCount                int64              `bson:"sales_return_not_paid_count" json:"sales_return_not_paid_count"`
	SalesReturnPaidPartiallyCount          int64              `bson:"sales_return_paid_partially_count" json:"sales_return_paid_partially_count"`
	QuotationSalesReturnCount              int64              `bson:"quotation_sales_return_count" json:"quotation_sales_return_count"`
	QuotationSalesReturnAmount             float64            `bson:"quotation_sales_return_amount" json:"quotation_sales_return_amount"`
	QuotationSalesReturnPaidAmount         float64            `bson:"quotation_sales_return_paid_amount" json:"quotation_sales_return_paid_amount"`
	QuotationSalesReturnBalanceAmount      float64            `bson:"quotation_sales_return_balance_amount" json:"quotation_sales_return_balance_amount"`
	QuotationSalesReturnProfit             float64            `bson:"quotation_sales_return_profit" json:"quotation_sales_return_profit"`
	QuotationSalesReturnLoss               float64            `bson:"quotation_sales_return_loss" json:"quotation_sales_return_loss"`
	QuotationSalesReturnPaidCount          int64              `bson:"quotation_sales_return_paid_count" json:"quotation_sales_return_paid_count"`
	QuotationSalesReturnNotPaidCount       int64              `bson:"quotation_sales_return_not_paid_count" json:"quotation_sales_return_not_paid_count"`
	QuotationSalesReturnPaidPartiallyCount int64              `bson:"quotation_sales_return_paid_partially_count" json:"quotation_sales_return_paid_partially_count"`
	QuotationCount                         int64              `bson:"quotation_count" json:"quotation_count"`
	QuotationAmount                        float64            `bson:"quotation_amount" json:"quotation_amount"`
	QuotationProfit                        float64            `bson:"quotation_profit" json:"quotation_profit"`
	QuotationLoss                          float64            `bson:"quotation_loss" json:"quotation_loss"`
	QuotationInvoiceCount                  int64              `bson:"quotation_invoice_count" json:"quotation_invoice_count"`
	QuotationInvoiceAmount                 float64            `bson:"quotation_invoice_amount" json:"quotation_invoice_amount"`
	QuotationInvoiceProfit                 float64            `bson:"quotation_invoice_profit" json:"quotation_invoice_profit"`
	QuotationInvoiceLoss                   float64            `bson:"quotation_invoice_loss" json:"quotation_invoice_loss"`
	QuotationInvoiceNotPaidCount           int64              `bson:"quotation_invoice_not_paid_count" json:"quotation_invoice_not_paid_count"`
	QuotationInvoicePaidPartiallyCount     int64              `bson:"quotation_invoice_paid_partially_count" json:"quotation_invoice_paid_partially_count"`
	QuotationInvoiceBalanceAmount          float64            `bson:"quotation_invoice_balance_amount" json:"quotation_invoice_balance_amount"`
	QuotationInvoicePaidCount              int64              `bson:"quotation_invoice_paid_count" json:"quotation_invoice_paid_count"`
	QuotationInvoicePaidAmount             float64            `bson:"quotation_invoice_paid_amount" json:"quotation_invoice_paid_amount"`
	DeliveryNoteCount                      int64              `bson:"delivery_note_count" json:"delivery_note_count"`
}

// Customer : Customer structure
type Customer struct {
	ID                         primitive.ObjectID       `json:"id,omitempty" bson:"_id,omitempty"`
	Code                       string                   `bson:"code,omitempty" json:"code,omitempty"`
	Name                       string                   `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic               string                   `bson:"name_in_arabic" json:"name_in_arabic"`
	SearchWords                []string                 `bson:"search_words,omitempty" json:"search_words,omitempty"`
	SearchWordsInArabic        []string                 `bson:"search_words_in_arabic,omitempty" json:"search_words_in_arabic,omitempty"`
	AdditionalKeywords         []string                 `bson:"additional_keywords" json:"additional_keywords"`
	VATNo                      string                   `bson:"vat_no,omitempty" json:"vat_no,omitempty"`
	VATNoInArabic              string                   `bson:"vat_no_in_arabic,omitempty" json:"vat_no_in_arabic,omitempty"`
	Phone                      string                   `bson:"phone,omitempty" json:"phone,omitempty"`
	PhoneInArabic              string                   `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic,omitempty"`
	Phone2                     string                   `bson:"phone2" json:"phone2"`
	Phone2InArabic             string                   `bson:"phone2_in_arabic" json:"phone2_in_arabic"`
	Title                      string                   `bson:"title,omitempty" json:"title,omitempty"`
	TitleInArabic              string                   `bson:"title_in_arabic,omitempty" json:"title_in_arabic,omitempty"`
	Email                      string                   `bson:"email,omitempty" json:"email,omitempty"`
	Address                    string                   `bson:"address,omitempty" json:"address,omitempty"`
	AddressInArabic            string                   `bson:"address_in_arabic,omitempty" json:"address_in_arabic,omitempty"`
	CountryName                string                   `bson:"country_name" json:"country_name"`
	CountryCode                string                   `bson:"country_code" json:"country_code"`
	NationalAddress            NationalAddress          `bson:"national_address,omitempty" json:"national_address,omitempty"`
	RegistrationNumber         string                   `bson:"registration_number,omitempty" json:"registration_number,omitempty"`
	RegistrationNumberInArabic string                   `bson:"registration_number_arabic,omitempty" json:"registration_number_in_arabic,omitempty"`
	ContactPerson              string                   `bson:"contact_person,omitempty" json:"contact_person,omitempty"`
	CreditLimit                float64                  `bson:"credit_limit" json:"credit_limit"`
	CreditBalance              float64                  `json:"credit_balance" bson:"credit_balance"`
	Account                    *Account                 `json:"account" bson:"account"`
	Deleted                    bool                     `bson:"deleted" json:"deleted"`
	DeletedBy                  *primitive.ObjectID      `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser              *User                    `json:"deleted_by_user,omitempty" bson:"-"`
	DeletedAt                  *time.Time               `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                  *time.Time               `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                  *time.Time               `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                  *primitive.ObjectID      `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                  *primitive.ObjectID      `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser              *User                    `json:"created_by_user,omitempty"`
	UpdatedByUser              *User                    `json:"updated_by_user,omitempty"`
	CreatedByName              string                   `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName              string                   `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName              string                   `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	SearchLabel                string                   `json:"search_label" bson:"search_label"`
	StoreID                    *primitive.ObjectID      `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Stores                     map[string]CustomerStore `bson:"stores" json:"stores"`
	Remarks                    string                   `bson:"remarks" json:"remarks"`
	Sponsor                    string                   `bson:"sponsor" json:"sponsor"`
	UseRemarksInSales          bool                     `bson:"use_remarks_in_sales" json:"use_remarks_in_sales"`
	Images                     []string                 `bson:"images,omitempty" json:"images,omitempty"`
}

func (customer *Customer) InitStore() (err error) {
	_, ok := customer.Stores[customer.StoreID.Hex()]
	if ok {
		return
	}

	customer.Stores = map[string]CustomerStore{}

	if !customer.StoreID.IsZero() && customer.StoreID.Hex() != "" {
		customer.Stores[customer.StoreID.Hex()] = CustomerStore{
			StoreID: *customer.StoreID,
		}
	}

	return nil
}

func (customer *Customer) CopyToStore(storeID *primitive.ObjectID) (err error) {
	store, err := FindStoreByID(storeID, bson.M{})
	if err != nil {
		return err
	}

	collection := db.GetDB("store_" + storeID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	customerInDb, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customerInDb != nil {
		return nil
	}

	customerInDb, err = store.FindCustomerByCode(customer.Code, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customerInDb != nil {
		return nil
	}

	customerInDb, err = store.FindCustomerByNameByVatNo(customer.Name, customer.VATNo, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customerInDb != nil {
		return nil
	}

	customer.StoreID = storeID
	customer.CreditBalance = 0
	customer.InitStore()
	err = customer.MakeCode()
	if err != nil {
		return err
	}

	_, err = collection.InsertOne(ctx, &customer)
	if err != nil {
		return err
	}

	customer.GenerateSearchWords()
	customer.SetAdditionalkeywords()
	customer.SetSearchLabel()

	err = customer.Update()
	if err != nil {
		return err
	}
	log.Print("Code:" + customer.Code)

	return nil
}

func (store *Store) SaveCustomerImage(customerID *primitive.ObjectID, filename string) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")

	filter := bson.M{
		"_id":      customerID,
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

func (customer *Customer) GenerateSearchWords() {
	cleanedWords := CleanString(strings.ToLower(customer.Name + "  " + customer.VATNo + "  " + customer.Phone))
	cleanedWordsArabic := CleanString(customer.NameInArabic + "  " + customer.VATNoInArabic + "  " + customer.PhoneInArabic)

	customer.SearchWords = generatePrefixesSuffixesSubstrings(cleanedWords)

	cleanedWords = CleanString(customer.Code)

	customer.SearchWords = append(customer.SearchWords, generatePrefixesSuffixesSubstrings(cleanedWords)...)

	additionalSearchTerms := customer.GetAdditionalSearchTerms()
	for _, term := range additionalSearchTerms {
		customer.SearchWords = append(customer.SearchWords, generatePrefixesSuffixesSubstrings(term)...)
	}

	if customer.Code != "" {
		customer.SearchWords = append(customer.SearchWords, strings.ToLower(string(customer.Code[0])))
	}

	if !govalidator.IsNull(customer.Name) {
		customer.SearchWords = append(customer.SearchWords, strings.ToLower(string(customer.Name[0])))
	}

	if !govalidator.IsNull(customer.VATNo) {
		customer.SearchWords = append(customer.SearchWords, string(customer.VATNo[0]))
	}

	if !govalidator.IsNull(customer.Phone) {
		customer.SearchWords = append(customer.SearchWords, string(customer.Phone[0]))
	}

	if cleanedWordsArabic != "" {
		customer.SearchWordsInArabic = generatePrefixesSuffixesSubstrings(cleanedWordsArabic)
	}
}

func (customer *Customer) GetAdditionalSearchTerms() []string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	searchTerm := []string{}

	if containsSpecialChars(customer.Name) {
		searchTerm = append(searchTerm, re.ReplaceAllString(strings.ToLower(customer.Name), ""))

	}
	if containsSpecialChars(customer.Code) {
		searchTerm = append(searchTerm, re.ReplaceAllString(strings.ToLower(customer.Code), ""))

	}

	if customer.CountryName != "" {
		searchTerm = append(searchTerm, re.ReplaceAllString(strings.ToLower(customer.CountryName), ""))
	}

	return searchTerm
}

func (customer *Customer) SetAdditionalkeywords() {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	if containsSpecialChars(customer.Code) {
		customer.AdditionalKeywords = append(customer.AdditionalKeywords, re.ReplaceAllString(customer.Code, ""))
	}

	if containsSpecialChars(customer.Name) {
		customer.AdditionalKeywords = append(customer.AdditionalKeywords, re.ReplaceAllString(customer.Name, ""))
	}
}

/*
func (customer *Customer) SetChangeLog(
	event string,
	attribute, oldValue, newValue *string,
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
	} else if event == "attribute_value_change" && attribute != nil {
		description = *attribute + " changed from " + *oldValue + " to " + *newValue + " by " + UserObject.Name
	}

	customer.ChangeLog = append(
		customer.ChangeLog,
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

func (customer *Customer) SetCreditBalance() error {
	if customer == nil {
		return nil
	}

	store, err := FindStoreByID(customer.StoreID, bson.M{})
	if err != nil {
		return err
	}
	var account *Account

	account, err = store.FindAccountByReferenceID(customer.ID, store.ID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("error finding customer account:" + err.Error())
	}

	if account == nil && customer.VATNo != "" {
		account, err = store.FindAccountByVatNoByName(customer.VATNo, customer.Name, &store.ID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("error finding vendor account:" + err.Error())
		}
	}

	if account == nil {
		return nil
	}

	/*
		if account == nil && customer.VATNo != "" {
			account, err = store.FindAccountByVatNo(customer.VATNo, &store.ID, bson.M{})
			if err != nil && err != mongo.ErrNoDocuments {
				return errors.New("error finding customer account:" + err.Error())
			}
		}

		if account == nil && customer.Phone != "" {
			account, err = store.FindAccountByPhone(customer.Phone, &store.ID, bson.M{})
			if err != nil && err != mongo.ErrNoDocuments {
				return errors.New("error finding customer account:" + err.Error())
			}
		}
	*/

	if account != nil {
		customer.Account = account
		customer.CreditBalance = account.Balance
		if account.Type == "liability" {
			customer.CreditBalance = account.Balance * -1
		}
		err = customer.Update()
		if err != nil {
			return errors.New("error updating customer credit balance:" + err.Error())
		}

		vendor, err := store.FindVendorByNameByVatNo(customer.Name, customer.VATNo, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return err
		}

		if vendor != nil {
			vendor.CreditBalance = customer.CreditBalance
			err = vendor.Update()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (customer *Customer) AttributesValueChangeEvent(customerOld *Customer) error {

	store, err := FindStoreByID(customer.StoreID, bson.M{})
	if err != nil {
		return nil
	}

	if customer.Name != customerOld.Name || customer.NameInArabic != customerOld.NameInArabic {
		err := store.UpdateManyByCollectionName(
			"order",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"sales_return",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"quotation",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"quotation_sales_return",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"customerwithdrawal",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"customerdeposit",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"delivery_note",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		//Histories
		err = store.UpdateManyByCollectionName(
			"product_history",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"product_sales_history",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"product_sales_return_history",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"product_quotation_history",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"product_quotation_sales_return_history",
			bson.M{"customer_id": customer.ID},
			bson.M{
				"customer_name":        customer.Name,
				"customer_name_arabic": customer.NameInArabic,
			},
		)
		if err != nil {
			return nil
		}

		if customerOld.VATNo != "" {
			err = store.UpdateManyByCollectionName(
				"vendor",
				bson.M{
					"name":   customerOld.Name,
					"vat_no": customerOld.VATNo,
				},
				bson.M{
					"name":           customer.Name,
					"name_in_arabic": customer.NameInArabic,
				},
			)
			if err != nil {
				return nil
			}

			err = store.UpdateManyByCollectionName(
				"account",
				bson.M{
					"name":   customerOld.Name,
					"vat_no": customerOld.VATNo,
				},
				bson.M{
					"name":        customer.Name,
					"name_arabic": customer.NameInArabic,
				},
			)
			if err != nil {
				return nil
			}
		} else {
			err = store.UpdateManyByCollectionName(
				"account",
				bson.M{"reference_id": customer.ID},
				bson.M{
					"name":        customer.Name,
					"name_arabic": customer.NameInArabic,
				},
			)
			if err != nil {
				return nil
			}
		}
	}

	if customer.VATNo != customerOld.VATNo {
		if customerOld.VATNo != "" {
			err = store.UpdateManyByCollectionName(
				"vendor",
				bson.M{
					"name":   customerOld.Name,
					"vat_no": customerOld.VATNo,
				},
				bson.M{
					"vat_no":           customer.VATNo,
					"vat_no_in_arabic": customer.VATNoInArabic,
				},
			)
			if err != nil {
				return nil
			}

			err = store.UpdateManyByCollectionName(
				"account",
				bson.M{
					"name":   customerOld.Name,
					"vat_no": customerOld.VATNo,
				},
				bson.M{
					"vat_no":           customer.VATNo,
					"vat_no_in_arabic": customer.VATNoInArabic,
				},
			)
			if err != nil {
				return nil
			}
		} else {
			err = store.UpdateManyByCollectionName(
				"account",
				bson.M{"reference_id": customer.ID},
				bson.M{
					"vat_no": customer.VATNo,
				},
			)
			if err != nil {
				return nil
			}
		}
	}

	return nil
}

func (store *Store) UpdateManyByCollectionName(
	collectionName string,
	filter bson.M,
	updateValues bson.M,
) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
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

func (customer *Customer) UpdateForeignLabelFields() error {

	if customer.CreatedBy != nil {
		createdByUser, err := FindUserByID(customer.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customer.CreatedByName = createdByUser.Name
	}

	if customer.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(customer.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customer.UpdatedByName = updatedByUser.Name
	}

	if customer.DeletedBy != nil && !customer.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(customer.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customer.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (store *Store) SearchCustomer(w http.ResponseWriter, r *http.Request) (customers []Customer, criterias SearchCriterias, err error) {

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
			return customers, criterias, err
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
			return customers, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		//criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customers, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["_id"] = bson.M{"$in": objecIds}
		}
	}

	//quotation invoice
	keys, ok = r.URL.Query()["search[quotation_invoice_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_count"] = value
		}
	}

	keys, ok = r.URL.Query()["search[credit_balance]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["credit_balance"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["credit_balance"] = value
		}
	}

	keys, ok = r.URL.Query()["search[credit_limit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["credit_limit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["credit_limit"] = value
		}
	}

	keys, ok = r.URL.Query()["search[ignore_zero_credit_balance]"]
	if ok && len(keys[0]) >= 1 {
		value := ParseBoolToInt(keys[0])
		if value == 1 {
			criterias.SearchBy["credit_balance"] = bson.M{"$ne": 0}
		}
	}

	keys, ok = r.URL.Query()["search[ignore_zero_qtn_credit_balance]"]
	if ok && len(keys[0]) >= 1 {
		value := ParseBoolToInt(keys[0])
		if value == 1 {
			criterias.SearchBy["$and"] = []bson.M{
				bson.M{"stores." + storeID.Hex() + ".quotation_invoice_balance_amount": bson.M{"$ne": 0}},
				bson.M{"stores." + storeID.Hex() + ".quotation_invoice_balance_amount": bson.M{"$ne": nil}},
			}
		}
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_count"] = value
		}

		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_not_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_not_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_not_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_not_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_paid_partially_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_partially_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_partially_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_paid_partially_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_paid_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_paid_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}
		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_balance_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_balance_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_loss", operator, &storeID, value)
	}

	//sales
	keys, ok = r.URL.Query()["search[sales_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_count"] = value
		}
	}

	keys, ok = r.URL.Query()["search[sales_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_paid_count"] = value
		}

		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_not_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_not_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_not_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_not_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_paid_partially_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_paid_partially_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_paid_partially_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_paid_partially_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_return_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_paid_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_paid_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_paid_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_paid_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}
		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_balance_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_balance_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_loss", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_amount"] = value
		}

		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_paid_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_paid_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_paid_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_paid_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_balance_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_balance_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_loss", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_paid_count"] = value
		}

		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_return_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_not_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_not_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_not_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_return_not_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_paid_partially_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_paid_partially_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".sales_return_paid_partially_count"] = value
		}
	}

	//qtn. sales return

	keys, ok = r.URL.Query()["search[quotation_sales_return_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_count"] = value
		}
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_amount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_paid_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_paid_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_paid_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_paid_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_balance_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_balance_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_loss", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_paid_count"] = value
		}

		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_return_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_not_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_not_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_not_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_return_not_paid_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_paid_partially_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_paid_partially_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_sales_return_paid_partially_count"] = value
		}
	}

	//end qtn. sales return

	keys, ok = r.URL.Query()["search[quotation_invoice_credit_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_credit_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_credit_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("quotation_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_credit_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_credit_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_credit_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("quotation_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_paid_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_paid_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("quotation_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_invoice_credit_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_credit_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_invoice_credit_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("quotation_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("quotation_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_amount"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("quotation_amount", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("quotation_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".quotation_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("quotation_loss", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[delivery_note_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return customers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stores."+storeID.Hex()+".delivery_note_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["stores."+storeID.Hex()+".delivery_note_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("delivery_note_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		/*
			criterias.SearchBy["$or"] = []bson.M{
				{"name": bson.M{"$regex": keys[0], "$options": "i"}},
				{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
				//{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
				//{"phone_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
			}*/
	}

	textSearching := false
	keys, ok = r.URL.Query()["search[query]"]
	if ok && len(keys[0]) >= 1 {
		textSearching = true
		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}

		searchWord := strings.ToLower(keys[0])
		criterias.SearchBy["$text"] = bson.M{"$search": "\"" + searchWord + "\""}
		//criterias.SearchBy["$text"] = bson.M{"$search": searchWord}
		/*
			criterias.SearchBy["$or"] = []bson.M{
				{"name": bson.M{"$regex": keys[0], "$options": "i"}},
				{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
				{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
				{"phone_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
				{"vat_no": bson.M{"$regex": keys[0], "$options": "i"}},
				{"code": bson.M{"$regex": keys[0], "$options": "i"}},
			}*/
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

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
		//criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customers, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return customers, criterias, err
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
			return customers, criterias, err
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
			return customers, criterias, err
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

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))

	if textSearching {
		sortBy := bson.D{
			bson.E{Key: "score", Value: bson.M{"$meta": "textScore"}},
			//bson.E{Key: sortFieldName, Value: sortValue},
		}
		findOptions.SetSort(sortBy)
	} else {
		findOptions.SetSort(criterias.SortBy)
	}

	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

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

			if _, ok := criterias.Select["updated_by_user.id"]; ok {
				updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
			}

			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
			}
		*/

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
		return customers, criterias, errors.New("Error fetching Customers:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return customers, criterias, errors.New("Cursor error:" + err.Error())
		}
		customer := Customer{}
		err = cur.Decode(&customer)
		if err != nil {
			return customers, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		/*
			if _, ok := criterias.Select["created_by_user.id"]; ok {
				customer.CreatedByUser, _ = FindUserByID(customer.CreatedBy, createdByUserSelectFields)
			}
			if _, ok := criterias.Select["updated_by_user.id"]; ok {
				customer.UpdatedByUser, _ = FindUserByID(customer.UpdatedBy, updatedByUserSelectFields)
			}
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				customer.DeletedByUser, _ = FindUserByID(customer.DeletedBy, deletedByUserSelectFields)
			}*/

		customers = append(customers, customer)
	} //end for loop

	return customers, criterias, nil

}

func (customer *Customer) SetSearchLabel() {
	if customer == nil {
		return
	}
	customer.SearchLabel = "#" + customer.Code + " " + customer.Name

	if customer.NameInArabic != "" {
		customer.SearchLabel += " | " + customer.NameInArabic
	}

	if customer.Phone != "" {
		customer.SearchLabel += " Phone: " + customer.Phone
	}

	if customer.PhoneInArabic != "" {
		customer.SearchLabel += " | " + customer.PhoneInArabic
	}

	if customer.VATNo != "" {
		customer.SearchLabel += " VAT #" + customer.VATNo
	}
}

func (customer *Customer) TrimSpaceFromFields() {
	customer.Name = strings.TrimSpace(customer.Name)
	customer.NameInArabic = strings.TrimSpace(customer.NameInArabic)
	customer.Phone = strings.TrimSpace(customer.Phone)
	customer.VATNo = strings.TrimSpace(customer.VATNo)
	customer.RegistrationNumber = strings.TrimSpace(customer.RegistrationNumber)
	customer.Email = strings.TrimSpace(customer.Email)
	customer.Address = strings.TrimSpace(customer.Address)
	customer.AddressInArabic = strings.TrimSpace(customer.AddressInArabic)
	customer.NationalAddress.BuildingNo = strings.TrimSpace(customer.NationalAddress.BuildingNo)
	customer.NationalAddress.StreetName = strings.TrimSpace(customer.NationalAddress.StreetName)
	customer.NationalAddress.StreetNameArabic = strings.TrimSpace(customer.NationalAddress.StreetNameArabic)
	customer.NationalAddress.DistrictName = strings.TrimSpace(customer.NationalAddress.DistrictName)
	customer.NationalAddress.DistrictNameArabic = strings.TrimSpace(customer.NationalAddress.DistrictNameArabic)
	customer.NationalAddress.CityName = strings.TrimSpace(customer.NationalAddress.CityName)
	customer.NationalAddress.CityNameArabic = strings.TrimSpace(customer.NationalAddress.CityNameArabic)
	customer.NationalAddress.ZipCode = strings.TrimSpace(customer.NationalAddress.ZipCode)
	customer.NationalAddress.AdditionalNo = strings.TrimSpace(customer.NationalAddress.AdditionalNo)
	customer.NationalAddress.UnitNo = strings.TrimSpace(customer.NationalAddress.UnitNo)
}

func (customer *Customer) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)
	customer.TrimSpaceFromFields()

	store, err := FindStoreByID(customer.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if customer.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsCustomerExists(&customer.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Customer:" + customer.ID.Hex()
		}

	}

	if customer.CountryCode == "" || customer.CountryCode == "SA" {
		if !govalidator.IsNull(strings.TrimSpace(customer.VATNo)) && !IsValidDigitNumber(strings.TrimSpace(customer.VATNo), "15") {
			errs["vat_no"] = "VAT No. should be 15 digits"
		} else if !govalidator.IsNull(strings.TrimSpace(customer.VATNo)) && !IsNumberStartAndEndWith(strings.TrimSpace(customer.VATNo), "3") {
			errs["vat_no"] = "VAT No. should start and end with 3"
		}
	}

	if !govalidator.IsNull(customer.RegistrationNumber) && !IsAlphanumeric(customer.RegistrationNumber) {
		errs["registration_number"] = "Registration Number should be alpha numeric(a-zA-Z|0-9)"
	}

	//National address
	if !govalidator.IsNull(strings.TrimSpace(customer.VATNo)) && store.Zatca.Phase == "2" {
		if !govalidator.IsNull(customer.NationalAddress.BuildingNo) && !IsValidDigitNumber(customer.NationalAddress.BuildingNo, "4") {
			errs["national_address_building_no"] = "Building number should be 4 digits"
		}

		if !govalidator.IsNull(customer.NationalAddress.ZipCode) && !IsValidDigitNumber(customer.NationalAddress.ZipCode, "5") {
			errs["national_address_zipcode"] = "Zip code should be 5 digits"
		}
	}

	if govalidator.IsNull(customer.Name) {
		errs["name"] = "Name is required"
	}

	if !govalidator.IsNull(strings.TrimSpace(customer.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(customer.Phone)) {
		errs["phone"] = "Invalid phone no."
	} else if !govalidator.IsNull(strings.TrimSpace(customer.Phone)) {
		if strings.HasPrefix(customer.Phone, "+966") {
			customer.Phone = strings.TrimPrefix(customer.Phone, "+966")
			customer.Phone = "0" + customer.Phone
		}

		/*
			phoneExists, err := customer.IsPhoneExists()
			if err != nil {
				errs["phone"] = err.Error()
			}

			if phoneExists {
				errs["phone"] = "Phone No. already exists."
			}

			if phoneExists {
				w.WriteHeader(http.StatusConflict)
				return errs
			}*/
	}

	if !govalidator.IsNull(strings.TrimSpace(customer.VATNo)) && !govalidator.IsNull(strings.TrimSpace(customer.Name)) {
		customerExists, err := customer.IsCustomerExistsByVatNoByName()
		if err != nil {
			errs["vat_no"] = err.Error()
		}

		if customerExists {
			errs["vat_no"] = "VAT No. already exists with customer name: " + customer.Name
		}

		if customerExists {
			w.WriteHeader(http.StatusConflict)
			return errs
		}
	}

	/*
		if scenario != "update" && !govalidator.IsNull(strings.TrimSpace(customer.VATNo)) {
			vatNoExists, err := customer.IsVatNoExists()
			if err != nil {
				errs["vat_no"] = err.Error()
			}

			if vatNoExists {
				errs["vat_no"] = "VAT No. already exists."
			}

			if vatNoExists {
				w.WriteHeader(http.StatusConflict)
				return errs
			}
		}*/

	if !govalidator.IsNull(strings.TrimSpace(customer.Code)) {
		codeExists, err := customer.IsCodeExists()
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

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (customer *Customer) IsCustomerExistsByVatNoByName() (exists bool, err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no": customer.VATNo,
			"name":   customer.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no": customer.VATNo,
			"name":   customer.Name,
			"_id":    bson.M{"$ne": customer.ID},
		})
	}

	return (count > 0), err
}

func (customer *Customer) Insert() error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	customer.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &customer)
	if err != nil {
		return err
	}

	return nil
}

func (customer *Customer) Update() error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": customer.ID},
		bson.M{"$set": customer},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (customer *Customer) DeleteCustomer(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = customer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	customer.Deleted = true
	customer.DeletedBy = &userID
	now := time.Now()
	customer.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customer.ID},
		bson.M{"$set": customer},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func IsValidDigitNumber(s string, digitsCount string) bool {
	// Regular expression to match exactly 4 digits
	re := regexp.MustCompile(`^\d{` + digitsCount + `}$`)
	return re.MatchString(s)
}

func (store *Store) FindCustomerByNameByPhone(
	Name string,
	Phone string,
	selectFields map[string]interface{},
) (customer *Customer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"name":     Name,
			"phone":    Phone,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&customer)
	if err != nil {
		return nil, err
	}

	return customer, err
}

func (store *Store) FindCustomerByNameByVatNo(
	Name string,
	VatNo string,
	selectFields map[string]interface{},
) (customer *Customer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"name":     Name,
			"vat_no":   VatNo,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&customer)
	if err != nil {
		return nil, err
	}

	return customer, err
}

func (store *Store) FindCustomerByName(
	Name string,
	selectFields map[string]interface{},
) (customer *Customer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"name":     Name,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&customer)
	if err != nil {
		return nil, err
	}

	return customer, err
}

func (store *Store) FindCustomerByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customer *Customer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
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
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&customer)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		customer.CreatedByUser, _ = FindUserByID(customer.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		customer.UpdatedByUser, _ = FindUserByID(customer.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		customer.DeletedByUser, _ = FindUserByID(customer.DeletedBy, fields)
	}

	return customer, err
}

func (store *Store) FindCustomerByCode(
	Code string,
	selectFields map[string]interface{},
) (customer *Customer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"code":     Code,
			"store_id": store.ID,
		}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&customer)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		customer.CreatedByUser, _ = FindUserByID(customer.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		customer.UpdatedByUser, _ = FindUserByID(customer.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		customer.DeletedByUser, _ = FindUserByID(customer.DeletedBy, fields)
	}

	return customer, err
}

func (customer *Customer) IsEmailExists() (exists bool, err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email":    customer.Email,
			"store_id": customer.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email":    customer.Email,
			"store_id": customer.StoreID,
			"_id":      bson.M{"$ne": customer.ID},
		})
	}

	return (count > 0), err
}

func (customer *Customer) IsPhoneExists() (exists bool, err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone":    customer.Phone,
			"store_id": customer.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone":    customer.Phone,
			"store_id": customer.StoreID,
			"_id":      bson.M{"$ne": customer.ID},
		})
	}

	return (count > 0), err
}

func (customer *Customer) IsVatNoExists() (exists bool, err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no":   customer.VATNo,
			"store_id": customer.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"vat_no":   customer.VATNo,
			"store_id": customer.StoreID,
			"_id":      bson.M{"$ne": customer.ID},
		})
	}

	return (count > 0), err
}

func (customer *Customer) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code":     customer.Code,
			"store_id": customer.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code":     customer.Code,
			"store_id": customer.StoreID,
			"_id":      bson.M{"$ne": customer.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) IsCustomerExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id":      ID,
		"store_id": store.ID,
	})

	return (count > 0), err
}

func ProcessCustomers() error {
	log.Printf("Processing customers")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		/*if store.Code != "MBDIT" && store.Code != "MBDI" {
			continue
		}*/

		log.Print("Branch name:" + store.BranchName)

		totalCount, err := store.GetTotalCount(bson.M{"store_id": store.ID}, "customer")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetSort(map[string]interface{}{"created_at": 1})
		findOptions.SetAllowDiskUse(true)

		cur, err := collection.Find(ctx, bson.M{"store_id": store.ID}, findOptions)
		if err != nil {
			return errors.New("Error fetching products" + err.Error())
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
			customer := Customer{}
			err = cur.Decode(&customer)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if customer.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			customer.GenerateSearchWords()
			customer.SetAdditionalkeywords()
			customer.SetSearchLabel()
			customer.Update()

			/*

				  move customers from one store to another
					destinations := []string{"MDNA-SIMULATION", "MDNA", "t1"}

					for _, destinationCode := range destinations {
						destinationStore, err := FindStoreByCode(destinationCode, bson.M{})
						if err != nil && err != mongo.ErrNoDocuments {
							return err
						}

						if destinationStore != nil {
							err = customer.CopyToStore(&destinationStore.ID)
							if err != nil {
								return err
							}
						}
					}
			*/

			/*
				customer.GenerateSearchWords()
				customer.SetSearchLabel()
				customer.SetAdditionalkeywords()
				err = customer.Update()
				if err != nil {
					log.Print("Store ID:" + store.ID.Hex())
					log.Print("Customer Code.:" + customer.Code)
					log.Print("Customer ID:" + customer.ID.Hex())
					continue
					//return err
				}*/

			/*customer.Name = strings.ToUpper(customer.Name)
			customer.Update()
			account, _ := store.FindAccountByID(customer.ID, bson.M{})
			if account != nil {
				account.Name = customer.Name
				account.Update()
			}*/

			/*
				customer.GenerateSearchWords()
				err = customer.Update()
				if err != nil {
					log.Print("Store ID:" + store.ID.Hex())
					log.Print("Code:" + customer.Code)
					log.Print("Name:" + customer.Name)
					continue
					//return err
				}*/

			//customer.SetCreditBalance()
			/*
				if customer.VATNo != "" && govalidator.IsNull(customer.NationalAddress.BuildingNo) {
					customer.NationalAddress.BuildingNo = "1234"
					customer.NationalAddress.StreetName = "test"
					customer.NationalAddress.DistrictName = "test"
					customer.NationalAddress.CityName = "test"
					customer.NationalAddress.ZipCode = "12345"
				}
			*/
			//customer.MakeCode()
			/*
				customer.Code = fmt.Sprintf("%s%0*d", store.CustomerSerialNumber.Prefix, store.CustomerSerialNumber.PaddingCount, i+1)

				err = customer.Update()
				if err != nil {
					return err
				}*/

			bar.Add(1)
		}
	}

	log.Print("DONE!")
	return nil
}

func (store *Store) GetCustomerCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (customer *Customer) MakeCode() error {
	store, err := FindStoreByID(customer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := customer.StoreID.Hex() + "_customer_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetCustomerCount()
		if err != nil {
			return err
		}

		startFrom := store.CustomerSerialNumber.StartFromCount

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

	paddingCount := store.CustomerSerialNumber.PaddingCount
	if store.CustomerSerialNumber.Prefix != "" {
		customer.Code = fmt.Sprintf("%s-%0*d", store.CustomerSerialNumber.Prefix, paddingCount, incr)
	} else {
		customer.Code = fmt.Sprintf("%s%0*d", store.CustomerSerialNumber.Prefix, paddingCount, incr)
	}

	if store.CountryCode != "" {
		timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]
		if ok {
			location, err := time.LoadLocation(timeZone)
			if err != nil {
				return errors.New("error loading location")
			}
			currentDate := time.Now().In(location).Format("20060102") // YYYYMMDD
			customer.Code = strings.ReplaceAll(customer.Code, "DATE", currentDate)
		}
	}
	return nil
}

func (customer *Customer) RestoreCustomer(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = customer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	customer.Deleted = false
	customer.DeletedBy = nil
	customer.DeletedAt = nil

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customer.ID},
		bson.M{"$set": customer},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
