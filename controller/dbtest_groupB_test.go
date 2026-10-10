package controller

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Helpers shared by the DB-backed *_Integration tests for delivery notes,
// ledger/accounts, stock transfers, users, user roles, warehouses and
// signatures. They build on the harness in dbtest_test.go.

// gbURL builds "<path>?search[store_id]=<store>&k=v..." with proper escaping.
func gbURL(path string, storeID primitive.ObjectID, kv ...string) string {
	q := url.Values{}
	if !storeID.IsZero() {
		q.Set("search[store_id]", storeID.Hex())
	}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}

// gbDateStr is the RFC3339 date string the legacy handlers expect (UTC, so
// the test does not depend on the machine's timezone).
func gbDateStr() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}

// gbStore loads a store document via the model layer.
func gbStore(t *testing.T, id primitive.ObjectID) *models.Store {
	t.Helper()
	s, err := models.FindStoreByID(&id, bson.M{})
	if err != nil {
		t.Fatalf("find store %s: %v", id.Hex(), err)
	}
	return s
}

// gbExpect asserts the HTTP code and the body status flag.
func gbExpect(t *testing.T, what string, r apiResp, code int, status bool) {
	t.Helper()
	if r.Code != code || r.Status != status {
		t.Fatalf("%s: got code=%d status=%v, want code=%d status=%v; body=%s", what, r.Code, r.Status, code, status, r.Raw)
	}
}

// gbExpectErr asserts the response is a failure that names every given key in errors.
func gbExpectErr(t *testing.T, what string, r apiResp, code int, keys ...string) {
	t.Helper()
	gbExpect(t, what, r, code, false)
	for _, k := range keys {
		if _, ok := r.Errors[k]; !ok {
			t.Errorf("%s: expected errors[%q], got %v", what, k, r.Errors)
		}
	}
}

// gbList decodes a list result.
func gbList(t *testing.T, r apiResp) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result is not a list: %s", r.Raw)
	}
	return out
}

// gbTotalCount returns the top-level total_count of a list response.
func gbTotalCount(t *testing.T, r apiResp) int64 {
	t.Helper()
	var env struct {
		TotalCount int64 `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(r.Raw), &env); err != nil {
		t.Fatalf("decode total_count: %v (%s)", err, r.Raw)
	}
	return env.TotalCount
}

// gbIDs collects the "id" field of every list row.
func gbIDs(rows []map[string]interface{}) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	for _, row := range rows {
		if id, ok := row["id"].(string); ok {
			out[id] = row
		}
	}
	return out
}

// gbStr / gbNum read loosely-typed JSON values.
func gbStr(m map[string]interface{}, k string) string {
	s, _ := m[k].(string)
	return s
}

func gbNum(m map[string]interface{}, k string) float64 {
	f, _ := m[k].(float64)
	return f
}

// gbWait polls cond until it holds or the timeout elapses (handlers update
// derived data such as stock in goroutines).
func gbWait(t *testing.T, timeout time.Duration, what string, cond func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := ""
	for {
		ok, state := cond()
		if ok {
			return
		}
		last = state
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s; last state: %s", timeout, what, last)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// gbWarehouseStocks reads product_stores.<store>.{stock,warehouse_stocks} of a product.
func gbWarehouseStocks(t *testing.T, store *models.Store, productID primitive.ObjectID) (float64, map[string]float64) {
	t.Helper()
	p, err := store.FindProductByID(&productID, bson.M{})
	if err != nil {
		t.Fatalf("find product %s: %v", productID.Hex(), err)
	}
	ps, ok := p.ProductStores[store.ID.Hex()]
	if !ok {
		t.Fatalf("product %s has no product_stores entry for store %s", productID.Hex(), store.ID.Hex())
	}
	wh := map[string]float64{}
	for k, v := range ps.WarehouseStocks {
		wh[k] = v
	}
	return ps.Stock, wh
}

// gbTinyPNGBase64 is a valid 1x1 PNG, base64-encoded.
const gbTinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// gbRemoveFileAndEmptyParents removes a file written by a handler (relative to
// the package dir) and then any parent directories that became empty, up to
// and including the top-level directory "stop".
func gbRemoveFileAndEmptyParents(path, stop string) {
	_ = os.Remove(path)
	dir := filepath.Dir(path)
	for dir != "." && dir != "/" {
		if os.Remove(dir) != nil {
			return
		}
		if dir == stop {
			return
		}
		dir = filepath.Dir(dir)
	}
}
