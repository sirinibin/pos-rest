package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestStockTransfer_Unauthenticated verifies that every stock-transfer endpoint
// returns HTTP 401 with errors.access_token when no authentication token is
// provided.
func TestStockTransfer_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListStockTransfer",
			method:  http.MethodGet,
			path:    "/v1/stock-transfer",
			handler: ListStockTransfer,
		},
		{
			name:    "CreateStockTransfer",
			method:  http.MethodPost,
			path:    "/v1/stock-transfer",
			handler: CreateStockTransfer,
		},
		{
			name:    "UpdateStockTransfer",
			method:  http.MethodPut,
			path:    "/v1/stock-transfer/64abc123456789001234abcd",
			handler: UpdateStockTransfer,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateStockTransferNetTotal",
			method:  http.MethodPost,
			path:    "/v1/stock-transfer/calculate-net-total",
			handler: CalculateStockTransferNetTotal,
		},
		{
			name:    "ViewStockTransfer",
			method:  http.MethodGet,
			path:    "/v1/stock-transfer/64abc123456789001234abcd",
			handler: ViewStockTransfer,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewPreviousStockTransfer",
			method:  http.MethodGet,
			path:    "/v1/stock-transfer/64abc123456789001234abcd/previous",
			handler: ViewPreviousStockTransfer,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewNextStockTransfer",
			method:  http.MethodGet,
			path:    "/v1/stock-transfer/64abc123456789001234abcd/next",
			handler: ViewNextStockTransfer,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewLastStockTransfer",
			method:  http.MethodGet,
			path:    "/v1/stock-transfer/last",
			handler: ViewLastStockTransfer,
		},
		{
			name:    "DeleteStockTransfer",
			method:  http.MethodDelete,
			path:    "/v1/stock-transfer/64abc123456789001234abcd",
			handler: DeleteStockTransfer,
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

// TestStockTransfer_Integration creates two fresh warehouses, moves stock
// main store -> W1 -> W2 through the handlers, edits the second transfer and
// checks that the product's per-warehouse stock follows every step.
func TestStockTransfer_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	store := gbStore(t, fx.StoreA)
	product := fx.ProductA2 // plain (non-set) product with 50 pcs opening stock in the main store

	newWarehouse := func() (string, string) {
		t.Helper()
		r := callHandler(t, CreateWarehouse, "POST", gbURL("/v1/warehouse", fx.StoreA), tok,
			map[string]interface{}{"name": uniqName("it transfer wh"), "store_id": fx.StoreA.Hex()})
		gbExpect(t, "create warehouse", r, http.StatusOK, true)
		m := r.resultMap(t)
		return gbStr(m, "id"), gbStr(m, "code")
	}
	w1, w1Code := newWarehouse()
	w2, w2Code := newWarehouse()

	transfer := func(from, fromCode, to, toCode string, qty float64) map[string]interface{} {
		b := map[string]interface{}{
			"store_id": fx.StoreA.Hex(), "date_str": gbDateStr(), "vat_percent": 15.0, "remarks": "it transfer",
			"products": []map[string]interface{}{{"product_id": product.Hex(), "name": "Oil Filter", "quantity": qty,
				"unit_price": 10.0, "unit_price_with_vat": 11.5, "purchase_unit_price": 10.0, "unit": "pcs"}},
		}
		if from != "" {
			b["from_warehouse_id"], b["from_warehouse_code"] = from, fromCode
		}
		if to != "" {
			b["to_warehouse_id"], b["to_warehouse_code"] = to, toCode
		}
		return b
	}
	expectStock := func(what string, want map[string]float64) {
		t.Helper()
		gbWait(t, 15*time.Second, what, func() (bool, string) {
			stock, wh := gbWarehouseStocks(t, store, product)
			sum := 0.0
			for code, q := range wh {
				if code != "main_store" {
					sum += q
				}
			}
			state := fmt.Sprintf("stock=%v warehouse_stocks=%v", stock, wh)
			if wh["main_store"] != stock-sum {
				return false, state + " (main_store != stock - warehouses)"
			}
			for code, q := range want {
				if wh[code] != q {
					return false, state
				}
			}
			return true, state
		})
	}
	createURL := gbURL("/v1/stocktransfer", fx.StoreA)

	// auth + validation failures
	gbExpectErr(t, "create without token", callHandler(t, CreateStockTransfer, "POST", createURL, "", transfer("", "", w1, w1Code, 1)), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "main store to main store", callHandler(t, CreateStockTransfer, "POST", createURL, tok, transfer("", "", "", "", 1)),
		http.StatusBadRequest, "from_warehouse_id", "to_warehouse_id")
	gbExpectErr(t, "same warehouse", callHandler(t, CreateStockTransfer, "POST", createURL, tok, transfer(w1, w1Code, w1, w1Code, 1)),
		http.StatusBadRequest, "to_warehouse_id")
	noProducts := transfer("", "", w1, w1Code, 1)
	delete(noProducts, "products")
	gbExpectErr(t, "no products", callHandler(t, CreateStockTransfer, "POST", createURL, tok, noProducts), http.StatusBadRequest, "product_id")
	zero := transfer("", "", w1, w1Code, 0)
	zero["products"].([]map[string]interface{})[0]["unit_price"] = 0.0
	gbExpectErr(t, "zero quantity and price", callHandler(t, CreateStockTransfer, "POST", createURL, tok, zero), http.StatusBadRequest, "quantity_0", "unit_price_0")
	gbExpectErr(t, "create without store", callHandler(t, CreateStockTransfer, "POST", "/v1/stocktransfer", tok, transfer("", "", w1, w1Code, 1)), http.StatusOK, "store_id")

	// 1) main store -> W1: 3 pcs
	r := callHandler(t, CreateStockTransfer, "POST", createURL, tok, transfer("", "", w1, w1Code, 3))
	gbExpect(t, "create main->W1", r, http.StatusOK, true)
	st1 := r.resultMap(t)
	st1ID := gbStr(st1, "id")
	if !strings.HasPrefix(gbStr(st1, "code"), "ST-TR-") || gbNum(st1, "total_quantity") != 3 || gbNum(st1, "total") != 30 ||
		gbNum(st1, "vat_price") != 4.5 || gbNum(st1, "net_total") != 34.5 || gbStr(st1, "created_by_name") != "Admin T1" || gbStr(st1, "uuid") == "" {
		t.Errorf("create main->W1: unexpected totals/labels: %s", r.Raw)
	}
	expectStock("3 pcs in W1 after main->W1", map[string]float64{w1Code: 3, w2Code: 0})

	// 2) W1 -> W2: 2 pcs
	r = callHandler(t, CreateStockTransfer, "POST", createURL, tok, transfer(w1, w1Code, w2, w2Code, 2))
	gbExpect(t, "create W1->W2", r, http.StatusOK, true)
	st2ID := gbStr(r.resultMap(t), "id")
	expectStock("W1=1, W2=2 after W1->W2", map[string]float64{w1Code: 1, w2Code: 2})

	// view
	r = callHandler(t, ViewStockTransfer, "GET", gbURL("/v1/stocktransfer/"+st2ID, fx.StoreA), tok, nil, "id", st2ID)
	gbExpect(t, "view", r, http.StatusOK, true)
	var viewed models.StockTransfer
	if err := json.Unmarshal(r.Result, &viewed); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if viewed.FromWarehouseID == nil || viewed.FromWarehouseID.Hex() != w1 || viewed.ToWarehouseID == nil || viewed.ToWarehouseID.Hex() != w2 ||
		len(viewed.Products) != 1 || viewed.Products[0].Quantity != 2 || viewed.Products[0].ItemCode != "OF-1" {
		t.Errorf("view: %s", r.Raw)
	}

	// list: filter by destination warehouse
	r = callHandler(t, ListStockTransfer, "GET", gbURL("/v1/stocktransfer", fx.StoreA, "search[to_warehouse_id]", w1), tok, nil)
	gbExpect(t, "list to W1", r, http.StatusOK, true)
	rows := gbIDs(gbList(t, r))
	if len(rows) != 1 || rows[st1ID] == nil || gbTotalCount(t, r) != 1 {
		t.Errorf("list to W1: expected only %s, got %s", st1ID, r.Raw)
	}
	r = callHandler(t, ListStockTransfer, "GET", gbURL("/v1/stocktransfer", fx.StoreA, "search[from_warehouse_id]", w1), tok, nil)
	gbExpect(t, "list from W1", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); len(rows) != 1 || rows[st2ID] == nil {
		t.Errorf("list from W1: expected only %s, got %s", st2ID, r.Raw)
	}

	// 3) update the W1 -> W2 transfer to 1 pc: stock moves back
	upd := transfer(w1, w1Code, w2, w2Code, 1)
	r = callHandler(t, UpdateStockTransfer, "PUT", gbURL("/v1/stocktransfer/"+st2ID, fx.StoreA), tok, upd, "id", st2ID)
	gbExpect(t, "update W1->W2", r, http.StatusOK, true)
	if m := r.resultMap(t); gbNum(m, "total_quantity") != 1 || gbNum(m, "net_total") != 11.5 {
		t.Errorf("update totals: %s", r.Raw)
	}
	expectStock("W1=2, W2=1 after editing W1->W2 to 1 pc", map[string]float64{w1Code: 2, w2Code: 1})

	// update/view failures
	bad := transfer(w2, w2Code, w2, w2Code, 1)
	gbExpectErr(t, "update to same warehouse", callHandler(t, UpdateStockTransfer, "PUT", gbURL("/v1/stocktransfer/"+st2ID, fx.StoreA), tok, bad, "id", st2ID),
		http.StatusBadRequest, "to_warehouse_id")
	missing := primitive.NewObjectID().Hex()
	gbExpectErr(t, "update unknown", callHandler(t, UpdateStockTransfer, "PUT", gbURL("/v1/stocktransfer/"+missing, fx.StoreA), tok, upd, "id", missing),
		http.StatusBadRequest, "find_stocktransfer")
	gbExpectErr(t, "view unknown", callHandler(t, ViewStockTransfer, "GET", gbURL("/v1/stocktransfer/"+missing, fx.StoreA), tok, nil, "id", missing),
		http.StatusBadRequest, "view")
	stID, _ := primitive.ObjectIDFromHex(st2ID)
	if db, err := store.FindStockTransferByID(&stID, bson.M{}); err != nil || db.Products[0].Quantity != 1 || db.UpdatedByName != "Admin T1" {
		t.Errorf("db after failed updates: %+v err=%v", db, err)
	}
	expectStock("stock unchanged by rejected updates", map[string]float64{w1Code: 2, w2Code: 1})
}
