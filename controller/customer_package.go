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

func ListCustomerPackage(w http.ResponseWriter, r *http.Request) {
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

	packages, criterias, err := models.SearchCustomerPackage(w, r)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find customer packages:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Criterias = criterias
	response.TotalCount, err = models.GetTotalCount(criterias.SearchBy, "customer_package")
	if err != nil {
		response.Status = false
		response.Errors["total_count"] = "Unable to find total count:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	if len(packages) == 0 {
		response.Result = []interface{}{}
	} else {
		response.Result = packages
	}
	json.NewEncoder(w).Encode(response)
}

func CreateCustomerPackage(w http.ResponseWriter, r *http.Request) {
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

	var pkg *models.CustomerPackage
	if !utils.Decode(w, r, &pkg) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	now := time.Now()
	pkg.CreatedAt = &now
	pkg.UpdatedAt = &now
	pkg.CreatedBy = &userID

	if errs := pkg.Validate(w, r, "create"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := pkg.Insert(); err != nil {
		response.Status = false
		response.Errors["insert"] = "Unable to insert:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = pkg
	json.NewEncoder(w).Encode(response)
}

func UpdateCustomerPackage(w http.ResponseWriter, r *http.Request) {
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
	pkgID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	pkg, err := models.FindCustomerPackageByID(&pkgID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find package:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !utils.Decode(w, r, &pkg) {
		return
	}

	userID, err := primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}
	pkg.UpdatedBy = &userID

	if errs := pkg.Validate(w, r, "update"); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := pkg.Update(); err != nil {
		response.Status = false
		response.Errors["update"] = "Unable to update:" + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	pkg, err = models.FindCustomerPackageByID(&pkg.ID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find package:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = pkg
	json.NewEncoder(w).Encode(response)
}

func ViewCustomerPackage(w http.ResponseWriter, r *http.Request) {
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
	pkgID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	selectFields := map[string]interface{}{}
	if keys, ok := r.URL.Query()["select"]; ok && len(keys[0]) >= 1 {
		selectFields = models.ParseSelectString(keys[0])
	}

	pkg, err := models.FindCustomerPackageByID(&pkgID, selectFields)
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find package:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	response.Status = true
	response.Result = pkg
	json.NewEncoder(w).Encode(response)
}

func DeleteCustomerPackage(w http.ResponseWriter, r *http.Request) {
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
	pkgID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["id"] = "Invalid ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	pkg, err := models.FindCustomerPackageByID(&pkgID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find"] = "Unable to find package:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if err := pkg.Delete(tokenClaims); err != nil {
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
