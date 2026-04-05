package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// List : handler for GET /purchase-return/history/<id>
func ListPurchaseReturnHistory(w http.ResponseWriter, r *http.Request) {
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

	histories, criterias, err := store.SearchPurchaseReturnHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase return history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_purchase_return_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchase return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchaseReturnHistoryStats, err := store.GetPurchaseReturnHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_purchase"] = "Unable to find total amount of purchase return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_purchase_return"] = purchaseReturnHistoryStats.TotalPurchaseReturn
	response.Meta["total_vat_return"] = purchaseReturnHistoryStats.TotalVatReturn
	response.Meta["total_quantity"] = purchaseReturnHistoryStats.TotalQuantity

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}

// PurchaseReturnHistorySummary : handler for GET v1/purchase/return/history/summary
func PurchaseReturnHistorySummary(w http.ResponseWriter, r *http.Request) {
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
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	criterias, err := store.BuildPurchaseReturnHistoryCriterias(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find product purchase return histories:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_purchase_return_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of product purchase histories:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var purchaseReturnHistoryStats models.PurchaseReturnHistoryStats

	purchaseReturnHistoryStats, err = store.GetPurchaseReturnHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_product_purchase_return_histories"] = "Unable to find total amount of product purchase return histories:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Result = purchaseReturnHistoryStats
	json.NewEncoder(w).Encode(response)
}
