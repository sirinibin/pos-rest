package controller

// mcp_inventory.go — MCP inventory / logistics endpoints under /v1/mcp/
//
//   GET /v1/mcp/warehouses         — list_warehouses
//   GET /v1/mcp/stock-transfers    — list_stock_transfers
//   GET /v1/mcp/delivery-notes     — list_delivery_notes
//
// Quotations are also in this file:
//   GET /v1/mcp/quotations/summary  — get_quotation_summary
//   GET /v1/mcp/quotations          — list_quotations
//   GET /v1/mcp/quotation/{id}      — get_quotation

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPListWarehouses handles GET /v1/mcp/warehouses
func MCPListWarehouses(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	warehouses, criterias, err := store.SearchWarehouse(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "warehouse")
	mcpOK(w, totalCount, warehouses, nil)
}

// MCPListStockTransfers handles GET /v1/mcp/stock-transfers
func MCPListStockTransfers(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	transfers, criterias, err := store.SearchStockTransfer(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "stocktransfer")
	mcpOK(w, totalCount, transfers, nil)
}

// MCPListDeliveryNotes handles GET /v1/mcp/delivery-notes
func MCPListDeliveryNotes(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	notes, criterias, err := store.SearchDeliveryNote(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "delivery_note")
	mcpOK(w, totalCount, notes, nil)
}

// ─── Quotations ───────────────────────────────────────────────────────────────

// MCPQuotationSummary handles GET /v1/mcp/quotations/summary
func MCPQuotationSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildQuotationCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "quotation")
	stats, err := store.GetQuotationStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":    store.ID.Hex(),
		"total_count": totalCount,
		"summary":     stats,
	})
}

// MCPListQuotations handles GET /v1/mcp/quotations
func MCPListQuotations(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	quotations, criterias, err := store.SearchQuotation(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "quotation")

	var meta map[string]interface{}
	if r.URL.Query().Get("include_stats") == "true" || r.URL.Query().Get("include_stats") == "1" {
		stats, _ := store.GetQuotationStats(criterias.SearchBy)
		meta = map[string]interface{}{
			"net_total":  stats.NetTotal,
			"net_profit": stats.NetProfit,
			"loss":       stats.Loss,
		}
	}

	mcpOK(w, totalCount, quotations, meta)
}

// MCPGetQuotation handles GET /v1/mcp/quotation/{id}
func MCPGetQuotation(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid quotation id", http.StatusBadRequest)
		return
	}
	quotation, err := store.FindQuotationByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "quotation not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, quotation)
}
