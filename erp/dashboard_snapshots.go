package erp

// dashboard_snapshots.go — ready-made dashboard data, kept up to date.
//
// Every dashboard endpoint (the BI dashboard, the main dashboard's feed, total
// expense, VAT, revenue, salary balance) is served from a per-store snapshot:
// the endpoint's response worked out in advance and kept in memory and in MongoDB
// (main DB, erp_dashboard_snapshots), so opening a dashboard is one quick read
// instead of a pass over the store's history.
//
// A snapshot is served as up to date only when nothing in the store changed since
// it was built. That is known from
//
//   - events: every write through the adapter (engine.go) and every legacy write
//     that marks the dashboard dirty (models.MarkDashboardDirty) rebuilds the
//     store's snapshots in the background a moment later (debounced);
//   - a fingerprint of the store: its settings and, per collection of its
//     database, the document count, the newest _id and the latest updated_at.
//     A scheduled check recomputes it every SnapshotCheckEvery and rebuilds when
//     anything changed some other way (the old app, imports, scripts), when the
//     store's day rolled over (figures that depend on "today"), and when a
//     snapshot is older than SnapshotMaxAge whatever happened.
//
// A read whose snapshot is out of date waits up to SnapshotWait for the rebuild;
// if that takes longer the previous snapshot is served marked stale and the page
// asks again a moment later. ?fresh=1 forces a rebuild (the dashboards' Recompute).
//
// Responses carry "snapshot": {generatedAt, stale, buildMs} and an ETag.

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collDashSnapshots = "erp_dashboard_snapshots"

var (
	// SnapshotWait is how long a read waits for an out-of-date snapshot's rebuild.
	SnapshotWait = 2500 * time.Millisecond
	// SnapshotDebounce: a burst of writes rebuilds once, this long after the last one…
	SnapshotDebounce = 1500 * time.Millisecond
	// …but never later than this after the first.
	SnapshotMaxDelay = 10 * time.Second
	// SnapshotCheckEvery is how often the scheduled check fingerprints active stores.
	SnapshotCheckEvery = 2 * time.Minute
	// SnapshotMaxAge: rebuilt at least this often, whatever the fingerprint says.
	SnapshotMaxAge = 6 * time.Hour
	// SnapshotActiveFor: snapshots read within this window are kept up to date.
	SnapshotActiveFor = 48 * time.Hour
	// SnapshotBuilders limits rebuilds running at once (database load).
	SnapshotBuilders = 3
	// SnapshotVerifyAfter: a read re-takes the store's fingerprint when the last one
	// is older than this, so a change made outside the app shows within seconds.
	SnapshotVerifyAfter = 5 * time.Second
)

// dashKind is one cached dashboard endpoint.
type dashKind struct {
	Name    string
	Params  []string // query parameters that are part of the key (besides storeId)
	Daily   bool     // depends on the store's today
	Rev     int      // response format revision: a new one never serves snapshots saved in an older format
	Handler func(c *Ctx, w http.ResponseWriter, r *http.Request) error
}

var dashKinds = map[string]*dashKind{}

func registerDashKind(k *dashKind) *dashKind {
	dashKinds[k.Name] = k
	return k
}

// dashSnap is one built response.
type dashSnap struct {
	Key, Store, Kind, Params string
	FP                       string // store fingerprint it was built from
	Gen                      int64  // store event generation it was built at
	Day                      string // store's today when built
	Body                     []byte // the endpoint's JSON object
	GeneratedAt              time.Time
	BuildMs                  int64
	loc                      *time.Location
}

// dashStore is the per-store state.
type dashStore struct {
	mu        sync.Mutex
	hex       string
	fp        string    // last fingerprint ("" = not known yet)
	fpAt      time.Time // when it was taken
	gen       int64     // bumped by every write event
	timer     *time.Timer
	firstAt   time.Time            // first event of the pending burst
	fpFlight  chan struct{}        // a fingerprint being taken
	used      map[string]time.Time // key → last read
	usedSaved map[string]time.Time // key → last read written to Mongo
	params    map[string]url.Values
}

