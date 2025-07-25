package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jung-kurt/gofpdf"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
)

type QuotationProduct struct {
	ProductID                primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                     string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic             string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode                 string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PrefixPartNumber         string             `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber               string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity                 float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	QuantityReturned         float64            `json:"quantity_returned" bson:"quantity_returned"`
	Unit                     string             `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitPrice                float64            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	UnitPriceWithVAT         float64            `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	PurchaseUnitPrice        float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	PurchaseUnitPriceWithVAT float64            `bson:"purchase_unit_price_with_vat,omitempty" json:"purchase_unit_price_with_vat,omitempty"`
	//Discount                 float64            `bson:"discount" json:"discount"`
	//DiscountPercent          float64            `bson:"discount_percent" json:"discount_percent"`
	UnitDiscount               float64 `bson:"unit_discount" json:"unit_discount"`
	UnitDiscountWithVAT        float64 `bson:"unit_discount_with_vat" json:"unit_discount_with_vat"`
	UnitDiscountPercent        float64 `bson:"unit_discount_percent" json:"unit_discount_percent"`
	UnitDiscountPercentWithVAT float64 `bson:"unit_discount_percent_with_vat" json:"unit_discount_percent_with_vat"`
	Profit                     float64 `bson:"profit" json:"profit"`
	Loss                       float64 `bson:"loss" json:"loss"`
}

// Quotation : Quotation structure
type Quotation struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code                     string              `bson:"code,omitempty" json:"code,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                  string              `json:"date_str,omitempty" bson:"-"`
	StoreID                  *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Store                    *Store              `json:"store,omitempty"`
	CustomerID               *primitive.ObjectID `json:"customer_id" bson:"customer_id"`
	Customer                 *Customer           `json:"customer"  bson:"-" `
	CustomerName             string              `json:"customer_name" bson:"customer_name"`
	Products                 []QuotationProduct  `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy              *primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredBySignatureID   *primitive.ObjectID `json:"delivered_by_signature_id,omitempty" bson:"delivered_by_signature_id,omitempty"`
	DeliveredBySignatureName string              `json:"delivered_by_signature_name,omitempty" bson:"delivered_by_signature_name,omitempty"`
	SignatureDate            *time.Time          `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	SignatureDateStr         string              `json:"signature_date_str,omitempty"`
	DeliveredByUser          *User               `json:"delivered_by_user,omitempty"`
	DeliveredBySignature     *UserSignature      `json:"delivered_by_signature,omitempty"`
	VatPercent               *float64            `bson:"vat_percent" json:"vat_percent"`
	Discount                 float64             `bson:"discount" json:"discount"`
	DiscountPercent          float64             `bson:"discount_percent" json:"discount_percent"`
	DiscountWithVAT          float64             `bson:"discount_with_vat" json:"discount_with_vat"`
	DiscountPercentWithVAT   float64             `bson:"discount_percent_with_vat" json:"discount_percent_with_vat"`
	ReturnDiscountWithVAT    float64             `bson:"return_discount_with_vat" json:"return_discount_vat"`
	ReturnDiscount           float64             `bson:"return_discount" json:"return_discount"`
	ReturnCashDiscount       float64             `bson:"return_cash_discount" json:"return_cash_discount"`
	ReturnCount              int64               `bson:"return_count" json:"return_count"`
	Status                   string              `bson:"status,omitempty" json:"status,omitempty"`
	TotalQuantity            float64             `bson:"total_quantity" json:"total_quantity"`
	VatPrice                 float64             `bson:"vat_price" json:"vat_price"`
	Total                    float64             `bson:"total" json:"total"`
	TotalWithVAT             float64             `bson:"total_with_vat" json:"total_with_vat"`
	NetTotal                 float64             `bson:"net_total" json:"net_total"`
	ActualVatPrice           float64             `bson:"actual_vat_price" json:"actual_vat_price"`
	ActualTotal              float64             `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT       float64             `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	ActualNetTotal           float64             `bson:"actual_net_total" json:"actual_net_total"`
	RoundingAmount           float64             `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount       bool                `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	Payments                 []QuotationPayment  `bson:"payments" json:"payments"`
	PaymentsInput            []QuotationPayment  `bson:"-" json:"payments_input"`
	PaymentsCount            int64               `bson:"payments_count" json:"payments_count"`
	PaymentStatus            string              `bson:"payment_status" json:"payment_status"`
	PaymentMethods           []string            `json:"payment_methods" bson:"payment_methods"`
	CashDiscount             float64             `bson:"cash_discount" json:"cash_discount"`
	TotalPaymentReceived     float64             `bson:"total_payment_received" json:"total_payment_received"`
	BalanceAmount            float64             `bson:"balance_amount" json:"balance_amount"`
	ShippingOrHandlingFees   float64             `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	Profit                   float64             `bson:"profit" json:"profit"`
	NetProfit                float64             `bson:"net_profit" json:"net_profit"`
	Loss                     float64             `bson:"loss" json:"loss"`
	NetLoss                  float64             `bson:"net_loss" json:"net_loss"`
	Deleted                  bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy                *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser            *User               `json:"deleted_by_user,omitempty"`
	DeletedAt                *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt                *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy                *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy                *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser            *User               `json:"created_by_user,omitempty"`
	UpdatedByUser            *User               `json:"updated_by_user,omitempty"`
	StoreName                string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	DeliveredByName          string              `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CreatedByName            string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName            string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName            string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ValidityDays             *int64              `bson:"validity_days,omitempty" json:"validity_days,omitempty"`
	DeliveryDays             *int64              `bson:"delivery_days,omitempty" json:"delivery_days,omitempty"`
	Remarks                  string              `bson:"remarks" json:"remarks"`
	Type                     string              `json:"type" bson:"type"`
	Phone                    string              `bson:"phone" json:"phone"`
	VatNo                    string              `bson:"vat_no" json:"vat_no"`
	Address                  string              `bson:"address" json:"address"`
	OrderID                  *primitive.ObjectID `json:"order_id" bson:"order_id"`
	OrderCode                *string             `json:"order_code" bson:"order_code"`
	ReportedToZatca          bool                `bson:"reported_to_zatca" json:"reported_to_zatca"`
	ReportedToZatcaAt        *time.Time          `bson:"reported_to_zatca_at" json:"reported_to_zatca_at"`
	ReturnAmount             float64             `bson:"return_amount" json:"return_amount"`
}

