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
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductQuotationHistory struct {
	ID                primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date              *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID           *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName         string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID         primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID        *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName      string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	QuotationID       *primitive.ObjectID `json:"quotation_id,omitempty" bson:"quotation_id,omitempty"`
	QuotationCode     string              `json:"quotation_code,omitempty" bson:"quotation_code,omitempty"`
	Quantity          float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	PurchaseUnitPrice float64             `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	UnitPrice         float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	UnitDiscount      float64             `bson:"unit_discount" json:"unit_discount"`
	Discount          float64             `bson:"discount" json:"discount"`
	DiscountPercent   float64             `bson:"discount_percent" json:"discount_percent"`
	Price             float64             `bson:"price,omitempty" json:"price,omitempty"`
	NetPrice          float64             `bson:"net_price,omitempty" json:"net_price,omitempty"`
	Profit            float64             `bson:"profit" json:"profit"`
	Loss              float64             `bson:"loss" json:"loss"`
	VatPercent        float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice          float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	Unit              string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Store             *Store              `json:"store,omitempty"`
	UnitPriceWithVAT  float64             `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	Customer          *Customer           `json:"customer,omitempty"`
	CreatedAt         *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt         *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	Type              string              `bson:"type" json:"type"`
	PaymentStatus     string              `bson:"payment_status" json:"payment_status"`
}

type QuotationHistoryStats struct {
	ID             *primitive.ObjectID `json:"id" bson:"_id"`
	TotalQuotation float64             `json:"total_quotation" bson:"total_quotation"`
	TotalProfit    float64             `json:"total_profit" bson:"total_profit"`
	TotalLoss      float64             `json:"total_loss" bson:"total_loss"`
	TotalVat       float64             `json:"total_vat" bson:"total_vat"`
}

func (store *Store) GetQuotationHistoryStats(filter map[string]interface{}) (stats QuotationHistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":             nil,
				"total_quotation": bson.M{"$sum": "$net_price"},
				"total_profit":    bson.M{"$sum": "$profit"},
				"total_loss":      bson.M{"$sum": "$loss"},
				"total_vat":       bson.M{"$sum": "$vat_price"},
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
		stats.TotalQuotation = RoundFloat(stats.TotalQuotation, 2)
		stats.TotalProfit = RoundFloat(stats.TotalProfit, 2)
		stats.TotalLoss = RoundFloat(stats.TotalLoss, 2)
		stats.TotalVat = RoundFloat(stats.TotalVat, 2)
	}

	return stats, nil
}

