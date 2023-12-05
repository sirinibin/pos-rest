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

// ListPurchasePayment : handler for GET /purchasepayment
func ListPurchasePayment(w http.ResponseWriter, r *http.Request) {
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

	purchasepayments, criterias, err := models.SearchPurchasePayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchasepayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "purchase_payment")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchasepayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasepaymentStats, err := models.GetPurchasePaymentStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_payment"] = "Unable to find total amount of purchasepayment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_payment"] = purchasepaymentStats.TotalPayment

	if len(purchasepayments) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchasepayments
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchasePayment : handler for POST /purchasepayment
func CreatePurchasePayment(w http.ResponseWriter, r *http.Request) {
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

	var purchasepayment *models.PurchasePayment
	// Decode data
	if !utils.Decode(w, r, &purchasepayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasepayment.CreatedBy = &userID
	purchasepayment.UpdatedBy = &userID
	now := time.Now()
	purchasepayment.CreatedAt = &now
	purchasepayment.UpdatedAt = &now

	// Validate data
	if errs := purchasepayment.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasepayment.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating purchase.payments
	purchase, _ := models.FindPurchaseByID(purchasepayment.PurchaseID, map[string]interface{}{})
	purchase.GetPayments()
	purchase.Update()

	response.Status = true
	response.Result = purchasepayment

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchasePayment : handler function for PUT /v1/purchasepayment call
func UpdatePurchasePayment(w http.ResponseWriter, r *http.Request) {
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

	var purchasepayment *models.PurchasePayment

	params := mux.Vars(r)

	purchasepaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid PurchasePayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasepayment, err = models.FindPurchasePaymentByID(&purchasepaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchasepayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasepayment.UpdatedBy = &userID
	now := time.Now()
	purchasepayment.UpdatedAt = &now

	// Validate data
	if errs := purchasepayment.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasepayment.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating purchase.payments
	purchase, _ := models.FindPurchaseByID(purchasepayment.PurchaseID, map[string]interface{}{})
	purchase.GetPayments()
	purchase.Update()

	purchasepayment, err = models.FindPurchasePaymentByID(&purchasepayment.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchase payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasepayment
	json.NewEncoder(w).Encode(response)
}

// ViewPurchasePayment : handler function for GET /v1/purchasepayment/<id> call
func ViewPurchasePayment(w http.ResponseWriter, r *http.Request) {
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

	purchasepaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid PurchasePayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchasepayment *models.PurchasePayment

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	purchasepayment, err = models.FindPurchasePaymentByID(&purchasepaymentID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasepayment

	json.NewEncoder(w).Encode(response)
}
