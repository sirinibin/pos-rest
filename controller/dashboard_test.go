package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)


func TestDashboardHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{"DashboardGetMonthly", http.MethodGet, "/v1/dashboard/monthly", DashboardGetMonthly},
		{"DashboardGetProducts", http.MethodGet, "/v1/dashboard/products", DashboardGetProducts},
		{"DashboardGetCustomers", http.MethodGet, "/v1/dashboard/customers", DashboardGetCustomers},
		{"DashboardGetOutstanding", http.MethodGet, "/v1/dashboard/outstanding", DashboardGetOutstanding},
		{"DashboardGetCategories", http.MethodGet, "/v1/dashboard/categories", DashboardGetCategories},
		{"DashboardGetVendors", http.MethodGet, "/v1/dashboard/vendors", DashboardGetVendors},
		{"DashboardGetAccounts", http.MethodGet, "/v1/dashboard/accounts", DashboardGetAccounts},
		{"DashboardGetStock", http.MethodGet, "/v1/dashboard/stock", DashboardGetStock},
		{"DashboardGetEmployee", http.MethodGet, "/v1/dashboard/employee", DashboardGetEmployee},
		{"DashboardBackfill", http.MethodPost, "/v1/dashboard/backfill", DashboardBackfill},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", ct)
			}

			// Dashboard handlers use models.Response with Errors["error"] key
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["error"]; !ok {
				t.Errorf("expected errors[\"error\"], got %v", resp.Errors)
			}
		})
	}
}

// TestMonthRangeToDateRange covers the pure helper that converts YYYY-MM month strings
// to date range strings used in MongoDB queries.
func TestMonthRangeToDateRange(t *testing.T) {
	cases := []struct {
		name      string
		fromMonth string
		toMonth   string
		wantFrom  string
		wantTo    string
	}{
		{
			name:      "both months set",
			fromMonth: "2024-01",
			toMonth:   "2024-03",
			wantFrom:  "2024-01-01",
			wantTo:    "2024-03-31",
		},
		{
			name:      "from month only",
			fromMonth: "2024-06",
			toMonth:   "",
			wantFrom:  "2024-06-01",
			wantTo:    "",
		},
		{
			name:      "to month only",
			fromMonth: "",
			toMonth:   "2024-12",
			wantFrom:  "",
			wantTo:    "2024-12-31",
		},
		{
			name:      "both empty",
			fromMonth: "",
			toMonth:   "",
			wantFrom:  "",
			wantTo:    "",
		},
		{
			name:      "december",
			fromMonth: "2023-12",
			toMonth:   "2023-12",
			wantFrom:  "2023-12-01",
			wantTo:    "2023-12-31",
		},
		{
			name:      "february gets -31 suffix for lexicographic range queries",
			fromMonth: "2024-02",
			toMonth:   "2024-02",
			wantFrom:  "2024-02-01",
			wantTo:    "2024-02-31",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			gotFrom, gotTo := monthRangeToDateRange(c.fromMonth, c.toMonth, 0)
			if gotFrom != c.wantFrom {
				t.Errorf("fromDate: want %q, got %q", c.wantFrom, gotFrom)
			}
			if gotTo != c.wantTo {
				t.Errorf("toDate: want %q, got %q", c.wantTo, gotTo)
			}
		})
	}
}
