package models

import (
	"regexp"
	"strings"
)

// CountryProfile is what a store's country decides: currency, VAT/GST, the
// tax-number and phone rules, the address scheme and whether ZATCA applies.
// StartERP supports the six GCC countries and India; stores saved with any
// other country keep the Saudi rules they always had (see
// CountryProfileOrSaudi).
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

	// GCC: one of the six Gulf countries (Arabic names and addresses are
	// required there; India keeps them optional).
	GCC bool

	// VatPercent is the standard rate; 0 when the country has no VAT.
	VatPercent float64
	HasVAT     bool
	// TaxNameEn / Ar: what the tax is called ("VAT", India "GST").
	TaxNameEn string
	TaxNameAr string
	// VatRates are the rates a document may use (nil = any 0..100).
	VatRates []float64
	// TaxSplit "gst": India's CGST+SGST (intra-state) or IGST (inter-state).
	TaxSplit string
	// TaxIDLabelEn / Ar name the VAT registration number ("TRN" in the UAE).
	TaxIDLabelEn string
	TaxIDLabelAr string
	// TaxIDRequired: the store must have a tax number (Saudi: ZATCA).
	TaxIDRequired bool
	TaxIDRule     *regexp.Regexp
	TaxIDHint     string
	// TaxIDCheck is an extra check on top of TaxIDRule (GSTIN checksum).
	TaxIDCheck func(string) bool

	CRLabelEn string
	CRLabelAr string
	CRRule    *regexp.Regexp
	CRHint    string
	// CRRequired: stores must give the CR / licence number (GCC).
	CRRequired bool

	DialCode   string
	MobileRule *regexp.Regexp // local or international form, cleaned
	PhoneRule  *regexp.Regexp
	MobileHint string

	// NationalAddress: Saudi National Address rules (4-digit building and
	// additional numbers, 5-digit postal code, Arabic street/district).
	NationalAddress bool
	PostalRule      *regexp.Regexp // nil = optional free text
	PostalHint      string
	// States: the country's states with their codes (India: GST state
	// codes); a store's address must name one. nil = no states.
	States []CountryState

	// EInvoicing names the live e-invoicing system the app supports
	// ("zatca"), "" when none.
	EInvoicing string

	InvoiceTitleEn string
	InvoiceTitleAr string
	TimeZone       string
	// RoundingStep: cash rounding of a document total (India: nearest ₹1;
	// 0 = the app's default 0.05).
	RoundingStep float64
}

// CountryState is a state / union territory and its code (GST state code).
type CountryState struct {
	Code string
	Name string
}

