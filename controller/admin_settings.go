package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetAdminSettingsHandler returns the global admin settings.
// GET /v1/admin-settings
func GetAdminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	settings, err := models.GetAdminSettings()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"result": settings})
}

// UpdateAdminSettingsHandler saves/updates the global admin settings.
// PUT /v1/admin-settings
func UpdateAdminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	var settings models.AdminSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if err := models.UpsertAdminSettings(&settings); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"result": settings, "success": true})
}
