package erp

// list_stats.go — the stat tiles above a list (Sales, Returns, Purchases, Expenses …).
//
//   GET /v1/erp/<resource>/stats?storeId=X&from=YYYY-MM-DD&to=YYYY-MM-DD
//       &sum=net,vat,paid,balance&q=…&keys=code,customerName&f.party=…&includeDeleted=1
//
// The web app downloads only the last year of documents (VITE_API_WINDOW_DAYS), so a
// list whose period reaches further back ("All time", an old custom range) cannot add
// its tiles up in the browser: it showed one year's total as "All time".  This
// endpoint adds them up on the server over every record of the period, with the same
// rules the browser uses on the rows it has (ListPage search, date range and filters;
// totals.js computeTotals for document figures), so the tiles agree with the rows.
//
// Sums: net, vat, paid, balance, taxable, profit and cost are a document's computed
// totals (ComputeTotals), retailProfit / wholesaleProfit a purchase's (R9), one is 1
// (a count),
// nonEmpty:<field> counts records with that field set, any other name sums that
// contract field (dotted path); "<sum>|<field>=<value>" adds only matching records.
// groupBy=party (customer or vendor) or a field also adds up per value ("groups").
// page=N (&limit, at most 500; &sort=[-]field, default -date) also returns "ids": that
// page of the matches in sort order, for a list screen that pages on the server with
// the same filters as its tiles (it then reads those records with ?ids=); with
// lines=payments it returns that page's lines themselves as "rows" instead.
// qstatus (a quotation's status, expired once past its validity), expiry and late (a
// pending delivery note past its estimated delivery) work as fields too
// (list_stats_derived.go).
// lines=payments adds up the payment lines instead (the Payments lists): each line is
// its record with date (the payment's, else the record's), amount, method and
// description taken from the payment.
// Filters (f.<key>): party, pstatus, zatca, paymentMethod (any payment), overdue as in
// DocumentList; nonEmpty:<field> = y / n (field set or not); any other key compares
// the contract field of that name, as ListPage does by default.

import (
	"context"
	"fmt"
	"net/http"
	sortpkg "sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// default ListPage search keys
var listSearchKeys = []string{"code", "nameEn", "nameAr", "customerName", "customerNameAr", "vendorName",
	"vendorNameAr", "phone", "vatNo"}

var totalMeasures = map[string]bool{"net": true, "vat": true, "paid": true, "balance": true, "taxable": true,
	"profit": true, "cost": true, "retailProfit": true, "wholesaleProfit": true, "one": true}

// StatsQuery is a parsed /stats request.
type StatsQuery struct {
	From, To       string // inclusive store days ("" = open)
	DateKey        string
	Search         string
	SearchKeys     []string
	Sums           []string
	Filters        map[string]string
	IncludeDeleted bool
	Today          string // store day, for the overdue filter
	Now            string // store wall-clock time (YYYY-MM-DDTHH:MM), for qstatus / late
	GroupBy        string // "" | "party" | a contract field: also add up per value
	Lines          string // "" | "payments": add up the records' payment lines instead
	Page, Limit    int    // page > 0: also return the ids of that page of matches (in Sort order)
	Sort           string // "<field>" or "-<field>" (a computed total, pstatus, zatca or a contract field)
	Running        string // with page: also each page record's running total of this field ("running")
}

// StatsResult is the response body (besides storeId/timezone).
type StatsResult struct {
	Count int      `json:"count"`
	IDs   []string `json:"ids,omitempty"`
	Rows  []M      `json:"rows,omitempty"` // lines=payments pages: the lines themselves
	// Running: per page record, the running total of q.Running over every record of
	// the list (not deleted, any filter) in date then code order (ledger.js W4)
	Running map[string]float64      `json:"running,omitempty"`
	Sums    map[string]float64      `json:"sums"`
	Groups  map[string]*StatsResult `json:"groups,omitempty"`
}

func parseStatsQuery(r *http.Request) (StatsQuery, error) {
	v := r.URL.Query()
	q := StatsQuery{DateKey: "date", Filters: map[string]string{}, SearchKeys: listSearchKeys}
	for _, k := range []string{"from", "to"} {
		s := strings.TrimSpace(v.Get(k))
		if s == "" {
			continue
		}
		if _, err := time.Parse(layoutDay, s); err != nil {
			return q, errBadRequest("Invalid "+k+".", map[string]string{k: "must be YYYY-MM-DD"})
		}
		if k == "from" {
			q.From = s
		} else {
			q.To = s
		}
	}
	if q.From != "" && q.To != "" && q.From > q.To {
		return q, errBadRequest("from is after to.", map[string]string{"from": "must not be after to"})
	}
	if s := strings.TrimSpace(v.Get("dateKey")); s != "" {
		q.DateKey = s
	}
	q.Search = strings.ToLower(strings.TrimSpace(v.Get("q")))
	if s := strings.TrimSpace(v.Get("keys")); s != "" {
		q.SearchKeys = splitList(s)
	}
	q.Sums = splitList(v.Get("sum"))
	if len(q.Sums) > 20 {
		return q, errBadRequest("Too many sums.", map[string]string{"sum": "at most 20"})
	}
	for k, vals := range v {
		if strings.HasPrefix(k, "f.") && len(k) > 2 && len(vals) > 0 && vals[0] != "" {
			q.Filters[k[2:]] = vals[0]
		}
	}
	q.GroupBy = strings.TrimSpace(v.Get("groupBy"))
	switch q.Lines = strings.TrimSpace(v.Get("lines")); q.Lines {
	case "", "payments":
	default:
		return q, errBadRequest("Unknown lines.", map[string]string{"lines": "payments or none"})
	}
	inc := v.Get("includeDeleted")
	q.IncludeDeleted = inc == "1" || strings.EqualFold(inc, "true")
	if s := strings.TrimSpace(v.Get("page")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return q, errBadRequest("Invalid page.", map[string]string{"page": "must be 1 or more"})
		}
		q.Page, q.Limit = n, 25
		if s := strings.TrimSpace(v.Get("limit")); s != "" {
			l, err := strconv.Atoi(s)
			if err != nil || l < 1 || l > maxStatsPage {
				return q, errBadRequest("Invalid limit.", map[string]string{"limit": "1 to " + strconv.Itoa(maxStatsPage)})
			}
			q.Limit = l
		}
		q.Sort = strings.TrimSpace(v.Get("sort"))
		if q.Sort == "" || q.Sort == "-" {
			q.Sort = "-" + q.DateKey
		}
		q.Running = strings.TrimSpace(v.Get("running"))
	}
	return q, nil
}

