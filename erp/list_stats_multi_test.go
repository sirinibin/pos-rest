package erp

import "testing"

// A list filter's search-and-select can pick several customers, vendors or employees:
// f.party=a,b and f.<x>Id=a,b match any of them; other fields stay plain equality.
func TestStatsFilterIDLists(t *testing.T) {
	r := statRow{Party: "c2", Vals: map[string]interface{}{"employeeId": "e1", "category": "a,b", "status": "paid"}}
	for _, tc := range []struct {
		key, val string
		want     bool
	}{
		{"party", "c2", true},
		{"party", "c1,c2", true},
		{"party", "c1, c2", true},
		{"party", "c1,c3", false},
		{"party", "", false},
		{"employeeId", "e1", true},
		{"employeeId", "e9,e1", true},
		{"employeeId", "e9", false},
		// not an id field: a comma is part of the value
		{"category", "a,b", true},
		{"category", "a", false},
		{"status", "paid", true},
	} {
		if got := statsFilter(tc.key, tc.val, r, nil, "2026-10-08"); got != tc.want {
			t.Errorf("%s=%q: %v, want %v", tc.key, tc.val, got, tc.want)
		}
	}
	if statsFilter("party", "c1", statRow{}, nil, "") {
		t.Error("a record without a party matches no party")
	}
}
