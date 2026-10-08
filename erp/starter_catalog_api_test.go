package erp

import (
	"testing"
	"time"
)

// DB-backed: the starter catalog is seeded on sign-up and when a platform
// admin adds a store, can be added to an existing store, never duplicates,
// and is ordinary data (editable, deletable).

func signupWithCategory(t *testing.T, category, country string, extra M) (string, string) {
	t.Helper()
	body := validSignup()
	body["owner"].(M)["email"] = "starter+" + time.Now().Format("150405.000000") + "@signup.example"
	co := body["company"].(M)
	co["type"] = category
	if country != "" && country != "SA" {
		co["countryCode"] = country
		co["vatNo"] = ""
		co["mobile"] = "+971501234567"
		co["address"] = M{"streetEn": "Sheikh Zayed Road", "cityEn": "Dubai"}
	}
	for k, v := range extra {
		co[k] = v
	}
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("signup %s: %d %s", category, r.Code, r.Raw)
	}
	return str(r.Body["accessToken"]), str(get(r.Body, "store.id"))
}

func listAll(t *testing.T, tok, path string) []interface{} {
	t.Helper()
	r := call(t, "GET", path, tok, nil)
	if r.Code != 200 {
		t.Fatalf("GET %s: %d %s", path, r.Code, r.Raw)
	}
	return r.data()
}

func TestAPI_StarterCatalog_SeededOnSignup(t *testing.T) {
	requireDB(t)
	start := time.Now()
	tok, sid := signupWithCategory(t, "Industrial Supplies", "SA", nil)
	defer cleanupStore(t, sid)
	took := time.Since(start)
	plan, _ := planStarterCatalog("industrial", "SA")
	want := plan.counts()

	cats := listAll(t, tok, "/categories?limit=500")
	if len(cats) != want.Categories {
		t.Fatalf("categories %d, want %d", len(cats), want.Categories)
	}
	var valves M
	for _, c := range cats {
		if m := c.(M); m["nameEn"] == "Valves" {
			valves = m
		}
	}
	if valves == nil || valves["nameAr"] != "المحابس" {
		t.Fatalf("Valves category with Arabic name: %v", valves)
	}
	brands := listAll(t, tok, "/brands?limit=500")
	if len(brands) != want.Brands {
		t.Errorf("brands %d, want %d", len(brands), want.Brands)
	}
	specs := listAll(t, tok, "/product-specs?limit=500&storeId="+sid)
	if len(specs) != want.Specs {
		t.Errorf("spec options %d, want %d", len(specs), want.Specs)
	}
	prods := listAll(t, tok, "/products?limit=1000&storeId="+sid)
	if len(prods) != want.Products+want.Services {
		t.Fatalf("products %d, want %d", len(prods), want.Products+want.Services)
	}
	var gv M
	for _, p := range prods {
		if m := p.(M); m["posKey"] == "in-gv-2" {
			gv = m
		}
	}
	if gv == nil {
		t.Fatalf("gate valve not seeded")
	}
	sp := sub(gv, "specs")
	if gv["posTerminal"] != "industrial" || gv["posSection"] != "valves" || len(arr(gv["categoryIds"])) != 1 ||
		str(arr(gv["categoryIds"])[0]) != str(valves["id"]) || str(gv["brandId"]) == "" || str(sp["size"]) == "" ||
		str(sp["class"]) == "" || str(sp["material"]) == "" || num(sub(gv, "pricing")["retail"]) != 260 {
		t.Fatalf("gate valve record: %v", gv)
	}
	if str(gv["code"]) == "" {
		t.Errorf("seeded products get a product code")
	}
	t.Logf("sign-up with %d starter items took %v", len(prods), took)

	// the POS filter chips come from this data: every starter category, brand
	// and class / size / material option in use shows up in the facets
	f := call(t, "GET", "/products/facets?storeId="+sid+"&where.forTerminal=industrial", tok, nil)
	if f.Code != 200 {
		t.Fatalf("facets: %d %s", f.Code, f.Raw)
	}
	if n := len(arr(f.Body["categoryId"])); n != want.Categories {
		t.Errorf("category facet %d, want %d", n, want.Categories)
	}
	if n := len(arr(f.Body["brandId"])); n != want.Brands {
		t.Errorf("brand facet %d, want %d", n, want.Brands)
	}
	for _, k := range []string{"specClass", "specSize", "specMaterial"} {
		if len(arr(f.Body[k])) == 0 {
			t.Errorf("%s facet empty", k)
		}
	}

	// preview: everything is in
	pv := call(t, "GET", "/stores/"+sid+"/starter-catalog", tok, nil)
	if pv.Code != 200 || pv.Body["available"] != true || num(pv.Body["missing"]) != 0 || pv.Body["terminal"] != "industrial" {
		t.Fatalf("preview: %d %s", pv.Code, pv.Raw)
	}
	// running it again adds nothing
	again := call(t, "POST", "/stores/"+sid+"/starter-catalog", tok, nil)
	if again.Code != 200 || num(get(again.Body, "created.products")) != 0 || num(get(again.Body, "created.categories")) != 0 ||
		num(get(again.Body, "created.brands")) != 0 || num(get(again.Body, "created.specs")) != 0 {
		t.Fatalf("second seed: %d %s", again.Code, again.Raw)
	}
	// seeded data is ordinary: edit a price, delete a product, then re-seeding restores only the deleted one
	g := call(t, "GET", "/products/"+str(gv["id"]), tok, nil)
	p := call(t, "PATCH", "/products/"+str(gv["id"]), tok, M{"pricing": M{"retail": 275.0}}, "If-Match", str(g.Body["version"]))
	if p.Code != 200 || num(get(p.Body, "pricing.retail")) != 275 {
		t.Fatalf("edit seeded product: %d %s", p.Code, p.Raw)
	}
	other := prods[0].(M)
	if other["id"] == gv["id"] {
		other = prods[1].(M)
	}
	if d := call(t, "DELETE", "/products/"+str(other["id"]), tok, nil); d.Code != 200 && d.Code != 204 {
		t.Fatalf("delete seeded product: %d %s", d.Code, d.Raw)
	}
	re := call(t, "POST", "/stores/"+sid+"/starter-catalog", tok, nil)
	if re.Code != 200 || num(get(re.Body, "created.products"))+num(get(re.Body, "created.services")) != 1 {
		t.Fatalf("re-seed after delete: %d %s", re.Code, re.Raw)
	}
	g2 := call(t, "GET", "/products/"+str(gv["id"]), tok, nil)
	if num(get(g2.Body, "pricing.retail")) != 275 {
		t.Errorf("re-seed must not overwrite the owner's price: %v", get(g2.Body, "pricing.retail"))
	}
}

