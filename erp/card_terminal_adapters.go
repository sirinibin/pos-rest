package erp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Adapters of the providers that publish an API StartERP's server can call
// (research: /mnt/project-files/card-terminals/research.md):
//
//	nearpay       Saudi Arabia  Remote "Proxy with HTTP" (blocking purchase)
//	adyen         UAE           Cloud Terminal API, sync endpoint (blocking)
//	tap-smartpos  Kuwait        Intents API (asynchronous, polled + webhook)

var (
	_ = registerTerminalAdapter("nearpay", nearpayAdapter{})
	_ = registerTerminalAdapter("adyen", adyenAdapter{})
	_ = registerTerminalAdapter("tap-smartpos", tapAdapter{})
)

// httpError is a provider's non-2xx answer.
type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	b := strings.TrimSpace(e.Body)
	if len(b) > 200 {
		b = b[:200]
	}
	if b == "" {
		return "HTTP " + strconv.Itoa(e.Status)
	}
	return "HTTP " + strconv.Itoa(e.Status) + ": " + b
}

func terminalDo(ctx context.Context, cfg terminalCfg, method, u string, headers map[string]string, body interface{}, out interface{}) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := cfg.HTTP
	if client == nil {
		client = terminalHTTP
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("no answer in time")
		}
		return fmt.Errorf("connection failed")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("unreadable answer")
		}
	}
	return nil
}

func lastDigits(pan string) string {
	d := ""
	for _, c := range pan {
		if c >= '0' && c <= '9' {
			d += string(c)
		}
	}
	if len(d) > 4 {
		d = d[len(d)-4:]
	}
	if d == "" {
		return ""
	}
	return "••••" + d
}

// ---------------------------------------------------------------- NearPay

// NearPay Remote Operations ("Proxy with HTTP"):
//   - StartERP signs an RS256 JWT {data:{ops:"auth", merchant_uuid, terminal_id}}
//     with the client private key NearPay issued to StartERP (partner field),
//   - POST /pair/jwt {jwt} → {room_id, token}, cached per terminal,
//   - POST /purchase {amount (minor units), jobId, customer_reference_number}
//     answers when the card is done with transactionReceipts[],
//   - POST /cancel {} stops the pending job, POST /ping {timeout} checks it.
//
// The session token is sent as "Authorization: Bearer <token>" with the
// room id in "x-room-id" and in the body (NearPay's page does not show how;
// confirm with NearPay when the sandbox account is issued).
type nearpayAdapter struct{}

type nearpaySession struct {
	room, token string
	at          time.Time
}

var (
	nearpaySessions   = map[string]nearpaySession{}
	nearpaySessionsMu sync.Mutex
	nearpaySessionTTL = 30 * time.Minute
)

func (nearpayAdapter) base(cfg terminalCfg) (string, error) {
	b := cfg.BaseURL
	if cfg.Env == "live" {
		if v := strings.TrimSpace(cfg.Partner["liveBaseUrl"]); v != "" {
			b = v
		}
	}
	if b == "" {
		return "", fmt.Errorf("NearPay's address is not set")
	}
	return strings.TrimRight(b, "/"), nil
}

// signRS256 builds a compact JWT signed with an RSA private key in PEM form.
func signRS256(pemText string, claims map[string]interface{}) (string, error) {
	blk, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if blk == nil {
		return "", fmt.Errorf("the NearPay private key is not a PEM file")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		key = k
	} else if k8, err8 := x509.ParsePKCS8PrivateKey(blk.Bytes); err8 == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("the NearPay private key is not an RSA key")
		}
		key = rk
	} else {
		return "", fmt.Errorf("the NearPay private key cannot be read")
	}
	enc := base64.RawURLEncoding
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	c, _ := json.Marshal(claims)
	signing := enc.EncodeToString(h) + "." + enc.EncodeToString(c)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc.EncodeToString(sig), nil
}

