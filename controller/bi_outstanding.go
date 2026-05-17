package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBIOutstanding : GET /v1/bi/outstanding
// Query params: search[store_id] (required), type ("AR"|"AP", default "AR"), limit (default 100)
func GetBIOutstanding(w http.ResponseWriter, r *http.Request) {
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

	outstandingType := r.URL.Query().Get("type")
	if outstandingType == "" {
		outstandingType = "AR"
	}

	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}

	summary, err := store.GetBIOutstanding(outstandingType, limit)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch BI outstanding: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = summary
	json.NewEncoder(w).Encode(response)
}
