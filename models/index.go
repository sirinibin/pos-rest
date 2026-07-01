package models

import (
	"context"
	"fmt"
	"log"
	"strings"
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
	var errs []string

	idx := func(coll string, fields bson.M) {
		if err := store.CreateIndex(coll, fields, false, false, ""); err != nil {
			errs = append(errs, fmt.Sprintf("%s %v: %v", coll, fields, err))
		}
	}
	tidx := func(coll string, fields bson.D, name string) {
		if err := store.CreateTextIndex(coll, fields, name); err != nil {
			errs = append(errs, fmt.Sprintf("%s text(%s): %v", coll, name, err))
		}
	}
	cidx := func(coll string, fields bson.D) {
		if err := store.CreateCompoundIndex(coll, fields); err != nil {
			errs = append(errs, fmt.Sprintf("%s compound%v: %v", coll, fields, err))
		}
	}

	// product
	tidx("product", bson.D{
		{Key: "name", Value: "text"},
		{Key: "name_prefixes", Value: "text"},
		{Key: "name_in_arabic", Value: "text"},
		{Key: "name_in_arabic_prefixes", Value: "text"},
		{Key: "part_number", Value: "text"},
		{Key: "item_code", Value: "text"},
		{Key: "ean_12", Value: "text"},
	}, "product_text_index")
	idx("product", bson.M{"name_prefixes": 1})
	idx("product", bson.M{"part_number": 1})
	idx("product", bson.M{"prefix_part_number": 1})
	idx("product", bson.M{"item_code": 1})
	idx("product", bson.M{"bar_code": 1})
	idx("product", bson.M{"category_id": 1})
	idx("product", bson.M{"brand_id": 1})
	idx("product", bson.M{"country_code": 1})
	idx("product", bson.M{"ean_12": 1})
	idx("product", bson.M{"deleted": 1})

	// customer
	tidx("customer", bson.D{
		{Key: "name", Value: "text"},
		{Key: "name_in_arabic", Value: "text"},
		{Key: "code", Value: "text"},
		{Key: "phone", Value: "text"},
		{Key: "phone_in_arabic", Value: "text"},
		{Key: "phone2", Value: "text"},
		{Key: "phone2_in_arabic", Value: "text"},
		{Key: "vat_no", Value: "text"},
		{Key: "vat_no_in_arabic", Value: "text"},
		{Key: "email", Value: "text"},
		{Key: "search_words_in_arabic", Value: "text"},
		{Key: "search_words", Value: "text"},
		{Key: "country_name", Value: "text"},
	}, "customer_text_index")
	idx("customer", bson.M{"search_words": 1})
	idx("customer", bson.M{"code": 1})
	idx("customer", bson.M{"vat_no": 1})
	idx("customer", bson.M{"phone": 1})
	idx("customer", bson.M{"created_at": -1})
	cidx("customer", bson.D{{Key: "deleted", Value: 1}, {Key: "created_at", Value: -1}})

	// vendor
	tidx("vendor", bson.D{
		{Key: "name", Value: "text"},
		{Key: "name_in_arabic", Value: "text"},
		{Key: "code", Value: "text"},
		{Key: "phone", Value: "text"},
		{Key: "phone_in_arabic", Value: "text"},
		{Key: "vat_no", Value: "text"},
		{Key: "vat_no_in_arabic", Value: "text"},
		{Key: "email", Value: "text"},
		{Key: "search_words_in_arabic", Value: "text"},
		{Key: "search_words", Value: "text"},
		{Key: "country_name", Value: "text"},
	}, "vendor_text_index")
	idx("vendor", bson.M{"search_words": 1})
	idx("vendor", bson.M{"code": 1})
	idx("vendor", bson.M{"vat_no": 1})
	idx("vendor", bson.M{"phone": 1})
	cidx("vendor", bson.D{{Key: "deleted", Value: 1}, {Key: "created_at", Value: -1}})

	// order
	idx("order", bson.M{"customer_id": 1})
	idx("order", bson.M{"date": -1})
	idx("order", bson.M{"created_at": -1})
	idx("order", bson.M{"code": 1})
	idx("order", bson.M{"invoice_count_value": 1})
	idx("order", bson.M{"payment_status": 1})
	cidx("order", bson.D{{Key: "deleted", Value: 1}})
	cidx("order", bson.D{{Key: "zatca.reporting_passed", Value: 1}, {Key: "zatca.reporting_passed_at", Value: -1}})

	// salesreturn
	idx("salesreturn", bson.M{"customer_id": 1})
	idx("salesreturn", bson.M{"date": -1})
	idx("salesreturn", bson.M{"created_at": -1})
	idx("salesreturn", bson.M{"code": 1})
	idx("salesreturn", bson.M{"invoice_count_value": 1})
	cidx("salesreturn", bson.D{{Key: "deleted", Value: 1}})

	// purchase
	idx("purchase", bson.M{"vendor_id": 1})
	idx("purchase", bson.M{"date": -1})
	idx("purchase", bson.M{"created_at": -1})
	idx("purchase", bson.M{"code": 1})
	cidx("purchase", bson.D{{Key: "deleted", Value: 1}})

	// purchasereturn
	idx("purchasereturn", bson.M{"vendor_id": 1})
	idx("purchasereturn", bson.M{"date": -1})
	idx("purchasereturn", bson.M{"created_at": -1})
	idx("purchasereturn", bson.M{"code": 1})
	cidx("purchasereturn", bson.D{{Key: "deleted", Value: 1}})

	// quotation
	idx("quotation", bson.M{"customer_id": 1})
	idx("quotation", bson.M{"date": -1})
	idx("quotation", bson.M{"created_at": -1})
	idx("quotation", bson.M{"code": 1})
	cidx("quotation", bson.D{{Key: "deleted", Value: 1}})

	// delivery_note
	idx("delivery_note", bson.M{"customer_id": 1})
	idx("delivery_note", bson.M{"date": -1})
	idx("delivery_note", bson.M{"created_at": -1})
	idx("delivery_note", bson.M{"code": 1})
	cidx("delivery_note", bson.D{{Key: "deleted", Value: 1}})

	// product_history
	idx("product_history", bson.M{"product_id": 1})
	idx("product_history", bson.M{"reference_id": 1})
	idx("product_history", bson.M{"reference_type": 1})
	idx("product_history", bson.M{"date": -1})
	idx("product_history", bson.M{"created_at": -1})
	idx("product_history", bson.M{"customer_id": 1})
	idx("product_history", bson.M{"vendor_id": 1})
	idx("product_history", bson.M{"warehouse_id": 1})
	idx("product_history", bson.M{"warehouse_code": 1})

	// product_sales_history
	idx("product_sales_history", bson.M{"product_id": 1})
	idx("product_sales_history", bson.M{"customer_id": 1})
	idx("product_sales_history", bson.M{"order_id": 1})
	idx("product_sales_history", bson.M{"order_code": 1})
	idx("product_sales_history", bson.M{"date": -1})
	idx("product_sales_history", bson.M{"created_at": -1})
	idx("product_sales_history", bson.M{"warehouse_id": 1})
	idx("product_sales_history", bson.M{"warehouse_code": 1})

	// posting
	idx("posting", bson.M{"account_id": 1})
	idx("posting", bson.M{"reference_id": 1})
	idx("posting", bson.M{"reference_model": 1})
	idx("posting", bson.M{"reference_code": 1})
	idx("posting", bson.M{"date": -1})
	idx("posting", bson.M{"account_number": 1})
	idx("posting", bson.M{"created_at": -1})
	idx("posting", bson.M{"posts.account_id": 1})
	idx("posting", bson.M{"posts.date": -1})
	cidx("posting", bson.D{{Key: "deleted", Value: 1}})
	cidx("posting", bson.D{{Key: "account_id", Value: 1}, {Key: "date", Value: 1}})

	// ledger
	idx("ledger", bson.M{"reference_id": 1})
	idx("ledger", bson.M{"reference_model": 1})
	idx("ledger", bson.M{"reference_code": 1})
	idx("ledger", bson.M{"created_at": -1})
	idx("ledger", bson.M{"journals.account_id": 1})
	idx("ledger", bson.M{"journals.date": -1})
	idx("ledger", bson.M{"deleted": 1})

	// product_quotation_sales_return_history
	idx("product_quotation_sales_return_history", bson.M{"product_id": 1})
	idx("product_quotation_sales_return_history", bson.M{"customer_id": 1})
	idx("product_quotation_sales_return_history", bson.M{"quotation_id": 1})
	idx("product_quotation_sales_return_history", bson.M{"quotation_code": 1})
	idx("product_quotation_sales_return_history", bson.M{"quotation_sales_return_id": 1})
	idx("product_quotation_sales_return_history", bson.M{"quotation_sales_return_code": 1})
	idx("product_quotation_sales_return_history", bson.M{"warehouse_id": 1})
	idx("product_quotation_sales_return_history", bson.M{"warehouse_code": 1})
	idx("product_quotation_sales_return_history", bson.M{"date": -1})
	idx("product_quotation_sales_return_history", bson.M{"created_at": -1})

	// product_quotation_history
	idx("product_quotation_history", bson.M{"product_id": 1})
	idx("product_quotation_history", bson.M{"customer_id": 1})
	idx("product_quotation_history", bson.M{"quotation_id": 1})
	idx("product_quotation_history", bson.M{"quotation_code": 1})
	idx("product_quotation_history", bson.M{"warehouse_id": 1})
	idx("product_quotation_history", bson.M{"warehouse_code": 1})
	idx("product_quotation_history", bson.M{"date": -1})
	idx("product_quotation_history", bson.M{"created_at": -1})
	idx("product_quotation_history", bson.M{"updated_at": -1})

	// product_purchase_return_history
	idx("product_purchase_return_history", bson.M{"product_id": 1})
	idx("product_purchase_return_history", bson.M{"vendor_id": 1})
	idx("product_purchase_return_history", bson.M{"purchase_return_id": 1})
	idx("product_purchase_return_history", bson.M{"purchase_return_code": 1})
	idx("product_purchase_return_history", bson.M{"purchase_id": 1})
	idx("product_purchase_return_history", bson.M{"purchase_code": 1})
	idx("product_purchase_return_history", bson.M{"warehouse_id": 1})
	idx("product_purchase_return_history", bson.M{"warehouse_code": 1})
	idx("product_purchase_return_history", bson.M{"date": -1})
	idx("product_purchase_return_history", bson.M{"created_at": -1})
	idx("product_purchase_return_history", bson.M{"updated_at": -1})

	// product_purchase_history
	idx("product_purchase_history", bson.M{"product_id": 1})
	idx("product_purchase_history", bson.M{"vendor_id": 1})
	idx("product_purchase_history", bson.M{"purchase_id": 1})
	idx("product_purchase_history", bson.M{"purchase_code": 1})
	idx("product_purchase_history", bson.M{"warehouse_id": 1})
	idx("product_purchase_history", bson.M{"warehouse_code": 1})
	idx("product_purchase_history", bson.M{"date": -1})
	idx("product_purchase_history", bson.M{"created_at": -1})
	idx("product_purchase_history", bson.M{"updated_at": -1})

	// product_sales_return_history
	idx("product_sales_return_history", bson.M{"product_id": 1})
	idx("product_sales_return_history", bson.M{"customer_id": 1})
	idx("product_sales_return_history", bson.M{"order_id": 1})
	idx("product_sales_return_history", bson.M{"order_code": 1})
	idx("product_sales_return_history", bson.M{"sales_return_id": 1})
	idx("product_sales_return_history", bson.M{"sales_return_code": 1})
	idx("product_sales_return_history", bson.M{"warehouse_id": 1})
	idx("product_sales_return_history", bson.M{"warehouse_code": 1})
	idx("product_sales_return_history", bson.M{"date": -1})
	idx("product_sales_return_history", bson.M{"created_at": -1})
	idx("product_sales_return_history", bson.M{"updated_at": -1})

	if len(errs) > 0 {
		return fmt.Errorf("store %s: %d index error(s): %s", store.ID.Hex(), len(errs), strings.Join(errs, " | "))
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

func (store *Store) CreateCompoundIndex(collectionName string, fields bson.D) error {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)

	mod := mongo.IndexModel{
		Keys:    fields,
		Options: options.Index(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	indexName, err := collection.Indexes().CreateOne(ctx, mod)
	if err != nil {
		log.Printf("Failed to create compound index for fields:%v, collection: %s", fields, collectionName)
		fmt.Println(err.Error())
		return err
	}

	log.Printf("Created compound index:%s for collection:%s for fields %v", indexName, collectionName, fields)
	return nil
}
