package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---------------------------------------------------------------------------
// Unauthenticated handler tests
//
// ListProductJson is intentionally excluded: it has no auth guard and resolves
// a store_id query parameter first, so it cannot reach 401.
// ---------------------------------------------------------------------------

func TestProductHandlers_Unauthenticated(t *testing.T) {
	const fakeID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListProduct",
			method:  http.MethodGet,
			path:    "/v1/product",
			handler: ListProduct,
		},
		{
			name:    "CreateProduct",
			method:  http.MethodPost,
			path:    "/v1/product",
			handler: CreateProduct,
		},
		{
			name:    "UpdateProduct",
			method:  http.MethodPut,
			path:    "/v1/product/" + fakeID,
			handler: UpdateProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewProduct",
			method:  http.MethodGet,
			path:    "/v1/product/" + fakeID,
			handler: ViewProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ViewProductByItemCode",
			method:  http.MethodGet,
			path:    "/v1/product/code/SKU-001",
			handler: ViewProductByItemCode,
			muxVars: map[string]string{"code": "SKU-001"},
		},
		{
			name:    "ViewProductByBarCode",
			method:  http.MethodGet,
			path:    "/v1/product/barcode/1234567890",
			handler: ViewProductByBarCode,
			muxVars: map[string]string{"barcode": "1234567890"},
		},
		{
			name:    "DeleteProduct",
			method:  http.MethodDelete,
			path:    "/v1/product/" + fakeID,
			handler: DeleteProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "RestoreProduct",
			method:  http.MethodPost,
			path:    "/v1/product/" + fakeID + "/restore",
			handler: RestoreProduct,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "ProductSummary",
			method:  http.MethodGet,
			path:    "/v1/product/summary",
			handler: ProductSummary,
		},
		{
			name:    "GetProductBiHistory",
			method:  http.MethodGet,
			path:    "/v1/product/" + fakeID + "/bi-history",
			handler: GetProductBiHistory,
			muxVars: map[string]string{"id": fakeID},
		},
		{
			name:    "MigrateProductRackToWarehouseRacks",
			method:  http.MethodPost,
			path:    "/v1/product/migrate-racks",
			handler: MigrateProductRackToWarehouseRacks,
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
				t.Errorf("expected 401, got %d", res.StatusCode)
			}

			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected application/json Content-Type, got %q", ct)
			}

			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[access_token], got %v", resp.Errors)
			}
		})
	}
}

// TestListProductJson_NoAuthRequired documents that ListProductJson has no auth
// guard. An invalid store_id (missing query param) leads to a non-401 error.
func TestListProductJson_NoAuthRequired(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/product.json", nil)
	w := httptest.NewRecorder()
	ListProductJson(w, r)

	res := w.Result()
	defer res.Body.Close()

	// Without a valid store_id the handler returns an error, but NOT 401 — it
	// never checks auth. Any non-401 response proves the guard is absent.
	if res.StatusCode == http.StatusUnauthorized {
		t.Error("ListProductJson must not require auth (it is a BarTender endpoint); got 401")
	}
}

// ---------------------------------------------------------------------------
// Integration stubs
// ---------------------------------------------------------------------------

// createProductForTest creates a product in store A through the CreateProduct
// handler and registers its cleanup.
func createProductForTest(t *testing.T, storeID primitive.ObjectID, token string, body map[string]interface{}) (primitive.ObjectID, apiResp) {
	t.Helper()
	r := callHandler(t, CreateProduct, "POST", "/v1/product?search[store_id]="+storeID.Hex(), token, body)
	if r.Code != http.StatusOK {
		t.Fatalf("create product: want 200, got %d %s", r.Code, r.Raw)
	}
	id, err := primitive.ObjectIDFromHex(fmt.Sprint(r.resultMap(t)["id"]))
	if err != nil || id.IsZero() {
		t.Fatalf("create product: no id in %s", r.Raw)
	}
	cleanupProduct(t, storeID, id)
	return id, r
}

// historyState summarises product_history rows as "type:qty:stock,...".
func historyState(rows []bson.M) string {
	var parts []string
	for _, h := range rows {
		parts = append(parts, fmt.Sprintf("%v:%v:%v", h["reference_type"], num(h["quantity"]), num(h["stock"])))
	}
	return strings.Join(parts, ",")
}

