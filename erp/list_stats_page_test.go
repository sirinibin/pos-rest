package erp

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestParseStatsQuery_Page(t *testing.T) {
	cases := []struct {
		qs          string
		page, limit int
		sort, err   string
	}{
		{qs: "", page: 0, limit: 0, sort: ""},
		{qs: "page=1", page: 1, limit: 25, sort: "-date"},
		{qs: "page=3&limit=50&sort=net", page: 3, limit: 50, sort: "net"},
		{qs: "page=2&sort=-customerName", page: 2, limit: 25, sort: "-customerName"},
		{qs: "page=1&dateKey=createdAt", page: 1, limit: 25, sort: "-createdAt"},
		{qs: "page=1&limit=500", page: 1, limit: 500, sort: "-date"},
		{qs: "page=0", err: "page"},
		{qs: "page=x", err: "page"},
		{qs: "page=1&limit=501", err: "limit"},
		{qs: "page=1&limit=0", err: "limit"},
		{qs: "page=1&lines=payments", err: "page"},
	}
	for _, c := range cases {
		q, err := parseStatsQuery(httptest.NewRequest("GET", "/x?"+c.qs, nil))
		if c.err != "" {
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), c.err) {
				t.Errorf("%q: err %v, want one about %s", c.qs, err, c.err)
			}
			continue
		}
		if err != nil || q.Page != c.page || q.Limit != c.limit || q.Sort != c.sort {
			t.Errorf("%q: page %d limit %d sort %q err %v", c.qs, q.Page, q.Limit, q.Sort, err)
		}
	}
}

func TestPageIDs(t *testing.T) {
	row := func(id, date, name string, net float64, status, zatca string) statRow {
		r := statRow{ID: id, Zatca: zatca, Vals: map[string]interface{}{"date": date}}
		if name != "" {
			r.Vals["customerName"] = name
		}
		r.T.Net, r.T.Status = net, status
		return r
	}
	hits := []statRow{
		row("a", "2026-01-02T10:00", "beta", 50, "paid", "reported"),
		row("b", "2026-03-01T09:00", "Alpha", 10, "unpaid", "not_reported"),
		row("c", "2025-12-31T23:59", "", 99.5, "partial", "cleared"),
		row("d", "2026-03-01T09:00", "gamma", 10, "paid", "failed"),
	}
	cases := []struct {
		sort        string
		page, limit int
		want        []string
	}{
		{"-date", 1, 25, []string{"d", "b", "a", "c"}}, // same date: id descending too
		{"date", 1, 25, []string{"c", "a", "b", "d"}},
		{"-date", 1, 2, []string{"d", "b"}},
		{"-date", 2, 2, []string{"a", "c"}},
		{"-date", 3, 2, []string{}},
		{"net", 1, 25, []string{"b", "d", "a", "c"}},
		{"-net", 1, 25, []string{"c", "a", "d", "b"}},
		{"customerName", 1, 25, []string{"c", "b", "a", "d"}}, // missing first, case-blind
		{"pstatus", 1, 25, []string{"a", "d", "c", "b"}},
		{"-zatca", 1, 25, []string{"a", "b", "d", "c"}},
	}
	for _, c := range cases {
		got := pageIDs(hits, c.sort, c.page, c.limit)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s p%d/%d = %v want %v", c.sort, c.page, c.limit, got, c.want)
		}
	}
}