type dashFlight struct {
	done chan struct{}
	snap *dashSnap
	err  error
}

var (
	dashMu      sync.Mutex
	dashStores  = map[string]*dashStore{}
	dashMem     = map[string]*dashSnap{}
	dashFlights = map[string]*dashFlight{}
	dashSem     chan struct{}
	dashStarted bool
	dashBuilds  int64 // snapshots built (tests)
	dashIdxOnce sync.Once
	// fpCollIdx: collections whose updated_at index was ensured.
	fpCollIdx sync.Map
)

func init() { dashSem = make(chan struct{}, SnapshotBuilders) }

func storeState(hex string) *dashStore {
	dashMu.Lock()
	defer dashMu.Unlock()
	st := dashStores[hex]
	if st == nil {
		st = &dashStore{hex: hex, used: map[string]time.Time{}, usedSaved: map[string]time.Time{}, params: map[string]url.Values{}}
		dashStores[hex] = st
	}
	return st
}

// snapKey is the cache key of kind + its key parameters for a store.
func snapKey(store string, k *dashKind, q url.Values) (string, string, url.Values) {
	p := url.Values{}
	parts := []string{}
	for _, name := range k.Params {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			p.Set(name, v)
			parts = append(parts, name+"="+v)
		}
	}
	params := strings.Join(parts, "&")
	name := k.Name
	if k.Rev > 0 {
		name += "@" + strconv.Itoa(k.Rev)
	}
	return store + "|" + name + "|" + params, params, p
}

// kindOfKey is the kind a snapshot key belongs to; nil when the kind is gone or the key
// is of an older format revision ("feed" once "feed@2" is current).
func kindOfKey(key string) *dashKind {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) < 3 {
		return nil
	}
	name, rev := parts[1], 0
	if i := strings.IndexByte(name, '@'); i >= 0 {
		rev, _ = strconv.Atoi(name[i+1:])
		name = name[:i]
	}
	k := dashKinds[name]
	if k == nil || k.Rev != rev {
		return nil
	}
	return k
}

// ---- events ----

// DashboardChanged says a store's data changed: its snapshots are out of date
// and are rebuilt in the background shortly (writes in a burst rebuild once).
func DashboardChanged(storeHex string) {
	if storeHex == "" {
		return
	}
	dashMu.Lock()
	started := dashStarted
	dashMu.Unlock()
	st := storeState(storeHex)
	// lock order: dashMu before a store's mu, never the other way round
	st.mu.Lock()
	defer st.mu.Unlock()
	st.gen++
	if !started {
		return
	}
	now := time.Now()
	if st.timer == nil {
		st.firstAt = now
	}
	delay := SnapshotDebounce
	if wait := SnapshotMaxDelay - now.Sub(st.firstAt); wait < delay {
		delay = wait
	}
	if delay < 0 {
		delay = 0
	}
	if st.timer != nil {
		st.timer.Stop()
	}
	st.timer = time.AfterFunc(delay, func() {
		st.mu.Lock()
		st.timer = nil
		st.mu.Unlock()
		refreshStore(st)
	})
}

// dashboardTouched is called by the adapter after every successful write.
func dashboardTouched(res *Resource, storeHex, id string) {
	if storeHex != "" {
		DashboardChanged(storeHex)
		return
	}
	if res != nil && res.Name == "stores" && id != "" {
		DashboardChanged(id) // settings (VAT, flags, BI settings, country)
	}
}

// ---- fingerprint ----

// fingerprintSkip: collections that never feed a dashboard figure.
var fingerprintSkip = []string{"history", "_log", "logs", "draft", "dashboard", "idempot", "notification", "whatsapp",
	"zatca", "thread", "message", "session", "otp", "rfq", "erp_"}

