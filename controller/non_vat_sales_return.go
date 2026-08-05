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

func ListNonVATSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	items, criterias, err := store.SearchNonVATSalesReturn(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales returns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "non_vat_sales_return")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var nonVATSalesReturnStats models.NonVATSalesReturnStats
	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 && keys[0] == "1" {
		nonVATSalesReturnStats, err = store.GetNonVATSalesReturnStats(criterias.SearchBy)
		if err != nil {
			response.Status = false
			response.Errors["stats"] = "Unable to get non vat sales return stats:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	response.Meta["total_non_vat_sales_return"] = nonVATSalesReturnStats.NetTotal
	response.Meta["net_profit"] = nonVATSalesReturnStats.NetProfit
	response.Meta["net_loss"] = nonVATSalesReturnStats.NetLoss
	response.Meta["vat_price"] = nonVATSalesReturnStats.VatPrice
	response.Meta["discount"] = nonVATSalesReturnStats.Discount
	response.Meta["cash_discount"] = nonVATSalesReturnStats.CashDiscount
	response.Meta["paid_non_vat_sales_return"] = nonVATSalesReturnStats.PaidNonVATSalesReturn
	response.Meta["unpaid_non_vat_sales_return"] = nonVATSalesReturnStats.UnPaidNonVATSalesReturn
	response.Meta["cash_non_vat_sales_return"] = nonVATSalesReturnStats.CashNonVATSalesReturn
	response.Meta["bank_account_non_vat_sales_return"] = nonVATSalesReturnStats.BankAccountNonVATSalesReturn
	response.Meta["shipping_handling_fees"] = nonVATSalesReturnStats.ShippingOrHandlingFees
	response.Meta["non_vat_sales_non_vat_sales_return"] = nonVATSalesReturnStats.NonVATSalesNonVATSalesReturn

	if len(items) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = items
	}
	json.NewEncoder(w).Encode(response)
}

func CreateNonVATSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	var item *models.NonVATSalesReturn
	if !utils.Decode(w, r, &item) {
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

	item.CreatedBy = &userID
	item.UpdatedBy = &userID
	now := time.Now()
	item.CreatedAt = &now
	item.UpdatedAt = &now

	if err = item.FindNetTotal(); err != nil {
		response.Status = false
		response.Errors["net_total"] = "error calculating net total: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	queue := GetOrCreateQueue(store.ID.Hex(), "non_vat_sales_return")
	queueToken := generateQueueToken()
	queue.Enqueue(Request{Token: queueToken})
	queue.WaitUntilMyTurn(queueToken)

	if errs := item.Validate(w, r, "create"); len(errs) > 0 {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales_return")
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	item.FindTotalQuantity()
	item.ProcessPayments()

	if err = item.UpdateForeignLabelFields(); err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales_return")
		response.Status = false
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err = item.MakeRedisCode(); err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales_return")
		response.Status = false
		response.Errors["code"] = "error making code: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err = item.Insert(); err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales_return")
		_ = item.UnMakeRedisCode()
		response.Status = false
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	queue.Pop()
	CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales_return")

	// Update return count on the parent non_vat_sales document
	if item.NonVATSalesID != nil && !item.NonVATSalesID.IsZero() {
		parent, err := store.FindNonVATSalesByID(item.NonVATSalesID, bson.M{})
		if err == nil {
			parent.ReturnCount++
			parent.ReturnAmount += item.NetTotal
			_ = parent.Update()
		}
	}

	go func() {
		_ = item.CreateProductsNonVATSalesReturnHistory()
		_ = item.SetProductsStock()
		_ = item.DoAccounting()
		_ = item.SetCustomerNonVATSalesReturnStats()
	}()

	response.Status = true
	response.Result = item
	json.NewEncoder(w).Encode(response)
}

func ViewNonVATSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)
	ID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	item, err := store.FindNonVATSalesReturnByID(&ID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	item.Store = store

	response.Status = true
	response.Result = item
	json.NewEncoder(w).Encode(response)
}

func UpdateNonVATSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)
	ID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	itemOld, err := store.FindNonVATSalesReturnByID(&ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var item *models.NonVATSalesReturn
	if !utils.Decode(w, r, &item) {
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

	item.UpdatedBy = &userID
	now := time.Now()
	item.UpdatedAt = &now
	item.Code = itemOld.Code
	item.CreatedAt = itemOld.CreatedAt
	item.CreatedBy = itemOld.CreatedBy

	if err = item.FindNetTotal(); err != nil {
		response.Status = false
		response.Errors["net_total"] = "error calculating net total: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if errs := item.Validate(w, r, "update"); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	item.FindTotalQuantity()
	item.ProcessPayments()

	if err = item.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err = item.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go func() {
		_ = item.UndoAccounting()
		_ = item.ClearProductsNonVATSalesReturnHistory()
		_ = item.CreateProductsNonVATSalesReturnHistory()
		_ = item.SetProductsStock()
		_ = item.DoAccounting()
		_ = item.SetCustomerNonVATSalesReturnStats()
	}()

	response.Status = true
	response.Result = item
	json.NewEncoder(w).Encode(response)
}

func DeleteNonVATSalesReturn(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)
	ID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	item, err := store.FindNonVATSalesReturnByID(&ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales return:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if err = item.Delete(tokenClaims); err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go func() {
		_ = item.UndoAccounting()
		_ = item.ClearProductsNonVATSalesReturnHistory()
		_ = item.SetProductsStock()
		_ = item.SetCustomerNonVATSalesReturnStats()
	}()

	response.Status = true
	response.Result = "Deleted successfully"
	json.NewEncoder(w).Encode(response)
}

func CalculateNonVATSalesReturnNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var ret *models.NonVATSalesReturn
	if !utils.Decode(w, r, &ret) {
		return
	}

	if err := ret.FindNetTotal(); err != nil {
		response.Status = false
		response.Errors["net_total"] = "Unable to calculate net total:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = ret
	json.NewEncoder(w).Encode(response)
}