func (a nearpayAdapter) session(ctx context.Context, cfg terminalCfg, fresh bool) (nearpaySession, error) {
	key := cfg.Env + "|" + cfg.get("merchantUuid") + "|" + cfg.Terminal
	nearpaySessionsMu.Lock()
	s, ok := nearpaySessions[key]
	nearpaySessionsMu.Unlock()
	if ok && !fresh && nowFn().Sub(s.at) < nearpaySessionTTL {
		return s, nil
	}
	base, err := a.base(cfg)
	if err != nil {
		return s, err
	}
	now := nowFn()
	tok, err := signRS256(cfg.Partner["clientPrivateKey"], map[string]interface{}{
		"data": map[string]string{"ops": "auth", "merchant_uuid": cfg.get("merchantUuid"), "terminal_id": cfg.Terminal},
		"iat":  now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		return s, err
	}
	var out struct {
		RoomID string `json:"room_id"`
		Token  string `json:"token"`
	}
	if err := terminalDo(ctx, cfg, "POST", base+"/pair/jwt", nil, map[string]string{"jwt": tok}, &out); err != nil {
		return s, err
	}
	if out.Token == "" {
		return s, fmt.Errorf("NearPay did not open a session")
	}
	s = nearpaySession{room: out.RoomID, token: out.Token, at: nowFn()}
	nearpaySessionsMu.Lock()
	nearpaySessions[key] = s
	nearpaySessionsMu.Unlock()
	return s, nil
}

// call runs one operation, opening a fresh session once when the cached one
// is refused.
func (a nearpayAdapter) call(ctx context.Context, cfg terminalCfg, path string, body map[string]interface{}, out interface{}) error {
	base, err := a.base(cfg)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		s, err := a.session(ctx, cfg, attempt > 0)
		if err != nil {
			return err
		}
		b := map[string]interface{}{"room_id": s.room}
		for k, v := range body {
			b[k] = v
		}
		err = terminalDo(ctx, cfg, "POST", base+path, map[string]string{"Authorization": "Bearer " + s.token, "x-room-id": s.room}, b, out)
		var he *httpError
		if errors.As(err, &he) && (he.Status == 401 || he.Status == 403) && attempt == 0 {
			continue
		}
		return err
	}
	return nil
}

type nearpayReceipt struct {
	TransactionUUID string `json:"transaction_uuid"`
	IsApproved      bool   `json:"is_approved"`
	IsReversed      bool   `json:"is_reversed"`
	ApprovalCode    string `json:"approval_code"`
	StatusMessage   string `json:"status_message"`
	ActionCode      string `json:"action_code"`
	Pan             string `json:"pan"`
	CardScheme      string `json:"card_scheme"`
	RRN             string `json:"retrieval_reference_number"`
}

func nearpayResult(receipts []nearpayReceipt, msg string) terminalResult {
	if len(receipts) == 0 {
		if msg == "" {
			msg = "The card machine did not return a receipt."
		}
		return terminalResult{Status: tpFailed, Message: msg}
	}
	r := receipts[0]
	out := terminalResult{ProviderRef: r.TransactionUUID, AuthCode: r.ApprovalCode, RRN: r.RRN,
		MaskedPan: lastDigits(r.Pan), Scheme: strings.ToLower(r.CardScheme), Message: r.StatusMessage}
	switch {
	case r.IsApproved && !r.IsReversed:
		out.Status = tpApproved
	case r.IsReversed:
		out.Status = tpCancelled
	default:
		out.Status = tpDeclined
	}
	return out
}

func (a nearpayAdapter) Check(ctx context.Context, cfg terminalCfg) error {
	if strings.TrimSpace(cfg.Partner["clientPrivateKey"]) == "" {
		return fmt.Errorf("StartERP's NearPay key is not set yet")
	}
	return a.call(ctx, cfg, "/ping", map[string]interface{}{"timeout": 5000}, nil)
}

