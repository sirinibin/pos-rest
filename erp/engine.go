package erp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// ListQuery carries the contract list parameters (§1.4).
type ListQuery struct {
	Limit          int
	Page           int
	IncludeDeleted bool
	From           *time.Time  // ?from= read as Asia/Riyadh; see fromIn
	FromRaw        string      // ?from= as sent (zone-less = store wall clock)
	Select         fieldSelect // ?select= (empty = every field)
	Search         string      // ?q= search text (pickers), see search.go
	IDs            []string    // ?ids= only these records
}

// WriteMeta carries per-write request metadata.
type WriteMeta struct {
	Action  string // X-Change-Reason / created / deleted / restored
	Replace bool   // PUT (Undo) instead of PATCH
}

// Backend stores one contract resource.
type Backend interface {
	List(c *Ctx, storeHex string, q ListQuery) ([]M, int64, error)
	// Get returns (nil, nil) when the id does not exist in that store.
	Get(c *Ctx, storeHex, id string, includeHidden bool) (M, error)
	// Locate finds which accessible store holds id ("" for org resources).
	Locate(c *Ctx, id string) (string, bool)
	Create(c *Ctx, storeHex string, body M, meta WriteMeta) (M, error)
	Update(c *Ctx, storeHex, id string, prev, next M, changed []string, meta WriteMeta) (M, error)
	Delete(c *Ctx, storeHex, id string, meta WriteMeta) (M, error)
	Restore(c *Ctx, storeHex, id string, meta WriteMeta) (M, error)
	HardDelete(c *Ctx, storeHex, id string, meta WriteMeta) error
}

// Resource describes one contract collection.
type Resource struct {
	Name      string // contract collection name (camelCase)
	Path      string // URL path segment (kebab-case)
	Scope     string // "org" | "store"
	Module    string // RBAC module
	DateField string // window field ("" = load all)
	Legacy    string // human description of the backing store (docs)
	Backend   Backend
	ReadOnly  string // non-empty: writes rejected with this message
}

var (
	registryMu sync.RWMutex
	registry   = map[string]*Resource{}
	regOrder   []*Resource
)

func register(r *Resource) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[r.Path]; !dup {
		regOrder = append(regOrder, r)
	}
	registry[r.Path] = r
}

func resourceByPath(p string) *Resource {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[p]
}

// Resources returns the registered resources in registration order.
func Resources() []*Resource {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return append([]*Resource{}, regOrder...)
}

// ---- id → store location cache ----

var locCache sync.Map // key res|id -> storeHex

func remember(res *Resource, id, storeHex string) {
	if id != "" && storeHex != "" {
		locCache.Store(res.Path+"|"+id, storeHex)
	}
}

// resolveStore determines and authorizes the store for a store-scoped
// request. For org-scoped resources it returns "".
func resolveStore(c *Ctx, res *Resource, id string, body M) (string, error) {
	if res.Scope != "store" {
		return "", nil
	}
	q := strings.TrimSpace(c.R.URL.Query().Get("storeId"))
	if q == "" && body != nil {
		q = str(body["storeId"])
	}
	if q != "" {
		if c.store(q) == nil {
			if id == "" || !c.Admin {
				return "", errForbidden("You do not have access to this store.")
			}
			return "", errForbidden("You do not have access to this store.")
		}
		if id == "" {
			return q, nil
		}
		return q, nil
	}
	if id == "" {
		return "", errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	if v, ok := locCache.Load(res.Path + "|" + id); ok {
		if s := v.(string); c.store(s) != nil {
			return s, nil
		}
	}
	if s, ok := res.Backend.Locate(c, id); ok {
		remember(res, id, s)
		return s, nil
	}
	return "", errNotFound()
}

// ---- HTTP handlers ----

func readBody(r *http.Request) (M, error) {
	b, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		return nil, errBadRequest("Unable to read request body.", nil)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return M{}, nil
	}
	var m M
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		return nil, errf(http.StatusBadRequest, "malformed_json", "Malformed JSON body: "+err.Error(), nil)
	}
	return normNumbers(m).(M), nil
}

// normNumbers converts json.Number to float64 (or int64 when integral).
func normNumbers(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, e := range t {
			t[k] = normNumbers(e)
		}
		return t
	case []interface{}:
		for i, e := range t {
			t[i] = normNumbers(e)
		}
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	}
	return v
}

