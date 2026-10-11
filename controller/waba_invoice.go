package controller

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Sending a document (a POS sale's invoice PDF) to a customer with an
// approved WhatsApp template whose header is a DOCUMENT, e.g. "invoice_pdf".
// Always through the store's official WhatsApp sender (wabaSendConfig).

// ErrWABANotConnected: the store has no official WhatsApp number set up.
var ErrWABANotConnected = errors.New("WhatsApp is not connected for this store. Connect it in Store settings → WhatsApp.")

// WABATemplateError is a template that can't take the send (missing, not
// approved, no document header): the caller's mistake, not Meta's.
type WABATemplateError struct{ Msg string }

func (e *WABATemplateError) Error() string { return e.Msg }

// WABAConnected tells whether the store has an official WhatsApp sender.
func WABAConnected(storeID string) bool {
	id, _ := wabaSendConfig(storeID)
	return id != ""
}

var reWABANumber = regexp.MustCompile(`^[1-9]\d{7,14}$`)

// ValidWABANumber is an international number in digits only (8-15 digits,
// country code first, no leading 0).
func ValidWABANumber(to string) bool { return reWABANumber.MatchString(to) }

// Swappable in tests: Meta's template list, media upload and template send.
var (
	wabaTemplatesFor = defaultWABATemplatesFor
	wabaUploadMedia  = metaUploadMedia
	wabaSendTemplate = metaSendTemplate
)

// defaultWABATemplatesFor lists the approved templates of the business account
// behind the store's sender: the saved bot account id when the bot number
// sends, else the one Meta reports for the sending number.
func defaultWABATemplatesFor(storeID, phoneNumberID, accessToken string) ([]WABATemplate, error) {
	wabaID := ""
	if oid, err := primitive.ObjectIDFromHex(storeID); err == nil {
		if store, err := models.FindStoreByID(&oid, bson.M{}); err == nil && store.Settings.BotWABAPhoneNumberID == phoneNumberID {
			wabaID = store.Settings.BotWABABusinessAccountID
		}
	}
	if wabaID == "" {
		_, _, wabaID = rfqCheckMetaStatus(phoneNumberID, accessToken)
	}
	if wabaID == "" {
		return nil, errors.New("could not read the WhatsApp Business Account of this number")
	}
	return metaListTemplates(wabaID, accessToken)
}

// pickWABATemplate is the approved template `name` in language `lang` (its
// first two letters match too: "en" takes "en_US"), else in any language.
func pickWABATemplate(list []WABATemplate, name, lang string) (WABATemplate, bool) {
	var any *WABATemplate
	lang = strings.ToLower(lang)
	for i := range list {
		t := list[i]
		if t.Name != name || (t.Status != "" && !strings.EqualFold(t.Status, "APPROVED")) {
			continue
		}
		l := strings.ToLower(t.Language)
		if lang != "" && (l == lang || strings.HasPrefix(l, lang+"_") || strings.HasPrefix(lang, l+"_")) {
			return t, true
		}
		if any == nil {
			any = &list[i]
		}
	}
	if any != nil {
		return *any, true
	}
	return WABATemplate{}, false
}

// wabaDocumentComponents builds the template's components: the uploaded PDF as
// the DOCUMENT header and `values` in the body's placeholders (in order; a
// missing value is "-", since Meta refuses empty parameters).
func wabaDocumentComponents(tpl WABATemplate, mediaID, filename string, values []string) ([]interface{}, error) {
	var comps []interface{}
	hasDocHeader := false
	for _, c := range tpl.Components {
		switch strings.ToUpper(c.Type) {
		case "HEADER":
			if strings.ToUpper(c.Format) != "DOCUMENT" {
				return nil, &WABATemplateError{fmt.Sprintf("Template %q has no document header, so it can't carry the PDF.", tpl.Name)}
			}
			hasDocHeader = true
			comps = append(comps, map[string]interface{}{
				"type": "header",
				"parameters": []interface{}{map[string]interface{}{
					"type":     "document",
					"document": map[string]interface{}{"id": mediaID, "filename": filename},
				}},
			})
		case "BODY":
			if params, _ := buildTemplateBodyParams(c.Text, values); len(params) > 0 {
				comps = append(comps, map[string]interface{}{"type": "body", "parameters": params})
			}
		}
	}
	if !hasDocHeader {
		return nil, &WABATemplateError{fmt.Sprintf("Template %q has no document header, so it can't carry the PDF.", tpl.Name)}
	}
	return comps, nil
}

// SendWABATemplateDocument sends `pdf` to `to` with the approved template
// `templateName`, its body filled with `values`.
func SendWABATemplateDocument(storeID, to, templateName, lang, filename string, pdf []byte, values []string) error {
	phoneID, token := wabaSendConfig(storeID)
	if phoneID == "" {
		return ErrWABANotConnected
	}
	list, err := wabaTemplatesFor(storeID, phoneID, token)
	if err != nil {
		return err
	}
	tpl, ok := pickWABATemplate(list, templateName, lang)
	if !ok {
		return &WABATemplateError{fmt.Sprintf("Template %q is not approved on this WhatsApp number. Create it in Meta WhatsApp Manager with a document header.", templateName)}
	}
	// check the template before uploading anything
	if _, err := wabaDocumentComponents(tpl, "", filename, values); err != nil {
		return err
	}
	mediaID, err := wabaUploadMedia(phoneID, token, "application/pdf", filename, pdf)
	if err != nil {
		return err
	}
	comps, _ := wabaDocumentComponents(tpl, mediaID, filename, values)
	lang = tpl.Language
	if lang == "" {
		lang = "en"
	}
	return wabaSendTemplate(phoneID, token, to, tpl.Name, lang, comps)
}

// saveInvoiceFile is saveAttachment with the admin storage settings,
// swappable in tests.
var saveInvoiceFile = func(relKey string, data []byte, contentType string) string {
	return saveAttachment(loadAdminS3Settings(), relKey, data, contentType)
}

// SaveSharedInvoicePDF stores a sale's PDF where a customer can open it from a
// WhatsApp message (S3 when configured, else local disk) and returns its
// public link (/cdn/attachments/invoices/<storeId>/<random>/<filename>). The
// random folder keeps the link unguessable.
func SaveSharedInvoicePDF(storeHex, filename string, pdf []byte) (string, error) {
	if _, err := primitive.ObjectIDFromHex(storeHex); err != nil {
		return "", errors.New("invalid store")
	}
	key, err := generatePrintKey()
	if err != nil {
		return "", err
	}
	link := saveInvoiceFile("attachments/invoices/"+storeHex+"/"+key+"/"+filename, pdf, "application/pdf")
	if link == "" {
		return "", errors.New("the PDF could not be stored")
	}
	return link, nil
}
