package controller

// email_polling.go — Background poller for OAuth email accounts (Zoho, Gmail, Outlook).
// Called from main.go via go StartEmailPolling().

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"html"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const emailPollInterval = 5 * time.Minute

// IMAP rate limiter — prevents Zoho account blocking from rapid connection attempts.
var (
	imapRateMu       sync.Mutex
	imapLastConnect  time.Time
	imapBlockedUntil time.Time
)

const (
	imapMinConnInterval = 5 * time.Second
	imapBlockBackoff    = 45 * time.Minute
)

// imapAcquire serializes IMAP connections and enforces minimum spacing.
// Returns false if the account is currently in a block backoff period.
// Sleeps while holding the mutex to guarantee only one connection at a time.
func imapAcquire() bool {
	imapRateMu.Lock()
	defer imapRateMu.Unlock()
	now := time.Now()
	if now.Before(imapBlockedUntil) {
		log.Printf("imap: account in block backoff until %v — skipping", imapBlockedUntil.Format(time.RFC3339))
		return false
	}
	if wait := imapLastConnect.Add(imapMinConnInterval).Sub(now); wait > 0 {
		time.Sleep(wait)
	}
	imapLastConnect = time.Now()
	return true
}

// imapSetBlocked records that Zoho blocked this account and pauses all IMAP for imapBlockBackoff.
func imapSetBlocked() {
	imapRateMu.Lock()
	imapBlockedUntil = time.Now().Add(imapBlockBackoff)
	imapRateMu.Unlock()
	log.Printf("imap: account blocked — pausing all IMAP connections for %v", imapBlockBackoff)
}

// zohoFlexBool handles Zoho's inconsistent hasAttachment field which may be
// a JSON boolean (true/false) or a JSON string ("true"/"false").
type zohoFlexBool bool

func (f *zohoFlexBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	*f = zohoFlexBool(s == "true" || s == "1")
	return nil
}

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

