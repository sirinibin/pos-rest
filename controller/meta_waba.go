package controller

// Meta Cloud API (Official WhatsApp Business API) helpers.
// All bot send/receive operations use these functions — no Evolution API involved.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"strings"
	"time"
)

const metaAPIVersion = "v17.0"
const metaGraphBase = "https://graph.facebook.com/" + metaAPIVersion

// ── Incoming webhook payload (Meta Cloud API) ────────────────────────────────

type metaWebhookPayload struct {
	Object string       `json:"object"`
	Entry  []metaEntry  `json:"entry"`
}

type metaEntry struct {
	ID      string        `json:"id"`
	Changes []metaChange  `json:"changes"`
}

type metaChange struct {
	Value metaChangeValue `json:"value"`
	Field string          `json:"field"`
}

type metaChangeValue struct {
	MessagingProduct string        `json:"messaging_product"`
	Metadata         metaMetadata  `json:"metadata"`
	Contacts         []metaContact `json:"contacts"`
	Messages         []metaMessage `json:"messages"`
	Statuses         []metaStatus  `json:"statuses"`
}

type metaMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type metaContact struct {
	Profile metaProfile `json:"profile"`
	WaID    string      `json:"wa_id"`
}

type metaProfile struct {
	Name string `json:"name"`
}

type metaStatus struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
	RecipientID  string `json:"recipient_id"`
}

type metaMessage struct {
	From      string               `json:"from"`
	ID        string               `json:"id"`
	Timestamp string               `json:"timestamp"`
	Type      string               `json:"type"`
	Text      *metaTextMsg         `json:"text,omitempty"`
	Image     *metaMediaMsg        `json:"image,omitempty"`
	Document  *metaDocMsg          `json:"document,omitempty"`
	Audio     *metaMediaMsg        `json:"audio,omitempty"`
	Video     *metaMediaMsg        `json:"video,omitempty"`
	Sticker   *metaMediaMsg        `json:"sticker,omitempty"`
	Context   *metaContext         `json:"context,omitempty"`
	Contacts  []metaContactPayload `json:"contacts,omitempty"`
}

// metaContactPayload represents a contact card sent via WhatsApp (type: "contacts").
type metaContactPayload struct {
	Name   metaContactName    `json:"name"`
	Phones []metaContactPhone `json:"phones"`
}

