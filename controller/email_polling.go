package controller

// email_polling.go — Background poller for OAuth email accounts (Zoho, Gmail, Outlook).
// Called from main.go via go StartEmailPolling().

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const emailPollInterval = 5 * time.Minute

// StartEmailPolling starts the background goroutine that polls all connected
// OAuth email accounts every emailPollInterval.
func StartEmailPolling() {
	log.Printf("email_polling: started (interval %v)", emailPollInterval)
	go func() {
		// Short delay to let the app finish starting up.
		time.Sleep(30 * time.Second)
		for {
			pollAllEmailAccounts()
			time.Sleep(emailPollInterval)
		}
	}()
}

// pollAllEmailAccounts iterates every store with OAuth email accounts and fetches new emails.
func pollAllEmailAccounts() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	proj := options.Find().SetProjection(bson.M{
		"_id": 1,
		"settings.enable_ai_rfq_bot":                     1,
		"settings.rfq_email_accounts":                    1,
		"settings.rfq_llm_provider":                      1,
		"settings.rfq_llm_model":                         1,
		"settings.rfq_llm_api_key":                       1,
		"settings.auto_delete_procurement_messages_days": 1,
	})
	cur, err := col.Find(ctx,
		bson.M{"settings.rfq_email_accounts": bson.M{"$exists": true, "$ne": bson.A{}}},
		proj,
	)
	if err != nil {
		log.Printf("email_polling: failed to list stores: %v", err)
		return
	}
	defer cur.Close(ctx)

	type storeDoc struct {
		ID       primitive.ObjectID   `bson:"_id"`
		Settings models.StoreSettings `bson:"settings"`
	}

	for cur.Next(ctx) {
		var s storeDoc
		if err := cur.Decode(&s); err != nil {
			continue
		}
		for _, acct := range s.Settings.RFQEmailAccounts {
			switch acct.Provider {
			case "zoho":
				if acct.ZohoAccessToken != "" || acct.ZohoRefreshToken != "" {
					go pollZohoAccount(s.ID, s.Settings, acct)
				}
			case "gmail":
				if acct.GmailAccessToken != "" || acct.GmailRefreshToken != "" {
					go pollGmailAccount(s.ID, s.Settings, acct)
				}
			case "outlook":
				if acct.OutlookAccessToken != "" || acct.OutlookRefreshToken != "" {
					go pollOutlookAccount(s.ID, s.Settings, acct)
				}
			}
		}
	}
}

// parsedEmail holds the fields extracted from a fetched email.
type parsedEmail struct {
	from, subject, bodyText string
	to                      []string
	date                    *time.Time // original send/receive time from the mail provider
	attachments             []models.ProcurementAttachment
}

// ─── Zoho polling ─────────────────────────────────────────────────────────────

// zohoMailBase derives the Zoho Mail API base URL from the OAuth accounts server URL.
// Zoho uses region-specific domains: zoho.com (US), zoho.eu (EU), zoho.in (IN), zoho.com.au (AU).
func zohoMailBase(accountsServer string) string {
	for _, suffix := range []string{"zoho.eu", "zoho.in", "zoho.com.au", "zoho.jp"} {
		if strings.Contains(accountsServer, suffix) {
			return "https://mail." + suffix
		}
	}
	return "https://mail.zoho.com"
}

