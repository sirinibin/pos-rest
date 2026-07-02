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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CustomerPackage struct {
	ID            primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name          string              `bson:"name,omitempty" json:"name,omitempty"`
	TabIDs        []string            `bson:"tab_ids" json:"tab_ids"`
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
}

func customerPackageCollection() *mongo.Collection {
	return db.Client("").Database(db.GetPosDB()).Collection("customer_package")
}

func (p *CustomerPackage) UpdateForeignLabelFields() error {
	if p.CreatedBy != nil {
		u, err := FindUserByID(p.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		p.CreatedByName = u.Name
	}
	if p.UpdatedBy != nil {
		u, err := FindUserByID(p.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		p.UpdatedByName = u.Name
	}
	return nil
}

func SearchCustomerPackage(w http.ResponseWriter, r *http.Request) (packages []CustomerPackage, criterias SearchCriterias, err error) {
	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}
	criterias.SearchBy = make(map[string]interface{})
	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	var keys []string
	var ok bool

	keys, ok = r.URL.Query()["search[name]"]
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

	offset := (criterias.Page - 1) * criterias.Size

	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(bson.D{bson.E{Key: "name", Value: 1}})

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		findOptions.SetProjection(criterias.Select)
	}

	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := collection.Find(ctx, criterias.SearchBy, findOptions)
	if err != nil {
		return packages, criterias, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var pkg CustomerPackage
		if err := cur.Decode(&pkg); err != nil {
			return packages, criterias, err
		}
		packages = append(packages, pkg)
	}

	return packages, criterias, nil
}

func FindCustomerPackageByID(id *primitive.ObjectID, selectFields map[string]interface{}) (*CustomerPackage, error) {
	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	var pkg CustomerPackage
	err := collection.FindOne(ctx, bson.M{"_id": id}, findOneOptions).Decode(&pkg)
	if err != nil {
		return nil, err
	}
	return &pkg, nil
}

func IsCustomerPackageExists(id *primitive.ObjectID) (bool, error) {
	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count, err := collection.CountDocuments(ctx, bson.M{"_id": id, "deleted": bson.M{"$ne": true}})
	return count > 0, err
}

func (p *CustomerPackage) IsNameExists() (bool, error) {
	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"name": p.Name, "deleted": bson.M{"$ne": true}}
	if !p.ID.IsZero() {
		filter["_id"] = bson.M{"$ne": p.ID}
	}
	count, err := collection.CountDocuments(ctx, filter)
	return count > 0, err
}

func (p *CustomerPackage) Validate(w http.ResponseWriter, r *http.Request, scenario string) map[string]string {
	errs := make(map[string]string)

	if scenario == "update" {
		if p.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsCustomerPackageExists(&p.ID)
		if err != nil || !exists {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "Invalid Customer Package ID"
			return errs
		}
	}

	if govalidator.IsNull(p.Name) {
		errs["name"] = "Name is required"
	}

	nameExists, err := p.IsNameExists()
	if err != nil {
		errs["name"] = err.Error()
	} else if nameExists {
		w.WriteHeader(http.StatusConflict)
		errs["name"] = "Name is already in use"
	}

	if len(errs) > 0 && errs["name"] == "" {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (p *CustomerPackage) Insert() error {
	if err := p.UpdateForeignLabelFields(); err != nil {
		return err
	}
	p.ID = primitive.NewObjectID()
	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.InsertOne(ctx, p)
	return err
}

func (p *CustomerPackage) Update() error {
	if err := p.UpdateForeignLabelFields(); err != nil {
		return err
	}
	now := time.Now()
	p.UpdatedAt = &now

	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.UpdateOne(ctx,
		bson.M{"_id": p.ID},
		bson.M{"$set": p},
		options.Update().SetUpsert(false),
	)
	if err != nil {
		return err
	}

	// Propagate tab_ids to all stores using this package
	return propagatePackageTabsToStores(p)
}

// propagatePackageTabsToStores updates customer_package_tab_ids on every store
// that has this package assigned, so the sidebar only needs one fetch.
func propagatePackageTabsToStores(p *CustomerPackage) error {
	storeCollection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := storeCollection.UpdateMany(ctx,
		bson.M{"customer_package_id": p.ID},
		bson.M{"$set": bson.M{"customer_package_tab_ids": p.TabIDs, "customer_package_name": p.Name}},
	)
	return err
}

func (p *CustomerPackage) Delete(tokenClaims TokenClaims) error {
	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return errors.New("invalid user id")
	}

	deletedByName := ""
	if u, err2 := FindUserByID(&userID, bson.M{"id": 1, "name": 1}); err2 == nil {
		deletedByName = u.Name
	}

	collection := customerPackageCollection()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	_, err = collection.UpdateOne(ctx,
		bson.M{"_id": p.ID},
		bson.M{"$set": bson.M{
			"deleted":         true,
			"deleted_by":      userID,
			"deleted_at":      now,
			"deleted_by_name": deletedByName,
		}},
	)
	return err
}
