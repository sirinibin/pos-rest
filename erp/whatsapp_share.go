package erp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/models"
)

// Sharing a sale (POS "Sale complete"): its PDF sent to the customer with the
// approved WhatsApp template "invoice_pdf" when the store has the official
// WhatsApp API connected, else a public PDF link to open in the user's own
// WhatsApp app, and a long-lived link to the signed ZATCA XML printed in the
// PDF. Also mounts the store's WhatsApp (WABA) connection endpoints that the
// Store settings → WhatsApp tab calls (legacy /v1/rfq-bot/*), signed in and
// store-checked.

// DefaultInvoiceTemplate is the template a sale's PDF goes out with.
const DefaultInvoiceTemplate = "invoice_pdf"

// maxSharedPDF caps a shared PDF (WhatsApp documents may be up to 100 MB; an
// invoice is far smaller).
const maxSharedPDF = 10 << 20

// Swappable in tests.
var (
	wabaConnected   = controller.WABAConnected
	wabaSendInvoice = controller.SendWABATemplateDocument
	saveSharedPDF   = controller.SaveSharedInvoicePDF
)

func registerWhatsAppShare(s *mux.Router) {
	s.HandleFunc("/whatsapp/status", authed(handleWhatsAppStatus)).Methods("GET")
	s.HandleFunc("/sales/{id}/whatsapp", authed(handleSaleWhatsApp)).Methods("POST")
	s.HandleFunc("/sales/{id}/pdf-link", authed(handleSalePDFLink)).Methods("POST")
	s.HandleFunc("/sales/{id}/share-links", authed(handleSaleShareLinks)).Methods("GET")
	for _, p := range rfqBotProxies {
		s.HandleFunc("/rfq-bot/"+p.path, authed(rfqBotProxy(p))).Methods(p.method)
	}
}

// ---- pure helpers ----

var reSafeFileName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// sharedPDFName is a safe file name ending in .pdf (letters, digits, . _ -),
// else "<code>.pdf", else "invoice.pdf".
func sharedPDFName(name, code string) string {
	clean := func(s string) string {
		s = strings.Trim(reSafeFileName.ReplaceAllString(strings.TrimSpace(s), "-"), "-.")
		if len(s) > 80 {
			s = s[:80]
		}
		return s
	}
	n := clean(strings.TrimSuffix(strings.TrimSuffix(name, ".pdf"), ".PDF"))
	if n == "" {
		n = clean(code)
	}
	if n == "" {
		n = "invoice"
	}
	return n + ".pdf"
}

// decodeSharedPDF reads a base64 PDF (a data: URL prefix is allowed).
func decodeSharedPDF(v string) ([]byte, map[string]string) {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, ","); strings.HasPrefix(v, "data:") && i > 0 {
		v = v[i+1:]
	}
	if v == "" {
		return nil, map[string]string{"pdf": "required"}
	}
	if base64.StdEncoding.DecodedLen(len(v)) > maxSharedPDF+3 {
		return nil, map[string]string{"pdf": "The PDF is larger than 10 MB."}
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, map[string]string{"pdf": "Not valid base64."}
	}
	if len(b) > maxSharedPDF {
		return nil, map[string]string{"pdf": "The PDF is larger than 10 MB."}
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return nil, map[string]string{"pdf": "Not a PDF file."}
	}
	return b, nil
}

// waDigits is a phone number as the digits WhatsApp expects ("+966 55-123"
// → "96655123"; a leading 00 is the international prefix).
func waDigits(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r >= '٠' && r <= '٩' {
			b.WriteRune('0' + (r - '٠'))
		}
	}
	return strings.TrimPrefix(b.String(), "00")
}

// templateValues are the template body values: one line each (Meta refuses
// new lines, tabs and long runs of spaces), at most 200 characters, at most 10.
func templateValues(v interface{}) []string {
	var out []string
	for _, x := range arr(v) {
		s := strings.Join(strings.Fields(str(x)), " ")
		if r := []rune(s); len(r) > 200 {
			s = string(r[:200])
		}
		out = append(out, s)
		if len(out) == 10 {
			break
		}
	}
	return out
}

// shareXMLLink re-signs a sale's ZATCA XML link (a 24 h link from the record)
// so it keeps working in a PDF the customer keeps.
func shareXMLLink(xmlURL string, now time.Time) string {
	base := strings.SplitN(xmlURL, "?", 2)[0]
	if !strings.HasPrefix(base, "/cdn/zatca/") {
		return ""
	}
	return models.SignCdnURLFor(base, now, models.CdnShareLinkTTL)
}

// ---- handlers ----

// shareStore is the store named by storeId / store_id (query or body), which
// the caller must be able to use.
func shareStore(c *Ctx, r *http.Request, body M) (string, error) {
	q := r.URL.Query()
	hex := strings.TrimSpace(q.Get("storeId"))
	if hex == "" {
		hex = strings.TrimSpace(q.Get("store_id"))
	}
	if hex == "" && body != nil {
		hex = str(body["storeId"])
		if hex == "" {
			hex = str(body["store_id"])
		}
	}
	if hex == "" {
		return "", errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	if c.store(hex) == nil {
		return "", errForbidden("You do not have access to this store.")
	}
	return hex, nil
}

// shareSale loads sale {id} the caller may view.
func shareSale(c *Ctx, r *http.Request, body M) (string, M, error) {
	res := resourceByPath("sales")
	if res == nil {
		return "", nil, errNotFound()
	}
	if err := res.checkPerm(c, "view"); err != nil {
		return "", nil, err
	}
	id := mux.Vars(r)["id"]
	var bodyStore M
	if body != nil {
		bodyStore = M{"storeId": str(body["storeId"])}
	}
	storeHex, err := resolveStore(c, res, id, bodyStore)
	if err != nil {
		return "", nil, err
	}
	rec, err := res.Backend.Get(c, storeHex, id, false)
	if err != nil {
		return "", nil, err
	}
	if rec == nil {
		return "", nil, errNotFound()
	}
	return storeHex, rec, nil
}

func handleWhatsAppStatus(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex, err := shareStore(c, r, nil)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, M{"connected": wabaConnected(storeHex), "template": DefaultInvoiceTemplate})
	return nil
}

