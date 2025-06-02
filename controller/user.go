package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/db"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// ListUser : handler for GET /user
func ListUser(w http.ResponseWriter, r *http.Request) {
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

	/*
		tokenClaims, err := models.AuthenticateByAccessToken(r)
		if err != nil {
			response.Status = false
			response.Errors["access_token"] = "Invalid Access token:" + err.Error()
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(response)
			return
		}

		accessingUserID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
		if err != nil {
			response.Status = false
			response.Errors["user_id"] = "invalid user id: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}

		accessingUser, err := models.FindUserByID(&accessingUserID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["user_id"] = "invalid user: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}

		if accessingUser.Role != "Admin" {
			response.Status = false
			response.Errors["user_id"] = "unauthorized access"
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	//accessingUser.StoreIDs

	users := []models.User{}

	users, criterias, err := models.SearchUser(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find users:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "user")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of users:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(users) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = users
	}

	json.NewEncoder(w).Encode(response)

}

// CreateUser : handler for POST /user
func CreateUser(w http.ResponseWriter, r *http.Request) {
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

	var user *models.User
	// Decode data
	if !utils.Decode(w, r, &user) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user.CreatedBy = &userID
	user.UpdatedBy = &userID
	now := time.Now()
	user.CreatedAt = &now
	user.UpdatedAt = &now

	// Validate data
	if errs := user.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = user.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["updating_labels"] = "error updating labels:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	user.ID = primitive.NewObjectID()
	// Insert new record
	user.Password = models.HashPassword(user.Password)

	if !govalidator.IsNull(user.PhotoContent) {
		err := user.SavePhoto()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["updating_photo"] = "error updating photo: " + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	err = user.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = user

	json.NewEncoder(w).Encode(response)

}

// UpdateUser : handler function for PUT /v1/user call
func UpdateUser(w http.ResponseWriter, r *http.Request) {
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

	var user *models.User

	params := mux.Vars(r)

	userID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customer_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user, err = models.FindUserByID(&userID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var userForm *models.UserForm
	// Decode data
	if !utils.Decode(w, r, &userForm) {
		return
	}

	user.Admin = userForm.Admin
	user.Email = userForm.Email
	user.Mob = userForm.Mob
	if !govalidator.IsNull(userForm.Password) {
		user.Password = models.HashPassword(userForm.Password)
	}
	user.Name = userForm.Name
	user.Photo = userForm.Photo
	user.PhotoContent = userForm.PhotoContent
	user.Role = userForm.Role
	user.StoreIDs = userForm.StoreIDs

	accessingUserID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user.UpdatedBy = &accessingUserID
	now := time.Now()
	user.UpdatedAt = &now

	// Validate data
	if errs := user.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = user.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["updating_labels"] = "error updating labels: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
	}

	now = time.Now()
	user.UpdatedAt = &now

	if !govalidator.IsNull(user.PhotoContent) {
		err := user.SavePhoto()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["saving_photo"] = "error saving photo: " + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
		}
	}

	err = user.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	user, err = models.FindUserByID(&user.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find user:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user.Password = ""

	response.Status = true
	response.Result = user

	json.NewEncoder(w).Encode(response)
}

// ViewUser : handler function for GET /v1/user/<id> call
func ViewUser(w http.ResponseWriter, r *http.Request) {
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

	userID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customer_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var user *models.User

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	user, err = models.FindUserByID(&userID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user.Password = ""
	response.Status = true
	response.Result = user

	json.NewEncoder(w).Encode(response)

}

// DeleteUser : handler function for DELETE /v1/user/<id> call
func DeleteUser(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)

	userID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customer_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	user, err := models.FindUserByID(&userID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = user.DeleteUser(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)

}

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

	user, err := models.FindUserByID(&userID, bson.M{})
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
	if errs := user.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	now := time.Now()
	user.UpdatedAt = &now
	user.CreatedAt = &now

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
