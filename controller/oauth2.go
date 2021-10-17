package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
)

// Authorize : Authorize a user account
func Authorize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	var auth *models.AuthorizeRequest

	if !utils.Decode(w, r, &auth) {
		return
	}

	// Authenticate
	if errs := auth.Authenticate(); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	authCode, err := auth.GenerateAuthCode()
	if err != nil {
		response.Status = false
		response.Errors["auth_code"] = err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = authCode

	json.NewEncoder(w).Encode(response)

}

// APIInfo : handler function for / call
func APIInfo(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var response models.Response
	response.Status = true
	response.Result = "GoLang / MongoDb Microservice [ OAuth2, JWT and Redis used for security ] "

	json.NewEncoder(w).Encode(response)
}
