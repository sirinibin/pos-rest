package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// deleteAttachmentDirs removes the on-disk attachment directories for a slice of
// procurement messages. Each attachment URL is "/attachments/{storeID}/{dir}/filename"
// so the parent directory to remove is "./attachments/{storeID}/{dir}".
func deleteAttachmentDirs(msgs []models.ProcurementMessage) {
	dirs := map[string]struct{}{}
	for _, m := range msgs {
		for _, a := range m.Attachments {
			if a.URL == "" {
				continue
			}
			// Strip leading slash and split into segments
			parts := strings.SplitN(strings.TrimPrefix(a.URL, "/attachments/"), "/", 3)
			if len(parts) >= 2 {
				dirs["./attachments/"+parts[0]+"/"+parts[1]] = struct{}{}
			}
		}
	}
	for dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			log.Printf("procurement: remove attachment dir %s: %v", dir, err)
		} else {
			log.Printf("procurement: removed attachment dir %s", dir)
		}
	}
}

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
	rfqFilter := r.URL.Query().Get("rfq_filter") // "yes" | "no" | ""
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	msgs, total, err := models.ListProcurementMessages(storeObjID, msgType, direction, search, rfqFilter, page, limit)
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
// Optional query param: scope=everyone — attempts to retract the message via Meta Cloud API
// before removing it from the local database.
func DeleteProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}

	msg, _ := models.GetProcurementMessage(id)

	// "Delete for everyone" — best-effort Meta Cloud API retraction before local delete.
	// Only outgoing messages have a wamid (ExternalID) we can retract.
	var retractErr string
	if r.URL.Query().Get("scope") == "everyone" && msg != nil && msg.Direction == "out" && msg.ExternalID != "" {
		storeIDStr := msg.StoreID.Hex()
		phoneNumberID, accessToken := rfqMetaConfig(storeIDStr, "store_rfq")
		if phoneNumberID == "" || accessToken == "" {
			phoneNumberID, accessToken = rfqMetaConfig(storeIDStr, "bot")
		}
		if phoneNumberID != "" && accessToken != "" {
			if rErr := metaDeleteMessage(phoneNumberID, accessToken, msg.ExternalID); rErr != nil {
				retractErr = rErr.Error()
				log.Printf("procurement: meta retract %s: %v", msg.ExternalID, rErr)
			}
		} else {
			retractErr = "WABA not configured"
		}
	} else if r.URL.Query().Get("scope") == "everyone" && msg != nil && msg.ExternalID == "" {
		retractErr = "message ID not available (only new sent messages can be retracted)"
	}

	// Delete disk files, then remove the DB record (always, regardless of retraction result).
	if msg != nil {
		deleteAttachmentDirs([]models.ProcurementMessage{*msg})
	}
	if err := models.DeleteProcurementMessage(id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	resp := map[string]interface{}{"success": true}
	if retractErr != "" {
		resp["retract_warning"] = "message deleted locally; WhatsApp retraction failed: " + retractErr
	}
	json.NewEncoder(w).Encode(resp)
}

// DeleteThreadProcurementMessagesHandler handles DELETE /v1/procurement-messages/thread?store_id=&contact=&type=
// Deletes all messages in a conversation thread for a given contact (admin-only, enforced in frontend).
func DeleteThreadProcurementMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	contact := strings.TrimSpace(r.URL.Query().Get("contact"))
	if contact == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "contact is required"})
		return
	}
	msgType := r.URL.Query().Get("type") // optional: "email" | "whatsapp"

	// Build filter matching all messages for this contact (from or to).
	filter := bson.M{
		"store_id": storeObjID,
		"$or": bson.A{
			bson.M{"from": bson.M{"$regex": contact, "$options": "i"}},
			bson.M{"to": bson.M{"$regex": contact, "$options": "i"}},
		},
	}
	if msgType != "" {
		filter["type"] = msgType
	}

	if toClean, _ := models.FetchMessagesForCleanup(filter); len(toClean) > 0 {
		deleteAttachmentDirs(toClean)
	}

	deleted, err := models.DeleteProcurementMessagesByFilter(filter)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": deleted})
}

// DeleteAllProcurementMessagesHandler handles DELETE /v1/procurement-messages?store_id=&type=
// Deletes all messages for the store (admin-only gate enforced in frontend; no server auth needed beyond token).
func DeleteAllProcurementMessagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	msgType := r.URL.Query().Get("type") // optional: "email" | "whatsapp"

	// Fetch attachment URLs before deleting DB records so we can clean up disk.
	filter := bson.M{"store_id": storeObjID}
	if msgType != "" {
		filter["type"] = msgType
	}
	if toClean, _ := models.FetchMessagesForCleanup(filter); len(toClean) > 0 {
		deleteAttachmentDirs(toClean)
	}

	deleted, err := models.DeleteAllProcurementMessages(storeObjID, msgType)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": deleted})
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
	// Fetch attachment URLs of messages that will be deleted so we can remove disk files.
	cutoff := time.Now().AddDate(0, 0, -days)
	filter := bson.M{"store_id": storeID, "created_at": bson.M{"$lt": cutoff}}
	if toClean, _ := models.FetchMessagesForCleanup(filter); len(toClean) > 0 {
		deleteAttachmentDirs(toClean)
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
func saveProcurementEmailMessage(storeID primitive.ObjectID, direction, provider, from string, to []string, subject, bodyText, bodyHTML, externalID, emailMessageID string, attachments []models.ProcurementAttachment, attachmentMissing, processedAsRFQ bool, rfqID *primitive.ObjectID, messageDate *time.Time) *models.ProcurementMessage {
	if attachments == nil {
		attachments = []models.ProcurementAttachment{}
	}
	msg := &models.ProcurementMessage{
		StoreID:           storeID,
		Type:              "email",
		Direction:         direction,
		Provider:          provider,
		From:              from,
		To:                to,
		Subject:           subject,
		BodyText:          bodyText,
		BodyHTML:          bodyHTML,
		ExternalID:        externalID,
		EmailMessageID:    emailMessageID,
		Attachments:       attachments,
		AttachmentMissing: attachmentMissing,
		ProcessedAsRFQ:    processedAsRFQ,
		RFQReceivedID:     rfqID,
		MessageDate:       messageDate,
		CreatedAt:         time.Now(),
	}
	if err := models.SaveProcurementMessage(msg); err != nil {
		log.Printf("procurement_messages: failed to save email message: %v", err)
		return nil
	}
	return msg
}

// saveProcurementWhatsAppMessage persists a WhatsApp message record and returns the saved message.
// externalID is the WhatsApp message ID (wamid) returned by the API; pass "" if not available.
func saveProcurementWhatsAppMessage(storeID primitive.ObjectID, direction, from string, to []string, bodyText, waMessageType, wabaPNID, externalID string, attachments []models.ProcurementAttachment, processedAsRFQ bool, rfqID *primitive.ObjectID, messageDate *time.Time) *models.ProcurementMessage {
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
		ExternalID:        externalID,
		Attachments:       attachments,
		ProcessedAsRFQ:    processedAsRFQ,
		RFQReceivedID:     rfqID,
		MessageDate:       messageDate,
		CreatedAt:         time.Now(),
	}
	if err := models.SaveProcurementMessage(msg); err != nil {
		log.Printf("procurement_messages: failed to save whatsapp message: %v", err)
		return nil
	}
	return msg
}

