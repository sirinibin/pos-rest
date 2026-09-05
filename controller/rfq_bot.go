package controller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// rfqEvoConfig reads bot OR store-rfq evolution settings from a store.
// role = "bot" | "store_rfq"
func rfqEvoConfig(storeIDStr, role string) (evoURL, evoKey, instanceName string) {
	evoURL, evoKey, instanceName = evoDefaultURL, evoGlobalKey, ""
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
	if role == "bot" {
		if store.Settings.BotEvolutionAPIURL != "" {
			evoURL = store.Settings.BotEvolutionAPIURL
		}
		if store.Settings.BotEvolutionAPIKey != "" {
			evoKey = store.Settings.BotEvolutionAPIKey
		}
		instanceName = store.Settings.BotEvolutionInstanceName
	} else {
		if store.Settings.StoreRFQEvolutionAPIURL != "" {
			evoURL = store.Settings.StoreRFQEvolutionAPIURL
		}
		if store.Settings.StoreRFQEvolutionAPIKey != "" {
			evoKey = store.Settings.StoreRFQEvolutionAPIKey
		}
		instanceName = store.Settings.StoreRFQEvolutionInstanceName
	}
	return
}

func rfqSaveWhatsAppSettings(storeID primitive.ObjectID, role, evoURL, instanceName, token string) error {
	var upd bson.M
	if role == "bot" {
		upd = bson.M{
			"settings.bot_evolution_api_url":       evoURL,
			"settings.bot_evolution_instance_name": instanceName,
			"settings.bot_evolution_api_key":       token,
		}
	} else {
		upd = bson.M{
			"settings.store_rfq_evolution_api_url":       evoURL,
			"settings.store_rfq_evolution_instance_name": instanceName,
			"settings.store_rfq_evolution_api_key":       token,
		}
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx, bson.M{"_id": storeID}, bson.M{"$set": upd})
	return err
}

