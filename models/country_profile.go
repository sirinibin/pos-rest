package models

import (
	"regexp"
	"strings"
)

// CountryProfile is what a store's country decides: currency, VAT, the
// tax-number and phone rules, the address scheme and whether ZATCA applies.
// StartERP supports the six GCC countries; stores saved with any other
// country keep the Saudi rules they always had (see CountryProfileOrSaudi).
// Mirrored by starterp-frontend-v1 src/lib/countryProfiles.js; keep in sync.
type CountryProfile struct {
	Code   string
	NameEn string
	NameAr string

	CurrencyCode     string
	CurrencyNameEn   string
	CurrencyNameAr   string
	CurrencySymbolEn string
	CurrencySymbolAr string
	FractionEn       string
	FractionAr       string
	Decimals         int

	// VatPercent is the standard rate; 0 when the country has no VAT.
	VatPercent float64
	HasVAT     bool
	// TaxIDLabelEn / Ar name the VAT registration number ("TRN" in the UAE).
	TaxIDLabelEn string
	TaxIDLabelAr string
	// TaxIDRequired: the store must have a tax number (Saudi: ZATCA).
	TaxIDRequired bool
	TaxIDRule     *regexp.Regexp
	TaxIDHint     string

	CRLabelEn string
	CRLabelAr string
	CRRule    *regexp.Regexp
	CRHint    string

	DialCode   string
	MobileRule *regexp.Regexp // local or international form, cleaned
	PhoneRule  *regexp.Regexp
	MobileHint string

	// NationalAddress: Saudi National Address rules (4-digit building and
	// additional numbers, 5-digit postal code, Arabic street/district).
	NationalAddress bool
	PostalRule      *regexp.Regexp // nil = optional free text

	// EInvoicing names the live e-invoicing system the app supports
	// ("zatca"), "" when none.
	EInvoicing string

	InvoiceTitleEn string
	InvoiceTitleAr string
	TimeZone       string
}

