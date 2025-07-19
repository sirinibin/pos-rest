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
	"go.mongodb.org/mongo-driver/mongo"
)

// ListPurchase : handler for GET /purchase
func ListPurchase(w http.ResponseWriter, r *http.Request) {
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
	purchases, criterias, err := store.SearchPurchase(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "purchase")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of purchases:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var purchaseStats models.PurchaseStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			purchaseStats, err = store.GetPurchaseStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["purchase_stats"] = "Unable to find purchase stats:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_purchase"] = purchaseStats.NetTotal
	response.Meta["vat_price"] = purchaseStats.VatPrice
	response.Meta["discount"] = purchaseStats.Discount
	response.Meta["cash_discount"] = purchaseStats.CashDiscount
	response.Meta["shipping_handling_fees"] = purchaseStats.ShippingOrHandlingFees
	response.Meta["net_retail_profit"] = purchaseStats.NetRetailProfit
	response.Meta["net_wholesale_profit"] = purchaseStats.NetWholesaleProfit
	response.Meta["paid_purchase"] = purchaseStats.PaidPurchase
	response.Meta["unpaid_purchase"] = purchaseStats.UnPaidPurchase
	response.Meta["cash_purchase"] = purchaseStats.CashPurchase
	response.Meta["bank_account_purchase"] = purchaseStats.BankAccountPurchase
	response.Meta["return_count"] = purchaseStats.ReturnCount
	response.Meta["return_amount"] = purchaseStats.ReturnAmount

	if len(purchases) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = purchases
	}

	json.NewEncoder(w).Encode(response)

}

// CreatePurchase : handler for POST /purchase
func CreatePurchase(w http.ResponseWriter, r *http.Request) {
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

	var purchase *models.Purchase
	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.CreatedBy = &userID
	purchase.UpdatedBy = &userID
	now := time.Now()
	purchase.CreatedAt = &now
	purchase.UpdatedAt = &now
	purchase.FindNetTotal()

	// Validate data
	if errs := purchase.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_vendor_from_name"] = "error creating new vendor from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.FindTotalQuantity()
	purchase.UpdateForeignLabelFields()
	purchase.MakeCode()
	purchase.CalculatePurchaseExpectedProfit()

	err = purchase.Insert()
	if err != nil {
		redisErr := purchase.UnMakeRedisCode()
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

	purchase.CreateProductsPurchaseHistory()

	err = purchase.AddPayments()
	if err != nil {
		response.Status = false
		response.Errors["creating_payments"] = "Error creating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetPaymentStatus()
	purchase.Update()

	err = purchase.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.UpdateProductUnitPriceInStore()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["product_unit_price"] = "Unable to update product unit price:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetProductsPurchaseStats()

	purchase.SetVendorPurchaseStats()

	err = purchase.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchase.VendorID != nil && !purchase.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchase.VendorID, bson.M{})
		if vendor != nil {
			vendor.SetCreditBalance()
		}
	}

	go purchase.SetPostBalances()

	go purchase.CreateProductsHistory()

	store.NotifyUsers("purchase_updated")
	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

// UpdatePurchase : handler function for PUT /v1/purchase call
func UpdatePurchase(w http.ResponseWriter, r *http.Request) {
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

	var purchase *models.Purchase
	var purchaseOld *models.Purchase

	params := mux.Vars(r)

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["purchase_id"] = "Invalid Purchase ID:" + err.Error()
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

	purchaseOld, err = store.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchase"] = "Unable to find purchase:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err = store.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_purchase"] = "Unable to find purchase:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.UpdatedBy = &userID
	now := time.Now()
	purchase.UpdatedAt = &now
	purchase.FindNetTotal()

	// Validate data
	if errs := purchase.Validate(w, r, "update", purchaseOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.CreateNewVendorFromName()
	if err != nil {
		response.Status = false
		response.Errors["new_vendor_from_name"] = "error creating new vendor from name: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.FindTotalQuantity()
	purchase.UpdateForeignLabelFields()
	purchase.CalculatePurchaseExpectedProfit()

	err = purchase.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.ClearProductsPurchaseHistory()
	purchase.CreateProductsPurchaseHistory()

	err = purchase.UpdatePayments()
	if err != nil {
		response.Status = false
		response.Errors["updated_payments"] = "Error updating payments: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetPaymentStatus()
	purchase.Update()

	err = purchaseOld.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["remove_stock"] = "Unable to remove stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["add_stock"] = "Unable to add stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.UpdateProductUnitPriceInStore()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["product_unit_price"] = "Unable to update product unit price:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.AttributesValueChangeEvent(purchaseOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase.SetProductsPurchaseStats()
	purchaseOld.SetProductsPurchaseStats()
	purchase.SetVendorPurchaseStats()

	err = purchase.UndoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["undo_accounting"] = "Error undo accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.DoAccounting()
	if err != nil {
		response.Status = false
		response.Errors["do_accounting"] = "Error do accounting: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	purchase, err = store.FindPurchaseByID(&purchase.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find purchase:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchase.VendorID != nil && !purchase.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchase.VendorID, bson.M{})
		if vendor != nil {
			vendor.SetCreditBalance()
		}
	}

	if purchaseOld.VendorID != nil && !purchaseOld.VendorID.IsZero() {
		vendor, _ := store.FindVendorByID(purchaseOld.VendorID, bson.M{})
		if vendor != nil {
			vendor.SetCreditBalance()
			purchaseOld.SetVendorPurchaseStats()
		}
		purchaseOld.SetProductsPurchaseStats()
	}

	go purchase.SetPostBalances()

	go func() {
		purchase.ClearProductsHistory()
		purchase.CreateProductsHistory()
	}()

	store.NotifyUsers("purchase_updated")
	response.Status = true
	response.Result = purchase
	json.NewEncoder(w).Encode(response)
}

// ViewPurchase : handler function for GET /v1/purchase/<id> call
func ViewPurchase(w http.ResponseWriter, r *http.Request) {
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

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Purchase ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var purchase *models.Purchase

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

	purchase, err = store.FindPurchaseByID(&purchaseID, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if purchase.VendorID != nil && !purchase.VendorID.IsZero() {
		vendor, err := store.FindVendorByID(purchase.VendorID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			w.WriteHeader(http.StatusBadRequest)
			response.Status = false
			response.Errors["view"] = "error fetching vendor:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
		vendor.SetSearchLabel()
		purchase.Vendor = vendor
	}

	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}

// DeletePurchase : handler function for DELETE /v1/purchase/<id> call
func DeletePurchase(w http.ResponseWriter, r *http.Request) {
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

	purchaseID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid Purchase ID:" + err.Error()
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

	purchase, err := store.FindPurchaseByID(&purchaseID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = purchase.DeletePurchase(tokenClaims)
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
func CalculatePurchaseNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var purchase *models.Purchase
	// Decode data
	if !utils.Decode(w, r, &purchase) {
		return
	}

	purchase.FindNetTotal()

	response.Status = true
	response.Result = purchase

	json.NewEncoder(w).Encode(response)
}
