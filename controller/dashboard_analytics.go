package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func dashboardStoreID(r *http.Request) (primitive.ObjectID, error) {
	return primitive.ObjectIDFromHex(r.URL.Query().Get("store_id"))
}

// dashboardMonthParams returns from_month / to_month from the request (YYYY-MM).
// Falls back to a single "month" param when no range is provided.
func dashboardMonthParams(r *http.Request) (from, to string) {
	from = r.URL.Query().Get("from_month")
	to = r.URL.Query().Get("to_month")
	if from == "" && to == "" {
		if m := r.URL.Query().Get("month"); m != "" {
			return m, m
		}
	}
	return from, to
}

func dashboardJSON(w http.ResponseWriter, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := models.Response{Status: true, Result: result}
	json.NewEncoder(w).Encode(resp)
}

func dashboardError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := models.Response{Status: false, Errors: map[string]string{"error": msg}}
	json.NewEncoder(w).Encode(resp)
}

func dashboardAuth(w http.ResponseWriter, r *http.Request) bool {
	if _, err := models.AuthenticateByAccessToken(r); err != nil {
		dashboardError(w, http.StatusUnauthorized, "Invalid access token: "+err.Error())
		return false
	}
	return true
}

func dashboardStoreAndTZ(w http.ResponseWriter, r *http.Request) (primitive.ObjectID, float64, bool) {
	storeID, err := dashboardStoreID(r)
	if err != nil {
		dashboardError(w, http.StatusBadRequest, "Invalid store_id")
		return primitive.NilObjectID, 0, false
	}
	store, err := models.FindStoreByID(&storeID, nil)
	if err != nil || store == nil {
		dashboardError(w, http.StatusBadRequest, "Store not found")
		return primitive.NilObjectID, 0, false
	}
	return storeID, models.CountryTimezoneOffset(store.CountryCode), true
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// GET /v1/dashboard/monthly?store_id=X&from_month=YYYY-MM&to_month=YYYY-MM
func DashboardGetMonthly(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, _, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	from, to := dashboardMonthParams(r)
	data, err := models.GetDashboardMonthly(storeID, from, to)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, "Failed to fetch monthly data: "+err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/products?store_id=X&from_month=YYYY-MM&to_month=YYYY-MM&limit=10
func DashboardGetProducts(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, tzOffset, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	from, to := dashboardMonthParams(r)
	// Convert YYYY-MM to YYYY-MM-DD range for the underlying product query
	fromDate, toDate := monthRangeToDateRange(from, to, tzOffset)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	data, err := models.GetDashboardTopProducts(storeID, fromDate, toDate, tzOffset, limit)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/customers?store_id=X&from_month=YYYY-MM&to_month=YYYY-MM&limit=10
func DashboardGetCustomers(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, tzOffset, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	from, to := dashboardMonthParams(r)
	fromDate, toDate := monthRangeToDateRange(from, to, tzOffset)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	data, err := models.GetDashboardTopCustomers(storeID, fromDate, toDate, tzOffset, limit)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/outstanding?store_id=X&limit=10
func DashboardGetOutstanding(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, _, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	data, err := models.GetDashboardOutstanding(storeID, limit)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/categories?store_id=X
func DashboardGetCategories(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, _, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	data, err := models.GetDashboardCategories(storeID)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/vendors?store_id=X&from_month=YYYY-MM&to_month=YYYY-MM
func DashboardGetVendors(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, tzOffset, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	from, to := dashboardMonthParams(r)
	fromDate, toDate := monthRangeToDateRange(from, to, tzOffset)
	data, err := models.GetDashboardVendors(storeID, fromDate, toDate, tzOffset)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/accounts?store_id=X
func DashboardGetAccounts(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, _, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	data, err := models.GetDashboardAccounts(storeID)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// GET /v1/dashboard/stock?store_id=X
func DashboardGetStock(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, _, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	data, err := models.GetDashboardStock(storeID)
	if err != nil {
		dashboardError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dashboardJSON(w, data)
}

// POST /v1/dashboard/backfill?store_id=X&months=12
// Triggers a historical backfill for one store. Runs asynchronously.
func DashboardBackfill(w http.ResponseWriter, r *http.Request) {
	if !dashboardAuth(w, r) {
		return
	}
	storeID, _, ok := dashboardStoreAndTZ(w, r)
	if !ok {
		return
	}
	months, _ := strconv.Atoi(r.URL.Query().Get("months"))
	go models.BackfillDashboardForStore(storeID, months)
	dashboardJSON(w, map[string]string{"message": "backfill started"})
}

// ─── helper: convert YYYY-MM range to YYYY-MM-DD range ───────────────────────

// monthRangeToDateRange converts from/to month strings ("2025-01") to the
// first-day-of-month strings ("2025-01-01") used by the product/customer/vendor
// underlying queries that still operate on date strings.
func monthRangeToDateRange(fromMonth, toMonth string, _ float64) (fromDate, toDate string) {
	if fromMonth != "" {
		fromDate = fromMonth + "-01"
	}
	if toMonth != "" {
		// Last day of toMonth: first day of next month minus one day is complex,
		// but these queries use $gte/$lte on date strings so "-31" safely covers
		// any month end when compared lexicographically as YYYY-MM-DD strings.
		toDate = toMonth + "-31"
	}
	return fromDate, toDate
}
