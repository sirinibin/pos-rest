//go:build e2e

package api

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Concurrency of every other document type: several users of one store (and
// two stores at once where it matters) fire requests through a start
// barrier, then the store is compared with an oracle computed here:
// numbering, the first ledger account of a new customer/vendor, two users
// editing the same document, returns and payments that together exceed the
// parent, completing a transfer or finalizing a draft twice, stock under
// product edits and movements, and store settings edited by two admins.
// Helpers use the cd prefix (concRace is shared with other concurrency
// tests).

// concRace records a race-only bug: the check holds when the same steps run
// one after another, so a run where the race did not happen is only logged.
func concRace(t testing.TB, id, what string, observed bool) {
	t.Helper()
	if observed {
		KnownBug(t, id, what, true)
		return
	}
	t.Logf("race %s did not reproduce this run", id)
}

// cdRun fires every call at the same moment (start barrier) and waits.
func cdRun(calls []func()) { concRun(calls) }

// cdStore is a signed-up store with a token per role ("owner" = the owner).
type cdStore struct {
	*Store
	tok map[string]string
}

func cdSetup(t *testing.T, country string, roles ...string) *cdStore {
	t.Helper()
	cs := &cdStore{Store: Signup(t, country), tok: map[string]string{}}
	cs.tok["owner"] = cs.Token
	for _, r := range roles {
		_, cs.tok[r] = cs.User(t, r)
	}
	return cs
}

// cdRoles: the users that may create documents of a module.
var cdRoles = map[string][]string{
	"sales":   {"owner", "r_manager", "r_salesman", "r_cashier"},
	"finance": {"owner", "r_manager", "r_accountant"},
	"other":   {"owner", "r_manager"},
}

// cdPatch PATCHes with an If-Match version (0 = none).
func cdPatch(t testing.TB, tok, path, id string, version float64, body M) Resp {
	if version > 0 {
		return Call(t, "PATCH", "/"+path+"/"+id, tok, body, "If-Match", Number(version).String(), "X-Change-Reason", "e2e")
	}
	return Call(t, "PATCH", "/"+path+"/"+id, tok, body, "X-Change-Reason", "e2e")
}

var cdTrail = regexp.MustCompile(`(\d+)$`)

// cdNo is the number at the end of a document code (-1 = none).
func cdNo(code string) int {
	m := cdTrail.FindStringSubmatch(code)
	if m == nil {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// cdMaxNo is the highest code number among the records of a list whose
// code matches series ("" = all).
func cdMaxNo(t testing.TB, s *Store, path, series string) int {
	t.Helper()
	max := 0
	re := regexp.MustCompile(series)
	for _, r := range List(t, s.Token, path, "storeId="+s.ID+"&limit=500&select=code") {
		if n := cdNo(S(r["code"])); n > max && re.MatchString(S(r["code"])) {
			max = n
		}
	}
	return max
}

// cdTotal is the list total of a resource in the store.
func cdTotal(t testing.TB, s *Store, path string) int {
	t.Helper()
	r := Must(t, Call(t, "GET", "/"+path+"?storeId="+s.ID+"&limit=1", s.Token, nil), 200, "list "+path)
	return int(Num(r.Body["total"]))
}

// cdNumbers checks codes are unique and exactly first..first+len-1.
func cdNumbers(codes []string, first int) (dups []string, gapless bool, nums []int) {
	seen := map[string]int{}
	for _, c := range codes {
		if seen[c]++; seen[c] == 2 {
			dups = append(dups, c)
		}
		nums = append(nums, cdNo(c))
	}
	sort.Ints(nums)
	gapless = true
	for i, n := range nums {
		if n != first+i {
			gapless = false
		}
	}
	return dups, gapless, nums
}

// cdSettle polls check until it reports "" (ok) or 15 s pass; it returns
// the last report.
func cdSettle(check func() string) string {
	deadline := time.Now().Add(15 * time.Second)
	for {
		msg := check()
		if msg == "" || time.Now().After(deadline) {
			return msg
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// cdAccounts lists the store's ledger accounts.
func cdAccounts(t testing.TB, s *Store) []M {
	t.Helper()
	return List(t, s.Token, "accounts", finQS(s, "limit=500"))
}

// cdAcctByRef: the accounts with a referenceId, and the signed balance of the first.
func cdAcctByRef(accts []M, ref string) (n int, bal float64) {
	for _, a := range accts {
		if S(a["referenceId"]) == ref {
			if n == 0 {
				bal = finSigned(a)
			}
			n++
		}
	}
	return n, bal
}

func cdAcctByName(accts []M, name string) float64 {
	for _, a := range accts {
		if S(a["nameEn"]) == name {
			return finSigned(a)
		}
	}
	return 0
}

// cdPaid is the sum of a document's payments.
func cdPaid(doc M) float64 {
	var p float64
	for _, x := range Objs(doc["payments"]) {
		p += F(x, "amount")
	}
	return p
}

// ---------- 1. numbering ----------

type cdNumFix struct {
	*cdStore
	pid, cid, vid, cat, wh, veh, sale, pur string
}

type cdNumType struct {
	name, path, module string
	series             string // regexp of the type's codes when the list mixes series
	body               func(f *cdNumFix, i int) M
}

func cdNumTypes() []cdNumType {
	return []cdNumType{
		{"purchases", "purchases", "other", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "vendorId": f.vid, "items": []M{f.Line(f.pid, 1, 10)}}
		}},
		{"purchase-returns", "purchase-returns", "other", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "purchaseId": f.pur, "vendorId": f.vid, "items": []M{f.Line(f.pid, 1, 10)}}
		}},
		{"sales-returns", "sales-returns", "sales", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "orderId": f.sale, "customerId": f.cid, "items": []M{f.Line(f.pid, 1, 20)}}
		}},
		{"quotations", "quotations", "sales", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "customerId": f.cid, "validityDays": 7, "deliveryDays": 3, "items": []M{f.Line(f.pid, 1, 20)}}
		}},
		{"nonvat-sales", "nonvat-sales", "sales", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "customerId": f.cid, "items": []M{f.Line(f.pid, 1, 12.5)}}
		}},
		{"delivery-notes", "delivery-notes", "sales", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "customerId": f.cid, "estDelivery": f.Today(), "items": []M{f.Line(f.pid, 1, 0)}}
		}},
		{"proformas", "proformas", "sales", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "customerId": f.cid, "validityDays": 7, "deliveryDays": 3, "items": []M{f.Line(f.pid, 1, 20)}}
		}},
		{"expenses", "expenses", "finance", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "categoryId": f.cat, "description": "Conc", "amount": 10 + i, "method": "cash"}
		}},
		{"capitals", "capitals", "finance", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "investor": finOwnerName, "amount": 100 + i, "method": "cash"}
		}},
		{"dividends", "dividends", "finance", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "recipient": finOwnerName, "amount": 1 + i, "method": "cash"}
		}},
		{"deposits", "deposits", "finance", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "customerId": f.cid, "amount": 10 + i, "method": "cash"}
		}},
		{"withdrawals", "withdrawals", "finance", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "customerId": f.cid, "amount": 1 + i, "method": "cash"}
		}},
		{"stock-transfers", "stock-transfers", "other", "^ST-TR-", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "fromWarehouseId": f.MS, "toWarehouseId": f.wh, "vatPercent": 15,
				"items": []M{{"productId": f.pid, "qty": 1, "unitPrice": 10}}}
		}},
		{"pending stock-transfers", "stock-transfers", "other", "^[A-Z]+-ST-TR-", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "fromWarehouseId": f.MS, "toWarehouseId": f.wh, "vatPercent": 15, "status": "pending",
				"items": []M{{"productId": f.pid, "qty": 1, "unitPrice": 10}}}
		}},
		{"warehouses", "warehouses", "other", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "nameEn": "Conc WH " + Uniq(), "nameAr": "مستودع", "code": "C" + Digits(6)}
		}},
		{"purchase-bills", "purchase-bills", "other", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "vendorId": f.vid, "receivedAt": f.Now(), "fileName": "bill.pdf", "amount": 115, "status": "new"}
		}},
		{"repair-jobs", "repair-jobs", "other", "", func(f *cdNumFix, i int) M {
			return M{"storeId": f.ID, "date": f.Now(), "status": "open", "vehicleId": f.veh, "customerId": f.cid, "complaint": "Conc", "labour": 10 + i}
		}},
	}
}

