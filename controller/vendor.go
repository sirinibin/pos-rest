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
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"
)

// ListVendor : handler for GET /vendor
func ListVendor(w http.ResponseWriter, r *http.Request) {
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

	vendors := []models.Vendor{}

	store, err := ParseStore(r)
	if err != nil {
		response.Status = false
		response.Errors["store_id"] = "Invalid store id:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vendors, criterias, err := store.SearchVendor(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find vendors:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = store.GetTotalCount(criterias.SearchBy, "vendor")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count of vendors:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(vendors) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = vendors
	}

	json.NewEncoder(w).Encode(response)

}

// CreateVendor : handler for POST /vendor
func CreateVendor(w http.ResponseWriter, r *http.Request) {
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

	var vendor *models.Vendor
	// Decode data
	if !utils.Decode(w, r, &vendor) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	vendor.Name = strings.ToUpper(vendor.Name)
	vendor.CreatedBy = &userID
	vendor.UpdatedBy = &userID
	now := time.Now()
	vendor.CreatedAt = &now
	vendor.UpdatedAt = &now

	// Validate data
	if errs := vendor.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	if govalidator.IsNull(strings.TrimSpace(vendor.Code)) {
		err = vendor.MakeCode()
		if err != nil {
			response.Status = false
			response.Errors["code"] = "Error making code: " + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	vendor.GenerateSearchWords()
	vendor.SetSearchLabel()
	vendor.SetAdditionalkeywords()

	err = vendor.Insert()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["insert"] = "Unable to insert to db:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = vendor

	json.NewEncoder(w).Encode(response)

}

// UpdateVendor : handler function for PUT /v1/vendor call
func UpdateVendor(w http.ResponseWriter, r *http.Request) {
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

	var vendor *models.Vendor

	params := mux.Vars(r)

	vendorID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["vendor_id"] = "Invalid Vendor ID:" + err.Error()
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

	vendorOld, err := store.FindVendorByID(&vendorID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vendor, err = store.FindVendorByID(&vendorID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Decode data
	if !utils.Decode(w, r, &vendor) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	vendor.Name = strings.ToUpper(vendor.Name)
	vendor.UpdatedBy = &userID
	now := time.Now()
	vendor.UpdatedAt = &now

	// Validate data
	if errs := vendor.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	vendor.GenerateSearchWords()
	vendor.SetSearchLabel()
	vendor.SetAdditionalkeywords()

	err = vendor.Update()
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["update"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	err = vendor.AttributesValueChangeEvent(vendorOld)
	if err != nil {
		response.Status = false
		response.Errors = make(map[string]string)
		response.Errors["attributes_value_change"] = "Unable to update:" + err.Error()

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	vendor, err = store.FindVendorByID(&vendor.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to find vendor:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = vendor

	json.NewEncoder(w).Encode(response)
}

// ViewVendor : handler function for GET /v1/vendor/<id> call
func ViewVendor(w http.ResponseWriter, r *http.Request) {
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

	vendorID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["vendor_id"] = "Invalid Vendor ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	var vendor *models.Vendor

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

	vendor, err = store.FindVendorByID(&vendorID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if vendor.VATNo != "" {
		account, err := store.FindAccountByVatNo(vendor.VATNo, &store.ID, bson.M{})
		if err != nil && err != mongo.ErrNoDocuments {
			response.Status = false
			response.Errors["account"] = "error finding account:" + err.Error()
			json.NewEncoder(w).Encode(response)
			return
		}

		if account != nil {
			vendor.CreditBalance = account.Balance
		} else {
			account, err = store.FindAccountByPhoneByName(vendor.Phone, vendor.Name, &store.ID, bson.M{})
			if err != nil && err != mongo.ErrNoDocuments {
				response.Status = false
				response.Errors["account"] = "error finding account:" + err.Error()
				json.NewEncoder(w).Encode(response)
				return
			}
			if account != nil {
				vendor.CreditBalance = account.Balance
			}
		}

	}

	response.Status = true
	response.Result = vendor

	json.NewEncoder(w).Encode(response)

}

// DeleteVendor : handler function for DELETE /v1/vendor/<id> call
func DeleteVendor(w http.ResponseWriter, r *http.Request) {
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

	vendorID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["vendor_id"] = "Invalid Vendor ID:" + err.Error()
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

	vendor, err := store.FindVendorByID(&vendorID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = vendor.DeleteVendor(tokenClaims)
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
func RestoreVendor(w http.ResponseWriter, r *http.Request) {
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

	vendorID, err := primitive.ObjectIDFromHex(params["id"])
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

	vendor, err := store.FindVendorByID(&vendorID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["view"] = "Unable to view:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	err = vendor.RestoreVendor(tokenClaims)
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
