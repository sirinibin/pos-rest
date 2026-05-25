package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListDeliveryNote : handler for GET /deliverynote
func ListDeliveryNote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)
	response.Meta = make(map[string]interface{})

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

	deliverynotes, criterias, err := store.SearchDeliveryNote(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find deliverynotes:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "delivery_note")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of deliverynotes:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var dnStats models.DeliveryNoteStats
	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 && keys[0] == "1" {
		dnStats, err = store.GetDeliveryNoteStats(criterias.SearchBy)
		if err != nil {
			response.Status = false
			response.Errors["stats"] = "Unable to find delivery note stats:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Meta = map[string]interface{}{
		"total_deliverynote":     dnStats.NetTotal,
		"vat_price":              dnStats.VatPrice,
		"discount":               dnStats.Discount,
		"shipping_handling_fees": dnStats.ShippingOrHandlingFees,
	}

	if len(deliverynotes) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = deliverynotes
	}

	json.NewEncoder(w).Encode(response)

}

// CreateDeliveryNote : handler for POST /deliverynote
func CreateDeliveryNote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var deliverynote *models.DeliveryNote
	// Decode data
	if !utils.Decode(w, r, &deliverynote) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	deliverynote.CreatedBy = &userID
	deliverynote.UpdatedBy = &userID
	now := time.Now()
	deliverynote.CreatedAt = &now
	deliverynote.UpdatedAt = &now

	// Validate data
	if errs := deliverynote.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.CreateNewCustomerFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_customer_from_name"] = "error creating new customer from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.MakeRedisCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error making code: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.Insert()
	if err != nil {
		redisErr := deliverynote.UnMakeRedisCode()
		if redisErr != nil {
			response.Errors["error_unmaking_code"] = "error_unmaking_code: " + redisErr.Error()
		}

		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go deliverynote.CreateProductsDeliveryNoteHistory()

	go deliverynote.SetProductsDeliveryNoteStats()
	go deliverynote.SetCustomerDeliveryNoteStats()

	go deliverynote.CreateProductsHistory(true, nil)

	response.Status = true
	response.Result = deliverynote

	json.NewEncoder(w).Encode(response)
}

// UpdateDeliveryNote : handler function for PUT /v1/deliverynote call
func UpdateDeliveryNote(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var deliverynote *models.DeliveryNote
	//var deliverynoteOld *models.DeliveryNote

	params := mux.Vars(r)

	deliverynoteID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid DeliveryNote ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
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

	deliverynoteOld, err := store.FindDeliveryNoteByID(&deliverynoteID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	deliverynote, err = store.FindDeliveryNoteByID(&deliverynoteID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &deliverynote) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	deliverynote.UpdatedBy = &userID
	now := time.Now()
	deliverynote.UpdatedAt = &now

	// If notify_at is being set to a NEW future time (i.e. the user changed it),
	// reset notified so the scheduler fires again. Preserve notified if notify_at
	// didn't change (protects against a frontend race condition where the form
	// submits with a default future notify_at before the existing value loads).
	notifyAtChanged := deliverynoteOld.NotifyAt == nil && deliverynote.NotifyAt != nil ||
		deliverynoteOld.NotifyAt != nil && deliverynote.NotifyAt != nil &&
			!deliverynoteOld.NotifyAt.Equal(*deliverynote.NotifyAt)
	if deliverynote.NotifyAt != nil && deliverynote.NotifyAt.After(now) && notifyAtChanged {
		deliverynote.Notified = false
	} else {
		// notify_at unchanged or in the past — preserve the notified flag from DB
		deliverynote.Notified = deliverynoteOld.Notified
	}

	// Validate data
	if errs := deliverynote.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.CreateNewCustomerFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_customer_from_name"] = "error creating new customer from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = deliverynote.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go deliverynote.ClearProductsDeliveryNoteHistory()
	go deliverynote.CreateProductsDeliveryNoteHistory()

	go deliverynote.SetProductsDeliveryNoteStats()
	go deliverynoteOld.SetProductsDeliveryNoteStats()

	go deliverynote.SetCustomerDeliveryNoteStats()
	go deliverynoteOld.SetCustomerDeliveryNoteStats()

	go func() {
		deliverynote.ClearProductsHistory()
		deliverynote.CreateProductsHistory(true, deliverynoteOld)
	}()

	deliverynote, err = store.FindDeliveryNoteByID(&deliverynote.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find deliverynote:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = deliverynote
	json.NewEncoder(w).Encode(response)
}

// ViewDeliveryNote : handler function for GET /v1/deliverynote/<id> call
func ViewDeliveryNote(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)

	deliverynoteID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["delivery_note_id"] = "Invalid DeliveryNote ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var deliverynote *models.DeliveryNote

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	deliverynote, err = store.FindDeliveryNoteByID(&deliverynoteID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, _ := store.FindCustomerByID(deliverynote.CustomerID, bson.M{})
	customer.SetSearchLabel()
	deliverynote.Customer = customer

	response.Status = true
	response.Result = deliverynote

	json.NewEncoder(w).Encode(response)
}

func CalculateDeliveryNoteNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var deliveryNote *models.DeliveryNote
	if !utils.Decode(w, r, &deliveryNote) {
		return
	}

	deliveryNote.FindNetTotal()

	response.Status = true
	response.Result = deliveryNote

	json.NewEncoder(w).Encode(response)
}

// ListDeliveryNoteReminders returns delivery notes that have been notified (notify_at reached)
// and have no sales created yet (order_id is nil). Used for the notification bell on page load.
func ListDeliveryNoteReminders(w http.ResponseWriter, r *http.Request) {
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

	reminders, err := store.GetActiveDeliveryNoteReminders()
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find reminders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	if len(reminders) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = reminders
	}
	json.NewEncoder(w).Encode(response)
}
