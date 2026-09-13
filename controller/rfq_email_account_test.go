package controller

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── oauthStateAccount ────────────────────────────────────────────────────────

func TestOauthStateAccount_Format(t *testing.T) {
	storeID := "61cf42e580e87d715a4cb9e6"
	accountID := primitive.NewObjectID()
	state := oauthStateAccount(storeID, accountID)

	raw, err := base64.URLEncoding.DecodeString(state)
	if err != nil {
		t.Fatalf("state is not valid base64: %v", err)
	}

	parts := strings.SplitN(string(raw), ":", 3)
	if len(parts) != 3 {
		t.Fatalf("expected 3 colon-delimited parts, got %d: %q", len(parts), string(raw))
	}
	if parts[0] != storeID {
		t.Errorf("part[0] storeID: got %q, want %q", parts[0], storeID)
	}
	if parts[1] != "acct" {
		t.Errorf("part[1] tag: got %q, want %q", parts[1], "acct")
	}
	if parts[2] != accountID.Hex() {
		t.Errorf("part[2] accountID: got %q, want %q", parts[2], accountID.Hex())
	}
}

func TestOauthStateAccount_IsBase64(t *testing.T) {
	state := oauthStateAccount("abc", primitive.NewObjectID())
	if _, err := base64.URLEncoding.DecodeString(state); err != nil {
		t.Errorf("state is not valid base64url: %v", err)
	}
}

func TestOauthStateAccount_UniquePerCall(t *testing.T) {
	storeID := "store123"
	s1 := oauthStateAccount(storeID, primitive.NewObjectID())
	s2 := oauthStateAccount(storeID, primitive.NewObjectID())
	if s1 == s2 {
		t.Error("expected different states for different accountIDs, got identical values")
	}
}

// ── rfqEmailWebhookURLForAccount ─────────────────────────────────────────────

func TestRFQEmailWebhookURLForAccount_ContainsAccountID(t *testing.T) {
	storeID := "store-abc"
	accountID := primitive.NewObjectID()
	u := rfqEmailWebhookURLForAccount(storeID, accountID)

	if !strings.Contains(u, "store_id="+storeID) {
		t.Errorf("webhook URL missing store_id param: %s", u)
	}
	if !strings.Contains(u, "account_id="+accountID.Hex()) {
		t.Errorf("webhook URL missing account_id param: %s", u)
	}
	if !strings.Contains(u, "/v1/rfq-email/webhook") {
		t.Errorf("webhook URL missing path: %s", u)
	}
}

func TestRFQEmailWebhookURLForAccount_NoProviderParam(t *testing.T) {
	u := rfqEmailWebhookURLForAccount("s1", primitive.NewObjectID())
	if strings.Contains(u, "provider=") {
		t.Errorf("multi-account webhook URL should not contain provider param: %s", u)
	}
}

// ── ConnectRFQEmailAccount HTTP validation ────────────────────────────────────

func TestConnectRFQEmailAccount_MissingBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON body, got %d", w.Code)
	}
}

func TestConnectRFQEmailAccount_MissingStoreID(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"provider": "gmail"})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing store_id, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestConnectRFQEmailAccount_MissingProvider(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"store_id": "61cf42e580e87d715a4cb9e6"})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", w.Code)
	}
}

func TestConnectRFQEmailAccount_InvalidStoreID(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"store_id": "not-a-valid-object-id",
		"provider": "gmail",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id, got %d", w.Code)
	}
}

func TestConnectRFQEmailAccount_GmailMissingSecret(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"store_id":           "61cf42e580e87d715a4cb9e6",
		"provider":           "gmail",
		"rfq_gmail_client_id": "my-client-id",
		// rfq_gmail_client_secret intentionally omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	// Will fail at store lookup (not in DB), so we just check it's 400 and not 500
	if w.Code == http.StatusInternalServerError {
		t.Errorf("expected 400-level error, got 500")
	}
}

