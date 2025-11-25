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
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type StockTransferProduct struct {
	ProductID                  primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name                       string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic               string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode                   string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PrefixPartNumber           string             `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber                 string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity                   float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	UnitPrice                  float64            `bson:"unit_price" json:"unit_price"`
	UnitPriceWithVAT           float64            `bson:"unit_price_with_vat" json:"unit_price_with_vat"`
	PurchaseUnitPrice          float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	PurchaseUnitPriceWithVAT   float64            `bson:"purchase_unit_price_with_vat,omitempty" json:"purchase_unit_price_with_vat,omitempty"`
	Unit                       string             `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitDiscount               float64            `bson:"unit_discount" json:"unit_discount"`
	UnitDiscountWithVAT        float64            `bson:"unit_discount_with_vat" json:"unit_discount_with_vat"`
	UnitDiscountPercent        float64            `bson:"unit_discount_percent" json:"unit_discount_percent"`
	UnitDiscountPercentWithVAT float64            `bson:"unit_discount_percent_with_vat" json:"unit_discount_percent_with_vat"`
	/*LineTotal                  float64            `bson:"line_total" json:"line_total"`
	LineTotalWithVAT           float64            `bson:"line_total_with_vat" json:"line_total_with_vat"`
	ActualLineTotal            float64            `bson:"actual_line_total" json:"actual_line_total"`
	ActualLineTotalWithVAT     float64            `bson:"actual_line_total_with_vat" json:"actual_line_total_with_vat"`
	*/
	Profit float64 `bson:"profit" json:"profit"`
	Loss   float64 `bson:"loss" json:"loss"`
}

// StockTransfer : StockTransfer structure
type StockTransfer struct {
	ID                 primitive.ObjectID     `json:"id,omitempty" bson:"_id,omitempty"`
	Date               *time.Time             `bson:"date,omitempty" json:"date,omitempty"`
	DateStr            string                 `json:"date_str,omitempty" bson:"-"`
	InvoiceCountValue  int64                  `bson:"invoice_count_value,omitempty" json:"invoice_count_value,omitempty"`
	Code               string                 `bson:"code,omitempty" json:"code,omitempty"`
	UUID               string                 `bson:"uuid,omitempty" json:"uuid,omitempty"`
	Hash               string                 `bson:"hash,omitempty" json:"hash,omitempty"`
	PrevHash           string                 `bson:"prev_hash,omitempty" json:"prev_hash,omitempty"`
	StoreID            *primitive.ObjectID    `json:"store_id,omitempty" bson:"store_id,omitempty"`
	FromWarehouseID    *primitive.ObjectID    `json:"from_warehouse_id" bson:"from_warehouse_id"`
	FromWarehouseCode  *string                `json:"from_warehouse_code" bson:"from_warehouse_code"`
	ToWarehouseID      *primitive.ObjectID    `json:"to_warehouse_id" bson:"to_warehouse_id"`
	ToWarehouseCode    *string                `json:"to_warehouse_code" bson:"to_warehouse_code"`
	Store              *Store                 `json:"store,omitempty" bson:"-"`
	Products           []StockTransferProduct `bson:"products,omitempty" json:"products,omitempty"`
	VatPercent         *float64               `bson:"vat_percent" json:"vat_percent"`
	TotalQuantity      float64                `bson:"total_quantity" json:"total_quantity"`
	VatPrice           float64                `bson:"vat_price" json:"vat_price"`
	Total              float64                `bson:"total" json:"total"`
	TotalWithVAT       float64                `bson:"total_with_vat" json:"total_with_vat"`
	NetTotal           float64                `bson:"net_total" json:"net_total"`
	ActualVatPrice     float64                `bson:"actual_vat_price" json:"actual_vat_price"`
	ActualTotal        float64                `bson:"actual_total" json:"actual_total"`
	ActualTotalWithVAT float64                `bson:"actual_total_with_vat" json:"actual_total_with_vat"`
	ActualNetTotal     float64                `bson:"actual_net_total" json:"actual_net_total"`
	RoundingAmount     float64                `bson:"rounding_amount" json:"rounding_amount"`
	AutoRoundingAmount bool                   `bson:"auto_rounding_amount" json:"auto_rounding_amount"`
	CreatedAt          *time.Time             `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt          *time.Time             `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy          *primitive.ObjectID    `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy          *primitive.ObjectID    `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser      *User                  `json:"created_by_user,omitempty" bson:"-"`
	UpdatedByUser      *User                  `json:"updated_by_user,omitempty" bson:"-"`
	CreatedByName      string                 `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName      string                 `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	Remarks            string                 `bson:"remarks" json:"remarks"`
}

