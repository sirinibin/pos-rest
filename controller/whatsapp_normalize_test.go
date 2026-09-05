package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── evoNormalizeQR ────────────────────────────────────────────────────────────

// helper: parse normalised output into a map
func parseNormalized(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("evoNormalizeQR returned non-JSON: %s", raw)
	}
	return m
}

func TestEvoNormalizeQR_FlatFormat(t *testing.T) {
	// v2.2.x Evolution API response: flat top-level fields
	input := []byte(`{"code":"2@test123","count":1,"base64":"data:image/png;base64,abc123"}`)

	out := evoNormalizeQR(input)
	m := parseNormalized(t, out)

	if m["base64"] != "data:image/png;base64,abc123" {
		t.Errorf("base64 = %q, want data:image/png;base64,abc123", m["base64"])
	}
	if m["code"] != "2@test123" {
		t.Errorf("code = %q, want 2@test123", m["code"])
	}
	if int(m["count"].(float64)) != 1 {
		t.Errorf("count = %v, want 1", m["count"])
	}
}

func TestEvoNormalizeQR_NestedFormat(t *testing.T) {
	// v2.3.x Evolution API response: nested under instance.qrcode
	input := []byte(`{
		"instance": {
			"instanceName": "rfqbot_lgk",
			"qrcode": {
				"code": "2@nested",
				"base64": "data:image/png;base64,nested_img",
				"count": 3
			}
		}
	}`)

	out := evoNormalizeQR(input)
	m := parseNormalized(t, out)

	if m["base64"] != "data:image/png;base64,nested_img" {
		t.Errorf("base64 = %q, want data:image/png;base64,nested_img", m["base64"])
	}
	if m["code"] != "2@nested" {
		t.Errorf("code = %q, want 2@nested", m["code"])
	}
	if int(m["count"].(float64)) != 3 {
		t.Errorf("count = %v, want 3", m["count"])
	}
}

