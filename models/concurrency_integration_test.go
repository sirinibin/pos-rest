//go:build integration

package models

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCounterClaims_ReleaseOnlyLatest(t *testing.T) {
	key := "test_counter_claims_" + primitive.NewObjectID().Hex()
	t.Cleanup(func() { db.RedisClient.Del(key) })
	if err := db.RedisClient.SetNX(key, 10, 0).Err(); err != nil {
		t.Fatal(err)
	}
	// SETNX never resets a counter already in use
	if err := db.RedisClient.SetNX(key, 3, 0).Err(); err != nil {
		t.Fatal(err)
	}

	var a, b counterClaims
	na, _ := a.incr(key) // 11
	nb, _ := b.incr(key) // 12
	if na != 11 || nb != 12 {
		t.Fatalf("numbers %d, %d; want 11, 12", na, nb)
	}
	// a fails after b took a later number: 11 becomes a gap, 12 stays b's
	if err := a.release(); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.RedisClient.Get(key).Int64(); v != 12 {
		t.Fatalf("counter %d after releasing a number someone followed, want 12", v)
	}
	// b fails while it is still the latest: its number is given back once
	_ = b.release()
	_ = b.release()
	if v, _ := db.RedisClient.Get(key).Int64(); v != 11 {
		t.Fatalf("counter %d after releasing the latest number twice, want 11", v)
	}
}

func TestNextStoreSerial_UniqueAndContinuesFromExisting(t *testing.T) {
	storeID := primitive.NewObjectID()
	col := db.GetDB("store_" + storeID.Hex()).Collection(collStoreSerials)
	t.Cleanup(func() { _ = col.Drop(context.Background()) })

	const n = 30
	got := make([]int64, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			v, err := nextStoreSerial(ctx, storeID, "series", 41)
			if err != nil {
				t.Error(err)
			}
			got[i] = v
		}(i)
	}
	wg.Wait()
	seen := map[int64]bool{}
	for _, v := range got {
		if v < 42 || v > 41+n || seen[v] {
			t.Fatalf("numbers %v: want %d distinct numbers from 42", got, n)
		}
		seen[v] = true
	}
	// a higher number stored meanwhile (by hand or another program) is followed
	ctx := context.Background()
	if v, _ := nextStoreSerial(ctx, storeID, "series", 500); v != 501 {
		t.Fatalf("after highest 500: got %d, want 501", v)
	}
	if v, _ := nextStoreSerial(ctx, storeID, "series", 3); v != 502 {
		t.Fatalf("a lower highest never moves the counter back: got %d, want 502", v)
	}
}

func TestGenerateRepairJobNumber_NoReuseAfterDelete(t *testing.T) {
	store := &Store{ID: primitive.NewObjectID()}
	sdb := db.GetDB("store_" + store.ID.Hex())
	t.Cleanup(func() { _ = sdb.Drop(context.Background()) })
	ctx := context.Background()
	// existing data: RJ-1..RJ-3, RJ-2 deleted (count+1 would hand out RJ-3 again)
	for i, num := range []string{"RJ-1", "RJ-2", "RJ-3"} {
		_, err := sdb.Collection("repair_job").InsertOne(ctx, bson.M{"job_number": num, "deleted": i == 1})
		if err != nil {
			t.Fatal(err)
		}
	}
	const n = 10
	nums := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := store.GenerateRepairJobNumber()
			if err != nil {
				t.Error(err)
			}
			nums[i] = v
		}(i)
	}
	wg.Wait()
	seen := map[string]bool{"RJ-1": true, "RJ-2": true, "RJ-3": true}
	for _, v := range nums {
		if seen[v] {
			t.Fatalf("numbers %v reuse %s", nums, v)
		}
		seen[v] = true
	}
	if !seen["RJ-4"] || !seen["RJ-13"] {
		t.Fatalf("numbers %v: want RJ-4..RJ-13", nums)
	}
}

func TestStoreUpdateChanged_KeepsOtherSavesChanges(t *testing.T) {
	store := makeTestStore(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = db.Client("").Database(db.GetPosDB()).Collection("store").DeleteOne(ctx, bson.M{"_id": store.ID})
	})
	// two admins read the store at the same time
	a, err := FindStoreByID(&store.ID, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := FindStoreByID(&store.ID, bson.M{})
	beforeA, _ := StoreSnapshot(a)
	beforeB, _ := StoreSnapshot(b)
	a.Name = "Renamed by A"
	b.Email = "b@example.com"
	if err := a.UpdateChanged(beforeA); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateChanged(beforeB); err != nil {
		t.Fatal(err)
	}
	got, _ := FindStoreByID(&store.ID, bson.M{})
	if got.Name != "Renamed by A" || got.Email != "b@example.com" {
		t.Fatalf("name %q email %q: one admin's change was lost", got.Name, got.Email)
	}
}
