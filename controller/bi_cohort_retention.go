package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBICohortRetention : GET /v1/bi/cohort-retention
// Query params: search[store_id] (required)
func GetBICohortRetention(w http.ResponseWriter, r *http.Request) {
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

	results, err := store.GetBICohortRetention()
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch cohort retention data: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
