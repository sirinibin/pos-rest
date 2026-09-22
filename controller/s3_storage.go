package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// loadAdminS3Settings fetches global S3 settings from the admin_settings collection.
// Returns an empty AdminSettings on any error (safe to use — S3 will be disabled).
func loadAdminS3Settings() models.AdminSettings {
	s, err := models.GetAdminSettings()
	if err != nil || s == nil {
		return models.AdminSettings{}
	}
	return *s
}

// s3BaseURL returns the base URL for the bucket, honoring custom endpoint / public base URL.
func s3BaseURL(s models.AdminSettings) string {
	if s.S3PublicBaseURL != "" {
		return strings.TrimRight(s.S3PublicBaseURL, "/")
	}
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		return ep + "/" + s.S3BucketName
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com", s.S3BucketName, s.S3Region)
}

// s3Host returns the HTTPS host for signing requests.
func s3Host(s models.AdminSettings) string {
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		ep = strings.TrimPrefix(ep, "https://")
		ep = strings.TrimPrefix(ep, "http://")
		return ep + "/" + s.S3BucketName
	}
	return fmt.Sprintf("%s.s3.%s.amazonaws.com", s.S3BucketName, s.S3Region)
}

// s3PutURL returns the URL for a PutObject request.
func s3PutURL(s models.AdminSettings, key string) string {
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		return ep + "/" + s.S3BucketName + "/" + key
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.S3BucketName, s.S3Region, key)
}

// uploadToS3 uploads data to S3 using SigV4-signed PUT, returns the public file URL.
func uploadToS3(s models.AdminSettings, key string, data []byte, contentType string) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	now := time.Now().UTC()
	dateStr := now.Format("20060102")
	timeStr := now.Format("20060102T150405Z")
	region := s.S3Region
	if region == "" {
		region = "us-east-1"
	}

	bodyHash := fmt.Sprintf("%x", sha256sum(data))
	host := s3Host(s)
	putURL := s3PutURL(s, key)

	req, err := http.NewRequest("PUT", putURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", bodyHash)

	// Canonical request
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		contentType, host, bodyHash, timeStr)
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"

	// For path-style (custom endpoint), the path includes bucket; for virtual-hosted it doesn't.
	urlPath := "/" + key
	if s.S3Endpoint != "" {
		urlPath = "/" + s.S3BucketName + "/" + key
	}

	canonicalRequest := strings.Join([]string{"PUT", urlPath, "", canonicalHeaders, signedHeaders, bodyHash}, "\n")

	credentialScope := strings.Join([]string{dateStr, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timeStr,
		credentialScope,
		fmt.Sprintf("%x", sha256sum([]byte(canonicalRequest))),
	}, "\n")

	signingKey := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256(
		[]byte("AWS4"+s.S3SecretKey), []byte(dateStr)),
		[]byte(region)),
		[]byte("s3")),
		[]byte("aws4_request"))
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.S3AccessKeyID, credentialScope, signedHeaders, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("s3 put %s: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body) //nolint:errcheck
		return "", fmt.Errorf("s3 put %s: status %d: %s", key, resp.StatusCode, buf.String())
	}

	return s3BaseURL(s) + "/" + key, nil
}

// saveAttachment saves data to S3 if configured, or to local disk otherwise.
// relKey is the path without leading "./" or "/" e.g. "attachments/{storeID}/{msgID}/file.pdf".
// Always returns "/cdn/relKey" — the CdnFileHandler route serves the file from S3 or local disk.
func saveAttachment(settings models.AdminSettings, relKey string, data []byte, contentType string) string {
	if settings.S3Enabled && settings.S3BucketName != "" && settings.S3AccessKeyID != "" {
		_, err := uploadToS3(settings, relKey, data, contentType)
		if err != nil {
			log.Printf("s3: upload failed for %s, falling back to local disk: %v", relKey, err)
			// Fall through to local disk below
		} else {
			return "/cdn/" + relKey
		}
	}
	// Local disk fallback
	localPath := "./" + relKey
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return ""
	}
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return ""
	}
	return "/cdn/" + relKey
}

// CdnFileHandler serves attachment files via the /cdn/ path.
// If S3 is configured, proxies the file through a SigV4-signed GET (private bucket safe).
// Falls back to local disk if the key is not found in S3.
// Registered with router.PathPrefix("/cdn/")
func CdnFileHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/cdn/")
	if path == "" || strings.Contains(path, "..") {
		http.NotFound(w, r)
		return
	}
	s := loadAdminS3Settings()
	if s.S3Enabled && s.S3BucketName != "" {
		if proxyS3Object(s, path, w) {
			return
		}
		// Not found in S3 — fall back to local disk
	}
	http.ServeFile(w, r, "./"+path)
}

