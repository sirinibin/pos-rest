package models

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestKeyedLocks_SerialisesOneKey(t *testing.T) {
	var k KeyedLocks
	var inside, maxInside int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer k.Lock("a")()
			n := atomic.AddInt32(&inside, 1)
			for {
				m := atomic.LoadInt32(&maxInside)
				if n <= m || atomic.CompareAndSwapInt32(&maxInside, m, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&inside, -1)
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Fatalf("%d holders at once, want 1", maxInside)
	}
	if len(k.m) != 0 {
		t.Fatalf("lock map keeps %d entries after every unlock, want 0", len(k.m))
	}
}

func TestKeyedLocks_DifferentKeysDoNotWait(t *testing.T) {
	var k KeyedLocks
	unlockA := k.Lock("a")
	defer unlockA()
	done := make(chan struct{})
	go func() {
		k.Lock("b")()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("key b waited for key a")
	}
}

func TestKeyedLocks_TimeoutAndIdempotentUnlock(t *testing.T) {
	var k KeyedLocks
	unlock := k.Lock("a")
	start := time.Now()
	late := k.LockWait("a", 50*time.Millisecond) // gives up waiting, never deadlocks
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("LockWait returned before its wait")
	}
	late()
	late() // second call is ignored
	if !k.held("a") {
		t.Fatal("the timed-out waiter's unlock released the holder's lock")
	}
	unlock()
	unlock()
	if k.held("a") || len(k.m) != 0 {
		t.Fatalf("lock still held or entry kept: held=%v entries=%d", k.held("a"), len(k.m))
	}
	k.Lock("a")() // free again
}

func TestStoreHold_ReleasedAfterRequestAndGoroutines(t *testing.T) {
	id := primitive.NewObjectID()
	h := HoldStore(id)
	gate := make(chan struct{})
	finished := make(chan struct{})
	h.Go(func() { <-gate; close(finished) })
	h.Release() // the request is done, its goroutine is not
	if !storeLocks.held(id.Hex()) {
		t.Fatal("store unlocked while a goroutine of the request still runs")
	}
	close(gate)
	<-finished
	deadline := time.Now().Add(2 * time.Second)
	for storeLocks.held(id.Hex()) {
		if time.Now().After(deadline) {
			t.Fatal("store still locked after the request and its goroutine ended")
		}
		time.Sleep(time.Millisecond)
	}
	h.Release() // extra release is ignored
	HoldStore(id).Release()
}

func TestStoreHold_NilIsValid(t *testing.T) {
	var h *StoreHold = HoldStoreOf(nil)
	h.Add()
	h.Release()
	ran := make(chan struct{})
	h.Go(func() { close(ran) })
	<-ran
}

func TestProductChanges_OnlyChangedFields(t *testing.T) {
	before := bson.M{"_id": 1, "name": "A", "product_stores": bson.M{
		"s1": bson.M{"stock": 5.0, "retail_unit_price": 10.0},
		"s2": bson.M{"stock": 1.0},
	}}
	after := bson.M{"_id": 1, "name": "A", "product_stores": bson.M{
		"s1": bson.M{"stock": 3.0, "retail_unit_price": 10.0},
		"s2": bson.M{"stock": 1.0},
	}}
	set := productChanges(before, after)
	if len(set) != 1 || set["product_stores.s1.stock"] != 3.0 {
		t.Fatalf("set = %v, want only product_stores.s1.stock", set)
	}
}

func TestChangedFields_NestedPaths(t *testing.T) {
	before := bson.M{"_id": 1, "name": "S", "settings": bson.M{"a": 1, "b": bson.M{"x": 1, "y": 2}}}
	after := bson.M{"_id": 2, "name": "S", "settings": bson.M{"a": 2, "b": bson.M{"x": 1, "y": 3}}, "new": "v"}
	set := bson.M{}
	changedFields("", before, after, 2, set)
	want := bson.M{"settings.a": 2, "settings.b.y": 3, "new": "v"}
	if len(set) != len(want) {
		t.Fatalf("set = %v, want %v", set, want)
	}
	for k, v := range want {
		if set[k] != v {
			t.Fatalf("set = %v, want %v", set, want)
		}
	}
}