func (a nearpayAdapter) Run(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	var out struct {
		Status   int              `json:"status"`
		Message  string           `json:"message"`
		Receipts []nearpayReceipt `json:"transactionReceipts"`
	}
	ref := req.Reference
	if ref == "" {
		ref = req.PaymentID
	}
	err := a.call(ctx, cfg, "/purchase", map[string]interface{}{
		"amount": req.minorUnits(), "jobId": req.PaymentID, "customer_reference_number": ref,
	}, &out)
	var he *httpError
	if errors.As(err, &he) && he.Status == 408 {
		return terminalResult{Status: tpTimeout, Message: "No card was presented in time."}, nil
	}
	if err != nil {
		return terminalResult{}, err
	}
	return nearpayResult(out.Receipts, out.Message), nil
}

func (a nearpayAdapter) Start(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	return a.Run(ctx, cfg, req)
}

func (nearpayAdapter) Status(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	return terminalResult{Status: tpPending, ProviderRef: ref}, nil
}

func (a nearpayAdapter) Cancel(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	if err := a.call(ctx, cfg, "/cancel", map[string]interface{}{}, nil); err != nil {
		return terminalResult{}, err
	}
	return terminalResult{Status: tpCancelled, Message: "Cancelled on the card machine."}, nil
}

// ---------------------------------------------------------------- Adyen

// Adyen Cloud Terminal API (sync):
//
//	POST {base}/merchants/{merchantAccount}/devices/{POIID}/sync   header x-API-key
//	{"SaleToPOIRequest": {"MessageHeader": {...}, "PaymentRequest": {...}}}
//
// A payment answers when the shopper is done (up to 150 s). Abort cancels it.
type adyenAdapter struct{}

const adyenSaleID = "StartERP"

func (adyenAdapter) url(cfg terminalCfg) (string, error) {
	base := cfg.BaseURL
	if cfg.Env == "live" {
		if r := strings.ToLower(strings.TrimSpace(cfg.get("region"))); r != "" && r != "eu" {
			switch r {
			case "us", "au", "apse", "nea":
				base = "https://device-api-live-" + r + ".adyen.com/v1"
			default:
				return "", fmt.Errorf("unknown Adyen region %q (use us, au, apse or nea)", r)
			}
		}
	}
	ma, poi := cfg.get("merchantAccount"), cfg.Terminal
	if base == "" || ma == "" || poi == "" {
		return "", fmt.Errorf("the merchant account and terminal POIID are required")
	}
	return strings.TrimRight(base, "/") + "/merchants/" + url.PathEscape(ma) + "/devices/" + url.PathEscape(poi) + "/sync", nil
}

func adyenServiceID(paymentID string) string {
	// ServiceID: at most 10 characters, unique per terminal within 48 hours
	if len(paymentID) > 10 {
		return paymentID[len(paymentID)-10:]
	}
	return paymentID
}

func adyenHeader(category, serviceID, poi string) map[string]interface{} {
	return map[string]interface{}{
		"ProtocolVersion": "3.0", "MessageClass": "Service", "MessageCategory": category, "MessageType": "Request",
		"SaleID": adyenSaleID, "ServiceID": serviceID, "POIID": poi,
	}
}

func (a adyenAdapter) send(ctx context.Context, cfg terminalCfg, body interface{}, out interface{}) error {
	u, err := a.url(cfg)
	if err != nil {
		return err
	}
	return terminalDo(ctx, cfg, "POST", u, map[string]string{"x-API-key": cfg.get("apiKey")}, body, out)
}

