package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── Types ──────────────────────────────────────────────────────────────────────

type DuplicateStepData struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Status   string  `json:"status"` // pending | running | done | error
	Progress float64 `json:"progress"`
	Message  string  `json:"message,omitempty"`
}

type DuplicateProgressData struct {
	Steps           []DuplicateStepData `json:"steps"`
	OverallProgress float64             `json:"overall_progress"`
	Done            bool                `json:"done"`
	Error           string              `json:"error,omitempty"`
	NewStoreID      string              `json:"new_store_id,omitempty"`
	NewStoreName    string              `json:"new_store_name,omitempty"`
}

// ── Job registry ───────────────────────────────────────────────────────────────

type DuplicateJob struct {
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
	duplicateJobs   = make(map[string]*DuplicateJob)
	duplicateJobsMu sync.Mutex
)

func newDuplicateJob(oldStoreID, newStoreID, newStoreName string) *DuplicateJob {
	jobID := fmt.Sprintf("dup_%d", time.Now().UnixNano())
	job := &DuplicateJob{
		JobID:        jobID,
		OldStoreID:   oldStoreID,
		NewStoreID:   newStoreID,
		NewStoreName: newStoreName,
		steps: []*DuplicateStepData{
			{ID: "copy_store_doc", Name: "Create new store record", Status: "pending"},
			{ID: "create_store_db", Name: "Initialize new store database", Status: "pending"},
			{ID: "copy_collections", Name: "Copy MongoDB collections", Status: "pending"},
			{ID: "update_store_refs", Name: "Update store references in all records", Status: "pending"},
			{ID: "copy_images", Name: "Copy store images", Status: "pending"},
			{ID: "copy_zatca", Name: "Copy Zatca files", Status: "pending"},
			{ID: "create_indexes", Name: "Create database indexes", Status: "pending"},
		},
	}
	duplicateJobsMu.Lock()
	duplicateJobs[jobID] = job
	duplicateJobsMu.Unlock()
	return job
}

func getDuplicateJob(jobID string) *DuplicateJob {
	duplicateJobsMu.Lock()
	defer duplicateJobsMu.Unlock()
	return duplicateJobs[jobID]
}

func deleteDuplicateJob(jobID string) {
	duplicateJobsMu.Lock()
	delete(duplicateJobs, jobID)
	duplicateJobsMu.Unlock()
}

