package controller

// mcp_vendors.go — MCP vendor endpoints under /v1/mcp/
//
//   GET /v1/mcp/vendors/summary  — get_vendor_summary
//   GET /v1/mcp/vendors          — list_vendors
//   GET /v1/mcp/vendor/{id}      — get_vendor

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPVendorSummary handles GET /v1/mcp/vendors/summary
func MCPVendorSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildVendorCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "vendor")
	stats, err := store.GetVendorStats(criterias.SearchBy)
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

// MCPListVendors handles GET /v1/mcp/vendors
func MCPListVendors(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	vendors, criterias, err := store.SearchVendor(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "vendor")
	mcpOK(w, totalCount, vendors, nil)
}

// MCPGetVendor handles GET /v1/mcp/vendor/{id}
func MCPGetVendor(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid vendor id", http.StatusBadRequest)
		return
	}
	vendor, err := store.FindVendorByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "vendor not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, vendor)
}