func fingerprintable(name string) bool {
	if strings.HasPrefix(name, "system.") {
		return false
	}
	l := strings.ToLower(name)
	for _, s := range fingerprintSkip {
		if strings.Contains(l, s) {
			return false
		}
	}
	return true
}

// StoreFingerprint changes whenever the store's settings or any record of its
// database is added, edited (updated_at) or removed.
func StoreFingerprint(storeHex string) (string, error) {
	oid, err := primitive.ObjectIDFromHex(storeHex)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := sha1.New()
	raw, err := mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid}).Raw()
	if err != nil && err != mongo.ErrNoDocuments {
		return "", err
	}
	h.Write(raw)
	sdb := storeDB(storeHex)
	names, err := sdb.ListCollectionNames(ctx, bson.M{})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	type part struct {
		i   int
		s   string
		err error
	}
	out := make(chan part, len(names))
	sem := make(chan struct{}, 6)
	n := 0
	for i, name := range names {
		if !fingerprintable(name) {
			continue
		}
		n++
		go func(i int, name string) {
			sem <- struct{}{}
			defer func() { <-sem }()
			s, err := collFingerprint(ctx, sdb.Collection(name), storeHex+"."+name)
			out <- part{i, name + ":" + s, err}
		}(i, name)
	}
	parts := make([]string, len(names))
	for k := 0; k < n; k++ {
		p := <-out
		if p.err != nil {
			return "", p.err
		}
		parts[p.i] = p.s
	}
	h.Write([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ensureFP re-takes the store's fingerprint when it is unknown or older than
// maxAge (concurrent callers share one).
func ensureFP(st *dashStore, maxAge time.Duration) {
	st.mu.Lock()
	if st.fp != "" && time.Since(st.fpAt) <= maxAge {
		st.mu.Unlock()
		return
	}
	if ch := st.fpFlight; ch != nil {
		st.mu.Unlock()
		<-ch
		return
	}
	ch := make(chan struct{})
	st.fpFlight = ch
	st.mu.Unlock()
	fp, err := StoreFingerprint(st.hex)
	st.mu.Lock()
	if err == nil {
		st.fp, st.fpAt = fp, time.Now()
	} else {
		log.Printf("[dashboard] fingerprint %s: %v", st.hex, err)
	}
	st.fpFlight = nil
	st.mu.Unlock()
	close(ch)
}

func collFingerprint(ctx context.Context, col *mongo.Collection, idxKey string) (string, error) {
	if _, done := fpCollIdx.LoadOrStore(idxKey, true); !done {
		// the latest updated_at is read through an index (one key, not a scan)
		_, _ = col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "updated_at", Value: -1}}})
	}
	count, err := col.EstimatedDocumentCount(ctx)
	if err != nil {
		return "", err
	}
	last := func(key string) string {
		raw, err := col.FindOne(ctx, bson.M{key: bson.M{"$exists": true}},
			options.FindOne().SetSort(bson.D{{Key: key, Value: -1}}).SetProjection(bson.M{key: 1})).Raw()
		if err != nil {
			return ""
		}
		return raw.Lookup(key).String()
	}
	return fmt.Sprintf("%d/%s/%s", count, last("_id"), last("updated_at")), nil
}

// ---- building ----

// systemCtx is an administrator context for one store (background rebuilds).
func systemCtx(storeHex string) (*Ctx, error) {
	doc := loadStoreRaw(storeHex)
	if doc == nil {
		return nil, errNotFound()
	}
	c := &Ctx{Admin: true, UserName: "system", RoleID: "r_admin", Stores: []M{doc}, storeIdx: map[string]M{storeHex: doc}}
	c.Perms = permsFromM(systemRoleByID("r_admin")["perms"])
	return c, nil
}

// bufWriter captures a handler's response.
type bufWriter struct {
	h      http.Header
	status int
	buf    bytes.Buffer
}