// rfqCreationGate checks preconditions for creating an RFQ from a procurement message.
// Returns (httpStatus, errorMsg); status 0 means clear to proceed.
func rfqCreationGate(msg *models.ProcurementMessage, store *models.Store) (int, string) {
	if msg.ProcessedAsRFQ && msg.RFQReceivedID != nil {
		return http.StatusConflict, "RFQ already created"
	}
	if !store.Settings.EnableAIRFQBot {
		return http.StatusBadRequest, "AI RFQ bot is not enabled for this store"
	}
	if store.Settings.RFQLLMAPIKey == "" {
		return http.StatusBadRequest, "no LLM API key configured"
	}
	return 0, ""
}

// procurementMessageSource maps a procurement message type to its RFQ source label.
func procurementMessageSource(msgType string) string {
	if msgType == "whatsapp" {
		return "whatsapp"
	}
	return "email"
}

// CreateRFQFromProcurementMessageHandler handles POST /v1/procurement-messages/{id}/create-rfq
// Manually triggers LLM extraction + RFQ creation for a procurement message that was not auto-processed.
func CreateRFQFromProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}

	procMsg, err := models.GetProcurementMessage(msgID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}

	store, storeObjID, err := rfqEmailGetStore(procMsg.StoreID.Hex())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}

	if status, errMsg := rfqCreationGate(procMsg, store); status != 0 {
		w.WriteHeader(status)
		out := map[string]string{"error": errMsg}
		if status == http.StatusConflict && procMsg.RFQReceivedID != nil {
			out["rfq_id"] = procMsg.RFQReceivedID.Hex()
		}
		json.NewEncoder(w).Encode(out)
		return
	}

	// Build text content from the procurement message (strip HTML/CSS from email bodies).
	emailText := htmlToPlainText(procMsg.BodyText)
	if emailText == "" {
		emailText = htmlToPlainText(procMsg.BodyHTML)
	}

	// For WhatsApp messages, body_text is already plain text — htmlToPlainText is a no-op.
	// Build attachment data for LLM.
	var imageDataURIs []string
	var pdfBase64s []string
	for _, att := range procMsg.Attachments {
		if att.URL == "" {
			continue
		}
		diskPath := "." + att.URL
		data, readErr := os.ReadFile(diskPath)
		if readErr != nil || len(data) == 0 {
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

	llmProvider := strings.ToLower(store.Settings.RFQLLMProvider)
	var extracted rfqExtractResult
	_, legacyEndpoint := resolveExtractionEndpoint(llmProvider, &store.Settings)
	raw, llmErr := callLLMExtractRFQ(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, llmProvider, emailText, imageDataURIs, pdfBase64s, legacyEndpoint)
	if llmErr == nil {
		jsonStr := extractJSONFromLLMResponse(raw)
		json.Unmarshal([]byte(jsonStr), &extracted) //nolint:errcheck
	}

	// Customer find-or-create.
	lookupEmail := extracted.CustomerEmail
	if lookupEmail == "" {
		lookupEmail = procMsg.From
	}
	var customerID *primitive.ObjectID
	if customer, custErr := store.FindOrCreateCustomerFromRFQ(
		extracted.CustomerName, lookupEmail, extracted.CustomerPhone,
		extracted.CustomerVATNo, extracted.CustomerCompany,
		extracted.CustomerContactPerson, extracted.CustomerNationalAddress,
	); custErr != nil {
		log.Printf("procurement_messages: FindOrCreateCustomerFromRFQ error: %v", custErr)
	} else if customer != nil {
		customerID = &customer.ID
	}

	// Prefer company name; fall back to contact person, then sender.
	fromName := extracted.CustomerName
	if fromName == "" {
		fromName = extracted.CustomerContactPerson
	}
	if fromName == "" {
		fromName = procMsg.From
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

	source := procurementMessageSource(procMsg.Type)

	rfq := &models.RFQReceived{
		StoreID:                 storeObjID,
		FromPhone:               procMsg.From,
		FromName:                fromName,
		MessageType:             "text",
		TextContent:             emailText,
		Source:                  source,
		Status:                  "ready_to_send",
		Products:                products,
		Categories:              extracted.Categories,
		GeneralInstructions:     extracted.GeneralInstructions,
		CustomerID:              customerID,
		CustomerName:            extracted.CustomerName,
		CustomerContactPerson:   extracted.CustomerContactPerson,
		CustomerPhone:           extracted.CustomerPhone,
		CustomerEmail:           extracted.CustomerEmail,
		CustomerCompany:         extracted.CustomerCompany,
		CustomerVATNo:           extracted.CustomerVATNo,
		CustomerCRNo:            extracted.CustomerCRNo,
		CustomerNationalAddress: extracted.CustomerNationalAddress,
		ProcurementMessageID:    &procMsg.ID,
		ProcurementMessageCode:  procMsg.Code,
	}
	if err := models.CreateRFQReceived(rfq); err != nil {
		log.Printf("procurement_messages: failed to create RFQ: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create RFQ"})
		return
	}

	models.LinkProcurementMessageToRFQ(procMsg.ID, rfq.ID) //nolint:errcheck

	models.AppendRFQLog(storeObjID, rfq.ID, models.RFQActivityLog{
		Step:    "input_received",
		Message: fmt.Sprintf("RFQ created manually from procurement inbox (%s) from %s", procMsg.Type, procMsg.From),
		Icon:    "bi-inbox-fill", Color: "primary",
		Details: map[string]interface{}{"source": source, "from": procMsg.From, "procurement_message_id": procMsg.ID.Hex()},
	})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "rfq_id": rfq.ID.Hex(), "rfq_code": rfq.Code})
}