func cdNumFixture(t *testing.T, country string) *cdNumFix {
	cs := cdSetup(t, country, "r_manager", "r_salesman", "r_cashier", "r_accountant")
	f := &cdNumFix{cdStore: cs}
	f.pid = S(cs.Product(t, 10, 20, 100000)["id"])
	f.cid = S(cs.Customer(t, "")["id"])
	f.vid = S(cs.Vendor(t)["id"])
	f.cat = finExpenseCat(t, cs.Store, "Conc "+Uniq())
	f.wh = S(Create(t, cs.Token, "warehouses", M{"storeId": cs.ID, "nameEn": "Branch " + Uniq(), "nameAr": "فرع", "code": "B" + Digits(6)})["id"])
	f.veh = S(Create(t, cs.Token, "vehicles", M{"storeId": cs.ID, "plate": "CD " + Digits(4), "make": "Ford", "model": "Focus", "customerId": f.cid})["id"])
	f.sale = S(Create(t, cs.Token, "sales", salesBody(cs.Store, f.cid, []M{cs.Line(f.pid, 100, 20)}, nil))["id"])
	f.pur = S(Create(t, cs.Token, "purchases", M{"storeId": cs.ID, "date": cs.Now(), "vendorId": f.vid, "items": []M{cs.Line(f.pid, 100, 10)}})["id"])
	return f
}

