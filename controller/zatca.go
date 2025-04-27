package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
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

// Define a struct to hold the JSON response
type PythonResponse struct {
	PrivateKey               string `json:"private_key"`
	Csr                      string `json:"csr"`
	CcsidRequestID           int64  `json:"ccsid_requestID"`
	CcsidBinarySecurityToken string `json:"ccsid_binarySecurityToken"`
	CcsidSecret              string `json:"ccsid_secret"`
	PcsidRequestID           int64  `json:"pcsid_requestID"`
	PcsidBinarySecurityToken string `json:"pcsid_binarySecurityToken"`
	PcsidSecret              string `json:"pcsid_secret"`
	Error                    string `json:"error"`
	Traceback                string `json:"traceback,omitempty"`
}

// ConnectStoreToZatc : handler for POST /store/zatca/connect
func ConnectStoreToZatca(w http.ResponseWriter, r *http.Request) {
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

	var zatcaConnectInput *models.ZatcaConnectInput
	// Decode data
	if !utils.Decode(w, r, &zatcaConnectInput) {
		return
	}

	storeID, err := primitive.ObjectIDFromHex(zatcaConnectInput.StoreID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid Store ID:" + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["store_id"] = "Error finding store: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
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

	// Validate data
	if errs := zatcaConnectInput.Validate(w, r); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		json.NewEncoder(w).Encode(response)
		return
	}

	serialNumberTemplate := fmt.Sprintf("%s-%0*d", store.SalesSerialNumber.Prefix, store.SalesSerialNumber.PaddingCount, 1)

	parts := strings.Split(serialNumberTemplate, "-")
	serialNumber := ""
	for k, part := range parts {
		serialNumber += strconv.Itoa((k + 1)) + "-" + part + "|"
	}

	serialNumber += strconv.Itoa((len(parts) + 1)) + "-4bd41220-f619-47bc-830b-7fedd3b33032"

	//log.Print("serialNumber:" + serialNumber)

	countryCode := "SA"
	if store.CountryCode != "" {
		countryCode = store.CountryCode
	}

	// Create JSON payload
	payload := map[string]interface{}{
		//"env":               env.Getenv("ZATCA_ENV", "NonProduction"),
		"env":               store.Zatca.Env,
		"otp":               zatcaConnectInput.Otp,
		"crn":               store.RegistrationNumber,
		"serial_number":     serialNumber,
		"vat":               store.VATNo,
		"name":              store.Name,
		"branch_name":       store.BranchName,
		"country_code":      countryCode,
		"invoice_type":      "1100",
		"address":           store.Address,
		"business_category": store.BusinessCategory,
	}

	// Convert payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		response.Status = false
		response.Errors["marhsallng_json"] = "Error marshalling JSON: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	pythonBinary := "ZatcaPython/venv/bin/python"
	scriptPath := "ZatcaPython/csr_and_onboarding.py"

	// Create command
	cmd := exec.Command(pythonBinary, scriptPath)

	// Set up pipes
	cmd.Stdin = bytes.NewReader(jsonData) // Send JSON data to stdin
	var output bytes.Buffer
	cmd.Stdout = &output // Capture stdout
	cmd.Stderr = &output // Capture stderr

	// Run the command
	err = cmd.Run()
	if err != nil {
		//fmt.Println("Error running Python script:", err)
		response.Status = false
		// Parse JSON response
		var pythonResponse PythonResponse
		err = json.Unmarshal(output.Bytes(), &pythonResponse)
		if err != nil {
			response.Errors["otp"] = "Error parsing error messages from zatca:" + err.Error()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

		//log.Print(pythonResponse.Error)
		if pythonResponse.Error != "" {
			response.Status = false
			response.Errors["otp"] = "Error connecting to zatac: " + pythonResponse.Error
			store.Zatca.ConnectionFailedCount++
			now := time.Now()
			store.Zatca.ConnectionLastFailedAt = &now
			store.Zatca.ConnectionErrors = append(store.Zatca.ConnectionErrors, "Connection failure1: "+pythonResponse.Error)
			err = store.Update()
			if err != nil {
				fmt.Println("Error saving store: ", err)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}
		return
	}

	// Parse JSON response
	var pythonResponse PythonResponse
	err = json.Unmarshal(output.Bytes(), &pythonResponse)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	//log.Print("pythonResponse:")
	//log.Print(pythonResponse)

	if pythonResponse.Error != "" {
		//log.Print(pythonResponse.Error)
		response.Status = false
		response.Errors["otp"] = "Error connecting to zatac: " + pythonResponse.Error
		store.Zatca.ConnectionFailedCount++
		now := time.Now()
		store.Zatca.ConnectionLastFailedAt = &now
		store.Zatca.ConnectionErrors = append(store.Zatca.ConnectionErrors, "Connection failure2: "+pythonResponse.Error)
		err = store.Update()
		if err != nil {
			fmt.Println("Error saving store: ", err)
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	store.Zatca.Otp = zatcaConnectInput.Otp
	store.Zatca.PrivateKey = pythonResponse.PrivateKey
	store.Zatca.Csr = pythonResponse.Csr

	//compliance
	store.Zatca.ComplianceRequestID = pythonResponse.CcsidRequestID
	store.Zatca.BinarySecurityToken = pythonResponse.CcsidBinarySecurityToken
	store.Zatca.Secret = pythonResponse.CcsidSecret

	//production
	store.Zatca.ProductionRequestID = pythonResponse.PcsidRequestID
	store.Zatca.ProductionBinarySecurityToken = pythonResponse.PcsidBinarySecurityToken
	store.Zatca.ProductionSecret = pythonResponse.PcsidSecret

	if !govalidator.IsNull(store.Zatca.PrivateKey) &&
		!govalidator.IsNull(store.Zatca.Csr) &&
		!govalidator.IsNull(store.Zatca.Secret) &&
		!govalidator.IsNull(store.Zatca.BinarySecurityToken) &&
		!govalidator.IsNull(store.Zatca.ProductionSecret) &&
		!govalidator.IsNull(store.Zatca.ProductionBinarySecurityToken) &&
		store.Zatca.ComplianceRequestID > 0 &&
		store.Zatca.ProductionRequestID > 0 {

		store.Zatca.Connected = true
		store.Zatca.ConnectedBy = &userID
		now := time.Now()
		store.Zatca.LastConnectedAt = &now
	}

	err = store.Update()
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["updating_store"] = "Error updating store: " + err.Error()
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Print parsed JSON values
	//fmt.Println("Message from Python:")
	//log.Print(pythonResponse)

	// Print output
	//fmt.Println(string(output))

	/*
		cmd := exec.Command("/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/venv/bin/python", "csr_and_onboarding.py")

		// Capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		// Print output
		fmt.Println(string(output))
	*/

	response.Status = true
	//response.Result = store

	json.NewEncoder(w).Encode(response)

}

// UpdateOrder : handler function for PUT /v1/order call
func ReportOrderToZatca(w http.ResponseWriter, r *http.Request) {
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

	params := mux.Vars(r)

	orderID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["order_id"] = "Invalid Order ID:" + err.Error()
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

	order, err = store.FindOrderByID(&orderID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_order"] = "Unable to find order:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	_, err = primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Validate data
	if errs := order.ValidateZatcaReporting(); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !IsConnectedToInternet() {
		response.Status = false
		response.Errors["internet"] = "not connected to internet"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if store.Zatca.Phase == "2" && store.Zatca.Connected {
		var lastOrder *models.Order

		lastOrder, err = order.FindPreviousOrder(bson.M{})
		if err != nil && err != mongo.ErrNoDocuments && err != mongo.ErrNilDocument {
			response.Status = false
			response.Errors["previous_order"] = "Error finding previous order"
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		if lastOrder != nil {
			if lastOrder.Zatca.ReportingFailedCount > 0 && !lastOrder.Zatca.ReportingPassed {
				response.Status = false
				response.Errors["previous_order"] = "Previous sale is not reported to Zatca. please report it and try again"
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(response)
				return
			}
		}

		err = order.ReportToZatca()
		if err != nil {
			response.Status = false
			response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		err = order.Update()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["update"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

	}

	response.Status = true
	response.Result = order
	json.NewEncoder(w).Encode(response)
}

// UpdateOrder : handler function for PUT /v1/order call
func ReportSalesReturnToZatca(w http.ResponseWriter, r *http.Request) {
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

	var salesReturn *models.SalesReturn

	params := mux.Vars(r)

	salesReturnID, err := primitive.ObjectIDFromHex(params["id"])
	if err != nil {
		response.Status = false
		response.Errors["sales_return_id"] = "invalid sales return ID:" + err.Error()
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

	salesReturn, err = store.FindSalesReturnByID(&salesReturnID, bson.M{})
	if err != nil {
		response.Status = false
		response.Errors["find_sales_return"] = "Unable to find sales return:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	_, err = primitive.ObjectIDFromHex(tokenClaims.UserID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid User ID:" + err.Error()
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Validate data
	if errs := salesReturn.ValidateZatcaReporting(); len(errs) > 0 {
		response.Status = false
		response.Errors = errs
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if !IsConnectedToInternet() {
		response.Status = false
		response.Errors["internet"] = "not connected to internet"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	if store.Zatca.Phase == "2" && store.Zatca.Connected {
		var lastSalesReturn *models.SalesReturn

		lastSalesReturn, err = salesReturn.FindPreviousSalesReturn(bson.M{})
		if err != nil && err != mongo.ErrNoDocuments && err != mongo.ErrNilDocument {
			response.Status = false
			response.Errors["previous_order"] = "Error finding previous sales return"
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		if lastSalesReturn != nil {
			if lastSalesReturn.Zatca.ReportingFailedCount > 0 && !lastSalesReturn.Zatca.ReportingPassed {
				response.Status = false
				response.Errors["previous_sales_return"] = "Previous sales return is not reported to Zatca. please report it and try again"
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(response)
				return
			}
		}

		err = salesReturn.ReportToZatca()
		if err != nil {
			response.Status = false
			response.Errors["reporting_to_zatca"] = "Error reporting to zatca: " + err.Error()
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}

		err = salesReturn.Update()
		if err != nil {
			response.Status = false
			response.Errors = make(map[string]string)
			response.Errors["update"] = "Unable to update:" + err.Error()

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(response)
			return
		}

	}

	response.Status = true
	response.Result = salesReturn
	json.NewEncoder(w).Encode(response)
}

// ConnectStoreToZatc : handler for POST /store/zatca/connect
func DisconnectStoreFromZatca(w http.ResponseWriter, r *http.Request) {
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

	var zatcaConnectInput *models.ZatcaConnectInput
	// Decode data
	if !utils.Decode(w, r, &zatcaConnectInput) {
		return
	}

	storeID, err := primitive.ObjectIDFromHex(zatcaConnectInput.StoreID)
	if err != nil {
		response.Status = false
		response.Errors["user_id"] = "Invalid Store ID:" + err.Error()
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

	store, err := models.FindStoreByID(&storeID, bson.M{})
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["store_id"] = "Error finding store: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	store.Zatca.Connected = false
	store.Zatca.DisconnectedBy = &userID

	now := time.Now()
	store.Zatca.LastDisconnectedAt = &now

	err = store.Update()
	if err != nil {
		fmt.Println("Error:", err)
		response.Status = false
		response.Errors["updating_store"] = "Error updating store: " + err.Error()
		json.NewEncoder(w).Encode(response)
		return
	}

	// Print parsed JSON values
	//fmt.Println("Message from Python:")
	//log.Print(pythonResponse)

	// Print output
	//fmt.Println(string(output))

	/*
		cmd := exec.Command("/Users/sirin/go/src/github.com/sirinibin/ZatcaPython/venv/bin/python", "csr_and_onboarding.py")

		// Capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		// Print output
		fmt.Println(string(output))
	*/

	response.Status = true
	//response.Result = store

	json.NewEncoder(w).Encode(response)

}

func IsConnectedToInternet() bool {
	timeout := 3 * time.Second
	_, err := net.DialTimeout("tcp", "8.8.8.8:53", timeout) // Google's DNS server
	if err != nil {
		return false
	}
	return true
}
