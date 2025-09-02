package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
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

type QuotationSalesReturnProduct struct {
	ProductID                  primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                       string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic               string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode                   string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PrefixPartNumber           string             `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber                 string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity                   float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit                       string             `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitPrice                  float64            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	UnitPriceWithVAT           float64            `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	PurchaseUnitPrice          float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	PurchaseUnitPriceWithVAT   float64            `bson:"purchase_unit_price_with_vat,omitempty" json:"purchase_unit_price_with_vat,omitempty"`
	UnitDiscount               float64            `bson:"unit_discount" json:"unit_discount"`
	UnitDiscountPercent        float64            `bson:"unit_discount_percent" json:"unit_discount_percent"`
	UnitDiscountWithVAT        float64            `bson:"unit_discount_with_vat" json:"unit_discount_with_vat"`
	UnitDiscountPercentWithVAT float64            `bson:"unit_discount_percent_with_vat" json:"unit_discount_percent_with_vat"`
	Profit                     float64            `bson:"profit" json:"profit"`
	Loss                       float64            `bson:"loss" json:"loss"`
	Selected                   bool               `bson:"selected" json:"selected"`
}

// QuotationSalesReturn : QuotationSalesReturn structure
type QuotationSalesReturn struct {
	ID                primitive.ObjectID            `json:"id,omitempty" bson:"_id,omitempty"`
	QuotationID       *primitive.ObjectID           `json:"quotation_id,omitempty" bson:"quotation_id,omitempty"`
	QuotationCode     string                        `bson:"quotation_code,omitempty" json:"quotation_code,omitempty"`
	Date              *time.Time                    `bson:"date,omitempty" json:"date,omitempty"`
	DateStr           string                        `json:"date_str,omitempty" bson:"-"`
	InvoiceCountValue int64                         `bson:"invoice_count_value,omitempty" json:"invoice_count_value,omitempty"`
	Code              string                        `bson:"code,omitempty" json:"code,omitempty"`
	UUID              string                        `bson:"uuid,omitempty" json:"uuid,omitempty"`
	Hash              string                        `bson:"hash,omitempty" json:"hash,omitempty"`
	PrevHash          string                        `bson:"prev_hash,omitempty" json:"prev_hash,omitempty"`
	CSID              string                        `bson:"csid,omitempty" json:"csid,omitempty"`
	StoreID           *primitive.ObjectID           `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID        *primitive.ObjectID           `json:"customer_id" bson:"customer_id"`
	Store             *Store                        `json:"store,omitempty"`
	Customer          *Customer                     `json:"customer,omitempty"`
	Products          []QuotationSalesReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	ReceivedBy        *primitive.ObjectID           `json:"received_by,omitempty" bson:"received_by,omitempty"`
	ReceivedByUser    *User                         `json:"received_by_user,omitempty"`
	//ReceivedBySignatureID   *primitive.ObjectID  `json:"received_by_signature_id,omitempty" bson:"received_by_signature_id,omitempty"`
	//ReceivedBySignatureName string               `json:"received_by_signature_name,omitempty" bson:"received_by_signature_name,omitempty"`
	//ReceivedBySignature     *Signature           `json:"received_by_signature,omitempty"`
	//SignatureDate     *time.Time           `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	//SignatureDateStr  string               `json:"signature_date_str,omitempty"`
	ShippingOrHandlingFees float64  `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	VatPercent             *float64 `bson:"vat_percent" json:"vat_percent"`
	Discount               float64  `bson:"discount" json:"discount"`
	DiscountPercent        float64  `bson:"discount_percent" json:"discount_percent"`
	DiscountWithVAT        float64  `bson:"discount_with_vat" json:"discount_with_vat"`
	DiscountPercentWithVAT float64  `bson:"discount_percent_with_vat" json:"discount_percent_with_vat"`
	ReturnDiscountWithVAT  float64  `bson:"return_discount_with_vat" json:"return_discount_vat"`
	Status                 string   `bson:"status,omitempty" json:"status,omitempty"`
	StockAdded             bool     `bson:"stock_added,omitempty" json:"stock_added,omitempty"`
	TotalQuantity          float64  `bson:"total_quantity" json:"total_quantity"`
	VatPrice               float64  `bson:"vat_price" json:"vat_price"`
	Total                  float64  `bson:"total" json:"total"`
	TotalWithVAT           float64  `bson:"total_with_vat" json:"total_with_vat"`
	NetTotal               float64  `bson:"net_total" json:"net_total"`
	ActualVatPrice         float64  `bson:"actual_vat_price" json:"actual_vat_price"`
	ActualTotal            float64  `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT     float64  `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	ActualNetTotal         float64  `bson:"actual_net_total" json:"actual_net_total"`
	RoundingAmount         float64  `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount     bool     `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	CashDiscount           float64  `bson:"cash_discount" json:"cash_discount"`
	PaymentMethods         []string `json:"payment_methods" bson:"payment_methods"`
	PaymentStatus          string   `bson:"payment_status" json:"payment_status"`
	//Deleted           bool                 `bson:"deleted,omitempty" json:"deleted,omitempty"`
	//DeletedBy         *primitive.ObjectID  `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	//DeletedByUser     *User                `json:"deleted_by_user,omitempty"`
	//DeletedAt         *time.Time           `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser *User               `json:"created_by_user,omitempty"`
	UpdatedByUser *User               `json:"updated_by_user,omitempty"`
	//ReceivedByName   string               `json:"received_by_name,omitempty" bson:"received_by_name,omitempty"`
	CustomerName        string                        `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic  string                        `json:"customer_name_arabic" bson:"customer_name_arabic"`
	StoreName           string                        `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName       string                        `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName       string                        `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName       string                        `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Profit              float64                       `bson:"profit" json:"profit"`
	NetProfit           float64                       `bson:"net_profit" json:"net_profit"`
	Loss                float64                       `bson:"loss" json:"loss"`
	NetLoss             float64                       `bson:"net_loss" json:"net_loss"`
	TotalPaymentPaid    float64                       `bson:"total_payment_paid" json:"total_payment_paid"`
	BalanceAmount       float64                       `bson:"balance_amount" json:"balance_amount"`
	Payments            []QuotationSalesReturnPayment `bson:"payments" json:"payments"`
	PaymentsInput       []QuotationSalesReturnPayment `bson:"-" json:"payments_input"`
	PaymentsCount       int64                         `bson:"payments_count" json:"payments_count"`
	Zatca               ZatcaReporting                `bson:"zatca,omitempty" json:"zatca,omitempty"`
	Remarks             string                        `bson:"remarks" json:"remarks"`
	Phone               string                        `bson:"phone" json:"phone"`
	VatNo               string                        `bson:"vat_no" json:"vat_no"`
	Address             string                        `bson:"address" json:"address"`
	EnableReportToZatca bool                          `json:"enable_report_to_zatca" bson:"-"`
}

func (quotationSalesReturn *QuotationSalesReturn) CloseQuotationSalesPayment() error {
	store, err := FindStoreByID(quotationSalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if !store.Settings.EnableAutoPaymentCloseOnReturn {
		return nil
	}

	quotation, err := store.FindQuotationByID(quotationSalesReturn.QuotationID, bson.M{})
	if err != nil {
		return err
	}

	amount := quotationSalesReturn.BalanceAmount

	if quotation.BalanceAmount < amount {
		amount = quotation.BalanceAmount
	}

	if quotation.PaymentStatus != "paid" && quotationSalesReturn.PaymentStatus != "paid" {
		newQuotationSalesPayment := QuotationPayment{
			Date:          quotationSalesReturn.Date,
			QuotationID:   &quotation.ID,
			QuotationCode: quotation.Code,
			Amount:        amount,
			Method:        "quotation_sales_return",
			CreatedAt:     quotationSalesReturn.CreatedAt,
			UpdatedAt:     quotationSalesReturn.UpdatedAt,
			StoreID:       quotationSalesReturn.StoreID,
			CreatedBy:     quotationSalesReturn.CreatedBy,
			UpdatedBy:     quotationSalesReturn.UpdatedBy,
			CreatedByName: quotationSalesReturn.CreatedByName,
			UpdatedByName: quotationSalesReturn.UpdatedByName,
			ReferenceType: "quotation_sales_return",
			ReferenceCode: quotationSalesReturn.Code,
			ReferenceID:   &quotationSalesReturn.ID,
		}
		err = newQuotationSalesPayment.Insert()
		if err != nil {
			return err
		}

		quotation.Payments = append(quotation.Payments, newQuotationSalesPayment)

		err = quotation.Update()
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

		//Sales Return
		newQuotationSalesReturnPayment := QuotationSalesReturnPayment{
			Date:                     quotationSalesReturn.Date,
			QuotationSalesReturnID:   &quotationSalesReturn.ID,
			QuotationSalesReturnCode: quotationSalesReturn.Code,
			QuotationID:              &quotation.ID,
			QuotationCode:            quotation.Code,
			Amount:                   amount,
			Method:                   "quotation_sales",
			CreatedAt:                quotationSalesReturn.CreatedAt,
			UpdatedAt:                quotationSalesReturn.UpdatedAt,
			StoreID:                  quotationSalesReturn.StoreID,
			CreatedBy:                quotationSalesReturn.CreatedBy,
			UpdatedBy:                quotationSalesReturn.UpdatedBy,
			CreatedByName:            quotationSalesReturn.CreatedByName,
			UpdatedByName:            quotationSalesReturn.UpdatedByName,
			ReferenceType:            "quotation_sales",
			ReferenceCode:            quotation.Code,
			ReferenceID:              &quotation.ID,
		}

		log.Print("newQuotationSalesReturnPayment.Re:" + newQuotationSalesReturnPayment.ReferenceCode)
		err = newQuotationSalesReturnPayment.Insert()
		if err != nil {
			return err
		}

		quotationSalesReturn.Payments = append(quotationSalesReturn.Payments, newQuotationSalesReturnPayment)

		err = quotationSalesReturn.Update()
		if err != nil {
			return err
		}

		_, err = quotationSalesReturn.SetPaymentStatus()
		if err != nil {
			return err
		}

		err = quotationSalesReturn.Update()
		if err != nil {
			return err
		}

	}

	return nil
}

func (quotationSalesReturn *QuotationSalesReturn) SetProductsStock() (err error) {
	store, err := FindStoreByID(quotationSalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(quotationSalesReturn.Products) == 0 {
		return nil
	}

	for _, quotationSalesReturnProduct := range quotationSalesReturn.Products {
		if !quotationSalesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&quotationSalesReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		err = product.SetStock()
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

func (model *QuotationSalesReturn) SetPostBalances() error {
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

func (quotationsalesReturn *QuotationSalesReturn) DeletePaymentsByPayablePaymentID(payablePaymentID primitive.ObjectID) error {
	//log.Printf("Clearing QuotationSales history of quotation id:%s", quotation.Code)
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"payable_payment_id": payablePaymentID})
	if err != nil {
		return err
	}
	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) UpdatePaymentFromPayablePayment(
	payablePayment PayablePayment,
	customerWithdrawal *CustomerWithdrawal,
) error {
	store, _ := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})

	paymentExists := false
	for _, quotationsalesReturnPayment := range quotationsalesReturn.Payments {
		if quotationsalesReturnPayment.PayablePaymentID != nil && quotationsalesReturnPayment.PayablePaymentID.Hex() == payablePayment.ID.Hex() {
			paymentExists = true
			quotationsalesReturnPaymentObj, err := store.FindQuotationSalesReturnPaymentByID(&quotationsalesReturnPayment.ID, bson.M{})
			if err != nil {
				return errors.New("error finding quotationsales payment: " + err.Error())
			}

			quotationsalesReturnPaymentObj.Amount = payablePayment.Amount
			quotationsalesReturnPaymentObj.Date = payablePayment.Date
			quotationsalesReturnPaymentObj.Method = payablePayment.Method
			quotationsalesReturnPaymentObj.UpdatedAt = payablePayment.UpdatedAt
			quotationsalesReturnPaymentObj.CreatedAt = payablePayment.CreatedAt
			quotationsalesReturnPaymentObj.UpdatedBy = payablePayment.UpdatedBy
			quotationsalesReturnPaymentObj.CreatedBy = payablePayment.CreatedBy
			quotationsalesReturnPaymentObj.PayableID = &customerWithdrawal.ID
			quotationsalesReturnPaymentObj.ReferenceType = "customer_withdrawal"
			quotationsalesReturnPaymentObj.ReferenceID = &customerWithdrawal.ID
			quotationsalesReturnPaymentObj.ReferenceCode = customerWithdrawal.Code

			err = quotationsalesReturnPaymentObj.Update()
			if err != nil {
				return err
			}
			break
		}
	}

	if !paymentExists {
		newQuotationSalesReturnPayment := QuotationSalesReturnPayment{
			QuotationSalesReturnID:   &quotationsalesReturn.ID,
			QuotationSalesReturnCode: quotationsalesReturn.Code,
			QuotationID:              quotationsalesReturn.QuotationID,
			QuotationCode:            quotationsalesReturn.QuotationCode,
			Amount:                   payablePayment.Amount,
			Date:                     payablePayment.Date,
			Method:                   payablePayment.Method,
			PayablePaymentID:         &payablePayment.ID,
			PayableID:                &customerWithdrawal.ID,
			CreatedBy:                payablePayment.CreatedBy,
			UpdatedBy:                payablePayment.UpdatedBy,
			CreatedAt:                payablePayment.CreatedAt,
			UpdatedAt:                payablePayment.UpdatedAt,
			StoreID:                  quotationsalesReturn.StoreID,
			ReferenceType:            "customer_withdrawal",
			ReferenceID:              &customerWithdrawal.ID,
			ReferenceCode:            customerWithdrawal.Code,
		}
		err := newQuotationSalesReturnPayment.Insert()
		if err != nil {
			return errors.New("error inserting quotationsales payment: " + err.Error())
		}
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) AddPayments() error {
	for _, payment := range quotationsalesReturn.PaymentsInput {
		quotationsalesReturnPayment := QuotationSalesReturnPayment{
			QuotationSalesReturnID:   &quotationsalesReturn.ID,
			QuotationSalesReturnCode: quotationsalesReturn.Code,
			QuotationID:              quotationsalesReturn.QuotationID,
			QuotationCode:            quotationsalesReturn.QuotationCode,
			Amount:                   payment.Amount,
			Method:                   payment.Method,
			Date:                     payment.Date,
			CreatedAt:                quotationsalesReturn.CreatedAt,
			UpdatedAt:                quotationsalesReturn.UpdatedAt,
			CreatedBy:                quotationsalesReturn.CreatedBy,
			CreatedByName:            quotationsalesReturn.CreatedByName,
			UpdatedBy:                quotationsalesReturn.UpdatedBy,
			UpdatedByName:            quotationsalesReturn.UpdatedByName,
			StoreID:                  quotationsalesReturn.StoreID,
			StoreName:                quotationsalesReturn.StoreName,
		}
		err := quotationsalesReturnPayment.Insert()
		if err != nil {
			return err
		}
	}
	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) UpdatePayments() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	quotationsalesReturn.SetPaymentStatus()
	now := time.Now()
	for _, payment := range quotationsalesReturn.PaymentsInput {
		if payment.ID.IsZero() {
			//Create new
			quotationsalesReturnPayment := QuotationSalesReturnPayment{
				QuotationSalesReturnID:   &quotationsalesReturn.ID,
				QuotationSalesReturnCode: quotationsalesReturn.Code,
				QuotationID:              quotationsalesReturn.QuotationID,
				QuotationCode:            quotationsalesReturn.QuotationCode,
				Amount:                   payment.Amount,
				Method:                   payment.Method,
				Date:                     payment.Date,
				CreatedAt:                &now,
				UpdatedAt:                &now,
				CreatedBy:                quotationsalesReturn.CreatedBy,
				CreatedByName:            quotationsalesReturn.CreatedByName,
				UpdatedBy:                quotationsalesReturn.UpdatedBy,
				UpdatedByName:            quotationsalesReturn.UpdatedByName,
				StoreID:                  quotationsalesReturn.StoreID,
				StoreName:                quotationsalesReturn.StoreName,
			}
			err := quotationsalesReturnPayment.Insert()
			if err != nil {
				return err
			}

		} else {
			//Update
			quotationsalesReturnPayment, err := store.FindQuotationSalesReturnPaymentByID(&payment.ID, bson.M{})
			if err != nil {
				return err
			}

			quotationsalesReturnPayment.Date = payment.Date
			quotationsalesReturnPayment.Amount = payment.Amount
			quotationsalesReturnPayment.Method = payment.Method
			quotationsalesReturnPayment.UpdatedAt = &now
			quotationsalesReturnPayment.UpdatedBy = quotationsalesReturn.UpdatedBy
			quotationsalesReturnPayment.UpdatedByName = quotationsalesReturn.UpdatedByName
			err = quotationsalesReturnPayment.Update()
			if err != nil {
				return err
			}
		}

	}

	//Deleting payments

	paymentsToDelete := []QuotationSalesReturnPayment{}

	for _, payment := range quotationsalesReturn.Payments {
		found := false
		for _, paymentInput := range quotationsalesReturn.PaymentsInput {
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
		payment.DeletedBy = quotationsalesReturn.UpdatedBy
		err := payment.Update()
		if err != nil {
			return err
		}

		err = quotationsalesReturn.RemoveInvoiceFromCustomerPayablePayment(&payment)
		if err != nil {
			return err
		}
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) RemoveInvoiceFromCustomerPayablePayment(quotationsalesReturnPayment *QuotationSalesReturnPayment) error {
	store, _ := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	//Remove Invoice from Customer receivable payment
	if quotationsalesReturnPayment.PayablePaymentID != nil && !quotationsalesReturnPayment.PayablePaymentID.IsZero() {
		customerWithdrawal, err := store.FindCustomerWithdrawalByID(quotationsalesReturnPayment.PayableID, bson.M{})
		if err != nil {
			return err
		}

		for i, payablePayment := range customerWithdrawal.Payments {
			if payablePayment.InvoiceID != nil && !payablePayment.InvoiceID.IsZero() {
				if payablePayment.ID.Hex() == quotationsalesReturnPayment.PayablePaymentID.Hex() &&
					customerWithdrawal.ID.Hex() == quotationsalesReturnPayment.PayableID.Hex() &&
					payablePayment.InvoiceID.Hex() == quotationsalesReturn.ID.Hex() {
					blankString := ""
					customerWithdrawal.Payments[i].InvoiceCode = &blankString
					customerWithdrawal.Payments[i].InvoiceID = nil
					customerWithdrawal.Payments[i].InvoiceType = &blankString
					err = customerWithdrawal.Update()
					if err != nil {
						return err
					}

					err = customerWithdrawal.UndoAccounting()
					if err != nil {
						return err
					}

					err = customerWithdrawal.DoAccounting()
					if err != nil {
						return err
					}
				}
			}
		}

	}
	return nil
}

// DiskQuotaUsageResult payload for disk quota usage
type QuotationSalesReturnStats struct {
	ID                                 *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal                           float64             `json:"net_total" bson:"net_total"`
	VatPrice                           float64             `json:"vat_price" bson:"vat_price"`
	Discount                           float64             `json:"discount" bson:"discount"`
	CashDiscount                       float64             `json:"cash_discount" bson:"cash_discount"`
	NetProfit                          float64             `json:"net_profit" bson:"net_profit"`
	NetLoss                            float64             `json:"net_loss" bson:"net_loss"`
	PaidQuotationSalesReturn           float64             `json:"paid_quotation_sales_return" bson:"paid_quotation_sales_return"`
	UnPaidQuotationSalesReturn         float64             `json:"unpaid_quotation_sales_return" bson:"unpaid_quotation_sales_return"`
	CashQuotationSalesReturn           float64             `json:"cash_quotation_sales_return" bson:"cash_quotation_sales_return"`
	BankAccountQuotationSalesReturn    float64             `json:"bank_account_quotation_sales_return" bson:"bank_account_quotation_sales_return"`
	ShippingOrHandlingFees             float64             `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
	QuotationSalesReturnCount          int64               `json:"quotation_sales_return_count" bson:"quotation_sales_return_count"`
	QuotationSalesQuotationSalesReturn float64             `json:"quotation_sales_quotation_sales_return" bson:"quotation_sales_quotation_sales_return"`
}

