package erp

import (
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// A set (bundle) is a product with components. The legacy product save stores
// is_set the wrong way round (true without components, false with them), so
// the flag is not trusted: products created or edited through the API must
// read as ordinary products and show in the POS grids (where.isSet=false),
// and a set stays a set after an edit.
func TestProductIsSet(t *testing.T) {
	comp := bson.A{M{"product_id": hexID(), "quantity": 2.0}}
	for _, c := range []struct {
		name string
		d    M
		want bool
	}{
		{"no flag, no set", M{}, false},
		{"legacy save of an ordinary product", M{"is_set": true, "set": M{"products": bson.A{}}}, false},
		{"flag without a set field", M{"is_set": true}, false},
		{"real set (legacy import)", M{"is_set": true, "set": M{"products": comp}}, true},
		{"set after a legacy save (flag off)", M{"is_set": false, "set": M{"products": comp}}, true},
		{"misspelled legacy key", M{"is_set": true, "set": M{"products": bson.A{M{"produc_id": hexID()}}}}, true},
	} {
		if got := productIsSet(c.d); got != c.want {
			t.Errorf("%s: productIsSet = %v, want %v", c.name, got, c.want)
		}
		x := testX(hexID(), M{"_id": hexID()})
		if got := productToContract(x, c.d)["isSet"]; got != c.want {
			t.Errorf("%s: contract isSet = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestProductSetWhere(t *testing.T) {
	yes, ok := productSetWhere("true", "")
	if !ok || yes["set.products.0"] == nil {
		t.Fatalf("isSet=true: %v", yes)
	}
	no, ok := productSetWhere("false", "")
	if !ok || no["set.products.0"] == nil || yes["set.products.0"].(bson.M)["$exists"] != true {
		t.Fatalf("isSet=false: %v", no)
	}
	for _, bad := range []string{"", "1", "yes", "TRUE"} {
		if _, ok := productSetWhere(bad, ""); ok {
			t.Errorf("where.isSet=%q must be rejected", bad)
		}
	}
	b := newProductsResource().Backend.(*legacyBackend)
	if b.listWhere["isSet"].fn == nil {
		t.Fatal("where.isSet is not productSetWhere")
	}
	if m, _ := b.listWhere["isSet"].fn("false", ""); m["set.products.0"] == nil {
		t.Fatalf("where.isSet=false: %v", m)
	}
}

// Against the database: a product created through the API is not a set, and
// the POS filter where.isSet=false finds it; the fixture's real set does not.
func TestAPI_ProductCreatedIsNotASet(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	r := call(t, "POST", "/products", tok, M{"storeId": storeA(), "nameEn": "Set flag probe", "unit": "Pcs",
		"pricing": M{"purchase": 5.0, "retail": 10.0}})
	if r.Code != http.StatusCreated && r.Code != http.StatusOK {
		t.Fatalf("create: %d %s", r.Code, r.Raw)
	}
	id := str(r.Body["id"])
	if r.Body["isSet"] != false {
		t.Fatalf("created product reads as a set: %v", r.Body["isSet"])
	}
	if g := call(t, "GET", "/products/"+id+"?storeId="+storeA(), tok, nil); g.Code != 200 || g.Body["isSet"] != false {
		t.Fatalf("get: %d isSet=%v", g.Code, g.Body["isSet"])
	}
	// an edit goes through the legacy save again
	if p := call(t, "PATCH", "/products/"+id+"?storeId="+storeA(), tok, M{"nameEn": "Set flag probe 2"},
		"If-Match", str(r.Header.Get("ETag"))); p.Code != 200 || p.Body["isSet"] != false {
		t.Fatalf("patch: %d %s", p.Code, p.Raw)
	}
	has := func(where, want string) bool {
		l := call(t, "GET", "/products?storeId="+storeA()+"&limit=500&page=1&select=nameEn&where.isSet="+where, tok, nil)
		if l.Code != 200 {
			t.Fatalf("list isSet=%s: %d %s", where, l.Code, l.Raw)
		}
		for _, row := range arr(l.Body["data"]) {
			if str(row.(M)["id"]) == want {
				return true
			}
		}
		return false
	}
	if !has("false", id) || has("true", id) {
		t.Error("the new product must be listed under isSet=false only")
	}
	if !has("true", fx.ProductA3.Hex()) || has("false", fx.ProductA3.Hex()) {
		t.Error("the fixture's set must be listed under isSet=true only")
	}
	if !has("false", fx.ProductA1.Hex()) {
		t.Error("an ordinary fixture product must be listed under isSet=false")
	}
	if l := call(t, "GET", "/products?storeId="+storeA()+"&where.isSet=maybe", tok, nil); l.Code != 400 {
		t.Errorf("where.isSet=maybe: %d", l.Code)
	}
}
