package models

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type DeliveryNoteProduct struct {
	ProductID        primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name             string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic     string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode         string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PrefixPartNumber string             `bson:"prefix_part_number" json:"prefix_part_number"`
	PartNumber       string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity         float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit             string             `bson:"unit,omitempty" json:"unit,omitempty"`
}

// DeliveryNote : DeliveryNote structure
type DeliveryNote struct {
	ID              primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Code            string                `bson:"code,omitempty" json:"code,omitempty"`
	Date            *time.Time            `bson:"date,omitempty" json:"date,omitempty"`
	DateStr         string                `json:"date_str,omitempty" bson:"-"`
	StoreID         *primitive.ObjectID   `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID      *primitive.ObjectID   `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Customer        *Customer             `json:"customer,omitempty"  bson:"-" `
	Products        []DeliveryNoteProduct `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy     *primitive.ObjectID   `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	CreatedAt       *time.Time            `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time            `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy       *primitive.ObjectID   `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       *primitive.ObjectID   `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CustomerName    string                `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName       string                `json:"store_name,omitempty" bson:"store_name,omitempty"`
	DeliveredByName string                `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CreatedByName   string                `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName   string                `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
}

func (deliverynote *DeliveryNote) UpdateForeignLabelFields() error {
	store, err := FindStoreByID(deliverynote.StoreID, bson.M{})
	if err != nil {
		return err
	}

	if deliverynote.StoreID != nil {
		store, err := FindStoreByID(deliverynote.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		deliverynote.StoreName = store.Name
	}

	if deliverynote.CustomerID != nil {
		customer, err := store.FindCustomerByID(deliverynote.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		deliverynote.CustomerName = customer.Name
	}

	if deliverynote.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(deliverynote.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		deliverynote.DeliveredByName = deliveredByUser.Name
	}

	if deliverynote.CreatedBy != nil {
		createdByUser, err := FindUserByID(deliverynote.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		deliverynote.CreatedByName = createdByUser.Name
	}

	if deliverynote.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(deliverynote.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		deliverynote.UpdatedByName = updatedByUser.Name
	}

	for i, product := range deliverynote.Products {
		productObject, err := store.FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1, "prefix_part_number": 1})
		if err != nil {
			return err
		}

		deliverynote.Products[i].Name = productObject.Name
		deliverynote.Products[i].NameInArabic = productObject.NameInArabic
		deliverynote.Products[i].ItemCode = productObject.ItemCode
		deliverynote.Products[i].PartNumber = productObject.PartNumber
		deliverynote.Products[i].PrefixPartNumber = productObject.PrefixPartNumber
	}

	return nil
}

func (store *Store) SearchDeliveryNote(w http.ResponseWriter, r *http.Request) (deliverynotes []DeliveryNote, criterias SearchCriterias, err error) {
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

	keys, ok = r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
		}

		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}

		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	}

	var startDate time.Time
	var endDate time.Time

	keys, ok = r.URL.Query()["search[from_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
		}
		if timeZoneOffset != 0 {
			startDate = ConvertTimeZoneToUTC(timeZoneOffset, startDate)
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
		}
		if timeZoneOffset != 0 {
			endDate = ConvertTimeZoneToUTC(timeZoneOffset, endDate)
		}

	}

	if !startDate.IsZero() && !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate, "$lte": endDate}
	} else if !startDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$gte": startDate}
	} else if !endDate.IsZero() {
		criterias.SearchBy["date"] = bson.M{"$lte": endDate}
	}

	var createdAtStartDate time.Time
	var createdAtEndDate time.Time

	keys, ok = r.URL.Query()["search[created_at]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
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
			return deliverynotes, criterias, err
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
			return deliverynotes, criterias, err
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

	keys, ok = r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return deliverynotes, criterias, err
		}
		criterias.SearchBy["store_id"] = storeID
	}

	keys, ok = r.URL.Query()["search[code]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["code"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[net_total]"]
	if ok && len(keys[0]) >= 1 {
		operator := GetMongoLogicalOperator(keys[0])
		keys[0] = TrimLogicalOperatorPrefix(keys[0])

		value, err := strconv.ParseFloat(keys[0], 32)
		if err != nil {
			return deliverynotes, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float64(value)}
		} else {
			criterias.SearchBy["net_total"] = float64(value)
		}

	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return deliverynotes, criterias, err
			}
			objecIds = append(objecIds, customerID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["customer_id"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return deliverynotes, criterias, err
			}
			objecIds = append(objecIds, userID)
		}

		if len(objecIds) > 0 {
			criterias.SearchBy["created_by"] = bson.M{"$in": objecIds}
		}
	}

	keys, ok = r.URL.Query()["search[status]"]
	if ok && len(keys[0]) >= 1 {
		statusList := strings.Split(keys[0], ",")
		if len(statusList) > 0 {
			criterias.SearchBy["status"] = bson.M{"$in": statusList}
		}
	}

	keys, ok = r.URL.Query()["search[delivered_by]"]
	if ok && len(keys[0]) >= 1 {
		deliveredByID, err := primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return deliverynotes, criterias, err
		}
		criterias.SearchBy["delivered_by"] = deliveredByID
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

	collection := db.GetDB("store_" + store.ID.Hex()).Collection("delivery_note")
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
		return deliverynotes, criterias, errors.New("Error fetching deliverynotes:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return deliverynotes, criterias, errors.New("Cursor error:" + err.Error())
		}
		deliverynote := DeliveryNote{}
		err = cur.Decode(&deliverynote)
		if err != nil {
			return deliverynotes, criterias, errors.New("Cursor decode error:" + err.Error())
		}
		deliverynotes = append(deliverynotes, deliverynote)
	} //end for loop

	return deliverynotes, criterias, nil
}

