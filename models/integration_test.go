//go:build integration

package models

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestMain initialises MongoDB and Redis connections before running integration
// tests and tears them down afterwards.  The dedicated test database
// (startpos_integration_test) keeps test data isolated from production.
//
// Required environment variables (with defaults):
//
//	MONGO_DB      — MongoDB database name  (default: startpos_integration_test)
//	MONGO_HOST    — MongoDB host            (default: localhost)
//	MONGO_PORT    — MongoDB port            (default: 27017)
//	REDIS_DSN     — Redis address           (default: localhost:6379)
func TestMain(m *testing.M) {
	// Point every model's db.GetDB() call to the isolated test database.
	if os.Getenv("MONGO_DB") == "" {
		os.Setenv("MONGO_DB", "startpos_integration_test")
	}

	// Initialise the MongoDB connection.  db.Client("") triggers the sync.Once
	// and registers a connection keyed by the test DB name.
	_ = db.Client(db.GetPosDB())

	// Initialise Redis.  Panics after 10 retries if Redis is not available.
	db.InitRedis()

	// Provide a deterministic JWT secret so any token-related helpers work.
	if os.Getenv("ACCESS_SECRET") == "" {
		os.Setenv("ACCESS_SECRET", "integration-test-secret")
	}

	os.Exit(m.Run())
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// makeTestStore builds a minimal Store and inserts it.  The caller is
// responsible for cleanup (use t.Cleanup).
func makeTestStore(t *testing.T) *Store {
	t.Helper()
	store := &Store{
		Name:  "Integration Test Store",
		Code:  "INT-TEST-" + primitive.NewObjectID().Hex()[:6],
		Email: "integration-test@example.com",
	}
	if err := store.Insert(); err != nil {
		t.Fatalf("makeTestStore: Insert failed: %v", err)
	}
	return store
}

// hardDeleteCustomer removes a customer document directly from MongoDB,
// bypassing the soft-delete path.
func hardDeleteCustomer(t *testing.T, storeID, customerID primitive.ObjectID) {
	t.Helper()
	collection := db.GetDB("store_" + storeID.Hex()).Collection("customer")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := collection.DeleteOne(ctx, bson.M{"_id": customerID}); err != nil {
		t.Logf("hardDeleteCustomer: warning: %v", err)
	}
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestCustomer_CreateAndFind inserts a Customer, fetches it by ID using
// Store.FindCustomerByID, verifies the round-tripped fields, and cleans up.
func TestCustomer_CreateAndFind(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() {
		if err := store.PermanentlyDelete(); err != nil {
			t.Logf("cleanup: store.PermanentlyDelete: %v", err)
		}
	})

	now := time.Now().Truncate(time.Second)
	customer := &Customer{
		Name:    "Integration Test Customer",
		Email:   "inttest@example.com",
		Phone:   "0500000001",
		StoreID: &store.ID,
		Stores:  map[string]CustomerStore{},
		CreatedAt: &now,
	}

	if err := customer.Insert(); err != nil {
		t.Fatalf("Customer.Insert: %v", err)
	}
	t.Cleanup(func() {
		hardDeleteCustomer(t, store.ID, customer.ID)
	})

	if customer.ID.IsZero() {
		t.Fatal("Insert did not set customer.ID")
	}

	found, err := store.FindCustomerByID(&customer.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindCustomerByID: %v", err)
	}

	if found.Name != customer.Name {
		t.Errorf("Name: got %q, want %q", found.Name, customer.Name)
	}
	if found.Email != customer.Email {
		t.Errorf("Email: got %q, want %q", found.Email, customer.Email)
	}
	if found.Phone != customer.Phone {
		t.Errorf("Phone: got %q, want %q", found.Phone, customer.Phone)
	}
	if found.StoreID == nil || *found.StoreID != store.ID {
		t.Errorf("StoreID: got %v, want %v", found.StoreID, store.ID)
	}
}

// TestOrder_CreateAndFindNetTotal creates a store, inserts an order with two
// products, verifies FindNetTotal calculates correctly, persists the order,
// fetches it back, and checks the stored NetTotal.
func TestOrder_CreateAndFindNetTotal(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() {
		if err := store.PermanentlyDelete(); err != nil {
			t.Logf("cleanup: store.PermanentlyDelete: %v", err)
		}
	})

	// Build an order with two line-items.
	// Product 1: qty=2, unit price=50.00, no discount  → line total = 100.00
	// Product 2: qty=3, unit price=20.00, unit disc=5  → line total = 3*(20-5) = 45.00
	// Combined total = 145.00, VAT 15% = 21.75, NetTotal = 166.75
	vatPct := 15.0
	order := &Order{
		StoreID:    &store.ID,
		VatPercent: &vatPct,
		Products: []OrderProduct{
			{Name: "Widget A", Quantity: 2, UnitPrice: 50.00, UnitDiscount: 0},
			{Name: "Widget B", Quantity: 3, UnitPrice: 20.00, UnitDiscount: 5.00},
		},
	}

	order.FindNetTotal()

	wantTotal := RoundTo2Decimals(2*50.00 + 3*(20.00-5.00))         // 145.00
	wantVAT := RoundTo2Decimals(wantTotal * 0.15)                   // 21.75
	wantNetTotal := RoundTo2Decimals(wantTotal + wantVAT)            // 166.75

	if order.Total != wantTotal {
		t.Errorf("Total: got %v, want %v", order.Total, wantTotal)
	}
	if order.VatPrice != wantVAT {
		t.Errorf("VatPrice: got %v, want %v", order.VatPrice, wantVAT)
	}
	if order.NetTotal != wantNetTotal {
		t.Errorf("NetTotal: got %v, want %v", order.NetTotal, wantNetTotal)
	}

	// Persist the order and verify the stored value matches.
	if err := order.Insert(); err != nil {
		t.Fatalf("Order.Insert: %v", err)
	}
	t.Cleanup(func() {
		if err := order.HardDelete(); err != nil {
			t.Logf("cleanup: order.HardDelete: %v", err)
		}
	})

	if order.ID.IsZero() {
		t.Fatal("Insert did not set order.ID")
	}

	found, err := store.FindOrderByID(&order.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindOrderByID: %v", err)
	}
	if found.NetTotal != wantNetTotal {
		t.Errorf("persisted NetTotal: got %v, want %v", found.NetTotal, wantNetTotal)
	}
}