func (product *Product) SetProductQuotationSalesQuantityByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationSalesStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
		"type":       "invoice",
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                      nil,
				"quotation_sales_quantity": bson.M{"$sum": "$quantity"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.QuotationSalesQuantity = stats.QuotationSalesQuantity
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (model *Quotation) SetPostBalances() error {
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

func (store *Store) GetReturnedAmountByQuotationID(quotationID primitive.ObjectID) (returnedAmount float64, returnCount int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats QuotationSalesReturnStats

	pipeline := []bson.M{
		bson.M{
			"$match": map[string]interface{}{
				"quotation_id": quotationID,
			},
		},
		bson.M{
			"$group": bson.M{
				"_id":                          nil,
				"quotation_sales_return_count": bson.M{"$sum": 1},
				"paid_quotation_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats.PaidQuotationSalesReturn, stats.QuotationSalesReturnCount, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return stats.PaidQuotationSalesReturn, stats.QuotationSalesReturnCount, err
		}
		stats.PaidQuotationSalesReturn = RoundFloat(stats.PaidQuotationSalesReturn, 2)
	}

	return stats.PaidQuotationSalesReturn, stats.QuotationSalesReturnCount, nil
}

func (quotation *Quotation) LinkOrUnLinkSales(quotationOld *Quotation) (err error) {
	var order *Order
	store, _ := FindStoreByID(quotation.StoreID, bson.M{})
	if quotation.OrderID != nil && !quotation.OrderID.IsZero() {
		order, err = store.FindOrderByID(quotation.OrderID, bson.M{})
		if err != nil {
			return err
		}
	} else if quotation.OrderCode != nil && !govalidator.IsNull(*quotation.OrderCode) {
		order, err = store.FindOrderByCode(*quotation.OrderCode, bson.M{})
		if err != nil {
			return errors.New("error_finding_order:" + err.Error())
		}
	} else {
		err := quotationOld.UnLinkSales()
		if err != nil {
			return err
		}
		return nil
	}

	order.QuotationID = &quotation.ID
	order.QuotationCode = &quotation.Code
	err = order.Update()
	if err != nil {
		return err
	}

	if order.Zatca.ReportingPassed {
		quotation.ReportedToZatca = true
		quotation.ReportedToZatcaAt = order.Zatca.ReportedAt
	} else {
		quotation.ReportedToZatca = false
	}
	err = quotation.Update()
	if err != nil {
		return err
	}

	return nil
}

func (quotation *Quotation) UnLinkSales() error {
	if quotation == nil {
		return nil
	}

	store, _ := FindStoreByID(quotation.StoreID, bson.M{})

	if quotation.OrderID != nil && quotation.OrderID.IsZero() {
		order, err := store.FindOrderByID(quotation.OrderID, bson.M{})
		if err != nil {
			return err
		}
		order.QuotationID = nil
		order.QuotationCode = nil
		err = order.Update()
		if err != nil {
			return err
		}

		quotation.OrderCode = nil
		quotation.OrderID = nil
		quotation.ReportedToZatca = false
		quotation.ReportedToZatcaAt = nil

		err = quotation.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

func (quotation *Quotation) CreateNewCustomerFromName() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(quotation.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customer != nil || govalidator.IsNull(quotation.CustomerName) {
		return nil
	}

	now := time.Now()
	newCustomer := Customer{
		Name:          quotation.CustomerName,
		Phone:         quotation.Phone,
		PhoneInArabic: ConvertToArabicNumerals(quotation.Phone),
		VATNo:         quotation.VatNo,
		VATNoInArabic: ConvertToArabicNumerals(quotation.VatNo),
		Remarks:       quotation.Remarks,
		CreatedBy:     quotation.CreatedBy,
		UpdatedBy:     quotation.CreatedBy,
		CreatedAt:     &now,
		UpdatedAt:     &now,
		StoreID:       quotation.StoreID,
	}

	err = newCustomer.MakeCode()
	if err != nil {
		return err
	}
	err = newCustomer.Insert()
	if err != nil {
		return err
	}
	err = newCustomer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}
	quotation.CustomerID = &newCustomer.ID
	return nil
}

func (quotation *Quotation) GetPaymentsCount() (count int64, err error) {
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"quotation_id": quotation.ID,
		"deleted":      bson.M{"$ne": true},
	})
}

func (quotation *Quotation) AddPayments() error {
	for _, payment := range quotation.PaymentsInput {
		quotationPayment := QuotationPayment{
			QuotationID:   &quotation.ID,
			QuotationCode: quotation.Code,
			Amount:        payment.Amount,
			Method:        payment.Method,
			Date:          payment.Date,
			CreatedAt:     quotation.CreatedAt,
			UpdatedAt:     quotation.UpdatedAt,
			CreatedBy:     quotation.CreatedBy,
			CreatedByName: quotation.CreatedByName,
			UpdatedBy:     quotation.UpdatedBy,
			UpdatedByName: quotation.UpdatedByName,
			StoreID:       quotation.StoreID,
			StoreName:     quotation.StoreName,
		}
		err := quotationPayment.Insert()
		if err != nil {
			return err
		}
	}
	return nil
}

func (quotation *Quotation) UpdatePayments() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store:" + err.Error())
	}

	_, err = quotation.SetPaymentStatus()
	if err != nil {
		return errors.New("error setting payment status: " + err.Error())
	}

	now := time.Now()
	for _, payment := range quotation.PaymentsInput {
		if payment.ID.IsZero() {
			//Create new
			quotationPayment := QuotationPayment{
				QuotationID:   &quotation.ID,
				QuotationCode: quotation.Code,
				Amount:        payment.Amount,
				Method:        payment.Method,
				Date:          payment.Date,
				CreatedAt:     &now,
				UpdatedAt:     &now,
				CreatedBy:     quotation.CreatedBy,
				CreatedByName: quotation.CreatedByName,
				UpdatedBy:     quotation.UpdatedBy,
				UpdatedByName: quotation.UpdatedByName,
				StoreID:       quotation.StoreID,
				StoreName:     quotation.StoreName,
			}
			err := quotationPayment.Insert()
			if err != nil {
				return errors.New("error creating payment:" + err.Error())
			}

		} else {
			//Update
			salesPayment, err := store.FindQuotationPaymentByID(&payment.ID, bson.M{})
			if err != nil {
				return errors.New("error finding payment by id:" + err.Error())
			}

			salesPayment.Date = payment.Date
			salesPayment.Amount = payment.Amount
			salesPayment.Method = payment.Method
			salesPayment.UpdatedAt = &now
			salesPayment.UpdatedBy = quotation.UpdatedBy
			salesPayment.UpdatedByName = quotation.UpdatedByName
			err = salesPayment.Update()
			if err != nil {
				return errors.New("error updating invoice payment: " + err.Error())
			}
		}

	}

	//Deleting payments

	paymentsToDelete := []QuotationPayment{}

	for _, payment := range quotation.Payments {
		found := false
		for _, paymentInput := range quotation.PaymentsInput {
			if paymentInput.ID.Hex() == payment.ID.Hex() {
				found = true
				break
			}
		}
		if !found {
			paymentsToDelete = append(paymentsToDelete, payment)
		}
	}

	for _, payment := range paymentsToDelete {
		payment.Deleted = true
		payment.DeletedAt = &now
		payment.DeletedBy = quotation.UpdatedBy
		err := payment.Update()
		if err != nil {
			return errors.New("error updating payment2: " + err.Error())
		}
	}

	return nil
}

func (quotation *Quotation) SetPaymentStatus() (models []QuotationPayment, err error) {
	if quotation.Type == "quotation" {
		quotation.BalanceAmount = 0
		quotation.TotalPaymentReceived = 0
		quotation.PaymentStatus = ""
		return models, nil
	}

	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation_payment")
	ctx := context.Background()
	findOptions := options.Find()

	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"quotation_id": quotation.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching quotation invoice payment history" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	totalPaymentReceived := float64(0.0)
	paymentMethods := []string{}

	//	log.Print("Starting for")
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		//log.Print("Loop")
		err := cur.Err()
		if err != nil {
			return models, errors.New("Cursor error:" + err.Error())
		}
		model := QuotationPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")

		models = append(models, model)

		totalPaymentReceived += model.Amount

		if !slices.Contains(paymentMethods, model.Method) {
			paymentMethods = append(paymentMethods, model.Method)
		}
	} //end for loop

	quotation.TotalPaymentReceived = RoundTo2Decimals(totalPaymentReceived)
	quotation.BalanceAmount = RoundTo2Decimals((quotation.NetTotal - quotation.CashDiscount) - totalPaymentReceived)
	quotation.PaymentMethods = paymentMethods
	quotation.Payments = models
	quotation.PaymentsCount = int64(len(models))

	if RoundTo2Decimals((quotation.NetTotal - quotation.CashDiscount)) <= totalPaymentReceived {
		quotation.PaymentStatus = "paid"
	} else if totalPaymentReceived > 0 {
		quotation.PaymentStatus = "paid_partially"
	} else if totalPaymentReceived <= 0 {
		quotation.PaymentStatus = "not_paid"
	}

	return models, err
}

func (store *Store) UpdateQuotationProfit() error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = quotation.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

type QuotationStats struct {
	ID        *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal  float64             `json:"net_total" bson:"net_total"`
	NetProfit float64             `json:"net_profit" bson:"net_profit"`
	Loss      float64             `json:"loss" bson:"loss"`
}

type QuotationInvoiceStats struct {
	ID                            *primitive.ObjectID `json:"id" bson:"_id"`
	InvoiceNetTotal               float64             `json:"invoice_net_total" bson:"invoice_net_total"`
	InvoiceNetProfit              float64             `json:"invoice_net_profit" bson:"invoice_net_profit"`
	InvoiceNetLoss                float64             `json:"invoice_net_loss" bson:"invoice_net_loss"`
	InvoiceVatPrice               float64             `json:"invoice_vat_price" bson:"vinvoice_at_price"`
	InvoiceDiscount               float64             `json:"invoice_discount" bson:"invoice_discount"`
	InvoiceShippingOrHandlingFees float64             `json:"invoice_hipping_handling_fees" bson:"invoice_shipping_handling_fees"`
	InvoicePaidSales              float64             `json:"invoice_paid_sales" bson:"invoice_paid_sales"`
	InvoiceUnPaidSales            float64             `json:"invoice_unpaid_sales" bson:"invoice_unpaid_sales"`
	InvoiceCashSales              float64             `json:"invoice_cash_sales" bson:"invoice_cash_sales"`
	InvoiceBankAccountSales       float64             `json:"invoice_bank_account_sales" bson:"invoice_bank_account_sales"`
	InvoiceCashDiscount           float64             `json:"invoice_cash_discount" bson:"invoice_cash_discount"`
}

