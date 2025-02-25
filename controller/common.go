package controller

import (
	"errors"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

func ParseStore(r *http.Request) (store *models.Store, err error) {
	var storeID primitive.ObjectID

	keys, ok := r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("store_id is required")
	}

	if storeID.IsZero() {
		return nil, errors.New("invalid store id(parsing): " + err.Error())
	} else {
		store, err = models.FindStoreByID(&storeID, bson.M{})
		if err != nil {
			return nil, errors.New("error finding store: " + err.Error())
		}
	}

	return store, nil
}