func TestAPI_StarterCatalog_CountryAndOptOut(t *testing.T) {
	requireDB(t)
	// UAE mobile shop: UAE telecom, prices in AED, VAT 5 %
	tok, sid := signupWithCategory(t, "Mobile Phones and Accessories", "AE", nil)
	defer cleanupStore(t, sid)
	prods := listAll(t, tok, "/products?limit=1000&storeId="+sid)
	keys := map[string]M{}
	for _, p := range prods {
		keys[str(p.(M)["posKey"])] = p.(M)
	}
	if keys["sim-stc"] != nil || keys["sim-du-ae"] == nil {
		t.Fatalf("UAE store SIMs: stc=%v du=%v (of %d)", keys["sim-stc"] != nil, keys["sim-du-ae"] != nil, len(prods))
	}
	// 55 AED incl. 5 % VAT → 52.381 ex-VAT
	if r := num(get(keys["sim-du-ae"], "pricing.retail")); r < 52.38 || r > 52.39 {
		t.Errorf("du SIM retail ex-VAT: %v", r)
	}

	// opt out at sign-up: nothing seeded, the preview offers it, POST adds it
	tok2, sid2 := signupWithCategory(t, "Barber Shop", "SA", M{"starterCatalog": false})
	defer cleanupStore(t, sid2)
	if n := len(listAll(t, tok2, "/products?limit=1000&storeId="+sid2)); n != 0 {
		t.Fatalf("opted-out store has %d products", n)
	}
	pv := call(t, "GET", "/stores/"+sid2+"/starter-catalog", tok2, nil)
	plan, _ := planStarterCatalog("barber", "SA")
	if pv.Body["available"] != true || int(num(pv.Body["missing"])) != len(plan.Items) || int(num(get(pv.Body, "counts.services"))) != plan.counts().Services {
		t.Fatalf("preview of empty barber store: %s", pv.Raw)
	}
	s := call(t, "POST", "/stores/"+sid2+"/starter-catalog", tok2, nil)
	if s.Code != 200 || int(num(get(s.Body, "created.services"))) != plan.counts().Services {
		t.Fatalf("seed barber: %d %s", s.Code, s.Raw)
	}
	svc := 0
	for _, p := range listAll(t, tok2, "/products?limit=1000&storeId="+sid2) {
		if p.(M)["isService"] == true {
			svc++
		}
	}
	if svc != plan.counts().Services {
		t.Errorf("barber services %d, want %d", svc, plan.counts().Services)
	}

	// other people's stores are out of reach
	if f := call(t, "POST", "/stores/"+sid+"/starter-catalog", tok2, nil); f.Code != 403 {
		t.Errorf("foreign store seed: %d %s", f.Code, f.Raw)
	}
	if f := call(t, "GET", "/stores/"+sid+"/starter-catalog", tok2, nil); f.Code != 403 {
		t.Errorf("foreign store preview: %d %s", f.Code, f.Raw)
	}
}

