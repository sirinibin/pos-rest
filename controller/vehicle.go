package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListVehicle : handler for GET /vehicle
func ListVehicle(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vehicles, criterias, err := store.SearchVehicle(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find vehicles:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "vehicle")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of vehicles:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(vehicles) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = vehicles
	}

	json.NewEncoder(w).Encode(response)
}

// CreateVehicle : handler for POST /vehicle
func CreateVehicle(w http.ResponseWriter, r *http.Request) {
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

	var vehicle *models.Vehicle
	if !utils.Decode(w, r, &vehicle) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vehicle.CreatedBy = &userID
	vehicle.UpdatedBy = &userID
	now := time.Now()
	vehicle.CreatedAt = &now
	vehicle.UpdatedAt = &now

	if errs := vehicle.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := vehicle.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := vehicle.Insert(); err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to insert vehicle:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = vehicle
	json.NewEncoder(w).Encode(response)
}

// ViewVehicle : handler for GET /vehicle/{id}
func ViewVehicle(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vehicle, err := store.FindVehicleByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find vehicle:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = vehicle
	json.NewEncoder(w).Encode(response)
}

// UpdateVehicle : handler for PUT /vehicle/{id}
func UpdateVehicle(w http.ResponseWriter, r *http.Request) {
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
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vehicle, err := store.FindVehicleByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find vehicle:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &vehicle) {
		return
	}

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	vehicle.UpdatedBy = &userID

	if errs := vehicle.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := vehicle.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := vehicle.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update vehicle:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = vehicle
	json.NewEncoder(w).Encode(response)
}

// DeleteVehicle : handler for DELETE /vehicle/{id}
func DeleteVehicle(w http.ResponseWriter, r *http.Request) {
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
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vehicle, err := store.FindVehicleByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find vehicle:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := vehicle.Delete(tokenClaims); err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete vehicle:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"
	json.NewEncoder(w).Encode(response)
}

// ListVehicleBrands : handler for GET /vehicle-brands — returns KSA vehicle brands static list
func ListVehicleBrands(w http.ResponseWriter, r *http.Request) {
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

	brands := models.GetKSAVehicleBrands()
	response.Status = true
	response.Result = brands
	json.NewEncoder(w).Encode(response)
}
