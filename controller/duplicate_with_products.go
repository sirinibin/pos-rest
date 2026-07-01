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

// ── Job registry ───────────────────────────────────────────────────────────────

type DuplicateWithProductsJob struct {
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
	duplicateWithProductsJobs   = make(map[string]*DuplicateWithProductsJob)
	duplicateWithProductsJobsMu sync.Mutex
)

func newDuplicateWithProductsJob(oldStoreID, newStoreID, newStoreName string) *DuplicateWithProductsJob {
	jobID := fmt.Sprintf("dwp_%d", time.Now().UnixNano())
	job := &DuplicateWithProductsJob{
		JobID:        jobID,
		OldStoreID:   oldStoreID,
		NewStoreID:   newStoreID,
		NewStoreName: newStoreName,
		steps: []*DuplicateStepData{
			{ID: "copy_store_doc", Name: "Create new store record", Status: "pending"},
			{ID: "create_store_db", Name: "Initialize new store database", Status: "pending"},
			{ID: "copy_product_categories", Name: "Copy product categories", Status: "pending"},
			{ID: "copy_product_brands", Name: "Copy product brands", Status: "pending"},
			{ID: "copy_products", Name: "Copy products", Status: "pending"},
			{ID: "update_store_refs", Name: "Update store references in catalog", Status: "pending"},
			{ID: "update_main_db_refs", Name: "Update product store keys in main DB", Status: "pending"},
			{ID: "clear_product_stats", Name: "Clear product stats & warehouse data", Status: "pending"},
			{ID: "copy_images", Name: "Copy product images", Status: "pending"},
			{ID: "create_indexes", Name: "Create database indexes", Status: "pending"},
		},
	}
	duplicateWithProductsJobsMu.Lock()
	duplicateWithProductsJobs[jobID] = job
	duplicateWithProductsJobsMu.Unlock()
	return job
}

func getDuplicateWithProductsJob(jobID string) *DuplicateWithProductsJob {
	duplicateWithProductsJobsMu.Lock()
	defer duplicateWithProductsJobsMu.Unlock()
	return duplicateWithProductsJobs[jobID]
}

func deleteDuplicateWithProductsJob(jobID string) {
	duplicateWithProductsJobsMu.Lock()
	delete(duplicateWithProductsJobs, jobID)
	duplicateWithProductsJobsMu.Unlock()
}

