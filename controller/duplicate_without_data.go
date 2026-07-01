package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── Job registry ───────────────────────────────────────────────────────────────

type DuplicateWithoutDataJob struct {
	JobID        string
	OldStoreID   string
	NewStoreID   string
	NewStoreName string
	steps        []*DuplicateStepData
	done         bool
	errMsg       string
	mu           sync.Mutex
}

var (
	duplicateWithoutDataJobs   = make(map[string]*DuplicateWithoutDataJob)
	duplicateWithoutDataJobsMu sync.Mutex
)

func newDuplicateWithoutDataJob(oldStoreID, newStoreID, newStoreName string) *DuplicateWithoutDataJob {
	jobID := fmt.Sprintf("dwd_%d", time.Now().UnixNano())
	job := &DuplicateWithoutDataJob{
		JobID:        jobID,
		OldStoreID:   oldStoreID,
		NewStoreID:   newStoreID,
		NewStoreName: newStoreName,
		steps: []*DuplicateStepData{
			{ID: "copy_store_doc", Name: "Create new store record", Status: "pending"},
			{ID: "create_store_db", Name: "Initialize new store database", Status: "pending"},
			{ID: "create_indexes", Name: "Create database indexes", Status: "pending"},
		},
	}
	duplicateWithoutDataJobsMu.Lock()
	duplicateWithoutDataJobs[jobID] = job
	duplicateWithoutDataJobsMu.Unlock()
	return job
}

func getDuplicateWithoutDataJob(jobID string) *DuplicateWithoutDataJob {
	duplicateWithoutDataJobsMu.Lock()
	defer duplicateWithoutDataJobsMu.Unlock()
	return duplicateWithoutDataJobs[jobID]
}

func deleteDuplicateWithoutDataJob(jobID string) {
	duplicateWithoutDataJobsMu.Lock()
	delete(duplicateWithoutDataJobs, jobID)
	duplicateWithoutDataJobsMu.Unlock()
}

func (j *DuplicateWithoutDataJob) setStep(stepID, status string, progress float64, msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, s := range j.steps {
		if s.ID == stepID {
			s.Status = status
			s.Progress = progress
			if msg != "" {
				s.Message = msg
			}
			return
		}
	}
}

func (j *DuplicateWithoutDataJob) fail(stepID, msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, s := range j.steps {
		if s.ID == stepID {
			s.Status = "error"
			s.Message = msg
		}
	}
	j.errMsg = msg
	j.done = true
}

func (j *DuplicateWithoutDataJob) snapshot() DuplicateProgressData {
	j.mu.Lock()
	defer j.mu.Unlock()
	steps := make([]DuplicateStepData, len(j.steps))
	doneCount := 0
	for i, s := range j.steps {
		steps[i] = *s
		if s.Status == "done" {
			doneCount++
		}
	}
	overall := float64(doneCount) / float64(len(j.steps)) * 100.0
	p := DuplicateProgressData{
		Steps:           steps,
		OverallProgress: overall,
		Done:            j.done,
		Error:           j.errMsg,
	}
	if j.done && j.errMsg == "" {
		p.NewStoreID = j.NewStoreID
		p.NewStoreName = j.NewStoreName
	}
	return p
}

// ── GET /v1/store/{id}/duplicate-without-data/size ────────────────────────────

type DuplicateWithoutDataSizeResult struct {
	StoreDocSize int64 `json:"store_doc_size"`
	TotalSize    int64 `json:"total_size"`
}

func GetStoreDuplicateWithoutDataSize(w http.ResponseWriter, r *http.Request) {
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

	vars := mux.Vars(r)
	storeID := vars["id"]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := DuplicateWithoutDataSizeResult{}

	mainDB := db.GetDB("")
	storeOID, _ := primitive.ObjectIDFromHex(storeID)
	var storeDoc bson.M
	if err := mainDB.Collection("store").FindOne(ctx, bson.M{"_id": storeOID}).Decode(&storeDoc); err == nil {
		if raw, err := bson.Marshal(storeDoc); err == nil {
			result.StoreDocSize = int64(len(raw))
		}
	}

	result.TotalSize = result.StoreDocSize

	response.Status = true
	response.Result = result
	json.NewEncoder(w).Encode(response)
}

// ── POST /v1/store/{id}/duplicate-without-data/start ─────────────────────────

