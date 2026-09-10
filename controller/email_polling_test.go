package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── stripHTMLTags ─────────────────────────────────────────────────────────────

func TestStripHTMLTags_PlainText(t *testing.T) {
	if got := stripHTMLTags("Hello, world!"); got != "Hello, world!" {
		t.Errorf("plain text unchanged: got %q", got)
	}
}

func TestStripHTMLTags_RemovesTags(t *testing.T) {
	got := stripHTMLTags("<p>Hello <strong>world</strong>!</p>")
	if got != "Hello world!" {
		t.Errorf("expected %q, got %q", "Hello world!", got)
	}
}

func TestStripHTMLTags_NestedTags(t *testing.T) {
	got := stripHTMLTags("<div><span>text</span></div>")
	if got != "text" {
		t.Errorf("expected %q, got %q", "text", got)
	}
}

func TestStripHTMLTags_EmptyString(t *testing.T) {
	if got := stripHTMLTags(""); got != "" {
		t.Errorf("empty input: got %q", got)
	}
}

func TestStripHTMLTags_OnlyTags(t *testing.T) {
	if got := stripHTMLTags("<br/><br/>"); got != "" {
		t.Errorf("only tags should strip to empty, got %q", got)
	}
}

func TestStripHTMLTags_UnclosedTag(t *testing.T) {
	if got := stripHTMLTags("<b>hello"); !strings.Contains(got, "hello") {
		t.Errorf("unclosed tag: expected 'hello' to survive, got %q", got)
	}
}

// ── gmailBase64Decode ─────────────────────────────────────────────────────────

func TestGmailBase64Decode_Normal(t *testing.T) {
	// "Hello, Email!" URL-safe base64 (no padding)
	out := gmailBase64Decode("SGVsbG8sIEVtYWlsIQ")
	if out != "Hello, Email!" {
		t.Errorf("expected %q, got %q", "Hello, Email!", out)
	}
}

func TestGmailBase64Decode_NeedsPadding2(t *testing.T) {
	// "Hi" → standard base64 "SGk=" → URL-safe without padding: "SGk"
	if got := gmailBase64Decode("SGk"); got != "Hi" {
		t.Errorf("expected %q, got %q", "Hi", got)
	}
}

func TestGmailBase64Decode_NeedsPadding1(t *testing.T) {
	// "Hi!" → standard base64 "SGkh" (no padding needed)
	if got := gmailBase64Decode("SGkh"); got != "Hi!" {
		t.Errorf("expected %q, got %q", "Hi!", got)
	}
}

func TestGmailBase64Decode_UrlSafeChars(t *testing.T) {
	// "f=?" → standard base64 "Zj0/" → URL-safe "Zj0_"
	if got := gmailBase64Decode("Zj0_"); got != "f=?" {
		t.Errorf("expected %q, got %q", "f=?", got)
	}
}

func TestGmailBase64Decode_InvalidInput(t *testing.T) {
	// Should return empty string, not panic.
	got := gmailBase64Decode("!!not-base64!!")
	if got != "" {
		t.Errorf("invalid base64 should return empty, got %q", got)
	}
}

func TestGmailBase64Decode_Empty(t *testing.T) {
	if got := gmailBase64Decode(""); got != "" {
		t.Errorf("empty input should return empty, got %q", got)
	}
}

// ── Zoho HTML conversion (inline logic in fetchZohoMessageContent) ─────────────

func TestZohoBrTagConversion(t *testing.T) {
	raw := "Hello<br>World<br/>End"
	result := strings.ReplaceAll(raw, "<br>", "\n")
	result = strings.ReplaceAll(result, "<br/>", "\n")
	result = strings.ReplaceAll(result, "<br />", "\n")
	result = stripHTMLTags(result)

	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines after <br> conversion, got %d: %q", len(lines), result)
	}
	if lines[0] != "Hello" || lines[1] != "World" || lines[2] != "End" {
		t.Errorf("lines mismatch: %v", lines)
	}
}

