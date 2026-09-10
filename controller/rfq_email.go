package controller

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

func exchangeZohoCode(code, clientID, clientSecret, redirectURI, accountsServer string) (*rfqOAuthToken, error) {
	if accountsServer == "" {
		accountsServer = "https://accounts.zoho.com"
	}
	resp, err := http.PostForm(accountsServer+"/oauth/v2/token", url.Values{
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

func fetchZohoEmail(accessToken, accountsServer string) string {
	mailBase := zohoMailBase(accountsServer)
	req, _ := http.NewRequest("GET", mailBase+"/api/accounts", nil)
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
	zohoAccountsServer := r.URL.Query().Get("accounts-server")

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
	decoded := string(raw)

	// New multi-account format: storeID:acct:accountID
	// Legacy single-account format: storeID:provider
	triParts := strings.SplitN(decoded, ":", 3)
	if len(triParts) == 3 && triParts[1] == "acct" {
		// ── Multi-account OAuth callback ──────────────────────────────
		storeID := triParts[0]
		accountIDHex := triParts[2]
		accountID, err := primitive.ObjectIDFromHex(accountIDHex)
		if err != nil {
			fail("invalid account id in state")
			return
		}
		store, storeObjID, err := rfqEmailGetStore(storeID)
		if err != nil {
			fail("store not found")
			return
		}
		// Find the draft account
		var acct *models.RFQEmailAccount
		for i := range store.Settings.RFQEmailAccounts {
			if store.Settings.RFQEmailAccounts[i].ID == accountID {
				acct = &store.Settings.RFQEmailAccounts[i]
				break
			}
		}
		if acct == nil {
			ids := make([]string, len(store.Settings.RFQEmailAccounts))
			for i, a := range store.Settings.RFQEmailAccounts {
				ids[i] = a.ID.Hex()
			}
			log.Printf("rfq_email oauth callback: account %s not found in store %s (have %d accounts: %v)", accountIDHex, storeID, len(ids), ids)
			fail("account not found")
			return
		}
		redirectURI := acct.OAuthCallbackURL
		if redirectURI == "" {
			redirectURI = rfqEmailOAuthCallbackURL()
		}
		var email, accessToken, refreshToken string
		switch acct.Provider {
		case "gmail":
			tok, err := exchangeGmailCode(code, acct.GmailClientID, acct.GmailClientSecret, redirectURI)
			if err != nil {
				fail(err.Error())
				return
			}
			email = fetchGmailEmail(tok.AccessToken)
			accessToken, refreshToken = tok.AccessToken, tok.RefreshToken
			rfqEmailAccountUpdate(storeObjID, accountID, bson.M{
				"gmail_access_token":  accessToken,
				"gmail_refresh_token": refreshToken,
				"email":               email,
			})
		case "outlook":
			tok, err := exchangeOutlookCode(code, acct.OutlookTenantID, acct.OutlookClientID, acct.OutlookClientSecret, redirectURI)
			if err != nil {
				fail(err.Error())
				return
			}
			email = fetchOutlookEmail(tok.AccessToken)
			accessToken, refreshToken = tok.AccessToken, tok.RefreshToken
			rfqEmailAccountUpdate(storeObjID, accountID, bson.M{
				"outlook_access_token":  accessToken,
				"outlook_refresh_token": refreshToken,
				"email":                 email,
			})
		case "zoho":
			tok, err := exchangeZohoCode(code, acct.ZohoClientID, acct.ZohoClientSecret, redirectURI, zohoAccountsServer)
			if err != nil {
				fail(err.Error())
				return
			}
			email = fetchZohoEmail(tok.AccessToken, zohoAccountsServer)
			accessToken, refreshToken = tok.AccessToken, tok.RefreshToken
			rfqEmailAccountUpdate(storeObjID, accountID, bson.M{
				"zoho_access_token":    accessToken,
				"zoho_refresh_token":   refreshToken,
				"zoho_accounts_server": zohoAccountsServer,
				"email":                email,
			})
		default:
			fail("unknown provider: " + acct.Provider)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, closeHTML)
		return
	}

	// ── Legacy single-account OAuth callback ──────────────────────────────────
	parts := strings.SplitN(decoded, ":", 2)
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
		tok, err := exchangeZohoCode(code, s.RFQZohoClientID, s.RFQZohoClientSecret, redirectURI, zohoAccountsServer)
		if err != nil {
			fail(err.Error())
			return
		}
		email := fetchZohoEmail(tok.AccessToken, zohoAccountsServer)
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
// Postmark, Amazon SES, or IMAP, analyses the content with the store's LLM to
// detect whether it is an RFQ, extracts products and customer information, links
// an existing customer when found, and creates a ready_to_send RFQ record.
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

	// Always log every inbound email regardless of RFQ classification.
	go func() { saveProcurementEmailMessage(storeObjID, "in", provider, sender, nil, subject, body, nil, false, nil, nil) }()
	go runAutoDeleteProcurementMessages(storeObjID, store.Settings.AutoDeleteProcurementMessagesDays)

	emailText := fmt.Sprintf("Subject: %s\n\n%s", subject, body)

	// ── Step 1: LLM classification — is this email an RFQ? ──────────────────
	if store.Settings.RFQLLMAPIKey != "" {
		if !isRFQMessage(store, emailText, nil) {
			log.Printf("rfq_email: email from %s is not an RFQ — ignoring", sender)
			json.NewEncoder(w).Encode(map[string]string{"status": "not an RFQ"})
			return
		}
	}

	// ── Step 2: LLM extraction — products + customer info ───────────────────
	var extracted rfqExtractResult
	if store.Settings.RFQLLMAPIKey != "" {
		llmProvider := strings.ToLower(store.Settings.RFQLLMProvider)
		raw, llmErr := callLLMExtractRFQ(
			store.Settings.RFQLLMAPIKey,
			store.Settings.RFQLLMModel,
			llmProvider,
			emailText, nil, nil,
		)
		if llmErr != nil {
			log.Printf("rfq_email: LLM extraction error for email from %s: %v", sender, llmErr)
		} else {
			jsonStr := extractJSONFromLLMResponse(raw)
			if err2 := json.Unmarshal([]byte(jsonStr), &extracted); err2 != nil {
				log.Printf("rfq_email: LLM JSON parse error: %v (raw: %s)", err2, raw)
			}
		}
	}

	// ── Step 3: Customer lookup by email, phone, or VAT number ──────────────
	// Prefer the email address from the extraction; fall back to the sender address.
	lookupEmail := extracted.CustomerEmail
	if lookupEmail == "" {
		lookupEmail = sender
	}
	var customerID *primitive.ObjectID
	if customer, _ := store.FindCustomerByEmailOrPhone(lookupEmail, extracted.CustomerPhone, bson.M{}); customer != nil {
		customerID = &customer.ID
		log.Printf("rfq_email: matched email to existing customer %s (%s)", customer.Name, customer.ID.Hex())
	}

	// ── Step 4: Build and persist the RFQ ───────────────────────────────────
	var products []models.RFQProduct
	for _, p := range extracted.Products {
		products = append(products, models.RFQProduct{
			Name:     p.Name,
			PartNo:   p.PartNo,
			Quantity: p.Quantity,
			Unit:     p.Unit,
		})
	}

	fromName := extracted.CustomerName
	if fromName == "" {
		fromName = sender
	}

	rfq := &models.RFQReceived{
		StoreID:         storeObjID,
		FromPhone:       sender, // email address used as sender identifier
		FromName:        fromName,
		MessageType:     "text",
		TextContent:     emailText,
		Source:          "email",
		Status:          "ready_to_send",
		Products:        products,
		CustomerID:      customerID,
		CustomerName:    extracted.CustomerName,
		CustomerPhone:   extracted.CustomerPhone,
		CustomerEmail:   extracted.CustomerEmail,
		CustomerCompany: extracted.CustomerCompany,
	}

	if err := models.CreateRFQReceived(rfq); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save RFQ"})
		return
	}

	// ── Step 5: Activity log ─────────────────────────────────────────────────
	customerLabel := fromName
	models.AppendRFQLog(storeObjID, rfq.ID, models.RFQActivityLog{
		Step:    "input_received",
		Message: fmt.Sprintf("RFQ received via email from %s (subject: %s)", customerLabel, subject),
		Icon:    "bi-envelope-fill", Color: "primary",
		Details: map[string]interface{}{
			"source":          "email",
			"from":            sender,
			"subject":         subject,
			"products_count":  len(products),
			"customer_linked": customerID != nil,
		},
	})
	if customerID != nil {
		models.AppendRFQLog(storeObjID, rfq.ID, models.RFQActivityLog{
			Step:    "customer_linked",
			Message: fmt.Sprintf("Linked to existing customer: %s", extracted.CustomerName),
			Icon:    "bi-person-check", Color: "success",
			Details: map[string]interface{}{"customer_id": customerID.Hex(), "customer_name": extracted.CustomerName},
		})
	}

	BroadcastRFQData(storeObjID.Hex(), "rfq_received", map[string]interface{}{
		"id": rfq.ID.Hex(), "from": sender, "source": "email",
	})

	// ── Step 6: Background — identify categories and find suppliers ──────────
	go autoCategorizeAndFindSuppliers(rfq, storeObjID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "ok",
		"rfq_id":            rfq.ID.Hex(),
		"products_extracted": len(products),
		"customer_linked":   customerID != nil,
	})
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

// ── Multi-account helpers ─────────────────────────────────────────────────────

// oauthStateAccount encodes a state token for the multi-account OAuth flow.
func oauthStateAccount(storeID string, accountID primitive.ObjectID) string {
	return base64.URLEncoding.EncodeToString([]byte(storeID + ":acct:" + accountID.Hex()))
}

// rfqEmailWebhookURLForAccount returns the webhook URL scoped to a specific account.
func rfqEmailWebhookURLForAccount(storeID string, accountID primitive.ObjectID) string {
	return fmt.Sprintf("%s/v1/rfq-email/webhook?store_id=%s&account_id=%s",
		rfqEmailAPIBase(), storeID, accountID.Hex())
}

// rfqEmailAccountUpdate updates fields on a single element inside rfq_email_accounts
// using MongoDB's positional filtered operator ($[elem]).
func rfqEmailAccountUpdate(storeObjID, accountID primitive.ObjectID, fields bson.M) error {
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	setDoc := bson.M{}
	for k, v := range fields {
		setDoc["settings.rfq_email_accounts.$[elem]."+k] = v
	}
	_, err := col.UpdateOne(ctx,
		bson.M{"_id": storeObjID},
		bson.M{"$set": setDoc},
		options.Update().SetArrayFilters(options.ArrayFilters{
			Filters: []interface{}{bson.M{"elem._id": accountID}},
		}),
	)
	return err
}

// rfqEmailAccountRequest is the body for ConnectRFQEmailAccount.
type rfqEmailAccountRequest struct {
	StoreID     string `json:"store_id"`
	Provider    string `json:"provider"`
	CallbackURL string `json:"callback_url"` // optional: frontend passes its own origin-based URL
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
	// AWS SES
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

// rfqEmailSafePushAccount appends acct to settings.rfq_email_accounts.
// It first converts a null field to [] so $push never hits the
// "field must be an array but is of type null" error.
func rfqEmailSafePushAccount(col *mongo.Collection, storeObjID primitive.ObjectID, acct interface{}) error {
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		col.UpdateOne(ctx,
			bson.M{"_id": storeObjID, "settings.rfq_email_accounts": nil},
			bson.M{"$set": bson.M{"settings.rfq_email_accounts": bson.A{}}})
		cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := col.UpdateOne(ctx, bson.M{"_id": storeObjID},
		bson.M{"$push": bson.M{"settings.rfq_email_accounts": acct}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("store not found")
	}
	return nil
}

// ConnectRFQEmailAccount creates a new email account entry and (for OAuth providers)
// returns an OAuth URL. Webhook/IMAP providers are marked connected immediately.
func ConnectRFQEmailAccount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req rfqEmailAccountRequest
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

	accountID := primitive.NewObjectID()
	redirectURI := req.CallbackURL
	if redirectURI == "" {
		redirectURI = rfqEmailOAuthCallbackURL()
	}
	state := oauthStateAccount(req.StoreID, accountID)
	webhookURL := rfqEmailWebhookURLForAccount(req.StoreID, accountID)

	col := db.Client("").Database(db.GetPosDB()).Collection("store")

	switch req.Provider {

	case "gmail":
		if req.GmailClientID == "" || req.GmailClientSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Client ID and Client Secret are required"})
			return
		}
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "gmail",
			GmailClientID: req.GmailClientID, GmailClientSecret: req.GmailClientSecret,
			OAuthCallbackURL: redirectURI,
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect gmail: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		log.Printf("rfq_email connect gmail: saved account %s to store %s", accountID.Hex(), req.StoreID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id": accountID.Hex(),
			"oauth_url":  gmailOAuthURL(req.GmailClientID, state, redirectURI),
		})

	case "outlook":
		if req.OutlookClientID == "" || req.OutlookClientSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Client ID and Client Secret are required"})
			return
		}
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "outlook",
			OutlookTenantID: req.OutlookTenantID, OutlookClientID: req.OutlookClientID,
			OutlookClientSecret: req.OutlookClientSecret,
			OAuthCallbackURL: redirectURI,
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect outlook: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		log.Printf("rfq_email connect outlook: saved account %s to store %s", accountID.Hex(), req.StoreID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id": accountID.Hex(),
			"oauth_url":  outlookOAuthURL(req.OutlookTenantID, req.OutlookClientID, state, redirectURI),
		})

	case "zoho":
		if req.ZohoClientID == "" || req.ZohoClientSecret == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Client ID and Client Secret are required"})
			return
		}
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "zoho",
			ZohoClientID: req.ZohoClientID, ZohoClientSecret: req.ZohoClientSecret,
			OAuthCallbackURL: redirectURI,
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect zoho: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		log.Printf("rfq_email connect zoho: saved account %s to store %s", accountID.Hex(), req.StoreID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id": accountID.Hex(),
			"oauth_url":  zohoOAuthURL(req.ZohoClientID, state, redirectURI),
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
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "mailgun",
			MailgunAPIKey: req.MailgunAPIKey, MailgunDomain: req.MailgunDomain,
			Email: "webhook@" + req.MailgunDomain,
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect mailgun: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id":  accountID.Hex(),
			"connected":   true,
			"email":       acct.Email,
			"webhook_url": webhookURL,
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
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "sendgrid",
			SendGridAPIKey: req.SendGridAPIKey, Email: "sendgrid-inbound",
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect sendgrid: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id":  accountID.Hex(),
			"connected":   true,
			"email":       acct.Email,
			"webhook_url": webhookURL,
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
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "postmark",
			PostmarkServerToken: req.PostmarkServerToken, Email: "postmark-inbound",
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect postmark: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id":  accountID.Hex(),
			"connected":   true,
			"email":       acct.Email,
			"webhook_url": webhookURL,
		})

	case "ses":
		if req.AWSSESAccessKeyID == "" || req.AWSSESSecretKey == "" || req.AWSSESRegion == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Access Key ID, Secret, and Region are required"})
			return
		}
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "ses",
			AWSSESAccessKeyID: req.AWSSESAccessKeyID, AWSSESSecretKey: req.AWSSESSecretKey,
			AWSSESRegion: req.AWSSESRegion, Email: "ses-" + req.AWSSESRegion,
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect ses: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id":  accountID.Hex(),
			"connected":   true,
			"email":       acct.Email,
			"webhook_url": webhookURL,
		})

	case "imap":
		if req.IMAPHost == "" || req.IMAPUsername == "" || req.IMAPPassword == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Host, Username, and Password are required"})
			return
		}
		if req.IMAPPort == 0 {
			req.IMAPPort = 993
		}
		if err := imapTestLogin(req.IMAPHost, req.IMAPPort, req.IMAPUseSSL, req.IMAPUsername, req.IMAPPassword); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "IMAP connection failed: " + err.Error()})
			return
		}
		acct := models.RFQEmailAccount{
			ID: accountID, Provider: "imap",
			IMAPHost: req.IMAPHost, IMAPPort: req.IMAPPort,
			IMAPUsername: req.IMAPUsername, IMAPPassword: req.IMAPPassword,
			IMAPUseSSL: req.IMAPUseSSL, Email: req.IMAPUsername,
		}
		if pushErr := rfqEmailSafePushAccount(col, storeObjID, acct); pushErr != nil {
			log.Printf("rfq_email connect imap: failed to save account %s to store %s: %v", accountID.Hex(), req.StoreID, pushErr)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to save account — please try again"})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"account_id": accountID.Hex(),
			"connected":  true,
			"email":      acct.Email,
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unknown provider: " + req.Provider})
	}
}

