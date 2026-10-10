package erp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Card Bridge: a small program the store installs on the shop's computer
// (Windows, macOS, Linux; cardbridge/ in this repo) for card machines that
// are reached over the shop's network, USB or a serial cable (ECR) instead
// of the provider's cloud. Browsers cannot open those connections, so the
// bridge keeps an outgoing connection to StartERP: it long-polls for jobs
// (send an amount, cancel, check) and posts the machine's answer back. No
// port is opened on the shop's network.
//
// Pairing: Settings → Card machines makes a one-time code (15 minutes); the
// bridge exchanges it for a token that only lets it fetch and answer jobs of
// that store.
//
//	POST   /card-bridges/pairing-code {storeId}       a one-time pairing code (settings edit)
//	GET    /card-bridges?storeId=                     the store's paired bridges
//	DELETE /card-bridges/{id}?storeId=                unpair (the token stops working)
//	GET    /card-bridge/downloads                     setup files (public)
//	GET    /card-bridge/download/{file}               one setup file (public)
//	POST   /card-bridge/pair {code,name,os,...}       bridge → token
//	POST   /card-bridge/hello {name,os,drivers,...}   bridge heartbeat / details
//	GET    /card-bridge/jobs?wait=20                  bridge long-poll for its next job
//	POST   /card-bridge/jobs/{id}/result              bridge → the machine's answer

const (
	bridgePairingColl = "erp_card_bridge_pairing" // main DB
	bridgeTokensColl  = "erp_card_bridge_token"   // main DB
	bridgesColl       = "erp_card_bridge"         // store DB
	bridgeJobsColl    = "erp_card_bridge_job"     // store DB

	bridgeTokenHeader  = "X-Card-Bridge-Token"
	bridgePairingTTL   = 15 * time.Minute
	maxBridgesPerStore = 20
)

// bridgeOnlineWindow: a bridge that polled within this window is online
// (it polls every ~20 s while idle).
var bridgeOnlineWindow = 60 * time.Second

// bridgePollEvery: how often a waiting long-poll looks for a new job.
var bridgePollEvery = 400 * time.Millisecond

var bridgeCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func bridgePairingOf() *mongo.Collection { return mainDB().Collection(bridgePairingColl) }
func bridgeTokensOf() *mongo.Collection  { return mainDB().Collection(bridgeTokensColl) }
func bridgesOf(storeHex string) *mongo.Collection {
	return storeDB(storeHex).Collection(bridgesColl)
}
func bridgeJobsOf(storeHex string) *mongo.Collection {
	return storeDB(storeHex).Collection(bridgeJobsColl)
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// newPairingCode: 8 characters without look-alikes, shown as XXXX-XXXX.
func newPairingCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 0, 9)
	for i, x := range b {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, bridgeCodeAlphabet[int(x)%len(bridgeCodeAlphabet)])
	}
	return string(out)
}

// normPairingCode accepts the code typed with spaces, dashes or lower case.
func normPairingCode(s string) string {
	s = strings.ToUpper(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.TrimSpace(s)))
	if len(s) != 8 {
		return ""
	}
	for _, r := range s {
		if !strings.ContainsRune(bridgeCodeAlphabet, r) {
			return ""
		}
	}
	return s[:4] + "-" + s[4:]
}

func newBridgeToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "cb1_" + base64.RawURLEncoding.EncodeToString(b)
}

func storeIDOf(s M) string {
	if v := hexOf(s["_id"]); v != "" {
		return v
	}
	return str(s["id"])
}

// bridgeOnline: the bridge polled recently.
func bridgeOnline(d M) bool {
	last := int64(num(d["lastSeenMs"]))
	return last > 0 && nowFn().UnixMilli()-last <= bridgeOnlineWindow.Milliseconds()
}

