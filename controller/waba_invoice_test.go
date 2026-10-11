package controller

import (
	"errors"
	"strings"
	"testing"
)

func TestValidWABANumber(t *testing.T) {
	for in, want := range map[string]bool{
		"966551234567": true, "96891234567": true, "12345678": true, "123456789012345": true,
		"1234567": false, "1234567890123456": false, "0551234567": false, "+966551234567": false, "": false,
	} {
		if got := ValidWABANumber(in); got != want {
			t.Errorf("ValidWABANumber(%q)=%v want %v", in, got, want)
		}
	}
}

func docTemplate(name, lang, body string) WABATemplate {
	return WABATemplate{Name: name, Status: "APPROVED", Language: lang, Components: []WABATemplateComponent{
		{Type: "HEADER", Format: "DOCUMENT"}, {Type: "BODY", Text: body}}}
}

func TestPickWABATemplate(t *testing.T) {
	list := []WABATemplate{
		{Name: "invoice_pdf", Status: "REJECTED", Language: "ar"},
		docTemplate("invoice_pdf", "en_US", "Hi {{1}}"),
		docTemplate("invoice_pdf", "ar", "مرحبا {{1}}"),
		docTemplate("quotation_pdf", "en", "Q {{1}}"),
	}
	cases := []struct{ name, lang, wantLang string }{
		{"invoice_pdf", "ar", "ar"},
		{"invoice_pdf", "en", "en_US"},
		{"invoice_pdf", "fr", "en_US"}, // no French: the first approved one
		{"invoice_pdf", "", "en_US"},
	}
	for _, c := range cases {
		got, ok := pickWABATemplate(list, c.name, c.lang)
		if !ok || got.Language != c.wantLang {
			t.Errorf("pick(%s,%s)=%v,%v want %s", c.name, c.lang, got.Language, ok, c.wantLang)
		}
	}
	if _, ok := pickWABATemplate(list, "missing", "en"); ok {
		t.Error("missing template found")
	}
	if _, ok := pickWABATemplate(list[:1], "invoice_pdf", "ar"); ok {
		t.Error("a rejected template must not be used")
	}
}

func TestWABADocumentComponents(t *testing.T) {
	comps, err := wabaDocumentComponents(docTemplate("invoice_pdf", "en", "Dear {{1}}, {{2}} for {{3}}."), "M1", "S-1.pdf", []string{"Ali", "Invoice S-1"})
	if err != nil || len(comps) != 2 {
		t.Fatalf("comps %v err %v", comps, err)
	}
	h := comps[0].(map[string]interface{})
	doc := h["parameters"].([]interface{})[0].(map[string]interface{})["document"].(map[string]interface{})
	if h["type"] != "header" || doc["id"] != "M1" || doc["filename"] != "S-1.pdf" {
		t.Errorf("header %v", h)
	}
	params := comps[1].(map[string]interface{})["parameters"].([]interface{})
	if len(params) != 3 || params[0].(map[string]interface{})["text"] != "Ali" || params[2].(map[string]interface{})["text"] != "-" {
		t.Errorf("body %v", params)
	}
	// a body without placeholders sends no body component
	if comps, _ := wabaDocumentComponents(docTemplate("t", "en", "Your invoice."), "M", "f.pdf", nil); len(comps) != 1 {
		t.Errorf("no-variable body: %v", comps)
	}
	for _, tpl := range []WABATemplate{
		{Name: "text_header", Components: []WABATemplateComponent{{Type: "HEADER", Format: "TEXT"}, {Type: "BODY", Text: "x"}}},
		{Name: "no_header", Components: []WABATemplateComponent{{Type: "BODY", Text: "x"}}},
	} {
		var te *WABATemplateError
		if _, err := wabaDocumentComponents(tpl, "M", "f.pdf", nil); !errors.As(err, &te) {
			t.Errorf("%s: want a template error, got %v", tpl.Name, err)
		}
	}
}

type tplSend struct {
	phoneID, token, to, name, lang string
	comps                          []interface{}
}

