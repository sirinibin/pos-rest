package controller

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// ListPurchase : handler for GET /purchase
func ListPurchase(w http.ResponseWriter, r *http.Request) {
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

	purchases := []models.Purchase{}

	purchases, criterias, err := models.SearchPurchase(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "purchase")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(purchases) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchases
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchase : handler for POST /purchase
func CreatePurchase(w http.ResponseWriter, r *http.Request) {
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

	var purchase *models.Purchase
	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.CreatedBy = &userID
	purchase.UpdatedBy = &userID
	now := time.Now()
	purchase.CreatedAt = &now
	purchase.UpdatedAt = &now

	// Validate data
	if errs := purchase.Validate(w, r, "create", nil); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}
	log.Print("Validated")

	purchase.FindNetTotal()
	purchase.FindTotal()
	purchase.FindTotalQuantity()
	purchase.FindVatPrice()

	log.Print("Before Insert")

	err = purchase.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Print("Inserted")
	err = purchase.AddStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	log.Print("Added Stock")

	err = purchase.UpdateProductUnitPriceInStore()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["product_unit_price"] = "Unable to update product unit price:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchase : handler function for PUT /v1/purchase call
func UpdatePurchase(w http.ResponseWriter, r *http.Request) {
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

	var purchase *models.Purchase
	var purchaseOld *models.Purchase

	params := mux.Vars(r)

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["purchase_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchaseOld, err = models.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchase"] = "Unable to find purchase:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err = models.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchase"] = "Unable to find purchase:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.UpdatedBy = &userID
	now := time.Now()
	purchase.UpdatedAt = &now

	// Validate data
	if errs := purchase.Validate(w, r, "update", purchaseOld); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.FindNetTotal()
	purchase.FindTotal()
	purchase.FindTotalQuantity()
	purchase.FindVatPrice()

	err = purchase.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchaseOld.RemoveStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.AddStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.UpdateProductUnitPriceInStore()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["product_unit_price"] = "Unable to update product unit price:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.AttributesValueChangeEvent(purchaseOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err = models.FindPurchaseByID(&purchase.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchase:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchase
	json.NewEncoder(w).Encode(response)
}

// ViewPurchase : handler function for GET /v1/purchase/<id> call
func ViewPurchase(w http.ResponseWriter, r *http.Request) {
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

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchase *models.Purchase

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	purchase, err = models.FindPurchaseByID(&purchaseID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

// DeletePurchase : handler function for DELETE /v1/purchase/<id> call
func DeletePurchase(w http.ResponseWriter, r *http.Request) {
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

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err := models.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.DeletePurchase(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchase.Status == "delivered" && !purchase.Deleted {
		err = purchase.RemoveStock()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}
