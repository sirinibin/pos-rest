package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
)

// TestProductTaxonomy_Unauthenticated verifies that every product-category,
// product-brand, and product-history endpoint returns HTTP 401 with a JSON
// body containing {"status":false,"errors":{"access_token":…}} when no
// authentication token is provided.
func TestProductTaxonomy_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── ProductCategory ───────────────────────────────────────────────────
		{
			name:    "ListProductCategory",
			method:  http.MethodGet,
			path:    "/v1/product-category",
			handler: ListProductCategory,
		},
		{
			name:    "CreateProductCategory",
			method:  http.MethodPost,
			path:    "/v1/product-category",
			handler: CreateProductCategory,
		},
		{
			name:    "UpdateProductCategory",
			method:  http.MethodPut,
			path:    "/v1/product-category/" + fakeID,
			handler: UpdateProductCategory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewProductCategory",
			method:  http.MethodGet,
			path:    "/v1/product-category/" + fakeID,
			handler: ViewProductCategory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteProductCategory",
			method:  http.MethodDelete,
			path:    "/v1/product-category/" + fakeID,
			handler: DeleteProductCategory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreProductCategory",
			method:  http.MethodPost,
			path:    "/v1/product-category/" + fakeID + "/restore",
			handler: RestoreProductCategory,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── ProductBrand ──────────────────────────────────────────────────────
		{
			name:    "ListProductBrand",
			method:  http.MethodGet,
			path:    "/v1/product-brand",
			handler: ListProductBrand,
		},
		{
			name:    "CreateProductBrand",
			method:  http.MethodPost,
			path:    "/v1/product-brand",
			handler: CreateProductBrand,
		},
		{
			name:    "UpdateProductBrand",
			method:  http.MethodPut,
			path:    "/v1/product-brand/" + fakeID,
			handler: UpdateProductBrand,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewProductBrand",
			method:  http.MethodGet,
			path:    "/v1/product-brand/" + fakeID,
			handler: ViewProductBrand,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteProductBrand",
			method:  http.MethodDelete,
			path:    "/v1/product-brand/" + fakeID,
			handler: DeleteProductBrand,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreProductBrand",
			method:  http.MethodPost,
			path:    "/v1/product-brand/" + fakeID + "/restore",
			handler: RestoreProductBrand,
			muxVars: map[string]string{"id": fakeID},
		},
		// ── ProductHistory ────────────────────────────────────────────────────
		{
			name:    "ListProductHistory",
			method:  http.MethodGet,
			path:    "/v1/product/history",
			handler: ListProductHistory,
		},
		{
			name:    "ProductHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/product/history/summary",
			handler: ProductHistorySummary,
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

// TestUploadProductImage_SkipAuth_Integration runs UploadProductImage (which
// parses the multipart form before, and instead of, any authentication)
// against a fresh product in the seeded store: form validation, a successful
// upload stored via the local-disk fallback (no S3 configured in the test DB)
// and appended to the product's images, and the no-token behaviour.
func TestUploadProductImage_SkipAuth_Integration(t *testing.T) {
	fx := requireDB(t)
	id := insertStoreDoc(t, fx.StoreA, "product", bson.M{"name": uniqName("Upload Product"), "store_id": fx.StoreA, "deleted": false})
	assertImageUpload(t, UploadProductImage, "/v1/product/upload-image", tokenFor(t, fx.AdminEmail), fx.StoreA, id, "product", "products")
}

// TestDeleteProductImage_NoAuthGuard documents that DeleteProductImage carries
// no authentication guard.  It validates required query parameters and returns
// 400 Bad Request (not 401) when they are absent.
func TestDeleteProductImage_NoAuthGuard(t *testing.T) {
	// Empty request — url, id, and storeID are all missing.
	r := httptest.NewRequest(http.MethodDelete, "/v1/product/image", nil)
	w := httptest.NewRecorder()
	DeleteProductImage(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required params, got %d", res.StatusCode)
	}
}
