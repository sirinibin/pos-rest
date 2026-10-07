package erp

// dashboard_feed.go — the main dashboard's ready-made data (#/app/dashboard).
//
//   GET /v1/erp/dashboard/feed?storeId=X
//
// The dashboard used to wait for the browser to download a year of full sales,
// returns, purchases, expenses, deposits … (500 records a page, up to 20 000 per
// list) and the whole customer, vendor, product and employee lists, and only then
// work out every invoice's totals.  This endpoint does that work on the server:
// every document comes back already reduced to the numbers the dashboard reads
// (finance.js xo / totals.js computeTotals: taxable, VAT, net, paid, balance,
// cost, profit …), the master lists come back with only the fields the dashboard
// shows, and the response is kept as a snapshot (dashboard_snapshots.go), so
// opening the dashboard is one quick read.
//
// The browser adds the ready-made numbers up for the period picked
// (finance.js i0/mx/M4/…), so the figures are the ones it showed before: same
// records (the same one-year window in the store's timezone), same formulas.

import (
	"math"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// FeedWindowDays is the document window the web app loads (VITE_API_WINDOW_DAYS).
const FeedWindowDays = 365

// FeedPayment is one payment of a document (finance.js xo payments).
type FeedPayment struct {
	Amount float64 `json:"amount"`
	Method string  `json:"method"`
	D      string  `json:"d"`
}

// FeedProductDay is one product's sales and returns on one store day, summed over the
// lines of every sale / non-VAT sale (and return) of that day: what the dashboard's
// product figures (finance.js xx) read, instead of every document's lines.
type FeedProductDay struct {
	D         string  `json:"d"`
	ProductID string  `json:"productId"`
	Qty       float64 `json:"qty"`    // sold
	Rev       float64 `json:"rev"`    // Σ qty × (unitPrice − unitDiscount)
	Cost      float64 `json:"cost"`   // Σ qty × purchasePrice
	Lines     int     `json:"lines"`  // invoice lines (xx "orders")
	RetQty    float64 `json:"retQty"` // returned
	RetRev    float64 `json:"retRev"`
}

// FeedDoc is a sales/purchase-side document as finance.js xo() reduces it.
type FeedDoc struct {
	ID         string        `json:"id"`
	Code       string        `json:"code"`
	Date       string        `json:"date"`
	Hour       *int          `json:"hour"` // null when the date does not parse (JS NaN)
	Dow        *int          `json:"dow"`
	CustomerID string        `json:"customerId,omitempty"`
	VendorID   string        `json:"vendorId,omitempty"`
	NameEn     string        `json:"nameEn"`
	NameAr     string        `json:"nameAr"`
	Taxable    float64       `json:"taxable"`
	Vat        float64       `json:"vat"`
	Net        float64       `json:"net"`
	Paid       float64       `json:"paid"`
	Balance    float64       `json:"balance"`
	Status     string        `json:"status"`
	Profit     float64       `json:"profit"`
	Cost       float64       `json:"cost"`
	Qty        float64       `json:"qty"`
	Payments   []FeedPayment `json:"payments"`
	Zatca      string        `json:"zatca,omitempty"`
	OrderID    string        `json:"orderId,omitempty"`   // returns: the invoice returned
	Remarks    string        `json:"remarks,omitempty"`   // returns: shown as the reason
	DocStatus  string        `json:"docStatus,omitempty"` // quotations: draft/sent/accepted …
	OrderIDs   []string      `json:"orderIds,omitempty"`  // quotations: invoices made from it
}

// FeedTotals is totals.js computeTotals.
type FeedTotals struct {
	Taxable, Vat, Net, Paid, Balance, Profit, Cost, Qty float64
	Status                                              string
}

// ComputeTotals is totals.js computeTotals on a contract-shaped document.
func ComputeTotals(rec M) FeedTotals {
	vatPct := 15.0
	if v, ok := rec["vatPercent"]; ok && v != nil {
		vatPct = num(v)
	}
	var sub, disc, cost, qty float64
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		q := num(im["qty"])
		qty += q
		sub += q * num(im["unitPrice"])
		disc += q * num(im["unitDiscount"])
		cost += q * num(im["purchasePrice"])
	}
	discount := num(rec["discount"])
	taxable := round2(sub - disc - discount + num(rec["shipping"]))
	vat := round2(taxable * vatPct / 100)
	before := round2(taxable + vat)
	var rounding float64
	if boolv(rec["roundingAuto"]) {
		rounding = round2(jsRound(before*20)/20 - before)
	} else {
		rounding = round2(num(rec["rounding"]))
	}
	net := round2(before + rounding)
	paid := 0.0
	for _, p := range arr(rec["payments"]) {
		pm, _ := p.(M)
		paid += num(pm["amount"])
	}
	paid = round2(paid)
	bal := round2(net - paid - num(rec["cashDiscount"]))
	status := "not_paid"
	if bal <= 0.004 {
		status = "paid"
	} else if paid > 0 {
		status = "paid_partially"
	}
	if bal < 0 {
		bal = 0
	}
	return FeedTotals{Taxable: taxable, Vat: vat, Net: net, Paid: paid, Balance: bal, Status: status,
		Profit: round2(sub - disc - discount - cost), Cost: round2(cost), Qty: qty}
}

