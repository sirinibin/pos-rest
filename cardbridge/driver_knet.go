package main

import (
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
)

// KNET (Kuwait) through eSocket.POS, which KNET installs on the shop's
// computer: TCP localhost:25000, each message a 2-byte big-endian length and
// an XML document (ACI eSocket.POS). Admin INIT opens the terminal;
// Transaction PURCHASE carries the amount in fils; ActionCode APPROVE is an
// approval. A purchase the till gave up on is reversed (Reversal="TRUE"), so
// a card accepted after the cashier cancelled is given back.
type knetDriver struct{}

var _ = registerDriver(knetDriver{})

func (knetDriver) ID() string    { return "knet-esocket" }
func (knetDriver) Title() string { return "KNET terminal via eSocket.POS" }

const espNS = "http://www.mosaicsoftware.com/Postilion/eSocket.POS/"

func knetAddr(j Job) string {
	if a := j.cfg("esocketAddress"); a != "" {
		return a
	}
	return "127.0.0.1:25000"
}

func knetTID(j Job) (string, error) {
	t := j.cfg("terminalId")
	if t == "" {
		return "", errors.New("enter the KNET terminal ID in StartERP (Settings → Card machines → Edit)")
	}
	return t, nil
}

func xmlAttr(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return strings.ReplaceAll(b.String(), `"`, "&quot;")
}

func espDoc(inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><Esp:Interface Version="1.0" xmlns:Esp="` + espNS + `">` + inner + `</Esp:Interface>`
}

// espWrite / espRead: the length-prefixed frames.
func espWrite(c net.Conn, doc string) error {
	b := []byte(doc)
	var h []byte
	if len(b) < 65535 {
		h = []byte{byte(len(b) >> 8), byte(len(b))}
	} else {
		h = make([]byte, 6)
		h[0], h[1] = 0xff, 0xff
		binary.BigEndian.PutUint32(h[2:], uint32(len(b)))
	}
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := c.Write(append(h, b...))
	return err
}

func espRead(c net.Conn, deadline time.Time) (string, error) {
	_ = c.SetReadDeadline(deadline)
	h := make([]byte, 2)
	if _, err := io.ReadFull(c, h); err != nil {
		return "", err
	}
	n := int(binary.BigEndian.Uint16(h))
	if n == 0xffff {
		x := make([]byte, 4)
		if _, err := io.ReadFull(c, x); err != nil {
			return "", err
		}
		n = int(binary.BigEndian.Uint32(x))
	}
	if n > 4<<20 {
		return "", errors.New("message too large")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(c, b); err != nil {
		return "", err
	}
	return string(b), nil
}

func espAttrs(doc, element string) map[string]string {
	re := regexp.MustCompile(`<(?:Esp:)?` + element + `\b([^>]*)/?>`)
	m := re.FindStringSubmatch(doc)
	if m == nil {
		return nil
	}
	out := map[string]string{}
	for _, a := range regexp.MustCompile(`([A-Za-z_][\w.-]*)\s*=\s*"([^"]*)"`).FindAllStringSubmatch(m[1], -1) {
		out[a[1]] = a[2]
	}
	return out
}

// knetTxnID: a 6-digit transaction id per payment.
func knetTxnID(j Job) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(j.PaymentID))
	return fmt.Sprintf("%06d", (h.Sum32()+uint32(time.Now().Unix()))%1000000)
}

func (d knetDriver) open(ctx context.Context, j Job) (net.Conn, string, error) {
	tid, err := knetTID(j)
	if err != nil {
		return nil, "", err
	}
	dl := net.Dialer{Timeout: 6 * time.Second}
	c, err := dl.DialContext(ctx, "tcp", knetAddr(j))
	if err != nil {
		return nil, "", fmt.Errorf("cannot reach KNET eSocket.POS at %s: %v. Check that eSocket.POS is installed and running on this computer", knetAddr(j), err)
	}
	if err := espWrite(c, espDoc(`<Esp:Admin TerminalId="`+xmlAttr(tid)+`" Action="INIT"/>`)); err != nil {
		c.Close()
		return nil, "", err
	}
	res, err := espRead(c, time.Now().Add(20*time.Second))
	if err != nil {
		c.Close()
		return nil, "", fmt.Errorf("eSocket.POS did not answer: %v", err)
	}
	if e := espAttrs(res, "Error"); e != nil {
		c.Close()
		return nil, "", fmt.Errorf("eSocket.POS refused the terminal %s: %s %s", tid, e["ResponseCode"], e["Description"])
	}
	return c, tid, nil
}

