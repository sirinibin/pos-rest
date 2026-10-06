package erp

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func recKeys(m M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sampleRec() M {
	return M{
		"id": "c1", "storeId": "st1", "version": int64(3), "deleted": false,
		"code": "OLY-C-0001", "nameEn": "Al Rajhi", "nameAr": "الراجحي", "phone": "0551234567",
		"history": []interface{}{M{"at": "2026-10-01T10:00", "action": "created"}},
	}
}

func TestParseSelect(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		include []string
		exclude []string
		wantErr bool
	}{
		{name: "empty", in: ""},
		{name: "blank", in: "   "},
		{name: "only commas", in: ",,"},
		{name: "include list", in: "code,nameEn", include: []string{"code", "nameEn"}},
		{name: "spaces trimmed", in: " code , nameEn ", include: []string{"code", "nameEn"}},
		{name: "exclude", in: "-history", exclude: []string{"history"}},
		{name: "several excludes", in: "-history,-items", exclude: []string{"history", "items"}},
		{name: "mixed", in: "code,-history", include: []string{"code"}, exclude: []string{"history"}},
		{name: "underscore and digits", in: "line_2", include: []string{"line_2"}},
		{name: "dotted path rejected", in: "items.qty", wantErr: true},
		{name: "operator rejected", in: "$where", wantErr: true},
		{name: "leading digit rejected", in: "2code", wantErr: true},
		{name: "double dash rejected", in: "--history", wantErr: true},
		{name: "lone dash rejected", in: "-", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := parseSelect(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				ae, ok := err.(*APIError)
				if !ok || ae.Status != http.StatusBadRequest || ae.Fields["select"] == "" {
					t.Fatalf("want 400 with select field error, got %#v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := func(m map[string]bool) []string {
				out := []string{}
				for k := range m {
					out = append(out, k)
				}
				sort.Strings(out)
				return out
			}
			want := func(s []string) []string {
				if s == nil {
					return []string{}
				}
				sort.Strings(s)
				return s
			}
			if !reflect.DeepEqual(got(fs.include), want(tc.include)) || !reflect.DeepEqual(got(fs.exclude), want(tc.exclude)) {
				t.Fatalf("include=%v exclude=%v", got(fs.include), got(fs.exclude))
			}
		})
	}
}

func TestFieldSelectApply(t *testing.T) {
	cases := []struct {
		name string
		sel  string
		want []string
	}{
		{name: "no select returns everything", sel: "", want: recKeys(sampleRec())},
		{name: "exclude history", sel: "-history", want: []string{"code", "deleted", "id", "nameAr", "nameEn", "phone", "storeId", "version"}},
		{name: "include keeps envelope keys", sel: "code,nameEn", want: []string{"code", "deleted", "id", "nameEn", "storeId", "version"}},
		{name: "include wins over exclude", sel: "code,-code", want: []string{"code", "deleted", "id", "storeId", "version"}},
		{name: "unknown included field is ignored", sel: "nope", want: []string{"deleted", "id", "storeId", "version"}},
		{name: "excluding an envelope key is honoured", sel: "-version", want: []string{"code", "deleted", "history", "id", "nameAr", "nameEn", "phone", "storeId"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := parseSelect(tc.sel)
			if err != nil {
				t.Fatal(err)
			}
			got := recKeys(fs.apply(sampleRec()))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestFieldSelectApplyDoesNotMutateInput(t *testing.T) {
	fs, _ := parseSelect("-history")
	rec := sampleRec()
	_ = fs.apply(rec)
	if _, ok := rec["history"]; !ok {
		t.Fatal("apply must not modify the input record")
	}
	if fs.apply(nil) != nil {
		t.Fatal("nil in, nil out")
	}
}

func TestFieldSelectApplyAll(t *testing.T) {
	fs, _ := parseSelect("-history")
	rows := fs.applyAll([]M{sampleRec(), sampleRec()})
	for _, r := range rows {
		if _, ok := r["history"]; ok {
			t.Fatal("history must be dropped from every row")
		}
	}
	var none fieldSelect
	if got := none.applyAll(nil); got != nil {
		t.Fatal("empty select passes rows through")
	}
}

func TestFieldSelectDBProjection(t *testing.T) {
	cases := []struct {
		sel, key string
		want     bson.M
	}{
		{"", "erp.h", nil},
		{"-history", "erp.h", bson.M{"erp.h": 0}},
		{"-history", "history", bson.M{"history": 0}},
		{"-history", "", nil},
		{"code,nameEn", "erp.h", bson.M{"erp.h": 0}}, // history not selected
		{"code,history", "erp.h", nil},
		{"-phone", "erp.h", nil}, // only history is projected away in the DB
	}
	for _, tc := range cases {
		fs, err := parseSelect(tc.sel)
		if err != nil {
			t.Fatal(err)
		}
		if got := fs.dbProjection(tc.key); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("select=%q key=%q: got %v want %v", tc.sel, tc.key, got, tc.want)
		}
	}
}

func TestParseListQuerySelect(t *testing.T) {
	res := &Resource{Name: "customers", Path: "customers", Scope: "store"}
	r := httptest.NewRequest("GET", "/v1/erp/customers?select=-history&limit=2000&page=1", nil)
	q, err := parseListQuery(r, res)
	if err != nil {
		t.Fatal(err)
	}
	if q.Limit != 2000 || q.Select.wants("history") || !q.Select.wants("nameEn") {
		t.Fatalf("query: %+v", q)
	}
	bad := httptest.NewRequest("GET", "/v1/erp/customers?select=a.b", nil)
	if _, err := parseListQuery(bad, res); err == nil {
		t.Fatal("dotted select must be rejected")
	}
}

// The select parameter is read only after authentication: an anonymous
// request with select still gets 401, never data or a 400 that leaks shape.
func TestSelect_Unauthenticated(t *testing.T) {
	for _, path := range []string{"/customers?storeId=x&select=-history", "/customers/abc?select=code", "/customers?select=$bad"} {
		rec := httptest.NewRecorder()
		testRouter.ServeHTTP(rec, httptest.NewRequest("GET", Prefix+path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", path, rec.Code)
		}
	}
}

// DB-backed: list and get honour select (opt-in, see main_test.go).
func TestAPI_ListAndGetSelect(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	for _, res := range Resources() {
		base := "/" + res.Path + "?limit=500&includeDeleted=1&page=1"
		if res.Scope == "store" {
			base += "&storeId=" + storeA()
		}
		full := call(t, "GET", base, tok, nil)
		trimmed := call(t, "GET", base+"&select=-history", tok, nil)
		if full.Code != 200 || trimmed.Code != 200 {
			t.Errorf("%s: %d / %d %s", res.Path, full.Code, trimmed.Code, trimmed.Raw)
			continue
		}
		if len(full.data()) != len(trimmed.data()) || num(full.Body["total"]) != num(trimmed.Body["total"]) {
			t.Errorf("%s: select changed the row set", res.Path)
		}
		for i, row := range trimmed.data() {
			m := row.(M)
			if _, ok := m["history"]; ok {
				t.Errorf("%s: history returned despite select=-history", res.Path)
			}
			f := full.data()[i].(M)
			delete(f, "history")
			if str(f["id"]) != str(m["id"]) || len(recKeys(f)) != len(recKeys(m)) {
				t.Errorf("%s: other fields must be unchanged: %v vs %v", res.Path, recKeys(f), recKeys(m))
			}
		}
	}
	// include mode keeps the envelope
	r := call(t, "GET", "/customers?storeId="+storeA()+"&select=nameEn", tok, nil)
	for _, row := range r.data() {
		if got := recKeys(row.(M)); !reflect.DeepEqual(got, []string{"deleted", "id", "nameEn", "storeId", "version"}) {
			t.Fatalf("include select: %v", got)
		}
	}
	// get by id
	g := call(t, "GET", "/sales/"+fx.OrderA1.Hex()+"?select=-history", tok, nil)
	if g.Code != 200 || g.Body["history"] != nil || str(g.Body["id"]) != fx.OrderA1.Hex() {
		t.Fatalf("get with select: %d %s", g.Code, g.Raw)
	}
	if bad := call(t, "GET", "/customers?storeId="+storeA()+"&select=a.b", tok, nil); bad.Code != 400 || bad.errField("select") == "" {
		t.Fatalf("bad select: %d %s", bad.Code, bad.Raw)
	}
}
