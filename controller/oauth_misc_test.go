package controller

// oauth_misc_test.go — unit tests for OAuth2 / misc handlers:
//
//   oauth2.go   : Accesstoken, RefreshAccesstoken, APIInfo
//   openapi.go  : ServeOpenAPISpec
//   automobile_dashboard.go : GetAutoMobileDashboard
//
// Authorize is intentionally excluded from the unauthenticated test below
// because auth.Authenticate() requires a live MongoDB connection.  The other
// handlers fail early on missing/invalid tokens, before any DB call.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Accesstoken ───────────────────────────────────────────────────────────────

// TestAccesstoken_NoAuthCode verifies that POST /v1/accesstoken without a
// valid auth-code header returns HTTP 401 with status=false and
// errors["auth_code"] set.
func TestAccesstoken_NoAuthCode(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/accesstoken", strings.NewReader(""))
	w := httptest.NewRecorder()
	Accesstoken(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}

	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["auth_code"]; !ok {
		t.Errorf("expected errors[\"auth_code\"] key, got errors=%v", resp.Errors)
	}
}

// ── RefreshAccesstoken ────────────────────────────────────────────────────────

// TestRefreshAccesstoken_NoToken verifies that POST /v1/refresh without a
// refresh token returns HTTP 401 with status=false and
// errors["refresh_token"] set.
func TestRefreshAccesstoken_NoToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/refresh", strings.NewReader(""))
	w := httptest.NewRecorder()
	RefreshAccesstoken(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}

	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["refresh_token"]; !ok {
		t.Errorf("expected errors[\"refresh_token\"] key, got errors=%v", resp.Errors)
	}
}

// ── APIInfo ───────────────────────────────────────────────────────────────────

