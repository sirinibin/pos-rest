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

// ListCapitalWithdrawal : handler for GET /capitalwithdrawal
func ListCapitalWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	capitalwithdrawals := []models.CapitalWithdrawal{}

	capitalwithdrawals, criterias, err := store.SearchCapitalWithdrawal(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find capitalwithdrawals:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "capitalwithdrawal")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of capitalwithdrawals:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	capitalwithdrawalStats, err := store.GetCapitalWithdrawalStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total"] = "Unable to find total amount of capitalwithdrawals:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total"] = capitalwithdrawalStats.Total

	if len(capitalwithdrawals) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = capitalwithdrawals
	}

	json.NewEncoder(w).Encode(response)

}

// CreateCapitalWithdrawal : handler for POST /capitalwithdrawal
func CreateCapitalWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	var capitalwithdrawal *models.CapitalWithdrawal
	// Decode data
	if !utils.Decode(w, r, &capitalwithdrawal) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	capitalwithdrawal.CreatedBy = &userID
	capitalwithdrawal.UpdatedBy = &userID
	now := time.Now()
	capitalwithdrawal.CreatedAt = &now
	capitalwithdrawal.UpdatedAt = &now

	// Validate data
	if errs := capitalwithdrawal.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = capitalwithdrawal.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = capitalwithdrawal

	json.NewEncoder(w).Encode(response)
}

// UpdateCapitalWithdrawal : handler function for PUT /v1/capitalwithdrawal call
func UpdateCapitalWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	var capitalwithdrawal *models.CapitalWithdrawal
	var capitalwithdrawalOld *models.CapitalWithdrawal

	params := mux.Vars(r)

	capitalwithdrawalID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["capitalwithdrawal_id"] = "Invalid CapitalWithdrawal ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &capitalwithdrawal) {
		return
	}

	var store *models.Store

	if capitalwithdrawal.StoreID.IsZero() {
		response.Status = false
		response.Errors["store_id"] = "store id is required: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	} else {
		store, err = models.FindStoreByID(capitalwithdrawal.StoreID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["store"] = "invalid store: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	capitalwithdrawalOld, err = store.FindCapitalWithdrawalByID(&capitalwithdrawalID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["capitalwithdrawal"] = "Unable to find capitalwithdrawal:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	capitalwithdrawal, err = store.FindCapitalWithdrawalByID(&capitalwithdrawalID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["capitalwithdrawal"] = "Unable to find capitalwithdrawal:" + err.Error()
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

	capitalwithdrawal.UpdatedBy = &userID
	now := time.Now()
	capitalwithdrawal.UpdatedAt = &now

	// Validate data
	if errs := capitalwithdrawal.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = capitalwithdrawal.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = capitalwithdrawal.AttributesValueChangeEvent(capitalwithdrawalOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	capitalwithdrawal, err = store.FindCapitalWithdrawalByID(&capitalwithdrawal.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find capitalwithdrawal:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = capitalwithdrawal
	json.NewEncoder(w).Encode(response)
}

// ViewCapitalWithdrawal : handler function for GET /v1/capitalwithdrawal/<id> call
func ViewCapitalWithdrawal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Status = false
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	capitalwithdrawalID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Errors["capitalwithdrawal_id"] = "Invalid CapitalWithdrawal ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var capitalwithdrawal *models.CapitalWithdrawal

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	capitalwithdrawal, err = store.FindCapitalWithdrawalByID(&capitalwithdrawalID, selectFields)
	if err != nil {
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = capitalwithdrawal

	json.NewEncoder(w).Encode(response)
}

// ViewCapitalWithdrawal : handler function for GET /v1/capitalwithdrawal/code/<code> call
func ViewCapitalWithdrawalByCode(w http.ResponseWriter, r *http.Request) {
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

	code := params["code"]
	if code == "" {
		response.Status = false
		response.Errors["code"] = "Invalid Code:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var capitalwithdrawal *models.CapitalWithdrawal

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	capitalwithdrawal, err = store.FindCapitalWithdrawalByCode(code, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = capitalwithdrawal

	json.NewEncoder(w).Encode(response)
}

// DeleteCapitalWithdrawal : handler function for DELETE /v1/capitalwithdrawal/<id> call
func DeleteCapitalWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	capitalwithdrawalID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["capitalwithdrawal_id"] = "Invalid CapitalWithdrawal ID:" + err.Error()
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

	capitalwithdrawal, err := store.FindCapitalWithdrawalByID(&capitalwithdrawalID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = capitalwithdrawal.DeleteCapitalWithdrawal(tokenClaims)
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
