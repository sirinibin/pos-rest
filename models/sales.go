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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
	"gopkg.in/mgo.v2/bson"
)

type OrderProduct struct {
	ProductID           primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic        string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode            string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PrefixPartNumber    string             `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber          string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity            float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	QuantityReturned    float64            `json:"quantity_returned" bson:"quantity_returned"`
	UnitPrice           float64            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	PurchaseUnitPrice   float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	Unit                string             `bson:"unit,omitempty" json:"unit,omitempty"`
	Discount            float64            `bson:"discount" json:"discount"`
	DiscountPercent     float64            `bson:"discount_percent" json:"discount_percent"`
	UnitDiscount        float64            `bson:"unit_discount" json:"unit_discount"`
	UnitDiscountPercent float64            `bson:"unit_discount_percent" json:"unit_discount_percent"`
	Profit              float64            `bson:"profit" json:"profit"`
	Loss                float64            `bson:"loss" json:"loss"`
}

// Order : Order structure
type Order struct {
	ID                     primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                   *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                string              `json:"date_str,omitempty" bson:"-"`
	InvoiceCountValue      int64               `bson:"invoice_count_value,omitempty" json:"invoice_count_value,omitempty"`
	Code                   string              `bson:"code,omitempty" json:"code,omitempty"`
	UUID                   string              `bson:"uuid,omitempty" json:"uuid,omitempty"`
	Hash                   string              `bson:"hash,omitempty" json:"hash,omitempty"`
	PrevHash               string              `bson:"prev_hash,omitempty" json:"prev_hash,omitempty"`
	StoreID                *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID             *primitive.ObjectID `json:"customer_id" bson:"customer_id"`
	Store                  *Store              `json:"store,omitempty"`
	Customer               *Customer           `json:"customer" bson:"-"`
	Products               []OrderProduct      `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy            *primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredByUser        *User               `json:"delivered_by_user,omitempty"`
	VatPercent             *float64            `bson:"vat_percent" json:"vat_percent"`
	Discount               float64             `bson:"discount" json:"discount"`
	DiscountPercent        float64             `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent      bool                `bson:"is_discount_percent" json:"is_discount_percent"`
	ReturnDiscount         float64             `bson:"return_discount" json:"return_discount"`
	Status                 string              `bson:"status,omitempty" json:"status,omitempty"`
	ShippingOrHandlingFees float64             `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	TotalQuantity          float64             `bson:"total_quantity" json:"total_quantity"`
	VatPrice               float64             `bson:"vat_price" json:"vat_price"`
	Total                  float64             `bson:"total" json:"total"`
	NetTotal               float64             `bson:"net_total" json:"net_total"`
	CashDiscount           float64             `bson:"cash_discount" json:"cash_discount"`
	ReturnCashDiscount     float64             `bson:"return_cash_discount" json:"return_cash_discount"`
	PaymentMethods         []string            `json:"payment_methods" bson:"payment_methods"`
	TotalPaymentReceived   float64             `bson:"total_payment_received" json:"total_payment_received"`
	BalanceAmount          float64             `bson:"balance_amount" json:"balance_amount"`
	Payments               []SalesPayment      `bson:"payments" json:"payments"`
	PaymentsInput          []SalesPayment      `bson:"-" json:"payments_input"`
	PaymentsCount          int64               `bson:"payments_count" json:"payments_count"`
	PaymentStatus          string              `bson:"payment_status" json:"payment_status"`
	Profit                 float64             `bson:"profit" json:"profit"`
	NetProfit              float64             `bson:"net_profit" json:"net_profit"`
	Loss                   float64             `bson:"loss" json:"loss"`
	NetLoss                float64             `bson:"net_loss" json:"net_loss"`
	ReturnCount            int64               `bson:"return_count" json:"return_count"`
	ReturnAmount           float64             `bson:"return_amount" json:"return_amount"`
	CreatedAt              *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt              *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy              *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy              *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser          *User               `json:"created_by_user,omitempty"`
	UpdatedByUser          *User               `json:"updated_by_user,omitempty"`
	DeliveredByName        string              `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CustomerName           string              `json:"customer_name" bson:"customer_name"`
	StoreName              string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName          string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName          string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	Zatca                  ZatcaReporting      `bson:"zatca,omitempty" json:"zatca,omitempty"`
	Remarks                string              `bson:"remarks" json:"remarks"`
	Phone                  string              `bson:"phone" json:"phone"`
	VatNo                  string              `bson:"vat_no" json:"vat_no"`
	Address                string              `bson:"address" json:"address"`
	EnableReportToZatca    bool                `json:"enable_report_to_zatca" bson:"-"`
}

type ZatcaReporting struct {
	IsSimplified                       bool       `bson:"is_simplified" json:"is_simplified"`
	CompliancePassed                   bool       `bson:"compliance_passed" json:"compliance_passed"`
	CompliancePassedAt                 *time.Time `bson:"compliance_passed_at,omitempty" json:"compliance_passed_at,omitempty"`
	ComplianceInvoiceHash              string     `bson:"compliance_invoice_hash,omitempty" json:"compliance_invoice_hash,omitempty"`
	ReportingPassed                    bool       `bson:"reporting_passed" json:"reporting_passed"`
	ReportedAt                         *time.Time `bson:"reporting_passed_at,omitempty" json:"reporting_passed_at,omitempty"`
	ReportingInvoiceHash               string     `bson:"reporting_invoice_hash,omitempty" json:"reporting_invoice_hash,omitempty"`
	QrCode                             string     `bson:"qr_code,omitempty" json:"qr_code,omitempty"`
	ECDSASignature                     string     `bson:"ecdsa_signature,omitempty" json:"ecdsa_signature,omitempty"`
	X509DigitalCertificate             string     `bson:"x509_digital_certificate,omitempty" json:"x509_digital_certificate,omitempty"`
	SigningTime                        *time.Time `bson:"signing_time,omitempty" json:"signing_time,omitempty"`
	SigningCertificateHash             string     `bson:"signing_certificate_hash,omitempty" json:"signing_certificate_hash,omitempty"`
	X509DigitalCertificateIssuerName   string     `bson:"x509_digital_certificate_issuer_name,omitempty" json:"x509_digital_certificate_issuer_name,omitempty"`
	X509DigitalCertificateSerialNumber string     `bson:"x509_digital_certificate_serial_number,omitempty" json:"x509_digital_certificate_serial_number,omitempty"`
	XadesSignedPropertiesHash          string     `bson:"xades_signed_properties_hash,omitempty" json:"xades_signed_properties_hash,omitempty"`
	ComplianceCheckFailedCount         int64      `bson:"compliance_check_failed_count,omitempty" json:"compliance_check_failed_count,omitempty"`
	ComplianceCheckErrors              []string   `bson:"compliance_check_errors,omitempty" json:"compliance_check_errors,omitempty"`
	ComplianceCheckLastFailedAt        *time.Time `bson:"compliance_check_last_failed_at,omitempty" json:"compliance_check_last_failed_at,omitempty"`
	ReportingFailedCount               int64      `bson:"reporting_failed_count,omitempty" json:"reporting_failed_count,omitempty"`
	ReportingErrors                    []string   `bson:"reporting_errors,omitempty" json:"reporting_errors,omitempty"`
	ReportingLastFailedAt              *time.Time `bson:"reporting_last_failed_at,omitempty" json:"reporting_last_failed_at,omitempty"`
}

func (store *Store) GetReturnedAmountByOrderID(orderID primitive.ObjectID) (returnedAmount float64, returnCount int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats SalesReturnStats

	pipeline := []bson.M{
		bson.M{
			"$match": map[string]interface{}{
				"order_id": orderID,
			},
		},
		bson.M{
			"$group": bson.M{
				"_id":                nil,
				"sales_return_count": bson.M{"$sum": 1},
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
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats.PaidSalesReturn, stats.SalesReturnCount, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return stats.PaidSalesReturn, stats.SalesReturnCount, err
		}
		stats.PaidSalesReturn = RoundFloat(stats.PaidSalesReturn, 2)
	}

	return stats.PaidSalesReturn, stats.SalesReturnCount, nil
}

func (store *Store) UpdateOrderProfit() error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
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
		order := Order{}
		err = cur.Decode(&order)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = order.CalculateOrderProfit()
		if err != nil {
			return err
		}
		err = order.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

func (order *Order) AttributesValueChangeEvent(orderOld *Order) error {

	if order.Status != orderOld.Status {
		/*
			order.SetChangeLog(
				"attribute_value_change",
				"status",
				orderOld.Status,
				order.Status,
			)
		*/

		//if order.Status == "delivered" || order.Status == "dispatched" {
		/*
			err := orderOld.AddStock()
			if err != nil {
				return err
			}

			err = order.RemoveStock()
			if err != nil {
				return err
			}
		*/
		//}
	}

	return nil
}

func (order *Order) GetSalesReturns() (salesReturns []SalesReturn, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("salesreturn")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"order_id": order.ID}, findOptions)
	if err != nil {
		return salesReturns, errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return salesReturns, errors.New("Cursor error:" + err.Error())
		}
		salesReturn := SalesReturn{}
		err = cur.Decode(&salesReturn)
		if err != nil {
			return salesReturns, errors.New("Cursor decode error:" + err.Error())
		}

		salesReturns = append(salesReturns, salesReturn)
	}

	return salesReturns, nil
}

func (order *Order) UpdateSalesReturnCustomer() error {
	salesReturns, err := order.GetSalesReturns()
	if err != nil {
		return err
	}

	for _, salesReturn := range salesReturns {
		salesReturn.CustomerID = order.CustomerID
		salesReturn.CustomerName = order.CustomerName
		err = salesReturn.Update()
		if err != nil {
			return err
		}
	}
	return nil
}

func (order *Order) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if order.StoreID != nil {
		store, err := FindStoreByID(order.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("error finding store: " + err.Error())
		}
		order.StoreName = store.Name
	}

	if order.CustomerID != nil && !order.CustomerID.IsZero() {
		customer, err := store.FindCustomerByID(order.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("error finding customer: " + err.Error())
		}
		if customer != nil {
			order.CustomerName = customer.Name
		}
	} else {
		order.CustomerName = ""
	}

	if order.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(order.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("error finding delivered by user: " + err.Error())
		}
		order.DeliveredByName = deliveredByUser.Name
	}

	/*
		if order.DeliveredBySignatureID != nil {
			deliveredBySignature, err := FindSignatureByID(order.DeliveredBySignatureID, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			order.DeliveredBySignatureName = deliveredBySignature.Name
		}
	*/

	if order.CreatedBy != nil {
		createdByUser, err := FindUserByID(order.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.CreatedByName = createdByUser.Name
	}

	if order.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(order.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.UpdatedByName = updatedByUser.Name
	}

	/*
		if order.DeletedBy != nil && !order.DeletedBy.IsZero() {
			deletedByUser, err := FindUserByID(order.DeletedBy, bson.M{"id": 1, "name": 1})
			if err != nil {
				return err
			}
			order.DeletedByName = deletedByUser.Name
		}
	*/

	for i, product := range order.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1})
		if err != nil {
			return err
		}
		//order.Products[i].Name = productObject.Name
		order.Products[i].NameInArabic = productObject.NameInArabic
		order.Products[i].ItemCode = productObject.ItemCode
		order.Products[i].PartNumber = productObject.PartNumber
		order.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

func (order *Order) FindNetTotal() {
	netTotal := float64(0.0)
	order.FindTotal()
	netTotal = order.Total

	order.ShippingOrHandlingFees = RoundTo2Decimals(order.ShippingOrHandlingFees)
	order.Discount = RoundTo2Decimals(order.Discount)

	netTotal += order.ShippingOrHandlingFees
	netTotal -= order.Discount

	order.FindVatPrice()
	netTotal += order.VatPrice

	order.NetTotal = RoundTo2Decimals(netTotal)
	order.CalculateDiscountPercentage()
}
func (order *Order) CalculateDiscountPercentage() {
	if order.NetTotal == 0 {
		order.DiscountPercent = 0
	}

	if order.Discount <= 0 {
		order.DiscountPercent = 0.00
		return
	}

	percentage := (order.Discount / order.NetTotal) * 100
	order.DiscountPercent = RoundTo2Decimals(percentage) // Use rounding here
}

func (order *Order) FindTotal() {
	total := float64(0.0)
	for i, product := range order.Products {
		order.Products[i].UnitPrice = RoundTo2Decimals(product.UnitPrice)
		order.Products[i].UnitDiscount = RoundTo2Decimals(product.UnitDiscount)

		if order.Products[i].UnitDiscount > 0 {
			order.Products[i].UnitDiscountPercent = RoundTo2Decimals((order.Products[i].UnitDiscount / order.Products[i].UnitPrice) * 100)
		}

		total += RoundTo2Decimals(product.Quantity * (order.Products[i].UnitPrice - order.Products[i].UnitDiscount))
	}

	order.Total = RoundTo2Decimals(total)
}

func (order *Order) FindVatPrice() {
	vatPrice := ((*order.VatPercent / float64(100.00)) * ((order.Total + order.ShippingOrHandlingFees) - order.Discount))
	order.VatPrice = RoundTo2Decimals(vatPrice)
}

func (order *Order) FindTotalQuantity() {
	totalQuantity := float64(0.0)
	for _, product := range order.Products {
		totalQuantity += product.Quantity
	}
	order.TotalQuantity = totalQuantity
}

type SalesStats struct {
	ID                     *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal               float64             `json:"net_total" bson:"net_total"`
	NetProfit              float64             `json:"net_profit" bson:"net_profit"`
	NetLoss                float64             `json:"net_loss" bson:"net_loss"`
	VatPrice               float64             `json:"vat_price" bson:"vat_price"`
	Discount               float64             `json:"discount" bson:"discount"`
	ShippingOrHandlingFees float64             `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
	PaidSales              float64             `json:"paid_sales" bson:"paid_sales"`
	UnPaidSales            float64             `json:"unpaid_sales" bson:"unpaid_sales"`
	CashSales              float64             `json:"cash_sales" bson:"cash_sales"`
	BankAccountSales       float64             `json:"bank_account_sales" bson:"bank_account_sales"`
	CashDiscount           float64             `json:"cash_discount" bson:"cash_discount"`
	ReturnCount            int64               `json:"return_count" bson:"return_count"`
	ReturnAmount           float64             `json:"return_amount" bson:"return_amount"`
}

