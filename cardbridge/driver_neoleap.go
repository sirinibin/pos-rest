package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// Neoleap mada terminals (Saudi Arabia) in ECR mode: the terminal's mada app
// listens on ws://<terminal-ip>:7000 (some N950 builds use 9998). One JSON
// message per frame: CHECK_STATUS → TERMINAL_STATUS, SALE → TERMINAL_RESPONSE
// (StatusCode 00 approved, 01 declined, 11 cancelled), CANCEL stops a sale.
// Newer N950 firmware answers with a <madaTransactionResult> XML block after
// a JSON prefix; both forms are read.
type neoleapDriver struct{}

var _ = registerDriver(neoleapDriver{})

func (neoleapDriver) ID() string    { return "neoleap-ws" }
func (neoleapDriver) Title() string { return "Neoleap mada terminal (shop network, ECR)" }

// neoleapAddr: host + port setting → ws://host:port
func neoleapAddr(j Job) (string, error) {
	h := j.cfg("host")
	if h == "" {
		return "", errors.New("enter the card machine's IP address in StartERP (Settings → Card machines → Edit)")
	}
	if strings.Contains(h, "://") {
		return h, nil
	}
	port := j.cfg("port")
	if port == "" {
		port = "7000"
	}
	if _, _, err := net.SplitHostPort(h); err == nil {
		return "ws://" + h, nil
	}
	return "ws://" + net.JoinHostPort(h, port), nil
}

func (d neoleapDriver) status(ctx context.Context, ws *wsConn) (string, error) {
	if err := ws.WriteText(`{"Command":"CHECK_STATUS"}`); err != nil {
		return "", err
	}
	end := time.Now().Add(8 * time.Second)
	for time.Now().Before(end) {
		b, err := ws.Read(end)
		if err != nil {
			return "", err
		}
		var m map[string]interface{}
		if json.Unmarshal(b, &m) == nil && fmt.Sprint(m["EventName"]) == "TERMINAL_STATUS" {
			return strings.ToUpper(fmt.Sprint(m["TerminalStatus"])), nil
		}
	}
	return "", errors.New("the card machine did not answer the status check")
}

func (d neoleapDriver) Check(ctx context.Context, j Job) error {
	addr, err := neoleapAddr(j)
	if err != nil {
		return err
	}
	ws, err := wsDial(ctx, addr)
	if err != nil {
		return fmt.Errorf("cannot reach the card machine at %s: %v. Check the IP address and that ECR mode is on", addr, err)
	}
	defer ws.Close()
	st, err := d.status(ctx, ws)
	if err != nil {
		return err
	}
	if st != "READY" {
		return fmt.Errorf("the card machine answered %s", st)
	}
	return nil
}

func (d neoleapDriver) Pay(ctx context.Context, j Job, progress func(string)) Result {
	addr, err := neoleapAddr(j)
	if err != nil {
		return failed("%s", err.Error())
	}
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	ws, err := wsDial(dctx, addr)
	cancel()
	if err != nil {
		return failed("Cannot reach the card machine at %s: %v", addr, err)
	}
	defer ws.Close()
	if st, err := d.status(ctx, ws); err != nil {
		return failed("The card machine did not answer: %v", err)
	} else if st == "BUSY" {
		return failed("The card machine is busy with another payment. Finish it on the machine and try again.")
	}
	req, _ := json.Marshal(map[string]string{"Command": "SALE", "Amount": fmt.Sprintf("%.2f", j.Amount), "AdditionalData": ecrRef(j, 20)})
	if err := ws.WriteText(string(req)); err != nil {
		return failed("Could not send the amount to the card machine: %v", err)
	}
	progress("Waiting for the card on the machine…")
	return readUntilAnswer(ctx, ws, func() { _ = ws.WriteText(`{"Command":"CANCEL"}`) }, parseNeoleap)
}

// readUntilAnswer reads messages until parse returns a final answer. When
// ctx ends it calls stop once and keeps reading for the machine's own final
// answer (a card accepted at the last moment is still reported).
func readUntilAnswer(ctx context.Context, ws *wsConn, stop func(), parse func([]byte) (Result, bool)) Result {
	stopped := false
	var after time.Time
	for {
		dl := time.Now().Add(2 * time.Second)
		if stopped && time.Now().After(after) {
			return Result{Status: "timeout", Message: "The card machine did not confirm the cancel. Check its last receipt before taking the payment again."}
		}
		b, err := ws.Read(dl)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if !stopped && ctx.Err() != nil {
					stopped = true
					after = time.Now().Add(45 * time.Second)
					stop()
				}
				continue
			}
			if stopped {
				return Result{Status: "cancelled", Message: "Cancelled on the card machine."}
			}
			return failed("The connection to the card machine dropped (%v). Check the machine's last receipt.", err)
		}
		if r, ok := parse(b); ok {
			return r
		}
		if !stopped && ctx.Err() != nil {
			stopped = true
			after = time.Now().Add(45 * time.Second)
			stop()
		}
	}
}

