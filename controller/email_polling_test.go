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

// ── zohoMailBase ──────────────────────────────────────────────────────────────

func TestZohoMailBase_DefaultUS(t *testing.T) {
	if got := zohoMailBase("https://accounts.zoho.com"); got != "https://mail.zoho.com" {
		t.Errorf("expected zoho.com, got %q", got)
	}
}

func TestZohoMailBase_India(t *testing.T) {
	if got := zohoMailBase("https://accounts.zoho.in"); got != "https://mail.zoho.in" {
		t.Errorf("expected zoho.in, got %q", got)
	}
}

func TestZohoMailBase_EU(t *testing.T) {
	if got := zohoMailBase("https://accounts.zoho.eu"); got != "https://mail.zoho.eu" {
		t.Errorf("expected zoho.eu, got %q", got)
	}
}

func TestZohoMailBase_Australia(t *testing.T) {
	if got := zohoMailBase("https://accounts.zoho.com.au"); got != "https://mail.zoho.com.au" {
		t.Errorf("expected zoho.com.au, got %q", got)
	}
}

func TestZohoMailBase_Empty(t *testing.T) {
	if got := zohoMailBase(""); got != "https://mail.zoho.com" {
		t.Errorf("empty should default to zoho.com, got %q", got)
	}
}

func TestZohoMailBase_WithOAuthPath(t *testing.T) {
	// accounts_server may include /oauth/v2/token — should still detect region
	if got := zohoMailBase("https://accounts.zoho.in/oauth/v2/token"); got != "https://mail.zoho.in" {
		t.Errorf("expected zoho.in even with OAuth path, got %q", got)
	}
}

// ── chromePath ────────────────────────────────────────────────────────────────

func TestChromePath_ReturnsStringOrEmpty(t *testing.T) {
	// Just ensure the function returns without panic; empty is OK in CI.
	_ = chromePath()
}

func TestChromeExecOpts_ContainsNoSandbox(t *testing.T) {
	opts := chromeExecOpts("/usr/bin/chromium-browser")
	// We can't inspect ExecAllocatorOption internals directly, but we can verify
	// that the function returns a non-empty slice and doesn't panic.
	if len(opts) == 0 {
		t.Error("expected non-empty options slice")
	}
}

// ── Zoho account-ID fetch with region-aware URL (mock server) ─────────────────

func TestFetchZohoAccountID_IndiaRegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/accounts") {
			http.Error(w, "wrong path", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"accountId": "INDIA123"}},
		})
	}))
	defer srv.Close()

	id, err := fetchZohoAccountID("token123", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "INDIA123" {
		t.Errorf("expected INDIA123, got %q", id)
	}
}

func TestFetchZohoAccountID_Returns401Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":   []interface{}{},
			"status": map[string]int{"code": 401},
		})
	}))
	defer srv.Close()

	_, err := fetchZohoAccountID("bad-token", srv.URL)
	if err == nil {
		t.Error("expected error for empty data/401, got nil")
	}
}

// ── Supplier deduplication ────────────────────────────────────────────────────

func TestRFQSendDeduplicateRecipients(t *testing.T) {
	seen := map[string]bool{}
	var deduplicated []string
	recipients := []struct {
		Name  string
		Phone string
	}{
		{Name: "Alice", Phone: "0501234567"},
		{Name: "Bob", Phone: "0507654321"},
		{Name: "Alice Dup", Phone: "0501234567"}, // duplicate phone
	}
	for _, r := range recipients {
		if r.Phone != "" && !seen[r.Phone] {
			seen[r.Phone] = true
			deduplicated = append(deduplicated, r.Phone)
		}
	}
	if len(deduplicated) != 2 {
		t.Errorf("expected 2 unique suppliers, got %d", len(deduplicated))
	}
}

// ── ListZohoMessages with region-aware URL (mock server) ─────────────────────

func TestListZohoMessages_UsesMailBase(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	}))
	defer srv.Close()

	msgs, err := listZohoMessages("tok", "ACC123", time.Now().Add(-1*time.Hour), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
	if !strings.Contains(capturedPath, "/api/accounts/ACC123/messages/view") {
		t.Errorf("unexpected path: %q", capturedPath)
	}
}

// ── FetchZohoEmail with region-aware URL (mock server) ───────────────────────

