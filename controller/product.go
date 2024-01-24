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

// ListProduct : handler for GET /product
func ListProductJson(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var response models.Response
	response.Errors = make(map[string]string)

	products, err := models.GetBarTenderProducts(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["find"] = "Unable to find products:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	json.NewEncoder(w).Encode(products)

}

// ListProduct : handler for GET /product
func ListProduct(w http.ResponseWriter, r *http.Request) {
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

	products := []models.Product{}

	products, criterias, err := models.SearchProduct(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find products:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var storeID primitive.ObjectID
	keys, ok := r.URL.Query()["search[store_id]"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			response.Status = false
			response.Errors["store_id"] = "Invalid store_id:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	var productStats models.ProductStats

	keys, ok = r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "product")
			if err != nil {
				response.Status = false
				response.Errors["total_count"] = "Unable to find total count of products:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}

			productStats, err = models.GetProductStats(criterias.SearchBy, storeID)
			if err != nil {
				response.Status = false
				response.Errors["product_stats"] = "Unable to find product stats:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["stock"] = productStats.Stock
	response.Meta["retail_stock_value"] = productStats.RetailStockValue
	response.Meta["wholesale_stock_value"] = productStats.WholesaleStockValue
	response.Meta["purchase_stock_value"] = productStats.PurchaseStockValue

	if len(products) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = products
	}

	json.NewEncoder(w).Encode(response)

}

// CreateProduct : handler for POST /product
func CreateProduct(w http.ResponseWriter, r *http.Request) {
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

	var product *models.Product
	// Decode data
	if !utils.Decode(w, r, &product) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	product.CreatedBy = &userID
	product.UpdatedBy = &userID
	now := time.Now()
	product.CreatedAt = &now
	product.UpdatedAt = &now

	// Validate data
	if errs := product.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	product.SetPartNumber()
	product.SetBarcode()
	product.SaveImages()
	product.UpdateForeignLabelFields()
	product.InitStoreUnitPrice()
	product.CalculateUnitProfit()

	err = product.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = product

	json.NewEncoder(w).Encode(response)
}

// UpdateProduct : handler function for PUT /v1/product call
func UpdateProduct(w http.ResponseWriter, r *http.Request) {
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

	var product *models.Product
	//var productOld *models.Product

	params := mux.Vars(r)

	productID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Product ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	/*
		productOld, err = models.FindProductByID(&productID, bson.M{})
		if err != nil {
			response.Status = false
			response.Errors["product"] = "Unable to find product:" + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	product, err = models.FindProductByID(&productID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["product"] = "Unable to find product:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &product) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	product.UpdatedBy = &userID
	now := time.Now()
	product.UpdatedAt = &now

	// Validate data
	if errs := product.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	product.SaveImages()
	product.UpdateForeignLabelFields()
	product.SetBarcode()
	product.SetPartNumber()
	product.InitStoreUnitPrice()
	product.CalculateUnitProfit()

	err = product.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}
	product.ReflectValidPurchaseUnitPrice()
	/*
		err = product.AttributesValueChangeEvent(productOld)
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
	*/

	product, err = models.FindProductByID(&product.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find product:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	product.SetProductSalesStatsByStoreID(*product.StoreID)
	product.SetProductSalesReturnStatsByStoreID(*product.StoreID)
	product.SetProductPurchaseStatsByStoreID(*product.StoreID)
	product.SetProductPurchaseReturnStatsByStoreID(*product.StoreID)
	product.SetProductQuotationStatsByStoreID(*product.StoreID)
	product.SetProductDeliveryNoteStatsByStoreID(*product.StoreID)

	response.Status = true
	response.Result = product
	json.NewEncoder(w).Encode(response)
}

// ViewProduct : handler function for GET /v1/product/<id> call
func ViewProduct(w http.ResponseWriter, r *http.Request) {
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

	productID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Errors["product_id"] = "Invalid Product ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var product *models.Product

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	product, err = models.FindProductByID(&productID, selectFields)
	if err != nil {
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var storeID primitive.ObjectID

	keys, ok = r.URL.Query()["store_id"]
	if ok && len(keys[0]) >= 1 {
		storeID, err = primitive.ObjectIDFromHex(keys[0])
		if err != nil {
			response.Errors["store_id"] = "Invalid store_id" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	err = product.GenerateBarCodeBase64ByStoreID(storeID)
	if err != nil {
		response.Errors["store_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = product

	json.NewEncoder(w).Encode(response)
}

// ViewProduct : handler function for GET /v1/product/code/<code> call
func ViewProductByItemCode(w http.ResponseWriter, r *http.Request) {
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

	itemCode := params["code"]
	if itemCode == "" {
		response.Status = false
		response.Errors["item_code"] = "Invalid Item Code:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var product *models.Product

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	product, err = models.FindProductByItemCode(itemCode, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = product

	json.NewEncoder(w).Encode(response)
}

// DeleteProduct : handler function for DELETE /v1/product/<id> call
func DeleteProduct(w http.ResponseWriter, r *http.Request) {
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

	productID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Product ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	product, err := models.FindProductByID(&productID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = product.DeleteProduct(tokenClaims)
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

// ViewProduct : handler function for GET /v1/product/barcode/<barcode> call
func ViewProductByBarCode(w http.ResponseWriter, r *http.Request) {
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

	itemCode := params["barcode"]
	if itemCode == "" {
		response.Status = false
		response.Errors["bar_code"] = "Invalid Bar Code:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var product *models.Product

	selectFields := map[string]interface{}{}
	keys, ok := r.URL.Query()["select"]
	if ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	product, err = models.FindProductByBarCode(itemCode, selectFields)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = product

	json.NewEncoder(w).Encode(response)
}
