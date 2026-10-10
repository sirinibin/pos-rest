package erp

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Adapter tests against fake provider servers (wire format + mapping).

type fakeCall struct {
	Method, Path string
	Header       http.Header
	Body         map[string]interface{}
}

type fakeProvider struct {
	mu    sync.Mutex
	calls []fakeCall
	reply func(c fakeCall) (int, interface{})
}

func (f *fakeProvider) server(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c := fakeCall{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone()}
		_ = json.Unmarshal(b, &c.Body)
		f.mu.Lock()
		f.calls = append(f.calls, c)
		f.mu.Unlock()
		code, body := f.reply(c)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (f *fakeProvider) last() fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func testRSAKey(t *testing.T) (string, *rsa.PrivateKey) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})), k
}

func TestSignRS256_Verifies(t *testing.T) {
	pemText, key := testRSAKey(t)
	tok, err := signRS256(pemText, map[string]interface{}{"data": map[string]string{"ops": "auth", "merchant_uuid": "m1", "terminal_id": "t1"}})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt: %s", tok)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("signature: %v", err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if !strings.Contains(string(payload), `"ops":"auth"`) || !strings.Contains(string(payload), `"terminal_id":"t1"`) {
		t.Fatalf("payload: %s", payload)
	}
	// PKCS#8 keys work too
	b8, _ := x509.MarshalPKCS8PrivateKey(key)
	if _, err := signRS256(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b8})), map[string]interface{}{}); err != nil {
		t.Fatalf("pkcs8: %v", err)
	}
	for _, bad := range []string{"", "not a pem", "-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----"} {
		if _, err := signRS256(bad, map[string]interface{}{}); err == nil {
			t.Fatalf("bad key %q must fail", bad)
		}
	}
}

func TestNearpayAdapter(t *testing.T) {
	pemText, _ := testRSAKey(t)
	pairs := 0
	f := &fakeProvider{}
	f.reply = func(c fakeCall) (int, interface{}) {
		switch c.Path {
		case "/pair/jwt":
			pairs++
			if str(c.Body["jwt"]) == "" {
				return 400, nil
			}
			return 201, M{"room_id": "room-1", "token": "tok-" + str(pairs)}
		case "/ping":
			return 201, M{}
		case "/purchase":
			if c.Header.Get("Authorization") == "Bearer tok-1" && pairs == 1 && num(c.Body["amount"]) == 4321 {
				return 401, M{"message": "expired"} // first session refused → re-pair
			}
			switch num(c.Body["amount"]) {
			case 999:
				return 408, M{"message": "timeout"}
			case 500:
				return 201, M{"status": 1, "transactionReceipts": []M{{"transaction_uuid": "tx-d", "is_approved": false, "status_message": "Declined", "pan": "4111********1111"}}}
			case 600:
				return 201, M{"status": 1, "transactionReceipts": []M{}, "message": "no receipt"}
			}
			return 201, M{"status": 1, "transactionReceipts": []M{{"transaction_uuid": "tx-1", "is_approved": true, "approval_code": "123456",
				"status_message": "Approved", "pan": "5243 **** **** 9871", "card_scheme": "MADA", "retrieval_reference_number": "000111222333"}}}
		case "/cancel":
			return 201, M{"command": 8, "status": 1}
		}
		return 404, nil
	}
	s := f.server(t)
	nearpaySessions = map[string]nearpaySession{}
	cfg := terminalCfg{Env: "sandbox", BaseURL: s.URL + "/", Partner: map[string]string{"clientPrivateKey": pemText},
		Store: map[string]string{"merchantUuid": "m-1", "terminalId": "0211"}, Terminal: "0211", HTTP: s.Client()}
	a := nearpayAdapter{}
	ctx := context.Background()
	if err := a.Check(ctx, cfg); err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := a.Check(ctx, terminalCfg{Partner: map[string]string{}}); err == nil {
		t.Fatal("check without StartERP key must fail")
	}
	req := terminalPayReq{PaymentID: "p1", Reference: "POS-9", Amount: 7, Decimals: 2, Currency: "SAR"}
	r, err := a.Run(ctx, cfg, req)
	if err != nil || r.Status != tpApproved || r.AuthCode != "123456" || r.RRN != "000111222333" || r.MaskedPan != "••••9871" || r.Scheme != "mada" || r.ProviderRef != "tx-1" {
		t.Fatalf("approved: %+v %v", r, err)
	}
	lc := f.last()
	if num(lc.Body["amount"]) != 700 || lc.Body["jobId"] != "p1" || lc.Body["customer_reference_number"] != "POS-9" || lc.Header.Get("Authorization") != "Bearer tok-1" || lc.Header.Get("x-room-id") != "room-1" {
		t.Fatalf("purchase wire: %+v", lc)
	}
	if pairs != 1 {
		t.Fatalf("session must be cached, paired %d times", pairs)
	}
	// refused session → re-pair once and retry
	r, err = a.Run(ctx, cfg, terminalPayReq{PaymentID: "p2", Amount: 43.21, Decimals: 2})
	if err != nil || r.Status != tpApproved || pairs != 2 {
		t.Fatalf("re-pair: %+v %v pairs=%d", r, err, pairs)
	}
	if r, _ := a.Run(ctx, cfg, terminalPayReq{PaymentID: "p3", Amount: 9.99, Decimals: 2}); r.Status != tpTimeout {
		t.Fatalf("408 → timeout: %+v", r)
	}
	if r, _ := a.Run(ctx, cfg, terminalPayReq{PaymentID: "p4", Amount: 5, Decimals: 2}); r.Status != tpDeclined || r.MaskedPan != "••••1111" {
		t.Fatalf("declined: %+v", r)
	}
	if r, _ := a.Run(ctx, cfg, terminalPayReq{PaymentID: "p5", Amount: 6, Decimals: 2}); r.Status != tpFailed || r.Message != "no receipt" {
		t.Fatalf("no receipt: %+v", r)
	}
	if r, err := a.Cancel(ctx, cfg, req, "run_p1"); err != nil || r.Status != tpCancelled || f.last().Path != "/cancel" {
		t.Fatalf("cancel: %+v %v", r, err)
	}
	// live without a production URL configured uses the catalog URL; with one, that URL
	live := cfg
	live.Env, live.BaseURL = "live", "https://x.invalid"
	live.Partner = map[string]string{"clientPrivateKey": pemText, "liveBaseUrl": s.URL}
	if b, _ := a.base(live); b != s.URL {
		t.Fatalf("live base: %s", b)
	}
}

