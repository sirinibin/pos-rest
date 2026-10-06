package erp

// dashboard_expense.go — the dashboard's "Total expense" figure.
//
//   GET /v1/erp/dashboard/total-expense?storeId=X&from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Same calculation as the "Expense" card of the old business dashboard
// (reactjs-pos src/stats/index.js profitLossExpenseNum, backed by the legacy
// /v1/order, /v1/expense, /v1/purchase … stats), including the store-setting
// dependent parts:
//
//   disable_purchases_on_accounts → only purchases flagged "on accounts" count,
//     and purchase-fund money received back through customer deposits is
//     taken off;
//   enable_sales_in_quotation     → quotation invoices' cash discounts count;
//   enable_employee_module        → salary paid in the period is added.
//
// Dates are whole days in the store's own timezone (CountryTimezoneOffset).

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
)

// TotalExpenseFlags are the store settings that change the calculation.
type TotalExpenseFlags struct {
	DisablePurchasesOnAccounts bool `json:"disablePurchasesOnAccounts"`
	SalesInQuotation           bool `json:"salesInQuotation"`
	EmployeeModule             bool `json:"employeeModule"`
}

// TotalExpenseInputs are the period totals the formula is made of (VAT included).
type TotalExpenseInputs struct {
	Expense                         float64 `json:"expense"`
	Purchase                        float64 `json:"purchase"`
	PurchaseReturn                  float64 `json:"purchaseReturn"`
	AccountedPurchase               float64 `json:"accountedPurchase"`
	AccountedPurchaseReturn         float64 `json:"accountedPurchaseReturn"`
	DepositPurchaseFund             float64 `json:"depositPurchaseFund"`
	SalesCashDiscount               float64 `json:"salesCashDiscount"`
	SalesReturnCashDiscount         float64 `json:"salesReturnCashDiscount"`
	PurchaseCashDiscount            float64 `json:"purchaseCashDiscount"`
	PurchaseReturnCashDiscount      float64 `json:"purchaseReturnCashDiscount"`
	AccountedPurchaseCashDiscount   float64 `json:"accountedPurchaseCashDiscount"`
	AccountedPurchaseReturnCashDisc float64 `json:"accountedPurchaseReturnCashDiscount"`
	QtnSalesCashDiscount            float64 `json:"qtnSalesCashDiscount"`
	QtnSalesReturnCashDiscount      float64 `json:"qtnSalesReturnCashDiscount"`
	SalesCommission                 float64 `json:"salesCommission"`
	SalesReturnCommission           float64 `json:"salesReturnCommission"`
	SalaryPaid                      float64 `json:"salaryPaid"`
}

// TotalExpenseResult is the formula's outcome plus its intermediate parts.
type TotalExpenseResult struct {
	Purchases       float64 `json:"purchases"`    // purchases − returns (accounted ones in on-account mode)
	CashDiscount    float64 `json:"cashDiscount"` // net cash-discount adjustment
	Commission      float64 `json:"commission"`   // sales − sales-return commission
	Salary          float64 `json:"salary"`       // 0 unless the employee module is on
	Total           float64 `json:"total"`        // with VAT
	Vat             float64 `json:"vat"`          // VAT portion of Total
	TotalWithoutVat float64 `json:"totalWithoutVat"`
}

// TotalExpense applies the old business dashboard's expense formula.
func TotalExpense(in TotalExpenseInputs, f TotalExpenseFlags, vatPercent float64) TotalExpenseResult {
	var purchases, purCD, purRetCD float64
	if f.DisablePurchasesOnAccounts {
		purchases = in.AccountedPurchase - in.AccountedPurchaseReturn - in.DepositPurchaseFund
		purCD = in.AccountedPurchaseCashDiscount
		purRetCD = in.AccountedPurchaseReturnCashDisc
	} else {
		purchases = in.Purchase - in.PurchaseReturn
		purCD = in.PurchaseCashDiscount
		purRetCD = in.PurchaseReturnCashDiscount
	}
	cd := in.SalesCashDiscount - in.SalesReturnCashDiscount + purRetCD - purCD
	if f.SalesInQuotation {
		cd += in.QtnSalesCashDiscount - in.QtnSalesReturnCashDiscount
	}
	comm := in.SalesCommission - in.SalesReturnCommission
	sal := 0.0
	if f.EmployeeModule {
		sal = in.SalaryPaid
	}
	total := in.Expense + purchases + cd + comm + sal
	if vatPercent <= 0 {
		vatPercent = 15
	}
	vat := total * vatPercent / (100 + vatPercent)
	return TotalExpenseResult{
		Purchases:       models.RoundFloat(purchases, 2),
		CashDiscount:    models.RoundFloat(cd, 2),
		Commission:      models.RoundFloat(comm, 2),
		Salary:          models.RoundFloat(sal, 2),
		Total:           models.RoundFloat(total, 2),
		Vat:             models.RoundFloat(vat, 2),
		TotalWithoutVat: models.RoundFloat(total-vat, 2),
	}
}

