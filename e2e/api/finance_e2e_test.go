//go:build e2e

package api

// finance_e2e_test.go — money, reports and countries: expenses, debit/credit
// notes, capital, dividends, salaries and employees, the read-only ledger
// accounts, every dashboard figure against an oracle worked out here from the
// inputs, the seven supported countries, subscription billing and the starter
// catalog.

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jameskeane/bcrypt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------- helpers (fin prefix: shared package) ----------

// finQS is "storeId=<id>" plus extra query parameters.
func finQS(s *Store, extra string) string {
	q := "storeId=" + s.ID
	if extra != "" {
		q += "&" + extra
	}
	return q
}

// finStats GETs /<path>/stats for the store.
func finStats(t testing.TB, s *Store, path, extra string) M {
	t.Helper()
	return Must(t, Call(t, "GET", "/"+path+"/stats?"+finQS(s, extra), s.Token, nil), 200, "stats "+path).Body
}

// finDash GETs a dashboard figure, rebuilt now (?fresh=1).
func finDash(t testing.TB, s *Store, kind, extra string) M {
	t.Helper()
	return Must(t, Call(t, "GET", "/dashboard/"+kind+"?fresh=1&"+finQS(s, extra), s.Token, nil), 200, "dashboard "+kind).Body
}

func finPeriod(from, to string) string {
	v := url.Values{}
	if from != "" {
		v.Set("from", from)
	}
	if to != "" {
		v.Set("to", to)
	}
	return v.Encode()
}

// finDay is the store day n days from today (store time).
func finDay(s *Store, n int) string {
	return time.Now().In(s.Loc).AddDate(0, 0, n).Format("2006-01-02")
}

func finR2(x float64) float64 { return float64(Cents(x)) / 100 }

// finR3 rounds to 3 decimals (OMR, BHD, KWD), half away from zero.
func finR3(x float64) float64 { return math.Round(x*1000) / 1000 }

func finEq3(t testing.TB, what string, got, want float64) {
	t.Helper()
	if math.Round(got*1000) != math.Round(want*1000) {
		t.Errorf("%s: got %.3f, want %.3f", what, got, want)
	}
}

// finAccounts reads the store's ledger accounts keyed by name.
func finAccounts(t testing.TB, s *Store) map[string]M {
	t.Helper()
	out := map[string]M{}
	for _, a := range List(t, s.Token, "accounts", finQS(s, "limit=500")) {
		out[S(a["nameEn"])] = a
	}
	return out
}

// finSigned is an account's balance, debit positive and credit negative.
func finSigned(a M) float64 {
	b := F(a, "balance")
	if S(a["debitOrCredit"]) == "credit_balance" {
		return -b
	}
	return b
}

func finAcctBy(accts map[string]M, key, val string) M {
	for _, a := range accts {
		if S(a[key]) == val {
			return a
		}
	}
	return nil
}

func finExpenseCat(t testing.TB, s *Store, name string) string {
	t.Helper()
	return S(Create(t, s.Token, "expense-categories", M{"nameEn": name})["id"])
}

func finEmployee(t testing.TB, s *Store, name string, salary float64, join string) M {
	t.Helper()
	return Create(t, s.Token, "employees", M{"storeId": s.ID, "nameEn": name, "nameAr": "موظف", "joinDate": join,
		"basicSalary": salary, "status": "active", "phone": "05" + Digits(8)})
}

// finSetFlags switches store settings (store.flags).
func finSetFlags(t testing.TB, s *Store, flags M) {
	t.Helper()
	cur := Read(t, s.Token, "stores", s.ID)
	r := Call(t, "PATCH", "/stores/"+s.ID, s.Token, M{"flags": flags}, "If-Match", Num(cur["version"]).String())
	Must(t, r, 200, "store flags")
	for k, v := range flags {
		if Get(r.Body, "flags."+k) != v {
			t.Fatalf("flag %s=%v not saved: %v", k, v, r.Body["flags"])
		}
	}
}

// finListChecks lists each path for the store (200) and for a store the
// caller may not use (403).
func finListChecks(t testing.TB, s, other *Store, paths ...string) {
	t.Helper()
	for _, p := range paths {
		List(t, s.Token, p, finQS(s, "limit=5"))
		r := Call(t, "GET", "/"+p+"?storeId="+other.ID, s.Token, nil)
		if p != "expense-categories" && p != "accounts" {
			Must(t, r, 403, "list "+p+" of another company's store")
			continue
		}
		// company-wide lists ignore a foreign storeId: only the caller's own rows
		Must(t, r, 200, "list "+p+" with another company's storeId")
		theirs := map[string]bool{}
		for _, x := range List(t, other.Token, p, "limit=500") {
			theirs[S(x["id"])] = true
		}
		for _, x := range r.Data() {
			if theirs[S(x["id"])] {
				t.Errorf("%s: another company's row %v listed", p, x["id"])
			}
		}
		// an error answer: an unknown sort field
		if r := Call(t, "GET", "/"+p+"?sort=bogusField", s.Token, nil); r.Code != 400 {
			t.Errorf("%s sort=bogusField: %s", p, r)
		}
	}
}

// finOwnerName is the owner's user name (investors are legacy users).
const finOwnerName = "E2E Owner"

var (
	finAdminOnce sync.Once
	finAdminTok  string
	finAdminWhy  string
)

// finPlatformAdmin returns a platform admin's token: E2E_ADMIN_EMAIL /
// E2E_ADMIN_PASSWORD when set, else a legacy Admin user written straight into
// the server's main database (E2E_MONGO_URI or MONGO_HOST:MONGO_PORT, database
// E2E_MONGO_DB or MONGO_DB or t1_e2e). "" (with the reason) when neither works:
// sign-up never makes a platform admin.
func finPlatformAdmin(t testing.TB) string {
	t.Helper()
	finAdminOnce.Do(func() {
		if e, p := os.Getenv("E2E_ADMIN_EMAIL"), os.Getenv("E2E_ADMIN_PASSWORD"); e != "" && p != "" {
			r := Call(t, "POST", "/auth/login", "", M{"email": e, "password": p})
			if r.Code == 200 {
				finAdminTok = S(r.Body["accessToken"])
				return
			}
			finAdminWhy = "E2E_ADMIN_EMAIL login failed: " + r.String()
			return
		}
		uri := os.Getenv("E2E_MONGO_URI")
		if uri == "" {
			uri = "mongodb://" + envOr("MONGO_HOST", "127.0.0.1") + ":" + envOr("MONGO_PORT", "27017")
		}
		dbName := envOr("E2E_MONGO_DB", envOr("MONGO_DB", "t1_e2e"))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cl, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetServerSelectionTimeout(5*time.Second))
		if err != nil {
			finAdminWhy = "no MongoDB: " + err.Error()
			return
		}
		defer cl.Disconnect(context.Background())
		email := "e2e-platform-admin+" + Uniq() + Digits(4) + "@e2e.example"
		pw := "Adm1n!" + Digits(6)
		hash, err := bcrypt.Hash(pw)
		if err != nil {
			finAdminWhy = "bcrypt: " + err.Error()
			return
		}
		now := time.Now()
		if _, err := cl.Database(dbName).Collection("user").InsertOne(ctx, bson.M{"_id": primitive.NewObjectID(),
			"name": "E2E Platform Admin", "role": "Admin", "email": email, "mob": "05" + Digits(8), "password": hash,
			"admin": true, "created_at": now, "updated_at": now}); err != nil {
			finAdminWhy = "insert admin: " + err.Error()
			return
		}
		r := Call(t, "POST", "/auth/login", "", M{"email": email, "password": pw})
		if r.Code != 200 {
			finAdminWhy = "admin written to " + dbName + " but login failed (server uses another database?): " + r.String()
			return
		}
		finAdminTok = S(r.Body["accessToken"])
	})
	if finAdminTok == "" {
		t.Logf("no platform admin available (%s): admin-only success paths not exercised", finAdminWhy)
	}
	return finAdminTok
}

// finPNG is a 1×1 PNG as a data URL.
const finPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// ---------- expenses and expense categories ----------

