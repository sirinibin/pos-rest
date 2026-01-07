package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// List : handler for GET /quotationsales-return/history/<id>
func ListQuotationSalesReturnHistory(w http.ResponseWriter, r *http.Request) {
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

	histories, criterias, err := store.SearchQuotationSalesReturnHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find quotationsales return history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_quotation_sales_return_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of quotationsales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesReturnHistoryStats, err := store.GetQuotationSalesReturnHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_quotationsales"] = "Unable to find total amount of quotationsales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_quotation_sales_return"] = quotationsalesReturnHistoryStats.TotalQuotationSalesReturn
	response.Meta["total_profit"] = quotationsalesReturnHistoryStats.TotalProfit
	response.Meta["total_loss"] = quotationsalesReturnHistoryStats.TotalLoss
	response.Meta["total_vat_return"] = quotationsalesReturnHistoryStats.TotalVatReturn
	response.Meta["total_quantity"] = quotationsalesReturnHistoryStats.TotalQuantity

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
