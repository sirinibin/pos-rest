package erp

import (
	"net/url"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSearchWords(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"valve", []string{"valve"}},
		{`  gate   valve 2" `, []string{"gate", "valve", `2"`}},
		{"Valve valve VALVE ss", []string{"Valve", "ss"}},
		{"a b c d e f g h i j", []string{"a", "b", "c", "d", "e", "f", "g", "h"}},
		{"صمام  بوابة", []string{"صمام", "بوابة"}},
	} {
		if got := searchWords(tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("searchWords(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}

func TestProductSearchWhere(t *testing.T) {
	if productSearchWhere("  ", productSearchLookups{}) != nil {
		t.Fatal("empty q must give no filter")
	}
	one := productSearchWhere("a.b", productSearchLookups{})
	or := one["$or"].(bson.A)
	if len(or) != len(productSearchKeys) {
		t.Fatalf("one word: %d clauses, want %d", len(or), len(productSearchKeys))
	}
	// escaped, case-insensitive
	if rx := or[0].(bson.M)["name"].(primitive.Regex); rx.Pattern != `a\.b` || rx.Options != "i" {
		t.Fatalf("regex %v", rx)
	}
	for i, k := range []string{"name", "name_in_arabic", "item_code", "part_number", "prefix_part_number", "bar_code",
		"additional_keywords", "country_name"} {
		if _, ok := or[i].(bson.M)[k]; !ok {
			t.Errorf("clause %d is not %s: %v", i, k, or[i])
		}
	}

	b1 := primitive.NewObjectID()
	lk := productSearchLookups{
		brands: map[string][]primitive.ObjectID{"kitz": {b1}},
		specs:  map[string]map[string][]string{"ss316": {"material": {"psp_m1"}}, "kitz": {"type": {"psp_t1"}}},
	}
	two := productSearchWhere("kitz ss316", lk)
	and := two["$and"].(bson.A)
	if len(and) != 2 {
		t.Fatalf("two words: %v", two)
	}
	has := func(f bson.M, key string, want interface{}) bool {
		for _, c := range f["$or"].(bson.A) {
			if v, ok := c.(bson.M)[key]; ok && reflect.DeepEqual(v, bson.M{"$in": want}) {
				return true
			}
		}
		return false
	}
	if !has(and[0].(bson.M), "brand_id", []primitive.ObjectID{b1}) || !has(and[0].(bson.M), "erp.x.specs.type", []string{"psp_t1"}) {
		t.Errorf("brand word: %v", and[0])
	}
	if !has(and[1].(bson.M), "erp.x.specs.material", []string{"psp_m1"}) {
		t.Errorf("spec word: %v", and[1])
	}
	// the Arabic name of a country of origin finds its English name
	if !has(productSearchWhere("الصين", productSearchLookups{}), "country_name", []string{"China"}) {
		t.Error("Arabic country name")
	}
	// an English country name needs no extra clause (the regex on country_name matches)
	for _, c := range productSearchWhere("china", productSearchLookups{})["$or"].(bson.A) {
		if v, ok := c.(bson.M)["country_name"].(bson.M); ok {
			t.Errorf("unexpected $in clause %v", v)
		}
	}
}

func TestAPI_Products_Search(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "prodsearch")
	defer cleanupStore(t, sid)
	post := func(path string, body M) M {
		r := call(t, "POST", path+"?storeId="+sid, owner, body)
		if r.Code != 201 && r.Code != 200 {
			t.Fatalf("POST %s: %d %s", path, r.Code, r.Raw)
		}
		return r.Body
	}
	spec := func(kind, name, ar string) string {
		return str(post("/product-specs", M{"storeId": sid, "kind": kind, "name": name, "nameAr": ar})["id"])
	}
	brand := str(post("/brands", M{"storeId": sid, "name": "Kitz"})["id"])
	typ := spec("type", "Gate valve", "صمام بوابة")
	size := spec("size", `2"`, "")
	mat := spec("material", "SS316", "")
	mk := func(name string, extra M) {
		body := M{"storeId": sid, "nameEn": name, "unit": "Pcs", "pricing": M{"retail": 10}}
		for k, v := range extra {
			body[k] = v
		}
		post("/products", body)
	}
	mk("Valve A", M{"partNo": "GV-150-2", "brandId": brand, "country": "China",
		"specs": M{"type": typ, "size": size, "material": mat}})
	mk("Flange B", M{"nameAr": "فلنجة", "partNo": "WN-300", "country": "Japan"})
	mk("Gasket C", M{"partNo": "SW-1", "specs": M{"size": size}})

	names := func(q string) []string {
		r := call(t, "GET", "/products?storeId="+sid+"&sort=nameEn&limit=50&select=nameEn&q="+url.QueryEscape(q), owner, nil)
		if r.Code != 200 {
			t.Fatalf("q=%s: %d %s", q, r.Code, r.Raw)
		}
		var out []string
		for _, p := range arr(r.Body["data"]) {
			out = append(out, str(p.(M)["nameEn"]))
		}
		return out
	}
	for q, want := range map[string][]string{
		"gv-150":        {"Valve A"},             // part no.
		"wn-300":        {"Flange B"},            // part no.
		"فلنجة":         {"Flange B"},            // Arabic name
		"kitz":          {"Valve A"},             // brand name
		"gate valve":    {"Valve A"},             // type name (two words)
		"صمام":          {"Valve A"},             // type's Arabic name
		`2"`:            {"Gasket C", "Valve A"}, // size
		"ss316":         {"Valve A"},             // material
		"china":         {"Valve A"},             // country
		"اليابان":       {"Flange B"},            // country, Arabic
		"wn300":         {"Flange B"},            // part no. without its dashes (legacy additional_keywords)
		`kitz 2" ss316`: {"Valve A"},             // words across fields, any order
		`ss316 gasket`:  nil,                     // every word must match
		"no such thing": nil,
	} {
		if got := names(q); !reflect.DeepEqual(got, want) {
			t.Errorf("q=%q: %v, want %v", q, got, want)
		}
	}
	// facets follow the search
	f := call(t, "GET", "/products/facets?storeId="+sid+"&q="+url.QueryEscape(`2"`), owner, nil)
	if f.Code != 200 || num(f.Body["total"]) != 2 {
		t.Fatalf("facets: %d %s", f.Code, f.Raw)
	}
}
