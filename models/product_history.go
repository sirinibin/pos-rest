package models

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductHistory struct {
	ID                 primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date               *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID            *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName          string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID          primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	ReferenceType      string              `json:"reference_type,omitempty" bson:"reference_type,omitempty"`
	ReferenceID        *primitive.ObjectID `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	ReferenceCode      string              `json:"reference_code,omitempty" bson:"reference_code,omitempty"`
	CustomerID         *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName       string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	CustomerNameArabic string              `json:"customer_name_arabic" bson:"customer_name_arabic"`
	VendorID           *primitive.ObjectID `json:"vendor_id,omitempty" bson:"vendor_id,omitempty"`
	VendorName         string              `json:"vendor_name,omitempty" bson:"vendor_name,omitempty"`
	VendorNameArabic   string              `json:"vendor_name_arabic" bson:"vendor_name_arabic"`
	Stock              float64             `json:"stock" bson:"stock"`
	Quantity           float64             `json:"quantity" bson:"quantity"`
	PurchaseUnitPrice  float64             `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	UnitPrice          float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit               string              `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitDiscount       float64             `bson:"unit_discount" json:"unit_discount"`
	Discount           float64             `bson:"discount" json:"discount"`
	DiscountPercent    float64             `bson:"discount_percent" json:"discount_percent"`
	Price              float64             `bson:"price" json:"price"`
	NetPrice           float64             `bson:"net_price" json:"net_price"`
	Profit             float64             `bson:"profit" json:"profit"`
	Loss               float64             `bson:"loss" json:"loss"`
	VatPercent         float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice           float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	UnitPriceWithVAT   float64             `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	Store              *Store              `json:"store,omitempty" bson:"-"`
	Customer           *Customer           `json:"customer,omitempty" bson:"-"`
	Vendor             *Vendor             `json:"vendor,omitempty" bson:"-"`
	CreatedAt          *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	WarehouseID        *primitive.ObjectID `json:"warehouse_id" bson:"warehouse_id"`
	WarehouseCode      *string             `json:"warehouse_code" bson:"warehouse_code"`
}

type HistoryStats struct {
	ID               *primitive.ObjectID `json:"id" bson:"_id"`
	TotalSales       float64             `json:"total_sales" bson:"total_sales"`
	TotalSalesProfit float64             `json:"total_sales_profit" bson:"total_sales_profit"`
	TotalSalesLoss   float64             `json:"total_sales_loss" bson:"total_sales_loss"`
	TotalSalesVat    float64             `json:"total_sales_vat" bson:"total_sales_vat"`

	TotalSalesReturn       float64 `json:"total_sale_return" bson:"total_sales_return"`
	TotalSalesReturnProfit float64 `json:"total_sales_return_profit" bson:"total_sales_return_profit"`
	TotalSalesReturnLoss   float64 `json:"total_sales_return_loss" bson:"total_sales_return_loss"`
	TotalSalesReturnVat    float64 `json:"total_sales_return_vat" bson:"total_sales_return_vat"`

	TotalPurchase       float64 `json:"total_purchase" bson:"total_purchase"`
	TotalPurchaseProfit float64 `json:"total_purchase_profit" bson:"total_purchase_profit"`
	TotalPurchaseLoss   float64 `json:"total_purchase_loss" bson:"total_purchase_loss"`
	TotalPurchaseVat    float64 `json:"total_purchase_vat" bson:"total_purchase_vat"`

	TotalPurchaseReturn       float64 `json:"total_purchase_return" bson:"total_purchase_return"`
	TotalPurchaseReturnProfit float64 `json:"total_purchase_return_profit" bson:"total_purchase_return_profit"`
	TotalPurchaseReturnLoss   float64 `json:"total_purchase_return_loss" bson:"total_purchase_return_loss"`
	TotalPurchaseReturnVat    float64 `json:"total_purchase_return_vat" bson:"total_purchase_return_vat"`

	TotalQuotation       float64 `json:"total_quotation" bson:"total_quotation"`
	TotalQuotationProfit float64 `json:"total_quotation_profit" bson:"total_quotation_profit"`
	TotalQuotationLoss   float64 `json:"total_quotation_loss" bson:"total_quotation_loss"`
	TotalQuotationVat    float64 `json:"total_quotation_vat" bson:"total_quotation_vat"`

	TotalQuotationSales       float64 `json:"total_quotation_sales" bson:"total_quotation_sales"`
	TotalQuotationSalesProfit float64 `json:"total_quotation_sales_profit" bson:"total_quotation_sales_profit"`
	TotalQuotationSalesLoss   float64 `json:"total_quotation_sales_loss" bson:"total_quotation_sales_loss"`
	TotalQuotationSalesVat    float64 `json:"total_quotation_sales_vat" bson:"total_quotation_sales_vat"`

	TotalQuotationSalesReturn       float64 `json:"total_quotation_sales_return" bson:"total_quotation_sales_return"`
	TotalQuotationSalesReturnProfit float64 `json:"total_quotation_sales_return_profit" bson:"total_quotation_sales_return_profit"`
	TotalQuotationSalesReturnLoss   float64 `json:"total_quotation_sales_return_loss" bson:"total_quotation_sales_return_loss"`
	TotalQuotationSalesReturnVat    float64 `json:"total_quotation_sales_return_vat" bson:"total_quotation_sales_return_vat"`

	TotalDeliveryNoteQuantity float64 `json:"total_delivery_note_quantity" bson:"total_delivery_note_quantity"`
}

