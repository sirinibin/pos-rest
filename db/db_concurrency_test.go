package db

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// Many requests for different stores open their databases at once. The
// connection maps used to be written without the lock, which the race
// detector flags and which can crash the process ("concurrent map writes").
// Run with -race. Connecting is lazy, so no MongoDB server is needed.
func TestGetDB_ConcurrentStoresAreSafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("store_concurrency_test_%d", i%8)
			if d := GetDB(name); d == nil || d.Name() != name {
				t.Errorf("GetDB(%q) = %v", name, d)
			}
		}(i)
	}
	wg.Wait()

	// Same database twice gives the same client.
	a, b := Client("store_concurrency_test_1"), Client("store_concurrency_test_1")
	if a != b {
		t.Fatalf("Client returned two different clients for one database")
	}

	// Cleanup running alongside lookups must not race either.
	wg.Add(2)
	go func() { defer wg.Done(); CloseConnections(time.Hour) }()
	go func() { defer wg.Done(); GetDB("store_concurrency_test_2") }()
	wg.Wait()
}
