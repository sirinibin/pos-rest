package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBIHandlers_Unauthenticated verifies every BI handler returns 401 with
// errors["access_token"] when no auth token is provided.
//
// BI handlers use biAuthAndStore or biCronOrJWT — both fall through to the JWT
// check when no valid cron API key is present, so the same assertion applies.
func TestBIHandlers_Unauthenticated(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		// bi_aws_batch_settings.go
		{"GetBIAwsBatchSettings", http.MethodGet, "/v1/bi/aws-batch-settings", GetBIAwsBatchSettings},
		{"SaveBIAwsBatchSettings", http.MethodPost, "/v1/bi/aws-batch-settings", SaveBIAwsBatchSettings},
		{"GetBICronStoreSettings", http.MethodGet, "/v1/bi/cron-store-settings", GetBICronStoreSettings},
		{"SaveBICronStoreSettings", http.MethodPost, "/v1/bi/cron-store-settings", SaveBICronStoreSettings},
		// bi_cohort_retention.go
		{"GetBICohortRetention", http.MethodGet, "/v1/bi/cohort-retention", GetBICohortRetention},
		// bi_custom_question.go
		{"ListBICustomQuestions", http.MethodGet, "/v1/bi/custom-questions", ListBICustomQuestions},
		{"CreateBICustomQuestion", http.MethodPost, "/v1/bi/custom-questions", CreateBICustomQuestion},
		{"DeleteBICustomQuestion", http.MethodDelete, "/v1/bi/custom-questions", DeleteBICustomQuestion},
		// bi_customer_churn.go
		{"GetBICustomerChurn", http.MethodGet, "/v1/bi/customer-churn", GetBICustomerChurn},
		// bi_customer_clv.go
		{"GetBICustomerCLV", http.MethodGet, "/v1/bi/customer-clv", GetBICustomerCLV},
		// bi_data.go
		{"BIProductSalesHistory", http.MethodGet, "/v1/bi/product-sales-history", BIProductSalesHistory},
		{"BIProducts", http.MethodGet, "/v1/bi/products", BIProducts},
		{"BICustomers", http.MethodGet, "/v1/bi/customers", BICustomers},
		{"BIOrders", http.MethodGet, "/v1/bi/orders", BIOrders},
		{"BILedger", http.MethodGet, "/v1/bi/ledger", BILedger},
		{"BIStoreSettings", http.MethodGet, "/v1/bi/store-settings", BIStoreSettings},
		{"BISalesReturns", http.MethodGet, "/v1/bi/sales-returns", BISalesReturns},
		// bi_expense_summary.go
		{"GetBIExpenseSummary", http.MethodGet, "/v1/bi/expense-summary", GetBIExpenseSummary},
		// bi_monthly_pl.go
		{"BIMonthlyPL", http.MethodGet, "/v1/bi/monthly-pl", BIMonthlyPL},
		// bi_monthly_revenue.go
		{"GetBIMonthlyRevenue", http.MethodGet, "/v1/bi/monthly-revenue", GetBIMonthlyRevenue},
		// bi_outstanding.go
		{"GetBIOutstanding", http.MethodGet, "/v1/bi/outstanding", GetBIOutstanding},
		// bi_product_abc_xyz.go
		{"GetBIProductAbcXyz", http.MethodGet, "/v1/bi/product-abc-xyz", GetBIProductAbcXyz},
		// bi_product_sales_trend.go
		{"GetBIProductSalesTrend", http.MethodGet, "/v1/bi/product-sales-trend", GetBIProductSalesTrend},
		// bi_quotation_conversion.go
		{"GetBIQuotationConversion", http.MethodGet, "/v1/bi/quotation-conversion", GetBIQuotationConversion},
		// bi_report_result.go
		{"SaveBIReportResult", http.MethodPost, "/v1/bi/report-result", SaveBIReportResult},
		{"DeleteBIReportResult", http.MethodDelete, "/v1/bi/report-result", DeleteBIReportResult},
		{"GetBIReportResult", http.MethodGet, "/v1/bi/report-result", GetBIReportResult},
		{"DownloadBIReportResult", http.MethodGet, "/v1/bi/report-result/download", DownloadBIReportResult},
		{"SaveBICronLog", http.MethodPost, "/v1/bi/cron-log", SaveBICronLog},
		{"DeleteBICronLog", http.MethodDelete, "/v1/bi/cron-log", DeleteBICronLog},
		{"GetBICronLog", http.MethodGet, "/v1/bi/cron-log", GetBICronLog},
		// SaveAbcXyzScores uses cron-API-key-only auth (errors["auth"]) — tested separately below
		{"SaveVelocityScores", http.MethodPost, "/v1/bi/velocity-scores", SaveVelocityScores},
		{"SaveCLVScores", http.MethodPost, "/v1/bi/clv-scores", SaveCLVScores},
		{"SaveCohortScores", http.MethodPost, "/v1/bi/cohort-scores", SaveCohortScores},
		{"SaveBIBatchCost", http.MethodPost, "/v1/bi/batch-cost", SaveBIBatchCost},
		{"GetBIBatchCosts", http.MethodGet, "/v1/bi/batch-costs", GetBIBatchCosts},
		{"SaveChurnScores", http.MethodPost, "/v1/bi/churn-scores", SaveChurnScores},
		// bi_sales_by_category.go
		{"GetBISalesByCategory", http.MethodGet, "/v1/bi/sales-by-category", GetBISalesByCategory},
		// bi_stock_alerts.go
		{"GetBIStockAlerts", http.MethodGet, "/v1/bi/stock-alerts", GetBIStockAlerts},
		// bi_top_customers.go
		{"GetBITopCustomers", http.MethodGet, "/v1/bi/top-customers", GetBITopCustomers},
		// bi_top_products.go
		{"GetBITopProducts", http.MethodGet, "/v1/bi/top-products", GetBITopProducts},
		// bi_vendor_performance.go
		{"GetBIVendorPerformance", http.MethodGet, "/v1/bi/vendor-performance", GetBIVendorPerformance},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))
			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", res.StatusCode)
			}
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected JSON Content-Type, got %q", ct)
			}
			var resp apiResponse
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Status {
				t.Error("expected status=false")
			}
			if _, ok := resp.Errors["access_token"]; !ok {
				t.Errorf("expected errors[\"access_token\"], got %v", resp.Errors)
			}
		})
	}
}

// TestSaveAbcXyzScores_CronOnly verifies that SaveAbcXyzScores uses a
// cron-API-key-only auth guard (not JWT) and returns errors["auth"] when
// no valid cron key is provided.
func TestSaveAbcXyzScores_CronOnly(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/bi/abc-xyz-scores", strings.NewReader(""))
	w := httptest.NewRecorder()
	SaveAbcXyzScores(w, r)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", res.StatusCode)
	}
	var resp apiResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status {
		t.Error("expected status=false")
	}
	if _, ok := resp.Errors["auth"]; !ok {
		t.Errorf("expected errors[\"auth\"] for cron-only endpoint, got %v", resp.Errors)
	}
}
