package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestWarehouseAndSignature_Unauthenticated verifies that every warehouse and
// signature endpoint returns HTTP 401 with errors.access_token when no
// authentication token is provided.
func TestWarehouseAndSignature_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── warehouse.go ──────────────────────────────────────────────────────
		{
			name:    "ListWarehouse",
			method:  http.MethodGet,
			path:    "/v1/warehouse",
			handler: ListWarehouse,
		},
		{
			name:    "CreateWarehouse",
			method:  http.MethodPost,
			path:    "/v1/warehouse",
			handler: CreateWarehouse,
		},
		{
			name:    "UpdateWarehouse",
			method:  http.MethodPut,
			path:    "/v1/warehouse/64abc123456789001234abcd",
			handler: UpdateWarehouse,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewWarehouse",
			method:  http.MethodGet,
			path:    "/v1/warehouse/64abc123456789001234abcd",
			handler: ViewWarehouse,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteWarehouse",
			method:  http.MethodDelete,
			path:    "/v1/warehouse/64abc123456789001234abcd",
			handler: DeleteWarehouse,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		// ── signature.go ──────────────────────────────────────────────────────
		{
			name:    "ListSignature",
			method:  http.MethodGet,
			path:    "/v1/signature",
			handler: ListSignature,
		},
		{
			name:    "CreateSignature",
			method:  http.MethodPost,
			path:    "/v1/signature",
			handler: CreateSignature,
		},
		{
			name:    "UpdateSignature",
			method:  http.MethodPut,
			path:    "/v1/signature/64abc123456789001234abcd",
			handler: UpdateSignature,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ViewSignature",
			method:  http.MethodGet,
			path:    "/v1/signature/64abc123456789001234abcd",
			handler: ViewSignature,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteSignature",
			method:  http.MethodDelete,
			path:    "/v1/signature/64abc123456789001234abcd",
			handler: DeleteSignature,
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

// TestWarehouse_Integration runs create -> view -> list -> update -> delete for
// a warehouse against a real MongoDB + Redis.
func TestWarehouse_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	store := gbStore(t, fx.StoreA)
	name := uniqName("IT Warehouse")
	body := map[string]interface{}{"name": name, "store_id": fx.StoreA.Hex(), "address": "Exit 5",
		"national_address": map[string]interface{}{"building_no": "4321", "zipcode": "12345", "city_name": "Riyadh"}}

	// validation errors must be rejected (note: the handler answers them with 500, not 400)
	rejected := func(what string, r apiResp, keys ...string) {
		t.Helper()
		if r.Status || r.Code < 400 {
			t.Fatalf("%s: expected a rejection, got code=%d body=%s", what, r.Code, r.Raw)
		}
		for _, k := range keys {
			if _, ok := r.Errors[k]; !ok {
				t.Errorf("%s: expected errors[%q], got %v", what, k, r.Errors)
			}
		}
	}
	gbExpectErr(t, "create without token", callHandler(t, CreateWarehouse, "POST", "/v1/warehouse", "", body), http.StatusUnauthorized, "access_token")
	rejected("create without name / bad address / bad phone", callHandler(t, CreateWarehouse, "POST", "/v1/warehouse", tok, map[string]interface{}{
		"store_id": fx.StoreA.Hex(), "phone": "abc",
		"national_address": map[string]interface{}{"building_no": "12", "zipcode": "123"}}),
		"name", "phone", "national_address_building_no", "national_address_zipcode")

	// create: the server assigns a WH<n> code (the fixture already holds WH1)
	r := callHandler(t, CreateWarehouse, "POST", "/v1/warehouse", tok, body)
	gbExpect(t, "create", r, http.StatusOK, true)
	created := r.resultMap(t)
	id, code := gbStr(created, "id"), gbStr(created, "code")
	if id == "" || !strings.HasPrefix(code, "WH") || code == "WH1" || gbStr(created, "created_by_name") != "Admin T1" {
		t.Fatalf("create: %s", r.Raw)
	}
	oid, _ := primitive.ObjectIDFromHex(id)
	viewURL := gbURL("/v1/warehouse/"+id, fx.StoreA)

	// a second warehouse gets a different code
	r = callHandler(t, CreateWarehouse, "POST", "/v1/warehouse", tok, map[string]interface{}{"name": uniqName("IT Warehouse B"), "store_id": fx.StoreA.Hex()})
	gbExpect(t, "create second", r, http.StatusOK, true)
	if c2 := gbStr(r.resultMap(t), "code"); c2 == code || !strings.HasPrefix(c2, "WH") {
		t.Errorf("warehouse codes not unique: %q vs %q", code, c2)
	}

	// view
	r = callHandler(t, ViewWarehouse, "GET", viewURL, tok, nil, "id", id)
	gbExpect(t, "view", r, http.StatusOK, true)
	var wh models.Warehouse
	if err := json.Unmarshal(r.Result, &wh); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if wh.Name != name || wh.Code != code || wh.Address != "Exit 5" || wh.NationalAddress.BuildingNo != "4321" || wh.StoreID == nil || *wh.StoreID != fx.StoreA {
		t.Errorf("view: %+v", wh)
	}
	missing := primitive.NewObjectID().Hex()
	gbExpectErr(t, "view unknown", callHandler(t, ViewWarehouse, "GET", gbURL("/v1/warehouse/"+missing, fx.StoreA), tok, nil, "id", missing), http.StatusOK, "view")
	gbExpectErr(t, "view bad id", callHandler(t, ViewWarehouse, "GET", gbURL("/v1/warehouse/xyz", fx.StoreA), tok, nil, "id", "xyz"), http.StatusOK, "warehouse_id")

	// list by name / by code; the fixture warehouse is in the full list
	r = callHandler(t, ListWarehouse, "GET", gbURL("/v1/warehouse", fx.StoreA, "search[name]", name), tok, nil)
	gbExpect(t, "list by name", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); len(rows) != 1 || rows[id] == nil || gbTotalCount(t, r) != 1 {
		t.Errorf("list by name: %s", r.Raw)
	}
	r = callHandler(t, ListWarehouse, "GET", gbURL("/v1/warehouse", fx.StoreA, "limit", "1000"), tok, nil)
	gbExpect(t, "list all", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); rows[fx.WarehouseA.Hex()] == nil || rows[id] == nil {
		t.Errorf("full list misses the fixture or the new warehouse: %s", r.Raw)
	}
	gbExpectErr(t, "list without store", callHandler(t, ListWarehouse, "GET", "/v1/warehouse", tok, nil), http.StatusOK, "store_id")

	// update
	upd := map[string]interface{}{"name": name + " v2", "store_id": fx.StoreA.Hex(), "address": "Exit 7"}
	r = callHandler(t, UpdateWarehouse, "PUT", viewURL, tok, upd, "id", id)
	gbExpect(t, "update", r, http.StatusOK, true)
	stored, err := store.FindWarehouseByID(&oid, bson.M{})
	if err != nil || stored.Name != name+" v2" || stored.Address != "Exit 7" || stored.Code != code || stored.UpdatedByName != "Admin T1" {
		t.Fatalf("update not persisted: %+v err=%v", stored, err)
	}
	rejected("update with blank name", callHandler(t, UpdateWarehouse, "PUT", viewURL, tok, map[string]interface{}{"name": "  ", "store_id": fx.StoreA.Hex()}, "id", id), "name")
	rejected("update unknown", callHandler(t, UpdateWarehouse, "PUT", gbURL("/v1/warehouse/"+missing, fx.StoreA), tok, upd, "id", missing), "view")
	if s, _ := store.FindWarehouseByID(&oid, bson.M{}); s == nil || s.Name != name+" v2" {
		t.Errorf("rejected update changed the warehouse: %+v", s)
	}

	// delete (soft): hidden from the list, flagged in the DB
	gbExpectErr(t, "delete without token", callHandler(t, DeleteWarehouse, "DELETE", viewURL, "", nil, "id", id), http.StatusUnauthorized, "access_token")
	r = callHandler(t, DeleteWarehouse, "DELETE", viewURL, tok, nil, "id", id)
	gbExpect(t, "delete", r, http.StatusOK, true)
	if s, err := store.FindWarehouseByID(&oid, bson.M{}); err != nil || !s.Deleted || s.DeletedBy == nil || *s.DeletedBy != fx.Admin {
		t.Errorf("delete not persisted: %+v err=%v", s, err)
	}
	r = callHandler(t, ListWarehouse, "GET", gbURL("/v1/warehouse", fx.StoreA, "search[name]", name), tok, nil)
	gbExpect(t, "list after delete", r, http.StatusOK, true)
	if rows := gbList(t, r); len(rows) != 0 {
		t.Errorf("deleted warehouse still listed: %s", r.Raw)
	}
}

// TestSignature_Integration runs create -> view -> list -> update -> delete for
// a user signature (base64 image saved to disk) against a real MongoDB + Redis.
func TestSignature_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	store := gbStore(t, fx.StoreA)
	name := uniqName("IT Signature")
	body := map[string]interface{}{"name": name, "store_id": fx.StoreA.Hex(), "signature_content": "data:image/png;base64," + gbTinyPNGBase64}

	gbExpectErr(t, "create without token", callHandler(t, CreateSignature, "POST", "/v1/signature", "", body), http.StatusUnauthorized, "access_token")
	gbExpectErr(t, "create without name/content", callHandler(t, CreateSignature, "POST", "/v1/signature", tok, map[string]interface{}{"store_id": fx.StoreA.Hex()}),
		http.StatusBadRequest, "name", "signature_content")
	gbExpectErr(t, "create with invalid base64", callHandler(t, CreateSignature, "POST", "/v1/signature", tok,
		map[string]interface{}{"name": uniqName("IT Sig bad"), "store_id": fx.StoreA.Hex(), "signature_content": "%%%not-base64%%%"}),
		http.StatusBadRequest, "signature_content")

	// create: the image is written to images/<store>/signatures/signature_<id>.png
	r := callHandler(t, CreateSignature, "POST", "/v1/signature", tok, body)
	gbExpect(t, "create", r, http.StatusOK, true)
	created := r.resultMap(t)
	id := gbStr(created, "id")
	file := "signature_" + id + ".png"
	diskPath := filepath.Join("images", fx.StoreA.Hex(), "signatures", file)
	t.Cleanup(func() { gbRemoveFileAndEmptyParents(diskPath, "images") })
	imageURL := "/images/" + fx.StoreA.Hex() + "/signatures/" + file
	if id == "" || gbStr(created, "signature") != imageURL || gbStr(created, "signature_content") != "" || gbStr(created, "created_by_name") != "Admin T1" {
		t.Fatalf("create: want signature URL %q; got %s", imageURL, r.Raw)
	}
	if data, err := os.ReadFile(diskPath); err != nil || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Errorf("signature image not written to %s: err=%v", diskPath, err)
	}
	oid, _ := primitive.ObjectIDFromHex(id)
	viewURL := gbURL("/v1/signature/"+id, fx.StoreA)
	if s, err := store.FindSignatureByID(&oid, bson.M{}); err != nil || s.Signature != file || s.CreatedBy == nil || *s.CreatedBy != fx.Admin {
		t.Errorf("db after create: %+v err=%v (want stored file name %q)", s, err, file)
	}

	// the same user may not reuse the name
	gbExpectErr(t, "duplicate name", callHandler(t, CreateSignature, "POST", "/v1/signature", tok, body), http.StatusConflict, "name")

	// view + list
	r = callHandler(t, ViewSignature, "GET", viewURL, tok, nil, "id", id)
	gbExpect(t, "view", r, http.StatusOK, true)
	if m := r.resultMap(t); gbStr(m, "name") != name || gbStr(m, "signature") != imageURL || gbStr(m, "store_id") != fx.StoreA.Hex() {
		t.Errorf("view: %s", r.Raw)
	}
	missing := primitive.NewObjectID().Hex()
	gbExpectErr(t, "view unknown", callHandler(t, ViewSignature, "GET", gbURL("/v1/signature/"+missing, fx.StoreA), tok, nil, "id", missing), http.StatusOK, "view")
	gbExpectErr(t, "view in another store", callHandler(t, ViewSignature, "GET", gbURL("/v1/signature/"+id, fx.StoreB), tok, nil, "id", id), http.StatusOK, "view")
	r = callHandler(t, ListSignature, "GET", gbURL("/v1/signature", fx.StoreA, "search[name]", name), tok, nil)
	gbExpect(t, "list", r, http.StatusOK, true)
	if rows := gbIDs(gbList(t, r)); len(rows) != 1 || rows[id] == nil || gbTotalCount(t, r) != 1 {
		t.Errorf("list by name: %s", r.Raw)
	}

	// update: rename without new content keeps the file
	r = callHandler(t, UpdateSignature, "PUT", viewURL, tok, map[string]interface{}{"name": name + " v2"}, "id", id)
	gbExpect(t, "update", r, http.StatusOK, true)
	stored, err := store.FindSignatureByID(&oid, bson.M{})
	if err != nil || stored.Name != name+" v2" || stored.Signature != file || stored.UpdatedByName != "Admin T1" {
		t.Fatalf("update not persisted: %+v err=%v", stored, err)
	}
	gbExpectErr(t, "update with invalid base64", callHandler(t, UpdateSignature, "PUT", viewURL, tok,
		map[string]interface{}{"signature_content": "%%%"}, "id", id), http.StatusBadRequest, "signature_content")
	gbExpectErr(t, "update with blank name", callHandler(t, UpdateSignature, "PUT", viewURL, tok,
		map[string]interface{}{"name": ""}, "id", id), http.StatusBadRequest, "name")
	if s, _ := store.FindSignatureByID(&oid, bson.M{}); s == nil || s.Name != name+" v2" {
		t.Errorf("rejected update changed the signature: %+v", s)
	}

	// delete (soft)
	gbExpectErr(t, "delete without token", callHandler(t, DeleteSignature, "DELETE", viewURL, "", nil, "id", id), http.StatusUnauthorized, "access_token")
	r = callHandler(t, DeleteSignature, "DELETE", viewURL, tok, nil, "id", id)
	gbExpect(t, "delete", r, http.StatusOK, true)
	if s, err := store.FindSignatureByID(&oid, bson.M{}); err != nil || !s.Deleted || s.DeletedBy == nil || *s.DeletedBy != fx.Admin {
		t.Errorf("delete not persisted: %+v err=%v", s, err)
	}
	r = callHandler(t, ListSignature, "GET", gbURL("/v1/signature", fx.StoreA, "search[name]", name), tok, nil)
	gbExpect(t, "list after delete", r, http.StatusOK, true)
	if rows := gbList(t, r); len(rows) != 0 {
		t.Errorf("deleted signature still listed: %s", r.Raw)
	}
}
