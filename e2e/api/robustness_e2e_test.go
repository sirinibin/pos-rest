//go:build e2e

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

// robPublic are the adapter routes that answer without a token.
var robPublic = map[string]bool{
	"GET /v1/erp/meta": true, "GET /v1/erp/countries": true, "POST /v1/erp/auth/login": true,
	"POST /v1/erp/auth/refresh": true, "POST /v1/erp/auth/signup": true,
	// card machines: provider call-backs are signed (?sig=), the Card Bridge
	// setup program is a public download and pairs with a one-time code
	"GET /v1/erp/card-terminal-webhooks/{provider}/{store}/{payment}":  true,
	"POST /v1/erp/card-terminal-webhooks/{provider}/{store}/{payment}": true,
	"GET /v1/erp/card-bridge/downloads":                                true, "GET /v1/erp/card-bridge/download/{file}": true,
	"POST /v1/erp/card-bridge/pair": true,
	// registered companies per country, for the marketing site footers
	"GET /v1/erp/site/companies": true,
}

// robKnown5xx are the routes that answer 5xx (or drop the connection) for a
// malformed request today. Each is reported as a known bug; a route that
// stops failing makes the test ask for its entry to be removed.
var robKnown5xx = map[string]string{}

type robReq struct {
	key, method, path, label string
	body                     interface{} // nil = no body; string = raw
	token                    string
}

type robRes struct {
	code int
	err  error
	body string
}

var robClient = &http.Client{Timeout: 60 * time.Second}

func robDo(q robReq) robRes {
	var rd io.Reader
	switch b := q.body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = strings.NewReader(string(j))
	}
	req, err := http.NewRequest(q.method, baseURL+q.path, rd)
	if err != nil {
		return robRes{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if q.token != "" {
		req.Header.Set("Authorization", "Bearer "+q.token)
	}
	res, err := robClient.Do(req)
	if err != nil {
		return robRes{err: err}
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	recordStatus(q.method, req.URL.Path, res.StatusCode)
	return robRes{code: res.StatusCode, body: string(b)}
}

var robVar = regexp.MustCompile(`\{[^}]+\}`)

// robPaths fills the route's variables with every combination of test values.
func robPaths(tpl string, ids []string) []string {
	out := []string{tpl}
	for {
		loc := robVar.FindStringIndex(out[0])
		if loc == nil {
			return out
		}
		name := out[0][loc[0]+1 : loc[1]-1]
		vals := ids
		if name == "docType" {
			vals = []string{"sales", "quotations", "purchases", "nope"}
		}
		next := []string{}
		for _, p := range out {
			l := robVar.FindStringIndex(p)
			for _, v := range vals {
				next = append(next, p[:l[0]]+v+p[l[1]:])
			}
		}
		out = next
	}
}

func TestRobustness_EveryRouteMalformedInput(t *testing.T) {
	t.Parallel()
	start := time.Now()
	a, b := Signup(t, ""), secStore(t)
	bc := b.Customer(t, "")
	owner := a.Token
	// the owner's own records: malformed bodies on them reach the update code
	ap := a.Product(t, 10, 20, 5)
	ac := a.Customer(t, "")
	own := map[string]string{"customers": S(ac["id"]), "products": S(ap["id"]), "vendors": S(a.Vendor(t)["id"]), "stores": a.ID,
		"sales": S(Create(t, owner, "sales", M{"storeId": a.ID, "date": a.Now(), "customerId": ac["id"], "items": []M{a.Line(S(ap["id"]), 1, 20)},
			"payments": []M{{"date": a.Now(), "amount": 23, "method": "cash"}}})["id"])}
	ids := []string{S(bc["id"]), "000000000000000000000000", "x", strings.Repeat("a", 300)}
	long := strings.Repeat("ق", 5000)
	bodies := []struct {
		label string
		body  interface{}
	}{
		{"no body", nil},
		{"null", "null"},
		{"broken json", "{"},
		{"array", "[]"},
		{"zero store", M{"storeId": "000000000000000000000000"}},
		{"wrong types", M{"storeId": a.ID, "qty": "abc", "items": "x", "lines": "x", "payments": "x", "date": "not-a-date",
			"amount": -1e308, "unitPrice": "x", "customerId": 5, "vendorId": []int{1}, "payload": "x", "perms": "x", "storeIds": "x",
			"stock": "x", "pricing": "x", "otp": 7, "email": 7, "password": 7, "refreshToken": 7, "status": M{}, "role": []int{}}},
		{"long strings", M{"storeId": a.ID, "nameEn": long, "name": long, "nameAr": long, "remarks": long, "email": long + "@x.com",
			"phone": long, "code": long, "description": long, "plate": long, "title": long}},
	}
	queries := []string{"", "limit=-1", "limit=100000", "page=0", "from=garbage", "to=garbage", "sort=nope", "select=%%%"}

	var reqs []robReq
	_ = routes.Walk(func(r *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tpl, err1 := r.GetPathTemplate()
		ms, err2 := r.GetMethods()
		if err1 != nil || err2 != nil {
			return nil
		}
		for _, m := range ms {
			key := m + " " + tpl
			paths := robPaths(tpl, ids)
			// no token: one request per route (all variables filled)
			reqs = append(reqs, robReq{key: key, method: m, path: paths[0], label: "no token"})
			if key == "POST /v1/erp/auth/logout" {
				continue // would end the owner's session; covered in TestAuth
			}
			if seg := strings.Split(strings.TrimPrefix(tpl, "/v1/erp/"), "/"); len(seg) > 1 && seg[1] == "{id}" && own[seg[0]] != "" &&
				m != "DELETE" && !strings.HasSuffix(tpl, "/starter-catalog") {
				paths = append(paths, strings.Replace(tpl, "{id}", own[seg[0]], 1))
			}
			for _, p := range paths {
				if m == "GET" {
					for _, q := range queries {
						for _, st := range []string{a.ID, "000000000000000000000000", "x"} {
							sep := "?"
							qs := "storeId=" + st
							if q != "" {
								qs += "&" + q
							}
							reqs = append(reqs, robReq{key: key, method: m, path: p + sep + qs, label: "query " + qs, token: owner})
						}
					}
					continue
				}
				for _, bd := range bodies {
					reqs = append(reqs, robReq{key: key, method: m, path: p, label: "body " + bd.label, body: bd.body, token: owner})
				}
			}
		}
		return nil
	})

	type hit struct{ label, path, detail string }
	var (
		mu       sync.Mutex
		bad      = map[string][]hit{} // route → 5xx / dropped
		noAuth   = map[string]string{}
		requests int
		statuses = map[int]int{}
	)
	work := make(chan robReq)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for q := range work {
				res := robDo(q)
				mu.Lock()
				requests++
				statuses[res.code]++
				switch {
				case res.err != nil:
					bad[q.key] = append(bad[q.key], hit{q.label, q.path, "no answer: " + res.err.Error()})
				case res.code >= 500:
					bad[q.key] = append(bad[q.key], hit{q.label, q.path, fmt.Sprintf("HTTP %d %.200s", res.code, res.body)})
				}
				if q.token == "" && !robPublic[q.key] && res.err == nil && res.code != 401 {
					noAuth[q.key] = fmt.Sprintf("HTTP %d %.150s", res.code, res.body)
				}
				mu.Unlock()
			}
		}()
	}
	for _, q := range reqs {
		work <- q
	}
	close(work)
	wg.Wait()

	// the server is still up and serving
	Must(t, Call(t, "GET", "/meta", "", nil), 200, "meta after the sweep")
	Must(t, Call(t, "GET", "/auth/me", owner, nil), 200, "owner token after the sweep")

	for k, v := range noAuth {
		t.Errorf("%s answers without a token: %s", k, v)
	}
	keys := make([]string, 0, len(bad))
	for k := range bad {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		hs := bad[k]
		causes := map[string]bool{}
		for _, h := range hs {
			causes[h.label] = true
		}
		var cl []string
		for c := range causes {
			cl = append(cl, c)
		}
		sort.Strings(cl)
		what := fmt.Sprintf("%s: %d requests failed (%s); e.g. %s %s → %s", k, len(hs), strings.Join(cl, "; "), hs[0].label, robShort(hs[0].path), hs[0].detail)
		if id, ok := robKnown5xx[k]; ok {
			KnownBug(t, id, what, true)
		} else {
			t.Errorf("5xx / dropped connection (new bug): %s", what)
		}
	}
	for k, id := range robKnown5xx {
		if _, ok := bad[k]; !ok {
			KnownBug(t, id, k+" answers 5xx for malformed input", false)
		}
	}
	t.Logf("robustness sweep: %d requests in %s, statuses %v (0 = no answer); %d routes with 5xx/panic", requests, time.Since(start).Round(time.Second), statuses, len(bad))
	if statuses[200]+statuses[201] == 0 || statuses[400] == 0 || statuses[401] == 0 || statuses[403] == 0 || statuses[404] == 0 {
		t.Errorf("the sweep should see successes and 400/401/403/404 answers: %v", statuses)
	}
}

