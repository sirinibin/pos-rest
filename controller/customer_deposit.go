package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListCustomerDeposit : handler for GET /customerdeposit
func ListCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	customerdeposits := []models.CustomerDeposit{}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposits, criterias, err := store.SearchCustomerDeposit(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find customerdeposits:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "customerdeposit")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of customerdeposits:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdepositStats, err := store.GetCustomerDepositStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total"] = "Unable to find total amount of customerdeposits:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total"] = customerdepositStats.Total

	if len(customerdeposits) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = customerdeposits
	}

	json.NewEncoder(w).Encode(response)

}

// CreateCustomerDeposit : handler for POST /customerdeposit
func CreateCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	var customerdeposit *models.CustomerDeposit
	// Decode data
	if !utils.Decode(w, r, &customerdeposit) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit.CreatedBy = &userID
	customerdeposit.UpdatedBy = &userID
	now := time.Now()
	customerdeposit.CreatedAt = &now
	customerdeposit.UpdatedAt = &now
	customerdeposit.FindNetTotal()
	for i, _ := range customerdeposit.Payments {
		customerdeposit.Payments[i].CreatedAt = &now
		customerdeposit.Payments[i].CreatedBy = &userID
		customerdeposit.Payments[i].UpdatedAt = &now
		customerdeposit.Payments[i].UpdatedBy = &userID
	}

	// Validate data
	if errs := customerdeposit.Validate(w, r, "create", nil); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.MakeRedisCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error making code: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.Insert()
	if err != nil {
		redisErr := customerdeposit.UnMakeRedisCode()
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

	err = customerdeposit.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if customerdeposit.CustomerID != nil && !customerdeposit.CustomerID.IsZero() {
		store, _ := models.FindStoreByID(customerdeposit.StoreID, bson.M{})
		if store != nil {
			customer, _ := store.FindCustomerByID(customerdeposit.CustomerID, bson.M{})
			if customer != nil {
				customer.SetCreditBalance()
			}
		}
	}

	if customerdeposit.VendorID != nil && !customerdeposit.VendorID.IsZero() {
		store, _ := models.FindStoreByID(customerdeposit.StoreID, bson.M{})
		if store != nil {
			vendor, _ := store.FindVendorByID(customerdeposit.VendorID, bson.M{})
			if vendor != nil {
				vendor.SetCreditBalance()
			}
		}
	}

	err = customerdeposit.CloseSalesPayments()
	if err != nil {
		response.Status = false
		response.Errors["closing_sales"] = "error closing sales payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.ClosePurchaseReturnPayments()
	if err != nil {
		response.Status = false
		response.Errors["closing_purchase_return"] = "error closing purchase return payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.CloseQuotationSalesPayments()
	if err != nil {
		response.Status = false
		response.Errors["closing_quotation_sales"] = "error closing quotation sales payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerdeposit

	json.NewEncoder(w).Encode(response)
}

// UpdateCustomerDeposit : handler function for PUT /v1/customerdeposit call
func UpdateCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	var customerdeposit *models.CustomerDeposit
	var customerdepositOld *models.CustomerDeposit

	params := mux.Vars(r)

	customerdepositID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit_id"] = "Invalid CustomerDeposit ID:" + err.Error()
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

	customerdepositOld, err = store.FindCustomerDepositByID(&customerdepositID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit"] = "Unable to find customerdeposit:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit, err = store.FindCustomerDepositByID(&customerdepositID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit"] = "Unable to find customerdeposit:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &customerdeposit) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit.UpdatedBy = &userID
	now := time.Now()
	customerdeposit.UpdatedAt = &now
	customerdeposit.FindNetTotal()
	for i, _ := range customerdeposit.Payments {
		customerdeposit.Payments[i].CreatedAt = &now
		customerdeposit.Payments[i].CreatedBy = &userID
		customerdeposit.Payments[i].UpdatedAt = &now
		customerdeposit.Payments[i].UpdatedBy = &userID
	}

	// Validate data
	if errs := customerdeposit.Validate(w, r, "update", customerdepositOld); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.AttributesValueChangeEvent(customerdepositOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	customerdeposit, err = store.FindCustomerDepositByID(&customerdeposit.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find customerdeposit:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if customerdeposit.CustomerID != nil && !customerdeposit.CustomerID.IsZero() {
		store, _ := models.FindStoreByID(customerdeposit.StoreID, bson.M{})
		if store != nil {
			customer, _ := store.FindCustomerByID(customerdeposit.CustomerID, bson.M{})
			if customer != nil {
				customer.SetCreditBalance()
			}
		}
	}

	if customerdeposit.VendorID != nil && !customerdeposit.VendorID.IsZero() {
		store, _ := models.FindStoreByID(customerdeposit.StoreID, bson.M{})
		if store != nil {
			vendor, _ := store.FindVendorByID(customerdeposit.VendorID, bson.M{})
			if vendor != nil {
				vendor.SetCreditBalance()
			}
		}
	}

	if customerdepositOld.CustomerID != nil && !customerdepositOld.CustomerID.IsZero() {
		store, _ := models.FindStoreByID(customerdepositOld.StoreID, bson.M{})
		if store != nil {
			customer, _ := store.FindCustomerByID(customerdepositOld.CustomerID, bson.M{})
			if customer != nil {
				customer.SetCreditBalance()
			}
		}
	}

	if customerdepositOld.VendorID != nil && !customerdepositOld.VendorID.IsZero() {
		store, _ := models.FindStoreByID(customerdepositOld.StoreID, bson.M{})
		if store != nil {
			vendor, _ := store.FindVendorByID(customerdepositOld.VendorID, bson.M{})
			if vendor != nil {
				vendor.SetCreditBalance()
			}
		}
	}

	err = customerdeposit.HandleDeletedPayments(customerdepositOld)
	if err != nil {
		response.Status = false
		response.Errors["closing_sales"] = "error deleting sales payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.CloseSalesPayments()
	if err != nil {
		response.Status = false
		response.Errors["closing_sales"] = "error closing sales payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.ClosePurchaseReturnPayments()
	if err != nil {
		response.Status = false
		response.Errors["closing_purchase_return"] = "error closing purchase return payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.CloseQuotationSalesPayments()
	if err != nil {
		response.Status = false
		response.Errors["closing_quotation_sales"] = "error closing quotation sales payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	go customerdeposit.SetPostBalances()

	response.Status = true
	response.Result = customerdeposit
	json.NewEncoder(w).Encode(response)
}

// ViewCustomerDeposit : handler function for GET /v1/customerdeposit/<id> call
func ViewCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	customerdepositID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Errors["customerdeposit_id"] = "Invalid CustomerDeposit ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var customerdeposit *models.CustomerDeposit

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

	customerdeposit, err = store.FindCustomerDepositByID(&customerdepositID, selectFields)
	if err != nil {
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if customerdeposit.Type == "customer" {
		customer, _ := store.FindCustomerByID(customerdeposit.CustomerID, bson.M{})
		customer.SetSearchLabel()
		customerdeposit.Customer = customer

	} else if customerdeposit.Type == "vendor" {
		vendor, _ := store.FindVendorByID(customerdeposit.VendorID, bson.M{})
		vendor.SetSearchLabel()
		customerdeposit.Vendor = vendor
	}

	response.Status = true
	response.Result = customerdeposit

	json.NewEncoder(w).Encode(response)
}

// ViewCustomerDeposit : handler function for GET /v1/customerdeposit/code/<code> call
func ViewCustomerDepositByCode(w http.ResponseWriter, r *http.Request) {
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

	var customerdeposit *models.CustomerDeposit

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

	customerdeposit, err = store.FindCustomerDepositByCode(code, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customerdeposit

	json.NewEncoder(w).Encode(response)
}

// DeleteCustomerDeposit : handler function for DELETE /v1/customerdeposit/<id> call
func DeleteCustomerDeposit(w http.ResponseWriter, r *http.Request) {
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

	customerdepositID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customerdeposit_id"] = "Invalid CustomerDeposit ID:" + err.Error()
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

	customerdeposit, err := store.FindCustomerDepositByID(&customerdepositID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customerdeposit.DeleteCustomerDeposit(tokenClaims)
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
