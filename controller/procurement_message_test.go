package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── rfqCreationGate ───────────────────────────────────────────────────────────

func storeWithBot(apiKey string) *models.Store {
	return &models.Store{Settings: models.StoreSettings{
		EnableAIRFQBot: true,
		RFQLLMAPIKey:   apiKey,
	}}
}

func TestRFQCreationGate_AlreadyProcessed(t *testing.T) {
	rfqID := primitive.NewObjectID()
	msg := &models.ProcurementMessage{ProcessedAsRFQ: true, RFQReceivedID: &rfqID}
	status, errMsg := rfqCreationGate(msg, storeWithBot("sk-test"))
	if status != http.StatusConflict {
		t.Errorf("status: got %d, want %d", status, http.StatusConflict)
	}
	if errMsg != "RFQ already created" {
		t.Errorf("errMsg: got %q", errMsg)
	}
}

func TestRFQCreationGate_ProcessedButNoRFQID_NotConflict(t *testing.T) {
	// ProcessedAsRFQ=true but RFQReceivedID=nil — intermediate state, not a conflict.
	msg := &models.ProcurementMessage{ProcessedAsRFQ: true, RFQReceivedID: nil}
	status, _ := rfqCreationGate(msg, storeWithBot("sk-test"))
	if status == http.StatusConflict {
		t.Error("should not be conflict when RFQReceivedID is nil")
	}
}

func TestRFQCreationGate_HasRFQIDButNotMarked_NotConflict(t *testing.T) {
	// ProcessedAsRFQ=false even though RFQReceivedID is set — should not block.
	rfqID := primitive.NewObjectID()
	msg := &models.ProcurementMessage{ProcessedAsRFQ: false, RFQReceivedID: &rfqID}
	status, _ := rfqCreationGate(msg, storeWithBot("sk-test"))
	if status == http.StatusConflict {
		t.Error("should not be conflict when ProcessedAsRFQ is false")
	}
}

func TestRFQCreationGate_AIBotDisabled(t *testing.T) {
	msg := &models.ProcurementMessage{}
	store := &models.Store{Settings: models.StoreSettings{EnableAIRFQBot: false, RFQLLMAPIKey: "sk-test"}}
	status, errMsg := rfqCreationGate(msg, store)
	if status != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", status, http.StatusBadRequest)
	}
	if errMsg != "AI RFQ bot is not enabled for this store" {
		t.Errorf("errMsg: got %q", errMsg)
	}
}

func TestRFQCreationGate_NoLLMAPIKey(t *testing.T) {
	msg := &models.ProcurementMessage{}
	store := &models.Store{Settings: models.StoreSettings{EnableAIRFQBot: true, RFQLLMAPIKey: ""}}
	status, errMsg := rfqCreationGate(msg, store)
	if status != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", status, http.StatusBadRequest)
	}
	if errMsg != "no LLM API key configured" {
		t.Errorf("errMsg: got %q", errMsg)
	}
}

func TestRFQCreationGate_AllClear(t *testing.T) {
	msg := &models.ProcurementMessage{}
	status, errMsg := rfqCreationGate(msg, storeWithBot("sk-test"))
	if status != 0 {
		t.Errorf("status: got %d, want 0 (clear)", status)
	}
	if errMsg != "" {
		t.Errorf("errMsg: got %q, want empty", errMsg)
	}
}

// AlreadyProcessed takes precedence over bot-disabled / missing API key.
func TestRFQCreationGate_ConflictTakesPrecedence(t *testing.T) {
	rfqID := primitive.NewObjectID()
	msg := &models.ProcurementMessage{ProcessedAsRFQ: true, RFQReceivedID: &rfqID}
	store := &models.Store{Settings: models.StoreSettings{EnableAIRFQBot: false, RFQLLMAPIKey: ""}}
	status, _ := rfqCreationGate(msg, store)
	if status != http.StatusConflict {
		t.Errorf("conflict should take precedence: got %d, want %d", status, http.StatusConflict)
	}
}

// ── procurementMessageSource ──────────────────────────────────────────────────

func TestProcurementMessageSource_WhatsApp(t *testing.T) {
	got := procurementMessageSource("whatsapp")
	if got != "whatsapp" {
		t.Errorf("got %q, want %q", got, "whatsapp")
	}
}

func TestProcurementMessageSource_Email(t *testing.T) {
	got := procurementMessageSource("email")
	if got != "email" {
		t.Errorf("got %q, want %q", got, "email")
	}
}

func TestProcurementMessageSource_Unknown_DefaultsToEmail(t *testing.T) {
	for _, typ := range []string{"", "sms", "fax", "unknown"} {
		got := procurementMessageSource(typ)
		if got != "email" {
			t.Errorf("type %q: got %q, want %q", typ, got, "email")
		}
	}
}

// ── Handler HTTP layer ────────────────────────────────────────────────────────

