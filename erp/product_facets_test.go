package erp

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestProductForTerminal(t *testing.T) {
	m, ok := productForTerminal("industrial", "")
	if !ok {
		t.Fatal("industrial is a terminal")
	}
	and := m["$and"].(bson.A)
	if and[1].(bson.M)["erp.x.posKey"].(bson.M)["$ne"] != "__misc" {
		t.Fatalf("misc product must be left out: %v", and[1])
	}
	or := and[0].(bson.M)["$or"].(bson.A)
	if or[0].(bson.M)["erp.x.posTerminal"] != "industrial" {
		t.Fatalf("tagged branch: %v", or[0])
	}
	if _, ok := or[1].(bson.M)["erp.x.posTerminal"].(bson.M)["$in"]; !ok {
		t.Fatalf("untagged branch: %v", or[1])
	}
	for _, bad := range []string{"", "casino", "INDUSTRIAL", "industrial "} {
		if _, ok := productForTerminal(bad, ""); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestProductOnlyTerminal(t *testing.T) {
	m, ok := productOnlyTerminal("grocery", "")
	if !ok {
		t.Fatal("grocery is a terminal")
	}
	and := m["$and"].(bson.A)
	if and[0].(bson.M)["erp.x.posTerminal"] != "grocery" || and[1].(bson.M)["erp.x.posKey"] == nil {
		t.Fatalf("onlyTerminal: %v", m)
	}
	if _, ok := productOnlyTerminal("nope", ""); ok {
		t.Error("unknown terminal accepted")
	}
}

func TestSectionValues(t *testing.T) {
	hex := primitive.NewObjectID().Hex()
	for _, c := range []struct {
		in         string
		keys, oids int
		ok         bool
	}{
		{"veg", 1, 0, true},
		{"veg, dairy ,", 2, 0, true},
		{hex + ",cat_oil", 2, 1, true},
		{"", 0, 0, false},
		{" , ", 0, 0, false},
		{"bad key", 0, 0, false},
		{"a;b", 0, 0, false},
		{strings.Repeat("a,", 101), 0, 0, false},
	} {
		k, o, ok := sectionValues(c.in)
		if ok != c.ok || len(k) != c.keys || len(o) != c.oids {
			t.Errorf("%q → %v %v %v", c.in, k, o, ok)
		}
	}
}

func TestProductSectionFilters(t *testing.T) {
	hex := primitive.NewObjectID().Hex()
	m, ok := productSection("veg,"+hex, "")
	or := m["$or"].(bson.A)
	if !ok || len(or) != 2 || or[0].(bson.M)["erp.x.posSection"].(bson.M)["$in"].(bson.A)[0] != "veg" {
		t.Fatalf("section: %v", m)
	}
	if m, _ := productSection("veg", ""); len(m["$or"].(bson.A)) != 1 {
		t.Errorf("no category branch without an id: %v", m)
	}
	n, ok := productSectionNot("veg,"+hex, "")
	and := n["$and"].(bson.A)
	if !ok || len(and) != 2 || and[0].(bson.M)["erp.x.posSection"].(bson.M)["$nin"] == nil || and[1].(bson.M)["category_id"].(bson.M)["$nin"] == nil {
		t.Fatalf("sectionNot: %v", n)
	}
	if _, ok := productSection("bad key", ""); ok {
		t.Error("bad section accepted")
	}
	if _, ok := productSectionNot("", ""); ok {
		t.Error("empty sectionNot accepted")
	}
}

func TestProductPosKeys(t *testing.T) {
	m, ok := productPosKeys("svc1, __misc", "")
	in := m["erp.x.posKey"].(bson.M)["$in"].(bson.A)
	if !ok || len(in) != 2 || in[1] != "__misc" {
		t.Fatalf("posKey: %v", m)
	}
	if _, ok := productPosKeys("bad key", ""); ok {
		t.Error("bad posKey accepted")
	}
}

func TestProductPosWhereRegistered(t *testing.T) {
	b := newProductsResource().Backend.(*legacyBackend)
	for k, key := range map[string]string{"specClass": "erp.x.specs.class", "specSize": "erp.x.specs.size",
		"specMaterial": "erp.x.specs.material", "posSection": "erp.x.posSection",
		"posTerminal": "erp.x.posTerminal", "jewelMetal": "erp.x.jewel.metal", "jewelPurity": "erp.x.jewel.purity"} {
		if b.listWhere[k].key != key {
			t.Errorf("where.%s → %q", k, b.listWhere[k].key)
		}
	}
	for _, k := range []string{"forTerminal", "onlyTerminal", "section", "sectionNot", "posKey"} {
		if b.listWhere[k].fn == nil {
			t.Errorf("where.%s missing", k)
		}
	}
	if b.listWhere["categoryId"].key != "category_id" {
		t.Error("forTerminal / categoryId filters missing")
	}
}

func TestFacetPipelineAndResult(t *testing.T) {
	p := facetPipeline(bson.M{"x": 1})
	if len(p) != 2 || p[0].(bson.M)["$match"].(bson.M)["x"] != 1 {
		t.Fatalf("pipeline: %v", p)
	}
	fc := p[1].(bson.M)["$facet"].(bson.M)
	if len(fc) != 9 || fc["total"].(bson.A)[0].(bson.M)["$count"] != "n" {
		t.Fatalf("facets: %v", fc)
	}
	if fc["categoryId"].(bson.A)[0].(bson.M)["$unwind"] != "$category_id" {
		t.Errorf("category facet must unwind: %v", fc["categoryId"])
	}
	if _, ok := fc["brandId"].(bson.A)[0].(bson.M)["$unwind"]; ok {
		t.Errorf("brand facet must not unwind")
	}
	oid := primitive.NewObjectID()
	out := facetResult(bson.M{
		"categoryId":  bson.A{bson.M{"_id": oid, "n": int32(3)}, bson.M{"_id": nil, "n": 9}},
		"specSize":    bson.A{bson.M{"_id": "psp_s1", "n": int64(2)}},
		"posSection":  bson.A{bson.M{"_id": "veg", "n": int32(4)}},
		"jewelPurity": bson.A{bson.M{"_id": "22K", "n": int32(5)}},
		"total":       bson.A{bson.M{"n": int32(12)}},
	})
	if ps := out["posSection"].([]M); len(ps) != 1 || ps[0]["id"] != "veg" || ps[0]["count"] != 4.0 {
		t.Errorf("sections: %v", ps)
	}
	if jp := out["jewelPurity"].([]M); len(jp) != 1 || jp[0]["id"] != "22K" || jp[0]["count"] != 5.0 {
		t.Errorf("jewel purities: %v", jp)
	}
	if out["total"] != int64(12) {
		t.Errorf("total: %v", out["total"])
	}
	cats := out["categoryId"].([]M)
	if len(cats) != 1 || cats[0]["id"] != oid.Hex() || cats[0]["count"] != 3.0 {
		t.Errorf("categories: %v", cats)
	}
	if sz := out["specSize"].([]M); len(sz) != 1 || sz[0]["id"] != "psp_s1" || sz[0]["count"] != 2.0 {
		t.Errorf("sizes: %v", sz)
	}
	if b := out["brandId"].([]M); len(b) != 0 {
		t.Errorf("brands: %v", b)
	}
	if e := facetResult(bson.M{}); len(e) != 9 || e["total"] != int64(0) {
		t.Error("every facet is present, empty")
	}
}