func pollZohoAccount(storeID primitive.ObjectID, settings models.StoreSettings, acct models.RFQEmailAccount) {
	since := time.Now().Add(-24 * time.Hour) // first poll looks back 24h
	if acct.LastPolledAt != nil {
		since = *acct.LastPolledAt
	}

	accessToken, err := ensureZohoToken(storeID, acct)
	if err != nil {
		log.Printf("email_polling: zoho token refresh failed for %s: %v", acct.Email, err)
		return
	}

	mailBase := zohoMailBase(acct.ZohoAccountsServer)
	zohoAccountID, err := fetchZohoAccountID(accessToken, mailBase)
	if err != nil {
		log.Printf("email_polling: failed to get zoho account id for %s (mailBase=%s): %v", acct.Email, mailBase, err)
		return
	}

	msgs, err := listZohoMessages(accessToken, zohoAccountID, since, mailBase, storeID.Hex())
	if err != nil {
		log.Printf("email_polling: failed to list zoho messages for %s: %v", acct.Email, err)
		return
	}

	log.Printf("email_polling: zoho %s: %d new messages since %s", acct.Email, len(msgs), since.Format(time.RFC3339))

	now := time.Now()
	for _, m := range msgs {
		processPolledEmail(storeID, settings, "zoho", m)
	}

	updateAccountLastPolled(storeID, acct.ID, now)
	runAutoDeleteProcurementMessages(storeID, settings.AutoDeleteProcurementMessagesDays)
}

