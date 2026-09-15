package controller

// SendRFQToSuppliersHandler — POST /v1/rfq-received/{id}/send?store_id=...
// GetRFQSendPreviewHandler  — GET  /v1/rfq-received/{id}/send-preview?store_id=...

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	_ "image/png"
)

var rePlaceholder = regexp.MustCompile(`\{\{(\w+)\}\}`)

// buildTemplateBodyParams builds component parameters for a template body.
// Handles both numbered ({{1}}) and named ({{supplier_name}}) placeholders.
// positional: [supplier, store, rfq_code, contact, email]
func buildTemplateBodyParams(bodyText string, positional []string) ([]interface{}, bool) {
	matches := rePlaceholder.FindAllStringSubmatch(bodyText, -1)
	if len(matches) == 0 {
		return nil, false
	}
	isNamed := false
	for _, m := range matches {
		if _, err := strconv.Atoi(m[1]); err != nil {
			isNamed = true
			break
		}
	}
	namedMap := map[string]int{
		"supplier_name": 0, "supplier": 0,
		"store_name": 1, "company": 1, "business": 1,
		"rfq_reference": 2, "rfq_code": 2, "rfq": 2, "reference": 2,
		"contact": 3, "phone": 3, "mobile": 3,
		"email": 4,
	}
	var params []interface{}
	for i, m := range matches {
		pName := m[1]
		val := "-"
		if isNamed {
			if idx, ok := namedMap[strings.ToLower(pName)]; ok && idx < len(positional) {
				val = positional[idx]
			} else if i < len(positional) {
				val = positional[i]
			}
			if val == "" {
				val = "-"
			}
			params = append(params, map[string]interface{}{
				"type": "text", "text": val, "parameter_name": pName,
			})
		} else {
			idx, _ := strconv.Atoi(pName)
			if idx > 0 && idx <= len(positional) {
				val = positional[idx-1]
			}
			if val == "" {
				val = "-"
			}
			params = append(params, map[string]interface{}{
				"type": "text", "text": val,
			})
		}
	}
	return params, isNamed
}

// fillTemplateBody replaces named and numbered placeholders in body text with values.
func fillTemplateBody(bodyText string, positional []string) string {
	namedMap := map[string]int{
		"supplier_name": 0, "supplier": 0,
		"store_name": 1, "company": 1, "business": 1,
		"rfq_reference": 2, "rfq_code": 2, "rfq": 2, "reference": 2,
		"contact": 3, "phone": 3, "mobile": 3,
		"email": 4,
	}
	return rePlaceholder.ReplaceAllStringFunc(bodyText, func(match string) string {
		inner := match[2 : len(match)-2] // strip {{ }}
		// Try numeric index
		if idx, err := strconv.Atoi(inner); err == nil {
			if idx > 0 && idx <= len(positional) {
				return positional[idx-1]
			}
			return match
		}
		// Try named
		if idx, ok := namedMap[strings.ToLower(inner)]; ok && idx < len(positional) {
			return positional[idx]
		}
		return match
	})
}

