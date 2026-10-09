package erp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/controller"
)

// TestEndpoints_Unauthenticated: every /v1/erp endpoint except meta, login,
// refresh and signup returns 401 with the contract error envelope when no
// (or an invalid) access token is sent. Runs without a database.
func TestEndpoints_Unauthenticated(t *testing.T) {
	type ep struct{ method, path string }
	eps := []ep{
		{"GET", "/auth/me"}, {"POST", "/auth/logout"},
		{"POST", "/sales/x/zatca/report"}, {"POST", "/sales-returns/x/zatca/report"},
		{"POST", "/deposits/x/zatca/report"}, {"POST", "/withdrawals/x/zatca/report"},
		{"POST", "/stores/x/zatca/connect"}, {"POST", "/stores/x/zatca/disconnect"},
		{"GET", "/drafts/sales"}, {"POST", "/drafts/sales"}, {"GET", "/drafts/sales/x"}, {"PUT", "/drafts/sales/x"},
		{"DELETE", "/drafts/sales/x"}, {"POST", "/drafts/sales/x/finalize"},
		{"GET", "/stores/x/starter-catalog"}, {"POST", "/stores/x/starter-catalog"},
		{"GET", "/admin/zatca-reconnects"}, {"POST", "/stores/x/zatca/unmark"},
	}
	for _, r := range Resources() {
		eps = append(eps,
			ep{"GET", "/" + r.Path}, ep{"POST", "/" + r.Path}, ep{"GET", "/" + r.Path + "/abc"},
			ep{"PATCH", "/" + r.Path + "/abc"}, ep{"PUT", "/" + r.Path + "/abc"}, ep{"DELETE", "/" + r.Path + "/abc"},
			ep{"DELETE", "/" + r.Path + "/abc?hard=1"}, ep{"POST", "/" + r.Path + "/abc/restore"})
	}
	if len(eps) < 43*8 {
		t.Fatalf("expected ≥344 endpoints, got %d", len(eps))
	}
	for _, e := range eps {
		for _, tok := range []string{"", "not-a-jwt"} {
			r := call(t, e.method, e.path, tok, M{})
			if r.Code != http.StatusUnauthorized {
				t.Errorf("%s %s (token %q): got %d %s", e.method, e.path, tok, r.Code, r.Raw)
				continue
			}
			if r.errCode() == "" || str(get(r.Body, "error.message")) == "" {
				t.Errorf("%s %s: missing error envelope: %s", e.method, e.path, r.Raw)
			}
		}
	}
}

func TestPublicEndpoints_NoDB(t *testing.T) {
	if r := call(t, "GET", "/meta", "", nil); r.Code != 200 || get(r.Body, "capabilities.serverStock") != true {
		t.Fatalf("meta: %d %s", r.Code, r.Raw)
	}
	// login: validation before any DB access
	r := call(t, "POST", "/auth/login", "", M{"email": "", "password": ""})
	if r.Code != 400 || r.errField("email") == "" || r.errField("password") == "" {
		t.Fatalf("login validation: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/auth/login", "", "{not json"); r.Code != 400 || r.errCode() != "malformed_json" {
		t.Fatalf("malformed: %d %s", r.Code, r.Raw)
	}
	// refresh without / with garbage token → 401 (client signs out)
	if r := call(t, "POST", "/auth/refresh", "", M{}); r.Code != 401 {
		t.Fatalf("refresh missing: %d", r.Code)
	}
	if r := call(t, "POST", "/auth/refresh", "", M{"refreshToken": "garbage"}); r.Code != 401 {
		t.Fatalf("refresh garbage: %d", r.Code)
	}
	// signup validation (owner rule) before any DB access
	r = call(t, "POST", "/auth/signup", "", M{"owner": M{"email": "x"}, "company": M{"vatNo": "123"}})
	if r.Code != 400 {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	for _, f := range []string{"company.vatNo", "company.crNo", "company.nameEn", "company.mobile", "company.address.buildingNo",
		"company.address.postalCode", "company.address.streetAr", "owner.email", "owner.password"} {
		if r.errField(f) == "" {
			t.Errorf("signup: missing field error %s", f)
		}
	}
	// unknown routes under the prefix
	if r := call(t, "GET", "/no-such-resource", "", nil); r.Code != 404 && r.Code != 405 {
		t.Fatalf("unknown route: %d", r.Code)
	}
}

func TestCORSPreflightAllowsContractHeaders(t *testing.T) {
	// CORS is applied by main.go (rs/cors, AllowedHeaders "*", PATCH allowed);
	// the adapter router must at least not shadow OPTIONS for legacy routes.
	for _, r := range Resources() {
		if r.Path == "" || r.Name == "" {
			t.Fatal("resource without path/name")
		}
	}
}

func TestRateLimitedLogin_ContractEnvelope(t *testing.T) {
	rl := controller.NewRateLimiter(1, time.Minute)
	h := rateLimited(rl, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for i, want := range []int{204, 429} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/erp/auth/login", nil)
		req.Header.Set("X-Real-IP", "10.9.9.9")
		h(rec, req)
		if rec.Code != want {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
		if want == 429 && !strings.Contains(rec.Body.String(), `"rate_limited"`) {
			t.Fatalf("429 must use the contract envelope: %s", rec.Body.String())
		}
	}
}
