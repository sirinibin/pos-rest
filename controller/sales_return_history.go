package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
)

// List : handler for GET /sales-return/history/<id>
func ListSalesReturnHistory(w http.ResponseWriter, r *http.Request) {
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

	histories := []models.ProductSalesReturnHistory{}

	histories, criterias, err := models.SearchSalesReturnHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find sales return history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "product_sales_return_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of sales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesReturnHistoryStats, err := models.GetSalesReturnHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_sales"] = "Unable to find total amount of sales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_sales_return"] = salesReturnHistoryStats.TotalSalesReturn
	response.Meta["total_profit"] = salesReturnHistoryStats.TotalProfit
	response.Meta["total_loss"] = salesReturnHistoryStats.TotalLoss
	response.Meta["total_vat_return"] = salesReturnHistoryStats.TotalVatReturn

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