func (a adyenAdapter) Check(ctx context.Context, cfg terminalCfg) error {
	var out struct {
		SaleToPOIResponse struct {
			DiagnosisResponse struct {
				Response struct {
					Result string `json:"Result"`
				} `json:"Response"`
			} `json:"DiagnosisResponse"`
		} `json:"SaleToPOIResponse"`
	}
	sid := "chk" + strconv.FormatInt(nowFn().Unix()%10000000, 10)
	err := a.send(ctx, cfg, map[string]interface{}{"SaleToPOIRequest": map[string]interface{}{
		"MessageHeader":    adyenHeader("Diagnosis", sid, cfg.Terminal),
		"DiagnosisRequest": map[string]interface{}{"HostDiagnosisFlag": false},
	}}, &out)
	if err != nil {
		return err
	}
	if r := out.SaleToPOIResponse.DiagnosisResponse.Response.Result; r != "" && r != "Success" {
		return fmt.Errorf("the terminal answered %s", r)
	}
	return nil
}

type adyenPaymentResponse struct {
	SaleToPOIResponse struct {
		PaymentResponse struct {
			Response struct {
				Result             string `json:"Result"`
				ErrorCondition     string `json:"ErrorCondition"`
				AdditionalResponse string `json:"AdditionalResponse"`
			} `json:"Response"`
			POIData struct {
				POITransactionID struct {
					TransactionID string `json:"TransactionID"`
				} `json:"POITransactionID"`
			} `json:"POIData"`
			PaymentResult struct {
				PaymentAcquirerData struct {
					ApprovalCode          string `json:"ApprovalCode"`
					AcquirerTransactionID struct {
						TransactionID string `json:"TransactionID"`
					} `json:"AcquirerTransactionID"`
				} `json:"PaymentAcquirerData"`
				PaymentInstrumentData struct {
					CardData struct {
						MaskedPan    string `json:"MaskedPan"`
						PaymentBrand string `json:"PaymentBrand"`
					} `json:"CardData"`
				} `json:"PaymentInstrumentData"`
			} `json:"PaymentResult"`
		} `json:"PaymentResponse"`
	} `json:"SaleToPOIResponse"`
}

func adyenResult(o adyenPaymentResponse) terminalResult {
	p := o.SaleToPOIResponse.PaymentResponse
	res := terminalResult{
		ProviderRef: p.POIData.POITransactionID.TransactionID,
		AuthCode:    p.PaymentResult.PaymentAcquirerData.ApprovalCode,
		RRN:         p.PaymentResult.PaymentAcquirerData.AcquirerTransactionID.TransactionID,
		MaskedPan:   lastDigits(p.PaymentResult.PaymentInstrumentData.CardData.MaskedPan),
		Scheme:      strings.ToLower(p.PaymentResult.PaymentInstrumentData.CardData.PaymentBrand),
	}
	if q, err := url.ParseQuery(p.Response.AdditionalResponse); err == nil {
		if v := q.Get("refusalReason"); v != "" {
			res.Message = v
		}
	}
	switch p.Response.Result {
	case "Success":
		res.Status = tpApproved
	case "Partial":
		// a partial approval is not a full payment: report it so the cashier voids it
		res.Status = tpDeclined
		res.Message = "Only part of the amount was approved. Void it on the machine."
	default:
		switch p.Response.ErrorCondition {
		case "Aborted", "Cancel":
			res.Status = tpCancelled
		case "Refusal", "NotAllowed", "WrongPIN":
			res.Status = tpDeclined
		case "Busy":
			res.Status = tpFailed
			res.Message = "The terminal is busy with another payment."
		case "":
			res.Status = tpFailed
		default:
			res.Status = tpFailed
		}
		if res.Message == "" {
			res.Message = strings.TrimSpace(p.Response.ErrorCondition)
		}
	}
	return res
}