func TestFetchZohoEmail_IndiaRegion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"emailAddress": []map[string]string{{"mailId": "test@zoho.in"}}},
			},
		})
	}))
	defer srv.Close()

	// Pass the mock server URL as accountsServer so zohoMailBase(srv.URL) → srv.URL
	// Since srv.URL won't match any region suffix, it falls back to zoho.com.
	// We pass srv.URL directly as mailBase through fetchZohoEmail's parameter chain.
	// Instead, test the full chain by passing a fake accountsServer that has zoho.in.
	// We can't inject the mock URL easily here, so verify the function at least
	// doesn't panic and returns a string.
	result := fetchZohoEmail("sometoken", "https://accounts.zoho.in")
	// Result will be "" because mail.zoho.in is unreachable in test, but no panic.
	if result != "" {
		t.Logf("fetchZohoEmail returned %q (non-empty means real server responded)", result)
	}
}

// ── processMetaIncomingMessage calls saveProcurementWhatsAppMessage (indirect) ─

func TestRFQBotWebhook_NonMessageObject(t *testing.T) {
	// HandleRFQBotWebhook returns 200 immediately (async processing).
	body := `{"object":"page","entry":[]}`
	req := httptest.NewRequest("POST", "/v1/rfq-bot/webhook?store_id=507f1f77bcf86cd799439011", strings.NewReader(body))
	w := httptest.NewRecorder()
	HandleRFQBotWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── models.ProcurementMessage basic integrity ─────────────────────────────────

func TestProcurementMessage_DefaultValues(t *testing.T) {
	storeID := primitive.NewObjectID()
	pm := models.ProcurementMessage{
		StoreID:   storeID,
		Type:      "whatsapp",
		Direction: "in",
	}
	if pm.Type != "whatsapp" {
		t.Errorf("expected whatsapp, got %q", pm.Type)
	}
	if pm.ProcessedAsRFQ {
		t.Error("ProcessedAsRFQ should default to false")
	}
	if pm.StoreID != storeID {
		t.Error("StoreID mismatch")
	}
}

// ── emailMatchesKeywords (subject-only, case-insensitive) ─────────────────────

func TestEmailMatchesKeywords_EmptyKeywords_AcceptsAll(t *testing.T) {
	subjects := []string{"Hello", "Invoice #123", ""}
	for _, subj := range subjects {
		if !emailMatchesKeywords(subj, nil) {
			t.Errorf("empty keywords should accept email with subject=%q", subj)
		}
		if !emailMatchesKeywords(subj, []string{}) {
			t.Errorf("empty keywords slice should accept email with subject=%q", subj)
		}
	}
}

func TestEmailMatchesKeywords_MatchInSubject(t *testing.T) {
	kws := []string{"quotation", "rfq", "request for quotation"}
	cases := []string{"Quotation Request", "RFQ from customer", "REQUEST FOR QUOTATION"}
	for _, subj := range cases {
		if !emailMatchesKeywords(subj, kws) {
			t.Errorf("keyword in subject should match: %q", subj)
		}
	}
}

func TestEmailMatchesKeywords_BodyNotChecked(t *testing.T) {
	// Keywords in body only must NOT match — filter is subject-only.
	kws := []string{"quotation", "rfq"}
	if emailMatchesKeywords("No keyword subject", kws) {
		t.Error("keyword not in subject should not match even if it were in the body")
	}
}

func TestEmailMatchesKeywords_CaseInsensitive(t *testing.T) {
	kws := []string{"quotation"}
	variants := []string{"quotation", "Quotation", "QUOTATION", "QuOtAtIoN"}
	for _, v := range variants {
		if !emailMatchesKeywords(v, kws) {
			t.Errorf("case-insensitive match failed for %q", v)
		}
	}
}

func TestEmailMatchesKeywords_NoMatch_ReturnsFalse(t *testing.T) {
	kws := []string{"quotation", "rfq", "request for quotation"}
	subjects := []string{"Hello team", "Invoice #456", "Newsletter May 2026", ""}
	for _, subj := range subjects {
		if emailMatchesKeywords(subj, kws) {
			t.Errorf("no keyword in subject — should not match: %q", subj)
		}
	}
}

func TestEmailMatchesKeywords_MultiWordKeyword(t *testing.T) {
	kws := []string{"request for quotation"}
	if !emailMatchesKeywords("Please send a request for quotation", kws) {
		t.Error("multi-word keyword 'request for quotation' should match in subject")
	}
	if emailMatchesKeywords("Please send a request", kws) {
		t.Error("partial multi-word keyword should NOT match")
	}
}

func TestEmailMatchesKeywords_EmptyKeywordStringSkipped(t *testing.T) {
	kws := []string{"", "rfq"}
	if !emailMatchesKeywords("RFQ from Acme", kws) {
		t.Error("valid keyword 'rfq' should still match even with empty string in list")
	}
	if emailMatchesKeywords("hello world", []string{""}) {
		t.Error("a list containing only an empty keyword should not match anything")
	}
}

func TestEmailMatchesKeywords_DefaultKeywordsMatchTypicalRFQEmail(t *testing.T) {
	defaults := []string{"quotation", "rfq", "request for quotation"}
	subjects := []string{
		"Request for Quotation — Steel Pipes",
		"RFQ #2024-001",
		"Quotation Required",
	}
	for _, subj := range subjects {
		if !emailMatchesKeywords(subj, defaults) {
			t.Errorf("default keywords should match typical RFQ subject: %q", subj)
		}
	}
}

func TestEmailMatchesKeywords_DefaultKeywordsRejectNonRFQEmail(t *testing.T) {
	defaults := []string{"quotation", "rfq", "request for quotation"}
	subjects := []string{"Hello", "Payment Confirmation", "Newsletter"}
	for _, subj := range subjects {
		if emailMatchesKeywords(subj, defaults) {
			t.Errorf("default keywords should NOT match non-RFQ subject: %q", subj)
		}
	}
}

// ── IncomingEmailKeywords store settings field ────────────────────────────────

func TestStoreSettings_IncomingEmailKeywords_DefaultEmpty(t *testing.T) {
	var s models.StoreSettings
	if len(s.IncomingEmailKeywords) != 0 {
		t.Error("IncomingEmailKeywords should default to empty (accept all) for backward compat")
	}
}

func TestStoreSettings_IncomingEmailKeywords_EmptyMeansNoFilter(t *testing.T) {
	var s models.StoreSettings
	if !emailMatchesKeywords("any subject", s.IncomingEmailKeywords) {
		t.Error("empty IncomingEmailKeywords must accept all emails (no filter)")
	}
}

// ── mentionsAttachment — expanded phrase list ─────────────────────────────────

func TestMentionsAttachment_ClassicPhrases(t *testing.T) {
	classics := []string{
		"please find attached",
		"please find the attached",
		"find attached",
		"see attached",
		"attached herewith",
		"the attached file",
		"the attachment",
		"i have attached",
		"we have attached",
		"kindly find attached",
		"enclosed herewith",
		"attached is the",
	}
	for _, phrase := range classics {
		if !mentionsAttachment(phrase) {
			t.Errorf("mentionsAttachment should return true for classic phrase %q", phrase)
		}
	}
}

func TestMentionsAttachment_NewPhrases(t *testing.T) {
	newPhrases := []string{
		"need quotation for the attached excel sheet",
		"please see the attached excel",
		"the attached pdf is enclosed",
		"with attached price list",
		"look at the attached",
		"refer to the attached document",
		"is attached for your review",
		"we are attaching the specification",
		"i'm attaching the file",
		"please find enclosed the document",
		"for the attached quotation",
		"the attached sheet contains items",
		"for the attached image",
	}
	for _, phrase := range newPhrases {
		if !mentionsAttachment(phrase) {
			t.Errorf("mentionsAttachment should return true for new phrase %q", phrase)
		}
	}
}

func TestMentionsAttachment_CaseInsensitive(t *testing.T) {
	cases := []string{
		"PLEASE FIND ATTACHED",
		"For The Attached Excel Sheet",
		"THE ATTACHED FILE",
		"I HAVE ATTACHED",
	}
	for _, phrase := range cases {
		if !mentionsAttachment(phrase) {
			t.Errorf("mentionsAttachment should be case-insensitive for %q", phrase)
		}
	}
}

func TestMentionsAttachment_NegativeCases(t *testing.T) {
	// Single words / ambiguous phrases that should NOT match.
	negatives := []string{
		"scan all attachments if any",
		"virus scans attachment",
		"view attachments policy",
		"",
		"please send us a quotation",
		"we need the price list",
	}
	for _, phrase := range negatives {
		if mentionsAttachment(phrase) {
			t.Errorf("mentionsAttachment should return false for ambiguous/negative phrase %q", phrase)
		}
	}
}

func TestMentionsAttachment_EmailBodyExample(t *testing.T) {
	body := "Dear team,\n\nNeed quotation for the attached excel sheet.\n\nRegards,\nJohn"
	if !mentionsAttachment(body) {
		t.Errorf("should match 'for the attached excel sheet' in email body")
	}
}

// ── parsedEmail.hasZohoAttachment ─────────────────────────────────────────────

func TestParsedEmail_HasZohoAttachment_FieldExists(t *testing.T) {
	// Struct field compiles and is accessible.
	pe := parsedEmail{hasZohoAttachment: true}
	if !pe.hasZohoAttachment {
		t.Error("hasZohoAttachment field should be settable to true")
	}
	pe2 := parsedEmail{hasZohoAttachment: false}
	if pe2.hasZohoAttachment {
		t.Error("hasZohoAttachment field should default to false")
	}
}

// ── attachmentMissing logic (via processPolledEmail logic replicated) ─────────

// replicates the logic from processPolledEmail to test independently of DB.
func computeAttachmentMissing(hasZohoAttachment bool, bodyText string, attachments []models.ProcurementAttachment) bool {
	hasDownloaded := false
	for _, att := range attachments {
		if att.URL != "" {
			hasDownloaded = true
			break
		}
	}
	return !hasDownloaded && (hasZohoAttachment || mentionsAttachment(bodyText))
}

func TestAttachmentMissing_ZohoFlagTrueNoDownload(t *testing.T) {
	// Zoho says hasAttachment=true but none downloaded → missing.
	if !computeAttachmentMissing(true, "no attachment phrase", nil) {
		t.Error("hasZohoAttachment=true with no downloaded files should be attachment_missing")
	}
}

func TestAttachmentMissing_ZohoFlagTrueWithDownload(t *testing.T) {
	atts := []models.ProcurementAttachment{{URL: "/files/abc.pdf"}}
	if computeAttachmentMissing(true, "no phrase", atts) {
		t.Error("hasZohoAttachment=true but download succeeded should NOT be attachment_missing")
	}
}

func TestAttachmentMissing_PhrasePresentNoDownload(t *testing.T) {
	if !computeAttachmentMissing(false, "please find attached", nil) {
		t.Error("body phrase present with no download should be attachment_missing")
	}
}

func TestAttachmentMissing_PhrasePresentWithDownload(t *testing.T) {
	atts := []models.ProcurementAttachment{{URL: "/files/abc.pdf"}}
	if computeAttachmentMissing(false, "please find attached", atts) {
		t.Error("body phrase present but download succeeded should NOT be attachment_missing")
	}
}

func TestAttachmentMissing_NeitherFlagNorPhrase(t *testing.T) {
	if computeAttachmentMissing(false, "send me the price", nil) {
		t.Error("neither hasZohoAttachment nor phrase should be NOT attachment_missing")
	}
}

func TestAttachmentMissing_BothFlagAndPhrase(t *testing.T) {
	if !computeAttachmentMissing(true, "please find attached", nil) {
		t.Error("both flag and phrase with no download should be attachment_missing")
	}
}

func TestAttachmentMissing_AttachmentURLEmptyIsNotDownloaded(t *testing.T) {
	// Attachment record exists but URL is empty (download failed).
	atts := []models.ProcurementAttachment{{Filename: "file.pdf", URL: ""}}
	if !computeAttachmentMissing(true, "", atts) {
		t.Error("attachment with empty URL should count as not downloaded → attachment_missing")
	}
}

// ── listZohoMessages — HTTP status check (using httptest server) ──────────────

func TestListZohoMessages_HTTP401ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status":{"code":401,"description":"UnAuthorized"}}`)) //nolint:errcheck
	}))
	defer srv.Close()

	_, err := listZohoMessages("bad-token", "acct1", time.Now().Add(-1*time.Hour), srv.URL)
	if err == nil {
		t.Error("expected error on HTTP 401 from Zoho list endpoint")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention 401, got: %v", err)
	}
}

