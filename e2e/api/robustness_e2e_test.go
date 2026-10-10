//go:build e2e

package apie2e

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Robustness sweep: every route registered in main.go is called with bad,
// empty and hostile input by a signed-in user. Whatever the input, the
// server must answer: a handler panic shows up here as a connection closed
// without a response (net/http recovers it and drops the connection), and a
// hang shows up as a timeout. The sweep found ~180 such crashes when it was
// written (a zero store id, a JSON null body, documents without vat_percent,
// payments for unknown invoices, ...).

type route struct{ method, path string }

var routeRe = regexp.MustCompile(`HandleFunc\("([^"]+)",\s*controller\.\w+\)\.Methods\("([A-Z]+)"\)`)

// routesFromMain lists the routes in main.go, ignoring commented-out ones.
func routesFromMain(t *testing.T) []route {
	t.Helper()
	raw, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(raw), "")
	seen := map[route]bool{}
	var out []route
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 && !strings.Contains(line[:i], `"`) {
			line = line[:i]
		}
		for _, m := range routeRe.FindAllStringSubmatch(line, -1) {
			r := route{m[2], m[1]}
			if !seen[r] {
				seen[r] = true
				out = append(out, r)
			}
		}
	}
	if len(out) < 300 {
		t.Fatalf("found only %d routes in main.go; did the registration format change?", len(out))
	}
	return out
}

// Routes the sweep leaves alone: they end the session, act on the host
// (systemctl, disk cleanup), or call third-party services (WhatsApp, e-mail,
// Google, ZATCA, file hosts) whose failures are not this server's.
var sweepSkip = regexp.MustCompile(`^/v1/(logout|admin/|verify-cleanup-disk|dashboard/backfill|whatsapp|rfq-bot|rfq-email|rfq-store|email-accounts|outgoing-email|procurement-email-send|procurement-extract-test|chart-image-share|share-pdf|upload-pdf|translate|store/zatca|store/\{id\}/zatca|mcp|rfq-suppliers/fetch-from-maps|rfq-received/extract|bi/batch-cost|bi/aws-batch-settings|server-status)`)

var pathVar = regexp.MustCompile(`\{[^}]+\}`)

// rawCall sends a request and reports whether the server answered.
func rawCall(method, path, auth string, body string) (int, error) {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, baseURL()+path, r)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	io.Copy(io.Discard, res.Body)
	return res.StatusCode, nil
}

func describeFailure(err error) string {
	if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") {
		return "connection closed without a response (handler panic; see the server log)"
	}
	return err.Error()
}

