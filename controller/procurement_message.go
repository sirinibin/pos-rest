package controller

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListProcurementMessagesHandler handles GET /v1/procurement-messages
// Query params: store_id, type (email|whatsapp), direction (in|out), search, page, limit
func ListProcurementMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	msgType := r.URL.Query().Get("type")
	direction := r.URL.Query().Get("direction")
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	msgs, total, err := models.ListProcurementMessages(storeObjID, msgType, direction, search, page, limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages":    msgs,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": (total + int64(limit) - 1) / int64(limit),
	})
}

// GetProcurementMessageHandler handles GET /v1/procurement-messages/{id}
func GetProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	msg, err := models.GetProcurementMessage(id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	// Mark as read
	models.MarkProcurementMessageRead(id) //nolint:errcheck
	json.NewEncoder(w).Encode(msg)
}

// DeleteProcurementMessageHandler handles DELETE /v1/procurement-messages/{id}
func DeleteProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	if err := models.DeleteProcurementMessage(id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// CleanupProcurementMessagesHandler handles POST /v1/procurement-messages/cleanup
// Deletes messages older than store.settings.auto_delete_procurement_messages_days.
func CleanupProcurementMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		// Fetch from store settings
		store, _, err2 := rfqEmailGetStore(r.URL.Query().Get("store_id"))
		if err2 == nil {
			days = store.Settings.AutoDeleteProcurementMessagesDays
		}
	}

	deleted, err := models.DeleteOldProcurementMessages(storeObjID, days)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": deleted, "days": days})
}

// runAutoDeleteProcurementMessages runs the auto-delete cleanup for a store.
// Call it in a goroutine after processing any incoming message.
func runAutoDeleteProcurementMessages(storeID primitive.ObjectID, days int) {
	if days <= 0 {
		return
	}
	deleted, err := models.DeleteOldProcurementMessages(storeID, days)
	if err != nil {
		log.Printf("procurement_messages: auto-delete error for store %s: %v", storeID.Hex(), err)
		return
	}
	if deleted > 0 {
		log.Printf("procurement_messages: auto-deleted %d messages older than %d days for store %s", deleted, days, storeID.Hex())
	}
}

// saveProcurementEmailMessage persists an email record and returns the saved message.
func saveProcurementEmailMessage(storeID primitive.ObjectID, direction, provider, from string, to []string, subject, bodyText string, attachments []models.ProcurementAttachment, processedAsRFQ bool, rfqID *primitive.ObjectID, messageDate *time.Time) *models.ProcurementMessage {
	if attachments == nil {
		attachments = []models.ProcurementAttachment{}
	}
	msg := &models.ProcurementMessage{
		StoreID:        storeID,
		Type:           "email",
		Direction:      direction,
		Provider:       provider,
		From:           from,
		To:             to,
		Subject:        subject,
		BodyText:       bodyText,
		Attachments:    attachments,
		ProcessedAsRFQ: processedAsRFQ,
		RFQReceivedID:  rfqID,
		MessageDate:    messageDate,
		CreatedAt:      time.Now(),
	}
	if err := models.SaveProcurementMessage(msg); err != nil {
		log.Printf("procurement_messages: failed to save email message: %v", err)
		return nil
	}
	return msg
}

// saveProcurementWhatsAppMessage persists a WhatsApp message record and returns the saved message.
func saveProcurementWhatsAppMessage(storeID primitive.ObjectID, direction, from string, to []string, bodyText, waMessageType, wabaPNID string, attachments []models.ProcurementAttachment, processedAsRFQ bool, rfqID *primitive.ObjectID) *models.ProcurementMessage {
	if attachments == nil {
		attachments = []models.ProcurementAttachment{}
	}
	if waMessageType == "" {
		waMessageType = "text"
	}
	msg := &models.ProcurementMessage{
		StoreID:           storeID,
		Type:              "whatsapp",
		Direction:         direction,
		Provider:          "meta_whatsapp",
		From:              from,
		To:                to,
		BodyText:          bodyText,
		WAMessageType:     waMessageType,
		WABAPhoneNumberID: wabaPNID,
		Attachments:       attachments,
		ProcessedAsRFQ:    processedAsRFQ,
		RFQReceivedID:     rfqID,
		CreatedAt:         time.Now(),
	}
	if err := models.SaveProcurementMessage(msg); err != nil {
		log.Printf("procurement_messages: failed to save whatsapp message: %v", err)
		return nil
	}
	return msg
}
