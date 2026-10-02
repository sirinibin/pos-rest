package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestArabicNameHandlers_Unauthenticated verifies all arabic_name.go handlers
// return 401 + errors["access_token"] without a valid token.
func TestArabicNameHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{"ListArabicName", http.MethodGet, ListArabicName, nil},
		{"CreateArabicName", http.MethodPost, CreateArabicName, nil},
		{"UpdateArabicName", http.MethodPut, UpdateArabicName, map[string]string{"id": id}},
		{"ViewArabicName", http.MethodGet, ViewArabicName, map[string]string{"id": id}},
		{"DeleteArabicName", http.MethodDelete, DeleteArabicName, map[string]string{"id": id}},
		{"HardDeleteArabicName", http.MethodDelete, HardDeleteArabicName, map[string]string{"id": id}},
		{"RestoreArabicName", http.MethodPost, RestoreArabicName, map[string]string{"id": id}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(""))
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
			if !strings.Contains(res.Header.Get("Content-Type"), "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", res.Header.Get("Content-Type"))
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

// TestStoreHandlers_Unauthenticated verifies all store.go handlers return 401
// + errors["access_token"] without a valid token.
func TestStoreHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{"ListStore", http.MethodGet, ListStore, nil},
		{"ListStoreList", http.MethodGet, ListStoreList, nil},
		{"CreateStore", http.MethodPost, CreateStore, nil},
		{"UpdateStore", http.MethodPut, UpdateStore, map[string]string{"id": id}},
		{"ViewStore", http.MethodGet, ViewStore, map[string]string{"id": id}},
		{"DeleteStore", http.MethodDelete, DeleteStore, map[string]string{"id": id}},
		{"RestoreStore", http.MethodPost, RestoreStore, map[string]string{"id": id}},
		{"MarkStoreForPermanentDeletion", http.MethodPost, MarkStoreForPermanentDeletion, map[string]string{"id": id}},
		{"PermanentlyDeleteStore", http.MethodDelete, PermanentlyDeleteStore, map[string]string{"id": id}},
		{"AbortStorePermanentDeletion", http.MethodPost, AbortStorePermanentDeletion, map[string]string{"id": id}},
		{"GetStoreSerialLocks", http.MethodGet, GetStoreSerialLocks, nil},
		{"ClearZatcaReconnect", http.MethodPost, ClearZatcaReconnect, map[string]string{"id": id}},
		{"UpdateStorePrintSettings", http.MethodPut, UpdateStorePrintSettings, map[string]string{"id": id}},
		{"UpdateStoreEmailSignatures", http.MethodPut, UpdateStoreEmailSignatures, map[string]string{"id": id}},
		{"UpdateStoreSidebarConfig", http.MethodPut, UpdateStoreSidebarConfig, map[string]string{"id": id}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(""))
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
			if !strings.Contains(res.Header.Get("Content-Type"), "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", res.Header.Get("Content-Type"))
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

// TestZatcaHandlers_Unauthenticated verifies ZATCA connection handlers return
// standard 401 + errors["access_token"].
func TestZatcaHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{"ConnectStoreToZatca", http.MethodPost, ConnectStoreToZatca, map[string]string{"id": id}},
		{"DisconnectStoreFromZatca", http.MethodPost, DisconnectStoreFromZatca, map[string]string{"id": id}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(""))
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