func TestAdyenAdapter(t *testing.T) {
	f := &fakeProvider{}
	f.reply = func(c fakeCall) (int, interface{}) {
		if c.Header.Get("x-API-key") != "key-1" {
			return 401, nil
		}
		req := sub(c.Body, "SaleToPOIRequest")
		h := sub(req, "MessageHeader")
		switch str(h["MessageCategory"]) {
		case "Diagnosis":
			return 200, M{"SaleToPOIResponse": M{"DiagnosisResponse": M{"Response": M{"Result": "Success"}}}}
		case "Abort":
			return 200, nil
		case "Payment":
			amt := num(get(req, "PaymentRequest.PaymentTransaction.AmountsReq.RequestedAmount"))
			resp := M{"Result": "Success"}
			switch amt {
			case 20.05:
				resp = M{"Result": "Failure", "ErrorCondition": "Refusal", "AdditionalResponse": "refusalReason=Not%20enough%20balance"}
			case 20.06:
				resp = M{"Result": "Failure", "ErrorCondition": "Aborted"}
			case 20.07:
				resp = M{"Result": "Failure", "ErrorCondition": "Busy"}
			case 20.08:
				resp = M{"Result": "Partial"}
			}
			return 200, M{"SaleToPOIResponse": M{"PaymentResponse": M{
				"Response": resp,
				"POIData":  M{"POITransactionID": M{"TransactionID": "BV0q001"}},
				"PaymentResult": M{
					"PaymentAcquirerData":   M{"ApprovalCode": "A1B2C3", "AcquirerTransactionID": M{"TransactionID": "8816"}},
					"PaymentInstrumentData": M{"CardData": M{"MaskedPan": "411111 **** 1111", "PaymentBrand": "visa"}},
				},
			}}}
		}
		return 400, nil
	}
	s := f.server(t)
	cfg := terminalCfg{Env: "sandbox", BaseURL: s.URL + "/v1", Store: map[string]string{"merchantAccount": "ShopAE", "apiKey": "key-1"},
		Terminal: "S1F2-000158", HTTP: s.Client()}
	a := adyenAdapter{}
	ctx := context.Background()
	if err := a.Check(ctx, cfg); err != nil {
		t.Fatalf("check: %v", err)
	}
	if f.last().Path != "/v1/merchants/ShopAE/devices/S1F2-000158/sync" {
		t.Fatalf("path: %s", f.last().Path)
	}
	bad := cfg
	bad.Store = map[string]string{"merchantAccount": "ShopAE", "apiKey": "nope"}
	if err := a.Check(ctx, bad); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad key: %v", err)
	}
	req := terminalPayReq{PaymentID: "65f0a1b2c3d4e5f600112233", Reference: "INV-7", Amount: 10.99, Currency: "AED", Decimals: 2}
	r, err := a.Run(ctx, cfg, req)
	if err != nil || r.Status != tpApproved || r.AuthCode != "A1B2C3" || r.MaskedPan != "••••1111" || r.Scheme != "visa" || r.ProviderRef != "BV0q001" {
		t.Fatalf("approved: %+v %v", r, err)
	}
	h := sub(sub(f.last().Body, "SaleToPOIRequest"), "MessageHeader")
	tx := get(f.last().Body, "SaleToPOIRequest.PaymentRequest.SaleData.SaleTransactionID.TransactionID")
	if h["ServiceID"] != "f600112233" || h["SaleID"] != "StartERP" || h["POIID"] != "S1F2-000158" || h["ProtocolVersion"] != "3.0" || tx != "INV-7" ||
		get(f.last().Body, "SaleToPOIRequest.PaymentRequest.PaymentTransaction.AmountsReq.Currency") != "AED" {
		t.Fatalf("payment wire: %v", f.last().Body)
	}
	for amt, want := range map[float64]string{20.05: tpDeclined, 20.06: tpCancelled, 20.07: tpFailed, 20.08: tpDeclined} {
		r, _ := a.Run(ctx, cfg, terminalPayReq{PaymentID: "p", Amount: amt, Currency: "AED", Decimals: 2})
		if r.Status != want {
			t.Errorf("%v: %s want %s (%s)", amt, r.Status, want, r.Message)
		}
		if amt == 20.05 && r.Message != "Not enough balance" {
			t.Errorf("refusal reason: %q", r.Message)
		}
	}
	if r, err := a.Cancel(ctx, cfg, req, "run_x"); err != nil || r.Status != tpCancelled {
		t.Fatalf("abort: %+v %v", r, err)
	}
	ab := sub(sub(f.last().Body, "SaleToPOIRequest"), "AbortRequest")
	if get(ab, "MessageReference.ServiceID") != "f600112233" || ab["AbortReason"] != "MerchantAbort" {
		t.Fatalf("abort wire: %v", ab)
	}
	// live regions
	live := cfg
	live.Env, live.BaseURL = "live", "https://device-api-live.adyen.com/v1"
	for region, want := range map[string]string{"": "https://device-api-live.adyen.com/v1", "eu": "https://device-api-live.adyen.com/v1", "us": "https://device-api-live-us.adyen.com/v1", "apse": "https://device-api-live-apse.adyen.com/v1"} {
		live.Store = map[string]string{"merchantAccount": "M", "region": region}
		live.Terminal = "T"
		u, err := a.url(live)
		if err != nil || u != want+"/merchants/M/devices/T/sync" {
			t.Errorf("region %q: %s %v", region, u, err)
		}
	}
	live.Store = map[string]string{"merchantAccount": "M", "region": "mars"}
	if _, err := a.url(live); err == nil {
		t.Error("unknown region must fail")
	}
}