func TestCreateRFQFromProcurementMessageHandler_InvalidID(t *testing.T) {
	cases := []string{"not-a-valid-id", "123", "gggggggggggggggggggggggg", ""}
	for _, id := range cases {
		r := httptest.NewRequest(http.MethodPost, "/v1/procurement-messages/"+id+"/create-rfq", nil)
		r = mux.SetURLVars(r, map[string]string{"id": id})
		w := httptest.NewRecorder()
		CreateRFQFromProcurementMessageHandler(w, r)
		if w.Code != http.StatusBadRequest {
			t.Errorf("id=%q: got %d, want %d", id, w.Code, http.StatusBadRequest)
		}
	}
}

func TestCreateRFQFromProcurementMessageHandler_ValidIDNotInDB_Returns404(t *testing.T) {
	// A syntactically valid ObjectID that does not exist in the DB → 404.
	validID := primitive.NewObjectID().Hex()
	r := httptest.NewRequest(http.MethodPost, "/v1/procurement-messages/"+validID+"/create-rfq", nil)
	r = mux.SetURLVars(r, map[string]string{"id": validID})
	w := httptest.NewRecorder()
	CreateRFQFromProcurementMessageHandler(w, r)
	// Without a live DB the handler returns 404.
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ── Email Signature appending logic ───────────────────────────────────────────

func applyDefaultSignature(body string, sigs []models.EmailSignature) (string, bool) {
	for _, sig := range sigs {
		if sig.IsDefault && sig.Content != "" {
			if sig.IsHtml {
				return body + "<br><br>--<br>" + sig.Content, true
			}
			return body + "\n\n--\n" + sig.Content, false
		}
	}
	return body, false
}

func TestEmailSignature_NoSignatures_BodyUnchanged(t *testing.T) {
	result, isHTML := applyDefaultSignature("Hello world", nil)
	if result != "Hello world" {
		t.Errorf("expected unchanged body, got %q", result)
	}
	if isHTML {
		t.Error("expected isHTML=false when no signatures")
	}
}

func TestEmailSignature_NoDefaultSignature_BodyUnchanged(t *testing.T) {
	sigs := []models.EmailSignature{
		{ID: "1", Name: "Sig A", Content: "Team A", IsDefault: false},
		{ID: "2", Name: "Sig B", Content: "Team B", IsDefault: false},
	}
	result, isHTML := applyDefaultSignature("Hello", sigs)
	if result != "Hello" {
		t.Errorf("expected unchanged body, got %q", result)
	}
	if isHTML {
		t.Error("expected isHTML=false")
	}
}

func TestEmailSignature_DefaultSignatureAppended(t *testing.T) {
	sigs := []models.EmailSignature{
		{ID: "1", Name: "Main", Content: "Best regards\nProcurement Team", IsDefault: true},
	}
	result, isHTML := applyDefaultSignature("Hello", sigs)
	want := "Hello\n\n--\nBest regards\nProcurement Team"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
	if isHTML {
		t.Error("expected isHTML=false for plain-text signature")
	}
}

func TestEmailSignature_FirstDefaultWins(t *testing.T) {
	sigs := []models.EmailSignature{
		{ID: "1", Name: "First Default", Content: "First Sig", IsDefault: true},
		{ID: "2", Name: "Second Default", Content: "Second Sig", IsDefault: true},
	}
	result, _ := applyDefaultSignature("Hi", sigs)
	if result != "Hi\n\n--\nFirst Sig" {
		t.Errorf("got %q", result)
	}
}

func TestEmailSignature_EmptyContentSkipped(t *testing.T) {
	sigs := []models.EmailSignature{
		{ID: "1", Name: "Empty", Content: "", IsDefault: true},
		{ID: "2", Name: "Real", Content: "Team", IsDefault: false},
	}
	result, _ := applyDefaultSignature("Hi", sigs)
	if result != "Hi" {
		t.Errorf("expected unchanged body when default sig content is empty, got %q", result)
	}
}

func TestEmailSignature_HTMLSignatureAppended(t *testing.T) {
	sigs := []models.EmailSignature{
		{ID: "1", Name: "HTML Sig", Content: "<p>Best regards,<br><b>Team</b></p>", IsDefault: true, IsHtml: true},
	}
	result, isHTML := applyDefaultSignature("Hello", sigs)
	want := "Hello<br><br>--<br><p>Best regards,<br><b>Team</b></p>"
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
	if !isHTML {
		t.Error("expected isHTML=true for HTML signature")
	}
}

func TestEmailSignature_HTMLFlagFalse_UsesPlainSeparator(t *testing.T) {
	sigs := []models.EmailSignature{
		{ID: "1", Name: "Plain Sig", Content: "Best regards", IsDefault: true, IsHtml: false},
	}
	result, isHTML := applyDefaultSignature("Hello", sigs)
	if result != "Hello\n\n--\nBest regards" {
		t.Errorf("got %q", result)
	}
	if isHTML {
		t.Error("expected isHTML=false for plain signature")
	}
}
