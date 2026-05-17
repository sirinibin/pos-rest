package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBIMonthlyRevenue : GET /v1/bi/monthly-revenue
// Query params: search[store_id] (required), months (default 12)
func GetBIMonthlyRevenue(w http.ResponseWriter, r *http.Request) {
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

	months := 12
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			months = n
		}
	}

	results, err := store.GetBIMonthlyRevenue(months)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch BI monthly revenue: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
