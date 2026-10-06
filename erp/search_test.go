package erp

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestParseSearch(t *testing.T) {
	many := make([]string, 201)
	for i := range many {
		many[i] = primitive.NewObjectID().Hex()
	}
	cases := []struct {
		name    string
		q, ids  string
		wantQ   string
		wantIDs []string
		errKey  string
	}{
		{name: "empty"},
		{name: "trimmed", q: "  ahmed  ", wantQ: "ahmed"},
		{name: "arabic", q: "الرياض", wantQ: "الرياض"},
		{name: "100 runes ok", q: strings.Repeat("ع", 100), wantQ: strings.Repeat("ع", 100)},
		{name: "101 runes rejected", q: strings.Repeat("a", 101), errKey: "q"},
		{name: "ids split and trimmed", ids: " a , b,,c ", wantIDs: []string{"a", "b", "c"}},
		{name: "duplicate ids dropped", ids: "a,b,a", wantIDs: []string{"a", "b"}},
		{name: "only commas", ids: ",,,"},
		{name: "200 ids ok", ids: strings.Join(many[:200], ","), wantIDs: many[:200]},
		{name: "201 ids rejected", ids: strings.Join(many, ","), errKey: "ids"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, ids, err := parseSearch(tc.q, tc.ids)
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
			if q != tc.wantQ || !reflect.DeepEqual(ids, tc.wantIDs) {
				t.Fatalf("got %q %v", q, ids)
			}
		})
	}
}

func TestSearchFilter(t *testing.T) {
	if searchFilter("", []string{"name"}) != nil || searchFilter("x", nil) != nil {
		t.Fatal("empty q or no keys must not filter")
	}
	one := searchFilter("abc", []string{"name"})
	if rx, ok := one["name"].(primitive.Regex); !ok || rx.Pattern != "abc" || rx.Options != "i" {
		t.Fatalf("single key: %v", one)
	}
	two := searchFilter("a", []string{"name", "phone"})
	if or, ok := two["$or"].(bson.A); !ok || len(or) != 2 {
		t.Fatalf("two keys: %v", two)
	}
	// regex metacharacters are escaped: the text is matched literally
	for _, in := range []string{".*", "a+b", "(x)", "[0-9]", "^$", `\d`, "a|b"} {
		rx := searchFilter(in, []string{"name"})["name"].(primitive.Regex)
		if rx.Pattern == in {
			t.Errorf("%q not escaped", in)
		}
	}
}

func TestLegacyIDsFilter(t *testing.T) {
	if legacyIDsFilter(nil) != nil {
		t.Fatal("no ids must not filter")
	}
	oid := primitive.NewObjectID()
	f := legacyIDsFilter([]string{oid.Hex(), "tmp_cus_1"})
	or := f["$or"].(bson.A)
	if len(or) != 2 {
		t.Fatalf("filter: %v", f)
	}
	in := or[0].(bson.M)["_id"].(bson.M)["$in"].(bson.A)
	if len(in) != 1 || in[0] != oid {
		t.Fatalf("object ids: %v", in)
	}
	cids := or[1].(bson.M)["erp.cid"].(bson.M)["$in"].(bson.A)
	if len(cids) != 2 {
		t.Fatalf("client ids: %v", cids)
	}
	// only client ids: no _id clause
	if g := legacyIDsFilter([]string{"abc"}); len(g["$or"].(bson.A)) != 1 {
		t.Fatalf("client-only: %v", g)
	}
}

func TestParseListQuerySearch(t *testing.T) {
	res := &Resource{Name: "customers", Path: "customers", Scope: "store"}
	r := httptest.NewRequest("GET", "/v1/erp/customers?q=+riyadh+&ids=a,b&limit=20", nil)
	q, err := parseListQuery(r, res)
	if err != nil {
		t.Fatal(err)
	}
	if q.Search != "riyadh" || !reflect.DeepEqual(q.IDs, []string{"a", "b"}) || q.Limit != 20 {
		t.Fatalf("query: %+v", q)
	}
	long := httptest.NewRequest("GET", "/v1/erp/customers?q="+strings.Repeat("x", 101), nil)
	if _, err := parseListQuery(long, res); err == nil {
		t.Fatal("long q must be rejected")
	}
}

