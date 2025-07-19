package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
)

// List : handler for GET /sales/history/<id>
func ListProductHistory(w http.ResponseWriter, r *http.Request) {
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

	histories := []models.ProductHistory{}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	histories, criterias, err := store.SearchHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find sales history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	productHistoryStats, err := store.GetHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total"] = "Unable to find total:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	//Sales
	response.Meta["total_sales"] = productHistoryStats.TotalSales
	response.Meta["total_sales_profit"] = productHistoryStats.TotalSalesProfit
	response.Meta["total_sales_loss"] = productHistoryStats.TotalSalesLoss
	response.Meta["total_sales_vat"] = productHistoryStats.TotalSalesVat
	//Sales return
	response.Meta["total_sales_return"] = productHistoryStats.TotalSalesReturn
	response.Meta["total_sales_return_profit"] = productHistoryStats.TotalSalesReturnProfit
	response.Meta["total_sales_return_loss"] = productHistoryStats.TotalSalesReturnLoss
	response.Meta["total_sales_return_vat"] = productHistoryStats.TotalSalesReturnVat
	//Purchase
	response.Meta["total_purchase"] = productHistoryStats.TotalPurchase
	response.Meta["total_purchase_profit"] = productHistoryStats.TotalPurchaseProfit
	response.Meta["total_purchase_loss"] = productHistoryStats.TotalPurchaseLoss
	response.Meta["total_purchase_vat"] = productHistoryStats.TotalPurchaseVat

	//Purchase Return
	response.Meta["total_purchase_return"] = productHistoryStats.TotalPurchaseReturn
	response.Meta["total_purchase_return_profit"] = productHistoryStats.TotalPurchaseReturnProfit
	response.Meta["total_purchase_return_loss"] = productHistoryStats.TotalPurchaseReturnLoss
	response.Meta["total_purchase_return_vat"] = productHistoryStats.TotalPurchaseReturnVat

	//Quotation
	response.Meta["total_quotation"] = productHistoryStats.TotalQuotation
	response.Meta["total_quotation_profit"] = productHistoryStats.TotalQuotationProfit
	response.Meta["total_quotation_loss"] = productHistoryStats.TotalQuotationLoss
	response.Meta["total_quotation_vat"] = productHistoryStats.TotalQuotationVat

	//Quotation Sales
	response.Meta["total_quotation_sales"] = productHistoryStats.TotalQuotationSales
	response.Meta["total_quotation_sales_profit"] = productHistoryStats.TotalQuotationSalesProfit
	response.Meta["total_quotation_sales_loss"] = productHistoryStats.TotalQuotationSalesLoss
	response.Meta["total_quotation_sales_vat"] = productHistoryStats.TotalQuotationSalesVat

	//Quotation Sales Return
	response.Meta["total_quotation_sales_return"] = productHistoryStats.TotalQuotationSalesReturn
	response.Meta["total_quotation_sales_return_profit"] = productHistoryStats.TotalQuotationSalesReturnProfit
	response.Meta["total_quotation_sales_return_loss"] = productHistoryStats.TotalQuotationSalesReturnLoss
	response.Meta["total_quotation_sales_return_vat"] = productHistoryStats.TotalQuotationSalesReturnVat

	//Delivery Note
	response.Meta["total_delivery_note_quantity"] = productHistoryStats.TotalDeliveryNoteQuantity

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