func TestAddUp_PageWithFilters(t *testing.T) {
	var rows []statRow
	for i, d := range []string{"2026-01-01", "2026-01-05", "2026-02-01", "2026-02-09", "2026-03-01"} {
		r := statRow{ID: string(rune('a' + i)), Party: "c1", Vals: map[string]interface{}{"date": d}}
		if i%2 == 1 {
			r.Party = "c2"
		}
		r.T.Net = float64(10 * (i + 1))
		rows = append(rows, r)
	}
	rows = append(rows, statRow{ID: "z", Deleted: true, Party: "c1", Vals: map[string]interface{}{"date": "2026-04-01"}})
	q := StatsQuery{DateKey: "date", From: "2026-01-02", Filters: map[string]string{"party": "c1,c2"}, Sums: []string{"net"},
		Page: 1, Limit: 2, Sort: "-date"}
	out := addUp(q, rows, nil)
	if out.Count != 4 || out.Sums["net"] != 140 || !reflect.DeepEqual(out.IDs, []string{"e", "d"}) {
		t.Fatalf("page 1: %+v", out)
	}
	q.Page = 2
	if out := addUp(q, rows, nil); !reflect.DeepEqual(out.IDs, []string{"c", "b"}) {
		t.Fatalf("page 2: %v", out.IDs)
	}
	q.Filters = map[string]string{"party": "c2"}
	q.Page = 1
	if out := addUp(q, rows, nil); out.Count != 2 || !reflect.DeepEqual(out.IDs, []string{"d", "b"}) {
		t.Fatalf("one party: %+v", out)
	}
	// no page asked: no ids
	q.Page = 0
	if out := addUp(q, rows, nil); out.IDs != nil {
		t.Fatalf("ids without page: %v", out.IDs)
	}
}

func TestStatFields_Sort(t *testing.T) {
	f := statFields(StatsQuery{DateKey: "date", Sort: "-customerName", Page: 1})
	if !f["customerName"] {
		t.Fatalf("sort field not read: %v", f)
	}
	for _, k := range []string{"net", "-balance", "pstatus", "zatca"} {
		f := statFields(StatsQuery{DateKey: "date", Sort: k, Page: 1})
		if f[strings.TrimPrefix(k, "-")] {
			t.Fatalf("computed sort %s read as a field", k)
		}
	}
}

// A list screen pages its documents with /stats?page=: the ids of every page, read
// with ?ids=, are each sale once, newest first, and match the count and filters.
func TestAPI_ListStats_PageIDs(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	all := call(t, "GET", "/sales?storeId="+storeA()+"&limit=500", tok, nil)
	n := len(all.data())
	if n < 2 {
		t.Fatalf("fixture sales: %d", n)
	}
	seen := map[string]bool{}
	last := "9999"
	for page := 1; page <= n; page++ {
		r := call(t, "GET", "/sales/stats?storeId="+storeA()+"&sum=net&page="+itoa(page)+"&limit=1", tok, nil)
		if r.Code != 200 || int(num(r.Body["count"])) != n {
			t.Fatalf("page %d: %d %s", page, r.Code, r.Raw)
		}
		ids := arr(r.Body["ids"])
		if len(ids) != 1 {
			t.Fatalf("page %d ids %v", page, ids)
		}
		id := str(ids[0])
		if seen[id] {
			t.Fatalf("id %s twice", id)
		}
		seen[id] = true
		got := call(t, "GET", "/sales?storeId="+storeA()+"&ids="+id+"&select=date,code", tok, nil)
		if len(got.data()) != 1 {
			t.Fatalf("read %s: %s", id, got.Raw)
		}
		d := str(got.data()[0].(M)["date"])
		if d > last {
			t.Fatalf("not newest first: %s after %s", d, last)
		}
		last = d
	}
	// past the end: no ids, same count
	end := call(t, "GET", "/sales/stats?storeId="+storeA()+"&page="+itoa(n+1)+"&limit=1", tok, nil)
	if len(arr(end.Body["ids"])) != 0 || int(num(end.Body["count"])) != n {
		t.Fatalf("past the end: %s", end.Raw)
	}
	// a filter narrows the pages like the tiles
	f := call(t, "GET", "/sales/stats?storeId="+storeA()+"&f.party="+fx.CustomerA2.Hex()+"&page=1&sort=-net", tok, nil)
	if int(num(f.Body["count"])) != 1 || len(arr(f.Body["ids"])) != 1 {
		t.Fatalf("party page: %s", f.Raw)
	}
	if bad := call(t, "GET", "/sales/stats?storeId="+storeA()+"&page=1&limit=9999", tok, nil); bad.Code != 400 {
		t.Fatalf("limit too big: %d", bad.Code)
	}
}