func (store *Store) SearchQuotationHistory(w http.ResponseWriter, r *http.Request) (models []ProductQuotationHistory, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[type]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] != "" {
			criterias.SearchBy["type"] = keys[0]
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

	keys, ok = r.URL.Query()["search[payment_status]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] != "" {
			criterias.SearchBy["payment_status"] = keys[0]
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_history")
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
		return models, criterias, errors.New("Error fetching product quotation history:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductQuotationHistory{}
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

func (quotation *Quotation) CreateProductsQuotationHistory() error {
	store, err := FindStoreByID(quotation.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsQuotationHistoryExistsByQuotationID(&quotation.ID)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	collection := db.GetDB("store_" + quotation.StoreID.Hex()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, quotationProduct := range quotation.Products {
		history := ProductQuotationHistory{
			Date:            quotation.Date,
			StoreID:         quotation.StoreID,
			StoreName:       quotation.StoreName,
			ProductID:       quotationProduct.ProductID,
			CustomerID:      quotation.CustomerID,
			CustomerName:    quotation.CustomerName,
			QuotationID:     &quotation.ID,
			QuotationCode:   quotation.Code,
			Quantity:        quotationProduct.Quantity,
			UnitPrice:       quotationProduct.UnitPrice,
			Unit:            quotationProduct.Unit,
			Discount:        quotationProduct.UnitDiscount,
			DiscountPercent: quotationProduct.UnitDiscountPercent,
			CreatedAt:       quotation.CreatedAt,
			UpdatedAt:       quotation.UpdatedAt,
			Type:            quotation.Type,
			PaymentStatus:   quotation.PaymentStatus,
		}

		history.UnitPrice = RoundTo8Decimals(quotationProduct.UnitPrice)
		history.UnitPriceWithVAT = RoundTo8Decimals(quotationProduct.UnitPriceWithVAT)
		history.Price = RoundTo2Decimals(((quotationProduct.UnitPrice - quotationProduct.UnitDiscount) * quotationProduct.Quantity))
		history.Profit = RoundFloat(quotationProduct.Profit, 2)
		history.Loss = RoundFloat(quotationProduct.Loss, 2)

		history.VatPercent = RoundFloat(*quotation.VatPercent, 2)
		history.VatPrice = RoundTo2Decimals((history.Price * (history.VatPercent / 100)))
		history.NetPrice = RoundTo2Decimals((history.Price + history.VatPrice))

		history.ID = primitive.NewObjectID()
		_, err := collection.InsertOne(ctx, &history)
		if err != nil {
			return err
		}

		product, err := store.FindProductByID(&quotationProduct.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}

				history := ProductQuotationHistory{
					Date:              quotation.Date,
					Type:              quotation.Type,
					StoreID:           quotation.StoreID,
					StoreName:         quotation.StoreName,
					ProductID:         *setProduct.ProductID,
					CustomerID:        quotation.CustomerID,
					CustomerName:      quotation.CustomerName,
					QuotationID:       &quotation.ID,
					QuotationCode:     quotation.Code,
					Quantity:          RoundTo8Decimals(quotationProduct.Quantity * setProduct.Quantity),
					PurchaseUnitPrice: RoundTo4Decimals(quotationProduct.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:              setProductObj.Unit,
					UnitDiscount:      RoundTo8Decimals(quotationProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:          RoundTo8Decimals((quotationProduct.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo8Decimals(quotationProduct.Quantity*setProduct.Quantity)),
					DiscountPercent:   quotationProduct.UnitDiscountPercent,
					CreatedAt:         quotation.CreatedAt,
					UpdatedAt:         quotation.UpdatedAt,
				}

				history.UnitPrice = RoundTo8Decimals(quotationProduct.UnitPrice * (setProduct.RetailPricePercent / 100))
				history.UnitPriceWithVAT = RoundTo8Decimals(quotationProduct.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				history.Price = RoundTo2Decimals((history.UnitPrice - history.UnitDiscount) * history.Quantity)
				history.Profit = RoundTo4Decimals(quotationProduct.Profit * (setProduct.RetailPricePercent / 100))
				history.Loss = RoundTo4Decimals(quotationProduct.Loss * (setProduct.RetailPricePercent / 100))

				history.VatPercent = RoundTo2Decimals(*quotation.VatPercent)
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

func (store *Store) IsQuotationHistoryExistsByQuotationID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"quotation_id": ID,
	})

	return (count > 0), err
}

func (model *Quotation) ClearProductsQuotationHistory() error {
	//log.Printf("Clearing Sales history of order id:%s", order.Code)
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_quotation_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"quotation_id": model.ID})
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) ProcessQuotationHistory() error {
	log.Print("Processing quotation history")
	totalCount, err := store.GetTotalCount(bson.M{}, "product_quotation_history")
	if err != nil {
		return err
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_history")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching product quotation history:" + err.Error())
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
		model := ProductQuotationHistory{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		quotation, err := store.FindQuotationByID(model.QuotationID, map[string]interface{}{})
		if err != nil {
			return errors.New("Error finding purchase:" + err.Error())
		}
		model.Date = quotation.Date
		err = model.Update()
		if err != nil {
			return errors.New("Error updating quotation history:" + err.Error())
		}
		bar.Add(1)
	}

	log.Print("Quotation history DONE!")
	return nil
}

func (model *ProductQuotationHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_quotation_history")
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