func (d knetDriver) Check(ctx context.Context, j Job) error {
	c, tid, err := d.open(ctx, j)
	if err != nil {
		return err
	}
	_ = espWrite(c, espDoc(`<Esp:Admin TerminalId="`+xmlAttr(tid)+`" Action="CLOSE"/>`))
	return c.Close()
}

func knetResult(doc string) (Result, bool) {
	if e := espAttrs(doc, "Error"); e != nil {
		return Result{Status: "declined", Message: "KNET declined the payment" + suffix(strings.TrimSpace(e["ResponseCode"]+" "+e["Description"])) + "."}, true
	}
	t := espAttrs(doc, "Transaction")
	if t == nil {
		return Result{}, false
	}
	r := Result{AuthCode: t["AuthorizationNumber"], RRN: t["RetrievalRefNr"], MaskedPan: maskPan(t["CardNumber"]), Scheme: "knet",
		ProviderRef: t["TransactionId"]}
	if strings.EqualFold(t["ActionCode"], "APPROVE") {
		r.Status, r.Message = "approved", "Approved by KNET."
	} else {
		r.Status, r.Message = "declined", "Declined by KNET"+suffix(t["ResponseCode"])+"."
	}
	return r, true
}

func (d knetDriver) Pay(ctx context.Context, j Job, progress func(string)) Result {
	c, tid, err := d.open(ctx, j)
	if err != nil {
		return failed("%s", err.Error())
	}
	defer func() {
		_ = espWrite(c, espDoc(`<Esp:Admin TerminalId="`+xmlAttr(tid)+`" Action="CLOSE"/>`))
		c.Close()
	}()
	txn := knetTxnID(j)
	if err := espWrite(c, espDoc(fmt.Sprintf(`<Esp:Transaction TerminalId="%s" TransactionId="%s" Type="PURCHASE" TransactionAmount="%d"/>`,
		xmlAttr(tid), txn, j.Minor))); err != nil {
		return failed("Could not send the amount to KNET: %v", err)
	}
	progress("Waiting for the card on the KNET machine…")
	for {
		doc, err := espRead(c, time.Now().Add(2*time.Second))
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if ctx.Err() != nil {
					return d.reverse(c, tid, txn, j)
				}
				continue
			}
			return failed("The connection to eSocket.POS dropped (%v). Check the machine's last receipt.", err)
		}
		if r, ok := knetResult(doc); ok {
			if r.Status == "approved" && ctx.Err() != nil {
				// the till already gave up: give the money back
				return d.reverse(c, tid, txn, j)
			}
			return r
		}
	}
}

// reverse cancels (or, once approved, reverses) the purchase.
func (d knetDriver) reverse(c net.Conn, tid, txn string, j Job) Result {
	_ = espWrite(c, espDoc(fmt.Sprintf(`<Esp:Transaction Reversal="TRUE" TerminalId="%s" TransactionId="%s" Type="PURCHASE" TransactionAmount="%d"/>`,
		xmlAttr(tid), txn, j.Minor)))
	end := time.Now().Add(45 * time.Second)
	for time.Now().Before(end) {
		doc, err := espRead(c, end)
		if err != nil {
			break
		}
		if t := espAttrs(doc, "Transaction"); t != nil && strings.EqualFold(t["Reversal"], "TRUE") {
			if strings.EqualFold(t["ActionCode"], "APPROVE") {
				return Result{Status: "cancelled", Message: "Cancelled; KNET reversed the payment."}
			}
		}
		if r, ok := knetResult(doc); ok && r.Status != "approved" {
			return Result{Status: "cancelled", Message: "Cancelled on the KNET machine."}
		}
	}
	return Result{Status: "timeout", Message: "KNET did not confirm the cancel. Check the machine's last receipt and reverse it there if it was charged."}
}
