package controller

import "testing"

// queueRelease: released once on every path, a panic included (NEW-sr-queue-stuck).
func TestQueueRelease_PanicReleasesOnce(t *testing.T) {
	const store = "queue-release-test-store"
	q := GetOrCreateQueue(store, "sales_return")
	q.Enqueue(Request{Token: "first"})
	q.Enqueue(Request{Token: "second"})
	func() {
		defer func() { _ = recover() }()
		q.WaitUntilMyTurn("first")
		release := queueRelease(q, "first")
		defer release()
		release() // the explicit release on the happy path
		panic("handler panics after releasing")
	}()
	if !q.CheckIfMyTurn("second") {
		t.Fatalf("second request is not at the front: a double release popped it")
	}
	func() {
		defer func() { _ = recover() }()
		release := queueRelease(q, "second")
		defer release()
		panic("handler panics before releasing")
	}()
	if _, err := q.Peek(); err == nil {
		t.Fatalf("queue still holds the panicking request")
	}
}
