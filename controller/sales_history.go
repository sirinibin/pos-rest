package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// List : handler for GET /sales/history/<id>
func ListSalesHistory(w http.ResponseWriter, r *http.Request) {
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

	histories := []models.ProductSalesHistory{}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	histories, criterias, err := store.SearchSalesHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find sales history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_sales_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesHistoryStats, err := store.GetSalesHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_sales"] = "Unable to find total amount of sales:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_sales"] = salesHistoryStats.TotalSales
	response.Meta["total_profit"] = salesHistoryStats.TotalProfit
	response.Meta["total_loss"] = salesHistoryStats.TotalLoss
	response.Meta["total_vat"] = salesHistoryStats.TotalVat
	response.Meta["total_quantity"] = salesHistoryStats.TotalQuantity

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
