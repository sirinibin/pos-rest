package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// ListStore : handler for GET /store
func ListStore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	stores := []models.Store{}

	stores, criterias, err := models.SearchStore(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find stores:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "store")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of stores:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(stores) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = stores
	}

	json.NewEncoder(w).Encode(response)

}

// CreateStore : handler for POST /store
func CreateStore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var store *models.Store
	// Decode data
	if !utils.Decode(w, r, &store) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	accessingUser, err := models.FindUserByID(&userID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "invalid user: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if accessingUser.Role != "Admin" {
		response.Status = false
		response.Errors["user_id"] = "unauthorized access"
		json.NewEncoder(w).Encode(response)
		return
	}

	store.CreatedBy = &userID
	store.UpdatedBy = &userID
	now := time.Now()
	store.CreatedAt = &now
	store.UpdatedAt = &now

	// Validate data
	if errs := store.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = store.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	_, err = store.CreateDB()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["create_store_db"] = "error creating store db: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = store

	json.NewEncoder(w).Encode(response)

}

func ObjectIDExists(id primitive.ObjectID, list []*primitive.ObjectID) bool {
	for _, v := range list {
		if *v == id {
			return true
		}
	}
	return false
}

// UpdateStore : handler function for PUT /v1/store call
func UpdateStore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var store *models.Store
	var storeOld *models.Store

	params := mux.Vars(r)

	storeID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	storeOld, err = models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err = models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &store) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	accessingUser, err := models.FindUserByID(&userID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "invalid user: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !ObjectIDExists(store.ID, accessingUser.StoreIDs) && accessingUser.Role != "Admin" {
		response.Status = false
		response.Errors["user_id"] = "unauthorized access"
		json.NewEncoder(w).Encode(response)
		return
	}

	store.UpdatedBy = &userID
	now := time.Now()
	store.UpdatedAt = &now

	// Validate data
	if errs := store.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = store.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = store.AttributesValueChangeEvent(storeOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err = models.FindStoreByID(&store.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find store:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = store

	json.NewEncoder(w).Encode(response)
}

// ViewStore : handler function for GET /v1/store/<id> call
func ViewStore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	storeID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var store *models.Store

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err = models.FindStoreByID(&storeID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	accessingUser, err := models.FindUserByID(&userID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "invalid user: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !ObjectIDExists(store.ID, accessingUser.StoreIDs) && accessingUser.Role != "Admin" {
		response.Status = false
		response.Errors["user_id"] = "unauthorized access"
		json.NewEncoder(w).Encode(response)
		return
	}

	//store.MarshalJSON()
	response.Status = true
	response.Result = store

	json.NewEncoder(w).Encode(response)

}

// DeleteStore : handler function for DELETE /v1/store/<id> call
func DeleteStore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	storeID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = store.DeleteStore(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)

}
