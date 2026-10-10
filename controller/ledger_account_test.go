package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestLedgerAndAccount_Unauthenticated verifies that every ledger and account
// endpoint returns HTTP 401 with errors.access_token when no authentication
// token is provided.
func TestLedgerAndAccount_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── ledger.go ─────────────────────────────────────────────────────────
		{
			name:    "ListLedger",
			method:  http.MethodGet,
			path:    "/v1/ledger",
			handler: ListLedger,
		},
		// ── account.go ────────────────────────────────────────────────────────
		{
			name:    "ListAccounts",
			method:  http.MethodGet,
			path:    "/v1/account",
			handler: ListAccounts,
		},
		{
			name:    "ViewAccount",
			method:  http.MethodGet,
			path:    "/v1/account/64abc123456789001234abcd",
			handler: ViewAccount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteAccount",
			method:  http.MethodDelete,
			path:    "/v1/account/64abc123456789001234abcd",
			handler: DeleteAccount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "RestoreAccount",
			method:  http.MethodPost,
			path:    "/v1/account/64abc123456789001234abcd/restore",
			handler: RestoreAccount,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false in body")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors.access_token key, got errors=%v", resp.Errors)
			}
		})
	}
}

// TestLedger_Integration posts a capital deposit through CreateCapital (which
// writes a double-entry ledger) and reads it back through ListLedger.
func TestLedger_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)

	gbExpectErr(t, "list without token", callHandler(t, ListLedger, "GET", gbURL("/v1/ledger", fx.StoreA), "", nil), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "list without store", callHandler(t, ListLedger, "GET", "/v1/ledger", tok, nil), http.StatusOK, "store_id")
	gbExpectErr(t, "list with bad account id", callHandler(t, ListLedger, "GET", gbURL("/v1/ledger", fx.StoreA, "search[account_id]", "nope"), tok, nil), http.StatusOK, "find")

	const amount = 777.25
	r := callHandler(t, CreateCapital, "POST", gbURL("/v1/capital", fx.StoreA), tok, map[string]interface{}{
		"store_id": fx.StoreA.Hex(), "date_str": gbDateStr(), "invested_by_user_id": fx.Admin.Hex(),
		"payment_method": "cash", "amount": amount, "description": uniqName("it ledger capital"),
	})
	gbExpect(t, "create capital", r, http.StatusOK, true)
	capital := r.resultMap(t)
	capitalID, capitalCode := gbStr(capital, "id"), gbStr(capital, "code")
	if capitalID == "" || capitalCode == "" {
		t.Fatalf("capital without id/code: %s", r.Raw)
	}

	r = callHandler(t, ListLedger, "GET", gbURL("/v1/ledger", fx.StoreA, "search[reference_id]", capitalID), tok, nil)
	gbExpect(t, "list by reference", r, http.StatusOK, true)
	var ledgers []models.Ledger
	if err := json.Unmarshal(r.Result, &ledgers); err != nil {
		t.Fatalf("decode ledgers: %v (%s)", err, r.Raw)
	}
	if len(ledgers) != 1 || gbTotalCount(t, r) != 1 {
		t.Fatalf("expected exactly 1 ledger for capital %s, got %d (total_count=%d)", capitalID, len(ledgers), gbTotalCount(t, r))
	}
	l := ledgers[0]
	if l.ReferenceModel != "capital" || l.ReferenceCode != capitalCode || l.ReferenceID.Hex() != capitalID || l.StoreID == nil || *l.StoreID != fx.StoreA {
		t.Errorf("ledger reference fields wrong: %+v", l)
	}
	if len(l.Journals) != 2 {
		t.Fatalf("expected 2 journals, got %+v", l.Journals)
	}
	var debit, credit float64
	var investorAccount primitive.ObjectID
	for _, j := range l.Journals {
		switch j.DebitOrCredit {
		case "debit":
			debit += j.Debit
			if j.AccountName != "CASH" {
				t.Errorf("debit journal should hit CASH, got %q", j.AccountName)
			}
		case "credit":
			credit += j.Credit
			investorAccount = j.AccountID
			if j.AccountName != "ADMIN T1 CAPITAL" {
				t.Errorf("credit journal should hit the investor capital account, got %q", j.AccountName)
			}
		default:
			t.Errorf("journal without debit_or_credit: %+v", j)
		}
		if j.AccountID.IsZero() || j.AccountNumber == "" {
			t.Errorf("journal without account id/number: %+v", j)
		}
	}
	if debit != amount || credit != amount {
		t.Errorf("ledger not balanced at %v: debit=%v credit=%v", amount, debit, credit)
	}

	// the account filter finds the ledger through journals.account_id
	r = callHandler(t, ListLedger, "GET", gbURL("/v1/ledger", fx.StoreA, "search[account_id]", investorAccount.Hex(), "search[reference_id]", capitalID), tok, nil)
	gbExpect(t, "list by account", r, http.StatusOK, true)
	if rows := gbList(t, r); len(rows) != 1 {
		t.Errorf("account filter: expected 1 ledger, got %d", len(rows))
	}
	r = callHandler(t, ListLedger, "GET", gbURL("/v1/ledger", fx.StoreA, "search[account_id]", primitive.NewObjectID().Hex(), "search[reference_id]", capitalID), tok, nil)
	gbExpect(t, "list by unrelated account", r, http.StatusOK, true)
	if rows := gbList(t, r); len(rows) != 0 {
		t.Errorf("unrelated account filter returned %d ledgers", len(rows))
	}

	// the investor account exists in the store and is visible through the account API
	r = callHandler(t, ViewAccount, "GET", gbURL("/v1/account/"+investorAccount.Hex(), fx.StoreA), tok, nil, "id", investorAccount.Hex())
	gbExpect(t, "view investor account", r, http.StatusOK, true)
	if acc := r.resultMap(t); gbStr(acc, "name") != "ADMIN T1 CAPITAL" || gbStr(acc, "type") != "capital" {
		t.Errorf("investor account: %s", r.Raw)
	}
}