// proxyS3Object fetches key from S3 using a SigV4-signed GET and streams it to w.
// Returns true if the response was written (success or non-404 error), false if key was not found in S3.
func proxyS3Object(s models.AdminSettings, key string, w http.ResponseWriter) bool {
	now := time.Now().UTC()
	dateStr := now.Format("20060102")
	timeStr := now.Format("20060102T150405Z")
	region := s.S3Region
	if region == "" {
		region = "us-east-1"
	}
	emptyHash := fmt.Sprintf("%x", sha256sum([]byte{}))
	host := s3Host(s)
	getURL := s3PutURL(s, key)

	req, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		http.Error(w, "s3 request error", http.StatusInternalServerError)
		return true
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", emptyHash)

	urlPath := "/" + key
	if s.S3Endpoint != "" {
		urlPath = "/" + s.S3BucketName + "/" + key
	}
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyHash, timeStr)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{"GET", urlPath, "", canonicalHeaders, signedHeaders, emptyHash}, "\n")

	credentialScope := strings.Join([]string{dateStr, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", timeStr, credentialScope,
		fmt.Sprintf("%x", sha256sum([]byte(canonicalRequest))),
	}, "\n")
	signingKey := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256(
		[]byte("AWS4"+s.S3SecretKey), []byte(dateStr)),
		[]byte(region)), []byte("s3")), []byte("aws4_request"))
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.S3AccessKeyID, credentialScope, signedHeaders, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "s3 fetch error", http.StatusBadGateway)
		return true
	}
	defer resp.Body.Close()

	// 404 from S3 means the file hasn't been migrated yet — fall back to local disk
	if resp.StatusCode == http.StatusNotFound {
		return false
	}
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "s3 error", resp.StatusCode)
		return true
	}

	// Forward relevant headers
	for _, h := range []string{"Content-Type", "Content-Length", "Last-Modified", "ETag", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body) //nolint:errcheck
	return true
}

// ─── Test S3 Connection ───────────────────────────────────────────────────────

// TestS3ConnectionHandler tests S3 credentials by uploading and deleting a tiny probe file.
// Accepts the settings in the request body (so you can test before saving).
// POST /v1/admin-settings/test-s3
func TestS3ConnectionHandler(w http.ResponseWriter, r *http.Request) {
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

	// Decode settings from request body; fill missing fields from DB.
	var s models.AdminSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil || s.S3BucketName == "" {
		if db := loadAdminS3Settings(); db.S3BucketName != "" {
			s = db
		}
	} else if s.S3SecretKey == "" {
		// Body has bucket/key but no secret (masked) — load secret from DB
		if db := loadAdminS3Settings(); db.S3SecretKey != "" {
			s.S3SecretKey = db.S3SecretKey
		}
	}
	if s.S3BucketName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bucket name is missing — fill in your S3 settings and try again"})
		return
	}
	if s.S3AccessKeyID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Access Key ID is missing — fill in your S3 settings and try again"})
		return
	}
	if s.S3SecretKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Secret Key is missing — fill in your S3 settings and try again"})
		return
	}

	probeKey := fmt.Sprintf("startpos-s3-probe-%d.txt", time.Now().UnixNano())
	url, err := uploadToS3(s, probeKey, []byte("startpos s3 probe"), "text/plain")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	// Best-effort delete the probe file (ignore error)
	deleteFromS3(s, probeKey) //nolint:errcheck
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "url": url})
}

// deleteFromS3 sends a DELETE request for the given key.
func deleteFromS3(s models.AdminSettings, key string) error {
	now := time.Now().UTC()
	dateStr := now.Format("20060102")
	timeStr := now.Format("20060102T150405Z")
	region := s.S3Region
	if region == "" {
		region = "us-east-1"
	}

	emptyHash := fmt.Sprintf("%x", sha256sum([]byte{}))
	host := s3Host(s)
	delURL := s3PutURL(s, key) // same URL, different method

	req, err := http.NewRequest("DELETE", delURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", emptyHash)

	urlPath := "/" + key
	if s.S3Endpoint != "" {
		urlPath = "/" + s.S3BucketName + "/" + key
	}
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyHash, timeStr)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{"DELETE", urlPath, "", canonicalHeaders, signedHeaders, emptyHash}, "\n")

	credentialScope := strings.Join([]string{dateStr, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timeStr,
		credentialScope,
		fmt.Sprintf("%x", sha256sum([]byte(canonicalRequest))),
	}, "\n")

	signingKey := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256(
		[]byte("AWS4"+s.S3SecretKey), []byte(dateStr)),
		[]byte(region)),
		[]byte("s3")),
		[]byte("aws4_request"))
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.S3AccessKeyID, credentialScope, signedHeaders, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ─── Migrate existing files to S3 (streaming SSE progress) ──────────────────

