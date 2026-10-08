package erp

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed API + integration tests: business category on sign-up, store
// create (platform admin) and edit, and the product POS catalog fields.

func validNewStore(tag string) M {
	return M{
		"nameEn": "Category Test " + tag, "nameAr": "فرع التجربة", "branchEn": "Olaya", "branchAr": "العليا",
		"vatNo": "310122393500003", "crNo": "1010101010", "email": "branch+" + tag + "@stores.example",
		"phone": "0112345678", "category": "coffee shop",
		"address": M{"buildingNo": "1234", "streetEn": "Olaya", "streetAr": "العليا", "districtEn": "Olaya", "districtAr": "العليا",
			"cityEn": "Riyadh", "cityAr": "الرياض", "postalCode": "12345", "additionalNo": "6789", "shortAddress": "RRRD1234"},
	}
}

func TestAPI_Signup_BusinessCategory(t *testing.T) {
	requireDB(t)
	body := validSignup()
	body["owner"].(M)["email"] = "cat+" + time.Now().Format("150405.000000") + "@signup.example"
	body["company"].(M)["type"] = "grocery"
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	sid := str(get(r.Body, "store.id"))
	defer cleanupStore(t, sid)
	if get(r.Body, "store.category") != "Grocery" || get(r.Body, "store.posTerminal") != "grocery" {
		t.Fatalf("store category=%v posTerminal=%v", get(r.Body, "store.category"), get(r.Body, "store.posTerminal"))
	}
	// the legacy field ZATCA onboarding reads holds the canonical ZATCA-safe value
	oid, _ := oidOf(sid)
	var raw bson.M
	ctx, cancel := dbctx()
	defer cancel()
	_ = mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid}).Decode(&raw)
	if raw["business_category"] != "Grocery" || !ValidZatcaCategory(str(raw["business_category"])) {
		t.Fatalf("legacy business_category=%v", raw["business_category"])
	}
	// an unknown category is a field error
	body["owner"].(M)["email"] = "cat2+" + time.Now().Format("150405.000000") + "@signup.example"
	body["company"].(M)["type"] = "Food & drink"
	if b := call(t, "POST", "/auth/signup", "", body); b.Code != 400 || b.errField("company.type") == "" {
		t.Fatalf("bad category: %d %s", b.Code, b.Raw)
	}
}

func TestAPI_Stores_CreateByPlatformAdmin(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	tag := time.Now().Format("150405000000")
	r := call(t, "POST", "/stores", admin, validNewStore(tag))
	if r.Code != 201 && r.Code != 200 {
		t.Fatalf("create store: %d %s", r.Code, r.Raw)
	}
	sid := str(r.Body["id"])
	defer cleanupStore(t, sid)
	if r.Body["category"] != "Coffee Shop" || r.Body["posTerminal"] != "coffee" || r.Body["nameEn"] != "Category Test "+tag ||
		r.Body["branchEn"] != "Olaya" || r.Body["branchAr"] != "العليا" || r.Body["plan"] != "professional" ||
		get(r.Body, "zatca.phase") != 2.0 {
		t.Fatalf("created store: %v", r.Body)
	}
	// it is listed for the admin and its store DB works (warehouses)
	if g := call(t, "GET", "/stores/"+sid, admin, nil); g.Code != 200 || g.Body["category"] != "Coffee Shop" {
		t.Fatalf("get new store: %d %s", g.Code, g.Raw)
	}
	if wh := call(t, "GET", "/warehouses?storeId="+sid, admin, nil); wh.Code != 200 || len(wh.data()) < 1 {
		t.Fatalf("new store warehouses: %d %s", wh.Code, wh.Raw)
	}
	// edit: change the category, then reject free text
	g := call(t, "GET", "/stores/"+sid, admin, nil)
	p := call(t, "PATCH", "/stores/"+sid, admin, M{"category": "Barber Shop"}, "If-Match", str(g.Body["version"]))
	if p.Code != 200 || p.Body["category"] != "Barber Shop" || p.Body["posTerminal"] != "barber" {
		t.Fatalf("patch category: %d %s", p.Code, p.Raw)
	}
	if b := call(t, "PATCH", "/stores/"+sid, admin, M{"category": "Bakery & sweets"}); b.Code != 400 || b.errField("category") == "" {
		t.Fatalf("free-text category: %d %s", b.Code, b.Raw)
	}
	// validation errors use contract keys
	bad := validNewStore(tag + "b")
	bad["category"] = "Food"
	bad["nameAr"] = ""
	delete(bad["address"].(M), "postalCode")
	if b := call(t, "POST", "/stores", admin, bad); b.Code != 400 || b.errField("category") == "" || b.errField("nameAr") == "" || b.errField("address.postalCode") == "" {
		t.Fatalf("invalid create: %d %s", b.Code, b.Raw)
	}
}

