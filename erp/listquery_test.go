package erp

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		{name: "where value too long", res: dated, qs: "where.customerId=" + strings.Repeat("a", 2601), errKey: "where.customerId"},
		{name: "min not a number", res: undated, qs: "min.creditLimit=abc", errKey: "min.creditLimit"},
		{name: "max infinite", res: undated, qs: "max.creditLimit=Inf", errKey: "max.creditLimit"},
		{name: "too many totals", res: undated, qs: "sum=a,b,c,d,e,f,g,h,i,j,k", errKey: "sum"},
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

func TestParseListRange(t *testing.T) {
	res := &Resource{Name: "customers", Path: "customers", Scope: "store"}
	q, err := parseListQuery(httptest.NewRequest("GET", "/v1/erp/customers?min.creditLimit=100&max.creditLimit=500.5&max.creditBalance=0&min.x=", nil), res)
	if err != nil {
		t.Fatal(err)
	}
	cl := q.Range["creditLimit"]
	if cl[0] == nil || *cl[0] != 100 || cl[1] == nil || *cl[1] != 500.5 {
		t.Fatalf("creditLimit range: %v", q.Range)
	}
	if cb := q.Range["creditBalance"]; cb[0] != nil || cb[1] == nil || *cb[1] != 0 {
		t.Fatalf("creditBalance max only: %v", q.Range)
	}
	if _, ok := q.Range["x"]; ok {
		t.Fatal("empty value is no filter")
	}
}

func TestMasterListExtras(t *testing.T) {
	initResources()
	be := func(name string) *legacyBackend { return resourceByName(name).Backend.(*legacyBackend) }
	lo, hi := 100.0, 500.0
	// customers: sort by name, over the credit limit, credit limit range
	f, s, err := be("customers").listExtras(nil, "st", ListQuery{Sort: "nameEn", Where: map[string]string{"overLimit": "true"},
		Range: map[string][2]*float64{"creditLimit": {&lo, &hi}}})
	if err != nil {
		t.Fatal(err)
	}
	if s[0].Key != "name" || s[0].Value != 1 {
		t.Fatalf("sort: %v", s)
	}
	parts := f["$and"].(bson.A)
	if len(parts) != 2 {
		t.Fatalf("filter: %v", f)
	}
	if _, ok := parts[0].(bson.M)["$expr"]; !ok {
		t.Fatalf("over limit compares balance and limit: %v", parts[0])
	}
	if r := parts[1].(bson.M)["credit_limit"].(bson.M); r["$gte"] != lo || r["$lte"] != hi {
		t.Fatalf("range: %v", r)
	}
	if _, _, err := be("customers").listExtras(nil, "st", ListQuery{Where: map[string]string{"overLimit": "maybe"}}); err == nil {
		t.Fatal("overLimit takes true or false")
	}
	if _, _, err := be("customers").listExtras(nil, "st", ListQuery{Range: map[string][2]*float64{"salary": {&lo, nil}}}); err == nil {
		t.Fatal("unknown range field")
	}
	// products: per-store fields use the store id
	_, s, err = be("products").listExtras(nil, "abc123", ListQuery{Sort: "stock", Desc: true})
	if err != nil || s[0].Key != "product_stores.abc123.stock" || s[0].Value != -1 {
		t.Fatalf("product stock sort: %v %v", s, err)
	}
	f, _, err = be("products").listExtras(nil, "abc123", ListQuery{Where: map[string]string{"isService": "false"}})
	if err != nil || !reflect.DeepEqual(f, bson.M{"is_service": bson.M{"$ne": true}}) {
		t.Fatalf("products, goods only: %v %v", f, err)
	}
	// employees: status from is_active
	f, _, err = be("employees").listExtras(nil, "st", ListQuery{Where: map[string]string{"status": "inactive"}})
	if err != nil || !reflect.DeepEqual(f, bson.M{"is_active": false}) {
		t.Fatalf("inactive employees: %v %v", f, err)
	}
	f, _, err = be("employees").listExtras(nil, "st", ListQuery{Where: map[string]string{"nationality": "saudi"}})
	if err != nil || !reflect.DeepEqual(f, bson.M{"iqama_no": bson.M{"$regex": "^1"}}) {
		t.Fatalf("saudi employees: %v %v", f, err)
	}
	if _, _, err := be("employees").listExtras(nil, "st", ListQuery{Where: map[string]string{"nationality": "x"}}); err == nil {
		t.Fatal("nationality takes saudi or expat")
	}
	// vehicles by make
	f, _, err = be("vehicles").listExtras(nil, "st", ListQuery{Where: map[string]string{"make": "Toyota"}})
	if err != nil || !reflect.DeepEqual(f, bson.M{"brand": "Toyota"}) {
		t.Fatalf("vehicles by make: %v %v", f, err)
	}
}

