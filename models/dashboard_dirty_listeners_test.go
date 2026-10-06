package models

import (
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Every legacy write that marks the dashboard dirty also tells the listeners (the
// StartERP dashboard snapshots), whatever the monthly worker does with it.
func TestOnDashboardDirty(t *testing.T) {
	var mu sync.Mutex
	got := []string{}
	done := make(chan struct{}, 8)
	OnDashboardDirty(func(store string) {
		mu.Lock()
		got = append(got, store)
		mu.Unlock()
		done <- struct{}{}
	})
	id := primitive.NewObjectID()
	now := time.Now()
	MarkDashboardDirty(id, &now)
	MarkDashboardDirty(id, nil) // no date: the monthly worker skips it, the listeners still hear
	MarkDashboardDirtyMonth(id, "2026-10")
	MarkDashboardDirty(primitive.NilObjectID, &now) // no store: nobody is told
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("listener not called (%d so far)", i)
		}
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || got[0] != id.Hex() || got[1] != id.Hex() || got[2] != id.Hex() {
		t.Errorf("got %v", got)
	}
}
