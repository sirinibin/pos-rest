package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
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
}

// SalesReturn : SalesReturn structure
type SalesReturn struct {
	ID                      primitive.ObjectID   `json:"id,omitempty" bson:"_id,omitempty"`
	OrderID                 *primitive.ObjectID  `json:"order_id,omitempty" bson:"order_id,omitempty"`
	OrderCode               string               `bson:"order_code,omitempty" json:"order_code,omitempty"`
	Date                    *time.Time           `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                 string               `json:"date_str,omitempty"`
	Code                    string               `bson:"code,omitempty" json:"code,omitempty"`
	StoreID                 *primitive.ObjectID  `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID              *primitive.ObjectID  `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Store                   *Store               `json:"store,omitempty"`
	Customer                *Customer            `json:"customer,omitempty"`
	Products                []SalesReturnProduct `bson:"products,omitempty" json:"products,omitempty"`
	ReceivedBy              *primitive.ObjectID  `json:"received_by,omitempty" bson:"received_by,omitempty"`
	ReceivedByUser          *User                `json:"received_by_user,omitempty"`
	ReceivedBySignatureID   *primitive.ObjectID  `json:"received_by_signature_id,omitempty" bson:"received_by_signature_id,omitempty"`
	ReceivedBySignatureName string               `json:"received_by_signature_name,omitempty" bson:"received_by_signature_name,omitempty"`
	ReceivedBySignature     *Signature           `json:"received_by_signature,omitempty"`
	SignatureDate           *time.Time           `bson:"signature_date,omitempty" json:"signature_date,omitempty"`
	SignatureDateStr        string               `json:"signature_date_str,omitempty"`
	VatPercent              *float64             `bson:"vat_percent" json:"vat_percent"`
	Discount                float64              `bson:"discount" json:"discount"`
	DiscountPercent         float64              `bson:"discount_percent" json:"discount_percent"`
	IsDiscountPercent       bool                 `bson:"is_discount_percent" json:"is_discount_percent"`
	Status                  string               `bson:"status,omitempty" json:"status,omitempty"`
	StockAdded              bool                 `bson:"stock_added,omitempty" json:"stock_added,omitempty"`
	TotalQuantity           float64              `bson:"total_quantity" json:"total_quantity"`
	VatPrice                float64              `bson:"vat_price" json:"vat_price"`
	Total                   float64              `bson:"total" json:"total"`
	NetTotal                float64              `bson:"net_total" json:"net_total"`
	PartiaPaymentAmount     float64              `bson:"partial_payment_amount" json:"partial_payment_amount"`
	PaymentMethod           string               `bson:"payment_method" json:"payment_method"`
	PaymentStatus           string               `bson:"payment_status" json:"payment_status"`
	Deleted                 bool                 `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy               *primitive.ObjectID  `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser           *User                `json:"deleted_by_user,omitempty"`
	DeletedAt               *time.Time           `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt               *time.Time           `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt               *time.Time           `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy               *primitive.ObjectID  `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy               *primitive.ObjectID  `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser           *User                `json:"created_by_user,omitempty"`
	UpdatedByUser           *User                `json:"updated_by_user,omitempty"`
	ReceivedByName          string               `json:"received_by_name,omitempty" bson:"received_by_name,omitempty"`
	CustomerName            string               `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName               string               `json:"store_name,omitempty" bson:"store_name,omitempty"`
	CreatedByName           string               `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName           string               `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName           string               `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog               []ChangeLog          `json:"change_log,omitempty" bson:"change_log,omitempty"`
	Profit                  float64              `bson:"profit" json:"profit"`
	NetProfit               float64              `bson:"net_profit" json:"net_profit"`
	Loss                    float64              `bson:"loss" json:"loss"`
}