func TestRobustness_EveryRouteAnswersBadInputWithoutCrashing(t *testing.T) {
	tok := authToken(t)
	sid := testStore(t)
	const fakeID = "5f1d7f3e9b1e8a3f4c2b1a00"

	queries := map[string]url.Values{
		"valid store":   {"search[store_id]": {sid}},
		"zero store id": {"search[store_id]": {"000000000000000000000000"}},
		"no store id":   {},
		"junk params": {
			"search[store_id]": {sid}, "page": {"abc"}, "limit": {"-5"}, "sort": {"-"},
			"search[created_at]": {"notadate"}, "search[created_at_from]": {"x"}, "search[created_at_to]": {"y"},
			"search[date_str]": {"x"}, "search[customer_id]": {"zz"}, "search[vendor_id]": {"zz"},
			"search[product_id]": {"zz"}, "search[order_id]": {"zz"}, "search[net_total]": {"abc"},
			"search[search_text]": {"(["},
		},
	}
	bodies := map[string]string{
		"empty object": `{}`,
		"json null":    `null`,
		"broken json":  `{x`,
		"no body":      ``,
		"store only":   fmt.Sprintf(`{"store_id":%q}`, sid),
		"wrong types":  `{"store_id":5,"products":"x","vat_percent":"a","payments_input":{},"amount":"1"}`,
		"empty lists":  fmt.Sprintf(`{"store_id":%q,"products":[],"payments_input":[],"payments":[],"vat_percent":15}`, sid),
	}

	var failures []string
	checked := 0
	for _, r := range routesFromMain(t) {
		if sweepSkip.MatchString(r.path) {
			continue
		}
		path := pathVar.ReplaceAllString(r.path, fakeID)
		try := func(label, query, body string) {
			p := path
			if query != "" {
				p += "?" + query
			}
			start := time.Now()
			code, err := rawCall(r.method, p, tok, body)
			checked++
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s %s [%s]: %s after %s", r.method, r.path, label, describeFailure(err), time.Since(start).Round(time.Millisecond)))
				return
			}
			if code == http.StatusUnauthorized && strings.Contains(label, "valid") {
				// The sweep's own session must survive every call.
				failures = append(failures, fmt.Sprintf("%s %s [%s]: HTTP 401 for a signed-in user", r.method, r.path, label))
			}
		}
		if r.method == "GET" {
			for label, q := range queries {
				try(label, q.Encode(), "")
			}
			continue
		}
		for label, b := range bodies {
			try(label, url.Values{"search[store_id]": {sid}}.Encode(), b)
		}
		try("zero store id", "search[store_id]=000000000000000000000000", `{}`)
		try("no store id", "", `{}`)
	}
	t.Logf("checked %d requests", checked)
	if len(failures) > 0 {
		t.Fatalf("%d requests failed:\n%s", len(failures), strings.Join(failures, "\n"))
	}

	// The session used for the sweep must still work afterwards.
	if code, res := call(t, "GET", "/v1/me", tok, nil); code != http.StatusOK || !res.Status {
		t.Fatalf("/v1/me after the sweep: HTTP %d %v", code, res.Errors)
	}
}

// A store id of all zeros used to panic ParseStore (err.Error() on a nil
// error) on ~170 endpoints. Pin the exact answer for a few of them.
func TestRobustness_ZeroStoreIDIsAValidationError(t *testing.T) {
	tok := authToken(t)
	for _, p := range []string{"/v1/product", "/v1/order", "/v1/customer", "/v1/purchase", "/v1/ledger"} {
		code, res := call(t, "GET", p+"?search[store_id]=000000000000000000000000", tok, nil)
		if res.Status {
			t.Fatalf("GET %s with a zero store id: HTTP %d status=true, want an error", p, code)
		}
		if _, ok := res.Errors["store_id"]; !ok {
			t.Fatalf("GET %s with a zero store id: want a store_id error, got HTTP %d %v", p, code, res.Errors)
		}
	}
}

// A JSON null body used to reach handlers as a nil document.
func TestRobustness_NullBodyIsRejected(t *testing.T) {
	tok := authToken(t)
	sid := testStore(t)
	for _, p := range []string{"/v1/order", "/v1/purchase", "/v1/customer", "/v1/product", "/v1/sales-return", "/v1/expense"} {
		code, res := call(t, "POST", p+"?"+storeQuery(sid), tok, "null")
		if code != http.StatusBadRequest || res.Status {
			t.Fatalf("POST %s with a null body: HTTP %d status=%v, want 400", p, code, res.Status)
		}
		if _, ok := res.Errors["input"]; !ok {
			t.Fatalf("POST %s with a null body: want an input error, got %v", p, res.Errors)
		}
	}
}

// The purchase-bill reader called a paid AI API without checking the token.
func TestRobustness_PurchaseBillParserNeedsAuth(t *testing.T) {
	code, err := rawCall("POST", "/v1/purchase/upload/image", "", "")
	if err != nil {
		t.Fatalf("POST /v1/purchase/upload/image: %s", describeFailure(err))
	}
	if code != http.StatusUnauthorized {
		t.Fatalf("POST /v1/purchase/upload/image without a token: HTTP %d, want 401", code)
	}
	code, err = rawCall("POST", "/v1/purchase/upload/image", authToken(t), "")
	if err != nil {
		t.Fatalf("POST /v1/purchase/upload/image: %s", describeFailure(err))
	}
	if code != http.StatusBadRequest {
		t.Fatalf("POST /v1/purchase/upload/image without an image: HTTP %d, want 400", code)
	}
}
