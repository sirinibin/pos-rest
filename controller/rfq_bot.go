package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ledongthuc/pdf"
	excelize "github.com/xuri/excelize/v2"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mimeFromFilename(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// rfqMetaConfig returns the Meta Cloud API credentials for the given role ("bot" | "store_rfq").
func rfqMetaConfig(storeIDStr, role string) (phoneNumberID, accessToken string) {
	if storeIDStr == "" {
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		return
	}
	if role == "store_rfq" {
		return store.Settings.StoreRFQWABAPhoneNumberID, store.Settings.StoreRFQWABAAccessToken
	}
	return store.Settings.BotWABAPhoneNumberID, store.Settings.BotWABAAccessToken
}

// rfqSaveMetaSettings persists Meta Cloud API credentials for the given role.
func rfqSaveMetaSettings(storeID primitive.ObjectID, role, phoneNumberID, accessToken string) error {
	var upd bson.M
	if role == "store_rfq" {
		upd = bson.M{
			"settings.store_rfq_waba_phone_number_id": phoneNumberID,
			"settings.store_rfq_waba_access_token":    accessToken,
		}
	} else {
		upd = bson.M{
			"settings.bot_waba_phone_number_id": phoneNumberID,
			"settings.bot_waba_access_token":    accessToken,
		}
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx, bson.M{"_id": storeID}, bson.M{"$set": upd})
	return err
}

// rfqClearMetaSettings clears stored Meta Cloud API credentials for the given role.
func rfqClearMetaSettings(storeID primitive.ObjectID, role string) error {
	var upd bson.M
	if role == "store_rfq" {
		upd = bson.M{
			"settings.store_rfq_waba_phone_number_id": "",
			"settings.store_rfq_waba_access_token":    "",
		}
	} else {
		upd = bson.M{
			"settings.bot_waba_phone_number_id": "",
			"settings.bot_waba_access_token":    "",
		}
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx, bson.M{"_id": storeID}, bson.M{"$set": upd})
	return err
}

// rfqCheckMetaStatus verifies Meta Cloud API credentials by fetching the phone number profile.
// Returns connected=true, the display phone number, and the WABA business account ID on success.
// The WABA ID fetch is best-effort and won't cause the verification to fail.
func rfqCheckMetaStatus(phoneNumberID, accessToken string) (connected bool, phone, wabaID string) {
	if phoneNumberID == "" || accessToken == "" {
		return false, "", ""
	}

	// Step 1: verify credentials with a basic fields call
	verifyURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/%s?fields=display_phone_number,verified_name&access_token=%s",
		phoneNumberID, accessToken,
	)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(verifyURL)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		return false, "", ""
	}
	var basic struct {
		DisplayPhone string `json:"display_phone_number"`
	}
	json.NewDecoder(resp.Body).Decode(&basic)
	resp.Body.Close()
	phone = basic.DisplayPhone

	// Step 2: try to fetch the WABA business account ID (best-effort, ignore errors)
	wabaURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/%s?fields=whatsapp_business_account{id}&access_token=%s",
		phoneNumberID, accessToken,
	)
	wabaResp, wabaErr := (&http.Client{Timeout: 6 * time.Second}).Get(wabaURL)
	if wabaErr == nil && wabaResp.StatusCode == 200 {
		var wabaData struct {
			WABA *struct {
				ID string `json:"id"`
			} `json:"whatsapp_business_account"`
		}
		json.NewDecoder(wabaResp.Body).Decode(&wabaData)
		wabaResp.Body.Close()
		if wabaData.WABA != nil {
			wabaID = wabaData.WABA.ID
		}
	} else if wabaResp != nil {
		wabaResp.Body.Close()
	}

	return true, phone, wabaID
}

// buildWebhookURL constructs the public webhook URL for the bot instance.
// It prefers the PUBLIC_SERVER_URL environment variable, falling back to the request's Host.
func buildWebhookURL(r *http.Request, storeID string) string {
	base := os.Getenv("PUBLIC_SERVER_URL")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return fmt.Sprintf("%s/v1/rfq-bot/webhook?store_id=%s", strings.TrimRight(base, "/"), storeID)
}

// ── 1. Bot WhatsApp Connect / QR / Status / Disconnect ───────────────────────

// connectMetaWhatsApp saves Meta Cloud API credentials for the given role ("bot" | "store_rfq").
// It verifies the credentials against Meta's Graph API before persisting them.
func connectMetaWhatsApp(w http.ResponseWriter, r *http.Request, role string) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		StoreID       string `json:"store_id"`
		PhoneNumberID string `json:"phone_number_id"`
		AccessToken   string `json:"access_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if body.StoreID == "" || body.PhoneNumberID == "" || body.AccessToken == "" {
		http.Error(w, `{"error":"store_id, phone_number_id and access_token are required"}`, http.StatusBadRequest)
		return
	}

	storeObjID, err := primitive.ObjectIDFromHex(body.StoreID)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	if _, err := models.FindStoreByID(&storeObjID, bson.M{}); err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}

	// Verify credentials with Meta and auto-fetch WABA Business Account ID
	connected, phone, wabaID := rfqCheckMetaStatus(body.PhoneNumberID, body.AccessToken)
	if !connected {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"error":"Meta API credentials invalid or phone number not found"}`)
		return
	}

	if err := rfqSaveMetaSettings(storeObjID, role, body.PhoneNumberID, body.AccessToken); err != nil {
		http.Error(w, `{"error":"failed to save store settings"}`, http.StatusInternalServerError)
		return
	}

	// Persist WABA Business Account ID if auto-fetched (needed for template listing)
	if wabaID != "" && role == "bot" {
		col := db.Client("").Database(db.GetPosDB()).Collection("store")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		col.UpdateOne(ctx, bson.M{"_id": storeObjID}, bson.M{"$set": bson.M{
			"settings.bot_waba_business_account_id": wabaID,
		}})
	}

	webhookURL := buildWebhookURL(r, body.StoreID)
	fmt.Fprintf(w, `{"success":true,"phone":%q,"waba_id":%q,"webhook_url":%q}`, phone, wabaID, webhookURL)
}

// POST /v1/rfq-bot/connect
func ConnectBotWhatsApp(w http.ResponseWriter, r *http.Request) {
	connectMetaWhatsApp(w, r, "bot")
}

// GET /v1/rfq-bot/qr?store_id=...  — not used with Meta Cloud API
func GetBotWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	http.Error(w, `{"error":"QR code not supported with Meta Cloud API"}`, http.StatusGone)
}

// GET /v1/rfq-bot/status?store_id=...
func GetBotWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	rfqMetaStatus(w, r, "bot")
}

// DELETE /v1/rfq-bot/disconnect?store_id=...
func DisconnectBotWhatsApp(w http.ResponseWriter, r *http.Request) {
	rfqMetaDisconnect(w, r, "bot")
}

// ── 2. Store RFQ WhatsApp Connect / QR / Status / Disconnect ─────────────────

// POST /v1/rfq-store/connect
func ConnectStoreRFQWhatsApp(w http.ResponseWriter, r *http.Request) {
	connectMetaWhatsApp(w, r, "store_rfq")
}

// GET /v1/rfq-store/qr?store_id=...  — not used with Meta Cloud API
func GetStoreRFQWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	http.Error(w, `{"error":"QR code not supported with Meta Cloud API"}`, http.StatusGone)
}

// GET /v1/rfq-store/status?store_id=...
func GetStoreRFQWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	rfqMetaStatus(w, r, "store_rfq")
}

// DELETE /v1/rfq-store/disconnect?store_id=...
func DisconnectStoreRFQWhatsApp(w http.ResponseWriter, r *http.Request) {
	rfqMetaDisconnect(w, r, "store_rfq")
}

func rfqMetaStatus(w http.ResponseWriter, r *http.Request, role string) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	phoneNumberID, accessToken := rfqMetaConfig(storeIDStr, role)
	if phoneNumberID == "" {
		fmt.Fprint(w, `{"connected":false}`)
		return
	}
	connected, phone, _ := rfqCheckMetaStatus(phoneNumberID, accessToken)
	if connected {
		fmt.Fprintf(w, `{"connected":true,"phone":%q,"phone_number_id":%q}`, phone, phoneNumberID)
	} else {
		fmt.Fprint(w, `{"connected":false}`)
	}
}

func rfqMetaDisconnect(w http.ResponseWriter, r *http.Request, role string) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		http.Error(w, `{"error":"store_id required"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	if err := rfqClearMetaSettings(storeObjID, role); err != nil {
		http.Error(w, `{"error":"failed to clear store settings"}`, http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, `{"success":true}`)
}

// ── 3. Check LLM Connection ───────────────────────────────────────────────────

// POST /v1/rfq-bot/check-llm
// Body: { "provider": "openai", "api_key": "sk-..." }
func CheckRFQLLMConnection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	var connected bool
	var errMsg string

	switch strings.ToLower(body.Provider) {
	case "openai":
		connected, errMsg = checkOpenAIConnection(body.APIKey)
	case "anthropic":
		connected, errMsg = checkAnthropicConnection(body.APIKey)
	case "gemini":
		connected, errMsg = checkGeminiConnection(body.APIKey)
	default:
		errMsg = "unknown provider"
	}

	if connected {
		fmt.Fprintf(w, `{"connected":true}`)
	} else {
		fmt.Fprintf(w, `{"connected":false,"error":%q}`, errMsg)
	}
}

// TestGoogleMapsHandler verifies a Google Maps API key by running a real Places API search
// without saving anything to the database.
// api_key is optional — falls back to the store's saved key so the user can test a freshly
// entered key before saving the store form.
// GET /v1/rfq-bot/test-google-maps?store_id=...&keyword=...&market=...&api_key=...
func TestGoogleMapsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	apiKey := strings.TrimSpace(r.URL.Query().Get("api_key"))
	if apiKey == "" {
		apiKey = store.Settings.GoogleMapsAPIKey
	}
	if apiKey == "" {
		http.Error(w, `{"error":"Enter a Google Maps API key above first"}`, http.StatusBadRequest)
		return
	}
	keyword := strings.TrimSpace(r.URL.Query().Get("keyword"))
	if keyword == "" {
		http.Error(w, `{"error":"keyword is required"}`, http.StatusBadRequest)
		return
	}
	market := strings.TrimSpace(r.URL.Query().Get("market"))

	suppliers, err := searchGoogleMapsSuppliers(apiKey, keyword, market, storeObjID, 10)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error()})
		return
	}
	type place struct {
		Name    string  `json:"name"`
		Phone   string  `json:"phone"`
		Address string  `json:"address"`
		Rating  float64 `json:"rating"`
		MapsURL string  `json:"maps_url"`
	}
	places := make([]place, len(suppliers))
	for i, s := range suppliers {
		places[i] = place{Name: s.Name, Phone: s.Phone, Address: s.Address, Rating: s.Rating, MapsURL: s.GoogleMapsURL}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(places), "keyword": keyword, "market": market, "places": places,
	})
}

// UploadWABAMedia uploads a file to Meta's media store and returns the media ID.
// POST /v1/rfq-bot/upload-media?store_id=...
// multipart/form-data: field "file"
func UploadWABAMedia(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		http.Error(w, `{"error":"store_id required"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		http.Error(w, `{"error":"Bot WhatsApp not connected"}`, http.StatusBadRequest)
		return
	}

	r.ParseMultipartForm(20 << 20) // 20 MB limit
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"file required"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, `{"error":"failed to read file"}`, http.StatusInternalServerError)
		return
	}
	mimeType := mimeFromFilename(header.Filename)
	if ct := header.Header.Get("Content-Type"); ct != "" && ct != "application/octet-stream" {
		mimeType = ct
	}
	mediaID, err := metaUploadMedia(phoneNumberID, accessToken, mimeType, header.Filename, data)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	fmt.Fprintf(w, `{"media_id":%q,"filename":%q}`, mediaID, header.Filename)
}

// SaveWABABusinessAccountID saves the WABA Business Account ID for the given store.
// POST /v1/rfq-bot/waba-business-account-id
func SaveWABABusinessAccountID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		StoreID             string `json:"store_id"`
		WABABusinessAccountID string `json:"waba_business_account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if body.StoreID == "" {
		http.Error(w, `{"error":"store_id required"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(body.StoreID)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = col.UpdateOne(ctx, bson.M{"_id": storeObjID}, bson.M{"$set": bson.M{
		"settings.bot_waba_business_account_id": body.WABABusinessAccountID,
	}})
	if err != nil {
		http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, `{"success":true}`)
}

// GetWABATemplates lists APPROVED WABA templates for the store's business account.
// GET /v1/rfq-bot/waba-templates?store_id=...
func GetWABATemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		http.Error(w, `{"error":"store_id required"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	wabaID := store.Settings.BotWABABusinessAccountID
	accessToken := store.Settings.BotWABAAccessToken
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	if accessToken == "" {
		http.Error(w, `{"error":"Access Token not found. Please connect the Bot WhatsApp first."}`, http.StatusBadRequest)
		return
	}
	// Auto-fetch WABA ID from Meta if not stored
	if wabaID == "" && phoneNumberID != "" {
		_, _, fetchedWABAID := rfqCheckMetaStatus(phoneNumberID, accessToken)
		if fetchedWABAID != "" {
			wabaID = fetchedWABAID
			// Persist so subsequent calls are fast
			col := db.Client("").Database(db.GetPosDB()).Collection("store")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			col.UpdateOne(ctx, bson.M{"_id": storeObjID}, bson.M{"$set": bson.M{
				"settings.bot_waba_business_account_id": wabaID,
			}})
		}
	}
	if wabaID == "" {
		http.Error(w, `{"error":"Could not determine WABA Business Account ID. Enter it manually in the store form."}`, http.StatusBadRequest)
		return
	}
	templates, err := metaListTemplates(wabaID, accessToken)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	out, _ := json.Marshal(map[string]interface{}{"templates": templates})
	w.Write(out)
}

// SendWABATestMessage sends a template message to a test number.
// POST /v1/rfq-bot/waba-test-message
func SendWABATestMessage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		StoreID      string        `json:"store_id"`
		To           string        `json:"to"`
		TemplateName string        `json:"template_name"`
		LanguageCode string        `json:"language_code"`
		Components   []interface{} `json:"components"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(body.StoreID)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		http.Error(w, `{"error":"WABA Phone Number ID and Access Token are required"}`, http.StatusBadRequest)
		return
	}
	langCode := body.LanguageCode
	if langCode == "" {
		langCode = "en"
	}
	if err := metaSendTemplate(phoneNumberID, accessToken, body.To, body.TemplateName, langCode, body.Components); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	fmt.Fprintf(w, `{"sent":true}`)
}

// CheckWhatsAppNumber — Meta Cloud API does not expose a per-number existence check.
// GET /v1/rfq-bot/check-whatsapp?store_id=...&phone=...
func CheckWhatsAppNumber(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Meta Cloud API does not provide a public API to check if an arbitrary number
	// has WhatsApp. Return exists=true so callers can still proceed; the send will
	// fail naturally if the number is unreachable.
	fmt.Fprint(w, `{"exists":true,"note":"WhatsApp number check not available with Meta Cloud API"}`)
}

func checkOpenAIConnection(apiKey string) (bool, string) {
	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func checkAnthropicConnection(apiKey string) (bool, string) {
	payload := []byte(`{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`)
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func checkGeminiConnection(apiKey string) (bool, string) {
	u := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Get(u)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200, fmt.Sprintf("HTTP %d", resp.StatusCode)
}

// ── 4. Webhook — incoming messages (Evolution API format OR Meta native format) ─

// VerifyRFQBotWebhook handles Meta's one-time GET challenge to verify the webhook URL.
// Meta sends: GET /v1/rfq-bot/webhook?hub.mode=subscribe&hub.verify_token=TOKEN&hub.challenge=XYZ
// We respond with the raw challenge string if the token matches.
// Set META_WEBHOOK_VERIFY_TOKEN env var to the token you enter in Meta's dashboard.
// GET /v1/rfq-bot/webhook
func VerifyRFQBotWebhook(w http.ResponseWriter, r *http.Request) {
	expected := os.Getenv("META_WEBHOOK_VERIFY_TOKEN")
	if expected == "" {
		expected = "startpos-rfq-verify"
	}
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")
	if mode == "subscribe" && token == expected {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, challenge)
		return
	}
	http.Error(w, "Forbidden", http.StatusForbidden)
}

// POST /v1/rfq-bot/webhook?store_id=...
// Handles incoming Meta Cloud API webhook messages.
func HandleRFQBotWebhook(w http.ResponseWriter, r *http.Request) {
	storeIDStr := r.URL.Query().Get("store_id")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))

	if storeIDStr == "" {
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	// Parse Meta Cloud API webhook payload
	var payload metaWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("rfq_bot: webhook JSON parse error: %v body=%.300s", err, string(bodyBytes))
		return
	}

	if payload.Object != "whatsapp_business_account" {
		log.Printf("rfq_bot: webhook unexpected object=%q", payload.Object)
		return
	}

	// Load store once
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		log.Printf("rfq_bot: webhook store load failed: %v", err)
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			val := change.Value
			for _, msg := range val.Messages {
				go processMetaIncomingMessage(store, storeObjID, val, msg)
			}
		}
	}
}

// processMetaIncomingMessage handles a single incoming message from Meta Cloud API.
func processMetaIncomingMessage(store *models.Store, storeObjID primitive.ObjectID, val metaChangeValue, msg metaMessage) {
	fromPhone := msg.From

	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken

	// Mark as read (best-effort)
	if msg.ID != "" && phoneNumberID != "" && accessToken != "" {
		metaMarkRead(phoneNumberID, accessToken, msg.ID)
	}

	// Skip non-content types only (reactions, stickers have no useful payload).
	// Audio, video, voice are now fully handled below.
	if msg.Type == "reaction" || msg.Type == "sticker" {
		return
	}

	// Extract text / caption from the message.
	var text string
	switch msg.Type {
	case "text":
		if msg.Text != nil {
			text = msg.Text.Body
		}
	case "image":
		if msg.Image != nil {
			text = msg.Image.Caption
		}
	case "video":
		if msg.Video != nil {
			text = msg.Video.Caption
		}
	case "document":
		if msg.Document != nil {
			text = msg.Document.Caption
		}
	}

	// Parse message timestamp.
	var msgDate *time.Time
	if ts, err2 := strconv.ParseInt(msg.Timestamp, 10, 64); err2 == nil && ts > 0 {
		t := time.Unix(ts, 0).UTC()
		msgDate = &t
	}

	// Download all media, save to disk, build attachment list for the procurement inbox.
	// Media is downloaded BEFORE saving the procurement message so attachments are stored together.
	var procAttachments []models.ProcurementAttachment
	var mediaURLs []string // data URIs kept for LLM image classification
	var metaMediaIDs []string
	var documents []models.RFQDocument
	var docTextParts []string

	saveToDisk := func(dataURI, mimeType, filename string) {
		_, b64data := splitDataURI(dataURI)
		raw, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil || len(raw) == 0 {
			return
		}
		url := saveEmailAttachment(storeObjID.Hex(), "wa_"+msg.ID, filename, raw)
		if url != "" {
			procAttachments = append(procAttachments, models.ProcurementAttachment{
				Filename:    filename,
				ContentType: mimeType,
				Size:        int64(len(raw)),
				URL:         url,
			})
		}
	}

	switch msg.Type {
	case "image":
		if msg.Image != nil && msg.Image.ID != "" && accessToken != "" {
			metaMediaIDs = append(metaMediaIDs, msg.Image.ID)
			if dataURI, mimeType, err := metaDownloadMedia(msg.Image.ID, accessToken); err != nil {
				log.Printf("rfq_bot: meta image download failed: %v", err)
			} else {
				if mimeType == "" {
					mimeType = msg.Image.MimeType
				}
				filename := "image_1" + mimeTypeToExt(mimeType)
				saveToDisk(dataURI, mimeType, filename)
				mediaURLs = append(mediaURLs, dataURI)
			}
		}

	case "audio", "voice":
		if msg.Audio != nil && msg.Audio.ID != "" && accessToken != "" {
			metaMediaIDs = append(metaMediaIDs, msg.Audio.ID)
			if dataURI, mimeType, err := metaDownloadMedia(msg.Audio.ID, accessToken); err != nil {
				log.Printf("rfq_bot: meta audio download failed: %v", err)
			} else {
				if mimeType == "" {
					mimeType = msg.Audio.MimeType
				}
				saveToDisk(dataURI, mimeType, "audio_1"+mimeTypeToExt(mimeType))
			}
		}

	case "video":
		if msg.Video != nil && msg.Video.ID != "" && accessToken != "" {
			metaMediaIDs = append(metaMediaIDs, msg.Video.ID)
			if dataURI, mimeType, err := metaDownloadMedia(msg.Video.ID, accessToken); err != nil {
				log.Printf("rfq_bot: meta video download failed: %v", err)
			} else {
				if mimeType == "" {
					mimeType = msg.Video.MimeType
				}
				saveToDisk(dataURI, mimeType, "video_1"+mimeTypeToExt(mimeType))
			}
		}

	case "document":
		if msg.Document != nil && msg.Document.ID != "" && accessToken != "" {
			metaMediaIDs = append(metaMediaIDs, msg.Document.ID)
			if dataURI, mimeType, err := metaDownloadMedia(msg.Document.ID, accessToken); err != nil {
				log.Printf("rfq_bot: meta document download failed: %v", err)
			} else {
				if mimeType == "" {
					mimeType = msg.Document.MimeType
				}
				filename := msg.Document.Filename
				if filename == "" {
					filename = "document_1" + mimeTypeToExt(mimeType)
				}
				saveToDisk(dataURI, mimeType, filename)
				documents = append(documents, models.RFQDocument{
					URL:      dataURI,
					FileName: filename,
					MimeType: mimeType,
				})
				_, b64data := splitDataURI(dataURI)
				if raw, err2 := base64.StdEncoding.DecodeString(b64data); err2 == nil {
					if extracted := extractDocumentText(raw, mimeType, filename); extracted != "" {
						docTextParts = append(docTextParts, extracted)
					}
				}
			}
		}
	}

	// Save every incoming message to the procurement inbox with its attachments.
	procMsg := saveProcurementWhatsAppMessage(storeObjID, "in", fromPhone, []string{phoneNumberID}, text, msg.Type, phoneNumberID, msg.ID, procAttachments, false, nil, msgDate)

	// If the sender is replying to one of the bot's relay messages, route as buyer follow-up
	if msg.Context != nil && msg.Context.ID != "" {
		rfq, supplierPhone, err := models.FindRFQByBuyerRelayMsgID(storeObjID, msg.Context.ID)
		if err == nil && supplierPhone != "" {
			go handleMetaBuyerFollowup(store, rfq, supplierPhone, fromPhone, senderName(val, fromPhone), msg)
			return
		}
	}

	// ── Allowed-senders whitelist routing ─────────────────────────────────────
	// Numbers in RFQAllowedSenders are RFQ buyers; all other numbers are
	// suppliers expected to send quotation replies.
	if len(store.Settings.RFQAllowedSenders) > 0 {
		allowed := false
		for _, s := range store.Settings.RFQAllowedSenders {
			if strings.TrimSpace(s) == fromPhone {
				allowed = true
				break
			}
		}
		if !allowed {
			// We already know it's a supplier — mark the procurement message immediately
			// without waiting for LLM analysis.
			if procMsg != nil {
				go models.LinkMessageAsQuotation(procMsg.ID, nil, "") //nolint:errcheck
			}
			var procMsgID *primitive.ObjectID
			if procMsg != nil {
				id := procMsg.ID
				procMsgID = &id
			}
			go handleMetaSupplierReply(store, fromPhone, senderName(val, fromPhone), msg, procMsgID)
			return
		}
	}

	hasImages := len(mediaURLs) > 0
	hasDocs := len(documents) > 0
	hasAnyMedia := len(procAttachments) > 0

	if text == "" && !hasAnyMedia {
		log.Printf("rfq_bot: empty message from %s (type=%s) — skipping RFQ flow", fromPhone, msg.Type)
		return
	}

	// Build RFQ record using already-downloaded media.
	rfq := &models.RFQReceived{
		StoreID:    storeObjID,
		FromPhone:  fromPhone,
		FromName:   senderName(val, fromPhone),
		BuyerMsgID: msg.ID,
		Source:     "whatsapp",
		Status:     "received",
	}
	rfq.TextContent = text
	rfq.MediaURLs = mediaURLs
	rfq.MetaMediaIDs = metaMediaIDs
	rfq.Documents = documents
	if len(docTextParts) > 0 {
		rfq.ExtractedText = strings.Join(docTextParts, "\n\n")
	}

	switch {
	case text != "" && (hasImages || hasDocs):
		rfq.MessageType = "mixed"
	case hasImages && hasDocs:
		rfq.MessageType = "mixed"
	case hasImages:
		rfq.MessageType = "image"
	case hasDocs:
		rfq.MessageType = "document"
	default:
		rfq.MessageType = msg.Type // preserves "audio", "video", "text"
	}

	if procMsg != nil {
		rfq.ProcurementMessageID = &procMsg.ID
		rfq.ProcurementMessageCode = procMsg.Code
	}

	// Gate RFQ creation on auto-create flag.
	if store.Settings.DisableAutoRFQFromWhatsApp {
		log.Printf("rfq_bot: auto RFQ from WhatsApp disabled for store %s — skipping RFQ creation for %s", storeObjID.Hex(), fromPhone)
		return
	}

	// LLM classification: "rfq" → create RFQ; "quotation" → label as Supplier Quotation; "other" → skip.
	llmText := text
	if llmText == "" {
		for _, part := range docTextParts {
			llmText += part + "\n"
		}
	}
	waClassify, _ := classifyIncomingMessage(store, llmText, mediaURLs)
	if waClassify == "quotation" {
		log.Printf("rfq_bot: WhatsApp from %s classified as supplier quotation — labeling", fromPhone)
		if procMsg != nil {
			go models.LinkMessageAsQuotation(procMsg.ID, nil, "")
		}
		return
	}
	if waClassify != "rfq" {
		log.Printf("rfq_bot: WhatsApp message from %s is not an RFQ — skipping", fromPhone)
		return
	}

	if err := models.CreateRFQReceived(rfq); err != nil {
		log.Printf("rfq_bot: failed to save RFQ: %v", err)
		return
	}

	// Back-link: update the procurement inbox message to point to this RFQ.
	if procMsg != nil {
		models.LinkProcurementMessageToRFQ(procMsg.ID, rfq.ID)
	}

	// Timeline: input received
	srcLabel := "WhatsApp"
	senderLabel := rfq.FromPhone
	if rfq.FromName != "" {
		phone := rfq.FromPhone
		if !strings.HasPrefix(phone, "+") {
			phone = "+" + phone
		}
		senderLabel = rfq.FromName + " (" + phone + ")"
	}
	attachInfo := ""
	if len(rfq.MediaURLs) > 0 && len(rfq.Documents) > 0 {
		attachInfo = fmt.Sprintf(" with %d image(s) and %d document(s)", len(rfq.MediaURLs), len(rfq.Documents))
	} else if len(rfq.MediaURLs) > 0 {
		attachInfo = fmt.Sprintf(" with %d image(s)", len(rfq.MediaURLs))
	} else if len(rfq.Documents) > 0 {
		attachInfo = fmt.Sprintf(" with %d document(s)", len(rfq.Documents))
	}
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "input_received",
		Message: fmt.Sprintf("Input received from %s via %s%s", senderLabel, srcLabel, attachInfo),
		Icon:    "bi-whatsapp", Color: "success",
		Details: map[string]interface{}{"source": rfq.Source, "from_phone": rfq.FromPhone, "message_type": rfq.MessageType},
	})
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "rfq_created",
		Message: fmt.Sprintf("RFQ created with code %s", rfq.Code),
		Icon:    "bi-file-earmark-check", Color: "primary",
		Details: map[string]interface{}{"code": rfq.Code},
	})

	BroadcastRFQEvent(storeObjID.Hex(), "rfq_received")
	go processRFQ(rfq, storeObjID)
}

