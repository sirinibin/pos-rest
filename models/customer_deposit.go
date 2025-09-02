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
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
)

// CustomerDeposit : CustomerDeposit structure
type CustomerDeposit struct {
	ID   primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Code string             `bson:"code" json:"code"`
	//Amount          float64             `bson:"amount" json:"amount"`
	Description        string              `bson:"description" json:"description"`
	Remarks            string              `bson:"remarks" json:"remarks"`
	BankReferenceNo    string              `bson:"bank_reference_no" json:"bank_reference_no"`
	Date               *time.Time          `bson:"date" json:"date"`
	DateStr            string              `json:"date_str,omitempty" bson:"-"`
	CustomerID         *primitive.ObjectID `json:"customer_id" bson:"customer_id"`
	Customer           *Customer           `json:"customer" bson:"-"`
	CustomerName       string              `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic string              `json:"customer_name_arabic" bson:"customer_name_arabic"`
	Type               string              `bson:"type" json:"type"`
	VendorID           *primitive.ObjectID `json:"vendor_id" bson:"vendor_id"`
	Vendor             *Vendor             `json:"vendor" bson:"-"`
	VendorName         string              `json:"vendor_name" bson:"vendor_name"`
	VendorNameArabic   string              `json:"vendor_name_arabic" bson:"vendor_name_arabic"`
	PaymentMethod      string              `json:"payment_method" bson:"payment_method"`
	StoreID            *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName          string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode          string              `json:"store_code,omitempty" bson:"store_code,omitempty"`
	Images             []string            `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent      []string            `json:"images_content,omitempty"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser      *User               `json:"created_by_user,omitempty"`
	UpdatedByUser      *User               `json:"updated_by_user,omitempty"`
	CategoryName       []string            `json:"category_name" bson:"category_name"`
	CreatedByName      string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName      string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Deleted            bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy          *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser      *User               `json:"deleted_by_user,omitempty"`
	DeletedAt          *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	Payments           []ReceivablePayment `bson:"payments" json:"payments"`
	NetTotal           float64             `bson:"net_total" json:"net_total"`
	PaymentMethods     []string            `json:"payment_methods" bson:"payment_methods"`
}

type ReceivablePayment struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date          *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr       string              `json:"date_str,omitempty" bson:"-"`
	Amount        float64             `json:"amount" bson:"amount"`
	Discount      float64             `bson:"discount" json:"discount"`
	Method        string              `json:"method" bson:"method"`
	BankReference *string             `json:"bank_reference" bson:"bank_reference"`
	Description   *string             `json:"description" bson:"description"`
	InvoiceID     *primitive.ObjectID `json:"invoice_id" bson:"invoice_id"`
	InvoiceCode   *string             `json:"invoice_code" bson:"invoice_code"`
	InvoiceType   *string             `json:"invoice_type" bson:"invoice_type"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id" bson:"store_id"`
	StoreName     string              `json:"store_name" bson:"store_name"`
}

func (model *CustomerDeposit) SetPostBalances() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(model.ID, *model.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("Error finding ledger by reference id: " + err.Error())
	}

	if err == mongo.ErrNoDocuments {
		return nil
	}

	err = ledger.SetPostBalancesByLedger(model.Date)
	if err != nil {
		return err
	}

	return nil
}

func (customerdeposit *CustomerDeposit) HandleDeletedPayments(customerdepositOld *CustomerDeposit) error {
	store, _ := FindStoreByID(customerdeposit.StoreID, bson.M{})

	for _, oldPayment := range customerdepositOld.Payments {
		found := false
		deletePayment := false
		for _, payment := range customerdeposit.Payments {
			if payment.ID.Hex() == oldPayment.ID.Hex() {
				found = true
				if (payment.InvoiceID == nil || payment.InvoiceID.IsZero()) && (oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero()) {
					deletePayment = true
				}
				break
			}
		} //end for2

		if !found || deletePayment {
			if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && *oldPayment.InvoiceType == "sales" {
				order, err := store.FindOrderByID(oldPayment.InvoiceID, bson.M{})
				if err != nil {
					return err
				}
				err = order.DeletePaymentsByReceivablePaymentID(oldPayment.ID)
				if err != nil {
					return err
				}

				_, err = order.SetPaymentStatus()
				if err != nil {
					return err
				}

				err = order.Update()
				if err != nil {
					return err
				}

				err = order.UndoAccounting()
				if err != nil {
					return err
				}

				err = order.DoAccounting()
				if err != nil {
					return err
				}

				err = order.SetCustomerSalesStats()
				if err != nil {
					return err
				}
			}

			if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && *oldPayment.InvoiceType == "quotation_sales" {
				quotation, err := store.FindQuotationByID(oldPayment.InvoiceID, bson.M{})
				if err != nil {
					return err
				}
				err = quotation.DeletePaymentsByReceivablePaymentID(oldPayment.ID)
				if err != nil {
					return err
				}

				_, err = quotation.SetPaymentStatus()
				if err != nil {
					return err
				}

				err = quotation.Update()
				if err != nil {
					return err
				}

				err = quotation.UndoAccounting()
				if err != nil {
					return err
				}

				err = quotation.DoAccounting()
				if err != nil {
					return err
				}

				err = quotation.SetCustomerQuotationStats()
				if err != nil {
					return err
				}
			}

			if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && *oldPayment.InvoiceType == "purchase_return" {
				purchaseReturn, err := store.FindPurchaseReturnByID(oldPayment.InvoiceID, bson.M{})
				if err != nil {
					return err
				}
				err = purchaseReturn.DeletePaymentsByReceivablePaymentID(oldPayment.ID)
				if err != nil {
					return err
				}

				_, err = purchaseReturn.SetPaymentStatus()
				if err != nil {
					return err
				}

				err = purchaseReturn.Update()
				if err != nil {
					return err
				}

				err = purchaseReturn.UndoAccounting()
				if err != nil {
					return err
				}

				err = purchaseReturn.DoAccounting()
				if err != nil {
					return err
				}

				err = purchaseReturn.SetVendorPurchaseReturnStats()
				if err != nil {
					return err
				}
			}
		}
	} //end for1
	return nil
}