// ── ExtractProcurementMessageHandler ──────────────────────────────────────────
// POST /v1/procurement-messages/{id}/extract
// Accepts multipart form with optional "files" field for additional uploads.
// Required form fields: llm_provider, llm_model (store_id used to resolve API key).
// Uses the email body + saved attachments + uploaded files as input to the LLM.
// Returns the same rfqExtractResult shape as /v1/rfq-received/extract.
func ExtractProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		// also try plain form
		r.ParseForm() //nolint:errcheck
	}

	llmProvider := strings.ToLower(strings.TrimSpace(r.FormValue("llm_provider")))
	llmModel := strings.TrimSpace(r.FormValue("llm_model"))

	if llmProvider == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "llm_provider is required"})
		return
	}

	// Resolve API key from store settings (store_id required).
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		storeIDStr = r.FormValue("store_id")
	}
	storeObjID, storeErr := primitive.ObjectIDFromHex(storeIDStr)
	var llmAPIKey, llmEndpointURL string
	if storeErr == nil {
		if store, sErr := models.FindStoreByID(&storeObjID, bson.M{}); sErr == nil {
			llmAPIKey, llmEndpointURL = resolveExtractionEndpoint(llmProvider, &store.Settings)
		}
	}
	if llmAPIKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "No API key configured for the selected provider. Please add it under Store → AI Models."})
		return
	}

	procMsg, err := models.GetProcurementMessage(msgID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}

	// Build content from the email message itself.
	// Strip HTML/CSS from both body variants; prefer body_text, fall back to body_html.
	emailText := "Subject: " + procMsg.Subject
	if procMsg.From != "" {
		emailText += "\nFrom: " + procMsg.From
	}
	bodyText := htmlToPlainText(procMsg.BodyText)
	bodyHTML := htmlToPlainText(procMsg.BodyHTML)
	if bodyText != "" {
		emailText += "\n\n" + bodyText
	} else if bodyHTML != "" {
		emailText += "\n\n" + bodyHTML
	}

	var imageDataURIs []string
	var pdfBase64s []string
	var textParts []string
	if emailText != "" {
		textParts = append(textParts, emailText)
	}

	// Load existing saved attachments from disk.
	for _, att := range procMsg.Attachments {
		if att.URL == "" {
			continue
		}
		diskPath := "." + att.URL
		data, readErr := os.ReadFile(diskPath)
		if readErr != nil || len(data) == 0 {
			continue
		}
		ext := strings.ToLower(filepath.Ext(att.Filename))
		switch {
		case isRFQImageExt(ext):
			mime := rfqImageMime(ext, att.ContentType)
			imageDataURIs = append(imageDataURIs, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
		case ext == ".pdf":
			pdfBase64s = append(pdfBase64s, base64.StdEncoding.EncodeToString(data))
			if extracted := extractPDFText(data); extracted != "" {
				textParts = append(textParts, "=== "+att.Filename+" (PDF text) ===\n"+extracted)
			}
		case ext == ".xls" || ext == ".xlsx":
			if txt, exErr := excelToText(att.Filename, data); exErr == nil {
				textParts = append(textParts, txt)
			}
		case ext == ".docx":
			if txt, exErr := docxToText(att.Filename, data); exErr == nil {
				textParts = append(textParts, txt)
			}
		case ext == ".csv" || ext == ".txt":
			textParts = append(textParts, "=== "+att.Filename+" ===\n"+string(data))
		}
	}

	// Extract embedded base64 images from body_html (put there at ingestion time) for LLM vision.
	if procMsg.BodyHTML != "" && len(imageDataURIs) < 5 {
		remaining := procMsg.BodyHTML
		for len(imageDataURIs) < 5 {
			start := strings.Index(remaining, `src="data:image/`)
			if start < 0 {
				break
			}
			valStart := start + 5 // skip 'src="'
			end := strings.Index(remaining[valStart:], `"`)
			if end < 0 {
				break
			}
			dataURI := remaining[valStart : valStart+end]
			imageDataURIs = append(imageDataURIs, dataURI)
			remaining = remaining[valStart+end:]
		}
	}

	// Load additionally uploaded files (user may supply extra context).
	if r.MultipartForm != nil {
		for _, fh := range r.MultipartForm.File["files"] {
			f, ferr := fh.Open()
			if ferr != nil {
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			ct := strings.ToLower(fh.Header.Get("Content-Type"))
			switch {
			case ext == ".xlsx" || ext == ".xls" || strings.Contains(ct, "spreadsheet") || strings.Contains(ct, "excel"):
				if txt, exErr := excelToText(fh.Filename, data); exErr == nil {
					textParts = append(textParts, txt)
				}
			case ext == ".docx" || strings.Contains(ct, "wordprocessingml"):
				if txt, exErr := docxToText(fh.Filename, data); exErr == nil {
					textParts = append(textParts, txt)
				}
			case ext == ".pdf" || strings.Contains(ct, "pdf"):
				pdfBase64s = append(pdfBase64s, base64.StdEncoding.EncodeToString(data))
				if extracted := extractPDFText(data); extracted != "" {
					textParts = append(textParts, "=== "+fh.Filename+" (PDF text) ===\n"+extracted)
				}
			case ext == ".csv" || ext == ".txt" || strings.HasPrefix(ct, "text/"):
				textParts = append(textParts, "=== "+fh.Filename+" ===\n"+string(data))
			case isRFQImageExt(ext) || strings.HasPrefix(ct, "image/"):
				mime := rfqImageMime(ext, ct)
				imageDataURIs = append(imageDataURIs, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
			}
		}
	}

	combinedText := strings.Join(textParts, "\n\n")
	if len(imageDataURIs)+len(pdfBase64s) == 0 && combinedText == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no content to extract from"})
		return
	}

	usedModel := llmModel
	if usedModel == "" {
		usedModel = llmProvider
	}

	responseText, llmErr := callLLMExtractRFQ(llmAPIKey, llmModel, llmProvider, combinedText, imageDataURIs, pdfBase64s, llmEndpointURL)
	if llmErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": llmErr.Error()})
		return
	}

	jsonStr := extractJSONFromLLMResponse(responseText)
	var result rfqExtractResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		result = rfqExtractResult{TextContent: jsonStr, LLMModel: usedModel}
	} else {
		result.LLMModel = usedModel
	}
	json.NewEncoder(w).Encode(result)
}