// sendSSE writes one Server-Sent Events data line and flushes immediately.
func sendSSE(w http.ResponseWriter, f http.Flusher, v interface{}) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "data: %s\n\n", b)
	f.Flush()
}

// MigrateAttachmentsToS3Handler uploads all local ./attachments/{storeID}/ files to S3
// and streams real-time progress via Server-Sent Events.
// POST /v1/store/{id}/migrate-to-s3
func MigrateAttachmentsToS3Handler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}
	storeID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store id"})
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled — save your settings first"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	storeIDStr := storeID.Hex()
	localDir := "./attachments/" + storeIDStr

	// First pass: count total files
	total := 0
	filepath.WalkDir(localDir, func(_ string, d fs.DirEntry, e error) error { //nolint:errcheck
		if e == nil && !d.IsDir() {
			total++
		}
		return nil
	})
	sendSSE(w, flusher, map[string]interface{}{"type": "start", "total": total})

	urlMap := map[string]string{}
	uploaded, skipped, processed := 0, 0, 0

	walkErr := filepath.WalkDir(localDir, func(path string, d fs.DirEntry, we error) error {
		if we != nil || d.IsDir() {
			return nil
		}
		processed++
		pct := 0
		if total > 0 {
			pct = processed * 100 / total
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			skipped++
			sendSSE(w, flusher, map[string]interface{}{
				"type": "progress", "processed": processed, "total": total, "percent": pct,
				"current_file": d.Name(), "uploaded": uploaded, "skipped": skipped,
			})
			return nil
		}
		relKey := strings.TrimPrefix(filepath.ToSlash(path), "./")
		contentType := mimeFromFilename(d.Name())
		s3URL, uploadErr := uploadToS3(s, relKey, data, contentType)
		if uploadErr != nil {
			log.Printf("s3 migrate: failed to upload %s: %v", path, uploadErr)
			skipped++
		} else {
			localURL := "/" + relKey
			urlMap[localURL] = s3URL
			uploaded++
		}
		sendSSE(w, flusher, map[string]interface{}{
			"type": "progress", "processed": processed, "total": total, "percent": pct,
			"current_file": d.Name(), "uploaded": uploaded, "skipped": skipped,
		})
		return nil
	})

	if walkErr != nil && !os.IsNotExist(walkErr) {
		sendSSE(w, flusher, map[string]interface{}{"type": "error", "error": "walk failed: " + walkErr.Error()})
		return
	}

	updatedMessages := 0
	if len(urlMap) > 0 {
		sendSSE(w, flusher, map[string]interface{}{"type": "progress", "message": "Updating database links…"})
		updatedMessages = updateAttachmentURLsInMongo(storeID, urlMap)
	}

	sendSSE(w, flusher, map[string]interface{}{
		"type": "done", "uploaded": uploaded, "skipped": skipped, "updated_messages": updatedMessages,
	})
}

// MigrateAllStoresAttachmentsToS3Handler runs MigrateAttachmentsToS3 for every store.
// POST /v1/migrate-all-stores-to-s3
func MigrateAllStoresAttachmentsToS3Handler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		http.Error(w, "admin only", http.StatusForbidden)
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	stores, err := models.GetAllStores()
	if err != nil {
		sendSSE(w, flusher, map[string]interface{}{"type": "error", "error": "failed to list stores: " + err.Error()})
		return
	}

	totalUploaded, totalSkipped, totalUpdated := 0, 0, 0

	for i, store := range stores {
		storeIDStr := store.ID.Hex()
		localDir := "./attachments/" + storeIDStr
		sendSSE(w, flusher, map[string]interface{}{
			"type": "store_start", "store_id": storeIDStr, "store_name": store.Name,
			"index": i + 1, "total_stores": len(stores),
		})

		// Count files for this store
		fileCount := 0
		filepath.WalkDir(localDir, func(_ string, d fs.DirEntry, e error) error { //nolint:errcheck
			if e == nil && !d.IsDir() {
				fileCount++
			}
			return nil
		})

		urlMap := map[string]string{}
		uploaded, skipped := 0, 0
		filepath.WalkDir(localDir, func(path string, d fs.DirEntry, we error) error { //nolint:errcheck
			if we != nil || d.IsDir() {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				skipped++
				return nil
			}
			relKey := strings.TrimPrefix(filepath.ToSlash(path), "./")
			s3URL, uploadErr := uploadToS3(s, relKey, data, mimeFromFilename(d.Name()))
			if uploadErr != nil {
				log.Printf("s3 migrate-all: failed to upload %s: %v", path, uploadErr)
				skipped++
			} else {
				urlMap["/"+relKey] = s3URL
				uploaded++
			}
			return nil
		})

		updated := 0
		if len(urlMap) > 0 {
			updated = updateAttachmentURLsInMongo(store.ID, urlMap)
		}

		totalUploaded += uploaded
		totalSkipped += skipped
		totalUpdated += updated

		sendSSE(w, flusher, map[string]interface{}{
			"type": "store_done", "store_id": storeIDStr, "store_name": store.Name,
			"uploaded": uploaded, "skipped": skipped, "updated_messages": updated,
		})
	}

	sendSSE(w, flusher, map[string]interface{}{
		"type": "done", "stores": len(stores),
		"uploaded": totalUploaded, "skipped": totalSkipped, "updated_messages": totalUpdated,
	})
}