// DB-backed: master lists paged, sorted and filtered by the server.
func TestAPI_MasterListPaging(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	st := "&storeId=" + storeA()
	get := func(path string) []M {
		r := call(t, "GET", path+st, tok, nil)
		if r.Code != 200 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Raw)
		}
		out := []M{}
		for _, row := range r.data() {
			out = append(out, row.(M))
		}
		return out
	}
	names := func(rows []M) []string {
		out := []string{}
		for _, r := range rows {
			out = append(out, str(r["nameEn"]))
		}
		return out
	}
	asc := names(get("/customers?sort=nameEn&limit=500"))
	desc := names(get("/customers?sort=-nameEn&limit=500"))
	if len(asc) < 2 || asc[0] != desc[len(desc)-1] {
		t.Fatalf("name order: %v / %v", asc, desc)
	}
	for i := 1; i < len(asc); i++ {
		if asc[i-1] > asc[i] {
			t.Fatalf("not sorted by name: %v", asc)
		}
	}
	for _, r := range get("/customers?min.creditLimit=1&limit=500") {
		if num(r["creditLimit"]) < 1 {
			t.Fatalf("credit limit below min: %v", r["creditLimit"])
		}
	}
	for _, r := range get("/customers?where.overLimit=true&limit=500") {
		if !(num(r["creditLimit"]) > 0 && num(r["creditBalance"]) > num(r["creditLimit"])) {
			t.Fatalf("not over limit: %v", r)
		}
	}
	for _, r := range get("/products?where.isService=false&sort=-stock&limit=500") {
		if r["isService"] == true {
			t.Fatalf("service in goods-only list: %v", r["id"])
		}
	}
	for _, r := range get("/employees?where.nationality=saudi&limit=500") {
		if !strings.HasPrefix(str(r["nationalId"]), "1") {
			t.Fatalf("not a Saudi id: %v", r["nationalId"])
		}
	}
	for _, r := range get("/employees?where.status=active&limit=500") {
		if str(r["status"]) != "active" {
			t.Fatalf("inactive employee listed: %v", r["id"])
		}
	}
	if r := call(t, "GET", "/customers?where.overLimit=maybe"+st, tok, nil); r.Code != 400 || r.errField("where.overLimit") == "" {
		t.Fatalf("bad overLimit: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/customers?min.salary=1"+st, tok, nil); r.Code != 400 {
		t.Fatalf("unknown range: %d %s", r.Code, r.Raw)
	}
}

func TestParseListSum(t *testing.T) {
	res := &Resource{Name: "customers", Path: "customers", Scope: "store"}
	q, err := parseListQuery(httptest.NewRequest("GET", "/v1/erp/customers?sum=+creditBalance,,creditLimit+", nil), res)
	if err != nil || !reflect.DeepEqual(q.Sum, []string{"creditBalance", "creditLimit"}) {
		t.Fatalf("sum: %v %v", q.Sum, err)
	}
}

// DB-backed: ?sum= totals every match, with the same filters as the rows.
func TestAPI_ListSums(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	st := "&storeId=" + storeA()
	check := func(path string) {
		t.Helper()
		r := call(t, "GET", path+"&limit=500"+st, tok, nil)
		if r.Code != 200 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Raw)
		}
		sums, _ := r.Body["sums"].(M)
		var bal, lim float64
		for _, row := range r.data() {
			bal += num(row.(M)["creditBalance"])
			lim += num(row.(M)["creditLimit"])
		}
		if math.Abs(num(sums["creditBalance"])-bal) > 0.01 || math.Abs(num(sums["creditLimit"])-lim) > 0.01 {
			t.Fatalf("%s: sums %v, rows give %v / %v", path, sums, bal, lim)
		}
	}
	check("/customers?sum=creditBalance,creditLimit")
	check("/customers?sum=creditBalance,creditLimit&q=riyadh")
	check("/customers?sum=creditBalance,creditLimit&min.creditLimit=1")
	// the page size does not change the totals
	a := call(t, "GET", "/customers?sum=creditLimit&limit=1"+st, tok, nil)
	b := call(t, "GET", "/customers?sum=creditLimit&limit=500"+st, tok, nil)
	if num(a.Body["sums"].(M)["creditLimit"]) != num(b.Body["sums"].(M)["creditLimit"]) {
		t.Fatalf("sums depend on the page: %v vs %v", a.Body["sums"], b.Body["sums"])
	}
	if r := call(t, "GET", "/employees?sum=basicSalary&where.status=active"+st, tok, nil); r.Code != 200 || r.Body["sums"] == nil {
		t.Fatalf("employee payroll: %d %s", r.Code, r.Raw)
	}
	for _, bad := range []string{"/customers?sum=nameEn", "/sales?sum=netTotal", "/categories?sum=x"} {
		if r := call(t, "GET", bad+st, tok, nil); r.Code != 400 || r.errField("sum") == "" {
			t.Errorf("%s: %d %s", bad, r.Code, r.Raw)
		}
	}
	// without ?sum= the answer has no sums
	if r := call(t, "GET", "/customers?limit=1"+st, tok, nil); r.Body["sums"] != nil {
		t.Fatal("sums only on request")
	}
}