// DiskQuotaUsageResult payload for disk quota usage
type SalesReturnStats struct {
	ID        *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal  float64             `json:"net_total" bson:"net_total"`
	VatPrice  float64             `json:"vat_price" bson:"vat_price"`
	Discount  float64             `json:"discount" bson:"discount"`
	NetProfit float64             `json:"net_profit" bson:"net_profit"`
	Loss      float64             `json:"loss" bson:"loss"`
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
				"_id":        nil,
				"net_total":  bson.M{"$sum": "$net_total"},
				"vat_price":  bson.M{"$sum": "$vat_price"},
				"discount":   bson.M{"$sum": "$discount"},
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
		stats.NetTotal = math.Round(stats.NetTotal*100) / 100
		stats.NetProfit = math.Round(stats.NetProfit*100) / 100
		stats.Loss = math.Round(stats.Loss*100) / 100
	}
	return stats, nil
}

func (salesreturn *SalesReturn) SetChangeLog(
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
		description = "Stock of product: " + name.(string) + " reduced from " + strconv.Itoa(oldValue.(int)) + " to " + strconv.Itoa(newValue.(int))
	} else if event == "add_stock" && name != nil {
		description = "Stock of product: " + name.(string) + " raised from " + strconv.Itoa(oldValue.(int)) + " to " + strconv.Itoa(newValue.(int))
	}

	salesreturn.ChangeLog = append(
		salesreturn.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
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

	if salesreturn.ReceivedBy != nil {
		receivedByUser, err := FindUserByID(salesreturn.ReceivedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.ReceivedByName = receivedByUser.Name
	}

	if salesreturn.ReceivedBySignatureID != nil {
		receivedBySignature, err := FindSignatureByID(salesreturn.ReceivedBySignatureID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.ReceivedBySignatureName = receivedBySignature.Name
	}

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

	if salesreturn.DeletedBy != nil && !salesreturn.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(salesreturn.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		salesreturn.DeletedByName = deletedByUser.Name
	}

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
		netTotal += (float64(product.Quantity) * product.UnitPrice)
	}

	netTotal -= salesreturn.Discount

	if salesreturn.VatPercent != nil {
		netTotal += netTotal * (*salesreturn.VatPercent / float64(100))
	}

	salesreturn.NetTotal = math.Round(netTotal*100) / 100
}

func (salesreturn *SalesReturn) FindTotal() {
	total := float64(0.0)
	for _, product := range salesreturn.Products {
		total += (float64(product.Quantity) * product.UnitPrice)
	}

	salesreturn.Total = math.Round(total*100) / 100
}

func (salesreturn *SalesReturn) FindTotalQuantity() {
	totalQuantity := float64(0)
	for _, product := range salesreturn.Products {
		totalQuantity += product.Quantity
	}
	salesreturn.TotalQuantity = totalQuantity
}

func (salesreturn *SalesReturn) FindVatPrice() {
	vatPrice := ((*salesreturn.VatPercent / 100) * (salesreturn.Total - salesreturn.Discount))
	vatPrice = math.Round(vatPrice*100) / 100
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

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return salesreturns, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[net_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
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

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return salesreturns, criterias, err
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
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			salesreturn.DeletedByUser, _ = FindUserByID(salesreturn.DeletedBy, deletedByUserSelectFields)
		}
		salesreturns = append(salesreturns, salesreturn)
	} //end for loop

	return salesreturns, criterias, nil
}

