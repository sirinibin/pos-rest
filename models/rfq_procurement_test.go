package models

import (
	"encoding/json"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ── StoreSettings procurement fields ─────────────────────────────────────────

func TestStoreSettings_ProcurementFields_JSON(t *testing.T) {
	s := StoreSettings{
		EnableAIRFQBot:               true,
		BotWhatsAppPhone:             "966501234567",
		BotEvolutionInstanceName:     "rfqbot_test",
		BotEvolutionAPIKey:           "token-bot",
		BotEvolutionAPIURL:           "http://localhost:8081",
		RFQLLMProvider:               "openai",
		RFQLLMModel:                  "gpt-4o-mini",
		RFQLLMAPIKey:                 "sk-test",
		GoogleMapsAPIKey:             "AIzaTest",
		StoreRFQWhatsAppPhone:        "966509876543",
		StoreRFQEvolutionInstanceName: "rfqsend_test",
		StoreRFQEvolutionAPIKey:      "token-send",
		StoreRFQEvolutionAPIURL:      "http://localhost:8081",
	}

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(StoreSettings) failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	checks := map[string]interface{}{
		"enable_ai_rfq_bot":                 true,
		"bot_whatsapp_phone":                "966501234567",
		"bot_evolution_instance_name":       "rfqbot_test",
		"bot_evolution_api_key":             "token-bot",
		"bot_evolution_api_url":             "http://localhost:8081",
		"rfq_llm_provider":                  "openai",
		"rfq_llm_model":                     "gpt-4o-mini",
		"rfq_llm_api_key":                   "sk-test",
		"google_maps_api_key":               "AIzaTest",
		"store_rfq_whatsapp_phone":          "966509876543",
		"store_rfq_evolution_instance_name": "rfqsend_test",
		"store_rfq_evolution_api_key":       "token-send",
		"store_rfq_evolution_api_url":       "http://localhost:8081",
	}
	for key, want := range checks {
		got, ok := m[key]
		if !ok {
			t.Errorf("JSON key %q missing", key)
			continue
		}
		// JSON numbers unmarshal as float64
		switch wv := want.(type) {
		case bool:
			if bv, ok := got.(bool); !ok || bv != wv {
				t.Errorf("key %q: got %v (%T), want %v", key, got, got, want)
			}
		case string:
			if sv, ok := got.(string); !ok || sv != wv {
				t.Errorf("key %q: got %q, want %q", key, got, want)
			}
		}
	}
}

func TestStoreSettings_EnableAIRFQBot_DefaultFalse(t *testing.T) {
	var s StoreSettings
	if s.EnableAIRFQBot {
		t.Error("EnableAIRFQBot should default to false")
	}
}

func TestStoreSettings_ProcurementFieldsEmpty_OmitEmpty(t *testing.T) {
	// When procurement string fields are empty they should be omitted from JSON
	s := StoreSettings{}
	b, _ := json.Marshal(s)
	var m map[string]interface{}
	json.Unmarshal(b, &m)

	omitEmptyKeys := []string{
		"bot_whatsapp_phone", "bot_evolution_instance_name",
		"bot_evolution_api_key", "bot_evolution_api_url",
		"rfq_llm_provider", "rfq_llm_model", "rfq_llm_api_key",
		"google_maps_api_key",
		"store_rfq_whatsapp_phone", "store_rfq_evolution_instance_name",
		"store_rfq_evolution_api_key", "store_rfq_evolution_api_url",
	}
	for _, key := range omitEmptyKeys {
		if _, present := m[key]; present {
			t.Errorf("key %q should be omitted when empty (omitempty)", key)
		}
	}
}

// ── RFQReceived model ─────────────────────────────────────────────────────────

func TestRFQReceived_JSONRoundTrip(t *testing.T) {
	storeID := primitive.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)
	rfq := RFQReceived{
		ID:          primitive.NewObjectID(),
		StoreID:     storeID,
		ReceivedAt:  now,
		FromPhone:   "966501234567",
		FromName:    "Ahmed",
		MessageType: "text",
		TextContent: "I need 100 units of steel pipes",
		Categories:  []string{"Steel Pipes"},
		Status:      "forwarded",
		ForwardedTo: []RFQForwardRecord{
			{
				SupplierName: "ABC Metals",
				Phone:        "966509876543",
				Status:       "sent",
			},
		},
	}

	b, err := json.Marshal(rfq)
	if err != nil {
		t.Fatalf("json.Marshal(RFQReceived) failed: %v", err)
	}

	var out RFQReceived
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal(RFQReceived) failed: %v", err)
	}

	if out.FromPhone != rfq.FromPhone {
		t.Errorf("FromPhone: got %q, want %q", out.FromPhone, rfq.FromPhone)
	}
	if out.Status != rfq.Status {
		t.Errorf("Status: got %q, want %q", out.Status, rfq.Status)
	}
	if len(out.ForwardedTo) != 1 {
		t.Fatalf("ForwardedTo: got %d, want 1", len(out.ForwardedTo))
	}
	if out.ForwardedTo[0].SupplierName != "ABC Metals" {
		t.Errorf("ForwardedTo[0].SupplierName: got %q", out.ForwardedTo[0].SupplierName)
	}
	if len(out.Categories) != 1 || out.Categories[0] != "Steel Pipes" {
		t.Errorf("Categories: got %v, want [Steel Pipes]", out.Categories)
	}
}

