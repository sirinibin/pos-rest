package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	PartiaPaymentAmount      float64             `bson:"partial_payment_amount" json:"partial_payment_amount"`
	PaymentMethod            string              `bson:"payment_method" json:"payment_method"`
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
	ChangeLog                []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func UpdateOrderProfit() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx := context.Background()
	findOptions := options.Find()

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

func (order *Order) SetChangeLog(
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
	} else if event == "remove_stock" && name != nil {
		description = "Stock of product: " + name.(string) + " reduced from " + fmt.Sprintf("%.02f", oldValue.(float64)) + " to " + fmt.Sprintf("%.02f", newValue.(float64))
	} else if event == "add_stock" && name != nil {
		description = "Stock of product: " + name.(string) + " raised from " + fmt.Sprintf("%.02f", oldValue.(float64)) + " to " + fmt.Sprintf("%.02f", newValue.(float64))
	}

	order.ChangeLog = append(
		order.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
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
		netTotal += netTotal * (*order.VatPercent / float64(100))
	}

	order.NetTotal = math.Round(netTotal*100) / 100
}

func (order *Order) FindTotal() {
	total := float64(0.0)
	for _, product := range order.Products {
		total += (float64(product.Quantity) * product.UnitPrice)
	}

	order.Total = math.Round(total*100) / 100
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
	vatPrice = math.Round(vatPrice*100) / 100
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
				"shipping_handling_fees": bson.M{"$sum": "$shipping_handling_fees"},
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
		stats.NetTotal = math.Round(stats.NetTotal*100) / 100
		stats.NetProfit = math.Round(stats.NetProfit*100) / 100
		stats.Loss = math.Round(stats.Loss*100) / 100
	}
	return stats, nil
}