// ExtractQuotationHandler handles POST /v1/procurement-messages/{id}/extract-quotation
// Extracts supplier prices from a procurement message (PDF/text attachments) and returns them.
// Unlike /extract which extracts customer RFQ data, this uses the supplier price extraction prompt.
// If the message is already linked to an RFQ, the extracted prices are saved to that RFQ's SupplierReply.
func ExtractQuotationHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
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
	// Allow overriding the LLM provider/model via query params (for the extraction modal).
	providerOverride := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("llm_provider")))
	modelOverride := strings.TrimSpace(r.URL.Query().Get("llm_model"))

	msg, err := models.GetProcurementMessage(msgID)
	if err != nil {
		http.Error(w, `{"error":"message not found"}`, http.StatusNotFound)
		return
	}

	// Build text from body + attachment text extraction.
	// Also collect PDFs as base64 for vision-capable providers (Gemini, Anthropic).
	var textParts []string
	var pdfBase64s []string
	if msg.BodyText != "" {
		textParts = append(textParts, msg.BodyText)
	}
	for _, att := range msg.Attachments {
		if att.URL == "" {
			continue
		}
		data, readErr := os.ReadFile("." + att.URL)
		if readErr != nil || len(data) == 0 {
			log.Printf("ExtractQuotationHandler: cannot read attachment %q at %q: %v", att.Filename, "."+att.URL, readErr)
			continue
		}
		ext := strings.ToLower(filepath.Ext(att.Filename))
		switch ext {
		case ".pdf":
			// Always collect as base64 for vision providers.
			pdfBase64s = append(pdfBase64s, base64.StdEncoding.EncodeToString(data))
			// Also try text extraction for text-only providers.
			if extracted := extractPDFText(data); extracted != "" {
				textParts = append(textParts, "=== "+att.Filename+" ===\n"+extracted)
			}
		case ".xlsx", ".xls":
			if txt, exErr := excelToText(att.Filename, data); exErr == nil {
				textParts = append(textParts, txt)
			}
		case ".docx":
			if txt, exErr := docxToText(att.Filename, data); exErr == nil {
				textParts = append(textParts, txt)
			}
		case ".csv", ".txt":
			textParts = append(textParts, string(data))
		}
	}
	combinedText := strings.Join(textParts, "\n\n")
	if combinedText == "" && len(pdfBase64s) == 0 {
		http.Error(w, `{"error":"no text content found in message or attachments"}`, http.StatusBadRequest)
		return
	}

	// Load RFQ products for context (better price matching) if linked to an RFQ.
	var rfqProducts []models.RFQProduct
	if msg.LinkedRFQReceivedID != nil {
		if rfq, rfqErr := models.FindRFQReceivedByID(*msg.LinkedRFQReceivedID, storeObjID); rfqErr == nil && rfq != nil {
			rfqProducts = rfq.Products
		}
	}

	analysis := analyzeSupplierReply(store, combinedText, pdfBase64s, rfqProducts, providerOverride, modelOverride)

	// Save extracted prices to the linked RFQ's SupplierReply (if linked).
	if msg.LinkedRFQReceivedID != nil && (analysis.IsQuotation || len(analysis.Prices) > 0) {
		rfq, rfqErr := models.FindRFQReceivedByID(*msg.LinkedRFQReceivedID, storeObjID)
		if rfqErr == nil && rfq != nil {
			supplierPhone := ""
			if msg.Type == "whatsapp" {
				supplierPhone = strings.TrimPrefix(msg.From, "+")
			}
			// Look for an existing SupplierReply linked to this procurement message.
			saved := false
			for _, sr := range rfq.SupplierReplies {
				if sr.ProcurementMessageID != nil && *sr.ProcurementMessageID == msgID {
					models.UpdateSupplierReplyPrices(storeObjID, rfq.ID, sr.ID, analysis.Prices, true, "done", "") //nolint:errcheck
					saved = true
					break
				}
			}
			if !saved {
				reply := models.SupplierReply{
					SupplierName:           msg.From,
					SupplierPhone:          supplierPhone,
					ReceivedAt:             time.Now(),
					RawText:                combinedText,
					IsQuotation:            true,
					GeneralNotes:           analysis.GeneralNotes,
					Prices:                 analysis.Prices,
					ExtractionStatus:       "done",
					Source:                 msg.Type,
					ProcurementMessageID:   &msgID,
					ProcurementMessageCode: msg.Code,
				}
				models.AddSupplierReplyToRFQ(storeObjID, rfq.ID, reply) //nolint:errcheck
			}
		}
	}

	// If no RFQ code was found in the document, try to suggest one by matching
	// the supplier's phone number to RFQs we previously forwarded to them.
	suggestedRFQCode := ""
	suggestedRFQID := ""
	if analysis.RFQCode == "" && msg.LinkedRFQReceivedID == nil && msg.From != "" {
		phone := strings.TrimPrefix(msg.From, "+")
		if rfqs, _ := models.FindRFQsForwardedToPhone(storeObjID, phone, 90*24*time.Hour, 5); len(rfqs) > 0 {
			// Pick the best match: if part numbers overlap, prefer that RFQ; otherwise take the newest.
			extractedPartNos := map[string]bool{}
			for _, p := range analysis.Prices {
				if p.PartNo != "" {
					extractedPartNos[strings.ToLower(strings.TrimSpace(p.PartNo))] = true
				}
			}
			best := &rfqs[0]
			if len(extractedPartNos) > 0 {
				for i := range rfqs {
					for _, rp := range rfqs[i].Products {
						if extractedPartNos[strings.ToLower(strings.TrimSpace(rp.PartNo))] {
							best = &rfqs[i]
							break
						}
					}
				}
			}
			suggestedRFQCode = best.Code
			suggestedRFQID = best.ID.Hex()
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":              "ok",
		"is_quotation":        analysis.IsQuotation,
		"rfq_code":            analysis.RFQCode,
		"prices":              analysis.Prices,
		"price_count":         len(analysis.Prices),
		"general_notes":       analysis.GeneralNotes,
		"suggested_rfq_code":  suggestedRFQCode,
		"suggested_rfq_id":    suggestedRFQID,
	})
}

// RetryProcurementMessageAttachmentsHandler re-fetches Zoho attachments for a
// message that was saved with attachment_missing=true.
// POST /v1/procurement-messages/{id}/retry-attachments
func RetryProcurementMessageAttachmentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	msg, err := models.GetProcurementMessage(msgID)
	if err != nil || msg == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}
	if msg.Provider != "zoho" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "retry only supported for zoho emails"})
		return
	}
	if msg.ExternalID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no external_id on message"})
		return
	}

	store, _, err := rfqEmailGetStore(msg.StoreID.Hex())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}

	var zohoAcct *models.RFQEmailAccount
	for i := range store.Settings.RFQEmailAccounts {
		a := &store.Settings.RFQEmailAccounts[i]
		if a.Provider == "zoho" && (a.ZohoAccessToken != "" || a.ZohoRefreshToken != "") {
			zohoAcct = a
			break
		}
	}
	if zohoAcct == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no zoho account configured for this store"})
		return
	}

	accessToken, err := ensureZohoToken(msg.StoreID, *zohoAcct)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "zoho token refresh failed: " + err.Error()})
		return
	}

	mailBase := zohoMailBase(zohoAcct.ZohoAccountsServer)
	accountID, err := fetchZohoAccountID(accessToken, mailBase)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get zoho account id: " + err.Error()})
		return
	}

	msgFolderID := fetchZohoMessageFolderID(accessToken, accountID, msg.ExternalID, mailBase)
	inboxFolderID := fetchZohoInboxFolderID(accessToken, accountID, mailBase)

	candidates := []string{}
	if msgFolderID != "" {
		candidates = append(candidates, msgFolderID)
	}
	if inboxFolderID != "" && inboxFolderID != msgFolderID {
		candidates = append(candidates, inboxFolderID)
	}
	candidates = append(candidates, "")

	atts := fetchZohoAttachments(accessToken, accountID, candidates, msg.ExternalID, mailBase, msg.StoreID.Hex(), zohoAcct.IMAPHost, zohoAcct.IMAPUsername, zohoAcct.IMAPPassword, msg.Subject)
	hasURL := false
	for _, a := range atts {
		if a.URL != "" {
			hasURL = true
			break
		}
	}
	if !hasURL {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "attachments could not be downloaded from Zoho — the file may no longer be accessible",
		})
		return
	}

	if err := models.UpdateProcurementMessageAttachments(msgID, atts); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to update message: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "attachments": atts})
}

