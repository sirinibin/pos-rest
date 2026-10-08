package erp

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestProductForTerminal(t *testing.T) {
	m, ok := productForTerminal("industrial", "")
	if !ok {
		t.Fatal("industrial is a terminal")
	}
	or := m["$or"].(bson.A)
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

func TestProductPosWhereRegistered(t *testing.T) {
	b := newProductsResource().Backend.(*legacyBackend)
	for k, key := range map[string]string{"specClass": "erp.x.specs.class", "specSize": "erp.x.specs.size",
		"specMaterial": "erp.x.specs.material", "posSection": "erp.x.posSection"} {
		if b.listWhere[k].key != key {
			t.Errorf("where.%s → %q", k, b.listWhere[k].key)
		}
	}
	if b.listWhere["forTerminal"].fn == nil || b.listWhere["categoryId"].key != "category_id" {
		t.Error("forTerminal / categoryId filters missing")
	}
}

func TestFacetPipelineAndResult(t *testing.T) {
	p := facetPipeline(bson.M{"x": 1})
	if len(p) != 2 || p[0].(bson.M)["$match"].(bson.M)["x"] != 1 {
		t.Fatalf("pipeline: %v", p)
	}
	fc := p[1].(bson.M)["$facet"].(bson.M)
	if len(fc) != 5 {
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
		"categoryId": bson.A{bson.M{"_id": oid, "n": int32(3)}, bson.M{"_id": nil, "n": 9}},
		"specSize":   bson.A{bson.M{"_id": "psp_s1", "n": int64(2)}},
	})
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
	if len(facetResult(bson.M{})) != 5 {
		t.Error("every facet is present, empty")
	}
}
