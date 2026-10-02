package controller

// mcp_test.go — unauthenticated tests for every MCP handler.
//
// MCP handlers use mcpWriteError which produces {"error":"..."} — NOT the
// standard apiResponse format.  The assertion therefore decodes into a
// map[string]string and checks that the "error" key is non-empty.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// TestMCPHandlers_Unauthenticated verifies that every MCP endpoint returns
// HTTP 401 with {"error": "..."} when no access token is supplied.
func TestMCPHandlers_Unauthenticated(t *testing.T) {
	// dummyID is a syntactically valid MongoDB ObjectID that the handlers can
	// parse before they reach any database call.
	const dummyID = "64abc123456789001234abcd"

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		muxVars map[string]string
	}{
		// ── mcp_auth.go (MCPLogin is public — skipped) ──────────────────────
		{
			name:    "MCPListStores",
			method:  http.MethodGet,
			path:    "/v1/mcp/stores",
			handler: MCPListStores,
		},
		{
			name:    "MCPMe",
			method:  http.MethodGet,
			path:    "/v1/mcp/me",
			handler: MCPMe,
		},

		// ── mcp_bi.go ────────────────────────────────────────────────────────
		{
			name:    "MCPBIMonthlyRevenue",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/monthly-revenue",
			handler: MCPBIMonthlyRevenue,
		},
		{
			name:    "MCPBITopProducts",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/top-products",
			handler: MCPBITopProducts,
		},
		{
			name:    "MCPBITopCustomers",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/top-customers",
			handler: MCPBITopCustomers,
		},
		{
			name:    "MCPBIExpenseSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/expense-summary",
			handler: MCPBIExpenseSummary,
		},
		{
			name:    "MCPBIOutstanding",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/outstanding",
			handler: MCPBIOutstanding,
		},
		{
			name:    "MCPBIStockAlerts",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/stock-alerts",
			handler: MCPBIStockAlerts,
		},
		{
			name:    "MCPBIVendorPerformance",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/vendor-performance",
			handler: MCPBIVendorPerformance,
		},
		{
			name:    "MCPBIQuotationConversion",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/quotation-conversion",
			handler: MCPBIQuotationConversion,
		},
		{
			name:    "MCPBISalesByCategory",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/sales-by-category",
			handler: MCPBISalesByCategory,
		},
		{
			name:    "MCPBIProductAbcXyz",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/product-abc-xyz",
			handler: MCPBIProductAbcXyz,
		},
		{
			name:    "MCPBICustomerChurn",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/customer-churn",
			handler: MCPBICustomerChurn,
		},
		{
			name:    "MCPBICustomerCLV",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/customer-clv",
			handler: MCPBICustomerCLV,
		},
		{
			name:    "MCPBICohortRetention",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/cohort-retention",
			handler: MCPBICohortRetention,
		},
		{
			name:    "MCPBIProductSalesTrends",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/product-sales-trends",
			handler: MCPBIProductSalesTrends,
		},
		{
			name:    "MCPBIMonthlyPL",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/monthly-pl",
			handler: MCPBIMonthlyPL,
		},
		{
			name:    "MCPDailyRevenue",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/daily-revenue",
			handler: MCPDailyRevenue,
		},
		{
			name:    "MCPProductReturnRate",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/product-return-rate",
			handler: MCPProductReturnRate,
		},
		{
			name:    "MCPCustomersOverCredit",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/customers-over-credit",
			handler: MCPCustomersOverCredit,
		},
		{
			name:    "MCPHourlySales",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/hourly-sales",
			handler: MCPHourlySales,
		},
		{
			name:    "MCPBIStoreSettings",
			method:  http.MethodGet,
			path:    "/v1/mcp/bi/store-settings",
			handler: MCPBIStoreSettings,
		},

		// ── mcp_customers.go ─────────────────────────────────────────────────
		{
			name:    "MCPCustomerSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/customers/summary",
			handler: MCPCustomerSummary,
		},
		{
			name:    "MCPListCustomers",
			method:  http.MethodGet,
			path:    "/v1/mcp/customers",
			handler: MCPListCustomers,
		},
		{
			name:    "MCPGetCustomer",
			method:  http.MethodGet,
			path:    "/v1/mcp/customer/" + dummyID,
			handler: MCPGetCustomer,
			muxVars: map[string]string{"id": dummyID},
		},
		{
			name:    "MCPGetNewCustomers",
			method:  http.MethodGet,
			path:    "/v1/mcp/customers/new",
			handler: MCPGetNewCustomers,
		},

		// ── mcp_expenses.go ──────────────────────────────────────────────────
		{
			name:    "MCPExpenseSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/expenses/summary",
			handler: MCPExpenseSummary,
		},
		{
			name:    "MCPListExpenses",
			method:  http.MethodGet,
			path:    "/v1/mcp/expenses",
			handler: MCPListExpenses,
		},
		{
			name:    "MCPGetExpense",
			method:  http.MethodGet,
			path:    "/v1/mcp/expense/" + dummyID,
			handler: MCPGetExpense,
			muxVars: map[string]string{"id": dummyID},
		},
		{
			name:    "MCPListExpenseCategories",
			method:  http.MethodGet,
			path:    "/v1/mcp/expense-categories",
			handler: MCPListExpenseCategories,
		},

		// ── mcp_finance.go ───────────────────────────────────────────────────
		{
			name:    "MCPListCustomerDeposits",
			method:  http.MethodGet,
			path:    "/v1/mcp/customer-deposits",
			handler: MCPListCustomerDeposits,
		},
		{
			name:    "MCPListCustomerWithdrawals",
			method:  http.MethodGet,
			path:    "/v1/mcp/customer-withdrawals",
			handler: MCPListCustomerWithdrawals,
		},
		{
			name:    "MCPListCapitals",
			method:  http.MethodGet,
			path:    "/v1/mcp/capitals",
			handler: MCPListCapitals,
		},
		{
			name:    "MCPListLedger",
			method:  http.MethodGet,
			path:    "/v1/mcp/ledger",
			handler: MCPListLedger,
		},
		{
			name:    "MCPListAccounts",
			method:  http.MethodGet,
			path:    "/v1/mcp/accounts",
			handler: MCPListAccounts,
		},

		// ── mcp_inventory.go ─────────────────────────────────────────────────
		{
			name:    "MCPListWarehouses",
			method:  http.MethodGet,
			path:    "/v1/mcp/warehouses",
			handler: MCPListWarehouses,
		},
		{
			name:    "MCPListStockTransfers",
			method:  http.MethodGet,
			path:    "/v1/mcp/stock-transfers",
			handler: MCPListStockTransfers,
		},
		{
			name:    "MCPListDeliveryNotes",
			method:  http.MethodGet,
			path:    "/v1/mcp/delivery-notes",
			handler: MCPListDeliveryNotes,
		},
		{
			name:    "MCPQuotationSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/quotations/summary",
			handler: MCPQuotationSummary,
		},
		{
			name:    "MCPListQuotations",
			method:  http.MethodGet,
			path:    "/v1/mcp/quotations",
			handler: MCPListQuotations,
		},
		{
			name:    "MCPGetQuotation",
			method:  http.MethodGet,
			path:    "/v1/mcp/quotation/" + dummyID,
			handler: MCPGetQuotation,
			muxVars: map[string]string{"id": dummyID},
		},

		// ── mcp_pnl.go ───────────────────────────────────────────────────────
		{
			name:    "MCPProfitLossStatement",
			method:  http.MethodGet,
			path:    "/v1/mcp/pnl",
			handler: MCPProfitLossStatement,
		},

		// ── mcp_products.go ──────────────────────────────────────────────────
		{
			name:    "MCPProductSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/products/summary",
			handler: MCPProductSummary,
		},
		{
			name:    "MCPListProducts",
			method:  http.MethodGet,
			path:    "/v1/mcp/products",
			handler: MCPListProducts,
		},
		{
			name:    "MCPGetProduct",
			method:  http.MethodGet,
			path:    "/v1/mcp/product/" + dummyID,
			handler: MCPGetProduct,
			muxVars: map[string]string{"id": dummyID},
		},
		{
			name:    "MCPListProductCategories",
			method:  http.MethodGet,
			path:    "/v1/mcp/product-categories",
			handler: MCPListProductCategories,
		},
		{
			name:    "MCPListProductBrands",
			method:  http.MethodGet,
			path:    "/v1/mcp/product-brands",
			handler: MCPListProductBrands,
		},

		// ── mcp_purchases.go ─────────────────────────────────────────────────
		{
			name:    "MCPPurchaseSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchases/summary",
			handler: MCPPurchaseSummary,
		},
		{
			name:    "MCPListPurchases",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchases",
			handler: MCPListPurchases,
		},
		{
			name:    "MCPGetPurchase",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchase/" + dummyID,
			handler: MCPGetPurchase,
			muxVars: map[string]string{"id": dummyID},
		},
		{
			name:    "MCPPurchaseHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchases/history/summary",
			handler: MCPPurchaseHistorySummary,
		},
		{
			name:    "MCPListPurchaseHistory",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchases/history",
			handler: MCPListPurchaseHistory,
		},
		{
			name:    "MCPPurchaseReturnSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchase-returns/summary",
			handler: MCPPurchaseReturnSummary,
		},
		{
			name:    "MCPListPurchaseReturns",
			method:  http.MethodGet,
			path:    "/v1/mcp/purchase-returns",
			handler: MCPListPurchaseReturns,
		},

		// ── mcp_sales.go ─────────────────────────────────────────────────────
		{
			name:    "MCPSalesSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/sales/summary",
			handler: MCPSalesSummary,
		},
		{
			name:    "MCPListOrders",
			method:  http.MethodGet,
			path:    "/v1/mcp/orders",
			handler: MCPListOrders,
		},
		{
			name:    "MCPGetOrder",
			method:  http.MethodGet,
			path:    "/v1/mcp/order/" + dummyID,
			handler: MCPGetOrder,
			muxVars: map[string]string{"id": dummyID},
		},
		{
			name:    "MCPSalesHistorySummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/sales/history/summary",
			handler: MCPSalesHistorySummary,
		},
		{
			name:    "MCPListSalesHistory",
			method:  http.MethodGet,
			path:    "/v1/mcp/sales/history",
			handler: MCPListSalesHistory,
		},
		{
			name:    "MCPSalesReturnSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/sales-return/summary",
			handler: MCPSalesReturnSummary,
		},
		{
			name:    "MCPListSalesReturns",
			method:  http.MethodGet,
			path:    "/v1/mcp/sales-returns",
			handler: MCPListSalesReturns,
		},
		{
			name:    "MCPListSalesReturnHistory",
			method:  http.MethodGet,
			path:    "/v1/mcp/sales-return/history",
			handler: MCPListSalesReturnHistory,
		},

		// ── mcp_vendors.go ───────────────────────────────────────────────────
		{
			name:    "MCPVendorSummary",
			method:  http.MethodGet,
			path:    "/v1/mcp/vendors/summary",
			handler: MCPVendorSummary,
		},
		{
			name:    "MCPListVendors",
			method:  http.MethodGet,
			path:    "/v1/mcp/vendors",
			handler: MCPListVendors,
		},
		{
			name:    "MCPGetVendor",
			method:  http.MethodGet,
			path:    "/v1/mcp/vendor/" + dummyID,
			handler: MCPGetVendor,
			muxVars: map[string]string{"id": dummyID},
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(""))

			// Inject gorilla/mux path variables when the route requires them.
			if len(tc.muxVars) > 0 {
				r = mux.SetURLVars(r, tc.muxVars)
			}

			w := httptest.NewRecorder()
			tc.handler(w, r)

			res := w.Result()
			defer res.Body.Close()

			// ── Status code ──────────────────────────────────────────────
			if res.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected status 401, got %d", res.StatusCode)
			}

			// ── Content-Type ─────────────────────────────────────────────
			ct := res.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				t.Errorf("expected Content-Type application/json, got %q", ct)
			}

			// ── Body: {"error":"..."} format ──────────────────────────────
			var resp map[string]string
			if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}
			if resp["error"] == "" {
				t.Errorf("expected non-empty 'error' field in body, got %v", resp)
			}
		})
	}
}