func (store *Store) GetQuotationInvoiceStats(filter map[string]interface{}) (stats QuotationInvoiceStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make a copy of the map
	newFilter := make(map[string]interface{})
	for k, v := range filter {
		newFilter[k] = v
	}

	typeStr, ok := newFilter["type"]
	if ok && typeStr == "quotation" {
		return stats, nil
	}

	newFilter["type"] = "invoice"

	pipeline := []bson.M{
		bson.M{
			"$match": newFilter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                            nil,
				"invoice_net_total":              bson.M{"$sum": "$net_total"},
				"invoice_net_profit":             bson.M{"$sum": "$net_profit"},
				"invoice_net_loss":               bson.M{"$sum": "$net_loss"},
				"invoice_vat_price":              bson.M{"$sum": "$vat_price"},
				"invoice_discount":               bson.M{"$sum": "$discount"},
				"invoice_cash_discount":          bson.M{"$sum": "$cash_discount"},
				"invoice_shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
				"invoice_paid_sales": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
				"invoice_unpaid_sales": bson.M{"$sum": "$balance_amount"},
				"invoice_cash_sales": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "cash"}},
									bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
				"invoice_bank_account_sales": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$or": []interface{}{
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "debit_card"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "credit_card"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "bank_card"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "bank_transfer"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
									bson.M{"$and": []interface{}{
										bson.M{"$eq": []interface{}{"$$payment.method", "bank_cheque"}},
										bson.M{"$gt": []interface{}{"$$payment.amount", 0}},
									}},
								}},
								"$$payment.amount",
								0,
							},
						},
					},
				}}},
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
	}
	return stats, nil
}

func (store *Store) GetQuotationStats(filter map[string]interface{}) (stats QuotationStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Make a copy of the map
	newFilter := make(map[string]interface{})
	for k, v := range filter {
		newFilter[k] = v
	}

	typeStr, ok := newFilter["type"]
	if ok && typeStr == "invoice" {
		return stats, nil
	}

	newFilter["type"] = "quotation"

	pipeline := []bson.M{
		bson.M{
			"$match": newFilter,
		},
		bson.M{
			"$group": bson.M{
				"_id":        nil,
				"net_total":  bson.M{"$sum": "$net_total"},
				"net_profit": bson.M{"$sum": "$net_profit"},
				"loss":       bson.M{"$sum": "$loss"},
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
		stats.NetTotal = RoundFloat(stats.NetTotal, 2)
		stats.NetProfit = RoundFloat(stats.NetProfit, 2)
		stats.Loss = RoundFloat(stats.Loss, 2)

		return stats, nil
	}
	return stats, nil
}

/*
func (quotation *Quotation) CalculateQuotationProfit() error {
	totalProfit := float64(0.0)
	totalLoss := float64(0.0)
	for i, quotationProduct := range quotation.Products {
		product, err := FindProductByID(&quotationProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}
		quantity := quotationProduct.Quantity

		salesPrice := (quantity * quotationProduct.UnitPrice) - quotationProduct.Discount

		purchaseUnitPrice := quotation.Products[i].PurchaseUnitPrice

		if purchaseUnitPrice == 0 {
			for _, store := range product.ProductStores {
				if store.StoreID == *quotation.StoreID {
					purchaseUnitPrice = store.PurchaseUnitPrice
					quotation.Products[i].PurchaseUnitPrice = purchaseUnitPrice
					break
				}
			}
		}

		profit := salesPrice - (quantity * purchaseUnitPrice)
		profit = RoundFloat(profit, 2)

		if profit >= 0 {
			quotation.Products[i].Profit = profit
			quotation.Products[i].Loss = 0.0
			totalProfit += quotation.Products[i].Profit
		} else {
			quotation.Products[i].Profit = 0
			quotation.Products[i].Loss = (profit * -1)
			totalLoss += quotation.Products[i].Loss
		}

	}

	quotation.Profit = RoundFloat(totalProfit, 2)
	quotation.NetProfit = RoundFloat((totalProfit - quotation.Discount), 2)
	quotation.Loss = totalLoss
	return nil
}
*/

func (model *Quotation) CalculateQuotationProfit() error {
	totalProfit := float64(0.0)
	totalLoss := float64(0.0)
	for i, quotationProduct := range model.Products {
		quantity := quotationProduct.Quantity

		quotationPrice := (quantity * (quotationProduct.UnitPrice - quotationProduct.UnitDiscount))
		purchaseUnitPrice := quotationProduct.PurchaseUnitPrice

		profit := 0.0
		if purchaseUnitPrice > 0 {
			profit = quotationPrice - (quantity * purchaseUnitPrice)
		}

		loss := 0.0

		//profit = RoundFloat(profit, 2)

		if profit >= 0 {
			model.Products[i].Profit = profit
			model.Products[i].Loss = loss
			totalProfit += model.Products[i].Profit
		} else {
			model.Products[i].Profit = 0
			loss = (profit * -1)
			model.Products[i].Loss = loss
			totalLoss += model.Products[i].Loss
		}

	}

	model.Profit = totalProfit
	model.NetProfit = (totalProfit - model.Discount)
	model.Loss = totalLoss
	model.NetLoss = totalLoss

	if model.NetProfit < 0 {
		model.NetLoss += (model.NetProfit * -1)
		model.NetProfit = 0.00
	}

	return nil
}

func (quotation *Quotation) GeneratePDF() error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 8)
	//pdf.Cell(float64(10), float64(5), "GULF UNION OZONE CO.")
	pdf.CellFormat(50, 7, "GULF UNION OZONE CO.", "1", 0, "LM", false, 0, "")
	pdf.AddLayer("layer1", true)
	//pdf.Rect(float64(5), float64(5), float64(201), float64(286), "")
	pdf.SetCreator("Sirin K", true)
	pdf.SetMargins(float64(2), float64(2), float64(2))
	pdf.Rect(float64(5), float64(5), float64(201), float64(286), "")
	pdf.AddPage()
	pdf.Cell(40, 10, "Hello, world2")

	filename := "pdfs/quotations/quotation_" + quotation.Code + ".pdf"

	return pdf.OutputFileAndClose(filename)
}

/*
func (quotation *Quotation) GeneratePDF() error {

	pdfg, err := wkhtml.NewPDFGenerator()
	if err != nil {
		return err
	}

	//	htmlStr := quotation.getHTML()

	html, err := ioutil.ReadFile("html-templates/quotation.html")
	//html, err := ioutil.ReadFile("html-templates/test.html")

	if err != nil {
		return err
	}

	//log.Print(html)

	//page := wkhtmltopdf.NewPageReader(bytes.NewReader(html))
	//pdfg.AddPage(page)
	//	page.NoBackground.Set(true)
	//	page.DisableExternalLinks.Set(false)

	// create a new instance of the PDF generator


	//pdfg.AddPage(wkhtml.NewPageReader(strings.NewReader(htmlStr)))

	//pageReader.PageOptions.EnableLocalFileAccess.Set(true)
	pdfg.Cover.EnableLocalFileAccess.Set(true)
	pdfg.AddPage(wkhtml.NewPageReader(bytes.NewReader(html)))

	// Create PDF document in internal buffer
	err = pdfg.Create()
	if err != nil {
		return err
	}

	filename := "pdfs/quotations/quotation_" + quotation.Code + ".pdf"

	//Your Pdf Name
	err = pdfg.WriteFile(filename)
	if err != nil {
		return err
	}
	return nil
}
*/

func (quotation *Quotation) AttributesValueChangeEvent(quotationOld *Quotation) error {

	if quotation.Status != quotationOld.Status {
		/*
			quotation.SetChangeLog(
				"attribute_value_change",
				"status",
				quotationOld.Status,
				quotation.Status,
			)
		*/
	}

	return nil
}

func (quotation *Quotation) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if quotation.StoreID != nil {
		store, err := FindStoreByID(quotation.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.StoreName = store.Name
	}

	if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(quotation.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.CustomerName = customer.Name
	} else {
		quotation.CustomerName = ""
	}

	if quotation.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(quotation.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeliveredByName = deliveredByUser.Name
	}

	if quotation.DeliveredBySignatureID != nil {
		deliveredBySignature, err := store.FindSignatureByID(quotation.DeliveredBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeliveredBySignatureName = deliveredBySignature.Name
	}

	if quotation.CreatedBy != nil {
		createdByUser, err := FindUserByID(quotation.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.CreatedByName = createdByUser.Name
	}

	if quotation.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(quotation.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.UpdatedByName = updatedByUser.Name
	}

	if quotation.DeletedBy != nil && !quotation.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(quotation.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeletedByName = deletedByUser.Name
	}

	for i, product := range quotation.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1})
		if err != nil {
			return err
		}
		//quotation.Products[i].Name = productObject.Name
		quotation.Products[i].NameInArabic = productObject.NameInArabic
		quotation.Products[i].ItemCode = productObject.ItemCode
		//quotation.Products[i].PartNumber = productObject.PartNumber
		quotation.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

func (quotation *Quotation) FindNetTotal() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	quotation.ShippingOrHandlingFees = RoundTo2Decimals(quotation.ShippingOrHandlingFees)
	quotation.Discount = RoundTo2Decimals(quotation.Discount)

	quotation.FindTotal()

	/*
		if quotation.DiscountWithVAT > 0 {
			quotation.Discount = RoundTo2Decimals(quotation.DiscountWithVAT / (1 + (*quotation.VatPercent / 100)))
		} else if quotation.Discount > 0 {
			quotation.DiscountWithVAT = RoundTo2Decimals(quotation.Discount * (1 + (*quotation.VatPercent / 100)))
		} else {
			quotation.Discount = 0
			quotation.DiscountWithVAT = 0
		}*/
	// Apply discount to the base amount first
	baseTotal := quotation.Total + quotation.ShippingOrHandlingFees - quotation.Discount
	baseTotal = RoundTo2Decimals(baseTotal)

	// Now calculate VAT on the discounted base
	quotation.VatPrice = RoundTo2Decimals(baseTotal * (*quotation.VatPercent / 100))

	if store.Settings.HideQuotationInvoiceVAT && quotation.Type == "invoice" {
		quotation.VatPrice = 0
	}

	quotation.NetTotal = RoundTo2Decimals(baseTotal + quotation.VatPrice)

	//Actual
	actualBaseTotal := quotation.ActualTotal + quotation.ShippingOrHandlingFees - quotation.Discount
	actualBaseTotal = RoundTo8Decimals(actualBaseTotal)

	// Now calculate VAT on the discounted base
	quotation.ActualVatPrice = RoundTo2Decimals(actualBaseTotal * (*quotation.VatPercent / 100))
	if store.Settings.HideQuotationInvoiceVAT && quotation.Type == "invoice" {
		quotation.ActualVatPrice = 0
	}

	quotation.ActualNetTotal = RoundTo2Decimals(actualBaseTotal + quotation.ActualVatPrice)

	if quotation.AutoRoundingAmount {
		quotation.RoundingAmount = RoundTo2Decimals(quotation.ActualNetTotal - quotation.NetTotal)
	}

	quotation.NetTotal = RoundTo2Decimals(quotation.NetTotal + quotation.RoundingAmount)

	quotation.CalculateDiscountPercentage()
	return nil
}

