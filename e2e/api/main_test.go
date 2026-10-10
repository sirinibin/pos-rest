//go:build e2e

// Package api holds the API end-to-end and functional tests of the StartERP
// adapter (/v1/erp). They call a RUNNING server over HTTP, exactly as the web
// app does, so routing, middleware, the legacy handlers, MongoDB, Redis and
// the background work all run as in production. Every test signs up its own
// new company (store) and owner, so tests never share data and can run in
// parallel. See e2e/README.md.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/erp"
)

var baseURL = strings.TrimRight(envOr("E2E_BASE_URL", "http://127.0.0.1:2010"), "/")

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var routes *mux.Router

func TestMain(m *testing.M) {
	routes = erp.NewRouter()
	// the server must be up: an unreachable server is a failure, not a skip
	ok := false
	for i := 0; i < 60; i++ {
		if res, err := http.Get(baseURL + "/v1/erp/meta"); err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				ok = true
				break
			}
		}
		time.Sleep(time.Second)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "e2e: no server at %s (start one with e2e/run.sh)\n", baseURL)
		os.Exit(1)
	}
	code := m.Run()
	writeRouteCoverage()
	if n := knownBugsSeen.Load(); n > 0 {
		fmt.Printf("e2e: %d checks hit known open bugs (KNOWN BUG lines above)\n", n)
	}
	os.Exit(code)
}

// ---------- route coverage of the adapter API ----------

var (
	covMu  sync.Mutex
	covHit = map[string]map[int]int{}
)

func routeKey(method, path string) string {
	req, _ := http.NewRequest(method, "http://x"+path, nil)
	var rm mux.RouteMatch
	if routes.Match(req, &rm) && rm.Route != nil && rm.MatchErr == nil {
		if tpl, err := rm.Route.GetPathTemplate(); err == nil {
			return method + " " + tpl
		}
	}
	return ""
}

func recordStatus(method, path string, code int) {
	if !strings.HasPrefix(path, "/v1/erp/") {
		return
	}
	k := routeKey(method, path)
	if k == "" {
		return
	}
	covMu.Lock()
	if covHit[k] == nil {
		covHit[k] = map[int]int{}
	}
	covHit[k][code]++
	covMu.Unlock()
}

func writeRouteCoverage() {
	var all []string
	_ = routes.Walk(func(r *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err1 := r.GetPathTemplate()
		ms, err2 := r.GetMethods()
		if err1 == nil && err2 == nil {
			for _, m := range ms {
				all = append(all, m+" "+tpl)
			}
		}
		return nil
	})
	sort.Strings(all)
	covMu.Lock()
	defer covMu.Unlock()
	called, ok := 0, 0
	var notCalled []string
	rows := []M{}
	for _, k := range all {
		st := covHit[k]
		succ := false
		for c := range st {
			if c < 400 {
				succ = true
			}
		}
		if len(st) > 0 {
			called++
		} else {
			notCalled = append(notCalled, k)
		}
		if succ {
			ok++
		}
		rows = append(rows, M{"route": k, "statuses": st, "success": succ})
	}
	fmt.Printf("e2e route coverage: %d/%d adapter routes called, %d answered with success\n", called, len(all), ok)
	if len(notCalled) > 0 && len(notCalled) <= 20 {
		fmt.Printf("e2e routes never called: %s\n", strings.Join(notCalled, ", "))
	}
	if f := os.Getenv("E2E_ROUTE_COVERAGE"); f != "" {
		b, _ := json.MarshalIndent(M{"total": len(all), "called": called, "success": ok, "routes": rows}, "", "  ")
		_ = os.WriteFile(f, b, 0o644)
	}
}
