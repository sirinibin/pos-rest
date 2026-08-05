package utils

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiResp struct {
	Status bool              `json:"status"`
	Errors map[string]string `json:"errors"`
}

// decodeResult holds what the handler wrote so we can inspect it.
func callDecode(body string, target interface{}) (bool, *httptest.ResponseRecorder) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ok := Decode(w, r, target)
	return ok, w
}

// ── valid JSON ────────────────────────────────────────────────────────────────

func TestDecode_ValidJSON_ReturnsTrue(t *testing.T) {
	var target map[string]interface{}
	ok, w := callDecode(`{"foo":"bar"}`, &target)
	if !ok {
		t.Fatal("Decode returned false for valid JSON")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if target["foo"] != "bar" {
		t.Errorf("decoded value = %v, want bar", target["foo"])
	}
}

func TestDecode_ValidStruct_PopulatesFields(t *testing.T) {
	type Payload struct {
		Name  string  `json:"name"`
		Price float64 `json:"price"`
	}
	var p Payload
	ok, _ := callDecode(`{"name":"Widget","price":9.99}`, &p)
	if !ok {
		t.Fatal("Decode returned false for valid struct JSON")
	}
	if p.Name != "Widget" {
		t.Errorf("Name = %q, want Widget", p.Name)
	}
	if p.Price != 9.99 {
		t.Errorf("Price = %v, want 9.99", p.Price)
	}
}

// ── malformed JSON ────────────────────────────────────────────────────────────

func TestDecode_InvalidJSON_ReturnsFalse(t *testing.T) {
	var target map[string]interface{}
	ok, w := callDecode(`{not valid json}`, &target)
	if ok {
		t.Fatal("Decode returned true for invalid JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDecode_InvalidJSON_WritesErrorBody(t *testing.T) {
	var target map[string]interface{}
	_, w := callDecode(`not-json`, &target)

	var resp apiResp
	if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&resp); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if resp.Status {
		t.Error("response.status should be false")
	}
	if resp.Errors["input"] == "" {
		t.Error("errors.input should be non-empty")
	}
	if !strings.HasPrefix(resp.Errors["input"], "Invalid Data:") {
		t.Errorf("errors.input = %q, want prefix 'Invalid Data:'", resp.Errors["input"])
	}
}

func TestDecode_EmptyBody_ReturnsFalse(t *testing.T) {
	var target map[string]interface{}
	ok, w := callDecode(``, &target)
	if ok {
		t.Fatal("Decode returned true for empty body")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDecode_TruncatedJSON_ReturnsFalse(t *testing.T) {
	var target map[string]interface{}
	ok, w := callDecode(`{"foo":`, &target)
	if ok {
		t.Fatal("Decode returned true for truncated JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
