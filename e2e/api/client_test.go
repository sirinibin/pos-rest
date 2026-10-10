//go:build e2e

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// M is a JSON object.
type M = map[string]interface{}

// Resp is one API answer.
type Resp struct {
	Code   int
	Body   M
	List   []interface{} // body when the answer is a JSON array
	Raw    string
	Header http.Header
}

// ErrCode is error.code of a contract error envelope.
func (r Resp) ErrCode() string { return S(Get(r.Body, "error.code")) }

// ErrField is error.fields[f] of a contract error envelope.
func (r Resp) ErrField(f string) string {
	if fs, ok := Get(r.Body, "error.fields").(M); ok {
		return S(fs[f])
	}
	return ""
}

// Data is the data[] array of a list answer.
func (r Resp) Data() []M { return Objs(r.Body["data"]) }

// ID is the id of a created/read record.
func (r Resp) ID() string { return S(r.Body["id"]) }

func (r Resp) String() string {
	raw := r.Raw
	if len(raw) > 1500 {
		raw = raw[:1500] + "…"
	}
	return fmt.Sprintf("HTTP %d %s", r.Code, raw)
}

var httpClient = &http.Client{Timeout: 120 * time.Second}

// Call sends one request to the adapter API (path relative to /v1/erp).
// headers are name/value pairs.
func Call(t testing.TB, method, path, token string, body interface{}, headers ...string) Resp {
	t.Helper()
	return CallURL(t, method, baseURL+"/v1/erp"+path, token, body, headers...)
}

// CallURL is Call with a full URL (legacy /v1 routes, raw paths).
func CallURL(t testing.TB, method, url, token string, body interface{}, headers ...string) Resp {
	t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		rd = bytes.NewReader(j)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: no answer (server crash or hang?): %v", method, url, err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	recordStatus(method, req.URL.Path, res.StatusCode)
	out := Resp{Code: res.StatusCode, Raw: string(b), Header: res.Header}
	if s := strings.TrimSpace(out.Raw); s != "" {
		var v interface{}
		if json.Unmarshal([]byte(s), &v) == nil {
			switch x := v.(type) {
			case M:
				out.Body = x
			case []interface{}:
				out.List = x
			}
		}
	}
	return out
}

// Must fails the test unless the answer has the wanted status.
func Must(t testing.TB, r Resp, want int, what string) Resp {
	t.Helper()
	if r.Code != want {
		t.Fatalf("%s: want HTTP %d, got %s", what, want, r)
	}
	return r
}

// Create POSTs a record and returns it (fails unless 201).
func Create(t testing.TB, tok, path string, body M) M {
	t.Helper()
	return Must(t, Call(t, "POST", "/"+path, tok, body), 201, "create "+path).Body
}

// Read GETs a record (fails unless 200).
func Read(t testing.TB, tok, path, id string) M {
	t.Helper()
	return Must(t, Call(t, "GET", "/"+path+"/"+id, tok, nil), 200, "read "+path+"/"+id).Body
}

// Patch PATCHes a record with If-Match from a fresh read (fails unless 200).
func Patch(t testing.TB, tok, path, id string, body M) M {
	t.Helper()
	return Must(t, PatchResp(t, tok, path, id, body), 200, "patch "+path+"/"+id).Body
}

// PatchResp is Patch without the status check.
func PatchResp(t testing.TB, tok, path, id string, body M) Resp {
	t.Helper()
	cur := Read(t, tok, path, id)
	return Call(t, "PATCH", "/"+path+"/"+id, tok, body, "If-Match", Num(cur["version"]).String(), "X-Change-Reason", "e2e")
}

// List GETs a list (query without the leading "?") and returns data[].
func List(t testing.TB, tok, path, query string) []M {
	t.Helper()
	return Must(t, Call(t, "GET", "/"+path+"?"+query, tok, nil), 200, "list "+path).Data()
}

// ---------- JSON helpers ----------

// Get reads a dotted path ("a.b.0.c") from nested JSON.
func Get(v interface{}, path string) interface{} {
	cur := v
	for _, p := range strings.Split(path, ".") {
		switch x := cur.(type) {
		case M:
			cur = x[p]
		case []interface{}:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			cur = x[i]
		default:
			return nil
		}
	}
	return cur
}

// S is v as a string ("" for nil).
func S(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// Number is a JSON number with helpers.
type Number float64

func (n Number) String() string { return strconv.FormatFloat(float64(n), 'f', -1, 64) }

// Num is v as a number (0 for anything else).
func Num(v interface{}) Number {
	switch x := v.(type) {
	case float64:
		return Number(x)
	case int:
		return Number(x)
	case int64:
		return Number(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return Number(f)
	}
	return 0
}

// F is Get(v, path) as a float.
func F(v interface{}, path string) float64 { return float64(Num(Get(v, path))) }

// Objs is v as a list of objects.
func Objs(v interface{}) []M {
	a, _ := v.([]interface{})
	out := make([]M, 0, len(a))
	for _, e := range a {
		if m, ok := e.(M); ok {
			out = append(out, m)
		}
	}
	return out
}

// Cents rounds money to integer cents (half away from zero) for exact compares.
func Cents(x float64) int64 { return int64(math.Round(x * 100)) }

// EqMoney fails unless got == want to the cent.
func EqMoney(t testing.TB, what string, got, want float64) {
	t.Helper()
	if Cents(got) != Cents(want) {
		t.Errorf("%s: got %.2f, want %.2f", what, got, want)
	}
}

// Eventually polls cond for up to 20 s (the server finishes some work in the
// background, e.g. stock and ledger updates after a save).
func Eventually(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var seq int64

// Uniq is a run-unique suffix.
func Uniq() string {
	return fmt.Sprintf("%s%03d", time.Now().Format("150405"), atomic.AddInt64(&seq, 1)%1000)
}

// Digits returns n run-unique digits.
func Digits(n int) string {
	s := fmt.Sprintf("%d%d", time.Now().UnixNano(), atomic.AddInt64(&seq, 1))
	return s[len(s)-n:]
}

// KnownBug marks a check for a bug that is reported but not fixed yet
// (notes/backend-bugs-found-2026-10-10.md). While stillBroken is true the test
// logs it and passes; once the bug is fixed the test fails and asks for the
// marker to become a normal assertion.
func KnownBug(t testing.TB, id, what string, stillBroken bool) {
	t.Helper()
	if stillBroken {
		t.Logf("KNOWN BUG %s still open: %s", id, what)
		knownBugsSeen.Add(1)
		return
	}
	t.Errorf("KNOWN BUG %s looks fixed (%s): replace KnownBug with a normal assertion", id, what)
}

var knownBugsSeen atomic.Int64