// UploadProcurementAttachmentHandler lets a user manually upload an attachment
// for an email that has attachment_missing=true (e.g. when the Zoho API is broken).
// POST /v1/procurement-messages/{id}/upload-attachment
// Multipart form field: "file" (any file type, max 20 MB).
func UploadProcurementAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	msg, err := models.GetProcurementMessage(msgID)
	if err != nil || msg == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large (max 20 MB)"})
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no file uploaded"})
		return
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read file"})
		return
	}
	defer f.Close()

	uploadDir := fmt.Sprintf("./attachments/%s/procurement/%s", msg.StoreID.Hex(), msgID.Hex())
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create upload directory"})
		return
	}

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	saveName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	savePath := filepath.Join(uploadDir, saveName)
	out, err := os.Create(savePath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save file"})
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, f); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to write file"})
		return
	}

	fileURL := fmt.Sprintf("/attachments/%s/procurement/%s/%s", msg.StoreID.Hex(), msgID.Hex(), saveName)
	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	att := models.ProcurementAttachment{
		Filename:    fh.Filename,
		ContentType: contentType,
		Size:        fh.Size,
		URL:         fileURL,
	}

	// Merge with any existing non-missing attachments.
	existing := msg.Attachments
	existing = append(existing, att)

	if err := models.UpdateProcurementMessageAttachments(msgID, existing); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to update message"})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "attachments": existing})
}

// ── ProcurementExtractTestHandler ─────────────────────────────────────────────
// POST /v1/procurement-extract-test?store_id=<id>
// Standalone LLM extraction test — accepts free-form text + file uploads.
// Required form fields: llm_provider, llm_model.
// Required query param: store_id (API key resolved from store settings).
// Optional: text (free text input), files[] (images / PDFs / spreadsheets / CSV / TXT).
// Returns the same rfqExtractResult shape.
func ProcurementExtractTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		r.ParseForm() //nolint:errcheck
	}

	llmProvider := strings.ToLower(strings.TrimSpace(r.FormValue("llm_provider")))
	llmModel := strings.TrimSpace(r.FormValue("llm_model"))
	freeText := strings.TrimSpace(r.FormValue("text"))

	if llmProvider == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "llm_provider is required"})
		return
	}

	// Resolve API key and endpoint URL from store settings
	storeIDStr := strings.TrimSpace(r.URL.Query().Get("store_id"))
	var llmAPIKey, llmEndpointURL string
	if storeIDStr != "" {
		if storeObjID, sErr := primitive.ObjectIDFromHex(storeIDStr); sErr == nil {
			if store, sErr2 := models.FindStoreByID(&storeObjID, bson.M{}); sErr2 == nil && store != nil {
				llmAPIKey, llmEndpointURL = resolveExtractionEndpoint(llmProvider, &store.Settings)
			}
		}
	}
	if llmAPIKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no API key found for provider '" + llmProvider + "' — please add it in Store → AI Models tab"})
		return
	}

	var imageDataURIs []string
	var pdfBase64s []string
	var textParts []string
	if freeText != "" {
		textParts = append(textParts, freeText)
	}

	if r.MultipartForm != nil {
		for _, fh := range r.MultipartForm.File["files"] {
			f, ferr := fh.Open()
			if ferr != nil {
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			ext := strings.ToLower(filepath.Ext(fh.Filename))
			ct := strings.ToLower(fh.Header.Get("Content-Type"))
			switch {
			case ext == ".xlsx" || ext == ".xls" || strings.Contains(ct, "spreadsheet") || strings.Contains(ct, "excel"):
				if txt, exErr := excelToText(fh.Filename, data); exErr == nil {
					textParts = append(textParts, txt)
				}
			case ext == ".docx" || strings.Contains(ct, "wordprocessingml"):
				if txt, exErr := docxToText(fh.Filename, data); exErr == nil {
					textParts = append(textParts, txt)
				}
			case ext == ".pdf" || strings.Contains(ct, "pdf"):
				pdfBase64s = append(pdfBase64s, base64.StdEncoding.EncodeToString(data))
				if extracted := extractPDFText(data); extracted != "" {
					textParts = append(textParts, "=== "+fh.Filename+" (PDF text) ===\n"+extracted)
				}
			case ext == ".csv" || ext == ".txt" || strings.HasPrefix(ct, "text/"):
				textParts = append(textParts, "=== "+fh.Filename+" ===\n"+string(data))
			case isRFQImageExt(ext) || strings.HasPrefix(ct, "image/"):
				mime := rfqImageMime(ext, ct)
				imageDataURIs = append(imageDataURIs, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(data))
			}
		}
	}

	combinedText := strings.Join(textParts, "\n\n")
	if len(imageDataURIs)+len(pdfBase64s) == 0 && combinedText == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no content to extract from — provide text or upload files"})
		return
	}

	// If the PDF has no extractable text and the selected provider is not vision-capable,
	// reject with a clear message rather than silently switching or hallucinating.
	if combinedText == "" && len(pdfBase64s) > 0 && llmProvider != "gemini" && llmProvider != "anthropic" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "The PDF has no extractable text (it may be Arabic, scanned, or image-based). Please select Gemini or Anthropic as the provider — they can read PDFs visually."})
		return
	}

	usedModel := llmModel
	if usedModel == "" {
		usedModel = llmProvider
	}

	responseText, llmErr := callLLMExtractRFQ(llmAPIKey, llmModel, llmProvider, combinedText, imageDataURIs, pdfBase64s, llmEndpointURL)
	if llmErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": llmErr.Error()})
		return
	}

	jsonStr := extractJSONFromLLMResponse(responseText)
	var result rfqExtractResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		result = rfqExtractResult{TextContent: responseText, LLMModel: usedModel}
	} else {
		result.LLMModel = usedModel
	}
	json.NewEncoder(w).Encode(result)
}

