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
//   − expense VAT on vendor VAT bills
//
// With disable_purchases_on_accounts only purchases (and returns) flagged
// "on accounts" count. The card is shown only when the store setting
// enable_vat_box is on.
//
// Expense VAT departs from the old card on purpose (decided by the owner,
// 2026-10-07): the old card added the expense "vat" field, which expenses never
// fill (always 0). Here VAT paid on an expense lowers the VAT owed, counting only
// expenses with a valid VAT bill: a vendor that exists in the store's database,
// a vendor invoice number, and a VAT amount (vat_price) above zero. Dates are
// whole days in the store's own timezone.

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
		VatPayable: models.RoundFloat(out-inp-in.ExpenseVendorVat, 2),
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
	run(func() (float64, error) { return sumExpenseVendorVat(store, f()) }, &in.ExpenseVendorVat)
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

// sumExpenseVendorVat sums vat_price of the expenses in filter that have a valid
// VAT bill: a vendor invoice number, a VAT amount above zero, and a vendor that
// exists in the store's vendor collection.
func sumExpenseVendorVat(store *models.Store, filter map[string]interface{}) (float64, error) {
	filter["vendor_id"] = bson.M{"$exists": true, "$ne": nil}
	filter["vendor_invoice_no"] = bson.M{"$regex": `\S`}
	filter["vat_price"] = bson.M{"$gt": 0}
	ctx, cancel := dbctx()
	defer cancel()
	cur, err := storeDB(store.ID.Hex()).Collection("expense").Aggregate(ctx, []bson.M{
		{"$match": filter},
		{"$lookup": bson.M{"from": "vendor", "localField": "vendor_id", "foreignField": "_id", "as": "v"}},
		{"$match": bson.M{"v.0": bson.M{"$exists": true}}},
		{"$group": bson.M{"_id": nil, "total": bson.M{"$sum": "$vat_price"}}},
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
