package controller

// mcp_pnl.go — Profit/Loss Statement endpoint under /v1/mcp/
//
//   GET /v1/mcp/profit-loss  — get_profit_loss_statement
//
// Replicates the exact formula from the frontend stats/index.js and
// the Python MCP server.py get_profit_loss_statement handler.
// All API calls are done in parallel using goroutines.

import (
	"net/http"
	"sync"
)

// MCPProfitLossStatement handles GET /v1/mcp/profit-loss
// Query params: store_id (required), date_str | from_date+to_date, vat_percent
func MCPProfitLossStatement(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}

	// Read vat_percent override from query (default to store's rate)
	vatPercent := store.VatPercent
	if vatPercent == 0 {
		vatPercent = 15 // Saudi Arabia default
	}
	qtnInvoiceAccounting := store.Settings.QuotationInvoiceAccounting
	disablePurchasesOnAccounts := store.Settings.DisablePurchasesOnAccounts

	// Build filter from date params (reuse existing search logic)
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildSalesCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	filter := criterias.SearchBy

	// Parallel data fetch
	type results struct {
		salesTotal     float64
		salesCD        float64
		salesComm      float64
		srTotal        float64
		srCD           float64
		srComm         float64
		expTotal       float64
		purTotal       float64
		purCD          float64
		purAccounted   float64
		purAccountedCD float64
		prrTotal       float64
		prrCD          float64
		prrAccounted   float64
		prrAccountedCD float64
		depPurchFund   float64
		qtnInvTotal    float64
		qtnInvCD       float64
		qtnSRTotal     float64
		qtnSRCD        float64
	}

	var res results
	var mu sync.Mutex
	var wg sync.WaitGroup
	errors := make([]string, 0)

	addErr := func(msg string) {
		mu.Lock()
		errors = append(errors, msg)
		mu.Unlock()
	}

	// Sales stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		stats, err := store.GetSalesStats(filter)
		if err != nil {
			addErr("sales: " + err.Error())
			return
		}
		mu.Lock()
		res.salesTotal = stats.NetTotal
		res.salesCD = stats.CashDiscount
		res.salesComm = stats.Commission
		mu.Unlock()
	}()

	// Sales return stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		srFilter := copyFilter(filter)
		stats, err := store.GetSalesReturnStats(srFilter)
		if err != nil {
			addErr("sales-return: " + err.Error())
			return
		}
		mu.Lock()
		res.srTotal = stats.NetTotal
		res.srCD = stats.CashDiscount
		res.srComm = stats.Commission
		mu.Unlock()
	}()

	// Expense stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		expFilter := copyFilter(filter)
		stats, err := store.GetExpenseStats(expFilter)
		if err != nil {
			addErr("expense: " + err.Error())
			return
		}
		mu.Lock()
		res.expTotal = stats.Total
		mu.Unlock()
	}()

	// Purchase stats (all + accounted-only)
	wg.Add(1)
	go func() {
		defer wg.Done()
		purFilter := copyFilter(filter)
		stats, err := store.GetPurchaseStats(purFilter)
		if err != nil {
			addErr("purchase: " + err.Error())
			return
		}
		// Accounted subset (enable_on_accounts=true)
		accFilter := copyFilter(filter)
		accFilter["enable_on_accounts"] = true
		accStats, _ := store.GetPurchaseStats(accFilter)
		mu.Lock()
		res.purTotal = stats.NetTotal
		res.purCD = stats.CashDiscount
		res.purAccounted = accStats.NetTotal
		res.purAccountedCD = accStats.CashDiscount
		mu.Unlock()
	}()

	// Purchase return stats (all + accounted-only)
	wg.Add(1)
	go func() {
		defer wg.Done()
		prrFilter := copyFilter(filter)
		stats, err := store.GetPurchaseReturnStats(prrFilter)
		if err != nil {
			addErr("purchase-return: " + err.Error())
			return
		}
		accFilter := copyFilter(filter)
		accFilter["enable_on_accounts"] = true
		accStats, _ := store.GetPurchaseReturnStats(accFilter)
		mu.Lock()
		res.prrTotal = stats.NetTotal
		res.prrCD = stats.CashDiscount
		res.prrAccounted = accStats.NetTotal
		res.prrAccountedCD = accStats.CashDiscount
		mu.Unlock()
	}()

	// Conditional: quotation invoice accounting
	if qtnInvoiceAccounting {
		wg.Add(1)
		go func() {
			defer wg.Done()
			qtnFilter := copyFilter(filter)
			stats, err := store.GetQuotationInvoiceStats(qtnFilter)
			if err != nil {
				addErr("quotation-invoice: " + err.Error())
				return
			}
			mu.Lock()
			res.qtnInvTotal = stats.InvoiceNetTotal
			res.qtnInvCD = stats.InvoiceCashDiscount
			mu.Unlock()
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			qtnSRFilter := copyFilter(filter)
			stats, err := store.GetQuotationSalesReturnStats(qtnSRFilter)
			if err != nil {
				addErr("quotation-sales-return: " + err.Error())
				return
			}
			mu.Lock()
			res.qtnSRTotal = stats.NetTotal
			res.qtnSRCD = stats.CashDiscount
			mu.Unlock()
		}()
	}

	// Conditional: disable purchases on accounts — need customer deposit purchase_fund
	if disablePurchasesOnAccounts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			depFilter := copyFilter(filter)
			stats, err := store.GetCustomerDepositStats(depFilter)
			if err != nil {
				addErr("customer-deposit: " + err.Error())
				return
			}
			mu.Lock()
			res.depPurchFund = stats.PurchaseFund
			mu.Unlock()
		}()
	}

	wg.Wait()

	if len(errors) > 0 {
		mcpWriteError(w, "partial errors: "+errors[0], http.StatusInternalServerError)
		return
	}

	// ── P&L formula — mirrors stats/index.js exactly ──────────────────────────
	revenueWithVAT := res.salesTotal - res.srTotal
	if qtnInvoiceAccounting {
		revenueWithVAT += res.qtnInvTotal - res.qtnSRTotal
	}

	// Cash discount adjustment
	var cdPurchase, cdPurReturn float64
	if disablePurchasesOnAccounts {
		cdPurchase = res.purAccountedCD
		cdPurReturn = res.prrAccountedCD
	} else {
		cdPurchase = res.purCD
		cdPurReturn = res.prrCD
	}
	cdQtn := 0.0
	cdQtnSR := 0.0
	if qtnInvoiceAccounting {
		cdQtn = res.qtnInvCD
		cdQtnSR = res.qtnSRCD
	}
	cashDiscountAdj := res.salesCD - res.srCD + cdPurReturn - cdPurchase + cdQtn - cdQtnSR
	commissionAdj := res.salesComm - res.srComm

	var expenseWithVAT float64
	if disablePurchasesOnAccounts {
		expenseWithVAT = res.expTotal - res.depPurchFund + res.purAccounted - res.prrAccounted + cashDiscountAdj + commissionAdj
	} else {
		expenseWithVAT = res.expTotal + res.purTotal - res.prrTotal + cashDiscountAdj + commissionAdj
	}

	plWithVAT := revenueWithVAT - expenseWithVAT
	vatInPL := plWithVAT * vatPercent / (100 + vatPercent)
	plWithoutVAT := plWithVAT - vatInPL

	mcpWriteJSON(w, map[string]interface{}{
		"store_id":                   store.ID.Hex(),
		"store_name":                 store.Name,
		"vat_percent":                vatPercent,
		"quotation_invoice_accounting":     qtnInvoiceAccounting,
		"disable_purchases_on_accounts":    disablePurchasesOnAccounts,
		"revenue_with_vat":           round2(revenueWithVAT),
		"expense_with_vat":           round2(expenseWithVAT),
		"profit_loss_with_vat":       round2(plWithVAT),
		"vat_in_profit_loss":         round2(vatInPL),
		"profit_loss_without_vat":    round2(plWithoutVAT),
		"is_profit":                  plWithVAT >= 0,
		"breakdown": map[string]interface{}{
			"gross_sales":              round2(res.salesTotal),
			"sales_returns":            round2(res.srTotal),
			"qtn_invoice_sales":        round2(res.qtnInvTotal),
			"qtn_sales_returns":        round2(res.qtnSRTotal),
			"total_expense":            round2(res.expTotal),
			"total_purchase":           round2(res.purTotal),
			"total_purchase_return":    round2(res.prrTotal),
			"accounted_purchase":       round2(res.purAccounted),
			"accounted_purchase_return": round2(res.prrAccounted),
			"deposit_purchase_fund":    round2(res.depPurchFund),
			"cash_discount_adj":        round2(cashDiscountAdj),
			"commission_adj":           round2(commissionAdj),
			"sales_cd":                 round2(res.salesCD),
			"sales_return_cd":          round2(res.srCD),
			"purchase_cd":              round2(cdPurchase),
			"purchase_return_cd":       round2(cdPurReturn),
			"qtn_invoice_cd":           round2(res.qtnInvCD),
			"qtn_sr_cd":                round2(res.qtnSRCD),
		},
	})
}

// copyFilter returns a shallow copy of a filter map.
func copyFilter(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// round2 rounds a float to 2 decimal places.
func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
