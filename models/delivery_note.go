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
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

type DeliveryNoteProduct struct {
	ProductID    primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Name         string             `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic string             `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	ItemCode     string             `bson:"item_code,omitempty" json:"item_code,omitempty"`
	PartNumber   string             `bson:"part_number,omitempty" json:"part_number,omitempty"`
	Quantity     float64            `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Unit         string             `bson:"unit,omitempty" json:"unit,omitempty"`
}

//DeliveryNote : DeliveryNote structure
type DeliveryNote struct {
	ID              primitive.ObjectID    `json:"id,omitempty" bson:"_id,omitempty"`
	Code            string                `bson:"code,omitempty" json:"code,omitempty"`
	Date            *time.Time            `bson:"date,omitempty" json:"date,omitempty"`
	DateStr         string                `json:"date_str,omitempty"`
	StoreID         *primitive.ObjectID   `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID      *primitive.ObjectID   `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
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

	if deliverynote.StoreID != nil {
		store, err := FindStoreByID(deliverynote.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		deliverynote.StoreName = store.Name
	}

	if deliverynote.CustomerID != nil {
		customer, err := FindCustomerByID(deliverynote.CustomerID, bson.M{"id": 1, "name": 1})
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
		productObject, err := FindProductByID(&product.ProductID, bson.M{"id": 1, "name": 1, "name_in_arabic": 1, "item_code": 1, "part_number": 1})
		if err != nil {
			return err
		}

		deliverynote.Products[i].Name = productObject.Name
		deliverynote.Products[i].NameInArabic = productObject.NameInArabic
		deliverynote.Products[i].ItemCode = productObject.ItemCode
		deliverynote.Products[i].PartNumber = productObject.PartNumber
	}

	return nil
}

func SearchDeliveryNote(w http.ResponseWriter, r *http.Request) (deliverynotes []DeliveryNote, criterias SearchCriterias, err error) {
	criterias = SearchCriterias{
		Page:   1,
		Size:   10,
		SortBy: map[string]interface{}{},
	}

	criterias.SearchBy = make(map[string]interface{})

	criterias.SearchBy["deleted"] = bson.M{"$ne": true}

	keys, ok := r.URL.Query()["search[date_str]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		startDate, err := time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
		}
		endDate := startDate.Add(time.Hour * time.Duration(24))
		endDate = endDate.Add(-time.Second * time.Duration(1))
		log.Print(startDate)
		log.Print(endDate)
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
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
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
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return deliverynotes, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

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

	if scenario == "update" {
		if deliverynote.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsDeliveryNoteExists(&deliverynote.ID)
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
		exists, err := IsCustomerExists(deliverynote.CustomerID)
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
			exists, err := IsProductExists(&product.ProductID)
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
	count, err := GetTotalCount(bson.M{"store_id": deliverynote.StoreID}, "delivery_note")
	if err != nil {
		return "", err
	}
	code := startFrom + int(count)
	return storeCode + "-" + strconv.Itoa(code+1), nil
}

func (deliverynote *DeliveryNote) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := deliverynote.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	store, err := FindStoreByID(deliverynote.StoreID, bson.M{})
	if err != nil {
		return err
	}

	deliverynote.ID = primitive.NewObjectID()
	if len(deliverynote.Code) == 0 {
		startAt := 90000
		for {
			code, err := deliverynote.GenerateCode(startAt, store.Code)
			if err != nil {
				return err
			}
			deliverynote.Code = code
			exists, err := deliverynote.IsCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
			startAt++
		}
	}

	_, err = collection.InsertOne(ctx, &deliverynote)
	if err != nil {
		return err
	}
	return nil
}

func (deliverynote *DeliveryNote) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if deliverynote.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": deliverynote.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": deliverynote.Code,
			"_id":  bson.M{"$ne": deliverynote.ID},
		})
	}

	return (count == 1), err
}

func (deliverynote *DeliveryNote) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
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

func FindDeliveryNoteByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (deliverynote *DeliveryNote, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&deliverynote)
	if err != nil {
		return nil, err
	}

	return deliverynote, err
}

func IsDeliveryNoteExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}

func ProcessDeliveryNotes() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("delivery_note")
	ctx := context.Background()
	findOptions := options.Find()

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

		err = deliverynote.AddProductsDeliveryNoteHistory()
		if err != nil {
			return err
		}

		err = deliverynote.Update()
		if err != nil {
			return err
		}
	}

	return nil
}
