package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ProfitBreakdown carries the component figures used to compute Total / Monthly Profit.
type ProfitBreakdown struct {
	SalesRevenue       float64 `json:"sales_revenue"`
	SalesReturnRevenue float64 `json:"sales_return_revenue"`
	QtnRevenue         float64 `json:"qtn_revenue"`
	QtnReturnRevenue   float64 `json:"qtn_return_revenue"`
	TotalRevenue       float64 `json:"total_revenue"`
	ExpenseTotal       float64 `json:"expense_total"`
	PurchaseTotal      float64 `json:"purchase_total"`
	SalaryPaid         float64 `json:"salary_paid"`
	TotalExpenses      float64 `json:"total_expenses"`
}

// ProductProfitBreakdown carries the sales vs return breakdown for a product type.
type ProductProfitBreakdown struct {
	SalesProfit  float64 `json:"sales_profit"`
	ReturnProfit float64 `json:"return_profit"`
}

// MonthlyProfitEntry holds profit breakdown for a single month.
type MonthlyProfitEntry struct {
	Month            string  `json:"month"` // "YYYY-MM"
	LabourProfit     float64 `json:"labour_profit"`
	SpareProfit      float64 `json:"spare_profit"`
	AdditionalProfit float64 `json:"additional_profit"`
	TotalProfit      float64 `json:"total_profit"`
	Revenue          float64 `json:"revenue"`
}

// AutoMobileDashboard holds the KPI values for the workshop dashboard.
type AutoMobileDashboard struct {
	TotalProfit      float64         `json:"total_profit"`
	MonthlyProfit    float64         `json:"monthly_profit"`
	MonthName        string          `json:"month_name"`
	CounterCash      float64         `json:"counter_cash"`
	CashAccountID    string          `json:"cash_account_id"`
	BankCash         float64         `json:"bank_cash"`
	BankAccountID    string          `json:"bank_account_id"`
	SparePartsValue  SparePartsAsset `json:"spare_parts_value"`
	TotalCredit      float64         `json:"total_credit"`
	LabourProfit     float64         `json:"labour_profit"`
	SpareProfit      float64         `json:"spare_profit"`
	AdditionalProfit float64         `json:"additional_profit"`
	UnpaidBill       float64         `json:"unpaid_bill"`
	// SalaryBalance is signed: negative means the store owes the employees
	// money (unpaid/accrued salary), positive means employees owe the store.
	SalaryBalance      float64 `json:"salary_balance"`
	AdditionalExpense  float64 `json:"additional_expense"`
	MonthlyBreakdown   []MonthlyProfitEntry `json:"monthly_breakdown"`

	// Breakdown details for frontend tooltips.
	TotalProfitBreakdown   ProfitBreakdown        `json:"total_profit_breakdown"`
	MonthlyProfitBreakdown ProfitBreakdown        `json:"monthly_profit_breakdown"`
	LabourBreakdown        ProductProfitBreakdown `json:"labour_breakdown"`
	SpareBreakdown         ProductProfitBreakdown `json:"spare_breakdown"`
	AdditionalBreakdown    ProductProfitBreakdown `json:"additional_breakdown"`
}

type SparePartsAsset struct {
	PurchaseValue float64 `json:"purchase_value"`
	RetailValue   float64 `json:"retail_value"`
}

