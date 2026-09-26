package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// ─── s3URIEncodeKey tests ─────────────────────────────────────────────────────

func TestS3URIEncodeKey_SpacesEncoded(t *testing.T) {
	key := "attachments/store1/wa_msg1/Cable Comnnection..pdf"
	got := s3URIEncodeKey(key)
	if strings.Contains(got, " ") {
		t.Errorf("spaces should be encoded, got %q", got)
	}
	if !strings.Contains(got, "Cable%20Comnnection..pdf") {
		t.Errorf("expected %%20 for space, got %q", got)
	}
	// Slashes between segments must be preserved
	parts := strings.Split(got, "/")
	if len(parts) != 4 {
		t.Errorf("expected 4 segments, got %d in %q", len(parts), got)
	}
}

func TestS3URIEncodeKey_NoSpecialChars_Unchanged(t *testing.T) {
	key := "attachments/store1/msg1/UMLJ-Quotation.pdf"
	got := s3URIEncodeKey(key)
	if got != key {
		t.Errorf("no special chars — key should be unchanged, got %q", got)
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
	if !strings.HasPrefix(url, "/cdn/") {
		t.Errorf("local URL should start with /cdn/, got %q", url)
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
	if !strings.HasPrefix(url, "/cdn/") {
		t.Errorf("S3 disabled — should use /cdn/ URL, got %q", url)
	}
}

func TestSaveAttachment_S3EnabledButNoBucket_FallsToLocal(t *testing.T) {
	dir := t.TempDir()
	relKey := filepath.ToSlash(strings.TrimPrefix(dir, "/")) + "/nobucket/f.txt"
	settings := models.AdminSettings{S3Enabled: true, S3BucketName: "", S3AccessKeyID: "k"}
	url := saveAttachment(settings, relKey, []byte("d"), "text/plain")
	if !strings.HasPrefix(url, "/cdn/") {
		t.Errorf("no bucket — should use /cdn/ URL, got %q", url)
	}
}

// When S3 is configured but the upload fails, saveAttachment must return "" (no disk fallback).
// This allows callers to mark the attachment as missing so the user can retry later.
func TestSaveAttachment_S3FailureReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<Error>AccessDenied</Error>")) //nolint:errcheck
	}))
	defer srv.Close()

	dir := t.TempDir()
	relKey := filepath.ToSlash(strings.TrimPrefix(dir, "/")) + "/s3fail/f.txt"
	settings := models.AdminSettings{
		S3Enabled:     true,
		S3BucketName:  "bucket",
		S3Region:      "us-east-1",
		S3AccessKeyID: "KEY",
		S3SecretKey:   "SECRET",
		S3Endpoint:    srv.URL,
	}
	got := saveAttachment(settings, relKey, []byte("data"), "text/plain")
	if got != "" {
		t.Errorf("S3 upload failed — saveAttachment should return empty URL, got %q", got)
	}
	// File must NOT have been written to disk (S3-only mode).
	if _, err := os.Stat("./" + relKey); err == nil {
		t.Error("file should NOT be written to disk when S3 is configured and upload fails")
		os.Remove("./" + relKey)
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

// ─── proxyS3Object returns false on non-200 ───────────────────────────────────

func TestProxyS3Object_ReturnsFalseOnNon200(t *testing.T) {
	// Spin up a fake S3 that returns 404
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := models.AdminSettings{
		S3Enabled:     true,
		S3BucketName:  "bucket",
		S3Region:      "us-east-1",
		S3AccessKeyID: "KEY",
		S3SecretKey:   "SECRET",
		S3Endpoint:    srv.URL,
	}
	w := httptest.NewRecorder()
	ok := proxyS3Object(s, "test/key.txt", w)
	if ok {
		t.Error("proxyS3Object should return false for 404 response")
	}
	if w.Body.Len() != 0 {
		t.Errorf("proxyS3Object must not write to w when returning false, body=%q", w.Body.String())
	}
}

func TestProxyS3Object_ReturnsTrueOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PDF content")) //nolint:errcheck
	}))
	defer srv.Close()

	s := models.AdminSettings{
		S3Enabled:     true,
		S3BucketName:  "bucket",
		S3Region:      "us-east-1",
		S3AccessKeyID: "KEY",
		S3SecretKey:   "SECRET",
		S3Endpoint:    srv.URL,
	}
	w := httptest.NewRecorder()
	ok := proxyS3Object(s, "test/key.txt", w)
	if !ok {
		t.Error("proxyS3Object should return true for 200 response")
	}
	if w.Body.String() != "PDF content" {
		t.Errorf("expected body 'PDF content', got %q", w.Body.String())
	}
}

// ─── CdnFileHandler disk fallback ─────────────────────────────────────────────

// TestProxyS3Object_DoesNotWriteOnFailure guards the CdnFileHandler disk-fallback contract:
// when proxyS3Object returns false it must not have written anything to w, so the
// caller can still serve from disk.
func TestProxyS3Object_DoesNotWriteOnFailure(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		s := models.AdminSettings{
			S3Enabled:     true,
			S3BucketName:  "bucket",
			S3Region:      "us-east-1",
			S3AccessKeyID: "KEY",
			S3SecretKey:   "SECRET",
			S3Endpoint:    srv.URL,
		}
		w := httptest.NewRecorder()
		if ok := proxyS3Object(s, "test/key.txt", w); ok {
			t.Errorf("status %d: proxyS3Object should return false", code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("status %d: must not write body when returning false, got %q", code, w.Body.String())
		}
		srv.Close()
	}
}

// ─── deleteS3Attachments no-op when S3 disabled ───────────────────────────────

func TestDeleteS3Attachments_NoopWhenS3Disabled(t *testing.T) {
	// loadAdminS3Settings returns empty settings (S3Enabled=false) in test env.
	// deleteS3Attachments should return without attempting any S3 call.
	// We use a message with a /cdn/ URL to exercise the S3-disabled early-return path.
	msgs := []models.ProcurementMessage{
		{Attachments: []models.ProcurementAttachment{{URL: "/cdn/attachments/store1/msg1/file.pdf"}}},
	}
	// Should not panic or error — just a no-op
	deleteS3Attachments(msgs)
}