func StartStoreDuplicateWithoutData(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID"
		json.NewEncoder(w).Encode(response)
		return
	}

	accessingUser, err := models.FindUserByID(&userID, bson.M{})
	if err != nil || accessingUser.Role != "Admin" {
		w.WriteHeader(http.StatusForbidden)
		response.Status = false
		response.Errors["user_id"] = "unauthorized access"
		json.NewEncoder(w).Encode(response)
		return
	}

	vars := mux.Vars(r)
	oldStoreID := vars["id"]

	var body struct {
		NewName         string `json:"new_name"`
		NewNameInArabic string `json:"new_name_in_arabic"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	if body.NewName == "" {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["new_name"] = "new_name is required"
		json.NewEncoder(w).Encode(response)
		return
	}

	newStoreOID := primitive.NewObjectID()
	newStoreIDStr := newStoreOID.Hex()

	job := newDuplicateWithoutDataJob(oldStoreID, newStoreIDStr, body.NewName)
	go runDuplicateWithoutDataJob(job, oldStoreID, newStoreOID, body.NewName, body.NewNameInArabic, userID)

	response.Status = true
	response.Result = map[string]string{"job_id": job.JobID, "new_store_id": newStoreIDStr}
	json.NewEncoder(w).Encode(response)
}

// ── GET /v1/store/{id}/duplicate-without-data/progress?job_id=xxx ────────────

func GetStoreDuplicateWithoutDataProgress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
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

	vars := mux.Vars(r)
	storeID := vars["id"]
	jobID := r.URL.Query().Get("job_id")

	if jobID == "" {
		response.Status = false
		response.Errors["job_id"] = "job_id required"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	job := getDuplicateWithoutDataJob(jobID)
	if job == nil || job.OldStoreID != storeID {
		response.Status = false
		response.Errors["job_id"] = "Job not found or expired"
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(response)
		return
	}

	snap := job.snapshot()
	response.Status = true
	response.Result = snap
	json.NewEncoder(w).Encode(response)
}

// ── Core duplicate-without-data logic ─────────────────────────────────────────

func runDuplicateWithoutDataJob(
	job *DuplicateWithoutDataJob,
	oldStoreIDStr string,
	newStoreOID primitive.ObjectID,
	newName string,
	newNameInArabic string,
	createdByUserID primitive.ObjectID,
) {
	ctx := context.Background()
	newStoreIDStr := newStoreOID.Hex()
	oldStoreOID, _ := primitive.ObjectIDFromHex(oldStoreIDStr)

	defer func() {
		if r := recover(); r != nil {
			job.fail("create_indexes", fmt.Sprintf("unexpected error: %v", r))
		}
	}()

	mainDB := db.GetDB("")

	// ── Step 1: Copy & modify store document in main DB ───────────────────────
	job.setStep("copy_store_doc", "running", 0, "")

	var storeDoc bson.M
	if err := mainDB.Collection("store").FindOne(ctx, bson.M{"_id": oldStoreOID}).Decode(&storeDoc); err != nil {
		job.fail("copy_store_doc", "Cannot find source store: "+err.Error())
		return
	}

	origNameInArabic, _ := storeDoc["name_in_arabic"].(string)
	finalNameInArabic := newNameInArabic
	if finalNameInArabic == "" {
		finalNameInArabic = origNameInArabic
	}

	now := time.Now()
	storeDoc["_id"] = newStoreOID
	storeDoc["name"] = newName
	storeDoc["name_in_arabic"] = finalNameInArabic
	storeDoc["created_at"] = now
	storeDoc["updated_at"] = now
	storeDoc["created_by"] = createdByUserID
	storeDoc["updated_by"] = createdByUserID
	storeDoc["created_by_name"] = ""
	storeDoc["updated_by_name"] = ""
	delete(storeDoc, "deleted")
	delete(storeDoc, "deleted_by")
	delete(storeDoc, "deleted_at")
	delete(storeDoc, "deleted_by_name")

	// Disconnect any Zatca Phase 2 credentials
	if zatca, ok := storeDoc["zatca"].(bson.M); ok {
		if phase, _ := zatca["phase"].(string); phase == "2" {
			zatca["connected"] = false
			delete(zatca, "otp")
			delete(zatca, "private_key")
			delete(zatca, "csr")
			delete(zatca, "binary_security_token")
			delete(zatca, "secret")
			delete(zatca, "production_binary_security_token")
			delete(zatca, "production_secret")
			delete(zatca, "compliance_request_id")
			delete(zatca, "production_request_id")
			delete(zatca, "last_connected_at")
			delete(zatca, "connected_by")
			delete(zatca, "disconnected_by")
			delete(zatca, "last_disconnected_at")
			delete(zatca, "connection_failed_count")
			delete(zatca, "connection_errors")
			delete(zatca, "connection_last_failed_at")
			storeDoc["zatca"] = zatca
		}
	}

	if _, err := mainDB.Collection("store").InsertOne(ctx, storeDoc); err != nil {
		job.fail("copy_store_doc", "Insert new store failed: "+err.Error())
		return
	}
	job.setStep("copy_store_doc", "done", 100, "New store record created with ID: "+newStoreIDStr)

	// ── Step 2: Initialize new store database ────────────────────────────────
	job.setStep("create_store_db", "running", 50, "")
	_ = db.GetDB("store_" + newStoreIDStr)
	job.setStep("create_store_db", "done", 100, "Database store_"+newStoreIDStr+" initialized")

	// ── Step 3: Create indexes on new store DB ───────────────────────────────
	job.setStep("create_indexes", "running", 50, "")
	newStore := &models.Store{}
	newStore.ID = newStoreOID
	if idxErr := newStore.CreateAllIndexes(); idxErr != nil {
		job.fail("create_indexes", "index creation failed: "+idxErr.Error())
		return
	}
	job.setStep("create_indexes", "done", 100, "")

	// Mark job complete — auto-clean after 5 minutes
	job.mu.Lock()
	job.done = true
	job.mu.Unlock()

	go func() {
		time.Sleep(5 * time.Minute)
		deleteDuplicateWithoutDataJob(job.JobID)
	}()
}