func (store *Store) GetQuotationSalesReturnStats(filter map[string]interface{}) (stats QuotationSalesReturnStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"net_total":              bson.M{"$sum": "$net_total"},
				"vat_price":              bson.M{"$sum": "$vat_price"},
				"discount":               bson.M{"$sum": "$discount"},
				"cash_discount":          bson.M{"$sum": "$cash_discount"},
				"net_profit":             bson.M{"$sum": "$net_profit"},
				"net_loss":               bson.M{"$sum": "$net_loss"},
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
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
				"unpaid_quotation_sales_return": bson.M{"$sum": "$balance_amount"},
				"cash_quotation_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"bank_account_quotation_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"quotation_sales_quotation_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "quotation_sales"}},
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
		stats.NetLoss = RoundFloat(stats.NetLoss, 2)
		stats.CashDiscount = RoundFloat(stats.CashDiscount, 2)
	}
	return stats, nil
}

func (quotationsalesreturn *QuotationSalesReturn) AttributesValueChangeEvent(quotationsalesreturnOld *QuotationSalesReturn) error {

	if quotationsalesreturn.Status != quotationsalesreturnOld.Status {

		//if quotationsalesreturn.Status == "delivered" || quotationsalesreturn.Status == "dispatched" {

		err := quotationsalesreturnOld.AddStock()
		if err != nil {
			return err
		}

		/*
			err = quotationsalesreturn.RemoveStock()
			if err != nil {
				return err
			}
		*/
		//}
	}

	return nil
}