func (stocktransfer *StockTransfer) AttributesValueChangeEvent(stocktransferOld *StockTransfer) error {

	//if stocktransfer.Status != stocktransferOld.Status {
	/*
		stocktransfer.SetChangeLog(
			"attribute_value_change",
			"status",
			stocktransferOld.Status,
			stocktransfer.Status,
		)
	*/

	//if stocktransfer.Status == "delivered" || stocktransfer.Status == "dispatched" {
	/*
		err := stocktransferOld.AddStock()
		if err != nil {
			return err
		}

		err = stocktransfer.RemoveStock()
		if err != nil {
			return err
		}
	*/
	//}
	//}

	return nil
}

func (stocktransfer *StockTransfer) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if stocktransfer.CreatedBy != nil {
		createdByUser, err := FindUserByID(stocktransfer.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		stocktransfer.CreatedByName = createdByUser.Name
	}

	if stocktransfer.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(stocktransfer.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		stocktransfer.UpdatedByName = updatedByUser.Name
	}

	for i, product := range stocktransfer.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1})
		if err != nil {
			return err
		}
		//stocktransfer.Products[i].Name = productObject.Name
		stocktransfer.Products[i].NameInArabic = productObject.NameInArabic
		stocktransfer.Products[i].ItemCode = productObject.ItemCode
		//stocktransfer.Products[i].PartNumber = productObject.PartNumber
		stocktransfer.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

func (stocktransfer *StockTransfer) FindNetTotal() {
	stocktransfer.FindTotal()

	/*
		if stocktransfer.DiscountWithVAT > 0 {
			stocktransfer.Discount = RoundTo2Decimals(stocktransfer.DiscountWithVAT / (1 + (*stocktransfer.VatPercent / 100)))
		} else if stocktransfer.Discount > 0 {
			stocktransfer.DiscountWithVAT = RoundTo2Decimals(stocktransfer.Discount * (1 + (*stocktransfer.VatPercent / 100)))
		} else {
			stocktransfer.Discount = 0
			stocktransfer.DiscountWithVAT = 0
		}
	*/

	// Apply discount to the base amount first
	baseTotal := stocktransfer.Total
	//baseTotal = RoundTo8Decimals(baseTotal)
	baseTotal = RoundTo2Decimals(baseTotal)

	// Now calculate VAT on the discounted base
	stocktransfer.VatPrice = RoundTo2Decimals(baseTotal * (*stocktransfer.VatPercent / 100))

	//log.Print(baseTotal + stocktransfer.VatPrice)
	stocktransfer.NetTotal = RoundTo2Decimals(baseTotal + stocktransfer.VatPrice)

	//Actual
	actualBaseTotal := stocktransfer.ActualTotal
	actualBaseTotal = RoundTo8Decimals(actualBaseTotal)

	// Now calculate VAT on the discounted base
	stocktransfer.ActualVatPrice = RoundTo2Decimals(actualBaseTotal * (*stocktransfer.VatPercent / 100))
	stocktransfer.ActualNetTotal = RoundTo2Decimals(actualBaseTotal + stocktransfer.ActualVatPrice)

	if stocktransfer.AutoRoundingAmount {
		stocktransfer.RoundingAmount = RoundTo2Decimals(stocktransfer.ActualNetTotal - stocktransfer.NetTotal)
	}

	stocktransfer.NetTotal = RoundTo2Decimals(stocktransfer.NetTotal + stocktransfer.RoundingAmount)
}

