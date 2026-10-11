package erp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestLedgerRepairRequest(t *testing.T) {
	sid := primitive.NewObjectID().Hex()
	for _, c := range []struct {
		name  string
		body  M
		field string
		apply bool
		store bool
	}{
		{"empty = dry run, all stores", M{}, "", false, false},
		{"apply", M{"apply": true}, "", true, false},
		{"store", M{"storeId": sid, "apply": false}, "", false, true},
		{"null store", M{"storeId": nil}, "", false, false},
		{"apply not bool", M{"apply": "yes"}, "apply", false, false},
		{"apply number", M{"apply": 1}, "apply", false, false},
		{"bad store id", M{"storeId": "nope"}, "storeId", false, false},
		{"store id not string", M{"storeId": 12}, "storeId", false, false},
	} {
		st, apply, err := ledgerRepairRequest(c.body)
		if c.field != "" {
			ae, ok := err.(*APIError)
			if !ok || ae.Fields[c.field] == "" {
				t.Errorf("%s: want a %s error, got %v", c.name, c.field, err)
			}
			continue
		}
		if err != nil || apply != c.apply || (st != nil) != c.store {
			t.Errorf("%s: got store=%v apply=%v err=%v", c.name, st, apply, err)
		}
	}
}

// ---- DB-backed: the repair on old data ----

// repairSnapshot is every ledger, posting, account, customer and vendor
// document of a store, raw, by collection.
func repairSnapshot(t *testing.T, sid string) map[string][]bson.Raw {
	t.Helper()
	out := map[string][]bson.Raw{}
	for _, coll := range []string{"ledger", "posting", "account", "customer", "vendor", "customerdeposit", "customerwithdrawal", "capital", "divident", "capitalwithdrawal"} {
		ctx, cancel := dbctx()
		cur, err := storeDB(sid).Collection(coll).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		for cur.Next(ctx) {
			raw := make(bson.Raw, len(cur.Current))
			copy(raw, cur.Current)
			out[coll] = append(out[coll], raw)
		}
		cur.Close(ctx)
		cancel()
	}
	return out
}

func sameSnapshot(t *testing.T, what string, a, b map[string][]bson.Raw) {
	t.Helper()
	for coll := range mergeKeys(a, b) {
		x, y := a[coll], b[coll]
		if len(x) != len(y) {
			t.Errorf("%s: %s has %d documents, want %d", what, coll, len(y), len(x))
			continue
		}
		for i := range x {
			if !bytes.Equal(x[i], y[i]) {
				t.Errorf("%s: %s document differs:\nwant %s\ngot  %s", what, coll, x[i], y[i])
			}
		}
	}
}

func mergeKeys(a, b map[string][]bson.Raw) map[string]bool {
	k := map[string]bool{}
	for x := range a {
		k[x] = true
	}
	for x := range b {
		k[x] = true
	}
	return k
}