// senderName extracts the contact display name for a given phone number from the Meta webhook.
func senderName(val metaChangeValue, fromPhone string) string {
	for _, c := range val.Contacts {
		if c.WaID == fromPhone {
			return c.Profile.Name
		}
	}
	return ""
}

// parseMsgIDFromEvoResponse extracts the WhatsApp message ID from an Evolution API send response.
func parseMsgIDFromEvoResponse(body []byte) string {
	var resp struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &resp); err == nil {
		return resp.Key.ID
	}
	return ""
}

// handleMetaBuyerFollowup forwards a buyer's reply (to a bot relay) to the correct supplier via Meta WABA.
func handleMetaBuyerFollowup(store *models.Store, rfq *models.RFQReceived, supplierPhone, buyerPhone, buyerName string, msg metaMessage) {
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		log.Printf("rfq_bot: buyer follow-up — WABA not configured, can't relay to supplier %s", supplierPhone)
		return
	}

	name := buyerName
	if name == "" {
		name = buyerPhone
	}
	header := fmt.Sprintf("📩 *Follow-up from buyer %s (+%s)*\n\n", name, buyerPhone)

	text := ""
	if msg.Text != nil {
		text = msg.Text.Body
	} else if msg.Image != nil {
		text = msg.Image.Caption
	} else if msg.Document != nil {
		text = msg.Document.Caption
	}
	if text == "" && msg.Image == nil && msg.Document == nil {
		text = "(no text)"
	}

	if text != "" {
		if _, err := metaSendText(phoneNumberID, accessToken, supplierPhone, header+text); err != nil {
			log.Printf("rfq_bot: buyer follow-up text to supplier %s failed: %v", supplierPhone, err)
		}
	}
	if msg.Image != nil && msg.Image.ID != "" {
		if dataURI, _, err := metaDownloadMedia(msg.Image.ID, accessToken); err == nil {
			metaSendImage(phoneNumberID, accessToken, supplierPhone, dataURI, "Image from buyer")
		}
	}
	if msg.Document != nil && msg.Document.ID != "" {
		if dataURI, mime, err := metaDownloadMedia(msg.Document.ID, accessToken); err == nil {
			metaSendDocument(phoneNumberID, accessToken, supplierPhone, dataURI, mime, msg.Document.Filename, "Document from buyer")
		}
	}
	log.Printf("rfq_bot: buyer follow-up from %s relayed to supplier %s (rfq=%s)", buyerPhone, supplierPhone, rfq.ID.Hex())
}

// handleMetaSupplierReply forwards a supplier's message back to the original buyer via Meta WABA,
// records the raw reply on the RFQ, and triggers async LLM price extraction.
// supplierReplyAnalysis holds the result of the unified LLM analysis of a supplier reply.
type supplierReplyAnalysis struct {
	RFQCode      string                      // extracted RFQ code (e.g. "RFQ-0015"), empty if not mentioned
	IsQuotation  bool                        // true when the message contains unit prices
	GeneralNotes string                      // quotation-wide conditions: validity, delivery, payment terms
	Prices       []models.SupplierReplyPrice // pre-extracted prices (zero-indexed relative to rfq.Products if available)
	SupplierName string                      // supplier company/person name from the document
	SupplierPhone string                     // supplier phone from the document (international format preferred)
}

// extractRFQCodeFromText scans raw text for a pattern matching the store's RFQ code format
// (e.g. "RFQ-0015") without needing an LLM call.
func extractRFQCodeFromText(text, prefix string) string {
	if prefix == "" {
		prefix = "RFQ"
	}
	// Build pattern: <PREFIX>-<digits> (case-insensitive)
	pattern := `(?i)\b` + regexp.QuoteMeta(strings.ToUpper(prefix)) + `-\d+\b`
	re := regexp.MustCompile(pattern)
	m := re.FindString(text)
	if m != "" {
		return strings.ToUpper(m)
	}
	return ""
}

// analyzeSupplierReply calls the store's LLM to extract RFQ reference, quotation status and prices
// from a raw supplier reply. rfqProducts is the product list from the matched RFQ (may be nil/empty
// when the RFQ is not yet identified; the LLM will still extract whatever is in the message).
// pdfBase64s contains base64-encoded PDF bytes for vision-capable providers (Gemini, Anthropic).
// providerOverride/modelOverride allow the caller to choose a specific LLM; empty string = use store default.
func analyzeSupplierReply(store *models.Store, msgText string, pdfBase64s []string, rfqProducts []models.RFQProduct, providerOverride, modelOverride string) supplierReplyAnalysis {
	provider := strings.ToLower(providerOverride)
	model := modelOverride
	if provider == "" {
		provider = strings.ToLower(store.Settings.RFQLLMProvider)
	}
	if model == "" {
		model = store.Settings.RFQLLMModel
	}
	apiKey := resolveExtractionAPIKey(provider, &store.Settings)
	if apiKey == "" {
		apiKey = store.Settings.RFQLLMAPIKey
	}
	if (msgText == "" && len(pdfBase64s) == 0) || apiKey == "" {
		return supplierReplyAnalysis{}
	}

	var productContext strings.Builder
	for i, p := range rfqProducts {
		productContext.WriteString(fmt.Sprintf("%d. %s", i+1, p.Name))
		if p.PartNo != "" {
			productContext.WriteString(" (part: " + p.PartNo + ")")
		}
		if p.Quantity > 0 {
			productContext.WriteString(fmt.Sprintf(", qty: %.0f %s", p.Quantity, p.Unit))
		}
		productContext.WriteString("\n")
	}

	productSection := ""
	if productContext.Len() > 0 {
		productSection = "\nRFQ Products (for matching):\n" + productContext.String()
	}

	msgSection := ""
	if msgText != "" {
		msgSection = "\nSupplier Message:\n" + msgText
	}

	prompt := fmt.Sprintf(`You are a procurement assistant. Analyse the following supplier reply.%s%s

Extract the following and return ONLY valid JSON (no explanation):
{
  "rfq_code": "<RFQ code if explicitly mentioned, e.g. RFQ-0015, or empty string>",
  "supplier_name": "<name of the company or person that issued this quotation/reply, empty string if not found>",
  "supplier_phone": "<phone number of the supplier shown in the document, in international format if possible (e.g. 966550988062), empty string if not found>",
  "is_quotation": <true if the message contains unit prices, false otherwise>,
  "general_notes": "<quotation-wide conditions only: validity period, delivery lead time, payment terms, warranty, overall terms — e.g. 'Validity: 2 days, Delivery: 7 days'. Empty string if none.>",
  "prices": [
    {
      "product_index": <0-based index from the product list above, -1 if unknown>,
      "product_name": "<as mentioned in the message>",
      "part_no": "<part number if mentioned, else empty>",
      "unit_price": <numeric price EXCLUDING VAT, 0 if not given>,
      "quantity": <numeric quantity if mentioned, 0 if not>,
      "currency": "<e.g. AED, SAR, USD; default SAR if not specified>",
      "vat_included": <false if price is excluding VAT, true if price already includes VAT>,
      "notes": "<product-specific notes only: dimensions, specs, MOQ, brand, model variant — empty string if nothing product-specific>"
    }
  ]
}
If no prices are mentioned, return an empty prices array.
IMPORTANT: general_notes is for conditions that apply to the whole quotation (validity, delivery, payment). Do NOT repeat them in each product's notes field.`, productSection, msgSection)

	// For scanned PDFs (no extractable text), auto-upgrade to a vision-capable provider.
	// Try the configured provider first; if it can't handle PDFs, switch to Gemini or Anthropic.
	if msgText == "" && len(pdfBase64s) > 0 && provider != "gemini" && provider != "anthropic" {
		if k := resolveExtractionAPIKey("gemini", &store.Settings); k != "" {
			log.Printf("rfq_bot: analyzeSupplierReply: %q can't handle scanned PDF — auto-switching to Gemini", provider)
			provider = "gemini"
			apiKey = k
			model = "" // let callGemini pick default
		} else if k := resolveExtractionAPIKey("anthropic", &store.Settings); k != "" {
			log.Printf("rfq_bot: analyzeSupplierReply: %q can't handle scanned PDF — auto-switching to Anthropic", provider)
			provider = "anthropic"
			apiKey = k
			model = "" // let callAnthropic pick default
		} else {
			log.Printf("rfq_bot: analyzeSupplierReply: no vision-capable LLM found for scanned PDF (provider=%q)", provider)
			return supplierReplyAnalysis{}
		}
	}

	var responseText string
	var llmErr error
	// For Gemini and Anthropic, use vision-capable calls so image-based PDFs are readable.
	switch provider {
	case "gemini":
		responseText, llmErr = callGeminiExtractRFQ(apiKey, model, prompt, nil, pdfBase64s, 2048)
	case "anthropic":
		responseText, llmErr = callAnthropicExtractRFQ(apiKey, model, prompt, nil, pdfBase64s, 2048)
	case "openai":
		responseText, llmErr = callOpenAI(apiKey, model, prompt, "")
	default:
		// OpenAI-compatible providers — text mode (PDFs already text-extracted above).
		baseURL := openAICompatBaseURL(provider)
		responseText, llmErr = callOpenAICompatExtractRFQ(apiKey, model, prompt, nil, 2048, baseURL)
	}
	if llmErr != nil {
		log.Printf("rfq_bot: analyzeSupplierReply LLM error: %v", llmErr)
		return supplierReplyAnalysis{}
	}

	jsonStr := extractJSONFromLLMResponse(responseText)
	var raw struct {
		RFQCode       string `json:"rfq_code"`
		SupplierName  string `json:"supplier_name"`
		SupplierPhone string `json:"supplier_phone"`
		IsQuotation   bool   `json:"is_quotation"`
		GeneralNotes  string `json:"general_notes"`
		Prices        []struct {
			ProductIndex int     `json:"product_index"`
			ProductName  string  `json:"product_name"`
			PartNo       string  `json:"part_no"`
			UnitPrice    float64 `json:"unit_price"`
			Quantity     float64 `json:"quantity"`
			Currency     string  `json:"currency"`
			VATIncluded  bool    `json:"vat_included"`
			Notes        string  `json:"notes"`
		} `json:"prices"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		log.Printf("rfq_bot: analyzeSupplierReply JSON parse error: %v (raw=%q)", err, jsonStr)
		return supplierReplyAnalysis{}
	}

	prices := make([]models.SupplierReplyPrice, 0, len(raw.Prices))
	for _, p := range raw.Prices {
		if p.UnitPrice <= 0 {
			continue
		}
		idx := p.ProductIndex
		if idx < 0 {
			idx = 0
		}
		prices = append(prices, models.SupplierReplyPrice{
			ProductIndex: idx,
			ProductName:  p.ProductName,
			PartNo:       p.PartNo,
			UnitPrice:    p.UnitPrice,
			Quantity:     p.Quantity,
			Currency:     p.Currency,
			VATIncluded:  p.VATIncluded,
			Notes:        p.Notes,
		})
	}
	return supplierReplyAnalysis{
		RFQCode:       strings.TrimSpace(raw.RFQCode),
		IsQuotation:   raw.IsQuotation,
		GeneralNotes:  strings.TrimSpace(raw.GeneralNotes),
		Prices:        prices,
		SupplierName:  strings.TrimSpace(raw.SupplierName),
		SupplierPhone: strings.TrimSpace(raw.SupplierPhone),
	}
}

func handleMetaSupplierReply(store *models.Store, supplierPhone, supplierName string, msg metaMessage, procMsgID *primitive.ObjectID) {
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken

	name := supplierName
	if name == "" {
		name = supplierPhone
	}

	// ── Collect message text ──────────────────────────────────────────────────
	msgText := ""
	if msg.Text != nil {
		msgText = msg.Text.Body
	} else if msg.Image != nil {
		msgText = msg.Image.Caption
	} else if msg.Document != nil {
		msgText = msg.Document.Caption
	}

	// ── Download document attachment early so we can extract text for LLM ────
	// When a supplier sends a PDF-only quotation (no caption), msgText is empty and
	// the LLM cannot detect it as a quotation without seeing the document content.
	var docData []byte
	var docMIME string
	if msg.Document != nil && msg.Document.ID != "" && accessToken != "" {
		if dataURI, mime, dlErr := metaDownloadMedia(msg.Document.ID, accessToken); dlErr == nil {
			docMIME = mime
			if parts := strings.SplitN(dataURI, ",", 2); len(parts) == 2 {
				if decoded, decErr := base64.StdEncoding.DecodeString(parts[1]); decErr == nil {
					docData = decoded
					if msgText == "" {
						extracted := extractDocumentText(decoded, mime, msg.Document.Filename)
						if extracted != "" {
							msgText = "[Document]\n" + extracted
						}
					}
				}
			}
		}
	}

	// ── Step 1: try to extract RFQ code from text (regex, no LLM cost) ───────
	rfqCodePrefix := ""
	if store.RFQReceivedSerialNumber.Prefix != "" {
		rfqCodePrefix = store.RFQReceivedSerialNumber.Prefix
	}
	codeFromText := extractRFQCodeFromText(msgText, rfqCodePrefix)

	// ── Step 2: find the RFQ ──────────────────────────────────────────────────
	var originalRFQ *models.RFQReceived
	var routingMethod string

	if codeFromText != "" {
		if rfq, err := models.FindRFQByCode(store.ID, codeFromText); err == nil {
			originalRFQ = rfq
			routingMethod = "code:" + codeFromText
			log.Printf("rfq_bot: supplier reply routed by extracted code %q to RFQ %s", codeFromText, rfq.ID.Hex())
		}
	}
	if originalRFQ == nil {
		// Fall back: most recent RFQ forwarded to this supplier's phone
		rfq, err := models.FindLatestRFQBySupplierPhone(store.ID, supplierPhone)
		if err != nil {
			log.Printf("rfq_bot: no forwarded RFQ found for supplier %s — ignoring reply", supplierPhone)
			return
		}
		originalRFQ = rfq
		routingMethod = "phone"
	}

	buyerPhone := originalRFQ.FromPhone

	// ── Step 3: LLM analysis — extract RFQ code (if regex missed), prices ────
	// Run with the now-known RFQ product list for better price matching.
	analysis := analyzeSupplierReply(store, msgText, nil, originalRFQ.Products, "", "")

	// If LLM found a code that regex missed and it points to a different RFQ, re-route.
	if analysis.RFQCode != "" && analysis.RFQCode != codeFromText && routingMethod == "phone" {
		if rfq, err := models.FindRFQByCode(store.ID, analysis.RFQCode); err == nil {
			originalRFQ = rfq
			buyerPhone = rfq.FromPhone
			routingMethod = "llm_code:" + analysis.RFQCode
			log.Printf("rfq_bot: supplier reply re-routed by LLM-extracted code %q to RFQ %s", analysis.RFQCode, rfq.ID.Hex())
		}
	}

	// ── Step 4: save the reply, already enriched with LLM results ────────────
	extractionStatus := "pending"
	if store.Settings.RFQLLMAPIKey == "" {
		extractionStatus = "done" // no LLM configured, nothing to extract
	} else if analysis.IsQuotation || len(analysis.Prices) > 0 {
		extractionStatus = "done" // already done in step 3
	}

	reply := models.SupplierReply{
		SupplierName:     name,
		SupplierPhone:    supplierPhone,
		ReceivedAt:       time.Now(),
		RawText:          msgText,
		IsQuotation:      analysis.IsQuotation,
		GeneralNotes:     analysis.GeneralNotes,
		Prices:           analysis.Prices,
		ExtractionStatus: extractionStatus,
	}
	if addErr := models.AddSupplierReplyToRFQ(originalRFQ.StoreID, originalRFQ.ID, reply); addErr != nil {
		log.Printf("rfq_bot: failed to save supplier reply: %v", addErr)
	} else {
		models.AppendRFQLog(originalRFQ.StoreID, originalRFQ.ID, models.RFQActivityLog{
			Step: "supplier_replied",
			Message: fmt.Sprintf("Supplier %s (+%s) has replied (routed by %s)", name, supplierPhone, routingMethod),
			Icon:  "bi-chat-left-text", Color: "info",
			Details: map[string]interface{}{
				"supplier_name":  name,
				"supplier_phone": supplierPhone,
				"routing":        routingMethod,
				"is_quotation":   analysis.IsQuotation,
				"price_count":    len(analysis.Prices),
			},
		})

		if analysis.IsQuotation && len(analysis.Prices) > 0 {
			modelName := store.Settings.RFQLLMModel
			if modelName == "" {
				modelName = store.Settings.RFQLLMProvider
			}
			models.AppendRFQLog(originalRFQ.StoreID, originalRFQ.ID, models.RFQActivityLog{
				Step: "prices_extracted",
				Message: fmt.Sprintf("Extracted %d product price(s) from %s reply using %s",
					len(analysis.Prices), name, modelName),
				Icon: "bi-cpu", Color: "secondary",
				Details: map[string]interface{}{"supplier_name": name, "price_count": len(analysis.Prices), "model": modelName},
			})
			models.AppendRFQLog(originalRFQ.StoreID, originalRFQ.ID, models.RFQActivityLog{
				Step: "prices_updated",
				Message: fmt.Sprintf("Updated RFQ with unit prices from supplier: %s (%d item(s) priced)",
					name, len(analysis.Prices)),
				Icon: "bi-currency-dollar", Color: "success",
				Details: map[string]interface{}{"supplier_name": name, "is_quotation": true, "price_count": len(analysis.Prices)},
			})
		} else if extractionStatus == "pending" {
			// LLM didn't find prices yet — run the detailed extraction pass with full RFQ product context
			go func() {
				updated, err := models.FindRFQReceivedByID(originalRFQ.ID, originalRFQ.StoreID)
				if err != nil {
					return
				}
				for i := len(updated.SupplierReplies) - 1; i >= 0; i-- {
					r := updated.SupplierReplies[i]
					if r.SupplierPhone == supplierPhone && r.ExtractionStatus == "pending" {
						extractSupplierPrices(store, updated, &updated.SupplierReplies[i])
						break
					}
				}
			}()
		}

		// Auto-link the procurement message to this RFQ as a supplier quotation.
		// The initial marking (is_supplier_quotation=true) was already done at routing time;
		// here we add the specific RFQ link now that we know which RFQ this reply belongs to.
		if procMsgID != nil {
			rfqIDCopy := originalRFQ.ID
			if linkErr := models.LinkMessageAsQuotation(*procMsgID, &rfqIDCopy, originalRFQ.Code); linkErr != nil {
				log.Printf("rfq_bot: auto-link procurement msg %s to RFQ %s failed: %v", procMsgID.Hex(), originalRFQ.Code, linkErr)
			}
		}
	}

	// Relay to buyer if WABA is configured and buyer phone is known.
	if buyerPhone != "" && phoneNumberID != "" && accessToken != "" {
		header := fmt.Sprintf("📩 *Reply from supplier %s (+%s)*\n\n", name, supplierPhone)
		displayText := msgText
		if displayText == "" {
			displayText = "(no text)"
		}
		relayMsgID, sendErr := metaSendText(phoneNumberID, accessToken, buyerPhone, header+displayText)
		if sendErr != nil {
			log.Printf("rfq_bot: supplier reply relay to buyer %s failed: %v", buyerPhone, sendErr)
		} else if relayMsgID != "" {
			_ = models.AddBuyerRelayToRFQ(originalRFQ.StoreID, originalRFQ.ID, models.BuyerRelayRecord{
				MsgID:         relayMsgID,
				SupplierPhone: supplierPhone,
				SentAt:        time.Now(),
			})
		}
		if msg.Image != nil && msg.Image.ID != "" {
			if dataURI, _, dlErr := metaDownloadMedia(msg.Image.ID, accessToken); dlErr == nil {
				metaSendImage(phoneNumberID, accessToken, buyerPhone, dataURI, "Image from supplier")
			}
		}
		if msg.Document != nil && msg.Document.ID != "" {
			if len(docData) > 0 {
				// Reuse the already-downloaded document data.
				dataURI := "data:" + docMIME + ";base64," + base64.StdEncoding.EncodeToString(docData)
				metaSendDocument(phoneNumberID, accessToken, buyerPhone, dataURI, docMIME, msg.Document.Filename, "Document from supplier")
			} else if dataURI, mime, dlErr := metaDownloadMedia(msg.Document.ID, accessToken); dlErr == nil {
				metaSendDocument(phoneNumberID, accessToken, buyerPhone, dataURI, mime, msg.Document.Filename, "Document from supplier")
			}
		}
	}
	log.Printf("rfq_bot: recorded and relayed supplier %s reply (rfq=%s)", supplierPhone, originalRFQ.ID.Hex())
}

// extractSupplierPrices uses the store's LLM to detect if a supplier reply is a quotation
// and extract unit prices per product. Updates the SupplierReply in the DB.
func extractSupplierPrices(store *models.Store, rfq *models.RFQReceived, reply *models.SupplierReply) {
	if reply.RawText == "" || len(rfq.Products) == 0 {
		_ = models.UpdateSupplierReplyPrices(rfq.StoreID, rfq.ID, reply.ID, nil, false, "done", "no text or no products")
		return
	}

	// Build product list for the prompt.
	var productLines strings.Builder
	for i, p := range rfq.Products {
		productLines.WriteString(fmt.Sprintf("%d. %s", i+1, p.Name))
		if p.PartNo != "" {
			productLines.WriteString(" (part: " + p.PartNo + ")")
		}
		if p.Quantity > 0 {
			productLines.WriteString(fmt.Sprintf(", qty: %.0f %s", p.Quantity, p.Unit))
		}
		productLines.WriteString("\n")
	}

	prompt := fmt.Sprintf(`You are a procurement assistant. The following is a supplier's reply to an RFQ.

RFQ Products:
%s
Supplier Reply:
%s

Task:
1. Determine if this reply contains unit prices for any products (is_quotation: true/false).
2. For each product that has a price, extract:
   - product_index (0-based, matching the list above)
   - product_name
   - part_no (if mentioned)
   - unit_price (numeric, base currency)
   - currency (e.g. SAR, USD; default SAR if not specified)
   - notes (any conditions like MOQ, lead time)

Return ONLY valid JSON:
{
  "is_quotation": true,
  "prices": [
    {"product_index": 0, "product_name": "...", "part_no": "...", "unit_price": 150.00, "currency": "SAR", "notes": "..."}
  ]
}`, productLines.String(), reply.RawText)

	var responseText string
	var llmErr error
	switch strings.ToLower(store.Settings.RFQLLMProvider) {
	case "openai":
		responseText, llmErr = callOpenAI(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, prompt, "")
	case "anthropic":
		responseText, llmErr = callAnthropic(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, prompt, "")
	case "gemini":
		responseText, llmErr = callGemini(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, prompt, "")
	default:
		_ = models.UpdateSupplierReplyPrices(rfq.StoreID, rfq.ID, reply.ID, nil, false, "done", "LLM not configured")
		return
	}
	if llmErr != nil {
		_ = models.UpdateSupplierReplyPrices(rfq.StoreID, rfq.ID, reply.ID, nil, false, "failed", llmErr.Error())
		return
	}

	// Parse JSON from LLM response (strip markdown code fences if present).
	jsonStr := extractJSONFromLLMResponse(responseText)
	var result struct {
		IsQuotation bool `json:"is_quotation"`
		Prices      []struct {
			ProductIndex int     `json:"product_index"`
			ProductName  string  `json:"product_name"`
			PartNo       string  `json:"part_no"`
			UnitPrice    float64 `json:"unit_price"`
			Currency     string  `json:"currency"`
			Notes        string  `json:"notes"`
		} `json:"prices"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		_ = models.UpdateSupplierReplyPrices(rfq.StoreID, rfq.ID, reply.ID, nil, false, "failed", "JSON parse: "+err.Error())
		return
	}

	prices := make([]models.SupplierReplyPrice, 0, len(result.Prices))
	for _, p := range result.Prices {
		prices = append(prices, models.SupplierReplyPrice{
			ProductIndex: p.ProductIndex,
			ProductName:  p.ProductName,
			PartNo:       p.PartNo,
			UnitPrice:    p.UnitPrice,
			Currency:     p.Currency,
			Notes:        p.Notes,
		})
	}
	_ = models.UpdateSupplierReplyPrices(rfq.StoreID, rfq.ID, reply.ID, prices, result.IsQuotation, "done", "")
	log.Printf("rfq_bot: supplier price extraction done for reply %s (is_quotation=%v, %d prices)", reply.ID.Hex(), result.IsQuotation, len(prices))

	modelName := store.Settings.RFQLLMModel
	if modelName == "" {
		modelName = store.Settings.RFQLLMProvider
	}
	if result.IsQuotation && len(prices) > 0 {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step: "prices_extracted",
			Message: fmt.Sprintf("Extracted %d product price(s) from %s reply using LLM model: %s",
				len(prices), reply.SupplierName, modelName),
			Icon: "bi-cpu", Color: "secondary",
			Details: map[string]interface{}{"supplier_name": reply.SupplierName, "price_count": len(prices), "model": modelName},
		})
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step: "prices_updated",
			Message: fmt.Sprintf("Updated RFQ with unit prices from supplier: %s (%d item(s) priced)",
				reply.SupplierName, len(prices)),
			Icon: "bi-currency-dollar", Color: "success",
			Details: map[string]interface{}{"supplier_name": reply.SupplierName, "is_quotation": result.IsQuotation, "price_count": len(prices)},
		})
	} else {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step:    "prices_extracted",
			Message: fmt.Sprintf("Reply from %s analysed — not a quotation (no prices found)", reply.SupplierName),
			Icon:    "bi-info-circle", Color: "secondary",
			Details: map[string]interface{}{"supplier_name": reply.SupplierName, "is_quotation": false},
		})
	}
}