// TestCreateProduct_Integration drives POST /v1/product: auth, store and
// validation errors, then a full create (generated EAN-12 barcode, category label,
// stock from the opening adjustment, creator) persisted in the store DB with
// its stock-adjustment history, and duplicate part-number rejection.
func TestCreateProduct_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	sA := fx.StoreA.Hex()
	url := "/v1/product?search[store_id]=" + sA

	if r := callHandler(t, CreateProduct, "POST", url, "", map[string]string{"name": "x"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", r.Code)
	}
	if r := callHandler(t, CreateProduct, "POST", "/v1/product", tok, map[string]string{"name": "xyz"}); r.Status || r.Errors["store_id"] == nil {
		t.Fatalf("missing store: want errors.store_id, got %d %s", r.Code, r.Raw)
	}
	if r := callHandler(t, CreateProduct, "POST", url, tok, productBody(fx.StoreA, "ab", "", primitive.NilObjectID, nil)); r.Code != http.StatusBadRequest || r.Errors["name"] == nil {
		t.Fatalf("short name: want 400 errors.name, got %d %s", r.Code, r.Raw)
	}
	if r := callHandler(t, CreateProduct, "POST", url, tok, productBody(fx.StoreA, uniqName("Bad Cat"), "", primitive.NewObjectID(), nil)); r.Code != http.StatusBadRequest || r.Errors["category_id_0"] == nil {
		t.Fatalf("unknown category: want 400 errors.category_id_0, got %d %s", r.Code, r.Raw)
	}
	bad := productBody(fx.StoreA, uniqName("Bad Adj"), "", primitive.NilObjectID, []map[string]interface{}{{"type": "adding", "quantity": 0}})
	if r := callHandler(t, CreateProduct, "POST", url, tok, bad); r.Code != http.StatusBadRequest ||
		r.Errors["adjustment_date_0"] == nil || r.Errors["adjustment_quantity_0"] == nil {
		t.Fatalf("invalid adjustment: want 400 date+quantity errors, got %d %s", r.Code, r.Raw)
	}

	name := uniqName("Brake Fluid DOT4")
	part := fmt.Sprintf("PN-T%d", time.Now().UnixNano())
	adj := []map[string]interface{}{{"date_str": "2026-01-15T10:00:00Z", "type": "adding", "quantity": 12, "reason": "opening"}}
	id, r := createProductForTest(t, fx.StoreA, tok, productBody(fx.StoreA, name, part, fx.CategoryA, adj))
	res := r.resultMap(t)
	if res["name"] != name || res["part_number"] != part || res["ean_12"] == nil || res["ean_12"] == "" || res["created_by"] != fx.Admin.Hex() {
		t.Fatalf("create response mismatch: %s", r.Raw)
	}

	storeA, err := models.FindStoreByID(&fx.StoreA, bson.M{})
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	p, err := storeA.FindProductByID(&id, bson.M{})
	if err != nil {
		t.Fatalf("created product not in DB: %v", err)
	}
	ps := p.ProductStores[sA]
	if p.Name != name || p.PartNumber != part || p.StoreID == nil || *p.StoreID != fx.StoreA || len(p.Ean12) != 12 ||
		len(p.CategoryName) != 1 || p.CategoryName[0] != "Engine Oil" || p.CreatedBy == nil || *p.CreatedBy != fx.Admin {
		t.Fatalf("persisted product mismatch: name=%q part=%q store=%v ean12=%q cats=%v createdBy=%v",
			p.Name, p.PartNumber, p.StoreID, p.Ean12, p.CategoryName, p.CreatedBy)
	}
	if ps.Stock != 12 || ps.RetailUnitPrice != 80 || ps.PurchaseUnitPrice != 50 || len(ps.StockAdjustments) != 1 {
		t.Fatalf("persisted product store mismatch: stock=%v retail=%v purchase=%v adjustments=%d",
			ps.Stock, ps.RetailUnitPrice, ps.PurchaseUnitPrice, len(ps.StockAdjustments))
	}
	if d := ps.StockAdjustments[0].Date; d == nil || !d.Equal(time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("adjustment date not parsed from date_str: %v", d)
	}

	// the opening adjustment is written to product_history in the background
	waitFor(t, 15*time.Second, "stock adjustment history", func() (bool, string) {
		rows := productHistory(t, fx.StoreA, id)
		st := historyState(rows)
		return st == "stock_adjustment_by_adding:12:12", st
	})

	// a second product with the same part number is rejected
	dup := productBody(fx.StoreA, uniqName("Dup Part"), part, primitive.NilObjectID, nil)
	if r := callHandler(t, CreateProduct, "POST", url, tok, dup); r.Code != http.StatusBadRequest || r.Errors["part_number"] != "Part Number Already Exists" {
		t.Fatalf("duplicate part number: want 400, got %d %s", r.Code, r.Raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if n, _ := db.GetDB("store_"+sA).Collection("product").CountDocuments(ctx, bson.M{"part_number": part}); n != 1 {
		t.Fatalf("want exactly 1 product with part number %s, got %d", part, n)
	}
}

// TestUpdateProduct_Integration_StockAdjustmentHistory drives
// PUT /v1/product/{id}: auth and id checks, then successive edits of the
// stock adjustments, each of which must recompute the stock and rebuild the
// product's stock-adjustment rows in product_history (clear + recreate).
func TestUpdateProduct_Integration_StockAdjustmentHistory(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	sA := fx.StoreA.Hex()
	name := uniqName("Coolant 1L")
	id, _ := createProductForTest(t, fx.StoreA, tok, productBody(fx.StoreA, name, "", primitive.NilObjectID, nil))
	url := "/v1/product/" + id.Hex() + "?search[store_id]=" + sA

	if r := callHandler(t, UpdateProduct, "PUT", url, "", map[string]string{"name": "x"}, "id", id.Hex()); r.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", r.Code)
	}
	if r := callHandler(t, UpdateProduct, "PUT", url, tok, map[string]string{"name": "xyz"}, "id", "bad"); r.Status || r.Errors["product_id"] == nil {
		t.Fatalf("bad id: want errors.product_id, got %s", r.Raw)
	}
	if r := callHandler(t, UpdateProduct, "PUT", url, tok, map[string]string{"name": "xyz"}, "id", primitive.NewObjectID().Hex()); r.Code != http.StatusBadRequest || r.Errors["product"] == nil {
		t.Fatalf("unknown product: want 400 errors.product, got %d %s", r.Code, r.Raw)
	}
	if got := productHistory(t, fx.StoreA, id); len(got) != 0 {
		t.Fatalf("product without adjustments must have no history, got %s", historyState(got))
	}

	steps := []struct {
		name      string
		adj       []map[string]interface{}
		wantStock float64
		wantHist  string
	}{
		{"add 7", []map[string]interface{}{
			{"date_str": "2026-02-01T08:00:00Z", "type": "adding", "quantity": 7, "reason": "found"},
		}, 7, "stock_adjustment_by_adding:7:7"},
		{"add 7 then remove 3", []map[string]interface{}{
			{"date_str": "2026-02-01T08:00:00Z", "type": "adding", "quantity": 7, "reason": "found"},
			{"date_str": "2026-02-02T08:00:00Z", "type": "removing", "quantity": 3, "reason": "damaged"},
		}, 4, "stock_adjustment_by_adding:7:7,stock_adjustment_by_removing:3:4"},
		{"replace with add 5", []map[string]interface{}{
			{"date_str": "2026-02-03T08:00:00Z", "type": "adding", "quantity": 5, "reason": "recount"},
		}, 5, "stock_adjustment_by_adding:5:5"},
		{"clear adjustments", nil, 0, ""},
	}
	storeA, err := models.FindStoreByID(&fx.StoreA, bson.M{})
	if err != nil {
		t.Fatalf("load store: %v", err)
	}
	for _, st := range steps {
		r := callHandler(t, UpdateProduct, "PUT", url, tok, productBody(fx.StoreA, name, "", primitive.NilObjectID, st.adj), "id", id.Hex())
		if r.Code != http.StatusOK || !r.Status {
			t.Fatalf("%s: want 200 status=true, got %d %s", st.name, r.Code, r.Raw)
		}
		p, err := storeA.FindProductByID(&id, bson.M{})
		if err != nil {
			t.Fatalf("%s: reload: %v", st.name, err)
		}
		ps := p.ProductStores[sA]
		if ps.Stock != st.wantStock || len(ps.StockAdjustments) != len(st.adj) || p.UpdatedBy == nil || *p.UpdatedBy != fx.Admin {
			t.Fatalf("%s: stock=%v adjustments=%d updatedBy=%v, want stock=%v adjustments=%d",
				st.name, ps.Stock, len(ps.StockAdjustments), p.UpdatedBy, st.wantStock, len(st.adj))
		}
		var res struct {
			ProductStores map[string]struct {
				Stock float64 `json:"stock"`
			} `json:"product_stores"`
		}
		_ = json.Unmarshal(r.Result, &res)
		if res.ProductStores[sA].Stock != st.wantStock {
			t.Fatalf("%s: response stock %v, want %v", st.name, res.ProductStores[sA].Stock, st.wantStock)
		}
		want := st.wantHist
		waitFor(t, 15*time.Second, st.name+" history", func() (bool, string) {
			s := historyState(productHistory(t, fx.StoreA, id))
			return s == want, s
		})
	}
}
