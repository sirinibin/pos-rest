package controller

import (
	"bytes"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Rule: whenever the official WhatsApp API (WABA) is connected, every WhatsApp
// send goes through it.

func TestPickWABASender(t *testing.T) {
	cases := []struct {
		name                 string
		rID, rTok, bID, bTok string
		wantID, wantTok      string
	}{
		{"store RFQ number wins", "r1", "rt", "b1", "bt", "r1", "rt"},
		{"bot when store RFQ unset", "", "", "b1", "bt", "b1", "bt"},
		{"bot when store RFQ has no token", "r1", "", "b1", "bt", "b1", "bt"},
		{"bot when store RFQ has no phone id", "", "rt", "b1", "bt", "b1", "bt"},
		{"none connected", "", "", "", "", "", ""},
		{"half-configured bot is not connected", "", "", "b1", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, tok := pickWABASender(tc.rID, tc.rTok, tc.bID, tc.bTok)
			if id != tc.wantID || tok != tc.wantTok {
				t.Errorf("got (%q,%q) want (%q,%q)", id, tok, tc.wantID, tc.wantTok)
			}
		})
	}
}

func TestWABARecipient(t *testing.T) {
	cases := map[string]string{
		"966551234567":                "966551234567",
		"+966 55 123 4567":            "966551234567",
		"966551234567@s.whatsapp.net": "966551234567",
		"  966551234567 ":             "966551234567",
		"123456789012345@lid":         "",
		"":                            "",
	}
	for in, want := range cases {
		if got := wabaRecipient(in); got != want {
			t.Errorf("wabaRecipient(%q)=%q want %q", in, got, want)
		}
	}
}

type docSend struct{ phoneID, token, to, dataURI, mime, filename, caption string }

func stubWABA(t *testing.T, phoneID, token string, sendErr error) *[]docSend {
	t.Helper()
	var sent []docSend
	oldCfg, oldSend := wabaSendConfig, sendWABADocument
	wabaSendConfig = func(string) (string, string) { return phoneID, token }
	sendWABADocument = func(p, tk, to, uri, mime, fn, cap string) error {
		sent = append(sent, docSend{p, tk, to, uri, mime, fn, cap})
		return sendErr
	}
	t.Cleanup(func() { wabaSendConfig, sendWABADocument = oldCfg, oldSend })
	return &sent
}

func sendDocRequest(t *testing.T, phone string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "inv.pdf")
	fw.Write([]byte("%PDF-1.4 test"))
	mw.WriteField("phone", phone)
	mw.WriteField("caption", "Invoice INV-1")
	mw.WriteField("filename", "waba-first-test.pdf")
	mw.WriteField("store_id", "")
	mw.Close()
	r := httptest.NewRequest("POST", "/v1/whatsapp/send-document", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func TestSendWhatsAppDocument_UsesWABAWhenConnected(t *testing.T) {
	sent := stubWABA(t, "1098765432101", "tok", nil)
	w := httptest.NewRecorder()
	SendWhatsAppDocument(w, sendDocRequest(t, "+966 55 123 4567"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"channel":"waba"`) {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	if len(*sent) != 1 {
		t.Fatalf("WABA sends=%d want 1", len(*sent))
	}
	s := (*sent)[0]
	want := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 test"))
	if s.phoneID != "1098765432101" || s.token != "tok" || s.to != "966551234567" || s.dataURI != want ||
		s.mime != "application/pdf" || s.filename != "waba-first-test.pdf" || s.caption != "Invoice INV-1" {
		t.Errorf("unexpected send %+v", s)
	}
}

func TestSendWhatsAppDocument_WABAErrorIsReportedNotFallenBack(t *testing.T) {
	stubWABA(t, "1098765432101", "tok", errors.New("template required"))
	w := httptest.NewRecorder()
	SendWhatsAppDocument(w, sendDocRequest(t, "966551234567"))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "WhatsApp API error: template required") {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSendWhatsAppDocument_LIDContactWithWABA(t *testing.T) {
	sent := stubWABA(t, "1098765432101", "tok", nil)
	w := httptest.NewRecorder()
	SendWhatsAppDocument(w, sendDocRequest(t, "123456789012345@lid"))
	if w.Code != http.StatusBadRequest || len(*sent) != 0 {
		t.Fatalf("code=%d sends=%d body=%s", w.Code, len(*sent), w.Body.String())
	}
}

func TestSendWhatsAppDocument_NoWABANeverCallsMeta(t *testing.T) {
	sent := stubWABA(t, "", "", nil)
	w := httptest.NewRecorder()
	SendWhatsAppDocument(w, sendDocRequest(t, "966551234567"))
	if len(*sent) != 0 {
		t.Fatalf("WABA must not be used when not connected")
	}
	if strings.Contains(w.Body.String(), `"channel":"waba"`) {
		t.Fatalf("body=%s", w.Body.String())
	}
}
