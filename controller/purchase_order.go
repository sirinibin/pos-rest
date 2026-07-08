package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ListPurchaseOrder : handler for GET /purchase-order
func ListPurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	pos, criterias, err := store.SearchPurchaseOrder(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	totalCount, err := store.GetPurchaseOrderTotalCount(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.TotalCount = totalCount

	var stats models.PurchaseOrderStats
	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 && keys[0] == "1" {
		stats, err = store.GetPurchaseOrderStats(criterias.SearchBy)
		if err != nil {
			response.Status = false
			response.Errors["stats"] = "Unable to find stats:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Meta = map[string]interface{}{
		"total_purchase_order": stats.NetTotal,
		"vat_price":            stats.VatPrice,
		"discount":             stats.Discount,
		"count":                stats.Count,
	}

	response.Result = pos
	json.NewEncoder(w).Encode(response)
}

// CreatePurchaseOrder : handler for POST /purchase-order
func CreatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	var po *models.PurchaseOrder
	if !utils.Decode(w, r, &po) {
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid user id in token"
		json.NewEncoder(w).Encode(response)
		return
	}

	now := time.Now()
	po.CreatedBy = &userID
	po.UpdatedBy = &userID
	po.CreatedAt = &now
	po.UpdatedAt = &now
	po.StoreID = &store.ID

	if po.Status == "" {
		po.Status = "draft"
	}

	po.FindNetTotal()

	errs := po.Validate(w, r, "create")
	if len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = po.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["vendor"] = "Error creating vendor:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = po.SetUnKnownVendorIfNoVendorSelected()
	if err != nil {
		response.Status = false
		response.Errors["vendor"] = "Error setting vendor:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	po.FindTotalQuantity()

	err = po.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["update_foreign"] = "Error updating label fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = po.MakeCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error generating code:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = po.Insert()
	if err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to create purchase order:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("purchase_order_updated")

	response.Status = true
	response.Result = po
	json.NewEncoder(w).Encode(response)
}

// ViewPurchaseOrder : handler for GET /purchase-order/{id}
func ViewPurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	po, err := store.FindPurchaseOrderByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase order not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = po
	json.NewEncoder(w).Encode(response)
}

func ViewPreviousPurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	po, err := store.FindPurchaseOrderByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase order not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	prev, err := po.FindPreviousPurchaseOrder(map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors["find"] = "Unable to find previous:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = prev
	json.NewEncoder(w).Encode(response)
}

func ViewNextPurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	po, err := store.FindPurchaseOrderByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase order not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	next, err := po.FindNextPurchaseOrder(map[string]interface{}{})
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors["find"] = "Unable to find next:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = next
	json.NewEncoder(w).Encode(response)
}

// UpdatePurchaseOrder : handler for PUT /purchase-order/{id}
func UpdatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	po, err := store.FindPurchaseOrderByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase order not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var poUpdate *models.PurchaseOrder
	if !utils.Decode(w, r, &poUpdate) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid user id in token"
		json.NewEncoder(w).Encode(response)
		return
	}

	now := time.Now()
	poUpdate.ID = po.ID
	poUpdate.Code = po.Code
	poUpdate.CreatedBy = po.CreatedBy
	poUpdate.CreatedAt = po.CreatedAt
	poUpdate.UpdatedBy = &userID
	poUpdate.UpdatedAt = &now
	poUpdate.StoreID = &store.ID

	poUpdate.FindNetTotal()

	errs := poUpdate.Validate(w, r, "update")
	if len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = poUpdate.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["vendor"] = "Error creating vendor:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = poUpdate.SetUnKnownVendorIfNoVendorSelected()
	if err != nil {
		response.Status = false
		response.Errors["vendor"] = "Error setting vendor:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	poUpdate.FindTotalQuantity()

	err = poUpdate.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["update_foreign"] = "Error updating label fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = poUpdate.Update()
	if err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update purchase order:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("purchase_order_updated")

	response.Status = true
	response.Result = poUpdate
	json.NewEncoder(w).Encode(response)
}

// DeletePurchaseOrder : handler for DELETE /purchase-order/{id}
func DeletePurchaseOrder(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	po, err := store.FindPurchaseOrderByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase order not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = po.Delete()
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete purchase order:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("purchase_order_updated")

	response.Status = true
	response.Result = "Purchase order deleted successfully"
	json.NewEncoder(w).Encode(response)
}

// PurchaseOrderSummary : handler for GET /purchase-order/summary
func PurchaseOrderSummary(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	_, criterias, err := store.SearchPurchaseOrder(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	stats, err := store.GetPurchaseOrderStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["stats"] = "Unable to get stats:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = stats
	json.NewEncoder(w).Encode(response)
}

// CalculatePurchaseOrderNetTotal : handler for POST /purchase-order/calculate-net-total
func CalculatePurchaseOrderNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var po *models.PurchaseOrder
	if !utils.Decode(w, r, &po) {
		return
	}

	po.FindNetTotal()

	response.Status = true
	response.Result = po
	json.NewEncoder(w).Encode(response)
}
