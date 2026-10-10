package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestVehicleAndRepairJob_Unauthenticated verifies that every vehicle and
// repair-job endpoint returns HTTP 401 with errors.access_token when no
// authentication token is provided.
func TestVehicleAndRepairJob_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── vehicle.go ────────────────────────────────────────────────────────
		{
			name:    "ListVehicle",
			method:  http.MethodGet,
			path:    "/v1/vehicle",
			handler: ListVehicle,
		},
		{
			name:    "CreateVehicle",
			method:  http.MethodPost,
			path:    "/v1/vehicle",
			handler: CreateVehicle,
		},
		{
			name:    "ViewVehicle",
			method:  http.MethodGet,
			path:    "/v1/vehicle/64abc123456789001234abcd",
			handler: ViewVehicle,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateVehicle",
			method:  http.MethodPut,
			path:    "/v1/vehicle/64abc123456789001234abcd",
			handler: UpdateVehicle,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteVehicle",
			method:  http.MethodDelete,
			path:    "/v1/vehicle/64abc123456789001234abcd",
			handler: DeleteVehicle,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "ListVehicleBrands",
			method:  http.MethodGet,
			path:    "/v1/vehicle/brands",
			handler: ListVehicleBrands,
		},
		// ── repair_job.go ─────────────────────────────────────────────────────
		{
			name:    "ListRepairJob",
			method:  http.MethodGet,
			path:    "/v1/repair-job",
			handler: ListRepairJob,
		},
		{
			name:    "CreateRepairJob",
			method:  http.MethodPost,
			path:    "/v1/repair-job",
			handler: CreateRepairJob,
		},
		{
			name:    "ViewRepairJob",
			method:  http.MethodGet,
			path:    "/v1/repair-job/64abc123456789001234abcd",
			handler: ViewRepairJob,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "UpdateRepairJob",
			method:  http.MethodPut,
			path:    "/v1/repair-job/64abc123456789001234abcd",
			handler: UpdateRepairJob,
			muxVars: map[string]string{"id": "64abc123456789001234abcd"},
		},
		{
			name:    "DeleteRepairJob",
			method:  http.MethodDelete,
			path:    "/v1/repair-job/64abc123456789001234abcd",
			handler: DeleteRepairJob,
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

// TestVehicle_Integration: create → view → list (search) → update against the
// real store DB. current_km is read-only on update (managed by sales).
func TestVehicle_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	sid := fx.StoreA.Hex()
	plate := fmt.Sprintf("GC-%d", time.Now().UnixNano()%1e9)

	// validation: brand/model/customer are required on create
	if r := callHandler(t, CreateVehicle, "POST", "/v1/vehicle", tok, map[string]interface{}{"store_id": sid}); r.Code != http.StatusBadRequest || r.Status ||
		r.Errors["brand"] == nil || r.Errors["model"] == nil || r.Errors["customer_id"] == nil {
		t.Fatalf("create validation: %d %s", r.Code, r.Raw)
	}

	cr := callHandler(t, CreateVehicle, "POST", "/v1/vehicle", tok, map[string]interface{}{
		"store_id": sid, "customer_id": fx.CustomerA1.Hex(), "brand": "Toyota", "model": "Camry",
		"vehicle_number": plate, "year": 2022, "current_km": 1234.0, "color": "White",
	})
	if cr.Code != http.StatusOK || !cr.Status {
		t.Fatalf("create: %d %s", cr.Code, cr.Raw)
	}
	created := cr.resultMap(t)
	id, _ := created["id"].(string)
	if id == "" || created["created_by"] != fx.Admin.Hex() || created["created_by_name"] == "" {
		t.Fatalf("create result: %v", created)
	}
	oid, _ := primitive.ObjectIDFromHex(id)
	if d := gcFindOne(t, fx.StoreA, "vehicle", bson.M{"_id": oid}); d == nil || d["vehicle_number"] != plate {
		t.Fatalf("vehicle not persisted: %v", d)
	}

	q := "?search[store_id]=" + sid
	vr := callHandler(t, ViewVehicle, "GET", "/v1/vehicle/"+id+q, tok, nil, "id", id)
	if !vr.Status || vr.resultMap(t)["vehicle_number"] != plate || vr.resultMap(t)["brand"] != "Toyota" {
		t.Fatalf("view: %s", vr.Raw)
	}
	if r := callHandler(t, ViewVehicle, "GET", "/v1/vehicle/x"+q, tok, nil, "id", "x"); r.Status || r.Errors["id"] == nil {
		t.Fatalf("view bad id: %s", r.Raw)
	}
	if r := callHandler(t, ViewVehicle, "GET", "/v1/vehicle/"+id, tok, nil, "id", id); r.Status || r.Errors["store_id"] == nil {
		t.Fatalf("view without store: %s", r.Raw)
	}

	lr := callHandler(t, ListVehicle, "GET", "/v1/vehicle"+q+"&search[vehicle_number]="+url.QueryEscape(plate), tok, nil)
	var list []map[string]interface{}
	if err := json.Unmarshal(lr.Result, &list); err != nil || !lr.Status || len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("list by plate: %s", lr.Raw)
	}

	ur := callHandler(t, UpdateVehicle, "PUT", "/v1/vehicle/"+id+q, tok, map[string]interface{}{
		"color": "Black", "model": "Corolla", "current_km": 99999.0,
	}, "id", id)
	if !ur.Status {
		t.Fatalf("update: %d %s", ur.Code, ur.Raw)
	}
	after := gcFindOne(t, fx.StoreA, "vehicle", bson.M{"_id": oid})
	if after["color"] != "Black" || after["model"] != "Corolla" || after["brand"] != "Toyota" {
		t.Fatalf("update not persisted: %v", after)
	}
	if km, _ := after["current_km"].(float64); km != 1234 {
		t.Fatalf("current_km must stay read-only on update: got %v", after["current_km"])
	}
}