func rfqClearWhatsAppSettings(storeID primitive.ObjectID, role string) error {
	var upd bson.M
	if role == "bot" {
		upd = bson.M{
			"settings.bot_evolution_api_url":       "",
			"settings.bot_evolution_instance_name": "",
			"settings.bot_evolution_api_key":       "",
		}
	} else {
		upd = bson.M{
			"settings.store_rfq_evolution_api_url":       "",
			"settings.store_rfq_evolution_instance_name": "",
			"settings.store_rfq_evolution_api_key":       "",
		}
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := col.UpdateOne(ctx, bson.M{"_id": storeID}, bson.M{"$set": upd})
	return err
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

// connectRFQWhatsApp is shared by ConnectBotWhatsApp and ConnectStoreRFQWhatsApp.
// role = "bot" | "store_rfq"; instancePrefix = "rfqbot" | "rfqsend"
func connectRFQWhatsApp(w http.ResponseWriter, r *http.Request, role, instancePrefix string) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		StoreID string `json:"store_id"`
		Phone   string `json:"phone"`
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
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, `{"error":"store not found"}`, http.StatusNotFound)
		return
	}

	// Save the phone number to settings
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	phoneField := "settings.bot_whatsapp_phone"
	if role == "store_rfq" {
		phoneField = "settings.store_rfq_whatsapp_phone"
	}
	ctx0, cancel0 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel0()
	col.UpdateOne(ctx0, bson.M{"_id": storeObjID}, bson.M{"$set": bson.M{phoneField: body.Phone}})

	evoURL := evoDefaultURL
	if store.Settings.EvolutionAPIURL != "" {
		evoURL = store.Settings.EvolutionAPIURL
	}

	// Build instance name: rfqbot_<code> or rfqsend_<code>
	code := strings.ToLower(store.Code)
	code = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, code)
	instanceName := instancePrefix + "_" + code

	// Delete any previous instance silently
	evoCall("DELETE", fmt.Sprintf("%s/instance/delete/%s", evoURL, instanceName), evoGlobalKey, nil)

	// Build create payload; add webhook only for bot role
	createMap := map[string]interface{}{
		"instanceName": instanceName,
		"integration":  "WHATSAPP-BAILEYS",
	}
	if role == "bot" {
		webhookURL := buildWebhookURL(r, body.StoreID)
		createMap["webhook"] = map[string]interface{}{
			"url":     webhookURL,
			"enabled": true,
			"events":  []string{"MESSAGES_UPSERT"},
		}
	}
	createPayload, _ := json.Marshal(createMap)

	respBody, status, err := evoCall("POST", fmt.Sprintf("%s/instance/create", evoURL), evoGlobalKey, createPayload)
	if err != nil || (status != 200 && status != 201) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"Evolution API create failed","detail":%s}`, string(respBody))
		return
	}

	var createResp struct {
		Hash string `json:"hash"`
	}
	json.Unmarshal(respBody, &createResp)
	token := createResp.Hash
	if token == "" {
		token = evoGlobalKey
	}

	if err := rfqSaveWhatsAppSettings(storeObjID, role, evoURL, instanceName, token); err != nil {
		http.Error(w, `{"error":"failed to save store settings"}`, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, `{"success":true,"instance_name":%q,"token":%q}`, instanceName, token)
}

// ── 1. Bot WhatsApp Connect / QR / Status / Disconnect ───────────────────────

// POST /v1/rfq-bot/connect
func ConnectBotWhatsApp(w http.ResponseWriter, r *http.Request) {
	connectRFQWhatsApp(w, r, "bot", "rfqbot")
}

// GET /v1/rfq-bot/qr?store_id=...
func GetBotWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	evoURL, evoKey, instanceName := rfqEvoConfig(storeIDStr, "bot")
	respBody, _, err := evoCall("GET",
		fmt.Sprintf("%s/instance/connect/%s", evoURL, instanceName), evoKey, nil)
	if err != nil {
		http.Error(w, `{"error":"Evolution API unreachable"}`, http.StatusBadGateway)
		return
	}
	w.Write(evoNormalizeQR(respBody))
}

// GET /v1/rfq-bot/status?store_id=...
func GetBotWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	rfqWhatsAppStatus(w, r, "bot")
}

// DELETE /v1/rfq-bot/disconnect?store_id=...
func DisconnectBotWhatsApp(w http.ResponseWriter, r *http.Request) {
	rfqDisconnect(w, r, "bot")
}

// ── 2. Store RFQ WhatsApp Connect / QR / Status / Disconnect ─────────────────

// POST /v1/rfq-store/connect
func ConnectStoreRFQWhatsApp(w http.ResponseWriter, r *http.Request) {
	connectRFQWhatsApp(w, r, "store_rfq", "rfqsend")
}

// GET /v1/rfq-store/qr?store_id=...
func GetStoreRFQWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	evoURL, evoKey, instanceName := rfqEvoConfig(storeIDStr, "store_rfq")
	respBody, _, err := evoCall("GET",
		fmt.Sprintf("%s/instance/connect/%s", evoURL, instanceName), evoKey, nil)
	if err != nil {
		http.Error(w, `{"error":"Evolution API unreachable"}`, http.StatusBadGateway)
		return
	}
	w.Write(evoNormalizeQR(respBody))
}

// GET /v1/rfq-store/status?store_id=...
func GetStoreRFQWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	rfqWhatsAppStatus(w, r, "store_rfq")
}

// DELETE /v1/rfq-store/disconnect?store_id=...
func DisconnectStoreRFQWhatsApp(w http.ResponseWriter, r *http.Request) {
	rfqDisconnect(w, r, "store_rfq")
}

func rfqWhatsAppStatus(w http.ResponseWriter, r *http.Request, role string) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	evoURL, evoKey, instanceName := rfqEvoConfig(storeIDStr, role)
	if instanceName == "" {
		fmt.Fprint(w, `{"connected":false}`)
		return
	}
	respBody, _, err := evoCall("GET", fmt.Sprintf("%s/instance/fetchInstances", evoURL), evoKey, nil)
	if err != nil {
		http.Error(w, `{"error":"Evolution API unreachable"}`, http.StatusBadGateway)
		return
	}
	// Accept both v2.2.x ("name") and v2.3.x ("instanceName") field names.
	var instances []struct {
		Name             string `json:"name"`
		InstanceName     string `json:"instanceName"`
		ConnectionStatus string `json:"connectionStatus"`
		OwnerJid         string `json:"ownerJid"`
	}
	if err := json.Unmarshal(respBody, &instances); err != nil {
		fmt.Fprint(w, `{"connected":false}`)
		return
	}
	for _, inst := range instances {
		instName := inst.Name
		if instName == "" {
			instName = inst.InstanceName
		}
		if instName == instanceName {
			connected := inst.ConnectionStatus == "open"
			phone := strings.TrimSuffix(inst.OwnerJid, "@s.whatsapp.net")
			fmt.Fprintf(w, `{"connected":%v,"phone":%q,"instance_name":%q,"status":%q}`,
				connected, phone, instanceName, inst.ConnectionStatus)
			return
		}
	}
	fmt.Fprintf(w, `{"connected":false,"instance_name":%q}`, instanceName)
}

func rfqDisconnect(w http.ResponseWriter, r *http.Request, role string) {
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
	evoURL, evoKey, instanceName := rfqEvoConfig(storeIDStr, role)
	if instanceName != "" {
		evoCall("DELETE", fmt.Sprintf("%s/instance/delete/%s", evoURL, instanceName), evoKey, nil)
	}
	if err := rfqClearWhatsAppSettings(storeObjID, role); err != nil {
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

// CheckWhatsAppNumber verifies whether a phone number has a WhatsApp account
// using the store's bot Evolution API instance.
// GET /v1/rfq-bot/check-whatsapp?store_id=...&phone=...
func CheckWhatsAppNumber(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	storeIDStr := r.URL.Query().Get("store_id")
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	if storeIDStr == "" || phone == "" {
		http.Error(w, `{"error":"store_id and phone are required"}`, http.StatusBadRequest)
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

	evoURL := store.Settings.BotEvolutionAPIURL
	if evoURL == "" {
		evoURL = evoDefaultURL
	}
	evoKey := store.Settings.BotEvolutionAPIKey
	if evoKey == "" {
		evoKey = evoGlobalKey
	}
	instance := store.Settings.BotEvolutionInstanceName
	if instance == "" {
		http.Error(w, `{"error":"Bot WhatsApp not connected"}`, http.StatusBadRequest)
		return
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"numbers": []string{phone},
	})
	respBody, status, err := evoCall("POST",
		fmt.Sprintf("%s/chat/whatsappNumbers/%s", strings.TrimRight(evoURL, "/"), instance),
		evoKey, payload)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadGateway)
		return
	}
	if status != 200 && status != 201 {
		http.Error(w, fmt.Sprintf(`{"error":"evolution API %d"}`, status), http.StatusBadGateway)
		return
	}

	// Evolution API returns: [{"exists":true,"jid":"...","number":"...","name":"..."}]
	var result []struct {
		Exists bool   `json:"exists"`
		Number string `json:"number"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || len(result) == 0 {
		// Fallback: return the raw response so the frontend knows
		w.Write([]byte(fmt.Sprintf(`{"exists":false,"raw":%q}`, string(respBody))))
		return
	}

	entry := result[0]
	if entry.Exists {
		fmt.Fprintf(w, `{"exists":true,"name":%q,"number":%q}`, entry.Name, entry.Number)
	} else {
		fmt.Fprintf(w, `{"exists":false}`)
	}
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

// ── 4. Webhook — incoming Evolution API messages ──────────────────────────────

