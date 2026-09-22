package controller

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

	threads, total, err := models.ListContactThreads(storeID, msgType, search, page, limit)
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
			sendErr = fmt.Errorf("Evolution API error %d: %s", status, string(respBody))
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

	// Save the sent file to disk so the sender can play/download it in the chat.
	var attachments []models.ProcurementAttachment
	attDir := fmt.Sprintf("./attachments/%s/%d", storeIDStr, time.Now().UnixNano())
	if err := os.MkdirAll(attDir, 0755); err == nil {
		destPath := filepath.Join(attDir, handler.Filename)
		if werr := os.WriteFile(destPath, fileBytes, 0644); werr == nil {
			att := models.ProcurementAttachment{
				Filename:    handler.Filename,
				ContentType: mimeType,
				Size:        int64(len(fileBytes)),
				URL:         "/attachments/" + storeIDStr + "/" + fmt.Sprintf("%d", time.Now().UnixNano()) + "/" + handler.Filename,
			}
			// Use the actual path we wrote to.
			att.URL = "/" + strings.TrimPrefix(filepath.ToSlash(destPath), "./")
			attachments = []models.ProcurementAttachment{att}
		}
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
