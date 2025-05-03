package models

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/asaskevich/govalidator"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	IsUnitPriceWithVAT      bool               `bson:"with_vat" json:"with_vat"`
	DamagedStock            float64            `bson:"damaged_stock" json:"damaged_stock"`
	Stock                   float64            `bson:"stock" json:"stock"`
	RetailUnitProfit        float64            `bson:"retail_unit_profit,omitempty" json:"retail_unit_profit,omitempty"`
	RetailUnitProfitPerc    float64            `bson:"retail_unit_profit_perc,omitempty" json:"retail_unit_profit_perc,omitempty"`
	WholesaleUnitProfit     float64            `bson:"wholesale_unit_profit,omitempty" json:"wholesale_unit_profit,omitempty"`
	WholesaleUnitProfitPerc float64            `bson:"wholesale_unit_profit_perc,omitempty" json:"wholesale_unit_profit_perc,omitempty"`
	SalesCount              int64              `bson:"sales_count" json:"sales_count"`
	SalesQuantity           float64            `bson:"sales_quantity,omitempty" json:"sales_quantity,omitempty"`
	Sales                   float64            `bson:"sales,omitempty" json:"sales,omitempty"`
	SalesReturnCount        int64              `bson:"sales_return_count" json:"sales_return_count"`
	SalesReturnQuantity     float64            `bson:"sales_return_quantity,omitempty" json:"sales_return_quantity,omitempty"`
	SalesReturn             float64            `bson:"sales_return,omitempty" json:"sales_return,omitempty"`
	SalesProfit             float64            `bson:"sales_profit,omitempty" json:"sales_profit,omitempty"`
	SalesLoss               float64            `bson:"sales_loss,omitempty" json:"sales_loss,omitempty"`
	PurchaseCount           int64              `bson:"purchase_count" json:"purchase_count"`
	PurchaseQuantity        float64            `bson:"purchase_quantity,omitempty" json:"purchase_quantity,omitempty"`
	Purchase                float64            `bson:"purchase,omitempty" json:"purchase,omitempty"`
	PurchaseReturnCount     int64              `bson:"purchase_return_count" json:"purchase_return_count"`
	PurchaseReturnQuantity  float64            `bson:"purchase_return_quantity,omitempty" json:"purchase_return_quantity,omitempty"`
	PurchaseReturn          float64            `bson:"purchase_return,omitempty" json:"purchase_return,omitempty"`
	SalesReturnProfit       float64            `bson:"sales_return_profit,omitempty" json:"sales_return_profit,omitempty"`
	SalesReturnLoss         float64            `bson:"sales_return_loss,omitempty" json:"sales_return_loss,omitempty"`
	QuotationCount          int64              `bson:"quotation_count" json:"quotation_count"`
	QuotationQuantity       float64            `bson:"quotation_quantity,omitempty" json:"quotation_quantity,omitempty"`
	Quotation               float64            `bson:"quotation,omitempty" json:"quotation,omitempty"`
	DeliveryNoteCount       int64              `bson:"delivery_note_count" json:"delivery_note_count"`
	DeliveryNoteQuantity    float64            `bson:"delivery_note_quantity,omitempty" json:"delivery_note_quantity,omitempty"`
}

// Product : Product structure
type Product struct {
	ID                   primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name                 string                `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic         string                `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	NamePrefixes         []string              `bson:"name_prefixes,omitempty" json:"name_prefixes,omitempty"`
	NameInArabicPrefixes []string              `bson:"name_in_arabic_prefixes,omitempty" json:"name_in_arabic_prefixes,omitempty"`
	ItemCode             string                `bson:"item_code,omitempty" json:"item_code,omitempty"`
	StoreID              *primitive.ObjectID   `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName            string                `json:"store_name,omitempty" bson:"store_name,omitempty"`
	StoreCode            string                `json:"store_code,omitempty" bson:"store_code,omitempty"`
	BarCode              string                `bson:"bar_code,omitempty" json:"bar_code,omitempty"`
	Ean12                string                `bson:"ean_12,omitempty" json:"ean_12,omitempty"`
	SearchLabel          string                `json:"search_label"`
	Rack                 string                `bson:"rack,omitempty" json:"rack,omitempty"`
	PrefixPartNumber     string                `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber           string                `bson:"part_number,omitempty" json:"part_number,omitempty"`
	CategoryID           []*primitive.ObjectID `json:"category_id" bson:"category_id"`
	Category             []*ProductCategory    `json:"category" bson:"-"`
	BrandID              *primitive.ObjectID   `json:"brand_id,omitempty" bson:"brand_id,omitempty"`
	BrandName            string                `json:"brand_name,omitempty" bson:"brand_name,omitempty"`
	BrandCode            string                `json:"brand_code,omitempty" bson:"brand_code,omitempty"`
	CountryName          string                `bson:"country_name" json:"country_name"`
	CountryCode          string                `bson:"country_code" json:"country_code"`
	//UnitPrices    []ProductUnitPrice    `bson:"unit_prices,omitempty" json:"unit_prices,omitempty"`
	//Stock         []ProductStock        `bson:"stock,omitempty" json:"stock,omitempty"`
	//Stores        []ProductStore          `bson:"stores,omitempty" json:"stores,omitempty"`
	ProductStores    map[string]ProductStore `bson:"product_stores,omitempty" json:"product_stores,omitempty"`
	Unit             string                  `bson:"unit,omitempty" json:"unit,omitempty"`
	Images           []string                `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent    []string                `json:"images_content,omitempty" bson:"-"`
	Deleted          bool                    `bson:"deleted" json:"deleted"`
	DeletedBy        *primitive.ObjectID     `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser    *User                   `json:"deleted_by_user" bson:"-"`
	DeletedAt        *time.Time              `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt        *time.Time              `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt        *time.Time              `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy        *primitive.ObjectID     `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy        *primitive.ObjectID     `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser    *User                   `json:"created_by_user,omitempty"`
	UpdatedByUser    *User                   `json:"updated_by_user,omitempty"`
	CategoryName     []string                `json:"category_name" bson:"category_name"`
	CreatedByName    string                  `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName    string                  `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName    string                  `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	BarcodeBase64    string                  `json:"barcode_base64" bson:"-"`
	LinkedProductIDs []*primitive.ObjectID   `json:"linked_product_ids" bson:"linked_product_ids"`
	LinkedProducts   []*Product              `json:"linked_products,omitempty" bson:"-"`
	LinkToProductID  *primitive.ObjectID     `json:"link_to_product_id,omitempty" bson:"-"`
}

