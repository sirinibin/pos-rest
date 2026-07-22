package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListPurchaseRequest : handler for GET /purchase-request
func ListPurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	prs, criterias, err := store.SearchPurchaseRequest(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase requests:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	totalCount, err := store.GetPurchaseRequestTotalCount(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.TotalCount = totalCount
	response.Result = prs
	json.NewEncoder(w).Encode(response)
}

// CreatePurchaseRequest : handler for POST /purchase-request
func CreatePurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	var pr *models.PurchaseRequest
	if !utils.Decode(w, r, &pr) {
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
	pr.CreatedBy = &userID
	pr.UpdatedBy = &userID
	pr.CreatedAt = &now
	pr.UpdatedAt = &now
	pr.StoreID = &store.ID
	pr.Status = "pending"

	pr.FindNetTotal()

	errs := pr.Validate(w, r, "create")
	if len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	pr.FindTotalQuantity()

	err = pr.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["update_foreign"] = "Error updating label fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = pr.MakeCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error generating code:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = pr.Insert()
	if err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to create purchase request:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Notify assignee
	if pr.AssignedTo != nil {
		models.NotifyUserByID(pr.AssignedTo, "purchase_request_received", map[string]interface{}{
			"id":              pr.ID.Hex(),
			"code":            pr.Code,
			"created_by_name": pr.CreatedByName,
		})
	}

	store.NotifyUsers("purchase_request_updated")

	response.Status = true
	response.Result = pr
	json.NewEncoder(w).Encode(response)
}

// ViewPurchaseRequest : handler for GET /purchase-request/{id}
func ViewPurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	pr, err := store.FindPurchaseRequestByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase request not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = pr
	json.NewEncoder(w).Encode(response)
}

// UpdatePurchaseRequest : handler for PUT /purchase-request/{id}
func UpdatePurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	pr, err := store.FindPurchaseRequestByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase request not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var prUpdate *models.PurchaseRequest
	if !utils.Decode(w, r, &prUpdate) {
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
	prUpdate.ID = pr.ID
	prUpdate.Code = pr.Code
	prUpdate.CreatedBy = pr.CreatedBy
	prUpdate.CreatedAt = pr.CreatedAt
	prUpdate.UpdatedBy = &userID
	prUpdate.UpdatedAt = &now
	prUpdate.StoreID = &store.ID

	if prUpdate.Status == "" {
		prUpdate.Status = pr.Status
	}

	prUpdate.FindNetTotal()

	errs := prUpdate.Validate(w, r, "update")
	if len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	prUpdate.FindTotalQuantity()

	err = prUpdate.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["update_foreign"] = "Error updating label fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = prUpdate.Update()
	if err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update purchase request:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("purchase_request_updated")

	response.Status = true
	response.Result = prUpdate
	json.NewEncoder(w).Encode(response)
}

// DeletePurchaseRequest : handler for DELETE /purchase-request/{id}
func DeletePurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	pr, err := store.FindPurchaseRequestByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase request not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = pr.Delete()
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete purchase request:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("purchase_request_updated")

	response.Status = true
	response.Result = "Purchase request deleted successfully"
	json.NewEncoder(w).Encode(response)
}

// AcceptPurchaseRequest : handler for POST /purchase-request/{id}/accept
func AcceptPurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	pr, err := store.FindPurchaseRequestByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase request not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode body for partial vs full accept
	var body struct {
		Partial bool `json:"partial"`
	}
	utils.Decode(w, r, &body)

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid user id in token"
		json.NewEncoder(w).Encode(response)
		return
	}

	now := time.Now()
	if body.Partial {
		pr.Status = "partially_accepted"
	} else {
		pr.Status = "accepted"
	}
	pr.UpdatedBy = &userID
	pr.UpdatedAt = &now

	err = pr.Update()
	if err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update purchase request:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Notify creator
	if pr.CreatedBy != nil {
		models.NotifyUserByID(pr.CreatedBy, "purchase_request_status_changed", map[string]interface{}{
			"id":     pr.ID.Hex(),
			"code":   pr.Code,
			"status": pr.Status,
		})
	}

	store.NotifyUsers("purchase_request_updated")

	response.Status = true
	response.Result = pr
	json.NewEncoder(w).Encode(response)
}

// RejectPurchaseRequest : handler for POST /purchase-request/{id}/reject
func RejectPurchaseRequest(w http.ResponseWriter, r *http.Request) {
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

	pr, err := store.FindPurchaseRequestByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase request not found:" + err.Error()
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
	pr.Status = "rejected"
	pr.UpdatedBy = &userID
	pr.UpdatedAt = &now

	err = pr.Update()
	if err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update purchase request:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Notify creator
	if pr.CreatedBy != nil {
		models.NotifyUserByID(pr.CreatedBy, "purchase_request_status_changed", map[string]interface{}{
			"id":     pr.ID.Hex(),
			"code":   pr.Code,
			"status": pr.Status,
		})
	}

	store.NotifyUsers("purchase_request_updated")

	response.Status = true
	response.Result = pr
	json.NewEncoder(w).Encode(response)
}

// CreatePurchaseOrderFromPR : handler for POST /purchase-request/{id}/create-purchase-order
func CreatePurchaseOrderFromPR(w http.ResponseWriter, r *http.Request) {
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

	pr, err := store.FindPurchaseRequestByID(&id, nil)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Purchase request not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if pr.Status != "accepted" && pr.Status != "partially_accepted" {
		response.Status = false
		response.Errors["status"] = "Purchase request must be accepted before creating a purchase order"
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

	po, err := pr.ConvertToPurchaseOrder(&userID)
	if err != nil {
		response.Status = false
		response.Errors["create_po"] = "Unable to create purchase order:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Link PO back to PR
	now := time.Now()
	pr.PurchaseOrderID = &po.ID
	pr.PurchaseOrderCode = &po.Code
	pr.UpdatedBy = &userID
	pr.UpdatedAt = &now
	pr.Update()

	// Notify creator
	if pr.CreatedBy != nil {
		models.NotifyUserByID(pr.CreatedBy, "purchase_request_po_created", map[string]interface{}{
			"id":                  pr.ID.Hex(),
			"code":                pr.Code,
			"purchase_order_id":   po.ID.Hex(),
			"purchase_order_code": po.Code,
		})
	}

	store.NotifyUsers("purchase_order_updated")
	store.NotifyUsers("purchase_request_updated")

	response.Status = true
	response.Result = map[string]interface{}{
		"purchase_request": pr,
		"purchase_order":   po,
	}
	json.NewEncoder(w).Encode(response)
}