// MigrateRFQAttachmentsToS3Handler migrates base64 attachment_data_uris stored inline in
// rfq_received documents to S3/disk and replaces them with /cdn/ URLs.
// POST /v1/migrate-rfq-attachments-to-s3
func MigrateRFQAttachmentsToS3Handler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled — save your settings first"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	col := db.Client("").Database(db.GetPosDB()).Collection("rfq_received")

	// Count documents that still have old base64 fields
	filter := bson.M{"$or": bson.A{
		bson.M{"attachment_data_uris": bson.M{"$exists": true, "$ne": nil, "$not": bson.M{"$size": 0}}},
		bson.M{"additional_attachment_data_uris": bson.M{"$exists": true, "$ne": nil, "$not": bson.M{"$size": 0}}},
	}}
	total64, _ := col.CountDocuments(ctx, filter)
	total := int(total64)
	sendSSE(w, flusher, map[string]interface{}{"type": "start", "total": total})

	if total == 0 {
		sendSSE(w, flusher, map[string]interface{}{"type": "done", "uploaded": 0, "skipped": 0, "updated_rfqs": 0})
		return
	}

	proj := options.Find().SetProjection(bson.M{
		"_id": 1, "store_id": 1,
		"attachment_data_uris":            1,
		"additional_attachment_data_uris": 1,
		"additional_attachment_filenames": 1,
	})
	cursor, err := col.Find(ctx, filter, proj)
	if err != nil {
		sendSSE(w, flusher, map[string]interface{}{"type": "error", "error": "cursor failed: " + err.Error()})
		return
	}
	defer cursor.Close(ctx)

	uploaded, skipped, processed, updatedRFQs := 0, 0, 0, 0
	for cursor.Next(ctx) {
		var doc struct {
			ID                            primitive.ObjectID `bson:"_id"`
			StoreID                       primitive.ObjectID `bson:"store_id"`
			AttachmentDataURIs            []string           `bson:"attachment_data_uris"`
			AdditionalAttachmentDataURIs  []string           `bson:"additional_attachment_data_uris"`
			AdditionalAttachmentFilenames []string           `bson:"additional_attachment_filenames"`
		}
		if err := cursor.Decode(&doc); err != nil {
			skipped++
			continue
		}
		processed++
		pct := 0
		if total > 0 {
			pct = processed * 100 / total
		}

		storeIDStr := doc.StoreID.Hex()
		rfqIDStr := doc.ID.Hex()
		changed := false
		update := bson.M{}
		unset := bson.M{}

		if len(doc.AttachmentDataURIs) > 0 {
			urls := migrateDataURIsToStorage(s, storeIDStr, rfqIDStr, "att", doc.AttachmentDataURIs, nil)
			if len(urls) > 0 {
				update["attachment_urls"] = urls
				unset["attachment_data_uris"] = ""
				uploaded += len(urls)
				changed = true
			} else {
				skipped++
			}
		}
		if len(doc.AdditionalAttachmentDataURIs) > 0 {
			urls := migrateDataURIsToStorage(s, storeIDStr, rfqIDStr, "add", doc.AdditionalAttachmentDataURIs, doc.AdditionalAttachmentFilenames)
			if len(urls) > 0 {
				update["additional_attachment_urls"] = urls
				unset["additional_attachment_data_uris"] = ""
				uploaded += len(urls)
				changed = true
			} else {
				skipped++
			}
		}

		if changed {
			upd := bson.M{"$set": update}
			if len(unset) > 0 {
				upd["$unset"] = unset
			}
			if _, err := col.UpdateOne(ctx, bson.M{"_id": doc.ID}, upd); err == nil {
				updatedRFQs++
			}
		}

		sendSSE(w, flusher, map[string]interface{}{
			"type": "progress", "processed": processed, "total": total, "percent": pct,
			"current_file": rfqIDStr[:8] + "…", "uploaded": uploaded, "skipped": skipped,
		})
	}

	sendSSE(w, flusher, map[string]interface{}{
		"type": "done", "uploaded": uploaded, "skipped": skipped, "updated_rfqs": updatedRFQs,
	})
}

