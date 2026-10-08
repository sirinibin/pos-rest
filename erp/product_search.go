package erp

import (
	"regexp"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Product search (?q= on GET /products and /products/facets), like the sales
// form's product picker but wider:
//
//	GET /products?q=gate valve 2" ss316
//
// Every word must match one of the product's fields: name (English or
// Arabic), item code, part no. (and its prefix), barcode, additional
// keywords, country of origin (English or Arabic name), brand name, or the
// name of its type / class / size / material option (product-specs). Words
// may come in any order and match different fields. Text is escaped, never
// used as a pattern.

const maxSearchWords = 8

// productSearchKeys are the product's own text fields a word may match.
var productSearchKeys = []string{"name", "name_in_arabic", "item_code", "part_number", "prefix_part_number",
	"bar_code", "additional_keywords", "country_name"}

// countryArabic: Arabic names of the countries of origin the product form
// offers (web app src/inventory/vehicleData.js); products keep the English one.
var countryArabic = map[string]string{
	"Saudi Arabia": "السعودية", "UAE": "الإمارات", "China": "الصين", "Japan": "اليابان",
	"Germany": "ألمانيا", "USA": "الولايات المتحدة", "Korea": "كوريا", "Italy": "إيطاليا",
	"India": "الهند", "Turkey": "تركيا", "France": "فرنسا", "UK": "المملكة المتحدة",
	"Taiwan": "تايوان", "Thailand": "تايلاند",
}

// searchWords splits q into at most maxSearchWords distinct words.
func searchWords(q string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(q) {
		lw := strings.ToLower(w)
		if seen[lw] {
			continue
		}
		seen[lw] = true
		out = append(out, w)
		if len(out) == maxSearchWords {
			break
		}
	}
	return out
}

func wordRx(w string) primitive.Regex {
	return primitive.Regex{Pattern: regexp.QuoteMeta(w), Options: "i"}
}

// productSearchLookups: per word, the brands and spec options whose name has
// it (the product only holds their ids).
type productSearchLookups struct {
	brands map[string][]primitive.ObjectID // word → brand ids
	specs  map[string]map[string][]string  // word → kind → option ids
}

// productWordFilter: one word in any of the product's fields, brand, specs or
// country.
func productWordFilter(w string, lk productSearchLookups) bson.M {
	rx := wordRx(w)
	or := bson.A{}
	for _, k := range productSearchKeys {
		or = append(or, bson.M{k: rx})
	}
	lw := strings.ToLower(w)
	var countries []string
	for en, ar := range countryArabic {
		if strings.Contains(ar, w) && !strings.Contains(strings.ToLower(en), lw) {
			countries = append(countries, en)
		}
	}
	if len(countries) > 0 {
		sort.Strings(countries)
		or = append(or, bson.M{"country_name": bson.M{"$in": countries}})
	}
	if ids := lk.brands[w]; len(ids) > 0 {
		or = append(or, bson.M{"brand_id": bson.M{"$in": ids}})
	}
	for _, kind := range ProductSpecKinds {
		if ids := lk.specs[w][kind]; len(ids) > 0 {
			or = append(or, bson.M{"erp.x.specs." + kind: bson.M{"$in": ids}})
		}
	}
	return bson.M{"$or": or}
}

// productSearchWhere: every word matches (nil when q has none).
func productSearchWhere(q string, lk productSearchLookups) bson.M {
	words := searchWords(q)
	if len(words) == 0 {
		return nil
	}
	if len(words) == 1 {
		return productWordFilter(words[0], lk)
	}
	and := bson.A{}
	for _, w := range words {
		and = append(and, productWordFilter(w, lk))
	}
	return bson.M{"$and": and}
}

// productSearch is the products backend's ?q= filter: looks up the store's
// brands and spec options whose names have a word, then productSearchWhere.
func productSearch(c *Ctx, storeHex, q string) (bson.M, error) {
	words := searchWords(q)
	if len(words) == 0 {
		return nil, nil
	}
	lk := productSearchLookups{brands: map[string][]primitive.ObjectID{}, specs: map[string]map[string][]string{}}
	ctx, cancel := dbctx()
	defer cancel()
	db := storeDB(storeHex)
	for _, w := range words {
		rx := wordRx(w)
		cur, err := db.Collection("product_brand").Find(ctx,
			bson.M{"deleted": bson.M{"$ne": true}, "$or": bson.A{bson.M{"name": rx}, bson.M{"name_in_arabic": rx}}},
			options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(200))
		if err != nil {
			return nil, errInternal("db: " + err.Error())
		}
		var brands []bson.M
		if err := cur.All(ctx, &brands); err != nil {
			return nil, errInternal("db: " + err.Error())
		}
		for _, b := range brands {
			if oid, ok := b["_id"].(primitive.ObjectID); ok {
				lk.brands[w] = append(lk.brands[w], oid)
			}
		}
		cur, err = db.Collection("erp_product_spec").Find(ctx,
			bson.M{"deleted": bson.M{"$ne": true}, "_erp_hd": bson.M{"$ne": true},
				"$or": bson.A{bson.M{"name": rx}, bson.M{"nameAr": rx}}},
			options.Find().SetProjection(bson.M{"_id": 1, "kind": 1}).SetLimit(500))
		if err != nil {
			return nil, errInternal("db: " + err.Error())
		}
		var specs []bson.M
		if err := cur.All(ctx, &specs); err != nil {
			return nil, errInternal("db: " + err.Error())
		}
		for _, s := range specs {
			kind, id := str(s["kind"]), str(s["_id"])
			if kind == "" || id == "" {
				continue
			}
			if lk.specs[w] == nil {
				lk.specs[w] = map[string][]string{}
			}
			lk.specs[w][kind] = append(lk.specs[w][kind], id)
		}
	}
	return productSearchWhere(q, lk), nil
}