func (quotation *Quotation) FindTotal() {
	total := float64(0.0)
	totalWithVAT := float64(0.0)
	//Actual
	actualTotal := float64(0.0)
	actualTotalWithVAT := float64(0.0)
	for i, product := range quotation.Products {

		/*
			if product.UnitPriceWithVAT > 0 {
				quotation.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPriceWithVAT / (1 + (*quotation.VatPercent / 100)))
			} else if product.UnitPrice > 0 {
				quotation.Products[i].UnitPriceWithVAT = RoundTo2Decimals(product.UnitPrice * (1 + (*quotation.VatPercent / 100)))
			}

			if product.UnitDiscountWithVAT > 0 {
				quotation.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscountWithVAT / (1 + (*quotation.VatPercent / 100)))
			} else if product.UnitDiscount > 0 {
				quotation.Products[i].UnitDiscountWithVAT = RoundTo2Decimals(product.UnitDiscount * (1 + (*quotation.VatPercent / 100)))
			}

			if product.UnitDiscountPercentWithVAT > 0 {
				quotation.Products[i].UnitDiscountPercent = RoundTo2Decimals((product.UnitDiscount / product.UnitPrice) * 100)
			} else if product.UnitDiscountPercent > 0 {
				quotation.Products[i].UnitDiscountPercentWithVAT = RoundTo2Decimals((product.UnitDiscountWithVAT / product.UnitPriceWithVAT) * 100)
			}*/

		total += (product.Quantity * (quotation.Products[i].UnitPrice - quotation.Products[i].UnitDiscount))
		totalWithVAT += (product.Quantity * (quotation.Products[i].UnitPriceWithVAT - quotation.Products[i].UnitDiscountWithVAT))
		total = RoundTo2Decimals(total)
		totalWithVAT = RoundTo2Decimals(totalWithVAT)

		//Actual values
		actualTotal += (product.Quantity * (quotation.Products[i].UnitPrice - quotation.Products[i].UnitDiscount))
		actualTotal = RoundTo8Decimals(actualTotal)
		actualTotalWithVAT += (product.Quantity * (quotation.Products[i].UnitPriceWithVAT - quotation.Products[i].UnitDiscountWithVAT))
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)
	}

	quotation.Total = total
	quotation.TotalWithVAT = totalWithVAT

	//Actual
	quotation.ActualTotal = actualTotal
	quotation.ActualTotalWithVAT = actualTotalWithVAT
}

func (quotation *Quotation) CalculateDiscountPercentage() {
	if quotation.Discount <= 0 {
		quotation.DiscountPercent = 0.00
		quotation.DiscountPercentWithVAT = 0.00
		return
	}

	baseBeforeDiscount := quotation.NetTotal + quotation.Discount
	if baseBeforeDiscount == 0 {
		quotation.DiscountPercent = 0.00
		quotation.DiscountPercentWithVAT = 0.00
		return
	}

	percentage := (quotation.Discount / baseBeforeDiscount) * 100
	quotation.DiscountPercent = RoundTo2Decimals(percentage)

	baseBeforeDiscountWithVAT := quotation.NetTotal + quotation.DiscountWithVAT
	if baseBeforeDiscountWithVAT == 0 {
		quotation.DiscountPercent = 0.00
		quotation.DiscountPercentWithVAT = 0.00
		return
	}

	percentage = (quotation.DiscountWithVAT / baseBeforeDiscountWithVAT) * 100
	quotation.DiscountPercentWithVAT = RoundTo2Decimals(percentage)
}

/*
func (quotation *Quotation) FindNetTotal() {
	netTotal := float64(0.0)
	quotation.FindTotal()
	netTotal = quotation.Total

	quotation.ShippingOrHandlingFees = RoundTo2Decimals(quotation.ShippingOrHandlingFees)
	quotation.Discount = RoundTo2Decimals(quotation.Discount)

	netTotal += quotation.ShippingOrHandlingFees
	netTotal -= quotation.Discount

	quotation.FindVatPrice()
	netTotal += quotation.VatPrice

	quotation.NetTotal = RoundTo2Decimals(netTotal)
	quotation.CalculateDiscountPercentage()
}

func (quotation *Quotation) CalculateDiscountPercentage() {
	if quotation.NetTotal == 0 {
		quotation.DiscountPercent = 0
	}

	if quotation.Discount <= 0 {
		quotation.DiscountPercent = 0.00
		return
	}

	percentage := (quotation.Discount / quotation.NetTotal) * 100
	quotation.DiscountPercent = RoundTo2Decimals(percentage) // Use rounding here
}

func (quotation *Quotation) FindTotal() {
	total := float64(0.0)
	for i, product := range quotation.Products {
		quotation.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPrice)
		quotation.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscount)

		if quotation.Products[i].UnitDiscount > 0 {
			quotation.Products[i].UnitDiscountPercent = RoundTo2Decimals((quotation.Products[i].UnitDiscount / quotation.Products[i].UnitPrice) * 100)
		}
		total += RoundTo2Decimals(product.Quantity * (quotation.Products[i].UnitPrice - quotation.Products[i].UnitDiscount))
	}

	quotation.Total = RoundTo2Decimals(total)
}

func (quotation *Quotation) FindVatPrice() {
	vatPrice := ((*quotation.VatPercent / float64(100.00)) * ((quotation.Total + quotation.ShippingOrHandlingFees) - quotation.Discount))
	quotation.VatPrice = RoundTo2Decimals(vatPrice)
}
*/

func (quotation *Quotation) FindTotalQuantity() {
	totalQuantity := float64(0.0)
	for _, product := range quotation.Products {
		totalQuantity += product.Quantity
	}
	quotation.TotalQuantity = totalQuantity
}

