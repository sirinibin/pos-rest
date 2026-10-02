package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestCustomerDepositAndWithdrawalHandlers_Unauthenticated verifies that every
// customer-deposit and customer-withdrawal handler returns HTTP 401 with
// errors["access_token"] when no auth token is supplied.
func TestCustomerDepositAndWithdrawalHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── customer_deposit.go ─────────────────────────────────────────────
		{
			name:    "ListCustomerDeposit",
			method:  http.MethodGet,
			path:    "/v1/customerdeposit",
			handler: ListCustomerDeposit,
		},
		{
			name:    "CreateCustomerDeposit",
			method:  http.MethodPost,
			path:    "/v1/customerdeposit",
			handler: CreateCustomerDeposit,
		},
		{
			name:    "UpdateCustomerDeposit",
			method:  http.MethodPut,
			path:    "/v1/customerdeposit/" + fakeID,
			handler: UpdateCustomerDeposit,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomerDeposit",
			method:  http.MethodGet,
			path:    "/v1/customerdeposit/" + fakeID,
			handler: ViewCustomerDeposit,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomerDepositByCode",
			method:  http.MethodGet,
			path:    "/v1/customerdeposit/code/REC-001",
			handler: ViewCustomerDepositByCode,
			muxVars: map[string]string{"code": "REC-001"},
		},
		{
			name:    "DeleteCustomerDeposit",
			method:  http.MethodDelete,
			path:    "/v1/customerdeposit/" + fakeID,
			handler: DeleteCustomerDeposit,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── customer_withdrawal.go ──────────────────────────────────────────
		{
			name:    "ListCustomerWithdrawal",
			method:  http.MethodGet,
			path:    "/v1/customerwithdrawal",
			handler: ListCustomerWithdrawal,
		},
		{
			name:    "CreateCustomerWithdrawal",
			method:  http.MethodPost,
			path:    "/v1/customerwithdrawal",
			handler: CreateCustomerWithdrawal,
		},
		{
			name:    "UpdateCustomerWithdrawal",
			method:  http.MethodPut,
			path:    "/v1/customerwithdrawal/" + fakeID,
			handler: UpdateCustomerWithdrawal,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomerWithdrawal",
			method:  http.MethodGet,
			path:    "/v1/customerwithdrawal/" + fakeID,
			handler: ViewCustomerWithdrawal,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomerWithdrawalByCode",
			method:  http.MethodGet,
			path:    "/v1/customerwithdrawal/code/PAY-001",
			handler: ViewCustomerWithdrawalByCode,
			muxVars: map[string]string{"code": "PAY-001"},
		},
		{
			name:    "DeleteCustomerWithdrawal",
			method:  http.MethodDelete,
			path:    "/v1/customerwithdrawal/" + fakeID,
			handler: DeleteCustomerWithdrawal,
			muxVars: map[string]string{"id": fakeID},
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