// DB-backed: product stock status and stock value agree with the web app's rules
// (inventory/helpers.js stockStatus, stockValue) on the same records.
func TestAPI_ProductListStock(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	st := "&storeId=" + storeA()
	// give one product a minimum above its stock so "low" has a member
	all := call(t, "GET", "/products?limit=500"+st, tok, nil)
	var goods []M
	for _, r := range all.data() {
		if r.(M)["isService"] != true {
			goods = append(goods, r.(M))
		}
	}
	if len(goods) == 0 {
		t.Fatal("no goods in the fixture")
	}
	// a minimum above the stock makes a product "low" (a product with stock)
	for _, p := range goods {
		stk := sub(p, "stock")
		var q float64
		for _, w := range stk {
			q += num(w.(M)["qty"])
		}
		if q <= 0 {
			continue
		}
		next := M{}
		for id, w := range stk {
			e := M{"qty": num(w.(M)["qty"]), "min": q + 1000}
			next[id] = e
		}
		up := call(t, "PATCH", "/products/"+str(p["id"]), tok, M{"stock": next})
		if up.Code != 200 {
			t.Fatalf("set min: %d %s", up.Code, up.Raw)
		}
		break
	}
	all = call(t, "GET", "/products?limit=500"+st, tok, nil)
	goods = nil
	for _, r := range all.data() {
		if r.(M)["isService"] != true {
			goods = append(goods, r.(M))
		}
	}
	stockOf := func(p M) (qty, min float64) {
		for _, w := range sub(p, "stock") {
			qty += num(w.(M)["qty"])
			min += num(w.(M)["min"])
		}
		return
	}
	status := func(p M) string {
		q, m := stockOf(p)
		switch {
		case q <= 0:
			return "out"
		case q <= m:
			return "low"
		}
		return "ok"
	}
	want := map[string]map[string]bool{"out": {}, "low": {}, "ok": {}}
	value := 0.0
	for _, p := range goods {
		want[status(p)][str(p["id"])] = true
		q, _ := stockOf(p)
		value += q * num(sub(p, "pricing")["purchase"])
	}
	if len(want["low"]) == 0 {
		t.Fatal("no low-stock product to check")
	}
	for _, s := range []string{"out", "low", "ok"} {
		r := call(t, "GET", "/products?limit=500&where.stockStatus="+s+st, tok, nil)
		if r.Code != 200 {
			t.Fatalf("%s: %d %s", s, r.Code, r.Raw)
		}
		got := map[string]bool{}
		for _, row := range r.data() {
			got[str(row.(M)["id"])] = true
		}
		if !reflect.DeepEqual(got, want[s]) {
			t.Errorf("stockStatus=%s: got %v want %v", s, got, want[s])
		}
	}
	r := call(t, "GET", "/products?limit=1&where.isService=false&sum=stockValue"+st, tok, nil)
	if r.Code != 200 || math.Abs(num(r.Body["sums"].(M)["stockValue"])-value) > 0.01 {
		t.Fatalf("stock value: %s want %v", r.Raw, value)
	}
	if r := call(t, "GET", "/products?where.stockStatus=maybe"+st, tok, nil); r.Code != 400 {
		t.Fatalf("bad stock status: %d", r.Code)
	}
}

func TestRepairJobStatusWhere(t *testing.T) {
	for _, tc := range []struct {
		val  string
		ok   bool
		want string
	}{
		{"open", true, `{"status":{"$nin":["completed","delivered","closed","cancelled"]}}`},
		{"done", true, `{"status":{"$in":["completed","delivered","closed","cancelled"]}}`},
		{"notCancelled", true, `{"status":{"$ne":"cancelled"}}`},
		{"in_progress", true, `{"status":"in_progress"}`},
		{"delivered", true, `{"status":"delivered"}`},
		{"nope", false, ""},
		{"", false, ""},
	} {
		m, ok := repairJobStatus(tc.val, "")
		if ok != tc.ok {
			t.Fatalf("%q: ok=%v", tc.val, ok)
		}
		if !ok {
			continue
		}
		b, _ := bson.MarshalExtJSON(m, false, false)
		if string(b) != tc.want {
			t.Fatalf("%q: %s want %s", tc.val, b, tc.want)
		}
	}
	if _, ok := vehicleOpenJob("maybe", "x"); ok {
		t.Fatal("openJob accepts only true/false")
	}
}

