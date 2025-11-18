package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// ListLedger : handler for GET /ledger
func ListLedger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	Ledgers := []models.Ledger{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	Ledgers, criterias, err := store.SearchLedger(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find ledgers:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "ledger")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of ledgers:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(Ledgers) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = Ledgers
	}

	json.NewEncoder(w).Encode(response)

}
