package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// WhatsAppContact mirrors a contact from the store's connected WhatsApp account.
// Stored in DB "store_{storeID}", collection "whatsapp_contacts".
type WhatsAppContact struct {
	ID           primitive.ObjectID  `bson:"_id,omitempty"    json:"id,omitempty"`
	StoreID      *primitive.ObjectID `bson:"store_id"         json:"store_id"`
	JID          string              `bson:"jid"              json:"jid"`
	Name         string              `bson:"name"             json:"name"`
	PushName     string              `bson:"push_name"        json:"push_name"`
	Phone        string              `bson:"phone"            json:"phone"`
	LastChatAt   int64               `bson:"last_chat_at"     json:"last_chat_at"`   // unix seconds from findChats conversationTimestamp
	EvoUpdatedAt *time.Time          `bson:"evo_updated_at"   json:"evo_updated_at"`
	SyncedAt     *time.Time          `bson:"synced_at"        json:"synced_at"`
}

func whatsAppContactsCollection(storeID primitive.ObjectID) *mongo.Collection {
	return db.GetDB("store_" + storeID.Hex()).Collection("whatsapp_contacts")
}

// SyncWhatsAppContactsForAllStores iterates every store with use_whatsapp_api=true
// and a connected Evolution API instance, and syncs their contacts to MongoDB.
// Called by the hourly cron job.
func SyncWhatsAppContactsForAllStores() {
	stores, err := GetAllStores()
	if err != nil {
		log.Print("[WhatsApp] SyncAllStores: error fetching stores:", err)
		return
	}
	for _, store := range stores {
		if !store.Settings.UseWhatsAppAPI || store.Settings.EvolutionInstanceName == "" {
			continue
		}
		if err := SyncWhatsAppContactsForStore(&store); err != nil {
			log.Printf("[WhatsApp] SyncContacts store=%s: %v", store.ID.Hex(), err)
		}
	}
}

