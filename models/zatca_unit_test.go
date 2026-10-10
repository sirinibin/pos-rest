package models

import (
	"strings"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGetZatcaUnit_PhysicalUnits(t *testing.T) {
	cases := []struct {
		unit string
		want string
	}{
		{"drum", "DRM"},
		{"Kg", "KGM"},
		{"Meter(s)", "MTR"},
		{"Gm", "GRM"},
		{"L", "LTR"},
		{"Mg", "MG"},
		{"set", "SET"},
		{"MMT", "MMT"},
		{"CMT", "CMT"},
	}
	for _, c := range cases {
		p := OrderProduct{Unit: c.unit, IsService: false}
		if got := p.GetZatcaUnit(); got != c.want {
			t.Errorf("GetZatcaUnit(%q) = %q, want %q", c.unit, got, c.want)
		}
	}
}

func TestGetZatcaUnit_ServiceLegacyStrings(t *testing.T) {
	cases := []struct {
		unit string
		want string
	}{
		{"hour", "HUR"},
		{"day", "DAY"},
		{"month", "MON"},
		{"session", "C62"},
		{"package", "C62"},
		{"visit", "C62"},
	}
	for _, c := range cases {
		p := OrderProduct{Unit: c.unit, IsService: true}
		if got := p.GetZatcaUnit(); got != c.want {
			t.Errorf("GetZatcaUnit(%q) = %q, want %q", c.unit, got, c.want)
		}
	}
}

func TestGetZatcaUnit_PassThroughCodes(t *testing.T) {
	codes := []string{"C62", "HUR", "DAY", "WEE", "MON", "ANN", "EA"}
	for _, code := range codes {
		p := OrderProduct{Unit: code}
		if got := p.GetZatcaUnit(); got != code {
			t.Errorf("GetZatcaUnit(%q) = %q, want pass-through %q", code, got, code)
		}
	}
}

func TestGetZatcaUnit_UnknownUnitIsService_ReturnsC62(t *testing.T) {
	p := OrderProduct{Unit: "consultation", IsService: true}
	if got := p.GetZatcaUnit(); got != "C62" {
		t.Errorf("GetZatcaUnit(unknown service) = %q, want C62", got)
	}
}

func TestGetZatcaUnit_EmptyUnitIsService_ReturnsC62(t *testing.T) {
	p := OrderProduct{Unit: "", IsService: true}
	if got := p.GetZatcaUnit(); got != "C62" {
		t.Errorf("GetZatcaUnit(empty service) = %q, want C62", got)
	}
}

func TestGetZatcaUnit_UnknownUnitNotService_ReturnsPCE(t *testing.T) {
	p := OrderProduct{Unit: "box", IsService: false}
	if got := p.GetZatcaUnit(); got != "PCE" {
		t.Errorf("GetZatcaUnit(unknown physical) = %q, want PCE", got)
	}
}

func TestGetZatcaUnit_EmptyUnitNotService_ReturnsPCE(t *testing.T) {
	p := OrderProduct{Unit: "", IsService: false}
	if got := p.GetZatcaUnit(); got != "PCE" {
		t.Errorf("GetZatcaUnit(empty physical) = %q, want PCE", got)
	}
}

// Two stores' first invoices share a code; their unsigned XML must not share
// a file, or one store would sign the other's invoice.
func TestZatcaXMLPath_ScopedByStore(t *testing.T) {
	a, b := primitive.NewObjectID(), primitive.NewObjectID()
	pa, pb := zatcaXMLPath("invoice", &a, "S-INV-000001"), zatcaXMLPath("invoice", &b, "S-INV-000001")
	if pa == pb {
		t.Fatalf("two stores' invoice S-INV-000001 share %s", pa)
	}
	if want := "ZatcaPython/templates/invoice_" + a.Hex() + "_S-INV-000001.xml"; pa != want {
		t.Fatalf("got %s, want %s", pa, want)
	}
	if zatcaXMLPath("credit_note", &a, "X") == zatcaXMLPath("debit_note", &a, "X") {
		t.Fatal("a credit note and a debit note with the same code share a file")
	}
}

// Documents of different stores are marshalled at the same time; one asking
// for whole amounts without ".00" must not change another's amounts.
func TestMarshalZatcaXML_ConcurrentFormatsStayApart(t *testing.T) {
	type doc struct {
		Percent TaxPercent `xml:"cbc:Percent"`
	}
	var wg sync.WaitGroup
	errs := make(chan string, 400)
	for i := 0; i < 200; i++ {
		raw := i%2 == 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := marshalZatcaXML(doc{Percent: 15}, raw)
			want := "<cbc:Percent>15.00</cbc:Percent>"
			if raw {
				want = "<cbc:Percent>15</cbc:Percent>"
			}
			if err != nil || !strings.Contains(string(out), want) {
				errs <- string(out)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatalf("got %s", e)
	}
	if zatcaRawWholeAmounts {
		t.Fatal("zatcaRawWholeAmounts left on")
	}
}
