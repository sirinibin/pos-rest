package erp

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestNextSerial_DistinctUnderConcurrencyAndFollowsExisting(t *testing.T) {
	requireDB(t)
	d := db.GetDB("")
	key := "test|" + primitive.NewObjectID().Hex()
	t.Cleanup(func() { _, _ = d.Collection(collSerialCounters).DeleteOne(context.Background(), bson.M{"_id": key}) })
	const n = 25
	got := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err := nextSerial(d, key, 7) // 7 = highest code already stored
			if err != nil {
				t.Error(err)
			}
			got[i] = v
		}(i)
	}
	wg.Wait()
	seen := map[int]bool{}
	for _, v := range got {
		if v < 8 || v > 7+n || seen[v] {
			t.Fatalf("numbers %v: want %d distinct numbers from 8", got, n)
		}
		seen[v] = true
	}
	if v, _ := nextSerial(d, key, 100); v != 101 {
		t.Fatalf("after a stored code 100: got %d, want 101", v)
	}
}

func TestClaimRecord_OneWinnerThenConflictOrNotFound(t *testing.T) {
	requireDB(t)
	col := db.GetDB("").Collection("test_claims_" + primitive.NewObjectID().Hex())
	t.Cleanup(func() { _ = col.Drop(context.Background()) })
	ctx := context.Background()
	if _, err := col.InsertOne(ctx, bson.M{"_id": "rec1"}); err != nil {
		t.Fatal(err)
	}

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = claimOrAnswer(col, "rec1")
		}(i)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		} else if ae, ok := err.(*APIError); !ok || ae.Status != http.StatusConflict {
			t.Fatalf("loser got %v, want 409", err)
		}
	}
	if wins != 1 {
		t.Fatalf("%d requests claimed the record, want 1", wins)
	}

	// the winner's work failed: the record can be claimed again
	releaseClaim(col, "rec1")
	if err := claimOrAnswer(col, "rec1"); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	// a claim left by a crash stops blocking after claimTTL
	_, _ = col.UpdateOne(ctx, bson.M{"_id": "rec1"}, bson.M{"$set": bson.M{claimField: time.Now().Add(-claimTTL - time.Minute)}})
	if err := claimOrAnswer(col, "rec1"); err != nil {
		t.Fatalf("claim over a stale claim: %v", err)
	}
	// the winner finished and removed the record: late requests get 404
	_, _ = col.DeleteOne(ctx, bson.M{"_id": "rec1"})
	if ae, ok := claimOrAnswer(col, "rec1").(*APIError); !ok || ae.Status != http.StatusNotFound {
		t.Fatalf("claim of a removed record: %v, want 404", ae)
	}
}
