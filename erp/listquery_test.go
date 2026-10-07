package erp

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestParseListExtras(t *testing.T) {
	dated := &Resource{Name: "sales", Path: "sales", Scope: "store", DateField: "date"}
	undated := &Resource{Name: "customers", Path: "customers", Scope: "store"}
	cases := []struct {
		name   string
		res    *Resource
		qs     string
		sort   string
		desc   bool
		where  map[string]string
		toRaw  string
		errKey string
	}{
		{name: "nothing", res: dated},
		{name: "to date", res: dated, qs: "to=2026-10-07", toRaw: "2026-10-07"},
		{name: "to ignored without a date field", res: undated, qs: "to=2026-10-07"},
		{name: "bad to", res: dated, qs: "to=tomorrow", errKey: "to"},
		{name: "sort ascending", res: dated, qs: "sort=code", sort: "code"},
		{name: "sort descending", res: dated, qs: "sort=-date", sort: "date", desc: true},
		{name: "bare minus", res: dated, qs: "sort=-", errKey: "sort"},
		{name: "where", res: dated, qs: "where.customerId=abc&where.paymentStatus=paid",
			where: map[string]string{"customerId": "abc", "paymentStatus": "paid"}},
		{name: "empty where value skipped", res: dated, qs: "where.customerId=&where.=x"},
		{name: "where value too long", res: dated, qs: "where.customerId=" + strings.Repeat("a", 101), errKey: "where.customerId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parseListQuery(httptest.NewRequest("GET", "/v1/erp/x?"+tc.qs, nil), tc.res)
			if tc.errKey != "" {
				ae, ok := err.(*APIError)
				if !ok || ae.Status != http.StatusBadRequest || ae.Fields[tc.errKey] == "" {
					t.Fatalf("want 400 on %s, got %v", tc.errKey, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if q.Sort != tc.sort || q.Desc != tc.desc || q.ToRaw != tc.toRaw || !reflect.DeepEqual(q.Where, tc.where) {
				t.Fatalf("got %+v", q)
			}
		})
	}
}

func TestListQueryToIn(t *testing.T) {
	loc := time.FixedZone("AST", 3*3600)
	q := ListQuery{ToRaw: "2026-10-07"}
	to := q.toIn(loc)
	if want := time.Date(2026, 10, 8, 0, 0, 0, 0, loc); !to.Equal(want) {
		t.Fatalf("date-only to is the end of that day in the store zone: %v want %v", to, want)
	}
	q = ListQuery{ToRaw: "2026-10-07T10:30:00"}
	if got := q.toIn(loc); !got.Equal(time.Date(2026, 10, 7, 10, 30, 0, 1, loc)) {
		t.Fatalf("date-time to is inclusive: %v", got)
	}
	if (ListQuery{}).toIn(loc) != nil {
		t.Fatal("no to, no bound")
	}
}

func TestLegacyListExtras(t *testing.T) {
	initResources()
	sales := resourceByName("sales").Backend.(*legacyBackend)
	cust := primitive.NewObjectID()
	f, s, err := sales.listExtras(nil, "", ListQuery{Sort: "netTotal", Desc: true,
		Where: map[string]string{"customerId": cust.Hex(), "paymentStatus": "not_paid"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, bson.D{{Key: "net_total", Value: -1}, {Key: "_id", Value: -1}}) {
		t.Fatalf("sort: %v", s)
	}
	and, _ := f["$and"].(bson.A)
	if len(and) != 2 {
		t.Fatalf("filter: %v", f)
	}
	found := map[string]interface{}{}
	for _, p := range and {
		for k, v := range p.(bson.M) {
			found[k] = v
		}
	}
	if found["payment_status"] != "not_paid" {
		t.Fatalf("payment status: %v", found)
	}
	in := found["customer_id"].(bson.M)["$in"].(bson.A)
	if in[0] != cust || in[1] != cust.Hex() {
		t.Fatalf("customer id matches the ObjectID and the hex: %v", in)
	}
	// no sort: the default order stays
	if _, s, _ := sales.listExtras(nil, "", ListQuery{}); s != nil {
		t.Fatalf("default sort: %v", s)
	}
	for _, bad := range []ListQuery{{Sort: "history"}, {Where: map[string]string{"vendorId": "x"}}} {
		if _, _, err := sales.listExtras(nil, "", bad); err == nil || err.(*APIError).Status != 400 {
			t.Fatalf("%+v: want 400, got %v", bad, err)
		}
	}
	// purchases filter by vendor; every dated list sorts by date and code
	if _, _, err := resourceByName("purchases").Backend.(*legacyBackend).listExtras(nil, "", ListQuery{Where: map[string]string{"vendorId": cust.Hex()}}); err != nil {
		t.Fatal(err)
	}
	exp := resourceByName("expenses").Backend.(*legacyBackend)
	if _, s, err := exp.listExtras(nil, "", ListQuery{Sort: "date", Desc: true}); err != nil || s[0].Key != "date" {
		t.Fatalf("expenses by date: %v %v", s, err)
	}
	if _, _, err := exp.listExtras(nil, "", ListQuery{Sort: "amount"}); err == nil {
		t.Fatal("expenses by amount is not supported")
	}
}

// Paging params are read only after authentication.
func TestListPaging_Unauthenticated(t *testing.T) {
	for _, path := range []string{"/sales?sort=-date&to=2026-10-07", "/sales?where.customerId=abc", "/sales?sort=-"} {
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, httptest.NewRequest("GET", Prefix+path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

// DB-backed: one page of a document list, sorted, filtered and windowed by the server.
func TestAPI_ListPaging(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	st := "&storeId=" + storeA()
	ids := func(r resp) []string {
		out := []string{}
		for _, row := range r.data() {
			out = append(out, str(row.(M)["id"]))
		}
		return out
	}
	a1, a2 := fx.OrderA1.Hex(), fx.OrderA2.Hex()
	old := time.Now().AddDate(0, 0, -400).Format("2006-01-02")
	// other tests add sales to store A too: check the fixture invoices' place and
	// that every row returned matches
	fixture := func(got []string) []string {
		out := []string{}
		for _, id := range got {
			if id == a1 || id == a2 {
				out = append(out, id)
			}
		}
		return out
	}
	get := func(path string) resp {
		r := call(t, "GET", path+st, tok, nil)
		if r.Code != 200 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Raw)
		}
		return r
	}
	order := []struct {
		name, path string
		want       []string
	}{
		{"newest first", "/sales?sort=-date&limit=500", []string{a1, a2}},
		{"oldest first", "/sales?sort=date&limit=500", []string{a2, a1}},
		{"by number", "/sales?sort=code&limit=500", []string{a2, a1}},
		{"by customer", "/sales?where.customerId=" + fx.CustomerA2.Hex(), []string{a2}},
		{"up to a date", "/sales?to=" + old, []string{a2}},
		{"between dates", "/sales?from=" + old + "&to=" + time.Now().AddDate(0, 0, 1).Format("2006-01-02"), []string{a1}},
		{"search by number", "/sales?q=s-inv-001", []string{a1}},
		{"search by customer name", "/sales?q=walk+in", []string{a2}},
		{"by payment status", "/sales?where.paymentStatus=paid&sort=-date", []string{a1, a2}},
		{"no fixture invoice is unpaid", "/sales?where.paymentStatus=not_paid", []string{}},
	}
	for _, tc := range order {
		t.Run(tc.name, func(t *testing.T) {
			if got := fixture(ids(get(tc.path))); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	// every row matches the filter
	for _, row := range get("/sales?where.customerId=" + fx.CustomerA2.Hex()).data() {
		if str(row.(M)["customerId"]) != fx.CustomerA2.Hex() {
			t.Fatalf("other customer's invoice: %v", row.(M)["id"])
		}
	}
	for _, row := range get("/sales?where.paymentStatus=not_paid").data() {
		if ps := str(row.(M)["legacyTotals"].(M)["paymentStatus"]); ps != "not_paid" {
			t.Fatalf("payment status %q", ps)
		}
	}
	net := -1.0
	for _, row := range get("/sales?sort=-netTotal&limit=500").data() {
		v := num(row.(M)["legacyTotals"].(M)["net"])
		if net >= 0 && v > net {
			t.Fatalf("net totals not descending: %v after %v", v, net)
		}
		net = v
	}
	// pages: page by page gives the same order as one big page; total is every match
	all := ids(get("/sales?sort=-date&limit=500"))
	paged := []string{}
	for p := 1; p <= len(all); p++ {
		r := get("/sales?sort=-date&limit=1&page=" + itoa(p))
		if int(num(r.Body["total"])) != len(all) {
			t.Fatalf("total %v want %d", r.Body["total"], len(all))
		}
		paged = append(paged, ids(r)...)
	}
	if !reflect.DeepEqual(paged, all) {
		t.Fatalf("paging order differs: %v vs %v", paged, all)
	}
	for _, bad := range []struct{ path, field string }{
		{"/sales?sort=history" + st, "sort"},
		{"/sales?where.vendorId=x" + st, "where.vendorId"},
		{"/sales?to=soon" + st, "to"},
	} {
		if r := call(t, "GET", bad.path, tok, nil); r.Code != 400 || r.errField(bad.field) == "" {
			t.Errorf("%s: %d %s", bad.path, r.Code, r.Raw)
		}
	}
}