func (deliverynote *DeliveryNote) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {
	errs = make(map[string]string)

	store, err := FindStoreByID(deliverynote.StoreID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		errs["store_id"] = "invalid store id"
		return errs
	}

	if govalidator.IsNull(deliverynote.DateStr) {
		errs["date_str"] = "Date is required"
	} else {
		const shortForm = "2006-01-02T15:04:05Z07:00"
		date, err := time.Parse(shortForm, deliverynote.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		deliverynote.Date = &date
	}

	/*
		if govalidator.IsNull(deliverynote.DateStr) {
			errs["date_str"] = "date_str is required"
		} else {
			const shortForm = "Jan 02 2006"
			date, err := time.Parse(shortForm, deliverynote.DateStr)
			if err != nil {
				errs["date_str"] = "Invalid date format"
			}
			deliverynote.Date = &date
		}
	*/

	if scenario == "update" {
		if deliverynote.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := store.IsDeliveryNoteExists(&deliverynote.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid DeliveryNote:" + deliverynote.ID.Hex()
		}

	}

	if deliverynote.StoreID == nil || deliverynote.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(deliverynote.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + deliverynote.StoreID.Hex()
		}
	}

	if deliverynote.CustomerID == nil || deliverynote.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := store.IsCustomerExists(deliverynote.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + deliverynote.CustomerID.Hex()
		}
	}

	if deliverynote.DeliveredBy == nil || deliverynote.DeliveredBy.IsZero() {
		errs["delivered_by"] = "Delivered By is required"
	} else {
		exists, err := IsUserExists(deliverynote.DeliveredBy)
		if err != nil {
			errs["delivered_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by"] = "Invalid Delivered By:" + deliverynote.DeliveredBy.Hex()
		}
	}

	if len(deliverynote.Products) == 0 {
		errs["product_id"] = "Atleast 1 product is required for deliverynote"
	}

	for index, product := range deliverynote.Products {
		if product.ProductID.IsZero() {
			errs["product_id_"+strconv.Itoa(index)] = "Product is required for deliverynote"
		} else {
			exists, err := store.IsProductExists(&product.ProductID)
			if err != nil {
				errs["product_id_"+strconv.Itoa(index)] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id_"+strconv.Itoa(index)] = "Invalid product_id:" + product.ProductID.Hex() + " in products"
			}
		}

		if product.Quantity == 0 {
			errs["quantity_"+strconv.Itoa(index)] = "Quantity is required"
		}

	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (deliverynote *DeliveryNote) GenerateCode(startFrom int, storeCode string) (string, error) {
	store, err := FindStoreByID(deliverynote.StoreID, bson.M{})
	if err != nil {
		return "", err
	}

	count, err := store.GetTotalCount(bson.M{"store_id": deliverynote.StoreID}, "delivery_note")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (store *Store) GetDeliveryNoteCount() (count int64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return collection.CountDocuments(ctx, bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	})
}

func (model *DeliveryNote) MakeCode() error {
	store, err := FindStoreByID(model.StoreID, bson.M{})
	if err != nil {
		return err
	}

	redisKey := model.StoreID.Hex() + "_delivery_note_counter"

	// Check if counter exists, if not set it to the custom startFrom - 1
	exists, err := db.RedisClient.Exists(redisKey).Result()
	if err != nil {
		return err
	}

	if exists == 0 {
		count, err := store.GetDeliveryNoteCount()
		if err != nil {
			return err
		}

		startFrom := store.DeliveryNoteSerialNumber.StartFromCount

		startFrom += count
		// Set the initial counter value (startFrom - 1) so that the first increment gives startFrom
		err = db.RedisClient.Set(redisKey, startFrom-1, 0).Err()
		if err != nil {
			return err
		}
	}

	incr, err := db.RedisClient.Incr(redisKey).Result()
	if err != nil {
		return err
	}

	paddingCount := store.DeliveryNoteSerialNumber.PaddingCount
	if store.DeliveryNoteSerialNumber.Prefix != "" {
		model.Code = fmt.Sprintf("%s-%0*d", store.DeliveryNoteSerialNumber.Prefix, paddingCount, incr)
	} else {
		model.Code = fmt.Sprintf("%s%0*d", store.DeliveryNoteSerialNumber.Prefix, paddingCount, incr)
	}

	if store.CountryCode != "" {
		timeZone, ok := TimezoneMap[strings.ToUpper(store.CountryCode)]
		if ok {
			location, err := time.LoadLocation(timeZone)
			if err != nil {
				return errors.New("error loading location")
			}
			currentDate := time.Now().In(location).Format("20060102") // YYYYMMDD
			model.Code = strings.ReplaceAll(model.Code, "DATE", currentDate)
		}
	}
	return nil
}

func (deliverynote *DeliveryNote) Insert() error {
	collection := db.GetDB("store_" + deliverynote.StoreID.Hex()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := deliverynote.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	deliverynote.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &deliverynote)
	if err != nil {
		return err
	}
	return nil
}

func (deliverynote *DeliveryNote) Update() error {
	collection := db.GetDB("store_" + deliverynote.StoreID.Hex()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := deliverynote.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": deliverynote.ID},
		bson.M{"$set": deliverynote},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (store *Store) FindDeliveryNoteByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (deliverynote *DeliveryNote, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("delivery_note")
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
		Decode(&deliverynote)
	if err != nil {
		return nil, err
	}

	return deliverynote, err
}

func (store *Store) IsDeliveryNoteExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count > 0), err
}

func (store *Store) ProcessDeliveryNotes() error {
	log.Print("Processing delivery notes")
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("delivery_note")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetNoCursorTimeout(true)
	findOptions.SetAllowDiskUse(true)

	cur, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return errors.New("Error fetching deliverynotes:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return errors.New("Cursor error:" + err.Error())
		}
		deliverynote := DeliveryNote{}
		err = cur.Decode(&deliverynote)
		if err != nil {
			return errors.New("Cursor decode error:" + err.Error())
		}

		err = deliverynote.ClearProductsDeliveryNoteHistory()
		if err != nil {
			return err
		}

		err = deliverynote.AddProductsDeliveryNoteHistory()
		if err != nil {
			return err
		}
		/*

			err = deliverynote.SetProductsDeliveryNoteStats()
			if err != nil {
				return err
			}
		*/

		/*
			err = deliverynote.SetCustomerDeliveryNoteStats()
			if err != nil {
				return err
			}


			err = deliverynote.Update()
			if err != nil {
				return err
			}*/
	}
	log.Print("DONE!")
	return nil
}