func (store *Store) GetSalesStats(filter map[string]interface{}) (stats SalesStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
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
				"net_profit":             bson.M{"$sum": "$net_profit"},
				"net_loss":               bson.M{"$sum": "$net_loss"},
				"vat_price":              bson.M{"$sum": "$vat_price"},
				"discount":               bson.M{"$sum": "$discount"},
				"cash_discount":          bson.M{"$sum": "$cash_discount"},
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
				"return_count":           bson.M{"$sum": "$return_count"},
				"return_amount":          bson.M{"$sum": "$return_amount"},
				"paid_sales": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"unpaid_sales": bson.M{"$sum": "$balance_amount"},
				"cash_sales": bson.M{"$sum": bson.M{"$sum": bson.M{
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
				"bank_account_sales": bson.M{"$sum": bson.M{"$sum": bson.M{
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

func (store *Store) GetAllOrders() (orders []Order, err error) {

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	//Fetch all device documents with (garbage:true AND (gc_processed:false if exist OR gc_processed not exist ))
	/* Note: Actual Record fetching will not happen here
	as it is using mongodb cursor and record fetching will
	start with we call cur.Next()
	*/
	filter := make(map[string]interface{})
	cur, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return orders, errors.New("Error fetching orders:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return orders, errors.New("Cursor error:" + err.Error())
		}
		order := Order{}
		err = cur.Decode(&order)
		if err != nil {
			return orders, errors.New("Cursor decode error:" + err.Error())
		}
		orders = append(orders, order)
	} //end for loop

	return orders, nil
}

func (store *Store) SearchOrder(w http.ResponseWriter, r *http.Request) (orders []Order, criterias SearchCriterias, err error) {

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

	/*
		keys, ok = r.URL.Query()["search[zatca.reporting_passed]"]
		if ok && len(keys[0]) >= 1 {
			value, err := strconv.ParseInt(keys[0], 10, 64)
			if err != nil {
				return orders, criterias, err
			}

			if value == 1 {
				criterias.SearchBy["zatca.reporting_passed"] = bson.M{"$eq": true}
			} else if value == 0 {
				criterias.SearchBy["zatca.reporting_passed"] = bson.M{"$ne": true}
			}
		}*/

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return orders, criterias, err
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
			return orders, criterias, err
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
			return orders, criterias, err
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
			return orders, criterias, err
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
			return orders, criterias, err
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
			return orders, criterias, err
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

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return orders, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["net_total"] = value
		}

	}

	keys, ok = r.URL.Query()["search[cash_discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return orders, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["cash_discount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["cash_discount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[total_payment_received]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return orders, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["total_payment_received"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["total_payment_received"] = value
		}

	}

	keys, ok = r.URL.Query()["search[balance_amount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return orders, criterias, err
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
			return orders, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["payments_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["payments_count"] = value
		}
	}

	keys, ok = r.URL.Query()["search[return_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return orders, criterias, err
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
			return orders, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["return_amount"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["return_amount"] = value
		}
	}

	keys, ok = r.URL.Query()["search[net_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return orders, criterias, err
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
			return orders, criterias, err
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
			return orders, criterias, err
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
				return orders, criterias, err
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
				return orders, criterias, err
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

		/*
			objecIds := []primitive.ObjectID{}

			for _, id := range categoryIds {
				categoryID, err := primitive.ObjectIDFromHex(id)
				if err != nil {
					return products, criterias, err
				}
				objecIds = append(objecIds, categoryID)
			}
		*/

		if len(paymentMethods) > 0 {
			criterias.SearchBy["payment_methods"] = bson.M{"$in": paymentMethods}
		}
	}

	keys, ok = r.URL.Query()["search[delivered_by]"]
	if ok && len(keys[0]) >= 1 {
		deliveredByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return orders, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
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
		return orders, criterias, errors.New("Error fetching orders:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return orders, criterias, errors.New("Cursor error:" + err.Error())
		}
		order := Order{}
		err = cur.Decode(&order)
		if err != nil {
			return orders, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			order.Store, _ = FindStoreByID(order.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			order.Customer, _ = store.FindCustomerByID(order.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			order.CreatedByUser, _ = FindUserByID(order.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			order.UpdatedByUser, _ = FindUserByID(order.UpdatedBy, updatedByUserSelectFields)
		}
		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				order.DeletedByUser, _ = FindUserByID(order.DeletedBy, deletedByUserSelectFields)
			}
		*/
		orders = append(orders, order)
	} //end for loop

	return orders, criterias, nil
}

func (order *Order) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldOrder *Order) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	if govalidator.IsNull(order.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, order.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		order.Date = &date
	}

	if !govalidator.IsNull(strings.TrimSpace(order.Phone)) && !ValidateSaudiPhone(strings.TrimSpace(order.Phone)) {
		errs["phone"] = "Invalid phone no."
	}

	if !govalidator.IsNull(strings.TrimSpace(order.VatNo)) && !IsValidDigitNumber(strings.TrimSpace(order.VatNo), "15") {
		errs["vat_no"] = "VAT No. should be 15 digits"
	} else if !govalidator.IsNull(strings.TrimSpace(order.VatNo)) && !IsNumberStartAndEndWith(strings.TrimSpace(order.VatNo), "3") {
		errs["vat_no"] = "VAT No. should start and end with 3"
	}

	if order.Discount < 0 {
		errs["discount"] = "Cash discount should not be < 0"
	}

	if order.CashDiscount >= order.NetTotal {
		errs["cash_discount"] = "Cash discount should not be >= " + fmt.Sprintf("%.02f", order.NetTotal)
	} else if order.CashDiscount < 0 {
		errs["cash_discount"] = "Cash discount should not < 0 "
	}

	totalPayment := float64(0.00)
	for _, payment := range order.PaymentsInput {
		if payment.Amount != nil {
			totalPayment += *payment.Amount
		}
	}

	totalAmountFromCustomerAccount := 0.00
	customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "Invalid customer"
		return errs
	}

	if customer == nil && !govalidator.IsNull(order.CustomerName) {
		now := time.Now()
		newCustomer := Customer{
			Name:      order.CustomerName,
			CreatedBy: order.CreatedBy,
			UpdatedBy: order.CreatedBy,
			CreatedAt: &now,
			UpdatedAt: &now,
			StoreID:   order.StoreID,
		}

		err = newCustomer.MakeCode()
		if err != nil {
			errs["customer_id"] = "error creating new code"
		}
		err = newCustomer.Insert()
		if err != nil {
			errs["customer_id"] = "error creating new customer"
		}
		newCustomer.UpdateForeignLabelFields()
		order.CustomerID = &newCustomer.ID
	}

	/*
		if totalPayment == 0 {

			_, ok := customer.Stores[store.ID.Hex()]
			if ok {
				//customer.Stores[store.ID.Hex()].SalesBalanceAmount

			}
		}*/

	/*if store.Zatca.Phase == "2" {
	var lastOrder *Order
	if order.ID.IsZero() {
		lastOrder, err = store.FindLastOrderByStoreID(&store.ID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments && err != mongo.ErrNilDocument {
			errs["store_id"] = "Error finding last order"
		}
	} else {
		lastOrder, err = order.FindPreviousOrder(bson.M{})
		if err != nil && err != mongo.ErrNoDocuments && err != mongo.ErrNilDocument {
			errs["store_id"] = "Error finding last order"
		}
	}

	if lastOrder != nil {
		/*
			if (lastOrder.Zatca.ReportingFailedCount > 0 || lastOrder.Zatca.ComplianceCheckFailedCount > 0) && !lastOrder.Zatca.ReportingPassed {
				errs["last_order"] = "Last sale is not reported to Zatca. please report it and try again"
			}
	*/
	/*
		if !lastOrder.Zatca.ReportingPassed && order.EnableReportToZatca {
			errs["reporting_to_zatca"] = "Last sale is not reported to Zatca. please report it and try again"
		}*/
	/*	}
		}*/

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

	var customerAccount *Account

	if customer != nil {
		customerAccount, err = store.FindAccountByReferenceID(customer.ID, *order.StoreID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			errs["customer_account"] = "Error finding customer account: " + err.Error()
		}

		if order.BalanceAmount > 0 && customerAccount != nil {
			if customerAccount.Type == "asset" && customer.CreditLimit > 0 && ((customerAccount.Balance + order.BalanceAmount) > customer.CreditLimit) {
				errs["customer_id"] = "Customer is exceeding credit limit amount: " + fmt.Sprintf("%.02f", (customer.CreditLimit)) + " as his credit balance is already " + fmt.Sprintf("%.02f", (customerAccount.Balance))
				return errs
			}
		}
	}

	for index, payment := range order.PaymentsInput {
		if govalidator.IsNull(payment.DateStr) {
			errs["payment_date_"+strconv.Itoa(index)] = "Payment date is required"
		} else {
			const shortForm = "2006-01-02T15:04:05Z07:00"
			date, err := time.Parse(shortForm, payment.DateStr)
			if err != nil {
				errs["payment_date_"+strconv.Itoa(index)] = "Invalid date format"
			}

			order.PaymentsInput[index].Date = &date
			payment.Date = &date

			if order.Date != nil && IsAfter(order.Date, order.PaymentsInput[index].Date) {
				errs["payment_date_"+strconv.Itoa(index)] = "Payment date time should be greater than or equal to order date time"
			}
		}

		if payment.Amount == nil {
			errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount is required"
		} else if *payment.Amount == 0 {
			errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
		}

		if payment.Method == "" {
			errs["payment_method_"+strconv.Itoa(index)] = "Payment method is required"
		}

		if payment.DateStr != "" && payment.Amount != nil && payment.Method != "" {
			if *payment.Amount <= 0 {
				errs["payment_amount_"+strconv.Itoa(index)] = "Payment amount should be greater than zero"
			}

			/*
				maxAllowedAmount := (order.NetTotal - order.CashDiscount) - (totalPayment - *payment.Amount)

				if maxAllowedAmount < 0 {
					maxAllowedAmount = 0
				}

				if maxAllowedAmount == 0 {
					errs["payment_amount_"+strconv.Itoa(index)] = "Total amount should not exceed " + fmt.Sprintf("%.02f", (order.NetTotal-order.CashDiscount)) + ", Please delete this payment"
				} else if *payment.Amount > RoundFloat(maxAllowedAmount, 2) {
					errs["payment_amount_"+strconv.Itoa(index)] = "Amount should not be greater than " + fmt.Sprintf("%.02f", (maxAllowedAmount)) + ", Please delete or edit this payment"
				}
			*/

		}

		if payment.Method == "customer_account" && customer == nil {
			errs["payment_method_"+strconv.Itoa(index)] = "Invalid payment method: Customer account"
		}

		if customer != nil && payment.Method == "customer_account" {
			totalAmountFromCustomerAccount += *payment.Amount
			log.Print("Checking customer account Balance")

			if customerAccount != nil {
				if scenario == "update" {
					extraAmount := 0.00
					var oldSalesPayment *SalesPayment
					oldOrder.GetPayments()
					for _, oldPayment := range oldOrder.Payments {
						if oldPayment.ID.Hex() == payment.ID.Hex() {
							oldSalesPayment = &oldPayment
							break
						}
					}

					if oldSalesPayment != nil && *oldSalesPayment.Amount < *payment.Amount {
						extraAmount = *payment.Amount - *oldSalesPayment.Amount
					} else if oldSalesPayment == nil {
						//New payment added
						extraAmount = *payment.Amount
					} else {
						log.Print("payment amount not increased")
					}

					if extraAmount > 0 {
						if customerAccount.Balance == 0 {
							errs["payment_method_"+strconv.Itoa(index)] = "customer account balance is zero, Please add " + fmt.Sprintf("%.02f", (extraAmount)) + " to customer account to continue"
						} else if customerAccount.Type == "asset" {
							errs["payment_method_"+strconv.Itoa(index)] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", (customerAccount.Balance+extraAmount)) + " to customer account to continue"
						} else if customerAccount.Type == "liability" && customerAccount.Balance < extraAmount {
							errs["payment_method_"+strconv.Itoa(index)] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", extraAmount) + " to customer account to continue"
						}
					}

				} else {
					if customerAccount.Balance == 0 {
						errs["payment_method_"+strconv.Itoa(index)] = "customer account balance is zero"
					} else if customerAccount.Type == "asset" {
						errs["payment_method_"+strconv.Itoa(index)] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance)
					} else if customerAccount.Type == "liability" && customerAccount.Balance < *payment.Amount {
						errs["payment_method_"+strconv.Itoa(index)] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance)
					}
				}

			} else {
				errs["payment_method_"+strconv.Itoa(index)] = "customer account balance is zero"
			}
		}

	} //end for

	if totalAmountFromCustomerAccount > 0 {
		if customer != nil && customerAccount != nil {
			if scenario == "update" {
				oldTotalAmountFromCustomerAccount := 0.0
				extraAmountRequired := 0.00
				oldOrder.GetPayments()
				for _, oldPayment := range oldOrder.Payments {
					if oldPayment.Method == "customer_account" {
						oldTotalAmountFromCustomerAccount += *oldPayment.Amount
					}
				}

				if totalAmountFromCustomerAccount > oldTotalAmountFromCustomerAccount {
					extraAmountRequired = totalAmountFromCustomerAccount - oldTotalAmountFromCustomerAccount
				}

				if extraAmountRequired > 0 {
					if customerAccount.Balance == 0 {
						errs["customer_id"] = "customer account balance is zero, Please add " + fmt.Sprintf("%.02f", (extraAmountRequired)) + " to customer account to continue"
					} else if customerAccount.Type == "asset" {
						errs["customer_id"] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", (customerAccount.Balance+extraAmountRequired)) + " to customer account to continue"
					} else if customerAccount.Type == "liability" && customerAccount.Balance < extraAmountRequired {
						errs["customer_id"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance) + ", Please add " + fmt.Sprintf("%.02f", extraAmountRequired) + " to customer account to continue"
					}

				}

			} else {
				if customerAccount.Balance == 0 {
					errs["customer_id"] = "customer account balance is zero"
				} else if customerAccount.Type == "asset" {
					errs["customer_id"] = "customer owe us: " + fmt.Sprintf("%.02f", customerAccount.Balance)
				} else if customerAccount.Balance < totalAmountFromCustomerAccount {
					errs["customer_id"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerAccount.Balance)
				}
			}

		} else {
			errs["customer_id"] = "customer account balance is zero"
		}
	}

	if scenario == "update" {
		if order.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsOrderExists(&order.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Order:" + order.ID.Hex()
		}

	}

	if order.StoreID == nil || order.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(order.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + order.StoreID.Hex()
			return errs
		}
	}

	if order.DeliveredBy == nil || order.DeliveredBy.IsZero() {
		errs["delivered_by"] = "Delivered By is required"
	} else {
		exists, err := IsUserExists(order.DeliveredBy)
		if err != nil {
			errs["delivered_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by"] = "Invalid Delivered By:" + order.DeliveredBy.Hex()
		}
	}

	if len(order.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for order"
	}

	/*
		if order.DeliveredBySignatureID != nil && !order.DeliveredBySignatureID.IsZero() {
			exists, err := IsSignatureExists(order.DeliveredBySignatureID)
			if err != nil {
				errs["delivered_by_signature_id"] = err.Error()
				return errs
			}

			if !exists {
				errs["delivered_by_signature_id"] = "Invalid Delivered By Signature:" + order.DeliveredBySignatureID.Hex()
			}
		}
	*/

	for index, product := range order.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for order"
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

		if product.UnitDiscount > product.UnitPrice && product.UnitPrice > 0 {
			errs["unit_discount_"+strconv.Itoa(index)] = "Unit discount should not be greater than unit price"
		}

		if scenario == "update" {
			if product.Quantity < product.QuantityReturned {
				errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be less than the returned quantity: " + fmt.Sprintf("%.02f", product.QuantityReturned)
			}
		}

		if product.UnitPrice == 0 {
			errs["unit_price_"+strconv.Itoa(index)] = "Unit Price is required"
		}

		/*
			stock, err := GetProductStockInStore(&product.ProductID, order.StoreID)
			if err != nil {
				errs["quantity_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if stock < product.Quantity {
				productObject, err := FindProductByID(&product.ProductID, bson.M{})
				if err != nil {
					errs["product_id_"+strconv.Itoa(index)] = err.Error()
					return errs
				}

				storeObject, err := FindStoreByID(order.StoreID, nil)
				if err != nil {
					errs["store"] = err.Error()
					return errs
				}

				errs["quantity_"+strconv.Itoa(index)] = "Product: " + productObject.Name + " stock is only " + fmt.Sprintf("%.02f", stock) + " in Store: " + storeObject.Name
			}
		*/
	}

	if order.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (order *Order) ValidateZatcaReporting() (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store"
		return errs
	}

	customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		errs["customer_id"] = "invalid customer"
		return errs
	}

	if customer != nil && customer.VATNo != "" {
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
			errs["customer_id"] = "Fix the customer errors: " + strings.Join(customerErrorMessages, ", ")
		}

	}

	return errs
}

