package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	// Non-VAT sales (populated when store.Settings.NonVATSales is true)
	NonVatSalesRevenue       float64 `json:"non_vat_sales_revenue"`
	NonVatSalesReturnRevenue float64 `json:"non_vat_sales_return_revenue"`
	NonVatNetRevenue         float64 `json:"non_vat_net_revenue"`
}

// VATBoxBreakdown holds the components for the VAT box KPI.
type VATBoxBreakdown struct {
	SalesVAT          float64 `json:"sales_vat"`
	SalesReturnVAT    float64 `json:"sales_return_vat"`
	PurchaseVAT       float64 `json:"purchase_vat"`
	PurchaseReturnVAT float64 `json:"purchase_return_vat"`
	ExpenseVAT        float64 `json:"expense_vat"`
	NetVAT            float64 `json:"net_vat"`
}

// ProductProfitBreakdown carries the sales vs return breakdown for a product type.
type ProductProfitBreakdown struct {
	SalesProfit        float64 `json:"sales_profit"`
	ReturnProfit       float64 `json:"return_profit"`
	NonVatSalesProfit  float64 `json:"non_vat_sales_profit"`
	NonVatReturnProfit float64 `json:"non_vat_return_profit"`
}

// EmployeeSalaryEntry is one row of the per-employee salary balance breakdown.
type EmployeeSalaryEntry struct {
	AccountID string  `json:"account_id"`
	Name      string  `json:"name"`
	Balance   float64 `json:"balance"`
	Direction string  `json:"direction"` // "owed_to_employee" | "employee_owes"
}

// ExpenseCategoryEntry is one row of the expense-category breakdown.
type ExpenseCategoryEntry struct {
	CategoryID string  `json:"category_id"`
	AccountID  string  `json:"account_id"`
	Name       string  `json:"name"`
	Total      float64 `json:"total"`
}

// CustomerCreditEntry is one row of the customer receivable breakdown.
type CustomerCreditEntry struct {
	CustomerID    string  `json:"customer_id"`
	AccountID     string  `json:"account_id"`
	Name          string  `json:"name"`
	CreditBalance float64 `json:"credit_balance"`
}

