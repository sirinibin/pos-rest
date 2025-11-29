package models

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func SetIndexes() error {
	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		store.RemoveAllIndexes()
		err = store.CreateAllIndexes()
		if err != nil {
			return err
		}
	}

	return nil
}

func (store *Store) CreateAllIndexes() error {
	//product
	textFields := bson.D{
		bson.E{Key: "name", Value: "text"},
		bson.E{Key: "name_prefixes", Value: "text"},
		bson.E{Key: "name_in_arabic", Value: "text"},
		bson.E{Key: "name_in_arabic_prefixes", Value: "text"},
		bson.E{Key: "part_number", Value: "text"},
	}
	err := store.CreateTextIndex("product", textFields, "product_text_index")
	if err != nil {
		return err
	}

	fields := bson.M{"bar_code": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"category_id": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"brand_id": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"country_code": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"ean_12": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}

	//customer
	textFields = bson.D{
		bson.E{Key: "name", Value: "text"},
		bson.E{Key: "name_in_arabic", Value: "text"},
		bson.E{Key: "code", Value: "text"},
		bson.E{Key: "phone", Value: "text"},
		bson.E{Key: "phone_in_arabic", Value: "text"},
		bson.E{Key: "phone2", Value: "text"},
		bson.E{Key: "phone2_in_arabic", Value: "text"},
		bson.E{Key: "vat_no", Value: "text"},
		bson.E{Key: "vat_no_in_arabic", Value: "text"},
		bson.E{Key: "email", Value: "text"},
		bson.E{Key: "search_words_in_arabic", Value: "text"},
		bson.E{Key: "search_words", Value: "text"},
		bson.E{Key: "country_name", Value: "text"},
	}
	err = store.CreateTextIndex("customer", textFields, "customer_text_index")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("customer", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"vat_no": 1}
	err = store.CreateIndex("customer", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"phone": 1}
	err = store.CreateIndex("customer", fields, false, false, "")
	if err != nil {
		return err
	}

	//vendor
	textFields = bson.D{
		bson.E{Key: "name", Value: "text"},
		bson.E{Key: "name_in_arabic", Value: "text"},
		bson.E{Key: "code", Value: "text"},
		bson.E{Key: "phone", Value: "text"},
		bson.E{Key: "phone_in_arabic", Value: "text"},
		bson.E{Key: "vat_no", Value: "text"},
		bson.E{Key: "vat_no_in_arabic", Value: "text"},
		bson.E{Key: "email", Value: "text"},
		bson.E{Key: "search_words_in_arabic", Value: "text"},
		bson.E{Key: "search_words", Value: "text"},
		bson.E{Key: "country_name", Value: "text"},
	}
	err = store.CreateTextIndex("vendor", textFields, "vendor_text_index")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("vendor", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"vat_no": 1}
	err = store.CreateIndex("vendor", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"phone": 1}
	err = store.CreateIndex("vendor", fields, false, false, "")
	if err != nil {
		return err
	}

	//order
	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"invoice_count_value": 1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

	//salesreturn
	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"invoice_count_value": 1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	//purchase
	fields = bson.M{"vendor_id": 1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	//purchase return
	fields = bson.M{"vendor_id": 1}
	err = store.CreateIndex("purchasereturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("purchasereturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("purchasereturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("purchasereturn", fields, false, false, "")
	if err != nil {
		return err
	}

	//quotation
	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("quotation", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("quotation", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("quotation", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("quotation", fields, false, false, "")
	if err != nil {
		return err
	}

	//delivery_note
	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("delivery_note", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("delivery_note", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("delivery_note", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("delivery_note", fields, false, false, "")
	if err != nil {
		return err
	}

	// product_history
	// Add these inside func (store *Store) CreateAllIndexes():

	// product_history collection indexes
	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_id": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_type": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"vendor_id": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_history", fields, false, false, "")
	if err != nil {
		return err
	}

	// product_sales_history
	// Add these inside func (store *Store) CreateAllIndexes():

	// product_sales_history collection indexes
	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"order_id": 1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"order_code": 1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_sales_history", fields, false, false, "")
	if err != nil {
		return err
	}

	// posting collection indexes
	fields = bson.M{"account_id": 1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_id": 1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_model": 1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_code": 1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"account_number": 1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	// For queries on embedded posts array fields:
	fields = bson.M{"posts.account_id": 1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"posts.date": -1}
	err = store.CreateIndex("posting", fields, false, false, "")
	if err != nil {
		return err
	}

	//Ledger
	fields = bson.M{"reference_id": 1}
	err = store.CreateIndex("ledger", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_model": 1}
	err = store.CreateIndex("ledger", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"reference_code": 1}
	err = store.CreateIndex("ledger", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("ledger", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"journals.account_id": 1}
	err = store.CreateIndex("ledger", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"journals.date": -1}
	err = store.CreateIndex("ledger", fields, false, false, "")
	if err != nil {
		return err
	}

	// Add these inside func (store *Store) CreateAllIndexes():

	// product_quotation_sales_return_history collection indexes

	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"quotation_id": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"quotation_code": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"quotation_sales_return_id": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"quotation_sales_return_code": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_quotation_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	// Add these inside func (store *Store) CreateAllIndexes():

	// product_quotation_history collection indexes
	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"quotation_id": 1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"quotation_code": 1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"updated_at": -1}
	err = store.CreateIndex("product_quotation_history", fields, false, false, "")
	if err != nil {
		return err
	}

	// Add these inside func (store *Store) CreateAllIndexes():

	// product_purchase_return_history collection indexes
	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"vendor_id": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_return_id": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_return_code": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_id": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_code": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"updated_at": -1}
	err = store.CreateIndex("product_purchase_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	// Add these inside func (store *Store) CreateAllIndexes():

	// product_purchase_history collection indexes
	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"vendor_id": 1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_id": 1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_code": 1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"updated_at": -1}
	err = store.CreateIndex("product_purchase_history", fields, false, false, "")
	if err != nil {
		return err
	}

	// Add these inside func (store *Store) CreateAllIndexes():

	// product_sales_return_history collection indexes
	fields = bson.M{"store_id": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"product_id": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"customer_id": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"order_id": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"order_code": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"sales_return_id": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"sales_return_code": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_id": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"warehouse_code": 1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"updated_at": -1}
	err = store.CreateIndex("product_sales_return_history", fields, false, false, "")
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) RemoveAllIndexes() {
	log.Print("Removing all indexes")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("vendor")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("order")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("purchase")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("delivery_note")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("quotation")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_history")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_history")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("posting")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("ledger")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_sales_return_history")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_quotation_history")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_return_history")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_purchase_history")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("product_sales_return_history")
	collection.Indexes().DropAll(context.Background())

}

// CreateIndex - creates an index for a specific field in a collection
func (store *Store) CreateIndex(collectionName string, fields bson.M, unique bool, text bool, overrideLang string) error {
	//collection := db.Client("").Database(db.GetPosDB()).Collection(collectionName)
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	//collection.Indexes().DropAll(context.Background())

	indexOptions := options.Index()
	if text {
		indexOptions.SetDefaultLanguage("english")
	}

	if unique {
		indexOptions.SetUnique(true)
	}

	if overrideLang != "" {
		indexOptions.SetLanguageOverride(overrideLang)
	}

	// 1. Lets define the keys for the index we want to create
	//var mod mongo.IndexModel
	mod := mongo.IndexModel{
		Keys:    fields, // index in ascending order or -1 for descending order
		Options: indexOptions,
	}

	// 2. Create the context for this operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 4. Create a single index
	indexName, err := collection.Indexes().CreateOne(ctx, mod)
	if err != nil {
		// 5. Something went wrong, we log it and return false
		log.Printf("Failed to create Index for field:%v, collection: %s", fields, collectionName)
		fmt.Println(err.Error())
		return err
	}

	log.Printf("Created Index:%s for collection:%s for fields %v", indexName, collectionName, fields)

	// 6. All went well, we return true
	return nil
}

func (store *Store) CreateTextIndex(collectionName string, fields bson.D, indexName string) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexModel := mongo.IndexModel{
		Keys:    fields,
		Options: options.Index().SetName(indexName).SetUnique(false).SetDefaultLanguage("none"),
	}

	createdIndexName, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("Failed to create text index: %v", err)
		return err
	}

	fmt.Println("Created text index:", createdIndexName)
	return nil
}
