//go:build e2e

// Package apie2e drives a running API server over real HTTP, the way the
// React app and other clients do. It needs a server started against a
// disposable MongoDB/Redis and a user created by ./e2e/seed:
//
//	MONGO_DB=pos_e2e go run ./e2e/seed
//	MONGO_DB=pos_e2e API_PORT=2000 ./pos-rest &
//	go test -tags e2e ./e2e/api/ -count=1 -v
//
// Environment: E2E_API_URL (default http://localhost:2000), E2E_EMAIL and
// E2E_PASSWORD (defaults match e2e/seed).
package apie2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type apiResponse struct {
	Status     bool              `json:"status"`
	TotalCount int64             `json:"total_count"`
	Result     json.RawMessage   `json:"result"`
	Errors     map[string]string `json:"errors"`
}

var (
	client  = &http.Client{Timeout: 30 * time.Second}
	runID   = fmt.Sprintf("%d", time.Now().UnixNano())
	ipSeq   = uint32(time.Now().UnixNano()) // random start: the server remembers addresses for 15 minutes across runs
	setupMu sync.Mutex
	token   string
	storeID string
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func baseURL() string  { return strings.TrimRight(getenv("E2E_API_URL", "http://localhost:2000"), "/") }
func email() string    { return getenv("E2E_EMAIL", "e2e-admin@startpos.test") }
func password() string { return getenv("E2E_PASSWORD", "E2e-Passw0rd!") }

// call sends a request and decodes the standard {status,result,errors} envelope.
func call(t *testing.T, method, path, auth string, body interface{}, headers ...map[string]string) (int, apiResponse) {
	t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		reader = bytes.NewReader(b)
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL()+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for _, h := range headers {
		for k, v := range h {
			req.Header.Set(k, v)
		}
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out apiResponse
	if len(raw) > 0 && strings.Contains(res.Header.Get("Content-Type"), "json") {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, truncate(raw), err)
		}
	}
	return res.StatusCode, out
}

func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300]) + "…"
	}
	return string(b)
}

func resultMap(t *testing.T, r apiResponse) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(r.Result, &m); err != nil {
		t.Fatalf("result is not an object: %s", truncate(r.Result))
	}
	return m
}

func resultList(t *testing.T, r apiResponse) []map[string]interface{} {
	t.Helper()
	var l []map[string]interface{}
	if len(r.Result) == 0 || string(r.Result) == "null" {
		return l
	}
	if err := json.Unmarshal(r.Result, &l); err != nil {
		t.Fatalf("result is not a list: %s", truncate(r.Result))
	}
	return l
}

// freshIP returns a client address no other request in this run has used.
// /v1/authorize is rate limited per client IP (taken from X-Real-IP, which
// nginx sets in production), so each login-related check gets its own
// budget and the checks cannot throttle one another.
func freshIP() map[string]string {
	n := atomic.AddUint32(&ipSeq, 1)
	return map[string]string{"X-Real-IP": fmt.Sprintf("10.%d.%d.%d", (n>>16)&0xff, (n>>8)&0xff, n&0xff)}
}

// login runs the same two-step flow as the React login page.
func login(t *testing.T, mail, pass string) (int, apiResponse, string) {
	t.Helper()
	code, res := call(t, "POST", "/v1/authorize", "", map[string]string{"email": mail, "password": pass}, freshIP())
	if !res.Status {
		return code, res, ""
	}
	authCode, _ := resultMap(t, res)["code"].(string)
	if authCode == "" {
		t.Fatalf("authorize returned no code: %s", truncate(res.Result))
	}
	code, res = call(t, "POST", "/v1/accesstoken", authCode, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("accesstoken: HTTP %d %v", code, res.Errors)
	}
	tok, _ := resultMap(t, res)["access_token"].(string)
	if tok == "" {
		t.Fatalf("accesstoken returned no access_token")
	}
	return code, res, tok
}

func authToken(t *testing.T) string {
	t.Helper()
	setupMu.Lock()
	defer setupMu.Unlock()
	if token == "" {
		code, res, tok := login(t, email(), password())
		if tok == "" {
			t.Fatalf("login as %s failed: HTTP %d %v (was the database seeded with ./e2e/seed?)", email(), code, res.Errors)
		}
		token = tok
	}
	return token
}

