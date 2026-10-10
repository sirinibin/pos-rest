package controller

import (
	"net/http/httptest"
	"testing"
)

// ParseStore runs before every store-scoped handler. An all-zero store id
// used to call err.Error() on a nil error and panic ~170 endpoints.
func TestParseStore_RejectsMissingAndMalformedIDs(t *testing.T) {
	cases := map[string]string{
		"missing":   "/v1/product",
		"empty":     "/v1/product?search[store_id]=",
		"all zero":  "/v1/product?search[store_id]=000000000000000000000000",
		"not hex":   "/v1/product?search[store_id]=zzzz",
		"too short": "/v1/product?search[store_id]=abc123",
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseStore panicked: %v", r)
				}
			}()
			store, err := ParseStore(httptest.NewRequest("GET", target, nil))
			if err == nil || store != nil {
				t.Fatalf("ParseStore(%s) = %v, %v; want an error", target, store, err)
			}
		})
	}
}
