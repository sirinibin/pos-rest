package controller

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type waFake struct {
	mu        sync.Mutex
	connected bool
	phone     string
	qr        string
	qrCount   int
	down      bool
	sent      []map[string]interface{}
	owners    []string
}

func (f *waFake) status(string) (whatsAppStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return whatsAppStatus{}, errors.New("unreachable")
	}
	return whatsAppStatus{Connected: f.connected, Phone: f.phone}, nil
}

func (f *waFake) qrCode(string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		return nil, errors.New("unreachable")
	}
	return []byte(fmt.Sprintf(`{"base64":%q,"count":%d}`, f.qr, f.qrCount)), nil
}

func (f *waFake) events() []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]interface{}(nil), f.sent...)
}

func useWhatsAppFake(t *testing.T) *waFake {
	t.Helper()
	f := &waFake{}
	oldEvery, oldFor, oldStatus, oldQR, oldSend := whatsAppWatchEvery, whatsAppWatchFor, whatsAppWatchStatus, whatsAppWatchQR, models.SendWhatsAppConnect
	whatsAppWatchEvery = 10 * time.Millisecond
	whatsAppWatchFor = time.Minute
	whatsAppWatchStatus = f.status
	whatsAppWatchQR = f.qrCode
	models.SendWhatsAppConnect = func(_ primitive.ObjectID, owner string, data map[string]interface{}) {
		f.mu.Lock()
		f.sent = append(f.sent, data)
		f.owners = append(f.owners, owner)
		f.mu.Unlock()
	}
	t.Cleanup(func() {
		whatsAppWatchEvery, whatsAppWatchFor, whatsAppWatchStatus, whatsAppWatchQR, models.SendWhatsAppConnect = oldEvery, oldFor, oldStatus, oldQR, oldSend
	})
	return f
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func watchRunning(id primitive.ObjectID) bool {
	whatsAppWatchMu.Lock()
	defer whatsAppWatchMu.Unlock()
	_, ok := whatsAppWatches[id]
	return ok
}

func TestWhatsAppConnectWatch_PushesEachNewQROnceThenConnected(t *testing.T) {
	f := useWhatsAppFake(t)
	id := primitive.NewObjectID()
	f.qr, f.qrCount = "QR1", 1
	startWhatsAppConnectWatch(id, "owner1")

	waitFor(t, "first QR", func() bool { return len(f.events()) == 1 })
	time.Sleep(60 * time.Millisecond) // several checks, same QR
	if n := len(f.events()); n != 1 {
		t.Fatalf("same QR pushed %d times", n)
	}

	f.mu.Lock()
	f.qr, f.qrCount = "QR2", 2
	f.mu.Unlock()
	waitFor(t, "second QR", func() bool { return len(f.events()) == 2 })

	f.mu.Lock()
	f.connected, f.phone = true, "966500000000"
	f.mu.Unlock()
	waitFor(t, "connected", func() bool { return len(f.events()) == 3 })
	waitFor(t, "watch ends", func() bool { return !watchRunning(id) })

	ev := f.events()
	if ev[0]["qr_base64"] != "QR1" || ev[1]["qr_base64"] != "QR2" {
		t.Fatalf("QR events %v", ev)
	}
	if ev[2]["connected"] != true || ev[2]["phone"] != "966500000000" || ev[2]["store_id"] != id.Hex() {
		t.Fatalf("connected event %v", ev[2])
	}
	if f.owners[0] != "owner1" {
		t.Fatalf("sent to %q, want the user who connected", f.owners[0])
	}
}

func TestWhatsAppConnectWatch_UnreachableEvolutionIsRetriedQuietly(t *testing.T) {
	f := useWhatsAppFake(t)
	id := primitive.NewObjectID()
	f.down = true
	startWhatsAppConnectWatch(id, "")
	time.Sleep(50 * time.Millisecond)
	if len(f.events()) != 0 {
		t.Fatal("pushed while Evolution API was unreachable")
	}
	f.mu.Lock()
	f.down, f.connected = false, true
	f.mu.Unlock()
	waitFor(t, "connected", func() bool { return len(f.events()) == 1 })
}

func TestWhatsAppConnectWatch_ExpiresAfterLimit(t *testing.T) {
	f := useWhatsAppFake(t)
	whatsAppWatchFor = 50 * time.Millisecond
	id := primitive.NewObjectID()
	startWhatsAppConnectWatch(id, "")
	waitFor(t, "expiry", func() bool {
		ev := f.events()
		return len(ev) == 1 && ev[0]["expired"] == true
	})
	waitFor(t, "watch ends", func() bool { return !watchRunning(id) })
}

func TestWhatsAppConnectWatch_NewConnectOrDisconnectEndsTheOldWatch(t *testing.T) {
	f := useWhatsAppFake(t)
	id := primitive.NewObjectID()
	startWhatsAppConnectWatch(id, "a")
	whatsAppWatchMu.Lock()
	first := whatsAppWatches[id]
	whatsAppWatchMu.Unlock()

	startWhatsAppConnectWatch(id, "b")
	select {
	case <-first:
	default:
		t.Fatal("the first watch was not stopped by the new connect")
	}

	stopWhatsAppConnectWatch(id)
	if watchRunning(id) {
		t.Fatal("disconnect left the watch running")
	}
	f.mu.Lock()
	f.connected = true
	f.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	if n := len(f.events()); n != 0 {
		t.Fatalf("a stopped watch pushed %d events", n)
	}
}
