package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
)

// List : handler for GET /deliverynote/history
func ListDeliveryNoteHistory(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	histories, criterias, err := store.SearchDeliveryNoteHistory(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find deliverynote history:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "product_delivery_note_history")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	deliveryNoteHistoryStats, err := store.GetDeliveryNoteHistoryStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_quantity"] = "Unable to find total quantity of delivery notes:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}
	response.Meta["total_quantity"] = deliveryNoteHistoryStats.TotalQuantity

	if len(histories) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = histories
	}

	json.NewEncoder(w).Encode(response)

}
