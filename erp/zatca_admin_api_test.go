package erp

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed API + integration tests: the business category never marks a
// store for ZATCA re-connection, and platform admins list marked stores and
// clear the mark without touching the store's ZATCA credentials.
func TestAPI_ZatcaReconnect_CategoryAndAdminUnmark(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	owner, sid := signupOwner(t, "zunmark")
	defer cleanupBilling(t, sid)
	oid, _ := oidOf(sid)

	// the store is connected to ZATCA Phase 2 and holds credentials
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"zatca.connected": true, "zatca.phase": "2", "zatca.env": "Simulation", "zatca.production_request_id": int64(4242),
		"zatca.zatca_reconnect_required": false, "zatca.private_key": "PK", "zatca.production_secret": "PSECRET",
		"zatca.production_binary_security_token": "PTOKEN"}}); err != nil {
		t.Fatal(err)
	}
	rawZatca := func() M {
		var raw bson.M
		c2, cl := dbctx()
		defer cl()
		_ = mainDB().Collection("store").FindOne(c2, bson.M{"_id": oid}).Decode(&raw)
		return sub(normDoc(raw), "zatca")
	}
	patch := func(tok string, body M) resp {
		g := call(t, "GET", "/stores/"+sid, tok, nil)
		return call(t, "PATCH", "/stores/"+sid, tok, body, "If-Match", str(g.Body["version"]), "X-Change-Reason", "zatca test")
	}

	// 1. changing the business category does NOT mark the store
	for _, cat := range []string{"Grocery", "Supermarket"} {
		p := patch(owner, M{"category": cat})
		if p.Code != 200 || p.Body["category"] != cat || get(p.Body, "zatca.reconnectNeeded") != false {
			t.Fatalf("category %s must not mark reconnect: %d %s", cat, p.Code, p.Raw)
		}
	}
	if boolv(rawZatca()["zatca_reconnect_required"]) {
		t.Fatalf("category change marked the store in the DB")
	}

	// 2. a ZATCA-sensitive change still marks it
	if p := patch(owner, M{"nameEn": "Al Noor Trading Company"}); p.Code != 200 || get(p.Body, "zatca.reconnectNeeded") != true {
		t.Fatalf("name change must mark reconnect: %d %s", p.Code, p.Raw)
	}

	// 3. only platform admins may list or unmark (the store's own owner may not)
	if r := call(t, "GET", "/admin/zatca-reconnects", owner, nil); r.Code != 403 {
		t.Fatalf("owner list: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/stores/"+sid+"/zatca/unmark", owner, M{}); r.Code != 403 {
		t.Fatalf("owner unmark: %d %s", r.Code, r.Raw)
	}
	if !boolv(rawZatca()["zatca_reconnect_required"]) {
		t.Fatalf("a refused unmark must keep the mark")
	}

	// 4. the admin list shows the marked store (search + limit)
	l := call(t, "GET", "/admin/zatca-reconnects?q=noor+trading+company&limit=10", admin, nil)
	if l.Code != 200 || intv(l.Body["total"]) < 1 {
		t.Fatalf("list: %d %s", l.Code, l.Raw)
	}
	var row M
	for _, it := range arr(l.Body["items"]) {
		if m, _ := it.(M); str(m["id"]) == sid {
			row = m
		}
	}
	if row == nil || row["nameEn"] != "Al Noor Trading Company" || row["category"] != "Supermarket" ||
		get(row, "zatca.phase") != "2" || get(row, "zatca.reconnectNeeded") != true || strings.Contains(l.Raw, "PSECRET") {
		t.Fatalf("list row: %v (%s)", row, l.Raw)
	}
	if r := call(t, "GET", "/admin/zatca-reconnects?q=no-such-store-zz", admin, nil); r.Code != 200 || len(arr(r.Body["items"])) != 0 {
		t.Fatalf("search miss: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/admin/zatca-reconnects?limit=0", admin, nil); r.Code != 400 || r.errField("limit") == "" {
		t.Fatalf("bad limit: %d %s", r.Code, r.Raw)
	}

	// 5. unmark: clears the flag, keeps credentials, phase and environment
	u := call(t, "POST", "/stores/"+sid+"/zatca/unmark", admin, M{})
	if u.Code != 200 || get(u.Body, "zatca.reconnectNeeded") != false || str(get(u.Body, "zatca.reconnectClearedAt")) == "" ||
		str(get(u.Body, "zatca.reconnectClearedBy")) == "" {
		t.Fatalf("unmark: %d %s", u.Code, u.Raw)
	}
	z := rawZatca()
	if boolv(z["zatca_reconnect_required"]) || z["private_key"] != "PK" || z["production_secret"] != "PSECRET" ||
		z["production_binary_security_token"] != "PTOKEN" || !boolv(z["connected"]) || str(z["phase"]) != "2" ||
		str(z["env"]) != "Simulation" || intv(z["production_request_id"]) != 4242 {
		t.Fatalf("unmark must keep the ZATCA credentials: %v", z)
	}
	g := call(t, "GET", "/stores/"+sid, owner, nil)
	if g.Code != 200 || get(g.Body, "zatca.reconnectNeeded") != false || get(g.Body, "zatca.connected") != true ||
		get(g.Body, "zatca.pcsid") != "4242" || str(get(g.Body, "zatca.reconnectClearedAt")) == "" {
		t.Fatalf("store after unmark: %d %s", g.Code, g.Raw)
	}

	// 6. no longer listed; unmarking again / unknown stores
	l2 := call(t, "GET", "/admin/zatca-reconnects?q=noor+trading+company", admin, nil)
	for _, it := range arr(l2.Body["items"]) {
		if m, _ := it.(M); str(m["id"]) == sid {
			t.Fatalf("unmarked store still listed: %s", l2.Raw)
		}
	}
	if r := call(t, "POST", "/stores/"+sid+"/zatca/unmark", admin, M{}); r.Code != 409 || r.errCode() != "zatca_not_marked" {
		t.Fatalf("unmark twice: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/stores/not-an-id/zatca/unmark", admin, M{}); r.Code != 404 {
		t.Fatalf("bad id: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/stores/0123456789abcdef01234567/zatca/unmark", admin, M{}); r.Code != 404 {
		t.Fatalf("unknown store: %d %s", r.Code, r.Raw)
	}

	// 7. a later sensitive change marks it again (unmarking is not permanent)
	if p := patch(owner, M{"vatNo": "310122393500013"}); p.Code != 200 || get(p.Body, "zatca.reconnectNeeded") != true {
		t.Fatalf("re-mark after unmark: %d %s", p.Code, p.Raw)
	}
}
