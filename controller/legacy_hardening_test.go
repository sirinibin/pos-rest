package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// #6: an all-zero store id is refused, not a nil-pointer panic.
func TestParseStore_ZeroStoreID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/product?search[store_id]=000000000000000000000000", nil)
	store, err := ParseStore(r)
	if err == nil || store != nil || !strings.Contains(err.Error(), "invalid store id") {
		t.Fatalf("ParseStore(zero id) = %v, %v; want an invalid store id error", store, err)
	}
}

// #7 #8: missing/unknown store ids and JSON null bodies are refused (status
// false, never 5xx) instead of panicking (the handlers dereferenced a nil store or a nil model).
func TestLegacyHandlers_NoStoreOrNullBody_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	for _, c := range []struct {
		name string
		h    http.HandlerFunc
		url  string
		body interface{}
	}{
		{"warehouse without store", CreateWarehouse, "/v1/warehouse", map[string]interface{}{"name": "X"}},
		{"warehouse unknown store", CreateWarehouse, "/v1/warehouse", map[string]interface{}{"name": "X", "store_id": "5f0000000000000000000001"}},
		{"signature without store", CreateSignature, "/v1/signature", map[string]interface{}{"name": "X"}},
		{"stock transfer without store", CreateStockTransfer, "/v1/stock-transfer", map[string]interface{}{"code": "X"}},
		{"stock transfer without vat_percent", CreateStockTransfer, "/v1/stock-transfer?search[store_id]=" + fx.StoreA.Hex(),
			map[string]interface{}{"store_id": fx.StoreA.Hex(), "products": []interface{}{}}},
		{"warehouse null body", CreateWarehouse, "/v1/warehouse", "null"},
		{"user null body", CreateUser, "/v1/user", "null"},
		{"signature null body", CreateSignature, "/v1/signature", "null"},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panic: %v", p)
				}
			}()
			r := callHandler(t, c.h, "POST", c.url, tok, c.body)
			// these legacy handlers answer some validation errors with 200 + status false
			if r.Status || r.Code >= 500 || (r.Code != http.StatusOK && r.Code != http.StatusBadRequest) {
				t.Fatalf("want a refusal (400 or status false), got %d %s", r.Code, r.Raw)
			}
		})
	}
}
