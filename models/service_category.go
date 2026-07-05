package models

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServiceCategory : ServiceCategory structure
type ServiceCategory struct {
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

func (serviceCategory *ServiceCategory) AttributesValueChangeEvent(serviceCategoryOld *ServiceCategory) error {
	return nil
}

func (serviceCategory *ServiceCategory) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(serviceCategory.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if serviceCategory.ParentID != nil && !serviceCategory.ParentID.IsZero() {
		parentCategory, err := store.FindServiceCategoryByID(serviceCategory.ParentID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		serviceCategory.ParentName = parentCategory.Name
	} else {
		serviceCategory.ParentName = ""
	}

	if serviceCategory.CreatedBy != nil {
		createdByUser, err := FindUserByID(serviceCategory.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		serviceCategory.CreatedByName = createdByUser.Name
	}

	if serviceCategory.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(serviceCategory.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		serviceCategory.UpdatedByName = updatedByUser.Name
	}

	if serviceCategory.DeletedBy != nil && !serviceCategory.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(serviceCategory.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		serviceCategory.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (store *Store) SearchServiceCategory(w http.ResponseWriter, r *http.Request) (serviceCategories []ServiceCategory, criterias SearchCriterias, err error) {

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
			return serviceCategories, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[deleted]"]
	if ok && len(keys[0]) >= 1 {
		value, err := strconv.ParseInt(keys[0], 10, 64)
		if err != nil {
			return serviceCategories, criterias, err
		}

		if value == 1 {
			criterias.SearchBy["deleted"] = bson.M{"$eq": true}
		}
	}

	ParseTextSearch(r, &criterias, "search[name]", "name")

	ParseTextSearch(r, &criterias, "search[parent_name]", "parent_name")

	if err = ParseObjectIDListFilter(r, &criterias, "search[created_by]", "created_by"); err != nil {
		return serviceCategories, criterias, err
	}

	if err = ParseExactDateFilter(r, &criterias, "search[created_at]", "created_at", timeZoneOffset); err != nil {
		return serviceCategories, criterias, err
	}

	if err = ParseDateRangeFilter(r, &criterias, "search[created_at_from]", "search[created_at_to]", "created_at", timeZoneOffset); err != nil {
		return serviceCategories, criterias, err
	}

	ParsePaginationAndSort(r, &criterias)

	offset := (criterias.Page - 1) * criterias.Size

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("service_category")

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

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return serviceCategories, criterias, errors.New("Error fetching service categories:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return serviceCategories, criterias, errors.New("Cursor error:" + err.Error())
		}
		serviceCategory := ServiceCategory{}
		err = cur.Decode(&serviceCategory)
		if err != nil {
			return serviceCategories, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			serviceCategory.CreatedByUser, _ = FindUserByID(serviceCategory.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			serviceCategory.UpdatedByUser, _ = FindUserByID(serviceCategory.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			serviceCategory.DeletedByUser, _ = FindUserByID(serviceCategory.DeletedBy, deletedByUserSelectFields)
		}

		serviceCategories = append(serviceCategories, serviceCategory)
	} //end for loop

	return serviceCategories, criterias, nil
}

func (serviceCategory *ServiceCategory) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(serviceCategory.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if serviceCategory.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsServiceCategoryExists(&serviceCategory.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Service Category:" + serviceCategory.ID.Hex()
		}

	}

	if govalidator.IsNull(serviceCategory.Name) {
		errs["name"] = "Name is required"
	}

	nameExists, err := serviceCategory.IsNameExists()
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

func (serviceCategory *ServiceCategory) IsNameExists() (exists bool, err error) {
	collection := db.GetDB("store_" + serviceCategory.StoreID.Hex()).Collection("service_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if serviceCategory.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": serviceCategory.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": serviceCategory.Name,
			"_id":  bson.M{"$ne": serviceCategory.ID},
		})
	}

	return (count > 0), err
}

func (serviceCategory *ServiceCategory) Insert() error {
	collection := db.GetDB("store_" + serviceCategory.StoreID.Hex()).Collection("service_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := serviceCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	serviceCategory.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &serviceCategory)
	if err != nil {
		return err
	}
	return nil
}

func (serviceCategory *ServiceCategory) Update() error {
	collection := db.GetDB("store_" + serviceCategory.StoreID.Hex()).Collection("service_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := serviceCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": serviceCategory.ID},
		bson.M{"$set": serviceCategory},
		updateOptions,
	)
	return err
}

func (serviceCategory *ServiceCategory) DeleteServiceCategory(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + serviceCategory.StoreID.Hex()).Collection("service_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = serviceCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	serviceCategory.Deleted = true
	serviceCategory.DeletedBy = &userID
	now := time.Now()
	serviceCategory.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": serviceCategory.ID},
		bson.M{"$set": serviceCategory},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindServiceCategoryByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (serviceCategory *ServiceCategory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("service_category")
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
		Decode(&serviceCategory)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		serviceCategory.CreatedByUser, _ = FindUserByID(serviceCategory.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		serviceCategory.UpdatedByUser, _ = FindUserByID(serviceCategory.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		serviceCategory.DeletedByUser, _ = FindUserByID(serviceCategory.DeletedBy, fields)
	}

	return serviceCategory, err
}

func (store *Store) IsServiceCategoryExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("service_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (serviceCategory *ServiceCategory) RestoreServiceCategory(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + serviceCategory.StoreID.Hex()).Collection("service_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = serviceCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	serviceCategory.Deleted = false
	serviceCategory.DeletedBy = nil
	serviceCategory.DeletedAt = nil

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": serviceCategory.ID},
		bson.M{"$set": serviceCategory},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}