type VendorPayableEntry struct {
	VendorID        string  `json:"vendor_id"`
	AccountID       string  `json:"account_id"`
	Name            string  `json:"name"`
	PurchaseBalance float64 `json:"purchase_balance"`
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

	// VAT box (populated when store.Settings.EnableVATBox is true)
	VATBox        VATBoxBreakdown `json:"vat_box"`
	VATBoxMonthly VATBoxBreakdown `json:"vat_box_monthly"`

	// Breakdown details for frontend tooltips.
	TotalProfitBreakdown    ProfitBreakdown        `json:"total_profit_breakdown"`
	MonthlyProfitBreakdown  ProfitBreakdown        `json:"monthly_profit_breakdown"`
	LabourBreakdown         ProductProfitBreakdown `json:"labour_breakdown"`
	SpareBreakdown          ProductProfitBreakdown `json:"spare_breakdown"`
	AdditionalBreakdown     ProductProfitBreakdown `json:"additional_breakdown"`
	EmployeeBreakdown        []EmployeeSalaryEntry  `json:"employee_breakdown"`
	ExpenseCategoryBreakdown []ExpenseCategoryEntry `json:"expense_category_breakdown"`
	CustomerCreditBreakdown  []CustomerCreditEntry  `json:"customer_credit_breakdown"`
	VendorPayablesBreakdown  []VendorPayableEntry   `json:"vendor_payables_breakdown"`
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
	totalCredit, err := store.getCustomerReceivables(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.TotalCredit = RoundFloat(totalCredit, 2)

	// ── Labour / Spare / Additional Profit ───────────────────────────────
	labourBD, spareBD, additionalBD, err := store.getProductTypeProfitBreakdowns(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.LabourProfit = RoundFloat(labourBD.SalesProfit-labourBD.ReturnProfit+labourBD.NonVatSalesProfit-labourBD.NonVatReturnProfit, 2)
	dash.SpareProfit = RoundFloat(spareBD.SalesProfit-spareBD.ReturnProfit, 2)
	dash.AdditionalProfit = RoundFloat(additionalBD.SalesProfit-additionalBD.ReturnProfit, 2)
	dash.LabourBreakdown = roundProductBreakdown(labourBD)
	dash.SpareBreakdown = roundProductBreakdown(spareBD)
	dash.AdditionalBreakdown = roundProductBreakdown(additionalBD)

	// ── Vendor Payables ───────────────────────────────────────────────────
	unpaidBill, err := store.getVendorPayables(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.UnpaidBill = RoundFloat(unpaidBill, 2)

	// ── Salary Balance ────────────────────────────────────────────────────
	salaryBalance, err := store.getEmployeeSalaryBalance(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.SalaryBalance = RoundFloat(salaryBalance, 2)
	if empBD, bdErr := store.getEmployeeSalaryBreakdown(fromDate, toDate); bdErr == nil {
		dash.EmployeeBreakdown = empBD
	}

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
	if catBD, bdErr := store.getExpenseCategoryBreakdown(expenseFilter); bdErr == nil {
		dash.ExpenseCategoryBreakdown = catBD
	}

	// ── Customer Credit Breakdown ─────────────────────────────────────────
	if custBD, bdErr := store.getCustomerCreditBreakdown(fromDate, toDate); bdErr == nil {
		dash.CustomerCreditBreakdown = custBD
	}

	// ── Vendor Payables Breakdown ─────────────────────────────────────────
	if vendBD, bdErr := store.getVendorPayablesBreakdown(fromDate, toDate); bdErr == nil {
		dash.VendorPayablesBreakdown = vendBD
	}

	// ── Monthly Breakdown (for charts) ────────────────────────────────────
	// ── VAT Box ───────────────────────────────────────────────────────────────
	if store.Settings.EnableVATBox {
		if vatBox, vErr := store.getVATBoxBreakdown(fromDate, toDate); vErr == nil {
			dash.VATBox = roundVATBox(vatBox)
		}
		if vatBoxM, vErr := store.getVATBoxBreakdown(&monthStart, &nextMonthStart); vErr == nil {
			dash.VATBoxMonthly = roundVATBox(vatBoxM)
		}
	}

	monthly, err := store.getMonthlyProfitBreakdown(12, fromDate, toDate)
	if err == nil {
		dash.MonthlyBreakdown = monthly
	}

	return dash, nil
}

func roundProfitBreakdown(bd ProfitBreakdown) ProfitBreakdown {
	return ProfitBreakdown{
		SalesRevenue:             RoundFloat(bd.SalesRevenue, 2),
		SalesReturnRevenue:       RoundFloat(bd.SalesReturnRevenue, 2),
		QtnRevenue:               RoundFloat(bd.QtnRevenue, 2),
		QtnReturnRevenue:         RoundFloat(bd.QtnReturnRevenue, 2),
		TotalRevenue:             RoundFloat(bd.TotalRevenue, 2),
		ExpenseTotal:             RoundFloat(bd.ExpenseTotal, 2),
		PurchaseTotal:            RoundFloat(bd.PurchaseTotal, 2),
		SalaryPaid:               RoundFloat(bd.SalaryPaid, 2),
		TotalExpenses:            RoundFloat(bd.TotalExpenses, 2),
		NonVatSalesRevenue:       RoundFloat(bd.NonVatSalesRevenue, 2),
		NonVatSalesReturnRevenue: RoundFloat(bd.NonVatSalesReturnRevenue, 2),
		NonVatNetRevenue:         RoundFloat(bd.NonVatNetRevenue, 2),
	}
}

func roundVATBox(bd VATBoxBreakdown) VATBoxBreakdown {
	return VATBoxBreakdown{
		SalesVAT:          RoundFloat(bd.SalesVAT, 2),
		SalesReturnVAT:    RoundFloat(bd.SalesReturnVAT, 2),
		PurchaseVAT:       RoundFloat(bd.PurchaseVAT, 2),
		PurchaseReturnVAT: RoundFloat(bd.PurchaseReturnVAT, 2),
		ExpenseVAT:        RoundFloat(bd.ExpenseVAT, 2),
		NetVAT:            RoundFloat(bd.NetVAT, 2),
	}
}

func roundProductBreakdown(bd ProductProfitBreakdown) ProductProfitBreakdown {
	return ProductProfitBreakdown{
		SalesProfit:        RoundFloat(bd.SalesProfit, 2),
		ReturnProfit:       RoundFloat(bd.ReturnProfit, 2),
		NonVatSalesProfit:  RoundFloat(bd.NonVatSalesProfit, 2),
		NonVatReturnProfit: RoundFloat(bd.NonVatReturnProfit, 2),
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

	if store.Settings.NonVATSales {
		nvSales, _ := store.sumCollectionField("non_vat_sales", "net_total", baseFilter)
		nvReturn, _ := store.sumCollectionField("non_vat_sales_return", "net_total", baseFilter)
		bd.NonVatSalesRevenue = nvSales
		bd.NonVatSalesReturnRevenue = nvReturn
		bd.NonVatNetRevenue = nvSales - nvReturn
	}

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

	nvSalesPipeline := buildProductProfitPipeline(store.ID, dateFilter, "non_vat_sales")
	nvReturnPipeline := buildProductProfitPipeline(store.ID, dateFilter, "non_vat_sales_return")
	nvSalesRes, err := runProductProfitPipeline(store, nvSalesPipeline, "non_vat_sales")
	if err != nil {
		return
	}
	nvReturnRes, err := runProductProfitPipeline(store, nvReturnPipeline, "non_vat_sales_return")
	if err != nil {
		return
	}

	labour = ProductProfitBreakdown{
		SalesProfit:        salesRes.Labour,
		ReturnProfit:       returnRes.Labour,
		NonVatSalesProfit:  nvSalesRes.Labour,
		NonVatReturnProfit: nvReturnRes.Labour,
	}
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

func (store *Store) getCustomerReceivables(fromDate, toDate *time.Time) (float64, error) {
	if fromDate == nil && toDate == nil {
		stats, err := store.GetCustomerStats(map[string]interface{}{
			"deleted": bson.M{"$ne": true},
		})
		if err != nil {
			return 0, err
		}
		return stats.CreditBalance, nil
	}
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dateF := bson.M{}
	if fromDate != nil {
		dateF["$gte"] = *fromDate
	}
	if toDate != nil {
		dateF["$lt"] = *toDate
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			"store_id":       store.ID,
			"deleted":        bson.M{"$ne": true},
			"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
			"status":         bson.M{"$ne": "cancelled"},
			"date":           dateF,
		}},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$balance_amount"}}},
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

func (store *Store) getVendorPayables(fromDate, toDate *time.Time) (float64, error) {
	if fromDate == nil && toDate == nil {
		stats, err := store.GetVendorStats(map[string]interface{}{
			"deleted": bson.M{"$ne": true},
		})
		if err != nil {
			return 0, err
		}
		return stats.PurchaseBalanceAmount, nil
	}
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dateF := bson.M{}
	if fromDate != nil {
		dateF["$gte"] = *fromDate
	}
	if toDate != nil {
		dateF["$lt"] = *toDate
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			"store_id":       store.ID,
			"deleted":        bson.M{"$ne": true},
			"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
			"date":           dateF,
		}},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$balance_amount"}}},
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

// getEmployeeSalaryBalance returns the net signed balance across all employee
// liability accounts.  Negative means the store owes the employees money.
// When a date range is provided, returns total salary paid in that period instead.
func (store *Store) getEmployeeSalaryBalance(fromDate, toDate *time.Time) (float64, error) {
	if fromDate == nil && toDate == nil {
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
	return store.getEmployeeSalaryPaid(fromDate, toDate)
}

// getEmployeeSalaryBreakdown returns per-employee salary rows for the tooltip.
// Without dates: current account balances. With dates: salary paid per employee in the period.
func (store *Store) getEmployeeSalaryBreakdown(fromDate, toDate *time.Time) ([]EmployeeSalaryEntry, error) {
	if fromDate == nil && toDate == nil {
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		filter := bson.M{
			"reference_model": "employee",
			"store_id":        store.ID,
			"deleted":         bson.M{"$ne": true},
			"open":            true,
			"balance":         bson.M{"$gt": 0},
		}
		opts := options.Find().
			SetSort(bson.M{"balance": -1}).
			SetProjection(bson.M{"name": 1, "balance": 1, "debit_or_credit_balance": 1})
		cur, err := collection.Find(ctx, filter, opts)
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		var entries []EmployeeSalaryEntry
		for cur.Next(ctx) {
			var acc Account
			if err := cur.Decode(&acc); err != nil {
				continue
			}
			direction := "employee_owes"
			if acc.DebitOrCreditBalance == "credit_balance" {
				direction = "owed_to_employee"
			}
			entries = append(entries, EmployeeSalaryEntry{
				AccountID: acc.ID.Hex(),
				Name:      acc.Name,
				Balance:   RoundFloat(acc.Balance, 2),
				Direction: direction,
			})
		}
		return entries, nil
	}

	// Date-filtered: group salary payments by employee
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("employee_salary_payment")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dateF := bson.M{}
	if fromDate != nil {
		dateF["$gte"] = *fromDate
	}
	if toDate != nil {
		dateF["$lt"] = *toDate
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			"store_id": store.ID,
			"deleted":  bson.M{"$ne": true},
			"date":     dateF,
		}},
		{"$group": bson.M{
			"_id":   "$employee_id",
			"name":  bson.M{"$first": "$employee_name"},
			"total": bson.M{"$sum": "$amount"},
		}},
		{"$match": bson.M{"total": bson.M{"$gt": 0}}},
		{"$sort": bson.M{"total": -1}},
	}
	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var entries []EmployeeSalaryEntry
	for cur.Next(ctx) {
		var row struct {
			ID    *primitive.ObjectID `bson:"_id"`
			Name  string              `bson:"name"`
			Total float64             `bson:"total"`
		}
		if err := cur.Decode(&row); err != nil {
			continue
		}
		empID := ""
		if row.ID != nil {
			empID = row.ID.Hex()
		}
		entries = append(entries, EmployeeSalaryEntry{
			AccountID: empID,
			Name:      row.Name,
			Balance:   RoundFloat(row.Total, 2),
			Direction: "paid_in_period",
		})
	}
	return entries, nil
}

// getExpenseCategoryBreakdown groups additional-expense amounts by category, with account ID for balance sheet links.
func (store *Store) getExpenseCategoryBreakdown(filter map[string]interface{}) ([]ExpenseCategoryEntry, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("expense")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Unwind by category_id (with array index to fetch matching name), then group.
	pipeline := []bson.M{
		{"$match": filter},
		{"$unwind": bson.M{"path": "$category_id", "includeArrayIndex": "catIdx", "preserveNullAndEmptyArrays": true}},
		{"$addFields": bson.M{"cat_name": bson.M{"$arrayElemAt": []interface{}{"$category_name", "$catIdx"}}}},
		{"$group": bson.M{
			"_id":   "$category_id",
			"name":  bson.M{"$first": "$cat_name"},
			"total": bson.M{"$sum": "$amount"},
		}},
		{"$sort": bson.M{"total": -1}},
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}

	type aggRow struct {
		ID    *primitive.ObjectID `bson:"_id"`
		Name  *string             `bson:"name"`
		Total float64             `bson:"total"`
	}
	var rows []aggRow
	for cur.Next(ctx) {
		var row aggRow
		if err := cur.Decode(&row); err != nil {
			continue
		}
		rows = append(rows, row)
	}
	cur.Close(ctx)

	// Collect category IDs to look up their accounts in one query.
	catAccountMap := map[string]string{}
	var catIDs []primitive.ObjectID
	for _, r := range rows {
		if r.ID != nil {
			catIDs = append(catIDs, *r.ID)
		}
	}
	if len(catIDs) > 0 {
		accCol := db.GetDB("store_" + store.ID.Hex()).Collection("account")
		accCtx, accCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer accCancel()
		accCur, accErr := accCol.Find(accCtx, bson.M{
			"reference_model": "expense_category",
			"reference_id":    bson.M{"$in": catIDs},
			"store_id":        store.ID,
			"deleted":         bson.M{"$ne": true},
		}, options.Find().SetProjection(bson.M{"_id": 1, "reference_id": 1}))
		if accErr == nil {
			for accCur.Next(accCtx) {
				var acc struct {
					ID          primitive.ObjectID  `bson:"_id"`
					ReferenceID *primitive.ObjectID `bson:"reference_id"`
				}
				if err := accCur.Decode(&acc); err != nil {
					continue
				}
				if acc.ReferenceID != nil {
					catAccountMap[acc.ReferenceID.Hex()] = acc.ID.Hex()
				}
			}
			accCur.Close(accCtx)
		}
	}

	var entries []ExpenseCategoryEntry
	for _, row := range rows {
		name := "Uncategorized"
		if row.Name != nil && *row.Name != "" {
			name = *row.Name
		}
		entry := ExpenseCategoryEntry{
			Name:  name,
			Total: RoundFloat(row.Total, 2),
		}
		if row.ID != nil {
			entry.CategoryID = row.ID.Hex()
			if accID, ok := catAccountMap[row.ID.Hex()]; ok {
				entry.AccountID = accID
			}
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// getCustomerCreditBreakdown returns the top customers by outstanding credit balance.
// Without dates: reads denormalized credit_balance from customer docs.
// With dates: aggregates balance_amount from order collection for the period.
func (store *Store) getCustomerCreditBreakdown(fromDate, toDate *time.Time) ([]CustomerCreditEntry, error) {
	type idName struct {
		id   primitive.ObjectID
		name string
		bal  float64
	}

	if fromDate == nil && toDate == nil {
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("customer")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cur, err := collection.Find(ctx, bson.M{
			"store_id":       store.ID,
			"deleted":        bson.M{"$ne": true},
			"credit_balance": bson.M{"$gt": 0},
		}, options.Find().
			SetSort(bson.M{"credit_balance": -1}).
			SetLimit(20).
			SetProjection(bson.M{"name": 1, "credit_balance": 1}))
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		var rows []idName
		for cur.Next(ctx) {
			var c Customer
			if err := cur.Decode(&c); err != nil {
				continue
			}
			rows = append(rows, idName{id: c.ID, name: c.Name, bal: RoundFloat(c.CreditBalance, 2)})
		}
		var custIDs []primitive.ObjectID
		for _, r := range rows {
			custIDs = append(custIDs, r.id)
		}
		refMap := lookupAccountIDsByReferenceIDsSlice(store, "customer", custIDs)
		var entries []CustomerCreditEntry
		for _, r := range rows {
			entries = append(entries, CustomerCreditEntry{
				CustomerID:    r.id.Hex(),
				AccountID:     refMap[r.id.Hex()],
				Name:          r.name,
				CreditBalance: r.bal,
			})
		}
		return entries, nil
	}

	// Date-filtered: aggregate from order collection
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dateF := bson.M{}
	if fromDate != nil {
		dateF["$gte"] = *fromDate
	}
	if toDate != nil {
		dateF["$lt"] = *toDate
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			"store_id":       store.ID,
			"deleted":        bson.M{"$ne": true},
			"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
			"status":         bson.M{"$ne": "cancelled"},
			"date":           dateF,
		}},
		{"$group": bson.M{
			"_id":   "$customer_id",
			"name":  bson.M{"$first": "$customer_name"},
			"total": bson.M{"$sum": "$balance_amount"},
		}},
		{"$match": bson.M{"total": bson.M{"$gt": 0}}},
		{"$sort": bson.M{"total": -1}},
		{"$limit": 20},
	}
	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var rows []idName
	for cur.Next(ctx) {
		var row struct {
			ID    *primitive.ObjectID `bson:"_id"`
			Name  string              `bson:"name"`
			Total float64             `bson:"total"`
		}
		if err := cur.Decode(&row); err != nil {
			continue
		}
		if row.ID == nil {
			continue
		}
		rows = append(rows, idName{id: *row.ID, name: row.Name, bal: RoundFloat(row.Total, 2)})
	}
	var custIDs []primitive.ObjectID
	for _, r := range rows {
		custIDs = append(custIDs, r.id)
	}
	refMap := lookupAccountIDsByReferenceIDsSlice(store, "customer", custIDs)
	var entries []CustomerCreditEntry
	for _, r := range rows {
		entries = append(entries, CustomerCreditEntry{
			CustomerID:    r.id.Hex(),
			AccountID:     refMap[r.id.Hex()],
			Name:          r.name,
			CreditBalance: r.bal,
		})
	}
	return entries, nil
}

// getVendorPayablesBreakdown returns vendors with outstanding purchase balance, sorted desc.
// Without dates: reads denormalized purchase_balance_amount from vendor docs.
// With dates: aggregates balance_amount from purchase collection for the period.
func (store *Store) getVendorPayablesBreakdown(fromDate, toDate *time.Time) ([]VendorPayableEntry, error) {
	type idVendor struct {
		id   primitive.ObjectID
		name string
		bal  float64
	}

	if fromDate == nil && toDate == nil {
		collection := db.GetDB("store_" + store.ID.Hex()).Collection("vendor")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		storeBalField := "stores." + store.ID.Hex() + ".purchase_balance_amount"
		cur, err := collection.Find(ctx, bson.M{
			"deleted":      bson.M{"$ne": true},
			storeBalField: bson.M{"$gt": 0},
		}, options.Find().
			SetSort(bson.M{storeBalField: -1}).
			SetLimit(20).
			SetProjection(bson.M{"name": 1, storeBalField: 1}))
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		var rows []idVendor
		for cur.Next(ctx) {
			var v Vendor
			if err := cur.Decode(&v); err != nil {
				continue
			}
			storeData, ok := v.Stores[store.ID.Hex()]
			if !ok {
				continue
			}
			rows = append(rows, idVendor{id: v.ID, name: v.Name, bal: RoundFloat(storeData.PurchaseBalanceAmount, 2)})
		}
		var vendIDs []primitive.ObjectID
		for _, r := range rows {
			vendIDs = append(vendIDs, r.id)
		}
		refMap := lookupAccountIDsByReferenceIDsSlice(store, "vendor", vendIDs)
		var entries []VendorPayableEntry
		for _, r := range rows {
			entries = append(entries, VendorPayableEntry{
				VendorID:        r.id.Hex(),
				AccountID:       refMap[r.id.Hex()],
				Name:            r.name,
				PurchaseBalance: r.bal,
			})
		}
		return entries, nil
	}

	// Date-filtered: aggregate from purchase collection
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("purchase")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dateF := bson.M{}
	if fromDate != nil {
		dateF["$gte"] = *fromDate
	}
	if toDate != nil {
		dateF["$lt"] = *toDate
	}
	pipeline := []bson.M{
		{"$match": bson.M{
			"store_id":       store.ID,
			"deleted":        bson.M{"$ne": true},
			"payment_status": bson.M{"$in": bson.A{"not_paid", "paid_partially"}},
			"date":           dateF,
		}},
		{"$group": bson.M{
			"_id":   "$vendor_id",
			"name":  bson.M{"$first": "$vendor_name"},
			"total": bson.M{"$sum": "$balance_amount"},
		}},
		{"$match": bson.M{"total": bson.M{"$gt": 0}}},
		{"$sort": bson.M{"total": -1}},
		{"$limit": 20},
	}
	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var rows []idVendor
	for cur.Next(ctx) {
		var row struct {
			ID    *primitive.ObjectID `bson:"_id"`
			Name  string              `bson:"name"`
			Total float64             `bson:"total"`
		}
		if err := cur.Decode(&row); err != nil {
			continue
		}
		if row.ID == nil {
			continue
		}
		rows = append(rows, idVendor{id: *row.ID, name: row.Name, bal: RoundFloat(row.Total, 2)})
	}
	var vendIDs []primitive.ObjectID
	for _, r := range rows {
		vendIDs = append(vendIDs, r.id)
	}
	refMap := lookupAccountIDsByReferenceIDsSlice(store, "vendor", vendIDs)
	var entries []VendorPayableEntry
	for _, r := range rows {
		entries = append(entries, VendorPayableEntry{
			VendorID:        r.id.Hex(),
			AccountID:       refMap[r.id.Hex()],
			Name:            r.name,
			PurchaseBalance: r.bal,
		})
	}
	return entries, nil
}

// lookupAccountIDsByReferenceIDs queries the account collection once and returns
// a map of referenceID.Hex() → accountID.Hex() for the given reference model.
func lookupAccountIDsByReferenceIDsSlice(store *Store, model string, ids []primitive.ObjectID) map[string]string {
	result := map[string]string{}
	if len(ids) == 0 {
		return result
	}
	col := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := col.Find(ctx, bson.M{
		"reference_model": model,
		"reference_id":    bson.M{"$in": ids},
		"store_id":        store.ID,
		"deleted":         bson.M{"$ne": true},
	}, options.Find().SetProjection(bson.M{"_id": 1, "reference_id": 1}))
	if err != nil {
		return result
	}
	defer cur.Close(ctx)
	for cur.Next(ctx) {
		var acc struct {
			ID          primitive.ObjectID  `bson:"_id"`
			ReferenceID *primitive.ObjectID `bson:"reference_id"`
		}
		if err := cur.Decode(&acc); err != nil {
			continue
		}
		if acc.ReferenceID != nil {
			result[acc.ReferenceID.Hex()] = acc.ID.Hex()
		}
	}
	return result
}

func copyInterfaceFilter(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

// sumCollectionField aggregates $sum of a field in a collection using the given filter.
func (store *Store) sumCollectionField(collectionName, field string, filter map[string]interface{}) (float64, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection(collectionName)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pipeline := []bson.M{
		{"$match": filter},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$" + field}}},
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

// getVATBoxBreakdown computes VAT box components for the given date range.
// Formula: Net VAT = SalesVAT - SalesReturnVAT - PurchaseVAT + PurchaseReturnVAT + ExpenseVAT
// Purchase VAT considers only accounted purchases when DisablePurchasesOnAccounts is true.
func (store *Store) getVATBoxBreakdown(fromDate, toDate *time.Time) (VATBoxBreakdown, error) {
	bd := VATBoxBreakdown{}

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

	var err error
	if bd.SalesVAT, err = store.sumCollectionField("order", "vat_price", baseFilter); err != nil {
		return bd, err
	}
	if bd.SalesReturnVAT, err = store.sumCollectionField("salesreturn", "vat_price", baseFilter); err != nil {
		return bd, err
	}

	if store.Settings.DisablePurchasesOnAccounts {
		acctFilter := copyInterfaceFilter(baseFilter)
		acctFilter["enable_on_accounts"] = true
		if bd.PurchaseVAT, err = store.sumCollectionField("purchase", "vat_price", acctFilter); err != nil {
			return bd, err
		}
		if bd.PurchaseReturnVAT, err = store.sumCollectionField("purchasereturn", "vat_price", acctFilter); err != nil {
			return bd, err
		}
	} else {
		if bd.PurchaseVAT, err = store.sumCollectionField("purchase", "vat_price", baseFilter); err != nil {
			return bd, err
		}
		if bd.PurchaseReturnVAT, err = store.sumCollectionField("purchasereturn", "vat_price", baseFilter); err != nil {
			return bd, err
		}
	}

	expenseFilter := copyInterfaceFilter(baseFilter)
	expenseFilter["vendor_id"] = bson.M{"$exists": true, "$ne": nil}
	expenseFilter["vendor_invoice_no"] = bson.M{"$exists": true, "$ne": ""}
	if bd.ExpenseVAT, err = store.sumCollectionField("expense", "vat", expenseFilter); err != nil {
		return bd, err
	}

	bd.NetVAT = bd.SalesVAT - bd.SalesReturnVAT - bd.PurchaseVAT + bd.PurchaseReturnVAT + bd.ExpenseVAT
	return bd, nil
}