func backupCount(t *testing.T, sid string) int64 {
	ctx, cancel := dbctx()
	defer cancel()
	n, err := storeDB(sid).Collection("erp_repair_backup").CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// settle waits for the store's background ledger work (the handlers' hold).
func settle(sid string) {
	oid, _ := primitive.ObjectIDFromHex(sid)
	models.HoldStore(oid).Release()
}

type repairDocs struct{ customer, deposit, withdrawal, capital, dividend, cw string }

// repairStoreDocs creates the same documents in a store through today's API.
func repairStoreDocs(t *testing.T, tok, sid string) repairDocs {
	t.Helper()
	me := call(t, "GET", "/auth/me", tok, nil)
	uid := str(get(me.Body, "user.id"))
	if uid == "" {
		t.Fatalf("me: %s", me.Raw)
	}
	dt := "2026-10-05T10:00"
	mk := func(path string, body M) string {
		t.Helper()
		body["storeId"] = sid
		r := call(t, "POST", path, tok, body)
		if r.Code != 201 {
			t.Fatalf("%s: %d %s", path, r.Code, r.Raw)
		}
		return str(r.Body["id"])
	}
	d := repairDocs{}
	d.customer = mk("/customers", M{"nameEn": "Repair Customer", "phone": "0551239876"})
	d.capital = mk("/capitals", M{"date": dt, "investorUserId": uid, "amount": 1000, "method": "cash"})
	d.deposit = mk("/deposits", M{"date": dt, "customerId": d.customer, "amount": 100, "method": "cash"})
	d.withdrawal = mk("/withdrawals", M{"date": dt, "customerId": d.customer, "amount": 30, "method": "cash"})
	d.dividend = mk("/dividends", M{"date": dt, "recipientUserId": uid, "amount": 40, "method": "bank_transfer"})
	d.cw = mk("/capital-withdrawals", M{"date": dt, "investorUserId": uid, "amount": 50, "method": "cash"})
	// a second capital withdrawal that stays posted (nothing to repair)
	mk("/capital-withdrawals", M{"date": dt, "investorUserId": uid, "amount": 7, "method": "cash"})
	settle(sid)
	return d
}

// breakLikeOldCode puts the store in the state the old code left: the
// deposit, withdrawal, capital and dividend deleted without their postings
// being taken off, and the capital withdrawal never posted.
func breakLikeOldCode(t *testing.T, sid string, d repairDocs) {
	t.Helper()
	ctx, cancel := dbctx()
	defer cancel()
	now := time.Now()
	for coll, id := range map[string]string{"customerdeposit": d.deposit, "customerwithdrawal": d.withdrawal, "capital": d.capital, "divident": d.dividend} {
		oid, _ := primitive.ObjectIDFromHex(id)
		if _, err := storeDB(sid).Collection(coll).UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"deleted": true, "deleted_at": now}}); err != nil {
			t.Fatal(err)
		}
	}
	// the capital withdrawal: no ledger, no postings, balances without it
	cw, _ := primitive.ObjectIDFromHex(d.cw)
	soid, _ := primitive.ObjectIDFromHex(sid)
	store, err := models.FindStoreByID(&soid, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := store.FindLedgerByReferenceID(cw, soid, bson.M{})
	if err != nil {
		t.Fatalf("the capital withdrawal must be posted today: %v", err)
	}
	accounts, err := ledger.GetRelatedAccounts()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = storeDB(sid).Collection("ledger").DeleteMany(ctx, bson.M{"reference_id": cw})
	_, _ = storeDB(sid).Collection("posting").DeleteMany(ctx, bson.M{"reference_id": cw})
	if err := models.SetAccountBalances(accounts); err != nil {
		t.Fatal(err)
	}
}

