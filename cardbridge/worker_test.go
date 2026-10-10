package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeERP is StartERP's Card Bridge API with a job queue.
type fakeERP struct {
	mu      sync.Mutex
	jobs    []Job
	results map[string][]Result
	hellos  int
	unpair  bool
	code    string
}

func newFakeERP(t *testing.T) (*fakeERP, *httptest.Server) {
	f := &fakeERP{results: map[string][]Result{}, code: "ABCD-EFGH"}
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.unpair || r.Header.Get("X-Card-Bridge-Token") != "cb1_tok" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"This Card Bridge was unpaired."}}`))
			return false
		}
		return true
	}
	mux.HandleFunc("/v1/erp/card-bridge/pair", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&b)
		if strings.ToUpper(strings.ReplaceAll(b["code"].(string), "-", "")) != "ABCDEFGH" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"code":"pairing_code","message":"This pairing code is wrong or was already used."}}`))
			return
		}
		if b["os"] == "" || b["drivers"] == nil {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"token":"cb1_tok","bridgeId":"b1","storeId":"s1","storeName":"Main shop"}`))
	})
	mux.HandleFunc("/v1/erp/card-bridge/hello", func(w http.ResponseWriter, r *http.Request) {
		if auth(w, r) {
			f.mu.Lock()
			f.hellos++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"storeName":"Main shop"}`))
		}
	})
	mux.HandleFunc("/v1/erp/card-bridge/jobs", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		end := time.Now().Add(300 * time.Millisecond)
		for time.Now().Before(end) {
			f.mu.Lock()
			if len(f.jobs) > 0 {
				j := f.jobs[0]
				f.jobs = f.jobs[1:]
				f.mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"job": j})
				return
			}
			f.mu.Unlock()
			time.Sleep(20 * time.Millisecond)
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("/v1/erp/card-bridge/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/erp/card-bridge/jobs/"), "/result")
		var res Result
		_ = json.NewDecoder(r.Body).Decode(&res)
		f.mu.Lock()
		f.results[id] = append(f.results[id], res)
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeERP) add(j Job) {
	f.mu.Lock()
	f.jobs = append(f.jobs, j)
	f.mu.Unlock()
}

func (f *fakeERP) final(id string) (Result, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.results[id] {
		if r.Status != "pending" {
			return r, true
		}
	}
	return Result{}, false
}

func pairedConfig(t *testing.T, server string) *configStore {
	cs, err := openConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = cs.update(func(c *Config) { c.Server, c.Token = server, "cb1_tok" })
	return cs
}

func TestWorker_PayCheckAndCancel(t *testing.T) {
	f, srv := newFakeERP(t)
	cs := pairedConfig(t, srv.URL+"/v1/erp")
	w := newWorker(cs, &ringLog{})
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go w.loop(ctx)

	f.add(Job{ID: "p1", Op: "pay", Driver: "simulator", PaymentID: "pay1", Amount: 1, Minor: 100, Decimals: 2, TimeoutSeconds: 180, TerminalName: "Till"})
	waitFor(t, 5*time.Second, func() bool { _, ok := f.final("p1"); return ok })
	if r, _ := f.final("p1"); r.Status != "approved" {
		t.Fatalf("pay: %+v", r)
	}
	f.mu.Lock()
	progress := f.results["p1"][0]
	f.mu.Unlock()
	if progress.Status != "pending" || !strings.Contains(progress.Message, "TEST") {
		t.Errorf("first answer should be progress: %+v", progress)
	}

	f.add(Job{ID: "c1", Op: "check", Driver: "simulator", TerminalName: "Till"})
	waitFor(t, 3*time.Second, func() bool { _, ok := f.final("c1"); return ok })
	if r, _ := f.final("c1"); r.OK == nil || !*r.OK {
		t.Errorf("check: %+v", r)
	}
	f.add(Job{ID: "c2", Op: "check", Driver: "nope", TerminalName: "Till"})
	waitFor(t, 3*time.Second, func() bool { _, ok := f.final("c2"); return ok })
	if r, _ := f.final("c2"); r.OK == nil || *r.OK || !strings.Contains(r.Message, "driver") {
		t.Errorf("unknown driver check: %+v", r)
	}

	// a payment cancelled from the till while the machine waits (.06 = no card)
	f.add(Job{ID: "p2", Op: "pay", Driver: "simulator", PaymentID: "pay2", Amount: 2.06, Minor: 206, Decimals: 2, TimeoutSeconds: 180})
	waitFor(t, 3*time.Second, func() bool {
		return w.snapshot().Busy != "" || func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.results["p2"]) > 0 }()
	})
	f.add(Job{ID: "x2", Op: "cancel", OfJob: "p2"})
	waitFor(t, 5*time.Second, func() bool { _, ok := f.final("p2"); return ok })
	if r, _ := f.final("p2"); r.Status != "timeout" {
		t.Errorf("cancelled .06 payment: %+v", r)
	}
	if r, ok := f.final("x2"); !ok || r.Status != "cancelled" {
		t.Errorf("cancel job: %+v", r)
	}

	f.add(Job{ID: "u1", Op: "pay", Driver: "missing", PaymentID: "pay3"})
	waitFor(t, 3*time.Second, func() bool { _, ok := f.final("u1"); return ok })
	if r, _ := f.final("u1"); r.Status != "failed" {
		t.Errorf("missing driver: %+v", r)
	}
	if !w.snapshot().Connected {
		t.Error("should be connected")
	}
	if cs.get().StoreName != "Main shop" {
		t.Errorf("store name from hello: %q", cs.get().StoreName)
	}
}