func (quotationsalesreturn *QuotationSalesReturn) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(quotationsalesreturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if quotationsalesreturn.StoreID != nil {
		store, err := FindStoreByID(quotationsalesreturn.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotationsalesreturn.StoreName = store.Name
	}

	if quotationsalesreturn.CustomerID != nil && !quotationsalesreturn.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(quotationsalesreturn.CustomerID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1})
		if err != nil {
			return err
		}
		quotationsalesreturn.CustomerName = customer.Name
		quotationsalesreturn.CustomerNameArabic = customer.NameInArabic
	} else {
		quotationsalesreturn.CustomerName = ""
		quotationsalesreturn.CustomerNameArabic = ""
	}

	/*
		if quotationsalesreturn.ReceivedBy != nil {
			receivedByUser, err := FindUserByID(quotationsalesreturn.ReceivedBy, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			quotationsalesreturn.ReceivedByName = receivedByUser.Name
		}
	*/

	/*
		if quotationsalesreturn.ReceivedBySignatureID != nil {
			receivedBySignature, err := FindSignatureByID(quotationsalesreturn.ReceivedBySignatureID, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			quotationsalesreturn.ReceivedBySignatureName = receivedBySignature.Name
		}
	*/

	if quotationsalesreturn.CreatedBy != nil {
		createdByUser, err := FindUserByID(quotationsalesreturn.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotationsalesreturn.CreatedByName = createdByUser.Name
	}

	if quotationsalesreturn.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(quotationsalesreturn.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotationsalesreturn.UpdatedByName = updatedByUser.Name
	}

	/*
		if quotationsalesreturn.DeletedBy != nil && !quotationsalesreturn.DeletedBy.IsZero() {
			deletedByUser, err := FindUserByID(quotationsalesreturn.DeletedBy, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			quotationsalesreturn.DeletedByName = deletedByUser.Name
		}*/

	for i, product := range quotationsalesreturn.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1})
		if err != nil {
			return err
		}
		//quotationsalesreturn.Products[i].Name = productObject.Name
		quotationsalesreturn.Products[i].NameInArabic = productObject.NameInArabic
		quotationsalesreturn.Products[i].ItemCode = productObject.ItemCode
		quotationsalesreturn.Products[i].PartNumber = productObject.PartNumber
		quotationsalesreturn.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) FindNetTotal() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	quotationsalesReturn.ShippingOrHandlingFees = RoundTo2Decimals(quotationsalesReturn.ShippingOrHandlingFees)
	quotationsalesReturn.Discount = RoundTo2Decimals(quotationsalesReturn.Discount)

	quotationsalesReturn.FindTotal()

	/*
		if quotationsalesReturn.DiscountWithVAT > 0 {
			quotationsalesReturn.Discount = RoundTo2Decimals(quotationsalesReturn.DiscountWithVAT / (1 + (*quotationsalesReturn.VatPercent / 100)))
		} else if quotationsalesReturn.Discount > 0 {
			quotationsalesReturn.DiscountWithVAT = RoundTo2Decimals(quotationsalesReturn.Discount * (1 + (*quotationsalesReturn.VatPercent / 100)))
		} else {
			quotationsalesReturn.Discount = 0
			quotationsalesReturn.DiscountWithVAT = 0
		}*/

	// Apply discount to the base amount first
	baseTotal := quotationsalesReturn.Total + quotationsalesReturn.ShippingOrHandlingFees - quotationsalesReturn.Discount
	baseTotal = RoundTo2Decimals(baseTotal)

	// Now calculate VAT on the discounted base
	quotationsalesReturn.VatPrice = RoundTo2Decimals(baseTotal * (*quotationsalesReturn.VatPercent / 100))

	if store.Settings.HideQuotationInvoiceVAT {
		quotationsalesReturn.VatPrice = 0
	}

	quotationsalesReturn.NetTotal = RoundTo2Decimals(baseTotal + quotationsalesReturn.VatPrice)

	//Actual
	actualBaseTotal := quotationsalesReturn.ActualTotal + quotationsalesReturn.ShippingOrHandlingFees - quotationsalesReturn.Discount
	actualBaseTotal = RoundTo8Decimals(actualBaseTotal)

	// Now calculate VAT on the discounted base
	quotationsalesReturn.ActualVatPrice = RoundTo2Decimals(actualBaseTotal * (*quotationsalesReturn.VatPercent / 100))
	if store.Settings.HideQuotationInvoiceVAT {
		quotationsalesReturn.ActualVatPrice = 0
	}

	quotationsalesReturn.ActualNetTotal = RoundTo2Decimals(actualBaseTotal + quotationsalesReturn.ActualVatPrice)

	if quotationsalesReturn.AutoRoundingAmount {
		quotationsalesReturn.RoundingAmount = RoundTo2Decimals(quotationsalesReturn.ActualNetTotal - quotationsalesReturn.NetTotal)
	}

	quotationsalesReturn.NetTotal = RoundTo2Decimals(quotationsalesReturn.NetTotal + quotationsalesReturn.RoundingAmount)

	quotationsalesReturn.CalculateDiscountPercentage()

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) FindTotal() {
	total := float64(0.0)
	totalWithVAT := float64(0.0)

	//Actual
	actualTotal := float64(0.0)
	actualTotalWithVAT := float64(0.0)

	for i, product := range quotationsalesReturn.Products {
		if !product.Selected {
			continue
		}

		/*
			if product.UnitPriceWithVAT > 0 {
				quotationsalesReturn.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPriceWithVAT / (1 + (*quotationsalesReturn.VatPercent / 100)))
			} else if product.UnitPrice > 0 {
				quotationsalesReturn.Products[i].UnitPriceWithVAT = RoundTo2Decimals(product.UnitPrice * (1 + (*quotationsalesReturn.VatPercent / 100)))
			}

			if product.UnitDiscountWithVAT > 0 {
				quotationsalesReturn.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscountWithVAT / (1 + (*quotationsalesReturn.VatPercent / 100)))
			} else if product.UnitDiscount > 0 {
				quotationsalesReturn.Products[i].UnitDiscountWithVAT = RoundTo2Decimals(product.UnitDiscount * (1 + (*quotationsalesReturn.VatPercent / 100)))
			}

			if product.UnitDiscountPercentWithVAT > 0 {
				quotationsalesReturn.Products[i].UnitDiscountPercent = RoundTo2Decimals((product.UnitDiscount / product.UnitPrice) * 100)
			} else if product.UnitDiscountPercent > 0 {
				quotationsalesReturn.Products[i].UnitDiscountPercentWithVAT = RoundTo2Decimals((product.UnitDiscountWithVAT / product.UnitPriceWithVAT) * 100)
			}*/

		total += (product.Quantity * (quotationsalesReturn.Products[i].UnitPrice - quotationsalesReturn.Products[i].UnitDiscount))
		totalWithVAT += (product.Quantity * (quotationsalesReturn.Products[i].UnitPriceWithVAT - quotationsalesReturn.Products[i].UnitDiscountWithVAT))
		total = RoundTo2Decimals(total)
		totalWithVAT = RoundTo2Decimals(totalWithVAT)

		//Actual values
		actualTotal += (product.Quantity * (quotationsalesReturn.Products[i].UnitPrice - quotationsalesReturn.Products[i].UnitDiscount))
		actualTotal = RoundTo8Decimals(actualTotal)
		actualTotalWithVAT += (product.Quantity * (quotationsalesReturn.Products[i].UnitPriceWithVAT - quotationsalesReturn.Products[i].UnitDiscountWithVAT))
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)
	}

	quotationsalesReturn.Total = total
	quotationsalesReturn.TotalWithVAT = totalWithVAT

	//Actual
	quotationsalesReturn.ActualTotal = actualTotal
	quotationsalesReturn.ActualTotalWithVAT = actualTotalWithVAT
}

func (quotationsalesReturn *QuotationSalesReturn) CalculateDiscountPercentage() {
	if quotationsalesReturn.Discount <= 0 {
		quotationsalesReturn.DiscountPercent = 0.00
		quotationsalesReturn.DiscountPercentWithVAT = 0.00
		return
	}

	baseBeforeDiscount := quotationsalesReturn.NetTotal + quotationsalesReturn.Discount
	if baseBeforeDiscount == 0 {
		quotationsalesReturn.DiscountPercent = 0.00
		quotationsalesReturn.DiscountPercentWithVAT = 0.00
		return
	}

	percentage := (quotationsalesReturn.Discount / baseBeforeDiscount) * 100
	quotationsalesReturn.DiscountPercent = RoundTo2Decimals(percentage)

	baseBeforeDiscountWithVAT := quotationsalesReturn.NetTotal + quotationsalesReturn.DiscountWithVAT
	if baseBeforeDiscountWithVAT == 0 {
		quotationsalesReturn.DiscountPercentWithVAT = 0.00
		quotationsalesReturn.DiscountPercent = 0.00
		return
	}

	percentage = (quotationsalesReturn.DiscountWithVAT / baseBeforeDiscountWithVAT) * 100
	quotationsalesReturn.DiscountPercentWithVAT = RoundTo2Decimals(percentage)
}

/*
func (quotationsalesreturn *QuotationSalesReturn) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range quotationsalesreturn.Products {
		if !product.Selected {
			continue
		}
		netTotal += (product.Quantity * (product.UnitPrice - product.UnitDiscount))
	}

	netTotal += quotationsalesreturn.ShippingOrHandlingFees
	netTotal -= quotationsalesreturn.Discount

	if quotationsalesreturn.VatPercent != nil {
		netTotal += netTotal * (*quotationsalesreturn.VatPercent / float64(100))
	}

	quotationsalesreturn.NetTotal = ToFixed2(netTotal, 2)
}
*/

/*
func (quotationsalesReturn *QuotationSalesReturn) FindNetTotal() {
	netTotal := float64(0.0)
	quotationsalesReturn.FindTotal()
	netTotal = quotationsalesReturn.Total
	quotationsalesReturn.ShippingOrHandlingFees = RoundTo2Decimals(quotationsalesReturn.ShippingOrHandlingFees)
	quotationsalesReturn.Discount = RoundTo2Decimals(quotationsalesReturn.Discount)

	netTotal += quotationsalesReturn.ShippingOrHandlingFees
	netTotal -= quotationsalesReturn.Discount

	quotationsalesReturn.FindVatPrice()
	netTotal += quotationsalesReturn.VatPrice

	quotationsalesReturn.NetTotal = RoundTo2Decimals(netTotal)
	quotationsalesReturn.CalculateDiscountPercentage()
}

func (quotationsalesReturn *QuotationSalesReturn) CalculateDiscountPercentage() {
	if quotationsalesReturn.NetTotal == 0 {
		quotationsalesReturn.DiscountPercent = 0
	}

	if quotationsalesReturn.Discount <= 0 {
		quotationsalesReturn.DiscountPercent = 0.00
		return
	}

	percentage := (quotationsalesReturn.Discount / quotationsalesReturn.NetTotal) * 100
	quotationsalesReturn.DiscountPercent = RoundTo2Decimals(percentage) // Use rounding here
}

func (quotationsalesReturn *QuotationSalesReturn) FindTotal() {
	total := float64(0.0)
	for i, product := range quotationsalesReturn.Products {
		if !product.Selected {
			continue
		}

		quotationsalesReturn.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPrice)
		quotationsalesReturn.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscount)
		if quotationsalesReturn.Products[i].UnitDiscount > 0 {
			quotationsalesReturn.Products[i].UnitDiscountPercent = RoundTo2Decimals((quotationsalesReturn.Products[i].UnitDiscount / quotationsalesReturn.Products[i].UnitPrice) * 100)
		}

		total += RoundTo2Decimals(product.Quantity * (quotationsalesReturn.Products[i].UnitPrice - quotationsalesReturn.Products[i].UnitDiscount))
	}

	quotationsalesReturn.Total = RoundTo2Decimals(total)
}

func (quotationsalesReturn *QuotationSalesReturn) FindVatPrice() {
	vatPrice := ((*quotationsalesReturn.VatPercent / float64(100.00)) * ((quotationsalesReturn.Total + quotationsalesReturn.ShippingOrHandlingFees) - quotationsalesReturn.Discount))
	quotationsalesReturn.VatPrice = RoundTo2Decimals(vatPrice)
}
*/

/*
func (quotationsalesreturn *QuotationSalesReturn) FindTotal() {
	total := float64(0.0)
	for _, product := range quotationsalesreturn.Products {
		if !product.Selected {
			continue
		}

		total += (product.Quantity * (product.UnitPrice - product.UnitDiscount))
	}
	quotationsalesreturn.Total = RoundFloat(total, 2)
}
*/

func (quotationsalesreturn *QuotationSalesReturn) FindTotalQuantity() {
	totalQuantity := float64(0)
	for _, product := range quotationsalesreturn.Products {
		if !product.Selected {
			continue
		}

		totalQuantity += product.Quantity
	}
	quotationsalesreturn.TotalQuantity = totalQuantity
}

/*
func (model *QuotationSalesReturn) FindVatPrice() {
	vatPrice := ((*model.VatPercent / float64(100.00)) * ((model.Total + model.ShippingOrHandlingFees) - model.Discount))
	vatPrice = RoundFloat(vatPrice, 2)
	model.VatPrice = vatPrice
}
*/