func (store *Store) GetHistoryStats(filter map[string]interface{}) (stats HistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				//Sales
				"total_sales": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales"}},
					"$net_price",
					0,
				}}},
				"total_sales_profit": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales"}},
					"$profit",
					0,
				}}},
				"total_sales_loss": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales"}},
					"$loss",
					0,
				}}},
				"total_sales_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales"}},
					"$vat_price",
					0,
				}}},
				//Sales return
				"total_sales_return": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales_return"}},
					"$net_price",
					0,
				}}},
				"total_sales_return_profit": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales_return"}},
					"$profit",
					0,
				}}},
				"total_sales_return_loss": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales_return"}},
					"$loss",
					0,
				}}},
				"total_sales_return_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "sales_return"}},
					"$vat_price",
					0,
				}}},
				//Purchase
				"total_purchase": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "purchase"}},
					"$net_price",
					0,
				}}},
				"total_purchase_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "purchase"}},
					"$vat_price",
					0,
				}}},
				//Purchase return
				"total_purchase_return": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "purchase_return"}},
					"$net_price",
					0,
				}}},
				"total_purchase_return_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "purchase_return"}},
					"$vat_price",
					0,
				}}},
				//Quotation
				"total_quotation": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation"}},
					"$net_price",
					0,
				}}},
				"total_quotation_profit": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation"}},
					"$profit",
					0,
				}}},
				"total_quotation_loss": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation"}},
					"$loss",
					0,
				}}},
				"total_quotation_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation"}},
					"$vat_price",
					0,
				}}},
				//Quotation Sales
				"total_quotation_sales": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_invoice"}},
					"$net_price",
					0,
				}}},
				"total_quotation_sales_profit": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_invoice"}},
					"$profit",
					0,
				}}},
				"total_quotation_sales_loss": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_invoice"}},
					"$loss",
					0,
				}}},
				"total_quotation_sales_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_invoice"}},
					"$vat_price",
					0,
				}}},
				//Quotation Sales Return
				"total_quotation_sales_return": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_sales_return"}},
					"$net_price",
					0,
				}}},
				"total_quotation_sales_return_profit": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_sales_return"}},
					"$profit",
					0,
				}}},
				"total_quotation_sales_return_loss": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_sales_return"}},
					"$loss",
					0,
				}}},
				"total_quotation_sales_return_vat": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "quotation_sales_return"}},
					"$vat_price",
					0,
				}}},
				//Delivery note
				"total_delivery_note_quantity": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$eq": []interface{}{"$reference_type", "delivery_note"}},
					"$quantity",
					0,
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
		//Sales
		stats.TotalSales = RoundTo2Decimals(stats.TotalSales)
		stats.TotalSalesProfit = RoundTo2Decimals(stats.TotalSalesProfit)
		stats.TotalSalesLoss = RoundTo2Decimals(stats.TotalSalesLoss)
		stats.TotalSalesVat = RoundTo2Decimals(stats.TotalSalesVat)

		//Sales Return
		stats.TotalSalesReturn = RoundTo2Decimals(stats.TotalSalesReturn)
		stats.TotalSalesReturnProfit = RoundTo2Decimals(stats.TotalSalesReturnProfit)
		stats.TotalSalesReturnLoss = RoundTo2Decimals(stats.TotalSalesReturnLoss)
		stats.TotalSalesReturnVat = RoundTo2Decimals(stats.TotalSalesReturnVat)

		//Purchase
		stats.TotalPurchase = RoundTo2Decimals(stats.TotalPurchase)
		stats.TotalPurchaseVat = RoundTo2Decimals(stats.TotalPurchaseVat)

		//Purchase Return
		stats.TotalPurchaseReturn = RoundTo2Decimals(stats.TotalPurchaseReturn)
		stats.TotalPurchaseReturnVat = RoundTo2Decimals(stats.TotalPurchaseReturnVat)

		//Delivery Note
		stats.TotalDeliveryNoteQuantity = RoundTo2Decimals(stats.TotalDeliveryNoteQuantity)
	}

	return stats, nil
}