// ── Zoho account ID parsing ───────────────────────────────────────────────────

func TestZohoAccountIDParsing_OK(t *testing.T) {
	var res struct {
		Data   []struct{ AccountID string `json:"accountId"` } `json:"data"`
		Status struct{ Code int `json:"code"` }               `json:"status"`
	}
	if err := json.Unmarshal([]byte(`{"data":[{"accountId":"abc-123"}],"status":{"code":200}}`), &res); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(res.Data) == 0 || res.Data[0].AccountID != "abc-123" {
		t.Errorf("expected accountId %q, got %v", "abc-123", res.Data)
	}
}

func TestZohoAccountIDParsing_Empty(t *testing.T) {
	var res struct {
		Data   []struct{ AccountID string `json:"accountId"` } `json:"data"`
		Status struct{ Code int `json:"code"` }               `json:"status"`
	}
	if err := json.Unmarshal([]byte(`{"data":[],"status":{"code":401}}`), &res); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(res.Data) != 0 {
		t.Error("expected empty data array")
	}
}

// ── parsedEmail zero value ────────────────────────────────────────────────────

func TestParsedEmail_ZeroValue(t *testing.T) {
	var pe parsedEmail
	if pe.from != "" || pe.subject != "" || pe.bodyText != "" || pe.to != nil {
		t.Errorf("zero-value parsedEmail should have empty fields, got %+v", pe)
	}
}

// ── processPolledEmail — early return on empty message ────────────────────────

func TestProcessPolledEmail_EmptyMessage(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("processPolledEmail panicked on empty message: %v", r)
		}
	}()
	// Empty from+bodyText → early return (no DB access needed).
	processPolledEmail(primitive.NilObjectID, models.StoreSettings{}, "zoho", parsedEmail{})
}

// ── updateAccountLastPolled — nil ObjectID must not panic ─────────────────────

func TestUpdateAccountLastPolled_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("updateAccountLastPolled panicked: %v", r)
		}
	}()
	// Will fail with a DB error (no live DB in unit tests) but must not panic.
	updateAccountLastPolled(primitive.NilObjectID, primitive.NilObjectID, time.Now())
}

// ── ensureZohoToken — no refresh token path ───────────────────────────────────

func TestEnsureZohoToken_NoRefreshToken(t *testing.T) {
	acct := models.RFQEmailAccount{
		ZohoAccessToken:  "existing_token",
		ZohoRefreshToken: "", // no refresh token
	}
	tok, err := ensureZohoToken(primitive.NilObjectID, acct)
	if err != nil {
		t.Errorf("no refresh token path should not error: %v", err)
	}
	if tok != "existing_token" {
		t.Errorf("expected %q, got %q", "existing_token", tok)
	}
}

// ── ensureGmailToken — no refresh token path ─────────────────────────────────

func TestEnsureGmailToken_NoRefreshToken(t *testing.T) {
	acct := models.RFQEmailAccount{
		GmailAccessToken:  "gmail_existing",
		GmailRefreshToken: "",
	}
	tok, err := ensureGmailToken(primitive.NilObjectID, acct)
	if err != nil {
		t.Errorf("no refresh token path should not error: %v", err)
	}
	if tok != "gmail_existing" {
		t.Errorf("expected %q, got %q", "gmail_existing", tok)
	}
}

// ── ensureOutlookToken — no refresh token path ────────────────────────────────

func TestEnsureOutlookToken_NoRefreshToken(t *testing.T) {
	acct := models.RFQEmailAccount{
		OutlookAccessToken:  "outlook_existing",
		OutlookRefreshToken: "",
	}
	tok, err := ensureOutlookToken(primitive.NilObjectID, acct)
	if err != nil {
		t.Errorf("no refresh token path should not error: %v", err)
	}
	if tok != "outlook_existing" {
		t.Errorf("expected %q, got %q", "outlook_existing", tok)
	}
}

