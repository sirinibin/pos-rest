package controller

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func rfqEmailAPIBase() string {
	base := os.Getenv("REACT_APP_API_URL")
	if base == "" {
		base = "https://startpos-api.startuptech.uk"
	}
	return base
}

func rfqEmailWebhookURL(storeID, provider string) string {
	return fmt.Sprintf("%s/v1/rfq-email/webhook?store_id=%s&provider=%s",
		rfqEmailAPIBase(), storeID, provider)
}

func rfqEmailOAuthCallbackURL() string {
	return rfqEmailAPIBase() + "/v1/rfq-email/oauth-callback"
}

func rfqEmailGetStore(storeIDStr string) (*models.Store, primitive.ObjectID, error) {
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		return nil, primitive.NilObjectID, fmt.Errorf("invalid store_id")
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		return nil, primitive.NilObjectID, fmt.Errorf("store not found")
	}
	return store, storeObjID, nil
}

// rfqEmailSave persists arbitrary email settings fields to the store document.
func rfqEmailSave(storeObjID primitive.ObjectID, fields bson.M) error {
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx, bson.M{"_id": storeObjID}, bson.M{"$set": fields})
	return err
}

// ── IMAP connection test ──────────────────────────────────────────────────────

func imapTestLogin(host string, port int, useSSL bool, username, password string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	timeout := 10 * time.Second

	var conn net.Conn
	var err error
	if useSSL {
		conn, err = tls.DialWithDialer(
			&net.Dialer{Timeout: timeout}, "tcp", addr,
			&tls.Config{ServerName: host},
		)
	} else {
		conn, err = net.DialTimeout("tcp", addr, timeout)
	}
	if err != nil {
		return fmt.Errorf("cannot connect to %s: %v", addr, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	reader := bufio.NewReader(conn)
	greeting, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("no greeting from server: %v", err)
	}
	if !strings.HasPrefix(greeting, "* OK") && !strings.HasPrefix(greeting, "* PREAUTH") {
		return fmt.Errorf("unexpected greeting: %s", strings.TrimSpace(greeting))
	}

	cmd := fmt.Sprintf("A1 LOGIN %s %s\r\n", imapQuote(username), imapQuote(password))
	if _, err := fmt.Fprint(conn, cmd); err != nil {
		return fmt.Errorf("failed to send LOGIN: %v", err)
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read LOGIN response: %v", err)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "A1 OK") {
			return nil
		}
		if strings.HasPrefix(line, "A1 NO") || strings.HasPrefix(line, "A1 BAD") {
			return fmt.Errorf("login rejected: %s", line)
		}
	}
}

func imapQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// ── API-key validators ────────────────────────────────────────────────────────