func (store *Store) SearchQuotation(w http.ResponseWriter, r *http.Request) (quotations []Quotation, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[return_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["return_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["return_count"] = value
		}
	}

	keys, ok = r.URL.Query()["search[return_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["return_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["return_amount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["created_at"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	keys, ok = r.URL.Query()["search[discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["discount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["discount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[cash_discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["cash_discount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["cash_discount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[created_at_from]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtStartDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[order_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["order_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[reported_to_zatca]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return quotations, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["reported_to_zatca"] = bson.M{"$eq": true}
		} else if value == 0 {
			criterias.SearchBy["reported_to_zatca"] = bson.M{"$ne": true}
		}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return quotations, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return quotations, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[type]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] != "" {
			criterias.SearchBy["type"] = keys[0]
		}
	}

	/*
		keys, ok = r.URL.Query()["search[payment_status]"]
		if ok && len(keys[0]) >= 1 {
			if keys[0] != "" {
				criterias.SearchBy["payment_status"] = keys[0]
			}
		}*/

	keys, ok = r.URL.Query()["search[payment_status]"]
	if ok && len(keys[0]) >= 1 {
		paymentStatusList := strings.Split(keys[0], ",")
		if len(paymentStatusList) > 0 {
			criterias.SearchBy["payment_status"] = bson.M{"$in": paymentStatusList}
		}
	}

	keys, ok = r.URL.Query()["search[payment_method]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethodList := strings.Split(keys[0], ",")
		if len(paymentMethodList) > 0 {
			criterias.SearchBy["payment_method"] = bson.M{"$in": paymentMethodList}
		}
	}

	keys, ok = r.URL.Query()["search[payment_methods]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethods := strings.Split(keys[0], ",")
		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_methods"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		statusList := strings.Split(keys[0], ",")
		if len(statusList) > 0 {
			criterias.SearchBy["status"] = bson.M{"$in": statusList}
		}
	}

	keys, ok = r.URL.Query()["search[delivered_by]"]
	if ok && len(keys[0]) >= 1 {
		deliveredByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotations, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	customerSelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
		if _, ok := criterias.Select["store.id"]; ok {
			storeSelectFields = ParseRelationalSelectString(keys[0], "store")
		}

		if _, ok := criterias.Select["customer.id"]; ok {
			customerSelectFields = ParseRelationalSelectString(keys[0], "customer")
		}

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
		return quotations, criterias, errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return quotations, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return quotations, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			quotation.Store, _ = FindStoreByID(quotation.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			quotation.Customer, _ = store.FindCustomerByID(quotation.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			quotation.CreatedByUser, _ = FindUserByID(quotation.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			quotation.UpdatedByUser, _ = FindUserByID(quotation.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			quotation.DeletedByUser, _ = FindUserByID(quotation.DeletedBy, deletedByUserSelectFields)
		}

		quotations = append(quotations, quotation)
	} //end for loop

	return quotations, criterias, nil
}

func (quotation *Quotation) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	customer, err := store.FindCustomerByID(quotation.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "Invalid customer"
		return errs
	}

	if customer == nil && govalidator.IsNull(quotation.CustomerName) {
		quotation.CustomerID = nil
	}

	if govalidator.IsNull(quotation.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, quotation.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		quotation.Date = &date
	}

	if !govalidator.IsNull(strings.TrimSpace(quotation.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(quotation.Phone)) {
		errs["phone"] = "Invalid phone no."
		return
	}

	if !govalidator.IsNull(strings.TrimSpace(quotation.VatNo)) && !IsValidDigitNumber(strings.TrimSpace(quotation.VatNo), "15") {
		errs["vat_no"] = "VAT No. should be 15 digits"
		return
	} else if !govalidator.IsNull(strings.TrimSpace(quotation.VatNo)) && !IsNumberStartAndEndWith(strings.TrimSpace(quotation.VatNo), "3") {
		errs["vat_no"] = "VAT No. should start and end with 3"
		return
	}

	if scenario == "update" {
		if quotation.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsQuotationExists(&quotation.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Quotation:" + quotation.ID.Hex()
		}

	}

	if quotation.Type == "invoice" {
		totalPayment := float64(0.00)
		for _, payment := range quotation.PaymentsInput {
			if payment.Amount > 0 {
				totalPayment += payment.Amount
			}
		}

		if totalPayment > RoundTo2Decimals(quotation.NetTotal-quotation.CashDiscount) {
			errs["total_payment"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(quotation.NetTotal-quotation.CashDiscount)) + " (Net Total - Cash Discount)"
			return
		}

		for index, payment := range quotation.PaymentsInput {
			if govalidator.IsNull(payment.DateStr) {
				errs["payment_date_"+strconv.Itoa(index)] = "Payment date is required"
			} else {
				const shortForm = "2006-01-02T15:04:05Z07:00"
				date, err := time.Parse(shortForm, payment.DateStr)
				if err != nil {
					errs["payment_date_"+strconv.Itoa(index)] = "Invalid date format"
				}

				quotation.PaymentsInput[index].Date = &date
				payment.Date = &date

				if quotation.Date != nil && IsAfter(quotation.Date, quotation.PaymentsInput[index].Date) {
					errs["payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to invoice date time"
				}
			}

			if payment.Amount == 0 {
				errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount is required"
			} else if payment.Amount < 0 {
				errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
			}

			if payment.Method == "" {
				errs["payment_method_"+strconv.Itoa(index)] = "Payment method is required"
			}

		} //end for
	}

	if len(quotation.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for quotation"
	}

	for index, product := range quotation.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for quotation"
		} else {
			exists, err := store.IsProductExists(&product.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(index)] = "Invalid product_id:" + product.ProductID.Hex() + " in products"
			}
		}

		if product.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(index)] = "Quantity is required"
		}

		if govalidator.IsNull(strings.TrimSpace(product.Name)) {
			errs["name_"+strconv.Itoa(index)] = "Name is required"
		} else if len(product.Name) < 3 {
			errs["name_"+strconv.Itoa(index)] = "Name requires min. 3 chars"
		}

		if product.UnitPrice == 0 {
			errs["unit_price_"+strconv.Itoa(index)] = "Unit Price is required"
		}

		if product.UnitDiscount > product.UnitPrice && product.UnitPrice > 0 {
			errs["unit_discount_"+strconv.Itoa(index)] = "Unit discount should not be greater than unit price"
		}

		if product.PurchaseUnitPrice == 0 {
			errs["purchase_unit_price_"+strconv.Itoa(index)] = "Purchase Unit Price is required"
		}

	}

	if quotation.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if quotation.ValidityDays == nil {
		errs["validity_days"] = "Validity days are required"
	} else if *quotation.ValidityDays < 1 {
		errs["validity_days"] = "Validity days should be greater than 0"
	}

	if quotation.DeliveryDays == nil {
		errs["delivery_days"] = "Delivery days are required"
	} else if *quotation.DeliveryDays < 1 {
		errs["delivery_days"] = "Delivery days should be greater than 0"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (quotation *Quotation) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": quotation.StoreID}, "quotation")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (quotation *Quotation) Insert() error {
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quotation.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &quotation)
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) GetQuotationsCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (quotation *Quotation) MakeRedisCode() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := quotation.StoreID.Hex() + "_quotation_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from order.CreatedAt or fallback to order.Date or now ===
	baseTime := quotation.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("quotation")
		if err != nil {
			return err
		}
		startFrom := store.QuotationSerialNumber.StartFromCount
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
	useMonthly := strings.Contains(store.QuotationSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := quotation.StoreID.Hex() + "_quotation_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.QuotationSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "quotation")
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
	paddingCount := store.QuotationSerialNumber.PaddingCount
	if store.QuotationSerialNumber.Prefix != "" {
		quotation.Code = fmt.Sprintf("%s-%0*d", store.QuotationSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		quotation.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(quotation.Code, "DATE") {
		orderDate := baseTime.Format("20060102") // YYYYMMDD
		quotation.Code = strings.ReplaceAll(quotation.Code, "DATE", orderDate)
	}

	return nil
}

func (quotation *Quotation) UnMakeRedisCode() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := quotation.StoreID.Hex() + "_quotation_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := quotation.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.QuotationSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := quotation.StoreID.Hex() + "_quotation_counter_" + monthKey

		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
func (model *Quotation) MakeRedisCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_quotation_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetQuotationCount()
		if err != nil {
			return err
		}

		startFrom := store.QuotationSerialNumber.StartFromCount

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

	paddingCount := store.QuotationSerialNumber.PaddingCount

	if store.QuotationSerialNumber.Prefix != "" {
		model.Code = fmt.Sprintf("%s-%0*d", store.QuotationSerialNumber.Prefix, paddingCount, incr)
	} else {
		model.Code = fmt.Sprintf("%s%0*d", store.QuotationSerialNumber.Prefix, paddingCount, incr)
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

func (quotation *Quotation) MakeCode() error {
	return quotation.MakeRedisCode()
}

func (store *Store) FindLastQuotationByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotation *Quotation, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": store.ID}, findOneOptions).
		Decode(&quotation)
	if err != nil {
		return nil, err
	}

	return quotation, err
}

func (quotation *Quotation) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if quotation.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotation.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotation.Code,
			"_id":  bson.M{"$ne": quotation.ID},
		})
	}

	return (count > 0), err
}

func (quotation *Quotation) Update() error {
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (quotation *Quotation) DeleteQuotation(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = quotation.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	quotation.Deleted = true
	quotation.DeletedBy = &userID
	now := time.Now()
	quotation.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindQuotationByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotation *Quotation, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
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
		Decode(&quotation)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		storeSelectFields := ParseRelationalSelectString(selectFields, "store")
		quotation.Store, _ = FindStoreByID(quotation.StoreID, storeSelectFields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		customerSelectFields := ParseRelationalSelectString(selectFields, "customer")
		quotation.Customer, _ = store.FindCustomerByID(quotation.CustomerID, customerSelectFields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		quotation.CreatedByUser, _ = FindUserByID(quotation.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		quotation.UpdatedByUser, _ = FindUserByID(quotation.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		quotation.DeletedByUser, _ = FindUserByID(quotation.DeletedBy, fields)
	}

	return quotation, err
}

func (store *Store) IsQuotationExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func ProcessQuotations() error {
	log.Print("Processing quotations")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		totalCount, err := store.GetTotalCount(bson.M{}, "quotation")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)

		cur, err := collection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			return errors.New("Error fetching quotations:" + err.Error())
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
			quotation := Quotation{}
			err = cur.Decode(&quotation)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if quotation.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			quotation.ClearProductsHistory()
			quotation.CreateProductsHistory()

			/*

				if store.Code == "MBDI" || store.Code == "LGK" {
					quotation.ClearProductsQuotationHistory()
					quotation.CreateProductsQuotationHistory()
				}*/

			/*
				if quotation.Type == "quotation" {
					quotation.SetPaymentStatus()
					err = quotation.Update()
					if err != nil {
						return err
					}
				}*/

			/*

				quotation.UndoAccounting()
				quotation.DoAccounting()
				if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() && quotation.Type == "invoice" {
					customer, _ := store.FindCustomerByID(quotation.CustomerID, bson.M{})
					if customer != nil {
						customer.SetCreditBalance()
					}
				}*/

			/*
				if quotation.Type != "invoice" {
					quotation.Type = "quotation"
					err = quotation.Update()
					if err != nil {
						return err
					}
				}

				quotation.ClearProductsQuotationHistory()
				quotation.AddProductsQuotationHistory()
			*/
			//quotation.SetCustomerQuotationStats()

			/*
				quotation.ClearProductsQuotationHistory()
				err = quotation.AddProductsQuotationHistory()
				if err != nil {
					return err
				}*/

			/*
				for i, product := range quotation.Products {
					if product.Discount > 0 {
						quotation.Products[i].UnitDiscount = product.Discount / product.Quantity
						quotation.Products[i].UnitDiscountPercent = product.DiscountPercent
					}
				}


			*/

			/*
				err = quotation.SetProductsQuotationStats()
				if err != nil {
					return err
				}*/

			/*
				err = quotation.SetCustomerQuotationStats()
				if err != nil {
					return err
				}*/

			//quotation.Date = quotation.CreatedAt
			/*
				err = quotation.Update()
				if err != nil {
					return err
				}
			*/
			bar.Add(1)
		}
	}
	log.Print("DONE!")
	return nil
}

type ProductQuotationStats struct {
	QuotationCount    int64   `json:"quotation_count" bson:"quotation_count"`
	QuotationQuantity float64 `json:"quotation_quantity" bson:"quotation_quantity"`
	Quotation         float64 `json:"quotation" bson:"quotation"`
}

func (product *Product) SetProductQuotationStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
		"type":       "quotation",
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                nil,
				"quotation_count":    bson.M{"$sum": 1},
				"quotation_quantity": bson.M{"$sum": "$quantity"},
				"quotation":          bson.M{"$sum": "$net_price"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}

		stats.Quotation = RoundFloat(stats.Quotation, 2)
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.QuotationCount = stats.QuotationCount
		productStoreTemp.QuotationQuantity = stats.QuotationQuantity
		productStoreTemp.Quotation = stats.Quotation
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (quotation *Quotation) SetProductsQuotationStats() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, quotationProduct := range quotation.Products {
		product, err := store.FindProductByID(&quotationProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductQuotationStatsByStoreID(*quotation.StoreID)
		if err != nil {
			return err
		}

		err = product.Update(nil)
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				err = setProductObj.SetProductQuotationStatsByStoreID(store.ID)
				if err != nil {
					return err
				}

				err = setProductObj.Update(&store.ID)
				if err != nil {
					return err
				}
			}
		}

	}
	return nil
}

//Qtn. sales

type ProductQuotationSalesStats struct {
	QuotationSalesCount    int64   `json:"quotation_sales_count" bson:"quotation_sales_count"`
	QuotationSalesQuantity float64 `json:"quotation_sales_quantity" bson:"quotation_sales_quantity"`
	QuotationSales         float64 `json:"quotation_sales" bson:"quotation_sales"`
}

func (product *Product) SetProductQuotationSalesStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationSalesStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
		"type":       "invoice",
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                      nil,
				"quotation_sales_count":    bson.M{"$sum": 1},
				"quotation_sales_quantity": bson.M{"$sum": "$quantity"},
				"quotation_sales":          bson.M{"$sum": "$net_price"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}

		stats.QuotationSales = RoundFloat(stats.QuotationSales, 2)
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.QuotationSalesCount = stats.QuotationSalesCount
		productStoreTemp.QuotationSalesQuantity = stats.QuotationSalesQuantity
		productStoreTemp.QuotationSales = stats.QuotationSales
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (quotation *Quotation) SetProductsQuotationSalesStats() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, quotationProduct := range quotation.Products {
		product, err := store.FindProductByID(&quotationProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductQuotationSalesStatsByStoreID(*quotation.StoreID)
		if err != nil {
			return err
		}

		err = product.Update(nil)
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				err = setProductObj.SetProductQuotationSalesStatsByStoreID(store.ID)
				if err != nil {
					return err
				}

				err = setProductObj.Update(&store.ID)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (quotation *Quotation) SetProductsStock() (err error) {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(quotation.Products) == 0 {
		return nil
	}

	for _, quotationProduct := range quotation.Products {
		product, err := store.FindProductByID(&quotationProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		err = product.SetStock()
		if err != nil {
			return err
		}

		err = product.Update(&store.ID)
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				err = setProductObj.SetStock()
				if err != nil {
					return err
				}

				err = setProductObj.Update(&store.ID)
				if err != nil {
					return err
				}

			}
		}

	}

	return nil
}

// Customer
/*
type CustomerQuotationStats struct {
	QuotationCount  int64   `json:"quotation_count" bson:"quotation_count"`
	QuotationAmount float64 `json:"quotation_amount" bson:"quotation_amount"`
	QuotationProfit float64 `json:"quotation_profit" bson:"quotation_profit"`
	QuotationLoss   float64 `json:"quotation_loss" bson:"quotation_loss"`
}
*/

type CustomerQuotationInvoiceStats struct {
	InvoiceCount              int64   `json:"invoice_count" bson:"invoice_count"`
	InvoiceAmount             float64 `json:"invoice_amount" bson:"invoice_amount"`
	InvoicePaidAmount         float64 `json:"invoice_paid_amount" bson:"invoice_paid_amount"`
	InvoiceBalanceAmount      float64 `json:"invoice_balance_amount" bson:"invoice_balance_amount"`
	InvoiceProfit             float64 `json:"invoice_profit" bson:"invoice_profit"`
	InvoiceLoss               float64 `json:"invoice_loss" bson:"invoice_loss"`
	InvoicePaidCount          int64   `json:"invoice_paid_count" bson:"invoice_paid_count"`
	InvoiceNotPaidCount       int64   `json:"invoice_not_paid_count" bson:"invoice_not_paid_count"`
	InvoicePaidPartiallyCount int64   `json:"invoice_paid_partially_count" bson:"invoice_paid_partially_count"`
}

type CustomerQuotationStats struct {
	QuotationCount  int64   `json:"quotation_count" bson:"quotation_count"`
	QuotationAmount float64 `json:"quotation_amount" bson:"quotation_amount"`
	QuotationProfit float64 `json:"quotation_profit" bson:"quotation_profit"`
	QuotationLoss   float64 `json:"quotation_loss" bson:"quotation_loss"`
}

func (customer *Customer) SetCustomerQuotationInvoiceStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerQuotationInvoiceStats

	filter := map[string]interface{}{
		"store_id":    storeID,
		"customer_id": customer.ID,
		"type":        "invoice",
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"invoice_count":          bson.M{"$sum": 1},
				"invoice_amount":         bson.M{"$sum": "$net_total"},
				"invoice_paid_amount":    bson.M{"$sum": "$total_payment_received"},
				"invoice_balance_amount": bson.M{"$sum": "$balance_amount"},
				"invoice_profit":         bson.M{"$sum": "$net_profit"},
				"invoice_loss":           bson.M{"$sum": "$loss"},
				"invoice_paid_count": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": []string{
								"$payment_status",
								"paid",
							},
						},
						"then": 1,
						"else": 0,
					},
				}},
				"invoice_not_paid_count": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": []string{
								"$payment_status",
								"not_paid",
							},
						},
						"then": 1,
						"else": 0,
					},
				}},
				"invoice_paid_partially_count": bson.M{"$sum": bson.M{
					"$cond": bson.M{
						"if": bson.M{
							"$eq": []string{
								"$payment_status",
								"paid_partially",
							},
						},
						"then": 1,
						"else": 0,
					},
				}},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
		stats.InvoiceAmount = RoundFloat(stats.InvoiceAmount, 2)
		stats.InvoicePaidAmount = RoundFloat(stats.InvoicePaidAmount, 2)
		stats.InvoiceBalanceAmount = RoundFloat(stats.InvoiceBalanceAmount, 2)
		stats.InvoiceProfit = RoundFloat(stats.InvoiceProfit, 2)
		stats.InvoiceLoss = RoundFloat(stats.InvoiceLoss, 2)
	}

	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if len(customer.Stores) == 0 {
		customer.Stores = map[string]CustomerStore{}
	}

	if customerStore, ok := customer.Stores[storeID.Hex()]; ok {
		customerStore.StoreID = storeID
		customerStore.StoreName = store.Name
		customerStore.StoreNameInArabic = store.NameInArabic
		customerStore.QuotationInvoiceCount = stats.InvoiceCount
		customerStore.QuotationInvoicePaidCount = stats.InvoicePaidCount
		customerStore.QuotationInvoiceNotPaidCount = stats.InvoiceNotPaidCount
		customerStore.QuotationInvoicePaidPartiallyCount = stats.InvoicePaidPartiallyCount
		customerStore.QuotationInvoiceAmount = stats.InvoiceAmount
		customerStore.QuotationInvoicePaidAmount = stats.InvoicePaidAmount
		customerStore.QuotationInvoiceBalanceAmount = stats.InvoiceBalanceAmount
		customerStore.QuotationInvoiceProfit = stats.InvoiceProfit
		customerStore.QuotationInvoiceLoss = stats.InvoiceLoss
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:                            storeID,
			StoreName:                          store.Name,
			StoreNameInArabic:                  store.NameInArabic,
			QuotationInvoiceCount:              stats.InvoiceCount,
			QuotationInvoicePaidCount:          stats.InvoicePaidCount,
			QuotationInvoiceNotPaidCount:       stats.InvoiceNotPaidCount,
			QuotationInvoicePaidPartiallyCount: stats.InvoicePaidPartiallyCount,
			QuotationInvoiceAmount:             stats.InvoiceAmount,
			QuotationInvoicePaidAmount:         stats.InvoicePaidAmount,
			QuotationInvoiceBalanceAmount:      stats.InvoiceBalanceAmount,
			QuotationInvoiceProfit:             stats.InvoiceProfit,
			QuotationInvoiceLoss:               stats.InvoiceLoss,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (customer *Customer) SetCustomerQuotationStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerQuotationStats

	filter := map[string]interface{}{
		"store_id":    storeID,
		"customer_id": customer.ID,
		"type":        "quotation",
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":              nil,
				"quotation_count":  bson.M{"$sum": 1},
				"quotation_amount": bson.M{"$sum": "$net_total"},
				"quotation_profit": bson.M{"$sum": "$net_profit"},
				"quotation_loss":   bson.M{"$sum": "$loss"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
		stats.QuotationAmount = RoundFloat(stats.QuotationAmount, 2)
		stats.QuotationProfit = RoundFloat(stats.QuotationProfit, 2)
		stats.QuotationLoss = RoundFloat(stats.QuotationLoss, 2)
	}

	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if len(customer.Stores) == 0 {
		customer.Stores = map[string]CustomerStore{}
	}

	if customerStore, ok := customer.Stores[storeID.Hex()]; ok {
		customerStore.StoreID = storeID
		customerStore.StoreName = store.Name
		customerStore.StoreNameInArabic = store.NameInArabic
		customerStore.QuotationCount = stats.QuotationCount
		customerStore.QuotationAmount = stats.QuotationAmount
		customerStore.QuotationProfit = stats.QuotationProfit
		customerStore.QuotationLoss = stats.QuotationLoss
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:           storeID,
			StoreName:         store.Name,
			StoreNameInArabic: store.NameInArabic,
			QuotationCount:    stats.QuotationCount,
			QuotationAmount:   stats.QuotationAmount,
			QuotationProfit:   stats.QuotationProfit,
			QuotationLoss:     stats.QuotationLoss,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (quotation *Quotation) SetCustomerQuotationStats() error {
	if quotation.CustomerID == nil || quotation.CustomerID.IsZero() {
		return nil
	}

	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(quotation.CustomerID, map[string]interface{}{})
	if err != nil {
		return err
	}

	if quotation.Type == "quotation" {
		err = customer.SetCustomerQuotationStatsByStoreID(*quotation.StoreID)
		if err != nil {
			return err
		}
	} else if quotation.Type == "invoice" {
		err = customer.SetCustomerQuotationInvoiceStatsByStoreID(*quotation.StoreID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (quotation *Quotation) GetPayments() (models []QuotationPayment, err error) {
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation_payment")
	ctx := context.Background()
	findOptions := options.Find()

	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"quotation_id": quotation.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching order payment history" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	//	log.Print("Starting for")
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		//log.Print("Loop")
		err := cur.Err()
		if err != nil {
			return models, errors.New("Cursor error:" + err.Error())
		}
		model := QuotationPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		models = append(models, model)

	} //end for loop

	return models, nil
}

func (quotation *Quotation) AdjustPayments() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if !store.Settings.AllowAdjustSameDatePayments {
		return nil
	}

	for i, quotationPayment := range quotation.Payments {
		if IsDateTimesEqual(quotation.Date, quotationPayment.Date) {
			newTime := quotation.Payments[i].Date.Add(1 * time.Minute)
			quotation.Payments[i].Date = &newTime
			err = quotation.Update()
			if err != nil {
				return err
			}
		}
	}

	quotationPayments, err := quotation.GetPayments()
	if err != nil {
		return err
	}

	for i, payment := range quotationPayments {
		if IsDateTimesEqual(quotation.Date, payment.Date) {
			newTime := quotationPayments[i].Date.Add(1 * time.Minute)
			quotationPayments[i].Date = &newTime
			err = quotationPayments[i].Update()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (quotation *Quotation) DoAccounting() error {
	if quotation.Type == "quotation" {
		return nil
	}

	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if !store.Settings.QuotationInvoiceAccounting {
		return nil
	}

	err = quotation.AdjustPayments()
	if err != nil {
		return errors.New("error adjusting payments: " + err.Error())
	}

	ledger, err := quotation.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (quotation *Quotation) UndoAccounting() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(quotation.ID, *quotation.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return errors.New("Error finding ledger by reference id: " + err.Error())
	}

	if err == mongo.ErrNoDocuments {
		return nil
	}

	ledgerAccounts := map[string]Account{}

	if ledger != nil {
		ledgerAccounts, err = ledger.GetRelatedAccounts()
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("Error getting related accounts: " + err.Error())
		}
	}

	err = store.RemoveLedgerByReferenceID(quotation.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(quotation.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

var extraQuotationSalesPayments []QuotationPayment

func (quotation *Quotation) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var customer *Customer

	if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(quotation.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	cashAccount, err := store.CreateAccountIfNotExists(quotation.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(quotation.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	salesAccount, err := store.CreateAccountIfNotExists(quotation.StoreID, nil, nil, "Sales", nil, nil)
	if err != nil {
		return nil, err
	}

	cashDiscountAllowedAccount, err := store.CreateAccountIfNotExists(quotation.StoreID, nil, nil, "Cash discount allowed", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(quotation.Payments) > 0 {
		firstPaymentDate = quotation.Payments[0].Date
	}

	if len(quotation.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(quotation.Date, firstPaymentDate)) {
		//Case: UnPaid
		customerName := ""
		var referenceID *primitive.ObjectID
		customerVATNo := ""
		customerPhone := ""
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = customer.VATNo
			customerPhone = customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}

		referenceModel := "customer"
		customerAccount, err := store.CreateAccountIfNotExists(
			quotation.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			&customerPhone,
			&customerVATNo,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidQuotationSale(
			quotation,
			customerAccount,
			salesAccount,
			cashDiscountAllowedAccount,
		)...)
	}

	if len(quotation.Payments) > 0 {
		totalSalesPaidAmount = float64(0.00)
		extraSalesAmountPaid = float64(0.00)
		extraQuotationSalesPayments = []QuotationPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupQuotationSalesPaymentsByDatetime(quotation.Payments)
		//fmt.Printf("%+v", paymentsByDatetime)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForQuotationSalesPaymentsByDatetime(
				quotation,
				customer,
				cashAccount,
				bankAccount,
				salesAccount,
				paymentByDatetime,
				cashDiscountAllowedAccount,
				paymentsByDatetimeNumber,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
			paymentsByDatetimeNumber++
		}

		if quotation.BalanceAmount < 0 && len(extraSalesPayments) > 0 {
			newJournals, err := MakeJournalsForQuotationSalesExtraPayments(
				quotation,
				customer,
				cashAccount,
				bankAccount,
				extraQuotationSalesPayments,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
		}

		totalSalesPaidAmount = float64(0.00)
		extraSalesAmountPaid = float64(0.00)

	}

	ledger = &Ledger{
		StoreID:        quotation.StoreID,
		ReferenceID:    quotation.ID,
		ReferenceModel: "quotation_sales",
		ReferenceCode:  quotation.Code,
		Journals:       journals,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}

	err = ledger.Insert()
	if err != nil {
		return nil, err
	}

	return ledger, nil
}

func MakeJournalsForUnpaidQuotationSale(
	quotation *Quotation,
	customerAccount *Account,
	salesAccount *Account,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()

	groupID := primitive.NewObjectID()

	journals := []Journal{}

	balanceAmount := RoundFloat((quotation.NetTotal - quotation.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          quotation.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         balanceAmount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if quotation.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          quotation.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         quotation.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          quotation.Date,
		AccountID:     salesAccount.ID,
		AccountNumber: salesAccount.Number,
		AccountName:   salesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        quotation.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

// Regroup sales payments by datetime
func RegroupQuotationSalesPaymentsByDatetime(payments []QuotationPayment) [][]QuotationPayment {
	paymentsByDatetime := map[string][]QuotationPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
	}

	paymentsByDatetime2 := [][]QuotationPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

func MakeJournalsForQuotationSalesPaymentsByDatetime(
	quotation *Quotation,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	salesAccount *Account,
	payments []QuotationPayment,
	cashDiscountAllowedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	groupID := primitive.NewObjectID()

	journals := []Journal{}
	totalPayment := float64(0.00)

	var firstPaymentDate *time.Time
	if len(payments) > 0 {
		firstPaymentDate = payments[0].Date
	}

	for _, payment := range payments {
		totalSalesPaidAmount += payment.Amount
		if totalSalesPaidAmount > (quotation.NetTotal - quotation.CashDiscount) {
			extraSalesAmountPaid = RoundFloat((totalSalesPaidAmount - (quotation.NetTotal - quotation.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraSalesAmountPaid > 0 {
			skip := false
			if extraSalesAmountPaid < payment.Amount {
				extraAmount := extraSalesAmountPaid
				extraSalesPayments = append(extraSalesPayments, SalesPayment{
					Date:   payment.Date,
					Amount: extraAmount,
					Method: payment.Method,
				})
				amount = RoundFloat((payment.Amount - extraSalesAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraSalesAmountPaid = 0
			} else if extraSalesAmountPaid >= payment.Amount {
				extraSalesPayments = append(extraSalesPayments, SalesPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})

				skip = true
				extraSalesAmountPaid = RoundFloat((extraSalesAmountPaid - payment.Amount), 2)
			}

			if skip {
				continue
			}

		}

		cashReceivingAccount := Account{}
		if payment.Method == "cash" {
			cashReceivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashReceivingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
			/*
				referenceModel := "customer"
				customerAccount, err := store.CreateAccountIfNotExists(
					order.StoreID,
					&customer.ID,
					&referenceModel,
					customer.Name,
					&customer.Phone,
					&customer.VATNo,
				)
				if err != nil {
					return nil, err
				}
				cashReceivingAccount = *customerAccount
			*/
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashReceivingAccount.ID,
			AccountNumber: cashReceivingAccount.Number,
			AccountName:   cashReceivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
		totalPayment += amount
	}

	if quotation.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(quotation.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          quotation.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         quotation.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	balanceAmount := RoundFloat(((quotation.NetTotal - quotation.CashDiscount) - totalPayment), 2)

	//Asset or debt increased
	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(quotation.Date, firstPaymentDate) {
		referenceModel := "customer"
		customerName := ""
		var referenceID *primitive.ObjectID
		var customerVATNo *string
		var customerPhone *string
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = &customer.VATNo
			customerPhone = &customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}

		customerAccount, err := store.CreateAccountIfNotExists(
			quotation.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}

		journals = append(journals, Journal{
			Date:          quotation.Date,
			AccountID:     customerAccount.ID,
			AccountNumber: customerAccount.Number,
			AccountName:   customerAccount.Name,
			DebitOrCredit: "debit",
			Debit:         balanceAmount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(quotation.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          quotation.Date,
			AccountID:     salesAccount.ID,
			AccountNumber: salesAccount.Number,
			AccountName:   salesAccount.Name,
			DebitOrCredit: "credit",
			Credit:        quotation.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(quotation.Date, firstPaymentDate) {
		referenceModel := "customer"
		customerName := ""
		var referenceID *primitive.ObjectID
		var customerVATNo *string
		var customerPhone *string
		if customer != nil {
			customerName = customer.Name
			referenceID = &customer.ID
			customerVATNo = &customer.VATNo
			customerPhone = &customer.Phone
		} else {
			customerName = "Customer Accounts - Unknown"
			referenceID = nil
		}

		customerAccount, err := store.CreateAccountIfNotExists(
			quotation.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}

		totalPayment = RoundFloat(totalPayment, 2)

		if totalPayment > 0 {
			journals = append(journals, Journal{
				Date:          firstPaymentDate,
				AccountID:     customerAccount.ID,
				AccountNumber: customerAccount.Number,
				AccountName:   customerAccount.Name,
				DebitOrCredit: "credit",
				Credit:        totalPayment,
				GroupID:       groupID,
				CreatedAt:     &now,
				UpdatedAt:     &now,
			})
		}
	}

	return journals, nil
}

func MakeJournalsForQuotationSalesExtraPayments(
	quotation *Quotation,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []QuotationPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	journals := []Journal{}
	groupID := primitive.NewObjectID()

	var lastPaymentDate *time.Time
	if len(extraPayments) > 0 {
		lastPaymentDate = extraPayments[len(extraPayments)-1].Date
	}

	for _, payment := range extraPayments {
		cashReceivingAccount := Account{}
		if payment.Method == "cash" {
			cashReceivingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashReceivingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
			/*
				referenceModel := "customer"
				customerAccount, err := store.CreateAccountIfNotExists(
					order.StoreID,
					&customer.ID,
					&referenceModel,
					customer.Name,
					&customer.Phone,
					&customer.VATNo,
				)
				if err != nil {
					return nil, err
				}
				cashReceivingAccount = *customerAccount
			*/
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashReceivingAccount.ID,
			AccountNumber: cashReceivingAccount.Number,
			AccountName:   cashReceivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	} //end for

	referenceModel := "customer"
	customerName := ""
	var referenceID *primitive.ObjectID
	var customerVATNo *string
	var customerPhone *string
	if customer != nil {
		customerName = customer.Name
		referenceID = &customer.ID
		customerVATNo = &customer.VATNo
		customerPhone = &customer.Phone
	} else {
		customerName = "Customer Accounts - Unknown"
		referenceID = nil
	}

	customerAccount, err := store.CreateAccountIfNotExists(
		quotation.StoreID,
		referenceID,
		&referenceModel,
		customerName,
		customerPhone,
		customerVATNo,
	)
	if err != nil {
		return nil, err
	}
	journals = append(journals, Journal{
		Date:          lastPaymentDate,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "credit",
		Credit:        quotation.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals, nil
}

func (quotation *Quotation) UpdatePaymentFromReceivablePayment(
	receivablePayment ReceivablePayment,
	customerDeposit *CustomerDeposit,
) error {
	store, _ := FindStoreByID(quotation.StoreID, bson.M{})

	paymentExists := false
	for _, quotationPayment := range quotation.Payments {
		if quotationPayment.ReceivablePaymentID != nil && quotationPayment.ReceivablePaymentID.Hex() == receivablePayment.ID.Hex() {
			paymentExists = true
			quotationPayment, err := store.FindQuotationPaymentByID(&quotationPayment.ID, bson.M{})
			if err != nil {
				return errors.New("error finding sales payment: " + err.Error())
			}

			quotationPayment.Amount = receivablePayment.Amount
			quotationPayment.Date = receivablePayment.Date
			quotationPayment.UpdatedAt = receivablePayment.UpdatedAt
			quotationPayment.CreatedAt = receivablePayment.CreatedAt
			quotationPayment.UpdatedBy = receivablePayment.UpdatedBy
			quotationPayment.CreatedBy = receivablePayment.CreatedBy
			quotationPayment.ReceivableID = &customerDeposit.ID

			err = quotationPayment.Update()
			if err != nil {
				return err
			}
			break
		}
	}

	if !paymentExists {
		newQuotationPayment := QuotationPayment{
			QuotationID:         &quotation.ID,
			QuotationCode:       quotation.Code,
			Amount:              receivablePayment.Amount,
			Date:                receivablePayment.Date,
			Method:              "customer_account",
			ReceivablePaymentID: &receivablePayment.ID,
			ReceivableID:        &customerDeposit.ID,
			CreatedBy:           receivablePayment.CreatedBy,
			UpdatedBy:           receivablePayment.UpdatedBy,
			CreatedAt:           receivablePayment.CreatedAt,
			UpdatedAt:           receivablePayment.UpdatedAt,
			StoreID:             quotation.StoreID,
		}
		err := newQuotationPayment.Insert()
		if err != nil {
			return errors.New("error inserting quotation payment: " + err.Error())
		}
	}

	return nil
}

func (quotation *Quotation) DeletePaymentsByReceivablePaymentID(receivablePaymentID primitive.ObjectID) error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("quotation_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"receivable_payment_id": receivablePaymentID})
	if err != nil {
		return err
	}
	return nil
}
