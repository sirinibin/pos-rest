package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── defaults (used when store has no Evolution API settings saved) ──────────
const evoDefaultURL = "http://localhost:8081"
const evoGlobalKey = "startpos-evo-local-key"
const evoDefaultInstance = "wa_umlj"
const evoDefaultToken = "0EB2B865-1CB8-4B2A-8302-E211FFC2C1AD"

// ── shared helpers ───────────────────────────────────────────────────────────

type evolutionMediaPayload struct {
	Number    string `json:"number"`
	MediaType string `json:"mediatype"`
	MimeType  string `json:"mimetype"`
	Caption   string `json:"caption"`
	Media     string `json:"media"`
	FileName  string `json:"fileName"`
}

func evoConfigFromStore(storeIDStr string) (url, key, instance string) {
	url, key, instance = evoDefaultURL, evoGlobalKey, evoDefaultInstance
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
	if store.Settings.EvolutionAPIURL != "" {
		url = store.Settings.EvolutionAPIURL
	}
	if store.Settings.EvolutionAPIKey != "" {
		key = store.Settings.EvolutionAPIKey
	}
	if store.Settings.EvolutionInstanceName != "" {
		instance = store.Settings.EvolutionInstanceName
	}
	return
}

// resolveLIDPhone tries to convert a @lid JID to a real phone number by asking
// Evolution API for the contact's profile. Returns the phone number string if
// resolved (e.g. "966501234567"), or "" if not resolvable.
func resolveLIDPhone(evoBase, evoKey, instance, lidJID string) string {
	payload, _ := json.Marshal(map[string]string{"number": lidJID})
	body, status, err := evoCall("POST",
		fmt.Sprintf("%s/contact/fetchProfile/%s", evoBase, instance),
		evoKey, payload)
	if err != nil || status != 200 {
		return ""
	}
	var resp struct {
		Wuid string `json:"wuid"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Wuid == "" {
		return ""
	}
	// wuid is typically "966XXXXXXXX@s.whatsapp.net"
	if strings.HasSuffix(resp.Wuid, "@s.whatsapp.net") {
		return strings.TrimSuffix(resp.Wuid, "@s.whatsapp.net")
	}
	return ""
}

func evoCall(method, url, apiKey string, body []byte) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("apikey", apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

func saveStoreWhatsAppSettings(storeID primitive.ObjectID, evoURL, instanceName, token string) error {
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.UpdateOne(ctx,
		bson.M{"_id": storeID},
		bson.M{"$set": bson.M{
			"settings.evolution_api_url":       evoURL,
			"settings.evolution_instance_name": instanceName,
			"settings.evolution_api_key":       token,
		}},
	)
	return err
}

func clearStoreWhatsAppSettings(storeID primitive.ObjectID) error {
	collection := db.Client("").Database(db.GetPosDB()).Collection("store")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := collection.UpdateOne(ctx,
		bson.M{"_id": storeID},
		bson.M{"$set": bson.M{
			"settings.evolution_api_url":       "",
			"settings.evolution_instance_name": "",
			"settings.evolution_api_key":       "",
		}},
	)
	return err
}

// ── 1. ConnectWhatsApp ───────────────────────────────────────────────────────
// POST /v1/whatsapp/connect
// Body JSON: { "store_id": "...", "phone": "966..." }
// Creates an Evolution API instance named wa_<storeCode>, saves it to the store.
func ConnectWhatsApp(w http.ResponseWriter, r *http.Request) {
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

	evoURL := evoDefaultURL
	if store.Settings.EvolutionAPIURL != "" {
		evoURL = store.Settings.EvolutionAPIURL
	}

	// Instance name: wa_<storeCode> (lowercase, alphanum only)
	code := strings.ToLower(store.Code)
	code = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, code)
	instanceName := "wa_" + code

	// Delete any previous instance for this store silently
	evoCall("DELETE",
		fmt.Sprintf("%s/instance/delete/%s", evoURL, instanceName),
		evoGlobalKey, nil)

	// Create new instance
	createPayload, _ := json.Marshal(map[string]string{
		"instanceName": instanceName,
		"integration":  "WHATSAPP-BAILEYS",
	})
	respBody, status, err := evoCall("POST",
		fmt.Sprintf("%s/instance/create", evoURL),
		evoGlobalKey, createPayload)
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
		token = evoGlobalKey // fallback
	}

	// Save to store settings
	if err := saveStoreWhatsAppSettings(storeObjID, evoURL, instanceName, token); err != nil {
		http.Error(w, `{"error":"failed to save store settings"}`, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, `{"success":true,"instance_name":%q,"token":%q}`, instanceName, token)
}

// ── 2. GetWhatsAppQR ─────────────────────────────────────────────────────────
// GET /v1/whatsapp/qr?store_id=...
// Proxies the QR code base64 from Evolution API to the frontend.
func GetWhatsAppQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	evoURL, evoKey, instanceName := evoConfigFromStore(storeIDStr)

	respBody, _, err := evoCall("GET",
		fmt.Sprintf("%s/instance/connect/%s", evoURL, instanceName),
		evoKey, nil)
	if err != nil {
		http.Error(w, `{"error":"Evolution API unreachable"}`, http.StatusBadGateway)
		return
	}
	w.Write(respBody)
}

// ── 3. GetWhatsAppStatus ─────────────────────────────────────────────────────
// GET /v1/whatsapp/status?store_id=...
// Returns { connected: bool, phone: "...", instance_name: "..." }
func GetWhatsAppStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	evoURL, evoKey, instanceName := evoConfigFromStore(storeIDStr)

	if instanceName == "" {
		fmt.Fprint(w, `{"connected":false}`)
		return
	}

	respBody, _, err := evoCall("GET",
		fmt.Sprintf("%s/instance/fetchInstances", evoURL),
		evoKey, nil)
	if err != nil {
		http.Error(w, `{"error":"Evolution API unreachable"}`, http.StatusBadGateway)
		return
	}

	var instances []struct {
		Name             string `json:"name"`
		ConnectionStatus string `json:"connectionStatus"`
		OwnerJid         string `json:"ownerJid"`
	}
	if err := json.Unmarshal(respBody, &instances); err != nil {
		fmt.Fprint(w, `{"connected":false}`)
		return
	}

	for _, inst := range instances {
		if inst.Name == instanceName {
			connected := inst.ConnectionStatus == "open"
			phone := strings.TrimSuffix(inst.OwnerJid, "@s.whatsapp.net")
			fmt.Fprintf(w, `{"connected":%v,"phone":%q,"instance_name":%q,"status":%q}`,
				connected, phone, instanceName, inst.ConnectionStatus)
			return
		}
	}
	fmt.Fprintf(w, `{"connected":false,"instance_name":%q}`, instanceName)
}

// ── 4. DisconnectWhatsApp ────────────────────────────────────────────────────
// DELETE /v1/whatsapp/disconnect?store_id=...
// Deletes the Evolution API instance and clears store settings.
func DisconnectWhatsApp(w http.ResponseWriter, r *http.Request) {
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

	evoURL, evoKey, instanceName := evoConfigFromStore(storeIDStr)

	// Delete instance from Evolution API (best-effort, don't fail on error)
	if instanceName != "" {
		evoCall("DELETE",
			fmt.Sprintf("%s/instance/delete/%s", evoURL, instanceName),
			evoKey, nil)
	}

	// Clear store settings
	if err := clearStoreWhatsAppSettings(storeObjID); err != nil {
		http.Error(w, `{"error":"failed to clear store settings"}`, http.StatusInternalServerError)
		return
	}

	// Clear all synced contacts for this store
	models.ClearWhatsAppContacts(storeObjID)

	fmt.Fprint(w, `{"success":true}`)
}

// ── 5. SendWhatsAppDocument ──────────────────────────────────────────────────
// POST /v1/whatsapp/send-document
// Receives a PDF file + phone + caption, sends via Evolution API.
func SendWhatsAppDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, `{"error":"failed to parse form"}`, http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing file field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	phone := strings.TrimSpace(r.FormValue("phone"))
	if phone == "" {
		http.Error(w, `{"error":"missing phone"}`, http.StatusBadRequest)
		return
	}
	caption := r.FormValue("caption")
	filename := r.FormValue("filename")
	if filename == "" {
		filename = handler.Filename
	}

	evoURL, evoKey, evoInstance := evoConfigFromStore(r.FormValue("store_id"))
	base := strings.TrimRight(evoURL, "/")

	pdfBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, `{"error":"failed to read file"}`, http.StatusInternalServerError)
		return
	}

	// Side-effect: save to ./pdfs for local URL fallback
	if dst, err := os.Create(fmt.Sprintf("%s/%s", uploadDir, filename)); err == nil {
		dst.Write(pdfBytes)
		dst.Close()
	}

	// For @lid contacts, try to resolve to a real phone number via fetchProfile.
	if strings.HasSuffix(phone, "@lid") {
		resolved := resolveLIDPhone(base, evoKey, evoInstance, phone)
		if resolved != "" {
			phone = resolved
		}
	}

	payload := evolutionMediaPayload{
		Number:    phone,
		MediaType: "document",
		MimeType:  "application/pdf",
		Caption:   caption,
		Media:     base64.StdEncoding.EncodeToString(pdfBytes),
		FileName:  filename,
	}
	payloadBytes, _ := json.Marshal(payload)

	respBody, status, err := evoCall("POST",
		fmt.Sprintf("%s/message/sendMedia/%s", base, evoInstance),
		evoKey, payloadBytes)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Evolution API unreachable: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	if status != http.StatusOK && status != http.StatusCreated {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"Evolution API error","detail":%s}`, string(respBody))
		return
	}
	fmt.Fprintf(w, `{"success":true,"detail":%s}`, string(respBody))
}