func (store *Store) SearchQuotationSalesReturn(w http.ResponseWriter, r *http.Request) (quotationsalesreturns []QuotationSalesReturn, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[payment_status]"]
	if ok && len(keys[0]) >= 1 {
		paymentStatusList := strings.Split(keys[0], ",")
		if len(paymentStatusList) > 0 {
			criterias.SearchBy["payment_status"] = bson.M{"$in": paymentStatusList}
		}
	}

	keys, ok = r.URL.Query()["search[zatca.reporting_passed]"]
	if ok && len(keys[0]) >= 1 {
		value := keys[0]

		if value == "reported" {
			criterias.SearchBy["zatca.reporting_passed"] = true //ok
		} else if value == "reporting_failed" {
			//criterias.SearchBy["zatca.reporting_passed"] = bson.M{"$ne": true}
			criterias.SearchBy["zatca.reporting_passed"] = bson.M{"$eq": false}
		} else if value == "not_reported" { //ok
			criterias.SearchBy["zatca.reporting_passed"] = bson.M{"$eq": nil}
			criterias.SearchBy["zatca.compliance_passed"] = bson.M{"$eq": nil}
		} else if value == "compliance_passed" {
			criterias.SearchBy["zatca.compliance_passed"] = bson.M{"$eq": true}
		} else if value == "compliance_failed" {
			criterias.SearchBy["zatca.compliance_passed"] = bson.M{"$eq": false} //pl
		}
	}

	keys, ok = r.URL.Query()["search[quotation_id]"]
	if ok && len(keys[0]) >= 1 {
		quotationID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
		}
		criterias.SearchBy["quotation_id"] = quotationID
	}

	keys, ok = r.URL.Query()["search[payment_methods]"]
	if ok && len(keys[0]) >= 1 {
		paymentMethods := strings.Split(keys[0], ",")

		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_methods"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[total_payment_paid]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["total_payment_paid"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["total_payment_paid"] = value
		}

	}

	keys, ok = r.URL.Query()["search[balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["balance_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["balance_amount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[payments_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["payments_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["payments_count"] = value
		}
	}

	timeZoneOffset := 0.0
	keys, ok = r.URL.Query()["search[timezone_offset]"]
	if ok && len(keys[0]) >= 1 {
		if s, err := strconv.ParseFloat(keys[0], 64); err == nil {
			timeZoneOffset = s
		}
	}

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))

		//log.Printf("Start Date:%v", startDate)
		//log.Printf("End Date:%v", endDate)
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	var startDate time.Time
	var endDate time.Time

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}
		log.Printf("Start Date:%v", startDate)
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if timeZoneOffset != 0 {
			endDate = ConvertTimeZoneToUTC(timeZoneOffset, endDate)
		}
		endDate = endDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		log.Printf("End Date:%v", endDate)
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
			return quotationsalesreturns, criterias, err
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
			return quotationsalesreturns, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
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

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[quotation_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["quotation_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[cash_discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["cash_discount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["cash_discount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["discount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["discount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[net_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_profit"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_profit"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["loss"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["loss"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[net_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return quotationsalesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_loss"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_loss"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return quotationsalesreturns, criterias, err
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
				return quotationsalesreturns, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		statusList := strings.Split(keys[0], ",")
		if len(statusList) > 0 {
			criterias.SearchBy["status"] = bson.M{"$in": statusList}
		}
	}

	keys, ok = r.URL.Query()["search[received_by]"]
	if ok && len(keys[0]) >= 1 {
		receivedByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
		}
		criterias.SearchBy["received_by"] = receivedByID
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return quotationsalesreturns, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")

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
	//deletedByUserSelectFields := map[string]interface{}{}

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

		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
			}*/

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
		return quotationsalesreturns, criterias, errors.New("Error fetching quotationsalesreturns:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return quotationsalesreturns, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotationsalesreturn := QuotationSalesReturn{}
		err = cur.Decode(&quotationsalesreturn)
		if err != nil {
			return quotationsalesreturns, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			quotationsalesreturn.Store, _ = FindStoreByID(quotationsalesreturn.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			quotationsalesreturn.Customer, _ = store.FindCustomerByID(quotationsalesreturn.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			quotationsalesreturn.CreatedByUser, _ = FindUserByID(quotationsalesreturn.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			quotationsalesreturn.UpdatedByUser, _ = FindUserByID(quotationsalesreturn.UpdatedBy, updatedByUserSelectFields)
		}
		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				quotationsalesreturn.DeletedByUser, _ = FindUserByID(quotationsalesreturn.DeletedBy, deletedByUserSelectFields)
			}
		*/
		quotationsalesreturns = append(quotationsalesreturns, quotationsalesreturn)
	} //end for loop

	return quotationsalesreturns, criterias, nil
}

func (model *QuotationSalesReturn) FindLastReportedQuotationSalesReturn(selectFields map[string]interface{}) (lastReportedQuotationSalesReturn *QuotationSalesReturn, err error) {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	findOneOptions.SetSort(map[string]interface{}{"zatca.reporting_passed_at": -1})

	err = collection.FindOne(ctx,
		bson.M{
			"zatca.reporting_passed": true,
			"store_id":               model.StoreID,
		}, findOneOptions).
		Decode(&lastReportedQuotationSalesReturn)
	if err != nil {
		return nil, err
	}

	return lastReportedQuotationSalesReturn, err
}

func (quotationsalesreturn *QuotationSalesReturn) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldQuotationSalesReturn *QuotationSalesReturn) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(quotationsalesreturn.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "Store is required"
	}

	if !govalidator.IsNull(strings.TrimSpace(quotationsalesreturn.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(quotationsalesreturn.Phone)) {
		errs["phone"] = "Invalid phone no."
		return
	}

	if !govalidator.IsNull(strings.TrimSpace(quotationsalesreturn.VatNo)) && !IsValidDigitNumber(strings.TrimSpace(quotationsalesreturn.VatNo), "15") {
		errs["vat_no"] = "VAT No. should be 15 digits"
		return
	} else if !govalidator.IsNull(strings.TrimSpace(quotationsalesreturn.VatNo)) && !IsNumberStartAndEndWith(strings.TrimSpace(quotationsalesreturn.VatNo), "3") {
		errs["vat_no"] = "VAT No. should start and end with 3"
		return
	}

	quotation, err := store.FindQuotationByID(quotationsalesreturn.QuotationID, bson.M{})
	if err != nil {
		errs["quotation_id"] = "Quotation is invalid"
	}

	customer, err := store.FindCustomerByID(quotationsalesreturn.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "invalid customer"
	}

	if quotationsalesreturn.Discount < 0 {
		errs["discount"] = "Cash discount should not be < 0"
	}

	if govalidator.IsNull(quotationsalesreturn.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, quotationsalesreturn.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		quotationsalesreturn.Date = &date
	}

	totalPayment := float64(0.00)
	for _, payment := range quotationsalesreturn.PaymentsInput {
		if payment.Amount > 0 {
			totalPayment += payment.Amount
		}
	}

	for index, payment := range quotationsalesreturn.PaymentsInput {
		if quotation.PaymentStatus == "not_paid" {
			break
		}

		if govalidator.IsNull(payment.DateStr) {
			errs["payment_date_"+strconv.Itoa(index)] = "Payment date is required"
		} else {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			date, err := time.Parse(shortForm, payment.DateStr)
			if err != nil {
				errs["payment_date_"+strconv.Itoa(index)] = "Invalid date format"
			}

			quotationsalesreturn.PaymentsInput[index].Date = &date
			payment.Date = &date

			if quotationsalesreturn.Date != nil && IsAfter(quotationsalesreturn.Date, quotationsalesreturn.PaymentsInput[index].Date) {
				errs["payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to quotationsales return date time"
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

	if quotationsalesreturn.QuotationID == nil || quotationsalesreturn.QuotationID.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		errs["quotation_id"] = "Quotation ID is required"
		return errs
	}

	maxDiscountAllowed := 0.00
	if scenario == "update" {
		maxDiscountAllowed = quotation.DiscountWithVAT - (quotation.ReturnDiscountWithVAT - oldQuotationSalesReturn.DiscountWithVAT)
	} else {
		maxDiscountAllowed = quotation.DiscountWithVAT - quotation.ReturnDiscountWithVAT
	}

	if quotationsalesreturn.DiscountWithVAT > maxDiscountAllowed {
		errs["discount_with_vat"] = "Discount shoul not be greater than " + fmt.Sprintf("%.2f", (maxDiscountAllowed))
	}

	if quotationsalesreturn.NetTotal > 0 && quotationsalesreturn.CashDiscount >= quotationsalesreturn.NetTotal {
		errs["cash_discount"] = "Cash discount should not be >= " + fmt.Sprintf("%.02f", quotationsalesreturn.NetTotal)
	} else if quotationsalesreturn.CashDiscount < 0 {
		errs["cash_discount"] = "Cash discount should not < 0 "
	}

	/*
		maxCashDiscountAllowed := 0.00
		if scenario == "update" {
			maxCashDiscountAllowed = quotation.CashDiscount - (quotation.ReturnCashDiscount - oldQuotationSalesReturn.CashDiscount)
		} else {
			maxCashDiscountAllowed = quotation.CashDiscount - quotation.ReturnCashDiscount
		}

		if quotationsalesreturn.NetTotal > 0 && quotationsalesreturn.CashDiscount > maxCashDiscountAllowed {
			errs["cash_discount"] = "Cash discount shouldn't greater than " + fmt.Sprintf("%.2f", (maxCashDiscountAllowed))
		}
	*/

	quotationsalesreturn.QuotationCode = quotation.Code

	/*
		if govalidator.IsNull(quotationsalesreturn.PaymentStatus) {
			errs["payment_status"] = "Payment status is required"
		}
	*/

	/*
		if !govalidator.IsNull(quotationsalesreturn.SignatureDateStr) {
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, quotationsalesreturn.SignatureDateStr)
			if err != nil {
				errs["signature_date_str"] = "Invalid date format"
			}
			quotationsalesreturn.SignatureDate = &date
		}*/

	if scenario == "update" {
		if quotationsalesreturn.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsQuotationSalesReturnExists(&quotationsalesreturn.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid QuotationSalesReturn:" + quotationsalesreturn.ID.Hex()
		}

	}

	if quotationsalesreturn.StoreID == nil || quotationsalesreturn.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(quotationsalesreturn.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + quotationsalesreturn.StoreID.Hex()
			return errs
		}
	}

	if len(quotationsalesreturn.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for quotationsalesreturn"
	}

	for index, quotationsalesReturnProduct := range quotationsalesreturn.Products {
		if !quotationsalesReturnProduct.Selected {
			continue
		}

		if quotationsalesReturnProduct.ProductID.IsZero() {
			errs["product_id"] = "Product is required for QuotationSales Return"
		} else {
			exists, err := store.IsProductExists(&quotationsalesReturnProduct.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(index)] = "Invalid product_id:" + quotationsalesReturnProduct.ProductID.Hex() + " in products"
			}
		}

		if quotationsalesReturnProduct.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(index)] = "Quantity is required"
		}

		if govalidator.IsNull(strings.TrimSpace(quotationsalesReturnProduct.Name)) {
			errs["name_"+strconv.Itoa(index)] = "Name is required"
		} else if len(quotationsalesReturnProduct.Name) < 3 {
			errs["name_"+strconv.Itoa(index)] = "Name requires min. 3 chars"
		}

		if quotationsalesReturnProduct.UnitDiscount > quotationsalesReturnProduct.UnitPrice && quotationsalesReturnProduct.UnitPrice > 0 {
			errs["unit_discount_"+strconv.Itoa(index)] = "Unit discount should not be greater than unit price"
		}

		for _, quotationProduct := range quotation.Products {
			if quotationProduct.ProductID == quotationsalesReturnProduct.ProductID {
				//soldQty := RoundFloat((quotationProduct.Quantity - quotationProduct.QuantityReturned), 2)
				maxAllowedQuantity := 0.00
				if scenario == "update" {
					maxAllowedQuantity = quotationProduct.Quantity - (quotationProduct.QuantityReturned - oldQuotationSalesReturn.Products[index].Quantity)
				} else {
					maxAllowedQuantity = quotationProduct.Quantity - quotationProduct.QuantityReturned
				}

				if quotationsalesReturnProduct.Quantity > maxAllowedQuantity {
					errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", maxAllowedQuantity) + " " + quotationProduct.Unit
				}
				/*
					soldQty := RoundFloat((quotationProduct.Quantity - quotationProduct.QuantityReturned), 2)
					if soldQty == 0 {
						errs["quantity_"+strconv.Itoa(index)] = "Already returned all sold quantities"
					} else if quotationsalesReturnProduct.Quantity > float64(soldQty) {
						errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", soldQty) + " " + quotationProduct.Unit
					}
				*/
			}
		}

		/*
			stock, err := GetProductStockInStore(&product.ProductID, quotationsalesreturn.StoreID, product.Quantity)
			if err != nil {
				errs["quantity"] = err.Error()
				return errs
			}

			if stock < product.Quantity {
				productObject, err := FindProductByID(&product.ProductID, bson.M{})
				if err != nil {
					errs["product"] = err.Error()
					return errs
				}

				storeObject, err := FindStoreByID(quotationsalesreturn.StoreID, nil)
				if err != nil {
					errs["store"] = err.Error()
					return errs
				}

				errs["quantity_"+strconv.Itoa(index)] = "Product: " + productObject.Name + " stock is only " + strconv.Itoa(stock) + " in Store: " + storeObject.Name
			}
		*/
	}

	if quotationsalesreturn.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if totalPayment > (quotationsalesreturn.NetTotal - quotationsalesreturn.CashDiscount) {
		errs["total_payment"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", (quotationsalesreturn.NetTotal-quotationsalesreturn.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if totalPayment > quotation.NetTotal {
		errs["total_payment"] = "Total payment amount should not exceed Original QuotationSales Net Total: " + fmt.Sprintf("%.02f", (quotation.NetTotal))
		return
	}

	if quotationsalesreturn.NetTotal > quotation.NetTotal {
		errs["net_total"] = "Net Total  should not exceed Original QuotationSales Net Total: " + fmt.Sprintf("%.02f", (quotation.NetTotal))
		return
	}

	if scenario == "update" {
		if totalPayment > (quotation.TotalPaymentReceived - (quotation.ReturnAmount - oldQuotationSalesReturn.TotalPaymentPaid)) {
			errs["total_payment"] = "Total payment should not be greater than " + fmt.Sprintf("%.2f", (quotation.TotalPaymentReceived-(quotation.ReturnAmount-oldQuotationSalesReturn.TotalPaymentPaid))) + " (total payment received)"
			return errs
		}
	} else {
		if totalPayment > (quotation.TotalPaymentReceived - quotation.ReturnAmount) {
			errs["total_payment"] = "Total payment should not be greater than " + fmt.Sprintf("%.2f", (quotation.TotalPaymentReceived-quotation.ReturnAmount)) + " (total payment received)"
			return errs
		}
	}

	if customer != nil && customer.CreditLimit > 0 {
		if customer.Account == nil {
			customer.Account = &Account{}
			if quotationsalesreturn.BalanceAmount > 0 {
				customer.Account.Type = "liability"
			} else {
				customer.Account.Type = "asset"
			}
		}

		if scenario != "update" && customer.IsCreditLimitExceeded(quotationsalesreturn.BalanceAmount, true) {
			errs["customer_credit_limit"] = "Exceeding customer credit limit: " + fmt.Sprintf("%.02f", (customer.CreditLimit-customer.CreditBalance))
			return errs
		} else if scenario == "update" && customer.WillEditExceedCreditLimit(oldQuotationSalesReturn.BalanceAmount, quotationsalesreturn.BalanceAmount, true) {
			errs["customer_credit_limit"] = "Exceeding customer credit limit: " + fmt.Sprintf("%.02f", ((customer.CreditLimit+oldQuotationSalesReturn.BalanceAmount)-customer.CreditBalance))
			return errs
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (quotationsalesreturn *QuotationSalesReturn) UpdateReturnedQuantityInQuotationProduct(quotationsalesReturnOld *QuotationSalesReturn) error {
	store, err := FindStoreByID(quotationsalesreturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	quotation, err := store.FindQuotationByID(quotationsalesreturn.QuotationID, bson.M{})
	if err != nil {
		return err
	}

	if quotationsalesReturnOld != nil {
		for _, quotationsalesReturnProduct := range quotationsalesReturnOld.Products {
			if !quotationsalesReturnProduct.Selected {
				continue
			}

			for index2, quotationProduct := range quotation.Products {
				if quotationProduct.ProductID == quotationsalesReturnProduct.ProductID {
					if quotation.Products[index2].QuantityReturned > 0 {
						quotation.Products[index2].QuantityReturned -= quotationsalesReturnProduct.Quantity
					}
				}
			}
		}
	}

	for _, quotationsalesReturnProduct := range quotationsalesreturn.Products {
		if !quotationsalesReturnProduct.Selected {
			continue
		}

		for index2, quotationProduct := range quotation.Products {
			if quotationProduct.ProductID == quotationsalesReturnProduct.ProductID {
				quotation.Products[index2].QuantityReturned += quotationsalesReturnProduct.Quantity
			}
		}
	}

	err = quotation.CalculateQuotationProfit()
	if err != nil {
		return err
	}

	err = quotation.Update()
	if err != nil {
		return err
	}

	return nil
}

/*
func GetProductStockInStore(
	productID *primitive.ObjectID,
	storeID *primitive.ObjectID,
	quotationsalesreturnQuantity int,
) (stock int, err error) {
	product, err := FindProductByID(productID, bson.M{})
	if err != nil {
		return 0, err
	}

	if storeID == nil {
		return 0, err
	}

	for _, productStore := range product.Stores {
		if productStore.StoreID.Hex() == storeID.Hex() {
			return productStore.Stock, nil
		}
	}

	return 0, err
}
*/

/*
func (quotationsalesreturn *QuotationSalesReturn) RemoveStock() (err error) {
	if len(quotationsalesreturn.Products) == 0 {
		return nil
	}

	for _, quotationsalesreturnProduct := range quotationsalesreturn.Products {
		product, err := FindProductByID(&quotationsalesreturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		for k, productStore := range product.Stores {
			if productStore.StoreID.Hex() == quotationsalesreturn.StoreID.Hex() {

				quotationsalesreturn.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stores[k].Stock,
					(product.Stores[k].Stock - quotationsalesreturnProduct.Quantity),
				)

				product.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stores[k].Stock,
					(product.Stores[k].Stock - quotationsalesreturnProduct.Quantity),
				)

				product.Stores[k].Stock -= quotationsalesreturnProduct.Quantity
				quotationsalesreturn.StockAdded = true
				break
			}
		}

		err = product.Update()
		if err != nil {
			return err
		}

	}

	err = quotationsalesreturn.Update()
	if err != nil {
		return err
	}
	return nil
}
*/

func (quotationsalesreturn *QuotationSalesReturn) AddStock() (err error) {
	store, err := FindStoreByID(quotationsalesreturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(quotationsalesreturn.Products) == 0 {
		return nil
	}

	for _, quotationsalesreturnProduct := range quotationsalesreturn.Products {
		if !quotationsalesreturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&quotationsalesreturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if productStoreTemp, ok := product.ProductStores[quotationsalesreturn.StoreID.Hex()]; ok {
			productStoreTemp.Stock += quotationsalesreturnProduct.Quantity
			product.ProductStores[quotationsalesreturn.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[quotationsalesreturn.StoreID.Hex()] = ProductStore{
				StoreID: *quotationsalesreturn.StoreID,
				Stock:   quotationsalesreturnProduct.Quantity,
			}
		}

		/*
			storeExistInProductStore := false
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == quotationsalesreturn.StoreID.Hex() {

					product.Stores[k].Stock += quotationsalesreturnProduct.Quantity
					storeExistInProductStore = true
					break
				}
			}

			if !storeExistInProductStore {
				productStore := ProductStore{
					StoreID: *quotationsalesreturn.StoreID,
					Stock:   quotationsalesreturnProduct.Quantity,
				}
				product.Stores = append(product.Stores, productStore)
			}
		*/

		err = product.Update(nil)
		if err != nil {
			return err
		}
	}

	quotationsalesreturn.StockAdded = false
	err = quotationsalesreturn.Update()
	if err != nil {
		return err
	}

	return nil
}

func (quotationsalesreturn *QuotationSalesReturn) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(quotationsalesreturn.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": quotationsalesreturn.StoreID}, "quotation_sales_return")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (quotationsalesreturn *QuotationSalesReturn) Insert() error {
	collection := db.GetDB("store_" + quotationsalesreturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quotationsalesreturn.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &quotationsalesreturn)
	if err != nil {
		return err
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) CalculateQuotationSalesReturnProfit() error {
	totalProfit := float64(0.0)
	totalLoss := float64(0.0)
	/*
		quotation, err := FindQuotationByID(quotationsalesReturn.QuotationID, map[string]interface{}{})
		if err != nil {
			return err
		}
	*/

	for i, product := range quotationsalesReturn.Products {
		if !product.Selected {
			continue
		}

		quantity := product.Quantity
		quotationsalesPrice := (quantity * (product.UnitPrice - product.UnitDiscount))
		//purchaseUnitPrice := product.PurchaseUnitPrice
		/*
			for _, quotationProduct := range quotation.Products {
				if quotationProduct.ProductID.Hex() == product.ProductID.Hex() {
					purchaseUnitPrice = quotationProduct.PurchaseUnitPrice
					break
				}
			}*/
		//quotationsalesReturn.Products[i].PurchaseUnitPrice = purchaseUnitPrice
		purchaseUnitPrice := quotationsalesReturn.Products[i].PurchaseUnitPrice

		/*
			if purchaseUnitPrice == 0 ||
				quotationsalesReturn.Products[i].Loss > 0 ||
				quotationsalesReturn.Products[i].Profit <= 0 {
				product, err := FindProductByID(&product.ProductID, map[string]interface{}{})
				if err != nil {
					return err
				}
				for _, productStore := range product.ProductStores {
					if productStore.StoreID == *quotationsalesReturn.StoreID {
						purchaseUnitPrice = productStore.PurchaseUnitPrice
						quotationsalesReturn.Products[i].PurchaseUnitPrice = purchaseUnitPrice
						break
					}
				}
			}*/

		profit := 0.0
		if purchaseUnitPrice > 0 {
			profit = quotationsalesPrice - (quantity * purchaseUnitPrice)
		}

		profit = RoundFloat(profit, 2)

		if profit >= 0 {
			quotationsalesReturn.Products[i].Profit = profit
			quotationsalesReturn.Products[i].Loss = 0.0
			totalProfit += quotationsalesReturn.Products[i].Profit
		} else {
			quotationsalesReturn.Products[i].Profit = 0
			quotationsalesReturn.Products[i].Loss = (profit * -1)
			totalLoss += quotationsalesReturn.Products[i].Loss
		}

	}
	quotationsalesReturn.Profit = totalProfit
	quotationsalesReturn.NetProfit = (totalProfit - quotationsalesReturn.CashDiscount) - quotationsalesReturn.Discount
	quotationsalesReturn.Loss = totalLoss
	quotationsalesReturn.NetLoss = totalLoss
	if quotationsalesReturn.NetProfit < 0 {
		quotationsalesReturn.NetLoss += (quotationsalesReturn.NetProfit * -1)
		quotationsalesReturn.NetProfit = 0.00
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) MakeRedisCode() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := quotationsalesReturn.StoreID.Hex() + "_quotation_return_invoice_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from quotation.CreatedAt or fallback to quotation.Date or now ===
	baseTime := quotationsalesReturn.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("quotation_sales_return")
		if err != nil {
			return err
		}
		startFrom := store.QuotationSalesReturnSerialNumber.StartFromCount
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

	// === 5. Determine which counter to use for quotation.Code ===
	useMonthly := strings.Contains(store.QuotationSalesReturnSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := quotationsalesReturn.StoreID.Hex() + "_quotation_return_invoice_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.QuotationSalesReturnSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "quotation_sales_return")
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
	paddingCount := store.QuotationSalesReturnSerialNumber.PaddingCount
	if store.QuotationSalesReturnSerialNumber.Prefix != "" {
		quotationsalesReturn.Code = fmt.Sprintf("%s-%0*d", store.QuotationSalesReturnSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		quotationsalesReturn.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(quotationsalesReturn.Code, "DATE") {
		quotationDate := baseTime.Format("20060102") // YYYYMMDD
		quotationsalesReturn.Code = strings.ReplaceAll(quotationsalesReturn.Code, "DATE", quotationDate)
	}

	// === 8. Set InvoiceCountValue (based on global counter) ===
	quotationsalesReturn.InvoiceCountValue = globalIncr - (store.QuotationSalesReturnSerialNumber.StartFromCount - 1)

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) UnMakeRedisCode() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := quotationsalesReturn.StoreID.Hex() + "_return_invoice_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := quotationsalesReturn.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.QuotationSalesReturnSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := quotationsalesReturn.StoreID.Hex() + "_return_invoice_counter_" + monthKey

		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) MakeCode() error {
	return quotationsalesReturn.MakeRedisCode()
}

func (quotationsalesReturn *QuotationSalesReturn) UnMakeCode() error {
	return quotationsalesReturn.UnMakeRedisCode()
}

func (store *Store) FindLastQuotationSalesReturnByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotationsalesReturn *QuotationSalesReturn, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"created_at": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&quotationsalesReturn)
	if err != nil {
		return nil, err
	}

	return quotationsalesReturn, err
}

func (quotationsalesReturn *QuotationSalesReturn) UpdateQuotationReturnDiscount(quotationsalesReturnOld *QuotationSalesReturn) error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	quotation, err := store.FindQuotationByID(quotationsalesReturn.QuotationID, bson.M{})
	if err != nil {
		return err
	}

	if quotationsalesReturnOld != nil {
		quotation.ReturnDiscount -= quotationsalesReturnOld.Discount
	}

	quotation.ReturnDiscount += quotationsalesReturn.Discount
	quotation.ReturnDiscountWithVAT += quotationsalesReturn.DiscountWithVAT
	return quotation.Update()
}

func (quotationsalesReturn *QuotationSalesReturn) UpdateQuotationReturnCount() (count int64, err error) {
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return 0, err
	}

	returnCount, err := collection.CountDocuments(ctx, bson.M{
		"quotation_id": quotationsalesReturn.QuotationID,
		"deleted":      bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}

	quotation, err := store.FindQuotationByID(quotationsalesReturn.QuotationID, bson.M{})
	if err != nil {
		return 0, err
	}

	quotation.ReturnCount = returnCount
	err = quotation.Update()
	if err != nil {
		return 0, err
	}

	return returnCount, nil
}

/*
func (quotationsalesReturn *QuotationSalesReturn) UpdateQuotationReturnAmount() (count int64, err error) {
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return 0, err
	}

	returnCount, err := collection.CountDocuments(ctx, bson.M{
		"quotation_id": quotationsalesReturn.QuotationID,
		"deleted":  bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}

	quotation, err := store.FindQuotationByID(quotationsalesReturn.QuotationID, bson.M{})
	if err != nil {
		return 0, err
	}

	quotation.ReturnAmount = returnCount
	err = quotation.Update()
	if err != nil {
		return 0, err
	}

	return returnCount, nil
}
*/

func (quotationsalesReturn *QuotationSalesReturn) UpdateQuotationReturnCashDiscount(quotationsalesReturnOld *QuotationSalesReturn) error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	quotation, err := store.FindQuotationByID(quotationsalesReturn.QuotationID, bson.M{})
	if err != nil {
		return err
	}

	if quotationsalesReturnOld != nil {
		quotation.ReturnCashDiscount -= quotationsalesReturnOld.CashDiscount
	}

	quotation.ReturnCashDiscount += quotationsalesReturn.CashDiscount
	return quotation.Update()
}

func (quotationsalesreturn *QuotationSalesReturn) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + quotationsalesreturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if quotationsalesreturn.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotationsalesreturn.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotationsalesreturn.Code,
			"_id":  bson.M{"$ne": quotationsalesreturn.ID},
		})
	}

	return (count > 0), err
}

func GenerateQuotationSalesReturnCode(n int) string {
	//letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	letterRunes := []rune("0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (quotationsalesreturn *QuotationSalesReturn) UpdateQuotationSalesReturnStatus(status string) (*QuotationSalesReturn, error) {
	collection := db.GetDB("store_" + quotationsalesreturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationsalesreturn.ID},
		bson.M{"$set": bson.M{"status": status}},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return quotationsalesreturn, nil
	}
	return nil, nil
}

func (quotationsalesreturn *QuotationSalesReturn) Update() error {
	collection := db.GetDB("store_" + quotationsalesreturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationsalesreturn.ID},
		bson.M{"$set": quotationsalesreturn},
		updateOptions,
	)
	if err != nil {
		return err
	}

	if updateResult.MatchedCount > 0 {
		return nil
	}
	return nil
}

func (quotationsalesreturn *QuotationSalesReturn) DeleteQuotationSalesReturn(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + quotationsalesreturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = quotationsalesreturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	/*
		userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
		if err != nil {
			return err
		}
			quotationsalesreturn.Deleted = true
			quotationsalesreturn.DeletedBy = &userID
			now := time.Now()
			quotationsalesreturn.DeletedAt = &now
	*/

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotationsalesreturn.ID},
		bson.M{"$set": quotationsalesreturn},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindQuotationSalesReturnByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotationsalesreturn *QuotationSalesReturn, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")
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
		Decode(&quotationsalesreturn)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "store")
		quotationsalesreturn.Store, _ = FindStoreByID(quotationsalesreturn.StoreID, fields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "customer")
		quotationsalesreturn.Customer, _ = store.FindCustomerByID(quotationsalesreturn.CustomerID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		quotationsalesreturn.CreatedByUser, _ = FindUserByID(quotationsalesreturn.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		quotationsalesreturn.UpdatedByUser, _ = FindUserByID(quotationsalesreturn.UpdatedBy, fields)
	}

	/*
		if _, ok := selectFields["deleted_by_user.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
			quotationsalesreturn.DeletedByUser, _ = FindUserByID(quotationsalesreturn.DeletedBy, fields)
		}
	*/

	return quotationsalesreturn, err
}

func (store *Store) IsQuotationSalesReturnExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (quotationsalesreturn *QuotationSalesReturn) HardDelete() (err error) {
	collection := db.GetDB("store_" + quotationsalesreturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.M{"_id": quotationsalesreturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func ProcessQuotationSalesReturns() error {
	log.Print("Processing quotationsales returns")

	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		/*
			if store.Code != "GUOJ" {
				break
			}*/

		totalCount, err := store.GetTotalCount(bson.M{
			"store_id": store.ID,
			//"zatca.compliance_passed": bson.M{"$eq": false},
			//"zatca.reporting_passed":              bson.M{"$ne": true},
			//"zatca.compliance_check_failed_count": nil,
		}, "quotation_sales_return")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("quotation_sales_return")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)
		//findOptions.SetSort(GetSortByFields("created_at"))
		findOptions.SetSort(bson.M{"date": 1})

		cur, err := collection.Find(ctx, bson.M{
			"store_id": store.ID,
			//"zatca.compliance_passed": bson.M{"$eq": false},
			//"zatca.reporting_passed":              bson.M{"$ne": true},
			//"zatca.compliance_check_failed_count": nil,
		}, findOptions)
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
			quotationsalesReturn := QuotationSalesReturn{}
			err = cur.Decode(&quotationsalesReturn)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if quotationsalesReturn.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			/*
				quotationsalesReturn.UpdateForeignLabelFields()
				quotationsalesReturn.ClearProductsHistory()
				quotationsalesReturn.ClearProductsQuotationSalesReturnHistory()
				quotationsalesReturn.CreateProductsHistory()
				quotationsalesReturn.CreateProductsQuotationSalesReturnHistory()
			*/

			/*
				if store.Code == "MBDI" || store.Code == "LGK" {
					quotationsalesReturn.ClearProductsQuotationSalesReturnHistory()
					quotationsalesReturn.CreateProductsQuotationSalesReturnHistory()
				}*/

			/*
				if store.Code == "LGK-SIMULATION" {
					if quotationsalesReturn.Code == "0" {
						quotationsalesReturn.Code = "QTN-SR-20250614-001"
					} else if quotationsalesReturn.Code == "QTN-SR-20250614-001" {
						quotationsalesReturn.Code = "QTN-SR-20250614-002"
					}

					quotationsalesReturn.Update()
				}*/

			quotationsalesReturn.UndoAccounting()
			quotationsalesReturn.DoAccounting()

			if quotationsalesReturn.CustomerID != nil && !quotationsalesReturn.CustomerID.IsZero() {
				customer, _ := store.FindCustomerByID(quotationsalesReturn.CustomerID, bson.M{})
				if customer != nil {
					customer.SetCreditBalance()
				}
			}

			/*
				quotation, _ := store.FindQuotationByID(quotationsalesReturn.QuotationID, bson.M{})
				quotation.ReturnAmount, quotation.ReturnCount, _ = store.GetReturnedAmountByQuotationID(*quotationsalesReturn.QuotationID)
				quotation.Update()
			*/

			/*
				log.Print("QuotationSales Return ID: " + quotationsalesReturn.Code)
				err = quotationsalesReturn.ReportToZatca()
				if err != nil {
					log.Print("Failed 1st time, trying 2nd time")

					if GetDecimalPoints(quotationsalesReturn.ShippingOrHandlingFees) > 2 {
						log.Print("Trimming shipping cost to 2 decimals")
						quotationsalesReturn.ShippingOrHandlingFees = RoundTo2Decimals(quotationsalesReturn.ShippingOrHandlingFees)
					}

					if GetDecimalPoints(quotationsalesReturn.Discount) > 2 {
						log.Print("Trimming discount to 2 decimals")
						quotationsalesReturn.Discount = RoundTo2Decimals(quotationsalesReturn.Discount)
					}

					quotationsalesReturn.FindNetTotal()
					quotationsalesReturn.Update()

					err = quotationsalesReturn.ReportToZatca()
					if err != nil {
						log.Print("Failed  2nd time. ")
						customer, _ := store.FindCustomerByID(quotationsalesReturn.CustomerID, bson.M{})
						if customer != nil {
							log.Print("Trying 3rd time ")
							if govalidator.IsNull(customer.NationalAddress.BuildingNo) {
								customer.NationalAddress.BuildingNo = "1234"
								customer.NationalAddress.StreetName = "test"
								customer.NationalAddress.DistrictName = "test"
								customer.NationalAddress.CityName = "test"
								customer.NationalAddress.ZipCode = "12345"

								log.Print("Setting national address for customer")
							}

							if !IsValidDigitNumber(customer.VATNo, "15") || !IsNumberStartAndEndWith(customer.VATNo, "3") {

								customer.VATNo = GenerateRandom15DigitNumber()
								log.Print("Replaced invalid vat no.")
							}

							customer.Update()
							err = quotationsalesReturn.ReportToZatca()
							if err != nil {
								log.Print("Failed  3rd time. dropping")
							} else {
								log.Print("REPORTED 3rd time")
							}

						} else {
							log.Print("dropping (coz: no customer found)")
						}
					} else {
						log.Print("REPORTED 2nd time")
					}
				} else {
					log.Print("REPORTED")
				}
			*/
			/*
				err = quotationsalesReturn.Update()
				if err != nil {
					return errors.New("Error updating: " + err.Error())
				}
			*/

			bar.Add(1)
		}
	}
	log.Print("QuotationSales Returns DONE!")
	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) SetPaymentStatus() (payments []QuotationSalesReturnPayment, err error) {
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"quotation_sales_return_id": quotationsalesReturn.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return payments, errors.New("Error fetching quotationsales return payment history" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	totalPaymentPaid := float64(0.0)
	paymentMethods := []string{}

	//	log.Print("Starting for")
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		//log.Print("Loop")
		err := cur.Err()
		if err != nil {
			return payments, errors.New("Cursor error:" + err.Error())
		}
		model := QuotationSalesReturnPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return payments, errors.New("Cursor decode error:" + err.Error())
		}

		payments = append(payments, model)

		totalPaymentPaid += model.Amount

		if !slices.Contains(paymentMethods, model.Method) {
			paymentMethods = append(paymentMethods, model.Method)
		}
	} //end for loop

	quotationsalesReturn.TotalPaymentPaid = ToFixed(totalPaymentPaid, 2)
	//quotationsalesReturn.BalanceAmount = ToFixed(quotationsalesReturn.NetTotal-totalPaymentPaid, 2)
	quotationsalesReturn.BalanceAmount = ToFixed((quotationsalesReturn.NetTotal-quotationsalesReturn.CashDiscount)-totalPaymentPaid, 2)
	quotationsalesReturn.PaymentMethods = paymentMethods
	quotationsalesReturn.Payments = payments
	quotationsalesReturn.PaymentsCount = int64(len(payments))

	if ToFixed((quotationsalesReturn.NetTotal-quotationsalesReturn.CashDiscount), 2) <= ToFixed(totalPaymentPaid, 2) {
		quotationsalesReturn.PaymentStatus = "paid"
	} else if ToFixed(totalPaymentPaid, 2) > 0 {
		quotationsalesReturn.PaymentStatus = "paid_partially"
	} else if ToFixed(totalPaymentPaid, 2) <= 0 {
		quotationsalesReturn.PaymentStatus = "not_paid"
	}

	return payments, err
}

