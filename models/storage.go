package models

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// s3StorageBaseURL returns the public base URL for the S3 bucket.
func s3StorageBaseURL(s AdminSettings) string {
	if s.S3PublicBaseURL != "" {
		return strings.TrimRight(s.S3PublicBaseURL, "/")
	}
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		return ep + "/" + s.S3BucketName
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com", s.S3BucketName, s.S3Region)
}

// s3StorageHost returns the HTTPS host used for SigV4 signing.
func s3StorageHost(s AdminSettings) string {
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		ep = strings.TrimPrefix(ep, "https://")
		ep = strings.TrimPrefix(ep, "http://")
		// Path-style: the bucket is in the path, not the Host header Go
		// sends, so it must not be in the signed host either (S3-compatible
		// services refused every signed request).
		if i := strings.Index(ep, "/"); i >= 0 {
			ep = ep[:i]
		}
		return ep
	}
	return fmt.Sprintf("%s.s3.%s.amazonaws.com", s.S3BucketName, s.S3Region)
}

// s3StoragePutURL returns the full URL for a PutObject request.
func s3StoragePutURL(s AdminSettings, key string) string {
	key = s3EncodeKey(key)
	if s.S3Endpoint != "" {
		ep := strings.TrimRight(s.S3Endpoint, "/")
		return ep + "/" + s.S3BucketName + "/" + key
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.S3BucketName, s.S3Region, key)
}

// s3EncodeKey percent-encodes each segment of key, so the signed path is
// the one Go sends (file names with spaces failed the signature check).
func s3EncodeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// sha256Bytes returns the SHA-256 digest of b as a byte slice.
func sha256Bytes(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// hmac256 returns the HMAC-SHA256 of data using key.
func hmac256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// uploadToS3Models uploads data to S3 using a SigV4-signed PUT request.
func uploadToS3Models(s AdminSettings, key string, data []byte, contentType string) error {
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

	bodyHash := fmt.Sprintf("%x", sha256Bytes(data))
	host := s3StorageHost(s)
	putURL := s3StoragePutURL(s, key)

	req, err := http.NewRequest("PUT", putURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", bodyHash)

	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		contentType, host, bodyHash, timeStr)
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"

	encodedKey := s3EncodeKey(key)
	urlPath := "/" + encodedKey
	if s.S3Endpoint != "" {
		urlPath = "/" + s.S3BucketName + "/" + encodedKey
	}

	canonicalRequest := strings.Join([]string{"PUT", urlPath, "", canonicalHeaders, signedHeaders, bodyHash}, "\n")

	credentialScope := strings.Join([]string{dateStr, region, "s3", "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timeStr,
		credentialScope,
		fmt.Sprintf("%x", sha256Bytes([]byte(canonicalRequest))),
	}, "\n")

	signingKey := hmac256(hmac256(hmac256(hmac256(
		[]byte("AWS4"+s.S3SecretKey), []byte(dateStr)),
		[]byte(region)),
		[]byte("s3")),
		[]byte("aws4_request"))
	signature := fmt.Sprintf("%x", hmac256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.S3AccessKeyID, credentialScope, signedHeaders, signature))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body) //nolint:errcheck
		return fmt.Errorf("s3 put %s: status %d: %s", key, resp.StatusCode, buf.String())
	}
	return nil
}

// SaveFileToStorage uploads data to S3 if configured, otherwise writes to local disk.
// relKey is the path without a leading slash, e.g. "images/storeID/expenses/file.jpg".
// Returns "/cdn/relKey" on success, or "" on failure.
func SaveFileToStorage(relKey string, data []byte, contentType string) string {
	s, _ := GetAdminSettings()
	if s == nil {
		s = &AdminSettings{}
	}
	if s.S3Enabled && s.S3BucketName != "" && s.S3AccessKeyID != "" {
		if err := uploadToS3Models(*s, relKey, data, contentType); err == nil {
			return "/cdn/" + relKey
		} else {
			log.Printf("s3: upload failed for %s, falling back to local disk: %v", relKey, err)
		}
	}
	// Local disk fallback
	localPath := "./" + relKey
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		log.Printf("storage: mkdir failed for %s: %v", localPath, err)
		return ""
	}
	if err := os.WriteFile(localPath, data, 0644); err != nil {
		log.Printf("storage: write failed for %s: %v", localPath, err)
		return ""
	}
	return "/cdn/" + relKey
}
