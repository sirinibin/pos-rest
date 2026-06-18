package controller

// mcp_bi.go — MCP BI Analytics endpoints under /v1/mcp/bi/
//
//   GET /v1/mcp/bi/monthly-revenue      — bi_get_monthly_revenue
//   GET /v1/mcp/bi/top-products         — bi_get_top_products
//   GET /v1/mcp/bi/top-customers        — bi_get_top_customers
//   GET /v1/mcp/bi/expense-summary      — bi_get_expense_summary
//   GET /v1/mcp/bi/outstanding          — bi_get_outstanding
//   GET /v1/mcp/bi/stock-alerts         — bi_get_stock_alerts
//   GET /v1/mcp/bi/vendor-performance   — bi_get_vendor_performance
//   GET /v1/mcp/bi/quotation-conversion — bi_get_quotation_conversion
//   GET /v1/mcp/bi/sales-by-category    — bi_get_sales_by_category
//   GET /v1/mcp/bi/product-abc-xyz      — bi_get_product_abc_xyz
//   GET /v1/mcp/bi/customer-churn       — bi_get_customer_churn
//   GET /v1/mcp/bi/customer-clv         — bi_get_customer_clv
//   GET /v1/mcp/bi/cohort-retention     — bi_get_cohort_retention
//   GET /v1/mcp/bi/product-sales-trends — bi_get_product_sales_trends
//   GET /v1/mcp/bi/monthly-pl           — bi_get_monthly_pl
//   GET /v1/mcp/bi/store-settings       — bi_get_store_settings

import (
	"net/http"
	"strconv"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func intParam(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func strParam(r *http.Request, key, def string) string {
	if v := r.URL.Query().Get(key); v != "" {
		return v
	}
	return def
}

// ── Endpoints ─────────────────────────────────────────────────────────────────

// MCPBIMonthlyRevenue handles GET /v1/mcp/bi/monthly-revenue
// Query params: store_id, months (default 12)
func MCPBIMonthlyRevenue(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	months := intParam(r, "months", 12)
	results, err := store.GetBIMonthlyRevenue(months)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"months":   months,
		"result":   results,
	})
}

// MCPBITopProducts handles GET /v1/mcp/bi/top-products
// Query params: store_id, period (30d|90d|all), limit
func MCPBITopProducts(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	period := strParam(r, "period", "30d")
	limit := intParam(r, "limit", 20)
	results, err := store.GetBITopProducts(period, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Cache miss: populate on demand then re-query.
	if len(results) == 0 {
		_ = models.UpsertBITopProducts(store.ID, period)
		results, _ = store.GetBITopProducts(period, limit)
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"period":   period,
		"result":   results,
	})
}

// MCPBITopCustomers handles GET /v1/mcp/bi/top-customers
func MCPBITopCustomers(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	period := strParam(r, "period", "30d")
	limit := intParam(r, "limit", 20)
	results, err := store.GetBITopCustomers(period, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Cache miss: populate on demand then re-query.
	if len(results) == 0 {
		_ = models.UpsertBITopCustomers(store.ID, period)
		results, _ = store.GetBITopCustomers(period, limit)
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"period":   period,
		"result":   results,
	})
}

// MCPBIExpenseSummary handles GET /v1/mcp/bi/expense-summary
func MCPBIExpenseSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	months := intParam(r, "months", 6)
	results, err := store.GetBIExpenseSummary(months)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Cache miss: populate on demand then re-query.
	if len(results) == 0 {
		models.RunBIExpenseSummaryUpdate(store.ID, months)
		results, _ = store.GetBIExpenseSummary(months)
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"months":   months,
		"result":   results,
	})
}

