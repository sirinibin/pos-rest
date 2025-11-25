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

type ProductStockTransferHistory struct {
	ID                primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date              *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID           *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	ProductID         primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	FromWarehouseID   *primitive.ObjectID `json:"from_warehouse_id" bson:"from_warehouse_id"`
	FromWarehouseCode *string             `json:"from_warehouse_code" bson:"from_warehouse_code"`
	ToWarehouseID     *primitive.ObjectID `json:"to_warehouse_id" bson:"to_warehouse_id"`
	ToWarehouseCode   *string             `json:"to_warehouse_code" bson:"to_warehouse_code"`
	StockTransferID   *primitive.ObjectID `json:"stocktransfer_id,omitempty" bson:"stocktransfer_id,omitempty"`
	StockTransferCode string              `json:"stocktransfer_code,omitempty" bson:"stocktransfer_code,omitempty"`
	Quantity          float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	PurchaseUnitPrice float64             `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	UnitPrice         float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Unit              string              `bson:"unit,omitempty" json:"unit,omitempty"`
	UnitDiscount      float64             `bson:"unit_discount" json:"unit_discount"`
	Discount          float64             `bson:"discount" json:"discount"`
	DiscountPercent   float64             `bson:"discount_percent" json:"discount_percent"`
	Price             float64             `bson:"price" json:"price"`
	NetPrice          float64             `bson:"net_price" json:"net_price"`
	VatPercent        float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice          float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	UnitPriceWithVAT  float64             `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	CreatedAt         *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt         *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type StockTransferHistoryStats struct {
	ID                         *primitive.ObjectID `json:"id" bson:"_id"`
	TotalStockTransferAmount   float64             `json:"total_stocktransfer_amount" bson:"total_stocktransfer_amount"`
	TotalStockTransferQuantity float64             `json:"total_stocktransfer_quantity" bson:"total_stocktransfer_quantity"`
}

func (store *Store) GetStockTransferHistoryStats(filter map[string]interface{}) (stats StockTransferHistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_stocktransfer_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                          nil,
				"total_stocktransfer_amount":   bson.M{"$sum": "$net_price"},
				"total_stocktransfer_quantity": bson.M{"$sum": "$quantity"},
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
		stats.TotalStockTransferAmount = RoundFloat(stats.TotalStockTransferAmount, 2)
		stats.TotalStockTransferQuantity = RoundFloat(stats.TotalStockTransferQuantity, 2)
	}

	return stats, nil
}