func TestRFQForwardRecord_JSONFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	r := RFQForwardRecord{
		SupplierName: "Test Supplier",
		Phone:        "966501234567",
		SentAt:       &now,
		Status:       "sent",
	}

	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal(RFQForwardRecord) failed: %v", err)
	}

	var m map[string]interface{}
	json.Unmarshal(b, &m)

	for _, key := range []string{"supplier_name", "phone", "sent_at", "status"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON key %q missing in RFQForwardRecord", key)
		}
	}
}

// ── RFQSupplier model ─────────────────────────────────────────────────────────

func TestRFQSupplier_JSONRoundTrip(t *testing.T) {
	storeID := primitive.NewObjectID()
	sup := RFQSupplier{
		ID:            primitive.NewObjectID(),
		StoreID:       storeID,
		Name:          "Alpha Steel Trading",
		Phone:         "966501234567",
		Address:       "Riyadh Industrial City",
		Latitude:      24.6877,
		Longitude:     46.7219,
		Categories:    []string{"Steel Pipes", "Valves"},
		Rating:        4.5,
		GooglePlaceID: "ChIJxxxxx",
		Website:       "https://alpha-steel.sa",
		IsActive:      true,
		AddedAt:       time.Now().UTC().Truncate(time.Millisecond),
	}

	b, err := json.Marshal(sup)
	if err != nil {
		t.Fatalf("json.Marshal(RFQSupplier) failed: %v", err)
	}

	var out RFQSupplier
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal(RFQSupplier) failed: %v", err)
	}

	if out.Name != sup.Name {
		t.Errorf("Name: got %q, want %q", out.Name, sup.Name)
	}
	if out.Phone != sup.Phone {
		t.Errorf("Phone: got %q, want %q", out.Phone, sup.Phone)
	}
	if out.Rating != sup.Rating {
		t.Errorf("Rating: got %v, want %v", out.Rating, sup.Rating)
	}
	if len(out.Categories) != 2 {
		t.Errorf("Categories: got %d, want 2", len(out.Categories))
	}
	if !out.IsActive {
		t.Error("IsActive: got false, want true")
	}
}

func TestRFQSupplier_PhoneRequired_JSON(t *testing.T) {
	// The model's phone field has no omitempty, so it always marshals.
	sup := RFQSupplier{Name: "No Phone Co"}
	b, _ := json.Marshal(sup)
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	if _, ok := m["phone"]; !ok {
		t.Error("phone field must be present in JSON even when empty")
	}
}

func TestRFQSupplier_IsActive_DefaultFalse(t *testing.T) {
	var sup RFQSupplier
	if sup.IsActive {
		t.Error("IsActive should default to false (zero value)")
	}
}
