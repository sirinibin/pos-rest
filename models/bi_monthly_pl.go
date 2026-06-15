package models

import (
	"fmt"
	"time"
)

// MonthlyPLRecord holds the monthly P&L breakdown for one calendar month,
// calculated using the EXACT same formula as the frontend Profit/Loss Statement.
type MonthlyPLRecord struct {
	Period    string `json:"period"`     // "2026-05"
	Year      int    `json:"year"`
	Month     int    `json:"month"`
	MonthName string `json:"month_name"`

	// ── Revenue components (mirrors P&L Statement Revenue tooltip) ────────────
	GrossSales      float64 `json:"gross_sales"`       // sales net_total
	SalesReturns    float64 `json:"sales_returns"`     // sales_return net_total
	QtnSales        float64 `json:"qtn_sales"`         // quotation invoice net_total (if qtn_accounting)
	QtnSalesReturns float64 `json:"qtn_sales_returns"` // quotation_sales_return net_total (if qtn_accounting)
	Revenue         float64 `json:"revenue"`           // = GrossSales − SalesReturns ± Qtn

	// ── Expense components (mirrors P&L Statement Expense tooltip) ────────────
	TotalExpense                        float64 `json:"total_expense"`
	DepositPurchaseFund                 float64 `json:"deposit_purchase_fund"`                   // DPA mode only
	TotalPurchase                       float64 `json:"total_purchase"`
	TotalPurchaseReturn                 float64 `json:"total_purchase_return"`
	AccountedPurchase                   float64 `json:"accounted_purchase"`                      // DPA mode
	AccountedPurchaseReturn             float64 `json:"accounted_purchase_return"`               // DPA mode
	SalesCashDiscount                   float64 `json:"sales_cash_discount"`
	SalesReturnCashDiscount             float64 `json:"sales_return_cash_discount"`
	PurchaseCashDiscount                float64 `json:"purchase_cash_discount"`
	PurchaseReturnCashDiscount          float64 `json:"purchase_return_cash_discount"`
	AccountedPurchaseCashDiscount       float64 `json:"accounted_purchase_cash_discount"`
	AccountedPurchaseReturnCashDiscount float64 `json:"accounted_purchase_return_cash_discount"`
	SalesCommission                     float64 `json:"sales_commission"`
	SalesReturnCommission               float64 `json:"sales_return_commission"`
	QtnSalesCashDiscount                float64 `json:"qtn_sales_cash_discount"`
	QtnSalesReturnCashDiscount          float64 `json:"qtn_sales_return_cash_discount"`
	Expense                             float64 `json:"expense"` // final P&L expense

	// ── Net ───────────────────────────────────────────────────────────────────
	ProfitLoss float64 `json:"profit_loss"`
	IsProfit   bool    `json:"is_profit"`
}

var monthNames = [12]string{
	"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
}

