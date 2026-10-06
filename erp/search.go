package erp

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Server-side search for pickers (?q= and ?ids= on list endpoints).
//
//	GET /customers?q=ahmed&limit=20&select=code,nameEn,phone
//	GET /customers?ids=64f…,64e…&select=code,nameEn
//
// q matches any of the resource's search fields, case-insensitive, as a
// substring (the text is escaped, never used as a pattern). ids returns only
// the listed records (labels for ids a screen already holds). Both combine
// with every other list parameter (limit, page, select, includeDeleted).
// The web app uses them so dropdowns no longer download whole collections.

const (
	maxSearchLen = 100
	maxSearchIDs = 200
)

// parseSearch validates ?q= and ?ids=.
func parseSearch(qs, ids string) (string, []string, error) {
	qs = strings.TrimSpace(qs)
	if utf8.RuneCountInString(qs) > maxSearchLen {
		return "", nil, errBadRequest("Search text is too long.", map[string]string{"q": "at most 100 characters"})
	}
	var out []string
	seen := map[string]bool{}
	for _, id := range strings.Split(ids, ",") {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) > maxSearchIDs {
		return "", nil, errBadRequest("Too many ids.", map[string]string{"ids": "at most 200 ids"})
	}
	return qs, out, nil
}

// searchFilter matches q (escaped, case-insensitive substring) in any of keys.
// Nil when q is empty or there are no keys.
func searchFilter(q string, keys []string) bson.M {
	if q == "" || len(keys) == 0 {
		return nil
	}
	rx := primitive.Regex{Pattern: regexp.QuoteMeta(q), Options: "i"}
	or := bson.A{}
	for _, k := range keys {
		or = append(or, bson.M{k: rx})
	}
	if len(or) == 1 {
		return or[0].(bson.M)
	}
	return bson.M{"$or": or}
}

// legacyIDsFilter matches legacy records by contract id (ObjectID hex or an
// adapter client id kept in erp.cid). Nil when ids is empty.
func legacyIDsFilter(ids []string) bson.M {
	if len(ids) == 0 {
		return nil
	}
	oids := bson.A{}
	cids := bson.A{}
	for _, id := range ids {
		if oid, ok := oidOf(id); ok {
			oids = append(oids, oid)
		}
		cids = append(cids, id)
	}
	or := bson.A{bson.M{"erp.cid": bson.M{"$in": cids}}}
	if len(oids) > 0 {
		or = append(bson.A{bson.M{"_id": bson.M{"$in": oids}}}, or...)
	}
	return bson.M{"$or": or}
}

// nativeSearchKeys are the contract fields searched on native (erp_*) collections.
var nativeSearchKeys = []string{"code", "nameEn", "nameAr", "name"}

// legacySearchKeys returns the legacy keys searched for a contract resource,
// or an error when the resource does not support ?q=.
func (b *legacyBackend) checkSearch(q ListQuery) error {
	if q.Search != "" && len(b.searchKeys) == 0 {
		return errBadRequest("This list cannot be searched.", map[string]string{"q": "not supported for this resource"})
	}
	return nil
}
