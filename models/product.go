package models

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
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

type ProductUnitPrice struct {
	StoreID        primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	WholeSalePrice float32            `bson:"wholesale_unit_price,omitempty" json:"wholesale_unit_price,omitempty"`
	RetailPrice    float32            `bson:"retail_unit_price,omitempty" json:"retail_unit_price,omitempty"`
}

type ProductStock struct {
	StoreID primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	Stock   int                `bson:"stock,omitempty" json:"stock"`
}

//Product : Product structure
type Product struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name         string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode     string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	CategoryID   primitive.ObjectID `json:"category_id,omitempty" bson:"category_id,omitempty"`
	UnitPrices   []ProductUnitPrice `bson:"unit_price,omitempty" json:"unit_price,omitempty"`
	Stock        []ProductStock     `bson:"stock,omitempty" json:"stock,omitempty"`
	Images       []string           `bson:"images,omitempty" json:"images,omitempty"`
	Deleted      bool               `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy    primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt    time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt    time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt    time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy    primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy    primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

func SearchProduct(w http.ResponseWriter, r *http.Request) (products []Product, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})

	keys, ok := r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"item_code": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	keys, ok = r.URL.Query()["search[item_code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["item_code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

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
		products = append(products, product)
	} //end for loop

	return products, criterias, nil

}

func (product *Product) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if product.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsProductExists(product.ID)
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
			return errs
		}
		if exists {
			errs["item_code"] = "Item Code Already Exists"
			return errs
		}
	}

	for _, price := range product.UnitPrices {
		if price.StoreID.IsZero() {
			errs["id"] = "store_id is required for unit price"
			return errs
		}
		exists, err := IsStoreExists(price.StoreID)
		if err != nil {
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store_id:" + price.StoreID.Hex() + " in unit_price"
			return errs
		}
	}

	for _, stock := range product.Stock {
		if stock.StoreID.IsZero() {
			errs["id"] = "store_id is required for stock"
			return errs
		}
		exists, err := IsStoreExists(stock.StoreID)
		if err != nil {
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store_id:" + stock.StoreID.Hex() + " in stock"
			return errs
		}
	}

	if product.CategoryID.IsZero() {
		errs["category_id"] = "Category is required"
	} else {
		exists, err := IsProductCategoryExists(product.CategoryID)
		if err != nil {
			errs["category_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["category_id"] = "Invalid category:" + product.CategoryID.Hex()
			return errs
		}
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (product *Product) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	product.ID = primitive.NewObjectID()
	if len(product.ItemCode) == 0 {
		for true {
			product.ItemCode = strings.ToUpper(GenerateItemCode(7))
			exists, err := product.IsItemCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}

	if len(product.Images) > 0 {
		err := product.SaveImages()
		if err != nil {
			return err
		}
	}

	_, err := collection.InsertOne(ctx, &product)
	if err != nil {
		return err
	}
	return nil
}

func (product *Product) SaveImages() error {

	for k, image := range product.Images {
		content, err := base64.StdEncoding.DecodeString(image)
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
		product.Images[k] = filename
	}

	return nil
}

func GenerateFileName(prefix, suffix string) string {
	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	return prefix + hex.EncodeToString(randBytes) + suffix
}

func GenerateItemCode(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (product *Product) Update() (*Product, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": product.ID},
		bson.M{"$set": product},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return product, nil
	}
	return nil, nil
}

func (product *Product) DeleteProduct(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	product.Deleted = true
	product.DeletedBy = userID
	product.DeletedAt = time.Now().Local()

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

func FindProductByID(ID primitive.ObjectID) (product *Product, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}).
		Decode(&product)
	if err != nil {
		return nil, err
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

	return (count == 1), err
}

func IsProductExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