func (quotationsalesReturn *QuotationSalesReturn) RemoveStock() (err error) {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(quotationsalesReturn.Products) == 0 {
		return nil
	}

	for _, quotationsalesReturnProduct := range quotationsalesReturn.Products {
		if !quotationsalesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&quotationsalesReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.ProductStores) == 0 {
			store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
			if err != nil {
				return err
			}

			product.ProductStores = map[string]ProductStore{}

			product.ProductStores[quotationsalesReturn.StoreID.Hex()] = ProductStore{
				StoreID:           *quotationsalesReturn.StoreID,
				StoreName:         quotationsalesReturn.StoreName,
				StoreNameInArabic: store.NameInArabic,
				Stock:             float64(0),
			}
		}

		if productStoreTemp, ok := product.ProductStores[quotationsalesReturn.StoreID.Hex()]; ok {
			productStoreTemp.Stock -= (quotationsalesReturnProduct.Quantity)
			product.ProductStores[quotationsalesReturn.StoreID.Hex()] = productStoreTemp
		}
		/*
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == quotationsalesReturn.StoreID.Hex() {
					product.Stores[k].Stock -= (quotationsalesReturnProduct.Quantity)
					break
				}
			}
		*/

		err = product.Update(nil)
		if err != nil {
			return err
		}

	}

	err = quotationsalesReturn.Update()
	if err != nil {
		return err
	}
	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) ClearPayments() error {
	//log.Printf("Clearing QuotationSales history of quotation id:%s", quotation.Code)
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"quotation_sales_return_id": quotationsalesReturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) GetPaymentsCount() (count int64, err error) {
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"quotation_sales_return_id": quotationsalesReturn.ID,
		"deleted":                   bson.M{"$ne": true},
	})
}

