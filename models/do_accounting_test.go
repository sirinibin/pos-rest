//go:build integration

package models

import (
	"context"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// makeMinimalOrder inserts a minimal order (no payments) into the test store
// and registers a t.Cleanup to hard-delete it afterwards.
func makeMinimalOrder(t *testing.T, store *Store) *Order {
	t.Helper()
	now := time.Now()
	vatPct := 15.0
	order := &Order{
		StoreID:    &store.ID,
		Date:       &now,
		VatPercent: &vatPct,
		Products: []OrderProduct{
			{Name: "Test Product", Quantity: 1, UnitPrice: 80.00, UnitDiscount: 0},
		},
	}
	order.FindTotal()
	order.FindNetTotal()
	if err := order.Insert(); err != nil {
		t.Fatalf("makeMinimalOrder: Insert: %v", err)
	}
	t.Cleanup(func() {
		col := db.GetDB("store_" + store.ID.Hex()).Collection("order")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		col.DeleteOne(ctx, bson.M{"_id": order.ID})
		// Also remove any ledger/posting created during the test (best-effort).
		store.RemoveLedgerByReferenceID(order.ID)   //nolint:errcheck
		store.RemovePostingsByReferenceID(order.ID) //nolint:errcheck
	})
	return order
}

// insertOrderLedger inserts a bare-minimum ledger document referencing the
// given order so tests can verify that UndoAccounting / DoAccounting removes
// stale entries.
func insertOrderLedger(t *testing.T, storeID, orderID primitive.ObjectID) {
	t.Helper()
	now := time.Now()
	ledger := &Ledger{
		StoreID:        &storeID,
		ReferenceID:    orderID,
		ReferenceModel: "sales",
		ReferenceCode:  "TEST-LEDGER",
		CreatedAt:      &now,
		UpdatedAt:      &now,
	}
	if err := ledger.Insert(); err != nil {
		t.Fatalf("insertOrderLedger: %v", err)
	}
}

// insertOrderPosting inserts a single posting document referencing the given
// order. Used to simulate stale / duplicate postings left by the old bug.
func insertOrderPosting(t *testing.T, storeID, orderID primitive.ObjectID, debit, credit float64) {
	t.Helper()
	now := time.Now()
	accountID := primitive.NewObjectID() // arbitrary account
	p := &Posting{
		StoreID:        &storeID,
		AccountID:      accountID,
		ReferenceID:    orderID,
		ReferenceModel: "sales",
		DebitTotal:     debit,
		CreditTotal:    credit,
		Date:           &now,
	}
	if err := p.Insert(); err != nil {
		t.Fatalf("insertOrderPosting: %v", err)
	}
}

// countLedgers returns the number of ledger documents referencing the order.
func countLedgers(t *testing.T, storeID, orderID primitive.ObjectID) int64 {
	t.Helper()
	col := db.GetDB("store_" + storeID.Hex()).Collection("ledger")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := col.CountDocuments(ctx, bson.M{"reference_id": orderID})
	if err != nil {
		t.Fatalf("countLedgers: %v", err)
	}
	return n
}

// countOrderPostings returns the number of posting documents referencing the order.
func countOrderPostings(t *testing.T, storeID, orderID primitive.ObjectID) int64 {
	t.Helper()
	col := db.GetDB("store_" + storeID.Hex()).Collection("posting")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := col.CountDocuments(ctx, bson.M{"reference_id": orderID})
	if err != nil {
		t.Fatalf("countOrderPostings: %v", err)
	}
	return n
}

// ─── UndoAccounting ───────────────────────────────────────────────────────────

// TestUndoAccounting_SafeOnOrderWithNoLedger verifies that UndoAccounting
// returns nil when there is no existing ledger/posting for the order — it must
// be a safe no-op for new orders.
func TestUndoAccounting_SafeOnOrderWithNoLedger(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	order := makeMinimalOrder(t, store)

	if err := order.UndoAccounting(); err != nil {
		t.Errorf("UndoAccounting on order with no ledger: want nil, got %v", err)
	}

	// Nothing should have been created by this no-op call.
	if n := countLedgers(t, store.ID, order.ID); n != 0 {
		t.Errorf("ledger count after UndoAccounting no-op: want 0, got %d", n)
	}
	if n := countOrderPostings(t, store.ID, order.ID); n != 0 {
		t.Errorf("posting count after UndoAccounting no-op: want 0, got %d", n)
	}
}

// TestUndoAccounting_RemovesLedgerAndPostings verifies that UndoAccounting
// deletes all ledger and posting documents linked to the order.
func TestUndoAccounting_RemovesLedgerAndPostings(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	order := makeMinimalOrder(t, store)

	// Pre-populate one ledger + two postings to simulate a prior DoAccounting run.
	insertOrderLedger(t, store.ID, order.ID)
	insertOrderPosting(t, store.ID, order.ID, 80, 0)
	insertOrderPosting(t, store.ID, order.ID, 0, 80)

	if n := countLedgers(t, store.ID, order.ID); n != 1 {
		t.Fatalf("pre-condition: want 1 ledger, got %d", n)
	}
	if n := countOrderPostings(t, store.ID, order.ID); n != 2 {
		t.Fatalf("pre-condition: want 2 postings, got %d", n)
	}

	if err := order.UndoAccounting(); err != nil {
		t.Fatalf("UndoAccounting: %v", err)
	}

	if n := countLedgers(t, store.ID, order.ID); n != 0 {
		t.Errorf("ledger count after UndoAccounting: want 0, got %d", n)
	}
	if n := countOrderPostings(t, store.ID, order.ID); n != 0 {
		t.Errorf("posting count after UndoAccounting: want 0, got %d", n)
	}
}

// ─── DoAccounting ─────────────────────────────────────────────────────────────

// TestDoAccounting_CreatesLedgerAndPostings verifies that a single DoAccounting
// call on a new order creates exactly one ledger document and at least one
// posting document.
func TestDoAccounting_CreatesLedgerAndPostings(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	order := makeMinimalOrder(t, store)

	if err := order.DoAccounting(); err != nil {
		t.Fatalf("DoAccounting: %v", err)
	}

	if n := countLedgers(t, store.ID, order.ID); n != 1 {
		t.Errorf("ledger count after DoAccounting: want 1, got %d", n)
	}
	if n := countOrderPostings(t, store.ID, order.ID); n == 0 {
		t.Error("posting count after DoAccounting: want ≥1, got 0")
	}
}

// TestDoAccounting_Idempotent_NoDuplicateEntries is the core regression test
// for the duplicate-posting bug (S-INV-20260625-1045 style).
// Calling DoAccounting twice must yield exactly the same ledger+posting count
// as calling it once — never more.
func TestDoAccounting_Idempotent_NoDuplicateEntries(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	order := makeMinimalOrder(t, store)

	if err := order.DoAccounting(); err != nil {
		t.Fatalf("first DoAccounting: %v", err)
	}

	afterFirst := countOrderPostings(t, store.ID, order.ID)
	ledgerAfterFirst := countLedgers(t, store.ID, order.ID)

	if err := order.DoAccounting(); err != nil {
		t.Fatalf("second DoAccounting: %v", err)
	}

	afterSecond := countOrderPostings(t, store.ID, order.ID)
	ledgerAfterSecond := countLedgers(t, store.ID, order.ID)

	if afterSecond != afterFirst {
		t.Errorf("posting count: after 1st=%d, after 2nd=%d — DoAccounting is not idempotent (duplicate postings created)", afterFirst, afterSecond)
	}
	if ledgerAfterSecond != ledgerAfterFirst {
		t.Errorf("ledger count: after 1st=%d, after 2nd=%d — DoAccounting created a duplicate ledger", ledgerAfterFirst, ledgerAfterSecond)
	}
}

// TestDoAccounting_ClearsStalePostingsBeforeRecreating verifies that if a
// stale duplicate posting already exists for the order (exactly the production
// bug), DoAccounting removes it and recreates only the correct set — leaving
// the same count as a clean first call.
func TestDoAccounting_ClearsStalePostingsBeforeRecreating(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() { store.PermanentlyDelete() })

	order := makeMinimalOrder(t, store)

	if err := order.DoAccounting(); err != nil {
		t.Fatalf("DoAccounting: %v", err)
	}
	baseline := countOrderPostings(t, store.ID, order.ID)

	// Inject a stale duplicate posting — simulates the bug.
	insertOrderPosting(t, store.ID, order.ID, 80, 0)

	if n := countOrderPostings(t, store.ID, order.ID); n != baseline+1 {
		t.Fatalf("pre-condition after injecting stale posting: want %d, got %d", baseline+1, n)
	}

	// A second DoAccounting call must clean up the stale entry.
	if err := order.DoAccounting(); err != nil {
		t.Fatalf("second DoAccounting: %v", err)
	}

	if n := countOrderPostings(t, store.ID, order.ID); n != baseline {
		t.Errorf("posting count after second DoAccounting: want %d (baseline), got %d — stale posting was not removed", baseline, n)
	}
}

// TestDoAccounting_ErrorPropagated_WhenStoreNotFound verifies that DoAccounting
// returns a non-nil error when the order's StoreID references a store that
// does not exist, rather than silently succeeding or panicking.
func TestDoAccounting_ErrorPropagated_WhenStoreNotFound(t *testing.T) {
	nonExistent := primitive.NewObjectID()
	order := &Order{
		StoreID: &nonExistent,
		ID:      primitive.NewObjectID(),
	}
	if err := order.DoAccounting(); err == nil {
		t.Error("DoAccounting with non-existent store: want error, got nil")
	}
}
