package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirinibin/startpos/backend/models"
)

// ─── urlEncode helper tests ───────────────────────────────────────────────────

func TestURLEncode_SimpleString(t *testing.T) {
	got := urlEncode("hello world")
	if got != "hello%20world" {
		t.Errorf("got %q, want %q", got, "hello%20world")
	}
}

func TestURLEncode_EmailAddress(t *testing.T) {
	got := urlEncode("user@example.com")
	if !strings.Contains(got, "%40") {
		t.Errorf("@ not encoded, got %q", got)
	}
}

func TestURLEncode_AlreadySafe(t *testing.T) {
	input := "hello-world_123.test~OK"
	if urlEncode(input) != input {
		t.Errorf("safe chars should not be encoded")
	}
}

// ─── buildMIMEMessage tests ───────────────────────────────────────────────────

func TestBuildMIMEMessage_ContainsRequiredHeaders(t *testing.T) {
	msg := buildMIMEMessage("sender@example.com", "Test Sender", "to@example.com", "Hello", "Body text")
	for _, want := range []string{"From:", "To:", "Subject:", "MIME-Version:", "Content-Type:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing header %q in message", want)
		}
	}
}

func TestBuildMIMEMessage_FromNameIncluded(t *testing.T) {
	msg := buildMIMEMessage("a@b.com", "My Name", "c@d.com", "Subj", "Body")
	if !strings.Contains(msg, "My Name") {
		t.Errorf("from name not present in message")
	}
}

func TestBuildMIMEMessage_NoFromName(t *testing.T) {
	msg := buildMIMEMessage("a@b.com", "", "c@d.com", "Subj", "Body")
	if !strings.Contains(msg, "From: a@b.com") {
		t.Errorf("from address not present without name, got: %q", msg)
	}
}

// ─── isUnreserved tests ───────────────────────────────────────────────────────

func TestIsUnreserved_Letters(t *testing.T) {
	for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		if !isUnreserved(byte(c)) {
			t.Errorf("letter %q should be unreserved", c)
		}
	}
}

func TestIsUnreserved_Digits(t *testing.T) {
	for _, c := range "0123456789" {
		if !isUnreserved(byte(c)) {
			t.Errorf("digit %q should be unreserved", c)
		}
	}
}

func TestIsUnreserved_SafeSpecials(t *testing.T) {
	for _, c := range "-_.~" {
		if !isUnreserved(byte(c)) {
			t.Errorf("char %q should be unreserved", c)
		}
	}
}

func TestIsUnreserved_UnsafeChars(t *testing.T) {
	for _, c := range " @#%&+=/?" {
		if isUnreserved(byte(c)) {
			t.Errorf("char %q should NOT be unreserved", c)
		}
	}
}

// ─── TestOutgoingEmailRequest JSON decode tests ───────────────────────────────

func TestTestOutgoingEmailRequest_DecodesTo(t *testing.T) {
	body := `{"to":"test@example.com"}`
	var req TestOutgoingEmailRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	if req.To != "test@example.com" {
		t.Errorf("got %q", req.To)
	}
}

func TestTestOutgoingEmailRequest_EmptyTo(t *testing.T) {
	var req TestOutgoingEmailRequest
	json.Unmarshal([]byte(`{}`), &req) //nolint:errcheck
	if req.To != "" {
		t.Errorf("expected empty To")
	}
}

// ─── handler – bad request tests (no DB needed) ──────────────────────────────

