package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// TestEmployeeHandlers_Unauthenticated verifies that every employee and
// employee-salary-payment handler returns HTTP 401 with errors["access_token"]
// when no auth token is supplied.
func TestEmployeeHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── employee.go ─────────────────────────────────────────────────────
		{
			name:    "ListEmployee",
			method:  http.MethodGet,
			path:    "/v1/employee",
			handler: ListEmployee,
		},
		{
			name:    "CreateEmployee",
			method:  http.MethodPost,
			path:    "/v1/employee",
			handler: CreateEmployee,
		},
		{
			name:    "ViewEmployee",
			method:  http.MethodGet,
			path:    "/v1/employee/" + fakeID,
			handler: ViewEmployee,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "UpdateEmployee",
			method:  http.MethodPut,
			path:    "/v1/employee/" + fakeID,
			handler: UpdateEmployee,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteEmployee",
			method:  http.MethodDelete,
			path:    "/v1/employee/" + fakeID,
			handler: DeleteEmployee,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "HardDeleteEmployee",
			method:  http.MethodDelete,
			path:    "/v1/employee/permanent/" + fakeID,
			handler: HardDeleteEmployee,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── salary payment ──────────────────────────────────────────────────
		{
			name:    "ListEmployeeSalaryPayment",
			method:  http.MethodGet,
			path:    "/v1/employee-salary-payment",
			handler: ListEmployeeSalaryPayment,
		},
		{
			name:    "CreateEmployeeSalaryPayment",
			method:  http.MethodPost,
			path:    "/v1/employee-salary-payment",
			handler: CreateEmployeeSalaryPayment,
		},
		{
			name:    "ViewEmployeeSalaryPayment",
			method:  http.MethodGet,
			path:    "/v1/employee-salary-payment/" + fakeID,
			handler: ViewEmployeeSalaryPayment,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "UpdateEmployeeSalaryPayment",
			method:  http.MethodPut,
			path:    "/v1/employee-salary-payment/" + fakeID,
			handler: UpdateEmployeeSalaryPayment,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteEmployeeSalaryPayment",
			method:  http.MethodDelete,
			path:    "/v1/employee-salary-payment/" + fakeID,
			handler: DeleteEmployeeSalaryPayment,
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

// TestSameOptionalDate exercises every meaningful branch of sameOptionalDate.
// The function compares two *time.Time by calendar date only (year/month/day),
// returning true when both pointers are nil or when both point to the same
// calendar day regardless of time-of-day.
func TestSameOptionalDate(t *testing.T) {
	// Helper so test cases read naturally.
	ptr := func(t time.Time) *time.Time { return &t }

	base := time.Date(2024, time.March, 15, 9, 0, 0, 0, time.UTC)
	sameDay := time.Date(2024, time.March, 15, 23, 59, 59, 0, time.UTC) // same date, different time
	nextDay := time.Date(2024, time.March, 16, 0, 0, 0, 0, time.UTC)
	diffMonth := time.Date(2024, time.April, 15, 9, 0, 0, 0, time.UTC)
	diffYear := time.Date(2025, time.March, 15, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		a    *time.Time
		b    *time.Time
		want bool
	}{
		{
			name: "both nil returns true",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "a nil b non-nil returns false",
			a:    nil,
			b:    ptr(base),
			want: false,
		},
		{
			name: "a non-nil b nil returns false",
			a:    ptr(base),
			b:    nil,
			want: false,
		},
		{
			name: "same pointer value returns true",
			a:    ptr(base),
			b:    ptr(base),
			want: true,
		},
		{
			name: "same calendar day different time-of-day returns true",
			a:    ptr(base),
			b:    ptr(sameDay),
			want: true,
		},
		{
			name: "consecutive days returns false",
			a:    ptr(base),
			b:    ptr(nextDay),
			want: false,
		},
		{
			name: "same day different month returns false",
			a:    ptr(base),
			b:    ptr(diffMonth),
			want: false,
		},
		{
			name: "same day different year returns false",
			a:    ptr(base),
			b:    ptr(diffYear),
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := sameOptionalDate(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("sameOptionalDate(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
