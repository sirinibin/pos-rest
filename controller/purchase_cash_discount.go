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

// ListPurchaseCashDiscount : handler for GET /purchasecashdiscount
func ListPurchaseCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	purchasecashdiscounts, criterias, err := models.SearchPurchaseCashDiscount(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchasecashdiscounts:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "purchase_cash_discount")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchasecashdiscounts:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasecashdiscountStats, err := models.GetPurchaseCashDiscountStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_purchase"] = "Unable to find total amount of purchasecashdiscount:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta["total_cash_discount"] = purchasecashdiscountStats.TotalCashDiscount

	if len(purchasecashdiscounts) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchasecashdiscounts
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchaseCashDiscount : handler for POST /purchasecashdiscount
func CreatePurchaseCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	var purchasecashdiscount *models.PurchaseCashDiscount
	// Decode data
	if !utils.Decode(w, r, &purchasecashdiscount) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasecashdiscount.CreatedBy = &userID
	purchasecashdiscount.UpdatedBy = &userID
	now := time.Now()
	purchasecashdiscount.CreatedAt = &now
	purchasecashdiscount.UpdatedAt = &now

	// Validate data
	if errs := purchasecashdiscount.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasecashdiscount.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasecashdiscount

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchaseCashDiscount : handler function for PUT /v1/purchasecashdiscount call
func UpdatePurchaseCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	var purchasecashdiscount *models.PurchaseCashDiscount

	params := mux.Vars(r)

	purchasecashdiscountID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid PurchaseCashDiscount ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasecashdiscount, err = models.FindPurchaseCashDiscountByID(&purchasecashdiscountID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchasecashdiscount) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasecashdiscount.UpdatedBy = &userID
	now := time.Now()
	purchasecashdiscount.UpdatedAt = &now

	// Validate data
	if errs := purchasecashdiscount.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasecashdiscount.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasecashdiscount, err = models.FindPurchaseCashDiscountByID(&purchasecashdiscount.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchase cash discount:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasecashdiscount
	json.NewEncoder(w).Encode(response)
}

// ViewPurchaseCashDiscount : handler function for GET /v1/purchasecashdiscount/<id> call
func ViewPurchaseCashDiscount(w http.ResponseWriter, r *http.Request) {
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

	purchasecashdiscountID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid PurchaseCashDiscount ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchasecashdiscount *models.PurchaseCashDiscount

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	purchasecashdiscount, err = models.FindPurchaseCashDiscountByID(&purchasecashdiscountID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchasecashdiscount

	json.NewEncoder(w).Encode(response)
}