// accountBalances is name → balance / totals of every account of a store.
func accountBalances(t *testing.T, sid string) map[string]string {
	t.Helper()
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(sid).Collection("account").Find(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	defer cur.Close(ctx)
	out := map[string]string{}
	for cur.Next(ctx) {
		var a models.Account
		if err := cur.Decode(&a); err != nil {
			t.Fatal(err)
		}
		out[a.Name] = fmt.Sprintf("balance=%.2f debit=%.2f credit=%.2f type=%s side=%s", a.Balance, a.DebitTotal, a.CreditTotal, a.Type, a.DebitOrCreditBalance)
	}
	return out
}

func customerBalance(t *testing.T, sid, id string) float64 {
	oid, _ := primitive.ObjectIDFromHex(id)
	ctx, cancel := dbctx()
	defer cancel()
	var c struct {
		CreditBalance float64 `bson:"credit_balance"`
	}
	if err := storeDB(sid).Collection("customer").FindOne(ctx, bson.M{"_id": oid}).Decode(&c); err != nil {
		t.Fatal(err)
	}
	return c.CreditBalance
}

func repairStore(t *testing.T, sid string) *models.Store {
	oid, _ := primitive.ObjectIDFromHex(sid)
	s, err := models.FindStoreByID(&oid, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func refIDs(refs []models.RepairDocRef) []string {
	out := []string{}
	for _, r := range refs {
		out = append(out, r.Type+":"+r.ID)
	}
	sort.Strings(out)
	return out
}

func accountRows(a []models.RepairAccountChange) string {
	rows := []string{}
	for _, x := range a {
		rows = append(rows, fmt.Sprintf("%s new=%v %.2f/%.2f/%.2f -> %.2f/%.2f/%.2f", x.Name, x.New,
			x.BalanceBefore, x.DebitTotalBefore, x.CreditTotalBefore, x.BalanceAfter, x.DebitTotalAfter, x.CreditTotalAfter))
	}
	sort.Strings(rows)
	b, _ := json.MarshalIndent(rows, "", " ")
	return string(b)
}

func TestLedgerRepair_OldDeletesAndUnpostedCapitalWithdrawals(t *testing.T) {
	requireDB(t)
	admin := login(t, fx.AdminEmail)

	// X: the old, broken state; Y: the same documents handled by today's code
	ownerX, sidX := signupOwner(t, "ledger-repair-x")
	defer cleanupStore(t, sidX)
	ownerY, sidY := signupOwner(t, "ledger-repair-y")
	defer cleanupStore(t, sidY)
	dx := repairStoreDocs(t, ownerX, sidX)
	dy := repairStoreDocs(t, ownerY, sidY)
	// in the repair's order (models.ledgerRepairTypes): a customer account left
	// at zero keeps the type of its last non-zero balance, so the order counts
	for _, d := range []struct{ path, id string }{{"deposits", dy.deposit}, {"withdrawals", dy.withdrawal}, {"capitals", dy.capital}, {"dividends", dy.dividend}} {
		if r := call(t, "DELETE", "/"+d.path+"/"+d.id, ownerY, nil); r.Code != 200 {
			t.Fatalf("delete %s: %d %s", d.path, r.Code, r.Raw)
		}
	}
	settle(sidY)
	breakLikeOldCode(t, sidX, dx)
	want := []string{"capital:" + dx.capital, "customer_deposit:" + dx.deposit, "customer_withdrawal:" + dx.withdrawal, "dividend:" + dx.dividend}
	sort.Strings(want)

	// today's store needs nothing
	if r, err := repairStore(t, sidY).RepairLedgers(false); err != nil || !r.NothingToDo() || len(r.Skipped) != 0 {
		t.Fatalf("fresh store reported work: %+v %v", r, err)
	}
	broken := repairSnapshot(t, sidX)
	brokenBalances := accountBalances(t, sidX)
	if fmt.Sprint(brokenBalances) == fmt.Sprint(accountBalances(t, sidY)) {
		t.Fatalf("the broken store must differ from today's: %v", brokenBalances)
	}

	// 1. dry run (default): reports both categories, changes nothing
	store := repairStore(t, sidX)
	dry, err := store.RepairLedgers(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := refIDs(dry.DeletedWithLedger); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dry run deleted with ledger: %v, want %v", got, want)
	}
	for _, r := range dry.DeletedWithLedger {
		if r.Amount <= 0 || r.Code == "" {
			t.Errorf("dry run item without amount / code: %+v", r)
		}
	}
	if len(dry.MissingLedger) != 1 || dry.MissingLedger[0].ID != dx.cw || dry.MissingLedger[0].Amount != 50 ||
		dry.Counts["capital_withdrawal_missing_ledger"] != 1 || dry.Counts["customer_deposit_deleted_with_ledger"] != 1 {
		t.Errorf("dry run missing ledger: %+v %v", dry.MissingLedger, dry.Counts)
	}
	if dry.Apply || dry.RunID != "" || dry.BackupDocs != 0 || len(dry.Accounts) == 0 {
		t.Errorf("dry run report: %+v", dry)
	}
	sameSnapshot(t, "after the dry run", broken, repairSnapshot(t, sidX))
	if n := backupCount(t, sidX); n != 0 {
		t.Errorf("dry run wrote %d backup documents", n)
	}

	// the route: platform admins only, validated, dry run by default
	if r := call(t, "POST", "/admin/repairs/ledger", ownerX, M{"storeId": sidX}); r.Code != 403 {
		t.Errorf("owner: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/admin/repairs/ledger", "", M{"storeId": sidX}); r.Code != 401 {
		t.Errorf("no token: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/admin/repairs/ledger", admin, M{"storeId": "bad"}); r.Code != 400 || r.errField("storeId") == "" {
		t.Errorf("bad store id: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/admin/repairs/ledger", admin, M{"storeId": primitive.NewObjectID().Hex()}); r.Code != 400 || r.errField("storeId") == "" {
		t.Errorf("unknown store: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", "/admin/repairs/ledger", admin, M{"apply": "yes"}); r.Code != 400 || r.errField("apply") == "" {
		t.Errorf("apply not bool: %d %s", r.Code, r.Raw)
	}
	rd := call(t, "POST", "/admin/repairs/ledger", admin, M{"storeId": sidX})
	if rd.Code != 200 || rd.Body["apply"] != false || len(arr(rd.Body["stores"])) != 1 {
		t.Fatalf("route dry run: %d %s", rd.Code, rd.Raw)
	}
	if s0, _ := arr(rd.Body["stores"])[0].(M); len(arr(s0["deletedWithLedger"])) != 4 || len(arr(s0["missingLedger"])) != 1 ||
		num(get(s0, "counts.capital_withdrawal_missing_ledger")) != 1 {
		t.Fatalf("route dry run report: %s", rd.Raw)
	}
	// every store: the broken one is listed, today's is not
	all := call(t, "POST", "/admin/repairs/ledger", admin, M{})
	if all.Code != 200 || intv(get(all.Body, "totals.stores")) < 2 {
		t.Fatalf("route all stores: %d %s", all.Code, all.Raw)
	}
	foundX := false
	for _, s := range arr(all.Body["stores"]) {
		if m, _ := s.(M); str(m["storeId"]) == sidX {
			foundX = true
		} else if str(m["storeId"]) == sidY {
			t.Errorf("today's store listed: %v", m)
		}
	}
	if !foundX {
		t.Errorf("the broken store is not in the all-stores run")
	}
	sameSnapshot(t, "after the route dry runs", broken, repairSnapshot(t, sidX))

	// 2. apply: both fixed, balances equal those of today's code
	ra := call(t, "POST", "/admin/repairs/ledger", admin, M{"storeId": sidX, "apply": true})
	if ra.Code != 200 || ra.Body["apply"] != true {
		t.Fatalf("route apply: %d %s", ra.Code, ra.Raw)
	}
	var run models.LedgerRepairRun
	if err := json.Unmarshal([]byte(ra.Raw), &run); err != nil || len(run.Stores) != 1 {
		t.Fatalf("apply report: %v %s", err, ra.Raw)
	}
	applied := run.Stores[0]
	if len(applied.Errors) != 0 || applied.RunID == "" || applied.BackupDocs == 0 {
		t.Fatalf("apply: %+v", applied)
	}
	if fmt.Sprint(refIDs(applied.DeletedWithLedger)) != fmt.Sprint(want) || len(applied.MissingLedger) != 1 {
		t.Errorf("apply items: %v %v", refIDs(applied.DeletedWithLedger), applied.MissingLedger)
	}
	// the dry run predicted the same accounts and balances
	if a, b := accountRows(dry.Accounts), accountRows(applied.Accounts); a != b {
		t.Errorf("dry run accounts differ from apply:\ndry   %s\napply %s", a, b)
	}
	if a, b := accountBalances(t, sidX), accountBalances(t, sidY); fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("repaired balances differ from today's code:\nrepaired %v\ntoday    %v", a, b)
	}
	if a, b := customerBalance(t, sidX, dx.customer), customerBalance(t, sidY, dy.customer); a != b {
		t.Errorf("customer balance: repaired %v, today %v", a, b)
	}
	ctx, cancel := dbctx()
	defer cancel()
	for _, id := range []string{dx.deposit, dx.withdrawal, dx.capital, dx.dividend} {
		oid, _ := primitive.ObjectIDFromHex(id)
		for _, coll := range []string{"ledger", "posting"} {
			if n, _ := storeDB(sidX).Collection(coll).CountDocuments(ctx, bson.M{"reference_id": oid}); n != 0 {
				t.Errorf("deleted %s still has %d %s documents", id, n, coll)
			}
		}
	}
	cwID, _ := primitive.ObjectIDFromHex(dx.cw)
	if n, _ := storeDB(sidX).Collection("ledger").CountDocuments(ctx, bson.M{"reference_id": cwID, "reference_model": "capital_withdrawal"}); n != 1 {
		t.Errorf("capital withdrawal ledgers: %d", n)
	}
	var marker bson.M
	if err := storeDB(sidX).Collection("erp_repair_backup").FindOne(ctx, bson.M{"_id": "ledger-2026-10-11:marker"}).Decode(&marker); err != nil {
		t.Errorf("marker: %v", err)
	}
	for _, kind := range []string{"removed", "modified", "party", "added"} {
		if n, _ := storeDB(sidX).Collection("erp_repair_backup").CountDocuments(ctx, bson.M{"repair": "ledger-2026-10-11", "kind": kind}); n == 0 {
			t.Errorf("no %s backup records", kind)
		}
	}
	added, _ := storeDB(sidX).Collection("erp_repair_backup").CountDocuments(ctx, bson.M{"kind": "added", "collection": "ledger", "reference_id": cwID})
	if added != 1 {
		t.Errorf("the created capital withdrawal ledger is not tagged: %d", added)
	}

	// 3. a second apply finds nothing and writes nothing
	repaired := repairSnapshot(t, sidX)
	backups := backupCount(t, sidX)
	again, err := store.RepairLedgers(true)
	if err != nil || !again.NothingToDo() || again.BackupDocs != 0 || again.RunID != "" {
		t.Fatalf("second apply: %+v %v", again, err)
	}
	sameSnapshot(t, "after the second apply", repaired, repairSnapshot(t, sidX))
	if n := backupCount(t, sidX); n != backups {
		t.Errorf("second apply wrote backups: %d -> %d", backups, n)
	}

	// 4. restore: the original state comes back exactly, once
	rest, err := store.RestoreLedgerRepair()
	if err != nil || len(rest.Runs) != 1 || rest.Reinserted == 0 || rest.RemovedAdded == 0 || rest.AccountsRestored == 0 || len(rest.AccountsKept) != 0 {
		t.Fatalf("restore: %+v %v", rest, err)
	}
	sameSnapshot(t, "after restore", broken, repairSnapshot(t, sidX))
	if again, err := store.RestoreLedgerRepair(); err != nil || !again.AlreadyRestored {
		t.Errorf("second restore: %+v %v", again, err)
	}
	sameSnapshot(t, "after the second restore", broken, repairSnapshot(t, sidX))
	// and the restored store is found broken again: the repair reruns
	if d2, err := store.RepairLedgers(false); err != nil || fmt.Sprint(refIDs(d2.DeletedWithLedger)) != fmt.Sprint(want) || len(d2.MissingLedger) != 1 {
		t.Errorf("dry run after restore: %+v %v", d2, err)
	}
	if r2, err := store.RepairLedgers(true); err != nil || len(r2.Errors) != 0 {
		t.Fatalf("re-apply: %+v %v", r2, err)
	}
	if a, b := accountBalances(t, sidX), accountBalances(t, sidY); fmt.Sprint(a) != fmt.Sprint(b) {
		t.Errorf("re-applied balances differ from today's code:\nrepaired %v\ntoday    %v", a, b)
	}
}

// A capital withdrawal without an investor cannot be posted: it is reported
// as skipped and left alone (every run).
func TestLedgerRepair_SkipsUnpostableCapitalWithdrawal(t *testing.T) {
	requireDB(t)
	_, sid := signupOwner(t, "ledger-repair-skip")
	defer cleanupStore(t, sid)
	soid, _ := primitive.ObjectIDFromHex(sid)
	id := primitive.NewObjectID()
	ctx, cancel := dbctx()
	defer cancel()
	if _, err := storeDB(sid).Collection("capitalwithdrawal").InsertOne(ctx, bson.M{"_id": id, "store_id": soid, "code": "CWD-OLD-1",
		"amount": 12.5, "payment_method": "cash", "date": time.Now()}); err != nil {
		t.Fatal(err)
	}
	store := repairStore(t, sid)
	for _, apply := range []bool{false, true} {
		r, err := store.RepairLedgers(apply)
		if err != nil || !r.NothingToDo() || len(r.Skipped) != 1 || r.Skipped[0].Code != "CWD-OLD-1" || r.Skipped[0].Reason == "" || r.BackupDocs != 0 {
			t.Errorf("apply=%v: %+v %v", apply, r, err)
		}
	}
	if n := backupCount(t, sid); n != 0 {
		t.Errorf("skipped repair wrote %d backups", n)
	}
}
