package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// ---------------------------------------------------------------------------
// Unauthenticated handler tests
//
// ListProductJson is intentionally excluded: it has no auth guard and resolves
// a store_id query parameter first, so it cannot reach 401.
// ---------------------------------------------------------------------------

func TestProductHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListProduct",
			method:  http.MethodGet,
			path:    "/v1/product",
			handler: ListProduct,
		},
		{
			name:    "CreateProduct",
			method:  http.MethodPost,
			path:    "/v1/product",
			handler: CreateProduct,
		},
		{
			name:    "UpdateProduct",
			method:  http.MethodPut,
			path:    "/v1/product/" + fakeID,
			handler: UpdateProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewProduct",
			method:  http.MethodGet,
			path:    "/v1/product/" + fakeID,
			handler: ViewProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewProductByItemCode",
			method:  http.MethodGet,
			path:    "/v1/product/code/SKU-001",
			handler: ViewProductByItemCode,
			muxVars: map[string]string{"code": "SKU-001"},
		},
		{
			name:    "ViewProductByBarCode",
			method:  http.MethodGet,
			path:    "/v1/product/barcode/1234567890",
			handler: ViewProductByBarCode,
			muxVars: map[string]string{"barcode": "1234567890"},
		},
		{
			name:    "DeleteProduct",
			method:  http.MethodDelete,
			path:    "/v1/product/" + fakeID,
			handler: DeleteProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreProduct",
			method:  http.MethodPost,
			path:    "/v1/product/" + fakeID + "/restore",
			handler: RestoreProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ProductSummary",
			method:  http.MethodGet,
			path:    "/v1/product/summary",
			handler: ProductSummary,
		},
		{
			name:    "GetProductBiHistory",
			method:  http.MethodGet,
			path:    "/v1/product/" + fakeID + "/bi-history",
			handler: GetProductBiHistory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "MigrateProductRackToWarehouseRacks",
			method:  http.MethodPost,
			path:    "/v1/product/migrate-racks",
			handler: MigrateProductRackToWarehouseRacks,
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
				t.Errorf("expected 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected application/json Content-Type, got %q", ct)
			}

			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[access_token], got %v", resp.Errors)
			}
		})
	}
}

// TestListProductJson_NoAuthRequired documents that ListProductJson has no auth
// guard. An invalid store_id (missing query param) leads to a non-401 error.
func TestListProductJson_NoAuthRequired(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/product.json", nil)
	w := httptest.NewRecorder()
	ListProductJson(w, r)

	res := w.Result()
	defer res.Body.Close()

	// Without a valid store_id the handler returns an error, but NOT 401 — it
	// never checks auth. Any non-401 response proves the guard is absent.
	if res.StatusCode == http.StatusUnauthorized {
		t.Error("ListProductJson must not require auth (it is a BarTender endpoint); got 401")
	}
}

// ---------------------------------------------------------------------------
// Integration stubs
// ---------------------------------------------------------------------------

// TestCreateProduct_Integration documents the full product-creation path.
func TestCreateProduct_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — run manually in a connected environment")
}

// TestUpdateProduct_Integration documents the stock-adjustment history path.
func TestUpdateProduct_Integration_StockAdjustmentHistory(t *testing.T) {
	t.Skip("requires live MongoDB with a seeded product and a valid auth token")
}