// GetAutoMobileDashboard computes all required KPIs for the workshop dashboard.
func (store *Store) GetAutoMobileDashboard(fromDate, toDate *time.Time) (AutoMobileDashboard, error) {
	dash := AutoMobileDashboard{}

	// ── Total Profit ─────────────────────────────────────────────────────────
	totalBD, err := store.getWorkshopProfitBreakdown(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	totalProfit := totalBD.TotalRevenue - totalBD.TotalExpenses
	dash.TotalProfit = RoundFloat(totalProfit, 2)
	dash.TotalProfitBreakdown = roundProfitBreakdown(totalBD)

	// ── Monthly Profit (current month, always unfiltered by date range) ────
	tzOffset := CountryTimezoneOffset(store.CountryCode)
	localNow := time.Now().UTC().Add(-1 * offsetDuration(tzOffset))
	dash.MonthName = localNow.Month().String()

	monthStart, nextMonthStart := store.currentMonthRangeUTC()
	monthBD, err := store.getWorkshopProfitBreakdown(&monthStart, &nextMonthStart)
	if err != nil {
		return dash, err
	}
	monthlyProfit := monthBD.TotalRevenue - monthBD.TotalExpenses
	dash.MonthlyProfit = RoundFloat(monthlyProfit, 2)
	dash.MonthlyProfitBreakdown = roundProfitBreakdown(monthBD)

	// ── Counter Cash & Bank Cash ──────────────────────────────────────────
	counterCash, cashID, err := store.getAccountBalanceWithID("CASH")
	if err != nil {
		return dash, err
	}
	dash.CounterCash = RoundFloat(counterCash, 2)
	dash.CashAccountID = cashID

	bankCash, bankID, err := store.getAccountBalanceWithID("BANK")
	if err != nil {
		return dash, err
	}
	dash.BankCash = RoundFloat(bankCash, 2)
	dash.BankAccountID = bankID

	// ── Spare Parts Inventory Value ───────────────────────────────────────
	productStats, err := store.GetProductStats(map[string]interface{}{
		"deleted":    bson.M{"$ne": true},
		"is_service": bson.M{"$ne": true},
		"product_stores." + store.ID.Hex() + ".stock": bson.M{"$gt": 0},
	}, store.ID, nil)
	if err != nil {
		return dash, err
	}
	dash.SparePartsValue = SparePartsAsset{
		PurchaseValue: RoundFloat(productStats.PurchaseStockValue, 2),
		RetailValue:   RoundFloat(productStats.RetailStockValue, 2),
	}

	// ── Customer Receivables ──────────────────────────────────────────────
	totalCredit, err := store.getCustomerReceivables()
	if err != nil {
		return dash, err
	}
	dash.TotalCredit = RoundFloat(totalCredit, 2)

	// ── Labour / Spare / Additional Profit ───────────────────────────────
	labourBD, spareBD, additionalBD, err := store.getProductTypeProfitBreakdowns(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.LabourProfit = RoundFloat(labourBD.SalesProfit-labourBD.ReturnProfit, 2)
	dash.SpareProfit = RoundFloat(spareBD.SalesProfit-spareBD.ReturnProfit, 2)
	dash.AdditionalProfit = RoundFloat(additionalBD.SalesProfit-additionalBD.ReturnProfit, 2)
	dash.LabourBreakdown = roundProductBreakdown(labourBD)
	dash.SpareBreakdown = roundProductBreakdown(spareBD)
	dash.AdditionalBreakdown = roundProductBreakdown(additionalBD)

	// ── Vendor Payables ───────────────────────────────────────────────────
	unpaidBill, err := store.getVendorPayables()
	if err != nil {
		return dash, err
	}
	dash.UnpaidBill = RoundFloat(unpaidBill, 2)

	// ── Salary Balance ────────────────────────────────────────────────────
	salaryBalance, err := store.getEmployeeSalaryBalance()
	if err != nil {
		return dash, err
	}
	dash.SalaryBalance = RoundFloat(salaryBalance, 2)

	// ── Additional Expense (Expenses only, no purchases / salary) ────────
	expenseFilter := map[string]interface{}{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	}
	dateFilter := bson.M{}
	if fromDate != nil {
		dateFilter["$gte"] = *fromDate
	}
	if toDate != nil {
		dateFilter["$lt"] = *toDate
	}
	if len(dateFilter) > 0 {
		expenseFilter["date"] = dateFilter
	}
	expenseStats, err := store.GetExpenseStats(expenseFilter)
	if err != nil {
		return dash, err
	}
	dash.AdditionalExpense = RoundFloat(expenseStats.Total, 2)

	// ── Monthly Breakdown (for charts) ────────────────────────────────────
	monthly, err := store.getMonthlyProfitBreakdown(12, fromDate, toDate)
	if err == nil {
		dash.MonthlyBreakdown = monthly
	}

	return dash, nil
}

func roundProfitBreakdown(bd ProfitBreakdown) ProfitBreakdown {
	return ProfitBreakdown{
		SalesRevenue:       RoundFloat(bd.SalesRevenue, 2),
		SalesReturnRevenue: RoundFloat(bd.SalesReturnRevenue, 2),
		QtnRevenue:         RoundFloat(bd.QtnRevenue, 2),
		QtnReturnRevenue:   RoundFloat(bd.QtnReturnRevenue, 2),
		TotalRevenue:       RoundFloat(bd.TotalRevenue, 2),
		ExpenseTotal:       RoundFloat(bd.ExpenseTotal, 2),
		PurchaseTotal:      RoundFloat(bd.PurchaseTotal, 2),
		SalaryPaid:         RoundFloat(bd.SalaryPaid, 2),
		TotalExpenses:      RoundFloat(bd.TotalExpenses, 2),
	}
}

func roundProductBreakdown(bd ProductProfitBreakdown) ProductProfitBreakdown {
	return ProductProfitBreakdown{
		SalesProfit:  RoundFloat(bd.SalesProfit, 2),
		ReturnProfit: RoundFloat(bd.ReturnProfit, 2),
	}
}

// getWorkshopProfitBreakdown computes revenue and expense components for the given period.
// Formula: Profit = Revenue(Sales-SalesReturn+QtnSales-QtnReturn) - Expenses(Expenses+Purchases+SalaryPaid)
func (store *Store) getWorkshopProfitBreakdown(fromDate, toDate *time.Time) (ProfitBreakdown, error) {
	bd := ProfitBreakdown{}

	baseFilter := map[string]interface{}{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	}
	dateFilter := bson.M{}
	if fromDate != nil {
		dateFilter["$gte"] = *fromDate
	}
	if toDate != nil {
		dateFilter["$lt"] = *toDate
	}
	if len(dateFilter) > 0 {
		baseFilter["date"] = dateFilter
	}

	salesStats, err := store.GetSalesStats(baseFilter)
	if err != nil {
		return bd, err
	}
	bd.SalesRevenue = salesStats.NetTotal

	salesReturnStats, err := store.GetSalesReturnStats(baseFilter)
	if err != nil {
		return bd, err
	}
	bd.SalesReturnRevenue = salesReturnStats.NetTotal

	if store.Settings.QuotationInvoiceAccounting {
		qtnStats, err := store.GetQuotationInvoiceStats(baseFilter)
		if err != nil {
			return bd, err
		}
		bd.QtnRevenue = qtnStats.InvoiceNetTotal

		qtnReturnStats, err := store.GetQuotationSalesReturnStats(baseFilter)
		if err != nil {
			return bd, err
		}
		bd.QtnReturnRevenue = qtnReturnStats.NetTotal
	}

	bd.TotalRevenue = bd.SalesRevenue - bd.SalesReturnRevenue + bd.QtnRevenue - bd.QtnReturnRevenue

	expenseStats, err := store.GetExpenseStats(baseFilter)
	if err != nil {
		return bd, err
	}
	bd.ExpenseTotal = expenseStats.Total

	purchaseStats, err := store.GetPurchaseStats(baseFilter)
	if err != nil {
		return bd, err
	}
	bd.PurchaseTotal = purchaseStats.NetTotal

	salaryPaid, err := store.getEmployeeSalaryPaid(fromDate, toDate)
	if err != nil {
		return bd, err
	}
	bd.SalaryPaid = salaryPaid

	bd.TotalExpenses = bd.ExpenseTotal + bd.PurchaseTotal + bd.SalaryPaid
	return bd, nil
}

// getEmployeeSalaryPaid sums actual salary payments made within the given period.
func (store *Store) getEmployeeSalaryPaid(fromDate, toDate *time.Time) (float64, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	}
	dateFilter := bson.M{}
	if fromDate != nil {
		dateFilter["$gte"] = *fromDate
	}
	if toDate != nil {
		dateFilter["$lt"] = *toDate
	}
	if len(dateFilter) > 0 {
		filter["date"] = dateFilter
	}

	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$amount"}}},
	}
	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)

	var res struct {
		Total float64 `bson:"total"`
	}
	if cur.Next(ctx) {
		cur.Decode(&res)
	}
	return res.Total, nil
}