func TestWorker_UnpairedOnServer(t *testing.T) {
	f, srv := newFakeERP(t)
	f.unpair = true
	cs := pairedConfig(t, srv.URL+"/v1/erp")
	w := newWorker(cs, &ringLog{})
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go w.loop(ctx)
	waitFor(t, 3*time.Second, func() bool { return !cs.get().paired() })
	if w.snapshot().Connected {
		t.Error("should not be connected")
	}
}

func TestUI_PairFlowAndGuards(t *testing.T) {
	_, erp := newFakeERP(t)
	cs, _ := openConfig(filepath.Join(t.TempDir(), "config.json"))
	l := &ringLog{}
	u := newUI(cs, newWorker(cs, l), l)
	h := u.handler()
	do := func(method, target, host string, form url.Values, origin string) *httptest.ResponseRecorder {
		var r *http.Request
		if form != nil {
			r = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			r = httptest.NewRequest(method, target, nil)
		}
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	host := "127.0.0.1:17777"
	if rec := do("GET", "/", "evil.example:17777", nil, ""); rec.Code != 403 {
		t.Errorf("foreign Host must be refused (DNS rebinding): %d", rec.Code)
	}
	rec := do("GET", "/pair?code=ABCD-EFGH&server="+url.QueryEscape(erp.URL+"/v1/erp"), host, nil, "")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `value="ABCD-EFGH"`) || !strings.Contains(body, "Pair this computer") {
		t.Fatalf("pair page: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("CSP missing")
	}
	// http:// on a non-local host is refused, and is not prefilled
	if strings.Contains(do("GET", "/pair?server=http://evil.example/v1", host, nil, "").Body.String(), "evil.example") {
		t.Error("insecure server prefilled")
	}
	form := url.Values{"code": {"abcd efgh"}, "server": {erp.URL + "/v1/erp"}, "name": {"Front desk"}}
	if rec := do("POST", "/do/pair", host, form, ""); rec.Code != 403 {
		t.Errorf("POST without the page key must be refused: %d", rec.Code)
	}
	form.Set("k", u.key)
	if rec := do("POST", "/do/pair", host, form, "https://evil.example"); rec.Code != 403 {
		t.Errorf("POST from another site must be refused: %d", rec.Code)
	}
	if rec := do("POST", "/do/pair", host, url.Values{"k": {u.key}, "code": {"ZZZZ-ZZZZ"}, "server": {erp.URL + "/v1/erp"}}, "http://127.0.0.1:17777"); !strings.Contains(rec.Body.String(), "wrong or was already used") {
		t.Errorf("wrong code message: %s", rec.Body.String())
	}
	if rec := do("POST", "/do/pair", host, form, "http://127.0.0.1:17777"); rec.Code != 303 {
		t.Fatalf("pair: %d %s", rec.Code, rec.Body.String())
	}
	c := cs.get()
	if !c.paired() || c.StoreName != "Main shop" || c.Name != "Front desk" || c.Token != "cb1_tok" {
		t.Fatalf("config %+v", c)
	}
	if !strings.Contains(do("GET", "/", "localhost:17777", nil, "").Body.String(), "Main shop") {
		t.Error("status page should name the store")
	}
	if rec := do("POST", "/do/unpair", host, url.Values{"k": {u.key}}, ""); rec.Code != 303 || cs.get().paired() {
		t.Errorf("unpair: %d", rec.Code)
	}
	var st map[string]interface{}
	_ = json.Unmarshal(do("GET", "/status.json", host, nil, "").Body.Bytes(), &st)
	if st["paired"] != false || st["version"] == nil {
		t.Errorf("status.json %v", st)
	}
}

func TestValidServer(t *testing.T) {
	ok := []string{"https://startpos-api-v2.gulfunionozone.com/v1/erp/", "http://localhost:2000/v1/erp", "http://127.0.0.1:2000"}
	bad := []string{"http://evil.example", "ftp://x", "https://u:p@x.com", "https://x.com/?a=1", "", "javascript:alert(1)"}
	for _, s := range ok {
		if _, v := validServer(s); !v {
			t.Errorf("%s should be valid", s)
		}
	}
	for _, s := range bad {
		if _, v := validServer(s); v {
			t.Errorf("%s should be refused", s)
		}
	}
	if !trustedServer("https://startpos-api-v2.gulfunionozone.com/v1/erp") || trustedServer("https://example.com") {
		t.Error("trustedServer")
	}
}

func TestConfigPersists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x", "config.json")
	cs, err := openConfig(p)
	if err != nil || cs.get().Server != DefaultServer || cs.get().Port != 17777 {
		t.Fatalf("defaults %+v %v", cs.get(), err)
	}
	_ = cs.update(func(c *Config) { c.Token = "cb1_x" })
	cs2, _ := openConfig(p)
	if cs2.get().Token != "cb1_x" {
		t.Error("not saved")
	}
}