// testStore creates (once per run) a store from fixtures/store.json with a
// run-unique code so repeated runs against one database do not collide.
func testStore(t *testing.T) string {
	t.Helper()
	tok := authToken(t)
	setupMu.Lock()
	defer setupMu.Unlock()
	if storeID != "" {
		return storeID
	}
	raw, err := os.ReadFile("../fixtures/store.json")
	if err != nil {
		t.Fatalf("read store fixture: %v", err)
	}
	var store map[string]interface{}
	if err := json.Unmarshal(raw, &store); err != nil {
		t.Fatalf("parse store fixture: %v", err)
	}
	store["code"] = "E2E" + runID[len(runID)-6:]
	store["name"] = "E2E Store " + runID
	code, res := call(t, "POST", "/v1/store", tok, store)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("create store: HTTP %d %v", code, res.Errors)
	}
	storeID, _ = resultMap(t, res)["id"].(string)
	if storeID == "" {
		t.Fatalf("create store returned no id")
	}
	return storeID
}

func storeQuery(id string) string { return "search[store_id]=" + url.QueryEscape(id) }

func TestMain(m *testing.M) {
	// Wait for the server so the suite does not race a slow startup.
	deadline := time.Now().Add(60 * time.Second)
	for {
		res, err := http.Get(baseURL() + "/v1/me")
		if err == nil {
			res.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(os.Stderr, "API at %s did not come up: %v\n", baseURL(), err)
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
	os.Exit(m.Run())
}

// ── Health ───────────────────────────────────────────────────────────────────

func TestHealth_ReportsMongoAndRedisUp(t *testing.T) {
	res, err := client.Get(baseURL() + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	defer res.Body.Close()
	var h struct {
		OK      bool                    `json:"ok"`
		Redis   struct{ Status string } `json:"redis"`
		MongoDB struct{ Status string } `json:"mongodb"`
	}
	if err := json.NewDecoder(res.Body).Decode(&h); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if res.StatusCode != http.StatusOK || !h.OK || h.Redis.Status != "ok" || h.MongoDB.Status != "ok" {
		t.Fatalf("health: HTTP %d %+v", res.StatusCode, h)
	}
}

// ── Authentication ───────────────────────────────────────────────────────────

func TestAuth_ProtectedEndpointsRejectMissingOrBadTokens(t *testing.T) {
	paths := []string{"/v1/me", "/v1/store", "/v1/customer", "/v1/product", "/v1/order", "/v1/quotation"}
	tokens := map[string]string{
		"missing":   "",
		"garbage":   "not-a-jwt",
		"malformed": "eyJhbGciOiJIUzI1NiJ9.e30.invalid",
	}
	for _, p := range paths {
		for name, tok := range tokens {
			t.Run(p+"/"+name, func(t *testing.T) {
				code, res := call(t, "GET", p, tok, nil)
				if code != http.StatusUnauthorized {
					t.Fatalf("want 401, got %d", code)
				}
				if res.Status {
					t.Fatalf("status should be false for an unauthenticated call")
				}
			})
		}
	}
}

func TestAuth_AuthorizeValidation(t *testing.T) {
	cases := []struct {
		name, email, password string
		wantErrKey            string
	}{
		{"empty email and password", "", "", "email"},
		{"empty password", email(), "", "password"},
		{"empty email", "", "x", "email"},
		{"wrong password", email(), "definitely-wrong", "password"},
		{"unknown user", "nobody-" + runID + "@startpos.test", "whatever", "password"},
		{"whitespace password", email(), "   ", "password"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, res := call(t, "POST", "/v1/authorize", "", map[string]string{"email": c.email, "password": c.password}, freshIP())
			if res.Status {
				t.Fatalf("authorize should fail")
			}
			if code < 400 {
				t.Fatalf("want 4xx, got %d", code)
			}
			if _, ok := res.Errors[c.wantErrKey]; !ok {
				t.Fatalf("want error on %q, got %v", c.wantErrKey, res.Errors)
			}
		})
	}
}

func TestAuth_MalformedJSONBodyIsRejected(t *testing.T) {
	code, res := call(t, "POST", "/v1/authorize", "", "{not json", freshIP())
	if res.Status || code < 400 {
		t.Fatalf("malformed body should be rejected, got HTTP %d status=%v", code, res.Status)
	}
}

func TestAuth_AccessTokenRejectsInvalidAuthCode(t *testing.T) {
	for _, c := range []string{"", "bogus", "eyJhbGciOiJIUzI1NiJ9.e30.invalid"} {
		code, res := call(t, "POST", "/v1/accesstoken", c, nil)
		if code != http.StatusUnauthorized || res.Status {
			t.Fatalf("auth code %q: want 401/false, got %d/%v", c, code, res.Status)
		}
	}
}

func TestAuth_LoginIsRateLimitedPerClient(t *testing.T) {
	ip := freshIP()
	body := map[string]string{"email": email(), "password": "wrong-" + runID}
	limited := 0
	for i := 0; i < 12; i++ {
		code, res := call(t, "POST", "/v1/authorize", "", body, ip)
		if code == http.StatusTooManyRequests {
			limited++
			if _, ok := res.Errors["rate_limit"]; !ok {
				t.Fatalf("429 without a rate_limit error: %v", res.Errors)
			}
			if i < 10 {
				t.Fatalf("throttled after only %d attempts; the budget is 10", i)
			}
		}
	}
	if limited == 0 {
		t.Fatalf("12 failed logins from one client were never throttled")
	}
	// The correct password from the throttled client is refused too…
	if code, _ := call(t, "POST", "/v1/authorize", "", map[string]string{"email": email(), "password": password()}, ip); code != http.StatusTooManyRequests {
		t.Fatalf("throttled client should get 429 even with the right password, got %d", code)
	}
	// …while another client is unaffected.
	if _, _, tok := login(t, email(), password()); tok == "" {
		t.Fatalf("a different client was throttled too")
	}
}

func TestAuth_LoginAndMe(t *testing.T) {
	_, _, tok := login(t, email(), password())
	code, res := call(t, "GET", "/v1/me", tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("/v1/me: HTTP %d %v", code, res.Errors)
	}
	me := resultMap(t, res)
	if got := me["email"]; got != strings.ToLower(email()) {
		t.Fatalf("/v1/me email = %v, want %s", got, email())
	}
	if me["password"] != nil && me["password"] != "" {
		t.Fatalf("/v1/me must not expose the password hash")
	}
}

// ── Store ────────────────────────────────────────────────────────────────────

func TestStore_CreateValidation(t *testing.T) {
	tok := authToken(t)
	code, res := call(t, "POST", "/v1/store", tok, map[string]interface{}{})
	if res.Status || code < 400 {
		t.Fatalf("empty store must be rejected, got HTTP %d", code)
	}
	for _, key := range []string{"name", "code", "vat_no", "email", "phone", "national_address_building_no"} {
		if _, ok := res.Errors[key]; !ok {
			t.Errorf("missing validation error for %q (got %v)", key, res.Errors)
		}
	}

	raw, _ := os.ReadFile("../fixtures/store.json")
	bad := []struct {
		name, field string
		value       interface{}
		errKey      string
	}{
		{"vat not 15 digits", "vat_no", "3000", "vat_no"},
		{"vat not starting/ending with 3", "vat_no", "100000000000001", "vat_no"},
		{"non alphanumeric CRN", "registration_number", "10-10", "registration_number"},
	}
	for _, b := range bad {
		t.Run(b.name, func(t *testing.T) {
			var store map[string]interface{}
			_ = json.Unmarshal(raw, &store)
			store["code"] = "BAD" + runID[len(runID)-5:]
			store[b.field] = b.value
			code, res := call(t, "POST", "/v1/store", tok, store)
			if res.Status || code < 400 {
				t.Fatalf("want rejection, got HTTP %d", code)
			}
			if _, ok := res.Errors[b.errKey]; !ok {
				t.Fatalf("want error on %q, got %v", b.errKey, res.Errors)
			}
		})
	}
}

func TestStore_CreateThenRead(t *testing.T) {
	tok := authToken(t)
	id := testStore(t)

	code, res := call(t, "GET", "/v1/store/"+id, tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("get store: HTTP %d %v", code, res.Errors)
	}
	if got := resultMap(t, res)["id"]; got != id {
		t.Fatalf("store id = %v, want %s", got, id)
	}

	code, res = call(t, "GET", "/v1/store?limit=100", tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("list stores: HTTP %d %v", code, res.Errors)
	}
	found := false
	for _, s := range resultList(t, res) {
		if s["id"] == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("new store %s not in store list", id)
	}
}

func TestStore_UnknownAndInvalidID(t *testing.T) {
	tok := authToken(t)
	for _, id := range []string{"000000000000000000000000", "not-an-object-id"} {
		code, res := call(t, "GET", "/v1/store/"+id, tok, nil)
		if res.Status || len(res.Errors) == 0 {
			t.Fatalf("store %q: want an error, got HTTP %d status=%v", id, code, res.Status)
		}
	}
}

// ── Customer ─────────────────────────────────────────────────────────────────

func TestCustomer_CreateValidateSearch(t *testing.T) {
	tok := authToken(t)
	sid := testStore(t)

	code, res := call(t, "POST", "/v1/customer", tok, map[string]interface{}{"store_id": sid})
	if res.Status || code < 400 {
		t.Fatalf("customer without a name must be rejected, got HTTP %d", code)
	}
	if _, ok := res.Errors["name"]; !ok {
		t.Fatalf("want name error, got %v", res.Errors)
	}

	name := "E2E Customer " + runID
	code, res = call(t, "POST", "/v1/customer", tok, map[string]interface{}{"store_id": sid, "name": name})
	if code != http.StatusOK || !res.Status {
		t.Fatalf("create customer: HTTP %d %v", code, res.Errors)
	}
	c := resultMap(t, res)
	id, _ := c["id"].(string)
	if id == "" {
		t.Fatalf("customer has no id")
	}
	if code, _ := c["code"].(string); code == "" {
		t.Fatalf("customer should get a generated code from the store's serial settings")
	}

	code, res = call(t, "GET", "/v1/customer/"+id+"?"+storeQuery(sid), tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("get customer: HTTP %d %v", code, res.Errors)
	}
	if got := resultMap(t, res)["name"]; !strings.EqualFold(fmt.Sprint(got), name) {
		t.Fatalf("customer name = %v, want %s", got, name)
	}

	code, res = call(t, "GET", "/v1/customer?"+storeQuery(sid)+"&limit=100", tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("list customers: HTTP %d %v", code, res.Errors)
	}
	found := false
	for _, row := range resultList(t, res) {
		if row["id"] == id {
			found = true
		}
	}
	if !found {
		t.Fatalf("customer %s missing from its store's list", id)
	}
}

// ── Product ──────────────────────────────────────────────────────────────────

func TestProduct_CreateValidateSearch(t *testing.T) {
	tok := authToken(t)
	sid := testStore(t)

	code, res := call(t, "POST", "/v1/product", tok, map[string]interface{}{"store_id": sid, "name": "x"})
	if code == http.StatusOK && res.Status {
		t.Fatalf("product without search[store_id] should be rejected")
	}
	if _, ok := res.Errors["store_id"]; !ok {
		t.Fatalf("want store_id error, got %v", res.Errors)
	}

	code, res = call(t, "POST", "/v1/product?"+storeQuery(sid), tok, map[string]interface{}{"store_id": sid})
	if code == http.StatusOK && len(res.Errors) == 0 {
		t.Fatalf("product without a name should be rejected")
	}
	if _, ok := res.Errors["name"]; !ok {
		t.Fatalf("want name error, got %v", res.Errors)
	}

	word := "Gizmo" + runID[len(runID)-6:]
	part := "E2E-" + runID[len(runID)-6:]
	code, res = call(t, "POST", "/v1/product?"+storeQuery(sid), tok, map[string]interface{}{
		"store_id": sid, "name": "E2E " + word, "part_number": part, "unit": "PC",
	})
	if code != http.StatusOK || len(res.Errors) > 0 {
		t.Fatalf("create product: HTTP %d %v", code, res.Errors)
	}
	id, _ := resultMap(t, res)["id"].(string)
	if id == "" {
		t.Fatalf("product has no id")
	}

	code, res = call(t, "GET", "/v1/product/"+id+"?"+storeQuery(sid), tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("get product: HTTP %d %v", code, res.Errors)
	}
	if got := resultMap(t, res)["part_number"]; got != part {
		t.Fatalf("part_number = %v, want %s", got, part)
	}

	code, res = call(t, "GET", "/v1/product?"+storeQuery(sid)+"&search[search_text]="+url.QueryEscape(word), tok, nil)
	if code != http.StatusOK || !res.Status {
		t.Fatalf("search products: HTTP %d %v", code, res.Errors)
	}
	rows := resultList(t, res)
	if len(rows) != 1 || rows[0]["id"] != id {
		t.Fatalf("search for %q: want exactly the new product, got %d rows", word, len(rows))
	}
}