func (stocktransfer *StockTransfer) FindTotal() {
	total := float64(0.0)
	totalWithVAT := float64(0.0)
	//Actual
	actualTotal := float64(0.0)
	actualTotalWithVAT := float64(0.0)

	for i, product := range stocktransfer.Products {

		total += (product.Quantity * (stocktransfer.Products[i].UnitPrice - stocktransfer.Products[i].UnitDiscount))
		total = RoundTo2Decimals(total)

		totalWithVAT += (product.Quantity * (stocktransfer.Products[i].UnitPriceWithVAT - stocktransfer.Products[i].UnitDiscountWithVAT))
		totalWithVAT = RoundTo2Decimals(totalWithVAT)

		//Actual values
		actualTotal += (product.Quantity * (stocktransfer.Products[i].UnitPrice - stocktransfer.Products[i].UnitDiscount))
		actualTotal = RoundTo8Decimals(actualTotal)
		actualTotalWithVAT += (product.Quantity * (stocktransfer.Products[i].UnitPriceWithVAT - stocktransfer.Products[i].UnitDiscountWithVAT))
		actualTotalWithVAT = RoundTo8Decimals(actualTotalWithVAT)

	}

	stocktransfer.Total = total
	stocktransfer.TotalWithVAT = totalWithVAT

	//Actual
	stocktransfer.ActualTotal = actualTotal
	stocktransfer.ActualTotalWithVAT = actualTotalWithVAT
}

func (stocktransfer *StockTransfer) FindTotalQuantity() {
	totalQuantity := float64(0.0)
	for _, product := range stocktransfer.Products {
		totalQuantity += product.Quantity
	}
	stocktransfer.TotalQuantity = totalQuantity
}

type StockTransferStats struct {
	ID            *primitive.ObjectID `json:"id" bson:"_id"`
	NetTotal      float64             `json:"net_total" bson:"net_total"`
	TotalQuantity float64             `json:"total_quantity" bson:"total_quantity"`
}

func (store *Store) GetStockTransferStats(filter map[string]interface{}) (stats StockTransferStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":            nil,
				"net_total":      bson.M{"$sum": "$net_total"},
				"total_quantity": bson.M{"$sum": "$total_quantity"},
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

func (store *Store) GetAllStockTransfers() (stocktransfers []StockTransfer, err error) {

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
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
		return stocktransfers, errors.New("Error fetching stocktransfers:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return stocktransfers, errors.New("Cursor error:" + err.Error())
		}
		stocktransfer := StockTransfer{}
		err = cur.Decode(&stocktransfer)
		if err != nil {
			return stocktransfers, errors.New("Cursor decode error:" + err.Error())
		}
		stocktransfers = append(stocktransfers, stocktransfer)
	} //end for loop

	return stocktransfers, nil
}