// extractJSONFromLLMResponse strips markdown code fences and extracts the JSON object/array.
func extractJSONFromLLMResponse(text string) string {
	text = strings.TrimSpace(text)
	// Strip <think>...</think> blocks produced by reasoning models (e.g. DeepSeek R1).
	for {
		start := strings.Index(text, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(text, "</think>")
		if end < 0 {
			text = strings.TrimSpace(text[:start])
			break
		}
		text = strings.TrimSpace(text[:start] + text[end+len("</think>"):])
	}
	if idx := strings.Index(text, "```json"); idx >= 0 {
		text = text[idx+7:]
		if end := strings.Index(text, "```"); end >= 0 {
			text = text[:end]
		}
	} else if idx := strings.Index(text, "```"); idx >= 0 {
		text = text[idx+3:]
		if end := strings.Index(text, "```"); end >= 0 {
			text = text[:end]
		}
	}
	text = strings.TrimSpace(text)
	// Find first '{' or '[' and last '}' or ']'
	start := strings.IndexAny(text, "{[")
	end := strings.LastIndexAny(text, "}]")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return text
}

// legacy stub — referenced by old test files; do not call in new code
func handleBuyerFollowupLegacy(store *models.Store, rfq *models.RFQReceived, supplierPhone, buyerPhone, buyerName string) {
	log.Printf("rfq_bot: handleBuyerFollowupLegacy called (use handleMetaBuyerFollowup instead)")
}

// legacy stub — referenced by old test files; do not call in new code
func handleSupplierReplyLegacy(store *models.Store, supplierPhone, supplierName string, text string) {
	log.Printf("rfq_bot: handleSupplierReplyLegacy called (use handleMetaSupplierReply instead)")
}

// --- kept for legacy test references ---
type evolutionWebhookPayload struct {
	Event    string `json:"event"`
	Instance string `json:"instance"`
	Data     struct {
		Key struct {
			RemoteJid string `json:"remoteJid"`
			FromMe    bool   `json:"fromMe"`
			ID        string `json:"id"`
		} `json:"key"`
		PushName    string `json:"pushName"`
		Message     struct {
			Conversation string `json:"conversation"`
			DocumentWithCaptionMessage struct {
				Message struct {
					DocumentMessage struct {
						URL      string `json:"url"`
						FileName string `json:"fileName"`
						Mimetype string `json:"mimetype"`
						Caption  string `json:"caption"`
					} `json:"documentMessage"`
				} `json:"message"`
			} `json:"documentWithCaptionMessage"`
		} `json:"message"`
		MessageType      string `json:"messageType"`
		MessageTimestamp int64  `json:"messageTimestamp"`
	} `json:"data"`
}

// handleBuyerFollowup — kept for legacy test compilation; redirects to Meta version stub.
func handleBuyerFollowup(store *models.Store, rfq *models.RFQReceived, supplierPhone, buyerPhone, buyerName string, payload evolutionWebhookPayload) {
	handleBuyerFollowupLegacy(store, rfq, supplierPhone, buyerPhone, buyerName)
}

// handleSupplierReply — kept for legacy test compilation; redirects to Meta version stub.
func handleSupplierReply(store *models.Store, supplierPhone, supplierName string, text string, payload evolutionWebhookPayload) {
	handleSupplierReplyLegacy(store, supplierPhone, supplierName, text)
}

// ── 5. RFQ Processing Pipeline ────────────────────────────────────────────────

func processRFQ(rfq *models.RFQReceived, storeID primitive.ObjectID) {
	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		log.Printf("rfq_bot: store not found: %v", err)
		return
	}
	if !store.Settings.EnableAIRFQBot {
		return
	}

	// Mark as processing
	rfq.Status = "processing"
	models.UpdateRFQReceived(rfq)

	// Download images for LLM (vision models)
	var imageBase64s []string
	for _, u := range rfq.MediaURLs {
		b64, err := downloadImageAsBase64(u)
		if err != nil {
			log.Printf("rfq_bot[%s]: image download failed (%v)", rfq.ID.Hex(), err)
			continue
		}
		if b64 == "" {
			log.Printf("rfq_bot[%s]: image download returned empty", rfq.ID.Hex())
			continue
		}
		log.Printf("rfq_bot[%s]: image ready len=%d prefix=%.40s", rfq.ID.Hex(), len(b64), b64)
		imageBase64s = append(imageBase64s, b64)
	}

	// Build LLM context: original text + extracted doc content + filename hints.
	// ExtractedText contains spreadsheet/CSV rows — useful for categorization but NOT sent to suppliers.
	llmText := rfq.TextContent
	// For manually created RFQs with products but no text, synthesise a description.
	if llmText == "" && rfq.Source == "manual" && len(rfq.Products) > 0 {
		var parts []string
		for _, p := range rfq.Products {
			s := p.Name
			if p.PartNo != "" {
				s = p.PartNo + " — " + s
			}
			if p.Quantity > 0 {
				s += fmt.Sprintf(", qty %.0f", p.Quantity)
				if p.Unit != "" {
					s += " " + p.Unit
				}
			}
			parts = append(parts, s)
		}
		llmText = "Request for quotation:\n" + strings.Join(parts, "\n")
	}
	if rfq.ExtractedText != "" {
		if llmText != "" {
			llmText += "\n\n" + rfq.ExtractedText
		} else {
			llmText = rfq.ExtractedText
		}
	}
	if len(rfq.Documents) > 0 {
		var docHints []string
		for _, d := range rfq.Documents {
			if d.FileName != "" {
				docHints = append(docHints, d.FileName)
			}
		}
		if len(docHints) > 0 {
			hint := "Attached files: " + strings.Join(docHints, ", ")
			if llmText != "" {
				llmText += "\n" + hint
			} else {
				llmText = hint
			}
		}
	}

	storeIDStr := storeID.Hex()
	rfqIDStr := rfq.ID.Hex()

	progress := func(stage string, step, total int, msg string, extra map[string]interface{}) {
		pct := 0
		if total > 0 {
			pct = step * 100 / total
		}
		data := map[string]interface{}{
			"rfq_id":  rfqIDStr,
			"stage":   stage,
			"step":    step,
			"total":   total,
			"percent": pct,
			"message": msg,
		}
		for k, v := range extra {
			data[k] = v
		}
		BroadcastRFQData(storeIDStr, "rfq_progress", data)
		log.Printf("rfq_bot[%s]: %s", rfqIDStr, msg)
	}

	// Pre-check: is this actually an RFQ? Greetings and casual messages are ignored.
	// Fail-open: if the message carried images but all decryption/downloads failed and there
	// is no caption text, we have nothing to classify — assume RFQ rather than drop silently.
	mediaFailOpen := len(rfq.MediaURLs) > 0 && len(imageBase64s) == 0 && strings.TrimSpace(llmText) == ""
	if mediaFailOpen {
		log.Printf("rfq_bot[%s]: %d media URL(s) but all image downloads failed — treating as RFQ (fail-open)", rfqIDStr, len(rfq.MediaURLs))
	}
	progress("classifying", 0, 100, "Analysing message with AI...", nil)
	isManual := rfq.Source == "manual"
	if !isManual && !mediaFailOpen && !isRFQMessage(store, llmText, imageBase64s) {
		log.Printf("rfq_bot: message from %s classified as non-RFQ — ignoring", rfq.FromPhone)
		rfq.Status = "ignored"
		rfq.ErrorMsg = "Message does not appear to be an RFQ (e.g. greeting or casual text)"
		models.UpdateRFQReceived(rfq)
		BroadcastRFQData(storeIDStr, "rfq_progress", map[string]interface{}{
			"rfq_id": rfqIDStr, "stage": "ignored", "percent": 100,
			"message": "Message is not an RFQ (greeting or casual text) — ignored",
		})
		return
	}

	// Check if this is a reminder/follow-up for a recent RFQ from the same sender.
	if !isManual && isReminderWhatsApp(store, rfq.FromPhone, llmText) {
		log.Printf("rfq_bot: message from %s detected as reminder/follow-up — not creating new RFQ", rfq.FromPhone)
		rfq.Status = "ignored"
		rfq.ErrorMsg = "Message appears to be a reminder or follow-up for an existing RFQ — not creating a duplicate"
		models.UpdateRFQReceived(rfq)
		BroadcastRFQData(storeIDStr, "rfq_progress", map[string]interface{}{
			"rfq_id": rfqIDStr, "stage": "ignored", "percent": 100,
			"message": "Reminder/follow-up detected — existing RFQ not duplicated",
		})
		return
	}

	// Auto-link or create customer record using whatever info we have.
	if rfq.CustomerID == nil {
		custEmail := rfq.CustomerEmail
		if custEmail == "" {
			custEmail = rfq.FromPhone // email is not available for WhatsApp — FromPhone is used as fallback key
		}
		if customer, cerr := store.FindOrCreateCustomerFromRFQ(
			rfq.CustomerName, custEmail, rfq.FromPhone, rfq.CustomerVATNo, rfq.CustomerCompany,
			rfq.CustomerContactPerson, rfq.CustomerNationalAddress,
		); cerr != nil {
			log.Printf("rfq_bot[%s]: FindOrCreateCustomerFromRFQ error: %v", rfqIDStr, cerr)
		} else if customer != nil {
			rfq.CustomerID = &customer.ID
			if rfq.CustomerName == "" {
				rfq.CustomerName = customer.Name
			}
			models.UpdateRFQReceived(rfq)
		}
	}

	// Identify product categories via LLM
	progress("classifying", 10, 100, "Identifying product categories...", nil)
	categories, err := identifyCategories(store, llmText, imageBase64s)
	if err != nil {
		log.Printf("rfq_bot: LLM error: %v", err)
		rfq.Status = "failed"
		rfq.ErrorMsg = "LLM error: " + err.Error()
		models.UpdateRFQReceived(rfq)
		BroadcastRFQData(storeIDStr, "rfq_progress", map[string]interface{}{
			"rfq_id": rfqIDStr, "stage": "failed", "percent": 100,
			"message": "LLM error: " + err.Error(),
		})
		return
	}
	rfq.Categories = categories
	progress("classifying", 20, 100, fmt.Sprintf("Categories identified: %s", strings.Join(categories, ", ")),
		map[string]interface{}{"categories": categories})
	{
		llmModelName := store.Settings.RFQLLMModel
		if llmModelName == "" {
			llmModelName = store.Settings.RFQLLMProvider
		}
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step: "categories_identified",
			Message: fmt.Sprintf("Identified %d product categor(y/ies): %s — using LLM model: %s",
				len(categories), strings.Join(categories, ", "), llmModelName),
			Icon: "bi-tags", Color: "info",
			Details: map[string]interface{}{"categories": categories, "model": llmModelName},
		})
	}

	// Find suppliers (DB first, then Google Maps)
	progress("finding_suppliers", 25, 100, "Searching for matching suppliers...", nil)
	suppliers, err := findSuppliers(store, storeID, categories)
	if err != nil {
		log.Printf("rfq_bot: supplier search error: %v", err)
	}

	// Fallback: if specific categories yield nothing, try broader terms derived from them.
	// e.g. "Rubber Couplings" → "Industrial Equipment Supplier"; "STROMAG VECTOR 32" → "Mechanical Parts"
	if len(suppliers) == 0 && store.Settings.GoogleMapsAPIKey != "" {
		broaderCategories := broadenCategories(store, categories, llmText)
		if len(broaderCategories) > 0 {
			progress("finding_suppliers", 27, 100,
				fmt.Sprintf("No results for %s — retrying with broader terms: %s",
					strings.Join(categories, ", "), strings.Join(broaderCategories, ", ")), nil)
			log.Printf("rfq_bot[%s]: broadening search from [%s] to [%s]", rfqIDStr,
				strings.Join(categories, ", "), strings.Join(broaderCategories, ", "))
			suppliers, _ = findSuppliers(store, storeID, broaderCategories)
		}
	}

	if len(suppliers) == 0 {
		rfq.Status = "failed"
		rfq.ErrorMsg = "No suppliers found for categories: " + strings.Join(categories, ", ")
		models.UpdateRFQReceived(rfq)
		BroadcastRFQData(storeIDStr, "rfq_progress", map[string]interface{}{
			"rfq_id": rfqIDStr, "stage": "failed", "percent": 100,
			"message": "No suppliers found for: " + strings.Join(categories, ", "),
		})
		// Notify the buyer via WhatsApp so they're not left waiting
		go notifyBuyerNoSuppliers(store, rfq, categories)
		return
	}
	numMarkets := len(store.Settings.PurchaseMarkets)
	if numMarkets == 0 {
		numMarkets = 1
	}
	progress("finding_suppliers", 30, 100,
		fmt.Sprintf("Found %d supplier(s) across %d market(s) × %d categor(y/ies)", len(suppliers), numMarkets, len(categories)),
		map[string]interface{}{"supplier_count": len(suppliers)})
	// Log per-market supplier matches
	marketSuppliers := map[string][]string{}
	for _, s := range suppliers {
		mkt := s.PurchaseMarket
		if mkt == "" {
			mkt = "General"
		}
		marketSuppliers[mkt] = append(marketSuppliers[mkt], s.Name)
	}
	for mkt, names := range marketSuppliers {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step: "suppliers_matched",
			Message: fmt.Sprintf("Matched %d categor(y/ies) with %d supplier(s) in %s purchase market",
				len(rfq.Categories), len(names), mkt),
			Icon: "bi-shop", Color: "primary",
			Details: map[string]interface{}{"market": mkt, "supplier_count": len(names), "supplier_names": names, "categories": rfq.Categories},
		})
	}
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "waiting_approval",
		Message: fmt.Sprintf("Ready to send RFQ to %d supplier(s) — waiting for system to proceed", len(suppliers)),
		Icon:    "bi-hourglass-split", Color: "warning",
		Details: map[string]interface{}{"total_suppliers": len(suppliers)},
	})

	// Forward RFQ to each supplier with delay
	now := time.Now()
	processedAt := now
	rfq.ProcessedAt = &processedAt
	rfq.Status = "forwarded"

	// Count suppliers with phones for accurate step tracking
	total := 0
	for i := range suppliers {
		if suppliers[i].Phone != "" {
			total++
		}
	}
	step := 0

	for i, sup := range suppliers {
		if sup.Phone == "" {
			log.Printf("rfq_bot[%s]: supplier %q has no phone — skipping", rfqIDStr, sup.Name)
			continue
		}
		step++
		pct := 30 + (step * 70 / total)
		market := sup.PurchaseMarket
		if market == "" {
			market = "any"
		}
		progress("forwarding", pct, 100,
			fmt.Sprintf("Sending to %s (%s) — %d/%d", sup.Name, market, step, total),
			map[string]interface{}{"supplier_name": sup.Name, "market": market, "step": step, "total": total})

		sent, errMsg, sentMsg := forwardRFQToSupplier(store, rfq, &sup, i)
		sentAt := time.Now()
		record := models.RFQForwardRecord{
			SupplierID:     sup.ID,
			SupplierName:   sup.Name,
			Phone:          sup.Phone,
			SentFromPhone:  store.Settings.BotWhatsAppPhone,
			PurchaseMarket: sup.PurchaseMarket,
			Category:       sup.MatchedCategory,
			GoogleMapsURL:  sup.GoogleMapsURL,
			SentMessage:    sentMsg,
			SentAt:         &sentAt,
		}
		if sent {
			record.Status = "sent"
			log.Printf("rfq_bot[%s]: ✓ forwarded to %s (%s) [%d/%d]", rfqIDStr, sup.Name, sup.Phone, step, total)
			models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
				Step: "rfq_sent_to_supplier",
				Message: fmt.Sprintf("RFQ sent to supplier: %s (+%s) in %s market [%d/%d]",
					sup.Name, sup.Phone, market, step, total),
				Icon: "bi-send", Color: "success",
				Details: map[string]interface{}{"supplier_name": sup.Name, "supplier_phone": sup.Phone, "market": market, "step": step, "total": total},
			})
		} else {
			record.Status = "failed"
			record.ErrorMsg = errMsg
			rfq.Status = "forwarded" // partial forward is still "forwarded"
			log.Printf("rfq_bot[%s]: ✗ failed to forward to %s (%s): %s [%d/%d]", rfqIDStr, sup.Name, sup.Phone, errMsg, step, total)
			models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
				Step: "rfq_send_failed",
				Message: fmt.Sprintf("Failed to send RFQ to supplier: %s (+%s) — %s", sup.Name, sup.Phone, errMsg),
				Icon: "bi-exclamation-triangle", Color: "danger",
				Details: map[string]interface{}{"supplier_name": sup.Name, "supplier_phone": sup.Phone, "error": errMsg},
			})
		}
		rfq.ForwardedTo = append(rfq.ForwardedTo, record)
		// Persist immediately so supplier replies arriving during the delay can be routed
		models.UpdateRFQReceived(rfq)

		// Broadcast per-supplier result
		BroadcastRFQData(storeIDStr, "rfq_progress", map[string]interface{}{
			"rfq_id":        rfqIDStr,
			"stage":         "forwarding",
			"step":          step,
			"total":         total,
			"percent":       pct,
			"supplier_name": sup.Name,
			"market":        market,
			"status":        record.Status,
			"message":       fmt.Sprintf("%s to %s (%s) — %d/%d", record.Status, sup.Name, market, step, total),
		})
		BroadcastRFQEvent(storeIDStr, "rfq_updated")

		// 60-90 second human-like delay between messages (skip after last)
		if step < total {
			delay := 60 + rand.Intn(31) // 60..90 seconds
			progress("waiting", pct, 100,
				fmt.Sprintf("Waiting %ds before next message (%d/%d done)...", delay, step, total),
				map[string]interface{}{"step": step, "total": total})
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}

	models.UpdateRFQReceived(rfq)
	BroadcastRFQEvent(storeIDStr, "rfq_updated")
	BroadcastRFQData(storeIDStr, "rfq_progress", map[string]interface{}{
		"rfq_id":  rfqIDStr,
		"stage":   "done",
		"step":    total,
		"total":   total,
		"percent": 100,
		"message": fmt.Sprintf("Done — forwarded to %d supplier(s)", total),
	})
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "waiting_replies",
		Message: fmt.Sprintf("RFQ forwarded to %d supplier(s) — waiting for supplier replies", total),
		Icon:    "bi-hourglass", Color: "secondary",
		Details: map[string]interface{}{"total_suppliers": total},
	})

	// Reply to the original sender with a per-market summary via the bot instance.
	replyRFQSummaryToSender(store, rfq)
}

// notifyBuyerNoSuppliers sends the buyer a WhatsApp message when no matching suppliers could be found.
func notifyBuyerNoSuppliers(store *models.Store, rfq *models.RFQReceived, categories []string) {
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		return
	}

	catStr := strings.Join(categories, ", ")
	companyName := store.Name
	if companyName == "" {
		companyName = "us"
	}
	msg := fmt.Sprintf(
		"Thank you for your RFQ. Unfortunately, we could not find any verified suppliers for *%s* at this time.\n\nWe will continue searching and get back to you as soon as we identify suitable suppliers. You may also contact %s directly for assistance.",
		catStr, companyName,
	)

	if _, err := metaSendText(phoneNumberID, accessToken, rfq.FromPhone, msg); err != nil {
		log.Printf("rfq_bot: failed to notify buyer %s of no-supplier result: %v", rfq.FromPhone, err)
	}
}