func TestAPI_Stores_CreateForbiddenForOwners(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "catown")
	defer cleanupStore(t, sid)
	if r := call(t, "POST", "/stores", owner, validNewStore("own")); r.Code != 403 {
		t.Fatalf("owner create store: %d %s", r.Code, r.Raw)
	}
	// owners still edit their own store's category
	g := call(t, "GET", "/stores/"+sid, owner, nil)
	p := call(t, "PATCH", "/stores/"+sid, owner, M{"category": "Supermarket"}, "If-Match", str(g.Body["version"]))
	if p.Code != 200 || p.Body["posTerminal"] != "supermarket" {
		t.Fatalf("owner patch category: %d %s", p.Code, p.Raw)
	}
	// unchanged legacy free-text category does not block other edits
	oid, _ := oidOf(sid)
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"business_category": "Retail"}})
	g = call(t, "GET", "/stores/"+sid, owner, nil)
	if g.Body["posTerminal"] != "" {
		t.Fatalf("legacy category terminal: %v", g.Body["posTerminal"])
	}
	if p := call(t, "PATCH", "/stores/"+sid, owner, M{"branchEn": "Second"}, "If-Match", str(g.Body["version"])); p.Code != 200 || p.Body["category"] != "Retail" {
		t.Fatalf("legacy category edit: %d %s", p.Code, p.Raw)
	}
}

