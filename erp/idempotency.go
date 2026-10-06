package erp

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// collIdempotency (main DB, NEW) stores replayable responses per user+key.
const collIdempotency = "erp_idempotency"

// idemTTL is how long a key is replayable (contract: ≥ 24 h).
const idemTTL = 48 * time.Hour

var idemIndexOnce sync.Once

// per-key locks so two concurrent retries with the same key don't both run.
var idemLocks sync.Map

func ensureIdemIndex() {
	idemIndexOnce.Do(func() {
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = mainDB().Collection(collIdempotency).Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "created_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(int32(idemTTL.Seconds())).SetName("erp_idem_ttl"),
		})
	})
}

type idemRecord struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"user_id"`
	Key       string    `bson:"key"`
	Method    string    `bson:"method"`
	Path      string    `bson:"path"`
	Status    int       `bson:"status"`
	Body      string    `bson:"body"`
	CreatedAt time.Time `bson:"created_at"`
}

// withAuthAndIdem authenticates, then runs fn at most once per
// (user, Idempotency-Key); repeats replay the stored status and body.
func withAuthAndIdem(w http.ResponseWriter, r *http.Request, fn func(c *Ctx) (int, interface{}, error)) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		status, body, err := fn(c)
		respond(w, status, body, err)
		return
	}
	if len(key) > 200 {
		key = key[:200]
	}
	ensureIdemIndex()
	docID := c.UserID.Hex() + "|" + key
	muI, _ := idemLocks.LoadOrStore(docID, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	defer func() {
		mu.Unlock()
		idemLocks.Delete(docID)
	}()

	ctx, cancel := dbctx()
	var prev idemRecord
	err = mainDB().Collection(collIdempotency).FindOne(ctx, bson.M{"_id": docID}).Decode(&prev)
	cancel()
	if err == nil && time.Since(prev.CreatedAt) < idemTTL {
		w.Header().Set("Idempotent-Replayed", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(prev.Status)
		if prev.Body != "" {
			_, _ = w.Write([]byte(prev.Body))
		}
		return
	}

	status, body, ferr := fn(c)
	var payload []byte
	if ferr != nil {
		ae, ok := ferr.(*APIError)
		if !ok {
			ae = errInternal(ferr.Error())
		}
		status = ae.Status
		payload, _ = json.Marshal(map[string]interface{}{"error": ae})
	} else if body != nil {
		payload, _ = json.Marshal(body)
	}
	if status == 0 {
		status = http.StatusOK
	}
	// 429/5xx are retryable: never pin them.
	if status < 500 && status != http.StatusTooManyRequests && status != http.StatusUnauthorized {
		ctx2, cancel2 := dbctx()
		_, _ = mainDB().Collection(collIdempotency).ReplaceOne(ctx2, bson.M{"_id": docID}, idemRecord{
			ID: docID, UserID: c.UserID.Hex(), Key: key, Method: r.Method, Path: r.URL.Path,
			Status: status, Body: string(payload), CreatedAt: time.Now(),
		}, options.Replace().SetUpsert(true))
		cancel2()
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if len(payload) > 0 && status != http.StatusNoContent {
		_, _ = w.Write(append(payload, '\n'))
	}
}

func respond(w http.ResponseWriter, status int, body interface{}, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, body)
}
