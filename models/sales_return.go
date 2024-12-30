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

type SalesReturnProduct struct {
	ProductID         primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name              string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic      string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode          string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber        string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity          float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit              string             `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitPrice         float64            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	PurchaseUnitPrice float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	Profit            float64            `bson:"profit" json:"profit"`
	Loss              float64            `bson:"loss" json:"loss"`
	Selected          bool               `bson:"selected" json:"selected"`
}

// SalesReturn : SalesReturn structure
type SalesReturn struct {
	ID             primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	OrderID        *primitive.ObjectID  `json:"order_id,omitempty" bson:"order_id,omitempty"`
	OrderCode      string               `bson:"order_code,omitempty" json:"order_code,omitempty"`
	Date           *time.Time           `bson:"date,omitempty" json:"date,omitempty"`
	DateStr        string               `json:"date_str,omitempty" bson:"-"`
	Code           string               `bson:"code,omitempty" json:"code,omitempty"`
	StoreID        *primitive.ObjectID  `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID     *primitive.ObjectID  `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Store          *Store               `json:"store,omitempty"`
	Customer       *Customer            `json:"customer,omitempty"`
	Products       []SalesReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	ReceivedBy     *primitive.ObjectID  `json:"received_by,omitempty" bson:"received_by,omitempty"`
	ReceivedByUser *User                `json:"received_by_user,omitempty"`
	//ReceivedBySignatureID   *primitive.ObjectID  `json:"received_by_signature_id,omitempty" bson:"received_by_signature_id,omitempty"`
	//ReceivedBySignatureName string               `json:"received_by_signature_name,omitempty" bson:"received_by_signature_name,omitempty"`
	//ReceivedBySignature     *Signature           `json:"received_by_signature,omitempty"`
	//SignatureDate     *time.Time           `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	//SignatureDateStr  string               `json:"signature_date_str,omitempty"`
	VatPercent        *float64 `bson:"vat_percent" json:"vat_percent"`
	Discount          float64  `bson:"discount" json:"discount"`
	DiscountPercent   float64  `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent bool     `bson:"is_discount_percent" json:"is_discount_percent"`
	Status            string   `bson:"status,omitempty" json:"status,omitempty"`
	StockAdded        bool     `bson:"stock_added,omitempty" json:"stock_added,omitempty"`
	TotalQuantity     float64  `bson:"total_quantity" json:"total_quantity"`
	VatPrice          float64  `bson:"vat_price" json:"vat_price"`
	Total             float64  `bson:"total" json:"total"`
	NetTotal          float64  `bson:"net_total" json:"net_total"`
	CashDiscount      float64  `bson:"cash_discount" json:"cash_discount"`
	PaymentMethods    []string `json:"payment_methods" bson:"payment_methods"`
	PaymentStatus     string   `bson:"payment_status" json:"payment_status"`
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
	CustomerName     string               `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName        string               `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName    string               `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName    string               `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName    string               `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	Profit           float64              `bson:"profit" json:"profit"`
	NetProfit        float64              `bson:"net_profit" json:"net_profit"`
	Loss             float64              `bson:"loss" json:"loss"`
	NetLoss          float64              `bson:"net_loss" json:"net_loss"`
	TotalPaymentPaid float64              `bson:"total_payment_paid" json:"total_payment_paid"`
	BalanceAmount    float64              `bson:"balance_amount" json:"balance_amount"`
	Payments         []SalesReturnPayment `bson:"payments" json:"payments"`
	PaymentsInput    []SalesReturnPayment `bson:"-" json:"payments_input"`
	PaymentsCount    int64                `bson:"payments_count" json:"payments_count"`
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
	salesReturn.GetPayments()
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
			salesReturnPayment, err := FindSalesReturnPaymentByID(&payment.ID, bson.M{})
			if err != nil {
				return err
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
}

func GetSalesReturnStats(filter map[string]interface{}) (stats SalesReturnStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":           nil,
				"net_total":     bson.M{"$sum": "$net_total"},
				"vat_price":     bson.M{"$sum": "$vat_price"},
				"discount":      bson.M{"$sum": "$discount"},
				"cash_discount": bson.M{"$sum": "$cash_discount"},
				"net_profit":    bson.M{"$sum": "$net_profit"},
				"net_loss":      bson.M{"$sum": "$net_loss"},
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
								bson.M{"$and": []interface{}{
									bson.M{"$eq": []interface{}{"$$payment.method", "bank_account"}},
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

	if salesreturn.Status != salesreturnOld.Status {

		//if salesreturn.Status == "delivered" || salesreturn.Status == "dispatched" {

		err := salesreturnOld.AddStock()
		if err != nil {
			return err
		}

		/*
			err = salesreturn.RemoveStock()
			if err != nil {
				return err
			}
		*/
		//}
	}

	return nil
}

func (salesreturn *SalesReturn) UpdateForeignLabelFields() error {

	if salesreturn.StoreID != nil {
		store, err := FindStoreByID(salesreturn.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.StoreName = store.Name
	}

	if salesreturn.CustomerID != nil {
		customer, err := FindCustomerByID(salesreturn.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.CustomerName = customer.Name
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
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1})
		if err != nil {
			return err
		}
		salesreturn.Products[i].Name = productObject.Name
		salesreturn.Products[i].NameInArabic = productObject.NameInArabic
		salesreturn.Products[i].ItemCode = productObject.ItemCode
		salesreturn.Products[i].PartNumber = productObject.PartNumber
	}

	return nil
}

func (salesreturn *SalesReturn) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range salesreturn.Products {
		if !product.Selected {
			continue
		}
		netTotal += (float64(product.Quantity) * product.UnitPrice)
	}

	netTotal -= salesreturn.Discount

	if salesreturn.VatPercent != nil {
		netTotal += netTotal * (*salesreturn.VatPercent / float64(100))
	}

	salesreturn.NetTotal = RoundFloat(salesreturn.NetTotal, 2)
}

func (salesreturn *SalesReturn) FindTotal() {
	total := float64(0.0)
	for _, product := range salesreturn.Products {
		if !product.Selected {
			continue
		}

		total += (product.Quantity * product.UnitPrice)
	}
	salesreturn.Total = RoundFloat(total, 2)
}

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

func (salesreturn *SalesReturn) FindVatPrice() {
	vatPrice := ((*salesreturn.VatPercent / 100) * float64(salesreturn.Total-salesreturn.Discount))
	vatPrice = RoundFloat(vatPrice, 2)
	salesreturn.VatPrice = vatPrice
}

func SearchSalesReturn(w http.ResponseWriter, r *http.Request) (salesreturns []SalesReturn, criterias SearchCriterias, err error) {

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

	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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
			salesreturn.Customer, _ = FindCustomerByID(salesreturn.CustomerID, customerSelectFields)
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

func (salesreturn *SalesReturn) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldSalesReturn *SalesReturn) (errs map[string]string) {

	errs = make(map[string]string)

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
		if payment.Amount != nil {
			totalPayment += *payment.Amount
		}
	}

	for index, payment := range salesreturn.PaymentsInput {
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
				maxAllowedAmount := (salesreturn.NetTotal - salesreturn.CashDiscount) - (totalPayment - *payment.Amount)

				if maxAllowedAmount < 0 {
					maxAllowedAmount = 0
				}

				if maxAllowedAmount == 0 {
					errs["payment_amount_"+strconv.Itoa(index)] = "Total amount should not exceed " + fmt.Sprintf("%.02f", (salesreturn.NetTotal-salesreturn.CashDiscount)) + ", Please delete this payment"
				} else if *payment.Amount > RoundFloat(maxAllowedAmount, 2) {
					errs["payment_amount_"+strconv.Itoa(index)] = "Amount should not be greater than " + fmt.Sprintf("%.02f", (maxAllowedAmount)) + ", Please delete or edit this payment"
				}
			*/

		}
	} //end for

	if salesreturn.OrderID == nil || salesreturn.OrderID.IsZero() {
		w.WriteHeader(http.StatusBadRequest)
		errs["order_id"] = "Order ID is required"
		return errs
	}

	order, err := FindOrderByID(salesreturn.OrderID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["order_id"] = err.Error()
		return errs
	}

	maxDiscountAllowed := 0.00
	if scenario == "update" {
		maxDiscountAllowed = order.Discount - (order.ReturnDiscount - oldSalesReturn.Discount)
	} else {
		maxDiscountAllowed = order.Discount - order.ReturnDiscount
	}

	if salesreturn.Discount > maxDiscountAllowed {
		errs["discount"] = "Discount shouldn't greater than " + fmt.Sprintf("%.2f", (maxDiscountAllowed))
	}

	if salesreturn.NetTotal > 0 && salesreturn.CashDiscount >= salesreturn.NetTotal {
		errs["cash_discount"] = "Cash discount should not be >= " + fmt.Sprintf("%.02f", salesreturn.NetTotal)
	}

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
		exists, err := IsSalesReturnExists(&salesreturn.ID)
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

	if salesreturn.CustomerID == nil || salesreturn.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(salesreturn.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + salesreturn.CustomerID.Hex()
		}
	}

	/*
		if salesreturn.ReceivedBy == nil || salesreturn.ReceivedBy.IsZero() {
			errs["received_by"] = "Received By is required"
		} else {
			exists, err := IsUserExists(salesreturn.ReceivedBy)
			if err != nil {
				errs["received_by"] = err.Error()
				return errs
			}

			if !exists {
				errs["received_by"] = "Invalid Received By:" + salesreturn.ReceivedBy.Hex()
			}
		}
	*/

	if len(salesreturn.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for salesreturn"
	}

	/*
		if salesreturn.ReceivedBySignatureID != nil && !salesreturn.ReceivedBySignatureID.IsZero() {
			exists, err := IsSignatureExists(salesreturn.ReceivedBySignatureID)
			if err != nil {
				errs["received_by_signature_id"] = err.Error()
				return errs
			}

			if !exists {
				errs["received_by_signature_id"] = "Invalid Received By Signature:" + salesreturn.ReceivedBySignatureID.Hex()
			}
		}
	*/

	for index, salesReturnProduct := range salesreturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		if salesReturnProduct.ProductID.IsZero() {
			errs["product_id"] = "Product is required for Sales Return"
		} else {
			exists, err := IsProductExists(&salesReturnProduct.ProductID)
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

		for _, orderProduct := range order.Products {
			if orderProduct.ProductID == salesReturnProduct.ProductID {
				//soldQty := RoundFloat((orderProduct.Quantity - orderProduct.QuantityReturned), 2)
				maxAllowedQuantity := 0.00
				if scenario == "update" {
					maxAllowedQuantity = orderProduct.Quantity - (orderProduct.QuantityReturned - oldSalesReturn.Products[index].Quantity)
				} else {
					log.Print("Creating")
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

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (salesreturn *SalesReturn) UpdateReturnedQuantityInOrderProduct(salesReturnOld *SalesReturn) error {
	order, err := FindOrderByID(salesreturn.OrderID, bson.M{})
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

func (salesreturn *SalesReturn) AddStock() (err error) {
	if len(salesreturn.Products) == 0 {
		return nil
	}

	for _, salesreturnProduct := range salesreturn.Products {
		if !salesreturnProduct.Selected {
			continue
		}

		product, err := FindProductByID(&salesreturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if productStoreTemp, ok := product.ProductStores[salesreturn.StoreID.Hex()]; ok {
			productStoreTemp.Stock += salesreturnProduct.Quantity
			product.ProductStores[salesreturn.StoreID.Hex()] = productStoreTemp
		} else {
			product.ProductStores = map[string]ProductStore{}
			product.ProductStores[salesreturn.StoreID.Hex()] = ProductStore{
				StoreID: *salesreturn.StoreID,
				Stock:   salesreturnProduct.Quantity,
			}
		}

		/*
			storeExistInProductStore := false
			for k, productStore := range product.ProductStores {
				if productStore.StoreID.Hex() == salesreturn.StoreID.Hex() {

					product.Stores[k].Stock += salesreturnProduct.Quantity
					storeExistInProductStore = true
					break
				}
			}

			if !storeExistInProductStore {
				productStore := ProductStore{
					StoreID: *salesreturn.StoreID,
					Stock:   salesreturnProduct.Quantity,
				}
				product.Stores = append(product.Stores, productStore)
			}
		*/

		err = product.Update()
		if err != nil {
			return err
		}
	}

	salesreturn.StockAdded = false
	err = salesreturn.Update()
	if err != nil {
		return err
	}

	return nil
}

func (salesreturn *SalesReturn) GenerateCode(startFrom int, storeCode string) (string, error) {
	count, err := GetTotalCount(bson.M{"store_id": salesreturn.StoreID}, "salesreturn")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (salesreturn *SalesReturn) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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
	order, err := FindOrderByID(salesReturn.OrderID, map[string]interface{}{})
	if err != nil {
		return err
	}

	for i, product := range salesReturn.Products {
		if !product.Selected {
			continue
		}

		quantity := product.Quantity
		salesPrice := quantity * product.UnitPrice
		purchaseUnitPrice := product.PurchaseUnitPrice
		for _, orderProduct := range order.Products {
			if orderProduct.ProductID.Hex() == product.ProductID.Hex() {
				purchaseUnitPrice = orderProduct.PurchaseUnitPrice
				break
			}
		}
		salesReturn.Products[i].PurchaseUnitPrice = purchaseUnitPrice

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

		}

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
	salesReturn.Profit = RoundFloat(totalProfit, 2)
	salesReturn.NetProfit = RoundFloat(((totalProfit - salesReturn.CashDiscount) - salesReturn.Discount), 2)
	salesReturn.Loss = totalLoss
	salesReturn.NetLoss = totalLoss
	if salesReturn.NetProfit < 0 {
		salesReturn.NetLoss += (salesReturn.NetProfit * -1)
		salesReturn.NetProfit = 0.00
	}

	return nil
}

func (salesReturn *SalesReturn) MakeCode() error {
	lastSalesReturn, err := FindLastSalesReturnByStoreID(salesReturn.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if lastSalesReturn == nil {
		store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
		if err != nil {
			return err
		}
		salesReturn.Code = store.Code + "-200000"
	} else {
		splits := strings.Split(lastSalesReturn.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			salesReturn.Code = storeCode + "-" + strconv.Itoa(codeInt)
		}
	}

	for {
		exists, err := salesReturn.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastSalesReturn.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++
		salesReturn.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func FindLastSalesReturnByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesReturn *SalesReturn, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&salesReturn)
	if err != nil {
		return nil, err
	}

	return salesReturn, err
}

func (salesReturn *SalesReturn) UpdateOrderReturnDiscount(salesReturnOld *SalesReturn) error {
	order, err := FindOrderByID(salesReturn.OrderID, bson.M{})
	if err != nil {
		return err
	}

	if salesReturnOld != nil {
		order.ReturnDiscount -= salesReturnOld.Discount
	}

	order.ReturnDiscount += salesReturn.Discount
	return order.Update()
}

func (salesReturn *SalesReturn) UpdateOrderReturnCount() (count int64, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	returnCount, err := collection.CountDocuments(ctx, bson.M{
		"order_id": salesReturn.OrderID,
		"deleted":  bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}

	order, err := FindOrderByID(salesReturn.OrderID, bson.M{})
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

func (salesReturn *SalesReturn) UpdateOrderReturnCashDiscount(salesReturnOld *SalesReturn) error {
	order, err := FindOrderByID(salesReturn.OrderID, bson.M{})
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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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

	return (count == 1), err
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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
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

func FindSalesReturnByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (salesreturn *SalesReturn, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
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
		salesreturn.Customer, _ = FindCustomerByID(salesreturn.CustomerID, fields)
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

func IsSalesReturnExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func (salesreturn *SalesReturn) HardDelete() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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
	totalCount, err := GetTotalCount(bson.M{}, "salesreturn")
	if err != nil {
		return err
	}
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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
		salesReturn := SalesReturn{}
		err = cur.Decode(&salesReturn)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = salesReturn.ClearProductsSalesReturnHistory()
		if err != nil {
			return err
		}

		err = salesReturn.CreateProductsSalesReturnHistory()
		if err != nil {
			return err
		}

		/*
			err = salesReturn.CalculateSalesReturnProfit()
			if err != nil {
				return err
			}



			salesReturn.GetPayments()

			err = salesReturn.SetProductsSalesReturnStats()
			if err != nil {
				return err
			}
		*/

		/*
			err = salesReturn.SetCustomerSalesReturnStats()
			if err != nil {
				return err
			}
		*/

		/*
			err = salesReturn.CalculateSalesReturnProfit()
			if err != nil {
				return err
			}
		*/

		//salesReturn.UpdateOrderReturnCount()

		/*
				salesReturn.GetPayments()

				err = salesReturn.UndoAccounting()
				if err != nil {
					return errors.New("error undo accounting: " + err.Error())
				}

				err = salesReturn.DoAccounting()
				if err != nil {
					return errors.New("error doing accounting: " + err.Error())
				}


			err = salesReturn.Update()
			if err != nil {
				return err
			}

			/*
				if salesReturn.Code == "GUOJ-200042" {
					err = salesReturn.HardDelete()
					if err != nil {
						return err
					}
				}
		*/
		bar.Add(1)
	}
	log.Print("Sales Returns DONE!")
	return nil
}

func (salesReturn *SalesReturn) GetPayments() (payments []SalesReturnPayment, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
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

		totalPaymentPaid += *model.Amount

		if !slices.Contains(paymentMethods, model.Method) {
			paymentMethods = append(paymentMethods, model.Method)
		}
	} //end for loop

	salesReturn.TotalPaymentPaid = ToFixed(totalPaymentPaid, 2)
	//salesReturn.BalanceAmount = ToFixed(salesReturn.NetTotal-totalPaymentPaid, 2)
	salesReturn.BalanceAmount = ToFixed((salesReturn.NetTotal-salesReturn.CashDiscount)-totalPaymentPaid, 2)
	salesReturn.PaymentMethods = paymentMethods
	salesReturn.Payments = payments
	salesReturn.PaymentsCount = int64(len(payments))

	if ToFixed((salesReturn.NetTotal-salesReturn.CashDiscount), 2) <= ToFixed(totalPaymentPaid, 2) {
		salesReturn.PaymentStatus = "paid"
	} else if ToFixed(totalPaymentPaid, 2) > 0 {
		salesReturn.PaymentStatus = "paid_partially"
	} else if ToFixed(totalPaymentPaid, 2) <= 0 {
		salesReturn.PaymentStatus = "not_paid"
	}

	return payments, err
}

func (salesReturn *SalesReturn) RemoveStock() (err error) {
	if len(salesReturn.Products) == 0 {
		return nil
	}

	for _, salesReturnProduct := range salesReturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		product, err := FindProductByID(&salesReturnProduct.ProductID, bson.M{})
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

		err = product.Update()
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
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"sales_return_id": salesReturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func (salesReturn *SalesReturn) GetPaymentsCount() (count int64, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_return_payment")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_return_history")
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

	err = product.Update()
	if err != nil {
		return err
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

func (salesReturn *SalesReturn) SetProductsSalesReturnStats() error {
	for _, salesReturnProduct := range salesReturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		product, err := FindProductByID(&salesReturnProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductSalesReturnStatsByStoreID(*salesReturn.StoreID)
		if err != nil {
			return err
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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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

	customer, err := FindCustomerByID(salesReturn.CustomerID, map[string]interface{}{})
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
		totalSalesReturnPaidAmount += *payment.Amount
		if totalSalesReturnPaidAmount > (salesReturn.NetTotal - salesReturn.CashDiscount) {
			extraSalesReturnAmountPaid = RoundFloat((totalSalesReturnPaidAmount - (salesReturn.NetTotal - salesReturn.CashDiscount)), 2)
		}
		amount := *payment.Amount

		if extraSalesReturnAmountPaid > 0 {
			skip := false
			if extraSalesReturnAmountPaid < *payment.Amount {
				amount = RoundFloat((*payment.Amount - extraSalesReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraSalesReturnAmountPaid = 0
			} else if extraSalesReturnAmountPaid >= *payment.Amount {
				skip = true
				extraSalesReturnAmountPaid = RoundFloat((extraSalesReturnAmountPaid - *payment.Amount), 2)
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
		customerAccount, err := CreateAccountIfNotExists(
			salesReturn.StoreID,
			&customer.ID,
			&referenceModel,
			customer.Name,
			&customer.Phone,
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
		totalSalesReturnPaidAmount += *payment.Amount
		if totalSalesReturnPaidAmount > (salesReturn.NetTotal - salesReturn.CashDiscount) {
			extraSalesReturnAmountPaid = RoundFloat((totalSalesReturnPaidAmount - (salesReturn.NetTotal - salesReturn.CashDiscount)), 2)
		}
		amount := *payment.Amount

		if extraSalesReturnAmountPaid > 0 {
			skip := false
			if extraSalesReturnAmountPaid < *payment.Amount {
				extraAmount := extraSalesReturnAmountPaid
				extraSalesReturnPayments = append(extraSalesReturnPayments, SalesReturnPayment{
					Date:   payment.Date,
					Amount: &extraAmount,
					Method: payment.Method,
				})
				amount = RoundFloat((*payment.Amount - extraSalesReturnAmountPaid), 2)
				//totalPaidAmount -= *payment.Amount
				//totalPaidAmount += amount
				extraSalesReturnAmountPaid = 0
			} else if extraSalesReturnAmountPaid >= *payment.Amount {
				extraSalesReturnPayments = append(extraSalesReturnPayments, SalesReturnPayment{
					Date:   payment.Date,
					Amount: payment.Amount,
					Method: payment.Method,
				})

				skip = true
				extraSalesReturnAmountPaid = RoundFloat((extraSalesReturnAmountPaid - *payment.Amount), 2)
			}

			if skip {
				continue
			}

		}

		cashPayingAccount := Account{}
		if payment.Method == "cash" {
			cashPayingAccount = *cashAccount
		} else if payment.Method == "bank_account" {
			cashPayingAccount = *bankAccount
		} else if payment.Method == "customer_account" {
			referenceModel := "customer"
			customerAccount, err := CreateAccountIfNotExists(
				salesReturn.StoreID,
				&customer.ID,
				&referenceModel,
				customer.Name,
				&customer.Phone,
			)
			if err != nil {
				return nil, err
			}
			cashPayingAccount = *customerAccount
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
		customerAccount, err := CreateAccountIfNotExists(
			salesReturn.StoreID,
			&customer.ID,
			&referenceModel,
			customer.Name,
			&customer.Phone,
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
	now := time.Now()
	journals := []Journal{}
	groupID := primitive.NewObjectID()

	var lastPaymentDate *time.Time
	if len(extraPayments) > 0 {
		lastPaymentDate = extraPayments[len(extraPayments)-1].Date
	}

	referenceModel := "customer"
	customerAccount, err := CreateAccountIfNotExists(
		salesReturn.StoreID,
		&customer.ID,
		&referenceModel,
		customer.Name,
		&customer.Phone,
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
		if payment.Method == "cash" {
			cashPayingAccount = *cashAccount
		} else if payment.Method == "bank_account" {
			cashPayingAccount = *bankAccount
		} else if payment.Method == "customer_account" {
			referenceModel := "customer"
			customerAccount, err := CreateAccountIfNotExists(
				salesReturn.StoreID,
				&customer.ID,
				&referenceModel,
				customer.Name,
				&customer.Phone,
			)
			if err != nil {
				return nil, err
			}
			cashPayingAccount = *customerAccount
		}

		journals = append(journals, Journal{
			Date:          payment.Date,
			AccountID:     cashPayingAccount.ID,
			AccountNumber: cashPayingAccount.Number,
			AccountName:   cashPayingAccount.Name,
			DebitOrCredit: "credit",
			Credit:        *payment.Amount,
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
	now := time.Now()

	customer, err := FindCustomerByID(salesReturn.CustomerID, bson.M{})
	if err != nil {
		return nil, err
	}

	cashAccount, err := CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Cash", nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Bank", nil)
	if err != nil {
		return nil, err
	}

	salesReturnAccount, err := CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Sales Return", nil)
	if err != nil {
		return nil, err
	}

	cashDiscountReceivedAccount, err := CreateAccountIfNotExists(salesReturn.StoreID, nil, nil, "Cash discount received", nil)
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
		customerAccount, err := CreateAccountIfNotExists(
			salesReturn.StoreID,
			&customer.ID,
			&referenceModel,
			customer.Name,
			&customer.Phone,
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

func (salesReturn *SalesReturn) UndoAccounting() error {
	ledger, err := FindLedgerByReferenceID(salesReturn.ID, *salesReturn.StoreID, bson.M{})
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

	err = RemoveLedgerByReferenceID(salesReturn.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = RemovePostingsByReferenceID(salesReturn.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}