// getProductTypeProfitBreakdowns aggregates Labour / Spare / Additional profits from
// both the sales (order) and sales_return collections.
// Profit per line = qty × (unit_price − purchase_unit_price).
// Labour  = products.name == "Labour Charge" (service)
// Spare   = products.is_service == false
// Additional = services with name != "Labour Charge"
func (store *Store) getProductTypeProfitBreakdowns(fromDate, toDate *time.Time) (
	labour, spare, additional ProductProfitBreakdown, err error,
) {
	dateFilter := bson.M{}
	if fromDate != nil {
		dateFilter["$gte"] = *fromDate
	}
	if toDate != nil {
		dateFilter["$lt"] = *toDate
	}

	salesPipeline := buildProductProfitPipeline(store.ID, dateFilter, "order")
	returnPipeline := buildProductProfitPipeline(store.ID, dateFilter, "sales_return")

	salesRes, err := runProductProfitPipeline(store, salesPipeline, "order")
	if err != nil {
		return
	}
	returnRes, err := runProductProfitPipeline(store, returnPipeline, "sales_return")
	if err != nil {
		return
	}

	labour = ProductProfitBreakdown{SalesProfit: salesRes.Labour, ReturnProfit: returnRes.Labour}
	spare = ProductProfitBreakdown{SalesProfit: salesRes.Spare, ReturnProfit: returnRes.Spare}
	additional = ProductProfitBreakdown{SalesProfit: salesRes.Additional, ReturnProfit: returnRes.Additional}
	return
}

