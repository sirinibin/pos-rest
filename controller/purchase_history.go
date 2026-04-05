package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// List : handler for GET /sales/history/<id>
func ListPurchaseHistory(w http.ResponseWriter, r *http.Request) {
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

	histories := []models.ProductPurchaseHistory{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	histories, criterias, err := store.SearchPurchaseHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_purchase_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchase:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchaseHistoryStats, err := store.GetPurchaseHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_purchases"] = "Unable to find total amount of purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_purchase"] = purchaseHistoryStats.TotalPurchase
	response.Meta["total_retail_profit"] = purchaseHistoryStats.TotalRetailProfit
	response.Meta["total_wholesale_profit"] = purchaseHistoryStats.TotalWholesaleProfit
	response.Meta["total_retail_loss"] = purchaseHistoryStats.TotalRetailLoss
	response.Meta["total_wholesale_loss"] = purchaseHistoryStats.TotalWholesaleLoss
	response.Meta["total_vat"] = purchaseHistoryStats.TotalVat
	response.Meta["total_quantity"] = purchaseHistoryStats.TotalQuantity

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}

// PurchaseHistorySummary : handler for GET v1/purchase/history/summary
func PurchaseHistorySummary(w http.ResponseWriter, r *http.Request) {
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

	criterias, err := store.BuildPurchaseHistoryCriterias(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find product purchase histories:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_purchase_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of product purchase histories:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var purchaseHistoryStats models.PurchaseHistoryStats

	purchaseHistoryStats, err = store.GetPurchaseHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_product_purchase_histories"] = "Unable to find total amount of product purchase histories:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Result = purchaseHistoryStats
	json.NewEncoder(w).Encode(response)
}
