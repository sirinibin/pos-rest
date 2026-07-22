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

func canAccessUserRoles(action string) bool {
	if models.UserObject == nil {
		return false
	}
	if models.UserObject.Role == "Admin" {
		return true
	}
	return models.UserHasPermission("user_roles", action)
}

func ListUserRole(w http.ResponseWriter, r *http.Request) {
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

	if !canAccessUserRoles("read") {
		response.Status = false
		response.Errors["access"] = "Access denied"
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	userRoles, criterias, err := models.SearchUserRole(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find user roles:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	// Total count is only relevant for single-store list (not the multi-store suggest call)
	if storeIDStr := r.URL.Query().Get("search[store_id]"); storeIDStr != "" {
		store, storeErr := ParseStore(r)
		if storeErr == nil {
			totalCount, countErr := models.GetUserRolesTotalCount(&store.ID, criterias.SearchBy)
			if countErr == nil {
				response.TotalCount = totalCount
			}
		}
	}

	if len(userRoles) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = userRoles
	}

	json.NewEncoder(w).Encode(response)
}

func CreateUserRole(w http.ResponseWriter, r *http.Request) {
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

	if !canAccessUserRoles("create") {
		response.Status = false
		response.Errors["access"] = "Access denied"
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
		return
	}

	var userRole *models.UserRole
	if !utils.Decode(w, r, &userRole) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userRole.CreatedBy = &userID
	userRole.UpdatedBy = &userID
	now := time.Now()
	userRole.CreatedAt = &now
	userRole.UpdatedAt = &now

	if errs := userRole.Validate("create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = userRole.Insert()
	if err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to insert user role:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = userRole
	json.NewEncoder(w).Encode(response)
}

func ViewUserRole(w http.ResponseWriter, r *http.Request) {
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

	if !canAccessUserRoles("read") {
		response.Status = false
		response.Errors["access"] = "Access denied"
		w.WriteHeader(http.StatusForbidden)
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userRole, err := models.FindUserRoleByID(&store.ID, &id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find user role:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = userRole
	json.NewEncoder(w).Encode(response)
}

func UpdateUserRole(w http.ResponseWriter, r *http.Request) {
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

	if !canAccessUserRoles("update") {
		response.Status = false
		response.Errors["access"] = "Access denied"
		w.WriteHeader(http.StatusForbidden)
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userRole, err := models.FindUserRoleByID(&store.ID, &id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find user role:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var updates *models.UserRole
	if !utils.Decode(w, r, &updates) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userRole.Name = updates.Name
	userRole.StoreID = updates.StoreID
	userRole.Permissions = updates.Permissions
	userRole.UpdatedBy = &userID
	now := time.Now()
	userRole.UpdatedAt = &now

	if errs := userRole.Validate("update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = userRole.Update()
	if err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update user role:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = userRole
	json.NewEncoder(w).Encode(response)
}

func DeleteUserRole(w http.ResponseWriter, r *http.Request) {
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

	if !canAccessUserRoles("delete") {
		response.Status = false
		response.Errors["access"] = "Access denied"
		w.WriteHeader(http.StatusForbidden)
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

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	userRole, err := models.FindUserRoleByID(&store.ID, &id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find user role:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	inUse, err := models.IsUserRoleInUse(&id)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to check role usage:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	if inUse {
		response.Status = false
		response.Errors["delete"] = "Cannot delete this role — it is currently assigned to one or more users"
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = userRole.Delete(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete user role:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "User role deleted successfully"
	json.NewEncoder(w).Encode(response)
}

// GetEffectivePermissions returns the union of permissions from all roles the authenticated user holds.
// Accessible to any authenticated user (so non-admin users can fetch their own permissions).
func GetEffectivePermissions(w http.ResponseWriter, r *http.Request) {
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

	permissions, err := models.GetEffectivePermissions(models.UserObject.StoreIDs, models.UserObject.RoleIDs)
	if err != nil {
		response.Status = false
		response.Errors["permissions"] = "Unable to load permissions:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = permissions
	json.NewEncoder(w).Encode(response)
}
