package models

import (
	"context"
	"time"

	"github.com/sirinibin/startpos/backend/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// AutoMobileDashboard holds the 12 KPI values for the workshop dashboard.
type AutoMobileDashboard struct {
	TotalProfit      float64         `json:"total_profit"`
	MonthlyProfit    float64         `json:"monthly_profit"`
	CounterCash      float64         `json:"counter_cash"`
	BankCash         float64         `json:"bank_cash"`
	SparePartsValue  SparePartsAsset `json:"spare_parts_value"`
	TotalCredit      float64         `json:"total_credit"`
	LabourProfit     float64         `json:"labour_profit"`
	SpareProfit      float64         `json:"spare_profit"`
	AdditionalProfit float64         `json:"additional_profit"`
	UnpaidBill       float64         `json:"unpaid_bill"`
	// SalaryBalance is signed: negative means the store owes the employees
	// money (unpaid/accrued salary), positive means employees owe the store
	// money on net (e.g. from over-advances). Zero means fully settled.
	SalaryBalance     float64 `json:"salary_balance"`
	AdditionalExpense float64 `json:"additional_expense"`
}

type SparePartsAsset struct {
	PurchaseValue float64 `json:"purchase_value"`
	RetailValue   float64 `json:"retail_value"`
}

// GetAutoMobileDashboard computes all required KPIs for the workshop dashboard.
func (store *Store) GetAutoMobileDashboard(fromDate, toDate *time.Time) (AutoMobileDashboard, error) {
	dash := AutoMobileDashboard{}

	totalProfit, err := store.getWorkshopProfitLoss(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.TotalProfit = RoundFloat(totalProfit, 2)

	monthStart, nextMonthStart := store.currentMonthRangeUTC()
	monthlyProfit, err := store.getWorkshopProfitLoss(&monthStart, &nextMonthStart)
	if err != nil {
		return dash, err
	}
	dash.MonthlyProfit = RoundFloat(monthlyProfit, 2)

	counterCash, err := store.getAccountBalance("CASH")
	if err != nil {
		return dash, err
	}
	dash.CounterCash = RoundFloat(counterCash, 2)

	bankCash, err := store.getAccountBalance("BANK")
	if err != nil {
		return dash, err
	}
	dash.BankCash = RoundFloat(bankCash, 2)

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

	totalCredit, err := store.getCustomerReceivables()
	if err != nil {
		return dash, err
	}
	dash.TotalCredit = RoundFloat(totalCredit, 2)

	labourProfit, spareProfit, additionalProfit, err := store.getProductTypeProfits(fromDate, toDate)
	if err != nil {
		return dash, err
	}
	dash.LabourProfit = RoundFloat(labourProfit, 2)
	dash.SpareProfit = RoundFloat(spareProfit, 2)
	dash.AdditionalProfit = RoundFloat(additionalProfit, 2)

	unpaidBill, err := store.getVendorPayables()
	if err != nil {
		return dash, err
	}
	dash.UnpaidBill = RoundFloat(unpaidBill, 2)

	salaryBalance, err := store.getEmployeeSalaryBalance()
	if err != nil {
		return dash, err
	}
	dash.SalaryBalance = RoundFloat(salaryBalance, 2)

	expenseStats, err := store.GetExpenseStats(map[string]interface{}{
		"deleted": bson.M{"$ne": true},
	})
	if err != nil {
		return dash, err
	}
	dash.AdditionalExpense = RoundFloat(expenseStats.Total, 2)

	return dash, nil
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

func (store *Store) getWorkshopProfitLoss(fromDate, toDate *time.Time) (float64, error) {
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
		return 0, err
	}
	salesReturnStats, err := store.GetSalesReturnStats(baseFilter)
	if err != nil {
		return 0, err
	}
	expenseStats, err := store.GetExpenseStats(baseFilter)
	if err != nil {
		return 0, err
	}
	purchaseStats, err := store.GetPurchaseStats(baseFilter)
	if err != nil {
		return 0, err
	}
	purchaseReturnStats, err := store.GetPurchaseReturnStats(baseFilter)
	if err != nil {
		return 0, err
	}

	accountedPurchaseFilter := copyInterfaceFilter(baseFilter)
	accountedPurchaseFilter["enable_on_accounts"] = true
	accountedPurchaseStats, err := store.GetPurchaseStats(accountedPurchaseFilter)
	if err != nil {
		return 0, err
	}
	accountedPurchaseReturnStats, err := store.GetPurchaseReturnStats(accountedPurchaseFilter)
	if err != nil {
		return 0, err
	}

	qtnInvoiceStats := QuotationInvoiceStats{}
	qtnSalesReturnStats := QuotationSalesReturnStats{}
	if store.Settings.QuotationInvoiceAccounting {
		qtnInvoiceStats, err = store.GetQuotationInvoiceStats(baseFilter)
		if err != nil {
			return 0, err
		}
		qtnSalesReturnStats, err = store.GetQuotationSalesReturnStats(baseFilter)
		if err != nil {
			return 0, err
		}
	}

	revenue := salesStats.NetTotal - salesReturnStats.NetTotal
	if store.Settings.QuotationInvoiceAccounting {
		revenue += qtnInvoiceStats.InvoiceNetTotal - qtnSalesReturnStats.NetTotal
	}

	purchaseCashDiscount := purchaseStats.CashDiscount
	purchaseReturnCashDiscount := purchaseReturnStats.CashDiscount
	if store.Settings.DisablePurchasesOnAccounts {
		purchaseCashDiscount = accountedPurchaseStats.CashDiscount
		purchaseReturnCashDiscount = accountedPurchaseReturnStats.CashDiscount
	}

	cashDiscountAdj := salesStats.CashDiscount - salesReturnStats.CashDiscount + purchaseReturnCashDiscount - purchaseCashDiscount
	if store.Settings.QuotationInvoiceAccounting {
		cashDiscountAdj += qtnInvoiceStats.InvoiceCashDiscount - qtnSalesReturnStats.CashDiscount
	}

	commissionAdj := salesStats.Commission - salesReturnStats.Commission

	expense := expenseStats.Total + purchaseStats.NetTotal - purchaseReturnStats.NetTotal
	if store.Settings.DisablePurchasesOnAccounts {
		depositStats, err := store.GetCustomerDepositStats(baseFilter)
		if err != nil {
			return 0, err
		}
		expense = expenseStats.Total - depositStats.PurchaseFund + accountedPurchaseStats.NetTotal - accountedPurchaseReturnStats.NetTotal
	}
	expense += cashDiscountAdj + commissionAdj

	return revenue - expense, nil
}

func copyInterfaceFilter(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

// getAccountBalance returns the current calculated balance for a named system account.
func (store *Store) getAccountBalance(name string) (float64, error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("account")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account := Account{}
	err := collection.FindOne(ctx, bson.M{
		"name":     name,
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
	}).Decode(&account)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if err := account.CalculateBalance(nil, nil); err != nil {
		return 0, err
	}
	return account.Balance, nil
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
// liability accounts, using the same sign convention as the employee balance
// sheet/ledger (see models/posting.go CreatePostings and
// frontend/src/utils/employeeBalance.js): negative means the store owes the
// employees money (liability/credit_balance accounts), positive means the
// employees owe the store money on net (asset/debit_balance accounts, e.g.
// from over-advances).
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

// getProductTypeProfits aggregates labour/spare/additional profit from sales order products.
func (store *Store) getProductTypeProfits(fromDate, toDate *time.Time) (labour, spare, additional float64, err error) {
	collection := db.GetDB("store_" + store.ID.Hex()).Collection("order")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	matchFilter := bson.M{
		"store_id": store.ID,
		"deleted":  bson.M{"$ne": true},
		"status":   bson.M{"$ne": "cancelled"},
	}
	dateFilter := bson.M{}
	if fromDate != nil {
		dateFilter["$gte"] = *fromDate
	}
	if toDate != nil {
		dateFilter["$lt"] = *toDate
	}
	if len(dateFilter) > 0 {
		matchFilter["date"] = dateFilter
	}

	pipeline := []bson.M{
		{"$match": matchFilter},
		{"$unwind": "$products"},
		{"$lookup": bson.M{
			"from": "product",
			"let":  bson.M{"product_id": "$products.product_id"},
			"pipeline": []bson.M{
				{"$match": bson.M{"$expr": bson.M{"$eq": bson.A{"$_id", "$$product_id"}}}},
				{"$project": bson.M{"is_service": 1, "service_category_name": 1}},
			},
			"as": "product_doc",
		}},
		{"$unwind": bson.M{"path": "$product_doc", "preserveNullAndEmptyArrays": true}},
		{"$addFields": bson.M{
			"line_is_service": bson.M{"$ifNull": bson.A{
				"$products.is_service",
				bson.M{"$ifNull": bson.A{"$product_doc.is_service", false}},
			}},
			"line_service_category_name": bson.M{"$ifNull": bson.A{"$product_doc.service_category_name", ""}},
		}},
		{"$group": bson.M{
			"_id": nil,
			"labour_profit": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$line_is_service", true}},
					bson.M{"$regexMatch": bson.M{
						"input":   "$line_service_category_name",
						"regex":   "labour",
						"options": "i",
					}},
				}},
				"$products.profit",
				0,
			}}},
			"spare_profit": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$ne": bson.A{"$line_is_service", true}},
				"$products.profit",
				0,
			}}},
			"additional_profit": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$and": bson.A{
					bson.M{"$eq": bson.A{"$line_is_service", true}},
					bson.M{"$not": bson.A{bson.M{"$regexMatch": bson.M{
						"input":   "$line_service_category_name",
						"regex":   "labour",
						"options": "i",
					}}}},
				}},
				"$products.profit",
				0,
			}}},
		}},
	}

	type result struct {
		LabourProfit     float64 `bson:"labour_profit"`
		SpareProfit      float64 `bson:"spare_profit"`
		AdditionalProfit float64 `bson:"additional_profit"`
	}

	cur, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, 0, err
	}
	defer cur.Close(ctx)

	r := result{}
	if cur.Next(ctx) {
		if err := cur.Decode(&r); err != nil {
			return 0, 0, 0, err
		}
	}
	return r.LabourProfit, r.SpareProfit, r.AdditionalProfit, nil
}
