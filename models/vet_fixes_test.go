package models

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// sanitizeUTF8 used `؀` in a regexp (invalid in Go), so the first
// invalid-UTF-8 product name panicked in regexp.MustCompile.
func TestSanitizeUTF8_InvalidInputDoesNotPanic(t *testing.T) {
	got := sanitizeUTF8("Oil\xff filter زيت")
	if got != "Oil filter زيت" {
		t.Errorf("sanitizeUTF8 = %q", got)
	}
	if got := sanitizeUTF8("valid زيت"); got != "valid زيت" {
		t.Errorf("valid input changed: %q", got)
	}
}

// The malformed tag made the driver store this flag as "simplifieddebitnote";
// the fixed tag must keep that key so existing stores still read it.
func TestComplianceCheck_SimplifiedDebitNoteKeepsStoredKey(t *testing.T) {
	raw, err := bson.Marshal(ComplianceCheck{SimplifiedDebitNote: true})
	if err != nil {
		t.Fatal(err)
	}
	var m bson.M
	_ = bson.Unmarshal(raw, &m)
	if m["simplifieddebitnote"] != true {
		t.Errorf("stored keys = %v, want simplifieddebitnote=true", m)
	}
	var back ComplianceCheck
	_ = bson.Unmarshal(raw, &back)
	if !back.SimplifiedDebitNote {
		t.Error("round trip lost SimplifiedDebitNote")
	}
}
