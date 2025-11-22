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

// ListWarehouse : handler for GET /warehouse
func ListWarehouse(w http.ResponseWriter, r *http.Request) {
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
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouses := []models.Warehouse{}

	warehouses, criterias, err := store.SearchWarehouse(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find warehouses:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "warehouse")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of warehouses:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(warehouses) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = warehouses
	}

	json.NewEncoder(w).Encode(response)

}

// CreateWarehouse : handler for POST /warehouse
func CreateWarehouse(w http.ResponseWriter, r *http.Request) {
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

	var warehouse *models.Warehouse
	// Decode data
	if !utils.Decode(w, r, &warehouse) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouse.CreatedBy = &userID
	warehouse.UpdatedBy = &userID
	now := time.Now()
	warehouse.CreatedAt = &now
	warehouse.UpdatedAt = &now

	// Validate data
	if errs := warehouse.Validate(w, r, "create"); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = warehouse.MakeCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error making code: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = warehouse.Insert()
	if err != nil {
		redisErr := warehouse.UnMakeCode()
		if redisErr != nil {
			response.Errors["error_unmaking_code"] = "error_unmaking_code: " + redisErr.Error()
		}

		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = warehouse

	json.NewEncoder(w).Encode(response)

}

// UpdateWarehouse : handler function for PUT /v1/warehouse call
func UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var warehouse *models.Warehouse
	var warehouseOld *models.Warehouse

	params := mux.Vars(r)

	warehouseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["warehouse_id"] = "Invalid Warehouse ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouseOld, err = store.FindWarehouseByID(&warehouseID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouse, err = store.FindWarehouseByID(&warehouseID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &warehouse) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouse.UpdatedBy = &userID
	now := time.Now()
	warehouse.UpdatedAt = &now

	// Validate data
	if errs := warehouse.Validate(w, r, "update"); len(errs) > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = warehouse.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = warehouse.AttributesValueChangeEvent(warehouseOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouse, err = store.FindWarehouseByID(&warehouse.ID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["view"] = "Unable to find warehouse:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = warehouse

	json.NewEncoder(w).Encode(response)
}

// ViewWarehouse : handler function for GET /v1/warehouse/<id> call
func ViewWarehouse(w http.ResponseWriter, r *http.Request) {
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

	warehouseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["warehouse_id"] = "Invalid Warehouse ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var warehouse *models.Warehouse

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	warehouse, err = store.FindWarehouseByID(&warehouseID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	//warehouse.MarshalJSON()
	response.Status = true
	response.Result = warehouse

	json.NewEncoder(w).Encode(response)

}

// DeleteWarehouse : handler function for DELETE /v1/warehouse/<id> call
func DeleteWarehouse(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	warehouseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["warehouse_id"] = "Invalid Warehouse ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	warehouse, err := store.FindWarehouseByID(&warehouseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = warehouse.DeleteWarehouse(tokenClaims)
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
