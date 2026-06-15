package controller

// mcp_expenses.go — MCP expense endpoints under /v1/mcp/
//
//   GET /v1/mcp/expenses/summary    — get_expense_summary
//   GET /v1/mcp/expenses            — list_expenses
//   GET /v1/mcp/expense/{id}        — get_expense
//   GET /v1/mcp/expense-categories  — list_expense_categories

import (
	"net/http"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MCPExpenseSummary handles GET /v1/mcp/expenses/summary
func MCPExpenseSummary(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	criterias, err := store.BuildExpenseCriterias(w, adapted)
	if err != nil {
		mcpWriteError(w, "filter error: "+err.Error(), http.StatusBadRequest)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "expense")
	stats, err := store.GetExpenseStats(criterias.SearchBy)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mcpWriteJSON(w, map[string]interface{}{
		"store_id":    store.ID.Hex(),
		"total_count": totalCount,
		"summary":     stats,
	})
}

// MCPListExpenses handles GET /v1/mcp/expenses
func MCPListExpenses(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	expenses, criterias, err := store.SearchExpense(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "expense")
	mcpOK(w, totalCount, expenses, nil)
}

// MCPGetExpense handles GET /v1/mcp/expense/{id}
func MCPGetExpense(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(vars["id"])
	if err != nil {
		mcpWriteError(w, "invalid expense id", http.StatusBadRequest)
		return
	}
	expense, err := store.FindExpenseByID(&id, nil)
	if err != nil {
		mcpWriteError(w, "expense not found: "+err.Error(), http.StatusNotFound)
		return
	}
	mcpWriteJSON(w, expense)
}

// MCPListExpenseCategories handles GET /v1/mcp/expense-categories
func MCPListExpenseCategories(w http.ResponseWriter, r *http.Request) {
	store, ok := mcpAuthAndStore(w, r)
	if !ok {
		return
	}
	adapted := mcpBuildRequest(r)
	cats, criterias, err := store.SearchExpenseCategory(w, adapted)
	if err != nil {
		mcpWriteError(w, "query error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	totalCount, _ := store.GetTotalCount(criterias.SearchBy, "expense_category")
	mcpOK(w, totalCount, cats, nil)
}