var reGCCLoose = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9/-]{0,29}$`)

// Countries in display order: the GCC (Saudi Arabia first, the default),
// then India.
var Countries = []CountryProfile{
	{
		Code: "SA", NameEn: "Saudi Arabia", NameAr: "المملكة العربية السعودية",
		CurrencyCode: "SAR", CurrencyNameEn: "Saudi Riyal", CurrencyNameAr: "ريال سعودي",
		CurrencySymbolEn: "SAR", CurrencySymbolAr: "ر.س", FractionEn: "Halala", FractionAr: "هللة", Decimals: 2,
		GCC: true, VatPercent: 15, HasVAT: true, TaxNameEn: "VAT", TaxNameAr: "ضريبة القيمة المضافة", TaxIDLabelEn: "VAT No.", TaxIDLabelAr: "الرقم الضريبي", TaxIDRequired: true,
		TaxIDRule: regexp.MustCompile(`^3\d{13}3$`), TaxIDHint: "15 digits, starting and ending with 3",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: regexp.MustCompile(`^\d{10}$`), CRHint: "10 digits", CRRequired: true,
		DialCode: "966", MobileRule: regexp.MustCompile(`^(05\d{8}|\+?9665\d{8})$`),
		PhoneRule: regexp.MustCompile(`^(0\d{8,9}|\+?966\d{8,9})$`), MobileHint: "05XXXXXXXX",
		NationalAddress: true, PostalRule: regexp.MustCompile(`^\d{5}$`), EInvoicing: "zatca",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Riyadh",
	},
	{
		Code: "AE", NameEn: "United Arab Emirates", NameAr: "الإمارات العربية المتحدة",
		CurrencyCode: "AED", CurrencyNameEn: "UAE Dirham", CurrencyNameAr: "درهم إماراتي",
		CurrencySymbolEn: "AED", CurrencySymbolAr: "د.إ", FractionEn: "Fils", FractionAr: "فلس", Decimals: 2,
		GCC: true, VatPercent: 5, HasVAT: true, TaxNameEn: "VAT", TaxNameAr: "ضريبة القيمة المضافة", TaxIDLabelEn: "TRN", TaxIDLabelAr: "رقم التسجيل الضريبي",
		TaxIDRule: regexp.MustCompile(`^\d{15}$`), TaxIDHint: "15-digit Tax Registration Number",
		CRLabelEn: "Trade licence no.", CRLabelAr: "رقم الرخصة التجارية", CRRule: reGCCLoose, CRHint: "letters, digits, - or /", CRRequired: true,
		DialCode: "971", MobileRule: regexp.MustCompile(`^(05\d{8}|\+?9715\d{8})$`),
		PhoneRule: regexp.MustCompile(`^(0\d{8,9}|\+?971\d{8,9})$`), MobileHint: "05XXXXXXXX",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Dubai",
	},
	{
		Code: "OM", NameEn: "Oman", NameAr: "سلطنة عُمان",
		CurrencyCode: "OMR", CurrencyNameEn: "Omani Rial", CurrencyNameAr: "ريال عماني",
		CurrencySymbolEn: "OMR", CurrencySymbolAr: "ر.ع.", FractionEn: "Baisa", FractionAr: "بيسة", Decimals: 3,
		GCC: true, VatPercent: 5, HasVAT: true, TaxNameEn: "VAT", TaxNameAr: "ضريبة القيمة المضافة", TaxIDLabelEn: "VATIN", TaxIDLabelAr: "رقم التعريف الضريبي",
		TaxIDRule: regexp.MustCompile(`^OM\d{10}$`), TaxIDHint: "OM followed by 10 digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /", CRRequired: true,
		DialCode: "968", MobileRule: regexp.MustCompile(`^(\+?968)?[79]\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?968)?[2-9]\d{7}$`), MobileHint: "9XXXXXXX",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Muscat",
	},
	{
		Code: "QA", NameEn: "Qatar", NameAr: "قطر",
		CurrencyCode: "QAR", CurrencyNameEn: "Qatari Riyal", CurrencyNameAr: "ريال قطري",
		CurrencySymbolEn: "QAR", CurrencySymbolAr: "ر.ق", FractionEn: "Dirham", FractionAr: "درهم", Decimals: 2,
		GCC: true, VatPercent: 0, HasVAT: false, TaxNameEn: "VAT", TaxNameAr: "ضريبة القيمة المضافة", TaxIDLabelEn: "Tax No.", TaxIDLabelAr: "الرقم الضريبي",
		TaxIDRule: reGCCLoose, TaxIDHint: "letters and digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /", CRRequired: true,
		DialCode: "974", MobileRule: regexp.MustCompile(`^(\+?974)?[3567]\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?974)?[2-7]\d{7}$`), MobileHint: "5XXXXXXX",
		InvoiceTitleEn: "Invoice", InvoiceTitleAr: "فاتورة", TimeZone: "Asia/Qatar",
	},
	{
		Code: "BH", NameEn: "Bahrain", NameAr: "البحرين",
		CurrencyCode: "BHD", CurrencyNameEn: "Bahraini Dinar", CurrencyNameAr: "دينار بحريني",
		CurrencySymbolEn: "BHD", CurrencySymbolAr: "د.ب", FractionEn: "Fils", FractionAr: "فلس", Decimals: 3,
		GCC: true, VatPercent: 10, HasVAT: true, TaxNameEn: "VAT", TaxNameAr: "ضريبة القيمة المضافة", TaxIDLabelEn: "VAT account no.", TaxIDLabelAr: "رقم حساب ضريبة القيمة المضافة",
		TaxIDRule: regexp.MustCompile(`^\d{15}$`), TaxIDHint: "15 digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /", CRRequired: true,
		DialCode: "973", MobileRule: regexp.MustCompile(`^(\+?973)?3\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?973)?[13]\d{7}$`), MobileHint: "3XXXXXXX",
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Bahrain",
	},
	{
		Code: "KW", NameEn: "Kuwait", NameAr: "الكويت",
		CurrencyCode: "KWD", CurrencyNameEn: "Kuwaiti Dinar", CurrencyNameAr: "دينار كويتي",
		CurrencySymbolEn: "KWD", CurrencySymbolAr: "د.ك", FractionEn: "Fils", FractionAr: "فلس", Decimals: 3,
		GCC: true, VatPercent: 0, HasVAT: false, TaxNameEn: "VAT", TaxNameAr: "ضريبة القيمة المضافة", TaxIDLabelEn: "Tax No.", TaxIDLabelAr: "الرقم الضريبي",
		TaxIDRule: reGCCLoose, TaxIDHint: "letters and digits",
		CRLabelEn: "CR No.", CRLabelAr: "السجل التجاري", CRRule: reGCCLoose, CRHint: "letters, digits, - or /", CRRequired: true,
		DialCode: "965", MobileRule: regexp.MustCompile(`^(\+?965)?[569]\d{7}$`),
		PhoneRule: regexp.MustCompile(`^(\+?965)?[1-9]\d{7}$`), MobileHint: "5XXXXXXX",
		InvoiceTitleEn: "Invoice", InvoiceTitleAr: "فاتورة", TimeZone: "Asia/Kuwait",
	},
	{
		Code: "IN", NameEn: "India", NameAr: "الهند",
		CurrencyCode: "INR", CurrencyNameEn: "Indian Rupee", CurrencyNameAr: "روبية هندية",
		CurrencySymbolEn: "₹", CurrencySymbolAr: "₹", FractionEn: "Paise", FractionAr: "بيسة", Decimals: 2,
		VatPercent: 18, HasVAT: true, TaxNameEn: "GST", TaxNameAr: "ضريبة السلع والخدمات",
		VatRates: IndiaGSTRates, TaxSplit: "gst",
		TaxIDLabelEn: "GSTIN", TaxIDLabelAr: "رقم GSTIN",
		TaxIDRule: reGSTIN, TaxIDCheck: ValidGSTINChecksum, TaxIDHint: "15-character GSTIN, e.g. 27ABCDE1234F1Z5",
		CRLabelEn: "PAN", CRLabelAr: "رقم PAN", CRRule: rePAN, CRHint: "10 characters, e.g. ABCDE1234F",
		DialCode: "91", MobileRule: regexp.MustCompile(`^(\+?91|0)?[6-9]\d{9}$`),
		PhoneRule: regexp.MustCompile(`^(\+?91|0)?[1-9]\d{9}$`), MobileHint: "98XXXXXXXX",
		PostalRule: regexp.MustCompile(`^[1-9]\d{5}$`), PostalHint: "6-digit PIN code",
		States: IndiaStates, RoundingStep: 1,
		InvoiceTitleEn: "Tax Invoice", InvoiceTitleAr: "فاتورة ضريبية", TimeZone: "Asia/Kolkata",
	},
}

// CountryProfileFor is the profile of a supported country (ISO code, any
// case), nil for any other country.
func CountryProfileFor(code string) *CountryProfile {
	code = strings.ToUpper(strings.TrimSpace(code))
	for i := range Countries {
		if Countries[i].Code == code {
			return &Countries[i]
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
	return &Countries[0]
}

// ZatcaApplies: ZATCA e-invoicing only exists for Saudi stores (and legacy
// stores without a GCC country, which always were Saudi).
func ZatcaApplies(countryCode string) bool {
	return CountryProfileOrSaudi(countryCode).EInvoicing == "zatca"
}

// ValidTaxID checks a tax number against the country's format (and the
// GSTIN checksum in India).
func (p *CountryProfile) ValidTaxID(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	if p.TaxIDRule != nil && !p.TaxIDRule.MatchString(s) {
		return false
	}
	return p.TaxIDCheck == nil || p.TaxIDCheck(s)
}

// ValidCR checks a commercial registration / trade licence number (PAN in
// India, any case).
func (p *CountryProfile) ValidCR(s string) bool {
	s = strings.TrimSpace(s)
	if p.Code == "IN" {
		s = strings.ToUpper(s)
	}
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

// ValidVatRate: a document's VAT/GST rate is one the country uses (any
// 0..100 where the country lists none).
func (p *CountryProfile) ValidVatRate(r float64) bool {
	if r < 0 || r > 100 {
		return false
	}
	if len(p.VatRates) == 0 || r == 0 {
		return true
	}
	for _, v := range p.VatRates {
		if v == r {
			return true
		}
	}
	return false
}

// StateName is the name of a state code ("" when unknown).
func (p *CountryProfile) StateName(code string) string {
	code = strings.TrimSpace(code)
	for _, s := range p.States {
		if s.Code == code {
			return s.Name
		}
	}
	return ""
}

// StateByName finds a state by its name or code (any case), nil if none.
func (p *CountryProfile) StateByName(v string) *CountryState {
	v = strings.TrimSpace(v)
	for i := range p.States {
		if p.States[i].Code == v || strings.EqualFold(p.States[i].Name, v) {
			return &p.States[i]
		}
	}
	return nil
}

// StoreCountryProfile is the store's country profile (Saudi Arabia for an
// empty or unknown country, and for a nil store).
func StoreCountryProfile(store *Store) *CountryProfile {
	if store == nil {
		return CountryProfileOrSaudi("")
	}
	return CountryProfileOrSaudi(store.CountryCode)
}

// ValidStorePhone is the legacy document/warehouse phone check: Saudi stores
// keep the Saudi mobile rule, other countries take their own mobile and
// landline numbers.
func ValidStorePhone(store *Store, phone string) bool {
	p := StoreCountryProfile(store)
	if p.Code == "SA" {
		return ValidateSaudiPhone(phone)
	}
	return p.ValidMobile(phone) || p.ValidPhone(phone)
}

// StoreVATNoError is the legacy tax-number check ("" when valid): Saudi
// stores keep "15 digits, starting and ending with 3"; other countries use
// their own format (UAE TRN, India GSTIN with its checksum, …).
func StoreVATNoError(store *Store, vat string) string {
	vat = strings.TrimSpace(vat)
	if vat == "" {
		return ""
	}
	p := StoreCountryProfile(store)
	if p.Code != "SA" {
		if p.ValidTaxID(vat) {
			return ""
		}
		return p.TaxIDLabelEn + ": " + p.TaxIDHint
	}
	if !IsValidDigitNumber(vat, "15") {
		return "VAT No. should be 15 digits"
	}
	if !IsNumberStartAndEndWith(vat, "3") {
		return "VAT No. should start and end with 3"
	}
	return ""
}
