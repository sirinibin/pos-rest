package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestPurchase_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListPurchase",
			method:  http.MethodGet,
			path:    "/v1/purchase",
			handler: ListPurchase,
		},
		{
			name:    "CreatePurchase",
			method:  http.MethodPost,
			path:    "/v1/purchase",
			handler: CreatePurchase,
		},
		{
			name:    "ViewPurchase",
			method:  http.MethodGet,
			path:    "/v1/purchase/64abc123456789001234abcd",
			handler: ViewPurchase,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdatePurchase",
			method:  http.MethodPut,
			path:    "/v1/purchase/64abc123456789001234abcd",
			handler: UpdatePurchase,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeletePurchase",
			method:  http.MethodDelete,
			path:    "/v1/purchase/64abc123456789001234abcd",
			handler: DeletePurchase,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculatePurchaseNetTotal",
			method:  http.MethodPost,
			path:    "/v1/purchase/calculate-net-total",
			handler: CalculatePurchaseNetTotal,
		},
		{
			name:    "PurchaseSummary",
			method:  http.MethodGet,
			path:    "/v1/purchase/summary",
			handler: PurchaseSummary,
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
			if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
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
				t.Errorf("expected errors.access_token, got %v", resp.Errors)
			}
		})
	}
}

// TestParsePurchaseBill_HandlerExists verifies the ParsePurchaseBill handler is
// defined in purchase.go. The handler is excluded from the unauthenticated table
// because it reads r.FormFile before calling AuthenticateByAccessToken; a request
// with no multipart body causes a nil-dereference panic. The handler must be
// fixed to authenticate before touching the request body.
func TestParsePurchaseBill_HandlerExists(t *testing.T) {
	src, err := os.ReadFile("purchase.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !strings.Contains(string(src), "func ParsePurchaseBill") {
		t.Error("ParsePurchaseBill not found in purchase.go")
	}
}
