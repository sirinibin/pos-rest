package erp

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// POS records: the working state of a POS terminal that has to be shared by
// every till and device of a store and survive a reload: held orders, open
// tables, the order board, appointments and queues, member cards, projects,
// subscriptions, measurements … (resource posRecords, NEW collection
// erp_pos_record in the store DB). Each record belongs to one terminal and is
// of one kind; `key` names the record when a terminal keeps one per subject
// (e.g. one per customer) and `data` holds the terminal's own fields.
//
//	{"terminal": "restaurant", "kind": "tables", "key": "", "status": "open", "data": {…}}
//
// Numbers a terminal hands out (order, ticket and job numbers) come from
// POST /pos-records/next-number, a per-store counter that never repeats across
// devices.

const (
	posRecordsColl  = "erp_pos_record"
	posCountersColl = "erp_pos_counter"
	maxPosDataBytes = 512 * 1024
	maxPosKeyLen    = 120
)

var posRecordWhere = map[string]bool{"terminal": true, "kind": true, "key": true, "status": true}

func posRecordValidate(x *mapCtx, rec M, prev M) map[string]string {
	e := map[string]string{}
	if t := str(rec["terminal"]); !posTerminals[t] {
		e["terminal"] = "unknown POS terminal"
	}
	if k := str(rec["kind"]); !rePosToken.MatchString(k) {
		e["kind"] = "letters, digits, - and _ only (max 40)"
	}
	if prev != nil {
		// a record never moves to another terminal or kind
		if str(prev["terminal"]) != "" && str(rec["terminal"]) != str(prev["terminal"]) {
			e["terminal"] = "cannot change"
		}
		if str(prev["kind"]) != "" && str(rec["kind"]) != str(prev["kind"]) {
			e["kind"] = "cannot change"
		}
	}
	if v, ok := rec["key"]; ok && v != nil {
		s, isStr := v.(string)
		if !isStr || len([]rune(s)) > maxPosKeyLen {
			e["key"] = "text of at most 120 characters"
		}
	}
	if v, ok := rec["status"]; ok && v != nil && str(v) != "" && !rePosToken.MatchString(str(v)) {
		e["status"] = "letters, digits, - and _ only (max 40)"
	}
	if v, ok := rec["data"]; ok && v != nil {
		switch v.(type) {
		case M, []interface{}, bson.A:
			if b, err := json.Marshal(v); err != nil || len(b) > maxPosDataBytes {
				e["data"] = "too large (at most 512 KB)"
			}
		default:
			e["data"] = "must be an object or a list"
		}
	}
	return e
}

func newPosRecordsResource() *Resource {
	r := newNativeResource("posRecords", "pos-records", "sales", "store", posRecordsColl, "", "", "pos", posRecordValidate)
	b := r.Backend.(*nativeBackend)
	b.whereKeys = posRecordWhere
	b.bareHistory = true
	return r
}

func registerPosRecords(s *mux.Router) {
	s.HandleFunc("/pos-records/next-number", authed(handlePosNextNumber)).Methods("POST")
}

// posCounterKey checks a counter name: terminal and key are POS tokens.
func posCounterKey(terminal, key string) (string, map[string]string) {
	e := map[string]string{}
	if !posTerminals[terminal] {
		e["terminal"] = "unknown POS terminal"
	}
	if !rePosToken.MatchString(key) {
		e["key"] = "letters, digits, - and _ only (max 40)"
	}
	if len(e) > 0 {
		return "", e
	}
	return terminal + ":" + key, nil
}

// POST /pos-records/next-number {storeId, terminal, key, start}
// → {"number": n}. The first number of a counter is `start` (default 1); every
// call after that is one higher, whichever device asks.
func handlePosNextNumber(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	if !c.can("sales", "create") {
		return errForbidden("")
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex := strings.TrimSpace(r.URL.Query().Get("storeId"))
	if storeHex == "" {
		storeHex = str(body["storeId"])
	}
	if storeHex == "" {
		return errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	if c.store(storeHex) == nil {
		return errForbidden("You do not have access to this store.")
	}
	id, errs := posCounterKey(str(body["terminal"]), str(body["key"]))
	start := int64(1)
	if v, ok := body["start"]; ok && v != nil {
		start = int64(num(v))
		if start < 0 || start > 1e12 {
			if errs == nil {
				errs = map[string]string{}
			}
			errs["start"] = "0 or more"
		}
	}
	if len(errs) > 0 {
		return errBadRequest("", errs)
	}
	n, err := nextPosNumber(storeHex, id, start)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, M{"number": n})
	return nil
}

func nextPosNumber(storeHex, id string, start int64) (int64, error) {
	ctx, cancel := dbctx()
	defer cancel()
	var out struct {
		Base int64 `bson:"base"`
		N    int64 `bson:"n"`
	}
	err := storeDB(storeHex).Collection(posCountersColl).FindOneAndUpdate(ctx,
		bson.M{"_id": id},
		bson.M{"$inc": bson.M{"n": int64(1)}, "$setOnInsert": bson.M{"base": start - 1}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&out)
	if err != nil {
		return 0, errInternal("db: " + err.Error())
	}
	return out.Base + out.N, nil
}

// maxPosSettingsBytes: store.posSettings (the owner's POS terminal setup, kept in
// erp.x) is one object of terminal → settings, at most 64 KB.
const maxPosSettingsBytes = 64 * 1024

// POS quick print formats (frontend src/pos/printFormats.js POS_PRINT_FORMATS)
var posPrintFormats = map[string]bool{"r80": true, "r58": true, "r112": true, "r76": true, "A4": true, "A5": true,
	"c95x11": true, "c95x55": true}

func validatePosSettings(rec M, e map[string]string) {
	v, ok := rec["posSettings"]
	if !ok || v == nil {
		return
	}
	m, isMap := v.(M)
	if !isMap {
		e["posSettings"] = "must be an object"
		return
	}
	for t, sv := range m {
		if !posTerminals[t] {
			e["posSettings."+t] = "unknown POS terminal"
			continue
		}
		sm, ok := sv.(M)
		if !ok && sv != nil {
			e["posSettings."+t] = "must be an object"
			continue
		}
		// printing (every terminal): quick print format and auto print
		if f, has := sm["printFormat"]; has && f != nil {
			if fs, isStr := f.(string); !isStr || !posPrintFormats[fs] {
				e["posSettings."+t+".printFormat"] = "unknown print format"
			}
		}
		if a, has := sm["autoPrint"]; has && a != nil {
			if _, isBool := a.(bool); !isBool {
				e["posSettings."+t+".autoPrint"] = "must be true or false"
			}
		}
	}
	if b, err := json.Marshal(v); err != nil || len(b) > maxPosSettingsBytes {
		e["posSettings"] = "too large (at most 64 KB)"
	}
}
