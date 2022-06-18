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

// ListCustomerDeposit : handler for GET /customerdeposit
func ListCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	customerdeposits := []models.CustomerDeposit{}

	customerdeposits, criterias, err := models.SearchCustomerDeposit(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find customerdeposits:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "customerdeposit")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of customerdeposits:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdepositStats, err := models.GetCustomerDepositStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total"] = "Unable to find total amount of customerdeposits:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total"] = customerdepositStats.Total

	if len(customerdeposits) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = customerdeposits
	}

	json.NewEncoder(w).Encode(response)

}

// CreateCustomerDeposit : handler for POST /customerdeposit
func CreateCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	var customerdeposit *models.CustomerDeposit
	// Decode data
	if !utils.Decode(w, r, &customerdeposit) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit.CreatedBy = &userID
	customerdeposit.UpdatedBy = &userID
	now := time.Now()
	customerdeposit.CreatedAt = &now
	customerdeposit.UpdatedAt = &now

	// Validate data
	if errs := customerdeposit.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerdeposit

	json.NewEncoder(w).Encode(response)
}

// UpdateCustomerDeposit : handler function for PUT /v1/customerdeposit call
func UpdateCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	var customerdeposit *models.CustomerDeposit
	var customerdepositOld *models.CustomerDeposit

	params := mux.Vars(r)

	customerdepositID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit_id"] = "Invalid CustomerDeposit ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdepositOld, err = models.FindCustomerDepositByID(&customerdepositID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit"] = "Unable to find customerdeposit:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit, err = models.FindCustomerDepositByID(&customerdepositID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit"] = "Unable to find customerdeposit:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &customerdeposit) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit.UpdatedBy = &userID
	now := time.Now()
	customerdeposit.UpdatedAt = &now

	// Validate data
	if errs := customerdeposit.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.AttributesValueChangeEvent(customerdepositOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit, err = models.FindCustomerDepositByID(&customerdeposit.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find customerdeposit:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerdeposit
	json.NewEncoder(w).Encode(response)
}

// ViewCustomerDeposit : handler function for GET /v1/customerdeposit/<id> call
func ViewCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	customerdepositID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Errors["customerdeposit_id"] = "Invalid CustomerDeposit ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var customerdeposit *models.CustomerDeposit

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	customerdeposit, err = models.FindCustomerDepositByID(&customerdepositID, selectFields)
	if err != nil {
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerdeposit

	json.NewEncoder(w).Encode(response)
}

// ViewCustomerDeposit : handler function for GET /v1/customerdeposit/code/<code> call
func ViewCustomerDepositByCode(w http.ResponseWriter, r *http.Request) {
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

	var customerdeposit *models.CustomerDeposit

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	customerdeposit, err = models.FindCustomerDepositByCode(code, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerdeposit

	json.NewEncoder(w).Encode(response)
}

// DeleteCustomerDeposit : handler function for DELETE /v1/customerdeposit/<id> call
func DeleteCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	customerdepositID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit_id"] = "Invalid CustomerDeposit ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit, err := models.FindCustomerDepositByID(&customerdepositID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.DeleteCustomerDeposit(tokenClaims)
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