func ensureZohoToken(storeID primitive.ObjectID, acct models.RFQEmailAccount) (string, error) {
	if acct.ZohoRefreshToken == "" {
		return acct.ZohoAccessToken, nil
	}
	accountsServer := acct.ZohoAccountsServer
	if accountsServer == "" {
		accountsServer = "https://accounts.zoho.com"
	}
	if idx := strings.Index(accountsServer, "/oauth"); idx > 0 {
		accountsServer = accountsServer[:idx]
	}

	resp, err := http.PostForm(accountsServer+"/oauth/v2/token", url.Values{
		"refresh_token": {acct.ZohoRefreshToken},
		"client_id":     {acct.ZohoClientID},
		"client_secret": {acct.ZohoClientSecret},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return acct.ZohoAccessToken, err
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&tok) //nolint:errcheck
	if tok.Error != "" {
		return acct.ZohoAccessToken, fmt.Errorf("refresh: %s", tok.Error)
	}
	if tok.AccessToken == "" {
		return acct.ZohoAccessToken, fmt.Errorf("refresh returned empty token")
	}

	rfqEmailAccountUpdate(storeID, acct.ID, bson.M{"zoho_access_token": tok.AccessToken}) //nolint:errcheck
	return tok.AccessToken, nil
}

func fetchZohoAccountID(accessToken, mailBase string) (string, error) {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	req, _ := http.NewRequest("GET", mailBase+"/api/accounts", nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var res struct {
		Data   []struct{ AccountID string `json:"accountId"` } `json:"data"`
		Status struct{ Code int `json:"code"` }               `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&res) //nolint:errcheck
	if len(res.Data) == 0 {
		return "", fmt.Errorf("no zoho accounts returned (api status %d)", res.Status.Code)
	}
	return res.Data[0].AccountID, nil
}

func listZohoMessages(accessToken, accountID string, since time.Time, mailBase, storeIDStr string) ([]parsedEmail, error) {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	// Zoho Mail API: list inbox messages received after `since` (unix ms).
	endpoint := fmt.Sprintf(
		"%s/api/accounts/%s/messages/view?limit=50&start=0&sortorder=false&receivedTime=%d",
		mailBase, accountID, since.UnixMilli(),
	)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var res struct {
		Data []struct {
			MessageID    string `json:"messageId"`
			FolderID     string `json:"folderId"`
			Subject      string `json:"subject"`
			Sender       string `json:"sender"`
			ToAddress    string `json:"toAddress"`
			ReceivedTime int64  `json:"receivedTime"` // unix milliseconds
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("zoho list parse error: %v (body: %.200s)", err, string(raw))
	}

	var result []parsedEmail
	for _, m := range res.Data {
		content := fetchZohoMessageContent(accessToken, accountID, m.FolderID, m.MessageID, mailBase)
		to := []string{}
		if m.ToAddress != "" {
			to = []string{m.ToAddress}
		}
		pe := parsedEmail{
			from:        m.Sender,
			subject:     m.Subject,
			bodyText:    content,
			to:          to,
			attachments: fetchZohoAttachments(accessToken, accountID, m.FolderID, m.MessageID, mailBase, storeIDStr),
		}
		if m.ReceivedTime > 0 {
			t := time.UnixMilli(m.ReceivedTime).UTC()
			pe.date = &t
		}
		result = append(result, pe)
	}
	return result, nil
}

func fetchZohoMessageContent(accessToken, accountID, folderID, messageID, mailBase string) string {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	endpoint := fmt.Sprintf(
		"%s/api/accounts/%s/folders/%s/messages/%s/content",
		mailBase, accountID, folderID, messageID,
	)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var res struct {
		Data struct{ Content string `json:"content"` } `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res) //nolint:errcheck
	content := strings.ReplaceAll(res.Data.Content, "<br>", "\n")
	content = strings.ReplaceAll(content, "<br/>", "\n")
	content = strings.ReplaceAll(content, "<br />", "\n")
	return stripHTMLTags(content)
}

// saveEmailAttachment writes bytes to ./attachments/{storeID}/{msgID}/{filename}
// and returns the relative URL path (served by /attachments/ static route).
func saveEmailAttachment(storeID, msgID, filename string, data []byte) string {
	dir := fmt.Sprintf("./attachments/%s/%s", storeID, msgID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}
	safe := filepath.Base(filename)
	if safe == "" || safe == "." {
		safe = "attachment"
	}
	path := filepath.Join(dir, safe)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}
	return "/attachments/" + storeID + "/" + msgID + "/" + safe
}

// fetchZohoAttachments returns ProcurementAttachment records for a Zoho message.
func fetchZohoAttachments(accessToken, accountID, folderID, messageID, mailBase, storeID string) []models.ProcurementAttachment {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	endpoint := fmt.Sprintf("%s/api/accounts/%s/folders/%s/messages/%s/attachments",
		mailBase, accountID, folderID, messageID)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var res struct {
		Data []struct {
			AttachmentID string `json:"attachmentId"`
			FileName     string `json:"fileName"`
			ContentType  string `json:"contentType"`
			Size         int64  `json:"size"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil
	}

	var atts []models.ProcurementAttachment
	for _, a := range res.Data {
		att := models.ProcurementAttachment{
			Filename:    a.FileName,
			ContentType: a.ContentType,
			Size:        a.Size,
		}
		// Download attachment content
		dlURL := fmt.Sprintf("%s/api/accounts/%s/folders/%s/messages/%s/attachments/%s",
			mailBase, accountID, folderID, messageID, a.AttachmentID)
		dlReq, _ := http.NewRequest("GET", dlURL, nil)
		dlReq.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
		dlResp, err := (&http.Client{Timeout: 30 * time.Second}).Do(dlReq)
		if err == nil && dlResp.StatusCode == 200 {
			data, _ := io.ReadAll(dlResp.Body)
			dlResp.Body.Close()
			if len(data) > 0 {
				att.URL = saveEmailAttachment(storeID, messageID, a.FileName, data)
			}
		} else if dlResp != nil {
			dlResp.Body.Close()
		}
		atts = append(atts, att)
	}
	return atts
}

// ─── Gmail polling ────────────────────────────────────────────────────────────

func pollGmailAccount(storeID primitive.ObjectID, settings models.StoreSettings, acct models.RFQEmailAccount) {
	since := time.Now().Add(-24 * time.Hour)
	if acct.LastPolledAt != nil {
		since = *acct.LastPolledAt
	}

	accessToken, err := ensureGmailToken(storeID, acct)
	if err != nil {
		log.Printf("email_polling: gmail token refresh failed for %s: %v", acct.Email, err)
		return
	}

	msgs, err := listGmailMessages(accessToken, since, storeID.Hex())
	if err != nil {
		log.Printf("email_polling: failed to list gmail messages for %s: %v", acct.Email, err)
		return
	}

	log.Printf("email_polling: gmail %s: %d new messages", acct.Email, len(msgs))
	now := time.Now()
	for _, m := range msgs {
		processPolledEmail(storeID, settings, "gmail", m)
	}
	updateAccountLastPolled(storeID, acct.ID, now)
	runAutoDeleteProcurementMessages(storeID, settings.AutoDeleteProcurementMessagesDays)
}

func ensureGmailToken(storeID primitive.ObjectID, acct models.RFQEmailAccount) (string, error) {
	if acct.GmailRefreshToken == "" {
		return acct.GmailAccessToken, nil
	}
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"client_id":     {acct.GmailClientID},
		"client_secret": {acct.GmailClientSecret},
		"refresh_token": {acct.GmailRefreshToken},
		"grant_type":    {"refresh_token"},
	})
	if err != nil {
		return acct.GmailAccessToken, err
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&tok) //nolint:errcheck
	if tok.Error != "" {
		return acct.GmailAccessToken, fmt.Errorf("gmail refresh: %s", tok.Error)
	}
	if tok.AccessToken == "" {
		return acct.GmailAccessToken, nil
	}
	rfqEmailAccountUpdate(storeID, acct.ID, bson.M{"gmail_access_token": tok.AccessToken}) //nolint:errcheck
	return tok.AccessToken, nil
}

func listGmailMessages(accessToken string, since time.Time, storeIDStr string) ([]parsedEmail, error) {
	afterDate := since.Format("2006/01/02")
	listURL := fmt.Sprintf(
		"https://gmail.googleapis.com/gmail/v1/users/me/messages?q=after:%s+in:inbox&maxResults=20",
		afterDate,
	)
	req, _ := http.NewRequest("GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listRes struct {
		Messages []struct{ ID string `json:"id"` } `json:"messages"`
	}
	json.NewDecoder(resp.Body).Decode(&listRes) //nolint:errcheck

	var result []parsedEmail
	for _, m := range listRes.Messages {
		if pe := fetchGmailMessageContentWithStore(accessToken, m.ID, storeIDStr); pe != nil {
			result = append(result, *pe)
		}
	}
	return result, nil
}

// gmailPart mirrors the recursive Gmail message part structure.
type gmailPart struct {
	PartID   string `json:"partId"`
	MimeType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		AttachmentId string `json:"attachmentId"`
		Size         int    `json:"size"`
		Data         string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

func fetchGmailMessageContent(accessToken, msgID string) *parsedEmail {
	return fetchGmailMessageContentWithStore(accessToken, msgID, "")
}

func fetchGmailMessageContentWithStore(accessToken, msgID, storeID string) *parsedEmail {
	msgURL := fmt.Sprintf(
		"https://gmail.googleapis.com/gmail/v1/users/me/messages/%s?format=full",
		msgID,
	)
	req, _ := http.NewRequest("GET", msgURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var gmsg struct {
		Payload gmailPart `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gmsg); err != nil {
		return nil
	}

	pe := &parsedEmail{}
	for _, h := range gmsg.Payload.Headers {
		switch strings.ToLower(h.Name) {
		case "from":
			pe.from = h.Value
		case "subject":
			pe.subject = h.Value
		case "to":
			pe.to = []string{h.Value}
		case "date":
			if t, err := parseRFC2822Date(h.Value); err == nil {
				pe.date = &t
			}
		}
	}

	// Recursively walk parts to extract body text and attachments.
	var walkParts func(parts []gmailPart)
	walkParts = func(parts []gmailPart) {
		for _, part := range parts {
			if strings.HasPrefix(part.MimeType, "multipart/") {
				walkParts(part.Parts)
				continue
			}
			if strings.HasPrefix(part.MimeType, "text/plain") && part.Body.Data != "" && pe.bodyText == "" {
				pe.bodyText = gmailBase64Decode(part.Body.Data)
				continue
			}
			if part.Filename != "" && part.Body.AttachmentId != "" {
				// It's an attachment — download it.
				att := models.ProcurementAttachment{
					Filename:    part.Filename,
					ContentType: part.MimeType,
					Size:        int64(part.Body.Size),
				}
				if storeID != "" {
					dlURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s/attachments/%s", msgID, part.Body.AttachmentId)
					dlReq, _ := http.NewRequest("GET", dlURL, nil)
					dlReq.Header.Set("Authorization", "Bearer "+accessToken)
					if dlResp, err := (&http.Client{Timeout: 30 * time.Second}).Do(dlReq); err == nil {
						var attBody struct{ Data string `json:"data"` }
						if json.NewDecoder(dlResp.Body).Decode(&attBody) == nil && attBody.Data != "" {
							data := []byte(gmailBase64Decode(attBody.Data))
							att.URL = saveEmailAttachment(storeID, msgID, part.Filename, data)
						}
						dlResp.Body.Close()
					}
				}
				pe.attachments = append(pe.attachments, att)
			}
		}
	}
	walkParts(gmsg.Payload.Parts)

	if pe.bodyText == "" && gmsg.Payload.Body.Data != "" {
		pe.bodyText = gmailBase64Decode(gmsg.Payload.Body.Data)
	}
	return pe
}

// parseRFC2822Date parses an email Date header (RFC 2822 and common variants).
func parseRFC2822Date(s string) (time.Time, error) {
	formats := []string{
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"02 Jan 2006 15:04:05 -0700",
		"02 Jan 2006 15:04:05 MST",
		time.RFC1123Z,
		time.RFC1123,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse date: %q", s)
}

// gmailBase64Decode decodes Gmail's URL-safe base64.
func gmailBase64Decode(s string) string {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// ─── Outlook polling ──────────────────────────────────────────────────────────

func pollOutlookAccount(storeID primitive.ObjectID, settings models.StoreSettings, acct models.RFQEmailAccount) {
	since := time.Now().Add(-24 * time.Hour)
	if acct.LastPolledAt != nil {
		since = *acct.LastPolledAt
	}

	accessToken, err := ensureOutlookToken(storeID, acct)
	if err != nil {
		log.Printf("email_polling: outlook token refresh failed for %s: %v", acct.Email, err)
		return
	}

	msgs, err := listOutlookMessages(accessToken, since)
	if err != nil {
		log.Printf("email_polling: failed to list outlook messages for %s: %v", acct.Email, err)
		return
	}

	log.Printf("email_polling: outlook %s: %d new messages", acct.Email, len(msgs))
	now := time.Now()
	for _, m := range msgs {
		processPolledEmail(storeID, settings, "outlook", m)
	}
	updateAccountLastPolled(storeID, acct.ID, now)
	runAutoDeleteProcurementMessages(storeID, settings.AutoDeleteProcurementMessagesDays)
}

func ensureOutlookToken(storeID primitive.ObjectID, acct models.RFQEmailAccount) (string, error) {
	if acct.OutlookRefreshToken == "" {
		return acct.OutlookAccessToken, nil
	}
	tenantID := acct.OutlookTenantID
	if tenantID == "" {
		tenantID = "common"
	}
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	resp, err := http.PostForm(tokenURL, url.Values{
		"client_id":     {acct.OutlookClientID},
		"client_secret": {acct.OutlookClientSecret},
		"refresh_token": {acct.OutlookRefreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {"https://graph.microsoft.com/Mail.Read offline_access"},
	})
	if err != nil {
		return acct.OutlookAccessToken, err
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&tok) //nolint:errcheck
	if tok.Error != "" {
		return acct.OutlookAccessToken, fmt.Errorf("outlook refresh: %s", tok.Error)
	}
	if tok.AccessToken == "" {
		return acct.OutlookAccessToken, nil
	}
	rfqEmailAccountUpdate(storeID, acct.ID, bson.M{"outlook_access_token": tok.AccessToken}) //nolint:errcheck
	return tok.AccessToken, nil
}

func listOutlookMessages(accessToken string, since time.Time) ([]parsedEmail, error) {
	sinceStr := url.QueryEscape(since.UTC().Format("2006-01-02T15:04:05Z"))
	msURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/me/mailFolders/Inbox/messages?$filter=receivedDateTime ge %s&$select=subject,from,toRecipients,body,receivedDateTime&$top=20&$orderby=receivedDateTime desc",
		sinceStr,
	)
	req, _ := http.NewRequest("GET", msURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Prefer", `outlook.body-content-type="text"`)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var res struct {
		Value []struct {
			Subject          string `json:"subject"`
			ReceivedDateTime string `json:"receivedDateTime"` // RFC3339
			From             struct {
				EmailAddress struct {
					Address string `json:"address"`
					Name    string `json:"name"`
				} `json:"emailAddress"`
			} `json:"from"`
			ToRecipients []struct {
				EmailAddress struct{ Address string `json:"address"` } `json:"emailAddress"`
			} `json:"toRecipients"`
			Body struct{ Content string `json:"content"` } `json:"body"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var result []parsedEmail
	for _, m := range res.Value {
		from := m.From.EmailAddress.Address
		if m.From.EmailAddress.Name != "" {
			from = m.From.EmailAddress.Name + " <" + from + ">"
		}
		to := make([]string, 0, len(m.ToRecipients))
		for _, r := range m.ToRecipients {
			to = append(to, r.EmailAddress.Address)
		}
		pe := parsedEmail{
			from:     from,
			subject:  m.Subject,
			bodyText: m.Body.Content,
			to:       to,
		}
		if m.ReceivedDateTime != "" {
			if t, err := time.Parse(time.RFC3339, m.ReceivedDateTime); err == nil {
				t = t.UTC()
				pe.date = &t
			}
		}
		result = append(result, pe)
	}
	return result, nil
}

// ─── Shared: process a polled email through the RFQ pipeline ──────────────────

// isReminderEmail asks the LLM whether a new email is a reminder or follow-up for
// one of the recent RFQs already recorded, rather than a fresh request. Returns true
// when the email should NOT create a new RFQ.
func isReminderEmail(store *models.Store, fromEmail, subject, bodyText string) bool {
	recent, err := models.FindRecentRFQsByEmail(store.ID, fromEmail, 30*24*time.Hour, 5)
	if err != nil || len(recent) == 0 {
		return false // no recent RFQs to compare against — treat as new
	}

	apiKey := store.Settings.RFQLLMAPIKey
	llmModel := store.Settings.RFQLLMModel
	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	if apiKey == "" || provider == "" {
		return false
	}

	var sb strings.Builder
	sb.WriteString("You are a procurement email classifier.\n\n")
	sb.WriteString("The customer sent this new email:\n")
	sb.WriteString(fmt.Sprintf("Subject: %s\n\n%s\n\n", subject, bodyText))
	sb.WriteString("---\n")
	sb.WriteString("The same customer already has these recent RFQs on record:\n")
	for i, r := range recent {
		sb.WriteString(fmt.Sprintf("%d. Code: %s | Date: %s | Items: %s\n",
			i+1, r.Code, r.ReceivedAt.Format("2006-01-02"), r.TextContent))
	}
	sb.WriteString("\n---\n")
	sb.WriteString("Is the new email a REMINDER or FOLLOW-UP for one of the existing RFQs above? " +
		"Reply with ONLY \"yes\" (it is a reminder) or \"no\" (it is a new, distinct request).")

	answer, err := callLLMText(apiKey, llmModel, sb.String(), provider)
	if err != nil {
		log.Printf("email_polling: isReminderEmail LLM error: %v — treating as new RFQ", err)
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "yes")
}

// processPolledEmail saves the message to the procurement log and, if AI RFQ bot
// is enabled, runs LLM classification + extraction and creates an RFQ record.
func processPolledEmail(storeID primitive.ObjectID, settings models.StoreSettings, provider string, msg parsedEmail) {
	if msg.from == "" && msg.bodyText == "" {
		return
	}

	emailText := fmt.Sprintf("Subject: %s\n\n%s", msg.subject, msg.bodyText)

	store, storeObjID, err := rfqEmailGetStore(storeID.Hex())
	if err != nil {
		log.Printf("email_polling: store not found %s: %v", storeID.Hex(), err)
		return
	}

	// Always record in procurement log first (synchronous so we can link RFQ ↔ message).
	procMsg := saveProcurementEmailMessage(storeObjID, "in", provider, msg.from, msg.to, msg.subject, msg.bodyText, msg.attachments, false, nil, msg.date)

	if !settings.EnableAIRFQBot || settings.RFQLLMAPIKey == "" {
		return
	}

	// LLM classification — is it an RFQ at all?
	if !isRFQMessage(store, emailText, nil) {
		log.Printf("email_polling: email from %s is not an RFQ — skipping", msg.from)
		return
	}

	// LLM duplicate check — is it a reminder for an existing RFQ?
	if isReminderEmail(store, msg.from, msg.subject, msg.bodyText) {
		log.Printf("email_polling: email from %s detected as reminder/follow-up — not creating new RFQ", msg.from)
		return
	}

	// LLM extraction
	var extracted rfqExtractResult
	llmProvider := strings.ToLower(settings.RFQLLMProvider)
	raw, llmErr := callLLMExtractRFQ(settings.RFQLLMAPIKey, settings.RFQLLMModel, llmProvider, emailText, nil, nil)
	if llmErr == nil {
		jsonStr := extractJSONFromLLMResponse(raw)
		json.Unmarshal([]byte(jsonStr), &extracted) //nolint:errcheck
	}

	// Customer find-or-create: use extracted info, fall back to email sender address.
	lookupEmail := extracted.CustomerEmail
	if lookupEmail == "" {
		lookupEmail = msg.from
	}
	var customerID *primitive.ObjectID
	if customer, err := store.FindOrCreateCustomerFromRFQ(
		extracted.CustomerName, lookupEmail, extracted.CustomerPhone,
		extracted.CustomerVATNo, extracted.CustomerCompany,
	); err != nil {
		log.Printf("email_polling: FindOrCreateCustomerFromRFQ error: %v", err)
	} else if customer != nil {
		customerID = &customer.ID
	}

	fromName := extracted.CustomerName
	if fromName == "" {
		fromName = msg.from
	}
	var products []models.RFQProduct
	for _, p := range extracted.Products {
		products = append(products, models.RFQProduct{
			Name:     p.Name,
			PartNo:   p.PartNo,
			Quantity: p.Quantity,
			Unit:     p.Unit,
		})
	}

	rfq := &models.RFQReceived{
		StoreID:         storeObjID,
		FromPhone:       msg.from,
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
	if procMsg != nil {
		rfq.ProcurementMessageID = &procMsg.ID
		rfq.ProcurementMessageCode = procMsg.Code
	}
	if err := models.CreateRFQReceived(rfq); err != nil {
		log.Printf("email_polling: failed to save RFQ from %s: %v", msg.from, err)
		return
	}

	models.AppendRFQLog(storeObjID, rfq.ID, models.RFQActivityLog{
		Step:    "input_received",
		Message: fmt.Sprintf("RFQ received via email poll (%s) from %s", provider, msg.from),
		Icon:    "bi-envelope-fill", Color: "primary",
		Details: map[string]interface{}{
			"source":   "email",
			"from":     msg.from,
			"subject":  msg.subject,
			"provider": provider,
		},
	})

	BroadcastRFQData(storeObjID.Hex(), "rfq_received", map[string]interface{}{
		"id": rfq.ID.Hex(), "from": msg.from, "source": "email",
	})
	go autoCategorizeAndFindSuppliers(rfq, storeObjID)
	log.Printf("email_polling: created RFQ %s from email %s (provider: %s)", rfq.ID.Hex(), msg.from, provider)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func updateAccountLastPolled(storeID, accountID primitive.ObjectID, t time.Time) {
	rfqEmailAccountUpdate(storeID, accountID, bson.M{"last_polled_at": t}) //nolint:errcheck
}

// stripHTMLTags removes HTML tags from content returned by Zoho.
func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
