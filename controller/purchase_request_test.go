package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestPurchaseRequest_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListPurchaseRequest",
			method:  http.MethodGet,
			path:    "/v1/purchase-request",
			handler: ListPurchaseRequest,
		},
		{
			name:    "CreatePurchaseRequest",
			method:  http.MethodPost,
			path:    "/v1/purchase-request",
			handler: CreatePurchaseRequest,
		},
		{
			name:    "ViewPurchaseRequest",
			method:  http.MethodGet,
			path:    "/v1/purchase-request/64abc123456789001234abcd",
			handler: ViewPurchaseRequest,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdatePurchaseRequest",
			method:  http.MethodPut,
			path:    "/v1/purchase-request/64abc123456789001234abcd",
			handler: UpdatePurchaseRequest,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeletePurchaseRequest",
			method:  http.MethodDelete,
			path:    "/v1/purchase-request/64abc123456789001234abcd",
			handler: DeletePurchaseRequest,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "AcceptPurchaseRequest",
			method:  http.MethodPost,
			path:    "/v1/purchase-request/64abc123456789001234abcd/accept",
			handler: AcceptPurchaseRequest,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "RejectPurchaseRequest",
			method:  http.MethodPost,
			path:    "/v1/purchase-request/64abc123456789001234abcd/reject",
			handler: RejectPurchaseRequest,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CreatePurchaseOrderFromPR",
			method:  http.MethodPost,
			path:    "/v1/purchase-request/64abc123456789001234abcd/create-purchase-order",
			handler: CreatePurchaseOrderFromPR,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
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