// ── 6. CheckWhatsAppNumbers ──────────────────────────────────────────────────
// POST /v1/whatsapp/check-numbers
// Body JSON: { "store_id": "...", "numbers": ["966501234567", ...] }
// Returns Evolution API's exists check per number.
func CheckWhatsAppNumbers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		StoreID string   `json:"store_id"`
		Numbers []string `json:"numbers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	evoURL, evoKey, evoInstance := evoConfigFromStore(body.StoreID)

	payload, _ := json.Marshal(map[string][]string{"numbers": body.Numbers})
	respBody, status, err := evoCall("POST",
		fmt.Sprintf("%s/chat/whatsappNumbers/%s", strings.TrimRight(evoURL, "/"), evoInstance),
		evoKey, payload)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Evolution API unreachable: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	if status != http.StatusOK && status != http.StatusCreated {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"Evolution API error","detail":%s}`, string(respBody))
		return
	}
	w.Write(respBody)
}

// ── 7. GetWhatsAppContacts ───────────────────────────────────────────────────
// GET /v1/whatsapp/contacts?store_id=...&page=1&limit=50&search=...
// Reads contacts from MongoDB (synced by hourly cron). Fast — no Evolution API call.
// Add ?sync=true to force an immediate re-sync from Evolution API.
func GetWhatsAppContacts(w http.ResponseWriter, r *http.Request) {
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

	// Optional force re-sync
	if r.URL.Query().Get("sync") == "true" {
		go models.TriggerSyncWhatsAppContacts(storeObjID)
	}

	// Parse pagination params
	page := int64(1)
	limit := int64(50)
	if v := r.URL.Query().Get("page"); v != "" {
		fmt.Sscan(v, &page)
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscan(v, &limit)
	}
	search := r.URL.Query().Get("search")

	result, err := models.GetWhatsAppContacts(storeObjID, page, limit, search)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to fetch contacts: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	respBody, _ := json.Marshal(result)
	w.Write(respBody)
}