// migrateDataURIsToStorage decodes base64 data URIs and uploads each to S3/disk.
func migrateDataURIsToStorage(s models.AdminSettings, storeID, rfqID, prefix string, dataURIs, filenames []string) []string {
	urls := make([]string, 0, len(dataURIs))
	for i, uri := range dataURIs {
		if strings.HasPrefix(uri, "/cdn/") {
			urls = append(urls, uri) // already migrated
			continue
		}
		// Parse data URI
		if !strings.HasPrefix(uri, "data:") {
			continue
		}
		semi := strings.Index(uri, ";")
		comma := strings.Index(uri, ",")
		if semi < 0 || comma < 0 {
			continue
		}
		mime := uri[5:semi]
		raw, err := base64.StdEncoding.DecodeString(uri[comma+1:])
		if err != nil {
			log.Printf("migrateDataURIsToStorage: decode error: %v", err)
			continue
		}
		filename := fmt.Sprintf("%s-%d%s", prefix, i+1, extForMIME(mime))
		if i < len(filenames) && filenames[i] != "" {
			filename = filepath.Base(filenames[i])
		}
		relKey := "attachments/" + storeID + "/rfq/" + rfqID + "/" + filename
		url := saveAttachment(s, relKey, raw, mime)
		if url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

// updateAttachmentURLsInMongo iterates ProcurementMessage records for the store
// and replaces local attachment URLs with S3 URLs from urlMap.
func updateAttachmentURLsInMongo(storeID primitive.ObjectID, urlMap map[string]string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	col := db.Client("").Database(db.GetPosDB()).Collection("procurement_messages")
	cursor, err := col.Find(ctx, bson.M{"store_id": storeID})
	if err != nil {
		return 0
	}
	defer cursor.Close(ctx)

	updated := 0
	for cursor.Next(ctx) {
		var msg models.ProcurementMessage
		if err := cursor.Decode(&msg); err != nil {
			continue
		}
		changed := false
		for i, att := range msg.Attachments {
			if s3URL, ok := urlMap[att.URL]; ok {
				msg.Attachments[i].URL = s3URL
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := models.UpdateProcurementMessageAttachments(msg.ID, msg.Attachments); err == nil {
			updated++
		}
	}
	return updated
}

// ─── Migrate entity images (product/customer/vendor/expense/capital/etc.) ────

// MigrateEntityImagesToS3Handler migrates locally-stored image files for
// product, customer, vendor, expense, capital, capital_withdrawal,
// customer_deposit, customer_withdrawal, and store logo/background,
// as well as ZATCA XML files for sales, sales_return, receivable, payable.
// All stores are processed in one pass.
// POST /v1/migrate-entity-images-to-s3
func MigrateEntityImagesToS3Handler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	// Collect all local image files under ./images/ and ./zatca/
	type fileEntry struct{ relKey, root string }
	var files []fileEntry
	for _, root := range []string{"images", "zatca"} {
		_ = filepath.WalkDir("./"+root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(".", filepath.ToSlash(p))
			files = append(files, fileEntry{relKey: rel, root: root})
			return nil
		})
	}

	total := len(files)
	sendSSE(w, flusher, map[string]interface{}{"type": "start", "total": total})

	uploaded, skipped := 0, 0
	for i, f := range files {
		data, readErr := os.ReadFile("./" + f.relKey)
		if readErr != nil {
			skipped++
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.relKey))
		contentType := mimeForExt(ext)
		_, uploadErr := uploadToS3(s, f.relKey, data, contentType)
		if uploadErr != nil {
			log.Printf("migrate-entity-images: upload failed %s: %v", f.relKey, uploadErr)
			skipped++
		} else {
			uploaded++
		}
		pct := (i + 1) * 100 / total
		sendSSE(w, flusher, map[string]interface{}{
			"type": "progress", "processed": i + 1, "total": total, "percent": pct,
			"current_file": filepath.Base(f.relKey), "uploaded": uploaded, "skipped": skipped,
		})
	}
	sendSSE(w, flusher, map[string]interface{}{
		"type": "done", "uploaded": uploaded, "skipped": skipped,
	})

	// After uploading, update MongoDB so stored basenames become /cdn/ URLs.
	// Run in the background — the SSE stream already reported done.
	go updateEntityImageURLsInMongo(ctx, s)
}

// mimeForExt returns a MIME type for a file extension.
func mimeForExt(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".xml":
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

// updateEntityImageURLsInMongo rewrites bare image filenames in MongoDB to /cdn/ URLs.
// It processes: product, customer, vendor (images + logo), store (logo + invoice_background),
// expense, capital, capital_withdrawal, customer_deposit, customer_withdrawal (images),
// and ZATCA cleared_xml_url for order, sales_return, customer_deposit, customer_withdrawal.
func updateEntityImageURLsInMongo(ctx context.Context, s models.AdminSettings) {
	posDB := db.Client("").Database(db.GetPosDB())

	// Helper: for a given per-store collection name, field holding an array of filenames,
	// and a disk path builder func, rewrite each bare filename to a /cdn/ URL.
	updateImagesArray := func(dbName, colName, field string, pathBuilder func(storeID, entityID, filename string) string) {
		storeCol := db.Client("").Database(dbName).Collection(colName)
		cursor, err := storeCol.Find(ctx, bson.M{field: bson.M{"$exists": true, "$ne": nil, "$not": bson.M{"$size": 0}}})
		if err != nil {
			return
		}
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var doc struct {
				ID      primitive.ObjectID `bson:"_id"`
				StoreID primitive.ObjectID `bson:"store_id"`
				Images  []string           `bson:"images"`
			}
			if colName == "vendor" {
				// vendor uses same struct
			}
			if err := cursor.Decode(&doc); err != nil {
				continue
			}
			newImages := make([]string, 0, len(doc.Images))
			changed := false
			for _, img := range doc.Images {
				if strings.HasPrefix(img, "/cdn/") || strings.HasPrefix(img, "/images/") {
					newImages = append(newImages, img)
					continue
				}
				// bare filename — build relKey and rewrite
				relKey := pathBuilder(doc.StoreID.Hex(), doc.ID.Hex(), img)
				newImages = append(newImages, "/cdn/"+relKey)
				changed = true
			}
			if changed {
				storeCol.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{field: newImages}})
			}
		}
	}

	// Process all stores
	storeCursor, err := posDB.Collection("store").Find(ctx, bson.M{})
	if err != nil {
		return
	}
	defer storeCursor.Close(ctx)

	for storeCursor.Next(ctx) {
		var store struct {
			ID                primitive.ObjectID `bson:"_id"`
			Logo              string             `bson:"logo"`
			InvoiceBackground string             `bson:"invoice_background"`
		}
		if err := storeCursor.Decode(&store); err != nil {
			continue
		}
		sid := store.ID.Hex()
		storeDBName := "store_" + sid

		// Store logo
		if store.Logo != "" && !strings.HasPrefix(store.Logo, "/cdn/") && !strings.HasPrefix(store.Logo, "/") {
			relKey := "images/" + sid + "/store/" + store.Logo
			posDB.Collection("store").UpdateOne(ctx, bson.M{"_id": store.ID},
				bson.M{"$set": bson.M{"logo": "/cdn/" + relKey}})
		}
		// Store invoice background
		if store.InvoiceBackground != "" && !strings.HasPrefix(store.InvoiceBackground, "/cdn/") && !strings.HasPrefix(store.InvoiceBackground, "/") {
			relKey := "images/" + sid + "/store/" + store.InvoiceBackground
			posDB.Collection("store").UpdateOne(ctx, bson.M{"_id": store.ID},
				bson.M{"$set": bson.M{"invoice_background": "/cdn/" + relKey}})
		}

		// Product images
		updateImagesArray(storeDBName, "product", "images", func(_, entityID, filename string) string {
			return "images/" + sid + "/products/" + entityID + "/" + filename
		})
		// Customer images
		updateImagesArray(storeDBName, "customer", "images", func(_, entityID, filename string) string {
			return "images/" + sid + "/customers/" + entityID + "/" + filename
		})
		// Vendor images + logo
		updateImagesArray(storeDBName, "vendor", "images", func(_, entityID, filename string) string {
			return "images/" + sid + "/vendors/" + entityID + "/" + filename
		})
		vendorCursor, _ := db.Client("").Database(storeDBName).Collection("vendor").Find(ctx, bson.M{"logo": bson.M{"$exists": true, "$ne": ""}})
		if vendorCursor != nil {
			for vendorCursor.Next(ctx) {
				var v struct {
					ID   primitive.ObjectID `bson:"_id"`
					Logo string             `bson:"logo"`
				}
				if vendorCursor.Decode(&v) != nil || strings.HasPrefix(v.Logo, "/cdn/") || strings.HasPrefix(v.Logo, "/") {
					continue
				}
				relKey := "images/" + sid + "/vendors/" + v.Logo
				db.Client("").Database(storeDBName).Collection("vendor").UpdateOne(ctx, bson.M{"_id": v.ID},
					bson.M{"$set": bson.M{"logo": "/cdn/" + relKey}})
			}
			vendorCursor.Close(ctx)
		}

		// Expense images (basenames stored flat in images/)
		updateImagesArray(storeDBName, "expense", "images", func(storeID, _, filename string) string {
			return "images/" + storeID + "/expenses/" + filename
		})
		// Capital images
		updateImagesArray(storeDBName, "capital", "images", func(storeID, _, filename string) string {
			return "images/" + storeID + "/capitals/" + filename
		})
		// Capital withdrawal images
		updateImagesArray(storeDBName, "capitalwithdrawal", "images", func(storeID, _, filename string) string {
			return "images/" + storeID + "/capital_withdrawals/" + filename
		})
		// Customer deposit images
		updateImagesArray(storeDBName, "customerdeposit", "images", func(storeID, _, filename string) string {
			return "images/" + storeID + "/customer_deposits/" + filename
		})
		// Customer withdrawal images + inline base64 → already handled by MigrateInlineBase64ToS3
		updateImagesArray(storeDBName, "customerwithdrawal", "images", func(storeID, _, filename string) string {
			return "images/" + storeID + "/customer_withdrawals/" + filename
		})
	}

	// ZATCA XML files — update cleared_xml_url in orders, sales_returns, etc.
	// These are per-store collections.
	storeCursor2, _ := posDB.Collection("store").Find(ctx, bson.M{})
	if storeCursor2 != nil {
		defer storeCursor2.Close(ctx)
		for storeCursor2.Next(ctx) {
			var store struct {
				ID primitive.ObjectID `bson:"_id"`
			}
			if storeCursor2.Decode(&store) != nil {
				continue
			}
			sid := store.ID.Hex()
			storeDB := db.Client("").Database("store_" + sid)
			for _, spec := range []struct {
				col    string
				subdir string
			}{
				{"order", "sales"},
				{"salesreturn", "sales-returns"},
				{"customerdeposit", "receivables"},
				{"customerwithdrawal", "payables"},
			} {
				zatcaCursor, _ := storeDB.Collection(spec.col).Find(ctx,
					bson.M{"code": bson.M{"$exists": true}, "zatca.cleared_xml_url": bson.M{"$exists": false}})
				if zatcaCursor == nil {
					continue
				}
				for zatcaCursor.Next(ctx) {
					var doc struct {
						ID   primitive.ObjectID `bson:"_id"`
						Code string             `bson:"code"`
					}
					if zatcaCursor.Decode(&doc) != nil || doc.Code == "" {
						continue
					}
					relKey := "zatca/" + sid + "/" + spec.subdir + "/xml/" + doc.Code + ".xml"
					if _, err := os.Stat("./" + relKey); err != nil {
						continue // file doesn't exist
					}
					storeDB.Collection(spec.col).UpdateOne(ctx, bson.M{"_id": doc.ID},
						bson.M{"$set": bson.M{"zatca.cleared_xml_url": "/cdn/" + relKey}})
				}
				zatcaCursor.Close(ctx)
			}
		}
	}
}