func (store *Store) GetProductStockInStore(
	productID *primitive.ObjectID,
	storeID *primitive.ObjectID,
) (stock float64, err error) {

	product, err := store.FindProductByID(productID, bson.M{})
	if err != nil {
		return 0, err
	}

	if storeID == nil {
		return 0, err
	}

	for _, productStore := range product.ProductStores {
		if productStore.StoreID.Hex() == storeID.Hex() {
			return productStore.Stock, nil
		}
	}

	return 0, err
}

func (order *Order) RemoveStock() (err error) {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := store.FindProductByID(&orderProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.ProductStores) == 0 {
			store, err := FindStoreByID(order.StoreID, bson.M{})
			if err != nil {
				return err
			}
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[order.StoreID.Hex()] = ProductStore{
				StoreID:           *order.StoreID,
				StoreName:         order.StoreName,
				StoreNameInArabic: store.NameInArabic,
				Stock:             float64(0),
			}
		}

		if productStoreTemp, ok := product.ProductStores[order.StoreID.Hex()]; ok {
			productStoreTemp.Stock -= (orderProduct.Quantity - orderProduct.QuantityReturned)
			product.ProductStores[order.StoreID.Hex()] = productStoreTemp
		}
		/*
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == order.StoreID.Hex() {
					product.ProductStores[k].Stock -= (orderProduct.Quantity - orderProduct.QuantityReturned)
					break
				}
			}
		*/

		err = product.Update(nil)
		if err != nil {
			return err
		}

	}

	err = order.Update()
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) AddStock() (err error) {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := store.FindProductByID(&orderProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		/*
			storeExistInProductStore := false
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == order.StoreID.Hex() {
					product.Stores[k].Stock += orderProduct.Quantity
					storeExistInProductStore = true
					break
				}
			}

			if !storeExistInProductStore {
				productStore := ProductStore{
					StoreID: *order.StoreID,
					Stock:   orderProduct.Quantity,
				}
				product.Stores = append(product.Stores, productStore)
			}
		*/

		if productStoreTemp, ok := product.ProductStores[order.StoreID.Hex()]; ok {
			productStoreTemp.Stock += orderProduct.Quantity
			product.ProductStores[order.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[order.StoreID.Hex()] = ProductStore{
				StoreID: *order.StoreID,
				Stock:   orderProduct.Quantity,
			}
		}

		err = product.Update(nil)
		if err != nil {
			return err
		}
	}

	err = order.Update()
	if err != nil {
		return err
	}

	return nil
}

func (order *Order) CalculateOrderProfit() error {
	totalProfit := float64(0.0)
	totalLoss := float64(0.0)
	for i, orderProduct := range order.Products {
		/*
			product, err := FindProductByID(&orderProduct.ProductID, map[string]interface{}{})
			if err != nil {
				return err
			}
		*/
		quantity := orderProduct.Quantity

		salesPrice := quantity * (orderProduct.UnitPrice - orderProduct.UnitDiscount)
		purchaseUnitPrice := orderProduct.PurchaseUnitPrice

		/*
			product, err := FindProductByID(&orderProduct.ProductID, map[string]interface{}{})
			if err != nil {
				return err
			}


				if purchaseUnitPrice == 0 ||
					order.Products[i].Loss > 0 ||
					order.Products[i].Profit <= 0 {
					for _, store := range product.ProductStores {
						if store.StoreID == *order.StoreID {
							purchaseUnitPrice = store.PurchaseUnitPrice
							order.Products[i].PurchaseUnitPrice = purchaseUnitPrice
							break
						}
					}
				}*/

		profit := 0.0
		if purchaseUnitPrice > 0 {
			profit = salesPrice - (quantity * purchaseUnitPrice)
		}

		loss := 0.0

		//profit = RoundFloat(profit, 2)

		if profit >= 0 {
			order.Products[i].Profit = profit
			order.Products[i].Loss = loss
			totalProfit += order.Products[i].Profit
		} else {
			order.Products[i].Profit = 0
			loss = (profit * -1)
			order.Products[i].Loss = loss
			totalLoss += order.Products[i].Loss
		}

	}
	order.Profit = totalProfit
	//order.NetProfit = RoundFloat(((totalProfit - order.CashDiscount) - order.Discount), 2)
	order.NetProfit = (totalProfit - order.CashDiscount) - order.Discount
	order.Loss = totalLoss
	order.NetLoss = totalLoss
	if order.NetProfit < 0 {
		order.NetLoss += (order.NetProfit * -1)
		order.NetProfit = 0.00
	}

	return nil
}

func (order *Order) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": order.StoreID}, "order")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (order *Order) ClearPayments() error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("sales_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"order_id": order.ID})
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) GetPaymentsCount() (count int64, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"order_id": order.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (order *Order) Update() error {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": order},
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

func (order *Order) Insert() error {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	order.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &order)
	if err != nil {
		return err
	}

	return nil
}

func (order *Order) AddPayments() error {
	for _, payment := range order.PaymentsInput {
		salesPayment := SalesPayment{
			OrderID:       &order.ID,
			OrderCode:     order.Code,
			Amount:        payment.Amount,
			Method:        payment.Method,
			Date:          payment.Date,
			CreatedAt:     order.CreatedAt,
			UpdatedAt:     order.UpdatedAt,
			CreatedBy:     order.CreatedBy,
			CreatedByName: order.CreatedByName,
			UpdatedBy:     order.UpdatedBy,
			UpdatedByName: order.UpdatedByName,
			StoreID:       order.StoreID,
			StoreName:     order.StoreName,
		}
		err := salesPayment.Insert()
		if err != nil {
			return err
		}

	}

	return nil
}

func (order *Order) UpdatePayments() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	order.GetPayments()
	now := time.Now()
	for _, payment := range order.PaymentsInput {
		if payment.ID.IsZero() {
			//Create new
			salesPayment := SalesPayment{
				OrderID:       &order.ID,
				OrderCode:     order.Code,
				Amount:        payment.Amount,
				Method:        payment.Method,
				Date:          payment.Date,
				CreatedAt:     &now,
				UpdatedAt:     &now,
				CreatedBy:     order.CreatedBy,
				CreatedByName: order.CreatedByName,
				UpdatedBy:     order.UpdatedBy,
				UpdatedByName: order.UpdatedByName,
				StoreID:       order.StoreID,
				StoreName:     order.StoreName,
			}
			err := salesPayment.Insert()
			if err != nil {
				return err
			}

		} else {
			//Update
			salesPayment, err := store.FindSalesPaymentByID(&payment.ID, bson.M{})
			if err != nil {
				return err
			}

			salesPayment.Date = payment.Date
			salesPayment.Amount = payment.Amount
			salesPayment.Method = payment.Method
			salesPayment.UpdatedAt = &now
			salesPayment.UpdatedBy = order.UpdatedBy
			salesPayment.UpdatedByName = order.UpdatedByName
			err = salesPayment.Update()
			if err != nil {
				return err
			}
		}

	}

	//Deleting payments

	paymentsToDelete := []SalesPayment{}

	for _, payment := range order.Payments {
		found := false
		for _, paymentInput := range order.PaymentsInput {
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
		payment.DeletedBy = order.UpdatedBy
		err := payment.Update()
		if err != nil {
			return err
		}
	}

	return nil
}

func (order *Order) GetPayments() (models []SalesPayment, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("sales_payment")
	ctx := context.Background()
	findOptions := options.Find()

	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"order_id": order.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching order payment history" + err.Error())
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
		model := SalesPayment{}
		err = cur.Decode(&model)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")

		models = append(models, model)

		totalPaymentReceived += *model.Amount

		if !slices.Contains(paymentMethods, model.Method) {
			paymentMethods = append(paymentMethods, model.Method)
		}
	} //end for loop

	order.TotalPaymentReceived = ToFixed(totalPaymentReceived, 2)
	order.BalanceAmount = ToFixed((order.NetTotal-order.CashDiscount)-totalPaymentReceived, 2)
	order.PaymentMethods = paymentMethods
	order.Payments = models
	order.PaymentsCount = int64(len(models))

	if ToFixed((order.NetTotal-order.CashDiscount), 2) <= ToFixed(totalPaymentReceived, 2) {
		order.PaymentStatus = "paid"
	} else if ToFixed(totalPaymentReceived, 2) > 0 {
		order.PaymentStatus = "paid_partially"
	} else if ToFixed(totalPaymentReceived, 2) <= 0 {
		order.PaymentStatus = "not_paid"
	}

	return models, err
}