type productProfitResult struct {
	Labour     float64
	Spare      float64
	Additional float64
}

func buildProductProfitPipeline(storeID interface{}, dateFilter bson.M, _ string) []bson.M {
	matchFilter := bson.M{
		"store_id": storeID,
		"deleted":  bson.M{"$ne": true},
		"status":   bson.M{"$ne": "cancelled"},
	}
	if len(dateFilter) > 0 {
		matchFilter["date"] = dateFilter
	}

	// profit_expr = products.quantity × (products.unit_price − products.purchase_unit_price)
	profitExpr := bson.M{"$multiply": bson.A{
		"$products.quantity",
		bson.M{"$subtract": bson.A{"$products.unit_price", "$products.purchase_unit_price"}},
	}}

	isLabour := bson.M{"$and": bson.A{
		bson.M{"$eq": bson.A{"$products.is_service", true}},
		bson.M{"$eq": bson.A{"$products.name", "Labour Charge"}},
	}}
	isSpare := bson.M{"$eq": bson.A{"$products.is_service", false}}
	isAdditional := bson.M{"$and": bson.A{
		bson.M{"$eq": bson.A{"$products.is_service", true}},
		bson.M{"$ne": bson.A{"$products.name", "Labour Charge"}},
	}}

	return []bson.M{
		{"$match": matchFilter},
		{"$unwind": "$products"},
		{"$group": bson.M{
			"_id": nil,
			"labour": bson.M{"$sum": bson.M{"$cond": bson.A{isLabour, profitExpr, 0}}},
			"spare":  bson.M{"$sum": bson.M{"$cond": bson.A{isSpare, profitExpr, 0}}},
			"additional": bson.M{"$sum": bson.M{"$cond": bson.A{isAdditional, profitExpr, 0}}},
		}},
	}
}