type ProductStats struct {
	ID                  *primitive.ObjectID `json:"id" bson:"_id"`
	Stock               float64             `json:"stock" bson:"stock"`
	RetailStockValue    float64             `json:"retail_stock_value" bson:"retail_stock_value"`
	WholesaleStockValue float64             `json:"wholesale_stock_value" bson:"wholesale_stock_value"`
	PurchaseStockValue  float64             `json:"purchase_stock_value" bson:"purchase_stock_value"`
}

func (store *Store) SaveProductImage(productID *primitive.ObjectID, filename string) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")

	filter := bson.M{
		"_id":      productID,
		"store_id": store.ID,
	}

	update := bson.M{
		"$push": bson.M{
			"images": filename,
		},
	}

	_, err := collection.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) GetProductStats(
	filter map[string]interface{},
	storeID primitive.ObjectID,
) (stats ProductStats, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	//stock_StoreCond := []interface{}{"$$store.store_id", "$$store.store_id"}
	//retailStockvalue_StoreCond := []interface{}{"$$store.store_id", "$$store.store_id"}
	//retailStockValueStock_StoreCond := []interface{}{"$$store.store_id", "$$store.store_id"}

	if !storeID.IsZero() {
		/*stock_StoreCond = []interface{}{
			"$$store.store_id", storeID,
		}

		retailStockvalue_StoreCond = []interface{}{
			"$$store.store_id", storeID,
		}
		*/

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

										 bson.M{"$cond":bson.M{
					[]interface{}{
						bson.M{"$gt": []interface{}{"$product_stores." + storeID.Hex() + ".stock", 0}},
						"$product_stores." + storeID.Hex() + ".stock",
						0,
					}},
	*/

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id": nil,
				"stock": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$gt": []interface{}{"$product_stores." + storeID.Hex() + ".stock", 0}},
					"$product_stores." + storeID.Hex() + ".stock",
					0,
				}}},
				"retail_stock_value": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$gt": []interface{}{"$product_stores." + storeID.Hex() + ".stock", 0}},
					bson.M{"$multiply": []interface{}{
						"$product_stores." + storeID.Hex() + ".stock",
						"$product_stores." + storeID.Hex() + ".retail_unit_price",
					}},
					0,
				}}},
				"wholesale_stock_value": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$gt": []interface{}{"$product_stores." + storeID.Hex() + ".stock", 0}},
					bson.M{"$multiply": []interface{}{
						"$product_stores." + storeID.Hex() + ".stock",
						"$product_stores." + storeID.Hex() + ".wholesale_unit_price",
					}},
					0,
				}}},
				"purchase_stock_value": bson.M{"$sum": bson.M{"$cond": []interface{}{
					bson.M{"$gt": []interface{}{"$product_stores." + storeID.Hex() + ".stock", 0}},
					bson.M{"$multiply": []interface{}{
						"$product_stores." + storeID.Hex() + ".stock",
						"$product_stores." + storeID.Hex() + ".purchase_unit_price",
					}},
					0,
				}}},
				/*"stock": bson.M{"$sum": bson.M{
				"input": "$product_stores." + storeID.Hex(),
				"as":    "store",
				"in": bson.M{
					"$cond": []interface{}{
						bson.M{"$gt": []interface{}{"$$store.stock", 0}},
						"$$store.stock",
						0,
					},
				}}},
				*/
				//"stock": bson.M{"$sum": "$product_stores." + storeID.Hex() + ".stock"},
				/*"retail_stock_value": bson.M{"$sum": bson.M{"$multiply": []interface{}{
					"$product_stores." + storeID.Hex() + ".stock",
					"$product_stores." + storeID.Hex() + ".retail_unit_price",
				}}},*/

				//"retail_stock_value": bson.M{"$sum": "$product_stores." + storeID.Hex() + ".stock*$product_stores." + storeID.Hex() + ".retail_unit_price"},
				/*
					"stock": bson.M{"$sum": bson.M{"$sum": bson.M{
						"$map": bson.M{
							"input": "$product_stores",
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
				*/
				/*
					"retail_stock_value": bson.M{"$sum": bson.M{"$sum": bson.M{
						"$map": bson.M{
							"input": "$product_stores",
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
							"input": "$product_stores",
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
							"input": "$product_stores",
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
				*/
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
		stats.Stock = RoundFloat(stats.Stock, 2)
		stats.RetailStockValue = RoundFloat(stats.RetailStockValue, 2)
		stats.WholesaleStockValue = RoundFloat(stats.WholesaleStockValue, 2)
		stats.PurchaseStockValue = RoundFloat(stats.PurchaseStockValue, 2)
	}
	return stats, nil
}

