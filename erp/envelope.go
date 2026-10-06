package erp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// The single additive sub-document written onto legacy documents:
//
//	erp: {
//	  v:   int64   adapter version counter (see versionOf)
//	  ts:  date    legacy updated_at observed right after the last adapter write
//	  h:   []      contract history entries {at, by, action, changes}
//	  cid: string  client-supplied id used at create (lookup alias)
//	  x:   {}      contract fields that have no legacy equivalent (preserved verbatim)
//	  ca:  string  client createdAt (verbatim), cb: createdBy display name
//	  hd:  bool    "hard deleted" through the adapter (legacy doc soft-deleted, hidden everywhere in the adapter)
//	  role: string contract role id (users only)
//	}
const envKey = "erp"

const maxHistory = 200

// serverOwned are never accepted from clients.
var serverOwned = map[string]bool{
	"id": true, "version": true, "history": true, "createdAt": true, "createdBy": true,
	"updatedAt": true, "updatedBy": true, "deleted": true,
}

// versionOf derives the contract version of a legacy document.
//   - erp.v when the legacy updated_at is unchanged since our last write;
//   - otherwise max(erp.v+1, unix(updated_at)) so edits made by the old app
//     (which never touch erp.*) still change the version and trigger 409s;
//   - documents never written by the adapter: unix(updated_at) (or 1).
func versionOf(doc M) int64 {
	env := sub(doc, envKey)
	v := intv(env["v"])
	upd, hasUpd := toTime(doc["updated_at"])
	ts, hasTs := toTime(env["ts"])
	if v > 0 {
		if !hasUpd || (hasTs && !upd.After(ts.Add(time.Second))) {
			return v
		}
		u := upd.Unix()
		if u > v+1 {
			return u
		}
		return v + 1
	}
	if hasUpd {
		return upd.Unix()
	}
	return 1
}

// historyOf returns the stored history or one synthesized from the legacy
// audit fields.
func historyOf(doc M) []interface{} { return historyOfIn(riyadh, doc) }

// historyOfIn is historyOf with synthesized times in the store zone loc.
func historyOfIn(loc *time.Location, doc M) []interface{} {
	if h := arr(get(doc, envKey+".h")); len(h) > 0 {
		return h
	}
	out := []interface{}{}
	if ca := fmtDTIn(loc, doc["created_at"]); ca != "" {
		out = append(out, M{"at": ca, "by": str(doc["created_by_name"]), "action": "created", "changes": []interface{}{}})
	}
	if ua := fmtDTIn(loc, doc["updated_at"]); ua != "" && ua != fmtDTIn(loc, doc["created_at"]) {
		out = append(out, M{"at": ua, "by": str(doc["updated_by_name"]), "action": "updated", "changes": []interface{}{}})
	}
	return out
}

// applyEnvelope adds the common contract envelope fields to a mapped record.
func applyEnvelope(rec M, doc M, deleted bool) M { return applyEnvelopeIn(riyadh, rec, doc, deleted) }

// applyEnvelopeIn is applyEnvelope with legacy audit times in the store zone.
func applyEnvelopeIn(loc *time.Location, rec M, doc M, deleted bool) M {
	env := sub(doc, envKey)
	rec["version"] = versionOf(doc)
	rec["deleted"] = deleted
	rec["history"] = historyOfIn(loc, doc)
	if ca := str(env["ca"]); ca != "" {
		rec["createdAt"] = ca
	} else if _, ok := rec["createdAt"]; !ok {
		rec["createdAt"] = fmtDTIn(loc, doc["created_at"])
	}
	if cb := str(env["cb"]); cb != "" {
		rec["createdBy"] = cb
	} else if _, ok := rec["createdBy"]; !ok {
		rec["createdBy"] = str(doc["created_by_name"])
	}
	if _, ok := rec["updatedAt"]; !ok {
		rec["updatedAt"] = fmtDTIn(loc, doc["updated_at"])
	}
	if _, ok := rec["updatedBy"]; !ok {
		rec["updatedBy"] = str(doc["updated_by_name"])
	}
	// preserved unknown fields never override mapped ones
	for k, v := range sub(env, "x") {
		cur, exists := rec[k]
		if !exists {
			rec[k] = v
			continue
		}
		// hybrid nested objects: legacy-mapped sub-keys override preserved ones
		if xm, ok := v.(M); ok {
			if cm, ok := cur.(M); ok {
				rec[k] = deepMerge(xm, cm)
			}
		}
	}
	return rec
}

