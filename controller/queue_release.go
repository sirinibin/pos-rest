package controller

import "sync"

// queueRelease returns a function that leaves the store's queue once: it pops
// this request (the front of the queue, as it waited for its turn) and drops
// the queue when it is empty.  Later calls are no-ops, so a handler both calls
// it where it used to pop and defers it: the queue is released on every path,
// a panic included (a queue never popped blocks every later request of the
// store for good).
func queueRelease(q *SafeQueue, storeID, modelName string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			q.Pop()
			CleanupQueueIfEmpty(storeID, modelName)
		})
	}
}
