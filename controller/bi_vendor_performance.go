package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBIVendorPerformance : GET /v1/bi/vendor-performance
// Query params: search[store_id] (required), period ("30d"|"90d"|"all"), limit (default 20)
func GetBIVendorPerformance(w http.ResponseWriter, r *http.Request) {
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

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "30d"
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	results, err := store.GetBIVendorPerformance(period, limit)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch BI vendor performance: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