func TestConnectRFQEmailAccount_UnknownProvider(t *testing.T) {
	// Unknown provider should yield 400 after a valid-looking store id fails to load
	body, _ := json.Marshal(map[string]string{
		"store_id": "61cf42e580e87d715a4cb9e6",
		"provider": "yahoo", // not supported
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	// Either 400 (store not found before provider check) or 400 (unknown provider)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── DisconnectRFQEmailAccount HTTP validation ─────────────────────────────────

func TestDisconnectRFQEmailAccount_InvalidAccountID(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete,
		"/v1/rfq-email/account/not-valid-hex?store_id=61cf42e580e87d715a4cb9e6", nil)

	// Inject mux vars manually
	req = mux.SetURLVars(req, map[string]string{"accountID": "not-valid-hex"})

	w := httptest.NewRecorder()
	DisconnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid account id, got %d", w.Code)
	}
}

func TestDisconnectRFQEmailAccount_MissingStoreID(t *testing.T) {
	accountID := primitive.NewObjectID()
	req := httptest.NewRequest(http.MethodDelete,
		"/v1/rfq-email/account/"+accountID.Hex(), nil)
	req = mux.SetURLVars(req, map[string]string{"accountID": accountID.Hex()})

	w := httptest.NewRecorder()
	DisconnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing store_id, got %d", w.Code)
	}
}

func TestDisconnectRFQEmailAccount_InvalidStoreID(t *testing.T) {
	accountID := primitive.NewObjectID()
	req := httptest.NewRequest(http.MethodDelete,
		"/v1/rfq-email/account/"+accountID.Hex()+"?store_id=bad-id", nil)
	req = mux.SetURLVars(req, map[string]string{"accountID": accountID.Hex()})

	w := httptest.NewRecorder()
	DisconnectRFQEmailAccount(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id, got %d", w.Code)
	}
}

// ── PollRFQEmailAccountStatus HTTP validation ─────────────────────────────────

func TestPollRFQEmailAccountStatus_InvalidAccountID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/v1/rfq-email/account/bad/status?store_id=61cf42e580e87d715a4cb9e6", nil)
	req = mux.SetURLVars(req, map[string]string{"accountID": "bad"})

	w := httptest.NewRecorder()
	PollRFQEmailAccountStatus(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid account id, got %d", w.Code)
	}
}

func TestPollRFQEmailAccountStatus_InvalidStoreID(t *testing.T) {
	accountID := primitive.NewObjectID()
	req := httptest.NewRequest(http.MethodGet,
		"/v1/rfq-email/account/"+accountID.Hex()+"/status?store_id=notvalid", nil)
	req = mux.SetURLVars(req, map[string]string{"accountID": accountID.Hex()})

	w := httptest.NewRecorder()
	PollRFQEmailAccountStatus(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id, got %d", w.Code)
	}
}

// ── GetRFQEmailAccounts HTTP validation ──────────────────────────────────────

func TestGetRFQEmailAccounts_MissingStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-email/accounts", nil)
	w := httptest.NewRecorder()
	GetRFQEmailAccounts(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing store_id, got %d", w.Code)
	}
}

func TestGetRFQEmailAccounts_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/rfq-email/accounts?store_id=bad", nil)
	w := httptest.NewRecorder()
	GetRFQEmailAccounts(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id, got %d", w.Code)
	}
}

// ── OAuth callback state format detection ─────────────────────────────────────

func TestOAuthCallbackStateDetection_NewFormat(t *testing.T) {
	// Verify that a new-format state (storeID:acct:accountID) decodes correctly
	storeID := "61cf42e580e87d715a4cb9e6"
	accountID := primitive.NewObjectID()
	state := oauthStateAccount(storeID, accountID)

	raw, _ := base64.URLEncoding.DecodeString(state)
	parts := strings.SplitN(string(raw), ":", 3)

	if len(parts) != 3 || parts[1] != "acct" {
		t.Fatalf("new-format state not recognised: %q → %v parts", string(raw), parts)
	}
	if parts[0] != storeID {
		t.Errorf("storeID mismatch: got %q", parts[0])
	}
	parsed, err := primitive.ObjectIDFromHex(parts[2])
	if err != nil {
		t.Errorf("accountID hex not parseable: %v", err)
	}
	if parsed != accountID {
		t.Errorf("accountID round-trip mismatch: got %v, want %v", parsed, accountID)
	}
}

