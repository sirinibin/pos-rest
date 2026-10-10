package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/erp/erpfixture"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestZatcaReportHandlers_Unauthenticated verifies that all four ZATCA report
// handlers (ReportOrderToZatca, ReportSalesReturnToZatca,
// ReportCustomerDepositToZatca, ReportCustomerWithdrawalToZatca) return HTTP
// 401 with errors["access_token"] when no authentication token is provided.
//
// These tests do not require a database connection because the auth check is
// the very first guard in each handler — the DB is never reached.
func TestZatcaReportHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
		muxVars map[string]string
	}{
		{
			name:    "ReportOrderToZatca",
			handler: ReportOrderToZatca,
			path:    "/v1/order/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ReportSalesReturnToZatca",
			handler: ReportSalesReturnToZatca,
			path:    "/v1/sales-return/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ReportCustomerDepositToZatca",
			handler: ReportCustomerDepositToZatca,
			path:    "/v1/customer-deposit/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ReportCustomerWithdrawalToZatca",
			handler: ReportCustomerWithdrawalToZatca,
			path:    "/v1/customer-withdrawal/zatca/report/64abc123456789001234abcd",
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(""))
			// Inject gorilla/mux path variables without starting a real router.
			r = mux.SetURLVars(r, tc.muxVars)

			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ──────────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ─────────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body ─────────────────────────────────────────────────────────
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if resp.Status {
				t.Error("expected status=false in body")
			}

			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"] key, got errors=%v", resp.Errors)
			}
		})
	}
}

// zatcaReportCases lists the four report handlers with the fixture document
// each one reports, the collection it lives in, and the handler's error keys
// for an unparsable id and for an id that is not found.
func zatcaReportCases(fx *erpfixture.Fixture) []struct {
	name     string
	handler  http.HandlerFunc
	coll     string
	docID    primitive.ObjectID
	idKey    string
	notFound string
} {
	return []struct {
		name     string
		handler  http.HandlerFunc
		coll     string
		docID    primitive.ObjectID
		idKey    string
		notFound string
	}{
		{"ReportOrderToZatca", ReportOrderToZatca, "order", fx.OrderA1, "order_id", "find_order"},
		{"ReportSalesReturnToZatca", ReportSalesReturnToZatca, "salesreturn", fx.SalesReturnA1, "sales_return_id", "find_sales_return"},
		{"ReportCustomerDepositToZatca", ReportCustomerDepositToZatca, "customerdeposit", fx.DepositA1, "deposit_id", "find_deposit"},
		{"ReportCustomerWithdrawalToZatca", ReportCustomerWithdrawalToZatca, "customerwithdrawal", fx.WithdrawalA1, "withdrawal_id", "find_withdrawal"},
	}
}

