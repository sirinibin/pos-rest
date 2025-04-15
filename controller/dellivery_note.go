package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
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

	err = deliverynote.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	deliverynote.AddProductsDeliveryNoteHistory()

	deliverynote.SetProductsDeliveryNoteStats()
	deliverynote.SetCustomerDeliveryNoteStats()

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
	/*
		deliverynoteOld, err = models.FindDeliveryNoteByID(&deliverynoteID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["view"] = "Unable to view:" + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
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

	// Validate data
	if errs := deliverynote.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
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

	deliverynote.ClearProductsDeliveryNoteHistory()
	deliverynote.AddProductsDeliveryNoteHistory()

	deliverynote.SetProductsDeliveryNoteStats()
	deliverynote.SetCustomerDeliveryNoteStats()

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
