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

// ListQuotation : handler for GET /quotation
func ListQuotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)
	response.Meta = make(map[string]interface{})

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	//quotations := []models.Quotation{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	quotations, criterias, err := store.SearchQuotation(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find quotations:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "quotation")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of quotations:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotationStats models.QuotationStats
	var quotationInvoiceStats models.QuotationInvoiceStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			quotationStats, err = store.GetQuotationStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_sales"] = "Unable to find total amount of quotation:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}

			quotationInvoiceStats, err = store.GetQuotationInvoiceStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_sales"] = "Unable to find total amount of quotation:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta["total_quotation"] = quotationStats.NetTotal
	response.Meta["profit"] = quotationStats.NetProfit
	response.Meta["loss"] = quotationStats.Loss
	//invoice

	response.Meta["invoice_total_sales"] = quotationInvoiceStats.InvoiceNetTotal
	response.Meta["invoice_net_profit"] = quotationInvoiceStats.InvoiceNetProfit
	response.Meta["invoice_net_loss"] = quotationInvoiceStats.InvoiceNetLoss
	response.Meta["invoice_vat_price"] = quotationInvoiceStats.InvoiceVatPrice
	response.Meta["invoice_discount"] = quotationInvoiceStats.InvoiceDiscount
	response.Meta["invoice_cash_discount"] = quotationInvoiceStats.InvoiceCashDiscount
	response.Meta["invoice_shipping_handling_fees"] = quotationInvoiceStats.InvoiceShippingOrHandlingFees
	response.Meta["invoice_paid_sales"] = quotationInvoiceStats.InvoicePaidSales
	response.Meta["invoice_unpaid_sales"] = quotationInvoiceStats.InvoiceUnPaidSales
	response.Meta["invoice_cash_sales"] = quotationInvoiceStats.InvoiceCashSales
	response.Meta["invoice_bank_account_sales"] = quotationInvoiceStats.InvoiceBankAccountSales
	response.Meta["invoice_sales_return_sales"] = quotationInvoiceStats.InvoiceSalesReturnSales

	if len(quotations) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = quotations
	}

	json.NewEncoder(w).Encode(response)

}

