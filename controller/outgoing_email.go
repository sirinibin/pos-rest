package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TestOutgoingEmailRequest is the body expected by TestOutgoingEmailHandler.
type TestOutgoingEmailRequest struct {
	To string `json:"to"`
}

// TestOutgoingEmailHandler sends a test email using the store's configured outgoing email provider.
// POST /v1/outgoing-email/test?store_id=<id>
// Body: { "to": "recipient@example.com" }
func TestOutgoingEmailHandler(w http.ResponseWriter, r *http.Request) {
	storeIDStr := mux.Vars(r)["store_id"]
	if storeIDStr == "" {
		storeIDStr = r.URL.Query().Get("store_id")
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	var req TestOutgoingEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.To == "" {
		http.Error(w, `{"error":"to field required"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	var store struct {
		Settings models.StoreSettings `bson:"settings"`
	}
	if err := col.FindOne(ctx, bson.M{"_id": storeObjID}).Decode(&store); err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}

	s := store.Settings
	subject := "Test email from StartPOS"
	body := "This is a test email sent from your StartPOS outgoing email configuration."
	from := s.OutgoingEmailFromAddress
	fromName := s.OutgoingEmailFromName
	if fromName != "" {
		from = fmt.Sprintf("%s <%s>", fromName, from)
	}

	switch s.OutgoingEmailProvider {
	case "smtp":
		err = sendViaSMTP(s, req.To, subject, body)
	case "sendgrid":
		err = sendViaSendGrid(s, req.To, subject, body)
	case "mailgun":
		err = sendViaMailgun(s, req.To, subject, body)
	case "ses":
		err = sendViaSES(s, req.To, subject, body)
	case "postmark":
		err = sendViaPostmark(s, req.To, subject, body)
	case "brevo":
		err = sendViaBrevo(s, req.To, subject, body)
	case "resend":
		err = sendViaResend(s, req.To, subject, body)
	default:
		http.Error(w, `{"error":"no outgoing email provider configured"}`, http.StatusBadRequest)
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = from // used in SMTP
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent", "to": req.To})
}

// ─── SMTP ─────────────────────────────────────────────────────────────────────

func sendViaSMTP(s models.StoreSettings, to, subject, body string) error {
	host := s.OutgoingEmailSMTPHost
	port := s.OutgoingEmailSMTPPort
	if port == 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	from := s.OutgoingEmailFromAddress
	auth := smtp.PlainAuth("", s.OutgoingEmailSMTPUsername, s.OutgoingEmailSMTPPassword, host)

	msg := buildMIMEMessage(from, s.OutgoingEmailFromName, to, subject, body)
	return smtp.SendMail(addr, auth, s.OutgoingEmailFromAddress, []string{to}, []byte(msg))
}

// sendViaZohoSMTPReply sends an email via Zoho SMTP (smtppro.zoho.in:465 implicit TLS).
// inReplyTo is the original email's Message-ID header; pass "" for non-reply sends.
// isHTML selects text/html content type instead of text/plain.
func sendViaZohoSMTPReply(username, password, from, to, subject, body, inReplyTo string, isHTML bool) error {
	const host = "smtppro.zoho.in"
	const port = 465

	tlsConfig := &tls.Config{ServerName: host}
	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", host, port), tlsConfig)
	if err != nil {
		return fmt.Errorf("zoho smtp dial: %w", err)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("zoho smtp client: %w", err)
	}
	defer client.Quit()

	if err = client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
		return fmt.Errorf("zoho smtp auth: %w", err)
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	var mimeStr string
	if isHTML {
		mimeStr = buildMIMEReplyHTML(from, to, subject, body, inReplyTo, "")
	} else {
		mimeStr = buildMIMEReplyMessage(from, to, subject, body, inReplyTo, "")
	}
	_, err = wc.Write([]byte(mimeStr))
	if err != nil {
		return err
	}
	return wc.Close()
}

// smtpHostFromIMAPHost derives the outgoing SMTP host/port/TLS from an IMAP host.
func smtpHostFromIMAPHost(imapHost string) (host string, port int, useImplicitTLS bool) {
	switch imapHost {
	case "imappro.zoho.in":
		return "smtppro.zoho.in", 465, true
	case "imap.zoho.eu":
		return "smtp.zoho.eu", 465, true
	case "imap.zoho.com.au":
		return "smtp.zoho.com.au", 465, true
	case "imap.zoho.com":
		return "smtp.zoho.com", 465, true
	case "imap.gmail.com":
		return "smtp.gmail.com", 587, false
	case "imap.mail.yahoo.com":
		return "smtp.mail.yahoo.com", 465, true
	case "outlook.office365.com", "imap-mail.outlook.com":
		return "smtp.office365.com", 587, false
	default:
		return strings.Replace(imapHost, "imap", "smtp", 1), 587, false
	}
}

// sendViaIMAPCredentialsSMTP sends an email using the IMAP username/password with the
// SMTP server derived from the IMAP host. Tries implicit TLS (port 465) then STARTTLS (port 587).
// inReplyTo is the original email's Message-ID header; pass "" for non-reply sends.
// isHTML selects text/html content type instead of text/plain.
func sendViaIMAPCredentialsSMTP(imapHost string, imapPort int, username, password, from, to, subject, body, inReplyTo string, isHTML bool) error {
	smtpHost, smtpPort, useSSL := smtpHostFromIMAPHost(imapHost)
	// If the caller provided a non-standard IMAP port, try to derive SMTP port heuristically.
	if imapPort == 993 || imapPort == 465 {
		useSSL = true
		if smtpPort == 587 {
			smtpPort = 465
		}
	}
	var msg string
	if isHTML {
		msg = buildMIMEReplyHTML(from, to, subject, body, inReplyTo, "")
	} else {
		msg = buildMIMEReplyMessage(from, to, subject, body, inReplyTo, "")
	}
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)

	if useSSL {
		// Implicit TLS
		tlsConfig := &tls.Config{ServerName: smtpHost}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("smtp dial %s: %w", addr, err)
		}
		client, err := smtp.NewClient(conn, smtpHost)
		if err != nil {
			return fmt.Errorf("smtp client %s: %w", addr, err)
		}
		defer client.Quit()
		if err = client.Auth(smtp.PlainAuth("", username, password, smtpHost)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		if err = client.Mail(from); err != nil {
			return err
		}
		if err = client.Rcpt(to); err != nil {
			return err
		}
		wc, err := client.Data()
		if err != nil {
			return err
		}
		if _, err = wc.Write([]byte(msg)); err != nil {
			return err
		}
		return wc.Close()
	}
	// STARTTLS
	return smtp.SendMail(addr, smtp.PlainAuth("", username, password, smtpHost), from, []string{to}, []byte(msg))
}

// ─── SendGrid ─────────────────────────────────────────────────────────────────

func sendViaSendGrid(s models.StoreSettings, to, subject, body string) error {
	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{"to": []map[string]string{{"email": to}}},
		},
		"from":    map[string]string{"email": s.OutgoingEmailFromAddress, "name": s.OutgoingEmailFromName},
		"subject": subject,
		"content": []map[string]string{{"type": "text/plain", "value": body}},
	}
	return doJSONPost("https://api.sendgrid.com/v3/mail/send",
		map[string]string{"Authorization": "Bearer " + s.OutgoingEmailSendGridAPIKey},
		payload, 202)
}

// ─── Mailgun ──────────────────────────────────────────────────────────────────

func sendViaMailgun(s models.StoreSettings, to, subject, body string) error {
	domain := s.OutgoingEmailMailgunDomain
	url := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", domain)
	from := s.OutgoingEmailFromAddress
	if s.OutgoingEmailFromName != "" {
		from = fmt.Sprintf("%s <%s>", s.OutgoingEmailFromName, from)
	}

	data := strings.NewReader(fmt.Sprintf("from=%s&to=%s&subject=%s&text=%s",
		urlEncode(from), urlEncode(to), urlEncode(subject), urlEncode(body)))

	req, err := http.NewRequest("POST", url, data)
	if err != nil {
		return err
	}
	req.SetBasicAuth("api", s.OutgoingEmailMailgunAPIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("mailgun returned %d", resp.StatusCode)
	}
	return nil
}

// ─── AWS SES (SES v2 REST API) ────────────────────────────────────────────────

func sendViaSES(s models.StoreSettings, to, subject, body string) error {
	region := s.OutgoingEmailSESRegion
	if region == "" {
		region = "us-east-1"
	}
	url := fmt.Sprintf("https://email.%s.amazonaws.com/v2/email/outbound-emails", region)

	from := s.OutgoingEmailFromAddress
	if s.OutgoingEmailFromName != "" {
		from = fmt.Sprintf("%s <%s>", s.OutgoingEmailFromName, from)
	}

	payload := map[string]interface{}{
		"FromEmailAddress": from,
		"Destination":      map[string]interface{}{"ToAddresses": []string{to}},
		"Content": map[string]interface{}{
			"Simple": map[string]interface{}{
				"Subject": map[string]string{"Data": subject},
				"Body":    map[string]interface{}{"Text": map[string]string{"Data": body}},
			},
		},
	}

	b, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	dateStr := now.Format("20060102")
	timeStr := now.Format("20060102T150405Z")

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Date", timeStr)
	req.Header.Set("Host", req.URL.Host)

	signSESRequest(req, b, s.OutgoingEmailSESAccessKeyID, s.OutgoingEmailSESSecretKey, region, dateStr, timeStr)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("SES returned %d", resp.StatusCode)
	}
	return nil
}

func signSESRequest(req *http.Request, body []byte, accessKey, secretKey, region, dateStr, timeStr string) {
	service := "ses"
	bodyHash := fmt.Sprintf("%x", sha256sum(body))

	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-date:%s\n",
		req.Header.Get("Content-Type"), req.URL.Host, timeStr)
	signedHeaders := "content-type;host;x-amz-date"

	canonicalRequest := strings.Join([]string{
		"POST",
		req.URL.Path,
		"",
		canonicalHeaders,
		signedHeaders,
		bodyHash,
	}, "\n")

	credentialScope := strings.Join([]string{dateStr, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		timeStr,
		credentialScope,
		fmt.Sprintf("%x", sha256sum([]byte(canonicalRequest))),
	}, "\n")

	signingKey := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256(
		[]byte("AWS4"+secretKey), []byte(dateStr)),
		[]byte(region)),
		[]byte(service)),
		[]byte("aws4_request"))
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature))
}

func sha256sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// ─── Postmark ─────────────────────────────────────────────────────────────────

func sendViaPostmark(s models.StoreSettings, to, subject, body string) error {
	from := s.OutgoingEmailFromAddress
	if s.OutgoingEmailFromName != "" {
		from = fmt.Sprintf("%s <%s>", s.OutgoingEmailFromName, from)
	}
	payload := map[string]interface{}{
		"From":     from,
		"To":       to,
		"Subject":  subject,
		"TextBody": body,
	}
	return doJSONPost("https://api.postmarkapp.com/email",
		map[string]string{
			"Accept":                  "application/json",
			"X-Postmark-Server-Token": s.OutgoingEmailPostmarkServerToken,
		},
		payload, 200)
}

// ─── Brevo (Sendinblue) ───────────────────────────────────────────────────────

func sendViaBrevo(s models.StoreSettings, to, subject, body string) error {
	payload := map[string]interface{}{
		"sender":      map[string]string{"name": s.OutgoingEmailFromName, "email": s.OutgoingEmailFromAddress},
		"to":          []map[string]string{{"email": to}},
		"subject":     subject,
		"textContent": body,
	}
	return doJSONPost("https://api.brevo.com/v3/smtp/email",
		map[string]string{"api-key": s.OutgoingEmailBrevoAPIKey},
		payload, 201)
}

// ─── Resend ───────────────────────────────────────────────────────────────────

func sendViaResend(s models.StoreSettings, to, subject, body string) error {
	from := s.OutgoingEmailFromAddress
	if s.OutgoingEmailFromName != "" {
		from = fmt.Sprintf("%s <%s>", s.OutgoingEmailFromName, from)
	}
	payload := map[string]interface{}{
		"from":    from,
		"to":      []string{to},
		"subject": subject,
		"text":    body,
	}
	return doJSONPost("https://api.resend.com/emails",
		map[string]string{"Authorization": "Bearer " + s.OutgoingEmailResendAPIKey},
		payload, 200)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func doJSONPost(url string, headers map[string]string, payload interface{}, wantStatus int) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		return fmt.Errorf("provider returned %d (want %d)", resp.StatusCode, wantStatus)
	}
	return nil
}

func buildMIMEMessage(from, fromName, to, subject, body string) string {
	if fromName != "" {
		from = fmt.Sprintf("%s <%s>", fromName, from)
	}
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n%s",
		from, to, subject, base64.StdEncoding.EncodeToString([]byte(body)))
}

// buildMIMEReplyMessage builds an RFC 2822 email with In-Reply-To and References headers
// so the reply threads correctly in the recipient's inbox. inReplyTo should be the
// original email's Message-ID header value (e.g. "<abc@domain.com>").
func buildMIMEReplyMessage(from, to, subject, body, inReplyTo, messageID string) string {
	var sb strings.Builder
	if messageID != "" {
		sb.WriteString("Message-ID: " + messageID + "\r\n")
	}
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	if inReplyTo != "" {
		sb.WriteString("In-Reply-To: " + inReplyTo + "\r\n")
		sb.WriteString("References: " + inReplyTo + "\r\n")
	}
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(base64.StdEncoding.EncodeToString([]byte(body)))
	return sb.String()
}

// emailAttachment holds a single file to attach to an outgoing email.
type emailAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// extractHTMLBodyContent strips <!DOCTYPE>, <html>, <head>, and outer <body> wrapper
// tags from a full HTML document, returning just the inner body content.
// If the input has no <body> tag it is returned as-is (already a fragment).
func extractHTMLBodyContent(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<body")
	if start < 0 {
		return html
	}
	tagEnd := strings.Index(lower[start:], ">")
	if tagEnd < 0 {
		return html
	}
	inner := strings.TrimSpace(html[start+tagEnd+1:])
	closeBody := strings.LastIndex(strings.ToLower(inner), "</body>")
	if closeBody >= 0 {
		inner = strings.TrimSpace(inner[:closeBody])
	}
	return inner
}

// buildHTMLEmailBody produces a complete, valid HTML email body that wraps the
// plain-text message and appends an optional HTML signature fragment.
// plainText is the user's message; htmlSig is already-extracted body content.
func buildHTMLEmailBody(plainText, htmlSig string) string {
	// Escape < > & in the plain-text portion before embedding in HTML
	escaped := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(plainText)
	// Convert newlines to <br> for readability
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")

	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE html><html><head><meta charset="UTF-8"></head><body style="margin:0;padding:0;font-family:Arial,sans-serif;font-size:14px;color:#202124;">`)
	sb.WriteString(`<p style="margin:0 0 16px;">`)
	sb.WriteString(escaped)
	sb.WriteString(`</p>`)
	if htmlSig != "" {
		sb.WriteString(`<p style="margin:16px 0 6px;color:#6b7280;font-size:12px;">--</p>`)
		sb.WriteString(htmlSig)
	}
	sb.WriteString(`</body></html>`)
	return sb.String()
}

// buildMIMEReplyHTML builds an RFC 2822 email identical to buildMIMEReplyMessage but
// with Content-Type: text/html so HTML signatures and markup render in email clients.
func buildMIMEReplyHTML(from, to, subject, body, inReplyTo, messageID string) string {
	var sb strings.Builder
	if messageID != "" {
		sb.WriteString("Message-ID: " + messageID + "\r\n")
	}
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	if inReplyTo != "" {
		sb.WriteString("In-Reply-To: " + inReplyTo + "\r\n")
		sb.WriteString("References: " + inReplyTo + "\r\n")
	}
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: base64\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(base64.StdEncoding.EncodeToString([]byte(body)))
	return sb.String()
}

// buildMIMEReplyFull builds a complete RFC 2822 MIME message, with optional
// In-Reply-To threading headers, a Message-ID, and file attachments.
// isHTML selects text/html content type instead of text/plain.
func buildMIMEReplyFull(from, to, subject, body, inReplyTo, messageID string, attachments []emailAttachment, isHTML bool) []byte {
	if len(attachments) == 0 {
		if isHTML {
			return []byte(buildMIMEReplyHTML(from, to, subject, body, inReplyTo, messageID))
		}
		return []byte(buildMIMEReplyMessage(from, to, subject, body, inReplyTo, messageID))
	}

	bodyContentType := "text/plain; charset=UTF-8"
	if isHTML {
		bodyContentType = "text/html; charset=UTF-8"
	}

	// Build the multipart body first so we know the boundary
	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	boundary := mw.Boundary()

	// Body part
	ph := make(textproto.MIMEHeader)
	ph.Set("Content-Type", bodyContentType)
	ph.Set("Content-Transfer-Encoding", "base64")
	pw, _ := mw.CreatePart(ph)
	pw.Write([]byte(base64.StdEncoding.EncodeToString([]byte(body)))) //nolint:errcheck

	// Attachment parts
	for _, att := range attachments {
		ct := att.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		ah := make(textproto.MIMEHeader)
		ah.Set("Content-Type", fmt.Sprintf("%s; name=%q", ct, att.Filename))
		ah.Set("Content-Transfer-Encoding", "base64")
		ah.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
		aw, _ := mw.CreatePart(ah)
		aw.Write([]byte(base64.StdEncoding.EncodeToString(att.Data))) //nolint:errcheck
	}
	mw.Close() //nolint:errcheck

	// Now assemble headers + multipart body
	var out bytes.Buffer
	if messageID != "" {
		fmt.Fprintf(&out, "Message-ID: %s\r\n", messageID)
	}
	fmt.Fprintf(&out, "From: %s\r\n", from)
	fmt.Fprintf(&out, "To: %s\r\n", to)
	fmt.Fprintf(&out, "Subject: %s\r\n", subject)
	if inReplyTo != "" {
		fmt.Fprintf(&out, "In-Reply-To: %s\r\n", inReplyTo)
		fmt.Fprintf(&out, "References: %s\r\n", inReplyTo)
	}
	fmt.Fprintf(&out, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&out, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	fmt.Fprintf(&out, "\r\n")
	out.Write(bodyBuf.Bytes())
	return out.Bytes()
}

// sendSMTPRaw sends a pre-built MIME message via implicit-TLS SMTP on port 465.
func sendSMTPRaw(smtpHost string, smtpPort int, username, password, from, to string, msgBytes []byte) error {
	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	tlsConfig := &tls.Config{ServerName: smtpHost}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		return fmt.Errorf("smtp client %s: %w", addr, err)
	}
	defer client.Quit()
	if err = client.Auth(smtp.PlainAuth("", username, password, smtpHost)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	wc, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = wc.Write(msgBytes); err != nil {
		return err
	}
	return wc.Close()
}

func urlEncode(s string) string {
	var buf strings.Builder
	for _, c := range []byte(s) {
		if isUnreserved(c) {
			buf.WriteByte(c)
		} else {
			fmt.Fprintf(&buf, "%%%02X", c)
		}
	}
	return buf.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}
