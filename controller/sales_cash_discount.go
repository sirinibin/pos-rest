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

// ListSalesCashDiscount : handler for GET /salescashdiscount
func ListSalesCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	salescashdiscounts, criterias, err := models.SearchSalesCashDiscount(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salescashdiscounts:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "sales_cash_discount")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of salescashdiscounts:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salescashdiscountStats, err := models.GetSalesCashDiscountStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_cash_discount"] = "Unable to find total amount of salescashdiscount:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_cash_discount"] = salescashdiscountStats.TotalCashDiscount

	if len(salescashdiscounts) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = salescashdiscounts
	}

	json.NewEncoder(w).Encode(response)

}

// CreateSalesCashDiscount : handler for POST /salescashdiscount
func CreateSalesCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	var salescashdiscount *models.SalesCashDiscount
	// Decode data
	if !utils.Decode(w, r, &salescashdiscount) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salescashdiscount.CreatedBy = &userID
	salescashdiscount.UpdatedBy = &userID
	now := time.Now()
	salescashdiscount.CreatedAt = &now
	salescashdiscount.UpdatedAt = &now

	// Validate data
	if errs := salescashdiscount.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salescashdiscount.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salescashdiscount

	json.NewEncoder(w).Encode(response)
}

// UpdateSalesCashDiscount : handler function for PUT /v1/salescashdiscount call
func UpdateSalesCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	var salescashdiscount *models.SalesCashDiscount

	params := mux.Vars(r)

	salescashdiscountID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesCashDiscount ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salescashdiscount, err = models.FindSalesCashDiscountByID(&salescashdiscountID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &salescashdiscount) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salescashdiscount.UpdatedBy = &userID
	now := time.Now()
	salescashdiscount.UpdatedAt = &now

	// Validate data
	if errs := salescashdiscount.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salescashdiscount.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	salescashdiscount, err = models.FindSalesCashDiscountByID(&salescashdiscount.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find sales cash discount:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salescashdiscount
	json.NewEncoder(w).Encode(response)
}

// ViewSalesCashDiscount : handler function for GET /v1/salescashdiscount/<id> call
func ViewSalesCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	salescashdiscountID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesCashDiscount ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var salescashdiscount *models.SalesCashDiscount

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	salescashdiscount, err = models.FindSalesCashDiscountByID(&salescashdiscountID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salescashdiscount

	json.NewEncoder(w).Encode(response)
}
