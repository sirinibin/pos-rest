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
// ---------------------------------------------------------------------------

func TestCustomerHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListCustomer",
			method:  http.MethodGet,
			path:    "/v1/customer",
			handler: ListCustomer,
		},
		{
			name:    "CreateCustomer",
			method:  http.MethodPost,
			path:    "/v1/customer",
			handler: CreateCustomer,
		},
		{
			name:    "UpdateCustomer",
			method:  http.MethodPut,
			path:    "/v1/customer/" + fakeID,
			handler: UpdateCustomer,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomer",
			method:  http.MethodGet,
			path:    "/v1/customer/" + fakeID,
			handler: ViewCustomer,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "DeleteCustomer",
			method:  http.MethodDelete,
			path:    "/v1/customer/" + fakeID,
			handler: DeleteCustomer,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreCustomer",
			method:  http.MethodPost,
			path:    "/v1/customer/" + fakeID + "/restore",
			handler: RestoreCustomer,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewCustomerByVatNoByName",
			method:  http.MethodGet,
			path:    "/v1/customer/by-vat-name",
			handler: ViewCustomerByVatNoByName,
		},
		{
			name:    "GetCustomerHistory",
			method:  http.MethodGet,
			path:    "/v1/customer/" + fakeID + "/history",
			handler: GetCustomerHistory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "CustomerSummary",
			method:  http.MethodGet,
			path:    "/v1/customer/summary",
			handler: CustomerSummary,
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
				t.Errorf("expected application/json, got %q", ct)
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

// FindOrCreateCustomerHandler has an auth guard but returns a non-standard JSON body.
func TestFindOrCreateCustomerHandler_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/customer/find-or-create", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	FindOrCreateCustomerHandler(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] == "" {
		t.Error("expected non-empty error field in response")
	}
}

// FindCustomerByPhoneHandler has no auth guard; it validates store_id first.
func TestFindCustomerByPhoneHandler_InvalidStoreID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/v1/customer/by-phone?store_id=bad&phone=0501234567",
		nil,
	)
	w := httptest.NewRecorder()
	FindCustomerByPhoneHandler(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad store_id, got %d", res.StatusCode)
	}
}

// FindCustomerByPhoneHandler requires a phone param.
func TestFindCustomerByPhoneHandler_MissingPhone(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"
	r := httptest.NewRequest(http.MethodGet,
		"/v1/customer/by-phone?store_id="+fakeID,
		nil,
	)
	w := httptest.NewRecorder()
	FindCustomerByPhoneHandler(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing phone, got %d", res.StatusCode)
	}
}

// NormalizeCustomerPhonesHandler has no auth guard; bad store_id → 400.
func TestNormalizeCustomerPhonesHandler_InvalidStoreID(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost,
		"/v1/customers/normalize-phones?store_id=notahex",
		nil,
	)
	w := httptest.NewRecorder()
	NormalizeCustomerPhonesHandler(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad store_id, got %d", res.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Pure-function tests: phoneNormVariants
// ---------------------------------------------------------------------------

func TestPhoneNormVariants(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// wantContains: all these must appear in the result slice
		wantContains []string
		// wantAbsent: none of these must appear
		wantAbsent []string
		wantNil    bool
	}{
		{
			name:    "empty string → nil",
			input:   "",
			wantNil: true,
		},
		{
			name:         "no digits (letters only) → returns raw",
			input:        "abc",
			wantContains: []string{"abc"},
		},
		{
			name:         "local 10-digit starting with 0 → adds 966 variant",
			input:        "0538886816",
			wantContains: []string{"0538886816", "966538886816"},
		},
		{
			name:         "local 0501234567 → both forms",
			input:        "0501234567",
			wantContains: []string{"0501234567", "966501234567"},
		},
		{
			name:         "E.164 +966 → strips + and adds 0 variant",
			input:        "+966538886816",
			wantContains: []string{"966538886816", "0538886816"},
		},
		{
			name:         "E.164 +966 includes trimmed raw",
			input:        "+966538886816",
			wantContains: []string{"+966538886816"},
		},
		{
			name:         "international without + (12 digits 966…) → adds 0 variant",
			input:        "966501234567",
			wantContains: []string{"966501234567", "0501234567"},
			wantAbsent:   []string{"+966501234567"}, // raw == digits, not appended
		},
		{
			name:         "formatted with spaces → digits + 0 variant + trimmed raw",
			input:        "+966 53 888 6816",
			wantContains: []string{"966538886816", "0538886816", "+966 53 888 6816"},
		},
		{
			name:         "short 0-prefix (< 9 digits) → no 966 conversion",
			input:        "0512345",
			wantContains: []string{"0512345"},
			wantAbsent:   []string{"966512345"},
		},
		{
			name:         "966 prefix but too short for 0 conversion (< 12 digits)",
			input:        "9665",
			wantContains: []string{"9665"},
			wantAbsent:   []string{"05"},
		},
		{
			name:         "digits only no special prefix → just digits",
			input:        "1234567890",
			wantContains: []string{"1234567890"},
		},
		{
			name:         "mixed formatting dashes",
			input:        "966-50-123-4567",
			wantContains: []string{"966501234567", "0501234567"},
		},
		{
			name:         "05 prefix exactly 9 digits (boundary) → 966 variant produced",
			input:        "012345678",
			wantContains: []string{"012345678", "966" + "12345678"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := phoneNormVariants(tc.input)

			if tc.wantNil {
				if got != nil {
					t.Errorf("want nil, got %v", got)
				}
				return
			}

			for _, want := range tc.wantContains {
				found := false
				for _, v := range got {
					if v == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("want %q in variants, got %v", want, got)
				}
			}

			for _, absent := range tc.wantAbsent {
				for _, v := range got {
					if v == absent {
						t.Errorf("want %q absent from variants, but found it in %v", absent, got)
					}
				}
			}
		})
	}
}

// TestPhoneNormVariants_NoDuplicateDigits verifies that the digits-only form
// is always the first element when there are digits.
func TestPhoneNormVariants_DigitsFirst(t *testing.T) {
	cases := []struct {
		input      string
		wantFirst  string
	}{
		{"0501234567", "0501234567"},
		{"+966501234567", "966501234567"},
		{"966501234567", "966501234567"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.input, func(t *testing.T) {
			got := phoneNormVariants(c.input)
			if len(got) == 0 {
				t.Fatal("expected at least one variant")
			}
			if got[0] != c.wantFirst {
				t.Errorf("want first variant %q, got %q", c.wantFirst, got[0])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration stubs
// ---------------------------------------------------------------------------

// TestFindCustomerByNameExact_Integration documents that findCustomerByNameExact
// requires a live MongoDB connection to search by name.
func TestFindCustomerByNameExact_Integration(t *testing.T) {
	t.Skip("requires live MongoDB — findCustomerByNameExact calls store.FindCustomerByName which needs a real DB connection")
}

// TestFindOrCreateCustomerHandler_Integration_Success documents the successful
// find-or-create flow that requires a live store and MongoDB.
func TestFindOrCreateCustomerHandler_Integration_Success(t *testing.T) {
	t.Skip("requires live MongoDB with a valid auth token and a seeded store")
}
