package erp

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mapCtx gives mappers access to the caller and the target store, with small
// per-request caches for lookups (warehouses, products, …).
type mapCtx struct {
	c        *Ctx
	storeHex string
	store    M
	wh       []M
	whLoaded bool
	cache    map[string]M
}

func newMapCtx(c *Ctx, storeHex string) *mapCtx {
	x := &mapCtx{c: c, storeHex: storeHex, cache: map[string]M{}}
	if c != nil {
		x.store = c.store(storeHex)
	}
	if x.store == nil && storeHex != "" {
		x.store = loadStoreRaw(storeHex)
	}
	return x
}

func loadStoreRaw(hex string) M {
	id, ok := oidOf(hex)
	if !ok {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	if err := mainDB().Collection("store").FindOne(ctx, bson.M{"_id": id}).Decode(&raw); err != nil {
		return nil
	}
	return normDoc(raw)
}

// loc is the store's timezone (Asia/Riyadh without a store).
func (x *mapCtx) loc() *time.Location {
	if x == nil {
		return riyadh
	}
	return storeLocation(x.store)
}

func (x *mapCtx) fmtDT(v interface{}) string  { return fmtDTIn(x.loc(), v) }
func (x *mapCtx) fmtDay(v interface{}) string { return fmtDayIn(x.loc(), v) }
func (x *mapCtx) parseTime(s string) (time.Time, error) {
	return parseClientTimeIn(x.loc(), s)
}
func (x *mapCtx) legacyDateStr(s string) (string, error) {
	return toLegacyDateStrIn(x.loc(), s)
}

func (x *mapCtx) vatPercent() float64 {
	if x.store != nil {
		if v, ok := x.store["vat_percent"]; ok && num(v) > 0 {
			return num(v)
		}
	}
	return 15
}

// warehouses returns the store's legacy warehouses (non-deleted first).
func (x *mapCtx) warehouses() []M {
	if x.whLoaded || x.storeHex == "" {
		return x.wh
	}
	x.whLoaded = true
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(x.storeHex).Collection("warehouse").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		x.wh = append(x.wh, bsonToM(cur.Current))
	}
	return x.wh
}

// mainStoreWarehouseID is the id of the virtual warehouse representing the
// legacy "main_store" stock bucket (lines/stock without a warehouse).
func mainStoreWarehouseID(storeHex string) string { return "ms_" + storeHex }

func isMainStoreWarehouse(id string) bool { return strings.HasPrefix(id, "ms_") }

// whContractID maps a legacy (warehouse_id, warehouse_code) to a contract id.
func (x *mapCtx) whContractID(legacyID interface{}, code interface{}) string {
	if h := hexOf(legacyID); h != "" {
		return h
	}
	cs := str(code)
	if cs != "" && cs != "main_store" {
		for _, w := range x.warehouses() {
			if str(w["code"]) == cs {
				return hexOf(w["_id"])
			}
		}
	}
	return mainStoreWarehouseID(x.storeHex)
}

func (x *mapCtx) whCodeByID(hex string) string {
	for _, w := range x.warehouses() {
		if hexOf(w["_id"]) == hex {
			return str(w["code"])
		}
	}
	return ""
}

// legacyWarehouse maps a contract warehouse id to legacy (warehouse_id, warehouse_code).
func (x *mapCtx) legacyWarehouse(id string) (interface{}, interface{}, error) {
	if id == "" || isMainStoreWarehouse(id) {
		return nil, nil, nil
	}
	hex, err := x.ref("warehouse", id)
	if err != nil {
		return nil, nil, err
	}
	if hex == nil {
		return nil, nil, nil
	}
	code := x.whCodeByID(hex.(string))
	return hex, nilIfEmpty(code), nil
}