func (order *Order) MakeRedisCode() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := order.StoreID.Hex() + "_invoice_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetSalesCount()
		if err != nil {
			return err
		}

		startFrom := store.SalesSerialNumber.StartFromCount

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

	paddingCount := store.SalesSerialNumber.PaddingCount
	if store.SalesSerialNumber.Prefix != "" {
		order.Code = fmt.Sprintf("%s-%0*d", store.SalesSerialNumber.Prefix, paddingCount, incr)
	} else {
		order.Code = fmt.Sprintf("%s%0*d", store.SalesSerialNumber.Prefix, paddingCount, incr)
	}

	if store.CountryCode != "" {
		timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]
		if ok {
			location, err := time.LoadLocation(timeZone)
			if err != nil {
				return errors.New("error loading location")
			}
			if order.Date != nil {
				currentDate := order.Date.In(location).Format("20060102") // YYYYMMDD
				order.Code = strings.ReplaceAll(order.Code, "DATE", currentDate)
			}
		}
	}

	order.InvoiceCountValue = incr - (store.SalesSerialNumber.StartFromCount - 1)
	return nil
}

func (store *Store) GetSalesCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (order *Order) UnMakeRedisCode() error {
	redisKey := order.StoreID.Hex() + "_invoice_counter"

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

func (order *Order) MakeCode() error {
	return order.MakeRedisCode()
}

func (order *Order) UnMakeCode() error {
	return order.UnMakeRedisCode()
}

func (store *Store) FindLastOrderByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (order *Order, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"created_at": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&order)
	if err != nil {
		return nil, err
	}

	return order, err
}

