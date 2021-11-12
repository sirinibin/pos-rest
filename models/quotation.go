package models

import (
	"context"
	"errors"
	"math"
	"math/rand"
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

type QuotationProduct struct {
	ProductID primitive.ObjectID `json:"product_id,omitempty" bson:"product_id,omitempty"`
	Quantity  int                `json:"quantity,omitempty" bson:"quantity,omitempty"`
	Price     float32            `bson:"price,omitempty" json:"price,omitempty"`
}

//Quotation : Quotation structure
type Quotation struct {
	ID                     primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Code                   string              `bson:"code,omitempty" json:"code,omitempty"`
	Date                   *time.Time          `bson:"date,omitempty" json:"date,omitempty"`
	DateStr                string              `json:"date_str,omitempty"`
	StoreID                *primitive.ObjectID `json:"store_id,omitempty" bson:"store_id,omitempty"`
	CustomerID             *primitive.ObjectID `json:"customer_id,omitempty" bson:"customer_id,omitempty"`
	Store                  *Store              `json:"store,omitempty"`
	Customer               *Customer           `json:"customer,omitempty"`
	Products               []QuotationProduct  `bson:"products,omitempty" json:"products,omitempty"`
	DeliveredBy            *primitive.ObjectID `json:"delivered_by,omitempty" bson:"delivered_by,omitempty"`
	DeliveredBySignatureID *primitive.ObjectID `json:"delivered_by_signature_id,omitempty" bson:"delivered_by_signature_id,omitempty"`
	DeliveredByUser        *User               `json:"delivered_by_user,omitempty"`
	DeliveredBySignature   *Signature          `json:"delivered_by_signature,omitempty"`
	VatPercent             *float32            `bson:"vat_percent,omitempty" json:"vat_percent,omitempty"`
	Discount               float32             `bson:"discount,omitempty" json:"discount,omitempty"`
	Status                 string              `bson:"status,omitempty" json:"status,omitempty"`
	NetTotal               float32             `bson:"net_total" json:"net_total"`
	Deleted                bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy              *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser          *User               `json:"deleted_by_user,omitempty"`
	DeletedAt              *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt              *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt              *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy              *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy              *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser          *User               `json:"created_by_user,omitempty"`
	UpdatedByUser          *User               `json:"updated_by_user,omitempty"`
	CustomerName           string              `json:"customer_name,omitempty" bson:"customer_name,omitempty"`
	StoreName              string              `json:"store_name,omitempty" bson:"store_name,omitempty"`
	DeliveredByName        string              `json:"delivered_by_name,omitempty" bson:"delivered_by_name,omitempty"`
	CreatedByName          string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName          string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName          string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog              []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
}

func (quotation *Quotation) SetChangeLog(
	event string,
	name, oldValue, newValue interface{},
) {
	now := time.Now()
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

	quotation.ChangeLog = append(
		quotation.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (quotation *Quotation) AttributesValueChangeEvent(quotationOld *Quotation) error {

	if quotation.Status != quotationOld.Status {
		quotation.SetChangeLog(
			"attribute_value_change",
			"status",
			quotationOld.Status,
			quotation.Status,
		)
	}

	return nil
}

func (quotation *Quotation) UpdateForeignLabelFields() error {

	if quotation.StoreID != nil {
		store, err := FindStoreByID(quotation.StoreID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.StoreName = store.Name
	}

	if quotation.CustomerID != nil {
		customer, err := FindCustomerByID(quotation.CustomerID, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.CustomerName = customer.Name
	}

	if quotation.DeliveredBy != nil {
		deliveredByUser, err := FindUserByID(quotation.DeliveredBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeliveredByName = deliveredByUser.Name
	}

	if quotation.CreatedBy != nil {
		createdByUser, err := FindUserByID(quotation.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.CreatedByName = createdByUser.Name
	}

	if quotation.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(quotation.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.UpdatedByName = updatedByUser.Name
	}

	if quotation.DeletedBy != nil && !quotation.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(quotation.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		quotation.DeletedByName = deletedByUser.Name
	}

	return nil
}

func (quotation *Quotation) FindNetTotal() {
	netTotal := float32(0.0)
	for _, product := range quotation.Products {
		netTotal += (float32(product.Quantity) * product.Price)
	}

	if quotation.VatPercent != nil {
		netTotal += netTotal * (*quotation.VatPercent / float32(100))
	}

	netTotal -= quotation.Discount
	quotation.NetTotal = float32(math.Ceil(float64(netTotal*100)) / float64(100))
}

func SearchQuotation(w http.ResponseWriter, r *http.Request) (quotations []Quotation, criterias SearchCriterias, err error) {

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
			return quotations, criterias, err
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
			return quotations, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[to_date]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		endDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
		}
	}

	keys, ok = r.URL.Query()["search[created_at_to]"]
	if ok && len(keys[0]) >= 1 {
		const shortForm = "Jan 02 2006"
		createdAtEndDate, err = time.Parse(shortForm, keys[0])
		if err != nil {
			return quotations, criterias, err
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
			return quotations, criterias, err
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
			return quotations, criterias, err
		}

		if operator != "" {
			criterias.SearchBy["net_total"] = bson.M{operator: float32(value)}
		} else {
			criterias.SearchBy["net_total"] = float32(value)
		}

	}

	keys, ok = r.URL.Query()["search[customer_id]"]
	if ok && len(keys[0]) >= 1 {

		customerIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range customerIds {
			customerID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return quotations, criterias, err
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
				return quotations, criterias, err
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
			return quotations, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx := context.Background()
	findOptions := options.Find()
	findOptions.SetSkip(int64(offset))
	findOptions.SetLimit(int64(criterias.Size))
	findOptions.SetSort(criterias.SortBy)
	findOptions.SetNoCursorTimeout(true)

	storeSelectFields := map[string]interface{}{}
	customerSelectFields := map[string]interface{}{}
	createdByUserSelectFields := map[string]interface{}{}
	updatedByUserSelectFields := map[string]interface{}{}
	deletedByUserSelectFields := map[string]interface{}{}

	keys, ok = r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		criterias.Select = ParseSelectString(keys[0])
		//Relational Select Fields
		if _, ok := criterias.Select["store.id"]; ok {
			storeSelectFields = ParseRelationalSelectString(keys[0], "store")
		}

		if _, ok := criterias.Select["customer.id"]; ok {
			customerSelectFields = ParseRelationalSelectString(keys[0], "customer")
		}

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
		return quotations, criterias, errors.New("Error fetching quotations:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return quotations, criterias, errors.New("Cursor error:" + err.Error())
		}
		quotation := Quotation{}
		err = cur.Decode(&quotation)
		if err != nil {
			return quotations, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		if _, ok := criterias.Select["store.id"]; ok {
			quotation.Store, _ = FindStoreByID(quotation.StoreID, storeSelectFields)
		}
		if _, ok := criterias.Select["customer.id"]; ok {
			quotation.Customer, _ = FindCustomerByID(quotation.CustomerID, customerSelectFields)
		}
		if _, ok := criterias.Select["created_by_user.id"]; ok {
			quotation.CreatedByUser, _ = FindUserByID(quotation.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			quotation.UpdatedByUser, _ = FindUserByID(quotation.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			quotation.DeletedByUser, _ = FindUserByID(quotation.DeletedBy, deletedByUserSelectFields)
		}

		quotations = append(quotations, quotation)
	} //end for loop

	return quotations, criterias, nil
}

func (quotation *Quotation) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if govalidator.IsNull(quotation.Status) {
		errs["status"] = "Status is required"
	}

	if govalidator.IsNull(quotation.DateStr) {
		errs["date_str"] = "date_str is required"
	} else {
		const shortForm = "Jan 02 2006"
		date, err := time.Parse(shortForm, quotation.DateStr)
		if err != nil {
			errs["date_str"] = "Invalid date format"
		}
		quotation.Date = &date
	}

	if scenario == "update" {
		if quotation.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsQuotationExists(&quotation.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Quotation:" + quotation.ID.Hex()
		}

	}

	if quotation.StoreID.IsZero() {
		errs["store_id"] = "Store is required"
	} else {
		exists, err := IsStoreExists(quotation.StoreID)
		if err != nil {
			errs["store_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["store_id"] = "Invalid store:" + quotation.StoreID.Hex()
			return errs
		}
	}

	if quotation.CustomerID.IsZero() {
		errs["customer_id"] = "Customer is required"
	} else {
		exists, err := IsCustomerExists(quotation.CustomerID)
		if err != nil {
			errs["customer_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["customer_id"] = "Invalid Customer:" + quotation.CustomerID.Hex()
		}
	}

	if quotation.DeliveredBy.IsZero() {
		errs["delivered_by"] = "Delivered By is required"
	} else {
		exists, err := IsUserExists(quotation.DeliveredBy)
		if err != nil {
			errs["delivered_by"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by"] = "Invalid Delivered By:" + quotation.DeliveredBy.Hex()
		}
	}

	if len(quotation.Products) == 0 {
		errs["products"] = "Atleast 1 product is required for quotation"
	}

	if !quotation.DeliveredBySignatureID.IsZero() {
		exists, err := IsSignatureExists(quotation.DeliveredBySignatureID)
		if err != nil {
			errs["delivered_by_signature_id"] = err.Error()
			return errs
		}

		if !exists {
			errs["delivered_by_signature_id"] = "Invalid Delivered By Signature:" + quotation.DeliveredBySignatureID.Hex()
		}
	}

	for _, product := range quotation.Products {
		if product.ProductID.IsZero() {
			errs["product_id"] = "Product is required for quotation"
		} else {
			exists, err := IsProductExists(&product.ProductID)
			if err != nil {
				errs["product_id"] = err.Error()
				return errs
			}

			if !exists {
				errs["product_id"] = "Invalid product_id:" + product.ProductID.Hex() + " in products"
			}
		}

		if product.Quantity == 0 {
			errs["quantity"] = "Quantity is required"
		}

	}

	if quotation.VatPercent == nil {
		errs["vat_percent"] = "VAT Percentage is required"
	}

	if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}
	return errs
}

func (quotation *Quotation) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := quotation.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	quotation.ID = primitive.NewObjectID()
	if len(quotation.Code) == 0 {
		for true {
			quotation.Code = strings.ToUpper(GenerateQuotationCode(12))
			exists, err := quotation.IsCodeExists()
			if err != nil {
				return err
			}
			if !exists {
				break
			}
		}
	}

	quotation.SetChangeLog("create", nil, nil, nil)

	_, err = collection.InsertOne(ctx, &quotation)
	if err != nil {
		return err
	}
	return nil
}

func (quotation *Quotation) IsCodeExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if quotation.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotation.Code,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"code": quotation.Code,
			"_id":  bson.M{"$ne": quotation.ID},
		})
	}

	return (count == 1), err
}

func GenerateQuotationCode(n int) string {
	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (quotation *Quotation) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err := quotation.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	quotation.SetChangeLog("update", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (quotation *Quotation) DeleteQuotation(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = quotation.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	quotation.Deleted = true
	quotation.DeletedBy = &userID
	now := time.Now()
	quotation.DeletedAt = &now

	quotation.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": quotation.ID},
		bson.M{"$set": quotation},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindQuotationByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (quotation *Quotation, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions).
		Decode(&quotation)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["store.id"]; ok {
		storeSelectFields := ParseRelationalSelectString(selectFields, "store")
		quotation.Store, _ = FindStoreByID(quotation.StoreID, storeSelectFields)
	}

	if _, ok := selectFields["customer.id"]; ok {
		customerSelectFields := ParseRelationalSelectString(selectFields, "customer")
		quotation.Customer, _ = FindCustomerByID(quotation.CustomerID, customerSelectFields)
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		quotation.CreatedByUser, _ = FindUserByID(quotation.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		quotation.UpdatedByUser, _ = FindUserByID(quotation.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		quotation.DeletedByUser, _ = FindUserByID(quotation.DeletedBy, fields)
	}

	return quotation, err
}

func IsQuotationExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("quotation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
