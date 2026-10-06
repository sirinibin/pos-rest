package erp

// dashboard_vat.go — the dashboard's "VAT box".
//
//   GET /v1/erp/dashboard/vat?storeId=X&from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Same calculation as the "VAT" figure of the old business dashboard's Overall
// Summary (reactjs-pos src/stats/index.js): VAT on sales minus VAT on sales
// returns, less VAT on purchases minus VAT on purchase returns, from the legacy
// vat_price stats. Expense VAT is not part of it, as on the old dashboard.
// Dates are whole days in the store's own timezone.

import (
	"net/http"
	"sync"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// DashboardVatInputs are the period's VAT totals.
type DashboardVatInputs struct {
	SalesVat          float64 `json:"salesVat"`
	SalesReturnVat    float64 `json:"salesReturnVat"`
	PurchaseVat       float64 `json:"purchaseVat"`
	PurchaseReturnVat float64 `json:"purchaseReturnVat"`
}

// DashboardVatResult is what the VAT box shows.
type DashboardVatResult struct {
	OutVat     float64 `json:"outVat"`     // sales − sales returns
	InVat      float64 `json:"inVat"`      // purchases − purchase returns
	VatPayable float64 `json:"vatPayable"` // out − in (negative: refundable)
}

// DashboardVat applies the old business dashboard's net VAT formula.
func DashboardVat(in DashboardVatInputs) DashboardVatResult {
	out := in.SalesVat - in.SalesReturnVat
	inp := in.PurchaseVat - in.PurchaseReturnVat
	return DashboardVatResult{
		OutVat:     models.RoundFloat(out, 2),
		InVat:      models.RoundFloat(inp, 2),
		VatPayable: models.RoundFloat(out-inp, 2),
	}
}

// dashboardStore resolves the store a dashboard figure is asked for and the
// legacy date filter for from/to in the store's timezone.
func dashboardStore(c *Ctx, r *http.Request) (*models.Store, bson.M, error) {
	q := r.URL.Query()
	storeHex := q.Get("storeId")
	if storeHex == "" {
		return nil, nil, errBadRequest("storeId is required.", map[string]string{"storeId": "required"})
	}
	if c.store(storeHex) == nil {
		return nil, nil, errNotFound()
	}
	if !c.Admin && !c.can("reports", "view") {
		return nil, nil, errForbidden("")
	}
	oid, err := primitive.ObjectIDFromHex(storeHex)
	if err != nil {
		return nil, nil, errNotFound()
	}
	store, err := models.FindStoreByID(&oid, bson.M{})
	if err != nil || store == nil {
		return nil, nil, errNotFound()
	}
	dateRange, err := DashboardDateRange(q.Get("from"), q.Get("to"), models.CountryTimezoneOffset(store.CountryCode))
	if err != nil {
		return nil, nil, err
	}
	return store, dateRange, nil
}

func dashboardFilter(store *models.Store, dateRange bson.M) map[string]interface{} {
	m := map[string]interface{}{"deleted": bson.M{"$ne": true}, "store_id": store.ID}
	if dateRange != nil {
		m["date"] = dateRange
	}
	return m
}

func handleDashboardVat(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	store, dateRange, err := dashboardStore(c, r)
	if err != nil {
		return err
	}
	var (
		in    DashboardVatInputs
		mu    sync.Mutex
		wg    sync.WaitGroup
		first error
	)
	run := func(fn func() (float64, error), dst *float64) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := fn()
			mu.Lock()
			defer mu.Unlock()
			*dst = v
			if err != nil && first == nil {
				first = err
			}
		}()
	}
	f := func() map[string]interface{} { return dashboardFilter(store, dateRange) }
	run(func() (float64, error) { s, err := store.GetSalesStats(f()); return s.VatPrice, err }, &in.SalesVat)
	run(func() (float64, error) { s, err := store.GetSalesReturnStats(f()); return s.VatPrice, err }, &in.SalesReturnVat)
	run(func() (float64, error) { s, err := store.GetPurchaseStats(f()); return s.VatPrice, err }, &in.PurchaseVat)
	run(func() (float64, error) { s, err := store.GetPurchaseReturnStats(f()); return s.VatPrice, err }, &in.PurchaseReturnVat)
	wg.Wait()
	if first != nil {
		return errInternal("Unable to calculate VAT.")
	}
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, M{
		"storeId": store.ID.Hex(),
		"from":    q.Get("from"),
		"to":      q.Get("to"),
		"inputs":  in,
		"result":  DashboardVat(in),
	})
	return nil
}
