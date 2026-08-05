package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

func ListNonVATSalesHistory(w http.ResponseWriter, r *http.Request) {
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

	histories, criterias, err := store.SearchNonVATSalesHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_non_vat_sales_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	stats, err := store.GetNonVATSalesHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["stats"] = "Unable to find stats:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{
		"total_non_vat_sales": stats.TotalNonVATSales,
		"total_profit":        stats.TotalProfit,
		"total_loss":          stats.TotalLoss,
		"total_vat":           stats.TotalVat,
		"total_quantity":      stats.TotalQuantity,
	}

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)
}
