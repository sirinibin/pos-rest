package erp

import (
	"net/http"
	"strings"

	"github.com/sirinibin/startpos/backend/models"
)

// Store country rules (GCC): currency, VAT, tax-number/phone/address formats
// and ZATCA availability come from models.CountryProfile.

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
	return M{"code": p.Code, "nameEn": p.NameEn, "nameAr": p.NameAr, "gcc": true,
		"hasVat": p.HasVAT, "vatPercent": p.VatPercent,
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
	return p.TaxIDLabelEn + ": " + p.TaxIDHint
}

// validPhoneFor accepts the country's mobiles and landlines; Saudi keeps
// ValidSaudiMobile/ValidSaudiPhone.
func validPhoneFor(p *models.CountryProfile, v string) bool {
	if p.Code == "SA" {
		return ValidSaudiMobile(v) || ValidSaudiPhone(v)
	}
	return p.ValidPhone(v)
}

// validAnyGCCPhone: a user's phone may be from any supported country.
func validAnyGCCPhone(v string) bool {
	for i := range models.GCCCountries {
		if validPhoneFor(&models.GCCCountries[i], v) {
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
// National Address digit rules; the other countries only bound the postal
// code.
func addressErrorsFor(p *models.CountryProfile, a M, prefix string) map[string]string {
	e := map[string]string{}
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
		e[prefix+"postalCode"] = "invalid postal code"
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
	for i := range models.GCCCountries {
		p := &models.GCCCountries[i]
		c := countryContract(p)
		c["currency"] = currencyContract(p)
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, M{"data": out})
}

// normCountry upper-cases and trims an ISO code.
func normCountry(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
