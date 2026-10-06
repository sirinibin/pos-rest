package erp

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestVersionOf(t *testing.T) {
	upd := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	later := upd.Add(time.Hour)
	tests := []struct {
		name string
		doc  M
		want int64
	}{
		{"never written, no updated_at", M{}, 1},
		{"never written, updated_at → unix", M{"updated_at": primitive.NewDateTimeFromTime(upd)}, upd.Unix()},
		{"adapter version, unchanged legacy", M{"updated_at": upd, "erp": M{"v": int64(3), "ts": upd}}, 3},
		{"adapter version, ts within 1s", M{"updated_at": upd.Add(500 * time.Millisecond), "erp": M{"v": int64(3), "ts": upd}}, 3},
		{"old app edited after adapter write", M{"updated_at": later, "erp": M{"v": int64(3), "ts": upd}}, later.Unix()},
		{"old app edit, large v", M{"updated_at": later, "erp": M{"v": later.Unix() + 10, "ts": upd}}, later.Unix() + 11},
		{"int32 erp.v", M{"erp": M{"v": int32(5)}}, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionOf(tc.doc); got != tc.want {
				t.Fatalf("versionOf=%d want %d", got, tc.want)
			}
		})
	}
}

func TestHistoryOf_SynthesizedFromLegacyAudit(t *testing.T) {
	c := time.Date(2026, 1, 1, 7, 0, 0, 0, time.UTC)
	u := c.Add(2 * time.Hour)
	h := historyOf(M{"created_at": c, "updated_at": u, "created_by_name": "A", "updated_by_name": "B"})
	if len(h) != 2 || h[0].(M)["action"] != "created" || h[1].(M)["by"] != "B" || h[0].(M)["at"] != "2026-01-01T10:00" {
		t.Fatalf("history=%v", h)
	}
	if len(historyOf(M{})) != 0 {
		t.Fatal("empty legacy doc has no history")
	}
	stored := []interface{}{M{"action": "x"}}
	if got := historyOf(M{"erp": M{"h": stored}}); len(got) != 1 {
		t.Fatal("stored history wins")
	}
}

func TestApplyEnvelope_ExtrasAndHybrid(t *testing.T) {
	doc := M{"created_at": time.Now(), "erp": M{"v": int64(2), "x": M{
		"posMeta": M{"till": "T1"},                    // unknown → preserved
		"nameEn":  "SHOULD NOT OVERRIDE",              // mapped keys win
		"stock":   M{"w1": M{"qty": 1.0, "min": 3.0}}, // hybrid: x as base
	}}}
	rec := applyEnvelope(M{"nameEn": "Legacy", "stock": M{"w1": M{"qty": 9.0}}}, doc, false)
	if rec["nameEn"] != "Legacy" {
		t.Fatal("mapped value must win over preserved extras")
	}
	if get(rec, "posMeta.till") != "T1" {
		t.Fatal("extras not preserved")
	}
	if get(rec, "stock.w1.qty") != 9.0 || get(rec, "stock.w1.min") != 3.0 {
		t.Fatalf("hybrid merge wrong: %v", rec["stock"])
	}
	if rec["version"] != int64(2) || rec["deleted"] != false {
		t.Fatalf("envelope: %v", rec)
	}
}

func TestDiff_ClientRules(t *testing.T) {
	prev := M{"a": "x", "n": 1.0, "items": []interface{}{1, 2}, "addr": M{"city": "R"}, "history": "ignored", "same": ""}
	next := M{"a": "y", "n": 1.0, "items": []interface{}{1}, "addr": M{"city": "J"}, "history": "changed"}
	ch := diff(prev, next)
	got := map[string]M{}
	for _, c := range ch {
		got[c.(M)["field"].(string)] = c.(M)
	}
	if len(ch) != 3 {
		t.Fatalf("changes=%v", ch)
	}
	if got["a"]["to"] != "y" || got["items"]["to"] != "1 items" || got["addr.city"]["from"] != "R" {
		t.Fatalf("diff=%v", got)
	}
	long := M{}
	for i := 0; i < 50; i++ {
		long["k"+itoa(i)] = i
	}
	if len(diff(M{}, long)) != 30 {
		t.Fatal("max 30 changes")
	}
}

func TestMergePatch_TopLevelNullClears(t *testing.T) {
	prev := M{"a": 1.0, "b": M{"x": 1.0, "y": 2.0}, "c": "keep", "id": "p1", "version": 3.0}
	next, ch := mergePatch(prev, M{"a": nil, "b": M{"x": 5.0}, "version": 99.0, "id": "hack"})
	if _, ok := next["a"]; ok {
		t.Fatal("null must clear")
	}
	if get(next, "b.y") != nil || get(next, "b.x") != 5.0 {
		t.Fatal("nested objects are replaced, not deep-merged")
	}
	if next["id"] != "p1" || next["version"] != 3.0 || next["c"] != "keep" {
		t.Fatalf("server-owned keys must be ignored: %v", next)
	}
	if len(ch) != 2 {
		t.Fatalf("changed=%v", ch)
	}
}

func TestStripServerOwnedAndExtras(t *testing.T) {
	in := M{"id": "x", "version": 4, "history": []interface{}{}, "createdAt": "a", "updatedBy": "b", "deleted": true, "name": "n", "storeId": "s", "foo": 1}
	out := stripServerOwned(in)
	if len(out) != 3 {
		t.Fatalf("stripped=%v", out)
	}
	x := extrasOf(out, knownSet("name"))
	if len(x) != 1 || x["foo"] != 1 {
		t.Fatalf("extras=%v", x)
	}
}

func TestDecodeChangeReason(t *testing.T) {
	if decodeChangeReason("paid") != "paid" || decodeChangeReason("status%3A%20done") != "status: done" {
		t.Fatal("decode")
	}
	long := "0123456789012345678901234567890123456789XYZ"
	if len(decodeChangeReason(long)) != 40 {
		t.Fatal("max 40 chars")
	}
}

func TestHistoryEntryAndCap(t *testing.T) {
	e := historyEntry("A", "", nil)
	if e["action"] != "updated" {
		t.Fatal("default action")
	}
	h := []interface{}{}
	for i := 0; i < maxHistory+5; i++ {
		h = appendHistory(h, M{"i": i})
	}
	if len(h) != maxHistory || h[0].(M)["i"] != 5 {
		t.Fatal("history must be capped keeping the newest")
	}
}
