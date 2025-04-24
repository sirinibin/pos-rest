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

// ListQuotation : handler for GET /quotation
func ListQuotation(w http.ResponseWriter, r *http.Request) {
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

	quotations := []models.Quotation{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	quotations, criterias, err := store.SearchQuotation(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find quotations:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "quotation")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of quotations:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotationStats models.QuotationStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			quotationStats, err = store.GetQuotationStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_sales"] = "Unable to find total amount of quotation:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta["total_quotation"] = quotationStats.NetTotal
	response.Meta["profit"] = quotationStats.NetProfit
	response.Meta["loss"] = quotationStats.Loss

	if len(quotations) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = quotations
	}

	json.NewEncoder(w).Encode(response)

}

// CreateQuotation : handler for POST /quotation
func CreateQuotation(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotation *models.Quotation
	// Decode data
	if !utils.Decode(w, r, &quotation) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.CreatedBy = &userID
	quotation.UpdatedBy = &userID
	now := time.Now()
	quotation.CreatedAt = &now
	quotation.UpdatedAt = &now

	// Validate data
	if errs := quotation.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.FindNetTotal()
	//quotation.FindTotal()
	quotation.FindTotalQuantity()
	//	quotation.FindVatPrice()
	quotation.UpdateForeignLabelFields()
	quotation.MakeCode()

	quotation.CalculateQuotationProfit()

	err = quotation.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.AddProductsQuotationHistory()

	quotation.SetProductsQuotationStats()
	quotation.SetCustomerQuotationStats()

	store.NotifyUsers("quotation_updated")
	response.Status = true
	response.Result = quotation

	json.NewEncoder(w).Encode(response)
}

// UpdateQuotation : handler function for PUT /v1/quotation call
func UpdateQuotation(w http.ResponseWriter, r *http.Request) {
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

	var quotation *models.Quotation
	//var quotationOld *models.Quotation

	params := mux.Vars(r)

	quotationID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Quotation ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	/*
		quotationOld, err = models.FindQuotationByID(&quotationID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["view"] = "Unable to view:" + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	quotation, err = store.FindQuotationByID(&quotationID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &quotation) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.UpdatedBy = &userID
	now := time.Now()
	quotation.UpdatedAt = &now

	// Validate data
	if errs := quotation.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.FindNetTotal()
	quotation.FindTotal()
	quotation.FindTotalQuantity()
	quotation.FindVatPrice()
	quotation.CalculateQuotationProfit()

	quotation.UpdateForeignLabelFields()
	err = quotation.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.ClearProductsQuotationHistory()
	quotation.AddProductsQuotationHistory()

	/*
		err = quotation.AttributesValueChangeEvent(quotationOld)
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	quotation.SetProductsQuotationStats()
	quotation.SetCustomerQuotationStats()

	quotation, err = store.FindQuotationByID(&quotation.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find quotation:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("quotation_updated")
	response.Status = true
	response.Result = quotation
	json.NewEncoder(w).Encode(response)
}

// ViewQuotation : handler function for GET /v1/quotation/<id> call
func ViewQuotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	quotationID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["product_id"] = "Invalid Quotation ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotation *models.Quotation

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation, err = store.FindQuotationByID(&quotationID, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(quotation.CustomerID, bson.M{})
		customer.SetSearchLabel()
		quotation.Customer = customer
	}

	response.Status = true
	response.Result = quotation

	json.NewEncoder(w).Encode(response)
}

// DeleteQuotation : handler function for DELETE /v1/quotation/<id> call
func DeleteQuotation(w http.ResponseWriter, r *http.Request) {
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

	quotationID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid Quotation ID:" + err.Error()
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

	quotation, err := store.FindQuotationByID(&quotationID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.DeleteQuotation(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}

// CreateOrder : handler for POST /order
func CalculateQuotationNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var quotation *models.Quotation
	// Decode data
	if !utils.Decode(w, r, &quotation) {
		return
	}

	quotation.FindNetTotal()
	quotation.FindTotal()
	quotation.FindVatPrice()

	response.Status = true
	response.Result = quotation

	json.NewEncoder(w).Encode(response)
}
