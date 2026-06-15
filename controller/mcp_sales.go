package controller

// mcp_sales.go — MCP sales endpoints under /v1/mcp/
//
//   GET /v1/mcp/sales/summary            — get_sales_summary
//   GET /v1/mcp/orders                   — list_orders
//   GET /v1/mcp/order/{id}               — get_order
//   GET /v1/mcp/sales/history/summary    — get_sales_history_summary
//   GET /v1/mcp/sales/history            — list_sales_history
//   GET /v1/mcp/sales-return/summary     — get_sales_return_summary
//   GET /v1/mcp/sales-returns            — list_sales_returns
//   GET /v1/mcp/sales-return/history     — list_sales_return_history

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPSalesSummary handles GET /v1/mcp/sales/summary
// Query params: store_id, date_str | from_date+to_date
func MCPSalesSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildSalesCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "order")
	stats, err := store.GetSalesStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":     store.ID.Hex(),
		"total_orders": totalCount,
		"summary":      stats,
	})
}

// MCPListOrders handles GET /v1/mcp/orders
// Query params: store_id, date_str, from_date, to_date, page, limit, sort,
//
//	status, payment_status, payment_methods, code, customer_id,
//	created_by, include_stats
func MCPListOrders(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	orders, criterias, err := store.SearchOrder(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "order")

	var meta map[string]interface{}
	if r.URL.Query().Get("include_stats") == "true" || r.URL.Query().Get("include_stats") == "1" {
		stats, _ := store.GetSalesStats(criterias.SearchBy)
		meta = map[string]interface{}{
			"total_sales":          stats.NetTotal,
			"paid_sales":           stats.PaidSales,
			"unpaid_sales":         stats.UnPaidSales,
			"vat_price":            stats.VatPrice,
			"net_profit":           stats.NetProfit,
			"net_loss":             stats.NetLoss,
			"return_count":         stats.ReturnCount,
			"return_amount":        stats.ReturnAmount,
			"cash_sales":           stats.CashSales,
			"bank_account_sales":   stats.BankAccountSales,
			"cash_discount":        stats.CashDiscount,
			"discount":             stats.Discount,
			"shipping_handling_fees": stats.ShippingOrHandlingFees,
			"commission":           stats.Commission,
		}
	}

	mcpOK(w, totalCount, orders, meta)
}

// MCPGetOrder handles GET /v1/mcp/order/{id}
// Query params: store_id (required)
func MCPGetOrder(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid order id", http.StatusBadRequest)
		return
	}
	order, err := store.FindOrderByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "order not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, order)
}

// MCPSalesHistorySummary handles GET /v1/mcp/sales/history/summary
// Query params: store_id, date_str, from_date, to_date,
//
//	product_id, customer_id, order_id, order_code, warehouse_code
func MCPSalesHistorySummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildSalesHistoryCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	stats, err := store.GetSalesHistoryStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"summary":  stats,
	})
}

// MCPListSalesHistory handles GET /v1/mcp/sales/history
func MCPListSalesHistory(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	rows, criterias, err := store.SearchSalesHistory(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product_sales_history")
	mcpOK(w, totalCount, rows, nil)
}

// MCPSalesReturnSummary handles GET /v1/mcp/sales-return/summary
func MCPSalesReturnSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildSalesReturnCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "salesreturn")
	stats, err := store.GetSalesReturnStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":             store.ID.Hex(),
		"sales_return_count":   totalCount,
		"summary":              stats,
	})
}

// MCPListSalesReturns handles GET /v1/mcp/sales-returns
func MCPListSalesReturns(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	returns, criterias, err := store.SearchSalesReturn(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "salesreturn")

	var meta map[string]interface{}
	if r.URL.Query().Get("include_stats") == "true" || r.URL.Query().Get("include_stats") == "1" {
		stats, _ := store.GetSalesReturnStats(criterias.SearchBy)
		meta = map[string]interface{}{
			"net_total":           stats.NetTotal,
			"paid_sales_return":   stats.PaidSalesReturn,
			"unpaid_sales_return": stats.UnPaidSalesReturn,
			"vat_price":           stats.VatPrice,
			"return_count":        stats.SalesReturnCount,
		}
	}

	mcpOK(w, totalCount, returns, meta)
}

// MCPListSalesReturnHistory handles GET /v1/mcp/sales-return/history
func MCPListSalesReturnHistory(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	rows, criterias, err := store.SearchSalesReturnHistory(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product_sales_return_history")
	mcpOK(w, totalCount, rows, nil)
}