func (customerdeposit *CustomerDeposit) CloseSalesPayments() error {
	store, _ := FindStoreByID(customerdeposit.StoreID, bson.M{})

	for _, receivablePayment := range customerdeposit.Payments {
		if receivablePayment.InvoiceID != nil && !receivablePayment.InvoiceID.IsZero() && *receivablePayment.InvoiceType == "sales" {
			order, _ := store.FindOrderByID(receivablePayment.InvoiceID, bson.M{})
			err := order.UpdatePaymentFromReceivablePayment(receivablePayment, customerdeposit)
			if err != nil {
				return errors.New("error updating sales payment from receivable payment: " + err.Error())
			}

			_, err = order.SetPaymentStatus()
			if err != nil {
				return errors.New("error setting payment status: " + err.Error())
			}

			err = order.Update()
			if err != nil {
				return errors.New("error updating order inside payment status: " + err.Error())
			}

			err = order.SetCustomerSalesStats()
			if err != nil {
				return err
			}

			err = order.UndoAccounting()
			if err != nil {
				return err
			}

			err = order.DoAccounting()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (customerdeposit *CustomerDeposit) ClosePurchaseReturnPayments() error {
	store, _ := FindStoreByID(customerdeposit.StoreID, bson.M{})

	for _, receivablePayment := range customerdeposit.Payments {
		if receivablePayment.InvoiceID != nil && !receivablePayment.InvoiceID.IsZero() && *receivablePayment.InvoiceType == "purchase_return" {
			purchaseReturn, _ := store.FindPurchaseReturnByID(receivablePayment.InvoiceID, bson.M{})
			err := purchaseReturn.UpdatePaymentFromReceivablePayment(receivablePayment, customerdeposit)
			if err != nil {
				return errors.New("error updating sales payment from receivable payment: " + err.Error())
			}

			_, err = purchaseReturn.SetPaymentStatus()
			if err != nil {
				return errors.New("error setting payment status: " + err.Error())
			}

			err = purchaseReturn.Update()
			if err != nil {
				return errors.New("error updating order inside payment status: " + err.Error())
			}

			err = purchaseReturn.SetVendorPurchaseReturnStats()
			if err != nil {
				return err
			}

			err = purchaseReturn.UndoAccounting()
			if err != nil {
				return err
			}

			err = purchaseReturn.DoAccounting()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (customerdeposit *CustomerDeposit) CloseQuotationSalesPayments() error {
	store, _ := FindStoreByID(customerdeposit.StoreID, bson.M{})

	for _, receivablePayment := range customerdeposit.Payments {
		if receivablePayment.InvoiceID != nil && !receivablePayment.InvoiceID.IsZero() && *receivablePayment.InvoiceType == "quotation_sales" {
			quotation, _ := store.FindQuotationByID(receivablePayment.InvoiceID, bson.M{})
			err := quotation.UpdatePaymentFromReceivablePayment(receivablePayment, customerdeposit)
			if err != nil {
				return errors.New("error updating sales payment from receivable payment: " + err.Error())
			}

			_, err = quotation.SetPaymentStatus()
			if err != nil {
				return errors.New("error setting payment status: " + err.Error())
			}

			err = quotation.Update()
			if err != nil {
				return errors.New("error updating order inside payment status: " + err.Error())
			}

			err = quotation.SetCustomerQuotationStats()
			if err != nil {
				return err
			}

			err = quotation.UndoAccounting()
			if err != nil {
				return err
			}

			err = quotation.DoAccounting()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (customerdeposit *CustomerDeposit) FindNetTotal() {
	netTotal := float64(0.00)
	paymentMethods := []string{}

	for _, payment := range customerdeposit.Payments {
		amount := payment.Amount
		if payment.Discount > 0 {
			amount -= payment.Discount
		}

		netTotal += amount

		if !slices.Contains(paymentMethods, payment.Method) {
			paymentMethods = append(paymentMethods, payment.Method)
		}
	}
	customerdeposit.NetTotal = RoundTo2Decimals(netTotal)
	customerdeposit.PaymentMethods = paymentMethods
}

func (customerdeposit *CustomerDeposit) AttributesValueChangeEvent(customerdepositOld *CustomerDeposit) error {

	return nil
}

func (customerdeposit *CustomerDeposit) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(customerdeposit.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customerdeposit.CategoryName = []string{}

	if customerdeposit.CustomerID != nil && !customerdeposit.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(customerdeposit.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customerdeposit.CustomerName = customer.Name
	}

	if customerdeposit.VendorID != nil && !customerdeposit.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(customerdeposit.VendorID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customerdeposit.VendorName = vendor.Name
	}

	if customerdeposit.CreatedBy != nil {
		createdByUser, err := FindUserByID(customerdeposit.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		customerdeposit.CreatedByName = createdByUser.Name
	}

	if customerdeposit.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(customerdeposit.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		customerdeposit.UpdatedByName = updatedByUser.Name
	}

	if customerdeposit.DeletedBy != nil && !customerdeposit.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(customerdeposit.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		customerdeposit.DeletedByName = deletedByUser.Name
	}

	if customerdeposit.StoreID != nil {
		store, err := FindStoreByID(customerdeposit.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		customerdeposit.StoreName = store.Name
		customerdeposit.StoreCode = store.Code
	}

	return nil
}

type CustomerDepositStats struct {
	ID    *primitive.ObjectID `json:"id" bson:"_id"`
	Total float64             `json:"total" bson:"total"`
}

func (store *Store) GetCustomerDepositStats(filter map[string]interface{}) (stats CustomerDepositStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":   nil,
				"total": bson.M{"$sum": "$net_total"},
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

func (store *Store) SearchCustomerDeposit(w http.ResponseWriter, r *http.Request) (customerdeposits []CustomerDeposit, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[type]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] != "" {
			criterias.SearchBy["type"] = keys[0]
		}
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customerdeposits, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["amount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["amount"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return customerdeposits, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[payment_methods]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethods := strings.Split(keys[0], ",")
		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_methods"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[payment_method]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["payment_method"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

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

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customerdeposits, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {

		vendorIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customerdeposits, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["vendor_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customerdeposits, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return customerdeposits, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	var startDate time.Time
	var endDate time.Time

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return customerdeposits, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return customerdeposits, criterias, err
		}

		if timeZoneOffset != 0 {
			endDate = ConvertTimeZoneToUTC(timeZoneOffset, endDate)
		}

		endDate = endDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
	}

	if !startDate.IsZero() && !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	} else if !startDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate}
	} else if !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$lte": endDate}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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
			return customerdeposits, criterias, err
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

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return customerdeposits, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
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
		return customerdeposits, criterias, errors.New("Error fetching customerdeposits:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return customerdeposits, criterias, errors.New("Cursor error:" + err.Error())
		}
		customerdeposit := CustomerDeposit{}
		err = cur.Decode(&customerdeposit)
		if err != nil {
			return customerdeposits, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			customerdeposit.CreatedByUser, _ = FindUserByID(customerdeposit.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			customerdeposit.UpdatedByUser, _ = FindUserByID(customerdeposit.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			customerdeposit.DeletedByUser, _ = FindUserByID(customerdeposit.DeletedBy, deletedByUserSelectFields)
		}

		customerdeposits = append(customerdeposits, customerdeposit)
	} //end for loop

	return customerdeposits, criterias, nil

}

func (customerDeposit *CustomerDeposit) Validate(w http.ResponseWriter, r *http.Request, scenario string, customerDepositOld *CustomerDeposit) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(customerDeposit.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if customerDeposit.Type == "customer" && (customerDeposit.CustomerID == nil || customerDeposit.CustomerID.IsZero()) {
		errs["customer_id"] = "Customer is required"
	}

	if customerDeposit.Type == "vendor" && (customerDeposit.VendorID == nil || customerDeposit.VendorID.IsZero()) {
		errs["vendor_id"] = "Vendor is required"
	}

	if govalidator.IsNull(customerDeposit.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		//const shortForm = "Jan 02 2006"
		//const shortForm = "	January 02, 2006T3:04PM"
		//from js:Thu Apr 14 2022 03:53:15 GMT+0300 (Arabian Standard Time)
		//	const shortForm = "Monday Jan 02 2006 15:04:05 GMT-0700 (MST)"
		//const shortForm = "Mon Jan 02 2006 15:04:05 GMT-0700 (MST)"
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, customerDeposit.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		customerDeposit.Date = &date
	}

	for index, payment := range customerDeposit.Payments {
		if payment.ID.IsZero() {
			customerDeposit.Payments[index].ID = primitive.NewObjectID()
		}

		if govalidator.IsNull(payment.DateStr) {
			errs["customer_receivable_payment_date_"+strconv.Itoa(index)] = "Payment date is required"
		} else {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			date, err := time.Parse(shortForm, payment.DateStr)
			if err != nil {
				errs["customer_receivable_payment_date_"+strconv.Itoa(index)] = "Invalid date format"
			}

			customerDeposit.Payments[index].Date = &date
			payment.Date = &date

			if customerDeposit.Date != nil && IsAfter(customerDeposit.Date, customerDeposit.Payments[index].Date) {
				errs["customer_receivable_payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to Receivable date time"
			}
		}

		if payment.Amount == 0 {
			errs["customer_receivable_payment_amount_"+strconv.Itoa(index)] = "Payment amount is required"
		} else if payment.Amount <= 0 {
			errs["customer_receivable_payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
		}

		if payment.Amount < payment.Discount {
			errs["customer_receivable_payment_discount_"+strconv.Itoa(index)] = "Payment discount should not be grater than amount"
		}

		if payment.Method == "" {
			errs["customer_receivable_payment_method_"+strconv.Itoa(index)] = "Payment method is required"
		}

		if payment.InvoiceID != nil && !payment.InvoiceID.IsZero() && *payment.InvoiceType == "sales" {
			order, err := store.FindOrderByID(payment.InvoiceID, bson.M{})
			if err != nil {
				errs["customer_receivable_payment_invoice_"+strconv.Itoa(index)] = "invalid invoice: " + err.Error()
			}

			if order.CustomerID != nil &&
				!order.CustomerID.IsZero() &&
				customerDeposit.CustomerID != nil &&
				!customerDeposit.CustomerID.IsZero() &&
				order.CustomerID.Hex() != customerDeposit.CustomerID.Hex() {
				errs["customer_receivable_payment_invoice_"+strconv.Itoa(index)] = "Invoice is not belongs to the selected customer"
			}

			orderBalanceAmount := order.BalanceAmount

			if scenario == "update" {
				oldTotalInvoicePaidAmount := float64(0.00)
				for _, oldPayment := range customerDepositOld.Payments {
					if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && oldPayment.InvoiceID.Hex() == order.ID.Hex() {
						oldTotalInvoicePaidAmount += oldPayment.Amount
					}
				}
				orderBalanceAmount += oldTotalInvoicePaidAmount
			}

			if RoundTo2Decimals(payment.Amount-payment.Discount) > orderBalanceAmount {
				errs["customer_receivable_payment_amount_"+strconv.Itoa(index)] = "Payment amount (-discount) should not be greater than " + fmt.Sprintf("%.02f", orderBalanceAmount) + " (Invoice Balance)"
			}

			totalInvoicePaidAmount := float64(0.00)
			for index2, payment2 := range customerDeposit.Payments {
				if payment2.InvoiceID != nil && !payment2.InvoiceID.IsZero() && payment2.InvoiceID.Hex() == order.ID.Hex() {
					if (orderBalanceAmount - totalInvoicePaidAmount) == 0 {
						errs["customer_receivable_payment_amount_"+strconv.Itoa(index2)] = "Payment is already closed for this invoice"
						break
					} else if (totalInvoicePaidAmount + payment2.Amount) > orderBalanceAmount {
						errs["customer_receivable_payment_amount_"+strconv.Itoa(index2)] = "Payment amount should not be greater than " + fmt.Sprintf("%.02f", (orderBalanceAmount-totalInvoicePaidAmount)) + " (Invoice Balance)"
						break
					}
					totalInvoicePaidAmount += payment2.Amount
				}
			}
		}

		if payment.InvoiceID != nil && !payment.InvoiceID.IsZero() && *payment.InvoiceType == "quotation_sales" {
			quotation, err := store.FindQuotationByID(payment.InvoiceID, bson.M{})
			if err != nil {
				errs["customer_receivable_payment_invoice_"+strconv.Itoa(index)] = "invalid invoice: " + err.Error()
			}

			if quotation.CustomerID != nil &&
				!quotation.CustomerID.IsZero() &&
				customerDeposit.CustomerID != nil &&
				!customerDeposit.CustomerID.IsZero() &&
				quotation.CustomerID.Hex() != customerDeposit.CustomerID.Hex() {
				errs["customer_receivable_payment_invoice_"+strconv.Itoa(index)] = "Invoice is not belongs to the selected customer"
			}

			quotationBalanceAmount := quotation.BalanceAmount

			if scenario == "update" {
				oldTotalInvoicePaidAmount := float64(0.00)
				for _, oldPayment := range customerDepositOld.Payments {
					if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && oldPayment.InvoiceID.Hex() == quotation.ID.Hex() {
						oldTotalInvoicePaidAmount += oldPayment.Amount
					}
				}
				quotationBalanceAmount += oldTotalInvoicePaidAmount
			}

			if RoundTo2Decimals(payment.Amount-payment.Discount) > quotationBalanceAmount {
				errs["customer_receivable_payment_amount_"+strconv.Itoa(index)] = "Payment amount (Amount - Discount) should not be greater than " + fmt.Sprintf("%.02f", quotation.BalanceAmount) + " (Invoice Balance)"
			}

			totalInvoicePaidAmount := float64(0.00)
			for index2, payment2 := range customerDeposit.Payments {
				if payment2.InvoiceID != nil && !payment2.InvoiceID.IsZero() && payment2.InvoiceID.Hex() == quotation.ID.Hex() {
					if (quotationBalanceAmount - totalInvoicePaidAmount) == 0 {
						errs["customer_receivable_payment_amount_"+strconv.Itoa(index2)] = "Payment is already closed for this invoice"
						break
					} else if (totalInvoicePaidAmount + payment2.Amount) > quotationBalanceAmount {
						errs["customer_receivable_payment_amount_"+strconv.Itoa(index2)] = "Payment amount should not be greater than " + fmt.Sprintf("%.02f", (quotationBalanceAmount-totalInvoicePaidAmount)) + " (Invoice Balance)"
						break
					}
					totalInvoicePaidAmount += payment2.Amount
				}
			}
		}

		if payment.InvoiceID != nil && !payment.InvoiceID.IsZero() && *payment.InvoiceType == "purchase_return" {
			purchaseReturn, err := store.FindPurchaseReturnByID(payment.InvoiceID, bson.M{})
			if err != nil {
				errs["customer_receivable_payment_invoice_"+strconv.Itoa(index)] = "invalid invoice: " + err.Error()
			}

			if purchaseReturn.VendorID != nil &&
				!purchaseReturn.VendorID.IsZero() &&
				customerDeposit.VendorID != nil &&
				!customerDeposit.VendorID.IsZero() &&
				purchaseReturn.VendorID.Hex() != customerDeposit.VendorID.Hex() {
				errs["customer_receivable_payment_invoice_"+strconv.Itoa(index)] = "Invoice is not belongs to the selected vendor"
			}

			purchaseReturnBalanceAmount := purchaseReturn.BalanceAmount

			if scenario == "update" {
				oldTotalInvoicePaidAmount := float64(0.00)
				for _, oldPayment := range customerDepositOld.Payments {
					if oldPayment.InvoiceID != nil && !oldPayment.InvoiceID.IsZero() && oldPayment.InvoiceID.Hex() == purchaseReturn.ID.Hex() {
						oldTotalInvoicePaidAmount += oldPayment.Amount
					}
				}
				purchaseReturnBalanceAmount += oldTotalInvoicePaidAmount
			}

			if RoundTo2Decimals(payment.Amount-payment.Discount) > purchaseReturnBalanceAmount {
				errs["customer_receivable_payment_amount_"+strconv.Itoa(index)] = "Payment amount(Amount - Discount) should not be greater than " + fmt.Sprintf("%.02f", purchaseReturn.BalanceAmount) + " (Invoice Balance)"
			}

			totalInvoicePaidAmount := float64(0.00)
			for index2, payment2 := range customerDeposit.Payments {
				if payment2.InvoiceID != nil && !payment2.InvoiceID.IsZero() && payment2.InvoiceID.Hex() == purchaseReturn.ID.Hex() {
					if (purchaseReturnBalanceAmount - totalInvoicePaidAmount) == 0 {
						errs["customer_receivable_payment_amount_"+strconv.Itoa(index2)] = "Payment is already closed for this invoice"
						break
					} else if (totalInvoicePaidAmount + payment2.Amount) > purchaseReturnBalanceAmount {
						errs["customer_receivable_payment_amount_"+strconv.Itoa(index2)] = "Payment amount should not be greater than " + fmt.Sprintf("%.02f", (purchaseReturnBalanceAmount-totalInvoicePaidAmount)) + " (Invoice Balance)"
						break
					}
					totalInvoicePaidAmount += payment2.Amount
				}
			}
		}

	} //end for

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (model *CustomerDeposit) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if model.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code":     model.Code,
			"store_id": model.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code":     model.Code,
			"store_id": model.StoreID,
			"_id":      bson.M{"$ne": model.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) FindLastCustomerDeposit(
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

func (store *Store) FindLastCustomerDepositByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

func (store *Store) GetCustomerDepositCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}
func (customerDeposit *CustomerDeposit) MakeRedisCode() error {
	store, err := FindStoreByID(customerDeposit.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := customerDeposit.StoreID.Hex() + "_customer_deposit_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from order.CreatedAt or fallback to order.Date or now ===
	baseTime := customerDeposit.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("customerdeposit")
		if err != nil {
			return err
		}
		startFrom := store.CustomerDepositSerialNumber.StartFromCount
		err = db.RedisClient.Set(redisKey, startFrom+count-1, 0).Err()
		if err != nil {
			return err
		}
	}

	// === 4. Increment global counter ===
	globalIncr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	// === 5. Determine which counter to use for order.Code ===
	useMonthly := strings.Contains(store.CustomerDepositSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := customerDeposit.StoreID.Hex() + "_customer_deposit_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.CustomerDepositSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "customerdeposit")
			if err != nil {
				return err
			}

			if monthlyCount == 0 {
				err = db.RedisClient.Set(monthlyRedisKey, startFrom+monthlyCount-1, 0).Err()
				if err != nil {
					return err
				}
			} else {
				err = db.RedisClient.Set(monthlyRedisKey, (globalIncr - 1), 0).Err()
				if err != nil {
					return err
				}
			}
		}

		// Increment monthly counter and use it
		monthlyIncr, err := db.RedisClient.Incr(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if store.Settings.EnableMonthlySerialNumber {
			serialNumber = monthlyIncr
		}
	}

	// === 6. Build the code ===
	paddingCount := store.CustomerDepositSerialNumber.PaddingCount
	if store.CustomerDepositSerialNumber.Prefix != "" {
		customerDeposit.Code = fmt.Sprintf("%s-%0*d", store.CustomerDepositSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		customerDeposit.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(customerDeposit.Code, "DATE") {
		orderDate := baseTime.Format("20060102") // YYYYMMDD
		customerDeposit.Code = strings.ReplaceAll(customerDeposit.Code, "DATE", orderDate)
	}

	return nil
}

func (customerDeposit *CustomerDeposit) UnMakeRedisCode() error {
	store, err := FindStoreByID(customerDeposit.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := customerDeposit.StoreID.Hex() + "_customer_deposit_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := customerDeposit.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.CustomerDepositSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := customerDeposit.StoreID.Hex() + "_customer_deposit_counter_" + monthKey
		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
func (model *CustomerDeposit) MakeCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_customer_deposit_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetCustomerDepositCount()
		if err != nil {
			return err
		}

		startFrom := store.CustomerDepositSerialNumber.StartFromCount

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

	paddingCount := store.CustomerDepositSerialNumber.PaddingCount

	if store.CustomerDepositSerialNumber.Prefix != "" {
		model.Code = fmt.Sprintf("%s-%0*d", store.CustomerDepositSerialNumber.Prefix, paddingCount, incr)
	} else {
		model.Code = fmt.Sprintf("%s%0*d", store.CustomerDepositSerialNumber.Prefix, paddingCount, incr)
	}

	if store.CountryCode != "" {
		timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]
		if ok {
			location, err := time.LoadLocation(timeZone)
			if err != nil {
				return errors.New("error loading location")
			}
			if model.Date != nil {
				currentDate := model.Date.In(location).Format("20060102") // YYYYMMDD
				model.Code = strings.ReplaceAll(model.Code, "DATE", currentDate)
			}
		}
	}

	return nil
}
*/

/*
func (customerdeposit *CustomerDeposit) MakeCode() error {
	store, err := FindStoreByID(customerdeposit.StoreID, bson.M{})
	if err != nil {
		return err
	}

	lastCustomerDeposit, err := store.FindLastCustomerDepositByStoreID(customerdeposit.StoreID, bson.M{})
	if err != nil && mongo.ErrNoDocuments != err {
		return err
	}

	if lastCustomerDeposit == nil {
		store, err := FindStoreByID(customerdeposit.StoreID, bson.M{})
		if err != nil {
			return err
		}
		customerdeposit.Code = store.Code + "-100000"
	} else {
		splits := strings.Split(lastCustomerDeposit.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			customerdeposit.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := customerdeposit.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastCustomerDeposit.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		customerdeposit.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}*/

func (customerdeposit *CustomerDeposit) Insert() (err error) {
	collection := db.GetDB("store_" + customerdeposit.StoreID.Hex()).Collection("customerdeposit")
	customerdeposit.ID = primitive.NewObjectID()

	err = customerdeposit.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &customerdeposit)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func (customerdeposit *CustomerDeposit) SaveImages() error {

	for _, imageContent := range customerdeposit.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		filename := "images/customer_deposits/" + GenerateFileName("customerdeposit_", extension)
		err = SaveBase64File(filename, content)
		if err != nil {
			return err
		}
		customerdeposit.Images = append(customerdeposit.Images, "/"+filename)
	}

	customerdeposit.ImagesContent = []string{}

	return nil
}

func (customerdeposit *CustomerDeposit) Update() error {
	collection := db.GetDB("store_" + customerdeposit.StoreID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	if len(customerdeposit.ImagesContent) > 0 {
		err := customerdeposit.SaveImages()
		if err != nil {
			return err
		}
	}

	err := customerdeposit.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customerdeposit.ID},
		bson.M{"$set": customerdeposit},
		updateOptions,
	)
	return err
}

func (customerdeposit *CustomerDeposit) DeleteCustomerDeposit(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + customerdeposit.StoreID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = customerdeposit.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	customerdeposit.Deleted = true
	customerdeposit.DeletedBy = &userID
	now := time.Now()
	customerdeposit.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customerdeposit.ID},
		bson.M{"$set": customerdeposit},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindCustomerDepositByCode(
	code string,
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
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
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

func (store *Store) FindCustomerDepositByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customerdeposit *CustomerDeposit, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
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
		Decode(&customerdeposit)
	if err != nil {
		return nil, err
	}

	return customerdeposit, err
}

/*
func (customerdeposit *CustomerDeposit) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + customerdeposit.StoreID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customerdeposit.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": customerdeposit.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": customerdeposit.Code,
			"_id":  bson.M{"$ne": customerdeposit.ID},
		})
	}

	return (count > 0), err
}*/

func (store *Store) IsCustomerDepositExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func ProcessCustomerDeposits() error {
	log.Printf("Processing customer deposits")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}
	for _, store := range stores {
		log.Print("Store: " + store.Name)
		totalCount, err := store.GetTotalCount(bson.M{"store_id": store.ID}, "customerdeposit")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("customerdeposit")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)
		findOptions.SetSort(bson.M{"date": 1})

		bar := progressbar.Default(totalCount)
		cur, err := collection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			return errors.New("Error fetching customerdeposits" + err.Error())
		}
		if cur != nil {
			defer cur.Close(ctx)
		}

		for i := 0; cur != nil && cur.Next(ctx); i++ {
			err := cur.Err()
			if err != nil {
				return errors.New("Cursor error:" + err.Error())
			}
			customerdeposit := CustomerDeposit{}
			err = cur.Decode(&customerdeposit)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			customerdeposit.UndoAccounting()
			customerdeposit.DoAccounting()

			if customerdeposit.CustomerID != nil && !customerdeposit.CustomerID.IsZero() {
				store, _ := FindStoreByID(customerdeposit.StoreID, bson.M{})
				if store != nil {
					customer, _ := store.FindCustomerByID(customerdeposit.CustomerID, bson.M{})
					if customer != nil {
						customer.SetCreditBalance()
					}
				}
			}

			if customerdeposit.VendorID != nil && !customerdeposit.VendorID.IsZero() {
				store, _ := FindStoreByID(customerdeposit.StoreID, bson.M{})
				if store != nil {
					vendor, _ := store.FindVendorByID(customerdeposit.VendorID, bson.M{})
					if vendor != nil {
						vendor.SetCreditBalance()
					}
				}
			}

			/*
				customerdeposit.CloseSalesPayments()
				customerdeposit.ClosePurchaseReturnPayments()
				customerdeposit.CloseQuotationSalesPayments()
			*/

			/*
				model.UndoAccounting()
				model.DoAccounting()
				if model.CustomerID != nil && !model.CustomerID.IsZero() {
					customer, _ := store.FindCustomerByID(model.CustomerID, bson.M{})
					if customer != nil {
						customer.SetCreditBalance()
					}
				}

				if model.VendorID != nil && !model.VendorID.IsZero() {
					vendor, _ := store.FindVendorByID(model.VendorID, bson.M{})
					if vendor != nil {
						vendor.SetCreditBalance()
					}
				}*/

			//model.Type = "customer"
			//model.Update()
			/*
				model.UndoAccounting()
				model.DoAccounting()
				if model.CustomerID != nil && !model.CustomerID.IsZero() {
					customer, _ := store.FindCustomerByID(model.CustomerID, bson.M{})
					if customer != nil {
						customer.SetCreditBalance()
					}
				}*/

			/*
				if len(model.Payments) == 0 {
					model.Payments = []ReceivablePayment{
						ReceivablePayment{
							Amount:        &model.Amount,
							Date:          model.Date,
							Method:        model.PaymentMethod,
							BankReference: &model.BankReferenceNo,
							Description:   &model.Description,
						},
					}
				}

				model.FindNetTotal()

				err = model.Update()
				if err != nil {
					log.Print("Error updating: " + model.Code + ", err: " + err.Error())
					//return err
				}
			*/
			bar.Add(1)
		}
	}
	log.Print("DONE!")
	return nil
}

func (customerDeposit *CustomerDeposit) AdjustPayments() error {
	if len(customerDeposit.Payments) == 0 || customerDeposit.Date == nil {
		return nil
	}

	// 1. Ensure first payment is at least 1 minute after customerDeposit.Date if they are the same
	firstPayment := customerDeposit.Payments[0]
	if firstPayment.Date != nil && firstPayment.Date.Equal(*customerDeposit.Date) {
		newTime := customerDeposit.Date.Add(1 * time.Minute)
		customerDeposit.Payments[0].Date = &newTime
	}

	// 2. For each subsequent payment, ensure strictly increasing by at least 1 minute
	for i := 1; i < len(customerDeposit.Payments); i++ {
		prev := customerDeposit.Payments[i-1].Date
		curr := customerDeposit.Payments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			customerDeposit.Payments[i].Date = &newTime
		}
	}

	err := customerDeposit.Update()
	if err != nil {
		return err
	}

	return nil
}

func (customerDeposit *CustomerDeposit) DoAccounting() error {
	err := customerDeposit.AdjustPayments()
	if err != nil {
		return errors.New("error adjusting payments: " + err.Error())
	}

	ledgers, err := customerDeposit.CreateLedger()
	if err != nil {
		return err
	}

	for _, ledger := range ledgers {
		_, err = ledger.CreatePostings()
		if err != nil {
			return err
		}
	}

	return nil
}

func (customerDeposit *CustomerDeposit) UndoAccounting() error {
	store, err := FindStoreByID(customerDeposit.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledgers, err := store.FindLedgersByReferenceID(customerDeposit.ID, *customerDeposit.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	var relatedLedgerAccounts map[string]Account
	for _, ledger := range ledgers {
		ledgerAccounts, err := ledger.GetRelatedAccounts()
		if err != nil {
			return err
		}
		relatedLedgerAccounts = MergeAccountMaps(relatedLedgerAccounts, ledgerAccounts)
	}

	err = store.RemoveLedgerByReferenceID(customerDeposit.ID)
	if err != nil {
		return err
	}

	err = store.RemovePostingsByReferenceID(customerDeposit.ID)
	if err != nil {
		return err
	}

	err = SetAccountBalances(relatedLedgerAccounts)
	if err != nil {
		return err
	}

	return nil
}

func (customerDeposit *CustomerDeposit) CreateLedger() (ledgers []Ledger, err error) {
	store, err := FindStoreByID(customerDeposit.StoreID, bson.M{})
	if err != nil {
		return ledgers, err
	}

	now := time.Now()
	var customer *Customer
	var vendor *Vendor

	if customerDeposit.Type == "customer" && customerDeposit.CustomerID != nil && !customerDeposit.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(customerDeposit.CustomerID, bson.M{})
		if err != nil {
			return ledgers, err
		}
	}

	if customerDeposit.Type == "vendor" && customerDeposit.VendorID != nil && !customerDeposit.VendorID.IsZero() {
		vendor, err = store.FindVendorByID(customerDeposit.VendorID, bson.M{})
		if err != nil {
			return ledgers, err
		}
	}

	if vendor == nil && customer == nil {
		return ledgers, err
	}

	referenceModel := ""

	var sendingAccount *Account

	if customerDeposit.Type == "customer" && customer != nil {
		referenceModel = "customer"
		customerAccount, err := store.CreateAccountIfNotExists(
			customerDeposit.StoreID,
			&customer.ID,
			&referenceModel,
			customer.Name,
			&customer.Phone,
			&customer.VATNo,
		)
		if err != nil {
			return ledgers, err
		}
		sendingAccount = customerAccount
	} else if customerDeposit.Type == "vendor" && vendor != nil {
		referenceModel = "vendor"
		vendorAccount, err := store.CreateAccountIfNotExists(
			customerDeposit.StoreID,
			&vendor.ID,
			&referenceModel,
			vendor.Name,
			&vendor.Phone,
			&vendor.VATNo,
		)
		if err != nil {
			return ledgers, err
		}
		sendingAccount = vendorAccount
	}

	cashAccount, err := store.CreateAccountIfNotExists(customerDeposit.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return ledgers, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(customerDeposit.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return ledgers, err
	}

	cashDiscountAllowedAccount, err := store.CreateAccountIfNotExists(customerDeposit.StoreID, nil, nil, "Cash discount allowed", nil, nil)
	if err != nil {
		return nil, err
	}

	for _, payment := range customerDeposit.Payments {
		journals := []Journal{}

		receivingAccount := Account{}
		if payment.Method == "cash" {
			receivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			receivingAccount = *bankAccount
		}

		groupID := primitive.NewObjectID()

		journals = append(journals, Journal{
			Date:           payment.Date,
			AccountID:      receivingAccount.ID,
			AccountNumber:  receivingAccount.Number,
			AccountName:    receivingAccount.Name,
			DebitOrCredit:  "debit",
			Debit:          RoundTo2Decimals(payment.Amount - payment.Discount),
			GroupID:        groupID,
			CreatedAt:      &now,
			UpdatedAt:      &now,
			ReferenceID:    payment.InvoiceID,
			ReferenceModel: payment.InvoiceType,
			ReferenceCode:  payment.InvoiceCode,
		})

		if payment.Discount > 0 {
			journals = append(journals, Journal{
				Date:           payment.Date,
				AccountID:      cashDiscountAllowedAccount.ID,
				AccountNumber:  cashDiscountAllowedAccount.Number,
				AccountName:    cashDiscountAllowedAccount.Name,
				DebitOrCredit:  "debit",
				Debit:          RoundTo2Decimals(payment.Discount),
				GroupID:        groupID,
				CreatedAt:      &now,
				UpdatedAt:      &now,
				ReferenceID:    payment.InvoiceID,
				ReferenceModel: payment.InvoiceType,
				ReferenceCode:  payment.InvoiceCode,
			})
		}

		journals = append(journals, Journal{
			Date:           payment.Date,
			AccountID:      sendingAccount.ID,
			AccountNumber:  sendingAccount.Number,
			AccountName:    sendingAccount.Name,
			DebitOrCredit:  "credit",
			Credit:         RoundTo2Decimals(payment.Amount),
			GroupID:        groupID,
			CreatedAt:      &now,
			UpdatedAt:      &now,
			ReferenceID:    payment.InvoiceID,
			ReferenceModel: payment.InvoiceType,
			ReferenceCode:  payment.InvoiceCode,
		})

		referenceModel = ""
		if customerDeposit.Type == "customer" {
			referenceModel = "customer_deposit"
		} else if customerDeposit.Type == "vendor" {
			referenceModel = "vendor_deposit"
		}

		ledger := &Ledger{
			StoreID:        customerDeposit.StoreID,
			ReferenceID:    customerDeposit.ID,
			ReferenceModel: referenceModel,
			ReferenceCode:  customerDeposit.Code,
			Journals:       journals,
			CreatedAt:      &now,
			UpdatedAt:      &now,
		}

		err = ledger.Insert()
		if err != nil {
			return ledgers, err
		}

		ledgers = append(ledgers, *ledger)
	}

	return ledgers, nil
}
