package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBISalesByCategory : GET /v1/bi/sales-by-category
// Query params: search[store_id] (required), period ("30d"|"90d"|"all", default "30d")
func GetBISalesByCategory(w http.ResponseWriter, r *http.Request) {
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

	results, err := store.GetBISalesByCategory(period)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch sales by category: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