// LinkAsQuotationHandler handles POST /v1/procurement-messages/{id}/link-as-quotation
// Body (JSON, optional): { "rfq_id": "<hex>", "rfq_code": "<str>", "unlink": false }
// If rfq_id/rfq_code are omitted, tries auto-match by RFQ code in text then by supplier phone.
// The message is marked is_supplier_quotation=true and a SupplierReply is upserted in the RFQ.
func LinkAsQuotationHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"}) //nolint:errcheck
		return
	}

	msg, err := models.GetProcurementMessage(msgID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"}) //nolint:errcheck
		return
	}

	var body struct {
		RFQIDHex string `json:"rfq_id"`
		RFQCode  string `json:"rfq_code"`
		Unlink   bool   `json:"unlink"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck

	if body.Unlink {
		if err := models.UnlinkMessageAsQuotation(msgID); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"success": true}) //nolint:errcheck
		return
	}

	storeID := msg.StoreID
	var rfq *models.RFQReceived

	if body.RFQIDHex != "" {
		if rfqObjID, e := primitive.ObjectIDFromHex(body.RFQIDHex); e == nil {
			rfq, _ = models.FindRFQReceivedByID(rfqObjID, storeID)
		}
	}
	if rfq == nil && body.RFQCode != "" {
		rfq, _ = models.FindRFQByCode(storeID, body.RFQCode)
	}
	if rfq == nil {
		bodyText := msg.BodyText + " " + msg.Subject
		for _, a := range msg.Attachments {
			bodyText += " " + a.Filename
		}
		if code := extractRFQCodeSimple(bodyText); code != "" {
			rfq, _ = models.FindRFQByCode(storeID, code)
		}
	}
	if rfq == nil && msg.From != "" {
		phone := strings.TrimPrefix(msg.From, "+")
		rfqs, _ := models.FindRFQsForwardedToPhone(storeID, phone, 90*24*time.Hour, 1)
		if len(rfqs) > 0 {
			rfq = &rfqs[0]
		}
	}

	var rfqIDPtr *primitive.ObjectID
	var rfqCode string
	if rfq != nil {
		id := rfq.ID
		rfqIDPtr = &id
		rfqCode = rfq.Code
	}
	if err := models.LinkMessageAsQuotation(msgID, rfqIDPtr, rfqCode); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
		return
	}

	matchedRFQCode := ""
	matchedRFQID := ""
	if rfq != nil {
		matchedRFQCode = rfq.Code
		matchedRFQID = rfq.ID.Hex()

		supplierPhone := ""
		supplierEmail := ""
		if msg.Type == "whatsapp" {
			supplierPhone = strings.TrimPrefix(msg.From, "+")
		} else {
			supplierEmail = msg.From
		}

		existing := false
		for _, sr := range rfq.SupplierReplies {
			if sr.ProcurementMessageID != nil && *sr.ProcurementMessageID == msgID {
				existing = true
				break
			}
		}
		if !existing {
			reply := models.SupplierReply{
				SupplierName:           msg.From,
				SupplierPhone:          supplierPhone,
				SupplierEmail:          supplierEmail,
				ReceivedAt:             time.Now(),
				RawText:                msg.BodyText,
				IsQuotation:            true,
				ExtractionStatus:       "pending",
				Source:                 msg.Type,
				ProcurementMessageID:   &msgID,
				ProcurementMessageCode: msg.Code,
			}
			for _, a := range msg.Attachments {
				if a.URL != "" {
					reply.MediaURLs = append(reply.MediaURLs, a.URL)
				}
			}
			_ = models.AddSupplierReplyToRFQ(storeID, rfq.ID, reply)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
		"success":          true,
		"is_quotation":     true,
		"matched_rfq_id":   matchedRFQID,
		"matched_rfq_code": matchedRFQCode,
	})
}

// ResolveProcurementSendersHandler handles POST /v1/procurement-messages/resolve-senders
// Backfills sender_name / sender_type on all inbound messages that have no sender resolved.
// Safe to call multiple times — only touches messages with empty sender_name.
func ResolveProcurementSendersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	if storeIDStr == "" {
		var body struct {
			StoreID string `json:"store_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		storeIDStr = body.StoreID
	}
	storeID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	updated, err := models.BackfillProcurementSenders(storeID, 0)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "updated": updated})
}

// extractRFQCodeSimple scans free text for an "RFQ-XXXX" style code and returns it.
func extractRFQCodeSimple(text string) string {
	return extractRFQCodeFromText(text, "RFQ")
}

// ReplyToProcurementMessageHandler handles POST /v1/procurement-messages/{id}/reply
// Sends a free-text WhatsApp reply to the sender of the given procurement message,
// then saves the outbound message as a new ProcurementMessage (direction: "out").
func ReplyToProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}

	var body struct {
		Text    string `json:"text"`
		StoreID string `json:"store_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "text is required"})
		return
	}

	// Allow store_id from query param as fallback
	if body.StoreID == "" {
		body.StoreID = r.URL.Query().Get("store_id")
	}

	origMsg, err := models.GetProcurementMessage(msgID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}

	recipientPhone := origMsg.From
	if recipientPhone == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "original message has no sender phone"})
		return
	}

	storeIDStr := body.StoreID
	if storeIDStr == "" {
		storeIDStr = origMsg.StoreID.Hex()
	}

	// Prefer Meta WhatsApp Cloud API (store_rfq → bot) then fall back to Evolution API
	metaPhoneID, metaToken := rfqMetaConfig(storeIDStr, "store_rfq")
	if metaPhoneID == "" || metaToken == "" {
		metaPhoneID, metaToken = rfqMetaConfig(storeIDStr, "bot")
	}
	var replyWamid string
	if metaPhoneID != "" && metaToken != "" {
		var err error
		replyWamid, err = metaSendText(metaPhoneID, metaToken, recipientPhone, body.Text)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": "WhatsApp API error: " + err.Error()})
			return
		}
	} else {
		evoURL, evoKey, evoInstance := evoConfigFromStore(storeIDStr)
		base := strings.TrimRight(evoURL, "/")
		if strings.HasSuffix(recipientPhone, "@lid") {
			if resolved := resolveLIDPhone(base, evoKey, evoInstance, recipientPhone); resolved != "" {
				recipientPhone = resolved
			}
		}
		payload, _ := json.Marshal(map[string]string{"number": recipientPhone, "text": body.Text})
		respBody, status, err := evoCall("POST",
			fmt.Sprintf("%s/message/sendText/%s", base, evoInstance),
			evoKey, payload)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]string{"error": "Evolution API unreachable: " + err.Error()})
			return
		}
		if status != http.StatusOK && status != http.StatusCreated {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprintf(w, `{"error":"Evolution API error","detail":%s}`, string(respBody))
			return
		}
	}

	// Save the outbound message
	now := time.Now()
	storeObjID := origMsg.StoreID
	if sid, err2 := primitive.ObjectIDFromHex(storeIDStr); err2 == nil {
		storeObjID = sid
	}
	saved := saveProcurementWhatsAppMessage(
		storeObjID, "out",
		"", // from (our side — empty for outbound)
		[]string{origMsg.From},
		body.Text, "text",
		origMsg.WABAPhoneNumberID,
		replyWamid,
		nil, false, nil, &now,
	)

	resp := map[string]interface{}{"success": true}
	if saved != nil {
		resp["message_id"] = saved.ID.Hex()
		resp["code"] = saved.Code
	}
	json.NewEncoder(w).Encode(resp)
}


// GetProcurementDiskUsageHandler returns the total disk space used by procurement message attachments.
// GET /v1/procurement-messages/disk-usage?store_id=X
func GetProcurementDiskUsageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeID := r.URL.Query().Get("store_id")
	if storeID == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"store_id required"}`)
		return
	}

	dir := "./attachments/" + storeID
	var totalBytes int64
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			totalBytes += info.Size()
		}
		return nil
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_bytes": totalBytes,
		"formatted":   formatStorageBytes(totalBytes),
	})
}

func formatStorageBytes(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
	}
}

