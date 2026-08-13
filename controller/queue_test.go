package controller

import (
	"sync"
	"testing"
	"time"
)

// ── SafeQueue data-structure tests ───────────────────────────────────────────

func TestQueue_Enqueue_Pop_FIFO(t *testing.T) {
	q := NewSafeQueue()
	q.Enqueue(Request{Token: "a"})
	q.Enqueue(Request{Token: "b"})
	q.Enqueue(Request{Token: "c"})

	for _, want := range []string{"a", "b", "c"} {
		got, err := q.Pop()
		if err != nil {
			t.Fatalf("Pop() error: %v", err)
		}
		if got.Token != want {
			t.Errorf("Pop() = %q, want %q", got.Token, want)
		}
	}
}

func TestQueue_Pop_EmptyQueue_ReturnsError(t *testing.T) {
	q := NewSafeQueue()
	_, err := q.Pop()
	if err == nil {
		t.Fatal("expected error popping empty queue, got nil")
	}
}

func TestQueue_Peek_DoesNotRemoveItem(t *testing.T) {
	q := NewSafeQueue()
	q.Enqueue(Request{Token: "x"})
	got, err := q.Peek()
	if err != nil || got.Token != "x" {
		t.Fatalf("Peek() = %v, %v; want x, nil", got, err)
	}
	// Peek must not remove the item
	got2, err := q.Peek()
	if err != nil || got2.Token != "x" {
		t.Errorf("second Peek() = %v, %v; want x, nil", got2, err)
	}
}

func TestQueue_Peek_EmptyQueue_ReturnsError(t *testing.T) {
	q := NewSafeQueue()
	_, err := q.Peek()
	if err == nil {
		t.Fatal("expected error peeking empty queue")
	}
}

func TestQueue_FrontToken_ReturnsFirstToken(t *testing.T) {
	q := NewSafeQueue()
	q.Enqueue(Request{Token: "first"})
	q.Enqueue(Request{Token: "second"})
	tok, err := q.FrontToken()
	if err != nil {
		t.Fatalf("FrontToken() error: %v", err)
	}
	if tok != "first" {
		t.Errorf("FrontToken() = %q, want %q", tok, "first")
	}
}

func TestQueue_FrontToken_EmptyQueue_ReturnsError(t *testing.T) {
	q := NewSafeQueue()
	_, err := q.FrontToken()
	if err == nil {
		t.Fatal("expected error on empty queue")
	}
}

func TestQueue_CheckIfMyTurn_FrontIsTrue(t *testing.T) {
	q := NewSafeQueue()
	q.Enqueue(Request{Token: "leader"})
	q.Enqueue(Request{Token: "follower"})

	if !q.CheckIfMyTurn("leader") {
		t.Error("leader token should be its turn")
	}
	if q.CheckIfMyTurn("follower") {
		t.Error("follower token should not be its turn yet")
	}
}

func TestQueue_CheckIfMyTurn_EmptyQueue_ReturnsFalse(t *testing.T) {
	q := NewSafeQueue()
	if q.CheckIfMyTurn("anything") {
		t.Error("empty queue: CheckIfMyTurn should return false")
	}
}

func TestQueue_WaitUntilMyTurn_AlreadyAtFront(t *testing.T) {
	q := NewSafeQueue()
	q.Enqueue(Request{Token: "solo"})

	done := make(chan struct{})
	go func() {
		q.WaitUntilMyTurn("solo")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Error("WaitUntilMyTurn timed out when already at front")
	}
}

func TestQueue_WaitUntilMyTurn_UnblocksAfterPop(t *testing.T) {
	q := NewSafeQueue()
	q.Enqueue(Request{Token: "first"})
	q.Enqueue(Request{Token: "second"})

	unblocked := make(chan struct{})
	go func() {
		q.WaitUntilMyTurn("second")
		close(unblocked)
	}()

	// Let goroutine start spinning
	time.Sleep(20 * time.Millisecond)

	// Pop "first" so "second" becomes front
	q.Pop() //nolint:errcheck

	select {
	case <-unblocked:
	case <-time.After(500 * time.Millisecond):
		t.Error("WaitUntilMyTurn did not unblock after Pop()")
	}
}

