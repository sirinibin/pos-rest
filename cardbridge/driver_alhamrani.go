package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Alhamrani Universal (AU) mada terminals (Saudi Arabia) through AU's
// "AlhamraniServicev2" Windows service: a SignalR 2 hub on
// http://localhost:9000/signalr (hub MyHub). The till calls
// Send("transaction", json) and the service broadcasts addMessage("response",
// json); answers are matched on ecr_receipt_no. response_code 000, 001, 003,
// 007, 060, 086, 087, 089, 300, 400 and 800 are approvals.
type alhamraniDriver struct{}

var _ = registerDriver(alhamraniDriver{})

func (alhamraniDriver) ID() string    { return "alhamrani-signalr" }
func (alhamraniDriver) Title() string { return "Alhamrani (AU) mada terminal via AlhamraniService" }

var auApproved = map[string]bool{"000": true, "001": true, "003": true, "007": true, "060": true, "086": true, "087": true, "089": true, "300": true, "400": true, "800": true}

func auBase(j Job) string {
	if v := j.cfg("serviceUrl"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:9000/signalr"
}

const auConnData = `[{"name":"myhub"}]`

type signalR struct {
	ws  *wsConn
	inv int
}

// auConnect: negotiate, connect over WebSockets, start.
func auConnect(ctx context.Context, base string) (*signalR, error) {
	hc := &http.Client{Timeout: 8 * time.Second}
	q := "clientProtocol=1.5&connectionData=" + url.QueryEscape(auConnData)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/negotiate?"+q, nil)
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach AlhamraniService at %s: %v. Check that AU's service is running on this computer", base, err)
	}
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	res.Body.Close()
	var n struct {
		ConnectionToken string
	}
	if json.Unmarshal(b, &n) != nil || n.ConnectionToken == "" {
		return nil, fmt.Errorf("AlhamraniService answered an unexpected negotiate reply (HTTP %d)", res.StatusCode)
	}
	q += "&transport=webSockets&connectionToken=" + url.QueryEscape(n.ConnectionToken)
	wsBase := "ws" + strings.TrimPrefix(base, "http")
	ws, err := wsDial(ctx, wsBase+"/connect?"+q)
	if err != nil {
		return nil, fmt.Errorf("AlhamraniService refused the connection: %v", err)
	}
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, base+"/start?"+q, nil)
	if res, err := hc.Do(req); err == nil {
		res.Body.Close()
	}
	return &signalR{ws: ws}, nil
}

func (s *signalR) send(keyword string, body interface{}) error {
	j, _ := json.Marshal(body)
	s.inv++
	m, _ := json.Marshal(map[string]interface{}{"H": "MyHub", "M": "Send", "A": []string{keyword, string(j)}, "I": fmt.Sprint(s.inv)})
	return s.ws.WriteText(string(m))
}

// messages returns the addMessage payloads in one SignalR frame.
func auMessages(b []byte) []map[string]string {
	var f struct {
		M []struct {
			H string
			M string
			A []interface{}
		}
	}
	if json.Unmarshal(b, &f) != nil {
		return nil
	}
	out := []map[string]string{}
	for _, m := range f.M {
		if !strings.EqualFold(m.M, "addMessage") {
			continue
		}
		for _, a := range m.A {
			s, ok := a.(string)
			if !ok || !strings.HasPrefix(strings.TrimSpace(s), "{") {
				continue
			}
			var raw map[string]interface{}
			if json.Unmarshal([]byte(s), &raw) == nil {
				x := map[string]string{}
				for k, v := range raw {
					if v != nil {
						x[k] = strings.TrimSpace(fmt.Sprint(v))
					}
				}
				out = append(out, x)
			}
		}
	}
	return out
}

func auBody(j Job, receipt, amount string) map[string]string {
	ecr := j.cfg("ecrNo")
	if ecr == "" {
		ecr = "1"
	}
	return map[string]string{"msg_id": "PUR", "ecr_no": ecr, "ecr_receipt_no": receipt, "amount": amount,
		"field1": "", "field2": "", "field3": "", "field4": "", "field5": "",
		"port_no_or_ip_adddress": j.cfg("machinePort"), "bill_no": receipt}
}

// auAmount: 12 digits; halalas unless the store set "riyals".
func auAmount(j Job) string {
	if strings.EqualFold(j.cfg("amountIn"), "riyals") {
		return fmt.Sprintf("%012d", int64(j.Amount))
	}
	return fmt.Sprintf("%012d", j.Minor)
}

func auReceipt(j Job) string {
	return fmt.Sprintf("%010d", time.Now().UnixNano()/1e6%10000000000)
}

func (d alhamraniDriver) Check(ctx context.Context, j Job) error {
	if j.cfg("machinePort") == "" {
		return errors.New("enter the machine's COM port or IP address in StartERP (Settings → Card machines → Edit)")
	}
	s, err := auConnect(ctx, auBase(j))
	if err != nil {
		return err
	}
	defer s.ws.Close()
	b := auBody(j, auReceipt(j), "")
	if err := s.send("check2", b); err != nil {
		return err
	}
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		msg, err := s.ws.Read(end)
		if err != nil {
			return fmt.Errorf("AlhamraniService did not answer the check: %v", err)
		}
		for _, m := range auMessages(msg) {
			if v, ok := m["is_connected"]; ok {
				if strings.EqualFold(v, "true") || v == "1" {
					return nil
				}
				return errors.New("AlhamraniService cannot reach the card machine; check the cable or IP address")
			}
		}
	}
	return errors.New("AlhamraniService did not answer the check")
}

func (d alhamraniDriver) Pay(ctx context.Context, j Job, progress func(string)) Result {
	if j.cfg("machinePort") == "" {
		return failed("Enter the machine's COM port or IP address in StartERP (Settings → Card machines → Edit).")
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	s, err := auConnect(cctx, auBase(j))
	cancel()
	if err != nil {
		return failed("%s", err.Error())
	}
	defer s.ws.Close()
	receipt := auReceipt(j)
	if err := s.send("transaction", auBody(j, receipt, auAmount(j))); err != nil {
		return failed("Could not send the amount to AlhamraniService: %v", err)
	}
	progress("Waiting for the card on the machine…")
	stop := func() {
		_ = s.send("cancel", map[string]string{"port_no_or_ip_adddress": j.cfg("machinePort")})
	}
	return readUntilAnswer(ctx, s.ws, stop, func(b []byte) (Result, bool) {
		for _, m := range auMessages(b) {
			if m["ecr_receipt_no"] != "" && m["ecr_receipt_no"] != receipt {
				continue
			}
			code := strings.ToUpper(m["response_code"])
			if code == "" {
				continue
			}
			r := Result{AuthCode: m["auth_code"], RRN: m["rrn"], MaskedPan: maskPan(m["pan"]), Scheme: schemeOf(m["card_type"]), ProviderRef: receipt}
			switch {
			case auApproved[code]:
				r.Status, r.Message = "approved", "Approved on the card machine."
			case strings.HasPrefix(code, "1") && len(code) == 3:
				r.Status, r.Message = "declined", "Declined on the card machine ("+code+")."
			case code == "CAN" || code == "UC":
				r.Status, r.Message = "cancelled", "Cancelled on the card machine."
			case code == "TO":
				r.Status, r.Message = "timeout", "No card was presented in time."
			default:
				r.Status, r.Message = "failed", "The card machine answered "+code+". Check its last receipt before taking the payment again."
			}
			return r, true
		}
		return Result{}, false
	})
}
