package erp

import "testing"

// The three Kerala tailoring / boutique business categories are offered to
// stores in India only: sign-up, store create and store edit refuse them for
// other countries, and changing an Indian store's country away is refused
// while it keeps one of them.

func TestKeralaCategories_Terminals(t *testing.T) {
	cases := map[string]string{
		"Kerala Tailor Shop and Gents Dress": "keralagents",
		"kerala tailor shop and gents dress": "keralagents",
		"Kerala Tailor Shop":                 "keralatailor",
		"Kerala Ladies Boutique":             "keralaboutique",
		// the display spelling is not ZATCA-safe and is not a stored value
		"Kerala Tailor Shop & Gents Dress": "",
	}
	for in, want := range cases {
		if got := CategoryTerminal(in); got != want {
			t.Errorf("CategoryTerminal(%q) = %q, want %q", in, got, want)
		}
	}
	for _, term := range []string{"keralagents", "keralatailor", "keralaboutique"} {
		if !posTerminals[term] {
			t.Errorf("%s is not a known POS terminal (products, POS records, settings)", term)
		}
	}
}

func TestCategoryAllowedIn(t *testing.T) {
	tests := []struct {
		cat, cc string
		want    bool
	}{
		{"Kerala Tailor Shop", "IN", true},
		{"Kerala Tailor Shop", "in", true},
		{"Kerala Tailor Shop", " IN ", true},
		{"Kerala Tailor Shop", "SA", false},
		{"Kerala Tailor Shop", "", false}, // no country = Saudi Arabia
		{"Kerala Ladies Boutique", "AE", false},
		{"Kerala Tailor Shop and Gents Dress", "IN", true},
		{"Kerala Tailor Shop and Gents Dress", "KW", false},
		// every other category is open everywhere
		{"Grocery", "IN", true},
		{"Grocery", "SA", true},
		{"Kerala Restaurant", "SA", true},
		{"Thobe Tailoring", "IN", true},
		// free text: left to the caller
		{"Retail", "SA", true},
		{"", "SA", true},
	}
	for _, tc := range tests {
		if got := CategoryAllowedIn(tc.cat, tc.cc); got != tc.want {
			t.Errorf("CategoryAllowedIn(%q, %q) = %v, want %v", tc.cat, tc.cc, got, tc.want)
		}
	}
	if got := categoryCountryError("Kerala Tailor Shop"); got != "this business category is for stores in India" {
		t.Errorf("error text %q", got)
	}
}

func TestStoreValidate_KeralaCategoryCountry(t *testing.T) {
	withNoDocs(t, false)
	x := newMapCtx(nil, "")
	const india = "this business category is for stores in India"
	tests := []struct {
		name string
		rec  M
		prev M
		err  string
	}{
		{"India store creates a Kerala category", M{"nameEn": "A", "category": "Kerala Tailor Shop", "countryCode": "IN"}, nil, ""},
		{"Saudi store cannot", M{"nameEn": "A", "category": "Kerala Tailor Shop", "countryCode": "SA"}, nil, india},
		{"no country (Saudi) cannot", M{"nameEn": "A", "category": "Kerala Ladies Boutique"}, nil, india},
		{"UAE store cannot", M{"nameEn": "A", "category": "Kerala Tailor Shop and Gents Dress", "countryCode": "AE"}, nil, india},
		{"Indian store switches to it", M{"nameEn": "A", "category": "Kerala Ladies Boutique"}, M{"business_category": "Grocery", "country_code": "IN"}, ""},
		{"Saudi store switches to it", M{"nameEn": "A", "category": "Kerala Ladies Boutique"}, M{"business_category": "Grocery", "country_code": "SA"}, india},
		{"Kerala store moves to Oman", M{"nameEn": "A", "category": "Kerala Tailor Shop", "countryCode": "OM"}, M{"business_category": "Kerala Tailor Shop", "country_code": "IN"}, india},
		{"Kerala store keeps India", M{"nameEn": "A", "category": "Kerala Tailor Shop", "countryCode": "IN"}, M{"business_category": "Kerala Tailor Shop", "country_code": "IN"}, ""},
		{"Kerala store edits other fields", M{"nameEn": "B", "category": "Kerala Tailor Shop"}, M{"business_category": "Kerala Tailor Shop", "country_code": "IN"}, ""},
		{"other categories are not limited", M{"nameEn": "A", "category": "Kerala Restaurant", "countryCode": "SA"}, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := storeValidate(x, tc.rec, tc.prev)
			if e["category"] != tc.err {
				t.Fatalf("category error %q, want %q (all: %v)", e["category"], tc.err, e)
			}
		})
	}
}

func TestValidateSignup_KeralaCategory(t *testing.T) {
	b := indiaSignup()
	b["company"].(M)["type"] = "Kerala Tailor Shop and Gents Dress"
	if e := ValidateSignup(b); len(e) != 0 {
		t.Fatalf("India sign-up with a Kerala category rejected: %v", e)
	}
	s := validSignup()
	s["company"].(M)["type"] = "Kerala Ladies Boutique"
	if e := ValidateSignup(s); e["company.type"] != "this business category is for stores in India" {
		t.Fatalf("Saudi sign-up with a Kerala category: %v", e)
	}
	if got := signupCategory("kerala tailor shop"); got != "Kerala Tailor Shop" {
		t.Fatalf("signupCategory = %q", got)
	}
}

func TestStarterCatalogs_KeralaAreIndianRupees(t *testing.T) {
	d, err := loadStarterCatalogs()
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"keralagents", "keralatailor", "keralaboutique"} {
		tc := d.Terminals[term]
		if tc == nil {
			t.Fatalf("%s: no starter catalog", term)
		}
		services := 0
		for _, it := range tc.Items {
			if len(it.Countries) != 1 || it.Countries[0] != "IN" || !it.Local {
				t.Errorf("%s: %s must be India-only with rupee (local) prices", term, it.Key)
			}
			if it.Service {
				services++
			}
		}
		if services < 5 {
			t.Errorf("%s: %d stitching services, want 5+", term, services)
		}
	}
}
