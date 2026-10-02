package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestExpenseHandlers_Unauthenticated verifies that every expense and
// expense-category handler returns HTTP 401 with errors["access_token"] when
// no auth token is supplied.
func TestExpenseHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── expense.go ──────────────────────────────────────────────────────
		{
			name:    "ListExpense",
			method:  http.MethodGet,
			path:    "/v1/expense",
			handler: ListExpense,
		},
		{
			name:    "CreateExpense",
			method:  http.MethodPost,
			path:    "/v1/expense",
			handler: CreateExpense,
		},
		{
			name:    "UpdateExpense",
			method:  http.MethodPut,
			path:    "/v1/expense/" + fakeID,
			handler: UpdateExpense,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewExpense",
			method:  http.MethodGet,
			path:    "/v1/expense/" + fakeID,
			handler: ViewExpense,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewExpenseByCode",
			method:  http.MethodGet,
			path:    "/v1/expense/code/EXP-001",
			handler: ViewExpenseByCode,
			muxVars: map[string]string{"code": "EXP-001"},
		},
		{
			name:    "DeleteExpense",
			method:  http.MethodDelete,
			path:    "/v1/expense/" + fakeID,
			handler: DeleteExpense,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ExpenseSummary",
			method:  http.MethodGet,
			path:    "/v1/expense/summary",
			handler: ExpenseSummary,
		},
		// ── expense_category.go ─────────────────────────────────────────────
		{
			name:    "ListExpenseCategory",
			method:  http.MethodGet,
			path:    "/v1/expense-category",
			handler: ListExpenseCategory,
		},
		{
			name:    "CreateExpenseCategory",
			method:  http.MethodPost,
			path:    "/v1/expense-category",
			handler: CreateExpenseCategory,
		},
		{
			name:    "UpdateExpenseCategory",
			method:  http.MethodPut,
			path:    "/v1/expense-category/" + fakeID,
			handler: UpdateExpenseCategory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewExpenseCategory",
			method:  http.MethodGet,
			path:    "/v1/expense-category/" + fakeID,
			handler: ViewExpenseCategory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteExpenseCategory",
			method:  http.MethodDelete,
			path:    "/v1/expense-category/" + fakeID,
			handler: DeleteExpenseCategory,
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
