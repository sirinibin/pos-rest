package controller

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gorilla/mux"
	"github.com/sirinibin/pos-rest/models"
	"github.com/sirinibin/pos-rest/utils"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"gopkg.in/mgo.v2/bson"
)

// ListCustomer : handler for GET /customer
func ListCustomer(w http.ResponseWriter, r *http.Request) {
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

	Customers := []models.Customer{}
	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	Customers, criterias, err := store.SearchCustomer(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find Customers:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "customer")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of customers:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(Customers) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = Customers
	}

	json.NewEncoder(w).Encode(response)

}

// CreateCustomer : handler for POST /customer
func CreateCustomer(w http.ResponseWriter, r *http.Request) {
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

	var customer *models.Customer
	// Decode data
	if !utils.Decode(w, r, &customer) {
		return
	}

	// Validate data
	if errs := customer.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customer.Name = strings.ToUpper(customer.Name)
	customer.CreatedBy = &userID
	customer.UpdatedBy = &userID
	now := time.Now()
	customer.CreatedAt = &now
	customer.UpdatedAt = &now
	customer.UpdateForeignLabelFields()

	if govalidator.IsNull(strings.TrimSpace(customer.Code)) {
		err = customer.MakeCode()
		if err != nil {
			response.Status = false
			response.Errors["code"] = "Error making code: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}
	customer.GenerateSearchWords()

	err = customer.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customer

	json.NewEncoder(w).Encode(response)

}

// UpdateCustomer : handler function for PUT /v1/customer call
func UpdateCustomer(w http.ResponseWriter, r *http.Request) {
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

	var customer *models.Customer
	var customerOld *models.Customer

	params := mux.Vars(r)

	customerID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customer_id"] = "Invalid Customer ID:" + err.Error()
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

	customerOld, err = store.FindCustomerByID(&customerID, nil)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, err = store.FindCustomerByID(&customerID, nil)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &customer) {
		return
	}

	// Validate data
	if errs := customer.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	customer.Name = strings.ToUpper(customer.Name)
	customer.UpdatedBy = &userID
	now := time.Now()
	customer.UpdatedAt = &now
	customer.UpdateForeignLabelFields()
	customer.GenerateSearchWords()

	err = customer.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customer.AttributesValueChangeEvent(customerOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	customer, err = store.FindCustomerByID(&customer.ID, nil)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find customer:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = customer

	json.NewEncoder(w).Encode(response)
}

// ViewCustomer : handler function for GET /v1/customer/<id> call
func ViewCustomer(w http.ResponseWriter, r *http.Request) {
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

	customerID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customer_id"] = "Invalid Customer ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var customer *models.Customer

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

	customer, err = store.FindCustomerByID(&customerID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	/*
		if customer.VATNo != "" {
			account, err := store.FindAccountByVatNo(customer.VATNo, &store.ID, bson.M{})
			if err != nil && err != mongo.ErrNoDocuments {
				response.Status = false
				response.Errors["account"] = "error finding account:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}

			if account != nil {
				customer.CreditBalance = account.Balance
			} else {
				account, err = store.FindAccountByPhoneByName(customer.Phone, customer.Name, &store.ID, bson.M{})
				if err != nil && err != mongo.ErrNoDocuments {
					response.Status = false
					response.Errors["account"] = "error finding account:" + err.Error()
					json.NewEncoder(w).Encode(response)
					return
				}
				if account != nil {
					customer.CreditBalance = account.Balance
				}
			}

		}*/

	customer.SetSearchLabel()

	response.Status = true
	response.Result = customer

	json.NewEncoder(w).Encode(response)

}

// DeleteCustomer : handler function for DELETE /v1/customer/<id> call
func DeleteCustomer(w http.ResponseWriter, r *http.Request) {
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

	customerID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["customer_id"] = "Invalid Customer ID:" + err.Error()
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

	customer, err := store.FindCustomerByID(&customerID, nil)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customer.DeleteCustomer(tokenClaims)
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

// RestoreCustomer : handler function for POST /v1/customer/<id> call
func RestoreCustomer(w http.ResponseWriter, r *http.Request) {
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

	customerID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["product_id"] = "Invalid Product ID:" + err.Error()
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

	customer, err := store.FindCustomerByID(&customerID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = customer.RestoreCustomer(tokenClaims)
	if err != nil {
		response.Status = false
		response.Errors["delete"] = "Unable to delete:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = "Restored successfully"

	json.NewEncoder(w).Encode(response)
}
