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

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/erp"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
	"github.com/sirinibin/startpos/backend/models"
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

// TestERP_Integration_MetaAndLogin mounts the adapter on a plain mux router
// exactly as main.go does (erp.Register(router)) and, against the seeded
// fixture DB, checks GET /v1/erp/meta and POST /v1/erp/auth/login for the
// fixture admin; the issued access token must be accepted by /auth/me and by
// the legacy v1 auth. The full adapter suite is ERP_TEST_DB=1 go test ./erp/.
func TestERP_Integration_MetaAndLogin(t *testing.T) {
	fx := controller.RequireDBExt(t)
	router := mux.NewRouter()
	erp.Register(router)
	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, erp.Prefix+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	if rec := do("GET", "/meta", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("meta: %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]string{"email": fx.AdminEmail, "password": erpfixture.Password})
	rec := do("POST", "/auth/login", "", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	var login struct {
		AccessToken  string                 `json:"accessToken"`
		RefreshToken string                 `json:"refreshToken"`
		User         map[string]interface{} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil || login.AccessToken == "" || login.RefreshToken == "" {
		t.Fatalf("login body: %s", rec.Body.String())
	}
	if login.User["id"] != fx.Admin.Hex() {
		t.Errorf("login user id = %v, want %s", login.User["id"], fx.Admin.Hex())
	}

	if me := do("GET", "/auth/me", login.AccessToken, ""); me.Code != http.StatusOK {
		t.Fatalf("me with login token: %d %s", me.Code, me.Body.String())
	}
	req := httptest.NewRequest("GET", "/v1/store", nil)
	req.Header.Set("Authorization", login.AccessToken)
	if claims, err := models.AuthenticateByAccessToken(req); err != nil || claims.UserID != fx.Admin.Hex() {
		t.Fatalf("legacy auth rejects the adapter token: %+v %v", claims, err)
	}

	// wrong password is refused
	bad, _ := json.Marshal(map[string]string{"email": fx.AdminEmail, "password": "wrong"})
	if rec := do("POST", "/auth/login", "", string(bad)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d %s", rec.Code, rec.Body.String())
	}
}