var reGCCLoose = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9/-]{0,29}$`)

// GCCCountries in display order (Saudi Arabia first, the default).
var GCCCountries = []CountryProfile{
	{
		Code: "SA", NameEn: "Saudi Arabia", NameAr: "المملكة العربية السعودية",
		CurrencyCode: "SAR", CurrencyNameEn: "Saudi Riyal", CurrencyNameAr: "ريال سعودي",
		CurrencySymbolEn: "SAR", CurrencySymbolAr: "ر.س", FractionEn: "Halala", FractionAr: "هللة", Decimals: 2,
		VatPercent: 15, HasVAT: true, TaxIDLabelEn: "VAT No.", TaxIDLabelAr: "الرقم الضريبي", TaxIDRequired: true,
		TaxIDRule: regexp.MustCompile(`^3\d{13}3$`), TaxIDHint: "15 digits, starting and ending with 3",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: regexp.MustCompile(`^\d{10}$`), CRHint: "10 digits",
		DialCode: "966", MobileRule: regexp.MustCompile(`^(05\d{8}|\+?9665\d{8})$`),
		PhoneRule: regexp.MustCompile(`^(0\d{8,9}|\+?966\d{8,9})$`), MobileHint: "05XXXXXXXX",
		NationalAddress: true, PostalRule: regexp.MustCompile(`^\d{5}$`), EInvoicing: "zatca",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Riyadh",
	},
	{
		Code: "AE", NameEn: "United Arab Emirates", NameAr: "الإمارات العربية المتحدة",
		CurrencyCode: "AED", CurrencyNameEn: "UAE Dirham", CurrencyNameAr: "درهم إماراتي",
		CurrencySymbolEn: "AED", CurrencySymbolAr: "د.إ", FractionEn: "Fils", FractionAr: "فلس", Decimals: 2,
		VatPercent: 5, HasVAT: true, TaxIDLabelEn: "TRN", TaxIDLabelAr: "رقم التسجيل الضريبي",
		TaxIDRule: regexp.MustCompile(`^\d{15}$`), TaxIDHint: "15-digit Tax Registration Number",
		CRLabelEn: "Trade licence no.", CRLabelAr: "رقم الرخصة التجارية", CRRule: reGCCLoose, CRHint: "letters, digits, - or /",
		DialCode: "971", MobileRule: regexp.MustCompile(`^(05\d{8}|\+?9715\d{8})$`),
		PhoneRule: regexp.MustCompile(`^(0\d{8,9}|\+?971\d{8,9})$`), MobileHint: "05XXXXXXXX",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Dubai",
	},
	{
		Code: "OM", NameEn: "Oman", NameAr: "سلطنة عُمان",
		CurrencyCode: "OMR", CurrencyNameEn: "Omani Rial", CurrencyNameAr: "ريال عماني",
		CurrencySymbolEn: "OMR", CurrencySymbolAr: "ر.ع.", FractionEn: "Baisa", FractionAr: "بيسة", Decimals: 3,
		VatPercent: 5, HasVAT: true, TaxIDLabelEn: "VATIN", TaxIDLabelAr: "رقم التعريف الضريبي",
		TaxIDRule: regexp.MustCompile(`^OM\d{10}$`), TaxIDHint: "OM followed by 10 digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /",
		DialCode: "968", MobileRule: regexp.MustCompile(`^(\+?968)?[79]\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?968)?[2-9]\d{7}$`), MobileHint: "9XXXXXXX",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Muscat",
	},
	{
		Code: "QA", NameEn: "Qatar", NameAr: "قطر",
		CurrencyCode: "QAR", CurrencyNameEn: "Qatari Riyal", CurrencyNameAr: "ريال قطري",
		CurrencySymbolEn: "QAR", CurrencySymbolAr: "ر.ق", FractionEn: "Dirham", FractionAr: "درهم", Decimals: 2,
		VatPercent: 0, HasVAT: false, TaxIDLabelEn: "Tax No.", TaxIDLabelAr: "الرقم الضريبي",
		TaxIDRule: reGCCLoose, TaxIDHint: "letters and digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /",
		DialCode: "974", MobileRule: regexp.MustCompile(`^(\+?974)?[3567]\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?974)?[2-7]\d{7}$`), MobileHint: "5XXXXXXX",
		InvoiceTitleEn: "Invoice", InvoiceTitleAr: "فاتورة", TimeZone: "Asia/Qatar",
	},
	{
		Code: "BH", NameEn: "Bahrain", NameAr: "البحرين",
		CurrencyCode: "BHD", CurrencyNameEn: "Bahraini Dinar", CurrencyNameAr: "دينار بحريني",
		CurrencySymbolEn: "BHD", CurrencySymbolAr: "د.ب", FractionEn: "Fils", FractionAr: "فلس", Decimals: 3,
		VatPercent: 10, HasVAT: true, TaxIDLabelEn: "VAT account no.", TaxIDLabelAr: "رقم حساب ضريبة القيمة المضافة",
		TaxIDRule: regexp.MustCompile(`^\d{15}$`), TaxIDHint: "15 digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /",
		DialCode: "973", MobileRule: regexp.MustCompile(`^(\+?973)?3\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?973)?[13]\d{7}$`), MobileHint: "3XXXXXXX",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Bahrain",
	},
	{
		Code: "KW", NameEn: "Kuwait", NameAr: "الكويت",
		CurrencyCode: "KWD", CurrencyNameEn: "Kuwaiti Dinar", CurrencyNameAr: "دينار كويتي",
		CurrencySymbolEn: "KWD", CurrencySymbolAr: "د.ك", FractionEn: "Fils", FractionAr: "فلس", Decimals: 3,
		VatPercent: 0, HasVAT: false, TaxIDLabelEn: "Tax No.", TaxIDLabelAr: "الرقم الضريبي",
		TaxIDRule: reGCCLoose, TaxIDHint: "letters and digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /",
		DialCode: "965", MobileRule: regexp.MustCompile(`^(\+?965)?[569]\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?965)?[1-9]\d{7}$`), MobileHint: "5XXXXXXX",
		InvoiceTitleEn: "Invoice", InvoiceTitleAr: "فاتورة", TimeZone: "Asia/Kuwait",
	},
}

// CountryProfileFor is the GCC profile for an ISO code (any case), nil for
// any other country.
func CountryProfileFor(code string) *CountryProfile {
	code = strings.ToUpper(strings.TrimSpace(code))
	for i := range GCCCountries {
		if GCCCountries[i].Code == code {
			return &GCCCountries[i]
		}
	}
	return nil
}

// CountryProfileOrSaudi is CountryProfileFor falling back to Saudi Arabia,
// the rules every store had before GCC support (empty or non-GCC country).
func CountryProfileOrSaudi(code string) *CountryProfile {
	if p := CountryProfileFor(code); p != nil {
		return p
	}
	return &GCCCountries[0]
}

// ZatcaApplies: ZATCA e-invoicing only exists for Saudi stores (and legacy
// stores without a GCC country, which always were Saudi).
func ZatcaApplies(countryCode string) bool {
	return CountryProfileOrSaudi(countryCode).EInvoicing == "zatca"
}

// ValidTaxID checks a tax number against the country's format.
func (p *CountryProfile) ValidTaxID(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	return p.TaxIDRule == nil || p.TaxIDRule.MatchString(s)
}

// ValidCR checks a commercial registration / trade licence number.
func (p *CountryProfile) ValidCR(s string) bool {
	s = strings.TrimSpace(s)
	return p.CRRule == nil || p.CRRule.MatchString(s)
}

var phoneCleaner = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "",
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4", "٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9")

// ValidMobile checks a mobile number in the country's local or +code form.
func (p *CountryProfile) ValidMobile(s string) bool {
	return p.MobileRule.MatchString(phoneCleaner.Replace(strings.TrimSpace(s)))
}

// ValidPhone checks a landline or mobile number (store phone).
func (p *CountryProfile) ValidPhone(s string) bool {
	c := phoneCleaner.Replace(strings.TrimSpace(s))
	return p.PhoneRule.MatchString(c) || p.MobileRule.MatchString(c)
}

// ValidPostal checks the postal code ("" passes when the country has no rule).
func (p *CountryProfile) ValidPostal(s string) bool {
	s = strings.TrimSpace(s)
	if p.PostalRule == nil {
		return len(s) <= 10
	}
	return p.PostalRule.MatchString(s)
}
