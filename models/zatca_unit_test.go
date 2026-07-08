package models

import "testing"

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
