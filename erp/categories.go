package erp

import (
	"regexp"
	"strings"
)

// BusinessCategory is one value a store's legacy `business_category` field
// may take in StartERP. The value is sent to ZATCA as the CSR "industry /
// business category" (OID 2.5.4.15) when the store onboards to Phase 2, so it
// is a plain English name made of letters and spaces only. Each category opens
// exactly one Saudi POS terminal in the web app (Terminal = the terminal id).
type BusinessCategory struct {
	Value    string
	Terminal string
}

// BusinessCategories mirrors src/pos/categories.js in starterp-frontend-v1.
var BusinessCategories = []BusinessCategory{
	{"Grocery", "grocery"},
	{"Supermarket", "supermarket"},
	{"Gifts and Cosmetics", "fancysa"},
	{"Mobile Phones and Accessories", "mobile"},
	{"Restaurant", "restaurant"},
	{"Pakistani Restaurant", "pakistani"},
	{"Yemeni Restaurant", "yemeni"},
	{"Kerala Restaurant", "kerala"},
	{"Coffee Shop", "coffee"},
	{"Cafeteria", "cafesa"},
	{"Ladies Beauty Salon", "salon"},
	{"Barber Shop", "barber"},
	{"Textiles and Tailoring", "textile"},
	{"Ladies Boutique", "boutique"},
	{"Thobe Tailoring", "thobe"},
	{"Ladies Tailoring", "ladiestailor"},
	{"Auto Spare Parts", "parts"},
	{"Auto Repair Workshop", "workshop"},
	{"Trading", "trading"},
	{"Industrial Supplies", "industrial"},
	{"Construction and Contracting", "construction"},
	{"Interior Design", "interior"},
	{"Software and IT Services", "softwaresa"},
	{"Gaming and Entertainment", "gamingsa"},
	// shown as "Business, Visa & Travels" in the app (ZATCA: letters and spaces only)
	{"Business Visa and Travels", "travel"},
	// India only (categoryCountries); "Kerala Tailor Shop & Gents Dress" in the app
	{"Kerala Tailor Shop and Gents Dress", "keralagents"},
	{"Kerala Tailor Shop", "keralatailor"},
	{"Kerala Ladies Boutique", "keralaboutique"},
}

// categoryCountries: business categories offered only in some countries (by
// terminal id). Every other category is open to every supported country.
var categoryCountries = map[string][]string{
	"keralagents":    {"IN"},
	"keralatailor":   {"IN"},
	"keralaboutique": {"IN"},
}

// CategoryCountries returns the countries a business category is limited to
// (nil: every country).
func CategoryCountries(value string) []string {
	return categoryCountries[CategoryTerminal(value)]
}

// CategoryAllowedIn reports whether a store in country cc (ISO code, "" =
// Saudi Arabia, the legacy default) may use the business category. Unknown
// free-text values are left to the caller.
func CategoryAllowedIn(value, cc string) bool {
	list := CategoryCountries(value)
	if len(list) == 0 {
		return true
	}
	cc = strings.ToUpper(strings.TrimSpace(cc))
	if cc == "" {
		cc = "SA"
	}
	for _, c := range list {
		if c == cc {
			return true
		}
	}
	return false
}

// categoryCountryError is the message when a category is not offered in the
// store's country.
func categoryCountryError(value string) string {
	if list := CategoryCountries(value); len(list) == 1 && list[0] == "IN" {
		return "this business category is for stores in India"
	}
	return "this business category is not offered in the store's country"
}

// reZatcaCategory: what ZATCA's CSR accepts safely as business category.
var reZatcaCategory = regexp.MustCompile(`^[A-Za-z][A-Za-z ]{1,63}$`)

// ValidZatcaCategory reports whether s can be sent to ZATCA as is.
func ValidZatcaCategory(s string) bool {
	return reZatcaCategory.MatchString(s) && !strings.Contains(s, "  ") && strings.TrimSpace(s) == s
}

// CanonicalCategory returns the canonical spelling of a known business
// category (matched case-insensitively), and whether it is known.
func CanonicalCategory(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, c := range BusinessCategories {
		if strings.EqualFold(c.Value, s) {
			return c.Value, true
		}
	}
	return "", false
}

// CategoryTerminal returns the POS terminal id of a business category, or ""
// for a free-text legacy value that matches none.
func CategoryTerminal(s string) string {
	s = strings.TrimSpace(s)
	for _, c := range BusinessCategories {
		if strings.EqualFold(c.Value, s) {
			return c.Terminal
		}
	}
	return ""
}

// posTerminals are the terminal ids a product may be tagged with (posTerminal);
// Saudi terminals from the categories plus the Indian demo terminals.
var posTerminals = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range BusinessCategories {
		m[c.Terminal] = true
	}
	for _, t := range []string{"boutiquein", "cafein", "fancyin", "gamingin", "partsin", "softwarein", "tailorin"} {
		m[t] = true
	}
	return m
}()

var rePosToken = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// validatePosFields checks the POS catalog fields a product may carry
// (kept in erp.x): posTerminal, posSection and posKey.
func validatePosFields(rec M, e map[string]string) {
	validateProductSpecs(rec, e)
	if v, ok := rec["posTerminal"]; ok && v != nil && str(v) != "" {
		if !posTerminals[str(v)] {
			e["posTerminal"] = "unknown POS terminal"
		}
	}
	for _, k := range []string{"posSection", "posKey"} {
		if v, ok := rec[k]; ok && v != nil && str(v) != "" && !rePosToken.MatchString(str(v)) {
			e[k] = "letters, digits, - and _ only (max 40)"
		}
	}
}
