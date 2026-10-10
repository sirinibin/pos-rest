package db

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// Handlers and dashboard fan-outs call GetDB from many goroutines at once;
// the connection cache must not race (run with -race) or crash with
// "concurrent map writes". mongo.Connect is lazy, so no server is needed.
func TestGetDB_ConcurrentCallers(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("test_concurrent_%d", i%4)
			if d := GetDB(name); d == nil || d.Name() != name {
				t.Errorf("GetDB(%q) = %v", name, d)
			}
			_ = Client(name)
		}(i)
	}
	wg.Add(1)
	go func() { defer wg.Done(); CloseConnections(time.Hour) }()
	wg.Wait()
	if a, b := GetDB("test_concurrent_1"), GetDB("test_concurrent_1"); a != b {
		t.Error("GetDB should reuse the cached database handle")
	}
}