// SyncWhatsAppContactsForStore fetches contacts from Evolution API and upserts
// them into the store's whatsapp_contacts collection.
func SyncWhatsAppContactsForStore(store *Store) error {
	evoURL := store.Settings.EvolutionAPIURL
	if evoURL == "" {
		evoURL = "http://localhost:8081"
	}
	evoKey := store.Settings.EvolutionAPIKey
	if evoKey == "" {
		evoKey = "startpos-evo-local-key"
	}
	instance := store.Settings.EvolutionInstanceName
	base := strings.TrimRight(evoURL, "/")

	// merged holds the union of contacts from both sources, keyed by JID.
	// findContacts is fetched first so that richer phonebook names win when
	// findChats provides the same JID with only a pushName.
	merged := map[string]map[string]interface{}{}

	// ── 1. findContacts (address-book cache) ────────────────────────────────
	// Use a long timeout — large contact lists (1000+) can take >15s on first load.
	contactsBody, contactsStatus, err := syncHTTPCall(
		"POST",
		fmt.Sprintf("%s/chat/findContacts/%s", base, instance),
		evoKey,
		[]byte("{}"),
	)
	if err != nil {
		return fmt.Errorf("Evolution API unreachable: %w", err)
	}
	if contactsStatus != http.StatusOK {
		return fmt.Errorf("findContacts %d: %s", contactsStatus, string(contactsBody))
	}
	var rawContacts []map[string]interface{}
	if err := json.Unmarshal(contactsBody, &rawContacts); err != nil {
		return fmt.Errorf("unmarshal findContacts: %w", err)
	}
	for _, c := range rawContacts {
		jid, _ := c["remoteJid"].(string)
		if jid != "" {
			merged[jid] = c
		}
	}

	// ── 2. findChats (conversation list — catches contacts not in address book) ─
	chatsBody, chatsStatus, _ := syncHTTPCall(
		"POST",
		fmt.Sprintf("%s/chat/findChats/%s", base, instance),
		evoKey,
		[]byte("{}"),
	)
	fromChatsOnly := 0
	if chatsStatus == http.StatusOK {
		var rawChats []map[string]interface{}
		if err := json.Unmarshal(chatsBody, &rawChats); err == nil {
			for _, c := range rawChats {
				// findChats uses "id" for the JID
				jid, _ := c["id"].(string)
				if jid == "" {
					jid, _ = c["remoteJid"].(string)
				}
				if jid == "" {
					continue
				}
				if existing, exists := merged[jid]; exists {
					// Contact already in merged from findContacts — just stamp the chat timestamp.
					if ts, ok := c["conversationTimestamp"]; ok {
						existing["conversationTimestamp"] = ts
						merged[jid] = existing
					}
				} else {
					entry := map[string]interface{}{"remoteJid": jid}
					if pn, ok := c["pushName"].(string); ok {
						entry["pushName"] = pn
					}
					if nm, ok := c["name"].(string); ok {
						entry["name"] = nm
					}
					if ts, ok := c["conversationTimestamp"]; ok {
						entry["conversationTimestamp"] = ts
					}
					merged[jid] = entry
					fromChatsOnly++
				}
			}
		}
	} else {
		log.Printf("[WhatsApp] findChats %d (non-fatal, contacts only)", chatsStatus)
	}

	collection := whatsAppContactsCollection(store.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{bson.E{Key: "jid", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	now := time.Now()
	upserted, skipped := 0, 0
	lidLogged := 0 // log raw fields of first 3 @lid contacts so we can see what's available
	for jid, c := range merged {
		// Skip only system broadcast channels.
		if strings.HasSuffix(jid, "@broadcast") {
			skipped++
			continue
		}

		if strings.HasSuffix(jid, "@lid") && lidLogged < 3 {
			raw, _ := json.Marshal(c)
			log.Printf("[WhatsApp] @lid sample jid=%s raw=%s", jid, string(raw))
			lidLogged++
		}

		var phone string
		switch {
		case strings.HasSuffix(jid, "@s.whatsapp.net"):
			phone = strings.TrimSuffix(jid, "@s.whatsapp.net")
		case strings.HasSuffix(jid, "@g.us"):
			phone = strings.TrimSuffix(jid, "@g.us")
		case strings.HasSuffix(jid, "@lid"):
			phone = strings.TrimSuffix(jid, "@lid")
		default:
			phone = jid
		}

		pushName, _ := c["pushName"].(string)
		name, _ := c["name"].(string)
		if name == "" {
			name = pushName
		}
		if name == "" {
			name = phone
		}

		var evoUpdatedAt *time.Time
		if evoUpdStr, _ := c["updatedAt"].(string); evoUpdStr != "" {
			if t, err := time.Parse(time.RFC3339, evoUpdStr); err == nil {
				evoUpdatedAt = &t
			}
		}
		if evoUpdatedAt == nil {
			evoUpdatedAt = &now
		}

		var lastChatAt int64
		switch v := c["conversationTimestamp"].(type) {
		case float64:
			lastChatAt = int64(v)
		case int64:
			lastChatAt = v
		}

		contact := WhatsAppContact{
			StoreID:      &store.ID,
			JID:          jid,
			Name:         name,
			PushName:     pushName,
			Phone:        phone,
			LastChatAt:   lastChatAt,
			EvoUpdatedAt: evoUpdatedAt,
			SyncedAt:     &now,
		}

		filter := bson.M{"jid": jid}
		update := bson.M{"$set": contact}
		opts := options.Update().SetUpsert(true)
		if _, err := collection.UpdateOne(ctx, filter, update, opts); err != nil {
			log.Printf("[WhatsApp] upsert contact %s: %v", jid, err)
			continue
		}
		upserted++
	}

	log.Printf("[WhatsApp] SyncContacts store=%s instance=%s: %d synced, %d skipped, %d total (findContacts=%d, fromChatsOnly=%d)",
		store.ID.Hex(), instance, upserted, skipped, len(merged), len(rawContacts), fromChatsOnly)
	return nil
}

// WhatsAppContactsResult is the paginated response.
type WhatsAppContactsResult struct {
	Contacts   []WhatsAppContact `json:"contacts"`
	TotalCount int64             `json:"total_count"`
	Page       int64             `json:"page"`
	Limit      int64             `json:"limit"`
	TotalPages int64             `json:"total_pages"`
}

// GetWhatsAppContacts returns a paginated, searchable list of contacts from MongoDB.
func GetWhatsAppContacts(storeID primitive.ObjectID, page, limit int64, search string) (*WhatsAppContactsResult, error) {
	collection := whatsAppContactsCollection(storeID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if search != "" {
		filter = bson.M{"$or": bson.A{
			bson.M{"name": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"push_name": bson.M{"$regex": search, "$options": "i"}},
			bson.M{"phone": bson.M{"$regex": search, "$options": "i"}},
		}}
	}

	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	skip := (page - 1) * limit
	totalPages := (total + limit - 1) / limit

	findOpts := options.Find().
		SetSort(bson.D{
			bson.E{Key: "last_chat_at", Value: -1},
			bson.E{Key: "evo_updated_at", Value: -1},
		}).
		SetSkip(skip).
		SetLimit(limit)

	cursor, err := collection.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var contacts []WhatsAppContact
	if err := cursor.All(ctx, &contacts); err != nil {
		return nil, err
	}
	if contacts == nil {
		contacts = []WhatsAppContact{}
	}

	return &WhatsAppContactsResult{
		Contacts:   contacts,
		TotalCount: total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// TriggerSyncWhatsAppContacts can be called when a store first connects WhatsApp
// so contacts are available immediately without waiting for the cron.
// TriggerSyncWhatsAppContacts syncs contacts for a single store immediately (non-blocking).
func TriggerSyncWhatsAppContacts(storeID primitive.ObjectID) {
	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		log.Printf("[WhatsApp] TriggerSync: store not found: %v", err)
		return
	}
	if err := SyncWhatsAppContactsForStore(store); err != nil {
		log.Printf("[WhatsApp] TriggerSync: %v", err)
	}
}

// SyncAndCountWhatsAppContacts syncs contacts for a store and returns the new count.
func SyncAndCountWhatsAppContacts(storeID primitive.ObjectID) (int64, error) {
	store, err := FindStoreByID(&storeID, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("store not found: %w", err)
	}
	if err := SyncWhatsAppContactsForStore(store); err != nil {
		return 0, err
	}
	return CountWhatsAppContacts(storeID)
}

// CountWhatsAppContacts returns how many contacts are stored for a store.
func CountWhatsAppContacts(storeID primitive.ObjectID) (int64, error) {
	collection := whatsAppContactsCollection(storeID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return collection.CountDocuments(ctx, bson.M{})
}

// ClearWhatsAppContacts deletes all contacts from the store's whatsapp_contacts collection.
func ClearWhatsAppContacts(storeID primitive.ObjectID) (int64, error) {
	collection := whatsAppContactsCollection(storeID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := collection.DeleteMany(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// syncHTTPCall is like whatsAppHTTPCall but with a 90-second timeout for large
// contact list fetches that can exceed the default 15-second limit.
func syncHTTPCall(method, url, apiKey string, body []byte) ([]byte, int, error) {
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
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return respBody, resp.StatusCode, nil
}

// ── tiny HTTP helper reused from controller package (copied to avoid circular import)
func whatsAppHTTPCall(method, url, apiKey string, body []byte) ([]byte, int, error) {
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
