package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func job(driver string, amount float64, decimals int, cfg map[string]string) Job {
	minor := int64(amount*pow10(decimals) + 0.5)
	return Job{ID: "j1", Op: "pay", Driver: driver, PaymentID: "64f0c0ffee0000000000abcd", Reference: "POS-ABC123", Amount: amount,
		Minor: minor, Decimals: decimals, Currency: "SAR", TerminalName: "Counter 1", TimeoutSeconds: 180, Config: cfg}
}

func pow10(n int) float64 {
	f := 1.0
	for i := 0; i < n; i++ {
		f *= 10
	}
	return f
}

// ---------------------------------------------------------------- simulator

func TestSimulatorDriver(t *testing.T) {
	d := simulatorDriver{delay: 10 * time.Millisecond}
	cases := []struct {
		amt  float64
		want string
	}{{1, "approved"}, {12.34, "approved"}, {10.05, "declined"}, {3.07, "failed"}}
	for _, c := range cases {
		r := d.Pay(context.Background(), job("simulator", c.amt, 2, nil), noProgress)
		if r.Status != c.want {
			t.Errorf("%.2f: got %s want %s", c.amt, r.Status, c.want)
		}
	}
	r := d.Pay(context.Background(), job("simulator", 1, 2, nil), noProgress)
	if r.AuthCode == "" || r.MaskedPan != "••••4242" || !strings.Contains(r.Message, "TEST") {
		t.Errorf("approval details missing: %+v", r)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if r := d.Pay(ctx, job("simulator", 2.06, 2, nil), noProgress); r.Status != "timeout" {
		t.Errorf(".06 should time out, got %s", r.Status)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if r := (simulatorDriver{delay: time.Second}).Pay(ctx2, job("simulator", 1, 2, nil), noProgress); r.Status != "cancelled" {
		t.Errorf("cancelled payment: got %s", r.Status)
	}
}

func TestDriversRegistered(t *testing.T) {
	want := []string{"alhamrani-signalr", "geidea-webecr", "knet-esocket", "neoleap-ws", "simulator"}
	if got := driverIDs(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("drivers = %v", got)
	}
	for _, id := range want {
		if driverByID(id).Title() == "" {
			t.Errorf("%s has no title", id)
		}
	}
}

// ---------------------------------------------------------------- neoleap

func neoleapMock(t *testing.T, status string, answer func(sale map[string]string) string) (*httptest.Server, *atomic.Int32) {
	cancels := &atomic.Int32{}
	srv := wsMock(t, func(s *wsServerConn, msg string) {
		var m map[string]string
		_ = json.Unmarshal([]byte(msg), &m)
		switch m["Command"] {
		case "CHECK_STATUS":
			s.send(`{"EventName":"TERMINAL_STATUS","TerminalStatus":"` + status + `"}`)
		case "SALE":
			if a := answer(m); a != "" {
				s.send(a)
			}
		case "CANCEL":
			cancels.Add(1)
			s.send(`{"API_Status":"0","EventName":"TERMINAL_RESPONSE","JsonResult":{"StatusCode":"11","TransactionResponseEnglish":"CANCELLED"}}`)
		}
	})
	return srv, cancels
}

func TestNeoleap_Approved_JSONObject(t *testing.T) {
	var got map[string]string
	srv, _ := neoleapMock(t, "READY", func(m map[string]string) string {
		got = m
		return `{"API_Status":"0","EventName":"TERMINAL_RESPONSE","JsonResult":{"StatusCode":"00","TransactionAuthCode":"A1B2C3","RetrievalReferenceNumber":"123456789012","PrimaryAccountNumber":"588845******1234","CardScheme":"P1","ECRReferenceNumber":"x"}}`
	})
	r := neoleapDriver{}.Pay(context.Background(), job("neoleap-ws", 150, 2, map[string]string{"host": hostPort(srv)}), noProgress)
	if r.Status != "approved" || r.AuthCode != "A1B2C3" || r.RRN != "123456789012" || r.MaskedPan != "••••1234" || r.Scheme != "mada" {
		t.Fatalf("result %+v", r)
	}
	if got["Amount"] != "150.00" || got["AdditionalData"] != "POSABC123" {
		t.Errorf("request %v", got)
	}
}

func TestNeoleap_JSONStringAndDeclined(t *testing.T) {
	srv, _ := neoleapMock(t, "READY", func(m map[string]string) string {
		inner, _ := json.Marshal(map[string]string{"StatusCode": "01", "TransactionResponseEnglish": "INSUFFICIENT FUNDS"})
		b, _ := json.Marshal(map[string]interface{}{"EventName": "TERMINAL_RESPONSE", "JsonResult": string(inner)})
		return string(b)
	})
	r := neoleapDriver{}.Pay(context.Background(), job("neoleap-ws", 9.5, 2, map[string]string{"host": hostPort(srv)}), noProgress)
	if r.Status != "declined" || !strings.Contains(r.Message, "INSUFFICIENT FUNDS") {
		t.Fatalf("result %+v", r)
	}
}

func TestNeoleap_XMLVariant(t *testing.T) {
	srv, _ := neoleapMock(t, "READY", func(m map[string]string) string {
		return `{"EventName":"TERMINAL_RESPONSE"}<madaTransactionResult><Result English="APPROVED" Arabic="x"/><ApprovalCode>778899</ApprovalCode><RRN>000011112222</RRN><PAN>4847 83** **** 9871</PAN><TerminalStatusCode>00</TerminalStatusCode></madaTransactionResult>`
	})
	r := neoleapDriver{}.Pay(context.Background(), job("neoleap-ws", 5, 2, map[string]string{"host": hostPort(srv)}), noProgress)
	if r.Status != "approved" || r.AuthCode != "778899" || r.RRN != "000011112222" || r.MaskedPan != "••••9871" {
		t.Fatalf("result %+v", r)
	}
}

func TestNeoleap_BusyAndCheck(t *testing.T) {
	srv, _ := neoleapMock(t, "BUSY", func(map[string]string) string { return "" })
	cfg := map[string]string{"host": hostPort(srv)}
	if r := (neoleapDriver{}).Pay(context.Background(), job("neoleap-ws", 5, 2, cfg), noProgress); r.Status != "failed" || !strings.Contains(r.Message, "busy") {
		t.Errorf("busy: %+v", r)
	}
	if err := (neoleapDriver{}).Check(context.Background(), job("neoleap-ws", 0, 2, cfg)); err == nil || !strings.Contains(err.Error(), "BUSY") {
		t.Errorf("check busy: %v", err)
	}
	ready, _ := neoleapMock(t, "READY", func(map[string]string) string { return "" })
	if err := (neoleapDriver{}).Check(context.Background(), job("neoleap-ws", 0, 2, map[string]string{"host": hostPort(ready)})); err != nil {
		t.Errorf("check ready: %v", err)
	}
	if err := (neoleapDriver{}).Check(context.Background(), job("neoleap-ws", 0, 2, nil)); err == nil || !strings.Contains(err.Error(), "IP address") {
		t.Errorf("no host: %v", err)
	}
}

func TestNeoleap_CancelWhileWaiting(t *testing.T) {
	srv, cancels := neoleapMock(t, "READY", func(map[string]string) string { return "" })
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	r := neoleapDriver{}.Pay(ctx, job("neoleap-ws", 5, 2, map[string]string{"host": hostPort(srv)}), noProgress)
	if r.Status != "cancelled" || cancels.Load() != 1 {
		t.Fatalf("result %+v cancels %d", r, cancels.Load())
	}
}

func TestNeoleapAddr(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.9": "ws://192.168.1.9:7000", "192.168.1.9:9998": "ws://192.168.1.9:9998", "ws://10.0.0.2:7000": "ws://10.0.0.2:7000",
	} {
		got, err := neoleapAddr(Job{Config: map[string]string{"host": in}})
		if err != nil || got != want {
			t.Errorf("%s → %s %v", in, got, err)
		}
	}
	got, _ := neoleapAddr(Job{Config: map[string]string{"host": "10.0.0.5", "port": "9998"}})
	if got != "ws://10.0.0.5:9998" {
		t.Errorf("port setting: %s", got)
	}
}

// ---------------------------------------------------------------- geidea

func TestGeidea_ConnectAndPurchase(t *testing.T) {
	var connect, purchase map[string]string
	srv := wsMock(t, func(s *wsServerConn, msg string) {
		var m map[string]string
		_ = json.Unmarshal([]byte(msg), &m)
		switch m["Event"] + "/" + m["Operation"] {
		case "CONNECTION/CONNECT":
			connect = m
			s.send(`{"Event":"OnConnect"}`)
		case "TRANSACTION/PURCHASE":
			purchase = m
			s.send(`{"Event":"OnTerminalStatus","TerminalStatus":"BUSY"}`)
			inner, _ := json.Marshal(map[string]string{"TransactionResponseEnglish": "APPROVED", "TransactionAuthCode": "G12345",
				"RetrievalReferenceNumber": "999988887777", "PrimaryAccountNumber": "440000******0001", "CardNameEnglish": "VISA"})
			b, _ := json.Marshal(map[string]string{"Event": "OnDataReceive", "JsonResult": string(inner)})
			s.send(string(b))
		}
	})
	cfg := map[string]string{"webEcrUrl": wsURL(srv, "/messages"), "machine": "com3"}
	var progress []string
	r := geideaDriver{}.Pay(context.Background(), job("geidea-webecr", 12.5, 2, cfg), func(m string) { progress = append(progress, m) })
	if r.Status != "approved" || r.AuthCode != "G12345" || r.Scheme != "visa" || r.MaskedPan != "••••0001" {
		t.Fatalf("result %+v", r)
	}
	if connect["ConnectionMode"] != "COM" || connect["ComName"] != "COM3" || connect["BaudRate"] != "38400" {
		t.Errorf("connect %v", connect)
	}
	if purchase["Amount"] != "12.50" || purchase["AppId"] != "11" || purchase["PrintSettings"] != "1" {
		t.Errorf("purchase %v", purchase)
	}
	if len(progress) < 2 {
		t.Errorf("progress %v", progress)
	}
}

func TestGeidea_TCPAndCancelledOnMachine(t *testing.T) {
	var connect map[string]string
	srv := wsMock(t, func(s *wsServerConn, msg string) {
		var m map[string]string
		_ = json.Unmarshal([]byte(msg), &m)
		if m["Operation"] == "CONNECT" {
			connect = m
			s.send(`{"Event":"OnConnect"}`)
		}
		if m["Operation"] == "PURCHASE" {
			s.send(`{"Event":"OnTerminalAction","TerminalAction":"USER_CANCELLED_AND_TIMEOUT"}`)
		}
	})
	cfg := map[string]string{"webEcrUrl": wsURL(srv, "/messages"), "machine": "192.168.1.20:6000"}
	r := geideaDriver{}.Pay(context.Background(), job("geidea-webecr", 1, 2, cfg), noProgress)
	if r.Status != "cancelled" || connect["ConnectionMode"] != "TCP" || connect["IpAddress"] != "192.168.1.20" || connect["Port"] != "6000" {
		t.Fatalf("result %+v connect %v", r, connect)
	}
	if _, err := geideaConnectMsg(Job{Config: map[string]string{"machine": "nonsense"}}); err == nil {
		t.Error("bad machine address accepted")
	}
	if err := (geideaDriver{}).Check(context.Background(), job("geidea-webecr", 0, 2, cfg)); err != nil {
		t.Errorf("check: %v", err)
	}
}

func TestGeidea_ServiceNotRunning(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	r := geideaDriver{}.Pay(context.Background(), job("geidea-webecr", 1, 2, map[string]string{"webEcrUrl": "ws://" + addr + "/messages", "machine": "COM1"}), noProgress)
	if r.Status != "failed" || !strings.Contains(r.Message, "Web ECR") {
		t.Fatalf("result %+v", r)
	}
}

// ---------------------------------------------------------------- knet

func knetMock(t *testing.T, onTxn func(c net.Conn, doc string)) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				for {
					doc, err := espRead(c, time.Now().Add(10*time.Second))
					if err != nil {
						return
					}
					switch {
					case strings.Contains(doc, `Action="INIT"`):
						_ = espWrite(c, espDoc(`<Esp:Admin TerminalId="T1" Action="INIT" ActionCode="APPROVE"/>`))
					case strings.Contains(doc, "Esp:Transaction"):
						onTxn(c, doc)
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestKNET_PurchaseFils(t *testing.T) {
	var sent string
	addr := knetMock(t, func(c net.Conn, doc string) {
		sent = doc
		_ = espWrite(c, espDoc(`<Esp:Transaction TerminalId="T1" TransactionId="1" Type="PURCHASE" ActionCode="APPROVE" ResponseCode="00" AuthorizationNumber="K777" RetrievalRefNr="RRN1" CardNumber="512345XXXXXX6789"/>`))
	})
	j := job("knet-esocket", 1.25, 3, map[string]string{"esocketAddress": addr, "terminalId": "T1"})
	j.Currency = "KWD"
	r := knetDriver{}.Pay(context.Background(), j, noProgress)
	if r.Status != "approved" || r.AuthCode != "K777" || r.RRN != "RRN1" || r.MaskedPan != "••••6789" || r.Scheme != "knet" {
		t.Fatalf("result %+v", r)
	}
	if !strings.Contains(sent, `TransactionAmount="1250"`) || !strings.Contains(sent, `TerminalId="T1"`) || !strings.Contains(sent, espNS) {
		t.Errorf("request %s", sent)
	}
}

func TestKNET_DeclinedAndError(t *testing.T) {
	addr := knetMock(t, func(c net.Conn, doc string) {
		_ = espWrite(c, espDoc(`<Esp:Error ActionCode="DECLINE" ResponseCode="51" Description="Insufficient"/>`))
	})
	r := knetDriver{}.Pay(context.Background(), job("knet-esocket", 1, 3, map[string]string{"esocketAddress": addr, "terminalId": "T1"}), noProgress)
	if r.Status != "declined" || !strings.Contains(r.Message, "51") {
		t.Fatalf("result %+v", r)
	}
	if err := (knetDriver{}).Check(context.Background(), job("knet-esocket", 0, 3, map[string]string{"esocketAddress": addr, "terminalId": "T1"})); err != nil {
		t.Errorf("check: %v", err)
	}
	if err := (knetDriver{}).Check(context.Background(), job("knet-esocket", 0, 3, map[string]string{"esocketAddress": addr})); err == nil {
		t.Error("missing terminal id accepted")
	}
}

func TestKNET_CancelReverses(t *testing.T) {
	var reversal atomic.Bool
	addr := knetMock(t, func(c net.Conn, doc string) {
		if strings.Contains(doc, `Reversal="TRUE"`) {
			reversal.Store(true)
			_ = espWrite(c, espDoc(`<Esp:Transaction Reversal="TRUE" ActionCode="APPROVE"/>`))
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	r := knetDriver{}.Pay(ctx, job("knet-esocket", 1, 3, map[string]string{"esocketAddress": addr, "terminalId": "T1"}), noProgress)
	if r.Status != "cancelled" || !reversal.Load() {
		t.Fatalf("result %+v reversal %v", r, reversal.Load())
	}
}

func TestEspFraming(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	big := strings.Repeat("x", 70000)
	go func() { _ = espWrite(a, "hello"); _ = espWrite(a, big) }()
	if s, err := espRead(b, time.Now().Add(time.Second)); err != nil || s != "hello" {
		t.Fatalf("%q %v", s, err)
	}
	if s, err := espRead(b, time.Now().Add(time.Second)); err != nil || s != big {
		t.Fatalf("long frame: %d %v", len(s), err)
	}
}

// ---------------------------------------------------------------- alhamrani

func auMock(t *testing.T, onSend func(s *wsServerConn, keyword string, body map[string]string)) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/signalr/negotiate", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ConnectionToken":"tok+/=","ConnectionId":"c1","TryWebSockets":true,"ProtocolVersion":"1.5"}`))
	})
	mux.HandleFunc("/signalr/start", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"Response":"started"}`)) })
	mux.HandleFunc("/signalr/connect", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("connectionToken") != "tok+/=" || r.URL.Query().Get("transport") != "webSockets" {
			http.Error(w, "bad", 400)
			return
		}
		s, err := wsUpgrade(w, r)
		if err != nil {
			return
		}
		defer s.c.Close()
		s.send(`{"C":"s-0,1","S":1,"M":[]}`)
		for {
			m, err := s.read()
			if err != nil {
				return
			}
			var inv struct {
				H, M string
				A    []string
			}
			_ = json.Unmarshal([]byte(m), &inv)
			body := map[string]string{}
			if len(inv.A) == 2 {
				_ = json.Unmarshal([]byte(inv.A[1]), &body)
				onSend(s, inv.A[0], body)
			}
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func auReply(s *wsServerConn, resp map[string]string) {
	b, _ := json.Marshal(resp)
	f, _ := json.Marshal(map[string]interface{}{"C": "x", "M": []map[string]interface{}{{"H": "MyHub", "M": "addMessage", "A": []string{"response", string(b)}}}})
	s.send(string(f))
}

func TestAlhamrani_Purchase(t *testing.T) {
	var sent map[string]string
	srv := auMock(t, func(s *wsServerConn, kw string, b map[string]string) {
		if kw == "transaction" {
			sent = b
			auReply(s, map[string]string{"ecr_receipt_no": "0000000000", "response_code": "000"}) // another till's answer
			auReply(s, map[string]string{"ecr_receipt_no": b["ecr_receipt_no"], "response_code": "000", "auth_code": "AU1", "rrn": "R1", "pan": "588845XXXXXX4321", "card_type": "mada"})
		}
	})
	r := alhamraniDriver{}.Pay(context.Background(), job("alhamrani-signalr", 12.34, 2, map[string]string{"serviceUrl": srv.URL + "/signalr", "machinePort": "COM4"}), noProgress)
	if r.Status != "approved" || r.AuthCode != "AU1" || r.MaskedPan != "••••4321" || r.Scheme != "mada" {
		t.Fatalf("result %+v", r)
	}
	if sent["amount"] != "000000001234" || sent["msg_id"] != "PUR" || sent["port_no_or_ip_adddress"] != "COM4" || len(sent["ecr_receipt_no"]) != 10 {
		t.Errorf("request %v", sent)
	}
}

func TestAlhamrani_DeclinedRiyalsAndCheck(t *testing.T) {
	var amount string
	srv := auMock(t, func(s *wsServerConn, kw string, b map[string]string) {
		switch kw {
		case "transaction":
			amount = b["amount"]
			auReply(s, map[string]string{"ecr_receipt_no": b["ecr_receipt_no"], "response_code": "116"})
		case "check2":
			auReply(s, map[string]string{"is_connected": "true", "tid": "T9"})
		}
	})
	cfg := map[string]string{"serviceUrl": srv.URL + "/signalr", "machinePort": "192.168.1.30", "amountIn": "riyals"}
	r := alhamraniDriver{}.Pay(context.Background(), job("alhamrani-signalr", 25, 2, cfg), noProgress)
	if r.Status != "declined" || amount != "000000000025" {
		t.Fatalf("result %+v amount %s", r, amount)
	}
	if err := (alhamraniDriver{}).Check(context.Background(), job("alhamrani-signalr", 0, 2, cfg)); err != nil {
		t.Errorf("check: %v", err)
	}
}

func TestAlhamrani_Cancel(t *testing.T) {
	var cancelled atomic.Bool
	srv := auMock(t, func(s *wsServerConn, kw string, b map[string]string) {
		if kw == "cancel" {
			cancelled.Store(true)
			auReply(s, map[string]string{"response_code": "CAN"})
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	r := alhamraniDriver{}.Pay(ctx, job("alhamrani-signalr", 1, 2, map[string]string{"serviceUrl": srv.URL + "/signalr", "machinePort": "COM4"}), noProgress)
	if r.Status != "cancelled" || !cancelled.Load() {
		t.Fatalf("result %+v", r)
	}
}

// ---------------------------------------------------------------- helpers

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{"588845******1234": "••••1234", "4847 83** **** 9871": "••••9871", "12": "", "": ""} {
		if got := maskPan(in); got != want {
			t.Errorf("maskPan(%q) = %q", in, got)
		}
	}
	for in, want := range map[string]string{"P1": "mada", "MADA": "mada", "VISA": "visa", "MasterCard": "mastercard", "American Express": "amex", "": ""} {
		if got := schemeOf(in); got != want {
			t.Errorf("schemeOf(%q) = %q", in, got)
		}
	}
	if got := ecrRef(Job{Reference: "POS-1234-ABCDEFGHIJKLMNOPQRSTUV"}, 10); got != "MNOPQRSTUV" {
		t.Errorf("ecrRef %q", got)
	}
	if got := ecrRef(Job{PaymentID: "abc"}, 10); got != "abc" {
		t.Errorf("ecrRef fallback %q", got)
	}
	if !strings.Contains(fmt.Sprint(wsAccept("dGhlIHNhbXBsZSBub25jZQ==")), "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=") {
		t.Error("RFC 6455 accept key")
	}
}
