package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
)

// loadStoreS3Settings fetches only the S3-related settings for a store by its hex ID.
// Returns an empty StoreSettings on any error (safe to use — S3 will be disabled).
func loadStoreS3Settings(storeIDStr string) models.StoreSettings {
	id, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		return models.StoreSettings{}
	}
	proj := map[string]interface{}{
		"settings.s3_enabled":        1,
		"settings.s3_bucket_name":    1,
		"settings.s3_region":         1,
		"settings.s3_access_key_id":  1,
		"settings.s3_secret_key":     1,
		"settings.s3_endpoint":       1,
		"settings.s3_public_base_url": 1,
	}
	store, err := models.FindStoreByID(&id, proj)
	if err != nil || store == nil {
		return models.StoreSettings{}
	}
	return store.Settings
}

// s3BaseURL returns the base URL for the bucket, honoring custom endpoint / public base URL.
func s3BaseURL(s models.StoreSettings) string {
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
func s3Host(s models.StoreSettings) string {
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		ep = strings.TrimPrefix(ep, "https://")
		ep = strings.TrimPrefix(ep, "http://")
		return ep + "/" + s.S3BucketName
	}
	return fmt.Sprintf("%s.s3.%s.amazonaws.com", s.S3BucketName, s.S3Region)
}

// s3PutURL returns the URL for a PutObject request.
func s3PutURL(s models.StoreSettings, key string) string {
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		return ep + "/" + s.S3BucketName + "/" + key
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.S3BucketName, s.S3Region, key)
}

// uploadToS3 uploads data to S3 using SigV4-signed PUT, returns the public file URL.
func uploadToS3(s models.StoreSettings, key string, data []byte, contentType string) (string, error) {
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
// Returns the URL to access the file (S3 URL or "/relKey").
func saveAttachment(settings models.StoreSettings, relKey string, data []byte, contentType string) string {
	if settings.S3Enabled && settings.S3BucketName != "" && settings.S3AccessKeyID != "" {
		url, err := uploadToS3(settings, relKey, data, contentType)
		if err != nil {
			log.Printf("s3: upload failed for %s, falling back to local disk: %v", relKey, err)
		} else {
			return url
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
	return "/" + relKey
}

// ─── Test S3 Connection ───────────────────────────────────────────────────────

// TestS3ConnectionHandler tests S3 credentials by uploading and deleting a tiny probe file.
// POST /v1/store/{id}/test-s3
func TestS3ConnectionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	storeID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store id"})
		return
	}
	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil || store == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}
	s := store.Settings
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured"})
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
func deleteFromS3(s models.StoreSettings, key string) error {
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

// ─── Migrate existing files to S3 ────────────────────────────────────────────

// MigrateAttachmentsToS3Handler uploads all local ./attachments/{storeID}/ files to S3
// and updates the MongoDB ProcurementMessage records with S3 URLs.
// POST /v1/store/{id}/migrate-to-s3
func MigrateAttachmentsToS3Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	storeID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store id"})
		return
	}
	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil || store == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}
	s := store.Settings
	if !s.S3Enabled || s.S3BucketName == "" || s.S3AccessKeyID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "S3 not configured or not enabled"})
		return
	}

	storeIDStr := storeID.Hex()
	localDir := "./attachments/" + storeIDStr

	// Build a map: localURL → s3URL for all files we successfully upload
	urlMap := map[string]string{}
	uploaded, skipped := 0, 0

	err = filepath.WalkDir(localDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			skipped++
			return nil
		}
		// relKey = "attachments/{storeID}/..." (no leading ./)
		relKey := strings.TrimPrefix(filepath.ToSlash(path), "./")
		contentType := mimeFromFilename(d.Name())
		s3URL, uploadErr := uploadToS3(s, relKey, data, contentType)
		if uploadErr != nil {
			log.Printf("s3 migrate: failed to upload %s: %v", path, uploadErr)
			skipped++
			return nil
		}
		localURL := "/" + relKey
		urlMap[localURL] = s3URL
		uploaded++
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "walk failed: " + err.Error()})
		return
	}

	if len(urlMap) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uploaded": 0, "skipped": skipped, "updated_messages": 0,
		})
		return
	}

	// Update MongoDB: load all messages for this store, rewrite attachment URLs.
	updatedMessages := updateAttachmentURLsInMongo(storeID, urlMap)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"uploaded":         uploaded,
		"skipped":          skipped,
		"updated_messages": updatedMessages,
	})
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
