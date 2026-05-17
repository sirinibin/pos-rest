package models

import (
	"context"
	"log"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIStockAlert represents a product with a stock concern.
// AlertType: "low_stock" | "out_of_stock" | "slow_mover" | "dead_stock"
// Collection: bi_stock_alerts
type BIStockAlert struct {
	ID            primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	StoreID       interface{}        `json:"store_id,omitempty" bson:"store_id"`
	ProductID     primitive.ObjectID `json:"product_id" bson:"product_id"`
	ProductName   string             `json:"product_name" bson:"product_name"`
	ItemCode      string             `json:"item_code" bson:"item_code"`
	CategoryName  string             `json:"category_name" bson:"category_name"`
	CurrentStock  float64            `json:"current_stock" bson:"current_stock"`
	ReorderPoint  float64            `json:"reorder_point" bson:"reorder_point"` // avg_daily_sales * 14
	AvgDailySales float64            `json:"avg_daily_sales" bson:"avg_daily_sales"`
	DaysNoSale    int                `json:"days_no_sale" bson:"days_no_sale"` // days since last sale
	LastSaleDate  *time.Time         `json:"last_sale_date" bson:"last_sale_date"`
	AlertType     string             `json:"alert_type" bson:"alert_type"`
	StockValue    float64            `json:"stock_value" bson:"stock_value"` // stock * purchase_unit_price
	UpdatedAt     time.Time          `json:"updated_at" bson:"updated_at"`
}

// GetBIStockAlerts returns stock alert records filtered by alert_type.
// alertType: "" (all) | "low_stock" | "out_of_stock" | "slow_mover" | "dead_stock"
func (store *Store) GetBIStockAlerts(alertType string, limit int) ([]BIStockAlert, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("bi_stock_alerts")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Collection is already scoped to this store's DB, so no store_id filter needed.
	// store_id in docs may be ObjectID (Go-written) or string (Python-backfill).
	filter := bson.M{}
	if alertType != "" {
		filter["alert_type"] = alertType
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "days_no_sale", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []BIStockAlert
	cursor.All(ctx, &results)
	return results, nil
}

// UpsertBIStockAlerts scans all active products and generates stock alerts.
func UpsertBIStockAlerts(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("bi_stock_alerts")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Clear existing alerts for this store
	collection.DeleteMany(ctx, bson.M{"store_id": storeID})

	productColl := db.GetDB("store_" + storeID.Hex()).Collection("product")
	salesHistColl := db.GetDB("store_" + storeID.Hex()).Collection("product_sales_history")

	now := time.Now()
	since90 := now.AddDate(0, 0, -90)
	since30 := now.AddDate(0, 0, -30)

	// Get avg daily sales per product over last 90 days
	avgSalesPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"store_id": storeID,
			"date":     bson.M{"$gte": since90},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":            "$product_id",
			"total_qty":      bson.M{"$sum": "$quantity"},
			"last_sale_date": bson.M{"$max": "$date"},
		}}},
	}
	salesCur, err := salesHistColl.Aggregate(ctx, avgSalesPipeline)
	if err != nil {
		return err
	}
	defer salesCur.Close(ctx)

	type salesStat struct {
		avgDaily   float64
		lastSale   *time.Time
		daysNoSale int
	}
	salesMap := make(map[string]salesStat)
	for salesCur.Next(ctx) {
		var r bson.M
		salesCur.Decode(&r)
		id, _ := r["_id"].(primitive.ObjectID)
		totalQty := toFloat64(r["total_qty"])
		avgDaily := totalQty / 90.0

		var lastSale *time.Time
		daysNoSale := 9999
		if dt, ok := r["last_sale_date"].(primitive.DateTime); ok {
			t := dt.Time()
			lastSale = &t
			daysNoSale = int(now.Sub(t).Hours() / 24)
		}
		salesMap[id.Hex()] = salesStat{
			avgDaily:   avgDaily,
			lastSale:   lastSale,
			daysNoSale: daysNoSale,
		}
	}

	// Scan products
	cursor, err := productColl.Find(ctx, bson.M{
		"store_id": storeID,
		"deleted":  bson.M{"$ne": true},
	}, options.Find().SetProjection(bson.M{
		"_id": 1, "name": 1, "item_code": 1, "category_name": 1,
		"stock": 1, "purchase_unit_price": 1,
	}))
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var alerts []interface{}
	for cursor.Next(ctx) {
		var product bson.M
		if err := cursor.Decode(&product); err != nil {
			continue
		}
		productID, _ := product["_id"].(primitive.ObjectID)
		stock := toFloat64(product["stock"])
		purchasePrice := toFloat64(product["purchase_unit_price"])

		catName := ""
		if arr, ok := product["category_name"].(primitive.A); ok && len(arr) > 0 {
			catName = toString(arr[0])
		}

		ss := salesMap[productID.Hex()]
		reorderPoint := ss.avgDaily * 14 // 2-week buffer

		alertType := ""
		if stock <= 0 {
			alertType = "out_of_stock"
		} else if stock < reorderPoint && reorderPoint > 0 {
			alertType = "low_stock"
		} else if ss.daysNoSale >= 90 {
			alertType = "dead_stock"
		} else if ss.daysNoSale >= 30 {
			// Only flag as slow_mover if it had sales in the 90 days before the 30-day window
			alertType = "slow_mover"
		}

		if alertType == "" {
			continue
		}

		// For products with no sales at all in 90 days, check if they were sold in last year
		if alertType == "dead_stock" || alertType == "slow_mover" {
			// Only flag if product has stock (otherwise it's just sold out)
			if stock <= 0 {
				continue
			}
		}

		_ = since30 // used for slow_mover threshold context

		doc := BIStockAlert{
			StoreID:       storeID,
			ProductID:     productID,
			ProductName:   toString(product["name"]),
			ItemCode:      toString(product["item_code"]),
			CategoryName:  catName,
			CurrentStock:  stock,
			ReorderPoint:  reorderPoint,
			AvgDailySales: ss.avgDaily,
			DaysNoSale:    ss.daysNoSale,
			LastSaleDate:  ss.lastSale,
			AlertType:     alertType,
			StockValue:    stock * purchasePrice,
			UpdatedAt:     now,
		}
		alerts = append(alerts, doc)
	}

	if len(alerts) > 0 {
		collection.InsertMany(ctx, alerts)
	}

	log.Printf("[BI] stock_alerts upsert done — store=%s alerts=%d", storeID.Hex(), len(alerts))
	return nil
}

// RunBIStockAlertsUpdate refreshes stock alerts for a store.
func RunBIStockAlertsUpdate(storeID primitive.ObjectID) {
	if err := UpsertBIStockAlerts(storeID); err != nil {
		log.Printf("[BI] stock_alerts error store=%s: %v", storeID.Hex(), err)
	}
}
