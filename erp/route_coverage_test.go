package erp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/gorilla/mux"
)

// Route coverage: with ERP_ROUTE_COVERAGE=<file> every request the tests send
// through testRouter is recorded as "METHOD /v1/erp/<route template>" with the
// HTTP statuses it answered. After the run the report (JSON) is written to
// <file> and the routes no test called are printed. ERP_ROUTE_COVERAGE_MIN=<n>
// fails the run when fewer than n percent of the routes were called with a
// success status and at least one error status (CI gate against untested
// endpoints).

type routeCoverage struct {
	router *mux.Router
	mu     sync.Mutex
	hits   map[string]map[int]int
}

func newRouteCoverage(r *mux.Router) *routeCoverage {
	return &routeCoverage{router: r, hits: map[string]map[int]int{}}
}

func (c *routeCoverage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	key := ""
	var m mux.RouteMatch
	if c.router.Match(req, &m) && m.Route != nil && m.MatchErr == nil {
		if tpl, err := m.Route.GetPathTemplate(); err == nil {
			key = req.Method + " " + tpl
		}
	}
	sw := &statusWriter{ResponseWriter: w, code: 200}
	c.router.ServeHTTP(sw, req)
	if key == "" {
		return
	}
	c.mu.Lock()
	if c.hits[key] == nil {
		c.hits[key] = map[int]int{}
	}
	c.hits[key][sw.code]++
	c.mu.Unlock()
}

type statusWriter struct {
	http.ResponseWriter
	code    int
	written bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.written {
		s.code, s.written = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.written = true
	return s.ResponseWriter.Write(b)
}

// allRoutes lists "METHOD template" for every route on the adapter router.
func allRoutes(r *mux.Router) []string {
	var out []string
	_ = r.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err := route.GetPathTemplate()
		if err != nil {
			return nil
		}
		ms, err := route.GetMethods()
		if err != nil {
			return nil
		}
		for _, m := range ms {
			out = append(out, m+" "+tpl)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

type routeRow struct {
	Route    string      `json:"route"`
	Statuses map[int]int `json:"statuses"`
	Success  bool        `json:"success"`
	Error    bool        `json:"error"`
}

// report writes the coverage file and returns (called, both, total, untested).
func (c *routeCoverage) report(file string) (int, int, int, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	routes := allRoutes(c.router)
	rows := make([]routeRow, 0, len(routes))
	var untested []string
	called, both := 0, 0
	for _, rt := range routes {
		st := c.hits[rt]
		row := routeRow{Route: rt, Statuses: st}
		for code := range st {
			if code < 400 {
				row.Success = true
			} else {
				row.Error = true
			}
		}
		if len(st) == 0 {
			untested = append(untested, rt)
		} else {
			called++
		}
		if row.Success && row.Error {
			both++
		}
		rows = append(rows, row)
	}
	if b, err := json.MarshalIndent(M{"total": len(routes), "called": called, "successAndError": both, "routes": rows}, "", "  "); err == nil {
		_ = os.WriteFile(file, b, 0o644)
	}
	return called, both, len(routes), untested
}

// finishRouteCoverage prints the summary; it returns false when the
// ERP_ROUTE_COVERAGE_MIN gate fails.
func finishRouteCoverage(c *routeCoverage) bool {
	file := os.Getenv("ERP_ROUTE_COVERAGE")
	called, both, total, untested := c.report(file)
	fmt.Printf("route coverage: %d/%d routes called, %d with both a success and an error case (report: %s)\n", called, total, both, file)
	if len(untested) > 0 {
		fmt.Printf("routes no test calls:\n  %s\n", strings.Join(untested, "\n  "))
	}
	var min float64
	if _, err := fmt.Sscanf(os.Getenv("ERP_ROUTE_COVERAGE_MIN"), "%g", &min); err == nil && total > 0 {
		pct := 100 * float64(called) / float64(total)
		if pct < min {
			fmt.Printf("FAIL route coverage %.1f%% is below ERP_ROUTE_COVERAGE_MIN=%g%%\n", pct, min)
			return false
		}
	}
	return true
}
