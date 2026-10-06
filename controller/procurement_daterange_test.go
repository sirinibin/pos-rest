package controller

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// date_from / date_to are store calendar days: local midnight in the store
// zone, not UTC midnight (the server runs in the UK).
func TestParseStoreDay_StoreZone(t *testing.T) {
	riyadh, _ := time.LoadLocation("Asia/Riyadh")
	london, _ := time.LoadLocation("Europe/London")
	tests := []struct {
		name string
		in   string
		loc  *time.Location
		want *time.Time
	}{
		{"SA midnight is 21:00Z prev day", "2026-10-07", riyadh, ptrTime(time.Date(2026, 10, 6, 21, 0, 0, 0, time.UTC))},
		{"GB BST midnight is 23:00Z prev day", "2026-10-07", london, ptrTime(time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC))},
		{"GB winter midnight is 00:00Z", "2026-12-01", london, ptrTime(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))},
		{"UTC fallback", "2026-10-07", time.UTC, ptrTime(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC))},
		{"empty", "", riyadh, nil},
		{"invalid", "07/10/2026", riyadh, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseStoreDay(tc.in, tc.loc)
			if (got == nil) != (tc.want == nil) || (got != nil && !got.Equal(*tc.want)) {
				t.Errorf("parseStoreDay(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestProcurementDateRange_NoDatesSkipsStoreLookup(t *testing.T) {
	from, to := procurementDateRange(primitive.NewObjectID(), "", "")
	if from != nil || to != nil {
		t.Errorf("want nil range, got %v %v", from, to)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
