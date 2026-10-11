package controller

// After POST /v1/whatsapp/connect the server watches the new instance and
// pushes "whatsapp_connect" over the WebSocket when the QR changes and when the
// phone is linked; the connect dialogs used to ask for the status and the QR
// every 3 seconds. One watch per store; a new connect or a disconnect ends the
// previous one, and it gives up after whatsAppWatchFor.

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var (
	whatsAppWatchEvery = 2 * time.Second
	whatsAppWatchFor   = 5 * time.Minute
	// Evolution API calls, replaced in tests.
	whatsAppWatchStatus = whatsAppConnectionStatus
	whatsAppWatchQR     = func(storeIDStr string) ([]byte, error) {
		evoURL, evoKey, instanceName := evoConfigFromStore(storeIDStr)
		return whatsAppQR(evoURL, evoKey, instanceName)
	}

	whatsAppWatchMu sync.Mutex
	whatsAppWatches = map[primitive.ObjectID]chan struct{}{}
)

func startWhatsAppConnectWatch(storeID primitive.ObjectID, owner string) {
	stop := make(chan struct{})
	whatsAppWatchMu.Lock()
	if prev, ok := whatsAppWatches[storeID]; ok {
		close(prev)
	}
	whatsAppWatches[storeID] = stop
	whatsAppWatchMu.Unlock()
	go watchWhatsAppConnect(storeID, owner, stop, whatsAppWatchSettings{
		every: whatsAppWatchEvery, limit: whatsAppWatchFor,
		status: whatsAppWatchStatus, qr: whatsAppWatchQR, send: models.SendWhatsAppConnect,
	})
}

// whatsAppWatchSettings are read once when the watch starts.
type whatsAppWatchSettings struct {
	every, limit time.Duration
	status       func(string) (whatsAppStatus, error)
	qr           func(string) ([]byte, error)
	send         func(primitive.ObjectID, string, map[string]interface{})
}

func stopWhatsAppConnectWatch(storeID primitive.ObjectID) {
	whatsAppWatchMu.Lock()
	if stop, ok := whatsAppWatches[storeID]; ok {
		close(stop)
		delete(whatsAppWatches, storeID)
	}
	whatsAppWatchMu.Unlock()
}

// endWhatsAppConnectWatch forgets the watch when it ends on its own, unless a
// newer connect already replaced it.
func endWhatsAppConnectWatch(storeID primitive.ObjectID, stop chan struct{}) {
	whatsAppWatchMu.Lock()
	if whatsAppWatches[storeID] == stop {
		delete(whatsAppWatches, storeID)
	}
	whatsAppWatchMu.Unlock()
}

func watchWhatsAppConnect(storeID primitive.ObjectID, owner string, stop chan struct{}, cfg whatsAppWatchSettings) {
	defer endWhatsAppConnectWatch(storeID, stop)
	sid := storeID.Hex()
	send := func(data map[string]interface{}) {
		select {
		case <-stop: // replaced or disconnected meanwhile: say nothing
		default:
			data["store_id"] = sid
			cfg.send(storeID, owner, data)
		}
	}

	ticker := time.NewTicker(cfg.every)
	defer ticker.Stop()
	giveUp := time.NewTimer(cfg.limit)
	defer giveUp.Stop()
	lastQR := ""
	for {
		select {
		case <-stop:
			return
		case <-giveUp.C:
			send(map[string]interface{}{"expired": true})
			return
		case <-ticker.C:
		}

		st, err := cfg.status(sid)
		if err != nil {
			continue // Evolution API briefly unreachable: try again next tick
		}
		if st.Connected {
			send(map[string]interface{}{"connected": true, "phone": st.Phone})
			return
		}
		raw, err := cfg.qr(sid)
		if err != nil {
			continue
		}
		var qr struct {
			Base64 string `json:"base64"`
			Count  int    `json:"count"`
		}
		if json.Unmarshal(raw, &qr) != nil || qr.Base64 == "" || qr.Base64 == lastQR {
			continue
		}
		lastQR = qr.Base64
		send(map[string]interface{}{"qr_base64": qr.Base64, "qr_count": qr.Count})
	}
}