func bridgeOut(d M, loc *time.Location) M {
	last := ""
	if ms := int64(num(d["lastSeenMs"])); ms > 0 {
		last = time.UnixMilli(ms).In(loc).Format(layoutDT)
	}
	drivers := []string{}
	for _, x := range arr(d["drivers"]) {
		drivers = append(drivers, str(x))
	}
	return M{
		"id": hexOf(d["_id"]), "name": str(d["name"]), "os": str(d["os"]), "arch": str(d["arch"]),
		"version": str(d["version"]), "drivers": drivers, "online": bridgeOnline(d), "lastSeen": last,
		"pairedAt": str(d["pairedAt"]), "pairedBy": str(d["pairedBy"]),
	}
}

func loadBridge(storeHex, id string) (M, error) {
	oid, ok := oidOf(id)
	if !ok {
		return nil, errNotFound()
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := bridgesOf(storeHex).FindOne(ctx, bson.M{"_id": oid, "revoked": bson.M{"$ne": true}}).Decode(&raw); err != nil {
		return nil, errNotFound()
	}
	return normDoc(raw), nil
}

// bridgeTerminalFields validates a Card Bridge machine's bridge and driver.
func bridgeTerminalFields(storeHex string, p *terminalProviderSpec, body M, prev M, set bson.M, errs map[string]string) {
	if v, ok := body["bridgeId"]; ok || prev == nil {
		id := strings.TrimSpace(str(v))
		if id == "" {
			errs["bridgeId"] = "pair a computer with the Card Bridge first, then choose it"
		} else if _, err := loadBridge(storeHex, id); err != nil {
			errs["bridgeId"] = "pair this computer's Card Bridge first"
		}
		set["bridgeId"] = id
	}
	if v, ok := body["driver"]; ok || prev == nil {
		d := strings.TrimSpace(str(v))
		if d == "" {
			d = p.BridgeDrivers[0]
		}
		found := false
		for _, x := range p.BridgeDrivers {
			found = found || x == d
		}
		if !found {
			errs["driver"] = "one of " + strings.Join(p.BridgeDrivers, ", ")
		}
		set["driver"] = d
	}
}

// terminalDriver: the machine's Card Bridge driver (the provider's first one
// when none was picked).
func terminalDriver(p *terminalProviderSpec, d M) string {
	if v := str(d["driver"]); v != "" {
		return v
	}
	if len(p.BridgeDrivers) > 0 {
		return p.BridgeDrivers[0]
	}
	return ""
}

// ---------------------------------------------------------------- store side

// POST /card-bridges/pairing-code {storeId}
func handleBridgePairingCode(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := storeIDParam(r, body)
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canManageTerminals(c) {
		return errForbidden("Only users who can edit the store's settings can pair a Card Bridge.")
	}
	ctx, cancel := dbctx()
	defer cancel()
	n, _ := bridgesOf(storeHex).CountDocuments(ctx, bson.M{"revoked": bson.M{"$ne": true}})
	if n >= maxBridgesPerStore {
		return errConflict("too_many_bridges", "A store can pair at most 20 computers. Unpair one first.")
	}
	// one live code per store: a new code replaces the previous one
	_, _ = bridgePairingOf().DeleteMany(ctx, bson.M{"storeId": storeHex})
	code := newPairingCode()
	exp := nowFn().Add(bridgePairingTTL)
	if _, err := bridgePairingOf().InsertOne(ctx, bson.M{
		"_id": sha256Hex(code), "storeId": storeHex, "expiresMs": exp.UnixMilli(), "createdBy": c.UserName,
	}); err != nil {
		return errInternal("Unable to make a pairing code.")
	}
	writeJSON(w, http.StatusCreated, M{"code": code, "expiresAt": exp.In(storeLocation(store)).Format(layoutDT),
		"expiresInSeconds": int(bridgePairingTTL.Seconds()), "server": publicAPIBase()})
	return nil
}

// GET /card-bridges?storeId=
func handleListBridges(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	store, err := terminalStore(c, storeHex)
	if err != nil {
		return err
	}
	if !canSeeTerminals(c) {
		return errForbidden("")
	}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := bridgesOf(storeHex).Find(ctx, bson.M{"revoked": bson.M{"$ne": true}},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 1}}).SetLimit(maxBridgesPerStore))
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	loc := storeLocation(store)
	items := []M{}
	for cur.Next(ctx) {
		var raw bson.M
		if cur.Decode(&raw) == nil {
			items = append(items, bridgeOut(normDoc(raw), loc))
		}
	}
	writeJSON(w, http.StatusOK, M{"items": items, "total": len(items)})
	return nil
}

