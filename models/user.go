package models

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jameskeane/bcrypt"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
) //import "encoding/json"

type User struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string              `bson:"name,omitempty" json:"name,omitempty"`
	Email         string              `bson:"email,omitempty" json:"email,omitempty"`
	Mob           string              `bson:"mob,omitempty" json:"mob,omitempty"`
	Password      string              `bson:"password,omitempty" json:"password,omitempty"`
	Photo         string              `bson:"photo,omitempty" json:"photo,omitempty"`
	PhotoContent  string              `json:"photo_content,omitempty"`
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
	ChangeLog     []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func (user *User) SetChangeLog(
	event string,
	name, oldValue, newValue interface{},
) {
	now := time.Now().Local()
	description := ""
	if event == "create" {
		description = "Created by" + UserObject.Name
	} else if event == "update" {
		description = "Updated by" + UserObject.Name
	} else if event == "delete" {
		description = "Deleted by" + UserObject.Name
	} else if event == "view" {
		description = "Viewed by" + UserObject.Name
	} else if event == "attribute_value_change" && name != nil {
		description = name.(string) + " changed from " + oldValue.(string) + " to " + newValue.(string) + " by " + UserObject.Name
	}

	changeLog := ChangeLog{
		Event:       event,
		Description: description,
		CreatedAt:   &now,
	}

	if !UserObject.ID.IsZero() {
		changeLog.CreatedBy = &UserObject.ID
		changeLog.CreatedByName = UserObject.Name
	}

	user.ChangeLog = append(
		user.ChangeLog,
		changeLog,
	)
}

func (user *User) AttributesValueChangeEvent(userOld *User) error {

	if user.Name != userOld.Name {

		err := UpdateManyByCollectionName(
			"quotation",
			bson.M{"delivered_by": user.ID},
			bson.M{"delivered_by_name": user.Name},
		)
		if err != nil {
			return nil
		}

		err = UpdateManyByCollectionName(
			"purchase",
			bson.M{"order_placed_by": user.ID},
			bson.M{"order_placed_by_name": user.Name},
		)
		if err != nil {
			return nil
		}

		usedInCollections := []string{
			"order",
			"customer",
			"purchase",
			"product_category",
			"product",
			"quotation",
			"signature",
			"store",
			"vendor",
		}

		for _, collectionName := range usedInCollections {

			err := UpdateManyByCollectionName(
				collectionName,
				bson.M{"created_by": user.ID},
				bson.M{"created_by_name": user.Name},
			)
			if err != nil {
				return nil
			}

			err = UpdateManyByCollectionName(
				collectionName,
				bson.M{"updated_by": user.ID},
				bson.M{"updated_by_name": user.Name},
			)
			if err != nil {
				return nil
			}

			err = UpdateManyByCollectionName(
				collectionName,
				bson.M{"deleted_by": user.ID},
				bson.M{"deleted_by_name": user.Name},
			)
			if err != nil {
				return nil
			}

			err = UpdateManyByCollectionName(
				collectionName,
				bson.M{"change_logs.created_by": user.ID},
				bson.M{"change_logs.$.created_by_name": user.Name},
			)
			if err != nil {
				return nil
			}

		}

	}

	return nil
}

func (user *User) UpdateForeignLabelFields() error {

	if user.CreatedBy != nil {
		createdByUser, err := FindUserByID(user.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		user.CreatedByName = createdByUser.Name
	}

	if user.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(user.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		user.UpdatedByName = updatedByUser.Name
	}

	if user.DeletedBy != nil {
		deletedByUser, err := FindUserByID(user.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		user.DeletedByName = deletedByUser.Name
	}

	return nil
}

func FindUserByEmail(email string) (user *User, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"email": email}).
		Decode(&user)
	if err != nil {
		return nil, err
	}
	return user, err
}

func (user *User) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if user.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsUserExists(&user.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid User:" + user.ID.Hex()
		}

	}

	if govalidator.IsNull(user.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(user.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(user.Mob) {
		errs["mob"] = "Mob is required"
	}

	if govalidator.IsNull(user.Password) {
		errs["password"] = "Password is required"
	}

	if !govalidator.IsNull(user.PhotoContent) {
		valid, err := IsStringBase64(user.PhotoContent)
		if err != nil {
			errs["photo_content"] = err.Error()
		}

		if !valid {
			errs["photo_content"] = "Invalid base64 string"
		}
	}

	emailExists, err := user.IsEmailExists()
	if err != nil {
		errs["email"] = err.Error()
	}

	if emailExists {
		errs["email"] = "E-mail is Already in use"
	}

	if emailExists {
		w.WriteHeader(http.StatusConflict)
	} else if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func SearchUser(w http.ResponseWriter, r *http.Request) (users []User, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[mob]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["mob"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
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

	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

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
		return users, criterias, errors.New("Error fetching Users:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return users, criterias, errors.New("Cursor error:" + err.Error())
		}
		user := User{}
		err = cur.Decode(&user)
		if err != nil {
			return users, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			user.CreatedByUser, _ = FindUserByID(user.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			user.UpdatedByUser, _ = FindUserByID(user.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			user.DeletedByUser, _ = FindUserByID(user.DeletedBy, deletedByUserSelectFields)
		}

		users = append(users, user)
	} //end for loop

	return users, criterias, nil

}

func (user *User) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := user.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	user.ID = primitive.NewObjectID()
	// Insert new record
	user.Password = HashPassword(user.Password)

	if !govalidator.IsNull(user.PhotoContent) {
		err := user.SavePhoto()
		if err != nil {
			return err
		}
	}

	user.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &user)
	if err != nil {
		return err
	}
	return nil
}

func (user *User) SavePhoto() error {
	content, err := base64.StdEncoding.DecodeString(user.PhotoContent)
	if err != nil {
		return err
	}

	extension, err := GetFileExtensionFromBase64(content)
	if err != nil {
		return err
	}

	filename := "images/users/user_" + user.ID.Hex() + extension
	err = SaveBase64File(filename, content)
	if err != nil {
		return err
	}
	user.Photo = "/" + filename
	user.PhotoContent = ""
	return nil
}

func (user *User) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := user.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	now := time.Now().Local()
	user.UpdatedAt = &now

	if !govalidator.IsNull(user.Password) {
		user.Password = HashPassword(user.Password)
	}

	if !govalidator.IsNull(user.PhotoContent) {
		err := user.SavePhoto()
		if err != nil {
			return err
		}
	}

	user.SetChangeLog("update", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": user},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (user *User) DeleteUser(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = user.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	user.Deleted = true
	user.DeletedBy = &userID
	now := time.Now().Local()
	user.DeletedAt = &now

	user.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": user.ID},
		bson.M{"$set": user},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindUserByID(
	ID *primitive.ObjectID,
	selectFields bson.M,
) (user *User, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&user)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		user.CreatedByUser, _ = FindUserByID(user.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		user.UpdatedByUser, _ = FindUserByID(user.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		user.DeletedByUser, _ = FindUserByID(user.DeletedBy, fields)
	}

	return user, err
}

func (user *User) IsEmailExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if user.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": user.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": user.Email,
			"_id":   bson.M{"$ne": user.ID},
		})
	}

	return (count == 1), err
}

func (user *User) IsPhoneExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if user.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"mob": user.Mob,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"mob": user.Mob,
			"_id": bson.M{"$ne": user.ID},
		})
	}

	return (count == 1), err
}

func IsUserExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func HashPassword(password string) string {
	salt, _ := bcrypt.Salt(10)
	hash, _ := bcrypt.Hash(password, salt)
	return hash
}