// MigrateInlineBase64ToS3Handler migrates inline base64 image data (imagescontent field)
// stored directly in MongoDB documents for expense, capital, capital_withdrawal,
// customer_deposit, and customer_withdrawal across all stores.
// POST /v1/migrate-inline-images-to-s3
func MigrateInlineBase64ToS3Handler(w http.ResponseWriter, r *http.Request) {
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	requestingUser, _ := models.FindUserByID(&userID, bson.M{})
	if requestingUser == nil || (!requestingUser.Admin && requestingUser.Role != "Admin") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "admin only"})
		return
	}

	s := loadAdminS3Settings()
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	type entitySpec struct {
		colName   string // mongo collection name
		dirName   string // images sub-directory
		prefix    string // filename prefix
		storeDBFn func(storeID string) string
	}
	specs := []entitySpec{
		{"expense", "expenses", "expense_", func(s string) string { return "store_" + s }},
		{"capital", "capitals", "capital_", func(s string) string { return "store_" + s }},
		{"capitalwithdrawal", "capital_withdrawals", "capitalwithdrawal_", func(s string) string { return "store_" + s }},
		{"customerdeposit", "customer_deposits", "customerdeposit_", func(s string) string { return "store_" + s }},
		{"customerwithdrawal", "customer_withdrawals", "customwithdrawal_", func(s string) string { return "store_" + s }},
	}

	// Count total docs across all stores and specs
	posDB := db.Client("").Database(db.GetPosDB())
	storeCursor, err := posDB.Collection("store").Find(ctx, bson.M{})
	if err != nil {
		sendSSE(w, flusher, map[string]interface{}{"type": "error", "error": "failed to list stores"})
		return
	}
	var storeIDs []string
	for storeCursor.Next(ctx) {
		var s2 struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if storeCursor.Decode(&s2) == nil {
			storeIDs = append(storeIDs, s2.ID.Hex())
		}
	}
	storeCursor.Close(ctx)

	total := 0
	for _, sid := range storeIDs {
		for _, spec := range specs {
			n, _ := db.Client("").Database(spec.storeDBFn(sid)).Collection(spec.colName).CountDocuments(ctx,
				bson.M{"imagescontent": bson.M{"$exists": true, "$ne": nil, "$not": bson.M{"$size": 0}}})
			total += int(n)
		}
	}

	sendSSE(w, flusher, map[string]interface{}{"type": "start", "total": total})

	uploaded, skipped, processed, updatedDocs := 0, 0, 0, 0
	for _, sid := range storeIDs {
		for _, spec := range specs {
			col := db.Client("").Database(spec.storeDBFn(sid)).Collection(spec.colName)
			filter := bson.M{"imagescontent": bson.M{"$exists": true, "$ne": nil, "$not": bson.M{"$size": 0}}}
			cursor, err := col.Find(ctx, filter)
			if err != nil {
				continue
			}
			for cursor.Next(ctx) {
				var doc struct {
					ID            primitive.ObjectID `bson:"_id"`
					ImagesContent []string           `bson:"imagescontent"`
					Images        []string           `bson:"images"`
				}
				if err := cursor.Decode(&doc); err != nil {
					skipped++
					continue
				}
				processed++
				pct := 0
				if total > 0 {
					pct = processed * 100 / total
				}

				urls := migrateInlineBase64Items(s, sid, doc.ID.Hex(), spec.dirName, spec.prefix, doc.ImagesContent)
				if len(urls) > 0 {
					uploaded += len(urls)
					col.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{
						"$push":  bson.M{"images": bson.M{"$each": urls}},
						"$unset": bson.M{"imagescontent": ""},
					})
					updatedDocs++
				} else {
					skipped++
				}

				sendSSE(w, flusher, map[string]interface{}{
					"type": "progress", "processed": processed, "total": total, "percent": pct,
					"current_file": doc.ID.Hex()[:8] + "…", "uploaded": uploaded, "skipped": skipped,
				})
			}
			cursor.Close(ctx)
		}
	}

	sendSSE(w, flusher, map[string]interface{}{
		"type": "done", "uploaded": uploaded, "skipped": skipped, "updated_docs": updatedDocs,
	})
}