func (a adyenAdapter) Run(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	tx := req.Reference
	if tx == "" {
		tx = req.PaymentID
	}
	var out adyenPaymentResponse
	err := a.send(ctx, cfg, map[string]interface{}{"SaleToPOIRequest": map[string]interface{}{
		"MessageHeader": adyenHeader("Payment", adyenServiceID(req.PaymentID), cfg.Terminal),
		"PaymentRequest": map[string]interface{}{
			"SaleData": map[string]interface{}{"SaleTransactionID": map[string]interface{}{
				"TransactionID": tx, "TimeStamp": nowFn().UTC().Format("2006-01-02T15:04:05.000Z"),
			}},
			"PaymentTransaction": map[string]interface{}{"AmountsReq": map[string]interface{}{
				"Currency": req.Currency, "RequestedAmount": req.Amount,
			}},
		},
	}}, &out)
	if err != nil {
		return terminalResult{}, err
	}
	return adyenResult(out), nil
}

func (a adyenAdapter) Start(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	return a.Run(ctx, cfg, req)
}

func (adyenAdapter) Status(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	return terminalResult{Status: tpPending, ProviderRef: ref}, nil
}

func (a adyenAdapter) Cancel(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	err := a.send(ctx, cfg, map[string]interface{}{"SaleToPOIRequest": map[string]interface{}{
		"MessageHeader": adyenHeader("Abort", "ab"+adyenServiceID(req.PaymentID)[2:], cfg.Terminal),
		"AbortRequest": map[string]interface{}{
			"AbortReason": "MerchantAbort",
			"MessageReference": map[string]interface{}{
				"MessageCategory": "Payment", "SaleID": adyenSaleID, "ServiceID": adyenServiceID(req.PaymentID), "POIID": cfg.Terminal,
			},
		},
	}}, nil)
	if err != nil {
		return terminalResult{}, err
	}
	return terminalResult{Status: tpCancelled, Message: "Cancelled on the card machine."}, nil
}

// ---------------------------------------------------------------- Tap SmartPOS

// Tap SmartPOS (Kuwait) Intents API:
//
//	POST {base}/intent   Authorization: Bearer <secret key>
//	  {idempotent, scope:"CHARGE", present_payments:"true",
//	   merchant:{id, terminal:{id, terminal_device:{id, serial_number}}},
//	   order:{amount, currency, reference}, post:{url}}  → {id, status:"INITIATED"}
//	GET  {base}/intent/{id}          → current status
//	PUT  {base}/intent/{id}/cancel   (only while INITIATED)
//
// Tap calls post.url when the intent finishes; we only use it as a nudge to
// read the intent again (the body is never trusted).
type tapAdapter struct{}

func (tapAdapter) headers(cfg terminalCfg) map[string]string {
	return map[string]string{"Authorization": "Bearer " + cfg.get("secretKey")}
}

func (tapAdapter) base(cfg terminalCfg) string { return strings.TrimRight(cfg.BaseURL, "/") }

type tapIntent struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"response"`
	Reference struct {
		Payment string `json:"payment"`
		Track   string `json:"track"`
	} `json:"reference"`
	Transaction struct {
		AuthorizationID string `json:"authorization_id"`
	} `json:"transaction"`
	Card struct {
		Brand     string `json:"brand"`
		Scheme    string `json:"scheme"`
		LastFour  string `json:"last_four"`
		LastFour2 string `json:"last4"`
	} `json:"card"`
	Charge struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"charge"`
}

// tapStatus maps Tap's intent / charge status to ours (unknown = still pending).
func tapStatus(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CAPTURED", "AUTHORIZED", "SUCCESS", "SUCCEEDED", "COMPLETED", "PAID", "APPROVED":
		return tpApproved
	case "DECLINED", "FAILED", "RESTRICTED", "VOID", "ABANDONED", "UNKNOWN", "REJECTED", "INVALID":
		return tpDeclined
	case "CANCELLED", "CANCELED":
		return tpCancelled
	case "TIMEDOUT", "TIMED_OUT", "EXPIRED":
		return tpTimeout
	}
	return tpPending
}

