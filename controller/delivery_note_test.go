package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestDeliveryNote_Unauthenticated verifies that every delivery-note endpoint
// returns HTTP 401 with errors.access_token when no authentication token is
// provided.
func TestDeliveryNote_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListDeliveryNote",
			method:  http.MethodGet,
			path:    "/v1/delivery-note",
			handler: ListDeliveryNote,
		},
		{
			name:    "CreateDeliveryNote",
			method:  http.MethodPost,
			path:    "/v1/delivery-note",
			handler: CreateDeliveryNote,
		},
		{
			name:    "UpdateDeliveryNote",
			method:  http.MethodPut,
			path:    "/v1/delivery-note/64abc123456789001234abcd",
			handler: UpdateDeliveryNote,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewDeliveryNote",
			method:  http.MethodGet,
			path:    "/v1/delivery-note/64abc123456789001234abcd",
			handler: ViewDeliveryNote,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateDeliveryNoteNetTotal",
			method:  http.MethodPost,
			path:    "/v1/delivery-note/calculate-net-total",
			handler: CalculateDeliveryNoteNetTotal,
		},
		{
			name:    "ListDeliveryNoteReminders",
			method:  http.MethodGet,
			path:    "/v1/delivery-note/reminders",
			handler: ListDeliveryNoteReminders,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

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

// TestDeliveryNote_Integration is a stub for DB-backed integration tests.
func TestDeliveryNote_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}