// GetMonthlyPL computes monthly revenue and expense for the last `months`
// calendar months using the same P&L formula as the frontend Statistics page.
//
// Store settings flags respected:
//   - quotation_invoice_accounting  → include quotation invoice sales
//   - disable_purchases_on_accounts → switch purchase mode to accounted only
func (store *Store) GetMonthlyPL(months int) ([]MonthlyPLRecord, error) {
	if months <= 0 {
		months = 12
	}
	if months > 60 {
		months = 60
	}

	qtnAccounting := store.Settings.QuotationInvoiceAccounting
	dpaMode := store.Settings.DisablePurchasesOnAccounts

	// Apply the same timezone offset used by the P&L Statement so that
	// "January 2026" means Jan 1 00:00 → Feb 1 00:00 in the STORE'S local time,
	// converted to UTC — exactly as CountryTimezoneOffset + ConvertTimeZoneToUTC
	// does in the sales/expense/purchase controllers.
	tzOffset := CountryTimezoneOffset(store.CountryCode)

	now := time.Now().UTC()
	curYear, curMonth := now.Year(), int(now.Month())

	records := make([]MonthlyPLRecord, 0, months)

	for i := 0; i < months; i++ {
		y := curYear
		m := curMonth - i
		for m <= 0 {
			m += 12
			y--
		}

		// Local midnight → UTC  (same logic as ConvertTimeZoneToUTC in the controller)
		localStart := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
		localEnd   := time.Date(y, time.Month(m)+1, 1, 0, 0, 0, 0, time.UTC)

		start := ConvertTimeZoneToUTC(tzOffset, localStart)
		end   := ConvertTimeZoneToUTC(tzOffset, localEnd)

		base := map[string]interface{}{
			"deleted": map[string]interface{}{"$ne": true},
			"date":    map[string]interface{}{"$gte": start, "$lt": end},
		}

		// ── Sales ─────────────────────────────────────────────────────────────
		salesStats, _ := store.GetSalesStats(base)

		// ── Sales Returns ─────────────────────────────────────────────────────
		sretStats, _ := store.GetSalesReturnStats(base)

		// ── Expenses ──────────────────────────────────────────────────────────
		expStats, _ := store.GetExpenseStats(base)
		depStats, _ := store.GetCustomerDepositStats(base)

		// ── Purchases ─────────────────────────────────────────────────────────
		purStats, _ := store.GetPurchaseStats(base)

		accountedPurFilter := copyFilter(base)
		accountedPurFilter["enable_on_accounts"] = true
		acctPurStats, _ := store.GetPurchaseStats(accountedPurFilter)

		// ── Purchase Returns ──────────────────────────────────────────────────
		purRetStats, _ := store.GetPurchaseReturnStats(base)

		accountedPurRetFilter := copyFilter(base)
		accountedPurRetFilter["enable_on_accounts"] = true
		acctPurRetStats, _ := store.GetPurchaseReturnStats(accountedPurRetFilter)

		// ── Quotation Invoice / Returns (if enabled) ──────────────────────────
		var qtnInvoiceStats QuotationInvoiceStats
		var qtnSalesRetStats QuotationSalesReturnStats
		if qtnAccounting {
			qtnInvoiceStats, _ = store.GetQuotationInvoiceStats(base)
			qtnSalesRetStats, _ = store.GetQuotationSalesReturnStats(base)
		}

		// ── Revenue formula (exact match to frontend P&L Statement) ───────────
		revenue := salesStats.NetTotal - sretStats.NetTotal
		if qtnAccounting {
			revenue += qtnInvoiceStats.InvoiceNetTotal - qtnSalesRetStats.NetTotal
		}

		// ── Cash discount adjustment (exact match to frontend) ────────────────
		purCashDisc := purStats.CashDiscount
		purRetCashDisc := purRetStats.CashDiscount
		if dpaMode {
			purCashDisc = acctPurStats.CashDiscount
			purRetCashDisc = acctPurRetStats.CashDiscount
		}
		cashDiscAdj := salesStats.CashDiscount - sretStats.CashDiscount +
			purRetCashDisc - purCashDisc
		if qtnAccounting {
			cashDiscAdj += qtnInvoiceStats.InvoiceCashDiscount - qtnSalesRetStats.CashDiscount
		}

		// ── Commission adjustment ─────────────────────────────────────────────
		commissionAdj := salesStats.Commission - sretStats.Commission

		// ── Expense formula (exact match to frontend P&L Statement) ───────────
		var expense float64
		if dpaMode {
			expense = expStats.Total - depStats.PurchaseFund +
				acctPurStats.NetTotal - acctPurRetStats.NetTotal
		} else {
			expense = expStats.Total + purStats.NetTotal - purRetStats.NetTotal
		}
		expense += cashDiscAdj + commissionAdj

		profitLoss := revenue - expense

		period := fmt.Sprintf("%04d-%02d", y, m)
		records = append(records, MonthlyPLRecord{
			Period:    period,
			Year:      y,
			Month:     m,
			MonthName: monthNames[m-1],

			GrossSales:      RoundTo2Decimals(salesStats.NetTotal),
			SalesReturns:    RoundTo2Decimals(sretStats.NetTotal),
			QtnSales:        RoundTo2Decimals(qtnInvoiceStats.InvoiceNetTotal),
			QtnSalesReturns: RoundTo2Decimals(qtnSalesRetStats.NetTotal),
			Revenue:         RoundTo2Decimals(revenue),

			TotalExpense:                        RoundTo2Decimals(expStats.Total),
			DepositPurchaseFund:                 RoundTo2Decimals(depStats.PurchaseFund),
			TotalPurchase:                       RoundTo2Decimals(purStats.NetTotal),
			TotalPurchaseReturn:                 RoundTo2Decimals(purRetStats.NetTotal),
			AccountedPurchase:                   RoundTo2Decimals(acctPurStats.NetTotal),
			AccountedPurchaseReturn:             RoundTo2Decimals(acctPurRetStats.NetTotal),
			SalesCashDiscount:                   RoundTo2Decimals(salesStats.CashDiscount),
			SalesReturnCashDiscount:             RoundTo2Decimals(sretStats.CashDiscount),
			PurchaseCashDiscount:                RoundTo2Decimals(purStats.CashDiscount),
			PurchaseReturnCashDiscount:          RoundTo2Decimals(purRetStats.CashDiscount),
			AccountedPurchaseCashDiscount:       RoundTo2Decimals(acctPurStats.CashDiscount),
			AccountedPurchaseReturnCashDiscount: RoundTo2Decimals(acctPurRetStats.CashDiscount),
			SalesCommission:                     RoundTo2Decimals(salesStats.Commission),
			SalesReturnCommission:               RoundTo2Decimals(sretStats.Commission),
			QtnSalesCashDiscount:                RoundTo2Decimals(qtnInvoiceStats.InvoiceCashDiscount),
			QtnSalesReturnCashDiscount:          RoundTo2Decimals(qtnSalesRetStats.CashDiscount),
			Expense:                             RoundTo2Decimals(expense),

			ProfitLoss: RoundTo2Decimals(profitLoss),
			IsProfit:   profitLoss >= 0,
		})
	}

	// Return in chronological order (oldest first)
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

func copyFilter(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
