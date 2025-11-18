package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirinibin/startpos/backend/models"
	"github.com/sirinibin/startpos/backend/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListQuotationSalesReturn : handler for GET /quotationsalesreturn
func ListQuotationSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	quotationsalesreturns := []models.QuotationSalesReturn{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	quotationsalesreturns, criterias, err := store.SearchQuotationSalesReturn(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find quotationsalesreturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "quotation_sales_return")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of quotationsalesreturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var quotationsalesReturnStats models.QuotationSalesReturnStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			quotationsalesReturnStats, err = store.GetQuotationSalesReturnStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_return_quotationsales"] = "Unable to find total amount of quotationsales return:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_quotation_sales_return"] = quotationsalesReturnStats.NetTotal
	response.Meta["net_profit"] = quotationsalesReturnStats.NetProfit
	response.Meta["net_loss"] = quotationsalesReturnStats.NetLoss
	response.Meta["vat_price"] = quotationsalesReturnStats.VatPrice
	response.Meta["discount"] = quotationsalesReturnStats.Discount
	response.Meta["cash_discount"] = quotationsalesReturnStats.CashDiscount
	response.Meta["paid_quotation_sales_return"] = quotationsalesReturnStats.PaidQuotationSalesReturn
	response.Meta["unpaid_quotation_sales_return"] = quotationsalesReturnStats.UnPaidQuotationSalesReturn
	response.Meta["cash_quotation_sales_return"] = quotationsalesReturnStats.CashQuotationSalesReturn
	response.Meta["bank_account_quotation_sales_return"] = quotationsalesReturnStats.BankAccountQuotationSalesReturn
	response.Meta["shipping_handling_fees"] = quotationsalesReturnStats.ShippingOrHandlingFees
	response.Meta["quotation_sales_quotation_sales_return"] = quotationsalesReturnStats.QuotationSalesQuotationSalesReturn

	if len(quotationsalesreturns) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = quotationsalesreturns
	}

	json.NewEncoder(w).Encode(response)

}

// CreateQuotationSalesReturn : handler for POST /quotationsalesreturn
func CreateQuotationSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var quotationsalesreturn *models.QuotationSalesReturn
	// Decode data
	if !utils.Decode(w, r, &quotationsalesreturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.CreatedBy = &userID
	quotationsalesreturn.UpdatedBy = &userID
	now := time.Now()
	quotationsalesreturn.CreatedAt = &now
	quotationsalesreturn.UpdatedAt = &now
	quotationsalesreturn.FindNetTotal()

	// Validate data
	if errs := quotationsalesreturn.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	//Queue
	queue := GetOrCreateQueue(store.ID.Hex(), "quotation_sales_return")
	queueToken := generateQueueToken()
	queue.Enqueue(Request{Token: queueToken})
	queue.WaitUntilMyTurn(queueToken)

	quotationsalesreturn.FindTotalQuantity()
	err = quotationsalesreturn.UpdateForeignLabelFields()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation_sales_return")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.CalculateQuotationSalesReturnProfit()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation_sales_return")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["profit_calculation"] = "error calc: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.MakeCode()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation_sales_return")

		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["code"] = "Error making code: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.UUID = uuid.New().String()

	err = quotationsalesreturn.Insert()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "quotation_sales_return")

		redisErr := quotationsalesreturn.UnMakeCode()
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
	CleanupQueueIfEmpty(store.ID.Hex(), "quotation_sales_return")

	quotationsalesreturn.UpdateQuotationReturnCount()
	quotationsalesreturn.UpdateQuotationReturnDiscount(nil)
	quotationsalesreturn.UpdateQuotationReturnCashDiscount(nil)

	quotationsalesreturn.CreateProductsQuotationSalesReturnHistory()

	err = quotationsalesreturn.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.SetPaymentStatus()
	quotationsalesreturn.Update()

	err = quotationsalesreturn.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.UpdateReturnedQuantityInQuotationProduct(nil)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.CloseQuotationSalesPayment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["closing_qtn_sales_payment"] = "error closing qtn. sales payment: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotationsalesreturn.CustomerID != nil && !quotationsalesreturn.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(quotationsalesreturn.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	quotation, _ := store.FindQuotationByID(quotationsalesreturn.QuotationID, bson.M{})
	quotation.ReturnAmount, quotation.ReturnCount, _ = store.GetReturnedAmountByQuotationID(quotation.ID)
	quotation.Update()

	quotationsalesreturn.SetCustomerQuotationSalesReturnStats()
	quotation.SetCustomerQuotationStats()

	go quotationsalesreturn.SetProductsQuotationSalesReturnStats()

	go quotationsalesreturn.SetPostBalances()

	go quotationsalesreturn.CreateProductsHistory()

	store.NotifyUsers("quotationsales_return_updated")

	response.Status = true
	response.Result = quotationsalesreturn

	json.NewEncoder(w).Encode(response)
}

