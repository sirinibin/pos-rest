package models

import (
	"context"
	"encoding/base64"
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

/*
type ProductUnitPrice struct {
	StoreID                 primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName               string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic       string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	PurchaseUnitPrice       float64            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	PurchaseUnitPriceSecret string             `bson:"purchase_unit_price_secret,omitempty" json:"purchase_unit_price_secret,omitempty"`
	WholesaleUnitPrice      float64            `bson:"wholesale_unit_price,omitempty" json:"wholesale_unit_price,omitempty"`
	RetailUnitPrice         float64            `bson:"retail_unit_price,omitempty" json:"retail_unit_price,omitempty"`
}

type ProductStock struct {
	StoreID           primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName         string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	Stock             float64            `bson:"stock" json:"stock"`
}
*/

type ProductStore struct {
	StoreID                 primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName               string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic       string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	PurchaseUnitPrice       float64            `bson:"purchase_unit_price" json:"purchase_unit_price"`
	PurchaseUnitPriceSecret string             `bson:"purchase_unit_price_secret,omitempty" json:"purchase_unit_price_secret,omitempty"`
	WholesaleUnitPrice      float64            `bson:"wholesale_unit_price" json:"wholesale_unit_price"`
	RetailUnitPrice         float64            `bson:"retail_unit_price" json:"retail_unit_price"`
	Stock                   float64            `bson:"stock" json:"stock"`
	RetailUnitProfit        float64            `bson:"retail_unit_profit" json:"retail_unit_profit"`
	RetailUnitProfitPerc    float64            `bson:"retail_unit_profit_perc" json:"retail_unit_profit_perc"`
	WholesaleUnitProfit     float64            `bson:"wholesale_unit_profit" json:"wholesale_unit_profit"`
	WholesaleUnitProfitPerc float64            `bson:"wholesale_unit_profit_perc" json:"wholesale_unit_profit_perc"`
	SalesCount              int64              `bson:"sales_count" json:"sales_count"`
	SalesQuantity           float64            `bson:"sales_quantity" json:"sales_quantity"`
	Sales                   float64            `bson:"sales" json:"sales"`
	SalesReturnCount        int64              `bson:"sales_return_count" json:"sales_return_count"`
	SalesReturnQuantity     float64            `bson:"sales_return_quantity" json:"sales_return_quantity"`
	SalesReturn             float64            `bson:"sales_return" json:"sales_return"`
	SalesProfit             float64            `bson:"sales_profit" json:"sales_profit"`
	SalesLoss               float64            `bson:"sales_loss" json:"sales_loss"`
	PurchaseCount           int64              `bson:"purchase_count" json:"purchase_count"`
	PurchaseQuantity        float64            `bson:"purchase_quantity" json:"purchase_quantity"`
	Purchase                float64            `bson:"purchase" json:"purchase"`
	PurchaseReturnCount     int64              `bson:"purchase_return_count" json:"purchase_return_count"`
	PurchaseReturnQuantity  float64            `bson:"purchase_return_quantity" json:"purchase_return_quantity"`
	PurchaseReturn          float64            `bson:"purchase_return" json:"purchase_return"`
	SalesReturnProfit       float64            `bson:"sales_return_profit" json:"sales_return_profit"`
	SalesReturnLoss         float64            `bson:"sales_return_loss" json:"sales_return_loss"`
	QuotationCount          int64              `bson:"quotation_count" json:"quotation_count"`
	QuotationQuantity       float64            `bson:"quotation_quantity" json:"quotation_quantity"`
	Quotation               float64            `bson:"quotation" json:"quotation"`
	DeliveryNoteCount       int64              `bson:"delivery_note_count" json:"delivery_note_count"`
	DeliveryNoteQuantity    float64            `bson:"delivery_note_quantity" json:"delivery_note_quantity"`
}

// Product : Product structure
type Product struct {
	ID           primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name         string                `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic string                `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode     string                `bson:"item_code,omitempty" json:"item_code,omitempty"`
	StoreID      *primitive.ObjectID   `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName    string                `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode    string                `json:"store_code,omitempty" bson:"store_code,omitempty"`
	BarCode      string                `bson:"bar_code,omitempty" json:"bar_code,omitempty"`
	Ean12        string                `bson:"ean_12,omitempty" json:"ean_12,omitempty"`
	SearchLabel  string                `json:"search_label"`
	Rack         string                `bson:"rack,omitempty" json:"rack"`
	PartNumber   string                `bson:"part_number,omitempty" json:"part_number,omitempty"`
	CategoryID   []*primitive.ObjectID `json:"category_id" bson:"category_id"`
	Category     []*ProductCategory    `json:"category,omitempty"`
	//UnitPrices    []ProductUnitPrice    `bson:"unit_prices,omitempty" json:"unit_prices,omitempty"`
	//Stock         []ProductStock        `bson:"stock,omitempty" json:"stock,omitempty"`
	//Stores        []ProductStore          `bson:"stores,omitempty" json:"stores,omitempty"`
	ProductStores map[string]ProductStore `bson:"product_stores,omitempty" json:"product_stores,omitempty"`
	Unit          string                  `bson:"unit" json:"unit"`
	Images        []string                `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent []string                `json:"images_content,omitempty"`
	Deleted       bool                    `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy     *primitive.ObjectID     `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser *User                   `json:"deleted_by_user,omitempty"`
	DeletedAt     *time.Time              `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt     *time.Time              `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time              `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID     `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID     `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser *User                   `json:"created_by_user,omitempty"`
	UpdatedByUser *User                   `json:"updated_by_user,omitempty"`
	BrandName     string                  `json:"brand_name,omitempty" bson:"brand_name,omitempty"`
	CategoryName  []string                `json:"category_name" bson:"category_name"`
	CreatedByName string                  `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string                  `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName string                  `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog     []ChangeLog             `json:"change_log,omitempty" bson:"change_log,omitempty"`
	BarcodeBase64 string                  `json:"barcode_base64"`
}

type ProductStats struct {
	ID                  *primitive.ObjectID `json:"id" bson:"_id"`
	Stock               float64             `json:"stock" bson:"stock"`
	RetailStockValue    float64             `json:"retail_stock_value" bson:"retail_stock_value"`
	WholesaleStockValue float64             `json:"wholesale_stock_value" bson:"wholesale_stock_value"`
	PurchaseStockValue  float64             `json:"purchase_stock_value" bson:"purchase_stock_value"`
}

func GetProductStats(
	filter map[string]interface{},
	storeID primitive.ObjectID,
) (stats ProductStats, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	stock_StoreCond := []interface{}{"$$store.store_id", "$$store.store_id"}
	retailStockvalue_StoreCond := []interface{}{"$$store.store_id", "$$store.store_id"}
	//retailStockValueStock_StoreCond := []interface{}{"$$store.store_id", "$$store.store_id"}

	if !storeID.IsZero() {
		stock_StoreCond = []interface{}{
			"$$store.store_id", storeID,
		}

		retailStockvalue_StoreCond = []interface{}{
			"$$store.store_id", storeID,
		}

		/*
			retailStockValueStock_StoreCond = []interface{}{
				"$$store.store_id", storeID,
			}
		*/

	}
	/*
		bson.M{"$sum": bson.M{"$sum": bson.M{
											"$map": bson.M{
												"input": "$stock",
												"as":    "stockItem",
												"in": bson.M{
													"$cond": []interface{}{
														bson.M{"$and": []interface{}{
															bson.M{"$eq": retailStockValueStock_StoreCond},
															bson.M{"$gt": []interface{}{"$$stockItem.stock", 0}},
														}},
														"$$stockItem.stock",
														0,
													},
												},
											},
										}}},
	*/

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				"stock": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$stores",
						"as":    "store",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": stock_StoreCond},
									bson.M{"$gt": []interface{}{"$$store.stock", 0}},
								}},
								"$$store.stock",
								0,
							},
						},
					},
				}}},
				"retail_stock_value": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$stores",
						"as":    "store",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": retailStockvalue_StoreCond},
									bson.M{"$gt": []interface{}{"$$store.stock", 0}},
								}},
								bson.M{"$multiply": []interface{}{
									"$$store.retail_unit_price",
									"$$store.stock",
								}},
								0,
							},
						},
					},
				}}},
				"wholesale_stock_value": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$stores",
						"as":    "store",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": retailStockvalue_StoreCond},
									bson.M{"$gt": []interface{}{"$$store.stock", 0}},
								}},
								bson.M{"$multiply": []interface{}{
									"$$store.wholesale_unit_price",
									"$$store.stock",
								}},
								0,
							},
						},
					},
				}}},
				"purchase_stock_value": bson.M{"$sum": bson.M{"$sum": bson.M{
					"$map": bson.M{
						"input": "$stores",
						"as":    "store",
						"in": bson.M{
							"$cond": []interface{}{
								bson.M{"$and": []interface{}{
									bson.M{"$eq": retailStockvalue_StoreCond},
									bson.M{"$gt": []interface{}{"$$store.stock", 0}},
								}},
								bson.M{"$multiply": []interface{}{
									"$$store.purchase_unit_price",
									"$$store.stock",
								}},
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
		stats.Stock = math.Round(stats.Stock*100) / 100
		stats.RetailStockValue = math.Round(stats.RetailStockValue*100) / 100
		stats.WholesaleStockValue = math.Round(stats.WholesaleStockValue*100) / 100
		stats.PurchaseStockValue = math.Round(stats.PurchaseStockValue*100) / 100
	}
	return stats, nil
}

func (product *Product) getRetailUnitPriceByStoreID(storeID primitive.ObjectID) (retailUnitPrice float64, err error) {
	if productStore, ok := product.ProductStores[storeID.Hex()]; ok {
		return productStore.RetailUnitPrice, nil
	}

	return retailUnitPrice, nil
	/*for _, store := range product.Stores {
		if store.StoreID == storeID {
			return store.RetailUnitPrice, nil
		}
	}
	return retailUnitPrice, err
	*/
}

func (product *Product) getPurchaseUnitPriceSecretByStoreID(storeID primitive.ObjectID) (secret string, err error) {

	if productStore, ok := product.ProductStores[storeID.Hex()]; ok {
		return productStore.PurchaseUnitPriceSecret, nil
	}

	return "", nil

	/*
		for _, productStore := range product.ProductStores {
			if productStore.StoreID == storeID {
				return productStore.PurchaseUnitPriceSecret, nil
			}
		}
		return secret, err
	*/
}

func (product *Product) AttributesValueChangeEvent(productOld *Product) error {

	if product.Name != productOld.Name {
		product.SetChangeLog(
			"attribute_value_change",
			"name",
			productOld.Name,
			product.Name,
		)
	}
	if product.NameInArabic != productOld.NameInArabic {
		product.SetChangeLog(
			"attribute_value_change",
			"name_in_arabic",
			productOld.NameInArabic,
			product.NameInArabic,
		)
	}

	return nil
}

func (product *Product) SetChangeLog(
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
		description = "Stock reduced from " + fmt.Sprintf("%.02f", oldValue.(float64)) + " to " + fmt.Sprintf("%.02f", newValue.(float64)) + " by " + UserObject.Name
	} else if event == "add_stock" && name != nil {
		description = "Stock raised from " + fmt.Sprintf("%.02f", oldValue.(float64)) + " to " + fmt.Sprintf("%.02f", newValue.(float64)) + " by " + UserObject.Name
	} else if event == "add_image" {
		description = "Added " + strconv.Itoa(newValue.(int)) + " new images by " + UserObject.Name
	} else if event == "remove_image" {
		description = "Removed " + strconv.Itoa(newValue.(int)) + " images by " + UserObject.Name
	}

	product.ChangeLog = append(
		product.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (product *Product) UpdateForeignLabelFields() error {

	product.CategoryName = []string{}

	for i, productStore := range product.ProductStores {
		store, err := FindStoreByID(&productStore.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding store:" + productStore.StoreID.Hex() + ",error:" + err.Error())
		}

		if productStore, ok := product.ProductStores[i]; ok {
			productStore.StoreName = store.Name
			productStore.StoreNameInArabic = store.NameInArabic
			product.ProductStores[i] = productStore
		}
	}

	for _, categoryID := range product.CategoryID {
		productCategory, err := FindProductCategoryByID(categoryID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding product category id:" + categoryID.Hex() + ",error:" + err.Error())
		}
		product.CategoryName = append(product.CategoryName, productCategory.Name)
	}

	for _, category := range product.Category {
		productCategory, err := FindProductCategoryByID(&category.ID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding product category id:" + category.ID.Hex() + ",error:" + err.Error())
		}
		product.CategoryName = append(product.CategoryName, productCategory.Name)
	}

	if product.CreatedBy != nil {
		createdByUser, err := FindUserByID(product.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind created_by user:" + err.Error())
		}
		product.CreatedByName = createdByUser.Name
	}

	if product.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(product.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind updated_by user:" + err.Error())
		}
		product.UpdatedByName = updatedByUser.Name
	}

	if product.DeletedBy != nil && !product.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(product.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error findind deleted_by user:" + err.Error())
		}
		product.DeletedByName = deletedByUser.Name
	}

	if product.StoreID != nil {
		store, err := FindStoreByID(product.StoreID, bson.M{"id": 1, "name": 1, "code": 1})
		if err != nil {
			return err
		}
		product.StoreName = store.Name
		product.StoreCode = store.Code
	}

	return nil
}

type BarTenderProductData struct {
	StoreName               string `json:"storename"`
	ProductName             string `json:"productname"`
	Price                   string `json:"price"`
	BarCode                 string `json:"barcode"`
	Rack                    string `json:"rack"`
	PurchaseUnitPriceSecret string `json:"purchase_unit_price_secret"`
}

func GetBarTenderProducts(r *http.Request) (products []BarTenderProductData, err error) {
	products = []BarTenderProductData{}

	criterias := SearchCriterias{
		SortBy: map[string]interface{}{"updated_at": -1},
	}

	storeCode := ""
	keys, ok := r.URL.Query()["store_code"]
	if ok && len(keys[0]) >= 1 {
		storeCode = keys[0]
	}

	if storeCode == "" {
		return products, errors.New("store_code is required")
	}

	store, err := FindStoreByCode(storeCode, bson.M{})
	if err != nil {
		return products, err
	}

	criterias.Select = ParseSelectString("id,name,barcode,unit_prices,rack")

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetProjection(criterias.Select)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return products, errors.New("Error fetching products:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return products, errors.New("Cursor error:" + err.Error())
		}
		product := Product{}
		err = cur.Decode(&product)
		if err != nil {
			return products, errors.New("Cursor decode error:" + err.Error())
		}

		if len(product.BarCode) == 0 {
			err = product.Update()
			if err != nil {
				return products, err
			}
		}

		productPrice := "0.00"
		purchaseUnitPriceSecret := ""
		if len(product.ProductStores) > 0 {
			for i, productStore := range product.ProductStores {
				if productStore.StoreID == store.ID {
					if len(productStore.PurchaseUnitPriceSecret) == 0 {
						if productStoreTemp, ok := product.ProductStores[i]; ok {
							productStoreTemp.PurchaseUnitPriceSecret = GenerateSecretCode(int(product.ProductStores[i].PurchaseUnitPrice))
							product.ProductStores[i] = productStoreTemp
						}
						//product.ProductStores[i].PurchaseUnitPriceSecret = GenerateSecretCode(int(product.Stores[i].PurchaseUnitPrice))
						err = product.Update()
						if err != nil {
							return products, err
						}
					}
					purchaseUnitPriceSecret = product.ProductStores[i].PurchaseUnitPriceSecret

					price := float64(productStore.RetailUnitPrice)
					vatPrice := (float64(float64(productStore.RetailUnitPrice) * float64(store.VatPercent/float64(100))))
					price += vatPrice
					price = math.Round(price*100) / 100
					productPrice = fmt.Sprintf("%.2f", price)
					break
				}
			}
		}

		barTenderProduct := BarTenderProductData{
			StoreName:               store.Name,
			ProductName:             product.Name,
			BarCode:                 product.BarCode,
			Price:                   productPrice,
			Rack:                    product.Rack,
			PurchaseUnitPriceSecret: purchaseUnitPriceSecret,
		}
		products = append(products, barTenderProduct)
	}

	return products, nil
}

func SearchProduct(w http.ResponseWriter, r *http.Request) (products []Product, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		searchWord := strings.Replace(keys[0], "\\", `\\`, -1)
		searchWord = strings.Replace(searchWord, "(", `\(`, -1)
		searchWord = strings.Replace(searchWord, ")", `\)`, -1)
		searchWord = strings.Replace(searchWord, "{", `\{`, -1)
		searchWord = strings.Replace(searchWord, "}", `\}`, -1)
		searchWord = strings.Replace(searchWord, "[", `\[`, -1)
		searchWord = strings.Replace(searchWord, "]", `\]`, -1)
		searchWord = strings.Replace(searchWord, `*`, `\*`, -1)

		searchWord = strings.Replace(searchWord, "_", `\_`, -1)
		searchWord = strings.Replace(searchWord, "+", `\\+`, -1)
		searchWord = strings.Replace(searchWord, "'", `\'`, -1)
		searchWord = strings.Replace(searchWord, `"`, `\"`, -1)

		criterias.SearchBy["$or"] = []bson.M{
			{"part_number": bson.M{"$regex": searchWord, "$options": "i"}},
			{"name": bson.M{"$regex": searchWord, "$options": "i"}},
			{"name_in_arabic": bson.M{"$regex": searchWord, "$options": "i"}},
		}
		//criterias.SearchBy["$text"] = bson.M{"$search": searchWord, "$language": "en"}
	}
	var storeID primitive.ObjectID
	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return products, criterias, err
		}
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.Replace(keys[0], "stores.", "product_stores."+storeID.Hex()+".", -1)
		criterias.SortBy = GetSortByFields(keys[0])
	}

	keys, ok = r.URL.Query()["search[retail_unit_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit": value,
						"store_id":           storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit": value,
					}
				}
			}
		*/

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".retail_unit_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".retail_unit_profit"] = value
		}
		//criterias.SearchBy["stores"] = element
	}

	keys, ok = r.URL.Query()["search[retail_unit_profit_perc]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit_perc": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit_perc": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit_perc": value,
						"store_id":                storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"retail_unit_profit_perc": value,
					}
				}
			}
		*/

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".retail_unit_profit_perc"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".retail_unit_profit_perc"] = value
		}
		//criterias.SearchBy["stores"] = element
	}

	keys, ok = r.URL.Query()["search[wholesale_unit_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit": value,
						"store_id":              storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit": value,
					}
				}
			}
		*/

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".wholesale_unit_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".wholesale_unit_profit"] = value
		}
		//criterias.SearchBy["stores"] = element
	}

	keys, ok = r.URL.Query()["search[wholesale_unit_profit_perc]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit_perc": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit_perc": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit_perc": value,
						"store_id":                   storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_profit_perc": value,
					}
				}
			}
		*/

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".wholesale_unit_profit_perc"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".wholesale_unit_profit_perc"] = value
		}
		//criterias.SearchBy["stores"] = element
	}

	keys, ok = r.URL.Query()["search[stock]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		stockValue, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		/*
			stockElement := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					stockElement["$elemMatch"] = bson.M{
						"stock": bson.M{
							operator: stockValue,
						},
						"store_id": storeID,
					}
				} else {
					stockElement["$elemMatch"] = bson.M{
						"stock": bson.M{
							operator: stockValue,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					stockElement["$elemMatch"] = bson.M{
						"stock":    stockValue,
						"store_id": storeID,
					}
				} else {
					stockElement["$elemMatch"] = bson.M{
						"stock": stockValue,
					}
				}
			}
		*/

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".stock"] = bson.M{operator: stockValue}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".stock"] = stockValue
		}

		//criterias.SearchBy["stores"] = stockElement
	}

	//sales
	keys, ok = r.URL.Query()["search[sales_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_quantity"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_quantity", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_loss", operator, &storeID, value)
	}

	// Sales return
	keys, ok = r.URL.Query()["search[sales_reutrn_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("sales_return_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_quantity"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_quantity", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_profit]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_profit"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_profit"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_profit", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[sales_return_loss]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_loss"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".sales_return_loss"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("sales_return_loss", operator, &storeID, value)
	}

	//purchase

	keys, ok = r.URL.Query()["search[purchase_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_quantity"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_quantity", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase", operator, &storeID, value)
	}

	// purchase return

	keys, ok = r.URL.Query()["search[purchase_return_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_return_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_return_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("purchase_return_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_return_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_return_quantity"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_return_quantity", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[purchase_return]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_return"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_return"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("purchase_return", operator, &storeID, value)
	}

	//Quotation
	keys, ok = r.URL.Query()["search[quotation_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".quotation_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".quotation_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("quotation_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[quotation_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".quotation_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".quotation_quantity"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("quotation_quantity", operator, &storeID, value)
	}

	//Delivery note
	keys, ok = r.URL.Query()["search[delivery_note_count]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".delivery_note_count"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".delivery_note_count"] = value
		}
		//criterias.SearchBy["stores"] = GetIntSearchElement("delivery_note_count", operator, &storeID, value)
	}

	keys, ok = r.URL.Query()["search[delivery_note_quantity]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".delivery_note_quantity"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".delivery_note_quantity"] = value
		}
		//criterias.SearchBy["stores"] = GetFloatSearchElement("delivery_note_quantity", operator, &storeID, value)
	}

	//-end
	keys, ok = r.URL.Query()["search[retail_unit_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".retail_unit_price"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".retail_unit_price"] = value
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"retail_unit_price": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"retail_unit_price": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"retail_unit_price": value,
						"store_id":          storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"retail_unit_price": value,
					}
				}
			}

			criterias.SearchBy["stores"] = element
		*/
	}

	keys, ok = r.URL.Query()["search[wholesale_unit_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".wholesale_unit_price"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".wholesale_unit_price"] = value
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_price": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_price": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_price": value,
						"store_id":             storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"wholesale_unit_price": value,
					}
				}
			}

			criterias.SearchBy["stores"] = element
		*/
	}

	keys, ok = r.URL.Query()["search[purchase_unit_price]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 64)
		if err != nil {
			return products, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_unit_price"] = bson.M{operator: value}
		} else {
			criterias.SearchBy["product_stores."+storeID.Hex()+".purchase_unit_price"] = value
		}

		/*
			element := bson.M{"$elemMatch": bson.M{}}

			if operator != "" {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"purchase_unit_price": bson.M{
							operator: value,
						},
						"store_id": storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"purchase_unit_price": bson.M{
							operator: value,
						},
					}
				}

			} else {
				if !storeID.IsZero() {
					element["$elemMatch"] = bson.M{
						"purchase_unit_price": value,
						"store_id":            storeID,
					}
				} else {
					element["$elemMatch"] = bson.M{
						"purchase_unit_price": value,
					}
				}
			}
			criterias.SearchBy["stores"] = element
		*/
	}

	keys, ok = r.URL.Query()["search[rack]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["rack"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[item_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["item_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[bar_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["bar_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[ean_12]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"ean_12": keys[0]},
			{"bar_code": keys[0]},
		}
	}

	keys, ok = r.URL.Query()["search[part_number]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["part_number"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[category_id]"]
	if ok && len(keys[0]) >= 1 {

		categoryIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range categoryIds {
			categoryID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return products, criterias, err
			}
			objecIds = append(objecIds, categoryID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["category_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return products, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return products, criterias, err
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
			return products, criterias, err
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
			return products, criterias, err
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

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return products, criterias, err
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

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	categorySelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields

		if _, ok := criterias.Select["category.id"]; ok {
			categorySelectFields = ParseRelationalSelectString(keys[0], "category")
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
		return products, criterias, errors.New("Error fetching products:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return products, criterias, errors.New("Cursor error:" + err.Error())
		}
		product := Product{}
		err = cur.Decode(&product)
		if err != nil {
			return products, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		product.SearchLabel = product.Name + " ( Part #: " + product.PartNumber + " )"

		if product.NameInArabic != "" {
			product.SearchLabel += " / " + product.NameInArabic
		}

		if _, ok := criterias.Select["category.id"]; ok {
			for _, categoryID := range product.CategoryID {
				category, _ := FindProductCategoryByID(categoryID, categorySelectFields)
				product.Category = append(product.Category, category)
			}
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			product.CreatedByUser, _ = FindUserByID(product.CreatedBy, createdByUserSelectFields)
		}

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			product.UpdatedByUser, _ = FindUserByID(product.UpdatedBy, updatedByUserSelectFields)
		}

		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			product.DeletedByUser, _ = FindUserByID(product.DeletedBy, deletedByUserSelectFields)
		}

		if product.BarCode != "" {
			product.Ean12 = product.Ean12 + "(Old:" + product.BarCode + ")"
		}

		products = append(products, product)
	} //end for loop

	return products, criterias, nil

}

func GenerateSecretCode(n int) string {
	if n == 0 {
		return ""
	}

	str := ""

	strN := strconv.Itoa(n)
	i := 0
	for i < len(strN) {
		intN, _ := strconv.Atoi(string(strN[i]))
		switch intN {
		case 1:
			str = str + "K"
		case 2:
			str = str + "L"
		case 3:
			str = str + "M"
		case 4:
			str = str + "N"
		case 5:
			str = str + "O"
		case 6:
			str = str + "P"
		case 7:
			str = str + "Q"
		case 8:
			str = str + "R"
		case 9:
			str = str + "S"
		case 0:
			str = str + "T"
		}
		i++
	}
	return str
}

func (product *Product) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if product.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsProductExists(&product.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Product:" + product.ID.Hex()
		}

	}

	if govalidator.IsNull(product.Name) {
		errs["name"] = "Name is required"
	}

	if !govalidator.IsNull(product.ItemCode) {
		exists, err := product.IsItemCodeExists()
		if err != nil {
			errs["item_code"] = err.Error()
		}

		if exists {
			errs["item_code"] = "Item Code Already Exists"
		}
	}

	if !govalidator.IsNull(product.PartNumber) {
		exists, err := product.IsPartNumberExists()
		if err != nil {
			errs["part_number"] = err.Error()
		}

		if exists {
			errs["part_number"] = "Part Number Already Exists"
		}
	}

	storeNo := 0
	for i, productStore := range product.ProductStores {
		if productStore.StoreID.IsZero() {
			errs["store_id_"+strconv.Itoa(storeNo)] = "store_id is required for unit price"
			return errs
		}
		exists, err := IsStoreExists(&productStore.StoreID)
		if err != nil {
			errs["store_id_"+strconv.Itoa(storeNo)] = err.Error()
		}

		if !exists {
			errs["store_id"+strconv.Itoa(storeNo)] = "Invalid store_id:" + productStore.StoreID.Hex() + " in stores"
		}

		if productStoreTemp, ok := product.ProductStores[i]; ok {
			productStoreTemp.PurchaseUnitPriceSecret = GenerateSecretCode(int(product.ProductStores[i].PurchaseUnitPrice))
			product.ProductStores[i] = productStoreTemp
		}
		//product.Stores[i].PurchaseUnitPriceSecret = GenerateSecretCode(int(product.Stores[i].PurchaseUnitPrice))
		storeNo++
	}

	if len(product.CategoryID) == 0 {
		errs["category_id"] = "Atleast 1 category is required"
	} else {
		for i, categoryID := range product.CategoryID {
			exists, err := IsProductCategoryExists(categoryID)
			if err != nil {
				errs["category_id_"+strconv.Itoa(i)] = err.Error()
			}

			if !exists {
				errs["category_id_"+strconv.Itoa(i)] = "Invalid category:" + categoryID.Hex()
			}
		}

	}

	for k, imageContent := range product.ImagesContent {
		splits := strings.Split(imageContent, ",")

		if len(splits) == 2 {
			product.ImagesContent[k] = splits[1]
		} else if len(splits) == 1 {
			product.ImagesContent[k] = splits[0]
		}

		valid, err := IsStringBase64(product.ImagesContent[k])
		if err != nil {
			errs["images_content"] = err.Error()
		}

		if !valid {
			errs["images_"+strconv.Itoa(k)] = "Invalid base64 string"
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func GeneratePartNumber(n int) string {
	letterRunes := []rune("1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (product *Product) GenerateBarCode(startFrom int, count int64) (string, error) {
	code := startFrom + int(count)
	return strconv.Itoa(code + 1), nil
}

func FindLastProduct(
	selectFields map[string]interface{},
) (product *Product, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{}, findOneOptions).
		Decode(&product)
	if err != nil {
		return nil, err
	}

	return product, err
}

func (product *Product) SetPartNumber() (err error) {
	if len(product.PartNumber) == 0 {
		for {
			product.PartNumber = strings.ToUpper(GeneratePartNumber(10))
			log.Print("product.PartNumber:")
			log.Print(product.PartNumber)
			exists, err := product.IsPartNumberExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}
	return nil
}

func (product *Product) SetBarcode() (err error) {
	if len(product.Ean12) == 0 {
		lastProduct, err := FindLastProduct(bson.M{})
		if err != nil {
			return err
		}
		barcode := ""
		if lastProduct != nil {
			lastEan12, err := strconv.Atoi(lastProduct.Ean12)
			if err != nil {
				return err
			}
			lastEan12++
			barcode = strconv.Itoa(lastEan12)
		} else {
			barcode = "100000000000"
		}

		for {
			product.Ean12 = barcode
			exists, err := product.IsEan12Exists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			lastEan12, err := strconv.Atoi(product.Ean12)
			if err != nil {
				return err
			}
			lastEan12++
			barcode = strconv.Itoa(lastEan12)
		}
	}
	return nil
}

func (product *Product) InitStoreUnitPrice() (err error) {
	if len(product.ProductStores) > 0 {
		return nil
	}

	product.ProductStores = map[string]ProductStore{}

	if !product.StoreID.IsZero() && product.StoreID.Hex() != "" {
		product.ProductStores[product.StoreID.Hex()] = ProductStore{
			StoreID:            *product.StoreID,
			StoreName:          product.StoreName,
			RetailUnitPrice:    0,
			WholesaleUnitPrice: 0,
			PurchaseUnitPrice:  0,
		}
	}

	/*
		product.ProductStores = make([]ProductStore, 1)
		product.ProductStores[0] = ProductStore{
			StoreID:            *product.StoreID,
			StoreName:          product.StoreName,
			RetailUnitPrice:    0,
			WholesaleUnitPrice: 0,
			PurchaseUnitPrice:  0,
		}
	*/
	return nil
}

func (product *Product) CalculateUnitProfit() (err error) {
	for i, _ := range product.ProductStores {
		if productStoreTemp, ok := product.ProductStores[i]; ok {
			productStoreTemp.RetailUnitProfit = product.ProductStores[i].RetailUnitPrice - product.ProductStores[i].PurchaseUnitPrice
			productStoreTemp.WholesaleUnitProfit = product.ProductStores[i].WholesaleUnitPrice - product.ProductStores[i].PurchaseUnitPrice
			productStoreTemp.RetailUnitProfitPerc = 0
			productStoreTemp.WholesaleUnitProfitPerc = 0
			product.ProductStores[i] = productStoreTemp
		}
		/*
			product.ProductStores[i].RetailUnitProfit = product.ProductStores[i].RetailUnitPrice - product.ProductStores[i].PurchaseUnitPrice
			product.ProductStores[i].WholesaleUnitProfit = product.ProductStores[i].WholesaleUnitPrice - product.ProductStores[i].PurchaseUnitPrice
			product.ProductStores[i].RetailUnitProfitPerc = 0
			product.ProductStores[i].WholesaleUnitProfitPerc = 0
		*/

		if product.ProductStores[i].PurchaseUnitPrice == 0 && product.ProductStores[i].RetailUnitProfit > 0 {
			if productStoreTemp, ok := product.ProductStores[i]; ok {
				productStoreTemp.RetailUnitProfitPerc = 100
				product.ProductStores[i] = productStoreTemp
			}
			//product.ProductStores[i].RetailUnitProfitPerc = 100
		} else if product.ProductStores[i].PurchaseUnitPrice != 0 {
			if productStoreTemp, ok := product.ProductStores[i]; ok {
				productStoreTemp.RetailUnitProfitPerc = (product.ProductStores[i].RetailUnitProfit / product.ProductStores[i].PurchaseUnitPrice) * 100
				product.ProductStores[i] = productStoreTemp
			}
			//product.ProductStores[i].RetailUnitProfitPerc = (product.ProductStores[i].RetailUnitProfit / product.ProductStores[i].PurchaseUnitPrice) * 100
		}

		if product.ProductStores[i].PurchaseUnitPrice == 0 && product.ProductStores[i].WholesaleUnitProfit > 0 {
			//product.ProductStores[i].WholesaleUnitProfitPerc = 100
			if productStoreTemp, ok := product.ProductStores[i]; ok {
				productStoreTemp.WholesaleUnitProfitPerc = 100
				product.ProductStores[i] = productStoreTemp
			}
		} else if product.ProductStores[i].PurchaseUnitPrice != 0 {
			if productStoreTemp, ok := product.ProductStores[i]; ok {
				productStoreTemp.WholesaleUnitProfitPerc = (product.ProductStores[i].WholesaleUnitProfit / product.ProductStores[i].PurchaseUnitPrice) * 100
				product.ProductStores[i] = productStoreTemp
			}
			//product.ProductStores[i].WholesaleUnitProfitPerc = (product.Stores[i].WholesaleUnitProfit / product.Stores[i].PurchaseUnitPrice) * 100
		}
	}
	return nil
}

func (product *Product) Insert() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	_, err = collection.InsertOne(ctx, &product)
	if err != nil {
		return err
	}
	return nil
}

func (product *Product) SaveImages() error {
	if len(product.ImagesContent) == 0 {
		return nil
	}

	for _, imageContent := range product.ImagesContent {
		content, err := base64.StdEncoding.DecodeString(imageContent)
		if err != nil {
			return err
		}

		extension, err := GetFileExtensionFromBase64(content)
		if err != nil {
			return err
		}

		filename := "images/products/" + GenerateFileName("product_", extension)
		err = SaveBase64File(filename, content)
		if err != nil {
			return err
		}
		product.Images = append(product.Images, "/"+filename)
	}

	product.ImagesContent = []string{}

	return nil
}

func (product *Product) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": product.ID},
		bson.M{"$set": product},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (product *Product) DeleteProduct(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = product.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	product.Deleted = true
	product.DeletedBy = &userID
	now := time.Now()
	product.DeletedAt = &now

	product.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": product.ID},
		bson.M{"$set": product},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindProductByItemCode(
	itemCode string,
	selectFields map[string]interface{},
) (product *Product, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"item_code": itemCode}, findOneOptions).
		Decode(&product)
	if err != nil {
		return nil, err
	}

	product.SearchLabel = product.Name + " (Part #" + product.PartNumber + ", Arabic: " + product.NameInArabic + ")"

	return product, err
}

func FindProductByBarCode(
	barCode string,
	selectFields map[string]interface{},
) (product *Product, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	criteria := make(map[string]interface{})
	criteria["$or"] = []bson.M{
		{"bar_code": barCode},
		{"ean_12": barCode},
	}

	err = collection.FindOne(ctx, criteria, findOneOptions).
		Decode(&product)
	if err != nil {
		return nil, err
	}

	product.SearchLabel = product.Name + " (Part #" + product.PartNumber + ", Arabic: " + product.NameInArabic + ")"

	return product, err
}

func FindProductByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (product *Product, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&product)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["category.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "category")
		for _, categoryID := range product.CategoryID {
			category, _ := FindProductCategoryByID(categoryID, fields)
			product.Category = append(product.Category, category)
		}

	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		product.CreatedByUser, _ = FindUserByID(product.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		product.UpdatedByUser, _ = FindUserByID(product.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		product.DeletedByUser, _ = FindUserByID(product.DeletedBy, fields)
	}

	return product, err
}

func (product *Product) IsItemCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"item_code": product.ItemCode,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"item_code": product.ItemCode,
			"_id":       bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func (product *Product) IsPartNumberExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"part_number": product.PartNumber,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"part_number": product.PartNumber,
			"_id":         bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func (product *Product) IsBarCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"bar_code": product.BarCode,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"bar_code": product.BarCode,
			"_id":      bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func (product *Product) IsEan12Exists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"ean_12": product.Ean12,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"ean_12": product.Ean12,
			"_id":    bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func IsProductExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func (product *Product) ReflectValidPurchaseUnitPrice() error {
	salesHistories, err := GetSalesHistoriesByProductID(&product.ID)
	if err != nil {
		return errors.New("Error fetching sales history of product:" + err.Error())
	}

	if len(salesHistories) == 0 {
		return nil
	}

	for _, salesHistory := range salesHistories {
		order, err := FindOrderByID(salesHistory.OrderID, map[string]interface{}{})
		if err != nil {
			return errors.New("Error fetching order:" + err.Error())
		}

		for k, _ := range order.Products {
			for _, store := range product.ProductStores {

				if store.StoreID == *order.StoreID &&
					order.Products[k].ProductID.Hex() == product.ID.Hex() &&
					(order.Products[k].Loss > 0 || order.Products[k].Profit <= 0 || order.Products[k].PurchaseUnitPrice == 0) &&
					store.PurchaseUnitPrice > 0 {
					//log.Printf("Updating purchase unit price of order: %s", order.Code)
					order.Products[k].PurchaseUnitPrice = store.PurchaseUnitPrice
				}

			}
		}

		err = order.Update()
		if err != nil {
			return errors.New("Error updating order:" + err.Error())
		}
	}
	return nil
}

func ProcessProducts() error {
	log.Printf("Processing products")
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching products" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		product := Product{}
		err = cur.Decode(&product)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		/*
			err = product.ReflectValidPurchaseUnitPrice()
			if err != nil {
				return err
			}
		*/
		if len(product.ProductStores) == 0 {
			product.ProductStores = map[string]ProductStore{}
		}

		for _, store := range product.ProductStores {
			if !store.StoreID.IsZero() && store.StoreID.Hex() != "" {
				product.ProductStores[store.StoreID.Hex()] = store
			}
		}

		err = product.Update()
		if err != nil {
			return err
		}
	}

	log.Print("DONE!")
	return nil
}

func (product *Product) HardDelete() error {
	log.Print("Deleting product")
	ctx := context.Background()
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	_, err := collection.DeleteOne(ctx, bson.M{
		"_id": product.ID,
	})
	if err != nil {
		return err
	}

	return nil
}