// ── Core bug fix: defer Pop() unblocks queue on panic ─────────────────────────
//
// Before the fix, a panic inside the critical section left the queue's front
// entry un-popped, blocking every subsequent request forever.
// The fix wraps Pop()+Cleanup in a defer so recovery from any panic still
// advances the queue.

func simulateZatcaHandlerWithDefer(storeID string, shouldPanic bool) (panicCaught bool) {
	q := GetOrCreateQueue(storeID, "zatca")
	token := generateQueueToken()
	q.Enqueue(Request{Token: token})
	q.WaitUntilMyTurn(token)
	defer func() {
		q.Pop()             //nolint:errcheck
		CleanupQueueIfEmpty(storeID, "zatca")
		if r := recover(); r != nil {
			panicCaught = true
		}
	}()
	if shouldPanic {
		panic("simulated nil pointer dereference in MakeXMLContent")
	}
	return false
}

func TestQueue_DeferPop_UnblocksNextRequestAfterPanic(t *testing.T) {
	storeID := "store-panic-test-" + time.Now().String()

	// First request panics but defer pops the queue.
	panicCaught := simulateZatcaHandlerWithDefer(storeID, true)
	if !panicCaught {
		t.Fatal("expected panic to be caught")
	}

	// Second request must be able to get its turn within a short timeout.
	q := GetOrCreateQueue(storeID, "zatca")
	token := generateQueueToken()
	q.Enqueue(Request{Token: token})

	got := make(chan bool, 1)
	go func() {
		deadline := time.After(500 * time.Millisecond)
		for {
			select {
			case <-deadline:
				got <- false
				return
			default:
				if q.CheckIfMyTurn(token) {
					q.Pop() //nolint:errcheck
					CleanupQueueIfEmpty(storeID, "zatca")
					got <- true
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	if !<-got {
		t.Error("queue deadlock: second request timed out after first panicked — defer Pop() not working")
	}
}

func TestQueue_DeferPop_UnblocksNextRequestAfterError(t *testing.T) {
	storeID := "store-error-test-" + time.Now().String()

	// First request exits early (error path), defer pops the queue.
	simulateZatcaHandlerWithDefer(storeID, false)

	// Second request should proceed immediately.
	q := GetOrCreateQueue(storeID, "zatca")
	token := generateQueueToken()
	q.Enqueue(Request{Token: token})

	got := make(chan bool, 1)
	go func() {
		deadline := time.After(500 * time.Millisecond)
		for {
			select {
			case <-deadline:
				got <- false
				return
			default:
				if q.CheckIfMyTurn(token) {
					q.Pop() //nolint:errcheck
					CleanupQueueIfEmpty(storeID, "zatca")
					got <- true
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	if !<-got {
		t.Error("queue blocked after normal error return — defer Pop() not working")
	}
}

func TestQueue_WithoutDefer_DeadlocksOnPanic(t *testing.T) {
	// Documents the old (broken) behavior: without defer, a panic leaves the
	// queue stuck. This test verifies the bug exists so readers understand
	// WHY the fix was needed.
	storeID := "store-no-defer-" + time.Now().String()

	func() {
		defer func() { recover() }() // catch panic but do NOT pop — simulates pre-fix code
		q := GetOrCreateQueue(storeID, "zatca")
		token := generateQueueToken()
		q.Enqueue(Request{Token: token})
		q.WaitUntilMyTurn(token)
		// No defer Pop() here — this is the bug.
		panic("panic without defer pop")
	}()

	// The queue front is now the dead token. A second request enqueued
	// after will never reach the front.
	q := GetOrCreateQueue(storeID, "zatca")
	token2 := generateQueueToken()
	q.Enqueue(Request{Token: token2})

	// It should NOT be token2's turn — the dead token is still at front.
	if q.CheckIfMyTurn(token2) {
		t.Error("unexpectedly at front — dead token was already cleaned up somehow")
	}
	// Clean up so we don't leak the stuck queue.
	q.Pop() //nolint:errcheck
	q.Pop() //nolint:errcheck
	CleanupQueueIfEmpty(storeID, "zatca")
}

// ── GetOrCreateQueue / CleanupQueueIfEmpty ────────────────────────────────────

func TestGetOrCreateQueue_ReturnsSameQueueForSameStore(t *testing.T) {
	id := "store-same-queue"
	q1 := GetOrCreateQueue(id, "zatca")
	q2 := GetOrCreateQueue(id, "zatca")
	if q1 != q2 {
		t.Error("expected same queue pointer for same storeID")
	}
	// cleanup
	CleanupQueueIfEmpty(id, "zatca")
}

func TestGetOrCreateQueue_DifferentStoresHaveIndependentQueues(t *testing.T) {
	q1 := GetOrCreateQueue("store-A", "zatca")
	q2 := GetOrCreateQueue("store-B", "zatca")
	if q1 == q2 {
		t.Error("different storeIDs must not share the same queue")
	}
	CleanupQueueIfEmpty("store-A", "zatca")
	CleanupQueueIfEmpty("store-B", "zatca")
}

func TestCleanupQueueIfEmpty_RemovesEmptyQueue(t *testing.T) {
	id := "store-cleanup-empty"
	q := GetOrCreateQueue(id, "zatca")
	q.Enqueue(Request{Token: "t"})
	q.Pop() //nolint:errcheck
	CleanupQueueIfEmpty(id, "zatca")

	// After cleanup, a new call returns a fresh (different) queue object.
	q2 := GetOrCreateQueue(id, "zatca")
	if q == q2 {
		t.Error("expected a fresh queue after cleanup, got the same pointer")
	}
	CleanupQueueIfEmpty(id, "zatca")
}

func TestCleanupQueueIfEmpty_KeepsNonEmptyQueue(t *testing.T) {
	id := "store-cleanup-nonempty"
	q := GetOrCreateQueue(id, "zatca")
	q.Enqueue(Request{Token: "still-there"})
	CleanupQueueIfEmpty(id, "zatca") // should be a no-op

	q2 := GetOrCreateQueue(id, "zatca")
	if q != q2 {
		t.Error("non-empty queue must not be removed by cleanup")
	}
	q.Pop() //nolint:errcheck
	CleanupQueueIfEmpty(id, "zatca")
}

// ── Concurrency: multiple goroutines see correct turn ordering ─────────────────

func TestQueue_ConcurrentEnqueue_OrderPreserved(t *testing.T) {
	q := NewSafeQueue()
	const n = 20
	var mu sync.Mutex
	order := make([]string, 0, n)

	var wg sync.WaitGroup
	// Serial enqueue so token order is deterministic.
	tokens := make([]string, n)
	for i := range tokens {
		tokens[i] = generateQueueToken()
		q.Enqueue(Request{Token: tokens[i]})
	}

	// Each goroutine waits for its turn, records position, then pops.
	for _, tok := range tokens {
		wg.Add(1)
		go func(tok string) {
			defer wg.Done()
			q.WaitUntilMyTurn(tok)
			mu.Lock()
			order = append(order, tok)
			mu.Unlock()
			q.Pop() //nolint:errcheck
		}(tok)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent queue test timed out — possible deadlock")
	}

	if len(order) != n {
		t.Fatalf("expected %d completions, got %d", n, len(order))
	}
	// Verify FIFO: the i-th goroutine to finish must hold tokens[i].
	for i, tok := range tokens {
		if order[i] != tok {
			t.Errorf("position %d: got %q, want %q (FIFO violated)", i, order[i], tok)
		}
	}
}
