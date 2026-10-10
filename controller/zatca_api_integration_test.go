//go:build integration

package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// API tests for the ZATCA report and onboarding handlers that need MongoDB
// but not ZATCA:
//
//	go test -tags integration ./controller/ -run 'TestZatca(ReReport|Connect)'

func zatcaTestStore(t *testing.T, env string) (*models.Store, string) {
	t.Helper()
	ctx := context.Background()
	store := &models.Store{
		Name: "ZATCA API Test Store", Code: "ZAPI-" + primitive.NewObjectID().Hex()[:6], Email: "zapi@example.com",
		VATNo: "399999999900003", RegistrationNumber: "4030360927", BusinessCategory: "Supply activities",
		Zatca: models.Zatca{Phase: "2", Env: env},
	}
	if err := store.Insert(); err != nil {
		t.Fatalf("store.Insert: %v", err)
	}
	t.Cleanup(func() { _ = store.PermanentlyDelete() })

	email := "zapi-" + primitive.NewObjectID().Hex() + "@example.com"
	userID := primitive.NewObjectID()
	users := db.Client("").Database(db.GetPosDB()).Collection("user")
	if _, err := users.InsertOne(ctx, bson.M{"_id": userID, "name": "ZATCA API", "email": email, "role": "Admin"}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = users.DeleteOne(ctx, bson.M{"_id": userID}) })
	tok, err := models.GenerateAccesstoken(email)
	if err != nil {
		t.Fatalf("GenerateAccesstoken: %v", err)
	}
	return store, tok.Token
}

func callZatcaHandler(t *testing.T, h http.HandlerFunc, token, target, id, body string) (int, apiResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Authorization", token)
	if id != "" {
		req = mux.SetURLVars(req, map[string]string{"id": id})
	}
	w := httptest.NewRecorder()
	h(w, req)
	var resp apiResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, w.Body.String())
	}
	return w.Code, resp
}

// A document ZATCA already accepted is refused with 409, for every document
// type, and is left as it was.
func TestZatcaReReport_AlreadyReportedIsRefused(t *testing.T) {
	store, token := zatcaTestStore(t, "NonProduction")
	store.Zatca.Connected = true
	if err := store.Update(); err != nil {
		t.Fatal(err)
	}
	storeDB := db.GetDB("store_" + store.ID.Hex())
	t.Cleanup(func() { _ = storeDB.Drop(context.Background()) })

	for _, tc := range []struct {
		name, collection, path string
		handler                http.HandlerFunc
	}{
		{"invoice", "order", "/v1/order/zatca/report/", ReportOrderToZatca},
		{"credit note", "salesreturn", "/v1/sales-return/zatca/report/", ReportSalesReturnToZatca},
		{"debit note (deposit)", "customerdeposit", "/v1/customer-deposit/zatca/report/", ReportCustomerDepositToZatca},
		{"credit note (withdrawal)", "customerwithdrawal", "/v1/customer-withdrawal/zatca/report/", ReportCustomerWithdrawalToZatca},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := primitive.NewObjectID()
			doc := bson.M{"_id": id, "store_id": store.ID, "code": "DOC-" + id.Hex()[:6], "uuid": "u-" + id.Hex(),
				"invoice_count_value": 1, "hash": "accepted-hash",
				"zatca": bson.M{"reporting_passed": true, "icv": 1, "reporting_invoice_hash": "accepted-hash"}}
			if _, err := storeDB.Collection(tc.collection).InsertOne(context.Background(), doc); err != nil {
				t.Fatal(err)
			}
			code, resp := callZatcaHandler(t, tc.handler, token, tc.path+id.Hex()+"?search[store_id]="+store.ID.Hex(), id.Hex(), "")
			if code != http.StatusConflict || resp.Status || resp.Errors["already_reported"] == "" {
				t.Fatalf("HTTP %d status=%v errors=%v, want 409 already_reported", code, resp.Status, resp.Errors)
			}
			var after bson.M
			if err := storeDB.Collection(tc.collection).FindOne(context.Background(), bson.M{"_id": id}).Decode(&after); err != nil {
				t.Fatal(err)
			}
			if after["hash"] != "accepted-hash" {
				t.Fatalf("the refused re-report changed the document's hash to %v", after["hash"])
			}
		})
	}
}

// Onboarding a production store with an OTP ZATCA rejects answers 400, keeps
// the store disconnected with no credentials, and records the failure. The
// onboarding script is replaced by one that prints what csr_and_onboarding.py
// prints when ZATCA's compliance CSID call answers Invalid-OTP.
func TestZatcaConnect_BadOTPInProductionLeavesStoreDisconnected(t *testing.T) {
	store, token := zatcaTestStore(t, "Production")

	dir := t.TempDir()
	sent := filepath.Join(dir, "payload.json")
	script := filepath.Join(dir, "onboard.sh")
	zatcaAnswer := `{"error": "Compliance CSID request failed: {\"errorCode\": \"Invalid-OTP\", \"errorMessage\": \"The provided OTP is invalid\"}"}`
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > '"+sent+"'\nprintf '%s' '"+zatcaAnswer+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	saved := zatcaOnboardingCommand
	zatcaOnboardingCommand = []string{"/bin/sh", script}
	t.Cleanup(func() { zatcaOnboardingCommand = saved })

	code, resp := callZatcaHandler(t, ConnectStoreToZatca, token, "/v1/store/zatca/connect", "",
		`{"id":"`+store.ID.Hex()+`","otp":"000000"}`)
	if code != http.StatusBadRequest || resp.Status || !strings.Contains(resp.Errors["otp"], "Invalid-OTP") {
		t.Fatalf("HTTP %d status=%v errors=%v, want 400 with ZATCA's Invalid-OTP in errors.otp", code, resp.Status, resp.Errors)
	}

	var payload map[string]interface{}
	raw, err := os.ReadFile(sent)
	if err != nil || json.Unmarshal(raw, &payload) != nil {
		t.Fatalf("the onboarding script got no JSON payload: %v %s", err, raw)
	}
	if payload["env"] != "Production" || payload["otp"] != "000000" || payload["vat"] != store.VATNo {
		t.Fatalf("onboarding payload env=%v otp=%v vat=%v", payload["env"], payload["otp"], payload["vat"])
	}

	after, err := models.FindStoreByID(&store.ID, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	z := after.Zatca
	if z.Connected || z.PrivateKey != "" || z.ProductionBinarySecurityToken != "" || z.ProductionSecret != "" {
		t.Fatalf("store after a rejected OTP: connected=%v key=%t token=%t secret=%t", z.Connected,
			z.PrivateKey != "", z.ProductionBinarySecurityToken != "", z.ProductionSecret != "")
	}
	if z.ConnectionFailedCount != 1 || len(z.ConnectionErrors) != 1 || !strings.Contains(z.ConnectionErrors[0], "Invalid-OTP") {
		t.Fatalf("failure not recorded: count=%d errors=%v", z.ConnectionFailedCount, z.ConnectionErrors)
	}
}
