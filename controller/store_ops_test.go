package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestStoreOpsHandlers_Unauthenticated verifies backup, store-data, duplicate,
// and posting handlers all return 401 + errors["access_token"] without a token.
func TestStoreOpsHandlers_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// backup.go
		{"GetStoreBackupSize", http.MethodGet, "/v1/store/" + id + "/backup/size", GetStoreBackupSize, map[string]string{"id": id}},
		{"StartStoreBackup", http.MethodPost, "/v1/store/" + id + "/backup/start", StartStoreBackup, map[string]string{"id": id}},
		{"GetStoreBackupProgress", http.MethodGet, "/v1/store/" + id + "/backup/progress", GetStoreBackupProgress, map[string]string{"id": id}},
		// DownloadStoreBackupFile uses http.Error (plain text) — tested separately below.
		// store_data.go
		{"PopulateStoreTestData", http.MethodPost, "/v1/store/" + id + "/populate-test-data", PopulateStoreTestData, map[string]string{"id": id}},
		{"ClearStoreData", http.MethodPost, "/v1/store/" + id + "/clear-data", ClearStoreData, map[string]string{"id": id}},
		// duplicate.go
		{"GetStoreDuplicateSize", http.MethodGet, "/v1/store/" + id + "/duplicate/size", GetStoreDuplicateSize, map[string]string{"id": id}},
		{"StartStoreDuplicate", http.MethodPost, "/v1/store/" + id + "/duplicate/start", StartStoreDuplicate, map[string]string{"id": id}},
		{"GetStoreDuplicateProgress", http.MethodGet, "/v1/store/" + id + "/duplicate/progress", GetStoreDuplicateProgress, map[string]string{"id": id}},
		// duplicate_with_products.go
		{"GetStoreDuplicateWithProductsSize", http.MethodGet, "/v1/store/" + id + "/duplicate-with-products/size", GetStoreDuplicateWithProductsSize, map[string]string{"id": id}},
		{"StartStoreDuplicateWithProducts", http.MethodPost, "/v1/store/" + id + "/duplicate-with-products/start", StartStoreDuplicateWithProducts, map[string]string{"id": id}},
		{"GetStoreDuplicateWithProductsProgress", http.MethodGet, "/v1/store/" + id + "/duplicate-with-products/progress", GetStoreDuplicateWithProductsProgress, map[string]string{"id": id}},
		// duplicate_with_products_no_images.go
		{"GetStoreDuplicateWithProductsNoImagesSize", http.MethodGet, "/v1/store/" + id + "/duplicate-no-images/size", GetStoreDuplicateWithProductsNoImagesSize, map[string]string{"id": id}},
		{"StartStoreDuplicateWithProductsNoImages", http.MethodPost, "/v1/store/" + id + "/duplicate-no-images/start", StartStoreDuplicateWithProductsNoImages, map[string]string{"id": id}},
		{"GetStoreDuplicateWithProductsNoImagesProgress", http.MethodGet, "/v1/store/" + id + "/duplicate-no-images/progress", GetStoreDuplicateWithProductsNoImagesProgress, map[string]string{"id": id}},
		// duplicate_without_data.go
		{"GetStoreDuplicateWithoutDataSize", http.MethodGet, "/v1/store/" + id + "/duplicate-without-data/size", GetStoreDuplicateWithoutDataSize, map[string]string{"id": id}},
		{"StartStoreDuplicateWithoutData", http.MethodPost, "/v1/store/" + id + "/duplicate-without-data/start", StartStoreDuplicateWithoutData, map[string]string{"id": id}},
		{"GetStoreDuplicateWithoutDataProgress", http.MethodGet, "/v1/store/" + id + "/duplicate-without-data/progress", GetStoreDuplicateWithoutDataProgress, map[string]string{"id": id}},
		// posting.go
		{"ListPostings", http.MethodGet, "/v1/posting", ListPostings, nil},
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
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", ct)
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

// TestDownloadStoreBackupFile_Unauthenticated verifies the download handler
// returns 401. It uses http.Error (plain text), not JSON.
func TestDownloadStoreBackupFile_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	r := httptest.NewRequest(http.MethodGet, "/v1/store/"+id+"/backup/download", strings.NewReader(""))
	r = mux.SetURLVars(r, map[string]string{"id": id})
	w := httptest.NewRecorder()
	DownloadStoreBackupFile(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestWhatsAppHandlers_NoAuthGuard documents that WhatsApp proxy handlers do
// not use JWT auth — they use store_id-based access. Without a store_id they
// return 400, not 401. These are internal proxy endpoints that relay requests
// to the Evolution API instance.
func TestWhatsAppHandlers_NoAuthGuard(t *testing.T) {
	noAuthHandlers := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"GetWhatsAppQR", http.MethodGet, GetWhatsAppQR},
		{"GetWhatsAppStatus", http.MethodGet, GetWhatsAppStatus},
		{"GetWhatsAppContacts", http.MethodGet, GetWhatsAppContacts},
		{"GetWhatsAppContactsCount", http.MethodGet, GetWhatsAppContactsCount},
		{"ClearWhatsAppContacts", http.MethodGet, ClearWhatsAppContacts},
	}

	for _, tc := range noAuthHandlers {
		tc := tc
		t.Run(tc.name+"_no_auth_guard", func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/?store_id=", strings.NewReader(""))
			w := httptest.NewRecorder()
			tc.handler(w, r)
			// These handlers do not return 401 — they return 400 (bad store_id) or
			// process via Evolution API. Verify they do NOT require a JWT by
			// confirming they don't return 401.
			if w.Code == http.StatusUnauthorized {
				t.Errorf("%s returned 401, but this handler has no JWT auth guard", tc.name)
			}
		})
	}
}