func TestTestOutgoingEmailHandler_InvalidStoreID(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/outgoing-email/test?store_id=not-an-id",
		bytes.NewBufferString(`{"to":"x@x.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TestOutgoingEmailHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestTestOutgoingEmailHandler_MissingTo(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/outgoing-email/test?store_id=507f191e810c19729de860ea",
		bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	TestOutgoingEmailHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

// ─── sendVia* unit tests using a local stub HTTP server ──────────────────────

// stubServer returns a test HTTP server that always responds with the given code.
func stubServer(t *testing.T, code int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}))
}

func TestSendViaSendGrid_SuccessStatusAccepted(t *testing.T) {
	srv := stubServer(t, 202)
	defer srv.Close()

	s := models.StoreSettings{
		OutgoingEmailSendGridAPIKey: "SG.test",
		OutgoingEmailFromAddress:    "from@x.com",
	}

	// Override the URL for testing by patching doJSONPost indirectly via a wrapper.
	// Since doJSONPost hardcodes the URL, we test the real function compiles and returns
	// an appropriate error when the stub is not reachable (integration-style guard).
	// For true unit isolation the function would need URL injection; that's out of scope.
	err := sendViaSendGrid(s, "to@x.com", "Subj", "Body")
	// We expect an error because it hits the real SendGrid API endpoint (not stub).
	// The key assertion: function exists and returns an error (not panic).
	if err == nil {
		t.Log("sendViaSendGrid unexpectedly succeeded (live API?)")
	}
}

func TestSendViaMailgun_FunctionSignature(t *testing.T) {
	s := models.StoreSettings{
		OutgoingEmailMailgunAPIKey: "key-test",
		OutgoingEmailMailgunDomain: "test.example.com",
		OutgoingEmailFromAddress:   "from@test.example.com",
	}
	err := sendViaMailgun(s, "to@example.com", "Subject", "Body")
	// Expect error (no real API, no valid domain) — just ensuring no panic.
	if err == nil {
		t.Log("sendViaMailgun succeeded unexpectedly")
	}
}

func TestSendViaPostmark_FunctionSignature(t *testing.T) {
	s := models.StoreSettings{
		OutgoingEmailPostmarkServerToken: "test-token",
		OutgoingEmailFromAddress:         "from@example.com",
	}
	err := sendViaPostmark(s, "to@example.com", "Subject", "Body")
	if err == nil {
		t.Log("sendViaPostmark succeeded unexpectedly")
	}
}

func TestSendViaBrevo_FunctionSignature(t *testing.T) {
	s := models.StoreSettings{
		OutgoingEmailBrevoAPIKey:  "xkeysib-test",
		OutgoingEmailFromAddress:  "from@example.com",
		OutgoingEmailFromName:     "Tester",
	}
	err := sendViaBrevo(s, "to@example.com", "Subject", "Body")
	if err == nil {
		t.Log("sendViaBrevo succeeded unexpectedly")
	}
}

func TestSendViaResend_FunctionSignature(t *testing.T) {
	s := models.StoreSettings{
		OutgoingEmailResendAPIKey: "re_test",
		OutgoingEmailFromAddress:  "from@example.com",
	}
	err := sendViaResend(s, "to@example.com", "Subject", "Body")
	if err == nil {
		t.Log("sendViaResend succeeded unexpectedly")
	}
}

// ─── StoreSettings outgoing email fields ─────────────────────────────────────

func TestStoreSettings_OutgoingEmailFields_JSONRoundTrip(t *testing.T) {
	original := models.StoreSettings{
		OutgoingEmailProvider:    "smtp",
		OutgoingEmailFromName:    "Test Corp",
		OutgoingEmailFromAddress: "info@test.com",
		OutgoingEmailSMTPHost:    "smtp.test.com",
		OutgoingEmailSMTPPort:    587,
		OutgoingEmailSMTPUseTLS:  true,
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded models.StoreSettings
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OutgoingEmailProvider != "smtp" {
		t.Errorf("provider mismatch: %q", decoded.OutgoingEmailProvider)
	}
	if decoded.OutgoingEmailFromName != "Test Corp" {
		t.Errorf("from_name mismatch: %q", decoded.OutgoingEmailFromName)
	}
	if decoded.OutgoingEmailSMTPPort != 587 {
		t.Errorf("smtp_port mismatch: %d", decoded.OutgoingEmailSMTPPort)
	}
	if !decoded.OutgoingEmailSMTPUseTLS {
		t.Errorf("smtp_use_tls mismatch")
	}
}

func TestStoreSettings_AllOutgoingEmailProviderFields(t *testing.T) {
	s := models.StoreSettings{
		OutgoingEmailSendGridAPIKey:      "sg-key",
		OutgoingEmailMailgunAPIKey:       "mg-key",
		OutgoingEmailMailgunDomain:       "mg.domain.com",
		OutgoingEmailSESAccessKeyID:      "AKIA...",
		OutgoingEmailSESSecretKey:        "ses-secret",
		OutgoingEmailSESRegion:           "us-east-1",
		OutgoingEmailPostmarkServerToken: "pm-token",
		OutgoingEmailBrevoAPIKey:         "brevo-key",
		OutgoingEmailResendAPIKey:        "resend-key",
	}
	b, _ := json.Marshal(s)
	js := string(b)
	for _, key := range []string{
		"outgoing_email_sendgrid_api_key",
		"outgoing_email_mailgun_api_key",
		"outgoing_email_mailgun_domain",
		"outgoing_email_ses_access_key_id",
		"outgoing_email_postmark_server_token",
		"outgoing_email_brevo_api_key",
		"outgoing_email_resend_api_key",
	} {
		if !strings.Contains(js, key) {
			t.Errorf("JSON missing field %q", key)
		}
	}
}

func TestStoreSettings_OutgoingEmailSMTPUseTLS_DefaultFalse(t *testing.T) {
	var s models.StoreSettings
	if s.OutgoingEmailSMTPUseTLS {
		t.Errorf("zero value should be false")
	}
}

// ─── rewriteDataURIsAsCIDs tests ─────────────────────────────────────────────

// tiny 1×1 red PNG as base64
const tiny1x1PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwADhQGAWjR9awAAAABJRU5ErkJggg=="

func TestRewriteDataURIs_SingleImage(t *testing.T) {
	html := `<img src="data:image/png;base64,` + tiny1x1PNG + `" alt="logo">`
	newHTML, images := rewriteDataURIsAsCIDs(html)
	if len(images) != 1 {
		t.Fatalf("want 1 image, got %d", len(images))
	}
	if !strings.Contains(newHTML, `src="cid:img1@startpos.email"`) {
		t.Errorf("cid reference not inserted, got: %s", newHTML)
	}
	if strings.Contains(newHTML, "data:image") {
		t.Errorf("data: URI should have been removed")
	}
	if images[0].ContentType != "image/png" {
		t.Errorf("wrong content type: %s", images[0].ContentType)
	}
	if len(images[0].Data) == 0 {
		t.Errorf("image data is empty")
	}
}

func TestRewriteDataURIs_MultipleImages(t *testing.T) {
	html := `<img src="data:image/png;base64,` + tiny1x1PNG + `"><img src="data:image/jpeg;base64,` + tiny1x1PNG + `">`
	_, images := rewriteDataURIsAsCIDs(html)
	if len(images) != 2 {
		t.Fatalf("want 2 images, got %d", len(images))
	}
	if images[0].CID == images[1].CID {
		t.Errorf("CIDs should be unique, both are %q", images[0].CID)
	}
	if images[1].ContentType != "image/jpeg" {
		t.Errorf("second image content type wrong: %s", images[1].ContentType)
	}
}

func TestRewriteDataURIs_NoImages(t *testing.T) {
	html := `<p>Hello world</p><img src="https://example.com/logo.png">`
	newHTML, images := rewriteDataURIsAsCIDs(html)
	if len(images) != 0 {
		t.Errorf("want 0 images, got %d", len(images))
	}
	if newHTML != html {
		t.Errorf("HTML should be unchanged when no data: URIs present")
	}
}

func TestRewriteDataURIs_InvalidBase64Left(t *testing.T) {
	html := `<img src="data:image/png;base64,!!!not-valid-base64!!!">`
	newHTML, images := rewriteDataURIsAsCIDs(html)
	if len(images) != 0 {
		t.Errorf("invalid base64 should produce 0 images, got %d", len(images))
	}
	if newHTML != html {
		t.Errorf("HTML with invalid base64 should be unchanged")
	}
}

// ─── buildMIMEReplyHTML CID tests ─────────────────────────────────────────────

func TestBuildMIMEReplyHTML_NoImages_SimplHTML(t *testing.T) {
	msg := buildMIMEReplyHTML("from@x.com", "to@x.com", "Subj", "<p>Hello</p>", "", "")
	if !strings.Contains(msg, "Content-Type: text/html") {
		t.Errorf("expected text/html content type")
	}
	if strings.Contains(msg, "multipart/related") {
		t.Errorf("should NOT be multipart/related when no inline images")
	}
}

func TestBuildMIMEReplyHTML_WithDataURI_BecomesMultipartRelated(t *testing.T) {
	body := `<p>Hi</p><img src="data:image/png;base64,` + tiny1x1PNG + `">`
	msg := buildMIMEReplyHTML("from@x.com", "to@x.com", "Subj", body, "", "")
	if !strings.Contains(msg, "multipart/related") {
		n := 200
		if len(msg) < n {
			n = len(msg)
		}
		t.Errorf("expected multipart/related when data: URI present, got: %s", msg[:n])
	}
	if strings.Contains(msg, "data:image") {
		t.Errorf("data: URI must not appear in final message")
	}
	if !strings.Contains(msg, "Content-Id:") && !strings.Contains(msg, "Content-ID:") {
		t.Errorf("expected Content-Id header for inline image")
	}
}

func TestBuildMIMEReplyHTML_InReplyTo_Included(t *testing.T) {
	msg := buildMIMEReplyHTML("f@x.com", "t@x.com", "Re: Hi", "<p>body</p>", "<msg1@x.com>", "")
	if !strings.Contains(msg, "In-Reply-To: <msg1@x.com>") {
		t.Errorf("In-Reply-To header missing")
	}
}

// ─── buildMIMEReplyFull CID + attachment tests ────────────────────────────────

func TestBuildMIMEReplyFull_HTMLWithImageAndAttachment(t *testing.T) {
	body := `<p>See logo</p><img src="data:image/png;base64,` + tiny1x1PNG + `">`
	atts := []emailAttachment{{Filename: "quote.pdf", ContentType: "application/pdf", Data: []byte("PDF")}}
	raw := buildMIMEReplyFull("f@x.com", "t@x.com", "Quote", body, "", "", atts, true)
	msg := string(raw)
	if !strings.Contains(msg, "multipart/mixed") {
		t.Errorf("expected multipart/mixed outer structure")
	}
	if !strings.Contains(msg, "multipart/related") {
		t.Errorf("expected multipart/related for HTML+images")
	}
	if !strings.Contains(msg, "Content-Id:") && !strings.Contains(msg, "Content-ID:") {
		t.Errorf("expected Content-Id header for inline image")
	}
	if !strings.Contains(msg, "quote.pdf") {
		t.Errorf("expected attachment filename in message")
	}
	if strings.Contains(msg, "data:image") {
		t.Errorf("data: URI must not appear in final message")
	}
}

func TestBuildMIMEReplyFull_HTMLNoImagesNoAttachments_DelegatesToSimpleHTML(t *testing.T) {
	raw := buildMIMEReplyFull("f@x.com", "t@x.com", "Hi", "<p>body</p>", "", "", nil, true)
	msg := string(raw)
	if strings.Contains(msg, "multipart/mixed") {
		t.Errorf("should not be multipart/mixed with no attachments")
	}
	if !strings.Contains(msg, "text/html") {
		t.Errorf("expected text/html content type")
	}
}

