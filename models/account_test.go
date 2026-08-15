//go:build integration

package models

import (
	"context"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func insertTestAccount(t *testing.T, storeID primitive.ObjectID, refID *primitive.ObjectID, name string, open bool, balance float64) *Account {
	t.Helper()
	refModel := "customer"
	now := time.Now()
	acc := &Account{
		StoreID:        &storeID,
		ReferenceID:    refID,
		ReferenceModel: &refModel,
		Name:           name,
		Number:         primitive.NewObjectID().Hex()[:6],
		Balance:        balance,
		Open:           open,
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}
	if err := acc.Insert(); err != nil {
		t.Fatalf("insertTestAccount: %v", err)
	}
	t.Cleanup(func() {
		col := db.GetDB("store_" + storeID.Hex()).Collection("account")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		col.DeleteOne(ctx, bson.M{"_id": acc.ID})
	})
	return acc
}

func insertTestPosting(t *testing.T, storeID, accountID primitive.ObjectID, debitTotal, creditTotal float64, date time.Time) *Posting {
	t.Helper()
	refModel := "test"
	refID := primitive.NewObjectID()
	p := &Posting{
		StoreID:        &storeID,
		AccountID:      accountID,
		ReferenceID:    refID,
		ReferenceModel: refModel,
		DebitTotal:     debitTotal,
		CreditTotal:    creditTotal,
		Date:           &date,
	}
	if err := p.Insert(); err != nil {
		t.Fatalf("insertTestPosting: %v", err)
	}
	t.Cleanup(func() {
		col := db.GetDB("store_" + storeID.Hex()).Collection("posting")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		col.DeleteOne(ctx, bson.M{"_id": p.ID})
	})
	return p
}

func insertTestCustomer(t *testing.T, storeID primitive.ObjectID, name string, creditBalance float64) *Customer {
	t.Helper()
	now := time.Now()
	c := &Customer{
		Name:          name,
		StoreID:       &storeID,
		CreditBalance: creditBalance,
		Stores:        map[string]CustomerStore{},
		CreatedAt:     &now,
	}
	if err := c.Insert(); err != nil {
		t.Fatalf("insertTestCustomer: %v", err)
	}
	t.Cleanup(func() {
		col := db.GetDB("store_" + storeID.Hex()).Collection("customer")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		col.DeleteOne(ctx, bson.M{"_id": c.ID})
	})
	return c
}

// ─── FindAccountByReferenceID ─────────────────────────────────────────────────

// TestFindAccountByReferenceID_SingleAccount_Open verifies that with one open
// account the function returns it regardless of the open flag logic.
func TestFindAccountByReferenceID_SingleAccount_Open(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	refID := primitive.NewObjectID()
	acc := insertTestAccount(t, store.ID, &refID, "CUSTOMER A", true, 940)

	found, err := store.FindAccountByReferenceID(refID, store.ID, bson.M{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found == nil {
		t.Fatal("expected account, got nil")
	}
	if found.ID != acc.ID {
		t.Errorf("wrong account returned: got %v, want %v", found.ID, acc.ID)
	}
}

// TestFindAccountByReferenceID_SingleAccount_ClosedBalance verifies that a
// single account with open=false (zero balance) is still returned so that
// SetCreditBalance can update the customer record.
func TestFindAccountByReferenceID_SingleAccount_ClosedBalance(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	refID := primitive.NewObjectID()
	acc := insertTestAccount(t, store.ID, &refID, "CUSTOMER B", false, 0)

	found, err := store.FindAccountByReferenceID(refID, store.ID, bson.M{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found == nil {
		t.Fatal("expected account (single, open=false), got nil")
	}
	if found.ID != acc.ID {
		t.Errorf("wrong account: got %v, want %v", found.ID, acc.ID)
	}
}

// TestFindAccountByReferenceID_MultipleAccounts_OneOpen verifies that with two
// accounts the open one is preferred.
func TestFindAccountByReferenceID_MultipleAccounts_OneOpen(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	refID := primitive.NewObjectID()
	insertTestAccount(t, store.ID, &refID, "CUSTOMER C OLD", false, 0)   // closed
	openAcc := insertTestAccount(t, store.ID, &refID, "CUSTOMER C", true, 940) // open

	found, err := store.FindAccountByReferenceID(refID, store.ID, bson.M{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found == nil {
		t.Fatal("expected account, got nil")
	}
	if found.ID != openAcc.ID {
		t.Errorf("expected open account %v, got %v", openAcc.ID, found.ID)
	}
}

// TestFindAccountByReferenceID_MultipleAccounts_AllClosed is the core bug
// regression: when totalCount > 1 and ALL accounts are closed (open=false),
// the function must still return one account via the fallback path instead of
// returning nil — which was causing SetCreditBalance to skip the update.
func TestFindAccountByReferenceID_MultipleAccounts_AllClosed(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	refID := primitive.NewObjectID()
	insertTestAccount(t, store.ID, &refID, "CUSTOMER D OLD", false, 0)
	insertTestAccount(t, store.ID, &refID, "CUSTOMER D", false, 0)

	found, err := store.FindAccountByReferenceID(refID, store.ID, bson.M{})
	// Before the fix this returned (nil, mongo.ErrNoDocuments), leaving the
	// customer credit_balance stale at 940 even though postings netted to 0.
	if err != nil && err != mongo.ErrNoDocuments {
		t.Fatalf("unexpected error: %v", err)
	}
	if found == nil {
		t.Fatal("regression: FindAccountByReferenceID returned nil when all accounts are closed — SetCreditBalance would silently skip the customer update")
	}
}

// TestFindAccountByReferenceID_NoAccount verifies nil + ErrNoDocuments for an
// unknown reference ID.
func TestFindAccountByReferenceID_NoAccount(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	found, err := store.FindAccountByReferenceID(primitive.NewObjectID(), store.ID, bson.M{})
	if err == nil {
		t.Fatal("expected error for missing account, got nil")
	}
	if found != nil {
		t.Errorf("expected nil account, got %v", found.ID)
	}
}

// ─── SetCreditBalance ─────────────────────────────────────────────────────────

// TestSetCreditBalance_SingleAccount_ZeroBalance verifies that when a customer
// account's postings net to zero the customer's CreditBalance is updated to 0.
func TestSetCreditBalance_SingleAccount_ZeroBalance(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	customer := insertTestCustomer(t, store.ID, "MARKAS TEST", 940)

	// Create the account linked to the customer.
	acc := insertTestAccount(t, store.ID, &customer.ID, "MARKAS TEST", true, 0)

	// Insert two postings that cancel out: DR 940 (invoice) + CR 940 (payment).
	now := time.Now()
	insertTestPosting(t, store.ID, acc.ID, 940, 0, now)
	insertTestPosting(t, store.ID, acc.ID, 0, 940, now.Add(time.Minute))

	if err := customer.SetCreditBalance(); err != nil {
		t.Fatalf("SetCreditBalance: %v", err)
	}

	// Reload from DB to confirm the update was persisted.
	updated, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID after SetCreditBalance: %v", err)
	}
	if updated.CreditBalance != 0 {
		t.Errorf("CreditBalance: got %v, want 0 — customer record is stale", updated.CreditBalance)
	}
}

// TestSetCreditBalance_MultipleAccounts_AllClosedAfterPayment is the full
// regression scenario: customer has two accounts (old name + new name), both
// end up with open=false after the invoice is fully paid, and SetCreditBalance
// must still set CreditBalance to 0.
func TestSetCreditBalance_MultipleAccounts_AllClosedAfterPayment(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	customer := insertTestCustomer(t, store.ID, "MARKAS NEW NAME", 940)

	// Old account (open=false, balance=0 after UndoAccounting removed postings).
	oldAcc := insertTestAccount(t, store.ID, &customer.ID, "MARKAS OLD NAME", false, 0)
	// New account (also open=false after postings DR+CR = 0).
	newAcc := insertTestAccount(t, store.ID, &customer.ID, "MARKAS NEW NAME", false, 0)

	// The new account has postings that net to 0 (fully paid invoice).
	now := time.Now()
	insertTestPosting(t, store.ID, newAcc.ID, 940, 0, now)           // invoice DR
	insertTestPosting(t, store.ID, newAcc.ID, 0, 940, now.Add(time.Minute)) // payment CR

	// Old account postings were removed by UndoAccounting — none remain.
	_ = oldAcc // confirming it is closed with no postings

	if err := customer.SetCreditBalance(); err != nil {
		t.Fatalf("SetCreditBalance: %v", err)
	}

	updated, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID after SetCreditBalance: %v", err)
	}
	if updated.CreditBalance != 0 {
		t.Errorf("CreditBalance: got %v, want 0 — stale 940 not cleared (regression)", updated.CreditBalance)
	}
}

// TestSetCreditBalance_UnpaidInvoice verifies that an unpaid invoice keeps the
// credit balance as the invoice amount.
func TestSetCreditBalance_UnpaidInvoice(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	customer := insertTestCustomer(t, store.ID, "UNPAID CUSTOMER", 0)
	acc := insertTestAccount(t, store.ID, &customer.ID, "UNPAID CUSTOMER", true, 940)

	now := time.Now()
	insertTestPosting(t, store.ID, acc.ID, 940, 0, now) // DR 940 only, no payment

	if err := customer.SetCreditBalance(); err != nil {
		t.Fatalf("SetCreditBalance: %v", err)
	}

	updated, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID: %v", err)
	}
	if updated.CreditBalance != 940 {
		t.Errorf("CreditBalance: got %v, want 940", updated.CreditBalance)
	}
}

// TestSetCreditBalance_PartialPayment verifies that a partial payment reduces
// the credit balance by the payment amount.
func TestSetCreditBalance_PartialPayment(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	customer := insertTestCustomer(t, store.ID, "PARTIAL CUSTOMER", 0)
	acc := insertTestAccount(t, store.ID, &customer.ID, "PARTIAL CUSTOMER", true, 540)

	now := time.Now()
	insertTestPosting(t, store.ID, acc.ID, 940, 0, now)                      // invoice
	insertTestPosting(t, store.ID, acc.ID, 0, 400, now.Add(time.Minute))     // partial payment

	if err := customer.SetCreditBalance(); err != nil {
		t.Fatalf("SetCreditBalance: %v", err)
	}

	updated, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID: %v", err)
	}
	// 940 DR - 400 CR = 540 net debit → CreditBalance = 540
	if updated.CreditBalance != 540 {
		t.Errorf("CreditBalance: got %v, want 540", updated.CreditBalance)
	}
}

// TestSetCreditBalance_NoAccount verifies that SetCreditBalance is a no-op and
// returns nil when the customer has no linked account.
func TestSetCreditBalance_NoAccount(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	customer := insertTestCustomer(t, store.ID, "NO ACCOUNT CUSTOMER", 500)

	// No account inserted — SetCreditBalance should return nil without touching
	// the customer document.
	if err := customer.SetCreditBalance(); err != nil {
		t.Fatalf("SetCreditBalance with no account: %v", err)
	}

	// credit_balance must be unchanged (500) since there is nothing to compute.
	updated, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID: %v", err)
	}
	if updated.CreditBalance != 500 {
		t.Errorf("CreditBalance changed unexpectedly: got %v, want 500", updated.CreditBalance)
	}
}

// TestSetCreditBalance_MultipleAccounts_OneOpen verifies the happy path where
// multiple accounts exist but one is still open — the open account's balance
// should be used.
func TestSetCreditBalance_MultipleAccounts_OneOpen(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	customer := insertTestCustomer(t, store.ID, "MULTI OPEN CUSTOMER", 0)

	// Closed old account with no postings.
	insertTestAccount(t, store.ID, &customer.ID, "MULTI OLD", false, 0)
	// Open current account with one unpaid invoice.
	openAcc := insertTestAccount(t, store.ID, &customer.ID, "MULTI OPEN CUSTOMER", true, 750)

	now := time.Now()
	insertTestPosting(t, store.ID, openAcc.ID, 750, 0, now)

	if err := customer.SetCreditBalance(); err != nil {
		t.Fatalf("SetCreditBalance: %v", err)
	}

	updated, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID: %v", err)
	}
	if updated.CreditBalance != 750 {
		t.Errorf("CreditBalance: got %v, want 750", updated.CreditBalance)
	}
}
