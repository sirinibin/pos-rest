package erp

import (
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// Server-side paging for list screens (on top of limit, page, from, q, ids):
//
//	GET /sales?from=2026-01-01&to=2026-10-07&sort=-date&where.customerId=64f…&where.paymentStatus=not_paid&page=2&limit=25
//
// to   — last day included (a zone-less date is the whole day in the store's time zone).
// sort — a contract field, "-" for newest/largest first. Only the fields a resource
//        lists in listSort can be sorted on; ties are broken by id in the same order.
// where.<field> — equality on a contract field the resource lists in listWhere.
// min.<field> / max.<field> — numeric range on a field the resource lists in listRange.
//
// Legacy keys may hold "{store}", replaced by the store id (per-store product fields).
//
// The web app uses these so a list screen asks for one page of rows instead of
// downloading every record.  An unsupported sort or where field is a 400, never
// silently ignored (the screen would show the wrong rows).

const (
	maxWhereLen  = 100
	maxSumFields = 10
)

// whereKey maps a contract filter field to the legacy document key.
type whereKey struct {
	key string
	oid bool // value is a record id (stored as ObjectID)
	// fn builds the filter itself (derived fields such as "over credit limit");
	// false = the value is not one this filter accepts (400).
	fn func(val string) (bson.M, bool)
}

// storeKey fills the {store} placeholder of a legacy key.
func storeKey(key, storeHex string) string { return strings.ReplaceAll(key, "{store}", storeHex) }

// parseListExtras reads ?to=, ?sort= and ?where.<field>= into q.
func parseListExtras(v url.Values, res *Resource, q *ListQuery) error {
	if s := strings.TrimSpace(v.Get("to")); s != "" && res.DateField != "" {
		t, err := parseClientTime(s)
		if err != nil {
			return errBadRequest("Invalid to date.", map[string]string{"to": "expected YYYY-MM-DD"})
		}
		q.To, q.ToRaw = &t, s
	}
	if s := strings.TrimSpace(v.Get("sum")); s != "" {
		for _, f := range strings.Split(s, ",") {
			if f = strings.TrimSpace(f); f != "" {
				q.Sum = append(q.Sum, f)
			}
		}
		if len(q.Sum) > maxSumFields {
			return errBadRequest("Too many totals.", map[string]string{"sum": "at most 10 fields"})
		}
	}
	if s := strings.TrimSpace(v.Get("sort")); s != "" {
		q.Desc = strings.HasPrefix(s, "-")
		q.Sort = strings.TrimSpace(strings.TrimPrefix(s, "-"))
		if q.Sort == "" {
			return errBadRequest("Invalid sort.", map[string]string{"sort": "expected a field name, - for descending"})
		}
	}
	for k, vals := range v {
		if len(vals) == 0 {
			continue
		}
		for _, pre := range []string{"min.", "max."} {
			if !strings.HasPrefix(k, pre) || strings.TrimSpace(vals[0]) == "" {
				continue
			}
			n, err := strconv.ParseFloat(strings.TrimSpace(vals[0]), 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return errBadRequest("Invalid number.", map[string]string{k: "must be a number"})
			}
			if q.Range == nil {
				q.Range = map[string][2]*float64{}
			}
			r := q.Range[strings.TrimPrefix(k, pre)]
			if pre == "min." {
				r[0] = &n
			} else {
				r[1] = &n
			}
			q.Range[strings.TrimPrefix(k, pre)] = r
		}
		if !strings.HasPrefix(k, "where.") {
			continue
		}
		f := strings.TrimPrefix(k, "where.")
		val := strings.TrimSpace(vals[0])
		if f == "" || val == "" {
			continue
		}
		if len(val) > maxWhereLen {
			return errBadRequest("Filter value is too long.", map[string]string{k: "at most 100 characters"})
		}
		if q.Where == nil {
			q.Where = map[string]string{}
		}
		q.Where[f] = val
	}
	return nil
}

// toIn is the exclusive upper bound for ?to=: a date-only value means the end
// of that day in loc; a date-time is used as given (inclusive, +1ns).
func (q ListQuery) toIn(loc *time.Location) *time.Time {
	if q.ToRaw == "" {
		return q.To
	}
	t, err := parseClientTimeIn(loc, q.ToRaw)
	if err != nil {
		return q.To
	}
	if len(strings.TrimSpace(q.ToRaw)) <= 10 {
		t = t.AddDate(0, 0, 1)
	} else {
		t = t.Add(time.Nanosecond)
	}
	return &t
}

// listExtras turns ?to=, ?where. and ?sort= into a filter and a sort for a
// legacy collection.  sortD is nil when no ?sort= was given.
func (b *legacyBackend) listExtras(c *Ctx, storeHex string, q ListQuery) (bson.M, bson.D, error) {
	var f bson.M
	if to := q.toIn(c.storeLoc(storeHex)); to != nil && b.dateKey != "" {
		f = andFilter(f, bson.M{b.dateKey: bson.M{"$lt": *to}})
	}
	for field, val := range q.Where {
		wk, ok := b.listWhere[field]
		if !ok {
			return nil, nil, errBadRequest("This list cannot be filtered by "+field+".",
				map[string]string{"where." + field: "not supported for this resource"})
		}
		if wk.fn != nil {
			m, ok := wk.fn(val)
			if !ok {
				return nil, nil, errBadRequest("Invalid filter value.", map[string]string{"where." + field: "unsupported value"})
			}
			f = andFilter(f, m)
			continue
		}
		key := storeKey(wk.key, storeHex)
		if wk.oid {
			in := bson.A{val}
			if oid, ok := oidOf(val); ok {
				in = append(bson.A{oid}, in...)
			}
			f = andFilter(f, bson.M{key: bson.M{"$in": in}})
		} else {
			f = andFilter(f, bson.M{key: val})
		}
	}
	for field, r := range q.Range {
		key, ok := b.listRange[field]
		if !ok {
			return nil, nil, errBadRequest("This list cannot be filtered by "+field+".",
				map[string]string{"min." + field: "not supported for this resource"})
		}
		cond := bson.M{}
		if r[0] != nil {
			cond["$gte"] = *r[0]
		}
		if r[1] != nil {
			cond["$lte"] = *r[1]
		}
		f = andFilter(f, bson.M{storeKey(key, storeHex): cond})
	}
	if q.Sort == "" {
		return f, nil, nil
	}
	key, ok := b.listSort[q.Sort]
	if !ok && b.listSort == nil && b.dateKey != "" {
		// every dated list sorts by date and number
		key, ok = map[string]string{"date": b.dateKey, "code": "code"}[q.Sort]
	}
	if !ok {
		return nil, nil, errBadRequest("This list cannot be sorted by "+q.Sort+".",
			map[string]string{"sort": "not supported for this resource"})
	}
	dir := 1
	if q.Desc {
		dir = -1
	}
	sortD := bson.D{}
	if key != "_id" {
		sortD = append(sortD, bson.E{Key: storeKey(key, storeHex), Value: dir})
	}
	return f, append(sortD, bson.E{Key: "_id", Value: dir}), nil
}

// Sort and filter fields of the document lists (sales, purchases, returns, …).
func docListSort(party string) map[string]string {
	m := map[string]string{"date": "date", "code": "code", "netTotal": "net_total", "total": "total",
		"vat": "vat_price", "balance": "balance_amount", "createdAt": "created_at"}
	switch party {
	case "customer":
		m["customerName"] = "customer_name"
		m["paid"] = "total_payment_received"
	case "vendor":
		m["vendorName"] = "vendor_name"
		m["paid"] = "total_payment_paid"
	}
	return m
}

func docListWhere(party string) map[string]whereKey {
	m := map[string]whereKey{"paymentStatus": {key: "payment_status"}}
	switch party {
	case "customer":
		m["customerId"] = whereKey{key: "customer_id", oid: true}
	case "vendor":
		m["vendorId"] = whereKey{key: "vendor_id", oid: true}
	}
	return m
}

func docSearchKeys(party string) []string {
	switch party {
	case "customer":
		return []string{"code", "customer_name", "customer_name_arabic", "phone", "vat_no"}
	case "vendor":
		return []string{"code", "vendor_name", "vendor_name_arabic", "vendor_invoice_no", "phone", "vat_no"}
	}
	return []string{"code"}
}

// Customers and vendors: sort, filters and ranges of the party lists.
var partyListSort = map[string]string{"code": "code", "nameEn": "name", "nameAr": "name_in_arabic", "phone": "phone",
	"vatNo": "vat_no", "creditLimit": "credit_limit", "creditBalance": "credit_balance"}

var partyListRange = map[string]string{"creditLimit": "credit_limit", "creditBalance": "credit_balance"}

// overLimit: a credit limit is set and the balance is above it.
func partyOverLimit(val string) (bson.M, bool) {
	switch val {
	case "true", "1":
		return bson.M{"credit_limit": bson.M{"$gt": 0}, "$expr": bson.M{"$gt": bson.A{"$credit_balance", "$credit_limit"}}}, true
	case "false", "0":
		return bson.M{"$or": bson.A{bson.M{"credit_limit": bson.M{"$not": bson.M{"$gt": 0}}},
			bson.M{"$expr": bson.M{"$lte": bson.A{"$credit_balance", "$credit_limit"}}}}}, true
	}
	return nil, false
}

// employee status lives in is_active (missing = active).
func employeeStatus(val string) (bson.M, bool) {
	switch val {
	case "active":
		return bson.M{"is_active": bson.M{"$ne": false}}, true
	case "inactive":
		return bson.M{"is_active": false}, true
	}
	return nil, false
}

// summer is a backend that totals numeric fields over every record a list
// request matches (?sum=creditBalance,creditLimit -> "sums" next to "total"),
// so stat tiles need no download of the whole list.
type summer interface {
	Sums(c *Ctx, storeHex string, q ListQuery) (M, error)
}

// Sums totals the ?sum= fields (those in listRange) over every match.
func (b *legacyBackend) Sums(c *Ctx, storeHex string, q ListQuery) (M, error) {
	if b.orgOverStores || b.mainOrg {
		return nil, errBadRequest("This list has no totals.", map[string]string{"sum": "not supported for this resource"})
	}
	group := bson.M{"_id": nil}
	names := map[string]string{}
	for i, f := range q.Sum {
		key, ok := b.listRange[f]
		if !ok {
			return nil, errBadRequest("This list has no total for "+f+".", map[string]string{"sum": f + " is not supported"})
		}
		n := "s" + strconv.Itoa(i)
		names[n] = f
		group[n] = bson.M{"$sum": bson.M{"$convert": bson.M{"input": "$" + storeKey(key, storeHex), "to": "double", "onError": 0.0, "onNull": 0.0}}}
	}
	f, _, err := b.listFilter(c, storeHex, q)
	if err != nil {
		return nil, err
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := b.col(storeHex).Aggregate(ctx, bson.A{bson.M{"$match": f}, bson.M{"$group": group}})
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	out := M{}
	for _, f := range q.Sum {
		out[f] = 0.0
	}
	if cur.Next(ctx) {
		var row bson.M
		if err := cur.Decode(&row); err != nil {
			return nil, errInternal("db: " + err.Error())
		}
		for n, f := range names {
			out[f] = roundN(num(row[n]), 2)
		}
	}
	return out, cur.Err()
}

// Saudi national ids start with 1, iqamas (expats) with 2 (the web app's rule).
func employeeNationality(val string) (bson.M, bool) {
	switch val {
	case "saudi":
		return bson.M{"iqama_no": bson.M{"$regex": "^1"}}, true
	case "expat":
		return bson.M{"iqama_no": bson.M{"$regex": "^2"}}, true
	}
	return nil, false
}
