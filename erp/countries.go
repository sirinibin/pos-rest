package erp

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirinibin/startpos/backend/models"
)

// Store country rules (GCC and India): currency, VAT/GST, tax-number,
// phone and address formats and ZATCA availability come from
// models.CountryProfile.

// storeProfile is the store's country profile (Saudi Arabia for an empty
// or non-GCC country, the rules those stores always had).
func storeProfile(store M) *models.CountryProfile {
	return models.CountryProfileOrSaudi(storeCountry(store))
}

// storeVatPercent is the store's VAT rate: 0 in a country without VAT,
// else the saved rate or the country's standard rate.
func storeVatPercent(store M) float64 {
	p := storeProfile(store)
	if !p.HasVAT {
		return 0
	}
	if v := num(store["vat_percent"]); v > 0 {
		return v
	}
	return p.VatPercent
}

// storeZatca reports whether ZATCA e-invoicing applies to the store.
func storeZatca(store M) bool { return models.ZatcaApplies(storeCountry(store)) }

// currencyContract is the contract `currency` of a country.
func currencyContract(p *models.CountryProfile) M {
	return M{"code": p.CurrencyCode, "nameEn": p.CurrencyNameEn, "nameAr": p.CurrencyNameAr,
		"symbolEn": p.CurrencySymbolEn, "symbolAr": p.CurrencySymbolAr,
		"fractionEn": p.FractionEn, "fractionAr": p.FractionAr, "decimals": p.Decimals}
}

// countryContract is the read-only `country` block of a store: what the
// client needs to label, format and gate country features.
func countryContract(p *models.CountryProfile) M {
	rates := []float64{}
	rates = append(rates, p.VatRates...)
	states := []M{}
	for _, s := range p.States {
		states = append(states, M{"code": s.Code, "name": s.Name})
	}
	step := p.RoundingStep
	if step == 0 {
		step = 0.05
	}
	return M{"code": p.Code, "nameEn": p.NameEn, "nameAr": p.NameAr, "gcc": p.GCC, "supported": true,
		"hasVat": p.HasVAT, "vatPercent": p.VatPercent, "taxNameEn": p.TaxNameEn, "taxNameAr": p.TaxNameAr,
		"vatRates": rates, "taxSplit": p.TaxSplit, "states": states, "roundingStep": step,
		"crRequired": p.CRRequired, "arabicRequired": p.GCC, "postalHint": p.PostalHint,
		"taxIdLabelEn": p.TaxIDLabelEn, "taxIdLabelAr": p.TaxIDLabelAr, "taxIdRequired": p.TaxIDRequired,
		"taxIdHint": p.TaxIDHint, "crLabelEn": p.CRLabelEn, "crLabelAr": p.CRLabelAr,
		"dialCode": p.DialCode, "mobileHint": p.MobileHint, "nationalAddress": p.NationalAddress,
		"einvoicing": p.EInvoicing, "zatca": p.EInvoicing == "zatca",
		"invoiceTitleEn": p.InvoiceTitleEn, "invoiceTitleAr": p.InvoiceTitleAr, "timeZone": p.TimeZone}
}

// validTaxIDFor checks a VAT/tax number against a country's format (the
// Saudi check stays ValidVAT).
func validTaxIDFor(p *models.CountryProfile, v string) bool {
	if p.Code == "SA" {
		return ValidVAT(v)
	}
	return p.ValidTaxID(v)
}

func taxIDErrorFor(p *models.CountryProfile) string {
	if p.Code == "SA" {
		return "VAT No. must be 15 digits starting and ending with 3"
	}
	if p.Code == "IN" {
		return "GSTIN: 15 characters (state code, PAN, entity, Z, check character) with a valid check character"
	}
	return p.TaxIDLabelEn + ": " + p.TaxIDHint
}

// countryChoiceError: the message for an unsupported country.
const countryChoiceError = "choose a supported country: Saudi Arabia, UAE, Oman, Qatar, Bahrain, Kuwait or India"

// autoRounding is the cash rounding of a total: the nearest 0.05 (the app's
// default) or the country's step (India: the nearest rupee).
func autoRounding(before float64, p *models.CountryProfile) float64 {
	if p == nil || p.RoundingStep <= 0 || p.RoundingStep == 0.05 {
		return round2(math.Round(before*20)/20 - before)
	}
	return round2(math.Round(before/p.RoundingStep)*p.RoundingStep - before)
}