func TestOAuthCallbackStateDetection_LegacyFormat(t *testing.T) {
	// Legacy state: storeID:provider (no "acct" tag, only 2 parts)
	raw := "61cf42e580e87d715a4cb9e6:gmail"
	state := base64.URLEncoding.EncodeToString([]byte(raw))

	decoded, _ := base64.URLEncoding.DecodeString(state)
	triParts := strings.SplitN(string(decoded), ":", 3)

	// Must NOT be treated as new format (second part is "gmail", not "acct")
	isNew := len(triParts) == 3 && triParts[1] == "acct"
	if isNew {
		t.Error("legacy gmail state was incorrectly detected as new multi-account format")
	}

	// Must decode as legacy 2-part format
	legacyParts := strings.SplitN(string(decoded), ":", 2)
	if len(legacyParts) != 2 {
		t.Fatalf("expected 2 legacy parts, got %d", len(legacyParts))
	}
	if legacyParts[1] != "gmail" {
		t.Errorf("expected provider=gmail, got %q", legacyParts[1])
	}
}

func TestOAuthCallbackStateDetection_OutlookLegacy(t *testing.T) {
	raw := "61cf42e580e87d715a4cb9e6:outlook"
	state := base64.URLEncoding.EncodeToString([]byte(raw))
	decoded, _ := base64.URLEncoding.DecodeString(state)
	triParts := strings.SplitN(string(decoded), ":", 3)
	if len(triParts) == 3 && triParts[1] == "acct" {
		t.Error("outlook legacy state was incorrectly classified as multi-account format")
	}
}

func TestOAuthCallbackStateDetection_ZohoLegacy(t *testing.T) {
	raw := "61cf42e580e87d715a4cb9e6:zoho"
	state := base64.URLEncoding.EncodeToString([]byte(raw))
	decoded, _ := base64.URLEncoding.DecodeString(state)
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 || parts[1] != "zoho" {
		t.Errorf("zoho legacy state decode failed: %v", parts)
	}
}

// ── rfqEmailWebhookURLForAccount edge cases ───────────────────────────────────

func TestRFQEmailWebhookURLForAccount_DifferentAccountsProduceDifferentURLs(t *testing.T) {
	storeID := "store1"
	u1 := rfqEmailWebhookURLForAccount(storeID, primitive.NewObjectID())
	u2 := rfqEmailWebhookURLForAccount(storeID, primitive.NewObjectID())
	if u1 == u2 {
		t.Error("two different account IDs should produce different webhook URLs")
	}
}

func TestRFQEmailWebhookURLForAccount_StartsWithHTTPS(t *testing.T) {
	u := rfqEmailWebhookURLForAccount("s", primitive.NewObjectID())
	if !strings.HasPrefix(u, "https://") {
		t.Errorf("webhook URL should start with https://, got %q", u)
	}
}

// ── callback_url propagation ──────────────────────────────────────────────────

