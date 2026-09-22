package controller

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

// ─── s3BaseURL tests ──────────────────────────────────────────────────────────

func TestS3BaseURL_StandardAWS(t *testing.T) {
	s := models.AdminSettings{S3BucketName: "my-bucket", S3Region: "us-east-1"}
	got := s3BaseURL(s)
	want := "https://my-bucket.s3.us-east-1.amazonaws.com"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestS3BaseURL_CustomEndpoint(t *testing.T) {
	s := models.AdminSettings{S3BucketName: "my-bucket", S3Endpoint: "https://sgp1.digitaloceanspaces.com"}
	got := s3BaseURL(s)
	if !strings.Contains(got, "my-bucket") {
		t.Errorf("expected bucket name in URL, got %q", got)
	}
}

func TestS3BaseURL_PublicBaseURL_Overrides(t *testing.T) {
	s := models.AdminSettings{
		S3BucketName:    "my-bucket",
		S3Region:        "us-east-1",
		S3PublicBaseURL: "https://cdn.example.com",
	}
	if got := s3BaseURL(s); got != "https://cdn.example.com" {
		t.Errorf("public base URL should override, got %q", got)
	}
}

func TestS3BaseURL_TrailingSlashStripped(t *testing.T) {
	s := models.AdminSettings{S3PublicBaseURL: "https://cdn.example.com/"}
	if got := s3BaseURL(s); strings.HasSuffix(got, "/") {
		t.Errorf("trailing slash should be stripped, got %q", got)
	}
}

// ─── s3PutURL tests ───────────────────────────────────────────────────────────

func TestS3PutURL_StandardAWS(t *testing.T) {
	s := models.AdminSettings{S3BucketName: "my-bucket", S3Region: "ap-southeast-1"}
	got := s3PutURL(s, "attachments/store1/msg1/file.pdf")
	want := "https://my-bucket.s3.ap-southeast-1.amazonaws.com/attachments/store1/msg1/file.pdf"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestS3PutURL_CustomEndpointPathStyle(t *testing.T) {
	s := models.AdminSettings{S3BucketName: "my-bucket", S3Endpoint: "https://minio.example.com"}
	got := s3PutURL(s, "attachments/x/y/file.pdf")
	if !strings.Contains(got, "my-bucket") || !strings.Contains(got, "file.pdf") {
		t.Errorf("expected bucket and key in URL, got %q", got)
	}
}

// ─── saveAttachment local fallback tests ──────────────────────────────────────

func TestSaveAttachment_LocalFallback_WritesFile(t *testing.T) {
	dir := t.TempDir()
	// Build a relative key inside the temp dir (strip leading /)
	relKey := filepath.ToSlash(strings.TrimPrefix(dir, "/")) + "/att/test.txt"

	settings := models.AdminSettings{S3Enabled: false}
	url := saveAttachment(settings, relKey, []byte("hello"), "text/plain")
	if url == "" {
		t.Fatal("expected a URL, got empty string")
	}
	if !strings.HasPrefix(url, "/") {
		t.Errorf("local URL should start with /, got %q", url)
	}
	data, err := os.ReadFile("./" + relKey)
	if err != nil {
		t.Fatalf("file not written to disk: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content wrong: %q", string(data))
	}
}

func TestSaveAttachment_S3DisabledFallsToLocal(t *testing.T) {
	dir := t.TempDir()
	relKey := filepath.ToSlash(strings.TrimPrefix(dir, "/")) + "/noS3/f.txt"
	settings := models.AdminSettings{S3Enabled: false, S3BucketName: "b", S3AccessKeyID: "k"}
	url := saveAttachment(settings, relKey, []byte("d"), "text/plain")
	if !strings.HasPrefix(url, "/") {
		t.Errorf("S3 disabled — should use local URL, got %q", url)
	}
}

func TestSaveAttachment_S3EnabledButNoBucket_FallsToLocal(t *testing.T) {
	dir := t.TempDir()
	relKey := filepath.ToSlash(strings.TrimPrefix(dir, "/")) + "/nobucket/f.txt"
	settings := models.AdminSettings{S3Enabled: true, S3BucketName: "", S3AccessKeyID: "k"}
	url := saveAttachment(settings, relKey, []byte("d"), "text/plain")
	if !strings.HasPrefix(url, "/") {
		t.Errorf("no bucket — should use local URL, got %q", url)
	}
}

// ─── AdminSettings S3 fields JSON ────────────────────────────────────────────

func TestAdminSettings_S3Fields_JSONKeys(t *testing.T) {
	s := models.AdminSettings{
		S3Enabled:       true,
		S3BucketName:    "my-bucket",
		S3Region:        "us-east-1",
		S3AccessKeyID:   "AKIATEST",
		S3SecretKey:     "supersecret",
		S3Endpoint:      "https://minio.example.com",
		S3PublicBaseURL: "https://cdn.example.com",
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, key := range []string{
		"s3_enabled", "s3_bucket_name", "s3_region",
		"s3_access_key_id", "s3_secret_key",
		"s3_endpoint", "s3_public_base_url",
	} {
		if !strings.Contains(js, `"`+key+`"`) {
			t.Errorf("JSON missing field %q", key)
		}
	}
}

func TestAdminSettings_S3Enabled_DefaultFalse(t *testing.T) {
	var s models.AdminSettings
	if s.S3Enabled {
		t.Errorf("S3Enabled should default to false")
	}
}
