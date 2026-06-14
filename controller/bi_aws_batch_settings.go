package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// GET /v1/bi/aws-batch-settings — returns the stored AWS Batch config.
func GetBIAwsBatchSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	settings, err := models.GetBIAwsBatchSettings()
	if err != nil {
		resp.Status = false
		resp.Errors["find"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = settings
	json.NewEncoder(w).Encode(resp)
}

// POST /v1/bi/aws-batch-settings — saves (upserts) the AWS Batch config.
func SaveBIAwsBatchSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var s models.BIAwsBatchSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		resp.Status = false
		resp.Errors["body"] = "Invalid JSON: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err := models.UpsertBIAwsBatchSettings(&s); err != nil {
		resp.Status = false
		resp.Errors["save"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = s
	json.NewEncoder(w).Encode(resp)
}

// GET /v1/bi/cron-store-settings?base_url=... — returns stored cron store settings.
func GetBICronStoreSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	baseURL := r.URL.Query().Get("base_url")

	settings, err := models.GetBICronStoreSettings(baseURL)
	if err != nil {
		resp.Status = false
		resp.Errors["find"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = settings
	json.NewEncoder(w).Encode(resp)
}

// POST /v1/bi/cron-store-settings — saves (upserts) cron store settings.
func SaveBICronStoreSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var resp models.Response
	resp.Errors = make(map[string]string)

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		resp.Status = false
		resp.Errors["access_token"] = "Invalid access token: " + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(resp)
		return
	}

	var s models.BICronStoreSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		resp.Status = false
		resp.Errors["body"] = "Invalid JSON: " + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err := models.UpsertBICronStoreSettings(&s); err != nil {
		resp.Status = false
		resp.Errors["save"] = err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Status = true
	resp.Result = s
	json.NewEncoder(w).Encode(resp)
}
