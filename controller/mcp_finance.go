package controller

// mcp_finance.go — MCP finance endpoints under /v1/mcp/
//
//   GET /v1/mcp/customer-deposits     — list_customer_deposits
//   GET /v1/mcp/customer-withdrawals  — list_customer_withdrawals
//   GET /v1/mcp/capitals              — list_capitals
//   GET /v1/mcp/ledger                — list_ledger
//   GET /v1/mcp/accounts              — list_accounts

import (
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// MCPListCustomerDeposits handles GET /v1/mcp/customer-deposits
func MCPListCustomerDeposits(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	deposits, criterias, err := store.SearchCustomerDeposit(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "customerdeposit")
	mcpOK(w, totalCount, deposits, nil)
}

// MCPListCustomerWithdrawals handles GET /v1/mcp/customer-withdrawals
func MCPListCustomerWithdrawals(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	withdrawals, criterias, err := store.SearchCustomerWithdrawal(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "customerwithdrawal")
	mcpOK(w, totalCount, withdrawals, nil)
}

// MCPListCapitals handles GET /v1/mcp/capitals
func MCPListCapitals(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	capitals, criterias, err := store.SearchCapital(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "capital")
	mcpOK(w, totalCount, capitals, nil)
}

// MCPListLedger handles GET /v1/mcp/ledger
// Query params: store_id, date_str, from_date, to_date, page, limit, sort, account_id
func MCPListLedger(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	entries, criterias, err := store.SearchLedger(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "ledger")
	mcpOK(w, totalCount, entries, nil)
}

// MCPListAccounts handles GET /v1/mcp/accounts
// Note: SearchAccount is a package-level function (not on Store).
// The store_id filter is applied via the search[store_id] param.
func MCPListAccounts(w http.ResponseWriter, r *http.Request) {
	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		mcpWriteError(w, "Invalid access token: "+err.Error(), http.StatusUnauthorized)
		return
	}
	adapted := mcpBuildRequest(r)
	accounts, criterias, err := models.SearchAccount(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := models.GetTotalCount(criterias.SearchBy, "account")
	mcpOK(w, totalCount, accounts, nil)
}
