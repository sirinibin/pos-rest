package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// sameOptionalDate compares two *time.Time by calendar day (nil-safe), used to
// detect whether an employee's OpeningBalanceDate actually changed on update.
func sameOptionalDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// ListEmployee : handler for GET /employee
func ListEmployee(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	employees, criterias, err := store.SearchEmployee(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find employees:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "employee")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of employees:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var employeeStats models.EmployeeStats
	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys) > 0 && keys[0] == "1" {
		employeeStats, err = store.GetEmployeeStats(criterias.SearchBy)
		if err != nil {
			response.Status = false
			response.Errors["stats"] = "Unable to compute employee stats: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Meta = map[string]interface{}{
		"total_employees":          employeeStats.TotalEmployees,
		"total_salary":             employeeStats.TotalSalary,
		"total_owed_to_employees":  employeeStats.TotalOwedToEmployees,
		"total_employees_owe":      employeeStats.TotalEmployeesOwe,
		"total_salary_paid":        employeeStats.TotalSalaryPaid,
	}

	if len(employees) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = employees
	}

	json.NewEncoder(w).Encode(response)
}

// CreateEmployee : handler for POST /employee
func CreateEmployee(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var employee *models.Employee
	if !utils.Decode(w, r, &employee) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	employee.CreatedBy = &userID
	employee.UpdatedBy = &userID
	now := time.Now()
	employee.CreatedAt = &now
	employee.UpdatedAt = &now

	if errs := employee.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := employee.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := employee.Insert(); err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to insert employee:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Create the employee liability account immediately on creation.
	store, _ := models.FindStoreByID(employee.StoreID, bson.M{})
	if store != nil {
		acc, err := employee.GetOrCreateLiabilityAccount(store)
		if err == nil && acc != nil {
			employee.Account = acc
		}

		if err := employee.PostOpeningBalanceIfNeeded(store); err != nil {
			response.Status = false
			response.Errors["opening_balance"] = "Unable to post opening balance:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		// Post any automatic salary-due accrual entries already owed as of
		// today (e.g. the employee's salary day already passed this month),
		// so it's visible immediately instead of waiting for the next
		// 3-hourly cron run.
		if err := employee.RecalculateSalaryDueForEmployee(store); err != nil {
			response.Status = false
			response.Errors["salary_due"] = "Unable to generate salary due entries:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		if err := employee.Update(); err != nil {
			response.Status = false
			response.Errors["account"] = "Unable to save employee account:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Status = true
	response.Result = employee
	json.NewEncoder(w).Encode(response)
}

// ViewEmployee : handler for GET /employee/{id}
func ViewEmployee(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	employee, err := store.FindEmployeeByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find employee:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	account, err := employee.GetOrCreateLiabilityAccount(store)
	if err != nil {
		response.Status = false
		response.Errors["account"] = "Unable to load employee account:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	if account != nil {
		employee.Account = account
		if err := employee.Account.CalculateBalance(nil, nil); err != nil {
			response.Status = false
			response.Errors["account_balance"] = "Unable to refresh employee account balance:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Status = true
	response.Result = employee
	json.NewEncoder(w).Encode(response)
}

// UpdateEmployee : handler for PUT /employee/{id}
func UpdateEmployee(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	employee, err := store.FindEmployeeByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find employee:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Remember prior opening-balance values so we only re-post the ledger entry
	// when they actually change (avoids needless ledger churn on unrelated edits).
	oldOpeningBalance := employee.OpeningBalance
	var oldOpeningBalanceDate *time.Time
	if employee.OpeningBalanceDate != nil {
		t := *employee.OpeningBalanceDate
		oldOpeningBalanceDate = &t
	}
	// Remember prior Salary/SalaryDay/JoiningDate so we can clear and
	// regenerate the automatic salary-due accrual entries whenever any of
	// them change (the old entries were computed from stale figures, or may
	// now fall before the migration cutover and need removing).
	oldSalary := employee.Salary
	oldSalaryDay := employee.SalaryDay
	var oldJoiningDate *time.Time
	if employee.JoiningDate != nil {
		t := *employee.JoiningDate
		oldJoiningDate = &t
	}

	if !utils.Decode(w, r, &employee) {
		return
	}

	userID, _ := primitive.ObjectIDFromHex(tokenClaims.UserID)
	employee.UpdatedBy = &userID

	if errs := employee.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := employee.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := employee.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update employee:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	acc, err := employee.GetOrCreateLiabilityAccount(store)
	if err != nil {
		response.Status = false
		response.Errors["account"] = "Unable to sync employee account:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	if acc != nil {
		employee.Account = acc
	}

	openingBalanceChanged := employee.OpeningBalance != oldOpeningBalance ||
		!sameOptionalDate(employee.OpeningBalanceDate, oldOpeningBalanceDate) ||
		(!employee.OpeningBalancePosted && employee.OpeningBalance != 0)
	if openingBalanceChanged {
		if err := employee.PostOpeningBalanceIfNeeded(store); err != nil {
			response.Status = false
			response.Errors["opening_balance"] = "Unable to post opening balance:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	// Clear and regenerate the automatic salary-due accrual entries whenever
	// Salary, SalaryDay, OpeningBalanceDate, or JoiningDate changes, so the
	// correction (including removal of any now-invalid entries that predate
	// the migration cutover) is visible immediately — testable same-day —
	// instead of waiting for the nightly cron job.
	if employee.Salary != oldSalary || employee.SalaryDay != oldSalaryDay ||
		!sameOptionalDate(employee.OpeningBalanceDate, oldOpeningBalanceDate) ||
		!sameOptionalDate(employee.JoiningDate, oldJoiningDate) {
		if err := employee.RecalculateSalaryDueForEmployee(store); err != nil {
			response.Status = false
			response.Errors["salary_due"] = "Unable to regenerate salary due entries:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if err := employee.Update(); err != nil {
		response.Status = false
		response.Errors["account"] = "Unable to save employee account:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = employee
	json.NewEncoder(w).Encode(response)
}

// DeleteEmployee : handler for DELETE /employee/{id}
func DeleteEmployee(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	employee, err := store.FindEmployeeByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find employee:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := employee.Delete(tokenClaims); err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete employee:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"
	json.NewEncoder(w).Encode(response)
}

// HardDeleteEmployee : handler for DELETE /employee/permanent/{id}
// Permanently removes the employee and every related ledger/posting/account
// record (see Employee.HardDelete for the full scope). This cannot be undone.
func HardDeleteEmployee(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	employee, err := store.FindEmployeeByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find employee:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := employee.HardDelete(); err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to permanently delete employee:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted permanently"
	json.NewEncoder(w).Encode(response)
}

// ──────────────────────────────────────────────────────────
// Salary Payment Handlers
// ──────────────────────────────────────────────────────────

// ListEmployeeSalaryPayment : handler for GET /employee-salary-payment
func ListEmployeeSalaryPayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	payments, criterias, err := store.SearchEmployeeSalaryPayment(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salary payments:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, _ = store.GetTotalCount(criterias.SearchBy, "employee_salary_payment")

	if len(payments) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = payments
	}

	json.NewEncoder(w).Encode(response)
}

// CreateEmployeeSalaryPayment : handler for POST /employee-salary-payment
func CreateEmployeeSalaryPayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	var payment *models.EmployeeSalaryPayment
	if !utils.Decode(w, r, &payment) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	payment.CreatedBy = &userID
	payment.UpdatedBy = &userID
	now := time.Now()
	payment.CreatedAt = &now
	payment.UpdatedAt = &now
	if payment.Date == nil {
		payment.Date = &now
	}

	if errs := payment.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.Insert(); err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to insert salary payment:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.DoAccounting(); err != nil {
		response.Status = false
		response.Errors["accounting"] = "Error creating ledger entries:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = payment
	json.NewEncoder(w).Encode(response)
}

// ViewEmployeeSalaryPayment : handler for GET /employee-salary-payment/{id}
func ViewEmployeeSalaryPayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	payment, err := store.FindEmployeeSalaryPaymentByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salary payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = payment
	json.NewEncoder(w).Encode(response)
}

// UpdateEmployeeSalaryPayment : handler for PUT /employee-salary-payment/{id}
// Reverses the existing ledger postings, applies the updated fields, re-posts the
// ledger, and re-checks salary-due accrual for both the old and new pay periods
// so the employee's balance sheet always reflects the latest data.
func UpdateEmployeeSalaryPayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	payment, err := store.FindEmployeeSalaryPaymentByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salary payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Remember old pay period so its accrual can be re-checked after the update.
	oldMonth, oldYear, oldEmployeeID := payment.Month, payment.Year, payment.EmployeeID

	var payload *models.EmployeeSalaryPayment
	if !utils.Decode(w, r, &payload) {
		return
	}

	// Reverse the ledger/postings created for the payment's current values.
	if err := payment.UndoAccounting(); err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undoing ledger entries:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	payment.Date = payload.Date
	payment.Amount = payload.Amount
	payment.PaymentMethod = payload.PaymentMethod
	payment.Month = payload.Month
	payment.Year = payload.Year
	payment.Description = payload.Description
	if payment.EmployeeID != nil && payload.EmployeeID != nil && !payload.EmployeeID.IsZero() {
		payment.EmployeeID = payload.EmployeeID
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	payment.UpdatedBy = &userID
	now := time.Now()
	payment.UpdatedAt = &now
	if payment.Date == nil {
		payment.Date = &now
	}

	if errs := payment.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update salary payment:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.DoAccounting(); err != nil {
		response.Status = false
		response.Errors["accounting"] = "Error creating ledger entries:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// The period the payment used to cover may now be unpaid again (e.g. its
	// month/year or employee was changed) — re-check and accrue if past due.
	if oldEmployeeID != nil && (oldMonth != payment.Month || oldYear != payment.Year || (payment.EmployeeID != nil && oldEmployeeID.Hex() != payment.EmployeeID.Hex())) {
		if err := models.RegenerateSalaryDueForEmployeePeriod(store, oldEmployeeID, oldMonth, oldYear); err != nil {
			response.Status = false
			response.Errors["regenerate_salary_due"] = "Error regenerating salary due ledger:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if err := payment.RegenerateSalaryDueIfNeeded(); err != nil {
		response.Status = false
		response.Errors["regenerate_salary_due"] = "Error regenerating salary due ledger:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = payment
	json.NewEncoder(w).Encode(response)
}

// DeleteEmployeeSalaryPayment : handler for DELETE /employee-salary-payment/{id}
func DeleteEmployeeSalaryPayment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	tokenClaims, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)
	id, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	payment, err := store.FindEmployeeSalaryPaymentByID(&id, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salary payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.UndoAccounting(); err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undoing ledger entries:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.Delete(tokenClaims); err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete salary payment:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := payment.RegenerateSalaryDueIfNeeded(); err != nil {
		response.Status = false
		response.Errors["regenerate_salary_due"] = "Error regenerating salary due ledger:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"
	json.NewEncoder(w).Encode(response)
}
