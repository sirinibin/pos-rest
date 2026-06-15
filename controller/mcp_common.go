package controller

// mcp_common.go — shared helpers for all /v1/mcp/ handlers.
//
// Key ideas:
//   - MCP handlers accept simplified query params (no "search[...]" prefix).
//   - mcpBuildRequest rewrites those into the format expected by existing model
//     functions so we can reuse all existing filter/pagination/sort logic.
//   - mcpAuthAndStore does auth + store resolution using the simplified
//     "store_id" query param.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// bsonCtx returns a context with a 10-second timeout for MongoDB queries.
func bsonCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// mcpWriteJSON writes v as JSON with status 200.
func mcpWriteJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// mcpWriteError writes {"error": msg} with the given HTTP status.
func mcpWriteError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// mcpAuthAndStore authenticates the request and resolves the store from the
// "store_id" query param.  Returns (store, true) on success.
func mcpAuthAndStore(w http.ResponseWriter, r *http.Request) (*models.Store, bool) {
	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		mcpWriteError(w, "Invalid access token: "+err.Error(), http.StatusUnauthorized)
		return nil, false
	}

	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		mcpWriteError(w, "store_id is required", http.StatusBadRequest)
		return nil, false
	}

	storeID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		mcpWriteError(w, "invalid store_id: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}

	store, err := models.FindStoreByID(&storeID, nil)
	if err != nil {
		mcpWriteError(w, "store not found: "+err.Error(), http.StatusBadRequest)
		return nil, false
	}

	return store, true
}

// mcpBuildRequest builds a new *http.Request whose URL query parameters are in
// the "search[field]=value" format expected by existing model Search functions.
//
// MCP param → search param mapping:
//   store_id          → search[store_id]
//   date_str          → search[date_str]
//   from_date         → search[from_date]
//   to_date           → search[to_date]
//   status            → search[status]
//   payment_status    → search[payment_status]
//   payment_methods   → search[payment_methods]
//   code              → search[code]
//   customer_id       → search[customer_id]
//   vendor_id         → search[vendor_id]
//   product_id        → search[product_id]
//   order_id          → search[order_id]
//   order_code        → search[order_code]
//   purchase_id       → search[purchase_id]
//   purchase_code     → search[purchase_code]
//   sales_return_id   → search[sales_return_id]
//   sales_return_code → search[sales_return_code]
//   warehouse_code    → search[warehouse_code]
//   category_id       → search[category_id]
//   account_id        → search[account_id]
//   amount            → search[amount]
//   payment_method    → search[payment_method]
//   include_stats     → search[stats]=1
//   name              → search[name]
//   query             → search[search]
//   phone             → search[phone]
//   email             → search[email]
//   vat_no            → search[vat_no]
//   credit_balance    → search[credit_balance]
//   item_code         → search[item_code]
//   brand_id          → search[brand_id]
//   created_by        → search[created_by]
//   vendor_invoice_no → search[vendor_invoice_no]
//   type              → search[type]
//   from_warehouse_code → search[from_warehouse_code]
//   to_warehouse_code   → search[to_warehouse_code]
//   delivered_by        → search[delivered_by]
//
// Pagination params (page, limit, sort) pass through unchanged.
func mcpBuildRequest(r *http.Request) *http.Request {
	q := r.URL.Query()
	newQ := url.Values{}

	// always suppress deleted records
	newQ.Set("search[deleted]", "0")

	// direct search[field] mappings
	searchFields := []string{
		"store_id", "date_str", "from_date", "to_date",
		"status", "payment_status", "payment_methods",
		"code", "customer_id", "vendor_id", "product_id",
		"order_id", "order_code", "purchase_id", "purchase_code",
		"sales_return_id", "sales_return_code", "warehouse_code",
		"category_id", "account_id", "amount", "payment_method",
		"name", "phone", "email", "vat_no", "credit_balance",
		"item_code", "brand_id", "created_by", "vendor_invoice_no",
		"type", "from_warehouse_code", "to_warehouse_code", "delivered_by",
	}
	for _, f := range searchFields {
		if v := q.Get(f); v != "" {
			newQ.Set("search["+f+"]", v)
		}
	}

	// "query" → search[search]
	if v := q.Get("query"); v != "" {
		newQ.Set("search[search]", v)
	}

	// include_stats → search[stats]=1
	if q.Get("include_stats") == "true" || q.Get("include_stats") == "1" {
		newQ.Set("search[stats]", "1")
	}

	// pagination — pass through as-is
	for _, f := range []string{"page", "limit", "sort"} {
		if v := q.Get(f); v != "" {
			newQ.Set(f, v)
		}
	}

	newURL := *r.URL
	newURL.RawQuery = newQ.Encode()
	newReq := r.Clone(r.Context())
	newReq.URL = &newURL
	return newReq
}

// mcpOK is a convenience wrapper for successful MCP responses.
func mcpOK(w http.ResponseWriter, totalCount int64, result interface{}, meta map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"status":      true,
		"total_count": totalCount,
		"result":      result,
	}
	if meta != nil {
		resp["meta"] = meta
	}
	json.NewEncoder(w).Encode(resp)
}
