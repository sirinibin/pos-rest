package erp

import (
	"testing"
	"time"
)

// Integration (needs MongoDB + Redis): the Kerala tailoring categories sign up
// and switch only in India, open their own POS terminal and seed a rupee
// starter catalog; a Saudi store cannot pick them.
func TestAPI_KeralaCategories_IndiaStore(t *testing.T) {
	requireDB(t)
	body := indiaSignup()
	body["owner"].(M)["email"] = "kerala+" + time.Now().Format("150405.000000") + "@signup.example"
	body["company"].(M)["type"] = "Kerala Tailor Shop and Gents Dress"
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("Kerala signup: %d %s", r.Code, r.Raw)
	}
	sid := str(get(r.Body, "store.id"))
	defer cleanupStore(t, sid)
	tok := str(r.Body["accessToken"])
	for _, c := range []struct{ value, terminal string }{
		{"Kerala Tailor Shop and Gents Dress", "keralagents"},
		{"Kerala Tailor Shop", "keralatailor"},
		{"Kerala Ladies Boutique", "keralaboutique"},
	} {
		plan, _ := planStarterCatalog(c.terminal, "IN")
		if c.terminal == "keralagents" { // sign-up already seeded its own category
			if n := len(listAll(t, tok, "/products?limit=1000&storeId="+sid)); len(plan.Items) < 20 || n != len(plan.Items) {
				t.Errorf("sign-up seeded %d of %d items", n, len(plan.Items))
			}
			continue
		}
		g := call(t, "GET", "/stores/"+sid, tok, nil)
		p := call(t, "PATCH", "/stores/"+sid, tok, M{"category": c.value}, "If-Match", str(g.Body["version"]))
		if p.Code != 200 {
			t.Fatalf("%s: set category: %d %s", c.value, p.Code, p.Raw)
		}
		if p.Body["posTerminal"] != c.terminal {
			t.Errorf("%s: posTerminal %v, want %s", c.value, p.Body["posTerminal"], c.terminal)
		}
		s := call(t, "POST", "/stores/"+sid+"/starter-catalog", tok, nil)
		if s.Code != 200 {
			t.Fatalf("%s: seed: %d %s", c.value, s.Code, s.Raw)
		}
		n := int(num(get(s.Body, "created.products")) + num(get(s.Body, "created.services")))
		if n != len(plan.Items) {
			t.Errorf("%s: created %d of %d items", c.value, n, len(plan.Items))
		}
	}

	// a Saudi store cannot take a Kerala category
	stok, ssid := signupOwner(t, "kerala-sa")
	defer cleanupStore(t, ssid)
	g := call(t, "GET", "/stores/"+ssid, stok, nil)
	if p := call(t, "PATCH", "/stores/"+ssid, stok, M{"category": "Kerala Ladies Boutique"}, "If-Match", str(g.Body["version"])); p.Code != 400 || p.errField("category") == "" {
		t.Errorf("Saudi store took a Kerala category: %d %s", p.Code, p.Raw)
	}
	sa := validSignup()
	sa["owner"].(M)["email"] = "kerala-sa+" + time.Now().Format("150405.000000") + "@signup.example"
	sa["company"].(M)["type"] = "Kerala Tailor Shop"
	if s := call(t, "POST", "/auth/signup", "", sa); s.Code != 400 || s.errField("company.type") == "" {
		t.Errorf("Saudi sign-up took a Kerala category: %d %s", s.Code, s.Raw)
	}
}
