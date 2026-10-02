package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func TestSalesReturn_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListSalesReturn",
			method:  http.MethodGet,
			path:    "/v1/salesreturn",
			handler: ListSalesReturn,
		},
		{
			name:    "CreateSalesReturn",
			method:  http.MethodPost,
			path:    "/v1/salesreturn",
			handler: CreateSalesReturn,
		},
		{
			name:    "ViewSalesReturn",
			method:  http.MethodGet,
			path:    "/v1/salesreturn/64abc123456789001234abcd",
			handler: ViewSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateSalesReturn",
			method:  http.MethodPut,
			path:    "/v1/salesreturn/64abc123456789001234abcd",
			handler: UpdateSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteSalesReturn",
			method:  http.MethodDelete,
			path:    "/v1/salesreturn/64abc123456789001234abcd",
			handler: DeleteSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UndeleteSalesReturn",
			method:  http.MethodPost,
			path:    "/v1/salesreturn/64abc123456789001234abcd/undelete",
			handler: UndeleteSalesReturn,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateSalesReturnNetTotal",
			method:  http.MethodPost,
			path:    "/v1/salesreturn/calculate-net-total",
			handler: CalculateSalesReturnNetTotal,
		},
		{
			name:    "SalesReturnSummary",
			method:  http.MethodGet,
			path:    "/v1/salesreturn/summary",
			handler: SalesReturnSummary,
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
