//go:build integration

package models

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// Uploads through SaveFileToStorage's signer against a real S3-compatible
// server (versitygw in CI; tests.yml creates the bucket). Skips when
// S3_TEST_ENDPOINT is unset.
func TestUploadToS3Models_RealServer(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT not set")
	}
	bucket := os.Getenv("S3_TEST_BUCKET")
	if bucket == "" {
		bucket = "pos-ci"
	}
	s := AdminSettings{S3Enabled: true, S3BucketName: bucket, S3Region: "us-east-1", S3Endpoint: endpoint,
		S3AccessKeyID: os.Getenv("S3_TEST_ACCESS_KEY"), S3SecretKey: os.Getenv("S3_TEST_SECRET_KEY")}
	key := fmt.Sprintf("images/ci/%d/expense bill (2).jpg", time.Now().UnixNano())
	if err := uploadToS3Models(s, key, []byte("jpeg bytes"), "image/jpeg"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	s.S3SecretKey = "not-the-secret"
	if err := uploadToS3Models(s, key+".2", []byte("x"), ""); err == nil {
		t.Fatalf("upload with a wrong secret succeeded")
	}
}
