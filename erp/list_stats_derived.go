package erp

// list_stats_derived.go — values a list works out from a record and the current time,
// so /stats can filter, sort and count by them like the web app does:
//
//   qstatus  a quotation's status, "expired" once its validity ran out
//            (conversions.js quotationStatus: accepted / rejected / cancelled stay)
//   expiry   when a quotation runs out: its date + validityDays days (store wall clock)
//   late     "y" for a delivery note still pending after its estimated delivery
//
// expiry is read with the record (it does not change with time); qstatus and late are
// worked out per request from the store's current time (StatsQuery.Now).

import (
	"strings"
	"time"
)

const layoutWall = "2006-01-02T15:04"

// derivedNeeds: the record fields each per-request value reads.
var derivedNeeds = map[string][]string{
	"qstatus": {"status", "expiry"},
	"late":    {"status", "estDelivery"},
}

// quotationExpiry: date + validityDays days as a store wall-clock time ("" when the
// date is unreadable).
func quotationExpiry(date string, validityDays float64) string {
	d := strings.TrimSpace(date)
	if len(d) > 16 {
		d = d[:16]
	}
	var t time.Time
	var err error
	if len(d) == 10 {
		t, err = time.Parse(layoutDay, d)
	} else {
		t, err = time.Parse(layoutWall, d)
	}
	if err != nil {
		return ""
	}
	return t.Add(time.Duration(validityDays * float64(24*time.Hour))).Format(layoutWall)
}

// quotationStatusAt is conversions.js quotationStatus at store time now.
func quotationStatusAt(status, expiry, now string) string {
	switch status {
	case "accepted", "rejected", "cancelled", "expired":
		return status
	}
	if expiry != "" && expiry < now {
		return "expired"
	}
	return status
}

// deliveryLateAt: a pending delivery note whose estimated delivery has passed.
func deliveryLateAt(status, est, now string) string {
	e := strings.TrimSpace(est)
	if len(e) > 16 {
		e = e[:16]
	}
	if status == "pending" && e != "" && e < now {
		return "y"
	}
	return "n"
}

// derivedOf lists the per-request values q refers to (filters, sort, sum conditions,
// group).
func derivedOf(q StatsQuery) []string {
	var out []string
	add := func(k string) {
		if _, ok := derivedNeeds[k]; ok {
			for _, have := range out {
				if have == k {
					return
				}
			}
			out = append(out, k)
		}
	}
	for k := range q.Filters {
		add(k)
	}
	add(strings.TrimPrefix(q.Sort, "-"))
	add(q.GroupBy)
	for _, s := range q.Sums {
		_, cf, _ := splitMeasure(s)
		add(cf)
	}
	return out
}

// withDerived returns r with the per-request values set (a copy: cached rows are
// shared between requests).
func withDerived(r statRow, keys []string, now string) statRow {
	vals := make(map[string]interface{}, len(r.Vals)+len(keys))
	for k, v := range r.Vals {
		vals[k] = v
	}
	for _, k := range keys {
		switch k {
		case "qstatus":
			vals[k] = quotationStatusAt(jsString(r.Vals["status"]), jsString(r.Vals["expiry"]), now)
		case "late":
			vals[k] = deliveryLateAt(jsString(r.Vals["status"]), jsString(r.Vals["estDelivery"]), now)
		}
	}
	r.Vals = vals
	return r
}