type metaContactName struct {
	FormattedName string `json:"formatted_name"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
}

type metaContactPhone struct {
	Phone string `json:"phone"`
	Type  string `json:"type"`
	WaID  string `json:"wa_id"`
}

type metaTextMsg struct {
	Body string `json:"body"`
}

type metaMediaMsg struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
}

type metaDocMsg struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
}

type metaContext struct {
	From string `json:"from"`
	ID   string `json:"id"` // quoted message ID
}

// ── Media download ───────────────────────────────────────────────────────────

// metaGetMediaURL fetches the download URL for a Meta media object.
func metaGetMediaURL(mediaID, accessToken string) (string, string, error) {
	url := fmt.Sprintf("%s/%s?access_token=%s", metaGraphBase, mediaID, accessToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.URL == "" {
		return "", "", fmt.Errorf("meta media info parse failed: %s", string(body))
	}
	return r.URL, r.MimeType, nil
}

// metaDownloadMedia downloads a Meta media file and returns it as a base64 data URI.
func metaDownloadMedia(mediaID, accessToken string) (string, string, error) {
	downloadURL, mimeType, err := metaGetMediaURL(mediaID, accessToken)
	if err != nil {
		return "", "", err
	}
	req, _ := http.NewRequest("GET", downloadURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if mimeType == "" {
		mimeType = detectImageMIME(data)
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return "data:" + mimeType + ";base64," + b64, mimeType, nil
}

// metaUploadMedia uploads bytes to Meta's media store and returns the media ID.
// Used when sending documents/images to suppliers.
func metaUploadMedia(phoneNumberID, accessToken, mimeType, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("messaging_product", "whatsapp")
	// Use CreatePart so we can set the correct Content-Type on the file part.
	// CreateFormFile always uses application/octet-stream which Meta rejects.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", mimeType)
	fw, err := w.CreatePart(h)
	if err != nil {
		return "", err
	}
	fw.Write(data)
	w.Close()

	url := fmt.Sprintf("%s/%s/media", metaGraphBase, phoneNumberID)
	req, _ := http.NewRequest("POST", url, &buf)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.ID == "" {
		return "", fmt.Errorf("meta upload failed: %s", string(body))
	}
	return r.ID, nil
}

// ── Message sending ──────────────────────────────────────────────────────────

func metaSendJSON(phoneNumberID, accessToken string, payload interface{}) ([]byte, int, error) {
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s/messages", metaGraphBase, phoneNumberID)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode, nil
}

// pickWABASender returns the official WhatsApp (WABA) sender to use: the
// store RFQ number when fully configured, else the bot number, else none.
func pickWABASender(rfqPhoneID, rfqToken, botPhoneID, botToken string) (phoneNumberID, accessToken string) {
	if rfqPhoneID != "" && rfqToken != "" {
		return rfqPhoneID, rfqToken
	}
	if botPhoneID != "" && botToken != "" {
		return botPhoneID, botToken
	}
	return "", ""
}

// wabaSendConfig is the store's connected official WhatsApp sender. Whenever
// it is non-empty every WhatsApp send must go through it, never Evolution.
var wabaSendConfig = func(storeID string) (phoneNumberID, accessToken string) {
	rID, rTok := rfqMetaConfig(storeID, "store_rfq")
	bID, bTok := rfqMetaConfig(storeID, "bot")
	return pickWABASender(rID, rTok, bID, bTok)
}

// sendWABADocument is metaSendDocument, swappable in tests.
var sendWABADocument = metaSendDocument

// metaSendText sends a plain text message.
// Returns the WhatsApp message ID from the API response.
func metaSendText(phoneNumberID, accessToken, to, text string) (string, error) {
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "text",
		"text":              map[string]interface{}{"preview_url": false, "body": text},
	}
	body, status, err := metaSendJSON(phoneNumberID, accessToken, payload)
	if err != nil {
		return "", err
	}
	if status != 200 && status != 201 {
		return "", fmt.Errorf("meta send text failed (%d): %s", status, string(body))
	}
	return parseMetaMsgID(body), nil
}

// metaSendImage sends an image by uploading it first.
// dataURI is a "data:image/jpeg;base64,..." string.
func metaSendImage(phoneNumberID, accessToken, to, dataURI, caption string) error {
	if dataURI == "" {
		return nil
	}
	mimeType, rawB64 := splitDataURI(dataURI)
	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return err
	}
	ext := mimeToExt(mimeType)
	mediaID, err := metaUploadMedia(phoneNumberID, accessToken, mimeType, "image"+ext, data)
	if err != nil {
		log.Printf("meta: image upload failed: %v", err)
		return err
	}
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "image",
		"image":             map[string]interface{}{"id": mediaID, "caption": caption},
	}
	body, status, err := metaSendJSON(phoneNumberID, accessToken, payload)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("meta send image failed (%d): %s", status, string(body))
	}
	return nil
}

// metaSendDocument sends a document file.
func metaSendDocument(phoneNumberID, accessToken, to, dataURI, mimeType, filename, caption string) error {
	if dataURI == "" {
		return nil
	}
	_, rawB64 := splitDataURI(dataURI)
	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return err
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if filename == "" {
		filename = "document" + mimeToExt(mimeType)
	}
	mediaID, err := metaUploadMedia(phoneNumberID, accessToken, mimeType, filename, data)
	if err != nil {
		log.Printf("meta: document upload failed: %v", err)
		return err
	}
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "document",
		"document": map[string]interface{}{
			"id":       mediaID,
			"filename": filename,
			"caption":  caption,
		},
	}
	body, status, err := metaSendJSON(phoneNumberID, accessToken, payload)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("meta send document failed (%d): %s", status, string(body))
	}
	return nil
}

// metaSendTemplate sends a WABA template message.
func metaSendTemplate(phoneNumberID, accessToken, to, templateName, languageCode string, components []interface{}) error {
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "template",
		"template": map[string]interface{}{
			"name":       templateName,
			"language":   map[string]interface{}{"code": languageCode},
			"components": components,
		},
	}
	body, status, err := metaSendJSON(phoneNumberID, accessToken, payload)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("meta send template failed (%d): %s", status, string(body))
	}
	return nil
}

// metaMarkRead marks a message as read (shows blue double tick to sender).
func metaMarkRead(phoneNumberID, accessToken, messageID string) {
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	}
	metaSendJSON(phoneNumberID, accessToken, payload) // best-effort
}

// ── Template listing ─────────────────────────────────────────────────────────

type WABATemplate struct {
	Name        string                 `json:"name"`
	Status      string                 `json:"status"`
	Language    string                 `json:"language"`
	Category    string                 `json:"category"`
	Components  []WABATemplateComponent `json:"components"`
}

type WABATemplateComponent struct {
	Type       string              `json:"type"`
	Format     string              `json:"format,omitempty"`
	Text       string              `json:"text,omitempty"`
	Parameters []WABATemplateParam `json:"parameters,omitempty"`
}

type WABATemplateParam struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// metaListTemplates fetches APPROVED templates for the given WABA ID.
func metaListTemplates(wabaID, accessToken string) ([]WABATemplate, error) {
	url := fmt.Sprintf("%s/%s/message_templates?status=APPROVED&limit=100&access_token=%s",
		metaGraphBase, wabaID, accessToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r struct {
		Data []struct {
			Name       string                 `json:"name"`
			Status     string                 `json:"status"`
			Language   string                 `json:"language"`
			Category   string                 `json:"category"`
			Components []WABATemplateComponent `json:"components"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse error: %s", string(body))
	}
	if r.Error != nil {
		return nil, fmt.Errorf("meta API error: %s", r.Error.Message)
	}
	templates := make([]WABATemplate, len(r.Data))
	for i, d := range r.Data {
		templates[i] = WABATemplate{
			Name:       d.Name,
			Status:     d.Status,
			Language:   d.Language,
			Category:   d.Category,
			Components: d.Components,
		}
	}
	return templates, nil
}

