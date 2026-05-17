package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBIQuotationConversion : GET /v1/bi/quotation-conversion
// Query params: search[store_id] (required), months (default 6)
func GetBIQuotationConversion(w http.ResponseWriter, r *http.Request) {
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

	months := 6
	if v := r.URL.Query().Get("months"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			months = n
		}
	}

	results, err := store.GetBIQuotationConversion(months)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch BI quotation conversion: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
