package erp

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed tests of the Card Bridge: pairing, the job queue, payments
// through a bridge, cancelling, late approvals and unpairing. The test plays
// the bridge's part over HTTP like cardbridge/ does.

func enableProvider(t *testing.T, admin, id string) {
	t.Helper()
	body := M{"enabled": true, "countries": []interface{}{"SA", "AE", "OM", "QA", "BH", "KW", "IN"}, "liveAllowed": true}
	if r := call(t, "PUT", "/admin/card-terminal-providers/"+id, admin, body); r.Code != 200 {
		t.Fatalf("enable %s: %d %s", id, r.Code, r.Raw)
	}
}

func resetBridges(t *testing.T) {
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = bridgePairingOf().DeleteMany(ctx, bson.M{})
	_, _ = bridgeTokensOf().DeleteMany(ctx, bson.M{})
}

// pairBridge makes a code as the store owner and pairs a bridge with it.
func pairBridge(t *testing.T, owner, sid, name string) (token, bridgeID string) {
	t.Helper()
	pc := call(t, "POST", "/card-bridges/pairing-code", owner, M{"storeId": sid})
	if pc.Code != 201 || len(str(pc.Body["code"])) != 9 || int(num(pc.Body["expiresInSeconds"])) != 900 {
		t.Fatalf("pairing code: %d %s", pc.Code, pc.Raw)
	}
	pr := call(t, "POST", "/card-bridge/pair", "", M{"code": strings.ToLower(str(pc.Body["code"])), "name": name, "os": "windows",
		"arch": "amd64", "version": "1.0.0", "drivers": []interface{}{"simulator", "neoleap-ws"}})
	if pr.Code != 201 || !strings.HasPrefix(str(pr.Body["token"]), "cb1_") || pr.Body["storeId"] != sid {
		t.Fatalf("pair: %d %s", pr.Code, pr.Raw)
	}
	// single use
	if again := call(t, "POST", "/card-bridge/pair", "", M{"code": str(pc.Body["code"])}); again.Code != 400 || again.errCode() != "pairing_code" {
		t.Fatalf("code reused: %d %s", again.Code, again.Raw)
	}
	return str(pr.Body["token"]), str(pr.Body["bridgeId"])
}

func bridgeCall(t *testing.T, method, path, token string, body interface{}) resp {
	t.Helper()
	return call(t, method, path, "", body, "X-Card-Bridge-Token", token)
}