// GetRFQEmailAccounts returns the list of connected email accounts (without secrets).
func GetRFQEmailAccounts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	store, _, err := rfqEmailGetStore(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	accounts := store.Settings.RFQEmailAccounts
	if accounts == nil {
		accounts = []models.RFQEmailAccount{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"accounts": accounts})
}

// DisconnectRFQEmailAccount removes a single email account from the store's account list.
func DisconnectRFQEmailAccount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	accountIDHex := vars["accountID"]
	accountID, err := primitive.ObjectIDFromHex(accountIDHex)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid account id"})
		return
	}
	_, storeObjID, err := rfqEmailGetStore(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = col.UpdateOne(ctx,
		bson.M{"_id": storeObjID},
		bson.M{"$pull": bson.M{"settings.rfq_email_accounts": bson.M{"_id": accountID}}},
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to remove account"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// PollRFQEmailAccountStatus is called by the frontend after OAuth to check if the account
// now has an email address (i.e. OAuth callback completed).
func PollRFQEmailAccountStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	accountIDHex := vars["accountID"]
	accountID, err := primitive.ObjectIDFromHex(accountIDHex)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid account id"})
		return
	}
	store, _, err := rfqEmailGetStore(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	for _, acct := range store.Settings.RFQEmailAccounts {
		if acct.ID == accountID {
			// Consider connected if we have OAuth tokens OR an email address.
			// Email fetch may fail (scope/API) but tokens are always set after a
			// successful OAuth exchange, so token presence is the reliable signal.
			hasTokens := acct.GmailAccessToken != "" || acct.ZohoAccessToken != "" || acct.OutlookAccessToken != ""
			json.NewEncoder(w).Encode(map[string]interface{}{
				"connected": acct.Email != "" || hasTokens,
				"email":     acct.Email,
				"provider":  acct.Provider,
			})
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "account not found"})
}
