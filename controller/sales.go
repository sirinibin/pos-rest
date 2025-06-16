package controller

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ListOrder : handler for GET /order
func ListOrder(w http.ResponseWriter, r *http.Request) {
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

	orders := []models.Order{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	orders, criterias, err := store.SearchOrder(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "order")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var salesStats models.SalesStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			salesStats, err = store.GetSalesStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_sales"] = "Unable to find total amount of orders:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_sales"] = salesStats.NetTotal
	response.Meta["net_profit"] = salesStats.NetProfit
	response.Meta["net_loss"] = salesStats.NetLoss
	response.Meta["vat_price"] = salesStats.VatPrice
	response.Meta["discount"] = salesStats.Discount
	response.Meta["cash_discount"] = salesStats.CashDiscount
	response.Meta["shipping_handling_fees"] = salesStats.ShippingOrHandlingFees
	response.Meta["paid_sales"] = salesStats.PaidSales
	response.Meta["unpaid_sales"] = salesStats.UnPaidSales
	response.Meta["cash_sales"] = salesStats.CashSales
	response.Meta["bank_account_sales"] = salesStats.BankAccountSales
	response.Meta["return_count"] = salesStats.ReturnCount
	response.Meta["return_amount"] = salesStats.ReturnAmount

	if len(orders) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = orders
	}

	json.NewEncoder(w).Encode(response)

}

// CreateOrder : handler for POST /order
func CreateOrder(w http.ResponseWriter, r *http.Request) {
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

	var order *models.Order
	// Decode data
	if !utils.Decode(w, r, &order) {
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
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.CreatedBy = &userID
	order.UpdatedBy = &userID
	now := time.Now()
	order.CreatedAt = &now
	order.UpdatedAt = &now

	//log.Print("order.SkipZatcaReporting:")
	//log.Print(order.SkipZatcaReporting)
	// Validate data
	order.FindNetTotal()

	if errs := order.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	if order.EnableReportToZatca && !IsConnectedToInternet() {
		response.Status = false
		response.Errors["reporting_to_zatca"] = "not connected to internet"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.CreateNewCustomerFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_customer_from_name"] = "error creating new customer from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.FindTotalQuantity()

	err = order.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.MakeCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error making code: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.CalculateOrderProfit()
	if err != nil {
		response.Status = false
		response.Errors["profit"] = "Error calculating order profit: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.UUID = uuid.New().String()

	if store.Zatca.Phase == "2" && store.Zatca.Connected && order.EnableReportToZatca {
		err = order.ReportToZatca()
		if err != nil {
			log.Print("reporting failed")
			redisErr := order.UnMakeRedisCode()
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

	err = order.Insert()
	if err != nil {
		redisErr := order.UnMakeRedisCode()
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

	err = order.LinkQuotation()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["link_quotation"] = "error linking quotation:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.CreateProductsSalesHistory()
	if err != nil {
		response.Status = false
		response.Errors["product"] = "Error creating products sales history: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	_, err = order.SetPaymentStatus()
	if err != nil {
		response.Status = false
		response.Errors["order"] = "Error getting payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.Update()
	if err != nil {
		response.Status = false
		response.Errors["order"] = "Error updating order: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.RemoveStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.SetProductsSalesStats()
	if err != nil {
		response.Status = false
		response.Errors["product"] = "Error setting product sales stats: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.SetCustomerSalesStats()
	if err != nil {
		response.Status = false
		response.Errors["customer"] = "Error setting customer sales stats: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = order

	err = order.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if order.CustomerID != nil && !order.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(order.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	store.NotifyUsers("sales_updated")

	json.NewEncoder(w).Encode(response)
}

// CreateOrder : handler for POST /order
func CalculateSalesNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var order *models.Order
	// Decode data
	if !utils.Decode(w, r, &order) {
		return
	}

	order.FindNetTotal()

	response.Status = true
	response.Result = order

	json.NewEncoder(w).Encode(response)
}

// UpdateOrder : handler function for PUT /v1/order call
func UpdateOrder(w http.ResponseWriter, r *http.Request) {
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

	var order *models.Order
	var orderOld *models.Order

	params := mux.Vars(r)

	orderID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["order_id"] = "Invalid Order ID:" + err.Error()
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

	orderOld, err = store.FindOrderByID(&orderID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_order"] = "Unable to find order:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	order, err = store.FindOrderByID(&orderID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_order"] = "Unable to find order:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &order) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.UpdatedBy = &userID
	now := time.Now()
	order.UpdatedAt = &now
	order.FindNetTotal()

	// Validate data
	if errs := order.Validate(w, r, "update", orderOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.CreateNewCustomerFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_customer_from_name"] = "error creating new customer from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.FindTotalQuantity()
	order.UpdateForeignLabelFields()
	order.CalculateOrderProfit()
	//order.GetPayments()
	/*
		if store.Zatca.Phase == "2" && store.Zatca.Connected && !order.SkipZatcaReporting {
			err = order.ReportToZatca()
			if err != nil {
				response.Status = false
				response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(response)
				return
			}
		}*/

	err = order.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.LinkQuotation()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["link_quotation"] = "error linking quotation:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.UpdateSalesReturnCustomer()
	if err != nil {
		response.Status = false
		response.Errors["updating_sales_return_customer"] = "Error updating sales returns " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.ClearProductsSalesHistory()
	order.CreateProductsSalesHistory()

	/*count, _ := order.GetPaymentsCount()
	if count == 1 && order.PaymentStatus == "paid" {
		order.ClearPayments()
		order.AddPayment()
	}
	*/

	err = order.UpdatePayments()
	if err != nil {
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	order.SetPaymentStatus()
	order.Update()

	err = orderOld.AddStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.RemoveStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	order.SetProductsSalesStats()
	order.SetCustomerSalesStats()

	order, err = store.FindOrderByID(&order.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find order:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if order.CustomerID != nil && !order.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(order.CustomerID, bson.M{})
		if customer != nil {
			customer.SetCreditBalance()
		}
	}

	if orderOld.CustomerID != nil && !orderOld.CustomerID.IsZero() {
		customer, _ := store.FindCustomerByID(orderOld.CustomerID, bson.M{})
		if customer != nil {
			log.Print("Setting old customer credit balance")
			customer.SetCreditBalance()
			orderOld.SetCustomerSalesStats()
		}
		orderOld.SetProductsSalesStats()
	}

	/*
		err = order.RectifyBalances()
		if err != nil {
			response.Status = false
			response.Errors["rectifying_balances"] = "Error rectifying balances: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}*/

	/*
		if store.Zatca.Phase == "2" && store.Zatca.Connected {
			err = order.ReportToZatca()
			if err != nil {
				response.Status = false
				response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}*/

	store.NotifyUsers("sales_updated")
	response.Status = true
	response.Result = order
	json.NewEncoder(w).Encode(response)
}

// ViewOrder : handler function for GET /v1/order/<id> call
func ViewOrder(w http.ResponseWriter, r *http.Request) {
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

	orderID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Order ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var order *models.Order

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

	order, err = store.FindOrderByID(&orderID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, _ := store.FindCustomerByID(order.CustomerID, bson.M{})
	customer.SetSearchLabel()
	order.Customer = customer

	response.Status = true
	response.Result = order

	json.NewEncoder(w).Encode(response)
}

// DeleteOrder : handler function for DELETE /v1/order/<id> call
func DeleteOrder(w http.ResponseWriter, r *http.Request) {
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

	orderID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid Order ID:" + err.Error()
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

	order, err := store.FindOrderByID(&orderID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = order.DeleteOrder(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if order.Status == "delivered" {
		err = order.AddStock()
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