func (salesreturn *SalesReturn) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldSalesReturn *SalesReturn) (errs map[string]string) {

	errs = make(map[string]string)

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

	if salesreturn.Discount > (order.Discount - order.ReturnDiscount) {
		errs["discount"] = "Discount shouldn't greater than " + fmt.Sprintf("%.2f", (order.Discount-order.ReturnDiscount))
	}

	salesreturn.OrderCode = order.Code

	if govalidator.IsNull(salesreturn.Status) {
		errs["status"] = "Status is required"
	}

	if govalidator.IsNull(salesreturn.PaymentMethod) {
		errs["payment_method"] = "Payment method is required"
	}

	if govalidator.IsNull(salesreturn.PaymentStatus) {
		errs["payment_status"] = "Payment status is required"
	}

	if govalidator.IsNull(salesreturn.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		/*
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, salesreturn.DateStr)
			if err != nil {
				errs["date_str"] = "Invalid date format"
			}
			salesreturn.Date = &date
		*/

		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, salesreturn.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		salesreturn.Date = &date
	}

	if !govalidator.IsNull(salesreturn.SignatureDateStr) {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, salesreturn.SignatureDateStr)
		if err != nil {
			errs["signature_date_str"] = "Invalid date format"
		}
		salesreturn.SignatureDate = &date
	}

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

		if oldSalesReturn != nil {
			if oldSalesReturn.Status == "delivered" || oldSalesReturn.Status == "dispatched" {
				if salesreturn.Status == "pending" || salesreturn.Status == "cancelled" || salesreturn.Status == "salesreturn_placed" {
					errs["status"] = "Can't change the status from delivered/dispatched to pending/cancelled/salesreturn_placed"
				}
			}
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

	if len(salesreturn.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for salesreturn"
	}

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

	for index, salesReturnProduct := range salesreturn.Products {
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
				soldQty := math.Round((orderProduct.Quantity-orderProduct.QuantityReturned)*100) / 100
				if soldQty == 0 {
					errs["quantity_"+strconv.Itoa(index)] = "Already returned all sold quantities"
				} else if salesReturnProduct.Quantity > float64(soldQty) {
					errs["quantity_"+strconv.Itoa(index)] = "Quantity should not be greater than purchased quantity: " + fmt.Sprintf("%.02f", soldQty) + " " + orderProduct.Unit
				}
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

func (salesreturn *SalesReturn) UpdateReturnedQuantityInOrderProduct() error {
	order, err := FindOrderByID(salesreturn.OrderID, bson.M{})
	if err != nil {
		return err
	}
	for _, salesReturnProduct := range salesreturn.Products {
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

	for _, stock := range product.Stock {
		if stock.StoreID.Hex() == storeID.Hex() {
			return stock.Stock, nil
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

		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == salesreturn.StoreID.Hex() {

				salesreturn.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stock[k].Stock,
					(product.Stock[k].Stock - salesreturnProduct.Quantity),
				)

				product.SetChangeLog(
					"remove_stock",
					product.Name,
					product.Stock[k].Stock,
					(product.Stock[k].Stock - salesreturnProduct.Quantity),
				)

				product.Stock[k].Stock -= salesreturnProduct.Quantity
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
		product, err := FindProductByID(&salesreturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		storeExistInProductStock := false
		for k, stock := range product.Stock {
			if stock.StoreID.Hex() == salesreturn.StoreID.Hex() {
				/*
					salesreturn.SetChangeLog(
						"add_stock",
						product.Name,
						product.Stock[k].Stock,
						(product.Stock[k].Stock + salesreturnProduct.Quantity),
					)

					product.SetChangeLog(
						"add_stock",
						product.Name,
						product.Stock[k].Stock,
						(product.Stock[k].Stock + salesreturnProduct.Quantity),
					)*/

				product.Stock[k].Stock += salesreturnProduct.Quantity
				storeExistInProductStock = true
				break
			}
		}

		if !storeExistInProductStock {
			productStock := ProductStock{
				StoreID: *salesreturn.StoreID,
				Stock:   salesreturnProduct.Quantity,
			}
			product.Stock = append(product.Stock, productStock)
		}

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

	err := salesreturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	salesreturn.ID = primitive.NewObjectID()
	if len(salesreturn.Code) == 0 {
		err = salesreturn.MakeCode()
		if err != nil {
			return err
		}
	}

	err = salesreturn.CalculateSalesReturnProfit()
	if err != nil {
		return err
	}

	_, err = collection.InsertOne(ctx, &salesreturn)
	if err != nil {
		return err
	}

	err = salesreturn.UpdateOrderReturnDiscount()
	if err != nil {
		return err
	}

	err = salesreturn.AddProductsSalesReturnHistory()
	if err != nil {
		return err
	}

	err = salesreturn.AddPayment()
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
			for _, unitPrice := range product.UnitPrices {
				if unitPrice.StoreID == *salesReturn.StoreID {
					purchaseUnitPrice = unitPrice.PurchaseUnitPrice
					salesReturn.Products[i].PurchaseUnitPrice = purchaseUnitPrice
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
			salesReturn.Products[i].Profit = profit
			salesReturn.Products[i].Loss = 0.0
			totalProfit += salesReturn.Products[i].Profit
		} else {
			salesReturn.Products[i].Profit = 0
			salesReturn.Products[i].Loss = (profit * -1)
			totalLoss += salesReturn.Products[i].Loss
		}

	}
	salesReturn.Profit = math.Round(totalProfit) * 100 / 100
	salesReturn.NetProfit = math.Round((totalProfit-salesReturn.Discount)*100) / 100
	salesReturn.Loss = totalLoss
	return nil
}

func (salesReturn *SalesReturn) MakeCode() error {
	lastQuotation, err := FindLastSalesReturnByStoreID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}
	if lastQuotation == nil {
		store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
		if err != nil {
			return err
		}
		salesReturn.Code = store.Code + "-200000"
	} else {
		splits := strings.Split(lastQuotation.Code, "-")
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

		splits := strings.Split(lastQuotation.Code, "-")
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

func (salesReturn *SalesReturn) AddPayment() error {
	amount := float64(0.0)
	if salesReturn.PaymentStatus == "paid" {
		amount = salesReturn.NetTotal
	} else if salesReturn.PaymentStatus == "paid_partially" {
		amount = salesReturn.PartiaPaymentAmount
	} else {
		return nil
	}

	payment := SalesReturnPayment{
		SalesReturnID:   &salesReturn.ID,
		SalesReturnCode: salesReturn.Code,
		OrderID:         salesReturn.OrderID,
		OrderCode:       salesReturn.OrderCode,
		Amount:          amount,
		Method:          salesReturn.PaymentMethod,
		CreatedAt:       salesReturn.CreatedAt,
		UpdatedAt:       salesReturn.UpdatedAt,
		CreatedBy:       salesReturn.CreatedBy,
		CreatedByName:   salesReturn.CreatedByName,
		UpdatedBy:       salesReturn.UpdatedBy,
		UpdatedByName:   salesReturn.UpdatedByName,
		StoreID:         salesReturn.StoreID,
		StoreName:       salesReturn.StoreName,
	}
	err := payment.Insert()
	if err != nil {
		return err
	}
	return nil
}

func (salesreturn *SalesReturn) UpdateOrderReturnDiscount() error {
	order, err := FindOrderByID(salesreturn.OrderID, bson.M{})
	if err != nil {
		return err
	}
	order.ReturnDiscount += salesreturn.Discount
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

	err := salesreturn.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	err = salesreturn.CalculateSalesReturnProfit()
	if err != nil {
		return err
	}

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

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	salesreturn.Deleted = true
	salesreturn.DeletedBy = &userID
	now := time.Now()
	salesreturn.DeletedAt = &now

	salesreturn.SetChangeLog("delete", nil, nil, nil)

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

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		salesreturn.DeletedByUser, _ = FindUserByID(salesreturn.DeletedBy, fields)
	}

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
	collection := db.Client().Database(db.GetPosDB()).Collection("salesreturn")
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
		salesReturn := SalesReturn{}
		err = cur.Decode(&salesReturn)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = salesReturn.CalculateSalesReturnProfit()
		if err != nil {
			return err
		}

		err = salesReturn.AddProductsSalesReturnHistory()
		if err != nil {
			return err
		}

		/*
			if salesReturn.PaymentStatus == "" {
				salesReturn.PaymentStatus = "paid"
			}

			if salesReturn.PaymentMethod == "" {
				salesReturn.PaymentMethod = "cash"
			}

			totalPaymentsCount, err := GetTotalCount(bson.M{"sales_return_id": salesReturn.ID}, "sales_return_payment")
			if err != nil {
				return err
			}

			if totalPaymentsCount == 0 {
				err = salesReturn.AddPayment()
				if err != nil {
					return err
				}
			}
		*/

		/*
			d := salesReturn.Date.Add(time.Hour * time.Duration(-3))
			salesReturn.Date = &d
		*/
		//salesReturn.Date = salesReturn.CreatedAt
		err = salesReturn.Update()
		if err != nil {
			return err
		}

		if salesReturn.Code == "GUOJ-200042" {
			err = salesReturn.HardDelete()
			if err != nil {
				return err
			}
		}

	}

	return nil
}