// TestAccount_Integration covers view/list/delete/restore of ledger accounts.
// Accounts have no create endpoint (posting creates them), so the test
// creates its own account through the model helper the posting code uses.
func TestAccount_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	store := gbStore(t, fx.StoreA)

	name := strings.ToUpper(uniqName("IT ACCOUNT"))
	acc, err := store.CreateAccountIfNotExists(&fx.StoreA, nil, nil, name, nil, nil)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	again, err := store.CreateAccountIfNotExists(&fx.StoreA, nil, nil, strings.ToLower(name), nil, nil)
	if err != nil || again.ID != acc.ID {
		t.Fatalf("CreateAccountIfNotExists is not idempotent by name: first=%s second=%+v err=%v", acc.ID.Hex(), again, err)
	}
	id := acc.ID.Hex()
	viewURL := gbURL("/v1/account/"+id, fx.StoreA)

	// failures
	gbExpectErr(t, "view without token", callHandler(t, ViewAccount, "GET", viewURL, "", nil, "id", id), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "view bad id", callHandler(t, ViewAccount, "GET", gbURL("/v1/account/xyz", fx.StoreA), tok, nil, "id", "xyz"), http.StatusBadRequest, "account_id")
	missing := primitive.NewObjectID().Hex()
	gbExpectErr(t, "view unknown id", callHandler(t, ViewAccount, "GET", gbURL("/v1/account/"+missing, fx.StoreA), tok, nil, "id", missing), http.StatusBadRequest, "view")
	gbExpectErr(t, "view in another store", callHandler(t, ViewAccount, "GET", gbURL("/v1/account/"+id, fx.StoreB), tok, nil, "id", id), http.StatusBadRequest, "view")
	gbExpectErr(t, "delete unknown id", callHandler(t, DeleteAccount, "DELETE", gbURL("/v1/account/"+missing, fx.StoreA), tok, nil, "id", missing), http.StatusBadRequest, "view")
	gbExpectErr(t, "list without store", callHandler(t, ListAccounts, "GET", "/v1/account", tok, nil), http.StatusOK, "store_id")

	// view
	r := callHandler(t, ViewAccount, "GET", viewURL, tok, nil, "id", id)
	gbExpect(t, "view", r, http.StatusOK, true)
	m := r.resultMap(t)
	if gbStr(m, "name") != name || gbStr(m, "number") != acc.Number || gbNum(m, "balance") != 0 || m["deleted"] != false {
		t.Errorf("view: %s", r.Raw)
	}

	// legacy fixture account
	r = callHandler(t, ViewAccount, "GET", gbURL("/v1/account/"+fx.AccountA1.Hex(), fx.StoreA), tok, nil, "id", fx.AccountA1.Hex())
	gbExpect(t, "view legacy account", r, http.StatusOK, true)
	if m := r.resultMap(t); gbStr(m, "name") != "CASH" || gbStr(m, "type") != "asset" || gbStr(m, "number") != "1000" {
		t.Errorf("legacy account: %s", r.Raw)
	}

	listed := func(what string, kv ...string) map[string]map[string]interface{} {
		t.Helper()
		r := callHandler(t, ListAccounts, "GET", gbURL("/v1/account", fx.StoreA, append([]string{"search[name]", name, "search[stats]", "1"}, kv...)...), tok, nil)
		gbExpect(t, what, r, http.StatusOK, true)
		return gbIDs(gbList(t, r))
	}

	rows := listed("list")
	row, ok := rows[id]
	if !ok || len(rows) != 1 {
		t.Fatalf("list by name: expected only %s, got %v", id, rows)
	}
	if want := name + " A/c #" + acc.Number; gbStr(row, "search_label") != want {
		t.Errorf("search_label = %q, want %q", gbStr(row, "search_label"), want)
	}

	// soft delete hides it from the default list, search[deleted]=1 shows it
	r = callHandler(t, DeleteAccount, "DELETE", viewURL, tok, nil, "id", id)
	gbExpect(t, "delete", r, http.StatusOK, true)
	if a, err := store.FindAccountByID(acc.ID, bson.M{}); err != nil || !a.Deleted {
		t.Fatalf("delete not persisted: %+v err=%v", a, err)
	}
	if rows := listed("list after delete"); len(rows) != 0 {
		t.Errorf("deleted account still listed: %v", rows)
	}
	if rows := listed("list deleted", "search[deleted]", "1"); rows[id] == nil {
		t.Errorf("deleted account not in search[deleted]=1 list")
	}

	// restore
	gbExpectErr(t, "restore without token", callHandler(t, RestoreAccount, "POST", viewURL, "", nil, "id", id), http.StatusUnauthorized, "access_token")
	r = callHandler(t, RestoreAccount, "POST", viewURL, tok, nil, "id", id)
	gbExpect(t, "restore", r, http.StatusOK, true)
	if a, err := store.FindAccountByID(acc.ID, bson.M{}); err != nil || a.Deleted {
		t.Fatalf("restore not persisted: %+v err=%v", a, err)
	}
	if rows := listed("list after restore"); rows[id] == nil {
		t.Errorf("restored account not listed")
	}
}
