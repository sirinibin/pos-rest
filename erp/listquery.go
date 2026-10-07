package erp

import (
	"net/url"
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
//
// The web app uses these so a list screen asks for one page of rows instead of
// downloading every record.  An unsupported sort or where field is a 400, never
// silently ignored (the screen would show the wrong rows).

const maxWhereLen = 100

// whereKey maps a contract filter field to the legacy document key.
type whereKey struct {
	key string
	oid bool // value is a record id (stored as ObjectID)
}

// parseListExtras reads ?to=, ?sort= and ?where.<field>= into q.
func parseListExtras(v url.Values, res *Resource, q *ListQuery) error {
	if s := strings.TrimSpace(v.Get("to")); s != "" && res.DateField != "" {
		t, err := parseClientTime(s)
		if err != nil {
			return errBadRequest("Invalid to date.", map[string]string{"to": "expected YYYY-MM-DD"})
		}
		q.To, q.ToRaw = &t, s
	}
	if s := strings.TrimSpace(v.Get("sort")); s != "" {
		q.Desc = strings.HasPrefix(s, "-")
		q.Sort = strings.TrimSpace(strings.TrimPrefix(s, "-"))
		if q.Sort == "" {
			return errBadRequest("Invalid sort.", map[string]string{"sort": "expected a field name, - for descending"})
		}
	}
	for k, vals := range v {
		if !strings.HasPrefix(k, "where.") || len(vals) == 0 {
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
		if wk.oid {
			in := bson.A{val}
			if oid, ok := oidOf(val); ok {
				in = append(bson.A{oid}, in...)
			}
			f = andFilter(f, bson.M{wk.key: bson.M{"$in": in}})
		} else {
			f = andFilter(f, bson.M{wk.key: val})
		}
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
		sortD = append(sortD, bson.E{Key: key, Value: dir})
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