func TestTapAdapter(t *testing.T) {
	status := "INITIATED"
	f := &fakeProvider{}
	f.reply = func(c fakeCall) (int, interface{}) {
		if c.Header.Get("Authorization") != "Bearer sk_test_abc" {
			return 401, M{"errors": []M{{"code": "2107"}}}
		}
		switch {
		case c.Method == "POST" && c.Path == "/v2/intent":
			return 200, M{"id": "intent_1", "status": "INITIATED"}
		case c.Method == "GET" && c.Path == "/v2/intent/intent_1":
			return 200, M{"id": "intent_1", "status": status, "transaction": M{"authorization_id": "778899"},
				"reference": M{"payment": "52001122"}, "card": M{"brand": "KNET", "last_four": "4321"}}
		case c.Method == "PUT" && c.Path == "/v2/intent/intent_1/cancel":
			if status != "INITIATED" {
				return 400, M{"errors": []M{{"description": "not cancellable"}}}
			}
			return 200, M{"id": "intent_1", "status": "CANCELLED"}
		}
		return 404, M{}
	}
	s := f.server(t)
	cfg := terminalCfg{Env: "sandbox", BaseURL: s.URL + "/v2", Terminal: "term_1", HTTP: s.Client(),
		Store: map[string]string{"merchantId": "m_1", "terminalId": "term_1", "terminalDeviceId": "dev_1", "serialNumber": "SN1", "secretKey": "sk_test_abc"}}
	a := tapAdapter{}
	ctx := context.Background()
	if err := a.Check(ctx, cfg); err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, c := range []struct {
		env, key string
	}{{"sandbox", "pk_test_x"}, {"live", "sk_test_abc"}, {"sandbox", "sk_live_x"}, {"sandbox", "sk_test_wrong"}} {
		b := cfg
		b.Env = c.env
		b.Store = map[string]string{"secretKey": c.key}
		if err := a.Check(ctx, b); err == nil {
			t.Errorf("check %s %s must fail", c.env, c.key)
		}
	}
	req := terminalPayReq{PaymentID: "p1", Reference: "POS-5", Amount: 1.25, Currency: "KWD", Decimals: 3, WebhookURL: "https://api/x"}
	r, err := a.Start(ctx, cfg, req)
	if err != nil || r.Status != tpPending || r.ProviderRef != "intent_1" {
		t.Fatalf("start: %+v %v", r, err)
	}
	b := f.last().Body
	if b["idempotent"] != "p1" || b["scope"] != "CHARGE" || b["present_payments"] != "true" || get(b, "merchant.terminal.terminal_device.serial_number") != "SN1" ||
		get(b, "order.amount") != 1.25 || get(b, "order.currency") != "KWD" || get(b, "post.url") != "https://api/x" || get(b, "merchant.terminal.id") != "term_1" {
		t.Fatalf("intent wire: %v", b)
	}
	if r, _ := a.Status(ctx, cfg, req, "intent_1"); r.Status != tpPending {
		t.Fatalf("initiated → pending: %+v", r)
	}
	status = "CAPTURED"
	if r, _ := a.Status(ctx, cfg, req, "intent_1"); r.Status != tpApproved || r.AuthCode != "778899" || r.RRN != "52001122" || r.MaskedPan != "••••4321" || r.Scheme != "knet" {
		t.Fatalf("captured: %+v", r)
	}
	// cancel after it was captured reports the capture
	if r, err := a.Cancel(ctx, cfg, req, "intent_1"); err != nil || r.Status != tpApproved {
		t.Fatalf("cancel after capture: %+v %v", r, err)
	}
	status = "INITIATED"
	if r, err := a.Cancel(ctx, cfg, req, "intent_1"); err != nil || r.Status != tpCancelled {
		t.Fatalf("cancel: %+v %v", r, err)
	}
	for in, want := range map[string]string{"CAPTURED": tpApproved, "authorized": tpApproved, "DECLINED": tpDeclined, "CANCELLED": tpCancelled,
		"INITIATED": tpPending, "IN_PROGRESS": tpPending, "": tpPending, "TIMEDOUT": tpTimeout, "FAILED": tpDeclined} {
		if got := tapStatus(in); got != want {
			t.Errorf("tapStatus(%q)=%s want %s", in, got, want)
		}
	}
}

func TestTerminalDo_Errors(t *testing.T) {
	f := &fakeProvider{reply: func(c fakeCall) (int, interface{}) { return 500, M{"error": strings.Repeat("x", 400)} }}
	s := f.server(t)
	err := terminalDo(context.Background(), terminalCfg{HTTP: s.Client()}, "GET", s.URL, nil, nil, nil)
	he, ok := err.(*httpError)
	if !ok || he.Status != 500 || len(he.Error()) > 230 {
		t.Fatalf("http error: %v", err)
	}
	if err := terminalDo(context.Background(), terminalCfg{}, "GET", "http://127.0.0.1:1/x", nil, nil, nil); err == nil || err.Error() != "connection failed" {
		t.Fatalf("refused: %v", err)
	}
	for in, want := range map[string]string{"5243 **** **** 9871": "••••9871", "": "", "****": "", "12": "••••12"} {
		if got := lastDigits(in); got != want {
			t.Errorf("lastDigits(%q)=%q", in, got)
		}
	}
}
