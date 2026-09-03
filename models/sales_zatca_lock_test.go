package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// boolPtr returns a pointer to the supplied bool — matches the *bool field type.
func boolPtr(b bool) *bool { return &b }

// evaluateLock mirrors the inline expression in sales.go:
//
//	disableSalesEditLock := store.Settings.DisableSalesEditOnceReportedToZatca == nil ||
//	                        *store.Settings.DisableSalesEditOnceReportedToZatca
func evaluateLock(setting *bool) bool {
	return setting == nil || *setting
}

// ── DisableSalesEditOnceReportedToZatca: field semantics ─────────────────────

func TestDisableSalesEditLock_NilMeansLocked(t *testing.T) {
	// nil (field absent in MongoDB doc for existing stores) must default to locked.
	if !evaluateLock(nil) {
		t.Error("nil setting must evaluate to locked (true)")
	}
}

func TestDisableSalesEditLock_ExplicitTrueMeansLocked(t *testing.T) {
	if !evaluateLock(boolPtr(true)) {
		t.Error("*true setting must evaluate to locked (true)")
	}
}

func TestDisableSalesEditLock_ExplicitFalseMeansUnlocked(t *testing.T) {
	if evaluateLock(boolPtr(false)) {
		t.Error("*false setting must evaluate to unlocked (false)")
	}
}

// ── DisableSalesEditOnceReportedToZatca: StoreSettings struct ────────────────

func TestDisableSalesEdit_FieldZeroValueIsNil(t *testing.T) {
	var s StoreSettings
	if s.DisableSalesEditOnceReportedToZatca != nil {
		t.Errorf("zero-value DisableSalesEditOnceReportedToZatca should be nil, got %v", s.DisableSalesEditOnceReportedToZatca)
	}
}

func TestDisableSalesEdit_ZeroValueImpliesLocked(t *testing.T) {
	var s StoreSettings
	if !evaluateLock(s.DisableSalesEditOnceReportedToZatca) {
		t.Error("zero-value (nil) field must evaluate to locked")
	}
}

// ── DisableSalesEditOnceReportedToZatca: JSON key name ───────────────────────