func stubTemplateSend(t *testing.T, list []WABATemplate) (*[]tplSend, *int) {
	t.Helper()
	var sends []tplSend
	uploads := 0
	oc, ol, ou, os := wabaSendConfig, wabaTemplatesFor, wabaUploadMedia, wabaSendTemplate
	wabaSendConfig = func(string) (string, string) { return "PH1", "TOK" }
	wabaTemplatesFor = func(string, string, string) ([]WABATemplate, error) { return list, nil }
	wabaUploadMedia = func(p, tk, mime, fn string, data []byte) (string, error) {
		uploads++
		if mime != "application/pdf" || !strings.HasPrefix(string(data), "%PDF") {
			t.Errorf("upload %s %q", mime, data)
		}
		return "MEDIA9", nil
	}
	wabaSendTemplate = func(p, tk, to, name, lang string, comps []interface{}) error {
		sends = append(sends, tplSend{p, tk, to, name, lang, comps})
		return nil
	}
	t.Cleanup(func() { wabaSendConfig, wabaTemplatesFor, wabaUploadMedia, wabaSendTemplate = oc, ol, ou, os })
	return &sends, &uploads
}

func TestSendWABATemplateDocument(t *testing.T) {
	sends, uploads := stubTemplateSend(t, []WABATemplate{docTemplate("invoice_pdf", "ar", "مرحبا {{1}} {{2}}")})
	if err := SendWABATemplateDocument("s1", "966551234567", "invoice_pdf", "ar", "S-1.pdf", []byte("%PDF-1.4"), []string{"Ali", "S-1"}); err != nil {
		t.Fatal(err)
	}
	if *uploads != 1 || len(*sends) != 1 {
		t.Fatalf("uploads %d sends %d", *uploads, len(*sends))
	}
	s := (*sends)[0]
	if s.phoneID != "PH1" || s.to != "966551234567" || s.name != "invoice_pdf" || s.lang != "ar" || len(s.comps) != 2 {
		t.Errorf("send %+v", s)
	}

	// unknown template / bad header: refused before anything is uploaded
	sends, uploads = stubTemplateSend(t, []WABATemplate{{Name: "invoice_pdf", Status: "APPROVED", Language: "en",
		Components: []WABATemplateComponent{{Type: "BODY", Text: "x"}}}})
	for _, name := range []string{"invoice_pdf", "nope"} {
		var te *WABATemplateError
		if err := SendWABATemplateDocument("s1", "966551234567", name, "en", "S-1.pdf", []byte("%PDF"), nil); !errors.As(err, &te) {
			t.Errorf("%s: want template error, got %v", name, err)
		}
	}
	if *uploads != 0 || len(*sends) != 0 {
		t.Error("nothing may be uploaded or sent for a bad template")
	}

	// not connected
	old := wabaSendConfig
	wabaSendConfig = func(string) (string, string) { return "", "" }
	defer func() { wabaSendConfig = old }()
	if err := SendWABATemplateDocument("s1", "966551234567", "invoice_pdf", "en", "S-1.pdf", []byte("%PDF"), nil); !errors.Is(err, ErrWABANotConnected) {
		t.Errorf("want ErrWABANotConnected, got %v", err)
	}
	if WABAConnected("s1") {
		t.Error("WABAConnected without a sender")
	}
}

func TestSaveSharedInvoicePDF(t *testing.T) {
	var key string
	old := saveInvoiceFile
	saveInvoiceFile = func(k string, data []byte, ct string) string {
		key = k
		if ct != "application/pdf" {
			t.Errorf("content type %s", ct)
		}
		return "/cdn/" + k
	}
	defer func() { saveInvoiceFile = old }()
	store := "5f1d7f5e8f1b2c3d4e5f6a7b"
	l1, err := SaveSharedInvoicePDF(store, "S-1.pdf", []byte("%PDF"))
	if err != nil || !strings.HasPrefix(key, "attachments/invoices/"+store+"/") || !strings.HasSuffix(key, "/S-1.pdf") || l1 != "/cdn/"+key {
		t.Fatalf("link %q key %q err %v", l1, key, err)
	}
	l2, _ := SaveSharedInvoicePDF(store, "S-1.pdf", []byte("%PDF"))
	if l1 == l2 || len(strings.Split(key, "/")[3]) != 32 {
		t.Errorf("links must be random per share: %q %q", l1, l2)
	}
	if _, err := SaveSharedInvoicePDF("../x", "a.pdf", nil); err == nil {
		t.Error("invalid store accepted")
	}
	saveInvoiceFile = func(string, []byte, string) string { return "" }
	if _, err := SaveSharedInvoicePDF(store, "a.pdf", nil); err == nil {
		t.Error("storage failure must be an error")
	}
}