func mailgunValidate(apiKey string) error {
	req, _ := http.NewRequest("GET", "https://api.mailgun.net/v4/domains", nil)
	req.SetBasicAuth("api", apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("mailgun API unreachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("mailgun API key invalid (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func sendgridValidate(apiKey string) error {
	req, _ := http.NewRequest("GET", "https://api.sendgrid.com/v3/user/profile", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("sendgrid API unreachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("sendgrid API key invalid (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func postmarkValidate(serverToken string) error {
	req, _ := http.NewRequest("GET", "https://api.postmarkapp.com/server", nil)
	req.Header.Set("X-Postmark-Server-Token", serverToken)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("postmark API unreachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 422 {
		return fmt.Errorf("postmark server token invalid (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// ── OAuth URL builders ────────────────────────────────────────────────────────

func oauthState(storeID, provider string) string {
	return base64.URLEncoding.EncodeToString([]byte(storeID + ":" + provider))
}

func gmailOAuthURL(clientID, state, redirectURI string) string {
	p := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI},
		"response_type": {"code"},
		"scope":         {"https://www.googleapis.com/auth/gmail.readonly https://www.googleapis.com/auth/userinfo.email"},
		"access_type":   {"offline"}, "prompt": {"consent"}, "state": {state},
	}
	return "https://accounts.google.com/o/oauth2/auth?" + p.Encode()
}

func outlookOAuthURL(tenantID, clientID, state, redirectURI string) string {
	if tenantID == "" {
		tenantID = "common"
	}
	p := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI},
		"response_type": {"code"},
		"scope":         {"https://graph.microsoft.com/Mail.Read offline_access https://graph.microsoft.com/User.Read"},
		"state":         {state},
	}
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?%s", tenantID, p.Encode())
}

func zohoOAuthURL(clientID, state, redirectURI string) string {
	p := url.Values{
		"client_id": {clientID}, "redirect_uri": {redirectURI},
		"response_type": {"code"},
		"scope":         {"ZohoMail.messages.READ ZohoMail.folders.READ ZohoMail.accounts.READ"},
		"access_type":   {"offline"}, "state": {state},
	}
	return "https://accounts.zoho.com/oauth/v2/auth?" + p.Encode()
}

// ── OAuth token exchange ──────────────────────────────────────────────────────

type rfqOAuthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func exchangeGmailCode(code, clientID, clientSecret, redirectURI string) (*rfqOAuthToken, error) {
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code": {code}, "client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {redirectURI}, "grant_type": {"authorization_code"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tok rfqOAuthToken
	json.NewDecoder(resp.Body).Decode(&tok)
	if tok.Error != "" {
		return nil, fmt.Errorf("token exchange: %s — %s", tok.Error, tok.ErrorDesc)
	}
	return &tok, nil
}

func exchangeOutlookCode(code, tenantID, clientID, clientSecret, redirectURI string) (*rfqOAuthToken, error) {
	if tenantID == "" {
		tenantID = "common"
	}
	resp, err := http.PostForm(
		fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID),
		url.Values{
			"code": {code}, "client_id": {clientID}, "client_secret": {clientSecret},
			"redirect_uri": {redirectURI}, "grant_type": {"authorization_code"},
			"scope": {"https://graph.microsoft.com/Mail.Read offline_access https://graph.microsoft.com/User.Read"},
		})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tok rfqOAuthToken
	json.NewDecoder(resp.Body).Decode(&tok)
	if tok.Error != "" {
		return nil, fmt.Errorf("token exchange: %s — %s", tok.Error, tok.ErrorDesc)
	}
	return &tok, nil
}

func exchangeZohoCode(code, clientID, clientSecret, redirectURI string) (*rfqOAuthToken, error) {
	resp, err := http.PostForm("https://accounts.zoho.com/oauth/v2/token", url.Values{
		"code": {code}, "client_id": {clientID}, "client_secret": {clientSecret},
		"redirect_uri": {redirectURI}, "grant_type": {"authorization_code"},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var tok rfqOAuthToken
	json.NewDecoder(resp.Body).Decode(&tok)
	if tok.Error != "" {
		return nil, fmt.Errorf("token exchange: %s — %s", tok.Error, tok.ErrorDesc)
	}
	return &tok, nil
}

func fetchGmailEmail(accessToken string) string {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var info struct{ Email string `json:"email"` }
	json.NewDecoder(resp.Body).Decode(&info)
	return info.Email
}

func fetchOutlookEmail(accessToken string) string {
	req, _ := http.NewRequest("GET", "https://graph.microsoft.com/v1.0/me?$select=mail,userPrincipalName", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var info struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	json.NewDecoder(resp.Body).Decode(&info)
	if info.Mail != "" {
		return info.Mail
	}
	return info.UserPrincipalName
}

func fetchZohoEmail(accessToken string) string {
	req, _ := http.NewRequest("GET", "https://mail.zoho.com/api/accounts", nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var res struct {
		Data []struct {
			EmailAddress []struct {
				MailID string `json:"mailId"`
			} `json:"emailAddress"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res)
	if len(res.Data) > 0 && len(res.Data[0].EmailAddress) > 0 {
		return res.Data[0].EmailAddress[0].MailID
	}
	return ""
}

// ── Request body ──────────────────────────────────────────────────────────────

type rfqEmailConnectReq struct {
	StoreID  string `json:"store_id"`
	Provider string `json:"provider"`
	// Gmail
	GmailClientID     string `json:"rfq_gmail_client_id"`
	GmailClientSecret string `json:"rfq_gmail_client_secret"`
	// Outlook
	OutlookTenantID     string `json:"rfq_outlook_tenant_id"`
	OutlookClientID     string `json:"rfq_outlook_client_id"`
	OutlookClientSecret string `json:"rfq_outlook_client_secret"`
	// Zoho
	ZohoClientID     string `json:"rfq_zoho_client_id"`
	ZohoClientSecret string `json:"rfq_zoho_client_secret"`
	// Mailgun
	MailgunAPIKey string `json:"rfq_mailgun_api_key"`
	MailgunDomain string `json:"rfq_mailgun_domain"`
	// SendGrid
	SendGridAPIKey string `json:"rfq_sendgrid_api_key"`
	// Postmark
	PostmarkServerToken string `json:"rfq_postmark_server_token"`
	// Amazon SES
	AWSSESAccessKeyID string `json:"rfq_aws_ses_access_key_id"`
	AWSSESSecretKey   string `json:"rfq_aws_ses_secret_key"`
	AWSSESRegion      string `json:"rfq_aws_ses_region"`
	// IMAP
	IMAPHost     string `json:"rfq_imap_host"`
	IMAPPort     int    `json:"rfq_imap_port"`
	IMAPUsername string `json:"rfq_imap_username"`
	IMAPPassword string `json:"rfq_imap_password"`
	IMAPUseSSL   bool   `json:"rfq_imap_use_ssl"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// ConnectRFQEmail validates credentials and saves them.
// For OAuth providers it returns an oauth_url; connection is confirmed after the OAuth callback.
func ConnectRFQEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req rfqEmailConnectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}
	if req.StoreID == "" || req.Provider == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "store_id and provider are required"})
		return
	}
	_, storeObjID, err := rfqEmailGetStore(req.StoreID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	redirectURI := rfqEmailOAuthCallbackURL()
	state := oauthState(req.StoreID, req.Provider)
	webhookURL := rfqEmailWebhookURL(req.StoreID, req.Provider)

	switch req.Provider {

	case "gmail":
		if req.GmailClientID == "" || req.GmailClientSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Client ID and Client Secret are required"})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":      "gmail",
			"settings.rfq_gmail_client_id":     req.GmailClientID,
			"settings.rfq_gmail_client_secret": req.GmailClientSecret,
			"settings.rfq_email_connected":     false,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"oauth_url": gmailOAuthURL(req.GmailClientID, state, redirectURI),
			"message":   "Credentials saved. Complete authorization in the opened window.",
		})

	case "outlook":
		if req.OutlookClientID == "" || req.OutlookClientSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Client ID and Client Secret are required"})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":        "outlook",
			"settings.rfq_outlook_tenant_id":     req.OutlookTenantID,
			"settings.rfq_outlook_client_id":     req.OutlookClientID,
			"settings.rfq_outlook_client_secret": req.OutlookClientSecret,
			"settings.rfq_email_connected":       false,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"oauth_url": outlookOAuthURL(req.OutlookTenantID, req.OutlookClientID, state, redirectURI),
			"message":   "Credentials saved. Complete authorization in the opened window.",
		})

	case "zoho":
		if req.ZohoClientID == "" || req.ZohoClientSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Client ID and Client Secret are required"})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":     "zoho",
			"settings.rfq_zoho_client_id":     req.ZohoClientID,
			"settings.rfq_zoho_client_secret": req.ZohoClientSecret,
			"settings.rfq_email_connected":    false,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"oauth_url": zohoOAuthURL(req.ZohoClientID, state, redirectURI),
			"message":   "Credentials saved. Complete authorization in the opened window.",
		})

	case "mailgun":
		if req.MailgunAPIKey == "" || req.MailgunDomain == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "API Key and Domain are required"})
			return
		}
		if err := mailgunValidate(req.MailgunAPIKey); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":  "mailgun",
			"settings.rfq_mailgun_api_key": req.MailgunAPIKey,
			"settings.rfq_mailgun_domain":  req.MailgunDomain,
			"settings.rfq_email_connected": true,
			"settings.rfq_email_address":   req.MailgunDomain,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected":   true,
			"webhook_url": webhookURL,
			"email":       req.MailgunDomain,
			"message":     "API key validated. Copy the webhook URL and add it to your Mailgun route.",
		})

	case "sendgrid":
		if req.SendGridAPIKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "API Key is required"})
			return
		}
		if err := sendgridValidate(req.SendGridAPIKey); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":   "sendgrid",
			"settings.rfq_sendgrid_api_key": req.SendGridAPIKey,
			"settings.rfq_email_connected":  true,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected":   true,
			"webhook_url": webhookURL,
			"message":     "API key validated. Set this URL as your SendGrid Inbound Parse webhook.",
		})

	case "postmark":
		if req.PostmarkServerToken == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Server API Token is required"})
			return
		}
		if err := postmarkValidate(req.PostmarkServerToken); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":        "postmark",
			"settings.rfq_postmark_server_token": req.PostmarkServerToken,
			"settings.rfq_email_connected":       true,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected":   true,
			"webhook_url": webhookURL,
			"message":     "Token validated. Set this URL as your Postmark Inbound Webhook.",
		})

	case "ses":
		if req.AWSSESAccessKeyID == "" || req.AWSSESSecretKey == "" || req.AWSSESRegion == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Access Key ID, Secret Key, and Region are required"})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":        "ses",
			"settings.rfq_aws_ses_access_key_id": req.AWSSESAccessKeyID,
			"settings.rfq_aws_ses_secret_key":    req.AWSSESSecretKey,
			"settings.rfq_aws_ses_region":        req.AWSSESRegion,
			"settings.rfq_email_connected":       true,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected":   true,
			"webhook_url": webhookURL,
			"message":     "Credentials saved. Configure your SES receipt rule with an SNS action pointing to the webhook URL.",
		})

	case "imap":
		if req.IMAPHost == "" || req.IMAPPort == 0 || req.IMAPUsername == "" || req.IMAPPassword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Host, Port, Username and Password are required"})
			return
		}
		if err := imapTestLogin(req.IMAPHost, req.IMAPPort, req.IMAPUseSSL, req.IMAPUsername, req.IMAPPassword); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "IMAP login failed: " + err.Error()})
			return
		}
		rfqEmailSave(storeObjID, bson.M{
			"settings.rfq_email_provider":  "imap",
			"settings.rfq_imap_host":       req.IMAPHost,
			"settings.rfq_imap_port":       req.IMAPPort,
			"settings.rfq_imap_username":   req.IMAPUsername,
			"settings.rfq_imap_password":   req.IMAPPassword,
			"settings.rfq_imap_use_ssl":    req.IMAPUseSSL,
			"settings.rfq_email_connected": true,
			"settings.rfq_email_address":   req.IMAPUsername,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"connected": true,
			"email":     req.IMAPUsername,
			"message":   "IMAP connection verified. Emails will be polled from Inbox and Junk/Spam folders.",
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unknown provider: " + req.Provider})
	}
}

