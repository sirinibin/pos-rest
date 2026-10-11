package erp

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/controller"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
)

var testPDF = base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n%test\n"))

func TestSharedPDFName(t *testing.T) {
	cases := []struct{ name, code, want string }{
		{"S-INV-001.pdf", "", "S-INV-001.pdf"},
		{"S-INV-001", "", "S-INV-001.pdf"},
		{"", "S-INV-002", "S-INV-002.pdf"},
		{"", "", "invoice.pdf"},
		{"../../etc/passwd", "", "etc-passwd.pdf"},
		{"فاتورة 1.pdf", "S-9", "1.pdf"},
		{"  ", "INV 7/2026", "INV-7-2026.pdf"},
		{strings.Repeat("a", 120), "", strings.Repeat("a", 80) + ".pdf"},
	}
	for _, c := range cases {
		if got := sharedPDFName(c.name, c.code); got != c.want {
			t.Errorf("sharedPDFName(%q,%q)=%q want %q", c.name, c.code, got, c.want)
		}
	}
}

func TestDecodeSharedPDF(t *testing.T) {
	if b, errs := decodeSharedPDF(testPDF); errs != nil || !strings.HasPrefix(string(b), "%PDF-") {
		t.Fatalf("plain base64: %v", errs)
	}
	if _, errs := decodeSharedPDF("data:application/pdf;base64," + testPDF); errs != nil {
		t.Fatalf("data URL: %v", errs)
	}
	big := base64.StdEncoding.EncodeToString(append([]byte("%PDF-"), make([]byte, maxSharedPDF)...))
	for name, in := range map[string]string{
		"empty":   "",
		"bad b64": "%%%",
		"not pdf": base64.StdEncoding.EncodeToString([]byte("<html>")),
		"too big": big,
	} {
		if _, errs := decodeSharedPDF(in); errs["pdf"] == "" {
			t.Errorf("%s: want a pdf error", name)
		}
	}
}

