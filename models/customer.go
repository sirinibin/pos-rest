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
}

func SearchCustomer(w http.ResponseWriter, r *http.Request) (customers []Customer, criterias SearchCriterias, err error) {

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
	customer.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &customer)
	if err != nil {
		return err
	}
	return nil
}

func (customer *Customer) Update() (*Customer, error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()
	updateResult, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": customer.ID},
		bson.M{"$set": customer},
		updateOptions,
	)
	if err != nil {
		return nil, err
	}

	if updateResult.MatchedCount > 0 {
		return customer, nil
	}
	return nil, nil
}

func (customer *Customer) DeleteCustomer(tokenClaims TokenClaims) (err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	updateOptions := options.Update()
	updateOptions.SetUpsert(true)
	defer cancel()

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		return err
	}

	customer.Deleted = true
	customer.DeletedBy = &userID
	now := time.Now().Local()
	customer.DeletedAt = &now

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