func (store *Store) SearchStockTransfer(w http.ResponseWriter, r *http.Request) (stocktransfers []StockTransfer, criterias SearchCriterias, err error) {

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
			return stocktransfers, criterias, err
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
			return stocktransfers, criterias, err
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
			return stocktransfers, criterias, err
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
			return stocktransfers, criterias, err
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
			return stocktransfers, criterias, err
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
			return stocktransfers, criterias, err
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
			return stocktransfers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["net_total"] = value
		}
	}

	keys, ok = r.URL.Query()["search[total_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return stocktransfers, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["total_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["total_quantity"] = value
		}
	}

	keys, ok = r.URL.Query()["search[from_warehouse_id]"]
	if ok && len(keys[0]) >= 1 {

		warehouseIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range warehouseIds {
			warehouseID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return stocktransfers, criterias, err
			}
			objecIds = append(objecIds, warehouseID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["from_warehouse_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[from_warehouse_code]"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.TrimSpace(keys[0])
		if strings.HasPrefix("main store", keys[0]) {
			criterias.SearchBy["from_warehouse_code"] = bson.M{"$in": []interface{}{nil, ""}}
		} else {
			criterias.SearchBy["from_warehouse_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		}
	}

	keys, ok = r.URL.Query()["search[to_warehouse_code]"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.TrimSpace(keys[0])
		if strings.HasPrefix("main store", keys[0]) {
			criterias.SearchBy["to_warehouse_code"] = bson.M{"$in": []interface{}{nil, ""}}
		} else {
			criterias.SearchBy["to_warehouse_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		}

	}

	keys, ok = r.URL.Query()["search[to_warehouse_id]"]
	if ok && len(keys[0]) >= 1 {

		warehouseIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range warehouseIds {
			warehouseID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return stocktransfers, criterias, err
			}
			objecIds = append(objecIds, warehouseID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["to_warehouse_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return stocktransfers, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return stocktransfers, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	storeSelectFields := map[string]interface{}{}
	//warehouseSelectFields := map[string]interface{}{}
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

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
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
		return stocktransfers, criterias, errors.New("Error fetching stocktransfers:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return stocktransfers, criterias, errors.New("Cursor error:" + err.Error())
		}
		stocktransfer := StockTransfer{}
		err = cur.Decode(&stocktransfer)
		if err != nil {
			return stocktransfers, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			stocktransfer.Store, _ = FindStoreByID(stocktransfer.StoreID, storeSelectFields)
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			stocktransfer.CreatedByUser, _ = FindUserByID(stocktransfer.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			stocktransfer.UpdatedByUser, _ = FindUserByID(stocktransfer.UpdatedBy, updatedByUserSelectFields)
		}
		/*
			if _, ok := criterias.Select["deleted_by_user.id"]; ok {
				stocktransfer.DeletedByUser, _ = FindUserByID(stocktransfer.DeletedBy, deletedByUserSelectFields)
			}
		*/
		stocktransfers = append(stocktransfers, stocktransfer)
	} //end for loop

	return stocktransfers, criterias, nil
}

func (stocktransfer *StockTransfer) Validate(w http.ResponseWriter, r *http.Request, scenario string, oldStockTransfer *StockTransfer) (errs map[string]string) {
	errs = make(map[string]string)
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		errs["store_id"] = "invalid store id"
	}

	var fromWarehouse *Warehouse
	var toWarehouse *Warehouse

	if stocktransfer.FromWarehouseID != nil && !stocktransfer.FromWarehouseID.IsZero() {
		fromWarehouse, err = store.FindWarehouseByID(stocktransfer.FromWarehouseID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			errs["from_warehouse_id"] = "Invalid warehouse"
			return errs
		}
	}

	if stocktransfer.ToWarehouseID != nil && !stocktransfer.ToWarehouseID.IsZero() {
		toWarehouse, err = store.FindWarehouseByID(stocktransfer.ToWarehouseID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			errs["to_warehouse_id"] = "Invalid warehouse"
			return errs
		}
	}

	if fromWarehouse == nil && toWarehouse == nil {
		errs["from_warehouse_id"] = "Both From & To warehouse cannot be Main Store"
		errs["to_warehouse_id"] = "Both From & To warehouse cannot be Main Store"
	}

	if fromWarehouse != nil && toWarehouse != nil && fromWarehouse.ID.Hex() == toWarehouse.ID.Hex() {
		errs["to_warehouse_id"] = "Choose different warehouse"
	}

	if govalidator.IsNull(stocktransfer.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, stocktransfer.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		stocktransfer.Date = &date
	}

	if scenario == "update" {
		if stocktransfer.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsStockTransferExists(&stocktransfer.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid StockTransfer:" + stocktransfer.ID.Hex()
		}

	}

	if stocktransfer.StoreID == nil || stocktransfer.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(stocktransfer.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + stocktransfer.StoreID.Hex()
			return errs
		}
	}

	if len(stocktransfer.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for stock transfer"
	}

	for index, product := range stocktransfer.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for stock transfer"
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

		if product.UnitPrice == 0 {
			errs["unit_price_"+strconv.Itoa(index)] = "Unit Price is required"
		}
	}

	if stocktransfer.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (stocktransfer *StockTransfer) SetProductsStock() (err error) {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if len(stocktransfer.Products) == 0 {
		return nil
	}

	for _, stocktransferProduct := range stocktransfer.Products {
		product, err := store.FindProductByID(&stocktransferProduct.ProductID, bson.M{})
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

func (stocktransfer *StockTransfer) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": stocktransfer.StoreID}, "stocktransfer")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (stocktransfer *StockTransfer) Update() error {
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": stocktransfer.ID},
		bson.M{"$set": stocktransfer},
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

func (stocktransfer *StockTransfer) Insert() error {
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stocktransfer.ID = primitive.NewObjectID()

	_, err := collection.InsertOne(ctx, &stocktransfer)
	if err != nil {
		return err
	}

	return nil
}

func (stocktransfer *StockTransfer) MakeRedisCode() error {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := stocktransfer.StoreID.Hex() + "_stocktransfer_counter" // Global counter key

	// === 1. Get location from store.CountryCode ===
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// === 2. Get date from stocktransfer.CreatedAt or fallback to stocktransfer.Date or now ===
	baseTime := stocktransfer.CreatedAt.In(location)

	// === 3. Always ensure global counter exists ===
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		count, err := store.GetCountByCollection("stocktransfer")
		if err != nil {
			return err
		}
		startFrom := store.StockTransferSerialNumber.StartFromCount
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

	// === 5. Determine which counter to use for stocktransfer.Code ===
	useMonthly := strings.Contains(store.StockTransferSerialNumber.Prefix, "DATE")
	var serialNumber int64 = globalIncr

	if useMonthly {
		// Generate monthly redis key
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := stocktransfer.StoreID.Hex() + "_stocktransfer_counter_" + monthKey

		// Ensure monthly counter exists
		monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result()
		if err != nil {
			return err
		}
		if monthlyExists == 0 {
			startFrom := store.StockTransferSerialNumber.StartFromCount
			fromDate := time.Date(baseTime.Year(), baseTime.Month(), 1, 0, 0, 0, 0, location)
			toDate := fromDate.AddDate(0, 1, 0).Add(-time.Nanosecond)

			monthlyCount, err := store.GetCountByCollectionInRange(fromDate, toDate, "stocktransfer")
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
	paddingCount := store.StockTransferSerialNumber.PaddingCount
	if store.StockTransferSerialNumber.Prefix != "" {
		stocktransfer.Code = fmt.Sprintf("%s-%0*d", store.StockTransferSerialNumber.Prefix, paddingCount, serialNumber)
	} else {
		stocktransfer.Code = fmt.Sprintf("%0*d", paddingCount, serialNumber)
	}

	// === 7. Replace DATE token if used ===
	if strings.Contains(stocktransfer.Code, "DATE") {
		stocktransferDate := baseTime.Format("20060102") // YYYYMMDD
		stocktransfer.Code = strings.ReplaceAll(stocktransfer.Code, "DATE", stocktransferDate)
	}

	// === 8. Set InvoiceCountValue (based on global counter) ===
	stocktransfer.InvoiceCountValue = globalIncr - (store.StockTransferSerialNumber.StartFromCount - 1)

	return nil
}

func (stocktransfer *StockTransfer) UnMakeRedisCode() error {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	// Global counter key
	redisKey := stocktransfer.StoreID.Hex() + "_stocktransfer_counter"

	// Get location from store.CountryCode
	location := time.UTC
	if timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]; ok {
		loc, err := time.LoadLocation(timeZone)
		if err == nil {
			location = loc
		}
	}

	// Use CreatedAt, or fallback to now
	baseTime := stocktransfer.CreatedAt.In(location)

	// Always try to decrement global counter
	if exists, err := db.RedisClient.Exists(redisKey).Result(); err == nil && exists != 0 {
		if _, err := db.RedisClient.Decr(redisKey).Result(); err != nil {
			return err
		}
	}

	// Decrement monthly counter only if Prefix contains "DATE"
	if strings.Contains(store.StockTransferSerialNumber.Prefix, "DATE") {
		monthKey := baseTime.Format("200601") // e.g., 202505
		monthlyRedisKey := stocktransfer.StoreID.Hex() + "_stocktransfer_counter_" + monthKey

		if monthlyExists, err := db.RedisClient.Exists(monthlyRedisKey).Result(); err == nil && monthlyExists != 0 {
			if _, err := db.RedisClient.Decr(monthlyRedisKey).Result(); err != nil {
				return err
			}
		}
	}

	return nil
}

func (stocktransfer *StockTransfer) MakeCode() error {
	return stocktransfer.MakeRedisCode()
}

func (stocktransfer *StockTransfer) UnMakeCode() error {
	return stocktransfer.UnMakeRedisCode()
}

func (store *Store) FindLastStockTransferByStoreID(
	storeID *primitive.ObjectID,
	selectFields map[string]interface{},
) (stocktransfer *StockTransfer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"created_at": -1})

	err = collection.FindOne(ctx,
		bson.M{"store_id": storeID}, findOneOptions).
		Decode(&stocktransfer)
	if err != nil {
		return nil, err
	}

	return stocktransfer, err
}

func (stocktransfer *StockTransfer) IsCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if stocktransfer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": stocktransfer.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": stocktransfer.Code,
			"_id":  bson.M{"$ne": stocktransfer.ID},
		})
	}

	return (count > 0), err
}

func (stocktransfer *StockTransfer) DeleteStockTransfer(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = stocktransfer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	/*
		userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
		if err != nil {
			return err
		}


			stocktransfer.Deleted = true
			stocktransfer.DeletedBy = &userID
			now := time.Now()
			stocktransfer.DeletedAt = &now
	*/

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": stocktransfer.ID},
		bson.M{"$set": stocktransfer},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindStockTransferByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (stocktransfer *StockTransfer, err error) {

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
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
		Decode(&stocktransfer)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "store")
		stocktransfer.Store, _ = FindStoreByID(stocktransfer.StoreID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		stocktransfer.CreatedByUser, _ = FindUserByID(stocktransfer.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		stocktransfer.UpdatedByUser, _ = FindUserByID(stocktransfer.UpdatedBy, fields)
	}

	/*
		if _, ok := selectFields["deleted_by_user.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
			stocktransfer.DeletedByUser, _ = FindUserByID(stocktransfer.DeletedBy, fields)
		}*/

	return stocktransfer, err
}

func (store *Store) FindStockTransferByCode(
	Code string,
	selectFields map[string]interface{},
) (stocktransfer *StockTransfer, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"code":     Code,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&stocktransfer)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "store")
		stocktransfer.Store, _ = FindStoreByID(stocktransfer.StoreID, fields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		stocktransfer.CreatedByUser, _ = FindUserByID(stocktransfer.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		stocktransfer.UpdatedByUser, _ = FindUserByID(stocktransfer.UpdatedBy, fields)
	}

	/*
		if _, ok := selectFields["deleted_by_user.id"]; ok {
			fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
			stocktransfer.DeletedByUser, _ = FindUserByID(stocktransfer.DeletedBy, fields)
		}*/

	return stocktransfer, err
}

func (stocktransfer *StockTransfer) FindNextStockTransfer(selectFields map[string]interface{}) (nextStockTransfer *StockTransfer, err error) {
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	findOneOptions.SetSort(bson.M{"date": 1})
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"date":     bson.M{"$gte": stocktransfer.Date},
			"_id":      bson.M{"$ne": stocktransfer.ID},
			"store_id": stocktransfer.StoreID,
		}, findOneOptions).
		Decode(&nextStockTransfer)
	if err != nil {
		return nil, err
	}

	return nextStockTransfer, err
}

func (stocktransfer *StockTransfer) FindPreviousStockTransfer(selectFields map[string]interface{}) (previousStockTransfer *StockTransfer, err error) {
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"invoice_count_value": (stocktransfer.InvoiceCountValue - 1),
			"store_id":            stocktransfer.StoreID,
		}, findOneOptions).
		Decode(&previousStockTransfer)
	if err != nil {
		return nil, err
	}

	return previousStockTransfer, err
}