func runProductProfitPipeline(store *Store, pipeline []bson.M, collectionName string) (productProfitResult, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return productProfitResult{}, err
	}
	defer cur.Close(ctx)

	var raw struct {
		Labour     float64 `bson:"labour"`
		Spare      float64 `bson:"spare"`
		Additional float64 `bson:"additional"`
	}
	if cur.Next(ctx) {
		cur.Decode(&raw)
	}
	return productProfitResult{Labour: raw.Labour, Spare: raw.Spare, Additional: raw.Additional}, nil
}

// getMonthlyProfitBreakdown aggregates last N months of labour/spare/additional profit
// from both sales (order) and sales_return collections.
func (store *Store) getMonthlyProfitBreakdown(months int, fromDate *time.Time, toDate *time.Time) ([]MonthlyProfitEntry, error) {
	tzOffset := CountryTimezoneOffset(store.CountryCode)
	localNow := time.Now().UTC().Add(-1 * offsetDuration(tzOffset))

	var monthList []string
	dateFilter := bson.M{}

	if fromDate != nil {
		startLocal := fromDate.Add(-1 * offsetDuration(tzOffset))
		startMonth := time.Date(startLocal.Year(), startLocal.Month(), 1, 0, 0, 0, 0, time.UTC)
		var endMonth time.Time
		if toDate != nil {
			endLocal := toDate.Add(-1 * offsetDuration(tzOffset)).Add(-time.Nanosecond)
			endMonth = time.Date(endLocal.Year(), endLocal.Month(), 1, 0, 0, 0, 0, time.UTC)
		} else {
			endMonth = time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		cur := startMonth
		for !cur.After(endMonth) {
			monthList = append(monthList, cur.Format("2006-01"))
			cur = cur.AddDate(0, 1, 0)
		}
		dateFilter["$gte"] = *fromDate
		if toDate != nil {
			dateFilter["$lt"] = *toDate
		}
	} else {
		firstMonth := time.Date(localNow.Year(), localNow.Month()-time.Month(months-1), 1, 0, 0, 0, 0, time.UTC)
		fromUTC := ConvertTimeZoneToUTC(tzOffset, firstMonth)
		dateFilter["$gte"] = fromUTC
		for i := months - 1; i >= 0; i-- {
			m := localNow.AddDate(0, -i, 0)
			monthList = append(monthList, m.Format("2006-01"))
		}
	}

	salesMap, err := store.monthlyProductProfitMap(dateFilter, "order")
	if err != nil {
		return nil, err
	}
	returnMap, err := store.monthlyProductProfitMap(dateFilter, "sales_return")
	if err != nil {
		return nil, err
	}

	result := make([]MonthlyProfitEntry, 0, len(monthList))
	for _, key := range monthList {
		s := salesMap[key]
		r := returnMap[key]
		labourP := s.Labour - r.Labour
		spareP := s.Spare - r.Spare
		additionalP := s.Additional - r.Additional
		total := labourP + spareP + additionalP
		result = append(result, MonthlyProfitEntry{
			Month:            key,
			LabourProfit:     RoundFloat(labourP, 2),
			SpareProfit:      RoundFloat(spareP, 2),
			AdditionalProfit: RoundFloat(additionalP, 2),
			TotalProfit:      RoundFloat(total, 2),
			Revenue:          RoundFloat(s.Revenue-r.Revenue, 2),
		})
	}
	return result, nil
}

type monthlyProfitRaw struct {
	Labour     float64
	Spare      float64
	Additional float64
	Revenue    float64
}

func (store *Store) monthlyProductProfitMap(dateFilter bson.M, collectionName string) (map[string]monthlyProfitRaw, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	matchFilter := bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
		"status":   bson.M{"$ne": "cancelled"},
	}
	if len(dateFilter) > 0 {
		matchFilter["date"] = dateFilter
	}

	profitExpr := bson.M{"$multiply": bson.A{
		"$products.quantity",
		bson.M{"$subtract": bson.A{"$products.unit_price", "$products.purchase_unit_price"}},
	}}
	isLabour := bson.M{"$and": bson.A{
		bson.M{"$eq": bson.A{"$products.is_service", true}},
		bson.M{"$eq": bson.A{"$products.name", "Labour Charge"}},
	}}
	isSpare := bson.M{"$eq": bson.A{"$products.is_service", false}}
	isAdditional := bson.M{"$and": bson.A{
		bson.M{"$eq": bson.A{"$products.is_service", true}},
		bson.M{"$ne": bson.A{"$products.name", "Labour Charge"}},
	}}

	pipeline := []bson.M{
		{"$match": matchFilter},
		{"$unwind": "$products"},
		{"$group": bson.M{
			"_id": bson.M{"$dateToString": bson.M{"format": "%Y-%m", "date": "$date"}},
			"labour":     bson.M{"$sum": bson.M{"$cond": bson.A{isLabour, profitExpr, 0}}},
			"spare":      bson.M{"$sum": bson.M{"$cond": bson.A{isSpare, profitExpr, 0}}},
			"additional": bson.M{"$sum": bson.M{"$cond": bson.A{isAdditional, profitExpr, 0}}},
			"revenue":    bson.M{"$sum": "$net_total"},
		}},
		{"$sort": bson.M{"_id": 1}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	result := map[string]monthlyProfitRaw{}
	for cur.Next(ctx) {
		var row struct {
			Month      string  `bson:"_id"`
			Labour     float64 `bson:"labour"`
			Spare      float64 `bson:"spare"`
			Additional float64 `bson:"additional"`
			Revenue    float64 `bson:"revenue"`
		}
		if err := cur.Decode(&row); err == nil {
			result[row.Month] = monthlyProfitRaw{
				Labour: row.Labour, Spare: row.Spare,
				Additional: row.Additional, Revenue: row.Revenue,
			}
		}
	}
	return result, nil
}

func (store *Store) currentMonthRangeUTC() (time.Time, time.Time) {
	tzOffset := CountryTimezoneOffset(store.CountryCode)
	localNow := time.Now().UTC().Add(-1 * offsetDuration(tzOffset))
	monthStartLocal := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonthLocal := monthStartLocal.AddDate(0, 1, 0)
	return ConvertTimeZoneToUTC(tzOffset, monthStartLocal), ConvertTimeZoneToUTC(tzOffset, nextMonthLocal)
}

func offsetDuration(offset float64) time.Duration {
	hours := int(offset)
	minutes := int((offset - float64(hours)) * 60)
	return time.Hour*time.Duration(hours) + time.Minute*time.Duration(minutes)
}

// getAccountBalanceWithID returns the balance and hex ID for a named system account.
func (store *Store) getAccountBalanceWithID(name string) (balance float64, id string, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account := Account{}
	err = collection.FindOne(ctx, bson.M{
		"name":     name,
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	}).Decode(&account)
	if err == mongo.ErrNoDocuments {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	if err := account.CalculateBalance(nil, nil); err != nil {
		return 0, "", err
	}
	return account.Balance, account.ID.Hex(), nil
}

func (store *Store) getCustomerReceivables() (float64, error) {
	stats, err := store.GetCustomerStats(map[string]interface{}{
		"deleted": bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}
	return stats.CreditBalance, nil
}

func (store *Store) getVendorPayables() (float64, error) {
	stats, err := store.GetVendorStats(map[string]interface{}{
		"deleted": bson.M{"$ne": true},
	})
	if err != nil {
		return 0, err
	}
	return stats.PurchaseBalanceAmount, nil
}

// getEmployeeSalaryBalance returns the net signed balance across all employee
// liability accounts.  Negative means the store owes the employees money.
func (store *Store) getEmployeeSalaryBalance() (float64, error) {
	stats, err := store.GetAccountListStats(map[string]interface{}{
		"reference_model": "employee",
		"store_id":        store.ID,
		"deleted":         bson.M{"$ne": true},
		"open":            true,
	})
	if err != nil {
		return 0, err
	}
	return stats.DebitBalanceTotal - stats.CreditBalanceTotal, nil
}

func copyInterfaceFilter(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
