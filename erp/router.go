package erp

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/controller"
)

// Prefix is where the adapter is mounted (no collision with legacy /v1/...).
const Prefix = "/v1/erp"

var initOnce sync.Once

func initResources() {
	initOnce.Do(func() {
		for _, r := range allResources() {
			register(r)
		}
	})
}

// Register mounts the StartERP adapter API on an existing router. It must be
// called before any catch-all route (e.g. the SPA handler).
func Register(router *mux.Router) {
	initResources()
	s := router.PathPrefix(Prefix).Subrouter()
	s.HandleFunc("/meta", handleMeta).Methods("GET")
	s.HandleFunc("/countries", handleCountries).Methods("GET")
	s.HandleFunc("/auth/login", rateLimited(loginLimiter, handleLogin)).Methods("POST")
	s.HandleFunc("/auth/refresh", handleRefresh).Methods("POST")
	s.HandleFunc("/auth/logout", handleLogout).Methods("POST")
	s.HandleFunc("/auth/me", handleMe).Methods("GET")
	s.HandleFunc("/auth/signup", rateLimited(signupLimiter, handleSignup)).Methods("POST")

	// ZATCA actions (delegating to the existing ZATCA flow)
	s.HandleFunc("/sales/{id}/zatca/report", handleZatcaReport("sales")).Methods("POST")
	s.HandleFunc("/sales-returns/{id}/zatca/report", handleZatcaReport("sales-returns")).Methods("POST")
	s.HandleFunc("/deposits/{id}/zatca/report", handleZatcaReport("deposits")).Methods("POST")
	s.HandleFunc("/withdrawals/{id}/zatca/report", handleZatcaReport("withdrawals")).Methods("POST")
	s.HandleFunc("/stores/{id}/zatca/connect", handleZatcaConnect).Methods("POST")
	s.HandleFunc("/stores/{id}/zatca/disconnect", handleZatcaDisconnect).Methods("POST")

	// Subscription billing by bank transfer (billing.go)
	registerBilling(s)

	// Starter catalog of the store's business category (starter_catalog.go)
	registerStarterCatalog(s)

	// Dashboard figures computed like the old business dashboard (dashboard_expense.go),
	// the BI dashboard (dashboard_bi.go) and the main dashboard's data (dashboard_feed.go),
	// all served from ready-made snapshots (dashboard_snapshots.go)
	for _, k := range dashboardKinds() {
		s.HandleFunc("/dashboard/"+k.Name, authed(snapshotted(k))).Methods("GET")
	}

	// Drafts (§2.1b): separate *_draft collections, never touch real data
	s.HandleFunc("/drafts/{docType}", handleDraftList).Methods("GET")
	s.HandleFunc("/drafts/{docType}", handleDraftCreate).Methods("POST")
	s.HandleFunc("/drafts/{docType}/{id}", handleDraftGet).Methods("GET")
	s.HandleFunc("/drafts/{docType}/{id}", handleDraftPut).Methods("PUT")
	s.HandleFunc("/drafts/{docType}/{id}", handleDraftDelete).Methods("DELETE")
	s.HandleFunc("/drafts/{docType}/{id}/finalize", handleDraftFinalize).Methods("POST")

	for _, res := range Resources() {
		res := res
		s.HandleFunc("/"+res.Path, func(w http.ResponseWriter, r *http.Request) { handleList(w, r, res) }).Methods("GET")
		if res.Path == "products" {
			// before /{id}: "facets" is not a record id
			s.HandleFunc("/products/facets", func(w http.ResponseWriter, r *http.Request) { handleProductFacets(w, r, res) }).Methods("GET")
			s.HandleFunc("/products/{id}/history", func(w http.ResponseWriter, r *http.Request) { handleProductHistory(w, r, res) }).Methods("GET")
		}
		if hasListStats(res) {
			// before /{id}: "stats" is not a record id
			s.HandleFunc("/"+res.Path+"/stats", func(w http.ResponseWriter, r *http.Request) { handleListStats(w, r, res) }).Methods("GET")
		}
		s.HandleFunc("/"+res.Path, func(w http.ResponseWriter, r *http.Request) { handleCreate(w, r, res) }).Methods("POST")
		s.HandleFunc("/"+res.Path+"/{id}", func(w http.ResponseWriter, r *http.Request) { handleGet(w, r, res) }).Methods("GET")
		s.HandleFunc("/"+res.Path+"/{id}", func(w http.ResponseWriter, r *http.Request) { handleUpdate(w, r, res, false) }).Methods("PATCH")
		s.HandleFunc("/"+res.Path+"/{id}", func(w http.ResponseWriter, r *http.Request) { handleUpdate(w, r, res, true) }).Methods("PUT")
		s.HandleFunc("/"+res.Path+"/{id}", func(w http.ResponseWriter, r *http.Request) { handleDelete(w, r, res) }).Methods("DELETE")
		s.HandleFunc("/"+res.Path+"/{id}/restore", func(w http.ResponseWriter, r *http.Request) { handleRestore(w, r, res) }).Methods("POST")
	}
	s.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeErr(w, errNotFound()) })
}

// dashboardKinds are the dashboard endpoints served from snapshots.
func dashboardKinds() []*dashKind {
	return []*dashKind{
		registerDashKind(&dashKind{Name: "total-expense", Params: []string{"from", "to"}, Handler: handleDashboardTotalExpense}),
		registerDashKind(&dashKind{Name: "vat", Params: []string{"from", "to"}, Handler: handleDashboardVat}),
		registerDashKind(&dashKind{Name: "revenue", Params: []string{"from", "to"}, Handler: handleDashboardRevenue}),
		registerDashKind(&dashKind{Name: "net-profit", Params: []string{"from", "to"}, Handler: handleDashboardNetProfit}),
		registerDashKind(&dashKind{Name: "salary-balance", Handler: handleDashboardSalaryBalance}),
		registerDashKind(&dashKind{Name: "bi", Daily: true, Handler: handleDashboardBI}),
		// Rev 2: per-day product rows instead of invoice lines
		registerDashKind(&dashKind{Name: "feed", Daily: true, Rev: 2, Handler: handleDashboardFeed}),
	}
}

// Adapter-specific limiters (same policy as the legacy /v1/authorize and
// /v1/guest-register limiters, separate budgets, contract error envelope).
// ERP_LOGIN_LIMIT / ERP_SIGNUP_LIMIT override the per-IP budgets.
var (
	loginLimiter  = controller.NewRateLimiter(envInt("ERP_LOGIN_LIMIT", 10), 15*time.Minute)
	signupLimiter = controller.NewRateLimiter(envInt("ERP_SIGNUP_LIMIT", 5), 60*time.Minute)
)

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

func clientIP(r *http.Request) string {
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func rateLimited(rl *controller.RateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "900")
			writeErr(w, errf(http.StatusTooManyRequests, "rate_limited", "Too many attempts. Please try again later.", nil))
			return
		}
		next(w, r)
	}
}

// NewRouter returns a router with only the adapter mounted (tests, tools).
func NewRouter() *mux.Router {
	r := mux.NewRouter()
	Register(r)
	return r
}
