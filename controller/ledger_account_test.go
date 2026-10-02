package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestLedgerAndAccount_Unauthenticated verifies that every ledger and account
// endpoint returns HTTP 401 with errors.access_token when no authentication
// token is provided.
func TestLedgerAndAccount_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── ledger.go ─────────────────────────────────────────────────────────
		{
			name:    "ListLedger",
			method:  http.MethodGet,
			path:    "/v1/ledger",
			handler: ListLedger,
		},
		// ── account.go ────────────────────────────────────────────────────────
		{
			name:    "ListAccounts",
			method:  http.MethodGet,
			path:    "/v1/account",
			handler: ListAccounts,
		},
		{
			name:    "ViewAccount",
			method:  http.MethodGet,
			path:    "/v1/account/64abc123456789001234abcd",
			handler: ViewAccount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteAccount",
			method:  http.MethodDelete,
			path:    "/v1/account/64abc123456789001234abcd",
			handler: DeleteAccount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "RestoreAccount",
			method:  http.MethodPost,
			path:    "/v1/account/64abc123456789001234abcd/restore",
			handler: RestoreAccount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
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

// TestLedger_Integration is a stub for DB-backed integration tests.
func TestLedger_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}

// TestAccount_Integration is a stub for DB-backed integration tests.
func TestAccount_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually against a test instance")
}
