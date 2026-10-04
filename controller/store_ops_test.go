package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestStoreOps_Unauthenticated verifies that every store-operation endpoint
// returns HTTP 401 with a JSON body containing {"status":false,"errors":{"access_token":…}}
// when no authentication token is provided.
//
// Covered files: backup.go, store_data.go, duplicate.go,
// duplicate_with_products.go, duplicate_with_products_no_images.go,
// duplicate_without_data.go, posting.go.
func TestStoreOps_Unauthenticated(t *testing.T) {
	const storeID = "64abc123456789001234abcd"
	muxWithStore := map[string]string{"id": storeID}

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── backup.go ────────────────────────────────────────────────────────────
		{
			name:    "GetStoreBackupSize",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/backup/size",
			handler: GetStoreBackupSize,
			muxVars: muxWithStore,
		},
		{
			name:    "StartStoreBackup",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/backup/start",
			handler: StartStoreBackup,
			muxVars: muxWithStore,
		},
		{
			name:    "GetStoreBackupProgress",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/backup/progress",
			handler: GetStoreBackupProgress,
			muxVars: muxWithStore,
		},
		// ── store_data.go ────────────────────────────────────────────────────────
		{
			name:    "PopulateStoreTestData",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/populate-test-data",
			handler: PopulateStoreTestData,
			muxVars: muxWithStore,
		},
		{
			name:    "ClearStoreData",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/clear-data",
			handler: ClearStoreData,
			muxVars: muxWithStore,
		},
		// ── duplicate.go ─────────────────────────────────────────────────────────
		// GetStoreDuplicateSize delegates to GetStoreBackupSize; auth guard is identical.
		{
			name:    "GetStoreDuplicateSize",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate/size",
			handler: GetStoreDuplicateSize,
			muxVars: muxWithStore,
		},
		{
			name:    "StartStoreDuplicate",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/duplicate/start",
			handler: StartStoreDuplicate,
			muxVars: muxWithStore,
		},
		{
			name:    "GetStoreDuplicateProgress",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate/progress",
			handler: GetStoreDuplicateProgress,
			muxVars: muxWithStore,
		},
		// ── duplicate_with_products.go ────────────────────────────────────────────
		{
			name:    "GetStoreDuplicateWithProductsSize",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate-with-products/size",
			handler: GetStoreDuplicateWithProductsSize,
			muxVars: muxWithStore,
		},
		{
			name:    "StartStoreDuplicateWithProducts",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/duplicate-with-products/start",
			handler: StartStoreDuplicateWithProducts,
			muxVars: muxWithStore,
		},
		{
			name:    "GetStoreDuplicateWithProductsProgress",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate-with-products/progress",
			handler: GetStoreDuplicateWithProductsProgress,
			muxVars: muxWithStore,
		},
		// ── duplicate_with_products_no_images.go ──────────────────────────────────
		{
			name:    "GetStoreDuplicateWithProductsNoImagesSize",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate-with-products-no-images/size",
			handler: GetStoreDuplicateWithProductsNoImagesSize,
			muxVars: muxWithStore,
		},
		{
			name:    "StartStoreDuplicateWithProductsNoImages",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/duplicate-with-products-no-images/start",
			handler: StartStoreDuplicateWithProductsNoImages,
			muxVars: muxWithStore,
		},
		{
			name:    "GetStoreDuplicateWithProductsNoImagesProgress",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate-with-products-no-images/progress",
			handler: GetStoreDuplicateWithProductsNoImagesProgress,
			muxVars: muxWithStore,
		},
		// ── duplicate_without_data.go ─────────────────────────────────────────────
		{
			name:    "GetStoreDuplicateWithoutDataSize",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate-without-data/size",
			handler: GetStoreDuplicateWithoutDataSize,
			muxVars: muxWithStore,
		},
		{
			name:    "StartStoreDuplicateWithoutData",
			method:  http.MethodPost,
			path:    "/v1/store/" + storeID + "/duplicate-without-data/start",
			handler: StartStoreDuplicateWithoutData,
			muxVars: muxWithStore,
		},
		{
			name:    "GetStoreDuplicateWithoutDataProgress",
			method:  http.MethodGet,
			path:    "/v1/store/" + storeID + "/duplicate-without-data/progress",
			handler: GetStoreDuplicateWithoutDataProgress,
			muxVars: muxWithStore,
		},
		// ── posting.go ───────────────────────────────────────────────────────────
		{
			name:    "ListPostings",
			method:  http.MethodGet,
			path:    "/v1/posting",
			handler: ListPostings,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ───────────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ──────────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body ──────────────────────────────────────────────────────────
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

// TestDownloadStoreBackupFile_Unauthenticated confirms that the backup file
// download endpoint returns HTTP 401 when no access token is provided.
//
// This handler uses http.Error rather than a structured JSON response, so only
// the status code is asserted here.
func TestDownloadStoreBackupFile_Unauthenticated(t *testing.T) {
	const storeID = "64abc123456789001234abcd"
	r := httptest.NewRequest(http.MethodGet, "/v1/store/"+storeID+"/backup/file", nil)
	r = mux.SetURLVars(r, map[string]string{"id": storeID})
	w := httptest.NewRecorder()

	DownloadStoreBackupFile(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", res.StatusCode)
	}
}
