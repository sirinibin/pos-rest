package erp

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/gorilla/mux"
)

func urlUnescape(s string) (string, error) { return url.QueryUnescape(s) }

// debugV1 (ERP_DEBUG=1) logs every in-process legacy call (local debugging).
var debugV1 = os.Getenv("ERP_DEBUG") == "1"

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// v1Result is the decoded legacy response envelope (models.Response).
type v1Result struct {
	HTTP   int
	Status bool              `json:"status"`
	Errors map[string]string `json:"errors"`
	Result json.RawMessage   `json:"result"`
}

// callV1 invokes an EXISTING v1 handler in-process with the caller's own
// access token, so the legacy code path (auth, validation, counters, stock,
// accounting, ZATCA hooks) runs exactly as for a request from the old app.
func callV1(c *Ctx, h http.HandlerFunc, method, path string, vars map[string]string, storeHex string, body interface{}) (*v1Result, error) {
	q := url.Values{}
	if storeHex != "" {
		q.Set("search[store_id]", storeHex)
	}
	return callV1Q(c, h, method, path, vars, q, body)
}

// callV1Q is callV1 with an explicit query string (some legacy handlers read
// ?store_id= instead of ?search[store_id]=).
func callV1Q(c *Ctx, h http.HandlerFunc, method, path string, vars map[string]string, q url.Values, body interface{}) (*v1Result, error) {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, errInternal("encode legacy payload: " + err.Error())
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	target := path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req := httptest.NewRequest(method, target, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if tz := c.R.Header.Get("X-Timezone-Offset"); tz != "" {
		req.Header.Set("X-Timezone-Offset", tz)
	}
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	if debugV1 {
		b, _ := json.Marshal(body)
		log.Printf("[erp→v1] %s %s payload=%s → %d %s", method, target, truncate(string(b), 4000), rec.Code, truncate(rec.Body.String(), 4000))
	}
	res := &v1Result{HTTP: rec.Code}
	_ = json.Unmarshal(rec.Body.Bytes(), res)
	if !res.Status && len(res.Errors) == 0 && len(res.Result) == 0 {
		// handlers that do not use models.Response (RFQ module): raw entity or {"error": "..."}
		var raw M
		if json.Unmarshal(rec.Body.Bytes(), &raw) == nil {
			if e, ok := raw["error"].(string); ok {
				res.Errors = map[string]string{"error": e}
			} else if _, ok := raw["id"]; ok && rec.Code < 400 {
				res.Status = true
				res.Result = json.RawMessage(rec.Body.Bytes())
			} else if rec.Code < 400 && (raw["result"] != nil || raw["success"] == true) {
				res.Status = true
			}
		}
	}
	if res.HTTP == 0 {
		res.HTTP = http.StatusOK
	}
	return res, nil
}

// ok reports legacy success. Some legacy handlers (e.g. CreateProduct) never
// set status=true, so "no errors + a result" also counts as success.
func (r *v1Result) ok() bool {
	if r.HTTP >= 400 {
		return false
	}
	if r.Status {
		return true
	}
	res := strings.TrimSpace(string(r.Result))
	return len(r.Errors) == 0 && res != "" && res != "null"
}

// resultID extracts result.id (or result._id) from a legacy response.
func (r *v1Result) resultID() string {
	var m M
	if err := json.Unmarshal(r.Result, &m); err != nil {
		return ""
	}
	if s := str(m["id"]); s != "" {
		return s
	}
	return str(m["_id"])
}

var reIndexed = regexp.MustCompile(`^(.*?)_(\d+)$`)

// legacyErr translates a failed legacy response into the contract error
// envelope. Validation errors become 400 with error.fields translated to
// contract field names; authorization problems keep 401/403.
func legacyErr(r *v1Result, fieldMap map[string]string, lineMap map[string]string, lineKey string) *APIError {
	fields := map[string]string{}
	keys := make([]string, 0, len(r.Errors))
	for k := range r.Errors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	infra := false
	for _, k := range keys {
		v := r.Errors[k]
		switch k {
		case "insert", "update", "view", "find", "delete", "db":
			if r.HTTP >= 500 {
				infra = true
			}
		}
		ck := k
		if mapped, ok := fieldMap[k]; ok {
			ck = mapped
		} else if m := reIndexed.FindStringSubmatch(k); m != nil && paymentErrBase(m[1]) != "" {
			ck = "payments." + m[2] + "." + paymentErrBase(m[1])
		} else if m := reIndexed.FindStringSubmatch(k); m != nil && lineKey != "" {
			base := m[1]
			if lm, ok := lineMap[base]; ok {
				ck = lineKey + "." + m[2] + "." + lm
			} else {
				ck = lineKey + "." + m[2] + "." + base
			}
		}
		if prev, ok := fields[ck]; ok && prev != v {
			fields[ck] = prev + "; " + v
		} else {
			fields[ck] = v
		}
	}
	msg := firstLegacyMsg(r.Errors)
	switch {
	case r.HTTP == http.StatusUnauthorized:
		if _, ok := r.Errors["access_token"]; ok {
			return errUnauthorized(msg)
		}
		return errForbidden(msg)
	case r.HTTP == http.StatusForbidden:
		e := errForbidden(msg)
		e.Fields = fields
		return e
	case infra:
		return errInternal(msg)
	}
	e := errBadRequest(msg, fields)
	return e
}

func firstLegacyMsg(errs map[string]string) string {
	keys := make([]string, 0, len(errs))
	for k := range errs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v := strings.TrimSpace(errs[k]); v != "" {
			return v
		}
	}
	return "The server rejected the request."
}

// paymentErrBase maps legacy indexed payment error keys
// (payment_amount_0, customer_receivable_payment_method_1, …).
func paymentErrBase(base string) string {
	for _, p := range []string{"customer_receivable_payment_", "customer_payable_payment_", "payment_"} {
		if strings.HasPrefix(base, p) {
			f := strings.TrimPrefix(base, p)
			if f == "invoice" {
				return "orderId"
			}
			return f
		}
	}
	return ""
}
