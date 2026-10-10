package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Geidea card machines through Geidea's "Web ECR" Windows service, which
// listens on ws://localhost:5000/messages and talks to the machine over USB
// (a COM port) or the shop network. JSON events: CONNECTION/CONNECT →
// OnConnect, TRANSACTION/PURCHASE → OnDataReceive (JsonResult, a JSON
// string; TransactionResponseEnglish APPROVED). The service has no cancel
// operation: the cashier cancels on the machine, which answers
// OnTerminalAction USER_CANCELLED_AND_TIMEOUT.
type geideaDriver struct{}

var _ = registerDriver(geideaDriver{})

func (geideaDriver) ID() string    { return "geidea-webecr" }
func (geideaDriver) Title() string { return "Geidea machine via Geidea Web ECR (Windows)" }

func geideaURL(j Job) string {
	if u := j.cfg("webEcrUrl"); u != "" {
		return u
	}
	return "ws://localhost:5000/messages"
}

// geideaConnectMsg: "COM3" (USB cable) or "192.168.1.20:6000" (network).
func geideaConnectMsg(j Job) (map[string]string, error) {
	m := j.cfg("machine")
	if m == "" {
		return nil, errors.New("enter how the machine is connected (COM port such as COM3, or IP:port) in StartERP (Settings → Card machines → Edit)")
	}
	if strings.HasPrefix(strings.ToUpper(m), "COM") {
		baud := j.cfg("baudRate")
		if baud == "" {
			baud = "38400"
		}
		return map[string]string{"Event": "CONNECTION", "Operation": "CONNECT", "ConnectionMode": "COM", "ComName": strings.ToUpper(m),
			"BaudRate": baud, "BraudRate": baud, "DataBits": "8", "Parity": "none"}, nil
	}
	host, port, err := net.SplitHostPort(m)
	if err != nil {
		return nil, fmt.Errorf("write the machine's network address as IP:port, for example 192.168.1.20:6000")
	}
	return map[string]string{"Event": "CONNECTION", "Operation": "CONNECT", "ConnectionMode": "TCP", "IpAddress": host, "Port": port}, nil
}

func geideaEvent(b []byte) (string, map[string]interface{}) {
	var m map[string]interface{}
	if json.Unmarshal(b, &m) != nil {
		return "", nil
	}
	return fmt.Sprint(m["Event"]), m
}

func (d geideaDriver) open(ctx context.Context, j Job) (*wsConn, error) {
	cm, err := geideaConnectMsg(j)
	if err != nil {
		return nil, err
	}
	dctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	ws, err := wsDial(dctx, geideaURL(j))
	cancel()
	if err != nil {
		return nil, fmt.Errorf("cannot reach Geidea Web ECR at %s: %v. Install and start Geidea's Web ECR service on this computer", geideaURL(j), err)
	}
	b, _ := json.Marshal(cm)
	if err := ws.WriteText(string(b)); err != nil {
		ws.Close()
		return nil, err
	}
	end := time.Now().Add(15 * time.Second)
	for time.Now().Before(end) {
		msg, err := ws.Read(end)
		if err != nil {
			ws.Close()
			return nil, fmt.Errorf("Geidea Web ECR did not connect to the machine: %v", err)
		}
		ev, m := geideaEvent(msg)
		switch ev {
		case "OnConnect":
			return ws, nil
		case "OnError", "OnDisConnect":
			ws.Close()
			return nil, fmt.Errorf("Geidea Web ECR could not connect to the machine: %v", firstOf(m, "Message", "Error", "error", "OptionalMessage"))
		}
	}
	ws.Close()
	return nil, errors.New("Geidea Web ECR did not connect to the machine in time")
}

func firstOf(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && fmt.Sprint(v) != "" {
			return fmt.Sprint(v)
		}
	}
	return "no details"
}

func (d geideaDriver) Check(ctx context.Context, j Job) error {
	ws, err := d.open(ctx, j)
	if err != nil {
		return err
	}
	_ = ws.WriteText(`{"Event":"CONNECTION","Operation":"DISCONNECT"}`)
	ws.Close()
	return nil
}

func (d geideaDriver) Pay(ctx context.Context, j Job, progress func(string)) Result {
	ws, err := d.open(ctx, j)
	if err != nil {
		return failed("%s", err.Error())
	}
	defer func() {
		_ = ws.WriteText(`{"Event":"CONNECTION","Operation":"DISCONNECT"}`)
		ws.Close()
	}()
	print := "1"
	if j.cfg("print") == "0" {
		print = "0"
	}
	req, _ := json.Marshal(map[string]string{"Event": "TRANSACTION", "Operation": "PURCHASE", "Amount": fmt.Sprintf("%.2f", j.Amount),
		"ECRNumber": ecrRef(j, 16), "PrintSettings": print, "AppId": "11"})
	if err := ws.WriteText(string(req)); err != nil {
		return failed("Could not send the amount to Geidea Web ECR: %v", err)
	}
	progress("Waiting for the card on the machine…")
	// no cancel operation: the cashier presses Cancel on the machine
	stop := func() { progress("Press Cancel on the card machine to stop the payment.") }
	return readUntilAnswer(ctx, ws, stop, func(b []byte) (Result, bool) { return parseGeidea(b, progress) })
}

func parseGeidea(b []byte, progress func(string)) (Result, bool) {
	ev, m := geideaEvent(b)
	switch ev {
	case "OnDataReceive":
		jr := map[string]interface{}{}
		switch v := m["JsonResult"].(type) {
		case string:
			_ = json.Unmarshal([]byte(v), &jr)
		case map[string]interface{}:
			jr = v
		}
		g := func(k string) string {
			if v, ok := jr[k]; ok && v != nil {
				return strings.TrimSpace(fmt.Sprint(v))
			}
			return ""
		}
		r := Result{AuthCode: g("TransactionAuthCode"), RRN: g("RetrievalReferenceNumber"), MaskedPan: maskPan(g("PrimaryAccountNumber")),
			Scheme: schemeOf(g("CardNameEnglish"))}
		text := g("TransactionResponseEnglish")
		if strings.EqualFold(text, "APPROVED") {
			r.Status, r.Message = "approved", "Approved on the card machine."
		} else {
			r.Status, r.Message = "declined", "Declined on the card machine"+suffix(text)+"."
		}
		return r, true
	case "OnTerminalAction":
		a := strings.ToUpper(fmt.Sprint(m["TerminalAction"]))
		if a == "" || a == "<NIL>" {
			a = strings.ToUpper(firstOf(m, "Action", "Message"))
		}
		if strings.Contains(a, "CANCEL") || strings.Contains(a, "TIMEOUT") {
			return Result{Status: "cancelled", Message: "Cancelled on the card machine (or no card in time)."}, true
		}
		if strings.Contains(a, "ERROR") {
			progress("Card read error: ask the customer to try the card again.")
		}
	case "OnTerminalStatus":
		if strings.Contains(strings.ToUpper(fmt.Sprint(m["TerminalStatus"])), "BUSY") {
			progress("The card machine is busy…")
		}
	case "OnError":
		return failed("Geidea Web ECR reported an error: %s", firstOf(m, "Message", "Error", "error", "OptionalMessage")), true
	case "OnDisConnect":
		return failed("The card machine disconnected. Check the machine's last receipt."), true
	}
	return Result{}, false
}
