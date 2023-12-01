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

// ListSalesReturnPayment : handler for GET /salesreturnpayment
func ListSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)
	response.Meta = make(map[string]interface{})

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayments, criterias, err := models.SearchSalesReturnPayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find sales return payments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "sales_return_payment")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of salesreturnpayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpaymentStats, err := models.GetSalesReturnPaymentStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_payment"] = "Unable to find total amount of sales return payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_payment"] = salesreturnpaymentStats.TotalPayment

	if len(salesreturnpayments) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = salesreturnpayments
	}

	json.NewEncoder(w).Encode(response)

}

// CreateSalesReturnPayment : handler for POST /salesreturnpayment
func CreateSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	var salesreturnpayment *models.SalesReturnPayment
	// Decode data
	if !utils.Decode(w, r, &salesreturnpayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayment.CreatedBy = &userID
	salesreturnpayment.UpdatedBy = &userID
	now := time.Now()
	salesreturnpayment.CreatedAt = &now
	salesreturnpayment.UpdatedAt = &now

	// Validate data
	if errs := salesreturnpayment.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturnpayment.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating salesReturn.payments
	salesReturn, _ := models.FindSalesReturnByID(salesreturnpayment.SalesReturnID, map[string]interface{}{})
	salesReturn.GetPayments()
	salesReturn.Update()

	response.Status = true
	response.Result = salesreturnpayment

	json.NewEncoder(w).Encode(response)
}

// UpdateSalesReturnPayment : handler function for PUT /v1/salesreturnpayment call
func UpdateSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	var salesreturnpayment *models.SalesReturnPayment

	params := mux.Vars(r)

	salesreturnpaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesReturnPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayment, err = models.FindSalesReturnPaymentByID(&salesreturnpaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &salesreturnpayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayment.UpdatedBy = &userID
	now := time.Now()
	salesreturnpayment.UpdatedAt = &now

	// Validate data
	if errs := salesreturnpayment.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturnpayment.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating salesReturn.payments
	salesReturn, _ := models.FindSalesReturnByID(salesreturnpayment.SalesReturnID, map[string]interface{}{})
	salesReturn.GetPayments()
	salesReturn.Update()

	salesreturnpayment, err = models.FindSalesReturnPaymentByID(&salesreturnpayment.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find salesreturn payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salesreturnpayment
	json.NewEncoder(w).Encode(response)
}

// ViewSalesReturnPayment : handler function for GET /v1/salesreturnpayment/<id> call
func ViewSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	salesreturnpaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesReturnPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var salesreturnpayment *models.SalesReturnPayment

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	salesreturnpayment, err = models.FindSalesReturnPaymentByID(&salesreturnpaymentID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salesreturnpayment

	json.NewEncoder(w).Encode(response)
}