func (store *Store) IsStockTransferExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}
func (stocktransfer *StockTransfer) HardDelete() error {
	log.Print("Delete stocktransfer")
	ctx := context.Background()
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("stocktransfer")
	_, err := collection.DeleteOne(ctx, bson.M{
		"_id": stocktransfer.ID,
	})
	if err != nil {
		return err
	}

	return nil
}

func ProcessStockTransfers() error {
	log.Print("Processing stock transfers")
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
		}, "stocktransfer")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("stocktransfer")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetSort(bson.M{"date": 1})
		findOptions.SetNoCursorTimeout(true)
		findOptions.SetAllowDiskUse(true)
		//findOptions.SetSort(GetSortByFields("created_at"))

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
			stocktransfer := StockTransfer{}
			err = cur.Decode(&stocktransfer)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			if stocktransfer.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			bar.Add(1)
		}

	}

	log.Print("StockTransfers DONE!")
	return nil
}

type ProductStockTransferStats struct {
	StockTransferCount    int64   `json:"stocktransfer_count" bson:"stocktransfer_count"`
	StockTransferQuantity float64 `json:"stocktransfer_quantity" bson:"stocktransfer_quantity"`
	StockTransferAmount   float64 `json:"stocktransfer_amount" bson:"stocktransfer_amount"`
}

