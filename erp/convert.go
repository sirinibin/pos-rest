// Package erp implements the StartERP adapter API (/v1/erp/...).
//
// It translates the StartERP REST contract (camelCase JSON documents, see
// docs/starterp/ADAPTER_MAPPING.md) onto the EXISTING StartPOS data model.
// Reads come straight from the legacy collections; writes are dispatched
// in-process to the existing v1 handlers so that validation, counters, stock,
// ledger postings, stats and ZATCA hooks behave exactly as for the old app.
// Everything this package persists on legacy documents lives in ONE additive
// sub-document named "erp" (see envelope.go); no legacy field is renamed,
// removed or retyped.
package erp

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// M is the generic document type used for both legacy (bson) and contract
// (JSON) shapes.
type M = map[string]interface{}

var riyadh = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Riyadh")
	if err != nil {
		return time.FixedZone("AST", 3*3600)
	}
	return loc
}()

const (
	layoutDT  = "2006-01-02T15:04"
	layoutDay = "2006-01-02"
)

// nowFn is replaceable in tests.
var nowFn = time.Now

// norm converts driver-specific container types (primitive.D, primitive.A,
// bson.M) into plain map[string]interface{} / []interface{} recursively so
// mappers never have to care how the driver decoded a legacy document.
func norm(v interface{}) interface{} {
	switch t := v.(type) {
	case primitive.D:
		m := make(M, len(t))
		for _, e := range t {
			m[e.Key] = norm(e.Value)
		}
		return m
	case primitive.M:
		m := make(M, len(t))
		for k, e := range t {
			m[k] = norm(e)
		}
		return m
	case map[string]interface{}:
		m := make(M, len(t))
		for k, e := range t {
			m[k] = norm(e)
		}
		return m
	case primitive.A:
		a := make([]interface{}, len(t))
		for i, e := range t {
			a[i] = norm(e)
		}
		return a
	case []interface{}:
		a := make([]interface{}, len(t))
		for i, e := range t {
			a[i] = norm(e)
		}
		return a
	default:
		return v
	}
}

func normDoc(v interface{}) M {
	if m, ok := norm(v).(M); ok {
		return m
	}
	return M{}
}

// get reads a dotted path from a document.
func get(m M, path string) interface{} {
	if m == nil {
		return nil
	}
	cur := interface{}(m)
	for _, p := range strings.Split(path, ".") {
		mm, ok := cur.(M)
		if !ok {
			return nil
		}
		cur, ok = mm[p]
		if !ok {
			return nil
		}
	}
	return cur
}

func sub(m M, key string) M {
	if v, ok := get(m, key).(M); ok {
		return v
	}
	return M{}
}

func arr(v interface{}) []interface{} {
	switch a := v.(type) {
	case []interface{}:
		return a
	case primitive.A:
		return []interface{}(a)
	case []string:
		out := make([]interface{}, len(a))
		for i, s := range a {
			out[i] = s
		}
		return out
	case []M:
		out := make([]interface{}, len(a))
		for i, s := range a {
			out[i] = s
		}
		return out
	}
	return nil
}

var arabicDigits = strings.NewReplacer(
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9", "٫", ".", "٬", "",
)

// num is a lenient number parser (FlexInt, numbers-as-strings, Arabic-Indic
// digits, Decimal128, nil → 0), mirroring the client's D() helper.
func num(v interface{}) float64 {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0
		}
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case uint32:
		return float64(t)
	case uint64:
		return float64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case json.Number:
		f, _ := t.Float64()
		return f
	case primitive.Decimal128:
		f, err := strconv.ParseFloat(t.String(), 64)
		if err != nil {
			return 0
		}
		return f
	case string:
		s := strings.TrimSpace(arabicDigits.Replace(t))
		s = strings.ReplaceAll(s, ",", "")
		if s == "" {
			return 0
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return f
	}
	return 0
}

func intv(v interface{}) int64 { return int64(math.Round(num(v))) }

// isNum reports whether v holds a value num() can interpret meaningfully.
func isNum(v interface{}) bool {
	switch t := v.(type) {
	case float64, float32, int, int32, int64, uint32, uint64, json.Number, primitive.Decimal128:
		return true
	case string:
		s := strings.ReplaceAll(strings.TrimSpace(arabicDigits.Replace(t)), ",", "")
		_, err := strconv.ParseFloat(s, 64)
		return err == nil
	}
	return false
}

func str(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case primitive.ObjectID:
		if t.IsZero() {
			return ""
		}
		return t.Hex()
	case *primitive.ObjectID:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.Hex()
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int, int32, int64:
		return fmt.Sprintf("%d", t)
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	case primitive.DateTime:
		return t.Time().UTC().Format(time.RFC3339)
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("%v", v)
}

func boolv(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(t))
		return b
	case nil:
		return false
	}
	return num(v) != 0
}

// strPtrOrNil returns nil for "" so legacy *ObjectID / *string fields are
// cleared rather than set to empty values.
func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// round2 is the client's se(): round half away from zero to 2 dp using the
// toPrecision(15) trick to avoid binary artefacts.
func round2(x float64) float64 {
	if x == 0 || math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	sign := 1.0
	if x < 0 {
		sign = -1
	}
	a := math.Abs(x) * 100
	p, _ := strconv.ParseFloat(strconv.FormatFloat(a, 'g', 15, 64), 64)
	return sign * math.Floor(p+0.5) / 100
}

func roundN(x float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(x*p) / p
}

// ---- time ----

