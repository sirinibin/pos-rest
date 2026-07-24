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

// ListRepairJob : handler for GET /repair-job
func ListRepairJob(w http.ResponseWriter, r *http.Request) {
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

	jobs, criterias, err := store.SearchRepairJob(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find repair jobs:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "repair_job")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of repair jobs:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(jobs) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = jobs
	}

	json.NewEncoder(w).Encode(response)
}

// CreateRepairJob : handler for POST /repair-job
func CreateRepairJob(w http.ResponseWriter, r *http.Request) {
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

	var job *models.RepairJob
	if !utils.Decode(w, r, &job) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
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

	job.StoreID = &store.ID

	// Auto-generate job number if not provided
	if job.JobNumber == "" {
		jobNumber, err := store.GenerateRepairJobNumber()
		if err != nil {
			response.Status = false
			response.Errors["job_number"] = "Unable to generate job number:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
		job.JobNumber = jobNumber
	}

	if job.Date == nil {
		now := time.Now()
		job.Date = &now
	}

	job.CreatedBy = &userID
	job.UpdatedBy = &userID
	now := time.Now()
	job.CreatedAt = &now
	job.UpdatedAt = &now

	if job.Status == "" {
		job.Status = "open"
	}

	if errs := job.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := job.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := job.Insert(); err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to insert repair job:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = job
	json.NewEncoder(w).Encode(response)
}

// ViewRepairJob : handler for GET /repair-job/{id}
func ViewRepairJob(w http.ResponseWriter, r *http.Request) {
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
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	job, err := store.FindRepairJobByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find repair job:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = job
	json.NewEncoder(w).Encode(response)
}

// UpdateRepairJob : handler for PUT /repair-job/{id}
func UpdateRepairJob(w http.ResponseWriter, r *http.Request) {
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
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	job, err := store.FindRepairJobByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find repair job:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &job) {
		return
	}

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	job.UpdatedBy = &userID

	if errs := job.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := job.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := job.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update repair job:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = job
	json.NewEncoder(w).Encode(response)
}

// DeleteRepairJob : handler for DELETE /repair-job/{id}
func DeleteRepairJob(w http.ResponseWriter, r *http.Request) {
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
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
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

	job, err := store.FindRepairJobByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find repair job:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := job.Delete(tokenClaims); err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete repair job:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"
	json.NewEncoder(w).Encode(response)
}
