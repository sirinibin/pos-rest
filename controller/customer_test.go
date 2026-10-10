package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// TestFindCustomerByNameExact_Integration checks findCustomerByNameExact
// against the seeded store: exact, case-sensitive, store-scoped matching that
// ignores soft-deleted customers.
func TestFindCustomerByNameExact_Integration(t *testing.T) {
	fx := requireDB(t)
	storeA, err := models.FindStoreByID(&fx.StoreA, bson.M{})
	if err != nil {
		t.Fatalf("load store A: %v", err)
	}
	storeB, err := models.FindStoreByID(&fx.StoreB, bson.M{})
	if err != nil {
		t.Fatalf("load store B: %v", err)
	}

	if c := findCustomerByNameExact(storeA, "Riyadh Motors"); c == nil || c.ID != fx.CustomerA1 {
		t.Fatalf("exact name: want CustomerA1 %s, got %+v", fx.CustomerA1.Hex(), c)
	}
	// legacy-shaped customer (no "deleted" key at all) is still found
	if c := findCustomerByNameExact(storeA, "Walk In Old"); c == nil || c.ID != fx.CustomerA2 {
		t.Fatalf("legacy customer without deleted flag: want CustomerA2, got %+v", c)
	}
	for _, name := range []string{"", "riyadh motors", "Riyadh", "Riyadh Motors "} {
		if c := findCustomerByNameExact(storeA, name); c != nil {
			t.Errorf("name %q must not match (exact match only), got %s", name, c.Name)
		}
	}
	// store scoping: store B has no "Riyadh Motors"
	if c := findCustomerByNameExact(storeB, "Riyadh Motors"); c != nil {
		t.Errorf("store B must not see store A's customer, got %s", c.ID.Hex())
	}

	// soft-deleted customers are ignored, live ones with the same name are found
	name := uniqName("Exact Name Co")
	insertStoreDoc(t, fx.StoreA, "customer", bson.M{"name": name, "store_id": fx.StoreA, "deleted": true})
	if c := findCustomerByNameExact(storeA, name); c != nil {
		t.Fatalf("deleted customer must not be returned, got %s", c.ID.Hex())
	}
	liveID := insertStoreDoc(t, fx.StoreA, "customer", bson.M{"name": name, "store_id": fx.StoreA, "deleted": false})
	if c := findCustomerByNameExact(storeA, name); c == nil || c.ID != liveID {
		t.Fatalf("live customer: want %s, got %+v", liveID.Hex(), c)
	}
}

