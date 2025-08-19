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

// ListPurchaseReturn : handler for GET /purchasereturn
func ListPurchaseReturn(w http.ResponseWriter, r *http.Request) {
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

	purchasereturns := []models.PurchaseReturn{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	purchasereturns, criterias, err := store.SearchPurchaseReturn(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchasereturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "purchasereturn")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchasereturns:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var purchaseReturnStats models.PurchaseReturnStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			purchaseReturnStats, err = store.GetPurchaseReturnStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["purchase_return_stats"] = "Unable to find purchase return stats:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_purchase_return"] = purchaseReturnStats.NetTotal
	response.Meta["vat_price"] = purchaseReturnStats.VatPrice
	response.Meta["discount"] = purchaseReturnStats.Discount
	response.Meta["cash_discount"] = purchaseReturnStats.CashDiscount
	response.Meta["shipping_handling_fees"] = purchaseReturnStats.ShippingOrHandlingFees
	response.Meta["paid_purchase_return"] = purchaseReturnStats.PaidPurchaseReturn
	response.Meta["unpaid_purchase_return"] = purchaseReturnStats.UnPaidPurchaseReturn
	response.Meta["cash_purchase_return"] = purchaseReturnStats.CashPurchaseReturn
	response.Meta["bank_account_purchase_return"] = purchaseReturnStats.BankAccountPurchaseReturn
	response.Meta["shipping_handling_fees"] = purchaseReturnStats.ShippingOrHandlingFees

	if len(purchasereturns) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchasereturns
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchaseReturn : handler for POST /purchasereturn
func CreatePurchaseReturn(w http.ResponseWriter, r *http.Request) {
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

	var purchasereturn *models.PurchaseReturn
	// Decode data
	if !utils.Decode(w, r, &purchasereturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturn.CreatedBy = &userID
	purchasereturn.UpdatedBy = &userID
	now := time.Now()
	purchasereturn.CreatedAt = &now
	purchasereturn.UpdatedAt = &now
	purchasereturn.FindNetTotal()

	// Validate data
	if errs := purchasereturn.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	//Queue
	queue := GetOrCreateQueue(store.ID.Hex(), "purchase_return")
	queueToken := generateQueueToken()
	queue.Enqueue(Request{Token: queueToken})
	queue.WaitUntilMyTurn(queueToken)

	purchasereturn.FindTotalQuantity()

	err = purchasereturn.UpdateForeignLabelFields()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase_return")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.MakeCode()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase_return")
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["code"] = "error making code: " + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.Insert()
	if err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "purchase_return")
		redisErr := purchasereturn.UnMakeRedisCode()
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
	CleanupQueueIfEmpty(store.ID.Hex(), "purchase_return")

	purchasereturn.UpdatePurchaseReturnDiscount(false)
	purchasereturn.CreateProductsPurchaseReturnHistory()

	err = purchasereturn.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	purchasereturn.SetPaymentStatus()
	purchasereturn.Update()

	err = purchasereturn.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.UpdateReturnedQuantityInPurchaseProduct(nil)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.ClosePurchasePayment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["closing_purchase_payment"] = "error closing purchase payment: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !store.Settings.DisablePurchasesOnAccounts {
		err = purchasereturn.DoAccounting()
		if err != nil {
			response.Status = false
			response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	if purchasereturn.VendorID != nil && !purchasereturn.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchasereturn.VendorID, bson.M{})
		if vendor != nil {
			vendor.SetCreditBalance()
		}
	}

	purchase, _ := store.FindPurchaseByID(purchasereturn.PurchaseID, bson.M{})
	purchase.ReturnAmount, purchase.ReturnCount, _ = store.GetReturnedAmountByPurchaseID(purchase.ID)
	purchase.Update()

	go purchasereturn.UpdatePurchaseReturnCount()
	go purchasereturn.SetProductsPurchaseReturnStats()
	go purchasereturn.SetVendorPurchaseReturnStats()
	go purchase.SetVendorPurchaseStats()

	go purchasereturn.SetPostBalances()

	go purchasereturn.CreateProductsHistory()

	store.NotifyUsers("purchase_return_updated")

	response.Status = true
	response.Result = purchasereturn

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchaseReturn : handler function for PUT /v1/purchasereturn call
func UpdatePurchaseReturn(w http.ResponseWriter, r *http.Request) {
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

	var purchasereturn *models.PurchaseReturn
	var purchasereturnOld *models.PurchaseReturn

	params := mux.Vars(r)

	purchasereturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["purchasereturn_id"] = "Invalid PurchaseReturn ID:" + err.Error()
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

	purchasereturnOld, err = store.FindPurchaseReturnByID(&purchasereturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchasereturn"] = "Unable to find purchasereturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturn, err = store.FindPurchaseReturnByID(&purchasereturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchasereturn"] = "Unable to find purchasereturn:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchasereturn) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturn.UpdatedBy = &userID
	now := time.Now()
	purchasereturn.UpdatedAt = &now
	purchasereturn.FindNetTotal()

	// Validate data
	if errs := purchasereturn.Validate(w, r, "update", purchasereturnOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturn.FindTotalQuantity()
	purchasereturn.UpdateForeignLabelFields()

	err = purchasereturn.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturn.ClearProductsPurchaseReturnHistory()
	purchasereturn.CreateProductsPurchaseReturnHistory()

	err = purchasereturn.UpdatePayments()
	if err != nil {
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchasereturn.SetPaymentStatus()
	purchasereturn.Update()

	err = purchasereturn.UpdatePurchaseReturnDiscount(true)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_purchase_return"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturnOld.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_old_stock"] = "Unable to add old stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.UpdateReturnedQuantityInPurchaseProduct(purchasereturnOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update_returned_quantity"] = "Unable to update returned quantity:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.ClosePurchasePayment()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		response.Status = false
		response.Errors["closing_purchase_payment"] = "error closing purchase payment: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !store.Settings.DisablePurchasesOnAccounts {
		err = purchasereturn.DoAccounting()
		if err != nil {
			response.Status = false
			response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	purchasereturn, err = store.FindPurchaseReturnByID(&purchasereturn.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchasereturn:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchasereturn.VendorID != nil && !purchasereturn.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchasereturn.VendorID, bson.M{})
		if vendor != nil {
			vendor.SetCreditBalance()
		}
	}

	purchase, _ := store.FindPurchaseByID(purchasereturn.PurchaseID, bson.M{})
	purchase.ReturnAmount, purchase.ReturnCount, _ = store.GetReturnedAmountByPurchaseID(purchase.ID)
	purchase.Update()

	go purchasereturn.UpdatePurchaseReturnCount()
	go purchasereturn.SetProductsPurchaseReturnStats()
	go purchasereturnOld.SetProductsPurchaseReturnStats()
	go purchasereturn.SetVendorPurchaseReturnStats()
	go purchase.SetVendorPurchaseStats()
	go purchasereturn.SetPostBalances()

	go func() {
		purchasereturn.ClearProductsHistory()
		purchasereturn.CreateProductsHistory()
	}()

	store.NotifyUsers("purchase_return_updated")

	response.Status = true
	response.Result = purchasereturn
	json.NewEncoder(w).Encode(response)
}

// ViewPurchaseReturn : handler function for GET /v1/purchasereturn/<id> call
func ViewPurchaseReturn(w http.ResponseWriter, r *http.Request) {
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

	purchasereturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid PurchaseReturn ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchasereturn *models.PurchaseReturn

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

	purchasereturn, err = store.FindPurchaseReturnByID(&purchasereturnID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	vendor, _ := store.FindVendorByID(purchasereturn.VendorID, bson.M{})
	vendor.SetSearchLabel()
	purchasereturn.Vendor = vendor

	response.Status = true
	response.Result = purchasereturn

	json.NewEncoder(w).Encode(response)
}

// DeletePurchaseReturn : handler function for DELETE /v1/purchasereturn/<id> call
func DeletePurchaseReturn(w http.ResponseWriter, r *http.Request) {
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

	purchasereturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid PurchaseReturn ID:" + err.Error()
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

	purchasereturn, err := store.FindPurchaseReturnByID(&purchasereturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchasereturn.DeletePurchaseReturn(tokenClaims)
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
func CalculatePurchaseReturnNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var purchaseReturn *models.PurchaseReturn
	// Decode data
	if !utils.Decode(w, r, &purchaseReturn) {
		return
	}

	purchaseReturn.FindNetTotal()

	response.Status = true
	response.Result = purchaseReturn

	json.NewEncoder(w).Encode(response)
}