// MCPBIOutstanding handles GET /v1/mcp/bi/outstanding
// Query params: store_id, type (AR|AP), limit
func MCPBIOutstanding(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	outType := strParam(r, "type", "AR")
	limit := intParam(r, "limit", 500)
	result, err := store.GetBIOutstanding(outType, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Cache miss: populate on demand then re-query.
	if len(result.Items) == 0 {
		_ = models.UpsertBIOutstanding(store.ID)
		result, _ = store.GetBIOutstanding(outType, limit)
	}
	mcpWriteJSON(w, result)
}

// MCPBIStockAlerts handles GET /v1/mcp/bi/stock-alerts
// Query params: store_id, alert_type, limit
func MCPBIStockAlerts(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	alertType := r.URL.Query().Get("alert_type")
	limit := intParam(r, "limit", 50)
	results, err := store.GetBIStockAlerts(alertType, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":      store.ID.Hex(),
		"total_alerts":  len(results),
		"result":        results,
	})
}

// MCPBIVendorPerformance handles GET /v1/mcp/bi/vendor-performance
func MCPBIVendorPerformance(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	period := strParam(r, "period", "30d")
	limit := intParam(r, "limit", 20)
	results, err := store.GetBIVendorPerformance(period, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Cache miss: populate on demand then re-query.
	if len(results) == 0 {
		_ = models.UpsertBIVendorPerformance(store.ID, period)
		results, _ = store.GetBIVendorPerformance(period, limit)
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"period":   period,
		"result":   results,
	})
}

// MCPBIQuotationConversion handles GET /v1/mcp/bi/quotation-conversion
func MCPBIQuotationConversion(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	months := intParam(r, "months", 6)
	results, err := store.GetBIQuotationConversion(months)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"months":   months,
		"result":   results,
	})
}

// MCPBISalesByCategory handles GET /v1/mcp/bi/sales-by-category
func MCPBISalesByCategory(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	period := strParam(r, "period", "30d")
	results, err := store.GetBISalesByCategory(period)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"period":   period,
		"result":   results,
	})
}

// MCPBIProductAbcXyz handles GET /v1/mcp/bi/product-abc-xyz
// Query params: store_id, abc_tier, xyz_tier, limit
func MCPBIProductAbcXyz(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	abcTier := r.URL.Query().Get("abc_tier")
	xyzTier := r.URL.Query().Get("xyz_tier")
	limit := intParam(r, "limit", 100)
	results, err := store.GetBIProductAbcXyz(abcTier, xyzTier, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"result":   results,
	})
}

// MCPBICustomerChurn handles GET /v1/mcp/bi/customer-churn
// Query params: store_id, tier (Critical|High|Medium|Low), limit
func MCPBICustomerChurn(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	tier := r.URL.Query().Get("tier")
	limit := intParam(r, "limit", 50)
	results, err := store.GetBICustomerChurn(tier, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"result":   results,
	})
}

// MCPBICustomerCLV handles GET /v1/mcp/bi/customer-clv
// Query params: store_id, segment, limit
func MCPBICustomerCLV(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	segment := r.URL.Query().Get("segment")
	limit := intParam(r, "limit", 50)
	results, err := store.GetBICustomerCLV(segment, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"result":   results,
	})
}

// MCPBICohortRetention handles GET /v1/mcp/bi/cohort-retention
func MCPBICohortRetention(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	results, err := store.GetBICohortRetention()
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"result":   results,
	})
}

// MCPBIProductSalesTrends handles GET /v1/mcp/bi/product-sales-trends
// Query params: store_id, trend, limit
func MCPBIProductSalesTrends(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	trend := r.URL.Query().Get("trend")
	limit := intParam(r, "limit", 50)
	results, err := store.GetBIProductSalesTrends(trend, limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"result":   results,
	})
}

// MCPBIMonthlyPL handles GET /v1/mcp/bi/monthly-pl
// Query params: store_id, months (default 12)
func MCPBIMonthlyPL(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	months := intParam(r, "months", 12)
	results, err := store.GetMonthlyPL(months)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"months":   months,
		"result":   results,
	})
}

