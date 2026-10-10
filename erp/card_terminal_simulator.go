package erp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// StartERP test terminal: a card machine that exists only in software, so a
// store (and our tests) can try the whole flow before a real machine is
// connected. It never moves money and every payment it approves is marked
// "TEST" in its reference. StartERP admins decide whether stores see it
// (off by default on the live server).
//
// What it answers depends on the amount's last two minor digits, like the
// test amounts of card networks:
//
//	.05 → declined   .06 → never answers (times out)   .07 → terminal offline (failed)
//	anything else → approved about 2 seconds after it was sent.
const simulatorApproveAfter = 2 * time.Second

type simulatorAdapter struct{}

var _ = registerTerminalAdapter("simulator", simulatorAdapter{})

func simOutcome(req terminalPayReq) string {
	switch req.minorUnits() % 100 {
	case 5:
		return tpDeclined
	case 6:
		return tpTimeout
	case 7:
		return tpFailed
	}
	return tpApproved
}

func (simulatorAdapter) Check(ctx context.Context, cfg terminalCfg) error {
	if strings.TrimSpace(cfg.Terminal) == "" {
		return fmt.Errorf("terminal name is missing")
	}
	return nil
}

func (simulatorAdapter) Start(ctx context.Context, cfg terminalCfg, req terminalPayReq) (terminalResult, error) {
	if simOutcome(req) == tpFailed {
		return terminalResult{Status: tpFailed, Message: "The test terminal is offline (amount ends in .07)."}, nil
	}
	ref := "sim_" + req.PaymentID + "_" + strconv.FormatInt(nowFn().UnixMilli(), 10)
	return terminalResult{Status: tpPending, ProviderRef: ref, Message: "Waiting for the card on the test terminal."}, nil
}

func simStarted(ref string) (time.Time, bool) {
	i := strings.LastIndex(ref, "_")
	if !strings.HasPrefix(ref, "sim_") || i < 0 {
		return time.Time{}, false
	}
	ms, err := strconv.ParseInt(ref[i+1:], 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(ms), true
}

func (simulatorAdapter) Status(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	at, ok := simStarted(ref)
	if !ok {
		return terminalResult{}, fmt.Errorf("unknown test payment")
	}
	if nowFn().Sub(at) < simulatorApproveAfter {
		return terminalResult{Status: tpPending, ProviderRef: ref, Message: "Waiting for the card on the test terminal."}, nil
	}
	switch simOutcome(req) {
	case tpDeclined:
		return terminalResult{Status: tpDeclined, ProviderRef: ref, Scheme: "mada", MaskedPan: "••••0005", Message: "Declined by the test terminal (amount ends in .05)."}, nil
	case tpTimeout:
		return terminalResult{Status: tpPending, ProviderRef: ref, Message: "Waiting for the card on the test terminal."}, nil
	}
	// a stable 6-digit approval code and 12-digit RRN from the payment id
	h := 0
	for _, c := range req.PaymentID {
		h = (h*31 + int(c)) % 1000000007
	}
	return terminalResult{
		Status: tpApproved, ProviderRef: ref, Scheme: "mada", MaskedPan: "••••4242",
		AuthCode: fmt.Sprintf("T%05d", h%100000), RRN: fmt.Sprintf("%012d", h%1000000000000),
		Message: "Approved by the test terminal (TEST, no money moved).",
	}, nil
}

func (simulatorAdapter) Cancel(ctx context.Context, cfg terminalCfg, req terminalPayReq, ref string) (terminalResult, error) {
	return terminalResult{Status: tpCancelled, ProviderRef: ref, Message: "Cancelled."}, nil
}
