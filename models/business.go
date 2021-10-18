package models

import (
	"context"
	"net/http"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

//Business : Business structure
type Business struct {
	ID              primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name            string             `bson:"name" json:"name"`
	NameInArabic    string             `bson:"name_in_arabic" json:"name_in_arabic"`
	Title           string             `bson:"title" json:"title"`
	TitleInArabic   string             `bson:"title_in_arabic" json:"title_in_arabic"`
	Email           string             `bson:"email" json:"email"`
	Phone           string             `bson:"phone" json:"phone"`
	PhoneInArabic   string             `bson:"phone_in_arabic" json:"phone_in_arabic"`
	Address         string             `bson:"address" json:"address"`
	AddressInArabic string             `bson:"address_in_arabic" json:"address_in_arabic"`
	VATNo           string             `bson:"vat_no" json:"vat_no"`
	VATNoInArabic   string             `bson:"vat_no_in_arabic" json:"vat_no_in_arabic"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
	CreatedBy       primitive.ObjectID `json:"created_by,omitempty" bson:"created_by,omitempty"`
	UpdatedBy       primitive.ObjectID `json:"updated_by,omitempty" bson:"updated_by,omitempty"`
}

func (business *Business) Validate(w http.ResponseWriter, r *http.Request, scenario string) (errs map[string]string) {

	errs = make(map[string]string)

	if scenario == "update" {
		if business.ID.IsZero() {
			errs["id"] = "ID is required"
			return errs
		}
		exists, err := IsBusinessExists(business.ID)
		if err != nil || !exists {
			errs["id"] = err.Error()
			return errs
		}

	}

	if govalidator.IsNull(business.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(business.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(business.Address) {
		errs["address"] = "Address is required"
	}

	if govalidator.IsNull(business.Phone) {
		errs["phone"] = "Phone is required"
	}

	emailExists, err := business.IsEmailExists()
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

func (business *Business) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("business")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	business.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &business)
	if err != nil {
		return err
	}
	return nil
}

func (business *Business) IsEmailExists() (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("business")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"email": business.Email,
	})

	return (count == 1), err
}

func IsBusinessExists(ID primitive.ObjectID) (exists bool, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("business")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)

	count, err = collection.CountDocuments(ctx, bson.M{
		"_id": ID,
	})

	return (count == 1), err
}