// POST /v1/rfq-bot/webhook?store_id=...
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

	// Parse Evolution API webhook payload
	var payload evolutionWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("rfq_bot: webhook JSON parse error: %v body=%.300s", err, string(bodyBytes))
		return
	}

	log.Printf("rfq_bot: webhook received event=%q msgType=%q fromMe=%v jid=%q",
		payload.Event, payload.Data.MessageType, payload.Data.Key.FromMe, payload.Data.Key.RemoteJid)

	// Only process incoming (not fromMe) text/image messages
	if payload.Event != "messages.upsert" && payload.Event != "MESSAGES_UPSERT" {
		log.Printf("rfq_bot: webhook skipped — unhandled event %q", payload.Event)
		return
	}
	if payload.Data.Key.FromMe {
		return
	}

	fromJID := payload.Data.Key.RemoteJid
	fromPhone := strings.TrimSuffix(fromJID, "@s.whatsapp.net")
	if strings.Contains(fromJID, "@g.us") {
		log.Printf("rfq_bot: webhook skipped — group message from %s", fromJID)
		return
	}

	// Load store for allowed-sender check and supplier-reply detection
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		log.Printf("rfq_bot: webhook store load failed: %v", err)
		return
	}

	// If the sender is replying to one of the bot's relay messages, route as a follow-up
	// (buyer ↔ supplier conversation continuation) — never process as a new RFQ.
	if stanzaID := extractStanzaID(payload); stanzaID != "" {
		rfq, supplierPhone, err := models.FindRFQByBuyerRelayMsgID(storeObjID, stanzaID)
		if err == nil && supplierPhone != "" {
			go handleBuyerFollowup(store, rfq, supplierPhone, fromPhone, payload.Data.PushName, payload)
			return
		}
	}

	// Allowed senders whitelist: if non-empty, only those numbers are treated as buyers
	if len(store.Settings.RFQAllowedSenders) > 0 {
		allowed := false
		for _, s := range store.Settings.RFQAllowedSenders {
			if strings.TrimSpace(s) == fromPhone {
				allowed = true
				break
			}
		}
		if !allowed {
			// Not a known buyer — check if it's a supplier replying to a forwarded RFQ
			go handleSupplierReply(store, fromPhone, payload.Data.PushName, payload.Data.Message.Conversation, payload)
			return
		}
	}

	rfq := &models.RFQReceived{
		StoreID:    storeObjID,
		FromPhone:  fromPhone,
		FromName:   payload.Data.PushName,
		BuyerMsgID: payload.Data.Key.ID,
		Status:     "received",
	}

	// Skip unsupported message types silently (audio, sticker, reaction, etc.)
	msgType := strings.ToLower(payload.Data.MessageType)
	skipTypes := map[string]bool{
		"audiomessage": true, "pttmessage": true, "stickermessage": true,
		"reactionmessage": true, "contactmessage": true, "locationmessage": true,
		"livemessage": true, "pollcreationmessage": true,
	}
	if skipTypes[msgType] {
		log.Printf("rfq_bot: webhook skipped — unsupported msgType=%q from %s", msgType, fromPhone)
		return
	}

	// Extract text from all known message variants
	msg := payload.Data.Message
	text := ""
	switch {
	case msg.Conversation != "":
		text = msg.Conversation
	case msg.ExtendedTextMessage.Text != "":
		text = msg.ExtendedTextMessage.Text
	case msg.ImageMessage.Caption != "":
		text = msg.ImageMessage.Caption
	case msg.VideoMessage.Caption != "":
		text = msg.VideoMessage.Caption
	case msg.DocumentMessage.Caption != "":
		text = msg.DocumentMessage.Caption
	case msg.DocumentWithCaptionMessage.Message.DocumentMessage.Caption != "":
		text = msg.DocumentWithCaptionMessage.Message.DocumentMessage.Caption
	}
	rfq.TextContent = text

	// ── Decrypt all media (images + documents) via Evolution API ──────────────
	evoURL := store.Settings.BotEvolutionAPIURL
	if evoURL == "" {
		evoURL = evoDefaultURL
	}
	evoKey := store.Settings.BotEvolutionAPIKey
	if evoKey == "" {
		evoKey = evoGlobalKey
	}
	evoInstance := store.Settings.BotEvolutionInstanceName
	if evoInstance == "" {
		evoInstance = evoURL // placeholder — avoids empty string
	}

	// Parse the raw webhook envelope once so we can pass data.message to decrypt endpoints
	var rawEnvelope struct {
		Data struct {
			Message json.RawMessage `json:"message"`
		} `json:"data"`
	}
	rawEnvelopeOK := json.Unmarshal(bodyBytes, &rawEnvelope) == nil && len(rawEnvelope.Data.Message) > 0

	// evoDecryptMedia calls Evolution API to decrypt a WhatsApp CDN message and returns a data URI.
	// fallbackMime is used when the API doesn't return a proper data URI prefix.
	evoDecryptMedia := func(fallbackMime string) string {
		if !rawEnvelopeOK {
			log.Printf("rfq_bot: data.message absent in webhook body — cannot decrypt")
			return ""
		}
		mediaPayload, _ := json.Marshal(map[string]interface{}{
			"message":      map[string]interface{}{"key": payload.Data.Key, "message": rawEnvelope.Data.Message},
			"convertToMp4": false,
		})
		respBody, status, err := evoCall("POST",
			fmt.Sprintf("%s/chat/getBase64FromMediaMessage/%s", strings.TrimRight(evoURL, "/"), evoInstance),
			evoKey, mediaPayload)
		if err != nil || (status != 200 && status != 201) {
			log.Printf("rfq_bot: getBase64FromMediaMessage failed status=%d err=%v", status, err)
			return ""
		}
		var mediaResp struct{ Base64 string `json:"base64"` }
		if json.Unmarshal(respBody, &mediaResp) != nil || mediaResp.Base64 == "" {
			log.Printf("rfq_bot: getBase64FromMediaMessage parse failed body=%.200s", string(respBody))
			return ""
		}
		if strings.HasPrefix(mediaResp.Base64, "data:") {
			return mediaResp.Base64
		}
		// Raw base64 — build data URI
		mime := fallbackMime
		if mime == "" {
			if raw, err2 := base64.StdEncoding.DecodeString(mediaResp.Base64); err2 == nil {
				mime = detectImageMIME(raw)
			} else {
				mime = "application/octet-stream"
			}
		}
		return "data:" + mime + ";base64," + mediaResp.Base64
	}

	// Collect images
	var mediaURLs []string
	if msg.ImageMessage.URL != "" {
		dataURI := evoDecryptMedia(msg.ImageMessage.Mimetype)
		if dataURI == "" {
			dataURI = msg.ImageMessage.URL // encrypted fallback
		}
		mediaURLs = append(mediaURLs, dataURI)
	}
	rfq.MediaURLs = mediaURLs

	// Collect documents (PDF, Excel, etc.) — also decrypt so we can forward them and extract text
	type rawDoc struct {
		url      string
		fileName string
		mimeType string
	}
	var rawDocs []rawDoc
	if msg.DocumentMessage.URL != "" {
		rawDocs = append(rawDocs, rawDoc{msg.DocumentMessage.URL, msg.DocumentMessage.FileName, msg.DocumentMessage.Mimetype})
	}
	if inner := msg.DocumentWithCaptionMessage.Message.DocumentMessage; inner.URL != "" {
		rawDocs = append(rawDocs, rawDoc{inner.URL, inner.FileName, inner.Mimetype})
	}

	var documents []models.RFQDocument
	var docTextParts []string // text extracted from document contents (XLSX, CSV, etc.)
	for _, rd := range rawDocs {
		dataURI := evoDecryptMedia(rd.mimeType)
		storedURL := dataURI
		if storedURL == "" {
			storedURL = rd.url // encrypted fallback — forwarding will fail but at least it's stored
		}
		documents = append(documents, models.RFQDocument{
			URL:      storedURL,
			FileName: rd.fileName,
			MimeType: rd.mimeType,
		})
		// Extract readable text from XLSX / CSV for LLM categorization
		if dataURI != "" {
			_, b64data := splitDataURI(dataURI)
			if raw, err2 := base64.StdEncoding.DecodeString(b64data); err2 == nil {
				extracted := extractDocumentText(raw, rd.mimeType, rd.fileName)
				if extracted != "" {
					docTextParts = append(docTextParts, extracted)
				}
			}
		}
	}
	rfq.Documents = documents

	// Store extracted document text (XLSX rows, CSV content) separately — used for LLM context only,
	// NOT appended to TextContent so it doesn't pollute supplier messages when the doc is forwarded.
	if len(docTextParts) > 0 {
		rfq.ExtractedText = strings.Join(docTextParts, "\n\n")
	}

	hasImages := len(mediaURLs) > 0
	hasDocs := len(documents) > 0

	// Skip if there's absolutely nothing to process
	if text == "" && !hasImages && !hasDocs {
		log.Printf("rfq_bot: empty message from %s (type=%s) — skipping", fromPhone, payload.Data.MessageType)
		return
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
		rfq.MessageType = "text"
	}

	if err := models.CreateRFQReceived(rfq); err != nil {
		log.Printf("rfq_bot: failed to save RFQ: %v", err)
		return
	}

	// Notify connected browser tabs immediately
	BroadcastRFQEvent(storeObjID.Hex(), "rfq_received")

	// Process in background
	go processRFQ(rfq, storeObjID)
}

