package erp

import (
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestNum_LenientParsing(t *testing.T) {
	d128, _ := primitive.ParseDecimal128("12.75")
	tests := []struct {
		name string
		in   interface{}
		want float64
	}{
		{"nil", nil, 0},
		{"float64", 12.5, 12.5},
		{"int32 (legacy FlexInt)", int32(7), 7},
		{"int64", int64(9), 9},
		{"string number", "15", 15},
		{"string with comma", "1,250.50", 1250.5},
		{"arabic-indic digits", "١٢٣٫٥", 123.5},
		{"blank string", "  ", 0},
		{"garbage", "abc", 0},
		{"bool true", true, 1},
		{"json.Number", json.Number("3.25"), 3.25},
		{"decimal128", d128, 12.75},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := num(tc.in); got != tc.want {
				t.Fatalf("num(%v)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestStrAndBool(t *testing.T) {
	oid := primitive.NewObjectID()
	cases := []struct {
		in   interface{}
		want string
	}{
		{nil, ""}, {"x", "x"}, {oid, oid.Hex()}, {&oid, oid.Hex()}, {primitive.NilObjectID, ""},
		{15.0, "15"}, {15.25, "15.25"}, {int32(4), "4"}, {true, "true"},
	}
	for _, c := range cases {
		if got := str(c.in); got != c.want {
			t.Errorf("str(%#v)=%q want %q", c.in, got, c.want)
		}
	}
	if !boolv("true") || boolv("false") || !boolv(1.0) || boolv(nil) || boolv(0) {
		t.Fatal("boolv mismatch")
	}
}

func TestRound2_MatchesClientSe(t *testing.T) {
	cases := map[float64]float64{
		1.005:   1.01, // binary artefact handled like toPrecision(15)
		2.675:   2.68,
		-1.005:  -1.01,
		806.49:  806.49,
		5376.6:  5376.6,
		0.125:   0.13,
		0:       0,
		6183.09: 6183.09,
	}
	for in, want := range cases {
		if got := round2(in); got != want {
			t.Errorf("round2(%v)=%v want %v", in, got, want)
		}
	}
}

func TestDates_RiyadhLocal(t *testing.T) {
	utc := time.Date(2026, 10, 2, 7, 30, 0, 0, time.UTC)
	if got := fmtDT(primitive.NewDateTimeFromTime(utc)); got != "2026-10-02T10:30" {
		t.Fatalf("fmtDT=%s", got)
	}
	if got := fmtDay(utc); got != "2026-10-02" {
		t.Fatalf("fmtDay=%s", got)
	}
	if fmtDT(nil) != "" || fmtDT("not a date") != "" {
		t.Fatal("expected empty for non-times")
	}
	tt, err := parseClientTime("2026-10-02T10:30")
	if err != nil || !tt.Equal(utc) {
		t.Fatalf("parseClientTime local: %v %v", tt, err)
	}
	if _, err := parseClientTime("2026-10-02"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseClientTime("2026-10"); err != nil {
		t.Fatal("YYYY-MM should parse (salary period)")
	}
	if _, err := parseClientTime("garbage"); err == nil {
		t.Fatal("expected error")
	}
	s, err := toLegacyDateStr("2026-10-02T10:30")
	if err != nil || s != "2026-10-02T10:30:00+03:00" {
		t.Fatalf("toLegacyDateStr=%s %v", s, err)
	}
	if s, _ := toLegacyDateStr("2026-10-02T07:30:00Z"); s != "2026-10-02T10:30:00+03:00" {
		t.Fatalf("RFC3339 input → %s", s)
	}
}

func TestIDs(t *testing.T) {
	oid := primitive.NewObjectID()
	if h := hexOf(oid); h != oid.Hex() {
		t.Fatal(h)
	}
	if h := hexOf(oid.Hex()); h != oid.Hex() {
		t.Fatal(h)
	}
	if hexOf("cus_abc") != "cus_abc" {
		t.Fatal("non-hex strings pass through")
	}
	if idOrNil(nil) != nil || idOrNil(primitive.NilObjectID) != nil {
		t.Fatal("expected nil")
	}
	if _, ok := oidOf("zz"); ok {
		t.Fatal("invalid hex must not parse")
	}
	got := ids(bson.A{oid, nil, oid.Hex(), primitive.NilObjectID})
	if len(got) != 2 {
		t.Fatalf("ids=%v", got)
	}
}

func TestNorm_ConvertsDriverTypes(t *testing.T) {
	in := primitive.D{{Key: "a", Value: primitive.A{primitive.D{{Key: "b", Value: int32(1)}}}}, {Key: "m", Value: primitive.M{"x": "y"}}}
	out := normDoc(in)
	arrv := arr(out["a"])
	if len(arrv) != 1 {
		t.Fatalf("%#v", out)
	}
	if _, ok := arrv[0].(M); !ok {
		t.Fatalf("nested doc not normalized: %#v", arrv[0])
	}
	if get(out, "m.x") != "y" {
		t.Fatalf("get path failed: %#v", out)
	}
	if get(out, "m.x.y") != nil || get(nil, "a") != nil {
		t.Fatal("expected nil for missing path")
	}
}

func TestValidators(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string) bool
		in   string
		want bool
	}{
		{"vat ok", ValidVAT, "310122393500003", true},
		{"vat 14 digits", ValidVAT, "31012239350003", false},
		{"vat not ending 3", ValidVAT, "310122393500001", false},
		{"vat starting 2", ValidVAT, "210122393500003", false},
		{"cr ok", ValidCR, "1010101010", true},
		{"cr 9", ValidCR, "101010101", false},
		{"cr letters", ValidCR, "10101A1010", false},
		{"mobile 05", ValidSaudiMobile, "0512345678", true},
		{"mobile spaced", ValidSaudiMobile, "05 1234 5678", true},
		{"mobile +966", ValidSaudiMobile, "+966512345678", true},
		{"mobile landline", ValidSaudiMobile, "0112345678", false},
		{"phone landline", ValidSaudiPhone, "0112345678", true},
		{"phone short", ValidSaudiPhone, "01123", false},
	}
	for _, c := range cases {
		if got := c.fn(c.in); got != c.want {
			t.Errorf("%s: %q → %v want %v", c.name, c.in, got, c.want)
		}
	}
	if !hasArabic("شركة") || hasArabic("Company") {
		t.Fatal("hasArabic")
	}
}

func TestToArabicDigitsAndPad(t *testing.T) {
	if toArabicDigits("12345") != "١٢٣٤٥" {
		t.Fatal(toArabicDigits("12345"))
	}
	if leftPad(7, 4) != "0007" || pad2(3) != "03" || pad2(11) != "11" {
		t.Fatal("padding")
	}
}