// TriggerEmailSyncHandler is a POST endpoint that immediately runs email polling
// for a single store, rather than waiting for the next scheduled interval.
// POST /v1/email-accounts/sync?store_id=...
func TriggerEmailSyncHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	proj := options.FindOne().SetProjection(bson.M{
		"_id": 1,
		"settings.enable_ai_rfq_bot":                     1,
		"settings.rfq_email_accounts":                    1,
		"settings.rfq_llm_provider":                      1,
		"settings.rfq_llm_model":                         1,
		"settings.rfq_llm_api_key":                       1,
		"settings.auto_delete_procurement_messages_days": 1,
	})
	var s struct {
		ID       primitive.ObjectID   `bson:"_id"`
		Settings models.StoreSettings `bson:"settings"`
	}
	if err := col.FindOne(ctx, bson.M{"_id": storeObjID}, proj).Decode(&s); err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}

	go func() {
		for _, acct := range s.Settings.RFQEmailAccounts {
			// Manual sync: always look back 24h so the user sees recent emails
			// regardless of when the last automatic poll ran. Deduplication by
			// external_id prevents re-saving emails already in the DB.
			acctCopy := acct
			acctCopy.LastPolledAt = nil
			switch acctCopy.Provider {
			case "zoho":
				if acctCopy.ZohoAccessToken != "" || acctCopy.ZohoRefreshToken != "" {
					pollZohoAccount(s.ID, s.Settings, acctCopy)
				}
			case "gmail":
				if acctCopy.GmailAccessToken != "" || acctCopy.GmailRefreshToken != "" {
					pollGmailAccount(s.ID, s.Settings, acctCopy)
				}
			case "outlook":
				if acctCopy.OutlookAccessToken != "" || acctCopy.OutlookRefreshToken != "" {
					pollOutlookAccount(s.ID, s.Settings, acctCopy)
				}
			}
		}
	}()

	w.Write([]byte(`{"status":"sync started"}`)) //nolint:errcheck
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
	from, subject, bodyText, bodyHTML, externalID string
	messageID                                     string // RFC 2822 Message-ID header, for reply threading
	inReplyTo                                     string // RFC 2822 In-Reply-To header (set when this is a reply)
	to                                            []string
	date                                          *time.Time // original send/receive time from the mail provider
	attachments                                   []models.ProcurementAttachment
	hasZohoAttachment                             bool   // set when Zoho reports hasAttachment=true (even if download failed)
	zohoFolderID                                  string // folder where this Zoho message resides (for attachment API)
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
	since := time.Now().UTC().Add(-7 * 24 * time.Hour) // no prior poll → look back 7 days in UTC
	if acct.LastPolledAt != nil {
		since = acct.LastPolledAt.UTC()
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

	// Phase 1: list messages (body only, no attachments) — fast, emails saved to DB immediately.
	msgs, err := listZohoMessages(accessToken, zohoAccountID, since, mailBase)
	if err != nil {
		log.Printf("email_polling: failed to list zoho messages for %s: %v", acct.Email, err)
		return
	}

	log.Printf("email_polling: zoho %s: %d new messages since %s", acct.Email, len(msgs), since.Format(time.RFC3339))

	now := time.Now()

	type savedEntry struct {
		extID    string
		folderID string
		subject  string
		dbID     primitive.ObjectID
	}
	var savedEntries []savedEntry
	for _, m := range msgs {
		pm := processPolledEmail(storeID, settings, "zoho", m)
		if pm != nil && m.hasZohoAttachment {
			savedEntries = append(savedEntries, savedEntry{m.externalID, m.zohoFolderID, m.subject, pm.ID})
		}
	}

	// Phase 2: fetch attachments for messages that need them and update DB records.
	// Emails are already visible in the UI; attachments arrive asynchronously.
	if len(savedEntries) > 0 {
		inboxFolderID := fetchZohoInboxFolderID(accessToken, zohoAccountID, mailBase)
		for _, e := range savedEntries {
			candidateFolders := []string{}
			if e.folderID != "" {
				candidateFolders = append(candidateFolders, e.folderID)
			}
			if inboxFolderID != "" && inboxFolderID != e.folderID {
				candidateFolders = append(candidateFolders, inboxFolderID)
			}
			candidateFolders = append(candidateFolders, "") // no-folder fallback
			atts := fetchZohoAttachments(accessToken, zohoAccountID, candidateFolders, e.extID, mailBase,
				storeID.Hex(), acct.IMAPHost, acct.IMAPUsername, acct.IMAPPassword, e.subject)
			log.Printf("email_polling: zoho msg %s hasAttachment=true → fetched %d attachment(s)", e.extID, len(atts))
			if len(atts) > 0 {
				if err := models.UpdateProcurementMessageAttachments(e.dbID, atts); err != nil {
					log.Printf("email_polling: failed to update attachments for msg %s: %v", e.extID, err)
				}
			}
		}
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

// fetchZohoInboxFolderID returns the real folder ID of the Inbox by querying
// the account's folder list. The virtual folderId returned by messages/view is
// not accepted by the attachment API, so we need the actual inbox folder ID.
func fetchZohoInboxFolderID(accessToken, accountID, mailBase string) string {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	req, _ := http.NewRequest("GET", mailBase+"/api/accounts/"+accountID+"/folders", nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		log.Printf("email_polling: zoho folder list error: %v", err)
		return ""
	}
	defer resp.Body.Close()
	var res struct {
		Data []struct {
			FolderID string `json:"folderId"`
			Name     string `json:"folderName"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}
	for _, f := range res.Data {
		if strings.EqualFold(f.Name, "inbox") {
			log.Printf("email_polling: zoho inbox folder id = %s", f.FolderID)
			return f.FolderID
		}
	}
	return ""
}

// fetchZohoMessageFolderID returns the folder ID where a specific Zoho message
// currently resides by querying the message view API with the messageId filter.
func fetchZohoMessageFolderID(accessToken, accountID, messageID, mailBase string) string {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	endpoint := fmt.Sprintf("%s/api/accounts/%s/folders/%s/messages/%s",
		mailBase, accountID, "0", messageID)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()
	var res struct {
		Data struct {
			FolderID string `json:"folderId"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res) //nolint:errcheck
	return res.Data.FolderID
}

func listZohoMessages(accessToken, accountID string, since time.Time, mailBase string) ([]parsedEmail, error) {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}

	const pageSize = 50
	const maxPages = 20 // cap at 1000 emails per poll to avoid runaway loops

	// Zoho Mail API: sortorder=false = descending (newest-first).
	// The receivedTime parameter is an UPPER bound ("before this time"), not a lower bound,
	// so we omit it and instead stop pagination as soon as we encounter a message older than
	// our `since` cursor — that way each poll only processes emails received since last run.
	sinceMs := since.UnixMilli()

	type zohoMsg struct {
		MessageID     string       `json:"messageId"`
		FolderID      string       `json:"folderId"`
		Subject       string       `json:"subject"`
		Sender        string       `json:"sender"`
		FromAddress   string       `json:"fromAddress"` // bare email address of the sender
		ToAddress     string       `json:"toAddress"`
		ReceivedTime  json.Number  `json:"receivedTime"`  // unix ms — Zoho returns as string
		HasAttachment zohoFlexBool `json:"hasAttachment"` // Zoho returns string "true"/"false" or bool
	}

	var result []parsedEmail
	for page := 0; page < maxPages; page++ {
		start := page * pageSize
		endpoint := fmt.Sprintf(
			"%s/api/accounts/%s/messages/view?limit=%d&start=%d&sortorder=false",
			mailBase, accountID, pageSize, start,
		)
		log.Printf("email_polling: zoho list page=%d URL: %s", page, endpoint)
		req, _ := http.NewRequest("GET", endpoint, nil)
		req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return result, fmt.Errorf("zoho list page %d: %v", page, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			return result, fmt.Errorf("zoho list HTTP %d page %d (body: %.300s)", resp.StatusCode, page, string(raw))
		}

		var res struct {
			Data []zohoMsg `json:"data"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return result, fmt.Errorf("zoho list parse error page %d: %v (body: %.200s)", page, err, string(raw))
		}

		log.Printf("email_polling: zoho list page=%d returned %d messages", page, len(res.Data))

		done := false
		for _, m := range res.Data {
			// Check message timestamp before fetching content — stop as soon as
			// we see a message older than `since` (descending order means the
			// rest of the pages are all older too).
			receivedTimeMs, _ := m.ReceivedTime.Int64()
			if receivedTimeMs > 0 && receivedTimeMs < sinceMs {
				done = true
				break
			}

			htmlBody, textBody := fetchZohoMessageContent(accessToken, accountID, m.FolderID, m.MessageID, mailBase)
			// Fetch actual sender email, In-Reply-To, and RFC 2822 Message-ID — the list endpoint only returns display name.
			fromAddr, inReplyTo, rfcMsgID := fetchZohoMessageFrom(accessToken, accountID, m.FolderID, m.MessageID, mailBase)
			// Fall back to FromAddress from the list endpoint if detail fetch returned empty.
			if fromAddr == "" {
				fromAddr = html.UnescapeString(strings.TrimSpace(m.FromAddress))
			}
			// Use the real RFC 2822 Message-ID when available; otherwise fall back to synthetic ID.
			emailMsgID := rfcMsgID
			if emailMsgID == "" {
				emailMsgID = "<" + m.MessageID + "@zoho>"
			}
			to := []string{}
			if m.ToAddress != "" {
				// Zoho HTML-encodes special chars in address fields — decode before storing.
				to = []string{html.UnescapeString(m.ToAddress)}
			}
			// Build from as "Name <email>" when both are available, else whichever exists.
			senderName := html.UnescapeString(strings.TrimSpace(m.Sender))
			from := senderName
			if fromAddr != "" && senderName != "" && senderName != fromAddr {
				from = senderName + " <" + fromAddr + ">"
			} else if fromAddr != "" {
				from = fromAddr
			}
			pe := parsedEmail{
				from:              from,
				subject:           m.Subject,
				bodyText:          textBody,
				bodyHTML:          htmlBody,
				externalID:        m.MessageID,
				messageID:         emailMsgID,
				inReplyTo:         inReplyTo,
				to:                to,
				hasZohoAttachment: bool(m.HasAttachment),
				zohoFolderID:      m.FolderID,
			}
			if receivedTimeMs > 0 {
				t := time.UnixMilli(receivedTimeMs).UTC()
				pe.date = &t
			}
			result = append(result, pe)
		}

		if done || len(res.Data) < pageSize {
			break
		}
	}
	return result, nil
}

// fetchZohoMessageHeaders calls the Zoho message header endpoint and returns the
// bare sender email address (From header), the In-Reply-To header value, and
// the RFC 2822 Message-ID of this message (for reply threading).
// The /header endpoint returns the raw RFC 2822 headers, including Message-ID.
func fetchZohoMessageFrom(accessToken, accountID, folderID, messageID, mailBase string) (fromAddr, inReplyTo, rfcMessageID string) {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	endpoint := fmt.Sprintf(
		"%s/api/accounts/%s/folders/%s/messages/%s/header",
		mailBase, accountID, folderID, messageID,
	)
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", "", ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var res struct {
		Data struct {
			HeaderContent string `json:"headerContent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		log.Printf("email_polling: fetchZohoMessageFrom parse error for %s: %v (body: %.300s)", messageID, err, string(raw))
		return "", "", ""
	}
	// Parse RFC 2822 headers from the raw header block.
	// Headers can be multi-line (folded with \r\n + whitespace).
	var currentKey, currentVal string
	flush := func() {
		v := strings.TrimSpace(currentVal)
		switch strings.ToLower(currentKey) {
		case "message-id":
			rfcMessageID = v
		case "in-reply-to":
			inReplyTo = v
		case "from":
			// Extract bare email from "Name <email>" or "email"
			if addr, err := mail.ParseAddress(v); err == nil {
				fromAddr = addr.Address
			} else if idx := strings.LastIndex(v, "<"); idx >= 0 {
				fromAddr = strings.Trim(v[idx:], "<> ")
			} else {
				fromAddr = v
			}
		}
	}
	for _, line := range strings.Split(res.Data.HeaderContent, "\r\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			// Continuation of previous header value (folded).
			currentVal += " " + strings.TrimSpace(line)
			continue
		}
		if currentKey != "" {
			flush()
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			currentKey = line[:idx]
			currentVal = strings.TrimSpace(line[idx+1:])
		} else {
			currentKey = ""
			currentVal = ""
		}
	}
	if currentKey != "" {
		flush()
	}
	return fromAddr, inReplyTo, rfcMessageID
}

// fetchZohoMessageContent returns the raw HTML body and a plain-text fallback.
func fetchZohoMessageContent(accessToken, accountID, folderID, messageID, mailBase string) (html, text string) {
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
		return "", ""
	}
	defer resp.Body.Close()

	var res struct {
		Data struct{ Content string `json:"content"` } `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&res) //nolint:errcheck
	rawHTML := res.Data.Content
	// Embed HTTP images (e.g. Zoho CDN) as base64 data URIs so they render in any browser context.
	rawHTML = embedHTTPImagesAsDataURIs(rawHTML, "Zoho-oauthtoken "+accessToken, 10, 512*1024)
	plain := strings.ReplaceAll(rawHTML, "<br>", "\n")
	plain = strings.ReplaceAll(plain, "<br/>", "\n")
	plain = strings.ReplaceAll(plain, "<br />", "\n")
	return rawHTML, stripHTMLTags(plain)
}

// embedHTTPImagesAsDataURIs replaces src="https://..." (or src='...') in HTML with base64 data URIs.
// Up to maxImages images are downloaded; images larger than maxBytes are skipped.
// authHeader is sent on every request (e.g. "Zoho-oauthtoken TOKEN"); pass "" for unauthenticated.
var imgSrcDoubleRe = regexp.MustCompile(`(?i)\bsrc="(https?://[^">\s]+)"`)
var imgSrcSingleRe = regexp.MustCompile(`(?i)\bsrc='(https?://[^'>\s]+)'`)

func embedHTTPImagesAsDataURIs(html, authHeader string, maxImages, maxBytes int) string {
	if html == "" {
		return html
	}
	embedded := 0
	cache := make(map[string]string) // url → data URI (or "" if failed)

	downloadAndCache := func(imgURL string) string {
		if dataURI, ok := cache[imgURL]; ok {
			return dataURI
		}
		if embedded >= maxImages {
			cache[imgURL] = ""
			return ""
		}
		req, err := http.NewRequest("GET", imgURL, nil)
		if err != nil {
			cache[imgURL] = ""
			return ""
		}
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil || resp.StatusCode >= 400 {
			if resp != nil {
				resp.Body.Close()
			}
			cache[imgURL] = ""
			return ""
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
		if err != nil || len(data) == 0 {
			cache[imgURL] = ""
			return ""
		}
		mimeType := resp.Header.Get("Content-Type")
		if idx := strings.IndexByte(mimeType, ';'); idx >= 0 {
			mimeType = strings.TrimSpace(mimeType[:idx])
		}
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			cache[imgURL] = ""
			return ""
		}
		dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
		cache[imgURL] = dataURI
		embedded++
		return dataURI
	}

	html = imgSrcDoubleRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := imgSrcDoubleRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		dataURI := downloadAndCache(sub[1])
		if dataURI == "" {
			return match
		}
		return `src="` + dataURI + `"`
	})
	html = imgSrcSingleRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := imgSrcSingleRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		dataURI := downloadAndCache(sub[1])
		if dataURI == "" {
			return match
		}
		return `src='` + dataURI + `'`
	})
	return html
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
// candidateFolderIDs is tried in order; "" means the no-folder URL variant.
// imapHost/imapUsername are used as a fallback when the REST API returns a 5xx error.
// msgSubject is used for IMAP SEARCH when the IMAP fallback is active.
func fetchZohoAttachments(accessToken, accountID string, candidateFolderIDs []string, messageID, mailBase, storeID, imapHost, imapUsername, imapPassword, msgSubject string) []models.ProcurementAttachment {
	if mailBase == "" {
		mailBase = "https://mail.zoho.com"
	}
	log.Printf("email_polling: zoho fetching attachments for msg %s (folderCandidates=%v)", messageID, candidateFolderIDs)

	// Build candidate URLs from the provided folder ID list.
	urls := []string{}
	seen := map[string]bool{}
	for _, fid := range candidateFolderIDs {
		var u string
		if fid != "" {
			u = fmt.Sprintf("%s/api/accounts/%s/folders/%s/messages/%s/attachments",
				mailBase, accountID, fid, messageID)
		} else {
			u = fmt.Sprintf("%s/api/accounts/%s/messages/%s/attachments",
				mailBase, accountID, messageID)
		}
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	var raw []byte
	for _, endpoint := range urls {
		req, _ := http.NewRequest("GET", endpoint, nil)
		req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			log.Printf("email_polling: zoho attachment list network error for msg %s: %v", messageID, err)
			return nil
		}
		raw, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == 200 {
			break
		}
		log.Printf("email_polling: zoho attachment list HTTP %d for msg %s (url: %s): %.200s", resp.StatusCode, messageID, endpoint, string(raw))
		raw = nil // clear so we don't process an error response
	}
	if raw == nil {
		// REST API failed — fall back to IMAP (password auth preferred, XOAUTH2 otherwise).
		if imapHost != "" && msgSubject != "" && (imapUsername != "" || imapPassword != "") {
			log.Printf("email_polling: zoho REST attachments failed for %s — trying IMAP fallback (host=%s user=%s subject=%q)", messageID, imapHost, imapUsernameForAccount(imapUsername, ""), msgSubject)
			return fetchZohoAttachmentsViaIMAP(accessToken, imapHost, imapUsername, imapPassword, msgSubject, storeID, messageID)
		}
		return nil
	}

	var res struct {
		Data []struct {
			AttachmentID string `json:"attachmentId"`
			FileName     string `json:"fileName"`
			ContentType  string `json:"contentType"`
			Size         int64  `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		log.Printf("email_polling: zoho attachment list parse error for msg %s: %v (body: %.200s)", messageID, err, string(raw))
		return nil
	}

	var atts []models.ProcurementAttachment
	for _, a := range res.Data {
		att := models.ProcurementAttachment{
			Filename:    a.FileName,
			ContentType: a.ContentType,
			Size:        a.Size,
		}
		// Download attachment content — try each candidate folder ID, then no-folder.
		dlURLs := []string{}
		dlSeen := map[string]bool{}
		for _, fid := range candidateFolderIDs {
			var u string
			if fid != "" {
				u = fmt.Sprintf("%s/api/accounts/%s/folders/%s/messages/%s/attachments/%s",
					mailBase, accountID, fid, messageID, a.AttachmentID)
			} else {
				u = fmt.Sprintf("%s/api/accounts/%s/messages/%s/attachments/%s",
					mailBase, accountID, messageID, a.AttachmentID)
			}
			if !dlSeen[u] {
				dlSeen[u] = true
				dlURLs = append(dlURLs, u)
			}
		}

		for _, dlURL := range dlURLs {
			dlReq, _ := http.NewRequest("GET", dlURL, nil)
			dlReq.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
			dlResp, dlErr := (&http.Client{Timeout: 30 * time.Second}).Do(dlReq)
			if dlErr != nil {
				log.Printf("email_polling: zoho attachment download network error for %s/%s: %v", messageID, a.FileName, dlErr)
				break
			}
			if dlResp.StatusCode == 200 {
				data, _ := io.ReadAll(dlResp.Body)
				dlResp.Body.Close()
				if len(data) > 0 {
					att.URL = saveEmailAttachment(storeID, messageID, a.FileName, data)
					log.Printf("email_polling: zoho attachment saved %s/%s → %s", messageID, a.FileName, att.URL)
				}
				break
			}
			body, _ := io.ReadAll(dlResp.Body)
			dlResp.Body.Close()
			log.Printf("email_polling: zoho attachment download HTTP %d for %s/%s url=%s: %.200s", dlResp.StatusCode, messageID, a.FileName, dlURL, string(body))
		}
		atts = append(atts, att)
	}
	return atts
}

// fetchZohoAttachmentsViaIMAP connects to Zoho IMAP and downloads attachments for
// the message whose Subject matches msgSubject. It uses plain LOGIN when imapPassword
// is set (preferred), falling back to XOAUTH2 with the OAuth access token.
func fetchZohoAttachmentsViaIMAP(accessToken, imapHost, imapUsername, imapPassword, msgSubject, storeID, msgID string) []models.ProcurementAttachment {
	if !imapAcquire() {
		return nil
	}

	conn, err := tls.Dial("tcp", net.JoinHostPort(imapHost, "993"), &tls.Config{})
	if err != nil {
		log.Printf("imap: connect to %s:993 failed: %v", imapHost, err)
		return nil
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(60 * time.Second)) //nolint:errcheck

	r := bufio.NewReader(conn)
	readLine := func() string {
		line, _ := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	writeLine := func(s string) { fmt.Fprintf(conn, "%s\r\n", s) }

	// Read banner
	banner := readLine()
	if !strings.HasPrefix(banner, "* OK") {
		log.Printf("imap: unexpected banner from %s: %s", imapHost, banner)
		return nil
	}

	// readTagged reads lines until the line starting with `tag` is seen,
	// then returns that tagged line. Zoho sends untagged responses (e.g.
	// "* CAPABILITY ...") before the final tagged response, so we must
	// skip them instead of checking only the first line.
	readTagged := func(tag string) string {
		for {
			line := readLine()
			if strings.HasPrefix(line, tag) {
				return line
			}
			if strings.Contains(strings.ToLower(line), "blocked") {
				return line // surface block errors immediately
			}
		}
	}

	// Authenticate: plain LOGIN (password) preferred, XOAUTH2 fallback.
	if imapPassword != "" {
		cmd := fmt.Sprintf("A1 LOGIN %s %s", imapQuote(imapUsername), imapQuote(imapPassword))
		writeLine(cmd)
		authResp := readTagged("A1")
		if !strings.Contains(authResp, "A1 OK") {
			log.Printf("imap: LOGIN failed: %s", authResp)
			if strings.Contains(strings.ToLower(authResp), "blocked") {
				imapSetBlocked()
			}
			return nil
		}
	} else {
		saslPlain := "user=" + imapUsername + "\x01auth=Bearer " + accessToken + "\x01\x01"
		saslB64 := base64.StdEncoding.EncodeToString([]byte(saslPlain))
		writeLine("A1 AUTHENTICATE XOAUTH2")
		challenge := readLine()
		if !strings.HasPrefix(challenge, "+ ") {
			log.Printf("imap: expected XOAUTH2 challenge, got: %s", challenge)
			return nil
		}
		writeLine(saslB64)
		authResp := readTagged("A1")
		if !strings.Contains(authResp, "A1 OK") {
			log.Printf("imap: XOAUTH2 auth failed: %s", authResp)
			if strings.Contains(strings.ToLower(authResp), "blocked") {
				imapSetBlocked()
			}
			return nil
		}
	}
	log.Printf("imap: authenticated as %s on %s", imapUsername, imapHost)

	// SELECT INBOX
	writeLine("A2 SELECT INBOX")
	for {
		line := readLine()
		if strings.HasPrefix(line, "A2 ") {
			if !strings.Contains(line, "A2 OK") {
				log.Printf("imap: SELECT INBOX failed: %s", line)
				return nil
			}
			break
		}
	}

	// SEARCH by Subject
	escapedSubj := strings.ReplaceAll(msgSubject, `"`, `\"`)
	writeLine(fmt.Sprintf(`A3 UID SEARCH HEADER Subject "%s"`, escapedSubj))
	var uids []string
	for {
		line := readLine()
		if strings.HasPrefix(line, "* SEARCH") {
			parts := strings.Fields(line)
			if len(parts) > 2 {
				uids = parts[2:]
			}
		}
		if strings.HasPrefix(line, "A3 ") {
			break
		}
	}
	if len(uids) == 0 {
		log.Printf("imap: no messages found for subject %q", msgSubject)
		return nil
	}
	log.Printf("imap: found UIDs %v for subject %q", uids, msgSubject)

	// FETCH full message RFC822 for first matching UID
	writeLine(fmt.Sprintf("A4 UID FETCH %s RFC822", uids[0]))
	var rawMsg []byte
	for {
		line := readLine()
		if strings.Contains(line, "RFC822") {
			// parse literal size: e.g. "* 3 FETCH (UID 42 RFC822 {12345}"
			start := strings.LastIndex(line, "{")
			end := strings.LastIndex(line, "}")
			if start >= 0 && end > start {
				var size int
				fmt.Sscanf(line[start+1:end], "%d", &size)
				if size > 0 {
					rawMsg = make([]byte, size)
					if _, err := io.ReadFull(r, rawMsg); err != nil {
						log.Printf("imap: read RFC822 body failed: %v", err)
						return nil
					}
				}
			}
		}
		if strings.HasPrefix(line, "A4 ") {
			break
		}
	}
	if len(rawMsg) == 0 {
		log.Printf("imap: empty RFC822 body for UID %s", uids[0])
		return nil
	}

	writeLine("A5 LOGOUT")

	return parseIMAPMIMEAttachments(rawMsg, storeID, msgID)
}

// parseIMAPMIMEAttachments extracts and saves attachment files from a raw RFC822 message.
func parseIMAPMIMEAttachments(rawMsg []byte, storeID, msgID string) []models.ProcurementAttachment {
	msg, err := mail.ReadMessage(bytes.NewReader(rawMsg))
	if err != nil {
		log.Printf("imap: failed to parse MIME message: %v", err)
		return nil
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		log.Printf("imap: message is not multipart (%s), no attachments", mediaType)
		return nil
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var atts []models.ProcurementAttachment
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		disp := part.Header.Get("Content-Disposition")
		if !strings.Contains(strings.ToLower(disp), "attachment") {
			continue
		}
		_, dispParams, _ := mime.ParseMediaType(disp)
		filename := dispParams["filename"]
		if filename == "" {
			filename = "attachment"
		}
		var data []byte
		encoding := strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Transfer-Encoding")))
		switch encoding {
		case "base64":
			data, err = io.ReadAll(base64.NewDecoder(base64.StdEncoding, part))
		case "quoted-printable":
			data, err = io.ReadAll(part) // net/mail already decodes QP
		default:
			data, err = io.ReadAll(part)
		}
		if err != nil || len(data) == 0 {
			continue
		}
		ct := part.Header.Get("Content-Type")
		if idx := strings.Index(ct, ";"); idx >= 0 {
			ct = strings.TrimSpace(ct[:idx])
		}
		url := saveEmailAttachment(storeID, msgID, filename, data)
		atts = append(atts, models.ProcurementAttachment{
			Filename:    filename,
			ContentType: ct,
			Size:        int64(len(data)),
			URL:         url,
		})
		log.Printf("imap: saved attachment %s → %s", filename, url)
	}
	return atts
}

// ─── Gmail polling ────────────────────────────────────────────────────────────

func pollGmailAccount(storeID primitive.ObjectID, settings models.StoreSettings, acct models.RFQEmailAccount) {
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	if acct.LastPolledAt != nil {
		since = acct.LastPolledAt.UTC()
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
	const gmailPageSize = 50
	const gmailMaxPages = 20

	var result []parsedEmail
	nextPageToken := ""
	for page := 0; page < gmailMaxPages; page++ {
		listURL := fmt.Sprintf(
			"https://gmail.googleapis.com/gmail/v1/users/me/messages?q=after:%s+in:inbox&maxResults=%d",
			afterDate, gmailPageSize,
		)
		if nextPageToken != "" {
			listURL += "&pageToken=" + url.QueryEscape(nextPageToken)
		}
		req, _ := http.NewRequest("GET", listURL, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
		if err != nil {
			return result, err
		}
		var listRes struct {
			Messages      []struct{ ID string `json:"id"` } `json:"messages"`
			NextPageToken string                            `json:"nextPageToken"`
		}
		json.NewDecoder(resp.Body).Decode(&listRes) //nolint:errcheck
		resp.Body.Close()

		log.Printf("email_polling: gmail list page=%d count=%d nextPage=%v", page, len(listRes.Messages), listRes.NextPageToken != "")
		for _, m := range listRes.Messages {
			if pe := fetchGmailMessageContentWithStore(accessToken, m.ID, storeIDStr); pe != nil {
				pe.externalID = m.ID
				result = append(result, *pe)
			}
		}

		nextPageToken = listRes.NextPageToken
		if nextPageToken == "" {
			break
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
		case "message-id":
			pe.messageID = h.Value
		}
	}

	// cidMap: Content-ID → data URI for inline images embedded via cid: references.
	cidMap := make(map[string]string)

	// Recursively walk parts to extract body text and attachments.
	var walkParts func(parts []gmailPart)
	walkParts = func(parts []gmailPart) {
		for _, part := range parts {
			if strings.HasPrefix(part.MimeType, "multipart/") {
				walkParts(part.Parts)
				continue
			}
			if strings.HasPrefix(part.MimeType, "text/html") && part.Body.Data != "" && pe.bodyHTML == "" {
				pe.bodyHTML = gmailBase64Decode(part.Body.Data)
				continue
			}
			if strings.HasPrefix(part.MimeType, "text/plain") && part.Body.Data != "" && pe.bodyText == "" {
				pe.bodyText = gmailBase64Decode(part.Body.Data)
				continue
			}
			// Inline image: image/* with Content-ID header — these are cid: references in the HTML body.
			if strings.HasPrefix(part.MimeType, "image/") && part.Body.AttachmentId != "" && part.Body.Size < 1024*1024 {
				var contentID string
				for _, h := range part.Headers {
					if strings.EqualFold(h.Name, "Content-ID") {
						contentID = strings.Trim(h.Value, "<> \t")
						break
					}
				}
				if contentID != "" {
					dlURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s/attachments/%s", msgID, part.Body.AttachmentId)
					dlReq, _ := http.NewRequest("GET", dlURL, nil)
					dlReq.Header.Set("Authorization", "Bearer "+accessToken)
					if dlResp, dErr := (&http.Client{Timeout: 30 * time.Second}).Do(dlReq); dErr == nil {
						var attBody struct{ Data string `json:"data"` }
						if json.NewDecoder(dlResp.Body).Decode(&attBody) == nil && attBody.Data != "" {
							imgBytes := []byte(gmailBase64Decode(attBody.Data))
							cidMap[contentID] = "data:" + part.MimeType + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)
						}
						dlResp.Body.Close()
					}
				}
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

	// Replace cid: references in HTML body with embedded base64 data URIs.
	if pe.bodyHTML != "" && len(cidMap) > 0 {
		for cid, dataURI := range cidMap {
			pe.bodyHTML = strings.ReplaceAll(pe.bodyHTML, "cid:"+cid, dataURI)
		}
	}

	// Fix relative image URLs from known email providers (e.g. Zoho Mail uses /mail/ImageDisplay?... paths).
	if pe.bodyHTML != "" {
		pe.bodyHTML = strings.ReplaceAll(pe.bodyHTML, `src="/mail/`, `src="https://mail.zoho.com/mail/`)
		pe.bodyHTML = strings.ReplaceAll(pe.bodyHTML, `src='/mail/`, `src='https://mail.zoho.com/mail/`)
	}

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
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	if acct.LastPolledAt != nil {
		since = acct.LastPolledAt.UTC()
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
	const outlookMaxPages = 20

	type outlookMsg struct {
		ID               string `json:"id"`
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
	}

	var result []parsedEmail
	nextURL := fmt.Sprintf(
		"https://graph.microsoft.com/v1.0/me/mailFolders/Inbox/messages?$filter=receivedDateTime ge %s&$select=subject,from,toRecipients,body,receivedDateTime&$top=50&$orderby=receivedDateTime asc",
		sinceStr,
	)

	for page := 0; page < outlookMaxPages && nextURL != ""; page++ {
		req, _ := http.NewRequest("GET", nextURL, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Prefer", `outlook.body-content-type="html"`)
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return result, err
		}
		var res struct {
			Value    []outlookMsg `json:"value"`
			NextLink string       `json:"@odata.nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			resp.Body.Close()
			return result, err
		}
		resp.Body.Close()

		log.Printf("email_polling: outlook list page=%d count=%d nextPage=%v", page, len(res.Value), res.NextLink != "")
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
				from:       from,
				subject:    m.Subject,
				bodyHTML:   m.Body.Content,
				bodyText:   stripHTMLTags(m.Body.Content),
				externalID: m.ID,
				to:         to,
			}
			if m.ReceivedDateTime != "" {
				if t, err := time.Parse(time.RFC3339, m.ReceivedDateTime); err == nil {
					t = t.UTC()
					pe.date = &t
				}
			}
			result = append(result, pe)
		}
		nextURL = res.NextLink
	}
	return result, nil
}

// ─── Shared: process a polled email through the RFQ pipeline ──────────────────

// emailMatchesKeywords returns true when the email subject or body contains at least
// one keyword (case-insensitive). An empty keywords list means "accept all" (no filter).
func emailMatchesKeywords(subject, bodyText string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	haystack := strings.ToLower(subject + " " + bodyText)
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if strings.Contains(haystack, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

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

// mentionsAttachment returns true when the email body contains phrases that
// clearly indicate the SENDER intended to include a file attachment.
// Single words like "attached" or "attachment" are intentionally excluded
// because they appear frequently in email disclaimers/signatures
// (e.g. "scan attachments (if any)") and cause false positives.
func mentionsAttachment(bodyText string) bool {
	lower := strings.ToLower(bodyText)
	phrases := []string{
		// "please find …" patterns
		"please find attached",
		"please find the attached",
		"please find enclosed",
		"kindly find attached",
		"kindly find the attached",
		// "find/see …" patterns
		"find attached",
		"find the attached",
		"see attached",
		"see the attached",
		// "as/for the attached …" — catches "need quotation for the attached excel"
		"as attached",
		"for the attached",
		"the attached file",
		"the attached document",
		"the attached spreadsheet",
		"the attached excel",
		"the attached sheet",
		"the attached price",
		"the attached list",
		"the attached quotation",
		"the attached pdf",
		"the attached image",
		"attached herewith",
		"the attachment",
		// "i/we have/am/are attaching/attached …"
		"i have attached",
		"i've attached",
		"i am attaching",
		"i'm attaching",
		"we have attached",
		"we've attached",
		"we are attaching",
		"we're attaching",
		// "refer/look at …"
		"refer to attached",
		"refer to the attached",
		"look at the attached",
		// "please see …"
		"please see attached",
		// "enclosed …"
		"enclosed herewith",
		"enclosed please find",
		"i have enclosed",
		"i've enclosed",
		"please find enclosed",
		// "attached is …"
		"attached is the",
		"attached is a",
		// generic "attached" as verb/adjective in clear attachment contexts
		"with attached",
		"is attached",
		"are attached",
		"sending attached",
	}
	for _, p := range phrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// processPolledEmail saves the message to the procurement log and, if AI RFQ bot
// is enabled, runs LLM classification + extraction and creates an RFQ record.
func processPolledEmail(storeID primitive.ObjectID, settings models.StoreSettings, provider string, msg parsedEmail) *models.ProcurementMessage {
	if msg.from == "" && msg.bodyText == "" {
		return nil
	}

	// Skip duplicate emails (same provider message already ingested).
	if models.ProcurementMessageExternalIDExists(storeID, msg.externalID) {
		log.Printf("email_polling: skipping duplicate external_id=%s", msg.externalID)
		return nil
	}

	emailText := fmt.Sprintf("Subject: %s\nFrom: %s\n\n%s", msg.subject, msg.from, msg.bodyText)

	store, storeObjID, err := rfqEmailGetStore(storeID.Hex())
	if err != nil {
		log.Printf("email_polling: store not found %s: %v", storeID.Hex(), err)
		return nil
	}

	// Detect if this is a reply email so we can bypass keyword/LLM filters.
	// A reply is identified by any of:
	//   1. In-Reply-To header matches one of our outbound message IDs
	//   2. The sender is already a known contact in our procurement threads
	//   3. Subject starts with "Re:" (universal reply indicator)
	senderEmail := msg.from
	if addr, err := mail.ParseAddress(msg.from); err == nil {
		senderEmail = addr.Address
	}
	subjectIsReply := strings.HasPrefix(strings.ToLower(strings.TrimSpace(msg.subject)), "re:")
	isReplyToOurs := models.IsReplyToOurMessage(storeID, msg.inReplyTo) ||
		subjectIsReply && models.IsKnownEmailContact(storeID, senderEmail) ||
		(msg.inReplyTo != "" && models.IsKnownEmailContact(storeID, senderEmail))
	if isReplyToOurs {
		log.Printf("email_polling: email from %s detected as reply (inReplyTo=%q subjectRe=%v) — bypassing keyword/LLM filters", msg.from, msg.inReplyTo, subjectIsReply)
	}

	// Keyword pre-filter: reject emails that don't contain any configured keyword.
	// This happens before DB insertion to reduce storage and LLM token usage.
	if !isReplyToOurs && !emailMatchesKeywords(msg.subject, msg.bodyText, store.Settings.IncomingEmailKeywords) {
		log.Printf("email_polling: email from %s ignored — does not match incoming keyword filter", msg.from)
		return nil
	}

	// Detect missing attachments:
	//  1. Zoho explicitly said the message has attachments (hasZohoAttachment) but none were downloaded, OR
	//  2. The email body text mentions an attachment but no files were received.
	hasDownloadedAttachments := false
	for _, att := range msg.attachments {
		if att.URL != "" {
			hasDownloadedAttachments = true
			break
		}
	}
	attachmentMissing := !hasDownloadedAttachments && (msg.hasZohoAttachment || mentionsAttachment(msg.bodyText))

	if attachmentMissing {
		log.Printf("email_polling: email from %s has missing attachments (zohoFlag=%v, phraseMatch=%v) — flagged as attachment_missing",
			msg.from, msg.hasZohoAttachment, mentionsAttachment(msg.bodyText))
	}

	// LLM classification runs BEFORE saving to DB so that marketing / cold-outreach
	// emails are never persisted. Replies to our own outbound messages bypass this
	// filter — they are always legitimate regardless of content.
	var msgType, rfqCode string
	if isReplyToOurs {
		msgType = "rfq" // treat reply as conversation continuation
	} else {
		msgType, rfqCode = classifyIncomingMessage(store, emailText, nil)
		if msgType == "other" {
			log.Printf("email_polling: email from %s classified as marketing/other — discarding without saving", msg.from)
			return nil
		}
	}

	// Record in procurement log (only rfq/quotation emails reach this point).
	procMsg := saveProcurementEmailMessage(storeObjID, "in", provider, msg.from, msg.to, msg.subject, msg.bodyText, msg.bodyHTML, msg.externalID, msg.messageID, msg.attachments, attachmentMissing, false, nil, msg.date)

	// Block RFQ creation when attachments were expected but not received.
	if attachmentMissing {
		log.Printf("email_polling: skipping RFQ creation for %s — waiting for missing attachments", msg.from)
		return procMsg
	}

	if !store.Settings.EnableAIRFQBot {
		log.Printf("email_polling: AI RFQ bot disabled for store %s — skipping RFQ creation for email from %s", storeID.Hex(), msg.from)
		return procMsg
	}
	if store.Settings.RFQLLMAPIKey == "" {
		log.Printf("email_polling: no LLM API key configured for store %s — skipping RFQ creation for email from %s", storeID.Hex(), msg.from)
		return procMsg
	}
	if store.Settings.DisableAutoRFQFromEmail {
		log.Printf("email_polling: auto RFQ from email disabled for store %s — skipping for %s", storeID.Hex(), msg.from)
		return procMsg
	}

	// Use pre-computed classification (already ran before save above).
	if msgType == "quotation" {
		log.Printf("email_polling: email from %s classified as supplier quotation — marking", msg.from)
		models.LinkMessageAsQuotation(procMsg.ID, nil, rfqCode) //nolint:errcheck
		return procMsg
	}
	// msgType == "" (LLM unavailable — pass through as rfq) or "rfq" — continue with RFQ creation.

	// LLM duplicate check — is it a reminder for an existing RFQ?
	if isReminderEmail(store, msg.from, msg.subject, msg.bodyText) {
		log.Printf("email_polling: email from %s detected as reminder/follow-up — not creating new RFQ", msg.from)
		return procMsg
	}

	// LLM extraction — build attachment content for the LLM.
	// Images → data URIs, PDFs → base64 blobs, Excel → plain text appended to prompt.
	llmProvider := strings.ToLower(store.Settings.RFQLLMProvider)
	var imageDataURIs []string
	var pdfBase64s []string
	for _, att := range msg.attachments {
		if att.URL == "" {
			continue
		}
		// att.URL is "/attachments/{storeID}/{msgID}/{filename}"; file lives at "./attachments/..."
		diskPath := "." + att.URL
		data, readErr := os.ReadFile(diskPath)
		if readErr != nil || len(data) == 0 {
			log.Printf("email_polling: could not read attachment %s: %v", diskPath, readErr)
			continue
		}
		ext := strings.ToLower(filepath.Ext(att.Filename))
		switch {
		case isRFQImageExt(ext):
			mime := rfqImageMime(ext, att.ContentType)
			imageDataURIs = append(imageDataURIs, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
		case ext == ".pdf":
			pdfBase64s = append(pdfBase64s, base64.StdEncoding.EncodeToString(data))
		case ext == ".xls" || ext == ".xlsx":
			if txt, exErr := excelToText(att.Filename, data); exErr == nil {
				emailText += "\n\n" + txt
			}
		}
	}

	var extracted rfqExtractResult
	_, legacyURL := resolveExtractionEndpoint(llmProvider, &store.Settings)
	raw, llmErr := callLLMExtractRFQ(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, llmProvider, emailText, imageDataURIs, pdfBase64s, legacyURL)
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
		extracted.CustomerContactPerson, extracted.CustomerNationalAddress,
	); err != nil {
		log.Printf("email_polling: FindOrCreateCustomerFromRFQ error: %v", err)
	} else if customer != nil {
		customerID = &customer.ID
	}

	// Prefer company name; fall back to contact person, then sender address.
	fromName := extracted.CustomerName
	if fromName == "" {
		fromName = extracted.CustomerContactPerson
	}
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
			Notes:    p.Notes,
		})
	}

	rfq := &models.RFQReceived{
		StoreID:             storeObjID,
		FromPhone:           msg.from,
		FromName:            fromName,
		MessageType:         "text",
		TextContent:         emailText,
		Source:              "email",
		Status:              "ready_to_send",
		Products:            products,
		GeneralInstructions: extracted.GeneralInstructions,
		CustomerID:              customerID,
		CustomerName:            extracted.CustomerName,
		CustomerContactPerson:   extracted.CustomerContactPerson,
		CustomerPhone:           extracted.CustomerPhone,
		CustomerEmail:           extracted.CustomerEmail,
		CustomerCompany:         extracted.CustomerCompany,
		CustomerVATNo:           extracted.CustomerVATNo,
		CustomerCRNo:            extracted.CustomerCRNo,
		CustomerNationalAddress: extracted.CustomerNationalAddress,
	}
	if procMsg != nil {
		rfq.ProcurementMessageID = &procMsg.ID
		rfq.ProcurementMessageCode = procMsg.Code
	}
	if err := models.CreateRFQReceived(rfq); err != nil {
		log.Printf("email_polling: failed to save RFQ from %s: %v", msg.from, err)
		return procMsg
	}

	// Back-link: update procurement message to reflect it was processed as an RFQ.
	if procMsg != nil {
		models.LinkProcurementMessageToRFQ(procMsg.ID, rfq.ID)
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
	return procMsg
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func updateAccountLastPolled(storeID, accountID primitive.ObjectID, t time.Time) {
	rfqEmailAccountUpdate(storeID, accountID, bson.M{"last_polled_at": t}) //nolint:errcheck
}

// testIMAPConnection dials imapHost:993 with TLS and authenticates via XOAUTH2.
// Returns (true, "") on success or (false, human-readable error) on failure.
func testIMAPConnection(accessToken, imapHost, imapUsername string) (bool, string) {
	if imapHost == "" {
		return false, "IMAP host is not configured"
	}
	conn, err := tls.Dial("tcp", net.JoinHostPort(imapHost, "993"), &tls.Config{})
	if err != nil {
		return false, fmt.Sprintf("Cannot connect to %s:993 — %v", imapHost, err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(20 * time.Second)) //nolint:errcheck

	r := bufio.NewReader(conn)
	readLine := func() string {
		line, _ := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	writeLine := func(s string) { fmt.Fprintf(conn, "%s\r\n", s) }

	banner := readLine()
	if !strings.HasPrefix(banner, "* OK") {
		return false, fmt.Sprintf("Unexpected IMAP banner: %s", banner)
	}

	saslPlain := "user=" + imapUsername + "\x01auth=Bearer " + accessToken + "\x01\x01"
	saslB64 := base64.StdEncoding.EncodeToString([]byte(saslPlain))

	writeLine("T1 AUTHENTICATE XOAUTH2")
	challenge := readLine()
	if !strings.HasPrefix(challenge, "+ ") {
		return false, fmt.Sprintf("Expected SASL challenge, got: %s", challenge)
	}
	writeLine(saslB64)
	authResp := readLine()
	if !strings.Contains(authResp, "T1 OK") {
		return false, fmt.Sprintf("Authentication failed: %s — check that IMAP Access is enabled in Zoho Mail Settings → Mail Accounts → IMAP", authResp)
	}
	writeLine("T2 LOGOUT")
	return true, ""
}

// stripHTMLTags removes HTML tags from content returned by Zoho.
func stripHTMLTags(s string) string {
	// Remove entire <style>…</style> and <script>…</script> blocks (case-insensitive).
	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	s = styleRe.ReplaceAllString(s, "")
	s = scriptRe.ReplaceAllString(s, "")

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