/*
func GetSalesHistoriesByProductID(productID *primitive.ObjectID) (models []ProductSalesHistory, err error) {
	//log.Print("Fetching sales histories")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"product_id": productID}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching product sales history" + err.Error())
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
		salesHistory := ProductSalesHistory{}
		err = cur.Decode(&salesHistory)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")
		models = append(models, salesHistory)
	} //end for loop

	return models, nil
}
*/

func (order *Order) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if order.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": order.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": order.Code,
			"_id":  bson.M{"$ne": order.ID},
		})
	}

	return (count > 0), err
}

/*
func GenerateOrderCode(startFrom int) (string, error) {
	//letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	/*
		letterRunes := []rune("0123456789")
		b := make([]rune, n)
		for i := range b {
			b[i] = letterRunes[rand.Intn(len(letterRunes))]
		}
		return string(b)
*/
/*
	count, err := GetTotalCount(bson.M{}, "order")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return strconv.Itoa(code + 1), nil
}*/

func (order *Order) UpdateOrderStatus(status string) (*Order, error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": bson.M{"status": status}},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return order, nil
	}
	return nil, nil
}

func (order *Order) DeleteOrder(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	/*
		userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
		if err != nil {
			return err
		}


			order.Deleted = true
			order.DeletedBy = &userID
			now := time.Now()
			order.DeletedAt = &now
	*/

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": order.ID},
		bson.M{"$set": order},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindOrderByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (order *Order, err error) {

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
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
		Decode(&order)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "store")
		order.Store, _ = FindStoreByID(order.StoreID, fields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "customer")
		order.Customer, _ = store.FindCustomerByID(order.CustomerID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		order.CreatedByUser, _ = FindUserByID(order.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		order.UpdatedByUser, _ = FindUserByID(order.UpdatedBy, fields)
	}

	/*
		if _, ok := selectFields["deleted_by_user.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
			order.DeletedByUser, _ = FindUserByID(order.DeletedBy, fields)
		}*/

	return order, err
}

func (order *Order) FindPreviousOrder(selectFields map[string]interface{}) (previousOrder *Order, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"invoice_count_value": (order.InvoiceCountValue - 1),
			"store_id":            order.StoreID,
		}, findOneOptions).
		Decode(&previousOrder)
	if err != nil {
		return nil, err
	}

	return previousOrder, err
}