type ProductQuotationSalesReturnStats struct {
	QuotationSalesReturnCount    int64   `json:"quotation_sales_return_count" bson:"quotation_sales_return_count"`
	QuotationSalesReturnQuantity float64 `json:"quotation_sales_return_quantity" bson:"quotation_sales_return_quantity"`
	QuotationSalesReturn         float64 `json:"quotation_sales_return" bson:"quotation_sales_return"`
	QuotationSalesReturnProfit   float64 `json:"quotation_sales_return_profit" bson:"quotation_sales_return_profit"`
	QuotationSalesReturnLoss     float64 `json:"quotation_sales_return_loss" bson:"quotation_sales_return_loss"`
}

func (product *Product) SetProductQuotationSalesReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_quotation_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationSalesReturnStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                             nil,
				"quotation_sales_return_count":    bson.M{"$sum": 1},
				"quotation_sales_return_quantity": bson.M{"$sum": "$quantity"},
				"quotation_sales_return":          bson.M{"$sum": "$net_price"},
				"quotation_sales_return_profit":   bson.M{"$sum": "$profit"},
				"quotation_sales_return_loss":     bson.M{"$sum": "$loss"},
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

		stats.QuotationSalesReturn = RoundFloat(stats.QuotationSalesReturn, 2)
		stats.QuotationSalesReturnProfit = RoundFloat(stats.QuotationSalesReturnProfit, 2)
		stats.QuotationSalesReturnLoss = RoundFloat(stats.QuotationSalesReturnLoss, 2)
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.QuotationSalesReturnCount = stats.QuotationSalesReturnCount
		productStoreTemp.QuotationSalesReturnQuantity = stats.QuotationSalesReturnQuantity
		productStoreTemp.QuotationSalesReturn = stats.QuotationSalesReturn
		productStoreTemp.QuotationSalesReturnProfit = stats.QuotationSalesReturnProfit
		productStoreTemp.QuotationSalesReturnLoss = stats.QuotationSalesReturnLoss
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	/*
		for storeIndex, store := range product.ProductStores {
			if store.StoreID.Hex() == storeID.Hex() {
				product.Stores[storeIndex].QuotationSalesReturnCount = stats.QuotationSalesReturnCount
				product.Stores[storeIndex].QuotationSalesReturnQuantity = stats.QuotationSalesReturnQuantity
				product.Stores[storeIndex].QuotationSalesReturn = stats.QuotationSalesReturn
				product.Stores[storeIndex].QuotationSalesReturnProfit = stats.QuotationSalesReturnProfit
				product.Stores[storeIndex].QuotationSalesReturnLoss = stats.QuotationSalesReturnLoss
				err = product.Update()
				if err != nil {
					return err
				}
				break
			}
		}
	*/

	return nil
}

func (product *Product) SetProductQuotationSalesReturnQuantityByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_quotation_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationSalesReturnStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                             nil,
				"quotation_sales_return_quantity": bson.M{"$sum": "$quantity"},
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
		productStoreTemp.QuotationSalesReturnQuantity = stats.QuotationSalesReturnQuantity
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (product *Product) GetProductQuotationSalesReturnQuantitySince(since *time.Time) (float64, error) {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_quotation_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductQuotationSalesReturnStats

	filter := map[string]interface{}{
		"store_id":   product.StoreID,
		"product_id": product.ID,
	}

	if since != nil {
		filter["date"] = bson.M{"$gte": since}
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                             nil,
				"quotation_sales_return_quantity": bson.M{"$sum": "$quantity"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return 0, err
		}
	}

	return stats.QuotationSalesReturnQuantity, nil
}

