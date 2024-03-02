package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/exp/slices"
	"gopkg.in/mgo.v2/bson"
)

type OrderProduct struct {
	ProductID         primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name              string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic      string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode          string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber        string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity          float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	QuantityReturned  float64            `json:"quantity_returned" bson:"quantity_returned"`
	UnitPrice         float64            `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	PurchaseUnitPrice float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	Unit              string             `bson:"unit,omitempty" json:"unit,omitempty"`
	Profit            float64            `bson:"profit" json:"profit"`
	Loss              float64            `bson:"loss" json:"loss"`
}

type OrderPayment struct {
	PaymentID     primitive.ObjectID  `json:"payment_id,omitempty" bson:"payment_id,omitempty"`
	Amount        float64             `json:"amount" bson:"amount"`
	Method        string              `json:"method" bson:"method"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
}

// Order : Order structure
type Order struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                  string              `json:"date_str,omitempty"`
	Code                     string              `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                  *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID               *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Store                    *Store              `json:"store,omitempty"`
	Customer                 *Customer           `json:"customer,omitempty"`
	Products                 []OrderProduct      `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy              *primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredByUser          *User               `json:"delivered_by_user,omitempty"`
	DeliveredBySignatureID   *primitive.ObjectID `json:"delivered_by_signature_id,omitempty" bson:"delivered_by_signature_id,omitempty"`
	DeliveredBySignatureName string              `json:"delivered_by_signature_name,omitempty" bson:"delivered_by_signature_name,omitempty"`
	DeliveredBySignature     *Signature          `json:"delivered_by_signature,omitempty"`
	SignatureDate            *time.Time          `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	SignatureDateStr         string              `json:"signature_date_str,omitempty"`
	VatPercent               *float64            `bson:"vat_percent" json:"vat_percent"`
	Discount                 float64             `bson:"discount" json:"discount"`
	ReturnDiscount           float64             `bson:"return_discount" json:"return_discount"`
	DiscountPercent          float64             `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent        bool                `bson:"is_discount_percent" json:"is_discount_percent"`
	Status                   string              `bson:"status,omitempty" json:"status,omitempty"`
	ShippingOrHandlingFees   float64             `bson:"shipping_handling_fees" json:"shipping_handling_fees"`
	TotalQuantity            float64             `bson:"total_quantity" json:"total_quantity"`
	VatPrice                 float64             `bson:"vat_price" json:"vat_price"`
	Total                    float64             `bson:"total" json:"total"`
	NetTotal                 float64             `bson:"net_total" json:"net_total"`
	CashDiscount             float64             `bson:"cash_discount" json:"cash_discount"`
	PartiaPaymentAmount      float64             `bson:"partial_payment_amount" json:"partial_payment_amount"`
	PaymentMethod            string              `bson:"payment_method" json:"payment_method"`
	PaymentMethods           []string            `json:"payment_methods" bson:"payment_methods"`
	TotalPaymentReceived     float64             `bson:"total_payment_received" json:"total_payment_received"`
	BalanceAmount            float64             `bson:"balance_amount" json:"balance_amount"`
	Payments                 []SalesPayment      `bson:"payments" json:"payments"`
	PaymentsInput            []SalesPayment      `json:"payments_input"`
	PaymentsCount            int64               `bson:"payments_count" json:"payments_count"`
	PaymentStatus            string              `bson:"payment_status" json:"payment_status"`
	Profit                   float64             `bson:"profit" json:"profit"`
	NetProfit                float64             `bson:"net_profit" json:"net_profit"`
	Loss                     float64             `bson:"loss" json:"loss"`
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
	DeliveredByName          string              `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CustomerName             string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName                string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName            string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName            string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName            string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
}

func UpdateOrderProfit() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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

func (order *Order) UpdateForeignLabelFields() error {

	if order.StoreID != nil {
		store, err := FindStoreByID(order.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.StoreName = store.Name
	}

	if order.CustomerID != nil {
		customer, err := FindCustomerByID(order.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.CustomerName = customer.Name
	}

	if order.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(order.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.DeliveredByName = deliveredByUser.Name
	}

	if order.DeliveredBySignatureID != nil {
		deliveredBySignature, err := FindSignatureByID(order.DeliveredBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.DeliveredBySignatureName = deliveredBySignature.Name
	}

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

	if order.DeletedBy != nil && !order.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(order.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		order.DeletedByName = deletedByUser.Name
	}

	for i, product := range order.Products {
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1})
		if err != nil {
			return err
		}
		order.Products[i].Name = productObject.Name
		order.Products[i].NameInArabic = productObject.NameInArabic
		order.Products[i].ItemCode = productObject.ItemCode
		order.Products[i].PartNumber = productObject.PartNumber
	}

	return nil
}

func (order *Order) FindNetTotal() {
	netTotal := float64(0.0)
	for _, product := range order.Products {
		netTotal += (float64(product.Quantity) * product.UnitPrice)
	}

	netTotal -= order.Discount
	netTotal += order.ShippingOrHandlingFees

	if order.VatPercent != nil {
		//netTotal += netTotal * (*order.VatPercent / float64(100))
		netTotal += ((netTotal * *order.VatPercent) / float64(100))
	}

	log.Print("netTotal:")
	log.Print(netTotal)
	order.NetTotal = RoundFloat(netTotal, 2)
	//order.NetTotal = RoundToTwoDecimal(netTotal)
	log.Print("order.NetTotal")
	//
	log.Print(order.NetTotal)
}

func (order *Order) FindTotal() {
	total := float64(0.0)
	for _, product := range order.Products {
		total += (float64(product.Quantity) * product.UnitPrice)
	}

	order.Total = RoundFloat(total, 2)
}

func (order *Order) FindTotalQuantity() {
	totalQuantity := float64(0.0)
	for _, product := range order.Products {
		totalQuantity += product.Quantity
	}
	order.TotalQuantity = totalQuantity
}

func (order *Order) FindVatPrice() {
	vatPrice := ((*order.VatPercent / 100) * float64(order.Total-order.Discount+order.ShippingOrHandlingFees))
	vatPrice = RoundFloat(vatPrice, 2)
	order.VatPrice = vatPrice
}

type SalesStats struct {
	ID                     *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal               float64             `json:"net_total" bson:"net_total"`
	NetProfit              float64             `json:"net_profit" bson:"net_profit"`
	Loss                   float64             `json:"loss" bson:"loss"`
	VatPrice               float64             `json:"vat_price" bson:"vat_price"`
	Discount               float64             `json:"discount" bson:"discount"`
	ShippingOrHandlingFees float64             `json:"shipping_handling_fees" bson:"shipping_handling_fees"`
	PaidSales              float64             `json:"paid_sales" bson:"paid_sales"`
	UnPaidSales            float64             `json:"unpaid_sales" bson:"unpaid_sales"`
	CashSales              float64             `json:"cash_sales" bson:"cash_sales"`
	BankAccountSales       float64             `json:"bank_account_sales" bson:"bank_account_sales"`
	CashDiscount           float64             `json:"cash_discount" bson:"cash_discount"`
}

func GetSalesStats(filter map[string]interface{}) (stats SalesStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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
				"loss":                   bson.M{"$sum": "$loss"},
				"vat_price":              bson.M{"$sum": "$vat_price"},
				"discount":               bson.M{"$sum": "$discount"},
				"cash_discount":          bson.M{"$sum": "$cash_discount"},
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
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
	}
	return stats, nil
}

func GetAllOrders() (orders []Order, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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

func SearchOrder(w http.ResponseWriter, r *http.Request) (orders []Order, criterias SearchCriterias, err error) {

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

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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
			order.Customer, _ = FindCustomerByID(order.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			order.CreatedByUser, _ = FindUserByID(order.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			order.UpdatedByUser, _ = FindUserByID(order.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			order.DeletedByUser, _ = FindUserByID(order.DeletedBy, deletedByUserSelectFields)
		}
		orders = append(orders, order)
	} //end for loop

	return orders, criterias, nil
}

func (order *Order) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldOrder *Order) (errs map[string]string) {

	errs = make(map[string]string)

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

	if order.CashDiscount >= order.NetTotal {
		errs["cash_discount"] = "Cash discount should not be >= " + fmt.Sprintf("%.02f", order.NetTotal)
	}

	totalPayment := float64(0.00)
	for _, payment := range order.PaymentsInput {
		if payment.Amount != nil {
			totalPayment += *payment.Amount
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

			if order.Date != nil && order.PaymentsInput[index].Date.Before(*order.Date) {
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

			maxAllowedAmount := (order.NetTotal - order.CashDiscount) - (totalPayment - *payment.Amount)

			if maxAllowedAmount < 0 {
				maxAllowedAmount = 0
			}

			if maxAllowedAmount == 0 {
				errs["payment_amount_"+strconv.Itoa(index)] = "Total amount should not exceed " + fmt.Sprintf("%.02f", (order.NetTotal-order.CashDiscount)) + ", Please delete this payment"
			} else if *payment.Amount > maxAllowedAmount {
				errs["payment_amount_"+strconv.Itoa(index)] = "Amount should not be greater than " + fmt.Sprintf("%.02f", (maxAllowedAmount)) + ", Please delete or edit this payment"
			}
		}

	}

	/*
		if govalidator.IsNull(order.PaymentStatus) {
			errs["payment_status"] = "Payment status is required"
		}
	*/

	if !govalidator.IsNull(order.SignatureDateStr) {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, order.SignatureDateStr)
		if err != nil {
			errs["signature_date_str"] = "Invalid date format"
		}
		order.SignatureDate = &date
	}

	if scenario == "update" {

		if order.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsOrderExists(&order.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Order:" + order.ID.Hex()
		}

	} else {
		if order.PaymentStatus != "not_paid" {
			/*
				if govalidator.IsNull(order.PaymentMethod) {
					errs["payment_method"] = "Payment method is required"
				}
			*/
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

	if order.CustomerID == nil || order.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(order.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + order.CustomerID.Hex()
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

	for index, product := range order.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for order"
		} else {
			exists, err := IsProductExists(&product.ProductID)
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

	if order.PaymentMethod == "customer_account" &&
		order.PaymentStatus != "not_paid" &&
		scenario != "update" {

		log.Print("Checking customer account Balance")
		customer, err := FindCustomerByID(order.CustomerID, bson.M{})
		if err != nil {
			errs["customer_id"] = "Invalid Customer:" + order.CustomerID.Hex()
		}

		referenceModel := "customer"
		customerAccount, err := CreateAccountIfNotExists(
			order.StoreID,
			&customer.ID,
			&referenceModel,
			customer.Name,
			&customer.Phone,
		)
		if err != nil {
			errs["customer_account"] = "Error creating customer account: " + err.Error()
		}

		customerBalance := customerAccount.Balance
		log.Print(customerBalance)
		accountType := customerAccount.Type

		if customerBalance == 0 {
			errs["payment_method"] = "customer account balance is zero"
		} else if accountType == "asset" {
			errs["payment_method"] = "customer owe us: " + fmt.Sprintf("%.02f", customerBalance)
		} else if accountType == "liability" && order.PaymentStatus == "paid" && customerBalance < order.NetTotal {
			errs["payment_method"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerBalance) + ", total: " + fmt.Sprintf("%.02f", order.NetTotal)
		} else if accountType == "liability" && order.PaymentStatus == "paid_partially" && customerBalance < order.PartiaPaymentAmount {
			errs["payment_method"] = "customer account balance is only: " + fmt.Sprintf("%.02f", customerBalance)
		}

		/*
			spendingAccount := &Account{}
			spendingAccountName := ""
			if *order.PayFromAccount == "cash_account" {
				cashAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Cash", nil)
				if err != nil {
					errs["pay_from_account"] = "error fetching cash account"
				}
				spendingAccount = cashAccount
				spendingAccountName = "cash"

			} else if *order.PayFromAccount == "bank_account" {
				bankAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Bank", nil)
				if err != nil {
					errs["pay_from_account"] = "error fetching bank account"
				}
				spendingAccount = bankAccount
				spendingAccountName = "bank"
			}

			if spendingAccount.Balance == 0 {
				errs["pay_from_account"] = spendingAccountName + " account balance is zero"
			} else if order.PaymentStatus == "paid" && spendingAccount.Balance < order.NetTotal {
				errs["pay_from_account"] = spendingAccountName + " account balance is only: " + fmt.Sprintf("%.02f", spendingAccount.Balance)
			} else if order.PaymentStatus == "paid_partially" && spendingAccount.Balance < order.PartiaPaymentAmount {
				errs["pay_from_account"] = spendingAccountName + " account balance is only: " + fmt.Sprintf("%.02f", spendingAccount.Balance)
			}
		*/
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func GetProductStockInStore(
	productID *primitive.ObjectID,
	storeID *primitive.ObjectID,
) (stock float64, err error) {
	product, err := FindProductByID(productID, bson.M{})
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
	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := FindProductByID(&orderProduct.ProductID, bson.M{})
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

		err = product.Update()
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
	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := FindProductByID(&orderProduct.ProductID, bson.M{})
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

		err = product.Update()
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

		salesPrice := quantity * orderProduct.UnitPrice
		purchaseUnitPrice := orderProduct.PurchaseUnitPrice

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

		}

		profit := 0.0
		if purchaseUnitPrice > 0 {
			profit = salesPrice - (quantity * purchaseUnitPrice)
		}

		loss := 0.0

		profit = RoundFloat(profit, 2)

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
	order.Profit = RoundFloat(totalProfit, 2)
	order.NetProfit = RoundFloat(((totalProfit - order.CashDiscount) - order.Discount), 2)
	order.Loss = totalLoss
	return nil
}

func (order *Order) GenerateCode(startFrom int, storeCode string) (string, error) {
	count, err := GetTotalCount(bson.M{"store_id": order.StoreID}, "order")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (order *Order) ClearPayments() error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"order_id": order.ID})
	if err != nil {
		return err
	}
	return nil
}

func (order *Order) GetPaymentsCount() (count int64, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"order_id": order.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (order *Order) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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
			salesPayment, err := FindSalesPaymentByID(&payment.ID, bson.M{})
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

	collection := db.Client().Database(db.GetPosDB()).Collection("sales_payment")
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

	if ToFixed((order.NetTotal-order.CashDiscount), 2) == ToFixed(totalPaymentReceived, 2) {
		order.PaymentStatus = "paid"
	} else if ToFixed(totalPaymentReceived, 2) > 0 {
		order.PaymentStatus = "paid_partially"
		order.PartiaPaymentAmount = totalPaymentReceived
	} else if ToFixed(totalPaymentReceived, 2) <= 0 {
		order.PaymentStatus = "not_paid"
	}

	return models, err
}

func (order *Order) MakeCode() error {
	lastOrder, err := FindLastOrderByStoreID(order.StoreID, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}
	if lastOrder == nil {
		store, err := FindStoreByID(order.StoreID, bson.M{})
		if err != nil {
			return err
		}
		order.Code = store.Code + "-100000"
	} else {
		splits := strings.Split(lastOrder.Code, "-")
		if len(splits) == 2 {
			storeCode := splits[0]
			codeStr := splits[1]
			codeInt, err := strconv.Atoi(codeStr)
			if err != nil {
				return err
			}
			codeInt++
			order.Code = storeCode + "-" + strconv.Itoa(codeInt)
			log.Printf("New code: %s", order.Code)
		}
	}

	for {
		exists, err := order.IsCodeExists()
		if err != nil {
			return err
		}
		if !exists {
			break
		}

		splits := strings.Split(lastOrder.Code, "-")
		storeCode := splits[0]
		codeStr := splits[1]
		codeInt, err := strconv.Atoi(codeStr)
		if err != nil {
			return err
		}
		codeInt++

		order.Code = storeCode + "-" + strconv.Itoa(codeInt)
	}

	return nil
}

func FindLastOrderByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (order *Order, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

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

	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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

	return (count == 1), err
}

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

	count, err := GetTotalCount(bson.M{}, "order")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return strconv.Itoa(code + 1), nil
}

func (order *Order) UpdateOrderStatus(status string) (*Order, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	order.Deleted = true
	order.DeletedBy = &userID
	now := time.Now()
	order.DeletedAt = &now

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

func FindOrderByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (order *Order, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
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
		order.Customer, _ = FindCustomerByID(order.CustomerID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		order.CreatedByUser, _ = FindUserByID(order.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		order.UpdatedByUser, _ = FindUserByID(order.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		order.DeletedByUser, _ = FindUserByID(order.DeletedBy, fields)
	}

	return order, err
}

func IsOrderExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
func (order *Order) HardDelete() error {
	log.Print("Delete order")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
	_, err := collection.DeleteOne(ctx, bson.M{
		"order_id": order.ID,
	})
	if err != nil {
		return err
	}

	return nil
}

func ProcessOrders() error {
	log.Print("Processing orders")
	//ledgersCount := 0
	//cashOrdersCount := 0
	//postingsCount := 0

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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

	//productCount := 1
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

		/*
			err = order.CalculateOrderProfit()
			if err != nil {
				return err
			}

			err = order.ClearProductsSalesHistory()
			if err != nil {
				return err
			}

			err = order.CreateProductsSalesHistory()
			if err != nil {
				return err
			}

			order.GetPayments()
		*/

		/*
				err = order.SetProductsSalesStats()
				if err != nil {
					return err
				}


			err = order.SetCustomerSalesStats()
			if err != nil {
				return err
			}
		*/

		/*
			_, err = order.GetPayments()
			if err != nil {
				return err
			}
		*/

		err = order.CalculateOrderProfit()
		if err != nil {
			return err
		}

		order.GetPayments()
		order.Update()

		err = order.UndoAccounting()
		if err != nil {
			return errors.New("error undo accounting: " + err.Error())
		}

		err = order.DoAccounting()
		if err != nil {
			return errors.New("error doing accounting: " + err.Error())
		}

		err = order.Update()
		if err != nil {
			return err
		}

		/*
			if order.StoreID.Hex() != "61cf42e580e87d715a4cb9e6" {
				continue
			}

			for _, method := range order.PaymentMethods {
				if method == "cash" {
					cashOrdersCount++
					ledgerCount, _ := GetTotalCount(bson.M{"reference_id": order.ID}, "ledger")
					if ledgerCount > 1 {
						log.Print("More than 1")
						log.Print(ledgerCount)
						log.Print(order.Code)
					}

					if ledgerCount == 0 {
						log.Print("No ledger found")
						log.Print(ledgerCount)
						log.Print(order.Code)
					}

					if ledgerCount > 0 {
						ledgersCount++
					}

					postingCount, _ := GetTotalCount(bson.M{"reference_id": order.ID, "account_number": "1001"}, "posting")
					if postingCount > 0 {
						postingsCount++
					}

					if postingCount == 0 {
						log.Print("No posting found")
						log.Print(postingCount)
						log.Print(order.Code)
					}

					if postingCount > 1 {
						log.Print("More than 1")
						log.Print(postingCount)
						log.Print(order.Code)
					}
				}
			}
		*/

	}

	/*
		log.Print("Ledger count: ")
		log.Print(ledgersCount)
		log.Print("Cash orders: ")
		log.Print(cashOrdersCount)

		log.Print("postings count: ")
		log.Print(postingsCount)
	*/

	log.Print("DONE!")
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
	collection := db.Client().Database(db.GetPosDB()).Collection("product_sales_history")
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

	err = product.Update()
	if err != nil {
		return err
	}

	return nil
}

func (order *Order) SetProductsSalesStats() error {
	for _, orderProduct := range order.Products {
		product, err := FindProductByID(&orderProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductSalesStatsByStoreID(*order.StoreID)
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
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
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

	customer, err := FindCustomerByID(order.CustomerID, map[string]interface{}{})
	if err != nil {
		return err
	}

	err = customer.SetCustomerSalesStatsByStoreID(*order.StoreID)
	if err != nil {
		return err
	}

	return nil
}

func MakeJournalsForUnpaidSale(
	order *Order,
	customerAccount *Account,
	salesAccount *Account,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()
	groupAccounts := []string{customerAccount.Number, salesAccount.Number}
	if order.CashDiscount > 0 {
		groupAccounts = append(groupAccounts, cashDiscountAllowedAccount.Number)
	}

	journals := []Journal{}

	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         (order.NetTotal - order.CashDiscount),
		GroupAccounts: groupAccounts,
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
			GroupAccounts: groupAccounts,
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
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func MakeJournalsForPaidSaleWithSinglePayment(
	order *Order,
	cashReceivingAccount *Account,
	salesAccount *Account,
	payment *SalesPayment,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()
	groupAccounts := []string{cashReceivingAccount.Number, salesAccount.Number}
	if order.CashDiscount > 0 {
		groupAccounts = append(groupAccounts, cashDiscountAllowedAccount.Number)
	}
	journals := []Journal{}
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     cashReceivingAccount.ID,
		AccountNumber: cashReceivingAccount.Number,
		AccountName:   cashReceivingAccount.Name,
		DebitOrCredit: "debit",
		Debit:         *payment.Amount,
		GroupAccounts: groupAccounts,
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
			GroupAccounts: groupAccounts,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     salesAccount.ID,
		AccountNumber: salesAccount.Number,
		AccountName:   salesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        order.NetTotal,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func MakeJournalsForPartialSalePayment(
	order *Order,
	customerAccount *Account,
	cashReceivingAccount *Account,
	salesAccount *Account,
	payment *SalesPayment,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()
	groupAccounts := []string{customerAccount.Number, cashReceivingAccount.Number, salesAccount.Number}
	if order.CashDiscount > 0 {
		groupAccounts = append(groupAccounts, cashDiscountAllowedAccount.Number)
	}
	balanceAmount := RoundFloat(((order.NetTotal - order.CashDiscount) - *payment.Amount), 2)
	journals := []Journal{}
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     cashReceivingAccount.ID,
		AccountNumber: cashReceivingAccount.Number,
		AccountName:   cashReceivingAccount.Name,
		DebitOrCredit: "debit",
		Debit:         *payment.Amount,
		GroupAccounts: groupAccounts,
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
			GroupAccounts: groupAccounts,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	//Asset or debt increased
	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         balanceAmount,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	//Sales account increased
	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     salesAccount.ID,
		AccountNumber: salesAccount.Number,
		AccountName:   salesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        order.NetTotal,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func MakeJournalsForNewSalePayment(
	order *Order,
	customerAccount *Account,
	cashReceivingAccount *Account,
	payment *SalesPayment,
) []Journal {
	now := time.Now()
	groupAccounts := []string{cashReceivingAccount.Number, customerAccount.Number}

	journals := []Journal{}
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     cashReceivingAccount.ID,
		AccountNumber: cashReceivingAccount.Number,
		AccountName:   cashReceivingAccount.Name,
		DebitOrCredit: "debit",
		Debit:         *payment.Amount,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	//Asset or debt descreased
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "credit",
		Credit:        *payment.Amount,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

// customer account journals
func MakeJournalsForPartialSalePaymentFromCustomerAccount(
	order *Order,
	customerAccount *Account,
	salesAccount *Account,
	payment *SalesPayment,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()
	groupAccounts := []string{customerAccount.Number, salesAccount.Number}
	if order.CashDiscount > 0 {
		groupAccounts = append(groupAccounts, cashDiscountAllowedAccount.Number)
	}
	balanceAmount := RoundFloat(((order.NetTotal - order.CashDiscount) - *payment.Amount), 2)
	journals := []Journal{}
	//Debtor acc up

	//Liability or account balance decrease
	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         *payment.Amount,
		GroupAccounts: groupAccounts,
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
			GroupAccounts: groupAccounts,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	//Debt or asset increase
	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         balanceAmount,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	//Sales increase
	journals = append(journals, Journal{
		Date:          order.Date,
		AccountID:     salesAccount.ID,
		AccountNumber: salesAccount.Number,
		AccountName:   salesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        order.NetTotal,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func MakeJournalsForPaidSaleWithSinglePaymentFromCustomerAccount(
	order *Order,
	customerAccount *Account,
	salesAccount *Account,
	payment *SalesPayment,
	cashDiscountAllowedAccount *Account,
) []Journal {
	now := time.Now()
	groupAccounts := []string{customerAccount.Number, salesAccount.Number}
	if order.CashDiscount > 0 {
		groupAccounts = append(groupAccounts, cashDiscountAllowedAccount.Number)
	}
	journals := []Journal{}
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         *payment.Amount,
		GroupAccounts: groupAccounts,
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
			GroupAccounts: groupAccounts,
			CreatedAt:     &now,
			UpdatedAt:     &now,
		})

	}

	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     salesAccount.ID,
		AccountNumber: salesAccount.Number,
		AccountName:   salesAccount.Name,
		DebitOrCredit: "credit",
		Credit:        order.NetTotal,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

func MakeJournalsForNewSalePaymentFromCustomerAccount(
	order *Order,
	customerAccount *Account,
	payment *SalesPayment,
) []Journal {
	now := time.Now()
	groupAccounts := []string{customerAccount.Number}
	journals := []Journal{}

	//Account balance or liability decrease
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "debit",
		Debit:         *payment.Amount,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	//Asset or debt decrease
	journals = append(journals, Journal{
		Date:          payment.Date,
		AccountID:     customerAccount.ID,
		AccountNumber: customerAccount.Number,
		AccountName:   customerAccount.Name,
		DebitOrCredit: "credit",
		Credit:        *payment.Amount,
		GroupAccounts: groupAccounts,
		CreatedAt:     &now,
		UpdatedAt:     &now,
	})

	return journals
}

//End customer account journals

func (order *Order) CreateLedger() (ledger *Ledger, err error) {
	now := time.Now()

	customer, err := FindCustomerByID(order.CustomerID, bson.M{})
	if err != nil {
		return nil, err
	}

	cashAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Cash", nil)
	if err != nil {
		return nil, err
	}

	bankAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Bank", nil)
	if err != nil {
		return nil, err
	}

	salesAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Sales", nil)
	if err != nil {
		return nil, err
	}

	cashDiscountAllowedAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Cash discount allowed", nil)
	if err != nil {
		return nil, err
	}

	journals := []Journal{}

	if len(order.Payments) == 0 {
		//Case: UnPaid
		referenceModel := "customer"
		customerAccount, err := CreateAccountIfNotExists(
			order.StoreID,
			&customer.ID,
			&referenceModel,
			customer.Name,
			&customer.Phone,
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
	} else {
		//Case: paid or paid_partially
		paymentNumber := 1
		firstPayment := &SalesPayment{}
		for _, payment := range order.Payments {
			if paymentNumber == 1 {
				firstPayment = &payment
			}

			cashReceivingAccount := Account{}
			if payment.Method == "cash" {
				cashReceivingAccount = *cashAccount
			} else if payment.Method == "bank_account" {
				cashReceivingAccount = *bankAccount
			} else if payment.Method == "customer_account" {
			}

			if firstPayment == nil || firstPayment.Date == nil {
				continue
			}

			if firstPayment.Date.Equal(*order.Date) && len(order.Payments) == 1 && order.PaymentStatus == "paid" {
				//Case: paid with 1 single payment at the time of sale
				if payment.Method == "customer_account" {
					referenceModel := "customer"
					customerAccount, err := CreateAccountIfNotExists(
						order.StoreID,
						&customer.ID,
						&referenceModel,
						customer.Name,
						&customer.Phone,
					)
					if err != nil {
						return nil, err
					}
					journals = append(journals, MakeJournalsForPaidSaleWithSinglePaymentFromCustomerAccount(
						order,
						customerAccount,
						salesAccount,
						&payment,
						cashDiscountAllowedAccount,
					)...)
				} else {
					journals = append(journals, MakeJournalsForPaidSaleWithSinglePayment(
						order,
						&cashReceivingAccount,
						salesAccount,
						&payment,
						cashDiscountAllowedAccount,
					)...)
				}

				break
			} else if firstPayment.Date.Equal(*order.Date) && len(order.Payments) > 0 {
				//Case: paid with 1 single partial payment at the time of sale and completed or not completed with many other payments
				//payment1
				referenceModel := "customer"
				customerAccount, err := CreateAccountIfNotExists(
					order.StoreID,
					&customer.ID,
					&referenceModel,
					customer.Name,
					&customer.Phone,
				)
				if err != nil {
					return nil, err
				}

				if paymentNumber == 1 {
					if payment.Method == "customer_account" {
						journals = append(journals, MakeJournalsForPartialSalePaymentFromCustomerAccount(
							order,
							customerAccount,
							salesAccount,
							&payment,
							cashDiscountAllowedAccount,
						)...)
					} else {
						journals = append(journals, MakeJournalsForPartialSalePayment(
							order,
							customerAccount,
							&cashReceivingAccount,
							salesAccount,
							&payment,
							cashDiscountAllowedAccount,
						)...)
					}
				} else if paymentNumber > 1 {
					//payment2,3 etc
					if payment.Method == "customer_account" {
						journals = append(journals, MakeJournalsForNewSalePaymentFromCustomerAccount(
							order,
							customerAccount,
							&payment,
						)...)
					} else {
						journals = append(journals, MakeJournalsForNewSalePayment(
							order,
							customerAccount,
							&cashReceivingAccount,
							&payment,
						)...)
					}
				}

			} else if !firstPayment.Date.Equal(*order.Date) && len(order.Payments) > 0 {
				//Case: paid with >0 payments after the time of sale
				referenceModel := "customer"
				customerAccount, err := CreateAccountIfNotExists(
					order.StoreID,
					&customer.ID,
					&referenceModel,
					customer.Name,
					&customer.Phone,
				)
				if err != nil {
					return nil, err
				}
				if paymentNumber == 1 {
					//Debtor & Sales acc up
					journals = append(journals, MakeJournalsForUnpaidSale(
						order,
						customerAccount,
						salesAccount,
						cashDiscountAllowedAccount,
					)...)
				}

				if payment.Method == "customer_account" {
					journals = append(journals, MakeJournalsForNewSalePaymentFromCustomerAccount(
						order,
						customerAccount,
						&payment,
					)...)
				} else {
					journals = append(journals, MakeJournalsForNewSalePayment(
						order,
						customerAccount,
						&cashReceivingAccount,
						&payment,
					)...)
				}
			}

			paymentNumber++
		} //end for

	}

	//Check if there is any cash discounts
	/*
		cashDiscounts, err := order.GetCashDiscounts()
		if err != nil {
			return nil, err
		}

		if len(cashDiscounts) > 0 {
			cashDiscountAllowedAccount, err := CreateAccountIfNotExists(order.StoreID, nil, nil, "Cash discount allowed", nil)
			if err != nil {
				return nil, err
			}

			for _, cashDiscunt := range cashDiscounts {
				cashSpendingAccount := Account{}
				if cashDiscunt.Method == "cash" {
					cashSpendingAccount = *cashAccount
				} else if cashDiscunt.Method == "bank_account" {
					cashSpendingAccount = *bankAccount
				} else if cashDiscunt.Method == "customer_account" {
					cashSpendingAccount = *customerAccount
				}

				groupAccounts := []string{cashDiscountAllowedAccount.Number, cashSpendingAccount.Number}

				journals = append(journals, Journal{
					Date:          cashDiscunt.Date,
					AccountID:     cashDiscountAllowedAccount.ID,
					AccountNumber: cashDiscountAllowedAccount.Number,
					AccountName:   cashDiscountAllowedAccount.Name,
					DebitOrCredit: "debit",
					Debit:         cashDiscunt.Amount,
					GroupAccounts: groupAccounts,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
				journals = append(journals, Journal{
					Date:          cashDiscunt.Date,
					AccountID:     cashSpendingAccount.ID,
					AccountNumber: cashSpendingAccount.Number,
					AccountName:   cashSpendingAccount.Name,
					DebitOrCredit: "credit",
					Credit:        cashDiscunt.Amount,
					GroupAccounts: groupAccounts,
					CreatedAt:     &now,
					UpdatedAt:     &now,
				})
			}
		}
	*/

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
	collection := db.Client().Database(db.GetPosDB()).Collection("sales_cash_discount")
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
	ledger, err := FindLedgerByReferenceID(order.ID, *order.StoreID, bson.M{})
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

	err = RemoveLedgerByReferenceID(order.ID)
	if err != nil {
		return errors.New("Error removing ledger by reference id: " + err.Error())
	}

	err = RemovePostingsByReferenceID(order.ID)
	if err != nil {
		return errors.New("Error removing postings by reference id: " + err.Error())
	}

	err = SetAccountBalances(ledgerAccounts)
	if err != nil {
		return errors.New("Error setting account balances: " + err.Error())
	}

	return nil
}