type evolutionWebhookPayload struct {
	Event    string `json:"event"`
	Instance string `json:"instance"`
	Data     struct {
		Key struct {
			RemoteJid string `json:"remoteJid"`
			FromMe    bool   `json:"fromMe"`
			ID        string `json:"id"`
		} `json:"key"`
		PushName string `json:"pushName"`
		Message  struct {
			Conversation        string `json:"conversation"`
			ExtendedTextMessage struct {
				Text        string `json:"text"`
				ContextInfo struct {
					StanzaID string `json:"stanzaId"`
				} `json:"contextInfo"`
			} `json:"extendedTextMessage"`
			ImageMessage struct {
				URL         string `json:"url"`
				Caption     string `json:"caption"`
				Mimetype    string `json:"mimetype"`
				ContextInfo struct {
					StanzaID string `json:"stanzaId"`
				} `json:"contextInfo"`
			} `json:"imageMessage"`
			DocumentMessage struct {
				URL         string `json:"url"`
				FileName    string `json:"fileName"`
				Mimetype    string `json:"mimetype"`
				Caption     string `json:"caption"`
				ContextInfo struct {
					StanzaID string `json:"stanzaId"`
				} `json:"contextInfo"`
			} `json:"documentMessage"`
			// Evolution API sometimes wraps document+caption in this type
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
			// Video message (skip content, but note it arrived)
			VideoMessage struct {
				URL      string `json:"url"`
				Caption  string `json:"caption"`
				Mimetype string `json:"mimetype"`
			} `json:"videoMessage"`
		} `json:"message"`
		MessageType      string `json:"messageType"`
		MessageTimestamp int64  `json:"messageTimestamp"`
	} `json:"data"`
}