func (store *Store) SearchStockTransferHistory(w http.ResponseWriter, r *http.Request) (models []ProductStockTransferHistory, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[from_warehouse_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["from_warehouse_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[to_warehouse_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["to_warehouse_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[from_warehouse_id]"]
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
			criterias.SearchBy["from_warehouse_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[to_warehouse_id]"]
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
			criterias.SearchBy["to_warehouse_id"] = bson.M{"$in": objecIds}
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

	keys, ok = r.URL.Query()["search[stocktransfer_id]"]
	if ok && len(keys[0]) >= 1 {
		stocktransferID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return models, criterias, err
		}
		criterias.SearchBy["stocktransfer_id"] = stocktransferID
	}

	keys, ok = r.URL.Query()["search[stocktransfer_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["stocktransfer_code"] = keys[0]
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_stocktransfer_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	//storeSelectFields := map[string]interface{}{}
	//customerSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return models, criterias, errors.New("Error fetching product stocktransfer history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductStockTransferHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		models = append(models, model)
	} //end for loop

	return models, criterias, nil
}

func (stocktransfer *StockTransfer) ClearProductsStockTransferHistory() error {
	//log.Printf("Clearing StockTransfer history of stocktransfer id:%s", stocktransfer.Code)
	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("product_stocktransfer_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"stocktransfer_id": stocktransfer.ID})
	if err != nil {
		return err
	}
	return nil
}

func (stocktransfer *StockTransfer) CreateProductsStockTransferHistory() error {
	store, err := FindStoreByID(stocktransfer.StoreID, bson.M{})
	if err != nil {
		return err
	}

	//log.Printf("Creating StockTransfer history of stocktransfer id:%s", stocktransfer.Code)
	exists, err := store.IsStockTransferHistoryExistsByStockTransferID(&stocktransfer.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + stocktransfer.StoreID.Hex()).Collection("product_stocktransfer_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, stocktransferProduct := range stocktransfer.Products {
		history := ProductStockTransferHistory{
			Date:              stocktransfer.Date,
			StoreID:           stocktransfer.StoreID,
			ProductID:         stocktransferProduct.ProductID,
			FromWarehouseID:   stocktransfer.FromWarehouseID,
			FromWarehouseCode: stocktransfer.FromWarehouseCode,
			ToWarehouseID:     stocktransfer.ToWarehouseID,
			ToWarehouseCode:   stocktransfer.ToWarehouseCode,
			StockTransferID:   &stocktransfer.ID,
			StockTransferCode: stocktransfer.Code,
			Quantity:          stocktransferProduct.Quantity,
			PurchaseUnitPrice: stocktransferProduct.PurchaseUnitPrice,
			Unit:              stocktransferProduct.Unit,
			UnitDiscount:      stocktransferProduct.UnitDiscount,
			Discount:          (stocktransferProduct.UnitDiscount * stocktransferProduct.Quantity),
			DiscountPercent:   stocktransferProduct.UnitDiscountPercent,
			CreatedAt:         stocktransfer.CreatedAt,
			UpdatedAt:         stocktransfer.UpdatedAt,
		}

		history.UnitPrice = RoundTo8Decimals(stocktransferProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo8Decimals(stocktransferProduct.UnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((stocktransferProduct.UnitPrice - stocktransferProduct.UnitDiscount) * stocktransferProduct.Quantity))

		history.VatPercent = RoundTo2Decimals(*stocktransfer.VatPercent)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))
		history.ID = primitive.NewObjectID()

		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		product, err := store.FindProductByID(&stocktransferProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				history := ProductStockTransferHistory{
					Date:              stocktransfer.Date,
					StoreID:           stocktransfer.StoreID,
					ProductID:         *setProduct.ProductID,
					FromWarehouseID:   stocktransfer.FromWarehouseID,
					FromWarehouseCode: stocktransfer.FromWarehouseCode,
					ToWarehouseID:     stocktransfer.ToWarehouseID,
					ToWarehouseCode:   stocktransfer.ToWarehouseCode,
					StockTransferID:   &stocktransfer.ID,
					StockTransferCode: stocktransfer.Code,
					Quantity:          RoundTo8Decimals(stocktransferProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice: RoundTo4Decimals(stocktransferProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:              setProductObj.Unit,
					UnitDiscount:      RoundTo8Decimals(stocktransferProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:          RoundTo8Decimals((stocktransferProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo8Decimals(stocktransferProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:   stocktransferProduct.UnitDiscountPercent,
					CreatedAt:         stocktransfer.CreatedAt,
					UpdatedAt:         stocktransfer.UpdatedAt,
				}

				history.UnitPrice = RoundTo8Decimals(stocktransferProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo8Decimals(stocktransferProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)

				history.VatPercent = RoundTo2Decimals(*stocktransfer.VatPercent)
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

func (store *Store) IsStockTransferHistoryExistsByStockTransferID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_stocktransfer_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"stocktransfer_id": ID,
	})

	return (count > 0), err
}

func (store *Store) GetStockTransferHistoriesCountByProductID(productID *primitive.ObjectID) (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_stocktransfer_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return collection.CountDocuments(ctx, bson.M{
		"product_id": productID,
	})
}

func (store *Store) GetStockTransferHistoriesByProductID(productID *primitive.ObjectID) (models []ProductStockTransferHistory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_stocktransfer_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{"product_id": productID}, findOptions)
	if err != nil {
		return models, errors.New("Error fetching product stocktransfer history" + err.Error())
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
		stocktransferHistory := ProductStockTransferHistory{}
		err = cur.Decode(&stocktransferHistory)
		if err != nil {
			return models, errors.New("Cursor decode error:" + err.Error())
		}

		//log.Print("Pushing")
		models = append(models, stocktransferHistory)
	} //end for loop

	return models, nil
}

func ProcessStockTransferHistory() error {
	log.Print("Processing stocktransfer history")
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		totalCount, err := store.GetTotalCount(bson.M{}, "product_stocktransfer_history")
		if err != nil {
			return err
		}

		collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_stocktransfer_history")
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
			model := ProductStockTransferHistory{}
			err = cur.Decode(&model)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			bar.Add(1)
		}
	}

	log.Print("StockTransfer DONE!")
	return nil
}

func (model *ProductStockTransferHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_stocktransfer_history")
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