// DELETE /card-bridges/{id}?storeId=
func handleDeleteBridge(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	if _, err := terminalStore(c, storeHex); err != nil {
		return err
	}
	if !canManageTerminals(c) {
		return errForbidden("")
	}
	d, err := loadBridge(storeHex, mux.Vars(r)["id"])
	if err != nil {
		return err
	}
	ctx, cancel := dbctx()
	defer cancel()
	id := hexOf(d["_id"])
	_, _ = bridgeTokensOf().DeleteMany(ctx, bson.M{"storeId": storeHex, "bridgeId": id})
	if _, err := bridgesOf(storeHex).UpdateOne(ctx, bson.M{"_id": d["_id"]}, bson.M{"$set": bson.M{
		"revoked": true, "revokedAt": nowFn().UTC().Format(time.RFC3339), "revokedBy": c.UserName,
	}}); err != nil {
		return errInternal("Unable to unpair the computer.")
	}
	// jobs still queued for it can never run
	_, _ = bridgeJobsOf(storeHex).UpdateMany(ctx, bson.M{"bridgeId": id, "status": "queued"}, bson.M{"$set": bson.M{
		"status": "done", "doneMs": nowFn().UnixMilli(), "result": M{"status": tpFailed, "message": "The Card Bridge was unpaired."},
	}})
	writeJSON(w, http.StatusOK, M{"id": id, "deleted": true})
	return nil
}

// ---------------------------------------------------------------- downloads

// cardBridgeDist: the folder with the built setup files (the deploy builds
// them with cardbridge/build.sh next to the API binary). CARD_BRIDGE_DIST
// overrides it.
func cardBridgeDist() string {
	if v := strings.TrimSpace(os.Getenv("CARD_BRIDGE_DIST")); v != "" {
		return v
	}
	if _, err := os.Stat("cardbridge-dist"); err == nil {
		return "cardbridge-dist"
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "cardbridge-dist")
	}
	return "cardbridge-dist"
}