// replyRFQSummaryToSender sends the forwarding summary back to the buyer via Meta Cloud API.
func replyRFQSummaryToSender(store *models.Store, rfq *models.RFQReceived) {
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		return
	}

	// Group successfully sent records by purchase market.
	type supplierSummary struct {
		name          string
		phone         string
		googleMapsURL string
	}
	type marketEntry struct {
		suppliers []supplierSummary
	}
	marketMap := map[string]*marketEntry{}
	marketOrder := []string{}
	for _, r := range rfq.ForwardedTo {
		if r.Status != "sent" {
			continue
		}
		market := r.PurchaseMarket
		if market == "" {
			market = "general"
		}
		if _, exists := marketMap[market]; !exists {
			marketMap[market] = &marketEntry{}
			marketOrder = append(marketOrder, market)
		}
		marketMap[market].suppliers = append(marketMap[market].suppliers, supplierSummary{
			name:          r.SupplierName,
			phone:         r.Phone,
			googleMapsURL: r.GoogleMapsURL,
		})
	}

	if len(marketOrder) == 0 {
		return
	}

	for _, market := range marketOrder {
		entry := marketMap[market]
		count := len(entry.suppliers)
		noun := "Suppliers"
		if count == 1 {
			noun = "Supplier"
		}

		var listLines string
		for i, sup := range entry.suppliers {
			line := fmt.Sprintf("\n  %d. %s", i+1, sup.name)
			if sup.phone != "" {
				line += fmt.Sprintf(" — +%s", sup.phone)
			}
			if sup.googleMapsURL != "" {
				line += fmt.Sprintf("\n     📍 %s", sup.googleMapsURL)
			}
			listLines += line
		}

		var msg string
		if market == "general" {
			msg = fmt.Sprintf("✅ Your RFQ has been forwarded to %d %s:%s\n\nYou should receive quotes shortly.", count, noun, listLines)
		} else {
			msg = fmt.Sprintf("✅ Forwarded your RFQ to %d %s in the *%s* market:%s\n\nQuotes coming soon!", count, noun, market, listLines)
		}

		if _, err := metaSendText(phoneNumberID, accessToken, rfq.FromPhone, msg); err != nil {
			log.Printf("rfq_bot: failed to send summary reply to %s: %v", rfq.FromPhone, err)
		}
	}
}

// isRFQMessage asks the LLM whether the incoming message is a genuine Request for Quotation.
// Returns true if it looks like an RFQ; false for greetings, casual messages, etc.
// Falls back to true when the LLM is not configured or unavailable (so messages still process).
// callLLMTextWithImages sends a prompt + optional images to the configured LLM and returns the raw text reply.
// Used for yes/no classification where the full vision payload is needed.
func callLLMTextWithImages(apiKey, model, provider, prompt string, imageBase64s []string) (string, error) {
	switch provider {
	case "openai":
		if model == "" {
			model = "gpt-4o-mini"
		}
		type part struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url,omitempty"`
		}
		var parts []part
		parts = append(parts, part{Type: "text", Text: prompt})
		for _, dataURI := range imageBase64s {
			parts = append(parts, part{Type: "image_url", ImageURL: &struct{ URL string `json:"url"` }{URL: dataURI}})
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"model":      model,
			"messages":   []map[string]interface{}{{"role": "user", "content": parts}},
			"max_tokens": 10,
		})
		req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var r struct {
			Choices []struct {
				Message struct{ Content string `json:"content"` } `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &r); err != nil || len(r.Choices) == 0 {
			return "", fmt.Errorf("openai parse error: %s", string(body))
		}
		return r.Choices[0].Message.Content, nil

	case "anthropic":
		if model == "" {
			model = "claude-haiku-4-5-20251001"
		}
		type block struct {
			Type   string `json:"type"`
			Text   string `json:"text,omitempty"`
			Source *struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			} `json:"source,omitempty"`
		}
		var blocks []block
		for _, dataURI := range imageBase64s {
			mime, raw := splitDataURI(dataURI)
			blocks = append(blocks, block{Type: "image", Source: &struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			}{Type: "base64", MediaType: mime, Data: raw}})
		}
		blocks = append(blocks, block{Type: "text", Text: prompt})
		payload, _ := json.Marshal(map[string]interface{}{
			"model": model, "max_tokens": 10,
			"messages": []map[string]interface{}{{"role": "user", "content": blocks}},
		})
		req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var r struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &r); err != nil || len(r.Content) == 0 {
			return "", fmt.Errorf("anthropic parse error: %s", string(body))
		}
		return r.Content[0].Text, nil

	case "gemini":
		if model == "" {
			model = "gemini-3.6-flash"
		}
		type inlinePart struct {
			Text       string `json:"text,omitempty"`
			InlineData *struct {
				MimeType string `json:"mime_type"`
				Data     string `json:"data"`
			} `json:"inlineData,omitempty"`
		}
		var parts []inlinePart
		parts = append(parts, inlinePart{Text: prompt})
		for _, dataURI := range imageBase64s {
			mime, raw := splitDataURI(dataURI)
			parts = append(parts, inlinePart{InlineData: &struct {
				MimeType string `json:"mime_type"`
				Data     string `json:"data"`
			}{MimeType: mime, Data: raw}})
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{{"parts": parts}},
		})
		apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var r struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("gemini parse error: %s", string(body))
		}
		return r.Candidates[0].Content.Parts[0].Text, nil
	}
	// OpenAI-compatible providers (cloudflare, groq, xai, mistral, etc.) — text only, no vision
	if len(imageBase64s) > 0 {
		return "", fmt.Errorf("provider %q does not support image inputs in callLLMTextWithImages", provider)
	}
	baseURL := openAICompatBaseURL(provider)
	return callOpenAICompatExtractRFQ(apiKey, model, prompt, nil, 50, baseURL)
}

func isRFQMessage(store *models.Store, text string, imageBase64s []string) bool {
	if strings.TrimSpace(text) == "" && len(imageBase64s) == 0 {
		return false
	}

	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel
	if apiKey == "" || provider == "" {
		return true // no LLM configured — allow all messages through
	}

	prompt := `You are a classifier for a trading/distribution company. Decide whether the message below is a genuine RFQ (Request for Quotation) — meaning the SENDER is a BUYER or CUSTOMER who wants to PURCHASE products FROM our company.

Reply with ONLY the single word "yes" or "no".

Answer "yes" ONLY if:
- The sender is clearly a customer/buyer asking OUR company to provide a price, quote, or availability of products THEY want to buy
- There are explicit purchase signals: "please quote", "RFQ", "requesting quotation", "need price for", "can you supply", "please send quotation", "we need X units", "kindly quote", "request for quotation"
- The sender wants to ORDER or PROCURE something from us

Answer "no" if:
- The sender is a SUPPLIER, MANUFACTURER, FACTORY, or EXPORTER introducing themselves and offering to SELL products TO us
- The email is a SALES PITCH, cold outreach, or supplier introduction (phrases like "we are a manufacturer", "we supply", "we offer", "factory-direct", "our products", "we would like to be your supplier")
- A company promoting their own products/catalogue to us as a potential distributor
- General greetings, networking, informational, or marketing messages without an explicit purchase request
- The sender mentions producing, supplying, manufacturing, or exporting — they are the SELLER, not the BUYER`

	if strings.TrimSpace(text) != "" {
		prompt += "\n\nMessage:\n" + text
	}

	answer, err := callLLMTextWithImages(apiKey, model, provider, prompt, imageBase64s)
	if err != nil {
		log.Printf("rfq_bot: isRFQMessage LLM error: %v — allowing message through", err)
		return true
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "yes")
}

// isReminderWhatsApp asks the LLM whether a new WhatsApp message is a reminder or
// follow-up for a recent existing RFQ from the same phone number. Returns true when
// the message should NOT create a new RFQ.
func isReminderWhatsApp(store *models.Store, fromPhone, bodyText string) bool {
	recent, err := models.FindRecentRFQsByPhone(store.ID, fromPhone, 30*24*time.Hour, 5)
	if err != nil || len(recent) == 0 {
		return false
	}

	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel
	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	if apiKey == "" || provider == "" {
		return false
	}

	var sb strings.Builder
	sb.WriteString("You are a procurement message classifier.\n\n")
	sb.WriteString("The customer sent this new WhatsApp message:\n")
	sb.WriteString(bodyText + "\n\n---\n")
	sb.WriteString("The same customer already has these recent RFQs on record:\n")
	for i, r := range recent {
		sb.WriteString(fmt.Sprintf("%d. Code: %s | Date: %s | Items: %s\n",
			i+1, r.Code, r.ReceivedAt.Format("2006-01-02"), r.TextContent))
	}
	sb.WriteString("\n---\n")
	sb.WriteString("Is the new message a REMINDER or FOLLOW-UP for one of the existing RFQs above? " +
		"Reply with ONLY \"yes\" (it is a reminder) or \"no\" (it is a new, distinct request).")

	answer, err := callLLMText(apiKey, model, sb.String(), provider)
	if err != nil {
		log.Printf("rfq_bot: isReminderWhatsApp LLM error: %v — treating as new RFQ", err)
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "yes")
}

// classifyIncomingMessage uses the configured LLM to classify a WhatsApp message as
// "rfq", "quotation", or "other", and extracts any RFQ reference code if it's a quotation.
// Falls back to "rfq" (allow-all) when the LLM is not configured.
func classifyIncomingMessage(store *models.Store, text string, imageBase64s []string) (msgType string, rfqCode string) {
	// Use dedicated classification LLM when configured; fall back to the RFQ LLM.
	provider := strings.ToLower(store.Settings.ClassifyLLMProvider)
	model := store.Settings.ClassifyLLMModel
	if provider == "" {
		provider = strings.ToLower(store.Settings.RFQLLMProvider)
	}
	if model == "" {
		model = store.Settings.RFQLLMModel
	}
	// Resolve API key: try per-provider extraction key first, then legacy RFQLLMAPIKey.
	apiKey := resolveExtractionAPIKey(provider, &store.Settings)
	if apiKey == "" {
		apiKey = store.Settings.RFQLLMAPIKey
	}
	if apiKey == "" || provider == "" {
		// No LLM — fall back to allowing everything through as RFQ
		return "rfq", ""
	}

	prompt := `Classify this incoming message as one of:
- "rfq": The sender wants to purchase items and is requesting prices/quotations/offers
- "quotation": A supplier is providing price information/unit prices for products
- "other": Company introduction, marketing/promotional content, newsletter, vendor cold-outreach, greetings, casual chat, thanks, or any unrelated content

Also, if it's a quotation, extract any RFQ reference code (like "RFQ-0001" or "RFQ-001") mentioned.

Reply with ONLY valid JSON: {"type": "rfq"|"quotation"|"other", "rfq_code": "..." or null}`

	if strings.TrimSpace(text) != "" {
		prompt += "\n\nMessage:\n" + text
	}

	answer, err := callLLMTextWithImages(apiKey, model, provider, prompt, imageBase64s)
	if err != nil {
		log.Printf("rfq_bot: classifyIncomingMessage LLM error: %v — defaulting to rfq", err)
		return "rfq", ""
	}

	// Strip markdown code fences if present
	cleaned := strings.TrimSpace(answer)
	if idx := strings.Index(cleaned, "```"); idx != -1 {
		cleaned = cleaned[idx:]
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimPrefix(cleaned, "```")
		if end := strings.LastIndex(cleaned, "```"); end > 0 {
			cleaned = cleaned[:end]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	var result struct {
		Type    string  `json:"type"`
		RFQCode *string `json:"rfq_code"`
	}
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		log.Printf("rfq_bot: classifyIncomingMessage JSON parse error: %v (raw: %s) — defaulting to rfq", err, answer)
		return "rfq", ""
	}

	t := strings.ToLower(strings.TrimSpace(result.Type))
	if t != "rfq" && t != "quotation" && t != "other" {
		t = "rfq"
	}
	code := ""
	if result.RFQCode != nil {
		code = strings.TrimSpace(*result.RFQCode)
	}
	return t, code
}

// identifyCategories calls the configured LLM to extract product categories from an RFQ.
// broadenCategories asks the LLM to suggest broader/related trade categories when the
// specific ones yielded no suppliers. Returns empty slice if LLM is not configured.
func broadenCategories(store *models.Store, specific []string, rfqText string) []string {
	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel
	if apiKey == "" {
		return nil
	}

	prompt := fmt.Sprintf(`You are a procurement expert. The following specific product categories produced no supplier results:
%s

RFQ context: %s

Suggest 1-3 BROADER trade/industry category names that a general industrial supplier or trading company would use to describe what they sell. These will be used as Google Maps search terms, so keep them short and common (e.g. "Industrial Equipment", "Power Transmission", "Mechanical Parts", "Engineering Supplies").
Return ONLY a valid JSON array of strings. No explanation.`, strings.Join(specific, ", "), rfqText)

	var result string
	var err error
	switch provider {
	case "openai":
		result, err = callLLMText(apiKey, model, prompt, "openai")
	case "anthropic":
		result, err = callLLMText(apiKey, model, prompt, "anthropic")
	case "gemini":
		result, err = callLLMText(apiKey, model, prompt, "gemini")
	default:
		// OpenAI-compatible providers (cloudflare, groq, xai, etc.)
		apiKey2, baseURL := resolveExtractionEndpoint(provider, &store.Settings)
		if apiKey2 != "" {
			apiKey = apiKey2
		}
		if baseURL == "" {
			baseURL = openAICompatBaseURL(provider)
		}
		result, err = callOpenAICompatExtractRFQ(apiKey, model, prompt, nil, 200, baseURL)
	}
	if err != nil || strings.TrimSpace(result) == "" {
		return nil
	}

	result = strings.TrimSpace(result)
	start := strings.Index(result, "[")
	end := strings.LastIndex(result, "]")
	if start == -1 || end <= start {
		return nil
	}
	var broader []string
	if json.Unmarshal([]byte(result[start:end+1]), &broader) != nil {
		return nil
	}
	return broader
}

func identifyCategories(store *models.Store, text string, imageBase64s []string) ([]string, error) {
	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel

	if apiKey == "" {
		return nil, fmt.Errorf("LLM API key not configured")
	}

	prompt := `You are a procurement expert. Analyze the following RFQ (Request for Quotation) and identify the main product categories needed.
Return ONLY a valid JSON array of short category strings (e.g. ["Steel Pipes", "Valves", "Fittings"]).
No explanation — just the JSON array.
If the RFQ contains spreadsheet rows or a list of items, extract all distinct product categories from those items.
If only a filename is provided, infer categories from the filename (e.g. "Consumables for Sadara.xlsx" → ["Consumables"]).
Always return at least one category — never return an empty array.`
	if text != "" {
		prompt += "\n\nRFQ Content:\n" + text
	}

	switch provider {
	case "openai":
		return callOpenAIForCategories(apiKey, model, prompt, imageBase64s)
	case "anthropic":
		return callAnthropicForCategories(apiKey, model, prompt, imageBase64s)
	case "gemini":
		return callGeminiForCategories(apiKey, model, prompt, imageBase64s)
	default:
		// OpenAI-compatible providers (cloudflare, groq, xai, etc.)
		apiKey2, baseURL := resolveExtractionEndpoint(provider, &store.Settings)
		if apiKey2 != "" {
			apiKey = apiKey2
		}
		if baseURL == "" {
			baseURL = openAICompatBaseURL(provider)
		}
		raw, err := callOpenAICompatExtractRFQ(apiKey, model, prompt, nil, 300, baseURL)
		if err != nil {
			return nil, err
		}
		return parseCategories(raw), nil
	}
}

func callOpenAIForCategories(apiKey, model, prompt string, imageBase64s []string) ([]string, error) {
	if model == "" {
		model = "gpt-4o-mini"
	}
	type contentPart struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	var parts []contentPart
	parts = append(parts, contentPart{Type: "text", Text: prompt})
	for _, dataURI := range imageBase64s {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &struct{ URL string `json:"url"` }{URL: dataURI}})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"messages":   []map[string]interface{}{{"role": "user", "content": parts}},
		"max_tokens": 300,
	})

	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI parse error: %s", string(body))
	}
	return parseCategories(result.Choices[0].Message.Content), nil
}

func callAnthropicForCategories(apiKey, model, prompt string, imageBase64s []string) ([]string, error) {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	type contentBlock struct {
		Type   string `json:"type"`
		Text   string `json:"text,omitempty"`
		Source *struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source,omitempty"`
	}
	var blocks []contentBlock
	for _, dataURI := range imageBase64s {
		mime, raw := splitDataURI(dataURI)
		blocks = append(blocks, contentBlock{
			Type: "image",
			Source: &struct {
				Type      string `json:"type"`
				MediaType string `json:"media_type"`
				Data      string `json:"data"`
			}{Type: "base64", MediaType: mime, Data: raw},
		})
	}
	blocks = append(blocks, contentBlock{Type: "text", Text: prompt})

	payload, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"max_tokens": 300,
		"messages":   []map[string]interface{}{{"role": "user", "content": blocks}},
	})

	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Content) == 0 {
		return nil, fmt.Errorf("Anthropic parse error: %s", string(body))
	}
	return parseCategories(result.Content[0].Text), nil
}

func callGeminiForCategories(apiKey, model, prompt string, imageBase64s []string) ([]string, error) {
	if model == "" {
		model = "gemini-3.6-flash"
	}
	type inlinePart struct {
		Text       string `json:"text,omitempty"`
		InlineData *struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"inlineData,omitempty"`
	}
	var parts []inlinePart
	parts = append(parts, inlinePart{Text: prompt})
	for _, dataURI := range imageBase64s {
		mime, raw := splitDataURI(dataURI)
		parts = append(parts, inlinePart{InlineData: &struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		}{MimeType: mime, Data: raw}})
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": parts}},
	})

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Candidates) == 0 {
		return nil, fmt.Errorf("Gemini parse error: %s", string(body))
	}
	if len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("Gemini: empty response")
	}
	return parseCategories(result.Candidates[0].Content.Parts[0].Text), nil
}

// parseCategories extracts a string slice from a JSON array string returned by the LLM.
func parseCategories(raw string) []string {
	// Strip markdown code fences if present
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Find the JSON array
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end <= start {
		// Fallback: split by comma
		parts := strings.Split(raw, ",")
		var cats []string
		for _, p := range parts {
			p = strings.Trim(p, `" \t\n\r`)
			if p != "" {
				cats = append(cats, p)
			}
		}
		return cats
	}
	var cats []string
	json.Unmarshal([]byte(raw[start:end+1]), &cats)
	return cats
}

// ── Supplier email extraction from website ────────────────────────────────────

var websiteEmailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// skipEmailDomain returns true for common non-contact email domains that appear
// in website HTML but are not the supplier's actual contact email.
var skipEmailDomain = map[string]bool{
	"example.com": true, "sentry.io": true, "w3.org": true, "schema.org": true,
	"google.com": true, "googleapis.com": true, "facebook.com": true, "twitter.com": true,
	"cloudflare.com": true, "jquery.com": true, "gravatar.com": true,
}

// contactLinkRe matches href attributes that look like contact pages.
var contactLinkRe = regexp.MustCompile(`(?i)href=["']([^"'#?]+contact[^"'#?]*)["']`)

// crawlWebsiteForEmail fetches the supplier's website and extracts the first
// real contact email address found in the HTML.
func crawlWebsiteForEmail(website string) string {
	if website == "" {
		return ""
	}
	if !strings.HasPrefix(website, "http://") && !strings.HasPrefix(website, "https://") {
		website = "https://" + website
	}
	baseURL := strings.TrimRight(website, "/")
	ua := "Mozilla/5.0 (compatible; StartPOS/1.0; +https://startpos.ai)"
	client := &http.Client{Timeout: 12 * time.Second}
	fetchPage := func(u string) string {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return ""
		}
		req.Header.Set("User-Agent", ua)
		resp, err := client.Do(req)
		if err != nil {
			return ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		return string(b)
	}

	homeHTML := fetchPage(website)
	for _, email := range extractEmailsFromHTML(homeHTML) {
		if email != "" {
			return email
		}
	}

	// Build candidate contact page URLs:
	// 1. Links found on the home page that contain "contact" in the path
	// 2. Common fallback paths
	seen := map[string]bool{}
	var candidates []string
	for _, m := range contactLinkRe.FindAllStringSubmatch(homeHTML, -1) {
		href := strings.TrimSpace(m[1])
		if href == "" || strings.HasPrefix(href, "javascript") {
			continue
		}
		var full string
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			full = href
		} else if strings.HasPrefix(href, "/") {
			// Extract origin from baseURL
			parts := strings.SplitN(baseURL, "/", 4)
			if len(parts) >= 3 {
				full = parts[0] + "//" + parts[2] + href
			}
		}
		if full != "" && !seen[full] {
			seen[full] = true
			candidates = append(candidates, full)
		}
		if len(candidates) >= 3 {
			break
		}
	}
	// Fallback common paths
	for _, suffix := range []string{"/contact", "/contact-us", "/en/contact", "/about/contact"} {
		u := baseURL + suffix
		if !seen[u] {
			seen[u] = true
			candidates = append(candidates, u)
		}
	}

	for _, u := range candidates {
		html := fetchPage(u)
		for _, email := range extractEmailsFromHTML(html) {
			if email != "" {
				return email
			}
		}
	}
	return ""
}

func extractEmailsFromHTML(html string) []string {
	matches := websiteEmailRe.FindAllString(html, -1)
	seen := map[string]bool{}
	var result []string
	for _, m := range matches {
		m = strings.ToLower(m)
		if seen[m] {
			continue
		}
		seen[m] = true
		parts := strings.SplitN(m, "@", 2)
		if len(parts) != 2 {
			continue
		}
		domain := parts[1]
		if skipEmailDomain[domain] {
			continue
		}
		// Skip image/file extensions that got caught (e.g. icon.png@2x)
		if strings.ContainsAny(parts[0], "/.") && !strings.Contains(parts[0], "+") {
			continue
		}
		result = append(result, m)
	}
	return result
}

// crawlAndSaveSupplierEmail crawls the supplier's website for a contact email
// and saves it to the DB if found. Intended to run as a goroutine.
func crawlAndSaveSupplierEmail(supplier models.RFQSupplier) {
	if supplier.Website == "" || supplier.Email != "" {
		return
	}
	email := crawlWebsiteForEmail(supplier.Website)
	if email == "" {
		log.Printf("crawlEmail[%s]: no email found on %s", supplier.Name, supplier.Website)
		return
	}
	log.Printf("crawlEmail[%s]: found email %s on %s", supplier.Name, email, supplier.Website)
	if err := models.SetRFQSupplierEmail(supplier.ID, email); err != nil {
		log.Printf("crawlEmail[%s]: save error: %v", supplier.Name, err)
	}
}

// POST /v1/rfq-suppliers/backfill-emails?store_id=...
// Finds all suppliers with a website but no email and crawls each in the background.
func BackfillSupplierEmailsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	suppliers, err := models.ListSuppliersWithWebsiteNoEmail(storeObjID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	count := len(suppliers)
	for _, s := range suppliers {
		go crawlAndSaveSupplierEmail(s)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"queued": count}) //nolint:errcheck
}

// ── Supplier category inference from website ──────────────────────────────────

var (
	htmlScriptStyleRe = regexp.MustCompile(`(?is)<(script|style|head)[^>]*>.*?</(script|style|head)>`)
	htmlTagStripRe    = regexp.MustCompile(`<[^>]*>`)
	htmlEntityRe      = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
)

// stripHTMLForText removes script/style/head blocks and all tags from HTML, returning plain text.
func stripHTMLForText(htmlStr string) string {
	s := htmlScriptStyleRe.ReplaceAllString(htmlStr, " ")
	s = htmlTagStripRe.ReplaceAllString(s, " ")
	s = htmlEntityRe.ReplaceAllString(s, " ")
	return s
}

