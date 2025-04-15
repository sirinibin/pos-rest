package models

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/schollz/progressbar/v3"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// ProductBrand : ProductBrand structure
type ProductBrand struct {
	ID        primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name      string              `bson:"name,omitempty" json:"name,omitempty"`
	CreatedAt *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	StoreID   *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
}

func (productBrand *ProductBrand) AttributesValueChangeEvent(productBrandOld *ProductBrand) error {
	store, err := FindStoreByID(productBrand.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if productBrand.Name != productBrandOld.Name {
		err := store.UpdateManyByCollectionName(
			"product",
			bson.M{"brand_id": productBrand.ID},
			bson.M{"brand_name": productBrand.Name},
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (store *Store) SearchProductBrand(w http.ResponseWriter, r *http.Request) (productBrands []ProductBrand, criterias SearchCriterias, err error) {

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
			return productBrands, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return productBrands, criterias, err
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
			return productBrands, criterias, err
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
			return productBrands, criterias, err
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_brand")

	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
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
		return productBrands, criterias, errors.New("Error fetching product brands:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return productBrands, criterias, errors.New("Cursor error:" + err.Error())
		}
		productBrand := ProductBrand{}
		err = cur.Decode(&productBrand)
		if err != nil {
			return productBrands, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		productBrands = append(productBrands, productBrand)
	} //end for loop

	return productBrands, criterias, nil
}

func (productBrand *ProductBrand) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(productBrand.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if scenario == "update" {
		if productBrand.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsProductBrandExists(&productBrand.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Product Brand:" + productBrand.ID.Hex()
		}

	}

	if govalidator.IsNull(productBrand.Name) {
		errs["name"] = "Name is required"
	}

	nameExists, err := productBrand.IsNameExists()
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

func (productBrand *ProductBrand) IsNameExists() (exists bool, err error) {
	collection := db.GetDB("store_" + productBrand.StoreID.Hex()).Collection("product_brand")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if productBrand.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": productBrand.Name,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"name": productBrand.Name,
			"_id":  bson.M{"$ne": productBrand.ID},
		})
	}

	return (count > 0), err
}

func (productBrand *ProductBrand) Insert() error {
	collection := db.GetDB("store_" + productBrand.StoreID.Hex()).Collection("product_brand")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	productBrand.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &productBrand)
	if err != nil {
		return err
	}
	return nil
}

func (productBrand *ProductBrand) Update() error {
	collection := db.GetDB("store_" + productBrand.StoreID.Hex()).Collection("product_brand")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	now := time.Now()
	productBrand.UpdatedAt = &now

	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": productBrand.ID},
		bson.M{"$set": productBrand},
		updateOptions,
	)
	return err
}

func (store *Store) FindProductBrandByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (productBrand *ProductBrand, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_brand")
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
		Decode(&productBrand)
	if err != nil {
		return nil, err
	}

	return productBrand, err
}

func (store *Store) IsProductBrandExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_brand")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func ProcessProductBrands() error {
	log.Printf("Processing product brands")

	stores, err := GetAllStores()
	if err != nil {
		return err
	}

	for _, store := range stores {
		log.Print("Branch name:" + store.BranchName)

		totalCount, err := store.GetTotalCount(bson.M{}, "product_brand")
		if err != nil {
			return err
		}
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("product_brand")
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
			brand := ProductBrand{}
			err = cur.Decode(&brand)
			if err != nil {
				return errors.New("Cursor decode error:" + err.Error())
			}

			err = brand.Update()
			if err != nil {
				return err
			}

			bar.Add(1)
		}
	}

	log.Print("DONE!")
	return nil
}