// TestProduct_CreateUpdateDelete creates a product, updates its name,
// verifies the update persisted, and then hard-deletes it.
func TestProduct_CreateUpdateDelete(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() {
		if err := store.PermanentlyDelete(); err != nil {
			t.Logf("cleanup: store.PermanentlyDelete: %v", err)
		}
	})

	product := &Product{
		Name:    "Integration Product v1",
		StoreID: &store.ID,
	}

	if err := product.Insert(); err != nil {
		t.Fatalf("Product.Insert: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort cleanup in case the test fails before the explicit delete.
		if err := product.HardDelete(); err != nil {
			t.Logf("cleanup: product.HardDelete: %v", err)
		}
	})

	if product.ID.IsZero() {
		t.Fatal("Insert did not set product.ID")
	}

	// Update the name.
	product.Name = "Integration Product v2"
	if err := product.Update(nil); err != nil {
		t.Fatalf("Product.Update: %v", err)
	}

	// Fetch and verify.
	found, err := store.FindProductByID(&product.ID, bson.M{})
	if err != nil {
		t.Fatalf("FindProductByID after update: %v", err)
	}
	if found.Name != "Integration Product v2" {
		t.Errorf("updated Name: got %q, want %q", found.Name, "Integration Product v2")
	}

	// Explicit hard delete (the t.Cleanup above is a safety net).
	if err := product.HardDelete(); err != nil {
		t.Fatalf("Product.HardDelete: %v", err)
	}

	// Confirm deletion.
	_, err = store.FindProductByID(&product.ID, bson.M{})
	if err == nil {
		t.Error("FindProductByID after delete: expected error (document gone), got nil")
	}
}
