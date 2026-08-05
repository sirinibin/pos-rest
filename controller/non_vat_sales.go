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
	"go.mongodb.org/mongo-driver/mongo"
)

func ListNonVATSales(w http.ResponseWriter, r *http.Request) {
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

	items, criterias, err := store.SearchNonVATSales(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "non_vat_sales")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	keys, ok := r.URL.Query()["search[stats]"]
	if ok && len(keys[0]) >= 1 && keys[0] == "1" {
		stats, err := store.GetNonVATSalesStats(criterias.SearchBy)
		if err == nil {
			response.Meta["net_total"] = stats.NetTotal
			response.Meta["net_profit"] = stats.NetProfit
			response.Meta["net_loss"] = stats.NetLoss
			response.Meta["vat_price"] = stats.VatPrice
			response.Meta["discount"] = stats.Discount
		}
	}

	if len(items) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = items
	}
	json.NewEncoder(w).Encode(response)
}

func CreateNonVATSales(w http.ResponseWriter, r *http.Request) {
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

	var item *models.NonVATSales
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

	queue := GetOrCreateQueue(store.ID.Hex(), "non_vat_sales")
	queueToken := generateQueueToken()
	queue.Enqueue(Request{Token: queueToken})
	queue.WaitUntilMyTurn(queueToken)

	if errs := item.Validate(w, r, "create"); len(errs) > 0 {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales")
		w.WriteHeader(http.StatusBadRequest)
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	item.FindTotalQuantity()

	if err = item.UpdateForeignLabelFields(); err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales")
		response.Status = false
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err = item.MakeRedisCode(); err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales")
		response.Status = false
		response.Errors["code"] = "error making code: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	item.ProcessPayments()

	if err = item.Insert(); err != nil {
		queue.Pop()
		CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales")
		_ = item.UnMakeRedisCode()
		response.Status = false
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	queue.Pop()
	CleanupQueueIfEmpty(store.ID.Hex(), "non_vat_sales")

	// Link to repair job(s)
	if item.RepairJobID != nil && !item.RepairJobID.IsZero() {
		store.LinkNonVATSalesToRepairJob(item.RepairJobID, item.ID, item.Code, item.NetTotal)
	}
	for _, jobID := range item.RepairJobIDs {
		jid := jobID
		store.LinkNonVATSalesToRepairJob(&jid, item.ID, item.Code, item.NetTotal)
	}

	go func() {
		_ = item.CreateProductsNonVATSalesHistory()
		_ = item.SetProductsStock()
		_ = item.DoAccounting()
		_ = item.SetCustomerNonVATSalesStats()
	}()

	response.Status = true
	response.Result = item
	json.NewEncoder(w).Encode(response)
}

func ViewNonVATSales(w http.ResponseWriter, r *http.Request) {
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

	item, err := store.FindNonVATSalesByID(&ID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	item.Store = store
	item.CalculateProfit()

	response.Status = true
	response.Result = item
	json.NewEncoder(w).Encode(response)
}

func UpdateNonVATSales(w http.ResponseWriter, r *http.Request) {
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

	itemOld, err := store.FindNonVATSalesByID(&ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var item *models.NonVATSales
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

	if err = item.UpdateForeignLabelFields(); err != nil {
		response.Status = false
		response.Errors["update_foreign_fields"] = "error updating foreign fields: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	item.ProcessPayments()

	if err = item.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	go func() {
		_ = item.UndoAccounting()
		_ = item.ClearProductsNonVATSalesHistory()
		_ = item.CreateProductsNonVATSalesHistory()
		_ = item.SetProductsStock()
		_ = item.DoAccounting()
		_ = item.SetCustomerNonVATSalesStats()
	}()

	response.Status = true
	response.Result = item
	json.NewEncoder(w).Encode(response)
}

func DeleteNonVATSales(w http.ResponseWriter, r *http.Request) {
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

	item, err := store.FindNonVATSalesByID(&ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find non vat sales:" + err.Error()
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
		_ = item.ClearProductsNonVATSalesHistory()
		_ = item.SetProductsStock()
		_ = item.SetCustomerNonVATSalesStats()
	}()

	response.Status = true
	response.Result = "Deleted successfully"
	json.NewEncoder(w).Encode(response)
}

func ViewPreviousNonVATSale(w http.ResponseWriter, r *http.Request) {
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
	saleID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	selectFields := map[string]interface{}{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	sale, err := store.FindNonVATSalesByID(&saleID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	prev, err := sale.FindPreviousNonVATSale(selectFields)
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors["view"] = "Unable to find previous:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if prev != nil {
		customer, _ := store.FindCustomerByID(prev.CustomerID, bson.M{})
		customer.SetSearchLabel()
		prev.Customer = customer
	}

	response.Status = true
	response.Result = prev
	json.NewEncoder(w).Encode(response)
}

func ViewNextNonVATSale(w http.ResponseWriter, r *http.Request) {
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
	saleID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	selectFields := map[string]interface{}{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	sale, err := store.FindNonVATSalesByID(&saleID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	next, err := sale.FindNextNonVATSale(selectFields)
	if err != nil && err != mongo.ErrNoDocuments {
		response.Status = false
		response.Errors["view"] = "Unable to find next:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if next != nil {
		customer, _ := store.FindCustomerByID(next.CustomerID, bson.M{})
		customer.SetSearchLabel()
		next.Customer = customer
	}

	response.Status = true
	response.Result = next
	json.NewEncoder(w).Encode(response)
}

func ViewLastNonVATSale(w http.ResponseWriter, r *http.Request) {
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

	selectFields := map[string]interface{}{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	sale, err := store.FindLastNonVATSaleByStoreID(&store.ID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find last:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, _ := store.FindCustomerByID(sale.CustomerID, bson.M{})
	customer.SetSearchLabel()
	sale.Customer = customer

	response.Status = true
	response.Result = sale
	json.NewEncoder(w).Encode(response)
}

func CalculateNonVATSalesNetTotal(w http.ResponseWriter, r *http.Request) {
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

	var sale *models.NonVATSales
	if !utils.Decode(w, r, &sale) {
		return
	}

	if err := sale.FindNetTotal(); err != nil {
		response.Status = false
		response.Errors["net_total"] = "Unable to calculate net total:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = sale
	json.NewEncoder(w).Encode(response)
}