func (product *Product) SetProductStockTransferStats() error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_stocktransfer_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductStockTransferStats

	filter := map[string]interface{}{
		"store_id":   product.StoreID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"stocktransfer_count":    bson.M{"$sum": 1},
				"stocktransfer_quantity": bson.M{"$sum": "$quantity"},
				"stocktransfer_amount":   bson.M{"$sum": "$net_price"},
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
		stats.StockTransferAmount = RoundFloat(stats.StockTransferAmount, 2)
	}

	if productStoreTemp, ok := product.ProductStores[product.StoreID.Hex()]; ok {
		productStoreTemp.StockTransferCount = stats.StockTransferCount
		productStoreTemp.StockTransferQuantity = stats.StockTransferQuantity
		productStoreTemp.StockTransferAmount = stats.StockTransferAmount
		product.ProductStores[product.StoreID.Hex()] = productStoreTemp
	}

	return nil
}

func (product *Product) SetProductStockTransferQuantity() error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_stocktransfer_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductStockTransferStats

	filter := map[string]interface{}{
		"store_id":   product.StoreID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"stocktransfer_quantity": bson.M{"$sum": "$quantity"},
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

	if productStoreTemp, ok := product.ProductStores[product.StoreID.Hex()]; ok {
		productStoreTemp.StockTransferQuantity = stats.StockTransferQuantity
		product.ProductStores[product.StoreID.Hex()] = productStoreTemp
	}

	return nil
}