func (b *bufWriter) Header() http.Header { return b.h }
func (b *bufWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.buf.Write(p)
}
func (b *bufWriter) WriteHeader(s int) { b.status = s }

func storeToday(loc *time.Location) string { return time.Now().In(orRiyadh(loc)).Format(layoutDay) }

// buildSnap runs the endpoint for the store (as the system) and keeps the result.
func buildSnap(st *dashStore, k *dashKind, key, params string, q url.Values) (*dashSnap, error) {
	dashSem <- struct{}{}
	defer func() { <-dashSem }()
	ensureFP(st, SnapshotVerifyAfter)
	st.mu.Lock()
	gen, fp := st.gen, st.fp
	st.mu.Unlock()
	if fp == "" {
		return nil, fmt.Errorf("dashboard %s: no fingerprint", k.Name)
	}
	c, err := systemCtx(st.hex)
	if err != nil {
		return nil, err
	}
	loc := c.storeLoc(st.hex)
	qq := url.Values{}
	for k, v := range q {
		qq[k] = v
	}
	qq.Set("storeId", st.hex)
	r, _ := http.NewRequest("GET", Prefix+"/dashboard/"+k.Name+"?"+qq.Encode(), nil)
	c.R = r
	w := &bufWriter{h: http.Header{}}
	start := time.Now()
	if err := k.Handler(c, w, r); err != nil {
		return nil, err
	}
	if w.status != http.StatusOK {
		return nil, fmt.Errorf("dashboard %s: status %d", k.Name, w.status)
	}
	body := bytes.TrimSpace(w.buf.Bytes())
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("dashboard %s: not a JSON object", k.Name)
	}
	s := &dashSnap{Key: key, Store: st.hex, Kind: k.Name, Params: params, FP: fp, Gen: gen, Day: storeToday(loc),
		Body: append([]byte{}, body...), GeneratedAt: time.Now(), BuildMs: time.Since(start).Milliseconds(), loc: loc}
	dashMu.Lock()
	dashMem[key] = s
	dashMu.Unlock()
	atomic.AddInt64(&dashBuilds, 1)
	go saveSnap(s)
	return s, nil
}

// rebuild builds key once at a time; callers share the flight.
func rebuild(st *dashStore, k *dashKind, key, params string, q url.Values) *dashFlight {
	dashMu.Lock()
	if f := dashFlights[key]; f != nil {
		dashMu.Unlock()
		return f
	}
	f := &dashFlight{done: make(chan struct{})}
	dashFlights[key] = f
	dashMu.Unlock()
	go func() {
		defer func() {
			if p := recover(); p != nil {
				f.err = fmt.Errorf("dashboard %s: %v", k.Name, p)
			}
			dashMu.Lock()
			delete(dashFlights, key)
			dashMu.Unlock()
			close(f.done)
		}()
		f.snap, f.err = buildSnap(st, k, key, params, q)
		if f.err != nil {
			log.Printf("[dashboard] rebuild %s: %v", key, f.err)
		}
	}()
	return f
}

// fresh: built from the store as it is now (and today, for daily figures).
func (s *dashSnap) fresh(st *dashStore, k *dashKind) bool {
	if s == nil {
		return false
	}
	st.mu.Lock()
	ok := st.fp != "" && s.FP == st.fp && s.Gen == st.gen
	st.mu.Unlock()
	if !ok {
		return false
	}
	if k.Daily && s.Day != storeToday(s.loc) {
		return false
	}
	return time.Since(s.GeneratedAt) < SnapshotMaxAge
}