// TestConcDocs_Numbering: about ten creates of each type at the same moment
// by mixed users, in two stores at once. Codes must be unique per store and,
// as every create succeeds, follow on from the store's last number without
// gaps (the order may differ from the arrival order).
func TestConcDocs_Numbering(t *testing.T) {
	t.Parallel()
	const n = 10
	stores := []*cdNumFix{cdNumFixture(t, "SA"), cdNumFixture(t, "AE")}
	for _, tp := range cdNumTypes() {
		tp := tp
		type run struct {
			first, before int
			codes         []string
		}
		runs := make([]*run, len(stores))
		var calls []func()
		var mu sync.Mutex
		for si, f := range stores {
			f := f
			ru := &run{first: cdMaxNo(t, f.Store, tp.path, tp.series) + 1, before: cdTotal(t, f.Store, tp.path)}
			if tp.name == "warehouses" {
				// legacy numbers warehouses WH<n> by a counter of the store's warehouses
				ru.first = cdTotal(t, f.Store, "warehouses") // the virtual main store is listed but not numbered
			}
			runs[si] = ru
			roles := cdRoles[tp.module]
			for i := 0; i < n; i++ {
				i, role := i, roles[i%len(roles)]
				body := tp.body(f, i)
				calls = append(calls, func() {
					r := Call(t, "POST", "/"+tp.path, f.tok[role], body)
					if r.Code != 201 {
						t.Errorf("%s store %s: %s creates: %s", tp.name, f.Country, role, r)
						return
					}
					mu.Lock()
					ru.codes = append(ru.codes, S(r.Body["code"]))
					mu.Unlock()
				})
			}
		}
		cdRun(calls)
		for si, f := range stores {
			ru := runs[si]
			dups, gapless, nums := cdNumbers(ru.codes, ru.first)
			if got := cdTotal(t, f.Store, tp.path); got != ru.before+len(ru.codes) {
				t.Errorf("%s store %s: %d listed after %d creates, want %d", tp.name, f.Country, got, len(ru.codes), ru.before+len(ru.codes))
			}
			if len(ru.codes) != n {
				continue
			}
			id := "NEW-CONC-NUM-" + strings.ToUpper(strings.ReplaceAll(tp.name, " ", "-"))
			if len(dups) > 0 || !gapless {
				concRace(t, id, fmt.Sprintf("%d concurrent POST /%s (store %s) got numbers %v, want %d..%d once each (duplicate codes %v)",
					n, tp.path, f.Country, nums, ru.first, ru.first+n-1, dups), true)
			} else {
				concRace(t, id, "", false)
			}
		}
	}
}

// ---------- 2. first documents of a new customer / vendor ----------

// TestConcDocs_FirstDocsNewParty: three new customers each get four credit
// sales and three new vendors four credit purchases, all at the same moment.
// Each party must end up with exactly one ledger account whose balance is
// what its documents still owe, and account numbers must stay unique.
func TestConcDocs_FirstDocsNewParty(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager", "r_salesman", "r_cashier")
	pid := S(cs.Product(t, 10, 20, 10000)["id"])
	vat := F(cs.Rec, "vatPercent")
	owed := map[string]int64{} // customer: receivable, vendor: payable (cents)
	var custs, vends []string
	for i := 0; i < 3; i++ {
		custs = append(custs, S(cs.Customer(t, "")["id"]))
		vends = append(vends, S(cs.Vendor(t)["id"]))
	}
	var calls []func()
	sales := cdRoles["sales"]
	for ci, cid := range custs {
		for k := 0; k < 4; k++ {
			cid, qty, role := cid, float64(1+k), sales[(ci+k)%len(sales)]
			net := salesOracle([]salesLn{{qty, 20, 0}}, 0, 0, vat).net
			var pays []M
			paid := int64(0)
			if k%2 == 1 {
				paid = 500
				pays = []M{salesPay(5, "cash")}
			}
			owed[cid] += net - paid
			body := salesBody(cs.Store, cid, []M{cs.Line(pid, qty, 20)}, pays)
			calls = append(calls, func() {
				if r := Call(t, "POST", "/sales", cs.tok[role], body); r.Code != 201 {
					t.Errorf("%s sale to a new customer: %s", role, r)
				}
			})
		}
	}
	for vi, vid := range vends {
		for k := 0; k < 4; k++ {
			vid, qty, role := vid, float64(1+k), cdRoles["other"][(vi+k)%2]
			_, _, net := purOracle([]purLine{{qty, 10, 0}}, 0, 0, vat)
			body := M{"storeId": cs.ID, "date": cs.Now(), "vendorId": vid, "items": []M{cs.Line(pid, qty, 10)}}
			paid := int64(0)
			if k%2 == 1 {
				paid = 300
				body["payments"] = []M{{"date": cs.Now(), "amount": 3, "method": "cash"}}
			}
			owed[vid] += net - paid
			calls = append(calls, func() {
				if r := Call(t, "POST", "/purchases", cs.tok[role], body); r.Code != 201 {
					t.Errorf("%s purchase from a new vendor: %s", role, r)
				}
			})
		}
	}
	cdRun(calls)
	if t.Failed() {
		t.FailNow()
	}

	var accts []M
	var custBad, vendBad []string
	report := cdSettle(func() string {
		accts = cdAccounts(t, cs.Store)
		custBad, vendBad = nil, nil
		for _, id := range append(append([]string{}, custs...), vends...) {
			n, bal := cdAcctByRef(accts, id)
			want, path := owed[id], "customers"
			if cdHas(vends, id) {
				path, want = "vendors", -want
			}
			cb := F(Read(t, cs.Token, path, id), "creditBalance")
			if n != 1 || Cents(bal) != want || Cents(cb) != want {
				msg := fmt.Sprintf("%s …%s: %d accounts, ledger %.2f, creditBalance %.2f, want %.2f", path, id[len(id)-6:], n, bal, cb, float64(want)/100)
				if path == "vendors" {
					vendBad = append(vendBad, msg)
				} else {
					custBad = append(custBad, msg)
				}
			}
		}
		return strings.Join(append(append([]string{}, custBad...), vendBad...), "; ")
	})
	dupAcct := false
	for _, id := range append(append([]string{}, custs...), vends...) {
		if n, _ := cdAcctByRef(accts, id); n > 1 {
			dupAcct = true
		}
	}
	names, numbers := map[string]int{}, map[string]int{}
	var dupNames, dupNumbers []string
	for _, a := range accts {
		if S(a["referenceId"]) == "" {
			if names[S(a["nameEn"])]++; names[S(a["nameEn"])] == 2 {
				dupNames = append(dupNames, S(a["nameEn"]))
			}
		}
		if numbers[S(a["code"])]++; numbers[S(a["code"])] == 2 {
			dupNumbers = append(dupNumbers, S(a["code"]))
		}
	}
	concRace(t, "NEW-CONC-ACCT-DUP", fmt.Sprintf("first documents of a new customer/vendor at the same moment create more than one ledger account for it, "+
		"or duplicate system accounts %v (models/account.go CreateAccountIfNotExists: find, then insert): %s", dupNames, report), dupAcct || len(dupNames) > 0)
	concRace(t, "NEW-CONC-ACCT-NUMBER", fmt.Sprintf("ledger accounts created at the same moment share account numbers %v (number = 1000 + count)", dupNumbers), len(dupNumbers) > 0)
	if !dupAcct {
		concRace(t, "NEW-sales-ledger-race", "new customers' balances after their first sales at the same moment: "+strings.Join(custBad, "; "), len(custBad) > 0)
		concRace(t, "NEW-CONC-VENDOR-LEDGER", "new vendors' balances after their first purchases at the same moment: "+strings.Join(vendBad, "; "), len(vendBad) > 0)
	}
}

