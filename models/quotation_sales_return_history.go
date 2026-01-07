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
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductQuotationSalesReturnHistory struct {
	ID                       primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                     *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID                  *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName                string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID                primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID               *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName             string              `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic       string              `json:"customer_name_arabic" bson:"customer_name_arabic"`
	QuotationID              *primitive.ObjectID `json:"quotation_id,omitempty" bson:"quotation_id,omitempty"`
	QuotationCode            string              `json:"quotation_code,omitempty" bson:"quotation_code,omitempty"`
	QuotationSalesReturnID   *primitive.ObjectID `json:"quotation_sales_return_id,omitempty" bson:"quotation_sales_return_id,omitempty"`
	QuotationSalesReturnCode string              `json:"quotation_sales_return_code,omitempty" bson:"quotation_sales_return_code,omitempty"`
	Quantity                 float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	PurchaseUnitPrice        float64             `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	UnitPrice                float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	UnitPriceWithVAT         float64             `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	UnitDiscount             float64             `bson:"unit_discount" json:"unit_discount"`
	Discount                 float64             `bson:"discount" json:"discount"`
	DiscountPercent          float64             `bson:"discount_percent" json:"discount_percent"`
	Price                    float64             `bson:"price,omitempty" json:"price,omitempty"`
	NetPrice                 float64             `bson:"net_price" json:"net_price"`
	Profit                   float64             `bson:"profit" json:"profit"`
	Loss                     float64             `bson:"loss" json:"loss"`
	VatPercent               float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice                 float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	Unit                     string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Store                    *Store              `json:"store,omitempty"`
	Customer                 *Customer           `json:"customer,omitempty"`
	CreatedAt                *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt                *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	WarehouseID              *primitive.ObjectID `json:"warehouse_id" bson:"warehouse_id"`
	WarehouseCode            *string             `json:"warehouse_code" bson:"warehouse_code"`
}

type QuotationSalesReturnHistoryStats struct {
	ID                        *primitive.ObjectID `json:"id" bson:"_id"`
	TotalQuotationSalesReturn float64             `json:"total_quotation_sales_return" bson:"total_quotation_sales_return"`
	TotalProfit               float64             `json:"total_profit" bson:"total_profit"`
	TotalLoss                 float64             `json:"total_loss" bson:"total_loss"`
	TotalVatReturn            float64             `json:"total_vat_return" bson:"total_vat_return"`
	TotalQuantity             float64             `json:"total_quantity" bson:"total_quantity"`
}

func (store *Store) GetQuotationSalesReturnHistoryStats(filter map[string]interface{}) (stats QuotationSalesReturnHistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Print(filter)

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                          nil,
				"total_quotation_sales_return": bson.M{"$sum": "$net_price"},
				"total_profit":                 bson.M{"$sum": "$profit"},
				"total_loss":                   bson.M{"$sum": "$loss"},
				"total_vat_return":             bson.M{"$sum": "$vat_price"},
				"total_quantity":               bson.M{"$sum": "$quantity"},
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
		stats.TotalQuotationSalesReturn = RoundFloat(stats.TotalQuotationSalesReturn, 2)
		stats.TotalProfit = RoundFloat(stats.TotalProfit, 2)
		stats.TotalLoss = RoundFloat(stats.TotalLoss, 2)
		stats.TotalVatReturn = RoundFloat(stats.TotalVatReturn, 2)
	}

	return stats, nil
}

func (store *Store) SearchQuotationSalesReturnHistory(w http.ResponseWriter, r *http.Request) (models []ProductQuotationSalesReturnHistory, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[net_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return models, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_price"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_price"] = float64(value)
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

	keys, ok = r.URL.Query()["search[warehouse_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["warehouse_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	keys, ok = r.URL.Query()["search[quotation_id]"]
	if ok && len(keys[0]) >= 1 {
		quotationID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["quotation_id"] = quotationID
	}

	keys, ok = r.URL.Query()["search[quotation_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["quotation_code"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_id]"]
	if ok && len(keys[0]) >= 1 {
		quotationID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["quotation_sales_return_id"] = quotationID
	}

	keys, ok = r.URL.Query()["search[quotation_sales_return_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["quotation_sales_return_code"] = keys[0]
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_sales_return_history")
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
		return models, criterias, errors.New("Error fetching product quotationsales return history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductQuotationSalesReturnHistory{}
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

func (quotationsalesReturn *QuotationSalesReturn) CreateProductsQuotationSalesReturnHistory() error {
	store, err := FindStoreByID(quotationsalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsQuotationSalesReturnHistoryExistsByQuotationSalesReturnID(&quotationsalesReturn.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("product_quotation_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, quotationsalesReturnProduct := range quotationsalesReturn.Products {
		if !quotationsalesReturnProduct.Selected {
			continue
		}

		warehouseCode := ""
		if quotationsalesReturnProduct.WarehouseCode != nil {
			warehouseCode = *quotationsalesReturnProduct.WarehouseCode
		}

		if warehouseCode == "" {
			warehouseCode = "main_store"
		}

		history := ProductQuotationSalesReturnHistory{
			Date:                     quotationsalesReturn.Date,
			StoreID:                  quotationsalesReturn.StoreID,
			StoreName:                quotationsalesReturn.StoreName,
			ProductID:                quotationsalesReturnProduct.ProductID,
			CustomerID:               quotationsalesReturn.CustomerID,
			CustomerName:             quotationsalesReturn.CustomerName,
			CustomerNameArabic:       quotationsalesReturn.CustomerNameArabic,
			QuotationID:              quotationsalesReturn.QuotationID,
			QuotationCode:            quotationsalesReturn.QuotationCode,
			QuotationSalesReturnID:   &quotationsalesReturn.ID,
			QuotationSalesReturnCode: quotationsalesReturn.Code,
			Quantity:                 quotationsalesReturnProduct.Quantity,
			UnitPrice:                quotationsalesReturnProduct.UnitPrice,
			Unit:                     quotationsalesReturnProduct.Unit,
			Discount:                 quotationsalesReturnProduct.UnitDiscount,
			DiscountPercent:          quotationsalesReturnProduct.UnitDiscountPercent,
			CreatedAt:                quotationsalesReturn.CreatedAt,
			UpdatedAt:                quotationsalesReturn.UpdatedAt,
			WarehouseID:              quotationsalesReturnProduct.WarehouseID,
			WarehouseCode:            &warehouseCode,
		}

		history.UnitPrice = RoundTo8Decimals(quotationsalesReturnProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo8Decimals(quotationsalesReturnProduct.UnitPriceWithVAT)
		history.Price = RoundFloat(((quotationsalesReturnProduct.UnitPrice - quotationsalesReturnProduct.UnitDiscount) * quotationsalesReturnProduct.Quantity), 2)
		history.VatPercent = RoundFloat(*quotationsalesReturn.VatPercent, 2)
		history.VatPrice = RoundFloat((history.Price * (history.VatPercent / 100)), 2)
		history.NetPrice = RoundFloat((history.Price + history.VatPrice), 2)
		history.Profit = RoundFloat(quotationsalesReturnProduct.Profit, 2)
		history.Loss = RoundFloat(quotationsalesReturnProduct.Loss, 2)

		history.ID = primitive.NewObjectID()

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		product, err := store.FindProductByID(&quotationsalesReturnProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				history := ProductQuotationSalesReturnHistory{
					Date:                     quotationsalesReturn.Date,
					StoreID:                  quotationsalesReturn.StoreID,
					StoreName:                quotationsalesReturn.StoreName,
					ProductID:                *setProduct.ProductID,
					CustomerID:               quotationsalesReturn.CustomerID,
					CustomerName:             quotationsalesReturn.CustomerName,
					CustomerNameArabic:       quotationsalesReturn.CustomerNameArabic,
					QuotationSalesReturnID:   &quotationsalesReturn.ID,
					QuotationSalesReturnCode: quotationsalesReturn.Code,
					QuotationID:              quotationsalesReturn.QuotationID,
					QuotationCode:            quotationsalesReturn.QuotationCode,
					Quantity:                 RoundTo8Decimals(quotationsalesReturnProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice:        RoundTo4Decimals(quotationsalesReturnProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:                     setProductObj.Unit,
					UnitDiscount:             RoundTo8Decimals(quotationsalesReturnProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:                 RoundTo8Decimals((quotationsalesReturnProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo8Decimals(quotationsalesReturnProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:          quotationsalesReturnProduct.UnitDiscountPercent,
					CreatedAt:                quotationsalesReturn.CreatedAt,
					UpdatedAt:                quotationsalesReturn.UpdatedAt,
					WarehouseID:              quotationsalesReturnProduct.WarehouseID,
					WarehouseCode:            quotationsalesReturnProduct.WarehouseCode,
				}

				history.UnitPrice = RoundTo8Decimals(quotationsalesReturnProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo8Decimals(quotationsalesReturnProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)
				history.Profit = RoundTo4Decimals(quotationsalesReturnProduct.Profit * (setProduct.RetailPricePercent / 100))
				history.Loss = RoundTo4Decimals(quotationsalesReturnProduct.Loss * (setProduct.RetailPricePercent / 100))

				history.VatPercent = RoundTo2Decimals(*quotationsalesReturn.VatPercent)
				history.VatPrice = RoundTo2Decimals(history.Price * (history.VatPercent / 100))
				history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
				history.ID = primitive.NewObjectID()

				_, err = collection.InsertOne(ctx, &history)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (store *Store) IsQuotationSalesReturnHistoryExistsByQuotationSalesReturnID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"quotation_sales_return_id": ID,
	})

	return (count > 0), err
}

func (quotationsalesReturn *QuotationSalesReturn) ClearProductsQuotationSalesReturnHistory() error {
	//log.Printf("Clearing QuotationSales history of quotation id:%s", quotation.Code)
	collection := db.GetDB("store_" + quotationsalesReturn.StoreID.Hex()).Collection("product_quotation_sales_return_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"quotation_sales_return_id": quotationsalesReturn.ID})
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) ProcessQuotationSalesReturnHistory() error {
	log.Print("Processing quotationsales return history")
	totalCount, err := store.GetTotalCount(bson.M{}, "product_quotation_sales_return_history")
	if err != nil {
		return err
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_sales_return_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching quotationsales return history:" + err.Error())
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
		model := ProductQuotationSalesReturnHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		quotationsalesReturn, err := store.FindQuotationSalesReturnByID(model.QuotationSalesReturnID, map[string]interface{}{})
		if err != nil {
			return errors.New("Error finding quotation:" + err.Error())
		}
		model.Date = quotationsalesReturn.Date
		err = model.Update()
		if err != nil {
			return errors.New("Error updating quotationsales return history:" + err.Error())
		}
		bar.Add(1)
	}

	log.Print("QuotationSales return history DONE!")
	return nil
}

func (model *ProductQuotationSalesReturnHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_quotation_sales_return_history")
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