// TestMCPBuildRequest_FieldMappings verifies that mcpBuildRequest correctly
// translates simplified MCP query params into search[field] format.
func TestMCPBuildRequest_FieldMappings(t *testing.T) {
	tests := []struct {
		name        string
		queryString string
		wantKey     string
		wantValue   string
	}{
		{
			name:        "store_id maps to search[store_id]",
			queryString: "store_id=abc123",
			wantKey:     "search[store_id]",
			wantValue:   "abc123",
		},
		{
			name:        "from_date maps to search[from_date]",
			queryString: "from_date=2024-01-01",
			wantKey:     "search[from_date]",
			wantValue:   "2024-01-01",
		},
		{
			name:        "to_date maps to search[to_date]",
			queryString: "to_date=2024-12-31",
			wantKey:     "search[to_date]",
			wantValue:   "2024-12-31",
		},
		{
			name:        "status maps to search[status]",
			queryString: "status=delivered",
			wantKey:     "search[status]",
			wantValue:   "delivered",
		},
		{
			name:        "customer_id maps to search[customer_id]",
			queryString: "customer_id=cust999",
			wantKey:     "search[customer_id]",
			wantValue:   "cust999",
		},
		{
			name:        "vendor_id maps to search[vendor_id]",
			queryString: "vendor_id=vend001",
			wantKey:     "search[vendor_id]",
			wantValue:   "vend001",
		},
		{
			name:        "query maps to search[search]",
			queryString: "query=laptop",
			wantKey:     "search[search]",
			wantValue:   "laptop",
		},
		{
			name:        "name maps to search[name]",
			queryString: "name=acme",
			wantKey:     "search[name]",
			wantValue:   "acme",
		},
		{
			name:        "category_id maps to search[category_id]",
			queryString: "category_id=cat42",
			wantKey:     "search[category_id]",
			wantValue:   "cat42",
		},
		{
			name:        "payment_status maps to search[payment_status]",
			queryString: "payment_status=paid",
			wantKey:     "search[payment_status]",
			wantValue:   "paid",
		},
		{
			name:        "code maps to search[code]",
			queryString: "code=INV-001",
			wantKey:     "search[code]",
			wantValue:   "INV-001",
		},
		{
			name:        "type maps to search[type]",
			queryString: "type=quotation",
			wantKey:     "search[type]",
			wantValue:   "quotation",
		},
		{
			name:        "item_code maps to search[item_code]",
			queryString: "item_code=SKU-42",
			wantKey:     "search[item_code]",
			wantValue:   "SKU-42",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+tc.queryString, nil)
			out := mcpBuildRequest(r)
			got := out.URL.Query().Get(tc.wantKey)
			if got != tc.wantValue {
				t.Errorf("param %q: want %q, got %q (full query: %s)",
					tc.wantKey, tc.wantValue, got, out.URL.RawQuery)
			}
		})
	}
}