func (quotationsalesReturn *QuotationSalesReturn) SetProductsQuotationSalesReturnStats() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, quotationsalesReturnProduct := range quotationsalesReturn.Products {
		if !quotationsalesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&quotationsalesReturnProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductQuotationSalesReturnStatsByStoreID(*quotationsalesReturn.StoreID)
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

				err = setProductObj.SetProductQuotationSalesReturnStatsByStoreID(store.ID)
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
type CustomerQuotationSalesReturnStats struct {
	QuotationSalesReturnCount              int64   `json:"quotation_sales_return_count" bson:"quotation_sales_return_count"`
	QuotationSalesReturnAmount             float64 `json:"quotation_sales_return_amount" bson:"quotation_sales_return_amount"`
	QuotationSalesReturnPaidAmount         float64 `json:"quotation_sales_return_paid_amount" bson:"quotation_sales_return_paid_amount"`
	QuotationSalesReturnBalanceAmount      float64 `json:"quotation_sales_return_balance_amount" bson:"quotation_sales_return_balance_amount"`
	QuotationSalesReturnProfit             float64 `json:"quotation_sales_return_profit" bson:"quotation_sales_return_profit"`
	QuotationSalesReturnLoss               float64 `json:"quotation_sales_return_loss" bson:"quotation_sales_return_loss"`
	QuotationSalesReturnPaidCount          int64   `json:"quotation_sales_return_paid_count" bson:"quotation_sales_return_paid_count"`
	QuotationSalesReturnNotPaidCount       int64   `json:"quotation_sales_return_not_paid_count" bson:"quotation_sales_return_not_paid_count"`
	QuotationSalesReturnPaidPartiallyCount int64   `json:"quotation_sales_return_paid_partially_count" bson:"quotation_sales_return_paid_partially_count"`
}

func (customer *Customer) SetCustomerQuotationSalesReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerQuotationSalesReturnStats

	filter := map[string]interface{}{
		"store_id":    storeID,
		"customer_id": customer.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                                   nil,
				"quotation_sales_return_count":          bson.M{"$sum": 1},
				"quotation_sales_return_amount":         bson.M{"$sum": "$net_total"},
				"quotation_sales_return_paid_amount":    bson.M{"$sum": "$total_payment_paid"},
				"quotation_sales_return_balance_amount": bson.M{"$sum": "$balance_amount"},
				"quotation_sales_return_profit":         bson.M{"$sum": "$net_profit"},
				"quotation_sales_return_loss":           bson.M{"$sum": "$loss"},
				"quotation_sales_return_paid_count": bson.M{"$sum": bson.M{
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
				"quotation_sales_return_not_paid_count": bson.M{"$sum": bson.M{
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
				"quotation_sales_return_paid_partially_count": bson.M{"$sum": bson.M{
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
		stats.QuotationSalesReturnAmount = RoundFloat(stats.QuotationSalesReturnAmount, 2)
		stats.QuotationSalesReturnPaidAmount = RoundFloat(stats.QuotationSalesReturnPaidAmount, 2)
		stats.QuotationSalesReturnBalanceAmount = RoundFloat(stats.QuotationSalesReturnBalanceAmount, 2)
		stats.QuotationSalesReturnProfit = RoundFloat(stats.QuotationSalesReturnProfit, 2)
		stats.QuotationSalesReturnLoss = RoundFloat(stats.QuotationSalesReturnLoss, 2)
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
		customerStore.QuotationSalesReturnCount = stats.QuotationSalesReturnCount
		customerStore.QuotationSalesReturnAmount = stats.QuotationSalesReturnAmount
		customerStore.QuotationSalesReturnPaidAmount = stats.QuotationSalesReturnPaidAmount
		customerStore.QuotationSalesReturnBalanceAmount = stats.QuotationSalesReturnBalanceAmount
		customerStore.QuotationSalesReturnProfit = stats.QuotationSalesReturnProfit
		customerStore.QuotationSalesReturnLoss = stats.QuotationSalesReturnLoss
		customerStore.QuotationSalesReturnPaidCount = stats.QuotationSalesReturnPaidCount
		customerStore.QuotationSalesReturnNotPaidCount = stats.QuotationSalesReturnNotPaidCount
		customerStore.QuotationSalesReturnPaidPartiallyCount = stats.QuotationSalesReturnPaidPartiallyCount
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:                                storeID,
			StoreName:                              store.Name,
			StoreNameInArabic:                      store.NameInArabic,
			QuotationSalesReturnCount:              stats.QuotationSalesReturnCount,
			QuotationSalesReturnAmount:             stats.QuotationSalesReturnAmount,
			QuotationSalesReturnPaidAmount:         stats.QuotationSalesReturnPaidAmount,
			QuotationSalesReturnBalanceAmount:      stats.QuotationSalesReturnBalanceAmount,
			QuotationSalesReturnProfit:             stats.QuotationSalesReturnProfit,
			QuotationSalesReturnLoss:               stats.QuotationSalesReturnLoss,
			QuotationSalesReturnPaidCount:          stats.QuotationSalesReturnPaidCount,
			QuotationSalesReturnNotPaidCount:       stats.QuotationSalesReturnNotPaidCount,
			QuotationSalesReturnPaidPartiallyCount: stats.QuotationSalesReturnPaidPartiallyCount,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) SetCustomerQuotationSalesReturnStats() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(quotationsalesReturn.CustomerID, map[string]interface{}{})
	if err != nil {
		return err
	}

	err = customer.SetCustomerQuotationSalesReturnStatsByStoreID(*quotationsalesReturn.StoreID)
	if err != nil {
		return err
	}

	return nil
}

// Accounting
// Journal entries
func MakeJournalsForUnpaidQuotationSalesReturn(
	quotationsalesReturn *QuotationSalesReturn,
	customerAccount *Account,
	quotationsalesReturnAccount *Account,
	cashDiscountReceivedAccount *Account,
) []Journal {
	now := time.Now()

	groupID := primitive.NewObjectID()

	journals := []Journal{}

	balanceAmount := RoundFloat((quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          quotationsalesReturn.Date,
		AccountID:     quotationsalesReturnAccount.ID,
		AccountNumber: quotationsalesReturnAccount.Number,
		AccountName:   quotationsalesReturnAccount.Name,
		DebitOrCredit: "debit",
		Debit:         quotationsalesReturn.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if quotationsalesReturn.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          quotationsalesReturn.Date,
			AccountID:     cashDiscountReceivedAccount.ID,
			AccountNumber: cashDiscountReceivedAccount.Number,
			AccountName:   cashDiscountReceivedAccount.Name,
			DebitOrCredit: "credit",
			Credit:        quotationsalesReturn.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          quotationsalesReturn.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "credit",
		Credit:        balanceAmount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

var totalQuotationSalesReturnPaidAmount float64
var extraQuotationSalesReturnAmountPaid float64
var extraQuotationSalesReturnPayments []QuotationSalesReturnPayment

func MakeJournalsForQuotationSalesReturnPaymentsByDatetime(
	quotationsalesReturn *QuotationSalesReturn,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	quotationsalesReturnAccount *Account,
	payments []QuotationSalesReturnPayment,
	cashDiscountReceivedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
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

	//Don't touch
	totalQuotationSalesReturnPaidAmountTemp := totalQuotationSalesReturnPaidAmount
	extraQuotationSalesReturnAmountPaidTemp := extraQuotationSalesReturnAmountPaid

	for _, payment := range payments {
		totalQuotationSalesReturnPaidAmount += payment.Amount
		if totalQuotationSalesReturnPaidAmount > (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount) {
			extraQuotationSalesReturnAmountPaid = RoundFloat((totalQuotationSalesReturnPaidAmount - (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraQuotationSalesReturnAmountPaid > 0 {
			skip := false
			if extraQuotationSalesReturnAmountPaid < payment.Amount {
				amount = RoundFloat((payment.Amount - extraQuotationSalesReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraQuotationSalesReturnAmountPaid = 0
			} else if extraQuotationSalesReturnAmountPaid >= payment.Amount {
				skip = true
				extraQuotationSalesReturnAmountPaid = RoundFloat((extraQuotationSalesReturnAmountPaid - payment.Amount), 2)
			}

			if skip {
				continue
			}

		}
		totalPayment += amount
	} //end for

	totalQuotationSalesReturnPaidAmount = totalQuotationSalesReturnPaidAmountTemp
	extraQuotationSalesReturnAmountPaid = extraQuotationSalesReturnAmountPaidTemp
	//Don't touch
	//Debits
	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(quotationsalesReturn.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          quotationsalesReturn.Date,
			AccountID:     quotationsalesReturnAccount.ID,
			AccountNumber: quotationsalesReturnAccount.Number,
			AccountName:   quotationsalesReturnAccount.Name,
			DebitOrCredit: "debit",
			Debit:         quotationsalesReturn.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(quotationsalesReturn.Date, firstPaymentDate) {
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
			quotationsalesReturn.StoreID,
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
				DebitOrCredit: "debit",
				Debit:         totalPayment,
				GroupID:       groupID,
				CreatedAt:     &now,
				UpdatedAt:     &now,
			})
		}
	}

	//Credits
	totalPayment = float64(0.00)
	for _, payment := range payments {
		totalQuotationSalesReturnPaidAmount += payment.Amount
		if totalQuotationSalesReturnPaidAmount > (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount) {
			extraQuotationSalesReturnAmountPaid = RoundFloat((totalQuotationSalesReturnPaidAmount - (quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraQuotationSalesReturnAmountPaid > 0 {
			skip := false
			if extraQuotationSalesReturnAmountPaid < payment.Amount {
				extraAmount := extraQuotationSalesReturnAmountPaid
				extraQuotationSalesReturnPayments = append(extraQuotationSalesReturnPayments, QuotationSalesReturnPayment{
					Date:   payment.Date,
					Amount: extraAmount,
					Method: payment.Method,
				})
				amount = RoundFloat((payment.Amount - extraQuotationSalesReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraQuotationSalesReturnAmountPaid = 0
			} else if extraQuotationSalesReturnAmountPaid >= payment.Amount {
				extraQuotationSalesReturnPayments = append(extraQuotationSalesReturnPayments, QuotationSalesReturnPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})

				skip = true
				extraQuotationSalesReturnAmountPaid = RoundFloat((extraQuotationSalesReturnAmountPaid - payment.Amount), 2)
			}

			if skip {
				continue
			}

		}

		cashPayingAccount := Account{}
		if payment.ReferenceType == "customer_withdrawal" || payment.ReferenceType == "quotation_sales" {
			continue // Ignoring customer receivable payments as it has already entered into the ledger
		} else if payment.Method == "cash" {
			cashPayingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashPayingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
			/*
				referenceModel := "customer"
				customerAccount, err := store.CreateAccountIfNotExists(
					quotationsalesReturn.StoreID,
					&customer.ID,
					&referenceModel,
					customer.Name,
					&customer.Phone,
					&customer.VATNo,
				)
				if err != nil {
					return nil, err
				}
				cashPayingAccount = *customerAccount
			*/
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashPayingAccount.ID,
			AccountNumber: cashPayingAccount.Number,
			AccountName:   cashPayingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
		totalPayment += amount
	} //end for

	if quotationsalesReturn.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(quotationsalesReturn.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          quotationsalesReturn.Date,
			AccountID:     cashDiscountReceivedAccount.ID,
			AccountNumber: cashDiscountReceivedAccount.Number,
			AccountName:   cashDiscountReceivedAccount.Name,
			DebitOrCredit: "credit",
			Credit:        quotationsalesReturn.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	balanceAmount := RoundFloat(((quotationsalesReturn.NetTotal - quotationsalesReturn.CashDiscount) - totalPayment), 2)

	//Asset or debt increased
	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(quotationsalesReturn.Date, firstPaymentDate) {
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
			quotationsalesReturn.StoreID,
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
			Date:          quotationsalesReturn.Date,
			AccountID:     customerAccount.ID,
			AccountNumber: customerAccount.Number,
			AccountName:   customerAccount.Name,
			DebitOrCredit: "credit",
			Credit:        balanceAmount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	}

	return journals, nil
}

func MakeJournalsForQuotationSalesReturnExtraPayments(
	quotationsalesReturn *QuotationSalesReturn,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []QuotationSalesReturnPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
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
		quotationsalesReturn.StoreID,
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
		DebitOrCredit: "debit",
		Debit:         quotationsalesReturn.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	for _, payment := range extraPayments {
		cashPayingAccount := Account{}
		if payment.ReferenceType == "customer_withdrawal" || payment.ReferenceType == "quotation_sales" {
			continue // Ignoring customer receivable payments as it has already entered into the ledger
		} else if payment.Method == "cash" {
			cashPayingAccount = *cashAccount
		} else if slices.Contains(BANK_PAYMENT_METHODS, payment.Method) {
			cashPayingAccount = *bankAccount
		} else if payment.Method == "customer_account" && customer != nil {
			continue
			/*
				referenceModel := "customer"
				customerAccount, err := store.CreateAccountIfNotExists(
					quotationsalesReturn.StoreID,
					&customer.ID,
					&referenceModel,
					customer.Name,
					&customer.Phone,
					&customer.VATNo,
				)
				if err != nil {
					return nil, err
				}
				cashPayingAccount = *customerAccount
			*/
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashPayingAccount.ID,
			AccountNumber: cashPayingAccount.Number,
			AccountName:   cashPayingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        payment.Amount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	} //end for

	return journals, nil
}

// Regroup quotationsales payments by datetime

func RegroupQuotationSalesReturnPaymentsByDatetime(payments []QuotationSalesReturnPayment) [][]QuotationSalesReturnPayment {
	paymentsByDatetime := map[string][]QuotationSalesReturnPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
		//log.Print(*payment.Amount)
	}

	paymentsByDatetime2 := [][]QuotationSalesReturnPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

//End customer account journals

func (quotationsalesReturn *QuotationSalesReturn) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var customer *Customer

	if quotationsalesReturn.CustomerID != nil && !quotationsalesReturn.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(quotationsalesReturn.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	cashAccount, err := store.CreateAccountIfNotExists(quotationsalesReturn.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(quotationsalesReturn.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	quotationsalesReturnAccount, err := store.CreateAccountIfNotExists(quotationsalesReturn.StoreID, nil, nil, "Sales Return", nil, nil)
	if err != nil {
		return nil, err
	}

	cashDiscountReceivedAccount, err := store.CreateAccountIfNotExists(quotationsalesReturn.StoreID, nil, nil, "Cash discount received", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(quotationsalesReturn.Payments) > 0 {
		firstPaymentDate = quotationsalesReturn.Payments[0].Date
	}

	if len(quotationsalesReturn.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(firstPaymentDate, quotationsalesReturn.Date)) {
		//Case: UnPaid
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
			quotationsalesReturn.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidQuotationSalesReturn(
			quotationsalesReturn,
			customerAccount,
			quotationsalesReturnAccount,
			cashDiscountReceivedAccount,
		)...)
	}

	if len(quotationsalesReturn.Payments) > 0 {
		totalQuotationSalesReturnPaidAmount = float64(0.00)
		extraQuotationSalesReturnAmountPaid = float64(0.00)
		extraQuotationSalesReturnPayments = []QuotationSalesReturnPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupQuotationSalesReturnPaymentsByDatetime(quotationsalesReturn.Payments)
		//fmt.Printf("%+v", paymentsByDatetime)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForQuotationSalesReturnPaymentsByDatetime(
				quotationsalesReturn,
				customer,
				cashAccount,
				bankAccount,
				quotationsalesReturnAccount,
				paymentByDatetime,
				cashDiscountReceivedAccount,
				paymentsByDatetimeNumber,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
			paymentsByDatetimeNumber++
		} //end for

		if quotationsalesReturn.BalanceAmount < 0 && len(extraQuotationSalesReturnPayments) > 0 {
			newJournals, err := MakeJournalsForQuotationSalesReturnExtraPayments(
				quotationsalesReturn,
				customer,
				cashAccount,
				bankAccount,
				extraQuotationSalesReturnPayments,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
		}

		totalQuotationSalesReturnPaidAmount = float64(0.00)
		extraQuotationSalesReturnAmountPaid = float64(0.00)

	}

	ledger = &Ledger{
		StoreID:        quotationsalesReturn.StoreID,
		ReferenceID:    quotationsalesReturn.ID,
		ReferenceModel: "quotation_sales_return",
		ReferenceCode:  quotationsalesReturn.Code,
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

func (quotationSalesReturn *QuotationSalesReturn) GetPayments() (models []QuotationSalesReturnPayment, err error) {
	collection := db.GetDB("store_" + quotationSalesReturn.StoreID.Hex()).Collection("quotation_sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()

	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"quotation_sales_return_id": quotationSalesReturn.ID, "deleted": bson.M{"$ne": true}}, findOptions)
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
		model := QuotationSalesReturnPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		models = append(models, model)

	} //end for loop

	return models, nil
}

func (quotationSalesReturn *QuotationSalesReturn) AdjustPayments() error {
	if len(quotationSalesReturn.Payments) == 0 || quotationSalesReturn.Date == nil {
		return nil
	}

	// 1. Ensure first payment is at least 1 minute after quotation.Date if they are the same
	firstPayment := quotationSalesReturn.Payments[0]
	if firstPayment.Date != nil && firstPayment.Date.Equal(*quotationSalesReturn.Date) {
		newTime := quotationSalesReturn.Date.Add(1 * time.Minute)
		quotationSalesReturn.Payments[0].Date = &newTime
	}

	// 2. For each subsequent payment, ensure strictly increasing by at least 1 minute
	for i := 1; i < len(quotationSalesReturn.Payments); i++ {
		prev := quotationSalesReturn.Payments[i-1].Date
		curr := quotationSalesReturn.Payments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			quotationSalesReturn.Payments[i].Date = &newTime
		}
	}

	quotationSalesReturnPayments, err := quotationSalesReturn.GetPayments()
	if err != nil {
		return err
	}

	for i := 1; i < len(quotationSalesReturnPayments); i++ {
		prev := quotationSalesReturnPayments[i-1].Date
		curr := quotationSalesReturnPayments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			quotationSalesReturnPayments[i].Date = &newTime
			err = quotationSalesReturnPayments[i].Update()
			if err != nil {
				return err
			}
		}
	}

	err = quotationSalesReturn.Update()
	if err != nil {
		return err
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) DoAccounting() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if !store.Settings.QuotationInvoiceAccounting {
		return nil
	}

	err = quotationsalesReturn.AdjustPayments()
	if err != nil {
		return errors.New("error adjusting payments: " + err.Error())
	}

	ledger, err := quotationsalesReturn.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) UndoAccounting() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(quotationsalesReturn.ID, *quotationsalesReturn.StoreID, bson.M{})
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

	err = store.RemoveLedgerByReferenceID(quotationsalesReturn.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(quotationsalesReturn.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) FindPreviousQuotationSalesReturn(selectFields map[string]interface{}) (previousQuotationSalesReturn *QuotationSalesReturn, err error) {
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("quotation_sales_return")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"invoice_count_value": (quotationsalesReturn.InvoiceCountValue - 1),
			"store_id":            quotationsalesReturn.StoreID,
		}, findOneOptions).
		Decode(&previousQuotationSalesReturn)
	if err != nil {
		return nil, err
	}

	return previousQuotationSalesReturn, err
}

func (quotationsalesReturn *QuotationSalesReturn) ValidateZatcaReporting() (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	customer, err := store.FindCustomerByID(quotationsalesReturn.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "Customer is required"
	}

	if customer != nil && customer.VATNo != "" && store.Zatca.Phase == "2" {
		customerErrorMessages := []string{}

		if !IsValidDigitNumber(customer.VATNo, "15") {
			customerErrorMessages = append(customerErrorMessages, "VAT No. should be 15 digits")
		} else if !IsNumberStartAndEndWith(customer.VATNo, "3") {
			customerErrorMessages = append(customerErrorMessages, "VAT No. should start and end with 3")
		}

		if !govalidator.IsNull(customer.RegistrationNumber) && !IsAlphanumeric(customer.RegistrationNumber) {
			customerErrorMessages = append(customerErrorMessages, "Registration Number should be alpha numeric(a-zA-Z|0-9)")
		}

		if govalidator.IsNull(customer.NationalAddress.BuildingNo) {
			customerErrorMessages = append(customerErrorMessages, "Building number is required")
		} else {
			if !IsValidDigitNumber(customer.NationalAddress.BuildingNo, "4") {
				customerErrorMessages = append(customerErrorMessages, "Building number should be 4 digits")
			}
		}

		if govalidator.IsNull(customer.NationalAddress.StreetName) {
			customerErrorMessages = append(customerErrorMessages, "Street name is required")
		}

		if govalidator.IsNull(customer.NationalAddress.DistrictName) {
			customerErrorMessages = append(customerErrorMessages, "District name is required")
		}

		if govalidator.IsNull(customer.NationalAddress.CityName) {
			customerErrorMessages = append(customerErrorMessages, "City name is required")
		}

		if govalidator.IsNull(customer.NationalAddress.ZipCode) {
			customerErrorMessages = append(customerErrorMessages, "Zip code is required")
		} else {
			if !IsValidDigitNumber(customer.NationalAddress.ZipCode, "5") {
				customerErrorMessages = append(customerErrorMessages, "Zip code should be 5 digits")
			}
		}

		if len(customerErrorMessages) > 0 {
			errs["customer_id"] = "Fix the customer errors: " + strings.Join(customerErrorMessages, ",")
		}
	}

	return errs
}
