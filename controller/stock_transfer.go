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
	"go.mongodb.org/mongo-driver/mongo"
)

// ListStockTransfer : handler for GET /stocktransfer
func ListStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	stocktransfers := []models.StockTransfer{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id(parsing 2):" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	stocktransfers, criterias, err := store.SearchStockTransfer(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find stocktransfers:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "stocktransfer")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of stocktransfers:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias

	var stocktransferStats models.StockTransferStats

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 {
		if keys[0] == "1" {
			stocktransferStats, err = store.GetStockTransferStats(criterias.SearchBy)
			if err != nil {
				response.Status = false
				response.Errors["total_stocktransfer"] = "Unable to find total amount of stocktransfers:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	response.Meta = map[string]interface{}{}

	response.Meta["total_stocktransfer"] = stocktransferStats.NetTotal
	response.Meta["total_quantity"] = stocktransferStats.TotalQuantity

	if len(stocktransfers) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = stocktransfers
	}

	json.NewEncoder(w).Encode(response)

}

// CreateStockTransfer : handler for POST /stocktransfer
func CreateStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	var stocktransfer *models.StockTransfer
	// Decode data
	if !utils.Decode(w, r, &stocktransfer) {
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

	stocktransfer.CreatedBy = &userID
	stocktransfer.UpdatedBy = &userID
	now := time.Now()
	stocktransfer.CreatedAt = &now
	stocktransfer.UpdatedAt = &now

	//log.Print("stocktransfer.SkipZatcaReporting:")
	//log.Print(stocktransfer.SkipZatcaReporting)
	// Validate data
	stocktransfer.FindNetTotal()

	if errs := stocktransfer.Validate(w, r, "create", nil); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	stocktransfer.FindTotalQuantity()

	err = stocktransfer.UpdateForeignLabelFields()
	if err != nil {
		response.Status = false
		response.Errors["foreign_fields"] = "Error updating foreign fields: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = stocktransfer.MakeCode()
	if err != nil {
		response.Status = false
		response.Errors["code"] = "Error making code: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	stocktransfer.UUID = uuid.New().String()

	err = stocktransfer.Insert()
	if err != nil {
		redisErr := stocktransfer.UnMakeRedisCode()
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

	go func() {
		stocktransfer.CreateProductsStockTransferHistory()
		stocktransfer.SetProductsStock()
		stocktransfer.SetProductsStockTransferStats()
		stocktransfer.SetWarehouseStockTransferStats()
		go stocktransfer.CreateProductsHistory()
		store.NotifyUsers("stocktransfer_updated")
	}()

	stocktransfer, err = store.FindStockTransferByID(&stocktransfer.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = stocktransfer

	json.NewEncoder(w).Encode(response)
}

// UpdateStockTransfer : handler function for PUT /v1/stocktransfer call
func UpdateStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	var stocktransfer *models.StockTransfer
	var stocktransferOld *models.StockTransfer

	params := mux.Vars(r)

	stocktransferID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["stocktransfer_id"] = "Invalid StockTransfer ID:" + err.Error()
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

	stocktransferOld, err = store.FindStockTransferByID(&stocktransferID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_stocktransfer"] = "Unable to find stocktransfer:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	stocktransfer, err = store.FindStockTransferByID(&stocktransferID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_stocktransfer"] = "Unable to find stocktransfer:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &stocktransfer) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	stocktransfer.UpdatedBy = &userID
	now := time.Now()
	stocktransfer.UpdatedAt = &now
	stocktransfer.FindNetTotal()

	// Validate data
	if errs := stocktransfer.Validate(w, r, "update", stocktransferOld); len(errs) > 0 {
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	stocktransfer.FindTotalQuantity()
	stocktransfer.UpdateForeignLabelFields()

	err = stocktransfer.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = stocktransferOld.SetProductsStock()
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["setting_products_stock"] = "Unable to set products stock:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = stocktransfer.SetProductsStock()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["setting_products_stock"] = "Unable to set products stock" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go func() {
		stocktransfer.ClearProductsStockTransferHistory()
		stocktransfer.CreateProductsStockTransferHistory()
		stocktransfer.SetProductsStock()
		stocktransfer.SetProductsStockTransferStats()
		stocktransferOld.SetProductsStockTransferStats()
		stocktransfer.SetWarehouseStockTransferStats()
		stocktransfer.ClearProductsHistory()
		stocktransfer.CreateProductsHistory()
		store.NotifyUsers("stocktransfer_updated")
	}()

	stocktransfer, err = store.FindStockTransferByID(&stocktransfer.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find stocktransfer:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.NotifyUsers("stocktransfer_updated")
	response.Status = true
	response.Result = stocktransfer
	json.NewEncoder(w).Encode(response)
}

// CreateStockTransfer : handler for POST /stocktransfer
func CalculateStockTransferNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var stocktransfer *models.StockTransfer
	// Decode data
	if !utils.Decode(w, r, &stocktransfer) {
		return
	}

	stocktransfer.FindNetTotal()

	response.Status = true
	response.Result = stocktransfer

	json.NewEncoder(w).Encode(response)
}

// ViewStockTransfer : handler function for GET /v1/stocktransfer/<id> call
func ViewStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	stocktransferID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid StockTransfer ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var stocktransfer *models.StockTransfer

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

	stocktransfer, err = store.FindStockTransferByID(&stocktransferID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = stocktransfer

	json.NewEncoder(w).Encode(response)
}

// ViewStockTransfer : handler function for GET /v1/stocktransfer/<id> call
func ViewPreviousStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	stocktransferID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid StockTransfer ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var stocktransfer *models.StockTransfer

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

	stocktransfer, err = store.FindStockTransferByID(&stocktransferID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	previousStockTransfer, err := stocktransfer.FindPreviousStockTransfer(selectFields)
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = previousStockTransfer

	json.NewEncoder(w).Encode(response)
}

// ViewStockTransfer : handler function for GET /v1/stocktransfer/<id> call
func ViewNextStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	stocktransferID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid StockTransfer ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	var stocktransfer *models.StockTransfer

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

	stocktransfer, err = store.FindStockTransferByID(&stocktransferID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	nextStockTransfer, err := stocktransfer.FindNextStockTransfer(selectFields)
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = nextStockTransfer

	json.NewEncoder(w).Encode(response)
}

func ViewLastStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	var stocktransfer *models.StockTransfer

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

	stocktransfer, err = store.FindLastStockTransferByStoreID(&store.ID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = stocktransfer

	json.NewEncoder(w).Encode(response)
}

// DeleteStockTransfer : handler function for DELETE /v1/stocktransfer/<id> call
func DeleteStockTransfer(w http.ResponseWriter, r *http.Request) {
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

	stocktransferID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["quotation_id"] = "Invalid StockTransfer ID:" + err.Error()
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

	stocktransfer, err := store.FindStockTransferByID(&stocktransferID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = stocktransfer.DeleteStockTransfer(tokenClaims)
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
