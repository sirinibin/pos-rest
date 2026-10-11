package erp

import (
	"encoding/json"
	"sort"
	"strings"
)

// A sales invoice reported to ZATCA (Phase 2) is locked: only its cash
// discount and the payments received may change afterwards. Neither is part
// of the reported invoice (the cash discount lowers the balance, never the
// invoice total or VAT), so the reported XML stays true. The store setting
// settings.disable_sales_edit_once_reported_to_zatca (on unless set false,
// as in models.Order.Validate) switches the lock off.
var zatcaLockedEditable = map[string]bool{"cashDiscount": true, "payments": true}

// zatcaLockOn: the legacy document d is reported to ZATCA in a store whose
// invoices lock once reported.
func zatcaLockOn(store, d M) bool {
	if d == nil || !boolv(get(d, "zatca.reporting_passed")) {
		return false
	}
	if str(get(store, "zatca.phase")) != "2" {
		return false
	}
	if v := get(store, "settings.disable_sales_edit_once_reported_to_zatca"); v != nil && !boolv(v) {
		return false
	}
	return true
}

// zatcaLockedChanges: the mapped fields of a write that really change the
// record (a PATCH may resend a value unchanged) and are not editable once
// reported, sorted.
func zatcaLockedChanges(known map[string]bool, prev, next M, changed []string) []string {
	out := []string{}
	for _, k := range changed {
		if !known[k] || zatcaLockedEditable[k] {
			continue
		}
		a, _ := json.Marshal(prev[k])
		b, _ := json.Marshal(next[k])
		if string(a) == string(b) {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

const zatcaLockedMsg = "This invoice was reported to ZATCA: only the cash discount and the payments received can be changed."

// zatcaLockGuard refuses an update of a reported invoice that changes more
// than the cash discount and the payments.
func zatcaLockGuard(b *legacyBackend) func(x *mapCtx, prevDoc, prev, next M, changed []string) error {
	return func(x *mapCtx, prevDoc, prev, next M, changed []string) error {
		if !zatcaLockOn(x.store, prevDoc) {
			return nil
		}
		bad := zatcaLockedChanges(b.known, prev, next, changed)
		if len(bad) == 0 {
			return nil
		}
		fields := map[string]string{}
		for _, k := range bad {
			fields[k] = "locked: reported to ZATCA"
		}
		return errBadRequest(zatcaLockedMsg+" ("+strings.Join(bad, ", ")+")", fields)
	}
}
