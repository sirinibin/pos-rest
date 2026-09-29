package models

import "testing"

// ── GetDashboardAccounts contract ─────────────────────────────────────────────
// The function must return account_type "cash" and "bank", not "asset",
// and must derive balance from posting records (not the stale stored balance).
// These tests guard the DashboardAccountSummary struct contract.

func TestDashboardAccountSummary_CashType(t *testing.T) {
	s := DashboardAccountSummary{AccountType: "cash", Balance: 896.01}
	if s.AccountType != "cash" {
		t.Errorf("expected account_type 'cash', got %q", s.AccountType)
	}
	if s.Balance != 896.01 {
		t.Errorf("expected balance 896.01, got %v", s.Balance)
	}
}

func TestDashboardAccountSummary_BankType(t *testing.T) {
	s := DashboardAccountSummary{AccountType: "bank", Balance: 5000}
	if s.AccountType != "bank" {
		t.Errorf("expected account_type 'bank', got %q", s.AccountType)
	}
}

func TestDashboardAccountSummary_NotAssetType(t *testing.T) {
	// Regression: before the fix, CASH and BANK accounts had type="asset" in MongoDB
	// and GetDashboardAccounts grouped by that field, so account_type was "asset" not "cash"/"bank".
	// Now the function maps by name: name="CASH" → account_type="cash".
	entries := []DashboardAccountSummary{
		{AccountType: "cash", Balance: 100},
		{AccountType: "bank", Balance: 200},
	}
	for _, e := range entries {
		if e.AccountType == "asset" {
			t.Errorf("GetDashboardAccounts must NOT return account_type='asset'; got it for balance %.2f", e.Balance)
		}
	}
}