func (product *Product) getRetailUnitPriceByStoreID(storeID primitive.ObjectID) (retailUnitPrice float64, withTax bool, err error) {
	if productStore, ok := product.ProductStores[storeID.Hex()]; ok {
		return productStore.RetailUnitPrice, productStore.IsUnitPriceWithVAT, nil
	}

	return retailUnitPrice, false, nil
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

	}
	if product.NameInArabic != productOld.NameInArabic {

	}

	return nil
}

func (product *Product) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return err
	}

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
		productCategory, err := store.FindProductCategoryByID(categoryID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding product category id:" + categoryID.Hex() + ",error:" + err.Error())
		}
		product.CategoryName = append(product.CategoryName, productCategory.Name)
	}

	if product.BrandID != nil {
		productBrand, err := store.FindProductBrandByID(product.BrandID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding product brand id:" + product.BrandID.Hex() + ",error:" + err.Error())
		}
		product.BrandName = productBrand.Name
	}

	for _, category := range product.Category {
		productCategory, err := store.FindProductCategoryByID(&category.ID, bson.M{"id": 1, "name": 1})
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

func (store *Store) GetBarTenderProducts(r *http.Request) (products []BarTenderProductData, err error) {
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

	/*
		store, err = FindStoreByCode(storeCode, bson.M{})
		if err != nil {
			return products, err
		}
	*/

	criterias.Select = ParseSelectString("id,name,barcode,unit_prices,rack")

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
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
			err = product.Update(nil)
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
						err = product.Update(nil)
						if err != nil {
							return products, err
						}
					}
					purchaseUnitPriceSecret = product.ProductStores[i].PurchaseUnitPriceSecret

					price := float64(productStore.RetailUnitPrice)
					vatPrice := (float64(float64(productStore.RetailUnitPrice) * float64(store.VatPercent/float64(100))))
					price += vatPrice
					price = RoundFloat(price, 2)
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

func (store *Store) SearchProduct(w http.ResponseWriter, r *http.Request, loadData bool) (products []Product, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return products, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		} /*else if value == 2 {
			//criterias.SearchBy["deleted"] = bson.M{"$eq": nil}

			criterias.SearchBy["$or"] = []bson.M{
				{"deleted": bson.M{"$eq": true}},
				{"deleted": bson.M{"$eq": false}},
				{"deleted": bson.M{"$eq": nil}},
			}
		}*/
	}

	textSearching := false

	keys, ok = r.URL.Query()["search[search_text]"]
	if ok && len(keys[0]) >= 1 {
		textSearching = true
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

		criterias.SearchBy["$text"] = bson.M{"$search": searchWord}
		criterias.SortBy["score"] = bson.M{"$meta": "textScore"}

		/*
			criterias.SearchBy["$or"] = []bson.M{
				{"part_number": bson.M{"$regex": searchWord, "$options": "i"}},
				{"name": bson.M{"$regex": searchWord, "$options": "i"}},
				{"name_in_arabic": bson.M{"$regex": searchWord, "$options": "i"}},
			}
			criterias.SortBy = bson.M{"name": 1}*/
	}

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		textSearching = true
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
			{"name": bson.M{"$regex": searchWord, "$options": "i"}},
			{"name_in_arabic": bson.M{"$regex": searchWord, "$options": "i"}},
		}
		criterias.SortBy = bson.M{"name": 1}

		/*
			criterias.SearchBy["$text"] = bson.M{"$search": searchWord}
			criterias.SortBy["score"] = bson.M{"$meta": "textScore"}
		*/

		//criterias.SearchBy["$text"] = bson.M{"$search": searchWord}

		//criterias.Select["score"] = bson.M{"$meta": "textScore"}

		//criterias.SearchBy["$or"] = []bson.M{
		//{"part_number": bson.M{"$regex": searchWord, "$options": "i"}},
		//{"name": bson.M{"$regex": searchWord, "$options": "i"}},
		//{"name_in_arabic": bson.M{"$regex": searchWord, "$options": "i"}},
		//{"$text": bson.M{"$search": searchWord}},
		//}

		//criterias.SearchBy["$text"] = bson.M{"$search": searchWord, "$language": "en"}
	}

	keys, ok = r.URL.Query()["search[part_number]"]
	if ok && len(keys[0]) >= 1 {
		/*
			criterias.SearchBy["$text"] = bson.M{"$search": keys[0]}
			criterias.SortBy["score"] = bson.M{"$meta": "textScore"}
		*/
		criterias.SearchBy["$or"] = []bson.M{
			{"prefix_part_number": bson.M{"$regex": keys[0], "$options": "i"}},
			{"part_number": bson.M{"$regex": keys[0], "$options": "i"}},
		}
		//criterias.SearchBy["part_number"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	var storeID primitive.ObjectID
	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return products, criterias, err
		}
		/*
			store, err := FindStoreByID(&storeID, bson.M{})
			if err != nil {
				return products, criterias, err
			}

			if len(store.UseProductsFromStoreID) > 0 {
				criterias.SearchBy["$or"] = []bson.M{
					{"store_id": storeID},
					{"store_id": bson.M{"$in": store.UseProductsFromStoreID}},
				}
			} else {
				criterias.SearchBy["store_id"] = storeID
			}*/
	}

	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		keys[0] = strings.Replace(keys[0], "stores.", "product_stores."+storeID.Hex()+".", -1)
		if !textSearching {
			criterias.SortBy = GetSortByFields(keys[0])
		}

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
	keys, ok = r.URL.Query()["search[sales_return_count]"]
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

	//
	keys, ok = r.URL.Query()["search[linked_products_of_product_id]"]
	if ok && len(keys[0]) >= 1 {
		productID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return products, criterias, err
		}
		product, err := store.FindProductByID(&productID, bson.M{})
		if err != nil {
			return products, criterias, err
		}

		criterias.SearchBy["_id"] = bson.M{"$in": product.LinkedProductIDs}
	}

	keys, ok = r.URL.Query()["search[quotation_products_of_quotation_id]"]
	if ok && len(keys[0]) >= 1 {
		quotationID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return products, criterias, err
		}
		quotation, err := store.FindQuotationByID(&quotationID, bson.M{})
		if err != nil {
			return products, criterias, err
		}

		ids := []*primitive.ObjectID{}
		for i, _ := range quotation.Products {
			ids = append(ids, &quotation.Products[i].ProductID)
		}
		criterias.SearchBy["_id"] = bson.M{"$in": ids}
	}

	keys, ok = r.URL.Query()["search[delivery_note_products_of_delivery_note_id]"]
	if ok && len(keys[0]) >= 1 {
		deliveryNoteID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return products, criterias, err
		}
		deliveryNote, err := store.FindDeliveryNoteByID(&deliveryNoteID, bson.M{})
		if err != nil {
			return products, criterias, err
		}

		ids := []*primitive.ObjectID{}
		for i, _ := range deliveryNote.Products {
			ids = append(ids, &deliveryNote.Products[i].ProductID)
		}
		criterias.SearchBy["_id"] = bson.M{"$in": ids}
	}

	keys, ok = r.URL.Query()["search[ids]"]
	if ok && len(keys[0]) >= 1 {
		Ids := strings.Split(keys[0], ",")
		objecIds := []primitive.ObjectID{}

		for _, id := range Ids {
			ID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return products, criterias, err
			}
			objecIds = append(objecIds, ID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["_id"] = bson.M{"$in": objecIds}
		}
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

	keys, ok = r.URL.Query()["search[country_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["country_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[country_code]"]
	if ok && len(keys[0]) >= 1 {
		countryCodes := strings.Split(keys[0], ",")
		if len(countryCodes) > 0 {
			criterias.SearchBy["country_code"] = bson.M{"$in": countryCodes}
		}
	}

	keys, ok = r.URL.Query()["search[brand_id]"]
	if ok && len(keys[0]) >= 1 {

		brandIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range brandIds {
			brandID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return products, criterias, err
			}
			objecIds = append(objecIds, brandID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["brand_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[product_id]"]
	if ok && len(keys[0]) >= 1 {

		productIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range productIds {
			productID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return products, criterias, err
			}
			objecIds = append(objecIds, productID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["_id"] = bson.M{"$in": objecIds}
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

	keys, ok = r.URL.Query()["limit"]
	if ok && len(keys[0]) >= 1 {
		criterias.Size, _ = strconv.Atoi(keys[0])
	}
	keys, ok = r.URL.Query()["page"]
	if ok && len(keys[0]) >= 1 {
		criterias.Page, _ = strconv.Atoi(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
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

	if !loadData {
		return products, criterias, nil
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

		product.SetSearchLabel(&store.ID)

		if _, ok := criterias.Select["category.id"]; ok {
			for _, categoryID := range product.CategoryID {
				category, _ := store.FindProductCategoryByID(categoryID, categorySelectFields)
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

func (product *Product) SetSearchLabel(storeID *primitive.ObjectID) {
	product.SearchLabel = ""
	if product.PrefixPartNumber != "" && product.PartNumber != "" {
		product.SearchLabel = "#" + product.PrefixPartNumber + " - " + product.PartNumber + " - "
	} else if product.PartNumber != "" {
		product.SearchLabel = "#" + product.PartNumber + " - "
	} else if product.PrefixPartNumber != "" {
		product.SearchLabel = "#" + product.PrefixPartNumber + " - "
	}

	product.SearchLabel = product.SearchLabel + product.Name

	if product.NameInArabic != "" {
		product.SearchLabel += " - " + product.NameInArabic
	}

	_, ok := product.ProductStores[storeID.Hex()]
	if ok {
		product.SearchLabel += " - Stock: " + fmt.Sprintf("%.2f", product.ProductStores[storeID.Hex()].Stock) + " " + product.Unit
		if product.ProductStores[storeID.Hex()].RetailUnitPrice != 0 {
			product.SearchLabel += " - Unit price: " + fmt.Sprintf("%.2f", product.ProductStores[storeID.Hex()].RetailUnitPrice)
			if product.ProductStores[storeID.Hex()].IsUnitPriceWithVAT {
				product.SearchLabel += " [with VAT]"
			} else {
				product.SearchLabel += " [without VAT]"
			}
		}
	}

	if product.BrandName != "" {
		product.SearchLabel += " - Brand: " + product.BrandName
	}

	if product.CountryName != "" {
		product.SearchLabel += " - Country: " + product.CountryName
	}
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

func (product *Product) TrimSpaceFromFields() {
	product.Name = strings.TrimSpace(product.Name)
	product.NameInArabic = strings.TrimSpace(product.NameInArabic)
	product.PartNumber = strings.TrimSpace(product.PartNumber)
	product.ItemCode = strings.TrimSpace(product.ItemCode)
}

func (product *Product) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)
	product.TrimSpaceFromFields()

	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = err.Error()
		return errs
	}

	if scenario == "update" {
		if product.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsProductExists(&product.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Product:" + product.ID.Hex()
		}

	}

	err = product.SetStock()
	if err != nil {
		errs["stock"] = "error setting stock: " + err.Error()
		return errs
	}

	/*
		_, ok := product.ProductStores[store.ID.Hex()]
		if ok {
			if product.ProductStores[store.ID.Hex()].DamagedStock > product.ProductStores[store.ID.Hex()].Stock {
				errs["damaged_stock_0"] = " damaged stock should be greater than stock value"
			}
		}*/

	if govalidator.IsNull(product.Name) {
		errs["name"] = "Name is required"
	} else if len(product.Name) < 3 {
		errs["name"] = "Name length should be min. 3 chars"
	} else if len(product.Name) > 500 {
		errs["name"] = "Name length should be max. 500 chars"
	}

	/*
		if !govalidator.IsNull(product.ItemCode) {
			exists, err := product.IsItemCodeExists()
			if err != nil {
				errs["item_code"] = err.Error()
			}

			if exists {
				errs["item_code"] = "Item Code Already Exists"
			}
		}
	*/

	if !govalidator.IsNull(product.PartNumber) {
		exists, err := product.IsPartNumberExists()
		if err != nil {
			errs["part_number"] = err.Error()
		}

		if exists {
			errs["part_number"] = "Part Number Already Exists"
		}
	}

	for storeID, _ := range product.ProductStores {
		if productStoreTemp, ok := product.ProductStores[storeID]; ok {
			productStoreTemp.PurchaseUnitPriceSecret = GenerateSecretCode(int(product.ProductStores[storeID].PurchaseUnitPrice))
			product.ProductStores[storeID] = productStoreTemp
		}
	}

	//if len(product.CategoryID) == 0 {
	//errs["category_id"] = "Atleast 1 category is required"
	//}
	if len(product.CategoryID) > 0 {
		for i, categoryID := range product.CategoryID {
			exists, err := store.IsProductCategoryExists(categoryID)
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

func (store *Store) FindLastProduct(
	selectFields map[string]interface{},
) (product *Product, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	//collection.Indexes().CreateOne()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}
	findOneOptions.SetSort(map[string]interface{}{"_id": -1})

	err = collection.FindOne(ctx,
		bson.M{
			//"store_id": store.ID,
		}, findOneOptions).
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

func (product *Product) SetBarcode() error {
	if product.Ean12 != "" {
		return nil
	}

	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := product.StoreID.Hex() + "_product_barcode_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		lastProduct, err := store.FindLastProduct(bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("error finding last product: " + err.Error())
		}

		if lastProduct == nil {
			err = db.RedisClient.Set(redisKey, 100000000000, 0).Err()
			if err != nil {
				return err
			}
		} else {

			lastEan12, err := strconv.Atoi(lastProduct.Ean12)
			if err != nil {
				return errors.New("error converting  ean12-1 string to int: " + err.Error())
			}
			err = db.RedisClient.Set(redisKey, lastEan12, 0).Err()
			if err != nil {
				return err
			}
		}
	}

	incr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	product.Ean12 = strconv.Itoa(int(incr))
	return nil
}

/*
func (product *Product) SetBarcode() (err error) {
	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if len(product.Ean12) == 0 {
		lastProduct, err := store.FindLastProduct(bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			return errors.New("error finding last product: " + err.Error())
		}
		barcode := ""
		if lastProduct != nil {
			lastEan12, err := strconv.Atoi(lastProduct.Ean12)
			if err != nil {
				return errors.New("error converting  ean12-1 string to int: " + err.Error())
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
				return errors.New("error checking ean12 exists or not " + err.Error())
			}
			if !exists {
				break
			}
			lastEan12, err := strconv.Atoi(product.Ean12)
			if err != nil {
				return errors.New("error converting  ean12-2 string to int: " + err.Error())
			}
			lastEan12++
			barcode = strconv.Itoa(lastEan12)
		}
	}
	return nil
}
*/

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
			productStoreTemp.RetailUnitProfit = RoundTo2Decimals(product.ProductStores[i].RetailUnitPrice - product.ProductStores[i].PurchaseUnitPrice)
			productStoreTemp.WholesaleUnitProfit = RoundTo2Decimals(product.ProductStores[i].WholesaleUnitPrice - product.ProductStores[i].PurchaseUnitPrice)
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
				productStoreTemp.RetailUnitProfitPerc = RoundTo2Decimals(productStoreTemp.RetailUnitProfitPerc)
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
				productStoreTemp.WholesaleUnitProfitPerc = RoundTo2Decimals(productStoreTemp.WholesaleUnitProfitPerc)
				product.ProductStores[i] = productStoreTemp
			}
			//product.ProductStores[i].WholesaleUnitProfitPerc = (product.Stores[i].WholesaleUnitProfit / product.Stores[i].PurchaseUnitPrice) * 100
		}
	}
	return nil
}

func (product *Product) Insert() (err error) {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	product.ID = primitive.NewObjectID()

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

func (product *Product) Update(storeID *primitive.ObjectID) error {
	var collection *mongo.Collection
	if storeID != nil {
		collection = db.GetDB("store_" + storeID.Hex()).Collection("product")
	} else {
		collection = db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	}

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
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
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

func (product *Product) RestoreProduct(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = product.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	product.Deleted = false
	product.DeletedBy = nil
	product.DeletedAt = nil

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

func (store *Store) FindProductByItemCode(
	itemCode string,
	selectFields map[string]interface{},
) (product *Product, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"item_code": itemCode,
			"store_id":  store.ID,
		}, findOneOptions).
		Decode(&product)
	if err != nil {
		return nil, err
	}

	product.SearchLabel = product.Name + " (Part #" + product.PartNumber + ", Arabic: " + product.NameInArabic + ")"

	return product, err
}

func (store *Store) FindProductByBarCode(
	barCode string,
	selectFields map[string]interface{},
) (product *Product, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	criteria := make(map[string]interface{})
	criteria["store_id"] = store.ID

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

// []*primitive.ObjectID
func (store *Store) FindProductsByIDs(
	IDs []*primitive.ObjectID,
) (products []*Product, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{
		"_id": bson.M{"$in": IDs},
		//"store_id": store.ID,
	}, findOptions)
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

		product.SetSearchLabel(&store.ID)

		products = append(products, &product)
	} //end for loop

	return products, err
}

func (store *Store) FindProductByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (product *Product, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"_id": ID,
			//"store_id": store.ID,
		}, findOneOptions).
		Decode(&product)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["category.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "category")
		for _, categoryID := range product.CategoryID {
			category, _ := store.FindProductCategoryByID(categoryID, fields)
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
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
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
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"part_number": product.PartNumber,
			"store_id":    product.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"part_number": product.PartNumber,
			"store_id":    product.StoreID,
			"_id":         bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func (product *Product) IsBarCodeExists() (exists bool, err error) {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"bar_code": product.BarCode,
			"store_id": product.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"bar_code": product.BarCode,
			"store_id": product.StoreID,
			"_id":      bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func (product *Product) IsEan12Exists() (exists bool, err error) {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if product.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"ean_12":   product.Ean12,
			"store_id": product.StoreID,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"ean_12":   product.Ean12,
			"store_id": product.StoreID,
			"_id":      bson.M{"$ne": product.ID},
		})
	}

	return (count > 0), err
}

func (store *Store) IsProductExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (product *Product) ReflectValidPurchaseUnitPrice() error {
	store, err := FindStoreByID(product.StoreID, bson.M{})
	if err != nil {
		return err
	}

	salesHistories, err := store.GetSalesHistoriesByProductID(&product.ID)
	if err != nil {
		return errors.New("Error fetching sales history of product:" + err.Error())
	}

	if len(salesHistories) == 0 {
		return nil
	}

	for _, salesHistory := range salesHistories {
		order, err := store.FindOrderByID(salesHistory.OrderID, map[string]interface{}{})
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

	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		log.Print("Branch name:" + store.BranchName)

		totalCount, err := store.GetTotalCount(bson.M{}, "product")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
		ctx := context.Background()
		findOptions := options.Find()
		findOptions.SetNoCursorTimeout(true)
		//findOptions.SetProjection(bson.M{"_id": 1, "name": 1, "part_number": 1, "name_in_arabic": 1, "store_id": 1, "product_stores": 1})
		findOptions.SetAllowDiskUse(true)

		cur, err := collection.Find(ctx, bson.M{}, findOptions)
		if err != nil {
			return errors.New("Error fetching products" + err.Error())
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
			product := Product{}
			err = cur.Decode(&product)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			//if product.PartNumber == `GI RED BUSH 1X1/2"` {
			//product.GeneratePrefixes()

			/*
				if !isValidUTF8(product.Name) {
					log.Print("Name:" + product.Name)
					log.Print(product.Name)
					log.Print(product.ID)
					log.Print(product.PartNumber)
					log.Print("Invalid UTF-8 detected in product name")
					//log.Fatal("Invalid UTF-8 detected in product name")
				}

				if !isValidUTF8(product.NameInArabic) {
					log.Print("Name in Arabic:" + product.NameInArabic)
					log.Print(product.NameInArabic)
					log.Print(product.ID)
					log.Print(product.PartNumber)
					log.Print("Invalid UTF-8 detected in product arabic name")
					//log.Fatal("Invalid UTF-8 detected in product name")
				}
				//log.Print(product.NamePrefixes)
				//log.Print(product.NameInArabicPrefixes)

				for _, word := range product.NamePrefixes {
					if word == "" {
						log.Fatal("Empty prefix found")
					}

					if !isValidUTF8(word) {
						log.Print("Word:" + word)
						log.Print(product.Name)
						log.Print(product.ID)
						log.Print(product.PartNumber)
						log.Print("Invalid UTF-8 detected in product name")
						//log.Fatal("Invalid UTF-8 detected in product name")
					}
				}

				for _, word := range product.NameInArabicPrefixes {
					if word == "" {
						log.Fatal("Empty prefix found")
					}

					if !isValidUTF8(word) {
						log.Print("Word:" + word)
						log.Print(product.Name)
						log.Print(product.ID)
						log.Print("Invalid UTF-8 detected in product name in arabic")
						//log.Fatal("Invalid UTF-8 detected in product name in arabic")
					}
				}*/
			//}

			//product.TrimSpaceFromFields()
			//log.Print(product.PartNumber)

			//product.NamePrefixes = []string{}
			//product.NameInArabicPrefixes = []string{}

			//product.BarcodeBase64 = ""

			if product.StoreID.Hex() != store.ID.Hex() {
				continue
			}

			/*
				err = product.SetStock()
				if err != nil {
					return err
				}*/
			//product.GeneratePrefixes()
			product.BarcodeBase64 = ""
			err = product.Update(&store.ID)
			if err != nil {
				return err
			}

			bar.Add(1)
		}
	}

	log.Print("DONE!")
	return nil
}

func isValidUTF8(str string) bool {
	// Check if the string is valid UTF-8
	return utf8.ValidString(str)
}

func (product *Product) HardDelete() error {
	log.Print("Deleting product")
	ctx := context.Background()
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product")
	_, err := collection.DeleteOne(ctx, bson.M{
		"_id": product.ID,
	})
	if err != nil {
		return err
	}

	return nil
}

func removeInvalidChars(input string) string {
	var cleaned []rune

	// Loop through each character in the input string
	for _, r := range input {
		// Check if the character is a valid UTF-8 character or space
		if utf8.ValidRune(r) && (unicode.IsPrint(r) || r == ' ') {
			cleaned = append(cleaned, r)
		}
	}

	// Return the cleaned string
	return removeSpecialCharacter(string(cleaned))
}

// Function to generate prefixes for a string with both English and Arabic words
func generatePrefixes(input string) []string {
	var prefixes []string
	words := strings.Fields(input) // split into words (no need to lowercase, as both languages are case-insensitive)

	for _, word := range words {
		word = removeSpecialCharacter(word)
		if word == "" {
			continue
		}
		// Generate prefixes for each word
		for i := 1; i <= len(word); i++ {
			newWord := word[:i]
			//log.Print("Before removing:" + newWord + "|")
			newWord = CleanString(newWord)
			newWord = removeSpecialCharacter(newWord)
			//log.Print("After removing:" + newWord + "|")
			if newWord == "" {
				continue
			}
			prefixes = append(prefixes, newWord)
		}
	}

	return prefixes
}

func generatePrefixesSuffixesSubstrings(input string) []string {
	uniqueSet := make(map[string]struct{})
	words := strings.Fields(input)

	for _, word := range words {
		word = CleanString(removeSpecialCharacter(word))
		if word == "" {
			continue
		}

		runes := []rune(word)
		length := len(runes)

		// Generate prefixes
		for i := 1; i <= length; i++ {
			prefix := string(runes[:i])
			uniqueSet[prefix] = struct{}{}
		}

		// Generate suffixes
		for i := 0; i < length; i++ {
			suffix := string(runes[i:])
			uniqueSet[suffix] = struct{}{}
		}

		// Generate all substrings
		for start := 0; start < length; start++ {
			for end := start + 1; end <= length; end++ {
				substring := string(runes[start:end])
				uniqueSet[substring] = struct{}{}
			}
		}
	}

	// Convert map keys to slice
	var result []string
	for str := range uniqueSet {
		cleaned := CleanString(removeSpecialCharacter(str))
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}

	return result
}

func removeSpecialCharacter(input string) string {
	// Replace the '�' character with an empty string
	return strings.ReplaceAll(input, "�", "")
}

func (product *Product) GeneratePrefixes() {
	cleanName := CleanString(product.PrefixPartNumber + " - " + product.PartNumber + " " + product.Name + " " + product.BrandName + " " + product.CountryName)
	cleanNameArabic := CleanString(product.NameInArabic)

	product.NamePrefixes = generatePrefixesSuffixesSubstrings(cleanName)
	if cleanNameArabic != "" {
		product.NameInArabicPrefixes = generatePrefixesSuffixesSubstrings(cleanNameArabic)
	}
}

/*
func (product *Product) GeneratePrefixes() {

	if containsArabic(product.Name) {
		product.NamePrefixes = generateArabicPrefixes(CleanString(product.Name))
	} else {
		product.NamePrefixes = generatePrefixes(CleanString(product.Name))
	}

	if product.NameInArabic != "" {
		product.NameInArabicPrefixes = generateArabicPrefixes(CleanString(product.NameInArabic))
	}
}
*/

func containsArabic(input string) bool {
	for _, r := range input {
		// Check if the rune is within the Arabic Unicode range
		if (r >= '\u0600' && r <= '\u06FF') || (r >= '\u0750' && r <= '\u077F') || (r >= '\u08A0' && r <= '\u08FF') {
			return true // Arabic character found
		}
	}
	return false // No Arabic characters found
}

var specialCharEscaper = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

func CleanString(input string) string {
	// Replace special characters with a space
	cleaned := specialCharEscaper.ReplaceAllString(input, " ")
	// Now sanitize the result
	//cleaned = sanitizeString(cleaned)
	// Remove extra spaces and trim leading/trailing spaces
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = sanitizeUTF8(cleaned)
	//cleaned = sanitizeArabString(cleaned)
	cleaned = removeSpecialCharacter(cleaned)
	cleaned = removeInvalidChars(cleaned)
	return cleaned
}

func sanitizeUTF8(input string) string {
	// Ensure that we keep only valid UTF-8 characters and replace any invalid sequences
	if !utf8.ValidString(input) {
		// Replace invalid characters with a placeholder or remove them entirely
		input = regexp.MustCompile(`[^a-zA-Z0-9\u0600-\u06FF]+`).ReplaceAllString(input, " ")
	}
	return input
}

func sanitizeString(input string) string {
	// Ensure the string is valid UTF-8
	if !utf8.ValidString(input) {
		input = regexp.MustCompile(`[^a-zA-Z0-9\u0600-\u06FF]+`).ReplaceAllString(input, " ")
	}
	return input
}

func replaceSpecialCharsWithSpace(input string) string {
	// This matches anything not a letter or number
	re := regexp.MustCompile(`[^\\p{L}\\p{N}]+`)
	// Replace all special characters with a space
	cleaned := re.ReplaceAllString(input, " ")
	return strings.TrimSpace(cleaned)
}

func (product *Product) SetStock() error {
	err := product.SetProductSalesQuantityByStoreID(*product.StoreID)
	if err != nil {
		return err
	}
	err = product.SetProductSalesReturnQuantityByStoreID(*product.StoreID)
	if err != nil {
		return err
	}
	err = product.SetProductPurchaseQuantityByStoreID(*product.StoreID)
	if err != nil {
		return err
	}
	err = product.SetProductPurchaseReturnQuantityByStoreID(*product.StoreID)
	if err != nil {
		return err
	}
	if productStoreTemp, ok := product.ProductStores[product.StoreID.Hex()]; ok {
		productStoreTemp.Stock = (productStoreTemp.PurchaseQuantity - productStoreTemp.PurchaseReturnQuantity) - (productStoreTemp.SalesQuantity - productStoreTemp.SalesReturnQuantity)
		productStoreTemp.Stock -= productStoreTemp.DamagedStock
		product.ProductStores[product.StoreID.Hex()] = productStoreTemp
	}

	return nil
}