// TestFindOrCreateCustomerHandler_Integration_Success drives
// POST /v1/customer/find-or-create: auth + store checks, phone-normalised
// lookup of an existing customer, creation of a new one (persisted with a
// serial code and digits-only phone) and idempotency of a repeat call.
func TestFindOrCreateCustomerHandler_Integration_Success(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	url := "/v1/customer/find-or-create?store_id=" + fx.StoreA.Hex()

	if r := callHandler(t, FindOrCreateCustomerHandler, "POST", url, "", map[string]string{"name": "x"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", r.Code)
	}
	if r := callHandler(t, FindOrCreateCustomerHandler, "POST", "/v1/customer/find-or-create?store_id=bad", tok, map[string]string{"name": "x"}); r.Code != http.StatusBadRequest {
		t.Fatalf("bad store_id: want 400, got %d", r.Code)
	}
	if r := callHandler(t, FindOrCreateCustomerHandler, "POST", "/v1/customer/find-or-create?store_id="+primitive.NewObjectID().Hex(), tok, map[string]string{"name": "x"}); r.Code != http.StatusNotFound {
		t.Fatalf("unknown store: want 404, got %d", r.Code)
	}
	if r := callHandler(t, FindOrCreateCustomerHandler, "POST", url, tok, "{not json"); r.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON: want 400, got %d", r.Code)
	}

	created := func(r apiResp) bool {
		var m struct {
			Created bool `json:"created"`
		}
		_ = json.Unmarshal([]byte(r.Raw), &m)
		return m.Created
	}

	// existing customer: CustomerA1 is stored as "0512345678"; an
	// international, space-formatted number must resolve to it.
	r := callHandler(t, FindOrCreateCustomerHandler, "POST", url, tok, map[string]string{"name": "Someone Else", "phone": "+966 51 234 5678"})
	if r.Code != http.StatusOK || created(r) {
		t.Fatalf("existing by phone: want 200 created=false, got %d %s", r.Code, r.Raw)
	}
	if id, _ := r.resultMap(t)["id"].(string); id != fx.CustomerA1.Hex() {
		t.Fatalf("existing by phone: want %s, got %s", fx.CustomerA1.Hex(), id)
	}
	// existing customer by exact name (no phone/email given)
	r = callHandler(t, FindOrCreateCustomerHandler, "POST", url, tok, map[string]string{"name": "Riyadh Motors"})
	if id, _ := r.resultMap(t)["id"].(string); r.Code != http.StatusOK || created(r) || id != fx.CustomerA1.Hex() {
		t.Fatalf("existing by name: got %d %s", r.Code, r.Raw)
	}

	// new customer
	n := time.Now().UnixNano()
	local := fmt.Sprintf("05%08d", n%100000000)
	name := uniqName("FindOrCreate Co")
	email := fmt.Sprintf("foc+%d@t1.example", n)
	formatted := "+966 " + local[1:3] + " " + local[3:6] + " " + local[6:]
	r = callHandler(t, FindOrCreateCustomerHandler, "POST", url, tok, map[string]string{
		"name": name, "phone": formatted, "email": email, "city_name": "Dammam", "contact_person": "Omar",
	})
	if r.Code != http.StatusOK || !created(r) {
		t.Fatalf("create: want 200 created=true, got %d %s", r.Code, r.Raw)
	}
	res := r.resultMap(t)
	newID, err := primitive.ObjectIDFromHex(fmt.Sprint(res["id"]))
	if err != nil {
		t.Fatalf("create: bad id in %s", r.Raw)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = db.GetDB("store_"+fx.StoreA.Hex()).Collection("customer").DeleteOne(ctx, bson.M{"_id": newID})
	})

	storeA, err := models.FindStoreByID(&fx.StoreA, bson.M{})
	if err != nil {
		t.Fatalf("load store A: %v", err)
	}
	saved, err := storeA.FindCustomerByID(&newID, bson.M{})
	if err != nil {
		t.Fatalf("created customer not in DB: %v", err)
	}
	wantPhone := "966" + local[1:] // digits-only form of the submitted number
	if saved.Name != name || saved.Email != email || saved.Phone != wantPhone || saved.ContactPerson != "Omar" ||
		saved.NationalAddress.CityName != "Dammam" || saved.StoreID == nil || *saved.StoreID != fx.StoreA {
		t.Fatalf("saved customer mismatch: name=%q email=%q phone=%q contact=%q city=%q store=%v",
			saved.Name, saved.Email, saved.Phone, saved.ContactPerson, saved.NationalAddress.CityName, saved.StoreID)
	}
	if !strings.HasPrefix(saved.Code, "CUST") {
		t.Errorf("new customer code %q should use the store's CUST serial", saved.Code)
	}

	// repeat with the local format of the same number: found, not duplicated
	r = callHandler(t, FindOrCreateCustomerHandler, "POST", url, tok, map[string]string{"name": "Other", "phone": local})
	if id, _ := r.resultMap(t)["id"].(string); r.Code != http.StatusOK || created(r) || id != newID.Hex() {
		t.Fatalf("repeat by local phone: want existing %s, got %d %s", newID.Hex(), r.Code, r.Raw)
	}
	// and by email only
	r = callHandler(t, FindOrCreateCustomerHandler, "POST", url, tok, map[string]string{"email": email})
	if id, _ := r.resultMap(t)["id"].(string); r.Code != http.StatusOK || created(r) || id != newID.Hex() {
		t.Fatalf("repeat by email: want existing %s, got %d %s", newID.Hex(), r.Code, r.Raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cnt, err := db.GetDB("store_"+fx.StoreA.Hex()).Collection("customer").CountDocuments(ctx, bson.M{"name": name})
	if err != nil || cnt != 1 {
		t.Fatalf("want exactly 1 customer named %q, got %d (%v)", name, cnt, err)
	}
}