// jsRound is JavaScript's Math.round (halves go up).
func jsRound(x float64) float64 { return math.Floor(x + 0.5) }

// jsDate is JS String(v): the dashboard slices dates as strings.
func jsDate(v interface{}) string {
	if v == nil {
		return "undefined"
	}
	return str(v)
}

func first10(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// wallClock reads a contract date ("YYYY-MM-DDTHH:mm", store wall clock) the way
// the browser's new Date(s) does: hour and weekday of that wall-clock time.
func wallClock(s string) (hour, dow *int) {
	var t time.Time
	var err error
	switch {
	case len(s) >= 16:
		t, err = time.Parse(layoutDT, s[:16])
	case len(s) == 10:
		// new Date("YYYY-MM-DD") is UTC midnight; the app always sends a time
		t, err = time.Parse(layoutDay, s)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, nil
	}
	h, d := t.Hour(), int(t.Weekday())
	return &h, &d
}

// FeedDocOf reduces a contract-shaped document like finance.js xo().
func FeedDocOf(rec M) FeedDoc {
	t := ComputeTotals(rec)
	date := str(rec["date"])
	d := FeedDoc{ID: str(rec["id"]), Code: str(rec["code"]), Date: date,
		CustomerID: str(rec["customerId"]), VendorID: str(rec["vendorId"]),
		Taxable: t.Taxable, Vat: t.Vat, Net: t.Net, Paid: t.Paid, Balance: t.Balance, Status: t.Status,
		Profit: t.Profit, Cost: t.Cost, Qty: t.Qty, Payments: []FeedPayment{}}
	d.Hour, d.Dow = wallClock(date)
	d.NameEn = str(rec["customerName"])
	if d.NameEn == "" {
		d.NameEn = str(rec["vendorName"])
	}
	d.NameAr = str(rec["customerNameAr"])
	if d.NameAr == "" {
		d.NameAr = str(rec["vendorNameAr"])
	}
	for _, p := range arr(rec["payments"]) {
		pm, _ := p.(M)
		pd := str(pm["date"])
		if pd == "" {
			pd = date
		}
		d.Payments = append(d.Payments, FeedPayment{Amount: num(pm["amount"]), Method: str(pm["method"]), D: first10(pd)})
	}
	if z, ok := rec["zatca"].(M); ok {
		d.Zatca = str(z["status"])
	}
	return d
}

// FeedExpense is an expense as finance.js xD() reduces it.
type FeedExpense struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Date        string  `json:"date"`
	Hour        *int    `json:"hour"`
	Dow         *int    `json:"dow"`
	Amount      float64 `json:"amount"`
	Vat         float64 `json:"vat"`
	Total       float64 `json:"total"`
	CategoryID  string  `json:"categoryId,omitempty"`
	Method      string  `json:"method,omitempty"`
	Description string  `json:"description,omitempty"`
}