func TestAPI_StarterCatalog_NoCategoryAndAdminCreate(t *testing.T) {
	requireDB(t)
	// a free-text category ("retail") has no terminal: nothing seeded, 409 on request
	tok, sid := signupOwner(t, "starter-none")
	defer cleanupStore(t, sid)
	if pv := call(t, "GET", "/stores/"+sid+"/starter-catalog", tok, nil); pv.Code != 200 || pv.Body["available"] != false {
		t.Fatalf("preview without category: %d %s", pv.Code, pv.Raw)
	}
	if s := call(t, "POST", "/stores/"+sid+"/starter-catalog", tok, nil); s.Code != 409 || s.errCode() != "no_business_category" {
		t.Fatalf("seed without category: %d %s", s.Code, s.Raw)
	}

	// a platform admin adding a store seeds it into THAT store only
	admin := login(t, fx.AdminEmail)
	before := len(listAll(t, admin, "/products?limit=5000&storeId="+storeA()))
	r := call(t, "POST", "/stores", admin, validNewStore(time.Now().Format("150405000000")))
	if r.Code != 201 && r.Code != 200 {
		t.Fatalf("create store: %d %s", r.Code, r.Raw)
	}
	nsid := str(r.Body["id"])
	defer cleanupStore(t, nsid)
	plan, _ := planStarterCatalog("coffee", "SA")
	if n := len(listAll(t, admin, "/products?limit=1000&storeId="+nsid)); n != len(plan.Items) {
		t.Fatalf("admin-created coffee shop has %d products, want %d", n, len(plan.Items))
	}
	if after := len(listAll(t, admin, "/products?limit=5000&storeId="+storeA())); after != before {
		t.Fatalf("another store changed: %d → %d products", before, after)
	}
	// starterCatalog: false on create skips it
	body := validNewStore(time.Now().Format("150405000000") + "x")
	body["starterCatalog"] = false
	r2 := call(t, "POST", "/stores", admin, body)
	if r2.Code != 201 && r2.Code != 200 {
		t.Fatalf("create store (no starter): %d %s", r2.Code, r2.Raw)
	}
	defer cleanupStore(t, str(r2.Body["id"]))
	if n := len(listAll(t, admin, "/products?limit=1000&storeId="+str(r2.Body["id"]))); n != 0 {
		t.Errorf("opted-out store has %d products", n)
	}
	if r2.Body["starterCatalog"] != nil {
		t.Errorf("starterCatalog must not be stored on the store: %v", r2.Body["starterCatalog"])
	}
}

// Every business category's starter catalog is accepted by the product,
// category, brand and option resources (legacy validation included).
func TestAPI_StarterCatalog_EveryCategorySeeds(t *testing.T) {
	requireDB(t)
	tok, sid := signupOwner(t, "starter-all")
	defer cleanupStore(t, sid)
	for _, bc := range BusinessCategories {
		if !CategoryAllowedIn(bc.Value, "SA") {
			continue // India-only categories: TestAPI_KeralaCategories_IndiaStore
		}
		g := call(t, "GET", "/stores/"+sid, tok, nil)
		if p := call(t, "PATCH", "/stores/"+sid, tok, M{"category": bc.Value}, "If-Match", str(g.Body["version"])); p.Code != 200 {
			t.Fatalf("%s: set category: %d %s", bc.Value, p.Code, p.Raw)
		}
		plan, _ := planStarterCatalog(bc.Terminal, "SA")
		s := call(t, "POST", "/stores/"+sid+"/starter-catalog", tok, nil)
		if s.Code != 200 {
			t.Fatalf("%s: seed: %d %s", bc.Value, s.Code, s.Raw)
		}
		n := int(num(get(s.Body, "created.products")) + num(get(s.Body, "created.services")))
		if n != len(plan.Items) {
			t.Errorf("%s: created %d of %d items", bc.Value, n, len(plan.Items))
		}
	}
}

// Brands whose names start alike get distinct legacy codes (was "Code is already in use").
func TestAPI_Brands_SimilarNamesGetUniqueCodes(t *testing.T) {
	requireDB(t)
	tok, sid := signupOwner(t, "brand-codes")
	defer cleanupStore(t, sid)
	for _, n := range []string{"Arabian Oud", "Arabian Pipes", "Arabian Gulf"} {
		if r := call(t, "POST", "/brands", tok, M{"name": n}); r.Code != 201 {
			t.Fatalf("create brand %s: %d %s", n, r.Code, r.Raw)
		}
	}
	codes := map[string]bool{}
	for _, b := range listAll(t, tok, "/brands?limit=100") {
		c := str(b.(M)["code"])
		if codes[c] {
			t.Errorf("duplicate brand code %s", c)
		}
		codes[c] = true
	}
}
