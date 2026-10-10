package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Job is one piece of work from StartERP: send an amount ("pay"), stop a
// payment still on the machine ("cancel") or check the machine ("check").
type Job struct {
	ID             string            `json:"id"`
	Op             string            `json:"op"`
	Driver         string            `json:"driver"`
	Provider       string            `json:"provider"`
	TerminalID     string            `json:"terminalId"`
	TerminalName   string            `json:"terminalName"`
	PaymentID      string            `json:"paymentId"`
	OfJob          string            `json:"ofJob"`
	Amount         float64           `json:"amount"`
	Minor          int64             `json:"minor"`
	Currency       string            `json:"currency"`
	Decimals       int               `json:"decimals"`
	Reference      string            `json:"reference"`
	Mode           string            `json:"mode"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
	Config         map[string]string `json:"config"`
}

// cfg reads one machine setting.
func (j Job) cfg(k string) string { return strings.TrimSpace(j.Config[k]) }

// Result is the machine's answer. Status: approved, declined, cancelled,
// failed, timeout (or pending for a progress message).
type Result struct {
	Status      string `json:"status,omitempty"`
	OK          *bool  `json:"ok,omitempty"`
	AuthCode    string `json:"authCode,omitempty"`
	RRN         string `json:"rrn,omitempty"`
	MaskedPan   string `json:"maskedPan,omitempty"`
	Scheme      string `json:"scheme,omitempty"`
	Message     string `json:"message,omitempty"`
	ProviderRef string `json:"providerRef,omitempty"`
}

func failed(format string, a ...interface{}) Result {
	return Result{Status: "failed", Message: fmt.Sprintf(format, a...)}
}

// Driver talks to one kind of card machine. Pay blocks until the machine
// answers; cancelling ctx asks the machine to stop the payment (the driver
// still returns the machine's final answer, which may be approved when the
// card was already accepted).
type Driver interface {
	ID() string
	// Title is shown on the bridge's local page.
	Title() string
	Check(ctx context.Context, job Job) error
	Pay(ctx context.Context, job Job, progress func(string)) Result
}

var (
	driversMu sync.RWMutex
	drivers   = map[string]Driver{}
)

func registerDriver(d Driver) bool {
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[d.ID()] = d
	return true
}

func driverByID(id string) Driver {
	driversMu.RLock()
	defer driversMu.RUnlock()
	return drivers[id]
}

func driverIDs() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	ids := make([]string, 0, len(drivers))
	for id := range drivers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ---------------------------------------------------------------- simulator

// simulatorDriver is the Card Bridge test machine: no money moves. Like the
// server's test terminal the amount's last two minor digits pick the answer:
// .05 declined, .06 no card (time-out), .07 machine offline, else approved
// after two seconds.
type simulatorDriver struct{ delay time.Duration }

var _ = registerDriver(simulatorDriver{delay: 2 * time.Second})

func (simulatorDriver) ID() string    { return "simulator" }
func (simulatorDriver) Title() string { return "Test machine (no money moves)" }

func (simulatorDriver) Check(ctx context.Context, job Job) error { return nil }

func (s simulatorDriver) Pay(ctx context.Context, job Job, progress func(string)) Result {
	progress("TEST machine: waiting for the card…")
	switch job.Minor % 100 {
	case 7:
		return failed("The test machine is offline (amount ends in .07).")
	case 6:
		<-ctx.Done()
		return Result{Status: "timeout", Message: "No card was presented in time (amount ends in .06)."}
	}
	select {
	case <-ctx.Done():
		return Result{Status: "cancelled", Message: "Cancelled on the test machine."}
	case <-time.After(s.delay):
	}
	if job.Minor%100 == 5 {
		return Result{Status: "declined", Scheme: "mada", MaskedPan: "••••0005", Message: "Declined by the test machine (amount ends in .05)."}
	}
	h := uint32(2166136261)
	for _, c := range job.PaymentID {
		h = (h ^ uint32(c)) * 16777619
	}
	return Result{Status: "approved", AuthCode: fmt.Sprintf("B%05d", h%100000), RRN: fmt.Sprintf("%012d", h),
		Scheme: "mada", MaskedPan: "••••4242", ProviderRef: "bridge-sim-" + job.PaymentID,
		Message: "Approved by the Card Bridge test machine (TEST, no money moved)."}
}
