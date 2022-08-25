package models

import (
	"context"
	"errors"
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

type ChangeLog struct {
	Event         string              `bson:"event,omitempty" json:"event,omitempty"`
	Description   string              `bson:"description,omitempty" json:"description,omitempty"`
	CreatedBy     *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	CreatedByName string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	CreatedAt     *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
}

//Customer : Customer structure
type Customer struct {
	ID              primitive.ObjectID  `json:"id,omitempty" bson:"_id,omitempty"`
	Name            string              `bson:"name,omitempty" json:"name,omitempty"`
	NameInArabic    string              `bson:"name_in_arabic,omitempty" json:"name_in_arabic,omitempty"`
	VATNo           string              `bson:"vat_no,omitempty" json:"vat_no,omitempty"`
	VATNoInArabic   string              `bson:"vat_no_in_arabic,omitempty" json:"vat_no_in_arabic,omitempty"`
	Phone           string              `bson:"phone,omitempty" json:"phone,omitempty"`
	PhoneInArabic   string              `bson:"phone_in_arabic,omitempty" json:"phone_in_arabic,omitempty"`
	Title           string              `bson:"title,omitempty" json:"title,omitempty"`
	TitleInArabic   string              `bson:"title_in_arabic,omitempty" json:"title_in_arabic,omitempty"`
	Email           string              `bson:"email,omitempty" json:"email,omitempty"`
	Address         string              `bson:"address,omitempty" json:"address,omitempty"`
	AddressInArabic string              `bson:"address_in_arabic,omitempty" json:"address_in_arabic,omitempty"`
	Deleted         bool                `bson:"deleted,omitempty" json:"deleted,omitempty"`
	DeletedBy       *primitive.ObjectID `json:"deleted_by,omitempty" bson:"deleted_by,omitempty"`
	DeletedByUser   *User               `json:"deleted_by_user,omitempty"`
	DeletedAt       *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt       *time.Time          `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt       *time.Time          `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	CreatedBy       *primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       *primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
	CreatedByUser   *User               `json:"created_by_user,omitempty"`
	UpdatedByUser   *User               `json:"updated_by_user,omitempty"`
	CreatedByName   string              `json:"created_by_name,omitempty" bson:"created_by_name,omitempty"`
	UpdatedByName   string              `json:"updated_by_name,omitempty" bson:"updated_by_name,omitempty"`
	DeletedByName   string              `json:"deleted_by_name,omitempty" bson:"deleted_by_name,omitempty"`
	ChangeLog       []ChangeLog         `json:"change_log,omitempty" bson:"change_log,omitempty"`
	SearchLabel     string              `json:"search_label"`
}

func (customer *Customer) SetChangeLog(
	event string,
	attribute, oldValue, newValue *string,
) {
	now := time.Now()
	description := ""
	if event == "create" {
		description = "Created by " + UserObject.Name
	} else if event == "update" {
		description = "Updated by " + UserObject.Name
	} else if event == "delete" {
		description = "Deleted by " + UserObject.Name
	} else if event == "view" {
		description = "Viewed by " + UserObject.Name
	} else if event == "attribute_value_change" && attribute != nil {
		description = *attribute + " changed from " + *oldValue + " to " + *newValue + " by " + UserObject.Name
	}

	customer.ChangeLog = append(
		customer.ChangeLog,
		ChangeLog{
			Event:         event,
			Description:   description,
			CreatedBy:     &UserObject.ID,
			CreatedByName: UserObject.Name,
			CreatedAt:     &now,
		},
	)
}

func (customer *Customer) AttributesValueChangeEvent(customerOld *Customer) error {

	if customer.Name != customerOld.Name {
		err := UpdateManyByCollectionName(
			"order",
			bson.M{"customer_id": customer.ID},
			bson.M{"customer_name": customer.Name},
		)
		if err != nil {
			return nil
		}
		attribute := "name"
		customer.SetChangeLog(
			"attribute_value_change",
			&attribute,
			&customerOld.Name,
			&customer.Name,
		)

		err = UpdateManyByCollectionName(
			"quotation",
			bson.M{"customer_id": customer.ID},
			bson.M{"customer_name": customer.Name},
		)
		if err != nil {
			return nil
		}
	}

	return nil
}

func UpdateManyByCollectionName(
	collectionName string,
	filter bson.M,
	updateValues bson.M,
) error {

	collection := db.Client().Database(db.GetPosDB()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	defer cancel()

	_, err := collection.UpdateMany(
		ctx,
		filter,
		bson.M{"$set": updateValues},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (customer *Customer) UpdateForeignLabelFields() error {

	if customer.CreatedBy != nil {
		createdByUser, err := FindUserByID(customer.CreatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customer.CreatedByName = createdByUser.Name
	}

	if customer.UpdatedBy != nil {
		updatedByUser, err := FindUserByID(customer.UpdatedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customer.UpdatedByName = updatedByUser.Name
	}

	if customer.DeletedBy != nil && !customer.DeletedBy.IsZero() {
		deletedByUser, err := FindUserByID(customer.DeletedBy, bson.M{"id": 1, "name": 1})
		if err != nil {
			return err
		}
		customer.DeletedByName = deletedByUser.Name
	}

	return nil
}

func SearchCustomer(w http.ResponseWriter, r *http.Request) (customers []Customer, criterias SearchCriterias, err error) {

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

	keys, ok = r.URL.Query()["search[name]"]
	if ok && len(keys[0]) >= 1 {
		//criterias.SearchBy["name"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
		criterias.SearchBy["$or"] = []bson.M{
			{"name": bson.M{"$regex": keys[0], "$options": "i"}},
			{"name_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone": bson.M{"$regex": keys[0], "$options": "i"}},
			{"phone_in_arabic": bson.M{"$regex": keys[0], "$options": "i"}},
		}
	}

	keys, ok = r.URL.Query()["search[phone]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["phone"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[email]"]
	if ok && len(keys[0]) >= 1 {
		criterias.SearchBy["email"] = map[string]interface{}{"$regex": keys[0], "$options": "i"}
	}

	keys, ok = r.URL.Query()["search[created_by]"]
	if ok && len(keys[0]) >= 1 {

		userIds := strings.Split(keys[0], ",")

		objecIds := []primitive.ObjectID{}

		for _, id := range userIds {
			userID, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return customers, criterias, err
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
			return customers, criterias, err
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
			return customers, criterias, err
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
			return customers, criterias, err
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

	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
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

		if _, ok := criterias.Select["updated_by_user.id"]; ok {
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
		return customers, criterias, errors.New("Error fetching Customers:" + err.Error())
	}
	if cur != nil {
		defer cur.Close(ctx)
	}

	for i := 0; cur != nil && cur.Next(ctx); i++ {
		err := cur.Err()
		if err != nil {
			return customers, criterias, errors.New("Cursor error:" + err.Error())
		}
		customer := Customer{}
		err = cur.Decode(&customer)
		if err != nil {
			return customers, criterias, errors.New("Cursor decode error:" + err.Error())
		}

		customer.SearchLabel = customer.Name

		if customer.NameInArabic != "" {
			customer.SearchLabel += " / " + customer.NameInArabic
		}

		if customer.Phone != "" {
			customer.SearchLabel += " " + customer.Phone
		}

		if customer.PhoneInArabic != "" {
			customer.SearchLabel += " / " + customer.PhoneInArabic
		}

		if _, ok := criterias.Select["created_by_user.id"]; ok {
			customer.CreatedByUser, _ = FindUserByID(customer.CreatedBy, createdByUserSelectFields)
		}
		if _, ok := criterias.Select["updated_by_user.id"]; ok {
			customer.UpdatedByUser, _ = FindUserByID(customer.UpdatedBy, updatedByUserSelectFields)
		}
		if _, ok := criterias.Select["deleted_by_user.id"]; ok {
			customer.DeletedByUser, _ = FindUserByID(customer.DeletedBy, deletedByUserSelectFields)
		}

		customers = append(customers, customer)
	} //end for loop

	return customers, criterias, nil

}

func (customer *Customer) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if customer.ID.IsZero() {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsCustomerExists(&customer.ID)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			errs["id"] = err.Error()
			return errs
		}

		if !exists {
			errs["id"] = "Invalid Customer:" + customer.ID.Hex()
		}

	}

	if govalidator.IsNull(customer.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(customer.Phone) {
		errs["phone"] = "Phone is required"
	}

	phoneExists, err := customer.IsPhoneExists()
	if err != nil {
		errs["phone"] = err.Error()
	}

	if phoneExists {
		errs["phone"] = "Phone No. Already exists."
	}

	if phoneExists {
		w.WriteHeader(http.StatusConflict)
	} else if len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
	}

	return errs
}

func (customer *Customer) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	customer.CreatedBy = &UserObject.ID
	customer.UpdatedBy = &UserObject.ID
	now := time.Now()
	customer.CreatedAt = &now
	customer.UpdatedAt = &now

	err := customer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	customer.SetChangeLog("create", nil, nil, nil)

	customer.ID = primitive.NewObjectID()
	_, err = collection.InsertOne(ctx, &customer)
	if err != nil {
		return err
	}

	return nil
}

func (customer *Customer) Update() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	customer.UpdatedBy = &UserObject.ID
	now := time.Now()
	customer.UpdatedAt = &now

	err := customer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	customer.SetChangeLog("update", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customer.ID},
		bson.M{"$set": customer},
		updateOptions,
	)
	if err != nil {
		return err
	}
	return nil
}

func (customer *Customer) DeleteCustomer(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	err = customer.UpdateForeignLabelFields()
	if err != nil {
		return err
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	customer.Deleted = true
	customer.DeletedBy = &userID
	now := time.Now()
	customer.DeletedAt = &now

	customer.SetChangeLog("delete", nil, nil, nil)

	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": customer.ID},
		bson.M{"$set": customer},
		updateOptions,
	)
	if err != nil {
		return err
	}

	return nil
}

func FindCustomerByID(
	ID *primitive.ObjectID,
	selectFields map[string]interface{},
) (customer *Customer, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	findOneOptions := options.FindOne()
	if len(selectFields) > 0 {
		findOneOptions.SetProjection(selectFields)
	}

	err = collection.FindOne(ctx,
		bson.M{"_id": ID}, findOneOptions). //"deleted": bson.M{"$ne": true}
		Decode(&customer)
	if err != nil {
		return nil, err
	}

	if _, ok := selectFields["created_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "created_by_user")
		customer.CreatedByUser, _ = FindUserByID(customer.CreatedBy, fields)
	}

	if _, ok := selectFields["updated_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "updated_by_user")
		customer.UpdatedByUser, _ = FindUserByID(customer.UpdatedBy, fields)
	}

	if _, ok := selectFields["deleted_by_user.id"]; ok {
		fields := ParseRelationalSelectString(selectFields, "deleted_by_user")
		customer.DeletedByUser, _ = FindUserByID(customer.DeletedBy, fields)
	}

	return customer, err
}

func (customer *Customer) IsEmailExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": customer.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": customer.Email,
			"_id":   bson.M{"$ne": customer.ID},
		})
	}

	return (count == 1), err
}

func (customer *Customer) IsPhoneExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	if customer.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone": customer.Phone,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"phone": customer.Phone,
			"_id":   bson.M{"$ne": customer.ID},
		})
	}

	return (count == 1), err
}

func IsCustomerExists(ID *primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