func (order *Order) FindLastReportedOrder(selectFields map[string]interface{}) (lastReportedOrder *Order, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
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
			"store_id":               order.StoreID,
		}, findOneOptions).
		Decode(&lastReportedOrder)
	if err != nil {
		return nil, err
	}

	return lastReportedOrder, err
}

func (store *Store) IsOrderExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}
func (order *Order) HardDelete() error {
	log.Print("Delete order")
	ctx := context.Background()
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("order")
	_, err := collection.DeleteOne(ctx, bson.M{
		"_id": order.ID,
	})
	if err != nil {
		return err
	}

	err = order.HardDeleteSalesReturn()
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) HardDeleteSalesReturn() error {
	log.Print("Delete sales Return")
	ctx := context.Background()
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("salesreturn")
	_, err := collection.DeleteOne(ctx, bson.M{
		"order_id": order.ID,
	})
	if err != nil {
		return err
	}

	return nil
}

func GenerateRandom15DigitNumber() string {
	rand.Seed(time.Now().UnixNano())

	// Generate 13 random digits
	middle := rand.Int63n(1e13) // 13 digits (from 0000000000000 to 9999999999999)

	// Format the number with start and end 3
	randomNumber := fmt.Sprintf("3%013d3", middle)

	return randomNumber
}

func GetDecimalPoints(num float64) int {
	str := strconv.FormatFloat(num, 'f', -1, 64) // Convert float to string without rounding
	parts := strings.Split(str, ".")
	if len(parts) == 2 {
		return len(parts[1]) // Return length of the decimal part
	}
	return 0
}

func ProcessOrders() error {
	log.Print("Processing sales")
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
		}, "order")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)
		findOptions.SetSort(GetSortByFields("created_at"))

		//	criterias.SearchBy["zatca.reporting_passed"] = bson.M{"$ne": true}
		//"zatca.compliance_check_failed_count": bson.M{"$lt": 1},
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
			order := Order{}
			err = cur.Decode(&order)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if order.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			order.ReturnAmount, order.ReturnCount, _ = store.GetReturnedAmountByOrderID(order.ID)
			order.Update()

			/*
				log.Print("Order ID: " + order.Code)
				err = order.ReportToZatca()
				if err != nil {
					log.Print("Failed 1st time, trying 2nd time")

					if GetDecimalPoints(order.ShippingOrHandlingFees) > 2 {
						log.Print("Trimming shipping cost to 2 decimals")
						order.ShippingOrHandlingFees = RoundTo2Decimals(order.ShippingOrHandlingFees)
					}

					if GetDecimalPoints(order.Discount) > 2 {
						log.Print("Trimming discount to 2 decimals")
						order.Discount = RoundTo2Decimals(order.Discount)
					}

					order.FindNetTotal()
					order.Update()

					err = order.ReportToZatca()
					if err != nil {
						log.Print("Failed  2nd time. ")
						customer, _ := store.FindCustomerByID(order.CustomerID, bson.M{})
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

							if utf8.RuneCountInString(customer.VATNo) != 15 || !IsValidDigitNumber(customer.VATNo, "15") || !IsNumberStartAndEndWith(customer.VATNo, "3") {

								customer.VATNo = GenerateRandom15DigitNumber()
								log.Print("Replaced invalid vat no.")
							}

							customer.Update()
							err = order.ReportToZatca()
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
			/* to get missing customers
			customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
			if err != nil && err != mongo.ErrNilCursor && err != mongo.ErrNoDocuments {
				return errors.New("error finding customer" + err.Error())
			}

			if customer != nil {
				if customer.StoreID.Hex() != order.StoreID.Hex() {
					customer.StoreID = order.StoreID
					err = customer.Update()
					if err != nil {
						log.Print("Error updating customer: " + err.Error())
					}
				}
			}
			*/

			/*
				err = order.Update()
				if err != nil {
					return errors.New("error updating sale:" + err.Error())
				}
			*/
			bar.Add(1)
		}

	}

	log.Print("Sales DONE!")
	return nil
}