// UpdateQuotationSalesReturn : handler function for PUT /v1/quotationsalesreturn call
func UpdateQuotationSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var quotationsalesreturn *models.QuotationSalesReturn
	var quotationsalesreturnOld *models.QuotationSalesReturn

	params := mux.Vars(r)

	quotationsalesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotationsalesreturn_id"] = "Invalid QuotationSalesReturn ID:" + err.Error()
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

	quotationsalesreturnOld, err = store.FindQuotationSalesReturnByID(&quotationsalesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_quotationsalesreturn"] = "Unable to find quotationsalesreturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn, err = store.FindQuotationSalesReturnByID(&quotationsalesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_quotationsalesreturn"] = "Unable to find quotationsalesreturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &quotationsalesreturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.UpdatedBy = &userID
	now := time.Now()
	quotationsalesreturn.UpdatedAt = &now
	quotationsalesreturn.FindNetTotal()

	// Validate data
	if errs := quotationsalesreturn.Validate(w, r, "update", quotationsalesreturnOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.FindTotalQuantity()
	quotationsalesreturn.UpdateForeignLabelFields()
	quotationsalesreturn.CalculateQuotationSalesReturnProfit()

	/*
		if store.Zatca.Phase == "2" && store.Zatca.Connected {

			err = quotationsalesreturn.ReportToZatca()
			if err != nil {
				response.Status = false
				response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(response)
				return
			}
		}*/

	err = quotationsalesreturn.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.UpdateQuotationReturnCount()
	quotationsalesreturn.UpdateQuotationReturnDiscount(quotationsalesreturnOld)
	quotationsalesreturn.UpdateQuotationReturnCashDiscount(quotationsalesreturnOld)

	quotationsalesreturn.ClearProductsQuotationSalesReturnHistory()
	quotationsalesreturn.CreateProductsQuotationSalesReturnHistory()
	//count, _ := quotationsalesreturn.GetPaymentsCount()

	/*
		if count == 1 && quotationsalesreturn.PaymentStatus == "paid" {
			quotationsalesreturn.ClearPayments()
			quotationsalesreturn.AddPayment()
		}
	*/

	err = quotationsalesreturn.UpdatePayments()
	if err != nil {
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn.SetPaymentStatus()
	quotationsalesreturn.Update()

	err = quotationsalesreturn.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.UpdateReturnedQuantityInQuotationProduct(quotationsalesreturnOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	/*
		err = quotationsalesreturn.AttributesValueChangeEvent(quotationsalesreturnOld)
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	err = quotationsalesreturn.CloseQuotationSalesPayment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["closing_qtn_sales_payment"] = "error closing qtn. sales payment: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	quotationsalesreturn, err = store.FindQuotationSalesReturnByID(&quotationsalesreturn.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find quotationsalesreturn:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotationsalesreturn.CustomerID != nil && !quotationsalesreturn.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(quotationsalesreturn.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	quotation, _ := store.FindQuotationByID(quotationsalesreturn.QuotationID, bson.M{})
	quotation.ReturnAmount, quotation.ReturnCount, _ = store.GetReturnedAmountByQuotationID(quotation.ID)
	quotation.Update()

	quotationsalesreturn.SetCustomerQuotationSalesReturnStats()
	quotation.SetCustomerQuotationStats()

	go quotationsalesreturn.SetProductsQuotationSalesReturnStats()
	go quotationsalesreturnOld.SetProductsQuotationSalesReturnStats()

	go quotationsalesreturn.SetPostBalances()

	go func() {
		quotationsalesreturn.ClearProductsHistory()
		quotationsalesreturn.CreateProductsHistory()
	}()

	store.NotifyUsers("quotationsales_return_updated")

	response.Status = true
	response.Result = quotationsalesreturn
	json.NewEncoder(w).Encode(response)
}

// ViewQuotationSalesReturn : handler function for GET /v1/quotationsalesreturn/<id> call
func ViewQuotationSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	quotationsalesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid QuotationSalesReturn ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var quotationsalesreturn *models.QuotationSalesReturn

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

	quotationsalesreturn, err = store.FindQuotationSalesReturnByID(&quotationsalesreturnID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, _ := store.FindCustomerByID(quotationsalesreturn.CustomerID, bson.M{})
	customer.SetSearchLabel()
	quotationsalesreturn.Customer = customer

	response.Status = true
	response.Result = quotationsalesreturn

	json.NewEncoder(w).Encode(response)
}

// DeleteQuotationSalesReturn : handler function for DELETE /v1/quotationsalesreturn/<id> call
func DeleteQuotationSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	quotationsalesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid QuotationSalesReturn ID:" + err.Error()
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

	quotationsalesreturn, err := store.FindQuotationSalesReturnByID(&quotationsalesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = quotationsalesreturn.DeleteQuotationSalesReturn(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if quotationsalesreturn.Status == "delivered" {
		err = quotationsalesreturn.AddStock()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Status = true
	response.Result = "Deleted successfully"

	json.NewEncoder(w).Encode(response)
}

// CreateQuotation : handler for POST /quotation
func CalculateQuotationSalesReturnNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var quotationsalesReturn *models.QuotationSalesReturn
	// Decode data
	if !utils.Decode(w, r, &quotationsalesReturn) {
		return
	}

	quotationsalesReturn.FindNetTotal()

	response.Status = true
	response.Result = quotationsalesReturn

	json.NewEncoder(w).Encode(response)
}
