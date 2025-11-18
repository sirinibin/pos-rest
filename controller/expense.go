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
	"go.mongodb.org/mongo-driver/mongo"
)

// ListExpense : handler for GET /expense
func ListExpense(w http.ResponseWriter, r *http.Request) {
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

	expenses := []models.Expense{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	expenses, criterias, err := store.SearchExpense(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find expenses:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "expense")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of expenses:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var expenseStats models.ExpenseStats
	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {

			expenseStats, err = store.GetExpenseStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total"] = "Unable to find total amount of expenses:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total"] = expenseStats.Total
	response.Meta["bank"] = expenseStats.Bank
	response.Meta["cash"] = expenseStats.Cash
	response.Meta["purchase_fund"] = expenseStats.PurchaseFund
	response.Meta["vat"] = expenseStats.Vat

	if len(expenses) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = expenses
	}

	json.NewEncoder(w).Encode(response)

}

// CreateExpense : handler for POST /expense
func CreateExpense(w http.ResponseWriter, r *http.Request) {
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

	var expense *models.Expense
	// Decode data
	if !utils.Decode(w, r, &expense) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	expense.CreatedBy = &userID
	expense.UpdatedBy = &userID
	now := time.Now()
	expense.CreatedAt = &now
	expense.UpdatedAt = &now

	store, err := models.FindStoreByID(expense.StoreID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if expense.VendorID != nil && !expense.VendorID.IsZero() {
		expense.VatPercent = &store.VatPercent
		baseAmount := models.RoundTo2Decimals(expense.Amount / (1 + (*expense.VatPercent / 100)))
		expense.VatPrice = models.RoundTo2Decimals(baseAmount * (*expense.VatPercent / 100))
	} else {
		expense.VatPercent = nil
		expense.VatPrice = 0
	}

	// Validate data
	if errs := expense.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_vendor_from_name"] = "error creating new vendor from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.MakeRedisCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error making code: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.Insert()
	if err != nil {
		redisErr := expense.UnMakeRedisCode()
		if redisErr != nil {
			response.Errors["error_unmaking_code"] = "error_unmaking_code: " + redisErr.Error()
		}
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	go expense.SetPostBalances()

	store.NotifyUsers("expense_updated")

	response.Status = true
	response.Result = expense

	json.NewEncoder(w).Encode(response)
}

// UpdateExpense : handler function for PUT /v1/expense call
func UpdateExpense(w http.ResponseWriter, r *http.Request) {
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

	var expense *models.Expense
	var expenseOld *models.Expense

	params := mux.Vars(r)

	expenseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["expense_id"] = "Invalid Expense ID:" + err.Error()
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

	expenseOld, err = store.FindExpenseByID(&expenseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["expense"] = "Unable to find expense:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	expense, err = store.FindExpenseByID(&expenseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["expense"] = "Unable to find expense:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &expense) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	expense.UpdatedBy = &userID
	now := time.Now()
	expense.UpdatedAt = &now

	if expense.VendorID != nil && !expense.VendorID.IsZero() {
		expense.VatPercent = &store.VatPercent
		baseAmount := models.RoundTo2Decimals(expense.Amount / (1 + (*expense.VatPercent / 100)))
		expense.VatPrice = models.RoundTo2Decimals(baseAmount * (*expense.VatPercent / 100))
	} else {
		expense.VatPercent = nil
		expense.VatPrice = 0
	}

	// Validate data
	if errs := expense.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_vendor_from_name"] = "error creating new vendor from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.AttributesValueChangeEvent(expenseOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	go expense.SetPostBalances()

	store, _ = models.FindStoreByID(expense.StoreID, bson.M{})
	store.NotifyUsers("expense_updated")

	expense, err = store.FindExpenseByID(&expense.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find expense:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = expense
	json.NewEncoder(w).Encode(response)
}

// ViewExpense : handler function for GET /v1/expense/<id> call
func ViewExpense(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Status = false
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	expenseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Errors["expense_id"] = "Invalid Expense ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var expense *models.Expense

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	expense, err = store.FindExpenseByID(&expenseID, selectFields)
	if err != nil {
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if expense.VendorID != nil && !expense.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(expense.VendorID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			w.WriteHeader(http.StatusBadRequest)
			response.Status = false
			response.Errors["view"] = "error fetching vendor:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
		vendor.SetSearchLabel()
		expense.Vendor = vendor
	}

	response.Status = true
	response.Result = expense

	json.NewEncoder(w).Encode(response)
}

// ViewExpense : handler function for GET /v1/expense/code/<code> call
func ViewExpenseByCode(w http.ResponseWriter, r *http.Request) {
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

	code := params["code"]
	if code == "" {
		response.Status = false
		response.Errors["code"] = "Invalid Code:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var expense *models.Expense

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	expense, err = store.FindExpenseByCode(code, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = expense

	json.NewEncoder(w).Encode(response)
}

// DeleteExpense : handler function for DELETE /v1/expense/<id> call
func DeleteExpense(w http.ResponseWriter, r *http.Request) {
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

	expenseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["expense_id"] = "Invalid Expense ID:" + err.Error()
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

	expense, err := store.FindExpenseByID(&expenseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = expense.DeleteExpense(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}
