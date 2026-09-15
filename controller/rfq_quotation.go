package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// rfqRefRe matches patterns like "RFQ-0010", "RFQ#0010", "RFQ 0010" (case-insensitive).
var rfqRefRe = regexp.MustCompile(`(?i)RFQ[#\-\s](\d+)`)

// extractRFQReference scans text for an RFQ code and returns it in canonical "RFQ-XXXX" form.
// Returns "" if no pattern is found.
func extractRFQReference(text string) string {
	m := rfqRefRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return "RFQ-" + m[1]
}

// findRFQForQuotation locates the best RFQ to attach a supplier quotation to.
// Priority: explicit rfqReference > latest by supplierPhone > (email field not stored, skipped).
func findRFQForQuotation(storeID primitive.ObjectID, supplierPhone, supplierEmail, rfqReference string) (*models.RFQReceived, error) {
	if rfqReference != "" {
		rfq, err := models.FindRFQByCode(storeID, rfqReference)
		if err == nil && rfq != nil {
			return rfq, nil
		}
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}
	if supplierPhone != "" {
		rfq, err := models.FindLatestRFQBySupplierPhone(storeID, supplierPhone)
		if err == nil && rfq != nil {
			return rfq, nil
		}
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}
	// forwarded_to.sent_to_email is not stored in the model — no lookup possible.
	return nil, nil
}

// ── Handler: List supplier replies ──────────────────────────────────────────

// ListSupplierRepliesHandler returns all supplier replies for a given RFQ.
// GET /v1/rfq-received/{id}/supplier-replies?store_id=...
func ListSupplierRepliesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

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

	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		http.Error(w, `{"error":"rfq not found"}`, http.StatusNotFound)
		return
	}

	replies := rfq.SupplierReplies
	if replies == nil {
		replies = []models.SupplierReply{}
	}
	resp := map[string]interface{}{
		"supplier_replies": replies,
		"products":         rfq.Products,
	}
	respBytes, _ := json.Marshal(resp)
	w.Write(respBytes)
}

// ── Handler: Add manual supplier reply ──────────────────────────────────────