var (
	reXMLTag = func(tag string) *regexp.Regexp {
		return regexp.MustCompile(`<` + tag + `[^>]*>\s*([^<]*?)\s*</` + tag + `>`)
	}
	reMadaApp = reXMLTag("ApprovalCode")
	reMadaRRN = reXMLTag("RRN")
	reMadaPAN = reXMLTag("PAN")
	reMadaTSC = reXMLTag("TerminalStatusCode")
	reMadaRes = regexp.MustCompile(`<Result[^>]*English\s*=\s*["']([^"']+)["']`)
)

func xmlVal(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseNeoleap(b []byte) (Result, bool) {
	s := string(b)
	if i := strings.Index(s, "<madaTransactionResult"); i >= 0 {
		x := s[i:]
		r := Result{AuthCode: xmlVal(reMadaApp, x), RRN: xmlVal(reMadaRRN, x), MaskedPan: maskPan(xmlVal(reMadaPAN, x)), Scheme: "mada"}
		res := strings.ToUpper(xmlVal(reMadaRes, x))
		code := xmlVal(reMadaTSC, x)
		switch {
		case res == "APPROVED" || code == "00":
			r.Status, r.Message = "approved", "Approved on the card machine."
		case code == "11":
			r.Status, r.Message = "cancelled", "Cancelled on the card machine."
		default:
			r.Status, r.Message = "declined", "Declined on the card machine"+suffix(res)+"."
		}
		return r, true
	}
	var m map[string]interface{}
	if json.Unmarshal(b, &m) != nil || fmt.Sprint(m["EventName"]) != "TERMINAL_RESPONSE" {
		return Result{}, false
	}
	jr := map[string]interface{}{}
	switch v := m["JsonResult"].(type) {
	case map[string]interface{}:
		jr = v
	case string:
		_ = json.Unmarshal([]byte(v), &jr)
	}
	g := func(k string) string {
		if v, ok := jr[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
	r := Result{AuthCode: g("TransactionAuthCode"), RRN: g("RetrievalReferenceNumber"), MaskedPan: maskPan(g("PrimaryAccountNumber")),
		Scheme: schemeOf(g("CardScheme")), ProviderRef: g("ECRReferenceNumber")}
	text := g("TransactionResponseEnglish")
	switch g("StatusCode") {
	case "00":
		r.Status, r.Message = "approved", "Approved on the card machine."
	case "11":
		r.Status, r.Message = "cancelled", "Cancelled on the card machine."
	case "":
		if strings.EqualFold(text, "APPROVED") {
			r.Status, r.Message = "approved", "Approved on the card machine."
		} else {
			r.Status, r.Message = "failed", "The card machine answered without a result"+suffix(text)+"."
		}
	default:
		r.Status, r.Message = "declined", "Declined on the card machine"+suffix(text)+"."
	}
	return r, true
}

func suffix(s string) string {
	if s = strings.TrimSpace(s); s != "" {
		return ": " + s
	}
	return ""
}

// maskPan keeps only the last four digits.
func maskPan(p string) string {
	p = strings.TrimSpace(p)
	d := []rune{}
	for _, r := range p {
		if r >= '0' && r <= '9' {
			d = append(d, r)
		}
	}
	if len(d) < 4 {
		return ""
	}
	return "••••" + string(d[len(d)-4:])
}

func schemeOf(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case s == "":
		return ""
	case strings.Contains(s, "mada") || s == "p1":
		return "mada"
	case strings.Contains(s, "visa"):
		return "visa"
	case strings.Contains(s, "master") || s == "mc":
		return "mastercard"
	case strings.Contains(s, "amex") || strings.Contains(s, "american"):
		return "amex"
	case strings.Contains(s, "knet"):
		return "knet"
	case strings.Contains(s, "union"):
		return "unionpay"
	}
	return s
}

// ecrRef: the payment's reference for the machine (letters and digits, max n).
func ecrRef(j Job, n int) string {
	src := j.Reference
	if src == "" {
		src = j.PaymentID
	}
	out := []rune{}
	for _, r := range src {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			out = append(out, r)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return string(out)
}
