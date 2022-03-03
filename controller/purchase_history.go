package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
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

	histories, criterias, err := models.SearchPurchaseHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "product_purchase_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchase:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchaseHistoryStats, err := models.GetPurchaseHistoryStats(criterias.SearchBy)
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
	response.Meta["total_loss"] = purchaseHistoryStats.TotalLoss
	response.Meta["total_vat"] = purchaseHistoryStats.TotalVat

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
