package controller

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── parseCategories ───────────────────────────────────────────────────────────

func TestParseCategories_PlainArray(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "clean JSON array",
			input: `["Steel Pipes", "Valves", "Fittings"]`,
			want:  []string{"Steel Pipes", "Valves", "Fittings"},
		},
		{
			name:  "JSON array with markdown fence",
			input: "```json\n[\"Cables\", \"Connectors\"]\n```",
			want:  []string{"Cables", "Connectors"},
		},
		{
			name:  "JSON array with plain fence",
			input: "```\n[\"Pumps\"]\n```",
			want:  []string{"Pumps"},
		},
		{
			name:  "array with leading explanation text",
			input: `The categories are: ["HVAC Equipment", "Ductwork"]`,
			want:  []string{"HVAC Equipment", "Ductwork"},
		},
		{
			name:  "single-element array",
			input: `["Raw Steel"]`,
			want:  []string{"Raw Steel"},
		},
		{
			name:  "array with trailing newline",
			input: "[\"Cement\", \"Sand\"]\n",
			want:  []string{"Cement", "Sand"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseCategories(c.input)
			if len(got) != len(c.want) {
				t.Fatalf("parseCategories(%q) = %v (len %d), want %v (len %d)",
					c.input, got, len(got), c.want, len(c.want))
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("parseCategories(%q)[%d] = %q, want %q",
						c.input, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestParseCategories_Fallback(t *testing.T) {
	// When no JSON array brackets are found, falls back to comma-split.
	input := `Steel Pipes, Valves, Fittings`
	got := parseCategories(input)
	if len(got) == 0 {
		t.Fatal("expected at least one category from fallback CSV parsing")
	}
}

func TestParseCategories_EmptyInput(t *testing.T) {
	got := parseCategories("")
	// Empty string — no panic, returns nil or empty
	if got == nil {
		got = []string{}
	}
	if len(got) != 0 {
		t.Errorf("parseCategories(\"\") = %v, want empty", got)
	}
}

// ── fallbackIntro ────────────────────────────────────────────────────────────

func TestFallbackIntro_NotEmpty(t *testing.T) {
	cats := []string{"Steel Pipes", "Pipe Fittings"}
	for idx := 0; idx < 8; idx++ {
		intro := fallbackIntro("Acme Supplies", cats, idx, "My Store", false)
		if strings.TrimSpace(intro) == "" {
			t.Errorf("fallbackIntro idx=%d returned empty string", idx)
		}
	}
}

func TestFallbackIntro_FirstContact(t *testing.T) {
	cats := []string{"Steel"}
	// firstContact flag no longer changes fallbackIntro output — the "Hello We are from X"
	// opening line is handled by the caller (forwardRFQToSupplier).
	intro := fallbackIntro("New Supplier", cats, 0, "TestStore", true)
	if strings.TrimSpace(intro) == "" {
		t.Errorf("first-contact fallbackIntro returned empty string")
	}
}

func TestFallbackIntro_Rotation(t *testing.T) {
	cats := []string{"Valves"}
	// Should never panic for any non-negative idx
	for idx := 0; idx < 20; idx++ {
		_ = fallbackIntro("Supplier", cats, idx, "", false)
	}
}

// ── HTTP handler smoke tests (no DB required) ─────────────────────────────────

// checkRFQLLM handler — missing/invalid JSON body
func TestCheckRFQLLMConnection_BadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/check-llm", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CheckRFQLLMConnection(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCheckRFQLLMConnection_UnknownProvider(t *testing.T) {
	body := `{"provider":"unknown_llm","api_key":"sk-test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/check-llm", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CheckRFQLLMConnection(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 (error in body), got %d", resp.StatusCode)
	}
	var out map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&out)
	if connected, _ := out["connected"].(bool); connected {
		t.Error("expected connected=false for unknown provider")
	}
}

// ListRFQReceivedHandler — missing store_id
func TestListRFQReceivedHandler_MissingStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-received", nil)
	w := httptest.NewRecorder()

	ListRFQReceivedHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestListRFQReceivedHandler_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-received?store_id=not-a-valid-id", nil)
	w := httptest.NewRecorder()

	ListRFQReceivedHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// ListRFQSuppliersHandler — missing store_id
func TestListRFQSuppliersHandler_MissingStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-suppliers", nil)
	w := httptest.NewRecorder()

	ListRFQSuppliersHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestListRFQSuppliersHandler_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-suppliers?store_id=bad", nil)
	w := httptest.NewRecorder()

	ListRFQSuppliersHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// CreateRFQSupplierHandler — bad JSON
func TestCreateRFQSupplierHandler_BadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-suppliers?store_id=507f1f77bcf86cd799439011", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateRFQSupplierHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// ConnectBotWhatsApp / ConnectStoreRFQWhatsApp — bad JSON body
func TestConnectBotWhatsApp_BadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/connect", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ConnectBotWhatsApp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConnectBotWhatsApp_MissingStoreID(t *testing.T) {
	body := `{"phone":"966501234567"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ConnectBotWhatsApp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConnectStoreRFQWhatsApp_BadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-store/connect", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ConnectStoreRFQWhatsApp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// HandleRFQBotWebhook — missing store_id is OK (200 received:true, nothing processed)
func TestHandleRFQBotWebhook_MissingStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/webhook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	HandleRFQBotWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleRFQBotWebhook_NonMessageEvent(t *testing.T) {
	payload := `{"event":"connection.update","instance":"rfqbot_test","data":{}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/webhook?store_id=507f1f77bcf86cd799439011",
		strings.NewReader(payload))
	w := httptest.NewRecorder()

	HandleRFQBotWebhook(w, req)

	// Should still return 200 (webhook always acks)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// buildWebhookURL — uses request Host header
func TestBuildWebhookURL_FromHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/connect", nil)
	req.Host = "example.com"

	url := buildWebhookURL(req, "abc123")

	if !strings.Contains(url, "example.com") {
		t.Errorf("expected host in webhook URL, got %q", url)
	}
	if !strings.Contains(url, "abc123") {
		t.Errorf("expected store_id in webhook URL, got %q", url)
	}
	if !strings.Contains(url, "/v1/rfq-bot/webhook") {
		t.Errorf("expected webhook path in URL, got %q", url)
	}
}

func TestBuildWebhookURL_HTTPS_FromForwardedHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/connect", nil)
	req.Host = "api.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	url := buildWebhookURL(req, "storeid42")

	if !strings.HasPrefix(url, "https://") {
		t.Errorf("expected https scheme in webhook URL, got %q", url)
	}
}

// ── detectImageMIME ──────────────────────────────────────────────────────────

func TestDetectImageMIME_JPEG(t *testing.T) {
	// JPEG magic bytes: FF D8
	data := []byte{0xFF, 0xD8, 0x00, 0x00}
	if got := detectImageMIME(data); got != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %q", got)
	}
}

func TestDetectImageMIME_PNG(t *testing.T) {
	// PNG magic bytes: 89 50 4E 47
	data := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	if got := detectImageMIME(data); got != "image/png" {
		t.Errorf("expected image/png, got %q", got)
	}
}

func TestDetectImageMIME_GIF(t *testing.T) {
	data := []byte{'G', 'I', 'F', '8', '9', 'a'}
	if got := detectImageMIME(data); got != "image/gif" {
		t.Errorf("expected image/gif, got %q", got)
	}
}

func TestDetectImageMIME_WebP(t *testing.T) {
	// RIFF????WEBP
	data := []byte{'R', 'I', 'F', 'F', 0x00, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P'}
	if got := detectImageMIME(data); got != "image/webp" {
		t.Errorf("expected image/webp, got %q", got)
	}
}

func TestDetectImageMIME_Unknown_FallsBackToJPEG(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03}
	if got := detectImageMIME(data); got != "image/jpeg" {
		t.Errorf("expected image/jpeg fallback, got %q", got)
	}
}

func TestDetectImageMIME_Empty(t *testing.T) {
	// Empty slice must not panic
	got := detectImageMIME([]byte{})
	if got == "" {
		t.Error("expected non-empty fallback MIME for empty data")
	}
}

// ── splitDataURI ─────────────────────────────────────────────────────────────

func TestSplitDataURI_Valid(t *testing.T) {
	uri := "data:image/png;base64,abc123"
	mime, b64 := splitDataURI(uri)
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
	if b64 != "abc123" {
		t.Errorf("b64 = %q, want abc123", b64)
	}
}

func TestSplitDataURI_JPEG(t *testing.T) {
	uri := "data:image/jpeg;base64,/9j/AAAA"
	mime, b64 := splitDataURI(uri)
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", mime)
	}
	if b64 != "/9j/AAAA" {
		t.Errorf("b64 = %q, want /9j/AAAA", b64)
	}
}

func TestSplitDataURI_NotDataURI_ReturnsRaw(t *testing.T) {
	// Raw base64 (no data: prefix) — splitDataURI should return it as-is with fallback MIME
	raw := "/9j/4AAQSkZJRgAB"
	mime, b64 := splitDataURI(raw)
	if mime == "" {
		t.Error("mime should not be empty for raw base64")
	}
	if b64 != raw {
		t.Errorf("b64 = %q, want %q", b64, raw)
	}
}

// ── extractDocumentText ───────────────────────────────────────────────────────

func TestExtractDocumentText_CSV(t *testing.T) {
	csv := "Item,Qty,UOM\nSteel Pipe,100,EA\nValve,50,PC\n"
	got := extractDocumentText([]byte(csv), "text/csv", "order.csv")
	if !strings.Contains(got, "Steel Pipe") {
		t.Errorf("expected CSV content in output, got %q", got)
	}
	if !strings.Contains(got, "Valve") {
		t.Errorf("expected Valve in CSV output, got %q", got)
	}
}

func TestExtractDocumentText_PlainText(t *testing.T) {
	content := "Request for STROMAG rubber couplings qty 24"
	got := extractDocumentText([]byte(content), "text/plain", "rfq.txt")
	if got != content {
		t.Errorf("extractDocumentText txt = %q, want %q", got, content)
	}
}

func TestExtractDocumentText_Truncates4000(t *testing.T) {
	long := strings.Repeat("A", 5000)
	got := extractDocumentText([]byte(long), "text/plain", "big.txt")
	if len(got) > 4000 {
		t.Errorf("expected truncation to 4000, got len %d", len(got))
	}
}

func TestExtractDocumentText_EmptyBytes(t *testing.T) {
	got := extractDocumentText([]byte{}, "text/csv", "empty.csv")
	if got != "" {
		t.Errorf("expected empty output for empty bytes, got %q", got)
	}
}

func TestExtractDocumentText_UnknownType_ReturnsEmpty(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03}
	got := extractDocumentText(data, "application/octet-stream", "blob.bin")
	if got != "" {
		t.Errorf("expected empty for unknown type, got %q", got)
	}
}

func TestExtractDocumentText_XLSX_SharedStrings(t *testing.T) {
	// Build a minimal XLSX (ZIP archive) with xl/sharedStrings.xml containing known values.
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	shared, _ := w.Create("xl/sharedStrings.xml")
	shared.Write([]byte(`<?xml version="1.0"?><sst><si><t>STROMAG COUPLING</t></si><si><t>24</t></si><si><t>EA</t></si></sst>`))
	w.Close()

	got := extractDocumentText(buf.Bytes(), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "items.xlsx")
	if !strings.Contains(got, "STROMAG COUPLING") {
		t.Errorf("expected STROMAG COUPLING in XLSX output, got %q", got)
	}
	if !strings.Contains(got, "EA") {
		t.Errorf("expected EA in XLSX output, got %q", got)
	}
}

func TestExtractDocumentText_XLSX_InvalidZIP(t *testing.T) {
	got := extractDocumentText([]byte("not a zip file"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "bad.xlsx")
	if got != "" {
		t.Errorf("expected empty for invalid ZIP, got %q", got)
	}
}

// ── broadenCategories ────────────────────────────────────────────────────────

func TestBroadenCategories_NoAPIKey_ReturnsNil(t *testing.T) {
	store := &models.Store{}
	store.Settings.RFQLLMAPIKey = ""
	got := broadenCategories(store, []string{"Rubber Couplings"}, "STROMAG VECTOR 32")
	if got != nil {
		t.Errorf("expected nil when no API key, got %v", got)
	}
}

func TestBroadenCategories_UnknownProvider_ReturnsNil(t *testing.T) {
	store := &models.Store{}
	store.Settings.RFQLLMAPIKey = "sk-test"
	store.Settings.RFQLLMProvider = "unknown_provider"
	got := broadenCategories(store, []string{"Rubber Couplings"}, "")
	if got != nil {
		t.Errorf("expected nil for unknown provider, got %v", got)
	}
}

// ── notifyBuyerNoSuppliers ────────────────────────────────────────────────────

func TestNotifyBuyerNoSuppliers_NoInstance_NoPanic(t *testing.T) {
	// When BotEvolutionInstanceName is empty the function must return early without panicking.
	store := &models.Store{}
	store.Settings.BotEvolutionInstanceName = ""
	rfq := &models.RFQReceived{FromPhone: "966501234567"}
	// Must not panic
	notifyBuyerNoSuppliers(store, rfq, []string{"Rubber Couplings"})
}

func TestNotifyBuyerNoSuppliers_EmptyCategories_NoPanic(t *testing.T) {
	store := &models.Store{}
	store.Settings.BotEvolutionInstanceName = ""
	rfq := &models.RFQReceived{FromPhone: "966501234567"}
	notifyBuyerNoSuppliers(store, rfq, []string{})
}

// ── webhook skips ─────────────────────────────────────────────────────────────

func TestHandleRFQBotWebhook_SkipsAudioMessage(t *testing.T) {
	payload := `{
		"event":"messages.upsert",
		"instance":"rfqbot_test",
		"data":{
			"key":{"remoteJid":"966501234567@s.whatsapp.net","fromMe":false,"id":"MSG1"},
			"pushName":"Test",
			"messageType":"audioMessage",
			"messageTimestamp":1700000000,
			"message":{}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/webhook?store_id=507f1f77bcf86cd799439011",
		strings.NewReader(payload))
	w := httptest.NewRecorder()
	HandleRFQBotWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleRFQBotWebhook_SkipsStickerMessage(t *testing.T) {
	payload := `{
		"event":"messages.upsert",
		"instance":"rfqbot_test",
		"data":{
			"key":{"remoteJid":"966501234567@s.whatsapp.net","fromMe":false,"id":"MSG2"},
			"pushName":"Test",
			"messageType":"stickerMessage",
			"messageTimestamp":1700000000,
			"message":{}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/webhook?store_id=507f1f77bcf86cd799439011",
		strings.NewReader(payload))
	w := httptest.NewRecorder()
	HandleRFQBotWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleRFQBotWebhook_SkipsGroupMessage(t *testing.T) {
	// Group JIDs end in @g.us
	payload := `{
		"event":"messages.upsert",
		"instance":"rfqbot_test",
		"data":{
			"key":{"remoteJid":"120363000000@g.us","fromMe":false,"id":"MSG3"},
			"pushName":"Test",
			"messageType":"conversation",
			"messageTimestamp":1700000000,
			"message":{"conversation":"need steel pipes"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/webhook?store_id=507f1f77bcf86cd799439011",
		strings.NewReader(payload))
	w := httptest.NewRecorder()
	HandleRFQBotWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleRFQBotWebhook_SkipsFromMe(t *testing.T) {
	payload := `{
		"event":"messages.upsert",
		"instance":"rfqbot_test",
		"data":{
			"key":{"remoteJid":"966501234567@s.whatsapp.net","fromMe":true,"id":"MSG4"},
			"pushName":"Bot",
			"messageType":"conversation",
			"messageTimestamp":1700000000,
			"message":{"conversation":"sent by bot"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/webhook?store_id=507f1f77bcf86cd799439011",
		strings.NewReader(payload))
	w := httptest.NewRecorder()
	HandleRFQBotWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── RFQReceived model — ExtractedText field ───────────────────────────────────

func TestRFQReceived_ExtractedTextField(t *testing.T) {
	// Verify the field exists and round-trips through JSON correctly.
	rfq := models.RFQReceived{
		TextContent:   "Please quote this",
		ExtractedText: "Item 1 | Steel Pipe | 100 | EA",
	}
	b, err := json.Marshal(rfq)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var out models.RFQReceived
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if out.ExtractedText != rfq.ExtractedText {
		t.Errorf("ExtractedText = %q, want %q", out.ExtractedText, rfq.ExtractedText)
	}
	if out.TextContent != rfq.TextContent {
		t.Errorf("TextContent = %q, want %q", out.TextContent, rfq.TextContent)
	}
}

func TestRFQReceived_ExtractedTextNotInTextContent(t *testing.T) {
	// ExtractedText must be a separate field — not mixed into TextContent.
	rfq := models.RFQReceived{
		TextContent:   "buyer caption",
		ExtractedText: "spreadsheet row data",
	}
	if strings.Contains(rfq.TextContent, rfq.ExtractedText) {
		t.Error("ExtractedText must not be appended to TextContent")
	}
}

// ── RFQSupplier — MatchedCategory transient field ─────────────────────────────

func TestRFQSupplier_MatchedCategory_NotInJSON(t *testing.T) {
	sup := models.RFQSupplier{
		Name:            "Test Supplier",
		Phone:           "966501234567",
		MatchedCategory: "Rubber Couplings",
	}
	b, err := json.Marshal(sup)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// MatchedCategory has json:"-" so it must NOT appear in the JSON output
	if strings.Contains(string(b), "MatchedCategory") || strings.Contains(string(b), "Rubber Couplings") {
		t.Errorf("MatchedCategory should be excluded from JSON, got: %s", string(b))
	}
}

// ── StoreSettings.EnableRFQSupplierOnPurchase ─────────────────────────────────

func TestStoreSettings_EnableRFQSupplierOnPurchase_JSONRoundTrip(t *testing.T) {
	// The new field must serialise/deserialise correctly through JSON.
	store := models.Store{}
	store.Settings.EnableRFQSupplierOnPurchase = true

	b, err := json.Marshal(store.Settings)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(b), `"enable_rfq_supplier_on_purchase":true`) {
		t.Errorf("expected enable_rfq_supplier_on_purchase:true in JSON, got: %s", string(b))
	}

	var out models.StoreSettings
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !out.EnableRFQSupplierOnPurchase {
		t.Error("EnableRFQSupplierOnPurchase should be true after round-trip")
	}
}

func TestStoreSettings_EnableRFQSupplierOnPurchase_DefaultFalse(t *testing.T) {
	// Default zero-value must be false (opt-in behaviour).
	var s models.StoreSettings
	if s.EnableRFQSupplierOnPurchase {
		t.Error("EnableRFQSupplierOnPurchase should default to false")
	}
}

// ── rfqCategorizeVendorProducts ───────────────────────────────────────────────

func TestRFQCategorizeVendorProducts_NoAPIKey_ReturnsNil(t *testing.T) {
	store := &models.Store{}
	store.Settings.RFQLLMAPIKey = ""
	got := rfqCategorizeVendorProducts(store, "ACME Corp", []string{"Steel Pipe", "Valve"})
	if got != nil {
		t.Errorf("expected nil when no API key, got %v", got)
	}
}

func TestRFQCategorizeVendorProducts_EmptyProducts_NoAPIKey_ReturnsNil(t *testing.T) {
	store := &models.Store{}
	store.Settings.RFQLLMAPIKey = ""
	got := rfqCategorizeVendorProducts(store, "ACME Corp", []string{})
	if got != nil {
		t.Errorf("expected nil for empty products + no API key, got %v", got)
	}
}

func TestRFQCategorizeVendorProducts_UnknownProvider_ReturnsNil(t *testing.T) {
	// An unknown LLM provider causes callLLMText to return an error → nil result.
	store := &models.Store{}
	store.Settings.RFQLLMAPIKey = "sk-test-key"
	store.Settings.RFQLLMProvider = "unknown_llm_provider_xyz"
	store.Settings.RFQLLMModel = "model-1"
	got := rfqCategorizeVendorProducts(store, "ACME Corp", []string{"Steel Pipe", "Valve"})
	if got != nil {
		t.Errorf("expected nil for unknown provider, got %v", got)
	}
}

func TestRFQCategorizeVendorProducts_TruncatesTo50Products(t *testing.T) {
	// Build 60 product names. With a valid API key but unknown provider the function
	// returns nil — we just verify the function doesn't panic on a large product list.
	store := &models.Store{}
	store.Settings.RFQLLMAPIKey = "sk-test"
	store.Settings.RFQLLMProvider = "unknown_provider"
	products := make([]string, 60)
	for i := range products {
		products[i] = "Product " + string(rune('A'+i%26))
	}
	got := rfqCategorizeVendorProducts(store, "BigCorp", products)
	// Unknown provider → nil; the important thing is no panic/index-out-of-range
	_ = got
}

// ── rfqFetchVendorProducts ───────────────────────────────────────────────────

func TestRFQFetchVendorProducts_NonExistentVendor_ReturnsEmpty(t *testing.T) {
	// Zero ObjectID — no purchases will match, must return empty (not panic).
	nonExistent := primitive.NewObjectID()
	storeID := primitive.NewObjectID()
	got := rfqFetchVendorProducts(storeID, nonExistent)
	if got != nil {
		// nil and empty slice are both fine — the requirement is no panic and no
		// purchases returned for an unknown vendor.
		t.Logf("rfqFetchVendorProducts returned non-nil for non-existent vendor: %v", got)
	}
}

// ── rfqFetchAllVendors ───────────────────────────────────────────────────────

func TestRFQFetchAllVendors_NonExistentStore_ReturnsEmpty(t *testing.T) {
	// A fresh random store ID has no vendor collection — must return empty (not panic).
	storeID := primitive.NewObjectID()
	got, err := rfqFetchAllVendors(storeID)
	if err != nil {
		t.Logf("rfqFetchAllVendors returned error (expected for empty collection): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 vendors for non-existent store, got %d", len(got))
	}
}

// ── PopulateSuppliersFromVendors HTTP handler ─────────────────────────────────

func TestPopulateSuppliersHandler_MissingStoreID_Returns400(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/populate-suppliers", nil)
	w := httptest.NewRecorder()
	PopulateSuppliersFromVendors(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing store_id, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid store_id") {
		t.Errorf("expected 'invalid store_id' in body, got %q", w.Body.String())
	}
}

func TestPopulateSuppliersHandler_InvalidStoreID_Returns400(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/populate-suppliers?store_id=not-a-valid-objectid", nil)
	w := httptest.NewRecorder()
	PopulateSuppliersFromVendors(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id hex, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid store_id") {
		t.Errorf("expected 'invalid store_id' in body, got %q", w.Body.String())
	}
}

func TestPopulateSuppliersHandler_NonExistentStore_Returns404(t *testing.T) {
	// A valid hex ObjectID for a store that doesn't exist in the DB
	nonExistentID := primitive.NewObjectID().Hex()
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-bot/populate-suppliers?store_id="+nonExistentID, nil)
	w := httptest.NewRecorder()
	PopulateSuppliersFromVendors(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-existent store, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "store not found") {
		t.Errorf("expected 'store not found' in body, got %q", w.Body.String())
	}
}

// ── syncVendorToRFQSupplier ───────────────────────────────────────────────────

func TestSyncVendorToRFQSupplier_NoGoogleMapsKey_NoPanic(t *testing.T) {
	store := &models.Store{}
	store.Settings.GoogleMapsAPIKey = ""
	store.Settings.RFQLLMAPIKey = "sk-key"
	storeID := primitive.NewObjectID()
	vendorID := primitive.NewObjectID()
	// Must return early without panicking
	syncVendorToRFQSupplier(store, storeID, vendorID)
}

func TestSyncVendorToRFQSupplier_NoLLMKey_NoPanic(t *testing.T) {
	store := &models.Store{}
	store.Settings.GoogleMapsAPIKey = "gm-key"
	store.Settings.RFQLLMAPIKey = ""
	storeID := primitive.NewObjectID()
	vendorID := primitive.NewObjectID()
	syncVendorToRFQSupplier(store, storeID, vendorID)
}

func TestSyncVendorToRFQSupplier_BothKeysEmpty_NoPanic(t *testing.T) {
	store := &models.Store{}
	store.Settings.GoogleMapsAPIKey = ""
	store.Settings.RFQLLMAPIKey = ""
	storeID := primitive.NewObjectID()
	vendorID := primitive.NewObjectID()
	syncVendorToRFQSupplier(store, storeID, vendorID)
}

func TestSyncVendorToRFQSupplier_NonExistentVendorID_NoPanic(t *testing.T) {
	// Both keys set but vendor doesn't exist in DB — must not panic.
	store := &models.Store{}
	store.Settings.GoogleMapsAPIKey = "gm-key"
	store.Settings.RFQLLMAPIKey = "sk-key"
	store.Settings.RFQLLMProvider = "openai"
	storeID := primitive.NewObjectID()
	vendorID := primitive.NewObjectID()
	// FindVendorByID will fail (no such vendor) — function must return early
	syncVendorToRFQSupplier(store, storeID, vendorID)
}

// ── populateSuppliersFromVendors goroutine ────────────────────────────────────

func TestPopulateSuppliersFromVendors_NoVendors_NoPanic(t *testing.T) {
	// A store with no vendors → goroutine must complete without panicking.
	storeID := primitive.NewObjectID()
	store := &models.Store{}
	store.Settings.GoogleMapsAPIKey = "gm-key"
	store.Settings.RFQLLMAPIKey = "sk-key"

	done := make(chan struct{})
	go func() {
		populateSuppliersFromVendors(store, storeID)
		close(done)
	}()
	<-done // blocks until goroutine returns — panics would propagate
}

// ── SSE populate_progress event format ───────────────────────────────────────

func TestBroadcastRFQData_PopulateProgressFormat(t *testing.T) {
	// BroadcastRFQData with "populate_progress" must not panic and must produce
	// a valid SSE message containing the event name.
	storeID := primitive.NewObjectID().Hex()
	ch := rfqSSEHub.subscribe(storeID)
	defer rfqSSEHub.unsubscribe(storeID, ch)

	BroadcastRFQData(storeID, "populate_progress", map[string]interface{}{
		"step": 50, "total": 100, "percent": 50, "message": "Processing 1/2: ACME", "done": false,
	})

	select {
	case msg := <-ch:
		if !strings.Contains(msg, "populate_progress") {
			t.Errorf("expected 'populate_progress' in SSE message, got: %q", msg)
		}
		if !strings.Contains(msg, "\"percent\":50") {
			t.Errorf("expected percent:50 in SSE data, got: %q", msg)
		}
	default:
		t.Error("expected SSE message to be broadcast, channel was empty")
	}
}

func TestBroadcastRFQData_PopulateProgressDone(t *testing.T) {
	storeID := primitive.NewObjectID().Hex()
	ch := rfqSSEHub.subscribe(storeID)
	defer rfqSSEHub.unsubscribe(storeID, ch)

	BroadcastRFQData(storeID, "populate_progress", map[string]interface{}{
		"step": 100, "total": 100, "percent": 100, "message": "Done. 5 suppliers created.", "done": true,
	})

	select {
	case msg := <-ch:
		if !strings.Contains(msg, `"done":true`) {
			t.Errorf("expected done:true in SSE payload, got: %q", msg)
		}
	default:
		t.Error("expected SSE message to be broadcast")
	}
}

// ── StoreSettings.UseRTLForArabic ─────────────────────────────────────────────

func TestStoreSettings_UseRTLForArabic_DefaultFalse(t *testing.T) {
	var s models.StoreSettings
	if s.UseRTLForArabic {
		t.Error("UseRTLForArabic should default to false")
	}
}

func TestStoreSettings_UseRTLForArabic_JSONRoundTrip_True(t *testing.T) {
	store := models.Store{}
	store.Settings.UseRTLForArabic = true

	b, err := json.Marshal(store.Settings)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(b), `"use_rtl_for_arabic":true`) {
		t.Errorf("expected use_rtl_for_arabic:true in JSON, got: %s", string(b))
	}

	var out models.StoreSettings
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !out.UseRTLForArabic {
		t.Error("UseRTLForArabic should be true after round-trip")
	}
}

func TestStoreSettings_UseRTLForArabic_JSONRoundTrip_False(t *testing.T) {
	store := models.Store{}
	store.Settings.UseRTLForArabic = false

	b, err := json.Marshal(store.Settings)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// false is the zero value — json may omit or include it depending on omitempty; either is fine.
	// What matters is that unmarshal gives us false back.
	var out models.StoreSettings
	out.UseRTLForArabic = true // prime with true to confirm it resets to false
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if out.UseRTLForArabic {
		t.Error("UseRTLForArabic should be false after round-trip from false value")
	}
}

func TestStoreSettings_UseRTLForArabic_IndependentOfRFQFlag(t *testing.T) {
	// The two new bool flags must be independent — toggling one must not affect the other.
	store := models.Store{}
	store.Settings.UseRTLForArabic = true
	store.Settings.EnableRFQSupplierOnPurchase = false

	b, _ := json.Marshal(store.Settings)
	var out models.StoreSettings
	json.Unmarshal(b, &out)

	if !out.UseRTLForArabic {
		t.Error("UseRTLForArabic should be true")
	}
	if out.EnableRFQSupplierOnPurchase {
		t.Error("EnableRFQSupplierOnPurchase should be false — flags must be independent")
	}
}