// Every resource the web app searches from a picker must support ?q=.
func TestSearchableResources(t *testing.T) {
	initResources()
	want := []string{"products", "customers", "vendors", "employees", "vehicles", "packages", "rfqSuppliers",
		"categories", "brands", "accounts"}
	for _, name := range want {
		res := resourceByName(name)
		if res == nil {
			t.Fatalf("%s: no resource", name)
		}
		lb, ok := res.Backend.(*legacyBackend)
		if !ok || len(lb.searchKeys) == 0 {
			t.Errorf("%s: no search keys", name)
		}
		if err := lb.checkSearch(ListQuery{Search: "x"}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	// a resource without search keys rejects ?q= with a field error
	sales := resourceByName("sales").Backend.(*legacyBackend)
	err := sales.checkSearch(ListQuery{Search: "x"})
	if ae, ok := err.(*APIError); !ok || ae.Status != 400 || ae.Fields["q"] == "" {
		t.Fatalf("sales ?q=: %v", err)
	}
	if err := sales.checkSearch(ListQuery{}); err != nil {
		t.Fatalf("sales without q: %v", err)
	}
}

func resourceByName(name string) *Resource {
	for _, r := range Resources() {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// ?q= and ?ids= are read only after authentication.
func TestSearch_Unauthenticated(t *testing.T) {
	for _, path := range []string{"/customers?q=riyadh", "/products?ids=abc", "/customers?q=" + strings.Repeat("x", 300)} {
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, httptest.NewRequest("GET", Prefix+path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

// DB-backed: search and ids against the legacy fixture (opt-in, see main_test.go).
func TestAPI_ListSearch(t *testing.T) {
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
	cases := []struct {
		name, path string
		want       []string
	}{
		{"customer by name, case-insensitive", "/customers?q=riyadh" + st, []string{fx.CustomerA1.Hex()}},
		{"customer by arabic name", "/customers?q=الرياض" + st, []string{fx.CustomerA1.Hex()}},
		{"customer by code", "/customers?q=cust-0002" + st, []string{fx.CustomerA2.Hex()}},
		{"customer by phone digits", "/customers?q=1234+5679" + st, []string{fx.CustomerA2.Hex()}},
		{"customer, no match", "/customers?q=zzzz-nobody" + st, []string{}},
		{"regex text is literal", "/customers?q=.*" + st, []string{}},
		{"vendor by vat no", "/vendors?q=3111111111" + st, []string{fx.VendorA1.Hex()}},
		{"product by item code", "/products?q=of-1" + st, []string{fx.ProductA2.Hex()}},
		{"product by part number", "/products?q=PN-003" + st, []string{fx.ProductA3.Hex()}},
		{"employee by name", "/employees?q=ahmed" + st, []string{fx.EmployeeA1.Hex()}},
		{"vehicle by owner", "/vehicles?q=riyadh" + st, []string{fx.VehicleA1.Hex()}},
		{"ids", "/customers?ids=" + fx.CustomerA2.Hex() + "," + fx.CustomerA1.Hex() + st, []string{fx.CustomerA1.Hex(), fx.CustomerA2.Hex()}},
		{"ids and q", "/customers?ids=" + fx.CustomerA1.Hex() + "," + fx.CustomerA2.Hex() + "&q=walk" + st, []string{fx.CustomerA2.Hex()}},
		{"unknown id", "/customers?ids=" + primitive.NewObjectID().Hex() + st, []string{}},
		{"other store's customer is not found", "/customers?ids=" + fx.CustomerB1.Hex() + st, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := call(t, "GET", tc.path, tok, nil)
			if r.Code != 200 {
				t.Fatalf("%d %s", r.Code, r.Raw)
			}
			got := ids(r)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for _, w := range tc.want {
				found := false
				for _, g := range got {
					found = found || g == w
				}
				if !found {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
			if num(r.Body["total"]) != float64(len(tc.want)) {
				t.Fatalf("total %v want %d", r.Body["total"], len(tc.want))
			}
		})
	}
	// search combines with select and limit (what the picker sends)
	r := call(t, "GET", "/customers?q=o&limit=1&select=code,nameEn,phone"+st, tok, nil)
	if r.Code != 200 || len(r.data()) != 1 || num(r.Body["total"]) < 2 {
		t.Fatalf("limit: %d %s", r.Code, r.Raw)
	}
	if got := recKeys(r.data()[0].(M)); !reflect.DeepEqual(got, []string{"code", "deleted", "id", "nameEn", "phone", "storeId", "version"}) {
		t.Fatalf("select with q: %v", got)
	}
	// resources without search fields reject q
	if bad := call(t, "GET", "/sales?q=x"+st, tok, nil); bad.Code != 400 || bad.errField("q") == "" {
		t.Fatalf("sales q: %d %s", bad.Code, bad.Raw)
	}
	// native resources search code/name
	if n := call(t, "GET", "/customer-categories?q=nothing-here", tok, nil); n.Code != 200 {
		t.Fatalf("native q: %d %s", n.Code, n.Raw)
	}
}
