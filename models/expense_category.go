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
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ExpenseCategory : ExpenseCategory structure
type ExpenseCategory struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	ParentID      *primitive.ObjectID `json:"parent_id" bson:"parent_id"`
	Name          string              `bson:"name,omitempty" json:"name,omitempty"`
	ParentName    string              `bson:"parent_name" json:"parent_name"`
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

func (expenseCategory *ExpenseCategory) AttributesValueChangeEvent(expenseCategoryOld *ExpenseCategory) error {
	store, err := FindStoreByID(expenseCategory.StoreID, bson.M{})
	if err != nil {
		return nil
	}

	if expenseCategory.Name != expenseCategoryOld.Name {
		err := store.UpdateManyByCollectionName(
			"expense",
			bson.M{"category_id": expenseCategory.ID},
			bson.M{"$pull": bson.M{
				"customer_name": expenseCategoryOld.Name,
			}},
		)
		if err != nil {
			return nil
		}

		err = store.UpdateManyByCollectionName(
			"expense",
			bson.M{"category_id": expenseCategory.ID},
			bson.M{"$push": bson.M{
				"customer_name": expenseCategory.Name,
			}},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}

func (expenseCategory *ExpenseCategory) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(expenseCategory.StoreID, bson.M{})
	if err != nil {
		return nil
	}

	if expenseCategory.ParentID != nil && !expenseCategory.ParentID.IsZero() {
		parentCategory, err := store.FindExpenseCategoryByID(expenseCategory.ParentID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		expenseCategory.ParentName = parentCategory.Name
	} else {
		expenseCategory.ParentName = ""
	}

	if expenseCategory.CreatedBy != nil {
		createdByUser, err := FindUserByID(expenseCategory.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		expenseCategory.CreatedByName = createdByUser.Name
	}

	if expenseCategory.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(expenseCategory.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		expenseCategory.UpdatedByName = updatedByUser.Name
	}

	if expenseCategory.DeletedBy != nil && !expenseCategory.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(expenseCategory.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		expenseCategory.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (store *Store) SearchExpenseCategory(w http.ResponseWriter, r *http.Request) (expenseCategories []ExpenseCategory, criterias SearchCriterias, err error) {

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
			return expenseCategories, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
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
				return expenseCategories, criterias, err
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
			return expenseCategories, criterias, err
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
			return expenseCategories, criterias, err
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
			return expenseCategories, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense_category")
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
		return expenseCategories, criterias, errors.New("Error fetching expense categories:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return expenseCategories, criterias, errors.New("Cursor error:" + err.Error())
		}
		expenseCategory := ExpenseCategory{}
		err = cur.Decode(&expenseCategory)
		if err != nil {
			return expenseCategories, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			expenseCategory.CreatedByUser, _ = FindUserByID(expenseCategory.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			expenseCategory.UpdatedByUser, _ = FindUserByID(expenseCategory.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			expenseCategory.DeletedByUser, _ = FindUserByID(expenseCategory.DeletedBy, deletedByUserSelectFields)
		}

		expenseCategories = append(expenseCategories, expenseCategory)
	} //end for loop

	return expenseCategories, criterias, nil
}

func (expenseCategory *ExpenseCategory) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(expenseCategory.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if expenseCategory.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsExpenseCategoryExists(&expenseCategory.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Expense Category:" + expenseCategory.ID.Hex()
		}

	}

	if govalidator.IsNull(expenseCategory.Name) {
		errs["name"] = "Name is required"
	}

	nameExists, err := expenseCategory.IsNameExists()
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

func (expenseCategory *ExpenseCategory) IsNameExists() (exists bool, err error) {
	collection := db.GetDB("store_" + expenseCategory.StoreID.Hex()).Collection("expense_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if expenseCategory.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": expenseCategory.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": expenseCategory.Name,
			"_id":  bson.M{"$ne": expenseCategory.ID},
		})
	}

	return (count > 0), err
}

func (expenseCategory *ExpenseCategory) Insert() error {
	collection := db.GetDB("store_" + expenseCategory.StoreID.Hex()).Collection("expense_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := expenseCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	expenseCategory.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &expenseCategory)
	if err != nil {
		return err
	}
	return nil
}

func (expenseCategory *ExpenseCategory) Update() error {
	collection := db.GetDB("store_" + expenseCategory.StoreID.Hex()).Collection("expense_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err := expenseCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": expenseCategory.ID},
		bson.M{"$set": expenseCategory},
		updateOptions,
	)
	return err
}

func (expenseCategory *ExpenseCategory) DeleteExpenseCategory(tokenClaims TokenClaims) (err error) {
	collection := db.GetDB("store_" + expenseCategory.StoreID.Hex()).Collection("expense_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(false)
	defer cancel()

	err = expenseCategory.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	expenseCategory.Deleted = true
	expenseCategory.DeletedBy = &userID
	now := time.Now()
	expenseCategory.DeletedAt = &now

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": expenseCategory.ID},
		bson.M{"$set": expenseCategory},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (store *Store) FindExpenseCategoryByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (expenseCategory *ExpenseCategory, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense_category")
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
		Decode(&expenseCategory)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		expenseCategory.CreatedByUser, _ = FindUserByID(expenseCategory.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		expenseCategory.UpdatedByUser, _ = FindUserByID(expenseCategory.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		expenseCategory.DeletedByUser, _ = FindUserByID(expenseCategory.DeletedBy, fields)
	}

	return expenseCategory, err
}

func (store *Store) IsExpenseCategoryExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense_category")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (store *Store) ProcessExpenseCategories() error {
	log.Printf("Processing expense categories")
	totalCount, err := store.GetTotalCount(bson.M{}, "expense_category")
	if err != nil {
		return err
	}
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense_category")
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
		category := ExpenseCategory{}
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
