package controller

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GET /v1/procurement-message-threads
// Returns distinct supplier contact threads sorted by last message date.
func ListProcurementThreadsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	msgType := r.URL.Query().Get("type")
	if msgType == "" {
		msgType = "whatsapp"
	}
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}

	phonesStr := strings.TrimSpace(r.URL.Query().Get("phones"))
	var phones []string
	if phonesStr != "" {
		for _, p := range strings.Split(phonesStr, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				phones = append(phones, p)
			}
		}
	}

	dateFrom, dateTo := procurementDateRange(storeID, r.URL.Query().Get("date_from"), r.URL.Query().Get("date_to"))

	threads, total, err := models.ListContactThreads(storeID, msgType, search, page, limit, phones, dateFrom, dateTo)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if threads == nil {
		threads = []models.ContactThread{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"threads": threads,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// RFQUnreadSummary represents unread WhatsApp messages for one supplier/customer of an RFQ.
type RFQUnreadSummary struct {
	RFQID       string `json:"rfq_id"`
	RFQCode     string `json:"rfq_code"`
	Phone       string `json:"phone"`
	ContactName string `json:"contact_name"`
	PhoneType   string `json:"phone_type"` // "supplier" | "customer"
	UnreadCount int    `json:"unread_count"`
	LastMsgDate string `json:"last_message_date,omitempty"`
	LastMsgText string `json:"last_message_text,omitempty"`
}

func fmtThreadDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// GET /v1/rfq-whatsapp-unread
// Returns unread WhatsApp message summaries grouped by RFQ.
func GetRFQWhatsAppUnreadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	// Step 1: get all WhatsApp threads with unread messages
	threads, _, err := models.ListContactThreads(storeID, "whatsapp", "", 1, 1000, nil, nil, nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Keep only threads with unread messages; sum total across ALL threads for the badge count.
	var unreadPhones []string
	unreadByPhone := map[string]models.ContactThread{}
	allThreadsUnread := 0
	for _, t := range threads {
		if t.UnreadCount > 0 {
			norm := strings.TrimPrefix(t.ContactPhone, "+")
			unreadPhones = append(unreadPhones, norm)
			unreadByPhone[norm] = t
			allThreadsUnread += t.UnreadCount
		}
	}

	if len(unreadPhones) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{"items": []RFQUnreadSummary{}, "total_unread": 0})
		return
	}

	// Step 2: find RFQs whose forwarded_to phones or customer_phone match the unread phones
	rfqs, err := models.FindRFQsByPhones(storeID, unreadPhones)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	normalise := func(p string) string { return strings.TrimPrefix(p, "+") }

	var items []RFQUnreadSummary
	for _, rfq := range rfqs {
		for _, s := range rfq.ForwardedTo {
			norm := normalise(s.Phone)
			if t, ok := unreadByPhone[norm]; ok {
				items = append(items, RFQUnreadSummary{
					RFQID:       rfq.ID.Hex(),
					RFQCode:     rfq.Code,
					Phone:       s.Phone,
					ContactName: s.SupplierName,
					PhoneType:   "supplier",
					UnreadCount: t.UnreadCount,
					LastMsgDate: fmtThreadDate(t.LastMessageDate),
					LastMsgText: t.LastMessageText,
				})
			}
		}
		if rfq.CustomerPhone != "" {
			norm := normalise(rfq.CustomerPhone)
			if t, ok := unreadByPhone[norm]; ok {
				items = append(items, RFQUnreadSummary{
					RFQID:       rfq.ID.Hex(),
					RFQCode:     rfq.Code,
					Phone:       rfq.CustomerPhone,
					ContactName: rfq.CustomerName,
					PhoneType:   "customer",
					UnreadCount: t.UnreadCount,
					LastMsgDate: fmtThreadDate(t.LastMessageDate),
					LastMsgText: t.LastMessageText,
				})
			}
		}
	}
	if items == nil {
		items = []RFQUnreadSummary{}
	}
	// Include unread threads that had no matching RFQ so the list never appears empty
	// while the badge count is > 0.  These show with phone as contact name and no RFQ code.
	matchedPhones := map[string]bool{}
	for _, it := range items {
		matchedPhones[normalise(it.Phone)] = true
	}
	for norm, t := range unreadByPhone {
		if !matchedPhones[norm] {
			items = append(items, RFQUnreadSummary{
				Phone:       t.ContactPhone,
				ContactName: t.ContactPhone,
				PhoneType:   "supplier",
				UnreadCount: t.UnreadCount,
				LastMsgDate: fmtThreadDate(t.LastMessageDate),
				LastMsgText: t.LastMessageText,
			})
		}
	}
	// total_unread counts ALL unread WhatsApp threads (not just RFQ-linked ones) so the
	// header badge reflects every incoming message, including ones not yet tied to an RFQ.
	json.NewEncoder(w).Encode(map[string]interface{}{"items": items, "total_unread": allThreadsUnread})
}