func (store *Store) SearchHistory(w http.ResponseWriter, r *http.Request) (models []ProductHistory, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SortBy = map[string]interface{}{
		"created_at": -1,
	}

	criterias.SearchBy = make(map[string]interface{})

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
			return models, criterias, err
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
			return models, criterias, err
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
			return models, criterias, err
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
			return models, criterias, err
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
			return models, criterias, err
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
			return models, criterias, err
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

	keys, ok = r.URL.Query()["search[reference_type]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] != "" {
			criterias.SearchBy["reference_type"] = keys[0]
		}
	}

	keys, ok = r.URL.Query()["search[store_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["store_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[customer_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["customer_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[vendor_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["vendor_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[vendor_id]"]
	if ok && len(keys[0]) >= 1 {
		vendorIds := strings.Split(keys[0], ",")
		objecIds := []primitive.ObjectID{}

		for _, id := range vendorIds {
			vendorID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, vendorID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["vendor_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["price"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[unit_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["unit_price"] = bson.M{operator: float64(value)}
		} else {
			//log.Print(value)
			//criterias.SearchBy["unit_price"] = bson.M{"$eq": float64(value)}
			criterias.SearchBy["unit_price"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[unit_price_with_vat]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["unit_price_with_vat"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["unit_price_with_vat"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[discount]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["discount"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["discount"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[discount_percent]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["discount_percent"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["discount_percent"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["quantity"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["quantity"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[stock]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["stock"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["stock"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["profit"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["profit"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["loss"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["loss"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[vat_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["vat_price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["vat_price"] = float64(value)
		}
	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return models, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[product_id]"]
	if ok && len(keys[0]) >= 1 {
		productID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["product_id"] = productID
	}

	keys, ok = r.URL.Query()["search[reference_id]"]
	if ok && len(keys[0]) >= 1 {
		orderID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["reference_id"] = orderID
	}

	keys, ok = r.URL.Query()["search[reference_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["reference_code"] = keys[0]
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	customerSelectFields := map[string]interface{}{}

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
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching product history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			model.Store, _ = FindStoreByID(model.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			model.Customer, _ = store.FindCustomerByID(model.CustomerID, customerSelectFields)
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil
}

func (model *Product) ClearStockAdjustmentHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *Order) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *SalesReturn) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *Purchase) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *PurchaseReturn) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *DeliveryNote) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *Quotation) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (model *QuotationSalesReturn) ClearProductsHistory() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"reference_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (product *Product) CreateStockAdjustmentHistory() error {
	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return err
	}

	//log.Printf("Creating  history of order id:%s", order.Code)
	exists, err := store.IsHistoryExistsByReferenceID(&product.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, stockAdjustment := range product.ProductStores[store.ID.Hex()].StockAdjustments {
		stock, err := product.GetProductQuantityBeforeOrEqualTo(stockAdjustment.Date)
		if err != nil {
			return err
		}

		newStock := float64(0.00)

		if stockAdjustment.Type == "adding" {
			newStock = stock + stockAdjustment.Quantity
		} else if stockAdjustment.Type == "removing" {
			newStock = stock - stockAdjustment.Quantity
		}

		history := ProductHistory{
			Date:          stockAdjustment.Date,
			StoreID:       product.StoreID,
			StoreName:     product.StoreName,
			ProductID:     product.ID,
			CustomerID:    nil,
			CustomerName:  "",
			ReferenceType: "stock_adjustment_by_" + stockAdjustment.Type,
			ReferenceID:   &product.ID,
			ReferenceCode: product.PartNumber,
			Stock:         newStock,
			Quantity:      stockAdjustment.Quantity,
			Unit:          product.Unit,
			WarehouseID:   stockAdjustment.WarehouseID,
			WarehouseCode: stockAdjustment.WarehouseCode,
			CreatedAt:     stockAdjustment.CreatedAt,
			UpdatedAt:     stockAdjustment.CreatedAt,
		}

		history.ID = primitive.NewObjectID()

		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		go product.AdjustStockInHistoryAfter(history.Date)
	}
	return nil
}

func (product *Product) AdjustStockInHistoryAfter(after *time.Time) error {
	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return err
	}

	histories, err := product.GetHistoriesAfter(after)
	if err != nil {
		return err
	}

	for i, history := range histories {
		stock, err := product.GetProductQuantityBeforeOrEqualTo(history.Date)
		if err != nil {
			return err
		}

		newStock := stock

		if history.ReferenceType == "sales" {
			newStock = stock - history.Quantity
		} else if history.ReferenceType == "sales_return" {
			newStock = stock + history.Quantity
		} else if history.ReferenceType == "purchase" {
			newStock = stock + history.Quantity
		} else if history.ReferenceType == "purchase_return" {
			newStock = stock - history.Quantity
		} else if history.ReferenceType == "quotation_invoice" {
			if store.Settings.UpdateProductStockOnQuotationSales {
				if store.IfStore2QuotationSalesShouldAffectTheStock(history.Date) {
					newStock = stock - history.Quantity
				}
			}
		} else if history.ReferenceType == "quotation_sales_return" {
			if store.Settings.UpdateProductStockOnQuotationSales {
				if store.IfStore2QuotationSalesShouldAffectTheStock(history.Date) {
					newStock = stock + history.Quantity
				}
			}
		} else if history.ReferenceType == "stock_adjustment_by_adding" {
			newStock = stock + history.Quantity
		} else if history.ReferenceType == "stock_adjustment_by_removing" {
			newStock = stock - history.Quantity
		}

		histories[i].Stock = newStock
		err = histories[i].Update()
		if err != nil {
			return err
		}

	}

	return nil
}

func (product *Product) GetHistoriesAfter(after *time.Time) (models []ProductHistory, err error) {
	//log.Print("Fetching sales histories")

	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)
	findOptions.SetSort(map[string]interface{}{"date": 1})

	cur, err := collection.Find(ctx, bson.M{
		"product_id": product.ID,
		"date":       bson.M{"$gt": after},
	}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching product sales history" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		//log.Print("Loop")
		err := cur.Err()
		if err != nil {
			return models, errors.New("Cursor error:" + err.Error())
		}
		history := ProductHistory{}
		err = cur.Decode(&history)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")
		models = append(models, history)
	} //end for loop

	return models, nil
}

func (order *Order) CreateProductsHistory() error {
	store, err := FindStoreByID(order.StoreID, bson.M{})
	if err != nil {
		return err
	}

	//log.Printf("Creating  history of order id:%s", order.Code)
	exists, err := store.IsHistoryExistsByReferenceID(&order.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + order.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, orderProduct := range order.Products {
		product, err := store.FindProductByID(&orderProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(order.Date)
		if err != nil {
			return err
		}

		history := ProductHistory{
			Date:               order.Date,
			StoreID:            order.StoreID,
			StoreName:          order.StoreName,
			ProductID:          orderProduct.ProductID,
			CustomerID:         order.CustomerID,
			CustomerName:       order.CustomerName,
			CustomerNameArabic: order.CustomerNameArabic,
			ReferenceType:      "sales",
			ReferenceID:        &order.ID,
			ReferenceCode:      order.Code,
			Stock:              (stock - orderProduct.Quantity),
			Quantity:           orderProduct.Quantity,
			PurchaseUnitPrice:  orderProduct.PurchaseUnitPrice,
			Unit:               orderProduct.Unit,
			UnitDiscount:       orderProduct.UnitDiscount,
			Discount:           (orderProduct.UnitDiscount * orderProduct.Quantity),
			DiscountPercent:    orderProduct.UnitDiscountPercent,
			CreatedAt:          order.CreatedAt,
			UpdatedAt:          order.UpdatedAt,
			WarehouseID:        orderProduct.WarehouseID,
			WarehouseCode:      orderProduct.WarehouseCode,
		}

		history.UnitPrice = RoundTo2Decimals(orderProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo2Decimals(orderProduct.UnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((orderProduct.UnitPrice - orderProduct.UnitDiscount) * orderProduct.Quantity))
		history.Profit = RoundTo2Decimals(orderProduct.Profit)
		history.Loss = RoundTo2Decimals(orderProduct.Loss)

		history.VatPercent = RoundTo2Decimals(*order.VatPercent)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
		history.ID = primitive.NewObjectID()

		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(order.Date)
				if err != nil {
					return err
				}

				history := ProductHistory{
					Date:               order.Date,
					StoreID:            order.StoreID,
					StoreName:          order.StoreName,
					ProductID:          *setProduct.ProductID,
					CustomerID:         order.CustomerID,
					CustomerName:       order.CustomerName,
					CustomerNameArabic: order.CustomerNameArabic,
					ReferenceType:      "sales",
					ReferenceID:        &order.ID,
					ReferenceCode:      order.Code,
					Stock:              (stock - RoundTo2Decimals(orderProduct.Quantity*setProduct.Quantity)),
					Quantity:           RoundTo2Decimals(orderProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice:  RoundTo2Decimals(orderProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:               setProductObj.Unit,
					UnitDiscount:       RoundTo2Decimals(orderProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:           RoundTo2Decimals((orderProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo2Decimals(orderProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:    orderProduct.UnitDiscountPercent,
					CreatedAt:          order.CreatedAt,
					UpdatedAt:          order.UpdatedAt,
					WarehouseID:        orderProduct.WarehouseID,
					WarehouseCode:      orderProduct.WarehouseCode,
				}

				history.UnitPrice = RoundTo2Decimals(orderProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo2Decimals(orderProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)
				history.Profit = RoundTo2Decimals(orderProduct.Profit * (setProduct.RetailPricePercent / 100))
				history.Loss = RoundTo2Decimals(orderProduct.Loss * (setProduct.RetailPricePercent / 100))

				history.VatPercent = RoundTo2Decimals(*order.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}
	}
	return nil
}

func (salesReturn *SalesReturn) CreateProductsHistory() error {
	store, err := FindStoreByID(salesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsHistoryExistsByReferenceID(&salesReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + salesReturn.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, salesReturnProduct := range salesReturn.Products {
		if !salesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&salesReturnProduct.ProductID, bson.M{})
		if err != nil {
			return errors.New("Product not found for sales return product id:" + salesReturnProduct.ProductID.Hex())
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(salesReturn.Date)
		if err != nil {
			return errors.New("Error getting product quantity for sales return:" + err.Error())
		}

		history := ProductHistory{
			Date:               salesReturn.Date,
			StoreID:            salesReturn.StoreID,
			StoreName:          salesReturn.StoreName,
			ProductID:          salesReturnProduct.ProductID,
			CustomerID:         salesReturn.CustomerID,
			CustomerName:       salesReturn.CustomerName,
			CustomerNameArabic: salesReturn.CustomerNameArabic,
			ReferenceType:      "sales_return",
			ReferenceID:        &salesReturn.ID,
			ReferenceCode:      salesReturn.Code,
			Stock:              (stock + salesReturnProduct.Quantity),
			Quantity:           salesReturnProduct.Quantity,
			UnitPrice:          salesReturnProduct.UnitPrice,
			Unit:               salesReturnProduct.Unit,
			Discount:           salesReturnProduct.UnitDiscount,
			DiscountPercent:    salesReturnProduct.UnitDiscountPercent,
			CreatedAt:          salesReturn.CreatedAt,
			UpdatedAt:          salesReturn.UpdatedAt,
			WarehouseID:        salesReturnProduct.WarehouseID,
			WarehouseCode:      salesReturnProduct.WarehouseCode,
		}

		history.UnitPrice = RoundTo2Decimals(salesReturnProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo2Decimals(salesReturnProduct.UnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((salesReturnProduct.UnitPrice - salesReturnProduct.UnitDiscount) * salesReturnProduct.Quantity))
		history.VatPercent = RoundFloat(*salesReturn.VatPercent, 2)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
		history.Profit = RoundFloat(salesReturnProduct.Profit, 2)
		history.Loss = RoundFloat(salesReturnProduct.Loss, 2)

		history.ID = primitive.NewObjectID()

		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return errors.New("Error inserting product history:" + err.Error())
		}

		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(salesReturn.Date)
				if err != nil {
					return errors.New("error GetProductQuantityBeforeOrEqualTo:" + err.Error())
				}

				history := ProductHistory{
					Date:               salesReturn.Date,
					StoreID:            salesReturn.StoreID,
					StoreName:          salesReturn.StoreName,
					ProductID:          *setProduct.ProductID,
					CustomerID:         salesReturn.CustomerID,
					CustomerName:       salesReturn.CustomerName,
					CustomerNameArabic: salesReturn.CustomerNameArabic,
					ReferenceType:      "sales_return",
					ReferenceID:        &salesReturn.ID,
					ReferenceCode:      salesReturn.Code,
					Stock:              (stock + RoundTo2Decimals(salesReturnProduct.Quantity*setProduct.Quantity)),
					Quantity:           RoundTo2Decimals(salesReturnProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice:  RoundTo2Decimals(salesReturnProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:               setProductObj.Unit,
					UnitDiscount:       RoundTo2Decimals(salesReturnProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:           RoundTo2Decimals((salesReturnProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo2Decimals(salesReturnProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:    salesReturnProduct.UnitDiscountPercent,
					CreatedAt:          salesReturn.CreatedAt,
					UpdatedAt:          salesReturn.UpdatedAt,
					WarehouseID:        salesReturnProduct.WarehouseID,
					WarehouseCode:      salesReturnProduct.WarehouseCode,
				}

				history.UnitPrice = RoundTo2Decimals(salesReturnProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo2Decimals(salesReturnProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)
				history.Profit = RoundTo2Decimals(salesReturnProduct.Profit * (setProduct.RetailPricePercent / 100))
				history.Loss = RoundTo2Decimals(salesReturnProduct.Loss * (setProduct.RetailPricePercent / 100))

				history.VatPercent = RoundTo2Decimals(*salesReturn.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}
	}

	return nil
}

func (purchase *Purchase) CreateProductsHistory() error {
	store, err := FindStoreByID(purchase.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsHistoryExistsByReferenceID(&purchase.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + purchase.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, purchaseProduct := range purchase.Products {

		product, err := store.FindProductByID(&purchaseProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(purchase.Date)
		if err != nil {
			return err
		}

		history := ProductHistory{
			Date:             purchase.Date,
			StoreID:          purchase.StoreID,
			StoreName:        purchase.StoreName,
			ProductID:        purchaseProduct.ProductID,
			VendorID:         purchase.VendorID,
			VendorName:       purchase.VendorName,
			VendorNameArabic: purchase.VendorNameArabic,
			ReferenceType:    "purchase",
			ReferenceID:      &purchase.ID,
			ReferenceCode:    purchase.Code,
			Stock:            (stock + purchaseProduct.Quantity),
			Quantity:         purchaseProduct.Quantity,
			UnitPrice:        purchaseProduct.PurchaseUnitPrice,
			Unit:             purchaseProduct.Unit,
			Discount:         purchaseProduct.UnitDiscount,
			DiscountPercent:  purchaseProduct.UnitDiscountPercent,
			CreatedAt:        purchase.CreatedAt,
			UpdatedAt:        purchase.UpdatedAt,
			WarehouseID:      purchaseProduct.WarehouseID,
			WarehouseCode:    purchaseProduct.WarehouseCode,
		}

		history.UnitPrice = RoundTo2Decimals(purchaseProduct.PurchaseUnitPrice)
		history.UnitPriceWithVAT = RoundTo2Decimals(purchaseProduct.PurchaseUnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((purchaseProduct.PurchaseUnitPrice - purchaseProduct.UnitDiscount) * purchaseProduct.Quantity))

		history.VatPercent = RoundFloat(*purchase.VatPercent, 2)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))

		history.ID = primitive.NewObjectID()

		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(purchase.Date)
				if err != nil {
					return err
				}

				history := ProductHistory{
					Date:             purchase.Date,
					StoreID:          purchase.StoreID,
					StoreName:        purchase.StoreName,
					ProductID:        *setProduct.ProductID,
					VendorID:         purchase.VendorID,
					VendorName:       purchase.VendorName,
					VendorNameArabic: purchase.VendorNameArabic,
					ReferenceType:    "purchase",
					ReferenceID:      &purchase.ID,
					ReferenceCode:    purchase.Code,
					Stock:            (stock + RoundTo2Decimals(purchaseProduct.Quantity*setProduct.Quantity)),
					Quantity:         RoundTo2Decimals(purchaseProduct.Quantity * setProduct.Quantity),
					UnitPrice:        RoundTo2Decimals(purchaseProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:             setProductObj.Unit,
					UnitDiscount:     RoundTo2Decimals(purchaseProduct.UnitDiscount * (setProduct.PurchasePricePercent / 100)),
					Discount:         RoundTo2Decimals((purchaseProduct.UnitDiscount * (setProduct.PurchasePricePercent / 100)) * RoundTo2Decimals(purchaseProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:  purchaseProduct.UnitDiscountPercent,
					CreatedAt:        purchase.CreatedAt,
					UpdatedAt:        purchase.UpdatedAt,
					WarehouseID:      purchaseProduct.WarehouseID,
					WarehouseCode:    purchaseProduct.WarehouseCode,
				}

				history.UnitPrice = RoundTo2Decimals(purchaseProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100))
				history.UnitPriceWithVAT = RoundTo2Decimals(purchaseProduct.PurchaseUnitPriceWithVAT * (setProduct.PurchasePricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)

				history.VatPercent = RoundTo2Decimals(*purchase.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}

	}
	return nil
}

func (purchaseReturn *PurchaseReturn) CreateProductsHistory() error {
	store, err := FindStoreByID(purchaseReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsHistoryExistsByReferenceID(&purchaseReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + purchaseReturn.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, purchaseReturnProduct := range purchaseReturn.Products {
		if !purchaseReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&purchaseReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(purchaseReturn.Date)
		if err != nil {
			return err
		}

		history := ProductHistory{
			Date:             purchaseReturn.Date,
			StoreID:          purchaseReturn.StoreID,
			StoreName:        purchaseReturn.StoreName,
			ProductID:        purchaseReturnProduct.ProductID,
			VendorID:         purchaseReturn.VendorID,
			VendorName:       purchaseReturn.VendorName,
			VendorNameArabic: purchaseReturn.VendorNameArabic,
			ReferenceType:    "purchase_return",
			ReferenceID:      &purchaseReturn.ID,
			ReferenceCode:    purchaseReturn.Code,
			Stock:            (stock - purchaseReturnProduct.Quantity),
			Quantity:         purchaseReturnProduct.Quantity,
			UnitPrice:        purchaseReturnProduct.PurchaseReturnUnitPrice,
			Unit:             purchaseReturnProduct.Unit,
			Discount:         purchaseReturnProduct.UnitDiscount,
			DiscountPercent:  purchaseReturnProduct.UnitDiscountPercent,
			CreatedAt:        purchaseReturn.CreatedAt,
			UpdatedAt:        purchaseReturn.UpdatedAt,
			WarehouseID:      purchaseReturnProduct.WarehouseID,
			WarehouseCode:    purchaseReturnProduct.WarehouseCode,
		}

		history.UnitPrice = RoundTo2Decimals(purchaseReturnProduct.PurchaseReturnUnitPrice)
		history.UnitPriceWithVAT = RoundTo2Decimals(purchaseReturnProduct.PurchaseReturnUnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((purchaseReturnProduct.PurchaseReturnUnitPrice - purchaseReturnProduct.UnitDiscount) * purchaseReturnProduct.Quantity))
		history.VatPercent = RoundFloat(*purchaseReturn.VatPercent, 2)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))

		history.ID = primitive.NewObjectID()

		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}
		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(purchaseReturn.Date)
				if err != nil {
					return err
				}

				history := ProductHistory{
					Date:             purchaseReturn.Date,
					StoreID:          purchaseReturn.StoreID,
					StoreName:        purchaseReturn.StoreName,
					ProductID:        *setProduct.ProductID,
					VendorID:         purchaseReturn.VendorID,
					VendorName:       purchaseReturn.VendorName,
					VendorNameArabic: purchaseReturn.VendorNameArabic,
					ReferenceType:    "purchase_return",
					ReferenceID:      &purchaseReturn.ID,
					ReferenceCode:    purchaseReturn.Code,
					Stock:            (stock - RoundTo2Decimals(purchaseReturnProduct.Quantity*setProduct.Quantity)),
					Quantity:         RoundTo2Decimals(purchaseReturnProduct.Quantity * setProduct.Quantity),
					UnitPrice:        RoundTo2Decimals(purchaseReturnProduct.PurchaseReturnUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:             setProductObj.Unit,
					UnitDiscount:     RoundTo2Decimals(purchaseReturnProduct.UnitDiscount * (setProduct.PurchasePricePercent / 100)),
					Discount:         RoundTo2Decimals((purchaseReturnProduct.UnitDiscount * (setProduct.PurchasePricePercent / 100)) * RoundTo2Decimals(purchaseReturnProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:  purchaseReturnProduct.UnitDiscountPercent,
					CreatedAt:        purchaseReturn.CreatedAt,
					UpdatedAt:        purchaseReturn.UpdatedAt,
					WarehouseID:      purchaseReturnProduct.WarehouseID,
					WarehouseCode:    purchaseReturnProduct.WarehouseCode,
				}

				history.UnitPrice = RoundTo2Decimals(purchaseReturnProduct.PurchaseReturnUnitPrice * (setProduct.PurchasePricePercent / 100))
				history.UnitPriceWithVAT = RoundTo2Decimals(purchaseReturnProduct.PurchaseReturnUnitPriceWithVAT * (setProduct.PurchasePricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)

				history.VatPercent = RoundTo2Decimals(*purchaseReturn.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}
	}
	return nil
}

func (deliverynote *DeliveryNote) CreateProductsHistory() error {
	store, err := FindStoreByID(deliverynote.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsHistoryExistsByReferenceID(&deliverynote.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + deliverynote.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, deliverynoteProduct := range deliverynote.Products {
		product, err := store.FindProductByID(&deliverynoteProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(deliverynote.Date)
		if err != nil {
			return err
		}

		history := ProductHistory{
			Date:               deliverynote.Date,
			StoreID:            deliverynote.StoreID,
			StoreName:          deliverynote.StoreName,
			ProductID:          deliverynoteProduct.ProductID,
			CustomerID:         deliverynote.CustomerID,
			CustomerName:       deliverynote.CustomerName,
			CustomerNameArabic: deliverynote.CustomerNameArabic,
			ReferenceType:      "delivery_note",
			ReferenceID:        &deliverynote.ID,
			ReferenceCode:      deliverynote.Code,
			//Stock:         (stock + deliverynoteProduct.Quantity),
			Stock:     stock,
			Quantity:  deliverynoteProduct.Quantity,
			Unit:      deliverynoteProduct.Unit,
			CreatedAt: deliverynote.CreatedAt,
			UpdatedAt: deliverynote.UpdatedAt,
		}

		history.ID = primitive.NewObjectID()
		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(deliverynote.Date)
				if err != nil {
					return err
				}

				history := ProductHistory{
					Date:               deliverynote.Date,
					StoreID:            deliverynote.StoreID,
					StoreName:          deliverynote.StoreName,
					ProductID:          *setProduct.ProductID,
					CustomerID:         deliverynote.CustomerID,
					CustomerName:       deliverynote.CustomerName,
					CustomerNameArabic: deliverynote.CustomerNameArabic,
					ReferenceType:      "delivery_note",
					ReferenceID:        &deliverynote.ID,
					ReferenceCode:      deliverynote.Code,
					//Stock:         (stock + (deliverynoteProduct.Quantity * setProduct.Quantity)),
					Stock:     stock,
					Quantity:  (deliverynoteProduct.Quantity * setProduct.Quantity),
					Unit:      deliverynoteProduct.Unit,
					CreatedAt: deliverynote.CreatedAt,
					UpdatedAt: deliverynote.UpdatedAt,
				}

				history.ID = primitive.NewObjectID()
				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}
	}

	return nil
}

func (quotation *Quotation) CreateProductsHistory() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsHistoryExistsByReferenceID(&quotation.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	referenceType := ""
	if quotation.Type == "quotation" {
		referenceType = "quotation"
	} else if quotation.Type == "invoice" {
		referenceType = "quotation_invoice"
	}

	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, quotationProduct := range quotation.Products {
		product, err := store.FindProductByID(&quotationProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(quotation.Date)
		if err != nil {
			return err
		}

		if quotation.Type == "invoice" {
			//stock -= quotationProduct.Quantity

			if store.Settings.UpdateProductStockOnQuotationSales {
				if store.IfStore2QuotationSalesShouldAffectTheStock(quotation.Date) {
					stock -= quotationProduct.Quantity
				}
			}
		} else {
			quotationProduct.WarehouseCode = nil
			quotationProduct.WarehouseID = nil
		}

		history := ProductHistory{
			Date:               quotation.Date,
			StoreID:            quotation.StoreID,
			StoreName:          quotation.StoreName,
			ProductID:          quotationProduct.ProductID,
			CustomerID:         quotation.CustomerID,
			CustomerName:       quotation.CustomerName,
			CustomerNameArabic: quotation.CustomerNameArabic,
			ReferenceType:      referenceType,
			ReferenceID:        &quotation.ID,
			ReferenceCode:      quotation.Code,
			Stock:              stock,
			Quantity:           quotationProduct.Quantity,
			UnitPrice:          quotationProduct.UnitPrice,
			Unit:               quotationProduct.Unit,
			Discount:           quotationProduct.UnitDiscount,
			DiscountPercent:    quotationProduct.UnitDiscountPercent,
			CreatedAt:          quotation.CreatedAt,
			UpdatedAt:          quotation.UpdatedAt,
			WarehouseID:        quotationProduct.WarehouseID,
			WarehouseCode:      quotationProduct.WarehouseCode,
		}

		history.UnitPrice = RoundTo2Decimals(quotationProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo2Decimals(quotationProduct.UnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((quotationProduct.UnitPrice - quotationProduct.UnitDiscount) * quotationProduct.Quantity))
		history.Profit = RoundFloat(quotationProduct.Profit, 2)
		history.Loss = RoundFloat(quotationProduct.Loss, 2)

		history.VatPercent = RoundFloat(*quotation.VatPercent, 2)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))

		history.ID = primitive.NewObjectID()
		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(quotation.Date)
				if err != nil {
					return err
				}

				if quotation.Type == "invoice" {
					if store.Settings.UpdateProductStockOnQuotationSales {
						if store.IfStore2QuotationSalesShouldAffectTheStock(quotation.Date) {
							stock -= quotationProduct.Quantity
						}
					}
				}

				history := ProductHistory{
					Date:               quotation.Date,
					StoreID:            quotation.StoreID,
					StoreName:          quotation.StoreName,
					ProductID:          *setProduct.ProductID,
					CustomerID:         quotation.CustomerID,
					CustomerName:       quotation.CustomerName,
					CustomerNameArabic: quotation.CustomerNameArabic,
					ReferenceType:      referenceType,
					ReferenceID:        &quotation.ID,
					ReferenceCode:      quotation.Code,
					Stock:              stock,
					Quantity:           RoundTo2Decimals(quotationProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice:  RoundTo2Decimals(quotationProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:               setProductObj.Unit,
					UnitDiscount:       RoundTo2Decimals(quotationProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:           RoundTo2Decimals((quotationProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo2Decimals(quotationProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:    quotationProduct.UnitDiscountPercent,
					CreatedAt:          quotation.CreatedAt,
					UpdatedAt:          quotation.UpdatedAt,
					WarehouseID:        quotationProduct.WarehouseID,
					WarehouseCode:      quotationProduct.WarehouseCode,
				}

				history.UnitPrice = RoundTo2Decimals(quotationProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo2Decimals(quotationProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)
				history.Profit = RoundTo2Decimals(quotationProduct.Profit * (setProduct.RetailPricePercent / 100))
				history.Loss = RoundTo2Decimals(quotationProduct.Loss * (setProduct.RetailPricePercent / 100))

				history.VatPercent = RoundTo2Decimals(*quotation.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}
	}

	return nil
}

func (quotationsalesReturn *QuotationSalesReturn) CreateProductsHistory() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsHistoryExistsByReferenceID(&quotationsalesReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 1000*time.Second)
	defer cancel()

	for _, quotationsalesReturnProduct := range quotationsalesReturn.Products {
		if !quotationsalesReturnProduct.Selected {
			continue
		}

		product, err := store.FindProductByID(&quotationsalesReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		stock, err := product.GetProductQuantityBeforeOrEqualTo(quotationsalesReturn.Date)
		if err != nil {
			return err
		}

		if store.Settings.UpdateProductStockOnQuotationSales {
			if store.IfStore2QuotationSalesShouldAffectTheStock(quotationsalesReturn.Date) {
				stock += quotationsalesReturnProduct.Quantity
			}
		}

		history := ProductHistory{
			Date:               quotationsalesReturn.Date,
			StoreID:            quotationsalesReturn.StoreID,
			StoreName:          quotationsalesReturn.StoreName,
			ProductID:          quotationsalesReturnProduct.ProductID,
			CustomerID:         quotationsalesReturn.CustomerID,
			CustomerName:       quotationsalesReturn.CustomerName,
			CustomerNameArabic: quotationsalesReturn.CustomerNameArabic,
			ReferenceType:      "quotation_sales_return",
			ReferenceID:        &quotationsalesReturn.ID,
			ReferenceCode:      quotationsalesReturn.Code,
			Stock:              stock,
			Quantity:           quotationsalesReturnProduct.Quantity,
			UnitPrice:          quotationsalesReturnProduct.UnitPrice,
			Unit:               quotationsalesReturnProduct.Unit,
			Discount:           quotationsalesReturnProduct.UnitDiscount,
			DiscountPercent:    quotationsalesReturnProduct.UnitDiscountPercent,
			CreatedAt:          quotationsalesReturn.CreatedAt,
			UpdatedAt:          quotationsalesReturn.UpdatedAt,
		}

		history.UnitPrice = RoundTo2Decimals(quotationsalesReturnProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo2Decimals(quotationsalesReturnProduct.UnitPriceWithVAT)
		history.Price = RoundFloat(((quotationsalesReturnProduct.UnitPrice - quotationsalesReturnProduct.UnitDiscount) * quotationsalesReturnProduct.Quantity), 2)
		history.VatPercent = RoundFloat(*quotationsalesReturn.VatPercent, 2)
		history.VatPrice = RoundFloat((history.Price * (history.VatPercent / 100)), 2)
		history.NetPrice = RoundFloat((history.Price + history.VatPrice), 2)
		history.Profit = RoundFloat(quotationsalesReturnProduct.Profit, 2)
		history.Loss = RoundFloat(quotationsalesReturnProduct.Loss, 2)

		history.ID = primitive.NewObjectID()

		_, err = collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		go product.AdjustStockInHistoryAfter(history.Date)

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				stock, err := setProductObj.GetProductQuantityBeforeOrEqualTo(quotationsalesReturn.Date)
				if err != nil {
					return err
				}

				if store.Settings.UpdateProductStockOnQuotationSales {
					if store.IfStore2QuotationSalesShouldAffectTheStock(quotationsalesReturn.Date) {
						stock += quotationsalesReturnProduct.Quantity
					}
				}

				history := ProductHistory{
					Date:               quotationsalesReturn.Date,
					StoreID:            quotationsalesReturn.StoreID,
					StoreName:          quotationsalesReturn.StoreName,
					ProductID:          *setProduct.ProductID,
					CustomerID:         quotationsalesReturn.CustomerID,
					CustomerName:       quotationsalesReturn.CustomerName,
					CustomerNameArabic: quotationsalesReturn.CustomerNameArabic,
					ReferenceType:      "quotation_sales_return",
					ReferenceID:        &quotationsalesReturn.ID,
					ReferenceCode:      quotationsalesReturn.Code,
					Stock:              stock,
					Quantity:           RoundTo2Decimals(quotationsalesReturnProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice:  RoundTo2Decimals(quotationsalesReturnProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:               setProductObj.Unit,
					UnitDiscount:       RoundTo2Decimals(quotationsalesReturnProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:           RoundTo2Decimals((quotationsalesReturnProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo2Decimals(quotationsalesReturnProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:    quotationsalesReturnProduct.UnitDiscountPercent,
					CreatedAt:          quotationsalesReturn.CreatedAt,
					UpdatedAt:          quotationsalesReturn.UpdatedAt,
				}

				history.UnitPrice = RoundTo2Decimals(quotationsalesReturnProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo2Decimals(quotationsalesReturnProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)
				history.Profit = RoundTo2Decimals(quotationsalesReturnProduct.Profit * (setProduct.RetailPricePercent / 100))
				history.Loss = RoundTo2Decimals(quotationsalesReturnProduct.Loss * (setProduct.RetailPricePercent / 100))

				history.VatPercent = RoundTo2Decimals(*quotationsalesReturn.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}

				go setProductObj.AdjustStockInHistoryAfter(history.Date)
			}
		}
	}

	return nil
}

func (store *Store) IsHistoryExistsByReferenceID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"reference_id": ID,
	})

	if err != nil && err != mongo.ErrNoDocuments {
		return (count > 0), err
	}

	return (count > 0), nil
}

func (store *Store) GetHistoriesCountByProductID(productID *primitive.ObjectID) (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return collection.CountDocuments(ctx, bson.M{
		"product_id": productID,
	})
}

func (store *Store) GetHistoriesByProductID(productID *primitive.ObjectID) (models []ProductHistory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"product_id": productID}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching product  history" + err.Error())
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
		History := ProductHistory{}
		err = cur.Decode(&History)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")
		models = append(models, History)
	} //end for loop

	return models, nil
}

func (store *Store) ProcessHistory() error {
	log.Print("Processing  history")
	totalCount, err := store.GetTotalCount(bson.M{}, "product_history")
	if err != nil {
		return err
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
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
		model := ProductHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}
		/*
				order, err := FindOrderByID(model.OrderID, map[string]interface{}{})
				if err != nil {
					return errors.New("Error finding order:" + err.Error())
				}
				model.Date = order.Date
			err = model.Update()
			if err != nil {
				return errors.New("Error updating history:" + err.Error())
			}
		*/
		bar.Add(1)
	}

	log.Print(" DONE!")
	return nil
}

func (model *ProductHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": model.ID},
		bson.M{"$set": model},
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
