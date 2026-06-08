package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
)

// GetBIProductAbcXyz : GET /v1/bi/product-abc-xyz
// Query params: search[store_id] (required), abc_tier ("A"|"B"|"C"), xyz_tier ("X"|"Y"|"Z"), limit (default 100)
func GetBIProductAbcXyz(w http.ResponseWriter, r *http.Request) {
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

	abcTier := r.URL.Query().Get("abc_tier")
	xyzTier := r.URL.Query().Get("xyz_tier")
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil {
			limit = n
		}
	}

	results, err := store.GetBIProductAbcXyz(abcTier, xyzTier, limit)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to fetch product ABC-XYZ classification: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = results
	json.NewEncoder(w).Encode(response)
}