// inferSupplierCategoriesFromWebsite fetches the supplier's website, extracts visible text,
// sends it to the store's configured LLM, and saves the inferred categories back to the DB.
// Intended to be called as a goroutine when a supplier has no categories but has a website.
func inferSupplierCategoriesFromWebsite(store *models.Store, supplier models.RFQSupplier) {
	if supplier.Website == "" || store.Settings.RFQLLMAPIKey == "" {
		return
	}
	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel
	provider := strings.ToLower(store.Settings.RFQLLMProvider)

	// Fetch website — follow up to one redirect, 10 s timeout.
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", supplier.Website, nil)
	if err != nil {
		log.Printf("inferCategories[%s]: bad URL %s: %v", supplier.Name, supplier.Website, err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; StartPOS/1.0; +https://startpos.ai)")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("inferCategories[%s]: fetch error: %v", supplier.Name, err)
		return
	}
	defer resp.Body.Close()
	rawBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

	// Strip HTML tags and collapse whitespace.
	text := strings.Join(strings.Fields(stripHTMLForText(string(rawBytes))), " ")
	if len(text) > 3000 {
		text = text[:3000]
	}
	if len(strings.TrimSpace(text)) < 50 {
		log.Printf("inferCategories[%s]: website content too short to classify", supplier.Name)
		return
	}

	prompt := fmt.Sprintf(`You are a procurement expert. Based on the following content from a supplier's website, identify 1-5 product/service categories this supplier specializes in.
These categories will be used to match suppliers to RFQs in a B2B procurement system.
Return ONLY a valid JSON array of short category strings (e.g. ["Industrial Valves", "Pipes & Fittings", "Steel Products"]).
No explanation — just the JSON array.

Supplier name: %s
Website content:
%s`, supplier.Name, text)

	var raw string
	var llmErr error
	switch provider {
	case "openai":
		raw, llmErr = callLLMText(apiKey, model, prompt, "openai")
	case "anthropic":
		raw, llmErr = callLLMText(apiKey, model, prompt, "anthropic")
	case "gemini":
		raw, llmErr = callLLMText(apiKey, model, prompt, "gemini")
	default:
		apiKey2, baseURL := resolveExtractionEndpoint(provider, &store.Settings)
		if apiKey2 != "" {
			apiKey = apiKey2
		}
		if baseURL == "" {
			baseURL = openAICompatBaseURL(provider)
		}
		raw, llmErr = callOpenAICompatExtractRFQ(apiKey, model, prompt, nil, 200, baseURL)
	}
	if llmErr != nil || raw == "" {
		log.Printf("inferCategories[%s]: LLM error: %v", supplier.Name, llmErr)
		return
	}

	categories := parseCategories(raw)
	if len(categories) == 0 {
		log.Printf("inferCategories[%s]: could not parse categories from LLM response", supplier.Name)
		return
	}
	log.Printf("inferCategories[%s]: inferred %v", supplier.Name, categories)
	if dbErr := models.SetRFQSupplierCategories(supplier.ID, supplier.StoreID, categories); dbErr != nil {
		log.Printf("inferCategories[%s]: DB update error: %v", supplier.Name, dbErr)
	}
}

// ── Generic LLM text helpers (no image support — text-only prompts) ──────────

// callOpenAI calls OpenAI and returns the raw text response.
func callOpenAI(apiKey, model, prompt, _ string) (string, error) {
	if model == "" {
		model = "gpt-4o-mini"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"messages":   []map[string]interface{}{{"role": "user", "content": prompt}},
		"max_tokens": 1000,
	})
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Choices) == 0 {
		return "", fmt.Errorf("OpenAI error: %s", string(body))
	}
	return r.Choices[0].Message.Content, nil
}

// callAnthropic calls Anthropic Claude and returns the raw text response.
func callAnthropic(apiKey, model, prompt, _ string) (string, error) {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"max_tokens": 1000,
		"messages":   []map[string]interface{}{{"role": "user", "content": prompt}},
	})
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Content) == 0 {
		return "", fmt.Errorf("Anthropic error: %s", string(body))
	}
	return r.Content[0].Text, nil
}

// callGemini calls Google Gemini and returns the raw text response.
func callGemini(apiKey, model, prompt, _ string) (string, error) {
	if model == "" {
		model = "gemini-3.6-flash"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": prompt}}}},
	})
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 40 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string `json:"text"` } `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini error: %s", string(body))
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}

// ── 6. LLM Intro Generator ───────────────────────────────────────────────────

// generateSupplierIntro uses the configured LLM to produce a short, unique opening
// paragraph for a forwarded RFQ message. The original message is appended verbatim
// by the caller. Falls back to a deterministic prefix if the LLM fails.
func generateSupplierIntro(store *models.Store, supplierName string, categories []string, rfqText string, idx int, firstContact bool) string {
	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel
	if apiKey == "" {
		return fallbackIntro(supplierName, categories, idx, store.Name, firstContact)
	}

	catStr := strings.Join(categories, ", ")
	storeName := store.Name
	if storeName == "" {
		storeName = "our company"
	}

	prompt := fmt.Sprintf(`You are a procurement assistant for %s. Write ONE short sentence (max 20 words) to introduce this RFQ to a supplier in %s. Do NOT greet or repeat the RFQ. Output only the sentence.

RFQ: %s`, storeName, catStr, rfqText)

	var intro string
	var err error
	switch provider {
	case "openai":
		intro, err = callLLMText(apiKey, model, prompt, "openai")
	case "anthropic":
		intro, err = callLLMText(apiKey, model, prompt, "anthropic")
	case "gemini":
		intro, err = callLLMText(apiKey, model, prompt, "gemini")
	}
	if err != nil || strings.TrimSpace(intro) == "" {
		return fallbackIntro(supplierName, categories, idx, store.Name, firstContact)
	}
	return strings.TrimSpace(intro)
}

func fallbackIntro(supplierName string, categories []string, idx int, storeName string, firstContact bool) string {
	catStr := strings.Join(categories, ", ")
	intros := []string{
		fmt.Sprintf("We have a procurement request matching your expertise in %s.", catStr),
		fmt.Sprintf("Please quote for the %s requirement below.", catStr),
		fmt.Sprintf("Kindly review this %s RFQ and share your best price.", catStr),
		fmt.Sprintf("We'd like your competitive quote for the %s request below.", catStr),
	}
	return intros[idx%len(intros)]
}

// callLLMText calls the specified provider's chat API and returns the text response.
func callLLMText(apiKey, model, prompt, provider string) (string, error) {
	switch provider {
	case "openai":
		if model == "" {
			model = "gpt-4o-mini"
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"model":      model,
			"messages":   []map[string]interface{}{{"role": "user", "content": prompt}},
			"max_tokens": 200,
		})
		req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var r struct {
			Choices []struct {
				Message struct{ Content string `json:"content"` } `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &r); err != nil || len(r.Choices) == 0 {
			return "", fmt.Errorf("openai text error: %s", string(body))
		}
		return r.Choices[0].Message.Content, nil

	case "anthropic":
		if model == "" {
			model = "claude-haiku-4-5-20251001"
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"model":      model,
			"max_tokens": 200,
			"messages":   []map[string]interface{}{{"role": "user", "content": prompt}},
		})
		req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var r struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &r); err != nil || len(r.Content) == 0 {
			return "", fmt.Errorf("anthropic text error: %s", string(body))
		}
		return r.Content[0].Text, nil

	case "gemini":
		if model == "" {
			model = "gemini-3.6-flash"
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{{"parts": []map[string]interface{}{{"text": prompt}}}},
		})
		apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
		req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var r struct {
			Candidates []struct {
				Content struct {
					Parts []struct{ Text string `json:"text"` } `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("gemini text error: %s", string(body))
		}
		return r.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", fmt.Errorf("unknown provider: %s", provider)
}

// ── 7. Supplier Discovery ─────────────────────────────────────────────────────

// rfqMinSuppliers returns the configured minimum, defaulting to 2.
func rfqMinSuppliers(store *models.Store) int {
	if store.Settings.RFQMinSuppliers > 0 {
		return store.Settings.RFQMinSuppliers
	}
	return 2
}

// findSuppliers collects up to `min` suppliers per (category × purchase market) pair
// from the DB, supplementing from Google Maps when DB doesn't have enough.
// No WhatsApp validation is done here — that happens only at send time.
func findSuppliers(store *models.Store, storeID primitive.ObjectID, categories []string) ([]models.RFQSupplier, error) {
	min := rfqMinSuppliers(store)

	markets := store.Settings.PurchaseMarkets
	if len(markets) == 0 {
		markets = []string{""}
	}

	searchCategories := categories
	if len(searchCategories) == 0 {
		searchCategories = []string{"supplier"}
	}

	// Deduplicate by phone across all (category × market) pairs
	globalSeen := map[string]bool{}
	var allSuppliers []models.RFQSupplier

	for _, market := range markets {
		for _, category := range searchCategories {
			// 1. DB lookup
			dbResult, _ := models.FindRFQSuppliersByMarketAndCategories(storeID, []string{category}, market, int64(min))

			var pairSuppliers []models.RFQSupplier
			for _, s := range dbResult {
				if s.Phone == "" || globalSeen[s.Phone] {
					continue
				}
				globalSeen[s.Phone] = true
				s.MatchedCategory = category
				pairSuppliers = append(pairSuppliers, s)
			}

			// 2. Supplement from Google Maps if DB doesn't have enough
			if len(pairSuppliers) < min && store.Settings.GoogleMapsAPIKey == "" {
				log.Printf("rfq_bot: Google Maps API key not configured — cannot supplement %d/%d suppliers for cat=%q market=%q", len(pairSuppliers), min, category, market)
			}
			if len(pairSuppliers) < min && store.Settings.GoogleMapsAPIKey != "" {
				needed := (min - len(pairSuppliers)) * 3
				if needed < 5 {
					needed = 5
				}
				mapsSuppliers, err := searchGoogleMapsSuppliers(store.Settings.GoogleMapsAPIKey, category, market, storeID, needed)
				if err != nil {
					log.Printf("rfq_bot: google maps error cat=%q market=%q: %v", category, market, err)
				} else {
					added := false
					for i := range mapsSuppliers {
						if len(pairSuppliers) >= min {
							break
						}
						sup := &mapsSuppliers[i]
						sup.StoreID = storeID
						sup.IsActive = true
						if sup.Phone == "" || globalSeen[sup.Phone] {
							continue
						}
						globalSeen[sup.Phone] = true
						sup.MatchedCategory = category
						// Persist the matched category so future DB lookups find this supplier
						hasCat := false
						for _, c := range sup.Categories {
							if strings.EqualFold(c, category) {
								hasCat = true
								break
							}
						}
						if !hasCat {
							sup.Categories = append(sup.Categories, category)
						}
						models.UpsertRFQSupplierByPlaceID(sup)
						pairSuppliers = append(pairSuppliers, *sup)
						added = true
					}
					if added {
						BroadcastRFQEvent(storeID.Hex(), "supplier_updated")
					}
				}
			}

			log.Printf("rfq_bot: market=%q cat=%q found %d suppliers", market, category, len(pairSuppliers))
			allSuppliers = append(allSuppliers, pairSuppliers...)
		}
	}

	return allSuppliers, nil
}

// searchGoogleMapsSuppliers uses the Places API (New) v1 which includes phone numbers
// directly in the search response — no separate detail lookup needed.
// market is appended to the query (e.g. "Steel Pipes supplier in Dammam"); empty = no location suffix.
// Results are cached in MongoDB for 30 days to avoid exhausting the daily quota.
func searchGoogleMapsSuppliers(apiKey, category, market string, storeID primitive.ObjectID, maxResults int) ([]models.RFQSupplier, error) {
	// --- cache lookup ---
	cached, err := models.GetGooglePlacesCache(category, market)
	if err != nil {
		log.Printf("rfq_bot: places cache read error (will call API): %v", err)
	}
	if cached != nil {
		log.Printf("rfq_bot: places cache HIT for category=%q market=%q (%d suppliers)", category, market, len(cached))
		if maxResults > 0 && len(cached) > maxResults {
			cached = cached[:maxResults]
		}
		return cachedSuppliersToRFQ(cached, category, market, storeID), nil
	}
	log.Printf("rfq_bot: places cache MISS for category=%q market=%q — calling Google API", category, market)

	// --- live API call ---
	const endpoint = "https://places.googleapis.com/v1/places:searchText"
	const fieldMask = "places.id,places.displayName,places.formattedAddress,places.rating,places.location,places.internationalPhoneNumber,places.websiteUri,places.primaryType,places.types"

	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > 20 {
		maxResults = 20
	}
	query := category + " supplier"
	if market != "" {
		query += " in " + market
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"textQuery":      query,
		"maxResultCount": maxResults,
	})
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", apiKey)
	req.Header.Set("X-Goog-FieldMask", fieldMask)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Places API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Places []struct {
			ID                 string   `json:"id"`
			FormattedAddress   string   `json:"formattedAddress"`
			Rating             float64  `json:"rating"`
			InternationalPhone string   `json:"internationalPhoneNumber"`
			WebsiteUri         string   `json:"websiteUri"`
			PrimaryType        string   `json:"primaryType"`
			Types              []string `json:"types"`
			DisplayName        struct {
				Text string `json:"text"`
			} `json:"displayName"`
			Location struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
		} `json:"places"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var toCache []models.CachedSupplier
	var suppliers []models.RFQSupplier
	for _, place := range result.Places {
		phone := digitsOnly(place.InternationalPhone)
		if phone == "" {
			continue
		}
		mapsURL := ""
		if place.ID != "" {
			mapsURL = "https://www.google.com/maps/place/?q=place_id:" + place.ID
		}
		// Build categories: search category + Google place types (filtered and humanised).
		placeTypes := placeTypesToCategories(place.PrimaryType, place.Types, category)
		sup := models.RFQSupplier{
			Name:           place.DisplayName.Text,
			Phone:          phone,
			Address:        place.FormattedAddress,
			Latitude:       place.Location.Latitude,
			Longitude:      place.Location.Longitude,
			Rating:         place.Rating,
			GooglePlaceID:  place.ID,
			GoogleMapsURL:  mapsURL,
			Website:        place.WebsiteUri,
			Categories:     placeTypes,
			PurchaseMarket: marketTitleCase(market),
			StoreID:        storeID,
		}
		suppliers = append(suppliers, sup)
		toCache = append(toCache, models.CachedSupplier{
			Name:    place.DisplayName.Text,
			Phone:   phone,
			Address: place.FormattedAddress,
			Lat:     place.Location.Latitude,
			Lng:     place.Location.Longitude,
			Rating:  place.Rating,
			PlaceID: place.ID,
			Website: place.WebsiteUri,
			Types:   placeTypes,
		})
	}

	// persist to cache (best-effort, don't fail the request on cache write error)
	if len(toCache) > 0 {
		if cerr := models.SetGooglePlacesCache(category, market, toCache); cerr != nil {
			log.Printf("rfq_bot: places cache write error: %v", cerr)
		}
	}

	return suppliers, nil
}

// cachedSuppliersToRFQ converts cache entries back to RFQSupplier structs.
func cachedSuppliersToRFQ(cached []models.CachedSupplier, category, market string, storeID primitive.ObjectID) []models.RFQSupplier {
	suppliers := make([]models.RFQSupplier, 0, len(cached))
	for _, c := range cached {
		mapsURL := ""
		if c.PlaceID != "" {
			mapsURL = "https://www.google.com/maps/place/?q=place_id:" + c.PlaceID
		}
		// Restore full type list from cache; fall back to search category if none stored.
		cats := c.Types
		if len(cats) == 0 {
			cats = []string{category}
		} else {
			// Ensure the search category is always present.
			found := false
			for _, t := range cats {
				if t == category {
					found = true
					break
				}
			}
			if !found {
				cats = append([]string{category}, cats...)
			}
		}
		suppliers = append(suppliers, models.RFQSupplier{
			Name:           c.Name,
			Phone:          c.Phone,
			Address:        c.Address,
			Latitude:       c.Lat,
			Longitude:      c.Lng,
			Rating:         c.Rating,
			GooglePlaceID:  c.PlaceID,
			GoogleMapsURL:  mapsURL,
			Website:        c.Website,
			Categories:     cats,
			PurchaseMarket: marketTitleCase(market),
			StoreID:        storeID,
		})
	}
	return suppliers
}

// placeTypesToCategories converts Google Places primaryType / types to a de-duped
// human-readable category list. The search category is always first.
// Generic infrastructure types are filtered out.
// marketTitleCase normalises a market name to Title Case ("dammam" → "Dammam").
func marketTitleCase(s string) string {
	words := strings.Fields(strings.TrimSpace(s))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

func placeTypesToCategories(primaryType string, types []string, searchCategory string) []string {
	// Types that carry no useful product/business meaning.
	generic := map[string]bool{
		"point_of_interest": true, "establishment": true, "geocode": true,
		"locality": true, "political": true, "route": true, "street_address": true,
		"sublocality": true, "sublocality_level_1": true, "sublocality_level_2": true,
		"neighborhood": true, "premise": true, "postal_code": true,
		"country": true, "administrative_area_level_1": true,
		"administrative_area_level_2": true, "administrative_area_level_3": true,
		"colloquial_area": true, "natural_feature": true,
	}
	toTitle := func(s string) string {
		words := strings.Split(s, "_")
		for i, w := range words {
			if len(w) > 0 {
				words[i] = strings.ToUpper(w[:1]) + w[1:]
			}
		}
		return strings.Join(words, " ")
	}
	seen := map[string]bool{searchCategory: true}
	result := []string{searchCategory}
	for _, t := range append([]string{primaryType}, types...) {
		if t == "" || generic[t] {
			continue
		}
		label := toTitle(t)
		if !seen[label] {
			seen[label] = true
			result = append(result, label)
		}
	}
	return result
}

// hasWhatsApp returns true if the given phone has a WhatsApp account.
// Meta Cloud API does not expose a per-number existence check, so we always
// return true for non-empty numbers and let the send fail naturally if unreachable.
func hasWhatsApp(phone string) bool {
	return phone != ""
}

// digitsOnly strips all non-digit characters from a phone string.
func digitsOnly(phone string) string {
	var b strings.Builder
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// ── 7. Forward RFQ to Supplier via WhatsApp ───────────────────────────────────

func forwardRFQToSupplier(store *models.Store, rfq *models.RFQReceived, supplier *models.RFQSupplier, idx int) (bool, string, string) {
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		return false, "Bot WhatsApp not connected (no Meta phone_number_id)", ""
	}

	// First-contact detection: supplier was just discovered or added within the last 24h
	firstContact := supplier.GooglePlaceID != "" || supplier.AddedAt.After(time.Now().Add(-24*time.Hour))

	intro := generateSupplierIntro(store, supplier.Name, rfq.Categories, rfq.TextContent, idx, firstContact)
	originalContent := rfq.TextContent
	if originalContent == "" {
		originalContent = "(No text — please see the attached image/document)"
	}

	companyName := store.Name
	if companyName == "" {
		companyName = "our company"
	}

	customIntro := strings.TrimSpace(store.Settings.RFQIntro)

	var msg string
	if firstContact {
		opening := fmt.Sprintf("Hello, We are from *%s*.", companyName)
		if customIntro != "" {
			opening = customIntro
		}
		msg = fmt.Sprintf("%s\n\nDear %s,\n\n%s\n\n---\n%s", opening, supplier.Name, intro, originalContent)
	} else {
		greetings := []string{"Hello", "Hi", "Dear"}
		greeting := greetings[idx%len(greetings)]
		msg = fmt.Sprintf("%s %s,\n\n%s\n\n---\n%s", greeting, supplier.Name, intro, originalContent)
	}

	if _, err := metaSendText(phoneNumberID, accessToken, supplier.Phone, msg); err != nil {
		return false, err.Error(), ""
	}

	// Forward images
	for _, mediaURL := range rfq.MediaURLs {
		if err := metaSendImage(phoneNumberID, accessToken, supplier.Phone, mediaURL, "RFQ Image"); err != nil {
			log.Printf("rfq_bot: sendImage to %s failed: %v", supplier.Phone, err)
		}
		time.Sleep(2 * time.Second)
	}

	// Forward documents
	for _, doc := range rfq.Documents {
		if doc.URL == "" {
			continue
		}
		mimeType := doc.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if err := metaSendDocument(phoneNumberID, accessToken, supplier.Phone, doc.URL, mimeType, doc.FileName, "RFQ Document"); err != nil {
			log.Printf("rfq_bot: sendDoc to %s failed: %v", supplier.Phone, err)
		}
		time.Sleep(2 * time.Second)
	}

	return true, "", msg
}

// detectImageMIME returns the MIME type of image bytes based on magic bytes.
func detectImageMIME(data []byte) string {
	if len(data) >= 12 {
		// WebP: RIFF????WEBP
		if data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
			data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P' {
			return "image/webp"
		}
	}
	if len(data) >= 4 {
		if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
			return "image/png"
		}
		if data[0] == 'G' && data[1] == 'I' && data[2] == 'F' {
			return "image/gif"
		}
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return "image/jpeg"
	}
	return "image/jpeg" // safe fallback
}

// extractDocumentText extracts human-readable text from XLSX or CSV document bytes.
// For XLSX it reads the ZIP-based XML; for CSV/plain text it returns the raw content.
// Returns at most 4000 characters to keep LLM payloads reasonable.
func extractDocumentText(data []byte, mimeType, fileName string) string {
	ext := strings.ToLower(filepath.Ext(fileName))
	isXLSX := ext == ".xlsx" || ext == ".xls" ||
		strings.Contains(mimeType, "spreadsheet") || strings.Contains(mimeType, "excel")
	isCSV := ext == ".csv" || strings.Contains(mimeType, "csv")
	isText := ext == ".txt" || strings.HasPrefix(mimeType, "text/")

	if isCSV || isText {
		content := string(data)
		if len(content) > 4000 {
			content = content[:4000]
		}
		return content
	}

	if isXLSX {
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return ""
		}
		// XLSX stores all unique string values in xl/sharedStrings.xml
		var texts []string
		for _, f := range r.File {
			if f.Name != "xl/sharedStrings.xml" {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				break
			}
			xmlData, _ := io.ReadAll(rc)
			rc.Close()
			// Extract all <t>...</t> elements
			xmlStr := string(xmlData)
			for {
				start := strings.Index(xmlStr, "<t>")
				if start < 0 {
					start = strings.Index(xmlStr, "<t ")
					if start < 0 {
						break
					}
					end := strings.Index(xmlStr[start:], ">")
					if end < 0 {
						break
					}
					start += end + 1
				} else {
					start += 3
				}
				end := strings.Index(xmlStr[start:], "</t>")
				if end < 0 {
					break
				}
				val := strings.TrimSpace(xmlStr[start : start+end])
				if val != "" {
					texts = append(texts, val)
				}
				xmlStr = xmlStr[start+end+4:]
			}
			break
		}
		result := strings.Join(texts, " | ")
		if len(result) > 4000 {
			result = result[:4000]
		}
		return result
	}

	return ""
}

// extractPDFText extracts plain text from PDF bytes using ledongthuc/pdf.
// Returns at most 8000 characters.
func extractPDFText(data []byte) string {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		if sb.Len() >= 8000 {
			break
		}
	}
	result := sb.String()
	if len(result) > 8000 {
		result = result[:8000]
	}
	return result
}

// mimeTypeToExt returns a file extension (with leading dot) for a MIME type.
func mimeTypeToExt(mimeType string) string {
	m := strings.Split(strings.ToLower(mimeType), ";")[0] // strip params like "; codecs=opus"
	switch m {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/heic", "image/heif":
		return ".heic"
	case "video/mp4":
		return ".mp4"
	case "video/3gpp":
		return ".3gp"
	case "video/quicktime":
		return ".mov"
	case "audio/ogg", "audio/ogg; codecs=opus":
		return ".ogg"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/aac":
		return ".aac"
	case "audio/amr":
		return ".amr"
	case "audio/wav", "audio/wave":
		return ".wav"
	case "application/pdf":
		return ".pdf"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "application/msword":
		return ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "text/plain":
		return ".txt"
	default:
		return ".bin"
	}
}

// splitDataURI splits a data URI ("data:mime/type;base64,DATA") into MIME type and raw base64.
func splitDataURI(dataURI string) (mimeType, b64Data string) {
	if !strings.HasPrefix(dataURI, "data:") {
		return "image/jpeg", dataURI
	}
	rest := dataURI[5:]
	semi := strings.Index(rest, ";")
	comma := strings.Index(rest, ",")
	if semi < 0 || comma < 0 {
		return "image/jpeg", dataURI
	}
	return rest[:semi], rest[comma+1:]
}