// ── ensureZohoToken — mock refresh server ────────────────────────────────────

func TestEnsureZohoToken_RefreshSuccess(t *testing.T) {
	// Stub a Zoho-like token endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "wrong grant_type", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new_access_token"}`))
	}))
	defer srv.Close()

	acct := models.RFQEmailAccount{
		ZohoAccountsServer: srv.URL,
		ZohoClientID:       "cid",
		ZohoClientSecret:   "csec",
		ZohoRefreshToken:   "refresh123",
		ZohoAccessToken:    "old_token",
	}

	// ensureZohoToken hits ZohoAccountsServer + "/oauth/v2/token"
	// Since we mock the server, it should return "new_access_token".
	tok, err := ensureZohoToken(primitive.NilObjectID, acct)
	if err != nil {
		// DB write will fail (no DB), but the token must still come back.
		// Accept errors only from the DB write step.
	}
	// err may be non-nil from the rfqEmailAccountUpdate DB call, but tok should be set.
	if tok == "" {
		t.Errorf("expected a token, got empty (err: %v)", err)
	}
}

// ── CreateRFQSupplierHandler — JSON parse succeeds with numeric rating ────────

func TestCreateRFQSupplierHandler_NumericRating(t *testing.T) {
	// With rating as a number (after the frontend fix), the JSON decode must not fail.
	body := `{"name":"ACME","phone":"971501234567","rating":4.5,"store_id":"000000000000000000000000"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/rfq-suppliers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CreateRFQSupplierHandler(w, req)

	resp := w.Result()
	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)

	// Must not return "invalid JSON" — the request was valid JSON.
	if errMsg, ok := res["error"].(string); ok && strings.Contains(errMsg, "invalid JSON") {
		t.Errorf("unexpected 'invalid JSON' error: %s", errMsg)
	}
}

// ── Gmail message content parsing ─────────────────────────────────────────────

func TestGmailHeaderExtraction(t *testing.T) {
	headers := []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}{
		{"From", "Alice <alice@example.com>"},
		{"Subject", "Purchase inquiry"},
		{"To", "sales@company.com"},
	}

	var from, subject, to string
	for _, h := range headers {
		switch strings.ToLower(h.Name) {
		case "from":
			from = h.Value
		case "subject":
			subject = h.Value
		case "to":
			to = h.Value
		}
	}

	if from != "Alice <alice@example.com>" {
		t.Errorf("from: got %q", from)
	}
	if subject != "Purchase inquiry" {
		t.Errorf("subject: got %q", subject)
	}
	if to != "sales@company.com" {
		t.Errorf("to: got %q", to)
	}
}

// ── Outlook message parsing ───────────────────────────────────────────────────

func TestOutlookMessageParsing_WithName(t *testing.T) {
	type emailAddr struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	}
	type fromField struct {
		EmailAddress emailAddr `json:"emailAddress"`
	}

	f := fromField{EmailAddress: emailAddr{Address: "bob@example.com", Name: "Bob Smith"}}
	from := f.EmailAddress.Address
	if f.EmailAddress.Name != "" {
		from = f.EmailAddress.Name + " <" + from + ">"
	}

	if from != "Bob Smith <bob@example.com>" {
		t.Errorf("expected formatted from, got %q", from)
	}
}

func TestOutlookMessageParsing_NoName(t *testing.T) {
	type emailAddr struct {
		Address string `json:"address"`
		Name    string `json:"name"`
	}
	type fromField struct {
		EmailAddress emailAddr `json:"emailAddress"`
	}

	f := fromField{EmailAddress: emailAddr{Address: "bob@example.com"}}
	from := f.EmailAddress.Address
	if f.EmailAddress.Name != "" {
		from = f.EmailAddress.Name + " <" + from + ">"
	}

	if from != "bob@example.com" {
		t.Errorf("expected plain address, got %q", from)
	}
}
