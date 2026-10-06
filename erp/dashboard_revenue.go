package erp

// dashboard_revenue.go — the dashboard's "Net revenue" / "Total revenue".
//
//   GET /v1/erp/dashboard/revenue?storeId=X&from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Same calculation as the revenue card of the old business dashboard
// (reactjs-pos src/business_dashboard/charts/KPICards.js, fed by the
// dashboard_monthly totals):
//
//   revenue = sales − sales returns
//             (+ quotation invoices − quotation sales returns with enable_sales_in_quotation)
//   total   = revenue (+ non-VAT sales − non-VAT sales returns with non_vat_sales)
//
// The VAT split is on revenue (non-VAT sales carry none). Dates are whole days in
// the store's own timezone.

import (
	"net/http"
	"sync"

	"github.com/sirinibin/startpos/backend/models"
)

// RevenueInputs are the period's net totals (VAT included).
type RevenueInputs struct {
	Sales             float64 `json:"sales"`
	SalesReturn       float64 `json:"salesReturn"`
	QtnSales          float64 `json:"qtnSales"`
	QtnSalesReturn    float64 `json:"qtnSalesReturn"`
	NonVatSales       float64 `json:"nonVatSales"`
	NonVatSalesReturn float64 `json:"nonVatSalesReturn"`
}

// RevenueFlags are the store settings the revenue card reads.
type RevenueFlags struct {
	SalesInQuotation bool `json:"salesInQuotation"`
	NonVatSales      bool `json:"nonVatSales"`
}

// RevenueResult is what the revenue card shows.
type RevenueResult struct {
	Revenue           float64 `json:"revenue"`           // with VAT, VAT sales only
	Vat               float64 `json:"vat"`               // VAT portion of Revenue
	RevenueWithoutVat float64 `json:"revenueWithoutVat"` // Revenue − Vat
	NonVatNet         float64 `json:"nonVatNet"`         // 0 unless non_vat_sales
	Total             float64 `json:"total"`             // Revenue + NonVatNet
}

// DashboardRevenue applies the old business dashboard's revenue formula.
func DashboardRevenue(in RevenueInputs, f RevenueFlags, vatPercent float64) RevenueResult {
	rev := in.Sales - in.SalesReturn
	if f.SalesInQuotation {
		rev += in.QtnSales - in.QtnSalesReturn
	}
	nonVat := 0.0
	if f.NonVatSales {
		nonVat = in.NonVatSales - in.NonVatSalesReturn
	}
	if vatPercent <= 0 {
		vatPercent = 15
	}
	vat := rev * vatPercent / (100 + vatPercent)
	return RevenueResult{
		Revenue:           models.RoundFloat(rev, 2),
		Vat:               models.RoundFloat(vat, 2),
		RevenueWithoutVat: models.RoundFloat(rev-vat, 2),
		NonVatNet:         models.RoundFloat(nonVat, 2),
		Total:             models.RoundFloat(rev+nonVat, 2),
	}
}

func handleDashboardRevenue(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	store, dateRange, err := dashboardStore(c, r)
	if err != nil {
		return err
	}
	flags := RevenueFlags{
		SalesInQuotation: store.Settings.EnableSalesInQuotation,
		NonVatSales:      store.Settings.NonVATSales,
	}
	var (
		in    RevenueInputs
		mu    sync.Mutex
		wg    sync.WaitGroup
		first error
	)
	sum := func(coll string, extra map[string]interface{}, dst *float64) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := dashboardFilter(store, dateRange)
			for k, v := range extra {
				f[k] = v
			}
			v, err := sumField(store, coll, f, "net_total")
			mu.Lock()
			defer mu.Unlock()
			*dst = v
			if err != nil && first == nil {
				first = err
			}
		}()
	}
	sum("order", nil, &in.Sales)
	sum("salesreturn", nil, &in.SalesReturn)
	if flags.SalesInQuotation {
		sum("quotation", map[string]interface{}{"type": "invoice"}, &in.QtnSales)
		sum("quotation_sales_return", nil, &in.QtnSalesReturn)
	}
	if flags.NonVatSales {
		sum("non_vat_sales", nil, &in.NonVatSales)
		sum("non_vat_sales_return", nil, &in.NonVatSalesReturn)
	}
	wg.Wait()
	if first != nil {
		return errInternal("Unable to calculate revenue.")
	}
	vat := store.VatPercent
	if vat <= 0 {
		vat = 15
	}
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, M{
		"storeId":    store.ID.Hex(),
		"from":       q.Get("from"),
		"to":         q.Get("to"),
		"vatPercent": vat,
		"flags":      flags,
		"inputs":     in,
		"result":     DashboardRevenue(in, flags, vat),
	})
	return nil
}
