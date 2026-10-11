package controller

import "sync"

// queueRelease returns a function that takes this request's token out of the
// store's queue once (SafeQueue.Done). Later calls are no-ops, so a handler both
// calls it where it used to pop and defers it: the queue is released on every
// path, a panic included (a queue never released blocks every later request of
// the store for good).
func queueRelease(q *SafeQueue, token string) func() {
	var once sync.Once
	return func() {
		once.Do(func() { q.Done(token) })
	}
}
