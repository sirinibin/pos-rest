package erp

// dashboard_bi.go — every figure on StartERP's BI dashboard (#/app/dashboard/bi).
//
//   GET /v1/erp/dashboard/bi?storeId=X
//
// The page used to work these out in the browser from the lists it had loaded:
// a year of documents (newest dropped past 20 000 rows), loaded once per sign-in,
// "today" frozen at page load and in the browser's own timezone.  So customers
// who first bought more than a year ago showed as new, invoices open for more
// than a year were missing from aging, walk-in sales (no customer) counted as one
// huge customer, and Refresh changed nothing.  Here every figure is computed on
// request from the store's whole history, with today and month boundaries in
// the store's country timezone.  The formulas are the page's own (finance.js
// xx/T4/yD/My/mx, totals.js computeTotals), applied to the same contract-shaped
// records the adapter serves.

import (
	"context"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BIMonths is how many calendar months (ending with the current one) the page
// covers unless the store's BI settings say otherwise.
const BIMonths = 12

// BISettings are the store's BI dashboard settings (store.bi in the contract,
// kept in the store's erp.x.bi). Zero values take the defaults.
type BISettings struct {
	Months          int `json:"months"`          // 6–24 calendar months shown, default 12
	ActiveDays      int `json:"activeDays"`      // "active customer" bought within, default 90
	ChurnMediumDays int `json:"churnMediumDays"` // medium churn risk after, default 45
	ChurnHighDays   int `json:"churnHighDays"`   // high churn risk after, default 120
	OverdueDays     int `json:"overdueDays"`     // "overdue" invoices older than, default 30
	SlowMonths      int `json:"slowMonths"`      // slow stock: no sale for, default 3 months
}

func clampInt(v, lo, hi, def int) int {
	if v == 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Normalized fills defaults and keeps every value in range (high risk after medium).
func (s BISettings) Normalized() BISettings {
	out := BISettings{
		Months:          clampInt(s.Months, 3, 36, BIMonths),
		ActiveDays:      clampInt(s.ActiveDays, 7, 365, 90),
		ChurnMediumDays: clampInt(s.ChurnMediumDays, 7, 365, 45),
		ChurnHighDays:   clampInt(s.ChurnHighDays, 14, 730, 120),
		OverdueDays:     clampInt(s.OverdueDays, 1, 365, 30),
		SlowMonths:      clampInt(s.SlowMonths, 1, 12, 3),
	}
	if out.ChurnHighDays <= out.ChurnMediumDays {
		out.ChurnHighDays = out.ChurnMediumDays + 1
	}
	return out
}

// BISettingsOf reads a store's BI settings from its stored record.
func BISettingsOf(store M) BISettings {
	b := sub(sub(sub(store, envKey), "x"), "bi")
	return BISettings{Months: int(intv(b["months"])), ActiveDays: int(intv(b["activeDays"])),
		ChurnMediumDays: int(intv(b["churnMediumDays"])), ChurnHighDays: int(intv(b["churnHighDays"])),
		OverdueDays: int(intv(b["overdueDays"])), SlowMonths: int(intv(b["slowMonths"]))}.Normalized()
}

// ---- period ----

// BIPeriod is "today" and the covered months in the store's timezone.
type BIPeriod struct {
	Today     string   `json:"today"`     // YYYY-MM-DD
	ThisMonth string   `json:"thisMonth"` // YYYY-MM
	Months    []string `json:"months"`    // oldest first, ends with ThisMonth
	From      string   `json:"from"`      // first day of Months[0]
	To        string   `json:"to"`        // last day of ThisMonth
}

func biAddMonths(ym string, n int) string {
	t, err := time.Parse("2006-01", ym)
	if err != nil {
		return ym
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}

func biMonthEnd(ym string) string {
	t, _ := time.Parse("2006-01", ym)
	return t.AddDate(0, 1, -1).Format(layoutDay)
}

// NewBIPeriod is the n months ending with the month `now` falls in, at loc.
func NewBIPeriod(now time.Time, loc *time.Location, n int) BIPeriod {
	if n < 1 {
		n = 1
	}
	today := now.In(orRiyadh(loc)).Format(layoutDay)
	this := today[:7]
	months := make([]string, n)
	for i := 0; i < n; i++ {
		months[i] = biAddMonths(this, i-n+1)
	}
	return BIPeriod{Today: today, ThisMonth: this, Months: months, From: months[0] + "-01", To: biMonthEnd(this)}
}

func (p BIPeriod) has(day string) bool { return day >= p.From && day <= p.To }

// biDays is the page's Vo(): whole days from a to b (YYYY-MM-DD).
func biDays(a, b string) int {
	ta, err1 := time.Parse(layoutDay, a)
	tb, err2 := time.Parse(layoutDay, b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

// ---- documents ----

// BILine is one item line of a sales-side document.
type BILine struct {
	ProductID string
	NameEn    string
	NameAr    string
	Qty       float64
	Rev       float64 // qty × (unit price − unit discount), VAT excluded
	Cost      float64 // qty × purchase price
}

// BIDoc is a sales-side document reduced to what the dashboard reads.
type BIDoc struct {
	ID         string
	Code       string
	Day        string // YYYY-MM-DD, store time
	CustomerID string
	NameEn     string
	NameAr     string
	Taxable    float64
	Net        float64 // incl. VAT and rounding
	Balance    float64
	Lines      []BILine
	OrderID    string   // returns: the invoice returned; deposits: the invoice paid
	Amount     float64  // deposits
	Status     string   // quotations
	OrderIDs   []string // quotations: invoices made from it
}

func (d BIDoc) month() string {
	if len(d.Day) < 7 {
		return ""
	}
	return d.Day[:7]
}

// biTotals is totals.js computeTotals: taxable amount, total incl. VAT, open balance.
func biTotals(rec M) (taxable, net, balance float64) {
	vat := 15.0
	if v, ok := rec["vatPercent"]; ok && v != nil {
		vat = num(v)
	}
	gross, disc := 0.0, 0.0
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		q := num(im["qty"])
		gross += q * num(im["unitPrice"])
		disc += q * num(im["unitDiscount"])
	}
	taxable = round2(gross - disc - num(rec["discount"]) + num(rec["shipping"]))
	before := round2(taxable + round2(taxable*vat/100))
	var rounding float64
	if boolv(rec["roundingAuto"]) {
		rounding = round2(math.Floor(before*20+0.5)/20 - before)
	} else {
		rounding = round2(num(rec["rounding"]))
	}
	net = round2(before + rounding)
	paid := 0.0
	for _, p := range arr(rec["payments"]) {
		pm, _ := p.(M)
		paid += num(pm["amount"])
	}
	balance = round2(net - round2(paid) - num(rec["cashDiscount"]))
	if balance < 0 {
		balance = 0
	}
	return taxable, net, balance
}

// BIDocOf reduces a contract-shaped record (as the adapter serves it).
func BIDocOf(rec M) BIDoc {
	taxable, net, balance := biTotals(rec)
	d := BIDoc{ID: str(rec["id"]), Code: str(rec["code"]), CustomerID: str(rec["customerId"]),
		NameEn: str(rec["customerName"]), NameAr: str(rec["customerNameAr"]), Taxable: taxable, Net: net, Balance: balance,
		OrderID: str(rec["orderId"]), Amount: num(rec["amount"]), Status: str(rec["status"]), OrderIDs: strs(rec["orderIds"])}
	if s := str(rec["date"]); len(s) >= 10 {
		d.Day = s[:10]
	}
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		q := num(im["qty"])
		d.Lines = append(d.Lines, BILine{ProductID: str(im["productId"]), NameEn: str(im["nameEn"]), NameAr: str(im["nameAr"]),
			Qty: q, Rev: q * (num(im["unitPrice"]) - num(im["unitDiscount"])), Cost: q * num(im["purchasePrice"])})
	}
	return d
}

// BICustomer is a customer master record (names and opening balance).
type BICustomer struct {
	ID             string
	NameEn, NameAr string
	Opening        float64 // receivable positive, payable negative
	WalkIn         bool    // the counter's cash / walk-in customer record
}

var (
	reWalkInEn = regexp.MustCompile(`(?i)\bwalk[\s-]*in\b|\bcash\s+(customer|client|sales?)\b`)
	reWalkInAr = regexp.MustCompile(`عميل\s*(نقدي|عابر)`)
)

// IsWalkInCustomer: the shared counter customer POS sales are booked to (category
// "Cash", or named like "Walk-in customer" / "عميل نقدي"). Its sales are not one
// customer's history, so customer analytics leave it out.
func IsWalkInCustomer(nameEn, nameAr string, category []string) bool {
	for _, c := range category {
		if strings.EqualFold(strings.TrimSpace(c), "cash") {
			return true
		}
	}
	return reWalkInEn.MatchString(nameEn) || reWalkInAr.MatchString(nameAr)
}

// BIProduct is a product master record (names, stock on hand, purchase price).
type BIProduct struct {
	ID             string
	NameEn, NameAr string
	Code           string
	Stock          float64
	Purchase       float64
	IsService      bool
	IsSet          bool
}

// BIInput is everything the dashboard is computed from.
type BIInput struct {
	Sales, NonVAT, Returns, Deposits, Quotations []BIDoc
	Customers                                    []BICustomer
	Products                                     map[string]BIProduct // sold products and stocked products
}

// ---- output ----

type BICustomerRow struct {
	ID      string  `json:"id"`
	NameEn  string  `json:"nameEn"`
	NameAr  string  `json:"nameAr"`
	Orders  int     `json:"orders"`
	Net     float64 `json:"net"` // lifetime spend incl. VAT, returns not deducted (as the page always did)
	First   string  `json:"first"`
	Last    string  `json:"last"`
	Recency int     `json:"recency"` // days since Last
	Tier    string  `json:"tier"`    // churn risk: low | medium | high
}

type BINewReturning struct {
	Key  string `json:"key"`
	NewC int    `json:"newC"`
	Ret  int    `json:"ret"`
}

type BIBucket struct {
	Label     string  `json:"label"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Customers int     `json:"customers"`
}

type BICohort struct {
	Key  string `json:"key"`
	Size int    `json:"size"`
	Heat []*int `json:"heat"` // % of the cohort buying in month +n; null for future months
}

type BICustomers struct {
	Rows           []BICustomerRow  `json:"rows"`
	Active         int              `json:"active"` // bought in the last 90 days
	Repeat         float64          `json:"repeat"` // % with more than one order
	AvgClv         float64          `json:"avgClv"`
	NewVsReturning []BINewReturning `json:"newVsReturning"`
	Clv            []BIBucket       `json:"clv"`
	Risk           map[string]int   `json:"risk"`
	Cohorts        []BICohort       `json:"cohorts"`
}

type BIProductRow struct {
	ID         string    `json:"id"`
	NameEn     string    `json:"nameEn"`
	NameAr     string    `json:"nameAr"`
	Code       string    `json:"code"`
	Rev        float64   `json:"rev"`
	Cost       float64   `json:"cost"`
	Qty        float64   `json:"qty"`
	RetQty     float64   `json:"retQty"`
	RetRev     float64   `json:"retRev"`
	Orders     int       `json:"orders"`
	Margin     float64   `json:"margin"`
	ByMonth    []float64 `json:"byMonth"`    // revenue per period month
	QtyByMonth []float64 `json:"qtyByMonth"` // units per period month
}

type BIAgingRow struct {
	ID      string  `json:"id"`
	Code    string  `json:"code"`
	D       string  `json:"d"`
	NameEn  string  `json:"nameEn"`
	NameAr  string  `json:"nameAr"`
	Age     int     `json:"age"`
	Balance float64 `json:"balance"`
}

type BIAging struct {
	Key    string       `json:"key"`
	Min    int          `json:"min"`
	Max    int          `json:"max"`
	Count  int          `json:"count"`
	Amount float64      `json:"amount"`
	Rows   []BIAgingRow `json:"rows"`
}

type BIQuotStatus struct {
	S     string  `json:"s"`
	N     int     `json:"n"`
	Value float64 `json:"value"`
}

type BIQuotations struct {
	Created  int            `json:"created"`
	Sent     int            `json:"sent"`
	Decided  int            `json:"decided"`
	Accepted int            `json:"accepted"`
	Invoiced int            `json:"invoiced"`
	Rate     float64        `json:"rate"`
	ByStatus []BIQuotStatus `json:"byStatus"`
}

type BIMonthRow struct {
	Key     string  `json:"key"`
	Revenue float64 `json:"revenue"` // net sales VAT excluded, returns deducted
	Orders  int     `json:"orders"`
}

type BIOverdue struct {
	ID      string  `json:"id"`
	NameEn  string  `json:"nameEn"`
	NameAr  string  `json:"nameAr"`
	Balance float64 `json:"balance"`
	Days    int     `json:"days"`
}

type BISlow struct {
	ID     string  `json:"id"`
	NameEn string  `json:"nameEn"`
	NameAr string  `json:"nameAr"`
	Qty    float64 `json:"qty"`
	Value  float64 `json:"value"`
}

type BIAsk struct {
	LastMonth    string         `json:"lastMonth"`
	TopLastMonth []BIProductRow `json:"topLastMonth"`
	Overdue      []BIOverdue    `json:"overdue"` // open more than OverdueDays
	Monthly      []BIMonthRow   `json:"monthly"`
	SlowCount    int            `json:"slowCount"`
	SlowValue    float64        `json:"slowValue"`
	Slow         []BISlow       `json:"slow"` // top 5 by value
}

// BIResult is the endpoint's response body (plus storeId/timezone/generatedAt).
type BIResult struct {
	Period      BIPeriod       `json:"period"`
	Customers   BICustomers    `json:"customers"`
	Products    []BIProductRow `json:"products"`
	Aging       []BIAging      `json:"aging"`
	Receivables float64        `json:"receivables"`
	Quotations  BIQuotations   `json:"quotations"`
	Ask         BIAsk          `json:"ask"`
	Settings    BISettings     `json:"settings"`
}

// ---- computation (pure) ----

// ComputeBI works out every dashboard figure from the store's records.
func ComputeBI(in BIInput, p BIPeriod, set BISettings) BIResult {
	set = set.Normalized()
	// sales after order credits: deposits against the invoice and open returns of it
	credits := map[string]float64{}
	for _, d := range in.Deposits {
		if d.OrderID != "" {
			credits[d.OrderID] += d.Amount
		}
	}
	for _, r := range in.Returns {
		if r.OrderID != "" && r.Balance != 0 {
			credits[r.OrderID] += r.Balance
		}
	}
	sales := make([]BIDoc, len(in.Sales))
	for i, s := range in.Sales {
		if c := credits[s.ID]; c != 0 {
			s.Balance = math.Max(0, round2(s.Balance-c))
		}
		sales[i] = s
	}
	all := append(append([]BIDoc{}, sales...), in.NonVAT...)
	custByID := map[string]BICustomer{}
	for _, c := range in.Customers {
		custByID[c.ID] = c
	}
	return BIResult{
		Period:      p,
		Customers:   biCustomers(all, custByID, p, set),
		Products:    biProducts(all, in.Returns, in.Products, p.Months, p.has),
		Aging:       biAging(sales, p.Today),
		Receivables: biReceivables(sales, p.Today),
		Quotations:  biQuotations(in.Quotations, p),
		Ask:         biAsk(all, sales, in, p, set),
		Settings:    set,
	}
}

func biCustomers(all []BIDoc, custByID map[string]BICustomer, p BIPeriod, set BISettings) BICustomers {
	type acc struct {
		row    BICustomerRow
		months map[string]bool
	}
	by := map[string]*acc{}
	order := []string{}
	for _, d := range all {
		// walk-in sales (no customer, or the counter's cash customer) are not a customer's history
		if d.CustomerID == "" || d.Day == "" {
			continue
		}
		if c, ok := custByID[d.CustomerID]; ok && c.WalkIn || !ok && IsWalkInCustomer(d.NameEn, d.NameAr, nil) {
			continue
		}
		a := by[d.CustomerID]
		if a == nil {
			a = &acc{row: BICustomerRow{ID: d.CustomerID, NameEn: d.NameEn, NameAr: d.NameAr, First: d.Day, Last: d.Day},
				months: map[string]bool{}}
			if c, ok := custByID[d.CustomerID]; ok {
				if c.NameEn != "" {
					a.row.NameEn = c.NameEn
				}
				if c.NameAr != "" {
					a.row.NameAr = c.NameAr
				}
			}
			by[d.CustomerID] = a
			order = append(order, d.CustomerID)
		}
		a.row.Orders++
		a.row.Net += d.Net
		if d.Day < a.row.First {
			a.row.First = d.Day
		}
		if d.Day > a.row.Last {
			a.row.Last = d.Day
		}
		a.months[d.month()] = true
	}
	out := BICustomers{Rows: []BICustomerRow{}, Risk: map[string]int{"low": 0, "medium": 0, "high": 0},
		NewVsReturning: []BINewReturning{}, Cohorts: []BICohort{}}
	repeat, total := 0, 0.0
	firstMonth := map[string]string{}
	for _, id := range order {
		a := by[id]
		r := &a.row
		r.Net = round2(r.Net)
		r.Recency = biDays(r.Last, p.Today)
		switch {
		case r.Recency > set.ChurnHighDays || (r.Recency > set.ChurnHighDays/2 && r.Orders <= 2):
			r.Tier = "high"
		case r.Recency > set.ChurnMediumDays:
			r.Tier = "medium"
		default:
			r.Tier = "low"
		}
		out.Risk[r.Tier]++
		if r.Recency <= set.ActiveDays {
			out.Active++
		}
		if r.Orders > 1 {
			repeat++
		}
		total += r.Net
		firstMonth[id] = r.First[:7]
		out.Rows = append(out.Rows, *r)
	}
	if n := len(out.Rows); n > 0 {
		out.Repeat = float64(repeat) / float64(n) * 100
		out.AvgClv = total / float64(n)
	}
	buyers := func(m string) []string {
		ids := []string{}
		for _, id := range order {
			if by[id].months[m] {
				ids = append(ids, id)
			}
		}
		return ids
	}
	for _, m := range p.Months {
		nr := BINewReturning{Key: m}
		for _, id := range buyers(m) {
			if firstMonth[id] == m {
				nr.NewC++
			} else {
				nr.Ret++
			}
		}
		out.NewVsReturning = append(out.NewVsReturning, nr)
	}
	for _, b := range []struct {
		min, max float64
		label    string
	}{{0, 5000, "< 5k"}, {5000, 10000, "5–10k"}, {10000, 25000, "10–25k"}, {25000, 50000, "25–50k"},
		{50000, 100000, "50–100k"}, {100000, 1e12, "100k+"}} {
		n := 0
		for _, r := range out.Rows {
			if r.Net >= b.min && r.Net < b.max {
				n++
			}
		}
		out.Clv = append(out.Clv, BIBucket{Label: b.label, Min: b.min, Max: b.max, Customers: n})
	}
	for _, m := range p.Months {
		cohort := []string{}
		for _, id := range order {
			if firstMonth[id] == m {
				cohort = append(cohort, id)
			}
		}
		if len(cohort) == 0 {
			continue
		}
		c := BICohort{Key: m, Size: len(cohort), Heat: make([]*int, len(p.Months))}
		for x := range p.Months {
			f := biAddMonths(m, x)
			if f > p.ThisMonth {
				continue
			}
			n := 0
			for _, id := range cohort {
				if by[id].months[f] {
					n++
				}
			}
			v := int(math.Floor(float64(n)/float64(len(cohort))*100 + 0.5))
			c.Heat[x] = &v
		}
		out.Cohorts = append(out.Cohorts, c)
	}
	return out
}

func biProducts(all, returns []BIDoc, prods map[string]BIProduct, months []string, in func(string) bool) []BIProductRow {
	idx := map[string]int{}
	for i, m := range months {
		idx[m] = i
	}
	by := map[string]*BIProductRow{}
	order := []string{}
	row := func(l BILine) *BIProductRow {
		r := by[l.ProductID]
		if r == nil {
			r = &BIProductRow{ID: l.ProductID, NameEn: l.NameEn, NameAr: l.NameAr,
				ByMonth: make([]float64, len(months)), QtyByMonth: make([]float64, len(months))}
			if p, ok := prods[l.ProductID]; ok {
				r.NameEn, r.NameAr, r.Code = p.NameEn, p.NameAr, p.Code
			}
			by[l.ProductID] = r
			order = append(order, l.ProductID)
		}
		return r
	}
	for _, d := range all {
		if !in(d.Day) {
			continue
		}
		for _, l := range d.Lines {
			if l.ProductID == "" {
				continue
			}
			r := row(l)
			r.Rev += l.Rev
			r.Cost += l.Cost
			r.Qty += l.Qty
			r.Orders++
			if i, ok := idx[d.month()]; ok {
				r.ByMonth[i] += l.Rev
				r.QtyByMonth[i] += l.Qty
			}
		}
	}
	for _, d := range returns {
		if !in(d.Day) {
			continue
		}
		for _, l := range d.Lines {
			if l.ProductID == "" {
				continue
			}
			r := row(l)
			r.RetQty += l.Qty
			r.RetRev += l.Rev
		}
	}
	out := make([]BIProductRow, 0, len(order))
	for _, id := range order {
		r := by[id]
		if r.Rev != 0 {
			r.Margin = (r.Rev - r.Cost) / r.Rev * 100
		}
		r.Rev, r.Cost, r.RetRev = round2(r.Rev), round2(r.Cost), round2(r.RetRev)
		for i := range r.ByMonth {
			r.ByMonth[i] = round2(r.ByMonth[i])
		}
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rev > out[j].Rev })
	return out
}

var biAgingBuckets = []BIAging{{Key: "0-30", Min: 0, Max: 30}, {Key: "31-60", Min: 31, Max: 60},
	{Key: "61-90", Min: 61, Max: 90}, {Key: "90+", Min: 91, Max: 1000000000}}

func biAging(sales []BIDoc, today string) []BIAging {
	out := make([]BIAging, len(biAgingBuckets))
	for i, b := range biAgingBuckets {
		b.Rows = []BIAgingRow{}
		out[i] = b
	}
	for _, s := range sales {
		if s.Balance <= 0.004 || s.Day == "" || s.Day > today {
			continue
		}
		age := biDays(s.Day, today)
		for i := range out {
			if age >= out[i].Min && age <= out[i].Max {
				out[i].Rows = append(out[i].Rows, BIAgingRow{ID: s.ID, Code: s.Code, D: s.Day, NameEn: s.NameEn, NameAr: s.NameAr,
					Age: age, Balance: s.Balance})
				out[i].Count++
				out[i].Amount += s.Balance
				break
			}
		}
	}
	for i := range out {
		out[i].Amount = round2(out[i].Amount)
		sort.SliceStable(out[i].Rows, func(a, b int) bool { return out[i].Rows[a].Age > out[i].Rows[b].Age })
	}
	return out
}

func biReceivables(sales []BIDoc, today string) float64 {
	t := 0.0
	for _, b := range biAging(sales, today) {
		t += b.Amount
	}
	return round2(t)
}

var biQuotStatuses = []string{"created", "delivered", "pending", "accepted", "rejected", "expired", "cancelled"}

func biQuotations(qs []BIDoc, p BIPeriod) BIQuotations {
	out := BIQuotations{ByStatus: []BIQuotStatus{}}
	n := map[string]int{}
	v := map[string]float64{}
	for _, q := range qs {
		if !p.has(q.Day) {
			continue
		}
		out.Created++
		n[q.Status]++
		v[q.Status] += q.Taxable
		if q.Status != "created" {
			out.Sent++
		}
		if len(q.OrderIDs) > 0 {
			out.Invoiced++
		}
	}
	out.Accepted = n["accepted"]
	out.Decided = n["accepted"] + n["rejected"] + n["expired"] + n["cancelled"]
	if out.Decided > 0 {
		out.Rate = float64(out.Accepted) / float64(out.Decided) * 100
	}
	for _, s := range biQuotStatuses {
		out.ByStatus = append(out.ByStatus, BIQuotStatus{S: s, N: n[s], Value: round2(v[s])})
	}
	return out
}

func biAsk(all, sales []BIDoc, in BIInput, p BIPeriod, set BISettings) BIAsk {
	out := BIAsk{LastMonth: biAddMonths(p.ThisMonth, -1), TopLastMonth: []BIProductRow{}, Overdue: []BIOverdue{},
		Monthly: []BIMonthRow{}, Slow: []BISlow{}}
	lm := out.LastMonth
	top := biProducts(all, nil, in.Products, []string{lm}, func(d string) bool { return len(d) >= 7 && d[:7] == lm })
	if len(top) > 5 {
		top = top[:5]
	}
	out.TopLastMonth = top

	// customers with invoices open for more than 30 days (opening balance included in the total)
	open := map[string][]BIDoc{}
	for _, s := range sales {
		if s.CustomerID != "" && s.Day != "" && s.Day <= p.Today && s.Balance > 0.004 {
			open[s.CustomerID] = append(open[s.CustomerID], s)
		}
	}
	for _, c := range in.Customers {
		docs := open[c.ID]
		if len(docs) == 0 {
			continue
		}
		bal, oldest := c.Opening, docs[0].Day
		for _, d := range docs {
			bal += d.Balance
			if d.Day < oldest {
				oldest = d.Day
			}
		}
		bal = round2(bal)
		if days := biDays(oldest, p.Today); bal > 0.004 && days > set.OverdueDays {
			out.Overdue = append(out.Overdue, BIOverdue{ID: c.ID, NameEn: c.NameEn, NameAr: c.NameAr, Balance: bal, Days: days})
		}
	}
	sort.SliceStable(out.Overdue, func(i, j int) bool { return out.Overdue[i].Balance > out.Overdue[j].Balance })

	idx := map[string]int{}
	for i, m := range p.Months {
		idx[m] = i
		out.Monthly = append(out.Monthly, BIMonthRow{Key: m})
	}
	for _, d := range all {
		if i, ok := idx[d.month()]; ok && p.has(d.Day) {
			out.Monthly[i].Revenue += d.Taxable
			out.Monthly[i].Orders++
		}
	}
	for _, d := range in.Returns {
		if i, ok := idx[d.month()]; ok && p.has(d.Day) {
			out.Monthly[i].Revenue -= d.Taxable
		}
	}
	for i := range out.Monthly {
		out.Monthly[i].Revenue = round2(out.Monthly[i].Revenue)
	}

	// stocked products with no sale in the last three months
	since := biAddMonths(p.ThisMonth, -set.SlowMonths) + "-01"
	sold := map[string]bool{}
	for _, d := range all {
		if d.Day >= since {
			for _, l := range d.Lines {
				sold[l.ProductID] = true
			}
		}
	}
	for _, pr := range in.Products {
		if pr.IsService || pr.IsSet || pr.Stock <= 0 || sold[pr.ID] {
			continue
		}
		s := BISlow{ID: pr.ID, NameEn: pr.NameEn, NameAr: pr.NameAr, Qty: pr.Stock, Value: round2(pr.Stock * pr.Purchase)}
		out.SlowCount++
		out.SlowValue += s.Value
		out.Slow = append(out.Slow, s)
	}
	out.SlowValue = round2(out.SlowValue)
	sort.SliceStable(out.Slow, func(i, j int) bool {
		if out.Slow[i].Value != out.Slow[j].Value {
			return out.Slow[i].Value > out.Slow[j].Value
		}
		return out.Slow[i].ID < out.Slow[j].ID
	})
	if len(out.Slow) > 5 {
		out.Slow = out.Slow[:5]
	}
	return out
}

// ---- loading ----

func resourceNamed(name string) *legacyBackend {
	for _, r := range Resources() {
		if r.Name == name {
			if b, ok := r.Backend.(*legacyBackend); ok {
				return b
			}
		}
	}
	return nil
}

// each streams a store's live records (not hidden, not deleted) in contract
// shape, without their change history.
func (b *legacyBackend) each(c *Ctx, storeHex string, extra bson.M, fn func(rec M)) error {
	f := andFilter(b.scopeFilter(c, storeHex), bson.M{"erp.hd": bson.M{"$ne": true}}, bson.M{"erp.del": bson.M{"$ne": true}})
	if b.deletedKey != "" {
		f = andFilter(f, bson.M{b.deletedKey: bson.M{"$ne": true}})
	}
	f = andFilter(f, extra)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cur, err := b.col(storeHex).Find(ctx, f, options.Find().SetProjection(bson.M{envKey + ".h": 0}).SetBatchSize(1000))
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	x := newMapCtx(c, storeHex)
	for cur.Next(ctx) {
		d := bsonToM(cur.Current)
		rec := b.toC(x, d)
		rec["id"] = hexOf(d["_id"])
		fn(rec)
	}
	return cur.Err()
}

// LoadBI reads a store's records for ComputeBI, the collections in parallel.
func LoadBI(c *Ctx, storeHex string, p BIPeriod, loc *time.Location) (BIInput, error) {
	in := BIInput{Products: map[string]BIProduct{}}
	docs := func(name string, extra bson.M, into *[]BIDoc) func() error {
		return func() error {
			b := resourceNamed(name)
			if b == nil {
				return nil
			}
			return b.each(c, storeHex, extra, func(rec M) { *into = append(*into, BIDocOf(rec)) })
		}
	}
	var salesReturns, nonvatReturns []BIDoc
	// quotations: only the covered months are shown
	qFrom, _ := time.ParseInLocation(layoutDay, p.From, orRiyadh(loc))
	jobs := []func() error{
		docs("sales", nil, &in.Sales),
		docs("nonvatSales", nil, &in.NonVAT),
		docs("salesReturns", nil, &salesReturns),
		docs("nonvatReturns", nil, &nonvatReturns),
		docs("deposits", nil, &in.Deposits),
		docs("quotations", bson.M{"date": bson.M{"$gte": qFrom.UTC()}}, &in.Quotations),
		func() error {
			b := resourceNamed("customers")
			if b == nil {
				return nil
			}
			return b.each(c, storeHex, nil, func(rec M) {
				o := num(rec["openingBalance"])
				if str(rec["openingBalanceType"]) == "payable" {
					o = -o
				}
				in.Customers = append(in.Customers, BICustomer{ID: str(rec["id"]), NameEn: str(rec["nameEn"]), NameAr: str(rec["nameAr"]), Opening: o,
					WalkIn: IsWalkInCustomer(str(rec["nameEn"]), str(rec["nameAr"]), strs(rec["category"]))})
			})
		},
	}
	errs := make(chan error, len(jobs))
	for _, j := range jobs {
		go func(j func() error) { errs <- j() }(j)
	}
	var first error
	for range jobs {
		if err := <-errs; err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return in, first
	}
	in.Returns = append(salesReturns, nonvatReturns...)

	// products: the ones sold or returned (for names) and the ones in stock (slow stock)
	ids := map[string]bool{}
	for _, list := range [][]BIDoc{in.Sales, in.NonVAT, in.Returns} {
		for _, d := range list {
			for _, l := range d.Lines {
				ids[l.ProductID] = true
			}
		}
	}
	oids := bson.A{}
	for id := range ids {
		if oid, err := primitive.ObjectIDFromHex(id); err == nil {
			oids = append(oids, oid)
		}
	}
	pb := resourceNamed("products")
	if pb == nil {
		return in, nil
	}
	f := bson.M{"$or": bson.A{
		bson.M{"_id": bson.M{"$in": oids}},
		bson.M{"product_stores." + storeHex + ".stock": bson.M{"$gt": 0}},
	}}
	err := pb.each(c, storeHex, f, func(rec M) {
		stock := 0.0
		for _, s := range sub(rec, "stock") {
			if sm, ok := s.(M); ok {
				stock += num(sm["qty"])
			}
		}
		in.Products[str(rec["id"])] = BIProduct{ID: str(rec["id"]), NameEn: str(rec["nameEn"]), NameAr: str(rec["nameAr"]),
			Code: str(rec["code"]), Stock: stock, Purchase: num(sub(rec, "pricing")["purchase"]),
			IsService: boolv(rec["isService"]), IsSet: boolv(rec["isSet"])}
	})
	return in, err
}

func handleDashboardBI(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	storeHex := r.URL.Query().Get("storeId")
	if storeHex == "" {
		return errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	if c.store(storeHex) == nil {
		return errNotFound()
	}
	if !c.Admin && !c.can("reports", "view") {
		return errForbidden("")
	}
	loc := c.storeLoc(storeHex)
	now := time.Now()
	set := BISettingsOf(c.store(storeHex))
	p := NewBIPeriod(now, loc, set.Months)
	in, err := LoadBI(c, storeHex, p, loc)
	if err != nil {
		return errInternal("Unable to calculate the BI dashboard.")
	}
	res := ComputeBI(in, p, set)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, M{
		"storeId":     storeHex,
		"timezone":    orRiyadh(loc).String(),
		"generatedAt": now.In(orRiyadh(loc)).Format("2006-01-02T15:04:05"),
		"period":      res.Period,
		"customers":   res.Customers,
		"products":    res.Products,
		"aging":       res.Aging,
		"receivables": res.Receivables,
		"quotations":  res.Quotations,
		"ask":         res.Ask,
		"settings":    res.Settings,
	})
	return nil
}
