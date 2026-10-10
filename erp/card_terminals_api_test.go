package erp

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed API + integration tests for card terminals, driven through the
// StartERP test terminal (simulator adapter).

func enableSimulator(t *testing.T, admin string, body M) {
	t.Helper()
	if body == nil {
		body = M{"enabled": true, "countries": []interface{}{"SA", "AE", "OM", "QA", "BH", "KW", "IN"}, "liveAllowed": true}
	}
	if r := call(t, "PUT", "/admin/card-terminal-providers/simulator", admin, body); r.Code != 200 {
		t.Fatalf("enable simulator: %d %s", r.Code, r.Raw)
	}
}

func resetTerminalProviders(t *testing.T) {
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = terminalProviderConfigs().DeleteMany(ctx, bson.M{})
}

func TestAPI_CardTerminals_FullFlow(t *testing.T) {
	requireDB(t)
	resetTerminalProviders(t)
	defer resetTerminalProviders(t)
	admin := login(t, fx.AdminEmail)
	owner, sid := signupOwner(t, "terminals")
	defer cleanupStore(t, sid)

	// simulator is off by default: not offered to the store
	pr := call(t, "GET", "/card-terminals/providers?storeId="+sid, owner, nil)
	if pr.Code != 200 || pr.Body["countryCode"] != "SA" {
		t.Fatalf("providers: %d %s", pr.Code, pr.Raw)
	}
	for _, it := range arr(pr.Body["items"]) {
		if str(it.(M)["id"]) == "simulator" {
			t.Fatal("simulator must be off by default")
		}
		if _, leaked := it.(M)["partnerFields"]; leaked {
			t.Fatal("stores must not see partner fields")
		}
	}
	if r := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Till 1", "provider": "simulator", "fields": M{"terminalId": "T1"}}); r.Code != 400 || r.errField("provider") == "" {
		t.Fatalf("disabled provider must be refused: %d %s", r.Code, r.Raw)
	}

	// only platform admins manage providers
	if r := call(t, "GET", "/admin/card-terminal-providers", owner, nil); r.Code != 403 {
		t.Fatalf("owner admin list: %d", r.Code)
	}
	if r := call(t, "PUT", "/admin/card-terminal-providers/simulator", owner, M{"enabled": true}); r.Code != 403 {
		t.Fatalf("owner admin save: %d", r.Code)
	}
	if r := call(t, "PUT", "/admin/card-terminal-providers/nope", admin, M{"enabled": true}); r.Code != 404 {
		t.Fatalf("unknown provider: %d", r.Code)
	}
	for _, bad := range []M{{"enabled": "yes"}, {"countries": "SA"}, {"countries": []interface{}{"FR"}}, {"liveAllowed": 1.0}, {"partner": "x"}} {
		if r := call(t, "PUT", "/admin/card-terminal-providers/simulator", admin, bad); r.Code != 400 {
			t.Fatalf("bad admin body %v: %d %s", bad, r.Code, r.Raw)
		}
	}
	enableSimulator(t, admin, nil)
	al := call(t, "GET", "/admin/card-terminal-providers", admin, nil)
	if al.Code != 200 || int(num(al.Body["total"])) != len(terminalProviders) {
		t.Fatalf("admin list: %d %s", al.Code, al.Raw)
	}

	// add the machine
	bad := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "", "provider": "simulator", "fields": M{}})
	if bad.Code != 400 || bad.errField("name") == "" || bad.errField("fields.terminalId") == "" {
		t.Fatalf("validation: %d %s", bad.Code, bad.Raw)
	}
	cr := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Front till", "provider": "simulator", "mode": "live", "fields": M{"terminalId": "TEST-1"}})
	if cr.Code != 201 || cr.Body["mode"] != "test" || cr.Body["isDefault"] != true || cr.Body["active"] != true || cr.Body["terminalId"] != "TEST-1" || cr.Body["connect"] != "api" {
		t.Fatalf("create: %d %s", cr.Code, cr.Raw)
	}
	tid := str(cr.Body["id"])
	cr2 := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Back till", "provider": "simulator", "isDefault": true, "fields": M{"terminalId": "TEST-2"}})
	if cr2.Code != 201 || cr2.Body["isDefault"] != true {
		t.Fatalf("second: %d %s", cr2.Code, cr2.Raw)
	}
	ls := call(t, "GET", "/card-terminals?storeId="+sid, owner, nil)
	defaults := 0
	for _, it := range arr(ls.Body["items"]) {
		if it.(M)["isDefault"] == true {
			defaults++
		}
	}
	if ls.Code != 200 || int(num(ls.Body["total"])) != 2 || defaults != 1 {
		t.Fatalf("list (one default): %d %s", ls.Code, ls.Raw)
	}
	if sel := call(t, "GET", "/card-terminals?storeId="+sid+"&select=name,provider", owner, nil); sel.Code != 200 {
		t.Fatalf("select: %d", sel.Code)
	} else if _, has := arr(sel.Body["items"])[0].(M)["fields"]; has {
		t.Fatalf("select must drop fields: %s", sel.Raw)
	}
	if r := call(t, "PATCH", "/card-terminals/"+tid, owner, M{"storeId": sid, "provider": "geidea"}); r.Code != 400 || r.errField("provider") == "" {
		t.Fatalf("provider is fixed: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "PATCH", "/card-terminals/"+tid, owner, M{"storeId": sid, "isDefault": true, "name": "Front till A"}); r.Code != 200 || r.Body["name"] != "Front till A" || r.Body["isDefault"] != true {
		t.Fatalf("patch: %d %s", r.Code, r.Raw)
	}
	if ck := call(t, "POST", "/card-terminals/"+tid+"/check", owner, M{"storeId": sid}); ck.Code != 200 || ck.Body["ok"] != true || str(ck.Body["at"]) == "" {
		t.Fatalf("check: %d %s", ck.Code, ck.Raw)
	}

	// someone else's store
	other := login(t, fx.ManagerEmail)
	if r := call(t, "GET", "/card-terminals?storeId="+sid, other, nil); r.Code != 403 {
		t.Fatalf("other store list: %d", r.Code)
	}
	if r := call(t, "POST", "/card-terminals/"+tid+"/payments", other, M{"storeId": sid, "amount": 5.0}); r.Code != 403 {
		t.Fatalf("other store pay: %d", r.Code)
	}
	if r := call(t, "GET", "/card-terminals", owner, nil); r.Code != 400 || r.errField("storeId") == "" {
		t.Fatalf("storeId required: %d", r.Code)
	}

	// pay: pending → approved after the simulator's delay
	old := nowFn
	defer func() { nowFn = old }()
	t0 := time.Now()
	nowFn = func() time.Time { return t0 }
	for _, b := range []M{{"amount": 0.0}, {"amount": -3.0}, {"amount": 1.234}, {"amount": "5"}, {"amount": 5.0, "reference": "bad ref with spaces"}} {
		b["storeId"] = sid
		if r := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, b); r.Code != 400 {
			t.Fatalf("bad payment %v: %d %s", b, r.Code, r.Raw)
		}
	}
	p1 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 115.5, "reference": "POS-1001"})
	if p1.Code != 201 || p1.Body["status"] != "pending" || p1.Body["currency"] != "SAR" || p1.Body["amount"] != 115.5 || p1.Body["test"] != true {
		t.Fatalf("start: %d %s", p1.Code, p1.Raw)
	}
	pid := str(p1.Body["id"])
	// same reference while pending → the same payment; another → busy
	if r := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 115.5, "reference": "POS-1001"}); r.Code != 200 || r.Body["id"] != pid {
		t.Fatalf("replay: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 9.0}); r.Code != 409 || r.errCode() != "terminal_busy" || r.errField("paymentId") != pid {
		t.Fatalf("busy: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/card-terminals/"+tid+"?storeId="+sid, owner, nil); r.Code != 409 {
		t.Fatalf("delete while pending: %d", r.Code)
	}
	if g := call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil); g.Code != 200 || g.Body["status"] != "pending" {
		t.Fatalf("still pending: %d %s", g.Code, g.Raw)
	}
	nowFn = func() time.Time { return t0.Add(3 * time.Second) }
	g := call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil)
	if g.Code != 200 || g.Body["status"] != "approved" || len(str(g.Body["authCode"])) != 6 || len(str(g.Body["rrn"])) != 12 || str(g.Body["finishedAt"]) == "" {
		t.Fatalf("approved: %d %s", g.Code, g.Raw)
	}
	if r := call(t, "POST", "/card-terminal-payments/"+pid+"/cancel", owner, M{"storeId": sid}); r.Code != 409 || r.errCode() != "payment_final" {
		t.Fatalf("cancel approved: %d %s", r.Code, r.Raw)
	}

	// declined (.05)
	nowFn = func() time.Time { return t0.Add(10 * time.Second) }
	p2 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 20.05})
	nowFn = func() time.Time { return t0.Add(13 * time.Second) }
	if g := call(t, "GET", "/card-terminal-payments/"+str(p2.Body["id"])+"?storeId="+sid, owner, nil); g.Body["status"] != "declined" {
		t.Fatalf("declined: %s", g.Raw)
	}
	// offline (.07) fails at once
	if p := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 20.07}); p.Code != 201 || p.Body["status"] != "failed" {
		t.Fatalf("failed: %d %s", p.Code, p.Raw)
	}
	// cancelled on the till
	p3 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 30.0})
	if c := call(t, "POST", "/card-terminal-payments/"+str(p3.Body["id"])+"/cancel", owner, M{"storeId": sid}); c.Code != 200 || c.Body["status"] != "cancelled" {
		t.Fatalf("cancel: %d %s", c.Code, c.Raw)
	}
	// no card (.06) → time-out after terminalTimeout
	nowFn = func() time.Time { return t0.Add(20 * time.Second) }
	p4 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 40.06})
	nowFn = func() time.Time { return t0.Add(20*time.Second + terminalTimeout - time.Second) }
	if g := call(t, "GET", "/card-terminal-payments/"+str(p4.Body["id"])+"?storeId="+sid, owner, nil); g.Body["status"] != "pending" {
		t.Fatalf("not yet timed out: %s", g.Raw)
	}
	nowFn = func() time.Time { return t0.Add(20*time.Second + terminalTimeout + time.Second) }
	if g := call(t, "GET", "/card-terminal-payments/"+str(p4.Body["id"])+"?storeId="+sid, owner, nil); g.Body["status"] != "timeout" {
		t.Fatalf("timeout: %s", g.Raw)
	}

	// webhook: wrong signature 404, right one refreshes
	nowFn = func() time.Time { return t0.Add(time.Hour) }
	p5 := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 12.0})
	p5id := str(p5.Body["id"])
	if r := call(t, "POST", "/card-terminal-webhooks/simulator/"+sid+"/"+p5id+"?sig=deadbeef", "", M{"status": "approved"}); r.Code != 404 {
		t.Fatalf("bad sig: %d", r.Code)
	}
	nowFn = func() time.Time { return t0.Add(time.Hour + 5*time.Second) }
	if r := call(t, "POST", "/card-terminal-webhooks/simulator/"+sid+"/"+p5id+"?sig="+webhookSig("simulator", sid, p5id), "", M{}); r.Code != 200 {
		t.Fatalf("webhook: %d %s", r.Code, r.Raw)
	}
	// providers that call back with GET
	if r := call(t, "GET", "/card-terminal-webhooks/simulator/"+sid+"/"+p5id+"?sig=deadbeef", "", nil); r.Code != 404 {
		t.Fatalf("GET webhook, bad sig: %d", r.Code)
	}
	if r := call(t, "GET", "/card-terminal-webhooks/simulator/"+sid+"/"+p5id+"?sig="+webhookSig("simulator", sid, p5id), "", nil); r.Code != 200 {
		t.Fatalf("GET webhook: %d %s", r.Code, r.Raw)
	}
	if g := call(t, "GET", "/card-terminal-payments/"+p5id+"?storeId="+sid, owner, nil); g.Body["status"] != "approved" {
		t.Fatalf("after webhook: %s", g.Raw)
	}

	// history
	hl := call(t, "GET", "/card-terminal-payments?storeId="+sid+"&terminalId="+tid+"&limit=3", owner, nil)
	if hl.Code != 200 || len(arr(hl.Body["items"])) != 3 || int(num(hl.Body["total"])) != 6 || str(arr(hl.Body["items"])[0].(M)["id"]) != p5id {
		t.Fatalf("history: %d %s", hl.Code, hl.Raw)
	}
	if r := call(t, "GET", "/card-terminal-payments?storeId="+sid+"&status=approved", owner, nil); int(num(r.Body["total"])) != 2 {
		t.Fatalf("history by status: %s", r.Raw)
	}
	for _, q := range []string{"&limit=0", "&limit=101", "&status=weird"} {
		if r := call(t, "GET", "/card-terminal-payments?storeId="+sid+q, owner, nil); r.Code != 400 {
			t.Fatalf("history %s: %d", q, r.Code)
		}
	}

	// switched off → refused; removed → gone from the list, payments stay
	call(t, "PATCH", "/card-terminals/"+tid, owner, M{"storeId": sid, "active": false})
	if r := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 1.0}); r.Code != 409 || r.errCode() != "terminal_inactive" {
		t.Fatalf("inactive: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/card-terminals/"+tid+"?storeId="+sid, owner, nil); r.Code != 200 {
		t.Fatalf("delete: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/card-terminals?storeId="+sid, owner, nil); int(num(r.Body["total"])) != 1 {
		t.Fatalf("after delete: %s", r.Raw)
	}
	if r := call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil); r.Code != 200 || r.Body["status"] != "approved" {
		t.Fatalf("payment kept: %d", r.Code)
	}

	// admin turns the provider off: the store can't pay with it any more
	enableSimulator(t, admin, M{"enabled": false})
	tid2 := str(cr2.Body["id"])
	if r := call(t, "POST", "/card-terminals/"+tid2+"/payments", owner, M{"storeId": sid, "amount": 1.0}); r.Code != 409 || r.errCode() != "provider_unavailable" {
		t.Fatalf("provider off: %d %s", r.Code, r.Raw)
	}
}