// downloadImageAsBase64 returns a data URI for an image.
// If the input is already a data URI (e.g. from Evolution API's getBase64FromMediaMessage),
// it is returned as-is. Otherwise it is fetched via HTTP and the MIME type is auto-detected.
func downloadImageAsBase64(imageURL string) (string, error) {
	if strings.HasPrefix(imageURL, "data:") {
		return imageURL, nil
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	mime := detectImageMIME(data)
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// ── 8. RFQ Received CRUD Endpoints ───────────────────────────────────────────

// GET /v1/rfq-received?store_id=...&page=...&limit=...&status=...
func ListRFQReceivedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		http.Error(w, `{"error":"store_id required"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	// When supplier_phone is given, return only RFQs forwarded to that supplier.
	// Resolves phone2 → primary phone so that RFQs stored under any alias are found.
	// Also matches by supplier name as a fallback (user suggestion).
	if sp := strings.TrimPrefix(r.URL.Query().Get("supplier_phone"), "+"); sp != "" {
		// Resolve to supplier record to collect all known phones + name.
		var extraPhones []string
		var supplierName string
		if sup, err := models.FindRFQSupplierByPhone(storeObjID, sp); err == nil && sup != nil {
			supplierName = sup.Name
			if sup.Phone != "" && sup.Phone != sp {
				extraPhones = append(extraPhones, sup.Phone)
			}
			if sup.Phone2 != "" && sup.Phone2 != sp {
				extraPhones = append(extraPhones, sup.Phone2)
			}
		}
		rfqs, err := models.FindRFQsForwardedToSupplier(storeObjID, sp, supplierName, extraPhones, 365*24*time.Hour, 50)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if rfqs == nil {
			rfqs = []models.RFQReceived{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"items": rfqs, "total_count": len(rfqs)})
		return
	}

	// When customer_id is given, return RFQs received from that customer.
	if cidStr := r.URL.Query().Get("customer_id"); cidStr != "" {
		cidObj, err := primitive.ObjectIDFromHex(cidStr)
		if err != nil {
			http.Error(w, `{"error":"invalid customer_id"}`, http.StatusBadRequest)
			return
		}
		rfqs, err := models.FindRFQsByCustomerID(storeObjID, cidObj, 50)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		if rfqs == nil {
			rfqs = []models.RFQReceived{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"result": rfqs, "total_count": len(rfqs)})
		return
	}

	page := int64(1)
	limit := int64(20)
	fmt.Sscan(r.URL.Query().Get("page"), &page)
	fmt.Sscan(r.URL.Query().Get("limit"), &limit)
	if page < 1 {
		page = 1
	}
	statusFilter := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")

	result, err := models.ListRFQReceived(storeObjID, page, limit, statusFilter, search)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	respBytes, _ := json.Marshal(result)
	w.Write(respBytes)
}

// GET /v1/rfq-received/{id}?store_id=...
func GetRFQReceivedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	respBytes, _ := json.Marshal(rfq)
	w.Write(respBytes)
}

// POST /v1/rfq-received/{id}/process?store_id=... — manually re-trigger processing
func TriggerRFQProcess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	rfq.Status = "received"
	rfq.ForwardedTo = nil
	rfq.ErrorMsg = ""
	rfq.Categories = nil
	models.UpdateRFQReceived(rfq)

	go processRFQ(rfq, storeObjID)
	fmt.Fprint(w, `{"success":true,"message":"Processing started"}`)
}

// CreateRFQReceivedHandler creates a manual RFQ entry.
// POST /v1/rfq-received?store_id=...
func CreateRFQReceivedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		CustomerID                   string              `json:"customer_id"`
		CustomerName                 string              `json:"customer_name"`
		CustomerPhone                string              `json:"customer_phone"`
		CustomerEmail                string              `json:"customer_email"`
		CustomerCompany              string              `json:"customer_company"`
		CustomerRFQID                string              `json:"customer_rfq_id"`
		TextContent                  string              `json:"text_content"`
		Products                     []models.RFQProduct `json:"products"`
		Categories                   []string            `json:"product_categories"`
		ExtractionModel              string              `json:"extraction_model"`
		AttachmentDataURIs           []string            `json:"attachment_data_uris"`
		AdditionalAttachmentDataURIs  []string            `json:"additional_attachment_data_uris"`
		AdditionalAttachmentFilenames []string            `json:"additional_attachment_filenames"`
		GeneralInstructions          string              `json:"general_instructions"`
		ProcurementMessageID         string              `json:"procurement_message_id"`
		ProcurementMessageCode       string              `json:"procurement_message_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if len(body.Products) == 0 && body.TextContent == "" && len(body.AttachmentDataURIs) == 0 {
		http.Error(w, `{"error":"products, text_content, or attachment files required"}`, http.StatusBadRequest)
		return
	}

	rfq := &models.RFQReceived{
		StoreID:                       storeObjID,
		Source:                        "manual",
		MessageType:                   "text",
		CustomerName:                  body.CustomerName,
		CustomerPhone:                 body.CustomerPhone,
		CustomerEmail:                 body.CustomerEmail,
		CustomerCompany:               body.CustomerCompany,
		CustomerRFQID:                 body.CustomerRFQID,
		TextContent:                   body.TextContent,
		Products:                      body.Products,
		Categories:                    body.Categories,
		AttachmentDataURIs:            body.AttachmentDataURIs,
		AdditionalAttachmentDataURIs:  body.AdditionalAttachmentDataURIs,
		AdditionalAttachmentFilenames: body.AdditionalAttachmentFilenames,
		GeneralInstructions:           body.GeneralInstructions,
		Status:                        "ready_to_send",
	}
	if body.ProcurementMessageID != "" {
		if msgObjID, err := primitive.ObjectIDFromHex(body.ProcurementMessageID); err == nil {
			rfq.ProcurementMessageID = &msgObjID
			rfq.ProcurementMessageCode = body.ProcurementMessageCode
		}
	}
	if body.CustomerID != "" {
		if custObjID, err2 := primitive.ObjectIDFromHex(body.CustomerID); err2 == nil {
			rfq.CustomerID = &custObjID
		}
	}
	if rfq.CustomerPhone != "" {
		rfq.FromPhone = rfq.CustomerPhone
	}
	if rfq.CustomerName != "" {
		rfq.FromName = rfq.CustomerName
	}

	if err := models.CreateRFQReceived(rfq); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if rfq.ProcurementMessageID != nil {
		models.LinkProcurementMessageToRFQ(*rfq.ProcurementMessageID, rfq.ID, rfq.Code) //nolint:errcheck
	}

	// Timeline logs for manual RFQ
	inputMsg := "Input received via manual form"
	if rfq.CustomerName != "" {
		inputMsg = fmt.Sprintf("Input received via manual form from %s", rfq.CustomerName)
		if rfq.CustomerPhone != "" {
			ph := rfq.CustomerPhone
			if !strings.HasPrefix(ph, "+") {
				ph = "+" + ph
			}
			inputMsg += " (" + ph + ")"
		}
	}
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "input_received",
		Message: inputMsg,
		Icon:    "bi-pencil-square", Color: "secondary",
		Details: map[string]interface{}{"source": "manual"},
	})
	if len(rfq.Products) > 0 {
		totalQty := 0.0
		prodList := make([]map[string]interface{}, len(rfq.Products))
		for i, p := range rfq.Products {
			totalQty += p.Quantity
			qty := p.Quantity
			if qty == 0 {
				qty = 1
			}
			prodList[i] = map[string]interface{}{
				"part_no":  p.PartNo,
				"name":     p.Name,
				"quantity": qty,
				"unit":     p.Unit,
			}
		}
		details := map[string]interface{}{
			"products": prodList,
		}
		if body.ExtractionModel != "" {
			details["llm_model"] = body.ExtractionModel
		}
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step: "products_identified",
			Message: fmt.Sprintf("Identified %d product(s) — total qty: %.0f",
				len(rfq.Products), totalQty),
			Details: details,
		})
	}
	if rfq.CustomerName != "" {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step:    "customer_identified",
			Message: fmt.Sprintf("Customer: %s", rfq.CustomerName),
			Icon:    "bi-person-check", Color: "info",
			Details: map[string]interface{}{"customer_name": rfq.CustomerName, "customer_phone": rfq.CustomerPhone, "customer_company": rfq.CustomerCompany},
		})
	}
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "rfq_created",
		Message: fmt.Sprintf("RFQ created with code %s — ready to send to suppliers", rfq.Code),
		Icon:    "bi-file-earmark-check", Color: "primary",
		Details: map[string]interface{}{"code": rfq.Code, "status": "ready_to_send"},
	})

	respBytes, _ := json.Marshal(map[string]interface{}{"success": true, "id": rfq.ID.Hex(), "code": rfq.Code})
	w.Write(respBytes)

	go autoCategorizeAndFindSuppliers(rfq, storeObjID)
}

// autoCategorizeAndFindSuppliers identifies product categories via LLM and finds suppliers per
// category per purchase market for a manually created RFQ.  Runs as a background goroutine after
// CreateRFQReceivedHandler returns so the HTTP response is not delayed.
func autoCategorizeAndFindSuppliers(rfq *models.RFQReceived, storeID primitive.ObjectID) {
	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		log.Printf("rfq_bot[autoCategorize]: store not found: %v", err)
		return
	}

	storeIDStr := storeID.Hex()
	rfqIDStr := rfq.ID.Hex()

	broadcast := func(stage string, pct int, msg string, extra map[string]interface{}) {
		data := map[string]interface{}{
			"rfq_id": rfqIDStr, "stage": stage, "percent": pct, "message": msg,
		}
		for k, v := range extra {
			data[k] = v
		}
		BroadcastRFQData(storeIDStr, "rfq_progress", data)
	}

	if store.Settings.RFQLLMAPIKey == "" {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step:    "ai_skipped",
			Message: "AI categorization skipped — LLM API key not configured in store procurement settings",
			Icon:    "bi-info-circle", Color: "secondary",
		})
		broadcast("ai_skipped", 0, "AI not configured — set LLM API key in store procurement settings", nil)
		return
	}

	// Build LLM text from free-text and product list
	llmText := rfq.TextContent
	if len(rfq.Products) > 0 {
		var lines []string
		for _, p := range rfq.Products {
			line := p.Name
			if p.PartNo != "" {
				line += " (Part No: " + p.PartNo + ")"
			}
			if p.Quantity > 0 {
				line += fmt.Sprintf(" qty: %.0f", p.Quantity)
			}
			lines = append(lines, line)
		}
		productsText := "Products:\n" + strings.Join(lines, "\n")
		if llmText != "" {
			llmText = llmText + "\n\n" + productsText
		} else {
			llmText = productsText
		}
	}
	if strings.TrimSpace(llmText) == "" {
		return
	}

	// Step 1: Identify categories via LLM
	broadcast("classifying", 10, "AI is identifying product categories from the RFQ...", nil)
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step:    "ai_categorizing",
		Message: "Identifying product categories using AI...",
		Icon:    "bi-cpu", Color: "secondary",
	})
	log.Printf("rfq_bot[autoCategorize][%s]: identifying categories", rfqIDStr)

	categories, err := identifyCategories(store, llmText, nil)
	if err != nil {
		log.Printf("rfq_bot[autoCategorize][%s]: LLM error: %v", rfqIDStr, err)
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step:    "categories_error",
			Message: "Category identification failed: " + err.Error(),
			Icon:    "bi-exclamation-circle", Color: "danger",
			Details: map[string]interface{}{"error": err.Error()},
		})
		broadcast("failed", 100, "Category identification failed: "+err.Error(), nil)
		return
	}

	rfq.Categories = categories
	if err := models.SetRFQCategories(rfq.StoreID, rfq.ID, categories); err != nil {
		log.Printf("rfq_bot[autoCategorize][%s]: save categories error: %v", rfqIDStr, err)
		return
	}
	llmModelName := store.Settings.RFQLLMModel
	if llmModelName == "" {
		llmModelName = store.Settings.RFQLLMProvider
	}
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step: "categories_identified",
		Message: fmt.Sprintf("Identified %d categor(y/ies): %s  [model: %s]",
			len(categories), strings.Join(categories, ", "), llmModelName),
		Icon: "bi-tags", Color: "info",
		Details: map[string]interface{}{"categories": categories, "model": llmModelName},
	})
	broadcast("categories_identified", 40, fmt.Sprintf("Categories identified: %s", strings.Join(categories, ", ")),
		map[string]interface{}{"categories": categories})

	// Step 2: Find suppliers per category per purchase market
	numMarkets := len(store.Settings.PurchaseMarkets)
	if numMarkets == 0 {
		numMarkets = 1
	}
	broadcast("finding_suppliers", 50,
		fmt.Sprintf("Searching for suppliers across %d market(s) × %d categor(y/ies)...", numMarkets, len(categories)), nil)
	log.Printf("rfq_bot[autoCategorize][%s]: finding suppliers for %v in %d market(s)", rfqIDStr, categories, numMarkets)

	suppliers, err := findSuppliers(store, storeID, categories)
	if err != nil {
		log.Printf("rfq_bot[autoCategorize][%s]: supplier search error: %v", rfqIDStr, err)
	}

	minRequired := rfqMinSuppliers(store)

	// Fallback: broaden if below minimum (not just when 0)
	if len(suppliers) < minRequired && store.Settings.GoogleMapsAPIKey != "" {
		broadcast("finding_suppliers", 70,
			fmt.Sprintf("Only %d/%d suppliers found — trying broader search terms...", len(suppliers), minRequired), nil)
		broaderCategories := broadenCategories(store, categories, llmText)
		if len(broaderCategories) > 0 {
			log.Printf("rfq_bot[autoCategorize][%s]: broadening to %v (have %d/%d)", rfqIDStr, broaderCategories, len(suppliers), minRequired)
			broadened, _ := findSuppliers(store, storeID, broaderCategories)
			// Merge broadened results (dedup by phone)
			existingPhones := map[string]bool{}
			for _, s := range suppliers {
				existingPhones[s.Phone] = true
			}
			for _, s := range broadened {
				if !existingPhones[s.Phone] {
					suppliers = append(suppliers, s)
					existingPhones[s.Phone] = true
				}
			}
		}
	} else if len(suppliers) < minRequired && store.Settings.GoogleMapsAPIKey == "" {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step:    "suppliers_below_minimum",
			Message: fmt.Sprintf("Only %d/%d supplier(s) found. Google Maps API key not configured — cannot search for more.", len(suppliers), minRequired),
			Icon:    "bi-exclamation-triangle", Color: "warning",
			Details: map[string]interface{}{"categories": categories, "markets": store.Settings.PurchaseMarkets, "found": len(suppliers), "min_required": minRequired},
		})
	}
	if len(suppliers) == 0 {
		models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
			Step:    "no_suppliers_found",
			Message: fmt.Sprintf("No suppliers found in rfq-suppliers or Google Maps for: %s", strings.Join(categories, ", ")),
			Icon:    "bi-exclamation-triangle", Color: "warning",
			Details: map[string]interface{}{"categories": categories, "markets": store.Settings.PurchaseMarkets},
		})
		broadcast("suppliers_found", 100, "No suppliers found — add them manually in RFQ Suppliers page", nil)
		return
	}

	// Persist matched supplier IDs on the RFQ so the Send preview can show them
	// without re-running findSuppliers (which may miss suppliers from broader search).
	// Use targeted $set to avoid overwriting activity_logs appended via $push.
	var matchedIDs []primitive.ObjectID
	for _, s := range suppliers {
		if !s.ID.IsZero() {
			matchedIDs = append(matchedIDs, s.ID)
		}
	}
	if len(matchedIDs) > 0 {
		if err := models.SetRFQMatchedSupplierIDs(rfq.StoreID, rfq.ID, matchedIDs); err != nil {
			log.Printf("rfq_bot: failed to save matched supplier IDs to RFQ: %v", err)
		}
	}

	supplierList := make([]map[string]string, 0, len(suppliers))
	for _, s := range suppliers {
		supplierList = append(supplierList, map[string]string{"name": s.Name, "phone": s.Phone})
	}
	models.AppendRFQLog(rfq.StoreID, rfq.ID, models.RFQActivityLog{
		Step: "suppliers_found",
		Message: fmt.Sprintf("Found %d supplier(s) — %d market(s) × %d categor(y/ies) × %d min required each",
			len(suppliers), numMarkets, len(categories), minRequired),
		Icon: "bi-people-fill", Color: "success",
		Details: map[string]interface{}{
			"supplier_count": len(suppliers),
			"markets":        store.Settings.PurchaseMarkets,
			"categories":     categories,
			"min_required":   minRequired,
			"suppliers":      supplierList,
		},
	})
	broadcast("suppliers_found", 100,
		fmt.Sprintf("Found %d supplier(s) ready — %d markets × %d categories × %d min each",
			len(suppliers), numMarkets, len(categories), minRequired),
		map[string]interface{}{"supplier_count": len(suppliers)})
}

// UpdateRFQReceivedHandler updates editable fields of an existing RFQ.
// PUT /v1/rfq-received/{id}?store_id=...
func UpdateRFQReceivedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		CustomerID                   string              `json:"customer_id"`
		CustomerName                 string              `json:"customer_name"`
		CustomerPhone                string              `json:"customer_phone"`
		CustomerEmail                string              `json:"customer_email"`
		CustomerCompany              string              `json:"customer_company"`
		CustomerRFQID                string              `json:"customer_rfq_id"`
		TextContent                  string              `json:"text_content"`
		Products                     []models.RFQProduct `json:"products"`
		GeneralInstructions          string              `json:"general_instructions"`
		AdditionalAttachmentDataURIs  []string            `json:"additional_attachment_data_uris"`
		AdditionalAttachmentFilenames []string            `json:"additional_attachment_filenames"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		http.Error(w, `{"error":"RFQ not found"}`, http.StatusNotFound)
		return
	}

	rfq.CustomerName        = body.CustomerName
	rfq.CustomerPhone       = body.CustomerPhone
	rfq.CustomerEmail       = body.CustomerEmail
	rfq.CustomerCompany     = body.CustomerCompany
	rfq.CustomerRFQID       = body.CustomerRFQID
	rfq.TextContent         = body.TextContent
	rfq.GeneralInstructions = body.GeneralInstructions
	if body.CustomerID != "" {
		if custObjID, err2 := primitive.ObjectIDFromHex(body.CustomerID); err2 == nil {
			rfq.CustomerID = &custObjID
		}
	} else {
		rfq.CustomerID = nil
	}
	if body.Products != nil {
		rfq.Products = body.Products
	}
	if body.AdditionalAttachmentDataURIs != nil {
		rfq.AdditionalAttachmentDataURIs = body.AdditionalAttachmentDataURIs
	}
	if body.AdditionalAttachmentFilenames != nil {
		rfq.AdditionalAttachmentFilenames = body.AdditionalAttachmentFilenames
	}

	if err := models.UpdateRFQReceived(rfq); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(rfq)
}

// AddSupplierReplyHandler manually adds a supplier reply to an RFQ and triggers price extraction.
// POST /v1/rfq-received/{id}/supplier-reply?store_id=...
func AddSupplierReplyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		SupplierName  string `json:"supplier_name"`
		SupplierPhone string `json:"supplier_phone"`
		RawText       string `json:"raw_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if body.SupplierName == "" && body.SupplierPhone == "" {
		http.Error(w, `{"error":"supplier_name or supplier_phone required"}`, http.StatusBadRequest)
		return
	}

	if _, err := models.FindRFQReceivedByID(id, storeObjID); err != nil {
		http.Error(w, `{"error":"rfq not found"}`, http.StatusNotFound)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}

	reply := models.SupplierReply{
		SupplierName:     body.SupplierName,
		SupplierPhone:    body.SupplierPhone,
		ReceivedAt:       time.Now(),
		RawText:          body.RawText,
		ExtractionStatus: "pending",
	}
	if err := models.AddSupplierReplyToRFQ(storeObjID, id, reply); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Trigger async price extraction.
	go func() {
		updated, err := models.FindRFQReceivedByID(id, storeObjID)
		if err != nil {
			return
		}
		for i := len(updated.SupplierReplies) - 1; i >= 0; i-- {
			r := updated.SupplierReplies[i]
			if r.SupplierName == body.SupplierName && r.SupplierPhone == body.SupplierPhone && r.ExtractionStatus == "pending" {
				extractSupplierPrices(store, updated, &updated.SupplierReplies[i])
				break
			}
		}
	}()

	fmt.Fprint(w, `{"success":true,"message":"Reply added and price extraction started"}`)
}

// ── 9. RFQ Supplier CRUD Endpoints ───────────────────────────────────────────