func TestWaDigits(t *testing.T) {
	cases := map[string]string{
		"+966 55 123 4567": "966551234567",
		"00968-9123-4567":  "96891234567",
		"٩٦٦٥٥١٢٣٤٥٦٧":     "966551234567",
		"abc":              "",
	}
	for in, want := range cases {
		if got := waDigits(in); got != want {
			t.Errorf("waDigits(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTemplateValues(t *testing.T) {
	got := templateValues([]interface{}{"Ali\nKhan", "  S-INV-1\t", strings.Repeat("x", 250), 12.5})
	want := []string{"Ali Khan", "S-INV-1", strings.Repeat("x", 200), "12.5"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("value %d = %q want %q", i, got[i], want[i])
		}
	}
	many := make([]interface{}, 15)
	if n := len(templateValues(many)); n != 10 {
		t.Errorf("capped at 10, got %d", n)
	}
	if templateValues(nil) != nil {
		t.Error("nil values")
	}
}

func TestShareXMLLink(t *testing.T) {
	now := time.Now()
	short := models.SignCdnURL("/cdn/zatca/abc/inv.xml", now)
	l := shareXMLLink(short, now)
	u, err := url.Parse(l)
	if err != nil || u.Path != "/cdn/zatca/abc/inv.xml" {
		t.Fatalf("link %q", l)
	}
	// still valid nine years on, unlike the 24 h link it came from
	later := now.Add(9 * 365 * 24 * time.Hour)
	if !models.ValidCdnSignature(u.Path, u.Query(), later) {
		t.Fatal("share link must outlive the invoice retention period")
	}
	su, _ := url.Parse(short)
	if models.ValidCdnSignature(su.Path, su.Query(), later) {
		t.Fatal("the record link stays a 24 h link")
	}
	for _, in := range []string{"", "/cdn/images/a.png", "https://x/cdn/zatca/a.xml"} {
		if got := shareXMLLink(in, now); got != "" {
			t.Errorf("shareXMLLink(%q)=%q want empty", in, got)
		}
	}
}

type sentInvoice struct {
	store, to, tpl, lang, name string
	pdf                        []byte
	values                     []string
}

func stubShare(t *testing.T, connected bool, sendErr error) *[]sentInvoice {
	t.Helper()
	var sent []sentInvoice
	oc, os, op := wabaConnected, wabaSendInvoice, saveSharedPDF
	wabaConnected = func(string) bool { return connected }
	wabaSendInvoice = func(store, to, tpl, lang, name string, pdf []byte, values []string) error {
		sent = append(sent, sentInvoice{store, to, tpl, lang, name, pdf, values})
		return sendErr
	}
	saveSharedPDF = func(store, name string, pdf []byte) (string, error) {
		return "/cdn/attachments/invoices/" + store + "/k/" + name, nil
	}
	t.Cleanup(func() { wabaConnected, wabaSendInvoice, saveSharedPDF = oc, os, op })
	return &sent
}

func TestAPI_SaleWhatsAppShare(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	sale := fx.OrderA1.Hex()
	path := "/sales/" + sale

	// status
	stubShare(t, false, nil)
	if r := call(t, "GET", "/whatsapp/status?storeId="+storeA(), tok, nil); r.Code != 200 || r.Body["connected"] != false || r.Body["template"] != "invoice_pdf" {
		t.Fatalf("status off: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/whatsapp/status", tok, nil); r.Code != 400 || r.errField("storeId") == "" {
		t.Fatalf("status without store: %d %s", r.Code, r.Raw)
	}
	other := login(t, fx.UserBEmail)
	if r := call(t, "GET", "/whatsapp/status?storeId="+storeA(), other, nil); r.Code != 403 {
		t.Fatalf("status of another store: %d %s", r.Code, r.Raw)
	}

	ok := M{"storeId": storeA(), "phone": "+966 55 123 4567", "pdf": testPDF, "filename": "S-INV-001.pdf", "lang": "ar",
		"values": []interface{}{"Customer A1", "Invoice S-INV-001", "SAR 115.00"}}
	// not connected → 409, nothing sent
	sent := stubShare(t, false, nil)
	if r := call(t, "POST", path+"/whatsapp", tok, ok); r.Code != 409 || r.errCode() != "whatsapp_not_connected" || len(*sent) != 0 {
		t.Fatalf("not connected: %d %s", r.Code, r.Raw)
	}

	sent = stubShare(t, true, nil)
	if r := call(t, "GET", "/whatsapp/status?storeId="+storeA(), tok, nil); r.Body["connected"] != true {
		t.Fatalf("status on: %s", r.Raw)
	}
	// validation
	for _, bad := range []struct {
		body  M
		field string
	}{
		{M{"storeId": storeA(), "pdf": testPDF}, "phone"},
		{M{"storeId": storeA(), "phone": "0551234567", "pdf": testPDF}, "phone"},
		{M{"storeId": storeA(), "phone": "12", "pdf": testPDF}, "phone"},
		{M{"storeId": storeA(), "phone": "966551234567"}, "pdf"},
		{M{"storeId": storeA(), "phone": "966551234567", "pdf": "aGVsbG8="}, "pdf"},
	} {
		if r := call(t, "POST", path+"/whatsapp", tok, bad.body); r.Code != 400 || r.errField(bad.field) == "" {
			t.Errorf("%v: want 400 on %s, got %d %s", bad.body, bad.field, r.Code, r.Raw)
		}
	}
	if len(*sent) != 0 {
		t.Fatal("invalid requests must not send")
	}
	// sent with invoice_pdf by default
	r := call(t, "POST", path+"/whatsapp", tok, ok)
	if r.Code != 200 || r.Body["sent"] != true || r.Body["to"] != "966551234567" || r.Body["template"] != "invoice_pdf" {
		t.Fatalf("send: %d %s", r.Code, r.Raw)
	}
	if len(*sent) != 1 {
		t.Fatalf("sent %d", len(*sent))
	}
	s := (*sent)[0]
	if s.store != storeA() || s.to != "966551234567" || s.tpl != "invoice_pdf" || s.lang != "ar" || s.name != "S-INV-001.pdf" ||
		!strings.HasPrefix(string(s.pdf), "%PDF-") || len(s.values) != 3 || s.values[2] != "SAR 115.00" {
		t.Fatalf("send args: %+v", s)
	}
	// unknown sale / other store's user
	if r := call(t, "POST", "/sales/000000000000000000000000/whatsapp", tok, ok); r.Code != 404 {
		t.Errorf("unknown sale: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", path+"/whatsapp", other, ok); r.Code != 403 {
		t.Errorf("other store: %d %s", r.Code, r.Raw)
	}
	// template problems → 400, Meta failures → 502
	stubShare(t, true, &controller.WABATemplateError{Msg: `Template "invoice_pdf" is not approved`})
	if r := call(t, "POST", path+"/whatsapp", tok, ok); r.Code != 400 || r.errField("template") == "" {
		t.Errorf("template error: %d %s", r.Code, r.Raw)
	}
	stubShare(t, true, errors.New("meta send template failed (400)"))
	if r := call(t, "POST", path+"/whatsapp", tok, ok); r.Code != 502 || r.errCode() != "whatsapp_error" {
		t.Errorf("meta error: %d %s", r.Code, r.Raw)
	}

	// PDF link for the WhatsApp app
	stubShare(t, false, nil)
	l := call(t, "POST", path+"/pdf-link", tok, M{"storeId": storeA(), "pdf": testPDF})
	if l.Code != 200 || l.Body["url"] != "/cdn/attachments/invoices/"+storeA()+"/k/S-INV-001.pdf" || l.Body["filename"] != "S-INV-001.pdf" {
		t.Fatalf("pdf link: %d %s", l.Code, l.Raw)
	}
	if r := call(t, "POST", path+"/pdf-link", tok, M{"storeId": storeA(), "pdf": "aGVsbG8="}); r.Code != 400 || r.errField("pdf") == "" {
		t.Errorf("pdf link of a non-PDF: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "POST", path+"/pdf-link", other, M{"storeId": storeA(), "pdf": testPDF}); r.Code != 403 {
		t.Errorf("pdf link, other store: %d %s", r.Code, r.Raw)
	}

	// ZATCA XML link: empty until reported, then a long-lived signed link
	ctx, cancel := dbctx()
	defer cancel()
	orders := storeDB(storeA()).Collection("order")
	if x := call(t, "GET", path+"/share-links?storeId="+storeA(), tok, nil); x.Code != 200 || x.Body["xmlUrl"] != "" {
		t.Fatalf("share links before report: %d %s", x.Code, x.Raw)
	}
	xml := "/cdn/zatca/" + storeA() + "/S-INV-001_signed.xml"
	if _, err := orders.UpdateOne(ctx, bson.M{"_id": fx.OrderA1}, bson.M{"$set": bson.M{"zatca.cleared_xml_url": xml}}); err != nil {
		t.Fatal(err)
	}
	defer orders.UpdateOne(ctx, bson.M{"_id": fx.OrderA1}, bson.M{"$unset": bson.M{"zatca.cleared_xml_url": ""}})
	x := call(t, "GET", path+"/share-links?storeId="+storeA(), tok, nil)
	u, err := url.Parse(str(x.Body["xmlUrl"]))
	if x.Code != 200 || err != nil || u.Path != xml || !models.ValidCdnSignature(u.Path, u.Query(), time.Now().Add(5*365*24*time.Hour)) {
		t.Fatalf("share links after report: %d %s", x.Code, x.Raw)
	}
	if r := call(t, "GET", path+"/share-links?storeId="+storeA(), other, nil); r.Code != 403 {
		t.Errorf("share links, other store: %d %s", r.Code, r.Raw)
	}
}

// Store settings → WhatsApp calls /v1/erp/rfq-bot/*: signed in, store-checked,
// errors in the contract envelope.
func TestAPI_RFQBotProxy(t *testing.T) {
	requireDB(t)
	tok := login(t, fx.ManagerEmail)
	if r := call(t, "GET", "/rfq-bot/status?store_id="+storeA(), tok, nil); r.Code != 200 || r.Body["connected"] != false {
		t.Fatalf("status: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/rfq-bot/status", tok, nil); r.Code != 400 {
		t.Fatalf("status without store: %d %s", r.Code, r.Raw)
	}
	other := login(t, fx.UserBEmail)
	if r := call(t, "GET", "/rfq-bot/status?store_id="+storeA(), other, nil); r.Code != 403 {
		t.Fatalf("status of another store: %d %s", r.Code, r.Raw)
	}
	// connecting needs the settings edit permission
	if r := call(t, "POST", "/rfq-bot/connect", tok, M{"store_id": storeA(), "phone_number_id": "1", "access_token": "x"}); r.Code != 403 {
		t.Fatalf("manager connect: %d %s", r.Code, r.Raw)
	}
	tok = login(t, fx.AdminEmail)
	// legacy validation error comes back as the envelope
	r := call(t, "POST", "/rfq-bot/connect", tok, M{"store_id": storeA()})
	if r.Code != 400 || r.errCode() != "whatsapp" || !strings.Contains(str(get(r.Body, "error.message")), "phone_number_id") {
		t.Fatalf("connect without credentials: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "GET", "/rfq-bot/waba-templates?store_id="+storeA(), tok, nil); r.Code != 400 || str(get(r.Body, "error.message")) == "" {
		t.Fatalf("templates when not connected: %d %s", r.Code, r.Raw)
	}
	if r := call(t, "DELETE", "/rfq-bot/disconnect?store_id="+storeA(), tok, nil); r.Code != 200 {
		t.Fatalf("disconnect: %d %s", r.Code, r.Raw)
	}
}