// refreshStore takes a new fingerprint and rebuilds the store's active snapshots
// that are out of date (all of them after a write event).
func refreshStore(st *dashStore) {
	ensureFP(st, 0)
	st.mu.Lock()
	type job struct {
		key    string
		params url.Values
		kind   *dashKind
	}
	jobs := []job{}
	for key, at := range st.used {
		if time.Since(at) > SnapshotActiveFor {
			delete(st.used, key)
			delete(st.params, key)
			continue
		}
		k := kindOfKey(key)
		if k == nil {
			delete(st.used, key) // a kind gone, or saved in an older format
			delete(st.params, key)
			continue
		}
		jobs = append(jobs, job{key, st.params[key], k})
	}
	st.mu.Unlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].key < jobs[j].key })
	for _, j := range jobs {
		dashMu.Lock()
		s := dashMem[j.key]
		dashMu.Unlock()
		if s == nil {
			s = loadSnap(j.key)
		}
		if s.fresh(st, j.kind) {
			continue
		}
		_, params, _ := snapKey(st.hex, j.kind, j.params)
		// a build already running may have started before the latest change: once more
		for try := 0; try < 2; try++ {
			f := rebuild(st, j.kind, j.key, params, j.params)
			<-f.done
			if f.err != nil || f.snap.fresh(st, j.kind) {
				break
			}
		}
	}
}

// ---- persistence ----

func ensureSnapIndexes() {
	dashIdxOnce.Do(func() {
		ctx, cancel := dbctx()
		defer cancel()
		col := mainDB().Collection(collDashSnapshots)
		_, _ = col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "used_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(14 * 24 * 3600)},
			{Keys: bson.D{{Key: "store", Value: 1}}},
		})
	})
}