func setStoreCountry(t *testing.T, sid, cc string) {
	t.Helper()
	oid, _ := oidOf(sid)
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := mainDB().Collection("store").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"country_code": cc}}); err != nil {
		t.Fatal(err)
	}
}

// A UAE store connects an Adyen terminal: secrets stay sealed and hidden, and a
// payment runs through the blocking (background) path against a fake Adyen.
func TestAPI_CardTerminals_AdyenSecretsAndBackgroundRun(t *testing.T) {
	requireDB(t)
	resetTerminalProviders(t)
	defer resetTerminalProviders(t)
	owner, sid := signupOwner(t, "termadyen")
	defer cleanupStore(t, sid)
	setStoreCountry(t, sid, "AE")

	release := make(chan struct{})
	f := &fakeProvider{}
	f.reply = func(c fakeCall) (int, interface{}) {
		if c.Header.Get("x-API-key") != "ak_TOPSECRET1234" {
			return 401, nil
		}
		switch str(get(c.Body, "SaleToPOIRequest.MessageHeader.MessageCategory")) {
		case "Diagnosis":
			return 200, M{"SaleToPOIResponse": M{"DiagnosisResponse": M{"Response": M{"Result": "Success"}}}}
		case "Abort":
			return 200, nil
		}
		<-release // the shopper takes a moment
		return 200, M{"SaleToPOIResponse": M{"PaymentResponse": M{"Response": M{"Result": "Success"},
			"POIData":       M{"POITransactionID": M{"TransactionID": "BV1"}},
			"PaymentResult": M{"PaymentAcquirerData": M{"ApprovalCode": "AB12CD"}, "PaymentInstrumentData": M{"CardData": M{"MaskedPan": "4111 **** 1111", "PaymentBrand": "visa"}}},
		}}}
	}
	srv := f.server(t)
	t.Setenv("CARD_TERMINAL_BASEURL_ADYEN", srv.URL+"/v1")

	pr := call(t, "GET", "/card-terminals/providers?storeId="+sid, owner, nil)
	ids := []string{}
	for _, it := range arr(pr.Body["items"]) {
		ids = append(ids, str(it.(M)["id"]))
	}
	if pr.Body["countryCode"] != "AE" || !strings.Contains(strings.Join(ids, ","), "adyen") || strings.Contains(strings.Join(ids, ","), "nearpay") {
		t.Fatalf("UAE providers: %v", ids)
	}
	if r := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "N", "provider": "nearpay", "fields": M{"merchantUuid": "x", "terminalId": "1"}}); r.Code != 400 {
		t.Fatalf("Saudi-only provider in UAE: %d", r.Code)
	}
	fields := M{"merchantAccount": "ShopAE", "terminalId": "S1F2-1", "apiKey": "ak_TOPSECRET1234"}
	cr := call(t, "POST", "/card-terminals", owner, M{"storeId": sid, "name": "Adyen till", "provider": "adyen", "mode": "test", "fields": fields})
	if cr.Code != 201 || strings.Contains(cr.Raw, "TOPSECRET") || get(cr.Body, "fields.apiKey.hint") != "••••1234" || get(cr.Body, "fields.merchantAccount") != "ShopAE" {
		t.Fatalf("create: %d %s", cr.Code, cr.Raw)
	}
	tid := str(cr.Body["id"])
	if ls := call(t, "GET", "/card-terminals?storeId="+sid, owner, nil); strings.Contains(ls.Raw, "TOPSECRET") {
		t.Fatalf("list leaks: %s", ls.Raw)
	}
	ctx, cancel := dbctx()
	defer cancel()
	var raw bson.M
	_ = terminalsOf(sid).FindOne(ctx, bson.M{}).Decode(&raw)
	if v := str(get(normDoc(raw), "fields.apiKey")); !strings.HasPrefix(v, sealedPrefix) || strings.Contains(v, "TOPSECRET") {
		t.Fatalf("api key stored in plain text: %q", v)
	}
	// PATCH without the secret keeps it; the connection check proves it
	if r := call(t, "PATCH", "/card-terminals/"+tid, owner, M{"storeId": sid, "name": "Adyen till 2", "fields": M{"apiKey": ""}}); r.Code != 200 || get(r.Body, "fields.apiKey.hint") != "••••1234" {
		t.Fatalf("patch: %d %s", r.Code, r.Raw)
	}
	if ck := call(t, "POST", "/card-terminals/"+tid+"/check", owner, M{"storeId": sid}); ck.Body["ok"] != true {
		t.Fatalf("check: %s", ck.Raw)
	}

	// payment: pending while the terminal works, approved when it answers
	p := call(t, "POST", "/card-terminals/"+tid+"/payments", owner, M{"storeId": sid, "amount": 10.99, "reference": "INV-1"})
	if p.Code != 201 || p.Body["status"] != "pending" || p.Body["currency"] != "AED" || p.Body["test"] != true {
		t.Fatalf("start: %d %s", p.Code, p.Raw)
	}
	pid := str(p.Body["id"])
	if g := call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil); g.Body["status"] != "pending" {
		t.Fatalf("pending: %s", g.Raw)
	}
	close(release)
	var g resp
	eventually(t, "adyen approval stored", func() bool {
		g = call(t, "GET", "/card-terminal-payments/"+pid+"?storeId="+sid, owner, nil)
		return g.Body["status"] == "approved"
	})
	if g.Body["authCode"] != "AB12CD" || g.Body["maskedPan"] != "••••1111" || g.Body["providerRef"] != "BV1" {
		t.Fatalf("approved: %s", g.Raw)
	}
}
