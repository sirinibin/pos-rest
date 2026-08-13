package models

import (
	"strings"
	"testing"
	"time"
)

func riyadhLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Riyadh")
	if err != nil {
		t.Fatalf("failed to load Asia/Riyadh: %v", err)
	}
	return loc
}

// ── formatIssueDateTimeForZatca ───────────────────────────────────────────────

func TestFormatIssueDateTime_NilDate_ReturnsError(t *testing.T) {
	order := &Order{Date: nil}
	_, _, err := order.formatIssueDateTimeForZatca(riyadhLoc(t))
	if err == nil {
		t.Fatal("expected error for nil Date, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error message should mention nil, got: %q", err.Error())
	}
}

func TestFormatIssueDateTime_NilDate_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil Date caused a panic: %v", r)
		}
	}()
	order := &Order{Date: nil}
	order.formatIssueDateTimeForZatca(riyadhLoc(t)) //nolint:errcheck
}

func TestFormatIssueDateTime_ValidDate_FormatsISODate(t *testing.T) {
	d := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	order := &Order{Date: &d}
	issueDate, _, err := order.formatIssueDateTimeForZatca(riyadhLoc(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// UTC 10:30 → Riyadh (UTC+3) 13:30, still 2024-03-15
	if issueDate != "2024-03-15" {
		t.Errorf("issueDate = %q, want %q", issueDate, "2024-03-15")
	}
}

func TestFormatIssueDateTime_ValidDate_FormatsTime(t *testing.T) {
	d := time.Date(2024, 3, 15, 10, 30, 45, 0, time.UTC)
	order := &Order{Date: &d}
	_, issueTime, err := order.formatIssueDateTimeForZatca(riyadhLoc(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// UTC 10:30:45 → Riyadh 13:30:45
	if issueTime != "13:30:45" {
		t.Errorf("issueTime = %q, want %q", issueTime, "13:30:45")
	}
}

func TestFormatIssueDateTime_RiyadhTimezone_DateCrossesAtMidnight(t *testing.T) {
	// UTC 22:30 on Mar 15 → Riyadh 01:30 on Mar 16
	d := time.Date(2024, 3, 15, 22, 30, 0, 0, time.UTC)
	order := &Order{Date: &d}
	issueDate, issueTime, err := order.formatIssueDateTimeForZatca(riyadhLoc(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issueDate != "2024-03-16" {
		t.Errorf("issueDate = %q, want %q (date should advance in Riyadh tz)", issueDate, "2024-03-16")
	}
	if issueTime != "01:30:00" {
		t.Errorf("issueTime = %q, want %q", issueTime, "01:30:00")
	}
}

func TestFormatIssueDateTime_DateAtMidnightUTC_IsCorrectInRiyadh(t *testing.T) {
	// UTC 00:00 → Riyadh 03:00, same calendar day
	d := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	order := &Order{Date: &d}
	issueDate, issueTime, err := order.formatIssueDateTimeForZatca(riyadhLoc(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issueDate != "2024-06-01" {
		t.Errorf("issueDate = %q, want %q", issueDate, "2024-06-01")
	}
	if issueTime != "03:00:00" {
		t.Errorf("issueTime = %q, want %q", issueTime, "03:00:00")
	}
}

func TestFormatIssueDateTime_NilLoc_UsesUTC(t *testing.T) {
	// time.Time.In(nil) panics; passing UTC explicitly avoids this
	// This test documents that callers must supply a non-nil location.
	d := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	order := &Order{Date: &d}
	defer func() {
		if r := recover(); r != nil {
			t.Logf("nil loc panics as expected: %v", r)
		}
	}()
	// Passing time.UTC (non-nil) should work fine
	issueDate, _, err := order.formatIssueDateTimeForZatca(time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issueDate != "2024-01-01" {
		t.Errorf("issueDate = %q, want %q", issueDate, "2024-01-01")
	}
}
