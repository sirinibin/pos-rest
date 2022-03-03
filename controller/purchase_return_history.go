package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
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

	histories, criterias, err := models.SearchPurchaseReturnHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchase return history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "product_purchase_return_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchase return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchaseReturnHistoryStats, err := models.GetPurchaseReturnHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_purchase"] = "Unable to find total amount of purchase return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_purchase_return"] = purchaseReturnHistoryStats.TotalPurchaseReturn
	response.Meta["total_vat_return"] = purchaseReturnHistoryStats.TotalVatReturn

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