// withResourceDateKey: without ?dateKey a list's period is its own date field
// (salaries: paymentDate), as the list's ?from/?to are; "date" read a field the
// salary records do not have, so a period matched none of them.
func withResourceDateKey(q StatsQuery, res *Resource, asked string) StatsQuery {
	if strings.TrimSpace(asked) != "" || res == nil || res.DateField == "" || res.DateField == q.DateKey {
		return q
	}
	if q.Sort == "-"+q.DateKey {
		q.Sort = "-" + res.DateField
	}
	q.DateKey = res.DateField
	return q
}

// maxStatsPage: the most ids one /stats page returns.
const maxStatsPage = 500

func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// field reads a dotted contract path.
func field(rec M, path string) interface{} {
	var cur interface{} = rec
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(M)
		if !ok {
			return nil
		}
		cur = m[k]
	}
	return cur
}

// jsString is JS String(v ?? "") for the values a contract record holds.
func jsString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64, int, int32, int64:
		return strconv.FormatFloat(num(t), 'f', -1, 64)
	}
	return str(v)
}

// statRow is what the totals read of one record: its computed totals, the few
// derived values the named filters test, and the fields the request names
// (search keys, date, other filters and field sums).  Items and payments are not
// kept, so a store's whole list stays small in the cache.
type statRow struct {
	ID      string
	Deleted bool
	T       FeedTotals
	Retail  float64 // purchases: conversions.js R9 retail / wholesale profit
	Whole   float64
	Party   string
	Zatca   string
	Methods []string
	Vals    map[string]interface{} // field → string or float64 (absent = nil)
	Lines   []statRow              // lines=payments: one per payment
}

// linesField marks the fields of a lines=payments query (the rows keep their lines).
const linesField = "~payments"