// GET /v1/rfq-suppliers?store_id=...&page=...&limit=...&search=...
func ListRFQSuppliersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		http.Error(w, `{"error":"store_id required"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	page := int64(1)
	limit := int64(20)
	fmt.Sscan(r.URL.Query().Get("page"), &page)
	fmt.Sscan(r.URL.Query().Get("limit"), &limit)
	if page < 1 {
		page = 1
	}
	search := r.URL.Query().Get("search")

	result, err := models.ListRFQSuppliers(storeObjID, page, limit, search, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	respBytes, _ := json.Marshal(result)
	w.Write(respBytes)
}

// POST /v1/rfq-suppliers
func CreateRFQSupplierHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var supplier models.RFQSupplier
	if err := json.NewDecoder(r.Body).Decode(&supplier); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		storeIDStr = supplier.StoreID.Hex()
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	supplier.StoreID = storeObjID

	if supplier.Phone == "" {
		http.Error(w, `{"error":"phone (WhatsApp number) is required"}`, http.StatusBadRequest)
		return
	}
	if existing, _ := models.FindRFQSupplierByPhone(storeObjID, supplier.Phone); existing != nil {
		http.Error(w, `{"error":"a supplier with this phone number already exists"}`, http.StatusConflict)
		return
	}
	if err := models.CreateRFQSupplier(&supplier); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	// Crawl website for contact email in background if not already provided.
	if supplier.Website != "" && supplier.Email == "" {
		go crawlAndSaveSupplierEmail(supplier)
	}
	BroadcastRFQEvent(supplier.StoreID.Hex(), "supplier_updated")
	respBytes, _ := json.Marshal(supplier)
	w.Write(respBytes)
}

// POST /v1/rfq-suppliers/fetch-from-maps
// Fetches suppliers from Google Maps for the given RFQ's categories × selected markets,
// saves/updates them in rfq_suppliers, and returns the found list so the frontend
// can immediately add them to the RECIPIENTS section.
func FetchSuppliersFromMapsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	var body struct {
		RFQID    string   `json:"rfq_id"`
		Markets  []string `json:"markets"`
		MinCount int      `json:"min_count"`
		MaxCount int      `json:"max_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if len(body.Markets) == 0 {
		http.Error(w, `{"error":"select at least one market"}`, http.StatusBadRequest)
		return
	}
	// Normalize market names to Title Case so "dammam" and "Dammam" are treated identically.
	for i, m := range body.Markets {
		body.Markets[i] = marketTitleCase(m)
	}

	store, err := models.FindStoreByID(&storeObjID, nil)
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	if store.Settings.GoogleMapsAPIKey == "" {
		http.Error(w, `{"error":"Google Maps API key not configured in store settings"}`, http.StatusBadRequest)
		return
	}

	rfqObjID, _ := primitive.ObjectIDFromHex(body.RFQID)
	rfq, err := models.FindRFQReceivedByID(rfqObjID, storeObjID)
	if err != nil {
		http.Error(w, `{"error":"RFQ not found"}`, http.StatusNotFound)
		return
	}
	if len(rfq.Categories) == 0 {
		http.Error(w, `{"error":"RFQ has no product categories identified yet — identify categories first"}`, http.StatusBadRequest)
		return
	}

	minPerSearch := body.MinCount
	if minPerSearch < 1 {
		minPerSearch = 5
	}
	maxPerSearch := body.MaxCount
	if maxPerSearch < minPerSearch {
		maxPerSearch = minPerSearch
	}
	if maxPerSearch < 1 {
		maxPerSearch = 20
	}
	if maxPerSearch > 20 {
		maxPerSearch = 20
	}

	type supplierEntry struct {
		supplier   models.RFQSupplier
		categories map[string]bool
	}
	addCategory := func(e *supplierEntry, cat string) {
		if e.categories == nil {
			e.categories = map[string]bool{}
		}
		e.categories[cat] = true
	}

	byKey := map[string]*supplierEntry{} // keyed by google_place_id, fallback phone

	entryKey := func(s models.RFQSupplier) string {
		if s.GooglePlaceID != "" {
			return s.GooglePlaceID
		}
		return s.Phone
	}

	// Phase 1 — DB first, then Google Maps for the gap.
	// For each RFQ category × market we first pull from our own rfq_suppliers DB.
	// Only if the DB count is below maxPerSearch do we call Google Maps to fill the gap.
	// This guarantees ≥1 RFQ category match and avoids unnecessary API calls.
	for _, market := range body.Markets {
		for _, category := range rfq.Categories {
			// 1a. DB lookup.
			dbSups, _ := models.FindRFQSuppliersByMarketAndCategories(storeObjID, []string{category}, market, int64(maxPerSearch))
			for _, s := range dbSups {
				k := entryKey(s)
				if k == "" {
					continue
				}
				if e, exists := byKey[k]; exists {
					addCategory(e, category)
				} else {
					e = &supplierEntry{supplier: s}
					addCategory(e, category)
					byKey[k] = e
				}
			}

			// 1b. Google Maps for the gap.
			gap := maxPerSearch - len(dbSups)
			if gap > 0 {
				mapsSups, err := searchGoogleMapsSuppliers(store.Settings.GoogleMapsAPIKey, category, market, storeObjID, gap)
				if err != nil {
					log.Printf("FetchSuppliersFromMaps: maps error cat=%q market=%q: %v", category, market, err)
				} else {
					for _, s := range mapsSups {
						k := entryKey(s)
						if k == "" {
							continue
						}
						if e, exists := byKey[k]; exists {
							addCategory(e, category)
						} else {
							e = &supplierEntry{supplier: s}
							addCategory(e, category)
							byKey[k] = e
						}
					}
				}
			}
		}
	}

	// Phase 2 — enrich qualifying suppliers with all other store product categories.
	// Check DB and Google Maps for each extra category × market; add that category ONLY
	// to already-qualifying suppliers — no new suppliers are added here.
	if len(byKey) > 0 {
		allStoreCategories, _ := models.GetAllProductCategoryNames(store)
		rfqCatSet := map[string]bool{}
		for _, c := range rfq.Categories {
			rfqCatSet[c] = true
		}
		for _, market := range body.Markets {
			for _, category := range allStoreCategories {
				if rfqCatSet[category] {
					continue // already covered in Phase 1
				}
				// DB lookup for this category+market.
				dbSups, _ := models.FindRFQSuppliersByMarketAndCategories(storeObjID, []string{category}, market, int64(maxPerSearch))
				for _, s := range dbSups {
					k := entryKey(s)
					if e, exists := byKey[k]; exists {
						addCategory(e, category)
					}
				}
				// Google Maps for the gap (only if DB didn't saturate).
				gap := maxPerSearch - len(dbSups)
				if gap > 0 {
					mapsSups, err := searchGoogleMapsSuppliers(store.Settings.GoogleMapsAPIKey, category, market, storeObjID, gap)
					if err != nil {
						continue
					}
					for _, s := range mapsSups {
						k := entryKey(s)
						if e, exists := byKey[k]; exists {
							addCategory(e, category)
						}
					}
				}
			}
		}
	}

	// Upsert each qualifying supplier with its full accumulated category list.
	seenPhone := map[string]bool{}
	var allFound []models.RFQSupplier
	for _, entry := range byKey {
		s := entry.supplier
		s.Categories = make([]string, 0, len(entry.categories))
		for c := range entry.categories {
			s.Categories = append(s.Categories, c)
		}
		if s.Phone == "" || seenPhone[s.Phone] {
			continue
		}
		seenPhone[s.Phone] = true
		if uErr := models.UpsertRFQSupplierByPlaceID(&s); uErr != nil {
			log.Printf("FetchSuppliersFromMaps: upsert error phone=%s: %v", s.Phone, uErr)
		}
		// When Google Maps returned no categories but the supplier has a website,
		// crawl the website and ask the LLM to infer categories asynchronously.
		if len(s.Categories) == 0 && s.Website != "" {
			go inferSupplierCategoriesFromWebsite(store, s)
		}
		allFound = append(allFound, s)
	}

	if allFound == nil {
		allFound = []models.RFQSupplier{}
	}
	BroadcastRFQEvent(storeIDStr, "supplier_updated")

	respBytes, _ := json.Marshal(map[string]interface{}{
		"found":     len(allFound),
		"suppliers": allFound,
	})
	w.Write(respBytes)
}

// PUT /v1/rfq-suppliers/{id}
func UpdateRFQSupplierHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	existing, err := models.FindRFQSupplierByID(id, storeObjID)
	if err != nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(existing); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	existing.ID = id
	existing.StoreID = storeObjID

	if err := models.UpdateRFQSupplier(existing); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	// Crawl for email in background if website was added and email is still missing.
	if existing.Website != "" && existing.Email == "" {
		go crawlAndSaveSupplierEmail(*existing)
	}
	BroadcastRFQEvent(existing.StoreID.Hex(), "supplier_updated")
	respBytes, _ := json.Marshal(existing)
	w.Write(respBytes)
}

// DELETE /v1/rfq-suppliers/{id}?store_id=...
func DeleteRFQSupplierHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	if err := models.DeleteRFQSupplier(id, storeObjID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, `{"success":true}`)
}

// POST /v1/rfq-suppliers/deduplicate?store_id=...
// Removes duplicate rfq_supplier records by phone number for the given store.
func DeduplicateRFQSuppliersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	removed, err := models.DeduplicateRFQSuppliers(storeObjID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "removed": removed})
}

// ── Populate RFQ Suppliers from Vendors ──────────────────────────────────────

// PopulateSuppliersFromVendors starts a background job that iterates all vendors,
// collects their purchase product history, asks the LLM for categories, queries
// Google Maps to find the vendor's WhatsApp number, and upserts RFQ supplier records.
// Real-time progress is streamed via SSE (event: populate_progress).
// POST /v1/rfq-bot/populate-suppliers?store_id=...
func PopulateSuppliersFromVendors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil || store == nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	go populateSuppliersFromVendors(store, storeObjID)
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprint(w, `{"status":"started"}`)
}

func populateSuppliersFromVendors(store *models.Store, storeID primitive.ObjectID) {
	storeIDStr := storeID.Hex()

	sendProgress := func(step, total int, msg string, done bool) {
		pct := 0
		if total > 0 {
			pct = step * 100 / total
		}
		BroadcastRFQData(storeIDStr, "populate_progress", map[string]interface{}{
			"step": step, "total": total, "percent": pct, "message": msg, "done": done,
		})
	}

	sendProgress(0, 100, "Loading vendors...", false)

	vendors, err := rfqFetchAllVendors(storeID)
	if err != nil || len(vendors) == 0 {
		sendProgress(100, 100, "No vendors found.", true)
		return
	}

	total := len(vendors)
	created := 0

	markets := store.Settings.PurchaseMarkets
	if len(markets) == 0 {
		markets = []string{""}
	}

	for i, vendor := range vendors {
		step := (i + 1) * 90 / total
		sendProgress(step, 100, fmt.Sprintf("Processing %d/%d: %s — identifying product categories…", i+1, total, vendor.Name), false)

		var categories []string

		// Use product_categories already set on the vendor form (no LLM call needed).
		if len(vendor.ProductCategories) > 0 {
			categories = vendor.ProductCategories
			log.Printf("rfq_bot: vendor %s already has %d product categories — skipping LLM", vendor.Name, len(categories))
		} else {
			// Derive categories from purchased products via LLM.
			products := rfqFetchVendorProducts(storeID, vendor.ID)
			if len(products) == 0 {
				sendProgress(step, 100, fmt.Sprintf("Processing %d/%d: %s — no purchase history, skipping", i+1, total, vendor.Name), false)
				continue
			}
			categories = rfqCategorizeVendorProducts(store, vendor.Name, products)
			if len(categories) > 0 {
				// Persist the LLM-derived categories back onto the vendor record so future
				// runs skip the LLM and users see them in the vendor form.
				rfqSaveVendorProductCategories(storeID, vendor.ID, categories)
			}
		}

		if len(categories) == 0 {
			sendProgress(step, 100, fmt.Sprintf("Processing %d/%d: %s — could not identify categories, skipping", i+1, total, vendor.Name), false)
			continue
		}

		if store.Settings.GoogleMapsAPIKey == "" {
			continue
		}

		for _, market := range markets {
			results, err := searchGoogleMapsSuppliers(store.Settings.GoogleMapsAPIKey, vendor.Name, market, storeID, 5)
			if err != nil || len(results) == 0 {
				continue
			}
			for j := range results {
				sup := &results[j]
				sup.StoreID = storeID
				sup.Categories = categories
				sup.IsActive = true
				if sup.Phone == "" {
					continue
				}
				models.UpsertRFQSupplierByPlaceID(sup)
				created++
				break // best match per market
			}
		}
	}

	BroadcastRFQEvent(storeIDStr, "supplier_updated")
	sendProgress(100, 100, fmt.Sprintf("Done. Processed %d vendors, %d RFQ suppliers created/updated.", total, created), true)
}

// rfqFetchAllVendors returns all non-deleted vendors for the given store.
func rfqFetchAllVendors(storeID primitive.ObjectID) ([]models.Vendor, error) {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("vendor")
	ctx := context.Background()
	cur, err := collection.Find(ctx, bson.M{"deleted": bson.M{"$ne": true}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var vendors []models.Vendor
	for cur.Next(ctx) {
		var v models.Vendor
		if err := cur.Decode(&v); err != nil {
			continue
		}
		vendors = append(vendors, v)
	}
	return vendors, nil
}

// rfqSaveVendorProductCategories persists LLM-derived categories back to the vendor record
// so future populate runs skip the LLM call. It merges without duplicates.
func rfqSaveVendorProductCategories(storeID, vendorID primitive.ObjectID, categories []string) {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("vendor")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": vendorID},
		bson.M{"$addToSet": bson.M{"product_categories": bson.M{"$each": categories}}},
	)
	if err != nil {
		log.Printf("rfq_bot: failed to save product_categories for vendor %s: %v", vendorID.Hex(), err)
	}
}

// rfqFetchVendorProducts returns distinct product names/part numbers from all purchases of a vendor.
func rfqFetchVendorProducts(storeID, vendorID primitive.ObjectID) []string {
	collection := db.GetDB("store_" + storeID.Hex()).Collection("purchase")
	ctx := context.Background()
	cur, err := collection.Find(ctx, bson.M{"vendor_id": vendorID})
	if err != nil {
		return nil
	}
	defer cur.Close(ctx)
	seen := map[string]bool{}
	var names []string
	for cur.Next(ctx) {
		var p struct {
			Products []struct {
				Name       string `bson:"name"`
				PartNumber string `bson:"part_number"`
			} `bson:"products"`
		}
		if err := cur.Decode(&p); err != nil {
			continue
		}
		for _, prod := range p.Products {
			if prod.Name != "" && !seen[prod.Name] {
				seen[prod.Name] = true
				names = append(names, prod.Name)
			}
			if prod.PartNumber != "" && !seen[prod.PartNumber] {
				seen[prod.PartNumber] = true
				names = append(names, prod.PartNumber)
			}
		}
	}
	return names
}

// rfqCategorizeVendorProducts calls the LLM to derive trade categories from a vendor's product list.
func rfqCategorizeVendorProducts(store *models.Store, vendorName string, products []string) []string {
	// Use populate-suppliers-specific provider/model when configured; fall back to main RFQ LLM.
	llmProvider := store.Settings.PopulateSuppliersLLMProvider
	llmModel := store.Settings.PopulateSuppliersLLMModel
	llmAPIKey := ""
	if llmProvider != "" {
		llmAPIKey = resolveExtractionAPIKey(llmProvider, &store.Settings)
	}
	if llmProvider == "" || llmAPIKey == "" {
		llmProvider = store.Settings.RFQLLMProvider
		llmModel = store.Settings.RFQLLMModel
		llmAPIKey = store.Settings.RFQLLMAPIKey
	}
	if llmAPIKey == "" {
		return nil
	}
	cap := 50
	if len(products) < cap {
		cap = len(products)
	}
	productList := strings.Join(products[:cap], "\n")
	prompt := fmt.Sprintf(`Vendor: %s
Products purchased from this vendor:
%s

List 1-5 short English trade category names (e.g. "Steel Pipes", "Rubber Couplings", "Electrical Equipment") that best describe what this vendor supplies. Respond ONLY with a JSON array of strings.`, vendorName, productList)

	resp, err := callLLMText(llmAPIKey, llmModel, prompt, llmProvider)
	if err != nil {
		return nil
	}
	resp = strings.TrimSpace(resp)
	if i := strings.Index(resp, "["); i >= 0 {
		resp = resp[i:]
	}
	if i := strings.LastIndex(resp, "]"); i >= 0 {
		resp = resp[:i+1]
	}
	var cats []string
	if err := json.Unmarshal([]byte(resp), &cats); err != nil {
		return nil
	}
	return cats
}

// syncVendorToRFQSupplier is called after a purchase is created/updated when
// EnableRFQSupplierOnPurchase is set. It runs in a goroutine.
func syncVendorToRFQSupplier(store *models.Store, storeID primitive.ObjectID, vendorID primitive.ObjectID) {
	if store.Settings.GoogleMapsAPIKey == "" {
		return
	}
	// Require at least one usable LLM key: populate-suppliers provider key, or legacy RFQ LLM key.
	populateProvider := store.Settings.PopulateSuppliersLLMProvider
	hasLLMKey := store.Settings.RFQLLMAPIKey != "" ||
		(populateProvider != "" && resolveExtractionAPIKey(populateProvider, &store.Settings) != "")
	if !hasLLMKey {
		return
	}
	vendor, err := store.FindVendorByID(&vendorID, bson.M{})
	if err != nil || vendor == nil {
		return
	}
	products := rfqFetchVendorProducts(storeID, vendorID)
	categories := rfqCategorizeVendorProducts(store, vendor.Name, products)
	if len(categories) == 0 {
		return
	}

	markets := store.Settings.PurchaseMarkets
	if len(markets) == 0 {
		markets = []string{""}
	}

	for _, market := range markets {
		results, err := searchGoogleMapsSuppliers(store.Settings.GoogleMapsAPIKey, vendor.Name, market, storeID, 5)
		if err != nil || len(results) == 0 {
			continue
		}
		for j := range results {
			sup := &results[j]
			sup.StoreID = storeID
			sup.Categories = categories
			sup.IsActive = true
			if sup.Phone == "" {
				continue
			}
			models.UpsertRFQSupplierByPlaceID(sup)
			BroadcastRFQEvent(storeID.Hex(), "supplier_updated")
			break
		}
	}
}

// ── ExtractRFQFromFilesHandler ─────────────────────────────────────────────────
// POST /v1/rfq-received/extract
// Accepts multipart form with field "files" (images, PDFs, Excel, text).
// Extracts customer + product info via LLM and returns a JSON preview for the
// frontend to show and confirm before the user creates the RFQ.

type rfqExtractResult struct {
	CustomerName            string              `json:"customer_name"`
	CustomerContactPerson   string              `json:"customer_contact_person"`
	CustomerPhone           string              `json:"customer_phone"`
	CustomerEmail           string              `json:"customer_email"`
	CustomerCompany         string              `json:"customer_company"`
	CustomerVATNo           string              `json:"customer_vat_no"`
	CustomerCRNo            string              `json:"customer_cr_no"`
	CustomerNationalAddress string              `json:"customer_national_address"`
	Products                []models.RFQProduct `json:"products"`
	Categories              []string            `json:"product_categories,omitempty"`
	GeneralInstructions     string              `json:"general_instructions"`
	TextContent             string              `json:"text_content"`
	LLMModel                string              `json:"llm_model,omitempty"`
}

func ExtractRFQFromFilesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, `{"error":"failed to parse uploaded files (max 50 MB)"}`, http.StatusBadRequest)
		return
	}

	var textParts []string
	var imageDataURIs []string // "data:<mime>;base64,<data>"
	var pdfBase64s []string    // raw base64, no data URI prefix

	// Accept an optional plain-text content field (text_content) so the endpoint
	// can also extract products from a typed message without any file upload.
	if textFromField := r.FormValue("text_content"); textFromField != "" {
		textParts = append(textParts, textFromField)
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 && len(textParts) == 0 {
		http.Error(w, `{"error":"no files uploaded and no text_content provided"}`, http.StatusBadRequest)
		return
	}

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(f)
		f.Close()

		ext := strings.ToLower(filepath.Ext(fh.Filename))
		ct := strings.ToLower(fh.Header.Get("Content-Type"))

		switch {
		case ext == ".xlsx" || ext == ".xls" ||
			strings.Contains(ct, "spreadsheet") || strings.Contains(ct, "excel"):
			if text, err := excelToText(fh.Filename, data); err == nil {
				textParts = append(textParts, text)
			}

		case ext == ".docx" || strings.Contains(ct, "wordprocessingml"):
			if text, err := docxToText(fh.Filename, data); err == nil {
				textParts = append(textParts, text)
			}

		case ext == ".pdf" || strings.Contains(ct, "pdf"):
			pdfBase64s = append(pdfBase64s, base64.StdEncoding.EncodeToString(data))
			// Also extract text so providers that don't support PDF binary (e.g. OpenAI) can still read the content
			if extracted := extractPDFText(data); extracted != "" {
				textParts = append(textParts, "=== "+fh.Filename+" (PDF text) ===\n"+extracted)
			}

		case ext == ".csv" || ext == ".txt" || strings.HasPrefix(ct, "text/"):
			textParts = append(textParts, "=== "+fh.Filename+" ===\n"+string(data))

		case isRFQImageExt(ext) || strings.HasPrefix(ct, "image/"):
			mime := rfqImageMime(ext, ct)
			b64 := base64.StdEncoding.EncodeToString(data)
			imageDataURIs = append(imageDataURIs, "data:"+mime+";base64,"+b64)
		}
	}

	combinedText := strings.Join(textParts, "\n\n")
	if len(imageDataURIs)+len(pdfBase64s) == 0 && combinedText == "" {
		http.Error(w, `{"error":"no readable content found in the uploaded files"}`, http.StatusBadRequest)
		return
	}

	// Caller may specify llm_provider + llm_model to override the store-level default.
	// The API key is always read from store settings (per-provider key, or fallback to RFQLLMAPIKey).
	reqProvider := strings.ToLower(r.FormValue("llm_provider"))
	reqModel := r.FormValue("llm_model")

	provider := reqProvider
	if provider == "" {
		provider = strings.ToLower(store.Settings.RFQLLMProvider)
	}

	usedModel := reqModel
	if usedModel == "" {
		usedModel = store.Settings.RFQLLMModel
	}
	if usedModel == "" {
		usedModel = provider
	}

	// Resolve API key and endpoint URL for the selected provider.
	apiKey, endpointURL := resolveExtractionEndpoint(provider, &store.Settings)
	if apiKey == "" {
		http.Error(w, `{"error":"No API key configured for the selected provider. Please add it under Store → AI Models."}`, http.StatusBadRequest)
		return
	}

	responseText, llmErr := callLLMExtractRFQ(
		apiKey,
		usedModel,
		provider,
		combinedText,
		imageDataURIs,
		pdfBase64s,
		endpointURL,
	)
	if llmErr != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, llmErr.Error()), http.StatusInternalServerError)
		return
	}

	jsonStr := extractJSONFromLLMResponse(responseText)
	var result rfqExtractResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// Return the stripped text (think blocks removed) so the frontend can show it
		out, _ := json.Marshal(rfqExtractResult{TextContent: jsonStr, LLMModel: usedModel})
		w.Write(out)
		return
	}
	result.LLMModel = usedModel
	out, _ := json.Marshal(result)
	w.Write(out)
}

// resolveExtractionEndpoint returns the API key and the correct base URL for a provider.
// For Cloudflare Workers AI the URL embeds the account ID; for all other providers it
// delegates to openAICompatBaseURL. baseURL is empty when the provider has its own
// dispatch path (gemini, anthropic) — callLLMExtractRFQ handles those separately.
func resolveExtractionEndpoint(provider string, s *models.StoreSettings) (apiKey, baseURL string) {
	apiKey = resolveExtractionAPIKey(provider, s)
	switch provider {
	case "gemini", "anthropic":
		// These are handled by dedicated code paths; no base URL needed.
	case "cloudflare":
		if s.ExtractionCloudflareAccountID == "" {
			// Signal missing config by returning a sentinel base URL that will
			// cause callLLMExtractRFQ to skip the openAICompatBaseURL fallback and
			// instead error clearly. We use a non-empty string so callers can detect
			// the misconfiguration before making a network call.
			baseURL = "__cloudflare_missing_account_id__"
		} else {
			baseURL = "https://api.cloudflare.com/client/v4/accounts/" + s.ExtractionCloudflareAccountID + "/ai/v1/chat/completions"
		}
	default:
		baseURL = openAICompatBaseURL(provider)
	}
	return
}

// resolveExtractionAPIKey returns the API key for a given provider from store settings.
// Falls back to the legacy RFQLLMAPIKey when no per-provider key is set.
func resolveExtractionAPIKey(provider string, s *models.StoreSettings) string {
	switch provider {
	case "openai":
		if s.ExtractionOpenAIAPIKey != "" {
			return s.ExtractionOpenAIAPIKey
		}
	case "anthropic":
		if s.ExtractionAnthropicAPIKey != "" {
			return s.ExtractionAnthropicAPIKey
		}
	case "gemini":
		if s.ExtractionGeminiAPIKey != "" {
			return s.ExtractionGeminiAPIKey
		}
	case "groq":
		if s.ExtractionGroqAPIKey != "" {
			return s.ExtractionGroqAPIKey
		}
	case "xai":
		if s.ExtractionXAIAPIKey != "" {
			return s.ExtractionXAIAPIKey
		}
	case "mistral":
		if s.ExtractionMistralAPIKey != "" {
			return s.ExtractionMistralAPIKey
		}
	case "cerebras":
		if s.ExtractionCerebrasAPIKey != "" {
			return s.ExtractionCerebrasAPIKey
		}
	case "together":
		if s.ExtractionTogetherAPIKey != "" {
			return s.ExtractionTogetherAPIKey
		}
	case "openrouter":
		if s.ExtractionOpenRouterAPIKey != "" {
			return s.ExtractionOpenRouterAPIKey
		}
	case "sambanova":
		if s.ExtractionSambanovaAPIKey != "" {
			return s.ExtractionSambanovaAPIKey
		}
	case "fireworks":
		if s.ExtractionFireworksAPIKey != "" {
			return s.ExtractionFireworksAPIKey
		}
	case "nvidia":
		if s.ExtractionNvidiaAPIKey != "" {
			return s.ExtractionNvidiaAPIKey
		}
	case "github":
		if s.ExtractionGitHubAPIKey != "" {
			return s.ExtractionGitHubAPIKey
		}
	case "huggingface":
		if s.ExtractionHuggingFaceAPIKey != "" {
			return s.ExtractionHuggingFaceAPIKey
		}
	case "cloudflare":
		if s.ExtractionCloudflareAPIKey != "" {
			return s.ExtractionCloudflareAPIKey
		}
	case "cohere":
		if s.ExtractionCohereAPIKey != "" {
			return s.ExtractionCohereAPIKey
		}
	case "perplexity":
		if s.ExtractionPerplexityAPIKey != "" {
			return s.ExtractionPerplexityAPIKey
		}
	case "deepinfra":
		if s.ExtractionDeepInfraAPIKey != "" {
			return s.ExtractionDeepInfraAPIKey
		}
	}
	// Legacy fallback
	return s.RFQLLMAPIKey
}

