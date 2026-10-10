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

// TestDeliveryNote_Unauthenticated verifies that every delivery-note endpoint
// returns HTTP 401 with errors.access_token when no authentication token is
// provided.
func TestDeliveryNote_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListDeliveryNote",
			method:  http.MethodGet,
			path:    "/v1/delivery-note",
			handler: ListDeliveryNote,
		},
		{
			name:    "CreateDeliveryNote",
			method:  http.MethodPost,
			path:    "/v1/delivery-note",
			handler: CreateDeliveryNote,
		},
		{
			name:    "UpdateDeliveryNote",
			method:  http.MethodPut,
			path:    "/v1/delivery-note/64abc123456789001234abcd",
			handler: UpdateDeliveryNote,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewDeliveryNote",
			method:  http.MethodGet,
			path:    "/v1/delivery-note/64abc123456789001234abcd",
			handler: ViewDeliveryNote,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "CalculateDeliveryNoteNetTotal",
			method:  http.MethodPost,
			path:    "/v1/delivery-note/calculate-net-total",
			handler: CalculateDeliveryNoteNetTotal,
		},
		{
			name:    "ListDeliveryNoteReminders",
			method:  http.MethodGet,
			path:    "/v1/delivery-note/reminders",
			handler: ListDeliveryNoteReminders,
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

// TestDeliveryNote_Integration runs create -> view -> list -> update against a
// real MongoDB + Redis and checks validation and auth failures.
func TestDeliveryNote_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	storeURL := func(path string, kv ...string) string { return gbURL(path, fx.StoreA, kv...) }
	line := func(qty float64) []map[string]interface{} {
		return []map[string]interface{}{{"product_id": fx.ProductA1.Hex(), "name": "Engine Oil 5W30 4L",
			"quantity": qty, "unit_price": 120.0, "unit_price_with_vat": 138.0, "unit": "pcs"}}
	}
	remarks := uniqName("it delivery note")
	body := map[string]interface{}{
		"store_id": fx.StoreA.Hex(), "date_str": gbDateStr(), "customer_id": fx.CustomerA1.Hex(),
		"vat_percent": 15.0, "remarks": remarks, "products": line(2),
	}

	// auth + validation failures
	gbExpectErr(t, "create without token", callHandler(t, CreateDeliveryNote, "POST", "/v1/delivery-note", "", body), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "create without products/date",
		callHandler(t, CreateDeliveryNote, "POST", "/v1/delivery-note", tok, map[string]interface{}{"store_id": fx.StoreA.Hex(), "vat_percent": 15.0}),
		http.StatusBadRequest, "product_id", "date_str")
	gbExpectErr(t, "create with unknown product and short name",
		callHandler(t, CreateDeliveryNote, "POST", "/v1/delivery-note", tok, map[string]interface{}{
			"store_id": fx.StoreA.Hex(), "date_str": gbDateStr(), "vat_percent": 15.0,
			"products": []map[string]interface{}{{"product_id": primitive.NewObjectID().Hex(), "name": "ab", "quantity": 1}}}),
		http.StatusBadRequest, "product_id_0", "name_0")
	gbExpectErr(t, "create with bad date",
		callHandler(t, CreateDeliveryNote, "POST", "/v1/delivery-note", tok, map[string]interface{}{
			"store_id": fx.StoreA.Hex(), "date_str": "10/10/2026", "vat_percent": 15.0, "products": line(1)}),
		http.StatusBadRequest, "date_str")

	// create
	r := callHandler(t, CreateDeliveryNote, "POST", "/v1/delivery-note", tok, body)
	gbExpect(t, "create", r, http.StatusOK, true)
	created := r.resultMap(t)
	id := gbStr(created, "id")
	code := gbStr(created, "code")
	if id == "" || !strings.HasPrefix(code, "DN-") || code == "DN-0001" {
		t.Fatalf("create: unexpected id=%q code=%q (fixture already holds DN-0001)", id, code)
	}
	if gbStr(created, "customer_name") != "Riyadh Motors" || gbStr(created, "store_name") != "Al Noor Trading" || gbStr(created, "created_by_name") != "Admin T1" {
		t.Errorf("create: foreign labels not filled: %v", created)
	}

	// view
	r = callHandler(t, ViewDeliveryNote, "GET", storeURL("/v1/delivery-note/"+id), tok, nil, "id", id)
	gbExpect(t, "view", r, http.StatusOK, true)
	var viewed models.DeliveryNote
	if err := json.Unmarshal(r.Result, &viewed); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	if viewed.Code != code || viewed.Remarks != remarks || len(viewed.Products) != 1 {
		t.Fatalf("view: got code=%q remarks=%q products=%d", viewed.Code, viewed.Remarks, len(viewed.Products))
	}
	p := viewed.Products[0]
	if p.Quantity != 2 || p.ItemCode != "EO-5W30-4L" || p.PartNumber != "PN-001" || p.NameInArabic == "" {
		t.Errorf("view: product labels/quantity wrong: %+v", p)
	}
	if viewed.Customer == nil || viewed.Customer.Name != "Riyadh Motors" {
		t.Errorf("view: embedded customer missing: %+v", viewed.Customer)
	}

	// the document in the DB
	store := gbStore(t, fx.StoreA)
	oid, _ := primitive.ObjectIDFromHex(id)
	dn, err := store.FindDeliveryNoteByID(&oid, bson.M{})
	if err != nil {
		t.Fatalf("db read: %v", err)
	}
	if dn.CreatedBy == nil || *dn.CreatedBy != fx.Admin || dn.CustomerID == nil || *dn.CustomerID != fx.CustomerA1 || dn.Date == nil {
		t.Errorf("db: created_by/customer_id/date wrong: %+v", dn)
	}

	// list (filtered by customer, our note must be there exactly once)
	r = callHandler(t, ListDeliveryNote, "GET", storeURL("/v1/delivery-note", "search[customer_id]", fx.CustomerA1.Hex(), "search[code]", code, "limit", "100"), tok, nil)
	gbExpect(t, "list", r, http.StatusOK, true)
	rows := gbIDs(gbList(t, r))
	if _, ok := rows[id]; !ok {
		t.Fatalf("list: created note %s not found in %v", id, r.Raw)
	}
	if _, ok := rows[fx.DeliveryNoteA1.Hex()]; ok {
		t.Errorf("list: code filter %q also returned fixture note DN-0001", code)
	}
	if n := gbTotalCount(t, r); n < 1 {
		t.Errorf("list: total_count=%d", n)
	}

	// update: quantity + remarks change; code stays
	upd := map[string]interface{}{"store_id": fx.StoreA.Hex(), "date_str": gbDateStr(), "customer_id": fx.CustomerA1.Hex(),
		"vat_percent": 15.0, "remarks": remarks + " (updated)", "products": line(5)}
	r = callHandler(t, UpdateDeliveryNote, "PUT", storeURL("/v1/delivery-note/"+id), tok, upd, "id", id)
	gbExpect(t, "update", r, http.StatusOK, true)
	dn, err = store.FindDeliveryNoteByID(&oid, bson.M{})
	if err != nil {
		t.Fatalf("db read after update: %v", err)
	}
	if dn.Code != code || dn.Remarks != remarks+" (updated)" || len(dn.Products) != 1 || dn.Products[0].Quantity != 5 || dn.UpdatedByName != "Admin T1" {
		t.Errorf("update not persisted: code=%q remarks=%q products=%+v updated_by=%q", dn.Code, dn.Remarks, dn.Products, dn.UpdatedByName)
	}

	// update validation failure leaves the document unchanged
	bad := map[string]interface{}{"store_id": fx.StoreA.Hex(), "date_str": gbDateStr(), "vat_percent": 15.0,
		"products": []map[string]interface{}{{"product_id": fx.ProductA1.Hex(), "name": "Engine Oil", "quantity": 0}}}
	gbExpectErr(t, "update with zero quantity", callHandler(t, UpdateDeliveryNote, "PUT", storeURL("/v1/delivery-note/"+id), tok, bad, "id", id),
		http.StatusBadRequest, "quantity_0")
	if dn2, _ := store.FindDeliveryNoteByID(&oid, bson.M{}); dn2 == nil || dn2.Products[0].Quantity != 5 {
		t.Errorf("failed update changed the stored document: %+v", dn2)
	}

	// not found / bad id / missing store / legacy fixture doc
	missing := primitive.NewObjectID().Hex()
	gbExpectErr(t, "view unknown id", callHandler(t, ViewDeliveryNote, "GET", storeURL("/v1/delivery-note/"+missing), tok, nil, "id", missing), http.StatusBadRequest, "view")
	gbExpectErr(t, "update unknown id", callHandler(t, UpdateDeliveryNote, "PUT", storeURL("/v1/delivery-note/"+missing), tok, upd, "id", missing), http.StatusBadRequest, "view")
	gbExpectErr(t, "view bad id", callHandler(t, ViewDeliveryNote, "GET", storeURL("/v1/delivery-note/xyz"), tok, nil, "id", "xyz"), http.StatusBadRequest, "delivery_note_id")
	gbExpectErr(t, "list without store", callHandler(t, ListDeliveryNote, "GET", "/v1/delivery-note", tok, nil), http.StatusOK, "store_id")
	r = callHandler(t, ViewDeliveryNote, "GET", storeURL("/v1/delivery-note/"+fx.DeliveryNoteA1.Hex()), tok, nil, "id", fx.DeliveryNoteA1.Hex())
	gbExpect(t, "view legacy note", r, http.StatusOK, true)
	if gbStr(r.resultMap(t), "code") != "DN-0001" {
		t.Errorf("legacy note: %s", r.Raw)
	}
}
