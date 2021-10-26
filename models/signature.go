package models

import (
	"context"
	"errors"
	"net/http"
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
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name      string             `bson:"name,omitempty" json:"name,omitempty"`
	Signature string             `bson:"signature,omitempty" json:"signature,omitempty"`
	Deleted   bool               `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedAt time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

func SearchSignature(w http.ResponseWriter, r *http.Request) (signatures []Signature, criterias SearchCriterias, err error) {

	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})

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
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsSignatureExists(signature.ID)
		if err != nil || !exists {
			errs["id"] = err.Error()
			return errs
		}

	}

	if govalidator.IsNull(signature.Name) {
		errs["name"] = "Name is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (signature *Signature) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	signature.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &signature)
	if err != nil {
		return err
	}
	return nil
}

func (signature *Signature) Update() (*Signature, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": signature.ID},
		bson.M{"$set": signature},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return signature, nil
	}
	return nil, nil
}

func (signature *Signature) DeleteSignature(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	signature.Deleted = true
	signature.DeletedBy = userID
	signature.DeletedAt = time.Now().Local()

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

func FindSignatureByID(ID primitive.ObjectID) (signature *Signature, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}).
		Decode(&signature)
	if err != nil {
		return nil, err
	}

	return signature, err
}

func IsSignatureExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("signature")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