// htmlToPlainText strips HTML/CSS markup and returns clean readable text.
// It removes <style> and <script> blocks entirely, then strips all remaining
// HTML tags, and finally normalises whitespace.
func htmlToPlainText(s string) string {
	// Remove <style>…</style> blocks (including inline CSS from email clients).
	styleRe := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	s = styleRe.ReplaceAllString(s, " ")
	// Remove <script>…</script> blocks.
	scriptRe := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	s = scriptRe.ReplaceAllString(s, " ")
	// Convert block-level tags to newlines before stripping.
	blockRe := regexp.MustCompile(`(?i)<(?:br|p|div|tr|li|h[1-6])[^>]*>`)
	s = blockRe.ReplaceAllString(s, "\n")
	// Strip remaining tags.
	tagRe := regexp.MustCompile(`<[^>]+>`)
	s = tagRe.ReplaceAllString(s, " ")
	// Decode common HTML entities.
	s = strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&nbsp;", " ", "&quot;", "\"", "&#39;", "'",
	).Replace(s)
	// Collapse whitespace / empty lines.
	lines := strings.Split(s, "\n")
	var kept []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// isJunkCell returns true for cell values that carry no useful information
// (artifacts from buggy JavaScript export tools, etc.).
func isJunkCell(s string) bool {
	switch strings.TrimSpace(s) {
	case "[object Object]", "undefined", "null", "NaN", "":
		return true
	}
	return false
}

// excelToText converts an Excel workbook to a plain-text markdown-style table
// suitable for LLM consumption. It uses the first non-empty row as headers when
// possible and formats data rows as "Header: Value" pairs. Junk cell values
// (e.g. "[object Object]") are silently dropped. Output is capped at 12 000 chars.
func excelToText(filename string, data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer f.Close()
	const maxChars = 12000
	const maxRowsPerSheet = 300
	var sb strings.Builder
	sb.WriteString("=== " + filename + " ===\n")
	sheets := f.GetSheetList()
	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		if len(sheets) > 1 {
			sb.WriteString("[Sheet: " + sheet + "]\n")
		}

		// Find header row: first row that has at least one non-junk cell.
		var headers []string
		dataStart := 0
		for i, row := range rows {
			clean := make([]string, len(row))
			hasContent := false
			for j, cell := range row {
				if !isJunkCell(cell) {
					clean[j] = strings.TrimSpace(cell)
					hasContent = true
				}
			}
			if hasContent {
				headers = clean
				dataStart = i + 1
				break
			}
		}

		// Write headers as a markdown table header row.
		if len(headers) > 0 {
			sb.WriteString("| " + strings.Join(headers, " | ") + " |\n")
			sep := make([]string, len(headers))
			for i := range sep {
				sep[i] = "---"
			}
			sb.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		}

		truncated := false
		for i := dataStart; i < len(rows); i++ {
			if i-dataStart >= maxRowsPerSheet {
				truncated = true
				break
			}
			row := rows[i]
			// Skip rows where every cell is junk.
			allJunk := true
			for _, cell := range row {
				if !isJunkCell(cell) {
					allJunk = false
					break
				}
			}
			if allJunk {
				continue
			}
			// Pad row to header length.
			for len(row) < len(headers) {
				row = append(row, "")
			}
			cells := make([]string, len(headers))
			for j := 0; j < len(headers); j++ {
				if j < len(row) && !isJunkCell(row[j]) {
					cells[j] = strings.TrimSpace(row[j])
				}
			}
			sb.WriteString("| " + strings.Join(cells, " | ") + " |\n")
			if sb.Len() >= maxChars {
				truncated = true
				break
			}
		}
		if truncated {
			sb.WriteString("[... truncated for length ...]\n")
		}
		if sb.Len() >= maxChars {
			break
		}
	}
	return sb.String(), nil
}

// docxToText extracts plain text from a .docx file (an OOXML ZIP archive).
// It reads word/document.xml, walks the XML token stream, and collects text
// from <w:t> elements while inserting newlines at <w:p> paragraph boundaries.
func docxToText(filename string, data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx: not a valid zip archive: %w", err)
	}
	for _, zf := range r.File {
		if zf.Name != "word/document.xml" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return "", err
		}
		xmlData, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString("=== " + filename + " ===\n")
		dec := xml.NewDecoder(bytes.NewReader(xmlData))
		inText := false
		for {
			tok, tokErr := dec.Token()
			if tokErr != nil {
				break
			}
			switch v := tok.(type) {
			case xml.StartElement:
				switch v.Name.Local {
				case "p": // paragraph → newline
					sb.WriteString("\n")
				case "t": // <w:t> text run
					inText = true
				}
			case xml.EndElement:
				if v.Name.Local == "t" {
					inText = false
				}
			case xml.CharData:
				if inText {
					sb.WriteString(string(v))
				}
			}
		}
		text := strings.TrimSpace(sb.String())
		if text == "" {
			return "", fmt.Errorf("docx: no text content found")
		}
		return text, nil
	}
	return "", fmt.Errorf("docx: word/document.xml not found in archive")
}

func isRFQImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

func rfqImageMime(ext, ct string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	if strings.Contains(ct, "image/") {
		return ct
	}
	return "image/jpeg"
}

func buildRFQExtractionPrompt(textContent string) string {
	prompt := `You are an assistant that extracts RFQ (Request for Quotation) information from documents, images, or spreadsheets.

Extract the following and respond with ONLY valid JSON (no markdown, no explanation):
{
  "customer_name": "company or organization name — PREFERRED over person name. If only a person's name is available and no company is found, use the person's name here",
  "customer_contact_person": "name of the individual person who sent or signed the RFQ (leave empty string if only a company name was found with no individual)",
  "customer_phone": "phone number (international format if possible)",
  "customer_email": "email address",
  "customer_company": "company or organization name (same as customer_name when a company is identified)",
  "customer_vat_no": "VAT registration number or tax ID if present (empty string if not found)",
  "customer_cr_no": "Commercial Registration (CR) number if present (empty string if not found)",
  "customer_national_address": "full national/postal address as a single string if present (empty string if not found)",
  "products": [
    {
      "part_no": "part number or product code (empty string if not found)",
      "name": "product name or description",
      "quantity": 1,
      "unit_price": 0,
      "unit": "unit of measure e.g. EA, PCS, KG, M (empty string if not found)",
      "notes": "technical specifications for THIS specific product only — e.g. voltage, power, pressure, flow rate, temperature, dimensions, model compliance requirements (empty string if none)"
    }
  ],
  "general_instructions": "instructions that apply to the whole RFQ, not to a specific product — e.g. 'provide technical datasheet, unit price, warranty, delivery lead time, delivery terms, availability'. Also include conditions like 'if exact model unavailable propose nearest equivalent'. Leave empty string if none.",
  "product_categories": ["broad trade category 1", "broad trade category 2"],
  "text_content": "a clean plain-text summary of the full enquiry"
}

Rules:
- If a field is not found, use an empty string or 0 for quantity.
- IMPORTANT: The CUSTOMER is the person/company who SENT this email or document — they are requesting the quotation. Use the "From:" header (email address / sender name) as the primary source for customer identity. Do NOT use the recipient's company name (the company receiving the RFQ) as the customer.
- customer_name should be the COMPANY name whenever one is present. Only fall back to a person's name when no company is identifiable.
- customer_contact_person is the individual's name (different from the company name).
- Extract ALL products/items mentioned; do not skip any.
- quantity must be a number (default to 1 if not stated).
- unit_price must be a number (0 if not found); extract the per-unit price excluding VAT when both are shown.
- IMPORTANT: product notes = only technical specs tied to that product (power, pressure, dimensions, etc.). General requests (datasheet, warranty, delivery terms, equivalent model clause) go in general_instructions, NOT in product notes.
- product_categories: identify 1-5 broad product trade categories the items belong to (e.g. "Valves", "Pipe Fittings", "Electrical Equipment", "Steel Pipes"). Use [] if not determinable.
- Do NOT wrap in markdown code blocks.`

	if textContent != "" {
		prompt += "\n\nDocument text content:\n" + textContent
	}
	return prompt
}

// openAICompatBaseURL returns the chat completions base URL for OpenAI-compatible providers.
func openAICompatBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1/chat/completions"
	case "openrouter":
		return "https://openrouter.ai/api/v1/chat/completions"
	case "groq":
		return "https://api.groq.com/openai/v1/chat/completions"
	case "mistral":
		return "https://api.mistral.ai/v1/chat/completions"
	case "together":
		return "https://api.together.xyz/v1/chat/completions"
	case "cerebras":
		return "https://api.cerebras.ai/v1/chat/completions"
	case "sambanova":
		return "https://api.sambanova.ai/v1/chat/completions"
	case "fireworks":
		return "https://api.fireworks.ai/inference/v1/chat/completions"
	case "nvidia":
		return "https://integrate.api.nvidia.com/v1/chat/completions"
	case "github":
		return "https://models.inference.ai.azure.com/chat/completions"
	case "xai":
		return "https://api.x.ai/v1/chat/completions"
	case "huggingface":
		return "https://api-inference.huggingface.co/v1/chat/completions"
	case "cohere":
		return "https://api.cohere.com/compatibility/v1/chat/completions"
	case "perplexity":
		return "https://api.perplexity.ai/chat/completions"
	case "deepinfra":
		return "https://api.deepinfra.com/v1/openai/chat/completions"
	default:
		return "https://api.openai.com/v1/chat/completions"
	}
}

// callLLMExtractRFQ calls the configured LLM with vision+text capabilities and
// returns the raw LLM text (expected to be JSON).
// baseURL overrides the endpoint for OpenAI-compatible providers (used for Cloudflare
// which requires an account-ID in the URL); pass "" to use the provider default.
func callLLMExtractRFQ(apiKey, model, provider, textContent string, imageDataURIs, pdfBase64s []string, baseURL string) (string, error) {
	prompt := buildRFQExtractionPrompt(textContent)
	const maxTokens = 16000

	switch provider {
	case "gemini":
		return callGeminiExtractRFQ(apiKey, model, prompt, imageDataURIs, pdfBase64s, maxTokens)
	case "anthropic":
		if model == "" {
			model = "claude-haiku-4-5-20251001"
		}
		return callAnthropicExtractRFQ(apiKey, model, prompt, imageDataURIs, pdfBase64s, maxTokens)
	default: // openai and all openai-compatible providers
		if baseURL == "" {
			baseURL = openAICompatBaseURL(provider)
		}
		return callOpenAICompatExtractRFQ(apiKey, model, prompt, imageDataURIs, maxTokens, baseURL)
	}
}

func callOpenAICompatExtractRFQ(apiKey, model, prompt string, imageDataURIs []string, maxTokens int, baseURL string) (string, error) {
	if model == "" {
		model = "gpt-4o-mini"
	}
	if baseURL == "__cloudflare_missing_account_id__" {
		return "", fmt.Errorf("cloudflare: Cloudflare Account ID is not configured. Add it under Store → AI Models → Cloudflare Account ID")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1/chat/completions"
	}
	// Derive a short provider label from the base URL for error messages.
	providerLabel := "openai"
	for _, kv := range []struct{ host, label string }{
		{"openrouter.ai", "openrouter"},
		{"groq.com", "groq"},
		{"mistral.ai", "mistral"},
		{"together.xyz", "together"},
		{"cerebras.ai", "cerebras"},
		{"sambanova.ai", "sambanova"},
		{"fireworks.ai", "fireworks"},
		{"nvidia.com", "nvidia"},
		{"inference.ai.azure.com", "github"},
		{"x.ai", "xai"},
		{"huggingface.co", "huggingface"},
		{"cohere.com", "cohere"},
		{"perplexity.ai", "perplexity"},
		{"deepinfra.com", "deepinfra"},
	} {
		if strings.Contains(baseURL, kv.host) {
			providerLabel = kv.label
			break
		}
	}
	type contentPart struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	var parts []contentPart
	parts = append(parts, contentPart{Type: "text", Text: prompt})
	for _, uri := range imageDataURIs {
		parts = append(parts, contentPart{Type: "image_url", ImageURL: &struct{ URL string `json:"url"` }{URL: uri}})
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"messages":   []map[string]interface{}{{"role": "user", "content": parts}},
		"max_tokens": maxTokens,
	})
	req, _ := http.NewRequest("POST", baseURL, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 300 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Choices []struct {
			Message struct{ Content string `json:"content"` } `json:"message"`
		} `json:"choices"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("%s parse error: %s", providerLabel, string(body))
	}
	if r.Error != nil {
		return "", fmt.Errorf("%s: %s", providerLabel, r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("%s: empty response", providerLabel)
	}
	return r.Choices[0].Message.Content, nil
}

func callAnthropicExtractRFQ(apiKey, model, prompt string, imageDataURIs, pdfBase64s []string, maxTokens int) (string, error) {
	type blockSource struct {
		Type      string `json:"type"`
		MediaType string `json:"media_type"`
		Data      string `json:"data"`
	}
	type block struct {
		Type   string       `json:"type"`
		Text   string       `json:"text,omitempty"`
		Source *blockSource `json:"source,omitempty"`
	}
	var blocks []block
	for _, uri := range imageDataURIs {
		mime, raw := splitDataURI(uri)
		blocks = append(blocks, block{Type: "image", Source: &blockSource{Type: "base64", MediaType: mime, Data: raw}})
	}
	for _, b64 := range pdfBase64s {
		blocks = append(blocks, block{Type: "document", Source: &blockSource{Type: "base64", MediaType: "application/pdf", Data: b64}})
	}
	blocks = append(blocks, block{Type: "text", Text: prompt})

	payload, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   []map[string]interface{}{{"role": "user", "content": blocks}},
	})
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(payload))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "pdfs-2024-09-25")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 300 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("anthropic parse error: %s", string(body))
	}
	if r.Error != nil {
		return "", fmt.Errorf("anthropic: %s", r.Error.Message)
	}
	if len(r.Content) == 0 {
		return "", fmt.Errorf("anthropic: empty response")
	}
	return r.Content[0].Text, nil
}

func callGeminiExtractRFQ(apiKey, model, prompt string, imageDataURIs, pdfBase64s []string, maxTokens int) (string, error) {
	if model == "" {
		model = "gemini-3.6-flash"
	}
	type inlinePart struct {
		Text       string `json:"text,omitempty"`
		InlineData *struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"inlineData,omitempty"`
	}
	var parts []inlinePart
	parts = append(parts, inlinePart{Text: prompt})
	for _, uri := range imageDataURIs {
		mime, raw := splitDataURI(uri)
		parts = append(parts, inlinePart{InlineData: &struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		}{MimeType: mime, Data: raw}})
	}
	for _, b64 := range pdfBase64s {
		parts = append(parts, inlinePart{InlineData: &struct {
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		}{MimeType: "application/pdf", Data: b64}})
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"contents":         []map[string]interface{}{{"parts": parts}},
		"generationConfig": map[string]interface{}{"maxOutputTokens": maxTokens},
	})
	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, _ := http.NewRequest("POST", apiURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 300 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string `json:"text"` } `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", fmt.Errorf("gemini parse error: %s", string(body))
	}
	if r.Error != nil {
		return "", fmt.Errorf("gemini: %s", r.Error.Message)
	}
	if len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty response")
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}

// DeleteAllRFQReceivedHandler handles DELETE /v1/rfq-received?store_id=...
// Hard-deletes all RFQ received records for a store (admin-only gate enforced in frontend).
func DeleteAllRFQReceivedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	deleted, err := models.DeleteAllRFQReceived(storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": deleted})
}

// DeleteRFQReceivedHandler handles DELETE /v1/rfq-received/{id}?store_id=...
// Hard-deletes a single RFQ and unlinks it from the connected procurement message.
func DeleteRFQReceivedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	rfq, err := models.DeleteRFQReceived(storeObjID, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	// Unlink from the procurement message that originated this RFQ (if any).
	if rfq.ProcurementMessageID != nil {
		_ = models.UnlinkRFQFromProcurementMessage(*rfq.ProcurementMessageID)
	}
	json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
}

// ── RefetchSupplierMapsHandler ─────────────────────────────────────────────
// POST /v1/rfq-suppliers/{id}/refetch-maps
// Searches Google Maps by the supplier's name and updates address, rating,
// website, maps URL, lat/lng in the stored record.

func RefetchSupplierMapsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	if store.Settings.GoogleMapsAPIKey == "" {
		http.Error(w, `{"error":"Google Maps API key not configured in Store Settings"}`, http.StatusBadRequest)
		return
	}
	sup, err := models.FindRFQSupplierByID(id, storeObjID)
	if err != nil || sup == nil {
		http.Error(w, `{"error":"supplier not found"}`, http.StatusNotFound)
		return
	}

	// Build list of markets to try: supplier's own market first, then all store markets,
	// then a blank (no market constraint) as final fallback.
	markets := []string{}
	if sup.PurchaseMarket != "" {
		markets = append(markets, sup.PurchaseMarket)
	}
	for _, m := range store.Settings.PurchaseMarkets {
		if m != sup.PurchaseMarket {
			markets = append(markets, m)
		}
	}
	markets = append(markets, "") // fallback: no location constraint

	var enriched bool
	var enrichErr error
	for _, market := range markets {
		sup.PurchaseMarket = market
		enriched, enrichErr = enrichSupplierFromGoogleMaps(store.Settings.GoogleMapsAPIKey, sup, store.Settings.PurchaseMarkets...)
		if enrichErr != nil {
			break
		}
		if enriched {
			break
		}
	}
	if enrichErr != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, enrichErr.Error()), http.StatusInternalServerError)
		return
	}
	if !enriched {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "no_match", "message": "No Google Maps result found for this supplier"})
		return
	}
	// Use UpdateRFQSupplier (matches by _id) to avoid creating a duplicate when the
	// existing record had no google_place_id (UpsertRFQSupplierByPlaceID would insert
	// a new doc in that case because the place-id filter finds nothing).
	if err := models.UpdateRFQSupplier(sup); err != nil {
		http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
		return
	}
	// If still no categories but a website is now known, infer them from the website.
	if len(sup.Categories) == 0 && sup.Website != "" {
		go inferSupplierCategoriesFromWebsite(store, *sup)
	}
	BroadcastRFQEvent(storeIDStr, "supplier_updated")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "supplier": sup})
}

// enrichSupplierFromGoogleMaps searches Google Maps by the supplier's name,
// takes the best match, and mutates the supplied record with address/rating/website/maps URL.
// If purchaseMarkets is non-empty, also sets PurchaseMarket by matching the city from address components.
// Returns true if a match was found.
func enrichSupplierFromGoogleMaps(apiKey string, sup *models.RFQSupplier, purchaseMarkets ...string) (bool, error) {
	const endpoint = "https://places.googleapis.com/v1/places:searchText"
	const fieldMask = "places.id,places.displayName,places.formattedAddress,places.addressComponents,places.rating,places.location,places.internationalPhoneNumber,places.websiteUri,places.primaryType,places.types"

	query := sup.Name
	if sup.PurchaseMarket != "" {
		query += " in " + sup.PurchaseMarket
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"textQuery":      query,
		"maxResultCount": 5,
	})
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", apiKey)
	req.Header.Set("X-Goog-FieldMask", fieldMask)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("Google Maps API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Places []struct {
			ID                 string  `json:"id"`
			FormattedAddress   string  `json:"formattedAddress"`
			Rating             float64 `json:"rating"`
			InternationalPhone string  `json:"internationalPhoneNumber"`
			WebsiteUri         string  `json:"websiteUri"`
			DisplayName        struct {
				Text string `json:"text"`
			} `json:"displayName"`
			Location struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
			AddressComponents []struct {
				LongText  string   `json:"longText"`
				ShortText string   `json:"shortText"`
				Types     []string `json:"types"`
			} `json:"addressComponents"`
		} `json:"places"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("parse error: %v", err)
	}
	if len(result.Places) == 0 {
		return false, nil
	}

	// Prefer a result whose phone matches the stored supplier phone; otherwise take first.
	// Among results with no phone match, prefer one that has both rating and website.
	best := result.Places[0]
	for _, p := range result.Places {
		if digitsOnly(p.InternationalPhone) == sup.Phone {
			best = p
			break
		}
		// Prefer first result that has rating or website over one that has neither.
		if (p.Rating > 0 || p.WebsiteUri != "") && best.Rating == 0 && best.WebsiteUri == "" {
			best = p
		}
	}

	log.Printf("enrichSupplierFromGoogleMaps: supplier=%q query=%q matched=%q phone=%q rating=%.1f website=%q address=%q",
		sup.Name, query, best.DisplayName.Text, best.InternationalPhone, best.Rating, best.WebsiteUri, best.FormattedAddress)

	mapsURL := ""
	if best.ID != "" {
		mapsURL = "https://www.google.com/maps/place/?q=place_id:" + best.ID
	}
	sup.GooglePlaceID = best.ID
	sup.GoogleMapsURL = mapsURL
	sup.Address = best.FormattedAddress
	sup.Rating = best.Rating
	sup.Latitude = best.Location.Latitude
	sup.Longitude = best.Location.Longitude
	if best.WebsiteUri != "" {
		sup.Website = best.WebsiteUri
	}
	// If the supplier's name was a phone/unknown placeholder, update with the Google name.
	if best.DisplayName.Text != "" && sup.Name == "" {
		sup.Name = best.DisplayName.Text
	}

	// Extract city from address components and match against configured purchase markets.
	if sup.PurchaseMarket == "" && len(purchaseMarkets) > 0 {
		// Extract city: locality > sublocality > administrative_area_level_2
		cityFromAddr := ""
		for _, priority := range []string{"locality", "sublocality_level_1", "sublocality", "administrative_area_level_2"} {
			for _, ac := range best.AddressComponents {
				for _, t := range ac.Types {
					if t == priority && ac.LongText != "" {
						cityFromAddr = ac.LongText
					}
				}
				if cityFromAddr != "" {
					break
				}
			}
			if cityFromAddr != "" {
				break
			}
		}
		if cityFromAddr != "" {
			for _, m := range purchaseMarkets {
				if strings.EqualFold(m, cityFromAddr) {
					sup.PurchaseMarket = m
					break
				}
				// fuzzy: market name contained in city or vice versa
				if strings.Contains(strings.ToLower(cityFromAddr), strings.ToLower(m)) ||
					strings.Contains(strings.ToLower(m), strings.ToLower(cityFromAddr)) {
					sup.PurchaseMarket = m
					break
				}
			}
		}
		log.Printf("enrichSupplierFromGoogleMaps: city=%q market=%q", cityFromAddr, sup.PurchaseMarket)
	}

	return true, nil
}

// POST /v1/rfq-suppliers/backfill-markets?store_id=...
// Goes through all suppliers with no purchase_market and tries to set it from Google Maps.
func BackfillSupplierMarketsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}
	if store.Settings.GoogleMapsAPIKey == "" {
		http.Error(w, `{"error":"Google Maps API key not configured"}`, http.StatusBadRequest)
		return
	}

	// Load all suppliers for this store with no purchase_market
	result, err := models.ListRFQSuppliers(storeObjID, 1, 500, "", nil)
	all := []models.RFQSupplier{}
	if result != nil {
		all = result.Items
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	updated, skipped, failed := 0, 0, 0
	for i := range all {
		sup := &all[i]
		if sup.PurchaseMarket != "" {
			skipped++
			continue
		}
		if sup.Name == "" {
			skipped++
			continue
		}
		// Mirror the G button: try each configured market, then fall back to no-location search.
		markets := append(append([]string{}, store.Settings.PurchaseMarkets...), "")
		var enriched bool
		var eErr error
		origMarket := sup.PurchaseMarket
		for _, market := range markets {
			sup.PurchaseMarket = market
			enriched, eErr = enrichSupplierFromGoogleMaps(store.Settings.GoogleMapsAPIKey, sup)
			if eErr != nil || enriched {
				break
			}
		}
		if !enriched {
			sup.PurchaseMarket = origMarket
		}
		if eErr != nil {
			log.Printf("backfill-markets: enrich error for %s: %v", sup.Phone, eErr)
			failed++
			continue
		}
		if enriched && sup.PurchaseMarket != "" {
			if uErr := models.UpdateRFQSupplier(sup); uErr != nil {
				log.Printf("backfill-markets: save error for %s: %v", sup.Phone, uErr)
				failed++
			} else {
				updated++
				log.Printf("backfill-markets: set market=%q for %s (%s)", sup.PurchaseMarket, sup.Name, sup.Phone)
			}
		} else {
			skipped++
		}
	}
	BroadcastRFQEvent(storeObjID.Hex(), "supplier_updated")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"updated": updated,
		"skipped": skipped,
		"failed":  failed,
	})
}