func handleSaleWhatsApp(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex, rec, err := shareSale(c, r, body)
	if err != nil {
		return err
	}
	fields := map[string]string{}
	to := waDigits(str(body["phone"]))
	if to == "" {
		fields["phone"] = "required"
	} else if !controller.ValidWABANumber(to) {
		fields["phone"] = "Enter the number with its country code, e.g. 966551234567."
	}
	pdf, perr := decodeSharedPDF(str(body["pdf"]))
	for k, v := range perr {
		fields[k] = v
	}
	if len(fields) > 0 {
		return errBadRequest("", fields)
	}
	if !wabaConnected(storeHex) {
		return errf(http.StatusConflict, "whatsapp_not_connected", controller.ErrWABANotConnected.Error(), nil)
	}
	tpl := strings.TrimSpace(str(body["template"]))
	if tpl == "" {
		tpl = DefaultInvoiceTemplate
	}
	name := sharedPDFName(str(body["filename"]), str(rec["code"]))
	if err := wabaSendInvoice(storeHex, to, tpl, str(body["lang"]), name, pdf, templateValues(body["values"])); err != nil {
		var te *controller.WABATemplateError
		switch {
		case errors.Is(err, controller.ErrWABANotConnected):
			return errf(http.StatusConflict, "whatsapp_not_connected", err.Error(), nil)
		case errors.As(err, &te):
			return errBadRequest(te.Msg, map[string]string{"template": te.Msg})
		default:
			return errf(http.StatusBadGateway, "whatsapp_error", "WhatsApp service error: "+err.Error(), nil)
		}
	}
	writeJSON(w, http.StatusOK, M{"sent": true, "to": to, "template": tpl, "filename": name})
	return nil
}

func handleSalePDFLink(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	body, err := readBody(r)
	if err != nil {
		return err
	}
	storeHex, rec, err := shareSale(c, r, body)
	if err != nil {
		return err
	}
	pdf, perr := decodeSharedPDF(str(body["pdf"]))
	if perr != nil {
		return errBadRequest("", perr)
	}
	name := sharedPDFName(str(body["filename"]), str(rec["code"]))
	link, err := saveSharedPDF(storeHex, name, pdf)
	if err != nil {
		return errInternal("The PDF could not be stored: " + err.Error())
	}
	writeJSON(w, http.StatusOK, M{"url": link, "filename": name})
	return nil
}

func handleSaleShareLinks(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	_, rec, err := shareSale(c, r, nil)
	if err != nil {
		return err
	}
	z := sub(rec, "zatca")
	writeJSON(w, http.StatusOK, M{"xmlUrl": shareXMLLink(str(z["xmlUrl"]), nowFn()), "zatcaStatus": str(z["status"])})
	return nil
}

// ---- Store settings → WhatsApp (legacy /v1/rfq-bot/* behind sign-in) ----

type rfqBotRoute struct {
	method, path string
	verb         string // settings permission: view | edit
	handler      http.HandlerFunc
}

var rfqBotProxies = []rfqBotRoute{
	{"GET", "status", "view", controller.GetBotWhatsAppStatus},
	{"GET", "waba-templates", "view", controller.GetWABATemplates},
	{"POST", "connect", "edit", controller.ConnectBotWhatsApp},
	{"POST", "waba-business-account-id", "edit", controller.SaveWABABusinessAccountID},
	{"DELETE", "disconnect", "edit", controller.DisconnectBotWhatsApp},
	{"POST", "waba-test-message", "edit", controller.SendWABATestMessage},
}

// capture is a minimal ResponseWriter that keeps what a legacy handler wrote.
type capture struct {
	h      http.Header
	status int
	buf    bytes.Buffer
}

func (c *capture) Header() http.Header         { return c.h }
func (c *capture) Write(b []byte) (int, error) { return c.buf.Write(b) }
func (c *capture) WriteHeader(s int) {
	if c.status == 0 {
		c.status = s
	}
}

// rfqBotProxy checks the caller (store access, settings permission) and runs
// the legacy handler, answering its {"error":"…"} failures in the contract
// envelope.
func rfqBotProxy(p rfqBotRoute) func(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	return func(c *Ctx, w http.ResponseWriter, r *http.Request) error {
		if !c.Admin && !c.can("settings", p.verb) {
			return errForbidden("")
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return errBadRequest("Unable to read request body.", nil)
		}
		var body M
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				return errf(http.StatusBadRequest, "malformed_json", "Malformed JSON body: "+err.Error(), nil)
			}
		}
		if _, err := shareStore(c, r, body); err != nil {
			return err
		}
		r2 := r.Clone(r.Context())
		r2.Body = io.NopCloser(bytes.NewReader(raw))
		r2.ContentLength = int64(len(raw))
		rec := &capture{h: http.Header{}}
		p.handler(rec, r2)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		if status >= 400 {
			var e struct {
				Error interface{} `json:"error"`
			}
			_ = json.Unmarshal(rec.buf.Bytes(), &e)
			msg, _ := e.Error.(string)
			if msg == "" {
				msg = strings.TrimSpace(rec.buf.String())
			}
			if msg == "" {
				msg = http.StatusText(status)
			}
			return errf(status, "whatsapp", msg, nil)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(rec.buf.Bytes())
		return nil
	}
}