func statRowOf(rec M, fields map[string]bool) statRow {
	r := statRow{ID: str(rec["id"]), Deleted: boolv(rec["deleted"]), T: ComputeTotals(rec), Vals: map[string]interface{}{}}
	r.Party = str(rec["customerId"])
	if r.Party == "" {
		r.Party = str(rec["vendorId"])
	}
	if z, ok := rec["zatca"].(M); ok {
		r.Zatca = str(z["status"])
	}
	if r.Zatca == "" {
		r.Zatca = "not_reported"
	}
	r.Retail, r.Whole = retailWholesaleProfit(rec)
	for _, p := range arr(rec["payments"]) {
		if pm, ok := p.(M); ok {
			r.Methods = append(r.Methods, str(pm["method"]))
		}
	}
	for f := range fields {
		if f == linesField {
			continue
		}
		if f == "expiry" {
			r.Vals[f] = quotationExpiry(jsString(rec["date"]), num(rec["validityDays"]))
			continue
		}
		if strings.HasPrefix(f, "nonEmpty:") {
			if nonEmpty(field(rec, f[len("nonEmpty:"):])) {
				r.Vals[f] = 1.0
			} else {
				r.Vals[f] = 0.0
			}
			continue
		}
		switch v := field(rec, f).(type) {
		case nil:
		case float64, int, int32, int64:
			r.Vals[f] = num(v)
		default:
			r.Vals[f] = jsString(v)
		}
	}
	if fields[linesField] {
		for _, p := range arr(rec["payments"]) {
			pm, _ := p.(M)
			l := r
			l.Lines = nil
			l.Vals = make(map[string]interface{}, len(r.Vals)+4)
			for k, v := range r.Vals {
				l.Vals[k] = v
			}
			if d := str(pm["date"]); d != "" {
				l.Vals["date"] = d
			} else {
				l.Vals["date"] = jsString(rec["date"])
			}
			l.Vals["amount"] = num(pm["amount"])
			l.Vals["method"] = str(pm["method"])
			l.Vals["description"] = str(pm["description"])
			l.Vals["paymentId"] = str(pm["id"])
			r.Lines = append(r.Lines, l)
		}
	}
	return r
}

// retailWholesaleProfit is conversions.js R9 for a purchase: what its lines would
// earn sold at their retail and wholesale prices.  Purchase lines always carry both
// prices (legacy retail_unit_price / wholesale_unit_price); a line without them
// counts as sold at cost.
func retailWholesaleProfit(rec M) (float64, float64) {
	var r, w float64
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		cost := num(im["unitPrice"]) - num(im["unitDiscount"])
		rp, wp := cost, cost
		if v, ok := im["retailPrice"]; ok && v != nil {
			rp = num(v)
		}
		if v, ok := im["wholesalePrice"]; ok && v != nil {
			wp = num(v)
		}
		q := num(im["qty"])
		r += q * (rp - cost)
		w += q * (wp - cost)
	}
	return round2(r), round2(w)
}

// statFields: the record fields q reads besides the derived ones.
func statFields(q StatsQuery) map[string]bool {
	f := map[string]bool{q.DateKey: true, "date": true}
	for _, k := range q.SearchKeys {
		f[k] = true
	}
	for k := range q.Filters {
		if !namedFilters[k] {
			f[k] = true
		}
	}
	if q.GroupBy != "" && q.GroupBy != "party" {
		f[q.GroupBy] = true
	}
	if q.Lines == "payments" {
		f[linesField] = true
	}
	if q.Running != "" {
		f[q.Running], f["code"] = true, true
	}
	if k := strings.TrimPrefix(q.Sort, "-"); k != "" && !totalMeasures[k] && k != "pstatus" && k != "zatca" {
		f[k] = true
	}
	for _, k := range derivedOf(q) {
		delete(f, k)
		for _, need := range derivedNeeds[k] {
			f[need] = true
		}
	}
	for _, s := range q.Sums {
		base, cf, _ := splitMeasure(s)
		if cf != "" {
			if _, ok := derivedNeeds[cf]; !ok {
				f[cf] = true
			}
		}
		if !totalMeasures[base] {
			f[base] = true
		}
	}
	return f
}

// splitMeasure reads a sum name: "<measure>" or "<measure>|<field>=<value>" (only
// records whose field is that value).  A measure is a computed total, a field, or
// "nonEmpty:<field>" (1 for each record whose field is set: a non-empty list or
// text, true, a non-zero number).
func splitMeasure(s string) (base, condField, condVal string) {
	base = s
	if i := strings.Index(s, "|"); i >= 0 {
		base = s[:i]
		if kv := s[i+1:]; strings.Contains(kv, "=") {
			j := strings.Index(kv, "=")
			condField, condVal = kv[:j], kv[j+1:]
		}
	}
	return
}

