package models

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ProductNonVATSalesReturnHistory struct {
	ID                   primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Date                 *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	StoreID              *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName            string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	ProductID            primitive.ObjectID  `json:"product_id,omitempty" bson:"product_id,omitempty"`
	CustomerID           *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	CustomerName         string              `json:"customer_name" bson:"customer_name"`
	CustomerNameArabic   string              `json:"customer_name_arabic" bson:"customer_name_arabic"`
	NonVATSalesReturnID  *primitive.ObjectID `json:"non_vat_sales_return_id,omitempty" bson:"non_vat_sales_return_id,omitempty"`
	NonVATSalesReturnCode string             `json:"non_vat_sales_return_code,omitempty" bson:"non_vat_sales_return_code,omitempty"`
	NonVATSalesID        *primitive.ObjectID `json:"non_vat_sales_id,omitempty" bson:"non_vat_sales_id,omitempty"`
	NonVATSalesCode      string              `json:"non_vat_sales_code,omitempty" bson:"non_vat_sales_code,omitempty"`
	Quantity             float64             `json:"quantity,omitempty" bson:"quantity,omitempty"`
	PurchaseUnitPrice    float64             `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	UnitPrice            float64             `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	UnitPriceWithVAT     float64             `bson:"unit_price_with_vat,omitempty" json:"unit_price_with_vat,omitempty"`
	UnitDiscount         float64             `bson:"unit_discount" json:"unit_discount"`
	Discount             float64             `bson:"discount" json:"discount"`
	DiscountPercent      float64             `bson:"discount_percent" json:"discount_percent"`
	Price                float64             `bson:"price,omitempty" json:"price,omitempty"`
	NetPrice             float64             `bson:"net_price" json:"net_price"`
	Profit               float64             `bson:"profit" json:"profit"`
	Loss                 float64             `bson:"loss" json:"loss"`
	VatPercent           float64             `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	VatPrice             float64             `bson:"vat_price,omitempty" json:"vat_price,omitempty"`
	Unit                 string              `bson:"unit,omitempty" json:"unit,omitempty"`
	Store                *Store              `json:"store,omitempty"`
	Customer             *Customer           `json:"customer,omitempty"`
	CreatedAt            *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt            *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type NonVATSalesReturnHistoryStats struct {
	TotalNonVATSalesReturn float64 `json:"total_non_vat_sales_return" bson:"total_non_vat_sales_return"`
	TotalProfit            float64 `json:"total_profit" bson:"total_profit"`
	TotalLoss              float64 `json:"total_loss" bson:"total_loss"`
	TotalVatReturn         float64 `json:"total_vat_return" bson:"total_vat_return"`
	TotalQuantity          float64 `json:"total_quantity" bson:"total_quantity"`
}

func (store *Store) GetNonVATSalesReturnHistoryStats(filter map[string]interface{}) (stats NonVATSalesReturnHistoryStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_non_vat_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{
			"_id":                      nil,
			"total_non_vat_sales_return": bson.M{"$sum": "$net_price"},
			"total_profit":               bson.M{"$sum": "$profit"},
			"total_loss":                 bson.M{"$sum": "$loss"},
			"total_vat_return":           bson.M{"$sum": "$vat_price"},
			"total_quantity":             bson.M{"$sum": "$quantity"},
		}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return stats, err
	}
	defer cur.Close(ctx)

	if cur.Next(ctx) {
		if err := cur.Decode(&stats); err != nil {
			return stats, err
		}
		stats.TotalNonVATSalesReturn = RoundFloat(stats.TotalNonVATSalesReturn, 2)
		stats.TotalProfit = RoundFloat(stats.TotalProfit, 2)
		stats.TotalLoss = RoundFloat(stats.TotalLoss, 2)
		stats.TotalVatReturn = RoundFloat(stats.TotalVatReturn, 2)
	}

	return stats, nil
}

func (store *Store) SearchNonVATSalesReturnHistory(w http.ResponseWriter, r *http.Request) (models []ProductNonVATSalesReturnHistory, criterias SearchCriterias, err error) {
	criterias = InitSearchCriterias()
	criterias.SortBy = map[string]interface{}{"created_at": -1}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	var keys []string
	var ok bool

	if err = ParseExactDateFilter(r, &criterias, "search[date_str]", "date", timeZoneOffset); err != nil {
		return models, criterias, err
	}
	if err = ParseDateRangeFilter(r, &criterias, "search[from_date]", "search[to_date]", "date", timeZoneOffset); err != nil {
		return models, criterias, err
	}
	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return models, criterias, err
	}
	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return models, criterias, err
	}

	ParseTextSearch(r, &criterias, "search[store_name]", "store_name")
	ParseTextSearch(r, &criterias, "search[customer_name]", "customer_name")

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

	for _, field := range []string{"vat_price", "net_price", "profit", "loss", "price", "unit_price", "unit_price_with_vat", "discount", "discount_percent", "quantity"} {
		if err = ParseFloatWithOperatorFilter(r, &criterias, "search["+field+"]", field); err != nil {
			return models, criterias, err
		}
	}

	ParseTextSearch(r, &criterias, "search[warehouse_code]", "warehouse_code")

	if err = ParseObjectIDFilter(r, &criterias, "search[store_id]", "store_id"); err != nil {
		return models, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[product_id]", "product_id"); err != nil {
		return models, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[non_vat_sales_return_id]", "non_vat_sales_return_id"); err != nil {
		return models, criterias, err
	}
	if err = ParseObjectIDFilter(r, &criterias, "search[non_vat_sales_id]", "non_vat_sales_id"); err != nil {
		return models, criterias, err
	}

	keys, ok = r.URL.Query()["search[non_vat_sales_return_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["non_vat_sales_return_code"] = keys[0]
	}

	keys, ok = r.URL.Query()["search[non_vat_sales_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["non_vat_sales_code"] = keys[0]
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_non_vat_sales_return_history")
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
		return models, criterias, errors.New("Error fetching non vat sales return history: " + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		if err := cur.Err(); err != nil {
			return models, criterias, errors.New("Cursor error:" + err.Error())
		}
		model := ProductNonVATSalesReturnHistory{}
		if err = cur.Decode(&model); err != nil {
			return models, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		if _, ok := criterias.Select["store.id"]; ok {
			model.Store, _ = FindStoreByID(model.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			model.Customer, _ = store.FindCustomerByID(model.CustomerID, customerSelectFields)
		}
		models = append(models, model)
	}

	return models, criterias, nil
}

func (nonVATSalesReturn *NonVATSalesReturn) CreateProductsNonVATSalesReturnHistory() error {
	store, err := FindStoreByID(nonVATSalesReturn.StoreID, bson.M{})
	if err != nil {
		return err
	}

	exists, err := store.IsNonVATSalesReturnHistoryExistsByNonVATSalesReturnID(&nonVATSalesReturn.ID)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	collection := db.GetDB("store_" + nonVATSalesReturn.StoreID.Hex()).Collection("product_non_vat_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vatPercent := float64(0)
	if nonVATSalesReturn.VatPercent != nil {
		vatPercent = *nonVATSalesReturn.VatPercent
	}

	for _, p := range nonVATSalesReturn.Products {
		if !p.Selected {
			continue
		}

		history := ProductNonVATSalesReturnHistory{
			ID:                   primitive.NewObjectID(),
			Date:                 nonVATSalesReturn.Date,
			StoreID:              nonVATSalesReturn.StoreID,
			StoreName:            nonVATSalesReturn.StoreName,
			ProductID:            p.ProductID,
			CustomerID:           nonVATSalesReturn.CustomerID,
			CustomerName:         nonVATSalesReturn.CustomerName,
			CustomerNameArabic:   nonVATSalesReturn.CustomerNameArabic,
			NonVATSalesReturnID:  &nonVATSalesReturn.ID,
			NonVATSalesReturnCode: nonVATSalesReturn.Code,
			NonVATSalesID:        nonVATSalesReturn.NonVATSalesID,
			NonVATSalesCode:      nonVATSalesReturn.NonVATSalesCode,
			Quantity:             p.Quantity,
			Unit:                 p.Unit,
			Discount:             p.UnitDiscount,
			DiscountPercent:      p.UnitDiscountPercent,
			CreatedAt:            nonVATSalesReturn.CreatedAt,
			UpdatedAt:            nonVATSalesReturn.UpdatedAt,
		}

		history.UnitPrice = RoundTo8Decimals(p.UnitPrice)
		history.UnitPriceWithVAT = RoundTo8Decimals(p.UnitPriceWithVAT)
		history.Price = RoundFloat(((p.UnitPrice - p.UnitDiscount) * p.Quantity), 2)
		history.VatPercent = RoundFloat(vatPercent, 2)
		history.VatPrice = RoundFloat((history.Price * (history.VatPercent / 100)), 2)
		history.NetPrice = RoundFloat((history.Price + history.VatPrice), 2)
		history.Profit = RoundFloat(p.Profit, 2)
		history.Loss = RoundFloat(p.Loss, 2)

		if _, err := collection.InsertOne(ctx, &history); err != nil {
			return err
		}

		product, err := store.FindProductByID(&p.ProductID, bson.M{})
		if err != nil {
			return err
		}

		if len(product.Set.Products) > 0 {
			for _, setProduct := range product.Set.Products {
				setProductObj, err := store.FindProductByID(setProduct.ProductID, bson.M{})
				if err != nil {
					return err
				}
				setHistory := ProductNonVATSalesReturnHistory{
					ID:                   primitive.NewObjectID(),
					Date:                 nonVATSalesReturn.Date,
					StoreID:              nonVATSalesReturn.StoreID,
					StoreName:            nonVATSalesReturn.StoreName,
					ProductID:            *setProduct.ProductID,
					CustomerID:           nonVATSalesReturn.CustomerID,
					CustomerName:         nonVATSalesReturn.CustomerName,
					CustomerNameArabic:   nonVATSalesReturn.CustomerNameArabic,
					NonVATSalesReturnID:  &nonVATSalesReturn.ID,
					NonVATSalesReturnCode: nonVATSalesReturn.Code,
					NonVATSalesID:        nonVATSalesReturn.NonVATSalesID,
					NonVATSalesCode:      nonVATSalesReturn.NonVATSalesCode,
					Quantity:             RoundTo8Decimals(p.Quantity * setProduct.Quantity),
					PurchaseUnitPrice:    RoundTo4Decimals(p.PurchaseUnitPrice * (setProduct.PurchasePricePercent / 100)),
					Unit:                 setProductObj.Unit,
					UnitDiscount:         RoundTo8Decimals(p.UnitDiscount * (setProduct.RetailPricePercent / 100)),
					Discount:             RoundTo8Decimals((p.UnitDiscount * (setProduct.RetailPricePercent / 100)) * RoundTo8Decimals(p.Quantity*setProduct.Quantity)),
					DiscountPercent:      p.UnitDiscountPercent,
					CreatedAt:            nonVATSalesReturn.CreatedAt,
					UpdatedAt:            nonVATSalesReturn.UpdatedAt,
				}
				setHistory.UnitPrice = RoundTo8Decimals(p.UnitPrice * (setProduct.RetailPricePercent / 100))
				setHistory.UnitPriceWithVAT = RoundTo8Decimals(p.UnitPriceWithVAT * (setProduct.RetailPricePercent / 100))
				setHistory.Price = RoundTo2Decimals((setHistory.UnitPrice - setHistory.UnitDiscount) * setHistory.Quantity)
				setHistory.Profit = RoundTo4Decimals(p.Profit * (setProduct.RetailPricePercent / 100))
				setHistory.Loss = RoundTo4Decimals(p.Loss * (setProduct.RetailPricePercent / 100))
				setHistory.VatPercent = RoundTo2Decimals(vatPercent)
				setHistory.VatPrice = RoundTo2Decimals(setHistory.Price * (setHistory.VatPercent / 100))
				setHistory.NetPrice = RoundTo2Decimals(setHistory.Price + setHistory.VatPrice)

				if _, err = collection.InsertOne(ctx, &setHistory); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (store *Store) IsNonVATSalesReturnHistoryExistsByNonVATSalesReturnID(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_non_vat_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, bson.M{"non_vat_sales_return_id": ID})
	return (count > 0), err
}

func (nonVATSalesReturn *NonVATSalesReturn) ClearProductsNonVATSalesReturnHistory() error {
	collection := db.GetDB("store_" + nonVATSalesReturn.StoreID.Hex()).Collection("product_non_vat_sales_return_history")
	ctx := context.Background()
	_, err := collection.DeleteMany(ctx, bson.M{"non_vat_sales_return_id": nonVATSalesReturn.ID})
	return err
}

func (model *ProductNonVATSalesReturnHistory) Update() error {
	collection := db.GetDB("store_" + model.StoreID.Hex()).Collection("product_non_vat_sales_return_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	_, err := collection.UpdateOne(ctx, bson.M{"_id": model.ID}, bson.M{"$set": model}, updateOptions)
	return err
}
