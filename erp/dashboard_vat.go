package erp

// dashboard_vat.go — the dashboard's "VAT box".
//
//   GET /v1/erp/dashboard/vat?storeId=X&from=YYYY-MM-DD&to=YYYY-MM-DD
//
// Same calculation as the "VAT" card of the old business dashboard
// (reactjs-pos src/business_dashboard/charts/KPICards.js, fed by the
// dashboard_monthly totals of models/dashboard_monthly.go):
//
//   sales VAT − sales return VAT − purchase VAT + purchase return VAT
//   + expense VAT on vendor invoices
//
// With disable_purchases_on_accounts only purchases (and returns) flagged
// "on accounts" count. The card is shown only when the store setting
// enable_vat_box is on. The expense term sums the expense "vat" field on
// expenses with a vendor and a vendor invoice number, exactly like
// dashboard_monthly (legacy expenses keep their VAT in vat_price, so the term
// is 0 there too). Dates are whole days in the store's own timezone.

import (
	"net/http"
	"sync"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// DashboardVatInputs are the period's VAT totals.
type DashboardVatInputs struct {
	SalesVat                   float64 `json:"salesVat"`
	SalesReturnVat             float64 `json:"salesReturnVat"`
	PurchaseVat                float64 `json:"purchaseVat"`
	PurchaseReturnVat          float64 `json:"purchaseReturnVat"`
	AccountedPurchaseVat       float64 `json:"accountedPurchaseVat"`
	AccountedPurchaseReturnVat float64 `json:"accountedPurchaseReturnVat"`
	ExpenseVendorVat           float64 `json:"expenseVendorVat"`
}

// DashboardVatFlags are the store settings the VAT card reads.
type DashboardVatFlags struct {
	DisablePurchasesOnAccounts bool `json:"disablePurchasesOnAccounts"`
	EnableVatBox               bool `json:"enableVatBox"`
}

// DashboardVatResult is what the VAT box shows.
type DashboardVatResult struct {
	OutVat     float64 `json:"outVat"`     // sales − sales returns
	InVat      float64 `json:"inVat"`      // purchases − purchase returns (accounted ones in on-account mode)
	ExpenseVat float64 `json:"expenseVat"` // expense VAT on vendor invoices (added, as on the old card)
	VatPayable float64 `json:"vatPayable"` // out − in + expense (negative: refundable)
}

// DashboardVat applies the old business dashboard's VAT card formula.
func DashboardVat(in DashboardVatInputs, f DashboardVatFlags) DashboardVatResult {
	out := in.SalesVat - in.SalesReturnVat
	inp := in.PurchaseVat - in.PurchaseReturnVat
	if f.DisablePurchasesOnAccounts {
		inp = in.AccountedPurchaseVat - in.AccountedPurchaseReturnVat
	}
	return DashboardVatResult{
		OutVat:     models.RoundFloat(out, 2),
		InVat:      models.RoundFloat(inp, 2),
		ExpenseVat: models.RoundFloat(in.ExpenseVendorVat, 2),
		VatPayable: models.RoundFloat(out-inp+in.ExpenseVendorVat, 2),
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
	dateRange, err := DashboardDateRange(q.Get("from"), q.Get("to"), storeLocation(M{"country_code": store.CountryCode}))
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
	acc := func() map[string]interface{} {
		m := f()
		m["enable_on_accounts"] = true
		return m
	}
	run(func() (float64, error) { s, err := store.GetSalesStats(f()); return s.VatPrice, err }, &in.SalesVat)
	run(func() (float64, error) { s, err := store.GetSalesReturnStats(f()); return s.VatPrice, err }, &in.SalesReturnVat)
	run(func() (float64, error) { s, err := store.GetPurchaseStats(f()); return s.VatPrice, err }, &in.PurchaseVat)
	run(func() (float64, error) { s, err := store.GetPurchaseReturnStats(f()); return s.VatPrice, err }, &in.PurchaseReturnVat)
	run(func() (float64, error) { s, err := store.GetPurchaseStats(acc()); return s.VatPrice, err }, &in.AccountedPurchaseVat)
	run(func() (float64, error) { s, err := store.GetPurchaseReturnStats(acc()); return s.VatPrice, err }, &in.AccountedPurchaseReturnVat)
	run(func() (float64, error) {
		m := f()
		m["vendor_id"] = bson.M{"$exists": true, "$ne": nil}
		m["vendor_invoice_no"] = bson.M{"$exists": true, "$ne": ""}
		return sumField(store, "expense", m, "vat")
	}, &in.ExpenseVendorVat)
	wg.Wait()
	if first != nil {
		return errInternal("Unable to calculate VAT.")
	}
	flags := DashboardVatFlags{
		DisablePurchasesOnAccounts: store.Settings.DisablePurchasesOnAccounts,
		EnableVatBox:               store.Settings.EnableVATBox,
	}
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, M{
		"storeId": store.ID.Hex(),
		"from":    q.Get("from"),
		"to":      q.Get("to"),
		"flags":   flags,
		"inputs":  in,
		"result":  DashboardVat(in, flags),
	})
	return nil
}

// sumField sums one numeric field over a store collection's matching documents.
func sumField(store *models.Store, coll string, filter map[string]interface{}, field string) (float64, error) {
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(store.ID.Hex()).Collection(coll).Aggregate(ctx, []bson.M{
		{"$match": filter},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$" + field}}},
	})
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	var res struct {
		Total float64 `bson:"total"`
	}
	if cur.Next(ctx) {
		_ = cur.Decode(&res)
	}
	return res.Total, nil
}
