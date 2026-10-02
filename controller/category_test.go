package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestServiceCategoryHandlers_Unauthenticated verifies all service category handlers
// return 401 when no auth token is provided.
func TestServiceCategoryHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{"ListServiceCategory", http.MethodGet, "/v1/service-category", ListServiceCategory, nil},
		{"CreateServiceCategory", http.MethodPost, "/v1/service-category", CreateServiceCategory, nil},
		{"UpdateServiceCategory", http.MethodPut, "/v1/service-category/" + id, UpdateServiceCategory, map[string]string{"id": id}},
		{"ViewServiceCategory", http.MethodGet, "/v1/service-category/" + id, ViewServiceCategory, map[string]string{"id": id}},
		{"DeleteServiceCategory", http.MethodDelete, "/v1/service-category/" + id, DeleteServiceCategory, map[string]string{"id": id}},
		{"RestoreServiceCategory", http.MethodPut, "/v1/service-category/" + id + "/restore", RestoreServiceCategory, map[string]string{"id": id}},
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
				t.Errorf("expected 401, got %d", res.StatusCode)
			}
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
			}
		})
	}
}

// TestExpenseCategoryHandlers_Unauthenticated verifies all expense category handlers
// return 401 when no auth token is provided.
func TestExpenseCategoryHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{"ListExpenseCategory", http.MethodGet, "/v1/expense-category", ListExpenseCategory, nil},
		{"CreateExpenseCategory", http.MethodPost, "/v1/expense-category", CreateExpenseCategory, nil},
		{"UpdateExpenseCategory", http.MethodPut, "/v1/expense-category/" + id, UpdateExpenseCategory, map[string]string{"id": id}},
		{"ViewExpenseCategory", http.MethodGet, "/v1/expense-category/" + id, ViewExpenseCategory, map[string]string{"id": id}},
		{"DeleteExpenseCategory", http.MethodDelete, "/v1/expense-category/" + id, DeleteExpenseCategory, map[string]string{"id": id}},
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
				t.Errorf("expected 401, got %d", res.StatusCode)
			}
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
			}
		})
	}
}

// TestVendorCategoryHandlers_Unauthenticated verifies all vendor category handlers
// return 401 when no auth token is provided.
func TestVendorCategoryHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{"ListVendorCategory", http.MethodGet, "/v1/vendor-category", ListVendorCategory, nil},
		{"CreateVendorCategory", http.MethodPost, "/v1/vendor-category", CreateVendorCategory, nil},
		{"UpdateVendorCategory", http.MethodPut, "/v1/vendor-category/" + id, UpdateVendorCategory, map[string]string{"id": id}},
		{"ViewVendorCategory", http.MethodGet, "/v1/vendor-category/" + id, ViewVendorCategory, map[string]string{"id": id}},
		{"DeleteVendorCategory", http.MethodDelete, "/v1/vendor-category/" + id, DeleteVendorCategory, map[string]string{"id": id}},
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
				t.Errorf("expected 401, got %d", res.StatusCode)
			}
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
			}
		})
	}
}
