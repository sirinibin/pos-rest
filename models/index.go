package models

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
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
	/*
		textFields := bson.D{
			{"name", "text"},
			{"part_number", "text"},
			{"name_in_arabic", "text"},
		}
		err := store.CreateTextIndex("product", textFields, "name_text_part_number_text_name_in_arabic_text")
		if err != nil {
			return err
		}*/

	fields := bson.M{"ean_12": 1}
	err := store.CreateIndex("product", fields, true, false, "")
	if err != nil {
		return err
	}

	/*
		fields = bson.M{"part_number": 1}
		err = store.CreateIndex("product", fields, true, false, "")
		if err != nil {
			return err
		}*/

	/*
		fields = bson.M{"name": "text"}
		err = store.CreateIndex("product", fields, false, true, "")
		if err != nil {
			return err
		}
	*/

	fields = bson.M{"category_id": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}
	fields = bson.M{"created_by": 1}
	err = store.CreateIndex("product", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_by": 1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

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

	fields = bson.M{"code": 1}
	err = store.CreateIndex("order", fields, true, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"payment_status": 1}
	err = store.CreateIndex("order", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("expense", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"vendor_invoice_no": "text"}
	err = store.CreateIndex("purchase", fields, false, true, "")
	if err != nil {
		return err
	}
	fields = bson.M{"vendor_id": 1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_by": 1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"created_at": -1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("purchase", fields, false, false, "")
	if err != nil {
		return err
	}

	//Sales Return indexes
	fields = bson.M{"date": -1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("salesreturn", fields, true, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"order_code": 1}
	err = store.CreateIndex("salesreturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("purchase", fields, true, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("purchasereturn", fields, true, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"date": -1}
	err = store.CreateIndex("purchasereturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"purchase_code": 1}
	err = store.CreateIndex("purchasereturn", fields, false, false, "")
	if err != nil {
		return err
	}

	fields = bson.M{"code": 1}
	err = store.CreateIndex("quotation", fields, true, false, "")
	if err != nil {
		return err
	}

	return nil
}

/*
name_text_part_number_text_name_in_arabic_text

	bson.D{
	            {"name", "text"},
	            {"part_number", "text"},
	            {"name_in_arabic", "text"},
	        }
*/
func (store *Store) CreateTextIndex(collectionName string, fields bson.D, indexName string) error {
	log.Print("inside CreateTextIndex")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexModel := mongo.IndexModel{
		Keys:    fields,
		Options: options.Index().SetName(indexName).SetUnique(false),
	}

	createdIndexName, err := collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		log.Printf("Failed to create text index: %v", err)
		return err
	}

	fmt.Println("Created text index:", createdIndexName)
	return nil
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
		log.Printf("Failed to create Index for field:%v", fields)
		fmt.Println(err.Error())
		return err
	}

	log.Printf("Created Index:%s for collection:%s for fields %v", indexName, collectionName, fields)

	// 6. All went well, we return true
	return nil
}

func (store *Store) RemoveAllIndexes() {
	log.Print("Removing all indexes")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("order")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("salesreturn")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("purchase")
	collection.Indexes().DropAll(context.Background())

	collection = db.GetDB("store_" + store.ID.Hex()).Collection("purchasereturn")
	collection.Indexes().DropAll(context.Background())
}
