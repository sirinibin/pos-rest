//go:build integration

package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// These tests talk to a real S3-compatible server (SeaweedFS in CI, see
// tests.yml), so they cover the SigV4 signing that the unit tests' fake
// servers accept blindly. They skip when S3_TEST_ENDPOINT is unset.
func s3TestSettings(t *testing.T) models.AdminSettings {
	t.Helper()
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT not set")
	}
	bucket := os.Getenv("S3_TEST_BUCKET")
	if bucket == "" {
		bucket = "pos-ci"
	}
	s := models.AdminSettings{
		S3Enabled:     true,
		S3BucketName:  bucket,
		S3Region:      "us-east-1",
		S3AccessKeyID: os.Getenv("S3_TEST_ACCESS_KEY"),
		S3SecretKey:   os.Getenv("S3_TEST_SECRET_KEY"),
		S3Endpoint:    endpoint,
	}
	if err := s3TestCreateBucket(s); err != nil {
		t.Fatalf("create bucket %s: %v", bucket, err)
	}
	return s
}

// s3TestCreateBucket sends a signed CreateBucket; an existing bucket is fine.
func s3TestCreateBucket(s models.AdminSettings) error {
	now := time.Now().UTC()
	dateStr, timeStr := now.Format("20060102"), now.Format("20060102T150405Z")
	emptyHash := fmt.Sprintf("%x", sha256sum(nil))
	host := s3Host(s)
	req, err := http.NewRequest("PUT", strings.TrimRight(s.S3Endpoint, "/")+"/"+s.S3BucketName, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("X-Amz-Content-Sha256", emptyHash)
	canonical := strings.Join([]string{"PUT", "/" + s.S3BucketName, "",
		fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyHash, timeStr),
		"host;x-amz-content-sha256;x-amz-date", emptyHash}, "\n")
	scope := dateStr + "/" + s.S3Region + "/s3/aws4_request"
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", timeStr, scope, fmt.Sprintf("%x", sha256sum([]byte(canonical)))}, "\n")
	key := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+s.S3SecretKey), []byte(dateStr)), []byte(s.S3Region)), []byte("s3")), []byte("aws4_request"))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=%x",
		s.S3AccessKeyID, scope, hmacSHA256(key, []byte(toSign))))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusConflict {
		var b bytes.Buffer
		b.ReadFrom(resp.Body) //nolint:errcheck
		return fmt.Errorf("status %d: %s", resp.StatusCode, b.String())
	}
	return nil
}

func TestS3Storage_UploadReadServeDelete(t *testing.T) {
	s := s3TestSettings(t)
	key := fmt.Sprintf("ci/%d/bill image (1).txt", time.Now().UnixNano())
	data := []byte("hello from the pos-rest CI")

	url, err := uploadToS3(s, key, data, "text/plain")
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if want := strings.TrimRight(s.S3Endpoint, "/") + "/" + s.S3BucketName + "/" + key; url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}
	if !headS3Object(s, key) {
		_, derr := downloadFromS3(s, key)
		t.Fatalf("HEAD after upload: object missing (GET: %v)", derr)
	}

	got, err := downloadFromS3(s, key)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("download = %q, %v; want %q", got, err, data)
	}

	rec := httptest.NewRecorder()
	if !proxyS3Object(s, key, rec) {
		t.Fatalf("proxy: not served")
	}
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("proxy: HTTP %d %q", rec.Code, rec.Body.String())
	}

	if err := deleteFromS3(s, key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if headS3Object(s, key) {
		t.Fatalf("HEAD after delete: object still there")
	}
	if _, err := downloadFromS3(s, key); err == nil {
		t.Fatalf("download after delete succeeded")
	}
}

func TestS3Storage_AttachmentRoundTrip(t *testing.T) {
	s := s3TestSettings(t)
	relKey := fmt.Sprintf("attachments/ci/%d/quote.pdf", time.Now().UnixNano())
	data := []byte("%PDF-1.4 ci")
	url := saveAttachment(s, relKey, data, "application/pdf")
	if url != "/cdn/"+relKey {
		t.Fatalf("saveAttachment = %q, want /cdn/%s", url, relKey)
	}
	if _, err := os.Stat("./" + relKey); err == nil {
		t.Fatalf("an S3 attachment was also written to local disk")
	}
	got, err := downloadAttachment(s, url)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("downloadAttachment = %q, %v", got, err)
	}
	if err := deleteFromS3(s, relKey); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestS3Storage_WrongSecretIsRefused(t *testing.T) {
	s := s3TestSettings(t)
	s.S3SecretKey = "not-the-secret"
	key := fmt.Sprintf("ci/%d/refused.txt", time.Now().UnixNano())
	if _, err := uploadToS3(s, key, []byte("x"), "text/plain"); err == nil {
		t.Fatalf("upload with a wrong secret succeeded")
	}
	if url := saveAttachment(s, key, []byte("x"), "text/plain"); url != "" {
		t.Fatalf("saveAttachment with a wrong secret = %q, want \"\"", url)
	}
	// A refused delete is an error, not a silent success.
	if err := deleteFromS3(s, key); err == nil {
		t.Fatalf("delete with a wrong secret reported success")
	}
}
