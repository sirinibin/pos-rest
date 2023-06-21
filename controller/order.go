package controller

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
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

	orders, criterias, err := models.SearchOrder(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "order")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	salesStats, err := models.GetSalesStats(criterias.SearchBy)
	if err != nil {
		response.Status = false
		response.Errors["total_sales"] = "Unable to find total amount of orders:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_sales"] = salesStats.NetTotal
	response.Meta["net_profit"] = salesStats.NetProfit
	response.Meta["loss"] = salesStats.Loss
	response.Meta["vat_price"] = salesStats.VatPrice
	response.Meta["discount"] = salesStats.Discount
	response.Meta["shipping_handling_fees"] = salesStats.ShippingOrHandlingFees

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

	order.CreatedBy = &userID
	order.UpdatedBy = &userID
	now := time.Now()
	order.CreatedAt = &now
	order.UpdatedAt = &now

	// Validate data
	if errs := order.Validate(w, r, "create", nil); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	order.FindNetTotal()
	order.FindTotal()
	order.FindTotalQuantity()
	order.FindVatPrice()

	order.UpdateForeignLabelFields()

	order.ID = primitive.NewObjectID()
	if len(order.Code) == 0 {
		order.MakeCode()
	}

	order.CalculateOrderProfit()

	err = order.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	order.CreateProductsSalesHistory()
	order.AddPayment()

	err = order.RemoveStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to update stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

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

	orderOld, err = models.FindOrderByID(&orderID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_order"] = "Unable to find order:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	order, err = models.FindOrderByID(&orderID, bson.M{})
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

	// Validate data
	if errs := order.Validate(w, r, "update", orderOld); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	order.FindNetTotal()
	order.FindTotal()
	order.FindTotalQuantity()
	order.FindVatPrice()

	order.UpdateForeignLabelFields()
	order.CalculateOrderProfit()
	order.GetPayments()

	err = order.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	order.ClearProductsSalesHistory()
	order.CreateProductsSalesHistory()
	count, _ := order.GetPaymentsCount()
	if count == 1 {
		order.ClearPayments()
		order.AddPayment()
	}

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

	order, err = models.FindOrderByID(&order.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find order:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

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

	order, err = models.FindOrderByID(&orderID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	order.SetChangeLog("view", nil, nil, nil)

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

	order, err := models.FindOrderByID(&orderID, bson.M{})
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

	if order.Status == "delivered" && !order.Deleted {
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
