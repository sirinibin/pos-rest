package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListQuotationSalesReturnPayment : handler for GET /quotationsalesreturnpayment
func ListQuotationSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturnpayments, criterias, err := store.SearchQuotationSalesReturnPayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find quotationsales return payments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "quotationsales_return_payment")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of quotationsalesreturnpayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturnpaymentStats, err := store.GetQuotationSalesReturnPaymentStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_payment"] = "Unable to find total amount of quotationsales return payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_payment"] = quotationsalesreturnpaymentStats.TotalPayment

	if len(quotationsalesreturnpayments) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = quotationsalesreturnpayments
	}

	json.NewEncoder(w).Encode(response)

}

// CreateQuotationSalesReturnPayment : handler for POST /quotationsalesreturnpayment
func CreateQuotationSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	var quotationsalesreturnpayment *models.QuotationSalesReturnPayment
	// Decode data
	if !utils.Decode(w, r, &quotationsalesreturnpayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturnpayment.CreatedBy = &userID
	quotationsalesreturnpayment.UpdatedBy = &userID
	now := time.Now()
	quotationsalesreturnpayment.CreatedAt = &now
	quotationsalesreturnpayment.UpdatedAt = &now

	// Validate data
	if errs := quotationsalesreturnpayment.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturnpayment.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
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

	//Updating quotationsalesReturn.payments
	quotationsalesReturn, _ := store.FindQuotationSalesReturnByID(quotationsalesreturnpayment.QuotationSalesReturnID, map[string]interface{}{})
	quotationsalesReturn.SetPaymentStatus()
	quotationsalesReturn.SetCustomerQuotationSalesReturnStats()
	quotationsalesReturn.Update()

	quotation, _ := store.FindQuotationByID(quotationsalesreturnpayment.QuotationID, bson.M{})
	quotation.ReturnAmount, quotation.ReturnCount, _ = store.GetReturnedAmountByQuotationID(quotation.ID)
	quotation.Update()

	err = quotationsalesReturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesReturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = quotationsalesreturnpayment

	json.NewEncoder(w).Encode(response)
}

// UpdateQuotationSalesReturnPayment : handler function for PUT /v1/quotationsalesreturnpayment call
func UpdateQuotationSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	var quotationsalesreturnpayment *models.QuotationSalesReturnPayment

	params := mux.Vars(r)

	quotationsalesreturnpaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid QuotationSalesReturnPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
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

	quotationsalesreturnpayment, err = store.FindQuotationSalesReturnPaymentByID(&quotationsalesreturnpaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &quotationsalesreturnpayment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturnpayment.UpdatedBy = &userID
	now := time.Now()
	quotationsalesreturnpayment.UpdatedAt = &now

	// Validate data
	if errs := quotationsalesreturnpayment.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturnpayment.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating quotationsalesReturn.payments
	quotationsalesReturn, _ := store.FindQuotationSalesReturnByID(quotationsalesreturnpayment.QuotationSalesReturnID, map[string]interface{}{})
	quotationsalesReturn.SetPaymentStatus()
	quotationsalesReturn.SetCustomerQuotationSalesReturnStats()
	quotationsalesReturn.Update()

	quotation, _ := store.FindQuotationByID(quotationsalesreturnpayment.QuotationID, bson.M{})
	quotation.ReturnAmount, quotation.ReturnCount, _ = store.GetReturnedAmountByQuotationID(quotation.ID)
	quotation.Update()

	err = quotationsalesReturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesReturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturnpayment, err = store.FindQuotationSalesReturnPaymentByID(&quotationsalesreturnpayment.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find quotationsalesreturn payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = quotationsalesreturnpayment
	json.NewEncoder(w).Encode(response)
}

// ViewQuotationSalesReturnPayment : handler function for GET /v1/quotationsalesreturnpayment/<id> call
func ViewQuotationSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	quotationsalesreturnpaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid QuotationSalesReturnPayment ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotationsalesreturnpayment *models.QuotationSalesReturnPayment

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

	quotationsalesreturnpayment, err = store.FindQuotationSalesReturnPaymentByID(&quotationsalesreturnpaymentID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = quotationsalesreturnpayment

	json.NewEncoder(w).Encode(response)
}

// DeleteQuotationSalesReturnPayment : handler function for DELETE /v1/quotationsales-return-payment/<id> call
func DeleteQuotationSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	quotationsalesReturnPaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotationsales_return_payment_id"] = "Invalid quotationsales return payment ID:" + err.Error()
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

	quotationsalesReturnPayment, err := store.FindQuotationSalesReturnPaymentByID(&quotationsalesReturnPaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Error finding quotationsales return payement: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesReturnPayment.Deleted = true
	quotationsalesReturnPayment.DeletedBy = &userID
	now := time.Now()
	quotationsalesReturnPayment.DeletedAt = &now

	err = quotationsalesReturnPayment.DeleteQuotationSalesReturnPayment()
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating quotationsalesReturn.payments
	if quotationsalesReturnPayment.QuotationSalesReturnID != nil {
		quotationsalesReturn, _ := store.FindQuotationSalesReturnByID(quotationsalesReturnPayment.QuotationSalesReturnID, map[string]interface{}{})
		quotationsalesReturn.SetPaymentStatus()
		quotationsalesReturn.SetCustomerQuotationSalesReturnStats()
		quotationsalesReturn.Update()
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)

}
