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

// ListSalesPayment : handler for GET /salespayment
func ListSalesPayment(w http.ResponseWriter, r *http.Request) {
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

	salespayments, criterias, err := models.SearchSalesPayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salespayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "sales_payment")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of salespayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salespaymentStats, err := models.GetSalesPaymentStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_payment"] = "Unable to find total amount of salespayment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_payment"] = salespaymentStats.TotalPayment

	if len(salespayments) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = salespayments
	}

	json.NewEncoder(w).Encode(response)

}

// CreateSalesPayment : handler for POST /salespayment
func CreateSalesPayment(w http.ResponseWriter, r *http.Request) {
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

	var salespayment *models.SalesPayment
	// Decode data
	if !utils.Decode(w, r, &salespayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salespayment.CreatedBy = &userID
	salespayment.UpdatedBy = &userID
	now := time.Now()
	salespayment.CreatedAt = &now
	salespayment.UpdatedAt = &now

	// Validate data
	if errs := salespayment.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salespayment.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating order.payments
	order, _ := models.FindOrderByID(salespayment.OrderID, map[string]interface{}{})
	order.GetPayments()
	order.SetCustomerSalesStats()
	order.Update()

	response.Status = true
	response.Result = salespayment

	json.NewEncoder(w).Encode(response)
}

// UpdateSalesPayment : handler function for PUT /v1/salespayment call
func UpdateSalesPayment(w http.ResponseWriter, r *http.Request) {
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

	var salespayment *models.SalesPayment

	params := mux.Vars(r)

	salespaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salespayment, err = models.FindSalesPaymentByID(&salespaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &salespayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salespayment.UpdatedBy = &userID
	now := time.Now()
	salespayment.UpdatedAt = &now

	// Validate data
	if errs := salespayment.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salespayment.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating order.payments
	order, _ := models.FindOrderByID(salespayment.OrderID, map[string]interface{}{})
	order.GetPayments()
	order.SetCustomerSalesStats()
	order.Update()

	salespayment, err = models.FindSalesPaymentByID(&salespayment.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find sales payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salespayment
	json.NewEncoder(w).Encode(response)
}

// ViewSalesPayment : handler function for GET /v1/salespayment/<id> call
func ViewSalesPayment(w http.ResponseWriter, r *http.Request) {
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

	salespaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var salespayment *models.SalesPayment

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	salespayment, err = models.FindSalesPaymentByID(&salespaymentID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salespayment

	json.NewEncoder(w).Encode(response)
}