func TestDisableSalesEdit_JSONKeyName(t *testing.T) {
	s := StoreSettings{DisableSalesEditOnceReportedToZatca: boolPtr(true)}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"disable_sales_edit_once_reported_to_zatca"`) {
		t.Errorf("expected JSON key 'disable_sales_edit_once_reported_to_zatca' in: %s", data)
	}
}

func TestDisableSalesEdit_NilMarshalsAsNull(t *testing.T) {
	s := StoreSettings{DisableSalesEditOnceReportedToZatca: nil}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	v, ok := m["disable_sales_edit_once_reported_to_zatca"]
	if !ok {
		t.Fatal("key 'disable_sales_edit_once_reported_to_zatca' must be present even when nil (no omitempty)")
	}
	if v != nil {
		t.Errorf("nil pointer must marshal as JSON null, got %v", v)
	}
}

func TestDisableSalesEdit_TrueMarshalledAsTrue(t *testing.T) {
	s := StoreSettings{DisableSalesEditOnceReportedToZatca: boolPtr(true)}
	data, _ := json.Marshal(s)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if m["disable_sales_edit_once_reported_to_zatca"] != true {
		t.Errorf("expected true, got %v", m["disable_sales_edit_once_reported_to_zatca"])
	}
}

func TestDisableSalesEdit_FalseMarshalledAsFalse(t *testing.T) {
	s := StoreSettings{DisableSalesEditOnceReportedToZatca: boolPtr(false)}
	data, _ := json.Marshal(s)
	var m map[string]interface{}
	json.Unmarshal(data, &m)
	if m["disable_sales_edit_once_reported_to_zatca"] != false {
		t.Errorf("expected false, got %v", m["disable_sales_edit_once_reported_to_zatca"])
	}
}

// ── DisableSalesEditOnceReportedToZatca: JSON round-trips ────────────────────

func TestDisableSalesEdit_RoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		setting *bool
	}{
		{"nil stays nil", nil},
		{"true stays true", boolPtr(true)},
		{"false stays false", boolPtr(false)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orig := StoreSettings{DisableSalesEditOnceReportedToZatca: c.setting}
			data, err := json.Marshal(orig)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded StoreSettings
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if c.setting == nil {
				if decoded.DisableSalesEditOnceReportedToZatca != nil {
					t.Errorf("nil round-trip: got %v, want nil", decoded.DisableSalesEditOnceReportedToZatca)
				}
			} else {
				if decoded.DisableSalesEditOnceReportedToZatca == nil {
					t.Fatal("round-trip: got nil, want non-nil pointer")
				}
				if *decoded.DisableSalesEditOnceReportedToZatca != *c.setting {
					t.Errorf("round-trip: got %v, want %v", *decoded.DisableSalesEditOnceReportedToZatca, *c.setting)
				}
			}
		})
	}
}

func TestDisableSalesEdit_UnmarshalFromJSONString(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantNil    bool
		wantValue  bool
		wantLocked bool
	}{
		{
			name:       "missing key → nil → locked (backward compat)",
			input:      `{}`,
			wantNil:    true,
			wantLocked: true,
		},
		{
			name:       "null → nil → locked",
			input:      `{"disable_sales_edit_once_reported_to_zatca": null}`,
			wantNil:    true,
			wantLocked: true,
		},
		{
			name:       "true → locked",
			input:      `{"disable_sales_edit_once_reported_to_zatca": true}`,
			wantNil:    false,
			wantValue:  true,
			wantLocked: true,
		},
		{
			name:       "false → unlocked",
			input:      `{"disable_sales_edit_once_reported_to_zatca": false}`,
			wantNil:    false,
			wantValue:  false,
			wantLocked: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var s StoreSettings
			if err := json.Unmarshal([]byte(c.input), &s); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if c.wantNil {
				if s.DisableSalesEditOnceReportedToZatca != nil {
					t.Errorf("expected nil, got %v", s.DisableSalesEditOnceReportedToZatca)
				}
			} else {
				if s.DisableSalesEditOnceReportedToZatca == nil {
					t.Fatal("expected non-nil pointer")
				}
				if *s.DisableSalesEditOnceReportedToZatca != c.wantValue {
					t.Errorf("*field = %v, want %v", *s.DisableSalesEditOnceReportedToZatca, c.wantValue)
				}
			}
			got := evaluateLock(s.DisableSalesEditOnceReportedToZatca)
			if got != c.wantLocked {
				t.Errorf("evaluateLock = %v, want %v", got, c.wantLocked)
			}
		})
	}
}

// ── Full lock-condition gate: Phase 2 + ReportingPassed + setting ─────────────

// evaluateFullLock mirrors the complete if-condition from sales.go line 2031,
// minus the DB-bound scenario/oldOrder checks (tested via integration tests).
func evaluateFullLock(phase string, reportingPassed bool, setting *bool) bool {
	disableSalesEditLock := setting == nil || *setting
	return reportingPassed && phase == "2" && disableSalesEditLock
}

func TestFullLockCondition(t *testing.T) {
	cases := []struct {
		name            string
		phase           string
		reportingPassed bool
		setting         *bool
		wantLocked      bool
	}{
		// Phase 2 + reported + nil setting (existing stores) → locked
		{"phase2 reported nil-setting → locked", "2", true, nil, true},
		// Phase 2 + reported + explicit true → locked
		{"phase2 reported explicit-true → locked", "2", true, boolPtr(true), true},
		// Phase 2 + reported + explicit false → NOT locked
		{"phase2 reported explicit-false → unlocked", "2", true, boolPtr(false), false},
		// Phase 1 (or no phase) + reported + nil → NOT locked (phase gate)
		{"phase1 reported nil-setting → unlocked", "1", true, nil, false},
		{"no-phase reported nil-setting → unlocked", "", true, nil, false},
		// Phase 2 + NOT reported + nil → NOT locked (not reported yet)
		{"phase2 not-reported nil-setting → unlocked", "2", false, nil, false},
		// Phase 2 + NOT reported + explicit false → NOT locked
		{"phase2 not-reported explicit-false → unlocked", "2", false, boolPtr(false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := evaluateFullLock(c.phase, c.reportingPassed, c.setting)
			if got != c.wantLocked {
				t.Errorf("got locked=%v, want %v", got, c.wantLocked)
			}
		})
	}
}