// migrateInlineBase64Items decodes each item (data URI or raw base64) and uploads to storage.
// Returns the list of /cdn/ URLs for successfully uploaded items.
func migrateInlineBase64Items(s models.AdminSettings, storeID, entityID, dirName, prefix string, items []string) []string {
	urls := make([]string, 0, len(items))
	for i, item := range items {
		if strings.HasPrefix(item, "/cdn/") {
			urls = append(urls, item)
			continue
		}
		var mime, b64 string
		if strings.HasPrefix(item, "data:") {
			semi := strings.Index(item, ";")
			comma := strings.Index(item, ",")
			if semi < 0 || comma < 0 {
				continue
			}
			mime = item[5:semi]
			b64 = item[comma+1:]
		} else {
			b64 = item
			mime = "image/jpeg"
		}
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			log.Printf("migrateInlineBase64Items: base64 decode: %v", err)
			continue
		}
		ext := extForMIME(mime)
		if ext == "" {
			if eFromData, eErr := models.GetFileExtensionFromBase64(raw); eErr == nil {
				ext = eFromData
			} else {
				ext = ".jpg"
			}
		}
		filename := fmt.Sprintf("%s%d%s", prefix, i+1, ext)
		relKey := "images/" + storeID + "/" + dirName + "/" + filename
		url := saveAttachment(s, relKey, raw, mime)
		if url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}