// validPhoneFor accepts the country's mobiles and landlines; Saudi keeps
// ValidSaudiMobile/ValidSaudiPhone.
func validPhoneFor(p *models.CountryProfile, v string) bool {
	if p.Code == "SA" {
		return ValidSaudiMobile(v) || ValidSaudiPhone(v)
	}
	return p.ValidPhone(v)
}

// validAnyGCCPhone: a user's phone may be from any supported country
// (GCC or India).
func validAnyGCCPhone(v string) bool {
	for i := range models.Countries {
		if validPhoneFor(&models.Countries[i], v) {
			return true
		}
	}
	return false
}

// validMobileFor accepts the country's mobile numbers.
func validMobileFor(p *models.CountryProfile, v string) bool {
	if p.Code == "SA" {
		return ValidSaudiMobile(v)
	}
	return p.ValidMobile(v)
}

// addressErrorsFor applies the address rules of a country to a contract
// address ("prefix" is prepended to the keys). Saudi stores keep the
// National Address digit rules; India checks the 6-digit PIN code and the
// state (address.stateCode); the other countries only bound the postal
// code.
func addressErrorsFor(p *models.CountryProfile, a M, prefix string) map[string]string {
	e := map[string]string{}
	if len(p.States) > 0 {
		if sc := strings.TrimSpace(str(a["stateCode"])); sc != "" && p.StateName(sc) == "" {
			e[prefix+"stateCode"] = "unknown state code"
		}
	}
	if p.NationalAddress {
		if b := str(a["buildingNo"]); b != "" && !re4.MatchString(b) {
			e[prefix+"buildingNo"] = "4 digits"
		}
		if v := str(a["postalCode"]); v != "" && !re5.MatchString(v) {
			e[prefix+"postalCode"] = "5 digits"
		}
		if v := str(a["additionalNo"]); v != "" && !re4.MatchString(v) {
			e[prefix+"additionalNo"] = "4 digits"
		}
		return e
	}
	if v := str(a["postalCode"]); v != "" && !p.ValidPostal(v) {
		if p.PostalHint != "" {
			e[prefix+"postalCode"] = p.PostalHint
		} else {
			e[prefix+"postalCode"] = "invalid postal code"
		}
	}
	if v := str(a["buildingNo"]); len(v) > 20 {
		e[prefix+"buildingNo"] = "at most 20 characters"
	}
	return e
}

// errZatcaNotApplicable: ZATCA calls on a store outside Saudi Arabia.
func errZatcaNotApplicable(p *models.CountryProfile) error {
	return errf(http.StatusConflict, "zatca_not_applicable",
		"ZATCA e-invoicing only applies to stores in Saudi Arabia. This store is in "+p.NameEn+".", nil)
}

// handleCountries: GET /countries (public) lists the supported countries.
func handleCountries(w http.ResponseWriter, r *http.Request) {
	out := []M{}
	for i := range models.Countries {
		p := &models.Countries[i]
		c := countryContract(p)
		c["currency"] = currencyContract(p)
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, M{"data": out})
}

// normCountry upper-cases and trims an ISO code.
func normCountry(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// partyStateCode is the place of supply of a party in India: the GSTIN's
// state code, else its address.stateCode ("" when unknown).
func partyStateCode(vatNo string, address M) string {
	if s := models.GSTINState(vatNo); s != "" {
		return s
	}
	return strings.TrimSpace(str(address["stateCode"]))
}

// gstinStateError: a GSTIN must belong to the state of the address it is
// saved with ("" when they agree or either is missing).
func gstinStateError(p *models.CountryProfile, vatNo string, address M) string {
	if p.TaxSplit != "gst" {
		return ""
	}
	g := models.GSTINState(vatNo)
	sc := strings.TrimSpace(str(address["stateCode"]))
	if g == "" || sc == "" || g == sc {
		return ""
	}
	return "GSTIN state code " + g + " (" + p.StateName(g) + ") does not match the address state " + sc + " (" + p.StateName(sc) + ")"
}

// vatRateError checks a document's VAT/GST rate against the country's rates.
func vatRateError(p *models.CountryProfile, v interface{}) string {
	if v == nil {
		return ""
	}
	r := num(v)
	if p.ValidVatRate(r) {
		return ""
	}
	return p.TaxNameEn + " rate must be one of " + ratesText(p.VatRates)
}

func ratesText(rs []float64) string {
	out := []string{}
	for _, r := range rs {
		out = append(out, strconvFloat(r)+"%")
	}
	return strings.Join(out, ", ")
}

func strconvFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