var reBridgeFile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,120}$`)

// GET /card-bridge/downloads → manifest.json written by cardbridge/build.sh
func handleBridgeDownloads(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(filepath.Join(cardBridgeDist(), "manifest.json"))
	if err != nil {
		writeJSON(w, http.StatusOK, M{"version": "", "files": []M{}, "available": false})
		return
	}
	var m M
	if json.Unmarshal(b, &m) != nil {
		writeJSON(w, http.StatusOK, M{"version": "", "files": []M{}, "available": false})
		return
	}
	m["available"] = len(arr(m["files"])) > 0
	writeJSON(w, http.StatusOK, m)
}

// GET /card-bridge/download/{file}
func handleBridgeDownload(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["file"]
	if !reBridgeFile.MatchString(name) || name == "manifest.json" || strings.Contains(name, "..") {
		writeErr(w, errNotFound())
		return
	}
	p := filepath.Join(cardBridgeDist(), name)
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		writeErr(w, errNotFound())
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, p)
}

// publicAPIBase: the API's public URL, given to the bridge when it pairs.
func publicAPIBase() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("API_PUBLIC_URL")), "/")
}

// ---------------------------------------------------------------- bridge side

type bridgeAuth struct {
	StoreHex string
	BridgeID string
	Bridge   M
}

// bridgeFromRequest checks the bridge token.
func bridgeFromRequest(r *http.Request) (*bridgeAuth, error) {
	tok := strings.TrimSpace(r.Header.Get(bridgeTokenHeader))
	if !strings.HasPrefix(tok, "cb1_") || len(tok) > 100 {
		return nil, errUnauthorized("This Card Bridge is not paired. Pair it again from Settings → Card machines.")
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := bridgeTokensOf().FindOne(ctx, bson.M{"_id": sha256Hex(tok)}).Decode(&raw); err != nil {
		return nil, errUnauthorized("This Card Bridge was unpaired. Pair it again from Settings → Card machines.")
	}
	t := normDoc(raw)
	b, err := loadBridge(str(t["storeId"]), str(t["bridgeId"]))
	if err != nil {
		return nil, errUnauthorized("This Card Bridge was unpaired. Pair it again from Settings → Card machines.")
	}
	return &bridgeAuth{StoreHex: str(t["storeId"]), BridgeID: str(t["bridgeId"]), Bridge: b}, nil
}

func bridgeHandler(fn func(a *bridgeAuth, w http.ResponseWriter, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := bridgeFromRequest(r)
		if err != nil {
			writeErr(w, err)
			return
		}
		if err := fn(a, w, r); err != nil {
			writeErr(w, err)
		}
	}
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > n {
		return string([]rune(s)[:n])
	}
	return s
}

// bridgeDetails: what a bridge says about itself (pair / hello).
func bridgeDetails(body M) bson.M {
	set := bson.M{}
	for k, n := range map[string]int{"name": 60, "os": 20, "arch": 20, "version": 30} {
		if v := clip(str(body[k]), n); v != "" {
			set[k] = v
		}
	}
	if v, ok := body["drivers"]; ok {
		ds := []string{}
		for _, x := range arr(v) {
			if d := clip(str(x), 40); d != "" && len(ds) < 30 {
				ds = append(ds, d)
			}
		}
		set["drivers"] = ds
	}
	return set
}

// POST /card-bridge/pair {code, name, os, arch, version, drivers}
func handleBridgePair(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	code := normPairingCode(str(body["code"]))
	if code == "" {
		writeErr(w, errBadRequest("Type the 8-character pairing code shown in Settings → Card machines.", map[string]string{"code": "8 letters and digits"}))
		return
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	// single use: the code is removed as it is read
	if err := bridgePairingOf().FindOneAndDelete(ctx, bson.M{"_id": sha256Hex(code)}).Decode(&raw); err != nil {
		writeErr(w, errf(http.StatusBadRequest, "pairing_code", "This pairing code is wrong or was already used. Make a new one in Settings → Card machines.", map[string]string{"code": "wrong or used"}))
		return
	}
	pc := normDoc(raw)
	if nowFn().UnixMilli() > int64(num(pc["expiresMs"])) {
		writeErr(w, errf(http.StatusBadRequest, "pairing_code", "This pairing code has expired. Make a new one in Settings → Card machines.", map[string]string{"code": "expired"}))
		return
	}
	storeHex := str(pc["storeId"])
	store := loadStoreDoc(storeHex)
	loc := riyadh
	if store != nil {
		loc = storeLocation(store)
	}
	set := bridgeDetails(body)
	if str(set["name"]) == "" {
		set["name"] = "Shop computer"
	}
	oid := primitive.NewObjectID()
	set["_id"] = oid
	set["pairedAt"] = nowFn().In(loc).Format(layoutDT)
	set["pairedBy"] = str(pc["createdBy"])
	set["lastSeenMs"] = nowFn().UnixMilli()
	if _, err := bridgesOf(storeHex).InsertOne(ctx, set); err != nil {
		writeErr(w, errInternal("Unable to pair the Card Bridge."))
		return
	}
	tok := newBridgeToken()
	if _, err := bridgeTokensOf().InsertOne(ctx, bson.M{"_id": sha256Hex(tok), "storeId": storeHex, "bridgeId": oid.Hex(),
		"createdAt": nowFn().UTC().Format(time.RFC3339)}); err != nil {
		writeErr(w, errInternal("Unable to pair the Card Bridge."))
		return
	}
	storeName := ""
	if s := loadStoreName(storeHex); s != "" {
		storeName = s
	}
	writeJSON(w, http.StatusCreated, M{"token": tok, "bridgeId": oid.Hex(), "storeId": storeHex, "storeName": storeName})
}

func loadStoreName(storeHex string) string {
	oid, ok := oidOf(storeHex)
	if !ok {
		return ""
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid},
		options.FindOne().SetProjection(bson.M{"name": 1, "branch_name": 1})).Decode(&raw); err != nil {
		return ""
	}
	d := normDoc(raw)
	n := str(d["name"])
	if b := str(d["branch_name"]); b != "" {
		n += " · " + b
	}
	return n
}

// POST /card-bridge/hello {name, os, arch, version, drivers}
func handleBridgeHello(a *bridgeAuth, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	set := bridgeDetails(body)
	set["lastSeenMs"] = nowFn().UnixMilli()
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = bridgesOf(a.StoreHex).UpdateOne(ctx, bson.M{"_id": a.Bridge["_id"]}, bson.M{"$set": set})
	writeJSON(w, http.StatusOK, M{"ok": true, "bridgeId": a.BridgeID, "storeId": a.StoreHex, "storeName": loadStoreName(a.StoreHex)})
	return nil
}

// bridgeWake lets a new job end a waiting long-poll at once.
var bridgeWake = struct {
	sync.Mutex
	ch map[string]chan struct{}
}{ch: map[string]chan struct{}{}}

func bridgeWaitChan(bridgeID string) chan struct{} {
	bridgeWake.Lock()
	defer bridgeWake.Unlock()
	c := bridgeWake.ch[bridgeID]
	if c == nil {
		c = make(chan struct{})
		bridgeWake.ch[bridgeID] = c
	}
	return c
}

func bridgeNotify(bridgeID string) {
	bridgeWake.Lock()
	defer bridgeWake.Unlock()
	if c := bridgeWake.ch[bridgeID]; c != nil {
		close(c)
		delete(bridgeWake.ch, bridgeID)
	}
}

// GET /card-bridge/jobs?wait=20 → 200 {job} or 204 when nothing came in time
func handleBridgeJobs(a *bridgeAuth, w http.ResponseWriter, r *http.Request) error {
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait < 0 {
		wait = 0
	}
	if wait > 25 {
		wait = 25
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for {
		touchBridge(a)
		job, err := takeBridgeJob(a)
		if err != nil {
			return err
		}
		if job != nil {
			writeJSON(w, http.StatusOK, M{"job": job})
			return nil
		}
		if !time.Now().Before(deadline) {
			w.WriteHeader(http.StatusNoContent)
			return nil
		}
		t := time.NewTimer(bridgePollEvery)
		select {
		case <-r.Context().Done():
			t.Stop()
			return nil
		case <-bridgeWaitChan(a.BridgeID):
			t.Stop()
		case <-t.C:
		}
	}
}

func touchBridge(a *bridgeAuth) {
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = bridgesOf(a.StoreHex).UpdateOne(ctx, bson.M{"_id": a.Bridge["_id"]}, bson.M{"$set": bson.M{"lastSeenMs": nowFn().UnixMilli()}})
}

// takeBridgeJob hands the oldest queued job to the bridge (expiring jobs
// nobody fetched in time) together with the machine's settings.
func takeBridgeJob(a *bridgeAuth) (M, error) {
	ctx, cancel := dbctx()
	defer cancel()
	col := bridgeJobsOf(a.StoreHex)
	stale := nowFn().Add(-(terminalTimeout + 30*time.Second)).UnixMilli()
	_, _ = col.UpdateMany(ctx, bson.M{"bridgeId": a.BridgeID, "status": "queued", "createdMs": bson.M{"$lt": stale}}, bson.M{"$set": bson.M{
		"status": "done", "doneMs": nowFn().UnixMilli(), "result": M{"status": tpTimeout, "message": "The Card Bridge did not pick up the payment in time."},
	}})
	var raw bson.M
	err := col.FindOneAndUpdate(ctx, bson.M{"bridgeId": a.BridgeID, "status": "queued"},
		bson.M{"$set": bson.M{"status": "taken", "takenMs": nowFn().UnixMilli()}},
		options.FindOneAndUpdate().SetSort(bson.D{{Key: "createdMs", Value: 1}}).SetReturnDocument(options.After)).Decode(&raw)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	j := normDoc(raw)
	out := M{
		"id": hexOf(j["_id"]), "op": str(j["op"]), "driver": str(j["driver"]), "terminalId": str(j["terminalId"]),
		"paymentId": str(j["paymentId"]), "ofJob": str(j["ofJob"]), "amount": num(j["amount"]), "minor": int64(num(j["minor"])),
		"currency": str(j["currency"]), "decimals": int(num(j["decimals"])), "reference": str(j["reference"]),
		"mode": str(j["mode"]), "timeoutSeconds": int(terminalTimeout.Seconds()), "config": M{},
	}
	// the machine's settings (opened secrets) go to the bridge only, never into the job record
	if td, err := loadTerminalAny(a.StoreHex, str(j["terminalId"])); err == nil {
		if p := terminalProviders[str(td["provider"])]; p != nil {
			cfg := M{}
			for k, v := range openFields(p.StoreFields, sub(td, "fields")) {
				cfg[k] = v
			}
			out["config"] = cfg
			out["terminalName"] = str(td["name"])
			out["provider"] = p.ID
		}
	}
	return out, nil
}

// POST /card-bridge/jobs/{id}/result {status, authCode, rrn, maskedPan, scheme, message, providerRef, ok}
func handleBridgeJobResult(a *bridgeAuth, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	oid, ok := oidOf(mux.Vars(r)["id"])
	if !ok {
		return errNotFound()
	}
	res := M{}
	for k, n := range map[string]int{"status": 20, "authCode": 20, "rrn": 30, "maskedPan": 30, "scheme": 20, "message": 300, "providerRef": 80} {
		if v := clip(str(body[k]), n); v != "" {
			res[k] = v
		}
	}
	if v, isB := body["ok"].(bool); isB {
		res["ok"] = v
	}
	st := str(res["status"])
	if st != "" && !tpFinal[st] && st != tpPending {
		return errBadRequest("", map[string]string{"status": "approved, declined, cancelled, failed, timeout or pending"})
	}
	ctx, cancel := dbctx()
	defer cancel()
	col := bridgeJobsOf(a.StoreHex)
	var raw bson.M
	if err := col.FindOne(ctx, bson.M{"_id": oid, "bridgeId": a.BridgeID}).Decode(&raw); err != nil {
		return errNotFound()
	}
	j := normDoc(raw)
	if st == tpPending {
		// progress only ("insert card…"): keep the job open
		_, _ = col.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"progress": res["message"]}})
		writeJSON(w, http.StatusOK, M{"ok": true})
		return nil
	}
	if str(j["status"]) == "done" {
		writeJSON(w, http.StatusOK, M{"ok": true, "duplicate": true})
		return nil
	}
	if _, err := col.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"status": "done", "doneMs": nowFn().UnixMilli(), "result": res}}); err != nil {
		return errInternal("db: " + err.Error())
	}
	if str(j["op"]) == "pay" && st != "" {
		storeBridgePayResult(a.StoreHex, j, res)
	}
	writeJSON(w, http.StatusOK, M{"ok": true})
	return nil
}

// storeBridgePayResult writes the machine's answer onto the payment. An
// approval that arrives after the till gave up (cancelled / timed out) is
// kept on the payment as lateApproval so the store can refund it.
func storeBridgePayResult(storeHex string, j M, res M) {
	pid, ok := oidOf(str(j["paymentId"]))
	if !ok {
		return
	}
	loc := riyadh
	if s := loadStoreDoc(storeHex); s != nil {
		loc = storeLocation(s)
	}
	tr := bridgeResult(res)
	out, err := applyTerminalResult(storeHex, loc, pid, tr)
	if err == nil && tr.Status == tpApproved && str(out["status"]) != tpApproved {
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = terminalPaymentsOf(storeHex).UpdateOne(ctx, bson.M{"_id": pid}, bson.M{"$set": bson.M{"lateApproval": M{
			"authCode": tr.AuthCode, "rrn": tr.RRN, "maskedPan": tr.MaskedPan, "scheme": tr.Scheme,
			"at": nowFn().In(loc).Format(layoutDT),
		}}})
	}
}

func bridgeResult(res M) terminalResult {
	return terminalResult{Status: str(res["status"]), ProviderRef: str(res["providerRef"]), AuthCode: str(res["authCode"]),
		RRN: str(res["rrn"]), MaskedPan: str(res["maskedPan"]), Scheme: str(res["scheme"]), Message: str(res["message"])}
}

// ---------------------------------------------------------------- adapter

// bridgeAdapter sends payments to a machine through the store's Card Bridge.
type bridgeAdapter struct{}

func (bridgeAdapter) bridge(cfg terminalCfg) (M, error) {
	if cfg.BridgeID == "" {
		return nil, errors.New("choose the shop computer (Card Bridge) this card machine is connected to in Settings → Card machines")
	}
	b, err := loadBridge(cfg.StoreHex, cfg.BridgeID)
	if err != nil {
		return nil, errors.New("the Card Bridge for this card machine was unpaired; pair the shop computer again in Settings → Card machines")
	}
	if !bridgeOnline(b) {
		return nil, errors.New("the Card Bridge on " + str(b["name"]) + " is offline; check that the computer is on and connected to the internet")
	}
	return b, nil
}

func (a bridgeAdapter) enqueue(cfg terminalCfg, op string, extra bson.M) (primitive.ObjectID, error) {
	oid := primitive.NewObjectID()
	doc := bson.M{"_id": oid, "bridgeId": cfg.BridgeID, "terminalId": cfg.TerminalID, "op": op, "driver": cfg.Driver,
		"mode": cfg.Env, "status": "queued", "createdMs": nowFn().UnixMilli()}
	for k, v := range extra {
		doc[k] = v
	}
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := bridgeJobsOf(cfg.StoreHex).InsertOne(ctx, doc); err != nil {
		return oid, err
	}
	bridgeNotify(cfg.BridgeID)
	return oid, nil
}

func loadBridgeJob(storeHex string, oid primitive.ObjectID) (M, error) {
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := bridgeJobsOf(storeHex).FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil {
		return nil, err
	}
	return normDoc(raw), nil
}

// waitBridgeJob waits for a job's answer (check / cancel).
func waitBridgeJob(ctx context.Context, storeHex string, oid primitive.ObjectID, max time.Duration) (M, bool) {
	end := time.Now().Add(max)
	for time.Now().Before(end) {
		if j, err := loadBridgeJob(storeHex, oid); err == nil && str(j["status"]) == "done" {
			return j, true
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(bridgePollEvery):
		}
	}
	return nil, false
}

var bridgeCheckWait = 15 * time.Second

func (a bridgeAdapter) Check(ctx context.Context, cfg terminalCfg) error {
	if _, err := a.bridge(cfg); err != nil {
		return err
	}
	oid, err := a.enqueue(cfg, "check", nil)
	if err != nil {
		return err
	}
	j, ok := waitBridgeJob(ctx, cfg.StoreHex, oid, bridgeCheckWait)
	if !ok {
		ctx2, cancel := dbctx()
		defer cancel()
		_, _ = bridgeJobsOf(cfg.StoreHex).UpdateOne(ctx2, bson.M{"_id": oid, "status": "queued"}, bson.M{"$set": bson.M{"status": "done", "doneMs": nowFn().UnixMilli()}})
		return errors.New("the Card Bridge did not answer; check that it is running on the shop computer")
	}
	res := sub(j, "result")
	if v, isB := res["ok"].(bool); isB && v {
		return nil
	}
	m := str(res["message"])
	if m == "" {
		m = "the card machine did not answer the Card Bridge"
	}
	return errors.New(m)
}

func (a bridgeAdapter) Start(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	b, err := a.bridge(cfg)
	if err != nil {
		return terminalResult{Status: tpFailed, Message: strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + "."}, nil
	}
	oid, err := a.enqueue(cfg, "pay", bson.M{"paymentId": req.PaymentID, "amount": req.Amount, "minor": req.minorUnits(),
		"currency": req.Currency, "decimals": req.Decimals, "reference": req.Reference})
	if err != nil {
		return terminalResult{}, err
	}
	return terminalResult{Status: tpPending, ProviderRef: "bridge_" + oid.Hex(), Message: "Sent to the card machine through the Card Bridge on " + str(b["name"]) + "."}, nil
}

func bridgeJobRef(ref string) (primitive.ObjectID, bool) {
	return oidOf(strings.TrimPrefix(ref, "bridge_"))
}

func (a bridgeAdapter) Status(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	oid, ok := bridgeJobRef(ref)
	if !ok {
		return terminalResult{}, errors.New("unknown Card Bridge job")
	}
	j, err := loadBridgeJob(cfg.StoreHex, oid)
	if err != nil {
		return terminalResult{}, errors.New("the Card Bridge job is missing")
	}
	if str(j["status"]) == "done" {
		r := bridgeResult(sub(j, "result"))
		if r.Status == "" {
			r.Status = tpFailed
		}
		return r, nil
	}
	msg := "Waiting for the card on the machine."
	if str(j["status"]) == "queued" {
		msg = "Waiting for the Card Bridge to pick up the payment."
	}
	if p := str(j["progress"]); p != "" {
		msg = p
	}
	return terminalResult{Status: tpPending, Message: msg}, nil
}

var bridgeCancelWait = 10 * time.Second

func (a bridgeAdapter) Cancel(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	oid, ok := bridgeJobRef(ref)
	if !ok {
		return terminalResult{Status: tpCancelled}, nil
	}
	dctx, cancel := dbctx()
	defer cancel()
	col := bridgeJobsOf(cfg.StoreHex)
	// not picked up yet: it never reaches the machine
	res := M{"status": tpCancelled, "message": "Cancelled on the till."}
	if u, err := col.UpdateOne(dctx, bson.M{"_id": oid, "status": "queued"}, bson.M{"$set": bson.M{"status": "done", "doneMs": nowFn().UnixMilli(), "result": res}}); err == nil && u.ModifiedCount == 1 {
		return terminalResult{Status: tpCancelled, Message: "Cancelled on the till."}, nil
	}
	j, err := loadBridgeJob(cfg.StoreHex, oid)
	if err != nil {
		return terminalResult{Status: tpCancelled}, nil
	}
	if str(j["status"]) == "done" {
		return bridgeResult(sub(j, "result")), nil
	}
	_, _ = col.UpdateOne(dctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"cancelRequested": true}})
	if _, err := a.enqueue(cfg, "cancel", bson.M{"ofJob": oid.Hex(), "paymentId": req.PaymentID}); err != nil {
		return terminalResult{}, err
	}
	// the bridge answers the payment job itself once the machine stopped
	if j, ok := waitBridgeJob(ctx, cfg.StoreHex, oid, bridgeCancelWait); ok {
		return bridgeResult(sub(j, "result")), nil
	}
	return terminalResult{}, errors.New("the Card Bridge has not confirmed the cancel yet")
}

func registerCardBridge(s *mux.Router) {
	s.HandleFunc("/card-bridges/pairing-code", authed(handleBridgePairingCode)).Methods("POST")
	s.HandleFunc("/card-bridges", authed(handleListBridges)).Methods("GET")
	s.HandleFunc("/card-bridges/{id}", authed(handleDeleteBridge)).Methods("DELETE")
	s.HandleFunc("/card-bridge/downloads", handleBridgeDownloads).Methods("GET")
	s.HandleFunc("/card-bridge/download/{file}", handleBridgeDownload).Methods("GET")
	s.HandleFunc("/card-bridge/pair", handleBridgePair).Methods("POST")
	s.HandleFunc("/card-bridge/hello", bridgeHandler(handleBridgeHello)).Methods("POST")
	s.HandleFunc("/card-bridge/jobs", bridgeHandler(handleBridgeJobs)).Methods("GET")
	s.HandleFunc("/card-bridge/jobs/{id}/result", bridgeHandler(handleBridgeJobResult)).Methods("POST")
}