func toTime(v interface{}) (time.Time, bool) {
	switch t := v.(type) {
	case primitive.DateTime:
		return t.Time(), true
	case time.Time:
		return t, !t.IsZero()
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, !t.IsZero()
	case string:
		if tt, err := parseClientTime(t); err == nil {
			return tt, true
		}
	}
	return time.Time{}, false
}

// fmtDT returns the contract's local datetime string (Asia/Riyadh,
// "YYYY-MM-DDTHH:mm") or "" when v is not a time.
func fmtDT(v interface{}) string {
	if t, ok := toTime(v); ok {
		return t.In(riyadh).Format(layoutDT)
	}
	return ""
}

func fmtDay(v interface{}) string {
	if t, ok := toTime(v); ok {
		return t.In(riyadh).Format(layoutDay)
	}
	return ""
}

var clientLayouts = []string{
	time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z07:00",
}
var localLayouts = []string{
	"2006-01-02T15:04:05.000", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", "2006-01",
}

// parseClientTime parses contract date/datetime strings. Strings without a
// zone are interpreted as Asia/Riyadh wall-clock time.
func parseClientTime(s string) (time.Time, error) {
	s = strings.TrimSpace(arabicDigits.Replace(s))
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	for _, l := range clientLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	for _, l := range localLayouts {
		if t, err := time.ParseInLocation(l, s, riyadh); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q", s)
}

// toLegacyDateStr converts a contract date to the RFC3339 string the legacy
// handlers expect in *_str fields ("2006-01-02T15:04:05Z07:00").
func toLegacyDateStr(s string) (string, error) {
	t, err := parseClientTime(s)
	if err != nil {
		return "", err
	}
	return t.In(riyadh).Format(time.RFC3339), nil
}

// ---- ids ----

func oidOf(v interface{}) (primitive.ObjectID, bool) {
	switch t := v.(type) {
	case primitive.ObjectID:
		return t, !t.IsZero()
	case *primitive.ObjectID:
		if t == nil {
			return primitive.NilObjectID, false
		}
		return *t, !t.IsZero()
	case string:
		id, err := primitive.ObjectIDFromHex(strings.TrimSpace(t))
		if err != nil {
			return primitive.NilObjectID, false
		}
		return id, !id.IsZero()
	}
	return primitive.NilObjectID, false
}

func hexOf(v interface{}) string {
	if id, ok := oidOf(v); ok {
		return id.Hex()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// idOrNil returns the hex string for an ObjectID-ish value or nil.
func idOrNil(v interface{}) interface{} {
	if h := hexOf(v); h != "" {
		return h
	}
	return nil
}

func strs(v interface{}) []string {
	out := []string{}
	for _, e := range arr(v) {
		if s := str(e); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func ids(v interface{}) []string {
	out := []string{}
	for _, e := range arr(v) {
		if h := hexOf(e); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// ---- validation helpers (mirror the UI rules) ----

var (
	reVAT        = regexp.MustCompile(`^3\d{13}3$`)
	reCR         = regexp.MustCompile(`^\d{10}$`)
	re4          = regexp.MustCompile(`^\d{4}$`)
	re5          = regexp.MustCompile(`^\d{5}$`)
	reShort      = regexp.MustCompile(`^[A-Z]{2,5}$`)
	reSaudiMob   = regexp.MustCompile(`^(05\d{8}|\+?9665\d{8})$`)
	reSaudiPhone = regexp.MustCompile(`^(0\d{8,9}|\+?966\d{8,9})$`)
	reEmail      = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	reArabic     = regexp.MustCompile(`[\x{0600}-\x{06FF}]`)
	reOTP        = regexp.MustCompile(`^\d{5,6}$`)
)

func cleanPhone(s string) string {
	return strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(arabicDigits.Replace(strings.TrimSpace(s)))
}

// ValidVAT reports whether s is a Saudi VAT number (15 digits, 3…3).
func ValidVAT(s string) bool { return reVAT.MatchString(strings.TrimSpace(s)) }

// ValidCR reports whether s is a 10-digit commercial registration number.
func ValidCR(s string) bool { return reCR.MatchString(strings.TrimSpace(s)) }

// ValidSaudiMobile accepts 05XXXXXXXX or +9665XXXXXXXX (spaces/dashes ignored).
func ValidSaudiMobile(s string) bool { return reSaudiMob.MatchString(cleanPhone(s)) }

// ValidSaudiPhone accepts landlines and mobiles (store phone rule).
func ValidSaudiPhone(s string) bool { return reSaudiPhone.MatchString(cleanPhone(s)) }

func validEmail(s string) bool { return reEmail.MatchString(strings.TrimSpace(s)) }

func hasArabic(s string) bool { return reArabic.MatchString(s) }

// toJSONMap round-trips any value through JSON into a generic map.
func toJSONMap(v interface{}) M {
	b, err := json.Marshal(v)
	if err != nil {
		return M{}
	}
	var m M
	_ = json.Unmarshal(b, &m)
	return m
}

func cloneM(m M) M {
	if m == nil {
		return M{}
	}
	b, _ := json.Marshal(m)
	var out M
	_ = json.Unmarshal(b, &out)
	if out == nil {
		out = M{}
	}
	return out
}

// bsonToM decodes raw bson into a normalized M.
func bsonToM(raw bson.Raw) M {
	var m bson.M
	if err := bson.Unmarshal(raw, &m); err != nil {
		return M{}
	}
	return normDoc(m)
}
