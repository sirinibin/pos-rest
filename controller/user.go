package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

	requestingUser, err := models.FindUserByID(&userID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Only Admins (role=Admin) can create users with role=Admin
	if user.Role == "Admin" {
		if requestingUser.Role != "Admin" {
			response.Status = false
			response.Errors["role"] = "Only admins can assign the Admin role"
			json.NewEncoder(w).Encode(response)
			return
		}
	}
	if errs := grantErrors(requestingUser, user.Admin, user.StoreIDs, nil); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

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

	if user.OpeningBalance > 0 && user.StoreID != nil {
		userStore, err := models.FindStoreByID(user.StoreID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["opening_balance"] = "Unable to find store for opening balance: " + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		if err := user.PostUserOpeningBalanceIfNeeded(userStore); err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["opening_balance"] = "Unable to post opening balance: " + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		if user.OpeningBalancePosted {
			_ = user.Update()
		}
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

	requestingID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, err := models.FindUserByID(&requestingID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}
	if msg := manageUserError(requestingUser, user); msg != "" {
		response.Status = false
		response.Errors["authorization"] = msg
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}
	if !isAdminUser(requestingUser) && requestingUser.ID == user.ID && userForm.Role != user.Role {
		response.Status = false
		response.Errors["role"] = "You can't change your own role"
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}
	if errs := grantErrors(requestingUser, userForm.Admin && !user.Admin, userForm.StoreIDs, user.StoreIDs); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	oldOpeningBalance := user.OpeningBalance
	oldOpeningBalanceType := user.OpeningBalanceType
	var oldOpeningBalanceDate *time.Time
	if user.OpeningBalanceDate != nil {
		t := *user.OpeningBalanceDate
		oldOpeningBalanceDate = &t
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
	user.RoleIDs = userForm.RoleIDs
	user.StoreID = userForm.StoreID
	user.OpeningBalance = userForm.OpeningBalance
	user.OpeningBalanceDate = userForm.OpeningBalanceDate
	user.OpeningBalanceType = userForm.OpeningBalanceType

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

	// Only Admins (role=Admin) can set role=Admin
	if user.Role == "Admin" {
		accessingUser, _ := models.FindUserByID(&accessingUserID, bson.M{})
		if accessingUser.Role != "Admin" {
			response.Status = false
			response.Errors["role"] = "Only admins can assign the Admin role"
			json.NewEncoder(w).Encode(response)
			return
		}
	}

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

	openingBalanceChanged := user.OpeningBalance != oldOpeningBalance ||
		!models.TimesEqual(user.OpeningBalanceDate, oldOpeningBalanceDate) ||
		user.OpeningBalanceType != oldOpeningBalanceType ||
		(!user.OpeningBalancePosted && user.OpeningBalance != 0)
	if openingBalanceChanged && user.StoreID != nil {
		userStore, err := models.FindStoreByID(user.StoreID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["opening_balance"] = "Unable to find store for opening balance: " + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		if err := user.PostUserOpeningBalanceIfNeeded(userStore); err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["opening_balance"] = "Unable to post opening balance: " + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		_ = user.Update()
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

	requestingID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, err := models.FindUserByID(&requestingID, bson.M{"role": 1, "admin": 1, "store_ids": 1})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}
	target, err := models.FindUserByID(&userID, bson.M{"store_ids": 1, "created_by": 1})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	if !viewUserAllowed(requestingUser, target) {
		response.Status = false
		response.Errors["authorization"] = "You can't view this user"
		w.WriteHeader(http.StatusForbidden)
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

	requestingID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, err := models.FindUserByID(&requestingID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}
	if msg := manageUserError(requestingUser, user); msg != "" {
		response.Status = false
		response.Errors["authorization"] = msg
		w.WriteHeader(http.StatusForbidden)
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

// ChangePassword handles PATCH /v1/user/{id}/change-password
func ChangePassword(w http.ResponseWriter, r *http.Request) {
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
	targetUserID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	requestingUserID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid requesting user ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	requestingUser, err := models.FindUserByID(&requestingUserID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Requesting user not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	targetUser, err := models.FindUserByID(&targetUserID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Target user not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !utils.Decode(w, r, &body) {
		return
	}

	isSelf := requestingUserID == targetUserID
	isAdmin := requestingUser.Admin || requestingUser.Role == "Admin"

	if !isSelf && !isAdmin {
		if requestingUser.Role != "Manager" {
			response.Status = false
			response.Errors["authorization"] = "Only Admin or Manager can change other users' passwords"
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
		if targetUser.Role != "Manager" && targetUser.Role != "SalesMan" {
			response.Status = false
			response.Errors["authorization"] = "Manager can only change passwords for Manager or SalesMan users"
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
		sharedStore := false
		for _, mStoreID := range requestingUser.StoreIDs {
			for _, tStoreID := range targetUser.StoreIDs {
				if mStoreID != nil && tStoreID != nil && *mStoreID == *tStoreID {
					sharedStore = true
					break
				}
			}
			if sharedStore {
				break
			}
		}
		if !sharedStore {
			response.Status = false
			response.Errors["authorization"] = "Target user is not in your store"
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if isSelf {
		if body.CurrentPassword == "" {
			response.Status = false
			response.Errors["current_password"] = "Current password is required"
			json.NewEncoder(w).Encode(response)
			return
		}
		if !targetUser.VerifyPassword(body.CurrentPassword) {
			response.Status = false
			response.Errors["current_password"] = "Current password is incorrect"
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if body.NewPassword == "" {
		response.Status = false
		response.Errors["new_password"] = "New password is required"
		json.NewEncoder(w).Encode(response)
		return
	}
	if len(body.NewPassword) < 6 {
		response.Status = false
		response.Errors["new_password"] = "New password must be at least 6 characters"
		json.NewEncoder(w).Encode(response)
		return
	}

	targetUser.Password = models.HashPassword(body.NewPassword)
	now := time.Now()
	targetUser.UpdatedAt = &now
	targetUser.UpdatedBy = &requestingUserID

	if err = targetUser.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update password:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Password changed successfully"
	json.NewEncoder(w).Encode(response)
}

// ToggleUserStatus handles PATCH /v1/user/{id}/toggle-status
func ToggleUserStatus(w http.ResponseWriter, r *http.Request) {
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
	targetUserID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	requestingUserID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid requesting user ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	requestingUser, err := models.FindUserByID(&requestingUserID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Requesting user not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	targetUser, err := models.FindUserByID(&targetUserID, bson.M{"_id": 1, "role": 1, "store_ids": 1, "deleted": 1})
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Target user not found:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	isAdmin := requestingUser.Admin || requestingUser.Role == "Admin"
	isSelf := requestingUserID == targetUserID

	if isSelf {
		response.Status = false
		response.Errors["authorization"] = "Cannot toggle your own status"
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !isAdmin {
		if requestingUser.Role != "Manager" {
			response.Status = false
			response.Errors["authorization"] = "Only Admin or Manager can toggle user status"
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
		if targetUser.Role != "Manager" && targetUser.Role != "SalesMan" {
			response.Status = false
			response.Errors["authorization"] = "Manager can only manage Manager or SalesMan users"
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
		sharedStore := false
		for _, mStoreID := range requestingUser.StoreIDs {
			for _, tStoreID := range targetUser.StoreIDs {
				if mStoreID != nil && tStoreID != nil && *mStoreID == *tStoreID {
					sharedStore = true
					break
				}
			}
			if sharedStore {
				break
			}
		}
		if !sharedStore {
			response.Status = false
			response.Errors["authorization"] = "Target user is not in your store"
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if targetUser.Deleted {
		err = targetUser.RestoreUser(&requestingUserID)
		if err != nil {
			response.Status = false
			response.Errors["update"] = "Unable to restore user:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		response.Status = true
		response.Result = "User activated successfully"
	} else {
		err = targetUser.DeleteUser(tokenClaims)
		if err != nil {
			response.Status = false
			response.Errors["update"] = "Unable to deactivate user:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		response.Status = true
		response.Result = "User deactivated successfully"
	}

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
