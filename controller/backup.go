package controller

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── Types ──────────────────────────────────────────────────────────────────────

type BackupStepData struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Status    string  `json:"status"` // pending | running | done | error
	Progress  float64 `json:"progress"`
	Message   string  `json:"message,omitempty"`
}

type BackupProgressData struct {
	Steps           []BackupStepData `json:"steps"`
	OverallProgress float64          `json:"overall_progress"`
	Done            bool             `json:"done"`
	Error           string           `json:"error,omitempty"`
	FileToken       string           `json:"file_token,omitempty"`
}

type BackupSizeResult struct {
	MongoDBStoreDB  int64 `json:"mongodb_store_db"`
	MongoDBStoreDoc int64 `json:"mongodb_store_doc"`
	MongoDBUsers    int64 `json:"mongodb_users"`
	MongoDBTotal    int64 `json:"mongodb_total"`
	ImagesSize      int64 `json:"images_size"`
	ZatcaSize       int64 `json:"zatca_size"`
	TotalSize       int64 `json:"total_size"`
}

// ── Job registry ───────────────────────────────────────────────────────────────

type BackupJob struct {
	JobID   string
	StoreID string
	steps   []*BackupStepData
	done    bool
	errMsg  string
	zipPath string
	mu      sync.Mutex
}

var (
	backupJobs   = make(map[string]*BackupJob)
	backupJobsMu sync.Mutex
)

func newBackupJob(storeID string) *BackupJob {
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	job := &BackupJob{
		JobID:   jobID,
		StoreID: storeID,
		steps: []*BackupStepData{
			{ID: "mongodb_store_db", Name: "Export MongoDB store_" + storeID, Status: "pending"},
			{ID: "mongodb_main_doc", Name: "Export store document from main DB", Status: "pending"},
			{ID: "mongodb_users", Name: "Export users assigned to this store (main DB)", Status: "pending"},
			{ID: "images", Name: "Copy images/" + storeID, Status: "pending"},
			{ID: "zatca", Name: "Copy zatca/" + storeID, Status: "pending"},
			{ID: "zip", Name: "Create ZIP archive", Status: "pending"},
		},
	}
	backupJobsMu.Lock()
	backupJobs[jobID] = job
	backupJobsMu.Unlock()
	return job
}

func getBackupJob(jobID string) *BackupJob {
	backupJobsMu.Lock()
	defer backupJobsMu.Unlock()
	return backupJobs[jobID]
}

func deleteBackupJob(jobID string) {
	backupJobsMu.Lock()
	j, ok := backupJobs[jobID]
	if ok {
		delete(backupJobs, jobID)
	}
	backupJobsMu.Unlock()
	if ok && j.zipPath != "" {
		os.Remove(j.zipPath)
	}
}