// ── Utility ──────────────────────────────────────────────────────────────────

func parseMetaMsgID(body []byte) string {
	var r struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &r); err == nil && len(r.Messages) > 0 {
		return r.Messages[0].ID
	}
	return ""
}

func mimeToExt(mime string) string {
	switch {
	case strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg"):
		return ".jpg"
	case strings.Contains(mime, "png"):
		return ".png"
	case strings.Contains(mime, "webp"):
		return ".webp"
	case strings.Contains(mime, "pdf"):
		return ".pdf"
	case strings.Contains(mime, "spreadsheet") || strings.Contains(mime, "excel"):
		return ".xlsx"
	case strings.Contains(mime, "csv"):
		return ".csv"
	default:
		return ".bin"
	}
}

// metaDeleteMessage retracts a sent WhatsApp message from the recipient's device.
// Meta Cloud API message deletion requires v21.0+; v17.0 returns "does not support this operation".
// Uses: DELETE https://graph.facebook.com/v21.0/{wamid}
func metaDeleteMessage(phoneNumberID, accessToken, wamID string) error {
	encodedWamID := strings.NewReplacer("=", "%3D", "+", "%2B").Replace(wamID)
	deleteURL := fmt.Sprintf("https://graph.facebook.com/v21.0/%s", encodedWamID)
	log.Printf("procurement: meta delete URL: %s", deleteURL)
	req, _ := http.NewRequest("DELETE", deleteURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("procurement: meta delete response %d: %s", resp.StatusCode, string(body))
	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		return nil
	}
	return fmt.Errorf("meta delete returned %d: %s", resp.StatusCode, string(body))
}

// transcodeWebmToOgg converts WebM/Opus audio to OGG/Opus using FFmpeg.
// Meta WhatsApp API does not accept audio/webm but does accept audio/ogg.
func transcodeWebmToOgg(webmData []byte) ([]byte, error) {
	tmpIn, err := os.CreateTemp("", "voice_in_*.webm")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpIn.Name())
	if _, err := tmpIn.Write(webmData); err != nil {
		tmpIn.Close()
		return nil, err
	}
	tmpIn.Close()

	outPath := tmpIn.Name() + ".ogg"
	defer os.Remove(outPath)

	out, err := exec.Command("ffmpeg", "-y", "-i", tmpIn.Name(), "-c:a", "libopus", "-vbr", "on", outPath).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", err, string(out))
	}
	return os.ReadFile(outPath)
}

// metaSendAudio uploads raw audio bytes and sends them as a WhatsApp audio message.
func metaSendAudio(phoneNumberID, accessToken, to string, data []byte, mimeType string) error {
	if mimeType == "" {
		mimeType = "audio/ogg; codecs=opus"
	}
	// Meta rejects audio/webm — transcode to OGG/Opus via FFmpeg.
	if strings.Contains(mimeType, "webm") {
		if transcoded, err := transcodeWebmToOgg(data); err != nil {
			log.Printf("meta: webm→ogg transcode failed: %v", err)
		} else {
			data = transcoded
			mimeType = "audio/ogg; codecs=opus"
		}
	}
	filename := "voice.ogg"
	switch {
	case strings.Contains(mimeType, "mp4") || strings.Contains(mimeType, "m4a"):
		filename = "voice.m4a"
	case strings.Contains(mimeType, "wav"):
		filename = "voice.wav"
	case strings.Contains(mimeType, "mpeg"):
		filename = "voice.mp3"
	}
	mediaID, err := metaUploadMedia(phoneNumberID, accessToken, mimeType, filename, data)
	if err != nil {
		return err
	}
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "audio",
		"audio":             map[string]interface{}{"id": mediaID},
	}
	body, status, err := metaSendJSON(phoneNumberID, accessToken, payload)
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("meta send audio failed (%d): %s", status, string(body))
	}
	return nil
}
