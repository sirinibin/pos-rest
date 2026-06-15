package controller

// mcp_purchases.go — MCP purchase endpoints under /v1/mcp/
//
//   GET /v1/mcp/purchases/summary           — get_purchase_summary
//   GET /v1/mcp/purchases                   — list_purchases
//   GET /v1/mcp/purchase/{id}               — get_purchase
//   GET /v1/mcp/purchases/history/summary   — get_purchase_history_summary
//   GET /v1/mcp/purchases/history           — list_purchase_history
//   GET /v1/mcp/purchase-returns/summary    — get_purchase_return_summary
//   GET /v1/mcp/purchase-returns            — list_purchase_returns

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPPurchaseSummary handles GET /v1/mcp/purchases/summary
func MCPPurchaseSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildPurchaseCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "purchase")
	stats, err := store.GetPurchaseStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":        store.ID.Hex(),
		"total_purchases": totalCount,
		"summary":         stats,
	})
}

// MCPListPurchases handles GET /v1/mcp/purchases
func MCPListPurchases(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	purchases, criterias, err := store.SearchPurchase(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "purchase")

	var meta map[string]interface{}
	if r.URL.Query().Get("include_stats") == "true" || r.URL.Query().Get("include_stats") == "1" {
		stats, _ := store.GetPurchaseStats(criterias.SearchBy)
		meta = map[string]interface{}{
			"net_total":         stats.NetTotal,
			"paid_purchase":     stats.PaidPurchase,
			"unpaid_purchase":   stats.UnPaidPurchase,
			"vat_price":         stats.VatPrice,
			"return_count":      stats.ReturnCount,
			"return_amount":     stats.ReturnAmount,
			"cash_purchase":     stats.CashPurchase,
			"bank_purchase":     stats.BankAccountPurchase,
			"cash_discount":     stats.CashDiscount,
			"discount":          stats.Discount,
		}
	}

	mcpOK(w, totalCount, purchases, meta)
}

// MCPGetPurchase handles GET /v1/mcp/purchase/{id}
// Query params: store_id (required)
func MCPGetPurchase(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid purchase id", http.StatusBadRequest)
		return
	}
	purchase, err := store.FindPurchaseByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "purchase not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, purchase)
}

// MCPPurchaseHistorySummary handles GET /v1/mcp/purchases/history/summary
func MCPPurchaseHistorySummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildPurchaseHistoryCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	stats, err := store.GetPurchaseHistoryStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"summary":  stats,
	})
}

// MCPListPurchaseHistory handles GET /v1/mcp/purchases/history
func MCPListPurchaseHistory(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	rows, criterias, err := store.SearchPurchaseHistory(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product_purchase_history")
	mcpOK(w, totalCount, rows, nil)
}

// MCPPurchaseReturnSummary handles GET /v1/mcp/purchase-returns/summary
func MCPPurchaseReturnSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildPurchaseReturnCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "purchasereturn")
	stats, err := store.GetPurchaseReturnStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":               store.ID.Hex(),
		"purchase_return_count":  totalCount,
		"summary":                stats,
	})
}

// MCPListPurchaseReturns handles GET /v1/mcp/purchase-returns
func MCPListPurchaseReturns(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	returns, criterias, err := store.SearchPurchaseReturn(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "purchasereturn")

	var meta map[string]interface{}
	if r.URL.Query().Get("include_stats") == "true" || r.URL.Query().Get("include_stats") == "1" {
		stats, _ := store.GetPurchaseReturnStats(criterias.SearchBy)
		meta = map[string]interface{}{
			"net_total":                    stats.NetTotal,
			"paid_purchase_return":         stats.PaidPurchaseReturn,
			"unpaid_purchase_return":       stats.UnPaidPurchaseReturn,
			"vat_price":                    stats.VatPrice,
			"purchase_return_count":        stats.PurchaseReturnCount,
		}
	}

	mcpOK(w, totalCount, returns, meta)
}