// TestAPIInfo_ReturnsOK verifies that GET / returns HTTP 200 with status=true
// and a non-empty result (the API info string).
func TestAPIInfo_ReturnsOK(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	APIInfo(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}

	var body struct {
		Status bool        `json:"status"`
		Result interface{} `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Status {
		t.Error("expected status=true")
	}
	if body.Result == nil {
		t.Error("expected non-nil result")
	}
}

// ── ServeOpenAPISpec ──────────────────────────────────────────────────────────

// TestServeOpenAPISpec_Public verifies that GET /v1/openapi.json (public, no
// auth required) returns HTTP 200 with Content-Type application/json and a
// valid JSON body containing an "openapi" key.
func TestServeOpenAPISpec_Public(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
	w := httptest.NewRecorder()
	ServeOpenAPISpec(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}

	// Body must be valid JSON with an "openapi" key.
	var spec map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&spec); err != nil {
		t.Fatalf("failed to decode OpenAPI spec: %v", err)
	}
	if _, ok := spec["openapi"]; !ok {
		t.Error("expected top-level \"openapi\" key in spec")
	}
}

// TestServeOpenAPISpec_AccessControlHeader verifies the CORS header is set.
func TestServeOpenAPISpec_AccessControlHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
	w := httptest.NewRecorder()
	ServeOpenAPISpec(w, r)

	res := w.Result()
	defer res.Body.Close()

	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin=*, got %q", got)
	}
}

// TestServeOpenAPISpec_DerivesSchemeAndHost verifies that the spec's server
// URL reflects the request's Host header (not a hardcoded value).
func TestServeOpenAPISpec_DerivesSchemeAndHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/openapi.json", nil)
	r.Host = "api.example.com"
	w := httptest.NewRecorder()
	ServeOpenAPISpec(w, r)

	res := w.Result()
	defer res.Body.Close()

	var spec struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&spec); err != nil {
		t.Fatalf("failed to decode spec: %v", err)
	}
	if len(spec.Servers) == 0 {
		t.Fatal("expected at least one server entry in spec")
	}
	if !strings.Contains(spec.Servers[0].URL, "api.example.com") {
		t.Errorf("expected server URL to contain host api.example.com, got %q",
			spec.Servers[0].URL)
	}
}

// ── GetAutoMobileDashboard ────────────────────────────────────────────────────

// TestGetAutoMobileDashboard_Unauthenticated verifies that GET
// /v1/automobile/dashboard without a valid access token returns HTTP 401 with
// status=false and errors["access_token"] set.
func TestGetAutoMobileDashboard_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/automobile/dashboard", nil)
	w := httptest.NewRecorder()
	GetAutoMobileDashboard(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", res.StatusCode)
	}

	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}

	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["access_token"]; !ok {
		t.Errorf("expected errors[\"access_token\"] key, got errors=%v", resp.Errors)
	}
}

// ── parseWorkshopDashboardDateRange ──────────────────────────────────────────

// TestParseWorkshopDashboardDateRange_FromMonth verifies that from_month /
// to_month (YYYY-MM format) are parsed and converted to UTC time pointers.
//
// Sign convention: tzOffset=-3 means the local timezone is UTC+3 (Saudi Arabia
// uses offset -3 in this system), so ConvertTimeZoneToUTC subtracts 3 hours.
// Midnight on 2024-01-01 local (UTC+3) is 2023-12-31 21:00:00 UTC — the
// returned time is therefore in December.
func TestParseWorkshopDashboardDateRange_FromMonth(t *testing.T) {
	// tzOffset = -3 (Saudi Arabia, UTC+3)
	const tz = -3.0
	r := httptest.NewRequest(http.MethodGet,
		"/?from_month=2024-01&to_month=2024-03", nil)

	from, to := parseWorkshopDashboardDateRange(r, tz)
	if from == nil {
		t.Fatal("expected non-nil from date for from_month=2024-01")
	}
	if to == nil {
		t.Fatal("expected non-nil to date for to_month=2024-03")
	}
	// from and to must be valid and to must be strictly after from.
	if !to.After(*from) {
		t.Errorf("expected to > from, got from=%v to=%v", from, to)
	}
}

// TestParseWorkshopDashboardDateRange_FromDate verifies that from_date /
// to_date (YYYY-MM-DD format) are parsed when month params are absent.
func TestParseWorkshopDashboardDateRange_FromDate(t *testing.T) {
	const tz = 0.0 // UTC
	r := httptest.NewRequest(http.MethodGet,
		"/?from_date=2024-06-01&to_date=2024-06-30", nil)

	from, to := parseWorkshopDashboardDateRange(r, tz)
	if from == nil {
		t.Fatal("expected non-nil from date for from_date=2024-06-01")
	}
	if to == nil {
		t.Fatal("expected non-nil to date for to_date=2024-06-30")
	}
	if from.Year() != 2024 || int(from.Month()) != 6 || from.Day() != 1 {
		t.Errorf("unexpected from date: %v", from)
	}
}

// TestParseWorkshopDashboardDateRange_EmptyParams verifies that missing params
// produce nil pointers without panicking.
func TestParseWorkshopDashboardDateRange_EmptyParams(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	from, to := parseWorkshopDashboardDateRange(r, 0)
	if from != nil {
		t.Errorf("expected nil from, got %v", from)
	}
	if to != nil {
		t.Errorf("expected nil to, got %v", to)
	}
}

// TestParseWorkshopDashboardDateRange_MonthTakesPrecedence verifies that
// from_month overrides from_date when both are present.
func TestParseWorkshopDashboardDateRange_MonthTakesPrecedence(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/?from_month=2024-01&from_date=2024-06-01", nil)
	from, _ := parseWorkshopDashboardDateRange(r, 0)
	if from == nil {
		t.Fatal("expected non-nil from date")
	}
	// Should reflect January (from_month) not June (from_date).
	if int(from.Month()) != 1 {
		t.Errorf("expected month=1 (January from from_month), got %v", from.Month())
	}
}

// TestParseWorkshopDashboardDateRange_InvalidFormats verifies that malformed
// date strings are silently ignored, leaving the pointer nil.
func TestParseWorkshopDashboardDateRange_InvalidFormats(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/?from_date=not-a-date&to_date=also-bad", nil)
	from, to := parseWorkshopDashboardDateRange(r, 0)
	if from != nil {
		t.Errorf("expected nil from for invalid from_date, got %v", from)
	}
	if to != nil {
		t.Errorf("expected nil to for invalid to_date, got %v", to)
	}
}