func feedExpenseOf(rec M) FeedExpense {
	e := FeedExpense{ID: str(rec["id"]), Code: str(rec["code"]), Date: jsDate(rec["date"]), Amount: num(rec["amount"]),
		Vat: num(rec["vatAmount"]), CategoryID: str(rec["categoryId"]), Method: str(rec["method"]), Description: str(rec["description"])}
	e.Total = round2(e.Amount + e.Vat)
	e.Hour, e.Dow = wallClock(e.Date)
	return e
}

// FeedSalary is a salary payment as finance.js xD() reduces it.
type FeedSalary struct {
	ID         string  `json:"id"`
	Code       string  `json:"code"`
	D          string  `json:"d"`
	Period     string  `json:"period"`
	Gross      float64 `json:"gross"`
	Net        float64 `json:"net"`
	Status     string  `json:"status"`
	EmployeeID string  `json:"employeeId"`
	NameEn     string  `json:"nameEn"`
	NameAr     string  `json:"nameAr"`
	Method     string  `json:"method,omitempty"`
}

func feedSalaryOf(rec M) FeedSalary {
	when := rec["paymentDate"]
	if str(when) == "" {
		when = rec["createdAt"]
	}
	return FeedSalary{ID: str(rec["id"]), Code: str(rec["code"]), D: first10(jsDate(when)), Period: str(rec["period"]),
		Gross: num(rec["totalEarnings"]), Net: num(rec["netSalary"]), Status: str(rec["status"]), EmployeeID: str(rec["employeeId"]),
		NameEn: str(rec["employeeName"]), NameAr: str(rec["employeeNameAr"]), Method: str(rec["method"])}
}

// FeedMoney is a deposit, withdrawal, capital, capital withdrawal or dividend.
type FeedMoney struct {
	D          string  `json:"d"`
	Amount     float64 `json:"amount"`
	Method     string  `json:"method,omitempty"`
	CustomerID string  `json:"customerId,omitempty"`
	OrderID    string  `json:"orderId,omitempty"` // deposits: the invoice it pays
}

func feedMoneyOf(rec M) FeedMoney {
	return FeedMoney{D: first10(jsDate(rec["date"])), Amount: num(rec["amount"]), Method: str(rec["method"]),
		CustomerID: str(rec["customerId"]), OrderID: str(rec["orderId"])}
}

// FeedRepairJob is a workshop job as finance.js xD() reduces it.
type FeedRepairJob struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	D           string  `json:"d"`
	Status      string  `json:"status"`
	Labour      float64 `json:"labour"`
	Additional  float64 `json:"additional"`
	PartsSale   float64 `json:"partsSale"`
	PartsCost   float64 `json:"partsCost"`
	SpareProfit float64 `json:"spareProfit"`
	CustomerID  string  `json:"customerId,omitempty"`
	NameEn      string  `json:"nameEn"`
	NameAr      string  `json:"nameAr"`
	Plate       string  `json:"plate,omitempty"`
	Make        string  `json:"make,omitempty"`
	Model       string  `json:"model,omitempty"`
}

func feedRepairJobOf(rec M) FeedRepairJob {
	var sale, cost float64
	for _, p := range arr(rec["parts"]) {
		pm, _ := p.(M)
		q := num(pm["qty"])
		sale += q * (num(pm["unitPrice"]) - num(pm["unitDiscount"]))
		cost += q * num(pm["purchasePrice"])
	}
	return FeedRepairJob{ID: str(rec["id"]), Code: str(rec["code"]), D: first10(jsDate(rec["date"])), Status: str(rec["status"]),
		Labour: num(rec["labour"]), Additional: num(rec["additional"]), PartsSale: round2(sale), PartsCost: round2(cost),
		SpareProfit: round2(sale - cost), CustomerID: str(rec["customerId"]), NameEn: str(rec["customerName"]),
		NameAr: str(rec["customerNameAr"]), Plate: str(rec["plate"]), Make: str(rec["make"]), Model: str(rec["model"])}
}

