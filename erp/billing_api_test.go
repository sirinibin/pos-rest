package erp

import (
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// DB-backed API + integration tests for bank-transfer subscription payments.

// signupOwner creates a fresh store through sign-up and returns the owner's
// token and the store id (an owner is the store's admin but NOT a platform admin).
func signupOwner(t *testing.T, tag string) (string, string) {
	t.Helper()
	body := validSignup()
	body["owner"].(M)["email"] = "billing-" + tag + "+" + time.Now().Format("150405.000000") + "@signup.example"
	r := call(t, "POST", "/auth/signup", "", body)
	if r.Code != 201 {
		t.Fatalf("signup: %d %s", r.Code, r.Raw)
	}
	return str(r.Body["accessToken"]), str(get(r.Body, "store.id"))
}

func cleanupBilling(t *testing.T, sid string) {
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = billingPayments().DeleteMany(ctx, bson.M{"storeId": sid})
	_, _ = billingReceipts().DeleteMany(ctx, bson.M{"storeId": sid})
	cleanupStore(t, sid)
}

func payBody(sid, ref string) M {
	p := validPayment()
	p["storeId"] = sid
	p["reference"] = ref
	p["transferDate"] = todayRiyadh().Format(layoutDay)
	return p
}

func TestAPI_Billing_FullFlow(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	manager := login(t, fx.ManagerEmail) // store A manager: settings view only
	owner, sid := signupOwner(t, "flow")
	defer cleanupBilling(t, sid)
	ref := "FT" + strings.ToUpper(sid[16:])
	ctx, cancel := dbctx()
	defer cancel()
	_, _ = billingSettings().DeleteOne(ctx, bson.M{"_id": bankAccountDocID})

	// platformAdmin flag on /auth/me
	if me := call(t, "GET", "/auth/me", admin, nil); get(me.Body, "user.platformAdmin") != true {
		t.Fatalf("admin platformAdmin: %v", get(me.Body, "user.platformAdmin"))
	}
	if me := call(t, "GET", "/auth/me", owner, nil); get(me.Body, "user.platformAdmin") != false {
		t.Fatalf("owner platformAdmin: %v", get(me.Body, "user.platformAdmin"))
	}

	// plans
	pl := call(t, "GET", "/billing/plans", owner, nil)
	if pl.Code != 200 || len(arr(pl.Body["plans"])) != 2 || get(arr(pl.Body["plans"])[1].(M), "monthly.total") != 343.85 {
		t.Fatalf("plans: %d %s", pl.Code, pl.Raw)
	}

	// new store: on trial for 14 days, nothing pending
	sub0 := call(t, "GET", "/billing/subscription?storeId="+sid, owner, nil)
	trialEnd := todayRiyadh().AddDate(0, 0, 14).Format(layoutDay)
	if sub0.Code != 200 || sub0.Body["status"] != "trial" || sub0.Body["trialEndsAt"] != trialEnd || sub0.Body["plan"] != "professional" ||
		sub0.Body["pendingPaymentId"] != "" {
		t.Fatalf("subscription: %d %s", sub0.Code, sub0.Raw)
	}
	if r := call(t, "GET", "/billing/subscription", owner, nil); r.Code != 400 || r.errField("storeId") == "" {
		t.Fatalf("subscription without storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/billing/subscription?storeId="+sid, manager, nil); r.Code != 404 {
		t.Fatalf("other company's store: %d", r.Code)
	}

	// bank account: not configured yet, only platform admins edit it
	if r := call(t, "GET", "/billing/bank-account", owner, nil); r.Code != 200 || r.Body["configured"] != false {
		t.Fatalf("bank account empty: %d %s", r.Code, r.Raw)
	}
	bank := M{"bankName": "Al Rajhi Bank", "accountName": "StartERP Co.", "iban": "sa03 8000 0000 6080 1016 7519", "swift": "rjhisari"}
	if r := call(t, "PUT", "/billing/bank-account", owner, bank); r.Code != 403 {
		t.Fatalf("owner edits bank account: %d", r.Code)
	}
	if r := call(t, "PUT", "/billing/bank-account", admin, M{"bankName": "X", "accountName": "Y", "iban": "SA00"}); r.Code != 400 || r.errField("iban") == "" {
		t.Fatalf("bad iban: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "PUT", "/billing/bank-account", admin, bank); r.Code != 200 || r.Body["iban"] != "SA0380000000608010167519" ||
		r.Body["swift"] != "RJHISARI" || r.Body["configured"] != true || r.Body["updatedBy"] != "Admin T1" {
		t.Fatalf("save bank: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/billing/bank-account?select=iban", owner, nil); r.Code != 200 || r.Body["iban"] != "SA0380000000608010167519" || r.Body["bankName"] != nil {
		t.Fatalf("bank select: %d %s", r.Code, r.Raw)
	}

	// submission validation
	bad := payBody(sid, ref)
	bad["receipt"] = M{"name": "x.gif", "data": dataURL("image/gif", []byte("GIF89a"))}
	if r := call(t, "POST", "/billing/payments", owner, bad); r.Code != 400 || r.errField("receipt") == "" {
		t.Fatalf("gif receipt: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/billing/payments", manager, payBody(storeA(), "MGR0001")); r.Code != 403 {
		t.Fatalf("manager (settings view only) must not pay: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/billing/payments", owner, payBody(storeA(), "OTHER001")); r.Code != 404 {
		t.Fatalf("owner pays for another company's store: %d", r.Code)
	}

	// submit
	sp := call(t, "POST", "/billing/payments", owner, payBody(sid, ref))
	if sp.Code != 201 || sp.Body["status"] != "pending" || sp.Body["amount"] != 343.85 || sp.Body["vat"] != 44.85 ||
		get(sp.Body, "receipt.type") != "application/pdf" || get(sp.Body, "receipt.data") != nil ||
		!strings.HasPrefix(str(sp.Body["number"]), "PAY-") || sp.Body["storeName"] == "" {
		t.Fatalf("submit: %d %s", sp.Code, sp.Raw)
	}
	pid := str(sp.Body["id"])
	if r := call(t, "POST", "/billing/payments", owner, payBody(sid, ref+"X")); r.Code != 409 || r.errCode() != "pending_exists" {
		t.Fatalf("second pending: %d %s", r.Code, r.Raw)
	}
	if s := call(t, "GET", "/billing/subscription?storeId="+sid, owner, nil); s.Body["pendingPaymentId"] != pid {
		t.Fatalf("pendingPaymentId: %s", s.Raw)
	}

	// listing + select
	ls := call(t, "GET", "/billing/payments?storeId="+sid+"&select=reference,status", owner, nil)
	if ls.Code != 200 || len(ls.data()) != 1 || ls.Body["total"] != 1.0 {
		t.Fatalf("owner list: %d %s", ls.Code, ls.Raw)
	}
	row := ls.data()[0].(M)
	if row["reference"] != ref || row["status"] != "pending" || row["id"] != pid || row["amount"] != nil || row["history"] != nil {
		t.Fatalf("select row: %v", row)
	}
	if r := call(t, "GET", "/billing/payments", owner, nil); r.Code != 400 {
		t.Fatalf("customer list without storeId: %d", r.Code)
	}
	if r := call(t, "GET", "/billing/payments?storeId="+sid+"&status=bogus", owner, nil); r.Code != 400 {
		t.Fatalf("bad status: %d", r.Code)
	}
	if r := call(t, "GET", "/billing/payments?storeId="+sid+"&select=1abc", owner, nil); r.Code != 400 {
		t.Fatalf("bad select: %d", r.Code)
	}
	al := call(t, "GET", "/billing/payments?status=pending&q="+ref, admin, nil)
	if al.Code != 200 || len(al.data()) != 1 || get(al.data()[0].(M), "id") != pid {
		t.Fatalf("admin pending list: %d %s", al.Code, al.Raw)
	}
	sm := call(t, "GET", "/billing/payments/summary?storeId="+sid, owner, nil)
	if sm.Code != 200 || get(sm.Body, "pending.count") != 1.0 || get(sm.Body, "accepted.count") != 0.0 {
		t.Fatalf("summary: %d %s", sm.Code, sm.Raw)
	}

	// receipt round-trip; hidden from other companies
	rc := call(t, "GET", "/billing/payments/"+pid+"/receipt", admin, nil)
	if rc.Code != 200 || rc.Body["data"] != dataURL("application/pdf", pdfBytes) || rc.Body["name"] != "slip.pdf" ||
		!strings.Contains(rc.Header.Get("Cache-Control"), "no-store") {
		t.Fatalf("receipt: %d %.200s", rc.Code, rc.Raw)
	}
	if r := call(t, "GET", "/billing/payments/"+pid+"/receipt", manager, nil); r.Code != 404 {
		t.Fatalf("receipt for other company: %d", r.Code)
	}
	if r := call(t, "GET", "/billing/payments/"+pid, manager, nil); r.Code != 404 {
		t.Fatalf("payment for other company: %d", r.Code)
	}
	if r := call(t, "GET", "/billing/payments/nothex", owner, nil); r.Code != 404 {
		t.Fatalf("bad id: %d", r.Code)
	}

	// review is platform-admin only
	if r := call(t, "POST", "/billing/payments/"+pid+"/accept", owner, M{}); r.Code != 403 {
		t.Fatalf("owner accepts own payment: %d", r.Code)
	}
	if r := call(t, "POST", "/billing/payments/"+pid+"/reject", owner, M{"reason": "nope nope"}); r.Code != 403 {
		t.Fatalf("owner rejects: %d", r.Code)
	}
	if r := call(t, "POST", "/billing/payments/"+pid+"/reject", admin, M{"reason": "no"}); r.Code != 400 || r.errField("reason") == "" {
		t.Fatalf("short reason: %d %s", r.Code, r.Raw)
	}
	rj := call(t, "POST", "/billing/payments/"+pid+"/reject", admin, M{"reason": "Amount not received in our account"})
	if rj.Code != 200 || rj.Body["status"] != "rejected" || rj.Body["rejectionReason"] != "Amount not received in our account" ||
		rj.Body["reviewedBy"] != "Admin T1" || len(arr(rj.Body["history"])) != 2 || rj.Body["version"] != 2.0 {
		t.Fatalf("reject: %d %s", rj.Code, rj.Raw)
	}
	if r := call(t, "POST", "/billing/payments/"+pid+"/accept", admin, M{}); r.Code != 409 || r.errCode() != "not_pending" {
		t.Fatalf("accept after reject: %d %s", r.Code, r.Raw)
	}
	// customer sees the reason
	if g := call(t, "GET", "/billing/payments/"+pid+"?select=status,rejectionReason", owner, nil); g.Body["rejectionReason"] != "Amount not received in our account" {
		t.Fatalf("customer view of rejection: %s", g.Raw)
	}
	if s := call(t, "GET", "/billing/subscription?storeId="+sid, owner, nil); s.Body["status"] != "trial" || s.Body["pendingPaymentId"] != "" {
		t.Fatalf("still trial after reject: %s", s.Raw)
	}

	// a rejected reference can be resubmitted; accept activates the subscription after the trial
	sp2 := call(t, "POST", "/billing/payments", owner, payBody(sid, ref))
	if sp2.Code != 201 {
		t.Fatalf("resubmit: %d %s", sp2.Code, sp2.Raw)
	}
	pid2 := str(sp2.Body["id"])
	ac := call(t, "POST", "/billing/payments/"+pid2+"/accept", admin, M{"note": "Matched statement line 14"})
	wantStart := todayRiyadh().AddDate(0, 0, 15)
	wantEnd := wantStart.AddDate(0, 1, -1).Format(layoutDay)
	if ac.Code != 200 || ac.Body["status"] != "accepted" || ac.Body["periodStart"] != wantStart.Format(layoutDay) ||
		ac.Body["periodEnd"] != wantEnd || ac.Body["adminNote"] != "Matched statement line 14" {
		t.Fatalf("accept: %d %s", ac.Code, ac.Raw)
	}
	if r := call(t, "POST", "/billing/payments/"+pid2+"/accept", admin, M{}); r.Code != 409 {
		t.Fatalf("double accept: %d", r.Code)
	}
	s2 := call(t, "GET", "/billing/subscription?storeId="+sid, owner, nil)
	if s2.Body["status"] != "active" || s2.Body["paidUntil"] != wantEnd || s2.Body["lastPaymentId"] != pid2 || s2.Body["period"] != "monthly" {
		t.Fatalf("active subscription: %s", s2.Raw)
	}
	// store record carries it; the owner cannot forge it through PATCH /stores
	st := call(t, "GET", "/stores/"+sid, owner, nil)
	if get(st.Body, "subscription.paidUntil") != wantEnd {
		t.Fatalf("store subscription: %s", st.Raw)
	}
	pr := call(t, "PATCH", "/stores/"+sid, owner, M{"plan": "enterprise", "trialEndsAt": "2099-01-01",
		"subscription": M{"paidUntil": "2099-12-31", "status": "active"}, "branchEn": "Main branch"},
		"If-Match", str(st.Body["version"]))
	if pr.Code != 200 || pr.Body["plan"] != "professional" || pr.Body["trialEndsAt"] != trialEnd ||
		get(pr.Body, "subscription.paidUntil") != wantEnd || pr.Body["branchEn"] != "Main branch" {
		t.Fatalf("forged billing fields via PATCH: %d %s", pr.Code, pr.Raw)
	}
	// an accepted reference cannot be reused
	if r := call(t, "POST", "/billing/payments", owner, payBody(sid, strings.ToLower(ref))); r.Code != 409 || r.errCode() != "duplicate_reference" {
		t.Fatalf("duplicate reference: %d %s", r.Code, r.Raw)
	}

	// renewal while active extends from paidUntil; yearly = 12 months
	ry := payBody(sid, ref+"Y")
	ry["period"] = "yearly"
	ry["plan"] = "starter"
	sp3 := call(t, "POST", "/billing/payments", owner, ry)
	if sp3.Code != 201 || sp3.Body["amount"] != 1138.5 {
		t.Fatalf("yearly: %d %s", sp3.Code, sp3.Raw)
	}
	ac3 := call(t, "POST", "/billing/payments/"+str(sp3.Body["id"])+"/accept", admin, M{})
	end1, _ := parseDay(wantEnd)
	if ac3.Body["periodStart"] != end1.AddDate(0, 0, 1).Format(layoutDay) || ac3.Body["periodEnd"] != end1.AddDate(1, 0, 0).Format(layoutDay) {
		t.Fatalf("renewal period: %s", ac3.Raw)
	}
	if s := call(t, "GET", "/billing/subscription?storeId="+sid, owner, nil); s.Body["plan"] != "starter" || s.Body["period"] != "yearly" {
		t.Fatalf("plan change on accept: %s", s.Raw)
	}

	// cancel: the customer withdraws a pending submission
	sp4 := call(t, "POST", "/billing/payments", owner, payBody(sid, ref+"C"))
	pid4 := str(sp4.Body["id"])
	if r := call(t, "POST", "/billing/payments/"+pid4+"/cancel", manager, M{}); r.Code != 404 {
		t.Fatalf("other company cancels: %d", r.Code)
	}
	if r := call(t, "POST", "/billing/payments/"+pid4+"/cancel", owner, M{}); r.Code != 200 || r.Body["status"] != "cancelled" {
		t.Fatalf("cancel: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/billing/payments/"+pid4+"/reject", admin, M{"reason": "too late now"}); r.Code != 409 {
		t.Fatalf("reject cancelled: %d", r.Code)
	}
	sm2 := call(t, "GET", "/billing/payments/summary?storeId="+sid, admin, nil)
	if get(sm2.Body, "accepted.count") != 2.0 || get(sm2.Body, "rejected.count") != 1.0 || get(sm2.Body, "cancelled.count") != 1.0 ||
		get(sm2.Body, "accepted.amount") != 1482.35 {
		t.Fatalf("summary after: %s", sm2.Raw)
	}
}

// Two reviewers acting at once: exactly one wins, the other gets 409, and the
// subscription is extended once.
func TestAPI_Billing_ConcurrentReview(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	owner, sid := signupOwner(t, "race")
	defer cleanupBilling(t, sid)
	sp := call(t, "POST", "/billing/payments", owner, payBody(sid, "RACE"+strings.ToUpper(sid[18:])))
	if sp.Code != 201 {
		t.Fatalf("submit: %d %s", sp.Code, sp.Raw)
	}
	pid := str(sp.Body["id"])
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				codes[i] = call(t, "POST", "/billing/payments/"+pid+"/accept", admin, M{}).Code
			} else {
				codes[i] = call(t, "POST", "/billing/payments/"+pid+"/reject", admin, M{"reason": "race reject"}).Code
			}
		}(i)
	}
	wg.Wait()
	ok := 0
	for _, c := range codes {
		if c == 200 {
			ok++
		} else if c != 409 {
			t.Errorf("unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("exactly one review must win, got %d (%v)", ok, codes)
	}
}

// Two submissions at once for one store: the partial unique index keeps one.
func TestAPI_Billing_ConcurrentSubmit(t *testing.T) {
	requireDB(t)
	owner, sid := signupOwner(t, "dup")
	defer cleanupBilling(t, sid)
	var wg sync.WaitGroup
	codes := make([]int, 5)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = call(t, "POST", "/billing/payments", owner, payBody(sid, "DUP"+strings.ToUpper(sid[18:])+string(rune('A'+i)))).Code
		}(i)
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		if c == 201 {
			created++
		}
	}
	ctx, cancel := dbctx()
	defer cancel()
	n, _ := billingPayments().CountDocuments(ctx, bson.M{"storeId": sid, "status": "pending"})
	if created != 1 || n != 1 {
		t.Fatalf("one pending per store: created=%d stored=%d codes=%v", created, n, codes)
	}
}

// Legacy stores (created by the old app) have no billing data: status none,
// and paying starts today.
func TestAPI_Billing_LegacyStore(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)
	s := call(t, "GET", "/billing/subscription?storeId="+storeB(), admin, nil)
	if s.Code != 200 || s.Body["status"] != "none" {
		t.Fatalf("legacy store: %d %s", s.Code, s.Raw)
	}
	if r := call(t, "GET", "/billing/payments?select=id", admin, nil); r.Code != 200 {
		t.Fatalf("admin lists all stores: %d", r.Code)
	}
}