// ── 8. GetWhatsAppContactsCount ──────────────────────────────────────────────
// GET /v1/whatsapp/contacts-count?store_id=...
// Returns the number of WhatsApp contacts stored for the store.
func GetWhatsAppContactsCount(w http.ResponseWriter, r *http.Request) {
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
	count, err := models.CountWhatsAppContacts(storeObjID)
	if err != nil {
		// collection may not exist yet — return 0
		fmt.Fprintf(w, `{"count":0}`)
		return
	}
	fmt.Fprintf(w, `{"count":%d}`, count)
}

// ── 9. SyncWhatsAppContacts ──────────────────────────────────────────────────
// POST /v1/whatsapp/sync-contacts
// Body JSON: { "store_id": "..." }
// Triggers an immediate sync from Evolution API and returns the new count.
func SyncWhatsAppContacts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var body struct {
		StoreID string `json:"store_id"`
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
	count, err := models.SyncAndCountWhatsAppContacts(storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":%q}`, err.Error())
		return
	}
	fmt.Fprintf(w, `{"success":true,"count":%d}`, count)
}

// ── 10. ClearWhatsAppContacts ────────────────────────────────────────────────
// DELETE /v1/whatsapp/contacts?store_id=...
// Deletes all contacts from the store's whatsapp_contacts collection.
func ClearWhatsAppContacts(w http.ResponseWriter, r *http.Request) {
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
	deleted, err := models.ClearWhatsAppContacts(storeObjID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `{"success":true,"deleted":%d}`, deleted)
}