func cdHas(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ---------- 3. two users edit the same document ----------

// TestConcDocs_SameDocPatch: in three rounds two users read the same version
// of an expense, a deposit, a purchase and a sales return and PATCH it at the
// same moment (If-Match as the web app sends it). At most one of each pair
// may win. Afterwards the ledger, CASH/BANK, the parties' balances, stock and
// the sale's returned quantity must match the final documents.
func TestConcDocs_SameDocPatch(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager", "r_accountant", "r_salesman")
	pid := S(cs.Product(t, 10, 20, 100)["id"])
	cat := finExpenseCat(t, cs.Store, "Conc "+Uniq())
	k1, k2 := S(cs.Customer(t, "")["id"]), S(cs.Customer(t, "")["id"])
	vid := S(cs.Vendor(t)["id"])
	exp := Create(t, cs.Token, "expenses", M{"storeId": cs.ID, "date": cs.Now(), "categoryId": cat, "description": "Conc", "amount": 100, "method": "cash"})
	dep := Create(t, cs.Token, "deposits", M{"storeId": cs.ID, "date": cs.Now(), "customerId": k1, "amount": 50, "method": "bank_transfer"})
	pur := Create(t, cs.Token, "purchases", M{"storeId": cs.ID, "date": cs.Now(), "vendorId": vid, "items": []M{cs.Line(pid, 10, 10)}})
	sale := Create(t, cs.Token, "sales", salesBody(cs.Store, k2, []M{cs.Line(pid, 10, 20)}, nil))
	ret := Create(t, cs.Token, "sales-returns", M{"storeId": cs.ID, "date": cs.Now(), "orderId": sale["id"], "customerId": k2,
		"items": []M{cs.Line(pid, 1, 20)}})
	docs := []struct {
		path, id string
		users    [2]string
		body     func(round, u int) M
	}{
		{"expenses", S(exp["id"]), [2]string{"owner", "r_accountant"}, func(r, u int) M { return M{"amount": float64(100 + 10*r + u)} }},
		{"deposits", S(dep["id"]), [2]string{"r_manager", "r_accountant"}, func(r, u int) M { return M{"amount": float64(50 + 5*r + u)} }},
		{"purchases", S(pur["id"]), [2]string{"owner", "r_manager"}, func(r, u int) M {
			return M{"payments": []M{{"date": cs.Now(), "amount": float64(20 + 10*r + u), "method": "cash"}}}
		}},
		{"sales-returns", S(ret["id"]), [2]string{"r_manager", "r_salesman"}, func(r, u int) M {
			return M{"items": []M{cs.Line(pid, float64(1+(r+u)%3), 20)}}
		}},
	}
	var both []string
	for round := 1; round <= 3; round++ {
		var calls []func()
		var mu sync.Mutex
		for _, d := range docs {
			d := d
			ver := F(Read(t, cs.Token, d.path, d.id), "version")
			ok := 0
			for u, user := range d.users {
				body := d.body(round, u+1)
				tok := cs.tok[user]
				calls = append(calls, func() {
					r := cdPatch(t, tok, d.path, d.id, ver, body)
					switch {
					case r.Code == 200:
						mu.Lock()
						if ok++; ok == 2 {
							both = append(both, fmt.Sprintf("%s round %d", d.path, round))
						}
						mu.Unlock()
					case r.Code != 409:
						t.Errorf("%s PATCH %s: %s", user, d.path, r)
					}
				})
			}
		}
		cdRun(calls)
	}
	if t.Failed() {
		t.FailNow()
	}
	concRace(t, "NEW-CONC-IFMATCH", fmt.Sprintf("two PATCHes with the same If-Match version both answered 200 (%s): the version check is read-then-write, "+
		"not part of the update (erp/engine.go handleUpdate), so one edit silently overwrites the other", strings.Join(both, ", ")), len(both) > 0)

	report := cdSettle(func() string {
		e, dp := Read(t, cs.Token, "expenses", S(exp["id"])), Read(t, cs.Token, "deposits", S(dep["id"]))
		p, rt := Read(t, cs.Token, "purchases", S(pur["id"])), Read(t, cs.Token, "sales-returns", S(ret["id"]))
		sl := Read(t, cs.Token, "sales", S(sale["id"]))
		ae, ad, pp, pnet := F(e, "amount"), F(dp, "amount"), cdPaid(p), F(p, "legacyTotals.net")
		rnet, q, snet := F(rt, "legacyTotals.net"), F(rt, "items.0.qty"), F(sl, "legacyTotals.net")
		acc := cdAccounts(t, cs.Store)
		var bad []string
		eq := func(what string, got, want float64) {
			if Cents(got) != Cents(want) {
				bad = append(bad, fmt.Sprintf("%s %.2f, want %.2f", what, got, want))
			}
		}
		_, b := cdAcctByRef(acc, cat)
		eq("expense category account", b, ae)
		_, b = cdAcctByRef(acc, k1)
		eq("deposit customer account", b, -ad)
		eq("deposit customer creditBalance", salesBalance(t, cs.Store, k1), -ad)
		_, b = cdAcctByRef(acc, vid)
		eq("vendor account", b, -(pnet - pp))
		eq("vendor creditBalance", F(Read(t, cs.Token, "vendors", vid), "creditBalance"), -(pnet - pp))
		eq("purchase paid", F(p, "legacyTotals.paid"), pp)
		_, b = cdAcctByRef(acc, k2)
		eq("returning customer account", b, snet-rnet)
		eq("returning customer creditBalance", salesBalance(t, cs.Store, k2), snet-rnet)
		eq("CASH", cdAcctByName(acc, "CASH"), -ae-pp)
		eq("BANK", cdAcctByName(acc, "BANK"), ad)
		eq("stock", cs.Stock(t, pid), 100+10-10+q)
		eq("sale qtyReturned", F(sl, "items.0.qtyReturned"), q)
		return strings.Join(bad, "; ")
	})
	concRace(t, "NEW-CONC-PATCH-LEDGER", "after two users edited the same documents at the same moment the books do not match the final documents: "+report, report != "")
}

// ---------- 4. returns that together exceed the sale / purchase ----------

// TestConcDocs_OverReturn: 5 sold and 5 bought; four returns of 2 each of the
// sale and of the purchase arrive at the same moment. At most 5 may be
// accepted in total, and stock and the parent's returned quantity must match
// what was accepted.
func TestConcDocs_OverReturn(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager", "r_salesman", "r_cashier")
	a, b := S(cs.Product(t, 10, 20, 100)["id"]), S(cs.Product(t, 10, 20, 0)["id"])
	cid, vid := S(cs.Customer(t, "")["id"]), S(cs.Vendor(t)["id"])
	sale := S(Create(t, cs.Token, "sales", salesBody(cs.Store, cid, []M{cs.Line(a, 5, 20)}, nil))["id"])
	pur := S(Create(t, cs.Token, "purchases", M{"storeId": cs.ID, "date": cs.Now(), "vendorId": vid, "items": []M{cs.Line(b, 5, 10)}})["id"])
	salesWaitStock(t, cs.Store, a, cs.MS, 95)
	salesWaitStock(t, cs.Store, b, cs.MS, 5)
	var mu sync.Mutex
	var srQty, prQty float64
	var calls []func()
	for i, role := range cdRoles["sales"] {
		role := role
		body := M{"storeId": cs.ID, "date": cs.Now(), "orderId": sale, "customerId": cid, "items": []M{cs.Line(a, 2, 20)}}
		calls = append(calls, func() {
			r := Call(t, "POST", "/sales-returns", cs.tok[role], body)
			if r.Code == 201 {
				mu.Lock()
				srQty += 2
				mu.Unlock()
			} else if r.Code != 400 {
				t.Errorf("%s sales return: %s", role, r)
			}
		})
		role = cdRoles["other"][i%2]
		pbody := M{"storeId": cs.ID, "date": cs.Now(), "purchaseId": pur, "vendorId": vid, "items": []M{cs.Line(b, 2, 10)}}
		calls = append(calls, func() {
			r := Call(t, "POST", "/purchase-returns", cs.tok[role], pbody)
			if r.Code == 201 {
				mu.Lock()
				prQty += 2
				mu.Unlock()
			} else if r.Code != 400 {
				t.Errorf("%s purchase return: %s", role, r)
			}
		})
	}
	cdRun(calls)
	concRace(t, "NEW-CONC-SR-OVERRETURN", fmt.Sprintf("sales returns at the same moment accepted %v of a sale of 5: each checks the sale's returned quantity before any of them saves it", srQty), srQty > 5)
	KnownBug(t, "NEW-PR-OVERRETURN", fmt.Sprintf("purchase returns accepted %v of a purchase of 5 (also one after another)", prQty), prQty > 5)
	parent := func(path, id, pid string, stock float64, qty float64) string {
		return cdSettle(func() string {
			got, st := F(Read(t, cs.Token, path, id), "items.0.qtyReturned"), cs.Stock(t, pid)
			if got != qty || st != stock {
				return fmt.Sprintf("%s qtyReturned %v, stock %v; want %v and %v", path, got, st, qty, stock)
			}
			return ""
		})
	}
	msg := parent("sales", sale, a, 95+srQty, srQty)
	concRace(t, "NEW-CONC-SR-PARENT", "sales returns of one sale at the same moment: "+msg, msg != "")
	msg = parent("purchases", pur, b, 5-prQty, prQty)
	concRace(t, "NEW-CONC-PR-PARENT", "purchase returns of one purchase at the same moment: "+msg, msg != "")
}

// ---------- 5. purchase payments that together overpay ----------

// TestConcDocs_PurchaseOverpay: four users pay the same credit purchase at
// the same moment, each from the same read (60 of 115), then each adds
// another 50 to the first payment. The payments are replaced, so the
// purchase must end with 60 and then 110 paid, never more than its net, and
// the vendor's balance and CASH must follow.
func TestConcDocs_PurchaseOverpay(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager")
	pid := S(cs.Product(t, 10, 20, 0)["id"])
	vid := S(cs.Vendor(t)["id"])
	p := Create(t, cs.Token, "purchases", M{"storeId": cs.ID, "date": cs.Now(), "vendorId": vid, "items": []M{cs.Line(pid, 10, 10)}})
	id, net := S(p["id"]), F(p, "legacyTotals.net")
	for step, want := range []float64{60, 110} {
		cur := Objs(Read(t, cs.Token, "purchases", id)["payments"])
		pays := append(cur, M{"date": cs.Now(), "amount": []float64{60, 50}[step], "method": "cash"})
		var calls []func()
		for i := 0; i < 4; i++ {
			role := cdRoles["other"][i%2]
			calls = append(calls, func() {
				if r := cdPatch(t, cs.tok[role], "purchases", id, 0, M{"payments": pays}); r.Code != 200 && r.Code != 400 {
					t.Errorf("%s pays: %s", role, r)
				}
			})
		}
		cdRun(calls)
		msg := cdSettle(func() string {
			d := Read(t, cs.Token, "purchases", id)
			paid, lp := cdPaid(d), F(d, "legacyTotals.paid")
			acc := cdAccounts(t, cs.Store)
			_, va := cdAcctByRef(acc, vid)
			cash := cdAcctByName(acc, "CASH")
			vb := F(Read(t, cs.Token, "vendors", vid), "creditBalance")
			if Cents(paid) != Cents(want) || Cents(lp) != Cents(want) || Cents(va) != Cents(-(net-want)) || Cents(vb) != Cents(-(net-want)) || Cents(cash) != Cents(-want) {
				return fmt.Sprintf("payments %.2f (%d records), paid %.2f, vendor account %.2f, vendor balance %.2f, CASH %.2f; want paid %.2f of net %.2f",
					paid, len(Objs(d["payments"])), lp, va, vb, cash, want, net)
			}
			return ""
		})
		concRace(t, "NEW-CONC-PUR-OVERPAY", fmt.Sprintf("four identical payment edits of one purchase at the same moment (step %d): %s", step+1, msg), msg != "")
	}
}

// ---------- 6. completing one pending transfer twice ----------

// TestConcDocs_TransferComplete: three users complete the same pending stock
// transfer at the same moment (twice, two transfers). Exactly one completion
// may succeed, one transfer is recorded and the stock moves once.
func TestConcDocs_TransferComplete(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager")
	pid := S(cs.Product(t, 10, 20, 10)["id"])
	wh := S(Create(t, cs.Token, "warehouses", M{"storeId": cs.ID, "nameEn": "Branch " + Uniq(), "nameAr": "فرع", "code": "B" + Digits(6)})["id"])
	var wins []int
	for round := 1; round <= 2; round++ {
		pend := Create(t, cs.Token, "stock-transfers", M{"storeId": cs.ID, "date": cs.Now(), "fromWarehouseId": cs.MS, "toWarehouseId": wh,
			"vatPercent": 15, "status": "pending", "items": []M{{"productId": pid, "qty": 3, "unitPrice": 10}}})
		var mu sync.Mutex
		won := 0
		var calls []func()
		for _, role := range []string{"owner", "r_manager", "owner"} {
			role := role
			calls = append(calls, func() {
				r := cdPatch(t, cs.tok[role], "stock-transfers", S(pend["id"]), F(pend, "version"), M{"status": "completed"})
				switch r.Code {
				case 200:
					mu.Lock()
					won++
					mu.Unlock()
				case 404, 409:
				default:
					t.Errorf("%s completes: %s", role, r)
				}
			})
		}
		cdRun(calls)
		wins = append(wins, won)
	}
	time.Sleep(time.Second)
	done := 0
	for _, r := range List(t, cs.Token, "stock-transfers", "storeId="+cs.ID+"&limit=50") {
		if S(r["status"]) == "completed" {
			done++
		}
	}
	_, msOK := salesStockSettles(t, cs.Store, pid, cs.MS, 4)
	_, whOK := salesStockSettles(t, cs.Store, pid, wh, 6)
	ms, w := salesStockAt(t, cs.Store, pid, cs.MS), salesStockAt(t, cs.Store, pid, wh)
	concRace(t, "NEW-CONC-TRANSFER-COMPLETE", fmt.Sprintf("three completions of one pending transfer at the same moment: %v succeeded per round, %d completed transfers recorded for 2, "+
		"stock main %v / branch %v (want 4 / 6): erp/res_rest.go stockTransfersBackend.Update creates the legacy transfer before it removes the pending one", wins, done, ms, w),
		wins[0] > 1 || wins[1] > 1 || done != 2 || !msOK || !whOK)
	if wins[0] == 0 || wins[1] == 0 {
		t.Errorf("no completion succeeded: %v", wins)
	}
}

// ---------- 7. finalizing one draft twice ----------

// TestConcDocs_DraftDoubleFinalize: three users finalize the same sales,
// quotation and deposit draft at the same moment (no Idempotency-Key).
// Exactly one document may be created per draft.
func TestConcDocs_DraftDoubleFinalize(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager", "r_salesman", "r_accountant")
	pid := S(cs.Product(t, 10, 20, 50)["id"])
	cid := S(cs.Customer(t, "")["id"])
	drafts := []struct {
		typ   string
		users []string
		pl    M
	}{
		{"sales", []string{"owner", "r_manager", "r_salesman"}, M{"date": cs.Now(), "customerId": cid, "items": []M{cs.Line(pid, 2, 20)}}},
		{"quotations", []string{"owner", "r_manager", "r_salesman"}, M{"date": cs.Now(), "customerId": cid, "validityDays": 5, "deliveryDays": 2, "items": []M{cs.Line(pid, 2, 20)}}},
		{"deposits", []string{"owner", "r_manager", "r_accountant"}, M{"date": cs.Now(), "customerId": cid, "amount": 75, "method": "cash"}},
	}
	created := map[string]int{}
	var mu sync.Mutex
	var calls []func()
	for _, d := range drafts {
		d := d
		id := S(Create(t, cs.Token, "drafts/"+d.typ, M{"storeId": cs.ID, "payload": d.pl})["id"])
		for _, u := range d.users {
			u := u
			calls = append(calls, func() {
				r := Call(t, "POST", "/drafts/"+d.typ+"/"+id+"/finalize?storeId="+cs.ID, cs.tok[u], nil)
				switch r.Code {
				case 201:
					mu.Lock()
					created[d.typ]++
					mu.Unlock()
				case 404, 409:
				default:
					t.Errorf("%s finalizes %s draft: %s", u, d.typ, r)
				}
			})
		}
	}
	cdRun(calls)
	var extra []string
	for _, d := range drafts {
		n := cdTotal(t, cs.Store, d.typ)
		if created[d.typ] != 1 || n != 1 {
			extra = append(extra, fmt.Sprintf("%s: %d finalizes answered 201, %d documents", d.typ, created[d.typ], n))
		}
		if created[d.typ] == 0 {
			t.Errorf("%s draft: no finalize succeeded", d.typ)
		}
	}
	got, stockOK := salesStockSettles(t, cs.Store, pid, cs.MS, 50-2*float64(created["sales"]))
	concRace(t, "NEW-CONC-STOCK-MOVES", fmt.Sprintf("%d sales of one product created at the same moment (finalized drafts): stock %v, want %v",
		created["sales"], got, 50-2*float64(created["sales"])), !stockOK)
	concRace(t, "NEW-CONC-DRAFT-FINALIZE", "one draft finalized by several users at the same moment creates several documents ("+strings.Join(extra, "; ")+
		"): erp/drafts.go handleDraftFinalize loads the draft, creates the document and only then deletes the draft", len(extra) > 0)
}

// ---------- 8 & 9. stock under product edits, sales and purchases ----------

// TestConcDocs_ProductStock: sales and purchases of two products in two
// warehouses at the same moment, while two users edit one of the products
// (name and price, not stock): stock per warehouse must equal the opening
// stock plus purchases minus sales. Then two users set a third product's
// stock (absolute) at the same moment: one of the two values must stand.
// Then one user sets it while it is sold and bought: the result must be the
// set value plus some of the movements.
func TestConcDocs_ProductStock(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_manager", "r_salesman", "r_cashier")
	wh := S(Create(t, cs.Token, "warehouses", M{"storeId": cs.ID, "nameEn": "Branch " + Uniq(), "nameAr": "فرع", "code": "B" + Digits(6)})["id"])
	cid, vid := S(cs.Customer(t, "")["id"]), S(cs.Vendor(t)["id"])
	whs := []string{cs.MS, wh}
	mk := func() string {
		return S(Create(t, cs.Token, "products", M{"storeId": cs.ID, "nameEn": "Stock " + Uniq(), "nameAr": "منتج",
			"pricing": M{"purchase": 10, "retail": 20}, "stock": M{cs.MS: M{"qty": 100}, wh: M{"qty": 50}}})["id"])
	}
	p1, p2 := mk(), mk()
	want := map[string]float64{p1 + cs.MS: 100, p1 + wh: 50, p2 + cs.MS: 100, p2 + wh: 50}
	line := func(pid, w string, q, price float64) M { l := cs.Line(pid, q, price); l["warehouseId"] = w; return l }
	var calls []func()
	for i := 0; i < 8; i++ {
		role := cdRoles["sales"][i%4]
		w1, w2 := whs[i%2], whs[(i+1)%2]
		want[p1+w1] -= 1
		want[p2+w2] -= 2
		body := salesBody(cs.Store, cid, []M{line(p1, w1, 1, 20), line(p2, w2, 2, 20)}, nil)
		calls = append(calls, func() {
			if r := Call(t, "POST", "/sales", cs.tok[role], body); r.Code != 201 {
				t.Errorf("%s sale: %s", role, r)
			}
		})
	}
	for i := 0; i < 6; i++ {
		role := cdRoles["other"][i%2]
		w1, w2 := whs[(i+1)%2], whs[i%2]
		want[p1+w1] += 3
		want[p2+w2] += 4
		body := M{"storeId": cs.ID, "date": cs.Now(), "vendorId": vid, "items": []M{line(p1, w1, 3, 10), line(p2, w2, 4, 10)}}
		calls = append(calls, func() {
			if r := Call(t, "POST", "/purchases", cs.tok[role], body); r.Code != 201 {
				t.Errorf("%s purchase: %s", role, r)
			}
		})
	}
	for i := 0; i < 4; i++ {
		role, body := cdRoles["other"][i%2], M{"nameEn": fmt.Sprintf("Edited %d %s", i, Uniq()), "pricing": M{"purchase": 10, "retail": 20 + i}}
		calls = append(calls, func() {
			if r := cdPatch(t, cs.tok[role], "products", p1, 0, body); r.Code != 200 {
				t.Errorf("%s edits the product: %s", role, r)
			}
		})
	}
	cdRun(calls)
	if t.Failed() {
		t.FailNow()
	}
	check := func(pid string) string {
		return cdSettle(func() string {
			var bad []string
			for _, w := range whs {
				if got := salesStockAt(t, cs.Store, pid, w); Cents(got) != Cents(want[pid+w]) {
					bad = append(bad, fmt.Sprintf("%s: %v, want %v", w, got, want[pid+w]))
				}
			}
			return strings.Join(bad, ", ")
		})
	}
	m1, m2 := check(p1), check(p2)
	concRace(t, "NEW-CONC-STOCK-MOVES", "sales and purchases of one product at the same moment leave its stock wrong (product saved whole with $set after a "+
		"stale read, models/product.go Product.Update): "+m2, m2 != "")
	concRace(t, "NEW-CONC-PRODUCT-PATCH-STOCK", "product edits (name, price) at the same moment as its sales and purchases leave its stock wrong: "+m1, m1 != "" && m2 == "")

	// two absolute stock edits at the same moment
	p3 := S(cs.Product(t, 10, 20, 100)["id"])
	calls = nil
	for i, q := range []float64{40, 70} {
		role, body := cdRoles["other"][i], M{"stock": M{cs.MS: M{"qty": q}}}
		calls = append(calls, func() {
			if r := cdPatch(t, cs.tok[role], "products", p3, 0, body); r.Code != 200 {
				t.Errorf("%s sets stock: %s", role, r)
			}
		})
	}
	cdRun(calls)
	time.Sleep(2 * time.Second)
	got := salesStockAt(t, cs.Store, p3, cs.MS)
	concRace(t, "NEW-CONC-STOCK-ABS", fmt.Sprintf("two users set a product's stock to 40 and 70 at the same moment: stock %v (each edit is turned into an adjustment "+
		"from the stock it read, erp/res_master.go productToLegacy)", got), got != 40 && got != 70)

	// an absolute edit while the product is sold and bought
	calls = nil
	calls = append(calls, func() {
		if r := cdPatch(t, cs.Token, "products", p3, 0, M{"stock": M{cs.MS: M{"qty": 30}}}); r.Code != 200 {
			t.Errorf("owner sets stock: %s", r)
		}
	})
	for i := 0; i < 4; i++ {
		role, body := cdRoles["sales"][1+i%3], salesBody(cs.Store, cid, []M{cs.Line(p3, 1, 20)}, nil)
		calls = append(calls, func() {
			if r := Call(t, "POST", "/sales", cs.tok[role], body); r.Code != 201 {
				t.Errorf("%s sale: %s", role, r)
			}
		})
	}
	for i := 0; i < 3; i++ {
		role, body := cdRoles["other"][i%2], M{"storeId": cs.ID, "date": cs.Now(), "vendorId": vid, "items": []M{cs.Line(p3, 2, 10)}}
		calls = append(calls, func() {
			if r := Call(t, "POST", "/purchases", cs.tok[role], body); r.Code != 201 {
				t.Errorf("%s purchase: %s", role, r)
			}
		})
	}
	cdRun(calls)
	time.Sleep(3 * time.Second)
	got = salesStockAt(t, cs.Store, p3, cs.MS)
	ok := false
	for s := 0; s <= 4; s++ {
		for p := 0; p <= 3; p++ {
			ok = ok || got == float64(30-s+2*p)
		}
	}
	concRace(t, "NEW-CONC-STOCK-ABS", fmt.Sprintf("stock set to 30 while 4 × 1 were sold and 3 × 2 bought at the same moment: stock %v, want 30 plus some of those movements", got), !ok)
}

// ---------- 10. two admins edit store settings ----------

// TestConcDocs_StorePatch: the owner and a second admin PATCH different
// settings of the store at the same moment (branch name, e-mail), five
// times. Both changes must survive every round.
func TestConcDocs_StorePatch(t *testing.T) {
	t.Parallel()
	cs := cdSetup(t, "SA", "r_admin")
	var lost []string
	for round := 1; round <= 5; round++ {
		branch, email := fmt.Sprintf("Branch %d %s", round, Uniq()), fmt.Sprintf("shop%d%s@e2e.example", round, Digits(4))
		cdRun([]func(){
			func() {
				if r := cdPatch(t, cs.tok["owner"], "stores", cs.ID, 0, M{"branchEn": branch}); r.Code != 200 {
					t.Errorf("owner patches the branch: %s", r)
				}
			},
			func() {
				if r := cdPatch(t, cs.tok["r_admin"], "stores", cs.ID, 0, M{"email": email}); r.Code != 200 {
					t.Errorf("admin patches the e-mail: %s", r)
				}
			},
		})
		st := Read(t, cs.Token, "stores", cs.ID)
		if S(st["branchEn"]) != branch || S(st["email"]) != email {
			lost = append(lost, fmt.Sprintf("round %d: branch %q email %q", round, st["branchEn"], st["email"]))
		}
	}
	concRace(t, "NEW-CONC-STORE-PATCH", "two admins editing different store settings at the same moment lose one change (the store is saved whole): "+strings.Join(lost, "; "), len(lost) > 0)
}
