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

// ListPurchaseReturnPayment : handler for GET /purchasereturnpayment
func ListPurchaseReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	purchasereturnpayments, criterias, err := models.SearchPurchaseReturnPayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase return payments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "purchase_return_payment")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchasereturnpayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturnpaymentStats, err := models.GetPurchaseReturnPaymentStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_payment"] = "Unable to find total amount of purchase return payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_payment"] = purchasereturnpaymentStats.TotalPayment

	if len(purchasereturnpayments) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchasereturnpayments
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchaseReturnPayment : handler for POST /purchasereturnpayment
func CreatePurchaseReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	var purchasereturnpayment *models.PurchaseReturnPayment
	// Decode data
	if !utils.Decode(w, r, &purchasereturnpayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturnpayment.CreatedBy = &userID
	purchasereturnpayment.UpdatedBy = &userID
	now := time.Now()
	purchasereturnpayment.CreatedAt = &now
	purchasereturnpayment.UpdatedAt = &now

	// Validate data
	if errs := purchasereturnpayment.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturnpayment.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasereturnpayment

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchaseReturnPayment : handler function for PUT /v1/purchasereturnpayment call
func UpdatePurchaseReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	var purchasereturnpayment *models.PurchaseReturnPayment

	params := mux.Vars(r)

	purchasereturnpaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid PurchaseReturnPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturnpayment, err = models.FindPurchaseReturnPaymentByID(&purchasereturnpaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchasereturnpayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturnpayment.UpdatedBy = &userID
	now := time.Now()
	purchasereturnpayment.UpdatedAt = &now

	// Validate data
	if errs := purchasereturnpayment.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturnpayment.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturnpayment, err = models.FindPurchaseReturnPaymentByID(&purchasereturnpayment.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchasereturn payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasereturnpayment
	json.NewEncoder(w).Encode(response)
}

// ViewPurchaseReturnPayment : handler function for GET /v1/purchasereturnpayment/<id> call
func ViewPurchaseReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	purchasereturnpaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Purchase Return Payment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchasereturnpayment *models.PurchaseReturnPayment

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	purchasereturnpayment, err = models.FindPurchaseReturnPaymentByID(&purchasereturnpaymentID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasereturnpayment

	json.NewEncoder(w).Encode(response)
}
