package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// apiResponse is a catch-all struct used to decode JSON responses in tests.
type apiResponse struct {
	Status    bool              `json:"status"`
	Errors    map[string]string `json:"errors"`
	OK        bool              `json:"ok"`
	Redis     struct {
		Status string `json:"status"`
	} `json:"redis"`
	MongoDB struct {
		Status string `json:"status"`
	} `json:"mongodb"`
	CheckedAt string `json:"checked_at"`
}

// TestHandlers_Unauthenticated verifies that every listed endpoint returns HTTP
// 401 with a JSON body containing {"status":false,"errors":{"access_token":…}}
// when no authentication token is provided.
func TestHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		// muxVars lets individual test cases inject gorilla/mux path variables
		// without starting a real router.
		muxVars map[string]string
	}{
		{
			name:    "ListOrder",
			method:  http.MethodGet,
			path:    "/v1/order",
			handler: ListOrder,
		},
		{
			name:    "ViewOrder",
			method:  http.MethodGet,
			path:    "/v1/order/64abc123456789001234abcd",
			handler: ViewOrder,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateSalesNetTotal",
			method:  http.MethodPost,
			path:    "/v1/order/calculate-net-total",
			handler: CalculateSalesNetTotal,
		},
		{
			name:    "CreateOrder",
			method:  http.MethodPost,
			path:    "/v1/order",
			handler: CreateOrder,
		},
		{
			name:    "ListPurchase",
			method:  http.MethodGet,
			path:    "/v1/purchase",
			handler: ListPurchase,
		},
		{
			name:    "ListQuotation",
			method:  http.MethodGet,
			path:    "/v1/quotation",
			handler: ListQuotation,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.method == http.MethodPost {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader("")
			}

			r := httptest.NewRequest(tc.method, tc.path, body)

			// Inject gorilla/mux path variables when the route requires them.
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}

			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ──────────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ─────────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body ─────────────────────────────────────────────────────────
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if resp.Status {
				t.Error("expected status=false in body")
			}

			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors.access_token key, got errors=%v", resp.Errors)
			}
		})
	}
}

// TestHealthCheck_ServicesDown calls HealthCheck with no Redis or MongoDB
// initialized (db.RedisClient == nil and db.Client has not been connected to a
// running instance) and asserts:
//   - HTTP 503
//   - ok: false
//   - redis.status and mongodb.status both present
//   - checked_at is non-empty
func TestHealthCheck_ServicesDown(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()

	HealthCheck(w, r)

	res := w.Result()
	defer res.Body.Close()

	// ── Status code ──────────────────────────────────────────────────────────
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", res.StatusCode)
	}

	// ── Content-Type ─────────────────────────────────────────────────────────
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// ── Body ─────────────────────────────────────────────────────────────────
	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}

	if resp.OK {
		t.Error("expected ok=false when services are down")
	}

	if resp.Redis.Status == "" {
		t.Error("expected redis.status to be present")
	}

	if resp.MongoDB.Status == "" {
		t.Error("expected mongodb.status to be present")
	}

	if resp.CheckedAt == "" {
		t.Error("expected checked_at to be non-empty")
	}
}
