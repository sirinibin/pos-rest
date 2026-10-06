package erp

import (
	"net/http/httptest"
	"testing"
)

func TestSystemRoles_MatchPrototype(t *testing.T) {
	roles := systemRoles()
	if len(roles) != 6 {
		t.Fatalf("want 6 system roles, got %d", len(roles))
	}
	want := map[string]map[string]map[string]bool{
		"r_admin":      {"settings": {"edit": true, "delete": true}, "sales": {"delete": true}},
		"r_manager":    {"settings": {"view": true, "edit": false}, "sales": {"delete": true}},
		"r_salesman":   {"sales": {"create": true, "delete": false}, "inventory": {"view": true, "create": false}, "purchases": {"view": false}},
		"r_cashier":    {"sales": {"create": true, "edit": false}, "customers": {"view": true, "create": false}, "finance": {"view": false}},
		"r_accountant": {"finance": {"delete": true}, "sales": {"view": true, "create": false}},
		"r_viewer":     {"sales": {"view": true, "create": false}, "settings": {"edit": false}},
	}
	for _, r := range roles {
		p := permsFromM(r["perms"])
		for mod, verbs := range want[str(r["id"])] {
			for v, exp := range verbs {
				if p[mod][v] != exp {
					t.Errorf("%s %s.%s=%v want %v", r["id"], mod, v, p[mod][v], exp)
				}
			}
		}
		if r["system"] != true {
			t.Errorf("%s must be system", r["id"])
		}
	}
	if systemRoleByID("r_nope") != nil {
		t.Fatal("unknown role")
	}
}

func TestLegacyRoleMapping(t *testing.T) {
	cases := []struct {
		role  string
		admin bool
		want  string
	}{
		{"Admin", false, "r_admin"}, {"", true, "r_admin"}, {"Manager", false, "r_manager"},
		{"SalesMan", false, "r_salesman"}, {"salesmen", false, "r_salesman"}, {"", false, "r_salesman"},
	}
	for _, c := range cases {
		if got := legacyRoleToContract(c.role, c.admin); got != c.want {
			t.Errorf("%q/%v → %s want %s", c.role, c.admin, got, c.want)
		}
	}
	// tenant admin must NEVER become the legacy platform super-admin
	if contractRoleToLegacy("r_admin") == "Admin" {
		t.Fatal("r_admin must not map to legacy Admin")
	}
	if contractRoleToLegacy("r_cashier") != "SalesMan" || contractRoleToLegacy("r_manager") != "Manager" {
		t.Fatal("legacy mapping")
	}
}

func TestLegacyPermissionsToPerms(t *testing.T) {
	p := legacyPermissionsToPerms([]interface{}{
		M{"resource": "products", "read": true, "create": true, "update": true, "delete": false},
		M{"resource": "receivables", "read": true},
		M{"resource": "unknown_res", "read": true, "create": true},
		"garbage",
	})
	if !p["inventory"]["view"] || !p["inventory"]["create"] || !p["inventory"]["edit"] || p["inventory"]["delete"] {
		t.Fatalf("inventory=%v", p["inventory"])
	}
	if !p["finance"]["view"] || p["finance"]["create"] {
		t.Fatalf("finance=%v", p["finance"])
	}
	u := unionPerms(p, permsFromM(systemRoleByID("r_viewer")["perms"]))
	if !u["sales"]["view"] || !u["inventory"]["create"] {
		t.Fatal("union")
	}
}

func TestVerbForMethod(t *testing.T) {
	cases := map[string]string{"GET": "view", "POST": "create", "PATCH": "edit", "PUT": "edit", "DELETE": "delete"}
	for m, want := range cases {
		if got := verbForMethod(m, false); got != want {
			t.Errorf("%s → %s", m, got)
		}
	}
	if verbForMethod("POST", true) != "edit" {
		t.Fatal("restore = edit")
	}
}

func TestCtxCan(t *testing.T) {
	c := &Ctx{Perms: permsFromM(systemRoleByID("r_salesman")["perms"])}
	if !c.can("sales", "create") || c.can("settings", "edit") || !c.can("", "anything") {
		t.Fatal("can")
	}
	if (&Ctx{}).can("sales", "view") {
		t.Fatal("nil perms deny")
	}
}

func TestBearerParsing(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if bearer(r) != "" {
		t.Fatal("empty")
	}
	r.Header.Set("Authorization", "Bearer abc")
	if bearer(r) != "abc" {
		t.Fatal("bearer")
	}
	r.Header.Set("Authorization", "rawtoken")
	if bearer(r) != "rawtoken" {
		t.Fatal("raw")
	}
}