// pick keeps only the named fields of a master record (plus id).
func pick(rec M, keys []string) M {
	out := M{"id": rec["id"]}
	for _, k := range keys {
		if v, ok := rec[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

// Master-record fields the dashboards read or show.
var (
	feedCustomerKeys = []string{"code", "nameEn", "nameAr", "phone", "vatNo", "openingBalance", "openingBalanceType", "creditLimit", "category", "status"}
	feedVendorKeys   = []string{"code", "nameEn", "nameAr", "phone", "vatNo", "openingBalance", "openingBalanceType", "status"}
	feedProductKeys  = []string{"code", "nameEn", "nameAr", "partNo", "barcode", "unit", "categoryIds", "brandId", "isService", "isSet", "stock", "pricing"}
	feedEmployeeKeys = []string{"code", "nameEn", "nameAr", "phone", "jobTitle", "department", "status", "basicSalary", "housing",
		"transport", "otherAllowances", "openingBalance", "openingBalanceType", "advances"}
)

// DashboardFeed is the main dashboard's whole input.
type DashboardFeed struct {
	Sales              []FeedDoc        `json:"sales"`
	NonVAT             []FeedDoc        `json:"nonvatSales"`
	SalesReturns       []FeedDoc        `json:"salesReturns"`
	NonVATReturns      []FeedDoc        `json:"nonvatReturns"`
	Purchases          []FeedDoc        `json:"purchases"`
	PurchaseReturns    []FeedDoc        `json:"purchaseReturns"`
	Quotations         []FeedDoc        `json:"quotations"`
	Expenses           []FeedExpense    `json:"expenses"`
	Salaries           []FeedSalary     `json:"salaries"`
	Deposits           []FeedMoney      `json:"deposits"`
	Withdrawals        []FeedMoney      `json:"withdrawals"`
	Capitals           []FeedMoney      `json:"capitals"`
	CapitalWithdrawals []FeedMoney      `json:"capitalWithdrawals"`
	Dividends          []FeedMoney      `json:"dividends"`
	RepairJobs         []FeedRepairJob  `json:"repairJobs"`
	Customers          []M              `json:"customers"`
	Vendors            []M              `json:"vendors"`
	Products           []M              `json:"products"`
	Employees          []M              `json:"employees"`
	ProductDays        []FeedProductDay `json:"productDays"`
	Counts             M                `json:"counts"`

	pdIdx map[string]int
}

// NewDashboardFeed is an empty feed (every list present, never null).
func NewDashboardFeed() *DashboardFeed {
	return &DashboardFeed{Sales: []FeedDoc{}, NonVAT: []FeedDoc{}, SalesReturns: []FeedDoc{}, NonVATReturns: []FeedDoc{},
		Purchases: []FeedDoc{}, PurchaseReturns: []FeedDoc{}, Quotations: []FeedDoc{}, Expenses: []FeedExpense{},
		Salaries: []FeedSalary{}, Deposits: []FeedMoney{}, Withdrawals: []FeedMoney{}, Capitals: []FeedMoney{},
		CapitalWithdrawals: []FeedMoney{}, Dividends: []FeedMoney{}, RepairJobs: []FeedRepairJob{},
		Customers: []M{}, Vendors: []M{}, Products: []M{}, Employees: []M{}, ProductDays: []FeedProductDay{},
		pdIdx:  map[string]int{},
		Counts: M{"pendingPurchaseRequests": 0, "draftPurchaseOrders": 0}}
}

// Add puts one contract-shaped record of collection `name` into the feed.
func (f *DashboardFeed) Add(name string, rec M) {
	switch name {
	case "sales":
		f.Sales = append(f.Sales, FeedDocOf(rec))
		f.addLines(rec, false)
	case "nonvatSales":
		f.NonVAT = append(f.NonVAT, FeedDocOf(rec))
		f.addLines(rec, false)
	case "salesReturns", "nonvatReturns":
		f.addLines(rec, true)
		d := FeedDocOf(rec)
		d.OrderID, d.Remarks = str(rec["orderId"]), str(rec["remarks"])
		if name == "salesReturns" {
			f.SalesReturns = append(f.SalesReturns, d)
		} else {
			f.NonVATReturns = append(f.NonVATReturns, d)
		}
	case "purchases":
		f.Purchases = append(f.Purchases, FeedDocOf(rec))
	case "purchaseReturns":
		f.PurchaseReturns = append(f.PurchaseReturns, FeedDocOf(rec))
	case "quotations":
		d := FeedDocOf(rec)
		d.DocStatus, d.OrderIDs = str(rec["status"]), strs(rec["orderIds"])
		f.Quotations = append(f.Quotations, d)
	case "expenses":
		f.Expenses = append(f.Expenses, feedExpenseOf(rec))
	case "salaries":
		f.Salaries = append(f.Salaries, feedSalaryOf(rec))
	case "deposits":
		f.Deposits = append(f.Deposits, feedMoneyOf(rec))
	case "withdrawals":
		f.Withdrawals = append(f.Withdrawals, feedMoneyOf(rec))
	case "capitals":
		f.Capitals = append(f.Capitals, feedMoneyOf(rec))
	case "capitalWithdrawals":
		f.CapitalWithdrawals = append(f.CapitalWithdrawals, feedMoneyOf(rec))
	case "dividends":
		f.Dividends = append(f.Dividends, feedMoneyOf(rec))
	case "repairJobs":
		f.RepairJobs = append(f.RepairJobs, feedRepairJobOf(rec))
	case "customers":
		f.Customers = append(f.Customers, pick(rec, feedCustomerKeys))
	case "vendors":
		f.Vendors = append(f.Vendors, pick(rec, feedVendorKeys))
	case "products":
		f.Products = append(f.Products, pick(rec, feedProductKeys))
	case "employees":
		f.Employees = append(f.Employees, pick(rec, feedEmployeeKeys))
	case "purchaseRequests":
		if str(rec["status"]) == "pending" {
			f.Counts["pendingPurchaseRequests"] = intv(f.Counts["pendingPurchaseRequests"]) + 1
		}
	case "purchaseOrders":
		if str(rec["status"]) == "draft" {
			f.Counts["draftPurchaseOrders"] = intv(f.Counts["draftPurchaseOrders"]) + 1
		}
	}
}

// addLines sums a sale's (or return's) lines into its day's product rows.
func (f *DashboardFeed) addLines(rec M, ret bool) {
	if f.pdIdx == nil {
		f.pdIdx = map[string]int{}
	}
	day := first10(str(rec["date"]))
	for _, it := range arr(rec["items"]) {
		im, _ := it.(M)
		pid := str(im["productId"])
		k := day + "\x00" + pid
		i, ok := f.pdIdx[k]
		if !ok {
			i = len(f.ProductDays)
			f.pdIdx[k] = i
			f.ProductDays = append(f.ProductDays, FeedProductDay{D: day, ProductID: pid})
		}
		p := &f.ProductDays[i]
		q := num(im["qty"])
		line := q * (num(im["unitPrice"]) - num(im["unitDiscount"]))
		if ret {
			p.RetQty += q
			p.RetRev += line
		} else {
			p.Rev += line
			p.Cost += q * num(im["purchasePrice"])
			p.Qty += q
			p.Lines++
		}
	}
}

// mergeProductDays adds another part's day/product rows (sales and returns of the same
// day and product end up in one row).
func (f *DashboardFeed) mergeProductDays(rows []FeedProductDay) {
	if f.pdIdx == nil {
		f.pdIdx = map[string]int{}
	}
	for _, r := range rows {
		k := r.D + "\x00" + r.ProductID
		i, ok := f.pdIdx[k]
		if !ok {
			f.pdIdx[k] = len(f.ProductDays)
			f.ProductDays = append(f.ProductDays, r)
			continue
		}
		p := &f.ProductDays[i]
		p.Qty += r.Qty
		p.Rev += r.Rev
		p.Cost += r.Cost
		p.Lines += r.Lines
		p.RetQty += r.RetQty
		p.RetRev += r.RetRev
	}
}

// Collections the feed reads: windowed documents and whole master lists.
var (
	feedWindowed = []string{"sales", "nonvatSales", "salesReturns", "nonvatReturns", "purchases", "purchaseReturns", "quotations",
		"expenses", "salaries", "deposits", "withdrawals", "capitals", "capitalWithdrawals", "dividends", "repairJobs",
		"purchaseRequests", "purchaseOrders"}
	feedMasters = []string{"customers", "vendors", "products", "employees"}
)

// FeedWindowStart is the first moment of the window: 00:00 store time, FeedWindowDays
// before the store's today (the web app's storeDayOffset(-windowDays)).
func FeedWindowStart(now time.Time, loc *time.Location) (string, time.Time) {
	loc = orRiyadh(loc)
	n := now.In(loc)
	day := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -FeedWindowDays)
	return day.Format(layoutDay), day
}

// LoadDashboardFeed reads a store's records into a feed, the collections in parallel.
func LoadDashboardFeed(c *Ctx, storeHex string, from time.Time) (*DashboardFeed, error) {
	type part struct {
		name string
		f    *DashboardFeed
		err  error
	}
	names := append(append([]string{}, feedWindowed...), feedMasters...)
	out := make(chan part, len(names))
	for _, name := range names {
		go func(name string) {
			p := part{name: name, f: NewDashboardFeed()}
			b := resourceNamed(name)
			if b == nil {
				out <- p
				return
			}
			var extra bson.M
			if b.dateKey != "" && !contains(feedMasters, name) {
				extra = bson.M{b.dateKey: bson.M{"$gte": from.UTC()}}
			}
			p.err = b.each(c, storeHex, extra, func(rec M) { p.f.Add(name, rec) })
			out <- p
		}(name)
	}
	parts := map[string]*DashboardFeed{}
	var first error
	for range names {
		p := <-out
		if p.err != nil && first == nil {
			first = p.err
		}
		parts[p.name] = p.f
	}
	if first != nil {
		return nil, first
	}
	// merge in a fixed order (each part only filled its own lists)
	f := NewDashboardFeed()
	for _, name := range names {
		p := parts[name]
		f.Sales = append(f.Sales, p.Sales...)
		f.NonVAT = append(f.NonVAT, p.NonVAT...)
		f.SalesReturns = append(f.SalesReturns, p.SalesReturns...)
		f.NonVATReturns = append(f.NonVATReturns, p.NonVATReturns...)
		f.Purchases = append(f.Purchases, p.Purchases...)
		f.PurchaseReturns = append(f.PurchaseReturns, p.PurchaseReturns...)
		f.Quotations = append(f.Quotations, p.Quotations...)
		f.Expenses = append(f.Expenses, p.Expenses...)
		f.Salaries = append(f.Salaries, p.Salaries...)
		f.Deposits = append(f.Deposits, p.Deposits...)
		f.Withdrawals = append(f.Withdrawals, p.Withdrawals...)
		f.Capitals = append(f.Capitals, p.Capitals...)
		f.CapitalWithdrawals = append(f.CapitalWithdrawals, p.CapitalWithdrawals...)
		f.Dividends = append(f.Dividends, p.Dividends...)
		f.RepairJobs = append(f.RepairJobs, p.RepairJobs...)
		f.Customers = append(f.Customers, p.Customers...)
		f.Vendors = append(f.Vendors, p.Vendors...)
		f.Products = append(f.Products, p.Products...)
		f.Employees = append(f.Employees, p.Employees...)
		f.mergeProductDays(p.ProductDays)
		for k, v := range p.Counts {
			f.Counts[k] = intv(f.Counts[k]) + intv(v)
		}
	}
	return f, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func handleDashboardFeed(c *Ctx, w http.ResponseWriter, r *http.Request) error {
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
	loc := orRiyadh(c.storeLoc(storeHex))
	now := time.Now()
	fromDay, from := FeedWindowStart(now, loc)
	feed, err := LoadDashboardFeed(c, storeHex, from)
	if err != nil {
		return errInternal("Unable to load the dashboard data.")
	}
	writeJSON(w, http.StatusOK, M{
		"storeId":     storeHex,
		"timezone":    loc.String(),
		"today":       now.In(loc).Format(layoutDay),
		"from":        fromDay,
		"generatedAt": now.In(loc).Format("2006-01-02T15:04:05"),
		"feed":        feed,
	})
	return nil
}
