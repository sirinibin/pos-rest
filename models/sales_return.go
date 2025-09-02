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

type SalesReturnProduct struct {
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

// SalesReturn : SalesReturn structure
type SalesReturn struct {
	ID                primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	OrderID           *primitive.ObjectID  `json:"order_id,omitempty" bson:"order_id,omitempty"`
	OrderCode         string               `bson:"order_code,omitempty" json:"order_code,omitempty"`
	Date              *time.Time           `bson:"date,omitempty" json:"date,omitempty"`
	DateStr           string               `json:"date_str,omitempty" bson:"-"`
	InvoiceCountValue int64                `bson:"invoice_count_value,omitempty" json:"invoice_count_value,omitempty"`
	Code              string               `bson:"code,omitempty" json:"code,omitempty"`
	UUID              string               `bson:"uuid,omitempty" json:"uuid,omitempty"`
	Hash              string               `bson:"hash,omitempty" json:"hash,omitempty"`
	PrevHash          string               `bson:"prev_hash,omitempty" json:"prev_hash,omitempty"`
	CSID              string               `bson:"csid,omitempty" json:"csid,omitempty"`
	StoreID           *primitive.ObjectID  `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID        *primitive.ObjectID  `json:"customer_id" bson:"customer_id"`
	Store             *Store               `json:"store,omitempty"`
	Customer          *Customer            `json:"customer,omitempty"`
	Products          []SalesReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	ReceivedBy        *primitive.ObjectID  `json:"received_by,omitempty" bson:"received_by,omitempty"`
	ReceivedByUser    *User                `json:"received_by_user,omitempty"`
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
	Status                 string   `bson:"status,omitempty" json:"status,omitempty"`
	StockAdded             bool     `bson:"stock_added,omitempty" json:"stock_added,omitempty"`
	TotalQuantity          float64  `bson:"total_quantity" json:"total_quantity"`
	VatPrice               float64  `bson:"vat_price" json:"vat_price"`
	Total                  float64  `bson:"total" json:"total"`
	TotalWithVAT           float64  `bson:"total_with_vat" json:"total_with_vat"`
	ActualVatPrice         float64  `bson:"actual_vat_price" json:"actual_vat_price"`
	ActualTotal            float64  `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT     float64  `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	RoundingAmount         float64  `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount     bool     `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	NetTotal               float64  `bson:"net_total" json:"net_total"`
	ActualNetTotal         float64  `bson:"actual_net_total" json:"actual_net_total"`
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
	CustomerName        string               `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic  string               `json:"customer_name_arabic" bson:"customer_name_arabic"`
	StoreName           string               `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName       string               `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName       string               `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName       string               `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Profit              float64              `bson:"profit" json:"profit"`
	NetProfit           float64              `bson:"net_profit" json:"net_profit"`
	Loss                float64              `bson:"loss" json:"loss"`
	NetLoss             float64              `bson:"net_loss" json:"net_loss"`
	TotalPaymentPaid    float64              `bson:"total_payment_paid" json:"total_payment_paid"`
	BalanceAmount       float64              `bson:"balance_amount" json:"balance_amount"`
	Payments            []SalesReturnPayment `bson:"payments" json:"payments"`
	PaymentsInput       []SalesReturnPayment `bson:"-" json:"payments_input"`
	PaymentsCount       int64                `bson:"payments_count" json:"payments_count"`
	Zatca               ZatcaReporting       `bson:"zatca,omitempty" json:"zatca,omitempty"`
	Remarks             string               `bson:"remarks" json:"remarks"`
	Phone               string               `bson:"phone" json:"phone"`
	VatNo               string               `bson:"vat_no" json:"vat_no"`
	Address             string               `bson:"address" json:"address"`
	EnableReportToZatca bool                 `json:"enable_report_to_zatca" bson:"-"`
}

func (model *SalesReturn) SetPostBalances() error {
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

func (salesReturn *SalesReturn) DeletePaymentsByPayablePaymentID(payablePaymentID primitive.ObjectID) error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("sales_return_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"payable_payment_id": payablePaymentID})
	if err != nil {
		return err
	}
	return nil
}

func (salesReturn *SalesReturn) UpdatePaymentFromPayablePayment(
	payablePayment PayablePayment,
	customerWithdrawal *CustomerWithdrawal,
) error {
	store, _ := FindStoreByID(salesReturn.StoreID, bson.M{})

	paymentExists := false
	for _, salesReturnPayment := range salesReturn.Payments {
		if salesReturnPayment.PayablePaymentID != nil && salesReturnPayment.PayablePaymentID.Hex() == payablePayment.ID.Hex() {
			paymentExists = true
			salesReturnPaymentObj, err := store.FindSalesReturnPaymentByID(&salesReturnPayment.ID, bson.M{})
			if err != nil {
				return errors.New("error finding sales payment: " + err.Error())
			}

			salesReturnPaymentObj.Amount = payablePayment.Amount
			salesReturnPaymentObj.Date = payablePayment.Date
			salesReturnPaymentObj.Method = payablePayment.Method
			salesReturnPaymentObj.UpdatedAt = payablePayment.UpdatedAt
			salesReturnPaymentObj.CreatedAt = payablePayment.CreatedAt
			salesReturnPaymentObj.UpdatedBy = payablePayment.UpdatedBy
			salesReturnPaymentObj.CreatedBy = payablePayment.CreatedBy
			salesReturnPaymentObj.PayableID = &customerWithdrawal.ID
			salesReturnPaymentObj.ReferenceID = &customerWithdrawal.ID
			salesReturnPaymentObj.ReferenceType = "customer_withdrawal"
			salesReturnPaymentObj.ReferenceCode = customerWithdrawal.Code

			err = salesReturnPaymentObj.Update()
			if err != nil {
				return err
			}
			break
		}
	}

	if !paymentExists {
		newSalesReturnPayment := SalesReturnPayment{
			SalesReturnID:    &salesReturn.ID,
			SalesReturnCode:  salesReturn.Code,
			OrderID:          salesReturn.OrderID,
			OrderCode:        salesReturn.OrderCode,
			Amount:           payablePayment.Amount,
			Date:             payablePayment.Date,
			Method:           payablePayment.Method,
			PayablePaymentID: &payablePayment.ID,
			PayableID:        &customerWithdrawal.ID,
			CreatedBy:        payablePayment.CreatedBy,
			UpdatedBy:        payablePayment.UpdatedBy,
			CreatedAt:        payablePayment.CreatedAt,
			UpdatedAt:        payablePayment.UpdatedAt,
			StoreID:          salesReturn.StoreID,
			ReferenceID:      &customerWithdrawal.ID,
			ReferenceType:    "customer_withdrawal",
			ReferenceCode:    customerWithdrawal.Code,
		}
		err := newSalesReturnPayment.Insert()
		if err != nil {
			return errors.New("error inserting sales payment: " + err.Error())
		}
	}

	return nil
}

func (salesReturn *SalesReturn) AddPayments() error {
	for _, payment := range salesReturn.PaymentsInput {
		salesReturnPayment := SalesReturnPayment{
			SalesReturnID:   &salesReturn.ID,
			SalesReturnCode: salesReturn.Code,
			OrderID:         salesReturn.OrderID,
			OrderCode:       salesReturn.OrderCode,
			Amount:          payment.Amount,
			Method:          payment.Method,
			Date:            payment.Date,
			CreatedAt:       salesReturn.CreatedAt,
			UpdatedAt:       salesReturn.UpdatedAt,
			CreatedBy:       salesReturn.CreatedBy,
			CreatedByName:   salesReturn.CreatedByName,
			UpdatedBy:       salesReturn.UpdatedBy,
			UpdatedByName:   salesReturn.UpdatedByName,
			StoreID:         salesReturn.StoreID,
			StoreName:       salesReturn.StoreName,
		}
		err := salesReturnPayment.Insert()
		if err != nil {
			return err
		}
	}
	return nil
}

func (salesReturn *SalesReturn) UpdatePayments() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	salesReturn.SetPaymentStatus()
	now := time.Now()
	for _, payment := range salesReturn.PaymentsInput {
		if payment.ID.IsZero() {
			//Create new
			salesReturnPayment := SalesReturnPayment{
				SalesReturnID:   &salesReturn.ID,
				SalesReturnCode: salesReturn.Code,
				OrderID:         salesReturn.OrderID,
				OrderCode:       salesReturn.OrderCode,
				Amount:          payment.Amount,
				Method:          payment.Method,
				Date:            payment.Date,
				CreatedAt:       &now,
				UpdatedAt:       &now,
				CreatedBy:       salesReturn.CreatedBy,
				CreatedByName:   salesReturn.CreatedByName,
				UpdatedBy:       salesReturn.UpdatedBy,
				UpdatedByName:   salesReturn.UpdatedByName,
				StoreID:         salesReturn.StoreID,
				StoreName:       salesReturn.StoreName,
			}
			err := salesReturnPayment.Insert()
			if err != nil {
				return err
			}

		} else {
			//Update
			salesReturnPayment, err := store.FindSalesReturnPaymentByID(&payment.ID, bson.M{})
			if err != nil {
				return errors.New("sales return payment id not found: " + err.Error())
			}

			salesReturnPayment.Date = payment.Date
			salesReturnPayment.Amount = payment.Amount
			salesReturnPayment.Method = payment.Method
			salesReturnPayment.UpdatedAt = &now
			salesReturnPayment.UpdatedBy = salesReturn.UpdatedBy
			salesReturnPayment.UpdatedByName = salesReturn.UpdatedByName
			err = salesReturnPayment.Update()
			if err != nil {
				return err
			}
		}

	}

	//Deleting payments

	paymentsToDelete := []SalesReturnPayment{}

	for _, payment := range salesReturn.Payments {
		found := false
		for _, paymentInput := range salesReturn.PaymentsInput {
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
		payment.DeletedBy = salesReturn.UpdatedBy
		err := payment.Update()
		if err != nil {
			return err
		}

		err = salesReturn.RemoveInvoiceFromCustomerPayablePayment(&payment)
		if err != nil {
			return err
		}
	}

	return nil
}

func (salesReturn *SalesReturn) RemoveInvoiceFromCustomerPayablePayment(salesReturnPayment *SalesReturnPayment) error {
	store, _ := FindStoreByID(salesReturn.StoreID, bson.M{})
	//Remove Invoice from Customer receivable payment
	if salesReturnPayment.PayablePaymentID != nil && !salesReturnPayment.PayablePaymentID.IsZero() {
		customerWithdrawal, err := store.FindCustomerWithdrawalByID(salesReturnPayment.PayableID, bson.M{})
		if err != nil {
			return err
		}

		for i, payablePayment := range customerWithdrawal.Payments {
			if payablePayment.InvoiceID != nil && !payablePayment.InvoiceID.IsZero() {
				if payablePayment.ID.Hex() == salesReturnPayment.PayablePaymentID.Hex() &&
					customerWithdrawal.ID.Hex() == salesReturnPayment.PayableID.Hex() &&
					payablePayment.InvoiceID.Hex() == salesReturn.ID.Hex() {
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
type SalesReturnStats struct {
	ID                     *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal               float64             `json:"net_total" bson:"net_total"`
	VatPrice               float64             `json:"vat_price" bson:"vat_price"`
	Discount               float64             `json:"discount" bson:"discount"`
	CashDiscount           float64             `json:"cash_discount" bson:"cash_discount"`
	NetProfit              float64             `json:"net_profit" bson:"net_profit"`
	NetLoss                float64             `json:"net_loss" bson:"net_loss"`
	PaidSalesReturn        float64             `json:"paid_sales_return" bson:"paid_sales_return"`
	UnPaidSalesReturn      float64             `json:"unpaid_sales_return" bson:"unpaid_sales_return"`
	CashSalesReturn        float64             `json:"cash_sales_return" bson:"cash_sales_return"`
	BankAccountSalesReturn float64             `json:"bank_account_sales_return" bson:"bank_account_sales_return"`
	ShippingOrHandlingFees float64             `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
	SalesReturnCount       int64               `json:"sales_return_count" bson:"sales_return_count"`
	SalesSalesReturn       float64             `json:"sales_sales_return" bson:"sales_sales_return"`
}

func (store *Store) GetSalesReturnStats(filter map[string]interface{}) (stats SalesReturnStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
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
				"paid_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"unpaid_sales_return": bson.M{"$sum": "$balance_amount"},
				"cash_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"bank_account_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"sales_sales_return": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$payments",
						"as":    "payment",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "sales"}},
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

func (salesreturn *SalesReturn) AttributesValueChangeEvent(salesreturnOld *SalesReturn) error {

	return nil
}

func (salesreturn *SalesReturn) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(salesreturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if salesreturn.StoreID != nil {
		store, err := FindStoreByID(salesreturn.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.StoreName = store.Name
	}

	if salesreturn.CustomerID != nil && !salesreturn.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(salesreturn.CustomerID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1})
		if err != nil {
			return err
		}
		salesreturn.CustomerName = customer.Name
		salesreturn.CustomerNameArabic = customer.NameInArabic
	} else {
		salesreturn.CustomerName = ""
		salesreturn.CustomerNameArabic = ""
	}

	/*
		if salesreturn.ReceivedBy != nil {
			receivedByUser, err := FindUserByID(salesreturn.ReceivedBy, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			salesreturn.ReceivedByName = receivedByUser.Name
		}
	*/

	/*
		if salesreturn.ReceivedBySignatureID != nil {
			receivedBySignature, err := FindSignatureByID(salesreturn.ReceivedBySignatureID, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			salesreturn.ReceivedBySignatureName = receivedBySignature.Name
		}
	*/

	if salesreturn.CreatedBy != nil {
		createdByUser, err := FindUserByID(salesreturn.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.CreatedByName = createdByUser.Name
	}

	if salesreturn.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(salesreturn.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.UpdatedByName = updatedByUser.Name
	}

	/*
		if salesreturn.DeletedBy != nil && !salesreturn.DeletedBy.IsZero() {
			deletedByUser, err := FindUserByID(salesreturn.DeletedBy, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			salesreturn.DeletedByName = deletedByUser.Name
		}*/

	for i, product := range salesreturn.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1})
		if err != nil {
			return err
		}
		//salesreturn.Products[i].Name = productObject.Name
		salesreturn.Products[i].NameInArabic = productObject.NameInArabic
		salesreturn.Products[i].ItemCode = productObject.ItemCode
		//salesreturn.Products[i].PartNumber = productObject.PartNumber
		salesreturn.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

func (salesReturn *SalesReturn) FindNetTotal() {
	salesReturn.ShippingOrHandlingFees = RoundTo2Decimals(salesReturn.ShippingOrHandlingFees)
	salesReturn.Discount = RoundTo2Decimals(salesReturn.Discount)

	salesReturn.FindTotal()

	/*
		if salesReturn.DiscountWithVAT > 0 {
			salesReturn.Discount = RoundTo2Decimals(salesReturn.DiscountWithVAT / (1 + (*salesReturn.VatPercent / 100)))
		} else if salesReturn.Discount > 0 {
			salesReturn.DiscountWithVAT = RoundTo2Decimals(salesReturn.Discount * (1 + (*salesReturn.VatPercent / 100)))
		} else {
			salesReturn.Discount = 0
			salesReturn.DiscountWithVAT = 0
		}*/

	// Apply discount to the base amount first
	baseTotal := salesReturn.Total + salesReturn.ShippingOrHandlingFees - salesReturn.Discount
	baseTotal = RoundTo2Decimals(baseTotal)

	//Actual
	actualBaseTotal := salesReturn.ActualTotal + salesReturn.ShippingOrHandlingFees - salesReturn.Discount
	actualBaseTotal = RoundTo8Decimals(actualBaseTotal)

	// Now calculate VAT on the discounted base
	salesReturn.VatPrice = RoundTo2Decimals(baseTotal * (*salesReturn.VatPercent / 100))

	//Actual
	salesReturn.ActualVatPrice = RoundTo2Decimals(actualBaseTotal * (*salesReturn.VatPercent / 100))

	salesReturn.NetTotal = RoundTo2Decimals(baseTotal + salesReturn.VatPrice)

	//actual
	salesReturn.ActualNetTotal = RoundTo2Decimals(actualBaseTotal + salesReturn.ActualVatPrice)

	if salesReturn.AutoRoundingAmount {
		salesReturn.RoundingAmount = RoundTo2Decimals(salesReturn.ActualNetTotal - salesReturn.NetTotal)
	}

	salesReturn.NetTotal = RoundTo2Decimals(salesReturn.NetTotal + salesReturn.RoundingAmount)

	salesReturn.CalculateDiscountPercentage()
}

func (salesReturn *SalesReturn) FindTotal() {
	total := float64(0.0)
	totalWithVAT := float64(0.0)
	actualTotal := float64(0.0)
	actualTotalWithVAT := float64(0.0)
	for i, product := range salesReturn.Products {
		if !product.Selected {
			continue
		}

		/*
			if product.UnitPriceWithVAT > 0 {
				salesReturn.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPriceWithVAT / (1 + (*salesReturn.VatPercent / 100)))
			} else if product.UnitPrice > 0 {
				salesReturn.Products[i].UnitPriceWithVAT = RoundTo2Decimals(product.UnitPrice * (1 + (*salesReturn.VatPercent / 100)))
			}

			if product.UnitDiscountWithVAT > 0 {
				salesReturn.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscountWithVAT / (1 + (*salesReturn.VatPercent / 100)))
			} else if product.UnitDiscount > 0 {
				salesReturn.Products[i].UnitDiscountWithVAT = RoundTo2Decimals(product.UnitDiscount * (1 + (*salesReturn.VatPercent / 100)))
			}

			if product.UnitDiscountPercentWithVAT > 0 {
				salesReturn.Products[i].UnitDiscountPercent = RoundTo2Decimals((product.UnitDiscount / product.UnitPrice) * 100)
			} else if product.UnitDiscountPercent > 0 {
				salesReturn.Products[i].UnitDiscountPercentWithVAT = RoundTo2Decimals((product.UnitDiscountWithVAT / product.UnitPriceWithVAT) * 100)
			}*/

		total += (product.Quantity * (salesReturn.Products[i].UnitPrice - salesReturn.Products[i].UnitDiscount))
		total = RoundTo2Decimals(total)

		actualTotal += (product.Quantity * (salesReturn.Products[i].UnitPrice - salesReturn.Products[i].UnitDiscount))
		actualTotal = RoundTo8Decimals(actualTotal)

		totalWithVAT += (product.Quantity * (salesReturn.Products[i].UnitPriceWithVAT - salesReturn.Products[i].UnitDiscountWithVAT))
		totalWithVAT = RoundTo2Decimals(totalWithVAT)

		actualTotalWithVAT += (product.Quantity * (salesReturn.Products[i].UnitPriceWithVAT - salesReturn.Products[i].UnitDiscountWithVAT))
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)
	}

	//salesReturn.Total = RoundTo2Decimals(total)
	//salesReturn.TotalWithVAT = RoundTo2Decimals(totalWithVAT)
	salesReturn.Total = total
	salesReturn.TotalWithVAT = totalWithVAT
	salesReturn.ActualTotal = actualTotal
	salesReturn.ActualTotalWithVAT = actualTotalWithVAT
}

func (salesReturn *SalesReturn) CalculateDiscountPercentage() {
	if salesReturn.Discount <= 0 {
		salesReturn.DiscountPercent = 0.00
		salesReturn.DiscountPercentWithVAT = 0.00
		return
	}

	baseBeforeDiscount := salesReturn.NetTotal + salesReturn.Discount
	if baseBeforeDiscount == 0 {
		salesReturn.DiscountPercent = 0.00
		salesReturn.DiscountPercentWithVAT = 0.00
		return
	}

	percentage := (salesReturn.Discount / baseBeforeDiscount) * 100
	salesReturn.DiscountPercent = RoundTo2Decimals(percentage)

	baseBeforeDiscountWithVAT := salesReturn.NetTotal + salesReturn.DiscountWithVAT
	if baseBeforeDiscountWithVAT == 0 {
		salesReturn.DiscountPercentWithVAT = 0.00
		salesReturn.DiscountPercent = 0.00
		return
	}

	percentage = (salesReturn.DiscountWithVAT / baseBeforeDiscountWithVAT) * 100
	salesReturn.DiscountPercentWithVAT = RoundTo2Decimals(percentage)
}

/*
func (salesreturn *SalesReturn) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range salesreturn.Products {
		if !product.Selected {
			continue
		}
		netTotal += (product.Quantity * (product.UnitPrice - product.UnitDiscount))
	}

	netTotal += salesreturn.ShippingOrHandlingFees
	netTotal -= salesreturn.Discount

	if salesreturn.VatPercent != nil {
		netTotal += netTotal * (*salesreturn.VatPercent / float64(100))
	}

	salesreturn.NetTotal = ToFixed2(netTotal, 2)
}
*/

/*
func (salesReturn *SalesReturn) FindNetTotal() {
	netTotal := float64(0.0)
	salesReturn.FindTotal()
	netTotal = salesReturn.Total
	salesReturn.ShippingOrHandlingFees = RoundTo2Decimals(salesReturn.ShippingOrHandlingFees)
	salesReturn.Discount = RoundTo2Decimals(salesReturn.Discount)

	netTotal += salesReturn.ShippingOrHandlingFees
	netTotal -= salesReturn.Discount

	salesReturn.FindVatPrice()
	netTotal += salesReturn.VatPrice

	salesReturn.NetTotal = RoundTo2Decimals(netTotal)
	salesReturn.CalculateDiscountPercentage()
}

func (salesReturn *SalesReturn) CalculateDiscountPercentage() {
	if salesReturn.NetTotal == 0 {
		salesReturn.DiscountPercent = 0
	}

	if salesReturn.Discount <= 0 {
		salesReturn.DiscountPercent = 0.00
		return
	}

	percentage := (salesReturn.Discount / salesReturn.NetTotal) * 100
	salesReturn.DiscountPercent = RoundTo2Decimals(percentage) // Use rounding here
}

func (salesReturn *SalesReturn) FindTotal() {
	total := float64(0.0)
	for i, product := range salesReturn.Products {
		if !product.Selected {
			continue
		}

		salesReturn.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPrice)
		salesReturn.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscount)
		if salesReturn.Products[i].UnitDiscount > 0 {
			salesReturn.Products[i].UnitDiscountPercent = RoundTo2Decimals((salesReturn.Products[i].UnitDiscount / salesReturn.Products[i].UnitPrice) * 100)
		}

		total += RoundTo2Decimals(product.Quantity * (salesReturn.Products[i].UnitPrice - salesReturn.Products[i].UnitDiscount))
	}

	salesReturn.Total = RoundTo2Decimals(total)
}

func (salesReturn *SalesReturn) FindVatPrice() {
	vatPrice := ((*salesReturn.VatPercent / float64(100.00)) * ((salesReturn.Total + salesReturn.ShippingOrHandlingFees) - salesReturn.Discount))
	salesReturn.VatPrice = RoundTo2Decimals(vatPrice)
}
*/

/*
func (salesreturn *SalesReturn) FindTotal() {
	total := float64(0.0)
	for _, product := range salesreturn.Products {
		if !product.Selected {
			continue
		}

		total += (product.Quantity * (product.UnitPrice - product.UnitDiscount))
	}
	salesreturn.Total = RoundFloat(total, 2)
}
*/

func (salesreturn *SalesReturn) FindTotalQuantity() {
	totalQuantity := float64(0)
	for _, product := range salesreturn.Products {
		if !product.Selected {
			continue
		}

		totalQuantity += product.Quantity
	}
	salesreturn.TotalQuantity = totalQuantity
}

/*
func (model *SalesReturn) FindVatPrice() {
	vatPrice := ((*model.VatPercent / float64(100.00)) * ((model.Total + model.ShippingOrHandlingFees) - model.Discount))
	vatPrice = RoundFloat(vatPrice, 2)
	model.VatPrice = vatPrice
}
*/

func (store *Store) SearchSalesReturn(w http.ResponseWriter, r *http.Request) (salesreturns []SalesReturn, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[order_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return salesreturns, criterias, err
		}
		criterias.SearchBy["order_id"] = orderID
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return salesreturns, criterias, err
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

	keys, ok = r.URL.Query()["search[order_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["order_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
			return salesreturns, criterias, err
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
				return salesreturns, criterias, err
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
				return salesreturns, criterias, err
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
			return salesreturns, criterias, err
		}
		criterias.SearchBy["received_by"] = receivedByID
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return salesreturns, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")

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
		return salesreturns, criterias, errors.New("Error fetching salesreturns:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return salesreturns, criterias, errors.New("Cursor error:" + err.Error())
		}
		salesreturn := SalesReturn{}
		err = cur.Decode(&salesreturn)
		if err != nil {
			return salesreturns, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			salesreturn.Store, _ = FindStoreByID(salesreturn.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			salesreturn.Customer, _ = store.FindCustomerByID(salesreturn.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			salesreturn.CreatedByUser, _ = FindUserByID(salesreturn.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			salesreturn.UpdatedByUser, _ = FindUserByID(salesreturn.UpdatedBy, updatedByUserSelectFields)
		}
		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				salesreturn.DeletedByUser, _ = FindUserByID(salesreturn.DeletedBy, deletedByUserSelectFields)
			}
		*/
		salesreturns = append(salesreturns, salesreturn)
	} //end for loop

	return salesreturns, criterias, nil
}

func (model *SalesReturn) FindLastReportedSalesReturn(selectFields map[string]interface{}) (lastReportedSalesReturn *SalesReturn, err error) {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("salesreturn")
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
		Decode(&lastReportedSalesReturn)
	if err != nil {
		return nil, err
	}

	return lastReportedSalesReturn, err
}

func (salesreturn *SalesReturn) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldSalesReturn *SalesReturn) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(salesreturn.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "Store is required"
	}

	if !govalidator.IsNull(strings.TrimSpace(salesreturn.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(salesreturn.Phone)) {
		errs["phone"] = "Invalid phone no."
		return
	}

	if !govalidator.IsNull(strings.TrimSpace(salesreturn.VatNo)) && !IsValidDigitNumber(strings.TrimSpace(salesreturn.VatNo), "15") {
		errs["vat_no"] = "VAT No. should be 15 digits"
		return
	} else if !govalidator.IsNull(strings.TrimSpace(salesreturn.VatNo)) && !IsNumberStartAndEndWith(strings.TrimSpace(salesreturn.VatNo), "3") {
		errs["vat_no"] = "VAT No. should start and end with 3"
		return
	}

	order, err := store.FindOrderByID(salesreturn.OrderID, bson.M{})
	if err != nil {
		errs["order_id"] = "Order is invalid"
	}

	customer, err := store.FindCustomerByID(salesreturn.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "invalid customer"
	}

	if salesreturn.Discount < 0 {
		errs["discount"] = "Cash discount should not be < 0"
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

		if !govalidator.IsNull(customer.NationalAddress.BuildingNo) && !IsValidDigitNumber(customer.NationalAddress.BuildingNo, "4") {
			customerErrorMessages = append(customerErrorMessages, "Building number should be 4 digits")
		}

		if !govalidator.IsNull(customer.NationalAddress.ZipCode) && !IsValidDigitNumber(customer.NationalAddress.ZipCode, "5") {
			customerErrorMessages = append(customerErrorMessages, "Zip code should be 5 digits")
		}

		if len(customerErrorMessages) > 0 {
			errs["customer_id"] = "Fix the customer errors: " + strings.Join(customerErrorMessages, ",")
		}
	}

	if govalidator.IsNull(salesreturn.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, salesreturn.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		salesreturn.Date = &date
	}

	totalPayment := float64(0.00)
	for _, payment := range salesreturn.PaymentsInput {
		if payment.Amount > 0 {
			totalPayment += payment.Amount
		}
	}

	for index, payment := range salesreturn.PaymentsInput {
		if order.PaymentStatus == "not_paid" {
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

			salesreturn.PaymentsInput[index].Date = &date
			payment.Date = &date

			if salesreturn.Date != nil && IsAfter(salesreturn.Date, salesreturn.PaymentsInput[index].Date) {
				errs["payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to sales return date time"
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

	if salesreturn.OrderID == nil || salesreturn.OrderID.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		errs["order_id"] = "Order ID is required"
		return errs
	}

	/*
		maxDiscountAllowed := 0.00
		if scenario == "update" {
			maxDiscountAllowed = order.DiscountWithVAT - (order.ReturnDiscountWithVAT - oldSalesReturn.DiscountWithVAT)
		} else {
			maxDiscountAllowed = order.DiscountWithVAT - order.ReturnDiscountWithVAT
		}

		if salesreturn.DiscountWithVAT > maxDiscountAllowed {
			errs["discount_with_vat"] = "Discount shoul not be greater than " + fmt.Sprintf("%.2f", (maxDiscountAllowed))
		}

		if salesreturn.NetTotal > 0 && salesreturn.CashDiscount >= salesreturn.NetTotal {
			errs["cash_discount"] = "Cash discount should not be >= " + fmt.Sprintf("%.02f", salesreturn.NetTotal)
		} else if salesreturn.CashDiscount < 0 {
			errs["cash_discount"] = "Cash discount should not < 0 "
		}*/

	/*
		maxCashDiscountAllowed := 0.00
		if scenario == "update" {
			maxCashDiscountAllowed = order.CashDiscount - (order.ReturnCashDiscount - oldSalesReturn.CashDiscount)
		} else {
			maxCashDiscountAllowed = order.CashDiscount - order.ReturnCashDiscount
		}

		if salesreturn.NetTotal > 0 && salesreturn.CashDiscount > maxCashDiscountAllowed {
			errs["cash_discount"] = "Cash discount shouldn't greater than " + fmt.Sprintf("%.2f", (maxCashDiscountAllowed))
		}
	*/

	salesreturn.OrderCode = order.Code

	/*
		if govalidator.IsNull(salesreturn.PaymentStatus) {
			errs["payment_status"] = "Payment status is required"
		}
	*/

	/*
		if !govalidator.IsNull(salesreturn.SignatureDateStr) {
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, salesreturn.SignatureDateStr)
			if err != nil {
				errs["signature_date_str"] = "Invalid date format"
			}
			salesreturn.SignatureDate = &date
		}*/

	if scenario == "update" {
		if salesreturn.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsSalesReturnExists(&salesreturn.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid SalesReturn:" + salesreturn.ID.Hex()
		}

	}

	if salesreturn.StoreID == nil || salesreturn.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(salesreturn.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + salesreturn.StoreID.Hex()
			return errs
		}
	}

	if len(salesreturn.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for salesreturn"
	}

	for index, salesReturnProduct := range salesreturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		if salesReturnProduct.ProductID.IsZero() {
			errs["product_id"] = "Product is required for Sales Return"
		} else {
			exists, err := store.IsProductExists(&salesReturnProduct.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(index)] = "Invalid product_id:" + salesReturnProduct.ProductID.Hex() + " in products"
			}
		}

		if salesReturnProduct.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(index)] = "Quantity is required"
		}

		if govalidator.IsNull(strings.TrimSpace(salesReturnProduct.Name)) {
			errs["name_"+strconv.Itoa(index)] = "Name is required"
		} else if len(salesReturnProduct.Name) < 3 {
			errs["name_"+strconv.Itoa(index)] = "Name requires min. 3 chars"
		}

		if salesReturnProduct.UnitDiscount > salesReturnProduct.UnitPrice && salesReturnProduct.UnitPrice > 0 {
			errs["unit_discount_"+strconv.Itoa(index)] = "Unit discount should not be greater than unit price"
		}

		for _, orderProduct := range order.Products {
			if orderProduct.ProductID == salesReturnProduct.ProductID {
				//soldQty := RoundFloat((orderProduct.Quantity - orderProduct.QuantityReturned), 2)
				maxAllowedQuantity := 0.00
				if scenario == "update" {
					maxAllowedQuantity = orderProduct.Quantity - (orderProduct.QuantityReturned - oldSalesReturn.Products[index].Quantity)
				} else {
					maxAllowedQuantity = orderProduct.Quantity - orderProduct.QuantityReturned
				}

				if salesReturnProduct.Quantity > maxAllowedQuantity {
					errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", maxAllowedQuantity) + " " + orderProduct.Unit
				}
				/*
					soldQty := RoundFloat((orderProduct.Quantity - orderProduct.QuantityReturned), 2)
					if soldQty == 0 {
						errs["quantity_"+strconv.Itoa(index)] = "Already returned all sold quantities"
					} else if salesReturnProduct.Quantity > float64(soldQty) {
						errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", soldQty) + " " + orderProduct.Unit
					}
				*/
			}
		}

		/*
			stock, err := GetProductStockInStore(&product.ProductID, salesreturn.StoreID, product.Quantity)
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

				storeObject, err := FindStoreByID(salesreturn.StoreID, nil)
				if err != nil {
					errs["store"] = err.Error()
					return errs
				}

				errs["quantity_"+strconv.Itoa(index)] = "Product: " + productObject.Name + " stock is only " + strconv.Itoa(stock) + " in Store: " + storeObject.Name
			}
		*/
	}

	if salesreturn.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if totalPayment > RoundTo2Decimals(salesreturn.NetTotal-salesreturn.CashDiscount) {
		errs["total_payment"] = "Total payment should not exceed: " + fmt.Sprintf("%.02f", RoundTo2Decimals(salesreturn.NetTotal-salesreturn.CashDiscount)) + " (Net Total - Cash Discount)"
		return
	}

	if totalPayment > order.NetTotal {
		errs["total_payment"] = "Total payment amount should not exceed Original Sales Net Total: " + fmt.Sprintf("%.02f", (order.NetTotal))
		return
	}

	if salesreturn.NetTotal > order.NetTotal {
		errs["net_total"] = "Net Total  should not exceed Original Sales Net Total: " + fmt.Sprintf("%.02f", (order.NetTotal))
		return
	}

	if scenario == "update" {
		if totalPayment > RoundTo2Decimals(order.TotalPaymentReceived-(order.ReturnAmount-oldSalesReturn.TotalPaymentPaid)) {
			errs["total_payment"] = "Total payment should not be greater than " + fmt.Sprintf("%.2f", (order.TotalPaymentReceived-(order.ReturnAmount-oldSalesReturn.TotalPaymentPaid))) + " (total payment received)"
			return errs
		}
	} else {
		if totalPayment > RoundTo2Decimals(order.TotalPaymentReceived-order.ReturnAmount) {
			errs["total_payment"] = "Total payment should not be greater than " + fmt.Sprintf("%.2f", (order.TotalPaymentReceived-order.ReturnAmount)) + " (total payment received)"
			return errs
		}
	}

	if customer != nil && customer.CreditLimit > 0 {
		if customer.Account == nil {
			customer.Account = &Account{}
			if salesreturn.BalanceAmount > 0 {
				customer.Account.Type = "liability"
			} else {
				customer.Account.Type = "asset"
			}
		}

		if scenario != "update" && customer.IsCreditLimitExceeded(salesreturn.BalanceAmount, true) {
			errs["customer_credit_limit"] = "Exceeding customer credit limit: " + fmt.Sprintf("%.02f", (customer.CreditLimit-customer.CreditBalance))
			return errs
		} else if scenario == "update" && customer.WillEditExceedCreditLimit(oldSalesReturn.BalanceAmount, salesreturn.BalanceAmount, true) {
			errs["customer_credit_limit"] = "Exceeding customer credit limit: " + fmt.Sprintf("%.02f", ((customer.CreditLimit+oldSalesReturn.BalanceAmount)-customer.CreditBalance))
			return errs
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (salesreturn *SalesReturn) UpdateReturnedQuantityInOrderProduct(salesReturnOld *SalesReturn) error {
	store, err := FindStoreByID(salesreturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	order, err := store.FindOrderByID(salesreturn.OrderID, bson.M{})
	if err != nil {
		return err
	}

	if salesReturnOld != nil {
		for _, salesReturnProduct := range salesReturnOld.Products {
			if !salesReturnProduct.Selected {
				continue
			}

			for index2, orderProduct := range order.Products {
				if orderProduct.ProductID == salesReturnProduct.ProductID {
					if order.Products[index2].QuantityReturned > 0 {
						order.Products[index2].QuantityReturned -= salesReturnProduct.Quantity
					}
				}
			}
		}
	}

	for _, salesReturnProduct := range salesreturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		for index2, orderProduct := range order.Products {
			if orderProduct.ProductID == salesReturnProduct.ProductID {
				order.Products[index2].QuantityReturned += salesReturnProduct.Quantity
			}
		}
	}

	err = order.CalculateOrderProfit()
	if err != nil {
		return err
	}

	err = order.Update()
	if err != nil {
		return err
	}

	return nil
}

/*
func GetProductStockInStore(
	productID *primitive.ObjectID,
	storeID *primitive.ObjectID,
	salesreturnQuantity int,
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
func (salesreturn *SalesReturn) RemoveStock() (err error) {
	if len(salesreturn.Products) == 0 {
		return nil
	}

	for _, salesreturnProduct := range salesreturn.Products {
		product, err := FindProductByID(&salesreturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		for k, productStore := range product.Stores {
			if productStore.StoreID.Hex() == salesreturn.StoreID.Hex() {

				salesreturn.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stores[k].Stock,
					(product.Stores[k].Stock - salesreturnProduct.Quantity),
				)

				product.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stores[k].Stock,
					(product.Stores[k].Stock - salesreturnProduct.Quantity),
				)

				product.Stores[k].Stock -= salesreturnProduct.Quantity
				salesreturn.StockAdded = true
				break
			}
		}

		err = product.Update()
		if err != nil {
			return err
		}

	}

	err = salesreturn.Update()
	if err != nil {
		return err
	}
	return nil
}
*/

func (salesreturn *SalesReturn) SetProductsStock() (err error) {
	store, err := FindStoreByID(salesreturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(salesreturn.Products) == 0 {
		return nil
	}

	for _, salesreturnProduct := range salesreturn.Products {
		if !salesreturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&salesreturnProduct.ProductID, bson.M{})
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

func (salesreturn *SalesReturn) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(salesreturn.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": salesreturn.StoreID}, "salesreturn")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (salesreturn *SalesReturn) Insert() error {
	collection := db.GetDB("store_" + salesreturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	salesreturn.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &salesreturn)
	if err != nil {
		return err
	}

	return nil
}

func (salesReturn *SalesReturn) CalculateSalesReturnProfit() error {
	totalProfit := float64(0.0)
	totalLoss := float64(0.0)
	/*
		order, err := FindOrderByID(salesReturn.OrderID, map[string]interface{}{})
		if err != nil {
			return err
		}
	*/

	for i, product := range salesReturn.Products {
		if !product.Selected {
			continue
		}

		quantity := product.Quantity
		salesPrice := (quantity * (product.UnitPrice - product.UnitDiscount))
		//purchaseUnitPrice := product.PurchaseUnitPrice
		/*
			for _, orderProduct := range order.Products {
				if orderProduct.ProductID.Hex() == product.ProductID.Hex() {
					purchaseUnitPrice = orderProduct.PurchaseUnitPrice
					break
				}
			}*/
		//salesReturn.Products[i].PurchaseUnitPrice = purchaseUnitPrice
		purchaseUnitPrice := salesReturn.Products[i].PurchaseUnitPrice

		/*
			if purchaseUnitPrice == 0 ||
				salesReturn.Products[i].Loss > 0 ||
				salesReturn.Products[i].Profit <= 0 {
				product, err := FindProductByID(&product.ProductID, map[string]interface{}{})
				if err != nil {
					return err
				}
				for _, productStore := range product.ProductStores {
					if productStore.StoreID == *salesReturn.StoreID {
						purchaseUnitPrice = productStore.PurchaseUnitPrice
						salesReturn.Products[i].PurchaseUnitPrice = purchaseUnitPrice
						break
					}
				}
			}*/

		profit := 0.0
		if purchaseUnitPrice > 0 {
			profit = salesPrice - (quantity * purchaseUnitPrice)
		}

		profit = RoundFloat(profit, 2)

		if profit >= 0 {
			salesReturn.Products[i].Profit = profit
			salesReturn.Products[i].Loss = 0.0
			totalProfit += salesReturn.Products[i].Profit
		} else {
			salesReturn.Products[i].Profit = 0
			salesReturn.Products[i].Loss = (profit * -1)
			totalLoss += salesReturn.Products[i].Loss
		}

	}
	salesReturn.Profit = totalProfit
	salesReturn.NetProfit = (totalProfit - salesReturn.CashDiscount) - salesReturn.Discount
	salesReturn.Loss = totalLoss
	salesReturn.NetLoss = totalLoss
	if salesReturn.NetProfit < 0 {
		salesReturn.NetLoss += (salesReturn.NetProfit * -1)
		salesReturn.NetProfit = 0.00
	}

	return nil
}

func (salesReturn *SalesReturn) MakeRedisCode() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := salesReturn.StoreID.Hex() + "_return_invoice_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from order.CreatedAt or fallback to order.Date or now ===
	baseTime := salesReturn.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("salesreturn")
		if err != nil {
			return err
		}
		startFrom := store.SalesReturnSerialNumber.StartFromCount
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
	useMonthly := strings.Contains(store.SalesReturnSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := salesReturn.StoreID.Hex() + "_return_invoice_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.SalesReturnSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "salesreturn")
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
	paddingCount := store.SalesReturnSerialNumber.PaddingCount
	if store.SalesReturnSerialNumber.Prefix != "" {
		salesReturn.Code = fmt.Sprintf("%s-%0*d", store.SalesReturnSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		salesReturn.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(salesReturn.Code, "DATE") {
		orderDate := baseTime.Format("20060102") // YYYYMMDD
		salesReturn.Code = strings.ReplaceAll(salesReturn.Code, "DATE", orderDate)
	}

	// === 8. Set InvoiceCountValue (based on global counter) ===
	salesReturn.InvoiceCountValue = globalIncr - (store.SalesReturnSerialNumber.StartFromCount - 1)

	return nil
}

func (salesReturn *SalesReturn) UnMakeRedisCode() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := salesReturn.StoreID.Hex() + "_return_invoice_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := salesReturn.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.SalesReturnSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := salesReturn.StoreID.Hex() + "_return_invoice_counter_" + monthKey

		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

/*
func (model *SalesReturn) MakeRedisCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_return_invoice_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetSalesReturnCount()
		if err != nil {
			return err
		}

		startFrom := store.SalesReturnSerialNumber.StartFromCount

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

	paddingCount := store.SalesReturnSerialNumber.PaddingCount
	if store.SalesReturnSerialNumber.Prefix != "" {
		model.Code = fmt.Sprintf("%s-%0*d", store.SalesReturnSerialNumber.Prefix, paddingCount, incr)
	} else {
		model.Code = fmt.Sprintf("%s%0*d", store.SalesReturnSerialNumber.Prefix, paddingCount, incr)
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

	model.InvoiceCountValue = incr - (store.SalesReturnSerialNumber.StartFromCount - 1)
	return nil
}

func (salesReturn *SalesReturn) UnMakeRedisCode() error {
	redisKey := salesReturn.StoreID.Hex() + "_return_invoice_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists != 0 {
		_, err := db.RedisClient.Decr(redisKey).Result()
		if err != nil {
			return err
		}
	}

	return nil
}
*/

func (salesReturn *SalesReturn) MakeCode() error {
	return salesReturn.MakeRedisCode()
}

func (salesReturn *SalesReturn) UnMakeCode() error {
	return salesReturn.UnMakeRedisCode()
}

func (store *Store) FindLastSalesReturnByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesReturn *SalesReturn, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"created_at": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&salesReturn)
	if err != nil {
		return nil, err
	}

	return salesReturn, err
}

func (salesReturn *SalesReturn) UpdateOrderReturnDiscount(salesReturnOld *SalesReturn) error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	order, err := store.FindOrderByID(salesReturn.OrderID, bson.M{})
	if err != nil {
		return err
	}

	if salesReturnOld != nil {
		order.ReturnDiscount -= salesReturnOld.Discount
	}

	order.ReturnDiscount += salesReturn.Discount
	order.ReturnDiscountWithVAT += salesReturn.DiscountWithVAT
	return order.Update()
}

func (salesReturn *SalesReturn) UpdateOrderReturnCount() (count int64, err error) {
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return 0, err
	}

	returnCount, err := collection.CountDocuments(ctx, bson.M{
		"order_id": salesReturn.OrderID,
		"deleted":  bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}

	order, err := store.FindOrderByID(salesReturn.OrderID, bson.M{})
	if err != nil {
		return 0, err
	}

	order.ReturnCount = returnCount
	err = order.Update()
	if err != nil {
		return 0, err
	}

	return returnCount, nil
}

/*
func (salesReturn *SalesReturn) UpdateOrderReturnAmount() (count int64, err error) {
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return 0, err
	}

	returnCount, err := collection.CountDocuments(ctx, bson.M{
		"order_id": salesReturn.OrderID,
		"deleted":  bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}

	order, err := store.FindOrderByID(salesReturn.OrderID, bson.M{})
	if err != nil {
		return 0, err
	}

	order.ReturnAmount = returnCount
	err = order.Update()
	if err != nil {
		return 0, err
	}

	return returnCount, nil
}
*/

func (salesReturn *SalesReturn) CloseSalesPayment() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if !store.Settings.EnableAutoPaymentCloseOnReturn {
		return nil
	}

	order, err := store.FindOrderByID(salesReturn.OrderID, bson.M{})
	if err != nil {
		return err
	}

	/*
		customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return err
		}

		if customer != nil {
			err = customer.CloseCustomerPendingSalesBySalesReturn(salesReturn)
			if err != nil {
				return err
			}
			return nil
		}*/

	amount := salesReturn.BalanceAmount

	if order.BalanceAmount < amount {
		amount = order.BalanceAmount
	}

	if order.PaymentStatus != "paid" && salesReturn.PaymentStatus != "paid" {
		newSalesPayment := SalesPayment{
			Date:          salesReturn.Date,
			OrderID:       &order.ID,
			OrderCode:     order.Code,
			Amount:        amount,
			Method:        "sales_return",
			CreatedAt:     salesReturn.CreatedAt,
			UpdatedAt:     salesReturn.UpdatedAt,
			StoreID:       salesReturn.StoreID,
			CreatedBy:     salesReturn.CreatedBy,
			UpdatedBy:     salesReturn.UpdatedBy,
			CreatedByName: salesReturn.CreatedByName,
			UpdatedByName: salesReturn.UpdatedByName,
			ReferenceType: "sales_return",
			ReferenceCode: salesReturn.Code,
			ReferenceID:   &salesReturn.ID,
		}
		err = newSalesPayment.Insert()
		if err != nil {
			return err
		}

		order.Payments = append(order.Payments, newSalesPayment)

		err = order.Update()
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

		//Sales Return
		newSalesReturnPayment := SalesReturnPayment{
			Date:            salesReturn.Date,
			SalesReturnID:   &salesReturn.ID,
			SalesReturnCode: salesReturn.Code,
			OrderID:         &order.ID,
			OrderCode:       order.Code,
			Amount:          amount,
			Method:          "sales",
			CreatedAt:       salesReturn.CreatedAt,
			UpdatedAt:       salesReturn.UpdatedAt,
			StoreID:         salesReturn.StoreID,
			CreatedBy:       salesReturn.CreatedBy,
			UpdatedBy:       salesReturn.UpdatedBy,
			CreatedByName:   salesReturn.CreatedByName,
			UpdatedByName:   salesReturn.UpdatedByName,
			ReferenceType:   "sales",
			ReferenceCode:   order.Code,
			ReferenceID:     &order.ID,
		}
		err = newSalesReturnPayment.Insert()
		if err != nil {
			return err
		}

		salesReturn.Payments = append(salesReturn.Payments, newSalesReturnPayment)

		err = salesReturn.Update()
		if err != nil {
			return err
		}

		_, err = salesReturn.SetPaymentStatus()
		if err != nil {
			return err
		}

		err = salesReturn.Update()
		if err != nil {
			return err
		}

	}

	return nil
}

func (customer *Customer) CloseCustomerPendingSalesBySalesReturn(salesReturn *SalesReturn) error {
	pendingSales, err := customer.GetPendingSales()
	if err != nil {
		return err
	}

	salesReturnBalanceAmount := salesReturn.BalanceAmount

	amountToSettle := float64(0.00)

	for _, pendingSale := range pendingSales {
		if pendingSale.BalanceAmount > salesReturnBalanceAmount {
			amountToSettle = salesReturnBalanceAmount
			salesReturnBalanceAmount = float64(0.00)
		} else {
			amountToSettle = pendingSale.BalanceAmount
			salesReturnBalanceAmount -= amountToSettle
		}

		//make payments and change payement status

		now := time.Now()
		//Add payment to sales
		newSalesPayment := SalesPayment{
			Date:          &now,
			OrderID:       &pendingSale.ID,
			OrderCode:     pendingSale.Code,
			Amount:        amountToSettle,
			Method:        "sales_return",
			CreatedAt:     &now,
			UpdatedAt:     &now,
			StoreID:       salesReturn.StoreID,
			CreatedBy:     salesReturn.UpdatedBy,
			UpdatedBy:     salesReturn.UpdatedBy,
			CreatedByName: salesReturn.UpdatedByName,
			UpdatedByName: salesReturn.UpdatedByName,
			ReferenceType: "sales_return",
			ReferenceCode: salesReturn.Code,
			ReferenceID:   &salesReturn.ID,
		}
		err = newSalesPayment.Insert()
		if err != nil {
			return err
		}

		pendingSale.Payments = append(pendingSale.Payments, newSalesPayment)

		err = pendingSale.Update()
		if err != nil {
			return err
		}

		_, err = pendingSale.SetPaymentStatus()
		if err != nil {
			return err
		}

		err = pendingSale.Update()
		if err != nil {
			return err
		}

		err = pendingSale.SetCustomerSalesStats()
		if err != nil {
			return err
		}

		//Add payment to purchase
		newSalesReturnPayment := SalesReturnPayment{
			Date:            &now,
			SalesReturnID:   &salesReturn.ID,
			SalesReturnCode: salesReturn.Code,
			Amount:          amountToSettle,
			Method:          "sales",
			CreatedAt:       &now,
			UpdatedAt:       &now,
			StoreID:         salesReturn.StoreID,
			CreatedBy:       salesReturn.UpdatedBy,
			UpdatedBy:       salesReturn.UpdatedBy,
			CreatedByName:   salesReturn.UpdatedByName,
			UpdatedByName:   salesReturn.UpdatedByName,
			ReferenceType:   "sales",
			ReferenceCode:   pendingSale.Code,
			ReferenceID:     &pendingSale.ID,
		}
		err = newSalesReturnPayment.Insert()
		if err != nil {
			return err
		}

		salesReturn.Payments = append(salesReturn.Payments, newSalesReturnPayment)

		err = salesReturn.Update()
		if err != nil {
			return err
		}

		_, err = salesReturn.SetPaymentStatus()
		if err != nil {
			return err
		}

		err = salesReturn.Update()
		if err != nil {
			return err
		}

		if salesReturnBalanceAmount > 0 {
			continue
		} else {
			break
		}
	}

	return nil
}

func (salesReturn *SalesReturn) UpdateOrderReturnCashDiscount(salesReturnOld *SalesReturn) error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	order, err := store.FindOrderByID(salesReturn.OrderID, bson.M{})
	if err != nil {
		return err
	}

	if salesReturnOld != nil {
		order.ReturnCashDiscount -= salesReturnOld.CashDiscount
	}

	order.ReturnCashDiscount += salesReturn.CashDiscount
	return order.Update()
}

func (salesreturn *SalesReturn) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + salesreturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if salesreturn.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": salesreturn.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": salesreturn.Code,
			"_id":  bson.M{"$ne": salesreturn.ID},
		})
	}

	return (count > 0), err
}

func GenerateSalesReturnCode(n int) string {
	//letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	letterRunes := []rune("0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (salesreturn *SalesReturn) UpdateSalesReturnStatus(status string) (*SalesReturn, error) {
	collection := db.GetDB("store_" + salesreturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": salesreturn.ID},
		bson.M{"$set": bson.M{"status": status}},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return salesreturn, nil
	}
	return nil, nil
}

func (salesreturn *SalesReturn) Update() error {
	collection := db.GetDB("store_" + salesreturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": salesreturn.ID},
		bson.M{"$set": salesreturn},
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

func (salesreturn *SalesReturn) DeleteSalesReturn(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + salesreturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = salesreturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	/*
		userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
		if err != nil {
			return err
		}
			salesreturn.Deleted = true
			salesreturn.DeletedBy = &userID
			now := time.Now()
			salesreturn.DeletedAt = &now
	*/

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": salesreturn.ID},
		bson.M{"$set": salesreturn},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindSalesReturnByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesreturn *SalesReturn, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
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
		Decode(&salesreturn)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "store")
		salesreturn.Store, _ = FindStoreByID(salesreturn.StoreID, fields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "customer")
		salesreturn.Customer, _ = store.FindCustomerByID(salesreturn.CustomerID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		salesreturn.CreatedByUser, _ = FindUserByID(salesreturn.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		salesreturn.UpdatedByUser, _ = FindUserByID(salesreturn.UpdatedBy, fields)
	}

	/*
		if _, ok := selectFields["deleted_by_user.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
			salesreturn.DeletedByUser, _ = FindUserByID(salesreturn.DeletedBy, fields)
		}
	*/

	return salesreturn, err
}

func (store *Store) IsSalesReturnExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (salesreturn *SalesReturn) HardDelete() (err error) {
	collection := db.GetDB("store_" + salesreturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.M{"_id": salesreturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func ProcessSalesReturns() error {
	log.Print("Processing sales returns")

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
		}, "salesreturn")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
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
			salesReturn := SalesReturn{}
			err = cur.Decode(&salesReturn)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if salesReturn.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			/*
				salesReturn.UpdateForeignLabelFields()
				salesReturn.ClearProductsHistory()
				salesReturn.ClearProductsSalesReturnHistory()
				salesReturn.CreateProductsHistory()
				salesReturn.CreateProductsSalesReturnHistory()
			*/

			salesReturn.UndoAccounting()
			salesReturn.DoAccounting()
			if salesReturn.CustomerID != nil && !salesReturn.CustomerID.IsZero() {
				customer, _ := store.FindCustomerByID(salesReturn.CustomerID, bson.M{})
				if customer != nil {
					customer.SetCreditBalance()
				}
			}

			//salesReturn.ClearProductsHistory()
			//salesReturn.CreateProductsHistory()

			/*
				if store.Code == "MBDI" || store.Code == "LGK" {
					salesReturn.ClearProductsSalesReturnHistory()
					salesReturn.CreateProductsSalesReturnHistory()
				}*/
			/*
				salesReturn.UndoAccounting()
				salesReturn.DoAccounting()

				if salesReturn.CustomerID != nil && !salesReturn.CustomerID.IsZero() {
					customer, _ := store.FindCustomerByID(salesReturn.CustomerID, bson.M{})
					if customer != nil {
						customer.SetCreditBalance()
					}
				}*/

			/*
				order, _ := store.FindOrderByID(salesReturn.OrderID, bson.M{})
				order.ReturnAmount, order.ReturnCount, _ = store.GetReturnedAmountByOrderID(*salesReturn.OrderID)
				order.Update()
			*/

			/*
				log.Print("Sales Return ID: " + salesReturn.Code)
				err = salesReturn.ReportToZatca()
				if err != nil {
					log.Print("Failed 1st time, trying 2nd time")

					if GetDecimalPoints(salesReturn.ShippingOrHandlingFees) > 2 {
						log.Print("Trimming shipping cost to 2 decimals")
						salesReturn.ShippingOrHandlingFees = RoundTo2Decimals(salesReturn.ShippingOrHandlingFees)
					}

					if GetDecimalPoints(salesReturn.Discount) > 2 {
						log.Print("Trimming discount to 2 decimals")
						salesReturn.Discount = RoundTo2Decimals(salesReturn.Discount)
					}

					salesReturn.FindNetTotal()
					salesReturn.Update()

					err = salesReturn.ReportToZatca()
					if err != nil {
						log.Print("Failed  2nd time. ")
						customer, _ := store.FindCustomerByID(salesReturn.CustomerID, bson.M{})
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
							err = salesReturn.ReportToZatca()
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
				err = salesReturn.Update()
				if err != nil {
					return errors.New("Error updating: " + err.Error())
				}
			*/

			bar.Add(1)
		}
	}
	log.Print("Sales Returns DONE!")
	return nil
}

func (salesReturn *SalesReturn) SetPaymentStatus() (payments []SalesReturnPayment, err error) {
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()
	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"sales_return_id": salesReturn.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return payments, errors.New("Error fetching sales return payment history" + err.Error())
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
		model := SalesReturnPayment{}
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

	salesReturn.TotalPaymentPaid = RoundTo2Decimals(totalPaymentPaid)
	//salesReturn.BalanceAmount = ToFixed(salesReturn.NetTotal-totalPaymentPaid, 2)
	salesReturn.BalanceAmount = RoundTo2Decimals((salesReturn.NetTotal - salesReturn.CashDiscount) - totalPaymentPaid)
	salesReturn.PaymentMethods = paymentMethods
	salesReturn.Payments = payments
	salesReturn.PaymentsCount = int64(len(payments))

	if RoundTo2Decimals((salesReturn.NetTotal - salesReturn.CashDiscount)) <= totalPaymentPaid {
		salesReturn.PaymentStatus = "paid"
	} else if totalPaymentPaid > 0 {
		salesReturn.PaymentStatus = "paid_partially"
	} else if totalPaymentPaid <= 0 {
		salesReturn.PaymentStatus = "not_paid"
	}

	return payments, err
}

func (salesReturn *SalesReturn) RemoveStock() (err error) {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(salesReturn.Products) == 0 {
		return nil
	}

	for _, salesReturnProduct := range salesReturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&salesReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.ProductStores) == 0 {
			store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
			if err != nil {
				return err
			}

			product.ProductStores = map[string]ProductStore{}

			product.ProductStores[salesReturn.StoreID.Hex()] = ProductStore{
				StoreID:           *salesReturn.StoreID,
				StoreName:         salesReturn.StoreName,
				StoreNameInArabic: store.NameInArabic,
				Stock:             float64(0),
			}
		}

		if productStoreTemp, ok := product.ProductStores[salesReturn.StoreID.Hex()]; ok {
			productStoreTemp.Stock -= (salesReturnProduct.Quantity)
			product.ProductStores[salesReturn.StoreID.Hex()] = productStoreTemp
		}
		/*
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == salesReturn.StoreID.Hex() {
					product.Stores[k].Stock -= (salesReturnProduct.Quantity)
					break
				}
			}
		*/

		err = product.Update(nil)
		if err != nil {
			return err
		}

	}

	err = salesReturn.Update()
	if err != nil {
		return err
	}
	return nil
}

func (salesReturn *SalesReturn) ClearPayments() error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("sales_return_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"sales_return_id": salesReturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func (salesReturn *SalesReturn) GetPaymentsCount() (count int64, err error) {
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("sales_return_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"sales_return_id": salesReturn.ID,
		"deleted":         bson.M{"$ne": true},
	})
}

type ProductSalesReturnStats struct {
	SalesReturnCount    int64   `json:"sales_return_count" bson:"sales_return_count"`
	SalesReturnQuantity float64 `json:"sales_return_quantity" bson:"sales_return_quantity"`
	SalesReturn         float64 `json:"sales_return" bson:"sales_return"`
	SalesReturnProfit   float64 `json:"sales_return_profit" bson:"sales_return_profit"`
	SalesReturnLoss     float64 `json:"sales_return_loss" bson:"sales_return_loss"`
}

func (product *Product) SetProductSalesReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductSalesReturnStats

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
				"_id":                   nil,
				"sales_return_count":    bson.M{"$sum": 1},
				"sales_return_quantity": bson.M{"$sum": "$quantity"},
				"sales_return":          bson.M{"$sum": "$net_price"},
				"sales_return_profit":   bson.M{"$sum": "$profit"},
				"sales_return_loss":     bson.M{"$sum": "$loss"},
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

		stats.SalesReturn = RoundFloat(stats.SalesReturn, 2)
		stats.SalesReturnProfit = RoundFloat(stats.SalesReturnProfit, 2)
		stats.SalesReturnLoss = RoundFloat(stats.SalesReturnLoss, 2)
	}

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.SalesReturnCount = stats.SalesReturnCount
		productStoreTemp.SalesReturnQuantity = stats.SalesReturnQuantity
		productStoreTemp.SalesReturn = stats.SalesReturn
		productStoreTemp.SalesReturnProfit = stats.SalesReturnProfit
		productStoreTemp.SalesReturnLoss = stats.SalesReturnLoss
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	/*
		for storeIndex, store := range product.ProductStores {
			if store.StoreID.Hex() == storeID.Hex() {
				product.Stores[storeIndex].SalesReturnCount = stats.SalesReturnCount
				product.Stores[storeIndex].SalesReturnQuantity = stats.SalesReturnQuantity
				product.Stores[storeIndex].SalesReturn = stats.SalesReturn
				product.Stores[storeIndex].SalesReturnProfit = stats.SalesReturnProfit
				product.Stores[storeIndex].SalesReturnLoss = stats.SalesReturnLoss
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

func (product *Product) SetProductSalesReturnQuantityByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductSalesReturnStats

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
				"_id":                   nil,
				"sales_return_quantity": bson.M{"$sum": "$quantity"},
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
		productStoreTemp.SalesReturnQuantity = stats.SalesReturnQuantity
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (salesReturn *SalesReturn) SetProductsSalesReturnStats() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, salesReturnProduct := range salesReturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&salesReturnProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductSalesReturnStatsByStoreID(*salesReturn.StoreID)
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

				err = setProductObj.SetProductSalesReturnStatsByStoreID(store.ID)
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
type CustomerSalesReturnStats struct {
	SalesReturnCount              int64   `json:"sales_return_count" bson:"sales_return_count"`
	SalesReturnAmount             float64 `json:"sales_return_amount" bson:"sales_return_amount"`
	SalesReturnPaidAmount         float64 `json:"sales_return_paid_amount" bson:"sales_return_paid_amount"`
	SalesReturnBalanceAmount      float64 `json:"sales_return_balance_amount" bson:"sales_return_balance_amount"`
	SalesReturnProfit             float64 `json:"sales_return_profit" bson:"sales_return_profit"`
	SalesReturnLoss               float64 `json:"sales_return_loss" bson:"sales_return_loss"`
	SalesReturnPaidCount          int64   `json:"sales_return_paid_count" bson:"sales_return_paid_count"`
	SalesReturnNotPaidCount       int64   `json:"sales_return_not_paid_count" bson:"sales_return_not_paid_count"`
	SalesReturnPaidPartiallyCount int64   `json:"sales_return_paid_partially_count" bson:"sales_return_paid_partially_count"`
}

func (customer *Customer) SetCustomerSalesReturnStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerSalesReturnStats

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
				"_id":                         nil,
				"sales_return_count":          bson.M{"$sum": 1},
				"sales_return_amount":         bson.M{"$sum": "$net_total"},
				"sales_return_paid_amount":    bson.M{"$sum": "$total_payment_paid"},
				"sales_return_balance_amount": bson.M{"$sum": "$balance_amount"},
				"sales_return_profit":         bson.M{"$sum": "$net_profit"},
				"sales_return_loss":           bson.M{"$sum": "$loss"},
				"sales_return_paid_count": bson.M{"$sum": bson.M{
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
				"sales_return_not_paid_count": bson.M{"$sum": bson.M{
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
				"sales_return_paid_partially_count": bson.M{"$sum": bson.M{
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
		stats.SalesReturnAmount = RoundFloat(stats.SalesReturnAmount, 2)
		stats.SalesReturnPaidAmount = RoundFloat(stats.SalesReturnPaidAmount, 2)
		stats.SalesReturnBalanceAmount = RoundFloat(stats.SalesReturnBalanceAmount, 2)
		stats.SalesReturnProfit = RoundFloat(stats.SalesReturnProfit, 2)
		stats.SalesReturnLoss = RoundFloat(stats.SalesReturnLoss, 2)
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
		customerStore.SalesReturnCount = stats.SalesReturnCount
		customerStore.SalesReturnAmount = stats.SalesReturnAmount
		customerStore.SalesReturnPaidAmount = stats.SalesReturnPaidAmount
		customerStore.SalesReturnBalanceAmount = stats.SalesReturnBalanceAmount
		customerStore.SalesReturnProfit = stats.SalesReturnProfit
		customerStore.SalesReturnLoss = stats.SalesReturnLoss
		customerStore.SalesReturnPaidCount = stats.SalesReturnPaidCount
		customerStore.SalesReturnNotPaidCount = stats.SalesReturnNotPaidCount
		customerStore.SalesReturnPaidPartiallyCount = stats.SalesReturnPaidPartiallyCount
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:                       storeID,
			StoreName:                     store.Name,
			StoreNameInArabic:             store.NameInArabic,
			SalesReturnCount:              stats.SalesReturnCount,
			SalesReturnAmount:             stats.SalesReturnAmount,
			SalesReturnPaidAmount:         stats.SalesReturnPaidAmount,
			SalesReturnBalanceAmount:      stats.SalesReturnBalanceAmount,
			SalesReturnProfit:             stats.SalesReturnProfit,
			SalesReturnLoss:               stats.SalesReturnLoss,
			SalesReturnPaidCount:          stats.SalesReturnPaidCount,
			SalesReturnNotPaidCount:       stats.SalesReturnNotPaidCount,
			SalesReturnPaidPartiallyCount: stats.SalesReturnPaidPartiallyCount,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (salesReturn *SalesReturn) SetCustomerSalesReturnStats() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(salesReturn.CustomerID, map[string]interface{}{})
	if err != nil {
		return err
	}

	err = customer.SetCustomerSalesReturnStatsByStoreID(*salesReturn.StoreID)
	if err != nil {
		return err
	}

	return nil
}

// Accounting
// Journal entries
func MakeJournalsForUnpaidSalesReturn(
	salesReturn *SalesReturn,
	customerAccount *Account,
	salesReturnAccount *Account,
	cashDiscountReceivedAccount *Account,
) []Journal {
	now := time.Now()

	groupID := primitive.NewObjectID()

	journals := []Journal{}

	balanceAmount := RoundFloat((salesReturn.NetTotal - salesReturn.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          salesReturn.Date,
		AccountID:     salesReturnAccount.ID,
		AccountNumber: salesReturnAccount.Number,
		AccountName:   salesReturnAccount.Name,
		DebitOrCredit: "debit",
		Debit:         salesReturn.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if salesReturn.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          salesReturn.Date,
			AccountID:     cashDiscountReceivedAccount.ID,
			AccountNumber: cashDiscountReceivedAccount.Number,
			AccountName:   cashDiscountReceivedAccount.Name,
			DebitOrCredit: "credit",
			Credit:        salesReturn.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          salesReturn.Date,
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

var totalSalesReturnPaidAmount float64
var extraSalesReturnAmountPaid float64
var extraSalesReturnPayments []SalesReturnPayment

func MakeJournalsForSalesReturnPaymentsByDatetime(
	salesReturn *SalesReturn,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	salesReturnAccount *Account,
	payments []SalesReturnPayment,
	cashDiscountReceivedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
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
	totalSalesReturnPaidAmountTemp := totalSalesReturnPaidAmount
	extraSalesReturnAmountPaidTemp := extraSalesReturnAmountPaid

	for _, payment := range payments {
		totalSalesReturnPaidAmount += payment.Amount
		if totalSalesReturnPaidAmount > (salesReturn.NetTotal - salesReturn.CashDiscount) {
			extraSalesReturnAmountPaid = RoundFloat((totalSalesReturnPaidAmount - (salesReturn.NetTotal - salesReturn.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraSalesReturnAmountPaid > 0 {
			skip := false
			if extraSalesReturnAmountPaid < payment.Amount {
				amount = RoundFloat((payment.Amount - extraSalesReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraSalesReturnAmountPaid = 0
			} else if extraSalesReturnAmountPaid >= payment.Amount {
				skip = true
				extraSalesReturnAmountPaid = RoundFloat((extraSalesReturnAmountPaid - payment.Amount), 2)
			}

			if skip {
				continue
			}

		}
		totalPayment += amount
	} //end for

	totalSalesReturnPaidAmount = totalSalesReturnPaidAmountTemp
	extraSalesReturnAmountPaid = extraSalesReturnAmountPaidTemp
	//Don't touch
	//Debits
	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(salesReturn.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          salesReturn.Date,
			AccountID:     salesReturnAccount.ID,
			AccountNumber: salesReturnAccount.Number,
			AccountName:   salesReturnAccount.Name,
			DebitOrCredit: "debit",
			Debit:         salesReturn.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(salesReturn.Date, firstPaymentDate) {
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
			salesReturn.StoreID,
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
		totalSalesReturnPaidAmount += payment.Amount
		if totalSalesReturnPaidAmount > (salesReturn.NetTotal - salesReturn.CashDiscount) {
			extraSalesReturnAmountPaid = RoundFloat((totalSalesReturnPaidAmount - (salesReturn.NetTotal - salesReturn.CashDiscount)), 2)
		}
		amount := payment.Amount

		if extraSalesReturnAmountPaid > 0 {
			skip := false
			if extraSalesReturnAmountPaid < payment.Amount {
				extraAmount := extraSalesReturnAmountPaid
				extraSalesReturnPayments = append(extraSalesReturnPayments, SalesReturnPayment{
					Date:   payment.Date,
					Amount: extraAmount,
					Method: payment.Method,
				})
				amount = RoundFloat((payment.Amount - extraSalesReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraSalesReturnAmountPaid = 0
			} else if extraSalesReturnAmountPaid >= payment.Amount {
				extraSalesReturnPayments = append(extraSalesReturnPayments, SalesReturnPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})

				skip = true
				extraSalesReturnAmountPaid = RoundFloat((extraSalesReturnAmountPaid - payment.Amount), 2)
			}

			if skip {
				continue
			}

		}

		cashPayingAccount := Account{}
		if payment.ReferenceType == "customer_withdrawal" || payment.ReferenceType == "sales" {
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
					salesReturn.StoreID,
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

	if salesReturn.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(salesReturn.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          salesReturn.Date,
			AccountID:     cashDiscountReceivedAccount.ID,
			AccountNumber: cashDiscountReceivedAccount.Number,
			AccountName:   cashDiscountReceivedAccount.Name,
			DebitOrCredit: "credit",
			Credit:        salesReturn.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	balanceAmount := RoundFloat(((salesReturn.NetTotal - salesReturn.CashDiscount) - totalPayment), 2)

	//Asset or debt increased
	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(salesReturn.Date, firstPaymentDate) {
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
			salesReturn.StoreID,
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
			Date:          salesReturn.Date,
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

func MakeJournalsForSalesReturnExtraPayments(
	salesReturn *SalesReturn,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []SalesReturnPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
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
		salesReturn.StoreID,
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
		Debit:         salesReturn.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	for _, payment := range extraPayments {
		cashPayingAccount := Account{}
		if payment.ReferenceType == "customer_withdrawal" || payment.ReferenceType == "sales" {
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
					salesReturn.StoreID,
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

// Regroup sales payments by datetime

func RegroupSalesReturnPaymentsByDatetime(payments []SalesReturnPayment) [][]SalesReturnPayment {
	paymentsByDatetime := map[string][]SalesReturnPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
		//log.Print(*payment.Amount)
	}

	paymentsByDatetime2 := [][]SalesReturnPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

//End customer account journals

func (salesReturn *SalesReturn) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var customer *Customer

	if salesReturn.CustomerID != nil && !salesReturn.CustomerID.IsZero() {
		customer, err = store.FindCustomerByID(salesReturn.CustomerID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}

	cashAccount, err := store.CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	salesReturnAccount, err := store.CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Sales Return", nil, nil)
	if err != nil {
		return nil, err
	}

	cashDiscountReceivedAccount, err := store.CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Cash discount received", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(salesReturn.Payments) > 0 {
		firstPaymentDate = salesReturn.Payments[0].Date
	}

	if len(salesReturn.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(firstPaymentDate, salesReturn.Date)) {
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
			salesReturn.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			customerPhone,
			customerVATNo,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidSalesReturn(
			salesReturn,
			customerAccount,
			salesReturnAccount,
			cashDiscountReceivedAccount,
		)...)
	}

	if len(salesReturn.Payments) > 0 {
		totalSalesReturnPaidAmount = float64(0.00)
		extraSalesReturnAmountPaid = float64(0.00)
		extraSalesReturnPayments = []SalesReturnPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupSalesReturnPaymentsByDatetime(salesReturn.Payments)
		//fmt.Printf("%+v", paymentsByDatetime)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForSalesReturnPaymentsByDatetime(
				salesReturn,
				customer,
				cashAccount,
				bankAccount,
				salesReturnAccount,
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

		if salesReturn.BalanceAmount < 0 && len(extraSalesReturnPayments) > 0 {
			newJournals, err := MakeJournalsForSalesReturnExtraPayments(
				salesReturn,
				customer,
				cashAccount,
				bankAccount,
				extraSalesReturnPayments,
			)
			if err != nil {
				return nil, err
			}

			journals = append(journals, newJournals...)
		}

		totalSalesReturnPaidAmount = float64(0.00)
		extraSalesReturnAmountPaid = float64(0.00)

	}

	ledger = &Ledger{
		StoreID:        salesReturn.StoreID,
		ReferenceID:    salesReturn.ID,
		ReferenceModel: "sales_return",
		ReferenceCode:  salesReturn.Code,
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

func (salesReturn *SalesReturn) DoAccounting() error {
	err := salesReturn.AdjustPayments()
	if err != nil {
		return errors.New("error adjusting payments: " + err.Error())
	}

	ledger, err := salesReturn.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (salesReturn *SalesReturn) GetPayments() (models []SalesReturnPayment, err error) {
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("sales_return_payment")
	ctx := context.Background()
	findOptions := options.Find()

	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"sales_return_id": salesReturn.ID, "deleted": bson.M{"$ne": true}}, findOptions)
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
		model := SalesReturnPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		models = append(models, model)

	} //end for loop

	return models, nil
}

func (salesReturn *SalesReturn) AdjustPayments() error {
	if len(salesReturn.Payments) == 0 || salesReturn.Date == nil {
		return nil
	}

	// 1. Ensure first payment is at least 1 minute after salesReturn.Date if they are the same
	firstPayment := salesReturn.Payments[0]
	if firstPayment.Date != nil && firstPayment.Date.Equal(*salesReturn.Date) {
		newTime := salesReturn.Date.Add(1 * time.Minute)
		salesReturn.Payments[0].Date = &newTime
	}

	// 2. For each subsequent payment, ensure strictly increasing by at least 1 minute
	for i := 1; i < len(salesReturn.Payments); i++ {
		prev := salesReturn.Payments[i-1].Date
		curr := salesReturn.Payments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			salesReturn.Payments[i].Date = &newTime
		}
	}

	salesReturnPayments, err := salesReturn.GetPayments()
	if err != nil {
		return err
	}

	for i := 1; i < len(salesReturnPayments); i++ {
		prev := salesReturnPayments[i-1].Date
		curr := salesReturnPayments[i].Date
		if prev != nil && curr != nil && (curr.Equal(*prev) || curr.Before(*prev)) {
			newTime := prev.Add(1 * time.Minute)
			salesReturnPayments[i].Date = &newTime
			err = salesReturnPayments[i].Update()
			if err != nil {
				return err
			}
		}
	}

	err = salesReturn.Update()
	if err != nil {
		return err
	}

	return nil
}

func (salesReturn *SalesReturn) UndoAccounting() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(salesReturn.ID, *salesReturn.StoreID, bson.M{})
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

	err = store.RemoveLedgerByReferenceID(salesReturn.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(salesReturn.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

func (salesReturn *SalesReturn) FindPreviousSalesReturn(selectFields map[string]interface{}) (previousSalesReturn *SalesReturn, err error) {
	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"invoice_count_value": (salesReturn.InvoiceCountValue - 1),
			"store_id":            salesReturn.StoreID,
		}, findOneOptions).
		Decode(&previousSalesReturn)
	if err != nil {
		return nil, err
	}

	return previousSalesReturn, err
}

func (salesReturn *SalesReturn) ValidateZatcaReporting() (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	customer, err := store.FindCustomerByID(salesReturn.CustomerID, bson.M{})
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

		if !govalidator.IsNull(customer.NationalAddress.BuildingNo) && !IsValidDigitNumber(customer.NationalAddress.BuildingNo, "4") {
			customerErrorMessages = append(customerErrorMessages, "Building number should be 4 digits")
		}

		if !govalidator.IsNull(customer.NationalAddress.ZipCode) && !IsValidDigitNumber(customer.NationalAddress.ZipCode, "5") {
			customerErrorMessages = append(customerErrorMessages, "Zip code should be 5 digits")
		}

		if len(customerErrorMessages) > 0 {
			errs["customer_id"] = "Fix the customer errors: " + strings.Join(customerErrorMessages, ",")
		}
	}

	return errs
}