func robShort(p string) string {
	if len(p) > 140 {
		return p[:140] + "…"
	}
	return p
}

// Records created at the same moment must still get distinct codes.
func TestRobustness_ConcurrentCreateCodes(t *testing.T) {
	t.Parallel()
	s := secStore(t)
	for _, res := range []struct {
		path string
		body func() M
	}{
		{"products", func() M {
			return M{"storeId": s.ID, "nameEn": "Conc " + Uniq(), "pricing": M{"purchase": 1, "retail": 2}}
		}},
		{"customers", func() M { return M{"storeId": s.ID, "nameEn": "Conc " + Uniq()} }},
	} {
		const n = 12
		codes := make([]string, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				if r := robDo(robReq{method: "POST", path: "/v1/erp/" + res.path, body: res.body(), token: s.Token}); r.err == nil && r.code == 201 {
					var m M
					_ = json.Unmarshal([]byte(r.body), &m)
					codes[i] = S(m["code"])
				}
			}(i)
		}
		wg.Wait()
		seen := map[string]int{}
		dups := []string{}
		for _, c := range codes {
			if c == "" {
				t.Errorf("%s: a concurrent create failed: %v", res.path, codes)
				break
			}
			if seen[c]++; seen[c] == 2 {
				dups = append(dups, c)
			}
		}
		// the code must be unique: the products PATCH refuses a shared one ("already used by another product")
		// a race: it doesn't show on every run, so a clean run is only logged
		if len(dups) > 0 {
			KnownBug(t, "NEW-CODES-"+strings.ToUpper(res.path), fmt.Sprintf("%d concurrent POST /%s answered duplicate codes %v", n, res.path, dups), true)
		} else {
			t.Logf("%s: %d concurrent creates got distinct codes this run", res.path, n)
		}
	}
}