type ProductSalesStats struct {
	SalesCount    int64   `json:"sales_count" bson:"sales_count"`
	SalesQuantity float64 `json:"sales_quantity" bson:"sales_quantity"`
	Sales         float64 `json:"sales" bson:"sales"`
	SalesProfit   float64 `json:"sales_profit" bson:"sales_profit"`
	SalesLoss     float64 `json:"sales_loss" bson:"sales_loss"`
}

func (product *Product) SetProductSalesStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductSalesStats

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
				"_id":            nil,
				"sales_count":    bson.M{"$sum": 1},
				"sales_quantity": bson.M{"$sum": "$quantity"},
				"sales":          bson.M{"$sum": "$net_price"},
				"sales_profit":   bson.M{"$sum": "$profit"},
				"sales_loss":     bson.M{"$sum": "$loss"},
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
		stats.Sales = RoundFloat(stats.Sales, 2)
		stats.SalesProfit = RoundFloat(stats.SalesProfit, 2)
		stats.SalesLoss = RoundFloat(stats.SalesLoss, 2)
	}

	/*
		for storeIndex, store := range product.Stores {
			if store.StoreID.Hex() == storeID.Hex() {
				product.Stores[storeIndex].SalesCount = stats.SalesCount
				product.Stores[storeIndex].SalesQuantity = stats.SalesQuantity
				product.Stores[storeIndex].Sales = stats.Sales
				product.Stores[storeIndex].SalesProfit = stats.SalesProfit
				product.Stores[storeIndex].SalesLoss = stats.SalesLoss
				err = product.Update()
				if err != nil {
					return err
				}
				break
			}
		}
	*/

	if productStoreTemp, ok := product.ProductStores[storeID.Hex()]; ok {
		productStoreTemp.SalesCount = stats.SalesCount
		productStoreTemp.SalesQuantity = stats.SalesQuantity
		productStoreTemp.Sales = stats.Sales
		productStoreTemp.SalesProfit = stats.SalesProfit
		productStoreTemp.SalesLoss = stats.SalesLoss
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (product *Product) SetProductSalesQuantityByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_sales_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductSalesStats

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
				"_id":            nil,
				"sales_quantity": bson.M{"$sum": "$quantity"},
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
		productStoreTemp.SalesQuantity = stats.SalesQuantity
		product.ProductStores[storeID.Hex()] = productStoreTemp
	}

	return nil
}

func (order *Order) SetProductsSalesStats() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, orderProduct := range order.Products {
		product, err := store.FindProductByID(&orderProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductSalesStatsByStoreID(*order.StoreID)
		if err != nil {
			return err
		}

		err = product.Update(nil)
		if err != nil {
			return err
		}

	}
	return nil
}

//Customer

type CustomerSalesStats struct {
	SalesCount              int64   `json:"sales_count" bson:"sales_count"`
	SalesAmount             float64 `json:"sales_amount" bson:"sales_amount"`
	SalesPaidAmount         float64 `json:"sales_paid_amount" bson:"sales_paid_amount"`
	SalesBalanceAmount      float64 `json:"sales_balance_amount" bson:"sales_balance_amount"`
	SalesProfit             float64 `json:"sales_profit" bson:"sales_profit"`
	SalesLoss               float64 `json:"sales_loss" bson:"sales_loss"`
	SalesPaidCount          int64   `json:"sales_paid_count" bson:"sales_paid_count"`
	SalesNotPaidCount       int64   `json:"sales_not_paid_count" bson:"sales_not_paid_count"`
	SalesPaidPartiallyCount int64   `json:"sales_paid_partially_count" bson:"sales_paid_partially_count"`
}

