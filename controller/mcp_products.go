package controller

// mcp_products.go — MCP product/inventory endpoints under /v1/mcp/
//
//   GET /v1/mcp/products/summary    — get_product_summary
//   GET /v1/mcp/products            — list_products
//   GET /v1/mcp/product/{id}        — get_product
//   GET /v1/mcp/product-categories  — list_product_categories
//   GET /v1/mcp/product-brands      — list_product_brands

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPProductSummary handles GET /v1/mcp/products/summary
func MCPProductSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildProductCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product")
	stats, err := store.GetProductStats(criterias.SearchBy, store.ID, nil)
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

// MCPListProducts handles GET /v1/mcp/products
func MCPListProducts(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	// loadData=true so product_stores data (stock, prices) is included
	products, criterias, err := store.SearchProduct(w, adapted, true)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product")
	mcpOK(w, totalCount, products, nil)
}

// MCPGetProduct handles GET /v1/mcp/product/{id}
func MCPGetProduct(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid product id", http.StatusBadRequest)
		return
	}
	product, err := store.FindProductByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "product not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, product)
}

// MCPListProductCategories handles GET /v1/mcp/product-categories
func MCPListProductCategories(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	categories, criterias, err := store.SearchProductCategory(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product_category")
	mcpOK(w, totalCount, categories, nil)
}

// MCPListProductBrands handles GET /v1/mcp/product-brands
func MCPListProductBrands(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	brands, criterias, err := store.SearchProductBrand(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "product_brand")
	mcpOK(w, totalCount, brands, nil)
}
