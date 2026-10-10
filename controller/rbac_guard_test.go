package controller

import (
	"net/http"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

func TestRBACTarget(t *testing.T) {
	cases := []struct {
		method, tpl, resource, action string
	}{
		{"POST", "/v1/product", "products", "create"},
		{"PUT", "/v1/product/{id}", "products", "update"},
		{"PATCH", "/v1/vendor/{id}", "vendors", "update"},
		{"DELETE", "/v1/customer/{id}", "customers", "delete"},
		{"POST", "/v1/order", "sales", "create"},
		{"POST", "/v1/capital-withdrawal", "capitals", "create"},
		{"GET", "/v1/product", "", ""},
		{"GET", "/v1/product/{id}", "", ""},
		{"POST", "/v1/order/calculate-net-total", "", ""},
		{"POST", "/v1/product/{id}", "", ""},
		{"PUT", "/v1/product/{id}/restore", "", ""},
		{"POST", "/v1/authorize", "", ""},
		{"POST", "/v1/user", "", ""},
		{"POST", "/v1/store", "", ""},
		{"POST", "", "", ""},
		{"POST", "/v2/product", "", ""},
	}
	for _, c := range cases {
		r, a := rbacTarget(c.method, c.tpl)
		if r != c.resource || a != c.action {
			t.Errorf("%s %s = %q %q, want %q %q", c.method, c.tpl, r, a, c.resource, c.action)
		}
	}
	if r, _ := rbacTarget(http.MethodPost, "/v1/purchase"); r != "purchases" {
		t.Errorf("purchase = %q", r)
	}
}

func TestRBACDenies(t *testing.T) {
	perms := []models.Permission{
		{Resource: "products", Read: true},
		{Resource: "customers", Read: true, Create: true, Update: true},
	}
	cases := []struct {
		resource, action string
		deny             bool
	}{
		{"products", "create", true},
		{"products", "update", true},
		{"products", "delete", true},
		{"customers", "create", false},
		{"customers", "update", false},
		{"customers", "delete", true},
		{"vendors", "create", false}, // no role mentions vendors: allowed, as in the app
	}
	for _, c := range cases {
		if got := rbacDenies(perms, c.resource, c.action); got != c.deny {
			t.Errorf("%s %s: deny = %v, want %v", c.action, c.resource, got, c.deny)
		}
	}
	if rbacDenies(nil, "products", "create") {
		t.Errorf("no permissions at all must not deny")
	}
}

func TestEnsureStatus(t *testing.T) {
	rec := &recorder{}
	tr := &statusTracker{ResponseWriter: rec}
	ensureStatus(tr, http.StatusBadRequest)
	if rec.code != http.StatusBadRequest {
		t.Fatalf("no status yet: got %d, want 400", rec.code)
	}
	rec2 := &recorder{}
	tr2 := &statusTracker{ResponseWriter: rec2}
	tr2.WriteHeader(http.StatusConflict)
	ensureStatus(tr2, http.StatusBadRequest)
	if rec2.code != http.StatusConflict || rec2.calls != 1 {
		t.Fatalf("status already set: got %d after %d calls, want 409 once", rec2.code, rec2.calls)
	}
}

type recorder struct {
	code, calls int
	header      http.Header
}

func (r *recorder) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}
func (r *recorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *recorder) WriteHeader(code int)        { r.code = code; r.calls++ }