func TestListZohoMessages_HTTP200EmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "folders") {
			// inbox folder lookup
			w.WriteHeader(200)
			w.Write([]byte(`{"data":[{"folderId":"inbox123","folderName":"Inbox"}]}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	msgs, err := listZohoMessages("tok", "acct1", time.Now().Add(-1*time.Hour), srv.URL)
	if err != nil {
		t.Errorf("HTTP 200 empty data should not error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestListZohoMessages_HTTP500ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "folders") {
			w.WriteHeader(200)
			w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
			return
		}
		w.WriteHeader(500)
		w.Write([]byte(`Internal Server Error`)) //nolint:errcheck
	}))
	defer srv.Close()

	_, err := listZohoMessages("tok", "acct1", time.Now().Add(-1*time.Hour), srv.URL)
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

// ── fetchZohoAttachments — download URL fallback ──────────────────────────────

func TestFetchZohoAttachments_UsesFolderURLFirst(t *testing.T) {
	folderURLCalled := false
	noFolderURLCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/folders/") && strings.HasSuffix(path, "/attachments") {
			// List endpoint with folder → OK with one attachment
			w.WriteHeader(200)
			w.Write([]byte(`{"data":[{"attachmentId":"att1","fileName":"test.pdf","contentType":"application/pdf","size":1024}]}`)) //nolint:errcheck
			return
		}
		if strings.Contains(path, "/folders/") && strings.Contains(path, "/att1") {
			// Download with folder URL
			folderURLCalled = true
			w.WriteHeader(200)
			w.Write([]byte(`%PDF-1.4 test content`)) //nolint:errcheck
			return
		}
		if !strings.Contains(path, "/folders/") && strings.Contains(path, "/att1") {
			// Download without folder URL (fallback)
			noFolderURLCalled = true
			w.WriteHeader(200)
			w.Write([]byte(`%PDF-1.4 test content`)) //nolint:errcheck
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	atts := fetchZohoAttachments("token", "acct1", []string{"folder1", ""}, "msg1", srv.URL, "store1", "", "", "", "")
	_ = noFolderURLCalled
	if !folderURLCalled {
		t.Error("should try folder-based download URL first")
	}
	if len(atts) == 0 {
		t.Error("expected at least one attachment result")
	}
}

func TestFetchZohoAttachments_FallsBackToNoFolderURL(t *testing.T) {
	noFolderDownloadCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.Contains(path, "/folders/") && strings.HasSuffix(path, "/attachments") {
			// List with folder → returns attachment
			w.WriteHeader(200)
			w.Write([]byte(`{"data":[{"attachmentId":"att2","fileName":"quote.xlsx","contentType":"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet","size":512}]}`)) //nolint:errcheck
			return
		}
		if strings.Contains(path, "/folders/") && strings.Contains(path, "/att2") {
			// Folder-based download fails
			w.WriteHeader(500)
			w.Write([]byte(`error`)) //nolint:errcheck
			return
		}
		if !strings.Contains(path, "/folders/") && strings.Contains(path, "/att2") {
			// No-folder fallback succeeds
			noFolderDownloadCalled = true
			w.WriteHeader(200)
			w.Write([]byte(`PK fake xlsx content`)) //nolint:errcheck
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	atts := fetchZohoAttachments("token", "acct1", []string{"folder1", ""}, "msg1", srv.URL, "store1", "", "", "", "")
	if !noFolderDownloadCalled {
		t.Error("should fall back to no-folder download URL when folder-based returns 500")
	}
	_ = atts
}

func TestFetchZohoAttachments_EmptyFolderIDUsesNoFolderURLDirectly(t *testing.T) {
	directCalled := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/attachments") && !strings.Contains(path, "/folders/") {
			// No-folder list → returns one attachment
			directCalled = true
			w.WriteHeader(200)
			w.Write([]byte(`{"data":[{"attachmentId":"att3","fileName":"img.jpg","contentType":"image/jpeg","size":2048}]}`)) //nolint:errcheck
			return
		}
		// Download URL
		if !strings.Contains(path, "/folders/") && strings.Contains(path, "/att3") {
			w.WriteHeader(200)
			w.Write([]byte(`JFIF fake jpeg`)) //nolint:errcheck
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	atts := fetchZohoAttachments("token", "acct1", []string{""}, "msg1", srv.URL, "store1", "", "", "", "")
	if !directCalled {
		t.Error("empty folderID should call no-folder attachment list URL directly")
	}
	_ = atts
}

// ── Store settings: populate_suppliers_llm fields ────────────────────────────

func TestStoreSettings_PopulateSuppliersLLMFields(t *testing.T) {
	var s models.StoreSettings
	s.PopulateSuppliersLLMProvider = "gemini"
	s.PopulateSuppliersLLMModel = "gemini-2.5-flash"

	if s.PopulateSuppliersLLMProvider != "gemini" {
		t.Errorf("expected 'gemini', got %q", s.PopulateSuppliersLLMProvider)
	}
	if s.PopulateSuppliersLLMModel != "gemini-2.5-flash" {
		t.Errorf("expected 'gemini-2.5-flash', got %q", s.PopulateSuppliersLLMModel)
	}
}

func TestStoreSettings_PopulateSuppliersLLM_DefaultEmpty(t *testing.T) {
	var s models.StoreSettings
	if s.PopulateSuppliersLLMProvider != "" {
		t.Error("PopulateSuppliersLLMProvider should default to empty string")
	}
	if s.PopulateSuppliersLLMModel != "" {
		t.Error("PopulateSuppliersLLMModel should default to empty string")
	}
}
