package controller

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const metaWHVerifyToken = "gulfunion_meta_wh_2026_startpos"

func HandleMetaWhatsAppWebhook(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		metaWebhookVerify(w, r)
	case http.MethodPost:
		metaWebhookReceive(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func metaWebhookVerify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == metaWHVerifyToken {
		log.Println("meta_whatsapp: webhook verified successfully")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(challenge))
		return
	}

	log.Printf("meta_whatsapp: verification failed — mode=%q token=%q", mode, token)
	http.Error(w, "forbidden", http.StatusForbidden)
}

// metaWAMessage is a minimal representation of a WhatsApp Cloud API message event.
type metaWAMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"` // text|image|audio|video|document|sticker
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    struct{ Caption string `json:"caption"` } `json:"image"`
	Document struct {
		Caption  string `json:"caption"`
		Filename string `json:"filename"`
	} `json:"document"`
}

type metaWAEntry struct {
	ID      string `json:"id"` // WABA ID
	Changes []struct {
		Value struct {
			MessagingProduct string          `json:"messaging_product"`
			Metadata         struct {
				PhoneNumberID string `json:"phone_number_id"`
			} `json:"metadata"`
			Messages []metaWAMessage `json:"messages"`
		} `json:"value"`
	} `json:"changes"`
}

func metaWebhookReceive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("meta_whatsapp: failed to read body: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"received":true}`))

	var payload struct {
		Entry []metaWAEntry `json:"entry"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("meta_whatsapp: JSON parse error: %v body=%.300s", err, string(body))
		return
	}

	log.Printf("meta_whatsapp: event received: %.500s", string(body))

	// Record each incoming message in the procurement log.
	go func() {
		for _, entry := range payload.Entry {
			for _, change := range entry.Changes {
				pnID := change.Value.Metadata.PhoneNumberID
				storeID := resolveStoreByWABAPNID(pnID)
				if storeID.IsZero() {
					continue
				}
				store, _, serr := rfqEmailGetStore(storeID.Hex())
				autoDays := 0
				if serr == nil {
					autoDays = store.Settings.AutoDeleteProcurementMessagesDays
				}
				for _, msg := range change.Value.Messages {
					text := msg.Text.Body
					waType := msg.Type
					if text == "" {
						switch waType {
						case "image":
							text = "[Image] " + msg.Image.Caption
						case "document":
							text = "[Document] " + msg.Document.Filename
							if msg.Document.Caption != "" {
								text += ": " + msg.Document.Caption
							}
						case "audio":
							text = "[Audio message]"
						case "video":
							text = "[Video] " + msg.Image.Caption
						}
					}
					saveProcurementWhatsAppMessage(storeID, "in", msg.From, nil, strings.TrimSpace(text), waType, pnID, msg.ID, nil, false, nil, nil) //nolint:errcheck
				}
				runAutoDeleteProcurementMessages(storeID, autoDays)
			}
		}
	}()
}

// resolveStoreByWABAPNID finds the store that has the given WABA phone number ID configured.
func resolveStoreByWABAPNID(phoneNumberID string) primitive.ObjectID {
	if phoneNumberID == "" {
		return primitive.NilObjectID
	}
	col := db.Client("").Database(db.GetPosDB()).Collection("store")
	var result struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := col.FindOne(ctx, bson.M{
		"$or": bson.A{
			bson.M{"settings.bot_waba_phone_number_id": phoneNumberID},
			bson.M{"settings.store_rfq_waba_phone_number_id": phoneNumberID},
		},
	}).Decode(&result); err != nil {
		return primitive.NilObjectID
	}
	return result.ID
}