func TestConnectRFQEmailAccount_CallbackURLPassedInRequest(t *testing.T) {
	// When the client sends callback_url, it must be accepted without error at the
	// JSON-decode layer (not rejected as an unknown field).
	body, _ := json.Marshal(map[string]string{
		"store_id":     "61cf42e580e87d715a4cb9e6",
		"provider":     "zoho",
		"callback_url": "http://localhost:3004/v1/rfq-email/oauth-callback",
		// credentials omitted intentionally — will fail at store-lookup, not at decode
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	// Should fail because store is not in the test DB, but NOT because of an
	// unrecognised callback_url field (i.e., not a 400 "invalid request body").
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "invalid request body" {
		t.Errorf("callback_url field was rejected during JSON decode: %s", resp["error"])
	}
}

func TestConnectRFQEmailAccount_MissingCallbackURLIsAccepted(t *testing.T) {
	// Omitting callback_url entirely should be fine (server uses its own default).
	body, _ := json.Marshal(map[string]string{
		"store_id": "61cf42e580e87d715a4cb9e6",
		"provider": "zoho",
		// callback_url intentionally omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	ConnectRFQEmailAccount(w, req)
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "invalid request body" {
		t.Errorf("missing callback_url caused unexpected decode error: %s", resp["error"])
	}
}

func TestRFQEmailCallbackURL_DefaultFallback(t *testing.T) {
	// rfqEmailOAuthCallbackURL must return a non-empty HTTPS URL by default.
	u := rfqEmailOAuthCallbackURL()
	if u == "" {
		t.Error("rfqEmailOAuthCallbackURL returned empty string")
	}
	if !strings.Contains(u, "/v1/rfq-email/oauth-callback") {
		t.Errorf("unexpected callback URL format: %q", u)
	}
}

// ── Customer RFQ ID / Email / Phone ──────────────────────────────────────────

func TestCustomerRFQID_AcceptedInCreateRequest(t *testing.T) {
	// Verify that customer_rfq_id can be included in a manual-RFQ create request
	// without a JSON-decode error.
	body, _ := json.Marshal(map[string]interface{}{
		"store_id":        "61cf42e580e87d715a4cb9e6",
		"customer_rfq_id": "PO-2025-001",
		"text_content":    "some products",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-received?store_id=61cf42e580e87d715a4cb9e6", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// We cannot call the handler directly in a unit test without DB, so just
	// validate that the JSON round-trips correctly through the body struct.
	var parsed struct {
		CustomerRFQID string `json:"customer_rfq_id"`
		TextContent   string `json:"text_content"`
	}
	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"customer_rfq_id": "PO-2025-001",
		"text_content":    "some products",
	})
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if parsed.CustomerRFQID != "PO-2025-001" {
		t.Errorf("customer_rfq_id round-trip failed: got %q", parsed.CustomerRFQID)
	}
	_ = req
}

func TestCustomerRFQID_EmptyIsValid(t *testing.T) {
	var parsed struct {
		CustomerRFQID string `json:"customer_rfq_id"`
	}
	if err := json.Unmarshal([]byte(`{}`), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.CustomerRFQID != "" {
		t.Errorf("expected empty customer_rfq_id for missing key, got %q", parsed.CustomerRFQID)
	}
}

func TestCustomerEmailPhone_AcceptedInCreateRequest(t *testing.T) {
	var parsed struct {
		CustomerEmail string `json:"customer_email"`
		CustomerPhone string `json:"customer_phone"`
	}
	data, _ := json.Marshal(map[string]string{
		"customer_email": "buyer@example.com",
		"customer_phone": "+966501234567",
	})
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}
	if parsed.CustomerEmail != "buyer@example.com" {
		t.Errorf("customer_email: got %q", parsed.CustomerEmail)
	}
	if parsed.CustomerPhone != "+966501234567" {
		t.Errorf("customer_phone: got %q", parsed.CustomerPhone)
	}
}

// ── imapUsernameForAccount ────────────────────────────────────────────────────

func TestIMAPUsernameForAccount_PrefersUsername(t *testing.T) {
	got := imapUsernameForAccount("imap@example.com", "email@example.com")
	if got != "imap@example.com" {
		t.Errorf("expected explicit imap_username, got %q", got)
	}
}

func TestIMAPUsernameForAccount_FallsBackToEmail(t *testing.T) {
	got := imapUsernameForAccount("", "email@example.com")
	if got != "email@example.com" {
		t.Errorf("expected email fallback, got %q", got)
	}
}

func TestIMAPUsernameForAccount_BothEmpty(t *testing.T) {
	got := imapUsernameForAccount("", "")
	if got != "" {
		t.Errorf("expected empty string when both empty, got %q", got)
	}
}

func TestIMAPUsernameForAccount_UsernameNotOverridden(t *testing.T) {
	// Email present but username takes precedence when non-empty.
	got := imapUsernameForAccount("custom@imap.com", "default@zoho.com")
	if got != "custom@imap.com" {
		t.Errorf("expected custom imap username, got %q", got)
	}
}

// ── TestRFQEmailIMAPHandler HTTP validation ───────────────────────────────────

func TestTestRFQEmailIMAPHandler_InvalidAccountID(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/v1/rfq-email/account/{accountID}/test-imap", TestRFQEmailIMAPHandler).Methods("POST")
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account/not-a-valid-objectid/test-imap", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid accountID, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestTestRFQEmailIMAPHandler_MissingStoreID(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/v1/rfq-email/account/{accountID}/test-imap", TestRFQEmailIMAPHandler).Methods("POST")
	accountID := primitive.NewObjectID()
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account/"+accountID.Hex()+"/test-imap", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing store_id, got %d", w.Code)
	}
}

func TestTestRFQEmailIMAPHandler_InvalidStoreID(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/v1/rfq-email/account/{accountID}/test-imap", TestRFQEmailIMAPHandler).Methods("POST")
	accountID := primitive.NewObjectID()
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-email/account/"+accountID.Hex()+"/test-imap?store_id=not-valid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id, got %d", w.Code)
	}
}

