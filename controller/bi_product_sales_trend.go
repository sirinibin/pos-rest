package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBIProductSalesTrend : GET /v1/bi/product-sales-trend
// Query params: search[store_id] (required),
//
//	trend ("Growing"|"Rising Star"|"Trending Up"|"Stable"|"Seasonal"|"Softening"|"Declining"),
//	limit (default 50)
func GetBIProductSalesTrend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	trendFilter := r.URL.Query().Get("trend")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil {
			limit = n
		}
	}

	results, err := store.GetBIProductSalesTrends(trendFilter, limit)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch product sales trend data: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
