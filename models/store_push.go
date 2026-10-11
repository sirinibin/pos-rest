package models

import (
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// StoreUpdatedEvent is the WebSocket event sent to a store's connected users
// whenever the store document changes (settings, print settings, sidebar
// config, ZATCA state, WhatsApp/email setup, logo, deletion state, ...).
// Browsers keep one copy of the store per page and re-read it only on this
// event (or after a reconnect), instead of asking for it on every screen.
// The event carries only the store id: the store holds credentials, so its
// fields are never broadcast; each browser re-reads it through the API,
// which applies the usual access checks.
const StoreUpdatedEvent = "store_updated"

// StorePushDelay bundles bursts of writes to one store (a form save that also
// stores a logo, a settings page saving several sections) into one event.
var StorePushDelay = 300 * time.Millisecond

// sendStoreUpdated delivers the event; tests replace it.
var sendStoreUpdated = func(storeID primitive.ObjectID) {
	emitToStoreUsersData(storeID, StoreUpdatedEvent, map[string]string{"store_id": storeID.Hex()})
}

var (
	storePushMu      sync.Mutex
	storePushPending = map[primitive.ObjectID]bool{}
)

// NotifyStoreUpdated tells the store's connected users that the store
// changed. Call it after every successful write to the store collection;
// TestStoreWritesNotify fails for a write that doesn't. It returns at once:
// the event goes out StorePushDelay later, once per burst of writes. A write
// made after the pending event has started sending schedules a new one, so
// the last write is always followed by an event.
func NotifyStoreUpdated(storeID primitive.ObjectID) {
	if storeID.IsZero() {
		return
	}
	storePushMu.Lock()
	if storePushPending[storeID] {
		storePushMu.Unlock()
		return
	}
	storePushPending[storeID] = true
	storePushMu.Unlock()

	time.AfterFunc(StorePushDelay, func() {
		storePushMu.Lock()
		delete(storePushPending, storeID)
		storePushMu.Unlock()
		sendStoreUpdated(storeID)
	})
}

// NotifyStoresUpdated is NotifyStoreUpdated for several stores.
func NotifyStoresUpdated(storeIDs []primitive.ObjectID) {
	for _, id := range storeIDs {
		NotifyStoreUpdated(id)
	}
}
