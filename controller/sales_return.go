package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// ListSalesReturn : handler for GET /salesreturn
func ListSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	salesreturns := []models.SalesReturn{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	salesreturns, criterias, err := store.SearchSalesReturn(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find salesreturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "salesreturn")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of salesreturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var salesReturnStats models.SalesReturnStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			salesReturnStats, err = store.GetSalesReturnStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_return_sales"] = "Unable to find total amount of sales return:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_sales_return"] = salesReturnStats.NetTotal
	response.Meta["net_profit"] = salesReturnStats.NetProfit
	response.Meta["net_loss"] = salesReturnStats.NetLoss
	response.Meta["vat_price"] = salesReturnStats.VatPrice
	response.Meta["discount"] = salesReturnStats.Discount
	response.Meta["cash_discount"] = salesReturnStats.CashDiscount
	response.Meta["paid_sales_return"] = salesReturnStats.PaidSalesReturn
	response.Meta["unpaid_sales_return"] = salesReturnStats.UnPaidSalesReturn
	response.Meta["cash_sales_return"] = salesReturnStats.CashSalesReturn
	response.Meta["bank_account_sales_return"] = salesReturnStats.BankAccountSalesReturn
	response.Meta["shipping_handling_fees"] = salesReturnStats.ShippingOrHandlingFees

	if len(salesreturns) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = salesreturns
	}

	json.NewEncoder(w).Encode(response)

}

