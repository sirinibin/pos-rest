package erp

import (
	"reflect"
	"strings"
	"testing"
)

// Pure-function tests for the registered-company list (company_registry.go).

func validCompany() M {
	return M{"nameEn": "Gulf Union Ozone Co.", "registeredIn": "SA", "crNo": "4031012345",
		"vatNo": "300000000000003", "website": "gulfunionozone.com", "email": "info@gulfunionozone.com",
		"phone": "+966 50 197 1075", "countries": []interface{}{"sa", "AE"}}
}

func TestNormalizeCompanyCountries(t *testing.T) {
	cases := []struct {
		in   interface{}
		want []string
	}{
		{nil, []string{}},
		{[]interface{}{}, []string{}},
		{[]interface{}{"kw", " sa ", "AE", "SA", ""}, []string{"SA", "AE", "KW"}},
		{[]interface{}{"IN", "BH", "OM", "QA"}, []string{"OM", "QA", "BH", "IN"}},
		{[]interface{}{"zz", "SA", "aa"}, []string{"SA", "AA", "ZZ"}},
		{[]string{"qa", "sa"}, []string{"SA", "QA"}},
	}
	for _, c := range cases {
		if got := NormalizeCompanyCountries(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v -> %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalizeWebsite(t *testing.T) {
	for in, want := range map[string]string{"": "", "  ": "", "gulfunionozone.com": "https://gulfunionozone.com",
		" http://x.sa ": "http://x.sa", "https://a.b/c": "https://a.b/c"} {
		if got := NormalizeWebsite(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}

func TestValidateCompany(t *testing.T) {
	if e := ValidateCompany(validCompany()); len(e) != 0 {
		t.Fatalf("valid company: %v", e)
	}
	// only the English name and the country of registration are required
	if e := ValidateCompany(M{"nameEn": "X", "registeredIn": "ae"}); len(e) != 0 {
		t.Fatalf("minimal company: %v", e)
	}
	long := strings.Repeat("x", 201)
	cases := []struct {
		name, field string
		set         M
	}{
		{"missing name", "nameEn", M{"nameEn": "  "}},
		{"missing country of registration", "registeredIn", M{"registeredIn": ""}},
		{"unknown country of registration", "registeredIn", M{"registeredIn": "US"}},
		{"bad website", "website", M{"website": "not a site"}},
		{"javascript website", "website", M{"website": "javascript:alert(1)"}},
		{"ftp website", "website", M{"website": "ftp://x.com"}},
		{"bad email", "email", M{"email": "info@"}},
		{"bad phone", "phone", M{"phone": "call me"}},
		{"bad CR", "crNo", M{"crNo": "<b>1</b>"}},
		{"Saudi VAT must be 15 digits 3...3", "vatNo", M{"vatNo": "123"}},
		{"unknown served country", "countries", M{"countries": []interface{}{"SA", "US"}}},
		{"countries not a list", "countries", M{"countries": "SA"}},
		{"name too long", "nameEn", M{"nameEn": long}},
		{"Arabic name too long", "nameAr", M{"nameAr": long}},
		{"address too long", "addressEn", M{"addressEn": strings.Repeat("x", 401)}},
	}
	for _, c := range cases {
		b := validCompany()
		for k, v := range c.set {
			b[k] = v
		}
		if e := ValidateCompany(b); e[c.field] == "" {
			t.Errorf("%s: want error on %s, got %v", c.name, c.field, e)
		}
	}
	// a 400-character address is fine; the VAT rule follows the country of registration
	b := validCompany()
	b["addressEn"] = strings.Repeat("x", 400)
	b["registeredIn"] = "QA"
	b["vatNo"] = ""
	if e := ValidateCompany(b); len(e) != 0 {
		t.Fatalf("Qatar company without VAT: %v", e)
	}
}

func TestCompanyDocAndRow(t *testing.T) {
	d := companyDoc(M{"nameEn": "  Gulf  ", "registeredIn": "sa", "website": "gulfunionozone.com",
		"vatNo": "300 000 000 000 003", "countries": []interface{}{"ae", "sa"}, "evil": "x"})
	if d["nameEn"] != "Gulf" || d["registeredIn"] != "SA" || d["website"] != "https://gulfunionozone.com" ||
		d["vatNo"] != "300000000000003" || !reflect.DeepEqual(d["countries"], []string{"SA", "AE"}) || d["evil"] != nil {
		t.Fatalf("doc: %v", d)
	}
	row := companyRow(M{"_id": "abc", "nameEn": "Gulf", "countries": []interface{}{"AE", "SA"}, "updatedBy": "admin"})
	if row["id"] != "abc" || row["nameEn"] != "Gulf" || row["nameAr"] != "" || row["updatedBy"] != "admin" ||
		!reflect.DeepEqual(row["countries"], []string{"SA", "AE"}) {
		t.Fatalf("row: %v", row)
	}
}

func TestCountryConflicts(t *testing.T) {
	others := []M{
		{"id": "a", "nameEn": "Gulf", "countries": []string{"SA", "AE"}},
		{"id": "b", "nameEn": "Oman Co", "countries": []string{"OM"}},
		{"id": "c", "nameEn": "Empty", "countries": []string{}},
	}
	if c := CountryConflicts([]string{"QA", "KW"}, others, ""); len(c) != 0 {
		t.Fatalf("free countries: %v", c)
	}
	if c := CountryConflicts([]string{"SA", "OM", "QA"}, others, ""); len(c) != 2 || c["SA"] != "Gulf" || c["OM"] != "Oman Co" {
		t.Fatalf("taken countries: %v", c)
	}
	// a company keeps its own countries when edited
	if c := CountryConflicts([]string{"SA", "AE"}, others, "a"); len(c) != 0 {
		t.Fatalf("own countries: %v", c)
	}
	if c := CountryConflicts(nil, others, ""); len(c) != 0 {
		t.Fatalf("no countries: %v", c)
	}
	err, _ := conflictError(map[string]string{"OM": "Oman Co", "AE": "Gulf"}).(*APIError)
	if err == nil || err.Status != 409 || err.Code != "country_taken" ||
		!strings.Contains(err.Message, "AE (Gulf), OM (Oman Co)") {
		t.Fatalf("conflict error: %+v", err)
	}
}

func TestSiteCompanies(t *testing.T) {
	if m := SiteCompanies(nil); len(m) != 0 {
		t.Fatalf("no companies: %v", m)
	}
	m := SiteCompanies([]M{
		{"id": "a", "nameEn": "Gulf", "crNo": "1", "email": "private@x.com", "phone": "1", "updatedBy": "admin",
			"countries": []string{"SA", "AE"}},
		{"id": "b", "nameEn": "Dup", "countries": []string{"AE", "KW"}},
		{"id": "c", "nameEn": "None", "countries": []string{}},
	})
	if len(m) != 3 {
		t.Fatalf("countries: %v", m)
	}
	sa := m["SA"].(M)
	if sa["nameEn"] != "Gulf" || sa["crNo"] != "1" || m["AE"].(M)["nameEn"] != "Gulf" || m["KW"].(M)["nameEn"] != "Dup" {
		t.Fatalf("map: %v", m)
	}
	for _, k := range []string{"email", "phone", "updatedBy", "id", "countries"} {
		if _, ok := sa[k]; ok {
			t.Fatalf("public map must not carry %s: %v", k, sa)
		}
	}
}

func TestDefaultCompany_CoversGCC(t *testing.T) {
	if e := ValidateCompany(DefaultCompany); len(e) != 0 {
		t.Fatalf("default company invalid: %v", e)
	}
	if got := NormalizeCompanyCountries(DefaultCompany["countries"]); !reflect.DeepEqual(got, []string{"SA", "AE", "OM", "QA", "BH", "KW"}) {
		t.Fatalf("default countries: %v", got)
	}
	if !strings.Contains(str(DefaultCompany["website"]), "gulfunionozone.com") {
		t.Fatalf("default website: %v", DefaultCompany["website"])
	}
}

func TestCompanyAdmin_RequiresPlatformAdmin(t *testing.T) {
	for _, h := range []func(*Ctx) error{
		func(c *Ctx) error { return handleCompanyList(c, nil, nil) },
		func(c *Ctx) error { return saveCompany(c, nil, nil, "") },
		func(c *Ctx) error { return handleCompanyDelete(c, nil, nil) },
	} {
		if err, _ := h(&Ctx{}).(*APIError); err == nil || err.Status != 403 {
			t.Fatalf("non-admin: %v", err)
		}
	}
}
