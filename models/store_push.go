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
	storePushPending = map[string]bool{}
)

// coalesce runs send once, StorePushDelay after the first call for this
// key; calls made while it is pending join it. A call made after
// the pending send has started schedules a new one, so the last change is
// always followed by a send.
func coalesce(key string, send func()) {
	storePushMu.Lock()
	if storePushPending[key] {
		storePushMu.Unlock()
		return
	}
	storePushPending[key] = true
	storePushMu.Unlock()

	time.AfterFunc(StorePushDelay, func() {
		storePushMu.Lock()
		delete(storePushPending, key)
		storePushMu.Unlock()
		send()
	})
}

// NotifyStoreUpdated tells the store's connected users that the store
// changed. Call it after every successful write to the store collection;
// TestStoreWritesNotify fails for a write that doesn't. It returns at once:
// the event goes out StorePushDelay later, once per burst of writes.
func NotifyStoreUpdated(storeID primitive.ObjectID) {
	if storeID.IsZero() {
		return
	}
	coalesce(StoreUpdatedEvent+"|"+storeID.Hex(), func() { sendStoreUpdated(storeID) })
}

// NotifyStoresUpdated is NotifyStoreUpdated for several stores.
func NotifyStoresUpdated(storeIDs []primitive.ObjectID) {
	for _, id := range storeIDs {
		NotifyStoreUpdated(id)
	}
}

// ProcurementMessagesChangedEvent tells a store's users that its WhatsApp or
// email messages changed (one arrived, was sent, or was deleted), so open
// conversation screens reload instead of polling. Data: {store_id, type}.
const ProcurementMessagesChangedEvent = "procurement_messages_changed"

// sendProcurementMessagesChanged delivers the event; tests replace it.
var sendProcurementMessagesChanged = func(storeID primitive.ObjectID, msgType string) {
	emitToStoreUsersData(storeID, ProcurementMessagesChangedEvent, map[string]string{"store_id": storeID.Hex(), "type": msgType})
}

// NotifyProcurementMessagesChanged announces a change to the store's messages
// of one type ("whatsapp" or "email"), bundled like NotifyStoreUpdated so a
// mail poll saving fifty messages sends one event.
func NotifyProcurementMessagesChanged(storeID primitive.ObjectID, msgType string) {
	if storeID.IsZero() {
		return
	}
	coalesce(ProcurementMessagesChangedEvent+"|"+storeID.Hex()+"|"+msgType, func() { sendProcurementMessagesChanged(storeID, msgType) })
}

// JobProgressEvent carries a background job's progress (store backup and
// duplication) to the user who started it, instead of the browser polling
// the job's progress endpoint. Data: {job_id, kind, progress}.
const JobProgressEvent = "job_progress"

// sendJobProgress delivers the event; tests replace it.
var sendJobProgress = func(ownerUserID string, data map[string]interface{}) {
	id, err := primitive.ObjectIDFromHex(ownerUserID)
	if err != nil {
		return
	}
	NotifyUserByID(&id, JobProgressEvent, data) //nolint:errcheck
}

// PushJobProgress sends the job's current progress to its owner, at most once
// per StorePushDelay (progress changes many times a second); the snapshot is
// taken when the event goes out, so it is always the latest. Call it after
// every progress change, including the final one.
func PushJobProgress(ownerUserID, kind, jobID string, snapshot func() interface{}) {
	if ownerUserID == "" || jobID == "" {
		return
	}
	coalesce(JobProgressEvent+"|"+jobID, func() {
		sendJobProgress(ownerUserID, map[string]interface{}{"job_id": jobID, "kind": kind, "progress": snapshot()})
	})
}

// WhatsAppConnectEvent tells the browser that started a WhatsApp connect what
// happened next: a new QR to scan, the phone linked, or the wait ran out.
// Data: {store_id, qr_base64, qr_count} | {store_id, connected, phone} |
// {store_id, expired}.
const WhatsAppConnectEvent = "whatsapp_connect"

// SendWhatsAppConnect delivers the event to the user who started the connect,
// or, when the request carried no token, to the store's users; tests replace it.
var SendWhatsAppConnect = func(storeID primitive.ObjectID, ownerUserID string, data map[string]interface{}) {
	if id, err := primitive.ObjectIDFromHex(ownerUserID); err == nil {
		NotifyUserByID(&id, WhatsAppConnectEvent, data) //nolint:errcheck
		return
	}
	emitToStoreUsersData(storeID, WhatsAppConnectEvent, data)
}

// FrontendDeployedEvent tells every open page that a new frontend build is
// live, so pages with auto refresh on check index.html once instead of every
// 10 minutes. The deploy script announces it after the site serves the build.
const FrontendDeployedEvent = "frontend_deployed"

// FrontendDeployedMinGap limits announcements: the endpoint needs no sign-in
// (the deploy runs from a Mac without a user token) and an announcement costs
// each open page one read of index.html, so at most one goes out per gap.
var FrontendDeployedMinGap = 30 * time.Second

var lastFrontendDeployed time.Time

// sendFrontendDeployed reaches every connected device; tests replace it.
var sendFrontendDeployed = func() {
	type target struct{ user, device string }
	var targets []target
	ClientsMu.Lock()
	for userID, devices := range Clients {
		for deviceID := range devices {
			targets = append(targets, target{userID, deviceID})
		}
	}
	ClientsMu.Unlock()
	for _, t := range targets {
		Emit(t.user, t.device, FrontendDeployedEvent, nil)
	}
}

// NotifyFrontendDeployed announces a new frontend build; false when one was
// announced less than FrontendDeployedMinGap ago.
func NotifyFrontendDeployed() bool {
	storePushMu.Lock()
	if !lastFrontendDeployed.IsZero() && time.Since(lastFrontendDeployed) < FrontendDeployedMinGap {
		storePushMu.Unlock()
		return false
	}
	lastFrontendDeployed = time.Now()
	storePushMu.Unlock()
	go sendFrontendDeployed()
	return true
}

// DashboardRecomputedEvent tells a store's users that a dashboard recompute
// (POST /v1/dashboard/backfill) finished, so open dashboards reload once
// instead of waiting a guessed 30 seconds. Data: {store_id}.
const DashboardRecomputedEvent = "dashboard_recomputed"

// sendDashboardRecomputed delivers the event; tests replace it.
var sendDashboardRecomputed = func(storeID primitive.ObjectID) {
	emitToStoreUsersData(storeID, DashboardRecomputedEvent, map[string]string{"store_id": storeID.Hex()})
}

// NotifyDashboardRecomputed announces a finished recompute.
func NotifyDashboardRecomputed(storeID primitive.ObjectID) {
	if storeID.IsZero() {
		return
	}
	coalesce(DashboardRecomputedEvent+"|"+storeID.Hex(), func() { sendDashboardRecomputed(storeID) })
}
