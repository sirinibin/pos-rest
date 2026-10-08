package erp

import (
	"net/http"

	"go.mongodb.org/mongo-driver/bson"
)

// POS product grids page products from the server (?page=&limit=50) instead
// of downloading the whole catalog. These filters and the facets endpoint let a
// terminal show its filter chips without the full list:
//
//	GET /products?where.forTerminal=industrial&where.isService=false&where.categoryId=…&where.specSize=…&q=…&page=1&limit=50
//	GET /products/facets?where.forTerminal=industrial&where.isService=false&where.isSet=false
//	  → {"categoryId":[{"id":…,"count":n}], "brandId":[…], "specClass":[…], "specSize":[…], "specMaterial":[…]}
//
// forTerminal: products tagged for the terminal (posTerminal) or not tagged at all.

func productForTerminal(val, _ string) (bson.M, bool) {
	if !posTerminals[val] {
		return nil, false
	}
	return bson.M{"$or": bson.A{
		bson.M{"erp.x.posTerminal": val},
		bson.M{"erp.x.posTerminal": bson.M{"$in": bson.A{nil, ""}}},
	}}, true
}

// productPosWhere are the POS filters added to the products list (listWhere).
var productPosWhere = map[string]whereKey{
	"forTerminal":  {fn: productForTerminal},
	"posSection":   {key: "erp.x.posSection"},
	"specClass":    {key: "erp.x.specs.class"},
	"specSize":     {key: "erp.x.specs.size"},
	"specMaterial": {key: "erp.x.specs.material"},
}

// productFacetFields: facet name → legacy key (category_id is an array).
var productFacetFields = []struct {
	name, key string
	array     bool
}{
	{"categoryId", "category_id", true},
	{"brandId", "brand_id", false},
	{"specClass", "erp.x.specs.class", false},
	{"specSize", "erp.x.specs.size", false},
	{"specMaterial", "erp.x.specs.material", false},
}

// facetPipeline counts the products matching f per category, brand and spec.
func facetPipeline(f bson.M) bson.A {
	facets := bson.M{}
	for _, ff := range productFacetFields {
		st := bson.A{}
		if ff.array {
			st = append(st, bson.M{"$unwind": "$" + ff.key})
		}
		st = append(st,
			bson.M{"$match": bson.M{ff.key: bson.M{"$nin": bson.A{nil, ""}}}},
			bson.M{"$group": bson.M{"_id": "$" + ff.key, "n": bson.M{"$sum": 1}}},
			bson.M{"$sort": bson.M{"n": -1}},
			bson.M{"$limit": 500},
		)
		facets[ff.name] = st
	}
	return bson.A{bson.M{"$match": f}, bson.M{"$facet": facets}}
}

// facetResult turns the $facet output into {name: [{id, count}]}.
func facetResult(doc bson.M) M {
	out := M{}
	for _, ff := range productFacetFields {
		rows := []M{}
		for _, r := range arr(doc[ff.name]) {
			rm, _ := r.(bson.M)
			if rm == nil {
				if m2, ok := r.(M); ok {
					rm = bson.M(m2)
				}
			}
			if rm == nil {
				continue
			}
			id := hexOf(rm["_id"])
			if id == "" {
				continue
			}
			rows = append(rows, M{"id": id, "count": num(rm["n"])})
		}
		out[ff.name] = rows
	}
	return out
}

func handleProductFacets(w http.ResponseWriter, r *http.Request, res *Resource) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := res.checkPerm(c, "view"); err != nil {
		writeErr(w, err)
		return
	}
	q, err := parseListQuery(r, res)
	if err != nil {
		writeErr(w, err)
		return
	}
	storeHex, err := resolveStore(c, res, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	if storeHex == "" {
		writeErr(w, errBadRequest("storeId is required.", map[string]string{"storeId": "required"}))
		return
	}
	b, ok := res.Backend.(*legacyBackend)
	if !ok {
		writeErr(w, errNotFound())
		return
	}
	f, _, err := b.listFilter(c, storeHex, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := b.col(storeHex).Aggregate(ctx, facetPipeline(f))
	if err != nil {
		writeErr(w, errInternal("db: "+err.Error()))
		return
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		writeErr(w, errInternal("db: "+err.Error()))
		return
	}
	out := M{}
	if len(docs) > 0 {
		out = facetResult(docs[0])
	} else {
		out = facetResult(bson.M{})
	}
	writeJSON(w, http.StatusOK, out)
}