func TestEvoNormalizeQR_EmptyQR_CountZero(t *testing.T) {
	// QR not ready yet — count 0, no base64
	input := []byte(`{"count":0}`)

	out := evoNormalizeQR(input)
	m := parseNormalized(t, out)

	if m["base64"] != "" {
		t.Errorf("base64 = %q, want empty string", m["base64"])
	}
	if int(m["count"].(float64)) != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

func TestEvoNormalizeQR_InvalidJSON_PassThrough(t *testing.T) {
	// Non-JSON input must come back unchanged (no panic)
	input := []byte(`not-json`)
	out := evoNormalizeQR(input)
	if string(out) != "not-json" {
		t.Errorf("evoNormalizeQR(invalid JSON) = %q, want passthrough", out)
	}
}

func TestEvoNormalizeQR_NestedEmptyQRCode(t *testing.T) {
	// Nested structure present but qrcode is an empty object — all fields empty/0
	input := []byte(`{"instance":{"instanceName":"x","qrcode":{}}}`)

	out := evoNormalizeQR(input)
	m := parseNormalized(t, out)

	if m["base64"] != "" {
		t.Errorf("base64 = %q, want empty", m["base64"])
	}
	if int(m["count"].(float64)) != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

func TestEvoNormalizeQR_FlatWithExtraFields(t *testing.T) {
	// Extra fields like pairingCode must be ignored; base64/count/code preserved
	input := []byte(`{"pairingCode":"1234-5678","code":"2@abc","count":2,"base64":"data:image/png;base64,xyz"}`)

	out := evoNormalizeQR(input)
	m := parseNormalized(t, out)

	if m["base64"] != "data:image/png;base64,xyz" {
		t.Errorf("base64 = %q, want data:image/png;base64,xyz", m["base64"])
	}
	if _, hasPairing := m["pairingCode"]; hasPairing {
		t.Errorf("pairingCode should not appear in normalized output")
	}
}

func TestEvoNormalizeQR_OutputAlwaysHasThreeFields(t *testing.T) {
	// Even an empty object must produce all three normalised keys
	input := []byte(`{}`)

	out := evoNormalizeQR(input)
	m := parseNormalized(t, out)

	for _, key := range []string{"base64", "count", "code"} {
		if _, ok := m[key]; !ok {
			t.Errorf("normalised output missing key %q", key)
		}
	}
}

// ── GetBotWhatsAppStatus — handler smoke tests (no DB / Evolution API needed) ─

func TestGetBotWhatsAppStatus_NoStoreID(t *testing.T) {
	// Empty store_id → rfqEvoConfig returns instanceName="" → early {"connected":false}
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-bot/status", nil)
	w := httptest.NewRecorder()

	GetBotWhatsAppStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var out map[string]interface{}
	json.NewDecoder(w.Body).Decode(&out)
	if connected, _ := out["connected"].(bool); connected {
		t.Error("expected connected=false when store_id is missing")
	}
}

func TestGetBotWhatsAppStatus_InvalidStoreID(t *testing.T) {
	// Non-hex store_id → ObjectIDFromHex fails → instanceName="" → early {"connected":false}
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-bot/status?store_id=not-a-valid-id", nil)
	w := httptest.NewRecorder()

	GetBotWhatsAppStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var out map[string]interface{}
	json.NewDecoder(w.Body).Decode(&out)
	if connected, _ := out["connected"].(bool); connected {
		t.Error("expected connected=false for invalid store_id")
	}
}

// ── GetStoreRFQWhatsAppStatus — same pattern ──────────────────────────────────

func TestGetStoreRFQWhatsAppStatus_NoStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-store/status", nil)
	w := httptest.NewRecorder()

	GetStoreRFQWhatsAppStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var out map[string]interface{}
	json.NewDecoder(w.Body).Decode(&out)
	if connected, _ := out["connected"].(bool); connected {
		t.Error("expected connected=false when store_id is missing")
	}
}

func TestGetStoreRFQWhatsAppStatus_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-store/status?store_id=bad", nil)
	w := httptest.NewRecorder()

	GetStoreRFQWhatsAppStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var out map[string]interface{}
	json.NewDecoder(w.Body).Decode(&out)
	if connected, _ := out["connected"].(bool); connected {
		t.Error("expected connected=false for invalid store_id")
	}
}

// ── GetWhatsAppStatus (main WhatsApp) — same pattern ─────────────────────────

func TestGetWhatsAppStatus_NoStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/whatsapp/status", nil)
	w := httptest.NewRecorder()

	GetWhatsAppStatus(w, req)

	// evoConfigFromStore("") returns evoDefaultInstance ("wa_umlj"),
	// so the handler WILL call Evolution API (localhost:8081). If it is
	// unreachable the handler returns 502; if reachable it returns 200.
	// Either way it must not panic.
	if w.Code != http.StatusOK && w.Code != http.StatusBadGateway {
		t.Errorf("expected 200 or 502, got %d", w.Code)
	}
}

func TestGetWhatsAppStatus_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/whatsapp/status?store_id=bad-id", nil)
	w := httptest.NewRecorder()

	GetWhatsAppStatus(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusBadGateway {
		t.Errorf("expected 200 or 502, got %d", w.Code)
	}
}

// ── DisconnectBotWhatsApp / DisconnectStoreRFQWhatsApp — missing store_id ─────

func TestDisconnectBotWhatsApp_MissingStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/v1/rfq-bot/disconnect", nil)
	w := httptest.NewRecorder()

	DisconnectBotWhatsApp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDisconnectBotWhatsApp_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/v1/rfq-bot/disconnect?store_id=bad", nil)
	w := httptest.NewRecorder()

	DisconnectBotWhatsApp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDisconnectStoreRFQWhatsApp_MissingStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/v1/rfq-store/disconnect", nil)
	w := httptest.NewRecorder()

	DisconnectStoreRFQWhatsApp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
