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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayments, criterias, err := store.SearchSalesReturnPayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find sales return payments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "sales_return_payment")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of salesreturnpayments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpaymentStats, err := store.GetSalesReturnPaymentStats(criterias.SearchBy)
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
		w.WriteHeader(http.StatusBadRequest)
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating salesReturn.payments
	salesReturn, _ := store.FindSalesReturnByID(salesreturnpayment.SalesReturnID, map[string]interface{}{})
	salesReturn.SetPaymentStatus()
	salesReturn.SetCustomerSalesReturnStats()
	salesReturn.Update()

	order, _ := store.FindOrderByID(salesreturnpayment.OrderID, bson.M{})
	order.ReturnAmount, order.ReturnCount, _ = store.GetReturnedAmountByOrderID(order.ID)
	order.Update()

	err = salesReturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesReturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayment, err = store.FindSalesReturnPaymentByID(&salesreturnpaymentID, bson.M{})
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
		w.WriteHeader(http.StatusBadRequest)
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
	salesReturn, _ := store.FindSalesReturnByID(salesreturnpayment.SalesReturnID, map[string]interface{}{})
	salesReturn.SetPaymentStatus()
	salesReturn.SetCustomerSalesReturnStats()
	salesReturn.Update()

	order, _ := store.FindOrderByID(salesreturnpayment.OrderID, bson.M{})
	order.ReturnAmount, order.ReturnCount, _ = store.GetReturnedAmountByOrderID(order.ID)
	order.Update()

	err = salesReturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesReturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayment, err = store.FindSalesReturnPaymentByID(&salesreturnpayment.ID, bson.M{})
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnpayment, err = store.FindSalesReturnPaymentByID(&salesreturnpaymentID, selectFields)
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

// DeleteSalesReturnPayment : handler function for DELETE /v1/sales-return-payment/<id> call
func DeleteSalesReturnPayment(w http.ResponseWriter, r *http.Request) {
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

	salesReturnPaymentID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["sales_return_payment_id"] = "Invalid sales return payment ID:" + err.Error()
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

	salesReturnPayment, err := store.FindSalesReturnPaymentByID(&salesReturnPaymentID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Error finding sales return payement: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesReturnPayment.Deleted = true
	salesReturnPayment.DeletedBy = &userID
	now := time.Now()
	salesReturnPayment.DeletedAt = &now

	err = salesReturnPayment.DeleteSalesReturnPayment()
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	//Updating salesReturn.payments
	if salesReturnPayment.SalesReturnID != nil {
		salesReturn, _ := store.FindSalesReturnByID(salesReturnPayment.SalesReturnID, map[string]interface{}{})
		salesReturn.SetPaymentStatus()
		salesReturn.SetCustomerSalesReturnStats()
		salesReturn.Update()
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)

}