func (customer *Customer) SetCustomerSalesStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerSalesStats

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
				"_id":                  nil,
				"sales_count":          bson.M{"$sum": 1},
				"sales_amount":         bson.M{"$sum": "$net_total"},
				"sales_paid_amount":    bson.M{"$sum": "$total_payment_received"},
				"sales_balance_amount": bson.M{"$sum": "$balance_amount"},
				"sales_profit":         bson.M{"$sum": "$net_profit"},
				"sales_loss":           bson.M{"$sum": "$loss"},
				"sales_paid_count": bson.M{"$sum": bson.M{
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
				"sales_not_paid_count": bson.M{"$sum": bson.M{
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
				"sales_paid_partially_count": bson.M{"$sum": bson.M{
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
		stats.SalesAmount = RoundFloat(stats.SalesAmount, 2)
		stats.SalesPaidAmount = RoundFloat(stats.SalesPaidAmount, 2)
		stats.SalesBalanceAmount = RoundFloat(stats.SalesBalanceAmount, 2)
		stats.SalesProfit = RoundFloat(stats.SalesProfit, 2)
		stats.SalesLoss = RoundFloat(stats.SalesLoss, 2)
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
		customerStore.SalesCount = stats.SalesCount
		customerStore.SalesPaidCount = stats.SalesPaidCount
		customerStore.SalesNotPaidCount = stats.SalesNotPaidCount
		customerStore.SalesPaidPartiallyCount = stats.SalesPaidPartiallyCount
		customerStore.SalesAmount = stats.SalesAmount
		customerStore.SalesPaidAmount = stats.SalesPaidAmount
		customerStore.SalesBalanceAmount = stats.SalesBalanceAmount
		customerStore.SalesProfit = stats.SalesProfit
		customerStore.SalesLoss = stats.SalesLoss
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:                 storeID,
			StoreName:               store.Name,
			StoreNameInArabic:       store.NameInArabic,
			SalesCount:              stats.SalesCount,
			SalesPaidCount:          stats.SalesPaidCount,
			SalesNotPaidCount:       stats.SalesNotPaidCount,
			SalesPaidPartiallyCount: stats.SalesPaidPartiallyCount,
			SalesAmount:             stats.SalesAmount,
			SalesPaidAmount:         stats.SalesPaidAmount,
			SalesBalanceAmount:      stats.SalesBalanceAmount,
			SalesProfit:             stats.SalesProfit,
			SalesLoss:               stats.SalesLoss,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (order *Order) SetCustomerSalesStats() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(order.CustomerID, map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if customer != nil {
		err = customer.SetCustomerSalesStatsByStoreID(*order.StoreID)
		if err != nil {
			return err
		}
	}

	return nil
}

// Accounting
// Journal entries
func MakeJournalsForUnpaidSale(
	order *Order,
	customerAccount *Account,
	salesAccount *Account,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()

	groupID := primitive.NewObjectID()

	journals := []Journal{}

	balanceAmount := RoundFloat((order.NetTotal - order.CashDiscount), 2)

	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         balanceAmount,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	if order.CashDiscount > 0 {
		journals = append(journals, Journal{
			Date:          order.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         order.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     salesAccount.ID,
		AccountNumber: salesAccount.Number,
		AccountName:   salesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        order.NetTotal,
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

var totalSalesPaidAmount float64
var extraSalesAmountPaid float64
var extraSalesPayments []SalesPayment

func MakeJournalsForSalesPaymentsByDatetime(
	order *Order,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	salesAccount *Account,
	payments []SalesPayment,
	cashDiscountAllowedAccount *Account,
	paymentsByDatetimeNumber int,
) ([]Journal, error) {
	store, err := FindStoreByID(order.StoreID, bson.M{})
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
		totalSalesPaidAmount += *payment.Amount
		if totalSalesPaidAmount > (order.NetTotal - order.CashDiscount) {
			extraSalesAmountPaid = RoundFloat((totalSalesPaidAmount - (order.NetTotal - order.CashDiscount)), 2)
		}
		amount := *payment.Amount

		if extraSalesAmountPaid > 0 {
			skip := false
			if extraSalesAmountPaid < *payment.Amount {
				extraAmount := extraSalesAmountPaid
				extraSalesPayments = append(extraSalesPayments, SalesPayment{
					Date:   payment.Date,
					Amount: &extraAmount,
					Method: payment.Method,
				})
				amount = RoundFloat((*payment.Amount - extraSalesAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraSalesAmountPaid = 0
			} else if extraSalesAmountPaid >= *payment.Amount {
				extraSalesPayments = append(extraSalesPayments, SalesPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})

				skip = true
				extraSalesAmountPaid = RoundFloat((extraSalesAmountPaid - *payment.Amount), 2)
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

	if order.CashDiscount > 0 && paymentsByDatetimeNumber == 1 && IsDateTimesEqual(order.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          order.Date,
			AccountID:     cashDiscountAllowedAccount.ID,
			AccountNumber: cashDiscountAllowedAccount.Number,
			AccountName:   cashDiscountAllowedAccount.Name,
			DebitOrCredit: "debit",
			Debit:         order.CashDiscount,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	balanceAmount := RoundFloat(((order.NetTotal - order.CashDiscount) - totalPayment), 2)

	//Asset or debt increased
	if paymentsByDatetimeNumber == 1 && balanceAmount > 0 && IsDateTimesEqual(order.Date, firstPaymentDate) {
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
			order.StoreID,
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
			Date:          order.Date,
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

	if paymentsByDatetimeNumber == 1 && IsDateTimesEqual(order.Date, firstPaymentDate) {
		journals = append(journals, Journal{
			Date:          order.Date,
			AccountID:     salesAccount.ID,
			AccountNumber: salesAccount.Number,
			AccountName:   salesAccount.Name,
			DebitOrCredit: "credit",
			Credit:        order.NetTotal,
			GroupID:       groupID,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})
	} else if paymentsByDatetimeNumber > 1 || !IsDateTimesEqual(order.Date, firstPaymentDate) {
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
			order.StoreID,
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

func MakeJournalsForSalesExtraPayments(
	order *Order,
	customer *Customer,
	cashAccount *Account,
	bankAccount *Account,
	extraPayments []SalesPayment,
) ([]Journal, error) {
	store, err := FindStoreByID(order.StoreID, bson.M{})
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
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashReceivingAccount.ID,
			AccountNumber: cashReceivingAccount.Number,
			AccountName:   cashReceivingAccount.Name,
			DebitOrCredit: "debit",
			Debit:         *payment.Amount,
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
		order.StoreID,
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
		Credit:        order.BalanceAmount * (-1),
		GroupID:       groupID,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals, nil
}

// Regroup sales payments by datetime
func RegroupSalesPaymentsByDatetime(payments []SalesPayment) [][]SalesPayment {
	paymentsByDatetime := map[string][]SalesPayment{}
	for _, payment := range payments {
		paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")] = append(paymentsByDatetime[payment.Date.Format("2006-01-02T15:04")], payment)
	}

	paymentsByDatetime2 := [][]SalesPayment{}
	for _, v := range paymentsByDatetime {
		paymentsByDatetime2 = append(paymentsByDatetime2, v)
	}

	sort.Slice(paymentsByDatetime2, func(i, j int) bool {
		return paymentsByDatetime2[i][0].Date.Before(*paymentsByDatetime2[j][0].Date)
	})

	return paymentsByDatetime2
}

//End customer account journals

func (order *Order) CreateLedger() (ledger *Ledger, err error) {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return nil, err
	}

	now := time.Now()

	customer, err := store.FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}

	cashAccount, err := store.CreateAccountIfNotExists(order.StoreID, nil, nil, "Cash", nil, nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := store.CreateAccountIfNotExists(order.StoreID, nil, nil, "Bank", nil, nil)
	if err != nil {
		return nil, err
	}

	salesAccount, err := store.CreateAccountIfNotExists(order.StoreID, nil, nil, "Sales", nil, nil)
	if err != nil {
		return nil, err
	}

	cashDiscountAllowedAccount, err := store.CreateAccountIfNotExists(order.StoreID, nil, nil, "Cash discount allowed", nil, nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	var firstPaymentDate *time.Time
	if len(order.Payments) > 0 {
		firstPaymentDate = order.Payments[0].Date
	}

	if len(order.Payments) == 0 || (firstPaymentDate != nil && !IsDateTimesEqual(order.Date, firstPaymentDate)) {
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
			order.StoreID,
			referenceID,
			&referenceModel,
			customerName,
			&customerPhone,
			&customerVATNo,
		)
		if err != nil {
			return nil, err
		}
		journals = append(journals, MakeJournalsForUnpaidSale(
			order,
			customerAccount,
			salesAccount,
			cashDiscountAllowedAccount,
		)...)
	}

	if len(order.Payments) > 0 {
		totalSalesPaidAmount = float64(0.00)
		extraSalesAmountPaid = float64(0.00)
		extraSalesPayments = []SalesPayment{}

		paymentsByDatetimeNumber := 1
		paymentsByDatetime := RegroupSalesPaymentsByDatetime(order.Payments)
		//fmt.Printf("%+v", paymentsByDatetime)

		for _, paymentByDatetime := range paymentsByDatetime {
			newJournals, err := MakeJournalsForSalesPaymentsByDatetime(
				order,
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

		if order.BalanceAmount < 0 && len(extraSalesPayments) > 0 {
			newJournals, err := MakeJournalsForSalesExtraPayments(
				order,
				customer,
				cashAccount,
				bankAccount,
				extraSalesPayments,
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
		StoreID:        order.StoreID,
		ReferenceID:    order.ID,
		ReferenceModel: "sales",
		ReferenceCode:  order.Code,
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

func (order *Order) GetCashDiscounts() (models []SalesCashDiscount, err error) {
	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("sales_cash_discount")
	ctx := context.Background()
	findOptions := options.Find()

	sortBy := map[string]interface{}{}
	sortBy["date"] = 1
	findOptions.SetSort(sortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"order_id": order.ID, "deleted": bson.M{"$ne": true}}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching sales cash discounts" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, errors.New("Cursor error:" + err.Error())
		}
		model := SalesCashDiscount{}
		err = cur.Decode(&model)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")

		models = append(models, model)
	} //end for loop

	return models, err
}

func (order *Order) DoAccounting() error {
	ledger, err := order.CreateLedger()
	if err != nil {
		return errors.New("error creating ledger: " + err.Error())
	}

	_, err = ledger.CreatePostings()
	if err != nil {
		return errors.New("error creating postings: " + err.Error())
	}

	return nil
}

func (order *Order) UndoAccounting() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	ledger, err := store.FindLedgerByReferenceID(order.ID, *order.StoreID, bson.M{})
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

	err = store.RemoveLedgerByReferenceID(order.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = store.RemovePostingsByReferenceID(order.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}

//End Accounting