// TestMCPBuildRequest_AlwaysSuppressesDeleted verifies that mcpBuildRequest
// always injects search[deleted]=0 regardless of the input params.
func TestMCPBuildRequest_AlwaysSuppressesDeleted(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?store_id=abc", nil)
	out := mcpBuildRequest(r)
	if got := out.URL.Query().Get("search[deleted]"); got != "0" {
		t.Errorf("expected search[deleted]=0, got %q", got)
	}
}

// TestMCPBuildRequest_IncludeStats verifies that include_stats=true and
// include_stats=1 both result in search[stats]=1.
func TestMCPBuildRequest_IncludeStats(t *testing.T) {
	for _, rawVal := range []string{"true", "1"} {
		r := httptest.NewRequest(http.MethodGet, "/?include_stats="+rawVal, nil)
		out := mcpBuildRequest(r)
		if got := out.URL.Query().Get("search[stats]"); got != "1" {
			t.Errorf("include_stats=%q: expected search[stats]=1, got %q", rawVal, got)
		}
	}
}

// TestMCPBuildRequest_PaginationPassThrough verifies that page, limit, and
// sort are forwarded unchanged.
func TestMCPBuildRequest_PaginationPassThrough(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?page=3&limit=50&sort=-created_at", nil)
	out := mcpBuildRequest(r)
	q := out.URL.Query()
	if got := q.Get("page"); got != "3" {
		t.Errorf("page: want 3, got %q", got)
	}
	if got := q.Get("limit"); got != "50" {
		t.Errorf("limit: want 50, got %q", got)
	}
	if got := q.Get("sort"); got != "-created_at" {
		t.Errorf("sort: want -created_at, got %q", got)
	}
}

// TestMCPBuildRequest_EmptyParamsSkipped verifies that empty/absent MCP params
// are not added to the resulting query string.
func TestMCPBuildRequest_EmptyParamsSkipped(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	out := mcpBuildRequest(r)
	q := out.URL.Query()
	// Only search[deleted] should be present from an empty input.
	if got := q.Get("search[store_id]"); got != "" {
		t.Errorf("expected search[store_id] to be absent, got %q", got)
	}
	if got := q.Get("search[stats]"); got != "" {
		t.Errorf("expected search[stats] to be absent for missing include_stats, got %q", got)
	}
}