func nonEmpty(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return false
	case []interface{}:
		return len(t) > 0
	case string:
		return t != ""
	case bool:
		return t
	case M:
		return len(t) > 0
	}
	return num(v) != 0
}

var namedFilters = map[string]bool{"party": true, "pstatus": true, "zatca": true, "paymentMethod": true, "overdue": true}

// statsMatch applies ListPage's search, date range and filters to one record.
func statsMatch(q StatsQuery, r statRow, credits map[string]float64) bool {
	if !q.IncludeDeleted && r.Deleted {
		return false
	}
	if q.Search != "" {
		hit := false
		for _, k := range q.SearchKeys {
			if strings.Contains(strings.ToLower(jsString(r.Vals[k])), q.Search) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if q.From != "" || q.To != "" {
		d := first10(jsString(r.Vals[q.DateKey]))
		if q.From != "" && d < q.From {
			return false
		}
		if q.To != "" && d > q.To {
			return false
		}
	}
	for k, v := range q.Filters {
		if !statsFilter(k, v, r, credits, q.Today) {
			return false
		}
	}
	return true
}

func statsFilter(key, val string, r statRow, credits map[string]float64, today string) bool {
	switch key {
	case "party":
		// several picked customers / vendors: any of them
		return inCommaList(val, r.Party)
	case "pstatus":
		return r.T.Status == val
	case "zatca":
		return r.Zatca == val
	case "paymentMethod":
		for _, m := range r.Methods {
			if m == val {
				return true
			}
		}
		return false
	case "overdue":
		days, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		bal := round2(r.T.Balance - credits[r.ID])
		if bal < 0 {
			bal = 0
		}
		return isOverdueDay(first10(jsString(r.Vals["date"])), bal, days, today)
	}
	if strings.HasPrefix(key, "nonEmpty:") {
		return (val == "y") == (num(r.Vals[key]) == 1)
	}
	if strings.HasSuffix(key, "Id") {
		return inCommaList(val, jsString(r.Vals[key]))
	}
	return jsString(r.Vals[key]) == val
}

// inCommaList: val is one id or several separated by commas (a list filter's
// search-and-select picks); have matches any of them.
func inCommaList(val, have string) bool {
	if have == "" {
		return false
	}
	for _, v := range strings.Split(val, ",") {
		if strings.TrimSpace(v) == have {
			return true
		}
	}
	return false
}

// isOverdueDay is finance.js isOverdue.
func isOverdueDay(d string, balance, days float64, today string) bool {
	if balance <= 0.004 || d == "" || !(d < today) {
		return false
	}
	t1, err1 := time.Parse(layoutDay, today)
	t0, err0 := time.Parse(layoutDay, d)
	if err0 != nil || err1 != nil {
		return false
	}
	return t1.Sub(t0).Hours()/24 > days
}

// addStats adds one matching record to the result.
func addStats(res *StatsResult, sums []string, r statRow) {
	res.Count++
	for _, s := range sums {
		base, cf, cv := splitMeasure(s)
		if cf != "" && jsString(r.Vals[cf]) != cv {
			continue
		}
		var v float64
		switch base {
		case "net":
			v = r.T.Net
		case "vat":
			v = r.T.Vat
		case "paid":
			v = r.T.Paid
		case "balance":
			v = r.T.Balance
		case "taxable":
			v = r.T.Taxable
		case "profit":
			v = r.T.Profit
		case "cost":
			v = r.T.Cost
		case "retailProfit":
			v = r.Retail
		case "wholesaleProfit":
			v = r.Whole
		case "one":
			v = 1
		default:
			v = num(r.Vals[base])
		}
		res.Sums[s] += v
	}
}

// ---- cache ----
//
// Reading and converting every record of a long-running store takes seconds, so the
// rows are kept per store, list and access scope and reused until the list changes:
// a write through the app (the dashboard event generation) or any other change to
// its collection (count, newest _id, latest updated_at).

var (
	// StatsCacheIdle: rows not read for this long are dropped.
	StatsCacheIdle = 30 * time.Minute
	// StatsCacheMax: at most this many lists are kept.
	StatsCacheMax = 40

	statsMu    sync.Mutex
	statsCache = map[string]*statsEntry{}
	statsBuilt int64 // builds (tests)
)

type statsEntry struct {
	mu     sync.Mutex
	fp     string
	gen    int64
	fields map[string]bool
	rows   []statRow
	used   time.Time
}

func statsEntryFor(key string) *statsEntry {
	statsMu.Lock()
	defer statsMu.Unlock()
	now := time.Now()
	for k, e := range statsCache {
		if k != key && now.Sub(e.used) > StatsCacheIdle {
			delete(statsCache, k)
		}
	}
	e := statsCache[key]
	if e == nil {
		if len(statsCache) >= StatsCacheMax {
			var oldK string
			var old time.Time
			for k, e := range statsCache {
				if oldK == "" || e.used.Before(old) {
					oldK, old = k, e.used
				}
			}
			delete(statsCache, oldK)
		}
		e = &statsEntry{}
		statsCache[key] = e
	}
	e.used = now
	return e
}

func storeGen(storeHex string) int64 {
	st := storeState(storeHex)
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.gen
}

func subset(a, b map[string]bool) bool {
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// statRows returns the list's rows (deleted ones included), from the cache when
// nothing changed since they were read.
func statRows(c *Ctx, storeHex string, res *Resource, b *legacyBackend, fields map[string]bool) ([]statRow, error) {
	scope := b.scopeFilter(c, storeHex)
	key := storeHex + "|" + res.Name + "|" + fmt.Sprint(scope)
	e := statsEntryFor(key)
	e.mu.Lock()
	defer e.mu.Unlock()
	ctx, cancel := statsCtx()
	defer cancel()
	col := b.col(storeHex)
	gen := storeGen(storeHex)
	fp, err := collFingerprint(ctx, col, storeHex+"."+col.Name())
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	if e.rows != nil && e.fp == fp && e.gen == gen && subset(fields, e.fields) {
		return e.rows, nil
	}
	want := map[string]bool{}
	for k := range fields {
		want[k] = true
	}
	if e.rows != nil && e.fp == fp && e.gen == gen {
		for k := range e.fields {
			want[k] = true
		}
	}
	f := andFilter(scope, bson.M{"erp.hd": bson.M{"$ne": true}})
	cur, err := col.Find(ctx, f, options.Find().SetProjection(bson.M{envKey + ".h": 0}).SetBatchSize(1000))
	if err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	defer cur.Close(ctx)
	x := newMapCtx(c, storeHex)
	rows := []statRow{}
	for cur.Next(ctx) {
		rows = append(rows, statRowOf(b.render(x, bsonToM(cur.Current), b.storeScoped()), want))
	}
	if err := cur.Err(); err != nil {
		return nil, errInternal("db: " + err.Error())
	}
	e.rows, e.fp, e.gen, e.fields = rows, fp, gen, want
	atomic.AddInt64(&statsBuilt, 1)
	return rows, nil
}

// ListStats adds up a store's records of one resource for q.
func ListStats(c *Ctx, storeHex string, res *Resource, q StatsQuery) (*StatsResult, error) {
	b, ok := res.Backend.(*legacyBackend)
	nb, native := res.Backend.(*nativeBackend)
	if !ok && !native {
		return nil, errBadRequest("This list has no server totals.", nil)
	}
	loc := orRiyadh(c.storeLoc(storeHex))
	if q.Today == "" {
		q.Today = time.Now().In(loc).Format(layoutDay)
	}
	if q.Now == "" {
		q.Now = time.Now().In(loc).Format(layoutWall)
	}
	var credits map[string]float64
	if _, ok := q.Filters["overdue"]; ok {
		credits = orderCreditsOf(c, storeHex)
	}
	var rows []statRow
	var err error
	if native {
		rows, err = nativeStatRows(c, storeHex, nb, statFields(q))
	} else {
		rows, err = statRows(c, storeHex, res, b, statFields(q))
	}
	if err != nil {
		return nil, err
	}
	return addUp(q, rows, credits), nil
}

// addUp adds up the rows q matches, and per group when q.GroupBy is set.
func addUp(q StatsQuery, rows []statRow, credits map[string]float64) *StatsResult {
	out := &StatsResult{Sums: map[string]float64{}}
	for _, s := range q.Sums {
		out.Sums[s] = 0
	}
	if q.GroupBy != "" {
		out.Groups = map[string]*StatsResult{}
	}
	if q.Lines != "" {
		var ls []statRow
		for _, r := range rows {
			ls = append(ls, r.Lines...)
		}
		rows = ls
	}
	var hits []statRow
	derived := derivedOf(q)
	for _, r := range rows {
		if len(derived) > 0 {
			r = withDerived(r, derived, q.Now)
		}
		if statsMatch(q, r, credits) {
			if q.Page > 0 {
				hits = append(hits, r)
			}
			addStats(out, q.Sums, r)
			if out.Groups != nil {
				key := r.Party
				if q.GroupBy != "party" {
					key = jsString(r.Vals[q.GroupBy])
				}
				g := out.Groups[key]
				if g == nil {
					g = &StatsResult{Sums: map[string]float64{}}
					out.Groups[key] = g
				}
				addStats(g, q.Sums, r)
			}
		}
	}
	if q.Page > 0 {
		out.IDs = pageIDs(hits, q.Sort, q.Page, q.Limit)
		if q.Lines != "" {
			out.Rows = lineRows(hits, q.Sort, q.Page, q.Limit)
			out.IDs = nil
		} else if q.Running != "" {
			out.Running = runningTotals(rows, q.DateKey, q.Running, out.IDs)
		}
	}
	out.round()
	return out
}

// sortValue: what a page of matches is ordered by (a number or text).
func sortValue(r statRow, key string) interface{} {
	switch key {
	case "net":
		return r.T.Net
	case "vat":
		return r.T.Vat
	case "paid":
		return r.T.Paid
	case "balance":
		return r.T.Balance
	case "taxable":
		return r.T.Taxable
	case "profit":
		return r.T.Profit
	case "cost":
		return r.T.Cost
	case "retailProfit":
		return r.Retail
	case "wholesaleProfit":
		return r.Whole
	case "pstatus":
		return r.T.Status
	case "zatca":
		return r.Zatca
	}
	return r.Vals[key]
}

// lessValue orders numbers before text, numbers by value and text case-blind
// (missing values first).
func lessValue(a, b interface{}) (less, equal bool) {
	fa, na := a.(float64)
	fb, nb := b.(float64)
	switch {
	case na && nb:
		return fa < fb, fa == fb
	case na != nb:
		return na, false
	}
	sa, sb := strings.ToLower(jsString(a)), strings.ToLower(jsString(b))
	return sa < sb, sa == sb
}

// pageIDs sorts the matches by sort ("-field" = descending; ties by id the same
// way) and returns the ids of one page.
func pageIDs(hits []statRow, sort string, page, limit int) []string {
	out := []string{}
	for _, i := range pageOrder(hits, sort, page, limit) {
		out = append(out, hits[i].ID)
	}
	return out
}

// lineRows: one page of payment lines (lines=payments), each line as the list shows
// it: "<record id>~<payment id>", its record's id as docId and the fields read.
func lineRows(hits []statRow, sort string, page, limit int) []M {
	out := []M{}
	for _, i := range pageOrder(hits, sort, page, limit) {
		h := hits[i]
		row := M{}
		for k, v := range h.Vals {
			row[k] = v
		}
		pid := jsString(h.Vals["paymentId"])
		if pid == "" {
			pid = strconv.Itoa(i)
		}
		row["id"], row["docId"] = h.ID+"~"+pid, h.ID
		if h.Party != "" {
			row["partyId"] = h.Party
		}
		out = append(out, row)
	}
	return out
}

// runningTotals is ledger.js W4 over the list's records (deleted ones left out): the
// running total of field in date then code order, for the ids asked.
func runningTotals(rows []statRow, dateKey, field string, ids []string) map[string]float64 {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	live := make([]statRow, 0, len(rows))
	for _, r := range rows {
		if !r.Deleted {
			live = append(live, r)
		}
	}
	sortpkg.SliceStable(live, func(i, j int) bool {
		di, dj := jsString(live[i].Vals[dateKey]), jsString(live[j].Vals[dateKey])
		if di != dj {
			return di < dj
		}
		return jsString(live[i].Vals["code"]) < jsString(live[j].Vals["code"])
	})
	out := map[string]float64{}
	var t float64
	for _, r := range live {
		t = round2(t + num(r.Vals[field]))
		if want[r.ID] {
			out[r.ID] = t
		}
	}
	return out
}

// pageOrder: the indexes of hits on one page, in sort order.
func pageOrder(hits []statRow, sort string, page, limit int) []int {
	desc := strings.HasPrefix(sort, "-")
	key := strings.TrimPrefix(sort, "-")
	vals := make([]interface{}, len(hits))
	for i, r := range hits {
		vals[i] = sortValue(r, key)
	}
	idx := make([]int, len(hits))
	for i := range idx {
		idx[i] = i
	}
	sortpkg.SliceStable(idx, func(i, j int) bool {
		a, b := idx[i], idx[j]
		lt, eq := lessValue(vals[a], vals[b])
		if eq {
			lt = hits[a].ID < hits[b].ID
			if desc {
				return hits[b].ID < hits[a].ID
			}
			return lt
		}
		if desc {
			gt, _ := lessValue(vals[b], vals[a])
			return gt
		}
		return lt
	})
	out := []int{}
	for i := (page - 1) * limit; i < len(idx) && i < page*limit; i++ {
		out = append(out, idx[i])
	}
	return out
}

func (s *StatsResult) round() {
	for k, v := range s.Sums {
		s.Sums[k] = round2(v)
	}
	for _, g := range s.Groups {
		g.round()
	}
}

// orderCreditsOf is finance.js orderCredits over the store's deposits and sales returns.
func orderCreditsOf(c *Ctx, storeHex string) map[string]float64 {
	cr := map[string]float64{}
	if b := resourceNamed("deposits"); b != nil {
		_ = b.each(c, storeHex, nil, func(rec M) {
			if id := str(rec["orderId"]); id != "" {
				cr[id] += num(rec["amount"])
			}
		})
	}
	if b := resourceNamed("salesReturns"); b != nil {
		_ = b.each(c, storeHex, nil, func(rec M) {
			if id := str(rec["orderId"]); id != "" {
				if bal := ComputeTotals(rec).Balance; bal != 0 {
					cr[id] += bal
				}
			}
		})
	}
	return cr
}

func handleListStats(w http.ResponseWriter, r *http.Request, res *Resource) {
	c, err := authenticate(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := res.checkPerm(c, "view"); err != nil {
		writeErr(w, err)
		return
	}
	q, err := parseStatsQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	q = withResourceDateKey(q, res, r.URL.Query().Get("dateKey"))
	storeHex, err := resolveStore(c, res, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	if storeHex == "" {
		writeErr(w, errBadRequest("storeId is required.", map[string]string{"storeId": "required"}))
		return
	}
	out, err := ListStats(c, storeHex, res, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	body := M{"storeId": storeHex, "timezone": orRiyadh(c.storeLoc(storeHex)).String(),
		"from": q.From, "to": q.To, "count": out.Count, "sums": out.Sums}
	if out.Groups != nil {
		body["groups"] = out.Groups
	}
	if q.Page > 0 && q.Lines != "" {
		body["rows"] = out.Rows
		body["page"], body["limit"], body["sort"] = q.Page, q.Limit, q.Sort
	} else if q.Page > 0 {
		body["ids"] = out.IDs
		if out.Running != nil {
			body["running"] = out.Running
		}
		body["page"], body["limit"], body["sort"] = q.Page, q.Limit, q.Sort
	}
	writeJSON(w, http.StatusOK, body)
}

// hasListStats: dated store lists (the web app holds only their last year).
func hasListStats(res *Resource) bool {
	if res.Scope != "store" || res.DateField == "" {
		return false
	}
	switch res.Backend.(type) {
	case *legacyBackend, *nativeBackend:
		return true
	}
	return false
}

// nativeStatRows reads every record of a native list (new collections stay small, so
// they are not cached), deleted ones included.
func nativeStatRows(c *Ctx, storeHex string, b *nativeBackend, fields map[string]bool) ([]statRow, error) {
	sel, _ := parseSelect("-history")
	rows := []statRow{}
	for page := 1; ; page++ {
		recs, total, err := b.List(c, storeHex, ListQuery{Limit: 1000, Page: page, IncludeDeleted: true, Select: sel})
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			rows = append(rows, statRowOf(rec, fields))
		}
		if len(recs) == 0 || int64(page*1000) >= total {
			return rows, nil
		}
	}
}

// statsCtx bounds one /stats read (every record of a store's list, like the dashboards).
func statsCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}
