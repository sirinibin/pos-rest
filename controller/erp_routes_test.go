package controller_test

// API Test Sync Rule coverage for the StartERP adapter (/v1/erp/...).
// The adapter lives in package erp (it imports controller), so these tests
// use the external test package to avoid an import cycle. The full adapter
// suite (mappers, API, old-data integration) is in erp/*_test.go; its
// DB-backed part is opt-in: ERP_TEST_DB=1 go test ./erp/ -count=1.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/erp"
)

func erpCall(method, path, token, body string) *httptest.ResponseRecorder {
	router := erp.NewRouter()
	req := httptest.NewRequest(method, erp.Prefix+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestERP_Endpoints_Unauthenticated: every protected adapter route answers
// 401 with the contract error envelope when no token is sent.
func TestERP_Endpoints_Unauthenticated(t *testing.T) {
	paths := []struct{ method, path string }{
		{"GET", "/auth/me"}, {"POST", "/auth/logout"},
		{"POST", "/sales/x/zatca/report"}, {"POST", "/stores/x/zatca/connect"}, {"POST", "/stores/x/zatca/disconnect"},
		{"GET", "/drafts/sales"}, {"POST", "/drafts/sales/x/finalize"},
	}
	for _, r := range erp.Resources() {
		paths = append(paths,
			struct{ method, path string }{"GET", "/" + r.Path},
			struct{ method, path string }{"GET", "/" + r.Path + "?select=-history"},
			struct{ method, path string }{"GET", "/" + r.Path + "/x?select=code"},
			struct{ method, path string }{"POST", "/" + r.Path},
			struct{ method, path string }{"PATCH", "/" + r.Path + "/x"},
			struct{ method, path string }{"DELETE", "/" + r.Path + "/x"},
			struct{ method, path string }{"POST", "/" + r.Path + "/x/restore"})
	}
	for _, p := range paths {
		rec := erpCall(p.method, p.path, "", "{}")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: got %d", p.method, p.path, rec.Code)
			continue
		}
		var body map[string]map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"]["code"] == nil {
			t.Errorf("%s %s: no contract error envelope: %s", p.method, p.path, rec.Body.String())
		}
	}
}

// TestERP_Meta_Public: the capability document needs no auth.
func TestERP_Meta_Public(t *testing.T) {
	rec := erpCall("GET", "/meta", "", "")
	if rec.Code != 200 {
		t.Fatalf("meta: %d", rec.Code)
	}
	var m struct {
		Capabilities map[string]interface{} `json:"capabilities"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	for _, k := range []string{"serverStock", "clientIds", "serverNumbers"} {
		if m.Capabilities[k] != true {
			t.Errorf("capabilities.%s must be true: %s", k, rec.Body.String())
		}
	}
}

// TestERP_Login_Validation: table-driven pure validation (no DB needed).
func TestERP_Login_Validation(t *testing.T) {
	cases := []struct {
		name, body string
		code       int
	}{
		{"empty", `{"email":"","password":""}`, 400},
		{"malformed", `{not json`, 400},
	}
	for _, c := range cases {
		if rec := erpCall("POST", "/auth/login", "", c.body); rec.Code != c.code {
			t.Errorf("%s: got %d %s", c.name, rec.Code, rec.Body.String())
		}
	}
}

// TestERP_Signup_ValidationRules: owner rules (VAT 15 digits 3…3, CR 10 digits).
func TestERP_Signup_ValidationRules(t *testing.T) {
	if !erp.ValidVAT("310122393500003") || erp.ValidVAT("123") || erp.ValidVAT("210122393500003") {
		t.Error("VAT rule: 15 digits, starts and ends with 3")
	}
	if !erp.ValidCR("1010101010") || erp.ValidCR("10101") {
		t.Error("CR rule: 10 digits")
	}
	rec := erpCall("POST", "/auth/signup", "", `{"company":{"vatNo":"1"},"owner":{}}`)
	if rec.Code != 400 {
		t.Fatalf("signup validation: %d", rec.Code)
	}
}

// TestERP_Integration_Stub: the DB-backed adapter integration suite lives in
// erp/integration_test.go (old-data compatibility, counters/stock/ledger
// parity with the old app, drafts regression).
func TestERP_Integration_Stub(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Skip("run: ERP_TEST_DB=1 MONGO_HOST=… MONGO_PORT=… REDIS_DSN=… go test ./erp/ -count=1")
}
