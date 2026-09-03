package models

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// VendorCategory : VendorCategory structure
type VendorCategory struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string              `bson:"name,omitempty" json:"name,omitempty"`
	Deleted       bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy     *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser *User               `json:"deleted_by_user,omitempty"`
	DeletedAt     *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
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

func (vendorCategory *VendorCategory) UpdateForeignLabelFields() error {
	if vendorCategory.CreatedBy != nil {
		createdByUser, err := FindUserByID(vendorCategory.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		vendorCategory.CreatedByName = createdByUser.Name
	}

	if vendorCategory.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(vendorCategory.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		vendorCategory.UpdatedByName = updatedByUser.Name
	}

	if vendorCategory.DeletedBy != nil && !vendorCategory.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(vendorCategory.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		vendorCategory.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (store *Store) SearchVendorCategory(w http.ResponseWriter, r *http.Request) (vendorCategories []VendorCategory, criterias SearchCriterias, err error) {

	criterias = InitSearchCriterias()
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	timeZoneOffset := CountryTimezoneOffset(store.CountryCode)
	var keys []string
	var ok bool

	var storeID primitive.ObjectID
	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return vendorCategories, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	ParseTextSearch(r, &criterias, "search[name]", "name")

	if err = ParseObjectIDListFilter(r, &criterias, "search[created_by]", "created_by"); err != nil {
		return vendorCategories, criterias, err
	}

	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return vendorCategories, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return vendorCategories, criterias, err
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor_category")
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

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			createdByUserSelectFields = ParseRelationalSelectString(keys[0], "created_by_user")
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			updatedByUserSelectFields = ParseRelationalSelectString(keys[0], "updated_by_user")
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			deletedByUserSelectFields = ParseRelationalSelectString(keys[0], "deleted_by_user")
		}
	}

	if criterias.Select != nil {
		findOptions.SetProjection(criterias.Select)
	}

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return vendorCategories, criterias, errors.New("Error fetching vendor categories:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return vendorCategories, criterias, errors.New("Cursor error:" + err.Error())
		}
		vendorCategory := VendorCategory{}
		err = cur.Decode(&vendorCategory)
		if err != nil {
			return vendorCategories, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			vendorCategory.CreatedByUser, _ = FindUserByID(vendorCategory.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			vendorCategory.UpdatedByUser, _ = FindUserByID(vendorCategory.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			vendorCategory.DeletedByUser, _ = FindUserByID(vendorCategory.DeletedBy, deletedByUserSelectFields)
		}

		vendorCategories = append(vendorCategories, vendorCategory)
	}

	return vendorCategories, criterias, nil
}

func (vendorCategory *VendorCategory) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(vendorCategory.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if vendorCategory.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsVendorCategoryExists(&vendorCategory.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}
		if !exists {
			errs["id"] = "Invalid Vendor Category:" + vendorCategory.ID.Hex()
		}
	}

	if govalidator.IsNull(vendorCategory.Name) {
		errs["name"] = "Name is required"
	}

	nameExists, err := vendorCategory.IsNameExists()
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

func (vendorCategory *VendorCategory) IsNameExists() (exists bool, err error) {
	collection := db.GetDB("store_" + vendorCategory.StoreID.Hex()).Collection("vendor_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if vendorCategory.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": vendorCategory.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": vendorCategory.Name,
			"_id":  bson.M{"$ne": vendorCategory.ID},
		})
	}

	return (count > 0), err
}

func (vendorCategory *VendorCategory) Insert() error {
	collection := db.GetDB("store_" + vendorCategory.StoreID.Hex()).Collection("vendor_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := vendorCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	vendorCategory.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &vendorCategory)
	if err != nil {
		return err
	}
	return nil
}

func (vendorCategory *VendorCategory) Update() error {
	collection := db.GetDB("store_" + vendorCategory.StoreID.Hex()).Collection("vendor_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := vendorCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": vendorCategory.ID},
		bson.M{"$set": vendorCategory},
		updateOptions,
	)
	return err
}

func (vendorCategory *VendorCategory) DeleteVendorCategory(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + vendorCategory.StoreID.Hex()).Collection("vendor_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = vendorCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	vendorCategory.Deleted = true
	vendorCategory.DeletedBy = &userID
	now := time.Now()
	vendorCategory.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": vendorCategory.ID},
		bson.M{"$set": vendorCategory},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindVendorCategoryByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (vendorCategory *VendorCategory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor_category")
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
		Decode(&vendorCategory)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		vendorCategory.CreatedByUser, _ = FindUserByID(vendorCategory.CreatedBy, fields)
	}
	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		vendorCategory.UpdatedByUser, _ = FindUserByID(vendorCategory.UpdatedBy, fields)
	}
	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		vendorCategory.DeletedByUser, _ = FindUserByID(vendorCategory.DeletedBy, fields)
	}

	return vendorCategory, err
}

func (store *Store) IsVendorCategoryExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}