func (j *DuplicateJob) setStep(stepID, status string, progress float64, msg string) {
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

func (j *DuplicateJob) fail(stepID, msg string) {
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

func (j *DuplicateJob) snapshot() DuplicateProgressData {
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

// ── GET /v1/store/{id}/duplicate/size ─────────────────────────────────────────
// Reuses the backup size logic since it measures the same data.

func GetStoreDuplicateSize(w http.ResponseWriter, r *http.Request) {
	GetStoreBackupSize(w, r)
}

// ── POST /v1/store/{id}/duplicate/start ───────────────────────────────────────

func StartStoreDuplicate(w http.ResponseWriter, r *http.Request) {
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

	job := newDuplicateJob(oldStoreID, newStoreIDStr, body.NewName)
	go runDuplicateJob(job, oldStoreID, newStoreOID, body.NewName, body.NewNameInArabic, userID)

	response.Status = true
	response.Result = map[string]string{"job_id": job.JobID, "new_store_id": newStoreIDStr}
	json.NewEncoder(w).Encode(response)
}

// ── GET /v1/store/{id}/duplicate/progress?job_id=xxx ──────────────────────────

func GetStoreDuplicateProgress(w http.ResponseWriter, r *http.Request) {
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

	job := getDuplicateJob(jobID)
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

// ── Core duplicate logic ───────────────────────────────────────────────────────

func runDuplicateJob(
	job *DuplicateJob,
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

	// Capture original Arabic name before overwriting
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

	// Zatca: if Phase 2 and connected, keep env=production but disconnect — user must reconnect manually
	if zatca, ok := storeDoc["zatca"].(bson.M); ok {
		if phase, _ := zatca["phase"].(string); phase == "2" {
			if connected, _ := zatca["connected"].(bool); connected {
				zatca["env"] = "production"
				zatca["connected"] = false
				zatca["otp"] = ""
				zatca["private_key"] = ""
				zatca["csr"] = ""
				zatca["binary_security_token"] = ""
				zatca["secret"] = ""
				zatca["production_binary_security_token"] = ""
				zatca["production_secret"] = ""
				zatca["compliance_request_id"] = int64(0)
				zatca["production_request_id"] = int64(0)
				zatca["last_connected_at"] = nil
				zatca["connected_by"] = nil
				zatca["disconnected_by"] = nil
				zatca["last_disconnected_at"] = nil
				zatca["connection_failed_count"] = int64(0)
				zatca["connection_errors"] = []string{}
				zatca["connection_last_failed_at"] = nil
				storeDoc["zatca"] = zatca
			}
		}
	}

	if _, err := mainDB.Collection("store").InsertOne(ctx, storeDoc); err != nil {
		job.fail("copy_store_doc", "Insert new store failed: "+err.Error())
		return
	}
	job.setStep("copy_store_doc", "done", 100, "New store record created with ID: "+newStoreIDStr)

	// ── Step 2: Initialize new store database ────────────────────────────────
	job.setStep("create_store_db", "running", 50, "")
	newStoreDB := db.GetDB("store_" + newStoreIDStr)
	job.setStep("create_store_db", "done", 100, "Database store_"+newStoreIDStr+" initialized")

	// ── Step 3: Copy all collections from old DB to new DB ───────────────────
	job.setStep("copy_collections", "running", 0, "")
	oldStoreDB := db.GetDB("store_" + oldStoreIDStr)

	colNames, err := oldStoreDB.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		job.fail("copy_collections", "list collections failed: "+err.Error())
		return
	}

	total := len(colNames)
	if total == 0 {
		total = 1
	}
	for i, colName := range colNames {
		progress := float64(i) / float64(total) * 100.0
		job.setStep("copy_collections", "running", progress, "Copying "+colName+"…")

		cursor, err := oldStoreDB.Collection(colName).Find(ctx, bson.D{})
		if err != nil {
			continue
		}
		var docs []interface{}
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				continue
			}
			docs = append(docs, doc)
		}
		cursor.Close(ctx)
		if len(docs) > 0 {
			_, _ = newStoreDB.Collection(colName).InsertMany(ctx, docs)
		}
	}
	job.setStep("copy_collections", "done", 100, fmt.Sprintf("Copied %d collections", len(colNames)))

	// ── Step 4: Update store_id / store_name / store_name_in_arabic ──────────
	job.setStep("update_store_refs", "running", 0, "")

	updatedColNames, _ := newStoreDB.ListCollectionNames(ctx, bson.D{})
	totalUpdate := len(updatedColNames)
	if totalUpdate == 0 {
		totalUpdate = 1
	}
	updateFields := bson.M{
		"store_id":              newStoreOID,
		"store_name":            newName,
		"store_name_in_arabic":  finalNameInArabic,
	}
	for i, colName := range updatedColNames {
		progress := float64(i) / float64(totalUpdate) * 100.0
		job.setStep("update_store_refs", "running", progress, "Updating "+colName+"…")
		_, _ = newStoreDB.Collection(colName).UpdateMany(
			ctx,
			bson.M{"store_id": oldStoreOID},
			bson.M{"$set": updateFields},
		)
	}
	job.setStep("update_store_refs", "done", 100, "")

	// ── Step 5: Copy images/<oldID> → images/<newID> ─────────────────────────
	job.setStep("copy_images", "running", 0, "")
	srcImages := "images/" + oldStoreIDStr
	dstImages := "images/" + newStoreIDStr
	if info, statErr := os.Stat(srcImages); statErr == nil && info.IsDir() {
		totalSize, _ := backupDirSize(srcImages)
		var doneSize int64
		if cpErr := backupCopyDir(srcImages, dstImages, func(fileSize int64) {
			doneSize += fileSize
			if totalSize > 0 {
				job.setStep("copy_images", "running", float64(doneSize)/float64(totalSize)*100, "")
			}
		}); cpErr != nil {
			job.fail("copy_images", "copy failed: "+cpErr.Error())
			return
		}
	}
	job.setStep("copy_images", "done", 100, "")

	// ── Step 6: Copy zatca/<oldID> → zatca/<newID> ───────────────────────────
	job.setStep("copy_zatca", "running", 0, "")
	srcZatca := "zatca/" + oldStoreIDStr
	dstZatca := "zatca/" + newStoreIDStr
	if info, statErr := os.Stat(srcZatca); statErr == nil && info.IsDir() {
		totalSize, _ := backupDirSize(srcZatca)
		var doneSize int64
		if cpErr := backupCopyDir(srcZatca, dstZatca, func(fileSize int64) {
			doneSize += fileSize
			if totalSize > 0 {
				job.setStep("copy_zatca", "running", float64(doneSize)/float64(totalSize)*100, "")
			}
		}); cpErr != nil {
			job.fail("copy_zatca", "copy failed: "+cpErr.Error())
			return
		}
	}
	job.setStep("copy_zatca", "done", 100, "")

	// ── Step 7: Create indexes on new store DB ───────────────────────────────
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
		deleteDuplicateJob(job.JobID)
	}()
}