func TestListWhereIDLists(t *testing.T) {
	res := &Resource{Name: "products"}
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = primitive.NewObjectID().Hex()
	}
	// 100 ids fit (a category with many sub-categories)
	var q ListQuery
	if err := parseListExtras(url.Values{"where.categoryId": {strings.Join(ids, ",")}}, res, &q); err != nil {
		t.Fatalf("100 ids: %v", err)
	}
	b := newProductsResource().Backend.(*legacyBackend)
	f, _, err := b.listExtras(&Ctx{}, "s1", q)
	if err != nil {
		t.Fatalf("100 ids: %v", err)
	}
	if in := f["category_id"].(bson.M)["$in"].(bson.A); len(in) != 200 {
		t.Fatalf("each id as ObjectID and text: %d", len(in))
	}
	// 101 ids are refused by name
	q = ListQuery{Where: map[string]string{"categoryId": strings.Join(append(ids, "a"), ",")}}
	if _, _, err := b.listExtras(&Ctx{}, "s1", q); err == nil || !strings.Contains(fmt.Sprint(err), "ids") {
		t.Fatalf("101 ids: %v", err)
	}
	// a value longer than 2600 characters is refused
	if err := parseListExtras(url.Values{"where.make": {strings.Repeat("x", 2601)}}, res, &ListQuery{}); err == nil {
		t.Fatal("long value accepted")
	}
}

func TestAPI_VehicleOpenJobAndRepairTotals(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	st := "&storeId=" + storeA()
	open := map[string]bool{}
	for _, r := range call(t, "GET", "/repair-jobs?limit=500&where.status=open"+st, tok, nil).data() {
		s := str(r.(M)["status"])
		if s == "completed" || s == "delivered" || s == "closed" || s == "cancelled" {
			t.Fatalf("open filter returned a %s job", s)
		}
		open[str(r.(M)["vehicleId"])] = true
	}
	if !open[fx.VehicleA1.Hex()] {
		t.Fatal("the fixture vehicle has an open job")
	}
	all := call(t, "GET", "/vehicles?limit=500"+st, tok, nil)
	in := call(t, "GET", "/vehicles?limit=500&where.openJob=true"+st, tok, nil)
	out := call(t, "GET", "/vehicles?limit=500&where.openJob=false"+st, tok, nil)
	if in.Code != 200 || out.Code != 200 {
		t.Fatalf("openJob: %d %d %s", in.Code, out.Code, out.Raw)
	}
	if num(in.Body["total"])+num(out.Body["total"]) != num(all.Body["total"]) {
		t.Fatalf("open %v + none %v != all %v", in.Body["total"], out.Body["total"], all.Body["total"])
	}
	for _, r := range in.data() {
		if !open[str(r.(M)["id"])] {
			t.Fatalf("vehicle %v has no open job", r.(M)["id"])
		}
	}
	for _, r := range out.data() {
		if open[str(r.(M)["id"])] {
			t.Fatalf("vehicle %v has an open job", r.(M)["id"])
		}
	}
	if r := call(t, "GET", "/vehicles?where.openJob=maybe"+st, tok, nil); r.Code != 400 {
		t.Fatalf("bad openJob: %d", r.Code)
	}
	// one vehicle's jobs, and the grand total of jobs that are not cancelled
	mine := call(t, "GET", "/repair-jobs?limit=500&where.vehicleId="+fx.VehicleA1.Hex()+st, tok, nil)
	for _, r := range mine.data() {
		if str(r.(M)["vehicleId"]) != fx.VehicleA1.Hex() {
			t.Fatalf("other vehicle's job: %v", r.(M)["vehicleId"])
		}
	}
	if len(mine.data()) == 0 {
		t.Fatal("no jobs for the fixture vehicle")
	}
	s := call(t, "GET", "/repair-jobs?limit=1&sum=grand&where.status=notCancelled"+st, tok, nil)
	if s.Code != 200 || s.Body["sums"] == nil || num(s.Body["sums"].(M)["grand"]) < 0 {
		t.Fatalf("grand total: %d %s", s.Code, s.Raw)
	}
}

// A POS terminal's Bills list asks for its own sales: where.posType on sales only.
func TestSalesWherePosType(t *testing.T) {
	sales := resourceByName("sales").Backend.(*legacyBackend)
	f, _, err := sales.listExtras(nil, "", ListQuery{Where: map[string]string{"posType": "thobe"}})
	if err != nil || f["erp.x.posType"] != "thobe" {
		t.Fatalf("filter %v err %v", f, err)
	}
	ret := resourceByName("salesReturns").Backend.(*legacyBackend)
	if _, _, err := ret.listExtras(nil, "", ListQuery{Where: map[string]string{"posType": "thobe"}}); err == nil || err.(*APIError).Status != 400 {
		t.Errorf("sales returns accepted where.posType: %v", err)
	}
}
