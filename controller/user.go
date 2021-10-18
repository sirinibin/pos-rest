package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sirinibin/pos-rest/db"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// LogOut : handler for DELETE /logout
func LogOut(w http.ResponseWriter, r *http.Request) {
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

	deleted, err := db.RedisClient.Del(tokenClaims.AccessUUID).Result()
	if err != nil || deleted == 0 {
		response.Status = false
		response.Errors["access_token"] = err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return

	}
	response.Status = true
	response.Result = "Successfully logged out"

	json.NewEncoder(w).Encode(response)

}

// Me : handler function for /v1/me call
func Me(w http.ResponseWriter, r *http.Request) {
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

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid UserID:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	user, err := models.FindUserByID(userID)
	if err != nil {
		response.Status = false
		response.Errors["find_user"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	user.Password = ""

	response.Status = true
	response.Result = user

	json.NewEncoder(w).Encode(response)
}

// Register : Register a new user account
func Register(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response

	var user *models.User

	// Decode data
	if !utils.Decode(w, r, &user) {
		return
	}

	// Validate data
	if errs := user.Validate(w, r); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	// Insert new record
	user.Password = models.HashPassword(user.Password)
	user.CreatedAt = time.Now().Local()
	user.UpdatedAt = time.Now().Local()

	err := user.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to Insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	user.Password = ""
	response.Result = user

	json.NewEncoder(w).Encode(response)

}
