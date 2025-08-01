package models

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ProductCategory : ProductCategory structure
type ProductCategory struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	ParentID      *primitive.ObjectID `json:"parent_id" bson:"parent_id"`
	Name          string              `bson:"name,omitempty" json:"name,omitempty"`
	ParentName    string              `bson:"parent_name" json:"parent_name"`
	Deleted       bool                `bson:"deleted" json:"deleted"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by" bson:"deleted_by"`
	DeletedByUser *User               `json:"deleted_by_user"`
	DeletedAt     *time.Time          `bson:"deleted_at" json:"deleted_at"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt     *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy     *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser *User               `json:"created_by_user,omitempty"`
	UpdatedByUser *User               `json:"updated_by_user,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	StoreID       *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
}

func (productCategory *ProductCategory) AttributesValueChangeEvent(productCategoryOld *ProductCategory) error {
	store, err := FindStoreByID(productCategory.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if productCategory.Name != productCategoryOld.Name {
		err := store.UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": productCategory.ID},
			bson.M{"$pull": bson.M{
				"customer_name": productCategoryOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"product",
			bson.M{"category_id": productCategory.ID},
			bson.M{"$push": bson.M{
				"customer_name": productCategory.Name,
			}},
		)
		if err != nil {
			return nil
		}
		/*
			productCategory.SetChangeLog(
				"attribute_value_change",
				"name",
				productCategoryOld.Name,
				productCategory.Name,
			)*/
	}

	return nil
}

func (productCategory *ProductCategory) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(productCategory.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if productCategory.ParentID != nil && !productCategory.ParentID.IsZero() {
		parentCategory, err := store.FindProductCategoryByID(productCategory.ParentID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		productCategory.ParentName = parentCategory.Name
	} else {
		productCategory.ParentName = ""
	}

	if productCategory.CreatedBy != nil {
		createdByUser, err := FindUserByID(productCategory.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		productCategory.CreatedByName = createdByUser.Name
	}

	if productCategory.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(productCategory.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		productCategory.UpdatedByName = updatedByUser.Name
	}

	if productCategory.DeletedBy != nil && !productCategory.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(productCategory.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		productCategory.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (store *Store) SearchProductCategory(w http.ResponseWriter, r *http.Request) (productCategories []ProductCategory, criterias SearchCriterias, err error) {

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

	var storeID primitive.ObjectID
	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return productCategories, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return productCategories, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[parent_name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["parent_name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return productCategories, criterias, err
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
			return productCategories, criterias, err
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
			return productCategories, criterias, err
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
			return productCategories, criterias, err
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
	keys, ok = r.URL.Query()["sort"]
	if ok && len(keys[0]) >= 1 {
		criterias.SortBy = GetSortByFields(keys[0])
	}

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_category")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields

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
		return productCategories, criterias, errors.New("Error fetching product categories:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return productCategories, criterias, errors.New("Cursor error:" + err.Error())
		}
		productCategory := ProductCategory{}
		err = cur.Decode(&productCategory)
		if err != nil {
			return productCategories, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			productCategory.CreatedByUser, _ = FindUserByID(productCategory.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			productCategory.UpdatedByUser, _ = FindUserByID(productCategory.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			productCategory.DeletedByUser, _ = FindUserByID(productCategory.DeletedBy, deletedByUserSelectFields)
		}

		productCategories = append(productCategories, productCategory)
	} //end for loop

	return productCategories, criterias, nil
}

func (productCategory *ProductCategory) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(productCategory.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if productCategory.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsProductCategoryExists(&productCategory.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Product Category:" + productCategory.ID.Hex()
		}

	}

	if govalidator.IsNull(productCategory.Name) {
		errs["name"] = "Name is required"
	}

	nameExists, err := productCategory.IsNameExists()
	if err != nil {
		errs["name"] = err.Error()
	}

	if nameExists {
		errs["name"] = "Name is Already in use"
	}

	if nameExists {
		w.WriteHeader(http.StatusConflict)
	} else if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (productCategory *ProductCategory) IsNameExists() (exists bool, err error) {
	collection := db.GetDB("store_" + productCategory.StoreID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if productCategory.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": productCategory.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": productCategory.Name,
			"_id":  bson.M{"$ne": productCategory.ID},
		})
	}

	return (count > 0), err
}

func (productCategory *ProductCategory) Insert() error {
	collection := db.GetDB("store_" + productCategory.StoreID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := productCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	productCategory.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &productCategory)
	if err != nil {
		return err
	}
	return nil
}

func (productCategory *ProductCategory) Update() error {
	collection := db.GetDB("store_" + productCategory.StoreID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := productCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": productCategory.ID},
		bson.M{"$set": productCategory},
		updateOptions,
	)
	return err
}

func (productCategory *ProductCategory) DeleteProductCategory(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + productCategory.StoreID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = productCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	productCategory.Deleted = true
	productCategory.DeletedBy = &userID
	now := time.Now()
	productCategory.DeletedAt = &now

	//productCategory.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": productCategory.ID},
		bson.M{"$set": productCategory},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindProductCategoryByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (productCategory *ProductCategory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"_id":      ID,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&productCategory)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		productCategory.CreatedByUser, _ = FindUserByID(productCategory.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		productCategory.UpdatedByUser, _ = FindUserByID(productCategory.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		productCategory.DeletedByUser, _ = FindUserByID(productCategory.DeletedBy, fields)
	}

	return productCategory, err
}

func (store *Store) FindProductCategoryByName(
	Name string,
	selectFields map[string]interface{},
) (productCategory *ProductCategory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{
			"name":     Name,
			"store_id": store.ID,
		}, findOneOptions).
		Decode(&productCategory)
	if err != nil {
		return nil, err
	}

	return productCategory, err
}

func (store *Store) IsProductCategoryExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (store *Store) ProcessProductCategories() error {
	log.Printf("Processing product categories")
	totalCount, err := store.GetTotalCount(bson.M{}, "product_category")
	if err != nil {
		return err
	}
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_category")
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

	bar := progressbar.Default(totalCount)
	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		category := ProductCategory{}
		err = cur.Decode(&category)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		if category.CreatedByName == "Shanoob" {
			store, err := FindStoreByCode("GUOCJ", bson.M{})
			if err != nil {
				return errors.New("Error finding store: " + err.Error())
			}
			category.StoreID = &store.ID
		} else {
			store, err := FindStoreByCode("GUOJ", bson.M{})
			if err != nil {
				return errors.New("Error finding store: " + err.Error())
			}
			category.StoreID = &store.ID
		}

		err = category.Update()
		if err != nil {
			return err
		}

		bar.Add(1)
	}

	log.Print("DONE!")
	return nil
}

func (productCategory *ProductCategory) RestoreProductCategory(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + productCategory.StoreID.Hex()).Collection("product_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = productCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	productCategory.Deleted = false
	productCategory.DeletedBy = nil
	productCategory.DeletedAt = nil

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": productCategory.ID},
		bson.M{"$set": productCategory},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