func (stocktransfer *StockTransfer) SetProductsStockTransferStats() error {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, stocktransferProduct := range stocktransfer.Products {
		product, err := store.FindProductByID(&stocktransferProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductStockTransferStats()
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

				err = setProductObj.SetProductStockTransferStats()
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

//Customer

type WarehouseStockTransferStats struct {
	StockTransferSentCount        int64   `json:"stocktransfer_sent_count" bson:"stocktransfer_sent_count"`
	StockTransferSentQuantity     float64 `json:"stocktransfer_sent_quantity" bson:"stocktransfer_sent_quantity"`
	StockTransferSentAmount       float64 `json:"stocktransfer_sent_amount" bson:"stocktransfer_sent_amount"`
	StockTransferReceivedCount    int64   `json:"stocktransfer_received_count" bson:"stocktransfer_received_count"`
	StockTransferReceivedQuantity float64 `json:"stocktransfer_received_quantity" bson:"stocktransfer_received_quantity"`
	StockTransferReceivedAmount   float64 `json:"stocktransfer_received_amount" bson:"stocktransfer_received_amount"`
}

func (warehouse *Warehouse) SetWarehouseStockTransferStats() error {
	collection := db.GetDB("store_" + warehouse.StoreID.Hex()).Collection("stocktransfer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats WarehouseStockTransferStats

	pipeline := []bson.M{
		{
			"$facet": bson.M{
				"sent": []bson.M{
					{"$match": bson.M{
						"store_id":          warehouse.StoreID,
						"from_warehouse_id": warehouse.ID,
					}},
					{"$group": bson.M{
						"_id":                         nil,
						"stocktransfer_sent_count":    bson.M{"$sum": 1},
						"stocktransfer_sent_amount":   bson.M{"$sum": "$net_total"},
						"stocktransfer_sent_quantity": bson.M{"$sum": "$total_quantity"},
					}},
				},
				"received": []bson.M{
					{"$match": bson.M{
						"store_id":        warehouse.StoreID,
						"to_warehouse_id": warehouse.ID,
					}},
					{"$group": bson.M{
						"_id":                             nil,
						"stocktransfer_received_count":    bson.M{"$sum": 1},
						"stocktransfer_received_amount":   bson.M{"$sum": "$net_total"},
						"stocktransfer_received_quantity": bson.M{"$sum": "$total_quantity"},
					}},
				},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		var result struct {
			Sent     []bson.M `bson:"sent"`
			Received []bson.M `bson:"received"`
		}

		err := cur.Decode(&result)
		if err != nil {
			return err
		}

		if len(result.Sent) > 0 {
			// Handle int32 or int64 for count
			switch v := result.Sent[0]["stocktransfer_sent_count"].(type) {
			case int32:
				stats.StockTransferSentCount = int64(v)
			case int64:
				stats.StockTransferSentCount = v
			}
			if val, ok := result.Sent[0]["stocktransfer_sent_amount"].(float64); ok {
				stats.StockTransferSentAmount = val
			}
			if val, ok := result.Sent[0]["stocktransfer_sent_quantity"].(float64); ok {
				stats.StockTransferSentQuantity = val
			}
		}
		if len(result.Received) > 0 {
			switch v := result.Received[0]["stocktransfer_received_count"].(type) {
			case int32:
				stats.StockTransferReceivedCount = int64(v)
			case int64:
				stats.StockTransferReceivedCount = v
			}
			if val, ok := result.Received[0]["stocktransfer_received_amount"].(float64); ok {
				stats.StockTransferReceivedAmount = val
			}
			if val, ok := result.Received[0]["stocktransfer_received_quantity"].(float64); ok {
				stats.StockTransferReceivedQuantity = val
			}
		}
	}

	warehouse.StockTransferSentCount = stats.StockTransferSentCount
	warehouse.StockTransferSentAmount = stats.StockTransferSentAmount
	warehouse.StockTransferSentQuantity = stats.StockTransferSentQuantity

	warehouse.StockTransferReceivedCount = stats.StockTransferReceivedCount
	warehouse.StockTransferReceivedAmount = stats.StockTransferReceivedAmount
	warehouse.StockTransferReceivedQuantity = stats.StockTransferReceivedQuantity

	err = warehouse.Update()
	if err != nil {
		return errors.New("Error updating warehouse: " + err.Error())
	}

	return nil
}

func (stocktransfer *StockTransfer) SetWarehouseStockTransferStats() error {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	fromWarehouse, err := store.FindWarehouseByID(stocktransfer.FromWarehouseID, map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if fromWarehouse != nil {
		err = fromWarehouse.SetWarehouseStockTransferStats()
		if err != nil {
			return err
		}
	}

	toWarehouse, err := store.FindWarehouseByID(stocktransfer.ToWarehouseID, map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		return err
	}

	if toWarehouse != nil {
		err = toWarehouse.SetWarehouseStockTransferStats()
		if err != nil {
			return err
		}
	}

	return nil
}
