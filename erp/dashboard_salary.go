package erp

// dashboard_salary.go — the dashboard's "Salary balance".
//
//   GET /v1/erp/dashboard/salary-balance?storeId=X
//
// Same figure as the "Salary Balance" card of the old business dashboard
// (reactjs-pos src/business_dashboard/charts/KPICards.js ← GET
// /v1/dashboard/employee ← models.GetDashboardEmployee): the net balance of the
// open employee ledger accounts (debit − credit; negative = the store owes its
// employees), with a per-employee breakdown. It is a running balance, so it does
// not depend on the dashboard period. The card shows only with
// enable_employee_module.

import (
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// SalaryBalanceStatus says who owes whom for a net employee balance.
func SalaryBalanceStatus(balance float64) string {
	switch {
	case balance < -0.004:
		return "owed_to_employees"
	case balance > 0.004:
		return "employees_owe"
	}
	return "settled"
}

func handleDashboardSalaryBalance(c *Ctx, w http.ResponseWriter, r *http.Request) error {
	store, _, err := dashboardStore(c, r)
	if err != nil {
		return err
	}
	res, err := models.GetDashboardEmployee(store.ID)
	if err != nil || res == nil {
		return errInternal("Unable to read the salary balance.")
	}
	employees := []M{}
	for _, e := range res.EmployeeBreakdown {
		employees = append(employees, M{"accountId": e.AccountID, "name": e.Name, "balance": e.Balance, "direction": e.Direction})
	}
	writeJSON(w, http.StatusOK, M{
		"storeId":        store.ID.Hex(),
		"employeeModule": store.Settings.EnableEmployeeModule,
		"balance":        res.SalaryBalance,
		"status":         SalaryBalanceStatus(res.SalaryBalance),
		"employees":      employees,
	})
	return nil
}