func parseListQuery(r *http.Request, res *Resource) (ListQuery, error) {
	q := ListQuery{Limit: 500, Page: 1}
	v := r.URL.Query()
	if s := v.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return q, errBadRequest("Invalid limit.", map[string]string{"limit": "must be a positive integer"})
		}
		if n > 5000 {
			n = 5000
		}
		q.Limit = n
	}
	if s := v.Get("page"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return q, errBadRequest("Invalid page.", map[string]string{"page": "must be >= 1"})
		}
		q.Page = n
	}
	inc := v.Get("includeDeleted")
	q.IncludeDeleted = inc == "1" || strings.EqualFold(inc, "true")
	sel, err := parseSelect(v.Get("select"))
	if err != nil {
		return q, err
	}
	q.Select = sel
	if q.Search, q.IDs, err = parseSearch(v.Get("q"), v.Get("ids")); err != nil {
		return q, err
	}
	if s := v.Get("from"); s != "" && res.DateField != "" {
		t, err := parseClientTime(s)
		if err != nil {
			return q, errBadRequest("Invalid from date.", map[string]string{"from": "expected YYYY-MM-DD"})
		}
		q.From, q.FromRaw = &t, s
	}
	return q, nil
}

// fromIn is ?from= with a zone-less value read in the store zone loc.
func (q ListQuery) fromIn(loc *time.Location) *time.Time {
	if q.FromRaw == "" {
		return q.From
	}
	if t, err := parseClientTimeIn(loc, q.FromRaw); err == nil {
		return &t
	}
	return q.From
}

func (res *Resource) checkPerm(c *Ctx, verb string) error {
	if !c.can(res.Module, verb) {
		return errForbidden("")
	}
	return nil
}

func handleList(w http.ResponseWriter, r *http.Request, res *Resource) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := res.checkPerm(c, "view"); err != nil {
		writeErr(w, err)
		return
	}
	q, err := parseListQuery(r, res)
	if err != nil {
		writeErr(w, err)
		return
	}
	storeHex, err := resolveStore(c, res, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, total, err := res.Backend.List(c, storeHex, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, row := range rows {
		remember(res, str(row["id"]), storeHex)
	}
	if rows == nil {
		rows = []M{}
	}
	writeJSON(w, http.StatusOK, M{"data": q.Select.applyAll(rows), "total": total})
}

func handleGet(w http.ResponseWriter, r *http.Request, res *Resource) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := res.checkPerm(c, "view"); err != nil {
		writeErr(w, err)
		return
	}
	sel, err := parseSelect(r.URL.Query().Get("select"))
	if err != nil {
		writeErr(w, err)
		return
	}
	id := mux.Vars(r)["id"]
	storeHex, err := resolveStore(c, res, id, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	rec, err := res.Backend.Get(c, storeHex, id, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	if rec == nil {
		writeErr(w, errNotFound())
		return
	}
	writeJSON(w, http.StatusOK, sel.apply(rec))
}

func handleCreate(w http.ResponseWriter, r *http.Request, res *Resource) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		if res.ReadOnly != "" {
			return 0, nil, errForbidden(res.ReadOnly)
		}
		if err := res.checkPerm(c, "create"); err != nil {
			return 0, nil, err
		}
		body, err := readBody(r)
		if err != nil {
			return 0, nil, err
		}
		if res.Scope == "org" {
			delete(body, "storeId")
		}
		storeHex, err := resolveStore(c, res, "", body)
		if err != nil {
			return 0, nil, err
		}
		if storeHex != "" {
			body["storeId"] = storeHex
		}
		rec, err := res.Backend.Create(c, storeHex, body, WriteMeta{Action: "created"})
		if err != nil {
			return 0, nil, err
		}
		remember(res, str(rec["id"]), storeHex)
		return http.StatusCreated, rec, nil
	})
}

func ifMatch(r *http.Request) (int64, bool) {
	s := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), `"W/`)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1, true
	}
	return n, true
}