// SendRFQToSuppliersHandler sends an RFQ to all matched suppliers via a WABA template.
// POST /v1/rfq-received/{id}/send?store_id=...
// Body: { "prepared_by": "...", "authorized_by": "...", "user_id": "..." }
func SendRFQToSuppliersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// ── Auth ──────────────────────────────────────────────────────────────────
	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized: " + err.Error()})
		return
	}
	// Look up user name for prepared_by fallback
	userName := ""
	if userObjID, parseErr := primitive.ObjectIDFromHex(tokenClaims.UserID); parseErr == nil {
		if u, fetchErr := models.FindUserByID(&userObjID, nil); fetchErr == nil && u != nil {
			userName = u.Name
		}
	}

	// ── Parse route / query params ────────────────────────────────────────────
	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	// ── Parse body ────────────────────────────────────────────────────────────
	var body struct {
		PreparedBy          string `json:"prepared_by"`
		AuthorizedBy        string `json:"authorized_by"`
		UserID              string `json:"user_id"`
		AttachmentMediaID   string `json:"attachment_media_id"`   // pre-uploaded Meta media ID (optional)
		AttachmentMediaType string `json:"attachment_media_type"` // "image" | "document"
		GeneratePDF         bool   `json:"generate_pdf"`          // auto-generate PDF instead of screenshot
		// Optional override: if set, send only to these recipients instead of DB-resolved suppliers
		Recipients []struct {
			Name  string `json:"name"`
			Phone string `json:"phone"`
		} `json:"recipients"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.PreparedBy == "" {
		body.PreparedBy = userName
	}

	// ── Load store & RFQ ──────────────────────────────────────────────────────
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}
	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ not found"})
		return
	}

	// ── WABA credentials check ────────────────────────────────────────────────
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	wabaID := store.Settings.BotWABABusinessAccountID
	if phoneNumberID == "" || accessToken == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bot WhatsApp not connected"})
		return
	}

	// ── Template check ────────────────────────────────────────────────────────
	templateName := store.Settings.WABATemplateRFQSupplier
	if templateName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ to Supplier Template not set. Please configure it in Store → Procurement → WABA Template Purpose."})
		return
	}

	// ── Save prepared_by / authorized_by ──────────────────────────────────────
	rfq.PreparedBy = body.PreparedBy
	rfq.AuthorizedBy = body.AuthorizedBy
	if err := models.UpdateRFQReceived(rfq); err != nil {
		log.Printf("rfq_send: failed to update RFQ with prepared_by: %v", err)
	}

	// ── Resolve suppliers ─────────────────────────────────────────────────────
	// If the caller provides an explicit recipient list, use it; otherwise resolve from DB.
	var suppliers []models.RFQSupplier
	if len(body.Recipients) > 0 {
		seenPhone := map[string]bool{}
		for _, r := range body.Recipients {
			if r.Phone != "" && !seenPhone[r.Phone] {
				seenPhone[r.Phone] = true
				suppliers = append(suppliers, models.RFQSupplier{Name: r.Name, Phone: r.Phone})
			}
		}
	} else {
		categories := rfq.Categories
		if len(categories) == 0 && len(rfq.Products) > 0 {
			for _, p := range rfq.Products {
				if p.Name != "" {
					word := strings.Fields(p.Name)[0]
					categories = append(categories, word)
				}
			}
		}
		if len(categories) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "No suppliers found. Run AI categorization first."})
			return
		}
		var supErr error
		suppliers, supErr = findSuppliers(store, storeObjID, categories)
		if supErr != nil || len(suppliers) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "No suppliers found. Run AI categorization first."})
			return
		}
	}

	// ── Fetch template definition from Meta ───────────────────────────────────
	var tmpl *WABATemplate
	if wabaID != "" {
		templates, listErr := metaListTemplates(wabaID, accessToken)
		if listErr != nil {
			log.Printf("rfq_send: could not list templates: %v", listErr)
		} else {
			for i := range templates {
				if strings.EqualFold(templates[i].Name, templateName) && strings.EqualFold(templates[i].Status, "APPROVED") {
					tmpl = &templates[i]
					break
				}
			}
		}
	}
	if tmpl == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Template '%s' not found or not APPROVED in your WABA account.", templateName)})
		return
	}

	// Auto-detect DOCUMENT header template → always use PDF (not screenshot)
	for _, comp := range tmpl.Components {
		if strings.EqualFold(comp.Type, "HEADER") && strings.EqualFold(comp.Format, "DOCUMENT") {
			body.GeneratePDF = true
			break
		}
	}

	// ── Generate or use pre-uploaded attachment ───────────────────────────────
	chromeBin := chromePath()
	var mediaID string
	var mediaType string // "image" | "document"
	if body.AttachmentMediaID != "" {
		// User pre-uploaded a custom attachment — skip auto-generation
		mediaID = body.AttachmentMediaID
		mediaType = body.AttachmentMediaType
		if mediaType == "" {
			mediaType = "image"
		}
		log.Printf("rfq_send: using pre-uploaded attachment mediaID=%s type=%s", mediaID, mediaType)
	} else if chromeBin != "" {
		key, keyErr := generatePrintKey()
		if keyErr != nil {
			log.Printf("rfq_send: failed to generate print key: %v", keyErr)
		} else {
			// Build print job data from RFQ — embed store for header, rfq already has PreparedBy set above
			type rfqForPrint struct {
				*models.RFQReceived
				Store *models.Store `json:"store"`
			}
			rfqJSON, _ := json.Marshal(rfqForPrint{RFQReceived: rfq, Store: store})
			printJobStore.Store(key, printJobData{
				Model:     rfqJSON,
				ModelName: "rfq_received",
				CreatedAt: time.Now(),
			})
			defer printJobStore.Delete(key)

			frontendURL := os.Getenv("FRONTEND_URL")
			if frontendURL == "" {
				frontendURL = "http://localhost:3004"
			}
			printURL := fmt.Sprintf("%s/rfq-print?key=%s", frontendURL, key)

			if body.GeneratePDF {
				// ── Auto PDF via chromedp PrintToPDF ─────────────────────────────
				mediaType = "document"
				opts := chromeExecOpts(chromeBin)
				allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
				defer cancelAlloc()
				chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
				defer cancelCtx()
				chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
				defer cancelTimeout()

				var pdfBuf []byte
				runErr := chromedp.Run(chromeCtx,
					chromedp.Navigate(printURL),
					chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
					chromedp.Sleep(500*time.Millisecond),
					chromedp.ActionFunc(func(ctx context.Context) error {
						var err error
						pdfBuf, _, err = page.PrintToPDF().
							WithPrintBackground(true).
							WithPaperWidth(8.27).
							WithPaperHeight(11.69).
							WithMarginTop(0).
							WithMarginBottom(0).
							WithMarginLeft(0).
							WithMarginRight(0).
							Do(ctx)
						return err
					}),
				)
				if runErr != nil {
					log.Printf("rfq_send: PDF generation failed: %v", runErr)
				} else if len(pdfBuf) > 0 {
					pdfName := fmt.Sprintf("%s.pdf", rfq.Code)
					mid, uploadErr := metaUploadMedia(phoneNumberID, accessToken, "application/pdf", pdfName, pdfBuf)
					if uploadErr != nil {
						log.Printf("rfq_send: PDF upload failed: %v", uploadErr)
					} else {
						mediaID = mid
						log.Printf("rfq_send: uploaded RFQ PDF mediaID=%s size=%d", mediaID, len(pdfBuf))
					}
				}
			} else {
				// ── Auto screenshot (PNG) via chromedp ───────────────────────────
				mediaType = "image"
				opts := append(chromeExecOpts(chromeBin), chromedp.Flag("force-device-scale-factor", "2"))
				allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
				defer cancelAlloc()
				chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
				defer cancelCtx()
				chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
				defer cancelTimeout()

				var screenshotBuf []byte
				runErr := chromedp.Run(chromeCtx,
					chromedp.Navigate(printURL),
					chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
					chromedp.Sleep(500*time.Millisecond),
					chromedp.ActionFunc(func(ctx context.Context) error {
						var err error
						screenshotBuf, err = page.CaptureScreenshot().
							WithFormat(page.CaptureScreenshotFormatPng).
							Do(ctx)
						return err
					}),
				)
				if runErr != nil {
					log.Printf("rfq_send: screenshot failed: %v", runErr)
				} else if len(screenshotBuf) > 0 {
					mid, uploadErr := metaUploadMedia(phoneNumberID, accessToken, "image/png", "rfq.png", screenshotBuf)
					if uploadErr != nil {
						log.Printf("rfq_send: media upload failed: %v", uploadErr)
					} else {
						mediaID = mid
						log.Printf("rfq_send: uploaded RFQ image (2x PNG), mediaID=%s", mediaID)
					}
				}
			}
		}
	} else {
		log.Printf("rfq_send: Chrome not found — skipping RFQ attachment")
	}

	// ── Determine if template has image or document header ────────────────────
	hasMediaHeader := false
	templateHeaderFormat := ""
	for _, comp := range tmpl.Components {
		if strings.EqualFold(comp.Type, "HEADER") {
			f := strings.ToUpper(comp.Format)
			if f == "IMAGE" || f == "DOCUMENT" {
				hasMediaHeader = true
				templateHeaderFormat = f
				break
			}
		}
	}

	// ── Send to each supplier ─────────────────────────────────────────────────
	sentCount := 0
	now := time.Now()
	var forwardedTo []models.RFQForwardRecord

	nonEmpty := func(s, fallback string) string {
		if strings.TrimSpace(s) == "" {
			return fallback
		}
		return s
	}
	storeName := nonEmpty(store.Name, "Store")
	rfqRef := nonEmpty(rfq.Code, "RFQ")
	// Use the WhatsApp number connected to Meta API as contact
	waContact := store.Settings.RFQMessageContactPhone
	if waContact == "" {
		waContact = store.Settings.BotWhatsAppPhone
	}
	if waContact == "" {
		waContact = store.Phone
	}
	contact := nonEmpty(waContact, phoneNumberID)
	email := nonEmpty(store.Email, "-")

	for _, sup := range suppliers {
		if sup.Phone == "" {
			continue
		}
		supplierPhone := sup.Phone
		supplierName := sup.Name

		// Build template components
		var components []interface{}

		// Header with image or document
		if hasMediaHeader && mediaID != "" {
			comp := buildRFQHeaderComponent(mediaType, templateHeaderFormat, mediaID, rfq.Code)
			components = append(components, comp)
		}

		// Body: build params using named/numbered detection
		bodyText := ""
		for _, comp := range tmpl.Components {
			if strings.EqualFold(comp.Type, "BODY") {
				bodyText = comp.Text
				break
			}
		}
		positional := []string{
			nonEmpty(supplierName, "Supplier"),
			storeName,
			rfqRef,
			contact,
			email,
		}
		bodyParams, _ := buildTemplateBodyParams(bodyText, positional)
		if len(bodyParams) > 0 {
			components = append(components, map[string]interface{}{
				"type":       "body",
				"parameters": bodyParams,
			})
		}

		if compJSON, _ := json.Marshal(components); len(compJSON) > 0 {
			log.Printf("rfq_send: components for %s: %s", supplierPhone, string(compJSON))
		}
		sendErr := metaSendTemplate(phoneNumberID, accessToken, supplierPhone, templateName, tmpl.Language, components)
		sentAt := time.Now()
		rec := models.RFQForwardRecord{
			SupplierID:    sup.ID,
			SupplierName:  supplierName,
			Phone:         supplierPhone,
			SentFromPhone: phoneNumberID,
			Category:      sup.MatchedCategory,
			GoogleMapsURL: sup.GoogleMapsURL,
			SentAt:        &sentAt,
		}
		if sendErr != nil {
			log.Printf("rfq_send: template send to %s failed: %v", supplierPhone, sendErr)
			rec.Status = "failed"
			rec.ErrorMsg = sendErr.Error()
		} else {
			rec.Status = "sent"
			sentCount++
		}
		forwardedTo = append(forwardedTo, rec)
		// Broadcast per-supplier status so the frontend modal can update in real time
		BroadcastRFQData(storeObjID.Hex(), "rfq_send_status", map[string]interface{}{
			"rfq_id":   rfq.ID.Hex(),
			"phone":    supplierPhone,
			"name":     supplierName,
			"status":   rec.Status,
			"error":    rec.ErrorMsg,
		})
		models.AppendRFQLog(storeObjID, rfq.ID, models.RFQActivityLog{
			At:      now,
			Step:    "template_sent",
			Message: fmt.Sprintf("RFQ sent to supplier %s (%s) via WABA template '%s': %s", supplierName, supplierPhone, templateName, rec.Status),
			Icon:    "bi-whatsapp", Color: "success",
			Details: map[string]interface{}{
				"supplier": supplierName, "phone": supplierPhone,
				"template": templateName, "status": rec.Status,
			},
		})
	}

	// ── Persist manually-added recipients to the rfq-suppliers collection ────
	// Suppliers provided via body.Recipients that have no pre-existing ID (ID.IsZero)
	// are new and should be saved so they appear at /dashboard/rfq-suppliers.
	if len(body.Recipients) > 0 {
		for _, sup := range suppliers {
			if sup.ID.IsZero() && sup.Phone != "" {
				newSup := &models.RFQSupplier{
					StoreID:  storeObjID,
					Name:     sup.Name,
					Phone:    sup.Phone,
					IsActive: true,
					AddedAt:  time.Now(),
				}
				if err := models.UpsertRFQSupplierByPlaceID(newSup); err != nil {
					log.Printf("rfq_send: failed to upsert new supplier %s: %v", sup.Phone, err)
				} else {
					log.Printf("rfq_send: upserted new supplier %s (%s)", sup.Name, sup.Phone)
					// Enrich with Google Maps in the background (address, rating, website, etc.)
					if store.Settings.GoogleMapsAPIKey != "" {
						go func(s *models.RFQSupplier, storeIDHex string) {
							if enriched, err := enrichSupplierFromGoogleMaps(store.Settings.GoogleMapsAPIKey, s); err != nil {
								log.Printf("rfq_send: Maps enrich error for %s: %v", s.Phone, err)
							} else if enriched {
								models.UpsertRFQSupplierByPlaceID(s) //nolint:errcheck
								BroadcastRFQEvent(storeIDHex, "supplier_updated")
							}
						}(newSup, storeObjID.Hex())
					}
				}
			}
		}
	}

	// ── Update RFQ document ───────────────────────────────────────────────────
	if rfq.ForwardedTo == nil {
		rfq.ForwardedTo = forwardedTo
	} else {
		rfq.ForwardedTo = append(rfq.ForwardedTo, forwardedTo...)
	}
	rfq.Status = "forwarded"
	if err := models.UpdateRFQReceived(rfq); err != nil {
		log.Printf("rfq_send: failed to update RFQ status: %v", err)
	}

	// ── Broadcast SSE ─────────────────────────────────────────────────────────
	BroadcastRFQEvent(storeObjID.Hex(), "rfq_updated")
	// Signal that all sends are complete
	BroadcastRFQData(storeObjID.Hex(), "rfq_send_done", map[string]interface{}{
		"rfq_id": rfq.ID.Hex(), "sent_count": sentCount,
	})

	// ── Response ──────────────────────────────────────────────────────────────
	recipientStatuses := make(map[string]string, len(forwardedTo))
	for _, rec := range forwardedTo {
		recipientStatuses[rec.Phone] = rec.Status
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":            true,
		"sent_count":         sentCount,
		"recipient_statuses": recipientStatuses,
	})
}

// ── GetRFQSendPreviewHandler ──────────────────────────────────────────────────
// GET /v1/rfq-received/{id}/send-preview?store_id=...
// Returns the supplier list and filled template text so the frontend can render
// a preview modal before the user confirms the send.
func GetRFQSendPreviewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}

	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
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
	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ not found"})
		return
	}

	// WABA credentials — soft-check: collect warnings but don't block supplier listing
	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	wabaID := store.Settings.BotWABABusinessAccountID
	templateName := store.Settings.WABATemplateRFQSupplier
	var configWarning string
	if phoneNumberID == "" || accessToken == "" {
		configWarning = "Bot WhatsApp not connected"
	} else if templateName == "" {
		configWarning = "RFQ to Supplier Template not set. Please configure it in Store → Procurement → WABA Template Purpose."
	}

	// Resolve suppliers: use stored IDs from the bot pipeline when available
	// (they capture suppliers from broader category searches), otherwise fall back to findSuppliers.
	var suppliers []models.RFQSupplier
	if len(rfq.MatchedSupplierIDs) > 0 {
		suppliers, _ = models.FindRFQSuppliersByIDs(rfq.MatchedSupplierIDs)
	}
	if len(suppliers) == 0 {
		categories := rfq.Categories
		if len(categories) == 0 && len(rfq.Products) > 0 {
			for _, p := range rfq.Products {
				if p.Name != "" {
					categories = append(categories, strings.Fields(p.Name)[0])
				}
			}
		}
		suppliers, _ = findSuppliers(store, storeObjID, categories)
	}

	// Fetch template body text from Meta
	templateBody := ""
	templateLanguage := ""
	hasImageHeader := false
	if wabaID != "" {
		if templates, listErr := metaListTemplates(wabaID, accessToken); listErr == nil {
			for _, t := range templates {
				if strings.EqualFold(t.Name, templateName) && strings.EqualFold(t.Status, "APPROVED") {
					templateLanguage = t.Language
					for _, comp := range t.Components {
						if strings.EqualFold(comp.Type, "BODY") {
							templateBody = comp.Text
						}
						if strings.EqualFold(comp.Type, "HEADER") && strings.EqualFold(comp.Format, "IMAGE") {
							hasImageHeader = true
						}
					}
					break
				}
			}
		}
	}

	// Resolve preview values using user-specified field mappings
	storeName := store.Name
	rfqRef := rfq.Code
	// {{contact}} = RFQ Message Contact Number (new field), then BotWhatsAppPhone, then store phone
	contact := store.Settings.RFQMessageContactPhone
	if contact == "" {
		contact = store.Settings.BotWhatsAppPhone
	}
	if contact == "" {
		contact = store.Phone
	}
	if contact == "" {
		contact = phoneNumberID
	}
	// {{supplier_name}} = first resolved supplier's name
	firstSupplierName := "Supplier Name"
	if len(suppliers) > 0 && suppliers[0].Name != "" {
		firstSupplierName = suppliers[0].Name
	}

	previewPositional := []string{firstSupplierName, storeName, rfqRef, contact, store.Email}
	filledBody := fillTemplateBody(templateBody, previewPositional)
	preFilledVars := map[string]string{
		// numbered keys
		"body_1": firstSupplierName,
		"body_2": storeName,
		"body_3": rfqRef,
		"body_4": contact,
		"body_5": store.Email,
		// named keys matching common template variable names
		"body_supplier_name": firstSupplierName,
		"body_supplier":      firstSupplierName,
		"body_store_name":    storeName,
		"body_company":       storeName,
		"body_business":      storeName,
		"body_rfq_reference": rfqRef,
		"body_rfq_code":      rfqRef,
		"body_rfq":           rfqRef,
		"body_reference":     rfqRef,
		"body_contact":       contact,
		"body_phone":         contact,
		"body_mobile":        contact,
		"body_email":         store.Email,
	}

	// Build supplier preview list
	type supplierPreview struct {
		ID              string   `json:"id"`
		Name            string   `json:"name"`
		Phone           string   `json:"phone"`
		Category        string   `json:"category"`
		Categories      []string `json:"categories"`
		CategoryMatched bool     `json:"category_matched"`
		Address         string   `json:"address,omitempty"`
		Website         string   `json:"website,omitempty"`
		GoogleMapsURL   string   `json:"google_maps_url,omitempty"`
		Rating          float64  `json:"rating,omitempty"`
		PurchaseMarket  string   `json:"purchase_market,omitempty"`
	}
	rfqCatSet := map[string]bool{}
	for _, c := range rfq.Categories {
		rfqCatSet[strings.ToLower(c)] = true
	}
	var supList []supplierPreview
	for _, s := range suppliers {
		matched := false
		for _, c := range s.Categories {
			if rfqCatSet[strings.ToLower(c)] {
				matched = true
				break
			}
		}
		if !matched && s.MatchedCategory != "" {
			matched = rfqCatSet[strings.ToLower(s.MatchedCategory)]
		}
		if !matched {
			matched = true // supplier was returned by findSuppliers — treat as matching
		}
		supList = append(supList, supplierPreview{
			ID:              s.ID.Hex(),
			Name:            s.Name,
			Phone:           s.Phone,
			Category:        s.MatchedCategory,
			Categories:      s.Categories,
			CategoryMatched: matched,
			Address:         s.Address,
			Website:         s.Website,
			GoogleMapsURL:   s.GoogleMapsURL,
			Rating:          s.Rating,
			PurchaseMarket:  s.PurchaseMarket,
		})
	}

	// Return the full template components so the frontend can build API components itself
	var templateComponents interface{}
	if wabaID != "" {
		if templates, listErr := metaListTemplates(wabaID, accessToken); listErr == nil {
			for _, t := range templates {
				if strings.EqualFold(t.Name, templateName) && strings.EqualFold(t.Status, "APPROVED") {
					templateComponents = t.Components
					break
				}
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"rfq_id":              rfq.ID.Hex(),
		"rfq_code":            rfq.Code,
		"template_name":       templateName,
		"template_language":   templateLanguage,
		"template_body":       filledBody,
		"template_components": templateComponents,
		"pre_filled_vars":     preFilledVars,
		"has_image_header":    hasImageHeader,
		"suppliers":           supList,
		"store_name":          storeName,
		"config_warning":      configWarning,
	})
}

// ── SendRFQTestMessageHandler ─────────────────────────────────────────────────
// POST /v1/rfq-received/{id}/send-test?store_id=...
// Body: { "phone": "971501234567", "name": "Test Recipient" }
// Sends the WABA template + RFQ image to a single number without recording
// the send or changing the RFQ status. Used to verify the template looks right.
func SendRFQTestMessageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
		return
	}
	testUserName := ""
	if userObjID, parseErr := primitive.ObjectIDFromHex(tokenClaims.UserID); parseErr == nil {
		if u, fetchErr := models.FindUserByID(&userObjID, nil); fetchErr == nil && u != nil {
			testUserName = u.Name
		}
	}

	vars := mux.Vars(r)
	idStr := vars["id"]
	storeIDStr := r.URL.Query().Get("store_id")

	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid store_id"})
		return
	}

	var body struct {
		Phone string `json:"phone"`
		Name  string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Phone == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "phone is required"})
		return
	}
	if body.Name == "" {
		body.Name = "Test Recipient"
	}

	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "store not found"})
		return
	}
	rfq, err := models.FindRFQReceivedByID(id, storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ not found"})
		return
	}

	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	wabaID := store.Settings.BotWABABusinessAccountID
	if phoneNumberID == "" || accessToken == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bot WhatsApp not connected"})
		return
	}
	templateName := store.Settings.WABATemplateRFQSupplier
	if templateName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ to Supplier Template not set"})
		return
	}

	// Fetch template
	var tmpl *WABATemplate
	if wabaID != "" {
		if templates, listErr := metaListTemplates(wabaID, accessToken); listErr == nil {
			for i := range templates {
				if strings.EqualFold(templates[i].Name, templateName) && strings.EqualFold(templates[i].Status, "APPROVED") {
					tmpl = &templates[i]
					break
				}
			}
		}
	}
	if tmpl == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Template '%s' not found or not APPROVED", templateName)})
		return
	}

	// Detect template header format (IMAGE or DOCUMENT)
	testHeaderFormat := ""
	for _, comp := range tmpl.Components {
		if strings.EqualFold(comp.Type, "HEADER") {
			f := strings.ToUpper(comp.Format)
			if f == "IMAGE" || f == "DOCUMENT" {
				testHeaderFormat = f
			}
			break
		}
	}

	// Generate RFQ media — PDF for DOCUMENT templates, screenshot for IMAGE templates
	if rfq.PreparedBy == "" && testUserName != "" {
		rfq.PreparedBy = testUserName
	}
	chromeBin := chromePath()
	var mediaID string
	var testMediaType string
	if chromeBin != "" && testHeaderFormat != "" {
		key, keyErr := generatePrintKey()
		if keyErr == nil {
			type rfqForPrint struct {
				*models.RFQReceived
				Store *models.Store `json:"store"`
			}
			rfqJSON, _ := json.Marshal(rfqForPrint{RFQReceived: rfq, Store: store})
			printJobStore.Store(key, printJobData{Model: rfqJSON, ModelName: "rfq_received", CreatedAt: time.Now()})
			defer printJobStore.Delete(key)

			frontendURL := os.Getenv("FRONTEND_URL")
			if frontendURL == "" {
				frontendURL = "http://localhost:3004"
			}
			printURL := fmt.Sprintf("%s/rfq-print?key=%s", frontendURL, key)

			if testHeaderFormat == "DOCUMENT" {
				opts := chromeExecOpts(chromeBin)
				allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
				defer cancelAlloc()
				chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
				defer cancelCtx()
				chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
				defer cancelTimeout()

				var pdfBuf []byte
				if runErr := chromedp.Run(chromeCtx,
					chromedp.Navigate(printURL),
					chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
					chromedp.Sleep(500*time.Millisecond),
					chromedp.ActionFunc(func(ctx context.Context) error {
						var err error
						pdfBuf, _, err = page.PrintToPDF().
							WithPrintBackground(true).
							WithPaperWidth(8.27).
							WithPaperHeight(11.69).
							WithMarginTop(0).WithMarginBottom(0).
							WithMarginLeft(0).WithMarginRight(0).
							Do(ctx)
						return err
					}),
				); runErr == nil && len(pdfBuf) > 0 {
					pdfName := fmt.Sprintf("%s.pdf", rfq.Code)
					mid, uploadErr := metaUploadMedia(phoneNumberID, accessToken, "application/pdf", pdfName, pdfBuf)
					if uploadErr == nil {
						mediaID = mid
						testMediaType = "document"
						log.Printf("rfq_send_test: uploaded PDF mediaID=%s name=%s", mediaID, pdfName)
					} else {
						log.Printf("rfq_send_test: PDF upload failed: %v", uploadErr)
					}
				}
			} else {
				opts := append(chromeExecOpts(chromeBin), chromedp.Flag("force-device-scale-factor", "2"))
				allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
				defer cancelAlloc()
				chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
				defer cancelCtx()
				chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
				defer cancelTimeout()

				var screenshotBuf []byte
				if runErr := chromedp.Run(chromeCtx,
					chromedp.Navigate(printURL),
					chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
					chromedp.Sleep(500*time.Millisecond),
					chromedp.ActionFunc(func(ctx context.Context) error {
						var err error
						screenshotBuf, err = page.CaptureScreenshot().
							WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
						return err
					}),
				); runErr == nil && len(screenshotBuf) > 0 {
					mid, uploadErr := metaUploadMedia(phoneNumberID, accessToken, "image/png", "rfq.png", screenshotBuf)
					if uploadErr == nil {
						mediaID = mid
						testMediaType = "image"
					}
				}
			}
		}
	}

	// Build header component using the same helper as the main send
	var components []interface{}
	if mediaID != "" && testHeaderFormat != "" {
		comp := buildRFQHeaderComponent(testMediaType, testHeaderFormat, mediaID, rfq.Code)
		log.Printf("rfq_send_test: header component: %+v", comp)
		components = append(components, comp)
	}

	ne2 := func(s, fallback string) string {
		if strings.TrimSpace(s) == "" {
			return fallback
		}
		return s
	}
	waContact2 := store.Settings.RFQMessageContactPhone
	if waContact2 == "" {
		waContact2 = store.Settings.BotWhatsAppPhone
	}
	if waContact2 == "" {
		waContact2 = store.Phone
	}
	contact2 := ne2(waContact2, phoneNumberID)
	testBodyText := ""
	for _, comp := range tmpl.Components {
		if strings.EqualFold(comp.Type, "BODY") {
			testBodyText = comp.Text
			break
		}
	}
	testPositional := []string{
		ne2(body.Name, "Supplier"),
		ne2(store.Name, "Store"),
		ne2(rfq.Code, "RFQ"),
		contact2,
		ne2(store.Email, "-"),
	}
	testBodyParams, _ := buildTemplateBodyParams(testBodyText, testPositional)
	if len(testBodyParams) > 0 {
		components = append(components, map[string]interface{}{
			"type":       "body",
			"parameters": testBodyParams,
		})
	}

	sendErr := metaSendTemplate(phoneNumberID, accessToken, body.Phone, templateName, tmpl.Language, components)
	if sendErr != nil {
		log.Printf("rfq_send_test: template send to %s failed: %v", body.Phone, sendErr)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": sendErr.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "phone": body.Phone})
}

// ── GenerateRFQPreviewImageHandler ───────────────────────────────────────────
// POST /v1/rfq-received/{id}/generate-image?store_id=...
// Takes a chromedp screenshot of the RFQ print page, uploads it to Meta,
// and returns the media_id so the frontend can include it in template components.
func GenerateRFQPreviewImageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	genTokenClaims, genAuthErr := models.AuthenticateByAccessToken(r)
	genUserName := ""
	if genAuthErr == nil {
		if genUserObjID, parseErr := primitive.ObjectIDFromHex(genTokenClaims.UserID); parseErr == nil {
			if u, fetchErr := models.FindUserByID(&genUserObjID, nil); fetchErr == nil && u != nil {
				genUserName = u.Name
			}
		}
	}
	storeIDStr := r.URL.Query().Get("store_id")
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
	vars := mux.Vars(r)
	rfqID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	rfq, err := models.FindRFQReceivedByID(rfqID, storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ not found"})
		return
	}

	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bot WhatsApp not connected"})
		return
	}

	chromeBin := chromePath()
	if chromeBin == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Chrome not available for screenshot"})
		return
	}

	key, keyErr := generatePrintKey()
	if keyErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate print key"})
		return
	}
	// Embed store for header display + set prepared_by from sender
	if rfq.PreparedBy == "" && genUserName != "" {
		rfq.PreparedBy = genUserName
	}
	type rfqForPrintGen struct {
		*models.RFQReceived
		Store *models.Store `json:"store"`
	}
	rfqJSON, _ := json.Marshal(rfqForPrintGen{RFQReceived: rfq, Store: store})
	printJobStore.Store(key, printJobData{Model: rfqJSON, ModelName: "rfq_received", CreatedAt: time.Now()})
	defer printJobStore.Delete(key)

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3004"
	}
	printURL := fmt.Sprintf("%s/rfq-print?key=%s", frontendURL, key)

	opts := append(chromeExecOpts(chromeBin), chromedp.Flag("force-device-scale-factor", "2"))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
	defer cancelTimeout()

	var screenshotBuf []byte
	if runErr := chromedp.Run(chromeCtx,
		chromedp.Navigate(printURL),
		chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			screenshotBuf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatPng).Do(ctx)
			return err
		}),
	); runErr != nil {
		log.Printf("rfq_generate_image: screenshot failed: %v", runErr)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "screenshot failed: " + runErr.Error()})
		return
	}

	mediaID, uploadErr := metaUploadMedia(phoneNumberID, accessToken, "image/png", "rfq.png", screenshotBuf)
	if uploadErr != nil {
		log.Printf("rfq_generate_image: upload failed: %v", uploadErr)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "media upload failed: " + uploadErr.Error()})
		return
	}

	log.Printf("rfq_generate_image: rfq=%s mediaID=%s", rfq.ID.Hex(), mediaID)
	json.NewEncoder(w).Encode(map[string]string{"media_id": mediaID})
}

// GenerateRFQPDFHandler generates a PDF of the RFQ print page via headless Chrome,
// uploads it to Meta as a document, and returns the media_id.
// POST /v1/rfq-received/{id}/generate-pdf?store_id=...
func GenerateRFQPDFHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	genTokenClaims, genAuthErr := models.AuthenticateByAccessToken(r)
	genUserName := ""
	if genAuthErr == nil {
		if genUserObjID, parseErr := primitive.ObjectIDFromHex(genTokenClaims.UserID); parseErr == nil {
			if u, fetchErr := models.FindUserByID(&genUserObjID, nil); fetchErr == nil && u != nil {
				genUserName = u.Name
			}
		}
	}
	storeIDStr := r.URL.Query().Get("store_id")
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
	vars := mux.Vars(r)
	rfqID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid id"})
		return
	}
	rfq, err := models.FindRFQReceivedByID(rfqID, storeObjID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "RFQ not found"})
		return
	}

	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Bot WhatsApp not connected"})
		return
	}

	chromeBin := chromePath()
	if chromeBin == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Chrome not available for PDF generation"})
		return
	}

	key, keyErr := generatePrintKey()
	if keyErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate print key"})
		return
	}
	if rfq.PreparedBy == "" && genUserName != "" {
		rfq.PreparedBy = genUserName
	}
	type rfqForPDF struct {
		*models.RFQReceived
		Store *models.Store `json:"store"`
	}
	rfqJSON, _ := json.Marshal(rfqForPDF{RFQReceived: rfq, Store: store})
	printJobStore.Store(key, printJobData{Model: rfqJSON, ModelName: "rfq_received", CreatedAt: time.Now()})
	defer printJobStore.Delete(key)

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3004"
	}
	printURL := fmt.Sprintf("%s/rfq-print?key=%s", frontendURL, key)

	opts := chromeExecOpts(chromeBin)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
	defer cancelTimeout()

	var pdfBuf []byte
	if runErr := chromedp.Run(chromeCtx,
		chromedp.Navigate(printURL),
		chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).   // A4 width in inches
				WithPaperHeight(11.69). // A4 height in inches
				WithMarginTop(0).
				WithMarginBottom(0).
				WithMarginLeft(0).
				WithMarginRight(0).
				Do(ctx)
			return err
		}),
	); runErr != nil {
		log.Printf("rfq_generate_pdf: PDF generation failed: %v", runErr)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "PDF generation failed: " + runErr.Error()})
		return
	}

	fileName := fmt.Sprintf("%s.pdf", rfq.Code)
	mediaID, uploadErr := metaUploadMedia(phoneNumberID, accessToken, "application/pdf", fileName, pdfBuf)
	if uploadErr != nil {
		log.Printf("rfq_generate_pdf: upload failed: %v", uploadErr)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "media upload failed: " + uploadErr.Error()})
		return
	}

	log.Printf("rfq_generate_pdf: rfq=%s mediaID=%s size=%d", rfq.ID.Hex(), mediaID, len(pdfBuf))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"media_id":   mediaID,
		"media_type": "document",
		"file_name":  fileName,
	})
}

// DownloadRFQPDFHandler generates a PDF for the RFQ and returns it as raw bytes.
// GET /v1/rfq-received/{id}/download-pdf?store_id=...
func DownloadRFQPDFHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	storeIDStr := r.URL.Query().Get("store_id")
	storeObjID, err := primitive.ObjectIDFromHex(storeIDStr)
	if err != nil {
		http.Error(w, "invalid store_id", http.StatusBadRequest)
		return
	}
	store, err := models.FindStoreByID(&storeObjID, bson.M{})
	if err != nil {
		http.Error(w, "store not found", http.StatusNotFound)
		return
	}
	vars := mux.Vars(r)
	rfqID, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	rfq, err := models.FindRFQReceivedByID(rfqID, storeObjID)
	if err != nil {
		http.Error(w, "RFQ not found", http.StatusNotFound)
		return
	}
	chromeBin := chromePath()
	if chromeBin == "" {
		http.Error(w, "Chrome not available", http.StatusServiceUnavailable)
		return
	}
	key, keyErr := generatePrintKey()
	if keyErr != nil {
		http.Error(w, "failed to generate print key", http.StatusInternalServerError)
		return
	}
	type rfqForPDF struct {
		*models.RFQReceived
		Store *models.Store `json:"store"`
	}
	rfqJSON, _ := json.Marshal(rfqForPDF{RFQReceived: rfq, Store: store})
	printJobStore.Store(key, printJobData{Model: rfqJSON, ModelName: "rfq_received", CreatedAt: time.Now()})
	defer printJobStore.Delete(key)

	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3004"
	}
	printURL := fmt.Sprintf("%s/rfq-print?key=%s", frontendURL, key)

	opts := chromeExecOpts(chromeBin)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	chromeCtx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	chromeCtx, cancelTimeout := context.WithTimeout(chromeCtx, 60*time.Second)
	defer cancelTimeout()

	var pdfBuf []byte
	if runErr := chromedp.Run(chromeCtx,
		chromedp.Navigate(printURL),
		chromedp.WaitVisible(`body[data-print-ready="true"]`, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).
				WithPaperHeight(11.69).
				WithMarginTop(0).WithMarginBottom(0).
				WithMarginLeft(0).WithMarginRight(0).
				Do(ctx)
			return err
		}),
	); runErr != nil {
		http.Error(w, "PDF generation failed", http.StatusInternalServerError)
		return
	}

	fileName := fmt.Sprintf("%s.pdf", rfq.Code)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBuf)))
	w.Write(pdfBuf) //nolint:errcheck
}

// UploadRFQAttachmentHandler uploads a user-supplied image or PDF to Meta and returns the media ID.
// The frontend calls this when the user picks "Upload file" mode in the send modal.
// POST /v1/rfq-received/{id}/upload-attachment?store_id=...
// Accepts multipart form with a single "file" field (image or PDF, max 20 MB).
// Returns { media_id, media_type: "image"|"document", mime_type, file_name }.
func UploadRFQAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

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

	phoneNumberID := store.Settings.BotWABAPhoneNumberID
	accessToken := store.Settings.BotWABAAccessToken
	if phoneNumberID == "" || accessToken == "" {
		http.Error(w, `{"error":"WhatsApp (WABA) not configured for this store"}`, http.StatusBadRequest)
		return
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

	var mimeType, mediaType string
	switch {
	case ext == ".pdf" || strings.Contains(ct, "pdf"):
		mimeType, mediaType = "application/pdf", "document"
	case ext == ".jpg" || ext == ".jpeg" || strings.Contains(ct, "jpeg"):
		mimeType, mediaType = "image/jpeg", "image"
	case ext == ".png" || strings.Contains(ct, "png"):
		mimeType, mediaType = "image/png", "image"
	case ext == ".webp" || strings.Contains(ct, "webp"):
		mimeType, mediaType = "image/webp", "image"
	case ext == ".gif" || strings.Contains(ct, "gif"):
		mimeType, mediaType = "image/gif", "image"
	default:
		if strings.HasPrefix(ct, "image/") {
			mimeType, mediaType = ct, "image"
		} else {
			http.Error(w, `{"error":"unsupported file type — use an image (JPG, PNG, WEBP) or PDF"}`, http.StatusBadRequest)
			return
		}
	}

	mediaID, uploadErr := metaUploadMedia(phoneNumberID, accessToken, mimeType, fh.Filename, data)
	if uploadErr != nil {
		http.Error(w, fmt.Sprintf(`{"error":%q}`, uploadErr.Error()), http.StatusBadGateway)
		return
	}

	log.Printf("rfq_upload_attachment: store=%s mediaID=%s type=%s file=%s", storeObjID.Hex(), mediaID, mediaType, fh.Filename)
	json.NewEncoder(w).Encode(map[string]string{
		"media_id":   mediaID,
		"media_type": mediaType,
		"mime_type":  mimeType,
		"file_name":  fh.Filename,
	})
}

// buildRFQHeaderComponent constructs the WABA template header component for an RFQ send.
// mediaType is the uploaded media type ("image"/"document"/""").
// templateHeaderFormat is the template's declared HEADER format ("IMAGE"/"DOCUMENT").
// mediaID is the Meta-uploaded media ID.
// rfqCode is the RFQ code (e.g. "RFQ-0017") used as the PDF filename.
func buildRFQHeaderComponent(mediaType, templateHeaderFormat, mediaID, rfqCode string) map[string]interface{} {
	effectiveType := mediaType
	if effectiveType == "" {
		if templateHeaderFormat == "DOCUMENT" {
			effectiveType = "document"
		} else {
			effectiveType = "image"
		}
	}
	mediaParam := map[string]interface{}{
		"id": mediaID,
	}
	if effectiveType == "document" {
		mediaParam["filename"] = fmt.Sprintf("%s.pdf", rfqCode)
	}
	return map[string]interface{}{
		"type": "header",
		"parameters": []interface{}{
			map[string]interface{}{
				"type":        effectiveType,
				effectiveType: mediaParam,
			},
		},
	}
}
