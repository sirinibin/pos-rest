package models

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
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

//Product : Product structure
type Product struct {
	ID            primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string                `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic  string                `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode      string                `bson:"item_code,omitempty" json:"item_code,omitempty"`
	BarCode       string                `bson:"bar_code,omitempty" json:"bar_code,omitempty"`
	Ean12         string                `bson:"ean_12,omitempty" json:"ean_12,omitempty"`
	SearchLabel   string                `json:"search_label"`
	Rack          string                `bson:"rack,omitempty" json:"rack"`
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
	BarcodeBase64 string                `json:"barcode_base64"`
}

func (product *Product) getRetailUnitPriceByStoreID(storeID primitive.ObjectID) (retailUnitPrice float64, err error) {
	for _, unitPrice := range product.UnitPrices {
		if unitPrice.StoreID == storeID {
			return unitPrice.RetailUnitPrice, nil
		}
	}
	return retailUnitPrice, err
}

func (product *Product) getPurchaseUnitPriceSecretByStoreID(storeID primitive.ObjectID) (secret string, err error) {
	for _, unitPrice := range product.UnitPrices {
		if unitPrice.StoreID == storeID {
			return unitPrice.PurchaseUnitPriceSecret, nil
		}
	}
	return secret, err
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
		if len(product.UnitPrices) > 0 {
			for i, unitPrice := range product.UnitPrices {
				if unitPrice.StoreID == store.ID {
					if len(unitPrice.PurchaseUnitPriceSecret) == 0 {
						product.UnitPrices[i].PurchaseUnitPriceSecret = GenerateSecretCode(int(product.UnitPrices[i].PurchaseUnitPrice))
						err = product.Update()
						if err != nil {
							return products, err
						}
					}
					purchaseUnitPriceSecret = product.UnitPrices[i].PurchaseUnitPriceSecret

					price := float64(unitPrice.RetailUnitPrice)
					vatPrice := (float64(float64(unitPrice.RetailUnitPrice) * float64(store.VatPercent/float64(100))))
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

	keys, ok := r.URL.Query()["search[name]"]
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

	keys, ok = r.URL.Query()["search[ean12]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["ean12"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

		product.SearchLabel = product.Name + "(Part#" + product.PartNumber + " Arabic:" + product.NameInArabic + ")"

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

	for i, price := range product.UnitPrices {
		if price.StoreID.IsZero() {
			errs["store_id_"+strconv.Itoa(i)] = "store_id is required for unit price"
			return errs
		}
		exists, err := IsStoreExists(&price.StoreID)
		if err != nil {
			errs["store_id_"+strconv.Itoa(i)] = err.Error()
		}

		if !exists {
			errs["store_id"+strconv.Itoa(i)] = "Invalid store_id:" + price.StoreID.Hex() + " in unit_price"
		}
		product.UnitPrices[i].PurchaseUnitPriceSecret = GenerateSecretCode(int(product.UnitPrices[i].PurchaseUnitPrice))
	}

	for i, stock := range product.Stock {
		if stock.StoreID.IsZero() {
			errs["store_id_"+strconv.Itoa(i)] = "store_id is required for stock"
		}
		exists, err := IsStoreExists(&stock.StoreID)
		if err != nil {
			errs["store_id_"] = err.Error()
		}

		if !exists {
			errs["store_id_"+strconv.Itoa(i)] = "Invalid store_id:" + stock.StoreID.Hex() + " in stock"
		}
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

func IsStringBase64(content string) (bool, error) {
	return regexp.MatchString(`^([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{2}==)?$`, content)
}

func GenerateItemCode(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func GeneratePartNumber(n int) string {
	letterRunes := []rune("1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (product *Product) GenerateBarCode(startFrom int) (string, error) {
	count, err := GetTotalCount(bson.M{}, "product")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return strconv.Itoa(code + 1), nil
}

func (product *Product) Insert() (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	product.ID = primitive.NewObjectID()
	if len(product.ItemCode) == 0 {
		for {
			exists, err := product.IsItemCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			product.ItemCode = strings.ToUpper(GenerateItemCode(7))
		}
	}
	if len(product.PartNumber) == 0 {
		for {
			product.PartNumber = strings.ToUpper(GeneratePartNumber(7))
			exists, err := product.IsPartNumberExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}

	if len(product.Ean12) == 0 {
		barcodeStartAt := 100000000000
		for {
			barcode, err := product.GenerateBarCode(barcodeStartAt)
			if err != nil {
				return err
			}
			product.Ean12 = barcode
			exists, err := product.IsBarCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			barcodeStartAt++
		}
	}

	if len(product.ImagesContent) > 0 {
		err := product.SaveImages()
		if err != nil {
			return err
		}
	}

	err = product.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

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

func (product *Product) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

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

	if len(product.Ean12) == 0 {
		barcodeStartAt := 100000000000
		for {
			barcode, err := product.GenerateBarCode(barcodeStartAt)
			if err != nil {
				return err
			}
			product.Ean12 = strings.ToUpper(barcode)
			exists, err := product.IsBarCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			barcodeStartAt++
		}
	}

	if len(product.PartNumber) == 0 {
		for {
			product.PartNumber = strings.ToUpper(GeneratePartNumber(7))
			exists, err := product.IsPartNumberExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}

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
		{"barcode": barCode},
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

func ProcessProducts() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("product")
	ctx := context.Background()
	findOptions := options.Find()

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
		model := Product{}
		err = cur.Decode(&model)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = model.Update()
		if err != nil {
			return err
		}

	}

	return nil
}
