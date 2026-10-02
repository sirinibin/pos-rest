package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestPDFGenerationHandlers_Unauthenticated verifies that InvoicePDF, ReceiptPDF,
// PostingPDF, and ReportPDF (the generate-and-store handlers) all require JWT auth.
func TestPDFGenerationHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"InvoicePDF", InvoicePDF},
		{"ReceiptPDF", ReceiptPDF},
		{"PostingPDF", PostingPDF},
		{"ReportPDF", ReportPDF},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", res.StatusCode)
			}
			if !strings.Contains(res.Header.Get("Content-Type"), "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", res.Header.Get("Content-Type"))
			}
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
			}
		})
	}
}

// TestPrintDataHandlers_UnknownKey verifies that InvoicePrintData, ReceiptPrintData,
// PostingPrintData, and ReportPrintData return 404 for an unknown print-job key.
// These handlers have no JWT auth guard — they use a print-job key from printJobStore.
func TestPrintDataHandlers_UnknownKey(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"InvoicePrintData", InvoicePrintData},
		{"ReceiptPrintData", ReceiptPrintData},
		{"PostingPrintData", PostingPrintData},
		{"ReportPrintData", ReportPrintData},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(""))
			r = mux.SetURLVars(r, map[string]string{"key": "nonexistent-key-xyz"})
			w := httptest.NewRecorder()
			tc.handler(w, r)

			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for unknown key, got %d", w.Code)
			}
		})
	}
}

// TestSavePdf_NoFile verifies SavePdf returns 400 when no file is provided.
// SavePdf has no JWT auth guard — it parses a multipart form first.
func TestSavePdf_NoFile(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/pdf/save", strings.NewReader(""))
	w := httptest.NewRecorder()
	SavePdf(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestSharePdf_NoFile verifies SharePdf returns 400 when no file is provided.
func TestSharePdf_NoFile(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/pdf/share", strings.NewReader(""))
	w := httptest.NewRecorder()
	SharePdf(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestProxyImageHandler_NoURL verifies ProxyImageHandler returns 400 when
// the url query param is missing. This handler has no JWT auth guard.
func TestProxyImageHandler_NoURL(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/proxy-image", nil)
	w := httptest.NewRecorder()
	ProxyImageHandler(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing url, got %d", w.Code)
	}
}

// TestTranslateHandler_Unauthenticated verifies TranslateHandler returns 401
// without a token. It uses http.Error (plain text), not a JSON response.
func TestTranslateHandler_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/translate", strings.NewReader(""))
	w := httptest.NewRecorder()
	TranslateHandler(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestGetProductLastPurchasePrice_Unauthenticated verifies the handler returns
// standard 401 + errors["access_token"] without a token.
func TestGetProductLastPurchasePrice_Unauthenticated(t *testing.T) {
	id := "64abc123456789001234abcd"
	r := httptest.NewRequest(http.MethodGet, "/v1/product/"+id+"/last-purchase-price", nil)
	r = mux.SetURLVars(r, map[string]string{"id": id})
	w := httptest.NewRecorder()
	GetProductLastPurchasePrice(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["access_token"]; !ok {
		t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
	}
}

// TestAdminServerHandlers_Unauthenticated verifies GetServerStatusHandler,
// RestartServerHandler, and RepairFrontendHandler return 401 with
// {"error":"unauthorized"} (requireAdminStateless format) without a token.
func TestAdminServerHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"GetServerStatusHandler", http.MethodGet, GetServerStatusHandler},
		{"RestartServerHandler", http.MethodPost, RestartServerHandler},
		{"RepairFrontendHandler", http.MethodPost, RepairFrontendHandler},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(""))
			w := httptest.NewRecorder()
			tc.handler(w, r)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
			var resp map[string]string
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp["error"] == "" {
				t.Errorf("expected non-empty error key, got %v", resp)
			}
		})
	}
}

// TestVerifyAndCleanupDiskHandler_Unauthenticated verifies the handler returns
// 401 + {"error":"unauthorized"} without a token.
func TestVerifyAndCleanupDiskHandler_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/admin/verify-cleanup-disk", strings.NewReader(""))
	w := httptest.NewRecorder()
	VerifyAndCleanupDiskHandler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] == "" {
		t.Errorf("expected non-empty error key, got %v", resp)
	}
}

// TestHandleMetaWhatsAppWebhook_NoAuthGuard documents that the Meta webhook
// handler has no JWT auth guard (it uses a Meta verify token instead).
// A GET request without any params returns 200 (verify) or similar, not 401.
func TestHandleMetaWhatsAppWebhook_NoAuthGuard(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/meta/whatsapp/webhook", nil)
	w := httptest.NewRecorder()
	HandleMetaWhatsAppWebhook(w, r)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("Meta webhook handler should not use JWT auth — got 401 unexpectedly")
	}
}

// TestWebSocketHandler_NoAuthGuard documents that WebSocketHandler has no JWT
// auth guard. A plain HTTP request (not a WebSocket upgrade) results in a non-401
// error from the upgrader.
func TestWebSocketHandler_NoAuthGuard(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	w := httptest.NewRecorder()
	WebSocketHandler(w, r)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("WebSocketHandler should not use JWT auth — got 401 unexpectedly")
	}
}
