package erp

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
	"go.mongodb.org/mongo-driver/bson"
)

func storeA() string { return fx.StoreA.Hex() }
func storeB() string { return fx.StoreB.Hex() }

// ---------------- auth ----------------

func TestAPI_Auth_LoginMeRefreshLogout(t *testing.T) {
	requireDB(t)
	// e-mail match is case-insensitive and trimmed (stored as "Manager+…@T1.example")
	r := call(t, "POST", "/auth/login", "", M{"email": "  " + strings.ToLower(fx.ManagerEmail) + " ", "password": erpfixture.Password})
	if r.Code != 200 || str(r.Body["accessToken"]) == "" || str(r.Body["refreshToken"]) == "" {
		t.Fatalf("login: %d %s", r.Code, r.Raw)
	}
	if get(r.Body, "user.role") != "r_manager" || get(r.Body, "user.lastLogin") == "" {
		t.Fatalf("login user: %v", r.Body["user"])
	}
	at, rt := str(r.Body["accessToken"]), str(r.Body["refreshToken"])
	// the issued token is a legacy token: accepted by existing v1 middleware too
	me := call(t, "GET", "/auth/me", at, nil)
	if me.Code != 200 {
		t.Fatalf("me: %d %s", me.Code, me.Raw)
	}
	ids := arr(get(me.Body, "user.storeIds"))
	stores := arr(me.Body["stores"])
	if len(ids) != 1 || ids[0] != storeA() || len(stores) != 1 || get(stores[0].(M), "id") != storeA() {
		t.Fatalf("me storeIds=%v stores=%d", ids, len(stores))
	}
	st := stores[0].(M)
	for _, k := range []string{"short", "serials", "vatPercent", "flags", "zatca", "address", "nameEn"} {
		if _, ok := st[k]; !ok {
			t.Errorf("store record missing %s", k)
		}
	}
	// wrong password / unknown e-mail
	if r := call(t, "POST", "/auth/login", "", M{"email": fx.ManagerEmail, "password": "nope"}); r.Code != 401 || str(get(r.Body, "error.message")) != "Email or password is incorrect." {
		t.Fatalf("wrong pw: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/auth/login", "", M{"email": "nobody@x.example", "password": "x"}); r.Code != 401 {
		t.Fatalf("unknown: %d", r.Code)
	}
	// refresh (no Authorization header needed), rotation revokes the used refresh token
	rr := call(t, "POST", "/auth/refresh", "", M{"refreshToken": rt})
	if rr.Code != 200 || str(rr.Body["accessToken"]) == "" {
		t.Fatalf("refresh: %d %s", rr.Code, rr.Raw)
	}
	if again := call(t, "POST", "/auth/refresh", "", M{"refreshToken": rt}); again.Code != 401 {
		t.Fatalf("used refresh token must be revoked: %d", again.Code)
	}
	if bad := call(t, "POST", "/auth/refresh", "", M{"refreshToken": at}); bad.Code != 401 {
		t.Fatal("an access token is not a refresh token")
	}
	// logout revokes the access token
	if lo := call(t, "POST", "/auth/logout", at, M{"refreshToken": str(rr.Body["refreshToken"])}); lo.Code != 204 {
		t.Fatalf("logout: %d", lo.Code)
	}
	if after := call(t, "GET", "/auth/me", at, nil); after.Code != 401 {
		t.Fatalf("token still valid after logout: %d", after.Code)
	}
	if after := call(t, "POST", "/auth/refresh", "", M{"refreshToken": str(rr.Body["refreshToken"])}); after.Code != 401 {
		t.Fatal("refresh token passed to logout must be revoked")
	}
}

func TestAPI_Auth_InactiveUserCannotLogin(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	email := "inactive+" + fx.StoreA.Hex()[18:] + "@t1.example"
	cr := call(t, "POST", "/users", admin, M{"name": "Temp User", "email": email, "phone": "0551230000", "role": "r_viewer",
		"storeIds": []string{storeA()}, "password": "Temp@12345"})
	if cr.Code != 201 {
		t.Fatalf("create user: %d %s", cr.Code, cr.Raw)
	}
	id := str(cr.Body["id"])
	if r := call(t, "POST", "/auth/login", "", M{"email": email, "password": "Temp@12345"}); r.Code != 200 {
		t.Fatalf("new user login: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "PATCH", "/users/"+id, admin, M{"status": "inactive"}, "X-Change-Reason", "disabled"); r.Code != 200 || r.Body["status"] != "inactive" {
		t.Fatalf("disable: %d %s", r.Code, r.Raw)
	}
	r := call(t, "POST", "/auth/login", "", M{"email": email, "password": "Temp@12345"})
	if r.Code != 403 || r.errCode() != "inactive" {
		t.Fatalf("inactive login: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_Signup(t *testing.T) {
	requireDB(t)
	body := validSignup()
	email := "owner+" + time.Now().Format("150405.000") + "@signup.example"
	body["owner"].(M)["email"] = email
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	if get(r.Body, "user.role") != "r_admin" || len(arr(get(r.Body, "user.storeIds"))) != 1 {
		t.Fatalf("owner: %v", r.Body["user"])
	}
	st := sub(r.Body, "store")
	if st["vatNo"] != "310122393500003" || st["crNo"] != "1010101010" || st["short"] != "ANTC" || get(st, "address.buildingNo") != "1234" ||
		get(st, "zatca.phase") != 2.0 || get(st, "zatca.connected") != false || st["vatPercent"] != 15.0 {
		t.Fatalf("store: %v", st)
	}
	tok := str(r.Body["accessToken"])
	me := call(t, "GET", "/auth/me", tok, nil)
	if me.Code != 200 || len(arr(me.Body["stores"])) != 1 {
		t.Fatalf("me after signup: %d %s", me.Code, me.Raw)
	}
	// warehouses list contains ≥1 warehouse (the virtual main store)
	sid := str(st["id"])
	wh := call(t, "GET", "/warehouses?storeId="+sid, tok, nil)
	if wh.Code != 200 || len(wh.data()) < 1 {
		t.Fatalf("warehouses: %d %s", wh.Code, wh.Raw)
	}
	// duplicate owner e-mail (case-insensitive)
	body["owner"].(M)["email"] = strings.ToUpper(email)
	if d := call(t, "POST", "/auth/signup", "", body); d.Code != 409 || d.errField("owner.email") == "" {
		t.Fatalf("duplicate: %d %s", d.Code, d.Raw)
	}
	// cleanup the signup store
	cleanupStore(t, sid)
}

func cleanupStore(t *testing.T, sid string) {
	oid, _ := oidOf(sid)
	ctx, cancel := dbctx()
	defer cancel()
	_ = db.GetDB("store_" + sid).Drop(ctx)
	_, _ = mainDB().Collection("store").DeleteOne(ctx, bson.M{"_id": oid})
	_, _ = mainDB().Collection("user").DeleteMany(ctx, bson.M{"store_ids": oid})
}

// ---------------- generic collection API ----------------

func TestAPI_ListShapeAndScopes(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.AdminEmail)
	for _, res := range Resources() {
		path := "/" + res.Path + "?limit=500&includeDeleted=1&page=1"
		if res.Scope == "store" {
			path += "&storeId=" + storeA()
		}
		r := call(t, "GET", path, tok, nil)
		if r.Code != 200 {
			t.Errorf("%s: %d %s", res.Path, r.Code, r.Raw)
			continue
		}
		if _, ok := r.Body["total"]; !ok {
			t.Errorf("%s: missing total", res.Path)
		}
		for _, row := range r.data() {
			m := row.(M)
			if str(m["id"]) == "" || m["version"] == nil || m["deleted"] == nil {
				t.Errorf("%s: envelope fields missing: %v", res.Path, m)
			}
			// rule 27: org records never carry storeId, store records always do
			_, has := m["storeId"]
			if res.Scope == "org" && has {
				t.Errorf("%s: org record has storeId", res.Path)
			}
			if res.Scope == "store" && m["storeId"] != storeA() {
				t.Errorf("%s: store record storeId=%v", res.Path, m["storeId"])
			}
		}
	}
	// store-scoped list without storeId
	if r := call(t, "GET", "/sales", tok, nil); r.Code != 400 || r.errField("storeId") == "" {
		t.Fatalf("missing storeId: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_PaginationWindowIncludeDeleted(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	all := call(t, "GET", "/customers?storeId="+storeA()+"&includeDeleted=1&limit=500", tok, nil)
	total := int(num(all.Body["total"]))
	if total < 2 {
		t.Fatalf("fixture customers: %d", total)
	}
	seen := map[string]bool{}
	for page := 1; page <= total+1; page++ {
		r := call(t, "GET", "/customers?storeId="+storeA()+"&includeDeleted=1&limit=1&page="+itoa(page), tok, nil)
		if int(num(r.Body["total"])) != total {
			t.Fatalf("total must be stable across pages")
		}
		if len(r.data()) == 0 {
			break
		}
		id := str(r.data()[0].(M)["id"])
		if seen[id] {
			t.Fatalf("duplicate id across pages: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != total {
		t.Fatalf("paging visited %d of %d", len(seen), total)
	}
	// window: `from` excludes the 700-day-old fixture sale, keeps the recent one
	from := time.Now().AddDate(0, 0, -365).Format("2006-01-02")
	r := call(t, "GET", "/sales?storeId="+storeA()+"&from="+from+"&limit=500", tok, nil)
	ids := map[string]bool{}
	for _, d := range r.data() {
		ids[str(d.(M)["id"])] = true
	}
	if !ids[fx.OrderA1.Hex()] || ids[fx.OrderA2.Hex()] {
		t.Fatalf("window filter wrong: %v", ids)
	}
	// old record still reachable by id (lazy load)
	if g := call(t, "GET", "/sales/"+fx.OrderA2.Hex(), tok, nil); g.Code != 200 {
		t.Fatalf("old sale by id: %d", g.Code)
	}
	if bad := call(t, "GET", "/sales?storeId="+storeA()+"&from=yesterday", tok, nil); bad.Code != 400 {
		t.Fatalf("bad from: %d", bad.Code)
	}
	if bad := call(t, "GET", "/sales?storeId="+storeA()+"&limit=0", tok, nil); bad.Code != 400 {
		t.Fatalf("bad limit: %d", bad.Code)
	}
	// includeDeleted
	cr := call(t, "POST", "/customers", tok, M{"storeId": storeA(), "nameEn": "To Delete"})
	id := str(cr.Body["id"])
	if d := call(t, "DELETE", "/customers/"+id, tok, nil); d.Code != 200 || d.Body["deleted"] != true {
		t.Fatalf("delete: %d %s", d.Code, d.Raw)
	}
	without := call(t, "GET", "/customers?storeId="+storeA()+"&limit=500", tok, nil)
	with := call(t, "GET", "/customers?storeId="+storeA()+"&limit=500&includeDeleted=1", tok, nil)
	found := func(r resp) bool {
		for _, d := range r.data() {
			if str(d.(M)["id"]) == id {
				return true
			}
		}
		return false
	}
	if found(without) || !found(with) {
		t.Fatal("includeDeleted semantics")
	}
}

func TestAPI_CRUD_VersionHistoryIfMatchExtras(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	cr := call(t, "POST", "/customers", tok, M{"id": "cus_api_1", "storeId": storeA(), "nameEn": "Api Customer", "phone": "0551231234",
		"category": []string{"Retail"}, "posMeta": M{"src": "pos"}, "version": 99, "deleted": true, "createdAt": "2026-10-01T09:00"},
		"Idempotency-Key", "op_crud_1")
	if cr.Code != 201 {
		t.Fatalf("create: %d %s", cr.Code, cr.Raw)
	}
	id := str(cr.Body["id"])
	if _, ok := oidOf(id); !ok || cr.Body["version"] != 1.0 || cr.Body["deleted"] != false || cr.Body["createdAt"] != "2026-10-01T09:00" {
		t.Fatalf("server-owned fields: %v", cr.Body)
	}
	if get(cr.Body, "posMeta.src") != "pos" || len(arr(cr.Body["category"])) != 1 {
		t.Fatal("unknown body fields must be preserved (rule 26)")
	}
	h := arr(cr.Body["history"])
	if len(h) != 1 || h[0].(M)["action"] != "created" || h[0].(M)["by"] != "Manager A" {
		t.Fatalf("history: %v", h)
	}
	// the client id still resolves (erp.cid alias) and is unique
	if g := call(t, "GET", "/customers/cus_api_1", tok, nil); g.Code != 200 || g.Body["id"] != id {
		t.Fatalf("client id lookup: %d", g.Code)
	}
	if dup := call(t, "POST", "/customers", tok, M{"id": "cus_api_1", "storeId": storeA(), "nameEn": "Dup"}); dup.Code != 409 {
		t.Fatalf("duplicate client id: %d %s", dup.Code, dup.Raw)
	}
	// PATCH with If-Match + X-Change-Reason
	p := call(t, "PATCH", "/customers/"+id, tok, M{"phone2": "0559990000", "posMeta": nil, "version": 1000}, "If-Match", "1", "X-Change-Reason", "status%3A%20vip")
	if p.Code != 200 || p.Body["version"] != 2.0 || p.Body["phone2"] != "0559990000" {
		t.Fatalf("patch: %d %s", p.Code, p.Raw)
	}
	if _, still := p.Body["posMeta"]; still {
		t.Fatal("null must clear a key")
	}
	last := arr(p.Body["history"])[len(arr(p.Body["history"]))-1].(M)
	if last["action"] != "status: vip" || len(arr(last["changes"])) == 0 {
		t.Fatalf("history entry: %v", last)
	}
	// stale If-Match → 409, no write
	if c := call(t, "PATCH", "/customers/"+id, tok, M{"phone2": "0550000000"}, "If-Match", "1"); c.Code != 409 || c.errCode() != "version_conflict" {
		t.Fatalf("conflict: %d %s", c.Code, c.Raw)
	}
	if g := call(t, "GET", "/customers/"+id, tok, nil); g.Body["phone2"] != "0559990000" {
		t.Fatal("conflicting write must not apply")
	}
	// PUT (undo) replaces the record without If-Match
	snap := cr.Body
	pu := call(t, "PUT", "/customers/"+id, tok, snap)
	if pu.Code != 200 || pu.Body["phone2"] != "" || get(pu.Body, "posMeta.src") != "pos" || pu.Body["version"] != 3.0 {
		t.Fatalf("put: %d %v", pu.Code, pu.Body)
	}
	// delete → restore → hard delete
	if d := call(t, "DELETE", "/customers/"+id, tok, nil, "If-Match", "3"); d.Code != 200 || d.Body["deleted"] != true {
		t.Fatalf("delete: %d %s", d.Code, d.Raw)
	}
	if rs := call(t, "POST", "/customers/"+id+"/restore", tok, M{}); rs.Code != 200 || rs.Body["deleted"] != false || lastAction(rs.Body) != "restored" {
		t.Fatalf("restore: %d %s", rs.Code, rs.Raw)
	}
	if hd := call(t, "DELETE", "/customers/"+id+"?hard=1", tok, nil); hd.Code != 204 {
		t.Fatalf("hard delete: %d %s", hd.Code, hd.Raw)
	}
	if g := call(t, "GET", "/customers/"+id, tok, nil); g.Code != 404 {
		t.Fatalf("hard-deleted record must be gone from the adapter: %d", g.Code)
	}
	// …but the legacy document is only soft-deleted (ledger/counters intact)
	oid, _ := oidOf(id)
	var raw bson.M
	ctx, cancel := dbctx()
	defer cancel()
	if err := storeDB(storeA()).Collection("customer").FindOne(ctx, bson.M{"_id": oid}).Decode(&raw); err != nil || raw["deleted"] != true {
		t.Fatalf("legacy doc must survive as deleted: %v %v", err, raw["deleted"])
	}
}

func lastAction(rec M) string {
	h := arr(rec["history"])
	if len(h) == 0 {
		return ""
	}
	return str(h[len(h)-1].(M)["action"])
}

func TestAPI_MalformedAndValidation(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	if r := call(t, "POST", "/customers", tok, "{bad json"); r.Code != 400 || r.errCode() != "malformed_json" {
		t.Fatalf("malformed: %d %s", r.Code, r.Raw)
	}
	cases := []struct {
		path  string
		body  M
		field string
	}{
		{"/customers", M{"storeId": storeA(), "nameEn": "x"}, "nameEn"},
		{"/customers", M{"storeId": storeA(), "nameEn": "Valid", "vatNo": "123"}, "vatNo"},
		{"/products", M{"storeId": storeA(), "nameEn": ""}, "nameEn"},
		{"/products", M{"storeId": storeA(), "nameEn": "Thing", "barcode": "12"}, "barcode"},
		{"/sales", M{"storeId": storeA(), "date": "2026-10-05T10:00", "items": []M{{"productId": fx.ProductA1.Hex(), "qty": 0, "unitPrice": 1}}}, "items.0.qty"},
		{"/sales", M{"storeId": storeA(), "items": []M{}}, "date"},
		{"/sales-returns", M{"storeId": storeA(), "date": "2026-10-05T10:00", "items": []M{{"productId": fx.ProductA1.Hex(), "qty": 1, "unitPrice": 1}}}, "orderId"},
		{"/expenses", M{"storeId": storeA(), "date": "2026-10-05", "amount": 10, "method": "cash"}, "categoryId"},
		{"/warehouses", M{"storeId": storeA(), "nameEn": "W"}, "code"},
		{"/vehicles", M{"storeId": storeA(), "plate": "A 1"}, "make"},
		{"/stock-transfers", M{"storeId": storeA(), "date": "2026-10-05", "fromWarehouseId": "a", "toWarehouseId": "a", "items": []M{{"productId": fx.ProductA1.Hex(), "qty": 1}}}, "toWarehouseId"},
		{"/purchase-bills", M{"storeId": storeA(), "amount": 0}, "vendorId"},
	}
	for _, c := range cases {
		r := call(t, "POST", c.path, tok, c.body)
		if r.Code != 400 || r.errField(c.field) == "" {
			t.Errorf("%s: want 400 with fields.%s, got %d %s", c.path, c.field, r.Code, r.Raw)
		}
	}
	// legacy validation is translated to contract field names (salary_day etc. never leak raw)
	admin := login(t, fx.AdminEmail)
	r := call(t, "PATCH", "/stores/"+storeA(), admin, M{"vatNo": "123"})
	if r.Code != 400 || r.errField("vatNo") == "" {
		t.Fatalf("store validation: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_IdempotencyReplay(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	key := "op_idem_" + time.Now().Format("150405.000000")
	body := M{"storeId": storeA(), "nameEn": "Idem Customer " + key}
	first := call(t, "POST", "/customers", tok, body, "Idempotency-Key", key)
	if first.Code != 201 {
		t.Fatalf("first: %d %s", first.Code, first.Raw)
	}
	second := call(t, "POST", "/customers", tok, body, "Idempotency-Key", key)
	if second.Code != 201 || second.Body["id"] != first.Body["id"] || second.Header.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay: %d %s", second.Code, second.Raw)
	}
	ctx, cancel := dbctx()
	defer cancel()
	n, _ := storeDB(storeA()).Collection("customer").CountDocuments(ctx, bson.M{"name": strings.ToUpper("Idem Customer " + key)})
	if n != 1 {
		t.Fatalf("replay must not re-execute: %d customers", n)
	}
	// a replayed failure stays a failure (non-retryable 4xx is pinned)
	bad := call(t, "POST", "/customers", tok, M{"storeId": storeA(), "nameEn": "x"}, "Idempotency-Key", key+"b")
	again := call(t, "POST", "/customers", tok, M{"storeId": storeA(), "nameEn": "Now Valid"}, "Idempotency-Key", key+"b")
	if bad.Code != 400 || again.Code != 400 {
		t.Fatalf("pinned 4xx: %d %d", bad.Code, again.Code)
	}
	// keys are per user: another user with the same key executes normally
	sales := login(t, fx.SalesEmail)
	other := call(t, "POST", "/customers", sales, M{"storeId": storeA(), "nameEn": "Other User Cust " + key}, "Idempotency-Key", key)
	if other.Code != 201 || other.Body["id"] == first.Body["id"] {
		t.Fatalf("per-user keys: %d %s", other.Code, other.Raw)
	}
}

func TestAPI_StoreIsolation(t *testing.T) {
	requireDB(t)
	b := login(t, fx.UserBEmail)
	if r := call(t, "GET", "/sales?storeId="+storeA(), b, nil); r.Code != 403 {
		t.Fatalf("list foreign store: %d", r.Code)
	}
	if r := call(t, "GET", "/sales/"+fx.OrderA1.Hex(), b, nil); r.Code != 404 {
		t.Fatalf("get foreign id: %d", r.Code)
	}
	if r := call(t, "POST", "/customers", b, M{"storeId": storeA(), "nameEn": "Intruder"}); r.Code != 403 {
		t.Fatalf("create in foreign store: %d", r.Code)
	}
	if r := call(t, "PATCH", "/products/"+fx.ProductA1.Hex(), b, M{"nameEn": "hijack"}); r.Code != 404 {
		t.Fatalf("patch foreign product: %d", r.Code)
	}
	if r := call(t, "DELETE", "/customers/"+fx.CustomerA1.Hex(), b, nil); r.Code != 404 {
		t.Fatalf("delete foreign: %d", r.Code)
	}
	if r := call(t, "GET", "/stores/"+storeA(), b, nil); r.Code != 404 {
		t.Fatalf("foreign store record: %d", r.Code)
	}
	if r := call(t, "PATCH", "/stores/"+storeA(), b, M{"nameEn": "x"}); r.Code != 404 && r.Code != 403 {
		t.Fatalf("foreign store patch: %d", r.Code)
	}
	// own store works; storeId cannot be moved by PATCH
	own := call(t, "GET", "/sales?storeId="+storeB(), b, nil)
	if own.Code != 200 || len(own.data()) != 1 {
		t.Fatalf("own store: %d %s", own.Code, own.Raw)
	}
	// legacy-only fields (string numbers) are read leniently
	it := arr(own.data()[0].(M)["items"])[0].(M)
	if it["unitPrice"] != 10.0 || it["qty"] != 3.0 {
		t.Fatalf("lenient legacy numbers: %v", it)
	}
	// org lists are restricted to the caller's stores
	users := call(t, "GET", "/users", b, nil)
	for _, u := range users.data() {
		if str(u.(M)["email"]) == strings.ToLower(fx.SalesEmail) {
			t.Fatal("user list leaks users of other stores")
		}
	}
}

func TestAPI_RBAC(t *testing.T) {
	requireDB(t)
	sales := login(t, fx.SalesEmail)
	manager := login(t, fx.ManagerEmail)
	cases := []struct {
		tok, method, path string
		body              M
	}{
		{sales, "POST", "/purchases", M{"storeId": storeA()}},
		{sales, "GET", "/expenses?storeId=" + storeA(), nil},
		{sales, "DELETE", "/customers/" + fx.CustomerA1.Hex(), nil},
		{sales, "PATCH", "/stores/" + storeA(), M{"nameEn": "x"}},
		{sales, "POST", "/products", M{"storeId": storeA(), "nameEn": "x"}},
		{manager, "POST", "/users", M{"name": "x"}},
		{manager, "PATCH", "/stores/" + storeA(), M{"nameEn": "x"}},
		{manager, "POST", "/roles", M{"name": "x"}},
		{manager, "POST", "/stores/" + storeA() + "/zatca/connect", M{"otp": "123456"}},
	}
	for _, c := range cases {
		r := call(t, c.method, c.path, c.tok, c.body)
		if r.Code != http.StatusForbidden {
			t.Errorf("%s %s: want 403 got %d %s", c.method, c.path, r.Code, r.Raw)
		}
	}
	// salesman may sell and create customers
	if r := call(t, "GET", "/sales?storeId="+storeA(), sales, nil); r.Code != 200 {
		t.Fatalf("salesman sales list: %d", r.Code)
	}
	if r := call(t, "POST", "/customers", sales, M{"storeId": storeA(), "nameEn": "Sales Created"}); r.Code != 201 {
		t.Fatalf("salesman customer create: %d %s", r.Code, r.Raw)
	}
	// legacy user_role grants are unioned (fixture role grants products create)
	admin := login(t, fx.AdminEmail)
	_ = admin
	me := call(t, "GET", "/auth/me", sales, nil)
	if get(me.Body, "user.perms.inventory.create") != false {
		t.Fatal("salesman has no inventory create without legacy role grants")
	}
}

func TestAPI_RolesAndUsers(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	roles := call(t, "GET", "/roles", admin, nil)
	ids := map[string]M{}
	for _, r := range roles.data() {
		ids[str(r.(M)["id"])] = r.(M)
	}
	for _, sys := range []string{"r_admin", "r_manager", "r_salesman", "r_cashier", "r_accountant", "r_viewer"} {
		if ids[sys] == nil {
			t.Errorf("system role %s missing", sys)
		}
	}
	legacy := ids[fx.LegacyRoleA.Hex()]
	if legacy == nil || get(legacy, "perms.inventory.create") != true || legacy["legacy"] != true {
		t.Fatalf("legacy user_role must be listed with mapped perms: %v", legacy)
	}
	if r := call(t, "PATCH", "/roles/r_admin", admin, M{"name": "x"}); r.Code != 403 {
		t.Fatalf("system role patch: %d", r.Code)
	}
	if r := call(t, "DELETE", "/roles/r_viewer", admin, nil); r.Code != 403 {
		t.Fatalf("system role delete: %d", r.Code)
	}
	if r := call(t, "PATCH", "/roles/"+fx.LegacyRoleA.Hex(), admin, M{"name": "x"}); r.Code != 409 {
		t.Fatalf("legacy role patch: %d", r.Code)
	}
	cr := call(t, "POST", "/roles", admin, M{"id": "rol_api", "name": "Clerk API", "perms": M{"inventory": M{"view": true}}, "maxDiscount": 5})
	if cr.Code != 201 || cr.Body["id"] != "rol_api" || cr.Body["system"] != false {
		t.Fatalf("custom role: %d %s", cr.Code, cr.Raw)
	}
	if dup := call(t, "POST", "/roles", admin, M{"name": "clerk api"}); dup.Code != 400 || dup.errField("name") == "" {
		t.Fatalf("role name unique (ci): %d", dup.Code)
	}
	// user with the custom role
	email := "clerk+" + time.Now().Format("150405.000") + "@t1.example"
	u := call(t, "POST", "/users", admin, M{"name": "Clerk", "email": email, "phone": "0551112223", "role": "rol_api", "storeIds": []string{storeA()}, "password": "Clerk@1234"})
	if u.Code != 201 || u.Body["role"] != "rol_api" {
		t.Fatalf("user with custom role: %d %s", u.Code, u.Raw)
	}
	ctok := str(call(t, "POST", "/auth/login", "", M{"email": email, "password": "Clerk@1234"}).Body["accessToken"])
	if r := call(t, "GET", "/products?storeId="+storeA(), ctok, nil); r.Code != 200 {
		t.Fatalf("custom role view: %d", r.Code)
	}
	if r := call(t, "GET", "/sales?storeId="+storeA(), ctok, nil); r.Code != 403 {
		t.Fatalf("custom role denies sales: %d", r.Code)
	}
	// e-mail unique ignoring case; own role is immutable
	if r := call(t, "POST", "/users", admin, M{"name": "Dup", "email": strings.ToUpper(email), "phone": "0551112224", "role": "r_viewer", "storeIds": []string{storeA()}}); r.Code != 400 || r.errField("email") == "" {
		t.Fatalf("email unique: %d %s", r.Code, r.Raw)
	}
	mgr := login(t, fx.ManagerEmail)
	me := call(t, "GET", "/auth/me", mgr, nil)
	_ = me
	if r := call(t, "PATCH", "/users/"+fx.Admin.Hex(), admin, M{"role": "r_viewer"}); r.Code != 403 {
		t.Fatalf("own role change: %d %s", r.Code, r.Raw)
	}
}

func TestAPI_StoresPatch(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	g := call(t, "GET", "/stores/"+storeA(), admin, nil)
	if g.Code != 200 {
		t.Fatalf("get store: %d", g.Code)
	}
	v := str(g.Body["version"])
	r := call(t, "PATCH", "/stores/"+storeA(), admin, M{"short": "ALN", "branchAr": "العليا", "phone2": "0551112222",
		"zatca": M{"phase": 2, "connected": true, "pcsid": "forged"}, "flags": M{"enable_rbac_module": true, "enable_signature": true},
		"titles": M{"invoiceEn": "Tax Invoice", "debitNoteEn": "Debit Note"}}, "If-Match", v, "X-Change-Reason", "settings")
	if r.Code != 200 {
		t.Fatalf("patch store: %d %s", r.Code, r.Raw)
	}
	if r.Body["short"] != "ALN" || r.Body["branchAr"] != "العليا" || get(r.Body, "zatca.connected") != false || get(r.Body, "zatca.pcsid") == "forged" {
		t.Fatalf("store patch result: short=%v zatca=%v", r.Body["short"], r.Body["zatca"])
	}
	if get(r.Body, "flags.enable_rbac_module") != true || get(r.Body, "flags.enable_signature") != true {
		t.Fatalf("flags (legacy-mapped + preserved): %v", r.Body["flags"])
	}
	// legacy settings.enable_rbac_module was written (old app sees it)
	oid, _ := oidOf(storeA())
	var raw bson.M
	ctx, cancel := dbctx()
	defer cancel()
	_ = mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid}).Decode(&raw)
	d := normDoc(raw)
	if get(d, "settings.enable_rbac_module") != true || get(d, "zatca.connected") == true || get(d, "erp.x.short") != "ALN" {
		t.Fatalf("legacy store doc: settings=%v zatca=%v", get(d, "settings.enable_rbac_module"), get(d, "zatca"))
	}
	// a platform admin may add stores (categories_api_test.go); an incomplete body is a 400
	if r := call(t, "POST", "/stores", admin, M{"nameEn": "x"}); r.Code != 400 || r.errField("category") == "" {
		t.Fatalf("incomplete store create: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/stores/"+storeA(), admin, nil); r.Code != 403 {
		t.Fatalf("store delete: %d", r.Code)
	}
}

// Settings → WhatsApp round trip: the default channel, connection status and
// WABA details are preserved state and must survive a reload; the mapped
// evolution fields go to legacy settings with the key masked.
func TestAPI_StoresPatch_WhatsappRoundTrip(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	g := call(t, "GET", "/stores/"+storeA(), admin, nil)
	if g.Code != 200 {
		t.Fatalf("get store: %d", g.Code)
	}
	if p := get(g.Body, "emailSettings.smtp.port"); p != nil && num(p) < 1 {
		t.Fatalf("unset smtp port must be null, got %#v", p)
	}
	wa := M{"mode": "waba",
		"evolution": M{"url": "https://wa.example", "instance": "shop1", "apiKey": "evo-secret", "status": "connected"},
		"waba":      M{"phoneNumberId": "1098765432101", "businessAccountId": "2233445566778", "token": "t", "verified": true}}
	r := call(t, "PATCH", "/stores/"+storeA(), admin, M{"whatsapp": wa}, "If-Match", str(g.Body["version"]), "X-Change-Reason", "settings")
	if r.Code != 200 {
		t.Fatalf("patch store: %d %s", r.Code, r.Raw)
	}
	g2 := call(t, "GET", "/stores/"+storeA(), admin, nil)
	for path, want := range map[string]interface{}{
		"whatsapp.mode": "waba", "whatsapp.evolution.status": "connected", "whatsapp.evolution.url": "https://wa.example",
		"whatsapp.evolution.instance": "shop1", "whatsapp.evolution.apiKey": masked, "whatsapp.waba.verified": true,
		"whatsapp.waba.phoneNumberId": "1098765432101",
	} {
		if got := get(g2.Body, path); got != want {
			t.Errorf("%s=%#v want %#v", path, got, want)
		}
	}
	oid, _ := oidOf(storeA())
	var raw bson.M
	ctx, cancel := dbctx()
	defer cancel()
	_ = mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid}).Decode(&raw)
	d := normDoc(raw)
	if get(d, "settings.evolution_api_url") != "https://wa.example" || get(d, "settings.evolution_api_key") != "evo-secret" {
		t.Fatalf("legacy evolution settings: %v", get(d, "settings"))
	}
	// re-saving with the masked key keeps the real key
	r = call(t, "PATCH", "/stores/"+storeA(), admin, M{"whatsapp": M{"mode": "evolution", "evolution": M{"url": "https://wa2.example", "instance": "shop1", "apiKey": masked}}},
		"If-Match", str(r.Body["version"]), "X-Change-Reason", "settings")
	if r.Code != 200 || get(r.Body, "whatsapp.mode") != "evolution" {
		t.Fatalf("second patch: %d mode=%v", r.Code, get(r.Body, "whatsapp.mode"))
	}
	_ = mainDB().Collection("store").FindOne(ctx, bson.M{"_id": oid}).Decode(&raw)
	if get(normDoc(raw), "settings.evolution_api_key") != "evo-secret" {
		t.Fatalf("masked key overwrote the real one")
	}
}

func TestAPI_UnsupportedLegacyOperations(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	if r := call(t, "DELETE", "/sales/"+fx.OrderA1.Hex(), tok, nil); r.Code != 409 || r.errCode() != "unsupported_legacy" {
		t.Fatalf("sales delete: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/delivery-notes/"+fx.DeliveryNoteA1.Hex(), tok, nil); r.Code != 409 {
		t.Fatalf("delivery note delete: %d", r.Code)
	}
	if r := call(t, "PATCH", "/warehouses/ms_"+storeA(), tok, M{"nameEn": "x"}); r.Code != 409 {
		t.Fatalf("virtual warehouse: %d", r.Code)
	}
	if r := call(t, "POST", "/accounts", tok, M{"nameEn": "x"}); r.Code != 403 {
		t.Fatalf("accounts read-only: %d", r.Code)
	}
}

// ---------------- ZATCA actions (external call stubbed) ----------------

func TestAPI_ZatcaActions(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	tok := login(t, fx.ManagerEmail)
	// store not connected → 409
	if r := call(t, "POST", "/sales/"+fx.OrderA2.Hex()+"/zatca/report", tok, M{}); r.Code != 409 || r.errCode() != "zatca_not_connected" {
		t.Fatalf("not connected: %d %s", r.Code, r.Raw)
	}
	// invalid OTP format → 400 invalid_otp, fields.otp
	if r := call(t, "POST", "/stores/"+storeA()+"/zatca/connect", admin, M{"otp": "12"}); r.Code != 400 || r.errCode() != "invalid_otp" || r.errField("otp") == "" {
		t.Fatalf("otp: %d %s", r.Code, r.Raw)
	}
	// stub the legacy onboarding (no python/ZATCA calls in tests)
	origConnect, origDisconnect := zatcaConnectHandler, zatcaDisconnectHandler
	origReport := zatcaReporters["sales"]
	defer func() {
		zatcaConnectHandler, zatcaDisconnectHandler = origConnect, origDisconnect
		zatcaReporters["sales"] = origReport
	}()
	sid, _ := oidOf(storeA())
	zatcaConnectHandler = func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": sid}, bson.M{"$set": bson.M{"zatca.connected": true, "zatca.phase": "2",
			"zatca.production_request_id": int64(777), "zatca.last_connected_at": time.Now(),
			"zatca.zatca_reconnect_required": false}}) // like controller.ConnectStoreToZatca
		writeJSON(w, 200, M{"status": true, "result": "connected"})
	}
	c := call(t, "POST", "/stores/"+storeA()+"/zatca/connect", admin, M{"otp": "123456"})
	if c.Code != 200 || get(c.Body, "zatca.connected") != true || get(c.Body, "zatca.pcsid") != "777" || str(get(c.Body, "zatca.snapshot")) == "" {
		t.Fatalf("connect: %d %s", c.Code, c.Raw)
	}
	// reporting: the legacy reporter (stubbed) marks the invoice reported
	var reportedID string
	zatcaReporters["sales"] = func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/order/zatca/report/")
		reportedID = id
		oid, _ := oidOf(id)
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = storeDB(storeA()).Collection("order").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
			"zatca.reporting_passed": true, "zatca.is_simplified": true, "zatca.qr_code": "STUBQR", "hash": "STUBHASH",
			"zatca.reporting_passed_at": time.Now()}})
		writeJSON(w, 200, M{"status": true, "result": "ok"})
	}
	rep := call(t, "POST", "/sales/"+fx.OrderA2.Hex()+"/zatca/report", tok, M{}, "Idempotency-Key", "op_zatca_1")
	if rep.Code != 200 || get(rep.Body, "zatca.status") != "reported" || get(rep.Body, "zatca.qr") != "STUBQR" || reportedID != fx.OrderA2.Hex() {
		t.Fatalf("report: %d %s", rep.Code, rep.Raw)
	}
	// already reported → returned as-is, legacy reporter not called again (no re-chaining)
	reportedID = ""
	again := call(t, "POST", "/sales/"+fx.OrderA2.Hex()+"/zatca/report", tok, M{})
	if again.Code != 200 || reportedID != "" {
		t.Fatalf("idempotent report: %d called=%q", again.Code, reportedID)
	}
	// changing a ZATCA-sensitive store field (here the Arabic street name)
	// marks the connected Phase 2 store for re-connection
	gs := call(t, "GET", "/stores/"+storeA(), admin, nil)
	addr := cloneM(sub(gs.Body, "address"))
	addr["streetAr"] = "شارع العليا الجديد"
	ps := call(t, "PATCH", "/stores/"+storeA(), admin, M{"address": addr}, "If-Match", str(gs.Body["version"]), "X-Change-Reason", "address")
	if ps.Code != 200 || get(ps.Body, "zatca.reconnectNeeded") != true {
		t.Fatalf("sensitive change must mark reconnect: %d %s", ps.Code, ps.Raw)
	}
	// a non-sensitive change keeps the mark (only a reconnect clears it)
	gs = call(t, "GET", "/stores/"+storeA(), admin, nil)
	ps = call(t, "PATCH", "/stores/"+storeA(), admin, M{"email": "ops@example.com"}, "If-Match", str(gs.Body["version"]), "X-Change-Reason", "email")
	if ps.Code != 200 || get(ps.Body, "zatca.reconnectNeeded") != true {
		t.Fatalf("reconnect mark must persist: %d %s", ps.Code, ps.Raw)
	}
	// sales, sales returns, debit notes (deposits) and credit notes (withdrawals)
	// are all refused until the store reconnects
	reportedID = ""
	for _, u := range []string{"/sales/" + fx.OrderA1.Hex(), "/sales-returns/" + fx.SalesReturnA1.Hex(),
		"/deposits/" + fx.DepositA1.Hex(), "/withdrawals/" + fx.WithdrawalA1.Hex()} {
		if r := call(t, "POST", u+"/zatca/report", tok, M{}); r.Code != 409 || r.errCode() != "zatca_reconnect_required" {
			t.Fatalf("%s while reconnect required: %d %s", u, r.Code, r.Raw)
		}
	}
	if reportedID != "" {
		t.Fatalf("legacy reporter must not run while reconnect is required")
	}
	// reconnecting with a new OTP clears the mark and reporting works again
	c2 := call(t, "POST", "/stores/"+storeA()+"/zatca/connect", admin, M{"otp": "654321"})
	if c2.Code != 200 || get(c2.Body, "zatca.reconnectNeeded") != false {
		t.Fatalf("reconnect: %d %s", c2.Code, c2.Raw)
	}
	// (OrderA1 is already cleared in the fixture, so it comes back as-is)
	if r := call(t, "POST", "/sales/"+fx.OrderA1.Hex()+"/zatca/report", tok, M{}); r.Code != 200 || get(r.Body, "zatca.status") != "cleared" {
		t.Fatalf("report after reconnect: %d %s", r.Code, r.Raw)
	}
	// failure path: legacy error → 200 with zatca.status failed + message
	zatcaReporters["sales"] = func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 400, M{"status": false, "errors": M{"reporting_to_zatca": "BR-KSA-44: invalid buyer VAT"}})
	}
	sale := call(t, "POST", "/sales", tok, M{"storeId": storeA(), "date": "2026-10-05T10:00", "items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 25, "warehouseId": "ms_" + storeA()}}})
	if sale.Code != 201 {
		t.Fatalf("sale: %d %s", sale.Code, sale.Raw)
	}
	f := call(t, "POST", "/sales/"+str(sale.Body["id"])+"/zatca/report", tok, M{})
	if f.Code != 200 || get(f.Body, "zatca.status") != "failed" || !strings.Contains(str(get(f.Body, "zatca.error")), "BR-KSA-44") {
		t.Fatalf("failed report: %d %s", f.Code, f.Raw)
	}
	// rule 61: client-sent zatca status is never trusted
	pos := call(t, "POST", "/sales", tok, M{"storeId": storeA(), "date": "2026-10-05T10:05", "zatca": M{"status": "cleared", "uuid": "pos-1234"},
		"items": []M{{"productId": fx.ProductA2.Hex(), "qty": 1, "unitPrice": 25, "warehouseId": "ms_" + storeA()}}})
	if get(pos.Body, "zatca.status") != "not_reported" || get(pos.Body, "zatca.uuid") == "pos-1234" {
		t.Fatalf("POS zatca must be server-owned: %v", pos.Body["zatca"])
	}
	// disconnect (stubbed legacy)
	zatcaDisconnectHandler = func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := dbctx()
		defer cancel()
		_, _ = mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": sid}, bson.M{"$set": bson.M{"zatca.connected": false}})
		writeJSON(w, 200, M{"status": true, "result": "disconnected"})
	}
	dc := call(t, "POST", "/stores/"+storeA()+"/zatca/disconnect", admin, M{})
	if dc.Code != 200 || get(dc.Body, "zatca.connected") != false || get(dc.Body, "zatca.pcsid") != nil || str(get(dc.Body, "zatca.disconnectedAt")) == "" {
		t.Fatalf("disconnect: %d %s", dc.Code, dc.Raw)
	}
}