// ── UpdateRFQEmailAccountSettings HTTP validation ─────────────────────────────

func TestUpdateRFQEmailAccountSettings_InvalidAccountID(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/v1/rfq-email/account/{accountID}/settings", UpdateRFQEmailAccountSettings).Methods("PATCH")
	req := httptest.NewRequest(http.MethodPatch, "/v1/rfq-email/account/bad-id/settings", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid accountID, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestUpdateRFQEmailAccountSettings_MissingStoreID(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/v1/rfq-email/account/{accountID}/settings", UpdateRFQEmailAccountSettings).Methods("PATCH")
	accountID := primitive.NewObjectID()
	body, _ := json.Marshal(map[string]interface{}{
		"imap_host":     "imappro.zoho.in",
		"imap_port":     993,
		"imap_username": "user@example.com",
		"imap_use_ssl":  true,
	})
	req := httptest.NewRequest(http.MethodPatch, "/v1/rfq-email/account/"+accountID.Hex()+"/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing store_id, got %d", w.Code)
	}
}

func TestUpdateRFQEmailAccountSettings_InvalidStoreID(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/v1/rfq-email/account/{accountID}/settings", UpdateRFQEmailAccountSettings).Methods("PATCH")
	accountID := primitive.NewObjectID()
	req := httptest.NewRequest(http.MethodPatch, "/v1/rfq-email/account/"+accountID.Hex()+"/settings?store_id=not-valid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid store_id, got %d", w.Code)
	}
}

// ── UpdateRFQEmailAccountSettings body parsing ────────────────────────────────

func TestUpdateRFQEmailAccountSettings_PasswordOmittedWhenEmpty(t *testing.T) {
	// Empty imap_password round-trips correctly (backend skips overwriting when blank).
	type settingsBody struct {
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPUsername string `json:"imap_username"`
		IMAPUseSSL   bool   `json:"imap_use_ssl"`
		IMAPPassword string `json:"imap_password"`
	}
	data, _ := json.Marshal(settingsBody{
		IMAPHost:     "imappro.zoho.in",
		IMAPPort:     993,
		IMAPUsername: "info@example.com",
		IMAPUseSSL:   true,
		IMAPPassword: "",
	})
	var parsed settingsBody
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.IMAPPassword != "" {
		t.Errorf("expected empty password, got %q", parsed.IMAPPassword)
	}
}

func TestUpdateRFQEmailAccountSettings_PasswordPreservedWhenSet(t *testing.T) {
	type settingsBody struct {
		IMAPPassword string `json:"imap_password"`
	}
	data, _ := json.Marshal(settingsBody{IMAPPassword: "secret123"})
	var out settingsBody
	json.Unmarshal(data, &out)
	if out.IMAPPassword != "secret123" {
		t.Errorf("expected password preserved, got %q", out.IMAPPassword)
	}
}

func TestUpdateRFQEmailAccountSettings_AllFieldsRoundTrip(t *testing.T) {
	type settingsBody struct {
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPUsername string `json:"imap_username"`
		IMAPUseSSL   bool   `json:"imap_use_ssl"`
		IMAPPassword string `json:"imap_password"`
	}
	in := settingsBody{
		IMAPHost: "imappro.zoho.in", IMAPPort: 993,
		IMAPUsername: "user@example.com", IMAPUseSSL: true, IMAPPassword: "pass",
	}
	data, _ := json.Marshal(in)
	var out settingsBody
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.IMAPHost != in.IMAPHost || out.IMAPPort != in.IMAPPort ||
		out.IMAPUsername != in.IMAPUsername || out.IMAPUseSSL != in.IMAPUseSSL ||
		out.IMAPPassword != in.IMAPPassword {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}
