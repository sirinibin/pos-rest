package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBICustomerChurn : GET /v1/bi/customer-churn
// Query params: search[store_id] (required), tier ("Critical"|"High"|"Medium"|"Low"), limit (default 50)
func GetBICustomerChurn(w http.ResponseWriter, r *http.Request) {
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

	tierFilter := r.URL.Query().Get("tier")
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil {
			limit = n
		}
	}

	results, err := store.GetBICustomerChurn(tierFilter, limit)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch customer churn data: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