// ReplyToEmailProcurementMessageHandler handles POST /v1/procurement-messages/{id}/email-reply
// Accepts multipart/form-data: fields store_id, subject, body, to, from + optional file[] attachments.
func ReplyToEmailProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	msgID, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}

	// Accept both multipart/form-data and legacy JSON
	var body struct {
		StoreID string
		Subject string
		Body    string
		To      string
		From    string
	}
	var attachments []emailAttachment

	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "multipart/form-data") {
		const maxSize = 25 << 20 // 25 MB
		if err := r.ParseMultipartForm(maxSize); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to parse form"})
			return
		}
		body.StoreID = r.FormValue("store_id")
		body.Subject = r.FormValue("subject")
		body.Body = r.FormValue("body")
		body.To = r.FormValue("to")
		body.From = r.FormValue("from")
		if r.MultipartForm != nil {
			for _, fhs := range r.MultipartForm.File {
				for _, fh := range fhs {
					f, ferr := fh.Open()
					if ferr != nil {
						continue
					}
					data, _ := io.ReadAll(f)
					f.Close()
					ct := fh.Header.Get("Content-Type")
					if ct == "" {
						ct = "application/octet-stream"
					}
					attachments = append(attachments, emailAttachment{
						Filename:    fh.Filename,
						ContentType: ct,
						Data:        data,
					})
				}
			}
		}
	} else {
		var jsonBody struct {
			StoreID string `json:"store_id"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			To      string `json:"to"`
			From    string `json:"from"`
		}
		if err := json.NewDecoder(r.Body).Decode(&jsonBody); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		body.StoreID = jsonBody.StoreID
		body.Subject = jsonBody.Subject
		body.Body = jsonBody.Body
		body.To = jsonBody.To
		body.From = jsonBody.From
	}

	if strings.TrimSpace(body.Body) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "body is required"})
		return
	}
	if body.StoreID == "" {
		body.StoreID = r.URL.Query().Get("store_id")
	}

	origMsg, err := models.GetProcurementMessage(msgID)
	if err != nil || origMsg == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}
	if origMsg.Type != "email" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "not an email message"})
		return
	}

	// Prefer explicit To from request, fall back to origMsg.From
	replyTo := strings.TrimSpace(body.To)
	if replyTo == "" {
		replyTo = origMsg.From
	}
	if replyTo == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "original message has no sender email"})
		return
	}
	// Decode HTML entities (&lt; → <, &gt; → >) before parsing
	replyTo = strings.ReplaceAll(replyTo, "&lt;", "<")
	replyTo = strings.ReplaceAll(replyTo, "&gt;", ">")
	replyTo = strings.ReplaceAll(replyTo, "&amp;", "&")
	// Extract bare email from "Name <email@domain.com>" format
	if addr, err := mail.ParseAddress(replyTo); err == nil {
		replyTo = addr.Address
	}
	// Validate: must contain @ and a domain
	if !strings.Contains(replyTo, "@") || strings.HasSuffix(replyTo, "@") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid recipient email address. Please enter a valid email in the To field."})
		return
	}

	storeIDStr := body.StoreID
	if storeIDStr == "" {
		storeIDStr = origMsg.StoreID.Hex()
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}

	var sendErr error
	var fromAddr string

	smtpUser := store.Settings.RFQZohoSMTPUsername
	smtpPass := store.Settings.RFQZohoSMTPPassword

	subject := strings.TrimSpace(body.Subject)
	if subject == "" {
		orig := strings.TrimSpace(origMsg.Subject)
		if strings.HasPrefix(strings.ToLower(orig), "re:") {
			subject = orig
		} else {
			subject = "Re: " + orig
		}
	}

	inReplyTo := origMsg.EmailMessageID // for threading: In-Reply-To / References headers

	// Determine fromAddr first, then build MIME and send
	var smtpSendHost string
	var smtpSendPort int
	var smtpSendUser, smtpSendPass string

	if smtpUser != "" && smtpPass != "" {
		fromAddr = smtpUser
		smtpSendHost = "smtppro.zoho.in"
		smtpSendPort = 465
		smtpSendUser = smtpUser
		smtpSendPass = smtpPass
	} else if store.Settings.RFQIMAPHost != "" && store.Settings.RFQIMAPUsername != "" && store.Settings.RFQIMAPPassword != "" {
		fromAddr = store.Settings.RFQIMAPUsername
		smtpSendHost, smtpSendPort, _ = smtpHostFromIMAPHost(store.Settings.RFQIMAPHost)
		if store.Settings.RFQIMAPPort == 993 || store.Settings.RFQIMAPPort == 465 {
			smtpSendPort = 465
		}
		smtpSendUser = store.Settings.RFQIMAPUsername
		smtpSendPass = store.Settings.RFQIMAPPassword
	} else {
		for _, acct := range store.Settings.RFQEmailAccounts {
			if acct.SMTPUsername != "" && acct.SMTPPassword != "" {
				fromAddr = acct.SMTPUsername
				smtpSendHost = acct.SMTPHost
				smtpSendPort = acct.SMTPPort
				if smtpSendHost == "" && acct.IMAPHost != "" {
					smtpSendHost, smtpSendPort, _ = smtpHostFromIMAPHost(acct.IMAPHost)
				}
				if smtpSendPort == 0 {
					smtpSendPort = 465
				}
				smtpSendUser = acct.SMTPUsername
				smtpSendPass = acct.SMTPPassword
				break
			}
			if acct.IMAPHost != "" && acct.IMAPUsername != "" && acct.IMAPPassword != "" {
				fromAddr = acct.IMAPUsername
				smtpSendHost, smtpSendPort, _ = smtpHostFromIMAPHost(acct.IMAPHost)
				if smtpSendPort == 587 {
					smtpSendPort = 465
				}
				smtpSendUser = acct.IMAPUsername
				smtpSendPass = acct.IMAPPassword
				break
			}
		}
		if fromAddr == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "No outgoing email configured. Add IMAP credentials (or Zoho SMTP) in Store Settings → Procurement → Email"})
			return
		}
	}

	// Allow frontend to override the From address
	if override := strings.TrimSpace(body.From); override != "" {
		if addr, err := mail.ParseAddress(override); err == nil {
			fromAddr = addr.Address
		} else {
			fromAddr = override
		}
	}

	// Build and send
	// Generate a Message-ID so customers' replies can be detected as replies to our emails.
	now := time.Now()
	outMsgID := primitive.NewObjectID()
	outEmailMsgID := fmt.Sprintf("<%s.%d@startpos.local>", outMsgID.Hex(), now.UnixNano())

	// Append default email signature if one is configured
	emailBody := body.Body
	for _, sig := range store.Settings.EmailSignatures {
		if sig.IsDefault && strings.TrimSpace(sig.Content) != "" {
			emailBody = emailBody + "\n\n--\n" + sig.Content
			break
		}
	}

	msgBytes := buildMIMEReplyFull(fromAddr, replyTo, subject, emailBody, inReplyTo, outEmailMsgID, attachments)
	sendErr = sendSMTPRaw(smtpSendHost, smtpSendPort, smtpSendUser, smtpSendPass, fromAddr, replyTo, msgBytes)

	if sendErr != nil {
		log.Printf("email-reply: SMTP error: %v", sendErr)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to send email: " + sendErr.Error()})
		return
	}

	// Record the outbound message.

	// Save attachment files to disk so they can be viewed/downloaded later.
	var savedAtts []models.ProcurementAttachment
	if len(attachments) > 0 {
		uploadDir := fmt.Sprintf("./attachments/%s/procurement/%s", storeObjID.Hex(), outMsgID.Hex())
		if err := os.MkdirAll(uploadDir, os.ModePerm); err == nil {
			for _, att := range attachments {
				ext := strings.ToLower(filepath.Ext(att.Filename))
				saveName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
				savePath := filepath.Join(uploadDir, saveName)
				if werr := os.WriteFile(savePath, att.Data, 0644); werr == nil {
					savedAtts = append(savedAtts, models.ProcurementAttachment{
						Filename:    att.Filename,
						ContentType: att.ContentType,
						Size:        int64(len(att.Data)),
						URL:         fmt.Sprintf("/attachments/%s/procurement/%s/%s", storeObjID.Hex(), outMsgID.Hex(), saveName),
					})
				}
			}
		}
	}

	out := &models.ProcurementMessage{
		ID:                    outMsgID,
		StoreID:               storeObjID,
		Type:                  "email",
		Direction:             "out",
		Provider:              origMsg.Provider,
		From:                  fromAddr,
		To:                    []string{replyTo},
		Subject:               subject,
		BodyText:              emailBody,
		EmailMessageID:        outEmailMsgID,
		Attachments:           savedAtts,
		CreatedAt:             now,
		MessageDate:           &now,
		LinkedRFQReceivedID:   origMsg.RFQReceivedID,
		LinkedRFQReceivedCode: origMsg.RFQReceivedCode,
	}
	out.Code = models.MakeProcurementMessageCode(storeObjID, "email")
	_ = models.SaveProcurementMessage(out)

	json.NewEncoder(w).Encode(map[string]interface{}{"status": "sent", "to": replyTo})
}

// SendNewEmailHandler sends a new (non-reply) email to an arbitrary address.
// POST /v1/procurement-email-send?store_id=
// Body: { "to": "...", "subject": "...", "body": "...", "store_id": "..." }
func SendNewEmailHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var body struct {
		StoreID string `json:"store_id"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	if body.StoreID == "" {
		body.StoreID = r.URL.Query().Get("store_id")
	}
	if strings.TrimSpace(body.To) == "" || strings.TrimSpace(body.Body) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "to and body are required"})
		return
	}

	storeObjID, err := primitive.ObjectIDFromHex(body.StoreID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}

	var sendErr error
	var fromAddr string

	smtpUser := store.Settings.RFQZohoSMTPUsername
	smtpPass := store.Settings.RFQZohoSMTPPassword

	subject := strings.TrimSpace(body.Subject)
	if subject == "" {
		subject = "Message from StartPOS"
	}

	if smtpUser != "" && smtpPass != "" {
		fromAddr = smtpUser
		sendErr = sendViaZohoSMTPReply(smtpUser, smtpPass, smtpUser, body.To, subject, body.Body, "")
	} else if store.Settings.RFQIMAPHost != "" && store.Settings.RFQIMAPUsername != "" && store.Settings.RFQIMAPPassword != "" {
		fromAddr = store.Settings.RFQIMAPUsername
		sendErr = sendViaIMAPCredentialsSMTP(
			store.Settings.RFQIMAPHost,
			store.Settings.RFQIMAPPort,
			store.Settings.RFQIMAPUsername,
			store.Settings.RFQIMAPPassword,
			fromAddr, body.To, subject, body.Body, "",
		)
	} else {
		// Check per-account SMTP or IMAP credentials
		for _, acct := range store.Settings.RFQEmailAccounts {
			if acct.SMTPUsername != "" && acct.SMTPPassword != "" {
				fromAddr = acct.SMTPUsername
				smtpHost := acct.SMTPHost
				smtpPort := acct.SMTPPort
				if smtpHost == "" && acct.IMAPHost != "" {
					smtpHost, smtpPort, _ = smtpHostFromIMAPHost(acct.IMAPHost)
				}
				if smtpPort == 0 {
					smtpPort = 465
				}
				sendErr = sendViaIMAPCredentialsSMTP(smtpHost, smtpPort, acct.SMTPUsername, acct.SMTPPassword, fromAddr, body.To, subject, body.Body, "")
				break
			}
			if acct.IMAPHost != "" && acct.IMAPUsername != "" && acct.IMAPPassword != "" {
				fromAddr = acct.IMAPUsername
				port := acct.IMAPPort
				if port == 0 {
					port = 993
				}
				sendErr = sendViaIMAPCredentialsSMTP(acct.IMAPHost, port, acct.IMAPUsername, acct.IMAPPassword, fromAddr, body.To, subject, body.Body, "")
				break
			}
		}
		if fromAddr == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "No outgoing email configured in Store Settings → Procurement → Email"})
			return
		}
	}

	if sendErr != nil {
		log.Printf("send-email: SMTP error: %v", sendErr)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to send email: " + sendErr.Error()})
		return
	}

	now := time.Now()
	out := &models.ProcurementMessage{
		StoreID:     storeObjID,
		Type:        "email",
		Direction:   "out",
		Provider:    "imap",
		From:        fromAddr,
		To:          []string{body.To},
		Subject:     subject,
		BodyText:    body.Body,
		CreatedAt:   now,
		MessageDate: &now,
	}
	out.Code = models.MakeProcurementMessageCode(storeObjID, "email")
	_ = models.SaveProcurementMessage(out)

	json.NewEncoder(w).Encode(map[string]interface{}{"status": "sent", "to": body.To})
}

// ProcurementEmailRFQHistoryHandler handles GET /v1/procurement-rfq-history
// Returns customer RFQs and supplier RFQs linked to a given email address.
func ProcurementEmailRFQHistoryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	storeIDStr := r.URL.Query().Get("store_id")
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if storeIDStr == "" || email == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "store_id and email required"})
		return
	}
	storeID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}
	// Strip "Name <email>" if present
	if addr, parseErr := mail.ParseAddress(email); parseErr == nil {
		email = addr.Address
	}
	email = strings.ToLower(email)

	customerRFQs, _ := models.FindRecentRFQsByEmail(storeID, email, 365*24*time.Hour, 10)
	supplierRFQs, _ := models.FindRFQsBySupplierEmail(storeID, email, 10)
	if customerRFQs == nil {
		customerRFQs = []models.RFQReceived{}
	}
	if supplierRFQs == nil {
		supplierRFQs = []models.RFQReceived{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"customer_rfqs": customerRFQs,
		"supplier_rfqs": supplierRFQs,
	})
}
