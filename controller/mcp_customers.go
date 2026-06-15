package controller

// mcp_customers.go — MCP customer endpoints under /v1/mcp/
//
//   GET /v1/mcp/customers/summary  — get_customer_summary
//   GET /v1/mcp/customers          — list_customers
//   GET /v1/mcp/customer/{id}      — get_customer
//   GET /v1/mcp/customers/new      — get_new_customers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MCPCustomerSummary handles GET /v1/mcp/customers/summary
func MCPCustomerSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildCustomerCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "customer")
	stats, err := store.GetCustomerStats(criterias.SearchBy)
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

// MCPListCustomers handles GET /v1/mcp/customers
func MCPListCustomers(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	customers, criterias, err := store.SearchCustomer(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "customer")
	mcpOK(w, totalCount, customers, nil)
}

// MCPGetCustomer handles GET /v1/mcp/customer/{id}
func MCPGetCustomer(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid customer id", http.StatusBadRequest)
		return
	}
	customer, err := store.FindCustomerByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "customer not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, customer)
}

// MCPGetNewCustomers handles GET /v1/mcp/customers/new
// Query params: store_id, days (default 30)
func MCPGetNewCustomers(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}

	days := 30
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}

	since := time.Now().AddDate(0, 0, -days)
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
	ctx, cancel := bsonCtx()
	defer cancel()

	filter := bson.M{
		"store_id":   store.ID,
		"created_at": bson.M{"$gte": since},
		"deleted":    bson.M{"$ne": true},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetProjection(bson.M{
			"name": 1, "phone": 1, "created_at": 1, "credit_balance": 1,
		})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var customers []bson.M
	if err := cursor.All(ctx, &customers); err != nil {
		mcpWriteError(w, "decode error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if customers == nil {
		customers = []bson.M{}
	}

	mcpWriteJSON(w, map[string]interface{}{
		"new_customer_count": len(customers),
		"since_date":         since.Format("Jan 02 2006"),
		"days":               days,
		"customers":          customers,
	})
}
