package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// worker fetches jobs from StartERP and runs them on the card machines.
type worker struct {
	cfg *configStore
	log *ringLog

	mu      sync.Mutex
	running map[string]context.CancelFunc // pay job id → stop it
	status  workerStatus
	kick    chan struct{} // re-read the settings (paired / unpaired)
}

type workerStatus struct {
	Connected bool      `json:"connected"`
	LastPoll  time.Time `json:"lastPoll"`
	LastError string    `json:"lastError"`
	Busy      string    `json:"busy"`
	StoreName string    `json:"storeName"`
}

func newWorker(cfg *configStore, l *ringLog) *worker {
	return &worker{cfg: cfg, log: l, running: map[string]context.CancelFunc{}, kick: make(chan struct{}, 1)}
}

func (w *worker) snapshot() workerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *worker) setStatus(fn func(s *workerStatus)) {
	w.mu.Lock()
	fn(&w.status)
	w.mu.Unlock()
}

func (w *worker) wake() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

// loop runs until ctx ends: while paired it long-polls StartERP for jobs.
func (w *worker) loop(ctx context.Context) {
	backoff := time.Second
	helloDone := ""
	for ctx.Err() == nil {
		c := w.cfg.get()
		if !c.paired() {
			w.setStatus(func(s *workerStatus) { s.Connected = false; s.LastError = "" })
			select {
			case <-ctx.Done():
				return
			case <-w.kick:
			case <-time.After(5 * time.Second):
			}
			continue
		}
		cl := newClient(c.Server, c.Token)
		if helloDone != c.Token {
			if a, err := cl.hello(ctx, c.Name); err == nil {
				helloDone = c.Token
				if a.StoreName != "" && a.StoreName != c.StoreName {
					_ = w.cfg.update(func(x *Config) { x.StoreName = a.StoreName })
				}
			} else if w.handleErr(err) {
				continue
			}
		}
		job, err := cl.next(ctx, 20)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if w.handleErr(err) {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-w.kick:
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		w.setStatus(func(s *workerStatus) {
			s.Connected, s.LastPoll, s.LastError, s.StoreName = true, time.Now(), "", c.StoreName
		})
		if job != nil {
			w.handle(ctx, cl, *job)
		}
	}
}

// handleErr records an error; true when the bridge was unpaired (the loop
// then waits for a new pairing).
func (w *worker) handleErr(err error) bool {
	w.setStatus(func(s *workerStatus) { s.Connected = false; s.LastError = err.Error() })
	if unpaired(err) {
		w.log.add("This computer was unpaired in StartERP. Pair it again from Settings → Card machines.")
		_ = w.cfg.update(func(x *Config) { x.Token, x.BridgeID = "", "" })
		return true
	}
	w.log.add("Cannot reach StartERP: " + err.Error())
	return false
}

func (w *worker) handle(ctx context.Context, cl *client, j Job) {
	switch j.Op {
	case "pay":
		w.startPay(ctx, cl, j)
	case "cancel":
		w.mu.Lock()
		stop := w.running[j.OfJob]
		w.mu.Unlock()
		if stop != nil {
			w.log.add("Cancelling the payment on " + j.TerminalName + "…")
			stop()
		}
		// the pay job answers with the machine's final state; this job just closes
		ok := true
		_ = cl.result(ctx, j.ID, Result{Status: "cancelled", OK: &ok, Message: "Cancel sent to the card machine."})
	case "check":
		go func() {
			d := driverByID(j.Driver)
			ok := false
			r := Result{Status: "failed", OK: &ok}
			if d == nil {
				r.Message = fmt.Sprintf("This Card Bridge (%s) has no %q driver. Download the latest Card Bridge.", Version, j.Driver)
			} else {
				cctx, cancel := context.WithTimeout(ctx, 12*time.Second)
				err := d.Check(cctx, j)
				cancel()
				if err != nil {
					r.Message = err.Error()
				} else {
					ok = true
					r = Result{Status: "approved", OK: &ok, Message: "The Card Bridge reached " + j.TerminalName + "."}
				}
			}
			w.log.add("Check " + j.TerminalName + ": " + r.Message)
			_ = cl.result(ctx, j.ID, r)
		}()
	default:
		_ = cl.result(ctx, j.ID, failed("This Card Bridge does not know the job %q. Download the latest Card Bridge.", j.Op))
	}
}

func (w *worker) startPay(ctx context.Context, cl *client, j Job) {
	d := driverByID(j.Driver)
	if d == nil {
		_ = cl.result(ctx, j.ID, failed("This Card Bridge (%s) has no %q driver. Download the latest Card Bridge.", Version, j.Driver))
		return
	}
	timeout := time.Duration(j.TimeoutSeconds) * time.Second
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 3 * time.Minute
	}
	// stop a little before StartERP gives up, so the machine's own answer wins
	pctx, stop := context.WithTimeout(ctx, timeout-10*time.Second)
	w.mu.Lock()
	w.running[j.ID] = stop
	w.mu.Unlock()
	w.setStatus(func(s *workerStatus) { s.Busy = j.TerminalName })
	w.log.add(fmt.Sprintf("Payment %.*f %s sent to %s", j.Decimals, j.Amount, j.Currency, j.TerminalName))
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("driver %s crashed: %v", j.Driver, r)
				_ = cl.result(context.Background(), j.ID, failed("The %s driver stopped unexpectedly.", j.Driver))
			}
			stop()
			w.mu.Lock()
			delete(w.running, j.ID)
			w.mu.Unlock()
			w.setStatus(func(s *workerStatus) { s.Busy = "" })
		}()
		var last string
		progress := func(m string) {
			if m != last {
				last = m
				_ = cl.result(ctx, j.ID, Result{Status: "pending", Message: m})
			}
		}
		r := d.Pay(pctx, j, progress)
		if r.Status == "" || r.Status == "pending" {
			r.Status, r.Message = "timeout", "The card machine did not answer in time."
		}
		w.log.add(fmt.Sprintf("Payment on %s: %s %s", j.TerminalName, r.Status, r.Message))
		// a fresh context: the answer must reach StartERP even when the payment was cancelled
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := cl.result(rctx, j.ID, r); err != nil {
			w.log.add("Could not send the machine's answer to StartERP: " + err.Error())
		}
	}()
}

// ringLog keeps the last lines for the local page.
type ringLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *ringLog) add(s string) {
	line := time.Now().Format("15:04:05") + "  " + s
	log.Print(s)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line)
	if len(l.lines) > 200 {
		l.lines = l.lines[len(l.lines)-200:]
	}
}

func (l *ringLog) tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.lines) {
		n = len(l.lines)
	}
	return append([]string{}, l.lines[len(l.lines)-n:]...)
}