func gz(b []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func saveSnap(s *dashSnap) {
	if mainDBUnavailable() {
		return
	}
	ensureSnapIndexes()
	body := gz(s.Body)
	if len(body) > 15<<20 {
		return // over MongoDB's document limit: kept in memory only
	}
	ctx, cancel := dbctx()
	defer cancel()
	_, err := mainDB().Collection(collDashSnapshots).ReplaceOne(ctx, bson.M{"_id": s.Key}, bson.M{
		"_id": s.Key, "store": s.Store, "kind": s.Kind, "params": s.Params, "fp": s.FP, "day": s.Day,
		"body": primitive.Binary{Data: body}, "generated_at": s.GeneratedAt, "build_ms": s.BuildMs, "used_at": time.Now(),
	}, options.Replace().SetUpsert(true))
	if err != nil {
		log.Printf("[dashboard] save %s: %v", s.Key, err)
	}
}

// loadSnap reads a snapshot saved by an earlier run (nil when none).
func loadSnap(key string) *dashSnap {
	if mainDBUnavailable() {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var d struct {
		Store       string           `bson:"store"`
		Kind        string           `bson:"kind"`
		Params      string           `bson:"params"`
		FP          string           `bson:"fp"`
		Day         string           `bson:"day"`
		Body        primitive.Binary `bson:"body"`
		GeneratedAt time.Time        `bson:"generated_at"`
		BuildMs     int64            `bson:"build_ms"`
	}
	if err := mainDB().Collection(collDashSnapshots).FindOne(ctx, bson.M{"_id": key}).Decode(&d); err != nil {
		return nil
	}
	body, err := gunzip(d.Body.Data)
	if err != nil {
		return nil
	}
	s := &dashSnap{Key: key, Store: d.Store, Kind: d.Kind, Params: d.Params, FP: d.FP, Day: d.Day, Body: body,
		GeneratedAt: d.GeneratedAt, BuildMs: d.BuildMs, loc: storeLocation(loadStoreRaw(d.Store))}
	dashMu.Lock()
	if cur := dashMem[key]; cur != nil {
		s = cur
	} else {
		dashMem[key] = s
	}
	dashMu.Unlock()
	return s
}

func touchUsed(st *dashStore, key string, params url.Values) {
	now := time.Now()
	st.mu.Lock()
	st.used[key] = now
	st.params[key] = params
	save := now.Sub(st.usedSaved[key]) > time.Hour
	if save {
		st.usedSaved[key] = now
	}
	st.mu.Unlock()
	if save && !mainDBUnavailable() {
		go func() {
			ctx, cancel := dbctx()
			defer cancel()
			_, _ = mainDB().Collection(collDashSnapshots).UpdateOne(ctx, bson.M{"_id": key}, bson.M{"$set": bson.M{"used_at": now}})
		}()
	}
}

// mainDBUnavailable is replaceable in tests (no database).
var mainDBUnavailable = func() bool { return false }

// ---- serving ----

// snapshotted serves kind from its snapshot (see the top of this file).
func snapshotted(k *dashKind) func(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	return func(c *Ctx, w http.ResponseWriter, r *http.Request) error {
		q := r.URL.Query()
		storeHex := q.Get("storeId")
		if storeHex == "" {
			return errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
		}
		if c.store(storeHex) == nil {
			return errNotFound()
		}
		if !c.Admin && !c.can("reports", "view") {
			return errForbidden("")
		}
		// bad parameters answer straight from the endpoint (its own validation)
		if err := validateDashParams(k, q); err != nil {
			return err
		}
		st := storeState(storeHex)
		key, params, kq := snapKey(storeHex, k, q)
		touchUsed(st, key, kq)
		ensureFP(st, SnapshotVerifyAfter)
		dashMu.Lock()
		s := dashMem[key]
		dashMu.Unlock()
		if s == nil {
			s = loadSnap(key)
		}
		force := q.Get("fresh") == "1" || strings.EqualFold(q.Get("fresh"), "true")
		if !force && s.fresh(st, k) {
			return writeSnap(w, r, s, false)
		}
		f := rebuild(st, k, key, params, kq)
		var timeout <-chan time.Time
		if s != nil && !force {
			timeout = time.After(SnapshotWait)
		}
		select {
		case <-f.done:
			if f.err == nil && f.snap != nil {
				return writeSnap(w, r, f.snap, false)
			}
			if s != nil {
				return writeSnap(w, r, s, true)
			}
			if ae, ok := f.err.(*APIError); ok {
				return ae
			}
			return errInternal("Unable to calculate the dashboard.")
		case <-timeout:
			return writeSnap(w, r, s, true)
		}
	}
}

// validateDashParams checks the parameters every figure endpoint shares.
func validateDashParams(k *dashKind, q url.Values) error {
	for _, p := range k.Params {
		if (p == "from" || p == "to") && q.Get(p) != "" {
			if _, err := time.Parse(layoutDay, strings.TrimSpace(q.Get(p))); err != nil {
				return errBadRequest(p+" must be a date (YYYY-MM-DD).", map[string]string{p: "invalid"})
			}
		}
	}
	if f, t := strings.TrimSpace(q.Get("from")), strings.TrimSpace(q.Get("to")); f != "" && t != "" && t < f {
		return errBadRequest("to must not be before from.", map[string]string{"to": "before_from"})
	}
	return nil
}

// SnapshotMeta is what a response says about its snapshot.
type SnapshotMeta struct {
	GeneratedAt string `json:"generatedAt"` // store wall clock, YYYY-MM-DDTHH:mm:ss
	Stale       bool   `json:"stale"`       // being rebuilt: ask again shortly
	BuildMs     int64  `json:"buildMs"`
	Version     string `json:"version"` // send back as If-None-Match: a 304 when unchanged
}

func (s *dashSnap) version() string {
	sum := sha1.Sum(s.Body)
	return hex.EncodeToString(sum[:10])
}

func writeSnap(w http.ResponseWriter, r *http.Request, s *dashSnap, stale bool) error {
	ver := s.version()
	meta, _ := json.Marshal(SnapshotMeta{GeneratedAt: s.GeneratedAt.In(orRiyadh(s.loc)).Format("2006-01-02T15:04:05"),
		Stale: stale, BuildMs: s.BuildMs, Version: ver})
	body := make([]byte, 0, len(s.Body)+len(meta)+16)
	body = append(body, `{"snapshot":`...)
	body = append(body, meta...)
	if len(s.Body) > 2 {
		body = append(body, ',')
	}
	body = append(body, s.Body[1:]...)
	etag := `"` + ver + `"`
	w.Header().Set("ETag", etag)
	// never kept by the browser: the server's snapshot is the cache (a client that
	// keeps its own copy may send If-None-Match and get a 304)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Vary", "Authorization, Accept-Encoding")
	// a stale answer always goes out in full: the client must learn to ask again
	if inm := r.Header.Get("If-None-Match"); !stale && inm != "" && strings.Contains(inm, etag) {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	if len(body) > 2048 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && w.Header().Get("Content-Encoding") == "" {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(gz(body))
		return nil
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return nil
}

// ---- schedule ----

// StartDashboardSnapshots starts the background side: write events rebuild
// snapshots, and the scheduled check runs every SnapshotCheckEvery. Snapshots
// read in the last SnapshotActiveFor before a restart are rebuilt right away.
func StartDashboardSnapshots() {
	if !startDashboardEvents() {
		return
	}
	go func() {
		seedActive()
		t := time.NewTicker(SnapshotCheckEvery)
		defer t.Stop()
		checkAll()
		for range t.C {
			checkAll()
		}
	}()
}

// startDashboardEvents makes write events rebuild snapshots (false when already on).
func startDashboardEvents() bool {
	dashMu.Lock()
	defer dashMu.Unlock()
	if dashStarted {
		return false
	}
	dashStarted = true
	models.OnDashboardDirty(DashboardChanged)
	return true
}

// seedActive picks up the snapshots in use before a restart.
func seedActive() {
	defer func() { _ = recover() }()
	ensureSnapIndexes()
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := mainDB().Collection(collDashSnapshots).Find(ctx,
		bson.M{"used_at": bson.M{"$gte": time.Now().Add(-SnapshotActiveFor)}},
		options.Find().SetProjection(bson.M{"store": 1, "kind": 1, "params": 1, "used_at": 1}))
	if err != nil {
		return
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var d struct {
			ID     string    `bson:"_id"`
			Store  string    `bson:"store"`
			Kind   string    `bson:"kind"`
			Params string    `bson:"params"`
			UsedAt time.Time `bson:"used_at"`
		}
		if cur.Decode(&d) != nil || kindOfKey(d.ID) == nil {
			continue
		}
		q, _ := url.ParseQuery(d.Params)
		st := storeState(d.Store)
		st.mu.Lock()
		st.used[d.ID] = d.UsedAt
		st.params[d.ID] = q
		st.usedSaved[d.ID] = d.UsedAt
		st.mu.Unlock()
	}
}

// checkAll is the scheduled check: every store with snapshots in use.
func checkAll() {
	defer func() {
		if p := recover(); p != nil {
			log.Printf("[dashboard] check: %v", p)
		}
	}()
	dashMu.Lock()
	stores := make([]*dashStore, 0, len(dashStores))
	for _, st := range dashStores {
		stores = append(stores, st)
	}
	// forget snapshots nobody reads any more (memory)
	for key, s := range dashMem {
		st := dashStores[s.Store]
		if st == nil {
			delete(dashMem, key)
			continue
		}
		st.mu.Lock()
		at, ok := st.used[key]
		st.mu.Unlock()
		if !ok || time.Since(at) > SnapshotActiveFor {
			delete(dashMem, key)
		}
	}
	dashMu.Unlock()
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, st := range stores {
		st.mu.Lock()
		active := len(st.used) > 0
		st.mu.Unlock()
		if !active {
			continue
		}
		wg.Add(1)
		go func(st *dashStore) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			refreshStore(st)
		}(st)
	}
	wg.Wait()
}

// resetDashboardSnapshots forgets everything (tests).
func resetDashboardSnapshots() {
	dashMu.Lock()
	defer dashMu.Unlock()
	dashStores = map[string]*dashStore{}
	dashMem = map[string]*dashSnap{}
	dashFlights = map[string]*dashFlight{}
}
