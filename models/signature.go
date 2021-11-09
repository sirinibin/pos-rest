package models

import (
	"context"
	"encoding/base64"
	"errors"
	"mime"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

//Signature : Signature structure
type Signature struct {
	ID               primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name             string              `bson:"name,omitempty" json:"name,omitempty"`
	Signature        string              `bson:"signature,omitempty" json:"signature,omitempty"`
	SignatureContent string              `json:"signature_content,omitempty"`
	Deleted          bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy        *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser    *User               `json:"deleted_by_user,omitempty"`
	DeletedAt        *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt        *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt        *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy        *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy        *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser    *User               `json:"created_by_user,omitempty"`
	UpdatedByUser    *User               `json:"updated_by_user,omitempty"`
	CreatedByName    string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName    string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName    string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog        []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func (signature *Signature) SetChangeLog(
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

	signature.ChangeLog = append(
		signature.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (signature *Signature) UpdateForeignLabelFields() error {

	if signature.CreatedBy != nil {
		createdByUser, err := FindUserByID(signature.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		signature.CreatedByName = createdByUser.Name
	}

	if signature.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(signature.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		signature.UpdatedByName = updatedByUser.Name
	}

	if signature.DeletedBy != nil {
		deletedByUser, err := FindUserByID(signature.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		signature.DeletedByName = deletedByUser.Name
	}

	return nil
}

func SearchSignature(w http.ResponseWriter, r *http.Request) (signatures []Signature, criterias SearchCriterias, err error) {

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

	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
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
		return signatures, criterias, errors.New("Error fetching signatures:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return signatures, criterias, errors.New("Cursor error:" + err.Error())
		}
		signature := Signature{}
		err = cur.Decode(&signature)
		if err != nil {
			return signatures, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			signature.CreatedByUser, _ = FindUserByID(signature.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			signature.UpdatedByUser, _ = FindUserByID(signature.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			signature.DeletedByUser, _ = FindUserByID(signature.DeletedBy, deletedByUserSelectFields)
		}

		signatures = append(signatures, signature)
	} //end for loop

	return signatures, criterias, nil
}

func GetTotalCount(filter map[string]interface{}, collectionName string) (count int64, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, filter)
}

func (signature *Signature) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if signature.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsSignatureExists(&signature.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Signature:" + signature.ID.Hex()
		}

	}

	if govalidator.IsNull(signature.Name) {
		errs["name"] = "Name is required"
	}

	if signature.ID.IsZero() && govalidator.IsNull(signature.SignatureContent) {
		errs["signature_content"] = "Signature is required"
	}

	if !govalidator.IsNull(signature.SignatureContent) {
		valid, err := IsStringBase64(signature.SignatureContent)
		if err != nil {
			errs["signature_content"] = err.Error()
			return errs
		}

		if !valid {
			errs["signature_content"] = "Invalid base64 string"
		}
	}

	nameExists, err := signature.IsNameExists()
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

func (signature *Signature) IsNameExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if signature.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name":       signature.Name,
			"created_by": signature.CreatedBy,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name":       signature.Name,
			"_id":        bson.M{"$ne": signature.ID},
			"created_by": signature.CreatedBy,
		})
	}

	return (count > 0), err
}

func GetFileExtensionFromBase64(content []byte) (ext string, err error) {
	filetype := http.DetectContentType(content)
	extensions, err := mime.ExtensionsByType(filetype)
	if err != nil {
		return "", err
	}

	return extensions[len(extensions)-1], nil
}

func (signature *Signature) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := signature.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	signature.ID = primitive.NewObjectID()

	if !govalidator.IsNull(signature.SignatureContent) {
		err := signature.SaveSignatureFile()
		if err != nil {
			return err
		}
	}

	signature.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &signature)
	if err != nil {
		return err
	}
	return nil
}

func (signature *Signature) SaveSignatureFile() error {
	content, err := base64.StdEncoding.DecodeString(signature.SignatureContent)
	if err != nil {
		return err
	}

	extension, err := GetFileExtensionFromBase64(content)
	if err != nil {
		return err
	}

	filename := "images/signatures/signature_" + signature.ID.Hex() + extension
	err = SaveBase64File(filename, content)
	if err != nil {
		return err
	}
	signature.Signature = "/" + filename
	signature.SignatureContent = ""
	return nil
}

func SaveBase64File(filename string, content []byte) error {

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(content); err != nil {
		if err != nil {
			return err
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}

	return nil
}

func (signature *Signature) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := signature.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	if !govalidator.IsNull(signature.SignatureContent) {
		err := signature.SaveSignatureFile()
		if err != nil {
			return err
		}
	}

	signature.SetChangeLog("update", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": signature.ID},
		bson.M{"$set": signature},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func (signature *Signature) DeleteSignature(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = signature.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	signature.Deleted = true
	signature.DeletedBy = &userID
	now := time.Now().Local()
	signature.DeletedAt = &now

	signature.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": signature.ID},
		bson.M{"$set": signature},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindSignatureByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (signature *Signature, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&signature)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		signature.CreatedByUser, _ = FindUserByID(signature.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		signature.UpdatedByUser, _ = FindUserByID(signature.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		signature.DeletedByUser, _ = FindUserByID(signature.DeletedBy, fields)
	}

	return signature, err
}

func IsSignatureExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