func TestAPI_Stores_BusinessVisaAndTravels(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "cattravel")
	defer cleanupStore(t, sid)
	g := call(t, "GET", "/stores/"+sid, owner, nil)
	// the display spelling ("," and "&") is not ZATCA-safe, so it is rejected
	if b := call(t, "PATCH", "/stores/"+sid, owner, M{"category": "Business, Visa & Travels"}, "If-Match", str(g.Body["version"])); b.Code != 400 || b.errField("category") == "" {
		t.Fatalf("display spelling accepted: %d %s", b.Code, b.Raw)
	}
	g = call(t, "GET", "/stores/"+sid, owner, nil)
	p := call(t, "PATCH", "/stores/"+sid, owner, M{"category": "business visa and travels"}, "If-Match", str(g.Body["version"]))
	if p.Code != 200 || p.Body["category"] != "Business Visa and Travels" || p.Body["posTerminal"] != "travel" {
		t.Fatalf("travel category: %d %s", p.Code, p.Raw)
	}
	body := M{"storeId": sid, "nameEn": "UAE visit visa (30 days)", "nameAr": "تأشيرة زيارة الإمارات", "unit": "Job", "isService": true,
		"pricing": M{"retail": 391.3043}, "posTerminal": "travel", "posSection": "visa", "posKey": "v-uae"}
	if r := call(t, "POST", "/products?storeId="+sid, owner, body); (r.Code != 201 && r.Code != 200) || r.Body["posTerminal"] != "travel" {
		t.Fatalf("travel product: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_Products_PosFields(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "catprod")
	defer cleanupStore(t, sid)
	body := M{"storeId": sid, "nameEn": "Fresh milk 1 L", "nameAr": "حليب طازج", "unit": "Btl", "barcode": "6281000000066",
		"pricing": M{"retail": 5.6522}, "posTerminal": "grocery", "posSection": "dairy", "posKey": "g6"}
	r := call(t, "POST", "/products?storeId="+sid, owner, body)
	if r.Code != 201 && r.Code != 200 {
		t.Fatalf("create product: %d %s", r.Code, r.Raw)
	}
	if r.Body["posTerminal"] != "grocery" || r.Body["posSection"] != "dairy" || r.Body["posKey"] != "g6" {
		t.Fatalf("pos fields: %v", r.Body)
	}
	id := str(r.Body["id"])
	// select keeps the POS fields the terminals request
	l := call(t, "GET", "/products?storeId="+sid+"&select=nameEn,pricing,posTerminal,posSection,posKey", owner, nil)
	if l.Code != 200 || len(l.data()) != 1 {
		t.Fatalf("list: %d %s", l.Code, l.Raw)
	}
	row := l.data()[0].(M)
	if row["posSection"] != "dairy" || row["posKey"] != "g6" || row["barcode"] != nil {
		t.Fatalf("selected row: %v", row)
	}
	// edit the section, reject an unknown terminal
	p := call(t, "PATCH", "/products/"+id+"?storeId="+sid, owner, M{"posSection": "drinks"}, "If-Match", str(r.Body["version"]))
	if p.Code != 200 || p.Body["posSection"] != "drinks" {
		t.Fatalf("patch section: %d %s", p.Code, p.Raw)
	}
	if b := call(t, "PATCH", "/products/"+id+"?storeId="+sid, owner, M{"posTerminal": "casino"}); b.Code != 400 || b.errField("posTerminal") == "" {
		t.Fatalf("bad terminal: %d %s", b.Code, b.Raw)
	}
}

// Industrial Supplies stores: class / size / material options (product-specs)
// picked on products (specs, kept in erp.x) and listed with ?select=specs.
func TestAPI_Products_Specs(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "catspecs")
	defer cleanupStore(t, sid)
	mk := func(kind, name string) string {
		r := call(t, "POST", "/product-specs?storeId="+sid, owner, M{"storeId": sid, "kind": kind, "name": name})
		if r.Code != 201 && r.Code != 200 {
			t.Fatalf("create %s %s: %d %s", kind, name, r.Code, r.Raw)
		}
		return str(r.Body["id"])
	}
	cl, sz, mat := mk("class", "150"), mk("size", `2"`), mk("material", "A105 CS")
	if b := call(t, "POST", "/product-specs?storeId="+sid, owner, M{"storeId": sid, "kind": "grade", "name": "X"}); b.Code != 400 || b.errField("kind") == "" {
		t.Fatalf("bad kind accepted: %d %s", b.Code, b.Raw)
	}
	if l := call(t, "GET", "/product-specs?storeId="+sid, owner, nil); l.Code != 200 || len(l.data()) != 3 {
		t.Fatalf("list specs: %d %s", l.Code, l.Raw)
	}
	body := M{"storeId": sid, "nameEn": "Gate valve 2in CL150", "unit": "Pcs", "pricing": M{"retail": 420},
		"specs": M{"class": cl, "size": sz, "material": mat}}
	r := call(t, "POST", "/products?storeId="+sid, owner, body)
	if r.Code != 201 && r.Code != 200 {
		t.Fatalf("create product: %d %s", r.Code, r.Raw)
	}
	if s := sub(r.Body, "specs"); s["class"] != cl || s["size"] != sz || s["material"] != mat {
		t.Fatalf("specs: %v", r.Body["specs"])
	}
	l := call(t, "GET", "/products?storeId="+sid+"&select=nameEn,specs,categoryIds,brandId", owner, nil)
	if l.Code != 200 || len(l.data()) != 1 || sub(l.data()[0].(M), "specs")["size"] != sz {
		t.Fatalf("selected specs: %d %s", l.Code, l.Raw)
	}
	if b := call(t, "PATCH", "/products/"+str(r.Body["id"])+"?storeId="+sid, owner, M{"specs": M{"grade": "x"}}); b.Code != 400 || b.errField("specs.grade") == "" {
		t.Fatalf("bad spec key accepted: %d %s", b.Code, b.Raw)
	}
}
