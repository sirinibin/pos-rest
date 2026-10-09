package erp

import (
	"net/http"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// POS product grids page products from the server (?page=&limit=50) instead
// of downloading the whole catalog. These filters and the facets endpoint let a
// terminal show its filter chips without the full list:
//
//	GET /products?where.forTerminal=industrial&where.isService=false&where.categoryId=…&where.specSize=…&q=…&page=1&limit=50
//	GET /products/facets?where.forTerminal=industrial&where.isService=false&where.isSet=false
//	  → {"categoryId":[{"id":…,"count":n}], "brandId":[…], "specClass":[…], "specSize":[…], "specMaterial":[…], "specType":[…], "posSection":[…], "total":n}
//
// forTerminal:  products tagged for the terminal (posTerminal) or not tagged at all.
// onlyTerminal: products tagged for the terminal.
// Both leave out the terminal's "__misc" product (it carries POS lines that
// have no product of their own and is never a tile).
// section:      a POS tab (comma list): the product's posSection, or one of
//               its categories, is one of the values.
// sectionNot:   the "Other" tab: neither its posSection nor a category is one
//               of the values.

const posMiscKey = "__misc"

var notMisc = bson.M{"erp.x.posKey": bson.M{"$ne": posMiscKey}}

func productForTerminal(val, _ string) (bson.M, bool) {
	if !posTerminals[val] {
		return nil, false
	}
	return bson.M{"$and": bson.A{
		bson.M{"$or": bson.A{
			bson.M{"erp.x.posTerminal": val},
			bson.M{"erp.x.posTerminal": bson.M{"$in": bson.A{nil, ""}}},
		}},
		notMisc,
	}}, true
}

func productOnlyTerminal(val, _ string) (bson.M, bool) {
	if !posTerminals[val] {
		return nil, false
	}
	return bson.M{"$and": bson.A{bson.M{"erp.x.posTerminal": val}, notMisc}}, true
}

// sectionValues splits a comma list of POS section keys (rePosToken each, at
// most 100) into the values and the ones that are also category ids.
func sectionValues(val string) (bson.A, bson.A, bool) {
	keys, oids := bson.A{}, bson.A{}
	for _, v := range strings.Split(val, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if !rePosToken.MatchString(v) {
			return nil, nil, false
		}
		keys = append(keys, v)
		if id, err := primitive.ObjectIDFromHex(v); err == nil {
			oids = append(oids, id)
		}
	}
	if len(keys) == 0 || len(keys) > 100 {
		return nil, nil, false
	}
	return keys, oids, true
}

// posKey: one key or a comma list (the demo items a terminal looks up).
func productPosKeys(val, _ string) (bson.M, bool) {
	keys, _, ok := sectionValues(val)
	if !ok {
		return nil, false
	}
	return bson.M{"erp.x.posKey": bson.M{"$in": keys}}, true
}

func productSection(val, _ string) (bson.M, bool) {
	keys, oids, ok := sectionValues(val)
	if !ok {
		return nil, false
	}
	or := bson.A{bson.M{"erp.x.posSection": bson.M{"$in": keys}}}
	if len(oids) > 0 {
		or = append(or, bson.M{"category_id": bson.M{"$in": oids}})
	}
	return bson.M{"$or": or}, true
}

func productSectionNot(val, _ string) (bson.M, bool) {
	keys, oids, ok := sectionValues(val)
	if !ok {
		return nil, false
	}
	and := bson.A{bson.M{"erp.x.posSection": bson.M{"$nin": keys}}}
	if len(oids) > 0 {
		and = append(and, bson.M{"category_id": bson.M{"$nin": oids}})
	}
	return bson.M{"$and": and}, true
}

// productPosWhere are the POS filters added to the products list (listWhere).
var productPosWhere = map[string]whereKey{
	"forTerminal":  {fn: productForTerminal},
	"onlyTerminal": {fn: productOnlyTerminal},
	"section":      {fn: productSection},
	"sectionNot":   {fn: productSectionNot},
	"posSection":   {key: "erp.x.posSection"},
	"posTerminal":  {key: "erp.x.posTerminal"},
	"posKey":       {fn: productPosKeys},
	"specClass":    {key: "erp.x.specs.class"},
	"specSize":     {key: "erp.x.specs.size"},
	"specMaterial": {key: "erp.x.specs.material"},
	"specType":     {key: "erp.x.specs.type"},
	// Jewellery terminal: metal (gold, silver, platinum) and purity (22K, 925 …)
	"jewelMetal":  {key: "erp.x.jewel.metal"},
	"jewelPurity": {key: "erp.x.jewel.purity"},
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
	{"specType", "erp.x.specs.type", false},
	{"posSection", "erp.x.posSection", false},
	{"jewelPurity", "erp.x.jewel.purity", false},
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
	facets["total"] = bson.A{bson.M{"$count": "n"}}
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
	total := int64(0)
	if t := arr(doc["total"]); len(t) > 0 {
		if tm, ok := t[0].(bson.M); ok {
			total = int64(num(tm["n"]))
		} else if tm, ok := t[0].(M); ok {
			total = int64(num(tm["n"]))
		}
	}
	out["total"] = total
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
