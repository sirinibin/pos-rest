package models

import (
	"log"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// In-process locks for writes that read, compute and save shared data (a
// store's books, a product's stock, a ledger account, a document's version).
//
// The API runs as ONE process: these locks serialise the requests and the
// background goroutines of that process only. They do not hold across
// several API processes.
//
// Every lock is taken with a bounded wait (LockWait): a holder that never
// releases (a bug, a hung call) slows the next writers down, it never stops
// them for good. Every unlock is idempotent, so a deferred unlock is safe on
// every path, panics included.

// LockWait is how long a writer waits for a lock before it logs and goes on
// without it.
var LockWait = 60 * time.Second

type lockEntry struct {
	ch   chan struct{} // capacity 1: holding the lock = one value in ch
	refs int           // waiters + holder; the entry is dropped at 0
}

// KeyedLocks is a set of mutexes by key, created on first use and dropped
// when nobody holds or waits for them (no growth with the number of keys).
type KeyedLocks struct {
	mu sync.Mutex
	m  map[string]*lockEntry
}

// Lock locks key (waiting at most LockWait) and returns its unlock.
func (k *KeyedLocks) Lock(key string) (unlock func()) {
	return k.LockWait(key, LockWait)
}

// LockWait locks key, waiting at most wait; after that it logs and returns
// without holding the lock (the returned unlock is then a no-op apart from
// bookkeeping).
func (k *KeyedLocks) LockWait(key string, wait time.Duration) (unlock func()) {
	k.mu.Lock()
	if k.m == nil {
		k.m = map[string]*lockEntry{}
	}
	e := k.m[key]
	if e == nil {
		e = &lockEntry{ch: make(chan struct{}, 1)}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()

	acquired := false
	timer := time.NewTimer(wait)
	select {
	case e.ch <- struct{}{}:
		acquired = true
	case <-timer.C:
		log.Printf("lock %q: still held after %s; continuing without it", key, wait)
	}
	timer.Stop()

	var once sync.Once
	return func() {
		once.Do(func() {
			if acquired {
				<-e.ch
			}
			k.mu.Lock()
			e.refs--
			if e.refs == 0 {
				delete(k.m, key)
			}
			k.mu.Unlock()
		})
	}
}

// held reports whether key is currently locked (tests).
func (k *KeyedLocks) held(key string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	e := k.m[key]
	return e != nil && len(e.ch) == 1
}

// keyedLock locks key in locks and returns its unlock.
func keyedLock(locks *KeyedLocks, key string) func() { return locks.Lock(key) }

// Checks that read the database and then insert ("is this email taken?",
// "was the same receipt just saved?", "is this draft still there?") pass for
// every one of several requests that arrive together. namedLocks serialises
// such check-and-write per key.
var namedLocks KeyedLocks

// LockKey locks a free-form key and returns its unlock. Lock order when
// several are needed: a document key → the store hold → books → account /
// product locks.
func LockKey(key string) (unlock func()) { return keyedLock(&namedLocks, key) }

// ---- per-store write hold ----

// storeLocks serialises the writes of one store: a document write (create,
// edit, delete, payment) together with the stock, ledger, balance and stats
// work its handler starts in the background. Different stores never wait for
// each other.
var storeLocks KeyedLocks

// StoreHold is a store's write lock held by a request and the background
// goroutines it starts (Go): the lock is released when the request and every
// one of those goroutines are done. A nil *StoreHold is valid and holds
// nothing.
type StoreHold struct {
	mu     sync.Mutex
	n      int
	unlock func()
}

// HoldStore takes the store's write lock for the calling request. Release it
// with Release (defer it right away); background work started with Go keeps
// the store locked until it ends too.
func HoldStore(storeID primitive.ObjectID) *StoreHold {
	return &StoreHold{n: 1, unlock: storeLocks.Lock(storeID.Hex())}
}

// Go runs fn in a new goroutine that shares the hold: the store stays locked
// until fn returns (or panics).
func (h *StoreHold) Go(fn func()) {
	if h == nil {
		go fn()
		return
	}
	h.mu.Lock()
	h.n++
	h.mu.Unlock()
	go func() {
		defer h.Release()
		fn()
	}()
}

// Release drops one share of the hold (the request's, or a goroutine's); the
// store is unlocked when the last share goes. Extra calls are ignored.
func (h *StoreHold) Release() {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.n <= 0 {
		h.mu.Unlock()
		return
	}
	h.n--
	done := h.n == 0
	h.mu.Unlock()
	if done {
		h.unlock()
	}
}

// ledgerJournalsMu guards the package-level payment totals (totalSalesPaidAmount,
// extraPurchasePayments, ...) that the CreateLedger of sales, quotations,
// purchases, their returns and non-VAT documents reset and fill while they
// build the journals: two ledgers built at once (any stores) mixed their
// amounts. Sales and quotation invoices share the same variables, so it is
// one mutex for all of them. Only account locks are taken while it is held.
var ledgerJournalsMu sync.Mutex