// GetRFQEmailStatus returns the current email connection status.
func GetRFQEmailStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, _, err := rfqEmailGetStore(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	s := store.Settings
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connected": s.RFQEmailConnected,
		"provider":  s.RFQEmailProvider,
		"email":     s.RFQEmailAddress,
	})
}

// DisconnectRFQEmail clears all email credentials and marks as disconnected.
func DisconnectRFQEmail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, storeObjID, err := rfqEmailGetStore(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	fields := bson.M{
		"settings.rfq_email_provider": "", "settings.rfq_email_connected": false,
		"settings.rfq_email_address": "",
		"settings.rfq_gmail_client_id": "", "settings.rfq_gmail_client_secret": "",
		"settings.rfq_gmail_access_token": "", "settings.rfq_gmail_refresh_token": "",
		"settings.rfq_outlook_tenant_id": "", "settings.rfq_outlook_client_id": "",
		"settings.rfq_outlook_client_secret": "", "settings.rfq_outlook_access_token": "",
		"settings.rfq_outlook_refresh_token": "",
		"settings.rfq_zoho_client_id": "", "settings.rfq_zoho_client_secret": "",
		"settings.rfq_zoho_access_token": "", "settings.rfq_zoho_refresh_token": "",
		"settings.rfq_mailgun_api_key": "", "settings.rfq_mailgun_domain": "",
		"settings.rfq_sendgrid_api_key": "",
		"settings.rfq_postmark_server_token": "",
		"settings.rfq_aws_ses_access_key_id": "", "settings.rfq_aws_ses_secret_key": "",
		"settings.rfq_aws_ses_region": "",
		"settings.rfq_imap_host": "", "settings.rfq_imap_port": 0,
		"settings.rfq_imap_username": "", "settings.rfq_imap_password": "",
		"settings.rfq_imap_use_ssl": false,
	}
	if err := rfqEmailSave(storeObjID, fields); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to clear settings"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// HandleRFQEmailOAuthCallback handles the OAuth2 redirect for Gmail / Outlook / Zoho.
func HandleRFQEmailOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	stateRaw := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	closeHTML := `<html><body><script>
if(window.opener){window.opener.postMessage({rfqEmailOAuth:'done'},'*');}
window.close();
</script><p>Authorization complete — you can close this window.</p></body></html>`

	fail := func(msg string) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><body><p style="color:red">Authorization failed: %s</p><script>
if(window.opener){window.opener.postMessage({rfqEmailOAuth:'error',msg:%q},'*');}
</script></body></html>`, msg, msg)
	}

	if errParam != "" {
		fail(errParam)
		return
	}
	if code == "" || stateRaw == "" {
		fail("missing code or state")
		return
	}

	raw, err := base64.URLEncoding.DecodeString(stateRaw)
	if err != nil {
		fail("invalid state")
		return
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		fail("malformed state")
		return
	}
	storeID, provider := parts[0], parts[1]

	store, storeObjID, err := rfqEmailGetStore(storeID)
	if err != nil {
		fail("store not found")
		return
	}
	s := store.Settings
	redirectURI := rfqEmailOAuthCallbackURL()
	var fields bson.M

	switch provider {
	case "gmail":
		tok, err := exchangeGmailCode(code, s.RFQGmailClientID, s.RFQGmailClientSecret, redirectURI)
		if err != nil {
			fail(err.Error())
			return
		}
		email := fetchGmailEmail(tok.AccessToken)
		fields = bson.M{
			"settings.rfq_gmail_access_token":  tok.AccessToken,
			"settings.rfq_gmail_refresh_token": tok.RefreshToken,
			"settings.rfq_email_connected":     true,
			"settings.rfq_email_address":       email,
		}
	case "outlook":
		tok, err := exchangeOutlookCode(code, s.RFQOutlookTenantID, s.RFQOutlookClientID, s.RFQOutlookClientSecret, redirectURI)
		if err != nil {
			fail(err.Error())
			return
		}
		email := fetchOutlookEmail(tok.AccessToken)
		fields = bson.M{
			"settings.rfq_outlook_access_token":  tok.AccessToken,
			"settings.rfq_outlook_refresh_token": tok.RefreshToken,
			"settings.rfq_email_connected":       true,
			"settings.rfq_email_address":         email,
		}
	case "zoho":
		tok, err := exchangeZohoCode(code, s.RFQZohoClientID, s.RFQZohoClientSecret, redirectURI)
		if err != nil {
			fail(err.Error())
			return
		}
		email := fetchZohoEmail(tok.AccessToken)
		fields = bson.M{
			"settings.rfq_zoho_access_token":  tok.AccessToken,
			"settings.rfq_zoho_refresh_token": tok.RefreshToken,
			"settings.rfq_email_connected":    true,
			"settings.rfq_email_address":      email,
		}
	default:
		fail("unknown provider: " + provider)
		return
	}

	if err := rfqEmailSave(storeObjID, fields); err != nil {
		fail("failed to save tokens: " + err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, closeHTML)
}

// HandleRFQEmailWebhook receives inbound email events from Mailgun, SendGrid,
// Postmark, or Amazon SES, and feeds them into the RFQ processing pipeline.
func HandleRFQEmailWebhook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	provider := r.URL.Query().Get("provider")

	store, storeObjID, err := rfqEmailGetStore(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if !store.Settings.RFQEmailConnected || !store.Settings.EnableAIRFQBot {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "rfq email disabled"})
		return
	}

	sender, subject, body := parseInboundEmail(provider, r)
	if sender == "" || body == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "empty email ignored"})
		return
	}

	rfq := &models.RFQReceived{
		StoreID:     storeObjID,
		FromPhone:   sender,
		FromName:    sender,
		MessageType: "text",
		TextContent: fmt.Sprintf("Subject: %s\n\n%s", subject, body),
		Status:      "received",
	}

	if err := models.CreateRFQReceived(rfq); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save RFQ"})
		return
	}

	BroadcastRFQData(storeObjID.Hex(), "rfq_received", map[string]interface{}{
		"id": rfq.ID.Hex(), "from": sender, "source": "email",
	})
	go processRFQ(rfq, storeObjID)

	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "rfq_id": rfq.ID.Hex()})
}

// parseInboundEmail extracts sender, subject, and plain-text body from
// provider-specific payloads.
func parseInboundEmail(provider string, r *http.Request) (sender, subject, body string) {
	switch provider {
	case "mailgun":
		r.ParseMultipartForm(8 << 20)
		sender = r.FormValue("sender")
		subject = r.FormValue("subject")
		body = r.FormValue("stripped-text")
		if body == "" {
			body = r.FormValue("body-plain")
		}
	case "sendgrid":
		r.ParseMultipartForm(8 << 20)
		sender = r.FormValue("from")
		subject = r.FormValue("subject")
		body = r.FormValue("text")
	case "postmark":
		var pm struct {
			From     string `json:"From"`
			Subject  string `json:"Subject"`
			TextBody string `json:"TextBody"`
		}
		rawBody, _ := io.ReadAll(r.Body)
		json.Unmarshal(rawBody, &pm)
		sender, subject, body = pm.From, pm.Subject, pm.TextBody
	case "ses":
		var sns struct {
			Type         string `json:"Type"`
			Message      string `json:"Message"`
			SubscribeURL string `json:"SubscribeURL"`
		}
		rawBody, _ := io.ReadAll(r.Body)
		json.Unmarshal(rawBody, &sns)
		if sns.Type == "SubscriptionConfirmation" && sns.SubscribeURL != "" {
			http.Get(sns.SubscribeURL)
			return
		}
		var sesMsg struct {
			Mail struct {
				Source        string `json:"source"`
				CommonHeaders struct {
					From    []string `json:"from"`
					Subject string   `json:"subject"`
				} `json:"commonHeaders"`
			} `json:"mail"`
			Content string `json:"content"`
		}
		json.Unmarshal([]byte(sns.Message), &sesMsg)
		sender = sesMsg.Mail.Source
		subject = sesMsg.Mail.CommonHeaders.Subject
		body = extractPlainTextFromRFC822(sesMsg.Content)
	default:
		var gen struct {
			From    string `json:"from"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			Text    string `json:"text"`
		}
		rawBody, _ := io.ReadAll(r.Body)
		json.Unmarshal(rawBody, &gen)
		sender = gen.From
		subject = gen.Subject
		body = gen.Body
		if body == "" {
			body = gen.Text
		}
	}
	return strings.TrimSpace(sender), strings.TrimSpace(subject), strings.TrimSpace(body)
}

// extractPlainTextFromRFC822 performs a minimal scan of a raw RFC 2822 email
// looking for the first text/plain section, without a MIME parser dependency.
func extractPlainTextFromRFC822(raw string) string {
	lines := strings.Split(raw, "\n")
	inPlain := false
	var buf strings.Builder
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "content-type: text/plain") {
			inPlain = true
			continue
		}
		if inPlain && strings.HasPrefix(lower, "content-type:") {
			break
		}
		if inPlain {
			buf.WriteString(line + "\n")
		}
	}
	return strings.TrimSpace(buf.String())
}