func handleUpdate(w http.ResponseWriter, r *http.Request, res *Resource, replace bool) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		if res.ReadOnly != "" {
			return 0, nil, errForbidden(res.ReadOnly)
		}
		if err := res.checkPerm(c, "edit"); err != nil {
			return 0, nil, err
		}
		id := mux.Vars(r)["id"]
		body, err := readBody(r)
		if err != nil {
			return 0, nil, err
		}
		storeHex, err := resolveStore(c, res, id, nil)
		if err != nil {
			return 0, nil, err
		}
		prev, err := res.Backend.Get(c, storeHex, id, false)
		if err != nil {
			return 0, nil, err
		}
		if prev == nil {
			return 0, nil, errNotFound()
		}
		if v, ok := ifMatch(r); ok && !replace {
			if v != intv(prev["version"]) {
				return 0, nil, errConflict("version_conflict", "")
			}
		}
		if res.Scope == "store" {
			if s := str(body["storeId"]); s != "" && s != storeHex {
				if c.store(s) == nil {
					return 0, nil, errForbidden("You do not have access to this store.")
				}
				return 0, nil, errBadRequest("A record cannot be moved to another store.", map[string]string{"storeId": "cannot change"})
			}
		}
		var next M
		var changed []string
		if replace {
			// PUT: full replacement minus server-owned fields
			next = stripServerOwned(body)
			keep := []string{"id", "version", "history", "createdAt", "createdBy", "deleted"}
			for _, k := range keep {
				if v, ok := prev[k]; ok {
					next[k] = v
				}
			}
			if res.Scope == "store" {
				next["storeId"] = storeHex
			}
			keys := map[string]bool{}
			for k := range prev {
				keys[k] = true
			}
			for k := range next {
				keys[k] = true
			}
			for k := range keys {
				if serverOwned[k] {
					continue
				}
				a, _ := json.Marshal(prev[k])
				b, _ := json.Marshal(next[k])
				if string(a) != string(b) {
					changed = append(changed, k)
				}
			}
		} else {
			next, changed = mergePatch(prev, body)
			if res.Scope == "store" {
				next["storeId"] = storeHex
			}
		}
		action := decodeChangeReason(r.Header.Get("X-Change-Reason"))
		if action == "" {
			action = "updated"
		}
		rec, err := res.Backend.Update(c, storeHex, id, prev, next, changed, WriteMeta{Action: action, Replace: replace})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, rec, nil
	})
}

func handleDelete(w http.ResponseWriter, r *http.Request, res *Resource) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		if res.ReadOnly != "" {
			return 0, nil, errForbidden(res.ReadOnly)
		}
		if err := res.checkPerm(c, "delete"); err != nil {
			return 0, nil, err
		}
		id := mux.Vars(r)["id"]
		storeHex, err := resolveStore(c, res, id, nil)
		if err != nil {
			return 0, nil, err
		}
		hard := r.URL.Query().Get("hard")
		if hard == "1" || strings.EqualFold(hard, "true") {
			prev, err := res.Backend.Get(c, storeHex, id, false)
			if err != nil {
				return 0, nil, err
			}
			if prev == nil {
				return 0, nil, errNotFound()
			}
			if err := res.Backend.HardDelete(c, storeHex, id, WriteMeta{Action: "hard deleted"}); err != nil {
				return 0, nil, err
			}
			return http.StatusNoContent, nil, nil
		}
		prev, err := res.Backend.Get(c, storeHex, id, false)
		if err != nil {
			return 0, nil, err
		}
		if prev == nil {
			return 0, nil, errNotFound()
		}
		if v, ok := ifMatch(r); ok && v != intv(prev["version"]) {
			return 0, nil, errConflict("version_conflict", "")
		}
		rec, err := res.Backend.Delete(c, storeHex, id, WriteMeta{Action: "deleted"})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, rec, nil
	})
}

func handleRestore(w http.ResponseWriter, r *http.Request, res *Resource) {
	withAuthAndIdem(w, r, func(c *Ctx) (int, interface{}, error) {
		if res.ReadOnly != "" {
			return 0, nil, errForbidden(res.ReadOnly)
		}
		if err := res.checkPerm(c, "edit"); err != nil {
			return 0, nil, err
		}
		id := mux.Vars(r)["id"]
		storeHex, err := resolveStore(c, res, id, nil)
		if err != nil {
			return 0, nil, err
		}
		prev, err := res.Backend.Get(c, storeHex, id, false)
		if err != nil {
			return 0, nil, err
		}
		if prev == nil {
			return 0, nil, errNotFound()
		}
		if !boolv(prev["deleted"]) {
			return http.StatusOK, prev, nil
		}
		rec, err := res.Backend.Restore(c, storeHex, id, WriteMeta{Action: "restored"})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, rec, nil
	})
}