func TestAPI_CardBridge_FullFlow(t *testing.T) {
	requireDB(t)
	resetTerminalProviders(t)
	resetBridges(t)
	defer resetTerminalProviders(t)
	defer resetBridges(t)
	oldCancel, oldCheck, oldPoll := bridgeCancelWait, bridgeCheckWait, bridgePollEvery
	bridgeCancelWait, bridgeCheckWait, bridgePollEvery = 3*time.Second, 3*time.Second, 50*time.Millisecond
	defer func() { bridgeCancelWait, bridgeCheckWait, bridgePollEvery = oldCancel, oldCheck, oldPoll }()

	admin := login(t, fx.AdminEmail)
	owner, sid := signupOwner(t, "cardbridge")
	defer cleanupStore(t, sid)
	enableProvider(t, admin, "bridge-simulator")

	// providers: the Card Bridge ones say so
	pr := call(t, "GET", "/card-terminals/providers?storeId="+sid, owner, nil)
	conn := map[string]string{}
	for _, it := range arr(pr.Body["items"]) {
		conn[str(it.(M)["id"])] = str(it.(M)["connect"])
	}
	if conn["neoleap"] != "bridge" || conn["bridge-simulator"] != "bridge" || conn["nearpay"] != "api" || conn["snb"] != "manual" {
		t.Fatalf("connect kinds: %v", conn)
	}

	// pairing is for settings editors of this store only
	other := login(t, fx.ManagerEmail)
	if r := call(t, "POST", "/card-bridges/pairing-code", other, M{"storeId": sid}); r.Code != 403 {
		t.Fatalf("other store code: %d", r.Code)
	}
	if r := call(t, "POST", "/card-bridge/pair", "", M{"code": "ABCD-EFGH"}); r.Code != 400 || r.errCode() != "pairing_code" {
		t.Fatalf("unknown code: %d %s", r.Code, r.Raw)
	}
	tok, bid := pairBridge(t, owner, sid, "Front desk PC")
	ls := call(t, "GET", "/card-bridges?storeId="+sid, owner, nil)
	if ls.Code != 200 || int(num(ls.Body["total"])) != 1 {
		t.Fatalf("bridges: %d %s", ls.Code, ls.Raw)
	}
	b0 := arr(ls.Body["items"])[0].(M)
	if b0["name"] != "Front desk PC" || b0["os"] != "windows" || b0["online"] != true || len(arr(b0["drivers"])) != 2 {
		t.Fatalf("bridge row: %v", b0)
	}
	if r := call(t, "GET", "/card-bridges?storeId="+sid, other, nil); r.Code != 403 {
		t.Fatalf("other store list: %d", r.Code)
	}
	if h := bridgeCall(t, "POST", "/card-bridge/hello", tok, M{"name": "Front desk", "version": "1.0.1", "drivers": []interface{}{"simulator"}}); h.Code != 200 || h.Body["storeId"] != sid {
		t.Fatalf("hello: %d %s", h.Code, h.Raw)
	}
	// nothing queued: 204 at once
	if j := bridgeCall(t, "GET", "/card-bridge/jobs?wait=0", tok, nil); j.Code != 204 {
		t.Fatalf("empty queue: %d %s", j.Code, j.Raw)
	}

	// a machine through the bridge
	if r := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Neo", "provider": "neoleap", "bridgeId": bid, "fields": M{}}); r.Code != 400 || r.errField("fields.host") == "" {
		t.Fatalf("neoleap needs the machine's IP: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Neo", "provider": "neoleap", "fields": M{"host": "10.0.0.5"}}); r.Code != 400 || r.errField("bridgeId") == "" {
		t.Fatalf("no bridge chosen: %d %v", r.Code, r.Body)
	}
	if r := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Neo", "provider": "neoleap", "bridgeId": "000000000000000000000000", "fields": M{"host": "10.0.0.5"}}); r.Code != 400 || r.errField("bridgeId") == "" {
		t.Fatalf("unknown bridge: %d %s", r.Code, r.Raw)
	}
	neo := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Neo", "provider": "neoleap", "bridgeId": bid, "mode": "live", "fields": M{"host": "10.0.0.5", "terminalId": "N1"}})
	if neo.Code != 201 || neo.Body["connect"] != "bridge" || neo.Body["bridgeId"] != bid || neo.Body["driver"] != "neoleap-ws" {
		t.Fatalf("neoleap machine: %d %s", neo.Code, neo.Raw)
	}
	cr := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Bridge test", "provider": "bridge-simulator", "bridgeId": bid, "isDefault": true})
	if cr.Code != 201 || cr.Body["mode"] != "test" || cr.Body["connect"] != "bridge" {
		t.Fatalf("bridge test machine: %d %s", cr.Code, cr.Raw)
	}
	tid := str(cr.Body["id"])

	// check: the bridge answers the check job
	done := make(chan resp, 1)
	go func() { done <- call(t, "POST", "/card-terminals/"+tid+"/check", owner, M{"storeId": sid}) }()
	j := bridgeCall(t, "GET", "/card-bridge/jobs?wait=5", tok, nil)
	if j.Code != 200 || get(j.Body, "job.op") != "check" || get(j.Body, "job.driver") != "simulator" {
		t.Fatalf("check job: %d %s", j.Code, j.Raw)
	}
	if r := bridgeCall(t, "POST", "/card-bridge/jobs/"+str(get(j.Body, "job.id"))+"/result", tok, M{"status": "approved", "ok": true, "message": "reached"}); r.Code != 200 {
		t.Fatalf("check result: %d %s", r.Code, r.Raw)
	}
	if ck := <-done; ck.Code != 200 || ck.Body["ok"] != true {
		t.Fatalf("check: %d %s", ck.Code, ck.Raw)
	}

	// pay → the bridge gets the job with the machine's settings → approved
	p1 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 25.5, "reference": "POS-77"})
	if p1.Code != 201 || p1.Body["status"] != "pending" || !strings.HasPrefix(str(p1.Body["providerRef"]), "bridge_") {
		t.Fatalf("start: %d %s", p1.Code, p1.Raw)
	}
	pid := str(p1.Body["id"])
	j = bridgeCall(t, "GET", "/card-bridge/jobs?wait=2", tok, nil)
	job := sub(j.Body, "job")
	if j.Code != 200 || job["op"] != "pay" || job["paymentId"] != pid || num(job["minor"]) != 2550 || job["currency"] != "SAR" || job["reference"] != "POS-77" || job["terminalName"] != "Bridge test" {
		t.Fatalf("pay job: %d %s", j.Code, j.Raw)
	}
	jid := str(job["id"])
	if r := bridgeCall(t, "POST", "/card-bridge/jobs/"+jid+"/result", tok, M{"status": "pending", "message": "Insert card"}); r.Code != 200 {
		t.Fatalf("progress: %d", r.Code)
	}
	if g := call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil); g.Body["status"] != "pending" || g.Body["message"] != "Insert card" {
		t.Fatalf("progress shown: %s", g.Raw)
	}
	if r := bridgeCall(t, "POST", "/card-bridge/jobs/"+jid+"/result", tok, M{"status": "bogus"}); r.Code != 400 {
		t.Fatalf("bad status: %d", r.Code)
	}
	if r := bridgeCall(t, "POST", "/card-bridge/jobs/"+jid+"/result", tok, M{"status": "approved", "authCode": "B12345", "rrn": "000111222333", "maskedPan": "••••4242", "scheme": "mada"}); r.Code != 200 {
		t.Fatalf("result: %d %s", r.Code, r.Raw)
	}
	if g := call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil); g.Body["status"] != "approved" || g.Body["authCode"] != "B12345" || g.Body["test"] != true {
		t.Fatalf("approved: %s", g.Raw)
	}
	if r := bridgeCall(t, "POST", "/card-bridge/jobs/"+jid+"/result", tok, M{"status": "declined"}); r.Code != 200 || r.Body["duplicate"] != true {
		t.Fatalf("second answer ignored: %d %s", r.Code, r.Raw)
	}

	// cancelled before the bridge picked it up: never reaches the machine
	p2 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 3.0})
	if c := call(t, "POST", "/card-terminal-payments/"+str(p2.Body["id"])+"/cancel", owner, M{"storeId": sid}); c.Code != 200 || c.Body["status"] != "cancelled" {
		t.Fatalf("cancel queued: %d %s", c.Code, c.Raw)
	}
	if j := bridgeCall(t, "GET", "/card-bridge/jobs?wait=0", tok, nil); j.Code != 204 {
		t.Fatalf("cancelled job must not be delivered: %d %s", j.Code, j.Raw)
	}

	// cancelled while on the machine: the bridge gets a cancel job, the machine answers cancelled
	p3 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 4.0})
	p3id := str(p3.Body["id"])
	pay3 := sub(bridgeCall(t, "GET", "/card-bridge/jobs?wait=2", tok, nil).Body, "job")
	go func() { done <- call(t, "POST", "/card-terminal-payments/"+p3id+"/cancel", owner, M{"storeId": sid}) }()
	cj := sub(bridgeCall(t, "GET", "/card-bridge/jobs?wait=3", tok, nil).Body, "job")
	if cj["op"] != "cancel" || cj["ofJob"] != pay3["id"] {
		t.Fatalf("cancel job: %v", cj)
	}
	_ = bridgeCall(t, "POST", "/card-bridge/jobs/"+str(cj["id"])+"/result", tok, M{"status": "cancelled", "ok": true})
	_ = bridgeCall(t, "POST", "/card-bridge/jobs/"+str(pay3["id"])+"/result", tok, M{"status": "cancelled", "message": "Cancelled on the card machine."})
	if c := <-done; c.Code != 200 || c.Body["status"] != "cancelled" {
		t.Fatalf("cancel on machine: %d %s", c.Code, c.Raw)
	}

	// the machine never confirms: cancel_failed (press Cancel on the machine), payment stays pending
	p4 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 5.0})
	p4id := str(p4.Body["id"])
	pay4 := sub(bridgeCall(t, "GET", "/card-bridge/jobs?wait=2", tok, nil).Body, "job")
	if c := call(t, "POST", "/card-terminal-payments/"+p4id+"/cancel", owner, M{"storeId": sid}); c.Code != 502 || c.errCode() != "cancel_failed" {
		t.Fatalf("unconfirmed cancel: %d %s", c.Code, c.Raw)
	}
	_ = bridgeCall(t, "GET", "/card-bridge/jobs?wait=0", tok, nil) // the cancel job
	// time-out on the till, then the card is approved after all: kept as lateApproval
	old := nowFn
	defer func() { nowFn = old }()
	t0 := time.Now()
	nowFn = func() time.Time { return t0.Add(terminalTimeout + 10*time.Second) }
	// the bridge is "offline" at that time, so the time-out cancel cannot reach it
	g := call(t, "GET", "/card-terminal-payments/"+p4id+"?storeId="+sid, owner, nil)
	if g.Body["status"] != "timeout" {
		t.Fatalf("time-out: %s", g.Raw)
	}
	nowFn = old
	_ = bridgeCall(t, "POST", "/card-bridge/jobs/"+str(pay4["id"])+"/result", tok, M{"status": "approved", "authCode": "LATE01"})
	ctx, cancel := dbctx()
	var raw bson.M
	oid, _ := oidOf(p4id)
	_ = terminalPaymentsOf(sid).FindOne(ctx, bson.M{"_id": oid}).Decode(&raw)
	cancel()
	if d := normDoc(raw); d["status"] != "timeout" || str(get(d, "lateApproval.authCode")) != "LATE01" {
		t.Fatalf("late approval: %v", d)
	}

	// offline bridge: payments fail at once with a clear message
	ctx, cancel = dbctx()
	_, _ = bridgesOf(sid).UpdateOne(ctx, bson.M{}, bson.M{"$set": bson.M{"lastSeenMs": time.Now().Add(-5 * time.Minute).UnixMilli()}})
	cancel()
	if p := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 6.0}); p.Code != 201 || p.Body["status"] != "failed" || !strings.Contains(str(p.Body["message"]), "offline") {
		t.Fatalf("offline: %d %s", p.Code, p.Raw)
	}
	if ck := call(t, "POST", "/card-terminals/"+tid+"/check", owner, M{"storeId": sid}); ck.Body["ok"] != false || !strings.Contains(str(ck.Body["message"]), "offline") {
		t.Fatalf("offline check: %s", ck.Raw)
	}

	// another store's token cannot answer this store's jobs
	owner2, sid2 := signupOwner(t, "cardbridge2")
	defer cleanupStore(t, sid2)
	tok2, _ := pairBridge(t, owner2, sid2, "Other shop")
	if r := bridgeCall(t, "POST", "/card-bridge/jobs/"+jid+"/result", tok2, M{"status": "approved"}); r.Code != 404 {
		t.Fatalf("foreign job: %d", r.Code)
	}

	// unpair: the token stops working, the machine's queued jobs fail
	if r := call(t, "DELETE", "/card-bridges/"+bid+"?storeId="+sid, owner, nil); r.Code != 200 {
		t.Fatalf("unpair: %d %s", r.Code, r.Raw)
	}
	if r := bridgeCall(t, "GET", "/card-bridge/jobs?wait=0", tok, nil); r.Code != 401 {
		t.Fatalf("unpaired token: %d", r.Code)
	}
	if r := call(t, "GET", "/card-bridges?storeId="+sid, owner, nil); int(num(r.Body["total"])) != 0 {
		t.Fatalf("unpaired list: %s", r.Raw)
	}
	if p := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 7.0}); p.Body["status"] != "failed" || !strings.Contains(str(p.Body["message"]), "unpaired") {
		t.Fatalf("after unpair: %s", p.Raw)
	}
}