func (j *BackupJob) setStep(stepID, status string, progress float64, msg string) {
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

func (j *BackupJob) fail(stepID, msg string) {
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

func (j *BackupJob) snapshot() BackupProgressData {
	j.mu.Lock()
	defer j.mu.Unlock()
	steps := make([]BackupStepData, len(j.steps))
	doneCount := 0
	for i, s := range j.steps {
		steps[i] = *s
		if s.Status == "done" {
			doneCount++
		}
	}
	overall := float64(doneCount) / float64(len(j.steps)) * 100.0
	p := BackupProgressData{
		Steps:           steps,
		OverallProgress: overall,
		Done:            j.done,
		Error:           j.errMsg,
	}
	if j.done && j.errMsg == "" {
		p.FileToken = j.JobID
	}
	return p
}

// ── Helper: directory size ─────────────────────────────────────────────────────

func backupDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// ── GET /v1/store/{id}/backup/size ─────────────────────────────────────────────

func GetStoreBackupSize(w http.ResponseWriter, r *http.Request) {
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

	result := BackupSizeResult{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// MongoDB store_<id> database stats
	storeDB := db.GetDB("store_" + storeID)
	var dbStats bson.M
	if err := storeDB.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&dbStats); err == nil {
		if sz, ok := dbStats["dataSize"].(float64); ok {
			result.MongoDBStoreDB = int64(sz)
		}
	}

	// Store document size from main DB
	mainDB := db.GetDB("")
	oidHex, _ := primitive.ObjectIDFromHex(storeID)
	var storeDoc bson.M
	if err := mainDB.Collection("store").FindOne(ctx, bson.M{"_id": oidHex}).Decode(&storeDoc); err == nil {
		if raw, err := bson.Marshal(storeDoc); err == nil {
			result.MongoDBStoreDoc = int64(len(raw))
		}
	}

	// Estimate size of users assigned to this store
	storeOID, _ := primitive.ObjectIDFromHex(storeID)
	userCursor, err := mainDB.Collection("user").Find(ctx, bson.M{"store_ids": storeOID})
	if err == nil {
		for userCursor.Next(ctx) {
			var raw bson.Raw
			if userCursor.Decode(&raw) == nil {
				result.MongoDBUsers += int64(len(raw))
			}
		}
		userCursor.Close(ctx)
	}

	result.MongoDBTotal = result.MongoDBStoreDB + result.MongoDBStoreDoc + result.MongoDBUsers
	result.ImagesSize, _ = backupDirSize("images/" + storeID)
	result.ZatcaSize, _ = backupDirSize("zatca/" + storeID)
	result.TotalSize = result.MongoDBTotal + result.ImagesSize + result.ZatcaSize

	response.Status = true
	response.Result = result
	json.NewEncoder(w).Encode(response)
}

// ── POST /v1/store/{id}/backup/start ──────────────────────────────────────────

func StartStoreBackup(w http.ResponseWriter, r *http.Request) {
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

	job := newBackupJob(storeID)
	go runBackupJob(job)

	response.Status = true
	response.Result = map[string]string{"job_id": job.JobID}
	json.NewEncoder(w).Encode(response)
}

// ── GET /v1/store/{id}/backup/progress?job_id=xxx ─────────────────────────────
// Returns a plain JSON snapshot of the current backup job state.
// The frontend polls this endpoint every 500 ms.

func GetStoreBackupProgress(w http.ResponseWriter, r *http.Request) {
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

	job := getBackupJob(jobID)
	if job == nil || job.StoreID != storeID {
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

// ── GET /v1/store/{id}/backup/file?job_id=xxx ─────────────────────────────────

func DownloadStoreBackupFile(w http.ResponseWriter, r *http.Request) {
	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	storeID := vars["id"]
	jobID := r.URL.Query().Get("job_id")

	job := getBackupJob(jobID)
	if job == nil || job.StoreID != storeID {
		http.Error(w, "Invalid or expired job", http.StatusNotFound)
		return
	}

	job.mu.Lock()
	isDone := job.done
	hasErr := job.errMsg
	zipPath := job.zipPath
	job.mu.Unlock()

	if !isDone || hasErr != "" {
		http.Error(w, "Backup not ready", http.StatusConflict)
		return
	}

	if zipPath == "" {
		http.Error(w, "Backup file missing", http.StatusNotFound)
		return
	}

	fileName := filepath.Base(zipPath)
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	w.Header().Set("Content-Type", "application/zip")
	http.ServeFile(w, r, zipPath)

	// Clean up after the response is sent
	go func() {
		time.Sleep(60 * time.Second)
		deleteBackupJob(jobID)
	}()
}

// ── Core backup logic ──────────────────────────────────────────────────────────

func runBackupJob(job *BackupJob) {
	storeID := job.StoreID

	defer func() {
		if r := recover(); r != nil {
			job.fail("zip", fmt.Sprintf("unexpected error: %v", r))
		}
	}()

	now := time.Now().Format("20060102_150405")
	backupDirName := fmt.Sprintf("backup_%s_%s", storeID, now)
	tmpDir := filepath.Join(os.TempDir(), backupDirName)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		job.fail("mongodb_store_db", "Failed to create temp dir: "+err.Error())
		return
	}
	// Remove tmpDir when done regardless
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// ── Step 1: Export store_<id> MongoDB database ────────────────────────────
	job.setStep("mongodb_store_db", "running", 0, "")
	mongoExportDir := filepath.Join(tmpDir, "store_"+storeID)
	if err := os.MkdirAll(mongoExportDir, 0755); err != nil {
		job.fail("mongodb_store_db", "mkdir failed: "+err.Error())
		return
	}

	storeDB := db.GetDB("store_" + storeID)
	colNames, err := storeDB.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		job.fail("mongodb_store_db", "list collections failed: "+err.Error())
		return
	}

	total := len(colNames)
	if total == 0 {
		total = 1
	}
	for i, colName := range colNames {
		progress := float64(i) / float64(total) * 90.0
		job.setStep("mongodb_store_db", "running", progress, "Exporting "+colName+"...")

		cursor, err := storeDB.Collection(colName).Find(ctx, bson.D{})
		if err != nil {
			continue
		}

		outFile, err := os.Create(filepath.Join(mongoExportDir, colName+".json"))
		if err != nil {
			cursor.Close(ctx)
			continue
		}

		outFile.WriteString("[")
		first := true
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				continue
			}
			b, err := json.Marshal(doc)
			if err != nil {
				continue
			}
			if !first {
				outFile.WriteString(",\n")
			}
			outFile.Write(b)
			first = false
		}
		outFile.WriteString("]\n")
		outFile.Close()
		cursor.Close(ctx)
	}
	job.setStep("mongodb_store_db", "done", 100, fmt.Sprintf("Exported %d collections", len(colNames)))

	// ── Step 2: Export store document from main DB ────────────────────────────
	job.setStep("mongodb_main_doc", "running", 50, "")
	mainDB := db.GetDB("")
	oidHex, _ := primitive.ObjectIDFromHex(storeID)
	var storeDoc bson.M
	if err := mainDB.Collection("store").FindOne(ctx, bson.M{"_id": oidHex}).Decode(&storeDoc); err == nil {
		if b, err := json.Marshal(storeDoc); err == nil {
			os.WriteFile(filepath.Join(mongoExportDir, "store_document.json"), b, 0644)
		}
	}
	job.setStep("mongodb_main_doc", "done", 100, "")

	// ── Step 3: Export users assigned to this store from main DB ─────────────
	job.setStep("mongodb_users", "running", 50, "")
	storeOID, _ := primitive.ObjectIDFromHex(storeID)
	userCursor, err := mainDB.Collection("user").Find(ctx, bson.M{"store_ids": storeOID})
	if err == nil {
		userFile, ferr := os.Create(filepath.Join(mongoExportDir, "users.json"))
		if ferr == nil {
			userFile.WriteString("[")
			first := true
			for userCursor.Next(ctx) {
				var udoc bson.M
				if derr := userCursor.Decode(&udoc); derr != nil {
					continue
				}
				// Remove sensitive password field from backup
				delete(udoc, "password")
				b, merr := json.Marshal(udoc)
				if merr != nil {
					continue
				}
				if !first {
					userFile.WriteString(",\n")
				}
				userFile.Write(b)
				first = false
			}
			userFile.WriteString("]\n")
			userFile.Close()
		}
		userCursor.Close(ctx)
	}
	job.setStep("mongodb_users", "done", 100, "")

	// ── Step 4: Copy images/<storeID> ─────────────────────────────────────────
	job.setStep("images", "running", 0, "")
	srcImages := filepath.Join("images", storeID)
	dstImages := filepath.Join(tmpDir, "images", storeID)
	if info, err := os.Stat(srcImages); err == nil && info.IsDir() {
		totalSize, _ := backupDirSize(srcImages)
		var doneSize int64
		if err := backupCopyDir(srcImages, dstImages, func(fileSize int64) {
			doneSize += fileSize
			if totalSize > 0 {
				job.setStep("images", "running", float64(doneSize)/float64(totalSize)*100, "")
			}
		}); err != nil {
			job.fail("images", "copy failed: "+err.Error())
			return
		}
	}
	job.setStep("images", "done", 100, "")

	// ── Step 4: Copy zatca/<storeID> ──────────────────────────────────────────
	job.setStep("zatca", "running", 0, "")
	srcZatca := filepath.Join("zatca", storeID)
	dstZatca := filepath.Join(tmpDir, "zatca", storeID)
	if info, err := os.Stat(srcZatca); err == nil && info.IsDir() {
		totalSize, _ := backupDirSize(srcZatca)
		var doneSize int64
		if err := backupCopyDir(srcZatca, dstZatca, func(fileSize int64) {
			doneSize += fileSize
			if totalSize > 0 {
				job.setStep("zatca", "running", float64(doneSize)/float64(totalSize)*100, "")
			}
		}); err != nil {
			job.fail("zatca", "copy failed: "+err.Error())
			return
		}
	}
	job.setStep("zatca", "done", 100, "")

	// ── Step 5: Create ZIP archive ─────────────────────────────────────────────
	job.setStep("zip", "running", 0, "")
	zipPath := filepath.Join(os.TempDir(), backupDirName+".zip")
	if err := backupCreateZip(tmpDir, zipPath, func(pct float64) {
		job.setStep("zip", "running", pct, "")
	}); err != nil {
		job.fail("zip", "zip creation failed: "+err.Error())
		return
	}
	job.setStep("zip", "done", 100, "")

	job.mu.Lock()
	job.zipPath = zipPath
	job.done = true
	job.mu.Unlock()
}

func backupCopyDir(src, dst string, onFile func(int64)) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		if err := backupCopyFile(path, dstPath); err != nil {
			return err
		}
		if onFile != nil {
			onFile(info.Size())
		}
		return nil
	})
}

func backupCopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func backupCreateZip(srcDir, dstZip string, onProgress func(float64)) error {
	var totalFiles, doneFiles int
	filepath.Walk(srcDir, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			totalFiles++
		}
		return nil
	})
	if totalFiles == 0 {
		totalFiles = 1
	}

	f, err := os.Create(dstZip)
	if err != nil {
		return err
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		entry, err := zw.Create(rel)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		if _, err := io.Copy(entry, src); err != nil {
			return err
		}
		doneFiles++
		if onProgress != nil {
			onProgress(float64(doneFiles) / float64(totalFiles) * 100.0)
		}
		return nil
	})
}
