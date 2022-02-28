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

// ListSalesReturn : handler for GET /salesreturn
func ListSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	salesreturns := []models.SalesReturn{}

	salesreturns, criterias, err := models.SearchSalesReturn(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salesreturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "salesreturn")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of salesreturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesReturnStats, err := models.GetSalesReturnStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_return_sales"] = "Unable to find total amount of sales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_sales_return"] = salesReturnStats.NetTotal
	response.Meta["vat_price"] = salesReturnStats.VatPrice

	if len(salesreturns) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = salesreturns
	}

	json.NewEncoder(w).Encode(response)

}

// CreateSalesReturn : handler for POST /salesreturn
func CreateSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var salesreturn *models.SalesReturn
	// Decode data
	if !utils.Decode(w, r, &salesreturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.CreatedBy = &userID
	salesreturn.UpdatedBy = &userID
	now := time.Now()
	salesreturn.CreatedAt = &now
	salesreturn.UpdatedAt = &now

	// Validate data
	if errs := salesreturn.Validate(w, r, "create", nil); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.FindNetTotal()
	salesreturn.FindTotal()
	salesreturn.FindTotalQuantity()
	salesreturn.FindVatPrice()

	err = salesreturn.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.AddStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.UpdateReturnedQuantityInOrderProduct()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salesreturn

	json.NewEncoder(w).Encode(response)
}

// UpdateSalesReturn : handler function for PUT /v1/salesreturn call
func UpdateSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var salesreturn *models.SalesReturn
	var salesreturnOld *models.SalesReturn

	params := mux.Vars(r)

	salesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["salesreturn_id"] = "Invalid SalesReturn ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturnOld, err = models.FindSalesReturnByID(&salesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_salesreturn"] = "Unable to find salesreturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn, err = models.FindSalesReturnByID(&salesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_salesreturn"] = "Unable to find salesreturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &salesreturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.UpdatedBy = &userID
	now := time.Now()
	salesreturn.UpdatedAt = &now

	// Validate data
	if errs := salesreturn.Validate(w, r, "update", salesreturnOld); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.FindNetTotal()
	salesreturn.FindTotal()
	salesreturn.FindTotalQuantity()
	salesreturn.FindVatPrice()

	err = salesreturn.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.AttributesValueChangeEvent(salesreturnOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn, err = models.FindSalesReturnByID(&salesreturn.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find salesreturn:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = salesreturn
	json.NewEncoder(w).Encode(response)
}

// ViewSalesReturn : handler function for GET /v1/salesreturn/<id> call
func ViewSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	salesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesReturn ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var salesreturn *models.SalesReturn

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	salesreturn, err = models.FindSalesReturnByID(&salesreturnID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.SetChangeLog("view", nil, nil, nil)

	response.Status = true
	response.Result = salesreturn

	json.NewEncoder(w).Encode(response)
}

// DeleteSalesReturn : handler function for DELETE /v1/salesreturn/<id> call
func DeleteSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	salesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid SalesReturn ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn, err := models.FindSalesReturnByID(&salesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.DeleteSalesReturn(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if salesreturn.Status == "delivered" && !salesreturn.Deleted {
		err = salesreturn.AddStock()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}
