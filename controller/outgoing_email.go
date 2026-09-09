package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
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
