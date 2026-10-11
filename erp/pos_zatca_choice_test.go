package erp

import (
	"testing"
)

// The Industrial supplies POS terminal lets the cashier choose, per sale, whether it
// is reported to ZATCA. The terminal reports a chosen sale through
// POST /sales/{id}/zatca/report after saving it; a sale saved with skipZatca:true is
// never reported by the save itself, keeps the flag (so the receipt prints the ZATCA
// Phase 1 QR code) and stays editable like any unreported sale. It can still be
// reported later from the Sales list.

func TestPosSale_SkipZatcaKeptAndNeverReportedOnSave(t *testing.T) {
	b := newSalesResource().Backend.(*legacyBackend)
	cases := []struct {
		name     string
		over     M
		wantSkip interface{}
	}{
		{"report chosen (no flag)", M{}, nil},
		{"not reported", M{"skipZatca": true}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, pid, wh := posSaleCtx()
			rec := posSaleRec(pid, wh, c.over)
			p, err := b.toL(x, rec, nil, allKeys(rec), true)
			if err != nil {
				t.Fatal(err)
			}
			// saving never reports: the terminal asks for the report separately
			if p["enable_report_to_zatca"] != false {
				t.Fatalf("enable_report_to_zatca %v", p["enable_report_to_zatca"])
			}
			ex := b.extras(rec)
			if ex["skipZatca"] != c.wantSkip {
				t.Fatalf("erp.x skipZatca %v, want %v", ex["skipZatca"], c.wantSkip)
			}
			if ex["posType"] != "parts" {
				t.Fatalf("posType not kept: %v", ex)
			}
		})
	}
}

func TestPosSale_SkipZatcaIsNotAMappedField(t *testing.T) {
	b := newSalesResource().Backend.(*legacyBackend)
	if b.known["skipZatca"] {
		t.Fatal("skipZatca must stay a client field kept in erp.x")
	}
	// an unreported sale (skipped or not) is never locked
	store := M{"zatca": M{"phase": "2"}}
	if zatcaLockOn(store, M{"zatca": M{"reporting_passed": false}, "erp": M{"x": M{"skipZatca": true}}}) {
		t.Fatal("a sale not reported to ZATCA must stay editable")
	}
}