func (j *DuplicateWithProductsJob) setStep(stepID, status string, progress float64, msg string) {
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

func (j *DuplicateWithProductsJob) fail(stepID, msg string) {
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

func (j *DuplicateWithProductsJob) snapshot() DuplicateProgressData {
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

// ── GET /v1/store/{id}/duplicate-with-products/size ───────────────────────────

type DuplicateWithProductsSizeResult struct {
	StoreDocSize           int64 `json:"store_doc_size"`
	CatalogCollectionsSize int64 `json:"catalog_collections_size"`
	ImagesSize             int64 `json:"images_size"`
	TotalSize              int64 `json:"total_size"`
}

func GetStoreDuplicateWithProductsSize(w http.ResponseWriter, r *http.Request) {
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

	result := DuplicateWithProductsSizeResult{}

	// Store document size from main DB
	mainDB := db.GetDB("")
	storeOID, _ := primitive.ObjectIDFromHex(storeID)
	var storeDoc bson.M
	if err := mainDB.Collection("store").FindOne(ctx, bson.M{"_id": storeOID}).Decode(&storeDoc); err == nil {
		if raw, err := bson.Marshal(storeDoc); err == nil {
			result.StoreDocSize = int64(len(raw))
		}
	}

	storeDB := db.GetDB("store_" + storeID)
	for _, colName := range []string{"product", "product_brand", "product_category"} {
		var colStats bson.M
		if err := storeDB.RunCommand(ctx, bson.D{
			{Key: "collStats", Value: colName},
		}).Decode(&colStats); err == nil {
			switch sz := colStats["size"].(type) {
			case float64:
				result.CatalogCollectionsSize += int64(sz)
			case int32:
				result.CatalogCollectionsSize += int64(sz)
			case int64:
				result.CatalogCollectionsSize += sz
			}
		}
	}

	result.ImagesSize, _ = backupDirSize("images/" + storeID)
	result.TotalSize = result.StoreDocSize + result.CatalogCollectionsSize + result.ImagesSize

	response.Status = true
	response.Result = result
	json.NewEncoder(w).Encode(response)
}

// ── POST /v1/store/{id}/duplicate-with-products/start ────────────────────────

func StartStoreDuplicateWithProducts(w http.ResponseWriter, r *http.Request) {
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

	job := newDuplicateWithProductsJob(oldStoreID, newStoreIDStr, body.NewName)
	go runDuplicateWithProductsJob(job, oldStoreID, newStoreOID, body.NewName, body.NewNameInArabic, userID)

	response.Status = true
	response.Result = map[string]string{"job_id": job.JobID, "new_store_id": newStoreIDStr}
	json.NewEncoder(w).Encode(response)
}

// ── GET /v1/store/{id}/duplicate-with-products/progress?job_id=xxx ───────────

func GetStoreDuplicateWithProductsProgress(w http.ResponseWriter, r *http.Request) {
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

	job := getDuplicateWithProductsJob(jobID)
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

// ── Core duplicate-with-products logic ────────────────────────────────────────

func runDuplicateWithProductsJob(
	job *DuplicateWithProductsJob,
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
	newStoreDB := db.GetDB("store_" + newStoreIDStr)
	oldStoreDB := db.GetDB("store_" + oldStoreIDStr)
	job.setStep("create_store_db", "done", 100, "Database store_"+newStoreIDStr+" initialized")

	// ── Steps 3–5: Copy only catalog collections ─────────────────────────────
	catalogCollections := []struct {
		stepID  string
		colName string
	}{
		{"copy_product_categories", "product_category"},
		{"copy_product_brands", "product_brand"},
		{"copy_products", "product"},
	}

	for _, c := range catalogCollections {
		job.setStep(c.stepID, "running", 0, "")

		cursor, err := oldStoreDB.Collection(c.colName).Find(ctx, bson.D{})
		if err != nil {
			job.fail(c.stepID, "find failed: "+err.Error())
			return
		}
		var docs []interface{}
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				continue
			}
			doc = replaceStoreID(doc, oldStoreIDStr, oldStoreOID, newStoreIDStr, newStoreOID)
			docs = append(docs, doc)
		}
		cursor.Close(ctx)

		if len(docs) > 0 {
			if _, err := newStoreDB.Collection(c.colName).InsertMany(ctx, docs); err != nil {
				job.fail(c.stepID, "insert failed: "+err.Error())
				return
			}
		}
		job.setStep(c.stepID, "done", 100, fmt.Sprintf("Copied %d documents", len(docs)))
	}

	// ── Step 6: Update store_id / store_name refs in catalog collections ──────
	job.setStep("update_store_refs", "running", 0, "")
	updateFields := bson.M{
		"store_id":             newStoreOID,
		"store_name":           newName,
		"store_name_in_arabic": finalNameInArabic,
	}
	for i, c := range catalogCollections {
		progress := float64(i) / float64(len(catalogCollections)) * 100.0
		job.setStep("update_store_refs", "running", progress, "Updating "+c.colName+"…")
		_, _ = newStoreDB.Collection(c.colName).UpdateMany(
			ctx,
			bson.M{"store_id": oldStoreOID},
			bson.M{"$set": updateFields},
		)
	}
	job.setStep("update_store_refs", "done", 100, "")

	// ── Step 7: Copy product_stores key in main DB product collection ─────────
	job.setStep("update_main_db_refs", "running", 0, "")
	productMainCol := mainDB.Collection("product")
	cur, err := productMainCol.Find(ctx, bson.M{"product_stores." + oldStoreIDStr: bson.M{"$exists": true}})
	if err == nil {
		for cur.Next(ctx) {
			var doc bson.M
			if err := cur.Decode(&doc); err != nil {
				continue
			}
			storeMap, ok := doc["product_stores"].(bson.M)
			if !ok {
				continue
			}
			oldData, exists := storeMap[oldStoreIDStr]
			if !exists {
				continue
			}
			_, _ = productMainCol.UpdateOne(ctx,
				bson.M{"_id": doc["_id"]},
				bson.M{"$set": bson.M{"product_stores." + newStoreIDStr: oldData}},
			)
		}
		cur.Close(ctx)
	}
	job.setStep("update_main_db_refs", "done", 100, "")

	// ── Step 8: Clear all stats & warehouse data from products ─────────────────
	job.setStep("clear_product_stats", "running", 0, "")

	storePrefix := "product_stores." + newStoreIDStr + "."

	numericStatsFields := []string{
		"stock", "stocks_added", "stocks_removed",
		"sales_count", "sales_quantity", "sales",
		"sales_return_count", "sales_return_quantity", "sales_return",
		"sales_profit", "sales_loss", "sales_return_profit", "sales_return_loss",
		"quotation_sales_count", "quotation_sales_quantity", "quotation_sales",
		"quotation_sales_profit", "quotation_sales_loss",
		"quotation_sales_return_count", "quotation_sales_return_quantity",
		"quotation_sales_return", "quotation_sales_return_profit", "quotation_sales_return_loss",
		"purchase_count", "purchase_quantity", "purchase",
		"purchase_return_count", "purchase_return_quantity", "purchase_return",
		"quotation_count", "quotation_quantity", "quotation",
		"delivery_note_count", "delivery_note_quantity",
		"stocktransfer_amount", "stocktransfer_count", "stocktransfer_quantity",
		"retail_unit_profit", "retail_unit_profit_perc",
		"wholesale_unit_profit", "wholesale_unit_profit_perc",
	}

	setFields := bson.M{}
	for _, f := range numericStatsFields {
		setFields[storePrefix+f] = 0
	}

	unsetFields := bson.M{
		// Warehouse maps and stock history
		storePrefix + "warehouse_stocks":   "",
		storePrefix + "warehouse_racks":    "",
		storePrefix + "product_warehouses": "",
		storePrefix + "stock_adjustments":  "",
		// Top-level BI / velocity fields
		"sales_velocity_trend":        "",
		"sales_velocity_trend_reason": "",
		"slop_percent_per_month":      "",
		"momentum_percent_per_3month": "",
		"avg_monthly_qty":             "",
		"recent_3month_qty":           "",
		"revenue":                     "",
		"class":                       "",
		"class_reason":                "",
		"abc_tier":                    "",
		"xyz_tier":                    "",
		"cv":                          "",
		"active_months":               "",
		"stocking_strategy":           "",
	}

	_, err = newStoreDB.Collection("product").UpdateMany(ctx, bson.M{}, bson.M{
		"$set":   setFields,
		"$unset": unsetFields,
	})
	if err != nil {
		job.fail("clear_product_stats", "failed to clear stats: "+err.Error())
		return
	}
	job.setStep("clear_product_stats", "done", 100, "")

	// ── Step 9: Copy images/<oldID> → images/<newID> ─────────────────────────
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

	// ── Step 10: Create indexes on new store DB ───────────────────────────────
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
		deleteDuplicateWithProductsJob(job.JobID)
	}()
}