// CreateSalesReturn : handler for POST /salesreturn
func CreateSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var salesreturn *models.SalesReturn
	// Decode data
	if !utils.Decode(w, r, &salesreturn) {
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

	salesreturn.CreatedBy = &userID
	salesreturn.UpdatedBy = &userID
	now := time.Now()
	salesreturn.CreatedAt = &now
	salesreturn.UpdatedAt = &now
	salesreturn.FindNetTotal()

	// Validate data
	if errs := salesreturn.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.FindTotalQuantity()
	salesreturn.UpdateForeignLabelFields()
	salesreturn.CalculateSalesReturnProfit()

	err = salesreturn.MakeCode()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["code"] = "Error making code: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	salesreturn.UUID = uuid.New().String()

	if salesreturn.EnableReportToZatca && !IsConnectedToInternet() {
		response.Status = false
		response.Errors["reporting_to_zatca"] = "not connected to internet"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if store.Zatca.Phase == "2" && store.Zatca.Connected && salesreturn.EnableReportToZatca {
		err = salesreturn.ReportToZatca()
		if err != nil {
			redisErr := salesreturn.UnMakeCode()
			if redisErr != nil {
				response.Errors["error_unmaking_code"] = "error_unmaking_code: " + redisErr.Error()
			}
			response.Status = false
			response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	err = salesreturn.Insert()
	if err != nil {
		redisErr := salesreturn.UnMakeCode()
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

	salesreturn.UpdateOrderReturnCount()
	salesreturn.UpdateOrderReturnDiscount(nil)
	salesreturn.UpdateOrderReturnCashDiscount(nil)
	salesreturn.CreateProductsSalesReturnHistory()
	/*
		if salesreturn.PaymentStatus != "not_paid" {
			salesreturn.AddPayment()
		}
	*/

	err = salesreturn.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.SetPaymentStatus()
	salesreturn.Update()

	err = salesreturn.AddStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.UpdateReturnedQuantityInOrderProduct(nil)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.SetProductsSalesReturnStats()
	salesreturn.SetCustomerSalesReturnStats()

	err = salesreturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order, _ := store.FindOrderByID(salesreturn.OrderID, bson.M{})
	order.ReturnAmount, order.ReturnCount, _ = store.GetReturnedAmountByOrderID(order.ID)
	order.Update()

	if salesreturn.CustomerID != nil && !salesreturn.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(salesreturn.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	store.NotifyUsers("sales_return_updated")

	response.Status = true
	response.Result = salesreturn

	json.NewEncoder(w).Encode(response)
}

// UpdateSalesReturn : handler function for PUT /v1/salesreturn call
func UpdateSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var salesreturn *models.SalesReturn
	var salesreturnOld *models.SalesReturn

	params := mux.Vars(r)

	salesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["salesreturn_id"] = "Invalid SalesReturn ID:" + err.Error()
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

	salesreturnOld, err = store.FindSalesReturnByID(&salesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_salesreturn"] = "Unable to find salesreturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn, err = store.FindSalesReturnByID(&salesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_salesreturn"] = "Unable to find salesreturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &salesreturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.UpdatedBy = &userID
	now := time.Now()
	salesreturn.UpdatedAt = &now
	salesreturn.FindNetTotal()

	// Validate data
	if errs := salesreturn.Validate(w, r, "update", salesreturnOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.FindTotalQuantity()
	salesreturn.UpdateForeignLabelFields()
	salesreturn.CalculateSalesReturnProfit()

	/*
		if store.Zatca.Phase == "2" && store.Zatca.Connected {

			err = salesreturn.ReportToZatca()
			if err != nil {
				response.Status = false
				response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(response)
				return
			}
		}*/

	err = salesreturn.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.UpdateOrderReturnCount()
	salesreturn.UpdateOrderReturnDiscount(salesreturnOld)
	salesreturn.UpdateOrderReturnCashDiscount(salesreturnOld)

	salesreturn.ClearProductsSalesReturnHistory()
	salesreturn.CreateProductsSalesReturnHistory()
	//count, _ := salesreturn.GetPaymentsCount()

	/*
		if count == 1 && salesreturn.PaymentStatus == "paid" {
			salesreturn.ClearPayments()
			salesreturn.AddPayment()
		}
	*/

	err = salesreturn.UpdatePayments()
	if err != nil {
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn.SetPaymentStatus()
	salesreturn.Update()

	err = salesreturnOld.RemoveStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.AddStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.UpdateReturnedQuantityInOrderProduct(salesreturnOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	/*
		err = salesreturn.AttributesValueChangeEvent(salesreturnOld)
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	salesreturn.SetProductsSalesReturnStats()
	salesreturn.SetCustomerSalesReturnStats()

	err = salesreturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order, _ := store.FindOrderByID(salesreturn.OrderID, bson.M{})
	order.ReturnAmount, order.ReturnCount, _ = store.GetReturnedAmountByOrderID(order.ID)
	order.Update()

	err = salesreturn.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesreturn, err = store.FindSalesReturnByID(&salesreturn.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find salesreturn:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if salesreturn.CustomerID != nil && !salesreturn.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(salesreturn.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	store.NotifyUsers("sales_return_updated")

	response.Status = true
	response.Result = salesreturn
	json.NewEncoder(w).Encode(response)
}

// ViewSalesReturn : handler function for GET /v1/salesreturn/<id> call
func ViewSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	salesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid SalesReturn ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var salesreturn *models.SalesReturn

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

	salesreturn, err = store.FindSalesReturnByID(&salesreturnID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, _ := store.FindCustomerByID(salesreturn.CustomerID, bson.M{})
	customer.SetSearchLabel()
	salesreturn.Customer = customer

	response.Status = true
	response.Result = salesreturn

	json.NewEncoder(w).Encode(response)
}

// DeleteSalesReturn : handler function for DELETE /v1/salesreturn/<id> call
func DeleteSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	salesreturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid SalesReturn ID:" + err.Error()
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

	salesreturn, err := store.FindSalesReturnByID(&salesreturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = salesreturn.DeleteSalesReturn(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if salesreturn.Status == "delivered" {
		err = salesreturn.AddStock()
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

// CreateOrder : handler for POST /order
func CalculateSalesReturnNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var salesReturn *models.SalesReturn
	// Decode data
	if !utils.Decode(w, r, &salesReturn) {
		return
	}

	salesReturn.FindNetTotal()

	response.Status = true
	response.Result = salesReturn

	json.NewEncoder(w).Encode(response)
}