// TestRepairJob_Integration: create (auto job number, default status) → view
// → list → update, with the vehicle's details copied onto the job.
func TestRepairJob_Integration(t *testing.T) {
	fx := requireDB(t)
	tok := tokenFor(t, fx.AdminEmail)
	sid := fx.StoreA.Hex()
	q := "?search[store_id]=" + sid

	if r := callHandler(t, CreateRepairJob, "POST", "/v1/repair-job"+q, tok, map[string]interface{}{}); r.Code != http.StatusBadRequest || r.Errors["title"] == nil {
		t.Fatalf("create validation: %d %s", r.Code, r.Raw)
	}

	plate := fmt.Sprintf("RJ-%d", time.Now().UnixNano()%1e9)
	vr := callHandler(t, CreateVehicle, "POST", "/v1/vehicle", tok, map[string]interface{}{
		"store_id": sid, "customer_id": fx.CustomerA1.Hex(), "brand": "Nissan", "model": "Patrol", "vehicle_number": plate,
	})
	if !vr.Status {
		t.Fatalf("vehicle: %s", vr.Raw)
	}
	vehicleID := vr.resultMap(t)["id"].(string)

	title := uniqName("Brake service")
	cr := callHandler(t, CreateRepairJob, "POST", "/v1/repair-job"+q, tok, map[string]interface{}{
		"title": title, "vehicle_id": vehicleID, "complaint": "squeaking", "labour_charge": 100.0, "vat_percent": 15.0,
	})
	if cr.Code != http.StatusOK || !cr.Status {
		t.Fatalf("create: %d %s", cr.Code, cr.Raw)
	}
	job := cr.resultMap(t)
	id, _ := job["id"].(string)
	jn, _ := job["job_number"].(string)
	if id == "" || !strings.HasPrefix(jn, "RJ-") || job["status"] != "open" || job["store_id"] != sid {
		t.Fatalf("create result: %v", job)
	}
	// vehicle details and its customer are copied onto the job
	if job["vehicle_number"] != plate || job["brand"] != "Nissan" || job["model"] != "Patrol" || job["customer_id"] != fx.CustomerA1.Hex() {
		t.Fatalf("vehicle fields not copied: %v", job)
	}

	v := callHandler(t, ViewRepairJob, "GET", "/v1/repair-job/"+id+q, tok, nil, "id", id)
	if !v.Status || v.resultMap(t)["title"] != title || v.resultMap(t)["job_number"] != jn {
		t.Fatalf("view: %s", v.Raw)
	}

	lr := callHandler(t, ListRepairJob, "GET", "/v1/repair-job"+q+"&search[vehicle_id]="+vehicleID, tok, nil)
	var list []map[string]interface{}
	if err := json.Unmarshal(lr.Result, &list); err != nil || !lr.Status || len(list) != 1 || list[0]["id"] != id {
		t.Fatalf("list by vehicle: %s", lr.Raw)
	}

	ur := callHandler(t, UpdateRepairJob, "PUT", "/v1/repair-job/"+id+q, tok, map[string]interface{}{
		"status": "in_progress", "work_done": "pads replaced",
	}, "id", id)
	if !ur.Status {
		t.Fatalf("update: %d %s", ur.Code, ur.Raw)
	}
	oid, _ := primitive.ObjectIDFromHex(id)
	d := gcFindOne(t, fx.StoreA, "repair_job", bson.M{"_id": oid})
	if d == nil || d["status"] != "in_progress" || d["work_done"] != "pads replaced" || d["title"] != title || d["job_number"] != jn {
		t.Fatalf("update not persisted: %v", d)
	}
}