// CreateQuotation : handler for POST /quotation
func CreateQuotation(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotation *models.Quotation
	// Decode data
	if !utils.Decode(w, r, &quotation) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.CreatedBy = &userID
	quotation.UpdatedBy = &userID
	now := time.Now()
	quotation.CreatedAt = &now
	quotation.UpdatedAt = &now
	quotation.FindNetTotal()

	// Validate data
	if errs := quotation.Validate(w, r, "create"); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	//Queue
	queue := GetOrCreateQueue(store.ID.Hex(), "quotation")
	queueToken := generateQueueToken()
	queue.Enqueue(Request{Token: queueToken})
	queue.WaitUntilMyTurn(queueToken)

	err = quotation.CreateNewCustomerFromName()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation")

		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["new_customer_from_name"] = "error creating new customer from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.LinkOrUnLinkSales(nil)
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation")

		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["order_code"] = "Not able to link Sale ID"
		json.NewEncoder(w).Encode(response)
		return
	}

	//quotation.FindTotal()
	quotation.FindTotalQuantity()
	//	quotation.FindVatPrice()
	err = quotation.UpdateForeignLabelFields()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.MakeCode()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["code"] = "error making code: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.CalculateQuotationProfit()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["profit_calculation"] = "error calc: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.Insert()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation")

		redisErr := quotation.UnMakeRedisCode()
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

	queue.Pop()
	CleanupQueueIfEmpty(store.ID.Hex(), "quotation")

	err = quotation.CreateProductsQuotationHistory()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error adding product history" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	_, err = quotation.SetPaymentStatus()
	if err != nil {
		response.Status = false
		response.Errors["order"] = "Error getting payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.Update()
	if err != nil {
		response.Status = false
		response.Errors["order"] = "Error updating order: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetProductsQuotationStats()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error adding product quotation stats" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetProductsQuotationSalesStats()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error adding product quotation sales stats" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetCustomerQuotationStats()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error adding customer quotation stats" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() && quotation.Type == "invoice" {
		customer, _ := store.FindCustomerByID(quotation.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	err = store.NotifyUsers("quotation_updated")
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error notifying users" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go quotation.SetPostBalances()
	go quotation.CreateProductsHistory(true)

	response.Status = true
	response.Result = quotation

	json.NewEncoder(w).Encode(response)
}

// UpdateQuotation : handler function for PUT /v1/quotation call
func UpdateQuotation(w http.ResponseWriter, r *http.Request) {
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

	var quotation *models.Quotation
	//var quotationOld *models.Quotation

	params := mux.Vars(r)

	quotationID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Quotation ID:" + err.Error()
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

	quotationOld, err := store.FindQuotationByID(&quotationID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation, err = store.FindQuotationByID(&quotationID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &quotation) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.UpdatedBy = &userID
	now := time.Now()
	quotation.UpdatedAt = &now
	quotation.FindNetTotal()

	// Validate data
	if errs := quotation.Validate(w, r, "update"); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.CreateNewCustomerFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_customer_from_name"] = "error creating new customer from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation.FindTotalQuantity()

	err = quotation.CalculateQuotationProfit()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["profit_calulation"] = "Error calulating quotation profit: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.UpdateForeignLabelFields()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["updated_foreign_labels"] = "Error updating foreing labels: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.UpdatePayments()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	_, err = quotation.SetPaymentStatus()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["payment_status"] = "Error setting payment status " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.Update()
	if err != nil {

		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.LinkOrUnLinkSales(quotationOld)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["order_code"] = "Not able to link Sale ID"
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.ClearProductsQuotationHistory()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["clearing_history"] = "error clearing products quotation histor " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.CreateProductsQuotationHistory()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["add_history"] = "error adding products quotation histor " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationOld.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	/*
		err = quotation.AttributesValueChangeEvent(quotationOld)
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	err = quotation.SetProductsQuotationStats()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error adding product quotation  stats" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetProductsQuotationSalesStats()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "error adding product quotation sales stats" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.SetCustomerQuotationStats()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["view"] = "error setting customer quotation stats:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotationOld.CustomerID != nil &&
		!quotationOld.CustomerID.IsZero() &&
		quotation.CustomerID != nil &&
		!quotation.CustomerID.IsZero() &&
		quotationOld.CustomerID.Hex() != quotation.CustomerID.Hex() {

		err = quotationOld.SetCustomerQuotationStats()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			response.Status = false
			response.Errors["view"] = "error setting customer quotation stats:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	quotationOld.SetProductsQuotationStats()
	quotationOld.SetProductsQuotationSalesStats()

	err = quotation.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() && quotation.Type == "invoice" {
		customer, _ := store.FindCustomerByID(quotation.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	if quotationOld.CustomerID != nil && !quotationOld.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(quotationOld.CustomerID, bson.M{})
		if customer != nil {
			//	log.Print("Setting old customer credit balance")
			customer.SetCreditBalance()
			quotationOld.SetCustomerQuotationStats()
		}
	}

	quotation, err = store.FindQuotationByID(&quotation.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find quotation:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	go func() {
		quotation.ClearProductsHistory()
		quotation.CreateProductsHistory(true)
	}()

	go quotation.SetPostBalances()

	/*go func() {
		quotation.ClearProductsHistory()
		quotation.CreateProductsHistory(true)
	}()*/

	store.NotifyUsers("quotation_updated")
	response.Status = true
	response.Result = quotation
	json.NewEncoder(w).Encode(response)
}

// ViewQuotation : handler function for GET /v1/quotation/<id> call
func ViewQuotation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	_, err := models.AuthenticateByAccessToken(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["access_token"] = "Invalid Access token:" + err.Error()
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	params := mux.Vars(r)

	quotationID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["product_id"] = "Invalid Quotation ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotation *models.Quotation

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	store, err := ParseStore(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotation, err = store.FindQuotationByID(&quotationID, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotation.CustomerID != nil && !quotation.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(quotation.CustomerID, bson.M{})
		customer.SetSearchLabel()
		quotation.Customer = customer
	}

	response.Status = true
	response.Result = quotation

	json.NewEncoder(w).Encode(response)
}

// DeleteQuotation : handler function for DELETE /v1/quotation/<id> call
func DeleteQuotation(w http.ResponseWriter, r *http.Request) {
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

	quotationID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid Quotation ID:" + err.Error()
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

	quotation, err := store.FindQuotationByID(&quotationID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotation.DeleteQuotation(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}

// CreateOrder : handler for POST /order
func CalculateQuotationNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var quotation *models.Quotation
	// Decode data
	if !utils.Decode(w, r, &quotation) {
		return
	}

	quotation.FindNetTotal()

	response.Status = true
	response.Result = quotation

	json.NewEncoder(w).Encode(response)
}
