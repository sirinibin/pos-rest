package models

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
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
	StoreID            primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName          string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic  string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	PurchaseUnitPrice  float32            `bson:"purchase_unit_price,omitempty" json:"purchase_unit_price,omitempty"`
	WholesaleUnitPrice float32            `bson:"wholesale_unit_price,omitempty" json:"wholesale_unit_price,omitempty"`
	RetailUnitPrice    float32            `bson:"retail_unit_price,omitempty" json:"retail_unit_price,omitempty"`
}

type ProductStock struct {
	StoreID           primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	StoreName         string             `bson:"store_name,omitempty" json:"store_name,omitempty"`
	StoreNameInArabic string             `bson:"store_name_in_arabic,omitempty" json:"store_name_in_arabic,omitempty"`
	Stock             float32            `bson:"stock,omitempty" json:"stock"`
}

//Product : Product structure
type Product struct {
	ID            primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string                `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic  string                `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode      string                `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber    string                `bson:"part_number,omitempty" json:"part_number,omitempty"`
	CategoryID    []*primitive.ObjectID `json:"category_id" bson:"category_id"`
	Category      []*ProductCategory    `json:"category,omitempty"`
	UnitPrices    []ProductUnitPrice    `bson:"unit_prices,omitempty" json:"unit_prices,omitempty"`
	Stock         []ProductStock        `bson:"stock,omitempty" json:"stock,omitempty"`
	Unit          string                `bson:"unit,omitempty" json:"unit,omitempty"`
	Images        []string              `bson:"images,omitempty" json:"images,omitempty"`
	ImagesContent []string              `json:"images_content,omitempty"`
	Deleted       bool                  `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy     *primitive.ObjectID   `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser *User                 `json:"deleted_by_user,omitempty"`
	DeletedAt     *time.Time            `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt     *time.Time            `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID   `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID   `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser *User                 `json:"created_by_user,omitempty"`
	UpdatedByUser *User                 `json:"updated_by_user,omitempty"`
	BrandName     string                `json:"brand_name,omitempty" bson:"brand_name,omitempty"`
	CategoryName  []string              `json:"category_name" bson:"category_name"`
	CreatedByName string                `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string                `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName string                `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog     []ChangeLog           `json:"change_log,omitempty" bson:"change_log,omitempty"`
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
		description = "Stock reduced from " + fmt.Sprintf("%f", oldValue.(float32)) + " to " + fmt.Sprintf("%f", newValue.(float32)) + " by " + UserObject.Name
	} else if event == "add_stock" && name != nil {
		description = "Stock raised from " + strconv.Itoa(oldValue.(int)) + " to " + fmt.Sprintf("%f", newValue.(float32)) + " by " + UserObject.Name
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

	for i, unitPrice := range product.UnitPrices {
		store, err := FindStoreByID(&unitPrice.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding store:" + unitPrice.StoreID.Hex() + ",error:" + err.Error())
		}

		product.UnitPrices[i].StoreName = store.Name
		product.UnitPrices[i].StoreNameInArabic = store.NameInArabic
	}

	for i, stock := range product.Stock {
		store, err := FindStoreByID(&stock.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return errors.New("Error Finding store:" + stock.StoreID.Hex() + ",error:" + err.Error())
		}

		product.Stock[i].StoreName = store.Name
		product.Stock[i].StoreNameInArabic = store.NameInArabic
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

	return nil
}

func SearchProduct(w http.ResponseWriter, r *http.Request) (products []Product, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

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
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return products, criterias, err
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
		exists, err := IsStoreExists(&price.StoreID)
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
		exists, err := IsStoreExists(&stock.StoreID)
		if err != nil {
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store_id:" + stock.StoreID.Hex() + " in stock"
			return errs
		}
	}

	if len(product.CategoryID) == 0 {
		errs["category_id"] = "Atleast 1 category is required"
	} else {
		for _, categoryID := range product.CategoryID {
			exists, err := IsProductCategoryExists(categoryID)
			if err != nil {
				errs["category_id"] = err.Error()
				return errs
			}

			if !exists {
				errs["category_id"] = "Invalid category:" + categoryID.Hex()
				return errs
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
			return errs
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

func IsStringBase64(content string) (bool, error) {
	return regexp.MatchString(`^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{2}==)?$`, content)
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

	if len(product.ImagesContent) > 0 {
		err := product.SaveImages()
		if err != nil {
			return err
		}
	}

	err := product.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	product.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &product)
	if err != nil {
		return err
	}
	return nil
}

func (product *Product) SaveImages() error {

	oldImagesCount := len(product.Images)

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

	product.SetChangeLog(
		"add_image",
		"images",
		oldImagesCount,
		len(product.Images),
	)

	product.ImagesContent = []string{}

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

func (product *Product) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	log.Print("product.ID1:" + product.ID.Hex())

	if len(product.ImagesContent) > 0 {
		err := product.SaveImages()
		if err != nil {
			return err
		}
	}

	log.Print("product.ID2:" + product.ID.Hex())

	err := product.UpdateForeignLabelFields()
	if err != nil {
		return err
	}
	log.Print("product.ID3:" + product.ID.Hex())

	product.SetChangeLog("update", nil, nil, nil)

	log.Print("product.ID:" + product.ID.Hex())
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": product.ID},
		bson.M{"$set": product},
		updateOptions,
	)
	return err
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

	return (count == 1), err
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