// TestZatcaReportHandlers_ReconnectRequired: in a store whose
// zatca.zatca_reconnect_required flag is set, every report handler answers
// 403 errors.zatca_reconnect before loading the document — the document is
// left untouched and ZATCA is never contacted.
func TestZatcaReportHandlers_ReconnectRequired(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	docs := map[string][]primitive.ObjectID{}
	for _, c := range zatcaReportCases(fx) {
		docs[c.coll] = append(docs[c.coll], c.docID)
	}
	// a copy of store A (with copies of the four documents) marked for
	// re-connection; zatca.connected stays false so that even a regression in
	// the guard could never reach the real ZATCA service from a test.
	storeID := gcCloneStore(t, fx.StoreA, bson.M{"zatca.zatca_reconnect_required": true, "zatca.connected": false}, docs)

	for _, c := range zatcaReportCases(fx) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			before := gcFindOne(t, storeID, c.coll, bson.M{"_id": c.docID})
			if before == nil {
				t.Fatalf("setup: %s %s not copied", c.coll, c.docID.Hex())
			}
			id := c.docID.Hex()
			r := callHandler(t, c.handler, "POST", "/v1/x/zatca/report/"+id+"?search[store_id]="+storeID.Hex(), tok, nil, "id", id)
			if r.Code != http.StatusForbidden || r.Status {
				t.Fatalf("want 403, got %d %s", r.Code, r.Raw)
			}
			msg, _ := r.Errors["zatca_reconnect"].(string)
			if !strings.Contains(msg, "ZATCA re-connection is required") {
				t.Fatalf("errors.zatca_reconnect = %q (%s)", msg, r.Raw)
			}
			if after := gcFindOne(t, storeID, c.coll, bson.M{"_id": c.docID}); !reflect.DeepEqual(before, after) {
				t.Fatalf("document changed although reporting was blocked:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// TestZatcaReportHandlers_InvalidID: with a valid token, an unparsable id is
// rejected with 400 errors.<kind>_id, and a well-formed id that does not
// exist in the store with 400 errors.find_<kind> (store not ZATCA-connected,
// so nothing is ever sent to ZATCA).
func TestZatcaReportHandlers_InvalidID(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	storeID := gcCloneStore(t, fx.StoreA, bson.M{"zatca.zatca_reconnect_required": false, "zatca.connected": false}, nil)
	q := "?search[store_id]=" + storeID.Hex()

	for _, c := range zatcaReportCases(fx) {
		c := c
		t.Run(c.name, func(t *testing.T) {
			r := callHandler(t, c.handler, "POST", "/v1/x/zatca/report/not-a-hex"+q, tok, nil, "id", "not-a-hex")
			if r.Code != http.StatusBadRequest || r.Status || r.Errors[c.idKey] == nil {
				t.Fatalf("bad id: want 400 errors.%s, got %d %s", c.idKey, r.Code, r.Raw)
			}
			unknown := primitive.NewObjectID().Hex()
			r = callHandler(t, c.handler, "POST", "/v1/x/zatca/report/"+unknown+q, tok, nil, "id", unknown)
			if r.Code != http.StatusBadRequest || r.Status || r.Errors[c.notFound] == nil {
				t.Fatalf("unknown id: want 400 errors.%s, got %d %s", c.notFound, r.Code, r.Raw)
			}
		})
	}
}

// TestClearZatcaReconnect_Unauthenticated verifies that PUT
// /v1/store/{id}/zatca/clear-reconnect returns HTTP 401 when no token is given.
func TestClearZatcaReconnect_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodPut,
		"/v1/store/64abc123456789001234abcd/zatca/clear-reconnect",
		strings.NewReader(""),
	)
	r = mux.SetURLVars(r, map[string]string{"id": "64abc123456789001234abcd"})

	w := httptest.NewRecorder()
	ClearZatcaReconnect(w, r)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}

	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["access_token"]; !ok {
		t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
	}
}

// TestClearZatcaReconnect_AdminOnly: Manager and SalesMan tokens get 403
// errors.role and the flag stays set; the Admin clears it (200) and
// zatca.zatca_reconnect_required is false in MongoDB afterwards.
func TestClearZatcaReconnect_AdminOnly(t *testing.T) {
	fx := requireDB(t)
	storeID := gcCloneStore(t, fx.StoreA, bson.M{"zatca.zatca_reconnect_required": true}, nil)
	id := storeID.Hex()
	path := "/v1/store/" + id + "/zatca/clear-reconnect"

	for _, email := range []string{fx.ManagerEmail, fx.SalesEmail} {
		r := callHandler(t, ClearZatcaReconnect, "PUT", path, tokenFor(t, email), nil, "id", id)
		if r.Code != http.StatusForbidden || r.Status || r.Errors["role"] == nil {
			t.Fatalf("%s: want 403 errors.role, got %d %s", email, r.Code, r.Raw)
		}
		if !gcStoreReconnectFlag(t, storeID) {
			t.Fatalf("%s: a non-admin must not clear the flag", email)
		}
	}

	r := callHandler(t, ClearZatcaReconnect, "PUT", path, tokenFor(t, fx.AdminEmail), nil, "id", id)
	if r.Code != http.StatusOK || !r.Status {
		t.Fatalf("admin: want 200, got %d %s", r.Code, r.Raw)
	}
	if gcStoreReconnectFlag(t, storeID) {
		t.Fatal("admin clear: zatca.zatca_reconnect_required still true in DB")
	}
}

// TestClearZatcaReconnect_InvalidStoreID: with an admin token an unparsable
// store id is 400 errors.id, and an unknown store id is 500 errors.store_id.
func TestClearZatcaReconnect_InvalidStoreID(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	r := callHandler(t, ClearZatcaReconnect, "PUT", "/v1/store/not-a-hex/zatca/clear-reconnect", tok, nil, "id", "not-a-hex")
	if r.Code != http.StatusBadRequest || r.Status || r.Errors["id"] == nil {
		t.Fatalf("bad id: want 400 errors.id, got %d %s", r.Code, r.Raw)
	}
	unknown := primitive.NewObjectID().Hex()
	r = callHandler(t, ClearZatcaReconnect, "PUT", "/v1/store/"+unknown+"/zatca/clear-reconnect", tok, nil, "id", unknown)
	if r.Code != http.StatusInternalServerError || r.Status || r.Errors["store_id"] == nil {
		t.Fatalf("unknown store: want 500 errors.store_id, got %d %s", r.Code, r.Raw)
	}
}
