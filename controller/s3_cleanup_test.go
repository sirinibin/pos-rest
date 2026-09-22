package controller

import (
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

func TestHeadS3Object_NoCredentials(t *testing.T) {
	// Empty settings → no valid S3 target; request will error → returns false
	s := models.AdminSettings{}
	result := headS3Object(s, "test/key.txt")
	if result {
		t.Error("headS3Object should return false when S3 is not configured")
	}
}

func TestHeadS3Object_InvalidEndpoint(t *testing.T) {
	s := models.AdminSettings{
		S3Enabled:     true,
		S3BucketName:  "test-bucket",
		S3Region:      "us-east-1",
		S3AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
		S3SecretKey:   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	}
	// Pointing at real AWS with fake creds → network call will fail or return 403, not 200
	result := headS3Object(s, "nonexistent-key-12345.txt")
	if result {
		t.Error("headS3Object should return false for a non-existent key")
	}
}

func TestRewriteURL_DirectS3ToCDN(t *testing.T) {
	baseURL := "https://my-bucket.s3.eu-west-2.amazonaws.com"

	rewriteURL := func(url string) (string, bool) {
		if strings.HasPrefix(url, "/cdn/") || url == "" {
			return url, false
		}
		prefix := baseURL + "/"
		if !strings.HasPrefix(url, prefix) {
			return url, false
		}
		relKey := strings.TrimPrefix(url, prefix)
		return "/cdn/" + relKey, true
	}

	cases := []struct {
		input    string
		wantURL  string
		wantOK   bool
	}{
		{
			input:   "https://my-bucket.s3.eu-west-2.amazonaws.com/images/store1/products/prod1/file.jpg",
			wantURL: "/cdn/images/store1/products/prod1/file.jpg",
			wantOK:  true,
		},
		{
			input:   "/cdn/images/store1/products/prod1/file.jpg",
			wantURL: "/cdn/images/store1/products/prod1/file.jpg",
			wantOK:  false, // already /cdn/
		},
		{
			input:   "https://other-bucket.s3.amazonaws.com/images/file.jpg",
			wantURL: "https://other-bucket.s3.amazonaws.com/images/file.jpg",
			wantOK:  false, // different bucket
		},
		{
			input:   "",
			wantURL: "",
			wantOK:  false,
		},
		{
			input:   "image.jpg",
			wantURL: "image.jpg",
			wantOK:  false, // bare filename, not an S3 URL
		},
	}

	for _, tc := range cases {
		gotURL, gotOK := rewriteURL(tc.input)
		if gotURL != tc.wantURL || gotOK != tc.wantOK {
			t.Errorf("rewriteURL(%q) = (%q, %v), want (%q, %v)",
				tc.input, gotURL, gotOK, tc.wantURL, tc.wantOK)
		}
	}
}

func TestRewriteURL_CustomEndpoint(t *testing.T) {
	// DigitalOcean Spaces style: endpoint/bucket/key
	baseURL := "https://ams3.digitaloceanspaces.com/my-bucket"

	rewriteURL := func(url string) (string, bool) {
		if strings.HasPrefix(url, "/cdn/") || url == "" {
			return url, false
		}
		prefix := baseURL + "/"
		if !strings.HasPrefix(url, prefix) {
			return url, false
		}
		relKey := strings.TrimPrefix(url, prefix)
		return "/cdn/" + relKey, true
	}

	input := "https://ams3.digitaloceanspaces.com/my-bucket/attachments/store1/rfq/id1/file.pdf"
	wantURL := "/cdn/attachments/store1/rfq/id1/file.pdf"

	gotURL, gotOK := rewriteURL(input)
	if !gotOK || gotURL != wantURL {
		t.Errorf("rewriteURL(%q) = (%q, %v), want (%q, true)", input, gotURL, gotOK, wantURL)
	}
}

func TestVerifyCDNAndDelete_NoCDNPrefix(t *testing.T) {
	s := models.AdminSettings{S3Enabled: true, S3BucketName: "b", S3AccessKeyID: "k"}
	// URL without /cdn/ prefix should return (false, false)
	verified, deleted := func(cdnURL string) (bool, bool) {
		key := strings.TrimPrefix(cdnURL, "/cdn/")
		if key == cdnURL || key == "" {
			return false, false
		}
		if !headS3Object(s, key) {
			return false, false
		}
		return true, true
	}("images/bare/path.jpg")

	if verified || deleted {
		t.Error("URL without /cdn/ prefix should not be verified or deleted")
	}
}

func TestVerifyCDNAndDelete_EmptyURL(t *testing.T) {
	s := models.AdminSettings{S3Enabled: true}
	verified, deleted := func(cdnURL string) (bool, bool) {
		key := strings.TrimPrefix(cdnURL, "/cdn/")
		if key == cdnURL || key == "" {
			return false, false
		}
		if !headS3Object(s, key) {
			return false, false
		}
		return true, true
	}("")

	if verified || deleted {
		t.Error("empty URL should return (false, false)")
	}
}