// deepMerge returns base overlaid with over (maps merged recursively).
func deepMerge(base, over M) M {
	out := cloneM(base)
	for k, v := range over {
		if vm, ok := v.(M); ok {
			if bm, ok := out[k].(M); ok {
				out[k] = deepMerge(bm, vm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// ---- history diff (mirrors client wA, L15404) ----

func histVal(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "—"
	case string:
		if t == "" {
			return "—"
		}
		if len([]rune(t)) > 80 {
			return string([]rune(t)[:80]) + "…"
		}
		return t
	case []interface{}:
		return fmt.Sprintf("%d items", len(t))
	case M:
		return "{…}"
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func diff(prev, next M) []interface{} {
	changes := []interface{}{}
	var walk func(prefix string, a, b M)
	walk = func(prefix string, a, b M) {
		keys := map[string]bool{}
		for k := range a {
			keys[k] = true
		}
		for k := range b {
			keys[k] = true
		}
		ks := make([]string, 0, len(keys))
		for k := range keys {
			if prefix == "" && (k == "history" || k == "updatedAt" || k == "updatedBy" || k == "version") {
				continue
			}
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			if len(changes) >= 30 {
				return
			}
			av, bv := a[k], b[k]
			am, aok := av.(M)
			bm, bok := bv.(M)
			if aok && bok {
				walk(prefix+k+".", am, bm)
				continue
			}
			ja, _ := json.Marshal(av)
			jb, _ := json.Marshal(bv)
			if string(ja) != string(jb) {
				from, to := histVal(av), histVal(bv)
				if from == to {
					continue // e.g. "" vs missing: no visible change
				}
				changes = append(changes, M{"field": prefix + k, "from": from, "to": to})
			}
		}
	}
	walk("", prev, next)
	return changes
}

func historyEntry(by, action string, changes []interface{}) M {
	return historyEntryIn(riyadh, by, action, changes)
}

// historyEntryIn stamps the entry with the current time in the store zone.
func historyEntryIn(loc *time.Location, by, action string, changes []interface{}) M {
	if action == "" {
		action = "updated"
	}
	if len(action) > 40 {
		action = action[:40]
	}
	return M{"at": nowFn().In(orRiyadh(loc)).Format(layoutDT), "by": by, "action": action, "changes": changes}
}

func appendHistory(h []interface{}, e M) []interface{} {
	out := append(append([]interface{}{}, h...), e)
	if len(out) > maxHistory {
		out = out[len(out)-maxHistory:]
	}
	return out
}

// stripServerOwned removes envelope keys a client may not set.
func stripServerOwned(body M) M {
	out := M{}
	for k, v := range body {
		if serverOwned[k] {
			continue
		}
		out[k] = v
	}
	return out
}

// mergePatch applies the contract's top-level PATCH semantics: each key is
// replaced wholesale; null clears it.
func mergePatch(prev, patch M) (M, []string) {
	next := cloneM(prev)
	changed := []string{}
	for k, v := range patch {
		if serverOwned[k] {
			continue
		}
		if v == nil {
			if _, ok := next[k]; ok {
				delete(next, k)
			}
		} else {
			next[k] = v
		}
		changed = append(changed, k)
	}
	sort.Strings(changed)
	return next, changed
}

func changedSet(keys []string) map[string]bool {
	s := map[string]bool{}
	for _, k := range keys {
		s[k] = true
	}
	return s
}

// extrasOf returns the contract fields not consumed by a mapper.
func extrasOf(rec M, known map[string]bool) M {
	x := M{}
	for k, v := range rec {
		if known[k] || serverOwned[k] || k == "storeId" {
			continue
		}
		x[k] = v
	}
	return x
}

// decodeChangeReason implements X-Change-Reason (URL-decoded when needed).
func decodeChangeReason(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "%") {
		if d, err := urlUnescape(s); err == nil {
			s = d
		}
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