// ref resolves a contract reference to a legacy ObjectID hex string:
// a valid hex is used as-is; anything else is looked up through erp.cid
// (ids the client generated at create time). Unknown ids → field error.
func (x *mapCtx) ref(coll string, v interface{}) (interface{}, error) {
	s := strings.TrimSpace(str(v))
	if s == "" {
		return nil, nil
	}
	if _, ok := oidOf(s); ok {
		return s, nil
	}
	if strings.HasPrefix(s, "ms_") && coll == "warehouse" {
		return nil, nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var col *mongo.Collection
	if mainDBCollections[coll] {
		col = mainDB().Collection(coll)
	} else {
		col = storeDB(x.storeHex).Collection(coll)
	}
	var raw bson.M
	if err := col.FindOne(ctx, bson.M{"erp.cid": s}, options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&raw); err != nil {
		return nil, errBadRequest("Unknown reference "+s, map[string]string{coll: "unknown id " + s})
	}
	return hexOf(raw["_id"]), nil
}

// doc loads one legacy document by hex id from a store collection (cached).
func (x *mapCtx) doc(coll, hex string) M {
	if hex == "" {
		return nil
	}
	key := coll + "|" + hex
	if d, ok := x.cache[key]; ok {
		return d
	}
	id, ok := oidOf(hex)
	if !ok {
		return nil
	}
	ctx, cancel := dbctx()
	defer cancel()
	var col *mongo.Collection
	if mainDBCollections[coll] {
		col = mainDB().Collection(coll)
	} else {
		col = storeDB(x.storeHex).Collection(coll)
	}
	var raw bson.M
	if err := col.FindOne(ctx, bson.M{"_id": id}).Decode(&raw); err != nil {
		x.cache[key] = nil
		return nil
	}
	d := normDoc(raw)
	x.cache[key] = d
	return d
}

var mainDBCollections = map[string]bool{
	"store": true, "user": true, "customer_package": true, "rfq_received": true, "rfq_suppliers": true,
}

type toCFn func(x *mapCtx, d M) M
type toLFn func(x *mapCtx, rec M, prev M, ch map[string]bool, create bool) (M, error)
type validateFn func(x *mapCtx, rec M, prev M) map[string]string

// v1Ops are the EXISTING legacy handlers a legacy resource writes through.
type v1Ops struct {
	path    string
	create  http.HandlerFunc
	update  http.HandlerFunc
	delete  http.HandlerFunc
	restore http.HandlerFunc
	// needStore: pass search[store_id] (default true for store DBs)
	noStoreParam  bool
	storeQueryKey string // legacy handlers reading ?store_id= instead of ?search[store_id]=
}

// legacyBackend serves a contract resource from an existing collection.
type legacyBackend struct {
	coll          string
	inMain        bool // collection lives in the main DB
	mainStoreKey  string
	orgOverStores bool // org-scoped resource backed by per-store collections
	dateKey       string
	deletedKey    string // "" when the legacy model has no soft delete
	baseFilter    bson.M
	toC           toCFn
	toL           toLFn
	known         map[string]bool
	validate      validateFn
	v1            v1Ops
	fieldErr      map[string]string
	lineErr       map[string]string
	lineKey       string
	afterWrite    func(x *mapCtx, hex string, rec, prevDoc M, ch map[string]bool, create bool) error
	beforeCreate  func(x *mapCtx, rec M) error
	noDelete      string
	noRestore     string
	readOnly      bool
	sortKey       string
	searchKeys    []string                      // legacy keys ?q= matches (search.go); none = ?q= not supported
	searchSort    string                        // sort key for ?q= results (e.g. name), "" = sortKey
	mainOrg       bool                          // org-scoped resource in the main DB (stores, users)
	access        func(c *Ctx) bson.M           // extra visibility filter (mainOrg)
	hybrid        map[string]bool               // keys mapped to legacy AND preserved in erp.x (nested objects partially mapped)
	storeHint     func(x *mapCtx, rec M) string // main-DB docs: which store DB context to use
	affectsStock  bool                          // document moves stock: bump the version of every product it touches (rule 53)
	// adapterSoftDelete: the legacy delete is DESTRUCTIVE (DeleteOne), so the
	// contract soft delete is kept in erp.del (restorable); ?hard=1 calls the
	// legacy delete.
	adapterSoftDelete bool
	// replaceUpdate: the legacy update handler decodes into a FRESH struct, so
	// the full legacy document is sent back (like the old app does) with the
	// mapped changes overlaid.
	replaceUpdate bool
	noUpdate      string // legacy update path unusable: mapped-field edits → 409 with this message
	// settleLedger: the legacy handler finishes accounting in a goroutine that
	// later re-saves the in-memory document (AdjustPayments → doc.Update()).
	// Wait (bounded) for the ledger entry so a quick follow-up PATCH is not
	// overwritten by that stale save.
	settleLedger bool
}

// settleWait bounds how long writes wait for legacy async accounting.
var settleWait = 4 * time.Second

func (b *legacyBackend) settle(storeHex, hex string, since time.Time) {
	if !b.settleLedger {
		return
	}
	oid, ok := oidOf(hex)
	if !ok {
		return
	}
	deadline := time.Now().Add(settleWait)
	for time.Now().Before(deadline) {
		ctx, cancel := dbctx()
		n, err := storeDB(storeHex).Collection("ledger").CountDocuments(ctx, bson.M{"reference_id": oid, "created_at": bson.M{"$gte": since.Add(-time.Second)}})
		cancel()
		if err != nil || n > 0 {
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(60 * time.Millisecond)
	}
}

// legacyJSONRenames: bson keys whose json tag differs (replace-style updates).
var legacyJSONRenames = map[string]string{
	"order_placed": "order_placed_by", "order_placed_signature_id": "order_placed_by_signature_id",
	"purchase_returned_signature_id": "purchase_returned_by_signature_id", "registration_number_arabic": "registration_number_in_arabic",
	"return_discount_with_vat": "return_discount_vat", "net_retail_profit": "net_retail_net_profit",
}

// legacyJSONOf converts a stored legacy document into the JSON body the old
// app would send back (dates as RFC3339 in the store zone loc, ids as hex,
// *_str companions).
func legacyJSONOf(d M, loc *time.Location) M {
	loc = orRiyadh(loc)
	base := M{}
	for k, v := range d {
		if k == envKey || k == "_id" {
			continue
		}
		if nk, ok := legacyJSONRenames[k]; ok {
			k = nk
		}
		base[k] = v
	}
	out := toJSONMap(base)
	out["id"] = hexOf(d["_id"])
	for _, k := range []string{"date", "expected_date", "signature_date"} {
		if t, ok := toTime(d[k]); ok {
			out[k+"_str"] = t.In(loc).Format(time.RFC3339)
		}
	}
	if ps := arr(d["payments"]); len(ps) > 0 {
		in := []interface{}{}
		for _, p := range ps {
			pm, _ := p.(M)
			if pm == nil || boolv(pm["deleted"]) {
				continue
			}
			jp := toJSONMap(pm)
			jp["id"] = hexOf(pm["_id"])
			if t, ok := toTime(pm["date"]); ok {
				jp["date_str"] = t.In(loc).Format(time.RFC3339)
			}
			in = append(in, jp)
		}
		out["payments_input"] = in
	}
	return out
}

// touchedProducts collects product ids from contract items/parts and legacy lines.
func touchedProducts(rec, prevDoc M) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v interface{}) {
		if h := hexOf(v); h != "" && !seen[h] {
			if _, ok := oidOf(h); ok {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	for _, k := range []string{"items", "parts"} {
		for _, it := range arr(rec[k]) {
			if im, ok := it.(M); ok {
				add(im["productId"])
			}
		}
	}
	for _, k := range []string{"products", "parts"} {
		for _, it := range arr(get(prevDoc, k)) {
			if im, ok := it.(M); ok {
				add(im["product_id"])
			}
		}
	}
	return out
}

// bumpProductVersions increments the adapter version of products whose
// stock the legacy code is (re)computing, so clients re-read them.
func bumpProductVersions(storeHex string, ids []string) {
	for _, h := range ids {
		oid, _ := oidOf(h)
		ctx, cancel := dbctx()
		var raw bson.M
		if err := storeDB(storeHex).Collection("product").FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err == nil {
			d := normDoc(raw)
			set := bson.M{"erp.v": versionOf(d) + 1}
			if t, ok := toTime(d["updated_at"]); ok {
				set["erp.ts"] = t
			}
			_, _ = storeDB(storeHex).Collection("product").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": set})
		}
		cancel()
	}
}

func (b *legacyBackend) afterStock(storeHex string, rec, prevDoc M) {
	if b.affectsStock {
		bumpProductVersions(storeHex, touchedProducts(rec, prevDoc))
	}
}

func knownSet(keys ...string) map[string]bool {
	s := map[string]bool{}
	for _, k := range keys {
		s[k] = true
	}
	return s
}

func (b *legacyBackend) col(storeHex string) *mongo.Collection {
	if b.inMain {
		return mainDB().Collection(b.coll)
	}
	return storeDB(storeHex).Collection(b.coll)
}

func idFilter(id string) bson.M {
	if oid, ok := oidOf(id); ok {
		return bson.M{"$or": bson.A{bson.M{"_id": oid}, bson.M{"erp.cid": id}}}
	}
	return bson.M{"erp.cid": id}
}

func andFilter(parts ...bson.M) bson.M {
	clean := bson.A{}
	for _, p := range parts {
		if len(p) > 0 {
			clean = append(clean, p)
		}
	}
	switch len(clean) {
	case 0:
		return bson.M{}
	case 1:
		return clean[0].(bson.M)
	}
	return bson.M{"$and": clean}
}

func (b *legacyBackend) scopeFilter(c *Ctx, storeHex string) bson.M {
	f := bson.M{}
	for k, v := range b.baseFilter {
		f[k] = v
	}
	if b.access != nil && c != nil {
		for k, v := range b.access(c) {
			f[k] = v
		}
	}
	if b.inMain && b.mainStoreKey != "" && storeHex != "" {
		if oid, ok := oidOf(storeHex); ok {
			f[b.mainStoreKey] = oid
		}
	}
	return f
}

func (b *legacyBackend) isDeleted(d M) bool {
	if boolv(get(d, envKey+".del")) {
		return true
	}
	if b.deletedKey == "" {
		return false
	}
	return boolv(d[b.deletedKey])
}

func (b *legacyBackend) render(x *mapCtx, d M, storeScoped bool) M {
	rec := b.toC(x, d)
	rec["id"] = hexOf(d["_id"])
	if storeScoped {
		rec["storeId"] = x.storeHex
	} else {
		delete(rec, "storeId")
	}
	return applyEnvelopeIn(x.loc(), rec, d, b.isDeleted(d))
}

func (b *legacyBackend) storeScoped() bool { return !b.orgOverStores && !b.mainOrg }

func (b *legacyBackend) loadRaw(c *Ctx, storeHex, id string, includeHidden bool) (M, error) {
	f := andFilter(b.scopeFilter(c, storeHex), idFilter(id))
	if !includeHidden {
		f = andFilter(f, bson.M{"erp.hd": bson.M{"$ne": true}})
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	err := b.col(storeHex).FindOne(ctx, f).Decode(&raw)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	return normDoc(raw), nil
}

func (b *legacyBackend) stores(c *Ctx, storeHex string) []string {
	if b.orgOverStores || storeHex == "" {
		return c.storeHexes()
	}
	return []string{storeHex}
}

func (b *legacyBackend) List(c *Ctx, storeHex string, q ListQuery) ([]M, int64, error) {
	if err := b.checkSearch(q); err != nil {
		return nil, 0, err
	}
	if b.mainOrg {
		return b.listOne(c, "", q, false)
	}
	if b.orgOverStores {
		all := []M{}
		for _, s := range c.storeHexes() {
			rows, _, err := b.listOne(c, s, q, true)
			if err != nil {
				return nil, 0, err
			}
			all = append(all, rows...)
		}
		all = uniqueByID(all)
		total := int64(len(all))
		start := (q.Page - 1) * q.Limit
		if start > len(all) {
			start = len(all)
		}
		end := start + q.Limit
		if end > len(all) {
			end = len(all)
		}
		return all[start:end], total, nil
	}
	return b.listOne(c, storeHex, q, false)
}

// uniqueByID keeps the first row of each id: a store DB copied from another
// keeps its records' ids, and an org-wide union must list each record once.
func uniqueByID(rows []M) []M {
	seen := make(map[string]bool, len(rows))
	out := rows[:0]
	for _, r := range rows {
		if id := str(r["id"]); id != "" {
			if seen[id] {
				continue
			}
			seen[id] = true
		}
		out = append(out, r)
	}
	return out
}

func (b *legacyBackend) listOne(c *Ctx, storeHex string, q ListQuery, all bool) ([]M, int64, error) {
	f := andFilter(b.scopeFilter(c, storeHex), bson.M{"erp.hd": bson.M{"$ne": true}})
	if !q.IncludeDeleted {
		f = andFilter(f, bson.M{"erp.del": bson.M{"$ne": true}})
		if b.deletedKey != "" {
			f = andFilter(f, bson.M{b.deletedKey: bson.M{"$ne": true}})
		}
	}
	if from := q.fromIn(c.storeLoc(storeHex)); from != nil && b.dateKey != "" {
		f = andFilter(f, bson.M{b.dateKey: bson.M{"$gte": *from}})
	}
	f = andFilter(f, searchFilter(q.Search, b.searchKeys), legacyIDsFilter(q.IDs))
	ctx, cancel := dbctx()
	defer cancel()
	total, err := b.col(storeHex).CountDocuments(ctx, f)
	if err != nil {
		return nil, 0, errInternal("db: " + err.Error())
	}
	sortKey := b.sortKey
	if sortKey == "" {
		sortKey = b.dateKey
	}
	if q.Search != "" && b.searchSort != "" {
		sortKey = b.searchSort
	}
	sortD := bson.D{}
	if sortKey != "" && sortKey != "_id" {
		sortD = append(sortD, bson.E{Key: sortKey, Value: 1})
	}
	sortD = append(sortD, bson.E{Key: "_id", Value: 1})
	opts := options.Find().SetSort(sortD)
	if p := q.Select.dbProjection(envKey + ".h"); p != nil {
		opts.SetProjection(p)
	}
	if !all {
		opts.SetSkip(int64((q.Page - 1) * q.Limit)).SetLimit(int64(q.Limit))
	}
	cur, err := b.col(storeHex).Find(ctx, f, opts)
	if err != nil {
		return nil, 0, errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	x := newMapCtx(c, storeHex)
	rows := []M{}
	for cur.Next(ctx) {
		rows = append(rows, b.render(x, bsonToM(cur.Current), b.storeScoped()))
	}
	return rows, total, cur.Err()
}

func (b *legacyBackend) Locate(c *Ctx, id string) (string, bool) {
	if b.mainOrg {
		return "", true
	}
	for _, s := range c.storeHexes() {
		if d, _ := b.loadRaw(c, s, id, true); d != nil {
			if b.inMain && b.mainStoreKey != "" {
				return hexOf(d[b.mainStoreKey]), c.store(hexOf(d[b.mainStoreKey])) != nil
			}
			return s, true
		}
		if b.inMain && b.mainStoreKey == "" {
			break
		}
	}
	return "", false
}

func (b *legacyBackend) findStore(c *Ctx, storeHex, id string) string {
	if b.mainOrg {
		return ""
	}
	if storeHex != "" && !b.orgOverStores {
		return storeHex
	}
	if s, ok := b.Locate(c, id); ok {
		return s
	}
	return ""
}

func (b *legacyBackend) Get(c *Ctx, storeHex, id string, includeHidden bool) (M, error) {
	storeHex = b.findStore(c, storeHex, id)
	if storeHex == "" && !b.inMain && !b.mainOrg {
		return nil, nil
	}
	d, err := b.loadRaw(c, storeHex, id, includeHidden)
	if err != nil || d == nil {
		return nil, err
	}
	return b.render(newMapCtx(c, storeHex), d, b.storeScoped()), nil
}

func allKeys(rec M) map[string]bool {
	s := map[string]bool{}
	for k := range rec {
		s[k] = true
	}
	return s
}

func (b *legacyBackend) storeParam(storeHex string) string {
	if b.v1.noStoreParam {
		return ""
	}
	return storeHex
}

func (b *legacyBackend) call(c *Ctx, h http.HandlerFunc, method, path string, vars map[string]string, storeHex string, body interface{}) (*v1Result, error) {
	if b.v1.storeQueryKey != "" && storeHex != "" {
		return callV1Q(c, h, method, path, vars, url.Values{b.v1.storeQueryKey: {storeHex}}, body)
	}
	return callV1(c, h, method, path, vars, b.storeParam(storeHex), body)
}

func (b *legacyBackend) Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error) {
	if b.readOnly || b.v1.create == nil {
		return nil, errForbidden("This resource is read-only.")
	}
	if b.orgOverStores {
		storeHex = c.primaryStore()
		if storeHex == "" {
			return nil, errForbidden("No store available for this account.")
		}
	}
	clientID := strings.TrimSpace(str(body["id"]))
	if clientID != "" {
		if d, _ := b.loadRaw(c, storeHex, clientID, true); d != nil {
			return nil, errConflict("id_conflict", "A record with this id already exists.")
		}
	}
	rec := stripServerOwned(body)
	x := newMapCtx(c, storeHex)
	if b.beforeCreate != nil {
		if err := b.beforeCreate(x, rec); err != nil {
			return nil, err
		}
	}
	if b.validate != nil {
		if errs := b.validate(x, rec, nil); len(errs) > 0 {
			return nil, errBadRequest("", errs)
		}
	}
	ch := allKeys(rec)
	payload, err := b.toL(x, rec, nil, ch, true)
	if err != nil {
		return nil, err
	}
	if !b.mainOrg && storeHex != "" {
		payload["store_id"] = storeHex // legacy create handlers read the store from the body
	}
	since := time.Now()
	res, err := b.call(c, b.v1.create, "POST", b.v1.path, nil, storeHex, payload)
	if err != nil {
		return nil, err
	}
	if !res.ok() {
		return nil, legacyErr(res, b.fieldErr, b.lineErr, b.lineKey)
	}
	hex := res.resultID()
	if hex == "" {
		return nil, errInternal("legacy create returned no id")
	}
	b.settle(storeHex, hex, since)
	if b.afterWrite != nil {
		if err := b.afterWrite(x, hex, rec, nil, ch, true); err != nil {
			return nil, err
		}
	}
	doc, _ := b.loadRaw(c, storeHex, hex, true)
	env := M{"v": int64(1), "h": []interface{}{historyEntryIn(x.loc(), c.UserName, "created", []interface{}{})},
		"x": b.extras(rec), "cb": c.UserName}
	if doc != nil {
		if t, ok := toTime(doc["updated_at"]); ok {
			env["ts"] = t
		}
	}
	if ca := str(body["createdAt"]); ca != "" {
		env["ca"] = ca
	}
	if clientID != "" && clientID != hex {
		env["cid"] = clientID
	}
	if err := b.setEnv(storeHex, hex, env); err != nil {
		return nil, err
	}
	b.afterStock(storeHex, rec, nil)
	return b.Get(c, storeHex, hex, true)
}

// extras returns the contract fields persisted in erp.x: unknown fields plus
// hybrid (partially mapped) nested objects.
func (b *legacyBackend) extras(rec M) M {
	x := extrasOf(rec, b.known)
	for k := range b.hybrid {
		if v, ok := rec[k]; ok {
			x[k] = v
		}
	}
	return x
}

func (b *legacyBackend) setEnv(storeHex, hex string, env M) error {
	oid, _ := oidOf(hex)
	ctx, cancel := dbctx()
	defer cancel()
	set := bson.M{}
	for k, v := range env {
		set[envKey+"."+k] = v
	}
	_, err := b.col(storeHex).UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": set})
	if err != nil {
		return errInternal("db: " + err.Error())
	}
	return nil
}

// bumpEnv records an adapter write on an existing legacy document.
func (b *legacyBackend) bumpEnv(c *Ctx, storeHex, hex string, prevDoc, prev, next M, action string, extra M) error {
	doc, _ := b.loadRaw(c, storeHex, hex, true)
	env := M{"v": versionOf(prevDoc) + 1,
		"h": appendHistory(historyOfIn(c.storeLoc(storeHex), prevDoc), historyEntryIn(c.storeLoc(storeHex), c.UserName, action, diff(prev, next)))}
	if doc != nil {
		if t, ok := toTime(doc["updated_at"]); ok {
			env["ts"] = t
		}
	}
	for k, v := range extra {
		env[k] = v
	}
	return b.setEnv(storeHex, hex, env)
}

func (b *legacyBackend) Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error) {
	if b.readOnly || (b.v1.update == nil && b.noUpdate == "") {
		return nil, errForbidden("This resource is read-only.")
	}
	storeHex = b.findStore(c, storeHex, id)
	prevDoc, err := b.loadRaw(c, storeHex, id, false)
	if err != nil {
		return nil, err
	}
	if prevDoc == nil {
		return nil, errNotFound()
	}
	hex := hexOf(prevDoc["_id"])
	x := newMapCtx(c, storeHex)
	if b.validate != nil {
		if errs := b.validate(x, next, prevDoc); len(errs) > 0 {
			return nil, errBadRequest("", errs)
		}
	}
	ch := changedSet(changed)
	mappedChange := false
	for k := range ch {
		if b.known[k] {
			mappedChange = true
		}
	}
	if mappedChange && b.noUpdate != "" {
		return nil, errUnsupported(b.noUpdate)
	}
	if mappedChange {
		payload, err := b.toL(x, next, prevDoc, ch, false)
		if err != nil {
			return nil, err
		}
		if len(payload) > 0 && b.replaceUpdate {
			full := legacyJSONOf(prevDoc, x.loc())
			for k, v := range payload {
				full[k] = v
			}
			payload = full
		}
		if len(payload) > 0 {
			if !b.mainOrg && storeHex != "" {
				payload["store_id"] = storeHex
			}
			since := time.Now()
			res, err := b.call(c, b.v1.update, "PUT", b.v1.path+"/"+hex, map[string]string{"id": hex}, storeHex, payload)
			if err != nil {
				return nil, err
			}
			if !res.ok() {
				return nil, legacyErr(res, b.fieldErr, b.lineErr, b.lineKey)
			}
			b.settle(storeHex, hex, since)
		}
		if b.afterWrite != nil {
			if err := b.afterWrite(x, hex, next, prevDoc, ch, false); err != nil {
				return nil, err
			}
		}
	}
	if err := b.bumpEnv(c, storeHex, hex, prevDoc, prev, next, meta.Action, M{"x": b.extras(next)}); err != nil {
		return nil, err
	}
	if mappedChange {
		b.afterStock(storeHex, next, prevDoc)
	}
	return b.Get(c, storeHex, hex, true)
}

func (b *legacyBackend) Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	storeHex = b.findStore(c, storeHex, id)
	prevDoc, err := b.loadRaw(c, storeHex, id, false)
	if err != nil {
		return nil, err
	}
	if prevDoc == nil {
		return nil, errNotFound()
	}
	hex := hexOf(prevDoc["_id"])
	if b.isDeleted(prevDoc) {
		return b.Get(c, storeHex, hex, true)
	}
	if b.adapterSoftDelete {
		if err := b.bumpEnv(c, storeHex, hex, prevDoc, M{"deleted": false}, M{"deleted": true}, "deleted", M{"del": true}); err != nil {
			return nil, err
		}
		return b.Get(c, storeHex, hex, true)
	}
	if b.v1.delete == nil {
		msg := b.noDelete
		if msg == "" {
			msg = "Deleting this kind of record is not supported by the existing system."
		}
		return nil, errUnsupported(msg)
	}
	res, err := b.call(c, b.v1.delete, "DELETE", b.v1.path+"/"+hex, map[string]string{"id": hex}, storeHex, nil)
	if err != nil {
		return nil, err
	}
	if !res.ok() {
		return nil, legacyErr(res, b.fieldErr, b.lineErr, b.lineKey)
	}
	after, _ := b.loadRaw(c, storeHex, hex, true)
	if after == nil {
		// the legacy delete removed the document permanently
		gone := b.render(newMapCtx(c, storeHex), prevDoc, b.storeScoped())
		gone["deleted"] = true
		return gone, nil
	}
	if !b.isDeleted(after) {
		msg := b.noDelete
		if msg == "" {
			msg = "The existing system does not delete this kind of record (its delete is a no-op); reverse it with a return/credit document instead."
		}
		return nil, errUnsupported(msg)
	}
	prev := b.render(newMapCtx(c, storeHex), prevDoc, b.storeScoped())
	next := cloneM(prev)
	next["deleted"] = true
	if err := b.bumpEnv(c, storeHex, hex, prevDoc, M{"deleted": false}, M{"deleted": true}, "deleted", nil); err != nil {
		return nil, err
	}
	b.afterStock(storeHex, nil, prevDoc)
	return b.Get(c, storeHex, hex, true)
}

func (b *legacyBackend) Restore(c *Ctx, storeHex, id string, meta WriteMeta) (M, error) {
	storeHex = b.findStore(c, storeHex, id)
	prevDoc, err := b.loadRaw(c, storeHex, id, false)
	if err != nil {
		return nil, err
	}
	if prevDoc == nil {
		return nil, errNotFound()
	}
	hex := hexOf(prevDoc["_id"])
	if b.adapterSoftDelete {
		if err := b.bumpEnv(c, storeHex, hex, prevDoc, M{"deleted": true}, M{"deleted": false}, "restored", M{"del": false}); err != nil {
			return nil, err
		}
		return b.Get(c, storeHex, hex, true)
	}
	if b.v1.restore == nil {
		msg := b.noRestore
		if msg == "" {
			msg = "Restoring this kind of record is not supported by the existing system."
		}
		return nil, errUnsupported(msg)
	}
	res, err := b.call(c, b.v1.restore, "POST", b.v1.path+"/restore/"+hex, map[string]string{"id": hex}, storeHex, M{})
	if err != nil {
		return nil, err
	}
	if !res.ok() {
		return nil, legacyErr(res, b.fieldErr, b.lineErr, b.lineKey)
	}
	if err := b.bumpEnv(c, storeHex, hex, prevDoc, M{"deleted": true}, M{"deleted": false}, "restored", nil); err != nil {
		return nil, err
	}
	b.afterStock(storeHex, nil, prevDoc)
	return b.Get(c, storeHex, hex, true)
}

// HardDelete never physically removes legacy documents (counters, ledger,
// ZATCA chain and the old app depend on them). It soft-deletes through the
// legacy delete path (which reverses stock/ledger effects exactly like the
// old app) and then hides the record from the adapter (erp.hd = true).
func (b *legacyBackend) HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error {
	storeHex = b.findStore(c, storeHex, id)
	prevDoc, err := b.loadRaw(c, storeHex, id, false)
	if err != nil {
		return err
	}
	if prevDoc == nil {
		return errNotFound()
	}
	hex := hexOf(prevDoc["_id"])
	if b.adapterSoftDelete && b.v1.delete != nil {
		res, err := b.call(c, b.v1.delete, "DELETE", b.v1.path+"/"+hex, map[string]string{"id": hex}, storeHex, nil)
		if err != nil {
			return err
		}
		if !res.ok() {
			return legacyErr(res, b.fieldErr, b.lineErr, b.lineKey)
		}
		return nil
	}
	if !b.isDeleted(prevDoc) {
		if _, err := b.Delete(c, storeHex, hex, meta); err != nil {
			return err
		}
	}
	if d, _ := b.loadRaw(c, storeHex, hex, true); d == nil {
		return nil
	}
	return b.setEnv(storeHex, hex, M{"hd": true})
}

// sortedKeys is a small helper for deterministic output.
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var _ Backend = (*legacyBackend)(nil)
var _ = primitive.NilObjectID
