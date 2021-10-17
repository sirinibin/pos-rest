package models

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jameskeane/bcrypt"
	"github.com/sirinibin/pos-rest/db"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
) //import "encoding/json"

type User struct {
	ID        primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	Name      string             `bson:"name" json:"name"`
	Email     string             `bson:"email" json:"email"`
	Mob       string             `bson:"mob" json:"mob"`
	Password  string             `bson:"password" json:"password,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

func FindUserByEmail(email string) (user *User, err error) {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"email": email}).
		Decode(&user)
	if err != nil {
		return nil, err
	}
	return user, err
}

func FindUserByID(userID primitive.ObjectID) (user *User, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = collection.FindOne(ctx,
		bson.M{"_id": userID}).
		Decode(&user)
	if err != nil {
		return nil, err
	}
	return user, err
}

func (user *User) IsEmailExists() (exists bool, err error) {

	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	count := int64(0)
	if user.ID.IsZero() {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": user.Email,
		})
	} else {
		count, err = collection.CountDocuments(ctx, bson.M{
			"email": user.Email,
			"_id":   bson.M{"$ne": user.ID},
		})
	}

	return (count == 1), err
}

func (user *User) Insert() error {
	collection := db.Client().Database(db.GetPosDB()).Collection("user")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user.ID = primitive.NewObjectID()
	_, err := collection.InsertOne(ctx, &user)
	if err != nil {
		return err
	}
	return nil
}

func (user *User) Validate(w http.ResponseWriter, r *http.Request) (errs map[string]string) {

	errs = make(map[string]string)

	if govalidator.IsNull(user.Name) {
		errs["name"] = "Name is required"
	}

	if govalidator.IsNull(user.Email) {
		errs["email"] = "E-mail is required"
	}

	if govalidator.IsNull(user.Mob) {
		errs["mob"] = "Mob is required"
	}

	if govalidator.IsNull(user.Password) {
		errs["password"] = "Password is required"
	}

	emailExists, err := user.IsEmailExists()
	if err != nil && err != sql.ErrNoRows {
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

func HashPassword(password string) string {
	salt, _ := bcrypt.Salt(10)
	hash, _ := bcrypt.Hash(password, salt)
	return hash
}