// EmailUnreadSummary represents one unread inbound email shown in the app header badge dropdown.
type EmailUnreadSummary struct {
	ID          string `json:"id"`
	Subject     string `json:"subject,omitempty"`
	From        string `json:"from"`
	Snippet     string `json:"snippet,omitempty"`
	MessageDate string `json:"message_date,omitempty"`
	Code        string `json:"code,omitempty"`
}

// GET /v1/email-unread
// Returns unread inbound email messages for the store.
func GetEmailUnreadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	msgs, totalUnread, err := models.ListUnreadEmailMessages(storeID, 50)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	items := []EmailUnreadSummary{}
	for _, m := range msgs {
		snippet := m.BodyText
		if len([]rune(snippet)) > 100 {
			snippet = string([]rune(snippet)[:100]) + "…"
		}
		items = append(items, EmailUnreadSummary{
			ID:          m.ID.Hex(),
			Subject:     m.Subject,
			From:        m.From,
			Snippet:     snippet,
			MessageDate: fmtThreadDate(m.MessageDate),
			Code:        m.Code,
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"items": items, "total_unread": totalUnread})
}

// GET /v1/procurement-message-threads/{phone}
// Returns all messages in the thread with the given contact phone, chronologically.
func GetThreadMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	contactPhone, _ := url.PathUnescape(mux.Vars(r)["phone"])
	if contactPhone == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing phone"})
		return
	}

	msgType := r.URL.Query().Get("type")
	if msgType == "" {
		msgType = "whatsapp"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 100
	}

	_ = models.MarkThreadMessagesRead(storeID, contactPhone, msgType)

	msgs, total, err := models.ListThreadMessages(storeID, contactPhone, msgType, page, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []models.ProcurementMessage{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": msgs,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

// POST /v1/procurement-message-threads/{phone}/send
// Sends a free-text WhatsApp message to the given contact via Meta WhatsApp Cloud API.
func SendThreadMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	contactPhone, _ := url.PathUnescape(mux.Vars(r)["phone"])
	if contactPhone == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing phone"})
		return
	}

	var body struct {
		Text    string `json:"text"`
		StoreID string `json:"store_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	if body.StoreID == "" {
		body.StoreID = r.URL.Query().Get("store_id")
	}
	if body.Text == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "text is required"})
		return
	}

	storeID, err := primitive.ObjectIDFromHex(body.StoreID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	// Resolve which channel to use: store_rfq WABA → bot WABA → Evolution API
	phoneNumberID, accessToken := rfqMetaConfig(body.StoreID, "store_rfq")
	if phoneNumberID == "" || accessToken == "" {
		phoneNumberID, accessToken = rfqMetaConfig(body.StoreID, "bot")
	}

	var sendErr error
	var fromID, wamid string
	if phoneNumberID != "" && accessToken != "" {
		wamid, sendErr = metaSendText(phoneNumberID, accessToken, contactPhone, body.Text)
		fromID = phoneNumberID
	} else {
		// Fall back to Evolution API
		evoURL, evoKey, evoInstance := evoConfigFromStore(body.StoreID)
		base := strings.TrimRight(evoURL, "/")
		phone := contactPhone
		if strings.HasSuffix(phone, "@lid") {
			if resolved := resolveLIDPhone(base, evoKey, evoInstance, phone); resolved != "" {
				phone = resolved
			}
		}
		payload, _ := json.Marshal(map[string]string{"number": phone, "text": body.Text})
		var status int
		var respBody []byte
		respBody, status, sendErr = evoCall("POST",
			fmt.Sprintf("%s/message/sendText/%s", base, evoInstance),
			evoKey, payload)
		if sendErr == nil && status != 200 && status != 201 {
			sendErr = fmt.Errorf("WhatsApp service error %d: %s", status, string(respBody))
		}
		fromID = evoInstance
	}

	if sendErr != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "WhatsApp send error: " + sendErr.Error()})
		return
	}

	now := time.Now()
	saved := saveProcurementWhatsAppMessage(
		storeID, "out",
		fromID,
		[]string{contactPhone},
		body.Text, "text",
		phoneNumberID,
		wamid,
		nil, false, nil, &now,
	)

	resp := map[string]interface{}{"success": true}
	if saved != nil {
		resp["message_id"] = saved.ID.Hex()
		resp["code"] = saved.Code
	}
	json.NewEncoder(w).Encode(resp)
}

// POST /v1/procurement-message-threads/{phone}/send-media
// Accepts multipart/form-data with fields: file, store_id, caption.
// Detects file type, uploads to Meta and sends as image/document/audio.
func SendThreadMediaHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to parse form"})
		return
	}

	contactPhone, _ := url.PathUnescape(mux.Vars(r)["phone"])
	if contactPhone == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing phone"})
		return
	}

	storeIDStr := r.FormValue("store_id")
	if storeIDStr == "" {
		storeIDStr = r.URL.Query().Get("store_id")
	}
	storeID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	caption := r.FormValue("caption")

	file, handler, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read file"})
		return
	}

	// Detect MIME type from Content-Type header, or infer from extension
	mimeType := handler.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(handler.Filename))
		switch ext {
		case ".jpg", ".jpeg":
			mimeType = "image/jpeg"
		case ".png":
			mimeType = "image/png"
		case ".gif":
			mimeType = "image/gif"
		case ".webp":
			mimeType = "image/webp"
		case ".pdf":
			mimeType = "application/pdf"
		case ".ogg":
			mimeType = "audio/ogg; codecs=opus"
		case ".mp3":
			mimeType = "audio/mpeg"
		case ".wav":
			mimeType = "audio/wav"
		case ".webm":
			mimeType = "audio/webm"
		case ".m4a", ".mp4":
			mimeType = "audio/mp4"
		default:
			mimeType = "application/octet-stream"
		}
	}

	// Resolve Meta config (store_rfq → bot)
	phoneNumberID, accessToken := rfqMetaConfig(storeIDStr, "store_rfq")
	if phoneNumberID == "" || accessToken == "" {
		phoneNumberID, accessToken = rfqMetaConfig(storeIDStr, "bot")
	}
	if phoneNumberID == "" || accessToken == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "WhatsApp WABA not configured for this store"})
		return
	}

	var sendErr error
	mediaKind := "document"
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		mediaKind = "image"
		sendErr = metaSendImage(phoneNumberID, accessToken, contactPhone,
			"data:"+mimeType+";base64,"+base64Encode(fileBytes), caption)
	case strings.HasPrefix(mimeType, "audio/"):
		mediaKind = "audio"
		sendErr = metaSendAudio(phoneNumberID, accessToken, contactPhone, fileBytes, mimeType)
	default:
		sendErr = metaSendDocument(phoneNumberID, accessToken, contactPhone,
			"data:"+mimeType+";base64,"+base64Encode(fileBytes),
			mimeType, handler.Filename, caption)
	}

	if sendErr != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "send failed: " + sendErr.Error()})
		return
	}

	// Save the sent file so the sender can play/download it in the chat (S3 or local disk).
	var attachments []models.ProcurementAttachment
	relKey := fmt.Sprintf("attachments/%s/%d/%s", storeIDStr, time.Now().UnixNano(), handler.Filename)
	s3Settings := loadAdminS3Settings()
	if attURL := saveAttachment(s3Settings, relKey, fileBytes, mimeType); attURL != "" {
		attachments = []models.ProcurementAttachment{{
			Filename:    handler.Filename,
			ContentType: mimeType,
			Size:        int64(len(fileBytes)),
			URL:         attURL,
		}}
	}

	now := time.Now()
	bodyText := caption
	if bodyText == "" {
		bodyText = handler.Filename
	}
	saved := saveProcurementWhatsAppMessage(
		storeID, "out",
		phoneNumberID,
		[]string{contactPhone},
		bodyText, mediaKind,
		phoneNumberID,
		"", // wamid not available for media sends yet
		attachments, false, nil, &now,
	)

	resp := map[string]interface{}{"success": true}
	if saved != nil {
		resp["message_id"] = saved.ID.Hex()
	}
	json.NewEncoder(w).Encode(resp)
}

// base64Encode encodes bytes to a base64 string.
func base64Encode(data []byte) string {
	return b64.StdEncoding.EncodeToString(data)
}

// POST /v1/procurement-message-threads/{phone}/pin?store_id=&type=
func PinThreadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	contact, _ := url.PathUnescape(mux.Vars(r)["phone"])
	msgType := r.URL.Query().Get("type")
	if msgType == "" {
		msgType = "whatsapp"
	}
	if err := models.PinContact(storeID, contact, msgType); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// DELETE /v1/procurement-message-threads/{phone}/pin?store_id=&type=
func UnpinThreadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	contact, _ := url.PathUnescape(mux.Vars(r)["phone"])
	msgType := r.URL.Query().Get("type")
	if msgType == "" {
		msgType = "whatsapp"
	}
	if err := models.UnpinContact(storeID, contact, msgType); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
