package models

import (
	"encoding/base64"
	"net/http"
	"testing"
)

// TestParseAuthCodeFromRequest_BasicAuthScheme verifies that the standard
// HTTP Basic Auth format (Authorization: Basic base64(code + ":")) is decoded
// correctly, matching what the starterp-react frontend sends on POST /accesstoken.
func TestParseAuthCodeFromRequest_BasicAuthScheme(t *testing.T) {
	fakeCode := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.fake.sig"
	encoded := base64.StdEncoding.EncodeToString([]byte(fakeCode + ":"))

	req, _ := http.NewRequest("POST", "/v1/accesstoken", nil)
	req.Header.Set("Authorization", "Basic "+encoded)

	got, err := ParseAuthCodeFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeCode {
		t.Errorf("expected %q, got %q", fakeCode, got)
	}
}

// TestParseAuthCodeFromRequest_BearerScheme verifies that the existing
// Bearer scheme still works (backward compatibility).
func TestParseAuthCodeFromRequest_BearerScheme(t *testing.T) {
	fakeCode := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.fake.sig"

	req, _ := http.NewRequest("POST", "/v1/accesstoken", nil)
	req.Header.Set("Authorization", "Bearer "+fakeCode)

	got, err := ParseAuthCodeFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeCode {
		t.Errorf("expected %q, got %q", fakeCode, got)
	}
}

// TestParseAuthCodeFromRequest_QueryParam verifies that ?auth_code= still works.
func TestParseAuthCodeFromRequest_QueryParam(t *testing.T) {
	fakeCode := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.fake.sig"

	req, _ := http.NewRequest("POST", "/v1/accesstoken?auth_code="+fakeCode, nil)

	got, err := ParseAuthCodeFromRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeCode {
		t.Errorf("expected %q, got %q", fakeCode, got)
	}
}

// TestParseAuthCodeFromRequest_Missing verifies that missing credentials return an error.
func TestParseAuthCodeFromRequest_Missing(t *testing.T) {
	req, _ := http.NewRequest("POST", "/v1/accesstoken", nil)
	_, err := ParseAuthCodeFromRequest(req)
	if err == nil {
		t.Fatal("expected error for missing auth code, got nil")
	}
}