func tapResult(in tapIntent) terminalResult {
	st := tapStatus(in.Status)
	if st == tpPending && in.Charge.Status != "" {
		st = tapStatus(in.Charge.Status)
	}
	last := in.Card.LastFour
	if last == "" {
		last = in.Card.LastFour2
	}
	r := terminalResult{Status: st, ProviderRef: in.ID, AuthCode: in.Transaction.AuthorizationID,
		RRN: in.Reference.Payment, Scheme: strings.ToLower(firstNonEmpty(in.Card.Scheme, in.Card.Brand)), Message: in.Response.Message}
	if last != "" {
		r.MaskedPan = "••••" + last
	}
	return r
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func (a tapAdapter) Check(ctx context.Context, cfg terminalCfg) error {
	key := cfg.get("secretKey")
	if !strings.HasPrefix(key, "sk_") {
		return fmt.Errorf("the secret key should start with sk_live_ or sk_test_")
	}
	if cfg.Env == "live" && strings.HasPrefix(key, "sk_test_") {
		return fmt.Errorf("a test key (sk_test_) cannot take live payments; use test mode or the sk_live_ key")
	}
	if cfg.Env == "sandbox" && strings.HasPrefix(key, "sk_live_") {
		return fmt.Errorf("test mode needs the sk_test_ key")
	}
	// reading an intent that does not exist proves the key is accepted (404 ≠ 401)
	err := terminalDo(ctx, cfg, "GET", a.base(cfg)+"/intent/starterp_connection_check", a.headers(cfg), nil, nil)
	var he *httpError
	if errors.As(err, &he) {
		if he.Status == 401 || he.Status == 403 {
			return fmt.Errorf("Tap refused the secret key")
		}
		return nil
	}
	return err
}

func (a tapAdapter) Start(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	ref := req.Reference
	if ref == "" {
		ref = req.PaymentID
	}
	body := map[string]interface{}{
		"idempotent": req.PaymentID, "scope": "CHARGE", "present_payments": "true",
		"merchant": map[string]interface{}{"id": cfg.get("merchantId"), "terminal": map[string]interface{}{
			"id": cfg.Terminal, "terminal_device": map[string]interface{}{"id": cfg.get("terminalDeviceId"), "serial_number": cfg.get("serialNumber")},
		}},
		"order": map[string]interface{}{"amount": req.Amount, "currency": req.Currency, "reference": map[string]string{"order": ref}},
	}
	if req.WebhookURL != "" {
		body["post"] = map[string]string{"url": req.WebhookURL}
	}
	var out tapIntent
	if err := terminalDo(ctx, cfg, "POST", a.base(cfg)+"/intent", a.headers(cfg), body, &out); err != nil {
		return terminalResult{}, err
	}
	if out.ID == "" {
		return terminalResult{}, fmt.Errorf("Tap did not create the payment")
	}
	r := tapResult(out)
	if r.Message == "" && r.Status == tpPending {
		r.Message = "Waiting for the card on the machine."
	}
	return r, nil
}

func (a tapAdapter) Status(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	var out tapIntent
	if err := terminalDo(ctx, cfg, "GET", a.base(cfg)+"/intent/"+url.PathEscape(ref), a.headers(cfg), nil, &out); err != nil {
		return terminalResult{}, err
	}
	if out.ID == "" {
		out.ID = ref
	}
	return tapResult(out), nil
}

func (a tapAdapter) Cancel(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	var out tapIntent
	err := terminalDo(ctx, cfg, "PUT", a.base(cfg)+"/intent/"+url.PathEscape(ref)+"/cancel", a.headers(cfg), map[string]interface{}{}, &out)
	if err != nil {
		// no longer INITIATED: tell the caller what it became instead
		if st, serr := a.Status(ctx, cfg, req, ref); serr == nil && st.Status != tpPending {
			return st, nil
		}
		return terminalResult{}, err
	}
	if st := tapResult(out); st.Status != tpPending {
		return st, nil
	}
	return terminalResult{Status: tpCancelled, ProviderRef: ref, Message: "Cancelled on the card machine."}, nil
}