// extractStanzaID returns the WhatsApp quoted-message ID from the payload, if the
// sender is replying to a specific message (contextInfo.stanzaId).
func extractStanzaID(payload evolutionWebhookPayload) string {
	m := payload.Data.Message
	if id := m.ExtendedTextMessage.ContextInfo.StanzaID; id != "" {
		return id
	}
	if id := m.ImageMessage.ContextInfo.StanzaID; id != "" {
		return id
	}
	if id := m.DocumentMessage.ContextInfo.StanzaID; id != "" {
		return id
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

// handleBuyerFollowup forwards a buyer's reply (to a bot relay) to the correct supplier.
// Triggered when contextInfo.stanzaId matches a previously saved BuyerRelayRecord.
func handleBuyerFollowup(store *models.Store, rfq *models.RFQReceived, supplierPhone, buyerPhone, buyerName string, payload evolutionWebhookPayload) {
	botURL := store.Settings.BotEvolutionAPIURL
	if botURL == "" {
		botURL = evoDefaultURL
	}
	botKey := store.Settings.BotEvolutionAPIKey
	if botKey == "" {
		botKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName
	if botInstance == "" {
		log.Printf("rfq_bot: buyer follow-up — bot instance not configured, can't relay to supplier %s", supplierPhone)
		return
	}

	name := buyerName
	if name == "" {
		name = buyerPhone
	}
	header := fmt.Sprintf("📩 *Follow-up from buyer %s (+%s)*\n\n", name, buyerPhone)

	// Extract text
	msg := payload.Data.Message
	text := ""
	switch {
	case msg.Conversation != "":
		text = msg.Conversation
	case msg.ExtendedTextMessage.Text != "":
		text = msg.ExtendedTextMessage.Text
	case msg.ImageMessage.Caption != "":
		text = msg.ImageMessage.Caption
	case msg.DocumentMessage.Caption != "":
		text = msg.DocumentMessage.Caption
	}
	if text == "" && msg.ImageMessage.URL == "" && msg.DocumentMessage.URL == "" {
		text = "(no text)"
	}

	// Send text to supplier
	if text != "" {
		textPayload, _ := json.Marshal(map[string]interface{}{
			"number": supplierPhone,
			"text":   header + text,
		})
		evoCall("POST",
			fmt.Sprintf("%s/message/sendText/%s", strings.TrimRight(botURL, "/"), botInstance),
			botKey, textPayload)
	}

	// Forward image if present
	if msg.ImageMessage.URL != "" {
		imgPayload, _ := json.Marshal(map[string]interface{}{
			"number":    supplierPhone,
			"mediatype": "image",
			"mimetype":  msg.ImageMessage.Mimetype,
			"caption":   "Image from buyer",
			"media":     msg.ImageMessage.URL,
		})
		evoCall("POST",
			fmt.Sprintf("%s/message/sendMedia/%s", strings.TrimRight(botURL, "/"), botInstance),
			botKey, imgPayload)
	}

	// Forward document if present
	docURL := msg.DocumentMessage.URL
	if docURL != "" {
		docPayload, _ := json.Marshal(map[string]interface{}{
			"number":    supplierPhone,
			"mediatype": "document",
			"mimetype":  msg.DocumentMessage.Mimetype,
			"fileName":  msg.DocumentMessage.FileName,
			"caption":   "Document from buyer",
			"media":     docURL,
		})
		evoCall("POST",
			fmt.Sprintf("%s/message/sendMedia/%s", strings.TrimRight(botURL, "/"), botInstance),
			botKey, docPayload)
	}

	log.Printf("rfq_bot: buyer follow-up from %s relayed to supplier %s (rfq=%s)", buyerPhone, supplierPhone, rfq.ID.Hex())
}

// handleSupplierReply forwards a supplier's incoming message back to the original buyer.
// Called when the sender is not in the RFQAllowedSenders whitelist — meaning they're
// likely a supplier responding to an RFQ we forwarded to them.
func handleSupplierReply(store *models.Store, supplierPhone, supplierName string, text string, payload evolutionWebhookPayload) {
	originalRFQ, err := models.FindLatestRFQBySupplierPhone(store.ID, supplierPhone)
	if err != nil {
		log.Printf("rfq_bot: no forwarded RFQ found for supplier %s — ignoring reply", supplierPhone)
		return
	}

	buyerPhone := originalRFQ.FromPhone
	if buyerPhone == "" {
		return
	}

	botURL := store.Settings.BotEvolutionAPIURL
	if botURL == "" {
		botURL = evoDefaultURL
	}
	botKey := store.Settings.BotEvolutionAPIKey
	if botKey == "" {
		botKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName
	if botInstance == "" {
		log.Printf("rfq_bot: supplier reply from %s — bot instance not configured, can't relay", supplierPhone)
		return
	}

	name := supplierName
	if name == "" {
		name = supplierPhone
	}

	// Build forwarding header
	header := fmt.Sprintf("📩 *Reply from supplier %s (+%s)*\n\n", name, supplierPhone)

	// Extract text from all message variants (supplier may quote the forwarded RFQ)
	msg := payload.Data.Message
	msgText := text
	if msgText == "" {
		switch {
		case msg.ExtendedTextMessage.Text != "":
			msgText = msg.ExtendedTextMessage.Text
		case msg.ImageMessage.Caption != "":
			msgText = msg.ImageMessage.Caption
		case msg.DocumentMessage.Caption != "":
			msgText = msg.DocumentMessage.Caption
		}
	}
	if msgText == "" {
		msgText = "(no text)"
	}
	textMsg := map[string]interface{}{
		"number": buyerPhone,
		"text":   header + msgText,
	}
	if originalRFQ.BuyerMsgID != "" {
		textMsg["quoted"] = map[string]interface{}{
			"key": map[string]interface{}{
				"remoteJid": buyerPhone + "@s.whatsapp.net",
				"fromMe":    false,
				"id":        originalRFQ.BuyerMsgID,
			},
		}
	}
	textPayload, _ := json.Marshal(textMsg)
	relayBody, status, sendErr := evoCall("POST",
		fmt.Sprintf("%s/message/sendText/%s", strings.TrimRight(botURL, "/"), botInstance),
		botKey, textPayload)
	if sendErr != nil || (status != 200 && status != 201) {
		log.Printf("rfq_bot: supplier reply relay to buyer %s failed: status=%d err=%v", buyerPhone, status, sendErr)
		return
	}
	// Save the outgoing message ID so we can route the buyer's follow-up to this supplier
	if relayMsgID := parseMsgIDFromEvoResponse(relayBody); relayMsgID != "" {
		_ = models.AddBuyerRelayToRFQ(originalRFQ.StoreID, originalRFQ.ID, models.BuyerRelayRecord{
			MsgID:         relayMsgID,
			SupplierPhone: supplierPhone,
			SentAt:        time.Now(),
		})
	}

	// Forward image if present
	if msg.ImageMessage.URL != "" {
		imgPayload, _ := json.Marshal(map[string]interface{}{
			"number":    buyerPhone,
			"mediatype": "image",
			"mimetype":  msg.ImageMessage.Mimetype,
			"caption":   "Image from supplier",
			"media":     msg.ImageMessage.URL,
		})
		evoCall("POST",
			fmt.Sprintf("%s/message/sendMedia/%s", strings.TrimRight(botURL, "/"), botInstance),
			botKey, imgPayload)
	}

	// Forward document if present
	docURL := msg.DocumentMessage.URL
	docName := msg.DocumentMessage.FileName
	if docURL == "" {
		docURL = msg.DocumentWithCaptionMessage.Message.DocumentMessage.URL
		docName = msg.DocumentWithCaptionMessage.Message.DocumentMessage.FileName
	}
	if docURL != "" {
		mimeType := msg.DocumentMessage.Mimetype
		if mimeType == "" {
			mimeType = msg.DocumentWithCaptionMessage.Message.DocumentMessage.Mimetype
		}
		docPayload, _ := json.Marshal(map[string]interface{}{
			"number":    buyerPhone,
			"mediatype": "document",
			"mimetype":  mimeType,
			"fileName":  docName,
			"caption":   "Document from supplier",
			"media":     docURL,
		})
		evoCall("POST",
			fmt.Sprintf("%s/message/sendMedia/%s", strings.TrimRight(botURL, "/"), botInstance),
			botKey, docPayload)
	}

	log.Printf("rfq_bot: relayed supplier %s reply to buyer %s", supplierPhone, buyerPhone)
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
	if !mediaFailOpen && !isRFQMessage(store, llmText, imageBase64s) {
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
		} else {
			record.Status = "failed"
			record.ErrorMsg = errMsg
			rfq.Status = "forwarded" // partial forward is still "forwarded"
			log.Printf("rfq_bot[%s]: ✗ failed to forward to %s (%s): %s [%d/%d]", rfqIDStr, sup.Name, sup.Phone, errMsg, step, total)
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

	// Reply to the original sender with a per-market summary via the bot instance.
	replyRFQSummaryToSender(store, rfq)
}

// notifyBuyerNoSuppliers sends the buyer a WhatsApp message when no matching suppliers could be found.
func notifyBuyerNoSuppliers(store *models.Store, rfq *models.RFQReceived, categories []string) {
	botURL := store.Settings.BotEvolutionAPIURL
	if botURL == "" {
		botURL = evoDefaultURL
	}
	botKey := store.Settings.BotEvolutionAPIKey
	if botKey == "" {
		botKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName
	if botInstance == "" {
		return
	}

	catStr := strings.Join(categories, ", ")
	companyName := store.Name
	if companyName == "" {
		companyName = "us"
	}
	msg := fmt.Sprintf(
		"Thank you for your RFQ. Unfortunately, we could not find any verified WhatsApp-enabled suppliers for *%s* at this time.\n\nWe will continue searching and get back to you as soon as we identify suitable suppliers. You may also contact %s directly for assistance.",
		catStr, companyName,
	)

	payload, _ := json.Marshal(map[string]interface{}{
		"number": rfq.FromPhone,
		"text":   msg,
		"quoted": map[string]interface{}{
			"key": map[string]interface{}{
				"remoteJid": rfq.FromPhone + "@s.whatsapp.net",
				"fromMe":    false,
				"id":        rfq.BuyerMsgID,
			},
		},
	})
	_, status, err := evoCall("POST",
		fmt.Sprintf("%s/message/sendText/%s", strings.TrimRight(botURL, "/"), botInstance),
		botKey, payload)
	if err != nil || (status != 200 && status != 201) {
		log.Printf("rfq_bot: failed to notify buyer %s of no-supplier result: status=%d err=%v", rfq.FromPhone, status, err)
	}
}

// replyRFQSummaryToSender sends the forwarding summary back to the buyer through the bot WhatsApp.
func replyRFQSummaryToSender(store *models.Store, rfq *models.RFQReceived) {
	botURL := store.Settings.BotEvolutionAPIURL
	if botURL == "" {
		botURL = evoDefaultURL
	}
	botKey := store.Settings.BotEvolutionAPIKey
	if botKey == "" {
		botKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName
	if botInstance == "" {
		return // no bot instance — can't reply
	}

	// Group successfully sent records by purchase market.
	type supplierSummary struct {
		name        string
		phone       string
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
			name:        r.SupplierName,
			phone:       r.Phone,
			googleMapsURL: r.GoogleMapsURL,
		})
	}

	if len(marketOrder) == 0 {
		return // nothing was sent successfully
	}

	for _, market := range marketOrder {
		entry := marketMap[market]
		count := len(entry.suppliers)
		noun := "Suppliers"
		if count == 1 {
			noun = "Supplier"
		}

		// Build numbered supplier list with phone and Maps link
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
		textMsg := map[string]interface{}{
			"number": rfq.FromPhone,
			"text":   msg,
		}
		// Quote the original buyer message so the summary appears in the same RFQ thread
		if rfq.BuyerMsgID != "" {
			textMsg["quoted"] = map[string]interface{}{
				"key": map[string]interface{}{
					"remoteJid": rfq.FromPhone + "@s.whatsapp.net",
					"fromMe":    false,
					"id":        rfq.BuyerMsgID,
				},
			}
		}
		payload, _ := json.Marshal(textMsg)
		_, status, err := evoCall("POST",
			fmt.Sprintf("%s/message/sendText/%s", strings.TrimRight(botURL, "/"), botInstance),
			botKey, payload)
		if err != nil || (status != 200 && status != 201) {
			log.Printf("rfq_bot: failed to send summary reply to %s: status=%d err=%v", rfq.FromPhone, status, err)
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
			model = "gemini-2.0-flash"
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
	return "", fmt.Errorf("unknown provider: %s", provider)
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

	prompt := `You are a procurement assistant. Decide whether the message or image below is a Request for Quotation (RFQ) — i.e. the sender wants to buy or source specific products or services.

Reply with ONLY the single word "yes" or "no".

An RFQ asks about pricing, availability, supply quantity, or procurement of specific items. It may be a text message, a table of parts, a product list, or any document requesting quotes.
A non-RFQ is a greeting, thanks, casual chat, or any message that does not request specific goods or services.`

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
		return nil
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
		return nil, fmt.Errorf("unsupported LLM provider: %s", provider)
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
		model = "gemini-2.0-flash"
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
			model = "gemini-2.0-flash"
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

// findSuppliers collects up to `min` suppliers per (category × purchase market) pair,
// supplementing from Google Maps when the DB doesn't have enough for a pair.
// Target total = min × numCategories × numMarkets (fewer when not enough suppliers exist).
func findSuppliers(store *models.Store, storeID primitive.ObjectID, categories []string) ([]models.RFQSupplier, error) {
	min := rfqMinSuppliers(store)

	// Build market list — fall back to no-market query if none configured
	markets := store.Settings.PurchaseMarkets
	if len(markets) == 0 {
		markets = []string{""}
	}

	// Build category list — fall back to generic search if none identified
	searchCategories := categories
	if len(searchCategories) == 0 {
		searchCategories = []string{"supplier"}
	}

	// Evo connection details for WhatsApp validation
	evoURL := store.Settings.BotEvolutionAPIURL
	if evoURL == "" {
		evoURL = evoDefaultURL
	}
	evoKey := store.Settings.BotEvolutionAPIKey
	if evoKey == "" {
		evoKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName

	// Track phones globally to avoid forwarding to the same supplier twice
	globalSeen := map[string]bool{}
	var allSuppliers []models.RFQSupplier

	for _, market := range markets {
		for _, category := range searchCategories {
			// 1. Look up `min` suppliers from DB for this (category, market) pair
			dbResult, _ := models.FindRFQSuppliersByMarketAndCategories(storeID, []string{category}, market, int64(min))

			var pairSuppliers []models.RFQSupplier
			for _, s := range dbResult {
				if s.Phone == "" || globalSeen[s.Phone] {
					continue
				}
				// Mark seen immediately so Maps doesn't waste a check on the same number
				globalSeen[s.Phone] = true
				if !hasWhatsApp(evoURL, evoKey, botInstance, s.Phone) {
					log.Printf("rfq_bot: db supplier %q (%s) has no WhatsApp — skipping", s.Name, s.Phone)
					continue
				}
				s.MatchedCategory = category
				pairSuppliers = append(pairSuppliers, s)
			}

			// 2. Supplement from Google Maps if DB doesn't have enough for this pair
			if len(pairSuppliers) < min && store.Settings.GoogleMapsAPIKey != "" {
				// Fetch extra results to account for numbers without WhatsApp
				needed := (min - len(pairSuppliers)) * 5
				if needed < 10 {
					needed = 10
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
						globalSeen[sup.Phone] = true // mark before WhatsApp check to avoid retrying
						if !hasWhatsApp(evoURL, evoKey, botInstance, sup.Phone) {
							log.Printf("rfq_bot: maps supplier %q (%s) has no WhatsApp — skipping, not saving to DB", sup.Name, sup.Phone)
							continue
						}
						sup.MatchedCategory = category
						models.UpsertRFQSupplierByPlaceID(sup)
						pairSuppliers = append(pairSuppliers, *sup)
						added = true
					}
					if added {
						BroadcastRFQEvent(storeID.Hex(), "supplier_updated")
					}
				}
			}

			log.Printf("rfq_bot: market=%q cat=%q found %d/%d suppliers", market, category, len(pairSuppliers), min)
			allSuppliers = append(allSuppliers, pairSuppliers...)
		}
	}

	return allSuppliers, nil
}

// searchGoogleMapsSuppliers uses the Places API (New) v1 which includes phone numbers
// directly in the search response — no separate detail lookup needed.
// market is appended to the query (e.g. "Steel Pipes supplier in Dammam"); empty = no location suffix.
func searchGoogleMapsSuppliers(apiKey, category, market string, storeID primitive.ObjectID, maxResults int) ([]models.RFQSupplier, error) {
	const endpoint = "https://places.googleapis.com/v1/places:searchText"
	const fieldMask = "places.id,places.displayName,places.formattedAddress,places.rating,places.location,places.internationalPhoneNumber"

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
			ID                   string  `json:"id"`
			FormattedAddress     string  `json:"formattedAddress"`
			Rating               float64 `json:"rating"`
			InternationalPhone   string  `json:"internationalPhoneNumber"`
			DisplayName          struct {
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
		sup := models.RFQSupplier{
			Name:           place.DisplayName.Text,
			Phone:          phone,
			Address:        place.FormattedAddress,
			Latitude:       place.Location.Latitude,
			Longitude:      place.Location.Longitude,
			Rating:         place.Rating,
			GooglePlaceID:  place.ID,
			GoogleMapsURL:  mapsURL,
			Categories:     []string{category},
			PurchaseMarket: market,
			StoreID:        storeID,
		}
		suppliers = append(suppliers, sup)
	}
	return suppliers, nil
}

// hasWhatsApp returns true if the given phone number has an active WhatsApp account,
// verified via Evolution API. Returns false on any error (fail-open: if the bot is
// not configured, we don't block the supplier from being used).
func hasWhatsApp(evoURL, evoKey, instance, phone string) bool {
	if phone == "" {
		return false
	}
	// If bot not configured we can't validate — allow the supplier through
	if instance == "" {
		return true
	}
	payload, _ := json.Marshal(map[string]interface{}{"numbers": []string{phone}})
	body, status, err := evoCall("POST",
		fmt.Sprintf("%s/chat/whatsappNumbers/%s", strings.TrimRight(evoURL, "/"), instance),
		evoKey, payload)
	if err != nil || (status != 200 && status != 201) {
		return false
	}
	var result []struct {
		Exists bool `json:"exists"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result) == 0 {
		return false
	}
	return result[0].Exists
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
	// Use the bot instance (same number that receives RFQs from buyers)
	evoURL := store.Settings.BotEvolutionAPIURL
	if evoURL == "" {
		evoURL = evoDefaultURL
	}
	evoKey := store.Settings.BotEvolutionAPIKey
	if evoKey == "" {
		evoKey = evoGlobalKey
	}
	instance := store.Settings.BotEvolutionInstanceName
	if instance == "" {
		return false, "Bot WhatsApp not connected", ""
	}

	// Save supplier as contact in the rfqsend WhatsApp before messaging (best-effort)
	saveContact(evoURL, evoKey, instance, supplier.Phone, supplier.Name)

	// First-contact detection: supplier was just discovered (GooglePlaceID set) or has no prior history
	firstContact := supplier.GooglePlaceID != "" || supplier.AddedAt.After(time.Now().Add(-24*time.Hour))

	// Build message: LLM-generated unique intro + original message verbatim
	intro := generateSupplierIntro(store, supplier.Name, rfq.Categories, rfq.TextContent, idx, firstContact)
	originalContent := rfq.TextContent
	if originalContent == "" {
		originalContent = "(No text — please see the attached image/document)"
	}

	companyName := store.Name
	if companyName == "" {
		companyName = "our company"
	}

	// Use store's custom intro if set, otherwise use the auto-generated one
	customIntro := strings.TrimSpace(store.Settings.RFQIntro)

	var msg string
	if firstContact {
		var opening string
		if customIntro != "" {
			opening = customIntro
		} else {
			opening = fmt.Sprintf("Hello, We are from *%s*.", companyName)
		}
		msg = fmt.Sprintf("%s\n\nDear %s,\n\n%s\n\n---\n%s", opening, supplier.Name, intro, originalContent)
	} else {
		greetings := []string{"Hello", "Hi", "Dear"}
		greeting := greetings[idx%len(greetings)]
		msg = fmt.Sprintf("%s %s,\n\n%s\n\n---\n%s", greeting, supplier.Name, intro, originalContent)
	}

	// Send text message (Evolution API v2.3.x flat format)
	textPayload, _ := json.Marshal(map[string]interface{}{
		"number": supplier.Phone,
		"text":   msg,
	})
	respBody, status, err := evoCall("POST",
		fmt.Sprintf("%s/message/sendText/%s", strings.TrimRight(evoURL, "/"), instance),
		evoKey, textPayload)
	if err != nil {
		return false, err.Error(), ""
	}
	if status != 200 && status != 201 {
		return false, fmt.Sprintf("evolution API %d: %s", status, string(respBody)), ""
	}

	// Forward images to supplier
	for _, mediaURL := range rfq.MediaURLs {
		// Evolution API sendMedia expects either a public URL or raw base64 (no data: prefix).
		// If we have a data URI, split it into MIME + raw base64.
		mediaField := mediaURL
		mimeType := "image/jpeg"
		if strings.HasPrefix(mediaURL, "data:") {
			mimeType, mediaField = splitDataURI(mediaURL)
		}
		imgPayload, _ := json.Marshal(map[string]interface{}{
			"number":    supplier.Phone,
			"mediatype": "image",
			"mimetype":  mimeType,
			"caption":   "RFQ Image",
			"media":     mediaField,
		})
		body, status, err := evoCall("POST",
			fmt.Sprintf("%s/message/sendMedia/%s", strings.TrimRight(evoURL, "/"), instance),
			evoKey, imgPayload)
		if err != nil || (status != 200 && status != 201) {
			log.Printf("rfq_bot: sendMedia to %s failed status=%d err=%v body=%.200s", supplier.Phone, status, err, string(body))
		}
		time.Sleep(2 * time.Second)
	}

	// Forward documents (PDF, Excel, etc.)
	// doc.URL is a data URI (decrypted at webhook time) or encrypted CDN URL (fallback).
	// Evolution API sendMedia expects raw base64 (no data: prefix) or a public URL.
	for _, doc := range rfq.Documents {
		if doc.URL == "" {
			continue
		}
		mediaField := doc.URL
		mimeType := doc.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if strings.HasPrefix(doc.URL, "data:") {
			mimeType, mediaField = splitDataURI(doc.URL)
		}
		docPayload, _ := json.Marshal(map[string]interface{}{
			"number":    supplier.Phone,
			"mediatype": "document",
			"mimetype":  mimeType,
			"media":     mediaField,
			"fileName":  doc.FileName,
			"caption":   "RFQ Document",
		})
		body, status, err := evoCall("POST",
			fmt.Sprintf("%s/message/sendMedia/%s", strings.TrimRight(evoURL, "/"), instance),
			evoKey, docPayload)
		if err != nil || (status != 200 && status != 201) {
			log.Printf("rfq_bot: sendDoc to %s failed status=%d err=%v body=%.200s", supplier.Phone, status, err, string(body))
		}
		time.Sleep(2 * time.Second)
	}

	return true, "", msg
}

// saveContact creates or updates a WhatsApp contact on the given instance (best-effort, errors ignored).
func saveContact(evoURL, evoKey, instance, phone, name string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"number":   phone,
		"fullName": name,
	})
	evoCall("POST",
		fmt.Sprintf("%s/contact/create/%s", strings.TrimRight(evoURL, "/"), instance),
		evoKey, payload)
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
	if err := models.CreateRFQSupplier(&supplier); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	BroadcastRFQEvent(supplier.StoreID.Hex(), "supplier_updated")
	respBytes, _ := json.Marshal(supplier)
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

	evoURL := store.Settings.BotEvolutionAPIURL
	if evoURL == "" {
		evoURL = evoDefaultURL
	}
	evoKey := store.Settings.BotEvolutionAPIKey
	if evoKey == "" {
		evoKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName

	markets := store.Settings.PurchaseMarkets
	if len(markets) == 0 {
		markets = []string{""}
	}

	for i, vendor := range vendors {
		step := (i + 1) * 90 / total
		sendProgress(step, 100, fmt.Sprintf("Processing %d/%d: %s", i+1, total, vendor.Name), false)

		products := rfqFetchVendorProducts(storeID, vendor.ID)
		if len(products) == 0 {
			continue
		}

		categories := rfqCategorizeVendorProducts(store, vendor.Name, products)
		if len(categories) == 0 {
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
				if !hasWhatsApp(evoURL, evoKey, botInstance, sup.Phone) {
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
	if store.Settings.RFQLLMAPIKey == "" {
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

	resp, err := callLLMText(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, prompt, store.Settings.RFQLLMProvider)
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
	if store.Settings.GoogleMapsAPIKey == "" || store.Settings.RFQLLMAPIKey == "" {
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

	evoURL := store.Settings.BotEvolutionAPIURL
	if evoURL == "" {
		evoURL = evoDefaultURL
	}
	evoKey := store.Settings.BotEvolutionAPIKey
	if evoKey == "" {
		evoKey = evoGlobalKey
	}
	botInstance := store.Settings.BotEvolutionInstanceName

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
			if !hasWhatsApp(evoURL, evoKey, botInstance, sup.Phone) {
				continue
			}
			models.UpsertRFQSupplierByPlaceID(sup)
			BroadcastRFQEvent(storeID.Hex(), "supplier_updated")
			break
		}
	}
}