func GetAllOrders() (orders []Order, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)

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

	if govalidator.IsNull(order.Status) {
		errs["status"] = "Status is required"
	}

	if govalidator.IsNull(order.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if govalidator.IsNull(order.PaymentStatus) {
		errs["payment_status"] = "Payment status is required"
	}

	if govalidator.IsNull(order.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		/*
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, order.DateStr)
			if err != nil {
				errs["date_str"] = "Invalid date format"
			}
			order.Date = &date
		*/

		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, order.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		order.Date = &date
		order.CreatedAt = &date
	}

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

	for _, productStore := range product.Stores {
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

		if len(product.Stores) == 0 {
			store, err := FindStoreByID(order.StoreID, bson.M{})
			if err != nil {
				return err
			}
			productStore := ProductStore{
				StoreID:           *order.StoreID,
				StoreName:         order.StoreName,
				StoreNameInArabic: store.NameInArabic,
				Stock:             float64(0),
			}
			product.Stores = []ProductStore{productStore}
		}

		for k, productStore := range product.Stores {
			if productStore.StoreID.Hex() == order.StoreID.Hex() {
				product.Stores[k].Stock -= (orderProduct.Quantity - orderProduct.QuantityReturned)
				break
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

func (order *Order) AddStock() (err error) {
	if len(order.Products) == 0 {
		return nil
	}

	for _, orderProduct := range order.Products {
		product, err := FindProductByID(&orderProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		storeExistInProductStore := false
		for k, productStore := range product.Stores {
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

		if purchaseUnitPrice == 0 ||
			order.Products[i].Loss > 0 ||
			order.Products[i].Profit <= 0 {
			product, err := FindProductByID(&orderProduct.ProductID, map[string]interface{}{})
			if err != nil {
				return err
			}
			for _, store := range product.Stores {
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

		profit = math.Round(profit*100) / 100

		if profit >= 0 {
			order.Products[i].Profit = profit
			order.Products[i].Loss = 0.0
			totalProfit += order.Products[i].Profit
		} else {
			order.Products[i].Profit = 0
			order.Products[i].Loss = (profit * -1)
			totalLoss += order.Products[i].Loss
		}

	}
	order.Profit = math.Round(totalProfit*100) / 100
	order.NetProfit = math.Round((totalProfit-order.Discount)*100) / 100
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

func (order *Order) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	err = order.CalculateOrderProfit()
	if err != nil {
		return err
	}

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

	err := order.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	order.ID = primitive.NewObjectID()
	if len(order.Code) == 0 {
		err = order.MakeCode()
		if err != nil {
			return err
		}
	}

	err = order.CalculateOrderProfit()
	if err != nil {
		return err
	}

	_, err = collection.InsertOne(ctx, &order)
	if err != nil {
		return err
	}

	err = order.AddProductsSalesHistory()
	if err != nil {
		return err
	}

	err = order.AddPayment()
	if err != nil {
		return err
	}

	return nil
}

func (order *Order) MakeCode() error {
	lastOrder, err := FindLastOrderByStoreID(order.StoreID, bson.M{})
	if err != nil {
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

func (order *Order) AddPayment() error {
	amount := float64(0.0)
	if order.PaymentStatus == "paid" {
		amount = order.NetTotal
	} else if order.PaymentStatus == "paid_partially" {
		amount = order.PartiaPaymentAmount
	} else {
		return nil
	}

	payment := SalesPayment{
		OrderID:       &order.ID,
		OrderCode:     order.Code,
		Amount:        amount,
		Method:        order.PaymentMethod,
		CreatedAt:     order.CreatedAt,
		UpdatedAt:     order.UpdatedAt,
		CreatedBy:     order.CreatedBy,
		CreatedByName: order.CreatedByName,
		UpdatedBy:     order.UpdatedBy,
		UpdatedByName: order.UpdatedByName,
		StoreID:       order.StoreID,
		StoreName:     order.StoreName,
	}
	err := payment.Insert()
	if err != nil {
		return err
	}
	return nil
}

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
	collection := db.Client().Database(db.GetPosDB()).Collection("order")
	ctx := context.Background()
	findOptions := options.Find()

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
			for _, orderProduct := range order.Products {
				if orderProduct.Profit < 0 || orderProduct.Loss > 0 {
					fmt.Printf("\nNo: %d", productCount)
					fmt.Printf("\nOrder ID: %s", order.Code)
					fmt.Printf("\nProduct Part#: %s", orderProduct.PartNumber)
					fmt.Printf("\nProduct Name: %s", orderProduct.Name)
					fmt.Printf("\nProduct Profit Recorded: %.02f", orderProduct.Profit)
					fmt.Printf("\nProduct Loss Recorded: %.02f", orderProduct.Loss)
					fmt.Printf("\nProduct Sold for Unit Price: %.02f", orderProduct.UnitPrice)
					fmt.Printf("\nProduct Purchase Unit Price(Marked when sold): %.02f\n", orderProduct.PurchaseUnitPrice)
					productCount++
				}
			}
		*/
		/*
			if order.Code == "GUO-100050" {
				for k, product := range order.Products {
					if product.ItemCode == "BUF" {
						order.Products[k].Quantity = 2
						order.Products[k].UnitPrice = 3850
					}
				}
				order.FindNetTotal()
				order.FindTotal()
				order.FindTotalQuantity()
				order.FindVatPrice()
			}
		*/

		/*
			if order.Code == "GUOJ-100100" {
				for k, product := range order.Products {
					if product.PartNumber == "BUF" {
						order.Products[k].Quantity = 2
						order.Products[k].UnitPrice = 3850
					}
				}
				order.FindNetTotal()
				order.FindTotal()
				order.FindTotalQuantity()
				order.FindVatPrice()
			}
		*/

		err = order.CalculateOrderProfit()
		if err != nil {
			return err
		}

		err = order.AddProductsSalesHistory()
		if err != nil {
			return err
		}

		/*
			if order.PaymentStatus == "" {
				order.PaymentStatus = "paid"
			}

			if order.PaymentMethod == "" {
				order.PaymentMethod = "cash"
			}

			totalPaymentsCount, err := GetTotalCount(bson.M{"order_id": order.ID}, "sales_payment")
			if err != nil {
				return err
			}

			if totalPaymentsCount == 0 {
				err = order.AddPayment()
				if err != nil {
					return err
				}
			}
		*/

		if order.Code == "GUOJ-101457" {
			for i, product := range order.Products {
				if product.PartNumber == "CRB" {
					order.Products[i].QuantityReturned = 0
				}
			}
		}

		err = order.Update()
		if err != nil {
			return err
		}

		/*
			if order.Code == "GUOJ-100199" {
				err = order.HardDelete()
				if err != nil {
					return err
				}
			}
		*/

	}

	return nil
}
