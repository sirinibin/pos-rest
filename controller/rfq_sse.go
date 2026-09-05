package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// sseHub manages Server-Sent Event connections keyed by store ID.
// Each store can have multiple browser tabs connected simultaneously.
type sseHub struct {
	mu      sync.RWMutex
	clients map[string][]chan string // storeID -> list of pre-formatted SSE message channels
}

var rfqSSEHub = &sseHub{clients: make(map[string][]chan string)}

func (h *sseHub) subscribe(storeID string) chan string {
	ch := make(chan string, 16)
	h.mu.Lock()
	h.clients[storeID] = append(h.clients[storeID], ch)
	h.mu.Unlock()
	return ch
}

func (h *sseHub) unsubscribe(storeID string, ch chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.clients[storeID]
	for i, c := range list {
		if c == ch {
			h.clients[storeID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(h.clients[storeID]) == 0 {
		delete(h.clients, storeID)
	}
}

func (h *sseHub) broadcast(storeID, msg string) {
	h.mu.RLock()
	list := h.clients[storeID]
	h.mu.RUnlock()
	for _, ch := range list {
		select {
		case ch <- msg:
		default: // client too slow — skip
		}
	}
}

// BroadcastRFQEvent sends a simple event (no payload) to all SSE clients for the given store.
func BroadcastRFQEvent(storeID, eventType string) {
	rfqSSEHub.broadcast(storeID, fmt.Sprintf("event: %s\ndata: {}\n\n", eventType))
}

// BroadcastRFQData sends an event with a JSON data payload to all SSE clients for the given store.
func BroadcastRFQData(storeID, eventType string, data map[string]interface{}) {
	b, err := json.Marshal(data)
	if err != nil {
		BroadcastRFQEvent(storeID, eventType)
		return
	}
	rfqSSEHub.broadcast(storeID, fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(b)))
}

// RFQEventsHandler is the SSE endpoint.
// GET /v1/rfq-bot/events?store_id=...
func RFQEventsHandler(w http.ResponseWriter, r *http.Request) {
	storeID := r.URL.Query().Get("store_id")
	if storeID == "" {
		http.Error(w, "store_id required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	ch := rfqSSEHub.subscribe(storeID)
	defer rfqSSEHub.unsubscribe(storeID, ch)

	log.Printf("rfq_sse: client connected store=%s", storeID)

	// Send an initial ping so the browser knows the stream is open
	fmt.Fprintf(w, "event: connected\ndata: ok\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			log.Printf("rfq_sse: client disconnected store=%s", storeID)
			return
		case msg := <-ch:
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}
