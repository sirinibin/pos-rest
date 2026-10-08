package models

import (
	"regexp"
	"strings"
)

// India: GST rates, states (with their GST state codes), GSTIN and PAN.
// Mirrored by starterp-frontend-v1 src/lib/india.js; keep in sync.

// IndiaGSTRates are the GST rates since GST 2.0 (22 Sep 2025): 5% and 18%
// with 40% for luxury and sin goods (0% for exempt / nil-rated supplies),
// plus 3% for gold, silver and jewellery and 0.25% for rough diamonds.
var IndiaGSTRates = []float64{0, 5, 18, 40, 3, 0.25}

// IndiaStates: states and union territories with their GST state codes
// (the first two digits of a GSTIN). 25 (Daman and Diu) merged into 26 in
// 2020; 97 is "Other Territory".
var IndiaStates = []CountryState{
	{"01", "Jammu and Kashmir"}, {"02", "Himachal Pradesh"}, {"03", "Punjab"}, {"04", "Chandigarh"},
	{"05", "Uttarakhand"}, {"06", "Haryana"}, {"07", "Delhi"}, {"08", "Rajasthan"}, {"09", "Uttar Pradesh"},
	{"10", "Bihar"}, {"11", "Sikkim"}, {"12", "Arunachal Pradesh"}, {"13", "Nagaland"}, {"14", "Manipur"},
	{"15", "Mizoram"}, {"16", "Tripura"}, {"17", "Meghalaya"}, {"18", "Assam"}, {"19", "West Bengal"},
	{"20", "Jharkhand"}, {"21", "Odisha"}, {"22", "Chhattisgarh"}, {"23", "Madhya Pradesh"}, {"24", "Gujarat"},
	{"26", "Dadra and Nagar Haveli and Daman and Diu"}, {"27", "Maharashtra"}, {"29", "Karnataka"}, {"30", "Goa"},
	{"31", "Lakshadweep"}, {"32", "Kerala"}, {"33", "Tamil Nadu"}, {"34", "Puducherry"},
	{"35", "Andaman and Nicobar Islands"}, {"36", "Telangana"}, {"37", "Andhra Pradesh"}, {"38", "Ladakh"},
	{"97", "Other Territory"},
}

// reGSTIN: 2-digit state code, 10-character PAN, entity number (1-9, A-Z),
// "Z" and a check character.
var reGSTIN = regexp.MustCompile(`^\d{2}[A-Z]{5}\d{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)

// rePAN: five letters, four digits, one letter.
var rePAN = regexp.MustCompile(`^[A-Z]{5}\d{4}[A-Z]$`)

const gstinChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GSTINCheckChar is the 15th character of a GSTIN computed from its first
// 14 (the GSTN mod-36 scheme); "" when the input has other characters.
func GSTINCheckChar(first14 string) string {
	first14 = strings.ToUpper(first14)
	if len(first14) != 14 {
		return ""
	}
	sum := 0
	for i, r := range first14 {
		v := strings.IndexRune(gstinChars, r)
		if v < 0 {
			return ""
		}
		f := 1
		if i%2 == 1 {
			f = 2
		}
		p := v * f
		sum += p/36 + p%36
	}
	return string(gstinChars[(36-sum%36)%36])
}

// ValidGSTINChecksum: the GSTIN's check character matches and its state
// code is a real state.
func ValidGSTINChecksum(g string) bool {
	g = strings.ToUpper(strings.TrimSpace(g))
	if !reGSTIN.MatchString(g) {
		return false
	}
	if !indiaStateKnown(g[:2]) {
		return false
	}
	return GSTINCheckChar(g[:14]) == g[14:]
}

// ValidGSTIN: format, state code and checksum.
func ValidGSTIN(g string) bool { return ValidGSTINChecksum(g) }

// ValidPAN checks a PAN (any case).
func ValidPAN(p string) bool { return rePAN.MatchString(strings.ToUpper(strings.TrimSpace(p))) }

// GSTINState is the state code of a GSTIN ("" when it is not one).
func GSTINState(g string) string {
	g = strings.ToUpper(strings.TrimSpace(g))
	if !reGSTIN.MatchString(g) {
		return ""
	}
	return g[:2]
}

func indiaStateKnown(code string) bool {
	for _, s := range IndiaStates {
		if s.Code == code {
			return true
		}
	}
	return false
}

// GSTSplit is how a document's GST is charged: CGST + SGST (half each) for a
// supply within the supplier's state, IGST between states. An unknown place
// of supply (walk-in customer) is the supplier's state.
type GSTSplit struct {
	Inter bool
	CGST  float64
	SGST  float64
	IGST  float64
}

// SplitGST splits a GST amount by place of supply; the paise left after
// halving go to SGST so CGST + SGST always equals the total.
func SplitGST(tax float64, supplierState, placeOfSupply string) GSTSplit {
	if placeOfSupply != "" && supplierState != "" && placeOfSupply != supplierState {
		return GSTSplit{Inter: true, IGST: RoundTo2Decimals(tax)}
	}
	c := RoundTo2Decimals(tax / 2)
	return GSTSplit{CGST: c, SGST: RoundTo2Decimals(tax - c)}
}
