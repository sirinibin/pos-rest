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

// ListCustomerWithdrawal : handler for GET /customerwithdrawal
func ListCustomerWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	customerwithdrawals := []models.CustomerWithdrawal{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerwithdrawals, criterias, err := store.SearchCustomerWithdrawal(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find customerwithdrawals:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "customerwithdrawal")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of customerwithdrawals:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerwithdrawalStats, err := store.GetCustomerWithdrawalStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total"] = "Unable to find total amount of customerwithdrawals:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total"] = customerwithdrawalStats.Total

	if len(customerwithdrawals) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = customerwithdrawals
	}

	json.NewEncoder(w).Encode(response)

}

// CreateCustomerWithdrawal : handler for POST /customerwithdrawal
func CreateCustomerWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	var customerwithdrawal *models.CustomerWithdrawal
	// Decode data
	if !utils.Decode(w, r, &customerwithdrawal) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerwithdrawal.CreatedBy = &userID
	customerwithdrawal.UpdatedBy = &userID
	now := time.Now()
	customerwithdrawal.CreatedAt = &now
	customerwithdrawal.UpdatedAt = &now

	// Validate data
	if errs := customerwithdrawal.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error doing accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerwithdrawal

	json.NewEncoder(w).Encode(response)
}

// UpdateCustomerWithdrawal : handler function for PUT /v1/customerwithdrawal call
func UpdateCustomerWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	var customerwithdrawal *models.CustomerWithdrawal
	var customerwithdrawalOld *models.CustomerWithdrawal

	params := mux.Vars(r)

	customerwithdrawalID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customerwithdrawal_id"] = "Invalid CustomerWithdrawal ID:" + err.Error()
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

	customerwithdrawalOld, err = store.FindCustomerWithdrawalByID(&customerwithdrawalID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["customerwithdrawal"] = "Unable to find customerwithdrawal:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customerwithdrawal, err = store.FindCustomerWithdrawalByID(&customerwithdrawalID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["customerwithdrawal"] = "Unable to find customerwithdrawal:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &customerwithdrawal) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerwithdrawal.UpdatedBy = &userID
	now := time.Now()
	customerwithdrawal.UpdatedAt = &now

	// Validate data
	if errs := customerwithdrawal.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.AttributesValueChangeEvent(customerwithdrawalOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	customerwithdrawal, err = store.FindCustomerWithdrawalByID(&customerwithdrawal.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find customerwithdrawal:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error doing accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerwithdrawal
	json.NewEncoder(w).Encode(response)
}

// ViewCustomerWithdrawal : handler function for GET /v1/customerwithdrawal/<id> call
func ViewCustomerWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	customerwithdrawalID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Errors["customerwithdrawal_id"] = "Invalid CustomerWithdrawal ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var customerwithdrawal *models.CustomerWithdrawal

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

	customerwithdrawal, err = store.FindCustomerWithdrawalByID(&customerwithdrawalID, selectFields)
	if err != nil {
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, _ := store.FindCustomerByID(customerwithdrawal.CustomerID, bson.M{})
	customer.SetSearchLabel()
	customerwithdrawal.Customer = customer

	response.Status = true
	response.Result = customerwithdrawal

	json.NewEncoder(w).Encode(response)
}

// ViewCustomerWithdrawal : handler function for GET /v1/customerwithdrawal/code/<code> call
func ViewCustomerWithdrawalByCode(w http.ResponseWriter, r *http.Request) {
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

	var customerwithdrawal *models.CustomerWithdrawal

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

	customerwithdrawal, err = store.FindCustomerWithdrawalByCode(code, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerwithdrawal

	json.NewEncoder(w).Encode(response)
}

// DeleteCustomerWithdrawal : handler function for DELETE /v1/customerwithdrawal/<id> call
func DeleteCustomerWithdrawal(w http.ResponseWriter, r *http.Request) {
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

	customerwithdrawalID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customerwithdrawal_id"] = "Invalid CustomerWithdrawal ID:" + err.Error()
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

	customerwithdrawal, err := store.FindCustomerWithdrawalByID(&customerwithdrawalID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerwithdrawal.DeleteCustomerWithdrawal(tokenClaims)
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