func TestFinanceExpenses(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	cat := finExpenseCat(t, s, "Rent "+Uniq())
	cat2 := finExpenseCat(t, s, "إيجار "+Uniq()) // Arabic category name
	otherCat := finExpenseCat(t, other, "Other "+Uniq())
	vendor := s.Vendor(t)
	today := s.Now()
	exp := func(extra M) M {
		b := M{"storeId": s.ID, "date": today, "categoryId": cat, "description": "Office rent", "amount": 100.0, "method": "cash"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}

	t.Run("validation", func(t *testing.T) {
		cases := []struct {
			name   string
			body   M
			fields []string
		}{
			{"empty", M{"storeId": s.ID}, []string{"categoryId", "date", "amount", "method"}},
			{"zero amount", exp(M{"amount": 0}), []string{"amount"}},
			{"negative amount", exp(M{"amount": -5}), []string{"amount"}},
			{"negative VAT", exp(M{"vatAmount": -1}), []string{"vatAmount"}},
			{"unknown method", exp(M{"method": "bitcoin"}), []string{"method"}},
			{"no method", exp(M{"method": ""}), []string{"method"}},
			{"no date", exp(M{"date": ""}), []string{"date"}},
		}
		for _, c := range cases {
			r := Call(t, "POST", "/expenses", s.Token, c.body)
			if r.Code != 400 || r.ErrCode() != "validation" {
				t.Errorf("%s: want 400 validation, got %s", c.name, r)
				continue
			}
			for _, f := range c.fields {
				if r.ErrField(f) == "" {
					t.Errorf("%s: error.fields.%s missing: %s", c.name, f, r)
				}
			}
		}
		// a category of another company
		r := Call(t, "POST", "/expenses", s.Token, exp(M{"categoryId": otherCat}))
		if r.Code != 400 {
			t.Fatalf("other company's category: want 400, got %s", r)
		}
		KnownBug(t, "NEW-fin-expcat-field", "unknown expense category is reported as error.fields.category_id_0 (legacy key), not categoryId",
			r.ErrField("categoryId") == "" && r.ErrField("category_id_0") != "")
		// no token
		Must(t, Call(t, "POST", "/expenses", "", exp(nil)), 401, "expense without token")
	})

	var plain, withVendor M
	t.Run("create without VAT", func(t *testing.T) {
		plain = Create(t, s.Token, "expenses", exp(M{"amount": 250.5, "description": "مياه وكهرباء"}))
		EqMoney(t, "amount", F(plain, "amount"), 250.5)
		EqMoney(t, "vatAmount", F(plain, "vatAmount"), 0)
		if !strings.HasPrefix(S(plain["code"]), "EXP-") || S(plain["description"]) != "مياه وكهرباء" ||
			S(plain["categoryId"]) != cat || plain["vendorId"] != nil || Num(plain["version"]) != 1 {
			t.Errorf("created expense: %v", plain)
		}
		// a blank description falls back to the category name (legacy requires one)
		e := Create(t, s.Token, "expenses", exp(M{"categoryId": cat2, "description": "", "amount": 1}))
		if !strings.HasPrefix(S(e["description"]), "إيجار") {
			t.Errorf("blank description: %q, want the category name", e["description"])
		}
		if S(e["code"]) <= S(plain["code"]) {
			t.Errorf("expense numbers must increase: %v then %v", plain["code"], e["code"])
		}
	})

	t.Run("create with VAT on a vendor invoice", func(t *testing.T) {
		// 200 + 15% = 230: the legacy amount is VAT-inclusive, its VAT is
		// worked out back from it at the store rate
		withVendor = Create(t, s.Token, "expenses", exp(M{"amount": 200, "vatAmount": 30, "vendorId": vendor["id"], "reference": "VINV-77"}))
		gross := 230.0
		vat := finR2(finR2(gross/1.15) * 0.15)
		EqMoney(t, "amount (before VAT)", F(withVendor, "amount"), gross-vat)
		EqMoney(t, "vatAmount", F(withVendor, "vatAmount"), vat)
		if S(withVendor["vendorId"]) != S(vendor["id"]) || S(withVendor["reference"]) != "VINV-77" || S(withVendor["payee"]) != S(vendor["nameEn"]) {
			t.Errorf("vendor expense: %v", withVendor)
		}
	})

	t.Run("VAT entered is not kept", func(t *testing.T) {
		// no vendor: the VAT entered is folded into the amount and dropped
		e := Create(t, s.Token, "expenses", exp(M{"amount": 100, "vatAmount": 15}))
		EqMoney(t, "gross kept", F(e, "amount")+F(e, "vatAmount"), 115)
		KnownBug(t, "NEW-fin-expense-vat", "POST /expenses without vendorId: vatAmount 15 comes back as amount 115, vatAmount 0",
			Cents(F(e, "vatAmount")) == 0)
		// vendor: VAT recomputed at 15% of the gross, the figure entered is ignored
		e = Create(t, s.Token, "expenses", exp(M{"amount": 100, "vatAmount": 5, "vendorId": vendor["id"]}))
		EqMoney(t, "gross kept", F(e, "amount")+F(e, "vatAmount"), 105)
		KnownBug(t, "NEW-fin-expense-vat", "POST /expenses with vendorId: amount 100 + vatAmount 5 comes back as 91.31 + 13.69",
			Cents(F(e, "vatAmount")) != 500)
		// a vendor id that does not exist is accepted
		r := Call(t, "POST", "/expenses", s.Token, exp(M{"vendorId": primitive.NewObjectID().Hex()}))
		KnownBug(t, "NEW-fin-expense-vendor", "POST /expenses accepts a vendorId that does not exist (201)", r.Code == 201)
	})

	t.Run("read, list, filters, stats", func(t *testing.T) {
		g := Read(t, s.Token, "expenses", S(plain["id"]))
		if S(g["id"]) != S(plain["id"]) || S(g["code"]) != S(plain["code"]) {
			t.Errorf("get: %v", g)
		}
		Must(t, Call(t, "GET", "/expenses/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "unknown expense")
		Must(t, Call(t, "GET", "/expenses/not-an-id", s.Token, nil), 404, "bad expense id")
		Must(t, Call(t, "GET", "/expenses/"+S(plain["id"]), other.Token, nil), 404, "expense of another company")
		// another company's list never shows them
		for _, e := range List(t, other.Token, "expenses", finQS(other, "")) {
			if S(e["id"]) == S(plain["id"]) {
				t.Fatal("expense leaked to another company")
			}
		}
		Must(t, Call(t, "GET", "/expenses?storeId="+s.ID, other.Token, nil), 403, "list of a store the user cannot use")

		all := List(t, s.Token, "expenses", finQS(s, "limit=500"))
		var gross, vat float64
		for _, e := range all {
			gross += F(e, "amount")
			vat += F(e, "vatAmount")
		}
		st := finStats(t, s, "expenses", "sum=amount,vatAmount,one&"+finPeriod(s.Today(), s.Today()))
		if int(F(st, "count")) != len(all) || int(F(st, "sums.one")) != len(all) {
			t.Errorf("stats count %v, list %d", st["count"], len(all))
		}
		EqMoney(t, "stats amount", F(st, "sums.amount"), gross)
		EqMoney(t, "stats VAT", F(st, "sums.vatAmount"), vat)
		// a period with nothing in it
		st = finStats(t, s, "expenses", "sum=amount&"+finPeriod("2020-01-01", "2020-01-31"))
		if F(st, "count") != 0 || F(st, "sums.amount") != 0 {
			t.Errorf("empty period stats: %v", st)
		}
		if r := Call(t, "GET", "/expenses/stats?"+finQS(s, finPeriod("2026-02-01", "2026-01-01")), s.Token, nil); r.Code != 400 || r.ErrField("from") == "" {
			t.Errorf("stats from > to: %s", r)
		}
		if r := Call(t, "GET", "/expenses/stats?"+finQS(s, "lines=bogus"), s.Token, nil); r.Code != 400 || r.ErrField("lines") == "" {
			t.Errorf("stats bad lines: %s", r)
		}
		// search, paging and select
		// the expenses list has no search; the tiles search like the list screen
		if r := Call(t, "GET", "/expenses?"+finQS(s, "q=x"), s.Token, nil); r.Code != 400 || r.ErrField("q") == "" {
			t.Errorf("list search: %s", r)
		}
		hit := finStats(t, s, "expenses", "sum=amount&page=1&keys=description&q="+url.QueryEscape("مياه"))
		if F(hit, "count") != 1 || S(Get(hit, "ids.0")) != S(plain["id"]) {
			t.Errorf("search by Arabic description: %v", hit)
		}
		p1 := Must(t, Call(t, "GET", "/expenses?"+finQS(s, "limit=2&page=1"), s.Token, nil), 200, "page 1")
		p2 := Must(t, Call(t, "GET", "/expenses?"+finQS(s, "limit=2&page=2"), s.Token, nil), 200, "page 2")
		if len(p1.Data()) != 2 || int(F(p1.Body, "total")) != len(all) || (len(p2.Data()) > 0 && S(p2.Data()[0]["id"]) == S(p1.Data()[0]["id"])) {
			t.Errorf("paging: %d/%v then %d", len(p1.Data()), p1.Body["total"], len(p2.Data()))
		}
		sel := List(t, s.Token, "expenses", finQS(s, "select=id,amount&limit=1"))
		if len(sel) != 1 || sel[0]["description"] != nil || sel[0]["amount"] == nil {
			t.Errorf("select=id,amount: %v", sel)
		}
		if r := Call(t, "GET", "/expenses?"+finQS(s, "to=31-12-2026"), s.Token, nil); r.Code != 400 || r.ErrField("to") == "" {
			t.Errorf("bad to date: %s", r)
		}
		// no expense is dated after today
		if rows := List(t, s.Token, "expenses", finQS(s, "from="+finDay(s, 1))); len(rows) != 0 {
			t.Errorf("from tomorrow: %d rows", len(rows))
		}
	})

	t.Run("patch, put, version, delete", func(t *testing.T) {
		id := S(plain["id"])
		p := Patch(t, s.Token, "expenses", id, M{"description": "Rent – October", "amount": 300})
		EqMoney(t, "patched amount", F(p, "amount"), 300)
		if S(p["description"]) != "Rent – October" || Num(p["version"]) <= Num(plain["version"]) {
			t.Errorf("patched: %v", p)
		}
		hist := Objs(p["history"])
		// the action is the X-Change-Reason sent with the change
		if len(hist) < 2 || S(hist[len(hist)-1]["action"]) != "e2e" || len(Objs(hist[len(hist)-1]["changes"])) != 2 {
			t.Errorf("history after patch: %v", p["history"])
		}
		// a stale version
		r := Call(t, "PATCH", "/expenses/"+id, s.Token, M{"amount": 1}, "If-Match", "1")
		if r.Code != 409 || r.ErrCode() != "version_conflict" {
			t.Errorf("stale If-Match: %s", r)
		}
		// PATCH validation
		if r := PatchResp(t, s.Token, "expenses", id, M{"amount": -3}); r.Code != 400 || r.ErrField("amount") == "" {
			t.Errorf("patch negative amount: %s", r)
		}
		// PUT replaces the record: a partial body fails validation
		cur := Read(t, s.Token, "expenses", id)
		if r := Call(t, "PUT", "/expenses/"+id, s.Token, M{"description": "only"}, "If-Match", Num(cur["version"]).String()); r.Code != 400 || r.ErrField("amount") == "" {
			t.Errorf("partial PUT: %s", r)
		}
		put := Must(t, Call(t, "PUT", "/expenses/"+id, s.Token, exp(M{"amount": 75.25, "method": "bank_transfer", "description": "Rent PUT"}),
			"If-Match", Num(cur["version"]).String()), 200, "PUT expense").Body
		EqMoney(t, "PUT amount", F(put, "amount"), 75.25)
		if S(put["method"]) != "bank_transfer" || S(put["code"]) != S(plain["code"]) {
			t.Errorf("PUT: %v", put)
		}
		Must(t, Call(t, "PATCH", "/expenses/"+primitive.NewObjectID().Hex(), s.Token, M{"amount": 1}), 404, "patch unknown")
		Must(t, Call(t, "PUT", "/expenses/"+id, other.Token, exp(nil)), 404, "PUT by another company")

		before := finStats(t, s, "expenses", "sum=amount")
		d := Must(t, Call(t, "DELETE", "/expenses/"+id, s.Token, nil), 200, "delete").Body
		if d["deleted"] != true {
			t.Errorf("deleted flag: %v", d)
		}
		after := finStats(t, s, "expenses", "sum=amount")
		if F(after, "count") != F(before, "count")-1 {
			t.Errorf("deleted expense still counted: %v → %v", before["count"], after["count"])
		}
		EqMoney(t, "stats after delete", F(after, "sums.amount"), F(before, "sums.amount")-75.25)
		inc := finStats(t, s, "expenses", "sum=one&includeDeleted=1")
		if F(inc, "count") != F(before, "count") {
			t.Errorf("includeDeleted count %v want %v", inc["count"], before["count"])
		}
		if r := Call(t, "POST", "/expenses/"+id+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore expense (legacy has none): %s", r)
		}
		Must(t, Call(t, "DELETE", "/expenses/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "delete unknown")
		Must(t, Call(t, "DELETE", "/expenses/"+S(withVendor["id"]), other.Token, nil), 404, "delete by another company")
		Must(t, Call(t, "DELETE", "/expenses/"+id+"?hard=1", s.Token, nil), 204, "hard delete")
		Must(t, Call(t, "GET", "/expenses/"+id, s.Token, nil), 404, "after hard delete")
	})

	t.Run("expense categories", func(t *testing.T) {
		r := Call(t, "POST", "/expense-categories", s.Token, M{"nameEn": "   "})
		if r.Code != 400 || r.ErrField("nameEn") == "" {
			t.Errorf("blank category: %s", r)
		}
		c := Create(t, s.Token, "expense-categories", M{"nameEn": "Fuel " + Uniq()})
		p := Patch(t, s.Token, "expense-categories", S(c["id"]), M{"nameEn": "Fuel & Oil"})
		if S(p["nameEn"]) != "Fuel & Oil" {
			t.Errorf("patched category: %v", p)
		}
		found := false
		for _, x := range List(t, s.Token, "expense-categories", "") {
			if S(x["id"]) == S(c["id"]) {
				found = true
			}
			if S(x["id"]) == otherCat {
				t.Error("another company's expense category listed")
			}
		}
		if !found {
			t.Error("new category not listed")
		}
		cur := Read(t, s.Token, "expense-categories", S(c["id"]))
		put := Call(t, "PUT", "/expense-categories/"+S(c["id"]), s.Token, M{"nameEn": "Fuel PUT"}, "If-Match", Num(cur["version"]).String())
		if put.Code != 200 || S(put.Body["nameEn"]) != "Fuel PUT" {
			t.Errorf("PUT category: %s", put)
		}
		Must(t, Call(t, "GET", "/expense-categories/"+otherCat, s.Token, nil), 404, "another company's category")
		finListChecks(t, s, other, "expense-categories")
		if r := PatchResp(t, s.Token, "expense-categories", S(c["id"]), M{"nameEn": ""}); r.Code != 400 || r.ErrField("nameEn") == "" {
			t.Errorf("patch blank category: %s", r)
		}
		Must(t, Call(t, "PUT", "/expense-categories/"+S(c["id"]), s.Token, M{"nameEn": " "}), 400, "PUT blank category")
		Must(t, Call(t, "DELETE", "/expense-categories/"+otherCat, s.Token, nil), 404, "delete another company's category")
		Must(t, Call(t, "DELETE", "/expense-categories/"+S(c["id"]), s.Token, nil), 200, "delete category")
		if r := Call(t, "POST", "/expense-categories/"+S(c["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore category: %s", r)
		}
	})
}

// ---------- debit notes (deposits) and credit notes (withdrawals) ----------

func TestFinanceDepositsWithdrawals(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	c := s.Customer(t, "")
	cid := S(c["id"])
	otherCust := other.Customer(t, "")
	p := s.Product(t, 30, 50, 20)
	// 2 × 50 = 100 + 15% VAT = 115, 15 paid now
	sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": cid,
		"items": []M{s.Line(S(p["id"]), 2, 50)}, "payments": []M{{"date": s.Now(), "amount": 15, "method": "cash"}}})
	EqMoney(t, "sale net", F(sale, "legacyTotals.net"), 115)
	balance := func() float64 { return F(Read(t, s.Token, "customers", cid), "creditBalance") }
	Eventually(t, "customer owes the unpaid 100", func() bool { return Cents(balance()) == 10000 })
	note := func(extra M) M {
		b := M{"storeId": s.ID, "date": s.Now(), "customerId": cid, "amount": 10.0, "method": "cash"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}

	t.Run("validation", func(t *testing.T) {
		for _, path := range []string{"deposits", "withdrawals"} {
			cases := []struct {
				body   M
				fields []string
			}{
				{M{"storeId": s.ID}, []string{"customerId", "date", "amount", "method"}},
				{note(M{"amount": 0}), []string{"amount"}},
				{note(M{"amount": -10}), []string{"amount"}},
				{note(M{"method": "gold"}), []string{"method"}},
				{note(M{"customerId": ""}), []string{"customerId"}},
			}
			for i, c := range cases {
				r := Call(t, "POST", "/"+path, s.Token, c.body)
				if r.Code != 400 {
					t.Errorf("%s case %d: want 400, got %s", path, i, r)
					continue
				}
				for _, f := range c.fields {
					if r.ErrField(f) == "" {
						t.Errorf("%s case %d: error.fields.%s missing: %s", path, i, f, r)
					}
				}
			}
			for _, bad := range []string{primitive.NewObjectID().Hex(), S(otherCust["id"])} {
				r := Call(t, "POST", "/"+path, s.Token, note(M{"customerId": bad}))
				KnownBug(t, "NEW-fin-note-customer", "POST /"+path+" with a customerId that is not a customer of the store answers 500 (want 400 customerId)",
					r.Code == 500)
				if r.Code == 201 {
					t.Errorf("%s accepted a foreign customer: %s", path, r)
				}
			}
		}
	})

	var settle, advance, refund M
	t.Run("debit note settles the invoice", func(t *testing.T) {
		settle = Create(t, s.Token, "deposits", note(M{"amount": 100, "orderId": sale["id"], "reference": "TRX-1", "notes": "دفعة"}))
		if S(settle["orderId"]) != S(sale["id"]) || S(settle["orderCode"]) != S(sale["code"]) || S(settle["partyType"]) != "customer" ||
			!strings.HasPrefix(S(settle["code"]), "CUST-RCVBLE-") || S(Get(settle, "zatca.status")) != "not_reported" {
			t.Errorf("deposit: %v", settle)
		}
		Eventually(t, "sale paid in full", func() bool {
			g := Read(t, s.Token, "sales", S(sale["id"]))
			return S(Get(g, "legacyTotals.paymentStatus")) == "paid" && Cents(F(g, "legacyTotals.balance")) == 0 &&
				Cents(F(g, "legacyTotals.paid")) == 11500
		})
		Eventually(t, "customer settled", func() bool { return Cents(balance()) == 0 })
	})

	t.Run("advance and refund move the customer balance", func(t *testing.T) {
		advance = Create(t, s.Token, "deposits", note(M{"amount": 40, "method": "bank_transfer"}))
		Eventually(t, "customer in credit 40", func() bool { return Cents(balance()) == -4000 })
		refund = Create(t, s.Token, "withdrawals", note(M{"amount": 25}))
		if S(refund["type"]) != "refund" || !strings.HasPrefix(S(refund["code"]), "CUST-PAYBLE-") ||
			S(Get(refund, "zatca.invoiceType")) != "credit-simplified" {
			t.Errorf("withdrawal: %v", refund)
		}
		Eventually(t, "customer in credit 15", func() bool { return Cents(balance()) == -1500 })
		p := Patch(t, s.Token, "deposits", S(advance["id"]), M{"amount": 60})
		EqMoney(t, "patched deposit", F(p, "amount"), 60)
		Eventually(t, "customer in credit 35", func() bool { return Cents(balance()) == -3500 })
		// a stale version
		if r := Call(t, "PATCH", "/deposits/"+S(advance["id"]), s.Token, M{"amount": 1}, "If-Match", "1"); r.Code != 409 {
			t.Errorf("stale deposit patch: %s", r)
		}
		if r := PatchResp(t, s.Token, "withdrawals", S(refund["id"]), M{"method": "gold"}); r.Code != 400 || r.ErrField("method") == "" {
			t.Errorf("patch bad method: %s", r)
		}
		cur := Read(t, s.Token, "withdrawals", S(refund["id"]))
		put := Call(t, "PUT", "/withdrawals/"+S(refund["id"]), s.Token, note(M{"amount": 30, "notes": "PUT"}), "If-Match", Num(cur["version"]).String())
		if put.Code != 200 || Cents(F(put.Body, "amount")) != 3000 {
			t.Errorf("PUT withdrawal: %s", put)
		}
		Eventually(t, "customer in credit 30", func() bool { return Cents(balance()) == -3000 })
		Must(t, Call(t, "PUT", "/deposits/"+S(advance["id"]), s.Token, M{"storeId": s.ID}), 400, "PUT deposit without fields")
		Must(t, Call(t, "PUT", "/withdrawals/"+S(refund["id"]), s.Token, note(M{"amount": -1})), 400, "PUT withdrawal negative")
		cur = Read(t, s.Token, "deposits", S(advance["id"]))
		put = Call(t, "PUT", "/deposits/"+S(advance["id"]), s.Token, note(M{"amount": 60, "method": "bank_transfer", "notes": "PUT"}), "If-Match", Num(cur["version"]).String())
		if put.Code != 200 || S(put.Body["notes"]) != "PUT" || Cents(F(put.Body, "amount")) != 6000 {
			t.Errorf("PUT deposit: %s", put)
		}
		if w := Patch(t, s.Token, "withdrawals", S(refund["id"]), M{"notes": "استرداد"}); S(w["notes"]) != "استرداد" {
			t.Errorf("patched withdrawal: %v", w)
		}
		Eventually(t, "customer still in credit 30", func() bool { return Cents(balance()) == -3000 })
	})

	t.Run("stats, lists, other company", func(t *testing.T) {
		st := finStats(t, s, "deposits", "sum=amount,one")
		if F(st, "count") != 2 {
			t.Errorf("deposits count %v", st["count"])
		}
		EqMoney(t, "deposits sum", F(st, "sums.amount"), 160)
		st = finStats(t, s, "withdrawals", "sum=amount&"+finPeriod(s.Today(), s.Today()))
		EqMoney(t, "withdrawals sum", F(st, "sums.amount"), 30)
		if st := finStats(t, s, "deposits", "sum=amount&"+finPeriod("2020-01-01", "2020-12-31")); F(st, "count") != 0 {
			t.Errorf("empty period: %v", st)
		}
		Must(t, Call(t, "GET", "/deposits/stats?"+finQS(s, "page=0"), s.Token, nil), 400, "stats page 0")
		Must(t, Call(t, "GET", "/withdrawals/stats?storeId="+s.ID, other.Token, nil), 403, "stats of a store the user cannot use")
		Must(t, Call(t, "GET", "/deposits/"+S(advance["id"]), other.Token, nil), 404, "deposit of another company")
		Must(t, Call(t, "GET", "/withdrawals/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "unknown withdrawal")
		finListChecks(t, s, other, "deposits", "withdrawals")
		rows := List(t, s.Token, "deposits", finQS(s, ""))
		if len(rows) != 2 {
			t.Errorf("deposits listed: %d", len(rows))
		}
	})

	t.Run("delete", func(t *testing.T) {
		Must(t, Call(t, "DELETE", "/withdrawals/"+S(refund["id"]), s.Token, nil), 200, "delete withdrawal")
		if r := Call(t, "POST", "/withdrawals/"+S(refund["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore withdrawal: %s", r)
		}
		Must(t, Call(t, "DELETE", "/deposits/"+S(advance["id"]), other.Token, nil), 404, "delete by another company")
		Must(t, Call(t, "DELETE", "/withdrawals/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "delete unknown withdrawal")
		Must(t, Call(t, "DELETE", "/deposits/"+S(advance["id"]), s.Token, nil), 200, "delete deposit")
		if r := Call(t, "POST", "/deposits/"+S(advance["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore deposit: %s", r)
		}
		st := finStats(t, s, "deposits", "sum=amount")
		EqMoney(t, "deposits after delete", F(st, "sums.amount"), 100)
		if st := finStats(t, s, "withdrawals", "sum=amount"); F(st, "count") != 0 {
			t.Errorf("deleted withdrawal still counted: %v", st)
		}
		// both notes gone: the customer is settled (the paid invoice only)
		time.Sleep(2 * time.Second)
		b := balance()
		KnownBug(t, "NEW-fin-delete-ledger", fmt.Sprintf("DELETE /deposits|/withdrawals leaves the ledger postings: customer balance %.2f after deleting both notes, want 0", b),
			Cents(b) != 0)
	})
}

// ---------- capital, capital withdrawals, dividends and the ledger ----------

func TestFinanceCapitalAndLedger(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	day := s.Now()
	money := func(personKey, person string, amount float64, method string) M {
		return M{"storeId": s.ID, "date": day, personKey: person, "amount": amount, "method": method}
	}
	// an investor must be a user of the company
	stranger := Create(t, other.Token, "users", M{"name": "Stranger " + Uniq(), "email": "str" + Uniq() + Digits(4) + "@e2e.example",
		"phone": "05" + Digits(8), "role": "r_viewer", "storeIds": []string{other.ID}, "password": "Staff@" + Digits(6)})

	t.Run("validation", func(t *testing.T) {
		for _, c := range []struct{ path, person string }{{"capitals", "investor"}, {"capital-withdrawals", "investor"}, {"dividends", "recipient"}} {
			r := Call(t, "POST", "/"+c.path, s.Token, M{"storeId": s.ID})
			for _, f := range []string{c.person, "date", "amount", "method"} {
				if r.Code != 400 || r.ErrField(f) == "" {
					t.Errorf("%s empty body: error.fields.%s missing: %s", c.path, f, r)
				}
			}
			for _, b := range []M{money(c.person, finOwnerName, -1, "cash"), money(c.person, finOwnerName, 0, "cash")} {
				if r := Call(t, "POST", "/"+c.path, s.Token, b); r.Code != 400 || r.ErrField("amount") == "" {
					t.Errorf("%s amount %v: %s", c.path, b["amount"], r)
				}
			}
			if r := Call(t, "POST", "/"+c.path, s.Token, money(c.person, finOwnerName, 5, "barter")); r.Code != 400 || r.ErrField("method") == "" {
				t.Errorf("%s bad method: %s", c.path, r)
			}
			for _, who := range []string{"Nobody " + Uniq(), S(stranger["name"])} {
				if r := Call(t, "POST", "/"+c.path, s.Token, money(c.person, who, 5, "cash")); r.Code != 400 || r.ErrField(c.person) == "" {
					t.Errorf("%s by %q (not a user of the company): %s", c.path, who, r)
				}
			}
		}
	})

	var capCash, capBank, cw, div M
	t.Run("create", func(t *testing.T) {
		capCash = Create(t, s.Token, "capitals", money("investor", finOwnerName, 5000, "cash"))
		// the name matches case-insensitively
		capBank = Create(t, s.Token, "capitals", money("investor", strings.ToUpper(finOwnerName), 2000, "bank_transfer"))
		if S(capCash["investorUserId"]) == "" || S(capCash["investorUserId"]) != S(capBank["investorUserId"]) || S(capCash["notes"]) != "capitals" {
			t.Errorf("capitals: %v / %v", capCash, capBank)
		}
		div = Create(t, s.Token, "dividends", money("recipient", finOwnerName, 400, "cash"))
		if S(div["recipientUserId"]) != S(capCash["investorUserId"]) || !strings.HasPrefix(S(div["code"]), "CAP-DRWNG-") {
			t.Errorf("dividend: %v", div)
		}
		cw = Create(t, s.Token, "capital-withdrawals", money("investor", finOwnerName, 100, "cash"))
		// numbers: capital withdrawals have their own series (store serial capitalWithdrawal "CWD-")
		KnownBug(t, "NEW-fin-capwd-number", fmt.Sprintf("capital withdrawal numbered %v in the capital series (capitals %v, %v): numbers repeat across both",
			cw["code"], capCash["code"], capBank["code"]), strings.HasPrefix(S(cw["code"]), "CAP-DPST-"))
		cap3 := Create(t, s.Token, "capitals", money("investor", finOwnerName, 1, "cash"))
		KnownBug(t, "NEW-fin-capwd-number", fmt.Sprintf("capital %v and capital withdrawal %v share one number", cap3["code"], cw["code"]),
			S(cap3["code"]) == S(cw["code"]))
		Must(t, Call(t, "DELETE", "/capitals/"+S(cap3["id"])+"?hard=1", s.Token, nil), 204, "hard delete capital")
		// by user id
		staffID, _ := s.User(t, "r_accountant")
		c := Create(t, s.Token, "capitals", M{"storeId": s.ID, "date": day, "investorUserId": staffID, "amount": 1, "method": "cash"})
		if S(c["investorUserId"]) != staffID || S(c["investor"]) != "E2E r_accountant" {
			t.Errorf("capital by user id: %v", c)
		}
		Must(t, Call(t, "DELETE", "/capitals/"+S(c["id"])+"?hard=1", s.Token, nil), 204, "hard delete capital")
	})

	t.Run("update and delete", func(t *testing.T) {
		p := Patch(t, s.Token, "capitals", S(capCash["id"]), M{"notes": "رأس المال"})
		if S(p["notes"]) != "رأس المال" {
			t.Errorf("patched capital: %v", p)
		}
		p = Patch(t, s.Token, "dividends", S(div["id"]), M{"notes": "Q3"})
		if S(p["notes"]) != "Q3" {
			t.Errorf("patched dividend: %v", p)
		}
		cur := Read(t, s.Token, "dividends", S(div["id"]))
		put := Call(t, "PUT", "/dividends/"+S(div["id"]), s.Token, money("recipient", finOwnerName, 400, "cash"), "If-Match", Num(cur["version"]).String())
		if put.Code != 200 {
			t.Errorf("PUT dividend: %s", put)
		}
		if r := PatchResp(t, s.Token, "dividends", S(div["id"]), M{"method": "gold"}); r.Code != 400 || r.ErrField("method") == "" {
			t.Errorf("patch dividend bad method: %s", r)
		}
		if r := PatchResp(t, s.Token, "capitals", S(capCash["id"]), M{"amount": 0}); r.Code != 400 || r.ErrField("amount") == "" {
			t.Errorf("patch capital amount 0: %s", r)
		}
		// the existing system cannot edit capital withdrawals
		if r := PatchResp(t, s.Token, "capital-withdrawals", S(cw["id"]), M{"notes": "x"}); r.Code != 409 || r.ErrCode() != "unsupported_legacy" {
			t.Errorf("patch capital withdrawal: %s", r)
		}
		if r := Call(t, "PUT", "/capital-withdrawals/"+S(cw["id"]), s.Token, money("investor", finOwnerName, 100, "cash")); r.Code != 409 {
			t.Errorf("PUT capital withdrawal: %s", r)
		}
		Must(t, Call(t, "GET", "/capitals/"+S(capCash["id"]), other.Token, nil), 404, "capital of another company")
		Must(t, Call(t, "DELETE", "/dividends/"+S(div["id"]), other.Token, nil), 404, "delete by another company")
		tmp := Create(t, s.Token, "dividends", money("recipient", finOwnerName, 7, "cash"))
		Must(t, Call(t, "DELETE", "/dividends/"+S(tmp["id"]), s.Token, nil), 200, "delete dividend")
		if r := Call(t, "POST", "/dividends/"+S(tmp["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore dividend: %s", r)
		}
		tmp = Create(t, s.Token, "capital-withdrawals", money("investor", finOwnerName, 3, "cash"))
		Must(t, Call(t, "DELETE", "/capital-withdrawals/"+S(tmp["id"]), s.Token, nil), 200, "delete capital withdrawal")
		if r := Call(t, "POST", "/capital-withdrawals/"+S(tmp["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore capital withdrawal: %s", r)
		}
		tmp = Create(t, s.Token, "capitals", money("investor", finOwnerName, 9, "cash"))
		Must(t, Call(t, "DELETE", "/capitals/"+S(tmp["id"]), s.Token, nil), 200, "delete capital")
		if r := Call(t, "POST", "/capitals/"+S(tmp["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore capital: %s", r)
		}
	})

	t.Run("lists", func(t *testing.T) {
		finListChecks(t, s, other, "capitals", "capital-withdrawals", "dividends", "accounts")
		cur := Read(t, s.Token, "capitals", S(capBank["id"]))
		put := Call(t, "PUT", "/capitals/"+S(capBank["id"]), s.Token, money("investor", finOwnerName, 2000, "bank_transfer"), "If-Match", Num(cur["version"]).String())
		if put.Code != 200 || S(put.Body["method"]) != "bank_transfer" {
			t.Errorf("PUT capital: %s", put)
		}
		Must(t, Call(t, "PUT", "/capitals/"+S(capBank["id"]), s.Token, money("investor", finOwnerName, -1, "cash")), 400, "PUT capital negative")
		Must(t, Call(t, "PUT", "/dividends/"+S(div["id"]), s.Token, money("recipient", "", 1, "cash")), 400, "PUT dividend without recipient")
		Must(t, Call(t, "DELETE", "/capitals/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "delete unknown capital")
		Must(t, Call(t, "DELETE", "/capital-withdrawals/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "delete unknown capital withdrawal")
		Read(t, s.Token, "capital-withdrawals", S(cw["id"]))
		Must(t, Call(t, "GET", "/capital-withdrawals/"+S(cw["id"]), other.Token, nil), 404, "capital withdrawal of another company")
		Must(t, Call(t, "GET", "/dividends/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "unknown dividend")
	})

	t.Run("stats", func(t *testing.T) {
		for _, c := range []struct {
			path string
			want float64
			n    int
		}{{"capitals", 7000, 2}, {"capital-withdrawals", 100, 1}, {"dividends", 400, 1}} {
			st := finStats(t, s, c.path, "sum=amount&"+finPeriod(s.Today(), s.Today()))
			if int(F(st, "count")) != c.n {
				t.Errorf("%s count %v want %d", c.path, st["count"], c.n)
			}
			EqMoney(t, c.path+" sum", F(st, "sums.amount"), c.want)
			if r := Call(t, "GET", "/"+c.path+"/stats?"+finQS(s, "from=2026-13-40"), s.Token, nil); r.Code != 400 {
				t.Errorf("%s stats bad from: %s", c.path, r)
			}
		}
	})

	t.Run("accounts are read-only", func(t *testing.T) {
		rows := List(t, s.Token, "accounts", finQS(s, ""))
		if len(rows) == 0 {
			t.Fatal("no accounts")
		}
		id := S(rows[0]["id"])
		g := Read(t, s.Token, "accounts", id)
		if S(g["id"]) != id || g["code"] == nil {
			t.Errorf("account: %v", g)
		}
		for _, c := range []struct{ m, path string }{{"POST", "/accounts"}, {"PATCH", "/accounts/" + id}, {"PUT", "/accounts/" + id},
			{"DELETE", "/accounts/" + id}, {"POST", "/accounts/" + id + "/restore"}} {
			r := Call(t, c.m, c.path, s.Token, M{"storeId": s.ID, "nameEn": "Hack"}, "If-Match", Num(g["version"]).String())
			if r.Code != 403 || r.ErrCode() != "forbidden" {
				t.Errorf("%s %s: want 403, got %s", c.m, c.path, r)
			}
		}
		Must(t, Call(t, "GET", "/accounts/"+id, other.Token, nil), 404, "account of another company")
		Must(t, Call(t, "GET", "/accounts/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "unknown account")
		for _, a := range List(t, other.Token, "accounts", "") {
			if S(a["id"]) == id {
				t.Error("account listed to another company")
			}
		}
		if hit := List(t, s.Token, "accounts", finQS(s, "q=CASH")); len(hit) == 0 || !strings.Contains(S(hit[0]["nameEn"]), "CASH") {
			t.Errorf("search accounts: %v", hit)
		}
	})

	t.Run("balances and trial balance", func(t *testing.T) {
		cat := finExpenseCat(t, s, "Utilities "+Uniq())
		vendor := s.Vendor(t)
		Create(t, s.Token, "expenses", M{"storeId": s.ID, "date": day, "categoryId": cat, "description": "Power", "amount": 100,
			"vatAmount": 15, "vendorId": vendor["id"], "reference": "EB-1", "method": "cash"})
		Create(t, s.Token, "expenses", M{"storeId": s.ID, "date": day, "categoryId": cat, "description": "Water", "amount": 50, "method": "bank_transfer"})
		cust := s.Customer(t, "")
		Create(t, s.Token, "deposits", M{"storeId": s.ID, "date": day, "customerId": cust["id"], "amount": 300, "method": "cash"})
		Create(t, s.Token, "withdrawals", M{"storeId": s.ID, "date": day, "customerId": cust["id"], "amount": 120, "method": "bank_transfer"})
		// cash:   5000 capital − 115 expense + 300 deposit − 400 dividend (− 100 capital withdrawal)
		// bank:   2000 capital − 50 expense − 120 refund
		wantCash, wantBank := 5000.0-115+300-400, 2000.0-50-120
		var acc map[string]M
		Eventually(t, "ledger postings", func() bool {
			acc = finAccounts(t, s)
			c := finAcctBy(acc, "referenceId", cat)
			cash := Cents(finSigned(acc["CASH"]))
			cu := finAcctBy(acc, "referenceId", S(cust["id"]))
			return c != nil && cu != nil && Cents(finSigned(cu)) == -18000 && Cents(finSigned(c)) == 16500 && acc["BANK"] != nil && Cents(finSigned(acc["BANK"])) == Cents(wantBank) &&
				(cash == Cents(wantCash-100) || cash == Cents(wantCash+1+9+1-7))
		})
		// deleted capitals (1, 9 and 1 by staff) and dividend (7) still move CASH,
		// the capital withdrawal (100) never does
		cash := finSigned(acc["CASH"])
		KnownBug(t, "NEW-fin-capwd-ledger", fmt.Sprintf("capital withdrawals are never posted and deleted capital/dividends keep their postings: CASH %.2f, want %.2f", cash, wantCash-100),
			Cents(cash) == Cents(wantCash+1+9+1-7))
		// one capital account per investor: the owner's holds the 7000 (staff
		// capitals were 1 and 1)
		var capAcc M
		for _, a := range acc {
			if S(a["referenceModel"]) == "investor" && (capAcc == nil || -finSigned(a) > -finSigned(capAcc)) {
				capAcc = a
			}
		}
		if capAcc == nil || S(capAcc["type"]) != "capital" {
			t.Fatalf("no capital account: %v", acc)
		}
		KnownBug(t, "NEW-fin-delete-ledger", fmt.Sprintf("DELETE /capitals (soft and ?hard=1) keeps the ledger posting: capital account %.2f, want credit 7000", -finSigned(capAcc)),
			Cents(finSigned(capAcc)) == -701000)
		if a := finAcctBy(acc, "referenceModel", "withdrawer"); a == nil || Cents(finSigned(a)) < 40000 || S(a["type"]) != "drawing" {
			t.Errorf("drawings account: %v (want debit 400)", a)
		}
		if a := finAcctBy(acc, "referenceId", S(cust["id"])); a == nil || Cents(finSigned(a)) != -18000 {
			t.Errorf("customer account: %v (want credit 180)", a)
		}
		// trial balance: debits = credits (once every posting has landed)
		var dr, cr float64
		balanced := func() bool {
			dr, cr = 0, 0
			for _, a := range finAccounts(t, s) {
				switch S(a["debitOrCredit"]) {
				case "debit_balance":
					dr += F(a, "balance")
				case "credit_balance":
					cr += F(a, "balance")
				}
			}
			return Cents(dr) == Cents(cr) && dr != 0
		}
		deadline := time.Now().Add(15 * time.Second)
		for !balanced() && time.Now().Before(deadline) {
			time.Sleep(300 * time.Millisecond)
		}
		if Cents(dr) != Cents(cr) || dr == 0 {
			t.Errorf("trial balance: debits %.2f, credits %.2f", dr, cr)
		}
	})
}

// ---------- employees and salaries ----------

func TestFinanceSalariesEmployees(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	now := time.Now().In(s.Loc)
	m0 := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, s.Loc)
	join := m0.AddDate(0, -2, 14).Format("2006-01-02") // the 15th, two months ago
	per := func(n int) string { return m0.AddDate(0, n, 0).Format("2006-01") }
	const basic = 1000.0

	var emp, emp2 M
	t.Run("employees", func(t *testing.T) {
		cases := []struct {
			body  M
			field string
		}{
			{M{"storeId": s.ID, "nameEn": " ", "joinDate": join}, "nameEn"},
			{M{"storeId": s.ID, "nameEn": "No Join"}, "joinDate"},
			{M{"storeId": s.ID, "nameEn": "Bad Join", "joinDate": "15/01/2026"}, "joinDate"},
			{M{"storeId": s.ID, "nameEn": "Bad Iqama", "joinDate": join, "nationalId": "3123456789"}, "nationalId"},
			{M{"storeId": s.ID, "nameEn": "Short Iqama", "joinDate": join, "nationalId": "12345"}, "nationalId"},
			{M{"storeId": s.ID, "nameEn": "Negative", "joinDate": join, "basicSalary": -1}, "basicSalary"},
		}
		for _, c := range cases {
			if r := Call(t, "POST", "/employees", s.Token, c.body); r.Code != 400 || r.ErrField(c.field) == "" {
				t.Errorf("employee %v: want 400 on %s, got %s", c.body["nameEn"], c.field, r)
			}
		}
		emp = Create(t, s.Token, "employees", M{"storeId": s.ID, "nameEn": "Ahmed Salem", "nameAr": "أحمد سالم", "joinDate": join,
			"basicSalary": basic, "status": "active", "nationalId": "2123456789", "jobTitle": "Cashier", "phone": "05" + Digits(8)})
		if S(emp["joinDate"]) != join || F(emp, "basicSalary") != basic || S(emp["status"]) != "active" || F(emp, "salaryDay") != 1 {
			t.Errorf("employee: %v", emp)
		}
		emp2 = finEmployee(t, s, "Zaid Inactive", 2500, join)
		p := Patch(t, s.Token, "employees", S(emp2["id"]), M{"status": "inactive", "jobTitle": "Driver"})
		if S(p["status"]) != "inactive" || S(p["jobTitle"]) != "Driver" {
			t.Errorf("patched employee: %v", p)
		}
		if r := PatchResp(t, s.Token, "employees", S(emp2["id"]), M{"basicSalary": -5}); r.Code != 400 || r.ErrField("basicSalary") == "" {
			t.Errorf("patch negative salary: %s", r)
		}
		act := List(t, s.Token, "employees", finQS(s, "where.status=active"))
		ina := List(t, s.Token, "employees", finQS(s, "where.status=inactive"))
		if len(act) != 1 || S(act[0]["id"]) != S(emp["id"]) || len(ina) != 1 || S(ina[0]["id"]) != S(emp2["id"]) {
			t.Errorf("status filter: %d active, %d inactive", len(act), len(ina))
		}
		srt := List(t, s.Token, "employees", finQS(s, "sort=-basicSalary"))
		if len(srt) != 2 || S(srt[0]["id"]) != S(emp2["id"]) {
			t.Errorf("sort by salary desc: %v", srt)
		}
		if rng := List(t, s.Token, "employees", finQS(s, "min.basicSalary=1500")); len(rng) != 1 || S(rng[0]["id"]) != S(emp2["id"]) {
			t.Errorf("min.basicSalary: %v", rng)
		}
		if hit := List(t, s.Token, "employees", finQS(s, "q="+url.QueryEscape("أحمد"))); len(hit) != 1 || S(hit[0]["id"]) != S(emp["id"]) {
			t.Errorf("Arabic search: %v", hit)
		}
		if r := Call(t, "GET", "/employees?"+finQS(s, "sort=nationalId"), s.Token, nil); r.Code != 400 || r.ErrField("sort") == "" {
			t.Errorf("unsupported sort: %s", r)
		}
		if r := Call(t, "GET", "/employees?"+finQS(s, "where.status=sleeping"), s.Token, nil); r.Code != 400 {
			t.Errorf("bad status filter: %s", r)
		}
		Must(t, Call(t, "GET", "/employees/"+S(emp["id"]), other.Token, nil), 404, "employee of another company")
		full := M{"storeId": s.ID, "nameEn": "Zaid Put", "joinDate": join, "basicSalary": 2500, "status": "inactive"}
		cur := Read(t, s.Token, "employees", S(emp2["id"]))
		put := Call(t, "PUT", "/employees/"+S(emp2["id"]), s.Token, full, "If-Match", Num(cur["version"]).String())
		KnownBug(t, "NEW-fin-employee-put", "PUT /employees without salaryDay answers 400 salaryDay (POST defaults it to 1)",
			put.Code == 400 && put.ErrField("salaryDay") != "")
		if put.Code != 200 {
			full["salaryDay"] = 1
			put = Call(t, "PUT", "/employees/"+S(emp2["id"]), s.Token, full, "If-Match", Num(cur["version"]).String())
		}
		if put.Code != 200 || S(put.Body["nameEn"]) != "Zaid Put" {
			t.Errorf("PUT employee: %s", put)
		}
		if r := Call(t, "PATCH", "/employees/"+S(emp2["id"]), s.Token, M{"nameEn": "x"}, "If-Match", "1"); r.Code != 409 {
			t.Errorf("stale employee patch: %s", r)
		}
	})

	// Ahmed's salary balance (the other employee accrues too, inactive or not)
	balance := func() M {
		b := finDash(t, s, "salary-balance", "")
		var sum float64
		for _, e := range Objs(b["employees"]) {
			sum += F(e, "balance")
			if strings.Contains(S(e["name"]), "AHMED SALEM") {
				b["ahmed"] = -F(e, "balance")
			}
		}
		EqMoney(t, "salary balance = − Σ employees", F(b, "balance"), -sum)
		return b
	}
	accrued := 3 * basic // months m-2 (from the 15th, accrued in full), m-1 and m (pay day 1)
	var sal1, sal2 M
	t.Run("salaries", func(t *testing.T) {
		eid := S(emp["id"])
		body := func(period string, net float64, extra M) M {
			b := M{"storeId": s.ID, "employeeId": eid, "period": period, "netSalary": net, "paymentDate": s.Today(), "method": "cash"}
			for k, v := range extra {
				b[k] = v
			}
			return b
		}
		for _, c := range []struct {
			body  M
			field string
		}{
			{body("2026/01", 1, nil), "period"},
			{body(per(-1), -1, nil), "netSalary"},
			{body(per(-1), 1, M{"housing": -1}), "housing"},
			{body(per(-1), 1, M{"deductions": -2}), "deductions"},
			{body(per(-1), 1, M{"employeeId": ""}), "employeeId"},
			{body(per(-1), 1, M{"employeeId": "nope"}), "employeeId"},
			{body(per(-1), 1, M{"paymentDate": "yesterday"}), "paymentDate"},
		} {
			if r := Call(t, "POST", "/salaries", s.Token, c.body); r.Code != 400 || r.ErrField(c.field) == "" {
				t.Errorf("salary %v: want 400 on %s, got %s", c.field, c.field, r)
			}
		}
		otherEmp := finEmployee(t, other, "Other Co Emp", 900, join)
		if r := Call(t, "POST", "/salaries", s.Token, body(per(-1), 1, M{"employeeId": otherEmp["id"]})); r.Code == 201 {
			t.Errorf("salary for another company's employee: %s", r)
		}

		sal1 = Create(t, s.Token, "salaries", body(per(-1), basic, M{"notes": "راتب"}))
		if S(sal1["period"]) != per(-1) || S(sal1["employeeName"]) != "Ahmed Salem" || S(sal1["employeeNameAr"]) != "أحمد سالم" ||
			F(sal1, "basicSalary") != basic || S(sal1["status"]) != "paid" || !strings.HasPrefix(S(sal1["code"]), "SAL-") {
			t.Errorf("salary: %v", sal1)
		}
		if r := Call(t, "POST", "/salaries", s.Token, body(per(-1), 5, nil)); r.Code != 400 || !strings.Contains(r.ErrField("period"), "already exists") {
			t.Errorf("second salary for the period: %s", r)
		}
		b := balance()
		if Cents(F(b, "ahmed")) != Cents(-(accrued-basic)) || S(b["status"]) != "owed_to_employees" || len(Objs(b["employees"])) != 2 {
			t.Errorf("salary balance %v (%v), want %.2f owed to Ahmed", b["employees"], b["status"], accrued-basic)
		}

		sal2 = Create(t, s.Token, "salaries", body(per(0), 500, nil))
		p := Patch(t, s.Token, "salaries", S(sal2["id"]), M{"netSalary": 800})
		EqMoney(t, "patched salary", F(p, "netSalary"), 800)
		if r := PatchResp(t, s.Token, "salaries", S(sal2["id"]), M{"period": per(-1)}); r.Code != 400 || r.ErrField("period") == "" {
			t.Errorf("patch salary onto a paid period: %s", r)
		}
		Eventually(t, "balance after second salary", func() bool { return Cents(F(balance(), "ahmed")) == Cents(-(accrued - basic - 800)) })
		cur := Read(t, s.Token, "salaries", S(sal2["id"]))
		put := Call(t, "PUT", "/salaries/"+S(sal2["id"]), s.Token, body(per(0), 750, nil), "If-Match", Num(cur["version"]).String())
		if put.Code != 200 || Cents(F(put.Body, "netSalary")) != 75000 {
			t.Errorf("PUT salary: %s", put)
		}
		Must(t, Call(t, "GET", "/salaries/"+S(sal1["id"]), other.Token, nil), 404, "salary of another company")
		finListChecks(t, s, other, "salaries", "employees")
		if rows := List(t, s.Token, "salaries", finQS(s, "")); len(rows) != 2 {
			t.Errorf("salaries listed: %d", len(rows))
		}
		Must(t, Call(t, "PUT", "/salaries/"+S(sal1["id"]), s.Token, body("2026-1", 1, nil)), 400, "PUT salary bad period")
		Must(t, Call(t, "DELETE", "/salaries/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "delete unknown salary")
		Must(t, Call(t, "DELETE", "/employees/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "delete unknown employee")
		Must(t, Call(t, "PUT", "/employees/"+S(emp["id"]), s.Token, M{"storeId": s.ID, "nameEn": ""}), 400, "PUT employee without name")

		st := finStats(t, s, "salaries", "sum=netSalary&"+finPeriod(s.Today(), s.Today()))
		if F(st, "count") != 2 {
			t.Errorf("salaries count %v", st["count"])
		}
		EqMoney(t, "salaries paid today", F(st, "sums.netSalary"), basic+750)
		if st := finStats(t, s, "salaries", "sum=netSalary&"+finPeriod(finDay(s, 1), "")); F(st, "count") != 0 {
			t.Errorf("salaries from tomorrow: %v", st)
		}
		if r := Call(t, "GET", "/salaries/stats?"+finQS(s, "sum="+strings.Repeat("a,", 21)), s.Token, nil); r.Code != 400 || r.ErrField("sum") == "" {
			t.Errorf("too many sums: %s", r)
		}

		// ledger: salary expense accrued, the employee's account owes the rest
		var acc map[string]M
		Eventually(t, "salary ledger", func() bool {
			acc = finAccounts(t, s)
			a := finAcctBy(acc, "referenceId", S(emp["id"]))
			return a != nil && Cents(finSigned(a)) == Cents(-(accrued-basic-750))
		})
		if a := acc["SALARY EXPENSE"]; a == nil || Cents(finSigned(a)) < Cents(accrued) {
			t.Errorf("salary expense account: %v", a)
		}

		// delete: the period can be paid again
		Must(t, Call(t, "DELETE", "/salaries/"+S(sal2["id"]), s.Token, nil), 200, "delete salary")
		Eventually(t, "balance after delete", func() bool { return Cents(F(balance(), "ahmed")) == Cents(-(accrued - basic)) })
		if r := Call(t, "POST", "/salaries/"+S(sal2["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore salary: %s", r)
		}
		again := Create(t, s.Token, "salaries", body(per(0), 100, nil))
		Must(t, Call(t, "DELETE", "/salaries/"+S(again["id"])+"?hard=1", s.Token, nil), 204, "hard delete salary")
		Must(t, Call(t, "GET", "/salaries/"+S(again["id"]), s.Token, nil), 404, "after hard delete")
		Must(t, Call(t, "DELETE", "/employees/"+S(emp2["id"]), s.Token, nil), 200, "delete employee")
		if r := Call(t, "POST", "/employees/"+S(emp2["id"])+"/restore", s.Token, M{}); r.Code != 409 {
			t.Errorf("restore employee: %s", r)
		}
	})

	t.Run("dashboard errors", func(t *testing.T) {
		if r := Call(t, "GET", "/dashboard/salary-balance", s.Token, nil); r.Code != 400 || r.ErrField("storeId") == "" {
			t.Errorf("no storeId: %s", r)
		}
		Must(t, Call(t, "GET", "/dashboard/salary-balance?storeId="+s.ID, other.Token, nil), 404, "another company's store")
		Must(t, Call(t, "GET", "/dashboard/salary-balance?storeId="+s.ID, "", nil), 401, "no token")
	})
}

// ---------- dashboards ----------

// finSale is a document's oracle: subtotal, VAT at the store rate and net.
type finDoc struct{ sub, vat, net float64 }

func finTotals(sub, rate float64) finDoc {
	v := finR2(sub * rate / 100)
	return finDoc{sub, v, finR2(sub + v)}
}

func TestDashboardFigures(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	finSetFlags(t, s, M{"enable_employee_module": true})
	d1, d2 := finDay(s, -20), finDay(s, -19)
	p := s.Product(t, 8, 50, 100)
	pid := S(p["id"])
	c := s.Customer(t, "")
	v := s.Vendor(t)

	// the empty store first: every figure 0
	for _, k := range []string{"revenue", "vat", "total-expense", "net-profit"} {
		b := finDash(t, s, k, "")
		for _, f := range []string{"result.total", "result.revenue", "result.vatPayable", "result.profit"} {
			if x := Get(b, f); x != nil && F(b, f) != 0 {
				t.Errorf("empty store %s %s = %v", k, f, x)
			}
		}
	}

	sale := func(when string, qty, price float64, paid float64) M {
		b := M{"storeId": s.ID, "date": when, "customerId": c["id"], "items": []M{s.Line(pid, qty, price)}}
		if paid > 0 {
			b["payments"] = []M{{"date": when, "amount": paid, "method": "cash"}}
		}
		return Create(t, s.Token, "sales", b)
	}
	sA, sB, sC := finTotals(100, 15), finTotals(40, 15), finTotals(30, 15)
	saleA := sale(d1+"T10:00", 2, 50, sA.net)
	sale(d1+"T23:50", 1, 40, 0) // 23:50 and 00:10 store time: both on d1 in UTC
	sale(d2+"T00:10", 3, 10, 0)
	ret := finTotals(50, 15)
	Create(t, s.Token, "sales-returns", M{"storeId": s.ID, "date": d2 + "T12:00", "orderId": saleA["id"], "customerId": c["id"],
		"items": []M{s.Line(pid, 1, 50)}, "payments": []M{{"date": d2 + "T12:00", "amount": ret.net, "method": "cash"}}})
	pur, pret := finTotals(80, 15), finTotals(16, 15)
	purchase := Create(t, s.Token, "purchases", M{"storeId": s.ID, "date": d1 + "T09:00", "vendorId": v["id"], "vendorInvoiceNo": "VI-" + Uniq(),
		"items": []M{s.Line(pid, 10, 8)}})
	Create(t, s.Token, "purchase-returns", M{"storeId": s.ID, "date": d2 + "T13:00", "purchaseId": purchase["id"], "vendorId": v["id"],
		"items": []M{s.Line(pid, 2, 8)}})
	cat := finExpenseCat(t, s, "Rent "+Uniq())
	e1 := Create(t, s.Token, "expenses", M{"storeId": s.ID, "date": d1 + "T11:00", "categoryId": cat, "description": "Rent", "amount": 200,
		"vatAmount": 30, "vendorId": v["id"], "reference": "R-1", "method": "cash"})
	e2 := Create(t, s.Token, "expenses", M{"storeId": s.ID, "date": d2 + "T08:00", "categoryId": cat, "description": "Water", "amount": 50, "method": "cash"})
	e1Gross, e2Gross := 230.0, 50.0
	emp := finEmployee(t, s, "Dash Employee", 1000, "2025-01-15")
	Create(t, s.Token, "salaries", M{"storeId": s.ID, "employeeId": emp["id"], "period": d2[:7], "netSalary": 1000, "paymentDate": d2, "method": "cash"})
	Create(t, s.Token, "deposits", M{"storeId": s.ID, "date": d1 + "T15:00", "customerId": c["id"], "amount": 20, "method": "cash"})
	Create(t, s.Token, "withdrawals", M{"storeId": s.ID, "date": d2 + "T15:00", "customerId": c["id"], "amount": 5, "method": "cash"})

	type want struct{ sales, sret, salesVat, sretVat, pur, pret, purVat, pretVat, exp, salary float64 }
	periods := []struct {
		name, from, to string
		w              want
	}{
		{"day before midnight", d1, d1, want{sA.net + sB.net, 0, sA.vat + sB.vat, 0, pur.net, 0, pur.vat, 0, e1Gross, 0}},
		{"day after midnight", d2, d2, want{sC.net, ret.net, sC.vat, ret.vat, 0, pret.net, 0, pret.vat, e2Gross, 1000}},
		{"both days", d1, d2, want{sA.net + sB.net + sC.net, ret.net, sA.vat + sB.vat + sC.vat, ret.vat, pur.net, pret.net, pur.vat, pret.vat, e1Gross + e2Gross, 1000}},
		{"from only", d2, "", want{sC.net, ret.net, sC.vat, ret.vat, 0, pret.net, 0, pret.vat, e2Gross, 1000}},
		{"to only", "", d1, want{sA.net + sB.net, 0, sA.vat + sB.vat, 0, pur.net, 0, pur.vat, 0, e1Gross, 0}},
		{"empty period", "2020-01-01", "2020-01-31", want{}},
	}
	for _, pp := range periods {
		pp := pp
		t.Run(pp.name, func(t *testing.T) {
			q, w := finPeriod(pp.from, pp.to), pp.w
			rev := w.sales - w.sret
			revVat := rev * 15 / 115
			r := finDash(t, s, "revenue", q)
			EqMoney(t, "revenue", F(r, "result.revenue"), rev)
			EqMoney(t, "revenue VAT", F(r, "result.vat"), revVat)
			EqMoney(t, "revenue without VAT", F(r, "result.revenueWithoutVat"), rev-finR2(revVat))
			EqMoney(t, "total revenue", F(r, "result.total"), rev)
			EqMoney(t, "sales input", F(r, "inputs.sales"), w.sales)
			EqMoney(t, "returns input", F(r, "inputs.salesReturn"), w.sret)
			if S(r["from"]) != pp.from || S(r["to"]) != pp.to || F(r, "vatPercent") != 15 || S(r["storeId"]) != s.ID {
				t.Errorf("revenue echo: %v %v %v", r["from"], r["to"], r["vatPercent"])
			}

			vt := finDash(t, s, "vat", q)
			out, in := w.salesVat-w.sretVat, w.purVat-w.pretVat
			EqMoney(t, "out VAT", F(vt, "result.outVat"), out)
			EqMoney(t, "in VAT", F(vt, "result.inVat"), in)
			// expense VAT is the expense "vat" field, which legacy expenses never set
			EqMoney(t, "expense VAT", F(vt, "result.expenseVat"), 0)
			EqMoney(t, "VAT payable", F(vt, "result.vatPayable"), out-in)

			te := finDash(t, s, "total-expense", q)
			total := w.exp + (w.pur - w.pret) + w.salary
			EqMoney(t, "expense input", F(te, "inputs.expense"), w.exp)
			EqMoney(t, "purchases", F(te, "result.purchases"), w.pur-w.pret)
			EqMoney(t, "salary", F(te, "result.salary"), w.salary)
			EqMoney(t, "total expense", F(te, "result.total"), total)
			EqMoney(t, "expense VAT part", F(te, "result.vat"), total*15/115)
			EqMoney(t, "expense without VAT", F(te, "result.totalWithoutVat"), total-finR2(total*15/115))
			if Get(te, "flags.employeeModule") != true {
				t.Errorf("employee module flag: %v", te["flags"])
			}

			np := finDash(t, s, "net-profit", q)
			EqMoney(t, "net profit", F(np, "result.profit"), rev-total)
			EqMoney(t, "profit revenue", F(np, "result.revenue"), rev)
			EqMoney(t, "profit expense", F(np, "result.expense"), total)
			EqMoney(t, "profit VAT", F(np, "result.vat"), (rev-total)*15/115)
			if (Get(np, "result.profitable") == true) != (Cents(rev-total) >= 0) {
				t.Errorf("profitable %v for %.2f", Get(np, "result.profitable"), rev-total)
			}

			// the list tiles cover the same days as the figures
			st := finStats(t, s, "sales", "sum=net,vat&"+q)
			EqMoney(t, "sales tile", F(st, "sums.net"), w.sales)
			EqMoney(t, "sales VAT tile", F(st, "sums.vat"), w.salesVat)
			st = finStats(t, s, "expenses", "sum=amount,vatAmount&"+q)
			EqMoney(t, "expense tiles", F(st, "sums.amount")+F(st, "sums.vatAmount"), w.exp)
		})
	}

	t.Run("store timezone decides the day", func(t *testing.T) {
		st := finStats(t, s, "sales", "sum=one&"+finPeriod(d1, d1))
		if F(st, "count") != 2 {
			t.Errorf("sales on %s: %v, want 2 (10:00 and 23:50)", d1, st["count"])
		}
		st = finStats(t, s, "sales", "sum=one&"+finPeriod(d2, d2))
		if F(st, "count") != 1 {
			t.Errorf("sales on %s: %v, want 1 (00:10)", d2, st["count"])
		}
		late := List(t, s.Token, "sales", finQS(s, finPeriod(d2, d2)))
		if len(late) != 1 || S(late[0]["date"]) != d2+"T00:10" {
			t.Errorf("sale at 00:10 store time: %v", late)
		}
	})

	t.Run("feed", func(t *testing.T) {
		f := finDash(t, s, "feed", "")
		byID := map[string]M{}
		for _, e := range Objs(Get(f, "feed.expenses")) {
			byID[S(e["id"])] = e
		}
		if e := byID[S(e1["id"])]; e == nil || Cents(F(e, "total")) != Cents(e1Gross) || Cents(F(e, "vat")) != 3000 || S(e["date"]) != d1+"T11:00" {
			t.Errorf("feed expense 1: %v", e)
		}
		if e := byID[S(e2["id"])]; e == nil || Cents(F(e, "total")) != Cents(e2Gross) {
			t.Errorf("feed expense 2: %v", e)
		}
		var dep, wd, sal float64
		for _, x := range Objs(Get(f, "feed.deposits")) {
			dep += F(x, "amount")
		}
		for _, x := range Objs(Get(f, "feed.withdrawals")) {
			wd += F(x, "amount")
		}
		for _, x := range Objs(Get(f, "feed.salaries")) {
			sal += F(x, "net")
		}
		EqMoney(t, "feed deposits", dep, 20)
		EqMoney(t, "feed withdrawals", wd, 5)
		EqMoney(t, "feed salaries", sal, 1000)
		var net float64
		for _, x := range Objs(Get(f, "feed.sales")) {
			net += F(x, "net")
		}
		EqMoney(t, "feed sales net", net, sA.net+sB.net+sC.net)
		if n := len(Objs(Get(f, "feed.sales"))); n != 3 {
			t.Errorf("feed sales: %d", n)
		}
		if Get(f, "snapshot.generatedAt") == nil || Get(f, "snapshot.stale") != false {
			t.Errorf("snapshot meta: %v", f["snapshot"])
		}
	})

	t.Run("bi", func(t *testing.T) {
		b := finDash(t, s, "bi", "")
		if S(Get(b, "period.today")) != s.Today() || len(Objs(Get(b, "ask.monthly"))) != 12 {
			t.Errorf("bi period: %v", b["period"])
		}
		// monthly revenue: sales before VAT less returns, by the store month
		want := map[string]float64{}
		orders := map[string]int{}
		want[d1[:7]] += sA.sub + sB.sub
		orders[d1[:7]] += 2
		want[d2[:7]] += sC.sub - ret.sub
		orders[d2[:7]]++
		for _, m := range Objs(Get(b, "ask.monthly")) {
			k := S(m["key"])
			EqMoney(t, "bi revenue "+k, F(m, "revenue"), want[k])
			if int(F(m, "orders")) != orders[k] {
				t.Errorf("bi orders %s: %v want %d", k, m["orders"], orders[k])
			}
		}
	})

	t.Run("errors and caching", func(t *testing.T) {
		for _, k := range []string{"revenue", "vat", "total-expense", "net-profit", "salary-balance", "bi", "feed"} {
			if r := Call(t, "GET", "/dashboard/"+k, s.Token, nil); r.Code != 400 || r.ErrField("storeId") == "" {
				t.Errorf("%s without storeId: %s", k, r)
			}
			Must(t, Call(t, "GET", "/dashboard/"+k+"?storeId="+s.ID, other.Token, nil), 404, k+" of another company")
			Must(t, Call(t, "GET", "/dashboard/"+k+"?storeId="+s.ID, "", nil), 401, k+" without token")
		}
		for _, k := range []string{"revenue", "vat", "total-expense", "net-profit"} {
			if r := Call(t, "GET", "/dashboard/"+k+"?"+finQS(s, "from=2026-02-30"), s.Token, nil); r.Code != 400 || r.ErrField("from") == "" {
				t.Errorf("%s bad from: %s", k, r)
			}
			if r := Call(t, "GET", "/dashboard/"+k+"?"+finQS(s, finPeriod(d2, d1)), s.Token, nil); r.Code != 400 || r.ErrField("to") == "" {
				t.Errorf("%s to before from: %s", k, r)
			}
		}
		// a salesman has no reports permission
		_, tok := s.User(t, "r_salesman")
		Must(t, Call(t, "GET", "/dashboard/revenue?storeId="+s.ID, tok, nil), 403, "salesman dashboard")
		_, acc := s.User(t, "r_accountant")
		Must(t, Call(t, "GET", "/dashboard/revenue?storeId="+s.ID, acc, nil), 200, "accountant dashboard")
		// the version answers 304 when nothing changed
		r := Must(t, Call(t, "GET", "/dashboard/vat?"+finQS(s, finPeriod(d1, d2)), s.Token, nil), 200, "vat")
		ver := S(Get(r.Body, "snapshot.version"))
		if r.Header.Get("ETag") != `"`+ver+`"` {
			t.Errorf("ETag %q, version %q", r.Header.Get("ETag"), ver)
		}
		Must(t, Call(t, "GET", "/dashboard/vat?"+finQS(s, finPeriod(d1, d2)), s.Token, nil, "If-None-Match", `"`+ver+`"`), 304, "unchanged vat")
	})
}

// ---------- countries ----------

type finCountry struct {
	cc, currency  string
	decimals      int
	vat           float64
	hasVat        bool
	title, tz     string
	badVat        string
	addrField     string
	extraRequired []string
}

var finCountries = []finCountry{
	{"SA", "SAR", 2, 15, true, "Tax Invoice", "Asia/Riyadh", "300000000000000", "buildingNo", nil},
	{"AE", "AED", 2, 5, true, "Tax Invoice", "Asia/Dubai", "12345", "cityEn", nil},
	{"OM", "OMR", 3, 5, true, "Tax Invoice", "Asia/Muscat", "OM12", "cityEn", nil},
	{"QA", "QAR", 2, 0, false, "Invoice", "Asia/Qatar", "@@@", "cityEn", nil},
	{"BH", "BHD", 3, 10, true, "Tax Invoice", "Asia/Bahrain", "2000", "cityEn", nil},
	{"KW", "KWD", 3, 0, false, "Invoice", "Asia/Kuwait", "@@@", "cityEn", nil},
	{"IN", "INR", 2, 18, true, "Tax Invoice", "Asia/Kolkata", "27AAPFU0939F1ZA", "cityEn", []string{"stateCode", "postalCode"}},
}

func TestCountryCatalog(t *testing.T) {
	t.Parallel()
	r := Must(t, Call(t, "GET", "/countries", "", nil), 200, "countries")
	got := map[string]M{}
	for _, c := range r.Data() {
		got[S(c["code"])] = c
	}
	for _, w := range finCountries {
		c := got[w.cc]
		if c == nil {
			t.Errorf("country %s missing", w.cc)
			continue
		}
		if S(Get(c, "currency.code")) != w.currency || int(F(c, "currency.decimals")) != w.decimals || F(c, "vatPercent") != w.vat ||
			c["hasVat"] != w.hasVat || S(c["invoiceTitleEn"]) != w.title || S(c["timeZone"]) != w.tz || c["supported"] != true {
			t.Errorf("country %s: %v", w.cc, c)
		}
		if (S(c["einvoicing"]) == "zatca") != (w.cc == "SA") {
			t.Errorf("country %s e-invoicing %q", w.cc, c["einvoicing"])
		}
	}
	if F(got["IN"], "roundingStep") != 1 || S(got["IN"]["taxSplit"]) != "gst" || len(Objs(got["IN"]["states"])) < 30 {
		t.Errorf("India: %v", got["IN"])
	}
	m := Must(t, Call(t, "GET", "/meta", "", nil), 200, "meta")
	if S(m.Body["adapter"]) != "pos-rest/erp" || F(m.Body, "apiVersion") != 1 || Get(m.Body, "capabilities.serverTotals") != true {
		t.Errorf("meta: %v", m.Body)
	}
}

func TestCountrySignupAndSale(t *testing.T) {
	t.Parallel()
	for _, w := range finCountries {
		w := w
		t.Run(w.cc, func(t *testing.T) {
			t.Parallel()
			t.Run("sign-up validation", func(t *testing.T) {
				cases := []struct {
					mod   func(b M)
					field string
				}{
					{func(b M) { b["company"].(M)["vatNo"] = w.badVat }, "company.vatNo"},
					{func(b M) { b["owner"].(M)["mobile"] = "123" }, "owner.mobile"},
					{func(b M) { b["company"].(M)["mobile"] = "12" }, "company.mobile"},
					{func(b M) { delete(b["company"].(M)["address"].(M), w.addrField) }, "company.address." + w.addrField},
					{func(b M) { b["owner"].(M)["password"] = "short" }, "owner.password"},
					{func(b M) { b["company"].(M)["plan"] = "gold" }, "company.plan"},
				}
				for _, f := range w.extraRequired {
					f := f
					cases = append(cases, struct {
						mod   func(b M)
						field string
					}{func(b M) { delete(b["company"].(M)["address"].(M), f) }, "company.address." + f})
				}
				if w.cc != "IN" {
					cases = append(cases, struct {
						mod   func(b M)
						field string
					}{func(b M) { b["company"].(M)["nameAr"] = "" }, "company.nameAr"})
				}
				for _, c := range cases {
					b := SignupBody(w.cc)
					c.mod(b)
					r := Call(t, "POST", "/auth/signup", "", b)
					if r.Code != 400 || r.ErrField(c.field) == "" {
						t.Errorf("%s: want 400 on %s, got %s", w.cc, c.field, r)
					}
				}
			})

			s := Signup(t, w.cc)
			if S(Get(s.Rec, "currency.code")) != w.currency || int(F(s.Rec, "currency.decimals")) != w.decimals ||
				F(s.Rec, "vatPercent") != w.vat || S(Get(s.Rec, "country.code")) != w.cc || S(s.Rec["timezone"]) != w.tz ||
				S(Get(s.Rec, "country.invoiceTitleEn")) != w.title {
				t.Errorf("store: currency %v, vat %v, timezone %v, title %v", s.Rec["currency"], s.Rec["vatPercent"], s.Rec["timezone"], Get(s.Rec, "country.invoiceTitleEn"))
			}

			// a sale at the country's rate, rounded to its currency
			qty, price := 3.0, 33.33
			if w.decimals == 3 {
				price = 1.239
			}
			p := s.Product(t, 1, price, 50)
			c := Create(t, s.Token, "customers", M{"storeId": s.ID, "nameEn": "Customer " + Uniq()})
			sale := Create(t, s.Token, "sales", M{"storeId": s.ID, "date": s.Now(), "customerId": c["id"], "items": []M{s.Line(S(p["id"]), qty, price)}})
			sub := qty * price
			unit := math.Pow(10, float64(w.decimals))
			vat := math.Round(sub*w.vat/100*unit) / unit
			if w.decimals == 3 {
				tot, vatGot, netGot := F(sale, "legacyTotals.total"), F(sale, "legacyTotals.vat"), F(sale, "legacyTotals.net")
				ok := math.Round(tot*1000) == math.Round(finR3(sub)*1000) && math.Round(vatGot*1000) == math.Round(vat*1000) &&
					math.Round(netGot*1000) == math.Round(finR3(sub+vat)*1000)
				KnownBug(t, "NEW-fin-3dec", fmt.Sprintf("%s (%s, 3 decimals): sale total/VAT/net %.3f/%.3f/%.3f, want %.3f/%.3f/%.3f (rounded to 2 decimals)",
					w.cc, w.currency, tot, vatGot, netGot, finR3(sub), vat, finR3(sub+vat)), !ok && Cents(tot) == Cents(sub))
				if !ok && Cents(tot) != Cents(sub) {
					t.Errorf("%s 3-decimal totals: %v", w.cc, sale["legacyTotals"])
				}
			} else {
				EqMoney(t, w.cc+" total", F(sale, "legacyTotals.total"), sub)
				EqMoney(t, w.cc+" VAT", F(sale, "legacyTotals.vat"), vat)
				EqMoney(t, w.cc+" net", F(sale, "legacyTotals.net"), sub+vat)
			}
			if !w.hasVat && F(sale, "legacyTotals.vat") != 0 {
				t.Errorf("%s has no VAT: sale VAT %v", w.cc, Get(sale, "legacyTotals.vat"))
			}
			rev := finDash(t, s, "revenue", "")
			if F(rev, "vatPercent") != w.vat {
				t.Errorf("%s dashboard VAT rate %v", w.cc, rev["vatPercent"])
			}
			if !w.hasVat && F(rev, "result.vat") != 0 {
				t.Errorf("%s dashboard VAT %v", w.cc, Get(rev, "result.vat"))
			}

			// ZATCA is Saudi only
			r := Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", s.Token, M{"otp": "123456"})
			if w.cc == "SA" {
				if r2 := Call(t, "POST", "/stores/"+s.ID+"/zatca/connect", s.Token, M{"otp": "12"}); r2.Code != 400 || r2.ErrCode() != "invalid_otp" {
					t.Errorf("SA bad OTP: %s", r2)
				}
			} else if r.Code != 409 || r.ErrCode() != "zatca_not_applicable" {
				t.Errorf("%s ZATCA connect: want 409 zatca_not_applicable, got %s", w.cc, r)
			}
		})
	}
}

// ---------- subscription billing ----------

func TestFinanceBilling(t *testing.T) {
	t.Parallel()
	s := Signup(t, "")
	other := Signup(t, "")
	admin := finPlatformAdmin(t)
	pdf := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n% e2e receipt\n%%EOF"))
	riyadh, _ := time.LoadLocation("Asia/Riyadh")
	today := time.Now().In(riyadh)
	day := func(n int) string { return today.AddDate(0, 0, n).Format("2006-01-02") }
	submit := func(extra M) M {
		b := M{"storeId": s.ID, "plan": "starter", "period": "monthly", "reference": "TRF " + Uniq() + Digits(3), "transferDate": day(0),
			"payerName": "دافع الاختبار", "payerBank": "Al Rajhi", "receipt": M{"name": "../../etc/receipt.pdf", "data": pdf}}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}

	t.Run("plans", func(t *testing.T) {
		r := Must(t, Call(t, "GET", "/billing/plans", s.Token, nil), 200, "plans")
		if F(r.Body, "vatRate") != 0.15 || S(r.Body["currency"]) != "SAR" {
			t.Errorf("plans: %v", r.Body)
		}
		monthly := map[string]float64{"starter": 99, "professional": 299}
		plans := Objs(r.Body["plans"])
		if len(plans) != 2 {
			t.Fatalf("plans: %v", plans)
		}
		for _, p := range plans {
			m := monthly[S(p["id"])]
			for per, sub := range map[string]float64{"monthly": m, "yearly": m * 10} {
				EqMoney(t, S(p["id"])+" "+per+" subtotal", F(p, per+".subtotal"), sub)
				EqMoney(t, S(p["id"])+" "+per+" VAT", F(p, per+".vat"), sub*0.15)
				EqMoney(t, S(p["id"])+" "+per+" total", F(p, per+".total"), sub*1.15)
			}
		}
		Must(t, Call(t, "GET", "/billing/plans", "", nil), 401, "plans without token")
	})

	t.Run("bank account", func(t *testing.T) {
		Must(t, Call(t, "GET", "/billing/bank-account", s.Token, nil), 200, "bank account")
		Must(t, Call(t, "GET", "/billing/bank-account", "", nil), 401, "bank account without token")
		good := M{"bankName": "Al Rajhi Bank", "accountName": "StartERP", "iban": "sa03 8000 0000 6080 1016 7519", "swift": "rjhisari"}
		Must(t, Call(t, "PUT", "/billing/bank-account", s.Token, good), 403, "store owner sets the company bank account")
		if admin == "" {
			return
		}
		for f, bad := range map[string]M{"iban": {"bankName": "B", "accountName": "A", "iban": "SA0380000000608010167518"},
			"bankName": {"accountName": "A", "iban": "SA0380000000608010167519"}, "swift": {"bankName": "B", "accountName": "A",
				"iban": "SA0380000000608010167519", "swift": "12"}} {
			if r := Call(t, "PUT", "/billing/bank-account", admin, bad); r.Code != 400 || r.ErrField(f) == "" {
				t.Errorf("bad bank account (%s): %s", f, r)
			}
		}
		r := Must(t, Call(t, "PUT", "/billing/bank-account", admin, good), 200, "admin sets bank account")
		if S(r.Body["iban"]) != "SA0380000000608010167519" || S(r.Body["swift"]) != "RJHISARI" || r.Body["configured"] != true {
			t.Errorf("saved bank account: %v", r.Body)
		}
		g := Must(t, Call(t, "GET", "/billing/bank-account?select=iban", s.Token, nil), 200, "bank account select")
		if S(g.Body["iban"]) != "SA0380000000608010167519" || g.Body["bankName"] != nil {
			t.Errorf("select=iban: %v", g.Body)
		}
	})

	t.Run("subscription", func(t *testing.T) {
		r := Must(t, Call(t, "GET", "/billing/subscription?storeId="+s.ID, s.Token, nil), 200, "subscription")
		if S(r.Body["status"]) != "trial" || S(r.Body["plan"]) != "professional" || S(r.Body["trialEndsAt"]) != day(14) || S(r.Body["paidUntil"]) != "" {
			t.Errorf("new store subscription: %v (want trial until %s)", r.Body, day(14))
		}
		if r := Call(t, "GET", "/billing/subscription", s.Token, nil); r.Code != 400 || r.ErrField("storeId") == "" {
			t.Errorf("no storeId: %s", r)
		}
		Must(t, Call(t, "GET", "/billing/subscription?storeId="+s.ID, other.Token, nil), 404, "another company's subscription")
	})

	var pay M
	t.Run("submit", func(t *testing.T) {
		cases := []struct {
			extra M
			field string
		}{
			{M{"plan": "enterprise"}, "plan"},
			{M{"plan": "gold"}, "plan"},
			{M{"period": "weekly"}, "period"},
			{M{"reference": "ab"}, "reference"},
			{M{"reference": "<script>"}, "reference"},
			{M{"transferDate": day(1)}, "transferDate"},
			{M{"transferDate": day(-91)}, "transferDate"},
			{M{"transferDate": "10/10/2026"}, "transferDate"},
			{M{"payerName": " "}, "payerName"},
			{M{"payerName": strings.Repeat("ن", 121)}, "payerName"},
			{M{"note": strings.Repeat("x", 501)}, "note"},
			{M{"receipt": M{}}, "receipt"},
			{M{"receipt": M{"data": "data:text/plain;base64,aGVsbG8="}}, "receipt"},
			{M{"receipt": M{"data": "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 not a png"))}}, "receipt"},
			{M{"receipt": M{"data": "data:application/pdf;base64,!!!"}}, "receipt"},
			{M{"storeId": ""}, "storeId"},
		}
		for _, c := range cases {
			if r := Call(t, "POST", "/billing/payments", s.Token, submit(c.extra)); r.Code != 400 || r.ErrField(c.field) == "" {
				t.Errorf("submit %v: want 400 on %s, got %s", c.extra, c.field, r)
			}
		}
		_, mgr := s.User(t, "r_manager")
		Must(t, Call(t, "POST", "/billing/payments", mgr, submit(nil)), 403, "manager pays the subscription")
		Must(t, Call(t, "POST", "/billing/payments", other.Token, submit(nil)), 404, "pay for another company")

		pay = Must(t, Call(t, "POST", "/billing/payments", s.Token, submit(M{"plan": "professional", "period": "yearly", "note": "سنوي"})), 201, "submit").Body
		EqMoney(t, "subtotal", F(pay, "subtotal"), 2990)
		EqMoney(t, "VAT", F(pay, "vat"), 448.5)
		EqMoney(t, "amount", F(pay, "amount"), 3438.5)
		if S(pay["status"]) != "pending" || !strings.HasPrefix(S(pay["number"]), "PAY-") || S(Get(pay, "receipt.name")) != "receipt.pdf" ||
			S(Get(pay, "receipt.type")) != "application/pdf" || S(pay["storeId"]) != s.ID {
			t.Errorf("payment: %v", pay)
		}
		r := Call(t, "POST", "/billing/payments", s.Token, submit(nil))
		if r.Code != 409 || r.ErrCode() != "pending_exists" {
			t.Errorf("second pending payment: %s", r)
		}
		sub := Must(t, Call(t, "GET", "/billing/subscription?storeId="+s.ID, s.Token, nil), 200, "subscription").Body
		if S(sub["pendingPaymentId"]) != S(pay["id"]) {
			t.Errorf("pending payment id: %v", sub)
		}
	})

	t.Run("read, list, summary, receipt", func(t *testing.T) {
		id := S(pay["id"])
		g := Must(t, Call(t, "GET", "/billing/payments/"+id, s.Token, nil), 200, "get payment").Body
		if S(g["reference"]) != S(pay["reference"]) || len(Objs(g["history"])) != 1 {
			t.Errorf("payment: %v", g)
		}
		Must(t, Call(t, "GET", "/billing/payments/"+id, other.Token, nil), 404, "another company's payment")
		Must(t, Call(t, "GET", "/billing/payments/"+primitive.NewObjectID().Hex(), s.Token, nil), 404, "unknown payment")
		rc := Must(t, Call(t, "GET", "/billing/payments/"+id+"/receipt", s.Token, nil), 200, "receipt").Body
		if S(rc["data"]) != pdf || S(rc["type"]) != "application/pdf" {
			t.Errorf("receipt round trip: %v", rc["type"])
		}
		Must(t, Call(t, "GET", "/billing/payments/"+id+"/receipt", other.Token, nil), 404, "another company's receipt")
		l := Must(t, Call(t, "GET", "/billing/payments?storeId="+s.ID, s.Token, nil), 200, "list").Body
		if F(l, "total") != 1 || S(Get(l, "data.0.id")) != id {
			t.Errorf("payments list: %v", l)
		}
		l = Must(t, Call(t, "GET", "/billing/payments?storeId="+s.ID+"&select=id,status&status=pending,cancelled", s.Token, nil), 200, "list select").Body
		if F(l, "total") != 1 || Get(l, "data.0.history") != nil || Get(l, "data.0.reference") != nil || S(Get(l, "data.0.status")) != "pending" {
			t.Errorf("payments list select: %v", l)
		}
		if r := Call(t, "GET", "/billing/payments?storeId="+s.ID+"&status=lost", s.Token, nil); r.Code != 400 || r.ErrField("status") == "" {
			t.Errorf("bad status filter: %s", r)
		}
		Must(t, Call(t, "GET", "/billing/payments", s.Token, nil), 400, "list without storeId")
		Must(t, Call(t, "GET", "/billing/payments?storeId="+s.ID, other.Token, nil), 404, "another company's payments")
		sm := Must(t, Call(t, "GET", "/billing/payments/summary?storeId="+s.ID, s.Token, nil), 200, "summary").Body
		if F(sm, "pending.count") != 1 || Cents(F(sm, "pending.amount")) != Cents(3438.5) || F(sm, "accepted.count") != 0 {
			t.Errorf("summary: %v", sm)
		}
		Must(t, Call(t, "GET", "/billing/payments/summary?storeId="+s.ID+"&status=x", s.Token, nil), 400, "summary bad status")
	})

	t.Run("review", func(t *testing.T) {
		id := S(pay["id"])
		Must(t, Call(t, "POST", "/billing/payments/"+id+"/accept", s.Token, M{}), 403, "store owner accepts")
		Must(t, Call(t, "POST", "/billing/payments/"+id+"/reject", s.Token, M{"reason": "not received"}), 403, "store owner rejects")
		Must(t, Call(t, "POST", "/billing/payments/"+id+"/cancel", other.Token, M{}), 404, "another company cancels")
		c := Must(t, Call(t, "POST", "/billing/payments/"+id+"/cancel", s.Token, M{}), 200, "cancel").Body
		if S(c["status"]) != "cancelled" {
			t.Errorf("cancelled: %v", c)
		}
		if r := Call(t, "POST", "/billing/payments/"+id+"/cancel", s.Token, M{}); r.Code != 409 || r.ErrCode() != "not_pending" {
			t.Errorf("cancel twice: %s", r)
		}
		if admin == "" {
			return
		}
		Must(t, Call(t, "POST", "/billing/payments/"+primitive.NewObjectID().Hex()+"/accept", admin, M{}), 404, "accept unknown")
		if r := Call(t, "POST", "/billing/payments/"+id+"/accept", admin, M{}); r.Code != 409 {
			t.Errorf("accept a cancelled payment: %s", r)
		}
		// accepted: the paid period starts the day after the trial
		p2 := Must(t, Call(t, "POST", "/billing/payments", s.Token, submit(M{"reference": pay["reference"], "period": "monthly"})), 201, "resubmit").Body
		if r := Call(t, "POST", "/billing/payments/"+S(p2["id"])+"/accept", admin, M{"note": strings.Repeat("n", 501)}); r.Code != 400 {
			t.Errorf("accept with a long note: %s", r)
		}
		a := Must(t, Call(t, "POST", "/billing/payments/"+S(p2["id"])+"/accept", admin, M{"note": "ok"}), 200, "accept").Body
		start := today.AddDate(0, 0, 15)
		end := start.AddDate(0, 1, -1)
		if S(a["status"]) != "accepted" || S(a["periodStart"]) != start.Format("2006-01-02") || S(a["periodEnd"]) != end.Format("2006-01-02") {
			t.Errorf("accepted: %v %v %v, want %s..%s", a["status"], a["periodStart"], a["periodEnd"], start.Format("2006-01-02"), end.Format("2006-01-02"))
		}
		sub := Must(t, Call(t, "GET", "/billing/subscription?storeId="+s.ID, s.Token, nil), 200, "subscription").Body
		if S(sub["status"]) != "active" || S(sub["paidUntil"]) != end.Format("2006-01-02") || S(sub["plan"]) != "starter" {
			t.Errorf("subscription after accept: %v", sub)
		}
		if r := Call(t, "POST", "/billing/payments", s.Token, submit(M{"reference": strings.ToLower(S(pay["reference"]))})); r.Code != 409 || r.ErrCode() != "duplicate_reference" {
			t.Errorf("accepted reference again: %s", r)
		}
		// rejected
		p3 := Must(t, Call(t, "POST", "/billing/payments", s.Token, submit(nil)), 201, "third").Body
		if r := Call(t, "POST", "/billing/payments/"+S(p3["id"])+"/reject", admin, M{"reason": "no"}); r.Code != 400 || r.ErrField("reason") == "" {
			t.Errorf("reject with a short reason: %s", r)
		}
		j := Must(t, Call(t, "POST", "/billing/payments/"+S(p3["id"])+"/reject", admin, M{"reason": "Transfer not found"}), 200, "reject").Body
		if S(j["status"]) != "rejected" || S(j["rejectionReason"]) != "Transfer not found" {
			t.Errorf("rejected: %v", j)
		}
		if r := Call(t, "POST", "/billing/payments/"+S(p3["id"])+"/reject", admin, M{"reason": "again please"}); r.Code != 409 {
			t.Errorf("reject twice: %s", r)
		}
		sm := Must(t, Call(t, "GET", "/billing/payments/summary?storeId="+s.ID, admin, nil), 200, "admin summary").Body
		if F(sm, "accepted.count") != 1 || F(sm, "rejected.count") != 1 || F(sm, "cancelled.count") != 1 || Cents(F(sm, "accepted.amount")) != Cents(113.85) {
			t.Errorf("summary: %v", sm)
		}
		Must(t, Call(t, "GET", "/billing/payments?status=pending&limit=1", admin, nil), 200, "admin lists every store")
	})
}

// ---------- starter catalog ----------

func TestFinanceStarterCatalog(t *testing.T) {
	t.Parallel()
	b := SignupBody("")
	b["company"].(M)["type"] = "Coffee Shop"
	s := SignupWith(t, b)
	retail := Signup(t, "")

	g := Must(t, Call(t, "GET", "/stores/"+s.ID+"/starter-catalog", s.Token, nil), 200, "preview").Body
	if g["available"] != true || S(g["terminal"]) != "coffee" || S(g["category"]) != "Coffee Shop" || F(g, "missing") != 0 || F(g, "added") == 0 {
		t.Fatalf("seeded at sign-up: %v", g)
	}
	prods := List(t, s.Token, "products", finQS(s, "limit=500"))
	if len(prods) == 0 || len(prods) < int(F(g, "counts.products")) {
		t.Fatalf("starter products: %d, counts %v", len(prods), g["counts"])
	}
	// nothing missing: seeding again adds nothing
	r := Must(t, Call(t, "POST", "/stores/"+s.ID+"/starter-catalog", s.Token, M{}), 200, "seed again").Body
	if F(r, "created.products") != 0 || F(r, "created.categories") != 0 {
		t.Errorf("seed twice: %v", r)
	}
	// a removed starter product comes back
	Must(t, Call(t, "DELETE", "/products/"+S(prods[0]["id"])+"?hard=1", s.Token, nil), 204, "remove a starter product")
	if g := Must(t, Call(t, "GET", "/stores/"+s.ID+"/starter-catalog", s.Token, nil), 200, "preview").Body; F(g, "missing") != 1 {
		t.Errorf("after removing one: %v", g)
	}
	r = Must(t, Call(t, "POST", "/stores/"+s.ID+"/starter-catalog", s.Token, M{}), 200, "seed").Body
	if F(r, "created.products")+F(r, "created.services") != 1 {
		t.Errorf("re-seed: %v", r)
	}
	if n := len(List(t, s.Token, "products", finQS(s, "limit=500"))); n != len(prods) {
		t.Errorf("products after re-seed: %d, want %d", n, len(prods))
	}

	// errors
	g = Must(t, Call(t, "GET", "/stores/"+retail.ID+"/starter-catalog", retail.Token, nil), 200, "retail preview").Body
	if g["available"] != false {
		t.Errorf("retail preview: %v", g)
	}
	if r := Call(t, "POST", "/stores/"+retail.ID+"/starter-catalog", retail.Token, M{}); r.Code != 409 || r.ErrCode() != "no_business_category" {
		t.Errorf("retail seed: %s", r)
	}
	Must(t, Call(t, "GET", "/stores/"+s.ID+"/starter-catalog", retail.Token, nil), 403, "another company's preview")
	Must(t, Call(t, "POST", "/stores/"+s.ID+"/starter-catalog", retail.Token, M{}), 403, "another company's seed")
	Must(t, Call(t, "POST", "/stores/"+s.ID+"/starter-catalog", "", M{}), 401, "seed without token")
	_, cashier := s.User(t, "r_cashier")
	Must(t, Call(t, "POST", "/stores/"+s.ID+"/starter-catalog", cashier, M{}), 403, "cashier seeds products")
}