// AddManualSupplierReplyHandler adds a manually entered supplier reply to an RFQ.
// POST /v1/rfq-received/{id}/supplier-replies?store_id=...
// Body:
//
//	{
//	  "supplier_name": "ABC Trading",
//	  "supplier_phone": "971501234567",
//	  "supplier_email": "abc@example.com",
//	  "raw_text": "Quote for RFQ-0010: Item 1 - 150 SAR",
//	  "prices": [{"product_index": 0, "unit_price": 150.0, "currency": "SAR"}],
//	  "run_llm_extraction": false
//	}
func AddManualSupplierReplyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

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
		SupplierName     string                      `json:"supplier_name"`
		SupplierPhone    string                      `json:"supplier_phone"`
		SupplierEmail    string                      `json:"supplier_email"`
		RawText          string                      `json:"raw_text"`
		Prices           []models.SupplierReplyPrice `json:"prices"`
		RunLLMExtraction bool                        `json:"run_llm_extraction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
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
		ID:            primitive.NewObjectID(),
		SupplierName:  body.SupplierName,
		SupplierPhone: body.SupplierPhone,
		SupplierEmail: body.SupplierEmail,
		ReceivedAt:    time.Now(),
		RawText:       body.RawText,
		Source:        "manual",
	}

	switch {
	case body.RunLLMExtraction:
		reply.ExtractionStatus = "pending"
	case len(body.Prices) > 0:
		reply.Prices = body.Prices
		reply.IsQuotation = true
		reply.ExtractionStatus = "done"
	default:
		reply.ExtractionStatus = "done"
	}

	if err := models.AddSupplierReplyToRFQ(storeObjID, id, reply); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if body.RunLLMExtraction {
		replyID := reply.ID
		go func() {
			updated, err := models.FindRFQReceivedByID(id, storeObjID)
			if err != nil {
				return
			}
			for i := len(updated.SupplierReplies) - 1; i >= 0; i-- {
				if updated.SupplierReplies[i].ID == replyID {
					extractSupplierPrices(store, updated, &updated.SupplierReplies[i])
					break
				}
			}
		}()
	}

	fmt.Fprintf(w, `{"success":true,"reply_id":%q}`, reply.ID.Hex())
}

// ── Handler: Delete supplier reply ──────────────────────────────────────────

// DeleteSupplierReplyHandler removes a supplier reply from an RFQ.
// DELETE /v1/rfq-received/{id}/supplier-replies/{reply_id}?store_id=...
func DeleteSupplierReplyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	replyID, err := primitive.ObjectIDFromHex(vars["reply_id"])
	if err != nil {
		http.Error(w, `{"error":"invalid reply_id"}`, http.StatusBadRequest)
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
	if err != nil {
		http.Error(w, `{"error":"invalid store_id"}`, http.StatusBadRequest)
		return
	}

	if err := models.DeleteSupplierReplyFromRFQ(storeObjID, id, replyID); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, `{"success":true}`)
}

// ── Email quotation processing ───────────────────────────────────────────────

// ProcessEmailQuotationReply processes an inbound email from a supplier that may contain a quotation.
// It matches the email to the best RFQ, saves the raw reply, and triggers async LLM price extraction.
// Called by the email polling / webhook pipeline when an email arrives from a known supplier address.
func ProcessEmailQuotationReply(store *models.Store, fromEmail, senderName, emailText string) {
	rfqRef := extractRFQReference(emailText)
	rfq, err := findRFQForQuotation(store.ID, "", fromEmail, rfqRef)
	if err != nil {
		log.Printf("rfq_quotation: error looking up RFQ for email from %s: %v", fromEmail, err)
		return
	}
	if rfq == nil {
		log.Printf("rfq_quotation: no matching RFQ found for email from %s (rfqRef=%q)", fromEmail, rfqRef)
		return
	}

	reply := models.SupplierReply{
		ID:               primitive.NewObjectID(),
		SupplierName:     senderName,
		SupplierEmail:    fromEmail,
		ReceivedAt:       time.Now(),
		RawText:          emailText,
		Source:           "email",
		ExtractionStatus: "pending",
	}

	if err := models.AddSupplierReplyToRFQ(store.ID, rfq.ID, reply); err != nil {
		log.Printf("rfq_quotation: failed to save email supplier reply: %v", err)
		return
	}

	// Reload so we have the stored reply with its assigned ID.
	go func() {
		updated, err := models.FindRFQReceivedByID(rfq.ID, rfq.StoreID)
		if err != nil {
			return
		}
		replyID := reply.ID
		for i := len(updated.SupplierReplies) - 1; i >= 0; i-- {
			if updated.SupplierReplies[i].ID == replyID {
				extractSupplierPrices(store, updated, &updated.SupplierReplies[i])
				break
			}
		}
	}()
}

// ── Handler: Parse uploaded quotation file ───────────────────────────────────

// ParseQuotationFileHandler extracts text from an uploaded quotation file so the user can
// review the content before saving it as a supplier reply.
// POST /v1/rfq-received/{id}/supplier-replies/parse-file?store_id=...
// Accepts multipart form with a single "file" field (PDF, image, Excel, or text).
// Returns { extracted_text, file_name, file_type, prices, price_count, is_quotation }.
func ParseQuotationFileHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	rfqID, _ := primitive.ObjectIDFromHex(vars["id"])

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

	// Load RFQ products for better price extraction context.
	var rfqProducts []models.RFQProduct
	if rfq, rfqErr := models.FindRFQReceivedByID(rfqID, storeObjID); rfqErr == nil && rfq != nil {
		rfqProducts = rfq.Products
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		http.Error(w, `{"error":"file too large (max 20 MB)"}`, http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, `{"error":"no file uploaded"}`, http.StatusBadRequest)
		return
	}
	fh := files[0]
	f, err := fh.Open()
	if err != nil {
		http.Error(w, `{"error":"failed to read file"}`, http.StatusInternalServerError)
		return
	}
	data, _ := io.ReadAll(f)
	f.Close()

	ext := strings.ToLower(filepath.Ext(fh.Filename))
	ct := strings.ToLower(fh.Header.Get("Content-Type"))

	var extractedText string
	var fileType string

	switch {
	case ext == ".xlsx" || ext == ".xls" || strings.Contains(ct, "spreadsheet") || strings.Contains(ct, "excel"):
		extractedText, err = excelToText(fh.Filename, data)
		fileType = "excel"

	case ext == ".csv" || ext == ".txt" || strings.HasPrefix(ct, "text/"):
		extractedText = string(data)
		fileType = "text"

	case isRFQImageExt(ext) || strings.HasPrefix(ct, "image/"):
		mime := rfqImageMime(ext, ct)
		b64 := base64.StdEncoding.EncodeToString(data)
		dataURI := "data:" + mime + ";base64," + b64
		extractedText, err = extractTextFromContentLLM(store, []string{dataURI}, nil)
		fileType = "image"

	case ext == ".pdf" || strings.Contains(ct, "pdf"):
		pdfB64 := base64.StdEncoding.EncodeToString(data)
		extractedText, err = extractTextFromContentLLM(store, nil, []string{pdfB64})
		fileType = "pdf"

	default:
		extractedText = string(data)
		fileType = "text"
	}

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
		return
	}

	// Run price extraction on the text content.
	// For PDFs, also pass the raw base64 so a vision LLM can read scanned/Arabic pages.
	var pdfBase64sForAnalysis []string
	if fileType == "pdf" {
		pdfBase64sForAnalysis = []string{base64.StdEncoding.EncodeToString(data)}
	}
	analysis := analyzeSupplierReply(store, extractedText, pdfBase64sForAnalysis, rfqProducts, "", "")

	resp := map[string]interface{}{
		"extracted_text": extractedText,
		"file_name":      fh.Filename,
		"file_type":      fileType,
		"prices":         analysis.Prices,
		"price_count":    len(analysis.Prices),
		"is_quotation":   analysis.IsQuotation,
	}
	json.NewEncoder(w).Encode(resp)
}

// extractTextFromContentLLM uses the store's configured vision LLM to extract raw text
// from images or PDFs. Returns the plain-text content.
func extractTextFromContentLLM(store *models.Store, imageDataURIs []string, pdfBase64s []string) (string, error) {
	provider := strings.ToLower(store.Settings.RFQLLMProvider)
	apiKey := store.Settings.RFQLLMAPIKey
	model := store.Settings.RFQLLMModel
	if apiKey == "" || provider == "" {
		return "", fmt.Errorf("LLM not configured — add API key in store settings")
	}

	const textExtractPrompt = `Extract ALL text from this document or image. Include product names, part numbers, prices, quantities, currencies, supplier name, date, reference numbers, and any other details. Preserve the structure. Return only the extracted text with no commentary.`

	switch provider {
	case "gemini":
		return callGeminiExtractRFQ(apiKey, model, textExtractPrompt, imageDataURIs, pdfBase64s, 2000)
	case "anthropic":
		if model == "" {
			model = "claude-haiku-4-5-20251001"
		}
		return callAnthropicExtractRFQ(apiKey, model, textExtractPrompt, imageDataURIs, pdfBase64s, 2000)
	default:
		return callOpenAICompatExtractRFQ(apiKey, model, textExtractPrompt, imageDataURIs, 2000, openAICompatBaseURL(provider))
	}
}