// MCPDailyRevenue handles GET /v1/mcp/bi/daily-revenue
// Returns day-by-day sales totals for a date range.
// Query params: store_id, date_str | from_date + to_date
func MCPDailyRevenue(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}

	tzOffset := models.CountryTimezoneOffset(store.CountryCode)

	// Build UTC start/end from the date params using the same helper as P&L Statement.
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildSalesCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	filter := criterias.SearchBy

	// Extract start/end from the filter map; fall back to last 7 days.
	var start, end time.Time
	if v, ok := filter["date"].(map[string]interface{}); ok {
		if s, ok := v["$gte"].(time.Time); ok {
			start = s
		}
		if e, ok := v["$lt"].(time.Time); ok {
			end = e
		}
	}
	if start.IsZero() {
		// Default: current month so far
		now := time.Now().UTC()
		localNow := now.Add(time.Duration(-tzOffset * float64(time.Hour)))
		start = models.ConvertTimeZoneToUTC(tzOffset,
			time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, time.UTC))
		end = now
	}

	results, err := store.GetDailyRevenue(start, end, tzOffset)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Compute summary totals
	var totalOrders int64
	var totalGross, totalNet float64
	for _, r := range results {
		totalOrders += r.OrderCount
		totalGross += r.GrossSales
		totalNet += r.NetSales
	}

	mcpWriteJSON(w, map[string]interface{}{
		"store_id":      store.ID.Hex(),
		"days":          len(results),
		"total_orders":  totalOrders,
		"total_gross":   models.RoundTo2Decimals(totalGross),
		"total_net":     models.RoundTo2Decimals(totalNet),
		"result":        results,
	})
}

// MCPProductReturnRate handles GET /v1/mcp/bi/product-return-rate
// Returns products ranked by return rate (returned units / sold units).
// Query params: store_id, limit (default 50)
func MCPProductReturnRate(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	limit := intParam(r, "limit", 50)
	results, err := store.GetProductReturnRates(limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"total":    len(results),
		"result":   results,
	})
}

// MCPCustomersOverCredit handles GET /v1/mcp/bi/customers-over-credit
// Returns customers whose outstanding balance exceeds their credit limit.
// Query params: store_id, limit (default 100)
func MCPCustomersOverCredit(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	limit := intParam(r, "limit", 100)
	results, err := store.GetCustomersOverCreditLimit(limit)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id": store.ID.Hex(),
		"total":    len(results),
		"result":   results,
	})
}

// MCPHourlySales handles GET /v1/mcp/bi/hourly-sales
// Returns order count and gross sales grouped by hour of day (0-23).
// Query params: store_id, date_str | from_date + to_date
func MCPHourlySales(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	tzOffset := models.CountryTimezoneOffset(store.CountryCode)

	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildSalesCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	filter := criterias.SearchBy

	var start, end time.Time
	if v, ok := filter["date"].(map[string]interface{}); ok {
		if s, ok := v["$gte"].(time.Time); ok {
			start = s
		}
		if e, ok := v["$lt"].(time.Time); ok {
			end = e
		}
	}
	if start.IsZero() {
		now := time.Now().UTC()
		localNow := now.Add(time.Duration(-tzOffset * float64(time.Hour)))
		start = models.ConvertTimeZoneToUTC(tzOffset,
			time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, time.UTC))
		end = now
	}

	results, err := store.GetHourlySales(start, end, tzOffset)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Find peak hour
	peakHour := results[0]
	for _, h := range results {
		if h.GrossSales > peakHour.GrossSales {
			peakHour = h
		}
	}

	mcpWriteJSON(w, map[string]interface{}{
		"store_id":   store.ID.Hex(),
		"peak_hour":  peakHour.HourLabel,
		"peak_sales": peakHour.GrossSales,
		"result":     results,
	})
}

// MCPBIStoreSettings handles GET /v1/mcp/bi/store-settings
// Returns store identity and key accounting flags.
func MCPBIStoreSettings(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"id":                              store.ID.Hex(),
		"name":                            store.Name,
		"branch_name":                     store.BranchName,
		"vat_no":                          store.VATNo,
		"registration_number":             store.RegistrationNumber,
		"address":                         store.Address,
		"vat_percent":                     store.VatPercent,
		"quotation_invoice_accounting":    store.Settings.QuotationInvoiceAccounting,
		"disable_purchases_on_accounts":   store.Settings.DisablePurchasesOnAccounts,
	})
}
