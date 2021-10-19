package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListBusiness : handler for GET /business
func ListBusiness(w http.ResponseWriter, r *http.Request) {
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

	businesses := []models.Business{}

	/*
		criterias := models.SearchCriterias{
			Page:   1,
			Size:   10,
			SortBy: "updated_at desc",
		}

		criterias.SearchBy = make(map[string]interface{})
		keys, ok := r.URL.Query()["search[name]"]
		if ok && len(keys[0]) >= 1 {
			criterias.SearchBy["name"] = keys[0]
		}
	*/

	businesses, criterias, err := models.SearchBusiness(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find businesses:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	if len(businesses) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = businesses
	}

	json.NewEncoder(w).Encode(response)

}

// CreateBusiness : handler for POST /business
func CreateBusiness(w http.ResponseWriter, r *http.Request) {
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

	var business *models.Business
	// Decode data
	if !utils.Decode(w, r, &business) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	business.CreatedBy = userID
	business.UpdatedBy = userID
	business.CreatedAt = time.Now().Local()
	business.UpdatedAt = time.Now().Local()

	// Validate data
	if errs := business.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = business.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = business

	json.NewEncoder(w).Encode(response)

}