func TestAPI_CardBridge_ExpiredCodeAndLongPoll(t *testing.T) {
	requireDB(t)
	resetBridges(t)
	defer resetBridges(t)
	oldPoll := bridgePollEvery
	bridgePollEvery = 50 * time.Millisecond
	defer func() { bridgePollEvery = oldPoll }()
	owner, sid := signupOwner(t, "cardbridge3")
	defer cleanupStore(t, sid)
	pc := call(t, "POST", "/card-bridges/pairing-code", owner, M{"storeId": sid})
	old := nowFn
	nowFn = func() time.Time { return time.Now().Add(bridgePairingTTL + time.Minute) }
	r := call(t, "POST", "/card-bridge/pair", "", M{"code": str(pc.Body["code"])})
	nowFn = old
	if r.Code != 400 || !strings.Contains(r.Raw, "expired") {
		t.Fatalf("expired code: %d %s", r.Code, r.Raw)
	}
	// a new code replaces the previous one
	a := call(t, "POST", "/card-bridges/pairing-code", owner, M{"storeId": sid})
	b := call(t, "POST", "/card-bridges/pairing-code", owner, M{"storeId": sid})
	if r := call(t, "POST", "/card-bridge/pair", "", M{"code": str(a.Body["code"])}); r.Code != 400 {
		t.Fatalf("replaced code still works: %d", r.Code)
	}
	tok := str(call(t, "POST", "/card-bridge/pair", "", M{"code": str(b.Body["code"]), "name": "PC"}).Body["token"])
	start := time.Now()
	if j := bridgeCall(t, "GET", "/card-bridge/jobs?wait=1", tok, nil); j.Code != 204 || time.Since(start) < 900*time.Millisecond {
		t.Fatalf("long-poll should wait: %d after %v", j.Code, time.Since(start))
	}
}
