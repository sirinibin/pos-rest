package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// ── WhatsApp integration stubs ────────────────────────────────────────────────
//
// None of the handlers in whatsapp.go (ConnectWhatsApp, GetWhatsAppQR,
// GetWhatsAppStatus, DisconnectWhatsApp, SendWhatsAppDocument,
// CheckWhatsAppNumbers, GetWhatsAppContacts, GetWhatsAppContactsCount,
// SyncWhatsAppContacts, ClearWhatsAppContacts) call
// models.AuthenticateByAccessToken. Authentication is implicit: each handler
// requires a valid store_id that maps to a configured Evolution API instance.
// Unit tests for these handlers require a live Evolution API and are therefore
// out of scope for this file.

// ── rfq_email.go integration stubs ───────────────────────────────────────────
//
// None of the handlers in rfq_email.go (ConnectRFQEmail, GetRFQEmailStatus,
// DisconnectRFQEmail, HandleRFQEmailOAuthCallback, HandleRFQEmailWebhook,
// GetRFQEmailAccounts, ConnectRFQEmailAccount, DisconnectRFQEmailAccount,
// UpdateRFQEmailAccountSettings, PollRFQEmailAccountStatus,
// TestRFQEmailIMAPHandler) call models.AuthenticateByAccessToken. They rely on
// store_id resolution and external email / OAuth providers. Unit-level tests are
// out of scope here.

// ── rfq_sse.go integration stub ──────────────────────────────────────────────
//
// RFQEventsHandler has no authentication guard. It expects a store_id query
// parameter and upgrades the connection to an SSE stream. Testing it requires
// streaming-aware infrastructure and is an integration concern.

// ── rfq_pdf.go ────────────────────────────────────────────────────────────────

// TestRFQReceivedPrintData_NoKey verifies that RFQReceivedPrintData returns
// HTTP 404 when no matching print-job key is present in the in-memory store.
// This handler has no authentication guard; it looks up a short-lived key
// injected by the PDF-generation flow.
func TestRFQReceivedPrintData_NoKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/rfq/print/nonexistent-key", nil)
	r = mux.SetURLVars(r, map[string]string{"key": "nonexistent-key"})
	w := httptest.NewRecorder()

	RFQReceivedPrintData(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for missing print key, got %d", res.StatusCode)
	}
}

// TestRFQReceivedPDF_Unauthenticated confirms that POST /v1/rfq/pdf returns
// HTTP 401 with {"status":false,"errors":{"access_token":…}} when no token is
// provided.
func TestRFQReceivedPDF_Unauthenticated(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/rfq/pdf", strings.NewReader(""))
	w := httptest.NewRecorder()

	RFQReceivedPDF(w, r)

	res := w.Result()
	defer res.Body.Close()

	// ── Status code ───────────────────────────────────────────────────────────
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", res.StatusCode)
	}

	// ── Content-Type ──────────────────────────────────────────────────────────
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// ── Body ──────────────────────────────────────────────────────────────────
	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if resp.Status {
		t.Error("expected status=false in body")
	}

	if _, ok := resp.Errors["access_token"]; !ok {
		t.Errorf("expected errors.access_token key, got errors=%v", resp.Errors)
	}
}

// ── rfq_quotation.go ──────────────────────────────────────────────────────────

// TestRFQSupplierHandlers_Unauthenticated verifies that the supplier-reply
// endpoints return HTTP 401 when no access token is provided.
//
// These handlers use http.Error to write the 401 response, so the body is the
// literal string {"error":"unauthorized"} (text/plain) rather than the standard
// structured JSON body used by other handlers in this package. Only the status
// code is asserted here.
func TestRFQSupplierHandlers_Unauthenticated(t *testing.T) {
	const rfqID = "64abc123456789001234abcd"
	muxWithRFQID := map[string]string{"id": rfqID}

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		{
			name:    "ListSupplierRepliesHandler",
			method:  http.MethodGet,
			path:    "/v1/rfq/" + rfqID + "/supplier-replies",
			handler: ListSupplierRepliesHandler,
			muxVars: muxWithRFQID,
		},
		{
			name:    "AddManualSupplierReplyHandler",
			method:  http.MethodPost,
			path:    "/v1/rfq/" + rfqID + "/supplier-replies",
			handler: AddManualSupplierReplyHandler,
			muxVars: muxWithRFQID,
		},
		{
			name:    "DeleteSupplierReplyHandler",
			method:  http.MethodDelete,
			path:    "/v1/rfq/" + rfqID + "/supplier-replies",
			handler: DeleteSupplierReplyHandler,
			muxVars: muxWithRFQID,
		},
		{
			name:    "ParseQuotationFileHandler",
			method:  http.MethodPost,
			path:    "/v1/rfq/" + rfqID + "/parse-file",
			handler: ParseQuotationFileHandler,
			muxVars: muxWithRFQID,
		},
		{
			name:    "UpdateProductPricesFromRFQHandler",
			method:  http.MethodPost,
			path:    "/v1/rfq/" + rfqID + "/update-prices",
			handler: UpdateProductPricesFromRFQHandler,
			muxVars: muxWithRFQID,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}
		})
	}
}
