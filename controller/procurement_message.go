package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
func DeleteProcurementMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id, err := primitive.ObjectIDFromHex(mux.Vars(r)["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	// Delete disk files before removing the DB record.
	if msg, _ := models.GetProcurementMessage(id); msg != nil {
		deleteAttachmentDirs([]models.ProcurementMessage{*msg})
	}
	if err := models.DeleteProcurementMessage(id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
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
func saveProcurementEmailMessage(storeID primitive.ObjectID, direction, provider, from string, to []string, subject, bodyText, bodyHTML, externalID string, attachments []models.ProcurementAttachment, attachmentMissing, processedAsRFQ bool, rfqID *primitive.ObjectID, messageDate *time.Time) *models.ProcurementMessage {
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
func saveProcurementWhatsAppMessage(storeID primitive.ObjectID, direction, from string, to []string, bodyText, waMessageType, wabaPNID string, attachments []models.ProcurementAttachment, processedAsRFQ bool, rfqID *primitive.ObjectID, messageDate *time.Time) *models.ProcurementMessage {
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

	// Build text content from the procurement message.
	emailText := procMsg.BodyText
	if emailText == "" {
		emailText = procMsg.BodyHTML
	}

	// For WhatsApp messages, body_text is already the text content.
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
	raw, llmErr := callLLMExtractRFQ(store.Settings.RFQLLMAPIKey, store.Settings.RFQLLMModel, llmProvider, emailText, imageDataURIs, pdfBase64s)
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
	); custErr != nil {
		log.Printf("procurement_messages: FindOrCreateCustomerFromRFQ error: %v", custErr)
	} else if customer != nil {
		customerID = &customer.ID
	}

	fromName := extracted.CustomerName
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
		})
	}

	source := procurementMessageSource(procMsg.Type)

	rfq := &models.RFQReceived{
		StoreID:                storeObjID,
		FromPhone:              procMsg.From,
		FromName:               fromName,
		MessageType:            "text",
		TextContent:            emailText,
		Source:                 source,
		Status:                 "ready_to_send",
		Products:               products,
		CustomerID:             customerID,
		CustomerName:           extracted.CustomerName,
		CustomerPhone:          extracted.CustomerPhone,
		CustomerEmail:          extracted.CustomerEmail,
		CustomerCompany:        extracted.CustomerCompany,
		ProcurementMessageID:   &procMsg.ID,
		ProcurementMessageCode: procMsg.Code,
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
// Required form fields: llm_provider, llm_model, llm_api_key.
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
	llmAPIKey := strings.TrimSpace(r.FormValue("llm_api_key"))

	if llmAPIKey == "" || llmProvider == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "llm_provider and llm_api_key are required"})
		return
	}

	procMsg, err := models.GetProcurementMessage(msgID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "message not found"})
		return
	}

	// Build content from the email message itself.
	emailText := procMsg.Subject
	if procMsg.BodyText != "" {
		emailText += "\n\n" + procMsg.BodyText
	} else if procMsg.BodyHTML != "" {
		emailText += "\n\n" + procMsg.BodyHTML
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
		case ext == ".csv" || ext == ".txt":
			textParts = append(textParts, "=== "+att.Filename+" ===\n"+string(data))
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

	responseText, llmErr := callLLMExtractRFQ(llmAPIKey, llmModel, llmProvider, combinedText, imageDataURIs, pdfBase64s)
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
// POST /v1/procurement-extract-test
// Standalone LLM extraction test — accepts free-form text + file uploads.
// Required form fields: llm_provider, llm_model, llm_api_key.
// Optional: text (free text input), files[] (images / PDFs / spreadsheets / CSV / TXT).
// Returns the same rfqExtractResult shape.
func ProcurementExtractTestHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		r.ParseForm() //nolint:errcheck
	}

	llmProvider := strings.ToLower(strings.TrimSpace(r.FormValue("llm_provider")))
	llmModel := strings.TrimSpace(r.FormValue("llm_model"))
	llmAPIKey := strings.TrimSpace(r.FormValue("llm_api_key"))
	freeText := strings.TrimSpace(r.FormValue("text"))

	if llmAPIKey == "" || llmProvider == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "llm_provider and llm_api_key are required"})
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

	usedModel := llmModel
	if usedModel == "" {
		usedModel = llmProvider
	}

	responseText, llmErr := callLLMExtractRFQ(llmAPIKey, llmModel, llmProvider, combinedText, imageDataURIs, pdfBase64s)
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