type ProductDeliveryNoteStats struct {
	DeliveryNoteCount    int64   `json:"delivery_note_count" bson:"delivery_note_count"`
	DeliveryNoteQuantity float64 `json:"delivery_note_quantity" bson:"delivery_note_quantity"`
}

func (product *Product) SetProductDeliveryNoteStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + product.StoreID.Hex()).Collection("product_delivery_note_history")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats ProductDeliveryNoteStats

	filter := map[string]interface{}{
		"store_id":   storeID,
		"product_id": product.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                    nil,
				"delivery_note_count":    bson.M{"$sum": 1},
				"delivery_note_quantity": bson.M{"$sum": "$quantity"},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
	}

	for storeIndex, store := range product.ProductStores {
		if store.StoreID.Hex() == storeID.Hex() {
			if productStore, ok := product.ProductStores[storeIndex]; ok {
				productStore.DeliveryNoteCount = stats.DeliveryNoteCount
				productStore.DeliveryNoteQuantity = stats.DeliveryNoteQuantity
				product.ProductStores[storeIndex] = productStore
			}

			break
		}
	}

	return nil
}

func (deliveryNote *DeliveryNote) SetProductsDeliveryNoteStats() error {
	store, err := FindStoreByID(deliveryNote.StoreID, bson.M{})
	if err != nil {
		return err
	}

	for _, deliveryNoteProduct := range deliveryNote.Products {
		product, err := store.FindProductByID(&deliveryNoteProduct.ProductID, map[string]interface{}{})
		if err != nil {
			return err
		}

		err = product.SetProductDeliveryNoteStatsByStoreID(*deliveryNote.StoreID)
		if err != nil {
			return err
		}

		err = product.Update(nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// Customer
type CustomerDeliveryNoteStats struct {
	DeliveryNoteCount int64 `json:"delivery_note_count" bson:"delivery_note_count"`
}

func (customer *Customer) SetCustomerDeliveryNoteStatsByStoreID(storeID primitive.ObjectID) error {
	collection := db.GetDB("store_" + customer.StoreID.Hex()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stats CustomerDeliveryNoteStats

	filter := map[string]interface{}{
		"store_id":    storeID,
		"customer_id": customer.ID,
	}

	pipeline := []bson.M{
		bson.M{
			"$match": filter,
		},
		bson.M{
			"$group": bson.M{
				"_id":                 nil,
				"delivery_note_count": bson.M{"$sum": 1},
			},
		},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}

	defer cur.Close(ctx)

	if cur.Next(ctx) {
		err := cur.Decode(&stats)
		if err != nil {
			return err
		}
	}

	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		return errors.New("error finding store: " + err.Error())
	}

	if len(customer.Stores) == 0 {
		customer.Stores = map[string]CustomerStore{}
	}

	if customerStore, ok := customer.Stores[storeID.Hex()]; ok {
		customerStore.StoreID = storeID
		customerStore.StoreName = store.Name
		customerStore.StoreNameInArabic = store.NameInArabic
		customerStore.DeliveryNoteCount = stats.DeliveryNoteCount
		customer.Stores[storeID.Hex()] = customerStore
	} else {
		customer.Stores[storeID.Hex()] = CustomerStore{
			StoreID:           storeID,
			StoreName:         store.Name,
			StoreNameInArabic: store.NameInArabic,
			DeliveryNoteCount: stats.DeliveryNoteCount,
		}
	}

	err = customer.Update()
	if err != nil {
		return errors.New("Error updating customer: " + err.Error())
	}

	return nil
}

func (deliveryNote *DeliveryNote) SetCustomerDeliveryNoteStats() error {
	store, err := FindStoreByID(deliveryNote.StoreID, bson.M{})
	if err != nil {
		return err
	}

	customer, err := store.FindCustomerByID(deliveryNote.CustomerID, map[string]interface{}{})
	if err != nil {
		return err
	}

	err = customer.SetCustomerDeliveryNoteStatsByStoreID(*deliveryNote.StoreID)
	if err != nil {
		return err
	}

	return nil
}
