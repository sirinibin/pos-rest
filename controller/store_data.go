package controller

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PopulateStoreTestData : handler for POST /v1/store/{id}/populate-test-data
// Fills the store with automobile-workshop sample data (customers, vehicles,
// technicians, spare parts, services and repair jobs).
func PopulateStoreTestData(w http.ResponseWriter, r *http.Request) {
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

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
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
		response.Errors["view"] = "Unable to find store:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	summary, err := store.PopulateAutomobileTestData(&userID)
	if err != nil {
		response.Status = false
		response.Errors["populate"] = "Unable to populate test data:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = summary
	json.NewEncoder(w).Encode(response)
}

// ClearStoreData : handler for POST /v1/store/{id}/clear-data
// Drops all collections in the store's database (store_<id>), resets serial
// number counters, recreates indexes and re-posts opening balances.
// Admin only.
func ClearStoreData(w http.ResponseWriter, r *http.Request) {
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

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	accessingUser, err := models.FindUserByID(&userID, bson.M{})
	if err != nil || accessingUser.Role != "Admin" {
		response.Status = false
		response.Errors["role"] = "Only Admins can clear store data"
		w.WriteHeader(http.StatusForbidden)
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
		response.Errors["view"] = "Unable to find store:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	droppedCount, err := store.ClearStoreData()
	if err != nil {
		response.Status = false
		response.Errors["clear"] = "Unable to clear store data:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = map[string]interface{}{
		"dropped_collections": droppedCount,
	}
	json.NewEncoder(w).Encode(response)
}
