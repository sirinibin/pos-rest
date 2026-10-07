package erp

// dashboard_profit.go — the dashboard's "Net profit" / "Net loss".
//
//   GET /v1/erp/dashboard/net-profit?storeId=X&from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Same calculation as the profit card of the old business dashboard
// (reactjs-pos src/business_dashboard/charts/KPICards.js):
//
//   profit = total revenue (dashboard_revenue.go) − total expense (dashboard_expense.go)
//
// both VAT included and following the same store settings; the VAT split is at the
// store rate. Dates are whole days in the store's own timezone.

import (
	"net/http"
	"sync"

	"github.com/sirinibin/startpos/backend/models"
)

// NetProfitResult is what the profit card shows.
type NetProfitResult struct {
	Revenue          float64 `json:"revenue"` // total revenue, VAT included
	Expense          float64 `json:"expense"` // total expense, VAT included
	Profit           float64 `json:"profit"`  // revenue − expense; negative is a loss
	Vat              float64 `json:"vat"`
	ProfitWithoutVat float64 `json:"profitWithoutVat"`
	Profitable       bool    `json:"profitable"`
}

// DashboardNetProfit applies the old business dashboard's profit formula.
func DashboardNetProfit(revenue, expense, vatPercent float64) NetProfitResult {
	if vatPercent <= 0 {
		vatPercent = 15
	}
	p := revenue - expense
	vat := p * vatPercent / (100 + vatPercent)
	return NetProfitResult{
		Revenue:          models.RoundFloat(revenue, 2),
		Expense:          models.RoundFloat(expense, 2),
		Profit:           models.RoundFloat(p, 2),
		Vat:              models.RoundFloat(vat, 2),
		ProfitWithoutVat: models.RoundFloat(p-vat, 2),
		Profitable:       p >= 0,
	}
}

func handleDashboardNetProfit(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	store, dateRange, err := dashboardStore(c, r)
	if err != nil {
		return err
	}
	vat := dashboardVatPercent(store)
	rf := revenueFlags(store)
	ef := TotalExpenseFlags{
		DisablePurchasesOnAccounts: store.Settings.DisablePurchasesOnAccounts,
		SalesInQuotation:           store.Settings.EnableSalesInQuotation,
		EmployeeModule:             store.Settings.EnableEmployeeModule,
	}
	var (
		rin        RevenueInputs
		ein        TotalExpenseInputs
		rerr, eerr error
		wg         sync.WaitGroup
	)
	wg.Add(2)
	go func() { defer wg.Done(); rin, rerr = loadRevenueInputs(store, dateRange, rf) }()
	go func() { defer wg.Done(); ein, eerr = loadTotalExpenseInputs(store, dateRange, ef) }()
	wg.Wait()
	if rerr != nil || eerr != nil {
		return errInternal("Unable to calculate the net profit.")
	}
	rev := DashboardRevenue(rin, rf, vat)
	exp := TotalExpense(ein, ef, vat)
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, M{
		"storeId":    store.ID.Hex(),
		"from":       q.Get("from"),
		"to":         q.Get("to"),
		"vatPercent": vat,
		"revenue":    M{"flags": rf, "inputs": rin, "result": rev},
		"expense":    M{"flags": ef, "inputs": ein, "result": exp},
		"result":     DashboardNetProfit(rev.Total, exp.Total, vat),
	})
	return nil
}