// DashboardDateRange turns from/to days (YYYY-MM-DD, either may be empty) in a
// timezone (CountryTimezoneOffset convention: UTC+3 → -3) into the legacy
// "date" filter: from 00:00:00 of `from` to 23:59:59 of `to`, store time.
func DashboardDateRange(from, to string, tzOffset float64) (bson.M, error) {
	var start, end time.Time
	if from = strings.TrimSpace(from); from != "" {
		d, err := time.Parse(layoutDay, from)
		if err != nil {
			return nil, errBadRequest("from must be a date (YYYY-MM-DD).", map[string]string{"from": "invalid"})
		}
		start = models.ConvertTimeZoneToUTC(tzOffset, d)
	}
	if to = strings.TrimSpace(to); to != "" {
		d, err := time.Parse(layoutDay, to)
		if err != nil {
			return nil, errBadRequest("to must be a date (YYYY-MM-DD).", map[string]string{"to": "invalid"})
		}
		end = models.ConvertTimeZoneToUTC(tzOffset, d).Add(24*time.Hour - time.Second)
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return nil, errBadRequest("to must not be before from.", map[string]string{"to": "before_from"})
	}
	switch {
	case !start.IsZero() && !end.IsZero():
		return bson.M{"$gte": start, "$lte": end}, nil
	case !start.IsZero():
		return bson.M{"$gte": start}, nil
	case !end.IsZero():
		return bson.M{"$lte": end}, nil
	}
	return nil, nil
}

func handleDashboardTotalExpense(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	store, dateRange, err := dashboardStore(c, r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	storeHex := store.ID.Hex()
	flags := TotalExpenseFlags{
		DisablePurchasesOnAccounts: store.Settings.DisablePurchasesOnAccounts,
		SalesInQuotation:           store.Settings.EnableSalesInQuotation,
		EmployeeModule:             store.Settings.EnableEmployeeModule,
	}
	in, err := loadTotalExpenseInputs(store, dateRange, flags)
	if err != nil {
		return errInternal("Unable to calculate the total expense.")
	}
	res := TotalExpense(in, flags, store.VatPercent)
	vat := store.VatPercent
	if vat <= 0 {
		vat = 15
	}
	writeJSON(w, http.StatusOK, M{
		"storeId":    storeHex,
		"from":       q.Get("from"),
		"to":         q.Get("to"),
		"vatPercent": vat,
		"flags":      flags,
		"inputs":     in,
		"result":     res,
	})
	return nil
}

// loadTotalExpenseInputs reads the legacy stats the formula needs, in parallel.
func loadTotalExpenseInputs(store *models.Store, dateRange bson.M, f TotalExpenseFlags) (TotalExpenseInputs, error) {
	filter := func() map[string]interface{} { return dashboardFilter(store, dateRange) }
	var (
		in    TotalExpenseInputs
		mu    sync.Mutex
		wg    sync.WaitGroup
		first error
	)
	run := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}()
	}
	run(func() error {
		s, err := store.GetExpenseStats(filter())
		mu.Lock()
		in.Expense = s.Total
		mu.Unlock()
		return err
	})
	run(func() error {
		s, err := store.GetSalesStats(filter())
		mu.Lock()
		in.SalesCashDiscount, in.SalesCommission = s.CashDiscount, s.Commission
		mu.Unlock()
		return err
	})
	run(func() error {
		s, err := store.GetSalesReturnStats(filter())
		mu.Lock()
		in.SalesReturnCashDiscount, in.SalesReturnCommission = s.CashDiscount, s.Commission
		mu.Unlock()
		return err
	})
	run(func() error {
		s, err := store.GetPurchaseStats(filter())
		mu.Lock()
		in.Purchase, in.PurchaseCashDiscount = s.NetTotal, s.CashDiscount
		mu.Unlock()
		return err
	})
	run(func() error {
		acc := filter()
		acc["enable_on_accounts"] = true
		s, err := store.GetPurchaseStats(acc)
		mu.Lock()
		in.AccountedPurchase, in.AccountedPurchaseCashDiscount = s.NetTotal, s.CashDiscount
		mu.Unlock()
		return err
	})
	run(func() error {
		s, err := store.GetPurchaseReturnStats(filter())
		mu.Lock()
		in.PurchaseReturn, in.PurchaseReturnCashDiscount = s.NetTotal, s.CashDiscount
		mu.Unlock()
		return err
	})
	run(func() error {
		acc := filter()
		acc["enable_on_accounts"] = true
		s, err := store.GetPurchaseReturnStats(acc)
		mu.Lock()
		in.AccountedPurchaseReturn, in.AccountedPurchaseReturnCashDisc = s.NetTotal, s.CashDiscount
		mu.Unlock()
		return err
	})
	if f.DisablePurchasesOnAccounts {
		run(func() error {
			s, err := store.GetCustomerDepositStats(filter())
			mu.Lock()
			in.DepositPurchaseFund = s.PurchaseFund
			mu.Unlock()
			return err
		})
	}
	if f.SalesInQuotation {
		run(func() error {
			s, err := store.GetQuotationInvoiceStats(filter())
			mu.Lock()
			in.QtnSalesCashDiscount = s.InvoiceCashDiscount
			mu.Unlock()
			return err
		})
		run(func() error {
			s, err := store.GetQuotationSalesReturnStats(filter())
			mu.Lock()
			in.QtnSalesReturnCashDiscount = s.CashDiscount
			mu.Unlock()
			return err
		})
	}
	if f.EmployeeModule {
		run(func() error {
			v, err := store.GetSalaryPaidInPeriod(filter())
			mu.Lock()
			in.SalaryPaid = v
			mu.Unlock()
			return err
		})
	}
	wg.Wait()
	return in, first
}
